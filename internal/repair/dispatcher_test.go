package repair

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/config"
	"github.com/pbleisch/kept/internal/confine"
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/run"
	"github.com/pbleisch/kept/internal/thread"
)

// theSecret is what an operator's keychain holds. It appears in this file only
// so that its absence everywhere else can be asserted: nothing in this package
// ever reads a credential, so the way it could arrive is by somebody writing it
// where a reference belongs.
const theSecret = "sk-ant-oat01-01AbCdEfGhIjKlMnOpQrStUvWxYz"

func session(t *testing.T) Session {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	temp := filepath.Join(t.TempDir(), "temp")
	for _, dir := range []string{home, temp} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return Session{Home: home, Temp: temp}
}

// stubRuntime is a runtime that reports a version and nothing else. Assembling
// asks the binary what it is rather than trusting a note beside it, so a test
// of assembly needs something that answers.
func stubRuntime(t *testing.T, version string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "runtime")
	script := "#!/bin/sh\necho '" + version + " (stub)'\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func configured(t *testing.T) config.Repair {
	t.Helper()
	return config.Repair{
		Runtime:           string(thread.Claude),
		Executable:        stubRuntime(t, thread.ClaudeVersion),
		Credential:        "apple-password://api.anthropic.com/operator",
		CredentialExpires: time.Now().Add(300 * 24 * time.Hour),
	}
}

// TestAssembleBuildsABrokeredRuntimeOutsideTheAdapterPackage is the shape
// decision 0052 asks for: configuration becomes a RuntimeConfig here, and the
// adapter package is handed a struct it could not have built for itself.
func TestAssembleBuildsABrokeredRuntimeOutsideTheAdapterPackage(t *testing.T) {
	user := configured(t)
	runtime, err := Assemble(context.Background(), user, session(t))
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if runtime.Config.Runtime != string(thread.Claude) || runtime.Config.Version != thread.ClaudeVersion {
		t.Fatalf("assembled config = %+v", runtime.Config)
	}
	if !runtime.Config.ClaudeBrokeredCredential {
		t.Fatal("the assembled config does not name the brokered channel")
	}
	// The field an adapter would read is empty, because the value it would have
	// carried never entered this process.
	if runtime.Config.ClaudeSetupToken != "" {
		t.Fatalf("the assembled config carries a token: %q", runtime.Config.ClaudeSetupToken)
	}
	if !runtime.Config.ClaudeSetupTokenExpiry.Equal(user.CredentialExpires) {
		t.Fatalf("expiry = %v, want the configured lapse %v", runtime.Config.ClaudeSetupTokenExpiry, user.CredentialExpires)
	}
	if runtime.Credential == nil {
		t.Fatal("the assembled runtime carries no credential route")
	}
	if runtime.Credential.Reference != user.Credential {
		t.Fatalf("route reference = %q, want the configured reference", runtime.Credential.Reference)
	}
	if runtime.Credential.Phantom != thread.ClaudeCredentialEnvironment {
		t.Fatalf("route phantom = %q", runtime.Credential.Phantom)
	}
	// The route's service name is what publishes <SERVICE>_BASE_URL, and the
	// runtime reads ANTHROPIC_BASE_URL, so this one is not free to change.
	if runtime.Credential.Service != "anthropic" {
		t.Fatalf("route service = %q, and the runtime reads ANTHROPIC_BASE_URL", runtime.Credential.Service)
	}
	// The environment allowlist never names the phantom: the profile publishes
	// it, and a name with two sources has one value and two policies.
	for _, name := range append(append([]string(nil), runtime.Env...), runtime.TempEnv...) {
		if name == thread.ClaudeCredentialEnvironment {
			t.Fatalf("the assembled environment names the phantom variable: %q", name)
		}
	}
}

// TestTheAssembledPolicyIsOneThatConfinementAccepts is what keeps this package
// and the confinement package from drifting apart. Every rule the profile
// enforces about a credential route is enforced against what this builds,
// rather than against a route a test wrote by hand.
func TestTheAssembledPolicyIsOneThatConfinementAccepts(t *testing.T) {
	runtime, err := Assemble(context.Background(), configured(t), session(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := confine.CheckCredentialRoute(runtime.Credential, []string{runtime.Domain}); err != nil {
		t.Fatalf("the assembled route is one confinement refuses: %v", err)
	}
	// And the domain the caller is told to allow is the route's own upstream,
	// so a caller cannot put the two in a profile that disagrees with itself.
	if !strings.Contains(runtime.Credential.Upstream, runtime.Domain) {
		t.Fatalf("route upstream %q and declared domain %q are not the same host", runtime.Credential.Upstream, runtime.Domain)
	}
}

// TestACredentialValueWhereAReferenceBelongsIsRefused is the mistake this
// package is placed to prevent. A pasted token would be rendered into a policy
// file on disk, so it is refused at the point configuration is read and the
// refusal never quotes what it refused.
func TestACredentialValueWhereAReferenceBelongsIsRefused(t *testing.T) {
	for _, written := range []string{
		theSecret,
		"anthropic_oauth_token",
		"not a reference at all",
	} {
		user := configured(t)
		user.Credential = written
		_, err := Assemble(context.Background(), user, session(t))
		if err == nil {
			t.Fatalf("a credential written as %q was accepted", written)
		}
		if !strings.Contains(err.Error(), "value rather than a reference") {
			t.Fatalf("refusal for %q = %v, want it to name the shape", written, err)
		}
		if strings.Contains(err.Error(), written) {
			t.Fatalf("the refusal quoted what it refused: %v", err)
		}
	}
}

// TestAssembleRefusesConfigurationItCannotUse breaks each remaining rule and
// watches it fail.
func TestAssembleRefusesConfigurationItCannotUse(t *testing.T) {
	for _, tc := range []struct {
		name   string
		breaks func(*config.Repair, *Session)
		want   string
	}{
		{"no runtime", func(c *config.Repair, _ *Session) { c.Runtime = "" }, "repair runtime is unset"},
		{"a runtime with no configured channel", func(c *config.Repair, _ *Session) {
			c.Runtime = string(thread.Pi)
		}, "has no configured credential channel"},
		{"an executable found on PATH rather than named", func(c *config.Repair, _ *Session) {
			c.Executable = "claude"
		}, "not an explicit absolute path"},
		{"no credential", func(c *config.Repair, _ *Session) { c.Credential = "" }, "credential reference is unset"},
		{"no session home", func(_ *config.Repair, s *Session) { s.Home = "" }, "session home is not an explicit absolute path"},
		{"no session scratch directory", func(_ *config.Repair, s *Session) { s.Temp = "" }, "temporary directory is not an explicit absolute path"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			user := configured(t)
			sess := session(t)
			tc.breaks(&user, &sess)
			_, err := Assemble(context.Background(), user, sess)
			if err == nil {
				t.Fatal("the configuration was accepted")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("refusal = %v, want it to name %q", err, tc.want)
			}
		})
	}
}

// TestTheVersionComesFromTheBinaryRatherThanFromConfiguration is why assembly
// runs a process at all. A version written down beside the binary is a second
// place the answer lives, and it is the place that is wrong after an upgrade.
func TestTheVersionComesFromTheBinaryRatherThanFromConfiguration(t *testing.T) {
	user := configured(t)
	user.Executable = stubRuntime(t, "9.9.9")
	runtime, err := Assemble(context.Background(), user, session(t))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.Config.Version != "9.9.9" {
		t.Fatalf("version = %q, want what the binary reported", runtime.Config.Version)
	}
	// Assembly does not judge the version — the adapter is what refuses one it
	// was not pinned against, and it still does with this config in hand.
	adapter, err := thread.Select(runtime.Config.Runtime)
	if err != nil {
		t.Fatal(err)
	}
	packet, err := thread.AssemblePacket(proofPacket())
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := thread.NewReceipt(writableLedger(t), thread.MintID())
	if err != nil {
		t.Fatal(err)
	}
	adapterReceipt, err := receipt.ForAdapter(thread.Claude, adapter.AdapterVersion())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Prepare(runtime.Config, packet, adapterReceipt); err == nil {
		t.Fatal("an unpinned version was accepted by the adapter")
	} else if !strings.Contains(err.Error(), "version is unsupported") {
		t.Fatalf("adapter refusal = %v, want it to name the version", err)
	}
}

// TestAssembleRefusesARuntimeThatCannotSayWhatItIs bounds the one external
// thing this package does. Nothing external runs without a deadline; a runtime
// that never answers is a refusal rather than a dispatch that hangs.
func TestAssembleRefusesARuntimeThatCannotSayWhatItIs(t *testing.T) {
	user := configured(t)
	user.Executable = filepath.Join(t.TempDir(), "not-there")
	if _, err := Assemble(context.Background(), user, session(t)); err == nil {
		t.Fatal("a runtime that is not there was accepted")
	} else if !strings.Contains(err.Error(), "did not report a version") {
		t.Fatalf("refusal = %v", err)
	}

	silent := filepath.Join(t.TempDir(), "silent")
	if err := os.WriteFile(silent, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	user.Executable = silent
	if _, err := Assemble(context.Background(), user, session(t)); err == nil {
		t.Fatal("a runtime that reported nothing was accepted")
	} else if !strings.Contains(err.Error(), "empty version") {
		t.Fatalf("refusal = %v", err)
	}
}

// TestTheCredentialValueReachesNothingThisPackageProduces holds the whole
// channel to its claim, against a secret that really exists.
//
// The value is written to a file standing in for the operator's keychain item,
// which is what gives the assertion teeth: a test that asserts the absence of a
// string nothing ever held would pass against a broken implementation. Here the
// bytes are on disk, the reference names them, and nothing kept assembles
// contains them — because resolving the reference is the provider's job and
// this process never does it.
func TestTheCredentialValueReachesNothingThisPackageProduces(t *testing.T) {
	held := filepath.Join(t.TempDir(), "operator-credential")
	if err := os.WriteFile(held, []byte(theSecret), 0o600); err != nil {
		t.Fatal(err)
	}
	user := configured(t)
	user.Credential = "file://" + held

	runtime, err := Assemble(context.Background(), user, session(t))
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	encoded, err := json.Marshal(runtime)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), theSecret) {
		t.Fatalf("the assembled runtime carries the credential value: %s", encoded)
	}
	// The reference is there, which is what says the absence above is the
	// value's rather than the whole route's.
	if !strings.Contains(string(encoded), held) {
		t.Fatalf("the assembled runtime does not carry the reference: %s", encoded)
	}
	// And the file was never opened: the reference resolves in the provider,
	// not here. Nothing in this package reads it, so the only proof available
	// is that the bytes are absent from everything it produced — which is what
	// the check above is.
	if runtime.Config.ClaudeSetupToken != "" {
		t.Fatal("assembly filled the direct channel's token field")
	}
}

func writableLedger(t *testing.T) *ledger.Ledger {
	t.Helper()
	return ledger.Open(ledger.NewFiles(t.TempDir()), run.Start("repair-dispatcher-test", nil))
}

// proofPacket is a packet shaped like the one a repair carries. It exists so
// the adapter can be asked for its answer; nothing here is about its contents.
func proofPacket() thread.PacketInput {
	return thread.PacketInput{
		Promise: thread.RatifiedPromise{
			ID:          "promise:the-corpus-cannot-write-standing-or-truth",
			Hash:        "sha256:dispatcher-proof",
			Norm:        1,
			Ratified:    true,
			Statement:   "the corpus cannot write standing or truth",
			Gather:      map[string]model.GatherSpec{"imports": {Gatherer: "go/imports"}},
			HoldWhen:    "obs.imports.ok",
			Trigger:     model.TriggerCadence,
			RepairScope: []string{"internal/**"},
		},
		Why: "a dispatcher assembled this configuration and the adapter judges it.",
		Observation: ledger.Observation{
			V:        ledger.ObsVersion,
			At:       time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC),
			Gatherer: "go/imports@1",
			World:    model.World{Tree: "sha256:dispatcher-proof-world"},
			Scope:    "sha256:dispatcher-proof-scope",
			Data:     map[string]any{"ok": false},
		},
	}
}

// TestTheAdaptersFixtureRecordsTheRouteThisPackageBuilds keeps decision 0052's
// broker-route record honest across the package boundary.
//
// The route's upstream and its method-and-path allowlist are decided here and
// written down in the adapter's fixture, which is the artifact 0052 admits a
// runtime on. Two places, one fact — so this is the test that stops them
// drifting apart, and it belongs here because this is the side that decides.
func TestTheAdaptersFixtureRecordsTheRouteThisPackageBuilds(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "thread", "testdata", "claude", "fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Brokered struct {
			Upstream  string   `json:"upstream"`
			Endpoints []string `json:"endpoints"`
		} `json:"brokered_credential"`
	}
	if err := json.Unmarshal(src, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Brokered.Upstream != ClaudeUpstream {
		t.Fatalf("the fixture records upstream %q and this package builds %q", fixture.Brokered.Upstream, ClaudeUpstream)
	}
	var built []string
	for _, endpoint := range claudeEndpoints {
		built = append(built, endpoint.Method+" "+endpoint.Path)
	}
	if !slices.Equal(fixture.Brokered.Endpoints, built) {
		t.Fatalf("the fixture records endpoints %v and this package forwards %v", fixture.Brokered.Endpoints, built)
	}
}
