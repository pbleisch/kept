# 0008 — Work is a ledger construct, not a corpus entity

**Status** accepted

## Context

Work has standing (delegated) and seeks truth, so it looked like the other
kinds and an earlier draft gave it a schema.

## Decision

Work is not a corpus entity. Nobody authors a work item — its content is
derived from a promise and an observation — so it lives in the ledger:

```
work w-5f30:
  serves:   promise:core-packages-are-tested @ sha256:ab12…
  standing: delegated
  trigger:  once
  repair:   { scope: "internal/" }        # inherited
```

The authored kinds — `promise`, `policy`, `canon`, `fact`, `document`,
`desire` — are corpus entities.

## Consequences

- The corpus does not fill with dead work items.
- The delegation names the promise **and its content hash**, so revising the
  promise mid-thread leaves the work's authority pointing at a statement that
  no longer exists, and the thread stops rather than landing.
- The durable record of an attempt is the ledger's thread entry — packet, diff,
  transcript, findings, cost — not a file.
