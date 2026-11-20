---
id: 0095-a-pass-is-not-spent-on-an-attempt-already-escalated
type: decision
title: 0095 — A pass is not spent on an attempt the derived checks already escalate, and a rework argues it first
fields:
  statement: >-
    After an attempt is anchored, kept works the derived checks before it starts
    the adversarial pass. A check about the act may have fired again, or may have
    been unreadable for a cause outside the episode. Either escalates the
    attempt whatever a ruling says, so kept starts no pass. The pass is recorded
    as deferred and adds no reason of its own to the escalation. If a person runs
    `kept repair --rework` on that thread, kept argues the deferred attempt
    before it opens the next one, without deciding again. The next packet then
    carries the case against it. A pass whose ruling can still change what
    follows, or whose argument the next attempt is owed, always runs.
links:
  - { target: decision:0086-the-pass-rules-and-a-rejected-repair-is-reworked, relation: amends }
  - { target: decision:0090-running-out-of-capacity-is-not-a-result-and-a-regression-is-not-anchored, relation: extends }
  - { target: decision:0081-the-adversarial-pass-argues-in-a-box-it-cannot-write, relation: depends-on }
  - { target: decision:0077-the-receipt-says-what-answered-and-what-it-consumed, relation: depends-on }
---

# 0095 — A pass is not spent on an attempt the derived checks already escalate, and a rework argues it first

## Context

0086's table checks escalation first. Its first two rows escalate on the derived
checks alone: a check about the act that fired again, and one that could not be
worked for a cause outside the episode. Whatever the pass rules, one of those
rows escalates the attempt. Yet the pass ran anyway, because it ran inside the
attempt, before anything read the table.

The receipt of thread `2ntpka5qh7ebj0sg6fjvbn5fmw` puts a price on that. The
thread holds three attempts across two runs, and these sessions:

| session | model | cost | ruling mattered? |
|---|---|---|---|
| attempt 1 repair | claude-sonnet-5 | $11.30 | |
| attempt 1 pass | claude-opus-5, xhigh | $6.02 | yes: a first `property-exercised` reworks either way, and the argument went to attempt 2 |
| attempt 2 repair | claude-sonnet-5 | $2.67 | |
| attempt 2 pass | claude-opus-5, xhigh | $5.34 | **no**, `property-exercised` fired again → escalate |
| attempt 3 repair | claude-sonnet-5 | $2.40 | |
| attempt 3 pass | claude-opus-5, xhigh | $4.78 | **no**, it fired a third time → escalate |

$10.12 of $32.51, 31%, went on two rulings that could not change what followed.
Each pass costs more than the rework session it argues about. A pass reads a brief
of up to 273 KB, which goes to cache at the one-hour write price, and then reads
the tree.

The arguments were not worthless. Attempt 2's pass found the recovery-ordering
bug that attempt 3 fixed. The question is when that argument is needed. An
escalation on a repeating check asks a person for *a predicate nobody has
written*. If the person writes it, a new episode opens, and the old thread's
arguments are not carried (0086). The argument is needed only if the person
instead says *try again*, with `kept repair --rework`.

## Decision

**The derived checks are worked before the pass, and the two escalating rows are
asked first.** After an attempt has anchored, kept reads the thread's attempts
from the receipt. This is the same `assess.Attempts` reading that 0086's table
decides from. It then asks the rows that escalate on the derived checks alone. If
either applies, kept starts no pass. The attempt's pass result is `deferred`, and
its detail names the escalation and the command that would argue the attempt. In
the table, `deferred` adds no row of its own, so the escalation reads exactly as
the repeated check made it.

**A pass over an attempt that may be reworked or may be ready always runs.** In an
attempt with every check clear, the ruling decides whether there is another
attempt. In one that will be reworked anyway, because a check fired for the first
time, the promise still does not hold, or the work regressed, the argument is half
of what the next attempt is told. Either way the pass runs as before. A reading
kept cannot work out before the pass is never a reason to skip one. A machine with no assessor configured is reported as it always
was, not as a deferral.

**`kept repair --rework` argues a deferred attempt first, and does not decide
again.** When the thread's last attempt has anchored work that no pass has argued
over at all, and an assessor is configured, the run argues that attempt before it
opens another. It then rebuilds the plan, so the next packet carries the argument
verbatim, as 0086 requires. It does not settle the attempt again. Settling would
read the same repeated check and escalate before the attempt the person asked for.
The person's `--rework` has already answered that escalation. A pass the account
cuts off here waits, as 0090's does. This is 0090's resume, extended from *a pass
the account cut off* to *a pass nobody has run yet*. An attempt from a time when no
assessor was configured is owed the same pass, and is argued the same way.

**Nothing new is recorded.** A deferral writes no receipt entry. *No brief session
in this attempt* is already what the receipt says, so it is derived on read like
every other reading. The disposition that escalates still names the thread, and
its detail still carries the escalating rows.

## Consequences

**On the measured thread this saves the two passes that decided nothing.** The
saving is $10.12 of $32.51. When an escalation is answered by a new predicate, as
the throughline intends, that pass is never paid for. When it is answered by
`--rework`, it is paid for once, at the start of that run.

**A person reading an escalation may read no argument about the last attempt.**
`kept review <thread>` shows that attempt's derived findings, and the earlier
attempts' arguments. The escalation is the repeated check, which `kept review`
prints. The argument that would have sat beside it is what the next `--rework`
produces first. A person who wants it without reworking has no verb for that today.
A `kept review --argue` would be one, and nothing here builds it.

**The receipt is read once more per attempt.** The derived reading now runs before
the pass as well as after it. On a receipt the size the unrepaired promise produces,
that is a measurable read. It is far cheaper than a session, and once 0087 lands
it is small.

**The pass on an attempt whose check fires for the first time still runs.** That
argument feeds the rework that follows, which is the whole reason the loop exists.

## Alternatives

**Drop the pass entirely when the checks escalate.** This was rejected because a
person's `--rework` would then open the next attempt with no case against the last
one. That is the rework 0086 exists to prevent.

**Skip every pass after the first in an episode, or run the pass only when the
checks are clear.** This was rejected because a check firing for the first time
reworks, and the pass's argument is half of what that rework is told. Thread
`2ntpka5qh7ebj0sg6fjvbn5fmw`'s attempt 2 would have lost the bug its pass found.

**Cap spend in dollars per episode.** 0079 chose to record money and never to
refuse on it, because a subscription's scarce resource is its usage window. The
window ceiling already refuses on that. This decision removes waste rather than
rationing it.

**Use a cheaper assessor.** That is the operator's choice in `assess:`. Deferral
removes sessions whose result could not be used, whatever the model.
