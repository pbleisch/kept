# 0039 — The ledger records acts, and `decision` is the corpus's word

**Status** accepted · removes the collision
[0019](0019-a-refusal-is-a-disposition.md) worked around · renames what
[0001](0001-standing-and-truth-live-in-the-ledger.md) and
[0032](0032-standing-is-a-fold-over-acts.md) describe, and changes nothing
either of them decided

## Context

`decision` names two things. In the corpus it is an artifact kind — every file
in this directory, and the `_schema/decision.yaml` that will declare what one
must say. In the ledger it is an act: the `Decision` record, the
`decisions.jsonl` stream, `ReadDecisions`.

The collision has already cost something. 0019 needed a name for what the tick
records when it declines to act, and had to spend a paragraph on it:

> **The word is `disposition`, not `decision`.** `decision` is already an
> artifact kind in the corpus and an act in the ledger; a third meaning would
> make `kept why` ambiguous about which it was answering.

That is a naming choice distorted by a collision, written down at the time by
the person making it. The cost is demonstrated rather than hypothetical, and
every verb that reads both sides — `why`, `status`, `doctor` — pays it again.

**Moving is free exactly now.** `.kept/ledger/` holds `truth/`,
`observations/`, `disposition/` and `runs/`, and no `decisions.jsonl` at all;
`kept status` in this repository reports that nothing has ever been ratified
here. The stream is append-only and never rewritten
([0018](0018-what-survives-a-crash.md)), so the day it holds one real record a
rename stops being a rename: it becomes a reader that has to know two stream
names, or a rewrite of records that were supposed to be permanent. Neither is
something to accept for a word.

## Decision

**The ledger's stream of standing-creating acts is `acts.jsonl`, its record is
`ledger.Act`, and `decision` belongs to the corpus.**

| was | is |
|---|---|
| `decisions.jsonl` | `acts.jsonl` |
| `ledger.Decision` | `ledger.Act` |
| `ledger.Decisions` | `ledger.Acts` |
| `ledger.ReadDecisions` | `ledger.ReadActs` |
| `run.Record.Decisions` | `run.Record.Acts` |

Nothing a record *says* changes. The envelope keeps `v: 1`, the kinds stay
`ratified` and `released`, and every field keeps its JSON name — because no
record's shape moved, and because there are no records. What moved is the name
of the box they go in.

### The corpus keeps the word because the corpus is read

`decision` is what the industry calls an architecture decision record, what all
thirty-nine of these files are, and what a person types at a prompt. The
ledger's name is a path component inside a ref no working tree checks out. When
two things want one word, the one that yields is the one nobody reads.

### `act` is not a new word

`model.md` has called these acts since it was written — the table is headed
**The acts**, and ratify, land and release are three rows of it. `ledger`'s own
doc comment already says "human acts, low volume". The rename does not coin
anything; it stops the code disagreeing with the model it implements. What the
acts table records as "a decision" is now recorded as "an act".

### Accepted ADRs are edited in place, and that is not a supersede

The convention is to supersede a decision rather than edit it. **Nothing here
changes a decision.** 0019 still decides that every refusal is recorded against
the artifact and is not written to the spine. 0032 still decides that standing
is a fold in event-time order and that a release empties the set. A reader who
acted on the old text and a reader who acts on the new one do the same thing.

That is the test, and it is the rule: **a terminology rename is edited in
place; a change to what was decided is superseded.** Superseding here would
leave every reader of 0032 reading an accepted decision that describes a stream
the code does not have, and forwarding them to a new file that repeats it
verbatim with one noun swapped — which makes the record longer and less true.

Two of the edits are more than a noun swap, and both are titles.

**0019 — *A refusal is a decision*** used *decision* in the ordinary English
sense, in order to contrast it against the stream. With `decision` reserved for
the artifact kind, that title reads as *a refusal is an ADR*, which is the
opposite of what it decided. It becomes **A refusal is a disposition, and it is
recorded** — the word the ADR itself settles — and the paragraph quoted above
is reworked, because its argument had two limbs and now has one.

**0032 — *Standing is a fold over decisions*** named the stream in its title.
It becomes **Standing is a fold over acts**.

Both files are renamed to match, because a filename that contradicts the title
inside it is the divergence `kept doctor`'s path/id check exists to catch. Both
renames happen **before any ADR is ratified**: the path is the id here, so
renaming a ratified artifact would drop its standing (0021, 0032) and leave the
ledger recording a retirement nobody performed.

## Alternatives

**Leave the collision.** Rejected. It is already being paid for — a paragraph
in 0019, and an ambiguity every reader of `kept why` would have to resolve from
context. The cost of fixing it is paid once and is at its minimum today; the
cost of not fixing it recurs and grows.

**Rename the corpus kind instead — `record`, `rationale`, `adr`.** Rejected on
the same reasoning that decides which side yields. It would also mean renaming
`docs/decision/`, thirty-nine files and every cross-reference between them, to
spare a filename nobody opens.

**Call the ledger's stream `events`.** Rejected. An event says something
happened and says nothing about who may make it happen. The whole content of
these records is that a *person* performed one of three acts that may not be
delegated, and `act` is the word the model already uses for exactly that set.

**Wait until a rename is worth doing.** Rejected: that moment is the moment it
stops being a rename. Append-only means the first real record is the last cheap
day.

## Consequences

- **`decisions.jsonl` is a name nothing in this build writes or reads.** No
  workspace holds one, so there is no migration and no compatibility path — and
  deliberately none, because a reader that understood both names would be the
  migration this decision exists to avoid.
- **`decision` names exactly one thing**, an artifact kind, and 0019 is one
  clause shorter for it.
- **`model.md`'s acts table and the ledger agree on a word.** They did not
  before, and the disagreement was invisible precisely because the table was
  right and the code was not.
- **Several accepted ADRs were edited in place and two were renamed.** The
  rule above is the licence for it, written here so that nobody reads the
  supersede convention as having been broken quietly. A future edit that
  changes what was decided still supersedes.
