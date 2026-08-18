# 0030 — A link manifest is one file per subject, and its filename is not identity

**Status** proposed · fixes the shape [0002](0002-artifacts-are-wiki-entities.md)
introduced as "`_links/` manifests for binaries" and
[0011](0011-tracked-formats-must-merge.md) rule 6 preferred without describing ·
extends [0021](0021-identity-is-authored-absence-is-not-an-act.md)'s rule about
filenames to a file that is not an artifact

## Context

A link has two ends, and the two link sources that already work require the
*linking* end to be an artifact. Frontmatter `links:` and an inline
`[[type:slug]]` are both things a thing says about itself, which is the split
[0010](0010-frontmatter-binds-body-informs.md) draws. Two subjects cannot say
anything about themselves at all:

- **A binary.** A cover image, a PDF, a golden file. It has no frontmatter to
  put a list in and never will, and [0007](0007-canon-is-ratified.md) makes
  citation the mechanism by which something without standing acquires force —
  so a file nothing can link *from* is a file nothing can bring into the graph
  except by naming it from the other end, which inverts who is asserting what.
- **An artifact whose frontmatter list is a bad place for the list.** 0011's
  rule 6 says exactly this and says it about artifacts, not binaries: *where a
  list in frontmatter would conflict spuriously, prefer a `_links/` sidecar*.

So `_links/` is named twice, for two different subjects, and neither mention
says what a file in it looks like. Increment 1 had to read them, so a shape got
chosen; this records it, and shows that 0011 forces it rather than merely
tolerating it.

## Decision

**A manifest is one file per subject, declaring its subject and carrying the
same `links:` list an artifact writes.**

```yaml
# docs/_links/cover.yaml
subject: canon/cover.png
links:
  - { target: canon:kael-eyes-are-green, relation: about }
```

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

Renaming the *subject* splits by what the subject is, and the asymmetry is worth
stating rather than discovering:

| the subject is | named by | renaming its file |
|---|---|---|
| an artifact | `type:slug` | changes nothing — 0021 already bought this |
| a file in the corpus | its path | **breaks the manifest** |

There is no cure for the second row and it is not an oversight.
[0028](0028-the-corpus-is-not-the-repository.md) settles that only corpus
entities have identity and everything else is the world, addressed by where it
is. A binary has no authored id to name, so a path is the only handle there is.
What is owed is that the break is loud: a manifest naming a subject that is
neither an artifact nor a file inside the corpus is reported against the
manifest, rather than resolving to an empty link set and quietly removing edges
nobody asked to remove. The reasoning is 0021's — an absence that cannot be told
apart from a misconfiguration must not be read as an act.

## Alternatives

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

Four things a later increment should not read into this.

- **It does not change link priority.** Frontmatter first, then the body, then a
  manifest — 0002's order, unchanged. A sidecar is where a link lives when
  nothing else can hold it, not a better place to put one.
- **It does not decide whether a manifest link binds.**
  [0003](0003-schema-declares-what-binds.md) reserves `links.<relation>` as a
  name a `hash` list may use. Whether an edge that arrived from a sidecar counts
  toward an artifact's binding hash the way a frontmatter edge does is open, and
  it is not a small question: if it does, something other than the artifact's
  author can change what a ratification was granted against by writing a file
  next to it. Increment 2 should answer that deliberately rather than inherit an
  answer from whichever code path was written first.
- **It does not make a manifest an entity.** It sits under an
  underscore-prefixed directory, which the walk never reads as a type
  directory, so 0028 already says what it is: not an artifact, and it has
  content and only content. It cannot be ratified and nothing about it has
  standing.
- **It does not decide attachments.** 0002's *attachments as sibling files
  declared in frontmatter* is a separate mechanism with a separate media
  whitelist, and a manifest is not how an attachment is declared.

## Consequences

- **The sidecar reuses the artifact's own list shape**, so moving a list out of
  frontmatter is a move rather than a translation, and one reader parses both.
- **A conflict in a manifest is about links and never about a predicate.** That
  is the whole of what rule 6 was reaching for and the reason to prefer the
  directory at all.
- **Two writers adding links to one subject still conflict**, and that is
  correct rather than tolerated. They disagree about one thing, and 0011 rule 1
  says a real conflict should surface as one.
- **`kept doctor` gains checks here**, alongside the ones 0021 gave it: a
  manifest whose subject nothing claims, a manifest that declares no `subject:`
  and so depends on its own filename, and a subject that resolves outside the
  corpus root.
- **A subject path is refused without being opened.** `../../etc/passwd` fails
  the containment check before anything reads it, which is the traversal
  protection 0002 asked for and the only place in the corpus walk that needs it.
  The check is lexical, so a subject naming a symlink that points outside the
  root is still followed — an accepted cost, because the corpus is the
  operator's own tree and the repair agent that could plant such a link is
  confined by [0017](0017-the-box-is-not-the-jail.md) rather than by this.
