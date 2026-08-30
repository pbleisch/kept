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
  behind, and whether a gather box at the landing target is reused across runs or
  cut fresh. Unbounded worktrees are a disk problem nothing currently bounds.
- **What a run does when it holds no lock and the drift is urgent.** Skipping
  is right when another run is on it, and indistinguishable from skipping
  because a wedged holder never released.
- **Landing contention.** Answered by
  [0015](decision/0015-landing-is-a-merge-queue.md), which also corrects the
  premise: git's pushes are atomic, so the problem was never order but
  isolation between the gather that gates a change and the write that ships it.
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

## 11. kept's own confinement tests, inside a kept box — closed

Answered by the second of the two candidates: the confinement suite is the one
thing a repair of kept cannot re-derive in a box, and it is gated outside one.
`promise:core-packages-are-tested` now runs `go/tests` over `./internal/...`
with `./internal/confine` excluded, and `promise:the-confinement-suite-passes`
runs that one package on the same scope and the same cadence. Nothing stopped
being checked. What changed is that the exception is named, bounded to one
package, and in the corpus rather than in folklore.

The first candidate — a repair box nesting a provider — was not refused on
principle. It was refused on where the evidence has to live. What makes those
tests evidence is that they cut their boxes and plant their canary sentinels
**under the operator's home**, because the provider's baseline grants the
operating system's temporary tree write access recursively and a boundary
asserted there passes having measured nothing (`makeCanaryState` and
`ungrantedRoot` both say so). A repair box grants three paths — the promise's
repair scope inside the box, and a temporary directory and a session home both
beside it — and the suite's own repositories and sentinels are in none of them.
Re-deriving it in a box means adding a grant under the operator's home, for a
confined agent, to the one place the suite depends on nothing being granted,
which does not measure confinement so much as dismantle it. Nesting a provider
is the smaller half of the problem; the sentinels are the larger one.

**What it cost.** The gap the question named is real and is now permanent
rather than pending: `internal/confine` is checked every tick and repaired by
nobody, and `promise:the-confinement-suite-passes` says so in its body, because
the body is what whoever repairs it would read. The mechanism is that it writes
no `repair` block — `internal/repair` and `internal/thread` both refuse to open
a thread for a promise that bounds nothing, and there is no second mechanism and
no flag. `TestTheConfinementPromiseAuthorizesNoRepair` holds the artifact to
that shape.

It also cost an argument on `go/tests`. Go's package patterns cannot negate, and
enumerating the packages that remain would put a list in a field that binds,
where a package added later is silently unmeasured — the same failure
`count(obs.tests.packages, p => p.ok) > 0` is written against. So `exclude`
takes package patterns, resolves them through `go list`, and **refuses an
exclusion that removes nothing**: the day `internal/confine` is renamed, the
narrowed promise reads `unknown` rather than quietly running the suite it was
written to keep out.

The repair bound did not narrow. `repair.scope` stays `internal/**`, because a
glob cannot negate either and a grant is always at least as wide as the scope it
stands for. What keeps `internal/confine` out of a repair is the box's
re-derivation measuring the narrowed promise, not the scope.

## 12. The refused credential family is incomplete, and nothing measures it

[0072](decision/0072-a-runtime-version-constraint-is-a-review-record.md) widened
the version constraint and, in doing so, measured something it did not set out
to. `claudeRefusedEnvironment` is the family of parent environment names a
Claude dispatch refuses because they would override, redirect or switch the
credential. It was assembled by reading two of the runtime's own enumerations —
its auth-state report and its `strictPolicyHelperKeys` set — which is not the
same as exhausting what the binary reads.

Reading `process.env` accesses out of the 2.1.241 and 2.1.251 images, both build
credential-shaped names the family does not name. `CLAUDE_CODE_WEBSOCKET_AUTH_FILE_DESCRIPTOR`
is the same file-descriptor credential shape as two names that *are* refused.
`CLAUDE_CODE_PROXY_AUTHENTICATE` is passed into a child alongside
`CLAUDE_CODE_PROXY_URL` and `CLAUDE_CODE_PROXY_HOST`, both refused, and is not.
`CLAUDE_BG_CLAIM_AUTH`, `CLAUDE_BG_PTY_AUTH` and `CLAUDE_BG_RV_AUTH` sit beside
two `CLAUDE_BG_*` paths that are refused. `CLAUDE_CODE_CLIENT_CERT` and its key
and passphrase load an mTLS client identity. None of these arrived with 2.1.251:
every one is read by the build the old pin was taken against, so this is not a
regression a version bump introduced and was never something a pin could have
caught.

What is undecided is the **instrument**, not whether the gap should close.
Enumerating by hand is what produced an incomplete list once already. Scanning
the image for `process.env.NAME` finds these, but `docs/nono-surface.md` already
records that a literal scan of a bundle under-reports — names composed at
runtime from a prefix are invisible to it — and under-reporting is the unsafe
direction for a deny list. A default-deny child environment would invert the
problem into kept's favour and is the shape worth costing: `Invocation.Env` is
already a complete child environment rather than an addition to the parent's, so
the family exists only to refuse a *parent* whose presence signals operator
intent kept would rather not silently override. Whether that signal is worth an
enumerable list, or whether the refusal should be a small allowlist of names
known to be harmless, is the actual question.

Not urgent in the sense that matters: confinement bounds what the child can
reach regardless ([0017](decision/0017-the-box-is-not-the-jail.md)), and the
brokered credential route means kept's own process never holds the secret
([0057](decision/0057-the-credential-is-brokered-and-the-box-holds-a-phantom.md)).
The exposure is an operator who has one of these set for their own reasons
getting a repair that authenticates or routes somewhere they did not intend,
without being told.
