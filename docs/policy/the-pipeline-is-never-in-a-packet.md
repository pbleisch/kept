---
id: the-pipeline-is-never-in-a-packet
type: policy
title: The pipeline is never in a packet
fields:
  statement: >-
    A packet carries the gap and the standing that bounds it, and nothing about the queue
    that produced it. There is no code path from a desire to an agent, and the assembly
    function is where that is enforced rather than convention.
  applies_to: dispatch
  on_fail: refuse
links:
  - { target: decision:0008-work-is-not-an-entity, relation: cites }
  - { target: decision:0009-repairs-are-not-enumerated, relation: cites }
---

A desire is a want nobody has committed to. It has no standing, so it obliges
nobody, and an agent that can see one will treat it as work — which is the
whole reason work is not an entity in this model. The queue is kept's own
bookkeeping about what to do next, and an agent reading it starts optimising
against the bookkeeping.

The enforcement point matters as much as the rule. "Do not put the pipeline in
the packet" as a convention holds until the first person who needs one more
field for a good reason; as a property of the function that assembles a packet,
it holds because there is nothing to reach for. Whatever a packet is made of,
it is made in one place.

**This binds by being read, and there is nothing to check yet.** Packet
assembly does not exist — `internal/thread` is a doc comment — so this policy
is authority for whoever writes it rather than a gate over anything running.
That is the ordinary case for a policy: one with no gatherer still binds, and
it is in the packet the moment there is a packet. See
[[decision:0008-work-is-not-an-entity]].
