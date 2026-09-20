package thread

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/model"
)

// reworkInput is a packet for a second attempt: the first anchored a change,
// the checklist was clear, and the pass argued and ruled reject.
func reworkInput() PacketInput {
	in := packetInput()
	in.PriorAttempts = nil
	in.Rework = &Rework{
		Base:    strings.Repeat("b", 40),
		Tip:     strings.Repeat("a", 40),
		Touched: []BriefPath{{Path: "internal/gather/source.go", Status: "modified"}},
		Diff:    "--- a/internal/gather/source.go\n+++ b/internal/gather/source.go\n",
		Earlier: []EarlierAttempt{{
			Anchor:    &Anchor{Commit: strings.Repeat("a", 40), Parent: strings.Repeat("b", 40), Promise: "promise:kept-is-tested"},
			Rederived: model.Kept,
			Derived:   []BriefFinding{{Check: "property-exercised", Outcome: "clear", Detail: "nothing measured less"}},
			Argument:  "The test named for the property never asserts it.\n\nruling: reject\n",
			Ruling:    RulingReject,
		}},
	}
	return in
}

// TestAReworkPacketCarriesTheEarlierAttemptsVerbatim is the gap of a second
// kind on the wire: the argument whole, the ruling beside it, and the packet
// saying of itself that it is a rework — which is what decides its ask. See
// docs/decision/0086.
func TestAReworkPacketCarriesTheEarlierAttemptsVerbatim(t *testing.T) {
	in := reworkInput()
	packet, err := AssemblePacket(in)
	if err != nil {
		t.Fatal(err)
	}
	if !packet.Rework() {
		t.Fatal("a packet carrying earlier attempts does not say it is a rework")
	}
	contents, err := ReadPacket(packet.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if contents.V != 3 || contents.Rework == nil {
		t.Fatalf("read back version %d with rework %v", contents.V, contents.Rework)
	}
	earlier := contents.Rework.Earlier
	if len(earlier) != 1 || earlier[0].Argument != in.Rework.Earlier[0].Argument || earlier[0].Ruling != RulingReject {
		t.Fatalf("the earlier attempt did not survive the round trip verbatim: %#v", earlier)
	}
	// Frozen at assembly: the caller's value moving afterwards moves nothing.
	before := string(packet.Bytes())
	in.Rework.Earlier[0].Argument = "rewritten"
	in.Rework.Earlier[0].Anchor.Commit = strings.Repeat("c", 40)
	if string(packet.Bytes()) != before {
		t.Fatal("the rework packet changed after its assembly input did")
	}

	plain, err := AssemblePacket(packetInput())
	if err != nil {
		t.Fatal(err)
	}
	if plain.Rework() {
		t.Fatal("a first attempt's packet says it is a rework")
	}
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(plain.Bytes(), &wire); err != nil {
		t.Fatal(err)
	}
	if _, found := wire["rework"]; found {
		t.Fatal("a first attempt's packet carries a rework field")
	}
}

// TestAReworkPacketIsRefusedUnlessItCanBeFollowed breaks each thing a rework
// must say, including the one pair of statements that cannot both be true.
func TestAReworkPacketIsRefusedUnlessItCanBeFollowed(t *testing.T) {
	for _, c := range []struct {
		name  string
		edit  func(*PacketInput)
		wants string
	}{
		{"no base", func(in *PacketInput) { in.Rework.Base = "" }, "names no base commit"},
		{"no tip", func(in *PacketInput) { in.Rework.Tip = "HEAD" }, "names no commit its box was cut at"},
		{"no earlier attempt", func(in *PacketInput) { in.Rework.Earlier = nil }, "carries no earlier attempt"},
		{"unmeasured and measured", func(in *PacketInput) { in.Rework.Unmeasured = "go/symbols: syntax error" }, "could not be measured and carries a measurement"},
		{"a ruling from nothing", func(in *PacketInput) { in.Rework.Earlier[0].Argument = "" }, "carries a ruling and no argument"},
		{"a ruling outside the set", func(in *PacketInput) { in.Rework.Earlier[0].Ruling = "maybe" }, "not one a pass can make"},
		{"a re-derivation that is not a value", func(in *PacketInput) { in.Rework.Earlier[0].Rederived = "repaired" }, "not a value a promise reads"},
		{"a broken anchor", func(in *PacketInput) { in.Rework.Earlier[0].Anchor.Parent = "" }, "anchor names no parent"},
		{"a finding with no outcome", func(in *PacketInput) { in.Rework.Earlier[0].Derived[0].Outcome = "" }, "no check or no outcome"},
		{"a first attempt with no measurement", func(in *PacketInput) { in.Rework = nil; in.Observations = nil }, "carries no recorded measurement"},
	} {
		t.Run(c.name, func(t *testing.T) {
			in := reworkInput()
			c.edit(&in)
			if _, err := AssemblePacket(in); err == nil || !strings.Contains(err.Error(), c.wants) {
				t.Fatalf("AssemblePacket error = %v, want it to say %q", err, c.wants)
			}
		})
	}

	// And the one packet that may carry no observations: a rework whose box
	// measured nothing, which says so instead of borrowing a measurement.
	in := reworkInput()
	in.Observations = nil
	in.Rework.Unmeasured = "go/symbols: internal/gather/source.go does not parse"
	if _, err := AssemblePacket(in); err != nil {
		t.Fatalf("a rework whose box measured nothing was refused: %v", err)
	}
}

// TestAReworkCarriesEvidenceAndNoPipeline closes the rework's field set, for the
// reason ArgumentRecord's is closed. The narrowing that would put the pipeline in
// a packet is not somebody deciding to — it is a field for attempts remaining or
// the reason a rework was selected arriving because it looked useful.
// policy:the-pipeline-is-never-in-a-packet.
func TestAReworkCarriesEvidenceAndNoPipeline(t *testing.T) {
	for _, c := range []struct {
		shape reflect.Type
		want  []string
	}{
		{reflect.TypeOf(Rework{}), []string{"Base", "Tip", "Touched", "Diff", "Elided", "Unmeasured", "Earlier"}},
		{reflect.TypeOf(EarlierAttempt{}), []string{"Anchor", "Rederived", "Derived", "Argument", "Ruling"}},
	} {
		var fields []string
		for i := range c.shape.NumField() {
			fields = append(fields, c.shape.Field(i).Name)
		}
		if !slices.Equal(fields, c.want) {
			t.Fatalf("%s carries %v, want exactly %v. A field beyond these is the pipeline or a shape over what a pass said: see docs/decision/0086.", c.shape.Name(), fields, c.want)
		}
	}
}

// TestAReworkPacketTravelsWithTheReworkAskAndNoOther is the pairing, broken both
// ways. The repair ask over a rework is a session told to stop when the promise
// holds, standing where it may already hold; the rework ask over a first attempt
// is a session told to answer criticism nobody made.
func TestAReworkPacketTravelsWithTheReworkAskAndNoOther(t *testing.T) {
	rework, err := AssemblePacket(reworkInput())
	if err != nil {
		t.Fatal(err)
	}
	plain, err := AssemblePacket(packetInput())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name   string
		ask    ClaudeAsk
		packet Packet
		ok     bool
	}{
		{"a rework with the rework ask", ClaudeReworkAsk, rework, true},
		{"a rework with the repair ask", ClaudeRepairAsk, rework, false},
		{"a first attempt with the rework ask", ClaudeReworkAsk, plain, false},
		{"a rework with the assessor's ask", ClaudeAssessAsk, rework, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			cfg := claudeConfig(t)
			cfg.ClaudeAsk = c.ask
			err := checkClaudeConfig(cfg, c.packet)
			if c.ok && err != nil {
				t.Fatalf("refused: %v", err)
			}
			if !c.ok && (err == nil || !strings.Contains(err.Error(), "does not suit its input")) {
				t.Fatalf("error = %v, want the mismatch refused by name", err)
			}
		})
	}

	text, err := claudeAskText(ClaudeReworkAsk)
	if err != nil {
		t.Fatal(err)
	}
	// The ask names nothing from any thread, and nothing from the pipeline.
	for _, borrowed := range []string{"promise:", "internal/", "attempts remain", "budget", "of 3"} {
		if strings.Contains(text, borrowed) {
			t.Errorf("the rework ask carries %q", borrowed)
		}
	}
	if !strings.Contains(text, "An account that agrees with the criticism and leaves the tree unchanged is a failed rework") {
		t.Error("the rework ask no longer names the failure a rework is most likely to produce")
	}
}

// TestAnAdapterWithNoReworkAskRefusesAReworkByName holds Pi and Codex to
// refusing, rather than launching a rework under a repair's framing.
func TestAnAdapterWithNoReworkAskRefusesAReworkByName(t *testing.T) {
	rework, err := AssemblePacket(reworkInput())
	if err != nil {
		t.Fatal(err)
	}
	if err := checkPiConfig(piConfig(t), rework); err == nil || !strings.Contains(err.Error(), "no rework ask") {
		t.Fatalf("pi: %v", err)
	}
	if err := checkCodexConfig(codexConfig(t), rework); err == nil || !strings.Contains(err.Error(), "no rework ask") {
		t.Fatalf("codex: %v", err)
	}
}

// TestABriefCarriesTheEpisodesEarlierAttempts is the pass's view of the series:
// what each earlier attempt left and what the pass over it argued and ruled.
func TestABriefCarriesTheEpisodesEarlierAttempts(t *testing.T) {
	in := briefInput()
	in.Earlier = reworkInput().Rework.Earlier
	brief, err := AssembleBrief(in)
	if err != nil {
		t.Fatal(err)
	}
	contents, err := ReadBrief(brief.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if contents.V != BriefVersion || len(contents.Earlier) != 1 || contents.Earlier[0].Ruling != RulingReject {
		t.Fatalf("the brief lost the earlier attempts: v%d %#v", contents.V, contents.Earlier)
	}
	in.Earlier[0].Ruling = "maybe"
	if _, err := AssembleBrief(in); err == nil || !strings.Contains(err.Error(), "not one a pass can make") {
		t.Fatalf("a brief carrying an unknown ruling was assembled: %v", err)
	}
}
