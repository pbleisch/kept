---
id: the-agents-report-is-never-an-input-to-the-verdict
type: policy
title: The agent's report is never an input to the verdict
fields:
  statement: >-
    The gate re-derives every verdict from the world. What the agent said about its own work
    is recorded for the assessor to read and is never evidence that the work was done.
  applies_to: land
  on_fail: refuse
links:
  - { target: decision:0006-no-probe-assess-the-thread, relation: cites }
  - { target: decision:0014-an-observation-names-its-world, relation: cites }
---

An agent reporting that it fixed something is a claim about the world, made by
the thing whose work is in question. It is worth recording and it is worth an
assessor reading, and it is not evidence. The gate gathers again, in the box,
against the tree the agent actually produced, and decides from that.

The failure this prevents has a specific shape and it is not the agent lying.
It is an agent that believes it succeeded and is wrong — a fix that addressed
the symptom, a test that was already passing, a file written to the wrong
path. A report is exactly as confident in that case as in the real one.

This is also why an observation names the world it measured. Without that, the
gather before the repair and the gather after it are indistinguishable records,
and a cache serves the second one the first one's answer — which is a repair
that did nothing passing the check written to catch it. See
[[decision:0014-an-observation-names-its-world]].
