---
id: the-box-is-not-the-jail
type: policy
title: The box is not the jail
fields:
  statement: >-
    A worktree bounds what may land and says nothing about what a process may read, where it
    may connect, or what credentials it holds. No agent is dispatched until confinement is
    configured — `unset` is not `none` — and until a canary has verified the provider rather
    than its name.
  applies_to: dispatch
  on_fail: refuse
links:
  - { target: decision:0017-the-box-is-not-the-jail, relation: cites }
  - { target: decision:0024-signing-is-designed-and-deferred, relation: cites }
---

The mistake this exists to stop is a comfortable one: a worktree looks like
isolation, so the isolation question feels answered. It is not. A box says what
may be merged. It says nothing about the network, the filesystem outside it, or
the token in the environment — and a box with push access to the origin makes
the gate decorative, because the agent can simply land its own work.

So confinement is configured separately from the box, and the absent value is
not the permissive one. `unset` means nobody has decided, and a run refuses to
dispatch until somebody has; `none` means somebody decided to run unconfined,
which is a decision a person made and a record can name.

The canary is the same argument one level down. A provider that is named but
not installed, or installed and not working, produces exactly the reassurance
the configuration was supposed to give. So the provider is verified by running
something that must fail if the confinement is real, and trusted by nothing
else. See [[decision:0017-the-box-is-not-the-jail]].
