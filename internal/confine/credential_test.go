//go:build !windows

package confine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The credential a route names is the one thing that must never appear in
// anything kept writes. It is not in these tests because kept never has it —
// which is the property — so what is asserted instead is that the reference is
// what gets rendered, and that a value put where a reference belongs is refused
// before a policy file exists.
//
// Nothing here resolves a reference, and nothing here should: resolution is the
// provider's and it reaches a real secret. The shape below is no longer a
// plausible-looking string, though. `apple-password://<server>/<account>` was
// measured resolving against a login-keychain *internet* password, with the
// value read at a listener standing in for the upstream, and it is the only
// keystore shape the provider accepts on this field. See docs/nono-surface.md.
const (
	credentialReference = "apple-password://api.anthropic.com/operator"
	credentialSecret    = "sk-ant-oat01-this-would-be-the-real-token"
)

// credentialProfile is a policy-shaped profile and nothing more: these are
// checks that run before a process exists, so no box is cut and no provider is
// started. Every directory is real, because the profile refuses one that is not.
func credentialProfile(t *testing.T) Profile {
	t.Helper()
	root := ungrantedDir(t)
	box := filepath.Join(root, "box")
	scope := filepath.Join(box, "internal")
	temp := filepath.Join(root, "temp")
	home := filepath.Join(root, "home")
	for _, dir := range []string{box, scope, temp, home} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	profile := Profile{
		Nono:          "/usr/bin/true",
		Kept:          filepath.Join(root, "kept"),
		Git:           "/usr/bin/git",
		Box:           box,
		Read:          []string{box},
		Write:         []string{scope},
		Domains:       []string{"api.anthropic.com"},
		Env:           []string{"CLAUDE_CONFIG_DIR"},
		Ledger:        filepath.Join(root, "ledger"),
		Artifacts:     filepath.Join(root, "artifacts"),
		Temp:          temp,
		TempEnv:       []string{"CLAUDE_CODE_TMPDIR"},
		Home:          home,
		ToolPath:      []string{"/usr/bin", "/bin"},
		CanaryTimeout: 60 * time.Second,
	}
	profile.Credential = &CredentialRoute{
		Service:   "anthropic",
		Upstream:  "https://api.anthropic.com",
		Reference: credentialReference,
		Phantom:   "CLAUDE_CODE_OAUTH_TOKEN",
		Header:    "Authorization",
		Format:    "Bearer {}",
		Endpoints: []CredentialEndpoint{{Method: "POST", Path: "/v1/messages"}},
	}
	return profile
}

// TestARenderedProfileCarriesTheReferenceAndNeverAValue is the property the
// whole brokered channel exists for, asserted over the bytes that reach disk.
//
// A rendered profile is written to a file the provider reads, so it is the one
// artifact of a repair that could hold a credential and be read by anything on
// the machine. It holds a reference instead, and this walks the encoded JSON
// rather than the struct, because a field added later would be in the bytes
// before anybody remembered to assert about it.
func TestARenderedProfileCarriesTheReferenceAndNeverAValue(t *testing.T) {
	profile := credentialProfile(t)
	rendered, err := renderNonoProfile(profile, "/usr/bin/true")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(rendered)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), credentialSecret) {
		t.Fatalf("the rendered profile carries a credential value: %s", encoded)
	}
	if !strings.Contains(string(encoded), credentialReference) {
		t.Fatalf("the rendered profile does not carry the reference: %s", encoded)
	}

	// The route is defined and enabled. Defining one without naming it in
	// `credentials` is configuration the provider does nothing with, which its
	// own guide says in as many words.
	if got := rendered.Network.Credentials; len(got) != 1 || got[0] != "anthropic" {
		t.Fatalf("rendered credentials = %v, want the one route enabled", got)
	}
	route, ok := rendered.Network.CustomCredentials["anthropic"]
	if !ok {
		t.Fatalf("the rendered profile defines no route: %s", encoded)
	}
	if route.EnvVar != "CLAUDE_CODE_OAUTH_TOKEN" || route.InjectHeader != "Authorization" || route.Format != "Bearer {}" {
		t.Fatalf("rendered route = %+v", route)
	}
	if len(route.EndpointRules) != 1 || route.EndpointRules[0].Method != "POST" || route.EndpointRules[0].Path != "/v1/messages" {
		t.Fatalf("rendered endpoint rules = %+v, want the one measured endpoint", route.EndpointRules)
	}
	// An empty rule list is the provider's spelling of *every endpoint*, so the
	// list being non-empty is the policy rather than a detail of it.
	if rendered.Network.AllowDomain == nil || rendered.Network.AllowDomain[0] != "api.anthropic.com" {
		t.Fatalf("the route's host is not in the allowlist: %v", rendered.Network.AllowDomain)
	}
}

// TestACredentialRouteIsRefusedBeforeAPolicyFileExists breaks each rule the
// measurements produced and watches it fail. A check that has never fired is
// not a check.
func TestACredentialRouteIsRefusedBeforeAPolicyFileExists(t *testing.T) {
	for _, tc := range []struct {
		name   string
		breaks func(*Profile)
		want   string
	}{
		{"a blocked network", func(p *Profile) {
			// Measured: the provider refuses this pair outright, because
			// credential injection needs its proxy reachable.
			p.BlockNetwork = true
			p.Domains = nil
		}, "blocks the network and carries a brokered credential"},
		{"a host the allowlist does not name", func(p *Profile) {
			p.Domains = []string{"api.openai.com"}
		}, "is not in the domain allowlist"},
		{"a bare account name, which could be a pasted secret", func(p *Profile) {
			p.Credential.Reference = "anthropic_oauth_token"
		}, "cannot be told apart from a secret"},
		{"a credential value where a reference belongs", func(p *Profile) {
			p.Credential.Reference = credentialSecret
		}, "cannot be told apart from a secret"},
		{"env://, which resolves against kept's own process", func(p *Profile) {
			p.Credential.Reference = "env://CLAUDE_CODE_OAUTH_TOKEN"
		}, "kept would hold the secret"},
		{"cmd://, which runs a program nobody reviewed", func(p *Profile) {
			p.Credential.Reference = "cmd://anthropic"
		}, "nothing reviewed that program"},
		{"keyring://, which the provider's own guide shows and its parser refuses", func(p *Profile) {
			p.Credential.Reference = "keyring://kept-claude-setup-token/paul"
		}, "the provider refuses on a credential route"},
		{"keyring:// however it is spelled", func(p *Profile) {
			// Measured: the separator, and the decode the guide's example
			// carries, make no difference — the field rejects the scheme.
			p.Credential.Reference = "keyring://kept-claude-setup-token:paul?decode=go-keyring"
		}, "the provider refuses on a credential route"},
		{"an unencrypted upstream that is not this machine", func(p *Profile) {
			p.Credential.Upstream = "http://api.anthropic.com"
		}, "is http and is not loopback"},
		{"an upstream that is neither http nor https", func(p *Profile) {
			p.Credential.Upstream = "ftp://api.anthropic.com"
		}, "is not https"},
		{"an upstream carrying a path", func(p *Profile) {
			p.Credential.Upstream = "https://api.anthropic.com/v1"
		}, "carries a path"},
		{"no endpoints, which the provider reads as all of them", func(p *Profile) {
			p.Credential.Endpoints = nil
		}, "which the provider reads as every endpoint"},
		{"an endpoint path written with a query", func(p *Profile) {
			p.Credential.Endpoints = []CredentialEndpoint{{Method: "POST", Path: "/v1/messages?beta=true"}}
		}, "endpoint path is invalid"},
		{"a format with no placeholder", func(p *Profile) {
			p.Credential.Format = "Bearer"
		}, "exactly one {} placeholder"},
		{"a phantom variable the allowlist also fills", func(p *Profile) {
			p.Env = append(p.Env, "CLAUDE_CODE_OAUTH_TOKEN")
		}, "which the credential route already publishes"},
		{"a phantom variable the temporary directory also sets", func(p *Profile) {
			p.Credential.Phantom = "CLAUDE_CODE_TMPDIR"
		}, "which the credential route already publishes"},
		{"a phantom variable the provider reserves", func(p *Profile) {
			p.Credential.Phantom = "NONO_PROXY_TOKEN"
		}, "which the provider reserves"},
		{"a service name that cannot become <SERVICE>_BASE_URL", func(p *Profile) {
			p.Credential.Service = "anthropic-api"
		}, "credential service name is invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			profile := credentialProfile(t)
			tc.breaks(&profile)
			_, err := NewNono(profile)
			if err == nil {
				t.Fatal("the profile was accepted")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("refusal = %v, want it to name %q", err, tc.want)
			}
		})
	}
}

// TestAFileReferenceMayNotSitWhereTheChildCanReadIt is the one scheme whose
// target is a path, so it is the one that can put the secret back inside the
// box while still calling itself brokered.
func TestAFileReferenceMayNotSitWhereTheChildCanReadIt(t *testing.T) {
	profile := credentialProfile(t)

	// Outside every grant: accepted, because kept still never reads it.
	profile.Credential.Reference = "file://" + filepath.Join(ungrantedDir(t), "token")
	if _, err := NewNono(profile); err != nil {
		t.Fatalf("a file reference outside every grant was refused: %v", err)
	}

	// Inside the box the repair writes in.
	inside := profile
	route := *profile.Credential
	route.Reference = "file://" + filepath.Join(profile.Box, "token")
	inside.Credential = &route
	if _, err := NewNono(inside); err == nil {
		t.Fatal("a credential file inside the box was accepted")
	} else if !strings.Contains(err.Error(), "inside the box") {
		t.Fatalf("refusal = %v, want it to name the box", err)
	}

	// And inside the session home, which is granted writable and is not the box.
	granted := profile
	homed := *profile.Credential
	homed.Reference = "file://" + filepath.Join(profile.Home, "token")
	granted.Credential = &homed
	if _, err := NewNono(granted); err == nil {
		t.Fatal("a credential file inside a granted path was accepted")
	} else if !strings.Contains(err.Error(), "inside a path the child is granted") {
		t.Fatalf("refusal = %v, want it to name the grant", err)
	}
}

// TestAnInvocationMayNotNameThePhantomVariable is the other half of "one field
// decides it": the profile publishes the phantom, so an adapter that also set
// the credential variable would be the direct channel wearing the brokered
// one's clothes, and which value the child saw would depend on a merge.
func TestAnInvocationMayNotNameThePhantomVariable(t *testing.T) {
	profile := credentialProfile(t)
	if err := validateInvocationEnvironment(profile, []string{"CLAUDE_CONFIG_DIR=/x"}); err != nil {
		t.Fatalf("an ordinary invocation was refused: %v", err)
	}
	err := validateInvocationEnvironment(profile, []string{"CLAUDE_CODE_OAUTH_TOKEN=" + credentialSecret})
	if err == nil {
		t.Fatal("an invocation carrying the credential variable was accepted")
	}
	if !strings.Contains(err.Error(), "which the credential route publishes") {
		t.Fatalf("refusal = %v, want it to name the route", err)
	}
	if strings.Contains(err.Error(), credentialSecret) {
		t.Fatalf("the refusal quoted the value it refused: %v", err)
	}
}
