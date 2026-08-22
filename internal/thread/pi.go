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

	"github.com/pbleisch/kept/internal/ids"
)

// PiVersion is the Pi RPC version the fixture records. A newer Pi may have a
// different native event meaning, so it is refused until its own evidence has
// been reviewed rather than being treated as compatible by name alone.
const PiVersion = "0.84.0"

const (
	piAdapterVersion = "pi:0.84.0/rpc:1"

	// The fixture is necessary evidence, not the confined interrupted live
	// proof decision 0052 requires before the dispatcher may start Pi.
	piAdmitted = false
)

// piArgs disables every Pi input-discovery surface that could turn the packet
// into configuration or a slash command. Repair tools remain enabled: their
// authority is the externally verified confinement profile, not this adapter.
var piArgs = []string{
	"--mode", "rpc",
	"--no-session",
	"--no-extensions",
	"--no-skills",
	"--no-prompt-templates",
	"--no-context-files",
}

func piArgv(cfg RuntimeConfig) []string {
	argv := append([]string(nil), piArgs...)
	return append(argv, "--provider", cfg.PiProvider, "--model", cfg.PiModel)
}

func checkPiConfig(cfg RuntimeConfig, packet Packet) error {
	switch {
	case cfg.Runtime != string(Pi):
		return &Refusal{Reason: "runtime configuration does not name pi", Detail: cfg.Runtime}
	case cfg.Executable == "":
		return &Refusal{Reason: "runtime executable is unset", Detail: string(Pi)}
	case !filepath.IsAbs(cfg.Executable):
		return &Refusal{Reason: "runtime executable is not an explicit absolute path", Detail: cfg.Executable}
	case cfg.Version == "":
		return &Refusal{Reason: "runtime version is unset", Detail: string(Pi)}
	case cfg.Version != PiVersion:
		return &Refusal{Reason: "runtime version is unsupported", Detail: "pi requires " + PiVersion}
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
	case cfg.PiCredentialEnvironment == "":
		return &Refusal{Reason: "pi credential environment is unset", Detail: string(Pi)}
	case !piEnvironmentName(cfg.PiCredentialEnvironment):
		return &Refusal{Reason: "pi credential environment name is invalid", Detail: cfg.PiCredentialEnvironment}
	case cfg.PiCredentialEnvironment == "HOME" || cfg.PiCredentialEnvironment == "PI_CODING_AGENT_DIR":
		return &Refusal{Reason: "pi credential environment is reserved", Detail: cfg.PiCredentialEnvironment}
	case cfg.PiCredential == "":
		return &Refusal{Reason: "pi credential is unset", Detail: cfg.PiCredentialEnvironment}
	case strings.IndexByte(cfg.PiCredential, 0) >= 0:
		return &Refusal{Reason: "pi credential contains NUL", Detail: cfg.PiCredentialEnvironment}
	case packetContains(cfg.PiCredential, packet.Bytes()):
		return &Refusal{Reason: "pi credential appears in the packet", Detail: cfg.PiCredentialEnvironment}
	case cfg.PiHome == "":
		return &Refusal{Reason: "pi HOME is unset", Detail: string(Pi)}
	case !filepath.IsAbs(cfg.PiHome):
		return &Refusal{Reason: "pi HOME is not an explicit absolute path", Detail: cfg.PiHome}
	}

	info, err := os.Stat(cfg.Executable)
	if err != nil {
		return &Refusal{Reason: "runtime executable is unavailable", Detail: cfg.Executable}
	}
	if !info.Mode().IsRegular() || info.Mode()&0111 == 0 {
		return &Refusal{Reason: "runtime executable is not executable", Detail: cfg.Executable}
	}
	info, err = os.Stat(cfg.PiHome)
	if err != nil {
		return &Refusal{Reason: "pi HOME is unavailable", Detail: cfg.PiHome}
	}
	if !info.IsDir() {
		return &Refusal{Reason: "pi HOME is not a directory", Detail: cfg.PiHome}
	}
	entries, err := os.ReadDir(cfg.PiHome)
	if err != nil {
		return &Refusal{Reason: "pi HOME cannot be checked", Detail: cfg.PiHome}
	}
	if len(entries) != 0 {
		return &Refusal{Reason: "pi HOME is not clean", Detail: cfg.PiHome}
	}
	return nil
}

func piUnsafeArgValue(value string) bool {
	return strings.HasPrefix(value, "-") || strings.ContainsAny(value, "\x00\r\n")
}

func piEnvironmentName(name string) bool {
	for i, r := range name {
		if r >= 'A' && r <= 'Z' {
			continue
		}
		if i > 0 && (r == '_' || r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return name != ""
}

// preparePi builds the direct RPC invocation without reading the parent
// environment, a keychain, a corpus or a ledger. It is deliberately separate
// from piAdapter.Prepare so the fixture can exercise the safe edge cases while
// the admission gate keeps actual repair dispatch closed.
func preparePi(cfg RuntimeConfig, packet Packet, receipt AdapterReceipt) (Invocation, error) {
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
	return Invocation{
		Path:  cfg.Executable,
		Args:  piArgv(cfg),
		Input: input,
		Env: []string{
			"HOME=" + cfg.PiHome,
			"PI_CODING_AGENT_DIR=" + filepath.Join(cfg.PiHome, "agent"),
			cfg.PiCredentialEnvironment + "=" + cfg.PiCredential,
		},
		Output: &piStream{receipt: receipt, secret: []byte(cfg.PiCredential)},
	}, nil
}

// piInput sends one and only one documented RPC prompt command. JSON's string
// encoding preserves the immutable packet after unescaping; ids.Mint keeps the
// transport correlation value from becoming a durable, composed identifier.
func piInput(packet Packet) ([]byte, error) {
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
	pending []byte
	turns   uint64
	closed  bool
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
		if err := s.record(line); err != nil {
			return consumed, err
		}
		src = src[at+1:]
	}
	return consumed, nil
}

func (s *piStream) Close() error {
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

func (s *piStream) append(src []byte) error {
	s.pending = append(s.pending, src...)
	return nil
}

func (s *piStream) record(raw []byte) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("decode Pi RPC native event: %w", err)
	}
	if envelope.Type == "" {
		return fmt.Errorf("Pi RPC native event has no type")
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
