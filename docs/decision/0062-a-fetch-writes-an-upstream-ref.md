---
id: 0062-a-fetch-writes-an-upstream-ref
type: decision
title: 0062 — A fetch writes an upstream ref, and only kept advances the ledger
fields:
  statement: >-
    An ordinary git fetch writes refs/kept/upstream/ledger and never the ledger
    itself, and the ledger is advanced onto it by the verbs that write and by
    kept ledger sync.
links:
  - { target: decision:0016-the-ledger-lives-on-its-own-ref, relation: amends }
  - { target: decision:0054-a-ledger-that-went-backwards-is-a-refusal, relation: extends }
  - { target: decision:0050-doctor-reports-an-unchecked-instrument, relation: depends-on }
---

# 0062 — A fetch writes an upstream ref, and only kept advances the ledger

## Context

[0016](0016-the-ledger-lives-on-its-own-ref.md) put the ledger on
`refs/kept/ledger` and had `kept init` configure

```
refs/kept/ledger:refs/kept/ledger
```

That is one ref. **Git gives every branch two**: an upstream copy
(`refs/remotes/origin/main`) that a fetch overwrites freely, and a local
authoritative one (`refs/heads/main`) that only an explicit merge advances.
Every safety property people attribute to "normal branch flow" comes from that
split rather than from living under `refs/heads/`. The refspec above has no
split, so **the destination of a network fetch is the authoritative durable
record itself**. There is nothing to merge from, nothing to compare against, and
no way for a fetch to be a no-op when the two disagree.

Three separate failures trace to that one choice, and they were reported as
three problems.

**Forced, it destroyed records.** On 2026-08-22 a worktree pool's routine
`git fetch origin` force-updated `refs/kept/ledger` backwards fourteen commits
onto a stale remote value, erasing two ratifications. Exit 0, no output. 0016's
revision removed the `+`, which was measured to stop it.

**Unforced, it refuses, and the refusal breaks every caller.** A ledger ahead of
its remote is — 0016's own words — *the resting state of this system*. In that
state a bare `git fetch` exits 1 with `! [rejected] … (non-fast-forward)`, and a
worktree pool that fetches on acquisition **hands out no worktree at all**. That
happened three times in one day, each time after an ordinary ratification, and
each time it stopped task dispatch against this repository until somebody
pushed a ref no kept verb pushes.

**On a fresh adoption it fails before anything has ticked.** Measured on an
ordinary repository with an ordinary remote: `git fetch` exits 0, `kept init`
runs, and `git fetch` then exits **128** with `fatal: couldn't find remote ref
refs/kept/ledger`. The source is a single named ref, and the remote does not
have it. That is the state of every repository kept has ever adopted, including
this one until 2026-08-21, and nothing had reported it because the test that
covers the refspec enumerates three cases it calls *every legitimate ledger* and
pushes the ledger to origin before each one.

**And it is not about the namespace.** `refs/notes` — the one non-branch
namespace git supports properly — has the identical hazard under its own
conventional refspec: `+refs/notes/*:refs/notes/*` rewinds a local notes ref
from a stale remote, and unforced it is rejected. Moving the ledger anywhere
else moves the reflog problem and keeps this one.

## Decision

**`kept init` configures**

```
+refs/heads/*:refs/remotes/origin/*
+refs/kept/ledger*:refs/kept/upstream/ledger*
```

The second line is what this decision was written about, and every character of
it is load-bearing and two of them read as reversals.

**The destination is `refs/kept/upstream/ledger`.** The authoritative ref is no
longer written by the network at all, which makes the class of loss that
happened on 2026-08-22 *unreachable* rather than refused — strictly stronger
than what 0016 achieved, and cheaper than what 0054 had to build. Measured
against an upstream destination: a remote with no ledger, a remote behind, a
remote ahead, a genuine fork, a remote that rewound its own ledger, and a
`--prune` after the remote drops the ref **all exit 0**, and the ledger ref is
untouched in every one of them.

**The leading `+` returns, and it is not a reversal.** It was wrong on a ref
kept's own writer owns and it is right on a ref that is a copy of what a remote
holds — which is why `+refs/heads/*:refs/remotes/origin/*` has carried one since
git had remotes. A remote that rewinds its own ledger is a fact about the
remote, and an upstream ref exists to record it.

**The `*` is not decoration.** Measured: a single-ref refspec whose source the
remote does not have is `fatal`, whether it is spelled `refs/kept/ledger:…`,
`+refs/kept/ledger:…`, or `+refs/kept/ledger:refs/kept/upstream/ledger`. A
pattern that matches nothing is a no-op. That one character is the whole of the
fresh-adoption fix.

**Both old spellings are removed, not added around.** `ConfigureFetch` now
corrects the forced *and* the unforced direct line, because git applies the
first refspec that matches and a config holding a direct line and a tracking one
behaves according to config file order rather than according to anything kept
decided. It reads the config back, for the reason it already did.

**The first line is a correction to this decision, made on 2026-08-25.** The
original printed the ledger's refspec alone, and it was configured that way: on
this repository `remote.origin.fetch` held that one value and nothing else, so
`git fetch origin` exited 0 and updated **no remote-tracking branch at all**.
Measured the day it was found, `refs/remotes/origin/main` was eight commits
behind what origin held, and it had been drifting since the line was written —
a stale ref that reads exactly like a current one, which is why nothing reported
it.

Nothing here decided that. This decision's subject is the *destination* of the
ledger's own refspec — that a network fetch must not write the durable record —
and it is silent on branches. The two are orthogonal: they share a config key
and nothing else, git applies every refspec that matches, and their sources
cannot overlap, because `refs/kept/` and `refs/heads/` are different
hierarchies. Adding one line where there had been none narrowed the remote as a
side effect.

Three things bound the correction. **It is added only when nothing already
brings `refs/heads/` into `refs/remotes/`** — a refspec of either shape alone
admits a config that leaves the tracking refs stale, so both halves are
required. **An existing branch refspec is never removed, narrowed or widened**:
an operator who configured `+refs/heads/main:refs/remotes/origin/main` has
answered this question, and correcting kept's own two direct spellings is a
different act from editing a line kept did not author. And **nothing is written
where there is no `remote.origin.url`**, because a workspace adopted before it
had a remote has nothing to fetch branches from.

`kept land` is not the caller this broke. It fetches its target's refspec
stated in full — `internal/land/git.go` says why, and names this repository's
own narrowing as the reason — so
[0015](0015-landing-is-a-merge-queue.md)'s in-lock check reads a
tracking ref it refreshed itself, and its two proofs that the check never reads
`refs/heads/<branch>` are unaffected. What it broke is everything else:
`git log origin/main`, `git rebase origin/main`, a worktree pool deciding
whether a branch is behind, and a person reading `git status`.

### Advancing the ledger is an act, and reads never perform it

A clone receives the upstream ref and no ledger. kept materializes it:
`ledger.Advance` fast-forwards `refs/kept/ledger` onto
`refs/kept/upstream/ledger` when the upstream holds records the ledger cannot
reach and the ledger holds none the upstream cannot. It cannot lose a record —
a fast-forward's destination reaches every value the source did, so 0054's
`Continuous` stays quiet across one by construction; both are statements about
the same DAG.

**The verbs that write call it. The verbs that read do not.** Both halves are
load-bearing and they fail in opposite directions.

A read that advanced could not run against a checkout nobody may write to, which
is what [0001](0001-standing-and-truth-live-in-the-ledger.md) and
[0032](0032-standing-is-a-fold-over-acts.md) make `kept status`, `kept why`,
`kept drift` and `kept list` answer from.

A write that did *not* advance is worse. Appending onto a ledger that is behind
**forks it**: a clone that fetched and then ticked would manufacture a
divergence out of nothing, every time. Git makes you merge before you can push
for exactly this reason; kept's spelling of it is *advance before you append*. So
a fork is what a fetch discovers rather than what a write creates.

The advance is reported on stderr rather than performed silently — moving the
durable record without a word is the failure on the other side of moving it at
all — and on stderr rather than stdout because a tick's stdout is a document
when `--json` is given.

### `kept ledger sync` fast-forwards, and states what it does not attempt

0016 named `kept ledger sync|push|fsck` and built none of them. One exists, and
the count is the decision rather than an omission.

`kept ledger sync` advances the ledger onto what the last fetch brought. **It
does not fetch, it does not push, and it does not merge.**

*It does not fetch* because kept has never made a network call and this is not
the place to start one. A fetch is `git fetch`, run by the operator, an editor,
or a worktree pool, and the whole point of this decision is that an ordinary one
is now safe. Wrapping it would give kept a credential path, a deadline under
[0026](0026-nothing-external-runs-without-a-deadline.md), and a diagnostic that
answers differently on a plane.

*It does not push*, and 0016's *"every run syncs at start and pushes at exit"* is
withdrawn rather than deferred. It is rejected on the measurement rather than on
taste: had automatic push existed, the first tick after the 2026-08-22 rewind
would have published the damaged 90-act ledger to origin sixteen minutes later,
as a clean fast-forward, overwriting the last copy of the good one.

*It does not merge.* Two clones that both appended have forked, and for an
append-only record the answer is the union of both sides — which `.gitattributes`
already declares with `merge=union`, which every derived read is already safe
under because it folds by event time rather than by file order, and which is
what the 2026-08-22 repair did by hand. That commit is one somebody makes; it is
not one kept composes. A fast-forward cannot lose a record and a merge kept
wrote could, and it would do it to the one file in this system that is not
reproducible. So `kept ledger sync` refuses a fork, names both tips, prints
`git merge-base` for where they parted, and stops.

**A fork refuses that verb and nothing else.** Exit 4, beside a rewound ledger
and for the same reason — there are acts in this repository's object store that
the fold every verdict is derived from cannot see, so the record is known to be
incomplete. But every other verb keeps answering, and a writing verb keeps
writing: a machine that could not record what it found because another clone had
also written would be a worse failure than a fold missing that clone's acts, and
appending to either side loses nothing a union will not recover.

### `doctor` reports divergence, which it could not before

0016 said this in as many words: *"how far ahead cannot be answered offline: the
refspec writes the remote's value straight into the local ref, so there is no
remote-tracking ref to compare against, and the only way to know is to ask the
remote."* That was true of that refspec and is false of this one. With both
values in one repository it is one `rev-list --left-right --count`, and no
network.

Two states are reported and two are not.

| | |
|---|---|
| no upstream ref | silence. Nobody has fetched, which is neither agreement nor divergence |
| level | silence |
| **ahead** | silence. This is the resting state, and reporting it would report the operator's push cadence as a defect |
| **behind or absent** | reported, naming `kept ledger sync` |
| **forked** | reported, naming the union |

It passes [0050](0050-doctor-reports-an-unchecked-instrument.md)'s three
conditions. Nothing refuses a ledger that is behind or forked — git accepts it,
kept accepts it, the corpus loads. Nothing else in kept reports it. And what
kept *does* about it is bounded in a way doctor sits outside: the verbs that
write advance, and doctor reads, so a checkout that is only ever read stays
behind indefinitely while every answer it gives comes from the older record.
That is a finding an operator can act on with one command, which is the test.

It is a finding and never an error, so [0027](0027-exit-codes-are-the-contract.md)'s
exit codes are unchanged for `doctor`, and `--fix` is untouched: it writes
identity, a git ref is not identity, and `kept ledger sync` is what advances one.

**This is also what makes [0013](0013-no-lock-is-load-bearing.md)'s staleness
bound answerable for the first time.** *Has this run caught up* is a comparison,
and the previous refspec destroyed the value it would have compared against.

## Alternatives

**Leave the refspec unforced and live with the refusal.** Rejected, and it is
the status quo. The refusal is correct and it breaks every caller that runs
`git fetch` — measured, a worktree pool acquiring a worktree exits 1 and hands
out nothing while the ledger is ahead, which is the resting state. It also does
nothing about the fresh-adoption failure, which fires on day one in every
repository kept has ever adopted. Documenting the hazard does not reach the tool
that ran the fetch.

**Move the ledger to an orphan branch.** Rejected on migration cost, not on
merit, and it is the closest alternative. Measured, `refs/heads/kept-ledger`
satisfies every requirement 0016 names — a box cut at a commit does not contain
it, it is writable from any branch and unchanged by a checkout, it never enters
[0015](0015-landing-is-a-merge-queue.md)'s queue, and it never appears in a diff
against `main` because it shares no history with it. It gets the upstream ref
and the reflog from git's defaults, which would let kept stop writing and
depending on two per-repository settings, one of which overwrites a value a
person may have set. The measured objections to `refs/heads/` do not hold:
`git branch --merged` does not list an orphan branch, `git branch -d` refuses it
(*"not fully merged"*), and `--prune` never touches a local branch. What decides
it is that moving a durable record is a migration every clone and every worktree
pool must perform, and one that misses it reads an empty ledger — *a system that
has never been used*, which is the failure `layout.md` was written against. The
refspec delivers the same structural property and moves nothing.

**Move the ledger to `refs/notes`.** Rejected. Measured, the conventional notes
refspec `+refs/notes/*:refs/notes/*` rewinds a local notes ref from a stale
remote exactly as the ledger's did, and unforced it is rejected exactly as the
ledger's is — because the defect was never the namespace. Notes would buy a
default reflog, which 0054 already provides, and `cat_sort_uniq`, which
`merge=union` already matches, at the cost of reshaping records keyed to
commits, which ledger records are not.

**Have `kept ledger sync` commit the union.** Rejected for now, and it is the
one alternative that would make the fork case complete rather than merely
honest. It is not refused on principle — union is what the records require and
what `merge=union` already declares — but on scope and on stakes: nothing in
this system has yet reached a fork, because the concurrency that exists is
eleven worktrees over one object store and one ref; and the first merge kept
composes for itself should be written against a fork somebody actually has,
rather than against one imagined. Refusing loses nothing while both refs exist.

**Advance the ledger inside `RefStore.mutate` instead of at the verb.** Rejected.
It would put two git calls in the append retry loop, and it would hide a write to
the durable record inside a method whose contract is *append one line*. The verb
is where a person's command enters, and it is where the report of what moved
belongs.

**Have reads advance too, so a clone needs no extra command.** Rejected: it makes
`kept status` something that cannot be run against a read-only checkout, which
0001 and 0032 both turn on.

## Consequences

- **A clone receives the upstream ref and no ledger until kept advances it.**
  That is one command on adoption — `kept ledger sync`, or any verb that writes
  — and it is the one axis on which the previous refspec was better: its direct
  mapping populated the authoritative ref straight from the fetch. `doctor`
  reports a workspace whose upstream ref is ahead of its ledger, so the state is
  visible rather than silent.
- **There is no data migration.** The ledger does not move, no ref is rewritten,
  and every existing clone keeps working the moment `kept init` runs there. A
  clone that never runs `init` again keeps its old spelling, and `doctor` is
  what reports it.
- **An already-narrowed repository is corrected by re-running `kept init`**,
  which is the same answer this decision already gave for a direct spelling:
  `ConfigureFetch` runs on every init, so the workspace this was found in is
  repaired by the verb that broke it. An operator who never runs it again is not
  left silent — `doctor` reports an origin that fetches no branch, beside the
  one that fetches straight into the ledger and for the same reason. It passes
  [0050](0050-doctor-reports-an-unchecked-instrument.md)'s three conditions:
  nothing refuses it, nothing else in kept reports it, and what kept does about
  it is one line one command writes.
- **`ConfigureFetch` now corrects two old spellings**, forced and unforced, and
  reads the config back for the reason it already does.
- **A `git push` of the ledger updates the upstream ref too**, because git
  updates a remote-tracking ref for a ref it just pushed. So the operator who
  pushes sees `doctor` go quiet without fetching.
- **4 has a second reachable cause.** 0054 reserved it for a rewound ledger; a
  `kept ledger sync` against a fork joins it. Both are *the record is known to be
  incomplete*, and neither makes any other verb refuse.
- **The regression test is the deliverable, and it is a table rather than a
  case.** `TestAnOrdinaryFetchExitsZeroInEveryStateAndNeverWritesTheLedger`
  enumerates seven states — remote absent, local ahead, nothing to do, remote
  ahead, local absent, both moved, and a remote that rewound its own ledger —
  and asserts exit 0 and an untouched ledger ref in every one. The guard that
  keeps it honest is `TestTheSpellingsThisReplacedFailTheStatesItPasses`, which
  runs the same table under both old spellings and requires each to fail: the
  forced one by rewinding, the unforced one by refusing, and both on the fresh
  adoption. A property test whose subject is not what it measures is worth
  nothing.
