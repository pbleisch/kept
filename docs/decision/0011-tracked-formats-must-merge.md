---
id: 0011-tracked-formats-must-merge
type: decision
title: 0011 — Everything tracked must merge, because instances are concurrent
---

# 0011 — Everything tracked must merge, because instances are concurrent

## Context

Earlier decisions assumed a single operator and treated concurrency as a
problem for later. That conflates two questions. There is one **principal** —
one person who ratifies and lands — but there will be many concurrent
**writers**: several instances of `kept` ticking, repairing, and recording at
once. A queue that drains without you is the point of the design, and it means
simultaneous writes from the first day it is useful.

adsr's leader election and field-level CAS exist because of concurrent writers,
not because of multiple people. So the constraint arrives much sooner than
"a second person appears".

## Decision

**Every format tracked in git must merge without a human resolving it, except
where the conflict is genuinely semantic.**

The way to get that is to ensure no two writers ever write the same bytes:

1. **One file per artifact.** Two agents editing different artifacts touch
   different files. Two editing the same artifact is a real conflict and should
   surface as one.
2. **One file per artifact's truth.** `truth/promise/{slug}.jsonl`, not a
   global stream.
3. **Streams are append-only JSONL under `merge=union`**, and every derived
   read folds by **event time, then id** — never by file order. Union merge
   interleaves arbitrarily; a fold that depends on order is silently wrong.
4. **Every path is keyed by something already unique** — the artifact, the
   thread, the observation's own content hash, the run — so two concurrent
   processes never choose the same file. No path is partitioned by which
   process wrote it; attribution is a `writer` field inside the record, holding
   a minted run id. See [0012](0012-a-run-id-is-opaque.md).
5. **No shared mutable file.** Nothing tracked is read-modify-written in place.
   If something needs that shape, it is derived and therefore not tracked.
6. **Where a list in frontmatter would conflict spuriously**, prefer a
   `_links/` sidecar. Two agents adding different links to one artifact should
   not collide.

## Consequences

**Compare-and-swap and leader election are not needed, and that is a
consequence rather than an assumption.** They exist to arbitrate two writers
over the same bytes; the rules above mean that case does not arise. If a future
format cannot obey them, it needs CAS and it needs an ADR saying why.

Merge is not the whole of coordination. Three things still need it, and all
three have known answers in the predecessors:

| Concern | Answer |
|---|---|
| Two instances opening a thread for the same drift | a lease per artifact with a TTL and expired-lease takeover (pt) |
| Two threads landing into the same origin at once | land, detect push rejection, reset and retry (backlog) |
| Two instances using the derived index | SQLite in WAL mode, and it is rebuildable if it tears (cpmux) |

**This reinforces 0001 from a second direction.** Standing and truth are kept
out of the artifact because a writer must not be able to mint its own
authority. They would also have to be kept out to make artifacts mergeable: an
artifact that recorded its own verdict would be rewritten on every tick by
whichever instance got there first.
