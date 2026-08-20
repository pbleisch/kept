---
id: record-only-what-has-already-happened
type: policy
title: Record only what has already happened
fields:
  statement: >-
    Observe, act, then record. Nothing is written down before the act it records has
    occurred, and a land is pushed before it is recorded rather than after.
  applies_to: record
  on_fail: refuse
links:
  - { target: decision:0018-what-survives-a-crash, relation: cites }
---

A crash is allowed to lose the record of something that happened. It is never
allowed to produce the record of something that did not. The two failures are
not symmetric: the first is a gap somebody can re-derive by observing again,
and the second is a lie in an append-only stream that nothing downstream can
tell from the truth.

The ordering is the whole mechanism, and it is one line in each place it
appears. Push, then record the land. Run the gather, then file the
observation. Write the run's diagnostics before the first record that names
them, because a crash between the two leaves a run with no records — which says
nothing untrue — rather than a record whose writer resolves to nothing, which
reads as diagnostics that aged out.

Writing first is always the tempting order, because it is where the identifier
is already in hand. See [[decision:0018-what-survives-a-crash]].
