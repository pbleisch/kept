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
const PacketVersion = 2

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
}

// Packet holds only the byte sequence assembled at dispatch. Its fields are
// unexported so a caller cannot alter the bytes after they have been addressed
// and before a runtime receives them.
type Packet struct{ bytes []byte }

// AssemblePacket freezes one packet from ratified authority and evidence. The
// caller must derive Ratified from the ledger before this boundary; a false
// value is refused here so an unratified artifact cannot accidentally be
// serialized as authority.
func AssemblePacket(in PacketInput) (Packet, error) {
	if why := packetWhy(in); why != "" {
		return Packet{}, errors.New(why)
	}

	// The anonymous wire shape makes the allow-list visible at the serialization
	// point. Adding a pipeline input means changing this type and its guard.
	wire := struct {
		V             int                           `json:"v"`
		Promise       RatifiedPromise               `json:"promise"`
		Why           string                        `json:"why"`
		Observations  map[string]ledger.Observation `json:"observations"`
		PriorAttempts []PriorAttempt                `json:"prior_attempts,omitempty"`
		Policies      []RatifiedDispatchPolicy      `json:"policies"`
	}{
		V:       PacketVersion,
		Promise: in.Promise,
		Why:     in.Why,
		// Copied, so the bytes are addressed from a set no caller still holds.
		// encoding/json sorts a string-keyed map, so a packet assembled twice
		// from one corpus is the same bytes without a sort here.
		Observations:  maps.Clone(in.Observations),
		PriorAttempts: append([]PriorAttempt(nil), in.PriorAttempts...),
		Policies:      append([]RatifiedDispatchPolicy(nil), in.Policies...),
	}
	bytes, err := json.Marshal(wire)
	if err != nil {
		return Packet{}, fmt.Errorf("encode packet: %w", err)
	}
	return Packet{bytes: bytes}, nil
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
	case len(in.Observations) == 0:
		return "packet carries no recorded measurement"
	}
	if why := measurementWhy(p.Gather, in.Observations); why != "" {
		return why
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
