---
id: each-clause-of-a-predicate-answers-on-its-own
type: promise
title: Each clause of a predicate answers on its own
fields:
  statement: >-
    internal/predicate reads a hold_when as the clauses a top-level and joins, and answers
    for each one alone. A clause is an exact span of the authored source, and it reports
    what it read and nothing its neighbors read, each reading as measured or as nobody
    measured it. A clause answers what its source answers parsed alone, and the clauses'
    answers joined by the evaluator's own three-valued and are the verdict the whole
    predicate reaches, except that a bound belongs to the whole: a predicate refused at a
    bound cannot be answered clause by clause.
  gather:
    tests:
      gatherer: go/tests
      scope: ["internal/**", "docs/**"]
      packages: "./internal/predicate"
    api:
      gatherer: go/symbols
      scope: ["internal/predicate/**/*.go"]
    testcode:
      gatherer: go/symbols
      scope: ["internal/predicate/**/*.go"]
      tests: true
  hold_when: >
    obs.tests.ok and obs.api.ok and obs.testcode.ok
    and count(obs.tests.packages, p => p.package == "github.com/pbleisch/kept/internal/predicate"
              and p.ok and p.skipped == 0) == 1
    and count(obs.api.defines, d => d.package == "github.com/pbleisch/kept/internal/predicate"
              and d.kind == "method" and d.name == "Clauses") == 1
    and count(obs.api.defines, d => d.package == "github.com/pbleisch/kept/internal/predicate"
              and d.kind == "func" and d.name == "And") == 1
    and count(obs.testcode.refs, r => r.package == "github.com/pbleisch/kept/internal/predicate"
              and r.path == "github.com/pbleisch/kept/internal/predicate" and r.name == "And") == 1
    and "TestAClauseIsATopLevelConjunctWrittenExactlyAsAuthored" in obs.tests.cases
    and "TestAClauseThatSpansLinesKeepsItsLineBreaksAndIndentation" in obs.tests.cases
    and "TestTheClausesJoinedByAndAreTheVerdictInEveryRow/not-kept_and_unknown" in obs.tests.cases
    and "TestTheClausesJoinedByAndAreTheVerdictInEveryRow/unknown_and_not-kept" in obs.tests.cases
    and "TestEachClauseReportsWhatItReadAndNothingItsNeighborsRead" in obs.tests.cases
    and "TestAClauseOverAnEmptyMeasurementHoldsAndReportsTheEmptyList" in obs.tests.cases
    and "TestABoundBelongsToTheWholePredicateAndAMistakeToItsClause" in obs.tests.cases
    and "TestEveryClauseInThisCorpusIsASpanThatAnswersAloneAsItDoesInPlace/promise:the-verdict-does-not-read-the-agent/every_reading_measured" in obs.tests.cases
    and "TestEveryClauseInThisCorpusIsASpanThatAnswersAloneAsItDoesInPlace/promise:the-readme-names-every-verb/every_reading_measured" in obs.tests.cases
    and "TestEveryClauseInThisCorpusIsASpanThatAnswersAloneAsItDoesInPlace/promise:each-clause-of-a-predicate-answers-on-its-own/every_reading_measured" in obs.tests.cases
  trigger: cadence
  max_age: 7d
  repair:
    scope: ["internal/predicate/**"]
links:
  - { target: desire:the-predicate-evaluator-answers-for-each-clause-of-a-hold-when, relation: realizes }
  - { target: desire:shape-trial-explains-each-clause-instead-of-dumping-instruments, relation: cites }
  - { target: desire:a-clause-reports-a-reading-that-failed-instead-of-leaving-it-out, relation: cites }
  - { target: decision:0036-the-predicate-language, relation: cites }
  - { target: decision:0078-the-derived-checklist-is-closed-and-the-argument-is-not, relation: cites }
---

`kept shape --trial` exists to show whether a check can fire, and today it
cannot say which part of a predicate decided anything. `internal/predicate`
hands back one verdict for the whole expression and nothing about its parts, and
its tree does not record where a part ends: every node carries only the position
it starts at, and a binary node's position is its operator's. So a reader of a
trial gets the verdict and then every instrument's raw output, which for
`promise:the-verdict-does-not-read-the-agent` was 272 KB on 2026-09-14. That
dump is the only way to find out that an `all()` held over an empty list.

This is the half of the fix that belongs to the language. The half that prints
it is `desire:shape-trial-explains-each-clause-instead-of-dumping-instruments`,
and it cannot be written first: a repair bounded to `cmd/kept` could split a
`hold_when` only by writing a second parser, and a second reading of a binding
expression is how two instruments come to disagree.

**This is the second wording, and the first was repaired.** Thread
`159qnynw8m5fhtkxt6kch6s8fh` added `Clauses` and four tests with the names the
first wording read, and the promise read `kept`. The adversarial pass over that
thread found the mechanism real and the tests weaker than their paragraphs: no
clause was ever answered `not-kept`, a clause reporting its whole predicate's
readings passed, the empty-list test never asked whether the clause held, no
expected clause spanned lines, and the corpus sweep compared `unknown` with
`unknown` over a scope where nothing was gathered. It also found two behaviours
the statement did not survive. Every gap is now a name or a reference this
predicate reads, so the same repair reads `not-kept`.

Three tests that repair wrote are no longer named here, because a test below
does what each one's paragraph asked for:
`TestEachClauseAnswersAloneAndTheirConjunctionIsTheVerdict`,
`TestAClauseSaysWhatItReadIncludingAnEmptyMeasurement` and
`TestEveryPredicateInThisCorpusIsTheConjunctionOfItsClauses`. A repair may
rewrite them into the tests below or remove them.

**A clause is a top-level conjunct and nothing deeper.** `a and b and c` is
three clauses. `(a and b) and c` is two, and the first keeps its parentheses.
`a or b and c` is one, and so is `not (a and b)`, because splitting either one
changes what the parts mean. An `and` inside a lambda's body or inside a string
is not a clause boundary. A clause's source is the bytes the author wrote
between its boundaries, trimmed at both ends and unchanged inside, so a clause
that spans lines keeps its line breaks and its indentation. It is a span of the
source and never a rendering of the tree: a reader matches it against the file
they are editing. Seventeen clauses in this corpus span lines on 2026-09-14,
four of them in this promise's own `hold_when`, because a YAML folded scalar
keeps the line break before a more-indented line.

**A clause answers what its own source answers.** Parsed as a predicate of its
own, a clause's source reaches the same value and the same reason as the clause
does in place, against any scope in which the whole predicate stays inside its
bounds. An unknown's position differs only by where the span starts. That ties the text a reader is shown to the answer shown beside
it, which the tree alone cannot: a clause answered from the right text and the
wrong subtree can agree in every test that only reads unknowns.

**The clauses' answers joined by `and` are the verdict.** The evaluator already
evaluates both sides of an `and` before it applies the table, so a clause
answered alone against the same scope gets exactly the value it has in place.
The rows that must not collapse are still `false and unknown` and `unknown and
false`, which are both `false`. A verdict is more than its value: it carries
the reason an unknown was reached and where, and which unknown survives a join
is the table's own rule, so a join restated over values alone can compare the
value and nothing else. So the package declares `And`, which joins verdicts
under the table the evaluator applies in place, and the corpus sweep joins
through it. It is Go, not language: nothing an author can write in a `hold_when`
changes.

**A bound belongs to the whole predicate, and a mistake belongs to its clause.**
`MaxSteps` is charged per evaluation, and a clause answered alone is an
evaluation with a budget of its own. A predicate refused at the bound in place
could then be answered clause by clause, each clause under the bound, and
joined into a verdict the predicate never reaches. That is
`TestExceedingABoundIsNeverAVerdict` broken through a second door. So when the
whole predicate is refused for exceeding a bound, answering its clauses is
refused too, as an error and never as an unknown. The opposite must survive the
fix. A predicate refused because one clause compares text with a number still
answers its other clauses, because which clause is wrong is what a trial exists
to show. Refusing every clause whenever the whole is refused would fix the bound
by losing that.

**What a clause read is reported as it was measured.** An empty list is
reported as the empty list, because a clause that holds over nothing is the one
a trial exists to show. A lambda's own variable is not a reading. A reference
nobody measured is reported as unmeasured rather than left out, since a missing
line and a missing measurement look alike to a reader. A clause reports what it
read and nothing its neighbors read, or a line under one clause names a value
another clause decided on.

One reference is not reported, and this promise does not owe it. A field
selected from a measurement that has no fields, like `obs.b.name.first` where
`obs.b.name` is text, is an expression mistake under 0036's rule rather than a
reading, and `Reads` leaves it out. Inside a fold over an empty list that can
happen in a clause that holds. What such an entry should carry, the evaluator's
error or the part of the reference that was measured, is a choice the printer
depends on, and
`desire:a-clause-reports-a-reading-that-failed-instead-of-leaving-it-out` is
that want.

## The named tests, and what each one must do

The predicate reads test *names*, and a test with the right name and no body
passes `go test`. So the contract is written here, where a reviewer compares it
with the diff. A test that does less than its paragraph says is a hollow
repair, whatever the verdict reads. Expected values are literals in the test,
never computed by the code under test. Each test is broken on purpose once
before it is trusted, and the break named under each one is the break it has to
catch.

- **`TestAClauseIsATopLevelConjunctWrittenExactlyAsAuthored`** is a table of
  expressions, each with the exact clause sources it must produce. It covers a
  chain of three, a parenthesized conjunction as a clause, an `or` at the top,
  `not (a and b)`, an `and` inside a lambda body and an `and` inside a string
  literal. *Break: split on every `and` token.*
- **`TestAClauseThatSpansLinesKeepsItsLineBreaksAndIndentation`** splits an
  expression shaped like a folded scalar with a more-indented continuation, such
  as `obs.a.ok`, then `and count(obs.b.items, i => i.ok`, then ten spaces and
  `and i.n == 0) == 1`, then `and obs.c.ok`, each on its own line. The middle
  clause's expected source is a literal holding the line break and the ten
  spaces inside it. That clause's source must also parse alone and answer what
  the clause answers in place. A table case whose line break falls between two
  clauses does not satisfy this. *Break: collapse the whitespace inside a
  clause.*
- **`TestTheClausesJoinedByAndAreTheVerdictInEveryRow`** has one subtest per row
  of the table, named for the row, such as `not-kept and unknown`: all nine
  ordered pairs of `kept`, `not-kept` and `unknown`, each a predicate of two
  clauses, and one of three clauses that are `kept`, `unknown` and `not-kept`.
  Each clause reads a different observation, so two unknowns have different
  reasons. For every row it asserts each clause's value and the whole
  predicate's value, both written as literals. It also asserts that `And` over
  the clauses' verdicts equals the whole predicate's verdict in value, reason and
  position. *Break: answer a clause that fails as unknown; separately, answer
  each clause from its neighbor's subtree.*
- **`TestEachClauseReportsWhatItReadAndNothingItsNeighborsRead`** uses a
  predicate of at least three clauses that read different observations, one of
  them nobody measured. For each clause it asserts the exact list of paths its
  readings name, and that the unmeasured one reads as unknown. A predicate of
  one clause, where a clause and its predicate are the same tree, does not
  satisfy this. *Break: walk the whole predicate instead of the clause.*
- **`TestAClauseOverAnEmptyMeasurementHoldsAndReportsTheEmptyList`** uses a
  predicate of several clauses, one of them a fold over a list measured empty.
  It asserts that clause's `Hold` is `kept`, and that its readings are exactly
  that list, reported as an empty list rather than unknown, with no reading for
  the lambda's own variable. *Break: have a clause answer anything but `kept`
  over an empty list, or report the empty list as unmeasured.*
- **`TestABoundBelongsToTheWholePredicateAndAMistakeToItsClause`** first builds
  a predicate of two clauses over a measured list, sized so that each clause's
  source parsed as a predicate of its own holds within `MaxSteps` and the whole
  predicate is refused. It asserts both premises. It then asserts that at least
  one clause's `Hold` is refused with an `*Error` and no value. Then a predicate
  of a clause that holds and a clause comparing text with a number: the whole
  is refused, the first clause answers `kept`, and the second is refused.
  *Break: give each clause a step budget of its own; separately, refuse every
  clause whenever the whole predicate is refused.*
- **`TestEveryClauseInThisCorpusIsASpanThatAnswersAloneAsItDoesInPlace`** loads
  the corpus this repository keeps, `docs/`, through `internal/corpus`, with one
  subtest per artifact that has a `hold_when`, named by the artifact's id. It
  lives in the external test package, because `internal/corpus` imports this one.
  For each artifact:
  - every clause's source is found in the authored `hold_when`, in order, with
    only whitespace and one `and` between neighbors and nothing else around
    them. No whitespace is normalized on either side.
  - each clause parses alone, and answers alone the value and reason it answers
    in place, with an unknown's position offset by where its span starts.
  - `And` over the clauses' verdicts equals `Hold`'s verdict.

  All of that holds against two scopes, each a subtest of the artifact's own.
  `nothing gathered` is a scope where nothing was gathered. In `every reading
  measured`, every reference the clauses report reading holds a value under
  which its clause evaluates. The sweep fails outright if it found no
  predicate, if no clause it checked spans lines, or unless, across the corpus,
  clauses answered each of `kept`, `not-kept` and `unknown` and some whole
  predicate answered `not-kept`. *Break: point it at an empty directory;
  separately, answer only against the scope where nothing was gathered.*

**What is mechanical and what is not.** The predicate names two rows of the
table, and three of the sweep's artifacts, each in the scope where every reading
is measured. A sweep that does not enumerate the real corpus does not produce
those names, and neither does one that answers only where nothing was gathered.
A test that claims them by hand is visibly not a sweep. The package must run
with nothing skipped, so a `t.Skip` does not satisfy a name. `Clauses` and `And`
must be declared by the package's own code: `api` reads no `_test.go`, so a join
written as a test helper does not count, and a repair that adds only tests does
not read `kept`.

`testcode` reads what the package's test files reference, and it requires a
reference to `And` through this package's own import path. Only an external test
package in this directory can make one, and the sweep is the test that has to
live there. So a sweep that restates the table beside it and never calls `And`
reads `not-kept`. That is a check on code, not on a label. It is `go/symbols`,
which `document:model`'s autonomy table puts in the structural row beside
`go/tests`, so the promise's grade does not move. A `command` instrument
searching the test file's text would have no row in that table, and the promise
would become ungraded.

What none of this can see is an assertion that is missing. The review sees
that, and so does the adversarial pass. So does the desire this unblocks: its
candidate measures `kept shape`'s output end to end, and an evaluator whose
clauses disagree with its verdict cannot pass it.

**The repair bound is the package and nothing else.** `internal/predicate/**`
renders as a grant for `internal/predicate`. The corpus the sweep reads is
outside the bound on purpose, so a repair cannot make the sweep pass by editing
a predicate. The language does not grow: no builtin, operator or syntax is
added. `hold_when` binds, and 0036 says a builtin arrives when a promise needs
one. This promise needs an answer per clause, not a new way to write one, and
`And` is Go API.
