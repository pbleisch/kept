---
id: 0042-doctor-adopts-a-corpus
type: decision
title: 0042 — `doctor` is about identity, and `--fix` writes one block
links:
  - { target: decision:0021-identity-is-authored-absence-is-not-an-act, relation: implements }
  - { target: decision:0027-the-machine-facing-surface, relation: implements }
  - { target: decision:0029-dependencies-pass-a-test, relation: implements }
  - { target: decision:0032-standing-is-a-fold-over-acts, relation: implements }
---

# 0042 — `doctor` is about identity, and `--fix` writes one block

## Context

`kept doctor` has an answer already written down, in the table of verbs that
are unwritten rather than blocked:

> `doctor` is unwritten rather than blocked: its checks already run — path/id
> divergence and duplicate ids in `kept list`, standing without content in
> `kept status` — and no verb collects them

Three decisions specified a doctor check and none of them built one. 0021 says
a filename that is not the slug is a diagnostic. 0032 says doctor gains a check
for an act naming an artifact id no corpus claims. 0029 sanctions exactly one
kind of write into a human-authored file, and names doctor as the thing that
would do it:

> | `doctor`, adopting a corpus | one missing `id` | a **line-level insert**
> into the frontmatter block |

Adoption is the case that makes this urgent. This repository has thirty-nine
ADRs and a schema for them, and every one of the thirty-nine is a file in a
type directory rather than an artifact, because identity is authored (0021) and
nobody authored it. That is not a defect in the files; it is what adopting kept
into an existing repository looks like from the inside, and **it is the path
every future kept user walks**. A one-off script would get this repository over
the line and teach the next person nothing.

## Decision

### `doctor` is about identity, and only about identity

**What names an artifact, and whether everything the ledger names is here.**
Not schema violations, not broken links, not predicates that will not parse:
those are `kept list`'s, they are about content, and content is what a person
wrote.

| finding | from |
|---|---|
| a markdown file in a type directory with no frontmatter | 0028 — placement is not enough |
| frontmatter with no `id`, or an `id` that cannot be a path component | 0021 |
| a filename that is not the slug | 0021 |
| frontmatter declaring a type its directory contradicts | 0028 |
| frontmatter that will not parse, so nothing can be read from it | 0002 |
| two artifacts claiming one id | 0021 — a load error, so every verb reports it |
| standing whose content cannot be found | 0021, 0032 |

That boundary is what makes `--fix` safe to have at all. Everything on the list
is about *which artifact this is*, and identity is the one thing 0029 permits
kept to write into a file somebody else authored. A doctor that also fixed a
missing required field would be editing what the artifact says, which is a
different act with a different author.

### Exit codes are 0027's, unchanged

`1` when something with standing is absent, because 0027 puts `absent` in the
known-false half deliberately. `2` when the corpus will not load, which is what
two artifacts claiming one id is. **`0` otherwise, findings printed** — the
same as `kept list`, which reports forty-two problems and exits 0.

A findings count is not an exit code here, and that is deliberate. 0027's rows
are about commitments and evidence: `1` means the world disagrees with
something somebody committed to, and `3` means there is no evidence either way.
A filename that does not match a slug is neither. Giving it a code would put a
third meaning on a row that two decisions have already argued over.

### `--fix` writes one frontmatter block, and nothing else

Two shapes, and which one applies is decided by what the file already has.

**A file with no frontmatter block gets one kept composed**, carrying `id`,
`type` and `title` and no other key. It is prepended; not one byte of what was
there moves. `id` is the filename without its extension, because during
adoption the filename is the only thing that can supply an identity without
inventing one — and it is also what makes the path/id check above agree
afterwards. `type` is the directory. `title` is the text of the first H1, and
is omitted when there is none.

**A file that has a block and no `id` gets one line inserted into it.** Only
`id`, which is 0029's sanction word for word. `type` and `title` are not added
there: a block that exists is one its author wrote, and a key they left out may
be a key they left out on purpose.

Nothing else is ever written. Not a reformat, not a reordered key, not a
normalized line ending — the file's own line endings are used for the lines
inserted, and a byte-order mark stays in front. A `--fix` that touched anything
beyond the block would be re-emitting a document kept parsed, which is exactly
the round-trip requirement 0029 retired.

**Every file it changed is named, and so is every finding it left alone**, with
the reason. Two are always left alone: frontmatter that will not parse, because
inserting a line into a block nothing could read is guessing; and a duplicate
id, because choosing which of two files keeps the name is a decision and this
verb does not make decisions.

### There is no `--json` yet

0027 makes the structured surface a versioned contract, and nothing consumes a
doctor document. A shape published now would be a contract shaped by
imagination rather than by a caller, and versioned from the moment it existed.
It is added when something reads it, and the human format is explicitly not a
contract in the meantime.

## Alternatives

**A one-off adoption script.** Rejected. The work is identical and the outcome
is not: a script gets this repository ratified and leaves the next corpus in
exactly the state this one is in now. Adoption is a supported path or it is a
thing the author did once by hand.

**`--fix` derives the slug from the title rather than the filename.** Rejected.
0021 makes the filename not the identity, which cuts both ways: it is not
authoritative, and it is also the only thing on hand that a person chose. A
slug derived from an H1 would rename thirty-nine files or leave every one of
them reporting path/id divergence the moment it was written.

**`--fix` also fills in required fields the schema declares.** Rejected on the
identity boundary above. A missing `statement` is content, and a machine
guessing at content is how a corpus fills up with sentences nobody wrote.

**`--fix` writes the `**Status**` line's relations into `links:`.** Tempting,
because those lines are regular enough to parse. Rejected: they are regular
enough to parse *wrongly*. "supersedes the `writer:` sketch in 0011" and
"amended by 0037" and "corrects the tier rule in `layout.md` and 0018" are
different assertions in the same shape, and one of them points backwards. A
structural edge that is wrong is worse than one that is absent, because the
corpus will then resolve it and report nothing. Those are read and written by a
person, once, and this is the pass where that happens.

**Report the findings without a fixer.** Rejected: the fixer is the reason the
verb is worth building now rather than after increment 5. Reporting thirty-nine
files that need a line each, and then not adding the line, is a tool that has
described the work.

## Consequences

- **kept writes into files a person authored**, for the first time, in one
  place, bounded to one block. That is worth stating plainly rather than
  leaving in a table in 0029.
- **`--fix` is not idempotent-by-accident but by construction**: a file that
  already has an `id` is not a finding, so running it twice does nothing the
  second time. The test that says so is the one that would catch a fixer that
  re-prepended.
- **Adoption is now a documented path** with a verb behind it, and the next
  corpus adopted into kept walks it rather than being scripted into shape.
- **`doctor` will grow.** 0032 named one check and this collects it; more will
  arrive as more of the ledger exists. The remit above is what keeps that
  growth from turning it into `kept list` with a second name.
