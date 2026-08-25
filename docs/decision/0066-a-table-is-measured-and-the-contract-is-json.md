---
id: 0066-a-table-is-measured-and-the-contract-is-json
type: decision
title: 0066 — A table is measured, not guessed, and the machine-facing contract stays `--json`
fields:
  statement: >-
    Every column kept prints is sized from the data in it and never truncated, the
    bounded fields come before the unbounded name, a value that distinguishes no
    row is not a column, the answer on stdout does not ask whether stdout is a
    terminal, and the only contract a machine reads is `--json`.
links:
  - { target: decision:0027-the-machine-facing-surface, relation: extends }
  - { target: decision:0065-a-run-says-what-it-is-doing, relation: depends-on }
  - { target: decision:0021-identity-is-authored-absence-is-not-an-act, relation: depends-on }
  - { target: decision:0048-the-operator-is-named-and-never-guessed, relation: depends-on }
---

# 0066 — A table is measured, not guessed, and the machine-facing contract stays `--json`

## Context

Four verbs print a row per artifact — `kept list`, `kept status`, `kept drift`
and the tick — and `kept repair` prints a fifth of the same shape. Three of them
padded with a literal, and the literal was too small.

```
  promise:core-packages-are-tested       kept       by go/tests since 2026-08-20 …
  promise:every-citation-in-the-corpus-resolves kept       by corpus since 2026-08-20 …
```

`%-38s` for the id, and the longest id in this repository's own corpus is
**seventy-three characters**. Every id past thirty-eight pushed the verdict one
place right, so the column stopped existing on exactly the rows a reader was
looking at — the promises with the most to say about themselves have the longest
names. The failure is not that the columns were badly ordered. It is that their
width was a guess about data nobody had measured.

Measured over the corpus in `docs/`, before any of this:

| | widest row | what was on it |
|---|---|---|
| `kept list` | **181** columns | type, slug, and the title that restates the slug |
| `kept status` | **132** columns | id, state, date, and `by Paul Bleisch <pbleisch@acm.org>` on every row |
| `kept drift` | 75 columns | id padded to 38, kind padded to 12 |
| the tick | — | id padded to 38, verdict padded to 10, an absolute stamp |

Three separate faults, and only the first is the padding.

**The columns were ordered least-informative first.** `kept list` led with the
type, which is the one thing every row in a run of sixty-five shares. `kept
status` led with the id, so the state — the answer, and the same word on
eighty-five of eighty-six rows — began at column seventy-five, past the right
edge of a terminal.

**It said the same thing twice.** `decision  0001-standing-and-truth-live-in-the-ledger
  0001 — Standing and truth live in the ledger, keyed by a content hash`. Of
eighty-six titles, sixty-five were the slug with a clause added and nineteen
were the slug spelled out with spaces. That is a property of this corpus rather
than of every corpus, and 0021 is why: a slug is authored once, never changes,
and is therefore written to be read.

**And it repeated what never varied.** Every row of `kept status` ended `by Paul
Bleisch <pbleisch@acm.org>` — thirty-four columns carrying nothing, because a
value the same in every row distinguishes no row.

### What the tools that solved this actually do

Each of these was run rather than remembered, and two of them are the opposite
of what a reasonable person would assume.

**`git branch -v` sizes the name column to the widest name and never truncates.**
Measured with a fifty-eight character branch beside a one-character one, and a
subject long enough to run off any screen: the output is **byte-identical piped
and on a sixty-column terminal**. Git lets the long value push and lets the
terminal wrap it.

**`git for-each-ref --format='%(align:30,left)…'` treats 30 as a minimum.** A
fifty-eight character name rendered at fifty-eight, not at thirty. Even git's
explicit alignment directive declines to cut a value down.

**`git status --porcelain=v2` carries its version in the flag, and an
unsupported one is fatal.** `--porcelain=v3` exits **128** with `fatal:
unsupported porcelain version 'v3'`. The machine format is not merely
documented; asking for a shape git does not have is refused rather than guessed.

**`git status -s` and `git status --porcelain` print the same layout and are not
the same interface.** From a subdirectory, `-s` printed `../bb.txt` and
`--porcelain` printed `bb.txt`: the human form honours `status.relativePaths`
and follows the reader's context, and the machine form is anchored to the
repository root whatever the reader's configuration says. That is the porcelain
split in one measurement — same bytes today, different promises about tomorrow.

**`ls` changes its structure entirely when it is not a terminal**: one entry per
line piped, multi-column (tab-separated, on the BSD `ls` this was run against)
on a terminal.

**`column -t` measures the data, then sizes.** So does `git branch -v`. Nobody
who solved this problem solved it with a constant.

**`kubectl -o` and `docker ps --format` hand the caller a projection language**
— `custom-columns`, `jsonpath`, a Go template — over structured data the tool
already emits. The template is not an alternative to the structured output; it
is a convenience built on top of it, and both tools ship the structured output
first.

### What a machine here is actually missing

0027 already made the split and named the machine surface: *the human format is
not a contract and may change; the structured format is, and changes to it are
versioned.* The captain's suggestion was `--csv`. Before adding one it is worth
asking what `--json` does not already carry, and the answer was measured rather
than argued:

```
$ kept status --json | jq -r '.artifacts[].id' | sort  >  a
$ kept list | …                                        >  b
$ diff a b   →  identical
```

`kept status --json` carries every artifact `kept list` prints, with `id`,
`type`, `title`, `statement` and the same `problems` array. `list` is the one
table with no `--json` of its own, and its facts are a strict subset of a
document that already exists. There is no missing capability.

## Decision

### A column is as wide as its widest cell, and nothing is ever truncated

One renderer, `cmd/kept/table.go`, used by every table kept prints. It measures
each column over the rows it actually holds and pads to that; the last column is
never padded; nothing trails a line with spaces; a column no row fills is
skipped rather than rendered as a band of empty space.

It never truncates, never elides a middle, and never wraps. Three reasons, in
order of weight. An abbreviated artifact id is one a reader cannot paste into
`kept why`, which is the next thing they will type. kept does not know how wide
the terminal is — 0065 records that there is nothing in the standard library
that answers, that `COLUMNS` is not exported by the shells that set it, and that
a module for it would have to pass 0029's test for something this cosmetic; the
progress display assumes eighty for the same reason. And it is what the tools
that solved this do: measured above, both `git branch -v` and `%(align:)` let a
long value push rather than cutting it.

### Bounded fields left, the unbounded name right

`ratified  2026-08-19  decision:0001-…`, not the reverse. This is `ls -l`, where
the name is last, and `git status -s`, where the two-character code is first.

It is the only order in which a row too long for the screen spends its overflow
on the part the reader has already identified. It also puts the column a reader
is scanning — the one word that is the same on eighty-five of eighty-six rows,
so that the eighty-sixth is what they are looking for — against a straight left
edge two characters in, instead of at column seventy-five.

The rule generalises: **the fields whose values come from a closed set go first,
in the order of how often they differ; the identifier goes last.** For the tick
that is verdict, instrument, age, artifact.

### A value that distinguishes no row is not a column

Three applications, each with what it costs.

**The type is not a column, because the id already carries it.** `kept list`
prints `decision:0001-…`, which is the spelling `show`, `why`, `ratify` and
`repair` all take and the spelling every other verb prints. One column instead
of two, and the row is now something a reader can paste.

**A title that restates its slug is not printed.** The test is on letters and
digits alone, in order: if the slug is a prefix of the title, the title has
already been read. `0001-standing-and-truth-live-in-the-ledger` against `0001 —
Standing and truth live in the ledger, keyed by a content hash` is a prefix and
is dropped; `a-failed-gather-is-unknown` against `A failed gatherer yields
unknown, never not-kept` is not, and is kept. A corpus whose slugs are not
descriptive — `ch01` against *The Fall of Rome* — fails on the first letter and
keeps every title, which is the case the rule exists to leave alone. The twenty
titles that survive it hang under their id rather than widening every row, since
a column a quarter of the rows fill is a gutter for the other three quarters.

**The principal is never a column in `kept status`.** It is stated once under
the table, and `kept why <artifact>` names it for one artifact every time. The
conditional version — a column only when more than one principal appears — was
built first and then measured, and this repository's own ledger holds **two**
spellings of one person, `Paul Bleisch` and `Paul Bleisch <pbleisch@acm.org>`,
so the conditional bought nothing here and would have been dead width in the
common case it was written for. 0048 is why the alternative is not to shorten
the name: the operator is named and never guessed, so kept prints what the act
said or it prints nothing.

**None of the three loses anything.** `kept show` prints one artifact whole,
`kept why` prints one artifact's standing with its principal, and `--json`
carries every field of all of it.

### The answer on stdout does not ask whether stdout is a terminal

kept consults `isatty` in exactly one place — `internal/progress`, which writes
to stderr and needs *both* streams to be terminals before it draws (0065). No
verb's answer varies with it, so the bytes on stdout are the same in a terminal,
a pipe, a file, a CI log and an agent's captured output.

This is a decision and not an omission. `ls` does branch, and can, because its
alternative rendering is a pure repacking of the same names; kept's rows are
records with fields, and a table that rearranged itself when piped would be a
second layout nobody had read. The measured precedent is on the other side
anyway: `git branch -v` is byte-identical at sixty columns and through a pipe.
And kept cannot measure a terminal's width, so the branch would have nothing to
decide with.

What piping is promised is **stability, not a parse**: deterministic for the
same input, no escape, no carriage return, no trailing whitespace, one record
per line with anything further indented under it. What it is not promised is
that the columns will be the same next release, and that is the whole of the
porcelain rule.

### The machine-facing contract stays exactly `--json`

No `--csv`, no `--format`, no `--porcelain`, and no `list --json`.

`--json` already carries every fact these tables print and more, on the four
verbs that answer questions about artifacts, and `kept status --json` is a
superset of `kept list` — measured above, identical id sets, plus title and
statement.

A second machine format would be **a second thing to version**, which is exactly
what 0027 refused when it chose one envelope for four verbs: *four shapes for
one set of facts is four things to version.* Two of them can disagree, and the
one that is wrong will be the one nobody tests.

It would also be **lossy in the direction kept must not be lossy**. A CSV row is
`promise:x,not-kept`. 0027 requires the reason to travel beside the value —
*a value of "unknown" with no reason is a shrug in a machine-readable wrapper* —
and a flat row has nowhere to put `standing.granted.hash`, `truth.world.tree`,
`truth.fresh` or `disposition.condition`. Flattening a nested document is a
projection, and every projection is a permanent choice about what a caller may
see. That is precisely why `kubectl` made it caller-chosen rather than fixed —
and a caller-chosen projection is a field-path query language, which kept would
then own forever. The same argument that keeps the predicate language from
growing keeps this from being written.

`jq` is the projection language, and where it is absent the answer is one more
line of the pipeline rather than one more contract.

## Alternatives

**Truncate or elide a value too long for the column.** Rejected on three
grounds, above; the strongest is that kept has no width to truncate to, and the
second is that a cut id cannot be pasted into the next command.

**Ask the terminal for its width, and lay out to it.** Rejected: it needs a
module, it would have to pass 0029's test to do something cosmetic, and 0065
already declined it once for the progress display. Taking it here would also
make the answer on stdout vary with the window, which is the property the
previous section exists to keep.

**Keep the id first and merely measure the width.** This alone fixes the
alignment, and it is worth saying plainly that it does: elastic sizing is the
whole of the bug. It was rejected because it leaves `kept status` a hundred and
thirty-two columns wide with the answer at column seventy-five, which is the
half of the complaint that is about reading rather than about arithmetic.

**Print the id alone in `kept list`, with no title ever.** Rejected as
over-fitted to this corpus: it is right here, where every slug is a sentence,
and useless for a corpus of `ch01`, `ch02`. The prefix test degrades correctly in
both directions.

**A `--csv` as asked for.** Rejected above, in full, and the argument is against
0027 rather than against the idea: it is a second versioned surface over facts
the first one already carries, and it cannot carry the reason a verdict has.

**Give `list` a `--json` of its own.** Rejected once it was measured: `kept
status --json` already emits every artifact in the corpus with type, title,
statement and the same problems array. The one thing `list` can do that `status`
cannot is answer when the ledger is unreadable — `status` folds acts and can
refuse at exit 4 under 0054, `list` never touches the ledger — and a machine
that needs the inventory of a repository whose ledger has gone backwards is
asking `kept doctor` a question, not `kept list`.

**Column headers, as `docker ps` prints them.** Rejected: no other verb here has
them, they would need a `--no-headers` for the pipeline case within a release,
and the state column beside a date says what the date is without one.

## Consequences

- **The human tables are narrower and one of them is longer.** `kept list` goes
  from 181 columns to 73 and from 88 lines to 125, because twenty titles now
  hang under their id instead of sitting beside it. That is the right trade at
  181 columns, which wraps on every terminal anyone owns.
- **The tick prints an age rather than a timestamp.** `4 days ago` instead of
  `2026-08-20 12:02:56-07:00`: the question a reader asks of a table is whether
  an answer is old, and the stamp was twenty-five columns of a line the id had
  already made too long. `kept why` still prints the moment to the second, and so
  does `--json`.
- **Anything parsing the human output breaks, once.** That is what 0027 said
  would happen and it is the first time it has. `--json` is the answer, and it is
  byte-identical across this change — proven against a build of the preceding
  commit, and pinned afterwards by a golden per verb in `cmd/kept/testdata/`.
- **`unratified` gained a second wording.** An artifact nobody ever ratified
  reads `never ratified` and one whose content moved reads `unratified` with a
  line under it naming both hashes. The state in the ledger and in `--json` is
  unchanged; this is a wording and not a sixth state.
- **`kept status` no longer says who ratified a row.** In a corpus several people
  ratify into, filtering by eye is gone and the answer is `--json` or `kept why`.
  Accepted because the alternative is thirty-four columns on every row of the
  common case.
- **The alignment guard is a check rather than a claim.** It renders every table
  over this repository's own corpus, where the ids are long enough to have broken
  the old layout, and there is a test that renders the old `%-38s` line over the
  same ids and watches the guard fail. A fixture would have passed on both.
