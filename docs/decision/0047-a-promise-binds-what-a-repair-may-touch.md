---
id: 0047-a-promise-binds-what-a-repair-may-touch
type: decision
title: 0047 — A promise binds what a repair may touch, and a policy binds what its verdict does
links:
  - { target: decision:0045-canon-binds-the-instrument-it-is-checked-by, relation: extends }
  - { target: decision:0003-schema-declares-what-binds, relation: extends }
  - { target: decision:0017-the-box-is-not-the-jail, relation: depends-on }
---

# 0047 — A promise binds what a repair may touch, and a policy binds what its verdict does

## Context

[0045](0045-canon-binds-the-instrument-it-is-checked-by.md) closed one instance
of a class and reported two more rather than fixing them, because fixing them
re-ratifies artifacts that exist:

> `_schema/promise.yaml` binds `[statement, gather, hold_when, trigger]` and
> declares `max_age` and `repair` without binding either. `repair.scope` is what
> a repair agent may write, so widening it on a ratified promise widens what an
> agent may change and standing does not notice.

> `_schema/policy.yaml` … declares `on_fail` without binding it. Flipping
> `refuse` to `warn` turns a policy that stops an act into one that only records
> it, with standing intact.

The count that decides it: **two promises and zero policies.** Closing both is
two `kept ratify` commands, and it is the cheapest it will ever be — every
promise written from here makes it dearer, and nothing makes it cheaper.

**A hash-list change always re-ratifies every artifact of the kind**, including
ones that do not declare the added field. Adding an unused `repair` to a
promise schema and re-reading a promise carrying no `repair` block at all:

```
before   promise:p  ratified
after    promise:p  unratified — ratified at sha256:fd4d97672cb9,
                     and it now says sha256:674e6e921fe5
```

So there is no cheap partial version of this and no reason to wait for one.

**The urgency is increment 5.** Until a repair agent exists, an unbound
`repair.scope` is a gap on paper. From the first dispatch it is a path: an agent
repairing a promise whose scope reaches the corpus can widen that promise's own
scope, land it, and standing does not drop — so the next dispatch is bounded by
a limit nobody granted. The assessor that would catch it (*are the touched paths
in scope, and proportionate to the statement?*) arrives in increment 7, two
increments after the exposure.

It is worst in prose. A manuscript promise's repair scope **is** the corpus,
because the corpus is the manuscript, so the widening is not an unusual
configuration there — it is the ordinary one.

## Decision

**`promise` binds `repair`. `policy` binds `on_fail`. `max_age` stays unbound,
in both, deliberately.**

```
promise   hash: [statement, gather, hold_when, trigger, repair]
policy    hash: [statement, gather, hold_when, applies_to, on_fail]
```

### Why `repair` and not `max_age`, when 0045 named them together

They are not alike, and treating them as one list is what made the gap look
like a single deferred chore.

`repair.scope` **is the commitment.** It is the bound on what may be changed in
this promise's name — the same kind of statement as `hold_when`, expressed over
paths rather than over observations. A ratification granted against a promise
was granted against how far a repair of it may reach.

`max_age` is *"how old this promise's answer may be before it is due again"*.
That is the **checking cadence**, not the commitment. Moving weekly to monthly
does not change what was promised, only how often kept re-asks — and binding it
would mean a person must re-ratify to tune a schedule, which trades a real cost
for no authority anyone was given.

The test is whether the field says *what is owed* or *how kept goes about
finding out*. `repair` is the first; `max_age` is the second; `trigger` is
already bound because it decides whether the artifact is a promise or a policy
at all.

### Why `on_fail`, on the same test

`on_fail` says whether a false predicate **stops an act or is only recorded**.
That is what the policy does when it is broken, which is the substance of what
was agreed to — a policy that refuses and a policy that warns are different
commitments wearing one statement. It is free today at zero policies, and it is
the last of the three that is free.

## Alternatives

**Bind every declared field.** Rejected: it makes the hash list a restatement of
the schema, and it binds `max_age`, which charges a re-ratification for tuning a
schedule. The list exists to say which fields are the commitment, and a list
that names all of them says nothing.

**Wait for the assessor.** Rejected: the assessor arrives two increments after
the exposure, it is one model reading another's work rather than a mechanism,
and 0006 already says verification attaches to acts rather than replacing the
gate.

**Wait until it matters.** Rejected on arithmetic. The cost is monotonic in the
number of promises and the exposure begins at increment 5, so every path that
waits pays more for less.

## Consequences

- **The two promises this corpus holds are re-ratified** as part of this change,
  by the operator. Nothing else in the corpus moves: `canon`, `decision`,
  `desire`, `document` and `fact` are untouched, and there are no policies.
- **A guard that fires**, per `AGENTS.md`: widening a ratified promise's
  `repair.scope` moves its hash, asserted against `docs/_schema/promise.yaml`
  itself rather than a copy, so it goes on guarding the file that decides what a
  ratification was granted against.
- **`max_age` remains unbound and that is now a decision rather than an
  omission.** A later reader finding it unbound will find this paragraph.
- **`internal/schema`'s test changes its assertion**, from *a promise does not
  bind on repair* to the opposite. That test was right about the state of the
  world and is now right about a different one.
- **This closes the class 0045 opened.** Every kind that declares a field
  deciding what is owed now binds it: `promise` its instrument, trigger and
  repair scope; `policy` its instrument, the act it gates and what failing does;
  `canon` its instrument and its body. The kinds that bind nothing have no
  standing to lose.
