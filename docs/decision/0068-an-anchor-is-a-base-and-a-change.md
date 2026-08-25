---
id: 0068-an-anchor-is-a-base-and-a-change
type: decision
title: 0068 — An anchor is a base and a change, and a land takes the change
fields:
  statement: >-
    A repair box is cut from the world the promise was measured in and an
    uncommitted workspace is replayed into it, so an anchor is a pair — a base
    commit holding the tree the repair started from, and a commit holding the
    change; a land merges the change with that base as the merge base, and what
    reaches the target has the target's tip as its only parent.
links:
  - { target: decision:0067-a-repair-anchors-what-it-produced, relation: extends }
  - { target: decision:0015-landing-is-a-merge-queue, relation: extends }
  - { target: decision:0060-a-repair-is-one-promise-a-person-named, relation: extends }
  - { target: decision:0033-due-is-a-digest-over-the-scope, relation: depends-on }
  - { target: decision:0059-an-out-of-scope-path-is-found-in-the-diff, relation: depends-on }
---

# 0068 — An anchor is a base and a change, and a land takes the change

## Context

[0067](0067-a-repair-anchors-what-it-produced.md) made the anchor a commit and
said what it is for, in one sentence: *the commit's parent is the commit the box
was cut from, so the commit's own diff against its parent is exactly the change
a person reviewed.* **The anchor is already a pair.** What reads it does not
treat it as one — a land merges the anchor, and merging a commit takes its
ancestry with it.

### It is measured, and it fails in the direction that looks like a scope defect

`repair.cut` resolves `git rev-parse HEAD`, so an anchor's parent is the
operator's checkout. When that checkout is **ahead** of the landing target, the
merge brings the operator's own unlanded commits along with the repair, and
`internal/land/land_test.go` pins what happens:
`TestAWorkspaceAheadOfTheTargetCannotSmuggleItsOwnCommitsIntoALand` asserts a
refusal — on **scope**, naming `unlanded-local-work.txt`, which is a path the
promise never bound.

It fails closed, which is right, and it reports the wrong thing, which is not.
The refusal names a stray path and leaves a person to work out that the cause
was where they were standing. `AGENTS.md` records the gap and names the intended
fix: [0015](0015-landing-is-a-merge-queue.md)'s third deployment constraint,
*boxes are cut from `origin/main`*, which is unmet.

### Cutting boxes from the target is the wrong fix

It does remove that symptom. What it costs is larger than what it buys, in three
ways, and the third is the one that decides it.

**A repair would be measured in a tree nobody reported drift in.** The drift was
observed in the operator's workspace; a box cut from the target is a different
world. Where the checkout is behind the target the promise may not be broken
there at all, and the agent is dispatched against a gap that is not in front of
anyone.

**It cannot express a dirty workspace.** The workspace diff is `HEAD → working
tree`; replaying it onto a different base gives a tree that is neither the
operator's nor the target's. So cutting from the target forecloses
[0015](0015-landing-is-a-merge-queue.md)'s uncommitted case permanently rather
than incidentally.

**It removes one instance of the ancestry problem and leaves the problem.** The
moment a box is cut from anywhere the target has not seen — which a dirty repair
requires — merging the anchor publishes that place again. A fix that has to be
true of every box forever is a constraint on deployments; a fix in how the
anchor is *read* is a rule.

## Decision

**An anchor is a pair: a base commit and a commit holding the change. A land
merges the change with the base as the merge base, and what reaches the target
has the target's tip as its only parent.**

### A box is cut from the world the promise was measured in

Not from the target, and not from `HEAD` because `HEAD` is convenient — from the
world the observation that opened the thread was made against, which is the
operator's workspace.

**A clean workspace** is that commit, exactly as today, and nothing changes.

**A dirty workspace** is committed into the box first, and that commit is the
base. The snapshot is **kept's own walk and nothing else**: every path
[0033](0033-due-is-a-digest-over-the-scope.md)'s walk counted is written into the
box, and every path in the cut commit that the walk did not count is removed
from it. Not `git add -A`, and the difference is not cosmetic — git honours
`.git/info/exclude` and a global `core.excludesFile` and 0033 deliberately does
not, so a tree built by staging would differ from the tree kept digests, and the
one property this is for would silently not hold.

That property: **the box's tree digests identically to the operator's
workspace.** 0033's digest is over content and workspace-relative paths, and
`workspace.TestTwoCheckoutsOfOneTreeAgree` is the standing measurement that two
copies of one tree agree. So the measurement made in the box is a measurement of
the tree the person is looking at, its observations are the same observations by
address, and they dedup with the tick's rather than forking the series.

The base commit is authored by the operator under 0067's rule, is on no branch,
and is held against `gc` by `refs/kept/threads/<thread>` exactly as the anchor is
— it is the anchor's parent, so the existing ref already holds both.

### A land takes the change, and the change is the diff against the base

```
base  = the anchor's parent   the tree the repair started from
ours  = the target's tip
theirs = the anchor           the tree the session left
```

A three-way merge with that base yields *the target plus the change* and nothing
else. This is measured rather than argued. In a repository where the operator's
workspace held an uncommitted edit to `wip.txt`, a repair to `src.txt`, and a
target that had moved on:

| | `wip.txt` | `src.txt` | `other.txt` |
|---|---|---|---|
| merge the anchor, base computed by git | **the operator's uncommitted edit** | repaired | the target's |
| merge with the base stated | the target's | repaired | the target's |

and the same rule on the *ahead* case drops `local.txt` — the operator's own
unlanded commit — from the result entirely, which is the refusal
`TestAWorkspaceAheadOfTheTargetCannotSmuggleItsOwnCommitsIntoALand` currently
pins. `git merge-tree --write-tree --merge-base <base> <ours> <theirs>` states
the base directly and reports a conflict as a non-zero exit, which is
[0015](0015-landing-is-a-merge-queue.md)'s existing `conflict` condition and
needs no new one.

**This is not applying a patch.** 0067 refused that and was right to: a patch
fails on context git would resolve, and it puts kept back to reading diff text,
which [0059](0059-an-out-of-scope-path-is-found-in-the-diff.md) refused. A
three-way tree merge is what `git merge` does; the only difference is that the
base is *stated* rather than computed from the graph, because the graph does not
know which of the anchor's ancestry was private.

### What reaches the target has one parent

The merge result is committed with the target's tip as its **only** parent. The
anchor is never a parent of what lands.

The reason is one sentence: **a parent is reachable, and the base is private.**
It is a snapshot of somebody's uncommitted tree, or the commits they have not
landed, or both. Making the anchor a parent publishes all of it to the target,
where the tree is clean and the history is not — which is the same defect as
before, moved from the tree into the graph, where it is harder to see.

Two things follow and neither is a loss worth the ancestry.

**The link back to the thread lives in the ledger act**, which names the thread,
the artifact, the target, the regime and the arrival. That is where a person
already looks to ask *why is this here*, and it survives a rewrite of the graph.

**Landing the same anchor twice is a no-op rather than a duplicate.** The
three-way merge is idempotent: the second land computes the same base against a
target that already holds the change, both sides agree, and the result is the
target unchanged. An empty result is detectable and is refused as *already
landed* rather than committed.

### What a dirty repair cannot do

Stated here as the mechanism's limits; 0015 states the same limits from the
land's side.

- **The drift may be the operator's own work in progress.** The measurement was
  over the dirty tree, so the gap the agent is given may be a half-written
  function. The repair scope bounds where an agent may write; nothing tells it
  whose work is there, and nothing can.
- **The box is a copy of whatever the tree holds**, including code that does not
  build. A gatherer measures that and reports what it finds; a syntax error is
  `unknown` and not `not-kept`, which is the model's existing answer and not a
  new one.
- **The snapshot is durable and the person did not make it.** Their uncommitted
  work is in the object store, in a commit authored in their name, on no branch,
  under `refs/kept/threads/`. That is what makes the evidence reproducible at
  all — without it a dirty repair's disposition names a world nothing holds —
  and it is a real cost, named below.

## Alternatives

**Cut boxes from the target**, which is 0015's third deployment constraint.
Rejected on the three grounds in the context; the deciding one is that it
forecloses a dirty repair rather than merely costing something.

**Merge the anchor and let git compute the base**, which is what happens today.
Rejected: it is the defect, and it is measured above.

**Cherry-pick the anchor, or apply its diff.** Rejected for 0067's own reason —
a patch fails on context a merge resolves, and reading diff text is what 0059
refused. `cherry-pick` is closer, being a three-way merge with the parent as
base, and it is the same operation performed in a working tree instead of over
trees; the tree form is preferred because a land already works in a box and does
not need a checkout to compute a merge.

**Two parents when the base is already reachable from the target, one
otherwise.** Rejected: two shapes for one act, and a reader has to establish
which one they are looking at before the ancestry means anything. One shape,
always, is what makes *the anchor is not an ancestor of the target* a fact a
person can rely on without checking.

**Refuse to land a repair from a checkout that is not the target.** Rejected: a
checkout that differs from the target is the ordinary state of a person's
machine, not an error to report.

**Refuse to repair a dirty workspace, and say so.** Honest, and rejected. It is
the *commit your work first* bar 0015 says nothing else in the model asks for,
and 0033 built the world digest over a workspace precisely so a measurement is
about the tree in front of a person.

**Stash the operator's work, repair the clean tree, restore.** Rejected on three
counts, any one of which is enough. It rewrites what a person wrote, which
`policy:nothing-kept-writes-rewrites-what-a-person-wrote` forbids outright. A run
that dies between the stash and the restore has taken somebody's work away, and
kept has no crash-safe restore to offer under
[0018](0018-what-survives-a-crash.md)'s own principle. And the drift being
repaired may be *in* the uncommitted work, so stashing it repairs a gap that is
not the one that was reported.

**Record the base on the ledger act instead of using the anchor's parent.**
Rejected: the anchor's parent *is* the base, so a field would be a second place
one answer lives, and the two could disagree after a rewrite of either.

## Consequences

- **A dirty repair writes the operator's uncommitted work into the object
  store**, in a commit they did not make. On no branch, unreachable from any,
  and held only by `refs/kept/threads/<thread>` — but `git log --all` shows it,
  and 0067's *`refs/kept/threads/*` accumulates and nothing prunes it* is now
  two commits per repair instead of one. Pruning still wants the rule 0067 says
  it wants.
- **`internal/land/land_test.go`'s two pinned tests change.**
  `TestAWorkspaceAheadOfTheTargetCannotSmuggleItsOwnCommitsIntoALand` must start
  failing and be replaced by one asserting the land **succeeds** and that
  `unlanded-local-work.txt` did not reach the target;
  `TestAWorkspaceBehindTheTargetStillLands` keeps passing unchanged, and the
  commit the target already held is still reachable because it is the parent.
- **The queued *cut boxes from `origin/main`* work is retired rather than
  resequenced.** What it was for is delivered here, and doing both would put a
  constraint back on deployments that this makes unnecessary.
- **`AGENTS.md`'s note that a repair box is cut at `HEAD` with no dirty
  overlay stops being true**, and with it the working rule that a break must be
  committed before `kept repair` gets past `Holds`. The rule that replaces it is
  narrower and still real: **a break under test must still be committed when the
  proof depends on the box's `git diff` being only the break**, which is what
  `internal/repair`'s answer-key discipline is about.
- **What lands is one commit per land**, so `git log --first-parent` on a target
  reads as a sequence of repairs rather than as merge bubbles. A person asking
  where a change came from reads the ledger act; the graph no longer answers it,
  and that is a deliberate trade rather than an oversight.
- **Nothing in a `hash` list moves**, so the count of artifacts this decision
  de-ratifies through its mechanism is **zero**. The anchor is a ledger record
  and a git ref; no schema mentions either.
- **Three checks that must be able to fire**, per `AGENTS.md`: a merge with the
  base stated leaves the operator's uncommitted edit out of the result where a
  merge without it does not; the anchor is not reachable from the target after a
  land; and a second land of the same anchor is refused as already landed rather
  than committing an empty change.
