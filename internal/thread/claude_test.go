package thread

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/ledger"
)

type claudeFixture struct {
	Runtime               string   `json:"runtime"`
	CLIVersion            string   `json:"cli_version"`
	DirectArgv            []string `json:"direct_argv"`
	CredentialEnvironment string   `json:"credential_environment"`
	ChildEnvironment      string   `json:"child_environment"`
	NativeExamples        string   `json:"native_examples"`
	Admission             string   `json:"admission"`
}

func loadClaudeFixture(t *testing.T) claudeFixture {
	t.Helper()
	where := filepath.Join("testdata", "claude", "fixture.json")
	src, err := os.ReadFile(where)
	if err != nil {
		t.Fatal(err)
	}
	var fixture claudeFixture
	if err := json.Unmarshal(src, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func claudeConfig() RuntimeConfig {
	return RuntimeConfig{
		Runtime:         string(Claude),
		Executable:      "/configured/claude",
		Version:         ClaudeVersion,
		AnthropicAPIKey: "test-anthropic-key",
	}
}

func prepareClaudeFixture(t *testing.T) (Invocation, *Receipt, ledger.Store) {
	t.Helper()
	receipt, store := testReceipt(t)
	adapter, err := receipt.ForAdapter(Claude, claudeAdapterVersion)
	if err != nil {
		t.Fatal(err)
	}
	packet, err := AssemblePacket(packetInput())
	if err != nil {
		t.Fatal(err)
	}
	invocation, err := prepareClaude(claudeConfig(), packet, adapter)
	if err != nil {
		t.Fatal(err)
	}
	return invocation, receipt, store
}

func TestClaudeFixturePinsTheDirectStreamContract(t *testing.T) {
	fixture := loadClaudeFixture(t)
	if fixture.Runtime != string(Claude) || fixture.CLIVersion != ClaudeVersion {
		t.Fatalf("fixture runtime/version = %q/%q, want claude/%s", fixture.Runtime, fixture.CLIVersion, ClaudeVersion)
	}
	if !reflect.DeepEqual(fixture.DirectArgv, claudeArgs) {
		t.Fatalf("fixture argv = %#v, adapter argv = %#v", fixture.DirectArgv, claudeArgs)
	}
	if fixture.CredentialEnvironment != "ANTHROPIC_API_KEY" || fixture.ChildEnvironment != "bare: exactly the credential environment entry" {
		t.Fatalf("fixture credential contract = %q / %q", fixture.CredentialEnvironment, fixture.ChildEnvironment)
	}
	if fixture.NativeExamples == "" || !strings.Contains(fixture.Admission, "unadmitted") {
		t.Fatalf("fixture omits its native examples or falsely claims admission: %#v", fixture)
	}
}

func TestClaudePreparationDeliversExactlyThePacketThroughBareStreamInput(t *testing.T) {
	invocation, _, _ := prepareClaudeFixture(t)
	packet, err := AssemblePacket(packetInput())
	if err != nil {
		t.Fatal(err)
	}
	if invocation.Path != claudeConfig().Executable || !reflect.DeepEqual(invocation.Args, claudeArgs) {
		t.Fatalf("Claude invocation = %#v, want configured path and fixed argv", invocation)
	}
	if invocation.Output == nil {
		t.Fatal("Claude invocation exposes no native stdout receiver")
	}
	if got, want := invocation.Env, []string{"ANTHROPIC_API_KEY=test-anthropic-key"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Claude environment = %#v, want its one injected credential", got)
	}
	for _, arg := range invocation.Args {
		if strings.Contains(arg, claudeConfig().AnthropicAPIKey) {
			t.Fatal("Claude credential was put in argv")
		}
	}
	var input struct {
		Type    string `json:"type"`
		Message struct {
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"message"`
	}
	if !bytes.HasSuffix(invocation.Input, []byte{'\n'}) || json.Unmarshal(bytes.TrimSuffix(invocation.Input, []byte{'\n'}), &input) != nil {
		t.Fatalf("Claude input is not one JSONL message: %q", invocation.Input)
	}
	if input.Type != "user" || input.Message.Role != "user" || len(input.Message.Content) != 1 || input.Message.Content[0].Type != "text" || input.Message.Content[0].Text != string(packet.Bytes()) {
		t.Fatalf("Claude stream input rebuilt or changed the immutable packet: %#v", input)
	}
}

func TestClaudePreparationRefusesEveryCredentialAndVersionGuard(t *testing.T) {
	packet, err := AssemblePacket(packetInput())
	if err != nil {
		t.Fatal(err)
	}
	receipt, _ := testReceipt(t)
	adapter, err := receipt.ForAdapter(Claude, claudeAdapterVersion)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		edit func(*RuntimeConfig, *PacketInput)
		want string
	}{
		{"wrong runtime", func(cfg *RuntimeConfig, _ *PacketInput) { cfg.Runtime = string(Codex) }, "does not name claude"},
		{"relative executable", func(cfg *RuntimeConfig, _ *PacketInput) { cfg.Executable = "claude" }, "explicit absolute path"},
		{"wrong version", func(cfg *RuntimeConfig, _ *PacketInput) { cfg.Version = "2.1.237" }, "version is unsupported"},
		{"no credential", func(cfg *RuntimeConfig, _ *PacketInput) { cfg.AnthropicAPIKey = "" }, "ANTHROPIC_API_KEY is unset"},
		{"credential in packet", func(cfg *RuntimeConfig, in *PacketInput) { in.Why = cfg.AnthropicAPIKey }, "appears in the packet"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := claudeConfig()
			in := packetInput()
			tc.edit(&cfg, &in)
			candidate, err := AssemblePacket(in)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := prepareClaude(cfg, candidate, adapter); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("prepareClaude(%+v) = %v, want refusal %q", cfg, err, tc.want)
			}
		})
	}
	if _, err := prepareClaude(claudeConfig(), packet, nil); err == nil || !strings.Contains(err.Error(), "adapter receipt is absent") {
		t.Fatalf("prepareClaude without an adapter receipt = %v, want refusal", err)
	}
}

func TestClaudeFixtureRetainsNativeEventsAndOnlyNativeBackedProjections(t *testing.T) {
	invocation, receipt, store := prepareClaudeFixture(t)
	fixture := loadClaudeFixture(t)
	src, err := os.ReadFile(filepath.Join("testdata", "claude", fixture.NativeExamples))
	if err != nil {
		t.Fatal(err)
	}
	// Deliberately split records across writes: confinement forwards a byte
	// stream, not a pre-tokenized event list.
	cut := bytes.IndexByte(src, '\n') + 7
	if _, err := invocation.Output.Write(src[:cut]); err != nil {
		t.Fatal(err)
	}
	if _, err := invocation.Output.Write(src[cut:]); err != nil {
		t.Fatal(err)
	}
	if err := invocation.Output.Close(); err != nil {
		t.Fatal(err)
	}

	entries, quarantined, err := ReadReceipt(store, receipt.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(quarantined) != 0 {
		t.Fatalf("fixture receipt quarantined its own evidence: %#v", quarantined)
	}
	if got, want := ReceiptText(entries), "thread_native,thread_native,thread_transcript,thread_native,thread_turn,thread_native,thread_cost,thread_turn"; got != want {
		t.Fatalf("fixture receipt kinds = %q, want %q", got, want)
	}
	if entries[1].Native.Type != "stream_event" || entries[2].Transcript.NativeRef != entries[1].Native.Ref {
		t.Fatalf("stream transcript did not refer to its retained native event: %#v", entries[1:3])
	}
	transcript, err := ledger.ReadArtifact(store, entries[2].Transcript.Artifact)
	if err != nil {
		t.Fatal(err)
	}
	if string(transcript) != "partial repair evidence" {
		t.Fatalf("stream transcript = %q, want the native delta", transcript)
	}
	if entries[4].Turn.NativeRef != entries[3].Native.Ref || entries[4].Turn.Count != 1 {
		t.Fatalf("assistant turn = %#v, want its first native turn boundary", entries[4].Turn)
	}
	if entries[5].Native.Type != "result" || entries[6].Cost.NativeRef != entries[5].Native.Ref || entries[6].Cost.Amount != "0.0042" || entries[7].Turn.NativeRef != entries[5].Native.Ref || entries[7].Turn.Count != 2 {
		t.Fatalf("result projections = %#v", entries[5:])
	}
}

func TestClaudeStreamRedactsItsInjectedCredentialBeforeRetention(t *testing.T) {
	invocation, receipt, store := prepareClaudeFixture(t)
	src := []byte(`{"type":"assistant","message":{"content":[{"type":"text","text":"test-anthropic-key"}]}}` + "\n")
	if _, err := invocation.Output.Write(src); err != nil {
		t.Fatal(err)
	}
	if err := invocation.Output.Close(); err != nil {
		t.Fatal(err)
	}
	entries, quarantined, err := ReadReceipt(store, receipt.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(quarantined) != 0 || len(entries) != 3 {
		t.Fatalf("redacted receipt = %#v, quarantine = %#v", entries, quarantined)
	}
	if entries[0].Native.Redaction.Matches != 1 || entries[1].Transcript.Redaction.Matches != 1 {
		t.Fatalf("redactions = %#v %#v, want one replacement in each durable form", entries[0].Native.Redaction, entries[1].Transcript.Redaction)
	}
	durable, err := ledger.ReadArtifact(store, entries[1].Transcript.Artifact)
	if err != nil {
		t.Fatal(err)
	}
	for _, retained := range [][]byte{entries[0].Native.Payload, durable} {
		if bytes.Contains(retained, []byte("test-anthropic-key")) || !bytes.Contains(retained, []byte("[REDACTED]")) {
			t.Fatalf("credential escaped redaction into durable evidence: %q", retained)
		}
	}
}

func TestClaudeStreamRefusesMalformedNativeEvidence(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  []byte
		want string
	}{
		{"missing type", []byte(`{"message":{}}` + "\n"), "has no type"},
		{"invalid JSON", []byte(`{"type":` + "\n"), "decode Claude native stream event"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invocation, receipt, store := prepareClaudeFixture(t)
			if _, err := invocation.Output.Write(tc.src); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("write error = %v, want refusal %q", err, tc.want)
			}
			entries, _, err := ReadReceipt(store, receipt.ID())
			if err != nil && !strings.Contains(err.Error(), "no such ledger stream") {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatalf("invalid Claude output made durable receipt entries: %#v", entries)
			}
		})
	}
}

func TestClaudeAdapterRemainsUnadmittedWithoutTheLiveProof(t *testing.T) {
	adapter, err := Select(string(Claude))
	if err != nil {
		t.Fatal(err)
	}
	packet, err := AssemblePacket(packetInput())
	if err != nil {
		t.Fatal(err)
	}
	receipt, _ := testReceipt(t)
	adapterReceipt, err := receipt.ForAdapter(Claude, adapter.AdapterVersion())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Prepare(claudeConfig(), packet, adapterReceipt); err == nil || !strings.Contains(err.Error(), "unadmitted") {
		t.Fatalf("Claude adapter launched without its interrupted live proof: %v", err)
	}
}
