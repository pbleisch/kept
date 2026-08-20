# Self-hosting

The goal: `kept` holds promises about `kept`, and a drift in one of them is
repaired by an agent `kept` dispatched.

The trap to avoid is the one grft's evaluation records — a self-host pass where
every repair is constructed to succeed proves the mechanisms work and nothing
about whether they are useful. Every promise below is about this repository as
it actually is, and the first ones are read-only.

## What moves into the corpus, and what does not

The corpus is not "where documentation lives". It is where **artifacts** live.
So the question for each file is whether it has standing, truth, or neither —
and for the prose kinds, whether anything can be in breach of it. See
[`model.md`](model.md).

| Today | Becomes | Why |
|---|---|---|
| `AGENTS.md` invariants | **policy** artifacts, one each | They are ratified rules about conduct. That is a policy in the model's sense, and several are checkable |
| `docs/decision/*.md` | **decision** artifacts | Ratified, and that is the whole of it: no cadence and no predicate, so no truth. Cited by the policies they justify |
| `docs/model.md`, `lineage.md` | **document** artifacts | Nothing can be in breach of one — if `model.md` and the code disagree, either could be the bug. Force only when something ratified cites them |
| `docs/increments.md` items | **desire** artifacts | Wants with no standing. Ratified into promises as you commit to each |
| `docs/open-questions.md` | stays a plain file | Questions, not wants. Nothing obliges and nothing observes |
| `README.md` | stays a plain file | The front door. GitHub renders it; the corpus would not improve it |

That table is what the corpus is **for**, and not a description of what is in
it. One row has happened: the forty-two decisions are artifacts with standing,
adopted by `kept doctor --fix` and ratified. `docs/policy/`, `docs/document/`,
`docs/desire/` and `docs/canon/` exist and are empty, so `AGENTS.md` is still a
markdown file an agent may or may not have read, and `model.md` and
`lineage.md` are still prose nothing cites. Two of the promises below depend on
that changing, and one of them is vacuous until it does.

**Layout.** `docs/` becomes the corpus root, so the corpus and the docs are one
tree and GitHub still renders it:

```
docs/_schema/{type}.yaml       one per kind, declaring what binds
docs/policy/{slug}.md          ratified rules about conduct
docs/decision/{slug}.md        the ADRs, ratified
docs/document/{slug}.md        model, lineage
docs/desire/{slug}.md          uncommitted wants
docs/promise/{slug}.md         commitments about this repository
.kept/ledger/                  standing, truth, observations — tracked
```

The full layout, and why the ledger is tracked while the index is not, is in
[`layout.md`](layout.md).

The ledger is **not** gitignored. Ratifications and verdicts are acts and
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

**This payoff has not been collected.** The move never happened, so it is
outstanding twice over: there are no policy artifacts, and there is no packet
assembly to put them in — `internal/thread` is a doc comment. The first half is
a morning's work and does not wait for increment 5; the second is what
increment 5 is.

## The promises

Written against `AGENTS.md`'s invariants, in the order they become checkable.

### Available at increment 3

| Promise | Gatherer | Predicate |
|---|---|---|
| the tree builds and vets clean | `command` | `obs.check.ok`, over `ok: exit == 0` |
| every package under `internal/` builds and passes | `tests` | `obs.tests.ok and count(obs.tests.packages, p => p.ok) > 0` |

`command` never stores what a program printed, so a promise reading its exit
status declares a field for it and the predicate reads that field —
`obs.check.exit` is not a thing a `hold_when` can say. See
[0023](decision/0023-what-may-be-captured.md).

Both are trivial, and worth doing first for the same reason grft's self pass
was: it is the first time the loop runs end to end against a repository nobody
constructed for it. They are `promise/the-tree-vets-clean.md` and
`promise/core-packages-are-tested.md`.

### Behavioural — also available at increment 3

Named-test predicates are where structured observations earn their place, and
they need no gatherer beyond `tests`, which declares `cases` and `failures` for
exactly this. A promise can say *this specific test exists and passes*, which a
boolean check cannot express:

```yaml
fields:
  gather:
    tests: { scope: "internal/**", packages: "./internal/..." }
  hold_when: >
    "TestAFailedGatherLeavesNoObservation" in obs.tests.cases
    and not ("TestAFailedGatherLeavesNoObservation" in obs.tests.failures)
```

Membership is the `in` operator and there is no `named()`:
[0036](decision/0036-the-predicate-language.md) closed the language at nine
builtins plus `prior()`, and a tenth is added when a promise needs one rather
than in advance.

This is also how the repo's own convention — *a check that has never fired is
not a check* — becomes enforceable: each named test is the one that
deliberately breaks the guard it covers. And the set never shrinking is
`all(prior(tests).cases, c => c in obs.tests.cases)`, which is the promise
`lineage.md`'s hollow-coverage measurement asked for.

### Structural — available at increment 4

These are the ones that matter, because they are the invariants that erode
silently and that no test would catch.

| Promise | How it is observed |
|---|---|
| **standing is minted in one place** — no package under `internal/` outside `internal/ledger` constructs a ratification | `symbols`. `cmd/kept` constructs one and must: the CLI is where a person's act enters, and a person is the only thing that may mint standing |
| **the corpus cannot write standing or truth** — `internal/corpus` does not depend on `internal/ledger` | `imports` |
| **the verdict does not read the agent** — `internal/predicate` and `internal/tick` do not depend on `internal/thread` | `imports`. It holds vacuously until increment 5 puts something in `internal/thread`, which is the increment that could break it |
| **gatherers do not write** — no filesystem write outside a cache path under `internal/gather` | `symbols` |
| **the core carries no domain vocabulary** — `model`, `predicate` and `ledger` name no gatherer and no domain noun | `imports` for the edges, `symbols` for the identifiers. Not `tick`, which imports `gather` by construction; and it has to be identifiers rather than lines, because the doc comments say `"tests@2"` on purpose |
| **every policy cites a decision that exists** | `corpus` — a dangling citation is a broken link, and the corpus already finds those |

Two things this table used to claim and cannot yet. **The pipeline cannot reach
a packet** is not writable at increment 4: there is no packet assembly to
check, and it becomes checkable in increment 5, alongside the assembly function
that enforces it. And **every policy cites a decision** is vacuous until the
invariants are actually policy artifacts — `docs/policy/` is empty, so today it
is a promise nothing can break, which is the failure this document's own
closing paragraph warns about.

Reading identifiers rather than lines is also what keeps these auto-landing.
A regexp over source is a *pattern* instrument in `model.md`'s autonomy table
and sets an assessor gate, so the two source-scan rows would not be
trustworthy until increment 7; a `symbols` gatherer answering off the syntax
tree is structural.

## The path

| Increment | Self-hosting step | Done when |
|---|---|---|
| 1–2 corpus + standing | Move the docs in. Ratify the `AGENTS.md` invariants as policies citing their decisions | An agent's packet contains the invariants as ratified authority, not as a file it was asked to read — **not yet**: the decisions moved and are ratified, `docs/policy/` is empty, and nothing assembles a packet |
| 3 truth | The first promises, via `command` and `tests` | `kept` reports them `kept`, and `not-kept` when you break the build on purpose — **done** |
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
