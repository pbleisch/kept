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
	canaryPush        = "KEPT_CONFINE_CANARY_PUSH"
	canaryKept        = "KEPT_CONFINE_CANARY_KEPT"
	canaryProbeWindow = "KEPT_CONFINE_CANARY_TIMEOUT_MS"
	// canaryUnwritable carries the directories a read-only box must be refused
	// in, one per line, and is empty for a box that is not read-only. It is the
	// one canary value that may be empty: an empty one says this box grants a
	// repair scope and the probe below does not apply, which is a different
	// statement from a value nobody set. See docs/decision/0081.
	canaryUnwritable = "KEPT_CONFINE_CANARY_UNWRITABLE"
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
	canaryPush,
	canaryKept,
	canaryProbeWindow,
	canaryUnwritable,
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
		unwritable: append([]string(nil), profile.Unwritable...),
		unix:       unix,
		tcp:        tcp,
		profile: canaryPolicyPaths{
			box: profile.Box, ledger: profile.Ledger, artifacts: profile.Artifacts,
			git: profile.Git, kept: profile.Kept,
		},
		outputBuf:   canaryOutput{output: boundedBuffer{limit: 64 << 10}},
		createdKept: createdKept,
	}
	targets := []string{state.escape(), filepath.Join(keptDir, "escaped")}
	for _, path := range state.unwritable {
		// The premise the read-only probe rests on, and it is the same one
		// every other write probe has: a directory the parent cannot write is
		// a refusal the profile did not cause.
		targets = append(targets, filepath.Join(path, "escaped"))
	}
	for _, target := range targets {
		if err := proveWritable(target); err != nil {
			state.close()
			return nil, err
		}
	}
	if err := state.provePush(profile); err != nil {
		state.close()
		return nil, err
	}
	return state, nil
}

// canaryPushRef is the branch origin_push tries to create in the sentinel
// origin, and canaryProofRef is the one the parent created there first.
const (
	canaryPushRef  = "refs/heads/escaped"
	canaryProofRef = "refs/heads/proof"
)

// canaryPushSentinel is what origin_push measures against: a real repository
// with a real origin, rather than a bare `git push origin` in a directory that
// is not a repository at all.
type canaryPushSentinel struct {
	// work is the repository the probe pushes from. It lies in the box's own
	// temporary directory, which is the only place outside the box a confined
	// child may write that a repair does not read afterwards — a git repository
	// left in the repair scope would arrive in the repair's diff. It is
	// discarded with the canary.
	work string
	// origin is the bare repository work pushes to. It sits beside the other
	// sentinels, under the operator's home, where the provider's baseline
	// grants nothing and the profile denies nothing.
	origin string
}

// provePush builds origin_push's sentinel and proves it works before the box is
// asked to fail at it.
//
// The parent pushes to the sentinel origin itself, exactly as proveWritable
// writes the escape target itself, and for the same reason: a push refused
// because origin does not exist and a push refused by the boundary are the same
// two lines. Measured against real git — an origin removed and an origin the
// process cannot open both report `does not appear to be a git repository` and
// exit 128. Nothing the confined probe can read tells them apart, so the parent
// settles it here, where it can.
func (s *canaryState) provePush(profile Profile) error {
	sentinel := canaryPushSentinel{
		origin: filepath.Join(s.dir, "origin.git"),
		work:   filepath.Join(profile.Temp, "kept-canary-push"),
	}
	if within(profile.Box, canonicalAncestor(sentinel.work)) {
		return fmt.Errorf("nono canary push sentinel landed inside the box at %s, where a repair would read it", sentinel.work)
	}
	if err := os.RemoveAll(sentinel.work); err != nil {
		return fmt.Errorf("clear the nono canary push sentinel at %s: %w", sentinel.work, err)
	}
	s.push = sentinel
	// 0026: nothing external runs without a deadline, the canary's git included.
	ctx, cancel := context.WithTimeout(context.Background(), profile.CanaryTimeout)
	defer cancel()
	for _, step := range [][]string{
		// No --initial-branch: every ref here is pushed and read by full name,
		// so the sentinel depends on no default and on no git recent enough to
		// have the flag.
		{"init", "--bare", "--quiet", sentinel.origin},
		{"init", "--quiet", sentinel.work},
		{"-C", sentinel.work, "remote", "add", "origin", sentinel.origin},
		{"-C", sentinel.work, "-c", "user.name=kept canary", "-c", "user.email=canary@kept.invalid",
			"commit", "--quiet", "--allow-empty", "--no-gpg-sign", "--message", "kept canary push sentinel"},
		{"-C", sentinel.work, "push", "--quiet", "origin", "HEAD:" + canaryProofRef},
		// The proof is the ref read back out of origin, not the exit code of
		// the push: a push that reported success and landed nothing would
		// leave the probe refusing at an origin that was never pushable.
		{"-C", sentinel.origin, "rev-parse", "--verify", "--quiet", canaryProofRef},
	} {
		cmd := exec.CommandContext(ctx, profile.Git, step...)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("nono canary origin at %s is not pushable, so a refusal there would prove nothing: git %s: %w: %s",
				sentinel.origin, strings.Join(step, " "), err, strings.TrimSpace(string(output)))
		}
	}
	return nil
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
		canaryPush:        s.push.work,
		canaryKept:        s.profileKept(),
		canaryProbeWindow: "1000",
		canaryUnwritable:  strings.Join(s.unwritable, "\n"),
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
	if s.push.work != "" {
		_ = os.RemoveAll(s.push.work)
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

// readOnlyCanaryProbe is required of a read-only box and of no other, because
// it is the only probe whose target depends on what sort of box this is. A
// profile that declared read-only and did not attempt it would be a boundary
// nothing measured; one that attempted it without declaring it would be probing
// a directory a repair is entitled to write.
const readOnlyCanaryProbe = "write_box"

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

func (r canaryReport) valid(readOnly bool) error {
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
	if readOnly && !seen[readOnlyCanaryProbe] {
		return fmt.Errorf("nono canary did not attempt %s, so the box's read-only declaration was measured by nothing", readOnlyCanaryProbe)
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
	box, outside, hostTemp, ledger, artifacts, credential, unix, host, git, push, kept string
	// unwritable is empty for a box that grants a repair scope, and is the
	// directories a read-only box must be refused in otherwise.
	unwritable []string
	timeout    time.Duration
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
		{canaryPush, &config.push},
		{canaryKept, &config.kept},
	} {
		*field.out, err = lookup(field.name)
		if err != nil {
			return config, err
		}
	}
	// The one value that may be absent, and the reason is above: empty says
	// this box is not read-only rather than saying nobody set it.
	for _, line := range strings.Split(os.Getenv(canaryUnwritable), "\n") {
		if strings.TrimSpace(line) != "" {
			config.unwritable = append(config.unwritable, line)
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
		{"origin_push", func() probeResult { return pushProbe(config.git, config.push, config.timeout) }},
		{"kept_executable", func() probeResult { return keptProbe(config.kept, config.timeout) }},
	}
	if len(config.unwritable) > 0 {
		probes = append(probes, struct {
			name string
			run  func() probeResult
		}{readOnlyCanaryProbe, func() probeResult { return boxWriteProbe(config.unwritable) }})
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

// probeDetail is what a probe's record says. The bound is generous because an
// instrument may write diagnostics of its own before it does the thing asked of
// it — measured, Apple's /usr/bin/git shim prepends two lines about a cache file
// it cannot create to the output of every command — and a bound that cuts the
// diagnosis leaves a record that says nothing about what the probe measured.
// Eleven probes at this size stay far inside the report's own 64 KiB bound.
func probeDetail(err error) string {
	if err == nil {
		return "access succeeded"
	}
	message := strings.TrimSpace(err.Error())
	if len(message) > 1024 {
		return message[:1024]
	}
	return message
}

func writeProbe(path string) error { return os.WriteFile(path, []byte("escaped\n"), 0o600) }

// boxWriteProbe attempts a write in every directory a read-only box declared
// unwritable, and is refused only if every one of them refused.
//
// One probe over the whole set rather than one probe per path, because the
// question is about the box and not about a directory: *can this session write
// anything a repair of the same promise could have written*. A set with one that
// reached is a read-only box that is not one, and the detail names which — a
// report saying only that something got through is a report nobody can act on.
func boxWriteProbe(paths []string) probeResult {
	var refused []string
	for _, path := range paths {
		target := filepath.Join(path, "escaped")
		if err := writeProbe(target); err == nil {
			_ = os.Remove(target)
			return probeResult{outcome: probeReached, detail: "wrote " + target}
		} else {
			refused = append(refused, probeDetail(err))
		}
	}
	return probeResult{outcome: probeRefused, detail: strings.Join(refused, "; ")}
}

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

// pushProbe attempts the escape 0017 names: a confined agent reaching origin.
// What it defends is 0015's gate — an agent that can push has walked around the
// merge queue, and every guarantee about what is on main becomes decorative.
//
// git exits non-zero both for failing to reach origin and for having nothing to
// push from, and reading the second as the first is what this probe used to do:
// `git push --dry-run origin` in a box that is not a git repository exits 128,
// and the probe reported that a confined agent cannot reach origin on evidence
// entirely about the absence of a repo.
//
// So the box is handed a real repository with a real origin the parent has
// already pushed to, and the probe establishes its own premises before it reads
// the push. A repository, an origin and a commit are all on the source side,
// all in the box's granted temporary directory, and all legible from inside:
// if one is missing, the push would decline about the sentinel rather than
// about origin, and the probe reports absent — which 0017 counts as a failure
// rather than as a boundary that held. Only with the three in place is a
// non-zero push an answer about reaching origin.
//
// What the probe cannot tell apart is an origin that is refused from an origin
// that is not there. That is provePush's question, answered in the parent
// before the child runs, the way proveWritable answers it for the write probes.
//
// The network half of reaching origin stays with host_outside_allowlist, which
// dials a port kept never named. This probe's origin is a path because that is
// what a box's origin is: 0017 puts the local repository there precisely so no
// credential reaching a remote has to exist in the box at all.
func pushProbe(git, repo string, timeout time.Duration) probeResult {
	// Not execProbe's stat, which reads a refused stat as the boundary. That is
	// right where the executable *is* what the policy denies; here the boundary
	// is origin, and a git the box cannot reach is a probe that measured
	// nothing about origin either way.
	if _, err := os.Stat(git); err != nil {
		return probeResult{outcome: probeAbsent, detail: probeDetail(fmt.Errorf(
			"origin_push has no git to push with at %s: %w", git, err))}
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	// Two streams, never one. What a premise reads is a value, and a value taken
	// from combined output is whatever the instrument also wrote on its way in:
	// measured, Apple's /usr/bin/git shim writes `couldn't create cache file` to
	// stderr on every invocation, and `remote get-url origin` read back as those
	// two lines followed by the URL. A record naming that as the origin is the
	// probe reporting on the shim's scratch directory instead of on origin.
	run := func(args ...string) (string, string, error) {
		cmd := exec.CommandContext(ctx, git, args...)
		cmd.Env = gitProbeEnvironment()
		var value, diagnostic strings.Builder
		cmd.Stdout, cmd.Stderr = &value, &diagnostic
		err := cmd.Run()
		return strings.TrimSpace(value.String()), strings.TrimSpace(diagnostic.String()), err
	}
	var origin string
	for _, premise := range []struct {
		missing string
		args    []string
		into    *string
	}{
		// git first, and without -C, so this asks about the instrument and
		// nothing else. A git that cannot start in the box has measured nothing
		// about origin, and reading its exit as a refusal is the same mistake
		// as reading `not a git repository` as one. It is not hypothetical: the
		// shim reaches for the host's per-user temporary directory through
		// confstr rather than TMPDIR, so it always writes somewhere the profile
		// denies. That is non-fatal today — measured, `git --version` exits 0
		// inside a box and the push that follows reaches origin and is refused
		// there — and this premise is what keeps it from ever being read as a
		// boundary if it stops being non-fatal.
		{missing: "a git that can start", args: []string{"--version"}},
		{missing: "a repository to push from", args: []string{"-C", repo, "rev-parse", "--git-dir"}},
		{missing: "an origin to push to", args: []string{"-C", repo, "remote", "get-url", "origin"}, into: &origin},
		{missing: "a commit to push", args: []string{"-C", repo, "rev-parse", "--verify", "--quiet", "HEAD"}},
	} {
		value, diagnostic, err := run(premise.args...)
		if err != nil {
			return probeResult{outcome: probeAbsent, detail: probeDetail(fmt.Errorf(
				"origin_push has no %s at %s, so a push would decline about itself and not about origin: %w: %s",
				premise.missing, repo, err, diagnostic))}
		}
		if premise.into != nil {
			*premise.into = value
		}
	}
	// The origin is named in the detail by the probe rather than left to git's
	// message, so what the outcome is about is in the record whichever way the
	// push failed.
	_, diagnostic, err := run("-C", repo, "push", "origin", "HEAD:"+canaryPushRef)
	if err == nil {
		return probeResult{outcome: probeReached, detail: probeDetail(nil) + ": pushed " + canaryPushRef + " to origin at " + origin}
	}
	return probeResult{outcome: probeRefused, detail: probeDetail(fmt.Errorf("push %s to origin at %s: %w: %s", canaryPushRef, origin, err, diagnostic))}
}

// gitProbeEnvironment is what the probe's git runs with. It carries no
// credential and cannot ask for one — a prompt inside an unattended box is a
// hang, and an askpass that answers is a credential the box was not supposed to
// hold. TMPDIR comes through because the profile denies the host's scratch
// trees by name, so a git that reached for one would fail at a path nobody
// granted rather than at origin.
func gitProbeEnvironment() []string {
	env := []string{"GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=/bin/false", "GIT_CONFIG_NOSYSTEM=1"}
	if temporary := os.Getenv("TMPDIR"); temporary != "" {
		env = append(env, "TMPDIR="+temporary)
	}
	return env
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
// pushProbe stats separately, because there the executable is the instrument
// rather than the thing denied.
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
