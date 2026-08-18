# 0005 — A promise names gatherers as a map

**Status:** Accepted

## Context

Some promises need evidence from more than one place — a build result and a
text scan, or the same gatherer over two scopes.

## Decision

`gather` is a map. The key is the binding name in the predicate namespace and
defaults to the gatherer's own name:

```yaml
gather:
  build:    { target: epub }
  chapters: { gatherer: text, scope: "chapters/**" }
  appendix: { gatherer: text, scope: "appendix/**" }
hold_when:  obs.build.ok and obs.chapters.hits == 0 and obs.appendix.hits == 0
```

A single-gatherer promise reads as `obs.tests`.

## Alternatives

**One gatherer per promise.** Forces either a compound gatherer per promise —
which defeats the catalogue — or an artificial split. Often splitting *is*
right, but not always.

**A bare list.** Leaves the predicate namespace ambiguous, and cannot express
the same gatherer over two scopes.

## Consequences

- **If any declared gatherer fails, truth is `unknown`** — not evaluated
  against what did arrive. So declare only what the predicate reads.
- Where several are named, the **weakest** sets the gate (see `model.md`).
  Mixing a `reading` into a promise makes the whole promise wait for a person,
  which is the right answer.
- Guidance: split into separate promises when the parts fail independently and
  would be repaired differently.
