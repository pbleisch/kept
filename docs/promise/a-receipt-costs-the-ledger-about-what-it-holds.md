---
id: a-receipt-costs-the-ledger-about-what-it-holds
type: promise
title: A receipt costs the ledger about what it holds
fields:
  statement: >-
    kept killed at any point while a runtime streams loses no line it had read from that
    runtime, and each ledger write made for a thread receipt introduces new git objects
    totaling at most twice the bytes of the lines it records plus 64 KiB — shown by two
    named tests in an internal/thread suite that passes whole.
  gather:
    tests:
      gatherer: go/tests
      scope: ["internal/thread/**", "internal/ledger/**", "go.mod", "go.sum"]
      packages: "./internal/thread"
  hold_when: >
    obs.tests.ok
    and count(obs.tests.packages, p => p.ok) == 1
    and obs.tests.skipped <= 1
    and "TestKeptKilledMidSessionLosesNoLineItHadRead" in obs.tests.cases
    and "TestEachReceiptWriteIsBoundedByTheLinesItRecords" in obs.tests.cases
  trigger: cadence
  max_age: 7d
  repair:
    scope: ["internal/thread/**", "internal/ledger/**"]
links:
  - { target: desire:a-thread-receipt-costs-the-ledger-about-what-it-holds, relation: realizes }
  - { target: decision:0087-a-receipt-is-journaled-and-sealed-into-the-ledger, relation: cites }
  - { target: decision:0084-a-runtimes-output-is-read-whole-and-a-torn-item-is-kept, relation: cites }
  - { target: decision:0018-what-survives-a-crash, relation: cites }
---

A repair session's receipt is the one record kept cannot re-derive, so it is
kept whole. Kept whole has cost the ledger far more than the receipt holds. On
2026-10-03 the ledger was 562 MiB packed, 461 MiB of it the receipts of eight
sessions, because every receipt line was its own commit and rewrote the whole
receipt. And a receipt kept whole in the ledger was still not safe: 0084's spool
held what kept had read in memory, so a kill of kept lost the backlog. The
measurement, and how this statement was reached, are in
[[desire:a-thread-receipt-costs-the-ledger-about-what-it-holds]].

## What the two clauses measure

**The bound is per write, in what kept hands git.** For every commit the ledger
receives while a recorded session is replayed through the adapter's own stream,
`TestEachReceiptWriteIsBoundedByTheLinesItRecords` sums the uncompressed sizes of
the objects that commit makes reachable and its parent does not. It holds that sum
to twice what the changed files grew by, plus 64 KiB. It is per write so that a
layout whose writes grow with the receipt fails on any fixture large enough to
pass the allowance, and it counts uncompressed bytes because packing is git's
choice, not kept's. The test refuses a replay too small for the bound to fire.

**No line is lost to a kill of kept.** `TestKeptKilledMidSessionLosesNoLineItHadRead`
kills the kept process mid-session, with the receipt already on the ref. "Read"
means the moment kept accepted a line from the runtime. It then reads what
survived from the ledger after the next run's first write, never from the killed
run's account, the way `internal/land/crash_test.go` proves 0018. "No line", not a
bounded loss, was a person's choice.

## What the predicate guards against

A named test that skips is still in `obs.tests.cases`, and its package still
reads `ok`, so presence alone would let a skip read as kept. The suite has
exactly one test that always skips, `TestExitingRuntimeHelper`, which is a
helper the suite re-executes rather than a test. So `skipped <= 1` admits that
helper and refuses either named test skipping beside it. Both tests sit inside
this promise's repair bound, because a glob cannot exclude a file and they need
the package's internals. A change to either one is therefore part of a repair's
diff, and it is the first thing a review of that diff should read.

## What already decides how

[[decision:0087-a-receipt-is-journaled-and-sealed-into-the-ledger]] is the design
both clauses were measured against. While a session runs, a receipt is journaled in
the Git common directory. The ledger receives it in sealed, content-addressed
segments named from the receipt stream, and the next writer seals what a dead run
left. Its consequences name what else the change owes: a read that drops an entry
it has already seen, and an adapter that still keeps pace with a runtime once its
spool's queue is gone. The history already written is not rewritten.

Both tests failed against `main` at `5b2afbc6ba`. The bound failed on write 93,
which introduced 66,776 bytes to record a 551-byte line. The kill test failed with
about 7,000 lines read and none of them in the ledger.
