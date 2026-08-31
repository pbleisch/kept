package thread

import (
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/pbleisch/kept/internal/runtimestate"
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
	// Versions is what the operator accepts as a runtime version, and it is the
	// one lever over the admission gate they have. Unset is the strict policy,
	// so a caller that never sets it gets what an exact pin gave it. See
	// VersionPolicy for why an unset value defaults rather than refusing.
	Versions VersionPolicy
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
	// so no inherited or operator configuration is reachable. ClaudeTemp is the
	// box's own temporary directory, delivered under the runtime's own name for
	// it: Claude Code does not read TMPDIR, it reads CLAUDE_CODE_TMPDIR and
	// otherwise uses a hardcoded /tmp, which a repair profile denies by name
	// (docs/decision/0056). None of these fields is read from the parent
	// environment.
	//
	// ClaudeBrokeredCredential is the other channel decision 0052 permits, and
	// exactly one of the two may be in force. Under it the confinement provider
	// resolves a reference kept never reads, publishes a session phantom in the
	// credential variable itself, and rewrites the phantom into the real value
	// on egress — so ClaudeSetupToken is empty, the child environment this
	// adapter builds carries no credential at all, and there is no secret here
	// to redact from a receipt. The expiry is still required, because it is a
	// fact about the credential rather than about the channel it travels by.
	// See docs/decision/0057.
	//
	// ClaudeAsk is what this session is being asked to do with the packet, and
	// it is the one thing here that is neither a credential nor a directory. A
	// packet states authority and evidence and never an ask (0009), and this
	// runtime reads one as a question unless something else says otherwise; the
	// ask is that something else, it travels on a channel of its own, and it is
	// a declared value from a closed set rather than text a caller composes.
	// See docs/decision/0058.
	ClaudeSetupToken         string
	ClaudeBrokeredCredential bool
	ClaudeSetupTokenExpiry   time.Time
	ClaudeHome               string
	ClaudeTemp               string
	ClaudeAsk                ClaudeAsk
	// PiProvider and PiModel are the explicit provider/model pair Pi receives
	// as direct argv values. PiCredential is the separately configured
	// credential, delivered through the one child environment value named by
	// runtimestate.PiCredentialEnvironment. PiState is the minimal private Pi
	// configuration internal/runtimestate materialized for this one session; it
	// is both the child HOME and its Pi agent directory, so Pi reads the
	// projection and has no operator auth file to fall back to. Its type is the
	// guarantee: a directory name could have been the operator's own.
	PiProvider   string
	PiModel      string
	PiCredential string
	PiState      *runtimestate.PiState
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

	// BindInput is present only for a runtime whose documented protocol keeps
	// its input channel open: because it needs a response from the child before
	// its next request can be written, or because its session ends at input
	// EOF. The confinement launch owns the child's stdin, hands it here before
	// forwarding stdout to Output, and closes it itself only when no adapter
	// took it. It is not a process or lifecycle capability: the deadline, the
	// group kill and the reap stay with the supervisor, which is what bounds an
	// adapter that never closes the channel. Adapters whose input is one shot
	// and whose runtime exits on its own leave it nil.
	BindInput func(io.WriteCloser) error
}

// Adapter has an unexported method, so this package owns the closed set. It is
// not a registry or plugin protocol: the only implementations are the three
// static types below, and adding one changes the set decision 0052 froze.
type Adapter interface {
	runtimeAdapter()
	Runtime() Runtime
	AdapterVersion(RuntimeConfig) string
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

// AdapterVersion is composed from the version that will actually run rather
// than from a constant, because a policy that admits more than one version
// makes a constant a claim about a session that did not happen. Reviewedness
// travels with it: an event written by a session on a build nobody read says
// so, which is what keeps LineVersions from being a silent acceptance.
//
// It is asked before Prepare validates the configuration, so an unsupported
// version can reach it — and never reaches a record, because Prepare refuses
// before the launch that writes the first event.
func (claudeAdapter) AdapterVersion(cfg RuntimeConfig) string {
	return claudeSupport.stamp(cfg.Version, cfg.Versions)
}
func (codexAdapter) AdapterVersion(cfg RuntimeConfig) string {
	return codexSupport.stamp(cfg.Version, cfg.Versions)
}
func (piAdapter) AdapterVersion(cfg RuntimeConfig) string {
	return piSupport.stamp(cfg.Version, cfg.Versions)
}

func (claudeAdapter) Prepare(cfg RuntimeConfig, packet Packet, receipt AdapterReceipt) (Invocation, error) {
	if err := checkClaudeConfig(cfg, packet); err != nil {
		return Invocation{}, err
	}
	// The gate decision 0052 requires, kept in the concrete adapter so that
	// selection never turns an evidence fixture into permission to start
	// Claude. It is open: see claudeAdmitted for what opened it. Closing it
	// again is what a version bump or a discarded proof should do, and the
	// refusal below is what a caller then sees.
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
	// The gate stays in the concrete adapter even now that Pi has passed it:
	// admission is per adapter, and selection must never turn one runtime's
	// evidence into permission to start another.
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
