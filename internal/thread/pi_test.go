package thread

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/ledger"
)

type piFixture struct {
	Runtime               string   `json:"runtime"`
	CLIVersion            string   `json:"cli_version"`
	DirectArgv            []string `json:"direct_argv"`
	CredentialEnvironment string   `json:"credential_environment"`
	ChildEnvironment      string   `json:"child_environment"`
	NativeExamples        string   `json:"native_examples"`
	Admission             string   `json:"admission"`
}

func loadPiFixture(t *testing.T) piFixture {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("testdata", "pi", "fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture piFixture
	if err := json.Unmarshal(src, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func piConfig(t *testing.T) RuntimeConfig {
	t.Helper()
	where := filepath.Join(t.TempDir(), "pi")
	if err := os.WriteFile(where, []byte("fixture executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	return RuntimeConfig{
		Runtime:                 string(Pi),
		Executable:              where,
		Version:                 PiVersion,
		PiProvider:              "anthropic",
		PiModel:                 "claude-sonnet-4-20250514",
		PiCredentialEnvironment: "ANTHROPIC_API_KEY",
		PiCredential:            "test-pi-key",
		PiHome:                  t.TempDir(),
	}
}

func preparePiFixture(t *testing.T) (Invocation, *Receipt, ledger.Store) {
	t.Helper()
	receipt, store := testReceipt(t)
	adapter, err := receipt.ForAdapter(Pi, piAdapterVersion)
	if err != nil {
		t.Fatal(err)
	}
	packet, err := AssemblePacket(packetInput())
	if err != nil {
		t.Fatal(err)
	}
	invocation, err := preparePi(piConfig(t), packet, adapter)
	if err != nil {
		t.Fatal(err)
	}
	return invocation, receipt, store
}

func TestPiFixturePinsTheDirectRPCContract(t *testing.T) {
	fixture := loadPiFixture(t)
	if fixture.Runtime != string(Pi) || fixture.CLIVersion != PiVersion {
		t.Fatalf("fixture runtime/version = %q/%q, want pi/%s", fixture.Runtime, fixture.CLIVersion, PiVersion)
	}
	cfg := piConfig(t)
	if !reflect.DeepEqual(fixture.DirectArgv, piArgv(cfg)) {
		t.Fatalf("fixture argv = %#v, adapter argv = %#v", fixture.DirectArgv, piArgv(cfg))
	}
	if fixture.CredentialEnvironment != cfg.PiCredentialEnvironment || !strings.Contains(fixture.ChildEnvironment, "exactly the configured provider credential") {
		t.Fatalf("fixture credential contract = %q / %q", fixture.CredentialEnvironment, fixture.ChildEnvironment)
	}
	if fixture.NativeExamples == "" || !strings.Contains(fixture.Admission, "unadmitted") {
		t.Fatalf("fixture omits native examples or falsely claims admission: %#v", fixture)
	}
}

func TestPiPreparationDeliversTheExactPacketAsOneStrictLFPrompt(t *testing.T) {
	receipt, _ := testReceipt(t)
	adapter, err := receipt.ForAdapter(Pi, piAdapterVersion)
	if err != nil {
		t.Fatal(err)
	}
	cfg := piConfig(t)
	in := packetInput()
	in.Why = "/skill:adversarial-packet-marker must remain data"
	packet, err := AssemblePacket(in)
	if err != nil {
		t.Fatal(err)
	}
	invocation, err := preparePi(cfg, packet, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if invocation.Path != cfg.Executable || !reflect.DeepEqual(invocation.Args, piArgv(cfg)) {
		t.Fatalf("Pi invocation = %#v, want configured path and fixed argv", invocation)
	}
	if invocation.Output == nil {
		t.Fatal("Pi invocation exposes no native stdout receiver")
	}
	wantEnv := []string{
		"HOME=" + cfg.PiHome,
		"PI_CODING_AGENT_DIR=" + filepath.Join(cfg.PiHome, "agent"),
		"ANTHROPIC_API_KEY=test-pi-key",
	}
	if !reflect.DeepEqual(invocation.Env, wantEnv) {
		t.Fatalf("Pi environment = %#v, want clean home/config plus its one credential", invocation.Env)
	}
	for _, arg := range invocation.Args {
		if strings.Contains(arg, cfg.PiCredential) {
			t.Fatal("Pi credential was put in argv")
		}
	}
	for _, required := range []string{"--no-extensions", "--no-skills", "--no-prompt-templates", "--no-context-files"} {
		if !containsArg(invocation.Args, required) {
			t.Fatalf("Pi argv omitted %q, allowing packet interpretation or discovery", required)
		}
	}
	if !bytes.HasSuffix(invocation.Input, []byte{'\n'}) || bytes.Count(invocation.Input, []byte{'\n'}) != 1 || bytes.Contains(invocation.Input, []byte{'\r'}) {
		t.Fatalf("Pi input is not exactly one strict-LF JSONL record: %q", invocation.Input)
	}
	var prompt struct {
		ID      string `json:"id"`
		Type    string `json:"type"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(bytes.TrimSuffix(invocation.Input, []byte{'\n'}), &prompt); err != nil {
		t.Fatalf("Pi input is not a JSON RPC prompt: %v", err)
	}
	if prompt.ID == "" || prompt.Type != "prompt" || prompt.Message != string(packet.Bytes()) {
		t.Fatalf("Pi RPC prompt rebuilt or changed immutable slash-prefixed packet: %#v", prompt)
	}
}

func TestPiPreparationRefusesEveryConfigurationGuard(t *testing.T) {
	receipt, _ := testReceipt(t)
	adapter, err := receipt.ForAdapter(Pi, piAdapterVersion)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		edit func(t *testing.T, cfg *RuntimeConfig, in *PacketInput)
		want string
	}{
		{"wrong runtime", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) { cfg.Runtime = string(Claude) }, "does not name pi"},
		{"no executable", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) { cfg.Executable = "" }, "executable is unset"},
		{"relative executable", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) { cfg.Executable = "pi" }, "explicit absolute path"},
		{"missing executable", func(t *testing.T, cfg *RuntimeConfig, _ *PacketInput) {
			cfg.Executable = filepath.Join(t.TempDir(), "missing-pi")
		}, "executable is unavailable"},
		{"nonexecutable file", func(t *testing.T, cfg *RuntimeConfig, _ *PacketInput) {
			cfg.Executable = filepath.Join(t.TempDir(), "not-executable")
			if err := os.WriteFile(cfg.Executable, []byte("fixture"), 0o600); err != nil {
				t.Fatal(err)
			}
		}, "executable is not executable"},
		{"no version", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) { cfg.Version = "" }, "version is unset"},
		{"wrong version", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) { cfg.Version = "0.83.0" }, "version is unsupported"},
		{"no packet", func(_ *testing.T, _ *RuntimeConfig, in *PacketInput) { *in = PacketInput{} }, "packet is absent"},
		{"no provider", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) { cfg.PiProvider = "" }, "provider is unset"},
		{"provider is an option", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) { cfg.PiProvider = "--no-tools" }, "provider is not a direct argv value"},
		{"no model", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) { cfg.PiModel = "" }, "model is unset"},
		{"model is an option", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) { cfg.PiModel = "--no-tools" }, "model is not a direct argv value"},
		{"no credential environment", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) { cfg.PiCredentialEnvironment = "" }, "credential environment is unset"},
		{"invalid credential environment", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) { cfg.PiCredentialEnvironment = "bad=key" }, "credential environment name is invalid"},
		{"reserved credential environment", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) { cfg.PiCredentialEnvironment = "HOME" }, "credential environment is reserved"},
		{"no credential", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) { cfg.PiCredential = "" }, "credential is unset"},
		{"NUL credential", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) { cfg.PiCredential = "test\x00key" }, "credential contains NUL"},
		{"credential in packet", func(_ *testing.T, cfg *RuntimeConfig, in *PacketInput) {
			cfg.PiCredential = "test\"key"
			in.Why = cfg.PiCredential
		}, "credential appears in the packet"},
		{"no HOME", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) { cfg.PiHome = "" }, "HOME is unset"},
		{"relative HOME", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) { cfg.PiHome = "home" }, "HOME is not an explicit absolute path"},
		{"missing HOME", func(t *testing.T, cfg *RuntimeConfig, _ *PacketInput) {
			cfg.PiHome = filepath.Join(t.TempDir(), "missing-home")
		}, "HOME is unavailable"},
		{"HOME is a file", func(t *testing.T, cfg *RuntimeConfig, _ *PacketInput) {
			cfg.PiHome = filepath.Join(t.TempDir(), "not-a-home")
			if err := os.WriteFile(cfg.PiHome, []byte("fixture"), 0o600); err != nil {
				t.Fatal(err)
			}
		}, "HOME is not a directory"},
		{"dirty HOME", func(t *testing.T, cfg *RuntimeConfig, _ *PacketInput) {
			if err := os.WriteFile(filepath.Join(cfg.PiHome, "auth.json"), []byte("do not reuse"), 0o600); err != nil {
				t.Fatal(err)
			}
		}, "HOME is not clean"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := piConfig(t)
			in := packetInput()
			tc.edit(t, &cfg, &in)
			var packet Packet
			if in.Promise.ID == "" {
				packet = Packet{}
			} else {
				var err error
				packet, err = AssemblePacket(in)
				if err != nil {
					t.Fatal(err)
				}
			}
			if _, err := preparePi(cfg, packet, adapter); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("preparePi(%+v) = %v, want refusal %q", cfg, err, tc.want)
			}
		})
	}
	packet, err := AssemblePacket(packetInput())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := preparePi(piConfig(t), packet, nil); err == nil || !strings.Contains(err.Error(), "adapter receipt is absent") {
		t.Fatalf("preparePi without an adapter receipt = %v, want refusal", err)
	}
}

func TestPiFixtureRetainsNativeRPCEventsAndOnlyNativeBackedProjections(t *testing.T) {
	invocation, receipt, store := preparePiFixture(t)
	fixture := loadPiFixture(t)
	src, err := os.ReadFile(filepath.Join("testdata", "pi", fixture.NativeExamples))
	if err != nil {
		t.Fatal(err)
	}
	cut := bytes.IndexByte(src, '\n') + 9
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
	if got, want := ReceiptText(entries), "thread_native,thread_native,thread_native,thread_native,thread_transcript,thread_native,thread_native,thread_cost,thread_turn,thread_native,thread_native,thread_transcript,thread_native,thread_cost,thread_turn"; got != want {
		t.Fatalf("fixture receipt kinds = %q, want %q", got, want)
	}
	if entries[3].Native.Type != "message_update" || entries[4].Transcript.NativeRef != entries[3].Native.Ref {
		t.Fatalf("Pi transcript did not refer to its retained message_update: %#v", entries[3:5])
	}
	first, err := ledger.ReadArtifact(store, entries[4].Transcript.Artifact)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != "partial repair evidence" {
		t.Fatalf("first Pi transcript = %q, want streamed native delta", first)
	}
	if entries[6].Native.Type != "turn_end" || entries[7].Cost.NativeRef != entries[6].Native.Ref || entries[7].Cost.Amount != "0.0042" || entries[8].Turn.NativeRef != entries[6].Native.Ref || entries[8].Turn.Count != 1 {
		t.Fatalf("first Pi turn projections = %#v", entries[6:9])
	}
	if entries[12].Native.Type != "turn_end" || entries[13].Cost.NativeRef != entries[12].Native.Ref || entries[13].Cost.Amount != "0.0063" || entries[14].Turn.NativeRef != entries[12].Native.Ref || entries[14].Turn.Count != 2 {
		t.Fatalf("second Pi turn projections = %#v", entries[12:])
	}
}

func TestPiStreamRedactsTheInjectedCredentialBeforeRetention(t *testing.T) {
	receipt, store := testReceipt(t)
	adapter, err := receipt.ForAdapter(Pi, piAdapterVersion)
	if err != nil {
		t.Fatal(err)
	}
	cfg := piConfig(t)
	cfg.PiCredential = "test\"pi-key"
	packet, err := AssemblePacket(packetInput())
	if err != nil {
		t.Fatal(err)
	}
	invocation, err := preparePi(cfg, packet, adapter)
	if err != nil {
		t.Fatal(err)
	}
	src, err := json.Marshal(struct {
		Type                  string `json:"type"`
		AssistantMessageEvent struct {
			Type  string `json:"type"`
			Delta string `json:"delta"`
		} `json:"assistantMessageEvent"`
	}{
		Type: "message_update",
	})
	if err != nil {
		t.Fatal(err)
	}
	var event struct {
		Type                  string `json:"type"`
		AssistantMessageEvent struct {
			Type  string `json:"type"`
			Delta string `json:"delta"`
		} `json:"assistantMessageEvent"`
	}
	if err := json.Unmarshal(src, &event); err != nil {
		t.Fatal(err)
	}
	event.AssistantMessageEvent.Type = "text_delta"
	event.AssistantMessageEvent.Delta = cfg.PiCredential
	src, err = json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := invocation.Output.Write(append(src, '\n')); err != nil {
		t.Fatal(err)
	}
	if err := invocation.Output.Close(); err != nil {
		t.Fatal(err)
	}
	entries, quarantined, err := ReadReceipt(store, receipt.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(quarantined) != 0 || len(entries) != 2 {
		t.Fatalf("redacted Pi receipt = %#v, quarantine = %#v", entries, quarantined)
	}
	if entries[0].Native.Redaction.Matches != 1 || entries[1].Transcript.Redaction.Matches != 1 {
		t.Fatalf("Pi redactions = %#v %#v, want one replacement in both durable forms", entries[0].Native.Redaction, entries[1].Transcript.Redaction)
	}
	durable, err := ledger.ReadArtifact(store, entries[1].Transcript.Artifact)
	if err != nil {
		t.Fatal(err)
	}
	for _, retained := range [][]byte{entries[0].Native.Payload, durable} {
		if packetContains(cfg.PiCredential, retained) || !bytes.Contains(retained, []byte("[REDACTED]")) {
			t.Fatalf("Pi credential escaped redaction into durable evidence: %q", retained)
		}
	}
}

func TestPiStreamRefusesMalformedNativeEvidence(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  []byte
		want string
	}{
		{"missing type", []byte(`{"message":{}}` + "\n"), "has no type"},
		{"invalid JSON", []byte(`{"type":` + "\n"), "decode Pi RPC native event"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invocation, receipt, store := preparePiFixture(t)
			if _, err := invocation.Output.Write(tc.src); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("write error = %v, want refusal %q", err, tc.want)
			}
			entries, _, err := ReadReceipt(store, receipt.ID())
			if err != nil && !strings.Contains(err.Error(), "no such ledger stream") {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatalf("invalid Pi output made durable receipt entries: %#v", entries)
			}
		})
	}
}

func TestPiSupervisorRetainsEventsBeforeItsParentDeadlineLifecycle(t *testing.T) {
	invocation, receipt, store := preparePiFixture(t)
	if _, err := invocation.Output.Write([]byte(`{"type":"message_update","assistantMessageEvent":{"type":"text_delta","delta":"before deadline"}}` + "\n")); err != nil {
		t.Fatal(err)
	}
	child := &fakeChild{wait: make(chan Exit, 1)}
	launch := &fakeLaunch{child: child}
	if err := (Supervisor{Timeout: time.Millisecond}).Run(context.Background(), launch, invocation, receipt); err != nil {
		t.Fatal(err)
	}
	entries, quarantined, err := ReadReceipt(store, receipt.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(quarantined) != 0 || ReceiptText(entries) != "thread_native,thread_transcript,thread_lifecycle,thread_lifecycle,thread_lifecycle,thread_lifecycle" {
		t.Fatalf("Pi interruption receipt = %q, quarantine %#v", ReceiptText(entries), quarantined)
	}
	if entries[0].Native.Type != "message_update" || entries[2].Lifecycle.Event != Started || entries[3].Lifecycle.Event != TimedOut || entries[4].Lifecycle.Event != Killed || entries[5].Lifecycle.Event != Exited || entries[5].Lifecycle.Exit.Signal != "SIGKILL" {
		t.Fatalf("Pi interruption lost native evidence or parent lifecycle: %#v", entries)
	}
	if child.killCall != 1 {
		t.Fatalf("Pi deadline killed %d child groups, want one", child.killCall)
	}
}

func TestPiAdapterRemainsUnadmittedWithoutTheLiveProof(t *testing.T) {
	adapter, err := Select(string(Pi))
	if err != nil {
		t.Fatal(err)
	}
	packet, err := AssemblePacket(packetInput())
	if err != nil {
		t.Fatal(err)
	}
	receipt, _ := testReceipt(t)
	adapterReceipt, err := receipt.ForAdapter(Pi, adapter.AdapterVersion())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Prepare(piConfig(t), packet, adapterReceipt); err == nil || !strings.Contains(err.Error(), "unadmitted") {
		t.Fatalf("Pi adapter launched without its interrupted live proof: %v", err)
	}
	launch := &fakeLaunch{verified: errors.New("should not be reached"), child: normalChild(0)}
	if err := Dispatch(context.Background(), piConfig(t), packet, launch, Supervisor{Timeout: time.Second}, receipt); err == nil || !strings.Contains(err.Error(), "canary failed") {
		t.Fatalf("Pi dispatch did not refuse before runtime preparation: %v", err)
	}
	if launch.started != 0 {
		t.Fatalf("unadmitted Pi launched %d children", launch.started)
	}
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}
