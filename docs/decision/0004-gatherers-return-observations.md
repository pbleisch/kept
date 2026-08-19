---
id: 0004-gatherers-return-observations
type: decision
title: 0004 — Gatherers return structured observations; predicates belong to the artifact
---

# 0004 — Gatherers return structured observations; predicates belong to the artifact

## Context

grft's `repo.check` returned `{pass, exit, out}` — a boolean, a number and a
blob. With that shape, "every package has a test file" is the only promise you
can write, and an agent satisfied it by adding files while replacing the one
test that mattered.

## Decision

Three things, not two:

- a **gatherer** runs something and returns a structured observation about the
  world. It does not know what anyone promised.
- a **predicate** belongs to the artifact and says what would satisfy this
  statement.
- **truth** is the result of applying one to the other, derived at read.

## Consequences

- `all(obs.tests.packages, p => p.tests_run >= prior(p).tests_run)` — "never
  shrinks" — becomes expressible. Against a boolean it is not, at any level of
  gatherer sophistication.
- One observation serves many promises, and is cached per tick by gatherer,
  scope and tree state.
- `unknown` falls out of the shape: a failed gatherer produces no observation,
  so no predicate runs.
- **Cost:** this needs a small expression language — comparison, existence, set
  membership, a fold, and `prior()`. That is the one component of this design
  that has not been built before. The escape hatch is always available:
  `gather: { check: { gatherer: command, cmd: "…" } }` with
  `hold_when: obs.check.exit == 0`.
