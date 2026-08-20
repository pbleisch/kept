---
id: a-failed-gather-is-unknown
type: policy
title: A failed gatherer yields unknown, never not-kept
fields:
  statement: >-
    A gather that could not run files no observation at all, so no predicate runs and the
    verdict is unknown. A machine that could not measure never records what it thinks it saw.
  applies_to: record
  on_fail: refuse
links:
  - { target: decision:0004-gatherers-return-observations, relation: cites }
  - { target: decision:0036-the-predicate-language, relation: cites }
---

A missing toolchain and a broken promise are both "false" to anything with two
values, and they are two entirely different conversations. One is a machine to
fix; the other is work to do. Collapsing them produces the report that destroys
trust in the whole system fastest: kept says a promise is broken, somebody
investigates, and nothing was ever wrong.

The rule lives in `gather.Record`, which files a measurement and files nothing
for a failure, rather than at each call site. That placement is the policy: a
call site that forgets reports `not-kept` for a machine that was never
installed, and there is no way to see from the record that it forgot.

The predicate language is built on the same split, one level up. A missing
field is a value called Unknown and a wrong expression is an error, and nothing
anywhere tells the two apart by reading a message — which is the whole reason
that language is hand-written. See [[decision:0036-the-predicate-language]].
