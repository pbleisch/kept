---
id: dependencies-are-all-recorded
type: promise
title: Every dependency is recorded
fields:
  statement: >-
    Every module go.mod requires is named in docs/dependencies.md. A dependency
    admitted without a written reason is one nobody decided to take.
  gather:
    mods:
      gatherer: command
      run: go mod edit -json
      scope: ["go.mod"]
      fields:
        ok: exit == 0
        paths: >-
          map(json(stdout).Require, r => r.Path)
    recorded:
      gatherer: command
      run: cat docs/dependencies.md
      scope: ["docs/dependencies.md"]
      fields:
        ok: exit == 0
        names: >-
          map(find_all(stdout, '`(?P<mod>[a-z0-9./-]+)`'), m => m.mod)
  hold_when: >
    obs.mods.ok and obs.recorded.ok
    and count(obs.mods.paths, m => true) > 0
    and all(obs.mods.paths, m => m in obs.recorded.names)
  trigger: cadence
  max_age: 30d
  repair:
    scope: ["docs/dependencies.md"]
links:
  - { target: decision:0029-dependencies-pass-a-test, relation: cites }
---

`AGENTS.md`, `docs/dependencies.md` and [[decision:0029-dependencies-pass-a-test]]
all already say the same sentence — *a module in `go.mod` with no line in that
file is a finding* — and until now nothing found one. Three prose statements of
a rule are three places it can be forgotten from at once; this is the one place
it is checked.

The instrument is deliberately blunt on one side. `go mod edit -json` is the
module list as the toolchain sees it, so the left-hand set cannot drift from
what is actually required. The right-hand set is every backticked name in the
document, which is far wider than the table — it catches `internal/corpus`,
`nono` and `go.mod` itself. That looseness is the correct direction: a module
mentioned anywhere in the file has been written about by somebody, and the
failure this exists to catch is a module nobody wrote about at all. A tighter
pattern over the table's first column would go stale the first time the table
gains a column.

**The guards are not a formality.** A `command` entry reports a non-zero exit as
data rather than as a failure, and `all()` over an empty list is true — so a
`go mod edit -json` that failed, or one that printed nothing a reader recognized,
would report this promise kept while checking no modules whatever. `obs.mods.ok`,
`obs.recorded.ok` and `count(obs.mods.paths, m => true) > 0` are what stop that,
and they are the same guards `the-readme-names-every-verb` and
`every-citation-in-the-corpus-resolves` each carry for their own reason.
`kept doctor` reports a `command` entry whose fields never read `exit`, and it
reported both of this promise's before they had one.

**What it does not check, and this is worth stating.** It holds the *table* to
`go.mod`. It does not hold the document's prose to the world — the closing
paragraph of `dependencies.md` claimed for a week that a repair had never been
dispatched into a box and that `confine.provider` was unset, while the config
set it and a repair had landed, and this promise read `kept` throughout. Prose
that describes a configuration is a different measurement from a list that
mirrors a manifest, and conflating them is how a promise comes to be trusted for
something it never looked at.

It fired before it was ratified. Adding `github.com/example/unrecorded` to
`go.mod` took it to `not-kept` and removing it took it back, which is the whole
of what `kept shape --trial` is for.
