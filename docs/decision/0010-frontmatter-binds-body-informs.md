---
id: 0010-frontmatter-binds-body-informs
type: decision
title: 0010 — Frontmatter binds, the body informs
---

# 0010 — Frontmatter binds, the body informs

**Status** accepted

## Context

A promise needs a one-line statement and a paragraph of rationale. YAML block
scalars make prose unpleasant to edit and ugly to diff; putting everything in
the body means parsing prose for structured values.

## Decision

Structured fields that determine the obligation — `statement`, `gather`,
`hold_when`, `trigger`, `scope` — live in frontmatter. Prose that explains
lives in the markdown body.

```markdown
---
id:    promise:core-packages-are-tested
type:  promise
title: Core packages are tested
fields:
  statement: every package under internal/ has a test that runs and passes
  gather:
    tests:   { scope: "internal/**" }
  hold_when: all(obs.tests.packages, p => p.tests_run > 0 and p.failed == 0)
  trigger:   on-change
  repair:    { scope: "internal/" }
links:
  - { target: canon:kernel-correctness, relation: serves }
---

Tests that cannot fail are not evidence of correctness. A coverage number went
up once while [[canon:confinement-canary]] lost all of its coverage.
```

`title` is short and produces the slug. `statement` is the binding one-liner.
The body is the `why`, and it is what a repairing agent is given.

**When a kind genuinely needs two prose blocks**, use H2 sections in the body,
parsed by heading, with the schema naming which sections it expects so they can
be linted. Rule: an unsectioned body is the kind's primary prose field; a
sectioned body yields named fields.

## Consequences

- Prose is real markdown — editable, diffable, highlighted, and it can carry
  wiki-links.
- Combined with 0003, the split has a second use: frontmatter is what the hash
  usually covers, so the rationale can be rewritten without dropping standing.
