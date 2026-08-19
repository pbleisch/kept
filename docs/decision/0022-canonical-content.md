---
id: 0022-canonical-content
type: decision
title: 0022 — The hash normalizes content, and says which rules it used
---

# 0022 — The hash normalizes content, and says which rules it used

**Status** accepted · completes [0003](0003-schema-declares-what-binds.md),
which canonicalized structured fields and left `body` as raw text

## Context

0003 settled that the binding hash is over "the canonical serialization of the
named `(field, value)` pairs, not raw bytes, so reformatting and key order do
not matter." That covers the structured half. It does not cover `body` — a
reserved name in every schema's `hash` list, and the whole markdown block.

Body is raw text, so every environmental difference in how text is stored
changes the hash, and a changed hash drops standing. The repository has no
`text=auto eol=lf` in `.gitattributes`, so a checkout with `core.autocrlf=true`
re-hashes every artifact and **de-ratifies the entire corpus at once**. It
presents exactly like 0003's deliberate case — *changing a schema's `hash` list
is a bulk re-ratification* — with no cause, no explanation, and nothing to
point at.

Unicode normalization is not hypothetical either. `layout.md`'s manuscript
corpus has character and location names; macOS and Linux disagree about NFC and
NFD, and the same file round-tripped through both hashes differently.

## Decision

**Normalization happens inside the hash function, not only at rest.**

Before hashing, a text value is:

1. stripped of a leading BOM
2. normalized to Unicode NFC
3. converted CRLF and CR to LF
4. stripped of trailing whitespace on each line
5. terminated with exactly one final newline

A CRLF checkout then produces **the same hash**, whether or not
`.gitattributes` was present, honoured, or reachable. The file keeps its
literal bytes; only the hash sees the normalized form.

**A whitespace-only edit no longer drops standing**, which is correct on its
own terms: nothing about what was promised changed.

### Hygiene as well, not instead

`kept init` writes `* text=auto eol=lf` into `.gitattributes`, and `kept
doctor` checks it. This stops the bytes churning in diffs; the normalization
above is what stops standing evaporating. Both, because they do different jobs.

### The rules are versioned

Every ratification record carries the canonicalization version it was computed
under:

```
ratified(promise:core-packages-are-tested, sha256:ab12…, norm:1, paul@…, at)
```

The field is `norm`, not `canon`: `canon` is already an artifact kind, and a
record that used the word twice would be read wrongly by someone in a hurry.

Changing the rules later re-hashes old records **under the version they were
written with**, so a refinement is not a bulk de-ratification. This is the same
insurance as the ledger envelope version in
[0013](0013-the-gate-makes-concurrency-safe.md), and it is what keeps this
decision from being a one-way door.

## Alternatives

**`.gitattributes` alone.** Rejected: it covers files that pass through git
with the attribute honoured, and nothing else — not a file written by a tool,
not unicode form, not a BOM, not a clone made before the file existed.

**Hash the raw bytes and tell people to configure git.** Rejected: the failure
is silent, total, and blamed on kept. A guarantee that depends on every
checkout being configured correctly is not a guarantee.

**Normalize the files themselves on read-write.** Rejected: kept would rewrite
prose it was not asked to touch, and the corpus is human-authored.

## Consequences

- **Normalization is lossy for the hash and lossless for the file.** Two bodies
  differing only in trailing whitespace are the same promise. Two differing in a
  word are not.
- **The version must be recorded from the first ratification**, not added when
  the rules first change.
- **`kept doctor` gains a check** for artifacts whose stored bytes differ from
  their normalized form — harmless, but worth surfacing before someone
  diagnoses a hash by eye.
