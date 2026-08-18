# 0018 — Records are retrospective, so a crash loses work and never truth

**Status** proposed · makes explicit a guarantee hinted at in
[0011](0011-tracked-formats-must-merge.md), [0012](0012-a-run-id-is-opaque.md)
and `layout.md`, and never stated

## Context

Several documents assume durability without defining it. 0012 says a run that
dies mid-tick is replaced by running again, and that there is no run state to
recover. `layout.md` says the index is rebuildable. `model.md` says a crash
re-derives from the corpus and the ledger. None of them says what happens to a
half-written record, a box with a dead agent in it, or a land that pushed and
then died.

The question has teeth because there are three processes that can die
independently — the run, the agent runtime it dispatched, and whatever the
gatherer shelled out to — and because the ledger is append-only and tracked, so
a bad record is permanent.

## Decision

### The principle: record only what has already happened

**Nothing is recorded as an intention.** Observe, act, then record — never
record, then act. The record of an act follows the act's completion, and a
crash between them loses the record and not the act.

Two guarantees fall out, and they are the whole of this decision:

- **A crash can lose the record of something that happened.**
- **A crash can never produce a record of something that did not.**

The first is recoverable and the second would not be. Truth is derived on read
(0001), so a lost verdict is re-derived by observing again; a lost observation
is re-gathered. What could not be recovered is a ledger that claims a
ratification, a land, or a verdict that never occurred, and the ordering rule
is what makes that unreachable.

**The land is where this bites.** Push first, record after. A crash between
them leaves `origin/main` carrying a gated change that the ledger does not
mention — the next tick observes the world, finds the promise kept, and records
it, which is the correct outcome by a different route. Recording first would
leave the ledger asserting a land that does not exist, which nothing can
detect.

### What each tier guarantees

| Tier | Guarantee | On loss |
|---|---|---|
| corpus | durable; ordinary git | recovered from the remote |
| ledger | durable; every record complete or discardable | a torn record is quarantined, not read |
| index, cache | none | rebuilt from the other two |
| boxes | none | discarded |
| in-flight thread | resumable only at recorded checkpoints | re-dispatched from a fresh box |

### Records are complete or discardable

A record is one line of JSON, appended with `O_APPEND` and flushed before the
act that follows it is considered done. A partially written line does not
parse, so **the reader quarantines unparseable lines and reports them, rather
than stopping or guessing.** This is adsr's quarantine-on-parse-failure, which
`lineage.md` already names as worth adopting; here it is load-bearing rather
than a nicety, because a fold that halts on one torn line is a system that one
crash stops permanently.

Observations get this for free and better: they are content-addressed, so a
partial write is detectable by hashing what is there, and a mismatch discards
the file rather than quarantining a line.

### What a dead process leaves

**A dead run.** Nothing to recover. A tick is idempotent, and the next run
re-derives. Its `runs/{id}.json` keeps `Ended` zero and `Exit` empty, which
0012 already defines as the ordinary evidence of an unclean stop. Its `flock`
was released by the kernel.

**A dead agent runtime.** The box holds partial edits of unknown meaning. The
thread record survives, because it is written at each transition — opened,
dispatched, gathered, assessed, landed — and the thread is marked interrupted.
**The box is discarded and the repair is re-dispatched from a fresh one**, and
that is a decision rather than an accident: a half-run agent's tree is work
nobody supervised toward a goal nobody can reconstruct, and resuming into it
would put the assessor in the position of judging a diff with two authors and
no transcript for one of them. The dead box is retained as an artifact under
its own retention so that it can be looked at.

**A dead gatherer.** No observation, so no predicate runs, so the verdict is
`unknown`. This is the existing invariant and it needs nothing added: a
gatherer killed by a crash and one killed by a missing toolchain are the same
fact.

**A crash mid-land, inside the lock.** The `flock` is released, the push either
happened or did not, and the next run's in-lock check against `origin/main`
observes which. There is no third state, because the push is atomic (0015).

### What is swept, and by what

Nothing self-heals silently. Each of these is reported and then acted on:

- worktrees whose thread is finished or dead — `git worktree prune` plus the
  discard policy above
- `runs/{id}.json` with no `Ended`, older than a threshold — closed out as
  unclean, never rewritten as clean
- quarantined ledger lines — surfaced, never deleted, because the surrounding
  records are what say whether one matters

## Alternatives

**Write-ahead intentions, then reconcile.** The standard answer, and it is what
durable execution engines do. Rejected: it makes the ledger able to assert
things that did not happen, and reconciling is a second code path that runs
rarely and is therefore wrong. `model.md` already rejects durable execution as
an answer to a scale problem this does not have; this is the same rejection for
a correctness reason.

**Resume a dead agent's box.** Rejected above, and it is the tempting one
because the box is right there and the tokens were spent.

**Halt on a torn ledger line.** Rejected: one crash would stop the system
permanently, and the failure is exactly the one that arises after a crash.

**Fsync every record.** Rejected as the default: transitions are frequent and
most are heartbeats. Flushed before a dependent act, fsynced at the acts that
matter — a ratification, a land — which is where the guarantee is actually
claimed.

## Consequences

- **`kept` never resumes a repair; it restarts one.** Threads are resumable at
  checkpoints, and a partially repaired tree is not a checkpoint.
- **Spent agent tokens are lost on an agent crash.** Accepted, and it is the
  reason the loss is reported rather than absorbed: a repair that dies twice is
  a signal about the packet, not about luck.
- **The ledger can be behind the world, never ahead of it.** Any tool reading
  it should treat absence as "not recorded", never as "did not happen" — and
  the way to answer "did it happen" is to observe, which is the whole model.
- **A quarantined line is visible forever.** It is in tracked history and
  nothing rewrites it. That is the cost of an append-only tracked ledger and it
  is preferred to a repair path that edits history.
