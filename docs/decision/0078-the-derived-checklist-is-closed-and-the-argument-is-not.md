---
id: 0078-the-derived-checklist-is-closed-and-the-argument-is-not
type: decision
title: 0078 — The derived checklist is closed and the adversarial argument is not
fields:
  statement: >-
    The thread assessor has two differently-shaped halves over one thread's ledger
    entry. The derived checklist is closed — one three-valued finding per question,
    each derived on read from evidence a repair already made durable. The
    adversarial argument is a model pass whose findings are unbounded and unslotted,
    prompted to argue that the repair does not meet the thread's intent, free to
    read the derived findings and free to notice what nobody thought to ask. Neither
    half is the other's fallback. A repair therefore files both of its box
    measurements in the observation stream and names them in the receipt.
links:
  - { target: decision:0009-repairs-are-not-enumerated, relation: extends }
  - { target: decision:0052-repair-runtimes-are-closed-adapters-with-native-receipts, relation: extends }
  - { target: decision:0058-the-ask-is-declared-and-travels-beside-the-packet, relation: depends-on }
  - { target: decision:0067-a-repair-anchors-what-it-produced, relation: depends-on }
  - { target: decision:0061-a-packet-carries-every-instruments-measurement, relation: depends-on }
  - { target: decision:0036-the-predicate-language, relation: depends-on }
  - { target: policy:the-agents-report-is-never-an-input-to-the-verdict, relation: implements }
---

# 0078 — The derived checklist is closed and the adversarial argument is not

## Context

A repair is checked twice today and neither check reads a word the session
wrote. The re-derivation asks *is the promise kept in the tree the session
left*, and the scope review asks *did it stay inside what the promise bound*. A
thread can pass both and still be wrong, and the way it does is not exotic: the
predicate is a boolean over one measurement, so it cannot see the measurement
get smaller. Six tests become two, `all(obs.tests.packages, p => p.ok)` is as
true as it was, and the coverage that mattered is gone.

`docs/document/model.md` names the answer — a second agent reading the thread's
ledger entry and working an explicit checklist — and names its constraints:
each check recorded on its own, framed as an attack, and verification attached
to acts rather than to instruments. Its sharpest sentence is about framing:
*an assessor asked whether criteria are met will find that they are*.

Two things had to be settled before any of it could be built.

**What each question is worked by.** The document's own worked example answers
half of it: of its two findings it says "found from the diff and the two
observations… the first finding is a pattern in a diff; the second is a
comparison of two observations that are both in the ledger already". Four of the
six questions are decidable that way and one more nearly is. A derivation of a
decidable question beats a model call on it for reasons that have nothing to do
with which model: it is reproducible, it runs in the suite on every commit, it
costs nothing, and it can be broken on purpose and watched to fail — *a check
that has never fired is not a check*. A question answered by a model that
nothing can re-run is a check nobody can test.

**What the model is for, then.** Not the leftovers. The last paragraph of this
decision's first draft made the case against that draft's own shape:

> A promise that was the wrong proxy from the start measures something nobody
> cared about, and every mechanism here can be working while that is true.
> Nothing in a checklist reaches it.

That is the failure class a model is *uniquely* able to reach, and a checklist
structurally cannot — so putting the model in a checklist slot gives it a
ceiling of one question and throws away the thing it was brought in for. The
value of a general adversarial pass is exactly that it is not enumerated.

**What a derivation would read.** The evidence is nearly all durable already.
The packet is stored as its own artifact before the launch and carries the
ratified promise, its `hold_when`, its `gather` and its repair scope, frozen at
dispatch. The anchor is a commit and its parent, so the diff is
`git diff <parent> <commit>` and did not need recording
([0067](0067-a-repair-anchors-what-it-produced.md)). What is missing is the pair
of measurements: `tick.Measure` writes nothing by design
([0035](0035-a-verdict-names-its-world-in-the-record.md)), so both sides of the
comparison were computed and dropped, and the *after* side existed nowhere at
all.

## Decision

**The assessor has two halves and they are shaped differently.** They are not
two implementations of one thing and neither is the other's fallback. A finding
says which half produced it.

### The derived checklist, which is closed

It is `docs/document/model.md`'s questions, in its order, minus the one that is
a reading of prose — five of the six. Each is answered separately and always: a
check left out is indistinguishable from a check that found nothing. **Closed**
means a question is not added to *this* half without a decision, because each
entry here is a derivation somebody has to write, test and be able to break on
purpose. It does not mean the assessor's reach is fixed; that is the other
half's, and the two must not be confused.

A finding is three-valued. `clear` is *worked, and found nothing*, `fired` is
*worked, and found something*, `unread` is *the evidence this needs is not
here*. Collapsing `unread` into `clear` is the same mistake as collapsing
`unknown` into `not-kept` one level down, and it does more damage here: it
reports a thread nobody examined as a repair nobody found fault with.

**Findings are derived on read and nothing is recorded.** `kept review <thread>`
measures nothing, dispatches nothing and appends nothing, so it runs against a
checkout nobody may write to — the rule `kept status` and `kept why` already
follow ([0001](0001-standing-and-truth-live-in-the-ledger.md),
[0032](0032-standing-is-a-fold-over-acts.md)). A stored finding would be a cache
that can disagree with its source, and the source moves: the ordinary next step
after a finding is revising the promise. What keeps the answer stable across
that revision is that the derivation reads the **packet's** promise — the
wording a person ratified before this repair — rather than the corpus's. The one
check that reads today's corpus is the one asking what *other* promises read
these paths, which is a question about the repository rather than about the
thread.

**The comparison is over the dimensions the predicate does not read.** This is
the rule the derived half turns on, and it is what separates a substitution from
a repair. A predicate already asserts a direction for every field it reads: a
promise holding when `count(obs.corpus.broken_links, l => true) == 0` wants that
list to empty, and a check reporting it emptying would fire on the repair the
promise exists to get. A substitution lives in what the wording never mentions —
`tests_run` is not in `all(…, p => p.ok)` — so the comparison is over exactly
the rest. `predicate.Expr.Selected` answers which names an expression selects,
off the tree and never off the text.

Three consequences of that rule are deliberate. A number that fell where another
rose is a measurement that moved rather than one that shrank, because a repair
turns failures into passes. A member that left one collection and is still
somewhere else in the measurement is not gone, because a failing test that stops
failing leaves `failures` and stays in `cases`. And list members are correlated
by their string fields jointly, rather than by a field this code chose: choosing
one means knowing what the list is about, which is how a checklist becomes an
assessment of the instrument.

### The adversarial argument, which is not closed

A model reads the whole ledger entry — the packet, the anchor's diff, both
measurements, the transcript, and the derived findings — and is asked to make
the case that **the repair does not meet the thread's intent**. Not to grade it,
not to confirm it, and not to answer a list.

**Its findings are unbounded and unslotted.** There is no fixed question set, no
enumeration of what it may look for, and no schema mapping a finding back to a
checklist entry. That freedom is the whole of its value: the failure it exists
to reach is the one nobody thought to ask about, and every mechanism that would
make its output tidier is a mechanism that narrows it. A future reader wanting
to add structure here should read this paragraph as the reason not to. If a
finding it produces turns out to be derivable, the answer is to write the
derivation and add it to the closed half — never to add the question to this
one.

**It may read the derived findings**, because an argument is better for knowing
what has already been established, and worse for having to re-establish it.
Nothing flows the other way: a derived finding is never revised, softened or
withdrawn by the argument, which is
`policy:the-agents-report-is-never-an-input-to-the-verdict` at the seam between
the two halves.

**It is prompted as an attack.** The ask is 0058's — a value from a closed set,
byte-identical for every thread, naming no promise, no file and no evidence — a
second constant beside the repair ask and never a template. What varies between
threads is the packet-shaped evidence it is handed, never the framing.

**Which model answers it, and how hard it tries, are configuration.** The
assessor's runtime, model and effort are selected independently of the
repairer's, and the ask may carry in-context material to raise its hit rate.
*Assessor and repairer share weights* is therefore a thing an operator tunes
rather than a property of the design, and it is not a reason for this half to be
small. It remains a real limit in the sense the model document means — two
instances of one model can find one substitution reasonable — and the answer to
that is a different model or more effort, not less argument.

**Its findings are durable.** A session is not reproducible, which is
`policy:durability-follows-reproducibility`, so they are receipt entries rather
than a derivation — the opposite of the closed half, and for a reason that
follows from what each one is.

### Both halves

**A repair files both box measurements.** Each goes to the observation stream it
would have gone to anywhere else — content-addressed, prunable, deduplicated
against an identical measurement whoever made it — and the receipt gains a
`thread_measurement` entry naming the side, the instrument's own key and the
address. The entry is the part that is not derivable from anything else: a box
is discarded, so without it nothing says these two were the sides of this
session. It carries **no verdict**, because a predicate over an observation is
derived wherever else kept asks that question and the packet already holds the
wording to apply.

**A receipt that named no measurement is still read from its packet.** A
packet's `observations` *are* the measurement that found the gap — what the
re-derivation in the box returned, and what the packet was assembled from — so
the before side is readable for every thread in every ledger, including the ones
written before this. It is not a second source for one fact: a receipt naming an
address wins, and this is the same measurement read out of the record that
already carried it. The after side has no such fallback and reads `unread`,
which is what the check then says.

**The grade is derived from the promise's instruments, weakest first, and an
ungraded instrument is no grade at all.** `docs/document/model.md`'s Autonomy
table is the source. `command` has no row there and neither does anything under
the third-party marker, so a promise naming one is reported as *ungraded*
rather than defaulted to either end — nothing has answered is not an answer.
The line printed says *grade* rather than *gate*: a promise has one grade, and
which gate a thread is waiting at is open question 3.

## Consequences

**Half of this is built and half is not, and the report says which.** The five
derived checks run today. The adversarial argument is specified here and
unbuilt: it needs its own ask
([0058](0058-the-ask-is-declared-and-travels-beside-the-packet.md)), a read-only
box, and a live confined proof of the kind
[0052](0052-repair-runtimes-are-closed-adapters-with-native-receipts.md)
requires before a dispatch is admitted. `kept review` reports it as a section
that has not run, beside the checklist rather than inside it, and names the
evidence that is waiting for it.

**Where the assessor runs, and what its outcome does, are not settled here.**
`kept review` is the only thing that invokes either half, and it is a command a
person types; nothing in `internal/repair` or `internal/tick` reaches this
package. That was never chosen — the derived half needs no box, so nothing forced
the question.
[0079](0079-a-repair-is-an-episode-that-reworks-itself.md) asks it and proposes
an answer: the assessor runs inside the repair episode, its outcome selects an
action, and `kept review` stays the person's entry point and gates nothing. Read
that before assuming an assessment does anything today; here it is a report.

**Its finding shape is deliberately undeclared.** Nothing here says what an
argument's finding is made of, and this build declares no type for one. A record
shape written before the thing that produces it is a guess about what it will
say, and the freedom above is the first thing such a guess would cost.

**So the `assessor must pass` gate cannot be met today.** Half the assessor does
not run, and a half that has not run is not a pass. That is the correct state
while nothing auto-lands, and it is why the grading is reported rather than
enforced.

**`prior()` is mechanical on the tick and unknown in a box.** The throughline
this was built for ends by strengthening a predicate so the same failure is
caught by the re-derivation. Measured: it is — over the workspace's series,
where `prior()` has a history to fold. In a box it is not, because
`tick.Measure` supplies no history, deliberately and with a reason: a verdict
reached in a box never entered the truth series, so there is nothing there to
read back. A promise strengthened this way therefore reads `unknown` in the box
and its repair is recorded `Undecided`. Closing that means deciding what a box's
own earlier measurement is to the predicate standing in it — whether
`World.Thread` is set on a box measurement, and whether a re-derivation is handed
the thread's own history — which is a question about what a world is and is not
this decision's to settle. It is recorded rather than resolved, with a test that
fails the day the answer changes.

[0080](0080-a-box-folds-its-own-lineage.md) answers both with yes, and amends the
two paragraphs above: a box measurement names its thread, a re-derivation folds
that thread's own earlier measurements and refuses any other lineage's, and the
packet's bytes carry the thread as a result. The record here is what was true
when this was written.

**A box measurement now reaches the ledger, and does not reach `prior()`.**
`prior()` folds the observations a *truth* record names, and a verdict reached
in a box is refused a truth record by `ledger.Reached`, so nothing about the
workspace's series moves. `World.Thread` stays unset, which keeps the packet's
bytes what they were; the receipt is what says which thread a measurement
belonged to.

**A finding that repeats is a predicate nobody has written**, and in the derived
half that is mechanical rather than advice. A check fires on a dimension the
wording cannot see; naming the dimension in `hold_when` takes it out of the
check's reach, and the re-derivation carries it from then on. The same movement
runs one step further out: an argument that keeps making one point is a
derivation nobody has written, and the answer is to write it into the closed
half.

**A finding is not an exit code.** `kept review` exits 0 whatever it found, the
reading `kept doctor` already gives its own findings: nothing here refuses,
nothing auto-lands, and reporting a finding as drift would say a promise with
standing is not kept when the promise is kept and only the route to it is in
question. What is not 0 is a checklist nothing could work, which is 3.

**Two costs in the comparison, both accepted.** `Selected` is a union over the
whole expression rather than an answer per gatherer, so a promise with two
instruments treats a field name read on either as read on both — which
over-approximates what the predicate sees and makes the check quieter, the safe
direction for something that must not cry wolf. And a list whose members carry
no string fields, or whose keys repeat, is compared by nothing rather than by
position: a correlation that guessed would report a member as lost when two of
them merely swapped places.

**What neither half reaches is a promise nobody should have written.** The
argument can see that a repair replaced an instrument; it cannot see that
counting test files was never what anybody cared about, because nothing in the
ledger entry says what was cared about. That one stays with whoever writes the
promise, and `why` is the only thing pointed at it.
