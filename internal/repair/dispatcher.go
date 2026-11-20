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

// Assemble builds one repair session's runtime configuration from the
// operator's per-machine configuration.
//
// It reads no keychain, no auth database and no environment variable. The only
// external thing it does is ask the named executable what version it is, which
// the adapter's pin requires and which no configuration key may answer instead:
// a version written down beside the binary is a second place the answer lives,
// and it is the place that is wrong after an upgrade.
func Assemble(ctx context.Context, user config.Repair, session Session) (Runtime, error) {
	return assemble(ctx, user, session, assembly{Block: "repair", Ask: thread.ClaudeRepairAsk, Model: user.Model, Effort: user.Effort})
}

// AssembleRework builds a rework session's runtime configuration: the repair
// runtime, with the ask that says the packet carries earlier attempts whose work
// was not accepted.
//
// It is a third entry point rather than a flag on the first for the reason the
// assessor's is: what differs is the ask, and an ask that arrived as a parameter
// a caller composed could be anything. See docs/decision/0058 and 0086.
func AssembleRework(ctx context.Context, user config.Repair, session Session) (Runtime, error) {
	return assemble(ctx, user, session, assembly{Block: "repair", Ask: thread.ClaudeReworkAsk, Model: user.Model, Effort: user.Effort})
}

// AssembleAssessor builds one adversarial pass's runtime configuration.
//
// It is a second entry point rather than a flag on the first, because what it
// assembles is a different kind of session: a different ask, and a runtime,
// model and effort the operator selected in a block of their own, independently
// of the repairer's.
// Decision 0078 asks for that independence by name, and the reason is the limit
// it exists to get past — two instances of one model can find one substitution
// reasonable, and the answer is a different model or more effort.
//
// A machine with no assessor configured reaches this with an empty block and is
// refused by the same checks an unset repair runtime meets. The caller reports
// that as *no pass is configured* rather than as a repair that failed: nobody
// has set one up is an ordinary state.
func AssembleAssessor(ctx context.Context, user config.Assess, session Session) (Runtime, error) {
	return assemble(ctx, user.Repair, session, assembly{
		Block:  "assess",
		Ask:    thread.ClaudeAssessAsk,
		Model:  user.Model,
		Effort: user.Effort,
	})
}

// assembly is what differs between the kinds of session an operator can
// configure. Block is which configuration block this came from, so a refusal
// names the key a person would actually go and set rather than the other one;
// Model and Effort are that block's selection, and empty is the runtime's own
// default.
type assembly struct {
	Block  string
	Ask    thread.ClaudeAsk
	Model  string
	Effort string
}

func assemble(ctx context.Context, user config.Repair, session Session, kind assembly) (Runtime, error) {
	switch {
	case user.Runtime == "":
		return Runtime{}, &thread.Refusal{Reason: kind.Block + " runtime is unset", Detail: "set " + kind.Block + ".runtime in the operator's config"}
	case user.Executable == "":
		return Runtime{}, &thread.Refusal{Reason: kind.Block + " runtime executable is unset", Detail: user.Runtime}
	case !filepath.IsAbs(user.Executable):
		return Runtime{}, &thread.Refusal{Reason: kind.Block + " runtime executable is not an explicit absolute path", Detail: user.Executable}
	}
	if thread.Runtime(user.Runtime) != thread.Claude {
		// One runtime reaches this today. The others keep their own channels
		// and neither is wired to configuration yet, so a name that would
		// silently assemble the wrong credential shape is refused instead.
		return Runtime{}, &thread.Refusal{Reason: kind.Block + " runtime has no configured credential channel", Detail: user.Runtime}
	}
	return assembleClaude(ctx, user, session, kind)
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

func assembleClaude(ctx context.Context, user config.Repair, session Session, kind assembly) (Runtime, error) {
	if user.Credential == "" {
		return Runtime{}, &thread.Refusal{Reason: kind.Block + " credential reference is unset", Detail: "set " + kind.Block + ".credential in the operator's config"}
	}
	if looksLikeAValue(user.Credential) {
		// The one mistake this file exists to make impossible. A pasted token
		// here would be rendered into a policy file on disk, and the refusal
		// names the shape rather than quoting what was written.
		return Runtime{}, &thread.Refusal{
			Reason: kind.Block + " credential is a value rather than a reference",
			Detail: kind.Block + ".credential must be a scheme://... reference the confinement provider resolves",
		}
	}
	if session.Home == "" || !filepath.IsAbs(session.Home) {
		return Runtime{}, &thread.Refusal{Reason: kind.Block + " session home is not an explicit absolute path", Detail: session.Home}
	}
	if session.Temp == "" || !filepath.IsAbs(session.Temp) {
		return Runtime{}, &thread.Refusal{Reason: kind.Block + " session temporary directory is not an explicit absolute path", Detail: session.Temp}
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
			Versions:   thread.VersionPolicy(user.RuntimeVersions),
			// No token: the provider resolves the reference and the child holds
			// a phantom. This is what the boundary looks like in the type — the
			// field an adapter would read is empty, because the value it would
			// have carried never entered this process.
			ClaudeBrokeredCredential: true,
			ClaudeSetupTokenExpiry:   user.CredentialExpires,
			ClaudeHome:               session.Home,
			ClaudeTemp:               session.Temp,
			// The ask is the caller's, and there are two callers. It is
			// stated here rather than defaulted in the adapter because an
			// adapter with a default ask would launch an unframed session and
			// it would look like a framed one; and it arrives as a parameter
			// from one of two entry points rather than as a field an operator
			// sets, because a channel that can carry anything can carry the
			// instruction decision 0009 keeps out of a packet. See 0058 and
			// 0081.
			ClaudeAsk: kind.Ask,
			// The block's selection, and empty when it names none, which is
			// the runtime's own default. Naming one is a request: the model a
			// receipt projects is still read from the runtime's own
			// system/init, so it stays an answer rather than an echo. See
			// docs/decision/0077 and 0093.
			ClaudeModel:  kind.Model,
			ClaudeEffort: kind.Effort,
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

// replacedRuntime asks a runtime's executable what it is after its session,
// because the file at its path is not the one that was launched. What it
// answers is said as it was answered, and a binary that cannot say is said to
// be that rather than guessed at.
func replacedRuntime(ctx context.Context, launched thread.Executable) string {
	version, err := runtimeVersion(ctx, launched.Path)
	if err != nil {
		return "nothing it can name"
	}
	return version
}
