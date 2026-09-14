---
id: the-predicate-evaluator-answers-for-each-clause-of-a-hold-when
fields:
  want: the predicate evaluator answers for each clause of a hold_when on its own
  becomes: promise
  candidate:
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
  shaped_from:
    - desire:shape-trial-explains-each-clause-instead-of-dumping-instruments
    - "internal/predicate/parse.go: every node records only where it starts, and a binary node's position is its operator's"
---

This want was split out of
`desire:shape-trial-explains-each-clause-instead-of-dumping-instruments`.
Explaining a trial clause by clause needs two changes. One is in the language:
the evaluator must answer for each clause alone and say what it read. The other
is in the verb: `kept shape` must print that answer. The language half comes
first, because a repair bounded to `cmd/kept` could split a `hold_when` only by
writing a second parser.

The candidate above is written out as
`promise:each-clause-of-a-predicate-answers-on-its-own`, which says in its own
links that it realizes this want. The two should agree. Where they differ, the
promise file is the one a person ratifies, and this candidate is the record of
how it was shaped. The promise's body holds the contract for its named tests.
