# 0021 — Identity is a slug in frontmatter, and absence is never an act

**Status** accepted · settles what `model.Artifact.ID` means, which
[0002](0002-artifacts-are-wiki-entities.md) left to the filename · supersedes 0002's
slug-derived-from-title and rename-propagation bullets

## Context

Every durable stream is keyed by the artifact's id, and two of them are keyed
by it as a *path*: `truth/promise/{slug}.jsonl` and, since
[0019](0019-a-refusal-is-a-decision.md), `disposition/promise/{slug}.jsonl`.
`model.Artifact` derives its id from the title, which derives the filename. So
identity is currently the filename, and that fails twice.

**A rename orphans everything.** Retitle a promise and its standing, its truth
series and its dispositions are left behind under the old key. It reappears as
unratified and unknown — indistinguishable from a promise nobody has ever
written.

**A deletion is silent.** Drift is a query over artifacts that exist, so a
ratified promise whose file is gone produces no drift, no verdict and no
record. Deleting the file is the cheapest possible way to make a promise stop
being a promise, and nothing anywhere says it happened.

## Decision

### The slug is authored, lives in frontmatter, and does not change

```yaml
id: core-packages-are-tested
```

Minted by the CLI when the artifact is created — which is also where
uniqueness is enforced — and never rewritten afterwards. It is a slug rather
than an opaque id because these are read by people in diffs, terminals and
prose, and 0012's argument against composed identifiers does not apply: nothing
is encoded in it, it names one authored thing, and it is not allocated
concurrently.

**The filename stops being identity.** The corpus walk reads the id from
content, so **a file rename changes nothing** — there is no rename to detect,
no `kept mv` to run, and no history to migrate. Path and id may diverge;
`kept doctor` reports it and offers to rename the *file*, never the id.

The id is not in the schema's `hash` list. It says which promise this is, not
what is promised — a distinction 0003 already draws.

Two failures become load errors rather than mysteries: **two artifacts
claiming one id** is refused with both paths named, and **an id edited by
hand** presents as one artifact gone absent and one new artifact appearing,
which is what it is.

### Absence is a drift kind, not a release

An artifact with standing whose content cannot be found is **absent**. It keeps
its standing, it appears in `kept`, and it reports that its content is missing.
It is never treated as released.

```
DriftAbsent   ratified, and the content it was ratified against is gone
```

Absence cannot be distinguished from a bad merge, a sparse checkout, a moved
corpus root, or a misconfigured `corpus:` path. `layout.md` already carries the
scar of the last one — *a working root sitting inside a gitignored path once
made grft's containment checks silently see nothing at all*. If absence
released, one misconfiguration would silently release the whole corpus.

Retiring a promise therefore requires `kept release`, which is a human act and
was always meant to be the only route. Deletion stops being a back door.

**A bulk-absence guard.** If more than a configured fraction of artifacts with
standing go absent in one tick, that is a configuration failure and not many
deletions: the tick refuses and says so, rather than emitting a drift record
for each.

## Alternatives

**A minted opaque id in frontmatter.** Rejected: 0002 makes these
human-authored wiki entities, and a uuid in every file taxes every reader to
solve a problem a unique slug already solves.

**Detect renames and deletes from git history.** Rejected: it works only for
changes made through git, and it answers the wrong question — with identity in
content there is nothing to detect.

**Treat absence as release after a grace period.** Rejected: a delay does not
make the inference sound. The misconfiguration is still indistinguishable, and
now it destroys standing later, when the cause is harder to find.

## Consequences

- **An artifact created and deleted between two runs leaves nothing**, because
  nothing ever depended on it. Standing is what makes absence matter; without
  it there is no record to contradict and no promise to be unenforced.
- **`kept doctor` becomes a real verb**, and its first checks are here:
  path/id divergence, duplicate ids, standing without content, and ledger
  records naming ids no artifact claims.
- **Renaming a file is free and unremarkable**, which is the point — the corpus
  is meant to be edited like ordinary prose.
