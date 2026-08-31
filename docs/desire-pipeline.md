# The desire pipeline

A proposal. Nothing here is decided, and the ADRs it asks for are named at the
end.

## Why this belongs in kept at all

Not organization — a tracker organizes fine. It belongs here because **kept is
the only thing that can tell you, before you commit, whether a proposed
commitment is measurable and what it would say today.** That is the shaping
work, it is what no tracker does, and the pipeline should be built around it.

That framing also fixes the observed failure. `docs/desire/` has been empty
since it was created, which says a directory that is only a place to put wants
does not get used. It has to pay before ratification, the way
[`self-hosting.md`](self-hosting.md) argues the policies did.

## What a desire is, and why an agent may write one

A desire is the one authored kind with **no route to either property**. No
standing — nothing ratified it. No truth — nothing observes a want. That is not
a gap; it is what makes the pipeline safe, and it answers the question anyone
should ask before letting an agent write into a corpus:

> **Content grants nothing.** An agent can write a hundred candidate promises
> and nothing changes: `kept status` reports them unratified, `kept drift`
> never sees them, no packet contains them.

The risk was priced in by the first property. The pipeline needs no new
authority mechanism. It needs a way to make a want shapeable, and a way to make
the shaping checkable.

**The one thing it must not become is a work tracker.**
[0008](decision/0008-work-is-not-an-entity.md) is unambiguous: nobody authors a
work item, so work is derived and lives in the ledger. A desire is authored, so
it is corpus. But a desire that grows an assignee, an estimate, or a state
somebody advances by hand has become the thing 0008 refused wearing a different
schema.

## Open question 6, closed: corpus

0008's own test settles it. Work is in the ledger *because nobody authors it*.
A desire is authored by definition, and shaping is weeks of editing prose, which
a ledger construct cannot support. The directory and the schema already exist.

## The four states are derived, not stored

`_schema/desire.yaml` carries `status: raw|shaped|committed|dropped`. **Drop the
field**, on [0041](decision/0041-a-decision-is-a-kind.md)'s precedent — a
decision carries no `status:` because accepted-ness *is* standing, and a second
place for the answer is the cache that can disagree with its source. The same
argument applies unchanged.

| state | how it is answered | |
|---|---|---|
| **raw** | it exists, and names no kind and no candidate | fold |
| **shaped** | it names the kind it would become, and a candidate that parses | fold |
| **committed** | a **ratified** artifact cites it | fold over corpus + register |
| **retired** | an act, and a trigger that could re-open it | ← the only one needing a decision |

`committed` keyed to *ratified* is the load-bearing part: a candidate promise
sitting unratified in `docs/promise/` leaves its desire `shaped`, not
`committed`. The pipeline cannot be advanced by writing a file.

For the fourth, **generalize the existing retire act rather than add a
thirteenth**. It means "retires standing" today; make it *a person retires this
artifact — for a kind that bears standing, the grant set empties*. Then
`kept retire desire:x --note "…"` needs no new verb and keeps the file. Deleting
the file is the tempting alternative and it is wrong for the reason a declined
backlog item is worth anything: without the record, somebody re-proposes it in
six months.

## Triggers: the condition that would change the answer

An earlier draft treated `retired` as the state that needed a decision and
otherwise as tidying up. A survey of `docs/**` says the opposite. **The
condition under which a desire's answer would change is the most valuable thing
written about it**, this repository has been recording those conditions
carefully in prose, and prose has no reader.

### Three ways to be "not now"

That earlier draft also handled only the first of them.

| | state | the condition is about | worked example |
|---|---|---|---|
| **declined** | retired | whether we *want* it | 0044's index |
| **not yet writable** | live | whether it can be *expressed* | *the pipeline cannot reach a packet* |
| **vacuous** | live, candidate parses and runs | whether it can *fail* | *the verdict does not read the agent* |

Only the first is a re-opening. The other two were never closed, which is why
one field will not obviously do — see the open call below.

### Four shapes, sorted by what can read them

**A measurement.** 0044 re-opens on *"a corpus where `kept drift` exceeds one
second"*; [`lineage.md`](lineage.md) retires incremental invalidation because it
*"pays above a register size one operator will not reach"*. Both are candidate
predicates, and the second is readable today — `corpus` declares `artifacts`, so
`obs.corpus.artifacts > N` is the whole trigger.

**An existence.** Something in the tree exists that did not.
[`self-hosting.md`](self-hosting.md)'s two deferred promises are both this
shape, and `go/symbols` and `go/imports` read them directly.

**An event.** 0043's *"whether the sketch survives its first plugin"*; 0044's
*"a verb that needs full-text ranking"*. Nobody can observe that somebody wanted
full-text ranking — that is an intent rather than a state of the world.
Permanently prose.

**An argument.** `lineage.md` retires evidence manifests because *"the predicate
covers it"*, so the trigger is *if it turns out not to*, which is a judgment.
Permanently prose, and correctly so.

The authoring rule that falls out: **a good decline names a number.** Of the
nine retirements already written down, the ones naming a threshold are the ones
a trial can read; the ones naming an intent are not. A decline that names no
number and does not claim permanence is usually one that has not been thought
through.

### The proof, observed 2026-08-25

Stated as history, because it has already happened and will have been fixed by
the time this is read.

`self-hosting.md` deferred two promises with a precisely stated condition —
*"Both become real in increment 5, alongside the code that could break them, and
that is when to write them"* — resting on `internal/thread` being empty and on
there being no packet assembly to check. Measured on that date:

```
internal/thread/    8 non-test files — claude, codex, packet, pi,
                    receipt, runtime, supervisor, doc
AssemblePacket      internal/thread/packet.go:102
predicate -> thread no edge. The promise would read kept, non-vacuously
```

**Both conditions had been met, and neither promise was written.** The document
still said `internal/thread` was empty and that nothing assembled a packet.

That is the whole argument for a trigger being a field. The condition was stated
exactly, by somebody who knew precisely what they meant, in a document that is
read often — and nothing re-evaluates a sentence.

### Where each half lives

**The reasoning goes on the act; the condition goes on the desire.** A `--note`
records why at the moment of retiring, which is history and does not change. A
condition is a claim about the future, edited as understanding improves — 0044's
*one second* is a threshold somebody may revise. History belongs in the ledger,
revisable content belongs in the corpus, and one field cannot be both.

The evidence that this is worth a field at all is that nine of them exist
already, scattered across three files: `lineage.md`'s *Deliberately not adopted*
is a retired-desire register in prose, every entry carrying **why** and several
carrying **what would re-open it**, plus
[0024](decision/0024-signing-is-designed-and-deferred.md)'s deferred signing and
0044's index.

### A condition is read exactly as a trial

Nothing new is needed to evaluate one. `kept shape --trial` on a retired desire
reads its condition instead of its candidate and prints whether it has fired.

That reuse is what keeps this inside the model rather than bending it. A desire
has **no route to truth**, and reading a condition must not open one: nothing is
recorded, nothing is keyed to a content version, and a fired condition obliges
nobody. It prints, and a person decides.

### Two limits worth pinning now

**No gatherer may read the ledger.** Confirmed against `internal/gather`, where
the `ledger.` references are types only, and `corpus` is documented as reading a
corpus root and never the ledger because data that moves while the world stands
still breaks 0034's dedup. So *"if a second principal appears"* — the condition
on both `lineage.md`'s trust levels and 0024's signing — is **permanently
unreadable**, though it looks like a clean count over `acts.by`. That is a
design boundary rather than an omission, and it belongs written down where
somebody would otherwise try to build it.

**Absence must not mean never.** Some retirements are permanent: durable
execution is retired because *"a crash re-derives from the corpus and the
ledger. There is nothing to resume."* That will not change. But an absent
condition and a permanent one are the same distinction as `unknown` against
*nothing has answered*, which `model.md` refuses to collapse one level up. So
`never` is written, and a retired desire carrying no condition at all is a
`doctor` finding — nothing rejects it, nothing else reads it, and kept does
nothing about it, which is 0050's three tests exactly.

### The open call: one field or two

Retired asks *what would make us want this again*; live-but-not-ripe asks *what
would make this writable, or able to fail*. Those are different questions.

**Two fields** — `reopens_when` and `ripe_when`. The states are derived and
mutually exclusive, so a desire carrying the wrong one for its state becomes a
checkable defect, which is this repository's own test for whether a distinction
is real. One field would mean two things depending on state, which is the shape
`Profile.TempEnv` refuses: *a variable with two sources has one value and two
policies*.

The counter is real and should be recorded rather than waved off: two fields is
two places a condition can live, and un-retiring a desire leaves the wrong one
behind.

### What a trigger must not become

A scheduler. A fired condition un-retires nothing and opens no work — it prints,
and a person decides. The moment a fired trigger writes something, a desire has
acquired a route to truth, and the model's claim about what a desire *is* stops
holding.

## Capture: make it nearly free

Capturing a want today means writing frontmatter, minting a slug, and knowing
`doctor --fix` exists. That is why the directory is empty.

```
kept want "repair should pick which drift to spend a session on"
```

Writes `docs/desire/<deterministic-slug>.md` with an id and a `want:` line, and
nothing else. Consistent with
[`policy:nothing-kept-writes-rewrites-what-a-person-wrote`](policy/nothing-kept-writes-rewrites-what-a-person-wrote.md):
`init` and `doctor --fix` already write content when a person asks, and a person
typing `kept want` is asking. Shaping is a separate act you may never perform.

## Shaping, and the half that needs no model

A shaped desire names the kind it would become and carries a `candidate`
mirroring that kind's binding fields — statement, `gather`, `hold_when`,
`repair.scope`. All of it inert, because `hash: []` means nothing in a desire
binds.

Then kept can do the thing no tracker can: **run the candidate against the
current world and print what it would say.**

```
$ kept shape desire:repair-picks-what-to-spend-a-session-on --trial
  becomes    promise
  statement  a tick that could act on several drifts records which it ranked first
  gather     tests → go/tests, scope internal/**
  predicate  parses. 2 builtins, no unknown functions
  trial      not-kept — the promise would be owed the moment you ratified it
```

Three things fall out, and the third is the payoff:

- **the predicate parses** and names a gatherer in the closed catalogue — an
  authoring error found before ratification rather than as `unknown` after it
- **the gatherer runs** on this tree, with no promise and no standing involved
- **the trial reads `not-kept`** — the want is real and unmet

And the inverse is the check worth building the whole thing for: **a candidate
that already reads `kept` is either done or vacuous, and you must say which
before ratifying.** That is a direct, mechanical attack on the hollow-coverage
failure [`lineage.md`](lineage.md) records — caught at the one moment it is
cheap, which is before a person grants standing. Nothing else in the system asks
*is this the right proxy* at a point where the answer changes anything.

**A trial is computed and printed, never recorded** — and not by convention. A
desire binds nothing, so there is no content version to key truth to. Same shape
as drift: a query result that cannot go stale because nothing wrote it.

### A trial reads history for free

That is right about the **verdict** and wrong by omission about the **gather**.
A trial's gather produces an ordinary observation, addressed by gatherer, args,
world and scope digest under 0034 — already recorded, already deduped, already
shared. And `model.md` says why that matters: *"Observations do transfer,
because they are about the world and know nothing about any promise. That is
what lets a strengthened predicate read history from before it existed."*

A candidate is a strengthened predicate that does not exist yet, so it can be
evaluated against **every observation the ledger already holds**, with no new
gather and no new record:

```
$ kept shape desire:the-verdict-does-not-read-the-agent --trial --history
  now       kept
  history   kept in 41 of 41 observations, 2026-08-19 -> 2026-08-25
            never read not-kept against any world this repository has been in
```

That is a far stronger vacuity check than *reads `kept` right now*: it is *a
check that has never fired is not a check*, made mechanical at authoring time,
before standing is granted. And it costs nothing.

Two honest limits. A candidate naming a gatherer nobody has run has **no
history**, and that absence is itself informative — the promise introduces a new
instrument, which is a larger commitment than reusing one. And it still does not
catch a candidate that *is* falsifiable and measures something nobody cares
about, which is the guard named as missing at the end of this brief.

The same verb answers the retired set. `--trial` on a retired desire evaluates
its condition instead of its candidate, and says whether what would bring it
back has arrived.

## The shaping session

The model half is a **second value in [0058](decision/0058-the-ask-is-declared-and-travels-beside-the-packet.md)'s
closed ask set**, which is what that decision anticipated: *a second kind of work
is a second value in the set and a second constant beside the first.* Not a
template, not a registry, not an extension point.

| | repair | shape |
|---|---|---|
| ask | `repair` | `shape` |
| input | a **packet** — ratified authority, measured evidence | a **brief** — the want, its `shaped_from`, the policies and canon a proposal must fit, the schemas, the promises that already exist |
| writes | the promise's `repair.scope` | **`docs/desire/<slug>.md`, and nothing else** |
| verdict | re-derivation in the box | none, and it says so |

Three things to hold firm.

**The scope is one file.** The session writes its candidate *into the desire*,
never into `docs/promise/`. Tightest possible bound, and it closes a hazard that
is otherwise real: an agent with write access to `docs/policy/` can edit a
ratified policy, which drops its standing and stops it binding — loud in
`kept status`, but it should not be reachable. One file, `hash: []`, nothing to
break.

**There is no verdict, and that gets stated rather than papered over.**
[`policy:the-agents-report-is-never-an-input-to-the-verdict`](policy/the-agents-report-is-never-an-input-to-the-verdict.md)
has no counterpart here, because there is nothing to re-derive. What replaces it
is that the output is inert: *a proposal obliges nobody until a person ratifies
it.* That deserves to be its own ratified policy, because it is the sentence the
design rests on.

**The ask must be framed to refuse.** A good shaping session should often
conclude *this want does not become a promise — it becomes a document, or
nothing*, by running `model.md`'s three-way test: if you cannot say in advance
which side loses when it and the world disagree, it is a document and it binds
nothing. A shaping run that never declines is not shaping, which is the house
convention — *a check that has never fired is not a check* — one level up. Make
"declined at least once" part of the admission evidence, as the interrupted
session is for repair.

One asymmetry to write down: **desires never enter a packet; ratified authority
does enter a brief.** A packet states what is owed and a want is not owed. A
brief states what a proposal must fit, and ratified rules are exactly that.

## Committing: no verb, deliberately

Promoting a shaped desire is *authoring a promise* and then *ratifying it* — two
acts that already exist, with the human one last. There should be no
`kept commit`, because bundling a file move with a ratification puts the one
irreducibly human act behind a convenience.

The promise cites back: `{ target: desire:x, relation: realizes }`. It costs
**zero re-ratifications** — `links` is not in `promise.yaml`'s hash list, so
adding a relation binds nothing.

## Renaming `release` to `retire`, and what it costs

> **Decided, and done differently from what this section assumed.** See
> [0074](decision/0074-the-act-is-retire.md). The window this section calls open
> had partly closed by the time it was acted on — the ledger held one `released`
> act, for `decision:0070-first-of-a-pair` — so the wire value was not free after
> all. It was renamed anyway: `retire` is written, `released` is read as the
> historical spelling of the same act and normalized at the fold, and the one
> existing record is left exactly as written. The ten re-ratifications costed
> below were **not** paid: 0039 is amended by 0074 rather than edited, so every
> ratified decision keeps the words it was decided in. What follows is the
> proposal as it stood on 2026-08-25.

**Recommended, and the window is open now.** Four independent places already
explain the act with the other word: `ledger.go` — *"Release retires
standing"*; `record.go` — *"Released retires standing"*; `register.go` — *"a
person retired it"*, and it already has a field named `retired`;
[0032](decision/0032-standing-is-a-fold-over-acts.md) — *"A release retires the
artifact, not a wording"*. When four places explain a name by reaching for a
different name, the second one is the name.

Two further reasons. `release` is doing two jobs in this repository already —
software releases (0017 uses it that way nine times) and lock release (0013,
0015, 0018, 0026) — so the word cannot be grepped without disambiguating by
hand. And it can be read as the **opposite** of its meaning: *released* suggests
shipped, or let into force, which is exactly what it is not. In a system whose
subject is unambiguous standing, that is a defect rather than a preference.

Measured against this tree, 2026-08-25:

| | cost | |
|---|---|---|
| **ledger wire value** | **free** | 120 acts, **all `ratified`, zero `released`**. No durable record holds the old name, so no reader must accept both |
| `--json` state value | **free** | 0027 makes it a compatibility surface, but there is no consumer and no v1 |
| policy statements | **zero** | none mentions the act |
| promise statements | **zero** | none mentions the act |
| link relations | **zero** | not bound |
| Go identifiers | ~34 | `ledger.Released`, `Ledger.Release`, `register.Released`; plus a small collision with `register.go`'s existing `retired` field |
| CLI, README, `AGENTS.md`, `model.md`, `layout.md` | mechanical | prose and one verb |
| **ratified decisions carrying the word in a binding section** | **10** | 0013, 0021, 0027, 0032, 0037, 0039, 0044, 0046, 0048, 0063 |

**Ten re-ratifications is the whole bill.** The hash does not know intent — a
rename inside `## Decision` drops standing exactly as a semantic change does,
and AGENTS.md's three-case table has no fourth row for a cosmetic edit. One of
the ten (0044) is a benchmark description you could defensibly leave alone,
making it nine and leaving two words for one act in the corpus.

Two of the ten are not cosmetic and should be read as revisions rather than
renames. **0039** pins the JSON name — *"`ratified` and `released`, and every
field keeps its JSON name"* — so renaming the wire value edits what it decided.
**0032** is the decision that defines the act.

**The window closes.** All of this is cheap only while no `released` act exists
in any ledger and nothing consumes the `--json` state. Both are true today and
both stop being true at the first `kept retire` and the first external consumer.
If it is going to happen it happens now, which is an argument for deciding it in
the same pass as the rest of this brief rather than later.

## Concrete deltas

**Schema** (`desire.yaml`): drop `status`; add `becomes` (enum over the kinds)
and `candidate` (map, unbinding); add `ripe_when` and `reopens_when` (each a
string — prose, `never`, or a predicate a trial can read); keep `want`,
`shaped_from`, `body: note`. `hash: []` unchanged, so every one of these costs
**zero re-ratifications**. Add `realizes` to the relations promise, canon and
policy allow.

**Verbs**: `kept want "<text>"`; `kept shape <desire> [--trial] [--history]`,
mirroring `kept repair` — box, brief, confined, present the diff, nothing lands;
`kept list --type desire` grouped by derived state; `kept retire` generalized.

**`doctor`**: a retired desire carrying no condition, and a desire carrying the
condition belonging to the state it is not in. Both pass 0050's three tests.

**ADRs, in order.** Numbers are claimed by whoever reaches `main` first, so
check what is free before writing.

1. *A desire is a corpus entity and its state is derived* — closes open question
   6, removes `status`, generalizes retire, and puts the re-open trigger on the
   desire while the reasoning stays on the act. Carries the rename and its count.
2. *A trial reading is computed and never recorded* — plus the rule that a
   candidate already reading `kept` must be justified before ratification.
   **Build this one first**: it needs no adapter work and it is where the value
   is.
3. *Shaping is a second ask and its output is inert* — extends 0058, requires a
   per-adapter live confined proof under 0052's gate, and adds the policy that a
   proposal obliges nobody until a person ratifies it.

## What this does not do

No ranking. Open questions 3 and 4 rank **drift**; ranking desires is a separate
question nobody has opened, and this must not quietly open it. No estimates, no
dates, no assignment, no dependency edges beyond links. Desires still never
reach a packet. A shaping session still cannot ratify — it is in a box, and
`kept` does not run there.

## The risk, stated plainly

This creates a surface for exactly the failure `self-hosting.md` warns about: a
corpus of plausible machine-drafted promises that exist because drafting them
became easy. *Do not write a promise you would not have written anyway* was
advice aimed at a person; aimed at an agent that can produce forty candidates an
hour, it needs teeth.

The trial reading is the teeth, and it is only half of them. It catches
**vacuity** — a candidate that is already true. It does not catch a well-formed,
falsifiable promise measuring something nobody cares about, which is `model.md`'s
own known limit — *a promise can be the wrong proxy* — arriving at a new surface
with a faster writer behind it. **There is no second guard proposed here, and
one is needed.** That gap is the strongest argument for building the trial
reading alone first and watching what the corpus does before any shaping session
exists.
