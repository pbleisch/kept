---
id: 0013-the-gate-makes-concurrency-safe
type: decision
title: 0013 — Concurrent runs are made safe by the gate, not by exclusion
---

# 0013 — Concurrent runs are made safe by the gate, not by exclusion

**Status** accepted · extends [0011](0011-tracked-formats-must-merge.md) and
[0012](0012-a-run-id-is-opaque.md) · answers open question 9 and corrects the
settled half of open question 4 · replaces an earlier draft of this number that
proposed a durable *instance* tier and an ownership partition

## Context

0011 settled storage: everything tracked merges, because no two writers write
the same bytes. It left three things to "coordination" and named answers for
none of them — what stops two runs repairing the same drift, what makes a
budget able to refuse, and what orders two lands.

The first draft of this decision answered with machinery: a durable **instance**
identifying the workspace a run happens in, and an **ownership partition**
assigning each artifact to one instance. Working the co-located shape through
showed both were over-built, for two reasons that are worth stating because
they are what the decision now rests on.

**A workspace is not durable.** A worktree is a temporary place that a piece of
work acquires and gives back — a few runs, then pruned. The durable thing is
the **thread**, which 0012 already gives an identity independent of any
process. So the place needs no identity beyond the work holding it, and there
is no long-lived thing left for an instance to name.

**Duplicate repair is wasteful, not wrong.** Two runs repair the same promise
from `main@X`. A lands. B's in-lock check under
[0015](0015-landing-is-a-merge-queue.md) sees `main` has moved, re-gates
against the new tree, and finds the promise already kept. The model already
stops it there: the assessor asks whether the predicate was satisfied before
the repair — a no-op claiming credit — and the gate re-derives from the world
rather than from what the agent said. **Both invariants were written for a
different reason and they happen to make concurrent repair safe.**

What is left for exclusion to provide, then, is not correctness. It is money:
an agent dispatch is the most expensive thing the system does, and doing it
twice for one drift is the most expensive avoidable waste.

One thing genuinely is a correctness rule, and it is not about concurrency at
all — see *No act on a stale view* below. It applies to a single run on a
single machine that has not fetched.

## Decision

### There is no instance

Four nouns, and they are enough:

| | is | durable |
|---|---|---|
| **run** | one execution of kept, start to exit | no — prunable diagnostics (0012) |
| **thread** | one repair, resumable, holds a box | yes |
| **box** | a tree acquired to work in | no |
| **world** | the tree a measurement was made against ([0014](0014-an-observation-names-its-world.md)) | it is a value, not a thing |

Each argument for a fifth resolves into one of these. A box outliving its run
is named by its **thread**. A stale view is the age of a **world**, not a
property of a place. And *which machine wrote this record* stops mattering once
observations are keyed by world: the same tree measured by the same gatherer is
the same observation, whoever ran it, which is what content addressing already
asserts. Where two machines really do disagree, the cause is the environment —
toolchain versions, core count — which belongs in the observation's data where a
predicate can read it, not in a node id on every record.

### A box is acquired and released

Boxes are a pool, not a per-workspace fixture. A thread acquires one for a
repair; **a tick acquires one for a gather**, because a gatherer that never
repairs the corpus still runs builds and tests that write, and two gathers in
one tree collide on their output. Released on completion, discarded on failure,
capped so that a pile of abandoned worktrees is not a disk problem.

`box` is already the word and `internal/box` already the package; generalizing
it is cheaper than coining a noun for the same thing acquired by a tick.

### Exclusion is a cost optimization

Before opening a thread, a run takes an advisory lock on the artifact —
`flock` on a path under the shared git common directory, which every worktree
of a repository can see and none of them track. Failing to take it means
another run is already on this drift, so skip it and move on.

The lock is not load-bearing. Losing it, ignoring it, or running without one
costs a duplicated repair and never a wrong outcome, which is what makes it
safe to have no answer at all for deployments where no shared lock exists.
Budget is the same shape: a counter under the same lock refuses accurately
where the lock exists, and over-spends by at most one run's own limit where it
does not.

`flock` releases when the holding process dies, because the lock lives on an
fd. A crashed holder needs no TTL, no reclaim and no takeover — which is the
whole of the lease-expiry question in open question 9, answered by choosing the
primitive rather than by building the machinery. A holder that is alive but
wedged is handled by giving up after a timeout, which is safe for the same
reason losing the lock is safe.

### Correctness that exclusion does not provide

Two things, and neither is a lock.

**The land critical section**, [0015](0015-landing-is-a-merge-queue.md).
Landing is a read-modify-write over the world — gather, then push — and git
provides atomicity for the write but no isolation for the pair. That one is
real, and it is why landing has a critical section while repairing does not.

**No act on a stale view.** Before opening a thread or landing, a run must have
caught up within `staleness.max_age`; past it, it refuses to act and reports
that it is stale. This is not a concurrency rule. Standing is created only by a
person, and nothing bounds how long it takes to *withdraw* one: a run that has
not fetched still holds a released promise as ratified, and still holds the
pre-revision content whose hash is still ratified, so it dispatches agents
against authority the operator has already taken back. The invariant in
`AGENTS.md` covers minting and says nothing about revocation. Gathering
continues while stale, because observing does not depend on standing.

### What is one-way

Distributed deployment — separate machines, containers, clones — is postponed,
so this decision has to say what may be deferred safely. The line is sharp:
**a durable record's shape is one-way; mechanism and policy are two-way.** The
ledger is append-only, tracked, and re-derived by nothing, so what a record
*says* is permanent and what a process *does* is not.

That line falls somewhere useful because of 0001. **Standing keys on a content
hash, which is world-independent by construction**, so ratification needs
nothing added for distribution, ever. Only truth is about a place.

Three fields, from the first durable write:

| Field | Why it cannot be added later |
|---|---|
| `world` on observation and truth | Nothing else records which tree was measured, and `prior()` folds over that history. [0014](0014-an-observation-names-its-world.md). |
| `seq`, monotonic per run | Recovers causal order within one writer independent of the clock. It fixes a present bug too: a clock stepped backwards, or two records in one tick, make pure event-time folding wrong on one machine. |
| a version on the ledger record envelope | Without it, a future reader of a changed shape is guessing which shape it holds. |

Everything else defers, and the reason it defers is this decision's thesis:
what distribution lacks is a lock that saves money and a critical section
around landing. Neither is a record shape.

The one code discipline that would otherwise block the port: **exclusion goes
through one interface — `acquire(artifact) → grant | refuse` — with one
implementation.** Calling `flock` inline at call sites encodes the assumption
in every caller, and a `refuse` that callers already handle is what makes a
lockless deployment a configuration rather than a rewrite.

## Alternatives

**A durable instance identity** — the earlier draft of this decision. Rejected:
nothing durable was left for it to name once a box belongs to a thread and a
measurement belongs to a world. It also had to be minted and persisted per
workspace, which is a file that a cloned container image would duplicate.

**An ownership partition, one instance per artifact.** Rejected on the strongest
of several grounds: **ranking is global.** The tick evaluates everything due and
ranks all drift to decide what is worth acting on, so confining a run to a slice
gives every run a partial view and makes the queue worse rather than merely
narrower. It also makes adding a run a config edit, and strands an artifact
whose owner is down.

**Leases as a correctness mechanism.** Rejected: the gate already re-derives
from the world and the assessor already asks whether the predicate held before
the repair. A lease that were load-bearing would be a second place the answer
lives, and the two could disagree.

**No exclusion at all.** Nearly right, and rejected only on cost: an agent
dispatch is expensive enough that spending it twice for one drift is worth a
lock file to avoid.

**A daemon owning coordination.** Rejected. `flock` gives crash-safe release
that a daemon would have to reimplement, and a daemon is a component with its
own liveness and its own state that can disagree with the ledger — the tier
discipline says what nothing can re-derive is durable and everything else is
thrown away. The case that would justify one is speculative land batching, which
is a throughput optimization to build after measuring, not before.

## Consequences

- **Two runs can repair the same drift.** The second lands nothing, and its
  thread records that the promise was already kept when it went to land. That
  path gets a test that deliberately produces it, per `AGENTS.md`.
- **Losing a lock is an ordinary outcome, not an error.** Callers handle
  `refuse` on the normal path, which is what keeps a lockless deployment a
  configuration change.
- **Boxes need a pool with a cap**, and a discard policy for the ones a failed
  run left behind. A worktree per concurrent gather is a disk cost that nothing
  currently bounds.
- **Distributed deployment is postponed at a known price**: duplicated agent
  spend, and no isolation around landing until something supplies it. Both are
  mechanism, so nothing written before then has to be rewritten.
- **0011's "several instances of kept" should be read as several runs.** There
  is no other kind of instance in this model.
