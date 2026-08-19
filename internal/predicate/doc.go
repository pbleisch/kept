// Package predicate is kept's expression language: the lexer, the parser and
// the evaluator behind an artifact's hold_when and behind the extractor 0023
// requires of the command gatherer.
//
// It is hand-written and takes no dependency, and the reason is not dependency
// count. It is that the two ways an expression can fail must be different
// types. See docs/decision/0036.
//
// # The two failures
//
//   - [Unknown] is a **value**: the observation carries no such field, prior()
//     has no history, a program did not print the JSON it was going to be read
//     for. Nobody measured. A hold_when that evaluates to one is
//     model.Unknown, which is a verdict.
//   - [Error] is an **error**: the expression is wrong. A type mismatch, a
//     function nothing declares, a fold over something that is not a list. The
//     promise is refused and the person is told where.
//
// Nothing anywhere reads a message to tell those apart, and nothing may start
// to. `expr` was disqualified because a missing field evaluates to false there,
// which reports not-kept for a gatherer that could not measure; CEL was
// rejected because it returns both through one channel, so the two are told
// apart only by matching English prose in the code path that decides a verdict.
// Both failure modes are avoided here by construction rather than by care.
//
// # One grammar, two namespaces
//
// [HoldWhen] reads obs.<name>.<field> and prior(<name>). [Extractor] reads
// stdout, stderr and exit. The same parser and the same evaluator serve both,
// which is what makes 0023's requirement met without a second language — and
// [Expr.Extract] is where they differ, in one sentence: an extractor field that
// is Unknown is absent from the observation rather than present and unmeasured,
// so a predicate reading it later gets Unknown for its own reason.
//
// # What the language has
//
// Comparison, existence, set membership, a fold over a collection, and
// prior(). Nine builtins — has, all, count, filter, map, lines, json,
// json_lines, find_all — and prior() as a form of its own. The bound on a tenth
// is 0036's: one is added when a promise in this repository or in a manuscript
// needs it, and the refusal message for an unknown function says so. Languages
// accrete, and the bound is the only thing that stops this one.
//
// There is no arithmetic and no indexing. Both are additive later; neither can
// be taken away, because hold_when is authored in the corpus and bound by
// _schema/promise.yaml's hash list, so the syntax is effectively permanent.
//
// # prior()
//
// The previous observation of that gatherer, whatever verdict it produced,
// reaching back exactly one, **folded within a world lineage** — see [Observed].
// That last part is one line and it is silently wrong if it is omitted: without
// it, "the set of tests never shrinks" compares a repair's box against the
// origin and presents two trees as a change over time, which is the failure
// docs/decision/0014 was written to stop.
//
// It may not read another artifact's truth. That is refused by name, because
// evaluation order and cycles are real and nothing needs it yet.
//
// # Where it is checked
//
// Everything decidable without the world is decided by [Parse]: the syntax, the
// roots, the functions, their arity, where a lambda may appear, and whether a
// literal pattern compiles. [Expr.Check] adds what needs the artifact — that
// every obs.<name> names a gatherer the artifact declares — so a promise
// reading an observation nothing produces is refused when the corpus loads
// rather than in the code path that decides a verdict.
//
// That does not remove the runtime rule and is not allowed to: a gatherer can
// return less than it declared, so an absent field is Unknown at evaluation
// however well the reference checked.
//
// # Bounds
//
// [MaxSource], [MaxDepth], [MaxNodes] and [MaxSteps]. The first three bound the
// text and the fourth bounds the fold, because nested folds over a large
// observation are polynomial in the data and no limit on the text can see that.
// Each has a test that deliberately exceeds it. Exceeding one is an [Error] and
// never a verdict: a promise must not break because its own predicate was too
// expensive.
//
// If any declared gatherer produced no observation, no predicate runs and the
// verdict is Unknown. See docs/decision/0004 and 0005.
package predicate
