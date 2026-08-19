---
id: the-tree-vets-clean
type: promise
title: The tree vets clean
fields:
  statement: go vet is clean over the whole module, which means it also builds
  gather:
    check:
      gatherer: command
      run: "go vet ./..."
      scope: ["**/*.go", "go.mod", "go.sum"]
      fields:
        ok: "exit == 0"
  hold_when: obs.check.ok
  trigger: cadence
  max_age: 7d
  repair:
    scope: ["**/*.go"]
links:
  - { target: docs/decision/0038-a-run-line-is-argv-and-there-is-no-shell.md, relation: cites }
---

The first promise `docs/self-hosting.md` schedules, and it is trivial on
purpose: it is the first time the loop runs end to end against a repository
nobody constructed for it.

`run:` is argv and there is no shell, so this is one program and not a chain —
`go vet` compiles what it inspects, so a tree that does not build does not vet
either, and one line covers both. `ok: exit == 0` is the extractor: a non-zero
exit is data rather than a failure to measure, which is why breaking the build
reads as `not-kept` and uninstalling the toolchain reads as `unknown`.

The scope is the Go source and the module files rather than the whole
workspace, so editing a decision record does not re-run vet. That is the point
of a scope: it is what makes this promise due.
