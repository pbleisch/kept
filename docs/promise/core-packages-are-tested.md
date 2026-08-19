---
id: core-packages-are-tested
type: promise
title: Core packages are tested
fields:
  statement: every package under internal/ builds, and every test in it passes
  gather:
    tests:
      scope: "internal/**"
      packages: "./internal/..."
  hold_when: obs.tests.ok and count(obs.tests.packages, p => p.ok) > 0
  trigger: cadence
  max_age: 7d
  repair:
    scope: "internal/**"
links:
  - { target: docs/decision/0004-gatherers-return-observations.md, relation: cites }
---

Tests that cannot fail are not evidence of correctness, so this promise is
about two things and not one. `ok` is every package building, running and
passing; `count(obs.tests.packages, p => p.ok) > 0` is that anything ran at all
— an empty conjunction is true, and a pattern that matches no package would
otherwise read as a corpus in perfect health.

It is scoped to `internal/**` because that is what it is about. The scope is
also what makes it due: edit a file under `internal/` and the next tick
re-derives this; edit a decision record and it does not, because nothing this
promise looks at moved. `max_age: 7d` is the other half — an answer a week old
is sought again whether or not anything changed, which is what keeps a verdict
from standing on evidence nobody has refreshed.

This is the first promise kept holds about kept, so it is also the test of
whether the loop is worth running. See [[docs/self-hosting.md]].
