# Lottery Search System - Design Document

## 1. Summary

This is my design for a lottery ticket search and allocation system. A user searches
6-digit ticket numbers with a pattern like `****23`. The system returns matching tickets
and holds them, so two users are never offered the same ticket at the same time.

The core idea: **I calculate the matching tickets, I don't search for them.** A 6-digit
number has exactly 1,000,000 possible values. So the set of numbers matching any pattern is
arithmetic. I never scan the ticket dataset to find matches. I only answer a much smaller
question: of the numbers that match, which ones still have a ticket available?

That removes the need for a wildcard index over 10 million records, which is the hardest
part of this problem otherwise.

---

## 2. Assumptions

The spec leaves several points open. I state each assumption with the reason I picked it.
The design doesn't depend on the exact values.

### 2.1 The dataset contains duplicate numbers

The spec says 10 million tickets, each a 6-digit number. There are only 1,000,000 possible
6-digit values, so the dataset must repeat numbers, roughly 10 copies of each on average.
That matches how lottery tickets are sold in real life, where many physical copies of the
same number exist.

**Consequence:** a ticket is not identified by its number. Each ticket has its own
identifier and the number is an attribute of it. "Allocate 100023" means "allocate one
available copy of 100023".

### 2.2 A search returns a page of 20 tickets

The spec doesn't say how many tickets a search returns. Returning every match doesn't work:
`****23` matches around 100,000 tickets at this scale, and holding all of them for one user
would freeze a large part of the inventory.

I chose 20 as a realistic browsing unit. The page size is configurable.

### 2.3 Exclusivity is global, not per-pattern

The spec says the same search pattern should not return the same ticket to multiple users
at the same time. Read strictly, that only constrains searches using the same pattern.

That reading doesn't hold. `****23` and `1***23` both match `100023`. If exclusivity
applied only within a pattern, two users on different patterns could be offered the same
ticket.

**So I apply exclusivity globally.** Once a user holds a ticket, nobody else can get it, no
matter which pattern they searched.

### 2.4 Allocation is a temporary hold with a 5-minute expiry

The spec says tickets must not go to multiple users *at the same time*, which means the
hold is temporary. No lifecycle is given, so I define one:

```
available  ->  reserved (5 min)  ->  sold
     ^                |
     +------- expired -+
```

Five minutes is long enough to look at 20 numbers and short enough that abandoned holds
come back quickly. An abandoned page frees itself twelve times an hour instead of twice.

The hold extends to 15 minutes once the user starts checkout.

### 2.5 A new search releases the previous page

Without this rule a user could hold unlimited tickets by searching over and over. Each
search would lock 20 more for five minutes.

Releasing the previous page on a new search caps each user at one active page.

### 2.6 One active page per user

This follows from 2.5, but I state it separately because it bounds the worst case: the
maximum inventory ever held is *(users holding reservations) x 20*.

Purchased tickets are not capped. The limit is on holds only.

### 2.7 Load assumptions

Two separate figures, because they answer different questions.

| Assumption | Question it answers |
|---|---|
| 1,000 searches per second at peak | Can the service handle the request load |
| 10,000 users holding reservations | Does the inventory look starved |

Lottery sales spike hard in the days before a draw, so peak load sits far above average.

### 2.8 Users are the same users as in Part 1

The spec doesn't connect the two parts. I assume one user identity across both.

---

## 3. Core mechanism: candidate generation

### 3.1 Why no wildcard index is needed

A pattern has 6 positions, each a digit or `*`, so 2^6 = 64 wildcard shapes. Against a
normal index on the ticket number they fall into three cases:

| Case | Example | Behaviour with a normal index |
|---|---|---|
| Fixed prefix | `123***` | Range scan, efficient |
| Fixed suffix | `***123` | No help - matches are scattered |
| Interior wildcards | `1**4*3` | No help |

One way to cover all three is to precompute all 64 masked forms of every ticket and index
them. At 10 million tickets that is 640 million index entries. Not reasonable.

The other way is to notice I don't need to discover the match set at all. For `****23` the
matching numbers are:

```
000023, 000123, 000223, 000323, ... , 999923
```

That is 10,000 numbers. I count the wildcard positions from 0000 to 9999 and append `23`.
No storage is touched. The pattern gives me the candidate set completely.

### 3.2 What the system actually looks up

The only question left is which candidates still have an available ticket, and which
specific ticket to hand out. A small availability structure answers that. I never search
the ticket dataset.

### 3.3 Search flow

1. Generate candidate numbers from the pattern, in a per-session order
2. Filter them against an in-memory availability bitmap
3. Batch-check the survivors in Redis
4. Take one ticket identifier per number, atomically
5. Record the reservations in MongoDB
6. Return 20 tickets and a cursor

I only need 20 results, so generation stops early. For a pattern with healthy stock I look
at roughly 20 candidates, not 10,000.

---

## 4. Storage design

### 4.1 Split of responsibility

| Store | Holds | Role |
|---|---|---|
| Redis | Available ticket identifiers, grouped by number | Decides who gets which ticket |
| MongoDB | Tickets, reservations, orders | Durable record of what happened |

MongoDB is the source of truth. Redis is the allocator. When the two disagree, I correct
Redis towards MongoDB, never the other way.

### 4.2 Redis structures

**Availability sets** - one set per number, holding the identifiers of tickets still
available:

```
avail:000023 -> { 4471023, 4471024, 4471025, ... }
```

Allocation is a single atomic command:

```
SPOP avail:000023
```

`SPOP` removes and returns one random member. If fifty users call it at the same moment,
each one gets a different identifier, and once the set is empty the rest get nothing.
**Redis enforces the exclusivity guarantee, not my application code.** So I need no
distributed lock on the allocation path.

Returning a random member also spreads users across the available tickets instead of making
them all fight over the first one.

The available count is the set cardinality (`SCARD`), so I keep no separate counter.

**Expiry index** - a sorted set of held tickets scored by expiry time:

```
ZADD reservations <expires_at> 4471023
```

**Availability bitmap** - one bit per number, saying whether any copy is left. 1,000,000
bits is 125 KB, small enough to keep in every service instance and refresh periodically.
Candidate filtering then runs at memory speed and only likely candidates reach Redis.

The bitmap is allowed to be stale, because it never allocates anything. It only decides
where to look. Both directions of staleness are safe:

| Bitmap says | Reality | Effect |
|---|---|---|
| No stock | Stock exists | The number is skipped this page. It comes back after the next refresh. |
| Has stock | Sold out | `SPOP` returns nothing and the number is skipped. One wasted lookup. |

Neither one can double-allocate, because only `SPOP` allocates and it acts on real state.

**Degraded path.** The failure worth designing for is the refresh job dying. The bitmap
would freeze and drift, and searches would quietly return fewer and fewer results while
inventory was actually fine. No error, just worse and worse output. So the bitmap carries
the timestamp of its last refresh. Once that is older than 30 seconds I bypass the filter
and send candidates straight to Redis. Searches get slower but stay correct.

### 4.3 MongoDB collections

**tickets**

| Field | Notes |
|---|---|
| `_id` | Numeric identifier (see 6.2) |
| `number` | 6-digit string |
| `status` | `available` / `reserved` / `sold` |
| `price` | |

**reservations**

| Field | Notes |
|---|---|
| `ticket_id` | |
| `user_id` | |
| `search_id` | Groups the 20 tickets of one page |
| `expires_at` | |
| `status` | `active` / `expired` / `completed` |

**orders** - a completed purchase, referencing the reservations it fulfilled.

Reservations are their own collection instead of fields on the ticket. The expiry sweep is
then a query over roughly 200,000 live reservations instead of a scan across 10 million
tickets.

Ownership lives in one place only. A purchase promotes a reservation to `completed` and
creates an order. It doesn't create a second parallel record of who owns what.

**Indexes**

- `{ expires_at: 1, status: 1 }` - for the expiry sweeper
- `{ number: 1, status: 1 }` - for rebuilding Redis
- `{ user_id: 1, status: 1 }` - for listing a user's holds

### 4.4 Why not a MongoDB TTL index on reservations

A TTL index looks like the natural fit for expiry, but it deletes the document instead of
telling the application. Nothing would return the ticket to Redis, and the record I need
for reconciliation would be gone. I use an explicit sweeper instead.

---

## 5. Reservation expiry and consistency

### 5.1 The problem

`SPOP` removes a ticket identifier from its availability set. A set member can't carry its
own time-to-live, so nothing in Redis brings it back. If the user never buys, the ticket is
lost unless I actively restore it.

### 5.2 Fast path - expiry sorted set

A worker runs every second:

```
ZRANGEBYSCORE reservations -inf <now>
```

For each expired identifier it does `SADD` back to the availability set and removes the
entry, both in one Lua script so the pair is atomic. The sweep is cheap because it only
touches entries that actually expired.

### 5.3 Slow path - reconciliation against MongoDB

A second job runs every minute over reservations that are past `expires_at` but still
active. It marks them expired and restores the tickets.

This one exists for the case where `SPOP` succeeded but the MongoDB write failed. That
ticket would otherwise be held by nobody and visible to nobody.

### 5.4 Why running both is safe

Restoration is idempotent by construction. A Redis set can't hold duplicates, so `SADD` of
an identifier that is already there is a no-op. Both jobs can restore the same ticket with
no harm.

### 5.5 Alternative considered - keyspace notifications

Storing each reservation as a key with a TTL and listening for expiry events reads nicely,
but Redis delivers those events at most once. If the listener is restarting when an event
fires, the ticket is silently lost. Fine for a cache. Not fine for inventory.

---

## 6. Performance analysis

### 6.1 Search cost

The worst case is a pattern whose matches are almost all sold. Finding 20 available tickets
may mean looking at thousands of candidates.

Four mitigations, using `****23` with 2,000 candidates examined as the example:

| Approach | Redis round trips | Approximate time |
|---|---|---|
| One lookup per candidate | 2,000 | ~400 ms |
| Batched `MGET` in chunks of 500 | 4 | ~1 ms |
| Bitmap filter first | ~1 | <1 ms |
| Exhausted-pattern cache hit | 1 | ~0.2 ms |

**Batching** is the one I can't skip. **The bitmap** drops most candidates before any
network call. **A scan cap** of 5,000 candidates bounds worst-case latency: the search
returns what it found plus a cursor instead of running unbounded. **Caching exhausted
patterns** for 30 seconds stops repeated full walks of a sold-out pattern.

### 6.2 Memory footprint

Roughly 1,000,000 Redis sets of about 10 members each. Sets this small are stored
compactly, so the cost comes down to whether identifiers are integers or strings.

| Identifier type | Encoding | Per number | Total |
|---|---|---|---|
| Integer (1-10,000,000) | intset | ~156 bytes | **~156 MB** |
| 24-character hex string | listpack | ~461 bytes | **~461 MB** |

Integer identifiers cost about a third as much. That is why tickets use a numeric
identifier and not a 24-character one.

The rest: the expiry sorted set at around 20 MB for 200,000 live reservations, and the
bitmap at 125 KB. Both are negligible.

These are estimates. Real overhead moves with Redis version, allocator and configuration. I
would check it with `MEMORY USAGE` on a sample and `INFO memory` under load.

### 6.3 Inventory pressure

With 10,000 users each holding 20 tickets, 200,000 tickets are held at any moment, 2% of
the dataset. Nobody notices that.

At 100,000 concurrent users it is 2,000,000, or 20% of inventory. Popular patterns would
start to look sold out even though nothing was bought. **That is where reserve-on-search
stops working**, and where the variant in section 8 is needed.

### 6.4 Request load

At 1,000 searches per second, each reserving up to 20 tickets, the allocation path handles
up to 20,000 atomic state changes per second. That number is the reason allocation lives in
Redis and not in the primary database.

---

## 7. Failure and recovery

### 7.1 Rebuilding Redis

If Redis is lost, allocation can't run safely until it is rebuilt. That is an availability
problem, not a cache-warming one.

The naive rebuild, scan all available tickets in MongoDB and populate the sets, has a
correctness flaw. If the scan runs from 10:00 to 10:03, a ticket sold at 10:01 that the
cursor already passed gets written back as available. Then it sells twice.

**What I do:** open a MongoDB change stream before the scan starts, buffer the events, run
the scan, then apply the buffered events in order. Nothing is missed and I need no
downtime.

Alternatives I considered: a read-only window during the rebuild (correct, but it costs
downtime at the worst possible moment), and rebuild-then-reconcile using a timestamp
(workable, but still needs a brief pause).

**Conflict bias:** wherever the two stores disagree, resolve towards *unavailable*. A
ticket wrongly marked sold is a lost sale. A ticket wrongly marked available is sold twice
and becomes a payment dispute. The asymmetry is on purpose.

**Operational points**

- Searches keep working during a rebuild; reservations are rejected
- Rebuild into a separate key prefix and swap atomically, so a partial state is never live
- Set `maxmemory-policy noeviction` on this instance - silently evicting availability sets
  is a correctness bug, not a performance one
- Enable AOF persistence so most restarts need no rebuild
- The reconciler from 5.3 keeps running afterwards and catches leftover drift

---

## 8. Production consideration: browse versus allocate

The design above reserves tickets the moment a search returns them, which is what the
requirement describes.

At scale that has a cost worth naming. If 10,000 users are browsing and every search
reserves 20 tickets, 200,000 tickets are held at any moment, and most of those users will
never buy. Past roughly that point the inventory looks sold out while nothing was sold, as
in 6.3.

A production system would usually split the two actions:

- **Search** returns matching tickets as a read-only view. Nothing is held.
- **Reserve** happens when the user picks a specific ticket.

The exclusivity guarantee doesn't change. No two users can ever hold or buy the same
ticket. But locking now applies to real intent instead of browsing. Contention drops by
roughly the ratio of browsers to buyers.

I describe both because the choice depends on whether the constraint applies to the search
response or to the allocation itself. The requirement's wording, that a search should not
*return* the same ticket to multiple users, points at the first. So that is my primary
design.

---

## 9. API contract

Under reserve-on-search, **a search changes state**. So it is a `POST`, it is not cacheable
and it is not safe to retry blindly. That falls straight out of the requirement.

### 9.1 Endpoints

```
POST   /lottery/searches          run a search, hold a page
GET    /lottery/reservations      list current holds
DELETE /lottery/reservations      release current holds
POST   /lottery/purchases         buy from current holds
```

### 9.2 Search

Request:

```json
{ "pattern": "****23", "cursor": null }
```

Response:

```json
{
  "tickets": [
    { "ticket_id": 4471023, "number": "447123", "price": 80 }
  ],
  "reservation_expires_at": "2026-08-20T10:35:00Z",
  "cursor": "eyJwIjoiKioqKjIzIiwibiI6NDQ3MTIzfQ",
  "has_more": true
}
```

I return no total count. Over live inventory I can't give an exact total honestly, and
`has_more` is the accurate substitute.

**Pattern validation:** exactly 6 characters, each one a digit or `*`. Anything else gets
`400`.

I allow the all-wildcard pattern `******`. It is valid by the spec's own definition, and
with candidate generation it isn't expensive: available tickets show up almost immediately.
The risk it looks like it carries is really a request-volume risk, and a script firing
`1*****`, `2*****`, `3*****` would do the same thing. Rate limiting is the right control
and it covers every case.

### 9.3 Pagination

Offset pagination doesn't work here. If page 2 is "skip 20, take 20" and one ticket from
page 1 sells in between, every later item shifts and some are never shown.

The cursor is a base64 value carrying:

- the **pattern**, so a later page can't silently change it
- the **last candidate reached**, so paging resumes by value instead of by count
- the **ordering seed**, so the per-session order stays consistent

It is opaque to clients and can be signed to stop tampering.

### 9.4 Purchase

Purchases are **all-or-nothing**. If any ticket in the request isn't validly held by the
caller, nothing is bought and the call returns `409`.

The reason is money. A failed call that changed nothing is trivial to retry. A partly
successful one isn't, and partial success plus payment plus retries is exactly where
double-charge bugs live. The cost to the user is small, because a purchase happens inside a
live reservation window where the tickets are already held.

**Idempotency:** the client sends an `Idempotency-Key`. I store the result against that key,
and a replayed request gets the original response back instead of buying twice.

### 9.5 Expired holds

A user seeing a ticket and then finding it gone when they act is routine, not exceptional:

```json
{
  "error": "reservation_expired",
  "message": "Your hold on these tickets has expired. Please search again."
}
```

Returned as `409 Conflict`.

### 9.6 Rate limiting

Each search holds 20 tickets, so an unlimited search rate is a way to freeze inventory
without buying anything. Here rate limiting is a correctness control, not only a protective
one.

---

## 10. Summary of rejected alternatives

| Rejected | Reason |
|---|---|
| Precomputed 64-mask index | 640 million entries at this scale |
| Per-pattern exclusivity | Overlapping patterns would double-allocate |
| Returning all matches | One user could freeze a large share of inventory |
| Offset pagination | Concurrent sales cause items to be skipped |
| MongoDB TTL index for expiry | Deletes the record instead of releasing the ticket |
| Redis keyspace notifications | At-most-once delivery loses inventory |
| Partial purchase | Payment plus partial success invites double charges |
| Rejecting `******` | Addresses the wrong risk; rate limiting is the real control |
