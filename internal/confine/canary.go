package confine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// canaryArgument is intentionally not a user-facing kept verb. A copied kept
// executable runs it only inside the nono verification profile; the actual
// kept executable is separately denied and probed from that copy.
const canaryArgument = "--kept-confine-canary"

const canaryExecProbeArgument = "--kept-confine-exec-probe"

const (
	canaryBox         = "KEPT_CONFINE_CANARY_BOX"
	canaryOutside     = "KEPT_CONFINE_CANARY_OUTSIDE"
	canaryHostTemp    = "KEPT_CONFINE_CANARY_HOST_TEMP"
	canaryLedger      = "KEPT_CONFINE_CANARY_LEDGER"
	canaryArtifacts   = "KEPT_CONFINE_CANARY_ARTIFACTS"
	canaryCredential  = "KEPT_CONFINE_CANARY_CREDENTIAL"
	canaryUnix        = "KEPT_CONFINE_CANARY_UNIX"
	canaryHost        = "KEPT_CONFINE_CANARY_HOST"
	canaryGit         = "KEPT_CONFINE_CANARY_GIT"
	canaryKept        = "KEPT_CONFINE_CANARY_KEPT"
	canaryProbeWindow = "KEPT_CONFINE_CANARY_TIMEOUT_MS"
)

var canaryEnvironment = []string{
	canaryBox,
	canaryOutside,
	canaryHostTemp,
	canaryLedger,
	canaryArtifacts,
	canaryCredential,
	canaryUnix,
	canaryHost,
	canaryGit,
	canaryKept,
	canaryProbeWindow,
}

// makeCanaryState prepares what the confined helper is pointed at. Every
// sentinel it creates lies where the provider's baseline grants nothing, which
// is the whole point: a probe aimed at a path kept itself added to
// filesystem.deny demonstrates that a denied path is denied, and says nothing
// about whether the boundary holds for a path nobody named. The operator's home
// is that place — nono's baseline grants the operating system's temporary tree
// write access recursively, so a sentinel under it is refused by nothing.
//
// The consequence is that the canary now runs under exactly the profile a
// repair runs under. It adds no deny entry of its own.
func makeCanaryState(profile Profile) (*canaryState, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("locate the operator's home for the nono canary sentinels: %w", err)
	}
	dir, err := os.MkdirTemp(home, ".kept-canary-")
	if err != nil {
		return nil, fmt.Errorf("create nono canary state: %w", err)
	}
	fail := func(cause error) (*canaryState, error) {
		_ = os.RemoveAll(dir)
		return nil, cause
	}
	if within(profile.Box, canonicalAncestor(dir)) {
		return fail(fmt.Errorf("nono canary sentinels landed inside the box at %s, where the policy grants them", dir))
	}
	credential := filepath.Join(dir, "credential")
	if err := os.WriteFile(credential, []byte("canary-secret\n"), 0o600); err != nil {
		return fail(fmt.Errorf("write nono canary credential: %w", err))
	}
	outside := filepath.Join(dir, "outside")
	if err := os.Mkdir(outside, 0o700); err != nil {
		return fail(fmt.Errorf("create nono canary outside path: %w", err))
	}
	// The real host temporary root, minted by creating a file there and taking
	// it back. Creation is the proof that this location is writable, so the
	// refusal the child must meet is the profile's and not the filesystem's.
	temporary, err := os.CreateTemp("", "kept-canary-escape-")
	if err != nil {
		return fail(fmt.Errorf("mint nono canary host temporary sentinel: %w", err))
	}
	hostTemp := temporary.Name()
	_ = temporary.Close()
	if err := os.Remove(hostTemp); err != nil {
		return fail(fmt.Errorf("clear nono canary host temporary sentinel: %w", err))
	}
	unixPath := filepath.Join(dir, "docker.sock")
	unix, err := net.Listen("unix", unixPath)
	if err != nil {
		return fail(fmt.Errorf("open nono canary Unix sentinel: %w", err))
	}
	tcp, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		_ = unix.Close()
		return fail(fmt.Errorf("open nono canary loopback sentinel: %w", err))
	}
	keptDir := filepath.Join(profile.Box, ".kept")
	createdKept := false
	if err := os.Mkdir(keptDir, 0o700); err == nil {
		createdKept = true
	} else if !errors.Is(err, os.ErrExist) {
		_ = unix.Close()
		_ = tcp.Close()
		return fail(fmt.Errorf("prepare nono canary .kept probe: %w", err))
	}
	state := &canaryState{
		dir:        dir,
		credential: credential,
		outside:    outside,
		hostTemp:   hostTemp,
		unix:       unix,
		tcp:        tcp,
		profile: canaryPolicyPaths{
			box: profile.Box, ledger: profile.Ledger, artifacts: profile.Artifacts,
			git: profile.Git, kept: profile.Kept,
		},
		outputBuf:   canaryOutput{output: boundedBuffer{limit: 64 << 10}},
		createdKept: createdKept,
	}
	for _, target := range []string{state.escape(), filepath.Join(keptDir, "escaped")} {
		if err := proveWritable(target); err != nil {
			state.close()
			return nil, err
		}
	}
	return state, nil
}

// proveWritable is the premise every write probe rests on. A refusal from
// inside the box means the boundary held only if the same write succeeds
// outside it; without this, an escape target that is merely unwritable — a
// read-only mount, a directory the operator cannot write — passes the probe
// while proving nothing.
func proveWritable(path string) error {
	if err := os.WriteFile(path, []byte("canary\n"), 0o600); err != nil {
		return fmt.Errorf("nono canary escape target %s is not writable, so a refusal there would prove nothing: %w", path, err)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("clear nono canary escape target %s: %w", path, err)
	}
	return nil
}

func (s *canaryState) environment() []string {
	values := map[string]string{
		canaryBox:         s.profileBox(),
		canaryOutside:     s.outside,
		canaryHostTemp:    s.hostTemp,
		canaryLedger:      s.profileLedger(),
		canaryArtifacts:   s.profileArtifacts(),
		canaryCredential:  s.credential,
		canaryUnix:        s.unix.Addr().String(),
		canaryHost:        s.tcp.Addr().String(),
		canaryGit:         s.profileGit(),
		canaryKept:        s.profileKept(),
		canaryProbeWindow: "1000",
	}
	output := make([]string, 0, len(canaryEnvironment))
	for _, name := range canaryEnvironment {
		output = append(output, name+"="+values[name])
	}
	return output
}

// escape is the path write_outside_box attempts. It is named here because both
// the parent's writability proof and the confined helper have to agree on it.
func (s *canaryState) escape() string { return filepath.Join(s.outside, "escaped") }

// The canary carries only the five policy paths it must probe. The profile
// itself remains with the parent and is never an input to the confined helper.
func (s *canaryState) profileBox() string       { return s.profile.box }
func (s *canaryState) profileLedger() string    { return s.profile.ledger }
func (s *canaryState) profileArtifacts() string { return s.profile.artifacts }
func (s *canaryState) profileGit() string       { return s.profile.git }
func (s *canaryState) profileKept() string      { return s.profile.kept }

type canaryPolicyPaths struct {
	box       string
	ledger    string
	artifacts string
	git       string
	kept      string
}

func (s *canaryState) close() {
	if s.unix != nil {
		_ = s.unix.Close()
	}
	if s.tcp != nil {
		_ = s.tcp.Close()
	}
	if s.createdKept {
		_ = os.Remove(filepath.Join(s.profile.box, ".kept"))
	}
	if s.hostTemp != "" {
		_ = os.Remove(s.hostTemp)
	}
	_ = os.RemoveAll(s.dir)
}

func (s *canaryState) output() []byte { return append([]byte(nil), s.outputBuf.Bytes()...) }

type canaryReport struct {
	Probes []canaryProbe `json:"probes"`
}

// canaryOutcome is what one probe measured. The three are kept apart because
// 0017 requires a probe that could not be attempted to count as a failure:
// folding absent into refused is exactly how a guard that is not there reads as
// a boundary that held.
type canaryOutcome string

const (
	probeReached canaryOutcome = "reached"
	probeRefused canaryOutcome = "refused"
	probeAbsent  canaryOutcome = "absent"
)

type canaryProbe struct {
	Name    string        `json:"name"`
	Outcome canaryOutcome `json:"outcome"`
	Detail  string        `json:"detail,omitempty"`
}

var requiredCanaryProbes = []string{
	"write_outside_box",
	"write_host_temp",
	"write_kept",
	"read_ledger",
	"read_artifacts",
	"read_credential",
	"unix_socket",
	"host_outside_allowlist",
	"origin_push",
	"kept_executable",
}

func parseCanaryReport(src []byte) (canaryReport, error) {
	var report canaryReport
	if len(src) == 0 {
		return report, errors.New("nono canary produced no report")
	}
	if err := json.Unmarshal(src, &report); err != nil {
		return report, fmt.Errorf("decode nono canary report: %w", err)
	}
	return report, nil
}

func (r canaryReport) valid() error {
	seen := make(map[string]bool, len(r.Probes))
	for _, probe := range r.Probes {
		if probe.Name == "" {
			return errors.New("nono canary reported an unnamed probe")
		}
		if seen[probe.Name] {
			return fmt.Errorf("nono canary reported %s twice", probe.Name)
		}
		seen[probe.Name] = true
		switch probe.Outcome {
		case probeRefused:
		case probeReached:
			return fmt.Errorf("nono canary escape probe %s succeeded: %s", probe.Name, probe.Detail)
		case probeAbsent:
			return fmt.Errorf("nono canary escape probe %s measured nothing, because what it probes is not there: %s", probe.Name, probe.Detail)
		default:
			return fmt.Errorf("nono canary escape probe %s reported an unknown outcome %q", probe.Name, probe.Outcome)
		}
	}
	for _, name := range requiredCanaryProbes {
		if !seen[name] {
			return fmt.Errorf("nono canary did not attempt %s", name)
		}
	}
	if runtime.GOOS == "darwin" && !seen["keychain"] {
		return errors.New("nono canary did not attempt keychain enumeration")
	}
	return nil
}

// CanaryArgs recognizes only the internal helper mode. cmd/kept calls this
// before its normal command dispatcher so running the copied executable needs
// neither a corpus nor a ledger.
func CanaryArgs(args []string) bool {
	return len(args) == 1 && (args[0] == canaryArgument || args[0] == canaryExecProbeArgument)
}

// RunCanary executes the confined half of provider verification. Its one JSON
// report is the proof that each probe ran; a process that cannot start or
// cannot reach the reporting pipe never reads as a passed boundary.
func RunCanary(args []string, stdout io.Writer) int {
	if len(args) != 1 {
		return 2
	}
	if args[0] == canaryExecProbeArgument {
		return 0
	}
	if args[0] != canaryArgument {
		return 2
	}
	config, err := readCanaryConfig()
	if err != nil {
		return 2
	}
	report := canaryReport{Probes: runCanaryProbes(config)}
	encoded, err := json.Marshal(report)
	if err != nil {
		return 2
	}
	if _, err := stdout.Write(encoded); err != nil {
		return 2
	}
	for _, probe := range report.Probes {
		if probe.Outcome != probeRefused {
			return 1
		}
	}
	return 0
}

type canaryConfig struct {
	box, outside, hostTemp, ledger, artifacts, credential, unix, host, git, kept string
	timeout                                                                      time.Duration
}

func readCanaryConfig() (canaryConfig, error) {
	lookup := func(name string) (string, error) {
		value := os.Getenv(name)
		if value == "" {
			return "", fmt.Errorf("nono canary %s is unset", name)
		}
		return value, nil
	}
	var config canaryConfig
	var err error
	for _, field := range []struct {
		name string
		out  *string
	}{
		{canaryBox, &config.box},
		{canaryOutside, &config.outside},
		{canaryHostTemp, &config.hostTemp},
		{canaryLedger, &config.ledger},
		{canaryArtifacts, &config.artifacts},
		{canaryCredential, &config.credential},
		{canaryUnix, &config.unix},
		{canaryHost, &config.host},
		{canaryGit, &config.git},
		{canaryKept, &config.kept},
	} {
		*field.out, err = lookup(field.name)
		if err != nil {
			return config, err
		}
	}
	window, err := lookup(canaryProbeWindow)
	if err != nil {
		return config, err
	}
	milliseconds, err := time.ParseDuration(window + "ms")
	if err != nil || milliseconds <= 0 {
		return config, fmt.Errorf("nono canary timeout is invalid: %q", window)
	}
	config.timeout = milliseconds
	return config, nil
}

func runCanaryProbes(config canaryConfig) []canaryProbe {
	probes := []struct {
		name string
		run  func() probeResult
	}{
		// Two writes outside the box, and neither target is one kept named.
		// The first is a location the provider grants nothing under, so it
		// measures the allow list; the second is the host's temporary root,
		// which the baseline grants recursively and the profile takes back.
		{"write_outside_box", func() probeResult { return refusal(writeProbe(filepath.Join(config.outside, "escaped"))) }},
		{"write_host_temp", func() probeResult { return refusal(writeProbe(config.hostTemp)) }},
		{"write_kept", func() probeResult { return refusal(writeProbe(filepath.Join(config.box, ".kept", "escaped"))) }},
		{"read_ledger", func() probeResult { return refusal(readProbe(config.ledger)) }},
		{"read_artifacts", func() probeResult { return refusal(readProbe(config.artifacts)) }},
		{"read_credential", func() probeResult { return refusal(readProbe(config.credential)) }},
		{"unix_socket", func() probeResult { return refusal(unixProbe(config.unix, config.timeout)) }},
		{"host_outside_allowlist", func() probeResult { return refusal(hostProbe(config.host, config.timeout)) }},
		{"origin_push", func() probeResult { return pushProbe(config.git, config.timeout) }},
		{"kept_executable", func() probeResult { return keptProbe(config.kept, config.timeout) }},
	}
	if runtime.GOOS == "darwin" {
		probes = append(probes, struct {
			name string
			run  func() probeResult
		}{"keychain", func() probeResult { return keychainProbe(config.timeout) }})
	}
	result := make([]canaryProbe, 0, len(probes))
	for _, probe := range probes {
		measured := probe.run()
		result = append(result, canaryProbe{Name: probe.name, Outcome: measured.outcome, Detail: measured.detail})
	}
	return result
}

type probeResult struct {
	outcome canaryOutcome
	detail  string
}

// refusal reads an attempted escape. An error is the boundary holding; no error
// is the boundary gone.
func refusal(err error) probeResult {
	if err == nil {
		return probeResult{outcome: probeReached, detail: probeDetail(nil)}
	}
	return probeResult{outcome: probeRefused, detail: probeDetail(err)}
}

func probeDetail(err error) string {
	if err == nil {
		return "access succeeded"
	}
	message := strings.TrimSpace(err.Error())
	if len(message) > 240 {
		return message[:240]
	}
	return message
}

func writeProbe(path string) error { return os.WriteFile(path, []byte("escaped\n"), 0o600) }

func readProbe(path string) error {
	_, err := os.ReadDir(path)
	if err == nil {
		return nil
	}
	if errors.Is(err, syscall.ENOTDIR) {
		_, err = os.ReadFile(path)
	}
	return err
}

func unixProbe(path string, timeout time.Duration) error {
	dialer := net.Dialer{Timeout: timeout}
	connection, err := dialer.DialContext(context.Background(), "unix", path)
	if err != nil {
		return err
	}
	_ = connection.Close()
	return nil
}

func hostProbe(address string, timeout time.Duration) error {
	connection, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return err
	}
	_ = connection.Close()
	return nil
}

func pushProbe(git string, timeout time.Duration) probeResult {
	return execProbe(git, timeout,
		[]string{"push", "--dry-run", "origin"},
		[]string{"GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=/bin/false"})
}

func keptProbe(kept string, timeout time.Duration) probeResult {
	return execProbe(kept, timeout, []string{canaryExecProbeArgument}, nil)
}

func keychainProbe(timeout time.Duration) probeResult {
	return execProbe("/usr/bin/security", timeout, []string{"dump-keychain"}, nil)
}

// execProbe attempts to run something the policy claims the box cannot run.
//
// It stats first because an exec error alone cannot tell a guard that refused
// from a path that names nothing, and the two are opposite results: a mistyped
// executable would otherwise report the boundary holding while the guard is
// absent — and the provider skips a command policy whose executable it cannot
// find with a warning, not an error. A stat that is itself refused is the
// boundary, not an absence, which is why only "does not exist" reads as absent.
func execProbe(path string, timeout time.Duration, args []string, env []string) probeResult {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return probeResult{outcome: probeAbsent, detail: probeDetail(err)}
		}
		return probeResult{outcome: probeRefused, detail: probeDetail(err)}
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
	if env != nil {
		cmd.Env = env
	}
	err := cmd.Run()
	if errors.Is(err, fs.ErrNotExist) {
		return probeResult{outcome: probeAbsent, detail: probeDetail(err)}
	}
	return refusal(err)
}
