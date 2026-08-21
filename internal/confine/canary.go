package confine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	canaryLedger,
	canaryArtifacts,
	canaryCredential,
	canaryUnix,
	canaryHost,
	canaryGit,
	canaryKept,
	canaryProbeWindow,
}

func makeCanaryState(profile Profile) (*canaryState, error) {
	dir, err := os.MkdirTemp("", "kept-nono-canary-")
	if err != nil {
		return nil, fmt.Errorf("create nono canary state: %w", err)
	}
	fail := func(cause error) (*canaryState, error) {
		_ = os.RemoveAll(dir)
		return nil, cause
	}
	credential := filepath.Join(dir, "credential")
	if err := os.WriteFile(credential, []byte("canary-secret\n"), 0o600); err != nil {
		return fail(fmt.Errorf("write nono canary credential: %w", err))
	}
	outside := filepath.Join(dir, "outside")
	if err := os.Mkdir(outside, 0o700); err != nil {
		return fail(fmt.Errorf("create nono canary outside path: %w", err))
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
	return &canaryState{
		dir:        dir,
		credential: credential,
		outside:    outside,
		unix:       unix,
		tcp:        tcp,
		profile: canaryPolicyPaths{
			box: profile.Box, ledger: profile.Ledger, artifacts: profile.Artifacts,
			git: profile.Git, kept: profile.Kept,
		},
		outputBuf:   canaryOutput{output: boundedBuffer{limit: 64 << 10}},
		createdKept: createdKept,
	}, nil
}

func (s *canaryState) environment() []string {
	values := map[string]string{
		canaryBox:         s.profileBox(),
		canaryOutside:     s.outside,
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

func (s *canaryState) deniedPaths() []string {
	return []string{
		s.outside,
		s.credential,
		s.unix.Addr().String(),
		filepath.Join(s.profile.box, ".kept"),
	}
}

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
	_ = os.RemoveAll(s.dir)
}

func (s *canaryState) output() []byte { return append([]byte(nil), s.outputBuf.Bytes()...) }

type canaryReport struct {
	Probes []canaryProbe `json:"probes"`
}

type canaryProbe struct {
	Name    string `json:"name"`
	Refused bool   `json:"refused"`
	Detail  string `json:"detail,omitempty"`
}

var requiredCanaryProbes = []string{
	"write_outside_box",
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
		if !probe.Refused {
			return fmt.Errorf("nono canary escape probe %s succeeded: %s", probe.Name, probe.Detail)
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
		if !probe.Refused {
			return 1
		}
	}
	return 0
}

type canaryConfig struct {
	box, outside, ledger, artifacts, credential, unix, host, git, kept string
	timeout                                                            time.Duration
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
		run  func() error
	}{
		{"write_outside_box", func() error { return writeProbe(filepath.Join(config.outside, "escaped")) }},
		{"write_kept", func() error { return writeProbe(filepath.Join(config.box, ".kept", "escaped")) }},
		{"read_ledger", func() error { return readProbe(config.ledger) }},
		{"read_artifacts", func() error { return readProbe(config.artifacts) }},
		{"read_credential", func() error { return readProbe(config.credential) }},
		{"unix_socket", func() error { return unixProbe(config.unix, config.timeout) }},
		{"host_outside_allowlist", func() error { return hostProbe(config.host, config.timeout) }},
		{"origin_push", func() error { return pushProbe(config.git, config.timeout) }},
		{"kept_executable", func() error { return keptProbe(config.kept, config.timeout) }},
	}
	if runtime.GOOS == "darwin" {
		probes = append(probes, struct {
			name string
			run  func() error
		}{"keychain", func() error { return keychainProbe(config.timeout) }})
	}
	result := make([]canaryProbe, 0, len(probes))
	for _, probe := range probes {
		err := probe.run()
		result = append(result, canaryProbe{Name: probe.name, Refused: err != nil, Detail: probeDetail(err)})
	}
	return result
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

func pushProbe(git string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, git, "push", "--dry-run", "origin")
	cmd.Env = []string{"GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=/bin/false"}
	return cmd.Run()
}

func keptProbe(kept string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return exec.CommandContext(ctx, kept, canaryExecProbeArgument).Run()
}

func keychainProbe(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return exec.CommandContext(ctx, "/usr/bin/security", "dump-keychain").Run()
}
