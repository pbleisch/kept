# 0007 — Canon is ratified, not observed

**Status** accepted

## Context

An earlier draft treated canon as data that a promise reads — a fact with
provenance, given force by something citing it.

## Decision

Canon is ratified. Declaring something canon is a commitment that anything
contradicting it is a defect, which is exactly what standing means.

```yaml
kind: canon
statement:  Kael's eyes are green
gather:     { text: { scope: "chapters/**" } }
hold_when:  obs.text.hits == 0
```

Canon and fact are opposites, not near-synonyms:

| | Canon | Fact |
|---|---|---|
| | the world must conform | this is what the world appears to be |
| | **ratified** | **observed** |
| on disagreement | the text is wrong | the fact is stale |

## Consequences

- Per-entry rather than one blanket "nothing contradicts canon", so drift names
  the contradicted entry instead of going red somewhere across 400 pages.
- **Most canon has standing and no truth-seeking.** "Valdris is coastal" binds
  and nothing checks it — no `gather`, no `hold_when`. Legal, and normal.
- Fact keeps its own row and its own machinery: provenance, modality
  (`verified` is system-produced only), append-only retraction, scope
  promotion.
- Two promotions, and only one is an authority act: **scope promotion** moves a
  fact somewhere wider and it stays a fact; **ratification** turns an
  observation into a commitment. Conflating them is how a system quietly grows
  its own canon out of things an agent noticed.
