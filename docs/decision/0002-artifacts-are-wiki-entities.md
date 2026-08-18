# 0002 — Artifacts are markdown entities in a git-backed corpus

**Status:** Accepted · the slug and rename bullets below are superseded by
[0021](0021-identity-is-authored-absence-is-not-an-act.md), which makes the slug
authored in frontmatter rather than derived from the title, so a rename
propagates nothing because the filename is not the identity · the link bullet is
widened by [0030](0030-a-link-manifest-is-one-file-per-subject.md), where a
reference at either end of a link is an artifact, a workspace file or a URI
rather than only `type:slug`, and a manifest speaks for any of them rather than
only for binaries · what an artifact is *not* is
[0028](0028-the-corpus-is-not-the-repository.md)

## Context

Artifacts need structured fields (predicate, gatherer, scope) and prose (what
this is for). They also need to link to each other, carry attachments, and be
editable without the tool running.

## Decision

Adopt the awiki / chopworks content model wholesale:

- one markdown file with YAML frontmatter per artifact, at `{type}/{slug}.md`,
  id `type:slug`, slug derived from the title by a deterministic algorithm
- `_schema/{type}.yaml` per type, **lenient** — required fields enforced, extra
  fields allowed
- three link sources deduplicated by priority: frontmatter `links:`, inline
  `[[type:slug|Display]]`, and `_links/` manifests for binaries
- attachments as sibling files declared in frontmatter, with a media whitelist
  and traversal protection
- a derived SQLite + FTS5 index, rebuildable from the files
- one internal write facade: validate → write → index → commit
- rename propagation on title change: file, attachments, inbound references

## Alternatives

**A flat directory of JSON declarations**, as in grft. Adequate for promises
about a repository; hopeless for a novel corpus, where the content *is* the
graph.

**A database as source of truth.** Rejected on the same grounds awiki rejects
it: content must survive the application.

## Consequences

- Citation is a typed link, so "a document gains force when something ratified
  cites it" is a mechanism rather than a convention.
- Broken-link detection catches a promise citing canon that was deleted, in the
  same scan that catches a chapter referencing a missing character.
- The facade is an implementation detail, not an endpoint. It exists because
  the tick, a thread, and the operator all write, and an unmediated write leaves
  the index disagreeing with git.
