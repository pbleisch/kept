# 0001 — Standing and truth live in the ledger, keyed by a content hash

**Status:** Accepted

## Context

An artifact has content, standing and truth. If all three sit in one file, an
edit in vim leaves standing and truth silently stale, and something has to
notice the edit and invalidate them.

## Decision

The file holds content only. Standing and truth are derived on read from the
ledger, keyed by a hash of the artifact's binding content (see 0003):

```
ratified(id, sha256:…, by, at)
truth(id, sha256:…, kept | not-kept | unknown, obs, at)
```

Changing the file changes the hash, so the records stop matching. Nothing has
to invalidate anything; staleness is computed rather than maintained.

## Alternatives

**Standing in the file, keyed by a hash of the content above it.** Workable —
a self-describing checksum. Rejected because the repair agent can write the
file, and therefore could write its own standing. Standing must live somewhere
a writer cannot reach.

**Standing in the file, invalidated by a write hook.** Requires every writer to
run the hook, and fails silently when one does not.

## Consequences

- A hand-edit fails safe and visibly: the artifact drops out of the active set
  and reports `unratified`, rather than silently obliging something new.
- An in-flight thread whose promise is revised has a delegation naming a hash
  that no longer matches, so it stops instead of landing.
- You cannot tell from the file alone whether it is ratified. `kept why <id>`
  and the tick output both say. **Do not** write a `# ratified …` comment back
  into the file; a cache that can disagree with its source is worse than none.
