---
id: 0037-a-conflict-is-discovered-by-exhausted-repair
type: decision
title: 0037 — A conflict is discovered by exhausted repair, and the gate covers every promise the diff touches
links:
  - { target: decision:0015-landing-is-a-merge-queue, relation: amends }
  - { target: decision:0019-a-refusal-is-a-disposition, relation: depends-on }
---

# 0037 — A conflict is discovered by exhausted repair, and the gate covers every promise the diff touches

## Context

`model.md` has declared a `conflicted` drift kind from the beginning — *"two
standing artifacts, incompatible truths"* — and `model.Drift` declares
`DriftConflicted` in code. **Nothing anywhere says how an artifact ever becomes
conflicted.** The row describes a state and not a test. Once the tick can emit
this kind, that stops being tolerable: either the tick has a rule, or the kind
is aspirational.

The reason nothing said how is that the obvious answers do not work, and it is
worth recording that they were tried rather than leaving the next reader to
rediscover it.

**Incompatibility is not a function of the verdicts.** Two promises both
`not-kept` are not in conflict — they are two broken promises. Two both `kept`
may well be in conflict and nothing about their being kept says so. There is
nothing to compute from the truth series, because the truth series does not
contain the information.

**Statically, over predicates.** Deciding whether two `hold_when` expressions can
both be satisfied is satisfiability over a language with folds and `prior()`,
which is undecidable in general — and the cases that would fall out of a
restricted fragment are the trivial ones nobody writes.

**Semantically, over prose.** Two canon entries saying *Kael's eyes are green*
and *Kael's eyes are brown* contradict, and most canon has no gatherer at all
([0007](0007-canon-is-ratified.md)). There is no truth to compare, only text, and
comparing text for contradiction is a model call — which is the thing
[0009](0009-repairs-are-not-enumerated.md) keeps out of the compiled half of the
loop.

### The larger problem underneath, and it is the substantive one

Working the question through surfaced something bigger than the drift kind.
**0015's gate re-derives only the promise its thread was about.**

0015 states its invariant as *every commit on `origin/main` was gated against the
tree it landed on*, which reads as a statement about the tree. It is not. It is a
statement about one promise measured against that tree. So a repair can satisfy
A, break B, and land clean — the merge is clean, the gate passes, and nothing
rejects it.

The assessor's sixth check — *did the repair touch evidence that other promises
read?* — is the closest thing kept has, and per
[0006](0006-no-probe-assess-the-thread.md) the assessor reads and reports. It
cannot block a land.

**Self-correction is not enough**, and this is why the two problems are one. The
next tick would see B `not-kept` and open a thread; that thread's repair could
break A; and the tick after that opens a thread for A. That is a **repair war**,
and without conflict detection it has no exit — it runs until the budget refuses,
silently, on a ground that describes the budget rather than the promises. Two
mechanisms that each look complete produce an unbounded loop between them.

## Decision

**The gate re-derives every standing artifact whose scope the diff touches. A
repair that satisfies its own promise and breaks another does not land. When it
happens twice, both artifacts are `conflicted`.**

### The gate is widened, which is what makes conflict observable at all

0015's gate runs against the promise the thread was for. It now runs against
that promise **and every standing artifact whose scope the diff's paths
intersect**. The invariant 0015 states becomes the invariant 0015 reads like:

> every commit on `origin/main` was gated against the tree it landed on, for
> every promise that tree's change could affect.

**The cost is smaller than it sounds, because both inputs already exist.** 0015
computes path overlap already, for the disjoint-scope skip — *the ledger records
what paths the intervening land touched, and the promise declares its
`RepairScope`; if the two are disjoint the re-gate is skipped*. That same overlap
answers *which standing artifacts care about these paths*, from the same two
inputs, in the other direction. And observations are cached per
`(gatherer, args, world)` ([0014](0014-an-observation-names-its-world.md)), so
promises naming the same gatherer over the same scope share one run of it. You
re-derive the overlapping few, not the corpus.

Where a promise's scope is unknown or unbounded, it is in the overlapping set.
That is the safe direction and it is the same direction 0015 already takes when
an intervening land's paths are unknown.

### A collateral break refuses the land and names the pair

A gate that finds the thread's own promise `kept` and some other standing
artifact newly `not-kept` **does not land**. The thread records the pair — which
promise was repaired, which was broken, and the observations on both sides — and
the change stays in its box.

**One occurrence is not a conflict. It is a bad repair**, and it is treated as
one. The next packet names the broken artifact under **prior attempts**, which
is machinery the packet already carries and which needs nothing new: the agent is
told, in the terms 0009 allows, that its last attempt satisfied A by breaking B.
Most of the time that is enough, because most of the time the first repair was
simply careless.

### The second occurrence escalates, and marks both

If a second attempt breaks the same artifact anyway — having been told — the
disposition is **`escalate`**, which is already one of 0019's four and needs no
fifth, and **both artifacts read `conflicted`**.

**Both, not one**, and the asymmetry that would pick one is exactly what there is
no basis for. Nothing in the ledger says A is the artifact that should yield.
They are two ratified statements that cannot both hold, and which one is wrong is
a question about what the operator meant — which is why `model.md` already says
`conflicted` *needs a ruling from whoever has standing over both* rather than
more observation.

Twice rather than once, because once is indistinguishable from carelessness and
this is a judgement the system should not reach cheaply. Twice, having been told,
is evidence about the promises rather than about the agent.

### Exit is a person's act, and needs no new verb

Release one, or revise one and ratify it again. Both are acts that already exist
and are already reserved to a person
([0032](0032-standing-is-a-fold-over-acts.md)), and both clear the state by
construction: `conflicted` is derived like every other drift kind
([0001](0001-standing-and-truth-live-in-the-ledger.md)), so there is no flag to
clear and no resolution to record. A released artifact has no standing, so there
is no longer a pair. A revised one has a new content hash, so its truth series
starts fresh and the next tick simply does not find the conflict.

That is what makes `conflicted` a query result rather than a record, which is the
property `model.md` requires of every drift kind.

### What this cannot see, and it is not a small exclusion

**Two unchecked canon entries that contradict in prose are invisible.** *Kael's
eyes are green* and *Kael's eyes are brown*, both ratified, neither gathered, is
exactly the case the phrase "incompatible truths" brings to mind — and this
decision does not detect it. Neither has a truth at all; there is only text.

It surfaces only when a promise about that prose fails, at which point it reduces
to the path above and is detected the same way. Recorded as a known limit rather
than left implied by a drift kind that appears to promise more, because a kind
whose name suggests it covers prose contradiction, in a tool built for
manuscripts, will otherwise be read as covering it.

### Why this shape and not a cleverer one

It is the only detectable form, and it fits the model's epistemology exactly.
**kept does not reason about statements; it observes.** You learn that two
promises cannot both hold the same way you learn everything else here — by trying
and watching. The evidence is a repair that was told and did it anyway, which is
a fact about the world and not an inference about meaning.

It is also the same move 0013 and 0015 both make: correctness moves to the place
where the facts are known. Conflict is not knowable when the promises are
authored, and it is knowable at the gate, after a repair, twice.

## Alternatives

**Detect statically, by analysing predicates.** Rejected: undecidable in general
over a language with folds and `prior()`, and a decidable fragment would catch
only the contradictions nobody writes.

**Detect semantically, by asking a model whether two statements contradict.**
Rejected. It puts a model call in the compiled half of the loop, against 0009,
and it makes a drift kind — which is a query result — depend on a judgement that
is not reproducible. It would also fire constantly on canon that merely sounds
similar.

**Infer it from verdicts.** Rejected because there is nothing to infer from.
Nothing distinguishes two promises that are both failing from two that are
mutually exclusive, and treating the first as the second would mark half a
broken corpus `conflicted`.

**Mark `conflicted` on the first collateral break.** Rejected: one break is a bad
repair, and the packet already has the machinery to say so. Reaching a judgement
about two ratified statements on one piece of evidence would make the state cheap
and therefore ignorable, and the whole value of `conflicted` is that it means
somebody has to decide something.

**Mark only the artifact that was broken.** Rejected: it implies the repaired one
was right, and nothing supports that. The pair is the finding.

**Let self-correction handle it — the next tick repairs B.** Rejected, and this
is the one that looks sufficient until the loop is drawn: A's repair breaks B, B's
repair breaks A, and nothing terminates it. The exit is a budget refusal recorded
against a ground that says nothing about the promises, which is precisely the
unexplainable queue 0019 exists to prevent.

**A fifth disposition, `conflicted`, beside 0019's four.** Rejected: `escalate`
already means *this needs a person*, which is exactly what has been established.
A fifth would be a second way of saying it, and the drift kind already carries the
distinction.

**Widen the gate and stop there, without conflict detection.** Rejected as
incomplete rather than wrong. A widened gate refuses the land, correctly, and
then the same repair is attempted forever with nothing recording why it never
succeeds. The two halves of this decision are one fix: **widening the gate is
what makes conflict detectable, and conflict detection is what stops the loop the
widened gate reveals.**

## Consequences

- **0015's gate is more expensive**, by the number of standing artifacts whose
  scopes the diff touches. Bounded by path overlap, which 0015 already computes,
  and reduced by the observation cache, which promises sharing a gatherer already
  share. A diff touching nothing any other promise names costs exactly what it
  costs today.
- **A land can now be refused for a reason that is not about its own promise**,
  which is new, and the refusal is a disposition under 0019 naming the artifact
  that was broken. A thread that fails this way is not a failed repair; it is a
  repair that worked and was not allowed to land, and the record says so.
- **A promise with no declared scope makes every gate wider.** That is the safe
  direction and it is a reason to declare one; `kept doctor` should say so.
- **`conflicted` is reachable**, and its two artifacts are found by the same
  derived query as every other drift kind. Nothing is written to say an artifact
  is conflicted, and nothing has to be cleared when it stops being so.
- **The repair war is bounded by a recorded state rather than by the budget.**
  Before this, the loop's exit was a refusal whose stated ground was money;
  after it, the exit names the two promises and asks for a ruling.
- **`model.md` gains a known limit** for prose contradiction between unchecked
  canon, and its drift table says how `conflicted` is reached rather than only
  what it looks like.
- **Two tests that must be able to fire**, per `AGENTS.md`: a repair that
  satisfies its own promise and breaks another is refused at the gate rather
  than landed, and a second such attempt marks both artifacts `conflicted`
  rather than opening a third thread.
