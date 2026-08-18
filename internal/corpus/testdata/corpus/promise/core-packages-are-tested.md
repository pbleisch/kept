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
  - { target: canon/portrait.png, relation: cites }
  - { target: promise/nothing-is-here.md, relation: cites }
  - { target: "https://example.com/kept/spec", relation: cites }
reviewed_by: the operator
---

Tests that cannot fail are not evidence of correctness. This one serves
[[canon:kernel-correctness]] and also mentions [[canon:absent-from-the-body]].

The three kinds of reference read the same way here as they do in frontmatter:
[[canon/correctness.md]] is a file in the workspace and
[[https://example.com/kept/rationale]] is not in the workspace at all.

A reference inside a fence is a sample, not an edge:

```
[[canon:inside-a-fence]]
```

~~~
[[canon:inside-a-tilde-fence]]
~~~

And `[[canon:inside-a-span]]` is not one either.
