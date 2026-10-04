---
id: 0087-a-receipt-is-journaled-and-sealed-into-the-ledger
type: decision
title: 0087 — A receipt is journaled and sealed into the ledger
fields:
  statement: >-
    While a session runs, every entry its receipt would append, and every artifact
    such an entry names, is appended to a journal in the repository's Git common
    directory at the moment kept produces it, so a line kept has read survives a
    kill of kept. The ledger receives the receipt in sealed segments, each written
    once as a content-addressed artifact and named by one entry in the thread's
    receipt stream. The next run that writes the ledger seals whatever a dead
    run's journal still holds.
links:
  - { target: decision:0084-a-runtimes-output-is-read-whole-and-a-torn-item-is-kept, relation: amends }
  - { target: decision:0018-what-survives-a-crash, relation: extends }
  - { target: decision:0020-durable-when-irreproducible, relation: implements }
  - { target: decision:0016-the-ledger-lives-on-its-own-ref, relation: depends-on }
  - { target: decision:0052-repair-runtimes-are-closed-adapters-with-native-receipts, relation: depends-on }
  - { target: desire:a-thread-receipt-costs-the-ledger-about-what-it-holds, relation: implements }
---

# 0087 — A receipt is journaled and sealed into the ledger

## Context

[`desire:a-thread-receipt-costs-the-ledger-about-what-it-holds`](../desire/a-thread-receipt-costs-the-ledger-about-what-it-holds.md)
holds the measurement. On 2026-10-03 the ledger was 562 MiB packed and 134,450
commits, 461 MiB of it the receipts of eight sessions. Every receipt line is one
ledger commit, and each commit writes a new blob of the whole receipt so far, so a
receipt's cost grows with the square of its length. Fetching the ledger once took
seven minutes, and publishing it would send about 550 MiB to origin.

Shaping turned the want into a candidate promise with two clauses and a named test
for each, both in `internal/thread`. **Every ledger write made for a receipt
introduces at most 2 × the bytes it records + 64 KiB of new objects.** That is
measured as the uncompressed size of the objects each commit makes reachable,
which is what kept hands git and does not depend on packing. **A kill of kept
mid-session loses no line kept had read.** The person who owns the want chose
"no line" over a bounded loss.

Both tests fail against `main` at `5b2afbc6ba`:

- **The bound fails on write 93.** That write introduced 66,776 bytes to record a
  551-byte line, rewriting the whole receipt as soon as the receipt passed the
  allowance.
- **The kill test fails completely.** It killed the process group with the
  receipt already on the ref, after kept had read between 6,848 and 7,074 lines
  across three runs. The ledger held none of them.

The second failure comes from 0084, not from batching. 0084's spool reads a
runtime's output as fast as the runtime writes it, which it has to: a runtime
whose output nobody reads discards its tail at exit. But the spool holds what it
read **in memory**, up to 64 MiB, while the receipt writes it out at two to eleven
lines a second. *A killed session keeps what arrived* holds when the runtime is
killed and kept survives to drain the spool. It has never held when kept is
killed.

### The replay

One real receipt, thread `7ghe0nz6a9s5grsgdcx7x0d1xy` (4,665 lines, 3,806,141
bytes; 4,494 of the lines are native events), was replayed into a scratch
repository under each candidate layout. Raw bytes are git's own object sizes:
blob content, tree entries and commit text. Packed bytes are measured after `git
fast-import` and a default `git repack -adf`.

| layout | commits | raw ÷ receipt | clause | packed | repack |
|---|---|---|---|---|---|
| today: whole file, a commit per line | 4,665 | 2,576× | fails | 13.79 MiB | 12.7 s |
| batching alone: whole file, 100 lines | 47 | 26.6× | fails | 1.68 MiB | 0.3 s |
| segments of 64 KiB, a commit per line | 4,665 | 43.8× | fails | 2.48 MiB | 0.3 s |
| segments of 64 KiB, 100 lines a commit | 47 | 1.0× | passes | 0.65 MiB | 0.1 s |
| journal, sealed 64 KiB segments | 60 | 1.0× | passes | 0.58 MiB | 0.1 s |
| **journal, sealed 256 KiB segments** | **16** | **1.0×** | **passes** | **0.55 MiB** | **0.2 s** |
| journal, sealed 1 MiB segments | 4 | 1.0× | passes | 0.64 MiB | 0.1 s |

The method reproduces the real ledger. Today's row packs to 13.79 MiB, and the
same thread measured 11–13 MiB on `refs/kept/ledger`. The table excludes the 73
transcript fragments the thread wrote to `artifacts/`. Each is written once
whatever the layout, under a path that already fans out two levels by digest.

Two rows pass the clause. Only the journal also passes the kill test, because
segments with batching still hold a batch, and the spool's backlog, in memory.

## Decision

### A receipt is written to a journal while its session runs

Each thread has one journal, `<git-common-dir>/kept/journal/<thread>.jsonl`. It
lives in the common directory because that is where `internal/lock` already keeps
what every worktree of a repository must share. A run that crashed in one worktree
is recovered by the next writer in any worktree. The journal is never in a working
tree and never in a box.

`Ledger.AppendThread` appends the envelope it would have written to the ledger to
the journal instead, and returns once `write(2)` has returned. An artifact that
a receipt entry names, such as a transcript fragment, undecodable bytes or the end
of stderr, is journaled with its bytes, before the entry that names it. That keeps
the rule the receipt already follows: a receipt never points at an object that
was never written. Every
line the journal holds is the adapter's redacted entry, never the runtime's raw
bytes.

What this guarantees is survival of a kill of kept. A machine crash is a
different question, and the answer for it is no worse than a ref update's: both
are local writes whose sync to disk is `core.fsync`'s to decide, and that is
unmeasured.

### The spool's queue is retired, and 0084's guarantee is kept another way

0084 needed the spool because retaining a line cost a ledger commit, a few hundred
times slower than a runtime writes. Appending to a file is not, so the adapter
keeps pace with the runtime on its own. The supervisor hands the launch the
adapter's stream directly, and the moment kept accepts a line is the moment it is
journaled. 0084's guarantee stands: output is read as fast as it is written, to
the end of the stream. What changes is how. Back-pressure now reaches the runtime
only when the disk cannot keep up, and the bound on what kept holds is the disk,
not 64 MiB of memory.

This **amends** 0084's first section and supersedes nothing else in it.
Undecodable output, torn final items and boxes outliving their anchors are
unchanged.

### The ledger receives a receipt in sealed segments

kept seals a journal when it holds at least 256 KiB that have not been sealed,
and again once the session's last lifecycle entry is journaled. A seal does three
things:

1. It puts every artifact the span names, through `PutArtifact` as today.
2. It puts the span as a `thread.segment` artifact. The segment is written once
   under its content address, which fans out like every other artifact.
3. It appends one `thread_segment` entry to `threads/<thread>.jsonl`, naming the
   segment's address and the first and last writer and sequence it holds.

The receipt stream becomes an index of segments. It keeps its path and its
envelope, so a receipt written before this decision reads exactly as it did, and
nothing on disk is rewritten.

`ReadReceipt` expands each segment entry in place and folds the result by event
order, as it already does. It **drops an entry it has already seen**, identified
by its writer and sequence number. So sealing a span twice is harmless, and the
recovery below needs no exclusion to be correct.

**Segments are sealed at 256 KiB** because the replay measured the smallest pack
at that size. A smaller segment does not reduce the per-write bound in any way that
matters, and a larger one stops shrinking the pack. The index grows by one entry of
about 300 bytes a seal, so it stays under the 64 KiB allowance until a receipt
passes about 55 MiB. The largest receipt in this ledger is 34 MB.

### The next writer seals what a dead run left

A run holds an `flock` on its journal for as long as its session runs, and
`flock` releases when the process dies. Before its first ledger write, a run
seals every journal in the directory whose lock it can take, then removes that
journal while still holding the lock. That is the point
`TestKeptKilledMidSessionLosesNoLineItHadRead` reads at: the next run's first
record.

The lock decides only whether a writer is alive. It is not what keeps the receipt
correct: two runs that both seal the same journal write the same segment address
and one duplicate index entry, which the read drops. The one thing the lock guards
is removal. A journal is removed only by the process holding its lock, so a live
writer's journal is never unlinked under it.

## Alternatives

**Batch lines in memory.** This is 0084's own deferred sentence, *batching stays
open if a receipt's size or latency forces it*. On a whole-file layout it fails
the bound: 26.6× raw, with the curve still quadratic at a smaller constant. On
segments it passes the bound and fails the kill test, because every batch, and the
spool's backlog in front of it, is in memory. The person who owns the want
declined the trade.

**Segments with a commit per line.** This keeps today's durability and passes the
kill test only once the spool is gone. It fails the bound at 43.8× raw, because
each write still rewrites the open segment and every tree on its path. In raw
objects, one commit per line costs several times its payload in any layout.

**Journal the runtime's raw bytes at the spool.** This is the smallest change: the
spool would append what it reads to a file instead of to memory. But the journal
would then hold output the adapter has not redacted, so a credential the runtime
echoed would sit on disk outside the ledger. Recovery would also need the adapter,
its state and its secret to turn bytes into entries, in a process that did not
start the session. Journaling entries after the adapter avoids both.

**Number the segment files, `threads/<id>/000001.jsonl`.** Reading them needs a
listing, which `Store` does not have and 0016 decided it should not have, or
probing until the first missing name. Probing turns two writers that seal one
span into two different files under one name, and `Put` replaces a file. Content
addressing plus an index entry needs neither.

**A `Store` operation that commits several files at once.** A seal would then be
one commit instead of one per artifact plus two. The bound does not need it, and
the measured cost of the extra commits is small. It would also widen the seam 0016
kept to three operations. It stays available if commit count ever matters.

**Seal on a turn boundary rather than a size.** Turns range from a few events to
thousands, so a turn-sealed segment does not bound a write. Size does. The end of
the session is the one session event that does force a seal, so that a finished
receipt is whole in the ledger.

**Move receipts out of the ledger.** The desire rules this out. A receipt is
irreproducible, and 0020 puts what is irreproducible in the durable record.

## Consequences

- **A session costs the ledger about what it holds.** On the replayed thread the
  receipt went from 13.79 MiB packed and 4,665 commits to 0.55 MiB and 16. That is
  0.15 × the receipt's own size, because a segment is written whole once and
  compresses.
- **Both tests pass**, and the promise the desire carries can be ratified, with
  each clause having been seen to fail first.
- **A receipt is not in the ledger until it is sealed.** Nothing reads a receipt
  while its session runs. The episode's rework gate, `sessionDiagnosis`, the
  adversarial pass, the assessor, the land and `kept review` all read after the
  session returns, and the end of a session always seals. A person who runs `kept
  review` from another terminal during a repair sees what was last sealed, up to
  256 KiB behind.
- **There is a second local place, and it empties itself.** A journal outlives
  only a run that died, and the next writer takes it. `kept doctor` should report
  a journal nobody can lock and nobody is running. That is a liveness question
  0050 admits, and it is not built here.
- **`ReadReceipt` learns to drop an entry it has already seen.** It is the read
  side of a recovery that needs no exclusion, and it needs its own test that seals
  a span twice.
- **History is unchanged.** The 461 MiB already written stays, and only future
  sessions cost less. Publishing the ledger still sends what it holds today.
- **The kill guarantee covers kept, not the machine.** That stays an open
  question about `core.fsync`, and it is the same question for a ref update.
- **The adapter's pace is now a claim to measure.** The spool existed because a
  ledger append is slow. The implementation has to show that an adapter
  journaling every entry keeps up with a runtime streaming hundreds of lines a
  second, using the captured session the replay used. Otherwise the tail 0084
  saved is lost again at exit.
