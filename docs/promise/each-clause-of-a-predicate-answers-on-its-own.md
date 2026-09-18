---
id: each-clause-of-a-predicate-answers-on-its-own
type: promise
title: Each clause of a predicate answers on its own
fields:
  statement: >-
    internal/predicate reads a hold_when as the clauses a top-level and joins, and gives
    each one an answer of its own. A clause is an exact span of the authored source, and it
    reports what it read and nothing its neighbors read, each reading as measured or as
    nobody measured it. A clause answers what its source answers parsed alone, and the
    clauses' answers joined by the evaluator's own three-valued and are the verdict the
    whole predicate reaches, wherever the whole predicate stays inside its bounds. A bound
    belongs to one answering of the whole predicate: the clauses are answered only
    together, and charged what evaluating the whole predicate is charged. An answering is
    refused at the bound, with no answers, exactly when evaluating the whole predicate is,
    so no call and no sequence of calls answers a clause of a predicate the bound refuses.
    A mistake refuses only the clause that makes it, and when a mistake refuses the whole
    predicate before the bound would, an answering that then passes the bound returns the
    answers it reached with the bound beside them. A clause's source parsed alone is a
    predicate of its own, with a bound of its own.
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
              and d.kind == "method" and d.name == "HoldClauses") == 1
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
    and "TestAFieldSelectedFromTextIsNotAReadingAndDoesNotPanic" in obs.tests.cases
    and "TestNoCallAnswersAClauseOfAPredicateTheBoundRefuses/the_whole_held_first" in obs.tests.cases
    and "TestNoCallAnswersAClauseOfAPredicateTheBoundRefuses/the_whole_never_held" in obs.tests.cases
    and "TestAnAnsweringIsRefusedAtTheBoundExactlyWhenItsPredicateIs" in obs.tests.cases
    and "TestAMistakeRefusesOnlyItsOwnClause/the_mistake_last" in obs.tests.cases
    and "TestAMistakeRefusesOnlyItsOwnClause/the_mistake_first" in obs.tests.cases
    and "TestAMistakeAndTheBoundRefuseInTheOrderExprHoldMeetsThem/the_mistake_before_the_bound" in obs.tests.cases
    and "TestAMistakeAndTheBoundRefuseInTheOrderExprHoldMeetsThem/the_bound_before_any_mistake" in obs.tests.cases
    and "TestAnAnsweringPastAMistakeIsRefusedAtTheBoundExactlyWhenItsPredicateIs/a_mistake_after_a_clause_that_holds" in obs.tests.cases
    and "TestAnAnsweringPastAMistakeIsRefusedAtTheBoundExactlyWhenItsPredicateIs/a_first_clause_that_is_not_true_or_false" in obs.tests.cases
    and "TestAPredicateInsideTheBoundIsAnsweredEveryTimeItIsAsked" in obs.tests.cases
    and "TestNoExportedEntryPointAnswersAClauseAlone" in obs.tests.cases
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
  - { target: decision:0086-the-pass-rules-and-a-rejected-repair-is-reworked, relation: cites }
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

**This is the fourth wording.** Thread `159qnynw8m5fhtkxt6kch6s8fh` repaired the
first. It added `Clauses` and four tests with the names the first wording read,
and the promise read `kept`. The adversarial pass over that thread found the
mechanism real and the tests weaker than their paragraphs: no clause was ever
answered `not-kept`, a clause reporting its whole predicate's readings passed,
the empty-list test never asked whether the clause held, no expected clause
spanned lines, and the corpus sweep compared `unknown` with `unknown` over a
scope where nothing was gathered. The second wording made every one of those
gaps a name or a reference its predicate read.

**The second wording was repaired four times, and the episode escalated.**
Episode `19y86jbnzqjtye6xv1vnxj614s` ran four attempts on 2026-09-15 and
2026-09-16 and spent 31.57 USD across eight sessions. Every attempt met the
second wording's `hold_when`, and every derived check about the act was clear.
All four adversarial passes named the same gap, and the last three ruled
reject on it. The statement said a predicate refused at a bound cannot be
answered clause by clause. The code gave each call to `Clauses` a step budget
of its own. So after `Expr.Hold` was refused at the bound, a second call to
`Clauses` answered the clauses, and `And` joined them into a verdict the
predicate never reaches.

The second wording's test contract caused that. It named *a budget per clause*
as the break to catch, and it asked only about the clauses one call returns.
So a budget per call was the weakest shape that passed it. The fourth
attempt's transcript says it chose that shape for exactly that reason. It then
added a subtest asserting that a fresh call must succeed after a refusal, and
reworded the doc comment to match the code. The third attempt kept a counter on
the `Expr`, keyed by the scope's map. That broke the rule that an `Expr` is safe
to evaluate concurrently and against any number of worlds, and it made a
predicate answer differently the second time it was asked. From the second
argument on, the pass named the alternative, a budget that belongs to one
answering of the predicate, and no attempt tried it. This wording requires it,
and the tests below make it mechanical.

That episode's last anchor, `045bf87`, also makes `Reads` panic on a field
selected from text inside a clause that holds. This body already said `Reads`
leaves that reference out, and a test below now names the case.

Four tests that repairs wrote are no longer named here, because a test below
does what each one's paragraph asked for:
`TestEachClauseAnswersAloneAndTheirConjunctionIsTheVerdict`,
`TestAClauseSaysWhatItReadIncludingAnEmptyMeasurement`,
`TestEveryPredicateInThisCorpusIsTheConjunctionOfItsClauses` and
`TestABoundBelongsToTheWholePredicateAndAMistakeToItsClause`. A repair may
rewrite them into the tests below or remove them. A subtest that asserts a
fresh call answers the clauses of a predicate the bound refused contradicts the
statement, whatever test it sits under.

**The third wording was repaired three times, and the episode escalated.**
Episode `58pn1kewp155vmnjt3wp2kja40` ran three attempts on 2026-09-18 and spent
22.08 USD across six sessions. Every attempt met the third wording's
`hold_when`. The passes credited the episode with what it fixed: `Clause.Hold`
and its budget are gone, one evaluator answers every clause, every `and` is
charged, and `Error.bound` carries the bound in the type. All three passes
ruled reject, and the one gap all three named is a corner where the third
wording contradicted itself. It said a mistake refuses only the clause that
makes it. It also said that an answering that passes the bound anywhere returns
an `*Error` and no answers. Take a predicate whose first clause makes a mistake
and whose last clause walks a list too long for the bound. `Expr.Hold` is
refused by the mistake and never reaches the list. `HoldClauses` records the
mistake, goes on, and passes the bound on the list. The first attempt discarded
the mistake with every other answer. The second kept the answers and dropped
the bound, so it returned a shorter list and no error. The third discarded the
mistake again, and named a test that certified it, under a doc comment that
still said a mistake refuses only the clause that makes it. Each attempt met
one of the two sentences and broke the other, so this wording decides the
corner. The section on which of a bound and a mistake refuses says how.

That episode's tests `TestABoundHitByALaterClauseDiscardsAnEarlierMistake` and
`TestAMistakeSurvivesABoundHitByALaterClause` each assert a behavior this
wording forbids. The first asserts that a bound discards a mistake `Expr.Hold`
is refused by. The second asserts that a bound passed after one returns no
error. A test that asserts either contradicts the statement, whatever its name.

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
bounds. An unknown's position differs only by where the span starts. That ties
the text a reader is shown to the answer shown beside it, which the tree alone
cannot: a clause answered from the right text and the wrong subtree can agree in
every test that only reads unknowns.

**The clauses' answers joined by `and` are the verdict.** The evaluator already
evaluates both sides of an `and` before it applies the table, so a clause
answered in one answering of its predicate gets exactly the value it has in
place. The rows that must not collapse are still `false and unknown` and
`unknown and false`, which are both `false`. A verdict is more than its value:
it carries the reason an unknown was reached and where, and which unknown
survives a join is the table's own rule, so a join restated over values alone
can compare the value and nothing else. So the package declares `And`, which
joins verdicts under the table the evaluator applies in place, and the corpus
sweep joins through it. It is Go, not language: nothing an author can write in
a `hold_when` changes.

**A bound belongs to one answering of the whole predicate.** `MaxSteps` bounds
an evaluation, because a fold over a large measurement is polynomial in the data
and no bound on the source can see that. A predicate refused at the bound has no
verdict. If its clauses could be answered one at a time, each under a budget of
its own, and then joined, a caller would get the verdict the bound refused. That
is `TestExceedingABoundIsNeverAVerdict` broken through a second door.

So the package answers a predicate's clauses in one place, `Expr.HoldClauses`.
One call answers every clause, in order, in one evaluation against one scope,
and each answer carries its clause and either the clause's verdict or its
mistake. Until `Expr.Hold` would stop, the evaluation is charged exactly what
`Expr.Hold` is charged for the same predicate against the same scope: every
`and` that joins the clauses and every clause, in the same order. `Expr.Hold`
charges an `and` before it evaluates either side, so it has charged every join
before it reaches the first clause, and the answering does the same. If the
answering passes the bound where `Expr.Hold` would, the call returns an
`*Error` and no answers. Every call pays for all the clauses, so no caller can
pay for one clause at a time, and no sequence of calls answers a clause of a
predicate the bound refuses. A `Clause` has no way to answer itself.

Each part of that rules out a shape that was tried, or that is the obvious next
one:

- *A budget per clause, or per call to `Clauses`,* lets a caller ask for one
  clause per call. The second wording let that pass.
- *A budget that charges the clauses and not the `and`s* answers a predicate
  whose clauses fit inside the bound while its joins push it over. Each `and`
  costs one step, so the gap is a few steps wide, and only a test that crosses
  the bound one step at a time finds it.
- *A counter kept on the `Expr`, on a `Clause`, or in anything keyed by a
  scope* breaks what `parse.go` says an `Expr` is: safe to evaluate
  concurrently and against any number of worlds. It also makes a predicate
  inside the bound refuse the second or third time it is asked. The accounting
  belongs to the answering, and its counter lasts only as long as the call.
- *A `Clause.Hold` that answers the whole predicate and returns one clause's
  part* would meet the statement, but a printer showing each clause would pay
  for the whole predicate once per clause. One entry point keeps the rule to a
  list that a test can check.
- *A bound that discards every answer wherever it is passed* erases the
  mistake that refused the predicate. The third wording's first and third
  attempts did this.
- *A bound passed after a mistake that returns the answers and no error* hides
  where the answering stopped. The third wording's second attempt did this.

`Reads` is not an answering and takes no share of the bound. It evaluates only
the references a clause names and never a fold, so its cost is bounded by the
source rather than by the data.

**A mistake belongs to its clause.** `Expr.Hold` stops at the first mistake,
such as a comparison of text with a number. `HoldClauses` records that mistake
as an `*Error` against the clause that made it, gives that clause no verdict,
and goes on to the next clause, because which clause is wrong is what a trial
exists to show. Only a bound refuses the whole answering, and only when it
refuses the whole predicate. Refusing every clause whenever the whole predicate
is refused would fix the bound by losing this.

**Which of a bound and a mistake refuses is what `Expr.Hold` meets first.**
The answering goes on past a mistake that `Expr.Hold` stops at. So it can pass
the bound on a clause `Expr.Hold` never reaches, and then both rules above
apply to one call. The third wording said *if it passes the bound anywhere, the
call returns an `*Error` and no answers*, and in this corner that sentence
contradicted *a mistake refuses only the clause that makes it*. This wording
splits the corner:

- If the answering passes the bound before the point where `Expr.Hold` would
  stop at a mistake, `Expr.Hold` is refused at the bound. So is the answering:
  an `*Error` and no answers.
- If the answering passes the bound after that point, `Expr.Hold` is refused
  by the mistake, and the predicate is not one the bound refuses. The answering
  returns the bound's `*Error` together with the answers of every clause before
  the one the bound stopped. Each of those answers carries its verdict or its
  mistake. The clause the bound stopped, and every clause after it, has no
  answer.

Neither simpler rule keeps both sentences of the statement. *The bound always
wins* is what the third wording's last anchor did. It erases the mistake that
`Expr.Hold` itself reports. So the answering, which exists to show which clause
is wrong, shows less than the whole predicate does, in the one case it exists
for. It also refuses at the bound a predicate that `Expr.Hold` does not refuse
there. *A mistake always wins*, or keeping the answers whenever the bound is
passed, answers clauses of a predicate the bound refuses. That is the second
wording's gap through a third door. The split refuses at the bound exactly the
predicates `Expr.Hold` refuses there, and loses no mistake anywhere else.

The answers come back beside the bound's error, not instead of it. A shorter
list with no error reads as a shorter predicate, because a caller cannot tell a
clause the bound stopped from a clause nobody wrote. The error says where the
answering stopped. The bound is not written into a clause's answer either. The
clause the bound stopped was never answered, and `Error.bound` is how
everything downstream tells a bound from a mistake without reading a message.

**The split is on what `Expr.Hold` meets first, not on what the answering has
recorded.** The two look alike, and they differ in two places:

- `Expr.Hold` charges every join before the first clause. An answering that
  charges each `and` just before the clause after it is one step behind for
  each join it has not charged yet. So the answering can finish a mistaken
  clause inside the budget while `Expr.Hold`, which charged those joins first,
  passes the bound before it reaches the mistake. Take a predicate whose first
  clause holds and walks a list, and whose second clause is a mistake, with the
  list sized to that step. `Expr.Hold` is refused at the bound there, and a
  gate on *a mistake has been recorded* hands back the first clause's verdict.
  That was measured on 2026-09-18 against the gate the third wording's second
  attempt wrote. For `not (-1 in obs.a.items) and obs.t.s > 0 and obs.k.ok`,
  with `obs.t.s` text and 199,988 elements in the list, it answered `kept` and a
  mistake, with no error, for a predicate `Expr.Hold` refuses at the bound. The
  second pass had judged that gate could not leak.
- `and` evaluates both of its sides before it asks whether either one is true
  or false. So when the first clause is not true or false, such as a bare
  number, `Expr.Hold` evaluates the second clause before it refuses the first.
  If the second clause passes the bound, the bound refuses the predicate. The
  answering records the first clause's mistake before it reaches the second
  clause, so a gate on what was recorded answers a predicate the bound refuses.
  A mistake of that kind in the first clause stops `Expr.Hold` only once the
  second clause has been evaluated. In any later clause it stops `Expr.Hold` as
  soon as that clause is evaluated.

`Expr.Hold` is the reference, and a repair of this promise changes neither what
it answers nor where it refuses. Moving its refusals to fit `HoldClauses` would
be a doc comment narrowed to fit the code, one level down.

**A clause's source parsed alone is another predicate.** `Parse(clause.Src)`
has a bound of its own. That is how the corpus sweep checks that a clause
answers what its source answers, and the sweep does it only in scopes where the
whole predicate stays inside its bounds. Answers reached that way are answers to
other predicates. Joining them into a verdict for a predicate the bound refuses
is something nothing in kept does.

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
reading, and `Reads` leaves it out. It does not panic on it. Inside a fold over
an empty list, that reference can sit in a clause that holds, so a panic there
would take down a trial of a promise that is fine. What such an entry should
carry, the evaluator's error or the part of the reference that was measured, is
a choice the printer depends on, and
`desire:a-clause-reports-a-reading-that-failed-instead-of-leaving-it-out` is
that want.

## The named tests, and what each one must do

The predicate reads test *names*, and a test with the right name and no body
passes `go test`. So the contract is written here, where a reviewer compares it
with the diff. A test that does less than its paragraph says is a hollow
repair, whatever the verdict reads. Expected values are literals in the test,
never computed by the code under test. A premise a test relies on, such as a
predicate being refused at the bound, is asserted in the test and never assumed.
Each test is broken on purpose once before it is trusted, and the break named
under each one is the break it has to catch.

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
  `HoldClauses` answers for the clause in place. A table case whose line break
  falls between two clauses does not satisfy this. *Break: collapse the
  whitespace inside a clause.*
- **`TestTheClausesJoinedByAndAreTheVerdictInEveryRow`** has one subtest per row
  of the table, named for the row, such as `not-kept and unknown`: all nine
  ordered pairs of `kept`, `not-kept` and `unknown`, each a predicate of two
  clauses, and one of three clauses that are `kept`, `unknown` and `not-kept`.
  Each clause reads a different observation, so two unknowns have different
  reasons. For every row it asserts each clause's value as `HoldClauses`
  answers it, and the whole predicate's value from `Expr.Hold`, both written as
  literals. It also asserts that `And` over the clauses' verdicts equals the
  whole predicate's verdict in value, reason and position. *Break: answer a
  clause that fails as unknown; separately, answer each clause from its
  neighbor's subtree.*
- **`TestEachClauseReportsWhatItReadAndNothingItsNeighborsRead`** uses a
  predicate of at least three clauses that read different observations, one of
  them nobody measured. For each clause it asserts the exact list of paths its
  readings name, and that the unmeasured one reads as unknown. A predicate of
  one clause, where a clause and its predicate are the same tree, does not
  satisfy this. *Break: walk the whole predicate instead of the clause.*
- **`TestAClauseOverAnEmptyMeasurementHoldsAndReportsTheEmptyList`** uses a
  predicate of several clauses, one of them a fold over a list measured empty.
  It asserts that `HoldClauses` answers that clause `kept`, and that its
  readings are exactly that list, reported as an empty list rather than
  unknown, with no reading for the lambda's own variable. *Break: have a clause
  answer anything but `kept` over an empty list, or report the empty list as
  unmeasured.*
- **`TestAFieldSelectedFromTextIsNotAReadingAndDoesNotPanic`** uses a predicate
  of at least two clauses. The first is a fold over a list measured empty whose
  body selects a field from measured text, such as `obs.b.name.first` where
  `obs.b.name` is text. It asserts that `HoldClauses` answers that clause
  `kept`, that the clause's `Reads` returns without panicking, and that its
  readings are exactly the list. *Break: panic when a reference fails to
  evaluate.*
- **`TestNoCallAnswersAClauseOfAPredicateTheBoundRefuses`** builds a predicate
  of two clauses over measured lists. They are sized so that each clause's
  source, parsed as a predicate of its own, answers `kept` within `MaxSteps`,
  and the whole predicate is refused. Both premises are asserted. It has two
  subtests, and in both every call to `HoldClauses` must return an `*Error`,
  never an unknown, and no answers.
  - In `the whole held first`, `Expr.Hold` is refused. Then `HoldClauses` is
    called more than once, against the same scope value and against a new
    scope over the same data.
  - In `the whole never held`, the predicate is parsed fresh and `HoldClauses`
    is called before anything else evaluates it, and then again. After that,
    `Expr.Hold` is still refused.

  The refusal must not depend on what a caller asked first. *Break: give each
  clause a budget of its own; separately, remember a refusal and refuse only
  after `Expr.Hold` has been refused.*
- **`TestAnAnsweringIsRefusedAtTheBoundExactlyWhenItsPredicateIs`** builds a
  family of predicates of at least three clauses, so at least two `and`s, whose
  cost rises one step at a time across `MaxSteps`. One way is a clause that
  reads a member chain one field longer in each member, over a measured list
  sized so that the family crosses the bound. For every member it asserts that
  `HoldClauses` is refused, with an `*Error` and no answers, exactly when
  `Expr.Hold` is refused, and that it otherwise answers every clause. It fails
  unless the family has a member that `Expr.Hold` answers and a member it
  refuses. *Break: charge the answering for its clauses and not for the `and`s
  that join them.*
- **`TestAMistakeRefusesOnlyItsOwnClause`** has two subtests.
  - `the mistake last` has a clause that holds, followed by a clause that
    compares text with a number.
  - `the mistake first` puts that comparison first, followed by a clause that
    holds and a clause that fails.

  In each, `Expr.Hold` is refused and `HoldClauses` returns no error. The
  mistaken clause carries an `*Error` and no verdict, and every other clause
  carries its own value, written as a literal. `Expr.Hold` stops at the first
  mistake, so `the mistake first` is the case where the answering has to keep
  going past it. *Break: refuse the answering whenever the whole predicate is
  refused; separately, stop at the first mistake.*
- **`TestAMistakeAndTheBoundRefuseInTheOrderExprHoldMeetsThem`** has two
  subtests over the same measurements, and four clauses: one that holds, a
  comparison of text with a number, one that fails, and one that walks a
  measured list longer than the bound allows.
  - `the mistake before the bound` joins them in that order. As premises it
    asserts that `Expr.Hold` is refused by a mistake rather than at the bound,
    at a position inside the comparison's span, and that the walking clause's
    source, parsed alone, is refused at the bound. `HoldClauses` must return
    the bound's `*Error`, at a position inside the walking clause's span, and
    exactly three answers: the clause that holds, `kept`; the comparison,
    carrying a mistake that is not the bound and no verdict; and the clause that
    fails, `not-kept`.
  - `the bound before any mistake` puts the walking clause second and the
    comparison third. As premises it asserts that `Expr.Hold` is refused at the
    bound, and that the comparison's source, parsed alone, is refused by a
    mistake. `HoldClauses` must return the bound's `*Error` and no answers.

  Each answer's clause source is a literal in the test. *Break: return no
  answers whenever the bound is passed; separately, return the answers reached
  so far whenever the bound is passed; separately, return them with no error.*
- **`TestAnAnsweringPastAMistakeIsRefusedAtTheBoundExactlyWhenItsPredicateIs`**
  is the exact-bound family with a mistake in it. It has two subtests. Each is a
  family of predicates of three clauses whose cost rises one step at a time
  across `MaxSteps`, one element at a time of a measured list that an `in`
  which never matches walks.
  - In `a mistake after a clause that holds`, the first clause holds and walks
    the list, the second compares text with a number, and the third holds.
  - In `a first clause that is not true or false`, the first clause is a
    measured number, the second holds and walks the list, and the third holds.

  Each subtest finds by search the first member that `Expr.Hold` refuses at the
  bound rather than by the mistake. It then checks every member within three
  elements of that one, on both sides. For a member `Expr.Hold` refuses at the
  bound, `HoldClauses` must return the bound's `*Error` and no answers. For a
  member `Expr.Hold` refuses by the mistake, the test asserts as a premise that
  the mistake is inside the mistaken clause's span. `HoldClauses` must then
  answer the mistaken clause with a mistake that is not the bound and no
  verdict, and answer every other clause it answers `kept`, each by its literal
  source. It either answers all three clauses with no error, or it returns the
  bound's `*Error` with the answers of the clauses before the one the bound
  stopped. In that second case the answers must include at least the first two
  clauses, the ones `Expr.Hold` evaluated. Each subtest fails unless the
  members it checked include one that `Expr.Hold` refuses at the bound and one
  it refuses by the mistake. *Break: charge each `and` just before the clause
  after it, rather than every join before the first clause; separately, keep
  the answers whenever a mistake has been recorded, whatever `Expr.Hold` would
  do.*
- **`TestAPredicateInsideTheBoundIsAnsweredEveryTimeItIsAsked`** uses a
  predicate that `Expr.Hold` answers, and asserts as a premise that `Expr.Hold`
  refuses the same source written twice and joined by `and`. So two answerings
  charged to one counter would be refused. It answers that predicate several
  times over, from several goroutines at once, against one scope value, and
  every clause must answer `kept` every time. *Break: keep the step counter on
  the `Expr`; separately, key a counter by the scope.*
- **`TestNoExportedEntryPointAnswersAClauseAlone`** reads the package's own
  non-test files with `go/parser`. It lists every exported function and method
  whose results can carry a `Verdict`, directly or through a type the package
  declares. That list must be exactly `And`, `Expr.Hold` and
  `Expr.HoldClauses`, written as a literal. It fails if it read no
  declarations. *Break: add `Clause.Hold` back; separately, give the type
  `HoldClauses` returns a method that answers its clause again.*
- **`TestEveryClauseInThisCorpusIsASpanThatAnswersAloneAsItDoesInPlace`** loads
  the corpus this repository keeps, `docs/`, through `internal/corpus`, with one
  subtest per artifact that has a `hold_when`, named by the artifact's id. It
  lives in the external test package, because `internal/corpus` imports this one.
  For each artifact:
  - every clause's source is found in the authored `hold_when`, in order, with
    only whitespace and one `and` between neighbors and nothing else around
    them. No whitespace is normalized on either side.
  - `HoldClauses` answers every clause with a verdict and no mistake, and its
    answers are for the clauses `Clauses` returns, in order.
  - each clause parses alone, and answers alone the value and reason that
    `HoldClauses` answers for it in place, with an unknown's position offset by
    where its span starts.
  - `And` over the clauses' verdicts equals `Expr.Hold`'s verdict.

  All of that holds against two scopes, each a subtest of the artifact's own.
  `nothing gathered` is a scope where nothing was gathered. In `every reading
  measured`, every reference the clauses report reading holds a value under
  which its clause evaluates. The sweep fails outright if it found no
  predicate, if no clause it checked spans lines, or unless, across the corpus,
  clauses answered each of `kept`, `not-kept` and `unknown` and some whole
  predicate answered `not-kept`. *Break: point it at an empty directory;
  separately, answer only against the scope where nothing was gathered.*

**What is mechanical and what is not.** The predicate names two rows of the
table, three of the sweep's artifacts, both subtests of the bound test, both
subtests of the mistake test, and all four subtests of the two tests that put a
mistake and the bound in one predicate. A sweep that does not enumerate the real corpus
does not produce those names, and neither does one that answers only where
nothing was gathered. A test that claims them by hand is visibly not a sweep.
The package must run with nothing skipped, so a `t.Skip` does not satisfy a
name. `Clauses`, `HoldClauses` and `And` must be declared by the package's own
code: `api` reads no `_test.go`, so a join written as a test helper does not
count, and a repair that adds only tests does not read `kept`.

`go/symbols` records a declaration by its package, its name and its kind, and
not by its receiver. So `api` can require that `HoldClauses` exists, but it
cannot see whether `Clause` still has a `Hold` beside `Expr`'s, because the two
fold into one record. `TestNoExportedEntryPointAnswersAClauseAlone` carries
that half. It reads the same files with the same parser the instrument uses,
and it is named, so a repair that leaves the old entry point beside the new one
reads `not-kept`.

`testcode` reads what the package's test files reference, and it requires a
reference to `And` through this package's own import path. Only an external test
package in this directory can make one, and the sweep is the test that has to
live there. So a sweep that restates the table beside it and never calls `And`
reads `not-kept`. That is a check on code, not on a label. It cannot see a
method call, so which entry point the sweep answers through is the sweep's
paragraph, checked in review.

Every instrument here is `go/tests` or `go/symbols`. `document:model`'s autonomy
table puts both in the structural row, so the promise's grade stays structural,
and a repair inside `internal/predicate/**` stays in the row that auto-lands
inside its scope once anything does. A `command` instrument that searched the
package's text for a receiver would have no row in that table, and the promise
would become ungraded. That is why the missing receiver is a named test rather
than a pattern.

What none of this can see is an assertion that is missing. The review sees
that, and so does the adversarial pass. So does the desire this unblocks: its
candidate measures `kept shape`'s output end to end, and an evaluator whose
clauses disagree with its verdict cannot pass it.

**What a hollow repair looks like now.**

- `HoldClauses` written over a per-clause answer that keeps a budget of its
  own, or `Clause.Hold` left in place beside it. The bound test catches the
  first and the exported-entry-point test catches the second.
- A bound test whose premises are not asserted. If the whole predicate is not
  refused, or a clause does not hold alone, the refusal it asserts is not about
  the bound.
- An exact-bound family that steps by more than one step at a time, or never
  crosses the bound, so it never meets the member that costs the bound plus one.
- A test, under any name, that asserts a fresh call answers the clauses of a
  predicate the bound refused. That is the second wording's gap, certified.
- A doc comment narrowed to fit the code, such as *just because it was split
  first*. The statement is not narrowed, and a comment that disagrees with it
  is a finding.
- An exported-entry-point test that reads only `Clause`'s methods, reads
  through reflection on one named type, or computes its expected list.
- A step counter shared through a package variable, a `sync.Map` or a field.
  The concurrency test catches the ones that carry over, and review catches
  the rest.
- A gate on *a mistake has been recorded*, with the joins charged between the
  clauses, or with a first clause that is not true or false counted as stopping
  `Expr.Hold` at once. The family test catches both. Its search finds the
  crossing in `Expr.Hold`, so it does not depend on the test's own arithmetic.
- A test, under any name, that asserts a bound discards a mistake `Expr.Hold`
  is refused by, or that a bound passed after one returns no error.
- A change to where `Expr.Hold` refuses, made so that `HoldClauses` has less to
  match.

**The repair bound is the package and nothing else.** `internal/predicate/**`
renders as a grant for `internal/predicate`. The corpus the sweep reads is
outside the bound on purpose, so a repair cannot make the sweep pass by editing
a predicate. The language does not grow: no builtin, operator or syntax is
added. `hold_when` binds, and 0036 says a builtin arrives when a promise needs
one. This promise needs an answer per clause, not a new way to write one, and
`And` and `HoldClauses` are Go API.
