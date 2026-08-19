---
id: 0040-a-sectioned-body-and-its-names
type: decision
title: 0040 — A sectioned body, and how a heading becomes a name
links:
  - { target: decision:0010-frontmatter-binds-body-informs, relation: extends }
  - { target: decision:0031-the-binding-preimage, relation: extends }
---

# 0040 — A sectioned body, and how a heading becomes a name

## Context

0010 ends with two sentences and no mechanism:

> **When a kind genuinely needs two prose blocks**, use H2 sections in the body,
> parsed by heading, with the schema naming which sections it expects so they
> can be linted. Rule: an unsectioned body is the kind's primary prose field; a
> sectioned body yields named fields.

0003 reserved `body.<section>` as a name a `hash` list may use, and
`internal/schema` records it with a comment saying nothing resolves one yet.
`canonical.resolve` refuses outright rather than guessing:

> the %s schema binds on %q, and how a schema declares its body sections is
> undecided (open question 5)

That refusal is the right shape and it blocks the decision kind, which is the
first kind that actually needs this: an ADR's Context and its Decision are two
prose blocks, and only one of them should bind.

Open question 5 names three sub-questions. Trying to answer them surfaces a
fourth that is worse than the other three together.

1. How the schema declares sections.
2. Whether a missing declared section is an error or an empty field.
3. How `body.<section>` in a `hash` list behaves when the section is absent.
4. **How a heading becomes a section identifier.** `## This Section Has Spaces`
   → what? Case, whitespace, punctuation, code spans, non-ASCII, and two
   headings that land on one name.

**The fourth is one-way.** A section identifier appears in a schema's `hash`
list, so it is inside the binding preimage. Change the rule later and every
artifact of that kind re-hashes and drops standing at once — which is 0003's
bulk-re-ratification consequence arriving by accident rather than on purpose,
and 0022 exists precisely so that a refinement of the rules is not mistaken for
somebody having edited the corpus. It is settled here with 0031's rigour and
frozen on the same terms.

## Decision

### The schema declares the sections, and `body:` is where it does it

`body:` already names what the markdown block becomes. It takes a **string** for
a kind whose body is one prose field, which is every schema today, and a **list
of section identifiers** for a kind whose body is several:

```yaml
body: why                                  # promise: the block is one field

body: [context, decision, consequences]    # decision: the block is three
```

Nothing new is declared beside it. The two forms are the two cases 0010 names,
and a reader can see which a kind is from one line.

**A declared name must already be an identifier**, by the rule below. A schema
writing `body: [Decision]` is a schema error rather than a diagnostic, because
leniency stops at the schemas themselves: a name that had to be normalized
before it could be compared would make `body.Decision` and `body.decision` two
spellings of one binding field, and a `hash` list must have exactly one.

### How a heading becomes an identifier — frozen at `norm:1`

Applied to the body after 0022's read-time normalization has already made the
bytes comparable:

1. **Only an ATX H2 opens a section**: a line beginning `##` at column zero,
   followed by a space or the end of the line, and **not inside a fenced code
   block**. A setext heading does not, because its H2 marker is `---`, which is
   the frontmatter fence and a horizontal rule as well. An indented `##` does
   not, because at one to three spaces it is prose in a list and at four it is
   code.
2. Take the text after the marker to the end of the line. Drop a trailing run
   of `#` and the whitespace before it — that is ATX's closing form — and trim
   what is left.
3. Normalize to **NFC**, then apply **Unicode lowercase**. NFC for 0022's
   reason: macOS and Linux disagree about NFC and NFD, and a heading with an
   accent must not hash differently on two machines.
4. **Every run of characters that is not a Unicode letter or digit becomes one
   `-`**, and leading and trailing `-` are dropped.
5. **Nothing else.** No markdown is interpreted, so a code span's backticks are
   punctuation like any other; nothing is transliterated; and there is no list
   of characters treated specially.
6. A heading whose text produces an empty identifier is **not a section**. It
   is a heading, it stays in the body, and it opens nothing.

```
## Context                      → context
## What this does not decide    → what-this-does-not-decide
## The `run:` line              → the-run-line
## Kael's eyes                  → kael-s-eyes
## Café                         → café
```

`kael-s-eyes` is ugly and it is deliberate. The alternative is a list of
characters that get dropped rather than replaced — apostrophes first, then
ampersands, then en dashes — and that list is a thing that grows. Every growth
re-hashes every artifact of every sectioned kind. One rule with no exceptions
is worth an ugly name that its author can see and rename at the moment they
write the heading, since the schema has to name it too.

### A section's text is what lies under its heading

From the end of the heading line to the start of the next H2 or the end of the
body, **with leading and trailing blank lines removed**. That last is 0022's
argument again: a blank line after a heading is not something an author meant,
and a corpus must not de-ratify because somebody added one.

**Text before the first H2 belongs to no section.** For a decision that is the
H1 and whatever sits under it, and it is not lost: `body` — the reserved name
for the whole markdown block — still means the whole block, headings included,
and is still nameable in a `hash` list.

### The three answers open question 5 asked for

**A missing declared section is neither an error nor an empty field. It is
absent, and it is reported.**

Absent rather than empty, because 0031 spent its `present`/`absent` marks
exactly so that a field the author did not write and one they wrote as empty
are different bytes. A missing `## Decision` and an empty one say different
things, and collapsing them would make deleting a section and blanking it
indistinguishable inside the hash.

Reported rather than silent, because that report is what makes a heading typo
findable. `## Desicion` produces two signals — an undeclared section nobody
declared, and a declared section nobody wrote — and without the second, a typo
shows up only as an artifact that mysteriously stopped being ratified, which
sends its author looking for an edit that never happened.

A diagnostic rather than a load error, by the line problem.go already draws:
the artifact still is what it is, and the map from id to artifact is not made
wrong by a heading somebody forgot. It can still be ratified, at a hash that
records the section as absent.

**An H2 that maps to no declared identifier is an undeclared section: legal,
kept, and unbindable.** This is 0002's leniency applied to the body, and it is
the same answer `Schema.Undeclared` already gives for fields — an undeclared
field is not a violation, it survives the read, and the caller gets it. So
`## Alternatives`, `## What this does not decide` and `## Not a session` stay
legal in this repository's own ADRs without the schema having to enumerate
every heading anyone might write. What an undeclared section may not do is
bind: a `hash` list naming `body.<something-undeclared>` is refused as
unbindable, because the schema is what says which prose is part of the
commitment.

**`body.<section>` in a `hash` list, when the section is absent, encodes as
absent.** The mechanism is already built and needs nothing added: the preimage
writes the name, then `Absent`, and moves on.

### Two headings on one identifier are ambiguous, and binding on one is refused

`## The decision` and `## The Decision` both produce `the-decision`. Nothing
here picks one. It is reported, and a `hash` list naming that identifier is
refused as unbindable, so the artifact cannot be ratified until a person
resolves it.

First-wins would be the natural implementation and it is the wrong one: which
heading is first is a property of how the document is ordered, so an ordinary
edit that moves a section would silently change what a ratification was granted
against — a change to binding content with no change any diff shows as one.

### It is part of `norm:1`, and not a version of its own

The rule feeds the binding preimage, so `canonical.Version` already names it.
Changing any of it — the marker, the case rule, the substitution, what a
section's text spans — is a norm bump under 0022's machinery, with old records
re-hashed under the version they were written with. A second version number
beside `norm` would be a second thing to check and a second way for two records
to look comparable when they are not.

`internal/schema` owns the identifier, because a schema is what declares one;
`internal/corpus` splits the body into sections; `internal/canonical` hashes
them. The direction is forced — canonical imports corpus and schema, and corpus
imports schema — and it is a package boundary rather than a second home for the
rule: `canonical.Version`'s own comment names `schema.SectionID` as under it.

## Alternatives

**The schema declares headings verbatim** — `body: ["What this does not
decide"]` — **and no identifier rule exists.** Rejected. The name has to appear
in a `hash` list, and `body.What this does not decide` is a sentence rather
than a name: nothing could parse the list, and the reserved-name prefix `body.`
would have to grow a quoting rule that a YAML list already spent.

**Every H2 becomes a field, and the schema declares nothing.** Tempting,
because it needs no declaration at all. Rejected, and it is the important
rejection: with no declaration, `## Desicion` is not a typo — it is a
perfectly good section called `desicion`, and `body.decision` silently becomes
absent. Nothing can report what nothing declared. Making the schema
authoritative is what turns that from a silent re-hash into a message.

**Drop apostrophes and ampersands rather than replacing them**, or spell `&` as
`and`. Rejected above: an exception list grows, and every entry added to it
re-hashes every sectioned artifact in every corpus. The cost of the plain rule
is an occasional ugly name; the cost of the list is a one-way door that gets
opened repeatedly.

**Transliterate non-ASCII** — `café` → `cafe`. Rejected. Transliteration is
locale-dependent and unbounded, and 0022 already chose NFC over anything
cleverer for exactly this reason. A letter is a letter.

**A missing declared section is a load error.** Rejected: it would make one
forgotten heading stop a corpus from loading, and 0002's leniency plus
problem.go's identity test both say otherwise. The artifact still is what it is.

**A missing declared section is an empty string.** Rejected: it makes deleting
a section and emptying one produce the same hash, which is the collision
0031's marks exist to prevent.

**A missing declared section is silent.** Rejected: it is the half of a typo's
signal that says what to do about it.

**First heading wins on a duplicate identifier.** Rejected above — it makes
document order part of the binding hash without saying so.

**Sections carry `required:` the way fields do.** Rejected as machinery bought
in advance. Declaring a section already says the kind expects it; a kind that
does not expect one simply does not declare it, and this repository's own
`## Alternatives` — in thirty-one ADRs of thirty-nine — is exactly that case.
If a kind ever needs both a bindable optional section and a report when it is
missing, the flag can be added then, additively, without re-hashing anything.

## Consequences

- **Open question 5 is closed**, and the decision kind is unblocked.
  `canonical.resolve` stops refusing `body.<section>` and starts answering.
- **A schema's `body:` key now has two shapes.** A reader has to know that a
  list means sections, which is one thing to learn and is visible in the file.
- **A heading is binding content for a sectioned kind**, so editing
  `## Decision` to `## The decision` drops standing on every ADR it is done to.
  That is correct — it changes which prose is the commitment — and it is worth
  saying out loud, because a heading does not look like a field.
- **The rule is frozen.** Changing case handling, the substitution character,
  or what a section's text spans is a `norm` bump and a deliberate
  re-ratification, never a fix.
- **Three new guards, and each gets a test that breaks it**: a declared section
  that is absent, an H2 that binds nothing because nothing declared it, and two
  headings that land on one name.
