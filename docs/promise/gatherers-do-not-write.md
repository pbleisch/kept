---
id: gatherers-do-not-write
type: promise
title: Gatherers do not write
fields:
  statement: >-
    internal/gather names no creating, removing, renaming or writing call in os or io. It
    reads the world and changes nothing in it.
  gather:
    names:
      gatherer: go/symbols
      scope: ["internal/gather/**/*.go"]
  hold_when: >
    obs.names.ok
    and count(obs.names.packages, p => p == "github.com/pbleisch/kept/internal/gather") == 1
    and count(obs.names.refs, r => (r.path == "os" or r.path == "io")
              and count(find_all(r.name,
                  "^(Create|CreateTemp|Mkdir|MkdirAll|MkdirTemp|OpenFile|Remove|RemoveAll|Rename|Symlink|Link|Truncate|Chmod|Chown|WriteFile|WriteString|Copy|CopyN|NewFile)$"),
                  m => true) > 0) == 0
  trigger: cadence
  max_age: 7d
  repair:
    scope: ["internal/gather/**/*.go"]
links:
  - { target: policy:a-gatherer-never-repairs, relation: serves }
  - { target: decision:0004-gatherers-return-observations, relation: cites }
---

`policy:a-gatherer-never-repairs` says why: a gatherer that fixes what it finds
cannot be re-run to check the fix, and its output has stopped being evidence.
This is what holds the package to it. Today `internal/gather` names exactly one
call into the filesystem, `os.ReadFile`, and runs programs through `os/exec` —
which is the escape hatch a promise authored, not a write the gatherer chose.

The list is the write verbs of `os` and `io`, matched against the identifier a
selector names rather than against the bytes of the file. That distinction is
the whole reason `symbols` exists: a byte scan for the same thing finds doc
comments, and [0049](docs/decision/0049-three-gatherers-parse-rather-than-scan.md)
records the measurement — seventeen hits on the sibling promise, every one of
them prose.

**What it does not see.** A write reached some other way — a third-party writer,
a file handle passed in, a method on a value — is not a package-qualified
reference and is not in this list. The bound is deliberate: this promise catches
the ordinary way a package starts writing, and widening the list to guess at the
rest would make it fail for references that are not writes. The scope excludes
`_test.go` by default, which is [0049](docs/decision/0049-three-gatherers-parse-rather-than-scan.md)'s
choice and the right one here — a test helper writing a temporary file is not
`internal/gather` writing to the corpus.

The `packages` clause is the guard against measuring nothing: a scope that
matched no Go file would satisfy the count that follows it, and report a
package that writes nothing because no package was read.
