# Self-hosting

The goal: `kept` holds promises about `kept`, and a drift in one of them is
repaired by an agent `kept` dispatched.

The trap to avoid is the one grft's evaluation records — a self-host pass where
every repair is constructed to succeed proves the mechanisms work and nothing
about whether they are useful. Every promise below is about this repository as
it actually is, and the first ones are read-only.

## What moves into the corpus, and what does not

The corpus is not "where documentation lives". It is where **artifacts** live.
So the question for each file is whether it has standing, truth, or neither.

| Today | Becomes | Why |
|---|---|---|
| `AGENTS.md` invariants | **policy** artifacts, one each | They are ratified rules about conduct. That is a policy in the model's sense, and several are checkable |
| `docs/decision/*.md` | **decision** artifacts | Records. Truth by construction — it happened. Cited by the policies they justify |
| `docs/model.md`, `lineage.md` | **document** artifacts | No standing of their own; force only when something ratified cites them |
| `docs/increments.md` items | **desire** artifacts | Wants with no standing. Ratified into promises as you commit to each |
| `docs/open-questions.md` | stays a plain file | Questions, not wants. Nothing obliges and nothing observes |
| `README.md` | stays a plain file | The front door. GitHub renders it; the corpus would not improve it |

**Layout.** `docs/` becomes the corpus root, so the corpus and the docs are one
tree and GitHub still renders it:

```
docs/_schema/{type}.yaml       one per kind, declaring what binds
docs/policy/{slug}.md          ratified rules about conduct
docs/decision/{slug}.md        the ADRs, as records
docs/document/{slug}.md        model, lineage
docs/desire/{slug}.md          uncommitted wants
docs/promise/{slug}.md         commitments about this repository
.kept/ledger/                  standing, truth, observations — tracked
```

The full layout, and why the ledger is tracked while the index is not, is in
[`layout.md`](layout.md).

The ledger is **not** gitignored. Ratifications and verdicts are decisions and
observations about this repository, and losing them on a fresh clone would mean
every promise reverts to unratified. Only the derived index and any cache are
ignored. Check this deliberately: grft was blinded once because a working root
sat inside a gitignored path and its containment checks silently saw nothing.

## The payoff arrives before anything is checkable

A policy with no gatherer still binds, and still goes into the packet. So
moving `AGENTS.md` into the corpus pays off at increment 2, before any promise
exists: the invariants stop being a markdown file an agent may or may not have
read, and start being ratified authority assembled into every packet.

That is the same shape as canon — most of it has standing and no truth-seeking.

## The promises

Written against `AGENTS.md`'s invariants, in the order they become checkable.

### With `command` only — available at increment 3

| Promise | Gatherer | Predicate |
|---|---|---|
| the tree builds and vets clean | `command` | `obs.check.exit == 0` |

Trivial, and worth doing first for the same reason grft's self pass was: it is
the first time the loop runs end to end against a repository nobody constructed
for it.

### Structural — available at increment 4

These are the ones that matter, because they are the invariants that erode
silently and that no test would catch.

| Promise | How it is observed |
|---|---|
| **standing is minted in one place** — no package outside `internal/ledger` constructs a ratification | import graph + symbol reference |
| **the corpus cannot write standing or truth** — `internal/corpus` does not depend on `internal/ledger` | import graph |
| **the verdict does not read the agent** — `internal/predicate` and `internal/tick` do not depend on `internal/thread` | import graph |
| **gatherers do not write** — no filesystem write outside a cache path under `internal/gather` | source scan |
| **the pipeline cannot reach a packet** — the packet assembly does not reference the desire kind | import graph + symbol reference |
| **the core carries no domain vocabulary** — `model`, `tick`, `predicate`, `ledger` mention no gatherer-specific or domain nouns | source scan |
| **every policy cites a decision that exists** | the corpus's own broken-link detection |

The last one is free: once the invariants are policies citing decision
artifacts, a dangling citation is exactly a broken link, and the corpus already
finds those.

### Behavioural — available at increment 4

Named-test predicates, which is where structured observations earn their place.
A promise can say *this specific test exists and passes*, which a boolean check
cannot express:

```yaml
gather:
  tests: { scope: "./..." }
hold_when:
  named(obs.tests, "TestFailedGathererYieldsUnknown").passed
  and named(obs.tests, "TestOutOfScopePathsAreReportedNotLanded").passed
  and named(obs.tests, "TestRevisionDropsStanding").passed
```

This is also how the repo's own convention — *a check that has never fired is
not a check* — becomes enforceable: each named test is the one that
deliberately breaks the guard it covers.

## The path

| Increment | Self-hosting step | Done when |
|---|---|---|
| 1–2 corpus + standing | Move the docs in. Ratify the `AGENTS.md` invariants as policies citing their decisions | An agent's packet contains the invariants as ratified authority, not as a file it was asked to read |
| 3 truth | The first promise, via `command` | `kept` reports it `kept`, and reports `not-kept` when you break the build on purpose |
| 4 catalogue | The structural and named-test promises | Deleting the `internal/corpus` → `internal/ledger` boundary makes a promise go `not-kept` within one tick |
| 5 threads | `kept repair` against a real drift, read-only until the diff is reviewed | An agent repairs one of these promises in a box, the gate re-derives from the tree rather than from its account, and you land it |
| 6 assessor | The throughline against this repository | The assessor catches something the predicate missed, and the fix is a stronger predicate |

**Self-hosted** means step 5 has happened on a promise nobody wrote to be easy.

## Two things to watch

**A promise about the code is easier to write than a promise about the corpus.**
Import-graph checks are comfortable territory. The harder and more valuable
ones are about the documents — every policy cites a live decision, every
decision referenced in code exists, the deck's verb list matches `main.go`.
Those exercise the parts of the corpus a code-only repository never would, and
they are the ones a manuscript would also need.

**Do not write a promise you would not have written anyway.** The failure mode
of a self-host pass is a register full of promises that exist to demonstrate
the register.
