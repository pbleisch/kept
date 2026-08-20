---
id: standing-is-created-only-by-ratification
type: policy
title: Standing is created only by ratification, and only by a person
fields:
  statement: >-
    Standing is created only by ratification, and only by a person. No rule, no agent, no
    ingested document and no inference mints it, and an act that cannot name the person
    performing it is refused rather than attributed.
  applies_to: ratify
  on_fail: refuse
links:
  - { target: decision:0001-standing-and-truth-live-in-the-ledger, relation: cites }
  - { target: decision:0048-the-operator-is-named-and-never-guessed, relation: cites }
---

This is the one act the model reserves to a person, and it is the only route to
standing there is. Everything else in kept derives: truth is observed, drift is
computed, the register is a fold. Standing is granted, and a grant nobody made
is not one.

Two ways it erodes, and they look nothing alike. The first is a code path that
mints a ratification for a reason — a promise that has held for a month, an
agent that reports success, a document ingested from somewhere that claims
authority. That is the path 0001 forbids and the reason `Ledger.Ratify` is the
only constructor: there is one call site to audit, and it is reached from the
verb a person types.

The second is quieter and is what [[decision:0048-the-operator-is-named-and-never-guessed]]
found: the act happens, and the record names a guess. An operating-system
username, or the literal `unknown`. The grant is real and the accountability it
was supposed to carry is gone, permanently, in an append-only stream. So the
refusal here covers both — no inferred grant, and no inferred grantor.
