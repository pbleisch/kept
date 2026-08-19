# Decisions

One file per decision, in the MADR-ish shape adsr and awiki use: context,
decision, alternatives that were really considered, consequences including the
ones accepted as costs.

These record the *why*. `../model.md` records the *what*. When a decision
changes, supersede the file rather than editing it.

Each opens with one status line — `**Status** accepted · what it extends,
corrects or supersedes`. The clause after the separator is where a decision
records its lineage, and it is the part worth reading first: it says which
earlier decision this one moved.

| # | Decision |
|---|---|
| [0001](0001-standing-and-truth-live-in-the-ledger.md) | Standing and truth live in the ledger, keyed by a content hash |
| [0002](0002-artifacts-are-wiki-entities.md) | Artifacts are markdown entities in a git-backed corpus |
| [0003](0003-schema-declares-what-binds.md) | The schema declares which fields are binding |
| [0004](0004-gatherers-return-observations.md) | Gatherers return structured observations; predicates belong to the artifact |
| [0005](0005-gatherers-are-a-named-map.md) | A promise names gatherers as a map, not one gatherer |
| [0006](0006-no-probe-assess-the-thread.md) | No per-promise probe; assess the thread instead |
| [0007](0007-canon-is-ratified.md) | Canon is ratified, not observed |
| [0008](0008-work-is-not-an-entity.md) | Work is a ledger construct, not a corpus entity |
| [0009](0009-repairs-are-not-enumerated.md) | Repairs are not enumerated |
| [0010](0010-frontmatter-binds-body-informs.md) | Frontmatter binds, the body informs |
| [0011](0011-tracked-formats-must-merge.md) | Everything tracked must merge; instances are concurrent |
| [0012](0012-a-run-id-is-opaque.md) | A run is identified by minted bits, not by where it ran |
| [0013](0013-the-gate-makes-concurrency-safe.md) | Concurrent runs are made safe by the gate, not by exclusion |
| [0014](0014-an-observation-names-its-world.md) | An observation names the world it measured |
| [0015](0015-landing-is-a-merge-queue.md) | Landing is a merge queue over `origin/main` |
| [0016](0016-the-ledger-lives-on-its-own-ref.md) | The ledger lives on its own ref |
| [0017](0017-the-box-is-not-the-jail.md) | The box bounds what may land; confinement bounds what may be touched |
| [0018](0018-what-survives-a-crash.md) | Records are retrospective, so a crash loses work and never truth |
| [0019](0019-a-refusal-is-a-decision.md) | A refusal is a decision, and it is recorded |
| [0020](0020-durable-when-irreproducible.md) | A record is durable when its act cannot be reproduced |
| [0021](0021-identity-is-authored-absence-is-not-an-act.md) | Identity is a slug in frontmatter; absence is never an act |
| [0022](0022-canonical-content.md) | The hash normalizes content, and says which rules it used |
| [0023](0023-what-may-be-captured.md) | A gatherer returns what it declared; raw output is a bounded receipt |
| [0024](0024-signing-is-designed-and-deferred.md) | Ratification signing is designed now and implemented later |
| [0025](0025-no-scheduler-cadence-is-change-driven.md) | kept has no scheduler, and a cadence is change-driven |
| [0026](0026-nothing-external-is-unbounded.md) | No external invocation is unbounded |
| [0027](0027-the-machine-facing-surface.md) | The machine-facing surface is a contract, and kept explains itself |
| [0028](0028-the-corpus-is-not-the-repository.md) | The corpus is not the repository; source code is the world |
| [0029](0029-dependencies-pass-a-test.md) | Dependencies pass a standing test; only the trust path gets an ADR |
| [0030](0030-a-link-manifest-is-one-file-per-subject.md) | A reference is an artifact, a workspace file or a URI; a manifest is one file per subject, and its edges do not bind |
| [0031](0031-the-binding-preimage.md) | The binding hash is over a canonical preimage |
| [0032](0032-standing-is-a-fold-over-decisions.md) | Standing is a fold over decisions, and a release retires the artifact |
| [0033](0033-due-is-a-digest-over-the-scope.md) | A cadence is due when the bytes in its scope change, and kept computes that itself |
| [0034](0034-the-observation-record.md) | What an observation is addressed by, and what it merely records |
| [0035](0035-a-verdict-names-its-world-in-the-record.md) | A verdict names its world in the record, and a disposition names it without being keyed by it |
| [0036](0036-the-predicate-language.md) | The predicate language is hand-written, three-valued, and does the extractor's job too |
