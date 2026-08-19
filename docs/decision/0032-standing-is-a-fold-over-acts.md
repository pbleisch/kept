---
id: 0032-standing-is-a-fold-over-acts
type: decision
title: 0032 — Standing is a fold over acts, and a release retires the artifact
---

# 0032 — Standing is a fold over acts, and a release retires the artifact

**Status** accepted · settles how the register reads the ledger, which
[0001](0001-standing-and-truth-live-in-the-ledger.md) describes for one record
and never for a stream · depends on [0011](0011-tracked-formats-must-merge.md)'s
fold-by-event-time rule and [0013](0013-the-gate-makes-concurrency-safe.md)'s
`seq`

## Context

0001 says standing is derived by comparing a ratification's content hash against
the artifact's current one, and shows a single record:

```
ratified(promise:core-packages-are-tested, sha256:ab12…, norm:1, paul@…, at)
```

A corpus in use has more than one. A promise is ratified, revised, ratified
again, released, ratified after that. `model.md` says the register is "the
derived view over the ledger: fold the ratifications, compare each against the
artifact's current hash, and what survives is what obliges anyone right now" —
which names the shape and leaves two questions that only appear once there is a
second record.

**Does a ratification stay in force, or does the latest one win?** The two
disagree in a case that will happen: revise a promise, decide the revision was
wrong, and put the old wording back. Latest-wins says the operator must ratify
again. Stay-in-force says the grant they never withdrew still applies.

**What does a release retire — the artifact, or the wording it named?** They
disagree in a case that matters more: release a promise, then edit its file back
into a shape somebody once approved.

Both are one-way. They decide what a stream of records already written *means*,
so changing the answer later reinterprets history rather than changing
behaviour.

## Decision

**The register folds an artifact's acts in order and keeps a set of
content hashes with standing in force. A ratification adds its hash to the set.
A release empties it. An artifact is ratified when the set contains the hash its
content produces now.**

```
ratified   the set holds this artifact's current hash
released   the set is empty and the last act was a release
absent     the set is not empty and no artifact claims this id
unratified anything else — never granted, or granted at a version this is not
```

The order the fold walks is 0011's and 0013's, unchanged: event time, then the
writing run, then that run's `seq`. Never file order, because union merge
interleaves two checkouts' appends arbitrarily.

### A grant stays in force until it is released

Standing attaches to a content version — `model.md` says so outright, and gives
the reason: *the operator agreed to the old wording*. Reverting to that wording
is therefore not a new commitment needing a new act; it is the same
commitment, and the grant that covers it was never withdrawn.

Latest-wins would say otherwise, and would be saying that a ratification expires
when a later one is made. Nothing in the model says that, and it would make
`git revert` on a corpus produce a promise the operator has to re-approve while
being unable to say what changed.

### A release retires the artifact, not a wording

Clearing the whole set rather than removing one hash. The act is *retiring a
promise* — "a promise stops being held to", in the acts table — and a retirement
that only covered the exact wording in force at the time would be undone by
editing the file into any other wording that had ever been ratified.

That failure has the same shape as the one 0021 removed when it made deletion
stop being a back door to release: a way to change what obliges you that is not
one of the three acts reserved to a person. Getting back is `kept ratify`, which
is a person's act, and that is the whole route.

### Absence keeps standing, and a release from absence is the way out

An artifact the ledger names and the corpus does not is **absent** when its set
is non-empty. It keeps its standing and says its content is gone, per
[0021](0021-identity-is-authored-absence-is-not-an-act.md) — never read as a
release, because absence cannot be told apart from a bad merge, a partial
checkout or a corpus root pointed at the wrong place.

So `kept release` has to work on an artifact whose file is gone. That is not an
edge case to tolerate; it is the case the verb exists for, and it is the reason
a release names an artifact id rather than a content version.

### A grant is compared on its rules as well as its digest

A ratification records the canonicalization version it was computed under
([0022](0022-canonical-content.md)), and the fold compares both. A digest that
agrees under different rules answered a different question.

Re-hashing an old record under its own rules is not built, because there has
only ever been one version and there is nothing to re-hash. What is built is the
honesty: an artifact whose every grant was computed under other rules is
reported as **unrehashed**, not as revised. The two send a reader to different
places — revised means somebody edited the artifact, unrehashed means nobody did
and the tool changed underneath it — and 0022 exists precisely so that a
refinement is not mistaken for a corpus-wide edit. Getting that wrong would be
the failure 0022 was written to prevent, arriving through a different door.

### There is no invalidation step, and no place to add one

The register writes nothing. It is a function of two inputs — the acts, and
what each artifact says now — computed on every read. An edit made by any editor,
by any process, at any time is accounted for the next time anything asks,
because nothing was ever remembered.

This is the load-bearing property of increment 2 and the reason it is worth an
ADR rather than a comment. The alternatives all work by *marking* something, and
a mark is a second place the answer lives; when the two disagree, the one that
is wrong is the one anybody reads. 0001 rejects a write hook for that reason and
this extends the same rejection to the stream: no compaction, no current-state
projection, no "standing" column anywhere. `kept status` recomputes, and its
cost is a corpus walk.

## Alternatives

**Latest ratification wins.** Rejected above. It is simpler to describe and it
makes a revert cost a human act for no reason anyone could state.

**A release retires only the content version it names.** Rejected: it makes
editing a file into an old shape a way to un-retire a promise, which is the back
door 0021 closed from the other side.

**Ratifications expire after some interval.** Rejected. Nothing bounds how long
a commitment lasts, and an expiry would create standing that lapses without
anyone deciding — the opposite of the invariant that standing is created and
retired only by a person. Staleness in
[0013](0013-the-gate-makes-concurrency-safe.md) bounds how long a *run* may act
on a view it has not refreshed, which is a different question and already
answered.

**A compacted current-state file, rebuilt on write.** Rejected: it is a cache
that can disagree with its source, which is the thing 0001 says is worse than
none. It also would have to be written by a reader, which turns `kept status`
into something that cannot be run against a read-only checkout.

**Fold in file order and treat the stream as a log.** Rejected by 0011 directly.
It would be correct in every single-run test and silently wrong the first time
two checkouts merged.

## What this does not decide

One thing, and it is a property of the choice above rather than a gap in it.

**A wording cannot be withdrawn on its own.** The set grows only by a person's
act and is emptied only by a release, so every wording ever ratified stays
acceptable for as long as the artifact has standing at all. There is no way to
say *this old wording is no longer acceptable and the promise still stands*.
Release is the only thing that removes a member and it removes all of them,
because it retires the artifact; the route back is a fresh ratification of the
current wording. That route works and it misdescribes itself — the ledger
records a retirement, and nothing distinguishes it from one that was meant.

This is written down because it follows from stay-in-force rather than from
anything anyone chose separately, and someone reading the fold will otherwise
re-derive it and wonder whether it was noticed. It was, and it is not worth a
mechanism. Every member of the set is a wording the operator did approve, and a
member that is not the content on disk obliges nobody — so reaching the bad case
takes a deliberate revert to an old wording, made by the one person who could
have released the promise instead. The alternative is a second kind of
withdrawal: a fourth act reserved to a person, a partial inverse of ratify, and
a register in which "what is in force" has two ways of being narrowed. That is a
worse trade than the asymmetry, so the asymmetry stands.

## Consequences

- **`kept status` is O(corpus + ledger) every time it runs.** Accepted at
  one-operator scale, and open question 7 — whether v1 needs the derived index —
  is where it would be revisited, by measuring rather than in advance.
- **A revert restores standing with no act.** Worth saying out loud, because it
  reads as surprising until it is read as *the operator already agreed to this
  exact text and never took it back*.
- **The ledger keeps every ratification forever**, and the set is derived from
  all of them. Pruning acts would change what is in force, which is why
  `config.yaml` marks them as never pruned.
- **A release is recorded with the content version in force when it was
  performed.** It is not what the fold keys on — the fold keys on the artifact —
  but it says what the operator was looking at when they retired it, which is
  the question anyone reading the record a year later will have.
- **`kept doctor` gains a check here too**: an act naming an artifact id no
  corpus claims, which is either an absent artifact or an id edited by hand.
