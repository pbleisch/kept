---
id: 0049-three-gatherers-parse-rather-than-scan
type: decision
title: 0049 — Three gatherers, and the two that parse read a tree that does not compile
links:
  - { target: decision:0043-the-catalogue-is-closed-and-a-third-party-is-namespaced, relation: implements }
  - { target: decision:0004-gatherers-return-observations, relation: implements }
  - { target: decision:0023-what-may-be-captured, relation: extends }
  - { target: decision:0034-the-observation-record, relation: depends-on }
  - { target: decision:0046-a-reading-is-a-ledger-act-and-the-scope-digest-carries-it, relation: depends-on }
---

# 0049 — Three gatherers, and the two that parse read a tree that does not compile

## Context

The structural claims in `docs/self-hosting.md` are the first policies meant to
move into the corpus, and none of them is writable without an instrument. Taken
row by row against the tree as it stands, six of the seven need one of three
things — an import graph, a symbol scan, or kept reading a corpus — and the
seventh needs a packet assembly that does not exist yet.

Every one of them is writable *today* with `command`, and that is the problem
rather than the answer.

**`command` lets the promise author forget to check that the instrument
worked.** The boundary promise *the corpus cannot write standing* was written
with `go list -deps` and the boundary was then broken on purpose:

```yaml
run: "go list -deps -f '{{.ImportPath}}' ./internal/corpus"
fields: { deps: "lines(stdout)" }
hold_when: not ("…/internal/ledger" in obs.graph.deps)
```

It reported **`kept`**. The added import created a cycle, `go list` exited
non-zero, stdout was empty, and `"…ledger" in []` is false. Adding
`ok: "exit == 0"` fixed the verdict and not the instrument: the corrected
promise reports `not-kept` because the tree did not compile, which is not a
boundary violation and is not what the promise says.

A native gatherer cannot make either mistake. Its failure is a gather failure,
`gather.Record` files nothing for one, and the verdict is `unknown` — which is
the sentence *nobody could measure*, said by the machinery rather than
remembered by an author.

**And a byte scan is provably the wrong instrument for two of the rows.** The
promise *the core carries no domain vocabulary* was scanned with a regexp over
the files in scope:

```
$ grep -rnE "\b(tests|command)\b" internal/model internal/tick internal/ledger
… 17 hits, every one of them in a doc comment
```

True of the code and false of the bytes. *Gatherers do not write* is the same
shape: what it means is that nothing in `internal/gather` calls a filesystem
write, and what a pattern matches is any file that mentions one.

## Decision

**Three built-in gatherers ship: `imports`, `symbols` and `corpus`. The first
two parse Go with `go/parser` and are one parse with two queries. `text` is not
built.**

### What each declares

`Declares` binds — a promise's `gather` map is in `_schema/promise.yaml`'s hash
list — so these field names are effectively permanent and are written out here
rather than left to the code.

| gatherer | fields |
|---|---|
| `imports` | `packages` — `[{package, dir, imports, deps}]`; `edges` — `[{from, to}]`; `external` — out-of-module import paths; `ok` |
| `symbols` | `packages` — in-module package paths; `refs` — `[{package, path, name}]`; `defines` — `[{package, name, kind}]`; `ok` |
| `corpus` | `artifacts` — a count; `findings` — `[{artifact, kind, detail}]`; `broken_links` — `[{from, target, relation}]`; `ok` |

Every list is sorted. That is not tidiness: an observation is addressed by its
data, so an order that came from map iteration or from which directory was
walked first would mint a fresh record on every tick of an unchanging world and
the dedup the whole content-addressed layout exists for would quietly stop
working (0034).

Both parsing gatherers take a `tests` argument, default false.

### `imports` parses, and does not shell out to `go list`

Stdlib, no toolchain to install, and the real reason: **it answers on a tree
that does not compile**, which is exactly the tree a repair agent is standing
in. `go list` answers `unknown` there — or worse, answers `kept`, as above. The
package path is `go.mod`'s module line plus the directory, which is the one
piece of resolution the toolchain was being asked for.

The alternative gets build tags right and gives the toolchain's own answer, and
it would be the right choice if these promises were about vendored or generated
code. They are about boundaries between packages in one module.

### `symbols` is the same parse with a different query, and it replaces `text`

The AST is already in hand. Asking it *which packages reference `os.WriteFile`*
costs a second walk of a tree already parsed, and it answers about identifiers
rather than about bytes — so a doc comment saying `"tests@2"` is not a hit and a
package named in a string literal is not an import.

**Two promises move from *pattern — assessor must pass* to *structural —
auto-land*** in `model.md`'s autonomy table as a result: *gatherers do not
write* and *the core carries no domain vocabulary*. That is worth more than the
machinery costs, because assessor-gating means neither is trustworthy until the
assessor exists, and the assessor is increment 7.

A third row, *standing is minted in one place*, is served by the same instrument
and is **not** counted among the two, because it is false as written — `cmd/kept`
constructs a ratification, and it must, since that is where a person's act
enters. It has to be restated as *no package under `internal/` outside
`internal/ledger`* before it is a promise at all, and restating it is not this
decision's to do.

`text` stays in the catalogue's design as the thing it is good at — prose, in a
manuscript — and 0023's note about `text` capturing matched lines into a
permanent record stops applying to a source scan over kept's own tree.

### `corpus` reads a corpus root, and never the ledger

It is pointed at a corpus root, so it can answer about a corpus that is not this
one. It counts artifacts, reports what the load found wrong, and reports links
that resolve to nothing.

**It does not read the ledger, and does not declare a read of it under 0046.**
Counting ratifications would move an observation's data while `World.Tree` stood
still — `.kept/` is excluded from the world (0033) — and 0034 forbids that
because it breaks dedup silently. 0046 makes that permitted *on declaration*
rather than forbidden, and the declaration is what would make the address name
what the data depends on. This gatherer declares nothing, so the refusal stands
for it, on the original grounds. A later `corpus` question that genuinely wants
standing declares `ratified` and pays for it in due-ness; that is a different
gatherer's problem, or this one's second version.

### Three costs, accepted, and stated because `Declares` binds

**Build-tagged files are all counted, whatever platform they are for.** Nothing
here evaluates `//go:build`, so `program_unix.go` and `program_other.go` both
contribute edges. This **over**-approximates the edge set, and for a boundary
promise that is the fail-closed direction: a boundary that is not crossed on any
platform is not crossed.

**A selector is read as a package reference whenever its left side names an
import in that file, with no scope analysis.** A local variable shadowing an
import name mints a reference nobody wrote. Measured over this tree it is six
references in seven hundred, and it is the same fail-closed direction: the
failure it produces is a promise reporting `not-kept` for a reference that is
not there, which is loud, and the failure it refuses is a promise reporting
`kept` for a boundary that was crossed, which is silent.

**`tests` defaults to false, so `_test.go` files are not parsed.** This is not
the fail-open direction dressed up: a boundary promise is about the package that
ships, and a test helper writing a temporary file is not `internal/gather`
writing to the corpus. Over this tree the difference is stark — ten packages
call an `os` writer with tests counted and three without — and three is the
answer the claim is about. A promise that means *including its tests* writes
`tests: true`, and the argument binds.

## Alternatives

**`text`, as the increment-4 report recommended.** Rejected on the report's own
evidence: seventeen hits, every one a doc comment. A pattern gatherer answers a
question about bytes, and every row that wanted one is a question about code.
It also costs an assessor gate on two promises that could auto-land, and it is
the gatherer 0023's redaction note applies most sharply to.

**`go list -deps -json` for the graph.** Rejected above: it cannot answer on a
tree that does not compile, which is the tree the repair agent works in and
therefore the tree the gate re-derives from. Its advantages — correct build
tags, the toolchain's own resolution — are advantages about the wrong question.

**One gatherer with a `query:` argument** rather than `imports` and `symbols`
as two. Rejected: the fields would have to differ by argument, which is exactly
the shape 0043 permits for `command` and refuses for a gatherer whose output
shape kept decides. Two names, two field sets, both in code.

**`corpus` reading the ledger and declaring `ratified` under 0046.** Rejected
for now, not on principle — 0046 makes it legal — but because no promise wants
it, and a declaration is a permanent widening of what a promise's due-ness
depends on. 0036's bound applies: built when something needs it.

**Type-checked resolution with `go/types`** instead of matching selectors
against the file's imports. Rejected: it needs the tree to compile, which is the
one property these two gatherers exist to not require, and it would trade six
false references for every answer being `unknown` mid-repair.

## Consequences

- **The catalogue is five**: `command`, `tests`, `imports`, `symbols`, `corpus`.
  A bare name still resolves inside it and nothing on `PATH` can shadow one
  (0043).
- **No schema changes and no re-ratification.** Nothing in this decision touches
  a `hash` list, so the count of artifacts it de-ratifies is **zero**.
- **The declared field names are permanent.** `gather` binds, so a promise
  reading `obs.imports.deps` cannot be moved to a renamed field without a person
  granting standing again. That is the reason they are in this document.
- **A promise about a boundary is auto-landable**, because all three are
  structural. The weakest gatherer sets the gate, so a promise naming one of
  these and `command` is still gated by `command`.
- **The costs above are properties of the measurement, not bugs.** A promise
  reporting `not-kept` for a build-tagged file or a shadowed import name is the
  instrument working as decided, and the fix is the promise's scope rather than
  the gatherer.
- **`text` is unbuilt and stays unbuilt** until a manuscript promise wants prose
  matched. The catalogue is closed, so adding it later is an ordinary addition
  and not a reopening.

- **`docs/increments.md`'s done-when for increment 4 is sharper than it reads,
  and this is the sharpening.** It asks that *a tree that does not compile makes
  the same promise go `unknown` rather than `not-kept`*, which is right about
  `command` and only half right here. *Does not compile* covers two things a
  parser tells apart:

  - a tree with a **type error, a missing symbol or an import cycle** parses
    perfectly, and `imports` answers over it correctly. That is the whole reason
    for parsing, and in the cycle the report was written about the honest answer
    is `not-kept` — the boundary genuinely was crossed — rather than the
    `unknown` a toolchain would have to give;
  - a tree with a **syntax error** yields a partial AST, and reading the imports
    out of one under-approximates the edge set silently, so it is `unknown` and
    the promise says nobody could measure.

  The done-when's second half is the one worth building for, and what it is
  actually asking is that **a tree kept cannot read is `unknown`, never
  `not-kept`**. That holds. What does not hold is the literal reading, and the
  literal reading would be a worse instrument: it would throw away a correct
  answer whenever the toolchain would have refused one.
