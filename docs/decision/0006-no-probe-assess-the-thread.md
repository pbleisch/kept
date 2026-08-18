# 0006 — No per-promise probe; assess the thread instead

**Status** accepted

## Context

grft attached a **probe** to every promise: a negative control proving the
check could fail, backing an `evidence.can_detect_regression` invariant.

It answers *could this check ever fail?*, which catches a **dead** check. It
does not catch a **shallow** one — a check that can fail and does not mean what
you wanted. The failure that actually cost something was the shallow kind: the
probe reported truthfully that the check could detect a regression while the
check's meaning had been hollowed out underneath it.

A model given a probe as an action reached for it unprompted and reasoned
correctly about proxy quality. So the probe as a *capability* worked; the probe
as a *declared per-promise obligation* added nothing, and doubled the authoring
cost of every promise.

## Decision

No probe field. Three things replace it, and all three are free:

1. **Declared strength at the gatherer kind.** `path` is weak because of what
   it is, not because of a measured rate. Known at authoring time.
2. **A thread assessor** over the ledger entry, with an explicit checklist of
   the ways a thread can pass every gate and still be wrong (see `model.md`).
3. **A never-fired signal** out of the ledger. A promise kept two hundred
   times and never once not-kept is worth looking at. A check that cannot fail shows up
   statistically instead of needing to be proved impossible by construction.

## Alternatives

**Measured strength via a seeded-fault corpus per gatherer.** Considered and
rejected: it is the same activity as the probe with better amortization, and it
becomes a permanent tax that never finishes. Fault injection stays available as
a *gatherer* a promise can name when it specifically wants one.

## Consequences

- **Verification attaches to acts, not to instruments.** An act is bounded, has
  a ledger entry, and is where a change happens.
- The assessor catches instrument substitution from the diff — "this repair
  replaced an existing test file" — which is what the probe structurally could
  not ask, because it only ever looked at the instrument in isolation.
- The pattern to expect: the assessor is where a new failure mode appears; the
  predicate is where it goes to stop recurring. A finding that repeats is a
  predicate nobody has written yet.
