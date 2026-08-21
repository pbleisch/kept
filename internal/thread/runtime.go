package thread

import (
	"fmt"
	"io"
	"path/filepath"
)

// Runtime is the explicitly selected V1 repair runtime. It is not a command
// name and is never resolved through PATH.
type Runtime string

const (
	Claude Runtime = "claude"
	Codex  Runtime = "codex"
	Pi     Runtime = "pi"
)

func (r Runtime) known() bool { return r == Claude || r == Codex || r == Pi }

// Refusal is a dispatch refusal. It is distinct from an agent failure: no
// runtime was started, so the caller records the disposition rather than an
// invented thread lifecycle outcome.
type Refusal struct {
	Reason string
	Detail string
}

func (e *Refusal) Error() string {
	if e.Detail == "" {
		return "repair dispatch refused: " + e.Reason
	}
	return fmt.Sprintf("repair dispatch refused: %s: %s", e.Reason, e.Detail)
}

// RuntimeConfig is explicit operator configuration. Executable is a literal
// absolute command path once an adapter has admission evidence; this package
// never searches PATH, substitutes a runtime or accepts a shell command.
type RuntimeConfig struct {
	Runtime    string
	Executable string
	Version    string
	// AnthropicAPIKey is used only to form Claude's one-entry child
	// environment. It has no route into argv, a packet or a receipt.
	AnthropicAPIKey string
}

// EventStream receives one runtime's stdout as the confinement launch drains
// it, then is closed after the child is reaped. Closing lets an adapter retain
// a final JSONL item even if the runtime did not finish it with a newline.
type EventStream interface {
	io.Writer
	Close() error
}

// Invocation is the direct argv-shaped result an admitted adapter gives to
// confinement. Env is the complete child environment, not additions to the
// parent environment. Output is where confinement streams the child's stdout
// while it is running; it is the adapter's only route to append native
// evidence, and never gives it a process or lifecycle capability.
type Invocation struct {
	Path   string
	Args   []string
	Input  []byte
	Env    []string
	Output EventStream
}

// Adapter has an unexported method, so this package owns the closed set. It is
// not a registry or plugin protocol: the only implementations are the three
// static types below, and adding one changes the set decision 0052 froze.
type Adapter interface {
	runtimeAdapter()
	Runtime() Runtime
	AdapterVersion() string
	Prepare(RuntimeConfig, Packet, AdapterReceipt) (Invocation, error)
}

type claudeAdapter struct{}
type codexAdapter struct{}
type piAdapter struct{}

func (claudeAdapter) runtimeAdapter() {}
func (codexAdapter) runtimeAdapter()  {}
func (piAdapter) runtimeAdapter()     {}

func (claudeAdapter) Runtime() Runtime { return Claude }
func (codexAdapter) Runtime() Runtime  { return Codex }
func (piAdapter) Runtime() Runtime     { return Pi }

func (claudeAdapter) AdapterVersion() string { return claudeAdapterVersion }
func (codexAdapter) AdapterVersion() string  { return "codex:unadmitted" }
func (piAdapter) AdapterVersion() string     { return "pi:unadmitted" }

func (claudeAdapter) Prepare(cfg RuntimeConfig, packet Packet, receipt AdapterReceipt) (Invocation, error) {
	if err := checkClaudeConfig(cfg, packet); err != nil {
		return Invocation{}, err
	}
	// The invocation is intentionally not returned until the fixture and the
	// interrupted live proof decision 0052 requires have landed. Keeping this
	// gate in the concrete adapter means selection never turns an evidence
	// fixture into permission to start Claude.
	if !claudeAdmitted {
		return Invocation{}, &Refusal{Reason: "runtime is unadmitted", Detail: "claude has no confined interrupted live proof"}
	}
	return prepareClaude(cfg, packet, receipt)
}
func (codexAdapter) Prepare(cfg RuntimeConfig, packet Packet, _ AdapterReceipt) (Invocation, error) {
	return unadmitted(Codex, cfg, packet)
}
func (piAdapter) Prepare(cfg RuntimeConfig, packet Packet, _ AdapterReceipt) (Invocation, error) {
	return unadmitted(Pi, cfg, packet)
}

// Select is a static switch rather than a map or registration mechanism. An
// unset or unknown runtime is refused at selection, before any launch attempt.
func Select(value string) (Adapter, error) {
	switch Runtime(value) {
	case "":
		return nil, &Refusal{Reason: "runtime is unset"}
	case Claude:
		return claudeAdapter{}, nil
	case Codex:
		return codexAdapter{}, nil
	case Pi:
		return piAdapter{}, nil
	default:
		return nil, &Refusal{Reason: "runtime is unknown", Detail: value}
	}
}

func unadmitted(runtime Runtime, cfg RuntimeConfig, packet Packet) (Invocation, error) {
	if cfg.Executable == "" {
		return Invocation{}, &Refusal{Reason: "runtime executable is unset", Detail: string(runtime)}
	}
	if !filepath.IsAbs(cfg.Executable) {
		return Invocation{}, &Refusal{Reason: "runtime executable is not an explicit absolute path", Detail: cfg.Executable}
	}
	if cfg.Version == "" {
		return Invocation{}, &Refusal{Reason: "runtime version is unset", Detail: string(runtime)}
	}
	if packet.Empty() {
		return Invocation{}, &Refusal{Reason: "runtime packet is absent", Detail: string(runtime)}
	}
	return Invocation{}, &Refusal{Reason: "runtime is unadmitted", Detail: string(runtime) + " has no runtime-specific interrupted live fixture"}
}
