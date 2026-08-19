---
id: 0035-a-verdict-names-its-world-in-the-record
type: decision
title: 0035 — A verdict names its world in the record, and a disposition names it without being keyed by it
---

# 0035 — A verdict names its world in the record, and a disposition names it without being keyed by it

**Status** accepted · resolves how [0014](0014-an-observation-names-its-world.md)'s
per-world truth series is stored, which `layout.md` shows one way and never
argues · extends the same question to
[0019](0019-a-refusal-is-a-disposition.md)'s dispositions, which were specified
without one

## Context

0014 says a truth series is per `(artifact content hash, world)`, and that
verdicts from different trees are never folded together. `layout.md` shows
`truth/promise/{slug}.jsonl` — one file per artifact, and no world anywhere in
the path.

Both cannot be complete. Either the world is a component of the path, and
`layout.md`'s shape is missing one; or it is a field in each record and the fold
filters, and `layout.md` is right for a reason nobody has written down. The two
differ in what they cost and in how they fail, and the choice is one-way in the
ordinary way: it decides what a stream of records already written means.

0019 raised none of this. It gave dispositions the same per-artifact path as
truth and the same *transitions, not ticks* rule, and its example record names a
content hash and no world.

## Decision

**The world is a field in the record. It is part of a truth series' key and it
is not part of a disposition's.**

### Truth: a field, because a path would mint files without bound

A world is identified by a tree, and a working tree changes on every save. A
path of the form `truth/promise/{slug}/{world}.jsonl` therefore creates a new
**tracked** file every time anyone edits anything the promise looks at — dozens
in an afternoon on a repository, hundreds on a manuscript, all of them
permanent, all of them holding one or two lines.

That is not a volume problem to be tuned. It is
[0011](0011-tracked-formats-must-merge.md) rule 4 read backwards: the rule asks
that every path be keyed by something already unique, and a world is unique in
the wrong direction — it is a value that changes constantly rather than a thing
that is named once. The artifact is the thing that is named once, and it is
already the key.

So: `truth/{type}/{slug}.jsonl`, as `layout.md` writes it, with `world` on each
record, and **the fold filters by world after it folds by event time**.

The cost is stated rather than hidden: **a fold that forgets to filter is
silently wrong and looks correct in every single-run test.** That is 0011's own
warning about file order, now applying to a second axis, and it earns the same
treatment — a test that deliberately interleaves two worlds' verdicts and asserts
that the fold returns each world's own answer.

### A verdict reached inside a box belongs to the thread

One refinement keeps the stream from filling with worlds nobody will ask about.
The thread loop gathers three times against two worlds, and only one of them is
about the artifact: the gate's gather, on the tree that ships. The in-box gather
answers *did the repair work on its own terms*, which is evidence about the
repair.

So a verdict reached in a world whose `Thread` is set is recorded on the thread,
not in the artifact's truth stream. The artifact's stream then holds only worlds
the operator actually has, and box verdicts are pruned with the thread under
[0020](0020-durable-when-irreproducible.md)'s retention rather than accumulating
forever beside the answers a person reads.

### Dispositions carry the world and are not keyed by it

A disposition carries `world` for the reason every one-way field is carried:
0014 says a record written without one cannot be backfilled, because nothing
else records which tree was being looked at. A refusal recorded in a box and one
recorded in the workspace would otherwise be indistinguishable.

But **`kept why` folds the whole stream and ignores it**, and that is the
decision rather than an implementation detail. The question the verb exists for
is *why has this not been repaired for six days*, and the answer — *the budget
refused on four of them and the policy suppressed on two* — is about the queue.
Splitting it per world would fragment the one question 0019 was written to make
answerable, and would do it in a way that looks like more precision.

So the world on a disposition is **provenance, exactly like `writer`**: recorded
because it cannot be added later, never part of the key. This is written down so
that a later reader who notices truth folds per world and dispositions do not can
see that it was a choice.

## Alternatives

**A path component per world, for truth.** Rejected on unbounded file creation.
It is otherwise attractive — it would give the same non-contention property the
rest of the layout has, and pruning one world would not rewrite another.

**Fold truth without the world and let the latest verdict win.** Rejected by
0014 directly: a verdict reached in a repair's worktree and one reached on the
tree that ships are about different things, and the fold would return whichever
was written last.

**Key dispositions by world as well, for symmetry with truth.** Rejected above.
Symmetry is not the property either stream is for: truth is about a tree, and a
refusal is about a queue.

**Leave the world off a disposition entirely.** Rejected on 0014's rule. It is
one field, it costs nothing, and it is unrecoverable if omitted.

## Consequences

- **Reading a truth series is a fold plus a filter**, and the filter gets a test
  that fails without it.
- **`kept drift` answers for the world it is asked in**, which is what 0014
  already promised and this is where it becomes true.
- **Comparing worlds is a query.** *This promise holds on main and not in this
  box* is a filter over one stream rather than a join across files, which is the
  natural shape of the thread gate.
- **A disposition stream is readable end to end by a person**, which was 0019's
  point, and stays that way as boxes multiply.
