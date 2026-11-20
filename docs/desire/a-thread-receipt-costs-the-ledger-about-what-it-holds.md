---
id: a-thread-receipt-costs-the-ledger-about-what-it-holds
fields:
  want: a thread receipt costs the ledger about what it holds
  becomes: promise
  candidate:
    statement: >-
      kept killed at any point while a runtime streams loses no line it had read from that
      runtime, and each write to refs/kept/sessions made for a thread receipt
      introduces new git objects totaling at most twice the bytes of the lines it
      records plus 64 KiB — shown by two named tests in an internal/thread suite that
      passes whole.
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
  ripe_when: >-
    A layout for receipts that both named tests pass, chosen by a decision that
    replayed a recorded receipt through the candidates. Both tests exist and both
    fail against today's writer, so the check has fired; they cannot reach main
    before the layout does, because promise:core-packages-are-tested runs every
    test under internal/.
  shaped_from:
    - "measurement of refs/kept/ledger on 2026-10-03, below"
    - "decision:0084-a-runtimes-output-is-read-whole-and-a-torn-item-is-kept"
    - "the shaping conversation of 2026-10-03: what kept can count, and who sets the bound"
---

What the operator wants is for a repair session to cost the ledger roughly the size of what it recorded. Today it costs several times that, and the cost grows faster than the session does. This is about the history git keeps, not the bytes in the current tree.

## What was measured

Measured on 2026-10-03 against this repository's own `refs/kept/ledger`, whose last write was on 2026-09-19:

- **The ledger is about 99% of the repository.** It packs to 562 MiB, and 522 MiB after a fresh fetch into a scratch repository. The branches pack to 3.9 MiB, and `.git` is 620 MB on disk.
- **It holds 134,450 commits, and 124,660 of them touch `ledger/threads/`.** All but about 1,500 were written between 2026-09-14 and 2026-09-19, by eight repair and assessment sessions.
- **Receipts are 461 MiB of the pack.** The current tree is 112 MB uncompressed, and receipts are 97 MB of that, in nine files. Observations are 10 MB in the tree and 0.4 MiB packed. Acts, truth, runs and dispositions together are under 0.1 MiB packed. The records the model reads stay small, as `docs/layout.md`'s volume rules intend.
- **The commits themselves cost 42 MiB**, in commit and tree objects. That is about 330 bytes per commit, whatever the commit changed.

Per thread:

| thread | lines | packed |
|---|---|---|
| `19y86jbnzqjtye6xv1vnxj614s` | 44,889 | 176 MiB |
| `4f8q0g537deca7sw2vvyvht111` | 23,823 | 130 MiB |
| `58pn1kewp155vmnjt3wp2kja40` | 29,558 | 92 MiB |
| `159qnynw8m5fhtkxt6kch6s8fh` | 8,599 | 25 MiB |
| `7ghe0nz6`, `1y6kx8c4`, `7gz5c1wm` | about 4,500 each | 11–13 MiB each |
| `7dpqfr5wdekr4rd4y902yv61vf` | 3,997 | 2 MiB |
| `6ca1kqeyfz205a2s4qf4cb991a` | 315 | under 0.1 MiB |

The largest receipt is 34 MB in the tree and 176 MiB in history, about five times its own size.

## Why

**Every receipt line is its own ledger commit, and each commit rewrites the whole receipt file.** A thread of 44,889 lines leaves 44,889 versions of a file that grows to 34 MB.

Git stores most of those versions as small deltas: the median is 236 bytes and the 99th percentile is 904 bytes. But git caps how long a chain of deltas can get, so every so often it stores a whole compressed copy, and each copy is larger than the last. Those whole copies are 378 MiB of the 461. That is why a receipt's cost grows faster than its line count, and why the 315-line thread from 2026-08-29 cost nothing.

Two side effects follow:

- **Moving the ledger is expensive.** Fetching it into a scratch repository took seven minutes and about an hour of CPU, nearly all of it git computing deltas over 134,000 commits.
- **Publishing it would be large.** On 2026-10-03, `refs/kept/upstream/ledger` was 13 MiB and 129,637 commits behind. The next push would send about 550 MiB to origin, and every clone that adopts the ledger fetches it.

## Where this is already open

Decision 0084 put a spool between the runtime and the receipt so that output is read whole, and it rejected batching as the fix for *lag*. Batching cannot close the lag, because a runtime with an unbounded queue outruns anything bounded by writes. It ends with *"Batching stays open if a receipt's size or latency forces it."* This desire is the size half of that sentence, measured.

## What it must not cost

- **A killed session keeps what arrived before the kill.** That is 0052's partial receipt and 0084's whole stream. A write that holds lines in memory widens what a crash loses. Shaping says how wide is acceptable and proves it the way `internal/land/crash_test.go` proves 0018: by killing a process, not by skipping a call.
- **The receipt stays in git and stays content.** Moving receipts out of git, or out of the record nothing can re-derive, is not this desire. A receipt is irreproducible. Since decision 0089 that record is `refs/kept/sessions` rather than `refs/kept/ledger`, which origin never receives.
- **Nothing on disk is rewritten after the fact.** The 461 MiB already in history stays. Squashing or rewriting `refs/kept/ledger` is ruled out by the reflog witness (0054) and by the ledger being append-only. What changes is what future sessions cost.

## Directions, not decisions

- **One commit per batch of lines**, flushed on size, time, or a session event such as a turn end or `result`. This makes the commit count per session hundreds rather than tens of thousands. It does not change the cost of each version, so the quadratic shape remains at a smaller constant.
- **Segmented receipts:** a thread's receipt becomes a series of files that are each sealed once full. A new line rewrites only the open segment, so the cost per version is bounded by the segment size and total cost becomes linear in what the receipt holds. Readers fold the segments in order. This changes the layout of `ledger/threads/`, so it needs a decision.
- **Native events as content-addressed payloads.** The receipt keeps small typed entries, and the bulky `thread_native` lines (38,919 of the largest receipt's 44,889) move out of it. `docs/layout.md`'s third volume rule already does this for transcripts. Whether a native event is "a large payload" under that rule is the question.

These compose. Shaping picks among them with a measurement, not a guess: replay a recorded receipt through each candidate into a scratch ledger and compare the packed size, commits, fetch time and what a kill at an arbitrary line loses.

## What the bound is

Shaping on 2026-10-03 answered the first two questions this desire used to leave open, and the answers move the remaining question somewhere else.

**Packed size cannot be the bound.** What git stores depends on which deltas it chooses and how deep it lets a chain go, and kept controls neither. A promise on packed bytes would measure git.

**What each write introduces can be.** Every ledger write creates a blob for the file it touches, a new tree for each directory on that file's path, and one commit. The sum of those objects' uncompressed sizes, read with `git cat-file -s`, is deterministic, independent of packing, and something kept writes and can count. Capped by a constant, the total is linear in the session whatever git does afterward. The clause is per write and not per session:

> each ledger write made for a thread receipt introduces new objects totaling at most **A** × the bytes of the lines it adds, plus **c**.

Per write, because then the clause does not depend on the fixture. A quadratic layout fails it on a 300-line receipt as surely as on a 45,000-line one, while a whole-session ratio only catches the curve on a fixture big enough to show it. The figure is raw bytes and overstates what is stored, so A is a ceiling on what kept hands git and not a measure of the pack.

**Commit count is not a separate clause.** The commit and the trees on its path are inside the per-write figure. Today they are about 7.5% of the pack, against 82% for receipt blobs, so the blobs come first. Once the blobs are bounded, the commit overhead becomes the floor: about 330 bytes packed per write against an average line of 760 bytes. A commit-count clause would bound a symptom of the same quantity.

## Durability and amplification pull against each other

At the level of git objects, one commit per line costs several times its payload whatever the layout. A commit writes a whole new blob for every file it touches and rewrites every tree on that file's path. Keeping the touched file small means many small files. Keeping their directory's tree small then means fanout, and even a fanout tree rewrites a few kilobytes on every write. Against a 760-byte line, the fixed cost c of a write is several times the payload. The only lever that amortizes c is writing several lines per commit.

So the directions above do not all answer the same question:

- **Batching alone fails the clause.** It lowers the constant on a file that is still rewritten whole, and no batch size makes each write bounded.
- **Segments meet it.** Only the open segment is rewritten, so a write is bounded by the segment size and the trees on its path. At one line per commit it is bounded but mostly overhead. Batching then amortizes it, at the cost of lines held in memory that a kill loses.
- **A local journal meets it with no loss.** Lines are appended to a file under `.kept/` as they arrive, and the ledger receives each batch once as a sealed file. A kill loses nothing, and the next writing verb folds a journal a crash left behind into the ledger. The cost is that a receipt is not readable from `refs/kept/ledger` until it is flushed or recovered, and that the journal is a second write path. That a commit per line is no more durable than an append to a local file is **assumed** and not measured: whether git syncs a ref update and its loose objects to disk depends on `core.fsync`, and nobody has checked what this repository's git does.
- **Moving native events out** helps only if what replaces them is also bounded per write.

A model of the three layouts over the 44,889-line receipt, at 760-byte lines, about 850 bytes of commit and path trees per write, and trees held under 256 entries, gives the trade's shape. Raw bytes rewrite whole at about 22,000 times the receipt. Segments of 16 KiB come to about 23 times at one line per commit and about 1.2 times at a hundred. A journal with sealed batches comes to about 19 times at one line and about 1.1 times at a hundred. These are a model and not a measurement. The replay below is the measurement.

## Who sets A and c

**Not a repair agent.** A and c are what is owed, and only a person's ratification gives what is owed its standing. A repair works toward a predicate and never authors one: an agent that picked the bound would be writing the bar it is graded against. The change is not repair-shaped either. It widens `Store.Append`, which promises *one line, durable before it returns*, and so it needs a decision before code.

**The numbers belong in the statement.** If the instrument is a named test and A and c live only in its code, the promise's hash does not cover them, and tightening or loosening them would move nothing anyone ratified. Written into `statement`, a change to either drops standing, which is the right cost for moving the bar.

**They arrive in three steps:**

1. **A person chooses the loss bound.** Chosen on 2026-10-03: **no line is lost**. A kill keeps every line kept had read from the runtime, which is what 0052's partial receipt and 0084's whole stream already promise. So batching is available only behind something a kill cannot lose, which is the journal.
2. **The first wording forbids growth and does not pretend to know the optimum.** A loose bound, such as A = 2 and c = 64 KiB, already separates bounded from growing. Today's code fails it once a receipt passes about 64 KiB, and the test should be seen to fail it before anything changes, because a check that has never fired is not a check. Any linear layout passes.
3. **The decision's replay sets the efficiency.** Replay a recorded receipt through each candidate layout, at the chosen loss bound, into a scratch ledger, and read each layout's per-write c and A, its commits, its fetch time and what a kill at an arbitrary line loses. If efficiency and not only linearity should be held, revising the promise to the measured numbers plus margin is a re-ratification.

## The candidate

The `candidate` above is a promise with two clauses, both measured by named tests in `internal/thread`.

- **`TestEachReceiptWriteIsBoundedByTheLinesItRecords`** replays the captured Claude session in `testdata/claude/session.jsonl`, back to back until 80 KiB has streamed, through the adapter's own stream into a store on real refs. For every commit the sessions ref received, it sums the uncompressed sizes of the objects that commit made reachable and its parent did not, and holds them to 2 × the bytes the changed files grew by + 64 KiB. It counts every commit and not only those naming `threads/`, because a transcript fragment or an undecodable item is written to `artifacts/` on a receipt's behalf. It refuses a replay that left the receipt under 64 KiB, because then the bound could not fire.
- **`TestKeptKilledMidSessionLosesNoLineItHadRead`** re-executes the test binary as the kept process, writing a receipt through the supervisor, its spool and the Claude adapter's stream, fed by a stand-in runtime that reports each line once kept has accepted it. Once at least 200 lines are reported and the receipt exists on the ref, it kills the process group. The next run then opens the ledger and writes one record of its own, and the test reads what survived from the sessions ref, never from the killed run's account. That is how `internal/land/crash_test.go` proves 0018.

The numbers are in the statement so the hash covers them. Tightening them to what the replay measures is a revision and a re-ratification.

**Why no line, and why that is affordable.** Every reader of a receipt reads it after its session has returned: the episode's rework gate, `sessionDiagnosis`, the adversarial pass, the assessor, the land and `kept review`. Nothing reads a receipt while its session is running. So a journal that the ledger receives in sealed batches costs no reader anything. The one exposure is a person running `kept review` from another terminal during a repair, who sees what was last flushed.

## Both tests fail today

Run on 2026-10-03 against `main` at `5b2afbc6ba`:

- **The bound fails on write 93**, which introduced 66,776 bytes of new objects to record a 551-byte line: the whole receipt again, as soon as the receipt passed the allowance.
- **The kill test fails worse than expected.** Killed with the receipt already on the ref, kept had read between 6,848 and 7,074 lines across three runs, and the ledger held **none** of them.

The second is not about batching at all. 0084's spool exists so that kept reads a runtime's output as fast as the runtime writes it, and it holds what it has read **in memory**, up to 64 MiB, while the receipt writes it out at two to eleven lines a second. A kept process killed mid-session loses that whole backlog. So the promise this desire's *must not cost* attributes to 0052 and 0084 — *a killed session keeps what arrived before the kill* — holds when the **runtime** is killed and kept survives to drain, and has never held when **kept** is killed. Nothing measured it until now.

That changes the shape of the answer. The journal is not only the way to batch without losing lines: it is the spool made durable. A spool that appends to a file under `.kept/` as it accepts output keeps 0084's pace with the runtime, loses nothing to a kill, and is exactly what the ledger can receive in sealed batches. Segments alone pass the bound and still fail the kill test, because the spool would still hold the backlog in memory.

## Still open

- **Whether the spool becomes the journal.** The measurement above argues it does; the decision says so or says why not.
- **What the journal's recovery is.** Which verb folds a journal a crash left behind into the ledger, and what it does when two runs left one each. Both are for the decision.
- **Whether a kill means the process or the machine.** The kill test proves the first. A journal and a ref update are both local and on the same disk, so the second is the same question for both, and it depends on `core.fsync`, which is **assumed**, not measured.
- **Does `kept doctor` report a ledger that has grown large but not been published?** The outbound comparison already exists. The 550 MiB the next push would send is something nobody was told.
