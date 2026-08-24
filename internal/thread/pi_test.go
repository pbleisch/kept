package thread

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/runtimestate"
)

type piFixture struct {
	Runtime               string            `json:"runtime"`
	CLIVersion            string            `json:"cli_version"`
	DirectArgv            []string          `json:"direct_argv"`
	CredentialEnvironment string            `json:"credential_environment"`
	ChildEnvironment      string            `json:"child_environment"`
	PacketInputChannel    string            `json:"packet_input_channel"`
	ConfigurationChannel  string            `json:"configuration_channel"`
	AuthorizedEndpoints   string            `json:"authorized_endpoints"`
	Interpreter           string            `json:"interpreter"`
	NativeExamples        string            `json:"native_examples"`
	NativeEventTypes      []string          `json:"native_event_types"`
	ProjectionMapping     map[string]string `json:"projection_mapping"`
	Admission             string            `json:"admission"`
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

// The fixture Pi configuration is a self-hosted workshop endpoint on loopback.
// It is invented for the test: no operator source directory is read, and the
// credential is a fixture value that never leaves the child environment.
const (
	piFixtureProvider = "workshop"
	piFixtureModel    = "workshop-medium"
	piFixtureEndpoint = "http://localhost:1234/v1"
)

// piProjection materializes a Pi configuration the way a dispatcher would,
// from a source that mimics an operator's Pi agent directory.
func piProjection(t *testing.T) *runtimestate.PiState {
	t.Helper()
	state, _ := piProjectionAt(t, piFixtureEndpoint, nil)
	return state
}

// piProjectionAt is piProjection with the endpoint named, so a test that
// actually starts Pi can point it at a port nothing serves rather than at the
// loopback authority a workshop endpoint really listens on. It returns the box
// beside the projection as well, because a started runtime needs a tree to run
// in that is not the projection.
func piProjectionAt(t *testing.T, baseURL string, endpoints []string) (*runtimestate.PiState, string) {
	t.Helper()
	root := t.TempDir()
	source := filepath.Join(root, "source")
	box := filepath.Join(root, "box")
	if err := os.MkdirAll(source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(box, 0o700); err != nil {
		t.Fatal(err)
	}
	write := func(name string, value any) {
		src, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(source, name), src, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("auth.json", map[string]any{
		piFixtureProvider: map[string]any{"type": "api_key", "key": "fixture-source-side-value"},
	})
	write("models.json", map[string]any{"providers": map[string]any{
		piFixtureProvider: map[string]any{
			"baseUrl": baseURL,
			"api":     "openai-completions",
			"models":  []any{map[string]any{"id": piFixtureModel}},
		},
	}})
	state, err := runtimestate.ProjectPi(runtimestate.PiRequest{
		Source:    source,
		Dir:       filepath.Join(root, "state"),
		Box:       box,
		Provider:  piFixtureProvider,
		Model:     piFixtureModel,
		Endpoints: endpoints,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = state.Remove() })
	return state, box
}

func piConfig(t *testing.T) RuntimeConfig {
	t.Helper()
	where := filepath.Join(t.TempDir(), "pi")
	if err := os.WriteFile(where, []byte("fixture executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	return RuntimeConfig{
		Runtime:      string(Pi),
		Executable:   where,
		Version:      PiVersion,
		PiProvider:   piFixtureProvider,
		PiModel:      piFixtureModel,
		PiCredential: "test-pi-key",
		PiState:      piProjection(t),
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
	if fixture.CredentialEnvironment != runtimestate.PiCredentialEnvironment || !strings.Contains(fixture.ChildEnvironment, "exactly the configured provider credential") {
		t.Fatalf("fixture credential contract = %q / %q", fixture.CredentialEnvironment, fixture.ChildEnvironment)
	}
	// Decision 0052 admits a named child environment value or a broker route,
	// and the projection is neither: it is configuration. The fixture has to
	// say that the file holds a reference and the environment holds the value,
	// or a reader would take the copied configuration for the channel.
	if !strings.Contains(fixture.ConfigurationChannel, "$"+runtimestate.PiCredentialEnvironment+" reference") {
		t.Fatalf("fixture does not record that the projection holds a reference rather than a credential: %q", fixture.ConfigurationChannel)
	}
	if !strings.Contains(fixture.ConfigurationChannel, "read-only projection is not an option") {
		t.Fatalf("fixture omits the observed lock behaviour that decides the projection's shape: %q", fixture.ConfigurationChannel)
	}
	if !strings.Contains(fixture.AuthorizedEndpoints, runtimestate.PiDefaultEndpoint) {
		t.Fatalf("fixture does not record the default endpoint: %q", fixture.AuthorizedEndpoints)
	}
	// The packet-input channel is part of the contract and not an
	// implementation detail: Pi's session ends at input EOF, so who holds the
	// channel and what closes it decides whether a session happens at all.
	if !strings.Contains(fixture.PacketInputChannel, "agent_settled") || !strings.Contains(fixture.PacketInputChannel, "stdin") {
		t.Fatalf("fixture does not record the input channel that keeps a session alive: %q", fixture.PacketInputChannel)
	}
	if !strings.Contains(fixture.Interpreter, "search path") {
		t.Fatalf("fixture does not record how a confined launch reaches Pi's interpreter: %q", fixture.Interpreter)
	}
	if fixture.NativeExamples == "" {
		t.Fatalf("fixture omits its native examples: %#v", fixture)
	}
	// Every native type the fixture claims is in the example stream, and every
	// projection the adapter makes is mapped. A fixture that named a
	// vocabulary nothing demonstrates would be a description rather than
	// evidence.
	src, err := os.ReadFile(filepath.Join("testdata", "pi", fixture.NativeExamples))
	if err != nil {
		t.Fatal(err)
	}
	streamed := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(src)), "\n") {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			t.Fatal(err)
		}
		streamed[envelope.Type] = true
	}
	for _, typ := range []string{"response", "message_update", "turn_end", "agent_settled"} {
		if !streamed[typ] {
			t.Fatalf("the native example stream demonstrates no %s event", typ)
		}
	}
	for _, projection := range []string{"transcript", "cost", "turn", "lifecycle"} {
		if fixture.ProjectionMapping[projection] == "" {
			t.Fatalf("fixture maps no %s projection: %#v", fixture.ProjectionMapping, projection)
		}
	}
	// Admission is claimed, so it names the evidence rather than asserting it.
	if !strings.Contains(fixture.Admission, "internal/confine/piinbox_test.go") || !strings.Contains(fixture.Admission, "interrupted") {
		t.Fatalf("fixture claims admission without naming the interrupted confined proof: %q", fixture.Admission)
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
		"HOME=" + cfg.PiState.Dir(),
		"PI_CODING_AGENT_DIR=" + cfg.PiState.Dir(),
		runtimestate.PiCredentialEnvironment + "=test-pi-key",
	}
	if !reflect.DeepEqual(invocation.Env, wantEnv) {
		t.Fatalf("Pi environment = %#v, want the private projection plus its one credential", invocation.Env)
	}
	for _, arg := range invocation.Args {
		if strings.Contains(arg, cfg.PiCredential) {
			t.Fatal("Pi credential was put in argv")
		}
	}
	// Direct argv and no shell. There is no interpreter in the invocation and
	// no element that would become one, so nothing in the packet, the provider
	// or the model name can be read as a command. See docs/decision/0038.
	for _, value := range append([]string{invocation.Path}, invocation.Args...) {
		switch filepath.Base(value) {
		case "sh", "bash", "zsh", "dash", "env", "xargs":
			t.Fatalf("Pi invocation names an interpreter at %q", value)
		}
		if value == "-c" {
			t.Fatal("Pi invocation carries a shell command flag")
		}
	}
	// The projection is configuration, not the credential channel: the file
	// refers to the environment value and never carries it.
	for _, name := range []string{"auth.json", "models.json", "settings.json"} {
		src, err := os.ReadFile(filepath.Join(cfg.PiState.Dir(), name))
		if err != nil {
			t.Fatal(err)
		}
		if packetContains(cfg.PiCredential, src) {
			t.Fatalf("Pi credential was written into the projected %s", name)
		}
	}
	for _, required := range []string{"--no-extensions", "--no-skills", "--no-prompt-templates", "--no-context-files", "--offline"} {
		if !containsArg(invocation.Args, required) {
			t.Fatalf("Pi argv omitted %q, allowing packet interpretation or discovery", required)
		}
	}
	// The packet reaches the child through the bound channel and nowhere else:
	// Pi's session ends at input EOF, so the adapter holds the channel rather
	// than letting the launch write one shot and close it.
	if invocation.Input != nil {
		t.Fatalf("Pi invocation carries one-shot input the launch would close after: %q", invocation.Input)
	}
	if invocation.BindInput == nil {
		t.Fatal("Pi invocation binds no input channel, so its session would end at EOF")
	}
	channel := &countedInput{}
	if err := invocation.BindInput(channel); err != nil {
		t.Fatal(err)
	}
	written := channel.Bytes()
	if !bytes.HasSuffix(written, []byte{'\n'}) || bytes.Count(written, []byte{'\n'}) != 1 || bytes.Contains(written, []byte{'\r'}) {
		t.Fatalf("Pi input is not exactly one strict-LF JSONL record: %q", written)
	}
	if channel.closes != 0 {
		t.Fatalf("Pi closed its input channel %d times before the session settled", channel.closes)
	}
	var prompt struct {
		ID      string `json:"id"`
		Type    string `json:"type"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(bytes.TrimSuffix(written, []byte{'\n'}), &prompt); err != nil {
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
		{"no credential", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) { cfg.PiCredential = "" }, "credential is unset"},
		{"NUL credential", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) { cfg.PiCredential = "test\x00key" }, "credential contains NUL"},
		{"credential in packet", func(_ *testing.T, cfg *RuntimeConfig, in *PacketInput) {
			cfg.PiCredential = "test\"key"
			in.Why = cfg.PiCredential
		}, "credential appears in the packet"},
		{"no projection", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) { cfg.PiState = nil }, "projection is absent"},
		{"unmaterialized projection", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) {
			cfg.PiState = &runtimestate.PiState{}
		}, "projection is unset"},
		{"discarded projection", func(t *testing.T, cfg *RuntimeConfig, _ *PacketInput) {
			if err := cfg.PiState.Remove(); err != nil {
				t.Fatal(err)
			}
		}, "projection is unavailable"},
		{"projection is not a directory", func(t *testing.T, cfg *RuntimeConfig, _ *PacketInput) {
			dir := cfg.PiState.Dir()
			if err := os.RemoveAll(dir); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(dir, []byte("fixture"), 0o600); err != nil {
				t.Fatal(err)
			}
		}, "projection is not a directory"},
		{"projection is readable by the group", func(t *testing.T, cfg *RuntimeConfig, _ *PacketInput) {
			if err := os.Chmod(cfg.PiState.Dir(), 0o750); err != nil {
				t.Fatal(err)
			}
		}, "projection is not owner-only"},
		{"projected credential is readable by the group", func(t *testing.T, cfg *RuntimeConfig, _ *PacketInput) {
			if err := os.Chmod(filepath.Join(cfg.PiState.Dir(), "auth.json"), 0o640); err != nil {
				t.Fatal(err)
			}
		}, "projection entry is not owner-only"},
		{"projection is incomplete", func(t *testing.T, cfg *RuntimeConfig, _ *PacketInput) {
			if err := os.Remove(filepath.Join(cfg.PiState.Dir(), "models.json")); err != nil {
				t.Fatal(err)
			}
		}, "projection is incomplete"},
		{"projected entry is a directory", func(t *testing.T, cfg *RuntimeConfig, _ *PacketInput) {
			path := filepath.Join(cfg.PiState.Dir(), "settings.json")
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatal(err)
			}
		}, "projection entry is not a regular file"},
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
	if got, want := ReceiptText(entries), "thread_native,thread_native,thread_native,thread_native,thread_transcript,thread_native,thread_native,thread_cost,thread_turn,thread_native,thread_native,thread_transcript,thread_native,thread_cost,thread_turn,thread_native,thread_native"; got != want {
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
		t.Fatalf("second Pi turn projections = %#v", entries[12:15])
	}
	// The settled event is retained like any other and projects nothing. What
	// it decides is the input channel, which this fixture never bound.
	if entries[15].Native.Type != "agent_end" || entries[16].Native.Type != "agent_settled" {
		t.Fatalf("Pi did not retain its own end-of-session events: %#v", entries[15:])
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

// Pi is admitted, and admission is per adapter rather than per package: the
// evidence decision 0052 requires exists for Pi and does not exist for the
// other two, so selecting one of those still refuses.
func TestPiIsAdmittedAndTheOtherRuntimesAreNot(t *testing.T) {
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
	invocation, err := adapter.Prepare(piConfig(t), packet, adapterReceipt)
	if err != nil {
		t.Fatalf("the admitted Pi adapter refused to prepare an invocation: %v", err)
	}
	if invocation.Path == "" || invocation.BindInput == nil || invocation.Output == nil {
		t.Fatalf("the admitted Pi adapter prepared an incomplete invocation: %#v", invocation)
	}
	for _, runtime := range []Runtime{Claude, Codex} {
		other, err := Select(string(runtime))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := other.Prepare(RuntimeConfig{Runtime: string(runtime)}, packet, adapterReceipt); err == nil {
			t.Fatalf("%s prepared an invocation without its own admission evidence", runtime)
		}
	}
}

// Confinement is proved before an adapter is asked for anything. A failed
// canary refuses the dispatch, and the receipt shows that nothing was prepared,
// no packet was stored and no runtime was started.
func TestPiDispatchRefusesBeforePreparationWhenTheCanaryFailed(t *testing.T) {
	packet, err := AssemblePacket(packetInput())
	if err != nil {
		t.Fatal(err)
	}
	receipt, store := testReceipt(t)
	launch := &fakeLaunch{verified: errors.New("escape probe reached"), child: normalChild(0)}
	err = Dispatch(context.Background(), piConfig(t), packet, launch, Supervisor{Timeout: time.Second}, receipt)
	if err == nil || !strings.Contains(err.Error(), "canary failed") {
		t.Fatalf("Pi dispatch did not refuse on a failed canary: %v", err)
	}
	if launch.started != 0 {
		t.Fatalf("a failed canary still launched %d children", launch.started)
	}
	// Nothing at all: not a started lifecycle, not a stored packet, not an
	// empty stream. A refusal to dispatch is a disposition, and inventing a
	// thread lifecycle for a runtime that never ran would be a record of
	// something that did not happen.
	if entries, _, err := ReadReceipt(store, receipt.ID()); err == nil {
		t.Fatalf("a refused dispatch wrote a receipt stream: %s", ReceiptText(entries))
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

// piLockedFiles is every Pi 0.84.0 store that takes a proper-lockfile lock in
// its agent directory. auth.json, models-store.json and settings.json lock
// through FileAuthStorageBackend; trust.json locks through the trust manager;
// and models.json is read by the settings/model load path in the same
// directory. The lock is a directory created beside the file it guards.
var piLockedFiles = []string{"auth.json", "models.json", "settings.json", "models-store.json", "trust.json"}

// piTree is every path under root, so a test can say which paths a run created
// rather than only that the ones it expected are there.
func piTree(t *testing.T, root string) map[string]bool {
	t.Helper()
	paths := map[string]bool{}
	if err := filepath.WalkDir(root, func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		paths[path] = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return paths
}

// Pi takes a lock to *read* its auth store, model cache, settings and trust
// store, so the projection has to be writable and the writes have to be
// confined to it. This drives the environment the adapter actually hands to
// confinement, resolving the agent directory the way Pi resolves it, and
// requires every lock and every file that produces to land in the private
// projection and nowhere else — including the operator's source, which the
// projection was copied from and must never be written back to.
func TestPiLocksAndWritesLandInsideTheProjectionAndNowhereElse(t *testing.T) {
	receipt, _ := testReceipt(t)
	adapter, err := receipt.ForAdapter(Pi, piAdapterVersion)
	if err != nil {
		t.Fatal(err)
	}
	packet, err := AssemblePacket(packetInput())
	if err != nil {
		t.Fatal(err)
	}
	cfg := piConfig(t)
	invocation, err := preparePi(cfg, packet, adapter)
	if err != nil {
		t.Fatal(err)
	}
	environment := map[string]string{}
	for _, value := range invocation.Env {
		name, rest, _ := strings.Cut(value, "=")
		environment[name] = rest
	}

	// Pi reads PI_CODING_AGENT_DIR and otherwise joins HOME with .pi/agent, so
	// an adapter that set only one of them would leave the other resolving to
	// somewhere this test would then find writes in.
	agent := environment["PI_CODING_AGENT_DIR"]
	if agent == "" {
		agent = filepath.Join(environment["HOME"], ".pi", "agent")
	}
	if agent != cfg.PiState.Dir() || environment["HOME"] != cfg.PiState.Dir() {
		t.Fatalf("Pi would resolve its agent directory to %s with HOME %s, want the projection at %s", agent, environment["HOME"], cfg.PiState.Dir())
	}

	// The enclosing tree holds the projection, the operator source it was
	// copied from, and the box.
	enclosing := filepath.Dir(cfg.PiState.Dir())
	before := piTree(t, enclosing)

	for _, name := range piLockedFiles {
		path := filepath.Join(agent, name)
		// FileAuthStorageBackend.withLock creates the parent and the file
		// before locking, so a read of an absent store still writes.
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("Pi could not create the %s parent inside the projection: %v", name, err)
		}
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
				t.Fatalf("Pi could not create %s inside the projection: %v", name, err)
			}
		}
		lock := path + ".lock"
		if err := os.Mkdir(lock, 0o700); err != nil {
			t.Fatalf("Pi could not take its %s lock inside the projection: %v", name, err)
		}
		if _, err := os.Stat(lock); err != nil {
			t.Fatalf("the %s lock did not land beside its file: %v", name, err)
		}
		if err := os.Remove(lock); err != nil {
			t.Fatal(err)
		}
	}

	for path := range piTree(t, enclosing) {
		if before[path] {
			continue
		}
		rel, err := filepath.Rel(cfg.PiState.Dir(), path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			t.Fatalf("Pi wrote %s, which is outside the projection at %s", path, cfg.PiState.Dir())
		}
	}
}

// The projection's shape was decided by what the installed Pi does, so when
// that Pi is present the claim is checked against it rather than against the
// simulation above. The endpoint is a loopback port nothing serves and stdin
// is closed, so the run starts, reads its stores and exits without making a
// model request.
//
// PATH is added to the adapter's environment here and only here. Pi is a Node
// script whose interpreter is resolved through PATH, so a dispatcher will have
// to supply one; the adapter does not, and that is an open item for the
// confined live proof admission still requires rather than something this test
// should paper over silently.
func TestRealPiAcceptsTheProjectionAndWritesOnlyInsideIt(t *testing.T) {
	executable, err := exec.LookPath("pi")
	if err != nil {
		t.Skip("Pi is not installed: " + err.Error())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	reported, err := exec.CommandContext(ctx, executable, "--version").Output()
	if err != nil {
		t.Skip("Pi did not report a version: " + err.Error())
	}
	if strings.TrimSpace(string(reported)) != PiVersion {
		t.Skipf("installed Pi is %s, the adapter is pinned to %s", strings.TrimSpace(string(reported)), PiVersion)
	}

	state, box := piProjectionAt(t, "http://127.0.0.1:9/v1", []string{"127.0.0.1:9"})
	receipt, _ := testReceipt(t)
	adapterReceipt, err := receipt.ForAdapter(Pi, piAdapterVersion)
	if err != nil {
		t.Fatal(err)
	}
	packet, err := AssemblePacket(packetInput())
	if err != nil {
		t.Fatal(err)
	}
	cfg := piConfig(t)
	cfg.Executable = executable
	cfg.PiState = state
	invocation, err := preparePi(cfg, packet, adapterReceipt)
	if err != nil {
		t.Fatal(err)
	}

	enclosing := filepath.Dir(state.Dir())
	before := piTree(t, enclosing)

	command := exec.CommandContext(ctx, invocation.Path, invocation.Args...)
	command.Dir = box
	command.Env = append(append([]string(nil), invocation.Env...), "PATH="+os.Getenv("PATH"))
	command.Stdin = bytes.NewReader(nil)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("Pi refused the projection: %v: %s", err, output)
	}

	created := map[string]bool{}
	for path := range piTree(t, enclosing) {
		if before[path] {
			continue
		}
		rel, err := filepath.Rel(state.Dir(), path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			t.Fatalf("Pi wrote %s, which is outside the projection at %s", path, state.Dir())
		}
		created[rel] = true
	}
	// Pi writes its model cache while reading it, which is the write a
	// read-only projection cannot take and the reason this one is a copy.
	if !created["models-store.json"] {
		t.Fatalf("Pi created %v inside the projection, want its model store among them", created)
	}

	// The same projection made read-only is the arrangement that would look
	// safer and is not available. Pi creates a lock directory beside a file it
	// is only reading, so a read-only copy produces EACCES on the lock rather
	// than a working session with fewer writes. What it does with that failure
	// depends on which store reached it first — the settings path warns and
	// carries on, the auth path exits uncaught, and the auth reload path
	// swallows it into an empty credential set — so the assertion is on the
	// failure itself and not on an exit code.
	readOnly, readOnlyBox := piProjectionAt(t, "http://127.0.0.1:9/v1", []string{"127.0.0.1:9"})
	if err := os.Chmod(readOnly.Dir(), 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(readOnly.Dir(), 0o700) })
	refused := exec.CommandContext(ctx, invocation.Path, invocation.Args...)
	refused.Dir = readOnlyBox
	refused.Env = []string{"HOME=" + readOnly.Dir(), "PI_CODING_AGENT_DIR=" + readOnly.Dir(), "PATH=" + os.Getenv("PATH")}
	refused.Stdin = bytes.NewReader(nil)
	output, _ = refused.CombinedOutput()
	if !strings.Contains(string(output), "EACCES") || !strings.Contains(string(output), ".lock") {
		t.Fatalf("a read-only projection did not fail on a Pi lock, so the copy would not have to be writable: %s", output)
	}
	if !strings.Contains(string(output), readOnly.Dir()) {
		t.Fatalf("Pi's lock failure names a path outside the projection: %s", output)
	}
}

// Pi's RPC loop ends at input EOF, so who closes the channel and when is the
// difference between a session that ran and a session that reported a clean
// exit after the user message and nothing else. Measured against the installed
// Pi before this was written; this is the property that keeps it true.
func TestPiEndsItsInputChannelOnItsOwnSettledEventAndNotBefore(t *testing.T) {
	invocation, _, _ := preparePiFixture(t)
	channel := &countedInput{}
	if err := invocation.BindInput(channel); err != nil {
		t.Fatal(err)
	}
	if channel.Len() == 0 {
		t.Fatal("binding the Pi input channel wrote no prompt")
	}
	if err := invocation.BindInput(channel); err == nil || !strings.Contains(err.Error(), "bound more than once") {
		t.Fatalf("Pi bound its input channel twice: %v", err)
	}

	// Everything up to and including the end of the last turn. Pi may still
	// retry, compact or dequeue after any of these, so none of them ends a
	// session.
	src, err := os.ReadFile(filepath.Join("testdata", "pi", "stream.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	settled := []byte("{\"type\":\"agent_settled\"}\n")
	before, found := bytes.CutSuffix(src, settled)
	if !found {
		t.Fatal("the Pi native example stream no longer ends in agent_settled, so this measures nothing")
	}
	if _, err := invocation.Output.Write(before); err != nil {
		t.Fatal(err)
	}
	if channel.closes != 0 {
		t.Fatalf("Pi ended its session after %d events, before agent_settled", bytes.Count(before, []byte{'\n'}))
	}

	// And the settled event, split across two writes, because a stream arrives
	// in whatever chunks the pipe delivers.
	if _, err := invocation.Output.Write(settled[:6]); err != nil {
		t.Fatal(err)
	}
	if channel.closes != 0 {
		t.Fatal("Pi ended its session on a partial line")
	}
	if _, err := invocation.Output.Write(settled[6:]); err != nil {
		t.Fatal(err)
	}
	if channel.closes != 1 {
		t.Fatalf("Pi closed its input channel %d times on agent_settled, want exactly one", channel.closes)
	}
	// The reaped child closes it too, and neither knows about the other.
	if err := invocation.Output.Close(); err != nil {
		t.Fatal(err)
	}
	if channel.closes != 1 {
		t.Fatalf("Pi closed its input channel %d times in all, want exactly one", channel.closes)
	}
}

// TestPiRefusesEveryParentEnvironmentHazard walks the family through the real
// preflight. Pi's child environment is three names and Invocation.Env replaces
// rather than adds, so this guards the one shape that construction cannot see:
// a confinement provider that merged the parent environment into the child.
func TestPiRefusesEveryParentEnvironmentHazard(t *testing.T) {
	receipt, _ := testReceipt(t)
	adapter, err := receipt.ForAdapter(Pi, piAdapterVersion)
	if err != nil {
		t.Fatal(err)
	}
	for name, hazard := range piRefusedEnvironment {
		t.Run(name, func(t *testing.T) {
			cfg := piConfig(t)
			packet, err := AssemblePacket(packetInput())
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv(name, "hostile-value-that-must-never-be-quoted")
			_, err = preparePi(cfg, packet, adapter)
			if err == nil || !strings.Contains(err.Error(), name+" is present in the parent environment") {
				t.Fatalf("%s produced %v, want a refusal naming it", name, err)
			}
			if !strings.Contains(err.Error(), hazard) {
				t.Fatalf("%s refusal = %v, want the hazard %q", name, err, hazard)
			}
			// A refusal that quoted the value would put a credential in a
			// disposition. Names only, always.
			if strings.Contains(err.Error(), "hostile-value-that-must-never-be-quoted") {
				t.Fatalf("%s refusal disclosed the variable's value: %v", name, err)
			}
		})
	}
}

// TestPiCredentialFamilyMembershipIsPinned is what makes the walk above mean
// something. A table-driven test that reads its own table cannot notice an
// entry being deleted — it simply tests one name fewer and still passes. So the
// membership is pinned here literally, and dropping a name from the family is a
// failing test rather than a silent widening of what may reach the child.
// Adding one is deliberately free: this list is a floor, not an inventory.
//
// Every name's evidence is in the comment beside it in pi.go, sourced to the
// installed Pi 0.84.0 package — its bundled docs, its shipped dist, or a probe
// measured against it. A name nothing there sources does not belong here.
func TestPiCredentialFamilyMembershipIsPinned(t *testing.T) {
	for name, hazard := range map[string]string{
		"KEPT_PI_CREDENTIAL":  piOverridesCredential,
		"LLAMA_API_KEY":       piOverridesCredential,
		"PI_CODING_AGENT_DIR": piNamesConfiguration,
		"http_proxy":          piRedirectsUpstream,
		"HTTP_PROXY":          piRedirectsUpstream,
		"https_proxy":         piRedirectsUpstream,
		"HTTPS_PROXY":         piRedirectsUpstream,
		"NODE_EXTRA_CA_CERTS": piRedirectsUpstream,
		"LLAMA_BASE_URL":      piRedirectsUpstream,
		"NODE_OPTIONS":        piRunsCodeInProcess,
	} {
		got, present := piRefusedEnvironment[name]
		if !present {
			t.Errorf("%s left the refused-credential family", name)
			continue
		}
		if got != hazard {
			t.Errorf("%s is classified %q, want %q", name, got, hazard)
		}
	}
	// The name kept owns is the one the whole credential channel rests on, and
	// it is pinned to the projection's own constant rather than to a literal so
	// the two cannot drift into naming different variables.
	if _, present := piRefusedEnvironment[runtimestate.PiCredentialEnvironment]; !present {
		t.Errorf("%s is not refused in the parent environment", runtimestate.PiCredentialEnvironment)
	}
}

// TestPiAdmitsTheDeliberatelyExcludedNames is the other half of the cut. A
// guard that fires on an ordinary developer machine gets switched off, so the
// exclusions are load-bearing and have to be as tested as the refusals.
//
// Two of these groups are why this list was read off Pi rather than adapted
// from the Claude adapter's. ALL_PROXY and all_proxy are refused there and are
// admitted here, because the undici dispatcher Pi installs globally never reads
// them — copying them across would have shipped two names guarding nothing. And
// there is no PI_ namespace rule to match the ANTHROPIC_ one, because a live Pi
// session exports PI_CODING_AGENT to every child and its bash tool exports five
// more, so a namespace refusal would refuse every dispatch made from inside one.
func TestPiAdmitsTheDeliberatelyExcludedNames(t *testing.T) {
	receipt, _ := testReceipt(t)
	adapter, err := receipt.ForAdapter(Pi, piAdapterVersion)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		// The provider-native key table: auth.json wins over every one of
		// them, and --provider pins which provider is asked at all.
		"ANTHROPIC_API_KEY",
		"ANTHROPIC_AUTH_TOKEN",
		"ANTHROPIC_OAUTH_TOKEN",
		"OPENAI_API_KEY",
		"OPENROUTER_API_KEY",
		"COPILOT_GITHUB_TOKEN",
		// The third-party stores behind the cloud providers.
		"AWS_ACCESS_KEY_ID",
		"AWS_BEARER_TOKEN_BEDROCK",
		"AWS_PROFILE",
		"AZURE_OPENAI_API_KEY",
		"GOOGLE_APPLICATION_CREDENTIALS",
		// Proxy names Pi's dispatcher does not read, and one that cannot send
		// anything anywhere.
		"ALL_PROXY",
		"all_proxy",
		"NO_PROXY",
		"no_proxy",
		// Pi's own process configuration, none of which can move a credential.
		"PI_OFFLINE",
		"PI_TELEMETRY",
		"PI_PACKAGE_DIR",
		"PI_CODING_AGENT_SESSION_DIR",
		"PI_SKIP_VERSION_CHECK",
		"PI_CACHE_RETENTION",
		// The ambient names a running Pi session exports to its children.
		"PI_CODING_AGENT",
		"PI_SESSION_ID",
		"PI_SESSION_FILE",
		"PI_PROVIDER",
		"PI_MODEL",
		"PI_REASONING_LEVEL",
	} {
		t.Run(name, func(t *testing.T) {
			cfg := piConfig(t)
			packet, err := AssemblePacket(packetInput())
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv(name, "present")
			if _, err := preparePi(cfg, packet, adapter); err != nil {
				t.Fatalf("%s refused dispatch: %v; it is excluded on purpose", name, err)
			}
		})
	}
}

// TestPiRefusalNamesTheSameOffenderEveryRun keeps the refusal off the
// environment's ordering. Two hostile names must not produce a message that
// changes between runs, because a disposition is a record somebody compares.
func TestPiRefusalNamesTheSameOffenderEveryRun(t *testing.T) {
	environment := []string{
		"PATH=/usr/bin",
		"NODE_OPTIONS=--require /tmp/hostile.cjs",
		"HTTPS_PROXY=http://elsewhere",
		runtimestate.PiCredentialEnvironment + "=somebody-elses-key",
	}
	name, hazard, refused := piRefusedParentEnvironment(environment)
	if !refused || name != "HTTPS_PROXY" || hazard != piRedirectsUpstream {
		t.Fatalf("refusal = %q/%q/%v, want the sorted-first offender", name, hazard, refused)
	}
	reversed := []string{environment[3], environment[2], environment[1], environment[0]}
	again, againHazard, _ := piRefusedParentEnvironment(reversed)
	if again != name || againHazard != hazard {
		t.Fatalf("reordering the environment changed the offender: %q then %q", name, again)
	}
	// A benign environment, including the two names kept sets itself for the
	// child and the ambient marker a running Pi exports to every child of its
	// own, is not an offender.
	if _, _, refused := piRefusedParentEnvironment([]string{
		"PATH=/usr/bin",
		"HOME=/home/x",
		"TMPDIR=/tmp",
		"PI_CODING_AGENT=true",
		"PI_PROVIDER=workshop",
	}); refused {
		t.Fatal("a clean environment was refused")
	}
	// An entry with no separator is not a name, and must not be read as one.
	if _, _, refused := piRefusedParentEnvironment([]string{"NODE_OPTIONS"}); refused {
		t.Fatal("a malformed environment entry was read as an exported name")
	}
}
