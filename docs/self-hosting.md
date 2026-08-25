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
| `docs/decision/*.md` | **decision** artifacts | Ratified decisions have standing; an authored, unratified ADR is a proposal. Neither has a cadence or predicate, so neither has truth. Cited by the policies they justify |
| `docs/model.md`, `lineage.md` | **document** artifacts | Nothing can be in breach of one — if `model.md` and the code disagree, either could be the bug. Force only when something ratified cites them |
| `docs/increments.md` items | **desire** artifacts | Wants with no standing. Ratified into promises as you commit to each |
| `docs/open-questions.md` | stays a plain file | Questions, not wants. Nothing obliges and nothing observes |
| `README.md` | stays a plain file | The front door. GitHub renders it; the corpus would not improve it |

That table is what the corpus is **for**, and it is now partly a description of
what is in it. Two rows have happened. The fifty decisions are artifacts with
standing, adopted by `kept doctor --fix` and ratified. And thirteen of
`AGENTS.md`'s invariants are ratified policies in `docs/policy/` — the ones
that are checked before an act, each citing the decisions it rests on; the rest
stay in `AGENTS.md` because nothing evaluates them at the moment of acting.
`docs/document/`, `docs/desire/` and `docs/canon/` are still empty, so
`model.md` and `lineage.md` are still prose nothing cites.

**Layout.** `docs/` becomes the corpus root, so the corpus and the docs are one
tree and GitHub still renders it:

```
docs/_schema/{type}.yaml       one per kind, declaring what binds
docs/policy/{slug}.md          ratified rules about conduct
docs/decision/{slug}.md        the ADRs; ratified ones have standing
docs/document/{slug}.md        model, lineage
docs/desire/{slug}.md          uncommitted wants
docs/promise/{slug}.md         commitments about this repository
refs/kept/ledger               standing, truth, observations — tracked
```

The full layout, and why the ledger is tracked while a cache is not, is in
[`layout.md`](layout.md).

The ledger ref is **not** gitignored. Ratifications and verdicts are acts and
observations about this repository, and losing them on a fresh clone would mean
every promise reverts to unratified. Its former checkout paths are ignored so a
local file cannot put them back in a box; only the cache and boxes are otherwise
ignored. Check this deliberately: grft was blinded once because a working root
sat inside a gitignored path and its containment checks silently saw nothing.

## The payoff arrives before anything is checkable

A policy with no gatherer still binds, and still goes into the packet. So
moving `AGENTS.md` into the corpus pays off at increment 2, before any promise
exists: the invariants stop being a markdown file an agent may or may not have
read, and start being ratified authority assembled into every packet.

That is the same shape as canon — most of it has standing and no truth-seeking.

**Half of this payoff has been collected.** The invariants that are checked
before an act are policy artifacts with standing, so what an agent is bound by
is ratified rather than remembered. What is still outstanding is the other
half: there is no packet assembly to put them in — `internal/thread` is a doc
comment — and that is what increment 5 is. `policy:the-pipeline-is-never-in-a-packet`
is authority waiting for it, which is the ordinary case for a policy: one with
no gatherer binds by being read.

## The promises

Written against `AGENTS.md`'s invariants, in the order they become checkable.

### Available at increment 3

| Promise | Gatherer | Predicate |
|---|---|---|
| the tree builds and vets clean | `command` | `obs.check.ok`, over `ok: exit == 0` |
| every package under `internal/` except `internal/confine` builds and passes | `go/tests` | `obs.tests.ok and count(obs.tests.packages, p => p.ok) > 0` |
| `internal/confine` builds and passes | `go/tests` | the same predicate, over the one package |

`command` never stores what a program printed, so a promise reading its exit
status declares a field for it and the predicate reads that field —
`obs.check.exit` is not a thing a `hold_when` can say. See
[0023](decision/0023-what-may-be-captured.md).

All three are trivial, and worth doing first for the same reason grft's self
pass was: it is the first time the loop runs end to end against a repository
nobody constructed for it. They are `promise/the-tree-vets-clean.md`,
`promise/core-packages-are-tested.md` and
`promise/the-confinement-suite-passes.md`.

**The last of those is one promise the loop deliberately cannot close, and it is
the only one.** `internal/confine`'s tests cut real boxes and plant their canary
sentinels under the operator's home, precisely because the provider's baseline
grants nothing there; re-deriving them inside a repair box would mean granting a
confined agent that same place. So the suite is split off, gathered on the same
scope and the same cadence as everything else under `internal/`, and given no
`repair` block — which is the whole of the mechanism, since `internal/repair`
and `internal/thread` both refuse to open a thread for a promise that bounds
nothing. The exception is one package, named in the corpus, rather than a
property of the loop nobody wrote down. Open question 11 records what it cost.

### Behavioural — also available at increment 3

Named-test predicates are where structured observations earn their place, and
they need no gatherer beyond `go/tests`, which declares `cases` and `failures` for
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
| **standing is minted in one place** — no package under `internal/` outside `internal/ledger` constructs a ratification | `go/symbols`. `cmd/kept` constructs one and must: the CLI is where a person's act enters, and a person is the only thing that may mint standing |
| **the corpus cannot write standing or truth** — `internal/corpus` does not depend on `internal/ledger` | `go/imports` |
| **the verdict does not read the agent** — `internal/predicate` and `internal/tick` do not depend on `internal/thread` | `go/imports`. It holds vacuously until increment 5 puts something in `internal/thread`, which is the increment that could break it |
| **gatherers do not write** — no filesystem write outside a cache path under `internal/gather` | `symbols` |
| **the core carries no domain vocabulary** — `model`, `predicate` and `ledger` name no gatherer and no domain noun | `go/imports` for the edges, `symbols` for the identifiers. Not `tick`, which imports `gather` by construction; and it has to be identifiers rather than lines, because the doc comments say `"tests@2"` on purpose |
| **every policy cites a decision that exists** | `corpus` — a dangling citation is a broken link, and the corpus already finds those |

**Five of those six are written, and each has a test that breaks the boundary it
names.** They are `promise/standing-is-minted-in-one-place.md`,
`the-corpus-cannot-write-standing-or-truth.md`, `gatherers-do-not-write.md`,
`the-core-carries-no-domain-vocabulary.md` and — the last row, widened to the
whole corpus so that it says something — `every-citation-in-the-corpus-resolves.md`.
That last one is no longer vacuous: there are thirteen policies now, each citing
the decisions it rests on, and deleting one of those decisions makes the promise
`not-kept` within a tick.

Two rows are still not written, and both for the same reason. **The verdict does
not read the agent** would hold vacuously — `internal/thread` is empty, so the
edge it forbids has nothing to be an edge to — and a promise that cannot fail
reports `kept` and means nothing. **The pipeline cannot reach a packet** is not
writable at all: there is no packet assembly to check. Both become real in
increment 5, alongside the code that could break them, and that is when to write
them.

One thing the first row cannot see, recorded because it bounds what was bought.
`symbols` reads package-qualified references, so a package that already holds a
`*ledger.Ledger` and calls `.Ratify` on it is invisible to *standing is minted in
one place*. The promise catches the shape changing — a second constructor, a
package opening a ledger of its own — and the policy is what covers the rest.

The positive half of the citation row is also unwritable today: that every policy
cites *something* needs a gatherer reporting the corpus's edges, and `corpus`
declares only the ones that point at nothing.

Reading identifiers rather than lines is also what keeps these auto-landing.
A regexp over source is a *pattern* instrument in `model.md`'s autonomy table
and sets an assessor gate, so the two source-scan rows would not be
trustworthy until increment 7; a `symbols` gatherer answering off the syntax
tree is structural.

## The path

| Increment | Self-hosting step | Done when |
|---|---|---|
| 1–2 corpus + standing | Move the docs in. Ratify the `AGENTS.md` invariants as policies citing their decisions | An agent's packet contains the invariants as ratified authority, not as a file it was asked to read — **the corpus half is done**: fifty decisions and thirteen policies, ratified, each policy citing what it rests on. Nothing assembles a packet yet, which is increment 5 |
| 3 truth | The first promises, via `command` and `tests` | `kept` reports them `kept`, and `not-kept` when you break the build on purpose — **done** |
| 4 catalogue | The structural and named-test promises | Deleting the `internal/corpus` → `internal/ledger` boundary makes a promise go `not-kept` within one tick — **done for the five structural rows**, each with a test that crosses the boundary on purpose and a second that moves something beside it and must not fire |
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
