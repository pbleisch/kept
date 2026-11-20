---
id: 0091-a-test-a-predicate-names-is-part-of-its-instrument
type: decision
title: 0091 — A test a predicate names is part of its instrument, and a named test does not depend on where a fix keeps its data
fields:
  statement: >-
    When a promise gathers go/tests and its predicate names a test by a string
    literal, the files that declared that test at the anchor's parent are part of
    the promise's instrument. `instrument-changed` fires when a repair changes or
    removes code those files held. It ignores comments, layout and declarations
    the repair added. A test a promise names asserts its property through the
    readers every implementation must satisfy, and never through where one
    implementation keeps its bytes. Otherwise the only fix that could satisfy the
    test would be one that edits the test.
links:
  - { target: decision:0078-the-derived-checklist-is-closed-and-the-argument-is-not, relation: amends }
  - { target: decision:0086-the-pass-rules-and-a-rejected-repair-is-reworked, relation: depends-on }
  - { target: decision:0087-a-receipt-is-journaled-and-sealed-into-the-ledger, relation: depends-on }
  - { target: decision:0006-no-probe-assess-the-thread, relation: depends-on }
  - { target: decision:0090-running-out-of-capacity-is-not-a-result-and-a-regression-is-not-anchored, relation: depends-on }
  - { target: policy:the-agents-report-is-never-an-input-to-the-verdict, relation: implements }
---

# 0091 — A test a predicate names is part of its instrument, and a named test does not depend on where a fix keeps its data

## Context

Thread `7v2p52085pvwtdepzzhz34f3ze` is in [0090](0090-running-out-of-capacity-is-not-a-result-and-a-regression-is-not-anchored.md).
Its attempt 2 anchored `0f983db`. That commit re-derived `kept` for
`promise:a-receipt-costs-the-ledger-about-what-it-holds`, and every derived check
about the act was clear. The promise names two tests:

```
"TestKeptKilledMidSessionLosesNoLineItHadRead" in obs.tests.cases
and "TestEachReceiptWriteIsBoundedByTheLinesItRecords" in obs.tests.cases
```

Both tests live in `internal/thread`, inside the promise's repair bound. They have
to, because they need the package's internals and a glob cannot leave one file out.
The promise says so: *a change to either one is therefore part of a repair's diff,
and it is the first thing a review of that diff should read.* `0f983db` changed
one of them. It replaced the bound test's vacuity guard, which reads *the replay
left more bytes than the allowance*, with one that an artifact write always
satisfies. It also raised the fixture from 80 KiB to 320 KiB. Only the adversarial
pass caught it, by running the old guard against the new tree and watching it
fire.

`instrument-changed` could not have caught it. The check knew two instruments: the
promise's own artifact file, and the program a `command` gather runs. A `go/tests`
gather's instrument is the test the predicate names, and the check did not look
for one.

The second finding is the one that matters more. **The guard as written could not
be satisfied by the design the promise cites.** It measured the receipt by reading
the one file at the thread's path. Decision 0087's design, which the promise body
names as *what already decides how*, keeps a receipt in sealed segments and leaves
that file as a small index. So every fix that follows 0087 has to edit the guard.
The hand-built fix `173c60a` (PR 5) edited it in a principled way: it counted the
segments too. The repair edited it the other way. Neither the predicate nor the
checks could tell the two apart, because both were edits to the instrument that
the instrument itself forced.

So a repair could not have kept this promise without editing its own judge. That
is the gap in kept's half: a check that could not see the judge being edited. The
other half is the promise's: a test that forced the edit.

## Decision

### The named tests are part of the instrument

`instrument-changed` reads one more instrument. A promise **gathers `go/tests`**,
and its `hold_when` holds a **string literal** naming a test. Any `_test.go` file
the anchor's diff touched, and that **declared that test at the anchor's parent**,
is the instrument. A rename's base is read from the path it came from.

The check reads the file on both sides of the change as Go. It compares every
top-level declaration the file held at the base: functions, methods, constants,
variables and types. Each is compared as printed code, with doc comments, inline
comments and blank lines removed. **A declaration that is missing or different at
the anchor fires the check**, and the finding names it. A file that was deleted,
or left unparseable, fires too.

Three things do not fire:

- **A declaration the repair added**, because nothing measured with it before.
- **A comment the repair corrected**, because prose is not code, and a repair
  that retires a mechanism may find the instrument's prose describing it.
- **A named test that did not exist at the base.** A promise may name a test
  nobody has written yet, and writing what the promise asked for is not moving
  the standard.

Helpers count because a guard can be weakened in a helper as easily as in the
test body. The fixture's size counts because enlarging it is how the attempt-2
repair made its weakened guard pass.

`predicate.Expr.Strings` is the reader. It returns every string literal in the
tree, for the reason `Roots` reads the tree rather than the text. The check is
still one of 0078's closed five. What changed is what *instrument* covers, which
0078 defined as *the gatherer, the predicate, or the promise itself*. For a test
gatherer that names its tests, the named tests are the gatherer's configuration.

Under 0086 the check is about the act, so its first firing reworks and its second
escalates. A repair that weakens a named test is sent back once, and then handed
to a person. Strengthening a named test was always a person's work. It is a
predicate nobody has written, which is 0086's reading of a repeating finding.

### A named test asks through the readers every implementation satisfies

The bound test's guard is rewritten by hand, in this change, to be independent of
where the receipt is kept. It asks two questions:

1. Does the receipt, **read back through `ReadReceipt`**, hold more bytes of
   runtime output than the allowance alone admits? `ReadReceipt` is the one reader
   every implementation has to satisfy, and the kill test already depends on it.
2. Did **the sessions ref's own writes** record more than the allowance? Without
   this, an implementation that kept the lines somewhere other than the ref would
   make no write for the bound to judge, and would pass with nothing measured.

Neither question names a file, a segment or an index. The bound assertion itself
is unchanged, and so is every assertion in the kill test. The kill test's prose no
longer says *spool*. That word named the mechanism in place at the time.

This was checked against both trees it has to judge:

- **On this branch**, which still has the old layout, the bound test still fails
  at write 92: 66,715 bytes introduced to record 551. The kill test still fails
  with 7,074 lines read and none on the ref. The promise still reads not-kept.
- **On PR 5's tree `2a2fc51`**, with this guard in place of its own, both tests
  pass unchanged.

So no fix that follows 0087 needs to edit either named test. Over the real anchors,
the extended check fires on `0f983db` and on `cc1de97`. It names
`receiptReplayBytes` and the bound test. It also fires on `2a2fc51` under the old
guard, which is the point: under the old guard, even the correct fix moved the
instrument.

## Consequences

**A legitimate fix that touches a named test's file now reads as a finding.** That
is the intended cost. The way out is to write the test so that no fix needs to
touch it. This decision did that for the one promise where the problem was
measured, and the other two promises that name tests are unchanged:
`doctor-reports-a-box-left-behind` and
`each-clause-of-a-predicate-answers-on-its-own`.

**The check reads two blobs per touched test file, and only for a promise that
gathers `go/tests`.** It reads them from the anchor's commits, which are durable,
so the check still measures nothing. A file it cannot read on both sides makes the
check unread, and it says which file.

**What this does not close.** The receipt promise's measured gaps run past what one
check can see, and each one stays where it belongs.

- **The promise measures `./internal/thread` and its repair bound includes
  `internal/ledger/**`.** Both repair anchors broke six `internal/ledger` tests
  inside that bound, and `0f983db` still read `kept`. The proposed amendment is for
  the captain to ratify, and it is not made here, because changing the gather
  changes the hash and drops the promise's standing. It would gather
  `packages: ["./internal/thread", "./internal/ledger"]` and hold
  `count(obs.tests.packages, p => p.ok) == 2`. The `internal/ledger` hook tests
  are now independent of a parent's git configuration, so measuring that package
  under a supervisor that sets one does not read `not-kept` for a reason the
  repair has nothing to do with.
- **A collateral break inside a promise that is already not-kept is invisible to
  a land's gate.** 0037's gate compares verdicts. `core-packages-are-tested` reads
  `not-kept` on `main` because this promise's two named tests fail, so a repair
  that also broke `internal/repair`'s
  `TestARepairFilesItsSessionOnTheSessionsRefAndTheLedgerCarriesNone` would be
  `not-kept → not-kept` at the gate: no break. Comparing what the measurement holds,
  such as a `go/tests` failure set, rather than only the verdict would see it.
  That is a change to 0037, and nothing here forces it yet.
- **The repair bound is not where the hand-built fix reaches.** `173c60a` also
  touched `internal/lock` and `AGENTS.md`. The `internal/lock` change was a
  convenience, since attempt 2 took a per-thread lock through the existing API. The
  `AGENTS.md` correction is project memory a repair is not allowed to write. A land
  of a repair that retires the spool leaves that prose wrong until a person fixes
  it.
- **The predicate's skip allowance is calibrated on a file a fix may retire.**
  `skipped <= 1` admits `TestExitingRuntimeHelper`, which lives in the spool's
  test file. The first repair run on this branch (thread
  `2ntpka5qh7ebj0sg6fjvbn5fmw`) deleted that file. That freed the slot, so a
  named test could skip and the promise would still read `kept`, and this check
  stayed clear, because the repair never touched a named test's file.
  `property-exercised` fired on the lost tests, and the adversarial pass named
  the skip allowance. `go/tests` reports how many cases skipped but not which
  ones. So no predicate in today's vocabulary refuses a named test skipping
  without depending on that helper. A list of the skipped cases, and a clause
  over it for each named test, is the proposal. Changing the gatherer is not
  part of this decision, and the promise's wording belongs to the captain.
- **The session deadline cost an attempt.** Attempt 1 was killed at 60 minutes
  without compiling. Its tree was anchored, argued over, and continued by attempt
  2, so continuation worked. The cost was one attempt out of three, plus a pass
  over a tree that did not build.
- **0087's append-only index grows by one entry per seal.** The adversarial pass
  measured the per-write bound breaking at about 31 MiB of receipt. The episode's
  own receipt was 36 MB. A fix that follows 0087 to the letter therefore shares
  that limit. Whether the bound is restated or the index is bounded is 0087's
  question, and it needs a fixture that seals many times.

## Alternatives

**Make the named test files unwritable in the box.** Rejected. A filesystem grant
is a path, the file sits inside a grant the repair needs, and denying it would add
a confinement rule for an assessment question. It would also refuse a repair that
had to write the named test, which is the one case that must work.

**Fire on any edit to a file that declares a named test.** Rejected. Correcting
prose a repair made stale, and adding a test beside a named one, are both
legitimate. Firing on them would teach the next attempt to leave stale prose in
place.

**Compare only the named test function.** Rejected. The fixture constant and the
helpers are where a weakened guard hides. The attempt-2 repair changed the
constant.

**Leave the guard tied to the stream file and accept that a fix edits it.**
Rejected. An instrument that every correct fix has to edit cannot tell a
correction from a substitution. That leaves the decision to the pass, a reading
that decides nothing on its own.
