package thread

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/pbleisch/kept/internal/ids"
	"github.com/pbleisch/kept/internal/runtimestate"
)

// PiVersion is the Pi RPC version the fixture records. A newer Pi may have a
// different native event meaning, so it is refused until its own evidence has
// been reviewed rather than being treated as compatible by name alone.
const PiVersion = "0.84.0"

const (
	piAdapterVersion = "pi:0.84.0/rpc:1"

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
