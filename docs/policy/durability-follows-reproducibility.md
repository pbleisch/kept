---
id: durability-follows-reproducibility
type: policy
title: Durability follows reproducibility, not size or type
fields:
  statement: >-
    Retention prunes only what can be gathered again. A record that re-running would not
    reproduce is destroyed by a configured act that names it, never by a prune — and it is
    captured incrementally on every exit path, failures and kills included.
  applies_to: prune
  on_fail: refuse
links:
  - { target: decision:0020-durable-when-irreproducible, relation: cites }
  - { target: decision:0023-what-may-be-captured, relation: cites }
---

The axis people reach for is size, or type, or age. All three are wrong here.
An observation is large, structured and old, and it is prunable — re-run the
gatherer against that world and the same record comes back, byte for byte,
because it is addressed by its data. An agent session is none of those things
and is not prunable: re-running it produces a *different* session, at a
different cost, and what it said the first time is gone.

So the question retention asks is not how big or how old, it is whether the
thing could be produced again. `thread.diff: keep` and
`session.transcript: discard-after-90d` in the config are not two settings of
one kind — the second is a deliberate destruction of something irreproducible,
named as such by a person, and that is the only way an irreproducible record
goes away.

The capture half is the same rule read forwards. A dispatch that writes its
transcript at the end loses everything when it is killed, and a killed dispatch
that cost money and left nothing to attribute it to is the exact failure this
prevents. See [[decision:0020-durable-when-irreproducible]].
