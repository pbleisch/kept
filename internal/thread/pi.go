package thread

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/pbleisch/kept/internal/ids"
	"github.com/pbleisch/kept/internal/runtimestate"
)

// PiVersion is the Pi RPC version the fixture records. A newer Pi may have a
// different native event meaning, so it is refused until its own evidence has
// been reviewed rather than being treated as compatible by name alone.
// piSupport is the Pi build the fixture records, and it declares no line. A
// newer Pi may give its native events a different meaning, and reviewing one
// costs more than reading a binary: Pi's live proof needs the self-hosted
// endpoint 0052's evidence was measured against. Until somebody pays that, the
// reviewed set is the whole of what this adapter accepts.
var piSupport = support{
	product:  "pi",
	wire:     "rpc:1",
	reviewed: []string{"0.84.0"},
}

var PiVersion = piSupport.newestReviewed()

// piAdapterVersion is what a session on the newest reviewed version stamps its native
// events with; an actual dispatch composes the value from the version that ran.
var piAdapterVersion = piSupport.adapterVersion(PiVersion, true)

const (

	// Admitted. Decision 0052's evidence for this adapter is in
	// internal/confine/piinbox_test.go, against a real Pi in a real box: the
	// documented direct argv and packet-input channel, the credential channel
	// identity, native events and the projections that reference them, a
	// deliberately interrupted session whose partial receipt retains the
	// timeout cause and the reaped kill, and a repair whose verdict is
	// re-derived from the box rather than read from the agent's account.
	// Claude and Codex remain unadmitted; the gate is per adapter.
	piAdmitted = true
)

// piArgs disables every Pi input-discovery surface that could turn the packet
// into configuration or a slash command, and --offline disables the startup
// network operations that would otherwise refresh a model catalogue from a host
// the confinement profile never authorized. Repair tools remain enabled: their
// authority is the externally verified confinement profile, not this adapter.
var piArgs = []string{
	"--mode", "rpc",
	"--no-session",
	"--no-extensions",
	"--no-skills",
	"--no-prompt-templates",
	"--no-context-files",
	"--offline",
}

func piArgv(cfg RuntimeConfig) []string {
	argv := append([]string(nil), piArgs...)
	return append(argv, "--provider", cfg.PiProvider, "--model", cfg.PiModel)
}

// The hazard a refused parent variable represents. It is the Refusal's detail,
// so an operator is told what the name would have done rather than only that it
// was unwelcome.
const (
	piOverridesCredential = "overrides or supplies the credential"
	piNamesConfiguration  = "names configuration kept did not write"
	piRedirectsUpstream   = "redirects or intercepts where the credential is sent"
	piRunsCodeInProcess   = "runs code inside the runtime process"
)

// piRefusedEnvironment is every parent variable that can hand a confined Pi a
// credential kept did not select, point it at configuration kept did not write,
// move where its requests go, or run code inside it. Membership is read off the
// installed 0.84.0 package rather than adapted from the Claude adapter's list,
// and each name carries where it came from: `docs/` is Pi's own bundled
// documentation, `dist/` is its shipped source, and *measured* is a probe run
// against this installation.
//
// **This is defense in depth and Pi was not exposed.** preparePi builds
// Invocation.Env as the complete child environment and it is exactly three
// names, and the credential travels under kept's own KEPT_PI_CREDENTIAL
// precisely so that no provider-native name can win a precedence fight for it.
// What this refuses is the one shape that construction cannot see: a
// confinement provider that merged the parent environment into the child
// instead of replacing it. Under a merge whose parent side wins, every name
// below is live.
//
// **There is no namespace rule here, and that is the deliberate inverse of the
// Claude adapter's.** Refusing `PI_` wholesale would refuse every dispatch made
// from inside a Pi session: docs/environment-variables.md says the CLI and RPC
// entry points export PI_CODING_AGENT to every child, and the bash tool adds
// PI_SESSION_ID, PI_SESSION_FILE, PI_PROVIDER, PI_MODEL and
// PI_REASONING_LEVEL. Those are ambient session state and say nothing about
// which credential is in force, so the family is enumerated instead.
//
// Four exclusions are load-bearing, because a guard that fires on an ordinary
// machine is a guard somebody turns off, and TestPiAdmitsTheDeliberatelyExcludedNames
// holds each one:
//
//   - **The provider-native API-key table.** docs/providers.md's Resolution
//     Order puts the auth.json entry ahead of any environment variable, and
//     dist/core/provider-composer.js resolves `input.credential` before it ever
//     consults ctx.env — pi-ai/dist/providers/anthropic.js is the same shape a
//     step down. runtimestate's projection always writes an auth.json entry for
//     the selected provider, and --provider pins which one is used, so an
//     ambient ANTHROPIC_API_KEY or OPENAI_API_KEY resolves nothing. The
//     third-party stores behind them — AWS_*, AZURE_*,
//     GOOGLE_APPLICATION_CREDENTIALS — are excluded for the same reason the
//     Claude adapter excludes them: they do nothing until a provider that reads
//     them is the selected one, and the projection is what selects it.
//   - **ALL_PROXY and all_proxy.** The Claude adapter refuses both. Pi's
//     dispatcher does not read them: dist/core/http-dispatcher.js installs
//     undici's EnvHttpProxyAgent globally and undici 8.9.0 reads only the http
//     and https pairs. Measured — a dead loopback proxy named in ALL_PROXY left
//     the request reaching its origin, while the same value in HTTP_PROXY
//     diverted it. Two names copied across would have guarded nothing.
//   - **NO_PROXY and no_proxy.** They only exclude hosts from proxying and
//     cannot send a credential anywhere.
//   - **PI_OFFLINE, PI_TELEMETRY, PI_PACKAGE_DIR and
//     PI_CODING_AGENT_SESSION_DIR.** dist/main.js resolves offline as
//     `args.includes("--offline") || isTruthyEnvFlag(process.env.PI_OFFLINE)`,
//     so the pinned argv wins and the variable can only turn offline further on.
//     PI_TELEMETRY adds attribution headers for four public hosts
//     (dist/core/provider-attribution.js) that a projection whose endpoints must
//     be loopback or private can never match, and its only other reader is
//     interactive mode. PI_PACKAGE_DIR relocates assets — themes, package.json,
//     README — and not code (dist/config.js, getPackageDir). And session storage
//     is moot under the pinned --no-session.
var piRefusedEnvironment = map[string]string{
	// kept's own reference target, and the reason it is here is precedence
	// rather than secrecy. The projected auth.json stores "$KEPT_PI_CREDENTIAL"
	// in place of a credential, and dist/core/resolve-config-value.js resolves
	// that reference as `env?.[name] || process.env[name]` — so an inherited
	// value winning a merge authenticates the box with a credential kept never
	// selected, and an inherited *empty* one is falsy there, which drops Pi
	// through to the provider-native names the projection exists to bypass.
	// runtimestate already refuses this name inside the source credential's own
	// env map; this is the same rule from the parent-process side.
	runtimestate.PiCredentialEnvironment: piOverridesCredential,
	// dist/extensions/llama/provider.js resolves the llama.cpp key as
	// `credential?.key ?? (await ctx.env("LLAMA_API_KEY")) ?? "local"`, and
	// ctx.env is process.env (pi-ai/dist/auth/context.js,
	// defaultProviderAuthContext). It is reachable because --no-extensions does
	// not remove it: dist/main.js composes `[...builtInExtensions, ...]`
	// unconditionally, and llama.cpp is the one built-in extension.
	"LLAMA_API_KEY": piOverridesCredential,

	// kept sets this itself, and an inherited one must not be left to win a
	// merge. docs/environment-variables.md: it overrides the config directory,
	// default ~/.pi/agent. Winning it would point the confined session at the
	// operator's real Pi state — the auth.json holding live Claude Pro/Max,
	// ChatGPT and Copilot OAuth tokens that the projection refuses to copy.
	//
	// HOME is set by kept for the same purpose and is deliberately *not* here:
	// it is present in every parent environment on every machine, so refusing
	// it would refuse every dispatch.
	"PI_CODING_AGENT_DIR": piNamesConfiguration,

	// dist/core/http-dispatcher.js builds undici's EnvHttpProxyAgent and calls
	// setGlobalDispatcher plus undici.install(), so this is the dispatcher
	// behind every request Pi makes, credential included. undici 8.9.0's
	// env-http-proxy-agent.js reads `process.env.http_proxy ??
	// process.env.HTTP_PROXY` and the https pair the same way — lowercase
	// first, which Pi's own documentation does not mention at all. All four
	// measured against that dispatcher: each diverted a request to a dead
	// loopback port that otherwise reached its origin.
	"http_proxy":  piRedirectsUpstream,
	"HTTP_PROXY":  piRedirectsUpstream,
	"https_proxy": piRedirectsUpstream,
	"HTTPS_PROXY": piRedirectsUpstream,
	// Trusting an extra certificate authority is what turns a redirect into an
	// interception the child cannot tell from the real upstream. Measured
	// against Pi's own bundled undici and the global fetch it installs: a TLS
	// peer refused as UNABLE_TO_VERIFY_LEAF_SIGNATURE was reached without
	// complaint once this named the signing CA.
	"NODE_EXTRA_CA_CERTS": piRedirectsUpstream,
	// dist/extensions/llama/provider.js resolves the llama.cpp server from
	// `credential?.env?.LLAMA_BASE_URL ?? await ctx.env("LLAMA_BASE_URL")` and
	// returns it as auth.baseUrl, which is where the request goes. A projected
	// provider's baseUrl must be loopback or private; this name is not bound by
	// that and may name any host.
	"LLAMA_BASE_URL": piRedirectsUpstream,

	// Pi is a Node script reached through #!/usr/bin/env node (0055), so this
	// is code execution inside the runtime process before Pi's own entry point
	// — which reads the credential straight out of the child environment and
	// can replace the dispatcher every other name here merely redirects.
	// Measured: NODE_OPTIONS=--require ran a file of this test's choosing
	// inside `pi --version` on the installed 0.84.0.
	"NODE_OPTIONS": piRunsCodeInProcess,
}

// piRefusedParentEnvironment reports the offending variable's name and what it
// would have done. It reads names only and never a value: the value is the
// secret, and a refusal that quoted one would put it in a disposition. Offenders
// are sorted so two hostile names produce the same refusal every run rather than
// whichever the environment happened to list first.
func piRefusedParentEnvironment(environment []string) (string, string, bool) {
	var found []string
	for _, entry := range environment {
		name, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if _, refused := piRefusedEnvironment[name]; refused {
			found = append(found, name)
		}
	}
	if len(found) == 0 {
		return "", "", false
	}
	sort.Strings(found)
	name := found[0]
	return name, piRefusedEnvironment[name], true
}

func checkPiConfig(cfg RuntimeConfig, packet Input) error {
	switch {
	case cfg.Runtime != string(Pi):
		return &Refusal{Reason: "runtime configuration does not name pi", Detail: cfg.Runtime}
	case cfg.Executable == "":
		return &Refusal{Reason: "runtime executable is unset", Detail: string(Pi)}
	case !filepath.IsAbs(cfg.Executable):
		return &Refusal{Reason: "runtime executable is not an explicit absolute path", Detail: cfg.Executable}
	}
	if _, err := piSupport.admit(Pi, cfg.Version, cfg.Versions); err != nil {
		return err
	}
	switch {
	case isBrief(packet):
		// An adversarial pass needs its own declared ask and a read-only box,
		// and this adapter has neither. Refused by name rather than launched
		// with a repair's framing over an assessment's evidence, which is a
		// session that would run and produce nothing anybody could account for.
		// See docs/decision/0081.
		return &Refusal{Reason: "runtime has no adversarial pass", Detail: string(Pi)}
	case isRework(packet):
		// A rework packet needs a framing that says the promise may already
		// hold and the earlier work was not accepted, and this adapter has none.
		// See docs/decision/0086.
		return &Refusal{Reason: "runtime has no rework ask", Detail: string(Pi)}
	case packet.Empty():
		return &Refusal{Reason: "runtime packet is absent", Detail: string(Pi)}
	case cfg.PiProvider == "":
		return &Refusal{Reason: "pi provider is unset", Detail: string(Pi)}
	case piUnsafeArgValue(cfg.PiProvider):
		return &Refusal{Reason: "pi provider is not a direct argv value", Detail: cfg.PiProvider}
	case cfg.PiModel == "":
		return &Refusal{Reason: "pi model is unset", Detail: string(Pi)}
	case piUnsafeArgValue(cfg.PiModel):
		return &Refusal{Reason: "pi model is not a direct argv value", Detail: cfg.PiModel}
	case cfg.PiCredential == "":
		return &Refusal{Reason: "pi credential is unset", Detail: runtimestate.PiCredentialEnvironment}
	case strings.IndexByte(cfg.PiCredential, 0) >= 0:
		return &Refusal{Reason: "pi credential contains NUL", Detail: runtimestate.PiCredentialEnvironment}
	case packetContains(cfg.PiCredential, packet.Bytes()):
		return &Refusal{Reason: "pi credential appears in the packet", Detail: runtimestate.PiCredentialEnvironment}
	case cfg.PiState == nil:
		return &Refusal{Reason: "pi configuration projection is absent", Detail: string(Pi)}
	case cfg.PiState.Dir() == "":
		return &Refusal{Reason: "pi configuration projection is unset", Detail: string(Pi)}
	}

	// The parent environment is read here for one purpose, which is to refuse.
	// Decision 0052 forbids reading it to *obtain* a credential and this never
	// does: no value is read, nothing is copied and nothing reaches the child.
	// Only names are consulted. Invocation.Env is the complete child
	// environment and it is three names, so none of these can reach Pi through
	// this adapter — but a provider that merged instead of replacing would hand
	// it a credential kept did not select, the operator's own Pi state, or an
	// upstream that is not the projected one, and each of those failures looks
	// exactly like success.
	// An exported name is a configured intent whatever its value, so an empty
	// one is refused too rather than reasoned about.
	if name, hazard, refused := piRefusedParentEnvironment(os.Environ()); refused {
		return &Refusal{Reason: name + " is present in the parent environment", Detail: hazard}
	}

	info, err := os.Stat(cfg.Executable)
	if err != nil {
		return &Refusal{Reason: "runtime executable is unavailable", Detail: cfg.Executable}
	}
	if !info.Mode().IsRegular() || info.Mode()&0111 == 0 {
		return &Refusal{Reason: "runtime executable is not executable", Detail: cfg.Executable}
	}
	return checkPiProjection(cfg.PiState.Dir())
}

// checkPiProjection re-reads what the materializer wrote. The projection type
// is already the guarantee that this directory is a copy rather than the
// operator's own Pi state, so this is the second check rather than the first:
// it catches a projection discarded, replaced or opened up between the moment
// it was made and the moment a child would read it.
func checkPiProjection(dir string) error {
	info, err := os.Lstat(dir)
	if err != nil {
		return &Refusal{Reason: "pi configuration projection is unavailable", Detail: dir}
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return &Refusal{Reason: "pi configuration projection is not a directory", Detail: dir}
	}
	if info.Mode().Perm()&0o077 != 0 {
		return &Refusal{Reason: "pi configuration projection is not owner-only", Detail: dir}
	}
	for _, name := range piProjectedFiles {
		path := filepath.Join(dir, name)
		file, err := os.Lstat(path)
		if err != nil {
			return &Refusal{Reason: "pi configuration projection is incomplete", Detail: path}
		}
		if !file.Mode().IsRegular() {
			return &Refusal{Reason: "pi configuration projection entry is not a regular file", Detail: path}
		}
		if file.Mode().Perm()&0o077 != 0 {
			return &Refusal{Reason: "pi configuration projection entry is not owner-only", Detail: path}
		}
	}
	return nil
}

// piProjectedFiles is what a Pi state projection must contain before a child
// may be pointed at it. Pi writes more into the same directory as it runs; the
// adapter requires only what the projection is responsible for.
var piProjectedFiles = []string{"auth.json", "models.json", "settings.json"}

func piUnsafeArgValue(value string) bool {
	return strings.HasPrefix(value, "-") || strings.ContainsAny(value, "\x00\r\n")
}

// preparePi builds the direct RPC invocation without reading the parent
// environment, a keychain, a corpus or a ledger. The configuration the child
// reads was materialized by internal/runtimestate before this point, and the
// credential reaches it through the one child environment value that
// projection refers to. It stays separate from piAdapter.Prepare because the
// admission gate belongs to the adapter: this builds an invocation, and
// deciding whether Pi may be started at all is a different question.
func preparePi(cfg RuntimeConfig, packet Input, receipt AdapterReceipt) (Invocation, error) {
	if err := checkPiConfig(cfg, packet); err != nil {
		return Invocation{}, err
	}
	if receipt == nil {
		return Invocation{}, &Refusal{Reason: "thread adapter receipt is absent", Detail: string(Pi)}
	}
	input, err := piInput(packet)
	if err != nil {
		return Invocation{}, err
	}
	// Both names point at the same private copy. Pi resolves its agent
	// directory from PI_CODING_AGENT_DIR and everything else it looks for
	// under a home from HOME, so a projection that is both is a projection
	// with nothing outside it to fall back to.
	dir := cfg.PiState.Dir()
	stream := &piStream{receipt: receipt, secret: []byte(cfg.PiCredential), prompt: input}
	return Invocation{
		Path: cfg.Executable,
		Args: piArgv(cfg),
		Env: []string{
			"HOME=" + dir,
			"PI_CODING_AGENT_DIR=" + dir,
			runtimestate.PiCredentialEnvironment + "=" + cfg.PiCredential,
		},
		Output:    stream,
		BindInput: stream.bindInput,
		Redact: func(src []byte) ([]byte, Redaction) {
			redacted, redaction := redactPi(src, []byte(cfg.PiCredential))
			return redacted, redaction
		},
	}, nil
}

// piInput sends one and only one documented RPC prompt command. JSON's string
// encoding preserves the immutable packet after unescaping; ids.Mint keeps the
// transport correlation value from becoming a durable, composed identifier.
func piInput(packet Input) ([]byte, error) {
	wire := struct {
		ID      string `json:"id"`
		Type    string `json:"type"`
		Message string `json:"message"`
	}{
		ID:      ids.Mint(),
		Type:    "prompt",
		Message: string(packet.Bytes()),
	}
	encoded, err := json.Marshal(wire)
	if err != nil {
		return nil, fmt.Errorf("encode Pi RPC prompt: %w", err)
	}
	return append(encoded, '\n'), nil
}

// piStream retains every native RPC JSONL event before making the projections
// decision 0052 admits. It receives bytes only; Supervisor alone owns Pi's
// deadline, process-group kill and reaped lifecycle receipt.
type piStream struct {
	receipt AdapterReceipt
	secret  []byte
	prompt  []byte
	pending []byte
	turns   uint64
	closed  bool

	mu          sync.Mutex
	input       io.WriteCloser
	inputClosed bool
}

// bindInput takes the child's input channel and writes the one prompt command
// into it.
//
// Pi's RPC session ends at input EOF. Writing the packet and closing the
// channel truncated a measured run after the user message — exit 0, no
// assistant turn, no tool call, no cost and no turn boundary — which is a
// completed session that did nothing and is exactly the receipt decision 0052's
// evidence contract refuses. So the adapter holds the channel open and closes
// it on Pi's own settled event. See docs/decision/0055.
func (s *piStream) bindInput(input io.WriteCloser) error {
	if input == nil {
		return fmt.Errorf("Pi RPC stdin is absent")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.input != nil {
		return fmt.Errorf("Pi RPC stdin was bound more than once")
	}
	if s.closed {
		return fmt.Errorf("Pi RPC stdin was bound after close")
	}
	s.input = input
	if _, err := input.Write(s.prompt); err != nil {
		return fmt.Errorf("write the Pi RPC prompt: %w", err)
	}
	return nil
}

// closeInput ends the session the way Pi's own protocol ends one. It is
// idempotent, because the settled event is the adapter's signal and the reaped
// child is the launch's, and neither knows about the other.
func (s *piStream) closeInput() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.input == nil || s.inputClosed {
		return nil
	}
	s.inputClosed = true
	if err := s.input.Close(); err != nil {
		return fmt.Errorf("close the Pi RPC input channel: %w", err)
	}
	return nil
}

func (s *piStream) Write(src []byte) (int, error) {
	if s.closed {
		return 0, fmt.Errorf("write Pi RPC stream after close")
	}
	consumed := 0
	for len(src) > 0 {
		at := bytes.IndexByte(src, '\n')
		if at < 0 {
			if err := s.append(src); err != nil {
				return consumed, err
			}
			return consumed + len(src), nil
		}
		if err := s.append(src[:at]); err != nil {
			return consumed, err
		}
		consumed += at + 1
		line := s.pending
		s.pending = nil
		if err := s.record(line, false); err != nil {
			return consumed, err
		}
		src = src[at+1:]
	}
	return consumed, nil
}

// Close retains what the stream ended inside, as Claude's does. See
// docs/decision/0084.
//
// The end of the runtime's output is the end of the session's stream, so the
// receipt is sealed here and a reader finds it whole. See docs/decision/0087.
func (s *piStream) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	if len(bytes.TrimSpace(s.pending)) != 0 {
		if err := s.record(s.pending, true); err != nil {
			return err
		}
		s.pending = nil
	}
	return s.receipt.Seal()
}

func (s *piStream) append(src []byte) error {
	s.pending = append(s.pending, src...)
	return nil
}

func (s *piStream) record(raw []byte, unterminated bool) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		content, redaction := redactPi(raw, s.secret)
		return s.receipt.Undecodable(content, fmt.Sprintf("decode Pi RPC native event: %v", err), unterminated, redaction)
	}
	if envelope.Type == "" {
		content, redaction := redactPi(raw, s.secret)
		return s.receipt.Undecodable(content, "Pi RPC native event has no type", unterminated, redaction)
	}
	redacted, redaction := redactPi(raw, s.secret)
	ref, err := s.receipt.Native(envelope.Type, redacted, redaction)
	if err != nil {
		return err
	}
	return s.project(envelope.Type, raw, ref)
}

func (s *piStream) project(typ string, raw []byte, ref string) error {
	switch typ {
	case "message_update":
		text, ok := piTextDelta(raw)
		if !ok {
			return nil
		}
		content, redaction := redactPi([]byte(text), s.secret)
		_, err := s.receipt.Transcript(ref, content, redaction)
		return err
	case "turn_end":
		if amount, ok := piAssistantCost(raw); ok {
			if err := s.receipt.Cost(ref, "USD", amount); err != nil {
				return err
			}
		}
		s.turns++
		return s.receipt.Turn(ref, s.turns)
	case "agent_settled":
		// Pi's own name for the point after which no retry, compaction retry or
		// queued continuation remains. Ending the session on anything earlier
		// would cut a run Pi was about to continue; ending it on nothing leaves
		// the session running until the supervisor's deadline kills it.
		return s.closeInput()
	}
	return nil
}

func piTextDelta(raw []byte) (string, bool) {
	var event struct {
		AssistantMessageEvent struct {
			Type  string `json:"type"`
			Delta string `json:"delta"`
		} `json:"assistantMessageEvent"`
	}
	if json.Unmarshal(raw, &event) != nil || event.AssistantMessageEvent.Type != "text_delta" || event.AssistantMessageEvent.Delta == "" {
		return "", false
	}
	return event.AssistantMessageEvent.Delta, true
}

func piAssistantCost(raw []byte) (string, bool) {
	var event struct {
		Message struct {
			Role  string `json:"role"`
			Usage struct {
				Cost struct {
					Total json.RawMessage `json:"total"`
				} `json:"cost"`
			} `json:"usage"`
		} `json:"message"`
	}
	if json.Unmarshal(raw, &event) != nil || event.Message.Role != "assistant" {
		return "", false
	}
	return piNumber(event.Message.Usage.Cost.Total)
}

func piNumber(src json.RawMessage) (string, bool) {
	if len(src) == 0 {
		return "", false
	}
	amount := strings.TrimSpace(string(src))
	value, err := strconv.ParseFloat(amount, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return "", false
	}
	return amount, true
}

func packetContains(secret string, packet []byte) bool {
	for _, form := range secretForms([]byte(secret)) {
		if bytes.Contains(packet, form) {
			return true
		}
	}
	return false
}

func redactPi(src, secret []byte) (json.RawMessage, Redaction) {
	redacted := append(json.RawMessage(nil), src...)
	matches := 0
	for _, form := range secretForms(secret) {
		count := bytes.Count(redacted, form)
		if count == 0 {
			continue
		}
		matches += count
		redacted = bytes.ReplaceAll(redacted, form, []byte("[REDACTED]"))
	}
	if matches == 0 {
		return redacted, Redaction{}
	}
	return redacted, Redaction{Occurred: true, Matches: matches}
}

// secretForms is the one definition of how an injected credential can appear in
// a runtime's output: as itself, and as the JSON-escaped string a runtime that
// echoes it would write. Every adapter that refuses a credential in a packet
// and redacts one out of durable evidence reads it from here, so the refusal
// and the redaction cannot come to disagree.
func secretForms(secret []byte) [][]byte {
	if len(secret) == 0 {
		return nil
	}
	forms := [][]byte{secret}
	if quoted, err := json.Marshal(string(secret)); err == nil && len(quoted) >= 2 {
		escaped := quoted[1 : len(quoted)-1]
		if !bytes.Equal(escaped, secret) {
			forms = append(forms, escaped)
		}
	}
	return forms
}
