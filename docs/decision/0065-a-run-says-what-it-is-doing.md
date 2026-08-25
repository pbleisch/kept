---
id: 0065-a-run-says-what-it-is-doing
type: decision
title: 0065 — A run says what it is doing, on stderr, and only about what it does not control
statement: >-
  A tick and a repair report each invocation kept started and does not control —
  the artifact, the instrument or phase, and the elapsed time against the
  deadline that will fire — on stderr, live when both streams are a terminal and
  as timestamped append-only lines otherwise, and nothing reported is recorded.
links:
  - { target: decision:0027-the-machine-facing-surface, relation: depends-on }
  - { target: decision:0026-nothing-external-is-unbounded, relation: depends-on }
  - { target: decision:0060-a-repair-is-one-promise-a-person-named, relation: depends-on }
---

# 0065 — A run says what it is doing, on stderr, and only about what it does not control

## Context

`kept` with no verb printed nothing at all until the whole pass was over, then
emitted the table at once. In between it shells out to other people's programs.
A person ran it, got a blank terminal for a minute, and had no way to tell
working from wedged from waiting on a subprocess that would never return.

The shape of it is measured rather than assumed. Over this repository, eight
artifacts seek truth and seven instruments run on a due tick:

| | cold, test cache cleared | warm |
|---|---|---|
| the workspace walk | 8 ms | 3 ms |
| the act stream | 15 ms | 14 ms |
| `go/tests` over `./internal/...` | 24.9 s | 0.9 s |
| `go/tests` over `./internal/confine` | 28.5 s | 0.3 s |
| `command` running `go vet ./...` | 204 ms | 132 ms |
| `corpus`, `go/symbols` ×2, `go/imports` | 32 ms | 33 ms |
| **the pass** | **56.4 s** | **3.5 s** |

Two calls are 94% of a cold pass. Everything that is not an external invocation
— the walk, the fold, the predicate, the truth append — is 55 milliseconds
together, in both columns. And the same tick over the same corpus varies by a
factor of sixteen with nothing on screen to say which case a person is in.

`kept repair` is worse on this axis rather than better. A dry run over this
repository is 203 ms cutting the box, 27.4 s measuring the promise in it, 13 ms
locating the runtime, 7 ms rendering the profile and asking the provider about
every path, and 885 ms verifying the box with the canary. The dispatch that
follows a real one was measured between 18 and 63 seconds — against a bound of
sixty minutes, from `timeouts.agent`, which is the longest silence in the
system.

## Decision

**A run reports every invocation kept started and does not control, and nothing
else.**

For a tick that is one gather: the artifact, the instrument as its promise names
it, and how long it has been running against the gatherer deadline. For a repair
it is a phase, because a repair is made of phases rather than instruments — the
box, the measurement in it, the runtime, the profile, the canary, the session,
the re-derivation, the diff — and the two derivations report the gather inside
them, so the innermost thing running is what a person waiting sees.

The rule is not "report progress"; it is **report what kept is waiting on**. A
display that also announced the walk, the fold and the evaluation would spread
itself evenly over phases that are not evenly expensive, and the reader would
learn least about the part that takes all the time.

**Nothing counts artifacts down**, for the same reason and from the same
numbers. A pass's remaining time is decided by which instruments are left rather
than how many, and over this corpus those differ by three orders of magnitude —
8 ms for `corpus` against 28 s for `go/tests`. "7 of 8" would mean either 4% or
96% of the wall clock, and a fraction that can be wrong by that factor is worse
than no fraction.

**The elapsed time is shown against the bound that will actually fire.** A
deadline nobody can see is not a deadline anybody is reassured by: `10m` says
nothing to a reader who cannot tell whether they are twelve seconds into it or
nine minutes. [0026](0026-nothing-external-is-unbounded.md) already requires
every external call to carry one; this is the first thing that shows it. A phase
with no deadline of its own says none rather than borrowing a neighbour's.

### Two forms, and which stream

**Progress is on stderr and the answer is on stdout.**
[0027](0027-the-machine-facing-surface.md) makes the machine-facing surface a
contract, and this is not part of it: `--json` is byte-identical with the
display running and with it silenced, and the exit codes are untouched. That
document's own line — *the human format is not a contract and may change* — is
what admits this without amending it.

**The live form needs both streams to be a terminal.** That is stricter than
"the stream being written is a terminal", and deliberately: the two are one
screen, and a run whose answer is being captured to a file is a run somebody
reads later, where a timestamp beats an animation. It is one line, redrawn in
place, erased before anything else is printed. It draws nothing until a step has
been running for a second — a warm pass finishes five of its seven gathers in
under twenty milliseconds each, and drawing those would be flicker reporting
nothing a person could read.

**Everything else gets append-only lines and no escape of any kind.** A pipe, a
file, a CI job, an agent's captured pane. Four fixed fields — an RFC3339 instant
in UTC, the word `kept`, the kind, and `started` or `finished` — then the step's
own words. A reader filters on the four and reads the rest. The instant is UTC
and RFC3339 rather than kept's own human stamp because it has to be one field
and it has to sort, and that stamp is neither: it carries a space and it is
local.

The plain form announces every start immediately, and does not wait as the live
form does. A start line earns its place precisely by existing before the finish
line — that is what tells a reader watching a running job that the gap is a
program working rather than a program gone — and a threshold would delete it for
exactly the step that was killed early.

### One flag, and why the default could not do it

**`--quiet`, on the tick and on `kept repair`, and no verbosity ladder.**

The default handles every case but one: a terminal gets the live form, and
everything else gets lines a log reader can consume. What it cannot do is give
an operator a stream carrying only problems, because **problems and progress
share stderr** — a refusal, a quarantined ledger line, an advanced ledger and a
rewound ref are all on it — so redirecting the stream to discard progress
discards the findings with it. That is the one thing the default cannot do, and
it is what the flag is for. Three levels nobody can remember would be a larger
contract answering a question nobody asked.

It is on those two verbs and no others, because those are the two that are slow.
A flag that did nothing on `kept drift` would be a knob that silently does
nothing, which this repository already refuses elsewhere.

### It is presentation and never record

Nothing reported here reaches the ledger, an observation, a verdict or a
disposition. `internal/progress` writes to one `io.Writer` and holds no other
handle; a display that failed outright would change no answer kept gives.
`policy:record-only-what-has-already-happened` and
`promise:standing-is-minted-in-one-place` are untouched, and a timing that only
exists inside a gatherer is passed out for display rather than stored.

A nil `*progress.Display` is a run nobody is watching, and every method on one is
safe. Callers hold the pointer and never guard it — a display that had to be
guarded would grow a guard somebody forgot, and the place they would forget it
is the path that records a verdict.

### The tick names what is wrong with the corpus

The same pass ended with `1 problem reading the corpus; kept list says what they
are`, naming nothing, directly under `8 artifacts · 8 kept`. It now prints the
problems, from the same function `kept list` prints them with.

A count with a pointer to another command is the weakest useful thing kept can
say about a defect it has already identified in full — the path and the detail
are in hand — and beside a confident summary it reads as a footnote to a clean
bill of health. A reader who has to run a second command to learn whether what
they were just told is true has not been told anything. Nothing is elided: a
truncated list is the same failure one step along.

It stays a report and never a refusal. [0050](0050-doctor-reports-what-is-well-formed-and-unwise.md)'s
split holds — `kept list` reports what is wrong — so naming it here changes no
exit code, no verdict and nothing recorded.

## Alternatives

**Print a permanent line per finished step, and redraw only the one in flight.**
Rejected for the terminal form: the tick's table is already the record of what
the pass found, and a scrollback holding a second account of it makes a reader
compare two things that have to agree. The plain form does keep both lines,
because a log has no table under it.

**Leave the terminal alone and let an operator run with `2>&1 | ts`.** Rejected:
it is the default invocation that is unreadable, and a person who has to know a
trick to see whether the tool is alive will conclude it is not.

**Report from inside `internal/gather`.** Rejected: the runner shares a
measurement between two promises naming the same instrument, so a display there
would either report one gather twice or report the sharing, and neither is what
a person is waiting on. The tick is where the artifact's name is, and the
artifact's name is half the line.

**Surface the session's own turn and tool events during a dispatch.** Not
rejected, and not built. The adapter receipt is an interface and could be
wrapped for display without touching a record, which is the shape it would take.
It is left out because the only evidence for what such a display should say
costs a live credentialed session per iteration, and the phase line with its
elapsed against `timeouts.agent` already answers the question that was asked.

**A verbosity ladder.** Rejected above: the default plus one flag covers every
case with a smaller contract.

## Consequences

- **stderr carries more on a non-terminal destination**, two lines per external
  invocation, and it scales with the corpus rather than with the wall clock. On
  this repository that is eighteen lines; on a large one it is the reason
  `--quiet` exists.
- **The live form assumes eighty columns.** kept asks no terminal for its width:
  there is nothing in the standard library that does, and a module for it would
  have to pass [0029](0029-dependencies-pass-a-test.md)'s test to answer
  something this cosmetic. A narrower terminal wraps the line and the erase then
  clears only its last row.
- **Every verb that opens a display closes it before printing an answer**, because
  the live form owns a row and an answer written under it would land on a line
  still being redrawn. That is an ordering a future verb has to keep.
- **The plain form is now something a log reader can depend on**, and its first
  four fields are the part that should be treated as settled. The words after
  them are the human half and may change.
- **The measurement in this record will go stale.** The numbers are what decided
  the design rather than what the design promises, and they were taken on one
  machine on one day. What survives them is the rule they produced: report the
  invocation kept does not control, and nothing else.
