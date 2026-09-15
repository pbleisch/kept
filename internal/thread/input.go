package thread

import "github.com/pbleisch/kept/internal/ledger"

// Input is what one confined session is handed on its input channel, and the
// record kept keeps of those bytes.
//
// The set is closed and it has two members, because there are two kinds of work
// a confined session does. A [Packet] states a gap a repair must close: ratified
// authority, the measurement that found the gap, and the paths a repair may
// change. A [Brief] states one thread's ledger entry for an adversarial pass to
// argue over: what the repair was authorized to do, what it did, what the two
// measurements say, what the session wrote, and what the derived checklist
// already found.
//
// Neither is the other's shape with a field added, and the guards say why. A
// packet is refused unless its authority is ratified and every instrument it
// names was measured; a brief has no authority and no gap, and must not be able
// to borrow a packet's. A caller that could hand one where the other belongs
// could dispatch a repair on evidence nobody granted.
//
// The unexported method closes the set to this package. An implementation from
// outside it would put bytes nothing here validated into the one channel a
// runtime reads.
type Input interface {
	sessionInput()

	// Bytes is an independent copy of exactly what the runtime receives.
	Bytes() []byte

	// Empty reports whether the input was never assembled.
	Empty() bool

	// Store records the immutable bytes before a launch. The returned
	// reference, never a rebuilt input, is what the receipt carries.
	Store(*ledger.Ledger) (ledger.ArtifactRef, error)

	// record names the stored bytes in the receipt, under the kind that says
	// which sort of session this was. It is unexported because the pairing of
	// a wire shape with its record kind is this package's and a caller that
	// could choose it could file a brief as a packet.
	record(*Receipt, ledger.ArtifactRef) error
}

// isBrief reports whether an input is an assessment's rather than a repair's.
//
// It is a function rather than a method on the interface, because what an
// adapter does about it differs: the one that has an adversarial pass pairs the
// input with its ask, and the two that do not refuse by name.
func isBrief(input Input) bool {
	_, brief := input.(Brief)
	return brief
}

// isRework reports whether an input is a packet carrying earlier attempts.
//
// The adapters with no rework ask refuse one by name, for the reason they refuse
// a brief: a repair's framing over a packet whose promise may already hold is a
// session that stops at once and looks exactly like one that did its work. See
// docs/decision/0086.
func isRework(input Input) bool {
	packet, ok := input.(Packet)
	return ok && packet.Rework()
}
