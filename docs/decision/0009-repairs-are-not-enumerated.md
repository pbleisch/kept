---
id: 0009-repairs-are-not-enumerated
type: decision
title: 0009 — Repairs are not enumerated
---

# 0009 — Repairs are not enumerated

## Context

adsr picks the cheapest Move whose preconditions hold, from a list authored in
advance. grft scores candidates from a rule registry compiled into the binary,
and states that models may fill typed slots in a rule template but never emit a
rule. Both enumerate the repair space up front.

The system in this lineage that produced the most work — backlog — has no
selection scheme at all. The item states what is wrong; the agent decides how
to fix it.

## Decision

The system identifies the **gap**, not the work. The packet carries the
statement, the `why`, the observation, the scope, and prior attempts. What to
do about that is the agent's.

**How to repair is free. Whether to open work is compiled.**

Ranking and the act/wait/suppress/escalate decision stay deterministic, because
a queue you cannot explain is worse than a queue that is occasionally wrong.

## Consequences

- No move library, no rule registry, no scored candidate selection.
- The compiled parts are what counts as done (`hold_when`) and what may be
  touched (`scope`) — not what to do.
- Learned selection is out of scope: it needs episode volume that does not
  exist, and the cheap version (bandit selection over candidates from recorded
  outcomes) presupposes candidates, which this design does not have.
