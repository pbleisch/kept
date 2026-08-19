---
id: 0031-the-binding-preimage
type: decision
title: 0031 — The binding hash is over a canonical preimage
---

# 0031 — The binding hash is over a canonical preimage

**Status** accepted · completes [0003](0003-schema-declares-what-binds.md) and
[0022](0022-canonical-content.md), which say *which fields* bind and *how text
is normalized* and neither of which says what bytes are hashed · implements
[0030](0030-a-link-manifest-is-one-file-per-subject.md)'s rule that a manifest
edge never binds, which is decided there and resolved here

## Context

0003 says the binding hash is over "the canonical serialization of the named
`(field, value)` pairs, not raw bytes". 0022 says how each text value is
normalized before it is hashed, and that the rules carry a version. Between
them, everything about the hash is settled except the one thing a hash is: a
byte sequence.

That gap has to close in increment 2 and it cannot be reopened. Every
ratification is a permanent record naming a digest, so the preimage that
produced it is a one-way choice in exactly the sense
[0013](0013-the-gate-makes-concurrency-safe.md) means — *a durable record's
shape is one-way; mechanism and policy are two-way*. Changing it later does not
mean recomputing some values; it means every ratification in every corpus
suddenly names a version of a file that never existed.

Three sub-questions arrive with it, and each has a wrong answer that looks
reasonable:

**What "canonical serialization" is.** The tempting answer is to reuse
something — sorted-key JSON, or re-emitted YAML. Both are formats designed to be
read back, which is not what this is for, and both bring their own decisions
about number formatting and escaping into the middle of an authority mechanism.

**Which of 0003's reserved names resolve to what.** `body` is easy.
`links.<relation>` needs a rule about *which sources* count, and that rule is
0030's rather than this one's — a link can come from three sources and 0030 made
one of them writable by a third party, which is a question about manifests and
not about hashing. What is left here is resolving the name once that is settled.
`body.<section>` is undecided outright — it is open question 5.

**Whether the `hash` list's order matters.** 0003 says changing the list is a
bulk re-ratification, and does not say whether reordering one counts as
changing it.

## Decision

### The preimage is a tagged, length-prefixed encoding, and nothing reads it back

```
domain    "kept/binding-hash\nnorm:1\n"
pairs     l<count>: for each name in the sorted hash list:
            s<len>:<normalized name>   then   '+' <encoded value>   or   '-'
values    n                       nil
          b1 · b0                 bool
          i<decimal>; u<decimal>; f<shortest-round-trip>;
          s<len>:<normalized text>
          l<count>:<item>…
          m<count>:(<key><value>)…  sorted by encoded key
```

Two properties are required and the rest is arbitrary:

**Deterministic.** Two machines reading one file produce identical bytes. That
rules out map iteration order and any float formatting that varies by platform.

**Unambiguous.** No two different values encode to the same bytes. This is what
the length prefixes are for, and the case that needs them is not contrived:
normalization ends every text with a newline, so without a length a multi-line
body whose lines happen to begin with `s` can be re-read as the key and value
that follow it. `{"a": "b\nsc\nsd"}` and `{"a\nsb": "c\nsd"}` are then the same
bytes — which is one promise silently becoming a different promise while keeping
the standing granted to the first. There is a test that produces exactly that
collision.

The domain prefix carries the `norm` version, so a record hashed under `norm:1`
and one hashed under `norm:2` cannot collide even where the content is identical.

**Nothing parses this.** It is not a serialization format, it is never written
to disk, and it never crosses a package boundary. It exists to be hashed, which
is why it can be this plain.

### The `hash` list is a set

Names are sorted before hashing. Two schemas naming the same fields in a
different order describe the same commitment, so reordering one does not
re-hash the corpus; adding or removing a name does, which is the bulk
re-ratification 0003 intends and accepts.

### An absent field is not an empty one

A field the author did not write and a field they wrote as empty are different
content, and the `+`/`-` tag keeps them apart. Without it, deleting a line and
blanking one would be indistinguishable, and an artifact could be changed into
another by an edit the hash could not see.

### `links.<relation>` resolves to what the author wrote, sorted

`links.<relation>` resolves to the targets of that relation from the sources the
artifact's own author wrote — its frontmatter and its prose — and never from a
`_links` manifest.

**The rule is 0030's, not this one's.** A manifest exists so that a third party
can assert a link about an artifact without editing it, and an edge that bound
would let such a writer change what a ratification was granted against; 0030
carries that argument, because it is about what a manifest may do rather than
about how bytes are hashed. It is named here only because the preimage is where
it takes effect, and because a reader working out what a `hash` list entry
covers should not have to go looking for the exclusion.

Targets are sorted for the same reason the `hash` list is: a link set has no
authored order, and reordering a `links:` list is not a change to what was
promised.

### `body.<section>` is refused, not guessed at

How a schema declares its sections is open question 5. A hash list naming
`body.<section>` therefore cannot be hashed, so the artifact cannot be ratified,
and the refusal says which question is open.

Refusing is the only answer that does not settle open question 5 by accident.
Any guess — the whole section, the section without its heading, the empty string
when it is missing — would be permanent the moment one person ratified under it.

### An artifact with nothing binding cannot be ratified

Two cases, one answer. A kind with no `_schema/{type}.yaml` declares nothing
binding; a schema with `hash: []` declares nothing binding on purpose, which is
what `desire.yaml` and `fact.yaml` already say.

Both refuse, and the alternative is worse than it looks: a hash over nothing is
the same digest for every version of the file, so standing granted against it
would survive every edit ever made. That is the exact failure this whole
mechanism exists to prevent, arriving through the one door nobody would think to
check. It also gives `desire.yaml`'s comment — *a desire cannot be ratified in
place* — a mechanism rather than a hope.

## Alternatives

**Canonical JSON, or re-emitted YAML.** Rejected. Both are read-back formats, so
both carry decisions about number representation, string escaping and key
collation that would land in the middle of an authority mechanism and be
inherited rather than chosen. Re-emitting YAML is worse still: it would mean
kept knowing how to write the human-authored file it is forbidden from
rewriting, which is the requirement [0029](0029-dependencies-pass-a-test.md)
retired.

**Hash the file's bytes for `body` and the structure for the rest.** Rejected as
what 0022 already rejected, arriving through the back door: raw bytes make the
hash a fact about a checkout rather than about content.

**Include the artifact's id or kind in the preimage.** Rejected. 0021 puts the
id outside what binds — it says which promise this is, not what is promised —
and every record is keyed by `(artifact, hash)` already, so a collision across
two artifacts is not reachable. Including the kind would mean moving a file
between type directories drops standing, which is a rule nobody asked for and
which 0003 does not imply.

**Hash-list order matters.** Rejected: it makes tidying a schema a bulk
de-ratification, and nothing about what binds changed.

The alternatives on whether a manifest edge binds are in
[0030](0030-a-link-manifest-is-one-file-per-subject.md), with the rule.

## Consequences

- **The preimage is frozen at `norm:1`.** Any change to normalization, to the
  encoding, to the sort rules, or to what a reserved name resolves to is a new
  `norm`, and old records are re-hashed under the version they were written
  with. That is 0022's insurance and this decision is what it now insures.
- **Two of 0003's three reserved names work and one refuses.** `body` and
  `links.<relation>` resolve; `body.<section>` is an error until open question 5
  closes, and closing it is a `norm` bump for any corpus that had used it —
  which is none, because it cannot have been used.
- **A schema with `hash: []` is a kind that cannot be ratified**, which turns
  `desire.yaml`'s and `fact.yaml`'s comments into behaviour. `kept status`
  reports those artifacts distinctly from ones nobody has got round to
  ratifying, because the two mean different things to whoever is reading.
- **A whitespace-only or prose-only edit keeps standing**, and a one-word change
  to a bound field drops it. Both get a test.
- **`kept doctor` gains a check** that 0022 asked for and this makes concrete:
  an artifact whose stored bytes differ from their normalized form is harmless
  and worth surfacing before someone diagnoses a hash by eye.
