---
id: 0036-the-predicate-language
type: decision
title: 0036 — The predicate language is hand-written, three-valued, and does the extractor's job too
---

# 0036 — The predicate language is hand-written, three-valued, and does the extractor's job too

**Status** accepted · closes open question 1 · settles the extractor
[0023](0023-what-may-be-captured.md) requires of `command` and does not describe
· exists because `AGENTS.md` requires an ADR for an architectural change, not
because [0029](0029-dependencies-pass-a-test.md) requires one for a dependency —
the decision here is to take none

## Context

Open question 1 has been the block on increment 3 since the plan was written,
and `model.md` names it as the place the cost estimate is weakest: *"A small
expression language over structured observations has not [been built], and it is
where the cost estimate is weakest."* Four things were undecided: what `prior()`
returns, how far it reaches, whether it may read another artifact's truth, and
the grammar.

A fifth arrived with 0023 and was never connected to it. `command` must have an
explicit extractor naming what to pull out of a program's output. If that
extractor is this language, one decision covers both. If it is not, kept has two
languages — which is itself an argument for making one general enough.

**It is the same language, and this was established by building it.** One
grammar and one evaluator, over two namespaces — `stdout`, `stderr` and `exit`
for the extractor, `obs` for the predicate — extracted 95 named passing tests
from this repository's real `go test -json` output and then answered `hold_when`
over exactly those fields.

The one place the two jobs disagree is what a missing field means, and it is not
a difference of language. It is settled below.

**Why this is an ADR at all.** 0029 retired *a dependency gets an ADR* in favour
of a standing test, and under that test a hand-written evaluator needs no ADR,
because it admits no dependency. The requirement comes from `AGENTS.md` instead:
*an architectural change gets an ADR before the code*. A new expression language
is one — it is authored in the corpus, it is bound by the hash list, and every
promise ever written depends on its grammar. 0029 is cited below only where it is
genuinely the frame, which is the part that weighs the two libraries.

## Decision

### The language is hand-written

Not on dependency count. On semantics, and the semantics is the invariant this
whole system rests on:

> *A failed gatherer yields `unknown`, never `not-kept`. A machine that could
> not measure does not record what it thinks it saw.*

Both candidate libraries were installed and run against it.

**`expr` returns `false` for a missing field, with no error.** A gatherer that
did not report `failed` would make the promise `not-kept`, silently. Its option
list has `AllowUndefinedVariables`, which loosens, and nothing that tightens.
That is the exact false report the three-valued verdict exists to prevent,
arriving through the evaluator. It is out on evidence rather than on preference.

**CEL is right about all of that and has one error channel.** It returns an
error for a missing key, and its connectives are correct and commutative —
`false && <missing>` is `false` whichever side the missing value is on, and
`has()` is built in. What it cannot do is say **which kind** of failure
happened. Every runtime failure is a `*types.Err`, and the discriminator is the
message text:

```
obs.tests.coverage > 80     "no such key: coverage"    the observation lacks a field   -> unknown
obs.tests.failed.deeper     "no such key: deeper"      a field read off an integer     -> refuse
```

Two opposite meanings, one message shape. Telling them apart means matching
English prose in the code path that decides whether a promise is `unknown` or
unauthorable. Declaring a typed environment does not move either to compile
time, because a CEL map type says nothing about which keys it has, so the
obvious escape is closed.

Hand-written, the split is two Go types decided where the failure occurs, and
the three-valued logic is about forty lines:

```
obs.tests.failed == 0 and obs.tests.coverage > 80   => unknown
obs.tests.failed == 1 and obs.tests.coverage > 80   => not-kept
```

**One argument cuts the other way and deserves saying, because it is the
strongest thing against this decision.** The syntax is one-way either way:
`hold_when` is authored in the corpus and `_schema/promise.yaml` binds on it, so
changing languages later rewrites every promise and de-ratifies the corpus at
once. If the choice is permanent regardless, borrowing a specified, widely
implemented syntax is a real hedge against our own future mistakes. What
outweighs it is where CEL's single channel lands: telling *nobody measured* from
*this expression is wrong* is not a nicety here, it is the distinction the whole
three-valued model is built on. Two Go types beat prose for the thing the system
is about — the same argument as `unknown` falling out of the shape rather than
being bolted on.

Where 0029 *is* the frame, it does not separate the candidates. Criterion 4 puts
a predicate in the trust path, so either library would need an ADR of its own.
Criterion 2 asks whether we would get it wrong by hand, and the honest answer is
*the grammar no, the tests yes if we are careless*. Criterion 3 says the same
thing about every candidate: the implementation sits behind
`internal/predicate` and is a swap, but the **syntax** is behind nothing. What
actually separated them is whether the evaluator can distinguish two kinds of
failure, and 0029's test has no slot for that. Worth adding to the test, or at
least noting that it was the deciding factor and the test did not ask for it.

### It is three-valued, and `unknown` is a distinct outcome from an error

Two failure kinds, and they go to different places:

- **Unknown** — the observation carries no such field, `prior()` has no history,
  a list has nothing by that name. The verdict is `unknown`. Nobody measured.
- **Error** — the expression is wrong. A type mismatch, an unknown function, a
  fold over something that is not a list. The promise is refused and the person
  is told where.

Errors carry a position and are rendered against the source:

```
  all(obs.tests.ok, p => p)                  => ERROR
      all needs a list, and got a true/false
        all(obs.tests.ok, p => p)
        ^ at line 1, column 1
```

### The extractor is this language over a different namespace

`command` declares its fields as named expressions over `stdout`, `stderr` and
`exit`, and the observation carries those and nothing else, which is 0023's
requirement satisfied rather than approximated:

```yaml
fields:
  gather:
    tests:
      gatherer: command
      run: "go test -json ./..."
      fields:
        passed: 'count(json_lines(stdout), e => has(e, "Test") and e.Action == "pass")'
        failed: 'count(json_lines(stdout), e => has(e, "Test") and e.Action == "fail")'
        cases:  'map(filter(json_lines(stdout), e => has(e, "Test") and e.Action == "pass"), e => e.Test)'
  hold_when: obs.tests.failed == 0 and "TestRevisionDropsStanding" in obs.tests.cases
```

Seven builtins carry the extraction that the predicate does not need — `lines`,
`json`, `json_lines`, `find_all`, `map`, `filter`, `has` — and all seven are
ordinary functions in the same evaluator. Patterns are Go's `regexp`, which
cannot backtrack catastrophically; an extractor runs over bytes a program
produced, and an unbounded one is a way to wedge a tick.

**`has()` is what makes one language do both jobs.** A missing field is
`unknown` everywhere by default, which is right for a predicate and wrong for
extraction over a heterogeneous stream — `go test -json` emits package-level
events with no `Test` key, and a record lacking a field there is ordinary data
rather than a failure to measure. `has()` is how an expression says so. Without
it, the first honest attempt at that extractor returns `unknown` for the whole
observation, which is what the spike did before `has()` existed. It was found by
the spike failing, not by reasoning about it, which is the reason to trust it.

**An extractor binds.** It is written under `gather:`, and
`_schema/promise.yaml` names `gather` in its hash list, so changing one drops
the promise's standing. That is right — the promise now measures something else
— and it is worth saying out loud, because it is not where anyone would look for
it. It is also what makes [0034](0034-the-observation-record.md)'s deferral of
redaction defensible: an extractor that captures a secret was authored by a
person and cannot be changed without a person granting standing again.

### `prior()` returns the previous observation, reaches back one, and stays in its own world

**The previous observation of that gatherer, whatever verdict it produced.**
*The set of tests never shrinks* needs the previous measurement regardless of
whether the promise held, and that is the case the whole three-part model was
built for. A last-good variant is speculative; `prior_kept()` is additive if
something ever needs it.

**One step back.** A window needs a retention rule, an ordering rule and a
syntax, and increment 3's done-condition forces none of them. `prior(tests, 2)`
is additive later; guessing now is permanent in every predicate written under the
guess.

**Within one world lineage.** `prior()` folds the observations of that gatherer
whose thread matches the current one — empty matching empty — ordered by event
time. Without that rule, *never shrinks* compares a box against the origin and
presents two trees as a change over time, which is the second of the three
failures 0014 was written to stop. It is one line and it is silently wrong if
omitted, so it gets a test that deliberately interleaves a box's observations
with the workspace's.

**It may not read another artifact's truth.** Refused, with a message naming
open question 1, which is how [0031](0031-the-binding-preimage.md) handled
`body.<section>` and it worked. Evaluation order and cycles are real and nothing
needs it yet.

### A predicate's field references are checked when the corpus loads

0023 makes a gatherer declare its output. That declaration is worth more than
0023 claims: it lets kept check every `obs.<name>.<field>` against the gatherer
bound to `<name>` **at load**, and refuse a promise that reads a field nothing
produces — naming the field, the gatherer, and what the gatherer does declare.

That turns most of the missing-field problem from a runtime three-valued
question into an error a person can fix, and it would have done so whichever
language had been chosen. It does not remove the runtime rule, because a
gatherer can return less than it declared, which is why `unknown` remains the
default.

## Alternatives

**`expr-lang/expr`.** One module, no transitive dependencies, a good error
message, and the wrong answer for the case that matters. Rejected.

**`cel-go`.** The close call, and it is closer than the dependency count
suggests: correct three-valued connectives, `has()` built in, a written spec,
and no parser to fuzz. Rejected on the one error channel above, and secondarily
on what it admits — antlr, protobuf and two genproto modules, in the trust path
of a tool whose whole dependency record is two lines. If the one-channel problem
were solved upstream, this rejection should be revisited rather than treated as
settled.

**A library behind a strict adapter that converts absence to an error.**
Genuinely considered, and rejected on where the remaining work lands: the
adapter has to re-implement member access, which is where the distinction lives,
leaving the library supplying a parser. A parser is the part we can write.

**Predicates as Go plugins, or a fixed list of checks.** Rejected by 0004 and
0009 respectively: the first makes a promise unreadable by the person who
authored it, the second is the enumeration mistake.

## Consequences

- **kept owns a language**, and languages accrete. The bound is that a builtin
  is added when a promise in this repository or in a manuscript needs it, and
  the refusal message says so.
- **The tests are the deliverable, not the parser.** A fuzz corpus over the
  parser, a depth limit and a node-count cap so an authored expression cannot
  wedge a tick, and a case per three-valued row — including the two that must
  not collapse, `false and unknown` and `true and unknown`. That is the work;
  the grammar is about a thousand lines and was built in a spike.
- **One decision covers `hold_when` and the extractor**, so 0023's requirement
  is met without a second grammar, and increment 4's gatherers declare their
  fields the same way `command` does.
- **A promise naming a field nothing declares does not load**, which is a new
  class of corpus problem and belongs beside the ones `internal/corpus` already
  reports.
- **The syntax is effectively permanent.** It is authored in the corpus and
  bound by the hash list, so replacing it later rewrites every promise and drops
  every promise's standing at once. That is the cost accepted here, and it is
  the reason the alternatives were weighed rather than counted.
- **The escape hatch 0004 promised still works.**
  `gather: { check: { gatherer: command, run: "…" } }` with
  `hold_when: obs.check.exit == 0` is the first self-hosting promise, and it
  needs one builtin and no extractor at all.
