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

### 1.1 Moving parts

The system is a handful of pieces. I list them here so the sections below have somewhere to
hang.

| Piece | Count | Job |
|---|---|---|
| API service | N instances behind a load balancer | Runs searches, holds tickets, sells them |
| Redis | One primary, one replica | Availability sets, expiry index, bitmap (4.2) |
| MongoDB | One replica set | Tickets, reservations, orders (4.3) |
| Expiry worker | One, every second | Returns expired holds to the pool (5.3) |
| Reservation reconciler | One, every minute | Fixes holds Redis released but MongoDB still calls active (5.4) |
| Orphan reconciler | One, continuous slow pass | Finds tickets that belong to nobody (5.4) |
| Bitmap refresh | Inside each API instance, every 5 seconds | Pulls the 125 KB availability bitmap (4.2) |

The API service keeps no per-user state. A search's position lives in the cursor it hands
back, and the bitmap is a cache any instance can refill with one `GET`. So any instance can
serve any request, and I scale by adding instances. The state that matters is in Redis and
MongoDB.

The three workers run as their own processes, not inside the API. I don't want a sweep
falling behind because request traffic is busy, and I want to restart them without
restarting the API. One instance of each is enough. Running two during a deploy is also
safe, because every restore they do is idempotent (5.5).

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

Releasing the previous page on a new search caps each user at one active page. This
applies to paging as well, which is what closes the loophole. See 9.3.

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

### 3.4 Candidate order

Generating candidates in plain ascending order would send every user searching `****23` at
`000023` first. They would all contend on the same few sets, and the low numbers would look
permanently sold out while the high ones never got offered. So each search walks the
candidate space in its own order.

The order has to satisfy two things at once. It has to look shuffled, and it has to be
resumable from a cursor without storing anything server-side. A stored shuffled list per
session would need state I don't want to keep, and a random jump per page can't tell me
what it has already shown.

I get both from an affine permutation. With `k` wildcards there are `N = 10^k` candidates,
indexed `0` to `N-1`. Index `i` maps to:

```
p(i) = (a * i + b) mod N
```

`b` is a per-search seed. `a` is derived from the same seed and forced coprime with `N`,
which for `N = 10^k` just means odd and not a multiple of 5. Coprime `a` makes `p` a
bijection, so walking `i` from 0 upwards visits every candidate exactly once, in an order
that differs per search. The digits of `p(i)` go into the wildcard positions and the fixed
digits stay put.

Nothing is precomputed or stored. The cursor carries `i`, and the next page resumes at
`i + 1`. That is the piece a value-based cursor cannot do: under a permutation the last
number shown says nothing about position, so paging has to resume by index.

---

## 4. Storage design

### 4.1 Split of responsibility

| Store | Holds | Role |
|---|---|---|
| Redis | Available ticket identifiers, grouped by number | Decides who gets which ticket |
| MongoDB | Tickets, reservations, orders | Durable record of what happened |

MongoDB is the source of truth. Redis is the allocator. When the two disagree, I correct
Redis towards MongoDB, never the other way.

**Why Redis for allocation.** One command has to remove a ticket from the pool and name its
new owner, with no lock around it. Redis executes commands one at a time, so `SPOP` is that
command. A set also gives me random selection for free, which spreads users apart instead
of piling them onto the first available ticket. The whole structure is 156 MB (see 6.2), so
it fits in memory on one node and needs no sharding.

**Why MongoDB for the record.** Part 1 already runs MongoDB, so this adds no new database
to operate. Change streams give me the rebuild path in 7.1 without a downtime window, and a
replica set gives durability. The access patterns here are point reads and short range
queries on indexed fields, which is not a demanding workload for it.

**Why not Postgres alone.** This is the real alternative and it deserves a straight answer.
`SELECT ... FOR UPDATE SKIP LOCKED` allocates correctly, in one store, with no Redis to
rebuild and no two-store drift to reconcile. Sections 5 and 7 would mostly disappear. I
would pick it for a smaller system.

I don't pick it here because of what reserve-on-search does to the write path. Every search
allocates, so peak load is 20,000 row locks per second plus the reservation writes, and
they land on the same primary that serves reads. That is a lot of pressure on the one node
I can't scale horizontally. The split puts the hot, disposable work in Redis and leaves
MongoDB with 1,000 writes per second (see 6.4). The cost is the reconciliation machinery in
section 5, and I would rather run that than run the primary at its limit.

If the system moved to the browse-versus-allocate model in section 8, allocation volume
would drop by the browser-to-buyer ratio and Postgres alone would become the better call.

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

Every `SPOP` on the allocation path is paired with this `ZADD` inside one Lua script. That
pairing is what makes the fast path in 5.3 complete: a ticket can never leave an
availability set without something scheduled to bring it back. If the two were separate
commands, a crash between them would lose the ticket with no record anywhere.

**Availability bitmap** - one bit per number, saying whether any copy is left. 1,000,000
bits is 125 KB, small enough to keep in every service instance and refresh periodically.
Candidate filtering then runs at memory speed and only likely candidates reach Redis.

The bit is maintained where the state changes, not by polling. The allocation script clears
it with `SETBIT` when an `SPOP` empties a set, and the restore script sets it when a `SADD`
puts the first member back. So Redis holds one authoritative 125 KB bitmap key, updated as
a side effect of work already happening.

Each service instance pulls that whole key every 5 seconds with a single `GET`, along with
a `refreshed_at` key written by the same scripts. 125 KB every 5 seconds per instance is
nothing. I rejected computing the bitmap by scanning: 1,000,000 `SCARD` calls per refresh
would cost more than the filter saves.

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
| `status` | `available` / `sold` |
| `price` | |

There is no `reserved` status on the ticket. A hold is not a property of the ticket, it is
a row in `reservations` with a lifetime. Writing `reserved` here too would mean 20,000
ticket updates per second at peak, for a fact that already lives somewhere else and expires
on its own. "Is this ticket held right now" is answered by the reservations collection.
`tickets.status` moves once, from `available` to `sold`, and that write happens at purchase
time when volume is low.

**reservations** - one document per search page, not per ticket:

| Field | Notes |
|---|---|
| `_id` | The `search_id` |
| `user_id` | |
| `items` | Array of `{ ticket_id, number, price, status }`, up to 20 |
| `expires_at` | |
| `status` | `active` / `expired` / `completed` |

**orders** - a completed purchase, referencing the reservation and the ticket ids it
fulfilled.

One document per page instead of per ticket cuts peak inserts from 20,000 per second to
1,000, and the whole page shares one `expires_at` anyway because it was held by one search.
Per-item `status` is there because a purchase covers the tickets the user asked for, which
can be a subset of the page. The reservation is `completed` once no item is still `active`.

Reservations are their own collection instead of fields on the ticket. The expiry sweep is
then a query over roughly 10,000 live reservation documents instead of a scan across 10
million tickets.

Ownership lives in one place only. A purchase promotes items to `completed` and creates an
order. It doesn't create a second parallel record of who owns what.

**Indexes**

- `{ expires_at: 1, status: 1 }` on `reservations` - for the expiry sweeper
- `{ user_id: 1, status: 1 }` on `reservations` - for listing a user's holds, and for
  releasing the previous page on a new search
- `{ items.ticket_id: 1, status: 1 }` on `reservations` - for the orphan check in 5.4
- `{ status: 1, number: 1 }` on `tickets` - for rebuilding Redis

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

### 5.2 Every state change, and what it does in Redis

A ticket leaves the availability set on reservation and comes back on expiry. Two other
transitions also have to touch Redis, and missing either one is a correctness bug, so I
list all of them together:

| Transition | Redis | MongoDB |
|---|---|---|
| Reserve | `SPOP avail:<number>` + `ZADD reservations <expires_at> <ticket_id>` | Insert the reservation document |
| Extend at checkout | `ZADD reservations GT <new_expires_at> <ticket_id>` | Update `expires_at` |
| Release (new search, or explicit `DELETE`) | `SADD avail:<number> <ticket_id>` + `ZREM reservations <ticket_id>` | Mark the old reservation `expired` |
| Expire | `SADD` + `ZREM`, by the worker in 5.3 | Mark `expired` |
| **Purchase** | **`ZREM reservations <ticket_id>`, and no `SADD`** | Items to `completed`, ticket to `sold`, create the order |

Each row is one Lua script, so the pair inside it is atomic.

**The purchase row is the one that is easy to miss.** `SPOP` already took the ticket out of
the availability set, so buying it looks like it needs no Redis work at all. But the
expiry entry from the reservation is still sitting in the sorted set. Leave it there and
five minutes later the sweeper does exactly what it is built to do: `SADD` the ticket back
into `avail:`. A sold ticket returns to the pool and gets sold a second time. Purchase must
`ZREM`.

`GT` on the checkout extension matters for the same reason. A plain `ZADD` would also
accept a lower score, so a retried or reordered request could shorten a hold instead of
extending it.

**Ordering between the two stores on purchase.** I do the `ZREM` first, then commit in
MongoDB. If the process dies in between, the ticket is in no availability set and has no
completed order, so it is held by nobody. That is a lost sale, and 5.4 finds it. The other
order fails worse: commit first, crash before `ZREM`, and the sweeper resurrects a ticket
that is already sold and paid for. This is the conflict bias from 7.1 applied to a single
request. When I have to choose, I lose the sale rather than sell twice.

### 5.3 Fast path - expiry sorted set

A worker runs every second:

```
ZRANGEBYSCORE reservations -inf <now>
```

For each expired identifier it does `SADD` back to the availability set and removes the
entry, both in one Lua script so the pair is atomic. The sweep is cheap because it only
touches entries that actually expired.

### 5.4 Slow path - reconciliation against MongoDB

Two jobs, because there are two different ways for state to drift and one query can't see
both.

**Expired reservations, every minute.** A query over reservations past `expires_at` and
still `active`. It marks them expired and restores their tickets. This one catches the case
where the ticket came back in Redis but the MongoDB update didn't land, so a user's hold
list still shows tickets they no longer have.

**Orphaned tickets, as a slow background pass.** A ticket is orphaned when MongoDB says it
is `available`, no reservation holds it, and it is not a member of its availability set. It
belongs to nobody and nobody can find it. That happens when a `ZREM`-then-commit purchase
dies halfway (5.2), when Redis restarts from an AOF that lost the last second of writes, or
when a reservation write to MongoDB failed after `SPOP` and the process died before the
`ZADD` was durable.

This one cannot be a query over MongoDB alone. If the reservation write failed there is no
reservation document to find, so a job looking for stale reservations will never see the
ticket. It has to compare the two stores: walk `tickets` where `status: available` in
number order, and for each number compare the ids against `SMEMBERS avail:<number>` and
against active reservations. Anything present in none of them is restored with `SADD`.

That is a full 10 million ticket comparison, so it does not run every minute. I run it in
chunks over a number range, continuously and slowly, so the whole space is covered every
hour or so, and immediately after any Redis failover or rebuild. It exists to stop slow
inventory leakage, not to serve a live request.

### 5.5 Why running both is safe

Restoration is idempotent by construction. A Redis set can't hold duplicates, so `SADD` of
an identifier that is already there is a no-op. Both jobs can restore the same ticket with
no harm.

### 5.6 Alternative considered - keyspace notifications

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
| Pipelined `SCARD` in chunks of 500 | 4 | ~1 ms |
| Bitmap filter first | ~1 | <1 ms |
| Exhausted-pattern cache hit | 1 | ~0.2 ms |

Availability is held in sets, so the batch check is a pipeline of `SCARD` calls, or one Lua
script taking the chunk of keys as arguments. It is not `MGET`, which reads string keys and
would return nothing here.

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

The rest: the expiry sorted set at around 20 MB for 200,000 held tickets, and the
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

MongoDB sees a different figure, and that is the point of the split:

| Path | Peak rate | Why |
|---|---|---|
| Redis allocation | 20,000 ops/s | One `SPOP` plus `ZADD` per ticket |
| Reservation inserts | 1,000 docs/s | One document per page, not per ticket (4.3) |
| Release of the previous page | 1,000 updates/s | One per search, on the same document shape |
| Ticket status writes | Purchase rate only | `tickets.status` never records a hold |

So MongoDB takes roughly 2,000 writes per second at peak against small indexed documents,
not the 40,000 it would take if every ticket had its own reservation document and its own
status update. A single replica set handles that.

The 20,000 Redis ops per second are also not 20,000 round trips. A page is one pipeline, so
it is roughly 1,000 pipelined batches per second, which is well inside what one Redis node
does.

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
- The reconcilers from 5.4 keep running afterwards and catch leftover drift

The change stream needs a replica set, and the oplog has to still cover the moment the
stream opened. A 10 million ticket scan can outrun a small oplog window, and then the
rebuild is silently incomplete. So the oplog is sized for several times the expected scan
duration, and the rebuild aborts rather than finishes if the stream reports it fell behind.

### 7.2 Redis failover

Redis holds the exclusivity guarantee, so its failure modes matter more than its capacity.
156 MB fits one node, and allocation is always single-key, so I run one primary with a
replica for failover and no cluster. Sharding would add operational cost and buy nothing
here.

The failure to be honest about is that replication is asynchronous. Redis acknowledges an
`SPOP` before the replica has it. Promote that replica and the ticket is back in the
availability set while a user is holding it, and it gets allocated twice. `WAIT` on every
allocation would close the window and would also put a network round trip on the hot path,
which is the thing this design spends its complexity to avoid.

So I don't treat a promoted replica as trustworthy for allocation. Failover is a rebuild
trigger:

- `appendfsync everysec` on the primary, so an unclean restart loses at most a second
- On promotion, reject reservations and run the change-stream rebuild from 7.1 against the
  new primary. Searches keep serving from the bitmap, they just stop holding tickets
- Run the orphan pass from 5.4 immediately afterwards, over the whole space rather than in
  slow chunks

A rebuild is a few minutes of read-only browsing. Double-allocating paid tickets is a
refund and a support case. The choice is the same one as everywhere else in this design.

### 7.3 What I would watch

Most of the failures above are quiet. They don't throw errors, they just make the system
slowly wrong, so the alerts matter as much as the mechanisms:

| Signal | What it catches |
|---|---|
| Bitmap age above 30 seconds | The refresh job died and searches went to the degraded path |
| Expiry sweep backlog, the size of the sorted set below `now` | The sweeper is behind and inventory is not coming back |
| Tickets restored per orphan pass | Should be near zero. A rising number means writes are being lost somewhere |
| Candidates examined per search | Rising means real inventory pressure, and it moves before users complain |
| Searches returning fewer than 20 | The user-visible version of the same thing |
| Redis memory against `maxmemory` | With `noeviction`, hitting it fails allocation rather than degrading it |

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

Every endpoint sits behind the JWT middleware from Part 1. The caller's `user_id` comes from
the token claim, never from the request body. That is what makes the one-page cap real.
Rules 2.5 and 2.6 are per user, so a caller who could name any `user_id` could release
someone else's holds, or hold twenty pages under twenty invented identities. An
unauthenticated request gets `401`.

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
- the **ordering seed**, which fixes the permutation in 3.4
- the **next candidate index**, so paging resumes by position in that permutation

It is opaque to clients and can be signed to stop tampering.

**Paging releases the previous page too.** A cursor request is still a `POST` to
`/lottery/searches`, so rule 2.5 applies to it unchanged: page 2 frees the 20 tickets from
page 1 and holds 20 new ones. Without that, paging would be the way around the one-page cap
that 2.5 exists to enforce. A client that pages 500 times would be holding 10,000 tickets,
which is exactly the behaviour the rule forbids when the same client re-searches instead.

The cost is that paging is forward-only. Going back to a page means searching again and
getting different tickets. I take that trade because the alternative is an inventory hole,
and because the tickets on a page a user already scrolled past are ones they chose not to
buy.

Retrying a search after a timeout is safe for the same reason. The retry releases whatever
the first attempt held, so a lost response costs a page of tickets for one round trip, not
a permanent leak. That is why search needs no idempotency key while purchase does.

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
| Postgres alone with `SKIP LOCKED` | Simpler and correct, but reserve-on-search puts 20,000 allocations per second on the primary (4.1) |
| A `reserved` status on the ticket | Duplicates a fact that already expires on its own, at 20,000 extra writes per second |
| One reservation document per ticket | Twenty times the write volume for one `expires_at` the whole page shares |
| A value-based cursor | Cannot resume a shuffled candidate order; the permutation index can |
| `WAIT` on every allocation | Puts a replication round trip on the hot path to close a window a rebuild already covers |
| Redis Cluster | 156 MB needs no sharding, and allocation is single-key anyway |
