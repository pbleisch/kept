// Package thread owns the repair-thread boundary: it assembles immutable
// packets, appends native runtime receipts, and lets the parent supervise a
// confined child. Cutting a box, gathering, assessment and landing join that
// boundary without giving a runtime access to the ledger or a verdict.
//
// The packet states the gap — statement, why, observation, scope, prior
// attempts — and not what to do about it. Enumerating repairs was the mistake
// the predecessors made; see docs/decision/0009.
//
// The assessor reads the ledger receipt and cannot write. It answers a
// different question from the predicate: not "is this kept now" but "was this
// a legitimate way to get here". See docs/decision/0006.
package thread
