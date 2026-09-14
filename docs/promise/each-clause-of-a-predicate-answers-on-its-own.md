---
id: each-clause-of-a-predicate-answers-on-its-own
type: promise
title: Each clause of a predicate answers on its own
fields:
  statement: >-
    internal/predicate reads a hold_when as the clauses a top-level and joins, and answers
    for each one alone: every clause carries the source it was written as and the
    observations it read, and the clauses' answers joined by and are the verdict the whole
    predicate reaches.
  gather:
    tests:
      gatherer: go/tests
      scope: ["internal/**", "docs/**"]
      packages: "./internal/predicate"
    api:
      gatherer: go/symbols
      scope: ["internal/predicate/**/*.go"]
  hold_when: >
    obs.tests.ok and obs.api.ok
    and count(obs.tests.packages, p => p.package == "github.com/pbleisch/kept/internal/predicate"
              and p.ok and p.skipped == 0) == 1
    and count(obs.api.defines, d => d.package == "github.com/pbleisch/kept/internal/predicate"
              and d.kind == "method" and d.name == "Clauses") == 1
    and "TestAClauseIsATopLevelConjunctWrittenExactlyAsAuthored" in obs.tests.cases
    and "TestEachClauseAnswersAloneAndTheirConjunctionIsTheVerdict" in obs.tests.cases
    and "TestAClauseSaysWhatItReadIncludingAnEmptyMeasurement" in obs.tests.cases
    and "TestEveryPredicateInThisCorpusIsTheConjunctionOfItsClauses/promise:the-verdict-does-not-read-the-agent" in obs.tests.cases
    and "TestEveryPredicateInThisCorpusIsTheConjunctionOfItsClauses/promise:the-readme-names-every-verb" in obs.tests.cases
  trigger: cadence
  max_age: 7d
  repair:
    scope: ["internal/predicate/**"]
links:
  - { target: desire:the-predicate-evaluator-answers-for-each-clause-of-a-hold-when, relation: realizes }
  - { target: desire:shape-trial-explains-each-clause-instead-of-dumping-instruments, relation: cites }
  - { target: decision:0036-the-predicate-language, relation: cites }
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
expression is how two instruments come to disagree about what a promise says.

**A clause is a top-level conjunct and nothing deeper.** `a and b and c` is
three clauses. `(a and b) and c` is two, and the first keeps its parentheses.
`a or b and c` is one, and so is `not (a and b)`, because splitting either one
changes what the parts mean. An `and` inside a lambda's body or inside a string
is not a clause boundary. A clause's source is the bytes the author wrote
between its boundaries, trimmed at both ends and unchanged inside, so a clause
that spans lines keeps its line breaks. It is a span of the source and never a
rendering of the tree: a reader matches it against the file they are editing.

**The clauses' answers joined by `and` are the verdict.** The evaluator already
evaluates both sides of an `and` before it applies the table, so a clause
answered alone against the same scope gets exactly the value it has in place.
The row that must not collapse is still `false and unknown`, which is `false`.
A clause reads the observations it names and gets `unknown` for one nobody
measured, as the whole expression does.

**What a clause read is reported as it was measured.** An empty list is
reported as the empty list, because a clause that holds over nothing is the one
a trial exists to show. A lambda's own variable is not a reading. A reference
nobody measured is reported as unmeasured rather than left out, since a missing
line and a missing measurement look alike to a reader.

## The named tests, and what each one must do

The predicate reads test *names*, and a test with the right name and no body
passes `go test`. So the contract is written here, where a reviewer compares it
with the diff. A test that does less than its paragraph says is a hollow
repair, whatever the verdict reads. Each test is broken on purpose once before
it is trusted, and the break named under each one is the break it has to catch.

- **`TestAClauseIsATopLevelConjunctWrittenExactlyAsAuthored`** is a table of
  expressions, each with the exact clause sources it must produce, written as
  literals in the test and never computed by the code under test. It covers a
  chain of three, a parenthesized conjunction as a clause, an `or` at the top,
  `not (a and b)`, an `and` inside a lambda body, an `and` inside a string
  literal, and a clause that spans lines. *Break: split on every `and` token.*
- **`TestEachClauseAnswersAloneAndTheirConjunctionIsTheVerdict`** uses one
  expression with a clause that holds, a clause that reads a field nobody
  measured, and a clause that fails. It asserts each clause's value, `kept`,
  `unknown` and `not-kept`, and that the whole expression is `not-kept`.
  *Break: answer a clause against an empty scope.*
- **`TestAClauseSaysWhatItReadIncludingAnEmptyMeasurement`** checks that a
  clause folding over a list measured empty holds and reports that list with
  its empty value. It also checks that a lambda variable read inside that clause
  is not reported, and that a reference nobody measured is reported as
  unmeasured. *Break: drop readings that are unknown.*
- **`TestEveryPredicateInThisCorpusIsTheConjunctionOfItsClauses`** loads the
  corpus this repository keeps, `docs/`, through `internal/corpus`, with one
  subtest per artifact that has a `hold_when`, named by the artifact's id. For
  each one: every clause's source is a span of the authored `hold_when`; it
  parses alone and holds alone to the value it holds in place; and the clauses'
  values joined under the three-valued `and` equal `Hold`'s verdict, at least
  against a scope where nothing was gathered. It fails outright if it found no
  predicate. *Break: point it at an empty directory.*

**What is mechanical and what is not.** The predicate names two of the sweep's
subtests. A sweep that does not enumerate the real corpus does not produce them,
and a test that only claims those names by hand is visibly not a sweep. The
package must run with nothing skipped, so a `t.Skip` does not satisfy a name.
`Clauses` must be a method the package declares, so a repair that adds only
tests does not read `kept`. What none of this can see is an assertion that is
missing. The review sees that, and so does the adversarial pass. So does the
desire this unblocks: its candidate measures `kept shape`'s output end to end,
and an evaluator whose clauses disagree with its verdict cannot pass it.

**The repair bound is the package and nothing else.** `internal/predicate/**`
renders as a grant for `internal/predicate`. The corpus the sweep reads is
outside the bound on purpose, so a repair cannot make the sweep pass by editing
a predicate. The language does not grow: no builtin, operator or syntax is
added. `hold_when` binds, and 0036 says a builtin arrives when a promise needs
one. This promise needs an answer per clause, not a new way to write one.
