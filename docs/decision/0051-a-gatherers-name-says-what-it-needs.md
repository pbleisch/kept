---
id: 0051-a-gatherers-name-says-what-it-needs
type: decision
title: 0051 — A gatherer's name says what it needs, and a group is one segment
links:
  - { target: decision:0043-the-catalogue-is-closed-and-a-third-party-is-namespaced, relation: amends }
  - { target: decision:0049-three-gatherers-parse-rather-than-scan, relation: amends }
  - { target: decision:0012-a-run-id-is-opaque, relation: depends-on }
  - { target: decision:0003-schema-declares-what-binds, relation: depends-on }
---

# 0051 — A gatherer's name says what it needs, and a group is one segment

## Context

[0043](0043-the-catalogue-is-closed-and-a-third-party-is-namespaced.md) closed
the built-in catalogue with two namespaces: a bare name is kept's, and `x/` is a
third party's. It wrote down the risk in its own words —

> ### The catalogue is being closed against one language, and that is the risk

— and answered it on the **domain** axis. The manuscript spike showed `entities`
dissolving, because a novel's vocabulary lives in `_schema/` and not in the
core. That result stands.

It does not transfer to the **language** axis, and three gatherers later the gap
is visible in the catalogue itself. Of five built-ins, three work only on Go:

| | reach |
|---|---|
| `tests` | parses `go test -json`'s event stream |
| `imports` | `go/parser`, and `go.mod` for the module path |
| `symbols` | the same Go AST, a second query |
| `corpus` | any corpus, in any language |
| `command` | anything, and the program is authored in the promise |

Each of the three carries a **universal name for a Go-only instrument**. A
promise writes `gatherer: tests`, `gather` is in `_schema/promise.yaml`'s hash
list, and the name it binds says nothing about what the tree must have. On a
Rust repository that promise is not wrong — it is unanswerable, and its text
gives a reviewer no way to know.

kept is meant to be neutral across languages and stacks. Neutral does not mean
having no toolchain-specific instruments; it means **an instrument says which
toolchain it needs, in the name a promise binds.**

## Decision

**A name has three forms, and the group segment answers *what must this tree
have*.**

| form | means | who decides the shape after the prefix |
|---|---|---|
| `<name>` | built-in, works on any tree, needs nothing installed | kept |
| `<group>/<name>` | built-in, and it needs whatever `<group>` names | kept, **one segment** |
| `x/…` | not kept's | **the third party** |

So `tests` becomes `go/tests`, `imports` becomes `go/imports`, `symbols` becomes
`go/symbols`. `command` and `corpus` keep bare names, because they need nothing
a tree might not have.

### A group is a capability, not a filing system

A group gathers built-ins that need the same thing present. `go` is one because
those three need the Go toolchain; `art` would be one because those need
whatever reads an image; `prose` might be. The question it answers is *do I have
what this needs*, and that is the same question whether the answer is a compiler
or a font library.

**The test for whether a group has earned its name: does anything resolve, gate
or refuse differently because of it?** For `go/` yes — it is the capability
check. For a `coding/` above it, no. Nothing would ever branch on it.

### One segment, because every segment is unrevisable

`coding/go/tests` is the tempting shape and it is refused, on
[0012](0012-a-run-id-is-opaque.md)'s rule:

> Everything encoded in an identifier becomes parseable, and everything
> parseable acquires readers.

A gatherer name binds, so **each segment is a taxonomy decision that cannot be
revised without de-ratifying everything beneath it.** One segment encodes one
claim: *this needs Go*. Two encode a second — *and Go is a kind of coding* —
which is a filing decision, and filing decisions look obvious now and arbitrary
later. Is `sql` under `coding`? Is `docker`? Is an epub builder `prose` or
`coding`? A grouping that exists only to read well belongs in documentation,
where being wrong is free.

If a deeper group ever earns its keep — something genuinely resolves or gates on
it — that is a new decision with a demonstrated trigger, and it costs a bulk
re-ratification, which is the honest price of changing a taxonomy after promises
have been written against it.

### `x/` is a marker, and kept does not legislate what follows it

`x/` does not mean *the third-party group*. It means **not kept's, and kept makes
no claims about the rest of the string**. `x/spellcheck` and
`x/acme/coding/go/whatever` are both legal, and the whole of what follows `x/` is
one opaque name that resolves deterministically to a binary.

The one-segment discipline binds where kept would have to live with being wrong.
Imposing it on other people's namespaces would be legislating a taxonomy for
trees kept has never seen, which is the thing this decision refuses to do at one
level and would be worse at two.

### Built-in is about who wrote it, not what it measures

0043's R7 — a third-party gatherer can never set a structural, auto-land gate —
reads under this decision as being about **authorship**, which is what it always
argued: *kept did not write it and cannot vouch for what it measured*. kept wrote
`go/tests`, `go/imports` and `go/symbols`. They stay built-in, stay structural,
and stop claiming to be universal. A grouped name is not a demotion.

### `tests` stays reserved, on purpose

The bare name is not taken by the Go implementation. A neutral `tests` is
reachable later over a common report format — JUnit XML is emitted by pytest,
jest, cargo-nextest and go-junit-report alike — with `go/tests` remaining the
native path that reads the toolchain's own stream. Spending the bare name now on
one language would spend it on the least neutral candidate.

## Alternatives

**Leave the names bare and dispatch on the project type.** A `tests` that reads
`go.mod` or `Cargo.toml` and picks. Rejected twice over: it is the per-language
accretion 0036 was written to stop, and it makes an observation's meaning depend
on the tree rather than on the promise — a `World` arriving through a door 0033
and 0038 both closed.

**Keep bare names and document the reach.** Rejected: the name is what binds and
what a reviewer reads in a diff. A promise whose text cannot be checked against
the repository it is about is the problem, and documentation elsewhere does not
reach the diff.

**Make the Go gatherers the first `x/` plugins.** Rejected: it would demote
kept's own self-hosting promises to assessor-gated under 0043's R7, which blocks
them until an assessor can pass them — and the demotion would be for a property (what language
they parse) that R7's argument is not about.

**Arbitrary depth now, in case.** Rejected above. It is cheap to add a level
later and impossible to remove one.

## Consequences

- **Five promises are de-ratified and granted again.** `core-packages-are-tested`,
  `gatherers-do-not-write`, `standing-is-minted-in-one-place`,
  `the-core-carries-no-domain-vocabulary` and
  `the-corpus-cannot-write-standing-or-truth` name a renamed gatherer, so
  `gather` moves and standing does not carry. This change affects five promises;
  any later promise that names a renamed gatherer is subject to the same rule.
- **A name with a `/` is not a path**, and nothing may read the group as a
  directory or a package. It is one segment of one identifier.
- **A grouped name cannot be a gather map key**, and this narrows
  [0005](0005-gatherers-are-a-named-map.md). That key is the name the predicate
  reads under — `obs.<key>` — so it is an identifier, and an identifier has no
  slash in it. 0005's *the gatherer defaults to the map key* therefore holds for
  bare names only; a grouped one is named explicitly:

  ```yaml
  gather:
    tests:                    # what the predicate reads: obs.tests
      gatherer: go/tests      # what runs, and what it needs
  ```

  That reads better than the shorthand it loses. The key says what this promise
  calls its evidence and the gatherer says where the evidence comes from, and
  the two were only ever the same word by coincidence.
- **A promise about a Rust repository is now writable as wrong on purpose.**
  `gatherer: rust/imports` refuses at load — nothing answers to it — which is an
  authoring error a person fixes, rather than an `unknown` in the code path that
  decides a verdict.
- **`x/` is still unimplemented**, as 0043's R8 left it, and this decision does
  not build it. What it does is make the built-in side honest first, so that
  when the wire arrives the third-party rule is the only one left to add.
- **The neutral half of the catalogue is now nameable.** `text`, `path`,
  `entities`, `fold`, `units`, `reading` and `judgement` all take bare names when
  they are built, and `model.md`'s table can say which of its kinds are neutral
  and which will carry a group.
