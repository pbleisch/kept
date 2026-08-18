# 0029 — Dependencies pass a standing test, and only the trust path gets an ADR

**Status** accepted · replaces the "a dependency is a decision and gets an ADR"
line in `AGENTS.md` · settles what the corpus parser has to do

## Context

`AGENTS.md` says: *no external dependencies yet. Keep it that way as long as it
is honest to; a dependency is a decision and gets an ADR.* At zero dependencies
that reads well. At the first real one it produces one of two bad outcomes — an
ADR per package, which is noise nobody will read by the tenth, or a quiet
`go get`, which is exactly the unexamined addition the rule was written to
prevent.

The intent was thoughtfulness, not paperwork. Those are separable.

The question is not hypothetical: increment 1 cannot be built without a YAML
parser, because 0002 fixes frontmatter as YAML and Go has none in its standard
library.

## Decision

**A dependency is admitted by passing a standing test and is recorded in one
line. Only dependencies in the trust path get an ADR of their own.**

### The test

1. **Does the standard library already do this, adequately?** If yes, refused.
   "More ergonomic" is not adequacy.
2. **Would we get it wrong by hand?** Parsers, crypto, compression, time zones,
   Unicode. Writing a YAML parser is a way to lose a month and still ship bugs.
   If the honest answer is *we could write this correctly in an afternoon*, we
   write it.
3. **What is the blast radius if it is abandoned or compromised?** A dependency
   that sits behind an interface we own is a swap. One whose types appear in our
   own signatures is a rewrite.
4. **Is it in the trust path?** Does it see secrets, decide what is authorized,
   confine a process, or write the ledger?

### Three outcomes

| | | |
|---|---|---|
| **Admitted** | passes 1–3, not in the trust path | one line in `docs/dependencies.md`: what it does, why not the stdlib, which package owns the seam, what would replace it |
| **Needs an ADR** | in the trust path, or it changes the model | [0017](0017-the-box-is-not-the-jail.md)'s `nono` is the worked case — it decides what an agent may touch, so the reasoning belongs in a decision |
| **Refused** | convenience | assertion libraries, CLI frameworks, logging frameworks, anything whose job is typing less |

`docs/dependencies.md` is the record, not `go.mod`. A module in `go.mod` with no
line in that file is a finding, and eventually a promise: a gatherer over the
module graph and a predicate that every non-stdlib module is accounted for.
That is the self-hosting shape — the rule about dependencies becomes a
dependency kept checks.

### Worked: the corpus parser

1. Standard library: no YAML. 2. By hand: no — YAML is a tarpit and frontmatter
is the front door to every artifact. 3. Blast radius: bounded, because of what
kept actually writes (below), and the seam is `internal/corpus`. 4. Trust path:
it parses content that feeds a content hash, so a parser change can change
hashes — which is precisely what 0022's `norm` version exists to absorb. It
decides no authority.

**Admitted, recorded, seam in `internal/corpus`.**

## What kept writes, which is less than assumed

Increment 1's done-condition asks for *unknown fields intact through a
parse-write round trip*. That requirement was overdrawn, and it was driving the
hardest version of the dependency.

**kept never rewrites a human-authored artifact.** Walking the write surface:

| | writes | to |
|---|---|---|
| `ratify` · `release` | a decision record | the ledger, never the file ([0001](0001-standing-and-truth-live-in-the-ledger.md)) |
| a truth transition | a record | the ledger |
| `promote` | a **copied** fact | a new file |
| retraction | an append-only tombstone | the ledger |
| `new` | a whole artifact kept just composed | a new file |
| the repair agent | the artifact | with its own tools, in a box. kept does not mediate the edit |
| `doctor`, adopting a corpus | one missing `id` | a **line-level insert** into the frontmatter block |

Only the last is an edit to a file a person wrote, it adds exactly one key, and
it is done by inserting a line rather than by parsing and re-emitting the
document.

So the requirement is not round-tripping. It is: **unknown fields survive a
read** — available to the caller, never an error, per 0002's leniency — and are
never written back, because kept does not write that file. Files kept *creates*
must be well-formed, and it composed them, so it knows every key.

## Alternatives

**Keep "an ADR per dependency."** Rejected: by the tenth nobody reads them, and
the ceremony would push a future maintainer toward vendoring or hand-rolling to
avoid it, which is worse on every axis.

**No rule; use judgement.** Rejected: judgement is what the test writes down. An
unwritten standard drifts toward whatever the last person found convenient.

**A dependency budget — "at most N".** Rejected: it counts the wrong thing. One
badly chosen module in the trust path is worse than ten format parsers.

## Consequences

- **`AGENTS.md` changes**, from *a dependency gets an ADR* to *a dependency
  passes the test and is recorded; the trust path gets an ADR*.
- **`docs/dependencies.md` is a new file** and is the first thing a reviewer
  checks against `go.mod`.
- **Increment 1's round-trip condition is retired** and replaced with the
  weaker, true one: unknown fields survive a read and a created file is
  well-formed.
- **The seam is a real requirement, not a note.** If a corpus parser's types
  reach beyond `internal/corpus`, criterion 3 has been violated after the fact,
  and that is a review finding.
