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
3. `operator:` in `.kept/config.yaml`;
4. **refuse**, naming the three ways to set it.

The operating-system fallback is removed and `"unknown"` is removed. Neither
`ratify` nor `release` writes a record that names a person the person did not
name.

### Why config, and not git

`.kept/config.yaml` is where an operator already states what kept should do, it
is tracked so the identity travels with the repository, and it is correctable —
which the ledger record it feeds is not. Stating it once is the whole cost.

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

**Refuse always, with no config key.** Rejected as the friction the config key
removes: every ratification would carry `--by`, and a repeated flag is a flag
people alias, which reintroduces the guess somewhere kept cannot see it.

## Consequences

- **`kept ratify` and `kept release` can now fail for a new reason**, and the
  message names all three routes. It is a usage error, not a refusal to act on
  drift — exit 2.
- **A guard that fires**: an act attempted with no `--by`, no `KEPT_BY` and no
  `operator:` is refused and nothing is appended. The half that matters is that
  **nothing is written** — a refused act that had already opened the stream
  would leave the run record it was refused during.
- **`kept init` writes the key**, commented, so a repository adopted by it is
  not one ratification away from this error.
- **Existing records are untouched.** The single `paul` line was reverted before
  it was committed, so every act in this ledger names the same operator and
  nothing needs reconciling.
- **`KEPT_BY` keeps its place above config** so that a second person on one
  machine can act without editing a tracked file, which is the shape open
  question 10 will meet when it arrives.
