---
id: 0016-the-ledger-lives-on-its-own-ref
type: decision
title: 0016 — The ledger lives on its own ref
links:
  - { target: decision:0011-tracked-formats-must-merge, relation: extends }
  - { target: decision:0015-landing-is-a-merge-queue, relation: extends }
  - { target: decision:0050-doctor-reports-an-unchecked-instrument, relation: depends-on }
---

# 0016 — The ledger lives on its own ref

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

### What a forced refspec cost, once

This decision originally named the refspec forced. That was never argued for; it
is the spelling one writes without thinking, because a refspec in an example
usually carries a `+`.

**On 2026-08-22 it erased two ratifications from this repository.** The grants
for the revised [0017](0017-the-box-is-not-the-jail.md) and
[0052](0052-repair-runtimes-are-closed-adapters-with-native-receipts.md) had
been appended locally, origin still held a copy of the ledger from the previous
day, and a worktree pool ran `git fetch origin` while acquiring a worktree. The
configured refspec force-updated `refs/kept/ledger` backwards fourteen commits
onto the stale remote value. Exit 0. No output. No reflog — git logs
`refs/heads/`, `refs/remotes/`, `refs/notes/` and `HEAD`, and nothing else, so
the durable tier is stored in the one ref namespace git keeps no safety log for,
and there was nothing to compare against afterwards. The records were recovered
from an object still unreferenced in the object store, which is luck rather than
a property.

Three things about that are worth separating, because only one of them is
kept's.

**The trigger was ordinary.** Not a kept verb — kept has never fetched or
pushed, so *sync is automatic* below remains unbuilt, which is itself why the
ledger is chronically ahead of its remote. It was `git fetch`, run by a
third-party pool manager on every acquisition. Any `git fetch` would have done
it, run by anyone.

**The state that made it destructive is permanent.** Acts are appended on every
tick and nothing pushes, so between any two pushes the local ledger holds
records the remote does not. A ledger ahead of its remote is not an incident; it
is the resting state of this system.

**What kept contributed was one character.** Removing the `+` and changing
nothing else — same fetch, same remote, same stale value — was measured to make
the loss not happen.

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
git config --add remote.origin.fetch 'refs/kept/ledger:refs/kept/ledger'
```

Every run syncs at start and pushes at exit. The ledger push does **not** go
through the merge queue — different ref, no gate, high frequency, append-only,
so a rejected push is fetch-merge-retry and costs nothing.

Explicit commands exist for when that is not enough: `kept ledger sync`,
`kept ledger push`, `kept ledger fsck`.

**This makes 0013's staleness bound concrete.** "Has this run caught up within
`staleness.max_age`" is the age of its last ledger sync, and refusing to act
past it is refusing to act on a ledger nobody has fetched.

### The refspec is not forced, and a rewind is a refusal

**The refspec carries no leading `+`, and that is load-bearing rather than
incidental.** It was forced when this was first written — the spelling one
reaches for without thinking — and on 2026-08-22 that erased two ratifications
from this repository. See *What a forced refspec cost, once*, above.

`RefStore.mutate` reads the ref, builds on the value it read, and writes with a
compare-and-swap; a losing writer retries. kept's writer can only move the ref
forward. So a ledger advances and never rewinds, and **every legitimate fetch is
already a fast-forward**: a new ref in a clone that has none, a remote that is
ahead, or nothing to do. All three were measured to work unforced.

The only case the `+` adds is the one that must never happen — a remote that has
fallen behind overwriting acts nobody pushed. Git refuses that by name, with a
non-zero exit and `! [rejected] … (non-fast-forward)`, unless something opts out
of the refusal. Since a side ref has no reflog, that refusal is the only warning
there is. It was a force flag whose entire realized effect was data loss.

This is also why 0013's *no lock is load-bearing* survives intact rather than
being contradicted. That argument accepts concurrent writers because the gate is
authoritative and the ledger only moves forward; a forced fetch broke the
premise from outside kept, where no gate, no lock and no compare-and-swap could
see it.

### A clone already configured is corrected, and told

Changing what this decision says rewrites nobody's git config. Every workspace
adopted under the forced spelling still carries it, and doing nothing about that
leaves the trap armed in every one of them.

Adding the unforced spelling *beside* the forced one is not a cautious middle.
Measured: with both configured and the forced line first — the order
`config --add` produces — the fetch is **still forced and the ledger is still
destroyed**; with the unforced line first it is refused. A config whose behavior
turns on which line git reads first is not a repair.

So `ConfigureFetch` removes the forced spelling, leaves the unforced one, and
reads the config back rather than trusting the write — reporting a correction
while leaving one configured is the exact silent failure this is about. It is
one exact string that kept's own earlier verb authored, so correcting it is not
[`policy:nothing-kept-writes-rewrites-what-a-person-wrote`](../policy/nothing-kept-writes-rewrites-what-a-person-wrote.md),
which is about content a person wrote. `kept init` says what it changed and why,
because a config kept rewrites without a word is the failure on the other side.

### `doctor` reports one nobody has corrected

[0050](0050-doctor-reports-an-unchecked-instrument.md) drew the line —
**`doctor` reports what is well-formed and unwise, `kept list` reports what is
wrong** — and set three conditions for admitting a finding, saying in as many
words that the sentence is what bounds the next one. This is the next one, and
it meets all three. Nothing refuses a forced refspec: git accepts it, kept
accepts it, the corpus loads. Nothing kept does about it: no kept verb fetches,
so nothing is blocked and nothing fails. And it has no other reader — `init` is
the adoption verb, and a workspace that has been adopted does not run it again,
so an operator who never re-inits is never told.

So this applies 0050 rather than widening it, and 0050 needs no amendment.
It is a finding and never an error, so 0027's exit codes are unchanged, and
`--fix` is untouched: it writes identity, a git config is not identity, and
`kept init` is what corrects this.

**Divergence itself is deliberately not reported.** With the ledger ahead of a
stale remote, every bare `git fetch` against such a repository fails until the
remote catches up — that is the visible half of the refusal working. But *how
far ahead* cannot be answered offline: the refspec writes the remote's value
straight into the local ref, so there is no remote-tracking ref to compare
against, and the only way to know is to ask the remote. A diagnostic that needs
the network answers differently on a plane, and the answer would be about the
operator's push cadence rather than about a defect. What is knowable locally is
whether the refusal that makes divergence safe has been switched off, and that
is what is reported.

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

**Unset `remote.origin.fetch` for the ledger entirely**, rather than unforcing
it. Rejected: it un-decides the paragraph above. A clone would stop bringing the
ledger, and a clone that reverts every promise to unratified is the failure
`layout.md` named and this decision was written against.

**Push the ledger to origin so the remote is never stale.** Rejected as the
fix, though worth doing on its own terms. It removes the staleness that makes a
forced fetch destructive and leaves the mechanism live: origin goes stale again
at the next ratification, and the trap re-arms silently. It restores the
appearance of health, which is the worst property a repair can have here.

**Give the ledger a reflog so a rewind is recoverable.** Rejected as the primary
answer: `core.logAllRefUpdates` does not cover `refs/kept/`, so it means a
per-repository configuration kept would have to write and then depend on, and it
makes the loss survivable rather than preventing it. Recovery from a reflog is
also a procedure somebody has to know to run, in a window before `git gc`.
Worth having; not this.

**Leave the forced spelling and document the hazard.** Rejected. The hazard is
armed by kept, in somebody else's git config, and fired by a command nobody
thinks twice about. Documentation does not reach the tool that ran the fetch.

**Refuse to touch a git config a person may have written.** Rejected on the
measured result: the alternative to correcting it is leaving a trap that fires
silently, and the string being corrected is one kept authored itself. The
conflict is resolved by saying so rather than by declining.

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
- **A bare `git fetch` against a repository whose ledger is ahead now fails.**
  Exit 1, with `[rejected] … (non-fast-forward)` naming the ref. Nothing inside
  kept is affected, because nothing inside kept fetches; what changes is that a
  person or a tool running git sees a refusal where there was silence. That is
  the point, and it is a cost: a pool manager or a script that treats a non-zero
  fetch as fatal will now stop. The remedy is to push the ledger, which is an
  act, not a configuration.
- **Every clone configured under the forced spelling is corrected the next time
  `init` runs there, and told.** One that never runs `init` again keeps it, and
  `doctor` is what reports it.
- **`ConfigureFetch` returns what it did** rather than whether it wrote —
  configuring and correcting are different acts and `init` says which.
- **The regression test is the deliverable, not the constant.** A ledger ahead
  of a stale remote, an ordinary fetch, and an assertion that the acts survive.
  It fails under the forced spelling — on the exit code, on the absent refusal,
  and on the acts — and passes under the unforced one. A test that would also
  have passed before the fix would be worth nothing here.
- **This revision changed what was decided, so standing dropped and was granted
  again.** The ledger holds both hashes with a date between them, which is the
  record that a person read the new wording. See `AGENTS.md` on revising rather
  than superseding: the answer here still stands, and one operative detail of it
  was wrong.
