---
id: 0050-doctor-reports-an-unchecked-instrument
type: decision
title: 0050 — `doctor` reports an instrument nobody checked, and reporting is all it does
links:
  - { target: decision:0042-doctor-adopts-a-corpus, relation: amends }
  - { target: decision:0023-what-may-be-captured, relation: extends }
  - { target: decision:0049-three-gatherers-parse-rather-than-scan, relation: depends-on }
---

# 0050 — `doctor` reports an instrument nobody checked, and reporting is all it does

## Context

[0049](0049-three-gatherers-parse-rather-than-scan.md) opens on a `command`
promise that reported `kept` while the boundary it was about was broken: the
program exited non-zero, printed nothing, and the predicate read the empty
output as an absence of violations. The fix in that promise was one line —
`ok: "exit == 0"` — and the promise author had to think of it.

**That is a class bug in `command`, and 0049 fixes three instances of it.** A
native gatherer's failure is a gather failure, and `gather.Record` files nothing
for one, so the verdict is `unknown` and there is nothing for an author to
remember. But `command` is the escape hatch and it is not going away: a
manuscript author will write `command` promises for years before a native
gatherer exists for their domain, and every one of those promises can be written
with the same hole in it.

Nothing in kept says so. The corpus loads, the extractor parses, the fields are
declared, the predicate checks — and a promise that never looks at `exit` passes
every one of those checks, because it is well-formed. It is not wrong. It is
unchecked, which is a different thing and has no reader.

## Decision

**`kept doctor` reports a `command` gather entry whose declared fields never
reference `exit`: you ran a program and never looked at whether it worked.**

### It is a finding, and never an error

`wc -l` legitimately ignores its exit status, and so does any program whose
output is the measurement and whose failure mode is empty output the predicate
handles. So this is reported and the corpus still loads, `kept list` still says
nothing is wrong, and the exit code is unchanged — the same shape as a filename
that is not a slug, which 0042 puts in neither half of 0027's table.

**Nothing fixes it.** Adding `ok: "exit == 0"` to a promise is writing what the
promise says, which is content, and `--fix` writes identity. The boundary 0042
draws around `--fix` is exactly the boundary that makes this safe to report.

### It fires on nothing in this corpus today

`the-tree-vets-clean` already carries `ok: "exit == 0"`, and it is the only
`command` promise here. A check whose first run reports the corpus it was
written in is a check written to fit, and this one is not.

### It reads the parsed expression, never the text

`exit` is one of three roots an extractor may read, so the question is whether
any declared field's expression references that root — not whether the string
`exit` appears in it. A field named `exit_code`, a string literal `"exit"` and a
comparison against a field called `exit` on some record are all not this, and a
substring match would report all three.

### `doctor`'s remit widens, once, and this says how far

0042 bounds `doctor` to identity — *what names an artifact, and whether
everything the ledger names is here* — and says in as many words that the remit
is what keeps the verb from becoming `kept list` with a second name. This is not
identity. It is admitted anyway, and the line that admits it is narrow enough to
state:

> **`doctor` reports what is well-formed and unwise. `kept list` reports what is
> wrong.**

A schema violation, a broken link and a predicate that will not parse are
defects: something the corpus will not accept, and the artifact's author has to
fix it before the corpus is whole. An unchecked instrument is none of those.
Nothing refuses it, nothing is blocked by it, and it will sit in a ratified
promise reporting `kept` forever. It has no reader anywhere else, and that is
the argument for giving it one here rather than the argument for a third verb.

## Alternatives

**Refuse the promise at load.** Rejected: `wc -l` is a real promise and refusing
it would be kept deciding that a program's exit status is always the
measurement, which is false. It also converts a judgment about wisdom into a
corpus that will not load, which is the heaviest instrument available for the
lightest possible finding.

**Report it in `kept list`.** Rejected on 0042's boundary, from the other side:
`kept list` reports what is wrong with what an artifact says, and this promise
says nothing wrong. Putting it there would make "42 problems" mean two different
kinds of thing, and the count is what a person reads.

**Default `ok: exit == 0` when `fields:` omits it.** Rejected, and it is the
tempting one. It would silently change what a ratified promise measures without
changing its bytes — `gather` is in `promise.yaml`'s hash list precisely so that
cannot happen — and it would make `wc -l` unwritable without an opt-out, which
is a second field to design. 0038 froze the reading of a `run:` line for the
same reason.

**Wait for native gatherers to cover the domains.** Rejected: that is the plan
already, and it is why this is worth doing. Three gatherers cover three domains
and `command` covers everything else, forever. The case this protects is the
case the gatherers do not reach.

## Consequences

- **`doctor` grows a finding that is not about identity**, and the sentence
  above is what bounds the next one. A finding belongs here when it is
  well-formed and unwise, has no other reader, and nothing kept does about it.
- **The check gets a test that deliberately writes the promise without `exit`**,
  and another that asserts the corpus as it stands reports nothing — because a
  check that has never fired is not a check, and one that fires on everything is
  not either.
- **`--fix` is untouched.** It writes identity, and this finding is left alone
  with the reason, the same as a duplicate id.
- **The exit code is unchanged.** 0027's rows are about commitments and
  evidence, and an unchecked instrument is neither.
- **Nothing in a `hash` list moves**, so the count of artifacts this
  de-ratifies is **zero**.
