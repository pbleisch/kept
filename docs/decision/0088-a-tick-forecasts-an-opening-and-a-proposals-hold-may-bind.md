---
id: 0088-a-tick-forecasts-an-opening-and-a-proposals-hold-may-bind
type: decision
title: 0088 — A tick forecasts an opening, a proposal is its own record, and a ratified rule may act on its hold
fields:
  statement: >-
    When kept opens repair episodes by itself, the opening rules are evaluated by
    the opener, which alone writes a disposition from them; a tick evaluates the
    same rules and shows the verdict as a forecast, in its output and in `kept
    status`, and records nothing from it. A session that proposes an order for the
    opener is filed under a record kind of its own, never under a thread id. And a
    ratified opening rule may read a proposal's hold and refuse to open on it, so a
    model's output may prevent work — but only where a person ratified a rule that
    says so.
links:
  - { target: decision:0009-repairs-are-not-enumerated, relation: amends }
  - { target: decision:0079-a-repair-is-an-episode-that-reworks-itself, relation: depends-on }
  - { target: decision:0019-a-refusal-is-a-disposition, relation: depends-on }
  - { target: decision:0077-the-receipt-says-what-answered-and-what-it-consumed, relation: depends-on }
  - { target: decision:0081-the-adversarial-pass-argues-in-a-box-it-cannot-write, relation: depends-on }
  - { target: decision:0086-the-pass-rules-and-a-rejected-repair-is-reworked, relation: depends-on }
  - { target: policy:a-tick-that-does-nothing-records-why, relation: implements }
  - { target: policy:record-only-what-has-already-happened, relation: implements }
  - { target: policy:standing-is-created-only-by-ratification, relation: implements }
---

# 0088 — A tick forecasts an opening, a proposal is its own record, and a ratified rule may act on its hold

## Context

`docs/increments.md` ends with a queue that drains overnight, and a queue that
drains overnight is opened by something that is not a person. Open question 3
records that closing it needs a supersession of
[0060](0060-a-repair-is-one-promise-a-person-named.md), which says a repair is
dispatched by `kept repair <promise>` and never by a tick. That supersession is
not written yet, and neither is the opener. Call it **the opener**: a verb run
from cron or by hand, not by a hook, that looks at what drifts and opens repair
episodes under rules a person ratified. Call those **opening rules**: ordinary
policy artifacts whose `applies_to` names the act of opening an episode. An
optional confined session may propose the order in which the opener considers
candidates when more qualify than a cap admits. Call that a **proposal**: an
ordering, which is the only part kept acts on, plus prose that nobody grades.

Three questions about that design have answers before any of it is built, and
each one constrains whatever decision introduces the opener:

1. Does a tick evaluate the opening rules, or only the opener?
2. Where does a proposal's receipt live, given that it repairs no promise and
   so has no thread?
3. May anything act on a proposal's judgement that a candidate should be held,
   or may a proposal only reorder?

`docs/document/model.md`'s loop sketch put `disposition = policy(drift)` inside
the tick. That answered the first question without asking it, and the sketch is
unratified.

## Decision

### The opener records what an opening rule says, and a tick only forecasts it

**Only the opener writes a disposition from an opening rule.** The opener
evaluates every ratified opening rule against each candidate at the moment it
decides whether to open an episode for it, and the disposition it writes (`act`,
`wait`, `suppress`, `escalate` or `refused`, with its condition) is the record of
that decision. This is 0019's rule unchanged: one disposition per transition,
written by the run that acted.

**A tick evaluates the same rules and records nothing from them.** It shows the
verdict as a forecast, saying what the opener would do with this drift if it ran
now, in the tick's own output and in `kept status`. `kept status` derives that
forecast on read and writes nothing, as it does for everything else it shows. The
tick's own disposition stays what it is: `refused · provider`, because a tick
starts nothing.

A forecast is not recorded because it is not an act. Suppose a tick in a hook
wrote `wait · policy` at 17:00 for a promise that the opener opens at 02:00. That
record would claim that the tick waited because of a rule, when a tick waits for nothing
and acts on nothing. It would also record a rule's verdict at a moment the real
decision never saw. `policy:record-only-what-has-already-happened` refuses that
shape. `policy:a-tick-that-does-nothing-records-why` is still met, because the
tick still records why it did nothing.

**This is in tension with ratified [0079](0079-a-repair-is-an-episode-that-reworks-itself.md),
and does not contradict it.** 0079's `### The shape is a policy artifact,
evaluated at the moment of dispatching` binds that a policy's `hold_when` is
"evaluated before a dispatch". A tick's forecast evaluates the opening rules
where no dispatch follows. Nothing in 0079 forbids that, and the evaluation
that binds is still the opener's, at the moment of opening, as 0079 requires. But
a policy has so far been something evaluated only at the moment of acting, and
here one is also evaluated to preview the act. The line that keeps the two
apart is that only the act's own evaluation is recorded or can refuse anything.

### A proposal has a record kind of its own

**A proposal's receipt is filed in a stream of its own in the ledger, beside
`threads/` and never inside it.** It carries what the session was handed, the
ordering it returned, what it argued verbatim, and the cost and window readings
of [0077](0077-the-receipt-says-what-answered-and-what-it-consumed.md), using the
receipt entry types `internal/thread` already has wherever they fit. A
disposition the opener writes from a run that used a proposal names that
proposal, in the same way an `act` names its thread.

Two alternatives were rejected. **A thread id** would make 0079's
`### A thread is the episode, and rework continues it` false: a thread would
become any session series, `kept review` would have to cope with a receipt that
repairs nothing, and the ratified text would have to be revised. **No record of
its own**, with each disposition pointing at the proposal's words, loses every
proposal that changes no disposition. Dispositions are written on transitions
only (`Disposition.Moved`), so a second night that reorders candidates whose
outcome stays `wait · rank` writes nothing, and nothing would point at that
proposal or hold its cost.

A proposal anchors nothing and creates no `refs/kept/threads/` ref, so it never
enters the land queue.

### A ratified rule may act on a proposal's hold

A proposal may say that a candidate should be held, for example because several
drifts share one cause and a person should look at them together. **kept exposes
that judgement to opening rules as plain facts per candidate**: whether a
proposal ordered this run, the position it gave this candidate, and whether it
held this candidate. A ratified opening rule may read the hold and refuse to open
on it. The disposition then names that rule, never the proposal, because the
authority is the rule's.

**With no such rule ratified, a proposal only reorders.** The facts exist and
nothing reads them, so a hold changes nothing until a person ratifies a rule
that reads it. That person's ratification is dated and revocable by retirement.
**A missing or failed proposal holds nothing.** Where no proposal ran, or the
session failed or returned something kept could not read, every candidate reads
as not held, so the rule refuses nothing. A proposer's absence is never a
refusal.

**This amends the second clause of [0009](0009-repairs-are-not-enumerated.md).**
0009's binding Decision says "Ranking and the act/wait/suppress/escalate decision
stay deterministic". A model proposing the order breaks the first clause,
ranking. That is the subject of its own amendment of 0009, which this decision
does not make and which any proposer needs. A rule acting on a proposal's hold
breaks the second clause as well, because whether a candidate gets `act` or
`wait` then depends on a model's output. This decision amends 0009 by link and
does not edit it, as 0079 did. The amendment is that whether to open work is
still compiled, and the compiled rule may take a model's judgement as one of its
inputs when a person has ratified a rule that says so. 0009's reason still holds:
the queue stays explainable, because every hold names the rule that acted on it
and the proposal's argument is in the record.

**This would be the first place a model's output prevents work.**
[0086](0086-the-pass-rules-and-a-rejected-repair-is-reworked.md)'s ruling already
changes what kept does, but it decides only whether to try again. It can cause
more work and never prevents any. A hold that a rule acts on prevents work. That
difference is why it is reachable only through a ratified artifact and never
through configuration or code.

## Consequences

- `docs/document/model.md`'s loop sketch moves opening-rule evaluation out of the
  tick, to the opener, and shows the tick's evaluation as a forecast that writes
  nothing.
- A tick's recorded output does not change, so `cmd/kept/testdata/drift.json` and
  the other ledger-facing goldens do not move for this reason. Its printed output
  and `kept status` gain a forecast when the opener is built, and `--json` would
  then need to carry the forecast as a derived field. That is a presentation
  change to the machine surface, and it is decided when built.
- `ledger.Outcome` is documented as "what the tick did". Once the opener writes
  dispositions it is what the run did, and the comment changes with the code that
  makes it true.
- A forecast can disagree with what the opener later does, because the counts and
  ages it reads move in between. That is what a forecast is, and it is why it is
  never recorded.
- The ledger gains a record kind and the opener gains a reader for it. Neither is
  built. Its name and layout are decided with the opener, beside `threads/` in
  `docs/layout.md`.
- A rule acting on a hold needs nothing beyond the facts a proposer already
  produces, so turning holds on later is a ratification and not a code release.
  Turning them off again is a retirement.
- If proposals hold the wrong candidates, a rule that acts on them leaves
  promises unopened until a person retires the rule. Each such refusal names the
  rule, so the cause is visible in `kept why`.
- [0081](0081-the-adversarial-pass-argues-in-a-box-it-cannot-write.md)'s "nothing
  grades it, nothing gates on it" is about the adversarial pass, and it is
  unchanged. A proposal is a different session with a different ask and a closed
  answer. Only that answer is read, and its prose is graded by nobody.
- The proposal facts are opening-rule inputs and never reach a repair packet,
  because a packet collects only dispatch policies.
  `policy:the-pipeline-is-never-in-a-packet` and
  `policy:the-agents-report-is-never-an-input-to-the-verdict` are unaffected,
  because a hold changes whether a repair starts and never whether a promise is
  kept.

## Alternatives

- **A tick records the opening rules' verdict as its disposition.** This would
  answer "will this be fixed tonight?" from the ledger. It was rejected because it
  writes down a decision nobody made, and the forecast answers the same question
  without recording anything.
- **Only the opener evaluates, and a tick shows nothing.** This was rejected
  because a person at a hook or in CI then has no view of what the rules say
  until the opener runs, or until they run it with a dry-run flag.
- **A proposal only ever reorders, and nothing may act on a hold.** This was
  rejected because the case a hold exists for, several drifts with one cause, is
  then expressible only as prose a person reads after the opener has spent a slot
  on a symptom. Keeping holds behind a ratified rule keeps that power off until a
  person grants it.
