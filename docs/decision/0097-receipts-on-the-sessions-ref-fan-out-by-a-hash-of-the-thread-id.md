---
id: 0097-receipts-on-the-sessions-ref-fan-out-by-a-hash-of-the-thread-id
type: decision
title: 0097 — Receipts on the sessions ref fan out by a hash of the thread id, and a repair moves them there
fields:
  statement: >-
    On refs/kept/sessions a thread's receipt stream is filed at
    threads/<hh>/<id>.jsonl, where hh is the first two hex digits of the SHA-256
    of the thread id. The logical path stays threads/<id>.jsonl, and the
    store's locate decides the fanout, as it already decides the ref. A receipt
    filed earlier, below ledger/threads/ on the ledger or at
    threads/<id>.jsonl on the sessions ref, is read where it is and never
    moved, and it reads as one stream ahead of the lines filed after it. What a
    write may cost in trees is held by
    promise:a-receipt-write-does-not-grow-with-the-threads-beside-it.
links:
  - { target: decision:0089-session-records-live-on-their-own-ref, relation: amends }
  - { target: decision:0087-a-receipt-is-journaled-and-sealed-into-the-ledger, relation: depends-on }
  - { target: decision:0091-a-test-a-predicate-names-is-part-of-its-instrument, relation: depends-on }
  - { target: decision:0096-a-pass-may-rule-the-fix-out-of-reach-and-the-episode-stops, relation: depends-on }
---

# 0097 — Receipts on the sessions ref fan out by a hash of the thread id, and a repair moves them there

## Context

[0089](0089-session-records-live-on-their-own-ref.md) filed every thread's
receipt at `threads/<id>.jsonl` on `refs/kept/sessions`, one directory for every
thread there has ever been. A write to a receipt makes one commit. That commit
holds a new blob for the file it changed and a new tree for every directory on
the file's path. So every receipt write rewrites the `threads/` tree whole, about
60 bytes for each thread it holds, and nothing prunes it.

The pass over attempt 4 of thread `0ax04xrq56sadd02k91z1xface` found this.
[0096](0096-a-pass-may-rule-the-fix-out-of-reach-and-the-episode-stops.md)
records the episode. The pass said that
`promise:a-receipt-costs-the-ledger-about-what-it-holds`'s per-write bound,
*at most twice the bytes of the lines it records plus 64 KiB*, fails above about
800 threads, and that no repair inside that promise's `repair.scope` could
change that. The literal `"threads/" + id + ".jsonl"` at
`internal/repair/sessions_test.go:51` sits outside the scope, and
`desire:a-thread-receipt-costs-the-ledger-about-what-it-holds` says a change to
the layout of `threads/` needs a decision. This is that decision.

Measured on `main` at `a9d5b138d` by the test this change adds: in a sessions ref
holding 10,000 threads, appending one line to one receipt made a commit that
introduced **600,034 bytes of tree objects**. That is 10,001 entries at 60 bytes
each. This repository holds one thread on `refs/kept/sessions` today, so the
ceiling is real but far off.

## Decision

**A receipt is filed below a fanout directory.** The sessions ref files thread
`<id>`'s receipt at `threads/<hh>/<id>.jsonl`, where `<hh>` is the first two hex
digits of the SHA-256 of `<id>`. That is 256 directories, the same width as the
`ab/` level `artifacts/` already has. The file keeps the whole id as its name, so
`git ls-tree -r refs/kept/sessions | grep <id>` still finds a receipt.

**The fanout is the store's location, not the receipt's name.** `ThreadPath`
stays `threads/<id>.jsonl`. Every caller and the `Files` store keep naming a
receipt that way. `locate` decides where in the sessions ref's tree that name is
filed, as it already decides which ref. A change of layout is then one
function's decision, and no caller has to know it.

**What was filed earlier is read where it is.** 0089 read `ledger/threads/` on
the ledger and did not move it. The same rule covers the flat sessions path.
Reading `threads/<id>.jsonl` returns three parts, joined in this order:

1. the part below `ledger/threads/` on the ledger
2. the part at the flat path on the sessions ref
3. the part at the fanned-out path

A torn final line in an earlier part is ended, not joined to the next part.
Nothing writes an earlier location again.

**The bound lives in a promise, and a repair does the work.** The property is
`promise:a-receipt-write-does-not-grow-with-the-threads-beside-it`: in a sessions
ref holding a receipt for each of 10,000 threads, every commit a receipt write
makes introduces at most 16 KiB of tree objects, and a receipt filed at an
earlier location still reads whole. Its two named tests are in
`internal/ledger/receiptneighbours_test.go`. Its `repair.scope` is
`internal/ledger/**`, and a kept repair of that promise is what makes the change.

**Its tests seed through the store, not through a layout.** The test files each
seeded receipt at `locate(ThreadPath(id))`, which is wherever the store would
file it, and then reads it back through `Read` before it measures anything. So
the test survives the change it asks for. This is
[0091](0091-a-test-a-predicate-names-is-part-of-its-instrument.md)'s rule that a
named test asks through what every implementation must satisfy. A layout decided
anywhere other than `locate` leaves the seed where the store does not read it,
and the test says so instead of measuring a tree the writer never touches.

## Consequences

- **The term is divided, not removed.** One level of 256 still grows linearly
  with the thread count. A write rewrites the 256-entry fanout tree, about
  7.4 KiB, plus one leaf of N/256 entries. A prototype of this layout measured
  9,678 and 9,918 bytes of trees per write at 10,000 threads. The 16 KiB bound
  holds up to about 38,000 threads, and the receipt promise's 64 KiB allowance
  up to about 250,000. Past that, the answer is a second level, and it is a
  revision of this decision and of the promise's statement.
- **The bound decides the shape.** 16 KiB at 10,000 threads refuses a flat
  directory above about 270 threads. It also refuses one level of 32 (about
  19.7 KiB) and one level of 1,024 (about 30 KiB, the obvious width for a
  two-character base32 prefix). It admits one level of 256 and any two-level
  layout.
- **A read asks up to three places.** A thread opened before this change and
  reworked after it costs one more `ls-tree` per read. New threads have nothing
  at the earlier locations.
- **One literal outside the repair bound was rewritten by hand, ahead of the
  repair.** `TestARepairFilesItsSessionOnTheSessionsRefAndTheLedgerCarriesNone`
  in `internal/repair/sessions_test.go` now reads the receipt through the store
  by `ThreadPath`, instead of looking for the flat path in the tree. It passes on
  the flat layout and on the fanned-out one. Two tests inside `internal/ledger`
  spell the flat sessions path. They are
  `TestASessionRecordIsFiledOnTheSessionsRefAndNeverOnTheLedger` and
  `TestARecordWrittenBeforeTheSessionsRefIsReadWhereItIs`. They are inside the
  repair bound and the promise names neither, so the repair edits them.
- **`docs/layout.md` and the receipt line in `AGENTS.md` describe the flat
  path** until the repair lands. Neither is in the repair bound. Correcting
  them is part of reviewing the land.
- **`promise:a-receipt-costs-the-ledger-about-what-it-holds` is not amended.**
  Its statement still covers the term in principle. Once the fanout lands, that
  term stays far below its allowance at any count this repository will reach.
  Re-ratifying that promise would refuse `kept repair --rework` on its open
  thread (0086), and which of its fixes lands is still open.
- **A journaled receipt gets the fanout for free.** Under 0087's design, a
  sealed segment is an artifact, which `artifacts/` already fans out. The index
  line that names it is appended through `ThreadPath`, so `locate` files it
  below `threads/<hh>/` like any receipt line.

## Alternatives

- **Fan out by characters of the id.** Thread ids are minted base32, so two
  characters would give 1,024 directories. But a minted id is opaque
  ([`internal/ids`](../../internal/ids/ids.go)), and the first character of a
  128-bit id carries only 3 bits. So "the first two characters" gives 256
  directories that look like 1,024. `ThreadPath` also accepts ids that are not
  minted. A hash of the id is uniform for every id the path accepts and reads
  nothing into it.
- **Two levels now.** Two levels hold the tree term near 2 KiB to millions of
  threads. One level of 256 holds the bound at the stated count, matches
  `artifacts/`, and keeps the legacy read to three places rather than four.
  This is cheap to revisit, because a later layout is one more earlier location
  to read.
- **Move the stream out of `threads/`, for example one ref per thread.** That
  bounds the tree term completely, but it scatters what 0089 put on one ref to
  keep it off origin. Every refspec and push guard would then match a family of
  refs instead of one ref.
- **Widen the receipt promise's `repair.scope` by one file** (amendment D1).
  This would have let that promise's own repair shard the tree. But it
  re-ratifies that promise, which ends the open episode's rework. It also puts
  a layout property in a promise that measures `internal/thread`.
- **Qualify the receipt promise's statement with a thread count** (amendment
  D3). That changes what the promise says without relieving the term. It is
  still available later, and nothing here forecloses it.
