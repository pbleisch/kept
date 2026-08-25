---
id: 0063-an-act-names-the-world-it-was-performed-in
type: decision
title: 0063 — An act names the world it was performed in, advisory, and doctor reports a grant nothing landed
fields:
  statement: >-
    A ratification and a release record the world they were performed in and
    whether that tree held changes no commit did; the world is never read to
    decide standing, and kept doctor reports a grant in force whose commit this
    tree cannot reach.
links:
  - { target: decision:0016-the-ledger-lives-on-its-own-ref, relation: amends }
  - { target: decision:0013-the-gate-makes-concurrency-safe, relation: amends }
  - { target: decision:0014-an-observation-names-its-world, relation: extends }
  - { target: decision:0035-a-verdict-names-its-world-in-the-record, relation: extends }
  - { target: decision:0050-doctor-reports-an-unchecked-instrument, relation: depends-on }
---

# 0063 — An act names the world it was performed in, advisory, and doctor reports a grant nothing landed

## Context

[0016](0016-the-ledger-lives-on-its-own-ref.md) puts the ledger on
`refs/kept/ledger`, which is repository-wide and not branch-scoped. A grant
binds a content hash rather than a location. Put those two together and
`kept ratify` on a task branch lands the act at once, for every worktree, before
anybody has reviewed the branch — and if the branch is then declined, the grant
stays in force for a wording that exists nowhere.

**It is not wrong.** `granted` is a *set* of content hashes, so the grant only
ever applies to the exact bytes a person read and approved, and the record is
true: a person did ratify that content. Measured on this repository, 0016's own
revision spent eight minutes in that state — ratified at 21:57, merged at 22:05 —
and the ordering `AGENTS.md` prescribes was satisfied by promptness rather than
by anything checking.

What it is, is unreportable. `AGENTS.md` says so in as many words: *nothing
reports the extra grant*.

### Why the obvious check does not work

The tempting check is *a grant whose hash matches no artifact in this corpus*.
`internal/register` explains why that is useless, and the sentence is load-bearing:

> `granted` is every content version **with standing in force**, keyed by hash.
> A set rather than the last one, because standing attaches to a content
> version: revert a promise to a wording the operator already ratified and that
> grant applies again, exactly as it did before the revision.

So a grant matching nothing here is the **normal** state of every artifact that
has ever been revised, and it cannot on its own tell apart:

- a **superseded** grant — that wording landed once, and is deliberately still in force
- an **unsupported** grant — that wording landed nowhere, because its branch was declined

An act records artifact, hash, by, at and note. Nothing in it says whether that
content ever existed in this history, so nothing can make the distinction.

### The asymmetry

kept already decided that a machine-derived answer names the tree it was reached
in. An observation names its world ([0014](0014-an-observation-names-its-world.md)).
A verdict names its world and `ledger.Reached` **refuses one without a world
outright** ([0035](0035-a-verdict-names-its-world-in-the-record.md)) — *a record
written without one cannot be backfilled*.

**An act — the more consequential record, the one only a person may perform, the
only thing that creates standing — named none.** Neither does the run behind it.
That is worth stating on its own, because it does not depend on anyone agreeing
that branch ratification is bad: it is a structural observation about which
records in kept are situated and which are not.

## Decision

**An act names the world it was performed in. The world is advisory. `kept
doctor` reports a grant in force whose commit this tree cannot reach.**

### The world is 0014's shape, and nothing new

`ledger.Where` carries the existing ratified `model.World` — remote, ref,
commit, tree, thread — and one field beside it. No new world shape, and no field
added to `model.World`: a world is folded into the observation preimage
[0031](0031-the-binding-preimage.md) freezes at `obs:1`, so extending it there
is a version bump rather than an edit, and it would move a record that has
nothing to do with this.

An act is not content-addressed, and `internal/ledger/record.go` already says a
JSON field is additive, so nothing else moves either.

### Advisory is the whole safety property

**The world is never consulted to decide standing.** Standing stays keyed to a
content version and to nothing else. A grant made conditional on a world would
evaporate under an ordinary rebase, merge or clone — the same content, the same
person, a different answer depending on which checkout asked — and that is the
option this decision does not take.

That is also what keeps `record.go`'s distribution argument intact. Its first
sentence — *it carries no world* — is now false, and the argument behind it does
not fall: standing still keys on a content hash, which is world-independent by
construction, so a ratification still means the same thing in every tree it is
read in and still needs nothing added for distribution, ever. The comment is
revised to say which half changed and which did not, so a future reader does not
read the field as the reasoning having been overturned. [0013](0013-the-gate-makes-concurrency-safe.md)'s
*what is one-way* table gains a row and loses none of its argument.

The property gets a test rather than a paragraph: the same content version,
granted by the same person, is decided identically with a world and without one
— and the world in that test is a hostile one, naming a tree nothing has held, a
commit no repository holds and a thread that never ran.

### The discriminator is reachability, not equality

Equality is useless: every ratification is against a tree this is not the moment
anything lands. What separates the two cases is whether the commit the act was
performed in is one **this tree** can reach.

| | |
|---|---|
| the commit is reachable | that wording landed here once, and this is an artifact that was revised — silence |
| the commit is not reachable | it landed nowhere the reader can see — **finding** |
| the act names no world | nobody recorded one — silence |

From HEAD rather than from every ref this repository holds, and that is the
decision rather than an implementation detail: the question is whether the
wording landed in *the tree being asked in*, which is what 0014 already makes
true of a verdict. Standing on the branch that granted it, the answer is yes and
there is nothing to report. Standing on the branch that declined it, the answer
is no, whether or not that branch's ref is still lying around.

A commit this repository does not hold at all is **unreachable** rather than an
error: a shallow clone says the same thing, which is why the finding is worded
*this tree cannot reach it* rather than *it does not exist*.

### A dirty tree is a second finding, not the first and not silence

`World.Tree` is kept's digest over a workspace, and it differs from the commit's
tree whenever the working copy was dirty. A ratification performed from a dirty
tree hashed bytes that may never have been committed at all, so **the
reachability of its commit proves nothing about those bytes** — the one piece of
evidence an act carries stops being evidence.

It cannot be derived afterwards. A commit's tree cannot be turned into kept's
digest without checking it out, so the fact is recorded at the time, as
`Where.Uncommitted`, beside the world rather than inside it.

It is reported **separately** from the unsupported grant, for the reason
`register` keeps `Revised` and `Unrehashed` apart: the two send a reader to
different places. *Landed nowhere* says a branch was declined and the grant is a
pre-authorization nothing can withdraw selectively. *Hashed out of an
uncommitted tree* says the commit landed and cannot vouch for the wording, and
the reader has to find out for themselves whether those bytes ever landed. A
grant that is both — dirty **and** unreachable — is reported once, on the
stronger of the two.

**It does not fire on the ordinary accident.** Edit, ratify, then commit is a
sequence that ends with the granted wording being what the artifact says, and a
grant in force for the current content is never examined. Only a dirty-tree
grant whose wording has *also* gone from the corpus is reported, which is exactly
the ambiguous case.

### It is a finding, and reporting is all it does

**Detection only.** Nothing refuses, blocks or prevents ratifying from a task
branch, and the exit code is unchanged: 0027's rows are about commitments and
evidence, and this is neither. `--fix` is untouched, because it writes identity.

It meets [0050](0050-doctor-reports-an-unchecked-instrument.md)'s three-part
test, which 0050 says in as many words is what bounds the next finding. Nothing
rejects a grant like this — kept accepts it, git accepts it, the corpus loads.
Nothing else reads it: `status`, `list`, `why` and `drift` all call such a
repository clean, which is the whole complaint. And kept does nothing about it.

### Three kinds of silence, each for its own reason

An act with **no world** names nothing. Every act written before this decision is
one — the entire existing ledger — and *nobody recorded one* is not *it never
landed*. An act whose world names a tree and **no commit** was performed where
there was no commit to name, outside a repository or on an unborn branch, and
there is no history for it to be missing from. And a repository that **cannot
answer** — no HEAD, no git, an object it does not hold the ancestry of —
establishes nothing. That last split is [0004](0004-gatherers-return-observations.md)'s,
one level up: a question nobody could put is not a negative answer.

## Alternatives

**Do nothing beyond the `AGENTS.md` paragraph.** Defensible, and rejected on the
one question that decided this: *is the detection cheaply possible today?* It is
not — an act carries nothing that could tell the two cases apart — so the change
is worth making. Documenting a workaround settles how to work around it; it does
not give the situation a reader.

**Refuse to ratify off the default branch.** A policy with `applies_to: ratify`,
which the policy schema already admits. **Considered and rejected.** It makes
ratification depend on where the operator is standing, which is a new kind of
dependency for an act that is world-independent by construction — and it blocks
ratifying a revision with the revision in front of you, which is the legitimate
workflow and is exactly what happened with 0016 itself.

**Make standing conditional on the world.** Rejected, and it is the one this
decision is careful about: a grant that only applied where its commit is
reachable would evaporate under an ordinary rebase, merge or clone. Standing
attaches to a content version. That is 0001 and it is not up for revision here.

**Add the flag to `model.World` instead of beside it.** Rejected on 0031: a
world is folded into the frozen observation preimage, so a field there is an
`obs:` bump that moves every observation's address for a fact no observation has.

**Report the dirty case as an unsupported grant.** Rejected: the commit *did*
land, and saying it did not is false. Folding them would also make the count a
person reads mean two different things, which is 0050's argument against putting
an unchecked instrument in `kept list`.

**Verify the content instead of the commit** — read the artifact out of the
recorded commit and re-bind it. Rejected. It is a stronger answer and it cannot
be asked: a path is not identity ([0021](0021-identity-is-authored-absence-is-not-an-act.md)),
so a file renamed since is a false negative, and it costs a `git show` and a
parse per grant for a diagnostic.

**Record the world on `run.Record` rather than on the act.** Rejected: a run's
diagnostics are prunable, so the evidence behind a finding would age out while
the grant it is about is permanent.

## Consequences

- **Every act written from now on carries a world, and every act already written
  does not.** The existing ledger — 82 artifacts' worth — reads as unknown and
  silent, which is why the finding fires on nothing in this repository today.
  That is stated rather than hidden: a check whose first run reports the corpus
  it was written in is a check written to fit.
- **`kept ratify` walks the workspace once.** It is a verb a person types by
  hand a few times a week, and the walk is what a world is. No stat cache is
  opened for it, because the cache is the tick's and warming it here would make
  ratify a verb that writes into `.kept/` on the way past.
- **A world kept could not work out is recorded as none**, and the act is
  recorded anyway. The world is advisory, so a tree that would not walk or a git
  that would not answer costs a diagnosis later and never costs a person their
  act.
- **An artifact can be named by two findings at once.** An absent artifact whose
  branch grant is unreachable is reported by 0021's *standing and no content*
  and by this one, and the two say different things: the first says nothing
  claims the id, and the second says the wording was granted somewhere this tree
  never went. That is the ambiguity 0021 accepts — *an absence cannot be told
  apart from a bad merge* — finally getting a second sentence.
- **`doctor` runs one `rev-list` in the ordinary case.** One walk answers
  whether every recorded commit is an ancestor; a per-commit question costs an
  invocation each and is reached only when that walk says something is not.
- **Nothing in a `hash` list moves**, so the count of artifacts this
  de-ratifies is **zero**. The `world` field is on a ledger record, not on an
  artifact, and no schema mentions it.
- **A release still empties an artifact's whole set**, so there is still no way
  to withdraw one wording's grant. This makes that visible; it does not fix it,
  and fixing it is a separate decision about what a release is.
