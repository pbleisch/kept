---
id: 0067-a-repair-anchors-what-it-produced
type: decision
title: 0067 — A repair anchors what it produced, and a land is handed a commit
fields:
  statement: >-
    A repair commits what its session left in the box and points
    refs/kept/threads/<thread> at it before the box is discarded; a land is handed
    that commit, and the anchor records the commit, its parent and the promise.
links:
  - { target: decision:0015-landing-is-a-merge-queue, relation: extends }
  - { target: decision:0060-a-repair-is-one-promise-a-person-named, relation: extends }
  - { target: decision:0020-durable-when-irreproducible, relation: depends-on }
  - { target: decision:0016-the-ledger-lives-on-its-own-ref, relation: depends-on }
---

# 0067 — A repair anchors what it produced, and a land is handed a commit

## Context

[0015](0015-landing-is-a-merge-queue.md) says a land gates *the box* and merges
it. [0013](0013-the-gate-makes-concurrency-safe.md)'s four nouns say a **thread**
is durable and *holds a box*. Both decisions are written as though the tree a
repair produced outlives the run that produced it.

**It did not.** What increment 5 shipped discards the box at the end of
`repair.Perform` unless `--keep` is given, and nothing anywhere commits in it.
Measured against the code as it stood, a repair left:

| | |
|---|---|
| the thread receipt — packet ref, native events, transcript, cost, turns, lifecycle | no field naming a box, a change, a path or a tree |
| the disposition | a world that is the **operator's** tree digest, not the box's |
| `Change.Diff` | printed to stdout and dropped |
| the box | removed by `git worktree remove --force`; with `--keep`, a registered worktree whose repair is **uncommitted** and whose path is recorded nowhere |
| two `write-tree` trees | unreferenced, collectable, named in no record |

So `cmd/kept`'s own reason for deferring the verb was exactly right and stayed
right: *a land puts a gated diff on main, and no thread produces one yet.*

This is the gap between two decisions that each look complete. 0015 describes
the queue and 0060 describes the repair, and neither says what passes between
them.

## Decision

**A repair commits what its session left in the box and points
`refs/kept/threads/<thread>` at it. A land is handed that commit.**

The receipt records an **anchor**: the commit, its parent, and the promise the
repair was authorized by.

### Why a commit and not the box

A box is a worktree, and keeping one per unlanded repair is a disk cost nothing
bounds — open question 9 already names the box pool as unbounded, and this would
make every repair a permanent claim on it. A commit is the smallest durable
thing git has, it is what a merge takes anyway, and it survives everything: the
box, the run, the machine being rebooted.

The commit's parent is the commit the box was cut from, so **the commit's own
diff against its parent is exactly the change a person reviewed**. Nothing has
to be stored twice, and the report a person read and the change a land gates are
the same object rather than two computations of one thing — which is the split
[0059](0059-an-out-of-scope-path-is-found-in-the-diff.md) already closed once
for the review.

### Why a ref, and why that ref

An unreferenced commit is collectable. The one property an anchor must have is
that git is not free to take it away between the repair and the land, and a ref
is how git is told so.

`refs/kept/threads/<thread>` is under `refs/kept/` because that is where kept
already anchors what it writes and no worktree checks it out. It is **not** a
branch, deliberately: an unlanded repair on a branch is a change that looks
landed to every other tool a person runs.

It is also deliberately outside [0062](0062-a-fetch-writes-an-upstream-ref.md)'s
`refs/kept/ledger*` fetch pattern. An anchor is local work nobody has reviewed,
and a fetch that collected other people's unlanded repairs would be collecting
exactly what has not been agreed to.

The ref name is derived from the thread id rather than stored beside it. A name
that is a pure function of an id the record already carries would otherwise be a
second place for one answer to live.

### The refs are the queue

`kept land` with no argument lists `refs/kept/threads/*`. That is not a
convenience: [`ledger.Store`](../../internal/ledger/store.go) has three
operations and no listing one, and the comment there says why — a fourth would
make the next implementation a filesystem. What can be enumerated is what git
already indexes, and that set is exactly the repairs whose work still exists.

### Anchor whatever the session produced

A repair anchors whenever the session changed the box — repaired or not, bounded
or not. What may land is the land's question: it re-derives every promise the
change touches and refuses on the scope review, and deciding it twice is how the
report and the gate come to disagree.

It anchors on the path where the diff was **too large to read**, too. That is
precisely the repair a person cannot review by eye, and discarding the tree
because the presentation failed would lose the work over a display bound.

### The commit is authored by a person

An agent produced the diff; a person is accountable for it. `AGENTS.md` is
explicit that nothing kept writes may credit a model, an agent or a tool in a
commit, and this is the one commit kept writes that can reach a branch.

kept's own operator first ([0048](0048-the-operator-is-named-and-never-guessed.md)),
then the identity Git itself would put on a commit made here. Failing both, the
repair still runs and reports and is simply **not anchored** — a repair that
refused to run because a machine had never needed an identity would be a larger
cost than a repair that cannot be landed until one is set.

## Alternatives

**The thread's box persists and the receipt names it.** The closest reading of
0013 and 0015 as written, and rejected on the pool: a worktree per unlanded
repair, with a cap and a discard policy nothing has, to hold a tree a single
commit holds for nothing.

**Store the diff as a ledger artifact and have the land apply it.**
`ledger.ArtifactRef` already exists and already holds packets and transcripts.
Rejected on two counts. The kind set is closed on purpose — *not a generic blob
store*, in as many words — so this would open it for a payload that is
reproducible from git, against [0020](0020-durable-when-irreproducible.md)'s own
test. And applying a patch is strictly weaker than merging a tree: it fails on
context git would resolve, and it puts kept back to reading diff text, which
0059 refused.

**`kept land` re-runs the repair.** No handoff at all. Rejected: it contradicts
0015's split — the gate is the expensive half and it runs *outside* the lock —
and it makes a land dispatch a live agent session, which 0060 reserves to a
promise a person named.

**Anchor only a repair that passed its own re-derivation.** Rejected as a second
gate in the wrong place. The land already re-derives, against the tree that
actually lands rather than against the box, and an anchor withheld here would
mean the record of what a session produced depended on whether it worked.

## Consequences

- **`refs/kept/threads/*` accumulates**, one ref per repair whose session
  changed anything, and nothing prunes it. A ref and a commit are small, and the
  set is readable with `git for-each-ref`, but this is a new unbounded tracked
  thing and it is named here rather than discovered later. Pruning wants a rule
  about when a repair stops being interesting, which is the same question the
  box pool has.
- **A repair now writes a commit**, which nothing in kept did before. It is on
  no branch, reaches nothing, and is authored by the operator; but a person
  running `git log --all` will see it, which is new.
- **The anchor is what a land trusts.** It names its own parent, so a land can
  compute the change without the box — and a receipt is JSON anybody's editor
  can reach, which is why the anchor is validated on the way in and its commit
  is checked to be an object id before it becomes an argument to git.
- **A repair on a machine that names nobody is not landable.** It still runs,
  still reports and still records its thread; it simply anchors nothing, and the
  warning says so.
- **Nothing in a `hash` list moves**, so the count of artifacts this
  de-ratifies is **zero**. The anchor is a ledger record and the ref is git's;
  no schema mentions either.
