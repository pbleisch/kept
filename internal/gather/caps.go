package gather

import "github.com/pbleisch/kept/internal/ledger"

// MaxRawOutput bounds what an extractor may read.
//
// Two bounds, because they fail differently. This one is on the raw output a
// gatherer produced, so that a runaway program cannot exhaust memory before any
// other layer runs; [ledger.MaxObservation] is on the encoded record, so that a
// well-formed extractor cannot write a hundred megabytes into a permanent,
// tracked, append-only record. Capping only the second would be no cap at all:
// the extractor runs first, so a program emitting gigabytes wedges the tick
// before the record is ever encoded. See docs/decision/0034.
//
// It is generous on purpose. A test run's output is the ordinary case and it is
// large; what this stops is the pathological one.
const MaxRawOutput = 8 << 20

// Bound refuses a payload over one of the two caps.
//
// Exceeding either is a **gather failure**, so the verdict is unknown and the
// receipt says which bound and by how much. Reporting not-kept instead would be
// the confusion the three-valued verdict exists to prevent: a program that
// printed too much is a machine that could not be measured, not a promise that
// was broken.
//
// This is the layer that bounds what the absence of redaction can cost. The
// other three of docs/decision/0023's four are the declared fields, the
// explicit extractor, and redaction — and redaction is deferred deliberately,
// so an authored extractor can still capture a secret into a declared field.
// That is a known accepted cost, and the first thing to revisit before this
// repository is shared.
func Bound(what string, size, limit int) error {
	if size <= limit {
		return nil
	}
	return &ledger.TooLarge{Bound: what, Size: size, Limit: limit}
}
