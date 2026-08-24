package repair

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/pbleisch/kept/internal/config"
	"github.com/pbleisch/kept/internal/confine"
	"github.com/pbleisch/kept/internal/thread"
)

// versionDeadline bounds asking a runtime what version it is. Nothing external
// runs without one. See docs/decision/0026.
const versionDeadline = 30 * time.Second

// Session is what one confined repair needs beyond its configuration: the
// directories the box cut for it, which are per session and never configured.
type Session struct {
	// Home is a newly created, empty directory that is the runtime's HOME and
	// the parent of its isolated configuration directory.
	Home string
	// Temp is the box's own temporary directory.
	Temp string
}

// Runtime is the assembled answer: what the adapter is handed, and what the
// confinement profile must carry for it. They are returned together because
// they are two statements of one fact — the credential variable is published by
// the route and refused in the invocation — and a caller that could get one
// without the other is a caller that can make them disagree.
type Runtime struct {
	Config thread.RuntimeConfig
	// Credential is the brokered route the profile carries, or nil when the
	// runtime's credential travels by its own channel.
	Credential *confine.CredentialRoute
	// Domain is the one host this runtime's credential is answered by. A caller
	// puts it in the profile's allowlist; the confinement package refuses a
	// route whose upstream is not there.
	Domain string
	// TempEnv is the variable name this runtime reads its scratch directory
	// under, for the profile field that both grants the directory and sets it.
	TempEnv []string
	// Env is the child environment allowlist this runtime needs. It never
	// contains the credential variable: under the brokered channel the profile
	// publishes that one, and naming it here as well would give it two sources.
	Env []string
}

// Assemble builds one session's runtime configuration from the operator's
// per-machine configuration.
//
// It reads no keychain, no auth database and no environment variable. The only
// external thing it does is ask the named executable what version it is, which
// the adapter's pin requires and which no configuration key may answer instead:
// a version written down beside the binary is a second place the answer lives,
// and it is the place that is wrong after an upgrade.
func Assemble(ctx context.Context, user config.Repair, session Session) (Runtime, error) {
	switch {
	case user.Runtime == "":
		return Runtime{}, &thread.Refusal{Reason: "repair runtime is unset", Detail: "set repair.runtime in the operator's config"}
	case user.Executable == "":
		return Runtime{}, &thread.Refusal{Reason: "repair runtime executable is unset", Detail: user.Runtime}
	case !filepath.IsAbs(user.Executable):
		return Runtime{}, &thread.Refusal{Reason: "repair runtime executable is not an explicit absolute path", Detail: user.Executable}
	}
	if thread.Runtime(user.Runtime) != thread.Claude {
		// One runtime reaches this today. The others keep their own channels
		// and neither is wired to configuration yet, so a name that would
		// silently assemble the wrong credential shape is refused instead.
		return Runtime{}, &thread.Refusal{Reason: "repair runtime has no configured credential channel", Detail: user.Runtime}
	}
	return assembleClaude(ctx, user, session)
}

// claudeCredentialService is the provider's name for the route, and it is not
// arbitrary: the provider publishes a route's local address as
// <SERVICE>_BASE_URL in the child, so the name has to be the one the runtime
// already reads. Claude Code reads ANTHROPIC_BASE_URL.
const claudeCredentialService = "anthropic"

// ClaudeUpstream is the one host a Claude subscription credential is answered
// by. Explicit configuration in the sense decision 0052 requires: nothing here
// discovers a provider, a model or an endpoint.
const ClaudeUpstream = "https://api.anthropic.com"

// claudeEndpoints is what a complete confined session actually called, and it
// is deliberately no wider. Measured against Claude Code 2.1.241 in a box: one
// endpoint, `POST /v1/messages`, matching with `?beta=true` present. The image
// references three dozen others and no measured session touched one, so
// granting a pattern over them would grant what nothing asked for. A repair
// that reaches past this is refused by the proxy with a 403 that lands in the
// receipt. See docs/decision/0057.
var claudeEndpoints = []confine.CredentialEndpoint{
	{Method: "POST", Path: "/v1/messages"},
}

func assembleClaude(ctx context.Context, user config.Repair, session Session) (Runtime, error) {
	if user.Credential == "" {
		return Runtime{}, &thread.Refusal{Reason: "repair credential reference is unset", Detail: "set repair.credential in the operator's config"}
	}
	if looksLikeAValue(user.Credential) {
		// The one mistake this file exists to make impossible. A pasted token
		// here would be rendered into a policy file on disk, and the refusal
		// names the shape rather than quoting what was written.
		return Runtime{}, &thread.Refusal{
			Reason: "repair credential is a value rather than a reference",
			Detail: "repair.credential must be a scheme://... reference the confinement provider resolves",
		}
	}
	if session.Home == "" || !filepath.IsAbs(session.Home) {
		return Runtime{}, &thread.Refusal{Reason: "repair session home is not an explicit absolute path", Detail: session.Home}
	}
	if session.Temp == "" || !filepath.IsAbs(session.Temp) {
		return Runtime{}, &thread.Refusal{Reason: "repair session temporary directory is not an explicit absolute path", Detail: session.Temp}
	}
	version, err := runtimeVersion(ctx, user.Executable)
	if err != nil {
		return Runtime{}, err
	}
	route := &confine.CredentialRoute{
		Service:   claudeCredentialService,
		Upstream:  ClaudeUpstream,
		Reference: user.Credential,
		Phantom:   thread.ClaudeCredentialEnvironment,
		Header:    "Authorization",
		Format:    "Bearer {}",
		Endpoints: append([]confine.CredentialEndpoint(nil), claudeEndpoints...),
	}
	return Runtime{
		Config: thread.RuntimeConfig{
			Runtime:    string(thread.Claude),
			Executable: user.Executable,
			Version:    version,
			// No token: the provider resolves the reference and the child holds
			// a phantom. This is what the boundary looks like in the type — the
			// field an adapter would read is empty, because the value it would
			// have carried never entered this process.
			ClaudeBrokeredCredential: true,
			ClaudeSetupTokenExpiry:   user.CredentialExpires,
			ClaudeHome:               session.Home,
			ClaudeTemp:               session.Temp,
			// This package assembles a repair, so the ask is the repair one.
			// It is written here rather than defaulted in the adapter because
			// an adapter with a default ask would launch an unframed session
			// and it would look like a framed one; and it is a value rather
			// than a parameter because the second kind of work — an assessor
			// judging whether a repair held — is a different caller with a
			// different ask, not this one configured differently. See
			// docs/decision/0058.
			ClaudeAsk: thread.ClaudeRepairAsk,
		},
		Credential: route,
		Domain:     "api.anthropic.com",
		TempEnv:    []string{thread.ClaudeTempEnvironment},
		Env:        []string{thread.ClaudeConfigEnvironment},
	}, nil
}

// looksLikeAValue is the shape test, and it errs toward refusing. A reference
// is a scheme://... URI; anything without that is either a bare keystore
// account name — which the provider accepts and which cannot be told apart from
// a pasted secret — or a secret.
func looksLikeAValue(reference string) bool {
	scheme, rest, found := strings.Cut(reference, "://")
	if !found || rest == "" {
		return true
	}
	for i := 0; i < len(scheme); i++ {
		c := scheme[i]
		if (c >= 'a' && c <= 'z') || (c == '-' && i > 0) {
			continue
		}
		return true
	}
	return scheme == ""
}

// runtimeVersion asks the executable what it is. The adapter refuses a version
// it was not pinned against, and this is where that question is put — once, per
// dispatch, bounded, against the binary that is about to run rather than
// against a note somebody wrote about it.
func runtimeVersion(ctx context.Context, executable string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, versionDeadline)
	defer cancel()
	command := exec.CommandContext(ctx, executable, "--version")
	output, err := command.Output()
	if ctx.Err() != nil {
		return "", &thread.Refusal{Reason: "repair runtime did not report a version before the deadline", Detail: executable}
	}
	if err != nil {
		return "", &thread.Refusal{Reason: "repair runtime did not report a version", Detail: executable}
	}
	reported, _, _ := strings.Cut(strings.TrimSpace(string(output)), " ")
	if reported == "" {
		return "", &thread.Refusal{Reason: "repair runtime reported an empty version", Detail: executable}
	}
	return reported, nil
}
