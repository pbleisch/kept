package thread

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/model"
)

func packetInput() PacketInput {
	return PacketInput{
		Promise: RatifiedPromise{
			ID:          "promise:kept-is-tested",
			Hash:        "sha256:promise",
			Norm:        1,
			Ratified:    true,
			Statement:   "the repository is tested",
			Gather:      map[string]model.GatherSpec{"tests": {Gatherer: "go/tests", Args: map[string]any{"scope": "internal/**"}}},
			HoldWhen:    "obs.tests.ok",
			Trigger:     model.TriggerCadence,
			RepairScope: []string{"internal/**"},
		},
		Why: "A repair must preserve the test boundary.",
		Observations: map[string]ledger.Observation{
			"tests": {
				V:        ledger.ObsVersion,
				At:       time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC),
				Gatherer: "go/tests@1",
				World:    model.World{Tree: "sha256:world"},
				Scope:    "sha256:scope",
				Data:     map[string]any{"ok": false},
			},
		},
		PriorAttempts: []PriorAttempt{{Thread: "prior-thread"}},
		Policies: []RatifiedDispatchPolicy{{
			ID: "policy:no-pipeline", Hash: "sha256:policy", Norm: 1, Ratified: true,
			Statement: "a packet carries no pipeline input", AppliesTo: "dispatch", OnFail: "refuse",
		}},
	}
}

func TestPacketContainsOnlyAuthorityAndEvidence(t *testing.T) {
	p, err := AssemblePacket(packetInput())
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(p.Bytes(), &wire); err != nil {
		t.Fatal(err)
	}
	for _, allowed := range []string{"v", "promise", "why", "observations", "prior_attempts", "policies"} {
		if _, found := wire[allowed]; !found {
			t.Errorf("packet is missing its required %q input", allowed)
		}
		delete(wire, allowed)
	}
	if len(wire) != 0 {
		t.Errorf("packet carries fields outside its authority-and-evidence allow-list: %v", wire)
	}
	if strings.Contains(string(p.Bytes()), "ratified") {
		t.Error("packet serialized caller-side standing flags rather than the ratified content version")
	}
}

func TestPacketFreezesItsByteSequence(t *testing.T) {
	in := packetInput()
	p, err := AssemblePacket(in)
	if err != nil {
		t.Fatal(err)
	}
	before := string(p.Bytes())
	in.Promise.Statement = "different"
	in.Observations["tests"].Data["ok"] = true
	in.Observations["second"] = ledger.Observation{}
	in.Policies[0].Statement = "different policy"
	if after := string(p.Bytes()); after != before {
		t.Fatalf("packet changed from %q to %q after its assembly input changed", before, after)
	}

	bytes := p.Bytes()
	bytes[0] = '!'
	if string(p.Bytes()) != before {
		t.Fatal("Packet.Bytes exposed mutable packet storage")
	}
}

func TestPacketAssemblyRefusesEveryMissingAuthorityGuard(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*PacketInput)
		want string
	}{
		{"unratified promise", func(in *PacketInput) { in.Promise.Ratified = false }, "not ratified"},
		{"unnamed promise", func(in *PacketInput) { in.Promise.ID = "" }, "names no artifact"},
		{"non-promise authority", func(in *PacketInput) { in.Promise.ID = "desire:ship-it" }, "not a promise"},
		{"promise without hash", func(in *PacketInput) { in.Promise.Hash = "" }, "content version"},
		{"promise without repair scope", func(in *PacketInput) { in.Promise.RepairScope = nil }, "no repair scope"},
		{"missing why", func(in *PacketInput) { in.Why = "" }, "no why"},
		{"no instrument", func(in *PacketInput) { in.Promise.Gather = nil }, "names no instrument"},
		{"no measurement at all", func(in *PacketInput) { in.Observations = nil }, "carries no recorded measurement"},
		{"unrecorded observation", func(in *PacketInput) {
			obs := in.Observations["tests"]
			obs.World.Tree = ""
			in.Observations["tests"] = obs
		}, "not a recorded measurement"},
		{"unnamed prior attempt", func(in *PacketInput) { in.PriorAttempts[0].Thread = "" }, "prior attempt"},
		{"unratified policy", func(in *PacketInput) { in.Policies[0].Ratified = false }, "not ratified"},
		{"wrong act policy", func(in *PacketInput) { in.Policies[0].AppliesTo = "land" }, "not dispatch"},
		{"non-policy authority", func(in *PacketInput) { in.Policies[0].ID = "desire:rank-it" }, "not a policy"},
		{"unknown policy reaction", func(in *PacketInput) { in.Policies[0].OnFail = "ignore" }, "unknown on_fail"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := packetInput()
			tc.edit(&in)
			_, err := AssemblePacket(in)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("AssemblePacket error = %v, want refusal mentioning %q", err, tc.want)
			}
		})
	}
}

// twoInstrumentPacket is a promise measured two ways, which is how a rule with
// two dimensions gets written: `graph` answers the edges, `names` answers the
// identifiers, and `hold_when` reads both.
func twoInstrumentPacket() PacketInput {
	in := packetInput()
	in.Promise.ID = "promise:the-core-carries-no-domain-vocabulary"
	in.Promise.Statement = "the core reaches no gatherer and names none"
	in.Promise.Gather = map[string]model.GatherSpec{
		"graph": {Gatherer: "go/imports", Args: map[string]any{"scope": "internal/**/*.go"}},
		"names": {Gatherer: "go/symbols", Args: map[string]any{"scope": "internal/**/*.go"}},
	}
	in.Promise.HoldWhen = "obs.graph.ok and obs.names.ok"
	in.Observations = map[string]ledger.Observation{
		"graph": {
			V: ledger.ObsVersion, At: time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC),
			Gatherer: "go/imports@1", World: model.World{Tree: "sha256:world"},
			Scope: "sha256:scope", Data: map[string]any{"ok": true},
		},
		"names": {
			V: ledger.ObsVersion, At: time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC),
			Gatherer: "go/symbols@1", World: model.World{Tree: "sha256:world"},
			Scope: "sha256:scope", Data: map[string]any{"ok": false},
		},
	}
	return in
}

// TestEveryInstrumentsMeasurementIsInThePacketUnderItsOwnKey is the guard that
// fails when evidence is dropped between the box and the packet.
//
// The key is the promise's own — `graph` and `names` are what `gather` declares
// and what `hold_when` reads as `obs.graph` and `obs.names` — so an agent
// matches a measurement to the clause that weighs it by name rather than by
// guessing at a position in a list.
func TestEveryInstrumentsMeasurementIsInThePacketUnderItsOwnKey(t *testing.T) {
	p, err := AssemblePacket(twoInstrumentPacket())
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Promise struct {
			Gather map[string]json.RawMessage `json:"gather"`
		} `json:"promise"`
		Observations map[string]ledger.Observation `json:"observations"`
	}
	if err := json.Unmarshal(p.Bytes(), &wire); err != nil {
		t.Fatal(err)
	}
	if len(wire.Observations) != len(wire.Promise.Gather) {
		t.Fatalf("the promise gathers %d instruments and the packet carries %d measurements",
			len(wire.Promise.Gather), len(wire.Observations))
	}
	for name := range wire.Promise.Gather {
		observation, carried := wire.Observations[name]
		if !carried {
			t.Fatalf("the packet carries no measurement under %q, which the promise gathers", name)
		}
		if observation.Gatherer == "" || observation.World.Tree == "" {
			t.Fatalf("the measurement under %q arrived hollow: %#v", name, observation)
		}
	}
	// The instruments are different, so the measurements must be too: a packet
	// that carried one measurement twice would satisfy every count above.
	if wire.Observations["graph"].Gatherer == wire.Observations["names"].Gatherer {
		t.Fatalf("both keys carry the same instrument's measurement: %s", p.Bytes())
	}
}

// TestAPacketWhoseMeasurementsDoNotMatchItsInstrumentsIsRefused is the same
// guard from the failing side, in both directions.
//
// A gathered name with no measurement is evidence dropped on the way, and an
// agent reading `hold_when` would find a clause with nothing to weigh. A
// measurement the promise does not gather is data in the packet that nothing in
// the packet accounts for.
func TestAPacketWhoseMeasurementsDoNotMatchItsInstrumentsIsRefused(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*PacketInput)
		want string
	}{
		{"a dropped measurement", func(in *PacketInput) {
			delete(in.Observations, "names")
		}, `gathers "names" and the packet carries no measurement for it`},
		{"the other dropped measurement", func(in *PacketInput) {
			delete(in.Observations, "graph")
		}, `gathers "graph" and the packet carries no measurement for it`},
		{"a hollow measurement", func(in *PacketInput) {
			in.Observations["names"] = ledger.Observation{V: ledger.ObsVersion}
		}, `measurement "names" is not a recorded measurement`},
		{"a measurement nothing gathers", func(in *PacketInput) {
			in.Observations["spare"] = in.Observations["graph"]
		}, `measurement named "spare", which the promise does not gather`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := twoInstrumentPacket()
			tc.edit(&in)
			_, err := AssemblePacket(in)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("AssemblePacket error = %v, want refusal mentioning %q", err, tc.want)
			}
		})
	}
}

// TestAPacketAssembledTwiceFromOneSetOfMeasurementsIsTheSameBytes is what a map
// costs and what it does not. Map iteration is not an order, and a packet is
// content-addressed before it is stored, so two assemblies of one set that
// produced different bytes would file two artifacts for one packet.
func TestAPacketAssembledTwiceFromOneSetOfMeasurementsIsTheSameBytes(t *testing.T) {
	first, err := AssemblePacket(twoInstrumentPacket())
	if err != nil {
		t.Fatal(err)
	}
	for range 32 {
		again, err := AssemblePacket(twoInstrumentPacket())
		if err != nil {
			t.Fatal(err)
		}
		if string(again.Bytes()) != string(first.Bytes()) {
			t.Fatalf("two assemblies of one set of measurements differ:\n%s\n%s", first.Bytes(), again.Bytes())
		}
	}
}
