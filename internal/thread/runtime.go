package thread

import (
	"fmt"
	"io"
	"path/filepath"
	"time"
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
	// CodexAccessToken is the one documented app-server credential channel.
	// CodexHome is a newly-created empty state directory for this one session;
	// CodexCWD is the repair box the app-server names in thread/start. The
	// protocol schema digest is explicit because app-server is experimental:
	// a matching command version alone does not make a changed wire shape safe.
	// None of these fields are read from the parent environment.
	CodexAccessToken  string
	CodexHome         string
	CodexCWD          string
	CodexSchemaSHA256 string
	// ClaudeSetupToken is the long-lived subscription OAuth token `claude
	// setup-token` mints; it reaches the child as CLAUDE_CODE_OAUTH_TOKEN and
	// has no route into argv, a packet or a receipt. It is named for the
	// credential it carries rather than for the slot it fills, so the brokered
	// route 0052 also permits becomes a sibling field rather than a rewrite of
	// this one. ClaudeSetupTokenExpiry is when that token lapses: the command
	// takes no options, prints no expiry and has no revoke, so kept is the only
	// thing that can refuse before a repair meets a dead credential. ClaudeHome
	// is a newly-created empty directory for this one session; it is both the
	// child HOME and the parent of its isolated Claude configuration directory,
	// so no inherited or operator configuration is reachable. None of these
	// fields is read from the parent environment.
	ClaudeSetupToken       string
	ClaudeSetupTokenExpiry time.Time
	ClaudeHome             string
	// PiProvider and PiModel are the explicit provider/model pair Pi receives
	// as direct argv values. PiCredentialEnvironment names the one provider
	// credential the child receives; PiCredential is its value. PiHome is a
	// freshly-created empty directory supplied by the caller for this one Pi
	// session. It is both the child HOME and the parent of its isolated Pi
	// configuration directory, so Pi cannot fall back to an operator auth file.
	PiProvider              string
	PiModel                 string
	PiCredentialEnvironment string
	PiCredential            string
	PiHome                  string
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

	// BindInput is present only for a runtime whose documented protocol needs
	// a response from the child before its next request can be written. The
	// confinement launch owns the child's stdin and must bind it before
	// forwarding stdout to Output. It is not a process or lifecycle capability.
	// Adapters with a one-shot input leave it nil.
	BindInput func(io.Writer) error
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
func (codexAdapter) AdapterVersion() string  { return codexAdapterVersion }
func (piAdapter) AdapterVersion() string     { return piAdapterVersion }

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
func (codexAdapter) Prepare(cfg RuntimeConfig, packet Packet, receipt AdapterReceipt) (Invocation, error) {
	if err := checkCodexConfig(cfg, packet); err != nil {
		return Invocation{}, err
	}
	// The local protocol fixture establishes only the documented wire shape.
	// Decision 0052 still requires a credentialed, confined session killed by
	// the parent after receiving partial native evidence before dispatch may
	// start Codex for a repair.
	if !codexAdmitted {
		return Invocation{}, &Refusal{Reason: "runtime is unadmitted", Detail: "codex has no credentialed confined interrupted live proof"}
	}
	return prepareCodex(cfg, packet, receipt)
}
func (piAdapter) Prepare(cfg RuntimeConfig, packet Packet, receipt AdapterReceipt) (Invocation, error) {
	if err := checkPiConfig(cfg, packet); err != nil {
		return Invocation{}, err
	}
	// The version-pinned fixture below is useful safe evidence, but it is not
	// the confined interruption proof decision 0052 requires for admission.
	if !piAdmitted {
		return Invocation{}, &Refusal{Reason: "runtime is unadmitted", Detail: "pi has no confined interrupted live proof"}
	}
	return preparePi(cfg, packet, receipt)
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
