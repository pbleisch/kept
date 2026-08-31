---
id: 0079-a-repair-is-an-episode-that-reworks-itself
type: decision
title: 0079 — A repair is an episode that reworks itself
fields:
  statement: >-
    A repair is an episode rather than a session. The assessor runs inside it, its
    outcome selects an action from a ratified dispatch policy, and a rework
    re-dispatches into the same thread — so a person prepares artifacts and lands
    work, and does not sit in the rework cycle. The thread is the episode's
    identity, an attempt is delimited by its packet entry, and the assessor reads
    every session in the thread. Two budgets bound it and both escalate rather than
    refusing quietly. `kept review` stays the person's entry point and gates nothing.
links:
  - { target: decision:0078-the-derived-checklist-is-closed-and-the-argument-is-not, relation: extends }
  - { target: decision:0060-a-repair-is-one-promise-a-person-named, relation: amends }
  - { target: decision:0009-repairs-are-not-enumerated, relation: amends }
  - { target: decision:0067-a-repair-anchors-what-it-produced, relation: depends-on }
  - { target: decision:0019-a-refusal-is-a-disposition, relation: depends-on }
  - { target: decision:0052-repair-runtimes-are-closed-adapters-with-native-receipts, relation: depends-on }
  - { target: decision:0077-the-receipt-says-what-answered-and-what-it-consumed, relation: depends-on }
  - { target: policy:the-agents-report-is-never-an-input-to-the-verdict, relation: implements }
---

# 0079 — A repair is an episode that reworks itself

## Context

**The goal that couples all of this** has not been written down before, and
without it these were three unrelated questions:

> Our northstar is that any repair rework can be automatic, where human
> interaction is focused on preparing artifacts — decisions, promises — and
> landing work.

A person prepares artifacts and lands work. A person does not sit in the rework
cycle. Say that out loud and `docs/open-questions.md`'s question 3 (the
act/wait/suppress/escalate policy) and question 4 (a budget that refuses) stop
being separable, and a third question joins them that was never recorded at all:
where the assessor runs. It was never asked because nothing forced it —
[0078](0078-the-derived-checklist-is-closed-and-the-argument-is-not.md)'s derived
half genuinely does not need a box, so it landed where it was easiest to reach
from.

**Where it landed is a dead end for this goal.** `internal/assess` is imported by
`cmd/kept/review.go` and `cmd/kept/json.go` and by nothing else: not
`internal/repair`, not `internal/tick`. It is a command a person types. An
automatic cycle is *assess → fail → re-dispatch → assess*, and a verb cannot be a
step in it.

**Three things turn out to be nearly built**, which changes what the open
questions cost:

- `ledger.Outcome` already declares `act`, `wait`, `suppress` and `escalate`, and
  `ledger.Condition` already declares `budget`, `policy` and `rank` — each
  carrying a comment naming the open question it is waiting for. The vocabulary
  question 3 asks about is already in the record shape; nothing writes those
  values.
- A **dispatch policy** is already a first-class ratified artifact. `policy` is a
  kind, `applies_to: dispatch` is a value the corpus already uses, `on_fail:
  refuse|warn` is declared, and `repair.dispatchAuthority` already collects every
  ratified one and refuses the whole packet if any is unratified. What no code
  does is *evaluate* one at the moment of dispatching — they are carried into the
  packet as authority and read by the agent. So question 3's "most machinery"
  objection to the policy-artifact shape is much weaker than when it was
  recorded: the kind, the schema, the collection and the carriage exist, and what
  is missing is the evaluation the kind was defined for.
- The receipt is already a per-thread append-only series. `thread.Entry` carries
  the thread id and `ledger.Envelope`'s `At`, `Writer` and `Seq`;
  `thread.ReadReceipt` folds by event time, then writer, then sequence, so
  entries written by several runs under one thread id come back in order. Its
  bodies are packet, native, transcript, cost, turn, model, window, lifecycle,
  anchor and measurement. `thread.LatestAnchor`'s own comment already anticipates
  a thread anchoring twice.

**The box question is timing, not provenance, and the record should stop
implying otherwise.** [0067](0067-a-repair-anchors-what-it-produced.md) settles
it: a repair commits what its session left in the box, with the box's own cut
commit as parent, and points `refs/kept/threads/<thread>` at it. The anchor *is*
the repaired box. A box cut from the anchor reconstructs that tree, so there is
no second, different tree and no question about which one is assessed. What a
re-cut box does not recover is what was never committed — untracked scratch, live
process state, and any ability to ask the session anything.

## Decision

### The assessor runs inside the episode, and `kept review` stays a person's

`internal/repair` drives the assessor. After the re-derivation and the scope
review, `assess` is worked over the thread's own entry, its outcome selects an
action, and the episode continues or stops. That is the loop; nothing outside it
is required for the cycle to close.

`kept review <thread>` keeps existing unchanged, and it is a **reader**: it
measures nothing, dispatches nothing, records nothing, gates nothing, and exits 0
whatever it finds. It is what a person types to see what the episode saw. A verb
that both reported and gated would be two things, and the one that gates has to
be reachable without a terminal.

**The verdict is still the re-derivation's.** An assessment decides whether to
rework; it never decides whether the promise is kept.
`policy:the-agents-report-is-never-an-input-to-the-verdict` is untouched and this
is the place a later reader will think otherwise: its subject is the *verdict*,
and a rework decision is not one. Truth continues to come only from measuring the
world.

### The box is re-cut per attempt, and that is a cost paid on purpose

Each attempt cuts a fresh box at the previous attempt's anchor. Correctness does
not turn on this — 0067 makes the anchor the repaired tree, so a re-cut box and
the box that produced it hold the same content — so the choice is cost against
what a long-lived box would buy, and what it would buy is small: untracked
scratch a repair is not entitled to carry forward, and process state no design
here has ever depended on.

What a fresh box buys is worth more. A box that survived an attempt has the
previous agent's leftovers in it, which is exactly the state a scope review
cannot see and a diff against the anchor does not report. Cutting again makes
every attempt's diff *its own*, against a parent the record already names, and
keeps `Change.Against` the one classification it is everywhere else. Measured on
this repository, cutting a box is 203ms against a session bounded in minutes; it
is not the cost that matters in this loop.

### A thread is the episode, and rework continues it

**A rework does not open a new thread.** Every attempt appends to
`threads/<id>.jsonl` under the id the episode was minted with. Three mechanisms
already in the tree decide this rather than a preference:

- **The refs are the queue.** `refs/kept/threads/<thread>` is one ref per thread
  and `kept land` with no argument enumerates them (0067). A thread per attempt
  fills that queue with superseded work nobody should land, and a person reading
  it would have to know which of five refs is the current one. One ref per
  episode, moved to each new anchor, is exactly what a queue of landable work
  should show.
- **The receipt already folds across runs**, by event time then writer then
  sequence, so attempts written by different runs accumulate in order under one
  id with nothing added.
- **The episode is the unit a person named and a budget bounds.** A thread that
  is one session leaves the episode nameless, and a budget can only bound what
  has a name.

**An attempt is delimited by its packet entry.** Exactly one `thread_packet` is
recorded per dispatch, immediately before the launch, so the entries from one
packet entry up to the next are one attempt. That is a derivation over a record
that already exists rather than an ordinal field on every body — a number written
into ten record types is ten places for one answer to live, and a version bump
for each.

### The assessor reads every session in the thread

Because the thread is the episode, cross-session evidence is `thread.ReadReceipt`
and nothing else. The assessor is handed the whole series: every packet, every
transcript, every cost, turn, model and window reading, every lifecycle event,
every measurement, every anchor — grouped into attempts by the rule above.

**This is the adversarial pass's territory and not the checklist's.** Whether an
agent is converging or thrashing, whether each attempt games the predicate a
different way, whether cost climbs while the diff shrinks — none of it is a
question somebody wrote down in advance, which is what 0078 says a closed
checklist cannot reach by construction. The pass gets the series; the derived
checks keep answering about the attempt that produced the anchor, because their
question is about that act.

A derived check may grow to read the series when somebody writes the derivation.
That is the ordinary movement 0078 already describes: what the argument keeps
saying becomes a derivation in the closed half.

### Outcomes select actions, and `unread` is two outcomes

| assessment | action |
|---|---|
| every check `clear` | **ready to land** — the episode stops with its anchor, and a person lands it |
| any check `fired` | **rework**, unless the same check fired before |
| the same check `fired` twice in one episode | **escalate** |
| `unread` from inside the episode | **rework**, and the finding says which evidence the repair left missing |
| `unread` from outside the episode | **escalate** |

**The split on `unread` is by cause, and kept can tell because it knows what it
did.** Evidence missing because *this repair* left the promise unmeasurable, or
destroyed what a check reads, is a finding about the repair: the re-derivation in
the box already reports it as `Undecided`, and that is a rework. Evidence missing
because a half of the assessor does not run, because an observation was pruned,
or because the repository no longer holds the anchor is nobody's fault inside the
episode and no amount of rework fixes it: that is a person's.

**The same check firing twice escalates, and this is where the northstar meets
the deck's own pattern.** `docs/model-deck.html` already says what a repeating
finding means — *the assessor is where a new failure mode shows up, the predicate
is where it goes to stop recurring; a finding that repeats is a predicate you
have not written yet*. Writing that predicate is revising a promise and
ratifying it, which is preparing an artifact, which is precisely the human half
of the northstar. So a repeating finding is not a harder rework; it is the loop
correctly handing back the one kind of work it was never meant to do.

### The shape is a policy artifact, evaluated at the moment of dispatching

Question 3 offers three shapes: a fixed ranking function, per-artifact policy
fields, or a policy artifact matching on the situation. **The policy artifact**,
and the reason the question's own "most machinery" objection no longer holds is
in the Context above: the kind, the schema, `applies_to: dispatch`, `on_fail`,
the collection and the packet carriage are built. What this decision adds is that
a dispatch policy's `hold_when` is **evaluated before a dispatch** and
`on_fail: refuse` stops it — which is what a policy has always been defined as
and has never yet done.

The other two shapes are refused for reasons this repository has already paid
for. A fixed ranking function puts the rule in Go, where changing it is a code
release and nobody ratified it — and standing is created only by ratification.
Per-artifact policy fields put the rule in the promise, where it binds: every
promise re-ratifies when the schema's hash list grows, and tuning a loop would
charge a bulk re-ratification, which is the trap [0047](0047-a-promise-binds-what-a-repair-may-touch.md)
already names for `max_age`.

The vocabulary is `ledger.Outcome`'s, unchanged and finally written: `act` opens
an attempt, `escalate` hands the episode to a person, `suppress` ends it without
one, `wait` is deliberately unused here — an episode runs to completion or stops,
and there is no queue for it to wait in — and `refused` with `Condition` names a
budget or a policy that said no. Each attempt writes exactly one disposition,
which is 0019's rule unchanged.

### Two budgets, and both escalate

Question 4 asks what a budget counts, what it is scoped to, and what happens at
the limit. The answer is **two counters with different jobs**, because one
counter cannot do both.

**Attempts per episode** bounds *this* repair. It is exact, local, needs no lock
and cannot drift, and it is the only counter that bounds thrashing — an agent
that spends one cent per attempt and never converges is stopped by this and by
nothing else. It is scoped to the episode because the episode is what a person
named.

**The runtime's reported window** bounds whether this machine should be starting
work at all. [0077](0077-the-receipt-says-what-answered-and-what-it-consumed.md)
gives it: the runtime's own reading of the usage windows, shared across every
machine and every session on the account, so it needs no lock, nothing
decrements it and it cannot drift. It refuses the *next* dispatch, which is what
question 4 requires of a budget.

**A runtime that reports no window is not a window at zero**, and
`thread.LatestWindow`'s second return value is the whole of that distinction. A
self-hosted runtime reports none; this half then refuses nothing and says so, and
the attempt bound still applies. A budget that read a missing reading as an empty
one would refuse everything on the day it should refuse nothing, and one that
read it as unlimited would refuse nothing on the day it should refuse
everything — so it answers neither and reports that nobody said.

**Money is recorded and is not the counter that refuses.** `CostUpdate` holds an
amount as text per runtime with no cross-runtime conversion, deliberately (0052),
and a self-hosted runtime reports none of it either. A budget that counted money
alone would read a repair costing fourteen minutes of somebody's hardware as
free.

**At the limit it escalates.** It never refuses quietly and never degrades to
detect-only. Under the northstar the limit is one of the few places a person
legitimately re-enters, so it must be visible: the episode stops, its anchor
stands, and the disposition is `escalate` — or `refused` on condition `budget`
where nothing had opened — so `kept why` answers *why was this left alone*
without anyone having to reconstruct it.

### A rework packet states a gap of a second kind

An episode's second attempt has a problem the packet cannot currently express.
After a repair whose assessment fired, the promise may be **kept** — the
re-derivation says so — and `repair.Perform` refuses to dispatch on a promise
that holds, correctly, because a packet states a gap and there is none. But
there is something wrong, and it has a name: the act that made the promise hold
is rejected, for reasons the assessor recorded.

So **a finding is a gap of a second kind**, and a rework packet carries the
findings of the attempts before it. The first kind is *the promise is not kept*;
the second is *the promise is kept and the way it got there is rejected, for
these named reasons*. Both state what is wrong. Neither says what to do about it,
which is the line [0009](0009-repairs-are-not-enumerated.md) draws, and this
extends 0009 rather than crossing it: a finding is evidence about an act that
happened, in the same category as an observation, and it is not a move from a
catalogue. There is still no repair library, no scored candidate and no template.

`policy:the-pipeline-is-never-in-a-packet` is untouched. A finding is not a
desire, a rank, a budget, a disposition or queue state — the budget's own
remaining balance and the attempt's position in any queue stay out, as they are
now.

The alternative was to re-dispatch from the original gap with the previous
attempt discarded and say nothing about why it was rejected. That is a lottery:
it spends a session to rediscover the same wrong answer, and under a budget that
escalates it converts a rework loop into an expensive way of reaching a person.

## Consequences

**It amends 0060 rather than contradicting it, and the distinction is worth
stating because a reader will otherwise find one.** 0060 binds *one promise,
named* and *a tick still never dispatches*, and gives its reason: "several drifts
are a choice a person makes rather than one kept makes for them". A rework makes
no such choice — the promise is the one the person already named, and the episode
is the one they already opened. And a rework is not a tick: a tick verifies
nothing and starts nothing, which is 0060's own reason that condition can never
be met in one. What changes is that `kept repair <promise>` opens an **episode**
rather than dispatching a single session, and 0060 should be read as binding
which promise a repair is about, not how many attempts it takes.

**What it does not settle, and the northstar will force next.** Nothing here
lets kept *open* an episode. A person still types `kept repair <promise>`, which
0060 binds and this does not touch — but `docs/increments.md` already states the
end state as "the queue drains overnight inside a budget it will not exceed",
and a queue that drains overnight is opened by something that is not a person.
That is the third half of question 3 — which drift is worth an episode — and it
is deliberately left open: it is a choice among drifts, which is exactly what
0060's reasoning protects, and taking it needs its own decision and probably its
own supersession of 0060.

**The loop cannot run until the adversarial pass exists.** 0078 leaves it
unbuilt, and an unbuilt half is `unread` from outside the episode on every
thread — which under the table above escalates every episode to a person, always.
That is the correct behaviour and it is also a forcing function: the automatic
cycle is gated on the half that has to be built anyway, rather than on somebody
remembering to check.

**Cost.** An episode can run several sessions before a person sees anything, and
the thing that bounds it is a number in a configuration file rather than a
person's judgement. That is the trade the northstar asks for. What keeps it
honest is that everything the loop did is in the thread — every packet, every
transcript, every finding, every disposition — so *why did this cost four
sessions* is answerable after the fact rather than being a shrug.

**A thread grows without bound within an episode.** A receipt that was one
session's is now an episode's, so a reader wanting one attempt has to group by
packet entry, and retention now prunes measurements out of a series somebody may
want to compare. `policy:durability-follows-reproducibility` already answers the
second — an observation is re-gatherable and prunes; a transcript is not and does
not — and the first is the cost of having one name for the episode.

**`kept review` will show more than one attempt.** Its checklist answers about
the attempt that produced the anchor, and the series is the argument's. A verb
that summed the attempts into one row would be the collapse 0078 already refuses
one level down.

## Alternatives

**Rework as a new thread each time, linked by `PriorAttempt`.** The packet
already has the field, and it would keep each receipt small. It loses on the
queue: `refs/kept/threads/*` is what `kept land` lists, so every superseded
attempt would appear as landable work and a person would have to know which. It
also makes cross-session analysis a walk over a series nothing indexes, where
continuing the thread makes it one read.

**The assessor as a separate verb something else drives** — a daemon, a hook, a
CI step. It keeps `internal/repair` simple and it puts the loop outside kept,
where nothing records why an attempt happened. The record is the product here;
a loop whose reasons live in someone's CI configuration is a loop `kept why`
cannot answer for.

**A fixed ranking function in Go**, for question 3. Simplest to build and
impossible to ratify: it is a rule nobody granted standing to, changed by a
release rather than by an act, which is what the policy kind exists to prevent.

**Per-artifact policy fields.** Rejected on the hash list: `repair` and the
fields that decide what is owed bind, so a loop parameter on a promise would
charge a re-ratification of every promise to tune a schedule — the same test
0047 used to keep `max_age` unbound.

**One budget counting money.** Rejected because it is not comparable across
runtimes, is absent entirely for a self-hosted one, and does not bound
thrashing: a cheap agent that never converges is the failure a budget is most
needed for.
