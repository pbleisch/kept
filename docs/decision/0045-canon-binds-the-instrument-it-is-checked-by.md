---
id: 0045-canon-binds-the-instrument-it-is-checked-by
type: decision
title: 0045 — Canon binds the instrument it is checked by
links:
  - { target: decision:0003-schema-declares-what-binds, relation: extends }
  - { target: decision:0007-canon-is-ratified, relation: extends }
  - { target: decision:0030-a-link-manifest-is-one-file-per-subject, relation: depends-on }
---

# 0045 — Canon binds the instrument it is checked by

## Context

[0003](0003-schema-declares-what-binds.md) put the binding field list in each
kind's schema, and gave canon the obvious one: *for canon the prose is the
commitment*, so `hash: [statement, body]`. That was written when canon was
assumed to have no gatherer — 0007's own words are *"most canon is prose and has
no gatherer at all"* — and `_schema/canon.yaml` then went on to declare `gather`
and `hold_when` as fields anyway, because some canon does.

Those two facts do not sit together, and the manuscript spike reproduced what
falls out of them end to end. Ratify a checked canon entry; leave `statement`
and the body untouched; replace the pattern with one that matches nothing and
the predicate with the literal `true`:

```
$ kept ratify canon:kael-eyes-are-green --by paul
  content  sha256:04e43018fdb3…

$ kept status
  canon:kael-eyes-are-green                 ratified    2026-08-20 by paul
  promise:every-scene-names-a-pov-character unratified  ratified at sha256:84db361c2ed9,
                                                        and it now says sha256:383f3d19e58f
```

**The identical gutting of a promise dropped its standing in the same command.**
The canon entry, whose instrument had been replaced with one that cannot fail,
is still in force — and there is no diff to notice, because the diff is exactly
the change standing was supposed to be keyed to.

Three things make this worse than a gap in a kind nobody uses yet.

- `model-deck.html` grades a `text`-gathered canon entry **auto-land**, and
  shows one as the headline example of the model over prose.
- The mitigation that would catch it is the assessor's *did the diff touch the
  gatherer, the predicate, or the promise itself?* — and the assessor is
  increment 7.
- **A repair agent may write the corpus.** [0030](0030-a-link-manifest-is-one-file-per-subject.md)
  makes that explicit for manifests, and canon is corpus. So an unsupervised
  repair can weaken the instrument a ratified canon entry is checked by, land
  it, and standing does not notice.

**Canon is serviced by the tick exactly like a promise.** There is no `Kind`
filter and no `Trigger` filter anywhere in `internal/tick`: the spike's run
gathered, evaluated, computed drift and recorded a disposition against
`canon:kael-eyes-are-green` with no special case at all. Canon-with-a-gatherer is
not a hypothetical shape — it is a shape the machine already handles and the
schema does not.

## Decision

**`_schema/canon.yaml` binds `[statement, body, gather, hold_when]`.**

A ratification is granted against a statement *and against the instrument that
tests it*. Where a canon entry carries a gatherer, changing that gatherer or its
predicate changes what was granted, so it drops standing and a person grants it
again — which is what the same edit to a promise already does, for the same
reason, under the same decision.

Canon keeps its own shape where its shape differs. `established_in` records
where a statement was first committed to, which is provenance rather than
commitment, so it is declared and does not bind. The body still binds, because
for canon the prose is the commitment; that is 0003's original judgement and
nothing here disturbs it.

### It is free exactly now, and this is why that is worth saying

Under 0003, changing a schema's `hash` list is a **bulk re-ratification**: every
artifact of the kind is re-hashed and drops standing at once. **There are zero
canon artifacts — in `docs/`, in `internal/*/testdata`, anywhere.** So the cost
today is nothing, and it grows with every canon entry ever ratified, in this
repository and in every corpus anyone runs kept against.

That is the same *free exactly now* argument that ordered the ledger's stream
rename ahead of the first record it would have had to rewrite
([0039](0039-the-ledger-records-acts.md)), and it is recorded here because a
later reader looking at a hash-list change — the one thing 0003 says is a bulk
re-ratification — will reasonably ask why it was thought safe. It was safe
because it was made before the thing it would have cost anything to change
existed, and it stops being available the moment somebody ratifies a canon
entry.

## Alternatives

**Rule that canon carries no gatherer at all**, and delete `gather` and
`hold_when` from the schema. This is the tidier fix and it is what 0007's *"most
canon has no gatherer"* invites, and it is wrong on the evidence. The tick
services canon exactly like a promise, with no `Kind` or `Trigger` filter
anywhere in `internal/tick` — so this alternative does not remove a capability
from the design, it removes a *declaration* from the schema and leaves the
capability working through undeclared fields, where nothing checks it and
nothing can bind it. It would also make `model-deck.html`'s headline example of
the model over prose — *canon · Kael's eyes are green · text · auto-land* —
false, and the manuscript spike found the checked canon entry to be the natural
way to write four of its five.

**Leave it and catch it at the assessor.** Rejected on sequence: the assessor is
increment 7, the auto-land gate is increment 6, and the cost of the fix rises
monotonically from today. It is also the wrong layer — the assessor reports and
does not block ([0006](0006-no-probe-assess-the-thread.md)) — so it would be a
finding about a change that had already landed.

**Bind the whole file, as a document-hash.** Rejected for 0003's original
reason, which has not changed: fixing a typo in prose that informs would drop
standing, and canon already binds the prose that commits.

## Consequences

- **Zero artifacts are re-hashed**, today. The ratified count is unchanged
  across the schema change — 44 before and 44 after — because none of them is a
  canon entry.
- **A ratified canon entry's instrument is now covered by the hash**, so the
  repair agent's write access to the corpus can no longer reach it without a
  person granting standing again.
- **A guard that fires**, per `AGENTS.md`: the spike's gutting, run as a test —
  a canon entry's pattern replaced with one that matches nothing and its
  predicate with the literal `true`, asserting the hash moves, each half
  separately as well as together. It reads `docs/_schema/canon.yaml` rather than
  copying it, so it goes on guarding the file that actually decides what a
  ratification is granted against.
- **Two things this deliberately does not fix, in canon.** `trigger`, `max_age`
  and `repair` are read from any artifact's frontmatter by `corpus.project()`
  regardless of kind, so a canon entry may carry them and they work — and canon
  declares none of the three. They are not unbindable: a hash list may name a
  field the schema does not declare, and it binds. They are simply undeclared,
  so nothing validates them, and unbound, so a checked canon entry's heartbeat
  and its repair scope can still be changed without re-ratification. Fixing that
  is a decision about which fields a kind must declare, which is wider than this
  one and is not free in the same way for every kind it would touch.
- **The same shape exists in two other schemas and is reported rather than
  fixed here**, because each is a separate question about which fields a kind
  must declare and bind. Neither is deferred on cost — the counts are given,
  because a cost stated without its count is a deferral nobody checked:
  - `_schema/promise.yaml` binds `[statement, gather, hold_when, trigger]` and
    declares `max_age` and `repair` without binding either. `repair.scope` is
    what a repair agent may write, so widening it on a ratified promise widens
    what an agent may change and standing does not notice. This is the sharper
    of the two and it is a deliberate current state rather than an oversight —
    `internal/schema`'s own test asserts that a promise does **not** bind on
    `repair`. **The corpus holds two promises**, both ratified, so closing it is
    two `kept ratify` commands rather than a migration.
  - `_schema/policy.yaml` binds `[statement, gather, hold_when, applies_to]` and
    declares `on_fail` without binding it. Flipping `refuse` to `warn` turns a
    policy that stops an act into one that only records it, with standing
    intact. **There are zero policy artifacts**, so this one is free exactly the
    way canon's was — but it is a different question from canon's, because
    `on_fail` is what the verdict *does* rather than how it is reached, and the
    window closes when `self-hosting.md`'s invariants move into the corpus as
    policies.
- **`decision`, `desire`, `document` and `fact` are correct as they stand.** A
  decision binds `[statement, body.decision]` on purpose
  ([0041](0041-a-decision-is-a-kind.md)); the other three bind nothing because
  they have no standing to lose.
