---
id: out-of-scope-paths-are-never-landed
type: policy
title: Out-of-scope paths are reported, never landed
fields:
  statement: >-
    A land carries only paths inside the promise's declared repair scope. Work outside it is
    reported and stays in the box: no promise covers it, so there is nothing to assess and
    nothing that was agreed to.
  applies_to: land
  on_fail: refuse
links:
  - { target: decision:0047-a-promise-binds-what-a-repair-may-touch, relation: cites }
  - { target: decision:0037-a-conflict-is-discovered-by-exhausted-repair, relation: cites }
---

The scope is what a person granted when they ratified the promise. A diff that
reaches outside it is not a worse repair, it is an unrelated change riding on
the authority of a related one — and the assessor cannot help, because there is
no statement for it to judge the work against.

Fail-closed is deliberate and it is the direction that costs something. An
agent that touched one file too many gets nothing landed rather than most of
it, and the operator reads the report and decides. The alternative — land what
is in scope and drop the rest — silently produces a half-repair that the gate
then judges as if it were the whole one.

This is the reason `repair` binds. Widening a ratified promise's scope changes
what may be changed in its name, so it changes the hash and the grant has to be
given again; without that, an agent repairing a promise whose scope reaches the
corpus could widen that promise's own scope, land it, and the next dispatch
would be bounded by a limit nobody granted. See
[[decision:0047-a-promise-binds-what-a-repair-may-touch]].
