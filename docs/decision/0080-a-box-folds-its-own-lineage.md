---
id: 0080-a-box-folds-its-own-lineage
type: decision
title: A box folds its own lineage
fields:
  statement: >-
    A measurement made in a repair's box names the thread the box belongs to,
    and a re-derivation there is handed that thread's own earlier measurements
    as history and nothing else.

links:
  - { target: decision:0078-the-derived-checklist-is-closed-and-the-argument-is-not, relation: amends }
  - { target: decision:0014-an-observation-names-its-world, relation: extends }
---

# A box folds its own lineage

## Context

`prior()` is what lets a promise say *the set of tests never shrinks*, and it is
the last step of the throughline in `docs/model-deck.html`: the assessor catches
an instrument substitution once, a clause is added to the promise, and the same
failure is caught from then on by the re-derivation in the box — before a person
sees it and before anything is landed.

Measured, it was caught on the tick and not in the box. `tick.Measure` handed the
predicate no history, so `prior()` folded nothing, the answer was `unknown`, and
the repair was recorded `Undecided`. A promise that compares against history
could not be repaired to a verdict at all.

[0078](0078-the-derived-checklist-is-closed-and-the-argument-is-not.md) recorded
that rather than closing it, and named the two sub-questions it turns on:
whether `World.Thread` is set on a box measurement, and whether a re-derivation
is handed the thread's own history. It declined to answer them because they are
a question about what a world is.

Three facts settle it, and none of them was obvious enough to assume.

**The material exists and it is not the truth series.** A verdict reached in a
box is refused a truth record by `ledger.Reached`, so `Tick.history` — which
folds the observations a *truth* record names — has nothing to read back and
never will. But 0078 made a repair file both of its box measurements in the
observation stream and name them in the receipt, keyed by side and by instrument.
A thread therefore indexes its own lineage, in the one record that outlives the
box.

**A thread is already half of a world, and the filter is already written.**
`model.World` declares `Thread`, `World.Same` compares it, `ledger.worldFields`
addresses it, and `predicate.prior` drops any observation whose thread is not the
current measurement's — *a thread matches a thread, and empty matches empty*. All
of it was built for this and none of it had a writer, so the guarantee was
vacuous: a box measurement and the workspace's own reading of the same tree
carried the same empty thread, hashed to one address, and would have been folded
into one lineage by the very filter written to keep them apart.

**The two comparisons section 13 weighed are not the same comparison.** A box's
before and after measurement are two readings of one lineage, and comparing them
is what a repair *is*. The workspace's last measurement and a box's are two trees
presented as a change over time, which is the second of the three failures
[0014](0014-an-observation-names-its-world.md) exists to stop. So this is not a
choice between reading history in a box and honouring 0014; it is one lineage
that may be folded and one that may not.

## Decision

**A measurement made in a repair's box names the thread the box belongs to.**
`tick.MeasureRequest.Thread` is that episode, and empty is a tree that is nobody's
box — the workspace, a trial, a land's gate. It travels into `model.World.Thread`,
which is addressed, so a box measurement and the same instrument's reading of the
same tree outside a box are two records rather than one. `ledger.Reached` already
refuses a truth record for a world naming a thread; that guard had no writer and
now has one.

**A re-derivation in a box is handed that thread's own earlier measurements as
history, and nothing else.** `tick.MeasureRequest.History` carries them, keyed by
the name the promise gathers each instrument under. They are handed over rather
than read, for the reason `Acts` is: `tick.Measure` opens no ledger, which is what
`promise:standing-is-minted-in-one-place` refuses by name. `kept repair` has them
in hand — it filed them and the receipt named them before the session started —
and a re-derivation performed later reads them back from the receipt.

**History from another lineage is refused, not filtered.** `prior()` would drop
it, and that is precisely why the boundary refuses instead: history the filter
dropped and history nobody offered are the same answer from outside — both
`unknown`, which in a box is recorded `Undecided`. A caller that handed over the
workspace's series would be told nothing at all, by every promise that reads no
history and by the one promise the mistake is about. So the refusal names both
lineages.

**The thread is minted before the box, and the receipt adopts it.** Everything
measured in a box names the thread it was measured in, and the first measurement
happens before there is anything to record. A receipt is opened when there is;
a repair that holds, refuses, or stops at a dry run records nothing and leaves the
id naming a box that was discarded.

## Consequences

**The throughline closes.** Strengthening the predicate now makes the
substitution mechanical in the box as well as on the tick, and a repair that
games its instrument is recorded `Unrepaired` — a verdict — rather than
`Undecided`. `TestAStrengthenedPredicateIsMechanicalInTheBoxToo` is the whole of
it, over the same fixture that recorded the gap.

**0014's guarantee is real rather than latent, and there are two layers of it.**
`internal/tick`'s refusal is what a caller meets, and `predicate.prior`'s filter
is what the definition is. Each has a test that breaks it on purpose:
`TestABoxMeasurementNamesItsThreadAndRefusesAnotherLineagesHistory` clears the
thread and watches the refusal stop refusing, and
`TestARepairsOwnLineageIsFoldedAndTheWorkspacesSeriesNeverIs` collapses the two
lineages into one and watches the substitution read `kept`.

**A box measurement no longer dedups with the workspace's, and that is the
cost.** Two identical trees measured by one instrument used to be one file
whether or not either was a box; now an episode's measurements are its own. What
that buys is 0035's own sentence becoming true — a verdict reached in a box
*belongs on the thread, where it is pruned with the thread rather than
accumulating forever beside the answers a person reads* — and what it costs is
one observation per instrument per side per episode, in the tier that is prunable
by design.

**The packet's bytes now carry the thread**, which is exactly what 0078 said
would change. It is provenance on a measurement and not an ask: a packet already
carries thread ids for prior attempts, and the world says where the evidence was
measured. Nothing in `hold_when` can read it.

**The first measurement of an episode has no predecessor and reads `unknown`.**
That is not the gap this closes and must not be treated as one. A promise
comparing against history answers a question about a change over time, and the
first reading of a lineage is not one; reporting it as `not-kept` would be the
collapse the three-valued verdict exists to prevent.

**A dry run's packet names a thread nothing opened.** The box was cut, the
promise was measured in it, and the packet says so; the episode then stopped
before there was anything to record. An id that names a discarded box is cheaper
than a measurement whose world cannot be stated until after the packet is
assembled from it.

**A land's gate is untouched, and still answers `unknown` for a promise that
reads history.** `internal/land` names no thread and folds no history on either
side of the merge, so such a promise reads `unknown` before and after, and
`unknown → unknown` is not a break. Widening it is a different question with a
different answer: a gate box holds a tree that has never existed anywhere, and
whether the workspace's series is its lineage or a second tree presented as a
change over time is not settled by anything here. Nothing has forced it, and this
decision does not guess.

## Alternatives

**Declare that a box re-derivation may never read history, and that such a
promise is simply not repairable.** Section 13 called it not obviously worse and
it was not: it is one sentence, it needs no code, and it keeps 0014's rule
trivially safe. What decided against it is that the field, the filter and the
comparison were all already built for this, and the material was already durable —
so the state being defended was not a decision anyone had made but the absence of
one assignment. It would also have put a permanent ceiling on `hold_when` that is
invisible from the promise: an author writing `prior()` would get a promise kept
could measure, refuse to repair, and never say why.

**Filter the offered history instead of refusing it.** Cheaper, and wrong in the
direction that is silent. The filter already exists one layer down; a second one
at the boundary would turn a caller's mistake into an `unknown`, which is
indistinguishable from the promise having no predecessor yet.

**Read the history back out of the receipt at the moment of re-derivation.** It
is the durable path and it is what a later assessment must do, but in the middle
of an episode the measurements are already in hand and were filed by this run —
so re-reading them would be a second answer to a settled question, with its own
failure mode, at the one point where the first answer cannot be wrong.

**Give a box its own truth series.** It would make `prior()` work by making a box
look like a workspace, and it is the thing 0035 refuses: verdicts reached in a box
are evidence about a repair, and a series for them would accumulate beside the
answers a person reads with nothing pruning it.
