package thread

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/pbleisch/kept/internal/ids"
)

// CodexVersion and CodexAppServerSchemaSHA256 identify the one experimental
// app-server wire contract this adapter understands. The schema was generated
// by Codex 0.149.0's stable v2 command. Either value changing is a refusal,
// not compatibility guessed from an executable named codex.
// codexSupport is the one experimental app-server build this adapter was
// written against, and it declares no line: an unreviewed neighbour is refused
// outright rather than offered to a policy. That is what this adapter's
// evidence supports. An experimental wire contract is precisely where a patch
// release may move the event language, the schema digest below is what actually
// binds it, and codexAdmitted is false regardless.
var codexSupport = support{
	product:  "codex",
	wire:     "app-server:2/stable-schema:9b3de71a5a2f",
	reviewed: []string{"0.149.0"},
}

var CodexVersion = codexSupport.newestReviewed()

// codexAdapterVersion is what a session on the newest reviewed version stamps its native
// events with; an actual dispatch composes the value from the version that ran.
var codexAdapterVersion = codexSupport.adapterVersion(CodexVersion, true)

const CodexAppServerSchemaSHA256 = "9b3de71a5a2ffc980b792a18aa8f8dec3f85f48829560222a0264fe494b679a9"

const (

	// A fixture can establish that this direct protocol is understood, but it
	// cannot establish the credentialed, confined interruption evidence 0052
	// requires. Keep dispatch closed until that separate proof exists.
	codexAdmitted = false
)

// codexArgs is the documented stdio app-server command. In particular it is
// neither codex exec nor a shell command. Disabling project instructions is a
// fixed protocol condition: the immutable packet must be the session input,
// not an addition to discovered host instructions.
var codexArgs = []string{
	"app-server",
	"-c", "project_doc_max_bytes=0",
	"--strict-config",
	"--stdio",
}

func checkCodexConfig(cfg RuntimeConfig, packet Input) error {
	switch {
	case cfg.Runtime != string(Codex):
		return &Refusal{Reason: "runtime configuration does not name codex", Detail: cfg.Runtime}
	case cfg.Executable == "":
		return &Refusal{Reason: "runtime executable is unset", Detail: string(Codex)}
	case !filepath.IsAbs(cfg.Executable):
		return &Refusal{Reason: "runtime executable is not an explicit absolute path", Detail: cfg.Executable}
	}
	if _, err := codexSupport.admit(Codex, cfg.Version, cfg.Versions); err != nil {
		return err
	}
	switch {
	case cfg.CodexSchemaSHA256 == "":
		return &Refusal{Reason: "Codex app-server schema digest is unset", Detail: string(Codex)}
	case cfg.CodexSchemaSHA256 != CodexAppServerSchemaSHA256:
		return &Refusal{Reason: "Codex app-server schema digest is unsupported", Detail: "codex requires " + CodexAppServerSchemaSHA256}
	case isBrief(packet):
		// An adversarial pass needs its own declared ask and a read-only box,
		// and this adapter has neither. Refused by name rather than launched
		// with a repair's framing over an assessment's evidence, which is a
		// session that would run and produce nothing anybody could account for.
		// See docs/decision/0081.
		return &Refusal{Reason: "runtime has no adversarial pass", Detail: string(Codex)}
	case isRework(packet):
		// A rework packet needs a framing that says the promise may already
		// hold and the earlier work was not accepted, and this adapter has none.
		// See docs/decision/0086.
		return &Refusal{Reason: "runtime has no rework ask", Detail: string(Codex)}
	case packet.Empty():
		return &Refusal{Reason: "runtime packet is absent", Detail: string(Codex)}
	case !utf8.Valid(packet.Bytes()):
		return &Refusal{Reason: "runtime packet is not valid UTF-8", Detail: string(Codex)}
	case cfg.CodexAccessToken == "":
		return &Refusal{Reason: "CODEX_ACCESS_TOKEN is unset", Detail: string(Codex)}
	case strings.IndexByte(cfg.CodexAccessToken, 0) >= 0:
		return &Refusal{Reason: "CODEX_ACCESS_TOKEN contains NUL", Detail: string(Codex)}
	case codexPacketContains(cfg.CodexAccessToken, packet.Bytes()):
		return &Refusal{Reason: "CODEX_ACCESS_TOKEN appears in the packet", Detail: string(Codex)}
	case cfg.CodexHome == "":
		return &Refusal{Reason: "CODEX_HOME is unset", Detail: string(Codex)}
	case !filepath.IsAbs(cfg.CodexHome):
		return &Refusal{Reason: "CODEX_HOME is not an explicit absolute path", Detail: cfg.CodexHome}
	case cfg.CodexCWD == "":
		return &Refusal{Reason: "Codex thread cwd is unset", Detail: string(Codex)}
	case !filepath.IsAbs(cfg.CodexCWD):
		return &Refusal{Reason: "Codex thread cwd is not an explicit absolute path", Detail: cfg.CodexCWD}
	}

	info, err := os.Stat(cfg.Executable)
	if err != nil {
		return &Refusal{Reason: "runtime executable is unavailable", Detail: cfg.Executable}
	}
	if !info.Mode().IsRegular() || info.Mode()&0111 == 0 {
		return &Refusal{Reason: "runtime executable is not executable", Detail: cfg.Executable}
	}
	info, err = os.Stat(cfg.CodexHome)
	if err != nil {
		return &Refusal{Reason: "CODEX_HOME is unavailable", Detail: cfg.CodexHome}
	}
	if !info.IsDir() {
		return &Refusal{Reason: "CODEX_HOME is not a directory", Detail: cfg.CodexHome}
	}
	entries, err := os.ReadDir(cfg.CodexHome)
	if err != nil {
		return &Refusal{Reason: "CODEX_HOME cannot be checked", Detail: cfg.CodexHome}
	}
	if len(entries) != 0 {
		return &Refusal{Reason: "CODEX_HOME is not clean", Detail: cfg.CodexHome}
	}
	info, err = os.Stat(cfg.CodexCWD)
	if err != nil {
		return &Refusal{Reason: "Codex thread cwd is unavailable", Detail: cfg.CodexCWD}
	}
	if !info.IsDir() {
		return &Refusal{Reason: "Codex thread cwd is not a directory", Detail: cfg.CodexCWD}
	}
	if codexPathsOverlap(cfg.CodexHome, cfg.CodexCWD) {
		return &Refusal{Reason: "CODEX_HOME overlaps the Codex thread cwd", Detail: cfg.CodexHome}
	}
	return nil
}

func codexPathsOverlap(a, b string) bool {
	for _, pair := range [][2]string{{a, b}, {b, a}} {
		rel, err := filepath.Rel(pair[0], pair[1])
		if err != nil {
			return true
		}
		if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
			return true
		}
	}
	return false
}

// prepareCodex builds only the direct app-server invocation. Its session
// object sends turn/start only after it observes the native response naming a
// real thread. The confinement launch supplies the child's stdin through
// BindInput; the adapter never sees a process or its lifecycle controls.
func prepareCodex(cfg RuntimeConfig, packet Input, receipt AdapterReceipt) (Invocation, error) {
	if err := checkCodexConfig(cfg, packet); err != nil {
		return Invocation{}, err
	}
	if receipt == nil {
		return Invocation{}, &Refusal{Reason: "thread adapter receipt is absent", Detail: string(Codex)}
	}

	stream := &codexStream{
		receipt:         receipt,
		secret:          []byte(cfg.CodexAccessToken),
		packet:          append([]byte(nil), packet.Bytes()...),
		threadRequestID: ids.Mint(),
		cwd:             cfg.CodexCWD,
	}
	input, err := codexInitialInput(cfg, stream.threadRequestID)
	if err != nil {
		return Invocation{}, err
	}
	return Invocation{
		Path:      cfg.Executable,
		Args:      append([]string(nil), codexArgs...),
		Input:     input,
		Env:       []string{"CODEX_HOME=" + cfg.CodexHome, "CODEX_ACCESS_TOKEN=" + cfg.CodexAccessToken},
		Output:    stream,
		BindInput: stream.bindInput,
		Redact: func(src []byte) ([]byte, Redaction) {
			redacted, redaction := redactCodex(src, []byte(cfg.CodexAccessToken))
			return redacted, redaction
		},
	}, nil
}

// codexInitialInput is the fixed opening of the documented JSON-RPC session.
// The packet does not appear until turn/start, after thread/start's result
// gives the adapter a server-minted thread id.
func codexInitialInput(cfg RuntimeConfig, threadRequestID string) ([]byte, error) {
	type clientInfo struct {
		Name    string `json:"name"`
		Title   string `json:"title"`
		Version string `json:"version"`
	}
	type initializeParams struct {
		ClientInfo   clientInfo `json:"clientInfo"`
		Capabilities struct {
			ExperimentalAPI bool `json:"experimentalApi"`
		} `json:"capabilities"`
	}
	type threadStartParams struct {
		CWD            string `json:"cwd"`
		ApprovalPolicy string `json:"approvalPolicy"`
		Sandbox        string `json:"sandbox"`
		Ephemeral      bool   `json:"ephemeral"`
	}

	var initialize initializeParams
	initialize.ClientInfo = clientInfo{Name: "kept", Title: "kept repair", Version: CodexVersion}
	initialize.Capabilities.ExperimentalAPI = false
	return codexJSONL(
		codexRequest{ID: ids.Mint(), Method: "initialize", Params: initialize},
		codexRequest{Method: "initialized", Params: struct{}{}},
		codexRequest{ID: threadRequestID, Method: "thread/start", Params: threadStartParams{
			CWD: cfg.CodexCWD, ApprovalPolicy: "never", Sandbox: "workspace-write", Ephemeral: true,
		}},
	)
}

type codexRequest struct {
	ID     string `json:"id,omitempty"`
	Method string `json:"method"`
	Params any    `json:"params"`
}

func codexJSONL(requests ...codexRequest) ([]byte, error) {
	var output bytes.Buffer
	for _, request := range requests {
		encoded, err := json.Marshal(request)
		if err != nil {
			return nil, fmt.Errorf("encode Codex app-server request: %w", err)
		}
		output.Write(encoded)
		output.WriteByte('\n')
	}
	return output.Bytes(), nil
}

// codexStream receives stdout while the app-server is alive. It retains every
// well-formed JSON-RPC response and notification, then makes only projections
// backed by the notification that supplied them.
type codexStream struct {
	receipt AdapterReceipt
	secret  []byte
	packet  []byte

	threadRequestID string
	threadID        string
	turnRequestID   string
	cwd             string
	input           io.WriteCloser

	pending       []byte
	turns         uint64
	sawTranscript bool
	sawUsage      bool
	sawTurn       bool
	closed        bool
	mu            sync.Mutex
}

func (s *codexStream) bindInput(input io.WriteCloser) error {
	if input == nil {
		return fmt.Errorf("Codex app-server stdin is absent")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.input != nil {
		return fmt.Errorf("Codex app-server stdin was bound more than once")
	}
	if s.closed {
		return fmt.Errorf("Codex app-server stdin was bound after close")
	}
	s.input = input
	return nil
}

func (s *codexStream) Write(src []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 0, fmt.Errorf("write Codex app-server stream after close")
	}
	consumed := 0
	for len(src) > 0 {
		at := bytes.IndexByte(src, '\n')
		if at < 0 {
			s.pending = append(s.pending, src...)
			return consumed + len(src), nil
		}
		s.pending = append(s.pending, src[:at]...)
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

func (s *codexStream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	var tail error
	if len(bytes.TrimSpace(s.pending)) != 0 {
		tail = s.record(s.pending, true)
		s.pending = nil
	}
	// The end of the runtime's output files this session's receipt into the
	// ledger, before the evidence this adapter requires is judged: a session
	// that ended without its pre-exit evidence is still a session whose lines
	// belong in the ledger. See docs/decision/0087.
	if ended := s.receipt.Ended(); tail != nil || ended != nil {
		return errors.Join(tail, ended)
	}
	var missing []string
	if !s.sawTranscript {
		missing = append(missing, "agent-message transcript")
	}
	if !s.sawUsage {
		missing = append(missing, "token usage")
	}
	if !s.sawTurn {
		missing = append(missing, "turn")
	}
	if len(missing) != 0 {
		return fmt.Errorf("Codex app-server ended before pre-exit %s evidence arrived", strings.Join(missing, ", "))
	}
	return nil
}

// record retains one line. A line that is not exactly one app-server message is
// retained as undecodable and interpreted as nothing — the strict envelope below
// is what keeps a schema change from being read as a session, and recording the
// line under its reason is not reading it as one. What was decoded and then
// contradicts the protocol still refuses. See docs/decision/0084.
func (s *codexStream) record(raw []byte, unterminated bool) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	message, err := decodeCodexRPC(raw)
	if err != nil {
		content, redaction := redactCodex(raw, s.secret)
		return s.receipt.Undecodable(content, err.Error(), unterminated, redaction)
	}
	typ := "response"
	if message.notification {
		typ = message.method
	}
	redacted, redaction := redactCodex(raw, s.secret)
	ref, err := s.receipt.Native(typ, redacted, redaction)
	if err != nil {
		return err
	}
	if message.notification {
		return s.project(message.method, message.params, ref)
	}
	return s.response(message)
}

type codexRPCMessage struct {
	notification bool
	id           string
	method       string
	params       json.RawMessage
	result       json.RawMessage
	err          json.RawMessage
}

// decodeCodexRPC implements the exact line-oriented app-server envelope. Its
// transport deliberately omits the JSON-RPC header; accepting a header, a
// batch, an ambiguous envelope, or a response with both result and error would
// silently turn a schema change into a session we cannot interpret.
func decodeCodexRPC(raw []byte) (codexRPCMessage, error) {
	if !utf8.Valid(raw) {
		return codexRPCMessage{}, fmt.Errorf("decode Codex app-server JSON-RPC: invalid UTF-8")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return codexRPCMessage{}, fmt.Errorf("decode Codex app-server JSON-RPC: %w", err)
	}
	if fields == nil {
		return codexRPCMessage{}, fmt.Errorf("decode Codex app-server JSON-RPC: message is not an object")
	}
	if _, present := fields["jsonrpc"]; present {
		return codexRPCMessage{}, fmt.Errorf("decode Codex app-server JSON-RPC: wire must omit the jsonrpc header")
	}
	method, hasMethod := fields["method"]
	id, hasID := fields["id"]
	result, hasResult := fields["result"]
	errPayload, hasError := fields["error"]
	params, hasParams := fields["params"]
	if hasMethod {
		if hasID || hasResult || hasError {
			return codexRPCMessage{}, fmt.Errorf("decode Codex app-server JSON-RPC: notification mixes request or response fields")
		}
		var name string
		if err := json.Unmarshal(method, &name); err != nil || name == "" {
			return codexRPCMessage{}, fmt.Errorf("decode Codex app-server JSON-RPC: notification has no method")
		}
		if !hasParams || !codexJSONObject(params) {
			return codexRPCMessage{}, fmt.Errorf("decode Codex app-server JSON-RPC: notification %q has no object params", name)
		}
		return codexRPCMessage{notification: true, method: name, params: append(json.RawMessage(nil), params...)}, nil
	}
	if !hasID || hasParams || hasResult == hasError {
		return codexRPCMessage{}, fmt.Errorf("decode Codex app-server JSON-RPC: message is neither one notification nor one response")
	}
	var responseID string
	if err := json.Unmarshal(id, &responseID); err != nil || responseID == "" {
		return codexRPCMessage{}, fmt.Errorf("decode Codex app-server JSON-RPC: response has no string id")
	}
	if hasError && !codexJSONObject(errPayload) {
		return codexRPCMessage{}, fmt.Errorf("decode Codex app-server JSON-RPC: response error is not an object")
	}
	message := codexRPCMessage{id: responseID}
	if hasResult {
		message.result = append(json.RawMessage(nil), result...)
	} else {
		message.err = append(json.RawMessage(nil), errPayload...)
	}
	return message, nil
}

func codexJSONObject(raw json.RawMessage) bool {
	var object map[string]json.RawMessage
	return json.Unmarshal(raw, &object) == nil && object != nil
}

func (s *codexStream) response(message codexRPCMessage) error {
	if len(message.err) != 0 {
		return fmt.Errorf("Codex app-server returned an error for request %q", message.id)
	}
	switch message.id {
	case s.threadRequestID:
		if s.threadID != "" {
			return fmt.Errorf("Codex app-server returned thread/start more than once")
		}
		threadID, err := codexThreadID(message.result)
		if err != nil {
			return err
		}
		if s.input == nil {
			return fmt.Errorf("Codex app-server returned thread/start before stdin was bound")
		}
		turnInput, turnRequestID, err := s.turnInput(threadID)
		if err != nil {
			return err
		}
		written, err := s.input.Write(turnInput)
		if err != nil {
			return fmt.Errorf("send Codex turn/start: %w", err)
		}
		if written != len(turnInput) {
			return fmt.Errorf("send Codex turn/start: wrote %d of %d bytes", written, len(turnInput))
		}
		s.threadID, s.turnRequestID = threadID, turnRequestID
	case s.turnRequestID:
		if s.turnRequestID == "" {
			return fmt.Errorf("Codex app-server returned turn/start before thread/start")
		}
		if _, err := codexTurnID(message.result); err != nil {
			return err
		}
	}
	return nil
}

func codexThreadID(result json.RawMessage) (string, error) {
	var response struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err := json.Unmarshal(result, &response); err != nil || response.Thread.ID == "" {
		return "", fmt.Errorf("Codex app-server thread/start result has no thread id")
	}
	return response.Thread.ID, nil
}

func codexTurnID(result json.RawMessage) (string, error) {
	var response struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	if err := json.Unmarshal(result, &response); err != nil || response.Turn.ID == "" {
		return "", fmt.Errorf("Codex app-server turn/start result has no turn id")
	}
	return response.Turn.ID, nil
}

func (s *codexStream) turnInput(threadID string) ([]byte, string, error) {
	type textInput struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	type sandboxPolicy struct {
		Type          string `json:"type"`
		NetworkAccess bool   `json:"networkAccess"`
	}
	type turnStartParams struct {
		ThreadID       string        `json:"threadId"`
		Input          []textInput   `json:"input"`
		CWD            string        `json:"cwd"`
		ApprovalPolicy string        `json:"approvalPolicy"`
		SandboxPolicy  sandboxPolicy `json:"sandboxPolicy"`
	}
	requestID := ids.Mint()
	input, err := codexJSONL(codexRequest{ID: requestID, Method: "turn/start", Params: turnStartParams{
		ThreadID:       threadID,
		Input:          []textInput{{Type: "text", Text: string(s.packet)}},
		CWD:            s.cwd,
		ApprovalPolicy: "never",
		SandboxPolicy:  sandboxPolicy{Type: "workspaceWrite", NetworkAccess: false},
	}})
	if err != nil {
		return nil, "", err
	}
	return input, requestID, nil
}

func (s *codexStream) project(method string, params json.RawMessage, ref string) error {
	switch method {
	case "item/agentMessage/delta":
		threadID, _, text, err := codexAgentDelta(params)
		if err != nil {
			return err
		}
		if err := s.checkThread(threadID); err != nil {
			return err
		}
		if text == "" {
			return nil
		}
		content, redaction := redactCodex([]byte(text), s.secret)
		if _, err := s.receipt.Transcript(ref, content, redaction); err != nil {
			return err
		}
		s.sawTranscript = true
	case "thread/tokenUsage/updated":
		threadID, err := codexTokenUsageThread(params)
		if err != nil {
			return err
		}
		if err := s.checkThread(threadID); err != nil {
			return err
		}
		// This is token evidence, not a monetary cost. The schema has no
		// tariff and decision 0052 forbids inventing one, so its durable common
		// receipt is the native entry above rather than Cost.
		s.sawUsage = true
	case "turn/started":
		threadID, _, err := codexTurnNotification(params)
		if err != nil {
			return err
		}
		if err := s.checkThread(threadID); err != nil {
			return err
		}
		s.turns++
		if err := s.receipt.Turn(ref, s.turns); err != nil {
			return err
		}
		s.sawTurn = true
	case "turn/completed":
		threadID, _, err := codexTurnNotification(params)
		if err != nil {
			return err
		}
		if err := s.checkThread(threadID); err != nil {
			return err
		}
	}
	return nil
}

func (s *codexStream) checkThread(threadID string) error {
	if s.threadID != "" && threadID != s.threadID {
		return fmt.Errorf("Codex app-server notification names a thread other than thread/start")
	}
	return nil
}

func codexAgentDelta(params json.RawMessage) (string, string, string, error) {
	var event struct {
		ThreadID string `json:"threadId"`
		TurnID   string `json:"turnId"`
		ItemID   string `json:"itemId"`
		Delta    string `json:"delta"`
	}
	if err := json.Unmarshal(params, &event); err != nil || event.ThreadID == "" || event.TurnID == "" || event.ItemID == "" {
		return "", "", "", fmt.Errorf("Codex app-server agent-message delta is incomplete")
	}
	return event.ThreadID, event.TurnID, event.Delta, nil
}

func codexTokenUsageThread(params json.RawMessage) (string, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(params, &fields); err != nil {
		return "", fmt.Errorf("Codex app-server token-usage update is malformed")
	}
	threadID := codexString(fields["threadId"])
	turnID := codexString(fields["turnId"])
	usage := fields["tokenUsage"]
	if threadID == "" || turnID == "" || !codexJSONObject(usage) {
		return "", fmt.Errorf("Codex app-server token-usage update is incomplete")
	}
	var totals map[string]json.RawMessage
	if err := json.Unmarshal(usage, &totals); err != nil || !codexJSONObject(totals["last"]) || !codexJSONObject(totals["total"]) {
		return "", fmt.Errorf("Codex app-server token-usage update has no last and total usage")
	}
	return threadID, nil
}

func codexTurnNotification(params json.RawMessage) (string, string, error) {
	var event struct {
		ThreadID string `json:"threadId"`
		Turn     struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	if err := json.Unmarshal(params, &event); err != nil || event.ThreadID == "" || event.Turn.ID == "" {
		return "", "", fmt.Errorf("Codex app-server turn notification is incomplete")
	}
	return event.ThreadID, event.Turn.ID, nil
}

func codexString(raw json.RawMessage) string {
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return ""
	}
	return value
}

func codexPacketContains(secret string, packet []byte) bool {
	for _, form := range codexSecretForms([]byte(secret)) {
		if bytes.Contains(packet, form) {
			return true
		}
	}
	return false
}

func redactCodex(src, secret []byte) (json.RawMessage, Redaction) {
	redacted := append(json.RawMessage(nil), src...)
	matches := 0
	for _, form := range codexSecretForms(secret) {
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

func codexSecretForms(secret []byte) [][]byte {
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
