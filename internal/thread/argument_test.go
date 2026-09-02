package thread

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/ledger"
)

func briefInput() BriefInput {
	packet, err := AssemblePacket(packetInput())
	if err != nil {
		panic(err)
	}
	contents, err := ReadPacket(packet.Bytes())
	if err != nil {
		panic(err)
	}
	return BriefInput{
		Thread: "01hzzz",
		Packet: contents,
		Anchor: Anchor{
			Commit:  strings.Repeat("a", 40),
			Parent:  strings.Repeat("b", 40),
			Promise: "promise:kept-is-tested",
		},
		Touched:    []BriefPath{{Path: "internal/gather/source.go", Status: "modified"}},
		Diff:       "--- a/internal/gather/source.go\n+++ b/internal/gather/source.go\n",
		Before:     packetInput().Observations,
		After:      packetInput().Observations,
		Transcript: "I removed the call.",
		Derived:    []BriefFinding{{Check: "already-satisfied", Outcome: "clear", Detail: "the predicate did not hold before"}},
	}
}

// TestABriefCarriesTheWholeEntryAndSurvivesTheRoundTrip is the assessor's input
// channel, and what it asserts is that nothing is rebuilt on the way back: the
// bytes a pass received are what a later reader gets, the way a packet's are.
func TestABriefCarriesTheWholeEntryAndSurvivesTheRoundTrip(t *testing.T) {
	brief, err := AssembleBrief(briefInput())
	if err != nil {
		t.Fatal(err)
	}
	contents, err := ReadBrief(brief.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if contents.V != BriefVersion {
		t.Fatalf("brief version = %d", contents.V)
	}
	if contents.Thread != "01hzzz" || contents.Promise != "promise:kept-is-tested" {
		t.Fatalf("a brief that does not name what it is about: %#v", contents)
	}
	// The packet's own frozen authority, not a reconstruction from a corpus.
	// It is what makes an assessment stable across the revision that ordinarily
	// follows a finding.
	if contents.Packet.Promise.HoldWhen != "obs.tests.ok" || contents.Packet.Promise.Hash != "sha256:promise" {
		t.Fatalf("the brief lost the packet's authority: %#v", contents.Packet.Promise)
	}
	if contents.Transcript != "I removed the call." {
		t.Fatalf("the brief lost the repair session's transcript: %q", contents.Transcript)
	}
	if len(contents.Derived) != 1 || contents.Derived[0].Check != "already-satisfied" {
		t.Fatalf("the brief lost the derived findings: %#v", contents.Derived)
	}
	if len(contents.Before) != 1 || len(contents.After) != 1 {
		t.Fatalf("the brief lost a measurement: before=%v after=%v", contents.Before, contents.After)
	}

	// Assembled twice from one entry, the same bytes. A brief is
	// content-addressed and stored before the launch, so map iteration order
	// reaching the wire would mint a second artifact for one piece of evidence.
	again, err := AssembleBrief(briefInput())
	if err != nil {
		t.Fatal(err)
	}
	if string(again.Bytes()) != string(brief.Bytes()) {
		t.Fatal("two briefs assembled from one entry are different bytes")
	}
}

// TestABriefRefusesOnlyWhatCannotBeRecoveredLater is the shape of its guard, and
// the shape is the point: a brief authorizes nothing, so it checks nothing about
// authority, and what it does refuse is the three things a later reader could
// not work out for itself.
func TestABriefRefusesOnlyWhatCannotBeRecoveredLater(t *testing.T) {
	for _, test := range []struct {
		name   string
		break_ func(*BriefInput)
		why    string
	}{
		{"no thread", func(in *BriefInput) { in.Thread = "" }, "names no thread"},
		{"no packet", func(in *BriefInput) { in.Packet = Contents{} }, "carries no packet"},
		{"no anchor", func(in *BriefInput) { in.Anchor = Anchor{} }, "anchor"},
		{"a finding with no outcome", func(in *BriefInput) { in.Derived[0].Outcome = "" }, "no check or no outcome"},
		{"a changed path with no status", func(in *BriefInput) { in.Touched[0].Status = "" }, "no path or no status"},
	} {
		t.Run(test.name, func(t *testing.T) {
			in := briefInput()
			test.break_(&in)
			if _, err := AssembleBrief(in); err == nil {
				t.Fatal("a brief missing what nothing can recover was assembled")
			} else if !strings.Contains(err.Error(), test.why) {
				t.Fatalf("refusal = %v, want it to name %q", err, test.why)
			}
		})
	}

	// And what it does not refuse: every piece of evidence that may legitimately
	// be absent. A session killed at its deadline leaves no re-derivation and no
	// transcript, and a pass reading a brief like that is told so by the field
	// being empty rather than by there being no brief at all.
	in := briefInput()
	in.After, in.Transcript, in.Diff, in.Derived, in.Touched = nil, "", "", nil, nil
	if _, err := AssembleBrief(in); err != nil {
		t.Fatalf("a brief over a killed session was refused: %v", err)
	}
}

// TestTheAssessorsAskIsAFramingRatherThanAQuestionList is the guard on the half
// of docs/decision/0078 that is easiest to undo by accident, and it breaks it on
// purpose.
//
// The pass's value is that it is not enumerated. A question list in the ask is a
// checklist arriving by the back door: the session answers what it was given and
// stops looking, and the failure nobody thought to ask about goes uncaught while
// the report says a pass ran. So the ask may frame the work and may not itemize
// it, and the test that says so is the one that would fail the day somebody adds
// "also check whether…".
func TestTheAssessorsAskIsAFramingRatherThanAQuestionList(t *testing.T) {
	ask, err := claudeAskText(ClaudeAssessAsk)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(ask) == "" {
		t.Fatal("the assessor's ask is empty, so a pass would launch unframed")
	}

	// No enumeration, in any of the three shapes prose reaches for. Each of
	// these would be a question somebody wrote down in advance, which is the
	// ceiling this half exists above.
	for _, line := range strings.Split(ask, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "-"), strings.HasPrefix(trimmed, "*"), strings.HasPrefix(trimmed, "•"):
			t.Errorf("the ask carries a bulleted list, which is a checklist: %q", trimmed)
		case len(trimmed) > 2 && trimmed[0] >= '1' && trimmed[0] <= '9' && (trimmed[1] == '.' || trimmed[1] == ')'):
			t.Errorf("the ask carries a numbered list, which is a checklist: %q", trimmed)
		}
	}
	// One example is a framing; several are a list without the bullets. The
	// count is what separates them and the bound is deliberately tight.
	if questions := strings.Count(ask, "?"); questions > 1 {
		t.Errorf("the ask puts %d questions to the session, and a set of questions is the checklist this half is not", questions)
	}

	// It names nothing from any particular thread, so it is the same sentence
	// for every brief there will ever be — which is what makes it a declaration
	// rather than an instruction about one repair. See docs/decision/0058.
	brief, err := AssembleBrief(briefInput())
	if err != nil {
		t.Fatal(err)
	}
	for _, named := range []string{"promise:kept-is-tested", "internal/gather/source.go", "obs.tests.ok"} {
		if strings.Contains(ask, named) {
			t.Errorf("the ask names %q, so it is about one thread rather than about the work", named)
		}
	}
	if strings.Contains(string(brief.Bytes()), ask) {
		t.Fatal("the brief carries the ask, which is what decision 0009 keeps out of the input channel")
	}

	// And it does not ask for a verdict. The promise's answer is the
	// re-derivation's, and a pass invited to give one would be the input to a
	// verdict that policy:the-agents-report-is-never-an-input-to-the-verdict
	// refuses.
	if !strings.Contains(ask, "Nothing you write decides whether the promise is kept") {
		t.Error("the ask no longer says that the session's account is not the verdict")
	}
}

// TestAnArgumentRecordCarriesProvenanceAndNoShapeForAFinding is the freedom
// itself, held by the one mechanism that can hold it: the record's own fields.
//
// docs/decision/0078 says the pass's findings are unbounded and unslotted, and
// the way that gets undone is not by somebody deciding to undo it. It is by a
// field — a severity, a category, a count, a verdict — arriving because it
// seemed useful, after which the pass is answering the question that field
// implies. So the field set is closed here and named, and adding one fails this
// test rather than quietly narrowing the half it belongs to.
func TestAnArgumentRecordCarriesProvenanceAndNoShapeForAFinding(t *testing.T) {
	var fields []string
	shape := reflect.TypeOf(ArgumentRecord{})
	for i := range shape.NumField() {
		fields = append(fields, shape.Field(i).Name)
	}
	// Every one of these says *who argued, from what evidence, and where the
	// words are*. None of them says anything about what the words contain.
	want := []string{"Runtime", "AdapterVersion", "Brief", "Artifact", "Redaction"}
	if !slices.Equal(fields, want) {
		t.Fatalf("ArgumentRecord carries %v, want exactly %v.\n"+
			"A field beyond these is a shape over what the pass said, and the pass's findings are"+
			" unbounded and unslotted on purpose: see docs/decision/0078.", fields, want)
	}
}

// TestAnArgumentIsRefusedUnlessItSaysWhatItReadAndWhatAnswered is the other half
// of that record: it constrains its provenance tightly and its content not at
// all.
func TestAnArgumentIsRefusedUnlessItSaysWhatItReadAndWhatAnswered(t *testing.T) {
	receipt, store := testReceipt(t)
	brief, err := AssembleBrief(briefInput())
	if err != nil {
		t.Fatal(err)
	}
	briefRef, err := brief.Store(receipt.ledger)
	if err != nil {
		t.Fatal(err)
	}
	if err := receipt.RecordBrief(briefRef); err != nil {
		t.Fatal(err)
	}
	said, err := receipt.ledger.PutArtifact(ledger.ThreadArgumentArtifact, []byte("the repair replaced the instrument."))
	if err != nil {
		t.Fatal(err)
	}
	whole := ArgumentRecord{Runtime: Claude, AdapterVersion: "test@1", Brief: briefRef, Artifact: said}

	for _, test := range []struct {
		name   string
		break_ func(*ArgumentRecord)
		why    string
	}{
		{"no runtime", func(a *ArgumentRecord) { a.Runtime = "" }, "names no runtime"},
		{"an unknown runtime", func(a *ArgumentRecord) { a.Runtime = "somebody-elses" }, "names no runtime"},
		{"no adapter version", func(a *ArgumentRecord) { a.AdapterVersion = "" }, "adapter version"},
		{"no brief", func(a *ArgumentRecord) { a.Brief = ledger.ArtifactRef{} }, "brief it was reached from"},
		{"the brief is a packet", func(a *ArgumentRecord) { a.Brief.Kind = ledger.ThreadPacketArtifact }, "brief it was reached from"},
		{"no words", func(a *ArgumentRecord) { a.Artifact = ledger.ArtifactRef{} }, "argument artifact"},
	} {
		t.Run(test.name, func(t *testing.T) {
			broken := whole
			test.break_(&broken)
			if err := receipt.RecordArgument(broken); err == nil {
				t.Fatal("an argument that does not say what it read or what answered was recorded")
			} else if !strings.Contains(err.Error(), test.why) {
				t.Fatalf("refusal = %v, want it to name %q", err, test.why)
			}
		})
	}

	if err := receipt.RecordArgument(whole); err != nil {
		t.Fatal(err)
	}
	entries, quarantined, err := ReadReceipt(store, receipt.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(quarantined) != 0 {
		t.Fatalf("a well-formed argument was quarantined on the way back in: %#v", quarantined)
	}
	arguments := Arguments(entries)
	if len(arguments) != 1 || arguments[0].Artifact != said {
		t.Fatalf("the argument did not survive the round trip: %#v", arguments)
	}
}

// TestAReceiptGroupsIntoTheSessionsThatProducedIt is docs/decision/0079's rule
// worked as a derivation: a thread is an episode, an attempt is delimited by its
// input record, and *which session wrote this* is answered from the record
// rather than from an ordinal on every body type.
func TestAReceiptGroupsIntoTheSessionsThatProducedIt(t *testing.T) {
	receipt, store := testReceipt(t)
	packet, err := AssemblePacket(packetInput())
	if err != nil {
		t.Fatal(err)
	}
	packetRef, err := packet.Store(receipt.ledger)
	if err != nil {
		t.Fatal(err)
	}
	// A measurement filed before the launch: it belongs to the box rather than
	// to a session, and a grouping that claimed it for the first session would
	// be claiming it for an input that had not been recorded when it was
	// written.
	address, err := receipt.ledger.Observe(ledger.Observation{
		V: ledger.ObsVersion, At: packetInput().Observations["tests"].At,
		Gatherer: "go/tests@1", World: packetInput().Observations["tests"].World,
		Scope: "sha256:scope", Data: map[string]any{"ok": false},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := receipt.RecordMeasurement(Measurement{Side: Before, Name: "tests", Address: address}); err != nil {
		t.Fatal(err)
	}
	if err := receipt.RecordPacket(packetRef); err != nil {
		t.Fatal(err)
	}
	if err := receipt.Started(); err != nil {
		t.Fatal(err)
	}
	brief, err := AssembleBrief(briefInput())
	if err != nil {
		t.Fatal(err)
	}
	briefRef, err := brief.Store(receipt.ledger)
	if err != nil {
		t.Fatal(err)
	}
	if err := receipt.RecordBrief(briefRef); err != nil {
		t.Fatal(err)
	}
	code := 0
	if err := receipt.Exited(Exit{Code: &code}); err != nil {
		t.Fatal(err)
	}

	entries, _, err := ReadReceipt(store, receipt.ID())
	if err != nil {
		t.Fatal(err)
	}
	sessions := Sessions(entries)
	if len(sessions) != 2 {
		t.Fatalf("a receipt holding a repair and a pass grouped into %d sessions", len(sessions))
	}
	if sessions[0].Kind != ledger.ThreadPacket || sessions[0].Input != packetRef {
		t.Fatalf("the first session is %#v, want the repair's packet", sessions[0])
	}
	if sessions[1].Kind != ledger.ThreadBrief || sessions[1].Input != briefRef {
		t.Fatalf("the second session is %#v, want the pass's brief", sessions[1])
	}
	if len(sessions[0].Entries) != 1 || sessions[0].Entries[0].Lifecycle == nil {
		t.Fatalf("the repair session holds %#v, want only the lifecycle written after its packet", sessions[0].Entries)
	}
	if len(sessions[1].Entries) != 1 || sessions[1].Entries[0].Lifecycle == nil {
		t.Fatalf("the pass holds %#v, want only the lifecycle written after its brief", sessions[1].Entries)
	}
}
