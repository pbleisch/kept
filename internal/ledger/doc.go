// Package ledger is the append-only record: decisions today, and observations,
// verdicts, threads and cost as they arrive.
//
// Nothing here is derivable from anything else, which is why it is durable. It
// also holds standing and truth, keyed by content hash, so that the repairing
// agent — which can write the corpus — cannot write either.
//
// Three things it makes possible that a current-state store cannot: regressed
// (this held before), the never-fired signal (a promise kept two hundred times
// and never once broken is worth looking at), and cost attributed per promise.
//
// # Record shape is one-way; storage is not
//
// A record is appended, never revised, and it is tracked, so what one says is
// permanent. Three fields exist from the first durable write because none of
// them can be added afterwards to records already written: the run that wrote
// it, a seq monotonic within that run, and the envelope version. See
// docs/decision/0013.
//
// Where the bytes live is the opposite kind of question. They are plain files
// under .kept/ledger today and belong on refs/kept/ledger — a ref no working
// tree checks out, so no box can contain them — before the first box exists.
// [Store] is that seam, and moving is a second implementation of it rather than
// a change to anything a record says. See docs/decision/0016.
//
// # Retrospective, and read by folding
//
// Nothing is recorded as an intention: observe, act, then record. A crash can
// lose the record of something that happened and can never produce a record of
// something that did not. See docs/decision/0018 — and note that the run's own
// diagnostics are the deliberate exception, because they are not a record of an
// act and are prunable.
//
// Streams merge by union, which interleaves arbitrarily, so every read folds by
// event time, then by the writer, then by seq — never by file order. A fold
// that depends on file order is silently wrong and looks correct in every
// single-run test. See docs/decision/0011.
//
// A line that will not parse is quarantined and reported rather than halting
// the read. One torn line after a crash must not be a system that stops.
package ledger
