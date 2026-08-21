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
		Observation: ledger.Observation{
			V:        ledger.ObsVersion,
			At:       time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC),
			Gatherer: "go/tests@1",
			World:    model.World{Tree: "sha256:world"},
			Scope:    "sha256:scope",
			Data:     map[string]any{"ok": false},
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
	for _, allowed := range []string{"v", "promise", "why", "observation", "prior_attempts", "policies"} {
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
	in.Observation.Data["ok"] = true
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
		{"unrecorded observation", func(in *PacketInput) { in.Observation.World.Tree = "" }, "not a recorded measurement"},
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
