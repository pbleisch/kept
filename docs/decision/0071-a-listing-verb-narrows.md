---
id: 0071-a-listing-verb-narrows
type: decision
title: 0071 — A listing verb narrows, and the exit code follows the narrowing
fields:
  statement: >-
    kept status, kept drift and kept list narrow by kind, by a glob over the whole
    printed id, and by the state that verb itself reports; the exit code and
    `--json` describe the narrowed set; a selector matching nothing is the question
    never having been put and a state nothing is in is an answer; and the
    unfiltered rendering of all three is byte for byte what it was.
links:
  - { target: decision:0027-the-machine-facing-surface, relation: extends }
  - { target: decision:0066-a-table-is-measured-and-the-contract-is-json, relation: extends }
  - { target: decision:0069-an-artifact-is-scaffolded-from-its-schema, relation: depends-on }
---

# 0071 — A listing verb narrows, and the exit code follows the narrowing

## Context

The corpus is ninety-one artifacts and growing. Three verbs print a row per
artifact — `kept status`, `kept drift` and `kept list` — and none of them could
be pointed at part of it. Answering *which of these have I not ratified* took
`grep` over the output of a verb that already knew.

[0066](0066-a-table-is-measured-and-the-contract-is-json.md) had just finished
making those tables readable: a column is as wide as its widest cell, the
bounded fields come before the unbounded name, and a value the same in every row
is not a column. A narrower table is worth less than a shorter one when the
reader already knows which sixteen rows they want, and 0066 deliberately spent
its last section refusing a second machine format on the grounds that `--json`
already carries everything. Narrowing is the other half of that: the answer to
"this is a lot of rows" is a query, not a projection language.

Two things about the shape of the problem are not obvious and decide most of
what follows.

**"State" is not one vocabulary.** `status` reports standing — `ratified`,
`unratified`, `never ratified`, `released`, `absent` — and `drift` reports truth
— `unverified`, `not-kept`, `regressed`, `stale`, `unknown`, `conflicted`,
`absent`. They share exactly one word, `absent`, and it means different things
in the two places. `kept list` reports neither, because it never touches the
ledger, and standing and truth both live there.

**An empty result has two meanings, and only one of them is bad news.** A filter
that matched nothing renders as an empty table, which reads as success. kept has
this lesson written down one level lower, in `internal/gather/tests.go`: *a
suite of no packages is not `ok` — nothing was demonstrated, and a pattern
matching nothing is the ordinary way that happens.* But a narrowing that returns
nothing is not always that. `kept status --state unratified` returning nothing
is the good news the operator was hoping for.

## Decision

### Three flags, on the three verbs that list artifacts

`--kind`, `--id` and `--state`, on `kept status`, `kept drift` and `kept list`.
Repeating one is `or`; giving two is `and`.

They are on all three rather than on `status` alone, which is where the request
started. A filter that works on one verb and not on its neighbours is a thing an
operator has to remember rather than a thing they can use, and the model is not
purer for it — `list` is `status` with the ledger left out, and `drift` is
`status` with truth added.

**The tick takes none of them.** A filter there would narrow what is *recorded*,
not what is shown, and `--all` is the flag it does have. This is a read path and
nothing on it writes to the ledger.

### `--kind` reads the schema directory

The accepted values are the types `docs/_schema` declares, so a kind added
tomorrow is filterable the day its schema lands with no code change. That is the
rule [0069](0069-an-artifact-is-scaffolded-from-its-schema.md) built the whole
of `kept new` on, and it is checked the same way: by adding a schema and nothing
else.

The set is the union of that with the kinds the corpus actually holds. A type
directory with no schema is a reported problem and its artifacts are still
loaded and still printed, so refusing to narrow to them would be refusing to
narrow to the rows a reader most wants to look at.

A kind nothing declares is refused by name at exit 2, with the declared ones
listed. It is an authoring error in what the operator typed, and 0027 makes
those 2.

### `--id` is a glob over the **whole printed id**

`type:slug`, which is the spelling every table here prints and the one `show`,
`why`, `ratify` and `repair` all take. `--id "promise:*"` is the promises;
`--id "*tests*"` is anything with that in either half.

A pattern that quietly matched the slug alone would answer differently from what
the reader is looking at, which is a bug they would file as confusion. So the
help says which, the help shows the pattern quoted — an unquoted `*` is expanded
by the shell before kept sees it — and a pattern that matches a slug but no id
is answered with the pattern that would have worked.

The syntax is `path.Match`: `*`, `?`, `[…]`, and no library. A malformed one is
refused rather than silently matching nothing.

### `--state` is the vocabulary the verb it is typed on reports, and refuses the other

`status` takes `ratified`, `unratified`, `never-ratified`, `released`, `absent`.
`drift` takes the drift kinds. `list` reports no state and takes no `--state` at
all.

One flag name serving two vocabularies is safe here only because of the
refusal: `kept drift --state ratified` exits 2 saying *drift reports drift kind*
and listing the seven it does take, and `kept list --state ratified` exits 2
saying *list reports no state* and naming the verb that answers that question.
A flag that quietly accepted both sets would match nothing on the wrong verb and
answer about neither.

`unratified` on `status` is every artifact that obliges nobody however it got
there — the state the ledger holds, the state `--json` carries, and the word the
summary line counts under. `never-ratified` narrows that to the ones nobody ever
granted, which is the wording 0066 gave those rows. There is no `revised`: an
artifact that was granted and has moved since is what `unratified` reports and
what exit 1 is about, so asking for it is asking for the finding rather than for
a spelling.

### The exit code describes the narrowed set

`kept status --kind promise` answers about the promises and says nothing about
the decisions it was not asked about. A script asking *are my promises ratified*
wants the answer to its own question, and a code that reported on the whole
corpus would make the flag useless in the hook the exit code exists for.

Unfiltered, every code is exactly what it was, and there is a test that breaks a
promise, watches `kept status` exit 1, and watches the same run narrowed onto an
unaffected kind exit 0.

Two things stay outside the narrowing and each has a reason. **Quarantined
ledger lines still make a run exit 3**, filtered or not: a line that could not
be read might have been the act that settles the very question that was asked.
And **problems are never narrowed** — a schema violation or a broken link is a
defect somebody must fix rather than one of the rows you asked to see, and a
filter is not a licence to hide one.

### A selector matching nothing is exit 3; a state nothing is in is not

This is the split the two meanings of an empty result demand, and it falls out
of a distinction the model already makes.

`--kind` and `--id` are **selectors**: they name a population out of the corpus,
and they are properties an artifact has whether or not anything was ever derived
about it. A selector that names nothing means **the question was never put** —
`kept status --kind desire` over a corpus with no desires, `--id "core-*"` where
no id looks like that. That is exit 3, with a sentence saying so and, where kept
can be sure of it, what to try instead. It is 0027's *no evidence either way*
and it is the same rule as *nothing has answered is not an answer of `unknown`*
one level up.

`--state` is a **predicate**: it is applied to what the verb derived about that
population, and a predicate admitting nothing is an answer — the things you
asked about are none of them in that state. That is the verb's ordinary exit
over an empty set, which is 0, and the run still says `4 artifacts, and none of
them is released` rather than printing an empty table, because an empty table
reads as success.

So the two empties are distinguished by a code *and* by a sentence, and the
sentence is different in the two cases. A kind declared with nothing in it and a
glob that matches no id are both the first case, and the message names which.

### `--json` is narrowed identically, and nothing else about it moves

It is the same query rendered differently, not a second surface. The artifacts
array carries what the table's rows carry, the summary counts the narrowed set —
a consumer adding it up must not get a number no row of the document supports —
and `exit` is the code the process returned. Problems travel unnarrowed, as
above.

No field was added and none was renamed, so the version stays `kept/v1` and the
goldens in `cmd/kept/testdata/` are unchanged.

### The unfiltered rendering is byte for byte what it was

A filter that changes the default output is not a filter. All three verbs are
pinned against goldens captured from the build immediately before the flags
existed, so passing them is a statement about a commit rather than about a
fixture written afterwards.

A run that *was* narrowed says so, in one line under the table: `narrowed to 8
of 91 artifacts by --kind promise --state ratified`. An operator who filtered
knows they filtered; an operator reading the captured output of somebody else's
command does not, and a table with no footer is indistinguishable from the whole
of a small corpus.

## Alternatives

**A single `--state` accepting both vocabularies.** Rejected. It would mean
different things depending on where it was typed, which is precisely what makes
a flag hard to remember, and the failure mode is silent: `kept drift --state
ratified` would match nothing and print an empty table, which is the reading
this decision spends a whole section preventing. Refusing the other vocabulary
by name teaches the operator which verb answers their question.

**Separate flag names — `--standing` on `status`, `--truth` on `drift`.** This
removes the ambiguity completely and was the closest call here. Rejected because
it costs an operator two names for one idea in exchange for a distinction the
refusal already makes, and because it would leave `list` with neither and no
sentence explaining why.

**Match the glob against the slug alone, or against either half.** Rejected: the
id is what is printed and what every other verb takes, and a filter that matches
something other than what the reader can see is a defect they will file as
confusion. The hint covers the ordinary mistake.

**A hardcoded list of kinds.** Rejected on the same ground `kept new` refuses a
per-kind template: it is a second copy of the schema directory, and it goes
stale the day a kind is added.

**Exit 3 whenever anything matched nothing, `--state` included.** Rejected: it
would make `kept status --state unratified` — the question that motivated all of
this — exit 3 on the good news. The selector/predicate split is what keeps the
`internal/gather/tests.go` lesson from being applied where it does not hold.

**Exit 0 whenever anything matched nothing.** Rejected in the other direction: a
kind that does not exist and a typo in a glob would then be indistinguishable
from a clean corpus, in a CI job, silently.

**Report the exit code over the whole corpus and let the filter be presentation
only.** Defensible, and someone will assume it, which is why it is pinned by a
test. Rejected because it makes the flag useless to the script that has the
question — and because the human reading a narrowed table has already been told,
by the footer, that they are not looking at everything.

**Narrow the problems too.** Rejected: many problems have no id to narrow by —
a file with no frontmatter, a type directory with no schema — and the ones that
do are still defects of the corpus rather than of the rows a reader asked for.
A filter must not be able to make a broken corpus look clean.

**Give the tick the same flags.** Rejected: it would narrow what is recorded,
which is not a rendering choice, and `--all` is the flag it has.

**A field-path query language, as `kubectl -o custom-columns` has.** Rejected
here for the reason 0066 rejected `--csv` and 0036 refuses to grow the predicate
language: it is a surface kept would then own forever, and `jq` already exists
on the other side of `--json`.

## Consequences

- **Three verbs gained three flags and one output line.** The line is printed
  only when a flag was given, so the unfiltered rendering is untouched and is
  pinned as such.
- **`kept list` can now exit 3.** Only under a selector that matched nothing;
  unfiltered it still only ever exits 0. It is the first exit code that verb has
  had other than 0 and 2.
- **`--state` will be typed on the wrong verb.** That is the case the refusal
  message is written for, and it names the vocabulary the verb does take rather
  than only saying no.
- **An operator reading a narrowed table sees a filtered corpus.** The footer is
  the whole mitigation, and it is one line rather than a banner because the
  common case is the operator who typed the flag themselves.
- **Repeating a flag is `or` and this is not written on the flag.** `--kind
  promise --kind policy` is the two kinds, which is what a person means; nothing
  in the Go flag package suggests it, so the help says it once.
