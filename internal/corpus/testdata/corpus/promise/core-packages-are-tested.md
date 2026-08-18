---
id: core-packages-are-tested
type: promise
title: Core packages are tested
fields:
  statement: every package under internal/ has a test that runs and passes
  gather:
    tests: { scope: "internal/**" }
    build: { gatherer: command, run: "go build ./..." }
  hold_when: all(obs.tests.packages, p => p.tests_run > 0 and p.failed == 0)
  trigger: cadence
  repair: { scope: "internal/" }
  owner: paul
  review_every: 90d
links:
  - { target: canon:kernel-correctness, relation: serves }
  - { target: canon:nothing-claims-this, relation: cites }
reviewed_by: the operator
---

Tests that cannot fail are not evidence of correctness. This one serves
[[canon:kernel-correctness]] and also mentions [[canon:absent-from-the-body]].

A reference inside a fence is a sample, not an edge:

```
[[canon:inside-a-fence]]
```

~~~
[[canon:inside-a-tilde-fence]]
~~~

And `[[canon:inside-a-span]]` is not one either.
