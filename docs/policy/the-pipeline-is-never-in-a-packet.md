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

**It binds by being read, and it is checked where it can be.** Packet assembly
is `thread.AssemblePacket`, and its input, `PacketInput`, has no field for a
desire, a rank, a budget, a disposition or queue state.
[[promise:the-pipeline-cannot-reach-a-packet]] holds the package that declares
both to naming none of those and to not reaching the tick that decides them, so
adding one is a verdict rather than a review comment. What it cannot see is a
caller filling a `string` with the queue's words, and that is what this policy,
in every packet, is for. See [[decision:0008-work-is-not-an-entity]].
