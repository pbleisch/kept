# Open questions

Genuinely undecided. Each says what it depends on, so it can be closed when
something forces the answer rather than by guessing now.

## 1. The predicate language

The one component with no precedent in the nine (see `lineage.md`). Settled: it
needs comparison, existence, set membership, a fold over a collection, and
`prior()`. Undecided:

- **What `prior()` returns.** The previous observation, or the previous
  observation that yielded `kept`? "Never shrinks" wants the previous
  observation regardless of verdict. A freshness rule might want the last good
  one. Possibly both, named differently.
- **How far back `prior()` can reach**, and whether a predicate may fold over a
  window of observations rather than just the last.
- **Whether it can reference another artifact's truth.** Tempting for "no
  chapter is final until canon is kept", and it introduces evaluation order and
  cycles. Default to no until something needs it.
- **Grammar and implementation.** Hand-written, `expr`-style library, or CEL.
  Zero dependencies is worth something; so is not writing a parser.

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

## 5. Sectioned bodies

Decision 0010 says a kind that needs two prose blocks uses H2 sections named by
the schema. Undecided: how the schema declares them, whether a missing section
is an error or an empty field, and how `body.<section>` in a `hash` list
behaves when the section is absent.

## 6. Where desires live

They must never reach a packet, which the context assembly function enforces.
Undecided whether they are corpus entities in a `desires/` directory (visible,
linkable, greppable) or a ledger construct like work. Being able to link a
desire to the observations that argued for it favours the corpus.

## 7. Does v1 need the index?

SQLite and FTS5 are derived and rebuildable. For a few hundred artifacts a
filesystem walk may be enough, and the index is a second thing that can be
wrong. Decide by measuring a real corpus rather than in advance.

## 8. The gatherer catalogue's boundary

Thirteen are sketched in `model.md`. Undecided which are built in, which are
subprocesses over a stdio protocol, and whether third-party gatherers resolve
ahead of built-ins on `PATH`. Whatever the answer, a gatherer declares its
output fields — [0023](decision/0023-what-may-be-captured.md) — so the protocol
carries a declaration and not just a payload. grft's plugin protocol — exec, one JSON object
in, one out, `PATH` wins — is the reference and is deliberately tiny.

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
