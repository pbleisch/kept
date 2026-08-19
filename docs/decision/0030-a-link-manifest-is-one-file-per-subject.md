---
id: 0030-a-link-manifest-is-one-file-per-subject
type: decision
title: 0030 — A reference is an artifact, a workspace file, or a URI, and a manifest is one file per subject
---

# 0030 — A reference is an artifact, a workspace file, or a URI, and a manifest is one file per subject

**Status** accepted · widens [0002](0002-artifacts-are-wiki-entities.md)'s link
target from `type:slug` to three kinds, in all three of its sources, and fixes
the shape 0002 introduced as "`_links/` manifests for binaries" and
[0011](0011-tracked-formats-must-merge.md) rule 6 preferred without describing ·
extends [0021](0021-identity-is-authored-absence-is-not-an-act.md)'s rule about
filenames to a file that is not an artifact · the question this left to
increment 2 is answered below, where a manifest edge does not bind, and the
preimage that implements it is [0031](0031-the-binding-preimage.md)

## Context

A link has two ends, and 0002 fixes both of them as artifacts: a target is
`type:slug`, and the *linking* end is whatever authored the list. That leaves
out most of what anyone actually wants to point at, and it leaves out the person
doing the pointing.

**Neither end is always an artifact.** A promise is about source, so it wants to
name `internal/corpus/corpus.go`. A canon entry rests on a specification nobody
here wrote, so it wants to name a URL. A cover image is a cover image, so
something has to be able to say what it depicts. Only one of those three is a
corpus entity, and 0028 already says why: an artifact has content, standing and
truth, and everything else in the repository is the world, which has content and
only content. That does not stop it being worth citing —
[0007](0007-canon-is-ratified.md) makes citation exactly how something without
standing acquires force.

**The author of an end is not always the person asserting the link.** Two
subjects cannot speak for themselves at all: a binary has no frontmatter to put
a list in and never will. But an artifact that *can* speak for itself still
needs somewhere else for a link to live when the person asserting it is not its
author — a reviewer, a downstream corpus, an agent proposing an edge. Requiring
them to edit the artifact means requiring a write to the very file whose content
a ratification is keyed to (0001). That is the general form of the binary's
problem, and it is the stronger reason for the directory: **a third party must
be able to assert a link about an artifact without editing that artifact.**

0011's rule 6 arrives at the same directory from a third direction — *where a
list in frontmatter would conflict spuriously, prefer a `_links/` sidecar* —
and says it about artifacts, not binaries. So `_links/` is named twice, for
different reasons, and neither mention says what a file in it looks like.
Increment 1 had to read one, so a shape got chosen; this records it, and shows
that 0011 forces it rather than merely tolerating it.

## Decision

**A reference is an artifact, a file in the workspace, or a URI. Both ends of a
link take any of the three, in every source a link can be written in.**

| kind | written | checked by |
|---|---|---|
| artifact | `type:slug` | the corpus claims it, or it does not |
| workspace file | a path under the workspace root | it is on disk, or it is not |
| URI | anything with a scheme | it parses, and nothing more |

**A manifest is one file per subject, declaring its subject and carrying the
same `links:` list an artifact writes.**

```yaml
# docs/_links/cover.yaml
subject: canon/cover.png
links:
  - { target: canon:kael-eyes-are-green, relation: about }
  - { target: "https://example.com/photo-credit", relation: cites }
```

### Telling the three apart

`type:slug` and `scheme:rest` are the same shape. `promise:tests-run` and
`mailto:paul@example.com` cannot be told apart by looking at them, so something
has to decide, and the only vocabulary available is the corpus's own.

Three rules, in order, each ruling out what follows it:

1. **No colon, or a slash before the first colon, and it is a path.** A type
   name cannot contain a slash and neither can a scheme, so `docs/model.md`,
   `Makefile` and `docs/notes:draft.md` are all paths and nothing else.
2. **A colon followed by `//` and it is a URI.** A slug cannot contain a slash
   either (0021), so no artifact reference can take this shape. This is the
   escape hatch, and it matters more than it looks — see below.
3. **Otherwise the corpus's own vocabulary decides.** A type the corpus has
   makes it an artifact reference; a syntactically valid scheme makes it a URI;
   anything else is a path with a colon in its name.

"The corpus has" is deliberately wider than "a schema declares": a type
directory holding artifacts counts even with no `_schema/{type}.yaml`, because a
reference to one of those artifacts must not be read as a URI on the strength of
a missing file.

**The cost, stated plainly: what a reference means depends on what types exist.**
Add a type named `spec` and every `spec:rfc-1234` in the corpus stops being a URI
and becomes an artifact reference. This is the interwiki-prefix problem, every
wiki that has one accepts the same trade, and it is accepted here because the
failure is loud — the reinterpreted reference resolves to no artifact and is
reported broken — and because rule 2 is an escape no vocabulary can take away.
The silent case needs an artifact of the new type to hold exactly that slug,
which is a collision the author would probably have wanted.

**An artifact reference names its type.** `[[held]]` is a path, not an
unqualified artifact. The bare-slug convenience survives only where a person is
typing at a terminal and can read the error — `kept show held` — because a
tracked reference whose meaning depends on what else happens to exist is one
nobody can review, which is the same objection 0021 raises to identity by
filename.

### Which root, and what is refused

**A file reference is relative to the workspace root — the directory holding
`.kept` — and not to the corpus root.** 0028 keeps those apart, and this is the
side of the line the references are on: the interesting file to name is source,
config or a fixture, all of which are the world, and a promise's `scope` is
already a glob in that same frame. For a manuscript the two roots are one
directory and nothing changes.

A leading slash means the workspace root rather than the filesystem root, so
`/Makefile` and `Makefile` are one reference. That leaves `..` as the only way
out and it is refused, exactly as 0021 refuses a slug that is not a path
component, for the same reason: **a reference that can name anything on the
machine is one nobody can review.**

### What "broken" means, and where this stops

The three kinds are checked by three different questions, and they are
deliberately not the same question.

- **An artifact** is claimed by the corpus or it is not. Nothing outside the
  corpus is consulted.
- **A file** is on disk under the workspace root or it is not. That is a stat in
  the tree the corpus walk already read, on no deadline, with no way to answer
  "nobody knows".
- **A URI is checked for shape and never for reach.** A malformed one is
  reported. An unreachable one is not, and must not be.

The last is the one worth arguing, because fetching would be easy and it would
be wrong. Whether a URL answers is a fact about the world at a moment, which
makes it an observation and not a property of the text. It needs a deadline
([0026](0026-nothing-external-is-unbounded.md)). It has to be able to say
`unknown` when the network is down rather than `not-kept`
([0004](0004-gatherers-return-observations.md)) — otherwise a laptop on a train
reports a corpus full of broken links. Its answer has to name the world it
measured ([0014](0014-an-observation-names-its-world.md)) so it can be cached and
compared. All three of those are the shape of a gatherer, and a link checker
that reached the network would be a gatherer wearing a linter's clothes,
delivering truth through a code path with none of truth's guarantees.

So: **reachability is truth, it belongs to a gatherer, and reading the corpus
never asks for it.** A `uri` gatherer named by a promise is how that question
gets asked, and that is increment 3 or later.

### One file per subject, because 0011 leaves nothing else

The rule set in 0011 is not one rule applied evenly. Rules 2 through 5 bound the
**machine-written** tier, where two runs of `kept` must never choose one path
because no person is present to resolve them. Rule 1 bounds the **authored**
tier, and it says something different: one file per artifact, and *two editing
the same artifact is a real conflict and should surface as one*. A manifest is
corpus, not ledger — content about content — so both halves apply, and between
them they leave one shape:

- **Rule 5 — no shared mutable file.** A single `_links/manifest.yaml` would be
  read-modify-written every time anyone linked anything. That is the one thing
  rule 5 names outright, and it is not a close call.
- **Rule 4 — every path keyed by something already unique.** The subject is
  already unique: it is an artifact's authored id, or a path inside the corpus.
  Nothing has to be minted, composed or counted to key on it, which is what
  rule 4 asks for and what [0012](0012-a-run-id-is-opaque.md) warns against
  getting wrong.
- **Rule 1 — one file per authored thing, and a real conflict surfaces.** The
  subject is what a manifest is *about*, so the subject is the unit two writers
  can genuinely disagree over. A key finer than the unit of authorship would
  buy collision-freedom for a writer that does not exist: per
  [0029](0029-dependencies-pass-a-test.md)'s write surface, **kept writes no
  manifest.** A person writes one, or a repair agent writes one inside a box and
  it merges through the queue like any other content.

### What rule 6 actually buys, and what it does not

Rule 6 is worth reading precisely, because the obvious reading is wrong. A
sidecar does not stop two writers who both add a link to one subject from
colliding — they still write one file, and they should, because they disagree
about one thing.

What it changes is the **blast radius**. A conflict inside an artifact's
frontmatter is not a conflict about links: conflict markers in that block make
the whole document unparseable, so the promise loses its `id`, its `statement`
and its predicate until someone resolves it, and until then it is an artifact
that has gone absent. Move the list to a sidecar and the same disagreement costs
the links and nothing else. That is the spurious part rule 6 objects to, and one
file per subject delivers it.

### Named by its subject, and named is not identified

The `subject:` key is what binds a manifest to what it speaks for. The filename
is a convenience: when it is absent, the file's basename is read as the subject,
so a manifest sitting beside a file of the same name does not have to repeat it.

This is 0021's rule, applied to a file that is not an artifact. `{type}/{slug}.md`
is convention and `id:` is identity; `_links/{subject}.yaml` is convention and
`subject:` is identity. **Renaming a manifest that declares its subject changes
nothing**, which is the property the corpus is supposed to have everywhere.

Renaming what a reference *names* splits by kind, and the asymmetry is worth
stating rather than discovering. It applies to every reference, not only a
manifest's subject:

| the reference names | by | renaming the thing |
|---|---|---|
| an artifact | `type:slug` | changes nothing — 0021 already bought this |
| a workspace file | its path | **breaks the reference** |
| a URI | itself | is not ours to rename |

There is no cure for the middle row and it is not an oversight.
[0028](0028-the-corpus-is-not-the-repository.md) settles that only corpus
entities have identity and everything else is the world, addressed by where it
is. A source file has no authored id to name, so a path is the only handle there
is — and giving it one would mean putting corpus frontmatter into the tree the
repair agent writes, which 0028 rejects outright.

What is owed is that the break is loud. A file reference that names nothing is
reported as broken, and a manifest whose *subject* names nothing is reported
separately from one of its targets going missing, because a manifest with a
missing subject asserts nothing at all while one with a missing target still
asserts the rest. Neither resolves to an empty link set that quietly removes
edges nobody asked to remove: the reasoning is 0021's, that an absence which
cannot be told apart from a misconfiguration must not be read as an act.

### A manifest edge never binds

The first draft of this decision deferred this and said increment 2 should reach
it deliberately rather than inherit it from whichever code path was written
first. This is that answer, and it is here rather than with the hashing because
it is a rule about what a manifest may do, not about how bytes are hashed.

**A `hash` list's `links.<relation>` covers the edges an artifact's own author
wrote — its frontmatter and its prose — and never the edges a manifest asserts.**

The reason is the sidecar's whole purpose turned around. A manifest exists so a
third party can say something about an artifact *without editing it*. If such an
edge counted toward the artifact's binding hash, then writing a file next to a
promise would change what its ratification was granted against — silently
de-ratifying somebody else's commitment without touching a byte of it. That is
standing being altered from outside the one act reserved to a person, by a
writer the model never granted that power, and
[0001](0001-standing-and-truth-live-in-the-ledger.md)'s whole point is that
standing must live somewhere such a writer cannot reach.

It is not a hypothetical writer either. The repair agent may write the corpus,
and a manifest is corpus — `.kept/**` being out of scope does not cover it. A
binding sidecar would hand the one unbidden writer a way to edit the record of
what it is allowed to do.

So **binding is the one thing the three sources do not share**, exactly as this
decision predicted when it deferred the question. A reference still does not
change meaning by moving between sources; what changes is whether the artifact's
own author is the one asserting it, and only the author's assertions bind.

## Alternatives

### For whether a manifest edge binds

**Let a manifest edge bind, like any other.** Rejected above. It is the reading
someone arrives at by treating the three sources as interchangeable, which they
are for every purpose except this one, and it costs the property that a
ratification is keyed to what its author wrote.

**Refuse to hash an artifact at all when a manifest speaks for it.** Considered,
and rejected as over-correction. A manifest saying something about a ratified
artifact is useful and harmless — that is the feature — and it is only *binding*
that has to be the author's. Refusing would make a third party able to block a
ratification instead of able to change one, which is a smaller hole in the same
wall.

**Let a schema opt in, with a `links.<relation>!` or similar.** Rejected: it
makes the guarantee a per-corpus configuration, so whether a third party can
reach your standing depends on a schema field somebody set once. A rule that can
be switched off is not a rule an operator can rely on without reading every
schema.

### For telling the three kinds apart

**An explicit prefix on every reference** — `artifact:promise:tests-run`,
`file:docs/model.md`, `uri:https://…`. Unambiguous, and it needs no vocabulary,
so nothing anyone adds later can reinterpret an existing string. Rejected on
what it costs the common case: artifact-to-artifact links are most of the graph
and they would all grow a prefix, `[[type:slug]]` would stop being 0002's
syntax, and every existing reference would have to be rewritten. It also picks
the worst possible prefix name — `file:` is itself a registered URI scheme, so
`file:docs/model.md` would read as a file URL to everyone who has seen one and
mean something else here.

**Decide by what exists**: try it as an artifact, then as a file, then call it a
URI. Rejected, and firmly. It makes a reference's meaning depend on the state of
the corpus rather than on its text, so creating a file changes what an unrelated
link means, and a broken link and a link of a different kind become
indistinguishable. It is the same mistake 0021 removed from identity, arriving
by a different door.

**Require a URI to have an authority** — treat every `word:rest` without `//` as
an artifact reference. Rejected: it is very nearly the rule adopted, but it
breaks `mailto:`, `urn:` and every other scheme with no authority, and those are
exactly the citations a corpus of prose wants.

### For the manifest's shape

**One manifest for the whole corpus.** Rejected by 0011 rule 5 directly: it is a
shared mutable file, read-modify-written on every link anyone adds. It also
inverts the blast-radius argument that justifies sidecars at all — one
unresolved conflict would take the link graph out for every subject at once,
which is worse than the frontmatter conflict it was meant to avoid.

**One file per (subject, target) pair.** This satisfies rule 4 more completely
than the decision does: two writers adding *different* edges to one subject
would never choose one path. Rejected on what it costs. The filename would have
to encode both ends, and 0012's argument applies with full force — an identifier
that encodes something becomes parseable, then depended on, then unfixable.
`_links/` would hold one small file per edge, so reading what a subject points
at would mean scanning a directory rather than opening a file, and the corpus
would stop being editable without the tool running, which is the property 0002
rejected a database to keep. It buys collision-freedom for concurrent manifest
writers, and there are none: kept writes no manifest.

**An append-only JSONL sidecar under `merge=union`**, the way the ledger stores
streams. Rejected: union merge is safe in the ledger *only* because every
derived read folds by event time, then id. A link list has no event time and
nothing to fold. It would also make removing a link impossible without a
tombstone, so retracting a citation would need the machinery
[0001](0001-standing-and-truth-live-in-the-ledger.md) built for decisions —
which is right for a decision and absurd for saying a picture is of a character.
The corpus is meant to be read and edited by a person; JSONL is the format of
the tier that is not.

**Keep every link in frontmatter and accept the conflicts.** This is the status
quo for artifacts and it is what 0011 rule 6 already declined. It also leaves the
binary with no way to link at all, which is the case 0002 raised the directory
for.

## What this does not decide

Four things a later increment should not read into this. A fifth — whether a
manifest edge binds — was on this list and is now decided above, in the
increment it was deferred to.

- **It does not change link priority.** Frontmatter first, then the body, then a
  manifest — 0002's order, unchanged. A sidecar is where a link lives when
  nothing else can hold it, not a better place to put one. Priority is about
  which source *wins* for a target named twice; binding is about which sources
  count at all, and the two are separate.
- **It does not give a workspace file standing or truth.** It is still the
  world (0028). Being pointed at makes it cited, which is force borrowed from
  the artifact doing the pointing (0007), and nothing more.
- **It does not make a manifest an entity.** It sits under an
  underscore-prefixed directory, which the walk never reads as a type
  directory, so 0028 already says what it is: not an artifact, and it has
  content and only content. It cannot be ratified and nothing about it has
  standing.
- **It does not decide attachments.** 0002's *attachments as sibling files
  declared in frontmatter* is a separate mechanism with a separate media
  whitelist, and a manifest is not how an attachment is declared.

## Consequences

- **A third party can assert a link about an artifact without editing it**,
  which is the whole reason a manifest subject may be an artifact and not only a
  binary. The artifact's own bytes are untouched, so its binding hash — and
  therefore its standing — is untouched with them. That holds twice over now
  that a manifest edge does not bind: not only are the bytes unchanged, the edge
  itself never reaches the hash, so the third party cannot reach standing by
  either route.
- **The three sources are one feature set, with one exception.** A reference
  does not change meaning by moving between them, and its kind is recorded
  rather than re-derived, so nothing downstream guesses or guesses differently
  from the loader. Binding is the exception, and it is the only one: an edge
  binds when the artifact's own author wrote it and not when a manifest asserts
  it. That asymmetry is deliberate and is the subject of its own section above.
- **The sidecar reuses the artifact's own list shape**, so moving a list out of
  frontmatter is a move rather than a translation, and one reader parses both.
- **A conflict in a manifest is about links and never about a predicate.** That
  is the whole of what rule 6 was reaching for and the reason to prefer the
  directory at all.
- **Two writers adding links to one subject still conflict**, and that is
  correct rather than tolerated. They disagree about one thing, and 0011 rule 1
  says a real conflict should surface as one.
- **A manifest cannot de-ratify anything, and that is worth a test rather than
  a reading of the code.** The rule is one line in the hash and it is the line
  standing back-pressure would push on first, so `AGENTS.md`'s bar applies: a
  test that adds a manifest edge to a ratified artifact and requires the hash
  not to move.
- **`kept doctor` gains checks here**, alongside the ones 0021 gave it: a
  manifest whose subject nothing claims, a manifest that declares no `subject:`
  and so depends on its own filename, a reference that resolves outside the
  workspace root, and — the one this decision creates — a type whose name is
  also used as a URI scheme somewhere in the corpus, which is the collision
  rule 3 resolves silently.
- **An escaping path is refused without being opened.** `../../etc/passwd`
  fails the containment check before anything reads it, which is the traversal
  protection 0002 asked for. The check is lexical, so a reference naming a
  symlink that points outside the root is still followed — an accepted cost,
  because the corpus is the operator's own tree and the repair agent that could
  plant such a link is confined by
  [0017](0017-the-box-is-not-the-jail.md) rather than by this.
- **Reading a corpus still touches nothing but the corpus and the workspace.**
  No name is resolved, no host is contacted, and no deadline is needed, which is
  what keeps `kept list` an operation that cannot fail because of where it was
  run.
