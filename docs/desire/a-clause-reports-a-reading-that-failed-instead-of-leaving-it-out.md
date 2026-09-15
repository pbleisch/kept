---
id: a-clause-reports-a-reading-that-failed-instead-of-leaving-it-out
fields:
  want: a clause reports a reading that failed instead of leaving it out
  becomes: promise
  candidate:
    statement: >-
      A clause of a hold_when reports every reference it reads. One whose reading is an
      expression mistake, a field selected from a measurement that has no fields, is
      reported as a reading that failed: never left out, and never shown as a value or as
      unmeasured.
    gather:
      tests:
        gatherer: go/tests
        scope: ["internal/**", "docs/**"]
        packages: "./internal/predicate"
    hold_when: >
      obs.tests.ok
      and count(obs.tests.packages, p => p.package == "github.com/pbleisch/kept/internal/predicate"
                and p.ok and p.skipped == 0) == 1
      and "TestAClauseReportsAReadingThatFailedInsteadOfLeavingItOut" in obs.tests.cases
    trigger: cadence
    max_age: 7d
    repair:
      scope: ["internal/predicate/**"]
  ripe_when: >-
    promise:each-clause-of-a-predicate-answers-on-its-own is ratified at the wording that
    names TestEachClauseReportsWhatItReadAndNothingItsNeighborsRead, and reads kept, so the
    readings this adds a third kind to are already attributed per clause and tested. Whoever
    writes it has also chosen what the entry carries: the evaluator's error, or the part of
    the reference that was measured.
  shaped_from:
    - "the adversarial pass over thread 159qnynw8m5fhtkxt6kch6s8fh, finding 7: Reads silently drops a reading it cannot evaluate"
    - "internal/predicate/clause.go at 78a34650f: Reads returns without a reading when evaluating a reference fails, under a comment saying this should not happen"
---

`Clause.Reads` evaluates each reference a clause makes and reports what it
found. When the evaluation fails, it reports nothing. That happens whenever a
reference selects a field from a measurement that has no fields. Measured at
`78a34650f` in a throwaway clone, with `obs.a.items` empty and `obs.b.name` the
text `"text"`:

```
all(obs.a.items, x => obs.b.name.first == x) and obs.c.ok
```

The whole predicate is `kept`, because the fold's body never runs over an empty
list. Its first clause is `kept`, and its readings are `obs.a.items` and nothing
else. `obs.b.name.first` is left out, although it is in the clause and it is
wrong for this measurement. That is the confusion
`promise:each-clause-of-a-predicate-answers-on-its-own` names: a missing line and
a missing measurement look alike to a reader.

## Why it is not in that promise

That promise's statement claims each reading as measured or as nobody measured
it. Under 0036 this is neither. A field read off something that has none is an
`*Error`, an expression that is wrong, and `Reading` has no channel for one. The
fix is not a test of something already claimed. It is a third kind of entry, and
its shape is a choice:

- **carry the evaluator's error** beside the path, so the entry says what is
  wrong in the author's own terms; or
- **report the part of the reference that was measured**, here `obs.b.name` as
  `"text"`, which is *as it was measured* and lets the reader see why `.first`
  reads nothing.

Both keep `Unknown` a value and `Error` an error, and neither is obviously
right. The reader that decides is
`desire:shape-trial-explains-each-clause-instead-of-dumping-instruments`, which
prints the entry. Choosing inside a repair packet would have a confined session
decide an API the next want depends on.

## What the candidate's test must do

- **`TestAClauseReportsAReadingThatFailedInsteadOfLeavingItOut`** uses a clause
  that holds over an empty list and whose lambda body selects a field from a
  measurement that has none. It asserts the clause's `Hold` is `kept`, and that
  its readings include an entry for that reference. The entry must be told
  apart from a measured value and from an unknown without reading a message.
  *Break: skip a reference whose evaluation fails, as `Reads` does today.*

Rewrite the candidate's statement and the test's paragraph for whichever shape
is chosen before trialling it.
