package thread

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/ledger"
)

type claudeFixture struct {
	Runtime               string   `json:"runtime"`
	CLIVersion            string   `json:"cli_version"`
	DirectArgv            []string `json:"direct_argv"`
	CredentialEnvironment string   `json:"credential_environment"`
	Credential            string   `json:"credential"`
	CredentialExpiry      string   `json:"credential_expiry"`
	AcceptedExposure      string   `json:"accepted_exposure"`
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

// claudeConfig is the safe path: a token with life left in it and an empty
// session home. It also clears ANTHROPIC_API_KEY for the test process, because
// the adapter refuses when that name is exported and whether a developer's
// shell happens to export it is not what any of these tests are about.
// t.Setenv records the prior state, so the cleanup restores it either way.
func claudeConfig(t *testing.T) RuntimeConfig {
	t.Helper()
	t.Setenv(claudeAPIKeyEnvironment, "cleared by the test harness")
	if err := os.Unsetenv(claudeAPIKeyEnvironment); err != nil {
		t.Fatal(err)
	}
	return RuntimeConfig{
		Runtime:                string(Claude),
		Executable:             "/configured/claude",
		Version:                ClaudeVersion,
		ClaudeSetupToken:       "test-claude-setup-token",
		ClaudeSetupTokenExpiry: time.Now().Add(365 * 24 * time.Hour),
		ClaudeHome:             t.TempDir(),
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
	invocation, err := prepareClaude(claudeConfig(t), packet, adapter)
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
	if fixture.CredentialEnvironment != claudeCredentialEnvironment || !strings.Contains(fixture.ChildEnvironment, "exactly the credential environment entry") {
		t.Fatalf("fixture credential contract = %q / %q", fixture.CredentialEnvironment, fixture.ChildEnvironment)
	}
	// Decision 0052 asks a direct environment channel to record the documented
	// derived credential and the exposure it accepts, not only the name.
	if !strings.Contains(fixture.Credential, "setup-token") || fixture.CredentialExpiry == "" || !strings.Contains(fixture.AcceptedExposure, "can read") {
		t.Fatalf("fixture omits its credential, expiry or accepted exposure: %#v", fixture)
	}
	if strings.Contains(fixture.ChildEnvironment, claudeAPIKeyEnvironment) && !strings.Contains(fixture.ChildEnvironment, claudeAPIKeyEnvironment+" is refused") {
		t.Fatalf("fixture describes an API key reaching the child: %q", fixture.ChildEnvironment)
	}
	if fixture.NativeExamples == "" || !strings.Contains(fixture.Admission, "unadmitted") {
		t.Fatalf("fixture omits its native examples or falsely claims admission: %#v", fixture)
	}
	// --bare is not a stale flag: its own help says OAuth is never read under
	// it, so it and the subscription credential channel cannot both be right.
	for _, arg := range fixture.DirectArgv {
		if arg == "--bare" {
			t.Fatal("fixture argv carries --bare, which ignores the OAuth token this adapter injects")
		}
	}
}

func TestClaudePreparationDeliversExactlyThePacketThroughItsStreamInput(t *testing.T) {
	receipt, _ := testReceipt(t)
	adapter, err := receipt.ForAdapter(Claude, claudeAdapterVersion)
	if err != nil {
		t.Fatal(err)
	}
	cfg := claudeConfig(t)
	packet, err := AssemblePacket(packetInput())
	if err != nil {
		t.Fatal(err)
	}
	invocation, err := prepareClaude(cfg, packet, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if invocation.Path != cfg.Executable || !reflect.DeepEqual(invocation.Args, claudeArgs) {
		t.Fatalf("Claude invocation = %#v, want configured path and fixed argv", invocation)
	}
	if invocation.Output == nil {
		t.Fatal("Claude invocation exposes no native stdout receiver")
	}
	wantEnv := []string{
		"HOME=" + cfg.ClaudeHome,
		"CLAUDE_CONFIG_DIR=" + filepath.Join(cfg.ClaudeHome, "config"),
		"CLAUDE_CODE_OAUTH_TOKEN=test-claude-setup-token",
	}
	if !reflect.DeepEqual(invocation.Env, wantEnv) {
		t.Fatalf("Claude environment = %#v, want a clean home plus exactly the subscription token", invocation.Env)
	}
	// The environment is constructed, not inherited, so the key Claude would
	// prefer over the subscription token has nowhere to come from.
	for _, entry := range invocation.Env {
		if strings.HasPrefix(entry, claudeAPIKeyEnvironment+"=") {
			t.Fatalf("Claude environment carries an API key: %q", entry)
		}
	}
	if containsArg(invocation.Args, "--bare") {
		t.Fatal("Claude argv carries --bare, under which OAuth and keychain are never read")
	}
	if !containsArg(invocation.Args, "--safe-mode") {
		t.Fatal("Claude argv omits --safe-mode, leaving CLAUDE.md, hooks, plugins and MCP servers discoverable")
	}
	for _, required := range []string{"--print", "--input-format", "--output-format", "--include-partial-messages", "--no-session-persistence"} {
		if !containsArg(invocation.Args, required) {
			t.Fatalf("Claude argv omitted %q", required)
		}
	}
	for _, arg := range invocation.Args {
		if strings.Contains(arg, cfg.ClaudeSetupToken) {
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
	receipt, _ := testReceipt(t)
	adapter, err := receipt.ForAdapter(Claude, claudeAdapterVersion)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		edit func(t *testing.T, cfg *RuntimeConfig, in *PacketInput)
		want string
	}{
		{"wrong runtime", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) { cfg.Runtime = string(Codex) }, "does not name claude"},
		{"no executable", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) { cfg.Executable = "" }, "executable is unset"},
		{"relative executable", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) { cfg.Executable = "claude" }, "explicit absolute path"},
		{"no version", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) { cfg.Version = "" }, "version is unset"},
		{"wrong version", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) { cfg.Version = "2.1.238" }, "version is unsupported"},
		{"no packet", func(_ *testing.T, _ *RuntimeConfig, in *PacketInput) { *in = PacketInput{} }, "packet is absent"},
		{"no credential", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) { cfg.ClaudeSetupToken = "" }, "CLAUDE_CODE_OAUTH_TOKEN is unset"},
		{"NUL credential", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) {
			cfg.ClaudeSetupToken = "test\x00token"
		}, "CLAUDE_CODE_OAUTH_TOKEN contains NUL"},
		{"credential in packet", func(_ *testing.T, cfg *RuntimeConfig, in *PacketInput) {
			in.Why = cfg.ClaudeSetupToken
		}, "CLAUDE_CODE_OAUTH_TOKEN appears in the packet"},
		{"escaped credential in packet", func(_ *testing.T, cfg *RuntimeConfig, in *PacketInput) {
			cfg.ClaudeSetupToken = "test\"token"
			in.Why = cfg.ClaudeSetupToken
		}, "CLAUDE_CODE_OAUTH_TOKEN appears in the packet"},
		{"no expiry", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) {
			cfg.ClaudeSetupTokenExpiry = time.Time{}
		}, "setup-token expiry is unset"},
		{"lapsed token", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) {
			cfg.ClaudeSetupTokenExpiry = time.Now().Add(-time.Minute)
		}, "setup-token has lapsed"},
		{"token lapsing inside the margin", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) {
			cfg.ClaudeSetupTokenExpiry = time.Now().Add(claudeSetupTokenMargin - time.Hour)
		}, "setup-token lapses inside the dispatch margin"},
		{"inherited API key", func(t *testing.T, _ *RuntimeConfig, _ *PacketInput) {
			t.Setenv(claudeAPIKeyEnvironment, "sk-ant-should-never-be-used")
		}, "ANTHROPIC_API_KEY is present in the parent environment"},
		{"exported empty API key", func(t *testing.T, _ *RuntimeConfig, _ *PacketInput) {
			t.Setenv(claudeAPIKeyEnvironment, "")
		}, "ANTHROPIC_API_KEY is present in the parent environment"},
		{"no HOME", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) { cfg.ClaudeHome = "" }, "HOME is unset"},
		{"relative HOME", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) {
			cfg.ClaudeHome = "home"
		}, "HOME is not an explicit absolute path"},
		{"missing HOME", func(t *testing.T, cfg *RuntimeConfig, _ *PacketInput) {
			cfg.ClaudeHome = filepath.Join(t.TempDir(), "missing-home")
		}, "HOME is unavailable"},
		{"HOME is a file", func(t *testing.T, cfg *RuntimeConfig, _ *PacketInput) {
			cfg.ClaudeHome = filepath.Join(t.TempDir(), "not-a-home")
			if err := os.WriteFile(cfg.ClaudeHome, []byte("fixture"), 0o600); err != nil {
				t.Fatal(err)
			}
		}, "HOME is not a directory"},
		{"inherited Claude configuration", func(t *testing.T, cfg *RuntimeConfig, _ *PacketInput) {
			if err := os.WriteFile(filepath.Join(cfg.ClaudeHome, ".claude.json"), []byte("do not reuse"), 0o600); err != nil {
				t.Fatal(err)
			}
		}, "HOME is not clean"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := claudeConfig(t)
			in := packetInput()
			tc.edit(t, &cfg, &in)
			var candidate Packet
			if in.Promise.ID != "" {
				var err error
				candidate, err = AssemblePacket(in)
				if err != nil {
					t.Fatal(err)
				}
			}
			if _, err := prepareClaude(cfg, candidate, adapter); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("prepareClaude(%+v) = %v, want refusal %q", cfg, err, tc.want)
			}
		})
	}
	packet, err := AssemblePacket(packetInput())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := prepareClaude(claudeConfig(t), packet, nil); err == nil || !strings.Contains(err.Error(), "adapter receipt is absent") {
		t.Fatalf("prepareClaude without an adapter receipt = %v, want refusal", err)
	}
}

// TestClaudeSetupTokenLifeIsRefusedBeforeItLapses pins the boundary itself.
// `claude setup-token` takes no options and has no revoke, so nothing outside
// kept can shorten or check the credential's life; a margin that silently
// became zero would let a repair start on a token dying minutes later.
func TestClaudeSetupTokenLifeIsRefusedBeforeItLapses(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name   string
		expiry time.Time
		want   string
	}{
		{"unset", time.Time{}, "setup-token expiry is unset"},
		{"long past", now.Add(-30 * 24 * time.Hour), "setup-token has lapsed"},
		{"exactly now", now, "setup-token has lapsed"},
		{"one second inside the margin", now.Add(claudeSetupTokenMargin - time.Second), "setup-token lapses inside the dispatch margin"},
		{"exactly at the margin", now.Add(claudeSetupTokenMargin), ""},
		{"a year out", now.Add(365 * 24 * time.Hour), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := checkClaudeSetupTokenLife(tc.expiry, now)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("checkClaudeSetupTokenLife(%v) = %v, want no refusal", tc.expiry, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("checkClaudeSetupTokenLife(%v) = %v, want refusal %q", tc.expiry, err, tc.want)
			}
			if !strings.Contains(err.Error(), "dispatch refused") {
				t.Fatalf("expiry produced %v, want the dispatch Refusal shape", err)
			}
		})
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
	src := []byte(`{"type":"assistant","message":{"content":[{"type":"text","text":"test-claude-setup-token"}]}}` + "\n")
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
		if bytes.Contains(retained, []byte("test-claude-setup-token")) || !bytes.Contains(retained, []byte("[REDACTED]")) {
			t.Fatalf("credential escaped redaction into durable evidence: %q", retained)
		}
	}
}

// TestClaudeStreamRedactsTheEscapedCredentialToo covers the form a runtime
// actually produces when it echoes a credential back inside a JSON string. The
// packet check refuses both forms, so retention has to remove both or the two
// disagree about what counts as the secret appearing.
func TestClaudeStreamRedactsTheEscapedCredentialToo(t *testing.T) {
	receipt, store := testReceipt(t)
	adapter, err := receipt.ForAdapter(Claude, claudeAdapterVersion)
	if err != nil {
		t.Fatal(err)
	}
	cfg := claudeConfig(t)
	cfg.ClaudeSetupToken = "test\"token"
	packet, err := AssemblePacket(packetInput())
	if err != nil {
		t.Fatal(err)
	}
	invocation, err := prepareClaude(cfg, packet, adapter)
	if err != nil {
		t.Fatal(err)
	}
	src := []byte(`{"type":"assistant","message":{"content":[{"type":"text","text":"leaked test\"token here"}]}}` + "\n")
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
	if !entries[0].Native.Redaction.Occurred {
		t.Fatalf("escaped credential survived native retention: %s", entries[0].Native.Payload)
	}
	durable, err := ledger.ReadArtifact(store, entries[1].Transcript.Artifact)
	if err != nil {
		t.Fatal(err)
	}
	for _, retained := range [][]byte{entries[0].Native.Payload, durable} {
		if bytes.Contains(retained, []byte(`test\"token`)) || bytes.Contains(retained, []byte(cfg.ClaudeSetupToken)) {
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
	if _, err := adapter.Prepare(claudeConfig(t), packet, adapterReceipt); err == nil || !strings.Contains(err.Error(), "unadmitted") {
		t.Fatalf("Claude adapter launched without its interrupted live proof: %v", err)
	}
}
