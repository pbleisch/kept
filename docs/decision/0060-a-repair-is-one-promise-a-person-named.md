---
id: 0060-a-repair-is-one-promise-a-person-named
type: decision
title: 0060 — A repair is one promise a person named, and a tick never dispatches
fields:
  statement: >-
    A repair is dispatched by `kept repair <promise>` and never by a tick; the gap it states
    is measured in the box it will run in, at the workspace's own commit; and the runtime
    that could not be brought to bear is a refusal of its own, recorded beside the jail's.
links:
  - { target: decision:0019-a-refusal-is-a-disposition, relation: extends }
  - { target: decision:0009-repairs-are-not-enumerated, relation: depends-on }
  - { target: decision:0017-the-box-is-not-the-jail, relation: depends-on }
  - { target: decision:0052-repair-runtimes-are-closed-adapters-with-native-receipts, relation: depends-on }
  - { target: decision:0059-an-out-of-scope-path-is-found-in-the-diff, relation: depends-on }
---

# 0060 — A repair is one promise a person named, and a tick never dispatches

## Context

Every part of increment 5 worked and none of it was reachable. The box, the
packet, the confined dispatch, the re-derivation and the scope review were each
measured, and twelve verbs shipped without one that could run them: `kept
repair` answered *"is blocked on a box to work in"*, which had stopped being
true. The whole sequence existed only inside tests.

The obvious place to make it say yes was `internal/tick`'s `dispose`. It is
already the single place a tick decides what to do about a drift, it already
writes the disposition [0019](0019-a-refusal-is-a-disposition.md) requires, and
it always refuses. Letting the drift it sees reach a dispatcher would need no new
verb at all.

**That reading does not survive contact with what a tick costs.** A tick
re-derives every promise whose scope moved, and re-deriving a promise runs its
instrument. Two of this repository's own promises measure with `go/tests` and
`go/vet`, so a tick shells the Go toolchain over `./internal/...` — which
re-enters this suite, which cuts boxes and dispatches live sessions on the
operator's credential. A tick that also dispatched would be a command an
operator runs in a hook, in CI, and on a keystroke, spending a subscription
credential each time on whichever drift happened to be first.

There is a second reason, and it is the design's own: **choosing which of
several drifts to spend a session on is a ranking and a budget**, and both are
increment 7. 0019's sketch has them in it — `disposition = policy(drift)` and
`budget.allows()` — and `rank` and `budget` are already conditions in the
enumeration. Wiring a dispatcher into the tick before either exists is not an
increment arriving early; it is the unattended operation increment 7 exists to
make defensible, arriving undefended.

Three things also had to be settled before a person could type the verb.

**Where the gap is measured.** A packet states a gap (0009), so something has to
establish there is one. The workspace's own verdict is the obvious candidate and
it is the wrong tree: the agent stands in a box cut from a commit, and
[0014](0014-an-observation-names-its-world.md) says an observation names the
world it measured. Evidence about the working tree handed to an agent standing
somewhere else is evidence about somewhere else.

**Which refusals get a condition.** 0019's enumeration was written when the only
thing that could refuse a dispatch was the jail. A repair runtime that is not
configured, is not admitted, reports a version its adapter is not pinned
against, or holds a credential that has lapsed refuses just as hard, from
[0052](0052-repair-runtimes-are-closed-adapters-with-native-receipts.md) and
[0057](0057-the-credential-is-brokered-and-the-box-holds-a-phantom.md) rather
than from 0017.

**Whether a person can see what an agent will be told.** A dispatch spends a
live credential and cannot be undone, and the packet is the whole of what the
session receives.

## Decision

### `kept repair <promise>` is the verb, and it repairs one

One promise, named. With no name the verb lists the promises that read
`not-kept` and exits on a usage error, so several drifts are a choice a person
makes rather than one kept makes for them.

Only `not-kept` and `regressed` are offered. `stale`, `unverified` and `unknown`
all say *nobody has measured*, and the answer to that is a tick rather than an
agent: dispatching on a reading nothing refreshed spends a session to discover
the promise was fine.

**A tick still never dispatches**, and its refusal is unchanged: `refused` on
condition `provider`, naming which of 0017's two states the workspace is in.
That is not a placeholder left behind — a tick verifies nothing and starts
nothing, so neither condition is met in one, permanently and by design.

### The gap is measured in the box, at the workspace's commit

The box is cut at `HEAD` with no dirty overlay, and the promise is re-derived
*there* before anything else happens. That measurement is both the decision to
dispatch and the evidence the packet carries.

It has three consequences worth stating rather than discovering. A promise that
holds at the commit has no gap, so nothing is dispatched and the run says so —
which is also the honest report of drift the working tree carries and the commit
does not. A promise nothing can measure in the box is `unknown`, and an unknown
is not a gap. And the re-derivation is restricted to the one promise, because a
pass over the whole corpus inside a box runs every other promise's instrument
against a tree nobody asked about.

The same re-derivation, run again after the session, is the verdict. Nothing the
session reported is read for it, and the change is reported through
[0059](0059-an-out-of-scope-path-is-found-in-the-diff.md)'s review rather than
by parsing diff text, so the report a person reads and the check a land will
refuse on are one computation.

**Nothing lands.** The box is discarded unless `--keep` says otherwise, the
operator's tree is not written, nothing is committed and nothing is pushed. What
comes back is a bounded diff and a review of it.

### `runtime` is a refusal condition beside `provider`

The repair runtime that could not be brought to bear is recorded as `refused` on
condition `runtime`. It is separate from `provider` because it comes from a
different part of the system, which is 0019's own test for what earns a
condition, and because collapsing the two would make the transition rule blind:
a disposition is a new record when the outcome or the condition moves and never
when only the detail does, so a box whose canary started passing on the day its
credential died would record nothing at all.

`kept repair` writes exactly one disposition, and only when it reached a
decision about acting: `act` when a thread opened, `refused` with its condition
when one did not. A promise that cannot authorize a repair however the machine is
configured — no standing, no bound scope, no instrument, or more instruments
than a packet carries — writes nothing, because every one of those is
re-derivable from the corpus and the ledger at any later time, which is the test
0019 uses to decide what has to be written down.

### `--dry-run` stops one call before the dispatch

It cuts the box, measures the promise, assembles the packet, renders the profile
and **verifies the provider** — the canary and the pre-flight both — and then
does not start a session. It prints the packet.

It is not a second path. It is the same path with the last call missing, and it
is worth having because the thing it stops before is irreversible and costs a
live credential. It is also the only way to answer *is this machine actually set
up* without spending one.

### The deadlines come from `.kept/config.yaml` and nowhere else

`timeouts.agent` bounds the session and `timeouts.git` bounds every Git call a
repair makes. Both were already in the file and read by nothing, which is the
same defect as a knob that does nothing wearing the other face. The canary's
bound is not a key: it bounds kept asking a provider about a policy kept just
rendered, which is identical for every repair on a machine.

## Alternatives

**Dispatch from the tick.** Rejected on cost and on order. A tick is what runs in
a hook and in CI, an ordinary tick of this repository already spends live agent
sessions through its own instruments, and the ranking and budget that make
unattended dispatch defensible are increment 7. The seam is kept open: `dispose`
is still the one place a tick decides, and what it will grow is a policy and a
budget rather than a dispatcher.

**`kept repair --all`, or a repair per drift.** Rejected as ranking by another
name. Order of iteration is not a priority, and a budget that does not exist
cannot refuse the fourth session.

**Measure the gap in the workspace and carry the observation into the box.**
Rejected under 0014: the packet's evidence would name a tree the agent is not
standing in. It also puts the dirty-working-tree question in the packet, where
an agent cannot see it, instead of in the box, where the measurement settles it.

**Carry the workspace's uncommitted work into the box as an overlay.** Not
rejected, not built. `box.Request.Overlay` exists for it and it is the right
shape, but choosing which dirty paths a repair inherits is a decision a person
makes and nothing asks them yet. Until something does, the box holds the commit
and a promise broken only by uncommitted work reads as holding there.

**Reuse `provider` for a lapsed credential.** Rejected: it is the transition the
record would then be unable to report, and 0019's own criterion for a condition
is that it comes from a different part of the system.

**Skip the dry run.** Rejected on the asymmetry. Everything it does is done
anyway on the way to a dispatch, so it costs one branch; what it buys is a look
at the packet before a credential is spent on it.

**Store the diff as a `thread.diff` artifact and record it on the receipt.** Not
built. `.kept/config.yaml` already reserves the retention rule for it, but adding
an entry kind is a change to 0052's receipt shape and this is the increment that
makes a repair reachable, not the one that changes what a receipt holds. The
diff is printed, and `--keep` leaves the tree it came from.

## Consequences

- **A repair is reachable and costs one command.** `kept repair <promise>` cuts
  a box, dispatches under a verified provider, re-derives, and prints a bounded
  diff with the review beside it.
- **A tick is unchanged**, including what it records. An operator who never runs
  the new verb sees exactly what they saw before.
- **A promise with two instruments cannot be repaired.** A packet carries one
  measurement and its shape is frozen at 0052's version; the refusal names the
  count rather than choosing which instrument matters. Widening it is a packet
  version, not an edit.
- **The confinement grant is reported beside the review**, because a grant
  rendered from a glob is always at least as wide as the glob, and a reader of a
  bounded review should be able to see the difference.
- **A repair scope whose first segment is a wildcard grants the whole box.**
  There is no narrower expression of one, and 0059 already says confinement is
  not the enforcement of a scope. It is reported.
- **The box is a worktree in the operator's repository while the repair runs.**
  It is discarded afterwards; `--keep` leaves it, and the run says where it is
  and how to remove it.
