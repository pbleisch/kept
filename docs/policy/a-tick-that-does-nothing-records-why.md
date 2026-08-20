---
id: a-tick-that-does-nothing-records-why
type: policy
title: A tick that does nothing still records why
fields:
  statement: >-
    Every refusal — budget, lock, staleness, policy, rank — is recorded as a disposition
    against the artifact before the tick moves past it. A tick never declines silently.
  applies_to: refuse
  on_fail: refuse
links:
  - { target: decision:0019-a-refusal-is-a-disposition, relation: cites }
---

Truth can be re-derived: observe again and the answer comes back. A refusal
cannot. The budget that was exhausted, the lock that was held, the rank that
put this fourth — none of them is recoverable from the world afterwards,
because none of them was ever a property of the world. They were properties of
one pass, and if that pass did not write them down they are gone.

A queue whose silence cannot be explained is not a queue anyone should trust,
and the failure it produces is specific: an operator sees a drift sitting
untouched for a week and has no way to learn whether kept looked at it and
declined, or never looked at all. Those are different problems with different
fixes and they present identically.

`kept why` is the reader this exists for, and it folds the whole disposition
stream on purpose — the question is about the queue rather than about a tree.
See [[decision:0019-a-refusal-is-a-disposition]].
