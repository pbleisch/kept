# Open questions

Genuinely undecided. Each says what it depends on, so it can be closed when
something forces the answer rather than by guessing now.

## 1. The predicate language — closed

Answered by [0036](decision/0036-the-predicate-language.md), which was decided
against real promises about this repository rather than in the abstract, and by
building the thing. `prior()` returns the previous observation whatever verdict
it produced, reaches back exactly one, folds within a world lineage, and may not
read another artifact's truth. The grammar is hand-written and three-valued.

The deciding argument was not the one this question anticipated. Zero
dependencies and not writing a parser were both real, and neither settled it:
`expr` evaluates a missing field to `false`, which reports `not-kept` for a
gatherer that could not measure, and CEL — which gets the three-valued
connectives right — has one error channel, so *nobody measured* and *this
expression is wrong* are told apart only by matching English prose. The
distinction the whole model rests on is worth two Go types.

One thing it settled that this question never asked: the extractor 0023 requires
of `command` is the same language over a different namespace, so one decision
covers both.

## 2. Freshness — closed

Answered by [0025](decision/0025-no-scheduler-cadence-is-change-driven.md), and
not by any of the three candidates. Freshness is not a separate mechanism: a
promise is due when its scope's tree changed *or* its last observation is older
than `max_age`, so staleness is the second half of *due* rather than a third
place for it to live.

## 3. The act / wait / suppress / escalate policy

Decision 0009 settles that this is compiled rather than a model call. It does
not settle the shape. Candidates: a fixed ranking function; per-artifact
policy fields; a policy artifact that matches on drift kind, magnitude and age.
The last is the most consistent with the model — a policy is already a first
class artifact — and the most machinery.

## 4. Budget

Settled that it must be able to **refuse a dispatch** rather than report after
the fact. Undecided: what it counts (money, wall time, attempts), what it is
scoped to (artifact, day, corpus), and what happens at the limit — refuse
silently, escalate, or degrade to detect-only.

Refusing needs a counter something can decrement under exclusion, so the
settled part is only settled where such a counter exists. Runs on one machine
share one, under the same lock they take before opening a thread. Runs that
share no lock over-spend by at most one run's own limit, which is the same
bounded waste [0013](decision/0013-the-gate-makes-concurrency-safe.md) accepts
for duplicated repair.

## 5. Sectioned bodies — closed

Answered by [0040](decision/0040-a-sectioned-body-and-its-names.md). A schema's
`body:` takes a list of section identifiers where it took one field name; a
declared section that is absent is absent rather than empty, and is reported; an
H2 that maps to no declared name is an undeclared section, legal and unbindable;
and `body.<section>` in a `hash` list encodes as absent when the section is,
using the marks 0031 already spent on exactly that distinction.

The question it did not ask is the one that mattered. **How a heading becomes an
identifier** is one-way — the name lands in a binding preimage — so it is frozen
at `norm:1` with the rest: ATX H2 only and never inside a fence, NFC then
lowercase, every run of non-alphanumerics to one `-`, and no exception list,
because an exception list grows and every entry re-hashes every sectioned
artifact there is.

## 6. Where desires live

They must never reach a packet, which the context assembly function enforces.
Undecided whether they are corpus entities in a `desires/` directory (visible,
linkable, greppable) or a ledger construct like work. Being able to link a
desire to the observations that argued for it favours the corpus.

## 7. The index — closed

Answered **no** by [0044](decision/0044-v1-has-no-index.md), and answered the
way this question asked for it: by measuring rather than in advance. `kept
drift` — the most expensive verb — takes 11 ms over the forty-four artifacts
this repository has and 108 ms over a synthetic two thousand, which is about
50 µs per artifact for a full walk and a full fold together, and one second at
roughly twenty thousand artifacts. Tripling the ledger against the same corpus
costs 2 ms, so the fold is not where the time goes.

None of the index's three jobs survived that. The drift query is fast without
it; the link graph is resolved at load and is what `kept list` reports; and
search has no verb, so full-text is a capability with no consumer. What was
left was a second thing that can be wrong, which is what 0001 and 0032 already
refused for standing.

`internal/index` is deleted rather than left as a stub, because a stub reads as
a thing that was decided and merely not built. The question re-opens on a verb
that needs full-text ranking, or a corpus where `kept drift` exceeds one
second.

## 8. The gatherer catalogue's boundary — the resolution rule is closed, the wire is a sketch

The half that had to be decided before increment 4 added a gatherer is decided by
[0043](decision/0043-the-catalogue-is-closed-and-a-third-party-is-namespaced.md):
**the built-in catalogue is closed and it wins, a third party is written
`x/<name>`, and `PATH` never shadows a built-in.** grft's reference is
deliberately tiny and it is right for grft; it does not transfer, because grft's
plugin output was advisory to a person and kept's gates an automated land.

That half had to close first because the instrument's *name* binds — `gather` is
in `_schema/promise.yaml`'s hash list — so moving a promise from `tests` to
`x/tests` drops its standing and a person grants it again. Under `PATH` wins,
which instrument answered would be decided by the environment: invisible in the
text, invisible in the diff, and uncovered by the hash.

**The wire is written down in 0043 and marked not yet binding.** No third-party
gatherer exists or is wanted, and every gatherer increment 4 adds belongs inside,
so building a compatibility surface with no consumer is the one case where
building teaches nothing. What stays open is therefore not *what the protocol
should be* but *whether the sketch survives its first plugin* — and the first
plugin revises it rather than being held to it.

## 9. Concurrency, beyond merge

Decision 0011 settles the storage side: everything tracked merges, because no
two writers write the same bytes. [0013](decision/0013-the-gate-makes-concurrency-safe.md)
answers most of the coordination around it: correctness comes from the gate
re-deriving from the world, so exclusion is a cost optimization rather than a
safety mechanism, and a duplicated repair lands nothing. It is proposed, not
accepted, and it leaves these open.

- **Lock granularity.** Per artifact, or per scope? Two threads repairing
  different promises that touch the same files do not collide until they land,
  where [0015](decision/0015-landing-is-a-merge-queue.md) catches them — so the
  question is now how much duplicated agent spend a coarser lock would save,
  which is a measurement rather than a design question.
- **The box pool.** How many boxes, who discards the ones a failed run left
  behind, and whether a gather box at `origin/main` is reused across runs or
  cut fresh. Unbounded worktrees are a disk problem nothing currently bounds.
- **What a run does when it holds no lock and the drift is urgent.** Skipping
  is right when another run is on it, and indistinguishable from skipping
  because a wedged holder never released.
- **Landing contention.** Answered by
  [0015](decision/0015-landing-is-a-merge-queue.md), which also corrects the
  premise: git's pushes are atomic, so the problem was never order but
  isolation between the gather that gates a change and the push that ships it.
  What remains open is whether a seconds-long critical section is fast enough
  in practice, and that is a measurement — the answer if it is not is
  speculative batching, and that is the first thing that would want a daemon.
- **What the staleness bound actually is.** A number picked in advance is a
  guess; it wants measuring against how often the operator really ratifies and
  releases.

## 10. A second principal

Distinct from concurrency, and further off. What breaks: standing has no
gradations, and the ledger has no notion of who may ratify what. harness's
trust levels and altitude bounds are the known answer and should not be built
before there is someone to bound.

## 11. kept's own confinement tests, inside a kept box

`promise:core-packages-are-tested` runs `go/tests` over `internal/**`, and
`internal/confine` is part of that. Those tests start real confined children,
so running them *inside* a box asks the provider to sandbox a process that is
already sandboxed — and the first repair kept performed had to route around it
by choosing a promise whose instrument parses instead
([0055](decision/0055-a-confined-runtime-is-resolved-homed-and-reaches-one-port.md)
records what that repair needed; the choice is in
`internal/confine/piinbox_test.go`).

What it depends on: whether a repair box may nest a provider at all, or whether
kept's own confinement suite is the one thing a repair of kept cannot re-derive
in a box and has to be gated outside one. The second is not obviously worse —
`policy:the-box-is-not-the-jail` already says the box is not the boundary — but
it means a promise whose scope covers `internal/confine` can never be repaired
by the loop that repairs everything else, and that is a real gap rather than a
detail. Nothing forces the answer until a repair is dispatched for a promise
whose gather actually reaches those tests.
