package ledger

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/pbleisch/kept/internal/model"
)

// Disposed is what a disposition record is: what the tick did about one
// artifact that had standing and lacked truth, and why.
const Disposed Kind = "disposition"

// Outcome is what the tick did. Five, from docs/decision/0019.
type Outcome string

const (
	OutcomeAct      Outcome = "act"      // a thread opened
	OutcomeWait     Outcome = "wait"     // the policy said not yet
	OutcomeSuppress Outcome = "suppress" // the policy said not at all
	OutcomeEscalate Outcome = "escalate" // the policy said a person
	OutcomeRefused  Outcome = "refused"  // something else said no, and Condition says what
)

// Condition is what produced a refusal. Each comes from a different part of the
// system and none of them can be reconstructed afterwards: the budget's balance
// at that moment, the lock another run held then, and the rank among that tick's
// drift are all momentary.
type Condition string

const (
	Budget   Condition = "budget"   // open question 4
	Lock     Condition = "lock"     // docs/decision/0013
	Stale    Condition = "stale"    // the view is stale, so acting on standing is refused
	Policy   Condition = "policy"   // docs/decision/0009
	Rank     Condition = "rank"     // below the cut for this tick
	Provider Condition = "provider" // none configured, or the canary failed — 0017

	// Runtime is the repair runtime rather than the jail it runs in: nothing
	// configured on this machine, an adapter with no admission evidence, a
	// version the adapter is not pinned against, or a credential that has
	// lapsed. It is separate from Provider because it comes from a different
	// part of the system — decisions 0052 and 0057 rather than 0017 — and
	// because the two send a reader to different places. Collapsing them would
	// also make the transition rule blind to a box whose canary started passing
	// on the day its credential died. See docs/decision/0060.
	Runtime Condition = "runtime"
)

// Disposition is why the tick did or did not act.
//
// The design's premise is a queue that drains without you, and a queue is only
// trustworthy if the negative case is answerable. *Why did you not fix this* is
// the question an operator has at nine in the morning, and without this record
// the answer is a shrug: truth can be re-derived by observing again, and a
// refusal cannot.
//
// Transitions, not ticks. A drift refused on the same grounds for four hundred
// consecutive ticks is one record and a heartbeat.
type Disposition struct {
	Envelope

	Artifact string `json:"artifact"`
	// Hash is the content version this was about, so a disposition against a
	// promise as it was worded then is not read as one against its revision.
	Hash string `json:"hash,omitempty"`
	Norm int    `json:"norm,omitempty"`

	// World is where this was decided. It is **provenance, exactly like the
	// writer**: recorded because docs/decision/0014 says a record written
	// without one cannot be backfilled — a refusal in a box and one in the
	// workspace would otherwise be indistinguishable — and never part of the
	// key.
	//
	// kept why folds the whole stream and ignores it, and that is the decision
	// rather than an implementation detail. The question the verb exists for is
	// *why has this not been repaired for six days*, and the answer — the
	// budget refused on four of them and the policy suppressed on two — is
	// about the queue. Splitting it per world would fragment the one question
	// 0019 was written to make answerable, in a way that looks like more
	// precision. See docs/decision/0035.
	World model.World `json:"world"`

	Outcome Outcome `json:"outcome"`
	// Condition names what produced a refusal, and is empty otherwise.
	Condition Condition `json:"condition,omitempty"`
	// Detail is the human-facing reason drawn from that condition — "day limit
	// reached: 4 of 4 dispatches".
	Detail string `json:"detail,omitempty"`
}

// DispositionPath is where an artifact's dispositions live:
// disposition/{type}/{slug}.jsonl. Beside truth and never inside
// acts.jsonl: that stream is the spine, human acts at low volume, and
// machine output three orders of magnitude larger would drown the record a
// person actually reads.
func DispositionPath(artifact string) (string, error) {
	return streamPath(DispositionDir, artifact)
}

// Dispose records what the tick did about one artifact.
func (l *Ledger) Dispose(d Disposition) (Disposition, error) {
	if d.Artifact == "" {
		return Disposition{}, errors.New("a disposition names no artifact")
	}
	if d.World.Tree == "" {
		return Disposition{}, errors.New("a disposition names no world, and a record written without one cannot be backfilled")
	}
	if why := outcomeWhy(d); why != "" {
		return Disposition{}, errors.New(why)
	}

	where, err := DispositionPath(d.Artifact)
	if err != nil {
		return Disposition{}, err
	}

	l.seq++
	d.V = Version
	d.Kind = Disposed
	d.At = time.Now().UTC()
	d.Writer = l.run.ID
	d.Seq = l.seq

	line, err := json.Marshal(d)
	if err != nil {
		return Disposition{}, err
	}
	if err := l.declare(); err != nil {
		return Disposition{}, err
	}
	if err := l.store.Append(where, line); err != nil {
		return Disposition{}, err
	}
	return d, nil
}

// DisposeMoved records a disposition only when it is a change from what stood,
// or when what stood is older than maxAge.
//
// It is 0019's transition rule, and it lives here rather than in the caller
// because there is more than one caller: a tick disposes of every drift it saw,
// and `kept repair` disposes of the one drift a person asked about. Two
// implementations of *the same refusal on the same grounds is one record and a
// heartbeat* would disagree in the direction that puts the volume argument back.
//
// previous is the artifact's folded stream and maxAge is its own heartbeat
// interval; a zero maxAge means the artifact names none, and a repetition is
// then never re-recorded. The returned bool reports whether anything was
// written, and the returned record is what stands either way.
func (l *Ledger) DisposeMoved(d Disposition, previous []Disposition, maxAge time.Duration) (Disposition, bool, error) {
	last, held := LastDisposition(previous)
	heartbeat := held && maxAge > 0 && time.Since(last.At) > maxAge
	if !d.Moved(last, held) && !heartbeat {
		return last, false, nil
	}
	written, err := l.Dispose(d)
	if err != nil {
		return Disposition{}, false, err
	}
	return written, true, nil
}

// ReadDispositions returns an artifact's dispositions, folded into the order
// they were decided in.
//
// Every world's, and that is the point: kept why answers about the queue, and
// the answer *the budget refused on four days and the policy suppressed on two*
// is one question rather than one per tree.
func ReadDispositions(store Store, artifact string) ([]Disposition, []Quarantined, error) {
	where, err := DispositionPath(artifact)
	if err != nil {
		return nil, nil, err
	}
	src, err := store.Read(where)
	if err != nil {
		if errors.Is(err, ErrNoStream) {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	records, quarantined := parse(where, src, dispositionWhy)
	fold(records)
	return records, quarantined, nil
}

func dispositionWhy(d Disposition) string {
	if d.Kind != Disposed {
		return fmt.Sprintf("kind %q is not a disposition", d.Kind)
	}
	if d.Artifact == "" {
		return "names no artifact"
	}
	if d.World.Tree == "" {
		return "names no world, so a refusal in a box cannot be told from one in the workspace"
	}
	return outcomeWhy(d)
}

// outcomeWhy is the one rule a disposition has beyond naming its subject: a
// refusal says what refused.
//
// A refusal with no condition is the record 0019 exists to prevent, arriving in
// the shape of a record that is there — every reason is enumerable and comes
// from a different part of the system, and *refused* alone is the shrug this
// stream was written to replace.
func outcomeWhy(d Disposition) string {
	switch d.Outcome {
	case OutcomeAct, OutcomeWait, OutcomeSuppress, OutcomeEscalate:
		return ""
	case OutcomeRefused:
		if d.Condition == "" {
			return "a refusal that names no condition is the shrug this stream exists to replace"
		}
		return ""
	case "":
		return "says nothing about what the tick did"
	default:
		return fmt.Sprintf("%q is not a disposition this build knows", d.Outcome)
	}
}

// Moved reports whether a disposition is a change rather than a repetition. A
// change of disposition or of the condition behind it is a new record; the same
// refusal on the same grounds is the heartbeat's business.
//
// The detail is deliberately not compared. "3 of 4 dispatches" becoming "4 of
// 4" is the same refusal on the same grounds, and recording it would put the
// volume argument back that this rule exists to answer.
func (d Disposition) Moved(previous Disposition, found bool) bool {
	return !found || previous.Outcome != d.Outcome || previous.Condition != d.Condition
}

// LastDisposition is the disposition in force for an artifact, folded across
// every world.
func LastDisposition(records []Disposition) (Disposition, bool) {
	if len(records) == 0 {
		return Disposition{}, false
	}
	return records[len(records)-1], true
}
