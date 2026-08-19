# 0041 — A decision is a kind, and there is no `record` kind

**Status** accepted · adds the seventh authored kind to
[0008](0008-work-is-not-an-entity.md)'s list · answers what
[0007](0007-canon-is-ratified.md) settles for canon, for the kind that looks
most like it and is not · needs
[0040](0040-a-sectioned-body-and-its-names.md), which is what makes the schema
below expressible

## Context

`docs/self-hosting.md` has said since it was written that `docs/decision/*.md`
becomes **decision** artifacts. Nothing declares the kind, so all thirty-nine
of them are files in a type directory and not artifacts, and `kept list` says
so thirty-nine times.

The routes do not distinguish it. An ADR is **ratified**, has **no cadence**
and has **no predicate** — which is the canon row of the nouns table with both
of canon's optional parts unused. `model.Kind` exists for directory placement,
schema selection and keeping candidates out of a packet, and on the first two
of those a decision and a canon entry would behave identically.

So the kind has to justify itself on something other than its routes, and if it
cannot, it should be canon. Anyone reading this later will ask, and the answer
is weaker if it has to be reconstructed from a schema file.

## Decision

**`decision` is an authored kind: ratified, no trigger, no predicate, and a
body read as sections.**

```yaml
type: decision
hash: [statement, body.decision]
body: [context, decision, consequences]
```

Two things distinguish it, and neither is a route.

### The body is sectioned, and one section binds

`hash: [statement, body.decision]` is 0010 reaching inside the body, and it is
the whole point. **The decision section binds; context, alternatives and
consequences inform.** A consequence discovered two years later can be added to
an accepted ADR without dropping its standing, because a consequence is what
followed from the decision rather than part of it. Rewriting what was decided
drops standing, which is correct: nobody approved the new wording.

Canon cannot have that shape. `_schema/canon.yaml` binds `[statement, body]` —
the whole block — because for canon the prose *is* the commitment. **One kind
has one hash list**, so a kind whose prose is entirely binding and a kind three
quarters of whose prose is a record of reasoning cannot be the same kind. That
is the structural argument, and it is the one that settles it: the difference
is not a matter of taste about nouns, it is that the two want different answers
from `_schema/{type}.yaml`.

`alternatives` is deliberately not declared. Thirty-one of thirty-nine ADRs
have one and eight had no alternative worth recording, and a schema reporting
those eight would be reporting on a judgment their author made rather than on a
defect. It stays an undeclared section under 0040 — legal, kept, and
unbindable — along with `## What this does not decide` and the rest.

### It is superseded rather than edited, and canon is not

Canon and a decision come out of force by different routes, and neither one's
route is available to the other.

**Canon stops being true when the world moves**, and the answer is drift and a
repair that moves the world back. **A decision stops being in force when a
later decision supersedes it**, and nothing about the world can do that. An
accepted ADR whose subject has changed underneath it is not drifting; it is
either still right about the decision that was made, or it has been superseded
by one that is.

That is why a decision has no trigger and no predicate, and why the absence is
not an oversight. There is nothing to gather. `0024` decides that signing is
designed and deferred: no measurement of any tree tells you whether that is
still the decision, and the only thing that could change it is a person writing
`0043`.

[0039](0039-the-ledger-records-acts.md) states the editing rule this implies
and its one exception: a terminology rename is edited in place, because a
reader who acted on the old text and one who acts on the new do the same thing;
a change to what was decided is superseded.

### Why it is ratified at all

Because these oblige. `AGENTS.md` requires an ADR before an architectural
change and tells a reader to consult `docs/lineage.md` before proposing
something a decision already rejected; the invariants it lists cite decisions
by number; and `self-hosting.md`'s first structural promise is that every
policy cites a decision that exists. Something that future work is held to is
something with standing, and standing is created by ratification and by nothing
else.

It also gives amendment its shape. Under
[0032](0032-standing-is-a-fold-over-acts.md) the register folds an artifact's
acts and compares the set against what the file says now, so an ADR edited in a
way that changes what it decided reads as unratified until a person ratifies it
again. That is the supersede convention with a mechanism behind it rather than
a habit.

### There is no `record` kind, now or later

An ADR looks like it wants a kind called `record` — something historical, true
by construction, that happened. The deck's nouns table carried a `Record` row
for exactly that intuition, with `none · — · it happened`.

**That row is the `document` row.** Anything historical that does not oblige
has no standing, and "it happened" is not a truth: truth is what a predicate
produces from an observation, and nothing observes the past. A kind whose
standing is none and whose truth is none is a document, and giving the same
cell a second name would mean two kinds a reader has to choose between with no
question that separates them — which is exactly what
[piece C's breach question](../model.md) exists to prevent.

This is [0008](0008-work-is-not-an-entity.md)'s refusal one step over. Work
looked like a kind because it had standing and truth; the answer was that
nobody authors one. `record` looks like a kind because history feels different
from explanation; the answer is that the table has no free cell for it. A
decision is not that kind either — it obliges, so it is not a document — which
is why it is a kind of its own and why the reason had to be something other
than "it is a record".

### An ADR's number is not a point-in-time name

`AGENTS.md` bans a position in a plan from anything durable, and a number in a
filename looks like one. It is not: `0022` is an identity, minted once, never
reordered, and cross-referenced by every ADR that touches canonicalization. It
names no increment, no phase and no date. It is the same rule the slug follows
— authored once and never rewritten — and the numbering is what makes
`See 0022` a thing a person can say.

## Alternatives

**Make ADRs canon.** The tempting one, since the routes agree. Rejected on the
hash list: canon binds its whole body and a decision must bind one section of
it, and one kind gets one `_schema/{type}.yaml`. Making them canon would mean
either that adding a consequence de-ratifies an ADR, or that canon stops
binding its prose — and canon binding its prose is 0007's entire content.

**Make ADRs documents.** They have no gatherer and no cadence, which is the
document row apart from standing. Rejected: they oblige. A document gains force
only by being cited, and a decision binds whether or not anything points at
it — which is what standing means, and standing is what the document kind does
not have.

**Add a `record` kind.** Rejected above.

**Give the kind a `status` field** — `proposed`, `accepted`, `superseded`.
Rejected, and this is the same refusal `AGENTS.md` makes about caching standing
in a file. Accepted-ness is standing: an ADR the ledger has a ratification for
is accepted, and one whose file has moved past its ratification is not. A
`status: accepted` line would be a second place that answer lived, written by
hand, and the one anybody read would be the one that was wrong.
[0042](0042-doctor-adopts-a-corpus.md) removes the prose `**Status**` line for
this reason, and its relations move into `links:` where they are the corpus's
own edges rather than a sentence.

**Bind the whole body and accept that a consequence re-ratifies.** Rejected:
it makes recording what a decision turned out to cost an act requiring a
person's approval, so the honest thing to do — write down the consequence you
found — becomes the expensive thing. That is a bad gradient to build into a
record whose value is that it stays true.

## Consequences

- **Thirty-nine files become artifacts**, and every one of them can drift into
  `unratified` by being edited. That is the point and it is also a new way to
  be noisy: an ADR nobody meant to change now reports.
- **Editing an ADR's Decision section drops its standing; editing its Context
  does not.** Both have tests.
- **`kept status` grows from two rows to forty-one.** Whether the register's
  presentation holds up at that size is not answered here, and open question 7
  — whether v1 needs the derived index — is where the cost would be measured.
- **`links:` becomes load-bearing for this kind.** The relations that lived in
  a prose sentence become edges the corpus resolves, which means a superseding
  ADR that names a file that is not there is a broken link rather than a
  sentence nobody re-read.
- **The nouns table has no spare row left.** Every cell of standing × truth is
  occupied and named, so the next kind anyone proposes has to justify itself
  the way this one did.
