---
id: the-core-carries-no-domain-vocabulary
type: promise
title: The core carries no domain vocabulary
fields:
  statement: >-
    internal/model, internal/predicate and internal/ledger do not reach internal/gather and
    declare no identifier named for a gatherer. What a promise measures with is the
    catalogue's business, and the core knows only that there are gatherers.
  gather:
    graph:
      gatherer: imports
      scope: ["internal/**/*.go"]
    names:
      gatherer: symbols
      scope: ["internal/**/*.go"]
  hold_when: >
    obs.graph.ok and obs.names.ok
    and count(obs.graph.packages, p => p.package == "github.com/pbleisch/kept/internal/model"
              or p.package == "github.com/pbleisch/kept/internal/predicate"
              or p.package == "github.com/pbleisch/kept/internal/ledger") == 3
    and count(filter(obs.graph.packages,
              p => p.package == "github.com/pbleisch/kept/internal/model"
              or p.package == "github.com/pbleisch/kept/internal/predicate"
              or p.package == "github.com/pbleisch/kept/internal/ledger"),
              p => "github.com/pbleisch/kept/internal/gather" in p.deps) == 0
    and count(filter(obs.names.defines,
              d => d.package == "github.com/pbleisch/kept/internal/model"
              or d.package == "github.com/pbleisch/kept/internal/predicate"
              or d.package == "github.com/pbleisch/kept/internal/ledger"),
              d => d.name == "Command" or d.name == "Tests" or d.name == "Imports"
              or d.name == "Symbols" or d.name == "Corpus") == 0
  trigger: cadence
  max_age: 7d
  repair:
    scope: ["internal/**/*.go"]
links:
  - { target: decision:0005-gatherers-are-a-named-map, relation: cites }
  - { target: decision:0043-the-catalogue-is-closed-and-a-third-party-is-namespaced, relation: cites }
---

The three packages that decide what an artifact is, what a predicate means and
what the ledger holds must not know that `tests` or `corpus` exist. A model
that names one gatherer has a second one arriving next, and then the catalogue
is in three places and the closed list [0043](docs/decision/0043-the-catalogue-is-closed-and-a-third-party-is-namespaced.md)
draws is not a list at all.

Two instruments, because the claim has two halves. `imports` answers the edges:
none of the three reaches `internal/gather`, transitively — `model` imports
nothing in-module at all, and `predicate` imports only `model`. `symbols`
answers the identifiers: none of the three declares a name that is a gatherer's.
`model.GatherSpec` and `predicate.Gathered` are the vocabulary the core is
allowed — that there are gatherers, and that something was gathered — and
neither names one.

**Identifiers rather than lines, and the measurement is why.** Scanned as a
pattern over the same files, this promise reports seventeen hits and every one
of them is a doc comment saying `"tests@2"` on purpose. That measurement is in
[0049](docs/decision/0049-three-gatherers-parse-rather-than-scan.md), and it is
what moved this row from *pattern, assessor must pass* to *structural,
auto-land*: the claim is true of the code and false of the bytes.

`internal/tick` is deliberately not in the list. It imports `internal/gather` by
construction — running the catalogue is what a tick does — and a promise that
included it would be false the day it was written.
