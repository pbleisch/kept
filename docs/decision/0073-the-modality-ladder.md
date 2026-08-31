---
id: 0073-the-modality-ladder
type: decision
title: 0073 — The modality ladder, and why measured is not verified
fields:
  statement: >-
    A fact's modality is an ordered ladder of how well the claim is established —
    verified, measured, inferred, claimed, asserted, assumed — with conflicted
    standing outside it; verified is reserved for what a gatherer produced and a
    tick reproduces, and measured is the strongest thing a person may write.
links:
  - { target: decision:0007-canon-is-ratified, relation: extends }
---

# 0073 — The modality ladder, and why measured is not verified

## Context

`_schema/fact.yaml` offered `verified · claimed · inferred · asserted ·
conflicted`, and reserved `verified` for the system: *"verified is produced by
the system only. An agent's claim is claimed."*

`docs/fact-register.md` measured that vocabulary against the only register this
repository actually keeps. `docs/nono-surface.md` classifies 79 rows of provider
surface by evidence, in its own three words — **measured**, **documented**,
**assumed** — and the mapping does not land:

| the register says | the schema offers | |
|---|---|---|
| by construction | `inferred` | fits |
| *"the guide says the opposite"* | `conflicted` | fits, and `conflicts_with` names the other |
| *"from the scout report, not re-verified here"* | `claimed` | fits |
| **documented** — the vendor's schema or guide says so | `asserted` | workable, and it reads oddly |
| **measured** — 32 of 79 rows, the backbone | **nothing** | `verified` is the system's |
| **assumed** — the whole of section 1 | **nothing** | |

Two of the register's three words have no rung at all, and they are the two that
matter most. **measured** is the strongest evidence in that document: somebody
ran a command on this machine and wrote down what it printed, which is what
`AGENTS.md` means when it says *`nono profile show` on a rendered profile is the
authority, not the guide*. Filing those 32 rows as `claimed` or `asserted`
inverts the register's ordering — it would rank a command that was run below a
guide that was read.

**assumed** is worse, because it is the one a reader is sent to first.
`AGENTS.md` points at *the assumed-and-silent set* before anything else, and the
largest entry in it — *kept pins no nono version and has no compatibility
check* — is a dependency nothing establishes at all. `claimed` is not that: a
claim has a claimant, and an assumption has nobody. There was no way to write
*kept relies on this and nothing behind it is true or false yet*, which is the
finding a register of provider surface exists to surface.

The window is what forces the timing. `fact.yaml` declares `hash: []`, so a fact
bears no standing and nothing is keyed to its content; `docs/fact/` holds only a
`.gitkeep`; and no act in the ledger has ever named a fact. So this costs **zero
re-ratifications and zero migrations today**, and it is free exactly once — after
the first fact exists, changing the vocabulary means rewriting every fact that
used the old one.

## Decision

**`modality` is an ordered ladder, weakest evidence last, and `conflicted` is
not on it.**

```
verified    a gatherer produced it, and a tick reproduces it
measured    a person or an agent ran it once and named the command
inferred    derived from something else already recorded here
claimed     a third party said so and nobody re-ran it
asserted    stated with none of the above behind it
assumed     kept relies on it and nothing establishes it at all
```

`conflicted` says two sources disagree and `conflicts_with` names the other. It
is a relation between two accounts rather than a grade of one, so it stands
beside the ladder rather than at the bottom of it.

**`verified` stays the system's, and `measured` is the ceiling for a person.**
The distance between them is the whole reason both exist, and it is not
confidence: `verified` means *a tick would produce this again*, which is a
property of an instrument, and `measured` means *somebody ran it once*, which is
a property of an occasion. A measurement taken against nono 0.73 says nothing
about 0.80 and cannot say so on its own — that is `nono-surface.md`'s N2, the
register's own largest assumption — whereas a verified fact carries the
instrument that would notice. Letting an author write `verified` would collapse
that distinction in the direction that reads as more certain, which is the
direction a register must not be able to drift in.

Nothing enforces the reservation, and this decision does not add a check. There
is no writer to enforce it against: no verb creates a fact, no gatherer produces
one, and `promote` — the verb that would move one to a wider scope — is
explicitly unwritten because a fact's scope is not modelled. A refusal with no
writer to refuse would be a guard that cannot fire, which this repository counts
as no guard at all. The reservation is stated where an author reads it and where
the first writer will be held to it.

**The default stays `claimed`.** A declared default says what an absent value
means, and absence here means somebody wrote a fact down without saying where it
came from — which is a third party's word taken on trust. Moving the default to
`assumed` would recategorize every future silence as a finding, and moving it up
the ladder would let silence claim evidence nobody produced.

## Consequences

**`documented` is not a rung, and that is a judgment.** The register uses the
word for 30-odd rows and this decision sends them to `asserted`. The
discriminator is the table above: `measured` and `assumed` had **nothing**, and
`documented` has a home that works and reads oddly. A rung is added where a
claim cannot be expressed, not where it can be expressed awkwardly — the same
bound [0036](0036-the-predicate-language.md) puts on the predicate
language's builtins, one kind over. The cost is real: a reader of a migrated
register cannot tell *the vendor wrote this down* from *somebody stated it*, and
the vendor's guide has already been wrong here more than once. **The window is
still open** — it closes at the first fact, not at this decision — so this is
reversible for as long as `docs/fact/` is empty, and reversing it is one line.

**Nothing is migrated.** `nono-surface.md` stays a document. Its trigger is
`fact-register.md`'s and it has not fired: `thread.PacketInput` is
`Promise · Why · Observations · PriorAttempts · Policies`, and
[0061](0061-a-packet-carries-every-instruments-measurement.md) landed since
without adding facts to it. A register of 79 artifacts that no packet reads is
79 files bought for a listing verb, and the migration would cost the risk
ordering — the reason `AGENTS.md` can say *read section 1 first* — which nothing
reconstructs from a directory.

**A future `verified` needs an instrument, not a rule.** The rung is empty and
will stay empty until something can re-derive a provider fact on a tick. That is
the shortest path from this register's weakest property to its strongest, and it
is a want rather than a decision: it would give N2 its answer, because a gatherer
that asks the provider anything can report which version answered.

## Alternatives

**Map `measured` onto `verified` and drop the reservation.** It is the smallest
edit and it destroys the only distinction the field carries. Every one of the 79
rows was produced by a person running a command; if that is `verified`, the word
means *somebody checked* and there is no word left for *kept would notice if this
changed*. The register's own N2 is the proof that those are different: the
profile was measured, and nothing would notice the provider moving under it.

**Leave the enum alone and write the ladder in prose only.** The register would
keep using three words the schema does not admit, which is where this started.
An enum's whole job is to refuse the value nobody meant, and one that refuses two
of the three words its only user writes is refusing the wrong things.

**Add every rung the register might ever want, now, while it is free.** This is
the strongest alternative and the reason `documented` is discussed above rather
than dismissed. It is declined because the freeness argument proves too much: it
justifies every value anybody can imagine, and an enum is a closed set precisely
so that adding to it costs a thought. The window closes at the first fact, and
until then the cost of being wrong here is one line either way.
