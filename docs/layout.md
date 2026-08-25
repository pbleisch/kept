# Layout

Two trees with different properties, and a third that is thrown away.

- **the corpus** — content. Human-written, human-read, diffed in review.
- **the ledger** — standing, truth, acts, threads. Machine-written,
  append-only, rarely read by eye.
- **derived** — caches and boxes. Rebuildable, gitignored. There is no
  index: [decision 0044](decision/0044-v1-has-no-index.md) measured the walk
  and kept it.

Keeping the first two apart matters because they merge differently, and
merging matters because **there is one operator but many concurrent runs** —
several `kept` processes ticking, repairing and recording at once. Every
tracked format must merge without anyone resolving it. See
[decision 0011](decision/0011-tracked-formats-must-merge.md).

The corpus is one file per artifact, so two runs touching different artifacts
never conflict. The ledger is append-only JSONL under `merge=union`,
which is safe only because every derived read folds by **event time, then id —
never file order**.

No path is partitioned by which process wrote it. Every stream is keyed by
something already unique — the artifact, the thread, the observation's own
content hash, the run — so two concurrent processes never choose the same file
and there is nothing to arbitrate. Attribution is a `writer` field inside each
record holding a minted [run id](decision/0012-a-run-id-is-opaque.md), not a
component of any filename.

## A repository kept holds promises about

```
myrepo/
  .kept/
    config.yaml               corpus root, retention, confinement

  refs/kept/ledger            ← the ledger is on its own ref, never checked
                                out, so no box contains it. Decision 0016.
                                A fetch never writes it: decision 0062.
    ledger/
      acts.jsonl              ratify · land · release · delegate. The spine
      threads/
        3k9wq2m1x8b4nptv0zrh7cdfy6.jsonl
                              one append-only receipt per thread: packet and
                              diff refs, transcript fragments, cost, turns,
                              lifecycle events and findings
      truth/
        promise/core-packages-are-tested.jsonl    transitions, one file per artifact
        canon/kael-eyes-are-green.jsonl
      disposition/
        promise/core-packages-are-tested.jsonl    why the tick did or did not act
      observations/
        sha256/ab/cdef…           one file per pinned observation; the world
                                  it measured is part of the address
      runs/
        3k9wq2m1x8b4nptv0zrh7cdfy6.json   one per process. Diagnostics, pruned
    artifacts/                  ← also on refs/kept/ledger, so no box can
      thread.diff/sha256/ab/cdef…   read what a previous repair saw (0023)
      session.transcript/sha256/…   tracked fragments; a session cannot be re-run
      gather.receipt/sha256/…       raw output, on failure only, redacted
    boxes/                    a thread's worktree            gitignored
    cache/                    observation cache per tick     gitignored
      digest.json             stat and blob oid to content   gitignored
                              digest; one slow tick to lose. 0033

  docs/                       ← the corpus root, set in config.yaml
    _schema/
      promise.yaml canon.yaml policy.yaml decision.yaml …
    _links/                   manifests for entities that cannot carry links
    promise/{slug}.md
    policy/{slug}.md
    canon/{slug}.md
    decision/{slug}.md          ADRs; ratified ones have standing
    document/{slug}.md
    desire/{slug}.md
    fact/{slug}.md
    model.md                  a file at the corpus root is not an entity;
    layout.md                 only type directories are walked

  cmd/  internal/  …          the repository's actual content
  .gitattributes              ledger/**/*.jsonl merge=union
                              (paths on refs/kept/ledger, which nothing
                               checks out — see decision 0016)
```

## A manuscript

The corpus root is the repository root, because here the corpus *is* the
content. Nothing else changes.

```
starweave/
  _schema/
  series/  book/  chapter/  scene/  character/  location/  plotline/
  canon/   promise/  policy/  desire/  fact/
  _links/
  .kept/…
```

## Why the ledger is tracked, and why not here

Ratifications and verdict transitions are acts and observations about this
repository. Ignore them and a fresh clone reverts every promise to unratified
and every verdict to unknown, which is the wrong kind of safe: it looks like a
system that has never been used.

Tracked, but not on the branch the code is on. A box is a worktree, so anything
on `main` is in the repair agent's tree — including the ledger, which the agent
must not be able to write. It lives on `refs/kept/ledger`, nothing checks it
out, and `kept init` adds the fetch refspec so an ordinary `git fetch` brings
it. See [decision 0016](decision/0016-the-ledger-lives-on-its-own-ref.md).

**A fetch brings it to `refs/kept/upstream/ledger`, not to the ledger.** That is
the second ref git gives every branch, and the ledger had none: with one ref the
destination of a network fetch was the durable record itself, which is how a
routine `git fetch` erased two ratifications on 2026-08-22 and, unforced, how it
then refused every fetch while the ledger was ahead of its remote. The verbs
that write fast-forward the ledger onto what the last fetch brought, and
`kept ledger sync` does it on its own; no verb that only reads will, because a
reader that writes cannot run against a read-only checkout. So **a clone holds
the upstream ref and nothing else until one of those runs**, and `kept doctor`
reports a workspace in that state. See
[decision 0062](decision/0062-a-fetch-writes-an-upstream-ref.md).

Only the derived tiers are ignored — cache, boxes. Session transcripts
are **not** among them, though they are the largest thing here: a transcript
cannot be re-derived by running the agent again, because that produces a
different session at a different cost. Durability follows reproducibility, not
size. See [decision 0020](decision/0020-durable-when-irreproducible.md).

**Check this deliberately when adopting kept into an existing repository.** A
working root sitting inside a gitignored path once made grft's containment
checks silently see nothing at all.

## Volume, and what keeps it down

The tick evaluates every due artifact, and most of the time nothing has
changed. Three rules stop that filling the repository:

1. **Truth and dispositions record transitions, not ticks.** A verdict is
   appended when it changes, plus a periodic heartbeat so staleness is visible.
   A promise kept for a month is a handful of lines, not thousands, and one
   refused for a month on the same grounds is the same.
2. **Observations rotate and prune.** They are retained per
   `(artifact, gatherer)` up to a window, because `prior()` needs history — but
   only a window of it. Anything a truth transition or a thread references is
   pinned.
3. **Large payloads are content-addressed and referenced.** A thread record in
   the ledger is small; the transcript it points at is not, and lives under
   `artifacts/` with its own retention. Retention on something irreproducible
   is `discard-after`, not `prune`: the hash stays in the thread record, so an
   absent transcript reads as destroyed on purpose rather than never captured.

This is adsr's tier discipline applied at one-operator scale: what nothing can
re-derive is durable, what something can re-derive is not, and volume goes
where it does not have to be cloned.

## One file per artifact, one file per artifact's truth

`truth/promise/core-packages-are-tested.jsonl` rather than one global stream.
Three reasons: two runs recording verdicts for different artifacts never touch
the same file, a diff shows which promise moved without reading a
timestamp, and pruning one artifact's history does not rewrite everyone else's.

Observations are content-addressed rather than partitioned by writer, which
gets the same property from a different direction: two runs that observe the
same thing write identical bytes to one path, and two that observe different
things write to different ones. Under
[decision 0014](decision/0014-an-observation-names-its-world.md) the world
measured is part of that address, so "the same thing" means the same tree. Appending to a shared stream instead
would be safe only for short lines and is not worth relying on.

Acts stay a single stream. They are a spine — order matters, volume is low,
and an act is meaningless without the ones around it. `decision` names an
artifact kind and nothing in the ledger; see
[decision 0039](decision/0039-the-ledger-records-acts.md). Under union
merge that stream can interleave, which is why it is read by folding on event
time rather than by reading it top to bottom.

## What this buys

No leader election, and no compare-and-swap over the files. Both exist to
arbitrate two writers over the same bytes, and one file per artifact means that
case does not arise for the records themselves. adsr needed them because it had
one shared artifact store written field by field; this does not.

**The ledger's ref is the exception, and it is not a small one.** This section
was written before
[decision 0016](decision/0016-the-ledger-lives-on-its-own-ref.md) moved the
ledger onto `refs/kept/ledger`, and it described the file layout, which is still
true of it. A ref is one value, so every ledger write is a read-modify-write
against the same bytes and there **is** a compare-and-swap: read the ref, build
a commit on the value read, and update the ref with that value as the
expectation. What the file layout buys is still bought — a losing writer rebuilds
and appends its own line, and no two writers ever contend for one artifact's
stream — but the arbitration happens one level down, in git, over the ref that
carries all of it. A losing writer waits and tries again, on the schedule in
[decision 0064](decision/0064-a-losing-writer-waits-and-losing-is-not-an-error.md).

Merge is not all of coordination, and merge is all git provides. Two runs can
open a thread for the same drift, and a budget cannot refuse without a counter
something can decrement under exclusion. Neither is a correctness problem:
the gate re-derives from the world, so a duplicated repair lands nothing. Both
are money, and on one machine `flock` saves it. See
[decision 0013](decision/0013-the-gate-makes-concurrency-safe.md).

Landing is not one of those, despite an earlier reading. Git's pushes are
atomic and one of two racing pushes is simply rejected; what is missing is
isolation between the gather that gates a change and the push that ships it,
which is why landing is a short critical section around a cheap check rather
than a lock held across a gate. See
[decision 0015](decision/0015-landing-is-a-merge-queue.md).
