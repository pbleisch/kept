---
id: shape-trial-explains-each-clause-instead-of-dumping-instruments
fields:
  want: shape --trial explains each clause instead of dumping instruments
  becomes: promise
  candidate:
    statement: >-
      kept shape --trial opens with its verdict and what that verdict means, gives one
      line for each clause of the predicate saying whether the clause held and what it
      read, and prints what each instrument measured only when asked.
    gather:
      plain:
        gatherer: command
        run: go run ./cmd/kept shape promise:the-verdict-does-not-read-the-agent --trial --quiet
        scope: ["cmd/kept/**/*.go", "internal/**/*.go", "docs/**"]
        fields:
          ok: exit == 0
          long_lines: >-
            count(find_all(stdout, '(?m)^.{1000}'), m => true)
          dumped: >-
            count(find_all(stdout, 'github\.com/pbleisch/kept/internal/assess'), m => true)
      asked:
        gatherer: command
        run: go run ./cmd/kept shape promise:the-verdict-does-not-read-the-agent --trial --quiet --instruments
        scope: ["cmd/kept/**/*.go", "internal/**/*.go", "docs/**"]
        fields:
          ok: exit == 0
          dumped: >-
            count(find_all(stdout, 'github\.com/pbleisch/kept/internal/assess'), m => true)
      vacuous:
        gatherer: command
        run: go run ./cmd/kept shape promise:the-readme-names-every-verb --trial --quiet --against docs
        scope: ["cmd/kept/**/*.go", "internal/**/*.go", "docs/**"]
        fields:
          ok: exit == 0
          verdict: >-
            map(find_all(stdout, '\A[ \t]*(?P<word>\S+)'), m => m.word)
          ok_mark: >-
            map(find_all(stdout, '(?m)^[ \t]*(?P<mark>\S+)[ \t]+obs\.verbs\.ok[ \t]*$'), m => m.mark)
          count_mark: >-
            map(find_all(stdout, '(?m)^[ \t]*(?P<mark>\S+)[ \t]+count\(obs\.verbs\.names, v => true\) > 0'), m => m.mark)
          all_mark: >-
            map(find_all(stdout, '(?m)^[ \t]*(?P<mark>\S+)[ \t]+all\(obs\.verbs\.names, v => v in obs\.readme\.mentions\)'), m => m.mark)
          mentions_named: >-
            count(find_all(stdout, 'obs\.readme\.mentions'), m => true)
      tests:
        gatherer: go/tests
        scope: ["cmd/kept/**", "internal/**"]
        packages: "./cmd/kept"
    hold_when: >
      obs.plain.ok and obs.plain.long_lines == 0 and obs.plain.dumped == 0
      and obs.asked.ok and obs.asked.dumped > 0
      and obs.vacuous.ok and "not-kept" in obs.vacuous.verdict
      and count(obs.vacuous.ok_mark, m => true) == 1
      and count(obs.vacuous.count_mark, m => true) == 1
      and count(obs.vacuous.all_mark, m => true) == 1
      and all(obs.vacuous.ok_mark, m => m in obs.vacuous.count_mark)
      and all(obs.vacuous.all_mark, m => not (m in obs.vacuous.count_mark))
      and obs.vacuous.mentions_named >= 2
      and obs.tests.ok
      and count(obs.tests.packages, p => p.package == "github.com/pbleisch/kept/cmd/kept"
                and p.ok and p.skipped == 0) == 1
      and "TestATrialOverAnEmptyMeasurementPrintsTheEmptyMeasurement" in obs.tests.cases
      and "TestATrialGivesEachClauseItsAnswerAndWhatItRead" in obs.tests.cases
      and "TestATrialPrintsWhatTheInstrumentsMeasuredOnlyWhenAsked" in obs.tests.cases
    trigger: cadence
    max_age: 7d
    repair:
      scope: ["cmd/kept/shape.go", "cmd/kept/shape_test.go", "cmd/kept/main.go", "cmd/kept/testdata/**"]
  ripe_when: >-
    promise:each-clause-of-a-predicate-answers-on-its-own is ratified and reads kept. Until
    internal/predicate declares Clauses, a repair bounded to cmd/kept could split a hold_when
    only by writing a second parser. A trial could read this as go/symbols defining a method
    named Clauses in github.com/pbleisch/kept/internal/predicate.
  shaped_from:
    - "kept shape promise:the-verdict-does-not-read-the-agent --trial, 2026-09-14: 271,993 bytes in 15 lines"
    - cmd/kept/shape.go, whose doc comment says a trial exists to show a check that can fire
---

## What was asked

The operator ran `kept shape promise:the-verdict-does-not-read-the-agent --trial`
and could not tell what it said or what to do with it. `shape --trial` exists
to show *a check that can fire*, and the reading buried that question under the
instruments.

Measured on 2026-09-14 at `53c1a5c`, the output was 271,993 bytes in 15 lines.
The verdict is one line at the top. The whole import graph and every identifier
the package references follow it, each printed as a single-line list. Trimmed
to 160 columns:

```
  kept  promise:the-verdict-does-not-read-the-agent

measured

  graph  edges     [{from: github.com/pbleisch/kept/internal/assess, to: github.com/pbleisch/kept/internal/corpus}, {from: github.com/pbleisch/kept/internal/ass
  graph  packages  [{deps: [github.com/pbleisch/kept/internal/canonical, github.com/pbleisch/kept/internal/corpus, github.com/pbleisch/kept/internal/gather, git
  names  refs      [{name: Context, package: github.com/pbleisch/kept/internal/assess, path: context}, {name: WithTimeout, package: github.com/pbleisch/kept/int
```

Nothing says which of the predicate's five clauses read which of those values.
Nothing says whether any clause could have come out differently. The one fact a
trial is for is whether a clause held over an empty measurement, and here it is
somewhere in a 100 KB line.

What is wanted:

- a plain first line: the verdict and what it means;
- one line per `hold_when` clause, with whether it held and the values it read;
- each instrument's full output only behind a flag. `--instruments` names what
  the flag prints, which a level word like `--verbose` does not.

`kept shape` has no `--json` today, so there is no machine surface to keep
stable. Adding one would be a 0027 envelope of its own, and it is not part of
this want.

## Why it is not a promise yet

It needs the evaluator to answer for each clause, and today it cannot. That
half is `promise:each-clause-of-a-predicate-answers-on-its-own`, which realizes
`desire:the-predicate-evaluator-answers-for-each-clause-of-a-hold-when`.
`ripe_when` above is the condition.

## What the candidate measures, and why it reads the binary

The named tests carry the detail. The candidate does not trust them alone,
because a test with the right name and no body passes. Three instruments run
`kept shape` itself, through `go run`, on subjects already in this corpus:

- **`plain`** trials `promise:the-verdict-does-not-read-the-agent`, the
  operator's own example. The reading may hold no line of 1,000 bytes or more,
  and it may not name `internal/assess`. That package appears only in the
  instrument data, never in the predicate.
- **`asked`** is the same trial with `--instruments`, and it must name that
  package. The measurement is still one flag away.
- **`vacuous`** trials `promise:the-readme-names-every-verb` against `docs/`,
  which has neither `README.md` nor `cmd/kept/main.go`. Three clauses fail
  there. The fourth, `all(obs.verbs.names, v => v in obs.readme.mentions)`,
  holds over two empty lists. That is exactly what a trial exists to expose.
  The candidate requires each clause's line to open with a mark followed by the
  clause's source, and it requires the vacuous clause's mark to differ from the
  failing ones'. That is 0066's own rule, bounded field left and unbounded name
  right. The words are left to the author: `holds` and `fails` are one choice,
  and `true` and `false` another. The vacuous clause must also name what it read
  somewhere besides its own source, which it does only if its readings are
  printed.

This costs two things, and both fail loudly. The candidate depends on two other
promises' wording, so revising either makes it read `not-kept`. And the
instruments run kept itself, which reads the ledger to print its last line. If
kept refuses to read the ledger, `ok` goes false, so the candidate reads
`not-kept` where `unknown` would be honest. Whoever turns this into a promise
trials it again first.

The named tests, and what each must do:

- **`TestATrialGivesEachClauseItsAnswerAndWhatItRead`** checks a candidate whose
  predicate has a clause that holds, a clause that fails and a clause nobody
  measured. Each gets its own line with its answer, and the values each read
  appear beside it.
- **`TestATrialPrintsWhatTheInstrumentsMeasuredOnlyWhenAsked`** checks that a
  value only an instrument carries is absent without `--instruments` and
  present with it.
- **`TestATrialOverAnEmptyMeasurementPrintsTheEmptyMeasurement`** exists today
  and asserts that `[]` is on the page. Its purpose survives the change: an
  empty measurement must still be visible without the flag, on the line of the
  clause that read it. Rewriting what it asserts is expected. Deleting that
  assertion is a hollow repair.

## What a repair here may not reach

`README.md` says a trial *prints what each of its instruments said beside the
verdict*, and that sentence goes stale when this lands. It cannot be in the
repair bound. A scope naming a root-level file renders as a grant for the box
root, which the confinement provider refuses, so a promise that included it
could never be repaired. Correct the README by hand in the same land.
