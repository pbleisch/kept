---
id: a-receipt-write-does-not-grow-with-the-threads-beside-it
type: promise
title: A receipt write does not grow with the threads beside it
fields:
  statement: >-
    In a refs/kept/sessions already holding a receipt for each of 10,000
    threads, every commit a write to one thread's receipt makes introduces tree
    objects totaling at most 16 KiB, and a receipt filed at an earlier location
    still reads whole — shown by two named tests in an internal/ledger suite
    that passes whole.
  gather:
    tests:
      gatherer: go/tests
      scope: ["internal/ledger/**", "go.mod", "go.sum"]
      packages: "./internal/ledger"
  hold_when: >
    obs.tests.ok
    and count(obs.tests.packages, p => p.ok) == 1
    and obs.tests.skipped == 0
    and "TestAReceiptWriteDoesNotGrowWithTheThreadsBesideIt" in obs.tests.cases
    and "TestAReceiptFiledBeforeTheLayoutMovedIsReadWhole" in obs.tests.cases
  trigger: cadence
  max_age: 7d
  repair:
    scope: ["internal/ledger/**"]
links:
  - { target: desire:a-thread-receipt-costs-the-ledger-about-what-it-holds, relation: realizes }
  - { target: decision:0097-receipts-on-the-sessions-ref-fan-out-by-a-hash-of-the-thread-id, relation: cites }
  - { target: decision:0089-session-records-live-on-their-own-ref, relation: cites }
  - { target: decision:0091-a-test-a-predicate-names-is-part-of-its-instrument, relation: cites }
  - { target: promise:a-receipt-costs-the-ledger-about-what-it-holds, relation: cites }
---

Every write to a thread's receipt is a commit on `refs/kept/sessions`. A commit
rewrites every tree on the path of the file it changed. While every receipt sits
in one `threads/` directory, every write rewrites that directory whole, about 60
bytes for each thread the ref has ever held. So the cost of recording one line
grows with every thread that came before it, and nothing prunes the directory.
[[promise:a-receipt-costs-the-ledger-about-what-it-holds]] bounds what a write
costs against the lines it records. This promise bounds the part of that cost
that comes from the threads beside it, which no change inside `internal/thread`
can reach.

[[decision:0097-receipts-on-the-sessions-ref-fan-out-by-a-hash-of-the-thread-id]]
decides how: a receipt is filed at `threads/<hh>/<id>.jsonl`, `<hh>` being the
first two hex digits of the SHA-256 of the id, and `locate` in
`internal/ledger/ref.go` makes that choice. It is the one place that already
decides where a record is filed. Receipts filed before the change stay where
they are, and are read where they are.

## What the two tests measure

**`TestAReceiptWriteDoesNotGrowWithTheThreadsBesideIt`** seeds the sessions ref
with one receipt for each of 10,000 minted threads, in one commit. It then
appends a line to one of them and opens one more. For every commit those two
writes made, it sums the uncompressed sizes of the tree objects the commit made
reachable and its parents did not, and holds each sum to 16 KiB. It counts trees
and nothing else, because trees are what the other threads move. The blob is the
receipt's own cost, which the sibling promise bounds.

**`TestAReceiptFiledBeforeTheLayoutMovedIsReadWhole`** writes one thread's
receipt the two ways kept has filed receipts before: below `ledger/threads/` on
`refs/kept/ledger`, and at `threads/<id>.jsonl` on `refs/kept/sessions`. It then
appends a line through the store. The receipt must read as all three lines in
order, and neither earlier file may be rewritten. This repository already holds
receipts at both earlier locations, so a fanout that dropped them would lose the
one record kept cannot re-derive.

On `main` at `a9d5b138d`, the first test fails: one append introduced 600,034
bytes of trees, 10,001 entries at 60 bytes each. The second passes, because
nothing has moved yet. A prototype of 0097's layout passed the first at 9,678
and 9,918 bytes. Without the earlier sessions path in its read, it failed the
second. So both tests have fired.

## What the predicate guards against

**The seed goes where the store files a receipt.** Each path comes from
`locate(ThreadPath(id))`, and the test reads three seeded receipts back through
`Read` before it measures anything. So the test names no layout, and it does not
have to be edited for the layout it asks for. A repair that decides the fanout
anywhere `locate` cannot see leaves the seed where the store does not read it.
The test then fails, saying so, and does not measure a tree the writer never
touches.

**Small trees are not enough.** A writer could get small trees by leaving the
other threads out of the commit. After the writes, the test checks three
things. The ref must still hold every seeded receipt. A seeded thread nothing
wrote to must read unchanged. Both measured receipts must read whole.

**Skips.** `internal/ledger` has no test that always skips, so `skipped == 0`
refuses either named test skipping.

**The instrument is one file**, `internal/ledger/receiptneighbours_test.go`,
helpers included. Under [[decision:0091-a-test-a-predicate-names-is-part-of-its-instrument]],
a repair that changes code in that file fires `instrument-changed`. The tests
the fanout does break are other ones. They are
`TestASessionRecordIsFiledOnTheSessionsRefAndNeverOnTheLedger` and
`TestARecordWrittenBeforeTheSessionsRefIsReadWhereItIs`, in
`internal/ledger/sessions_test.go`, and both spell the flat path. They are
inside the repair bound, and this promise names neither of them.

## What the bound is not

The numbers are in the statement so that the hash covers them. One level of 256
divides the term by 256; it does not remove it. Under this layout a write's
trees reach 16 KiB at about 38,000 threads. Holding the bound at a larger count
means changing the stated count and the layout together. That is a revision and
a re-ratification, not a repair.
