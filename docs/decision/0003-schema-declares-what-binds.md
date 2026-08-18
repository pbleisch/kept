# 0003 — The schema declares which fields are binding

**Status:** Accepted

## Context

Standing is keyed to a content hash (0001). Hashing the whole file means fixing
a typo in the prose drops standing, which is wrong. Hard-coding a list of
binding fields in the tool means every kind binds the same way, which is also
wrong — for canon the prose *is* the commitment.

## Decision

Each `_schema/{type}.yaml` declares the fields that participate in the hash:

```yaml
# _schema/promise.yaml
hash: [statement, gather, hold_when, trigger]

# _schema/canon.yaml
hash: [statement, body]
```

Reserved names: `body` for the whole markdown block, `body.<section>` for one
H2 section, `links.<relation>` for links of one relation type — so a policy
that binds on the document it cites drops standing when that citation changes,
while a promise citing canon for context does not.

The hash is over the canonical serialization of the named `(field, value)`
pairs, not raw bytes, so reformatting and key order do not matter.

## Consequences

- Prose can be rewritten freely; a change to `hold_when` drops standing.
- **Changing a schema's `hash` list is a bulk re-ratification.** Every artifact
  of that kind gets a new hash and drops standing at once. Accepted: you have
  changed what counts as the same promise. It fails loudly — the next tick
  shows the whole kind as unratified.
