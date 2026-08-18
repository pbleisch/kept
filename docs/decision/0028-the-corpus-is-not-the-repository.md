# 0028 — The corpus is not the repository, and source code is the world

**Status** proposed · answers a question [0002](0002-artifacts-are-wiki-entities.md)
left open by defining what an artifact *is* and never what everything else is

## Context

0002 settles that an artifact is one markdown file with YAML frontmatter at
`{type}/{slug}.md`. `layout.md` shows a corpus root set in config — `docs/` for
a repository, the repository root for a manuscript — and notes that only type
directories are walked, so a file sitting at the corpus root is not an entity.

What nothing says is what the rest of the repository is. The question arrives
the moment kept self-hosts: the corpus is `docs/`, the code is `internal/`, and
a promise in one is about the other. Is `internal/audit/audit.go` an artifact?
Can it have standing? Can it have truth?

It is worth answering explicitly because the wrong answer is attractive. A repo
where every file could be ratified sounds more uniform, and "this file is
approved" is a thing people genuinely want to say.

## Decision

**An artifact is a corpus entity. Everything else in the repository is the
world — the thing gatherers observe.**

| | is | may have |
|---|---|---|
| `{type}/{slug}.md` under the corpus root | an artifact | content, standing, truth |
| a loose file at the corpus root | not an entity | content |
| an attachment declared in frontmatter | not an entity | content |
| everything else — source, config, fixtures | the world | content |

**Source code has content and only content.** Not as a restriction, but because
the other two questions do not apply to it. Standing answers *may this oblige
anyone?* and truth answers *is this so?* — both are asked of a **statement**,
and a source file does not make one in that sense. It is evidence, and evidence
is what the model is careful never to confuse with a commitment.

### Why it must stay that way

**A writer would be able to mint its own authority.** The repair agent writes
source. If source could carry standing, an agent could grant standing by
writing a file — and 0001 exists precisely to make that impossible. This
reason is decisive on its own and the other two are supporting.

**There would be nothing to key the hash on.** Standing attaches to a content
version, computed over the fields a schema names as binding
([0003](0003-schema-declares-what-binds.md)). Source has no schema and no
declared fields, so the hash would be over the whole file and every whitespace
change would drop standing. Ratification would become something you do
constantly, and a ratification you perform constantly is not a deliberate act.

**It would duplicate what the model already does better.** *This file must not
change without me* is a promise: a predicate over a `path` or `fold` gatherer,
authored as an artifact and ratified once. The commitment is written down and
the file is observed — which is the separation everything else rests on.

### What to do instead: cite it

[0007](0007-canon-is-ratified.md) already supplies the mechanism. Something
without standing acquires force by being cited by something that has it. A lint
configuration, a schema, a golden file, a generated interface — name it in a
promise's scope, or cite it from a policy, and it is bound by the artifact that
points at it rather than by a property of its own.

## Alternatives

**Every tracked file is an artifact.** Rejected on all three grounds above, the
first of them fatal.

**A parallel sidecar per source file** — `internal/audit/audit.go.kept.yaml` —
carrying frontmatter for a file that cannot hold it. Rejected: it is a promise
with an implicit statement and a hidden scope of one file, so it is strictly
worse than writing the promise, and it puts a corpus entity inside the tree the
agent may write.

**Let standing attach to a directory or a path glob.** Tempting, because a
scope is already a glob. Rejected: it inverts the model. A scope says *which
part of the world this statement is about*; standing on a glob would be a
statement with no text, which nobody can review, revise, or disagree with.

## Consequences

- **The boundary is configuration**, so it can be set wrong, and `kept doctor`
  checks it: files with artifact-shaped frontmatter outside a type directory,
  type directories outside the corpus root, and a corpus root that resolves to
  nothing. `layout.md` already records what a mis-set root cost grft.
- **Becoming an artifact is placement plus frontmatter**, not file type. A
  design note becomes a `document` by moving into a type directory and gaining
  frontmatter — nothing converts, and nothing is inferred from the extension.
- **`.kept/config.yaml` is world, not corpus** — and privileged world, since it
  decides what is enforced. That is why `.kept/**` is categorically out of scope
  for every repair ([0016](0016-the-ledger-lives-on-its-own-ref.md)) and not
  only because the ledger lives there.
- **A manuscript blurs the line and does not break it.** There the corpus root
  is the repository root, so nearly everything is an entity — but the rule is
  unchanged, because what makes a chapter an artifact is that it sits in a type
  directory with frontmatter, not that it is prose.
