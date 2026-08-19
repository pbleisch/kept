---
id: 0015-landing-is-a-merge-queue
type: decision
title: 0015 — Landing is a merge queue over `origin/main`
---

# 0015 — Landing is a merge queue over `origin/main`

**Status** accepted · closes the landing question left open in
[0011](0011-tracked-formats-must-merge.md) and open question 9 · depends on
[0014](0014-an-observation-names-its-world.md) · **amended by
[0037](0037-a-conflict-is-discovered-by-exhausted-repair.md)**, which widens what
the gate covers from one promise to every standing artifact the diff can affect

## Context

Landing has a destination, and it was never written down: **a repaired change
belongs on `main`, and `main` belongs on `origin`.** Anything short of that is a
change sitting in a place the operator has to go and collect, which is the
failure mode the design exists to avoid.

The first sketch — 0011's consequences table, and open question 9 — said
several concurrent lands "need an order", implying git needs help to push
safely. It does not. Ref updates are atomic and locked: two pushes to one ref,
one wins and the other is rejected non-fast-forward; two pushes to different
refs both succeed. Nothing is lost or corrupted, ever. That framing was wrong
and this decision replaces it.

What is missing is not atomicity but **isolation**. Landing is a
read-modify-write over the world:

```
gather again, on the origin     ← read
land                            ← write
```

Git makes the write atomic. Nothing makes the pair serializable, and two
distinct failures follow.

**The clean merge is the dangerous one.** Two threads touch disjoint files. Git
merges them without complaint, both push, nothing is rejected and nothing
retries. Thread A gated against `main@X`, thread B gated against `main@X`, and
the tree that now exists at Z was gated by nobody. The combination can break a
promise neither gate evaluated, and because the merge was clean, nothing tells
either thread to re-check. The invariant that the gate re-derives from the
world holds — against a world that stopped existing between the gather and the
push.

**The rejected merge is a cost problem, not a safety one.** On rejection the
retry is reset, **re-gather**, push — and re-gathering is the expensive part,
often a test run. Under contention the loser can re-gather and lose repeatedly.
Git guarantees that someone progresses, not that any particular thread does.

## Decision

**Landing is a merge queue: the gate runs optimistically outside a lock, and a
cheap check inside the lock decides whether it still applies.**

```
outside the lock   gate the box against origin/main@X          minutes
inside the lock    fetch; is origin/main still X?              seconds
     still X       merge, push origin main, release            seconds
     moved to Y    release; re-gate against Y, or skip; retry
```

The lock is `flock` on a path under the shared git common directory, which
every worktree of a repository can see and none of them track. It is held for
seconds, not for the duration of a gate.

**The in-lock check is against `origin/main`, not local `main`.** A local lock
excludes local runs and nothing else — the operator pushing from a laptop, CI,
a second machine. Fetching inside the lock is what makes the invariant true
rather than merely locally consistent.

**The invariant this buys:** every commit on `origin/main` was gated against
the tree it landed on.

**Read that precisely, because as written here it claimed more than it
delivered.** The gate above re-derives *the promise the thread was about*, so
the invariant was really *gated against the tree it landed on, for one promise*.
A repair could satisfy A, break B, and land clean — the merge is clean, the gate
passes, and nothing rejects it.
[0037](0037-a-conflict-is-discovered-by-exhausted-repair.md) closes that: the
gate re-derives every standing artifact whose scope the diff's paths touch, and a
collateral break refuses the land. The overlap it needs is the one already
computed below for the disjoint-scope skip, read in the other direction.

### Skipping the re-gate, soundly

When `origin/main` moved to Y, the naive answer is to re-gate, and it is often
unnecessary. The ledger records what paths the intervening land touched, and
the promise declares its `RepairScope`. **If the two are disjoint, the
intervening change cannot have affected this gate, and the re-gate is skipped
on a stated rule rather than on optimism.**

This is the useful half of backlog's independence scoring, and it is cheap here
only because both inputs already exist. Where they overlap, or where the
intervening land's paths are unknown, re-gate. The skip is recorded with the
land so the reasoning is inspectable, because a skipped gate that turns out to
be wrong must be traceable to the rule that skipped it and not to a mood.

The same two inputs answer a second question, which is what makes 0037
affordable: *which standing artifacts care about these paths*. One direction
decides whether an intervening land can have invalidated this gate; the other
decides which promises this diff must be gated against. Neither needs anything
the ledger and the promise do not already carry.

### Three deployment constraints

These make the protocol clean rather than merely possible, and they are part of
the decision:

1. **`main` is checked out in no worktree.** It is then an ordinary ref any
   run can update under the lock. Checked out somewhere, git refuses
   updates to it and the arrangement needs `receive.denyCurrentBranch` games
   for no gain.
2. **Boxes are detached at `origin/main`, not on branches.** Git forbids one
   branch in two worktrees but permits any number of detached worktrees at one
   commit — so every box cut for a gather observes **the same world**, and
   under 0014 those observations are identical content and dedup to one file.
   The one-branch-one-worktree rule stops being a constraint, and concurrent
   gathers stop paying for each other's build output.
3. **Boxes are cut from `origin/main`.** A thread's diff then applies to the
   landing target by construction — a box cut for a gather and a box cut for a
   repair differ in what happens inside them, not in where they come from.

## Alternatives

**Push, detect rejection, reset, retry — 0011's answer.** Rejected as
insufficient rather than wrong: it is exactly right for the rejected case and
does nothing at all for the clean-merge case, which is the one that silently
ships an ungated tree.

**Hold the lock across the gate.** Correct and simple, and it caps land
throughput at one per test run. Rejected on cost; the merge-queue split gets
the same invariant with a seconds-long critical section.

**A daemon owning a land queue.** Rejected for now. `flock` releases on process
death because the lock lives on an fd, so a crashed lander needs no TTL, no
reclaim, and no takeover — a daemon would have to reimplement that, and would
add a component with its own liveness and its own state that can disagree with
the ledger. The case that would justify one is speculative batching (test A+B
together, split on failure), which is a throughput optimization to build after
measuring contention, not before.

**Land onto a branch per thread and reconcile later.** Rejected: it removes
contention by removing convergence, and leaves the operator merging N branches
by hand. The destination is the point.

## Consequences

- **`flock` has no TTL, so a wedged holder blocks landing indefinitely.** The
  gate inside the critical section is bounded by a timeout; this is the honest
  cost of preferring `flock` to a lease.
- **Land throughput is one merge per few seconds**, not one per gate. Whether
  that is enough is a measurement, and the answer if it is not is speculative
  batching.
- **A land records what it was gated against** — the world from 0014 — and
  whether its re-gate was skipped and by which disjointness. Without that, an
  ungated combination is unattributable after the fact.
- **This is single-machine coordination**, per
  [0013](0013-the-gate-makes-concurrency-safe.md). Machines that share no
  `flock` share no critical section, and this is the one place where that
  costs correctness rather than money — everything else 0013 leaves to the
  gate. The section is small and well defined, which is what makes the port
  tractable when it is wanted.
- **Two threads repairing different promises that touch the same files** still
  collide, and now collide in a place that reports: the second one's re-gate
  sees an overlapping scope and re-runs. The lease-granularity question in open
  question 9 loses most of its force, because the correctness decision has
  moved to land time where the facts are known — which is the same move
  [0013](0013-the-gate-makes-concurrency-safe.md) makes for repair.
