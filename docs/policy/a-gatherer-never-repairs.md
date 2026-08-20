---
id: a-gatherer-never-repairs
type: policy
title: A gatherer never repairs
fields:
  statement: >-
    A gatherer observes and changes nothing. It writes no file, runs no fix, and leaves the
    world exactly as it found it.
  applies_to: gather
  on_fail: refuse
links:
  - { target: decision:0004-gatherers-return-observations, relation: cites }
---

The argument is not tidiness of layering. It is that a gatherer which fixes
what it finds cannot be re-run to check the fix, and the moment that is true
its output has stopped being evidence.

Follow it through. A gatherer formats the file it was measuring and reports
that formatting is clean. Run it again and it reports clean again — it would
report clean against any tree at all, because it makes the tree it measures.
The verdict is now a statement about the instrument. Worse, the observation is
addressed by its data, so the second run dedups against the first and the
record looks like a stable world rather than a moving one.

This is also what makes the gate work. The gate re-derives a verdict from the
world after a repair, and it can only do that because gathering is the one
thing in the loop that has no effect. See
[[decision:0004-gatherers-return-observations]].
