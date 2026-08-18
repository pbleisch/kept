// Package ledger is the append-only record: observations, verdicts,
// ratifications, delegations, threads and cost.
//
// Nothing here is derivable from anything else, which is why it is durable. It
// also holds standing and truth, keyed by content hash, so that the repairing
// agent — which can write the corpus — cannot write either.
//
// Three things it makes possible that a current-state store cannot: regressed
// (this held before), the never-fired signal (a promise kept two hundred times
// and never once broken is worth looking at), and cost attributed per promise.
package ledger
