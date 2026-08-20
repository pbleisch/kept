---
id: 0048-the-operator-is-named-and-never-guessed
type: decision
title: 0048 — The operator is named, and never guessed
links:
  - { target: decision:0012-a-run-id-is-opaque, relation: extends }
  - { target: decision:0001-standing-and-truth-live-in-the-ledger, relation: depends-on }
  - { target: decision:0024-signing-is-designed-and-deferred, relation: depends-on }
---

# 0048 — The operator is named, and never guessed

## Context

`cmd/kept`'s `whoami` fills the `by` field on a ratification from, in order,
`KEPT_BY`, the operating system's username, and failing both the literal string
`"unknown"`. The flag's own help says *"defaults to the user running it"*.

Running `kept ratify` without `--by` in this repository produced this, beside
forty-four records written by the same person for the same kind of act:

```
44  'Paul Bleisch <pbleisch@acm.org>'
 1  'paul'
```

Two identities for one operator, in the stream that records who granted
standing. It was caught because the count was small and the line had not been
committed. At forty-four to one it was obvious; at four hundred to forty it
would be a reconciliation.

**An operating system username is not an identity.** It is machine-local, it
collides — two people called `paul` on two machines are one string in a cloned
ledger — and it means nothing to a reader who was not on the machine. That is
[0012](0012-a-run-id-is-opaque.md)'s argument about a hostname, in the field
that names a person rather than a process:

> Everything encoded in an identifier becomes parseable … A hostname would then
> be in a tracked repository for as long as the record lives, unfixable and
> unremovable.

`"unknown"` is worse than either. It writes a permanent, tracked record naming
nobody, on the **one act the model reserves to a person**
([0001](0001-standing-and-truth-live-in-the-ledger.md)) — a ratification that
cannot say who ratified is a ratification with the property it exists to carry
removed.

And [0024](0024-signing-is-designed-and-deferred.md) pins the payload a
signature will one day be computed over, `by` among its fields. A guessed value
in a signed record is a signature over a claim nobody made.

## Decision

**`by` is named or the act is refused. Nothing infers it.**

In order:

1. `--by`, stated on the command line;
2. `KEPT_BY` in the environment;
3. `operator:` in the **user's** config — `os.UserConfigDir()/kept/config.yaml`;
4. **refuse**, naming the three ways to set it.

The operating-system fallback is removed and `"unknown"` is removed. Neither
`ratify` nor `release` writes a record that names a person the person did not
name.

### Identity is the operator's, so it is not in the repository's config

`.kept/config.yaml` is tracked. An `operator:` key in it would attribute the
next person who clones this repository and ratifies anything to **whoever
committed the config** — silently, and permanently, in an append-only stream.

That is this decision's own failure one level up. Reading identity from a
tracked file somebody else wrote *is* an inference: it infers that whoever
cloned the repository is whoever configured it. A repository-scoped default here
would close the door and leave the window open, so there is no repository-level
fallback at all — not even a last-resort one, because a last resort is exactly
where a misattribution would land unnoticed.

Git decided this the same way and had every opportunity not to: `user.email`
lives in `.git/config` or `~/.gitconfig`, never in the tracked tree, and a fresh
clone refuses to commit until it is set. That friction is the correct behaviour
rather than a wart, and it is the behaviour adopted here.

**This is the first operator-scoped setting, and it will not be the last.**
`.kept/config.yaml` currently mixes what is true of the repository — the corpus
root, retention, the digest's exclusions — with what is true of a machine or a
person: which confinement provider is installed, how long a gather may take
before this hardware is the reason, whether a hostname may appear in a record.
Those travel to other people's machines, where they are wrong; `confine.provider`
is the sharpest after identity, because 0017 refuses to dispatch on an
unavailable provider, so a tracked one makes kept refuse to work on a correctly
configured machine. Moving them is a separate decision with a separate argument.
Identity moves now because it is the one where being wrong is unrecoverable.

### Why a config file, and not git

The user's config is per machine and per person, which is what an identity is,
and it is correctable — which the ledger record it feeds is not. Stating it once
per machine is the whole cost, and it is the same once-per-machine git asks for.

Reading `user.email` from git would match what these records already hold, and
is rejected for two reasons. **kept shells out to git nowhere.** The only
`exec` in the tree is `internal/gather/program.go` running a program a promise
authored; not `kept init`, not the workspace digest — 0033 chose to hash the
tree itself rather than ask git. Adding a git subprocess for a default would
make the binary depend on git being installed to record an act that has nothing
to do with git. And resolving git's own precedence — system, global, local,
worktree, `includeIf` — is exactly what [0029](0029-dependencies-pass-a-test.md)'s
second criterion says not to hand-write.

### Why refusing is not friction

Standing is created only by a person. A default that supplies an identity is
the tool asserting one on their behalf, on the only act it may not perform
itself. Refusing once, with a message that says how to fix it, costs one line
of config in the life of a repository.

## Alternatives

**Keep the username default and normalise later.** Rejected: the ledger is
append-only and tracked, so "later" means a release-and-re-ratify pair for every
affected record, or living with two names for one person forever.

**Shell out to `git config --get user.email`.** Rejected above. Worth noting it
would also be the first thing that made `kept ratify` fail on a machine with no
git, for a field git does not own.

**Refuse always, with no config key at all.** Rejected as the friction the user
config removes: every ratification would carry `--by`, and a repeated flag is a
flag people alias, which reintroduces the guess somewhere kept cannot see it.

**Keep `operator:` in `.kept/config.yaml` as a last resort.** Rejected, and it
was the first shape this decision took. A single operator makes it correct and a
second one makes it silently wrong, in the stream where wrong is permanent —
which is the property that made the username fallback unacceptable, so it cannot
be acceptable here.

## Consequences

- **`kept ratify` and `kept release` can now fail for a new reason**, and the
  message names all three routes. It is a usage error, not a refusal to act on
  drift — exit 2.
- **A guard that fires**: an act attempted with no `--by`, no `KEPT_BY` and no
  `operator:` is refused and nothing is appended. The half that matters is that
  **nothing is written** — a refused act that had already opened the stream
  would leave the run record it was refused during.
- **`kept init` does not write the key**, because it is not the repository's to
  write. It reports whether one is set, which is the useful half.
- **Existing records are untouched.** The single `paul` line was reverted before
  it was committed, so every act in this ledger names the same operator and
  nothing needs reconciling.
- **`KEPT_BY` keeps its place above the file** so that a second person on one
  machine can act without editing another person's config, which is the shape
  open question 10 will meet when it arrives.
- **A user config is a new file kept reads**, and it is read only for this. It
  is not a second place for repository settings to live, and a key in it that
  the repository owns is ignored rather than merged — one setting, one home.
