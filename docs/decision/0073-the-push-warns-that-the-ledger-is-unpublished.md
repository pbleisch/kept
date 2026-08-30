---
id: 0073-the-push-warns-that-the-ledger-is-unpublished
type: decision
title: 0073 — The push warns that the ledger is unpublished
fields:
  statement: >-
    kept init writes a pre-push hook that warns, on stderr and without refusing, when the
    ledger holds acts origin did not have as of the last fetch; it publishes nothing, it is
    written into the shared hooks directory and nowhere else, and doctor reports a repository
    where nothing at push time would say it.
links:
  - { target: decision:0062-a-fetch-writes-an-upstream-ref, relation: extends }
  - { target: decision:0050-doctor-reports-an-unchecked-instrument, relation: extends }
  - { target: decision:0016-the-ledger-lives-on-its-own-ref, relation: depends-on }
---

# 0073 — The push warns that the ledger is unpublished

## Context

**git push does not carry a side ref.** `refs/kept/ledger` is not under
`refs/heads/`, so nothing in an ordinary push, and nothing in `git status`,
mentions it. Measured on this repository on 2026-08-25: a ratification, a
`git push` that exited 0, a clean `git status`, and three ledger commits that
stayed on the machine for a day.

[0062](0062-a-fetch-writes-an-upstream-ref.md) already gave `doctor` the
outbound finding, and it is well worded — it names the two refs, the count, the
exact command, and the sentence about the side ref. It was in force on
2026-08-25 and the ledger still sat unpublished. **The gap is not information.
It is the moment.** `doctor` is a verb a person chooses to run, and the moment
that matters is the one where they are already publishing.

`git status` cannot be told about this: there is no config, no hook and no ref
report that admits external state, and the only hook that runs during a status
does not fire on a settled tree, has its exit code ignored, and is bypassed by
`core.hooksPath`. That leaves the push.

Two mechanisms reach a push, and only one of them is safe.

**`remote.origin.push` was measured in full and rejected.** Configuring
`refs/kept/ledger*:refs/kept/ledger*` there publishes the ledger beside a bare
`git push` — but a refspec on the command line overrides it entirely, so
`git push origin main` leaves the ledger behind; `--all` ignores it; setting it
replaces `push.default` rather than adding to it, so the operator's own branch
push changes behavior; and **`git push --force` carries the force into the
ledger and rewinds the published copy at exit 0**. That last one is the
2026-08-22 incident's exact shape — a forced refspec, exit 0, no output — moved
to the outbound side, where it destroys the copy 0062 made unreachable from the
network on the inbound side.

**A `pre-push` hook fires on every push regardless of refspec** — measured on
seven forms: bare, `origin main`, `-u`, `-u origin topic`, `--all`, `--tags`,
`--follow-tags`. It is the only mechanism that is not defeated by the shape of
the command.

## Decision

`kept init` writes `<git-common-dir>/hooks/pre-push`. It **warns and publishes
nothing**.

**Warn, not publish.** A publishing hook closes the gap outright, and it was the
scouting recommendation, on the ground that a line the operator must read and
act on is the shape that already failed. It is rejected on what it makes kept
into. kept has never opened a socket; `kept ledger sync` states that boundary in
its own help, and 0062 declined to push for the operator on it. A hook kept
writes and git runs is still kept arranging a network write, and the write it
arranges cannot be conditioned: measured, a hook's environment and stdin are
**byte-identical** between a real push and `git push --dry-run`, so a publishing
hook really publishes during a dry run and there is no signal to condition on.
A warning is immune to that by construction — measured, `--dry-run` prints the
warning, moves no ref, and leaves origin byte-identical.

Because the warning is what ships, and because the failure mode of a warning is
documented, the warning is held to what it must be: it says **the count, the
command, and why this push is silent about it**, in `doctor`'s own register, and
nothing else.

```
kept: the ledger holds 3 commits origin did not have as of the last fetch.
      git push does not carry a side ref, so this push leaves them behind:

          git push origin refs/kept/ledger:refs/kept/ledger
```

Six properties are load-bearing, and each is measured.

**It exits 0 on every path.** A warning that refuses is a publish with worse
manners. Nothing about a push is held hostage to a ledger.

**It is silent unless it has something to say, and the comparison is offline.**
Two local refs and one `rev-list`, which is `Divergence` spelled in shell. No
round trip, no credential, no deadline to bound.

**It self-quiets, and only because of 0062.** Publishing the ledger updates
`refs/kept/upstream/ledger`, because `kept init` configured the refspec that
maps it — measured both ways: with that refspec the upstream ref advances on the
push, and without it (a clone that never adopted) it does not and the warning
repeats. 0062's consequence is what makes this check free.

**It reads the ref list rather than discarding it.** stdin's third field is the
remote ref, so a push already carrying `refs/kept/ledger` says nothing — without
it, running the command the hook just printed prints it again. Reading to the
end is also what a `pre-push` hook owes git.

**A fork is told apart from an unpublished ledger** by one
`rev-list --left-right --count`, and is handed `kept doctor` rather than a push
origin would refuse — whose refusal git annotates with `use 'git pull' before
pushing again`, advice that is wrong for an append-only record.

**An upstream ref nothing has fetched speaks, and is handed the publication
rather than a fetch.** That state is the fresh adoption — clone, `kept init`,
ratify, push — and it is the one this exists for; measured, an earlier draft
that kept silent there said nothing at all through exactly the sequence the
decision is about. It cannot claim origin lacks the acts, so it says *nothing
here has heard from origin*, and the command it hands over is the push: a fetch
that brings nothing leaves the state unchanged, so naming one would repeat
itself forever, while the push is unforced — it publishes where origin has
nothing or is behind, and origin refuses it where origin is ahead. Either
outcome resolves the state, and a successful one writes the upstream ref and
quiets it.

### Written by `kept init`, not behind a flag

The three git settings 0062 and [0054](0054-a-ledger-that-went-backwards-is-a-refusal.md)
need are written by `init` without asking, and this is the fourth. A workspace
that has to ask for its own safety net does not have one. It is inert where
there is no ledger, it is deleted by deleting one file, and the file says so on
its second line.

### An existing `pre-push` hook is refused, not appended to or shadowed

Git runs exactly one `pre-push` hook. Appending to one is editing a program a
person wrote, which
[`policy:nothing-kept-writes-rewrites-what-a-person-wrote`](../policy/nothing-kept-writes-rewrites-what-a-person-wrote.md)
refuses; writing a second file beside it that nothing invokes is worse, because
it reads as installed and is not — the defect
[`BranchRefspec`](0062-a-fetch-writes-an-upstream-ref.md) exists to describe on
another key. So the file is left exactly as it is and both `init` and `doctor`
say so, naming what still reports the ledger.

kept's own hook is recognized by one marker line and **never rewritten**, which
is `kept init`'s existing rule: write what is missing, never rewrite what is
there. A person who edited kept's hook keeps their edit.

### `core.hooksPath` is reported and never written into

A repository configured to look elsewhere runs no hook from `.git/hooks`, so
kept writes none there and none anywhere else. Not into the configured
directory: it belongs to whatever hook runner set it, a relative value is
resolved per working tree — measured, `myhooks` found a hook from the primary
checkout and none from a linked worktree of the same repository — and a value
set globally would reach every repository on the machine, which is outside
everything `kept init` writes. Not into `.git/hooks` either, because a file git
will never read, reported as *wrote*, is the same lie the file-beside-it would
be.

Which directory git reads is git's own answer: `git rev-parse --git-path hooks`
returns `core.hooksPath` when it is set and the common hooks directory when it
is not, so the check compares that against `--git-common-dir`/`hooks` and
resolves nothing itself.

### `doctor` reports all three

A repository with no hook, a `pre-push` kept did not write, and a
`core.hooksPath` that bypasses it. Each passes
[0050](0050-doctor-reports-an-unchecked-instrument.md)'s three conditions:
nothing refuses any of them, nothing else in kept reports them, and what kept
does about them is bounded by what `init` writes. It is a **separate finding**
from the unpublished ledger above it — that one reports the state, and this one
reports whether anyone will be told about the state while they are looking.

## Consequences

**Nothing depends on the hook having run.** `git push --no-verify` skips it and
so does `git -c core.hooksPath=/nonexistent push`. That is acceptable here for
the reason it is not acceptable for a guard: this is a convenience, and the
`doctor` finding it points at does not go away.

**A hook is not cloned.** A fresh clone has none until `kept init` runs there —
the same sentence 0062 writes about the refspec and 0054 about the reflog, and
`doctor` reports it for the same reason.

**It costs one `git rev-parse` on a push in a repository with no ledger, and
three git invocations where there is one.** Measured over 30 pushes a side
against a local bare origin, twice: 46 ms a push without it, 61 ms with it —
**about +15 ms**, and about +4 ms where there is no ledger to compare.

**It warns on a push to any remote,** because the fact is about origin whichever
remote is being pushed to, and the command it prints is correct there. Narrowing
it to origin would silence it for an operator who pushes through a fork.

**An operator who keeps a ledger and never publishes it is warned on every
push.** That is the finding, not a defect — `doctor` says the same — and the
answer is the one the file's own second line gives: delete it. Nothing depends
on it having run.

**It is the first file kept writes into the git directory.** Everything before
this was a config value in `remote.*` or `core.*`. The bound stays narrow: one
file, one name, in the shared hooks directory, written only where nothing is
there.

## Alternatives

**Publish rather than warn.** Rejected above, on the boundary and on
`--dry-run`, having been the scouting recommendation. The measurement that would
reopen it is another unpublished-for-a-day incident with this hook installed.

**`remote.origin.push`.** Measured in full and rejected: `--force` rewinds the
published ledger at exit 0, a command-line refspec and `--all` both ignore it,
and it changes what a bare `git push` means.

**A flag on `init`, or a `doctor` recommendation the operator installs.** It is
the shape `kept new` uses for `ratify`, and it is right there because
ratification is a person's act. Installing a warning is not.

**A file `git status` would show,** such as `.kept/UNPUBLISHED`. A cache that
can disagree with its source, maintainable only by a writing verb, so a
read-only checkout would show a stale one forever.

**`git notes`, `post-merge`, `post-checkout`, `reference-transaction`.** Notes
put standing in a second place. The other three run kept on events that did not
ask, which is a scheduler by another name — [0025](0025-no-scheduler-cadence-is-change-driven.md).
