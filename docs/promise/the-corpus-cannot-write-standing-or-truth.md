---
id: the-corpus-cannot-write-standing-or-truth
type: promise
title: The corpus cannot write standing or truth
fields:
  statement: >-
    internal/corpus does not reach internal/ledger, directly or through anything it imports,
    so the package that reads what a person wrote cannot write what a person granted.
  gather:
    graph:
      gatherer: go/imports
      scope: ["internal/**/*.go"]
  hold_when: >
    obs.graph.ok
    and count(obs.graph.packages, p => p.package == "github.com/pbleisch/kept/internal/corpus") == 1
    and count(obs.graph.packages, p => p.package == "github.com/pbleisch/kept/internal/corpus"
              and "github.com/pbleisch/kept/internal/ledger" in p.deps) == 0
  trigger: cadence
  max_age: 7d
  repair:
    scope: ["internal/**/*.go"]
links:
  - { target: policy:standing-and-truth-are-never-in-an-artifact-file, relation: serves }
  - { target: decision:0016-the-ledger-lives-on-its-own-ref, relation: cites }
---

The corpus is what a repair agent may write. The ledger is what it may not.
Keeping the first from reaching the second is what makes that a property of the
code rather than a rule somebody remembers — a `corpus` package holding a
`*ledger.Ledger` is one refactor away from a load path that records something.

`deps` and not `imports`, because reachability is the question. A direct edge
is the obvious break and a path through a third package is the one that arrives
without anybody deciding to add it. The dependency runs the other way today and
that is the shape to keep: `internal/ledger` reaches `internal/corpus` through
`internal/canonical`, because hashing an artifact needs to know what an artifact
is.

The middle clause is the one that keeps this honest. `all` over a list that was
never populated is true, and so is a `count(...) == 0` over one — so the promise
first asserts that `internal/corpus` was actually parsed. Without it, a scope
typo reports a boundary intact that nothing looked at. That is the same false
green [0049](docs/decision/0049-three-gatherers-parse-rather-than-scan.md)
records for the `go list` version of this exact promise, which reported `kept`
because breaking the boundary created an import cycle and an empty list contains
nothing.
