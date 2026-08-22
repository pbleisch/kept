package thread

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ClaudeVersion is the Claude Code version whose direct stream shape and
// documented credential precedence this adapter understands. A different
// installed version is a refusal, not a best-effort invocation whose evidence
// might mean something else.
const ClaudeVersion = "2.1.240"

const (
	claudeAdapterVersion = "claude-code:2.1.240/stream-json:1"

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
	// API rates instead of drawing on the subscription that was configured.
	claudeAPIKeyEnvironment = "ANTHROPIC_API_KEY"

	// claudeConfigEnvironment relocates every Claude configuration read into
	// the session's own empty directory, so nothing the operator authored is
	// reachable from the box.
	claudeConfigEnvironment = "CLAUDE_CONFIG_DIR"
)

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
var claudeArgs = []string{
	"--safe-mode",
	"--print",
	"--input-format", "stream-json",
	"--output-format", "stream-json",
	"--include-partial-messages",
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
	case cfg.ClaudeSetupToken == "":
		return &Refusal{Reason: claudeCredentialEnvironment + " is unset", Detail: string(Claude)}
	case strings.IndexByte(cfg.ClaudeSetupToken, 0) >= 0:
		return &Refusal{Reason: claudeCredentialEnvironment + " contains NUL", Detail: string(Claude)}
	case packetContains(cfg.ClaudeSetupToken, packet.Bytes()):
		return &Refusal{Reason: claudeCredentialEnvironment + " appears in the packet", Detail: string(Claude)}
	case cfg.ClaudeHome == "":
		return &Refusal{Reason: "claude HOME is unset", Detail: string(Claude)}
	case !filepath.IsAbs(cfg.ClaudeHome):
		return &Refusal{Reason: "claude HOME is not an explicit absolute path", Detail: cfg.ClaudeHome}
	}
	if err := checkClaudeSetupTokenLife(cfg.ClaudeSetupTokenExpiry, time.Now()); err != nil {
		return err
	}

	// The parent environment is read here for one purpose, which is to refuse.
	// Decision 0052 forbids reading it to *obtain* a credential and this never
	// does: the value is not read, not copied and reaches nothing. Only the
	// presence of the name is consulted. Invocation.Env is the complete child
	// environment rather than an addition to this process's, so a key here
	// cannot reach the child through this adapter — but a provider that merged
	// instead of replacing would hand Claude the credential it prefers, and
	// that failure bills at API rates while looking exactly like success.
	// An exported name is a configured intent whatever its value, so an empty
	// one is refused too rather than reasoned about.
	if _, present := os.LookupEnv(claudeAPIKeyEnvironment); present {
		return &Refusal{Reason: claudeAPIKeyEnvironment + " is present in the parent environment", Detail: string(Claude)}
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
	return Invocation{
		Path:  cfg.Executable,
		Args:  append([]string(nil), claudeArgs...),
		Input: input,
		Env: []string{
			"HOME=" + cfg.ClaudeHome,
			claudeConfigEnvironment + "=" + filepath.Join(cfg.ClaudeHome, "config"),
			claudeCredentialEnvironment + "=" + cfg.ClaudeSetupToken,
		},
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
