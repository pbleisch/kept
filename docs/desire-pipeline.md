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

## Retiring carries a trigger, and that is half the value

An earlier draft of this treated `retired` as the state that needed a decision
and otherwise as tidying up. A survey of `docs/**` says the opposite: **the
retired set is the half of a backlog people actually consult**, because it is
what stops the same proposal arriving twice.

The evidence is that this repository already keeps one, in prose.
[`lineage.md`](lineage.md)'s *Deliberately not adopted* is a retired-desire
register: seven entries, every one carrying **why**, and several carrying **what
would re-open it** — trust levels wait on *a second principal*, the index on *a
verb needing full-text ranking, or `kept drift` exceeding one second*, and
incremental invalidation on *a register size one operator will not reach*. Add
[0024](decision/0024-signing-is-designed-and-deferred.md)'s deferred signing,
which waits on the same second principal, and there are **nine retirements with
a recorded trigger** already written down and scattered across three files.

So the trigger is a field rather than a note. Two things follow.

**The why goes on the act; the trigger goes on the desire.** A `--note` records
the reasoning at the moment of retiring, which is history and does not change. A
trigger is a claim about the future, and it is edited as understanding improves
— 0044's *one second* is a threshold somebody may revise. History belongs in the
ledger, revisable content belongs in the corpus, and the two must not be one
field.

**A trigger may be a candidate predicate, and it is read exactly as a trial.**
0044's is already measurable — `kept drift` over one second — and so is
`lineage.md`'s register-size bound. `kept shape --trial` on a retired desire
evaluates its `reopens_when` and prints whether it has fired.

That reuse is what keeps it inside the model rather than bending it. A desire
has **no route to truth**, and evaluating a trigger must not open one: nothing
is recorded, nothing is keyed to a content version, and a fired trigger obliges
nobody — it prints, and a person decides whether to un-retire. It is the same
mechanism as a candidate's trial reading, for the same reason, with the same
prohibition on writing anything down.

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

The same verb answers the retired set. `--trial` on a retired desire evaluates
its `reopens_when` instead of its candidate, and says whether the condition that
would bring it back has arrived.

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

**Schema** (`desire.yaml`): drop `status`; add `becomes` (enum over the kinds),
`candidate` (map, unbinding) and `reopens_when` (string — prose, or a predicate a
trial can read); keep `want`, `shaped_from`, `body: note`. `hash: []` unchanged,
so every one of these costs **zero re-ratifications**. Add `realizes` to the
relations promise, canon and policy allow.

**Verbs**: `kept want "<text>"`; `kept shape <desire> [--trial]`, mirroring
`kept repair` — box, brief, confined, present the diff, nothing lands;
`kept list --type desire` grouped by derived state; `kept retire` generalized.

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
