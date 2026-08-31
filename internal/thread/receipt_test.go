package thread

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/run"
)

func testReceipt(t *testing.T) (*Receipt, ledger.Store) {
	t.Helper()
	store := ledger.NewFiles(t.TempDir())
	receipt, err := NewReceipt(ledger.Open(store, run.Start("test", nil)), MintID())
	if err != nil {
		t.Fatal(err)
	}
	return receipt, store
}

func TestAThreadReceiptRetainsNativeEvidenceAndOnlyItsReferences(t *testing.T) {
	receipt, store := testReceipt(t)
	when := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	receipt.now = func() time.Time { return when }

	packet, err := AssemblePacket(packetInput())
	if err != nil {
		t.Fatal(err)
	}
	packetRef, err := packet.Store(receipt.ledger)
	if err != nil {
		t.Fatal(err)
	}
	if err := receipt.RecordPacket(packetRef); err != nil {
		t.Fatal(err)
	}

	adapter, err := receipt.ForAdapter(Claude, "fixture:1")
	if err != nil {
		t.Fatal(err)
	}
	nativeRef, err := adapter.Native("content_block_delta", json.RawMessage(`{"text":"hello"}`), Redaction{Occurred: true, Matches: 1})
	if err != nil {
		t.Fatal(err)
	}
	transcript, err := adapter.Transcript(nativeRef, []byte("hello [REDACTED]"), Redaction{Occurred: true, Matches: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Cost(nativeRef, "USD", "0.031"); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Turn(nativeRef, 2); err != nil {
		t.Fatal(err)
	}
	if err := receipt.Started(); err != nil {
		t.Fatal(err)
	}
	code := 0
	if err := receipt.Exited(Exit{Code: &code}); err != nil {
		t.Fatal(err)
	}

	entries, quarantined, err := ReadReceipt(store, receipt.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(quarantined) != 0 {
		t.Fatalf("receipt quarantined its own entries: %#v", quarantined)
	}
	if got, want := ReceiptText(entries), "thread_packet,thread_native,thread_transcript,thread_cost,thread_turn,thread_lifecycle,thread_lifecycle"; got != want {
		t.Fatalf("receipt kinds = %q, want %q", got, want)
	}
	if entries[1].Native.Runtime != Claude || entries[1].Native.Type != "content_block_delta" || entries[1].Native.Ref != nativeRef {
		t.Fatalf("native receipt = %#v, want Claude's unflattened native event", entries[1].Native)
	}
	if entries[2].Transcript.NativeRef != nativeRef || entries[2].Transcript.Artifact != transcript {
		t.Fatalf("transcript projection = %#v, want a reference to native %q and %s", entries[2].Transcript, nativeRef, transcript.Address)
	}
	if entries[2].Transcript.Redaction.Matches != 1 || !entries[2].Transcript.Redaction.Occurred {
		t.Error("transcript redaction was not retained beside the durable fragment")
	}
	if _, err := ledger.ReadArtifact(store, transcript); err != nil {
		t.Fatalf("transcript artifact is not durable with its receipt: %v", err)
	}
}

func TestAProjectionCannotManufactureANativeEvent(t *testing.T) {
	receipt, _ := testReceipt(t)
	adapter, err := receipt.ForAdapter(Pi, "fixture:1")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		call func() error
	}{
		{"transcript", func() error { _, err := adapter.Transcript("made-up", []byte("text"), Redaction{}); return err }},
		{"cost", func() error { return adapter.Cost("made-up", "USD", "1") }},
		{"turn", func() error { return adapter.Turn("made-up", 1) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); err == nil || !strings.Contains(err.Error(), "did not receive") {
				t.Fatalf("projection error = %v, want refusal to manufacture native evidence", err)
			}
		})
	}
}

func TestReceiptRefusesMalformedNativeProjectionAndLifecycleGuards(t *testing.T) {
	receipt, _ := testReceipt(t)
	for _, tc := range []struct {
		name  string
		kind  ledger.Kind
		entry Entry
		want  string
	}{
		{"two bodies", ledger.ThreadCost, Entry{Thread: receipt.ID(), Cost: &CostUpdate{NativeRef: "n", Currency: "USD", Amount: "1"}, Turn: &TurnUpdate{NativeRef: "n", Count: 1}}, "more than one"},
		{"native without version", ledger.ThreadNative, Entry{Thread: receipt.ID(), Native: &NativeEvent{Ref: "n", Runtime: Claude, Type: "x", Payload: json.RawMessage(`{}`)}}, "adapter version"},
		{"native non-json", ledger.ThreadNative, Entry{Thread: receipt.ID(), Native: &NativeEvent{Ref: "n", Runtime: Claude, AdapterVersion: "1", Type: "x", Payload: json.RawMessage(`not-json`)}}, "not JSON"},
		{"bad redaction", ledger.ThreadNative, Entry{Thread: receipt.ID(), Native: &NativeEvent{Ref: "n", Runtime: Claude, AdapterVersion: "1", Type: "x", Payload: json.RawMessage(`{}`), Redaction: Redaction{Matches: 1}}}, "without saying it occurred"},
		{"turn zero", ledger.ThreadTurn, Entry{Thread: receipt.ID(), Turn: &TurnUpdate{NativeRef: "n"}}, "turn projection"},
		{"timeout without bound", ledger.ThreadLifecycle, Entry{Thread: receipt.ID(), Lifecycle: &Lifecycle{Event: TimedOut}}, "timeout lifecycle"},
		{"exit both code and signal", ledger.ThreadLifecycle, Entry{Thread: receipt.ID(), Lifecycle: &Lifecycle{Event: Exited, Exit: &Exit{Code: ptr(1), Signal: "SIGTERM"}}}, "both code and signal"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := receipt.append(tc.kind, tc.entry); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("append error = %v, want refusal mentioning %q", err, tc.want)
			}
		})
	}
}

func TestReadReceiptQuarantinesBrokenAndUnreferencedProjections(t *testing.T) {
	receipt, store := testReceipt(t)
	stream, err := ledger.ThreadPath(string(receipt.ID()))
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	entry := Entry{
		Envelope: ledger.Envelope{V: ledger.Version, Kind: ledger.ThreadCost, At: at, Writer: run.ID("writer"), Seq: 1},
		Thread:   receipt.ID(), Cost: &CostUpdate{NativeRef: "missing", Currency: "USD", Amount: "1"},
	}
	line, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Append(stream, line); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(stream, []byte(`{"v":1`)); err != nil {
		t.Fatal(err)
	}

	entries, quarantined, err := ReadReceipt(store, receipt.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("unreferenced projection read as receipt evidence: %#v", entries)
	}
	if len(quarantined) != 2 || !strings.Contains(quarantined[0].Why+quarantined[1].Why, "not retained earlier") || !strings.Contains(quarantined[0].Why+quarantined[1].Why, "not a receipt") {
		t.Fatalf("broken receipt lines were not quarantined with both causes: %#v", quarantined)
	}
}

func TestAThreadReceiptUsesAMintedPathAndLedgerSequence(t *testing.T) {
	receipt, store := testReceipt(t)
	if _, err := ledger.ThreadPath("../other"); err == nil {
		t.Fatal("a thread id escaped its receipt directory")
	}
	if err := receipt.Started(); err != nil {
		t.Fatal(err)
	}
	if err := receipt.Killed(); err != nil {
		t.Fatal(err)
	}
	entries, _, err := ReadReceipt(store, receipt.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Seq >= entries[1].Seq || entries[0].Writer == "" {
		t.Fatalf("receipt envelopes do not carry the ledger writer sequence: %#v", entries)
	}
}

func ptr(n int) *int { return &n }

func TestReceiptRejectsAChangedTranscriptArtifact(t *testing.T) {
	receipt, store := testReceipt(t)
	adapter, err := receipt.ForAdapter(Claude, "fixture:1")
	if err != nil {
		t.Fatal(err)
	}
	native, err := adapter.Native("delta", json.RawMessage(`{}`), Redaction{})
	if err != nil {
		t.Fatal(err)
	}
	ref, err := adapter.Transcript(native, []byte("first"), Redaction{})
	if err != nil {
		t.Fatal(err)
	}
	where, err := ledger.ArtifactPath(ref.Kind, ref.Address)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(where, []byte("changed")); err != nil {
		t.Fatal(err)
	}
	var corrupt *ledger.CorruptArtifact
	if _, err := ledger.ReadArtifact(store, ref); !errors.As(err, &corrupt) {
		t.Fatalf("changed transcript read as %v", err)
	}
}

// TestAWindowProjectionOfNothingIsRefusedAtTheWrite is the write-side half of
// the rule the Claude adapter is held to: *no window reported* is said by
// holding no window entry, so a window entry that holds no window is a record
// that cannot be told from a window at zero once it is durable.
//
// The order and the duplicate rules are here for a smaller reason and a real
// one. The native reading is a JSON object, so an order taken from it is map
// iteration order, and one window read twice in one event is two answers to one
// question.
func TestAWindowProjectionOfNothingIsRefusedAtTheWrite(t *testing.T) {
	receipt, _ := testReceipt(t)
	adapter, err := receipt.ForAdapter(Claude, "fixture:1")
	if err != nil {
		t.Fatal(err)
	}
	ref, err := adapter.Native("rate_limit_event", json.RawMessage(`{}`), Redaction{})
	if err != nil {
		t.Fatal(err)
	}

	for name, windows := range map[string][]Window{
		"no window at all":                   {},
		"a window with no name":              {{Name: "", Utilization: "0.25"}},
		"a utilization that is not a number": {{Name: "five_hour", Utilization: "most"}},
		"a negative utilization":             {{Name: "five_hour", Utilization: "-0.25"}},
		"one window twice":                   {{Name: "five_hour", Utilization: "0.25"}, {Name: "five_hour", Utilization: "0.26"}},
		"windows out of order":               {{Name: "seven_day", Utilization: "0.11"}, {Name: "five_hour", Utilization: "0.25"}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := adapter.Window(ref, windows); err == nil {
				t.Fatalf("recorded %#v", windows)
			}
		})
	}
	// The reading that is a reading, so the refusals above are not passing
	// against a method that refuses everything.
	if err := adapter.Window(ref, []Window{
		{Name: "five_hour", Utilization: "0.25"},
		{Name: "seven_day", Utilization: "0.11"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Model(ref, "claude-sonnet-5"); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Model(ref, ""); err == nil {
		t.Fatal("recorded a model projection naming no model")
	}
	// And neither may name an event the receipt never received, which is the
	// rule every other projection here is held to.
	if err := adapter.Window("not-an-event", []Window{{Name: "five_hour", Utilization: "0.25"}}); err == nil {
		t.Fatal("recorded a window projection against an event this receipt does not hold")
	}
	if err := adapter.Model("not-an-event", "claude-sonnet-5"); err == nil {
		t.Fatal("recorded a model projection against an event this receipt does not hold")
	}
}
