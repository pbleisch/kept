package ledger

import (
	"time"

	"github.com/pbleisch/kept/internal/run"
)

// Version is the ledger record envelope version.
//
// It is on every record from the first durable write, because a reader holding
// a shape it cannot identify guesses, and guesses wrong exactly once. Adding it
// afterwards is impossible: the records that would need it are the ones already
// written. See docs/decision/0013.
const Version = 1

// Streams. Acts stay a single stream because they are a spine — order
// matters, volume is low, and an act means little without the ones around it.
// The word is act rather than decision: decision is the corpus's, and a name
// that meant both would make kept why ambiguous about which it answered. See
// docs/decision/0039.
//
// Truth and dispositions are per artifact and arrive with increment 3; see
// docs/layout.md for why they are split and this one is not.
const Acts = "acts.jsonl"

// Kind says what a record is. It is on the envelope rather than inferred from
// which fields are set, so a reader never has to guess and a record with a kind
// this build does not know is quarantined rather than read as something else.
type Kind string

const (
	// Ratified grants standing to one content version. It is the only way
	// standing is created, and only a person performs it.
	Ratified Kind = "ratified"
	// Released retires standing. Also a person's act, and the only way a
	// promise stops being held to — deleting its file is not one, because an
	// absence cannot be told apart from a bad merge. See docs/decision/0021.
	Released Kind = "released"
)

// Envelope is on every record in every stream.
//
// Signing reserves three more names — sig, sig_alg and sig_by — and they are
// deliberately not declared here. Adding a JSON field later is additive, and
// fields that are always empty are machinery for an adversary this system does
// not have: one principal, on their own machine. See docs/decision/0024, which
// pins what would be signed so that turning it on costs nothing.
type Envelope struct {
	// V is the shape this record was written under.
	V int `json:"v"`
	// Kind is what happened.
	Kind Kind `json:"kind"`
	// At is when it happened. Reads fold by this first.
	At time.Time `json:"at"`

	// Writer is the run that appended this. Attribution lives in a field rather
	// than in a path, so no stream is partitioned by which process wrote it and
	// two runs never choose one file. The run's own diagnostics are at
	// runs/{id}.json and are prunable; a Writer that resolves to nothing means
	// they aged out, not that this record is in doubt. See docs/decision/0012.
	Writer run.ID `json:"writer"`
	// Seq is monotonic within Writer. It recovers causal order inside one run
	// independent of the clock, which matters twice: two records written in one
	// tick can carry the same event time, and a clock stepped backwards makes
	// pure event-time folding wrong on a single machine. See docs/decision/0013.
	Seq uint64 `json:"seq"`
}

// Act is what creates or retires standing. Ratify and release today; land and
// delegate join them without changing this shape.
//
// It carries no world. Standing keys on a content hash, which is
// world-independent by construction, so a ratification means the same thing in
// every tree it is read in — which is why it needs nothing added for
// distribution, ever, and truth does. See docs/decision/0013.
type Act struct {
	Envelope

	// Artifact is "type:slug", the authored identity. Not a path: renaming the
	// file changes nothing, and this record still names the same artifact. See
	// docs/decision/0021.
	Artifact string `json:"artifact"`

	// Hash is the content version this act is about, as "sha256:…", and
	// Norm is the canonicalization rules it was computed under. Both are
	// required on a ratification, because standing attaches to a content
	// version and a later refinement of the rules has to re-hash old records
	// under the version they were written with. See docs/decision/0022.
	Hash string `json:"hash,omitempty"`
	Norm int    `json:"norm,omitempty"`

	// By is who performed it. Ratifying and releasing are a person's acts and
	// may not be delegated, so this is a person.
	By string `json:"by"`
	// Note is what they said about it, if anything.
	Note string `json:"note,omitempty"`
}

// Quarantined is a line that could not be read as a record.
//
// It is reported and never deleted or repaired. A torn line is what a crash
// mid-append leaves, and a fold that halted on one would make a single crash
// stop the system permanently; a fold that guessed at one would invent a
// act nobody made. Neither is acceptable, so it is set aside and said out
// loud. See docs/decision/0018.
// record is what every stream's records have in common. It exists so the fold
// and the parse are written once: two streams folding by different rules would
// be a bug that looks correct in every single-run test.
type record interface{ envelope() Envelope }

func (e Envelope) envelope() Envelope { return e }

type Quarantined struct {
	Stream string
	// Line is 1-based, as an editor counts.
	Line int
	Raw  string
	Why  string
}
