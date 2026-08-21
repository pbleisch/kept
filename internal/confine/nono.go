//go:build !windows

package confine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/pbleisch/kept/internal/thread"
)

// Profile is kept's confinement policy for one repair box. It is deliberately
// not nono's profile shape: callers derive Read and Write from the promise's
// bound repair scope, name the domain allowlist and the one child environment
// they are willing to inject, and this package renders the supported subset.
//
// Every path is absolute. Read and Write must stay below Box and must not name
// .kept: the machine-written tier is not part of a repair tree. Ledger and
// Artifacts are the independently protected side-ref locations the canary
// proves unreachable.
type Profile struct {
	// Nono and Kept are explicit executable paths. Neither provider nor
	// canary resolves a command on PATH: a path chosen by the operator is part
	// of the policy that is verified.
	Nono string
	Kept string
	Git  string

	Box       string
	Read      []string
	Write     []string
	Domains   []string
	Env       []string
	Ledger    string
	Artifacts string

	// CanaryTimeout bounds the provider verification. Zero is refused rather
	// than turning a broken provider into an unbounded external invocation.
	CanaryTimeout time.Duration
}

// Nono is a production ConfinementLaunch. It invokes `nono run` and owns the
// process group it starts; the repair runtime never receives a process handle.
// One Nono belongs to one profile and verifies that profile before its first
// launch. A caller that changes any policy data constructs a new Nono and gets
// a fresh canary rather than accidentally carrying a prior result forward.
type Nono struct {
	profile Profile

	verifyOnce sync.Once
	verifyErr  error
}

// NewNono validates the policy before a process exists. A profile that the
// provider cannot express is refused here, rather than narrowed silently at
// launch. It does no provider I/O; Verified performs the bounded real canary.
func NewNono(profile Profile) (*Nono, error) {
	profile = cloneProfile(profile)
	if err := normalizeBoxPaths(&profile); err != nil {
		return nil, err
	}
	if err := validateProfile(profile); err != nil {
		return nil, err
	}
	return &Nono{profile: profile}, nil
}

func cloneProfile(profile Profile) Profile {
	profile.Read = append([]string(nil), profile.Read...)
	profile.Write = append([]string(nil), profile.Write...)
	profile.Domains = append([]string(nil), profile.Domains...)
	profile.Env = append([]string(nil), profile.Env...)
	return profile
}

func normalizeBoxPaths(profile *Profile) error {
	if profile.Box == "" || !filepath.IsAbs(profile.Box) {
		return nil
	}
	box, err := canonicalPath(profile.Box)
	if err != nil {
		return fmt.Errorf("resolve confinement box: %w", err)
	}
	profile.Box = box
	for _, paths := range []*[]string{&profile.Read, &profile.Write} {
		for i, path := range *paths {
			if err := validateBoxPath(box, path); err != nil {
				return err
			}
			resolved, err := canonicalPath(path)
			if err != nil {
				return fmt.Errorf("resolve confinement scope path %s: %w", path, err)
			}
			(*paths)[i] = resolved
		}
	}
	return nil
}

func validateProfile(profile Profile) error {
	for _, executable := range []struct {
		name string
		path string
	}{
		{"nono executable", profile.Nono},
		{"kept executable", profile.Kept},
		{"git executable", profile.Git},
	} {
		if executable.path == "" {
			return fmt.Errorf("nono confinement %s is unset", executable.name)
		}
		if !filepath.IsAbs(executable.path) {
			return fmt.Errorf("nono confinement %s is not an explicit absolute path: %s", executable.name, executable.path)
		}
	}
	if profile.Box == "" || !filepath.IsAbs(profile.Box) {
		return errors.New("nono confinement box is not an explicit absolute path")
	}
	if profile.Ledger == "" || !filepath.IsAbs(profile.Ledger) {
		return errors.New("nono confinement ledger is not an explicit absolute path")
	}
	if profile.Artifacts == "" || !filepath.IsAbs(profile.Artifacts) {
		return errors.New("nono confinement artifacts are not an explicit absolute path")
	}
	if profile.CanaryTimeout <= 0 {
		return errors.New("nono confinement canary deadline is unset")
	}
	if len(profile.Read) == 0 {
		return errors.New("nono confinement has no scope-derived readable paths")
	}
	if len(profile.Write) == 0 {
		return errors.New("nono confinement has no scope-derived writable paths")
	}
	if len(profile.Domains) == 0 {
		// A non-empty domain rule is what makes nono start a proxy. `run`
		// without one can report a proxy while the child has unrestricted
		// networking; decision 0017 rejects that configuration.
		return errors.New("nono confinement domain allowlist is empty")
	}
	if len(profile.Env) == 0 {
		return errors.New("nono confinement child environment allowlist is empty")
	}

	box := profile.Box
	for _, path := range append(append([]string(nil), profile.Read...), profile.Write...) {
		if err := validateBoxPath(box, path); err != nil {
			return err
		}
	}
	for _, path := range []string{profile.Kept, profile.Ledger, profile.Artifacts} {
		if within(box, path) {
			return fmt.Errorf("nono confinement protected path is inside the box: %s", path)
		}
	}
	for _, name := range profile.Env {
		if !environmentName(name) {
			return fmt.Errorf("nono confinement child environment name is invalid: %q", name)
		}
		if originCredentialEnvironment(name) {
			return fmt.Errorf("nono confinement child environment %s can reach origin", name)
		}
	}
	for _, domain := range profile.Domains {
		if strings.TrimSpace(domain) == "" || strings.ContainsAny(domain, "\x00\r\n") {
			return fmt.Errorf("nono confinement domain is invalid: %q", domain)
		}
	}
	return nil
}

func originCredentialEnvironment(name string) bool {
	switch name {
	case "GIT_ASKPASS", "GIT_SSH", "GIT_SSH_COMMAND", "GIT_TOKEN", "GH_TOKEN", "GITHUB_TOKEN", "SSH_AUTH_SOCK":
		return true
	default:
		return false
	}
}

func canonicalPath(path string) (string, error) {
	return filepath.EvalSymlinks(path)
}

func validateBoxPath(box, path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("nono confinement scope path is not an explicit absolute path: %s", path)
	}
	for _, part := range strings.Split(filepath.Clean(path), string(filepath.Separator)) {
		if part == ".kept" {
			return fmt.Errorf("nono confinement scope path reaches .kept or its parent: %s", path)
		}
	}
	// Check .kept before resolving the path: a profile must be refused even
	// when that protected directory does not exist in this particular box.
	rel, err := filepath.Rel(box, path)
	if err == nil && (rel == "." || rel == ".kept" || strings.HasPrefix(rel, ".kept"+string(filepath.Separator))) {
		return fmt.Errorf("nono confinement scope path reaches .kept or its parent: %s", path)
	}
	resolved, err := canonicalPath(path)
	if err == nil {
		path = resolved
	}
	if !within(box, path) {
		return fmt.Errorf("nono confinement scope path is outside the box: %s", path)
	}
	rel, err = filepath.Rel(box, path)
	if err != nil {
		return fmt.Errorf("relate confinement scope path %s: %w", path, err)
	}
	if rel == "." || rel == ".kept" || strings.HasPrefix(rel, ".kept"+string(filepath.Separator)) {
		return fmt.Errorf("nono confinement scope path reaches .kept or its parent: %s", path)
	}
	return nil
}

func within(parent, path string) bool {
	rel, err := filepath.Rel(parent, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func environmentName(name string) bool {
	for i, r := range name {
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r == '_' {
			continue
		}
		if i > 0 && r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return name != ""
}

// Verified runs the real canary exactly once for this immutable profile. A
// failed or timed-out result remains failed: it is evidence about this launch,
// not a transient affirmative to retry until it says yes.
func (n *Nono) Verified() error {
	n.verifyOnce.Do(func() { n.verifyErr = n.verify() })
	return n.verifyErr
}

// Start starts a directly shaped runtime invocation under `nono run`. The
// caller has already called Verified; Start repeats neither canary nor policy
// selection. Its Child kills the complete nono-led process group and reaps it.
func (n *Nono) Start(ctx context.Context, invocation thread.Invocation) (thread.Child, error) {
	if ctx == nil {
		return nil, errors.New("nono confinement launch context is nil")
	}
	if err := n.Verified(); err != nil {
		return nil, fmt.Errorf("nono confinement canary failed: %w", err)
	}
	if invocation.Path == "" || !filepath.IsAbs(invocation.Path) {
		return nil, errors.New("nono confinement runtime executable is not an explicit absolute path")
	}
	if err := validateInvocationEnvironment(n.profile.Env, invocation.Env); err != nil {
		return nil, err
	}
	return n.start(ctx, invocation, nil)
}

func validateInvocationEnvironment(allowed, values []string) error {
	allowedSet := make(map[string]bool, len(allowed))
	for _, name := range allowed {
		allowedSet[name] = true
	}
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		name, _, found := strings.Cut(value, "=")
		if !found || !environmentName(name) {
			return fmt.Errorf("nono confinement child environment is malformed: %q", value)
		}
		if seen[name] {
			return fmt.Errorf("nono confinement child environment names %s twice", name)
		}
		if !allowedSet[name] {
			return fmt.Errorf("nono confinement child environment %s is not in the profile allowlist", name)
		}
		seen[name] = true
	}
	return nil
}

func (n *Nono) start(ctx context.Context, invocation thread.Invocation, canaryDeny []string) (thread.Child, error) {
	dir, err := os.MkdirTemp("", "kept-nono-")
	if err != nil {
		return nil, fmt.Errorf("create nono launch state: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(dir) }

	profile := cloneProfile(n.profile)
	if canaryDeny != nil {
		profile.Env = append([]string(nil), canaryEnvironment...)
	}
	profilePath := filepath.Join(dir, "profile.json")
	if err := writeNonoProfile(profilePath, profile, invocation.Path, canaryDeny); err != nil {
		cleanup()
		return nil, err
	}

	args := []string{"run", "--silent", "--no-audit", "--no-rollback", "--profile", profilePath, "--", invocation.Path}
	args = append(args, invocation.Args...)
	cmd := exec.Command(n.profile.Nono, args...)
	cmd.Dir = profile.Box
	cmd.Env = childEnvironment(invocation.Env)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("open nono runtime input: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("open nono runtime output: %w", err)
	}
	stderr := &boundedBuffer{limit: 8 << 10}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		cleanup()
		return nil, fmt.Errorf("start nono run: %w", err)
	}
	if invocation.BindInput != nil {
		if err := invocation.BindInput(stdin); err != nil {
			_ = killProcessGroup(cmd.Process.Pid)
			_ = cmd.Wait()
			cleanup()
			return nil, fmt.Errorf("bind confined runtime input: %w", err)
		}
	}
	if len(invocation.Input) != 0 {
		if _, err := stdin.Write(invocation.Input); err != nil {
			_ = killProcessGroup(cmd.Process.Pid)
			_ = cmd.Wait()
			cleanup()
			return nil, fmt.Errorf("write confined runtime input: %w", err)
		}
	}
	if invocation.BindInput == nil {
		if err := stdin.Close(); err != nil {
			_ = killProcessGroup(cmd.Process.Pid)
			_ = cmd.Wait()
			cleanup()
			return nil, fmt.Errorf("close confined runtime input: %w", err)
		}
	}

	child := &child{cmd: cmd, stdin: stdin, cleanup: cleanup, stderr: stderr}
	child.copyDone = make(chan struct{})
	go func() {
		if invocation.Output != nil {
			_, child.copyErr = io.Copy(invocation.Output, stdout)
		} else {
			_, child.copyErr = io.Copy(io.Discard, stdout)
		}
		close(child.copyDone)
	}()
	return child, nil
}

func childEnvironment(injected []string) []string {
	merged := make(map[string]string, len(os.Environ())+len(injected))
	for _, value := range os.Environ() {
		name, _, found := strings.Cut(value, "=")
		if found {
			merged[name] = value
		}
	}
	for _, value := range injected {
		name, _, _ := strings.Cut(value, "=")
		merged[name] = value
	}
	names := make([]string, 0, len(merged))
	for name := range merged {
		names = append(names, name)
	}
	sort.Strings(names)
	output := make([]string, 0, len(names))
	for _, name := range names {
		output = append(output, merged[name])
	}
	return output
}

type nonoProfile struct {
	Filesystem struct {
		Read  []string `json:"read"`
		Allow []string `json:"allow"`
		Deny  []string `json:"deny"`
	} `json:"filesystem"`
	Network struct {
		AllowDomain []string `json:"allow_domain"`
	} `json:"network"`
	Environment struct {
		Allow []string `json:"allow_vars"`
	} `json:"environment"`
	CommandPolicies map[string]any `json:"command_policies"`
}

func writeNonoProfile(path string, profile Profile, entrypoint string, extraDeny []string) error {
	var rendered nonoProfile
	rendered.Filesystem.Read = append([]string(nil), profile.Read...)
	rendered.Filesystem.Allow = append([]string(nil), profile.Write...)
	rendered.Filesystem.Deny = append([]string{profile.Kept, profile.Ledger, profile.Artifacts}, extraDeny...)
	rendered.Network.AllowDomain = append([]string(nil), profile.Domains...)
	rendered.Environment.Allow = append([]string(nil), profile.Env...)
	// Filesystem denial is not enough for an executable that macOS has already
	// mapped for a process. nono's tool sandbox guards the direct session edge
	// as well, so the known kept binary cannot be re-executed to ratify from a
	// repair even if the host's executable cache can read it.
	entryName := filepath.Base(entrypoint)
	if entryName == filepath.Base(profile.Kept) {
		return errors.New("nono confinement runtime executable is the denied kept executable")
	}
	rendered.CommandPolicies = map[string]any{
		"entrypoint": entryName,
		"commands": map[string]any{
			entryName: map[string]any{
				"executable":                entrypoint,
				"sandbox":                   map[string]any{},
				"allow_writable_executable": true,
				"allow_direct_exec_bypass":  []string{entrypoint},
			},
			filepath.Base(profile.Kept): map[string]any{
				"executable":                profile.Kept,
				"from":                      map[string]any{"session": "deny"},
				"allow_writable_executable": true,
			},
		},
	}
	encoded, err := json.Marshal(rendered)
	if err != nil {
		return fmt.Errorf("render nono profile: %w", err)
	}
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		return fmt.Errorf("write nono profile: %w", err)
	}
	return nil
}

type child struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	cleanup func()
	stderr  *boundedBuffer

	copyDone chan struct{}
	copyErr  error
	waitOnce sync.Once
	exit     thread.Exit
}

func (c *child) diagnostic() string { return c.stderr.String() }

type boundedBuffer struct {
	mu        sync.Mutex
	bytes     bytes.Buffer
	limit     int
	truncated bool
}

func (b *boundedBuffer) Write(src []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	remaining := b.limit - b.bytes.Len()
	if b.bytes.Len() < b.limit {
		if remaining > len(src) {
			remaining = len(src)
		}
		_, _ = b.bytes.Write(src[:remaining])
	}
	if len(src) > remaining {
		b.truncated = true
	}
	return len(src), nil
}

func (b *boundedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.TrimSpace(b.bytes.String())
}

func (b *boundedBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.bytes.Bytes()...)
}

func (b *boundedBuffer) Truncated() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.truncated
}

func (c *child) Kill() error {
	return killProcessGroup(c.cmd.Process.Pid)
}

func killProcessGroup(pid int) error {
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
		return fmt.Errorf("kill nono process group %d: %w", pid, err)
	}
	return nil
}

func (c *child) Wait() thread.Exit {
	c.waitOnce.Do(func() {
		waitErr := c.cmd.Wait()
		if c.stdin != nil {
			_ = c.stdin.Close()
		}
		<-c.copyDone
		c.exit = processExit(c.cmd.ProcessState, waitErr)
		c.cleanup()
	})
	return c.exit
}

func processExit(state *os.ProcessState, waitErr error) thread.Exit {
	if state != nil {
		if status, ok := state.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			return thread.Exit{Signal: status.Signal().String()}
		}
		code := state.ExitCode()
		if code >= 0 {
			return thread.Exit{Code: &code}
		}
	}
	// A process that could not be reaped is still an observed failure, not a
	// successful runtime. Exit's shape has no transport-error field, so retain
	// the conventional nonzero code while callers surface launch errors at the
	// boundary that produced them.
	code := 1
	if waitErr == nil {
		code = 0
	}
	return thread.Exit{Code: &code}
}

func (n *Nono) verify() error {
	ctx, cancel := context.WithTimeout(context.Background(), n.profile.CanaryTimeout)
	defer cancel()

	state, err := makeCanaryState(n.profile)
	if err != nil {
		return err
	}
	defer state.close()

	helperDir, err := os.MkdirTemp("", "kept-nono-canary-helper-")
	if err != nil {
		return fmt.Errorf("create nono canary helper state: %w", err)
	}
	defer func() {
		_ = os.Chmod(helperDir, 0o700)
		_ = os.RemoveAll(helperDir)
	}()
	copyPath, err := copyCanary(n.profile.Kept, helperDir)
	if err != nil {
		return err
	}
	// Tool-sandbox correctly refuses an entrypoint whose parent is writable by
	// the process launching it. The parent creates this one-time helper, then
	// makes both the file and its directory immutable before handing it to
	// nono; neither belongs to the repair profile.
	if err := os.Chmod(copyPath, 0o500); err != nil {
		return fmt.Errorf("seal nono canary executable: %w", err)
	}
	if err := os.Chmod(helperDir, 0o500); err != nil {
		return fmt.Errorf("seal nono canary helper directory: %w", err)
	}
	input := thread.Invocation{
		Path:   copyPath,
		Args:   []string{canaryArgument},
		Env:    state.environment(),
		Output: &state.outputBuf,
	}
	child, err := n.start(ctx, input, state.deniedPaths())
	if err != nil {
		return err
	}

	done := make(chan thread.Exit, 1)
	go func() { done <- child.Wait() }()
	select {
	case exit := <-done:
		if state.outputBuf.Truncated() {
			return errors.New("nono canary report exceeded its bound")
		}
		report, err := parseCanaryReport(state.output())
		if err != nil {
			detail := ""
			if diagnostic, ok := child.(interface{ diagnostic() string }); ok {
				detail = diagnostic.diagnostic()
			}
			return fmt.Errorf("nono canary did not complete (%s): %w: %s", describeExit(exit), err, detail)
		}
		if err := report.valid(); err != nil {
			return err
		}
		if exit.Code == nil || *exit.Code != 0 {
			return fmt.Errorf("nono canary reported success but exited %s", describeExit(exit))
		}
		return nil
	case <-ctx.Done():
		if err := child.Kill(); err != nil {
			return fmt.Errorf("nono canary timed out: %w; kill canary: %v", ctx.Err(), err)
		}
		<-done
		return fmt.Errorf("nono canary timed out: %w", ctx.Err())
	}

}

func describeExit(exit thread.Exit) string {
	if exit.Code != nil {
		return fmt.Sprintf("code %d", *exit.Code)
	}
	if exit.Signal != "" {
		return "signal " + exit.Signal
	}
	return "an unknown exit"
}

func copyCanary(source, dir string) (string, error) {
	input, err := os.Open(source)
	if err != nil {
		return "", fmt.Errorf("open kept canary executable: %w", err)
	}
	defer input.Close()
	copy := filepath.Join(dir, "canary")
	output, err := os.OpenFile(copy, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o700)
	if err != nil {
		return "", fmt.Errorf("create kept canary executable: %w", err)
	}
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		return "", fmt.Errorf("copy kept canary executable: %w", err)
	}
	if err := output.Close(); err != nil {
		return "", fmt.Errorf("close kept canary executable: %w", err)
	}
	return copy, nil
}

type canaryState struct {
	dir         string
	credential  string
	outside     string
	unix        net.Listener
	tcp         net.Listener
	profile     canaryPolicyPaths
	outputBuf   canaryOutput
	createdKept bool
}

type canaryOutput struct{ output boundedBuffer }

func (o *canaryOutput) Write(src []byte) (int, error) { return o.output.Write(src) }
func (*canaryOutput) Close() error                    { return nil }
func (o *canaryOutput) Bytes() []byte                 { return o.output.Bytes() }
func (o *canaryOutput) Truncated() bool               { return o.output.Truncated() }
