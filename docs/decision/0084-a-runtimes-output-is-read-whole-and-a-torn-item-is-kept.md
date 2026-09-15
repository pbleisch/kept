---
id: 0084-a-runtimes-output-is-read-whole-and-a-torn-item-is-kept
type: decision
title: 0084 — A runtime's output is read whole, and a torn item is kept
fields:
  statement: >-
    kept reads a runtime's output as fast as the runtime writes it, to the end of
    the stream, and retains it at the pace the receipt can be written. Output an
    adapter cannot read as an event — a malformed line, or the item the stream
    ended inside — is recorded as undecodable with its reason and never fails the
    dispatch, so a session that exited with its output cut off is re-derived and
    anchored exactly as a clean one is. And a box a session ran in is never
    discarded by an error before its work is anchored.
links:
  - { target: decision:0052-repair-runtimes-are-closed-adapters-with-native-receipts, relation: extends }
  - { target: decision:0020-durable-when-irreproducible, relation: implements }
  - { target: decision:0018-what-survives-a-crash, relation: extends }
  - { target: decision:0026-nothing-external-is-unbounded, relation: depends-on }
  - { target: decision:0067-a-repair-anchors-what-it-produced, relation: depends-on }
  - { target: policy:the-agents-report-is-never-an-input-to-the-verdict, relation: implements }
---

# 0084 — A runtime's output is read whole, and a torn item is kept

## Context

On 2026-09-14 two repairs of `promise:each-clause-of-a-predicate-answers-on-its-own`
failed the same way, 19 and 16 minutes into confined Claude Code sessions:

```
kept: retain runtime output: decode Claude native stream event: unexpected end of JSON input
```

Threads `1y6kx8c48hb6mtjkpda76brtz0` and `7gz5c1wmwn5s4pv2rsqk7whs32`. Each
receipt holds thousands of native events, an `exited` lifecycle with code 0, and
no `result`, no cost, no after-measurement and no anchor. Each box had been
discarded, so the sessions' work was gone. The last event in each is a
`stream_event` delta in the middle of a `Write` tool call.

The binary had been reinstalled underneath both sessions. That looked like the
cause and is not. What the receipts and the binary show is below, and each point
was measured.

**The receipt is minutes behind the runtime.** Each native line is its own
ledger append, and over thread `1y6k…` the gap between appends is never less
than 86 ms, with a median of 122 ms. Claude streams a delta per token. Over the
retry's 16 minutes there is exactly one gap of two seconds or more, so kept was
never idle and never caught up. Its 175-second auto-compaction, which Claude
reports every 30 seconds, was recorded in 1.8 seconds: at that point the receipt
was at least three minutes behind what the runtime had already written. Receipt
timestamps are when kept recorded an event, not when the runtime emitted it.
Read as runtime time, they put the last usage-window event beside the npm
install. It was really about three minutes earlier.

**Claude Code discards output nobody has read when it exits.** It queues stream
output in the process without limit while a session runs. At the end,
`drainStdoutBeforeExit` waits `max(2 s, queued bytes ÷ 256 KiB/s)`, capped at
30 s, then exits 0 whatever is left. Measured with 2.1.272 against a loopback
stand-in upstream: with a reader slower than the stream, the process exited 0
at 5.9 s. It had delivered 292 KB of 1.55 MB, 1,189 of 6,000 deltas, no
`result`, and a final line cut mid-record. The same session with a fast reader
delivered everything.

**kept lost output of its own as well.** `child.Wait` called `exec.Cmd.Wait`,
which closes a pipe `StdoutPipe` made the moment the process is reaped. The copy
still reading from it then failed, and nothing looked at the error. Reproduced:
36 KB of 626 KB never arrived, and the last line that did ended mid-record.

**The masking condition.** Thread `7dpqfr5wdekr4rd4y902yv61vf`, the repair
before these, lost its tail too: no `result`, no `message_stop`, no
`thread_cost`. Its last surviving read happened to end on a line boundary, so
`Close` had nothing to decode, the re-derivation found the promise kept, and the
repair was anchored. The one real thread that kept its `result`,
`6ca1kqeyfz205a2s4qf4cb991a`, was 20 seconds long. Whether the last surviving
line is torn is chance. The loss is not.

**Replacing the binary does not end a session.** The package was replaced the
way `npm install --global` replaces it: the directory renamed away, a new one
put in place two seconds later, the old one deleted. That was done mid-stream
under a running print session, unconfined and then under `nono` with the
adapter's own argv. Both sessions ran to `result: success` and exit 0, and a
`Bash` call made after the swap ran the new binary. The updates were run by the
other, unconfined `claude` processes on the machine. Their npm logs recur on a
30-minute cadence, and a confined session can neither reach the registry nor
write the install.

**A malformed line mid-stream was worse than a torn one.** Every adapter
refused it from `Write`. The launch then stopped reading and discarded the
error, so every event after the line was lost and the dispatch reported
success.

## Decision

### Output is read as fast as it is written, to the end of the stream

The supervisor puts a spool in front of the adapter's stream. The launch writes
into the spool, which takes a copy and returns at once. A worker hands the bytes
to the adapter in order, at the pace the receipt can be written. `Close` returns
only once every accepted byte has been handed on, so no native event is recorded
after the parent's record of the exit. The spool holds at most 64 MiB, which is
several long sessions' worth, and past that it applies back-pressure: what every
stream did before it existed.

After an adapter's write fails, the spool keeps accepting and discarding, so the
runtime is never wedged by a receipt that can no longer be written. The failure
is what `Close` returns.

The confinement launch makes its own output pipe and reads it until the stream
ends. It no longer stops at the reap. That wait is bounded to ten seconds after
the reap, for a descendant still holding the write end, because nothing external
runs without a deadline ([0026](0026-nothing-external-is-unbounded.md)).

### Output that does not decode is kept, and the stream goes on

A line an adapter cannot read as one event is recorded as a `thread_undecodable`
entry: the runtime, the adapter version, the adapter's own reason, and whether
the stream ended inside it (`unterminated`). The bytes are stored, redacted
exactly as a native event is, as a `session.undecodable` artifact. It is an
artifact rather than a string, because undecodable bytes need not be UTF-8. The
stream continues with the next line, and nothing is projected from the item.

It is not a native event. A native entry carries the runtime's own type and
JSON, and writing one with a type kept made up would be the invented event
[0052](0052-repair-runtimes-are-closed-adapters-with-native-receipts.md)
refuses. It is not dropped either, because the bytes are what the runtime wrote,
and [0020](0020-durable-when-irreproducible.md) keeps a session's output whole
on every exit path.

This holds for Claude, Pi and Codex alike. Codex's strict envelope still
interprets nothing it cannot read, since recording a line under its reason is
not reading it as a message. A decoded message that contradicts the protocol
still refuses, as before.

### A session that exited with its output cut off is repaired as any other

A torn final item with exit 0 goes on to the re-derivation, the anchor and the
`act` disposition, exactly as a clean exit does. The repair says the output was
cut off. It does not stop, and it does not keep the box instead.

That is the choice between the two behaviours the failure allowed, and three
things decide it:

- **The verdict is measured from the tree.** Whether the session's account of
  itself arrived whole is a fact about the account, and
  [`policy:the-agents-report-is-never-an-input-to-the-verdict`](../policy/the-agents-report-is-never-an-input-to-the-verdict.md)
  keeps that account out of the verdict in both directions.
- **A killed session already goes on to the re-derivation.** A deadline records
  its timeout, kill and reaped exit, then returns without error. A session that
  exited 0 is less abnormal than one kept had to kill, so stopping on it would
  be the stricter rule for the milder case.
- **The torn tail is kept's doing, and routine.** It is what a long session
  looked like to a receipt that could not keep up, and `7dpq…` was one. A rule
  that failed the repair on it would have failed repairs that held.

[0018](0018-what-survives-a-crash.md)'s *dead agent runtime* is a runtime that
died. One that exited 0 did not.

### A box a session ran in outlives an error

If a repair fails after its session started and before its work is anchored,
the box is kept, `Result.Box` names it, and the error ends with where it is and
the two commands that read and discard it. The `act` disposition is still
written, because the session ran and spent a credential. Nothing is re-derived
from a thread whose record is broken. 0018 already says the dead box *"is
retained as an artifact under its own retention so that it can be looked at"*,
and the discard ran ahead of that. `kept doctor` reports a kept box as a
worktree left behind, and removing it is a person's call.

## Alternatives

**Treat a torn stream as a failed session: record it, keep the box, re-derive
nothing.** It was the other behaviour on offer. It fails a repair over a defect
in kept's capture, and it would have failed `7dpq…`, which held.

**Batch receipt appends until the receipt keeps up.** That narrows the lag and
cannot close it. A runtime with an unbounded queue always outruns something
bounded by writes, and the loss happens at exit, when the queue is abandoned.
The spool closes it without changing how the ledger is written. Batching stays
open if a receipt's size or latency forces it.

**Run the repair on a binary no update can replace**, whether the operator pins
a private install or kept copies the image for the session. That was measured
not to be the cause. A private install is fine hygiene, but it would not have
saved either session. kept copying an image would also detach the version
[0072](0072-a-runtime-version-constraint-is-a-review-record.md) admits from the
binary that runs.

**Set `DISABLE_AUTOUPDATER` in the confined environment.** It changes nothing
measured. A print session runs no updater, the confined session cannot reach
the registry, and the updates came from other processes.

**Keep refusing a malformed line, and make the refusal propagate.** Stopping on
it stops reading the runtime, which then waits or discards. That throws away
everything after one bad line, for evidence that decides nothing.

## Consequences

- **A long session's receipt is whole again**: its last turns, its `result` and
  its cost, which [0020](0020-durable-when-irreproducible.md) and 0052 already
  required and which long sessions had silently been missing. Receipt timestamps
  still say when kept recorded an event, not when the runtime emitted it. A
  reader correlating them with anything outside kept has to account for that.
- **kept holds up to 64 MiB of a runtime's output in memory** during a dispatch.
- **A failed repair can leave a box behind**, and repeated failures leave
  several. `kept doctor` reports each; nothing removes one on its own.
- **A build from before this decision quarantines a `thread_undecodable` line**
  as an unknown kind. The addition is additive and nothing is rewritten. The
  work in the two failed threads' boxes is not recoverable.
- **A Codex response that does not decode stalls that protocol.** The session is
  bounded by its deadline, and `Close` then reports the missing evidence. Codex
  is unadmitted.
- **Appends to one receipt are serialized.** The supervisor and the adapter's
  worker append from different goroutines during a deadline, and the ledger's
  sequence was never safe to advance from two at once.
