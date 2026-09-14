---
id: 0083-a-desire-is-a-corpus-entity-and-its-state-is-derived
type: decision
title: 0083 — A desire is a corpus entity, and its state is derived
fields:
  statement: >-
    A desire lives in the corpus and carries no state of its own. Whether it is
    raw, shaped, committed or retired is derived on read, from what it says, from
    the ratified artifacts whose own links say they realize it, and from the acts
    naming it — and a later act wins over an earlier one, so a file edit never
    overturns an act. Retire applies to a desire, records the act and keeps the
    file. The reason for retiring travels on the act; the condition that would
    change the answer is written on the desire, in reopens_when for a retired one
    and ripe_when for one nobody has retired. kept want captures one with an id
    and a want line and nothing else.
links:
  - { target: decision:0008-work-is-not-an-entity, relation: extends }
  - { target: decision:0041-a-decision-is-a-kind, relation: extends }
  - { target: decision:0032-standing-is-a-fold-over-acts, relation: extends }
  - { target: decision:0074-the-act-is-retire, relation: amends }
  - { target: decision:0069-an-artifact-is-scaffolded-from-its-schema, relation: extends }
  - { target: decision:0050-doctor-reports-an-unchecked-instrument, relation: extends }
  - { target: decision:0030-a-link-manifest-is-one-file-per-subject, relation: depends-on }
  - { target: decision:0021-identity-is-authored-absence-is-not-an-act, relation: depends-on }
---

# 0083 — A desire is a corpus entity, and its state is derived

## Context

Open question 6 asked where desires live — a `desire/` directory in the corpus,
or a ledger construct like work — and it has been open since the corpus was
designed. The directory and `_schema/desire.yaml` both exist, and measured on
2026-09-13 `docs/desire/` holds nothing. `docs/desire-pipeline.md` reads that as
the finding rather than the gap: a directory that is only a place to put wants
does not get used, so it has to be cheap to write into and it has to say
something back.

Four things in the schema and the verbs stood in the way.

**The schema carried a state a person advances by hand.** `status: raw | shaped
| committed | dropped` is the field
[0041](0041-a-decision-is-a-kind.md) refused a decision: accepted-ness is
standing, and a `status: accepted` line is a second place that answer lives,
written by hand, where the one anybody reads is the one that is wrong. The same
argument applies to a want. And a desire whose state somebody advances has
become the work item [0008](0008-work-is-not-an-entity.md) refused, wearing a
different schema.

**Nothing could say a want had been declined.** `kept retire` refused any
artifact with no standing to end, which is every desire there will ever be,
because `hash: []` means nothing about one binds. So the only way to be rid of a
want was to delete its file — and a deleted want is the one somebody proposes
again in six months, because nothing records that it was ever turned down.

**The conditions under which an answer would change are written in prose, and
prose has no reader.** `docs/self-hosting.md` deferred two structural promises
with an exact condition, and on 2026-08-25 both conditions had been met and
neither promise was written. `desire-pipeline.md` records that as the proof. A
second instance is in `docs/lineage.md`: its *Deliberately not adopted* register
lists eight entries, and seven of them are declines. The eighth, *quarantine on
parse failure*, says "worth adopting later" and states what would make it worth
it — which is not a decline at all but a live want filed under one. Measured on
2026-09-13 it has been adopted twice over: the corpus reads an unreadable file as
a diagnostic and loads the rest, and the ledger quarantines a torn line and reads
the rest. The entry still says it was not adopted, and nothing could have said
otherwise, because a heading does not distinguish *we do not want this* from *we
cannot do this yet*.

**Capturing a want cost a schema's worth of knowledge.** Frontmatter, a slug,
and knowing which fields go under `fields:`. `kept new desire <slug>` removed
most of that and still needs a slug invented and a `--set`.

## Decision

### A desire is a corpus entity

0008's own test settles open question 6. Work is in the ledger *because nobody
authors it* — its content is derived from a promise and an observation. A desire
is authored by definition, and shaping one is weeks of editing prose, which no
ledger construct supports. It is linkable, greppable and reviewable in a diff,
and `thread.PacketInput` has no field a desire could arrive in, so living in the
corpus does not bring it one step nearer a packet.

What it must not grow is a field somebody advances. There is no assignee, no
estimate, no date and no state on a desire, and every state it has is computed.

### The four states are derived

`status` is removed from the schema. The state is worked out on read by the
register, from the corpus and the acts together, and nothing is written anywhere
to hold it:

| state | derived from |
|---|---|
| **raw** | it exists, and does not yet say what it would become in a form that reads |
| **shaped** | `becomes` names a kind, and `candidate` reads as that kind's fields with nothing wrong |
| **committed** | a **ratified** artifact's own `links:` say `{ target: desire:x, relation: realizes }` |
| **retired** | the ledger holds a retirement naming it |

**Shaped is the corpus reader's own judgment, applied to the candidate.**
`candidate` is the `fields:` block of the artifact the want would become, and it
reads when the corpus load would report nothing about it as the fields of an
artifact of that kind: the schema's required fields and types, a `max_age` that
parses, gather entries the catalogue can run as written, and a `hold_when` that
parses and reads only what those gatherers declare. It is one rule used twice
rather than a second one, so a candidate that shapes is a candidate that would
load. A raw desire carries the reason it is not shaped — which part is missing,
or what the load would have said about its candidate — and the reason is carried
to whoever reads its state rather than reported as a corpus problem, because a
draft is not a defect.

`becomes` admits `promise`, `policy` and `canon`: the kinds a person ratifies
whose fields a `candidate` map can hold. A document binds nothing, so nothing
could ever commit a want to one. A decision's binding half is a body section,
which a map cannot carry — its `candidate` would shape as `{}` — and a want that
is a choice about how something is built is already what an unratified ADR is.
Widening the enum later is an edit to a schema whose hash list is empty.

**Committed is keyed to ratified, and that is the load-bearing part.** A
candidate promise written into `docs/promise/` and not ratified leaves its desire
shaped. Writing a file cannot commit anything; a person's grant is the step.

Three narrowings keep that true:

- **Only the realizing artifact's own frontmatter counts.** A body wiki-link
  states no relation, and a `_links` manifest edge is a third party's assertion
  about an artifact it did not write — which
  [0030](0030-a-link-manifest-is-one-file-per-subject.md) and the invariant
  *only what an artifact's own author wrote binds* both exclude.
- **The realizing artifact's schema must admit the relation.** `realizes` is
  added to what `promise`, `policy` and `canon` allow. A relation a kind does not
  admit is a problem `kept list` reports, and a defect commits nothing.
- **The grant is the one in force now.** A realizing artifact revised since it
  was ratified obliges nobody, and a desire it cites is not committed by it.

### A later act wins, and a file edit is not an act

A desire can be retired while something ratified realizes it, and something can
be ratified to realize a desire after it was retired. Both are real, and the
register already has the answer for its own kinds: [0032](0032-standing-is-a-fold-over-acts.md)
folds acts in order, a retirement empties the set, and *getting back is `kept
ratify`, which is a person's act, and that is the whole route*.

So a retired desire is **committed** exactly when a realizing artifact's grant in
force was performed after the retirement, and **retired** otherwise. A person's
decline is overturned only by a person's later grant of something that realizes
the want. Adding a `realizes` link to a promise ratified last year does not do
it, because that grant is older than the decline.

Retiring a committed desire is allowed, and it takes effect. `retire` is never
refused where it changes what is derived — [0069](0069-an-artifact-is-scaffolded-from-its-schema.md)
calls it the way out — and what it leaves is readable: the retired desire still
names the ratified artifacts that cite it.

### Retire is generalized

The act means *a person retires this artifact*. For a kind that bears standing,
the grant set empties, exactly as [0074](0074-the-act-is-retire.md) and 0032
describe. For a desire, the want is declined. `kept retire desire:x --note "…"`
needs no new verb and no new kind of act, and it **keeps the file**: the record
of a declined want is what makes it worth having declined.

A retirement is refused only where it would change nothing a reader derives,
which is *record only what has already happened* applied to both directions: a
desire already retired, and a document or a fact, which have neither standing to
end nor a state a retirement could move.

### The reason is on the act, the condition is on the desire

**A `--note` records why, at the moment of retiring.** That is history, it does
not change, and the ledger is where history is kept. **A condition is a claim
about the future**, edited as understanding improves — 0044's *one second* is a
threshold somebody may revise — and revisable content belongs in the corpus. One
field cannot be both.

The condition is a string: prose, `never`, or something a trial can read. How a
trial reads one is the next decision's; this one only says where it is written.
**Absence is not `never`.** A retired desire with no condition at all has not
said whether it is permanent, and `never` is how it says so.

### Two fields, and the evidence that settles it

`desire-pipeline.md` left open whether the condition is one field or two. It is
two — **`reopens_when`** on a retired desire, *what would make us want this
again*, and **`ripe_when`** on a desire nobody has retired, *what would make this
writable, or able to fail* — and two things settle it that can be checked rather
than argued.

**The one register this repository keeps in prose already mixed them, and the
mixed entry is the one that went stale.** *Quarantine on parse failure* is a
ripeness condition filed among declines, and nothing noticed it had come true.
Under one field that is the resting state of every desire: a condition means one
thing or the other depending on a state the file does not say.

**The finding the proposal asks for can only fire with two.** A retired desire
carrying no condition is a `doctor` finding. Under one field, a want retired
while it still carried a ripeness condition satisfies that finding with the
wrong answer, and the check that exists to catch an unconsidered decline is
silenced by the leftover of a different question. With two, the leftover is
itself a finding.

The counter is real and is recorded rather than waved off: two fields are two
places a condition can live, and a transition leaves the wrong one behind.
Retiring a want does exactly that to its `ripe_when`. The answer is that the two
fields are what make it reportable — and the other direction, returning a
retired want to shaping, is not built here at all.

So `doctor` reports three things, under [0050](0050-doctor-reports-an-unchecked-instrument.md)'s
test — nothing refuses any of them, nothing else reads them, and kept does
nothing about them:

- a retired desire with no `reopens_when`;
- a retired desire still carrying `ripe_when`;
- a desire nobody has retired carrying `reopens_when`.

A committed desire is reported for neither: its conditions are the record of how
it got there, and nothing is owed on a want something ratified now realizes.

### What returns a retired want to shaping is not decided

The one way out of retired that this decision builds is commitment, and it falls
out of the fold rather than being added to it. A person who reads a fired
condition and decides to want the thing again, but not yet to commit to it, has
no act to say so: `kept ratify` on a desire grants nothing, because nothing about
a desire binds, and a new kind of act is what generalizing retire exists to
avoid. Writing a second desire is available and keeps the decline on the record.
Nothing yet forces a better answer, and it is recorded as an open question rather
than guessed at.

### `kept want` makes capture nearly free

```
kept want "repair should pick which drift to spend a session on"
```

writes `docs/desire/<slug>.md` holding an `id` and a `want:` under `fields:`, and
nothing else — no title, no comments, no body, no ledger record. It is 0069's
scaffold with the scaffold removed, and it is inside
`policy:nothing-kept-writes-rewrites-what-a-person-wrote` for the reason 0069 is:
a person typing `kept want` is asking for the file, the file is created
exclusively, and an existing one is left exactly as it is.

**The slug is a function of the text alone.** It is `schema.SectionID` — NFC,
lowercase, every run of what is not a letter or a digit to one hyphen — which is
frozen already under [0040](0040-a-sectioned-body-and-its-names.md), so the same
words produce the same slug from every build rather than from this one. It is
bounded at **64 bytes**, the longest slug a person has written in this corpus,
cut at the last hyphen that fits, or at a character boundary when one word alone
is longer.

**A collision is refused, never numbered.** Two wants whose words produce one
slug are the same want spelled twice, or near enough that a person should look
at the first before writing the second. So `kept want` names the file that
already claims the slug and writes nothing. A counter would make the slug depend
on what the corpus held at the moment, which is what *deterministic* rules out,
and it would hide exactly the re-proposal a declined want exists to catch.

A want with no letter or digit in it produces no slug and is refused, naming
`kept new desire <slug> --set want=…`. A want is one line; a line break or a
control character is refused, and the detail belongs in the note under the
frontmatter.

### Where the state is read

**`kept status`** prints a desire's state where it prints standing — a desire's
standing is only ever *nothing binds* or *retired*, which says nothing about the
want — and `--state` takes `raw`, `shaped` and `committed` beside the words it
already took, each admitting desires by the word printed for them. By default a
desire needs no attention, and the line that counts what was not listed names
desires by state. `--json` carries a `desire` object on a desire's record and on
no other.

**`kept why desire:x`** adds a section saying what became of it: the state, what
it would become, what realizes it, and for a retired want the note on the act and
`reopens_when` from the file.

**`kept list` and `kept show` do not.** They read the corpus and never the
ledger, which is what lets them answer against a ledger that has refused every
other verb, and committed and retired are both answers the ledger holds half of.

### What this costs in re-ratifications

**Zero.** `_schema/desire.yaml` keeps `hash: []`, so removing `status` and adding
`becomes`, `candidate`, `ripe_when` and `reopens_when` re-hashes nothing, and no
desire exists to carry any of them. Adding `realizes` to `links.allowed_relations`
in `promise.yaml`, `policy.yaml` and `canon.yaml` touches no hash list, because
none of the three binds `links`. Verified rather than argued, on 2026-09-13:
`kept status --all` gives every one of the 105 artifacts it read before the
change the same state after it — 103 ratified, 1 retired, 1 unratified — and the
one row it adds is this decision, never ratified.

## Alternatives

**A ledger construct, like work.** Rejected on 0008's own test above. It would
also make every edit to a want an act, which is weeks of acts for one paragraph.

**Keep `status` and validate it against the derivation.** A stored field checked
against a derived one is a cache with a consistency check, and the check is the
admission that the field is redundant. 0041 already refused this shape.

**Committed when any artifact cites the desire, ratified or not.** It makes the
pipeline advance by writing a file, which is what the whole of `committed` is
defined to prevent. An agent that can draft forty candidates an hour would commit
forty wants.

**Bind `links.realizes` in the hash lists of the kinds that realize, so that the
citation is part of the grant.** It would close the gap described in the costs
below. It re-ratifies every artifact of each kind at once — ten promises and
thirteen policies on 2026-09-13 — and it binds a relation that says which want a
commitment answers, not what is committed to, which is `max_age`'s shape: a
field about how the corpus is navigated rather than what is owed.

**Committed wins over retired, whenever both hold.** Simpler to state, and it
lets a link added to an old grant silently overturn a recorded decline — a file
edit undoing a person's act.

**Retired wins over committed, whenever both hold.** Also simpler, and it ignores
a later grant that explicitly says it realizes the want. A person ratifying that
is deciding to want it again, and the fold would refuse to hear it.

**One condition field.** Rejected above, on the finding it would silence.

**Delete the file to decline a want.** Rejected in the context: a declined want
with no record is the one proposed again.

**Number colliding slugs, or truncate less.** A suffix counted from the corpus is
not a function of the text. A longer bound buys filenames nobody reads for a
collision rate no corpus here approaches; the full text is in `want:` whatever
the slug keeps.

**Report desire state in `kept list`.** It is the verb a person reaches for, and
`desire-pipeline.md` proposed it. It would make the one verb that answers against
a refused ledger read the ledger, and it would give `list` a `--state` for one
kind, which [0071](0071-a-listing-verb-narrows.md) refuses the verb for all of
them.

## Consequences

- **Open question 6 is closed**, and a narrower one takes its place: what returns
  a retired want to shaping short of committing it.
- **The register computes something that is not standing.** `register.Standing`
  gains a `Desire` for desires and for nothing else, and every caller of
  `register.Derive` pays for it — one candidate check per desire, over a corpus
  that holds none today.
- **A citation added to an artifact already ratified commits a live desire with
  no new act.** The relation is not part of any grant, so the standing it rests
  on was granted by a person but the claim that it realizes this want was not.
  It cannot overturn a retirement, because that needs a later grant, and it
  cannot create standing, because nothing written to a file can. It is the
  accepted cost of the alternative above.
- **A commitment can lapse without anyone touching the desire.** Revise the
  realizing promise and the desire reads shaped until the revision is ratified —
  exactly the span in which the promise itself obliges nobody.
- **`kept status --state unratified` no longer includes desires.** Each desire is
  admitted by the word its row prints, and none prints `unratified`.
- **A retired desire can be declined without saying why.** Nothing requires a
  `--note`, for a desire or for a promise. What is reported is a missing
  condition, because the condition is what somebody reads later to decide
  whether the answer has changed; the reason is useful to that reader and not
  load-bearing for it.
- **The trial reading of a condition, the rule for a candidate that already reads
  `kept`, and shaping sessions are not decided here.** `kept shape --trial` reads
  a desire exactly as before: a desire gathers nothing, so there is nothing to
  measure until a trial reads its candidate.
