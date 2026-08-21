package thread

import (
	"fmt"
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
}

// Invocation is the argv-shaped result a future admitted direct adapter may
// give to confinement. No V1 runtime currently returns one: without the live
// interrupted fixture, constructing an argv would falsely claim evidence.
type Invocation struct {
	Path  string
	Args  []string
	Input []byte
	Env   []string
}

// Adapter has an unexported method, so this package owns the closed set. It is
// not a registry or plugin protocol: the only implementations are the three
// static types below, and adding one changes the set decision 0052 froze.
type Adapter interface {
	runtimeAdapter()
	Runtime() Runtime
	Prepare(RuntimeConfig, Packet) (Invocation, error)
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

func (claudeAdapter) Prepare(cfg RuntimeConfig, packet Packet) (Invocation, error) {
	return unadmitted(Claude, cfg, packet)
}
func (codexAdapter) Prepare(cfg RuntimeConfig, packet Packet) (Invocation, error) {
	return unadmitted(Codex, cfg, packet)
}
func (piAdapter) Prepare(cfg RuntimeConfig, packet Packet) (Invocation, error) {
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
