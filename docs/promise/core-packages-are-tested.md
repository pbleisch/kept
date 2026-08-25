---
id: core-packages-are-tested
type: promise
title: Core packages are tested
fields:
  statement: >-
    every package under internal/ except internal/confine builds, and every test in it
    passes
  gather:
    tests:
      gatherer: go/tests
      scope: "internal/**"
      packages: "./internal/..."
      exclude: "./internal/confine"
  hold_when: obs.tests.ok and count(obs.tests.packages, p => p.ok) > 0
  trigger: cadence
  max_age: 7d
  repair:
    scope: "internal/**"
links:
  - { target: docs/decision/0004-gatherers-return-observations.md, relation: cites }
  - { target: promise:the-confinement-suite-passes, relation: cites }
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

`internal/confine` is the one package this does not run, and it is
[[docs/promise/the-confinement-suite-passes.md]] that holds it. Nothing stopped
being checked: both promises gather on the same scope and the same cadence, so
the confinement suite is measured every tick this one is. What the split buys is
that **this** promise can be repaired end to end. Its re-derivation runs the
suite inside the box the repair was cut in, and `internal/confine`'s tests start
real confined children of their own — so the exception is named here, bounded to
one package, rather than left as a promise the loop quietly cannot close.

The exclusion is written as a package pattern rather than as a list of the
packages that remain, because a list in a field that binds would go stale
silently: a package added under `internal/` later would be unmeasured and
nothing would say so, which is the same failure `count(…) > 0` is here to
refuse. An exclusion that removes nothing is refused by the gatherer, so the day
`internal/confine` is renamed this reads `unknown` rather than quietly running
the suite it was written to keep out.

The repair bound did not narrow with the statement. A glob cannot negate any
more than a package pattern can, and a repair scope is already always at least
as wide as the bound it stands for — what keeps `internal/confine` out of a
repair is the box's re-derivation measuring this promise, not the scope.

This is the first promise kept holds about kept, so it is also the test of
whether the loop is worth running. See [[docs/self-hosting.md]].
