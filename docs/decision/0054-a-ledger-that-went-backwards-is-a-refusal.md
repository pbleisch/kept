---
id: 0054-a-ledger-that-went-backwards-is-a-refusal
type: decision
title: 0054 — A ledger that went backwards is a refusal
fields:
  statement: >-
    The ledger ref is logged, and a value it has held is never unreachable from the value
    it holds now. `kept init` sets `core.logAllRefUpdates` to `always`, `doctor` reports a
    repository where it is not, and every verb that reads or writes the ledger refuses at
    exit 4 — naming the unreachable value — when the reflog shows the ref has gone
    backwards.
links:
  - { target: decision:0016-the-ledger-lives-on-its-own-ref, relation: extends }
  - { target: decision:0018-what-survives-a-crash, relation: extends }
  - { target: decision:0027-the-machine-facing-surface, relation: depends-on }
  - { target: decision:0050-doctor-reports-an-unchecked-instrument, relation: depends-on }
---

# 0054 — A ledger that went backwards is a refusal

## Context

[0016](0016-the-ledger-lives-on-its-own-ref.md) unforced the ledger's fetch
refspec, which closes the vector that erased two ratifications from this
repository on 2026-08-22. What it does not close is everything else that can
move a ref: `git update-ref`, a push into the repository, a hand-edited config,
and every clone `kept init` has not corrected. Against all of those kept has no
detection at all and no recovery but luck — the lost head on 2026-08-22 was
recovered because an unreferenced object happened to survive long enough to be
found by scanning the loose object store, and a `git gc` in that window would
have made it unrecoverable.

kept reads only the tip tree. `RefStore.content` is `ls-tree` plus
`show <rev>:<path>`; nothing walks history and nothing compares the ref against
anything. The only adjacent distinction is `ledger.Empty` — an absent stream
versus an empty one — which exists so a clone made without the refspec does not
read as a corpus nobody ever ratified. Good distinction, wrong axis: it
separates *nothing was ever written* from *something was*, and cannot tell
ninety records from ninety-two.

[0018](0018-what-survives-a-crash.md) gives the ledger a guarantee about one
torn record — *every record complete or discardable* — and has no row for the
ref being replaced. Its guarantee is about record framing; the failure was about
the ref. That is the gap.

0016 considered a reflog and rejected it *as the primary answer*, saying "worth
having; not this". That judgment was right about priority and its fact base has
one error, which this decision corrects:

- **"`core.logAllRefUpdates` does not cover `refs/kept/`."** True of the value
  `true`, which is the value every repository has, because it is what `git init`
  writes. It is not true of `always`, which 0016 does not name. Git logs a ref
  update only for a hardcoded set of prefixes — `refs/heads/`, `refs/remotes/`,
  `refs/notes/` and `HEAD`; `always` lifts that restriction for every ref under
  `refs/`. Measured on git 2.53 under both the `files` and `reftable` backends:
  under `true` the ledger ref accumulates no reflog at all, and under `always`
  every update to it is recorded.
- **"a per-repository configuration kept would have to write and then depend
  on."** True, and 0016 itself now writes one, corrects one it previously wrote,
  reads the config back rather than trusting the write, and reports through
  `doctor` when a repository lacks it. This is a strictly smaller commitment:
  additive, local, colliding with nothing, and no behavior of kept's depends on
  its presence — the check answers *nothing has been recorded* when it is absent.
- **"it makes the loss survivable rather than preventing it"**, and **"a
  procedure somebody has to know to run, in a window before `git gc`."** Both
  true, and both are why this is the detection and recovery layer rather than
  the prevention layer, which has shipped. The window is `gc.reflogExpire`, 90
  days by default, because reflog entries are garbage-collection anchors. And
  nobody has to know the procedure, because kept refuses and prints it.

## Decision

**The ledger ref is logged, and a value it has held is never unreachable from
the value it holds now.**

**`ConfigureFetch` also sets `core.logAllRefUpdates` to `always`.** Git then
writes a reflog for `refs/kept/ledger` on every update, whoever performs it,
with the reason attached — `fetch origin --prune --quiet: forced-update` is what
the 2026-08-22 fetch would have left. This is the property no file kept writes
can have, because kept is not running during a `git fetch`. The reflog lives in
the common directory, so one setting covers every worktree of a repository, and
its entries anchor a lost commit against garbage collection, which is the
recovery half rather than merely the detection half.

It is the config value rather than a pre-created log file. Git logs a ref "only
when the file exists", and deleting the ref deletes the file: measured, a
touched log file stops logging permanently after the ref is deleted once, while
`always` recreates it on the next update. A witness that silently stops
witnessing is worse than none.

The setting is repository-wide — git offers no way to log one ref hierarchy and
not another — so every tag and remote-tracking ref is logged too. Measured: 29 KB
of logs for a repository holding 251 refs, about 115 bytes per entry and about
140 for kept's own, whose reason strings are longer. It is written over an
explicit `false`, which is the one value
here kept did not author: a repository that logs no ref update cannot record
what the ledger held, so adopting it without this would be adopting it into a
guarantee that cannot hold there. `kept init` says what it changed, which is the
treatment 0016 gave the refspec.

**`doctor` reports a repository where the setting is not `always`**, beside the
forced-refspec finding 0016 added. Same [0050](0050-doctor-reports-an-unchecked-instrument.md)
argument, same remedy: nothing refuses it, git accepts it, kept accepts it, the
corpus loads; no other kept verb reads the setting; and `init` is the adoption
verb, so an operator who never re-inits is never told. It reports what git will
do next rather than how many entries exist, because a correctly configured
repository whose ledger has not moved since adoption has an empty reflog and no
defect.

**kept refuses to act when the ledger ref has gone backwards.** Every value the
reflog records must be an ancestor of the value the ref holds now. When one is
not, kept names the unreachable value, prints the recovery, and exits 4 — the
code [0027](0027-the-machine-facing-surface.md) reserves for a run that refused
to answer on a stale view. `kept list` and `kept doctor` do not refuse: they
read the corpus, and they are what an operator has left in that state.

The rule is stated over git's DAG rather than over the ledger's contents,
because the failure replaces the ref rather than editing the records. It is
self-clearing under the correct repair and only under it: a union commit naming
the lost value — what the 2026-08-22 repair actually used — makes it an ancestor
again and the refusal stops, while a repair that discards the lost records
leaves it standing.

**Both ends of the reflog are read.** `git reflog show` reports the value each
entry moved the ref *to*, and the value it moved *from* survives only on the
oldest entry, reachable as `Ref@{n}` where n is the entry count. Reading only
the new values has a blind spot exactly where the guard matters most: a
repository that already held a ledger when logging was switched on, whose first
recorded update is the rewind, reports clean. That is `kept init` on a live
repository followed by a bad fetch, and it is measured rather than imagined.

**A repository with no reflog for the ref answers *nothing has been recorded*.**
That is neither a rewind nor a failure — it is the fresh-clone state — and it is
the same distinction `ErrNoStream` draws one level down. Collapsing it would
make every clone look like a broken one, which is the mistake one level up from
collapsing `unknown` into `not-kept`.

**kept's own ref writes name themselves**, by passing `-m` to `update-ref`, so
an entry that is not one of kept's stands out immediately. It is local
diagnostic text: nothing reads it back and nothing is addressed by it.

## Alternatives

**A high-water mark kept writes itself.** Rejected on placement, not principle.
Tracked in `.kept/` it is a single file read-modify-written in place and keyed by
nothing, which
[`policy:everything-tracked-in-git-merges-without-a-human`](../policy/everything-tracked-in-git-merges-without-a-human.md)
refuses by name; it would conflict on every merge of two branches that both
ticked. Untracked it is per-worktree and lost on clone. In `.git/` it is a
hand-built, worse copy of a file git already writes — and the decisive
difference is that git writes its copy during a `git fetch`, when kept is not
running.

**Recording the act count, a hash chain, the ratification set, or a monotonic
sequence.** All four summarize the ledger's *contents*, and the failure replaces
its *location*. A count cannot tell truncation from replacement — ninety lines
replaced by a different ninety passes. A chain needs a field in the record, so
it lives inside the thing that gets removed. The ratification set is standing,
and caching standing is forbidden by name; the cache would also have to be
written by a reader, making `kept status` something that cannot run against a
read-only checkout. A sequence is a counter, and ids are minted rather than
counted.

**A `reference-transaction` hook that vetoes the write.** It reaches writers the
refspec cannot and protects every worktree, because hooks live in the common
directory. Rejected as a default: it taxes every ref transaction in the
repository with a fork and exec, paid by every commit forever to protect one
ref; it is disabled outright by `git -c core.hooksPath=…`, which hook managers
set routinely; and the naive form breaks `git pack-refs`, because packing a
loose ref is expressed as a deletion. 0016 closed the measured vector at no
recurring cost.

**Reporting the missing reflog by counting entries rather than reading the
setting.** Rejected: a correctly configured repository whose ledger has not
moved since adoption has zero entries and no defect, so the finding would fire
on the workspaces that are right and say nothing an operator can act on.

**Moving the ledger to a real branch.** Rejected. It would inherit a reflog, but
so does the side ref for one config line; forge branch protection, but this
remote has no branches at all and the damage was to the local authoritative
copy; merge drivers, which already apply to paths on the side ref. What it would
cost is putting the durable record in the namespace with the heaviest automated
traffic — a worktree pool creating, rebasing and deleting branches is what ran
the fetch in the first place. 0016's three reasons are untouched.

**Pushing the ledger at every exit, so the remote is never stale.** Rejected as
automatic, beyond 0016's own reasons. Had it been in place, the first tick after
the 2026-08-22 rewind would have published the damaged ledger to origin and
overwritten the last copy of the good one, sixteen minutes after the loss, as a
clean fast-forward. Automatic push turns a local loss into a distributed one,
silently. An explicit verb, refused when this check fails, is the shape it
should take if it is built.

## Consequences

- **This makes a loss survivable, not impossible**, and every verb that mentions
  it says so. Four things it does not cover: deleting the ref takes its reflog
  with it and nothing here recovers that; `.git/logs` is an ordinary directory
  anyone can remove; entries expire at `gc.reflogExpire`, 90 days by default; and
  a clone is unprotected until `kept init` runs there, because reflogs are not
  cloned and the setting reverts to git's default. `doctor` reports the last of
  those and nothing refuses it. A guard that oversells itself is worse than one
  that states its edges.
- **Detection is per-run.** kept ticks once and exits ([0025](0025-no-scheduler-cadence-is-change-driven.md)),
  so a repository fetched daily and ticked weekly is checked weekly. Closing that
  would want a daemon, which 0025 refuses.
- **Exit 4 is reachable, and one thing reaches it.** 0027's table gains a row
  rather than changing: 1 is still drift, 3 is still what kept could not decide.
  A rewound ledger is neither — the question was never properly put.
- **`ConfigureFetch` returns two outcomes rather than one**, because configuring
  the refspec and turning logging on are separate acts and `init` says which
  happened.
- **`core.logAllRefUpdates` is repository-wide**, so adopting a workspace changes
  how git treats every ref in it, including an explicit `false` somebody set.
  Measured storage cost: 29 KB for 251 refs. `kept init` reports it.
- **The check costs about 15 ms per run**, and that is a real cost rather than a
  rounding error: measured, `kept status` over this corpus goes from 32 ms to
  47 ms. It is four local git invocations — the tip, the reflog, the oldest
  entry's pre-image, and one `rev-list` — and the walk itself is the cheap part,
  5 ms at 25 recorded values and 7 ms at 120, because `rev-list` traverses the
  DAG once however many entry points it is given. The per-value `is-ancestor`
  loop runs only in the failure path. For comparison, `kept drift` over this
  corpus is 11 ms.
- **0027 needs nothing.** Its table already reads *"refused — stale view,
  unconfigured provider, failed canary, budget"* for 4. This is the stale view,
  so the code is being used as reserved rather than widened.
- **0018 is owed a row and does not get one here.** Its tier table gives the
  ledger *"every record complete or discardable"*, which is about record framing
  and has nothing to say about the ref being replaced. The table is inside
  0018's `## Decision`, so `body.decision` binds and adding the row would drop
  its standing. It is left as it is, and the row is proposed rather than
  written.
- **0016 says two things that this makes inaccurate, and one of them binds.** Its
  Context and its reflog alternative both evaluate `core.logAllRefUpdates: true`,
  which indeed does not cover `refs/kept/`, and neither names `always`; those
  sections do not bind, so correcting them costs nothing. But its Decision
  section says *"Since a side ref has no reflog, that refusal is the only warning
  there is"*, and in a repository kept has configured that is no longer so.
  `body.decision` binds, so changing it drops 0016's standing and needs a
  person's grant. It is left exactly as it is here, and the correction is
  proposed rather than made.
- **Every guard here has a test that breaks it.** The 2026-08-22 event is
  reproduced rather than abstracted — a ledger ahead of a stale remote, the
  forced refspec the repository carried, one ordinary fetch — and asserted both
  ways: unlogged, the fetch takes the ledger and the repository holds no record
  that it ever held another value; logged, the reflog names the lost value and
  the word `forced-update`, and the next kept run exits 4. The union repair is
  asserted to clear it and a plain append is asserted not to, because a guard
  that fires and never stops firing is a guard that gets disabled.
