# 0020 — A record is durable when its act cannot be reproduced

**Status** accepted · corrects the tier rule in `layout.md` and
[0018](0018-what-survives-a-crash.md), which split durable from derived by the
record's type

## Context

`layout.md` sorts storage into what nothing can re-derive (durable) and what
something can (derived), and then applies it by **type**: the ledger is
durable, the index and cache are not. That is right for almost everything and
wrong for the one record that matters most.

grft reached the same rule and then found the exception the expensive way. Its
observability document opens on a day in which the same session transcript was
lost three times for three unrelated reasons, and several diagnoses were made
from evidence that did not exist. The conclusion it drew:

> An effect receipt is derived when the effect is **reproducible**, and durable
> when it is **not**. Reproducibility is a property of the worker, so the worker
> should declare it.

Applied here, the current arrangement is backwards in exactly one place.

**An observation is reproducible.** Gather that world again and the answer is
the same, which is why pruning observations is right and why content addressing
makes a re-gather converge on the file that already exists.

**An agent session is not.** Re-dispatching does not re-derive the transcript;
it produces a *different* repair, at a different cost, with a different diff,
against a box that is gone and a model that is not deterministic. And
`layout.md` gitignores `session.transcript` by default and prunes it after
ninety days — so the least reproducible and most expensive thing kept does is
the one record it discards. Size was mistaken for derivability.

Two related losses follow from the same confusion. grft lost two forty-minute
sessions to a runner that asked for a single JSON envelope emitted only on
completion, taking the transcript, the cost and the turn count with it. kept has
the same shape and worse exposure, because [0018](0018-what-survives-a-crash.md)
makes a dead agent an ordinary path — box discarded, repair re-dispatched — so
unattributed spend would be systematic rather than exceptional.

## Decision

**Durability is decided by whether the act can be reproduced, declared by
whatever performed it, not inferred from the record's type.**

| | reproducible | durable |
|---|---|---|
| observation | yes — re-gather that world | no; prunable, per retention |
| index, cache, box | yes — rebuild, re-cut | no |
| truth, standing, disposition | n/a — they are conclusions | yes |
| **agent transcript** | **no** | **yes, and tracked** |
| **cost** | **no — the money is spent** | **yes** |
| **the packet as given** | **no — its inputs prune and change** | **yes** |
| assessor findings, canary report | no | yes |

Three consequences are the decision:

**Transcripts leave `.gitignore`.** They are content-addressed and referenced by
a small thread record, so the ledger stays small and the payload lives where
`layout.md` already puts payloads.

**Capture is incremental and covers every exit path, including the failures.**
Never a single envelope on completion. Cost is accrued as it is spent, so a
killed dispatch has a cost, a partial transcript, and a turn count rather than
nothing at all.

**The packet is recorded as given, not reconstructed.** It is assembled from a
statement, a `why`, an observation, a scope and prior attempts, and every one of
those can prune or change underneath it. A packet rebuilt six months later is
not the packet the agent saw.

### Discarding is deliberate, and says so

Retention on an irreproducible artifact is not pruning; it is a decision to
destroy the only copy. Config says so — `discard-after` rather than `prune` —
and the thread record keeps the content hash, so an absent transcript reads as
*discarded on purpose* rather than *never existed*. This is 0012's rule for run
records applied to payloads: the reference is durable even when the thing it
names has aged out.

### The test

Adopted verbatim from grft, because it decides the marginal cases better than a
rule about types:

> Could a reviewer reconstruct what happened and judge it, six months later,
> with the box long gone?

Anything that fails that test and cannot be re-run is durable.

## Alternatives

**Keep transcripts derived, because they are large.** Rejected: size is a
storage problem with storage answers, and derivability is a different property.
The assessor's whole job is judging whether a repair was legitimate, and the
transcript is its primary evidence — 0006 puts the transcript in the assessor's
checklist explicitly.

**Store transcripts durably but untracked.** Rejected: it works until the
question is asked on another machine, which is the ordinary case for a system
whose premise is unattended runs.

**Declare reproducibility per artifact role in config.** Rejected: it is a
property of the act, and config would let an operator mark an agent session
reproducible, which it is not.

## Consequences

- **The repository grows with transcripts.** Accepted, with `discard-after` as
  the bound and the dangling hash as the receipt. `kept gc` performs discards
  and records that it did.
- **`layout.md`'s gitignore list shrinks** to genuinely derived tiers: index,
  cache, boxes.
- **Assessor findings are recorded per check**, not as a verdict. The checklist
  has six named questions and *which one fired* is the whole diagnostic value.
- **A crashed dispatch is attributable.** Cost, partial transcript and exit
  cause survive it, which is what makes 0018's discard-and-re-dispatch honest
  rather than lossy.
- **This is a one-way door.** A transcript not captured is not recoverable by
  any later change of mind.
