---
id: 0044-v1-has-no-index
type: decision
title: 0044 — v1 has no index, and every verb walks the corpus
links:
  - { target: decision:0001-standing-and-truth-live-in-the-ledger, relation: extends }
  - { target: decision:0032-standing-is-a-fold-over-acts, relation: extends }
---

# 0044 — v1 has no index, and every verb walks the corpus

## Context

awiki's derived SQLite and FTS5 index is in [`lineage.md`](../lineage.md) as
one of the things taken from it, and [`layout.md`](../layout.md) has `index.db`
in the derived tier with three jobs: search, the link graph, and the drift
query. `internal/index` has existed since the first commit as a doc comment
saying so.

Open question 7 asked whether v1 needs it and said to decide **by measuring a
real corpus rather than in advance**. There is now a real corpus — forty-four
artifacts, forty-four acts — which is enough to measure and not enough to
settle anything, so it was measured there and at a synthetic corpus of two
thousand.

## Decision

**There is no index in v1. Every verb walks the corpus and folds the ledger.**

| corpus | `list` | `status` | `drift` | `doctor` |
|---|---|---|---|---|
| 44 artifacts, 44 acts — 564 KB of docs, 284 KB of ledger | 7.4 ms | 7.6 ms | 10.9 ms | 7.9 ms |
| 2000 artifacts, 2000 acts — 9.5 MB of docs, 8.3 MB of ledger | 77 ms | 91 ms | 108 ms | 89 ms |
| 2000 artifacts, 6000 acts — the same corpus, released and re-ratified | | | 110 ms | |

Medians of seven, warm page cache, one machine, `/usr/bin/true` at 1.5 ms on
the same measurement for the process-start floor. Forty-five times the
artifacts costs about ten times the wall clock, because at forty-four
process start is most of it. The marginal cost is **50 µs per artifact** for a
full walk *and* a full fold, and `drift` — the most expensive verb, and the one
open question 7 names — reaches one second at roughly **twenty thousand
artifacts**.

Tripling the ledger against the same corpus costs 2 ms. The fold is not what
the time goes on; reading and hashing the artifacts is, and an index does not
avoid that for `drift`, which has to hash content to answer at all.

The index's three jobs do not survive those numbers. The drift query is fast
without it. The link graph is already resolved at load and is what `kept list`
reports. And **search has no verb**: there is no `kept search`, so full-text is
a capability with no consumer.

What is left is a second thing that can be wrong. `internal/index/doc.go` says
it is authoritative for nothing and that if it disagrees with the corpus and
the ledger it is wrong — which is the honest description of a cache.
[0001](0001-standing-and-truth-live-in-the-ledger.md) and
[0032](0032-standing-is-a-fold-over-acts.md) both refused a cache for standing
on exactly those grounds, and refusing one for reads is the same argument at a
lower stake.

## Alternatives

**Build it now, before the corpus is large enough to need it.** Rejected on
the measurement: it would be built against a workload it beats by a factor
nobody would notice, and every read path would grow a second answer that has to
be kept in agreement with the first. The stale-index bug is not hypothetical —
it is the one grft's containment checks hit from the other direction, where the
derived view saw nothing and said so confidently.

**Keep `internal/index` as a stub and decide later.** Rejected because a stub
is a claim. `layout.md` describing an index and a package existing for it is
how the next person concludes it was decided and is merely unbuilt, which is
the opposite of what was decided.

**Index only the link graph, and skip search.** Rejected: the link graph is
computed at load and is 7 ms of the 108. The saving is inside the noise and the
agreement problem is unchanged.

## Consequences

`internal/index` is deleted. `layout.md`'s derived tier loses `index.db` and
keeps caches and boxes, and `lineage.md` moves awiki's index from what was
taken to what was deliberately not adopted — beside grft's incremental
invalidation, which was refused on the same shape of argument: measured to
work, and it pays above a register size one operator will not reach.

This is re-opened on either of two triggers, written down so the next answer is
not a guess: **a verb that needs full-text ranking** rather than substring
matching, or **a corpus where `kept drift` exceeds one second**. Neither is
speculative; both are measurable the day they arrive.

The measurement is warm-cache, single-machine and synthetic above forty-four
artifacts. A cold cache over a network filesystem could be an order of
magnitude worse, which would not change the conclusion — still under a second —
but would narrow the margin claimed here. It is recorded in this decision
rather than as a `fact` artifact because a fact has no standing, and this is a
decision that turns on it.
