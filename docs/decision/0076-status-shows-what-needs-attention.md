---
id: 0076-status-shows-what-needs-attention
type: decision
title: 0076 — Status shows what needs attention, and --all is the whole register
fields:
  statement: >-
    kept status prints the artifacts whose standing needs a person — never
    granted, granted and revised since, granted under rules this build cannot
    compare against, or granted and gone — and counts the rest; an artifact
    whose kind binds nothing is counted rather than listed, because nothing can
    ratify it; --all is the register unchanged, --state replaces the default and
    the selectors compose with it; and --json still carries every artifact, with
    a boolean on each saying which the table printed.
links:
  - { target: decision:0071-a-listing-verb-narrows, relation: amends }
  - { target: decision:0066-a-table-is-measured-and-the-contract-is-json, relation: extends }
  - { target: decision:0027-the-machine-facing-surface, relation: depends-on }
---

# 0076 — Status shows what needs attention, and --all is the whole register

## Context

This corpus is ninety-nine artifacts and ninety-seven of them are ratified and
untouched. `kept status` printed a row for each, so the four that were not were
four lines in ninety-nine, and the operator found them by typing `--state
unratified` — which means the default made him work to see the one thing the
verb exists to tell him. [0071](0071-a-listing-verb-narrows.md) gave the verb
that flag and it worked; what it did not do is change what the verb says when
nobody types one.

The measurement that decided this was not the ratio. It was that the answer to
`--state unratified` on this corpus is **`document:model`, and `kept ratify`
refuses it**: the document schema declares `hash: []`, so nothing about a
document binds, there is no content version to key a grant to, and there never
will be. It cannot leave that state. A permanent entry in a list of things to
do is worse than a long list, because a reader learns the list contains rows
that are not for them and stops reading it — which is the failure mode this
change is actually about, and the reason "print fewer rows" is not the whole of
the answer.

Two things constrain what may be done about it, and both are load-bearing
enough that getting either wrong would cost more than the noise does.

**The exit code is a contract.** `kept status` exits 1 when standing lapsed and
3 when it could not tell, and it is used as a hook. A default view that changed
what a code means makes every script reading one wrong, silently, which is a
worse defect than the one being fixed.

**`--json` is the only stable format** and says so in its own help. A default
that removed ninety-five of ninety-nine records from it would break every
consumer at once and without a word.

## Decision

### The default is what needs attention, and it is one list

`kept status` prints an artifact when its standing is something a person can and
should act on. That is `needsAttention` in `cmd/kept/status.go` and it is the
whole of the judgement:

| state | listed | why |
|---|---|---|
| `ratified` | no | the resting state of an approved corpus, and not news |
| `retired` | no | an act somebody performed on purpose; nothing is owed, and editing the file does not bring it back |
| `unratified`, never granted | **yes** | a promise or a policy written and never granted standing. Exit 0, and still the thing the operator went looking for |
| `unratified`, revised since | **yes** | standing lapsed under an edit. Exit 1 |
| `unratified`, granted under other canonicalization rules | **yes** | nobody edited anything and the comparison cannot be made. Exit 3 |
| `unratified`, nothing about the kind binds, never granted | no | nothing can ratify it, so it can never leave this state |
| `unratified`, nothing about the kind binds, **granted once** | **yes** | a schema's hash list was emptied underneath a ratification. Exit 3 |
| `absent` | **yes** | ratified content that cannot be found, which 0021 refuses to read as a retirement. Exit 1 |

The shape that falls out is worth naming, because it is what says the rule is
not arbitrary: **every artifact the exit code is about is listed**, plus the
never-granted ones, which are exit 0 by design and are the only rows on this
list an operator settles with a single command.

It is **one list and not a severity ranking.** The question is *what do I have
to look at*, and an answer that sorted those rows into tiers would ask the
reader to learn a taxonomy before reading a finding. If two levels are ever
genuinely needed, the evidence will be a reader ignoring one of them.

### An artifact nothing can ratify is counted, not listed and not silent

One line per kind, under the table:

    1 document is not listed: nothing about a document binds, so it cannot be ratified

Silence was the other candidate and it is wrong: a reader who knows the corpus
holds documents and sees no document anywhere goes looking for the bug, and a
reader who does not know never learns the kind exists. Listing it is wrong for
the stronger reason above. The line names the kind rather than the artifacts,
because the reason is a property of the kind and repeating it per row would be a
second table of things nobody can act on.

**The grant is the discrimination.** An unbindable artifact that somebody had
already ratified is listed, because a grant in force against a kind that no
longer binds means a hash list was emptied underneath a ratification and it was
de-ratified with nobody deciding to. That is exit 3, nothing else in kept
reports it, and it is the opposite of a permanent fixture — somebody can put the
hash list back. It is also the case a rule written as "hide unbinding" would
swallow, so it has a test that breaks it on purpose.

### `--all`, and not `--details`

`--all` lists every artifact and is byte for byte what `kept status` printed
before this decision, pinned against a golden captured from the commit before
it. `--details` was the alternative and it answers a different question: it
would show more *about* each row, and there is nothing withheld per row to show
— 0066 already refuses to truncate a cell or to guess a width, and what a table
drops is on `kept show`, `kept why` and `--json`. The ask was about which rows,
so the flag is about which rows. It is the same word the tick already spends on
the same idea — *do not apply the default narrowing* — which is worth more than
a name that is merely accurate.

### `--state` replaces the default; the selectors compose with it

`--kind` and `--id` name a population and the default draws attention within it,
so `kept status --kind promise` is *the promises that need attention*. A filter
narrows what the reader would otherwise have seen, which is the only reading
under which three flags and a default all compose.

`--state` is the exception and it is not an accident. Asking for a state by name
is asking to see it, and under the default `kept status --state ratified` would
print nothing whatever the corpus held — a flag that cannot answer its own
question. This also keeps 0071's own selector/predicate split intact: the
default *is* a predicate over state, and a predicate the operator writes by hand
replaces the one kept assumed rather than intersecting with it.

### `--json` carries every artifact, with a boolean on each

The document is unchanged except for one added field, `standing.attention`,
emitted on every record and never omitted when false so that a consumer can tell
a build that answers this question from one that does not. Explicit filters
still narrow it, exactly as 0071 says.

So the table's default is an **emphasis in a rendering** and the explicit
filters are **narrowings of the question**, and only the second reaches the
machine surface. That is a distinction 0071 did not have to make, because it had
no default of its own. The version stays `kept/v1`: a field added is not a shape
changed for a consumer that reads by name, where a removal or a rename would be.

### The exit code does not move

It is computed over the population the explicit filters admit, before the
default is applied — which is the same set it was computed over before this
decision, so every code is what it was. A run whose only finding is hidden
cannot happen: the hidden case is the one that contributes no code.

## Alternatives

**Show drift in the default view too.** The clearest candidate and it is
refused. `status` reports standing and `drift` reports truth; a `not-kept`
promise in `status` would make it exit 1 for a second, different reason, which
is the one thing this change may not do. It would also make the verb re-derive
or read truth on every run, which is `drift`'s cost and not this one's.

**Treat `stale` as an anomaly.** Not `status`'s question at all — it is a drift
kind — and the answer would have been no anyway. Seven promises read `stale` on
the morning this was written because no tick had run, not because anything
failed, and a view that reported them as findings would have been reporting the
scheduler. Staleness is `max_age` doing its job; what a reader wants from it is
`kept tick`, not a row.

**Report retired-but-still-cited.** A real anomaly and it belongs to
[`doctor`](0050-doctor-reports-an-unchecked-instrument.md), by 0050's own three-part
test: nothing rejects it, it has no other reader, and kept does nothing about
it. It is also not a fact about standing, which is all `status` reads. Left
unbuilt here rather than smuggled in.

**Hide the unbindable artifacts silently.** Rejected above.

**Let `--state` compose with the default rather than replace it.** Rejected:
`--state ratified` would then always print nothing, and an operator would read
that as a corpus with no ratified artifacts in it.

**Narrow `--json` with the default and let the flag turn it back on.** Rejected.
It makes a display default into a change to a versioned surface, and every
existing consumer would start reading four records where it read ninety-nine,
with no error and no version bump to notice.

**Bump the schema version for the added field.** Rejected. A consumer reading by
name is unaffected by an addition, and spending a version on one would make the
version stop meaning *the shape changed under you*.

**A severity ranking — errors, warnings, notices.** Rejected. It is a taxonomy
to learn before a finding can be read, and there are eight rows in the table
above, not eighty. Two levels can be argued for later against a reader who
ignores one.

**Leave the default and tell the operator to type `--state unratified`.** That
is what he did, and the answer he got contained one artifact that can never be
ratified. The flag existing is not the same as the verb answering.

## Consequences

- **`kept status` no longer prints the corpus.** On this repository it went from
  ninety-nine rows to three lines. That is the point, and `--all` is one flag
  away and unchanged.
- **0071's "the unfiltered rendering is byte for byte what it was" no longer
  holds for `status`,** deliberately. It moved onto `--all`, pinned against the
  same golden, so the claim is still made about a commit rather than about a
  fixture written afterwards. `list` and `drift` are untouched.
- **`--json` gained a field and lost nothing.** The goldens moved by exactly one
  line per record.
- **`drift` and `why` carry `standing.attention` too.** It is a property of
  standing and is well defined wherever standing is, and a field that appeared
  on one verb's records and not another's would be read as meaning something
  about the verb.
- **A reader of a narrowed run sees two footers.** The fraction needing
  attention within the population, and 0071's line saying the population is not
  the corpus. They answer different questions and neither implies the other.
- **`drift` keeps its default.** Every row it prints is already a finding, so
  there is nothing there to hide. If that stops being true the argument here
  applies to it unchanged.
