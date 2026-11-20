---
id: 0093-the-repairers-model-is-selected-as-the-assessors-is
type: decision
title: 0093 — The repairer's model and effort are selected the way the assessor's are
fields:
  statement: >-
    The operator's repair: block may name a model and an effort, as the assess:
    block already may, and `kept repair --model` and `--effort` replace that pair
    for one run. A repair or rework session asks the runtime for them. The two
    blocks are still read separately, so tuning one never tunes the other. Naming
    a model is a request: the model a receipt projects is still read from the
    runtime's own system/init, so it remains an answer rather than an echo.
    Nothing in a promise or in .kept/config.yaml selects a model.
links:
  - { target: decision:0077-the-receipt-says-what-answered-and-what-it-consumed, relation: amends }
  - { target: decision:0081-the-adversarial-pass-argues-in-a-box-it-cannot-write, relation: amends }
  - { target: decision:0078-the-derived-checklist-is-closed-and-the-argument-is-not, relation: depends-on }
  - { target: decision:0048-the-operator-is-named-and-never-guessed, relation: depends-on }
  - { target: decision:0086-the-pass-rules-and-a-rejected-repair-is-reworked, relation: extends }
---

# 0093 — The repairer's model and effort are selected the way the assessor's are

## Context

Thread `2ntpka5qh7ebj0sg6fjvbn5fmw` repaired
`promise:a-receipt-costs-the-ledger-about-what-it-holds` over three attempts and
two runs. Every repair session ran on the runtime's default model,
`claude-sonnet-5`, because kept could not ask for any other. Every adversarial
pass ran on `claude-opus-5` at effort `xhigh`, because the operator's `assess:`
block named them.

The fix the promise needs is a design change of about 1,000 lines across two
packages, built to decision 0087. The repairs reached `kept` on the first
attempt without editing a test the promise names, so nothing in kept's
mechanics blocked them. What failed was the design. Three passes found real
departures from 0087: no read-side dedup, a lock that became load-bearing, reads
that write, and recovery ordered after the read it was meant to serve. Every
attempt also deleted the spool's real-child test of 0084's guarantee. The
hand-built fix, PR 5 (`2a2fc51`), keeps that test and passes all of it.

0077 wrote *kept names no model on the way in*, and gave the reason: the model
a receipt projects is read from the runtime's `system/init`, so that it is an
answer rather than an echo of a request. 0081 then let the `assess:` block name
a model and an effort. It noted that doing so does not change what a receipt
projects, because a session given a name it did not honour still says which
model answered. So the reason 0077 gave survives a named model. Nothing else
kept the repairer from having one. The asymmetry is an accident of order: 0081
needed the lever and the repairer did not yet.

Cost was measured on the same thread, and selection is the lever that moves
it. The repair sessions cost $11.30, $2.67 and $2.40, and the passes $6.02,
$5.34 and $4.78. Opus is priced 2.5 times Sonnet per token. Whether a stronger
repairer spends more per attempt and fewer attempts per episode is something a
person decides promise by promise, and today they cannot make that choice.

## Decision

**The repair: block takes `model` and `effort`, with the same meaning and the
same checks as the assess: block's.** `config.Repair` carries both fields, and
`config.Assess` embeds `Repair`, so the two blocks spell them once and are read
separately. A repair session and a rework session ask for the `repair:` pair. The
pass asks for the `assess:` pair. An unset field is the runtime's own default, as
before.

**`kept repair --model <name>` and `--effort <level>` replace the repair: pair
for one run.** Which model a design-scale repair needs is known to the person who
names the promise, and a run is a person's word (0086). The flags are held to
the adapter's rule before anything is cut. A value that is not one argv value,
or an effort outside the runtime's own closed set, exits 2. A run that asks for
either prints `repair sessions ask for model … and effort …` under its bounds
line. A run that asks for neither prints nothing new, so every golden stands.

**A requested model is a request, and the receipt still answers.** `thread_model`
is still projected from `system/init`, and nothing records what was asked for.
The run's header line and the operator's config say what was asked, and the
receipt says what answered.

**No promise and no tracked file selects a model.** A promise's `repair:` block
is in its hash list, so adding a key there drops the standing of every promise.
A model name is also a fact about the runtime, the credential and the account
of one machine. That is the same reason 0048 keeps `repair.runtime` out of
`.kept/config.yaml`.

## Consequences

**A repairer and an assessor may now be the same model at the same effort.**
0078 asked for independence so that an operator *can* make them differ, and the
two blocks are still read separately. An operator who names the same pair in
both has chosen that, and 0078's caution is theirs to weigh.

**Cost moves with the choice.** At list prices Opus costs 2.5 times Sonnet for the
same tokens. Measured on this thread, the repairer and the passes each took half
of $32.51. A stronger repairer roughly doubles that half per attempt, and it pays
off only if it saves attempts, and the pass that each attempt brings. The
validation runs reported beside this decision measure that trade. Nothing here
makes it automatically.

**The admission evidence is unchanged.** `--model` and `--effort` were already in
the reviewed argv for the pass (0081), and the adapter builds a repair's argv with
the same function. No new argv element exists.

## Alternatives

**Select the model in the promise.** This was rejected because it would
de-ratify every promise, and because a model name is a vendor's word that moves
with the vendor rather than with the repository.

**Select it in `.kept/config.yaml`.** This was rejected for 0048's reason: the
file is tracked, and the runtime and account it would name belong to one machine.

**Escalate a design-scale promise to a stronger model automatically, after an
attempt fails.** This was rejected for now because kept has no measure of design
scale. An attempt that fails says nothing about whether a different model would
have done better, and an automatic switch would spend the most expensive session
on exactly the episodes that are already escalating. A person who names the
promise can decide in one flag.

**Leave it as it was and raise effort on the default model.** That is a subset of
this decision, and `--effort` alone does it.
