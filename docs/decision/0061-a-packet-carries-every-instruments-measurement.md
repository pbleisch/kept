---
id: 0061-a-packet-carries-every-instruments-measurement
type: decision
title: 0061 — A packet carries every instrument's measurement, keyed by the promise's own name
fields:
  statement: >-
    A packet carries one measurement per instrument the promise names, keyed by the name the
    promise gathers it under and reads it as. All of them travel, always: which half of a
    multi-dimensional promise is failing is a reading of the evidence, and kept does not make
    that reading on the agent's behalf. The packet's version moves rather than growing a
    second field, so the single-instrument case is the plural case with one key.
links:
  - { target: decision:0052-repair-runtimes-are-closed-adapters-with-native-receipts, relation: amends }
  - { target: decision:0009-repairs-are-not-enumerated, relation: depends-on }
  - { target: decision:0005-gatherers-are-a-named-map, relation: depends-on }
  - { target: decision:0014-an-observation-names-its-world, relation: depends-on }
  - { target: decision:0060-a-repair-is-one-promise-a-person-named, relation: depends-on }
---

# 0061 — A packet carries every instrument's measurement, keyed by the promise's own name

## Context

[0052](0052-repair-runtimes-are-closed-adapters-with-native-receipts.md) froze
the packet as *the artifact's binding content, the `why`, **the observation**,
the bound repair scope, and prior attempts* — one measurement, singular, and
`thread.PacketInput` said so in its type. `internal/repair`'s `subject()`
refused a promise naming more than one outright: *"names 2 instruments and a
packet carries one measurement"*.

That refusal was the right call at the time. Sending an agent half the evidence
is worse than refusing, because the half that arrives looks complete: a packet
carries the promise's whole `hold_when`, so an agent handed one measurement of
two reads a clause it has nothing to weigh and cannot tell whether the silence
means *holding* or *not sent*.

But the shape it refused is ordinary. A rule with two dimensions gets written
with two instruments, and this repository has one:
[`promise:the-core-carries-no-domain-vocabulary`](../promise/the-core-carries-no-domain-vocabulary.md)
gathers `go/imports` under `graph` and `go/symbols` under `names`, because the
claim has two halves — none of `model`, `predicate` or `ledger` *reaches*
`internal/gather`, and none of them *declares a name* that is a gatherer's. Its
predicate reads `obs.graph.ok and obs.names.ok` and four conditions across both.
One instrument cannot answer it, and there is no version of it that one could.

So exactly one of this corpus's seven promises was unrepairable, and it was
unrepairable for a reason about the packet rather than about the promise.

## Decision

**The packet carries a measurement for each instrument the promise names, keyed
by the name the promise gathers it under.** `PacketInput.Observation` becomes
`PacketInput.Observations map[string]ledger.Observation`, and the wire field
`observation` becomes `observations`.

**Keyed, and by the promise's own name.** `gather:` declares `graph` and
`names`; `hold_when` reads `obs.graph` and `obs.names`; the packet carries
`observations.graph` and `observations.names`. The key is already in the packet
twice over — in the promise's binding `gather` map and in its `hold_when` text —
so nothing new is named here, and an agent matches a measurement to the clause
that weighs it by name rather than by guessing at a position in a list. A
`ledger.Observation` carries no key of its own and must not grow one: it is
about the world and knows nothing about any artifact, which is why one
observation serves many predicates ([0004](0004-gatherers-return-observations.md),
[0034](0034-the-observation-record.md)). The key belongs to the packet, which is
where a promise and a measurement meet.

**Every instrument, always — never the failing subset.** kept does not decide
which half of the evidence matters:

- A promise whose clauses are coupled has no failure attributable to one half.
  `count(filter(obs.graph.packages, …), p => … in p.deps) == 0` is about the
  graph, and a promise could as easily read one instrument's output through the
  other's; there is no general rule that says which measurement a false
  conjunction is *about*.
- Even where the failing half is obvious, the half that holds is what says what
  the repair must not break. Sending only what is broken is sending the gap
  without the boundary.
- And choosing is a step toward enumerating the repair.
  [0009](0009-repairs-are-not-enumerated.md) draws that line at the packet:
  authority and evidence travel, an ask does not
  ([0058](0058-the-ask-is-declared-and-travels-beside-the-packet.md)), and
  *"this half is why"* is kept's conclusion rather than kept's evidence.

**kept does not say which half failed, and that is the same decision.** The
agent holds `hold_when` and every measurement, so it can evaluate the predicate
itself; a field naming the failing clause would be an answer where the packet's
job is to state the question. The one place kept *does* name an instrument is a
verdict of `unknown` — `verdict()` reports *"names: the gatherer could not
run"* — and that is not a reading of evidence but the report that there is none.

**The version moves; a second field does not appear.** `PacketVersion` becomes
2 and the singular field is gone rather than kept beside the plural one.
Evidence a reader has to look for in two places is evidence a reader can miss in
one of them, and a packet cannot be reconstructed from a later corpus or ledger
read — so the version is what tells a future reader which shape it is holding.
The single-instrument case is the plural case with one key, in the same field,
under the name its promise already gave it.

**Assembly holds the packet to the promise, both directions.** A gathered name
with no measurement is refused, and so is a measurement under a name the promise
does not gather. The first is evidence dropped between the box and the packet;
the second is data in a packet that nothing in the packet accounts for. Both are
checked at `AssemblePacket` rather than trusted to the caller, which is where
every other packet guard lives.

**`tick.Measure` returns the measurements keyed.** It already ran every
instrument the artifact names and already knew each one's name; it returned an
ordered slice and dropped them. Keyed is what carries the name to the one caller
that needs it.

## Consequences

`internal/repair`'s `subject()` no longer refuses on instrument count.
`promise:the-core-carries-no-domain-vocabulary` is repairable, and every promise
in this corpus now is. The guard that remains is *names no instrument at all*,
which is a promise that can state no gap and check no repair.

The single-instrument path is unchanged in behaviour and changed in bytes: one
promise, one key, one measurement, dispatching exactly as before. What moved is
the field name and the version, which is what a version is for.

**A repair that closes one half and leaves the other fails safe, and the record
says which. Measured, twice.** Two confined Claude sessions were dispatched
against this promise in a copy of this repository, each with a break committed
into the box's own commit.

The first break was one only `names` could see: `internal/model` declaring a
type called `Corpus`, with no new import edge. The session removed it, the
re-derivation read `kept`, and one path changed inside the bound scope. Its own
account names both measurements and is the evidence that the keyed packet is
legible — *"the import-graph half already held (none of model/predicate/ledger
reach `internal/gather`), but `model.Corpus` matched one of the five gatherer
names the promise forbids the core from declaring"*. Nothing told it that. It
read `observations.graph`, read `observations.names`, and weighed the predicate
itself, which is what a complete packet is for.

The second break was one both could see: the same type, plus an import of
`internal/gather` to type a field. The session closed the graph half — dropped
the import, retyped the field — then examined the names half and got it wrong,
concluding *"`model.Corpus` type name is fine since it's a type"*. It is not:
`Corpus` is in the promise's list and `go/symbols` reports a type as a
`defines` entry. The session reported a clean build and a passing suite and
stopped. **The re-derivation read `not-kept → not-kept`, `Perform` recorded
`Unrepaired`, `kept repair` exited 1, and nothing landed** — because the verdict
is the whole predicate applied to the whole tree by `tick.Measure`, reading
nothing the session reported
([`policy:the-agents-report-is-never-an-input-to-the-verdict`](../policy/the-agents-report-is-never-an-input-to-the-verdict.md)).

Both halves of that are worth carrying. A wider packet is a wider invitation and
a promise's `repair.scope` bounds where an agent may write rather than which of
its own clauses the writing satisfies, so a partial repair is a shape this makes
reachable. And it is caught by the thing that was already there. Nothing lands
either way, so the cost is a session rather than a corpus.

**A packet grows with the promise, and nothing bounds that.** Two instruments is
two observations, and `ledger.MaxObservation` bounds each one at a megabyte
rather than bounding their sum. A promise naming several broad instruments could
assemble a packet large enough to be a poor prompt long before it is a poor
record — measured here, `promise:standing-is-minted-in-one-place` alone produces
a 176 KB observation. No cap is added now, because the number to set one against
does not exist yet: the corpus has one two-instrument promise and its packet is
ordinary. If one is added it belongs at assembly, where the refusal can name the
promise.

**A prior attempt still carries only a thread id**, and a receipt still records
the packet as a content-addressed reference rather than as fields. So nothing
downstream reads a measurement out of a packet today, and the assessor of
increment 7 — which reads a receipt to judge whether a repair held — will find
the evidence under the same keys the promise uses, rather than a positional list
whose meaning depends on a map iteration order that was never recorded.

**Map iteration is not an order, and this does not depend on one.**
`encoding/json` sorts a string-keyed map, so a packet assembled twice from one
set of measurements is the same bytes — which content addressing requires, since
two encodings of one packet would file two artifacts for it.

## Alternatives

**Keep the packet singular and pick the failing instrument.** It needs no
version bump and serves the common case where one half is plainly at fault. It
fails outright when the clauses are coupled, it hides the boundary the holding
half describes, and *picking* is exactly the judgement 0009 keeps out of the
packet. Rejected on the third ground: the first two are about how often it is
wrong, and the third is about what a packet is.

**Carry an ordered list rather than a keyed map.** It matches `tick.Measure`'s
existing return and is the smaller diff. It also makes an agent match
measurements to clauses by position, when the promise itself names them — and
the names are already in the packet. A list would be a second, weaker
identification of things that already have one.

**Add `observations` beside `observation` and leave version 1 alone.** No reader
breaks and the plural case is additive. It also means two places evidence can
live, one of which is right for one packet and wrong for the next, and a reader
that checks only the field it remembers. The version exists precisely so this
does not have to be done.

**Name the failing instrument in the packet.** It is one field and it is what a
person would write in a bug report. It is also a conclusion rather than
evidence, it is undefined for a coupled predicate, and the agent can compute it
from what it already holds. Rejected under 0009.

The second measured session above is the strongest argument for it and does not
carry: that session misjudged a clause it could have read correctly, and a field
naming the failing half would have saved it. But what would then be load-bearing
is kept's reading rather than the agent's, and kept has no privileged one — the
predicate is the promise's and the packet already carries it whole. The
misjudgement was caught by the re-derivation, at the cost of a session and
nothing else, which is the outcome the loop is built to produce. A conclusion in
the packet trades an error that fails safe for a dependency that does not.
