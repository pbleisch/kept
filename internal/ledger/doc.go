// Package ledger is the append-only record: acts, observations, verdicts,
// dispositions and repair receipts. Its Store seam keeps the content-addressed
// packets and transcript fragments on the same side-ref boundary as receipts.
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
// In a Git workspace, the bytes live on refs/kept/ledger — a ref no working
// tree checks out, so no box can contain them. A local-only manuscript has no
// worktree to dispatch into and uses Files until it is committed to Git.
// [Store] keeps that mechanism choice out of every record's shape. See
// docs/decision/0016.
//
// A fetch never writes that ref. [FetchRefspec] puts what a remote holds on
// [UpstreamRef], which is the second ref git gives every branch, and [Advance]
// is what moves the durable record onto it — from the verbs that write, and
// from kept ledger sync. See docs/decision/0062.
//
// # Retrospective, and read by folding
//
// Nothing is recorded as an intention: observe, act, then record. A crash can
// lose the record of something that happened and can never produce a record of
// something that did not. See docs/decision/0018 — and note that the run's own
// diagnostics are the deliberate exception, because they are not a record of an
// act and are prunable.
//
// # Four streams, keyed four ways
//
// Acts are one stream, because they are a spine. Truth and dispositions
// are one file per artifact, with the world as a field on each record rather
// than a component of the path — a path per world would mint a tracked file
// every time anyone saved anything, since a world is a tree and a working tree
// changes constantly. Truth folds and then filters by world; a disposition is
// about the queue rather than about a tree, so kept why folds the whole stream
// and ignores it. See docs/decision/0035.
//
// Observations are content-addressed, one file per measurement, and the world
// they measured is part of the address. See docs/decision/0034.
//
// Streams merge by union, which interleaves arbitrarily, so every read folds by
// event time, then by the writer, then by seq — never by file order. A fold
// that depends on file order is silently wrong and looks correct in every
// single-run test. See docs/decision/0011.
//
// A line that will not parse is quarantined and reported rather than halting
// the read. One torn line after a crash must not be a system that stops.
package ledger
