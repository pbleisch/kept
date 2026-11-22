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

// retirement resolves an act kind to the one word this build reasons with.
//
// Two spellings reach a reader and one meaning leaves it. Everything past this
// point — the malformed-act check, the projection a gatherer reads, the
// register's fold, the exit codes and --json — sees `retire` and never has a
// second case to keep in step with the first. That is the same rule
// [landingWhy] is written for one layer over: one function asked at both ends,
// rather than two places deciding what an act means.
//
// It normalizes rather than distinguishing, and the era of an act is not lost
// by it: every act carries its own event time, and the bytes on disk are never
// rewritten, so the stream itself is still the answer to which word a record was
// written with. See docs/decision/0074.
func retirement(kind Kind) Kind {
	if kind == retiredHistorical {
		return Retired
	}
	return kind
}

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
	// Retired ends standing. Also a person's act, and the only way a promise
	// stops being held to — deleting its file is not one, because an absence
	// cannot be told apart from a bad merge. See docs/decision/0021. On a desire
	// it declines the want, and the file stays. See docs/decision/0083.
	//
	// It was spelled `released` until docs/decision/0074, which is why
	// [retiredHistorical] exists.
	Retired Kind = "retire"
	// Landed records that a thread's repair reached a landing target, gated
	// against the tree it landed on. It is a person's act too — a land happens
	// because somebody typed `kept land` — and it creates no standing whatever.
	//
	// What landed is Where.World.Commit and what it was gated against is
	// Where.World.Tree, because a land is performed in the gated tree and that
	// tree's commit is the thing that reached the ref.
	//
	// It is the one act kind that carries a [Landing]. The comment on [Act]
	// anticipated a kind joining the other two without adding a field, and this
	// is the exception rather than a revision of it: the three fields say which
	// of two guarantees a land carries, and that is a question no other act has.
	// See docs/decision/0015 and 0067.
	Landed Kind = "landed"

	// retiredHistorical is the value a retirement was written under until
	// docs/decision/0074, and it is exactly one line of one ledger: the
	// retirement of `decision:0070-first-of-a-pair` on 2026-08-25, which undid a
	// ratification performed by mistake.
	//
	// Nothing writes it and nothing but [retirement] reads it, so it looks
	// removable and is not. Deleting it would quarantine that act, which would
	// take it out of the fold, which would report an artifact a person retired
	// as still ratified — an act undone by a refactor. The ledger is append-only
	// and its bytes are never rewritten, so this constant is permanent in the
	// same way the line it reads is.
	retiredHistorical Kind = "released"

	// ThreadPacket records the immutable packet artifact a repair was given.
	// It is separate from an act: assembling a packet records preparation, not
	// a delegation or a claim that a repair happened. See decision 0052.
	ThreadPacket Kind = "thread_packet"
	// ThreadNative retains one redacted runtime event with its own native type.
	ThreadNative Kind = "thread_native"
	// ThreadTranscript points at one redacted transcript fragment.
	ThreadTranscript Kind = "thread_transcript"
	// ThreadCost records a runtime-reported cost update.
	ThreadCost Kind = "thread_cost"
	// ThreadTurn records a runtime-reported cumulative turn update.
	ThreadTurn Kind = "thread_turn"
	// ThreadModel records which model the runtime says answered.
	ThreadModel Kind = "thread_model"
	// ThreadWindow records the runtime's own reading of the usage windows in
	// force for the account it answered on. It is optional per adapter: a
	// runtime that reports no window writes none, and no window reported is not
	// a window that is empty. See docs/decision/0077.
	ThreadWindow Kind = "thread_window"
	// ThreadLifecycle records what the parent, not the runtime, observed of a
	// child process.
	ThreadLifecycle Kind = "thread_lifecycle"
	// ThreadAnchor records the commit a thread's repair produced, and the ref
	// holding it against gc. It is what a land is given: without it a repair's
	// tree is discarded with its box and nothing durable names it. See
	// docs/decision/0067.
	ThreadAnchor Kind = "thread_anchor"
	// ThreadMeasurement names one instrument's measurement of the promise in
	// the box, on one side of the session, by the address it is filed under in
	// the observation stream.
	//
	// It records no verdict. The packet already carries the ratified `hold_when`
	// this repair was authorized by, and a predicate applied to an observation
	// is derived on read wherever else kept asks the question — a verdict stored
	// beside the measurement it was reached from would be the cache that can
	// disagree with its source. What is not derivable is *which* measurements
	// these were, because a box is discarded and its tree is not walked again.
	// See docs/decision/0078.
	ThreadMeasurement Kind = "thread_measurement"
	// ThreadBrief names the immutable brief an adversarial pass was given: one
	// thread's ledger entry, frozen as that session received it. It is the
	// packet's counterpart for the other kind of work a confined session does,
	// and it is what delimits a pass inside an episode — the entries from one
	// input record to the next are one session's. See docs/decision/0081.
	ThreadBrief Kind = "thread_brief"
	// ThreadArgument names what an adversarial pass concluded.
	//
	// It is written after the session ends, so a pass killed at its deadline
	// leaves its transcript and no argument — the same guarantee an anchor has
	// against a repair that produced nothing. It carries no verdict, no
	// severity and no category: the pass's findings are unbounded and
	// unslotted, and a field here that graded them would be the ceiling
	// docs/decision/0078 exists to prevent.
	ThreadArgument Kind = "thread_argument"
	// ThreadUndecodable retains output a runtime wrote on its native stream that
	// its adapter could not read as one native event — a malformed line, or the
	// item the stream ended inside. It is not a native event with a type kept
	// made up, and it is not dropped: the bytes are what the runtime actually
	// wrote, and the moment they arrive is the only moment they can be kept. See
	// docs/decision/0084.
	ThreadUndecodable Kind = "thread_undecodable"
	// ThreadExecutable records a runtime's executable as the parent observed it,
	// once before the launch and once after the reap, so that a binary replaced
	// during a session is a comparison of two records. See docs/decision/0085.
	ThreadExecutable Kind = "thread_executable"
	// ThreadStderr retains the end of what a runtime wrote on its error stream,
	// redacted, once the process is reaped — which is where a runtime says why
	// it stopped when its native stream does not. See docs/decision/0085.
	ThreadStderr Kind = "thread_stderr"
	// ThreadSegment names one sealed span of this receipt: the artifact holding
	// those entries verbatim, and the first and last place in a run they came
	// from.
	//
	// It is how a receipt reaches the ledger without costing it the receipt's
	// own length on every line. While a session runs its entries are written to
	// a journal, and the ledger receives them in spans, each written once under
	// its content address and named by one entry here. The receipt stream keeps
	// its path and its envelope and becomes an index, so a receipt written
	// before this read exactly as it did; a read expands each span in place.
	// See docs/decision/0087.
	ThreadSegment Kind = "thread_segment"
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

// Act is what creates or retires standing, and what lands. Ratify, retire and
// land today; delegate joins them without changing this shape.
//
// It carries the world it was performed in, and that world is **advisory**:
// nothing reads it to decide standing. The reasoning this sentence replaced —
// *it carries no world, so it needs nothing added for distribution, ever* — was
// two claims, and only the first one has changed. Standing still keys on a
// content hash, which is world-independent by construction, so a ratification
// still means the same thing in every tree it is read in and still needs
// nothing added for distribution. What the world buys is not distribution: it
// is the one question an act could not answer about itself, which is whether
// the content it granted ever landed anywhere the reader can see. See
// docs/decision/0013 and docs/decision/0063.
type Act struct {
	Envelope
	Where

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

	// By is who performed it. Ratifying, releasing and landing are a person's
	// acts and may not be delegated, so this is a person.
	By string `json:"by"`
	// Note is what they said about it, if anything.
	Note string `json:"note,omitempty"`

	// Landing is on a landed act and on no other kind. Both halves of that are
	// checked: an act of another kind carrying one is refused as readily as a
	// land carrying none.
	Landing
}

// Landing is how a change arrived, on a [Landed] act. Three fields beside
// 0063's [Where], and none of them has a default.
//
// They are recorded rather than derived because none of them can be. The world
// names a tree and a commit and says nothing about where the change was going,
// so a reader working the arrival out from an empty field is a reader who
// eventually reads a worktree arrival as a land onto a ref — and those two do
// not carry the same guarantee. A ref advance carries *every commit that
// reaches a landing target was gated against the tree it landed on*; a worktree
// arrival puts no commit on a target at all and carries the narrower claim that
// the change was gated against exactly the tree it produced. One verb produces
// both, so the record is the only thing that keeps them distinguishable, and
// the ledger is append-only: a `landed` that does not say which it was is
// permanent.
//
// See docs/decision/0015 and [landingWhy], which is where the rules are.
type Landing struct {
	// Target is where the land was going, as a person names it: "origin/main"
	// for a shared target and "main" for a local one.
	Target string `json:"target,omitempty"`
	// Regime is the check the in-lock half made, and it follows from whether
	// the target has writers a local lock cannot exclude.
	Regime Regime `json:"regime,omitempty"`
	// Arrival is how the change got there.
	Arrival Arrival `json:"arrival,omitempty"`
}

// Regime is which in-lock check a land made. It follows from the target: a
// target naming a remote has writers no local lock can see — another machine,
// CI, a second checkout — and one naming none is written only by a process on
// this machine. See docs/decision/0015.
type Regime string

const (
	// SharedRegime: the target names a remote, so the in-lock check fetched
	// the target's refspec and re-read the tracking ref. Never the local
	// branch of the same name, which would answer about this machine when the
	// question is about the remote.
	SharedRegime Regime = "shared"
	// LocalRegime: the target names no remote, so the check was a ref read and
	// the land reached no network at all.
	LocalRegime Regime = "local"
)

// Arrival is how the change got to the target. One verb, two arrivals, the way
// git has a fast-forward and a merge under one `git merge`.
type Arrival string

const (
	// RefArrival: the target's ref moved to a commit kept made.
	RefArrival Arrival = "ref"
	// WorktreeArrival: the change was merged into a working tree and nothing
	// moved — no ref, no commit. It lands beside the operator's own edits.
	WorktreeArrival Arrival = "worktree"
)

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
