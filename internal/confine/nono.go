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
	"slices"
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
// Every path is absolute and stays below Box, and neither set may name the
// box's own .kept: the machine-written tier is not part of a repair tree.
// Read names the box root, because that is the confined child's working
// directory and a child that cannot read it cannot resolve its own cwd. Write
// never does: the writable set is the promise's repair scope, and the box root
// is the whole tree. Ledger and Artifacts are the independently protected
// side-ref locations the canary proves unreachable.
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
	for _, set := range []struct {
		want  grant
		paths *[]string
	}{
		{readable, &profile.Read},
		{writable, &profile.Write},
	} {
		for i, path := range *set.paths {
			if err := validateBoxPath(box, path, set.want); err != nil {
				return err
			}
			resolved, err := canonicalPath(path)
			if err != nil {
				return fmt.Errorf("resolve confinement scope path %s: %w", path, err)
			}
			(*set.paths)[i] = resolved
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
	for _, root := range hostTemporaryRoots() {
		// Denial beats a grant, so a box under a root the profile takes back is
		// a box the confined child cannot read, cannot write and cannot resolve
		// its own working directory in — while every escape probe is refused
		// and the canary passes. Measured. That is the failure this whole
		// package's canary exists to catch, so the profile is refused here
		// rather than rendered into a boundary that proves nothing.
		if within(root, canonicalAncestor(profile.Box)) {
			return fmt.Errorf("nono confinement box is inside the host temporary tree the profile denies at %s: %s", root, profile.Box)
		}
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
	if !slices.Contains(profile.Read, profile.Box) {
		// Nono.start makes the box root the child's working directory. A
		// readable set that omits it produces a child whose getcwd is refused,
		// which reads downstream as a build that cannot find its own module
		// rather than as a confinement that was configured wrong.
		return errors.New("nono confinement readable paths do not name the box root, which is the confined working directory")
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
	for _, set := range []struct {
		want  grant
		paths []string
	}{
		{readable, profile.Read},
		{writable, profile.Write},
	} {
		for _, path := range set.paths {
			if err := validateBoxPath(box, path, set.want); err != nil {
				return err
			}
		}
	}
	for _, path := range []string{profile.Kept, profile.Ledger, profile.Artifacts} {
		// Resolve before comparing: the box is canonical, so a protected path
		// written through a symlinked prefix would otherwise relate as outside
		// a box it is in fact inside.
		if within(box, canonicalAncestor(path)) {
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

// canonicalAncestor resolves the deepest existing ancestor of path and rejoins
// the remainder. Existence is not a precondition for relating a path to the
// box: a scope path may name a directory this box does not have, and a
// protected location may never be materialized at all, because the ledger lives
// on a side ref. EvalSymlinks answers nothing about a path that is not there,
// and on a machine whose temporary tree is itself a symlink an unresolved path
// then relates as outside a box it is in fact inside.
func canonicalAncestor(path string) string {
	path = filepath.Clean(path)
	remainder := ""
	for {
		if resolved, err := canonicalPath(path); err == nil {
			return filepath.Join(resolved, remainder)
		}
		parent := filepath.Dir(path)
		if parent == path {
			return filepath.Join(path, remainder)
		}
		remainder = filepath.Join(filepath.Base(path), remainder)
		path = parent
	}
}

// grant is what one scope path is being validated for. The two differ at the
// box root and nowhere else, and the difference is the boundary: the root is
// the confined child's working directory and must be readable, while making it
// writable would hand a repair the whole tree instead of the scope its promise
// bound. See docs/decision/0047-a-promise-binds-what-a-repair-may-touch.md.
type grant string

const (
	readable grant = "readable"
	writable grant = "writable"
)

// hiddenTier is the machine-written directory, relative to a tree's root.
const hiddenTier = ".kept"

// validateBoxPath decides whether one scope path may carry one grant.
//
// The hidden tier is located relative to the box and never by scanning the
// absolute path for a component named .kept. A box lives at
// <repository>/.kept/boxes/<id>, so every path inside an ordinary box carries
// that component and a scan refuses every box internal/box creates. What has to
// stay out of reach is the machine-written tier itself — the ledger, the
// artifacts and kept's own state — and all of that lies outside the box:
// containment is the check that keeps a scope path away from it, and
// validateProfile separately refuses a profile that puts one of them inside.
func validateBoxPath(box, path string, want grant) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("nono confinement scope path is not an explicit absolute path: %s", path)
	}
	// Relate the path as written as well as resolved. The written form refuses a
	// name that reaches the hidden tier through a symlink pointing somewhere
	// innocent; the resolved form refuses a symlink that reaches it under an
	// innocent name. Containment is asked only of the resolved form, because an
	// unresolved path can differ from the canonical box by a symlinked prefix.
	if err := validateBoxRelation(box, path, path, want); err != nil {
		return err
	}
	resolved := canonicalAncestor(path)
	if !within(box, resolved) {
		return fmt.Errorf("nono confinement scope path is outside the box: %s", path)
	}
	return validateBoxRelation(box, resolved, path, want)
}

// validateBoxRelation applies the rules about where inside the box a path may
// lie. reported is the path as the caller wrote it, so a refusal names what was
// configured rather than what a symlink resolved to.
func validateBoxRelation(box, path, reported string, want grant) error {
	rel, err := filepath.Rel(box, path)
	if err != nil {
		return fmt.Errorf("relate confinement scope path %s: %w", reported, err)
	}
	if rel == hiddenTier || strings.HasPrefix(rel, hiddenTier+string(filepath.Separator)) {
		return fmt.Errorf("nono confinement scope path reaches .kept inside the box: %s", reported)
	}
	if rel == "." && want == writable {
		return fmt.Errorf("nono confinement writable scope path is the box root: %s", reported)
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
	return n.start(ctx, invocation, repairLaunch)
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

// launchKind is which of the two things this package starts. The rendered
// policy is the same for both, which is what makes the canary evidence about a
// repair rather than about itself; only the child environment differs, because
// the confined helper is told where its sentinels are.
type launchKind bool

const (
	repairLaunch launchKind = false
	canaryLaunch launchKind = true
)

func (n *Nono) start(ctx context.Context, invocation thread.Invocation, kind launchKind) (thread.Child, error) {
	dir, err := os.MkdirTemp("", "kept-nono-")
	if err != nil {
		return nil, fmt.Errorf("create nono launch state: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(dir) }

	profile := cloneProfile(n.profile)
	if kind == canaryLaunch {
		profile.Env = append([]string(nil), canaryEnvironment...)
	}
	profilePath := filepath.Join(dir, "profile.json")
	if err := writeNonoProfile(profilePath, profile, invocation.Path); err != nil {
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

// hostTemporaryRoots are the host's scratch trees, which the profile takes back
// from the provider's baseline.
//
// They are the largest grant nothing in kept ever asked for: nono's baseline
// grants the operating system's temporary trees write access *recursively*, so
// a repair that was refused every path kept names could still write anywhere
// under them — measured, from inside a rendered profile, before this entry
// existed. Nothing narrower closes it: denying /tmp alone leaves the per-user
// root under /var/folders granted, and on darwin the grant covers that whole
// tree rather than only its T directory.
//
// A path here is not conditional on the platform. A root that does not exist
// cannot be reached and the provider accepts naming it, while omitting one per
// platform would make the rendered policy differ from the policy read.
// Consequences: a box under one of these roots is refused outright rather than
// silently unconfined, which is why a test that measures the boundary cuts its
// box under the operator's home; and a confined child is left with no writable
// temporary directory at all until one inside the box is granted to it.
func hostTemporaryRoots() []string {
	seen := make(map[string]bool, 8)
	roots := make([]string, 0, 8)
	add := func(path string) {
		path = filepath.Clean(path)
		// The filesystem root would deny the box along with everything else.
		if path == "" || path == string(filepath.Separator) || seen[path] {
			return
		}
		seen[path] = true
		roots = append(roots, path)
	}
	for _, root := range []string{"/tmp", "/var/tmp", "/var/folders", os.TempDir()} {
		// Both forms: the provider resolves a deny entry, and a policy that
		// depends on it resolving one is a policy that reads differently from
		// what it enforces.
		add(root)
		add(canonicalAncestor(root))
	}
	return roots
}

func writeNonoProfile(path string, profile Profile, entrypoint string) error {
	var rendered nonoProfile
	rendered.Filesystem.Read = append([]string(nil), profile.Read...)
	rendered.Filesystem.Allow = append([]string(nil), profile.Write...)
	// The box root is readable, so the box's own machine-written tier is denied
	// by name rather than by being left ungranted. Denial beats a read grant, so
	// this is what keeps kept's state out of a repair's reach once the tree
	// around it is legible. The canary's write_kept probe is aimed at this
	// entry, which is policy every launch carries rather than a canary-only one.
	rendered.Filesystem.Deny = append([]string{
		profile.Kept,
		profile.Ledger,
		profile.Artifacts,
		filepath.Join(profile.Box, hiddenTier),
	}, hostTemporaryRoots()...)
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

	copyPath, release, err := sealedCanary(n.profile.Kept)
	if err != nil {
		return err
	}
	defer release()
	input := thread.Invocation{
		Path:   copyPath,
		Args:   []string{canaryArgument},
		Env:    state.environment(),
		Output: &state.outputBuf,
	}
	child, err := n.start(ctx, input, canaryLaunch)
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

// sealedCanary copies the configured kept executable somewhere the launching
// process cannot then change, and returns the copy with the release that
// removes it.
//
// Tool-sandbox correctly refuses an entrypoint whose parent is writable by the
// process launching it, so both the file and its directory are made immutable
// before nono sees them. Neither belongs to the repair profile.
func sealedCanary(kept string) (string, func(), error) {
	dir, err := os.MkdirTemp("", "kept-nono-canary-helper-")
	if err != nil {
		return "", nil, fmt.Errorf("create nono canary helper state: %w", err)
	}
	release := func() {
		_ = os.Chmod(dir, 0o700)
		_ = os.RemoveAll(dir)
	}
	copyPath, err := copyCanary(kept, dir)
	if err != nil {
		release()
		return "", nil, err
	}
	if err := os.Chmod(copyPath, 0o500); err != nil {
		release()
		return "", nil, fmt.Errorf("seal nono canary executable: %w", err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		release()
		return "", nil, fmt.Errorf("seal nono canary helper directory: %w", err)
	}
	return copyPath, release, nil
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
	hostTemp    string
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
