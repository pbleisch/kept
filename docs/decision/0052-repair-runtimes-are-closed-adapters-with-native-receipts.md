---
id: 0052-repair-runtimes-are-closed-adapters-with-native-receipts
type: decision
title: 0052 — Repair runtimes are closed adapters with native receipts
fields:
  statement: >-
    The repair-agent dispatcher supports exactly Claude, Codex, and Pi through closed direct
    adapters, each launched by argv with an immutable packet and an incremental native receipt;
    an unavailable or unsupported runtime is refused rather than discovered or substituted.
links:
  - { target: decision:0009-repairs-are-not-enumerated, relation: extends }
  - { target: decision:0011-tracked-formats-must-merge, relation: depends-on }
  - { target: decision:0017-the-box-is-not-the-jail, relation: extends }
  - { target: decision:0020-durable-when-irreproducible, relation: extends }
  - { target: decision:0026-nothing-external-is-unbounded, relation: extends }
  - { target: decision:0047-a-promise-binds-what-a-repair-may-touch, relation: depends-on }
---

# 0052 — Repair runtimes are closed adapters with native receipts

## Context

The increment-5 plan makes `repair` an act rather than a diagram: a thread cuts
a box, assembles a packet, dispatches an agent under the named confinement
provider, re-gathers in the box, and presents a diff. The decisions already
settled the surrounding boundaries. The agent may change a box but not the
ledger ([0016](0016-the-ledger-lives-on-its-own-ref.md)); its process is
confined by `nono`, not by the box ([0017](0017-the-box-is-not-the-jail.md)); a
promise's bound repair scope decides what may land
([0047](0047-a-promise-binds-what-a-repair-may-touch.md)); and the gate, not the
agent's account, decides whether the promise is kept.

The product surface now requires three repair-agent runtimes: Claude, Codex,
and Pi. A runtime is not the confinement provider. `nono` controls what the
process may touch; a runtime starts the coding session, receives the packet,
and emits evidence about that session. Treating those as one program would let
a runtime choice weaken confinement, which is already a ratified decision.

The runtimes do not have one event language. Their event names, payloads, turn
boundaries, usage units, and exit reports are evidence about what happened in a
session. Reducing all of them to an invented `message` event would erase exactly
the detail an assessor needs. The opposite extreme is also wrong: a
completion-only envelope loses a killed session's transcript, cost, turns, and
reason for stopping — the failure [0020](0020-durable-when-irreproducible.md)
was written to prevent.

Three direct integrations are small and deliberate. A plugin protocol,
runtime discovery mechanism, or generic compatibility layer would freeze an
unbounded surface before a fourth runtime has a demonstrated need. It repeats
the premature protocol mistake [0043](0043-the-catalogue-is-closed-and-a-third-party-is-namespaced.md)
avoids for gatherers.

## Decision

### The runtime set is closed

**The only V1 repair runtimes are `claude`, `codex`, and `pi`.** Runtime
selection is an explicit configuration value in that closed set. An unset or
unknown value, a missing executable, or a version outside the adapter's
supported range is a dispatch refusal. It never falls back to another runtime,
searches `PATH` for a candidate, or runs unconfined.

Each value resolves to one concrete adapter in `internal/thread`. The adapter
boundary has exactly three implementations and a static selection; it is not a
registry, plugin API, or extension point. Adding, removing, or renaming a
runtime changes the closed set and needs a new decision. The adapter's command
path and fixed arguments are explicit configuration, resolved before dispatch,
and executed as argv — never as `sh -c` or an interpolated command string.

### The shared adapter contract is deliberately small

Every adapter has the same five responsibilities, while retaining its runtime's
own event meaning:

1. **Launch.** Construct the documented direct argv invocation for its fixed
   runtime version range and start it in the thread's box under the already
   selected confinement provider.
2. **Deliver the packet.** Convey one immutable packet byte sequence through
   that runtime's documented noninteractive input channel. The adapter records
   the exact bytes as the packet artifact before launch; it never rebuilds them
   from corpus or ledger reads afterwards.
3. **Inject the credential.** Pass only the configured runtime credential into
   the confined child through the adapter's explicitly named environment
   channel. It is never an argv value, packet field, ledger value, artifact
   path, or a credential capable of pushing to `origin`.
4. **Receive evidence.** Read the runtime's native event stream as it arrives,
   preserve the redacted native event and its native type, and append the
   receipt projections the common thread record needs.
5. **Stop and reap.** Observe normal exit; on the configured deadline, kill the
   complete launched process group through the confinement launch, then record
   the timeout, kill, and reaped exit that actually occurred.

The packet contains only the ratified authority and the gap the agent may
address: the artifact's binding content, the `why`, the observation, the bound
repair scope, and prior attempts. It contains the policies that bind dispatch,
but never a desire, ranking, budget, queue state, or another pipeline input.
This enforces [0009](0009-repairs-are-not-enumerated.md) and
[`policy:the-pipeline-is-never-in-a-packet`](../policy/the-pipeline-is-never-in-a-packet.md)
at assembly rather than asking a runtime to ignore data it was given.

The selected confinement provider and its canary remain outside this boundary.
An adapter receives a box and a confinement launch capability; it neither
constructs a jail profile nor determines whether a canary passed. An unset
provider or failed canary refuses before any adapter starts its runtime.

### Native events are retained; common projections are references

The durable receipt is an append-only stream keyed by the minted thread id.
Every event that arrives from a runtime is retained as a `native` entry with the
runtime name, adapter version, native event type, redacted native payload, and
an event reference. The entry preserves the runtime's own semantics; no common
event enum claims that a Claude turn, a Codex item, and a Pi message mean the
same thing.

The adapter also appends only these shared projections, each referencing the
native event that supplied it:

| Projection | Meaning |
|---|---|
| transcript fragment | Bytes that belong in the session transcript, in received-stream order; stored as a content-addressed durable artifact |
| cost update | The runtime's reported monetary cost, or a versioned conversion from its reported usage that can be reproduced without a network read |
| turn update | The runtime's reported turn or iteration boundary and its cumulative count |
| lifecycle | Parent-observed `started`, `timed_out`, `killed`, and normal exit-code or terminating-signal events |

The raw native entry remains the evidence if a projection is incomplete or a
runtime changes its vocabulary. A projection cannot overwrite a prior value,
and it cannot manufacture a cost, turn, transcript, or exit that the runtime or
parent did not observe. Redaction occurs before a native payload or transcript
fragment becomes durable, with the record stating that it occurred, as
[0023](0023-what-may-be-captured.md) requires.

Each receipt item is written only after it happened: a received native event, a
started child, an elapsed deadline, a kill performed, or a reaped exit. A crash
can therefore leave a partial receipt and never a completed session inferred
after the fact. The receipt and its content-addressed fragments are durable
under [0020](0020-durable-when-irreproducible.md); their per-thread append-only
layout keeps concurrent dispatches from rewriting one another under
[0011](0011-tracked-formats-must-merge.md).

The receipt is evidence for an assessor. It is never an input to the
post-repair gather or verdict, which are re-derived from the box's world under
[`policy:the-agents-report-is-never-an-input-to-the-verdict`](../policy/the-agents-report-is-never-an-input-to-the-verdict.md).

### Every adapter proves the interruption path

An adapter is admitted only with a runtime-specific fixture and a deliberately
interrupted live session. The fixture records the documented runtime version,
direct argv, packet-input channel, credential environment name, native event
examples, and the adapter's projection mapping. The live test uses a bounded,
non-destructive session whose packet carries a unique marker and which remains
active long enough to emit multiple receipt updates before the deadline.

| Adapter | Required concrete evidence |
|---|---|
| `claude` | A Claude native-stream fixture; a direct Claude launch receiving the packet marker; transcript, cost, and turn projections referencing native Claude events; and a kill after multiple updates that retains the partial receipt, timeout cause, and reaped exit. |
| `codex` | A Codex native-stream fixture; a direct Codex launch receiving the packet marker; transcript, cost, and turn projections referencing native Codex events; and a kill after multiple updates that retains the partial receipt, timeout cause, and reaped exit. |
| `pi` | A Pi native-stream fixture; a direct Pi launch receiving the packet marker; transcript, cost, and turn projections referencing native Pi events; and a kill after multiple updates that retains the partial receipt, timeout cause, and reaped exit. |

For all three, the test also proves that the argv has no shell, the exact packet
bytes survive the adapter's input channel, confinement passes before launch and
blocks launch on a failed canary, and the runtime cannot read the ledger,
keychain, credentials capable of pushing to `origin`, or paths outside its
confinement policy. A completion-only cost or turn report does not meet this
contract: it is not incremental evidence and is lost on the failure path the
test exercises.

## Alternatives

**Keep one selected runtime.** Rejected: it does not satisfy the required
Claude, Codex, and Pi product surface, and a hidden fallback would make a
runtime choice unreviewable.

**Build an open runner/plugin framework.** Rejected: there are exactly three
known runtimes. An extension protocol would commit kept to event, cancellation,
and credential vocabulary for unknown programs before a fourth runtime created
evidence that the abstraction holds.

**Flatten native streams into a generic message protocol.** Rejected: native
event type and payload are evidence. Flattening hides different turn and usage
semantics, making a future assessor read an invented summary rather than the
record the runtime produced.

**Keep only each runtime's native stream.** Rejected: `thread` needs common,
queryable references to transcript, cost, turns, and lifecycle. The projections
provide those without discarding the native entries they came from.

**Accept a completion-only summary.** Rejected: it loses exactly the
irreproducible record a timeout or kill needs to leave behind, violating
[0020](0020-durable-when-irreproducible.md).

**Let each runtime construct confinement.** Rejected: it combines the agent
protocol with what the process may touch, weakening [0017](0017-the-box-is-not-the-jail.md)'s
separation and duplicating its canary decision.

## Consequences

- V1 has exactly three direct runtime adapters: Claude, Codex, and Pi. There
  is no runtime discovery, plugin protocol, generic fallback, or unbounded
  future-runtime framework.
- Runtime configuration is explicit and closed. The remaining per-adapter
  configuration decisions are the documented executable/version range, fixed
  argv, packet-input channel, credential environment name, event mapping, and
  any versioned cost conversion. Each is accepted only with the adapter evidence
  above.
- The thread receipt stores native evidence and minimal common projections side
  by side. Its record envelope and event references must be versioned before
  the first write; changing an adapter mapping does not rewrite earlier events.
- Timeout, process-group kill, exit, partial receipt retention, box discard,
  and re-dispatch remain the existing outcomes of [0018](0018-what-survives-a-crash.md)
  and [0026](0026-nothing-external-is-unbounded.md). An adapter cannot resume a
  dead session.
- Credential brokerage for delegated tools, a fourth runtime, runtime selection
  UX beyond the closed value, assessment, budget policy, and automatic landing
  remain deferred. Adding a fourth runtime requires a new decision rather than
  an unreviewed adapter registration.
