---
id: 0033-due-is-a-digest-over-the-scope
type: decision
title: 0033 — A cadence is due when the bytes in its scope change, and kept computes that itself
links:
  - { target: decision:0025-no-scheduler-cadence-is-change-driven, relation: extends }
  - { target: decision:0014-an-observation-names-its-world, relation: amends }
---

# 0033 — A cadence is due when the bytes in its scope change, and kept computes that itself

## Context

0025 closed open question 2 by deciding that a promise is due when its inputs
moved, and named the mechanism in one line: *"The scope's tree hash is
`git rev-parse HEAD:<path>` — plumbing, no watcher, no index, no state of its
own — compared against `World.Tree` on the last observation."*

That line does not survive contact with a scope. Three things are wrong with it
and the first is silent.

**It cannot resolve a glob, and it fails by succeeding.**

```
$ git rev-parse 'HEAD:internal/**'
HEAD:internal/**
$ echo $?
0
```

It echoes the argument and exits zero. A scope is a glob: `internal/**` is what
0025 writes in its own example, and what the testdata promise writes at
`internal/corpus/testdata/corpus/promise/core-packages-are-tested.md`. A
due-check built on this would digest the literal string `HEAD:internal/**`,
which never changes, so **every globbed promise would be permanently not-due
and silently never observed again.** Nothing reports it, and the promise reads
as peacefully kept because nobody looked. `--verify` at least fails loudly and
still cannot answer.

**It cannot see the working tree, which is the case 0025 said it had to see.**
0025's own consequences say *"the honest answer is to hash the working tree's
version of the scope, so editing a file makes its promises due before the
commit"*. `git rev-parse HEAD:<path>` answers for `HEAD`; an unstaged edit and
an untracked file both leave it unchanged. `git ls-files -s` has the same
blindness for the same reason: it reads the index. The only git route that sees
the working tree is `write-tree` over a temporary index, which costs two orders
of magnitude more and writes blob objects into the object store as a side effect
of a read-only question.

**It compares two different quantities.** `World.Tree` is *the tree the gatherer
ran against* — the whole tree. A scope digest is about some paths in it. For any
scope narrower than the workspace they never agree, so every promise is due on
every tick, which is the outcome 0025 exists to avoid.

Underneath all three is a question nobody has answered: **what a scope glob
means.** Go's standard library cannot express one —
`filepath.Match("internal/**", "internal/corpus/corpus.go")` is false, and
`filepath.Match("internal/**/*_test.go", …)` silently means one intervening
segment rather than any number. Git is no help either, because git does not have
one answer:

| pattern | `git ls-files` | `git ls-tree -r HEAD` |
|---|---|---|
| `internal/**` | 52 | 0 |
| `*.go` | 45 | 0 |
| `:(glob)*.go` | 0 | `fatal: pathspec magic not supported` |

Default pathspecs let `*` cross `/`; `:(glob)` magic does not; `ls-tree` rejects
the magic outright. *Use git's globs* does not name a behaviour.

## Decision

**Due-ness is a content digest over the scope, computed by kept, in process,
over the working tree. `World.Tree` is a content digest over the workspace root,
honouring ignore rules. Both are kept's own, and neither asks git anything.**

```
due(artifact) =  never observed
              or scope_digest(now) != the scope digest on the last observation
              or age(last truth record) > max_age        # when max_age is set
```

The digest is a fold over the matched paths, each contributing its path and a
digest of its bytes, ordered by path. Paths are what the scope's globs match
under the **workspace** root — not the corpus root, per
[0030](0030-a-link-manifest-is-one-file-per-subject.md), because a scope is about
the world.

### kept defines what a glob means

`*` matches within one path segment. `**` matches any number of segments,
including none. The match is against the workspace-relative slash path. That is
about twenty-five lines, and writing it is right under
[0029](0029-dependencies-pass-a-test.md)'s second criterion — it is small and we
would not get it wrong. Leaving it undefined, which is the state today, is what
is wrong.

### The definition is one sentence; everything else is how to compute it quickly

`World.Tree` is a content digest over the workspace root, honouring ignore
rules. Four accelerations compute *that number* faster, and **none of them
changes what the number means** — which is testable, and the test is that every
path yields an identical digest for one tree.

Measured on a 6.6 GB repository: 79,719 files, of which 27,299 are not ignored;
6,087 MB in total, of which **685 MB** is not ignored.

| | | vs naive |
|---|---|---|
| hash every file | 27.3 s | 1× |
| hash only non-ignored files | 2.53 s | 10.8× |
| …parallelised eight ways | 0.64 s | 43× |
| stat-only over non-ignored, steady state | **0.16 s** | 171× |
| git's index, using already-computed blob oids | 0.01 s | 2730× |
| this repository, full content hash | 0.03 s | — |

1. **Honour ignore rules.** Ten times faster, and a correctness requirement
   independently of speed: if `target/` counts, every build changes the world,
   every promise is due on every tick, and change-driven cadence is worth
   nothing.
2. **A stat cache under `.kept/cache/`**, mapping path → (size, mtime_ns, inode,
   mode) → content hash. Stat everything; re-read only what moved. It is derived
   state under [0018](0018-what-survives-a-crash.md)'s tier rule, so losing it
   costs exactly one slow tick and can never corrupt a record.
3. **Parallelise the cold path.**
4. **Use git's index as a pre-warmed cache when git is present.** An
   accelerator, which must agree with the plain path and is **never the
   definition**. This is what keeps the identity from splitting by environment.

Steady state is milliseconds on 6.6 GB, which is the bar for something invoked
the way `make` is.

### Three edges, and two of them fail silently

**The digest excludes `.kept/`, or the tick feeds itself.** In increment 3 the
ledger is plain files inside the workspace, so recording a truth transition
changes the world, which makes every promise due, which records more.
[0016](0016-the-ledger-lives-on-its-own-ref.md) removes the cause later by moving
the ledger off the working tree; until then the exclusion is a stated rule rather
than a consequence of layout. `.kept/` is already categorically out of scope for
every repair, for a different reason, and this is a second one.

**Stat can lie, and it lies in the silent direction.** Content changed, stat did
not, the promise is never re-observed, and nothing reports it. Three mitigations
and none of them is a proof: key on size **and** mtime_ns **and** inode **and**
mode; adopt git's racy-index rule and re-read anything whose mtime falls within
the cache's own snapshot granularity; `--all` forces. This is said out loud
because this project designs against exactly this failure shape, and here it is
being accepted deliberately — the alternative is reading 685 MB on every tick to
close a window measured in nanoseconds.

**Content hashing is the definition; stat is only the cache key.** Stat alone
cannot be `World.Tree`, and the reason is not performance. Two identical
checkouts on different machines have different mtimes and different inodes, so
their observations would never dedup — which breaks 0013's *the same tree
measured by the same gatherer is the same observation, whoever ran it*, and with
it the whole point of content-addressing an observation.

### Ignore semantics are defined here, not inherited

Following from the glob finding: kept says what "ignored" means rather than
borrowing a behaviour that turned out to be three behaviours.

- **`.gitignore` is honoured as a file format, not as a git invocation.** No git
  need be installed. A workspace that is not a repository and happens to carry a
  `.gitignore` gets the same answer as one that is.
- **A `.gitignore` applies to its own directory and below**, nearest-first, last
  matching pattern winning, with `!` re-including — the format's own rules. A
  `.gitignore` that appears mid-tree therefore takes effect for that subtree from
  the tick that sees it, and the resulting change of digest makes the affected
  promises due once. That is the safe direction and it is a real event, not an
  error: what the promise is about genuinely changed.
- **`.git/` and `.kept/` are excluded unconditionally**, whatever any ignore file
  says, because the first is not content and the second is the edge above.
- **Nothing else is excluded by default.** What more a workspace excludes is a
  list in `config.yaml`.
- **kept does not consult `.git/info/exclude` or a global core.excludesFile.**
  Both are per-checkout and untracked, so honouring them would make one tree
  produce different digests on two machines — the same identity split the
  accelerator rule exists to prevent.

### Matching nothing is not hashing nothing

A scope that matches no path must be **always due**, which 0025 already says is
the safe direction. A digest cannot deliver that on its own: the fold over an
empty set is a perfectly stable value, so a mis-typed scope would read as
*nothing has changed* forever — the same silent-permanence failure as the glob,
reached from the other side. Matching nothing is a distinct outcome and is
recorded as one.

### `World.Tree` has one definition, and that is the point

The world is not the scope, and 0014 is right that it cannot be: a gatherer
reads more than the paths a promise names — a test run reads `go.mod` — so a
scope digest is not a sound identity for a measurement.

So `World.Tree` is the digest above, and it is **the same digest in a
repository, outside one, and inside a box**. `Remote`, `Ref` and `Commit` are
provenance: filled in from git when git is present, empty when it is not, and
never part of the identity.

The alternative — a git tree oid when git is available — is the trap, and it is
worth naming because it looks like the cheap option. Two observations of one
identical tree would compare unequal across the two cases, splitting the cache,
the truth series and `prior()` with nothing reporting it. Worse, defining `Tree`
one way here and another when boxes arrive would orphan every observation
written before the change, and 0014 says outright that a record written without a
world cannot be backfilled.

## Alternatives

**`git rev-parse HEAD:<path>`, as 0025 wrote it.** Rejected on all three grounds
in the context. The glob failure is the disqualifying one, because it is silent.

**`git ls-files -s` over a pathspec, hashed.** Rejected: it reads the index, so
it cannot see an unstaged edit — the case 0025 required an answer for. It also
inherits git's non-uniform pathspec semantics, which is the thing this decision
is pinning down rather than adopting.

**`git write-tree` over a temporary index.** Rejected: 666 ms on a 3,847-file
repository, and it writes objects into the store to answer a read-only question.

**Hash file metadata rather than contents.** Rejected as the *definition*, and
adopted as the *cache key*, which is the distinction the third edge above draws.
As a definition it breaks cross-machine dedup; as a cache key it is 171× and
costs a nanosecond-wide race that the racy-index rule narrows.

**Do not honour ignore rules.** Rejected twice over: ten times slower, and
wrong, because a build would make every promise due.

**Let git decide what is ignored, by shelling out.** Rejected as a definition
and kept as an accelerator. As a definition it makes one digest mean different
things depending on whether git is installed, which is the identity split this
decision exists to prevent.

**Keep the scope digest out of the record and re-derive it from `World.Tree`.**
Rejected: it needs that tree to still be a retrievable git object, which it is
not in a dirty box and may not be on another machine.

## Consequences

- **A tick's cost is one workspace walk**, from which the world's digest and
  every scope's digest are folded, rather than one walk per scope. In steady
  state that is a stat pass over the non-ignored tree.
- **kept has no git dependency until boxes arrive**, which is increment 5, and
  the reason is now a property rather than an intention: nothing in due-ness or
  in world identity asks git anything. Where git is present it is an accelerator
  that must agree with the answer kept would have computed anyway.
- **The observation record carries a scope digest** as a field of its own,
  separate from the world. That is a one-way record shape and it is
  [0034](0034-the-observation-record.md)'s.
- **`.kept/cache/` gains a stat cache**, which is derived, gitignored, and
  disposable. A corrupt or deleted cache costs one slow tick.
- **The accelerations get a test that they agree.** One tree, five paths to the
  digest, one answer. That is the check that keeps an accelerator from becoming
  a second definition, and per `AGENTS.md` it is a check that must be able to
  fire: the test deliberately desynchronises the stat cache and asserts the
  content path wins.
- **`kept doctor` gains two checks**: a scope matching nothing, and a scope
  whose matched bytes are dominated by paths the ignore rules exclude — which is
  a mis-stated scope rather than something for the tool to correct silently.
