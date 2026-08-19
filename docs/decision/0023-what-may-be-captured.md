---
id: 0023-what-may-be-captured
type: decision
title: 0023 — A gatherer returns what it declared; raw output is a bounded receipt
---

# 0023 — A gatherer returns what it declared; raw output is a bounded receipt

**Status** accepted · constrains [0004](0004-gatherers-return-observations.md)
and [0020](0020-durable-when-irreproducible.md), and extends
[0016](0016-the-ledger-lives-on-its-own-ref.md) to artifacts

## Context

Observations are structured gatherer output: content-addressed, tracked, and
permanent in an append-only ledger. Increment 3's first gatherer is `command`,
which runs a program. Build logs carry tokens, environment dumps and connection
strings, and once one is pushed it cannot be taken back — doubly so for a
ledger whose whole design is that nothing rewrites it.

0012 already made this argument about a hostname: *"a hostname in a tracked
record cannot be taken back."* Nobody applied it to the records that carry
arbitrary program output.

Transcripts are the larger surface and pull the opposite way. 0020 requires the
agent transcript to be captured in full, incrementally, on every exit path,
because it is irreproducible and it is the assessor's primary evidence. A
transcript cannot be reduced to declared fields without destroying what it is
for — and a coding agent's transcript contains everything it read.

**And `artifacts/` is under `.kept/`, which is tracked on `main`, which means
every past transcript is inside every box.** An agent repairing one promise can
read every secret a previous agent stumbled across. That is a privilege
escalation path sitting in the layout.

## Decision

### The whole machine-written tier moves off `main`

0016 moved the ledger to `refs/kept/ledger` so no box would contain it. The same
reasoning applies unchanged to what the ledger references:

| | lives on | why |
|---|---|---|
| `.kept/config.yaml` | `main` | authored, reviewed in diffs, changed by a person |
| `.kept/ledger/**` | `refs/kept/ledger` | machine-written; an agent must not write it |
| `.kept/artifacts/**` | `refs/kept/ledger` | machine-written; an agent must not **read** it |

Transcripts stay durable and tracked, so 0020 is untouched. No box contains
one, so no agent can mine the history of previous repairs.

### A gatherer returns declared fields, and nothing else

A gatherer declares its output shape. The observation carries the declared
fields; **undeclared output is dropped rather than stored.** This is 0004's
"structure, not a boolean" taken to its conclusion, and it is where increment 4
is already heading — *the promises in `lineage.md`'s prose example can all be
written without a `command` escape hatch*.

**`command` never stores stdout by default.** It requires an explicit
extractor naming what to pull out. Wanting the whole log means asking for it,
and asking is the moment to think about what is in it.

**An observation has a size cap.** Over it, the gather fails and the verdict is
`unknown`, because a payload that large is the shape a leak arrives in and a
predicate that needed it was reading the wrong thing.

### Failure captures a receipt; success does not

The asymmetry that makes this affordable: raw output is needed when something
breaks, not when it works.

A gatherer that fails writes a **diagnostic receipt** — bounded, redacted,
content-addressed, stored beside the transcripts and pruned on its own
retention. A gatherer that succeeds writes its declared fields and nothing
else.

### Redaction is the second line

Whatever remains — receipts, transcripts, extractor output — is scanned before
it is written, and matches are replaced. **The record says that redaction
occurred and how many matches were replaced**, because a silent redaction is
its own failure: a reviewer who cannot see that evidence was altered will
misread what is left.

Redaction is best-effort by nature, which is why it is fourth and not first.
The first three need no dependency at all; a detector does, and it gets its own
ADR under `AGENTS.md`'s rule when the escape hatch proves unavoidable.
`gitleaks` is the candidate: Go, embeddable, and a maintained ruleset.

## Alternatives

**Redaction alone.** Rejected as the primary control. A miss is permanent and
unrewritable, and relying on pattern matching means accepting that class of
loss for the sake of not designing the output shape.

**Keep transcripts out of the repository.** Rejected by 0020: the question is
asked on another machine, which is the ordinary case here.

**Encrypt artifacts at rest.** Rejected for now: key management is exactly the
UX cost this project is deferring on signatures, and moving the tier off `main`
removes the vector that matters — an agent reading them.

**Store everything and cap nothing.** Rejected: the volume discipline in
`layout.md` exists, and an unbounded observation defeats it as surely as it
defeats the secret rule.

## Consequences

- **The `.kept/**` out-of-scope rule now has two reasons**, and both should be
  said: an agent must not write the ledger, and must not read the artifacts.
- **A gatherer's declaration is part of its contract**, which feeds open
  question 8 — a third-party gatherer over a stdio protocol declares its fields
  the same way a built-in does.
- **Redaction can fire on legitimate content.** A promise about secret handling
  might observe something that looks like a key. The recorded match count is how
  that gets noticed rather than silently corrupting evidence.
- **Residual exposure is a secret in the operator's own side-ref history.** For
  one operator with a private repository that is an acceptable residue; it stops
  being one the moment the repository is shared, and that is when redaction
  stops being optional.
