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

## 3. The act / wait / suppress / escalate policy — the shape is proposed, the selection is open

Decision 0009 settles that this is compiled rather than a model call. It does
not settle the shape. Candidates: a fixed ranking function; per-artifact
policy fields; a policy artifact that matches on drift kind, magnitude and age.
The last is the most consistent with the model — a policy is already a first
class artifact — and the most machinery.

**[0079](decision/0079-a-repair-is-an-episode-that-reworks-itself.md) proposes
the policy artifact**, and the "most machinery" objection is weaker than it was
when this was written: `policy` is a kind, `applies_to: dispatch` is a value the
corpus already uses, `on_fail: refuse|warn` is declared, `repair.dispatchAuthority`
already collects every ratified one, and `ledger.Outcome` already declares `act`,
`wait`, `suppress` and `escalate` with `ledger.Condition` declaring `budget`,
`policy` and `rank` — each with a comment naming this question. What is missing
is that nothing *evaluates* a dispatch policy at the moment of dispatching,
which is what the kind was defined to be.

It answers this question for **one half** of it. 0079's northstar — rework is
automatic, a person prepares artifacts and lands work — makes *what happens to an
assessment* a policy decision and settles it. **Which drift is worth opening an
episode for is still open**, and it is the half 0060's reasoning protects:
several drifts are a choice a person makes rather than one kept makes for them.
`docs/increments.md` states an end state where the queue drains overnight, and a
queue that drains overnight is opened by something that is not a person — so
closing this needs its own decision and probably a supersession of 0060.

**[0086](decision/0086-the-pass-rules-and-a-rejected-repair-is-reworked.md)
builds that half, and builds it in Go rather than as an evaluated policy.** Once
an attempt has dispatched, its reading selects ready, rework or escalate, and a
reject from the adversarial pass is one of the readings. The table lives in
`repair.decide`. No ratified artifact states it, so this question's objection to
a fixed function still applies to that table. What stops it deciding anything
0060 protects is that it only ever acts on an episode a person opened. The
unbidden opener, concurrency caps and the proposer are still open, deliberately.

**[0088](decision/0088-a-tick-forecasts-an-opening-and-a-proposals-hold-may-bind.md)
proposes three answers inside that open half**, ahead of the opener itself. Only
the opener writes a disposition from an opening rule, and a tick shows that
verdict as a forecast and records nothing from it. A proposal's receipt is a
record kind of its own rather than a thread. A ratified opening rule may refuse
on a proposal's hold, which amends the second clause of 0009. The opener, the
cap and the proposer stay open. So do the supersession of 0060 and the amendment
of 0009's first clause, ranking, which any proposer needs.

A third question belongs with this one and was never recorded here at all:
**where the assessor runs**. Nothing forced it, because
[0078](decision/0078-the-derived-checklist-is-closed-and-the-argument-is-not.md)'s
derived half needs no box; the northstar forces it, because a command a person
types cannot be a step in an automatic cycle. 0079 answers it.

## 4. Budget — proposed in 0079

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

**One counter now exists and it is a better one than that paragraph imagines.**
[0077](decision/0077-the-receipt-says-what-answered-and-what-it-consumed.md)
projects the runtime's own reading of the usage windows it is spending —
measured on the one real repair in this ledger, `five_hour` utilization 0.24 to
0.25 with `seven_day` at 0.11. It is a reading of something shared across every
machine and every session on the account, including a person's interactive ones,
so it needs no lock, nothing decrements it, and it cannot drift. It refuses only
the *next* dispatch, which is what this question asks for. Two things it does not
settle: a self-hosted runtime reports **no window at all**, so a budget reading
this must tell *no window reported* from *window empty*; and it is not money, so
a budget that counted money alone would read a repair costing fourteen minutes of
someone's hardware as free.

**[0079](decision/0079-a-repair-is-an-episode-that-reworks-itself.md) proposes
answers to all three**, and the reason it can is that it states a goal that makes
the budget non-optional: an automatic rework loop with no bound is unbounded
spend. It proposes two counters rather than one — attempts per episode, which is
exact and local and is the only thing that bounds thrashing, and 0077's reported
window, which bounds whether this machine should start work at all — scoped to
the episode and to the account respectively, with money recorded and never the
counter that refuses. At the limit it **escalates**: under that goal the limit is
one of the few places a person legitimately re-enters, so it must be visible
rather than quiet.

**[0086](decision/0086-the-pass-rules-and-a-rejected-repair-is-reworked.md)
builds both counters.** The attempts bound belongs to the repository: it is
`repair.attempts` in `.kept/config.yaml`, 3 when unset, and an operator or
`--attempts` may only lower it. It is scoped to one run of `kept repair` rather
than to the thread, because a person's `--rework` is a fresh word. The ceiling
belongs to the operator: it is `repair.window_ceiling`, and it reads only the
episode's own repair sessions, so a runtime that reported no window refuses
nothing. Both escalate at the limit. A ratified dispatch policy may only be
stricter than the ceiling, and nothing evaluates one yet.

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

## 6. Where desires live — closed

Answered **the corpus** by
[0083](decision/0083-a-desire-is-a-corpus-entity-and-its-state-is-derived.md),
on [0008](decision/0008-work-is-not-an-entity.md)'s own test: work is in the
ledger because nobody authors it, and a desire is authored by definition. It
still never reaches a packet — `thread.PacketInput` has no field one could arrive
in — so living in the corpus brings it no nearer one.

What it settled that this question never asked is that a desire carries **no
state of its own**. `status` is gone from the schema: raw, shaped, committed and
retired are derived on read, committed means a *ratified* artifact's own links
say it `realizes` the desire, and a later act wins over an earlier one, so a
file edit never overturns a retirement. Retire applies to a desire and keeps the
file; the reason is the act's note and the condition that would change the
answer is the desire's own `reopens_when` or `ripe_when`.

It left one narrower question open, which is 14.

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
  [0079](decision/0079-a-repair-is-an-episode-that-reworks-itself.md) adds to
  this rather than answering it: it cuts a fresh box per rework attempt, so an
  episode churns several where it used to cut one, and a crashed episode can
  leave more than one registered worktree behind. `kept doctor` now reports
  every box git still has registered, with the command that discards it
  (`promise:doctor-reports-a-box-left-behind`), and removes nothing; who
  discards one, and when, is what stays open.
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

## 13. What a box's own earlier measurement is to the predicate standing in it — closed

Answered by [0080](decision/0080-a-box-folds-its-own-lineage.md), and by the
first of the two shapes this question named. A measurement made in a repair's
box names the thread the box belongs to, and a re-derivation there is handed
that thread's own earlier measurements as history and nothing else.

The deciding argument was that the two comparisons weighed here are not one
comparison to be permitted or forbidden. A box's before and after measurement
are two readings of one lineage, and the workspace's series and a box's are two
lineages — so honouring
[0014](decision/0014-an-observation-names-its-world.md) and folding history in a
box were never in tension. What made it look like a tension is that
`World.Thread` had no writer: with it empty on both sides the two lineages
hashed to one address and the filter written to keep them apart matched them.

Two things it settled that this question did not ask. The history does not come
from the truth series and never will — a verdict reached in a box is refused a
truth record — but it does not have to: 0078 already had a repair file both box
measurements and name them in the receipt, so a thread indexes its own lineage.
And offering the wrong lineage is **refused** rather than filtered, because
history the filter drops and history nobody offered are the same `unknown` from
outside.

One thing it deliberately left alone: a land's gate names no thread and folds no
history, so a promise reading `prior()` answers `unknown` on both sides of a
merge and is never a break there. A gate box holds a tree that has never existed
anywhere, and whether the workspace's series is its lineage is a different
question that nothing has yet forced.

## 14. What returns a retired desire to shaping

[0083](decision/0083-a-desire-is-a-corpus-entity-and-its-state-is-derived.md)
builds one way out of `retired`, and it falls out of the fold: a later grant of
an artifact whose own links say it realizes the desire commits it. There is no
act for the step before that — a person reads a fired `reopens_when`, decides to
want the thing again, and is not yet ready to commit to it.

`kept ratify` cannot be that act, because nothing about a desire binds and a
grant against no content version would survive every edit. A new kind of act is
what generalizing retire exists to avoid. Writing a second desire that cites the
first works today and keeps the decline on the record, at the cost of two files
for one want and a slug `kept want` will refuse if the words are the same.

Nothing forces an answer yet: nothing prints that a condition has fired until a
trial reads one, which is the next decision in `docs/desire-pipeline.md`. The
first person who acts on a fired condition is what closes this.

## 15. A repair checks itself in one environment and is measured in another

**What forced it.** On 2026-09-14 thread `7dpqfr5wdekr4rd4y902yv61vf` repaired
`promise:doctor-reports-a-box-left-behind`, a promise that `go test ./cmd/kept`
pass whole and contain two named tests. The confined session wrote both, ran
them, and the receipt shows `--- PASS` for each. kept's re-derivation of the same
tree then read `not-kept`: the box's two filed measurements are `ok: true`, 223
passed before, and `ok: false`, one failure, after — the failure one of the two
new tests. `kept land` refused on the same failure, and nothing reached a branch.

The cause is one path. The session's `TMPDIR` was the scratch directory
`internal/box` cuts beside the box under the operator's home; the re-derivation
runs `go/tests` with the operator's inherited environment
(`internal/gather/program.go`), whose `TMPDIR` is under `/var/folders`. That is a
symlink on macOS, git reports a worktree by its resolved path, and the test
compared git's spelling against a path it had composed. The feature was landed
by hand with the test resolving its path.

**What held.** The verdict. The re-derivation and the gate both measure in the
environment the promise is owed in, and the session's own account was never an
input to either — which is
`policy:the-agents-report-is-never-an-input-to-the-verdict` doing exactly its
job against a session that believed it had finished.

**What it exposes is that the repairer cannot see what it is judged on**, and
nothing in the design says so:

- **The box's environment differs by construction, not by accident.** The
  profile denies the host's scratch trees, so `TMPDIR` must be granted beside the
  box; `HOME` is `Profile.Home`; `PATH` is constructed (0053). None of the three
  can be the operator's without undoing the confinement that chose them.
- **An observation's world names the tree and the thread, not the environment**
  (0014, 0033). Two measurements of one tree that disagree therefore read as a
  flaky instrument rather than as two environments.
- **`go/tests` records which cases failed and not why.** A packet carrying the
  failing measurement hands the agent a test name and no output — and here the
  name alone points at a test the agent has just watched pass.
- **0079's rework does not reach it.** It re-cuts a box at the anchor and
  re-dispatches with the failing measurement as the gap, into the same
  environment, where the test passes again. Attempts are spent until the budget
  escalates, which is today's outcome by a longer and more expensive route.

**Directions, none chosen.**

- *Re-derive in the box's environment.* The repair's verdict would then agree
  with the session, but it would be a verdict about an environment nothing ships
  into, and the gate would still refuse at the land. It moves the disagreement
  rather than removing it.
- *Hand the session the measuring environment's distinguishing features*, such as
  a `TMPDIR` reached through a symlink. Each property is its own question and the
  list is never finished; it is an enumeration of environments where 0009 refuses
  an enumeration of repairs.
- *Carry the failure's output into a rework packet.* It needs `go/tests` to
  record output, and test output routinely holds temporary paths and timings —
  fields that move on their own in a record addressed by its data (0034).
- *Name the relevant parts of the environment in the world.* Which parts is the
  whole question, and every one chosen mints a fresh address on any machine where
  it differs.
- *Accept it*: work whose result depends on the environment escalates to a
  person, as this one did. It is the cheapest, and the question is whether that
  counts as one of the legitimate re-entries 0079's northstar allows or as the
  loop failing to close.

A second thread whose box and gate disagree for an environmental reason is what
would make this a pattern rather than an incident, and building 0079's rework is
what would make its cost concrete.
