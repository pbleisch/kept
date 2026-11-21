---
id: 0096-a-pass-may-rule-the-fix-out-of-reach-and-the-episode-stops
type: decision
title: 0096 — A pass may rule the fix out of the repair's reach, and the episode stops there
fields:
  statement: >-
    The adversarial pass has a third ruling, `ruling: out-of-reach: <reason>`,
    for a case no attempt at the repair can answer: what would answer it lies
    outside the promise's repair scope, or is a decision only a person can make.
    It is read from the same last line as accept and reject and carries its
    reason on that line, verbatim. The first time a pass makes it, the episode
    escalates to a person, whatever else the attempt's reading says and however
    much of the attempts budget is left. The escalation records the pass's
    reason, `kept repair` prints it, and a person's `--rework` still continues
    the thread.
links:
  - { target: decision:0086-the-pass-rules-and-a-rejected-repair-is-reworked, relation: amends }
  - { target: decision:0081-the-adversarial-pass-argues-in-a-box-it-cannot-write, relation: extends }
  - { target: decision:0079-a-repair-is-an-episode-that-reworks-itself, relation: depends-on }
  - { target: policy:the-agents-report-is-never-an-input-to-the-verdict, relation: implements }
---

# 0096 — A pass may rule the fix out of the repair's reach, and the episode stops there

## Context

[0086](0086-the-pass-rules-and-a-rejected-repair-is-reworked.md) gave the pass two
rulings. Reject means another attempt should answer the case, and accept means
the pass could not make one. A pass that finds the fix cannot be made inside the
repair's bound has no way to say so. It can only reject, and a reject reworks
until the attempts budget is spent.

Thread `0ax04xrq56sadd02k91z1xface` repaired
`promise:a-receipt-costs-the-ledger-about-what-it-holds`. It ran four opus/xhigh
repair sessions and six passes, $101.13 in all. The pass over attempt 2 first
argued that the flat `threads/` tree grows without bound. Attempts 3 and 4 and
the passes over them, $48.91, worked around that inside `repair.scope`. The
pass over attempt 4 then said it outright: *no repair confined to this promise's
own `repair.scope` can make the statement true*. Relieving the ceiling means
sharding `threads/` or moving the stream. Either change breaks a literal path in
`internal/repair/sessions_test.go:51`, which is outside the scope
`["internal/thread/**", "internal/ledger/**"]`, and the desire behind the promise
already says that layout change needs a decision. The pass ruled reject, because
reject was the only token that said the work was not acceptable. kept read it as
"try again", and only the budget stopped it.

The pass was right, and it was right in a sentence the episode could not act on.
0086 refuses to read the argument's prose, and that refusal is correct: reading
tone is how a pass's opinion becomes a grade. So the pass needs a token for this
case.

## Decision

### A third ruling, with its reason on the line

The pass may end with `ruling: out-of-reach: ` followed by one sentence. **Out of
reach** means the case it made is one no attempt at this repair can answer,
because what would answer it lies outside the repair scope the promise binds, or
because it needs a decision only a person can make. The sentence says which.

`thread.ReadRuling` reads it from the same last line, under the same trimming,
and with the same strictness as the other two. The line must begin exactly
`ruling: out-of-reach: `, and the reason must not be empty. A bare
`ruling: out-of-reach`, a missing space, or an empty reason is **no ruling**, and
0086 already escalates that. A reason after `ruling: reject` makes the line no
ruling either. Nothing above the last line is read, so a pass that argues
out-of-reach in prose and then rules reject has ruled reject.

**The reason is the one part of a ruling that is words, and it is carried
verbatim and never interpreted.** `ReadRuling`'s second value already says why
there is no ruling. For this ruling it carries the reason instead, so
`RulingWhy` reaches every surface that already shows a ruling (the event, the
review, `--json`) with no new field. The ruling is still derived on read, as 0086
requires. The receipt records it because the receipt files the argument verbatim,
and the ruling line is the argument's last line. No receipt entry, argument field
or packet field is added, and `ArgumentRecord` keeps its five fields.

The assessor's ask names the third form beside the other two and says what it
does: the repair is handed to a person and not attempted again. The ask still
names no promise and no path, and it is still byte-identical for every brief.

### The first out-of-reach ruling escalates

This amends 0086's table with a row in the escalating block:

| reading | action |
|---|---|
| the pass ruled the fix out of reach | **escalate** |

Escalation still wins, so the row applies whatever else the reading says: a
promise that holds, every check about the act clear, a first-time finding that
would rework, or attempts left in the budget. An attempt confined to the same
scope meets the same wall, and spending one to find that out is the cost this row
removes. "The first time" is literal. Unlike a reject, this ruling is not
counted toward a second occurrence. The pass said the condition holds of every
attempt, and kept takes the claim at its word for the one decision a ruling
makes: whether to try again.

The escalation's reason is `the adversarial pass ruled the fix out of this
repair's reach: <the pass's sentence>`. It is written into the `escalate`
disposition's detail and printed in the run's phase line and at its end. For
this escalation, `kept repair`'s next steps name the move that answers it: widen
the promise's repair scope, or make the decision the pass named, ratify that, and
open a new episode.

### What it does not do

The ruling is still never an input to the verdict, and it creates no standing.
It refuses no land: the anchored work can still be landed by a person who reads
the case and disagrees with it. It does not stop `kept repair --rework`. A person
who disagrees can ask for another attempt, which runs, and its packet carries the
earlier attempt's argument and its ruling like any other. And kept does not check
the reason. Whether the cited path really is outside `repair.scope` is left to the
person reading it, because checking it would mean parsing the argument.

## Consequences

**One pass can end an episode that the budget would have run to its end.** On
thread `0ax04xrq…`, a pass over attempt 2 that ruled out of reach on the ceiling
it first argued would have stopped the episode before attempts 3 and 4. That is
only true if the pass rules out of reach the first time it sees the wall. Whether a live pass does so reliably has not been
measured. The ask now offers the token. Before this, the pass had no way to say
it.

**A pass can now end an episode wrongly in a new way.** A pass that rules out of
reach when an in-scope fix existed escalates an episode that a rework would have
finished. That is the direction 0086 chose for every failure of the assessor: a
person sees it sooner, and nothing lands unreviewed. The reason on the line is
what lets that person disagree quickly.

**The packet's ruling field gains a value.** `EarlierAttempt.Ruling` may now hold
`out-of-reach`, and `thread.Ruling.Known` admits it. `PacketVersion` does not
move. The field is the same field with the same meaning, the one line read out of
the argument beside it. A build older than this one reads an out-of-reach line as
no ruling, so it escalates on it and leaves the field empty when it carries the
attempt forward. It never meets the value it does not know, so a version bump
would buy nothing.

**Like the ruling line it extends, this has no confined live proof.** The suite's
stand-in pass rules out of reach under the real provider and spends nothing. The
first live out-of-reach ruling is the proof.

## Alternatives

**Detect out-of-reach in the argument's prose.** Matching "outside the repair
scope" or "needs a decision" is exactly the reading of tone 0086 refuses. It would
also fire on a pass that names the wall in order to say an attempt got around it.

**Count it like a reject, and escalate on the second.** This was the cost being
removed. The second pass would arrive only after a whole attempt had been spent
against a wall the first pass had already named.

**Have kept check the claim against `repair.scope`.** The reason names a path in
prose, and pulling a path out of prose is parsing the argument. The "needs a
decision only a person can make" half has no path to check at all.

**A separate token on the line before the ruling, such as `reach: <reason>`.** A
second machine-read line widens what kept reads from one line to two, and a pass
that wrote one without the other would need a third reading. One line that
carries both the token and its reason keeps 0086's rule that only the last line
is read.

**Store the reason in a receipt entry of its own.** The reason is part of words
the receipt already holds verbatim, and a stored copy is the cache 0086 refuses
for the ruling itself.
