package thread

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
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
	RefusedEnvironment    string   `json:"refused_parent_environment"`
	NativeExamples        string   `json:"native_examples"`
	NativeProvenance      string   `json:"native_examples_provenance"`
	CapturedSession       string   `json:"captured_session"`
	CapturedProvenance    string   `json:"captured_session_provenance"`
	CapturedWindow        string   `json:"captured_window"`
	CapturedWindowFrom    string   `json:"captured_window_provenance"`
	CapturedSynthetic     string   `json:"captured_synthetic"`
	CapturedSyntheticFrom string   `json:"captured_synthetic_provenance"`
	Distribution          string   `json:"distribution"`
	ArgvMeasured          string   `json:"argv_measured"`
	TemporaryDirectory    string   `json:"temporary_directory"`
	Network               string   `json:"network"`
	CredentialMeasured    string   `json:"credential_channel_measured"`
	LiveSessions          string   `json:"live_sessions_measured"`
	Admission             string   `json:"admission"`

	// Brokered is the other credential channel decision 0052 permits, and the
	// one every live confined session ran on. A broker route has its own
	// recording requirements there — the allowed upstream, the method and path
	// policy, and proof that the child cannot read the real credential — so it
	// is a shape rather than a sentence.
	Brokered struct {
		Channel    string   `json:"channel"`
		Service    string   `json:"service"`
		Upstream   string   `json:"upstream"`
		Reference  string   `json:"reference"`
		Phantom    string   `json:"phantom_environment"`
		Header     string   `json:"header"`
		Endpoints  []string `json:"endpoints"`
		Policy     string   `json:"endpoint_policy"`
		CannotRead string   `json:"child_cannot_read_the_credential"`
		Network    string   `json:"network"`
	} `json:"brokered_credential"`

	// Ask is the channel decision 0058 added beside the pinned argv above, and
	// the closed set of values it may carry. The words themselves are not here:
	// they are a reviewed constant in the adapter, and a second copy of them in
	// this file would be a second place one rule lived.
	Ask struct {
		Channel        string   `json:"channel"`
		Values         []string `json:"values"`
		Why            string   `json:"why"`
		Measured       string   `json:"measured"`
		NotInThePacket string   `json:"not_in_the_packet"`
	} `json:"ask"`
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

// claudeCleanParentEnvironment removes every name the adapter refuses from the
// test process. Whether a developer's shell exports HTTPS_PROXY or a Claude
// session exported something is not what most of these tests are about, and a
// suite that passed only on a bare machine would be worse than no suite.
// t.Setenv records the prior state, so the cleanup restores it either way.
func claudeCleanParentEnvironment(t *testing.T) {
	t.Helper()
	for _, entry := range os.Environ() {
		name, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		_, refused := claudeRefusedEnvironment[name]
		if !refused && !strings.HasPrefix(name, claudeRefusedNamespace) {
			continue
		}
		t.Setenv(name, "cleared by the test harness")
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
	}
}

// claudeConfig is the safe path: a token with life left in it, an empty session
// home and a scratch directory of its own, in a parent environment carrying
// nothing the adapter refuses.
func claudeConfig(t *testing.T) RuntimeConfig {
	t.Helper()
	claudeCleanParentEnvironment(t)
	executable := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(executable, []byte("fixture executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	return RuntimeConfig{
		Runtime:                string(Claude),
		Executable:             executable,
		Version:                ClaudeVersion,
		ClaudeSetupToken:       "test-claude-setup-token",
		ClaudeSetupTokenExpiry: time.Now().Add(365 * 24 * time.Hour),
		ClaudeHome:             t.TempDir(),
		ClaudeTemp:             t.TempDir(),
		ClaudeAsk:              ClaudeRepairAsk,
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
	// The fixture was captured against one build, and the question is whether
	// that build is still one this adapter has reviewed — not whether it is the
	// newest of them. Pinning it to the newest would make every entry added to
	// the reviewed set demand a re-captured fixture for a stream shape the new
	// entry was admitted for having left alone.
	if fixture.Runtime != string(Claude) || !ClaudeSupports(fixture.CLIVersion) {
		t.Fatalf("fixture runtime/version = %q/%q, want claude on a reviewed version", fixture.Runtime, fixture.CLIVersion)
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
	// The credential channel is only as good as what is kept away from it, so
	// the fixture records the refused family as part of the contract.
	if !strings.Contains(fixture.RefusedEnvironment, claudeRefusedNamespace) || !strings.Contains(fixture.RefusedEnvironment, "never values") {
		t.Fatalf("fixture omits the refused parent environment: %q", fixture.RefusedEnvironment)
	}
	if fixture.NativeExamples == "" {
		t.Fatalf("fixture omits its native examples: %#v", fixture)
	}
	// Decision 0052 asks a *broker* route for three things the direct channel
	// has no equivalent of: which upstream the credential may be spent against,
	// which methods and paths the route forwards, and evidence that the child
	// cannot read the real value. Every live confined session ran on this
	// channel, so the fixture records it rather than only the direct one.
	// The upstream and the endpoint list are internal/repair's to decide, so
	// that package's own test is what holds the fixture to them; what this one
	// checks is the half thread owns and that the record is not empty.
	broker := fixture.Brokered
	if broker.Upstream == "" || broker.Phantom != claudeCredentialEnvironment {
		t.Fatalf("fixture broker route names upstream %q and phantom %q", broker.Upstream, broker.Phantom)
	}
	if len(broker.Endpoints) == 0 || broker.Policy == "" || !strings.Contains(broker.Policy, "403") {
		t.Fatalf("fixture broker route records no measured method-and-path policy: %#v", broker)
	}
	if !strings.Contains(broker.CannotRead, "phantom") || !strings.Contains(broker.CannotRead, "401") {
		t.Fatalf("fixture broker route does not record that the child cannot read the credential: %q", broker.CannotRead)
	}
	// And the record of what a live session actually settled, which is what the
	// admission line above is refusing to overstate.
	if !strings.Contains(fixture.LiveSessions, "interrupted") && !strings.Contains(fixture.LiveSessions, "killed") {
		t.Fatalf("fixture records no interrupted live session: %q", fixture.LiveSessions)
	}
	// The interrupted live session is what admission turns on, so the record
	// says it happened rather than that it is expected to.
	if !strings.Contains(fixture.LiveSessions, "repair") || !strings.Contains(fixture.LiveSessions, "re-derived") {
		t.Fatalf("the live-session record does not say a repair was made and re-derived: %q", fixture.LiveSessions)
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
	// The pinned argv, then the declared ask on its own channel. Written out
	// here rather than rebuilt with the same helper the adapter uses, so that a
	// change to either half is a change this test has to be told about.
	wantArgs := append(append([]string(nil), claudeArgs...), "--append-system-prompt", claudeRepairAskText)
	if invocation.Path != cfg.Executable || !reflect.DeepEqual(invocation.Args, wantArgs) {
		t.Fatalf("Claude invocation = %#v, want configured path, fixed argv and the repair ask", invocation)
	}
	if invocation.Output == nil {
		t.Fatal("Claude invocation exposes no native stdout receiver")
	}
	wantEnv := []string{
		"HOME=" + cfg.ClaudeHome,
		"CLAUDE_CONFIG_DIR=" + filepath.Join(cfg.ClaudeHome, "config"),
		"CLAUDE_CODE_TMPDIR=" + cfg.ClaudeTemp,
		"CLAUDE_CODE_OAUTH_TOKEN=test-claude-setup-token",
	}
	if !reflect.DeepEqual(invocation.Env, wantEnv) {
		t.Fatalf("Claude environment = %#v, want a clean home, a box scratch directory and exactly the subscription token", invocation.Env)
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
	// --verbose is in this list because the session refuses to start without it
	// under stream-json output, and --replay-user-messages because it is what
	// puts the packet's arrival in the receipt of a session that is killed
	// before it produces an assistant turn.
	for _, required := range []string{"--print", "--verbose", "--input-format", "--output-format", "--include-partial-messages", "--replay-user-messages", "--no-session-persistence"} {
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
		{"missing executable", func(t *testing.T, cfg *RuntimeConfig, _ *PacketInput) {
			cfg.Executable = filepath.Join(t.TempDir(), "missing-claude")
		}, "executable is unavailable"},
		{"nonexecutable file", func(t *testing.T, cfg *RuntimeConfig, _ *PacketInput) {
			cfg.Executable = filepath.Join(t.TempDir(), "not-executable")
			if err := os.WriteFile(cfg.Executable, []byte("fixture"), 0o600); err != nil {
				t.Fatal(err)
			}
		}, "executable is not executable"},
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
			t.Setenv(claudeAPIKeyEnvironment, "placeholder that is not a credential")
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

// TestClaudeRefusesEveryCredentialPrecedenceName walks the whole declared
// family and proves each entry is live. A name added to the table without a
// working refusal, or left in it after the check stopped consulting it, fails
// here rather than being discovered when a repair bills to the wrong account.
func TestClaudeRefusesEveryCredentialPrecedenceName(t *testing.T) {
	receipt, _ := testReceipt(t)
	adapter, err := receipt.ForAdapter(Claude, claudeAdapterVersion)
	if err != nil {
		t.Fatal(err)
	}
	if len(claudeRefusedEnvironment) == 0 {
		t.Fatal("the refused-environment family is empty")
	}
	for name, hazard := range claudeRefusedEnvironment {
		t.Run(name, func(t *testing.T) {
			cfg := claudeConfig(t)
			packet, err := AssemblePacket(packetInput())
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv(name, "hostile-value-that-must-never-be-quoted")
			_, err = prepareClaude(cfg, packet, adapter)
			if err == nil {
				t.Fatalf("%s in the parent environment did not refuse dispatch", name)
			}
			if !strings.Contains(err.Error(), name+" is present in the parent environment") {
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

// TestClaudeCredentialFamilyMembershipIsPinned is what makes the walk above
// mean something. A table-driven test that reads its own table cannot notice an
// entry being deleted — it simply tests one name fewer and still passes. So the
// membership is pinned here literally, and dropping a name from the family is a
// failing test rather than a silent widening of what may reach the child.
// Adding one is deliberately free: this list is a floor, not an inventory.
func TestClaudeCredentialFamilyMembershipIsPinned(t *testing.T) {
	for name, hazard := range map[string]string{
		"CLAUDE_CODE_API_KEY_FILE_DESCRIPTOR":     claudeOverridesCredential,
		"CLAUDE_CODE_OAUTH_TOKEN_FILE_DESCRIPTOR": claudeOverridesCredential,
		"CLAUDE_CODE_OAUTH_REFRESH_TOKEN":         claudeOverridesCredential,
		"CLAUDE_CODE_HOST_AUTH_ENV_VAR":           claudeOverridesCredential,
		"CLAUDE_CODE_HOST_CREDS_FILE":             claudeOverridesCredential,
		"CLAUDE_CODE_SESSION_ACCESS_TOKEN":        claudeOverridesCredential,
		"CLAUDE_API_KEY":                          claudeOverridesCredential,
		"CLAUDE_BRIDGE_OAUTH_TOKEN":               claudeOverridesCredential,
		"CLAUDE_TRUSTED_DEVICE_TOKEN":             claudeOverridesCredential,
		"CLAUDE_SESSION_INGRESS_TOKEN_FILE":       claudeOverridesCredential,
		"CLAUDE_BG_AUTH_SNAPSHOT_PATH":            claudeOverridesCredential,
		"CLAUDE_BG_SOCKET_TOKENS_PATH":            claudeOverridesCredential,
		"CCR_OAUTH_TOKEN_FILE":                    claudeOverridesCredential,
		"CLAUDE_CODE_MANAGED_SETTINGS_PATH":       claudeCarriesAuthHelper,
		"CLAUDE_CODE_REMOTE_SETTINGS_PATH":        claudeCarriesAuthHelper,
		"CLAUDE_SECURESTORAGE_CONFIG_DIR":         claudeCarriesAuthHelper,
		"CLAUDE_ENV_FILE":                         claudeCarriesAuthHelper,
		"CLAUDE_CONFIG_DIR":                       claudeCarriesAuthHelper,
		"CLAUDE_CODE_API_BASE_URL":                claudeRedirectsUpstream,
		"CLAUDE_CODE_CUSTOM_OAUTH_URL":            claudeRedirectsUpstream,
		"CLAUDE_CODE_PROXY_URL":                   claudeRedirectsUpstream,
		"CLAUDE_CODE_PROXY_HOST":                  claudeRedirectsUpstream,
		"CLAUDE_CODE_HTTP_PROXY":                  claudeRedirectsUpstream,
		"CLAUDE_CODE_HTTPS_PROXY":                 claudeRedirectsUpstream,
		"CLAUDE_CODE_ENABLE_PROXY_AUTH_HELPER":    claudeRedirectsUpstream,
		"CLAUDE_AI_AUTHORIZE_URL":                 claudeRedirectsUpstream,
		"CLAUDE_LOCAL_OAUTH_API_BASE":             claudeRedirectsUpstream,
		"CLAUDE_BRIDGE_BASE_URL":                  claudeRedirectsUpstream,
		"HTTP_PROXY":                              claudeRedirectsUpstream,
		"HTTPS_PROXY":                             claudeRedirectsUpstream,
		"ALL_PROXY":                               claudeRedirectsUpstream,
		"http_proxy":                              claudeRedirectsUpstream,
		"https_proxy":                             claudeRedirectsUpstream,
		"all_proxy":                               claudeRedirectsUpstream,
		"NODE_EXTRA_CA_CERTS":                     claudeRedirectsUpstream,
		"CLAUDE_CODE_USE_BEDROCK":                 claudeSwitchesProvider,
		"CLAUDE_CODE_USE_VERTEX":                  claudeSwitchesProvider,
		"CLAUDE_CODE_USE_FOUNDRY":                 claudeSwitchesProvider,
		"CLAUDE_CODE_USE_MANTLE":                  claudeSwitchesProvider,
		"CLAUDE_CODE_USE_GATEWAY":                 claudeSwitchesProvider,
		"CLAUDE_CODE_USE_CCR_V2":                  claudeSwitchesProvider,
		"CLAUDE_CODE_USE_ANTHROPIC_AWS":           claudeSwitchesProvider,
		"CLAUDE_CODE_USE_ANTHROPIC_GOOGLE_CLOUD":  claudeSwitchesProvider,
	} {
		got, present := claudeRefusedEnvironment[name]
		if !present {
			t.Errorf("%s left the refused-credential family", name)
			continue
		}
		if got != hazard {
			t.Errorf("%s is classified %q, want %q", name, got, hazard)
		}
	}
	if claudeRefusedNamespace != "ANTHROPIC_" {
		t.Errorf("refused namespace = %q, want the whole ANTHROPIC_ family", claudeRefusedNamespace)
	}
	// The API-key refusal this family grew out of is still one of its members
	// rather than a case that quietly became special again.
	if !strings.HasPrefix(claudeAPIKeyEnvironment, claudeRefusedNamespace) {
		t.Errorf("%s is no longer covered by the namespace rule", claudeAPIKeyEnvironment)
	}
}

// TestClaudeRefusesTheWholeAnthropicNamespace proves the namespace half is a
// rule and not a list: a name that does not appear anywhere in this repository,
// as a future release's would not, is refused on its prefix alone.
func TestClaudeRefusesTheWholeAnthropicNamespace(t *testing.T) {
	receipt, _ := testReceipt(t)
	adapter, err := receipt.ForAdapter(Claude, claudeAdapterVersion)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"ANTHROPIC_API_KEY",
		"ANTHROPIC_AUTH_TOKEN",
		"ANTHROPIC_BASE_URL",
		"ANTHROPIC_CUSTOM_HEADERS",
		"ANTHROPIC_UNIX_SOCKET",
		"ANTHROPIC_IDENTITY_TOKEN_FILE",
		"ANTHROPIC_FEDERATION_RULE_ID",
		"ANTHROPIC_CONFIG_DIR",
		"ANTHROPIC_A_NAME_THIS_RELEASE_DOES_NOT_HAVE",
	} {
		t.Run(name, func(t *testing.T) {
			cfg := claudeConfig(t)
			packet, err := AssemblePacket(packetInput())
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv(name, "hostile")
			if _, err := prepareClaude(cfg, packet, adapter); err == nil ||
				!strings.Contains(err.Error(), name+" is present in the parent environment") {
				t.Fatalf("%s = %v, want a refusal naming it", name, err)
			}
		})
	}
}

// TestClaudeAdmitsTheDeliberatelyExcludedNames is the other half of the cut. A
// guard that fires on an ordinary developer machine gets switched off, so the
// exclusions are load-bearing and have to be as tested as the refusals: a
// third-party credential store does nothing without a provider switch, NO_PROXY
// cannot send anything anywhere, and a live Claude Code session exports its own
// ambient names that say nothing about which credential is in force.
func TestClaudeAdmitsTheDeliberatelyExcludedNames(t *testing.T) {
	receipt, _ := testReceipt(t)
	adapter, err := receipt.ForAdapter(Claude, claudeAdapterVersion)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"AWS_ACCESS_KEY_ID",
		"AWS_SECRET_ACCESS_KEY",
		"AWS_PROFILE",
		"AZURE_OPENAI_API_KEY_CHAT",
		"GOOGLE_APPLICATION_CREDENTIALS",
		"NO_PROXY",
		"CLAUDECODE",
		"CLAUDE_CODE_ENTRYPOINT",
		"CLAUDE_CODE_SESSION_ID",
		"CLAUDE_CODE_MESSAGING_TOKEN",
	} {
		t.Run(name, func(t *testing.T) {
			cfg := claudeConfig(t)
			packet, err := AssemblePacket(packetInput())
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv(name, "present")
			if _, err := prepareClaude(cfg, packet, adapter); err != nil {
				t.Fatalf("%s refused dispatch: %v; it is excluded on purpose", name, err)
			}
		})
	}
}

// TestClaudeRefusalNamesTheSameOffenderEveryRun keeps the refusal off the
// environment's ordering. Two hostile names must not produce a message that
// changes between runs, because a disposition is a record somebody compares.
func TestClaudeRefusalNamesTheSameOffenderEveryRun(t *testing.T) {
	environment := []string{
		"PATH=/usr/bin",
		"HTTPS_PROXY=http://elsewhere",
		"ANTHROPIC_API_KEY=secret",
		"CLAUDE_CODE_USE_BEDROCK=1",
	}
	name, hazard, refused := claudeRefusedParentEnvironment(environment)
	if !refused || name != "ANTHROPIC_API_KEY" || hazard != claudeOverridesCredential {
		t.Fatalf("refusal = %q/%q/%v, want the sorted-first offender", name, hazard, refused)
	}
	reversed := []string{environment[3], environment[2], environment[1], environment[0]}
	again, _, _ := claudeRefusedParentEnvironment(reversed)
	if again != name {
		t.Fatalf("reordering the environment changed the offender: %q then %q", name, again)
	}
	if _, _, refused := claudeRefusedParentEnvironment([]string{"PATH=/usr/bin", "HOME=/home/x"}); refused {
		t.Fatal("a clean environment was refused")
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
	if got, want := ReceiptText(entries), "thread_native,thread_model,thread_native,thread_native,thread_transcript,thread_native,thread_turn,thread_native,thread_window,thread_native,thread_cost,thread_turn"; got != want {
		t.Fatalf("fixture receipt kinds = %q, want %q", got, want)
	}
	// The runtime's own replay of the packet is retained as native evidence and
	// projects nothing: it is what the child received, not what it produced, and
	// a transcript kept authored from its own packet would be kept quoting
	// itself back as though the runtime had said it.
	// The session-start event says which model answered, and it is projected
	// from the runtime's own account rather than from anything kept configured:
	// kept names no model at all on the way in.
	if entries[1].Model.NativeRef != entries[0].Native.Ref || entries[1].Model.Model != "claude-sonnet-5" {
		t.Fatalf("init model projection = %#v, want the model its native init named", entries[1].Model)
	}
	if entries[2].Native.Type != "user" || !bytes.Contains(entries[2].Native.Payload, []byte("promise:kept-is-tested")) {
		t.Fatalf("the replayed packet was not retained as a native user event: %#v", entries[2].Native)
	}
	if entries[3].Native.Type != "stream_event" || entries[4].Transcript.NativeRef != entries[3].Native.Ref {
		t.Fatalf("stream transcript did not refer to its retained native event: %#v", entries[3:5])
	}
	transcript, err := ledger.ReadArtifact(store, entries[4].Transcript.Artifact)
	if err != nil {
		t.Fatal(err)
	}
	if string(transcript) != "partial repair evidence" {
		t.Fatalf("stream transcript = %q, want the native delta", transcript)
	}
	if entries[6].Turn.NativeRef != entries[5].Native.Ref || entries[6].Turn.Count != 1 {
		t.Fatalf("assistant turn = %#v, want its first native turn boundary", entries[6].Turn)
	}
	// Both windows or neither, in the runtime's own names for them and in an
	// order that is not the native object's.
	window := entries[8].Window
	if window.NativeRef != entries[7].Native.Ref || len(window.Windows) != 2 {
		t.Fatalf("window projection = %#v, want both windows of its native event", window)
	}
	if window.Windows[0].Name != "five_hour" || window.Windows[0].Utilization != "0.25" {
		t.Fatalf("five-hour window = %#v", window.Windows[0])
	}
	if window.Windows[1].Name != "seven_day" || window.Windows[1].Utilization != "0.11" {
		t.Fatalf("seven-day window = %#v", window.Windows[1])
	}
	if got, want := window.Windows[0].ResetsAt, time.Unix(1788078000, 0).UTC(); !got.Equal(want) {
		t.Fatalf("five-hour reset = %s, want %s", got, want)
	}
	if entries[9].Native.Type != "result" || entries[10].Cost.NativeRef != entries[9].Native.Ref || entries[10].Cost.Amount != "0.0042" || entries[11].Turn.NativeRef != entries[9].Native.Ref || entries[11].Turn.Count != 2 {
		t.Fatalf("result projections = %#v", entries[9:])
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

// TestTheClaudeAdapterIsAdmittedAndTheRecordAgrees is the admission gate seen
// from both sides at once, and it replaces the test that held it shut.
//
// Decision 0052 admits a runtime on evidence rather than on a flag, so the flag
// and the record of what was measured must not be able to disagree: an adapter
// that dispatches while its fixture still says unadmitted is a gate somebody
// opened without writing down why, and a fixture claiming admission while the
// adapter refuses is a record of a session nobody can run.
func TestTheClaudeAdapterIsAdmittedAndTheRecordAgrees(t *testing.T) {
	adapter, err := Select(string(Claude))
	if err != nil {
		t.Fatal(err)
	}
	packet, err := AssemblePacket(packetInput())
	if err != nil {
		t.Fatal(err)
	}
	receipt, _ := testReceipt(t)
	adapterReceipt, err := receipt.ForAdapter(Claude, adapter.AdapterVersion(claudeConfig(t)))
	if err != nil {
		t.Fatalf("the admitted Claude adapter has no receipt: %v", err)
	}
	if _, err := adapter.Prepare(claudeConfig(t), packet, adapterReceipt); err != nil {
		t.Fatalf("the admitted Claude adapter refused to prepare an invocation: %v", err)
	}
	fixture := loadClaudeFixture(t)
	if strings.Contains(fixture.Admission, "unadmitted") {
		t.Fatalf("the adapter dispatches and its fixture says it does not: %q", fixture.Admission)
	}
	// And the record says where the evidence is, rather than asserting it.
	if !strings.Contains(fixture.Admission, "claudeadmission_test.go") {
		t.Fatalf("the fixture claims admission without naming what proved it: %q", fixture.Admission)
	}
}

// TestClaudeFixtureRecordsWhatWasMeasuredAndWhatWasNot holds the fixture to the
// distinction the whole record turns on. Decision 0052 admits a runtime on
// evidence, so a fixture that mixed a recording with a documented shape would
// be claiming a session that never happened.
func TestClaudeFixtureRecordsWhatWasMeasuredAndWhatWasNot(t *testing.T) {
	fixture := loadClaudeFixture(t)
	if !strings.Contains(fixture.NativeProvenance, "not a recording") {
		t.Fatalf("the fixture stream does not say it is a documented shape: %q", fixture.NativeProvenance)
	}
	if fixture.CapturedSession == "" || !strings.Contains(fixture.CapturedProvenance, "verbatim") {
		t.Fatalf("the fixture names no verbatim captured session: %#v", fixture)
	}
	// The captured session is the honest half: it says what it is *not*.
	if !strings.Contains(fixture.CapturedProvenance, "not the interrupted live session") {
		t.Fatalf("the captured session claims to be the admission evidence: %q", fixture.CapturedProvenance)
	}
	// A Node-hosted runtime resolves an interpreter; this one does not, and the
	// difference is what decides whether ResolveNode or ResolveNative applies.
	if !strings.Contains(fixture.Distribution, "not Node-hosted") || !strings.Contains(fixture.Distribution, "refused by name") {
		t.Fatalf("the fixture does not record the runtime's distribution shape: %q", fixture.Distribution)
	}
	if !strings.Contains(fixture.TemporaryDirectory, claudeTempEnvironment) || !strings.Contains(fixture.TemporaryDirectory, "not TMPDIR") {
		t.Fatalf("the fixture does not record the temporary-directory channel: %q", fixture.TemporaryDirectory)
	}
	if !strings.Contains(fixture.Network, "api.anthropic.com") || !strings.Contains(fixture.Network, "block is wrong") {
		t.Fatalf("the fixture does not record the one host this runtime needs: %q", fixture.Network)
	}
	if !strings.Contains(fixture.CredentialMeasured, "401") {
		t.Fatalf("the fixture does not record what the credential channel was measured against: %q", fixture.CredentialMeasured)
	}
	if !strings.Contains(fixture.ArgvMeasured, "--verbose") || !strings.Contains(fixture.ArgvMeasured, "--safe-mode") {
		t.Fatalf("the fixture does not record what the argv was measured against: %q", fixture.ArgvMeasured)
	}
	if !strings.Contains(fixture.ChildEnvironment, claudeTempEnvironment) {
		t.Fatalf("the fixture's child environment omits the scratch directory: %q", fixture.ChildEnvironment)
	}
}

// TestClaudeRefusesAScratchDirectoryItCannotUse is the guard for the failure
// this adapter's own measurement produced: a confined session with no
// CLAUDE_CODE_TMPDIR opens a host scratch root the profile denies and dies
// having emitted nothing at all. Every branch that would let a dispatch reach
// that state refuses before a process exists.
func TestClaudeRefusesAScratchDirectoryItCannotUse(t *testing.T) {
	receipt, _ := testReceipt(t)
	adapter, err := receipt.ForAdapter(Claude, claudeAdapterVersion)
	if err != nil {
		t.Fatal(err)
	}
	packet, err := AssemblePacket(packetInput())
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		with func(*RuntimeConfig)
		want string
	}{
		{"unset", func(c *RuntimeConfig) { c.ClaudeTemp = "" }, claudeTempEnvironment + " is unset"},
		{"relative", func(c *RuntimeConfig) { c.ClaudeTemp = "scratch" }, "not an explicit absolute path"},
		{"the session home", func(c *RuntimeConfig) { c.ClaudeTemp = c.ClaudeHome }, "is the session home"},
		{"missing", func(c *RuntimeConfig) { c.ClaudeTemp = filepath.Join(t.TempDir(), "gone") }, "is unavailable"},
		{"a file", func(c *RuntimeConfig) { c.ClaudeTemp = file }, "is not a directory"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := claudeConfig(t)
			tc.with(&cfg)
			_, err := prepareClaude(cfg, packet, adapter)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("prepareClaude with %s scratch directory = %v, want a refusal naming %q", tc.name, err, tc.want)
			}
			if !strings.Contains(err.Error(), "dispatch refused") {
				t.Fatalf("scratch refusal produced %v, want the dispatch Refusal shape", err)
			}
		})
	}
}

// TestClaudeProjectsItsCapturedConfinedSession runs the recording of a real
// confined Claude session through the adapter's own stream.
//
// It is not the admission evidence and does not pretend to be: the session had
// no valid credential, so it reached authentication and stopped. What it does
// establish is that the event vocabulary the adapter parses is the vocabulary
// the runtime actually emits from inside a box — including the events the
// hand-written fixture has no way to invent, and including the packet coming
// back out as the runtime's own record of what it read.
func TestClaudeProjectsItsCapturedConfinedSession(t *testing.T) {
	invocation, receipt, store := prepareClaudeFixture(t)
	fixture := loadClaudeFixture(t)
	src, err := os.ReadFile(filepath.Join("testdata", "claude", fixture.CapturedSession))
	if err != nil {
		t.Fatal(err)
	}
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
	if len(quarantined) != 0 {
		t.Fatalf("the captured session quarantined its own evidence: %#v", quarantined)
	}

	seen := map[string]int{}
	var replayed []byte
	for _, entry := range entries {
		if entry.Native == nil {
			continue
		}
		seen[entry.Native.Type]++
		if entry.Native.Type == "user" {
			replayed = entry.Native.Payload
		}
	}
	// system carries init, status and api_retry; the adapter retains each with
	// its own native type and invents no common meaning for any of them.
	for _, want := range []string{"system", "user", "assistant", "result"} {
		if seen[want] == 0 {
			t.Fatalf("the captured session retained no %s native event: %v", want, seen)
		}
	}
	if !bytes.Contains(replayed, []byte("promise:the-corpus-cannot-write-standing-or-truth")) {
		t.Fatalf("the captured session's replayed packet does not carry its marker: %s", replayed)
	}
	// The --safe-mode surface, from the runtime's own account of the session it
	// started rather than from the flag's help.
	var init []byte
	for _, entry := range entries {
		if entry.Native != nil && entry.Native.Type == "system" && bytes.Contains(entry.Native.Payload, []byte(`"subtype":"init"`)) {
			init = entry.Native.Payload
		}
	}
	if init == nil {
		t.Fatal("the captured session retained no init event")
	}
	for _, want := range []string{`"mcp_servers":[]`, `"plugins":[]`, `"permissionMode":"bypassPermissions"`} {
		if !bytes.Contains(init, []byte(want)) {
			t.Fatalf("the captured confined session did not report %s: %s", want, init)
		}
	}

	// And the projections, which a completion-only report would not have: a
	// turn from the assistant boundary and a cost and turn from the result, each
	// naming the native event it came from.
	assertClaudeProjectionsAreNativeBacked(t, entries)
	kinds := map[string]int{}
	for _, kind := range strings.Split(ReceiptText(entries), ",") {
		kinds[kind]++
	}
	if kinds["thread_turn"] == 0 || kinds["thread_cost"] == 0 {
		t.Fatalf("the captured session produced no turn or cost projection: %v", kinds)
	}
}

func assertClaudeProjectionsAreNativeBacked(t *testing.T, entries []Entry) {
	t.Helper()
	refs := map[string]bool{}
	for _, entry := range entries {
		if entry.Native != nil {
			refs[entry.Native.Ref] = true
		}
	}
	for i, entry := range entries {
		var ref, kind string
		switch {
		case entry.Transcript != nil:
			ref, kind = entry.Transcript.NativeRef, "transcript"
		case entry.Cost != nil:
			ref, kind = entry.Cost.NativeRef, "cost"
		case entry.Turn != nil:
			ref, kind = entry.Turn.NativeRef, "turn"
		case entry.Model != nil:
			ref, kind = entry.Model.NativeRef, "model"
		case entry.Window != nil:
			ref, kind = entry.Window.NativeRef, "window"
		default:
			continue
		}
		if ref == "" || !refs[ref] {
			t.Fatalf("entry %d is a %s projection naming %q, which this receipt does not hold", i, kind, ref)
		}
	}
}

// TestABrokeredClaudeCarriesNoCredentialAtAll is the adapter half of decision
// 0057. Under the brokered channel the confinement provider publishes the
// credential variable itself, so this adapter must not — and what proves it is
// the complete child environment rather than the absence of a field.
func TestABrokeredClaudeCarriesNoCredentialAtAll(t *testing.T) {
	cfg := claudeConfig(t)
	cfg.ClaudeSetupToken = ""
	cfg.ClaudeBrokeredCredential = true

	receipt, _ := testReceipt(t)
	adapter, err := receipt.ForAdapter(Claude, claudeAdapterVersion)
	if err != nil {
		t.Fatal(err)
	}
	packet, err := AssemblePacket(packetInput())
	if err != nil {
		t.Fatal(err)
	}
	invocation, err := prepareClaude(cfg, packet, adapter)
	if err != nil {
		t.Fatalf("a brokered configuration was refused: %v", err)
	}
	for _, value := range invocation.Env {
		name, _, _ := strings.Cut(value, "=")
		if name == claudeCredentialEnvironment {
			t.Fatalf("the brokered invocation carries the credential variable: %q", name)
		}
	}
	// The rest of the environment is unchanged, so the absence above is this
	// one field rather than an invocation that failed to be built.
	for _, want := range []string{"HOME=", claudeConfigEnvironment + "=", claudeTempEnvironment + "="} {
		found := false
		for _, value := range invocation.Env {
			if strings.HasPrefix(value, want) {
				found = true
			}
		}
		if !found {
			t.Fatalf("the brokered invocation is missing %s: %v", want, invocation.Env)
		}
	}
	// And nothing else a repair writes down carries a credential either,
	// because under this channel there is no credential in the process to carry.
	if len(invocation.Args) == 0 {
		t.Fatal("the brokered invocation has no argv")
	}
	for _, arg := range invocation.Args {
		if strings.Contains(strings.ToLower(arg), "token") || strings.Contains(arg, "sk-ant") {
			t.Fatalf("argv carries something credential-shaped: %q", arg)
		}
	}
}

// TestClaudeRefusesTwoCredentialChannelsAtOnce is the guard that keeps the two
// channels from both being in force. Whichever won the merge would decide
// whether the box holds a real credential, and nothing would report which.
func TestClaudeRefusesTwoCredentialChannelsAtOnce(t *testing.T) {
	packet, err := AssemblePacket(packetInput())
	if err != nil {
		t.Fatal(err)
	}
	cfg := claudeConfig(t)
	cfg.ClaudeBrokeredCredential = true
	err = checkClaudeConfig(cfg, packet)
	if err == nil {
		t.Fatal("a configuration naming both credential channels was accepted")
	}
	if !strings.Contains(err.Error(), "both brokered and direct") {
		t.Fatalf("refusal = %v, want it to name the collision", err)
	}

	// Neither channel is refused too, and by the direct channel's own message:
	// a brokered dispatch is a deliberate choice rather than what happens when
	// a token was forgotten.
	empty := claudeConfig(t)
	empty.ClaudeSetupToken = ""
	if err := checkClaudeConfig(empty, packet); err == nil {
		t.Fatal("a configuration naming no credential channel was accepted")
	} else if !strings.Contains(err.Error(), claudeCredentialEnvironment+" is unset") {
		t.Fatalf("refusal = %v, want it to name the unset credential", err)
	}
}

// TestABrokeredClaudeStillRefusesALapsedCredential is the expiry check under
// the channel that does not carry the token. The lapse is a fact about the
// credential rather than about how it travels, so the refusal is the same one —
// and it is reached without reading the token, which is the whole point of
// sourcing it from configuration.
func TestABrokeredClaudeStillRefusesALapsedCredential(t *testing.T) {
	packet, err := AssemblePacket(packetInput())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		expiry time.Time
		want   string
	}{
		{"unset", time.Time{}, "setup-token expiry is unset"},
		{"lapsed", time.Now().Add(-time.Hour), "setup-token has lapsed"},
		{"inside the margin", time.Now().Add(claudeSetupTokenMargin / 2), "lapses inside the dispatch margin"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := claudeConfig(t)
			cfg.ClaudeSetupToken = ""
			cfg.ClaudeBrokeredCredential = true
			cfg.ClaudeSetupTokenExpiry = tc.expiry
			err := checkClaudeConfig(cfg, packet)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("checkClaudeConfig with expiry %v = %v, want %q", tc.expiry, err, tc.want)
			}
		})
	}
}

// TestABrokeredReceiptHasNoCredentialToRedactAndSaysWhatItRetains records the
// consequence decision 0057 accepts, rather than leaving it to be discovered.
//
// Under the brokered channel there is no secret in this process, so a receipt
// cannot leak one — that half is stronger than redaction, not weaker. What kept
// cannot do is remove the *phantom*: the provider mints it and never tells kept,
// so a session that echoes its own environment into a tool result puts it in the
// receipt. The phantom dies with the session and is useless outside its box, and
// this test is what keeps that from being a surprise later.
func TestABrokeredReceiptHasNoCredentialToRedactAndSaysWhatItRetains(t *testing.T) {
	const operatorSecret = "sk-ant-oat01-the-operators-real-credential"
	const sessionPhantom = "3f6a2c1de4b8079a5c2f18d0aa7be3419cd05f27ab6183e4c90d7f25ba48e1c6"

	cfg := claudeConfig(t)
	cfg.ClaudeSetupToken = ""
	cfg.ClaudeBrokeredCredential = true

	receipt, store := testReceipt(t)
	adapter, err := receipt.ForAdapter(Claude, claudeAdapterVersion)
	if err != nil {
		t.Fatal(err)
	}
	packet, err := AssemblePacket(packetInput())
	if err != nil {
		t.Fatal(err)
	}
	// The packet is what the runtime is handed, and it carries neither.
	if bytes.Contains(packet.Bytes(), []byte(operatorSecret)) {
		t.Fatal("the packet carries the credential")
	}
	invocation, err := prepareClaude(cfg, packet, adapter)
	if err != nil {
		t.Fatal(err)
	}

	// A session that echoed its whole environment back. Both values are in the
	// event so the two outcomes are read from one record rather than argued.
	src := []byte(`{"type":"assistant","message":{"content":[{"type":"text","text":"CLAUDE_CODE_OAUTH_TOKEN=` +
		sessionPhantom + ` and nothing named ` + operatorSecret[:12] + `"}]}}` + "\n")
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
	if len(quarantined) != 0 || len(entries) == 0 {
		t.Fatalf("brokered receipt = %#v, quarantine = %#v", entries, quarantined)
	}

	// Nothing was redacted, because there was nothing to redact.
	if entries[0].Native.Redaction.Occurred {
		t.Fatalf("a brokered stream reported a redaction: %#v", entries[0].Native.Redaction)
	}
	retained := entries[0].Native.Payload
	if bytes.Contains(retained, []byte(operatorSecret)) {
		t.Fatalf("the operator's credential reached the receipt: %q", retained)
	}
	// And the phantom is retained, which is the accepted cost stated as a test.
	if !bytes.Contains(retained, []byte(sessionPhantom)) {
		t.Fatalf("the phantom was removed by something, so this no longer records what it claims: %q", retained)
	}
}

// TestTheClaudeAskIsDeclaredAndNeverDefaulted is the guard on decision 0058's
// closed set. An adapter that quietly launched with no ask would produce the
// unframed session the channel exists to stop being, and its receipt would look
// exactly like a framed one's.
func TestTheClaudeAskIsDeclaredAndNeverDefaulted(t *testing.T) {
	packet, err := AssemblePacket(packetInput())
	if err != nil {
		t.Fatal(err)
	}
	for _, unwelcome := range []ClaudeAsk{"", "argue", "Repair", "repair ", "Assess"} {
		cfg := claudeConfig(t)
		cfg.ClaudeAsk = unwelcome
		err := checkClaudeConfig(cfg, packet)
		if err == nil {
			t.Fatalf("a configuration whose ask is %q was accepted", unwelcome)
		}
		if !strings.Contains(err.Error(), "claude ask is") {
			t.Fatalf("ask %q was refused for some other reason: %v", unwelcome, err)
		}
	}
	// A known ask that does not suit the input in front of it is the other
	// failure, and it is refused separately and by name. The assessor's ask
	// over a packet is a session told to attack an act while being handed a gap
	// to close, which is the mismatch that would otherwise look like a session
	// that simply did nothing. See docs/decision/0081.
	cfg := claudeConfig(t)
	cfg.ClaudeAsk = ClaudeAssessAsk
	err = checkClaudeConfig(cfg, packet)
	if err == nil {
		t.Fatal("the assessor's ask was accepted over a repair packet")
	}
	if !strings.Contains(err.Error(), "does not suit its input") {
		t.Fatalf("the mismatched ask was refused for some other reason: %v", err)
	}

	// And each value there is resolves to something a runtime can read.
	for _, ask := range []ClaudeAsk{ClaudeRepairAsk, ClaudeAssessAsk} {
		text, err := claudeAskText(ask)
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(text) == "" {
			t.Fatalf("the %s ask resolves to nothing, so a session would be launched with an empty system prompt addition", ask)
		}
	}
}

// TestTheClaudeAskTravelsBesideThePacketAndCarriesNothingFromIt is decision
// 0058's boundary, which is the whole reason the ask is a separate channel
// rather than a prefix.
//
// Three properties. The packet's bytes are still the entire user message, with
// nothing prepended — claudeInput's job, asserted here from the invocation the
// adapter actually returns. The ask is one argv element and does not vary with
// the packet, so it is a declaration rather than a per-repair instruction. And
// it carries nothing the packet carries, which is what says the two channels
// are not one channel written twice.
func TestTheClaudeAskTravelsBesideThePacketAndCarriesNothingFromIt(t *testing.T) {
	receipt, _ := testReceipt(t)
	adapter, err := receipt.ForAdapter(Claude, claudeAdapterVersion)
	if err != nil {
		t.Fatal(err)
	}
	cfg := claudeConfig(t)

	first, err := AssemblePacket(packetInput())
	if err != nil {
		t.Fatal(err)
	}
	// A second, different packet: another promise, another gap, another scope.
	other := packetInput()
	other.Promise.ID = "promise:something-else-entirely"
	other.Promise.Statement = "the manuscript compiles"
	other.Promise.RepairScope = []string{"manuscript/**"}
	other.Why = "A different why, for a different promise."
	second, err := AssemblePacket(other)
	if err != nil {
		t.Fatal(err)
	}
	if string(first.Bytes()) == string(second.Bytes()) {
		t.Fatal("the two packets are the same bytes, so this proves nothing")
	}

	asks := make([]string, 0, 2)
	for _, packet := range []Packet{first, second} {
		invocation, err := prepareClaude(cfg, packet, adapter)
		if err != nil {
			t.Fatal(err)
		}
		index := slices.Index(invocation.Args, "--append-system-prompt")
		if index < 0 || index+1 >= len(invocation.Args) {
			t.Fatalf("the invocation carries no ask channel: %v", invocation.Args)
		}
		if slices.Index(invocation.Args[index+1:], "--append-system-prompt") >= 0 {
			t.Fatalf("the invocation carries the ask channel twice: %v", invocation.Args)
		}
		ask := invocation.Args[index+1]
		asks = append(asks, ask)

		// The packet, whole and unprefixed, still the entire user message.
		var delivered struct {
			Message struct {
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal(bytes.TrimSuffix(invocation.Input, []byte{'\n'}), &delivered); err != nil {
			t.Fatal(err)
		}
		if len(delivered.Message.Content) != 1 || delivered.Message.Content[0].Text != string(packet.Bytes()) {
			t.Fatal("the ask channel changed what the input channel delivers")
		}
		// And nothing from the packet is in the ask. The promise id is the
		// sharpest of these: it is what a per-repair instruction would name.
		for _, borrowed := range []string{string(packet.Bytes()), "promise:", "manuscript/**", "internal/**"} {
			if strings.Contains(ask, borrowed) {
				t.Fatalf("the ask carries %q, which belongs to the packet", borrowed)
			}
		}
	}
	if asks[0] != asks[1] {
		t.Fatal("the ask differs between two packets, so it is an instruction about one repair rather than a declaration about the work")
	}
}

// TestTheClaudeFixtureRecordsTheAskChannel holds the fixture to the argv it
// pins. Decision 0052 says the fixture records the direct argv; decision 0058
// added an element to it, and a fixture that recorded only the old half would
// be a record of an invocation that no longer happens.
func TestTheClaudeFixtureRecordsTheAskChannel(t *testing.T) {
	fixture := loadClaudeFixture(t)
	if !strings.Contains(fixture.Ask.Channel, "--append-system-prompt") {
		t.Fatalf("the fixture does not name the ask's channel: %q", fixture.Ask.Channel)
	}
	if !strings.Contains(fixture.Ask.Channel, "0058") {
		t.Fatalf("the fixture's ask does not cite the decision that added it: %q", fixture.Ask.Channel)
	}
	// The closed set, and the fixture holds exactly what the adapter resolves.
	if !slices.Equal(fixture.Ask.Values, []string{string(ClaudeRepairAsk)}) {
		t.Fatalf("the fixture records asks %v, and the adapter resolves %q and nothing else", fixture.Ask.Values, ClaudeRepairAsk)
	}
	for _, value := range fixture.Ask.Values {
		if _, err := claudeAskText(ClaudeAsk(value)); err != nil {
			t.Fatalf("the fixture records an ask the adapter refuses: %q: %v", value, err)
		}
	}
	if fixture.Ask.Why == "" || fixture.Ask.Measured == "" || fixture.Ask.NotInThePacket == "" {
		t.Fatalf("the fixture's ask omits why it exists, what was measured, or what keeps it out of the packet: %#v", fixture.Ask)
	}
	// And the words are not here. One rule, one place: the fixture says which
	// channel and which values, and the adapter says what they say.
	if strings.Contains(fixture.Ask.Why+fixture.Ask.Measured+fixture.Ask.NotInThePacket+fixture.Ask.Channel, claudeRepairAskText) {
		t.Fatal("the fixture holds a second copy of the ask text")
	}
}

// TestARuntimesOwnNoticeIsNotProjectedAsSomethingAModelSaid is a recording of
// the one session in this repository's ledger that a runtime answered without
// calling a model, and it is the failure rather than the shape.
//
// An adversarial pass was launched onto an account at its rate limit. The
// runtime composed one assistant message itself — model `<synthetic>`,
// `is_api_error_message` true, 65 bytes reading "You've hit your session limit ·
// resets 1pm" — and a result reporting `is_error`, `terminal_reason: api_error`
// and status 429, at zero cost. No streamed text had arrived, so the adapter read
// that message as the session's transcript and internal/repair filed it as the
// pass's argument. The episode escalated for the right reason with a record
// saying a pass argued, which is the one thing an argument entry exists to be
// able to say the opposite of.
//
// What must still happen is everything else: the events are retained whole,
// because a receipt keeps what the runtime wrote whether or not kept can make a
// projection of it, and the turn and the cost are the runtime's own numbers.
func TestARuntimesOwnNoticeIsNotProjectedAsSomethingAModelSaid(t *testing.T) {
	fixture := loadClaudeFixture(t)
	if fixture.CapturedSynthetic == "" || !strings.Contains(fixture.CapturedSyntheticFrom, "verbatim") {
		t.Fatalf("fixture omits its captured synthetic session: %q / %q", fixture.CapturedSynthetic, fixture.CapturedSyntheticFrom)
	}
	entries := replayClaude(t, fixture.CapturedSynthetic)

	var natives, turns int
	for _, entry := range entries {
		switch {
		case entry.Transcript != nil:
			t.Fatalf("a notice the runtime composed itself was projected as the session's transcript, at %#v.\n"+
				"A message with no model behind it is not something a model said, and filing it as one"+
				" records an argument nobody made.", entry.Transcript.Artifact)
		case entry.Native != nil:
			natives++
		case entry.Turn != nil:
			turns++
		}
	}
	// Retained, not discarded. A reader who knows what the runtime meant has the
	// bytes, which is the same rule a window reading is kept under.
	if natives != 2 {
		t.Fatalf("%d native events were retained, want the session's own two", natives)
	}
	if turns == 0 {
		t.Fatal("no turn was counted, and the runtime counted one in its own num_turns")
	}
	// And the runtime's own account of why, which is what tells a reader that
	// this session had no chance to say anything from one that had nothing to
	// say. Both leave a receipt with no transcript.
	session := Session{Entries: entries}
	if why := TerminalError(session); why != "api_error (429)" {
		t.Fatalf("the runtime's terminal error reads %q, want its own words and status", why)
	}
	// And it is the one failure that decides something: the account had no
	// capacity left, in the runtime's own words. See docs/decision/0090.
	exhausted, out := OutOfCapacity(session)
	if !out || exhausted.Said != "You've hit your session limit · resets 1pm (America/Los_Angeles)" {
		t.Fatalf("OutOfCapacity = %#v, %v, want the runtime's own notice", exhausted, out)
	}
	if !exhausted.Resets.IsZero() {
		t.Fatalf("a session that reported no window at its limit was given a reset of %v", exhausted.Resets)
	}
}

// TestOnlyARefusalForCapacityIsOutOfCapacity holds OutOfCapacity to the
// runtime's refusal and nothing near it: a session that failed on its
// credential failed for another reason, one that finished and wrote about a
// limit finished, and the reset is the earliest window the runtime last put at
// or past its limit — never one under it.
func TestOnlyARefusalForCapacityIsOutOfCapacity(t *testing.T) {
	fixture := loadClaudeFixture(t)
	if _, out := OutOfCapacity(Session{Entries: replayClaude(t, fixture.CapturedSession)}); out {
		t.Fatal("a session refused on its credential (401) was read as one the account had no capacity for")
	}
	finished := `{"type":"result","subtype":"success","is_error":false,"result":"I hit no session limit","api_error_status":429}`
	if _, out := OutOfCapacity(Session{Entries: replayClaudeLines(t, finished)}); out {
		t.Fatal("a result that was not an error was read as a refusal")
	}

	resets := time.Date(2026, 11, 20, 8, 40, 0, 0, time.UTC)
	later := resets.Add(4 * 24 * time.Hour)
	session := []string{
		fmt.Sprintf(`{"type":"rate_limit_event","rate_limit_info":{"unifiedWindows":{"five_hour":{"utilization":0.6,"resetsAt":%d}}}}`, resets.Add(-time.Hour).Unix()),
		fmt.Sprintf(`{"type":"rate_limit_event","rate_limit_info":{"unifiedWindows":{"five_hour":{"utilization":1.01,"resetsAt":%d},"seven_day":{"utilization":1,"resetsAt":%d},"overage":{"utilization":0.33,"resetsAt":%d}}}}`, resets.Unix(), later.Unix(), resets.Add(-2*time.Hour).Unix()),
		`{"type":"result","subtype":"success","is_error":true,"terminal_reason":"api_error","api_error_status":429,"result":"You've hit your session limit · resets 12:40am (America/Los_Angeles)"}`,
	}
	exhausted, out := OutOfCapacity(Session{Entries: replayClaudeLines(t, session...)})
	if !out {
		t.Fatal("a 429 result was not read as a refusal for capacity")
	}
	if !exhausted.Resets.Equal(resets) {
		t.Fatalf("the reset reads %v, want the earliest window at or past its limit, %v", exhausted.Resets, resets)
	}
	if d := Diagnose(Session{Entries: replayClaudeLines(t, session...)}); d.Exhausted == nil || d.Exhausted.Said != exhausted.Said {
		t.Fatalf("the diagnosis does not carry the exhaustion: %#v", d.Exhausted)
	}
}

// TestClaudeProjectsTheWindowTheOneRealRepairReported is the evidence half of
// decision 0077, and it is a recording rather than a construction: the two
// rate-limit payloads in testdata/claude/window.jsonl are the ones the one real
// repair in this repository's ledger retained on 2026-08-29, taken verbatim
// from its receipt.
//
// It is the reading a budget would refuse on. What it must show is not that a
// number arrives but that the right number does, under the runtime's own name
// for the window it belongs to, with the reset it was given.
func TestClaudeProjectsTheWindowTheOneRealRepairReported(t *testing.T) {
	fixture := loadClaudeFixture(t)
	if fixture.CapturedWindow == "" || !strings.Contains(fixture.CapturedWindowFrom, "verbatim") {
		t.Fatalf("fixture omits its captured window reading: %q / %q", fixture.CapturedWindow, fixture.CapturedWindowFrom)
	}
	entries := replayClaude(t, fixture.CapturedWindow)

	var readings []*WindowUpdate
	for _, entry := range entries {
		if entry.Window != nil {
			readings = append(readings, entry.Window)
		}
	}
	if len(readings) != 2 {
		t.Fatalf("the captured session's two rate-limit events produced %d window readings", len(readings))
	}
	// The window moved across the session, which is the whole reason a reading
	// is a stream rather than a summary: it is a shared counter, and the second
	// reading is what the next dispatch would be judged against.
	for i, want := range []string{"0.24", "0.25"} {
		if got := readings[i].Windows[0]; got.Name != "five_hour" || got.Utilization != want {
			t.Fatalf("five-hour reading %d = %#v, want utilization %s", i, got, want)
		}
		if got := readings[i].Windows[1]; got.Name != "seven_day" || got.Utilization != "0.11" {
			t.Fatalf("seven-day reading %d = %#v", i, got)
		}
	}
	latest, reported := LatestWindow(entries)
	if !reported || latest.Windows[0].Utilization != "0.25" {
		t.Fatalf("the reading in force = %#v, reported = %v", latest, reported)
	}
	if got, want := latest.Windows[0].ResetsAt, time.Date(2026, 8, 30, 8, 20, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("five-hour reset = %s, want %s", got, want)
	}
}

// TestASessionThatReportsNoWindowProjectsNoneRatherThanAnEmptyOne is the case
// that matters more than the present one, because it is where a budget would go
// wrong: a self-hosted runtime reports no window at all, and so would a build
// that renamed the event this adapter reads.
//
// Its evidence is a real session too. The captured 2.1.241 session carries no
// rate_limit_event of any kind, so what a receipt reporting no window looks like
// is measured rather than constructed — and the guard breaks by asking the same
// reader about the session that did report one.
func TestASessionThatReportsNoWindowProjectsNoneRatherThanAnEmptyOne(t *testing.T) {
	fixture := loadClaudeFixture(t)
	silent := replayClaude(t, fixture.CapturedSession)
	for _, entry := range silent {
		if entry.Native != nil && entry.Native.Type == "rate_limit_event" {
			t.Fatal("the captured session reports a window after all, so it is the wrong evidence for the silent case")
		}
		if entry.Window != nil {
			t.Fatalf("a session that reported no window produced a window projection: %#v", entry.Window)
		}
	}
	if _, reported := LatestWindow(silent); reported {
		t.Fatal("a session that reported no window reads as having reported one")
	}
	// The same reader, on the session that did report one. Without this the
	// test above passes for a projection that never fires at all.
	if _, reported := LatestWindow(replayClaude(t, fixture.CapturedWindow)); !reported {
		t.Fatal("a session that reported a window reads as having reported none")
	}
}

// TestAWindowThisAdapterCannotReadIsMissingRatherThanWrong holds the projection
// to the rule decision 0052 already puts on cost and turn. `rate_limit_event` is
// one runtime's vocabulary and not a contract, so every way it can move — a
// rename, a restructure, a window whose utilization is no longer a number —
// must produce no projection rather than a plausible one.
//
// A partial reading is refused for the same reason: a receipt that answered
// *what is left* with one window silently dropped is worse than one that
// answered nothing.
func TestAWindowThisAdapterCannotReadIsMissingRatherThanWrong(t *testing.T) {
	for name, line := range map[string]string{
		"renamed event":      `{"type":"usage_limit_event","rate_limit_info":{"unifiedWindows":{"five_hour":{"utilization":0.25}}}}`,
		"restructured":       `{"type":"rate_limit_event","rate_limit_info":{"windows":{"five_hour":{"utilization":0.25}}}}`,
		"no windows at all":  `{"type":"rate_limit_event","rate_limit_info":{"unifiedWindows":{}}}`,
		"utilization a word": `{"type":"rate_limit_event","rate_limit_info":{"unifiedWindows":{"five_hour":{"utilization":"most of it"}}}}`,
		"one window unread":  `{"type":"rate_limit_event","rate_limit_info":{"unifiedWindows":{"five_hour":{"utilization":0.25},"seven_day":{"utilization":null}}}}`,
		"a negative window":  `{"type":"rate_limit_event","rate_limit_info":{"unifiedWindows":{"five_hour":{"utilization":-1}}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			entries := replayClaudeLines(t, line)
			for _, entry := range entries {
				if entry.Window != nil {
					t.Fatalf("a reading this adapter cannot make produced %#v", entry.Window)
				}
			}
			// The evidence is still there. Nothing is projected and nothing is
			// discarded: a reader who knows the new shape has the bytes.
			if len(entries) != 1 || entries[0].Native == nil {
				t.Fatalf("the unreadable event was not retained as native evidence: %#v", entries)
			}
		})
	}
	// And the reading that is readable, so the cases above are not all passing
	// on a projection that never fires.
	entries := replayClaudeLines(t, `{"type":"rate_limit_event","rate_limit_info":{"unifiedWindows":{"five_hour":{"utilization":1.4}}}}`)
	window, reported := LatestWindow(entries)
	if !reported || window.Windows[0].Utilization != "1.4" {
		t.Fatalf("a window in overage = %#v, reported = %v", window, reported)
	}
}

// TestAModelIsProjectedFromWhatAnsweredAndNeverInvented is the other half. kept
// asks Claude for no model, so the only place an answer can come from is the
// runtime's own session-start event — and an init that names none must leave the
// receipt saying nobody recorded one.
func TestAModelIsProjectedFromWhatAnsweredAndNeverInvented(t *testing.T) {
	fixture := loadClaudeFixture(t)
	// The real session, which named one.
	model, reported := LatestModel(replayClaude(t, fixture.CapturedSession))
	if !reported || model.Model != "claude-sonnet-5" {
		t.Fatalf("the captured session's model = %#v, reported = %v", model, reported)
	}
	for name, line := range map[string]string{
		"an init naming no model":     `{"type":"system","subtype":"init","claude_code_version":"2.1.251"}`,
		"an init with an empty model": `{"type":"system","subtype":"init","model":""}`,
		"another system subtype":      `{"type":"system","subtype":"status","model":"claude-sonnet-5"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, reported := LatestModel(replayClaudeLines(t, line)); reported {
				t.Fatal("a model was projected from an event that named none")
			}
		})
	}
}

// replayClaude runs a captured or hand-written stream fixture through the
// adapter's own output sink and returns the receipt it produced.
func replayClaude(t *testing.T, name string) []Entry {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("testdata", "claude", name))
	if err != nil {
		t.Fatal(err)
	}
	return replayClaudeBytes(t, src)
}

func replayClaudeLines(t *testing.T, lines ...string) []Entry {
	t.Helper()
	return replayClaudeBytes(t, []byte(strings.Join(lines, "\n")+"\n"))
}

func replayClaudeBytes(t *testing.T, src []byte) []Entry {
	t.Helper()
	invocation, receipt, store := prepareClaudeFixture(t)
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
	if len(quarantined) != 0 {
		t.Fatalf("replayed receipt quarantined its own evidence: %#v", quarantined)
	}
	return entries
}
