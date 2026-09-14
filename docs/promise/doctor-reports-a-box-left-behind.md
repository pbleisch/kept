---
id: doctor-reports-a-box-left-behind
type: promise
title: Doctor reports a box left behind
fields:
  statement: >-
    kept doctor reports every box git still has registered as a worktree of this repository,
    and names the command that removes it without removing anything itself — shown by two
    named tests in a cmd/kept suite that passes whole.
  gather:
    tests:
      gatherer: go/tests
      scope: ["cmd/**", "internal/**", "go.mod", "go.sum"]
      packages: "./cmd/kept"
  hold_when: >
    obs.tests.ok
    and count(obs.tests.packages, p => p.ok) > 0
    and "TestDoctorReportsABoxLeftRegisteredAsAWorktree" in obs.tests.cases
    and "TestTheLeftBoxFindingFiresOnNothingHere" in obs.tests.cases
  trigger: cadence
  max_age: 7d
  repair:
    scope: ["cmd/kept/**"]
links:
  - { target: decision:0018-what-survives-a-crash, relation: cites }
  - { target: decision:0050-doctor-reports-an-unchecked-instrument, relation: cites }
---

[[decision:0018-what-survives-a-crash]] says what a dead process leaves and that
nothing self-heals silently: *worktrees whose thread is finished or dead* are
reported, and then acted on. The report was never built. A land killed between
its gate and its record leaves its gate box registered as a git worktree, and
`internal/land/crash_test.go` measured that nothing prunes or reports it. A repair
killed mid-session leaves its box the same way, and `--keep` on either verb
leaves one on purpose. Each is a full checkout under `.kept/boxes/`, git counts it
in `git worktree list`, and nothing kept prints ever mentions it again.

This is a `kept doctor` finding because it passes the three tests
[[decision:0050-doctor-reports-an-unchecked-instrument]] sets for one: nothing
rejects it, nothing else reads it, and kept does nothing about it.

**What it reports.** Every worktree git has registered for this repository whose
path is a box — a directory directly under a `.kept/boxes/` directory — one per
line, with its path, and the one command that discards it:
`git worktree remove --force <path>`. Git's own index is the authority on what is
registered, not a listing of `.kept/boxes/`: a directory git no longer knows is
not a worktree, and a registration whose directory is gone is one `git worktree
prune` clears, which the finding may name instead. At most one line of why: a
repair or a land running right now, or one run with `--keep`, has a box too, so
this is a report and never a removal.

**What it must not do.** It removes nothing, writes nothing, and moves no exit
code — [[decision:0027-the-machine-facing-surface]]'s rows are about commitments
and evidence, and a box is neither. It says nothing outside a git repository, the
way the fetch findings do. Every git call it makes has a deadline.

**The two tests are the promise.** `TestDoctorReportsABoxLeftRegisteredAsAWorktree`
registers a real worktree under `.kept/boxes/` in a real repository and asserts
the finding names its path and the command. `TestTheLeftBoxFindingFiresOnNothingHere`
asserts that a repository with no box — including one with an ordinary linked
worktree somewhere else — reports nothing, because a check that has never fired is
not a check and one that fires on everything is not one either. A test with the
right name that registers nothing and asserts nothing reads `kept` here, so the
diff is what a person reviews; the names are only what makes it measurable.

**Why the whole suite, and not only the two names.** No other promise runs
`go test ./cmd/kept` — `core-packages-are-tested` stops at `internal/` — so
`obs.tests.ok` is what keeps a repair of this promise from breaking a neighbouring
test in the package it is bounded to and still reading `kept`.

**The bound is `cmd/kept/**` on purpose.** The finding belongs to the verb, and a
change under `internal/` would make a land's gate re-derive the promises that run
this repository's own `internal/` suites on both sides of the merge. Reading
`git worktree list --porcelain` is a few lines; it does not need a new package.
