---
id: 0058-the-ask-is-declared-and-travels-beside-the-packet
type: decision
title: 0058 — The ask is declared, and travels beside the packet
fields:
  statement: >-
    A repair runtime is told what kind of work it has been given, on a channel that is not
    the packet's. The ask is a declared value from a closed per-adapter set, rendered from a
    reviewed constant in the adapter; it names no promise, no file and no evidence, it is
    identical for every packet, and no operator or corpus can supply one. For Claude it is
    appended to 0052's pinned argv as a system-prompt addition, which is a change to that
    argv and is why this decision exists.
links:
  - { target: decision:0052-repair-runtimes-are-closed-adapters-with-native-receipts, relation: amends }
  - { target: decision:0009-repairs-are-not-enumerated, relation: extends }
---

# 0058 — The ask is declared, and travels beside the packet

## Context

A packet states ratified authority and measured evidence and never an ask.
[0009](0009-repairs-are-not-enumerated.md) is deliberate about that: the packet
states the gap and the agent decides, because a packet that said what to do
would be a repair library written one repair at a time.

That works when the runtime's own harness supplies the framing. Pi's does: the
Pi adapter's admission evidence is a confined session that read a packet and
repaired the promise, and nothing in kept told it the message was work.

Claude Code's does not, and it was measured. Eight confined sessions ran against
`api.anthropic.com` through the brokered route of
[0057](0057-the-credential-is-brokered-and-the-box-holds-a-phantom.md), in boxes
cut from kept's own corpus with a promise deliberately broken there. The launch
half of 0052's table held completely: the packet arrived and was replayed, the
projections referenced native events, and a session killed at its deadline kept
its partial receipt, its timeout cause and its reaped signal. Of the six that
ran to their own end, one performed the repair — and that one had read the test
authoring the break out of its own box. Every session with a clean box located
the fault from the packet alone, named the file, the line and the enclosing
function, wrote a correct account of why the promise does not hold, and stopped
without editing anything. Carrying the packet's dispatch policies changed
nothing.

It is not confinement. A shell in the same box appends to the file under repair
and is refused one directory up, measured separately. The runtime was launched
with `--print`, which is a question-answering mode, and it read a statement of
authority and evidence as a question and answered it — accurately.

So something has to say that the message is work. There are exactly three places
it could go, and each is ratified design: the packet's content (0009), the
adapter's input channel, which the Claude adapter keeps free of any prefix so
that the exact packet bytes are what arrives, or the argv 0052 pinned. Whichever
is chosen is a decision rather than a wording, which is why the gate stayed shut
until this one was taken.

## Decision

**An adapter receives a declared *ask* alongside the packet and its
configuration, and renders it on a channel that is not the packet's.**

The ask is a value from a closed set the adapter declares, one member today:
`repair`. It is not a string a caller composes, an operator configures, or a
corpus artifact carries. Each value resolves, inside the adapter, to one
reviewed constant — the words a runtime is given — and an unset or unknown ask
is a dispatch refusal rather than a default.

**What the ask may say is bounded by what the packet already says.** An ask
states what kind of work this session is and how the work is judged. It names no
promise, no artifact, no file, no observation and no repair. It is byte-identical
for every packet of its kind, which is the checkable form of that rule and is
what keeps 0009 intact: the packet still states the gap, the agent still decides,
and nothing per-repair travels on this channel.

**The framing is a property of the pair — this runtime, this kind of work — and
never of the runtime alone.** A second kind of work is a second value in the set
and a second constant beside the first. It is not a template, a registry, a
scenario framework or an extension point; 0052 refuses an open runtime protocol
for the same reason, and there is one kind of work.

**For Claude the channel is `--append-system-prompt`, appended to the pinned
argv.** That extends 0052's fixed argv by two elements, which is the amendment
this decision makes. `--append-system-prompt` rather than `--system-prompt`
because replacing the default prompt takes the runtime's own tool and permission
instructions with it. Measured against 2.1.241 with the pinned argv: the
appended text is honoured under `--safe-mode` with `--print` and `stream-json`
output, which the flag's own help does not say — `--safe-mode` disables what a
session *discovers*, and an explicit argv value is not that.

**The adapter's input channel is unchanged.** The packet is still the whole of
one stdin JSONL user message, with nothing prepended to it, so "the exact packet
bytes survive the input channel" still means what it meant.

**A pinned argv carrying prose changes what the no-shell test can say.** The
rule that no argv element carries a shell metacharacter is a proxy for "nothing
here expects an interpreter", and it holds for every element composed from
configuration or from a packet. The ask is exempt from it by name and held to a
stronger rule instead: it is a constant in reviewed code, it is one argv element,
and it does not vary with the packet.

## Alternatives

**Put the ask in the packet.** Rejected: it is what 0009 refuses, and the refusal
is not a technicality — a packet that carries an instruction is a packet whose
authority and whose instruction cannot be told apart afterwards, and the first
per-promise wording added to it is the beginning of the repair library.

**Prefix the input channel.** Rejected: the exact bytes arriving is the property
the whole packet artifact rests on, and a prefix makes the marker found in a
session's evidence evidence about something kept composed rather than about the
packet.

**Let the operator configure the wording.** Rejected: a channel that can carry
anything can carry a per-repair instruction, which is 0009 again by another
route, and an operator-authored prompt is neither reviewed nor recorded with the
act.

**Build a scenario framework — a registry, templates, per-promise framing.**
Rejected: there is one kind of work. 0043 and 0052 both refuse an extension
point built before a second case exists, and a second value in a closed set with
a switch arm beside it is what an extension point would have to beat.

**Leave Claude unadmitted.** Rejected: everything else 0052 asks for is measured
and holds, the confinement is correct, and what was missing was that nobody had
told the runtime the message was work. Declining to say it is not a smaller
change than saying it — it is the same decision with the answer "no", and it
leaves a runtime the operator configured permanently unusable for the reason
that it is polite.

## Consequences

- 0052's pinned Claude argv gains `--append-system-prompt <ask>`. The adapter
  fixture records the channel and the closed set of values, and not the words:
  those are in the adapter, in one place, and a second copy in the fixture would
  be a second place one rule lived.
- `RuntimeConfig` carries the ask. A dispatcher sets it; `internal/repair` sets
  `repair` because that is what it assembles. An adapter never defaults it,
  because a default would launch an unframed session that looked framed.
- The other two adapters are unaffected and unanswered. Pi needs no ask today
  and Codex is unadmitted; when either gets a second kind of work it needs its
  own value and its own words, decided the same way.
- The ask is not evidence and is not recorded in the receipt. It is in the argv
  the launch composes and in reviewed source, which is where a reader looks for
  what every session of a kind was told; a per-session copy would suggest it
  could differ per session.
- What the runtime does with the ask is still not kept's to assert. The verdict
  is re-derived from the box's world after the session exits, under
  [`policy:the-agents-report-is-never-an-input-to-the-verdict`](../policy/the-agents-report-is-never-an-input-to-the-verdict.md).
  An ask that made a session act and produce a wrong repair is caught by the
  same re-derivation that catches a session that did nothing.
