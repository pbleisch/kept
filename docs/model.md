# The model

## Three properties

Every artifact has **content**. It may have **standing**, and it may have
**truth**. Its type is only which routes to those two are open to it.

**Content** is authored. The statement, the predicate, the scope, the trigger,
and the prose. Free to write, free to change, and it grants nothing.

**Standing** answers *may this oblige anyone?* It is created only by
ratification, which is a human act, and passed to work by delegation.

**Truth** answers *is this so?* It is acquired only by observing the world,
derived when read, and recorded in the ledger as a series.

Neither converts into the other. A pile of evidence does not grant permission
to act, and permission does not make anything so.

## Canon, fact and document, told apart by one question

All three are prose about the world. Telling them apart by standing — *canon is
the ratified one* — is true and does not tell an author which to write. The
question that separates all three is **can something be in breach of it?**

| | if it and the world disagree | can something be in breach |
|---|---|---|
| **canon** | the world is wrong | **yes** |
| **fact** | the record is wrong, and stale | no |
| **document** | whichever is wrong, is wrong | no |

The sharpest test is a file in this repository. `AGENTS.md` says of `model.md`:
*if the code and `model.md` disagree, one of them is a bug; say which.* That is
precisely not canon — canon says the work is wrong, full stop. **If you cannot
say in advance which side loses, it is a document.**

Two authoring rules fall out of it.

**Canon is a declaration of what is; a document is an explanation of
something.** "Kael's eyes are green" is canon. "How eye colour is used across
the series" is a document, however carefully argued and however much of it is
true — because when it and the series disagree, nothing says beforehand which
one gives way.

**A promise is a commitment to do; canon is a declaration of what is.** That is
why most canon has no gatherer, and why canon is a kind at all rather than
everything being a promise: a statement can bind without anyone having
undertaken to make it so.

A fact is on neither side of it. It is what the world appeared to be when
something looked, so it carries provenance and modality and no standing — and
when it and the world disagree nothing is in breach, because nothing was
committed to. The record is simply old.

## Standing and truth are not in the file

The file holds content. Both properties are derived on read, from the ledger,
keyed by a hash of the fields the schema declares binding:

```
ratified(promise:core-packages-are-tested, sha256:ab12…, norm:1, paul@…, at, world:9f31…)
truth(promise:core-packages-are-tested, sha256:ab12…, world:9f31…, not-kept, obs #8817, at)
```

The world on a truth record is part of its key; the world on an act is
**advisory** and is read by nothing that decides standing. It says where the act
was performed so that a grant for a wording that landed nowhere can be told
apart from one that landed and was revised — the question an act could not
answer about itself. See [decision 0063](decision/0063-an-act-names-the-world-it-was-performed-in.md).

The hash normalizes before it hashes — unicode form, line endings, trailing
whitespace — so a checkout on another platform does not silently revoke every
ratification in the corpus. `norm:1` names the rules it used, so refining them
later re-hashes old records under the version they were written with.

Edit the file and the hash changes, so standing and truth stop applying with
nothing having to notice the edit. There is no invalidation step, and no way
for the two to fall out of step.

Keeping standing out of the file is not tidiness. The repair agent can write
the file; it cannot write the ledger — and it cannot write it because it never
has the bytes. The ledger and the artifacts it references live on their own
git ref, which no working tree checks out.

**That ref is guarded twice, because it is the one record here nothing can
re-derive.** Git gives every branch two refs — a tracking copy a fetch
overwrites freely, and a local one only an explicit act advances — and in a
workspace `kept init` has run in, the ledger has both. A `git fetch` writes
`refs/kept/upstream/ledger`; moving `refs/kept/ledger` onto it is an act kept
performs, in `kept ledger sync` or in any verb that writes, and it
fast-forwards or it refuses. A fork is a union commit a person makes, because a
merge kept composed could lose the one file in this system that is not
reproducible. The ref is logged as well, so a value it has held and can no
longer reach is caught: every verb that reads or writes the ledger refuses,
naming what to recover. That makes a loss survivable rather than impossible —
deleting the ref takes its log with it. See `decision/0062` and
`decision/0054`.

**The register is the answer to what is in force.** It is not a store — there
is nothing to keep in step. It is the derived view over the ledger: fold the
ratifications, compare each against the artifact's current hash, and what
survives is what obliges anyone right now. `kept status` prints it. Like drift,
it is computed rather than kept.

**Standing attaches to a content version.** Revise a ratified promise and
standing must be granted again — the operator agreed to the old wording.

**Truth is keyed to the content version too.** Change the predicate and the
prior verdicts do not transfer; they were about a different statement.
Observations do transfer, because they are about the world and know nothing
about any promise. That is what lets a strengthened predicate read history from
before it existed.

**Truth is keyed to a world as well.** A verdict reached in a repair's box and
one reached in the workspace are about different trees, and a fold filters by
world after it folds by event time. A box's verdict is not even a second series
in the artifact's stream: a verdict whose world names a thread is refused there
and recorded on the thread, because *did the repair work on its own terms* is
evidence about the repair. See `decision/0035`.

## Identity, and what absence means

An artifact's identity is a slug authored in its frontmatter, minted once and
never rewritten. **The filename is not the identity**, so renaming a file
changes nothing — there is no rename to detect and no history to migrate. Two
artifacts claiming one slug is a load error naming both files.

A ratified artifact whose content cannot be found is **absent**: it keeps its
standing and reports that its content is gone. Absence is never read as a
release, because it cannot be told apart from a bad merge, a partial checkout,
or a corpus root pointed at the wrong place — and one misconfiguration must not
silently release everything. Retiring a promise is `release`, which is a human
act. Deleting the file is not a way to be rid of it.

## Observation, predicate, truth

Three things, not two.

| | Belongs to | Is about |
|---|---|---|
| Observation | the gatherer | the world. It does not know what anyone promised |
| Predicate | the artifact | what would satisfy this particular statement |
| Truth | neither — derived | the result of applying one to the other |

```
gather(scope, world)      → observation      # cached per (world, gatherer, args)
evaluate(predicate, obs)  → kept | not-kept | unknown
```

An observation names the **world** it measured — the tree, not just the paths.
Scope says where it looked; world says what it looked at. Without it the two
gathers in the thread loop below are indistinguishable, and a cache keyed
without it serves the second one the first one's answer. See `decision/0014`.

Three consequences:

- **One observation serves many promises**, in one world. Run `tests` once per
  tick; every promise about tests in that tree reads the same observation.
- **`unknown` falls out of the shape.** If a gatherer fails — toolchain
  missing, timeout — there is no observation, so no predicate runs. A machine
  that could not measure does not record what it thinks it saw.
- **Predicates read observations; packets read facts.** A fact is a record of
  an observation *then*, not evidence of the world *now*.

## Gatherers

A gatherer runs something and returns **structure, not a boolean**. That is the
difference between a promise that can only say "a test file exists" and one
that can say "the set of tests never shrinks".

Promises name gatherers as a map, so the predicate namespace is explicit. The
fields a schema declares sit under `fields:` in the frontmatter, below the
envelope keys — `id`, `type`, `title`, `links` — so that a domain field called
`title` or `links` cannot collide with the one around it. See `decision/0010`.

```yaml
fields:
  gather:
    build:    { target: epub }
    chapters: { gatherer: text, scope: "chapters/**" }
  hold_when:  obs.build.ok and obs.chapters.hits == 0
```

If **any** declared gatherer fails to produce an observation, truth is
`unknown` — not evaluated against what did arrive. Declare only what the
predicate reads.

**A gatherer also declares what it returns**, and an observation carries those
fields and nothing else. Undeclared output is dropped rather than stored, which
is what keeps a permanent, tracked record from accumulating whatever happened
to be on a program's stderr. Raw output is kept only when a gatherer *fails*,
as a bounded and redacted receipt, because that is when it is needed.

## Drift

Drift is a query result, never a record. Nothing creates it and nothing resolves
it; it is computed on demand and never written down, so it cannot go stale,
cannot need invalidating, and cannot leak:

```
drift = artifacts where standing != none and truth != kept
```

The kinds people enumerate are the ways truth-seeking failed:

| | |
|---|---|
| never observed | `unverified` |
| observed, predicate false | `not-kept` |
| false now, true before | `regressed` |
| ratified content cannot be found | `absent` |
| the last answer is older than `max_age`, not yet re-observed | `stale` |
| gatherer could not run, or ran out of time | `unknown` |
| repairing one standing artifact broke another, twice | `conflicted` |

`conflicted` is a standing question, not an evidence question. More observation
will not settle it; it needs a ruling from whoever has standing over both.

**It is reached by exhausted repair, and that is the only way in.**
Incompatibility cannot be computed: two promises both `not-kept` are not in
conflict, two both `kept` may be, and deciding it from the predicates is
undecidable. What *is* mechanical is a repair that satisfies its own promise and
breaks another. The gate catches that and refuses the land; the next packet says
so under prior attempts; and a second attempt that does it anyway escalates and
marks **both** artifacts `conflicted` — both, because nothing says which of two
ratified statements should yield. The way out is release, or revise and ratify
again. See `decision/0037`.

## The loop

`kept` ticks once and exits. It has no scheduler and no daemon: a person runs
it the way they run `make`, and a watcher or a cron entry that drives it lives
outside kept.

**A cadence is due when its inputs moved.** The scope's tree hash is compared
against the tree the last observation named; a promise is due when they differ,
when it has never been observed, or when its last answer is older than an
optional `max_age`. There is no schedule to honour and nothing to keep running.

```
tick:                                        # seconds. no model runs here
  for each artifact whose scope moved, or whose answer aged out:
      obs   = gather(artifact.gather)         # on a deadline; timeout → unknown
      truth = evaluate(artifact.hold_when, obs)
      record(truth)

  drift = artifacts with standing and truth != kept

  for each drift, ranked:
      disposition = policy(drift)             # act | wait | suppress | escalate
      record(disposition)                     # including every refusal, and why
      if disposition == act and budget.allows() and lock.acquired():
          open a thread                       # the ranking, the budget and the
                                              # dispatch are not a tick's; see
                                              # below
```

**A tick that does nothing still says why.** Every refusal — the policy, the
budget, a lock another run holds, a ledger too far behind to act on, a rank
below the cut — is recorded against the artifact. None of them can be
reconstructed afterwards, and a queue whose silence cannot be explained is not
one to trust.

**A tick never dispatches, and its disposition says so.** A tick verifies
nothing and starts nothing, so it refuses on `provider` — permanently, and by
design rather than as a placeholder left behind. What opens a thread is a
person: `kept repair <promise>` repairs one promise, named. Choosing which of
several drifts to spend a live session on is a ranking and a budget, neither of
which is built, and a tick is what runs in a hook and in CI — where re-deriving
this repository's own promises already shells a toolchain over its own suite.
The seam is kept open rather than filled: what the tick's one decision point
grows is the policy and the budget above, not a dispatcher. See
`decision/0060`.

```
thread(one promise a person named):
  box     = worktree at the workspace's own commit  # a different world
  before  = gather + evaluate, in the box        # the gap, measured where the
                                                 # agent will stand. it holds
                                                 # there → nothing is dispatched
  packet  = statement + why + one measurement per instrument, keyed by the name
            the promise gathers it under + scope + prior attempts + the policies
            checked before a dispatch            # recorded as given, not rebuilt

  ── repair: dispatch an agent into the box ──   # the only unbidden writer. what
                                                 # kind of work this is travels
                                                 # beside the packet, never in it

  after   = gather + evaluate, in the box        # did it work on its own terms.
                                                 # this is the verdict, and never
                                                 # the agent's account of itself
  review  = the box's changed paths, as git names them, against the repair scope

  present(diff, review, receipt)                 # ── kept repair stops here ──

  ── assess: an agent reads the ledger entry ──  # read-only

  land on a human act, or a policy with a deadline

  gate: merge onto origin/main@X and gather      # X is origin/main as the gate
                                                 # saw it. did it work where it
                                                 # counts — for this promise, and
                                                 # for every standing artifact
                                                 # the diff touches
  ── land ──  under a lock: is origin/main still X?
              still X   → push
              moved     → re-gate, unless the paths that moved are outside scope
              broke another standing artifact → refuse; twice → conflicted
```

**Everything above `present` runs.** `kept repair <promise>` cuts the box,
measures the gap in it, assembles the packet, verifies the jail before a session
exists, dispatches a confined agent, re-derives the promise from the tree that
session left, and prints the diff with the scope review beside it. **Nothing
lands**: the box is discarded, the working tree is never written, nothing is
committed and nothing is pushed. The assessor, the gate and `land` are what is
left.

Landing has a destination: the change belongs on `main`, and `main` belongs on
`origin`. Git makes the push atomic, so the lock is not there to keep two
pushes apart — it is there so that **the tree that was gated is the tree that
lands**. Two clean merges of separately gated changes produce a tree nobody
gated, and nothing would report it.

## What the agent may touch

Three things write. The operator writes content, and performs the acts that
create standing. A run of `kept` writes the ledger, and never writes content
unbidden. The repair agent writes one box's working tree — and it is the only
writer that acts **without a person deciding what it writes**, which is why it
is the only one that needs bounding.

Three boundaries bound it. They are different mechanisms and none substitutes
for another.

| | bounds | how |
|---|---|---|
| the **scope** | what may land | out-of-scope paths are reported, never landed |
| the **box** | what reaches the repository | a worktree, and a gate that re-derives from the world |
| **confinement** | what the process may touch | writable paths, reachable hosts, credentials held, programs runnable |

A worktree is a directory, so the box confines nothing by itself: a process
inside one can still read every credential on the machine, reach any host, and
push to the origin. Confinement is configured separately and explicitly —
`unset` is not `none` — and a provider is verified by a canary that attempts
each escape and requires refusal, because a jail that claims to confine and
does not produces a record saying the work was contained.

Two of those escapes would void guarantees stated elsewhere. **An agent holding
push access to the origin** walks around the gate entirely. **An agent that can
run `kept`** can ratify, which is the one act reserved to a person. The canary
probes ten escapes in all — those two among them — each an attempt that must
come back refused, and a probe that could not be attempted at all counts as a
failure. Beside it a pre-flight asks the provider, before a process exists,
about every path kept declared; a denial refuses the dispatch by name rather
than failing obscurely from inside.

**Confinement is an outer bound, never the report.** A scope is a glob and a
filesystem grant is a path, so the grant rendered for `internal/gather/**/*.go`
is the directory `internal/gather`, and there is no finer expression of it; a
scope whose first segment is a wildcard grants the whole box. Measured in a real
box: a write the grant admits and the glob does not lands, and so does a delete.
So what reports is the box's own change — the paths git names, never the diff
text, because a delete's header names `/dev/null` — classified once against the
scope, which is the same computation a land will refuse on. The grant is printed
beside the review, because a reader of a bounded review should be able to see
how much wider the rendering was. What the jail *refused* is not reported at
all — the clause is about work that stays in the box, and a refused write is
work that never happened. See `decision/0059`.

A box reaches a little past itself, and every path it reaches is named in the
profile and asked about before the dispatch: its own temporary directory beside
the worktree, the runtime's home — a projection of the operator's configuration,
discarded with the box — and a toolchain's caches when the promise's own
instrument names a toolchain.

**The credential is brokered, so the box holds a phantom.** kept assembles a
reference the provider resolves and never reads the secret itself: the provider
publishes a session phantom in the variable the profile names and rewrites the
header on the way out, under a default-deny endpoint allowlist. The network is
blocked unless a host is named, and naming one runs a proxy rather than granting
a socket. kept cannot redact what it never held — a session that echoes its own
environment puts the phantom in its receipt — and that is a smaller exposure
than handing the box a live token, not none. See `decision/0057`.

**A runtime is admitted per adapter, on live confined evidence**, and the set of
adapters is closed. Two of the three are admitted — each by a session that ran
in a box and by one deliberately interrupted in it — and the third is written
down and refused, so nothing dispatches it. A runtime told nothing about the
work it has been given reads a packet as a question and answers it: measured
across eight confined sessions, every one with a clean box located the fault
from the packet alone, wrote a correct account of why the promise did not hold,
and stopped without editing anything. So what kind of work this is travels
beside the packet: a value from a closed set, byte-identical for every packet,
naming no promise, no file and no evidence. See `decision/0058`.

**How to repair is free. Whether to open work is compiled.** Enumerating
repairs was the mistake in the predecessors; the packet states the gap and the
agent decides what to do about it.

## The acts

Twelve, and who may perform each. The three human-only ones are the whole of
the authority model.

| Act | Who | What it does | Recorded as |
|---|---|---|---|
| author · revise | a person, or an agent by proposal | writes or changes content. Free, reversible, grants nothing | a content version |
| **ratify** | **a person** | grants standing to a content version. Required again after a revision | an act |
| delegate | the system | passes standing from a promise lacking truth to a unit of work | a work item |
| gather | the system | runs a gatherer; returns the fields it declared, of a named world | an observation |
| evaluate | the system | applies a predicate to an observation | a truth transition |
| repair | an agent | changes the world, inside a box, inside a scope, inside a jail. One promise, named by a person | a receipt: the runtime's own events, a transcript, cost |
| assess | an agent | reads a thread's ledger entry and reports findings. No write access | findings, per check |
| **land** | **a person**, or a policy with a deadline | puts a gated diff on `main`, and `main` on `origin` | an act |
| **release** | **a person** | retires standing. A promise stops being held to | an act |
| promote | the system, or a person | moves a fact to a wider scope. Still a fact, still no standing | a copied fact |
| refuse | the system | declines to act — budget, lock, staleness, policy, rank — and says which | a disposition |
| record | the system | appends to the ledger, after the act and never before it | — |

Ratifying and releasing may not be delegated. Landing may, to a policy with a
deadline, and that is the only one of the three that bends.

The diff a repair produces is printed and not recorded. A receipt is runtime
evidence and a lifecycle its parent observed; a diff is a derivation over a box,
and what a land was gated against is a record `land` will owe rather than one
`repair` writes.

## The assessor

The predicate answers *is the promise kept now*. It does not answer *was this a
legitimate way to get there*. A second agent reads the ledger entry for a
thread and works an explicit checklist:

- did the diff touch the gatherer, the predicate, or the promise itself?
- was the predicate already satisfied before the repair — a no-op claiming credit?
- does the observation show the property exercised, or a vacuous pass?
- are the touched paths in scope, and proportionate to the statement?
- does the transcript show the agent optimizing for the predicate rather than
  the statement?
- did the repair touch evidence that other promises read?

Each check is recorded on its own, not collapsed into a verdict: which one
fired is the whole diagnostic value. The transcript it read is durable and
tracked, because re-running the agent does not reproduce it — it produces a
different session, at a different cost, with a different diff.

Framed as an attack, not an evaluation. An assessor asked whether criteria are
met will find that they are.

**Verification attaches to acts, not to instruments.** An act is bounded, has a
ledger entry, and is where a change happens. Verifying an instrument is
unbounded and never finishes.

**None of the assessor is built.** What checks a repair today is the
re-derivation and the scope review, and neither reads a word the session wrote.
What does exist is the assessor's evidence: a thread receipt written
incrementally from the first event, so a session killed at its deadline still
leaves its partial transcript, its cost, the cause of the timeout and the signal
it was reaped with.

## Autonomy

The gatherer sets the gate, so the grade is per promise and derived rather than
chosen:

| Gatherer kind | Gate |
|---|---|
| structural — `corpus`, `entities`, `fold`, `units`, and the grouped `go/tests`, `go/imports`, `go/symbols` | auto-land, inside scope |
| pattern — `text`, `path`, and a grouped `<group>/lint` | assessor must pass |
| `judgement` — a model | assessor, plus a spot check |
| `reading` — a person | you, and the truth decays back to `unknown` |

Where a promise names several gatherers, the **weakest** sets the gate.

**A name says what the tree must have.** A bare name works anywhere and needs
nothing installed — `command`, `corpus`, and the neutral kinds above as they are
built. A name carrying a group needs whatever the group names: `go/` is the Go
toolchain, and a future `rust/` or `art/` is whatever those require. The group
is one segment and it is in the name because `gather` binds, so a promise's own
text says what it needs rather than leaving a reviewer to find out by running it.
A gatherer kept did not write is `x/…`, and kept makes no claim about the shape
after the marker. See `decision/0051`.

Nothing about a group is a demotion: a grouped built-in is still kept's, still
vouched for, and still sets a structural gate. What cannot is a third party's,
and that is about authorship rather than about what it measures.

**Nothing auto-lands yet**, so the grade above is what a landing gate will read
rather than something running now: a repair ends at a diff a person merges,
whatever its promise measures with. The catalogue built so far is `command` and
`corpus`, which need nothing installed, and `go/tests`, `go/imports` and
`go/symbols`, which need the Go toolchain and say so. `command` runs a program a
promise names and has no row in the table; nothing has needed one, because
nothing lands unattended.

## What this does not do

Trust levels and altitude: ways of saying standing is partial and earned. There
is one principal. Add them as attributes on the ratification act if a second
appears.

Incremental invalidation and durable execution: answers to scale problems a
single corpus does not have. A crash re-derives from the corpus and the ledger.
Nothing is recorded as an intention, so a crash can lose the record of an act
and can never claim one that did not happen.

A scheduler, a daemon, or a coordination service. `kept` ticks once and exits.
There are four nouns — a **run** is one execution, a **thread** is one repair,
a **box** is a tree acquired to work in, a **world** is the tree a measurement
was made against — and none of them is a long-lived process.

Field-level merge and leader election: not because writes are serial — several
runs of `kept` execute at once — but because the storage layout means no two
writers ever choose the same file. See `decision/0011` and `0012`.

Locking that anything depends on. Two runs may repair the same drift; the
second one's gate finds the promise already kept and it lands nothing. The
lock that avoids the duplicate is there to save an agent dispatch, not to make
the outcome right. See `decision/0013`, and `0015` for the one place — between
the gate and the push — where a critical section is load-bearing.

A repair library: enumerating repairs was the mistake. The one thing a runtime
is told beyond the packet is what kind of work it has been given — a value from
a closed set, identical for every packet, naming no promise, no file and no
evidence — and a second kind of work would be a second value beside the first
rather than a template anyone can fill in.

## Known limits

**A promise can be the wrong proxy.** Every mechanism here can be working while
the promise measures something you did not care about. The assessor can see a
substituted instrument; it cannot see a badly chosen one. `why` is the only
thing aimed at this, and it is unmeasured.

**Assessor and repairer share weights.** A substitution one finds reasonable,
the other may too. The named checklist helps; it is not a proof.

**A contradiction between two unchecked artifacts is invisible.** Two ratified
canon entries saying Kael's eyes are green and brown are the case `conflicted`
brings to mind, and nothing here detects it: neither has a gatherer, so neither
has a truth, and there is only text. Conflict is found by repairing one artifact
and breaking another, so a contradiction nobody observes surfaces only when a
promise about that prose fails — at which point it is found the ordinary way.
Comparing statements for contradiction would be a model call deciding a derived
state, which is what `decision/0009` keeps out of the compiled loop.

**The ledger's integrity is conventional.** Append-only streams merge by union,
so anything that can write the ledger ref can add a line and nothing can tell a
forged ratification from a real one. The mechanisms that make that hard are
layout, not proof: the ledger is on a ref no box checks out, `.kept/**` is out
of scope for every repair, and `kept` is not runnable inside a box. Signed
ratifications are specified and deliberately not built, because the adversary
they answer — a second principal — does not exist yet. What is detected is loss
rather than forgery: the ref is logged, and a value it can no longer reach makes
every verb refuse until a person unions it back.

**The predicate language had not been built before, and is now frozen.**
Everything else here had run somewhere; a small expression language over
structured observations had not. It is built, and what was a cost estimate is a
commitment: `hold_when` is authored in the corpus and bound by the promise
schema's hash list, so an operator or a type added to it now cannot be taken
away without re-ratifying every promise that has one. A builtin is added when a
promise needs it and never in advance. See `decision/0036`.

**An agent may not be able to check its own work.** What puts a toolchain in a
box is the promise's own instrument name: `go/…` resolves the Go toolchain and
grants its caches, `command` and `corpus` resolve nothing, and there is no
configuration an operator can use to grant one. The sessions that admitted the
Claude adapter ran in boxes with no compiler at all. A session's own account of
a clean build is worth nothing either way — the verdict is the re-derivation,
and a measured session that reported a clean build and a passing suite was
recorded unrepaired, because the promise still did not hold in the tree it left.
Where Go is granted the tree builds, tests and vets in a box, with one measured
exception: cgo does not link there, because Apple's `/usr/bin` wrappers find the
per-user temporary directory through `confstr` rather than through `TMPDIR`.

**Work the jail refused cannot be reported at all.** Measured at every surface
the provider offers — its audit trail, its stderr, a trace log: a denied write
leaves no record anywhere, because on macOS the kernel enforces and the provider
is not in that process. Everything reported about a repair is therefore about
work that happened.

**The writable set is always wider than the scope it stands for.** It is derived
from the promise's repair scope, but a scope is a glob and a grant is a path,
and a scope beginning with a wildcard grants the whole box. Narrowing it to the
files a glob matches now would refuse a repair its own new file, so the width is
printed beside the review rather than closed.

**kept's own confinement tests cannot be re-derived inside a kept box, so one
promise is checked every tick and repaired by nobody.** They start real confined
children, so running them in a box asks the provider to sandbox a process that
is already sandboxed — and the larger half is where their evidence has to live:
they cut their boxes and plant their canary sentinels under the operator's home,
because the provider's baseline grants nothing there, and a repair box must not
grant a confined agent that place. The limit is now named rather than pending.
`promise:core-packages-are-tested` measures every package under `internal/`
except `internal/confine`, and `promise:the-confinement-suite-passes` measures
that one on the same scope and the same cadence with no `repair` block — which
is the whole of the mechanism, since a promise that bounds nothing authorizes no
repair. Nothing stopped being checked; what cannot happen is the loop closing
this one. See open question 11.

**Confinement is measured on macOS, with one provider, for Go.** The baseline
policy a box rests on is the provider's rather than kept's, the temporary roots
and the loopback capability are platform data nobody has measured elsewhere, and
the native-image check is Mach-O. On a platform with no verified profile the
provider stays unset, and `unset` is not `none`: a run refuses to dispatch.
