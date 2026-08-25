---
id: the-confinement-suite-passes
type: promise
title: The confinement suite passes
fields:
  statement: internal/confine builds, and every test in it passes
  gather:
    tests:
      gatherer: go/tests
      scope: "internal/**"
      packages: "./internal/confine"
  hold_when: obs.tests.ok and count(obs.tests.packages, p => p.ok) > 0
  trigger: cadence
  max_age: 7d
links:
  - { target: policy:the-box-is-not-the-jail, relation: serves }
  - { target: docs/decision/0017-the-box-is-not-the-jail.md, relation: cites }
---

`internal/confine` is where the box is cut and where the canary proves it holds,
so this suite is the evidence every repair rests on. It is gathered every tick,
on the same scope and the same cadence as
[[docs/promise/core-packages-are-tested.md]], which runs every other package
under `internal/`. The scope is `internal/**` rather than
`internal/confine/**` because that is what this measurement depends on: these
tests compile against half the tree, and an edit in `internal/box` can break
them.

**This promise names no repair scope, so it authorizes no repair, and that is
deliberate.** A promise with no `repair` block bounds nothing, and both
`internal/repair` and `internal/thread` refuse to open a thread for one — the
refusal is the mechanism and there is no second one. Whoever reads this because
a tick reported drift: nobody is going to dispatch an agent at it. Fix it by
hand, outside a box.

The reason is not caution. These tests start real confined children, so
re-deriving this promise inside a repair box asks the provider to sandbox a
process that is already sandboxed. And the thing that makes them evidence is
where they run: a box under the operating system's temporary tree proves
nothing, because the provider's baseline grants that tree write access
recursively and every escape probe passes having measured nothing. So the suite
cuts its boxes and plants its canary sentinels **under the operator's home** —
`makeCanaryState` and `ungrantedRoot` both say so in those words — precisely
because the baseline grants nothing there.

That is what closes the door. A repair box grants three paths — the promise's
repair scope inside the box, and a temporary directory and a session home both
beside it — and all three are discarded when the session ends. The suite's own
repositories and sentinels are in none of them: `ungrantedRoot` puts a
repository under `~/.cache` and `makeCanaryState` cuts its state directly under
the home. Re-deriving this promise in a box therefore means adding a grant
there — write access under the operator's home, for a confined agent, to the one
place the suite depends on nothing being granted. That does not measure
confinement so much as dismantle it. See [[docs/open-questions.md]] question 11,
which this is the answer to, and
[[docs/decision/0017-the-box-is-not-the-jail.md]].
