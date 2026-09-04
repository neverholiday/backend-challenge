# Lottery Search System - Design Document

## 1. Summary

This document describes a design for a lottery ticket search and allocation
system. Users search for 6-digit ticket numbers using a pattern such as
`****23`, and the system returns matching tickets that are held for them so
that no two users are ever offered the same ticket at the same time.

The central idea of the design is that **matching tickets are calculated, not
searched for**. A 6-digit number has exactly 1,000,000 possible values, so the
set of numbers matching any pattern can be produced by arithmetic. The system
never scans the ticket dataset to find matches. It only needs to answer a much
smaller question: of the numbers that match, which still have a ticket
available?

This removes the need for any wildcard index over 10 million records, which is
otherwise the hardest part of the problem.

---

## 2. Assumptions

The specification leaves several points open. Each assumption below is stated
with the reason it was chosen. The design does not depend on the exact values.

### 2.1 The dataset contains duplicate numbers

The specification describes 10 million tickets, each a 6-digit number. There
are only 1,000,000 possible 6-digit values, so the dataset must contain
repeated numbers - roughly 10 copies of each on average. This matches how
lottery tickets are sold in practice, where many physical copies of the same
number exist.

**Consequence:** a ticket is not identified by its number. Each ticket has its
own identifier, and the number is an attribute of it. "Allocate 100023" means
"allocate one available copy of 100023".

### 2.2 A search returns a page of 20 tickets

The specification does not say how many tickets a search returns. Returning
every match is not workable: `****23` matches around 100,000 tickets at this
scale, and holding all of them for one user would freeze a large part of the
inventory.

A page of 20 was chosen as a realistic browsing unit. The page size is
configurable.

### 2.3 Exclusivity is global, not per-pattern

The specification says the same search pattern should not return the same
ticket to multiple users at the same time. Read strictly, this constrains only
searches using the same pattern.

That reading does not hold. The patterns `****23` and `1***23` both match
`100023`. If exclusivity applied only within a pattern, two users searching
different patterns could be offered the same ticket.

**This design applies exclusivity globally.** Once a ticket is held by a user,
it is unavailable to everyone, regardless of which pattern they searched.

### 2.4 Allocation is a temporary hold with a 5-minute expiry

The specification says tickets must not be returned to multiple users *at the
same time*, which implies the hold is temporary. No lifecycle is given, so one
is defined here:

```
available  ->  reserved (5 min)  ->  sold
     ^                |
     +------- expired -+
```

Five minutes is long enough to consider 20 numbers and short enough that
abandoned holds return to circulation quickly. An abandoned page frees itself
twelve times an hour rather than twice.

The hold is extended to 15 minutes once the user begins checkout.

### 2.5 A new search releases the previous page

Without this rule, a user could hold unlimited tickets by repeatedly
searching. Each search would lock 20 more for five minutes.

Releasing the previous page on a new search caps each user at one active page.

### 2.6 One active page per user

This follows from 2.5 but is stated separately because it bounds the worst
case: the maximum inventory ever held is *(users holding reservations) x 20*.

Purchased tickets are not capped. The limit applies to holds only.

### 2.7 Load assumptions

Two separate figures, because they answer different questions.

| Assumption | Question it answers |
|---|---|
| 1,000 searches per second at peak | Can the service handle the request load |
| 10,000 users holding reservations | Does the inventory appear starved |

Lottery sales are strongly peaked in the days before a draw, so peak load is
far above average.

### 2.8 Users are the same users as in Part 1

The specification does not connect the two parts. This design assumes a single
user identity across both.

---

## 3. Core mechanism: candidate generation

### 3.1 Why no wildcard index is needed

A pattern has 6 positions, each a digit or `*`, giving 2^6 = 64 possible
wildcard shapes. These fall into three cases when queried against a
conventional index on the ticket number:

| Case | Example | Behaviour with a normal index |
|---|---|---|
| Fixed prefix | `123***` | Range scan, efficient |
| Fixed suffix | `***123` | No help - matches are scattered |
| Interior wildcards | `1**4*3` | No help |

One way to handle all three is to precompute all 64 masked forms of every
ticket and index them. At 10 million tickets that is 640 million index
entries, which is not reasonable.

The alternative is to notice that the match set does not need to be discovered
at all. For `****23`, the matching numbers are:

```
000023, 000123, 000223, 000323, ... , 999923
```

That is 10,000 numbers, produced by counting the wildcard positions from 0000
to 9999 and appending `23`. No storage is consulted. The pattern determines
the candidate set completely.

### 3.2 What the system actually looks up

The only question left is which candidates still have an available ticket, and
which specific ticket to hand out. That is answered by a small availability
structure rather than by searching the ticket dataset.

### 3.3 Search flow

1. Generate candidate numbers from the pattern, in a per-session order
2. Filter them against an in-memory availability bitmap
3. Batch-check the survivors in Redis
4. Take one ticket identifier per number, atomically
5. Record the reservations in MongoDB
6. Return 20 tickets and a cursor

Because only 20 results are needed, generation stops early. For a pattern with
healthy stock, roughly 20 candidates are examined, not 10,000.

---

## 4. Storage design

### 4.1 Split of responsibility

| Store | Holds | Role |
|---|---|---|
| Redis | Available ticket identifiers, grouped by number | Decides who gets which ticket |
| MongoDB | Tickets, reservations, orders | Durable record of what happened |

MongoDB is authoritative. Redis is the allocator. Where the two disagree,
Redis is corrected towards MongoDB, never the reverse.

### 4.2 Redis structures

**Availability sets** - one set per number, holding the identifiers of tickets
still available:

```
avail:000023 -> { 4471023, 4471024, 4471025, ... }
```

Allocation is a single atomic command:

```
SPOP avail:000023
```

`SPOP` removes and returns one random member. If fifty users call it at the
same moment, each receives a different identifier, and once the set is empty
the remaining callers receive nothing. **The exclusivity guarantee is enforced
by Redis rather than by application logic**, which removes the need for
distributed locking on the allocation path.

Returning a random member also spreads users across the available tickets
rather than causing them all to contend for the first one.

The available count is the set cardinality (`SCARD`), so no separate counter
is maintained.

**Expiry index** - a sorted set of held tickets scored by expiry time:

```
ZADD reservations <expires_at> 4471023
```

**Availability bitmap** - one bit per number indicating whether any copy
remains. 1,000,000 bits is 125 KB, small enough to hold in each service
instance and refresh periodically. Candidate filtering then happens at memory
speed, and only likely candidates reach Redis.

The bitmap is allowed to be stale, because it never allocates anything - it
only decides where to look. Both directions of staleness are safe:

| Bitmap says | Reality | Effect |
|---|---|---|
| No stock | Stock exists | The number is skipped this page. It reappears after the next refresh. |
| Has stock | Sold out | `SPOP` returns nothing and the number is skipped. One wasted lookup. |

Neither can cause a double allocation, because only `SPOP` allocates and it
acts on real state.

**Degraded path.** The failure worth designing for is the refresh job dying.
The bitmap would then freeze and drift, and searches would silently return
fewer and fewer results while inventory was actually healthy - no error, just
worsening output. The bitmap therefore carries the timestamp of its last
refresh, and once that exceeds 30 seconds the filter is bypassed and
candidates go directly to Redis. Searches become slower but stay correct.

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

Reservations are a separate collection rather than fields on the ticket. The
expiry sweep is then a query over roughly 200,000 live reservations instead of
a scan across 10 million tickets.

Ownership is recorded in one place only. A purchase promotes a reservation to
`completed` and creates an order; it does not create a second parallel record
of who owns what.

**Indexes**

- `{ expires_at: 1, status: 1 }` - for the expiry sweeper
- `{ number: 1, status: 1 }` - for rebuilding Redis
- `{ user_id: 1, status: 1 }` - for listing a user's holds

### 4.4 Why not a MongoDB TTL index on reservations

A TTL index looks like a natural fit for expiry, but it deletes the document
rather than notifying the application. Nothing would then return the ticket to
Redis, and the record needed for reconciliation would be gone. Expiry is
handled by an explicit sweeper instead.

---

## 5. Reservation expiry and consistency

### 5.1 The problem

`SPOP` removes a ticket identifier from its availability set. A set member
cannot carry its own time-to-live, so nothing in Redis returns it. If the user
never buys, the ticket is lost unless it is actively restored.

### 5.2 Fast path - expiry sorted set

A worker runs every second:

```
ZRANGEBYSCORE reservations -inf <now>
```

For each expired identifier it performs `SADD` back to the availability set
and removes the entry, both inside one Lua script so the pair is atomic. The
sweep is cheap because it only touches entries that have actually expired.

### 5.3 Slow path - reconciliation against MongoDB

A second job runs every minute over reservations that are past `expires_at`
but still marked active. It marks them expired and restores the tickets.

This exists to catch the case where `SPOP` succeeded but the MongoDB write
failed. The ticket would otherwise be held by nobody and visible to nobody.

### 5.4 Why running both is safe

Restoration is idempotent by construction: a Redis set cannot hold duplicates,
so `SADD` of an identifier already present is a no-op. Both jobs may restore
the same ticket without harm.

### 5.5 Alternative considered - keyspace notifications

Storing each reservation as a key with a TTL and listening for expiry events
reads well, but Redis delivers those events at most once. If the listener is
restarting when an event fires, the ticket is silently lost. That is
acceptable for a cache and not acceptable for inventory.

---

## 6. Performance analysis

### 6.1 Search cost

The worst case is a pattern whose matches are almost entirely sold. Finding 20
available tickets may require examining thousands of candidates.

Four mitigations, using `****23` with 2,000 candidates examined as the
example:

| Approach | Redis round trips | Approximate time |
|---|---|---|
| One lookup per candidate | 2,000 | ~400 ms |
| Batched `MGET` in chunks of 500 | 4 | ~1 ms |
| Bitmap filter first | ~1 | <1 ms |
| Exhausted-pattern cache hit | 1 | ~0.2 ms |

**Batching** is essential. **The bitmap** removes most candidates before any
network call. **A scan cap** of 5,000 candidates bounds worst-case latency -
the search returns what it found plus a cursor rather than running unbounded.
**Caching exhausted patterns** for 30 seconds prevents repeated full walks of
a sold-out pattern.

### 6.2 Memory footprint

Roughly 1,000,000 Redis sets of about 10 members each. Sets this small are
stored compactly, so the cost is driven by whether identifiers are integers or
strings.

| Identifier type | Encoding | Per number | Total |
|---|---|---|---|
| Integer (1-10,000,000) | intset | ~156 bytes | **~156 MB** |
| 24-character hex string | listpack | ~461 bytes | **~461 MB** |

Integer identifiers cost roughly a third as much. This is the reason tickets
use a numeric identifier rather than a 24-character one.

Additional structures: the expiry sorted set at around 20 MB for 200,000 live
reservations, and the bitmap at 125 KB. Both are negligible.

These are estimates. Actual overhead varies with Redis version, allocator and
configuration, and would be verified with `MEMORY USAGE` on a sample and
`INFO memory` under load.

### 6.3 Inventory pressure

With 10,000 users each holding 20 tickets, 200,000 tickets are held at any
moment - 2% of the dataset. This is not noticeable.

At 100,000 concurrent users the figure is 2,000,000, or 20% of inventory.
Popular patterns would begin to appear sold out even though nothing had been
purchased. **This is where reserve-on-search stops working**, and where the
variant in section 8 would be needed.

### 6.4 Request load

At 1,000 searches per second, each reserving up to 20 tickets, the allocation
path handles up to 20,000 atomic state changes per second. This is the figure
that places allocation in Redis rather than in the primary database.

---

## 7. Failure and recovery

### 7.1 Rebuilding Redis

If Redis is lost, allocation cannot proceed safely until it is rebuilt. This
is an availability concern, not a cache-warming concern.

The naive rebuild - scan all available tickets in MongoDB and populate the
sets - has a correctness flaw. If the scan runs from 10:00 to 10:03, a ticket
sold at 10:01 that the cursor has already passed is written back as available,
and would then be sold twice.

**Chosen approach:** open a MongoDB change stream before the scan begins,
buffer the events, run the scan, then apply the buffered events in order.
Nothing is missed and no downtime is required.

Alternatives considered: a read-only window during rebuild (correct but costs
downtime at the worst possible time), and rebuild-then-reconcile using a
timestamp (workable, but still needs a brief pause).

**Conflict bias:** wherever the two stores disagree, resolve towards
*unavailable*. A ticket wrongly marked sold is a lost sale. A ticket wrongly
marked available is sold twice and becomes a payment dispute. The asymmetry is
deliberate.

**Operational points**

- Searches continue during a rebuild; reservations are rejected
- Rebuild into a separate key prefix and swap atomically, so a partial state
  is never live
- Set `maxmemory-policy noeviction` on this instance - silently evicting
  availability sets would be a correctness bug, not a performance one
- Enable AOF persistence so most restarts do not require a rebuild
- The reconciler from 5.3 continues afterwards and catches residual drift

---

## 8. Production consideration: browse versus allocate

The design above reserves tickets at the moment they are returned by a search,
which is what the requirement describes.

At scale this has a cost worth naming. If 10,000 users are browsing and every
search reserves 20 tickets, 200,000 tickets are held at any moment, and most
of those users will never buy. Beyond roughly that point, the inventory begins
to look sold out while nothing has actually been sold, as shown in 6.3.

A production system would usually separate the two actions:

- **Search** returns matching tickets as a read-only view. Nothing is held.
- **Reserve** happens when the user selects a specific ticket.

The exclusivity guarantee is unchanged - no two users can ever hold or buy the
same ticket - but locking is applied to real intent rather than to browsing.
Contention drops by roughly the ratio of browsers to buyers.

Both are described here because the choice depends on whether the constraint
is meant to apply to the search response or to the allocation itself. The
requirement's wording - that a search should not *return* the same ticket to
multiple users - points to the first, so that is the primary design.

---

## 9. API contract

Under reserve-on-search, **a search changes state**. It is therefore a `POST`,
it is not cacheable, and it is not safe to retry blindly. This is a direct
consequence of the requirement.

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

No total count is returned. Over live inventory an exact total cannot be given
honestly, and `has_more` is the accurate substitute.

**Pattern validation:** exactly 6 characters, each a digit or `*`. Anything
else returns `400`.

The all-wildcard pattern `******` is permitted. It is valid by the
specification's own definition, and with candidate generation it is not
expensive - available tickets are found almost immediately. The risk it
appears to pose is really a request-volume risk, and a script issuing
`1*****`, `2*****`, `3*****` would do the same. Rate limiting is the correct
control and covers all cases.

### 9.3 Pagination

Offset pagination is not usable here. If page 2 is "skip 20, take 20" and one
ticket from page 1 is sold in between, every later item shifts and some are
never shown.

The cursor is a base64 value carrying:

- the **pattern**, so a later page cannot silently change it
- the **last candidate reached**, so paging resumes by value rather than by
  count
- the **ordering seed**, so the per-session order stays consistent

It is opaque to clients and may be signed to prevent tampering.

### 9.4 Purchase

Purchases are **all-or-nothing**. If any ticket in the request is not validly
held by the caller, nothing is bought and the call returns `409`.

The reason is that purchase involves money. A failed call that changed nothing
is trivially safe to retry; a partially successful one is not, and partial
success combined with payment and retries is where double-charge defects
appear. The user cost is small, because a purchase happens inside a live
reservation window where the tickets are already held.

**Idempotency:** the client supplies an `Idempotency-Key`. The result is
stored against that key, and a replayed request returns the original response
rather than buying twice.

### 9.5 Expired holds

A user seeing a ticket, then finding it gone when they act, is routine rather
than exceptional:

```json
{
  "error": "reservation_expired",
  "message": "Your hold on these tickets has expired. Please search again."
}
```

Returned as `409 Conflict`.

### 9.6 Rate limiting

Each search holds 20 tickets, so an unlimited search rate is a way to freeze
inventory without buying anything. Here rate limiting is a correctness control
rather than only a protective measure.

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
