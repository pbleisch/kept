---
id: 0016-the-ledger-lives-on-its-own-ref
type: decision
title: 0016 — The ledger lives on its own ref
---

# 0016 — The ledger lives on its own ref

**Status** accepted · refines [0011](0011-tracked-formats-must-merge.md), which
tracked the ledger without saying where · closes a containment hole opened by
[0015](0015-landing-is-a-merge-queue.md)'s boxes

## Context

The ledger is tracked, for the reason `layout.md` gives: a fresh clone that
reverted every promise to unratified would look like a system that had never
been used. That reasoning holds. What was never decided is *which ref* it is
tracked on, and the implicit answer — the same branch as the code — is wrong
three times.

**A box contains the ledger.** A box is a git worktree, so it checks out
everything tracked, including `.kept/ledger/**`. The repair agent works in the
box. It can append

```
ratified(promise:whatever, sha256:…, paul@…, at)
```

and `merge=union` is designed not to drop lines, so it merges clean and silent
on land. That defeats the invariant the model is built on: standing is created
only by a person, and the agent that may write the corpus may not write
standing or truth. Today that is enforced by *layout* — neither property is in
the artifact file — and layout stops enforcing anything once the ledger is
inside the agent's tree.

**The rates are incompatible.** Truth transitions are appended every tick.
Code lands go through 0015's merge queue, which exists to guarantee every
commit on `main` was gated. Putting per-tick ledger churn through a lock and
into `main`'s history is absurd, and gating a ledger append is meaningless.

**The audiences are different.** `main`'s history is what a person reviews. The
ledger is machine-written and rarely read by eye, per `layout.md`'s own
description, and it would swamp the history a person reads.

## Decision

**The ledger lives on `refs/kept/ledger`, and no box ever checks it out.**

[0023](0023-what-may-be-captured.md) extends this to `.kept/artifacts/**` for
the second half of the same reason: an agent must not write the ledger, and
must not read the transcripts of previous repairs. Only `.kept/config.yaml`
stays on `main`, because it is authored and reviewed rather than
machine-written.

An agent cannot write what it cannot see, so the guarantee stops depending on
a scope list being correct. Belt and braces, because a second mechanism costs a
line: **`.kept/**` is categorically out of scope for any repair, whatever a
promise declares.** A promise that could put the ledger in scope is a promise
that can mint standing.

### This is not two checked-out refs

The working tree has one `HEAD`, always. The ledger ref is never checked out;
it is written programmatically — a temporary `GIT_INDEX_FILE`, or
`hash-object` and `mktree` directly, which suits append-only JSONL — and read
with `git show`. A dedicated hidden worktree is an acceptable first
implementation and an unnecessary one later.

### Precedent

This shape is well travelled, and the closest analogues are worth naming
because they also answer how to manage it:

| | |
|---|---|
| **git-annex** | Location metadata on its own `git-annex` branch, never checked out, union-merged by a custom driver, kept in sync as a side effect of ordinary commands. The closest match to this design in every respect. |
| **git-bug** | Issues under `refs/bugs/*`, with explicit `push` and `pull` subcommands. |
| **git-notes** | `refs/notes/*` with a `cat_sort_uniq` merge strategy — literally union-merge-and-dedup. Also the cautionary tale: it is not fetched by default, and people forget it exists. |
| **`refs/pull/*`, `refs/changes/*`** | Forge-side state on refs nobody checks out. |
| **`refs/replace`, `refs/stash`, `refs/bisect`** | Git's own out-of-tree state. |

The lesson from notes is the one that shapes the CLI below: an out-of-tree ref
that requires the operator to remember it will be forgotten.

### Sync is automatic, with explicit commands for repair

`kept init` adds the refspec, so ordinary `git fetch` brings the ledger:

```
git config --add remote.origin.fetch '+refs/kept/ledger:refs/kept/ledger'
```

Every run syncs at start and pushes at exit. The ledger push does **not** go
through the merge queue — different ref, no gate, high frequency, append-only,
so a rejected push is fetch-merge-retry and costs nothing.

Explicit commands exist for when that is not enough: `kept ledger sync`,
`kept ledger push`, `kept ledger fsck`.

**This makes 0013's staleness bound concrete.** "Has this run caught up within
`staleness.max_age`" is the age of its last ledger sync, and refusing to act
past it is refusing to act on a ledger nobody has fetched.

## Alternatives

**Keep the ledger on `main` and rely on scope enforcement.** Rejected: it makes
the model's central invariant depend on every promise's scope being right and
on the scope check having no gaps, when the alternative is the agent never
having the bytes.

**A separate repository for the ledger.** Rejected: two clones to keep in step,
two remotes to authenticate, and it loses the property that a content hash in
the ledger refers to an object the same repository holds.

**Untracked, local only.** Rejected outright by `layout.md`'s reasoning, and
worse now — a fleet of boxes and runs would each have a private ledger.

## Consequences

- **`git clone` no longer brings the ledger.** `kept init` adds the refspec, so
  the cost is one command on adoption rather than a permanent trap — but a
  clone made without it looks like a system that has never been used, which is
  exactly the failure `layout.md` warned about. `kept` reports an absent ledger
  ref distinctly from an empty one.
- **`.gitattributes` moves with it.** The `merge=union` rule applies to paths on
  a ref that is never checked out, which works, and is worth a comment saying
  why the pattern refers to files nobody will see in a working tree.
- **Ledger and code can disagree about time.** A ledger record can reference a
  commit that has not been pushed, or a commit can land before its record
  arrives. Both are already handled: records are retrospective
  ([0018](0018-what-survives-a-crash.md)), and truth is derived on read.
- **Two mechanisms now protect standing** — the agent cannot see the ledger, and
  the ledger is out of scope — and both get the test `AGENTS.md` requires.
