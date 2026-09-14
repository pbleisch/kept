---
id: the-pipeline-cannot-reach-a-packet
type: promise
title: The pipeline cannot reach a packet
fields:
  statement: >-
    internal/thread is the one place under internal/ that assembles a packet, and it does not
    reach internal/tick and names no disposition, no outcome or condition a tick decided, no
    rank, no budget and no desire, so the queue has no route into what a repair runtime is
    given.
  gather:
    graph:
      gatherer: go/imports
      scope: ["internal/**/*.go"]
    names:
      gatherer: go/symbols
      scope: ["internal/**/*.go"]
  hold_when: >
    obs.graph.ok and obs.names.ok
    and count(obs.names.defines, d => d.name == "AssemblePacket") == 1
    and count(obs.names.defines, d => d.name == "AssemblePacket"
              and d.package == "github.com/pbleisch/kept/internal/thread") == 1
    and count(obs.graph.packages, p => p.package == "github.com/pbleisch/kept/internal/thread") == 1
    and count(obs.graph.packages, p => p.package == "github.com/pbleisch/kept/internal/thread"
              and "github.com/pbleisch/kept/internal/tick" in p.deps) == 0
    and count(filter(obs.names.refs,
              r => r.package == "github.com/pbleisch/kept/internal/thread"),
              r => (r.path == "github.com/pbleisch/kept/internal/ledger"
                    and count(find_all(r.name, "Dispos|^(Outcome|Condition$|Budget$|Rank$)"), m => true) > 0)
                   or (r.path == "github.com/pbleisch/kept/internal/model" and r.name == "KindDesire")) == 0
  trigger: cadence
  max_age: 7d
  repair:
    scope: ["internal/**/*.go"]
links:
  - { target: policy:the-pipeline-is-never-in-a-packet, relation: serves }
  - { target: decision:0008-work-is-not-an-entity, relation: cites }
  - { target: decision:0009-repairs-are-not-enumerated, relation: cites }
  - { target: decision:0049-three-gatherers-parse-rather-than-scan, relation: cites }
---

`policy:the-pipeline-is-never-in-a-packet` puts the enforcement in the assembly
function rather than in a convention: whatever a packet is made of, it is made
in one place, from an input with nowhere to put the queue. `thread.AssemblePacket`
is that place and `PacketInput` is that input. This is the structural half — it
holds the package that declares both to having no way to *name* the queue, so a
field for a disposition, a rank or a desire cannot be added to `PacketInput`
without the promise going `not-kept`.

**What the pipeline is, in code, today.** A disposition and the outcome and
condition it carries are `internal/ledger`'s types, and `rank` and `budget` are
two of its conditions; `internal/tick` is what decides them, so reaching it is
reaching the queue by another door; and a desire is the kind `model.KindDesire`
names. The name clause reads `Dispos` as a substring, so `ReadDispositions`,
`LastDisposition` and `DispositionPath` fall under it without being listed, and
`Outcome` as a prefix, so every outcome constant does. `internal/thread` reaches
`internal/ledger` by construction — a packet carries observations and a receipt
is ledger records — which is why the ledger's half is a clause over names rather
than an edge.

The first two clauses keep it from being vacuous in a way the policy predicted.
A promise about what `internal/thread` names means nothing the day packets are
assembled somewhere else, so it asserts that `AssemblePacket` is declared exactly
once under `internal/` and that the declaration is `internal/thread`'s; and it
asserts that `internal/thread` was parsed, because a `count(...) == 0` over a
package nobody read is true.

**Why these instruments rather than a named test.** `thread`'s own
`TestPacketContainsOnlyAuthorityAndEvidence` checks the serialized packet against
its allow-list, and it is a good test. Named through `go/tests` it would still be
structural in `document:model`'s autonomy table, but it has to build and run the
suite to answer, so a tree that does not compile — the tree a repair agent stands
in — would read `not-kept` rather than `unknown`; and it sees only what a
marshalled fixture carried, so a new `omitempty` field left empty passes it.
`go/imports` and `go/symbols` parse, answer on a tree with a type error, and read
the declaration rather than one value of it. See
[0049](docs/decision/0049-three-gatherers-parse-rather-than-scan.md).

**What it does not see, said plainly.** Names, not values. `PacketInput.Why` is a
`string` its caller fills — `internal/repair` fills it from the promise's body —
and nothing structural stops a caller filling it with a disposition's detail; a
type `internal/thread` declares for itself that carries a rank under another name
is invisible too, as is a method called off a ledger already in hand. What this
catches is the shape changing: a pipeline type in the input, an edge to the
queue, assembly moving. `packetWhy`'s refusals and the policy, which a repair
reads in every packet, are the authority over the rest.
