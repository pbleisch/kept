package thread

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ClaudeVersion is the Claude Code version whose direct stream shape and
// documented credential precedence this adapter understands. A different
// installed version is a refusal, not a best-effort invocation whose evidence
// might mean something else.
//
// It moved from 2.1.240 to 2.1.241 against the installed binary rather than by
// assumption: every name in the refused family below is still present in it,
// `setup-token` still takes no options and `auth` still has no revoke, and the
// direct argv still resolves — with one correction the earlier pin never
// measured, recorded on claudeArgs.
const ClaudeVersion = "2.1.241"

const (
	claudeAdapterVersion = "claude-code:2.1.241/stream-json:1"

	// This stays false until the decision-0052 confined, interrupted live
	// session has been recorded. The parser and fixture below are deliberately
	// testable before that evidence exists; neither turns a fixture into an
	// admitted runtime.
	claudeAdmitted = false
)

const (
	// claudeCredentialEnvironment is the one named child environment value
	// decision 0052 admits for this adapter: the long-lived subscription token
	// `claude setup-token` mints. Decision 0017 accepts the exposure it carries
	// — a tool-capable agent in the box can read its own credential — because
	// the alternative on macOS is a keychain grant, which that decision refuses
	// outright. The brokered route 0052 also permits is a separate channel and
	// is not built here.
	claudeCredentialEnvironment = "CLAUDE_CODE_OAUTH_TOKEN"

	// claudeAPIKeyEnvironment is never written by this adapter. It is named
	// only so its presence can be refused: Claude Code prefers an API key over
	// the subscription token, so one reaching this child would silently bill at
	// API rates instead of drawing on the subscription that was configured. It
	// is one member of the family below rather than a special case.
	claudeAPIKeyEnvironment = "ANTHROPIC_API_KEY"

	// claudeConfigEnvironment relocates every Claude configuration read into
	// the session's own empty directory, so nothing the operator authored is
	// reachable from the box.
	claudeConfigEnvironment = "CLAUDE_CONFIG_DIR"

	// claudeTempEnvironment is where Claude Code puts its scratch tier, and it
	// is not TMPDIR. `gme()` in the installed binary reads CLAUDE_CODE_TMPDIR
	// and otherwise returns the string "/tmp" — TMPDIR is never consulted — so
	// a confined session with this unset opens /tmp/claude-<uid>, which a
	// repair profile denies by name because it is a host scratch root. Measured
	// from inside a real box: `EPERM: operation not permitted, open
	// '/tmp/claude-501'`, exit 1, and not one stream event. The binary's own
	// guard even names the remedy — "Set CLAUDE_CODE_TMPDIR to a directory you
	// control". The value is the box's own temporary directory, which the
	// confinement profile grants and declares in one field. See 0056.
	claudeTempEnvironment = "CLAUDE_CODE_TMPDIR"
)

// The three names above, exported for the one caller outside this package that
// has to agree with them: a dispatcher renders a confinement profile that
// grants the scratch directory, publishes the credential phantom and allows the
// configuration directory, and those are the same facts as the child
// environment this adapter builds. Two spellings of one fact is the failure
// every one of them was written to prevent, so there is one spelling and it
// lives here. The values are not exported; nothing outside needs a credential.
const (
	ClaudeCredentialEnvironment = claudeCredentialEnvironment
	ClaudeConfigEnvironment     = claudeConfigEnvironment
	ClaudeTempEnvironment       = claudeTempEnvironment
)

// The hazard a refused parent variable represents. It is the Refusal's detail,
// so an operator is told what the name would have done rather than only that it
// was unwelcome.
const (
	claudeOverridesCredential = "overrides or supplies the credential"
	claudeCarriesAuthHelper   = "names configuration that can carry an auth helper"
	claudeRedirectsUpstream   = "redirects where the credential is sent"
	claudeSwitchesProvider    = "switches to a provider with its own credentials"
)

// claudeRefusedNamespace is refused whole, every name under it, rather than by
// enumeration. It is the vendor's provider-configuration namespace and it holds
// the entire first-party credential surface — ANTHROPIC_API_KEY and
// ANTHROPIC_AUTH_TOKEN, ANTHROPIC_BASE_URL and ANTHROPIC_API_HOST,
// ANTHROPIC_CUSTOM_HEADERS, ANTHROPIC_UNIX_SOCKET, ANTHROPIC_IDENTITY_TOKEN and
// its _FILE, the ANTHROPIC_FEDERATION_RULE_ID / ANTHROPIC_ORGANIZATION_ID
// workload-identity pair, ANTHROPIC_CONFIG_DIR, and every third-party base URL
// and key. Enumerating that is a list that rots on the next release; refusing
// the namespace covers a name Anthropic has not shipped yet.
//
// It is safe to refuse wholesale because it is configuration rather than
// ambient session state: a live Claude Code session exports none of it, so this
// cannot fire merely because kept was started from one. The CLAUDE_ family
// below is the opposite — a running session exports a dozen of those names —
// which is why that half is enumerated and this half is not.
const claudeRefusedNamespace = "ANTHROPIC_"

// claudeRefusedEnvironment is every remaining parent variable that can override
// the subscription credential, hand Claude a different one, redirect where it
// is sent, or switch it onto a provider billed some other way. The membership
// is read off the installed 2.1.241 binary rather than guessed: its own
// auth-state report enumerates ANTHROPIC_API_KEY, ANTHROPIC_AUTH_TOKEN,
// apiKeyHelper, CLAUDE_CODE_API_KEY_FILE_DESCRIPTOR, CLAUDE_CODE_OAUTH_TOKEN
// and ANTHROPIC_UNIX_SOCKET as the sources it considers, and its
// strictPolicyHelperKeys set names apiKeyHelper, proxyAuthHelper,
// awsCredentialExport, awsAuthRefresh, gcpAuthRefresh and processWrapper as
// settings that resolve a credential by running a program.
//
// Two exclusions are deliberate, because a guard that fires on an ordinary
// machine is a guard somebody turns off. A third-party credential store —
// AWS_*, AZURE_*, GOOGLE_APPLICATION_CREDENTIALS — does nothing until one of
// the provider switches below is on, so the switch is refused and the store is
// not. And NO_PROXY only excludes hosts from proxying; it cannot send the
// credential anywhere, so it is not a redirect.
var claudeRefusedEnvironment = map[string]string{
	// Carries a credential, or says where to read one.
	"CLAUDE_CODE_API_KEY_FILE_DESCRIPTOR":     claudeOverridesCredential,
	"CLAUDE_CODE_OAUTH_TOKEN_FILE_DESCRIPTOR": claudeOverridesCredential,
	"CLAUDE_CODE_OAUTH_REFRESH_TOKEN":         claudeOverridesCredential,
	"CLAUDE_CODE_HOST_AUTH_ENV_VAR":           claudeOverridesCredential,
	"CLAUDE_CODE_HOST_CREDS_FILE":             claudeOverridesCredential,
	"CLAUDE_API_KEY":                          claudeOverridesCredential,
	"CLAUDE_BRIDGE_OAUTH_TOKEN":               claudeOverridesCredential,
	"CLAUDE_TRUSTED_DEVICE_TOKEN":             claudeOverridesCredential,
	"CLAUDE_SESSION_INGRESS_TOKEN_FILE":       claudeOverridesCredential,
	"CLAUDE_BG_AUTH_SNAPSHOT_PATH":            claudeOverridesCredential,
	"CLAUDE_BG_SOCKET_TOKENS_PATH":            claudeOverridesCredential,
	"CCR_OAUTH_TOKEN_FILE":                    claudeOverridesCredential,
	// A git credential-helper password rather than an inference credential:
	// the binary builds a helper script around it. 0017's rule that no
	// credential in the box reaches a remote is what refuses this one.
	"CLAUDE_CODE_SESSION_ACCESS_TOKEN": claudeOverridesCredential,

	// Names settings or an environment file kept did not write. --safe-mode
	// disables customization but says admin-managed policy settings still
	// apply, and a policy settings file is exactly where apiKeyHelper and
	// proxyAuthHelper live — so this is the route --safe-mode leaves open.
	"CLAUDE_CODE_MANAGED_SETTINGS_PATH": claudeCarriesAuthHelper,
	"CLAUDE_CODE_REMOTE_SETTINGS_PATH":  claudeCarriesAuthHelper,
	"CLAUDE_SECURESTORAGE_CONFIG_DIR":   claudeCarriesAuthHelper,
	"CLAUDE_ENV_FILE":                   claudeCarriesAuthHelper,
	// kept sets this itself; an inherited one must not be left to win a merge.
	claudeConfigEnvironment: claudeCarriesAuthHelper,

	// Sends the credential somewhere other than the configured upstream.
	"CLAUDE_CODE_API_BASE_URL":             claudeRedirectsUpstream,
	"CLAUDE_CODE_CUSTOM_OAUTH_URL":         claudeRedirectsUpstream,
	"CLAUDE_CODE_PROXY_URL":                claudeRedirectsUpstream,
	"CLAUDE_CODE_PROXY_HOST":               claudeRedirectsUpstream,
	"CLAUDE_CODE_HTTP_PROXY":               claudeRedirectsUpstream,
	"CLAUDE_CODE_HTTPS_PROXY":              claudeRedirectsUpstream,
	"CLAUDE_CODE_ENABLE_PROXY_AUTH_HELPER": claudeRedirectsUpstream,
	"CLAUDE_AI_AUTHORIZE_URL":              claudeRedirectsUpstream,
	"CLAUDE_LOCAL_OAUTH_API_BASE":          claudeRedirectsUpstream,
	"CLAUDE_BRIDGE_BASE_URL":               claudeRedirectsUpstream,
	"HTTP_PROXY":                           claudeRedirectsUpstream,
	"HTTPS_PROXY":                          claudeRedirectsUpstream,
	"ALL_PROXY":                            claudeRedirectsUpstream,
	"http_proxy":                           claudeRedirectsUpstream,
	"https_proxy":                          claudeRedirectsUpstream,
	"all_proxy":                            claudeRedirectsUpstream,
	// Trusting an extra certificate authority is what turns a redirect into an
	// interception the child cannot tell from the real upstream.
	"NODE_EXTRA_CA_CERTS": claudeRedirectsUpstream,

	// Authenticates and bills somewhere other than the operator's
	// subscription, which is the whole reason this channel was chosen.
	"CLAUDE_CODE_USE_BEDROCK":                claudeSwitchesProvider,
	"CLAUDE_CODE_USE_VERTEX":                 claudeSwitchesProvider,
	"CLAUDE_CODE_USE_FOUNDRY":                claudeSwitchesProvider,
	"CLAUDE_CODE_USE_MANTLE":                 claudeSwitchesProvider,
	"CLAUDE_CODE_USE_GATEWAY":                claudeSwitchesProvider,
	"CLAUDE_CODE_USE_CCR_V2":                 claudeSwitchesProvider,
	"CLAUDE_CODE_USE_ANTHROPIC_AWS":          claudeSwitchesProvider,
	"CLAUDE_CODE_USE_ANTHROPIC_GOOGLE_CLOUD": claudeSwitchesProvider,
}

// claudeRefusedParentEnvironment reports the offending variable's name and what
// it would have done. It reads names only and never a value: the value is the
// secret, and a refusal that quoted one would put it in a disposition. Offenders
// are sorted so two hostile names produce the same refusal every run rather than
// whichever the environment happened to list first.
func claudeRefusedParentEnvironment(environment []string) (string, string, bool) {
	var found []string
	for _, entry := range environment {
		name, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if strings.HasPrefix(name, claudeRefusedNamespace) {
			found = append(found, name)
			continue
		}
		if _, refused := claudeRefusedEnvironment[name]; refused {
			found = append(found, name)
		}
	}
	if len(found) == 0 {
		return "", "", false
	}
	sort.Strings(found)
	name := found[0]
	hazard, ok := claudeRefusedEnvironment[name]
	if !ok {
		hazard = claudeOverridesCredential
	}
	return name, hazard, true
}

// claudeSetupTokenMargin is how much life a setup token must have left before
// a repair may start on it. `claude setup-token` accepts no options, prints no
// expiry and has no revoke command, so the lifetime is neither selectable nor
// readable back, and the only way to learn that a token died is to spend a
// request against it — which is a live authenticated call the adapter must not
// make. So the operator states when it lapses and this margin decides how early
// the refusal lands. A day is longer than any repair deadline a caller can
// sensibly configure under 0026, so a dispatch that passes this check cannot
// reach the lapse while it is still running; and it costs a one-year token
// nothing that matters.
const claudeSetupTokenMargin = 24 * time.Hour

// claudeArgs is the documented noninteractive stream mode for Claude Code
// 2.1.240.
//
// --bare is deliberately absent and must not come back. Its own help says
// Anthropic auth under it is strictly ANTHROPIC_API_KEY or an apiKeyHelper and
// that OAuth is never read, so it cannot authenticate the subscription token
// this adapter now carries — the flag and the credential channel are mutually
// exclusive. --safe-mode replaces the half of --bare that was actually wanted:
// it disables CLAUDE.md, skills, plugins, hooks, MCP servers, custom commands,
// agents and the rest of the customization surface, so the packet over stdin
// stays the only instruction the session receives, while auth, built-in tools
// and permissions keep working.
//
// --bare also skipped keychain reads, and dropping it means the process may
// attempt one. That is confinement's answer and not an adapter flag's: 0017
// grants no keychain, ever, so the attempt is refused by the profile the canary
// verified — and a flag that could be forgotten was never the thing standing
// between the box and the operator's login. The injected token takes precedence
// over a stored credential in any case.
//
// --include-partial-messages is what makes the transcript incremental rather
// than a completion-only summary, which 0052 refuses as evidence. Permission
// prompts are bypassed only inside the already verified external confinement
// provider; no adapter creates or relaxes that provider.
//
// --verbose is not decoration and is not optional: `--print` with
// `--output-format stream-json` refuses to start without it. Measured, because
// the flag list this adapter was first written from came out of `--help` and
// was never launched — a confined session exited 1 with `Error: When using
// --print, --output-format=stream-json requires --verbose` and no session at
// all. A pinned argv that cannot start is a fixture, not an invocation.
//
// And --safe-mode is load-bearing in a repair box rather than merely tidy. The
// tree under repair is somebody's repository and may carry its own
// .claude/settings.json; measured both ways in a real box, a SessionStart hook
// planted there runs without the flag and does not run with it. Confinement
// still bounds what such a command could touch — 0017 — but the flag is what
// keeps the tree being repaired from choosing what runs at all. What it does
// not disable is the runtime's own built-in skills, which are in the image:
// measured identical with and without it, and named here because the flag's own
// help says otherwise.
//
// --replay-user-messages is what makes the packet's arrival evidence rather
// than inference. The runtime re-emits the user message it read from stdin as a
// native `user` event carrying `isReplay`, so the receipt holds the runtime's
// own account of the bytes it received — measured in a confined box. Without
// it, "the packet arrived" can only be argued from a marker turning up in an
// assistant turn, and the session decision 0052 cares most about is the one
// killed before any assistant turn exists. The packet is already stored as its
// own artifact before launch, so this is a second, independent record of the
// same bytes and never the first.
var claudeArgs = []string{
	"--safe-mode",
	"--print",
	"--verbose",
	"--input-format", "stream-json",
	"--output-format", "stream-json",
	"--include-partial-messages",
	"--replay-user-messages",
	"--no-session-persistence",
	"--permission-mode", "bypassPermissions",
}

func checkClaudeConfig(cfg RuntimeConfig, packet Packet) error {
	switch {
	case cfg.Runtime != string(Claude):
		return &Refusal{Reason: "runtime configuration does not name claude", Detail: cfg.Runtime}
	case cfg.Executable == "":
		return &Refusal{Reason: "runtime executable is unset", Detail: string(Claude)}
	case !filepath.IsAbs(cfg.Executable):
		return &Refusal{Reason: "runtime executable is not an explicit absolute path", Detail: cfg.Executable}
	case cfg.Version == "":
		return &Refusal{Reason: "runtime version is unset", Detail: string(Claude)}
	case cfg.Version != ClaudeVersion:
		return &Refusal{Reason: "runtime version is unsupported", Detail: "claude requires " + ClaudeVersion}
	case packet.Empty():
		return &Refusal{Reason: "runtime packet is absent", Detail: string(Claude)}
	case cfg.ClaudeBrokeredCredential && cfg.ClaudeSetupToken != "":
		// Two channels in force is one channel too many, and the failure is
		// silent in the direction that matters: the provider publishes the
		// phantom, this adapter would publish the real token, and whichever won
		// the merge would decide whether the box holds a credential. Decision
		// 0057 makes them exclusive here and the confinement launch refuses the
		// same collision from the other side.
		return &Refusal{Reason: "claude credential is both brokered and direct", Detail: string(Claude)}
	case !cfg.ClaudeBrokeredCredential && cfg.ClaudeSetupToken == "":
		return &Refusal{Reason: claudeCredentialEnvironment + " is unset", Detail: string(Claude)}
	case strings.IndexByte(cfg.ClaudeSetupToken, 0) >= 0:
		return &Refusal{Reason: claudeCredentialEnvironment + " contains NUL", Detail: string(Claude)}
	case cfg.ClaudeSetupToken != "" && packetContains(cfg.ClaudeSetupToken, packet.Bytes()):
		return &Refusal{Reason: claudeCredentialEnvironment + " appears in the packet", Detail: string(Claude)}
	case cfg.ClaudeHome == "":
		return &Refusal{Reason: "claude HOME is unset", Detail: string(Claude)}
	case !filepath.IsAbs(cfg.ClaudeHome):
		return &Refusal{Reason: "claude HOME is not an explicit absolute path", Detail: cfg.ClaudeHome}
	case cfg.ClaudeTemp == "":
		return &Refusal{Reason: claudeTempEnvironment + " is unset", Detail: string(Claude)}
	case !filepath.IsAbs(cfg.ClaudeTemp):
		return &Refusal{Reason: claudeTempEnvironment + " is not an explicit absolute path", Detail: cfg.ClaudeTemp}
	case cfg.ClaudeTemp == cfg.ClaudeHome:
		// Two directories discarded on different schedules and holding
		// different things. Collapsing them would put the session's scratch
		// tier inside the state directory whose emptiness is checked below.
		return &Refusal{Reason: claudeTempEnvironment + " is the session home", Detail: cfg.ClaudeTemp}
	}
	if err := checkClaudeSetupTokenLife(cfg.ClaudeSetupTokenExpiry, time.Now()); err != nil {
		return err
	}

	// The parent environment is read here for one purpose, which is to refuse.
	// Decision 0052 forbids reading it to *obtain* a credential and this never
	// does: no value is read, nothing is copied and nothing reaches the child.
	// Only names are consulted. Invocation.Env is the complete child
	// environment rather than an addition to this process's, so none of these
	// can reach Claude through this adapter — but a provider that merged
	// instead of replacing would hand it a credential it prefers, or an
	// upstream that is not the one configured, and that failure looks exactly
	// like success while billing or disclosing somewhere else.
	// An exported name is a configured intent whatever its value, so an empty
	// one is refused too rather than reasoned about.
	if name, hazard, refused := claudeRefusedParentEnvironment(os.Environ()); refused {
		return &Refusal{Reason: name + " is present in the parent environment", Detail: hazard}
	}

	info, err := os.Stat(cfg.ClaudeHome)
	if err != nil {
		return &Refusal{Reason: "claude HOME is unavailable", Detail: cfg.ClaudeHome}
	}
	if !info.IsDir() {
		return &Refusal{Reason: "claude HOME is not a directory", Detail: cfg.ClaudeHome}
	}
	entries, err := os.ReadDir(cfg.ClaudeHome)
	if err != nil {
		return &Refusal{Reason: "claude HOME cannot be checked", Detail: cfg.ClaudeHome}
	}
	if len(entries) != 0 {
		return &Refusal{Reason: "claude HOME is not clean", Detail: cfg.ClaudeHome}
	}
	// The scratch directory is not required to be empty — it is the box's own
	// temporary directory and a toolchain may already have used it — but a
	// grant naming something that is not there is a boundary that is not one,
	// and the failure it produces arrives from inside a box as a runtime that
	// could not make its own scratch tier.
	temp, err := os.Stat(cfg.ClaudeTemp)
	if err != nil {
		return &Refusal{Reason: claudeTempEnvironment + " is unavailable", Detail: cfg.ClaudeTemp}
	}
	if !temp.IsDir() {
		return &Refusal{Reason: claudeTempEnvironment + " is not a directory", Detail: cfg.ClaudeTemp}
	}
	return nil
}

// checkClaudeSetupTokenLife refuses a dispatch that could outlive its
// credential. Lapsed and lapsing-soon are kept apart because they tell the
// operator different things: one says mint a replacement now, the other says
// there is still a window. Both refuse, and neither is discovered by asking
// the provider — no live call is made to find out.
func checkClaudeSetupTokenLife(expiry, now time.Time) error {
	switch {
	case expiry.IsZero():
		return &Refusal{Reason: "claude setup-token expiry is unset", Detail: string(Claude)}
	case !now.Before(expiry):
		return &Refusal{Reason: "claude setup-token has lapsed", Detail: expiry.Format(time.RFC3339)}
	case expiry.Sub(now) < claudeSetupTokenMargin:
		return &Refusal{Reason: "claude setup-token lapses inside the dispatch margin", Detail: expiry.Format(time.RFC3339)}
	}
	return nil
}

// prepareClaude constructs the admitted invocation. The child environment is
// built entirely from explicit configuration: nothing is copied from this
// process's environment, the operator's home, a keychain or an auth file. It is
// separate from claudeAdapter.Prepare solely so the fixture can exercise all
// safe code while claudeAdmitted keeps actual dispatch closed.
func prepareClaude(cfg RuntimeConfig, packet Packet, receipt AdapterReceipt) (Invocation, error) {
	if err := checkClaudeConfig(cfg, packet); err != nil {
		return Invocation{}, err
	}
	if receipt == nil {
		return Invocation{}, &Refusal{Reason: "thread adapter receipt is absent", Detail: string(Claude)}
	}
	input, err := claudeInput(packet)
	if err != nil {
		return Invocation{}, err
	}
	environment := []string{
		"HOME=" + cfg.ClaudeHome,
		claudeConfigEnvironment + "=" + filepath.Join(cfg.ClaudeHome, "config"),
		claudeTempEnvironment + "=" + cfg.ClaudeTemp,
	}
	// Under the brokered channel the credential variable is the profile's, and
	// naming it here is refused by the confinement launch. Under the direct one
	// it is this adapter's, and the value it carries is what the stream redacts.
	if !cfg.ClaudeBrokeredCredential {
		environment = append(environment, claudeCredentialEnvironment+"="+cfg.ClaudeSetupToken)
	}
	return Invocation{
		Path:   cfg.Executable,
		Args:   append([]string(nil), claudeArgs...),
		Input:  input,
		Env:    environment,
		Output: &claudeStream{receipt: receipt, secret: []byte(cfg.ClaudeSetupToken)},
	}, nil
}

// claudeInput puts exactly one immutable packet byte sequence in Claude's
// documented stdin JSONL user-message channel. The packet is the text value;
// no prompt prefix or rebuilt corpus data can alter what Claude receives.
func claudeInput(packet Packet) ([]byte, error) {
	wire := struct {
		Type    string `json:"type"`
		Message struct {
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"message"`
	}{Type: "user"}
	wire.Message.Role = "user"
	wire.Message.Content = []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}{{Type: "text", Text: string(packet.Bytes())}}
	encoded, err := json.Marshal(wire)
	if err != nil {
		return nil, fmt.Errorf("encode Claude stream input: %w", err)
	}
	return append(encoded, '\n'), nil
}

// claudeStream retains each native JSONL item before making the small common
// projections decision 0052 permits. It is intentionally an output sink, not
// a process handle: timeout, kill and reaping stay with Supervisor.
type claudeStream struct {
	receipt       AdapterReceipt
	secret        []byte
	pending       []byte
	turns         uint64
	sawStreamText bool
	closed        bool
}

func (s *claudeStream) Write(src []byte) (int, error) {
	if s.closed {
		return 0, fmt.Errorf("write Claude stream after close")
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
		if err := s.record(line); err != nil {
			return consumed, err
		}
		src = src[at+1:]
	}
	return consumed, nil
}

func (s *claudeStream) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	if len(bytes.TrimSpace(s.pending)) == 0 {
		return nil
	}
	if err := s.record(s.pending); err != nil {
		return err
	}
	s.pending = nil
	return nil
}

func (s *claudeStream) append(src []byte) error {
	s.pending = append(s.pending, src...)
	return nil
}

func (s *claudeStream) record(raw []byte) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("decode Claude native stream event: %w", err)
	}
	if envelope.Type == "" {
		return fmt.Errorf("Claude native stream event has no type")
	}
	redacted, redaction := redactClaude(raw, s.secret)
	ref, err := s.receipt.Native(envelope.Type, redacted, redaction)
	if err != nil {
		return err
	}
	return s.project(envelope.Type, raw, ref)
}

func (s *claudeStream) project(typ string, raw []byte, ref string) error {
	switch typ {
	case "stream_event":
		text, ok := claudeDeltaText(raw)
		if !ok {
			return nil
		}
		s.sawStreamText = true
		content, redaction := redactClaude([]byte(text), s.secret)
		_, err := s.receipt.Transcript(ref, content, redaction)
		return err
	case "assistant":
		if !s.sawStreamText {
			for _, text := range claudeAssistantText(raw) {
				content, redaction := redactClaude([]byte(text), s.secret)
				if _, err := s.receipt.Transcript(ref, content, redaction); err != nil {
					return err
				}
			}
		}
		s.turns++
		return s.receipt.Turn(ref, s.turns)
	case "result":
		if amount, ok := claudeResultNumber(raw, "total_cost_usd"); ok {
			if err := s.receipt.Cost(ref, "USD", amount); err != nil {
				return err
			}
		}
		if count, ok := claudeResultTurns(raw); ok {
			return s.receipt.Turn(ref, count)
		}
	}
	return nil
}

func claudeAssistantText(raw []byte) []string {
	var event struct {
		Message struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal(raw, &event) != nil {
		return nil
	}
	var text []string
	for _, block := range event.Message.Content {
		if block.Type == "text" && block.Text != "" {
			text = append(text, block.Text)
		}
	}
	return text
}

func claudeDeltaText(raw []byte) (string, bool) {
	var event struct {
		Event struct {
			Type  string `json:"type"`
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
		} `json:"event"`
	}
	if json.Unmarshal(raw, &event) != nil || event.Event.Type != "content_block_delta" || event.Event.Delta.Type != "text_delta" || event.Event.Delta.Text == "" {
		return "", false
	}
	return event.Event.Delta.Text, true
}

func claudeResultNumber(raw []byte, field string) (string, bool) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return "", false
	}
	src, ok := fields[field]
	if !ok {
		return "", false
	}
	amount := strings.TrimSpace(string(src))
	value, err := strconv.ParseFloat(amount, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return "", false
	}
	return amount, true
}

func claudeResultTurns(raw []byte) (uint64, bool) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return 0, false
	}
	src, ok := fields["num_turns"]
	if !ok {
		return 0, false
	}
	count, err := strconv.ParseUint(strings.TrimSpace(string(src)), 10, 64)
	if err != nil || count == 0 {
		return 0, false
	}
	return count, true
}

// redactClaude replaces every form of the injected credential the packet check
// already refuses, so the two cannot disagree about what counts as the secret
// appearing. A token echoed back inside a JSON string is escaped, and matching
// only the raw bytes would retain it.
func redactClaude(src, secret []byte) (json.RawMessage, Redaction) {
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
