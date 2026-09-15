---
id: 0085-a-failed-repair-explains-itself-from-its-receipt
type: decision
title: 0085 — A failed repair explains itself from its receipt
fields:
  statement: >-
    A thread's receipt records the executable a runtime was launched from, as
    the parent observed it before the launch and after the reap, and the end of
    what the runtime wrote on its error stream, redacted by its own adapter. A
    repair that fails after opening a thread reports which phase failed and
    where its box is. It also reports how far its session got, how it ended,
    whether its runtime was replaced and what it last said, all derived from the
    receipt. kept review gives the same account of any thread, finished or not.
links:
  - { target: decision:0084-a-runtimes-output-is-read-whole-and-a-torn-item-is-kept, relation: extends }
  - { target: decision:0077-the-receipt-says-what-answered-and-what-it-consumed, relation: extends }
  - { target: decision:0052-repair-runtimes-are-closed-adapters-with-native-receipts, relation: extends }
  - { target: decision:0023-what-may-be-captured, relation: depends-on }
  - { target: decision:0020-durable-when-irreproducible, relation: implements }
---

# 0085 — A failed repair explains itself from its receipt

## Context

On 2026-09-14 a repair that had run for nineteen minutes ended with this, and
nothing else:

```
kept: retain runtime output: decode Claude native stream event: unexpected end of JSON input
```

The retry did the same. Working out what had happened
([0084](0084-a-runtimes-output-is-read-whole-and-a-torn-item-is-kept.md) has the
account) took hours of reading 3.5 MB receipts with `jq`, correlating them with
npm's debug logs and a binary's modification time, and building a model of
kept's own append rate. Most of that time went on three gaps.

**Where the session got to was only in the receipt, and nothing read it back.**
How many events arrived, what the last one was, whether the runtime's `result`
ever came, and how the process exited are all in the entries. The one line
printed said none of it.

**Whether the binary was replaced was an inference.** A reinstall's
modification time sat beside a receipt timestamp. That comparison was wrong in
one thread, because the receipt was minutes behind, and meaningless in the
other, because it was a same-version reinstall. Nothing had recorded which file
was launched.

**What the runtime said on stderr was not kept at all.** The launch held the
first 8 KiB of it in memory for the canary's own error messages, and threw it
away with the process.

## Decision

### The receipt records the executable, twice

A `thread_executable` entry records the path the invocation named, what it
resolved to, and the resolved file's inode, size and modification time.
`thread.Dispatch` writes one immediately before the invocation is handed to
confinement, carrying the version the runtime was admitted as. The supervisor
writes one after the reap and before the exit is recorded, so a receipt still
ends at its exit. The second carries no version, because nothing asked.

A package manager replaces a runtime by renaming its directory and putting a new
file in its place, so the identity that moves is the file's, and *was it
replaced* is a comparison of the two records. Hashing the file was the
alternative and it is not needed. Claude Code's image is 200 MB, and inode, size
and modification time already tell a rename from the file that was launched.

### The receipt records the end of stderr

The launch now keeps the last 16 KiB of the error stream as well as the first
8 KiB. The supervisor records that tail as a `thread_stderr` entry, a redacted
`session.stderr` artifact plus a count of the bytes before it, once the process
is reaped and before the exit is recorded.

Redaction is the adapter's. `Invocation.Redact` is the same function the adapter
applies to its native events, and **an invocation without one records no stderr
at all**, because 0023 does not allow a runtime's error stream to become durable
with its credential still in it. An empty stream writes nothing: a receipt with
no stderr entry is a runtime that said nothing there, and an entry holding
nothing would look like one whose bytes were lost.

### A failure names its phase, and the rest is read from the receipt

A repair that fails after its receipt exists returns a `repair.Failure`. It
carries only what the receipt cannot say: the phase this run was in, and where
the box was left. The phase is one of four:

- `launch`: the session never started;
- `session`: it ran, and its output or exit could not be recorded;
- `record`: it ran, and recording that it ran failed;
- `re-derive`: it ran, and the promise could not be measured in the box
  afterwards.

When the executable is a different file after the session, the repair asks it
what version it is, bounded like every other runtime call, and says so. A
successful repair says it as a warning, and a failed one in its report. That
answer is not recorded. A later reader sees that the file was different, and
what it reports then is a question for then.

Everything else is `thread.Diagnose`, derived on read: the lifecycle, how many
events arrived and the last one's type and time, the last turn count, the model,
whether the runtime's closing event arrived (`result` for Claude), what did not
decode, the two executable observations, the stderr tail, and whether the work
was anchored. Nothing summarizes a receipt beside it, because a summary stored
next to its source is a second place the answer lives.

`kept repair` prints the diagnosis as rows when it fails, followed by the
commands that apply. `kept review` prints the same rows above its checklist, for
any thread, including one that never finished, and its `--json` carries them as
`assessment.session`.

## Alternatives

**Record a finding entry when the executable changed.** It would be the
comparison of the two identity records stored beside them, a cache that can
disagree with its source. The difference is derived on read.

**Ask the runtime its version after every session.** It would be an external
call per dispatch, answering a question the identity records already answer
when nothing changed.

**Keep the beginning of stderr, as the launch already did.** A launch that
cannot start explains itself at the beginning. A session that ran explains why
it stopped at the end, and that is what a failed repair is.

**Record stderr unredacted, or through a redaction the supervisor owns.** The
supervisor holds no credential and knows no adapter's secret forms. Redaction
is the adapter's, as it already is for native events.

## Consequences

- **A receipt grows by two small entries and at most 16 KiB per session.**
- **A receipt written before this decision has no executable records and no
  stderr entry.** It reads as *not retained by the build that ran this
  session*, rather than as a runtime that wrote nothing.
- **What a replaced runtime reports is said once and not kept.** A review a
  week later says the file was different, not what it called itself.
- **Adapters own stderr redaction.** An adapter added later that sets no
  `Redact` keeps no stderr, which is safe and silent.
- **`kept review --json` gains `assessment.session`.** The addition is additive,
  and nothing existing moved.
- **Replacing the runtime underneath a running session does not end it.** 0084
  measured that. The runtime's own tools do exec its path during a session,
  though (Claude Code's `grep` is a shell function around it), so an install no
  background updater touches keeps what they run the same as what kept
  admitted. That is operator hygiene, not a fix for anything 0084 found.
