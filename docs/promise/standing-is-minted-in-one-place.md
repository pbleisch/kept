---
id: standing-is-minted-in-one-place
type: promise
title: Standing is minted in one place
fields:
  statement: >-
    Under internal/, only internal/ledger declares the function that constructs a
    ratification, and nothing there opens a ledger of its own to call it with.
  gather:
    names:
      gatherer: symbols
      scope: ["internal/**/*.go"]
  hold_when: >
    obs.names.ok
    and count(obs.names.defines, d => d.name == "Ratify") == 1
    and count(obs.names.defines, d => d.name == "Ratify"
              and d.package == "github.com/pbleisch/kept/internal/ledger") == 1
    and count(obs.names.refs, r => r.path == "github.com/pbleisch/kept/internal/ledger"
              and (r.name == "Open" or r.name == "Ratify")) == 0
  trigger: cadence
  max_age: 7d
  repair:
    scope: ["internal/**/*.go"]
links:
  - { target: policy:standing-is-created-only-by-ratification, relation: serves }
  - { target: decision:0049-three-gatherers-parse-rather-than-scan, relation: cites }
---

`policy:standing-is-created-only-by-ratification` says no rule, no agent and no
inference mints standing. This is the structural half of it: `Ledger.Ratify` is
the only constructor, so there is one call site to audit, and it is reached from
the verb a person types. `cmd/kept` calls it and must — the CLI is where a
person's act enters — which is why the scope is `internal/**` and not the whole
tree.

Three clauses, and each one is a different way the shape erodes. A second
`Ratify` declared anywhere under `internal/` is a second constructor. A
`Ratify` declared under `internal/` that is not the ledger's is the same
constructor moved. And `ledger.Open` named under `internal/` is a package
acquiring a writable ledger of its own rather than being handed one by the
command a person typed.

**What it does not see, said plainly.** `symbols` reads package-qualified
references — `ledger.Open` — and a selector off a variable is not one, so a
package that already holds a `*ledger.Ledger` and calls `.Ratify` on it is
invisible to this promise. That is [0049](docs/decision/0049-three-gatherers-parse-rather-than-scan.md)'s
*no scope analysis on a selector*, and it is why the policy is the authority and
this is the guard: the guard catches the shape changing, and the assessor 0006
defers is what reads a diff that used the shape as it stands.

`obs.names.ok` is the first clause because a scope matching no Go file would
otherwise satisfy every count in it. A promise about what a package does not
declare must not report that it declares nothing when nothing was read.
