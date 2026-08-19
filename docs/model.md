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
ratified(promise:core-packages-are-tested, sha256:ab12…, norm:1, paul@…, at)
truth(promise:core-packages-are-tested, sha256:ab12…, world:9f31…, not-kept, obs #8817, at)
```

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

**Truth is keyed to a world as well.** A verdict reached in a repair's
worktree and one reached on `origin/main` are about different trees, so they
are separate series and are never folded together.

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
          open a thread
```

**A tick that does nothing still says why.** Every refusal — the policy, the
budget, a lock another run holds, a ledger too far behind to act on, a rank
below the cut — is recorded against the artifact. None of them can be
reconstructed afterwards, and a queue whose silence cannot be explained is not
one to trust.

```
thread(drift):
  box     = worktree from origin/main@X          # a different world
  packet  = statement + why + observation + scope + prior attempts
                                                 # recorded as given, not rebuilt

  ── repair: dispatch an agent into the box ──   # the only unbidden writer

  gather again, in the box                       # did it work on its own terms

  ── assess: an agent reads the ledger entry ──  # read-only

  present(diff, findings, cost)
  land on a human act, or a policy with a deadline

  gate: merge onto origin/main@X and gather      # did it work where it counts —
                                                 # for this promise and for every
                                                 # standing artifact the diff touches
  ── land ──  under a lock: is origin/main still X?
              still X   → push
              moved     → re-gate, unless the paths that moved are outside scope
              broke another standing artifact → refuse; twice → conflicted
```

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
run `kept`** can ratify, which is the one act reserved to a person.

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
| repair | an agent | changes the world, inside a box, inside a scope, inside a jail | a diff, a transcript, cost |
| assess | an agent | reads a thread's ledger entry and reports findings. No write access | findings, per check |
| **land** | **a person**, or a policy with a deadline | puts a gated diff on `main`, and `main` on `origin` | an act |
| **release** | **a person** | retires standing. A promise stops being held to | an act |
| promote | the system, or a person | moves a fact to a wider scope. Still a fact, still no standing | a copied fact |
| refuse | the system | declines to act — budget, lock, staleness, policy, rank — and says which | a disposition |
| record | the system | appends to the ledger, after the act and never before it | — |

Ratifying and releasing may not be delegated. Landing may, to a policy with a
deadline, and that is the only one of the three that bends.

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

## Autonomy

The gatherer sets the gate, so the grade is per promise and derived rather than
chosen:

| Gatherer kind | Gate |
|---|---|
| structural — `build`, `tests`, `entities`, `fold`, `units` | auto-land, inside scope |
| pattern — `text`, `path`, `lint` | assessor must pass |
| `judgement` — a model | assessor, plus a spot check |
| `reading` — a person | you, and the truth decays back to `unknown` |

Where a promise names several gatherers, the **weakest** sets the gate.

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

A repair library: enumerating repairs was the mistake.

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
they answer — a second principal — does not exist yet.

**The predicate language has not been built before.** Everything else here has
run somewhere. A small expression language over structured observations has
not, and it is where the cost estimate is weakest.
