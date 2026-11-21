---
id: 0094-a-rework-answers-every-finding-and-a-finding-names-what-was-lost
type: decision
title: 0094 — A rework answers every finding, and a finding names what was lost
fields:
  statement: >-
    The rework ask names both halves of what sent the work back, the derived
    checks and the pass's case, and says the work answers both. The repair ask
    says how every repair is judged: what the promise measures is compared before
    and after, and reaching the promise by measuring less is not a repair.
    `property-exercised` treats a list the predicate reads only through
    `"literal" in` as read for those literals alone. When the measurement fell,
    the finding also names every other member that left it, such as a test the
    predicate never named. A name explains a fall and never makes one: a member
    renamed, or retired beside more added, is not a finding.
links:
  - { target: decision:0078-the-derived-checklist-is-closed-and-the-argument-is-not, relation: amends }
  - { target: decision:0086-the-pass-rules-and-a-rejected-repair-is-reworked, relation: amends }
  - { target: decision:0058-the-ask-is-declared-and-travels-beside-the-packet, relation: extends }
  - { target: decision:0009-repairs-are-not-enumerated, relation: depends-on }
  - { target: decision:0091-a-test-a-predicate-names-is-part-of-its-instrument, relation: extends }
---

# 0094 — A rework answers every finding, and a finding names what was lost

## Context

Thread `2ntpka5qh7ebj0sg6fjvbn5fmw` has three attempts at
`promise:a-receipt-costs-the-ledger-about-what-it-holds`. Every attempt retired
`internal/thread/spool.go` and deleted `spool_test.go` with it, including
`TestARuntimeThatDiscardsUnreadOutputAtExitLosesNothing`. That test runs a real
child that discards unread output at exit, and it is the only real-runtime
instrument of 0084's guarantee, which 0087 says still stands. The hand-built fix
in PR 5 moved that test into `drain_test.go` and kept it.

`property-exercised` fired on all three attempts, and the third attempt
escalated on the repeat. Two things about the way it fired explain why three
attempts did not answer it.

**The finding did not say what was lost.** It read:

```
tests.packages[package=…/internal/thread].tests_run 426 → 421
tests.passed 423 → 421
internal/thread/spool_test.go was deleted, not added
```

The test cases are in the measurement, as `obs.tests.cases`, and the predicate
reads that list: `"TestKeptKilled…" in obs.tests.cases`. 0078's rule compares
only the dimensions a predicate does not read, so the whole list was taken as
read. It was not. The predicate has an opinion about two members of a list of
426, and the other 424 can leave without the wording noticing. That is exactly
where a substitution lives. A count that fell by five gives the next attempt and
the person nothing to act on, while a test name does.

**The rework ask pointed only at the argument.** It ended *stop when the work
answers that case and the promise holds*, and *that case* was the reviewer's.
The third attempt did exactly the two things the second pass called enough,
each with a regression test it watched fail. It left `internal/thread`
untouched, although its packet carried the `property-exercised` finding from
both earlier attempts and both earlier arguments named the deleted test. The
ask had told it what to stop on, and the derived finding was not part of that.

The first attempt had a third gap. It was never told that its work would be
compared before and after. The repair ask said to stop when the promise holds,
and a session that retires a subsystem deletes that subsystem's tests unless
something says a smaller measurement is not accepted.

## Decision

### The asks say how the work is judged

**The rework ask names both halves.** It now reads: *read what the derived checks
found and the case made against the earlier work … and stop when the work answers
both and the promise holds. A derived finding is as much a reason the earlier work
was not accepted as anything the reviewer argued, and the checks are worked again
after you exit.*

**The repair ask says what every repair is judged by.** It now reads: *what the
promise measures is compared with what it measured before, and a reviewer argues
against the change. Reaching the promise by measuring less is not a repair,
however the promise then reads: a property that was tested before and is not
tested after, or a check made weaker.*

Both texts are still byte-identical for every thread, and both still name no
promise, no file and no test. A test holds them to that. They do not say how to
repair anything (0009). They say how a repair is judged, and that is true of
every repair: `property-exercised` and `instrument-changed` run on all of them.
A judgment applied to every repair is not the pipeline that
`policy:the-pipeline-is-never-in-a-packet` keeps out. It names no budget, no
attempts left and no disposition.

### A finding names what was lost

**A list read only through `"literal" in` is read for those literals and no
others.** `predicate.Expr.Members` returns each such field and its literals. A
field read in any other way as well, such as counted, iterated or compared, is
left out, because then the predicate has an opinion about the whole list.

**A member nobody named that left the measurement is reported beside a fall, and
only beside one.** `weakened` keeps these lines apart from its others. It reports
them only when its existing rule reports something that fell. So the names
explain a smaller measurement and never create one. A test renamed is not a
finding, and neither is a test retired beside two added. That is 0078's
*less, never different*, applied to members as it already applied to numbers.
Run against the real before and after measurements of the thread above, the
finding now begins:

```
tests.cases[TestARuntimeThatDiscardsUnreadOutputAtExitLosesNothing] is no longer measured anywhere
tests.cases[TestExitingRuntimeHelper] is no longer measured anywhere
tests.cases[TestTheSpoolAcceptsOutputAsFastAsItArrives] is no longer measured anywhere
…
```

The check still fires in exactly the cases it fired in before. A member the
predicate does name is still the predicate's to judge and is never reported here.

## Consequences

**The rework packet and the brief carry names.** Both already carry every derived
finding's lines verbatim (0086), so nothing about their shape changes.

**PR 5's tree is still clear.** It retires the spool's own tests and adds the
journal's. Its `tests_run` rose, so nothing fell and no name is reported. A
repair that matches it is judged the same way.

**The asks have no confined live proof beyond the runs reported beside this
decision.** This is 0086's caveat again. How reliably a live session ports a test
instead of deleting it is measured by running one, not by the wording.

**A predicate author now has a reason to write a membership clause they mean.**
The finding that names the deleted test is the one the predicate would refuse if
it named that test too. That is the throughline: a finding that repeats is a
predicate nobody has written. The amendment proposed for this promise does exactly
that, and it is a person's to ratify.

## Alternatives

**Report every member of a membership-only list that left, fall or no fall.** This
was rejected because it fires on PR 5. Its spool tests go and more journal tests
arrive. A check that fires on the hand-built fix would make the fix unreachable by
repair, and would be switched off.

**Give `go/tests` a field per test, or a `skips` list.** That is a gatherer
change, and it binds through `Declares`. It is the better answer for *which test
skipped*, and it is proposed for this promise rather than made here.

**Put the deleted tests in the rework packet as a list of protected tests.** This
was rejected because it would be a repair instruction (0009). The packet already
carries the finding, and the finding now names them.
