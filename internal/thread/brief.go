package thread

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"

	"github.com/pbleisch/kept/internal/ledger"
)

// BriefVersion is the shape of the immutable byte sequence an adversarial pass
// sees. It carries a version for the reason a packet does: what a session
// received cannot be rebuilt from a later corpus or ledger read, so the version
// is in the first brief rather than added when a reader needs one.
const BriefVersion = 1

// BriefFinding is one derived check as the pass is shown it.
//
// It is the closed half's shape restated on the wire, not the open half's. The
// pass may read what has already been established — an argument is better for
// knowing it and worse for having to re-establish it — and nothing flows the
// other way: a derived finding is never revised, softened or withdrawn by the
// argument, which is policy:the-agents-report-is-never-an-input-to-the-verdict
// at the seam between the two halves.
//
// It is declared here rather than shared with internal/assess because that
// package reads this one. The dependency runs assess → thread and never back:
// a wire shape in the package that owns the channel is what keeps a runtime's
// input from depending on the thing that derives half of it.
type BriefFinding struct {
	Check   string   `json:"check"`
	Outcome string   `json:"outcome"`
	Detail  string   `json:"detail"`
	Lines   []string `json:"lines,omitempty"`
}

// BriefContents is the brief's wire shape: the whole of what an adversarial
// pass is given.
//
// It is named and shared with [ReadBrief] rather than written twice, for the
// reason [Contents] is: a wire shape declared in two places can disagree with
// itself.
//
// There is deliberately no field for what the pass should look for, no ranking,
// no budget and no queue state. The framing is the declared ask of decision
// 0058, which travels on a channel of its own and says the same thing for every
// thread; what varies here is only the evidence.
type BriefContents struct {
	V int `json:"v"`

	// Thread is the episode this entry belongs to, and Promise is the artifact
	// the repair was authorized by. Both are carried because a brief is read
	// back on its own, out of an artifact store, with nothing around it.
	Thread  string `json:"thread"`
	Promise string `json:"promise"`

	// Packet is the frozen authority and evidence the repair session received:
	// the promise as it was ratified before this repair, its hold_when, its
	// gather, its bound scope, and the measurement that found the gap.
	//
	// It is the packet's own contents rather than a reconstruction from
	// today's corpus, and that is what makes an assessment stable across the
	// revision that ordinarily follows a finding.
	Packet Contents `json:"packet"`

	// Anchor is the commit the session left and the commit the box was cut
	// from, so the diff below is that commit's own against its parent.
	Anchor Anchor `json:"anchor"`

	// Touched is what the anchor's diff changed and how, and Diff is the diff
	// itself. Elided says what was left out when the diff was too large to
	// carry, because a brief that silently truncated its evidence would have a
	// pass arguing about a change it was shown half of.
	Touched []BriefPath `json:"touched,omitempty"`
	Diff    string      `json:"diff,omitempty"`
	Elided  string      `json:"elided,omitempty"`

	// Before and After are the promise's measurements on either side of the
	// session, keyed by the name the promise gathers each instrument under —
	// the same key its predicate reads and the same key the packet carries.
	// Either may be absent: a session killed at its deadline leaves no
	// re-derivation.
	Before map[string]ledger.Observation `json:"before,omitempty"`
	After  map[string]ledger.Observation `json:"after,omitempty"`

	// Transcript is what the repair session wrote, in the order it wrote it.
	// It is the evidence no derivation reaches — whether the session was
	// working toward the statement or toward the predicate is a reading of
	// what it said — and it is why the pass exists.
	Transcript string `json:"transcript,omitempty"`

	// Lifecycle is what the parent observed of the repair session's process,
	// in order. A session killed at its deadline and one that finished are
	// different acts, and a pass reading the second as the first would argue
	// about work that was interrupted rather than work that was done.
	Lifecycle []Lifecycle `json:"lifecycle,omitempty"`

	// Derived is what the closed checklist already answered, all of it, in
	// checklist order.
	Derived []BriefFinding `json:"derived,omitempty"`
}

// BriefPath is one path the repair changed, and how git reported it.
type BriefPath struct {
	Path   string `json:"path"`
	Status string `json:"status"`
	From   string `json:"from,omitempty"`
}

// BriefInput is the narrow constructor input. The absence of a desire, a rank,
// a budget, a disposition and any queue state is the enforcement of
// policy:the-pipeline-is-never-in-a-packet on this channel too: a brief is
// evidence about an act that happened, and the pass decides what to make of it.
type BriefInput struct {
	Thread     string
	Packet     Contents
	Anchor     Anchor
	Touched    []BriefPath
	Diff       string
	Elided     string
	Before     map[string]ledger.Observation
	After      map[string]ledger.Observation
	Transcript string
	Lifecycle  []Lifecycle
	Derived    []BriefFinding
}

// Brief holds only the byte sequence assembled at dispatch. Its field is
// unexported so a caller cannot alter the bytes after they have been addressed
// and before a runtime receives them.
type Brief struct{ bytes []byte }

func (Brief) sessionInput() {}

// AssembleBrief freezes one brief from a thread's ledger entry.
//
// What it refuses is small and it is the part that cannot be recovered later: a
// brief that names no thread is an argument about nothing in particular, one
// with no packet has no statement of what the repair was for, and one with no
// anchor is an argument about a change that does not exist. Everything else may
// legitimately be absent — a killed session leaves no re-derivation, a pruned
// observation leaves no measurement — and the pass is told which by the field
// being empty rather than by the brief refusing to exist.
//
// It checks no authority, because it carries none. A packet's guard exists so
// that a repair cannot act on a grant nobody made; a brief authorizes nothing
// and the session it feeds may write nothing, which is what a read-only box is
// for.
func AssembleBrief(in BriefInput) (Brief, error) {
	if why := briefWhy(in); why != "" {
		return Brief{}, errors.New(why)
	}
	wire := BriefContents{
		V:       BriefVersion,
		Thread:  in.Thread,
		Promise: in.Anchor.Promise,
		Packet:  in.Packet,
		Anchor:  in.Anchor,
		// Copied, so the bytes are addressed from a set no caller still holds.
		// encoding/json sorts a string-keyed map, so a brief assembled twice
		// from one entry is the same bytes without a sort here.
		Touched:    append([]BriefPath(nil), in.Touched...),
		Diff:       in.Diff,
		Elided:     in.Elided,
		Before:     maps.Clone(in.Before),
		After:      maps.Clone(in.After),
		Transcript: in.Transcript,
		Lifecycle:  append([]Lifecycle(nil), in.Lifecycle...),
		Derived:    append([]BriefFinding(nil), in.Derived...),
	}
	encoded, err := json.Marshal(wire)
	if err != nil {
		return Brief{}, fmt.Errorf("encode brief: %w", err)
	}
	return Brief{bytes: encoded}, nil
}

func briefWhy(in BriefInput) string {
	switch {
	case in.Thread == "":
		return "brief names no thread"
	case in.Packet.V == 0:
		return "brief carries no packet, so nothing says what the repair was authorized to do"
	case in.Packet.Promise.ID == "":
		return "brief packet names no promise"
	}
	if why := anchorWhy(in.Anchor); why != "" {
		return "brief anchor " + why
	}
	for _, finding := range in.Derived {
		if finding.Check == "" || finding.Outcome == "" {
			return "brief carries a derived finding naming no check or no outcome"
		}
	}
	for _, touched := range in.Touched {
		if touched.Path == "" || touched.Status == "" {
			return "brief carries a changed path naming no path or no status"
		}
	}
	return ""
}

// ReadBrief reads back the bytes an adversarial pass was given.
//
// A version this build does not know is refused rather than read, for the
// reason [ReadPacket] refuses one: a reader holding a shape it cannot identify
// guesses, and guesses wrong exactly once.
func ReadBrief(src []byte) (BriefContents, error) {
	var out BriefContents
	if err := json.Unmarshal(src, &out); err != nil {
		return BriefContents{}, fmt.Errorf("read brief: %w", err)
	}
	if out.V == 0 {
		return BriefContents{}, errors.New("read brief: no brief version")
	}
	if out.V > BriefVersion {
		return BriefContents{}, fmt.Errorf("read brief: version %d, and this build reads %d", out.V, BriefVersion)
	}
	return out, nil
}

// Bytes returns an independent copy of exactly what must be delivered.
func (b Brief) Bytes() []byte { return append([]byte(nil), b.bytes...) }

// Empty reports whether AssembleBrief did not produce a brief.
func (b Brief) Empty() bool { return len(b.bytes) == 0 }

// Store records the immutable brief before a launch.
func (b Brief) Store(l *ledger.Ledger) (ledger.ArtifactRef, error) {
	if b.Empty() {
		return ledger.ArtifactRef{}, errors.New("cannot store an empty brief")
	}
	return l.PutArtifact(ledger.ThreadBriefArtifact, b.bytes)
}

func (b Brief) record(r *Receipt, ref ledger.ArtifactRef) error { return r.RecordBrief(ref) }
