package thread

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"strings"
)

// ClaudeVersion is the Claude Code version whose direct stream shape this
// adapter understands. A different installed version is a refusal, not a
// best-effort invocation whose evidence might mean something else.
const ClaudeVersion = "2.1.238"

const (
	claudeAdapterVersion = "claude-code:2.1.238/stream-json:1"

	// This stays false until the decision-0052 confined, interrupted live
	// session has been recorded. The parser and fixture below are deliberately
	// testable before that evidence exists; neither turns a fixture into an
	// admitted runtime.
	claudeAdmitted = false
)

// claudeArgs is the documented noninteractive stream mode for Claude Code
// 2.1.238. --bare makes the process avoid settings, hooks, keychain reads and
// automatic project context; the adapter supplies only the packet over stdin.
// Permission prompts are bypassed only inside the already verified external
// confinement provider; no adapter creates or relaxes that provider.
var claudeArgs = []string{
	"--bare",
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
	case cfg.AnthropicAPIKey == "":
		return &Refusal{Reason: "ANTHROPIC_API_KEY is unset", Detail: string(Claude)}
	case bytes.Contains(packet.Bytes(), []byte(cfg.AnthropicAPIKey)):
		return &Refusal{Reason: "ANTHROPIC_API_KEY appears in the packet", Detail: string(Claude)}
	}
	return nil
}

// prepareClaude constructs the admitted invocation without reading the parent
// environment, a keychain, a corpus or a ledger. It is separate from
// claudeAdapter.Prepare solely so the fixture can exercise all safe code while
// claudeAdmitted keeps actual dispatch closed.
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
		Path:   cfg.Executable,
		Args:   append([]string(nil), claudeArgs...),
		Input:  input,
		Env:    []string{"ANTHROPIC_API_KEY=" + cfg.AnthropicAPIKey},
		Output: &claudeStream{receipt: receipt, secret: []byte(cfg.AnthropicAPIKey)},
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

func redactClaude(src, secret []byte) (json.RawMessage, Redaction) {
	if len(secret) == 0 {
		return append(json.RawMessage(nil), src...), Redaction{}
	}
	matches := bytes.Count(src, secret)
	if matches == 0 {
		return append(json.RawMessage(nil), src...), Redaction{}
	}
	return bytes.ReplaceAll(src, secret, []byte("[REDACTED]")), Redaction{Occurred: true, Matches: matches}
}
