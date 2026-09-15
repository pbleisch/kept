package thread

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/model"
)

// PacketVersion is the shape of the immutable byte sequence a runtime sees.
// A packet cannot be reconstructed from a later corpus or ledger read, so its
// version is in the first packet rather than added when a runtime needs it.
//
// 2 carries every instrument's measurement under the name the promise gathers
// it by, where 1 carried one. It is a version rather than a second field beside
// the first: evidence a reader has to look for in two places is evidence a
// reader can miss in one of them, so the singular case is the plural case with
// one key. See docs/decision/0061.
//
// 3 adds `rework`, the gap of a second kind: a packet for a later attempt in an
// episode carries the attempts before it. It is a version rather than an
// optional field a version-2 reader would silently drop, because a reader that
// dropped it would read a rework as a repair of a promise that may already
// hold. See docs/decision/0086.
const PacketVersion = 3

// RatifiedPromise is the binding authority for the one promise a repair may
// address. It deliberately names only promise fields that bind, plus the hash
// a person ratified. Title, filename, desires, queue state and budgets have no
// route into a packet.
type RatifiedPromise struct {
	ID          string                      `json:"id"`
	Hash        string                      `json:"hash"`
	Norm        int                         `json:"norm"`
	Ratified    bool                        `json:"-"`
	Statement   string                      `json:"statement"`
	Gather      map[string]model.GatherSpec `json:"gather,omitempty"`
	HoldWhen    string                      `json:"hold_when,omitempty"`
	Trigger     model.Trigger               `json:"trigger"`
	RepairScope []string                    `json:"repair_scope"`
}

// RatifiedDispatchPolicy is a policy whose binding content applies before a
// dispatch. Policies that govern another act are authority for that act, not
// instructions a repair runtime may receive.
type RatifiedDispatchPolicy struct {
	ID        string                      `json:"id"`
	Hash      string                      `json:"hash"`
	Norm      int                         `json:"norm"`
	Ratified  bool                        `json:"-"`
	Statement string                      `json:"statement"`
	AppliesTo string                      `json:"applies_to"`
	Gather    map[string]model.GatherSpec `json:"gather,omitempty"`
	HoldWhen  string                      `json:"hold_when,omitempty"`
	OnFail    string                      `json:"on_fail"`
}

// PriorAttempt is evidence that a prior thread was opened for this promise.
// The thread id leads to its append-only receipt; it is not a work queue entry
// and carries no ranking, budget or proposed repair.
type PriorAttempt struct {
	Thread string `json:"thread"`
}

// Rework is the gap of a second kind: the promise may hold, and the way the
// earlier attempts of this episode got there was not accepted, for the reasons
// they carry. See docs/decision/0079 and 0086.
//
// It is evidence about acts that already happened — the change so far, and what
// the derived checks and the adversarial pass found about each attempt — and
// nothing else. There is no field for how many attempts remain, which action was
// selected or why, or anything a disposition says: those are the pipeline, and
// policy:the-pipeline-is-never-in-a-packet keeps them out. The session gets the
// findings and the argument, and what they mean is its work.
type Rework struct {
	// Base is the commit the episode's first box was cut from, and every anchor
	// in the episode is a change against it. Tip is the commit this box was cut
	// at: the tree the earlier attempts left.
	Base string `json:"base"`
	Tip  string `json:"tip"`

	// Touched and Diff are the episode's change so far, base to tip, and Elided
	// says what was left out when the diff was too large to carry.
	Touched []BriefPath `json:"touched,omitempty"`
	Diff    string      `json:"diff,omitempty"`
	Elided  string      `json:"elided,omitempty"`

	// Unmeasured is what measuring the promise in this box said when it reached
	// no verdict, and is empty when it did. A packet saying so carries no
	// observations: a box that measured nothing has no measurement to carry, and
	// borrowing one from a tree the agent is not standing in would be evidence
	// about somewhere else. See docs/decision/0014.
	Unmeasured string `json:"unmeasured,omitempty"`

	// Earlier is every earlier attempt in the episode, oldest first.
	Earlier []EarlierAttempt `json:"earlier"`
}

// EarlierAttempt is one earlier attempt of an episode, as the next one is shown
// it.
//
// Its argument is the adversarial pass's own words, whole. It is not
// summarized and not parsed; the ruling beside it is the one line kept read out
// of it, and it is here because the next attempt should know which way the
// case went as surely as a person does.
type EarlierAttempt struct {
	// Anchor is the commit this attempt left, and absent when it changed nothing.
	Anchor *Anchor `json:"anchor,omitempty"`
	// Rederived is what the promise read in the tree this attempt left, and empty
	// when nothing re-derived it.
	Rederived model.Value `json:"rederived,omitempty"`
	// Derived is every derived finding over the episode's act as it stood after
	// this attempt, in checklist order.
	Derived []BriefFinding `json:"derived,omitempty"`
	// Argument is what the adversarial pass over this attempt said, verbatim,
	// and Ruling is the ruling read from its last line. Both are empty when no
	// pass argued, and Ruling alone is empty when none could be read.
	Argument string `json:"argument,omitempty"`
	Ruling   Ruling `json:"ruling,omitempty"`
}

// PacketInput is intentionally a narrow constructor input. The absence of
// desires, ranks, budgets, dispositions and queue state is the enforcement of
// policy:the-pipeline-is-never-in-a-packet, rather than a convention a caller
// has to remember.
type PacketInput struct {
	Promise RatifiedPromise `json:"promise"`
	Why     string          `json:"why"`

	// Observations is what each of the promise's instruments measured, keyed by
	// the name the promise gathers it under.
	//
	// Keyed rather than ordered, and the key is the promise's own rather than a
	// new one: `gather` declares `graph` and `names`, `hold_when` reads
	// `obs.graph` and `obs.names`, and the packet carries `observations.graph`
	// and `observations.names`. A bare list would leave an agent matching
	// measurements to clauses by guessing, which is the one thing a promise with
	// two dimensions makes hard.
	//
	// Every instrument, always. A promise whose halves are coupled has no
	// failure attributable to one of them, and even where one is attributable
	// the other half is what says the repair must not break it. Sending the
	// failing half alone would be kept deciding which evidence matters, which is
	// a step toward enumerating the repair. See docs/decision/0009 and 0061.
	Observations map[string]ledger.Observation `json:"observations"`

	PriorAttempts []PriorAttempt           `json:"prior_attempts,omitempty"`
	Policies      []RatifiedDispatchPolicy `json:"policies"`

	// Rework is set for a later attempt in an episode, and nil for the first.
	Rework *Rework `json:"rework,omitempty"`
}

// Contents is the packet's wire shape: the whole of what a runtime is given,
// and the allow-list made visible at the point of serialization. Adding a
// pipeline input means changing this type and [packetWhy], which is what
// policy:the-pipeline-is-never-in-a-packet is enforced by rather than by a
// convention a caller has to remember.
//
// It is named and shared with [ReadPacket] rather than written twice, because a
// wire shape declared in two places is a wire shape that can disagree with
// itself — the reader would accept a field the writer never emits, or drop one
// it does.
//
// Ratified does not survive the round trip: it is `json:"-"` on both authority
// types, deliberately, because it is a claim the ledger makes about an artifact
// and not a property of the artifact that could be carried in bytes. A reader
// wanting it derives it from the ledger the way the writer did.
type Contents struct {
	V             int                           `json:"v"`
	Promise       RatifiedPromise               `json:"promise"`
	Why           string                        `json:"why"`
	Observations  map[string]ledger.Observation `json:"observations"`
	PriorAttempts []PriorAttempt                `json:"prior_attempts,omitempty"`
	Policies      []RatifiedDispatchPolicy      `json:"policies"`
	Rework        *Rework                       `json:"rework,omitempty"`
}

// ReadPacket reads back the bytes a runtime was given.
//
// It is the packet as evidence rather than the packet as input: the frozen
// authority a repair acted under, and the measurement that found the gap, both
// as they stood at dispatch. That is what makes an assessment of a thread stable
// after the promise it was about has been revised — which is the ordinary next
// step when an assessment fires. See docs/decision/0078.
//
// A version this build does not know is refused rather than read, because a
// reader holding a shape it cannot identify guesses, and guesses wrong exactly
// once.
func ReadPacket(src []byte) (Contents, error) {
	var out Contents
	if err := json.Unmarshal(src, &out); err != nil {
		return Contents{}, fmt.Errorf("read packet: %w", err)
	}
	if out.V == 0 {
		return Contents{}, errors.New("read packet: no packet version")
	}
	if out.V > PacketVersion {
		return Contents{}, fmt.Errorf("read packet: version %d, and this build reads %d", out.V, PacketVersion)
	}
	return out, nil
}

// Packet holds only the byte sequence assembled at dispatch. Its fields are
// unexported so a caller cannot alter the bytes after they have been addressed
// and before a runtime receives them.
type Packet struct {
	bytes  []byte
	rework bool
}

func (Packet) sessionInput() {}

// AssemblePacket freezes one packet from ratified authority and evidence. The
// caller must derive Ratified from the ledger before this boundary; a false
// value is refused here so an unratified artifact cannot accidentally be
// serialized as authority.
func AssemblePacket(in PacketInput) (Packet, error) {
	if why := packetWhy(in); why != "" {
		return Packet{}, errors.New(why)
	}

	wire := Contents{
		V:       PacketVersion,
		Promise: in.Promise,
		Why:     in.Why,
		// Copied, so the bytes are addressed from a set no caller still holds.
		// encoding/json sorts a string-keyed map, so a packet assembled twice
		// from one corpus is the same bytes without a sort here.
		Observations:  maps.Clone(in.Observations),
		PriorAttempts: append([]PriorAttempt(nil), in.PriorAttempts...),
		Policies:      append([]RatifiedDispatchPolicy(nil), in.Policies...),
		Rework:        cloneRework(in.Rework),
	}
	bytes, err := json.Marshal(wire)
	if err != nil {
		return Packet{}, fmt.Errorf("encode packet: %w", err)
	}
	return Packet{bytes: bytes, rework: in.Rework != nil}, nil
}

// cloneRework copies what a caller handed over, so the bytes are addressed from
// a value no caller still holds.
func cloneRework(in *Rework) *Rework {
	if in == nil {
		return nil
	}
	out := *in
	out.Touched = append([]BriefPath(nil), in.Touched...)
	out.Earlier = make([]EarlierAttempt, 0, len(in.Earlier))
	for _, attempt := range in.Earlier {
		copied := attempt
		if attempt.Anchor != nil {
			anchor := *attempt.Anchor
			copied.Anchor = &anchor
		}
		copied.Derived = append([]BriefFinding(nil), attempt.Derived...)
		out.Earlier = append(out.Earlier, copied)
	}
	return &out
}

func packetWhy(in PacketInput) string {
	p := in.Promise
	switch {
	case !p.Ratified:
		return "packet promise is not ratified"
	case p.ID == "":
		return "packet promise names no artifact"
	case !strings.HasPrefix(p.ID, "promise:"):
		return fmt.Sprintf("packet repair authority %q is not a promise", p.ID)
	case p.Hash == "" || p.Norm == 0:
		return "packet promise names no ratified content version"
	case p.Statement == "":
		return "packet promise has no binding statement"
	case len(p.RepairScope) == 0:
		return "packet promise grants no repair scope"
	case in.Why == "":
		return "packet has no why for the repair agent"
	case len(p.Gather) == 0:
		return "packet promise names no instrument, so nothing measured a gap for it to state"
	case len(in.Observations) == 0 && (in.Rework == nil || in.Rework.Unmeasured == ""):
		// A rework whose box measured nothing says so instead, and is the one
		// packet that may carry none. See [Rework.Unmeasured].
		return "packet carries no recorded measurement"
	}
	if len(in.Observations) > 0 {
		if why := measurementWhy(p.Gather, in.Observations); why != "" {
			return why
		}
	}
	if in.Rework != nil {
		if why := reworkWhy(*in.Rework, len(in.Observations)); why != "" {
			return why
		}
	}
	for _, attempt := range in.PriorAttempts {
		if attempt.Thread == "" {
			return "packet has a prior attempt naming no thread"
		}
	}
	for _, policy := range in.Policies {
		switch {
		case !policy.Ratified:
			return fmt.Sprintf("packet policy %q is not ratified", policy.ID)
		case policy.ID == "" || policy.Hash == "" || policy.Norm == 0:
			return "packet policy names no ratified content version"
		case !strings.HasPrefix(policy.ID, "policy:"):
			return fmt.Sprintf("packet authority %q is not a policy", policy.ID)
		case policy.Statement == "":
			return fmt.Sprintf("packet policy %q has no binding statement", policy.ID)
		case policy.AppliesTo != "dispatch":
			return fmt.Sprintf("packet policy %q applies to %q, not dispatch", policy.ID, policy.AppliesTo)
		case policy.OnFail != "refuse" && policy.OnFail != "warn":
			return fmt.Sprintf("packet policy %q has unknown on_fail %q", policy.ID, policy.OnFail)
		}
	}
	return ""
}

// reworkWhy refuses a rework that names no episode, carries no earlier attempt,
// or says two contradictory things about its own box.
//
// Both directions of the measurement rule are held, for the reason
// [measurementWhy] holds both of its own: a rework saying its box could not be
// measured and carrying a measurement anyway is one of those two statements
// being false, and a session has no way to tell which.
func reworkWhy(rework Rework, observations int) string {
	switch {
	case !objectID(rework.Base):
		return "packet rework names no base commit for the episode"
	case !objectID(rework.Tip):
		return "packet rework names no commit its box was cut at"
	case len(rework.Earlier) == 0:
		return "packet rework carries no earlier attempt, so there is nothing for it to rework"
	case rework.Unmeasured != "" && observations > 0:
		return "packet rework says its box could not be measured and carries a measurement"
	}
	for _, touched := range rework.Touched {
		if touched.Path == "" || touched.Status == "" {
			return "packet rework carries a changed path naming no path or no status"
		}
	}
	for _, attempt := range rework.Earlier {
		if why := earlierWhy(attempt); why != "" {
			return "packet rework " + why
		}
	}
	return ""
}

// earlierWhy holds one earlier attempt to evidence a later reader can follow.
// It is shared by a rework packet and a brief, which carry the same shape.
func earlierWhy(attempt EarlierAttempt) string {
	if attempt.Anchor != nil {
		if why := anchorWhy(*attempt.Anchor); why != "" {
			return "earlier attempt anchor " + why
		}
	}
	switch attempt.Rederived {
	case "", model.Kept, model.NotKept, model.Unknown:
	default:
		return fmt.Sprintf("earlier attempt re-derived %q, which is not a value a promise reads", attempt.Rederived)
	}
	for _, finding := range attempt.Derived {
		if finding.Check == "" || finding.Outcome == "" {
			return "earlier attempt carries a derived finding naming no check or no outcome"
		}
	}
	switch {
	case attempt.Ruling != "" && !attempt.Ruling.Known():
		return fmt.Sprintf("earlier attempt carries ruling %q, which is not one a pass can make", attempt.Ruling)
	case attempt.Ruling != "" && strings.TrimSpace(attempt.Argument) == "":
		return "earlier attempt carries a ruling and no argument it was read from"
	}
	return ""
}

// Rework reports whether this packet carries earlier attempts, which is what
// decides the ask it travels with. See docs/decision/0086.
func (p Packet) Rework() bool { return p.rework }

// measurementWhy holds a packet to carrying exactly the promise's instruments,
// each one measured.
//
// Both directions, because they are different failures. A gathered name with no
// measurement is evidence dropped between the box and the packet, and an agent
// reading `hold_when` would find a clause it has nothing to weigh. A measurement
// the promise does not gather is evidence about something no clause reads, which
// is data in a packet that nothing in the packet accounts for.
//
// Sorted, so the refusal names the same thing every time a packet is assembled
// from the same inputs. Map iteration is not an order, and a refusal message
// that varies is a refusal nobody can test against.
func measurementWhy(gather map[string]model.GatherSpec, observations map[string]ledger.Observation) string {
	for _, name := range slices.Sorted(maps.Keys(gather)) {
		observation, measured := observations[name]
		if !measured {
			return fmt.Sprintf("packet promise gathers %q and the packet carries no measurement for it", name)
		}
		if observation.V == 0 || observation.At.IsZero() || observation.Gatherer == "" || observation.World.Tree == "" {
			return fmt.Sprintf("packet measurement %q is not a recorded measurement", name)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(observations)) {
		if _, gathers := gather[name]; !gathers {
			return fmt.Sprintf("packet carries a measurement named %q, which the promise does not gather", name)
		}
	}
	return ""
}

// Bytes returns an independent copy of exactly what must be delivered. No
// adapter receives corpus or ledger inputs from which it could rebuild a later
// packet.
func (p Packet) Bytes() []byte { return append([]byte(nil), p.bytes...) }

// Empty reports whether AssemblePacket did not produce a packet.
func (p Packet) Empty() bool { return len(p.bytes) == 0 }

// Store records the immutable packet before a launch. The returned reference,
// not a rebuilt packet, is what the per-thread receipt carries.
func (p Packet) Store(l *ledger.Ledger) (ledger.ArtifactRef, error) {
	if p.Empty() {
		return ledger.ArtifactRef{}, errors.New("cannot store an empty packet")
	}
	return l.PutArtifact(ledger.ThreadPacketArtifact, p.bytes)
}

func (p Packet) record(r *Receipt, ref ledger.ArtifactRef) error { return r.RecordPacket(ref) }
