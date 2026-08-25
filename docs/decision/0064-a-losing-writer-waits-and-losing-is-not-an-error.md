---
id: 0064-a-losing-writer-waits-and-losing-is-not-an-error
type: decision
title: 0064 — A losing writer waits, and losing is not the operator's error
fields:
  statement: >-
    A writer that loses the ledger ref's compare-and-swap waits a jittered,
    doubling interval whose width is a multiple of what an attempt costs on this
    machine, and a run whose records never landed exits 3 rather than 2.
links:
  - { target: decision:0016-the-ledger-lives-on-its-own-ref, relation: extends }
  - { target: decision:0013-the-gate-makes-concurrency-safe, relation: depends-on }
  - { target: decision:0027-the-machine-facing-surface, relation: depends-on }
---

# 0064 — A losing writer waits, and losing is not the operator's error

## Context

The ledger is one ref, and every ledger write is a read-modify-write against
it: read the ref, build a commit on the value read, and update the ref with
that value as the compare-and-swap expectation. A writer that loses reads
again and rebuilds. [0016](0016-the-ledger-lives-on-its-own-ref.md) says so in
as many words, and the whole *a ledger only moves forward* argument rests on
it.

What was never decided is the **schedule**. The loop retried a fixed eight
times with no wait at all between attempts, which is the worst schedule
available for contention rather than merely an unconsidered one: every loser
of one compare-and-swap read the same value and lost to the same winner, so
they are a cohort with a common clock, and retrying them all at once re-runs
the entire cohort into the next collision together. The budget is then spent
on collisions the schedule itself arranged.

**Measured, on this repository.** Eight concurrent writers, each appending the
handful of records one `kept tick` appends — a tick writes acts, truth,
observations, dispositions and its own diagnostics, so one line per writer
reproduces nothing and never did. Under the old schedule **26 of 80 appends
were abandoned and 54 of 80 lines reached the ref**. The budget was the
binding constraint rather than bad luck: eight attempts against eight writers
is a budget equal to the contention it has to absorb, and an attempt yields to
at most one winner.

Two things were wrong there and they are separable.

**The exit code was a lie.** A tick whose ledger write did not land exited 2,
which in [0027](0027-the-machine-facing-surface.md)'s table is *usage error —
bad flags, unknown verb, unparseable corpus*. Losing a race to another run is
not something an operator asked for. It is the ordinary consequence of running
kept twice at once, which is a thing kept is built to allow.

**`docs/layout.md` said there is no compare-and-swap.** Under *What this buys*:

> No compare-and-swap, and no leader election. Both exist to arbitrate two
> writers over the same bytes, and the layout means that case does not arise.

`RefStore.Append`'s own doc comment said the opposite — *a compare-and-swap
retry preserves both writers' lines*. One of the two had to change, and which
one is not a judgment call: that paragraph was written in this repository's
first commit, three days before the ledger moved to a ref, and `git log -L`
over it shows it has not been touched since. It was true of the **file
layout**, where two runs appending to `truth/promise/<id>.jsonl` from separate
checkouts really do never write the same bytes and git's union merge is the
whole coordination story. 0016 moved the ledger onto one ref, and a ref update
*is* two writers over the same bytes. The document describes a layout that no
longer holds the ledger.

## Decision

**A losing writer waits, drawing its delay uniformly from a doubling window
that is capped.** Full jitter, in the ordinary sense: `wait = rand[0, window)`.
The jitter is the half that does the work — it is what breaks the cohort apart,
and a fixed delay of any size would keep the cohort synchronized while merely
slowing it down.

**The window is measured in attempts rather than in milliseconds.** W writers
each holding the ref for A cannot all land inside less than W·A of serialized
work, so the window they draw from has to be a multiple of A — and A is not a
property of this code. One attempt is seven Git invocations: **53 ms on an idle
machine here, and about 220 ms with the rest of the test suite running beside
it**. A constant window is therefore right for one load and wrong for every
other one, which is not a subtlety: see *A window in milliseconds*, below, which
was written, measured and thrown away.

So the first window is a multiple of what the attempt that just failed cost, and
it doubles from there.

| | | |
|---|---|---|
| scale | 2× the attempt just made | By the fourth window that is 16·A, which is twice the serialized work eight writers represent — whatever the machine is doing. |
| floor | 25 ms | What the window is when an attempt cost nothing worth measuring. |
| cap | 5 s | Measured rather than chosen: at 3 s and at 4 s a loaded cohort of eight still lost one append of eighty; at 5 s neither a loaded eight nor an idle sixteen lost any. |
| attempts | 12 | Half again the cohort that exhausted the old budget of eight. It is not larger because the schedule is what does the work now: sixteen concurrent writers, twice the cores here, land every line inside twelve. |

**The bound is attempts, not a deadline.** `RefUpdateBudget` states the ceiling
— 55 s, every window at the cap — so nobody has to reconstruct it. It is a
ceiling and not a typical: it is reached only when the measured attempt cost has
pushed every window to the cap, and only a writer that goes the whole distance
pays any of it, which is a writer that would otherwise have failed outright. A
wall-clock deadline was rejected for the reason it usually is: it makes the
number of attempts depend on how loaded the machine is, so the budget shrinks
exactly when contention is worst — the same mistake, one level up, as sizing the
window in milliseconds.

**A run whose records never landed exits 3.** Not 2, which blames the operator.
Not 1, which would be the CLI recording what it thinks it saw — nothing about
the tree is known false. Not 4, which says the record standing is derived from
is incomplete, and that is the one thing certainly untrue here: the ledger is
intact and consistent and this run is simply not in it.

Three is 0027's own definition rather than a widening of it. That table's line
is *the world disagrees with a commitment* against *there is no evidence either
way*, and a tick that gathered, evaluated, and then wrote nothing and printed
nothing produced no evidence. This applies 0027 the way 0016 applies
[0050](0050-doctor-reports-an-unchecked-instrument.md), and 0027 needs no
amendment.

**An exhausted budget is a distinct error, and a genuine failure is untouched.**
`ledger.ErrContended` is what an exhausted budget wraps, and `unrecorded` in
`cmd/kept` is the one place that reads it. The discrimination that matters is
older and stricter and stays exactly as it was: `retryUpdate` retries only two
Git messages, both of which mean *somebody else wrote first* — a compare-and-swap
that found a different value, and a ref that appeared while this writer thought
there was none. A broken ref, a permission problem, a corrupt object or a ref
that cannot be created matches neither and surfaces as itself on the first
attempt, so a longer schedule cannot spend the budget hiding one.

**`docs/layout.md` is corrected rather than the code.** *What this buys* now
says what it was always about — the tier that is one file per artifact, union
merged — and says that the ledger is not in that tier and has a compare-and-swap
because of it.

**This is not a lock, and it does not touch 0013 or 0015.**
[0013](0013-the-gate-makes-concurrency-safe.md) says no lock is load-bearing,
and nothing here is: a losing writer is not excluded from anything, it rebuilds
its own commit on what it read and appends its own bytes, and if it gives up
nothing is lost because it never wrote. [0015](0015-landing-is-a-merge-queue.md)
names the one critical section that is load-bearing, which is between a gate and
a push and is not this. A retry is the opposite of exclusion: it is what a
system does *instead* of arranging for only one writer at a time.

## Alternatives

**A window in milliseconds.** Written first, and measured: a constant 25 ms
first window doubling to a constant 2 s cap, derived from the 53 ms an
uncontended attempt costs on an idle machine. Eight concurrent writers landed
every line at rest, over five runs — and **lost three of eighty appends the
first time the full test suite ran beside them**, because an attempt then cost
about 220 ms and a 2 s cap had stopped being a multiple of it. Raising the
constant would have moved the load at which it fails rather than removing the
failure. It is recorded here rather than quietly replaced because the failure is
the argument for what replaced it.

**A wall-clock retry deadline instead of an attempt count.** Rejected: it makes
the number of attempts depend on how loaded the machine is, so a writer gets
fewer tries exactly when contention is worst. It is the same mistake as sizing
the window in milliseconds, one level up.

**A lock around the ledger write.** Rejected, and it would contradict
[0013](0013-the-gate-makes-concurrency-safe.md) and
[0015](0015-landing-is-a-merge-queue.md) rather than merely being unnecessary. A
retry is the opposite of exclusion: a losing writer is not kept out of anything,
it rebuilds its own commit on the value it read and appends its own bytes, and
if it gives up nothing is lost because it never wrote. `flock` would also be
one more thing to hold across seven Git invocations, on a path every worktree
would have to agree on, to save work that is already cheap and already correct.

**Retry forever.** Rejected: a writer that never gives up cannot be told apart
from a wedged one, and an unattended run that never returns is worse than one
that returns 3.

**Make the attempt cheaper instead.** Not rejected, just not this. Seven Git
invocations per attempt is what makes the contention window wide in the first
place, and a `mutate` that built its commit with fewer processes would need less
schedule to converge. It is a larger change than this one and it does not remove
the need for a schedule.

## Consequences

- **Eight concurrent writers land every line.** Measured before and after over
  the same cohort — eight writers, ten appends each, which is what one tick
  appends: **26 of 80 abandoned before, 0 of 80 after**, on an idle machine and
  with the full test suite running beside it. Sixteen writers, twice the cores
  here, also land every line.
- **A contended write is slower and the slowness is bounded.** A writer that
  goes the whole distance waits at most `RefUpdateBudget`, and only a writer
  that would otherwise have failed pays it. An uncontended write is unchanged:
  the wait happens only after a retryable failure.
- **The ceiling is large and it is stated rather than hidden.** 55 s is a long
  time for one ledger append, and it is the price of not losing the write. What
  makes it acceptable is that it is unreachable except under contention that
  would otherwise abandon the writer outright, and that every window before the
  cap is proportional to what an attempt on this machine actually costs.
- **The schedule is a value rather than three constants read out of the loop**,
  because the schedule it replaced has to stay runnable. A contention test that
  only ever runs the schedule that works is a test that has never fired, and
  the claim being made is a comparison between two schedules rather than a fact
  about one. `internal/ledger/contention_test.go` runs both.
- **`kept` gained a reachable exit 3 that is not about an artifact's verdict.**
  A CI job that branches on 3 as "could not determine, try again" is already
  correct for it; one that branches on 2 as "the invocation was wrong" stops
  being told a lie.
- **The cap is still this machine's number, and the rest of the schedule is
  not.** Scaling the window to the measured attempt cost is what removes the
  machine from the first four windows; the cap is a constant above them and was
  fixed against a loaded eight and an idle sixteen here. A machine where an
  attempt costs very much more would reach the cap sooner and get less spread
  from it. That is the cost accepted, and it is smaller than the one the first
  version of this decision accepted, which was the whole schedule.
- **Nothing about what a record says or what standing keys on changed.** The
  bytes a writer appends are the bytes it would have appended, and a losing
  writer rebuilds on the value it read rather than merging anything.
