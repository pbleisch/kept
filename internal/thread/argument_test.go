package thread

import (
	"encoding/json"
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
	// The before side is not repeated here, because it is byte for byte what the
	// packet above carries — which is what a first attempt's before side always
	// is, the packet having been assembled from it.
	// TestABriefDoesNotRepeatWhatThePacketAlreadyCarries is where that is the
	// subject rather than an aside.
	if len(contents.Before) != 0 || !contents.BeforeInPacket {
		t.Fatalf("the brief repeated the packet's own measurement: before=%v, before_in_packet=%v", contents.Before, contents.BeforeInPacket)
	}
	if len(contents.Packet.Observations) != 1 {
		t.Fatalf("the packet no longer holds the measurement the brief points at: %v", contents.Packet.Observations)
	}
	if len(contents.After) != 1 {
		t.Fatalf("the brief lost the after-side measurement: %v", contents.After)
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

// TestABriefDoesNotRepeatWhatThePacketAlreadyCarries is the one rule the brief's
// own size is bounded by, and it is here because a brief that fits is not the
// same thing as a brief a session can work in.
//
// Measured on one real pass: the before-side measurement was byte for byte the
// packet's own observations, 59,499 of 289,890 bytes — 20.5% — and the whole
// message took 162,567 of a 200,000-token window on the first request. The
// runtime compacted at 184,192 tokens and the session argued from evidence it
// had stopped being able to reach. A first attempt's before side is *always* the
// packet's observations, because the packet was assembled from them, so this is
// the ordinary case rather than a coincidence.
//
// What it must not do is lose anything. So the flag is emitted either way and
// the packet is untouched, and the other half of the test is the case where the
// two genuinely differ — a rework's before side is its own box's reading, and
// that one is carried.
func TestABriefDoesNotRepeatWhatThePacketAlreadyCarries(t *testing.T) {
	// A real rework packet, so both duplications are the ones a rework actually
	// produces rather than two fields set to match.
	packet, err := AssemblePacket(reworkInput())
	if err != nil {
		t.Fatal(err)
	}
	rework, err := ReadPacket(packet.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	in := briefInput()
	in.Packet = rework
	in.Before = rework.Observations
	in.Earlier = rework.Rework.Earlier

	brief, err := AssembleBrief(in)
	if err != nil {
		t.Fatal(err)
	}
	contents, err := ReadBrief(brief.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(contents.Before) != 0 || !contents.BeforeInPacket {
		t.Errorf("the before side is repeated: before=%v, before_in_packet=%v", contents.Before, contents.BeforeInPacket)
	}
	if len(contents.Earlier) != 0 || !contents.EarlierInPacket {
		t.Errorf("the earlier attempts are repeated: %d carried, earlier_in_packet=%v", len(contents.Earlier), contents.EarlierInPacket)
	}
	// Nothing was lost: both are in the packet, which is the frozen record and
	// is never trimmed.
	if len(contents.Packet.Observations) != 1 {
		t.Errorf("the packet no longer holds the measurement: %v", contents.Packet.Observations)
	}
	if contents.Packet.Rework == nil || len(contents.Packet.Rework.Earlier) != 1 {
		t.Errorf("the packet no longer holds the earlier attempts: %#v", contents.Packet.Rework)
	}
	// And it is smaller by the duplication rather than by something else. The
	// number is logged because it is the whole reason the rule exists, and
	// because it is necessary and not sufficient: the remainder of a brief this
	// size is still the diff and the after side.
	repeated := contents
	repeated.Before, repeated.BeforeInPacket = rework.Observations, false
	repeated.Earlier, repeated.EarlierInPacket = rework.Rework.Earlier, false
	whole, err := json.Marshal(repeated)
	if err != nil {
		t.Fatal(err)
	}
	if len(brief.Bytes()) >= len(whole) {
		t.Fatalf("the deduplicated brief is %d bytes against %d", len(brief.Bytes()), len(whole))
	}
	t.Logf("this fixture's brief is %d bytes rather than %d, %.1f%% smaller",
		len(brief.Bytes()), len(whole),
		100*float64(len(whole)-len(brief.Bytes()))/float64(len(whole)))

	// The other direction. A rework's own box measures the tree the last attempt
	// left, so its before side is not the packet's, and a brief that dropped it
	// on a resemblance would take the pass's second measurement away.
	differs := briefInput()
	differs.Before = map[string]ledger.Observation{"tests": {Gatherer: "go/tests", Data: map[string]any{"ok": true}}}
	brief, err = AssembleBrief(differs)
	if err != nil {
		t.Fatal(err)
	}
	contents, err = ReadBrief(brief.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(contents.Before) != 1 || contents.BeforeInPacket {
		t.Fatalf("a before side that is not the packet's was dropped: before=%v, before_in_packet=%v", contents.Before, contents.BeforeInPacket)
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

// TestTheAssessorsAskSaysWhatTheBoxIs is the other half of that ask, and it
// exists because of a measured session rather than a preference.
//
// One real pass was handed a brief that took 162,567 of a 200,000-token window,
// read the tree twice on its own initiative, was auto-compacted by the runtime,
// and then argued that it could not read files it had every grant to read: the
// compaction prompt's own "Do NOT call any tools" had survived into the summary
// as a standing instruction, and the brief that would have contradicted it was
// the thing the summary replaced. The ask is re-sent with every request, so it
// is the one channel a compacted session still has, and what it must carry is
// what is true of the box.
//
// It is asserted phrase by phrase rather than by length, because the failure is
// a specific fact going missing. And none of it may arrive as a question: the
// test above bounds those, and the two run together.
func TestTheAssessorsAskSaysWhatTheBoxIs(t *testing.T) {
	ask, err := claudeAskText(ClaudeAssessAsk)
	if err != nil {
		t.Fatal(err)
	}
	for what, phrase := range map[string]string{
		"the copy is the session's own working directory": "this session's working directory",
		"the tree is the one the repair left":             "holds the tree the repair left",
		"every file in it is readable":                    "Every file in it is readable",
		"nothing in it can be written":                    "nothing in it can be written",
		"git history is not in the box":                   "Git history is not there",
		"a git read there answers about no repository":    "report about no repository",
		"a claim the brief lacks is settled by reading":   "settled by opening the file and reading it",
		"the tools last the whole session":                "available for the whole of this session",
	} {
		if !strings.Contains(ask, phrase) {
			t.Errorf("the ask no longer says %s: %q is not in it.\n"+
				"A pass whose brief was summarized away keeps only this channel, and what it"+
				" stops knowing is what the box is.", what, phrase)
		}
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
