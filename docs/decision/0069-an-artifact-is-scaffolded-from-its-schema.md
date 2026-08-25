---
id: 0069-an-artifact-is-scaffolded-from-its-schema
type: decision
title: 0069 — A new artifact is scaffolded from its own schema
fields:
  statement: >-
    A schema declares its kind's field list, body shape and id shape; kept new
    generates a new artifact from all three rather than from a template, mints
    the declared id shape, and writes nothing into a field the hash list names.
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

**The same hole shows up in the id.** Sixty-eight of sixty-eight decisions are
numbered `NNNN-slug`, and 0 of 13 policies and 0 of 8 promises are. The number is
not a filename habit: it is in the `id:`, so
`decision:0040-a-sectioned-body-and-its-names` is what every citation of it
writes. No schema declared it and nothing validated it, so a hand-written
decision with a plain-slug id was accepted in silence — and a generator built
without the declaration would have produced the only unnumbered decision in the
corpus, which is drift from convention introduced by the tool built to stop it.

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

### The schema declares the id shape, and `kept new` mints it

```yaml
# _schema/decision.yaml
id:
  shape: numbered
  width: 4
```

Absent is `shape: slug`, which is every other kind here and needs no
declaration. `kept new decision what-was-decided` mints
`0069-what-was-decided` — the highest number the kind already has, plus one,
zero-padded to the declared width — and the corpus reports an id that is not its
kind's declared shape.

**The set of shapes is closed rather than a pattern**, and that is the whole
argument. A `pattern:` regex is the general answer and it answers only half the
question: nothing derives the next value from `^[0-9]{4}-`, so a corpus with a
pattern would need a *second* declaration saying how to produce a matching id —
the rule living in two places that can disagree, which is what this decision
refuses everywhere else. One declaration has to serve both readers, so it is a
named shape with the one parameter minting needs. A kind that wants a third
shape adds a value to the set rather than a language, and until one exists there
is nothing to design against.

**Only the number is minted.** The slug stays required and is taken verbatim;
kept authors no words. A slug that already carries a number is refused rather
than accepted or renumbered, because two sources for one number is the same
two-places problem — renumbering is an edit to the file, with a re-ratification
behind it where the artifact has standing.

0021 already puts this here: the slug is *minted by the CLI when the artifact is
created, which is also where uniqueness is enforced*. And an ordinal is not the
composed identifier 0012 refuses: it names one authored thing a person cites in
prose, nothing is encoded in it, and it is not allocated by concurrent runs.

### Two artifacts numbered alike are reported, and it is not a load error

Minting takes the highest number and adds one, so nothing kept does creates a
collision — but two branches cut from one commit both find the same highest
number, which `AGENTS.md` already records as *a number is claimed by whoever
reaches `main` first*. The corpus reports it.

It is a diagnostic rather than the load error a duplicate **id** is, by
`problem.go`'s own line: the slugs differ, so the map from id to artifact is
still right and every stream keyed to one is still that artifact's. What is
wrong is that a number in an id is how a person cites a decision in prose, and
two artifacts numbered alike make every such citation ambiguous to a reader even
though no code is confused by it. Reported early is what makes renumbering cheap
— it costs a re-ratification once the artifact has standing, and nothing before.

Neither finding is ever corrected. An id is authored and does not change (0021),
so renumbering one is a person's act; `doctor --fix` writes identity nobody
wrote and does not touch identity somebody did.

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

**Leave the number to the author, on the grounds that numbering is per-kind
knowledge no schema declares.** This was the first answer here and it was wrong
in its premise rather than its logic: the schema is exactly where per-kind
knowledge belongs, and the fix is to declare the shape rather than to skip it.
Skipping it would have made `kept new decision foo` produce `id: foo`, the only
unnumbered decision in a corpus of sixty-eight.

**Declare the shape as a `pattern:` regular expression.** Rejected in the
decision: it validates and cannot mint, so it needs a second declaration beside
it. It is also a mini-language in a schema, and every character an author adds to
one is a character the reader and the generator must agree about.

**Make a contested number a load error, as a duplicate id is.** Rejected: the map
from id to artifact is still right, which is the test `problem.go` draws the line
with, and refusing to load a corpus over two ADRs that a rebase will renumber
would be a large cost for an ambiguity only a reader notices.

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
- **A schema file gains a top-level `id:` key, and it de-ratifies nothing.**
  Changing a `hash` list is a bulk re-ratification (0003), so the question about
  any new key is what it costs; this one binds nothing, because 0021 already
  keeps the id out of every hash list — it says which artifact this is rather
  than what is promised. Verified rather than argued, before and after: the
  corpus reads 89 ratified either way, and
  `TestDeclaringAnIDShapeMovesNoArtifactsHash` pins it.
- **A check is turned on over a corpus that already satisfies it.** All
  sixty-eight existing decisions are `NNNN-slug` with four digits, so the new
  finding reports nothing today, which is the only safe way to add one.
- **`kept new` is now a reader of `_schema` at the same depth `kept list` is.**
  A schema that will not load is already a load error, so nothing new breaks;
  what is new is that a schema's field *descriptions* are now user-facing text.
