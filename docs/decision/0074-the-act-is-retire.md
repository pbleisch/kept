---
id: 0074-the-act-is-retire
type: decision
title: 0074 — The act is retire, and the ledger keeps the word it was written with
fields:
  statement: >-
    The act that ends standing is retire — the verb a person types and the value
    a record carries. Nothing writes released again; the fold reads it as the
    historical spelling of the same act and normalizes it, so exactly one word
    reaches everything downstream; and the one released act already in the ledger
    is left exactly as it was written, because it is the record of something that
    happened.
links:
  - { target: decision:0039-the-ledger-records-acts, relation: amends }
  - { target: decision:0032-standing-is-a-fold-over-acts, relation: amends }
---

# 0074 — The act is retire, and the ledger keeps the word it was written with

## Context

`release` was never the word. Four independent places explain the act by
reaching for the other one: `ledger.go` — *"Release retires standing"*;
`record.go` — *"Released retires standing"*; `register.go` — *"a person retired
it"*, over a field already named `retired`; and
[0032](0032-standing-is-a-fold-over-acts.md) — *"A release retires the artifact,
not a wording"*. When four places explain a name by using a different name, the
second one is the name.

Two things make it worse than a preference. `release` already does two other
jobs in this repository — software releases, and dropping a lock — so the word
cannot be grepped without disambiguating by hand. And it can be read as the
**opposite** of what it means: *released* suggests shipped, or let into force,
which is exactly what the act is not. In a system whose subject is unambiguous
standing, a verb that reads backwards is a defect.

**The window `desire-pipeline.md` costed has partly closed, and that is what
this decision is actually about.** It measured the rename on 2026-08-25 at *120
acts, all `ratified`, zero `released`* and concluded the wire value was free.
Today the ledger holds **128 acts, one of them `released`** — for
`decision:0070-first-of-a-pair`, on 2026-08-25. So *no durable record holds the
old name* is no longer true, and the question is no longer whether to rename but
what happens to that one act.

**That act is the record of a real event.** An artifact was ratified into this
ledger by mistake, and it was released to undo that. Both halves are in the
stream, in order, with the person who performed them named. It is the only
worked example this repository has of a mistaken grant being handled, and it is
evidence of the thing the append-only design exists to preserve.

[0039](0039-the-ledger-records-acts.md) is what has to be amended, and its own
reason is what expired. It renamed the stream and the type and deliberately left
the wire alone: *"The envelope keeps `v: 1`, the kinds stay `ratified` and
`released`, and every field keeps its JSON name — because no record's shape
moved, **and because there are no records**."* The second clause was load-bearing
and it is now false.

## Decision

**The act is `retire`: the verb a person types, the value a record carries, and
the word every derived surface prints.** `kept release` is gone as a verb and
refused by name, pointing at `kept retire`. `ledger.Retired` is `"retire"`,
`register.Retired` is `"retired"`, and `kept status --state retired` is the
filter.

**Nothing writes `released` again, and the fold reads it.** `released` becomes a
named historical constant beside the current one, resolved at the boundary where
a line is read into memory, so exactly one kind reaches `actWhy`, the projection,
the register, the exit codes and `--json`.

**The one existing act is not rewritten, and the ledger is not force-updated.**
A side ref can mechanically be replaced and that is not the question. The
question is what the refusals rest on, and they rest on this — from the guard
that admits a landing:

> *"the ledger is append-only and a malformed act discovered later is a permanent
> one: the only moment anything can be prevented is the write."*

Every refuse-at-write rule in this system is bought with that sentence. Editing
history once to tidy a word establishes that history is editable, which is the
premise those refusals deny; and the specific bytes it would tidy are the record
of a mistake being handled, which is the last thing to make disappear. The
ledger keeps the word it was written with. This decision is the explanation a
reader who finds it is owed.

### Normalized on read, not kept distinguishable

The two spellings collapse to one meaning at the fold. The alternative — a
second `Kind` constant carried the whole way down so a reader can tell which era
an act is from — was considered and declined for three reasons.

**One meaning must have one word downstream.** Two constants for one act means
`actWhy`, `projection`, `register`'s fold, the `--state` vocabulary and `--json`
each grow a second case, and every one of those is a place the two can come to
disagree. That is the failure this repository names in its own first paragraph —
*a rule written in two places is a rule that can disagree with itself* — and the
same reason `gather.refuseUnrunnableSpec` and `ledger.landingWhy` are each one
function asked at both ends.

**The distinction could not survive anyway.** `register.State` is derived, and it
has to be one value: a `--state` flag admitting both `released` and `retired` for
one state would be two words for one answer, which is exactly what
[0071](0071-a-listing-verb-narrows.md) refuses of a filter vocabulary. So two
`Kind`s would buy a distinction that dies one layer up regardless.

**Era is already recoverable, twice.** Every act carries `at`, and the bytes on
disk are never rewritten — so the archive still says `released`, and
`git show refs/kept/ledger:ledger/acts.jsonl` is the answer to *which era*. What
normalization changes is what the code reasons with, not what the record says.

The explanation goes where a reader will actually be standing when they need it:
`released` survives in the source as a named constant whose comment says which
act it is, when it was written, and why it was left alone. A reader who greps
the old word lands on the answer rather than on archaeology.

### The envelope version does not move

`v: 1` guards the record's **shape**, and the shape has not moved: same fields,
same names, same types. A kind value this build does not know is already
quarantined rather than guessed at — `actWhy` returns *kind is not an act this
build knows* — and a quarantined line takes the run to exit 3, *no evidence
either way*. So an older reader meeting a `retire` act reports that it could not
decide, which is the honest answer and the one the three-valued model exists to
produce. A version bump would buy a second way to say what the kind check
already says.

### The corpus keeps its own words, and 0039 is amended rather than edited

Ten ratified decisions use `release` for this act, several inside the section
that binds. **None of them is edited.** A decision records what was decided
then; rewriting its operative wording drops its standing and asks a person to
re-grant something they already granted, for a word. This decision is the record
of the new answer, and it names 0039 and 0032 as what it amends, which is the
mechanism this repository already has for exactly this.

So the corpus and the ledger tell the same story in the same way: **what
happened keeps the words it was written with, and everything from here is
`retire`.** One act in the ledger and ten decisions in the corpus are the record
of a vocabulary that changed, in a system whose whole subject is that the record
of what was decided is not revised in place.

## Consequences

**Two words for one act are readable in this repository, permanently.** A reader
of 0013, 0021, 0027, 0032, 0037, 0039, 0044, 0046, 0048 or 0063 finds `release`
and has to reach this decision to learn it is `retire`. That is the accepted
cost, and the alternative was ten re-ratifications for a rename.

**An operator's muscle memory breaks once, loudly.** `kept release` exits 2 and
names `kept retire`. It is a refusal rather than an alias, because an alias is a
second name for one thing and this decision exists to remove one.

**`README.md` had to change in the same commit, and a promise is why.**
`promise:the-readme-names-every-verb` reads the dispatch, so renaming the verb
took it to `not-kept` until the README said `kept retire`. That is the promise
working, and it is the first time a rename in this repository was caught by
something other than a person remembering.

**The historical constant is dead weight that must never be deleted.** It is
referenced by nothing but the normalizer, so it looks removable, and removing it
would silently un-retire `decision:0070-first-of-a-pair` — the act would
quarantine, the fold would not see it, and an artifact a person retired would
read as ratified. `TestTheOneHistoricalReleaseActIsStillReadAsARetirement` is the
guard, and it breaks that property on purpose.

## Alternatives

**Rewrite the one act and force-update the ref.** Mechanically easy, and it is
the option the captain raised and rejected. It would leave one vocabulary, one
constant and no normalizer. It costs the premise every refuse-at-write rule in
this system is justified by, and it costs the evidence that a mistaken
ratification happened and was handled — which is the only worked example of that
the repository has.

**Keep the wire value `released` and rename only the verb and the identifiers.**
Free, and it puts the disagreement in the one place that can never be revised:
the durable record would say `released` forever while every verb, document and
identifier said `retire`. That is the defect this decision exists to remove,
reinstated at the bottom of the stack.

**Bump the envelope to `v: 2`.** Honest, and it says *this vocabulary is the
second one*. Declined because the shape did not move and the kind check already
refuses what it does not know, safely. A version that meant *some values changed*
would make every future value change a version bump, and the envelope would stop
meaning shape.

**Do nothing.** The strongest argument for it is that the window closed on
2026-08-25 and the price is no longer zero. But the price is now one normalizer,
one historical constant and one decision, and it does not fall again — every
further act written as `released` makes the eventual rename cost more, and the
argument for the word has not weakened.
