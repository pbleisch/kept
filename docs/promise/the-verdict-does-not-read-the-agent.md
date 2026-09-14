---
id: the-verdict-does-not-read-the-agent
type: promise
title: The verdict does not read the agent
fields:
  statement: >-
    internal/predicate and internal/tick do not reach internal/thread, directly or through
    anything they import, and name none of the ledger's thread or session records, so what
    decides a verdict has no route to what a repair session said.
  gather:
    graph:
      gatherer: go/imports
      scope: ["internal/**/*.go"]
    names:
      gatherer: go/symbols
      scope: ["internal/**/*.go"]
  hold_when: >
    obs.graph.ok and obs.names.ok
    and count(obs.graph.packages, p => p.package == "github.com/pbleisch/kept/internal/predicate"
              or p.package == "github.com/pbleisch/kept/internal/tick") == 2
    and count(filter(obs.graph.packages,
              p => p.package == "github.com/pbleisch/kept/internal/predicate"
              or p.package == "github.com/pbleisch/kept/internal/tick"),
              p => "github.com/pbleisch/kept/internal/thread" in p.deps) == 0
    and count(filter(obs.names.refs,
              r => (r.package == "github.com/pbleisch/kept/internal/predicate"
              or r.package == "github.com/pbleisch/kept/internal/tick")
              and r.path == "github.com/pbleisch/kept/internal/ledger"),
              r => count(find_all(r.name, "^(Thread|Session)"), m => true) > 0) == 0
  trigger: cadence
  max_age: 7d
  repair:
    scope: ["internal/**/*.go"]
links:
  - { target: policy:the-agents-report-is-never-an-input-to-the-verdict, relation: serves }
  - { target: decision:0006-no-probe-assess-the-thread, relation: cites }
  - { target: decision:0049-three-gatherers-parse-rather-than-scan, relation: cites }
---

`policy:the-agents-report-is-never-an-input-to-the-verdict` says the gate
re-derives from the world and never from what the agent said about its work.
This is the structural half of it. `internal/predicate` is what a `hold_when`
means and `internal/tick` is what gathers and applies one, in a workspace and in
a box alike; `internal/thread` is where a session's words arrive — the adapters,
the supervisor, the receipt. A verdict package that can reach the receipt is one
convenient refactor away from reading *repaired* out of it.

`deps` and not `imports`, because the edge that arrives through a third package
is the one nobody decides to add. `internal/repair`, `internal/land` and
`internal/assess` all reach `internal/thread` and must — a repair dispatches a
session, a land takes its commit from the receipt's anchor, and the assessor
reads the whole entry — and each hands `tick` a tree rather than a report, which
is why they are not in the list. The dependency runs from them to both sides.

**Two instruments, because the receipt has two homes.** The adapters and the
receipt's writer are `internal/thread`, which `imports` answers. The records a
receipt is made of — a native event, a transcript, a cost, an argument — are
kinds declared in `internal/ledger`, and `tick` reaches the ledger by
construction, so no import boundary could keep it from naming
`ledger.ThreadTranscript`. `symbols` answers that half: no reference from either
package to a ledger name beginning `Thread` or `Session`. That is wider than the
agent's own words, and deliberately — it also takes in a thread's anchor and
measurements, and a box re-derivation is *handed* its own lineage's history
rather than reading it ([0080](docs/decision/0080-a-box-folds-its-own-lineage.md)),
so a verdict package naming one of those has stopped being handed its evidence.

**What it does not see, said plainly.** `symbols` reads package-qualified
references, so a value already in hand is invisible: a `MeasureRequest` field of
type `string` that a caller fills with a transcript names nothing, and neither
does a selector off a variable. What this catches is the shape changing — a new
edge, a record kind named where a verdict is decided — and the policy is the
authority over the rest. The same bound is recorded for *standing is minted in
one place* and `.Ratify`.

The count of two is what keeps this honest. `all` over a list nobody populated
is true, and so is a `count(...) == 0` over one, so the promise first asserts
that both packages were actually parsed; a scope typo would otherwise report a
boundary intact that nothing looked at. It held vacuously while
`internal/thread` was a doc comment, and was written once that package carried
something for the edge to reach.
