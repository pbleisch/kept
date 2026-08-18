# 0014 — An observation names the world it measured

**Status** proposed · corrects the `Observation` and `Truth` shapes established
by [0001](0001-standing-and-truth-live-in-the-ledger.md) and
[0004](0004-gatherers-return-observations.md)

## Context

`model.md` says an observation belongs to the gatherer and is about the world,
and knows nothing about any promise. That is right, and it is why one
observation serves many predicates and why observations survive a revision that
resets an artifact's truth history.

What it does not say is **which** world. `model.Observation` carries a scope —
a path glob, saying which paths were looked at — and nothing saying which tree
they were looked at in.

This is not a concurrency problem and it does not wait for a second run. The
thread loop in `model.md` gathers three times against two different worlds
in a single process:

```
gather again, in the box        # the repaired tree, uncommitted
gather again, on the origin     # the tree that will ship
```

Those two are recorded identically today.

Three consequences, in descending order of how quietly they fail.

**The tick's observation cache.** Dedup is advertised as a feature — run
`tests` once per tick, and every promise about tests reads the same
observation. Keyed by `(gatherer, scope)` alone, the box's post-repair gather
**hits the origin's pre-repair entry**. The thread's "did it work on its own
terms" check then passes by reading a result from before the repair ran. A
repair that did nothing is confirmed by the exact mechanism meant to catch it,
and it is one of the things the assessor checklist explicitly hunts for.

**`prior()` and history folds.** "The set of tests never shrinks" compares an
observation to its predecessor. Across a branch and its origin, that is a
comparison between two trees presented as a change over time.

**Truth series.** Verdicts reached in different trees interleave in one series,
and the fold returns whichever was written last.

## Decision

**An observation names the world it measured, and truth names the world it was
reached in.**

```go
type World struct {
    Remote string // "" for a repository with no remote
    Ref    string // the checked-out ref, or "" when detached
    Commit string // the commit HEAD resolves to
    Tree   string // the tree actually measured — differs from the commit's
                  // tree whenever the working tree is dirty
    Thread string // set when this world is a thread's box rather than a
                  // workspace's own tree
}
```

`Tree` rather than `Commit` alone, because a box mid-repair is uncommitted and
has no commit of its own. The tree is what the gatherer ran against, so the
tree is what identifies the measurement.

Three keying rules follow, and all three are the reason the field exists:

1. **The observation cache is keyed by `(gatherer, args, world)`.** The box
   gather and the origin gather can no longer collide.
2. **The content-addressed observation path includes the world.** Two worlds
   are different content and land in different files, which is the same rule
   0011 already applies, extended to the thing that was missing from it.
3. **A truth series is per `(artifact content hash, world)`.** Verdicts from
   different trees are not comparable and are never folded together. `kept
   drift` answers for the world it is asked in.

## Alternatives

**Leave it out; rely on scope and time.** Rejected. Scope says which paths,
never which tree, and time only separates the box gather from the origin gather
by accident of ordering. Every failure above is silent, and the cache one
produces a false pass rather than an error.

**Identify the world by the box or workspace that measured it.** Rejected, and
the distinction is what makes the rest of the model simpler: a box moves
between commits, so it names a *place* and not a *tree*. Two observations from
one box an hour apart are usually of different worlds, and two boxes at the
same commit are one world and should dedup to one record. Keying on the tree is
what lets [0013](0013-the-gate-makes-concurrency-safe.md) do without any
durable identity for the place at all.

**Record only the commit.** Rejected: a dirty box has no commit that describes
it, and the box is precisely where the interesting gather happens.

**Make the world a property of the artifact or the predicate.** Rejected: an
observation knows nothing about promises, per 0004. The world is a property of
the measurement, which is why it belongs to the observation.

## Consequences

- **Observations no longer dedup across worlds**, so there are more of them.
  Retention is per `(artifact, gatherer, world)`. Accepted as a volume cost for
  a correctness property.
- **"One observation serves many promises" narrows to "many promises in one
  world"**, which is what it always meant.
- **Comparing worlds becomes possible**, and is new. "This promise holds on
  main and not in this box" is now a query rather than an inference, and it is
  the natural shape of the thread gate.
- **Standing needs nothing.** Ratification names a content hash, which is
  world-independent by construction. Only truth is about a place.
- **A repository with no remote works unchanged**, with an empty `Remote`.
- **This is a one-way door and it comes before the first durable write.** A
  record written without a world cannot be backfilled — nothing else records
  which tree it measured — and `prior()` folds over exactly that history.
