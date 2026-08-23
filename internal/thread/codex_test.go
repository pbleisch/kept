package thread

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/ledger"
)

type codexFixture struct {
	Runtime               string   `json:"runtime"`
	CLIVersion            string   `json:"cli_version"`
	SchemaSHA256          string   `json:"app_server_schema_sha256"`
	DirectArgv            []string `json:"direct_argv"`
	CredentialEnvironment string   `json:"credential_environment"`
	ChildEnvironment      string   `json:"child_environment"`
	NativeExamples        string   `json:"native_examples"`
	Admission             string   `json:"admission"`
}

func loadCodexFixture(t *testing.T) codexFixture {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("testdata", "codex", "fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture codexFixture
	if err := json.Unmarshal(src, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func codexConfig(t *testing.T) RuntimeConfig {
	t.Helper()
	executable := filepath.Join(t.TempDir(), "codex")
	if err := os.WriteFile(executable, []byte("fixture executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	return RuntimeConfig{
		Runtime:           string(Codex),
		Executable:        executable,
		Version:           CodexVersion,
		CodexSchemaSHA256: CodexAppServerSchemaSHA256,
		CodexAccessToken:  "test-codex-access-token",
		CodexHome:         t.TempDir(),
		CodexCWD:          t.TempDir(),
	}
}

func prepareCodexFixture(t *testing.T) (Invocation, *Receipt, ledger.Store) {
	t.Helper()
	receipt, store := testReceipt(t)
	adapter, err := receipt.ForAdapter(Codex, codexAdapterVersion)
	if err != nil {
		t.Fatal(err)
	}
	packet, err := AssemblePacket(packetInput())
	if err != nil {
		t.Fatal(err)
	}
	invocation, err := prepareCodex(codexConfig(t), packet, adapter)
	if err != nil {
		t.Fatal(err)
	}
	return invocation, receipt, store
}

func TestCodexFixturePinsTheDirectAppServerContract(t *testing.T) {
	fixture := loadCodexFixture(t)
	if fixture.Runtime != string(Codex) || fixture.CLIVersion != CodexVersion {
		t.Fatalf("fixture runtime/version = %q/%q, want codex/%s", fixture.Runtime, fixture.CLIVersion, CodexVersion)
	}
	if fixture.SchemaSHA256 != CodexAppServerSchemaSHA256 {
		t.Fatalf("fixture schema = %q, want %q", fixture.SchemaSHA256, CodexAppServerSchemaSHA256)
	}
	if !reflect.DeepEqual(fixture.DirectArgv, codexArgs) {
		t.Fatalf("fixture argv = %#v, adapter argv = %#v", fixture.DirectArgv, codexArgs)
	}
	if fixture.CredentialEnvironment != "CODEX_ACCESS_TOKEN" || !strings.Contains(fixture.ChildEnvironment, "exactly CODEX_ACCESS_TOKEN") {
		t.Fatalf("fixture credential contract = %q / %q", fixture.CredentialEnvironment, fixture.ChildEnvironment)
	}
	if fixture.NativeExamples == "" || !strings.Contains(fixture.Admission, "unadmitted") {
		t.Fatalf("fixture omits native examples or falsely claims admission: %#v", fixture)
	}
}

func TestCodexPreparationUsesTheOnlyDocumentedAppServerSequence(t *testing.T) {
	cfg := codexConfig(t)
	packet, err := AssemblePacket(packetInput())
	if err != nil {
		t.Fatal(err)
	}
	// Rebuild with one configuration so comparisons do not accidentally pass
	// against independently-created temporary paths.
	receipt, _ := testReceipt(t)
	adapter, err := receipt.ForAdapter(Codex, codexAdapterVersion)
	if err != nil {
		t.Fatal(err)
	}
	invocation, err := prepareCodex(cfg, packet, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if invocation.Path != cfg.Executable || !reflect.DeepEqual(invocation.Args, codexArgs) {
		t.Fatalf("Codex invocation = %#v, want configured path and fixed app-server argv", invocation)
	}
	if invocation.Output == nil || invocation.BindInput == nil {
		t.Fatal("Codex invocation did not provide its app-server input/output session")
	}
	if got, want := invocation.Env, []string{"CODEX_HOME=" + cfg.CodexHome, "CODEX_ACCESS_TOKEN=" + cfg.CodexAccessToken}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Codex environment = %#v, want clean state plus only its access token", got)
	}
	for _, arg := range invocation.Args {
		if strings.Contains(arg, cfg.CodexAccessToken) || strings.Contains(arg, "exec") {
			t.Fatalf("Codex argv leaked a credential or chose exec: %#v", invocation.Args)
		}
	}
	if bytes.Contains(invocation.Input, []byte(cfg.CodexAccessToken)) || bytes.Contains(invocation.Input, []byte("turn/start")) {
		t.Fatalf("Codex initial input leaked a credential or sent turn/start before thread/start: %s", invocation.Input)
	}
	requests := codexRequests(t, invocation.Input)
	if len(requests) != 3 || requests[0].Method != "initialize" || requests[1].Method != "initialized" || requests[2].Method != "thread/start" {
		t.Fatalf("Codex initial JSONL = %#v, want initialize, initialized, thread/start", requests)
	}
	if requests[0].ID == "" || requests[1].ID != "" || requests[2].ID == "" {
		t.Fatalf("Codex initial request ids are not JSON-RPC request/notification ids: %#v", requests)
	}
	var start struct {
		CWD            string `json:"cwd"`
		ApprovalPolicy string `json:"approvalPolicy"`
		Sandbox        string `json:"sandbox"`
		Ephemeral      bool   `json:"ephemeral"`
	}
	if err := json.Unmarshal(requests[2].Params, &start); err != nil {
		t.Fatal(err)
	}
	if start.CWD != cfg.CodexCWD || start.ApprovalPolicy != "never" || start.Sandbox != "workspace-write" || !start.Ephemeral {
		t.Fatalf("thread/start params = %#v", start)
	}

	var stdin bytes.Buffer
	if err := invocation.BindInput(heldInput{&stdin}); err != nil {
		t.Fatal(err)
	}
	if _, err := invocation.Output.Write(codexResult(requests[2].ID, `{"thread":{"id":"thread-fixture"}}`)); err != nil {
		t.Fatal(err)
	}
	turn := codexRequests(t, stdin.Bytes())
	if len(turn) != 1 || turn[0].ID == "" || turn[0].Method != "turn/start" {
		t.Fatalf("Codex thread/start did not produce one turn/start request: %#v", turn)
	}
	var turnParams struct {
		ThreadID string `json:"threadId"`
		Input    []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"input"`
		CWD            string `json:"cwd"`
		ApprovalPolicy string `json:"approvalPolicy"`
		SandboxPolicy  struct {
			Type          string `json:"type"`
			NetworkAccess bool   `json:"networkAccess"`
		} `json:"sandboxPolicy"`
	}
	if err := json.Unmarshal(turn[0].Params, &turnParams); err != nil {
		t.Fatal(err)
	}
	if turnParams.ThreadID != "thread-fixture" || len(turnParams.Input) != 1 || turnParams.Input[0].Type != "text" || turnParams.Input[0].Text != string(packet.Bytes()) || turnParams.CWD != cfg.CodexCWD || turnParams.ApprovalPolicy != "never" || turnParams.SandboxPolicy.Type != "workspaceWrite" || turnParams.SandboxPolicy.NetworkAccess {
		t.Fatalf("turn/start changed the packet or protocol policy: %#v", turnParams)
	}
}

func TestCodexPreparationRefusesEveryAdmissionPrecondition(t *testing.T) {
	receipt, _ := testReceipt(t)
	adapter, err := receipt.ForAdapter(Codex, codexAdapterVersion)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		edit func(t *testing.T, cfg *RuntimeConfig, in *PacketInput)
		want string
	}{
		{"wrong runtime", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) { cfg.Runtime = string(Pi) }, "does not name codex"},
		{"no executable", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) { cfg.Executable = "" }, "executable is unset"},
		{"relative executable", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) { cfg.Executable = "codex" }, "explicit absolute path"},
		{"missing executable", func(t *testing.T, cfg *RuntimeConfig, _ *PacketInput) {
			cfg.Executable = filepath.Join(t.TempDir(), "missing-codex")
		}, "executable is unavailable"},
		{"nonexecutable file", func(t *testing.T, cfg *RuntimeConfig, _ *PacketInput) {
			cfg.Executable = filepath.Join(t.TempDir(), "not-executable")
			if err := os.WriteFile(cfg.Executable, []byte("fixture"), 0o600); err != nil {
				t.Fatal(err)
			}
		}, "executable is not executable"},
		{"no version", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) { cfg.Version = "" }, "version is unset"},
		{"wrong version", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) { cfg.Version = "0.149.1" }, "version is unsupported"},
		{"no schema", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) { cfg.CodexSchemaSHA256 = "" }, "schema digest is unset"},
		{"schema drift", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) { cfg.CodexSchemaSHA256 = "changed" }, "schema digest is unsupported"},
		{"no packet", func(_ *testing.T, _ *RuntimeConfig, in *PacketInput) { *in = PacketInput{} }, "packet is absent"},
		{"no credential", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) { cfg.CodexAccessToken = "" }, "CODEX_ACCESS_TOKEN is unset"},
		{"NUL credential", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) { cfg.CodexAccessToken = "test\x00token" }, "CODEX_ACCESS_TOKEN contains NUL"},
		{"credential in packet", func(_ *testing.T, cfg *RuntimeConfig, in *PacketInput) {
			cfg.CodexAccessToken = "test\"token"
			in.Why = cfg.CodexAccessToken
		}, "appears in the packet"},
		{"no CODEX_HOME", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) { cfg.CodexHome = "" }, "CODEX_HOME is unset"},
		{"relative CODEX_HOME", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) { cfg.CodexHome = "home" }, "CODEX_HOME is not an explicit absolute path"},
		{"missing CODEX_HOME", func(t *testing.T, cfg *RuntimeConfig, _ *PacketInput) {
			cfg.CodexHome = filepath.Join(t.TempDir(), "missing-home")
		}, "CODEX_HOME is unavailable"},
		{"CODEX_HOME file", func(t *testing.T, cfg *RuntimeConfig, _ *PacketInput) {
			cfg.CodexHome = filepath.Join(t.TempDir(), "not-a-home")
			if err := os.WriteFile(cfg.CodexHome, []byte("fixture"), 0o600); err != nil {
				t.Fatal(err)
			}
		}, "CODEX_HOME is not a directory"},
		{"dirty CODEX_HOME", func(t *testing.T, cfg *RuntimeConfig, _ *PacketInput) {
			if err := os.WriteFile(filepath.Join(cfg.CodexHome, "auth.json"), []byte("do not reuse"), 0o600); err != nil {
				t.Fatal(err)
			}
		}, "CODEX_HOME is not clean"},
		{"no thread cwd", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) { cfg.CodexCWD = "" }, "thread cwd is unset"},
		{"relative thread cwd", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) { cfg.CodexCWD = "box" }, "thread cwd is not an explicit absolute path"},
		{"missing thread cwd", func(t *testing.T, cfg *RuntimeConfig, _ *PacketInput) {
			cfg.CodexCWD = filepath.Join(t.TempDir(), "missing-box")
		}, "thread cwd is unavailable"},
		{"thread cwd file", func(t *testing.T, cfg *RuntimeConfig, _ *PacketInput) {
			cfg.CodexCWD = filepath.Join(t.TempDir(), "not-a-box")
			if err := os.WriteFile(cfg.CodexCWD, []byte("fixture"), 0o600); err != nil {
				t.Fatal(err)
			}
		}, "thread cwd is not a directory"},
		{"state and box overlap", func(_ *testing.T, cfg *RuntimeConfig, _ *PacketInput) { cfg.CodexCWD = cfg.CodexHome }, "overlaps the Codex thread cwd"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := codexConfig(t)
			in := packetInput()
			tc.edit(t, &cfg, &in)
			var packet Packet
			if in.Promise.ID != "" {
				var err error
				packet, err = AssemblePacket(in)
				if err != nil {
					t.Fatal(err)
				}
			}
			if _, err := prepareCodex(cfg, packet, adapter); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("prepareCodex(%+v) = %v, want refusal %q", cfg, err, tc.want)
			}
		})
	}
	packet, err := AssemblePacket(packetInput())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := prepareCodex(codexConfig(t), packet, nil); err == nil || !strings.Contains(err.Error(), "adapter receipt is absent") {
		t.Fatalf("prepareCodex without receipt = %v, want refusal", err)
	}
}

func TestCodexFixtureRetainsNativeAppServerEvidenceAndOnlyNativeBackedProjections(t *testing.T) {
	invocation, receipt, store := prepareCodexFixture(t)
	requests := codexRequests(t, invocation.Input)
	var stdin bytes.Buffer
	if err := invocation.BindInput(heldInput{&stdin}); err != nil {
		t.Fatal(err)
	}
	if _, err := invocation.Output.Write(codexResult(requests[0].ID, `{}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := invocation.Output.Write(codexResult(requests[2].ID, `{"thread":{"id":"thread-fixture"}}`)); err != nil {
		t.Fatal(err)
	}
	turn := codexRequests(t, stdin.Bytes())
	if _, err := invocation.Output.Write(codexResult(turn[0].ID, `{"turn":{"id":"turn-fixture"}}`)); err != nil {
		t.Fatal(err)
	}
	fixture := loadCodexFixture(t)
	src, err := os.ReadFile(filepath.Join("testdata", "codex", fixture.NativeExamples))
	if err != nil {
		t.Fatal(err)
	}
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
		t.Fatalf("fixture receipt quarantined evidence: %#v", quarantined)
	}
	if got, want := ReceiptText(entries), "thread_native,thread_native,thread_native,thread_native,thread_native,thread_turn,thread_native,thread_transcript,thread_native,thread_native"; got != want {
		t.Fatalf("Codex receipt kinds = %q, want %q", got, want)
	}
	if entries[4].Native.Type != "turn/started" || entries[5].Turn.NativeRef != entries[4].Native.Ref || entries[5].Turn.Count != 1 {
		t.Fatalf("turn projection is not backed by the native turn notification: %#v", entries[4:6])
	}
	if entries[6].Native.Type != "item/agentMessage/delta" || entries[7].Transcript.NativeRef != entries[6].Native.Ref {
		t.Fatalf("transcript projection is not backed by the native delta: %#v", entries[6:8])
	}
	transcript, err := ledger.ReadArtifact(store, entries[7].Transcript.Artifact)
	if err != nil {
		t.Fatal(err)
	}
	if string(transcript) != "partial repair evidence" {
		t.Fatalf("Codex transcript = %q", transcript)
	}
	if entries[8].Native.Type != "thread/tokenUsage/updated" || entries[9].Native.Type != "turn/completed" {
		t.Fatalf("Codex token/turn evidence was not retained in native vocabulary: %#v", entries[8:])
	}
}

func TestCodexStreamRedactsItsOnlyCredentialBeforeRetention(t *testing.T) {
	receipt, store := testReceipt(t)
	adapter, err := receipt.ForAdapter(Codex, codexAdapterVersion)
	if err != nil {
		t.Fatal(err)
	}
	cfg := codexConfig(t)
	cfg.CodexAccessToken = "test\"codex-token"
	packet, err := AssemblePacket(packetInput())
	if err != nil {
		t.Fatal(err)
	}
	invocation, err := prepareCodex(cfg, packet, adapter)
	if err != nil {
		t.Fatal(err)
	}
	src, err := json.Marshal(map[string]any{
		"method": "item/agentMessage/delta",
		"params": map[string]any{
			"threadId": "thread-fixture", "turnId": "turn-fixture", "itemId": "item-fixture", "delta": cfg.CodexAccessToken,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := invocation.Output.Write(append(src, '\n')); err != nil {
		t.Fatal(err)
	}
	if err := invocation.Output.Close(); err == nil || !strings.Contains(err.Error(), "token usage") {
		t.Fatalf("close without all pre-exit evidence = %v, want refusal", err)
	}
	entries, quarantined, err := ReadReceipt(store, receipt.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(quarantined) != 0 || len(entries) != 2 {
		t.Fatalf("redacted Codex receipt = %#v, quarantine = %#v", entries, quarantined)
	}
	if entries[0].Native.Redaction.Matches != 1 || entries[1].Transcript.Redaction.Matches != 1 {
		t.Fatalf("Codex redactions = %#v %#v", entries[0].Native.Redaction, entries[1].Transcript.Redaction)
	}
	durable, err := ledger.ReadArtifact(store, entries[1].Transcript.Artifact)
	if err != nil {
		t.Fatal(err)
	}
	for _, retained := range [][]byte{entries[0].Native.Payload, durable} {
		if codexPacketContains(cfg.CodexAccessToken, retained) || !bytes.Contains(retained, []byte("[REDACTED]")) {
			t.Fatalf("Codex credential escaped durable redaction: %q", retained)
		}
	}
}

func TestCodexStreamRefusesMalformedJSONRPCWithoutInventingEvidence(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"invalid JSON", `{"method":`, "decode Codex app-server JSON-RPC"},
		{"JSON-RPC header", `{"jsonrpc":"2.0","method":"turn/started","params":{}}`, "must omit the jsonrpc header"},
		{"batch", `[]`, "decode Codex app-server JSON-RPC"},
		{"notification without params", `{"method":"turn/started"}`, "has no object params"},
		{"ambiguous response", `{"id":"id","result":{},"error":{}}`, "neither one notification nor one response"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invocation, receipt, store := prepareCodexFixture(t)
			if _, err := invocation.Output.Write([]byte(tc.src + "\n")); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("write error = %v, want refusal %q", err, tc.want)
			}
			entries, _, err := ReadReceipt(store, receipt.ID())
			if err != nil && !strings.Contains(err.Error(), "no such ledger stream") {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatalf("malformed Codex JSON-RPC made durable receipt entries: %#v", entries)
			}
		})
	}
}

func TestCodexStreamRefusesMissingPreExitEvidence(t *testing.T) {
	invocation, _, _ := prepareCodexFixture(t)
	if _, err := invocation.Output.Write([]byte(`{"method":"turn/started","params":{"threadId":"thread-fixture","turn":{"id":"turn-fixture"}}}` + "\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := invocation.Output.Write([]byte(`{"method":"item/agentMessage/delta","params":{"threadId":"thread-fixture","turnId":"turn-fixture","itemId":"item-fixture","delta":"only a transcript"}}` + "\n")); err != nil {
		t.Fatal(err)
	}
	if err := invocation.Output.Close(); err == nil || !strings.Contains(err.Error(), "token usage") {
		t.Fatalf("Codex stream ended without token evidence = %v, want refusal", err)
	}
}

func TestCodexSupervisorRetainsPartialNativeEvidenceBeforeItsParentDeadlineLifecycle(t *testing.T) {
	invocation, receipt, store := prepareCodexFixture(t)
	requests := codexRequests(t, invocation.Input)
	var stdin bytes.Buffer
	if err := invocation.BindInput(heldInput{&stdin}); err != nil {
		t.Fatal(err)
	}
	if _, err := invocation.Output.Write(codexResult(requests[2].ID, `{"thread":{"id":"thread-fixture"}}`)); err != nil {
		t.Fatal(err)
	}
	stream := []byte(
		`{"method":"turn/started","params":{"threadId":"thread-fixture","turn":{"id":"turn-fixture"}}}` + "\n" +
			`{"method":"item/agentMessage/delta","params":{"threadId":"thread-fixture","turnId":"turn-fixture","itemId":"item-fixture","delta":"before deadline"}}` + "\n" +
			`{"method":"thread/tokenUsage/updated","params":{"threadId":"thread-fixture","turnId":"turn-fixture","tokenUsage":{"last":{},"total":{}}}}` + "\n",
	)
	if _, err := invocation.Output.Write(stream); err != nil {
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
	if len(quarantined) != 0 {
		t.Fatalf("Codex interruption quarantine = %#v", quarantined)
	}
	if got, want := ReceiptText(entries), "thread_native,thread_native,thread_turn,thread_native,thread_transcript,thread_native,thread_lifecycle,thread_lifecycle,thread_lifecycle,thread_lifecycle"; got != want {
		t.Fatalf("Codex interruption receipt = %q, want %q", got, want)
	}
	if entries[0].Native.Type != "response" || entries[3].Native.Type != "item/agentMessage/delta" || entries[6].Lifecycle.Event != Started || entries[7].Lifecycle.Event != TimedOut || entries[8].Lifecycle.Event != Killed || entries[9].Lifecycle.Event != Exited || entries[9].Lifecycle.Exit.Signal != "SIGKILL" {
		t.Fatalf("Codex interruption lost native or parent lifecycle evidence: %#v", entries)
	}
	if child.killCall != 1 {
		t.Fatalf("Codex deadline killed %d child groups, want one", child.killCall)
	}
}

func TestCodexAdapterRemainsUnadmittedWithoutTheLiveProof(t *testing.T) {
	adapter, err := Select(string(Codex))
	if err != nil {
		t.Fatal(err)
	}
	packet, err := AssemblePacket(packetInput())
	if err != nil {
		t.Fatal(err)
	}
	receipt, _ := testReceipt(t)
	adapterReceipt, err := receipt.ForAdapter(Codex, adapter.AdapterVersion())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Prepare(codexConfig(t), packet, adapterReceipt); err == nil || !strings.Contains(err.Error(), "unadmitted") {
		t.Fatalf("Codex adapter launched without its credentialed confined interrupted proof: %v", err)
	}
	launch := &fakeLaunch{verified: errors.New("should not be reached"), child: normalChild(0)}
	if err := Dispatch(context.Background(), codexConfig(t), packet, launch, Supervisor{Timeout: time.Second}, receipt); err == nil || !strings.Contains(err.Error(), "canary failed") {
		t.Fatalf("Codex dispatch did not refuse before runtime preparation: %v", err)
	}
	if launch.started != 0 {
		t.Fatalf("unadmitted Codex launched %d children", launch.started)
	}
}

type codexWireRequest struct {
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

func codexRequests(t *testing.T, src []byte) []codexWireRequest {
	t.Helper()
	if !bytes.HasSuffix(src, []byte{'\n'}) || bytes.Contains(src, []byte{'\r'}) {
		t.Fatalf("Codex input is not strict-LF JSONL: %q", src)
	}
	var requests []codexWireRequest
	for _, line := range bytes.Split(bytes.TrimSuffix(src, []byte{'\n'}), []byte{'\n'}) {
		var request codexWireRequest
		if err := json.Unmarshal(line, &request); err != nil {
			t.Fatalf("decode Codex input %q: %v", line, err)
		}
		requests = append(requests, request)
	}
	return requests
}

func codexResult(id, result string) []byte {
	return []byte(fmt.Sprintf(`{"id":%q,"result":%s}`+"\n", id, result))
}
