package thread

import (
	"bytes"
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/ledger"
)

// undecodableCase is one adapter's shape for the tests below: a line it reads
// as an event and projects nothing from, lines it cannot read as one, and the
// credential its redaction removes.
type undecodableCase struct {
	name      string
	prepare   func(t *testing.T) (Invocation, *Receipt, ledger.Store)
	secret    string
	event     string
	malformed []struct{ name, line, why string }
	// torn is the reason a line cut in half is given.
	torn string
}

func undecodableCases() []undecodableCase {
	return []undecodableCase{
		{
			name:    "claude",
			prepare: prepareClaudeFixture,
			secret:  "test-claude-setup-token",
			event:   `{"type":"system","subtype":"status","status":"requesting"}`,
			malformed: []struct{ name, line, why string }{
				{"missing type", `{"message":{}}`, "has no type"},
				{"invalid JSON", `{"type":`, "decode Claude native stream event"},
			},
			torn: "decode Claude native stream event: unexpected end of JSON input",
		},
		{
			name:    "pi",
			prepare: preparePiFixture,
			secret:  "test-pi-key",
			event:   `{"type":"agent_start"}`,
			malformed: []struct{ name, line, why string }{
				{"missing type", `{"message":{}}`, "has no type"},
				{"invalid JSON", `{"type":`, "decode Pi RPC native event"},
			},
			torn: "decode Pi RPC native event: unexpected end of JSON input",
		},
		{
			name:    "codex",
			prepare: prepareCodexFixture,
			secret:  "test-codex-access-token",
			event:   `{"method":"item/started","params":{"threadId":"thread-fixture"}}`,
			malformed: []struct{ name, line, why string }{
				{"invalid JSON", `{"method":`, "decode Codex app-server JSON-RPC"},
				{"JSON-RPC header", `{"jsonrpc":"2.0","method":"turn/started","params":{}}`, "must omit the jsonrpc header"},
				{"batch", `[]`, "decode Codex app-server JSON-RPC"},
				{"notification without params", `{"method":"turn/started"}`, "has no object params"},
				{"ambiguous response", `{"id":"id","result":{},"error":{}}`, "neither one notification nor one response"},
			},
			torn: "decode Codex app-server JSON-RPC: unexpected end of JSON input",
		},
	}
}

// closeIgnoringEvidence closes an adapter's stream. Codex refuses at close when
// its pre-exit evidence never arrived, which is a separate check with its own
// test; what these tests hold is that nothing about a line that did not decode
// is a failure.
func closeIgnoringEvidence(t *testing.T, invocation Invocation) {
	t.Helper()
	if err := invocation.Output.Close(); err != nil && !strings.Contains(err.Error(), "pre-exit") {
		t.Fatalf("closing a stream that held an undecodable item failed: %v", err)
	}
}

// TestAnUndecodableLineIsRetainedAndTheStreamGoesOn is a malformed line in the
// middle of a session. It was refused, and the refusal went nowhere: the launch
// stopped reading at the first error its stream returned and nothing reported
// it, so every event after the line was lost and the dispatch read as clean.
// Now the line is kept, under the reason it did not decode, and the next event
// is retained as though nothing had happened — because nothing had, to the
// session.
func TestAnUndecodableLineIsRetainedAndTheStreamGoesOn(t *testing.T) {
	for _, adapter := range undecodableCases() {
		for _, malformed := range adapter.malformed {
			t.Run(adapter.name+"/"+malformed.name, func(t *testing.T) {
				invocation, receipt, store := adapter.prepare(t)
				if _, err := invocation.Output.Write([]byte(malformed.line + "\n" + adapter.event + "\n")); err != nil {
					t.Fatalf("an undecodable line failed the stream: %v", err)
				}
				closeIgnoringEvidence(t, invocation)

				entries, quarantined, err := ReadReceipt(store, receipt.ID())
				if err != nil {
					t.Fatal(err)
				}
				if len(quarantined) != 0 {
					t.Fatalf("the receipt quarantined its own evidence: %#v", quarantined)
				}
				if got := ReceiptText(entries); got != "thread_undecodable,thread_native" {
					t.Fatalf("receipt = %q, want the line kept as undecodable and the event after it retained", got)
				}
				item := entries[0].Undecodable
				if item.Unterminated || !strings.Contains(item.Why, malformed.why) {
					t.Fatalf("undecodable item = %#v, want a terminated line under the reason %q", item, malformed.why)
				}
				if item.Runtime != Runtime(adapter.name) || item.AdapterVersion == "" {
					t.Fatalf("undecodable item does not say what wrote it: %#v", item)
				}
				kept, err := ledger.ReadArtifact(store, item.Artifact)
				if err != nil {
					t.Fatal(err)
				}
				if string(kept) != malformed.line {
					t.Fatalf("retained bytes = %q, want exactly the line the runtime wrote", kept)
				}
				// And nothing was invented from it: the one native event is the
				// line that was one.
				if !bytes.Equal(entries[1].Native.Payload, []byte(adapter.event)) {
					t.Fatalf("native event = %s, want the decodable line", entries[1].Native.Payload)
				}
				if items, unterminated := receipt.Undecoded(); items != 1 || unterminated {
					t.Fatalf("receipt reports %d undecodable items, unterminated %v", items, unterminated)
				}
			})
		}
	}
}

// TestAStreamThatEndsInsideAnItemRetainsItAsUnterminated is thread
// 1y6kx8c48hb6mtjkpda76brtz0's final line. The runtime exited 0 with an event
// half written, the stream's close tried to decode the half, and the error it
// returned aborted the repair and discarded the box the session had worked in
// for nineteen minutes.
func TestAStreamThatEndsInsideAnItemRetainsItAsUnterminated(t *testing.T) {
	for _, adapter := range undecodableCases() {
		t.Run(adapter.name, func(t *testing.T) {
			invocation, receipt, store := adapter.prepare(t)
			torn := adapter.event[:len(adapter.event)/2]
			if _, err := invocation.Output.Write([]byte(adapter.event + "\n" + torn)); err != nil {
				t.Fatal(err)
			}
			closeIgnoringEvidence(t, invocation)

			entries, quarantined, err := ReadReceipt(store, receipt.ID())
			if err != nil {
				t.Fatal(err)
			}
			if len(quarantined) != 0 || ReceiptText(entries) != "thread_native,thread_undecodable" {
				t.Fatalf("receipt = %q, quarantine %#v", ReceiptText(entries), quarantined)
			}
			item := entries[1].Undecodable
			if !item.Unterminated || item.Why != adapter.torn {
				t.Fatalf("torn item = %#v, want unterminated under %q", item, adapter.torn)
			}
			kept, err := ledger.ReadArtifact(store, item.Artifact)
			if err != nil {
				t.Fatal(err)
			}
			if string(kept) != torn {
				t.Fatalf("retained bytes = %q, want the half the runtime wrote", kept)
			}
			if items, unterminated := receipt.Undecoded(); items != 1 || !unterminated {
				t.Fatalf("receipt reports %d undecodable items, unterminated %v", items, unterminated)
			}
		})
	}
}

// TestAFinalItemThatDecodesIsAnEventWhateverItsLineEnd keeps the other half of
// what Close is for: a runtime that finished its last event and did not write a
// newline after it wrote an event, not a torn one.
func TestAFinalItemThatDecodesIsAnEventWhateverItsLineEnd(t *testing.T) {
	for _, adapter := range undecodableCases() {
		t.Run(adapter.name, func(t *testing.T) {
			invocation, receipt, store := adapter.prepare(t)
			if _, err := invocation.Output.Write([]byte(adapter.event)); err != nil {
				t.Fatal(err)
			}
			closeIgnoringEvidence(t, invocation)
			entries, _, err := ReadReceipt(store, receipt.ID())
			if err != nil {
				t.Fatal(err)
			}
			if got := ReceiptText(entries); got != "thread_native" {
				t.Fatalf("receipt = %q, want one native event", got)
			}
			if items, _ := receipt.Undecoded(); items != 0 {
				t.Fatalf("a decodable final item was counted undecodable")
			}
		})
	}
}

// TestUndecodableOutputIsRedactedBeforeItIsKept holds a torn item to the rule
// every durable byte of a session is held to. A runtime that echoed its own
// credential and was cut off mid-line has not written an event, and it has
// still written the credential.
func TestUndecodableOutputIsRedactedBeforeItIsKept(t *testing.T) {
	for _, adapter := range undecodableCases() {
		t.Run(adapter.name, func(t *testing.T) {
			invocation, receipt, store := adapter.prepare(t)
			torn := `{"type":"assistant","message":{"content":[{"type":"text","text":"` + adapter.secret
			if _, err := invocation.Output.Write([]byte(torn)); err != nil {
				t.Fatal(err)
			}
			closeIgnoringEvidence(t, invocation)
			entries, _, err := ReadReceipt(store, receipt.ID())
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 || entries[0].Undecodable == nil {
				t.Fatalf("receipt = %q, want one undecodable item", ReceiptText(entries))
			}
			item := entries[0].Undecodable
			if !item.Redaction.Occurred || item.Redaction.Matches != 1 {
				t.Fatalf("redaction = %#v, want the credential replaced once", item.Redaction)
			}
			kept, err := ledger.ReadArtifact(store, item.Artifact)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(kept, []byte(adapter.secret)) || !bytes.Contains(kept, []byte("[REDACTED]")) {
				t.Fatalf("the credential reached durable evidence: %q", kept)
			}
		})
	}
}

// TestAnUndecodableItemNamesWhyAndWhatWroteIt is the guard on the record
// itself: an item that did not say why it did not decode is a gap in a receipt
// with nothing to read, and it is refused at the write.
func TestAnUndecodableItemNamesWhyAndWhatWroteIt(t *testing.T) {
	receipt, _ := testReceipt(t)
	adapter, err := receipt.ForAdapter(Claude, claudeAdapterVersion)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Undecodable([]byte("{"), " ", true, Redaction{}); err == nil {
		t.Fatal("an undecodable item with no reason was written")
	}
	if err := adapter.Undecodable([]byte("{"), "torn", true, Redaction{Occurred: true}); err == nil {
		t.Fatal("an undecodable item with an incoherent redaction was written")
	}
	if items, _ := receipt.Undecoded(); items != 0 {
		t.Fatalf("a refused item was counted: %d", items)
	}
}
