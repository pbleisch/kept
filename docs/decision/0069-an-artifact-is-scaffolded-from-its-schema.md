---
id: 0069-an-artifact-is-scaffolded-from-its-schema
type: decision
title: 0069 — A new artifact is scaffolded from its own schema
fields:
  statement: >-
    kept new writes a new artifact's frontmatter and body headings from its
    kind's schema rather than from a template, and writes nothing into a field
    the schema's hash list names.
links:
  - { target: decision:0010-frontmatter-binds-body-informs, relation: implements }
  - { target: decision:0021-identity-is-authored-absence-is-not-an-act, relation: depends-on }
  - { target: decision:0027-the-machine-facing-surface, relation: extends }
  - { target: decision:0040-a-sectioned-body-and-its-names, relation: depends-on }
---

# 0069 — A new artifact is scaffolded from its own schema

## Context

There was no way to create an artifact. `list`, `show`, `ratify`, `release`,
`status`, `drift`, `why`, `tick`, `repair`, `land`, `ledger`, `doctor` and
`init` all read something that already exists or act on it, so every author
hand-wrote frontmatter, and correctness came from copying a neighbour or from
having read `docs/_schema/`.

That produced a measured defect rather than a hypothetical one. Decisions 0062
through 0066 — five consecutive artifacts authored on 2026-08-24 — all put
`statement:` at the top level, where `Artifact.Fields` never looks. Each hashed
as though it had no statement, which is the opposite of what
`hash: [statement, body.decision]` promises. 0060, 0061, 0067 and 0068, written
either side of them, nested it correctly. That spread is what copying whatever
is nearest looks like; it is not ignorance, and a person reading the schema
would have got it right.

Commit `3dfc1e1` made the mistake **loud**, by reporting a declared field
written at the top level as `misplaced-field`. This is the other half: making
the correct shape **cheap**. A report catches the sixth occurrence; a scaffold
means there is no sixth.

Two constraints bound what such a verb may do, and both are already decided.

**Standing is created only by ratification, and only by a person.** Anything a
scaffold writes will be ratified later, and ratification binds a content hash —
so whatever kept invents in a binding field is what a person can end up granting
standing to without ever having written it.

**A schema's default is recorded, never substituted.** `internal/schema` says so
in `Field.Default` and `TestCheckDoesNotFillInADefault` enforces it on the read
path: filling in an absent field would put a value in the artifact that its
author did not write. A writer that substitutes defaults breaks the same rule
from the other side.

## Decision

### `kept new <type> <slug>`, generated from `_schema/<type>.yaml`

The verb reads the kind's [`schema.Schema`](../../internal/schema/schema.go) and
emits what it declares: the field list in its authored order, under `fields:`;
the H2 headings its `body:` names; the relations its `links:` admits. Nothing is
per-kind and there is no template.

A template per kind would be a second copy of the seven schema files, and a
second copy of a schema is a thing that can disagree with the schema — the same
class of defect this verb exists to prevent, one level up. The practical test is
in the suite: a field added to a schema appears in the scaffold with no code
change.

### A field the hash list names is never given words kept invented

**The hash list is the discriminator, and kept already has it.** A bindable
field is one a ratification is granted against, so a promise scaffolded with
`statement: TODO` can be ratified exactly as it stands — which grants standing
to the word TODO, in the field that says what is promised. Nothing downstream
would report it: `TODO` is a well-formed statement, and `kept status` would call
it ratified and clean.

So a bindable field is either the value its author supplied or nothing at all.
When one is **required as well** — `promise.statement`, `policy.statement`,
`policy.applies_to`, `canon.statement` — kept refuses to write the artifact and
names the field, its description and the flag that supplies it. The value
arrives on `--set <field>=<value>`, which is a person writing it.

An enum is refused for a neighbouring reason: every value an enum admits is an
assertion about the artifact, and the one value that would not be — a
placeholder — is outside the enum and would be reported by the next read. Its
declared default is a statement about what absence means, not a value to write.

What is left is what kept can write without asserting anything: a required
string nothing binds gets `TODO`, and a required map or list gets an empty one.
Over the seven kinds shipped here, that is `fact.claim` and `desire.want`, in
two kinds whose `hash` list is empty and which have no standing to grant.

**Guidance goes in YAML comments**, which are read by the person filling the
file in, survive in the file, and are not content: nothing parses one into a
field and no hash covers one. Each field's comment carries its type, whether it
is required, **whether it binds**, its enum values, what its absence means, and
the schema's own description.

### An optional field is written as a comment, not as an empty key

Absent is what the author has said so far, and an empty key is a value: a
required field reads `` as unanswered, and `max_age:` with nothing after it is a
duration nobody could parse. So an optional field appears as `# <name>:` — the
shape, with nothing asserted.

When *every* declared field is optional, `fields:` is commented out with them.
A `fields:` key with only comments under it parses as null, which every read
reports as a `fields:` block that could not be read.

### The body follows what the schema says binds

Three cases, and the schema decides which.

- **The whole body binds** — `canon`, whose `hash` names `body` — so the
  scaffold writes **no body at all**. Every byte it wrote there would be inside
  the hash a ratification is granted against, the H1 included.
- **The body is read as sections** — `decision` — so one heading per declared
  section and nothing under any of them. A declared section that is absent is
  reported by every read, and a bindable one (`body.decision`) must be empty for
  the reason above.
- **Otherwise**, the H1 this corpus starts every artifact with, and nothing
  else.

The heading a section identifier is written as has to produce that identifier
again under `schema.SectionID`, or it opens a section the hash does not cover.
A heading that does not round-trip is written as the identifier itself.

### The slug is required and kept never invents one

`kept new decision 0069-an-artifact-is-scaffolded-from-its-schema` takes the
slug exactly as typed. kept does not pick the next decision number.

The reason is the same one that makes the verb schema-driven: **numbering is
per-kind knowledge and no schema declares it.** Decisions are numbered here;
promises, policies, canon, facts, desires and documents are not, and nothing in
`_schema/decision.yaml` says so. Encoding it would be exactly the second copy of
a convention this decision refuses everywhere else.

The usual argument for minting one — that it prevents collisions — does not hold
here either. `AGENTS.md` already records that a number is claimed by whoever
reaches `main` first, so two branches cut from one commit both pick the same
free number whether a person or kept picks it. What kept would buy is one `ls`,
at the cost of a naming rule in code.

**The file is `<type>/<slug>.md`.** The identity is the id in frontmatter and
renaming the file changes nothing (0021) — but a filename that is not the slug
is a `path-mismatch` problem every read reports, and a scaffold whose output
generates a finding is not a scaffold.

### It writes by default and `--stdout` prints

Writing is the point: the friction being removed is having to produce a correct
file. `--stdout` prints the same bytes and creates nothing, for reading the
shape before it lands in a corpus, and its output is byte-identical to the file
— a test pins that, so the flag cannot become a second renderer.

It is not a machine format and does not touch 0066's *the machine surface is
`--json` and nothing else*: it is one destination for bytes kept already
composes, with no second thing to version.

**It refuses to overwrite.** The file is created with `O_EXCL`, so an existing
one is left exactly as it is, whatever it holds — `policy:nothing-kept-writes-rewrites-what-a-person-wrote`
is ratified and this is the verb it most obviously applies to. A slug the corpus
already claims is refused before anything is written, because two artifacts
claiming one id is the single thing that stops a corpus loading.

### It creates nothing but the artifact

No ledger is opened, no act is appended, no standing is minted and no truth is
recorded. A new artifact is unratified, which is what a new artifact is.

## Alternatives

**A template per kind.** Rejected in the first paragraph of the decision: seven
copies of seven schemas, kept in step by hand.

**Write a placeholder into every required field, bindable or not.** This is what
makes the verb frictionless and it is the one thing it must not do. `statement:
TODO` is ratifiable as it stands, and nothing downstream would ever say so.

**Write the schema's declared default into optional fields.** Rejected: it is
`TestCheckDoesNotFillInADefault` broken from the writing side. `trigger:
cadence` in a scaffolded promise is a value its author did not write, and
`trigger` binds.

**Let `--set` take YAML, so a map or a list can be supplied.** Rejected for now:
it puts a parser in an argv element for a case no kind has — no shipped schema
declares a required map or list. What it costs is stated rather than hidden: a
kind that added one could not be scaffolded cleanly, and `kept new` says so
instead of writing a string where a map belongs.

**Mint the next decision number.** Rejected above: per-kind knowledge no schema
declares, buying an `ls` and preventing no collision that actually happens.

**Print only, and never write.** Tempting, because it keeps kept out of the
corpus entirely. Rejected: the friction being removed is producing a correct
file, and `kept new … > docs/decision/0069-….md` is a redirection that will
happily clobber, which is worse than the refusal `O_EXCL` gives.

**Make `kept ratify` refuse an artifact whose bindable field still holds
scaffold text.** Not decided here, and deliberately not built. It is a second
check for a hazard this decision removes at the source — there is no scaffold
text in a bindable field to find — and it would need a marker in the corpus for
`ratify` to recognize, which is content kept wrote standing inside the hash a
person is granting. If a future scaffold does put words in a binding field, that
is the decision to revisit, not this guard.

## Consequences

- **kept writes corpus files for the second time.** `doctor --fix` was the
  first, and its justification was that it writes frontmatter kept can derive
  with nobody deciding anything. The same argument carries here only because of
  the bindable-field rule: everything `kept new` writes is either the envelope,
  a value a person supplied, a comment, or a placeholder in a field nothing
  binds. Widen what it fills in and that justification stops holding.
- **A verb is added to 0027's surface.** Exit codes are unchanged: `0` when it
  wrote, `2` for a refusal, and nothing here can reach `1`, `3` or `4` because
  it evaluates nothing.
- **Three kinds cannot be scaffolded in one word.** `promise`, `policy` and
  `canon` need `--set`, which is friction, and it is the friction that keeps
  invented words out of a binding hash. The refusal names every field, its
  description and its flag, so the second attempt is a copy-paste.
- **A new field in a schema reaches authors immediately**, with its description,
  without anyone updating a template or a doc.
- **`kept new` is now a reader of `_schema` at the same depth `kept list` is.**
  A schema that will not load is already a load error, so nothing new breaks;
  what is new is that a schema's field *descriptions* are now user-facing text.
