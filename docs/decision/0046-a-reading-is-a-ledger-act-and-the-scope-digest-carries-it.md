---
id: 0046-a-reading-is-a-ledger-act-and-the-scope-digest-carries-it
type: decision
title: 0046 — A reading is a ledger act, and the scope digest carries it
links:
  - { target: decision:0033-due-is-a-digest-over-the-scope, relation: amends }
  - { target: decision:0034-the-observation-record, relation: amends }
  - { target: decision:0039-the-ledger-records-acts, relation: extends }
  - { target: decision:0032-standing-is-a-fold-over-acts, relation: depends-on }
  - { target: decision:0024-signing-is-designed-and-deferred, relation: depends-on }
---

# 0046 — A reading is a ledger act, and the scope digest carries it

## Context

`model.md`'s autonomy table has always carried a `reading` gatherer — *a person,
and the truth decays back to `unknown`* — and it is the only time-based truth
transition in the model. The manuscript spike asked whether it fits, against the
machinery that exists.

**The decay fits and costs nothing.** All three states the deck asks for —
*nobody has read it · read and it works · read and it does not* — fall out of one
predicate over the language that already exists, with the gatherer omitting the
field for a revision nobody has read:

```
count(obs.read.items, c => true) > 0 and all(obs.read.items, c => c.ok)

nobody has read it (field absent)    unknown
read, and it works                   kept
read, and it does not                not-kept
one bad, one unread                  not-kept
one good, one unread                 unknown
```

That is `Expr.Extract`'s existing rule and `callAll`'s three-valued fold. There
is no new truth-record shape, no world-less record, and no second timer. A
decayed answer is not a verdict reached with no gather; it is an ordinary gather
that measured *nobody has answered*, which is what `unknown` means.

**The recording does not fit, and that is the finding.** `Tick.due` is
exhaustively: forced · nothing has answered for this wording · the gatherer does
not exist · this gatherer did not produce the last answer · the scope matched
nothing · the bytes in the scope changed · the answer is older than `max_age`.
And `workspace.Excluded` is `.git` and `.kept`. Put those together:

> A person reads chapter 12, records that they read it, and nothing kept
> measures has moved. The promise is not due, the last verdict is carried
> forward, and `kept` goes on reporting `unknown` until the chapter changes —
> which un-reads it — or `max_age` expires.

**Reading is the one act in the model that makes a promise more true and moves
nothing the tick can see.** Every other act either edits the tree or is a ledger
act the tick does not gate on.

### What 0033 claimed, and what is now true

0033 said what due-ness is computed from, and it said it in terms that admit
only the tree:

> Due-ness is a content digest over the scope, computed by kept, in process,
> over the working tree.

and, listing the edges, that the digest excludes `.kept/` **or the tick feeds
itself** — recording a truth transition changes the world, which makes every
promise due, which records more.

Both halves stand for every gatherer that exists. What is now true is narrower
than the first sentence and does not weaken the second:

> **A scope digest is a digest over the scope's bytes, and over a projection of
> the ledger that the gatherer *declared* — restricted to the acts a person
> performs.** A gatherer that declares nothing gets the digest 0033 defines,
> byte for byte.

The `.kept/` exclusion is untouched and is what makes the second half safe: the
ledger is still not part of `World.Tree`, still not walked, and still not
readable by a gatherer. What a gatherer may name is a *projection* the tick
computes and hands over, of streams the tick never writes.

### The narrow reading of 0034 that made this look forbidden

`AGENTS.md` states 0034's rule as *an observation's data holds nothing that moves
on its own* — no duration, no timestamp, no pid — and read loosely that forbids
a ledger-reading gatherer outright. It does not, and the difference is measurable
rather than rhetorical. A duration moves on **every tick of an unchanging
world**, which mints a fresh record each tick and stops dedup working. A reading
count moves **once per human act**.

The invariant actually at stake is one step along: **an observation's data is a
deterministic function of everything its address names.** So the fix is to make
the address name the ledger, not to keep the ledger out.

And the address already very nearly does. `Observation.Address()` hashes
gatherer, args, world, scope **and data**, so two `reading` observations taken
before and after a reading are two honest, distinct, non-colliding records
today:

```
before  sha256:88b5d5ff336777c2…
after   sha256:db4e33571464f729…
addresses differ: true
cache keys equal: true
  sha256:9bc74682141e40e3…
```

**What lies is `gather.cacheKey`** — the same preimage with the data left out —
whose own comment says why it is derived that way: *"Deriving it from the same
preimage rather than composing a string is what stops the cache and the address
becoming two answers to one question."* That construction asserts, in code rather
than in prose, that **`(gatherer, args, world, scope)` determines the
measurement**, and a ledger-reading gatherer falsifies exactly that and nothing
else.

## Decision

**A `reading` is recorded in the ledger, in the acts stream, as the
person-performed act it is. What makes a promise notice it is a digest over the
declared act kinds, projected to the artifacts in that promise's scope, folded
into the scope digest — not a new field in the observation preimage.**

### It is an act, because it is a ratification with a different verb

A reading is a person asserting something about a content version:
`(artifact, content_hash, by, at)`. That is `ledger.Act`'s shape, field for
field. `acts.jsonl` holds `ratified` and `released` today and gains one `Kind`
constant; the record shape needs nothing.

### The component is a projection, and the three near-misses all fail

| candidate | why not |
|---|---|
| a ledger head digest | moves on every truth record, disposition, observation and run. Every tick writes truth records, so everything is due every tick — **the tick feeds itself inside the address instead of inside the world** |
| a per-artifact stream version | same defect, narrower. Truth and disposition are per artifact and tick-written, so it still self-triggers |
| a digest over the whole reading stream | converges, but a promise about chapters 1–5 becomes due when chapter 40 is read |
| **a digest over the declared act kinds, projected to the artifacts in the promise's scope** | moves only when a person records an act about something this promise is about |

### The guard: only streams the tick does not write may be declared

The ledger's streams split cleanly, and the split is already visible on disk
(`.kept/ledger/`: `acts.jsonl` · `truth/` · `disposition/` · `observations/` ·
`runs/`). `acts.jsonl` is human-paced. Everything else is tick-written, and a
gatherer declaring a dependency on truth or disposition would gather, cause an
append, move its own digest, and be due again.

**So a gatherer may declare a dependency only on ledger streams the tick does not
write.** One rule, checkable at catalogue load, and it gets a test that
deliberately declares `truth` and asserts the refusal.

One accepted consequence: `ratified` is in the acts stream too, so ratifying
anything in a `reading` promise's scope makes it due once. Bounded by human
action, and cheap.

### It goes into the scope digest, and that is where the cost is not

`Tick.measured` maps `(gatherer, args)` to the recorded `Scope` string, and `due`
compares the current scope digest against it. The `Scope` string is *already*
what due-ness is driven by, so folding the projection into it makes
`scope.Unchanged(recorded)` catch a recorded reading with **zero new `Tick.due`
clauses** — the trigger is added by not adding a trigger. And `Scope` is already
in the address (0034) and already in `cacheKey`, so **both move together, for
free**.

### It costs every other gatherer nothing, byte for byte

Provided the projection is appended **only for a gatherer that declares it**.
`tests`, `command` and a future `imports` declare no ledger read, so the input to
their scope fold is unchanged, so their digests are unchanged, so their addresses
are unchanged and their cache still works. The address *shape* does not vary by
gatherer — the preimage is untouched — only the input to one string does, and
that already varies by gatherer, because `args` does.

Two implementation constraints, both with named failure modes:

- **The composition happens in `gather.NewRequest`, never in
  `internal/workspace`.** `NewRequest` is the single place `Scope` is set, and
  `internal/gather` already imports `internal/ledger`. Putting the projection
  inside `workspace.Scope` instead would make the ledger part of the world —
  which is 0033's `World.Tree` ceasing to be the tree, and tick-feeds-itself at
  the deepest possible level.
- **`Tick.due` re-derives the digest independently** and compares it to the
  recorded string. Two places would compute it and they must compose
  identically. Diverge one way and the promise is due forever; the other way and
  it is never due, which is the unsafe direction and is silent. **The fix is that
  `due` builds the request rather than re-deriving the digest**, so there is one
  computation and not two.

### No `obs:` bump — and 0034's consequence is narrowed to say so

0034's consequences say *"Changing what the scope digest covers is an `obs:`
bump, not an edit, because the digest is in the preimage."* That rule is right
for the change it was written about — 0033's ignore rules being refined, which
moves **every existing observation's digest** and makes every stored address
unreproducible. It does not reach a change that is opt-in per declaration and
moves no existing digest at all. **So the rule now reads: changing what the
scope digest covers *for a gatherer that already recorded one* is an `obs:`
bump.** Adding a component that only a declaring gatherer receives is not.

That narrowing matters because **the bump 0034 prescribes does not currently
work**, and this is a latent trap worth naming on its own merits.
`Observation.Address()` builds its domain from the build's `ObsVersion` constant,
never from the record's `V`:

```
obs:1 sha256:88b5d5ff336777c2…
obs:2 sha256:97a708142ffdf241…    (identical fields, version tag only)
Address() consults o.V at all: false
```

and `decodeObservation` rejects any record whose fields do not reproduce the
address it is filed under. So an `obs:2` build would read **every existing
observation as `Corrupt`** — actively rejected, not merely orphaned — unless
`Address()` is first made version-aware, which nothing today does. **`ObsVersion`
is a build constant used where a record field belongs.** The blast radius today
is seven observations, and truth records cite them by address in `from`, so every
citation would dangle. Small now, unbounded later, and exactly the shape 0034's
own context calls the one-way part.

### The carried constraint

**A reading stored in the chapter's own frontmatter is impossible** unless that
kind's schema binds `hash: [body]`, because writing `read_at: <hash>` changes the
file, which changes the hash it must equal. This holds whichever home a reading
is given, and it is stated here because this is where `reading` is specified.

### What this opens, and the one thing it makes dangerous

A ledger-reading gatherer is a capability rather than a patch for `reading`.
Things nothing today can express: the never-fired signal as a gate, which
[0001](0001-standing-and-truth-live-in-the-ledger.md) names as one of three
things the ledger makes possible and which nothing can act on; every ratified
decision has been re-read since it was last amended; nothing was landed against a
verdict older than *n*, which is an audit promise about kept's own conduct.

**It also reopens a refusal already accepted, and the reason changes rather than
the answer.** The proposed `corpus` gatherer was kept out of the ledger because
counting ratifications would move its data while the world stood still. Under
this decision that is no longer forbidden — it is **permitted on declaration**,
and the declaration is what makes the address name what the data depends on. The
refusal was right for a gatherer that does *not* declare it. Said here so a later
reader does not find two decisions that appear to disagree.

**Dangerous, and it is a model-level statement rather than an implementation
detail:**

> Today, ledger write access buys **standing**. Under this decision it also buys
> **truth**. A forged `read` act makes a promise `kept`.

`model.md`'s known limits already say that *anything that can write the ledger
ref can add a line and nothing can tell a forged ratification from a real one*,
and the model's opening says of standing and truth that *neither converts into
the other*. A ledger-reading gatherer makes ledger-write a route to both. Nothing
breaks today — the adversary [0024](0024-signing-is-designed-and-deferred.md)
defers signing against is a second principal, who does not exist — but **the
trigger for undeferring changes**. 0024 currently reads *when a second principal
appears*; it should read *when a second principal appears, or when a gatherer
reads the ledger*.

## Alternatives

**A tracked file inside the promise's scope**, which was the spike's own first
recommendation and is the option this replaces. It works today with no mechanism
at all: recording a reading moves the scope digest, the promise is due, the
gather runs, and nothing in 0033, 0034 or 0035 changes. It was rejected on one
row that is not a cost:

> **A repair agent can write a tracked file. It cannot write the ledger.**

`reading` is the one promise whose entire value is that a *person* did something.
A tracked file puts the evidence for it in the one place an automated repair can
reach, protected only by each promise remembering to declare a narrow
`repair.scope` — and `repair` is not in `_schema/promise.yaml`'s hash list, so
that scope can be widened on a ratified promise without dropping standing
([0045](0045-canon-binds-the-instrument-it-is-checked-by.md) reports it). The
ledger is protected by three structural mechanisms that already exist and are
already load-bearing for standing: it is on a ref no box checks out, `.kept/**`
is out of scope for every repair whatever a promise declares, and `kept` is not
runnable inside a box. **This is a soundness failure in the alternative, not a
cost**, which is why it outranks the alternative's advantage on every other
axis.

It also has a second problem worth recording: if that tracked file is a corpus
artifact it is a `fact`, and gating on it looks very like *letting a predicate
read facts*, which `AGENTS.md` forbids. The distinction is real — a tracked file
is the world under [0028](0028-the-corpus-is-not-the-repository.md), and the
gatherer measures *does a reading exist matching the content this file has now* —
but it is thin, and a rule that needs a paragraph to survive contact with an
invariant is a rule somebody will get wrong.

**A ledger act with the address left alone**, which is what the spike refused
first. Rejected: the cache key would assert that `(gatherer, args, world, scope)`
determines the measurement while it did not, so one key would serve two
measurements and the observation cache would return the stale one. That is the
failure 0014 exists to prevent, arriving through the one door 0014 had no reason
to check.

**A new field in the observation preimage.** Rejected: it needs `obs:2`, and the
section above is what that costs — seven observations read as `Corrupt` and every
truth record's `from` citation dangling. The scope digest already carries the
scope into both the address and the cache key, so a new field would be a second
route to a place there is already a route to.

**A ledger head digest, or a per-artifact stream version.** Rejected in the table
above: both self-trigger, because truth and disposition records are written every
tick.

**A digest over the whole reading stream.** Rejected as correct but coarse: it
converges, and it makes a promise about chapters 1–5 due when chapter 40 is read.

## Consequences

- **`Gatherer` gains a declaration of what it reads**, and the catalogue refuses
  a declaration naming a tick-written stream. That is the guard, and per
  `AGENTS.md` it gets a test that deliberately declares `truth` and asserts the
  refusal.
- **`ledger.Kind` gains one constant.** `acts.jsonl` is the spine and stays one
  stream; a reading is an act on it, beside `ratified` and `released`.
- **`Tick.due` gains no clause.** `scope.Unchanged(recorded)` already asks the
  question. What must change is that `due` builds the request rather than
  re-deriving the digest, so the two computations become one — and if they
  cannot be collapsed, that is the condition under which this decision should be
  reconsidered, because the divergence fails silently in the unsafe direction.
- **Every existing gatherer is byte-identical.** No observation is orphaned, no
  address moves, no cache is invalidated, and there is no `obs:` bump.
- **It works in a box**, which is not obvious. The gatherer never opens the
  ledger — the tick projects it and hands it over in the `Request`, exactly as it
  already hands over `Paths` and `Scope`. `kept` is not runnable inside a box and
  no box checks out `refs/kept/ledger`, so the projection is computed by the outer
  run against the tree the box holds. Nothing about
  [0016](0016-the-ledger-lives-on-its-own-ref.md) or 0023 moves.
- **0024's undeferral trigger widens** to *a second principal, or a gatherer that
  reads the ledger*, because ledger-write now buys truth as well as standing.
- **`prior()` still means the previous observation of that gatherer**, which is
  what 0036 says — but for a ledger-reading gatherer it now steps over ledger
  changes as well as tree changes, and someone will expect it to mean *the
  previous tree*.
- **`kept why`'s explanation changes shape.** *"The bytes N looks at have
  changed"* becomes, sometimes, *"an act it looks at was recorded"*, and that is
  a sentence a person reads and should not have to decode.
- **`ObsVersion` is a build constant used where a record field belongs**, and
  this decision does not fix it. It is not triggered here, and it is the thing to
  fix before anything ever does need an `obs:` bump.
- **Nothing is implemented by this decision.** No gatherer declares a ledger
  read, so building the mechanism before a promise needs it would be the
  speculative half 0036's bound refuses. What it costs to build is written down
  in the commit that carries this decision.
