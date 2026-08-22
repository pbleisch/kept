//go:build !windows

package confine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/run"
	"github.com/pbleisch/kept/internal/thread"
)

func TestNonoRefusesAProfileItCannotEnforce(t *testing.T) {
	profile := fixtureProfile(t)
	for _, test := range []struct {
		name string
		edit func(*Profile)
		want string
	}{
		{"no domains", func(p *Profile) { p.Domains = nil }, "domain allowlist is empty"},
		{"write outside box", func(p *Profile) { p.Write = []string{t.TempDir()} }, "outside the box"},
		{"write kept tier", func(p *Profile) { p.Write = []string{filepath.Join(p.Box, ".kept")} }, "reaches .kept"},
		{"read kept tier", func(p *Profile) { p.Read = append(p.Read, filepath.Join(p.Box, ".kept")) }, "reaches .kept"},
		{"writable box root", func(p *Profile) { p.Write = append(p.Write, p.Box) }, "writable scope path is the box root"},
		{"unreadable box root", func(p *Profile) { p.Read = []string{filepath.Join(p.Box, "read")} }, "do not name the box root"},
		{"unlisted environment", func(p *Profile) { p.Env = []string{"bad-name"} }, "environment name is invalid"},
		{"origin credential", func(p *Profile) { p.Env = []string{"GIT_TOKEN"} }, "can reach origin"},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := cloneProfile(profile)
			test.edit(&changed)
			if _, err := NewNono(changed); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("NewNono error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestNonoExpressesABoxBeneathTheHiddenTier(t *testing.T) {
	// internal/box puts every box at <repository>/.kept/boxes/<id>, so every
	// scope path in an ordinary box carries a .kept component. Refusing on that
	// component refuses the whole design; what must be refused is the tier
	// relative to the box.
	repository := t.TempDir()
	box := filepath.Join(repository, ".kept", "boxes", "0123456789abcdef")
	scope := filepath.Join(box, "internal")
	if err := os.MkdirAll(scope, 0o700); err != nil {
		t.Fatal(err)
	}
	profile := fixtureProfile(t)
	profile.Box = box
	profile.Read = []string{box}
	profile.Write = []string{scope}
	profile.Ledger = filepath.Join(repository, ".kept", "ledger")
	profile.Artifacts = filepath.Join(repository, ".kept", "artifacts")

	launch, err := NewNono(profile)
	if err != nil {
		t.Fatalf("NewNono refused a box the box package creates: %v", err)
	}
	if got := launch.profile.Read; len(got) != 1 || got[0] != canonicalAncestor(box) {
		t.Fatalf("normalized readable set = %v, want the box root", got)
	}

	// The box's own hidden tier stays refused, which is the protection the
	// component scan was reaching for.
	refused := cloneProfile(profile)
	refused.Read = append(refused.Read, filepath.Join(box, ".kept"))
	if _, err := NewNono(refused); err == nil || !strings.Contains(err.Error(), "reaches .kept") {
		t.Fatalf("NewNono error = %v, want the box's own .kept refused", err)
	}
}

func TestNonoRefusesAScopePathThatWouldReachAProtectedLocation(t *testing.T) {
	// Nothing the ledger, the artifacts or the kept executable live in may be
	// named by a scope path, and containment is what enforces it: a scope path
	// stays inside the box and a protected location stays outside it. Both
	// halves are checked here, including the half that a symlinked prefix would
	// otherwise defeat.
	profile := fixtureProfile(t)
	repository := filepath.Dir(profile.Box)
	for _, test := range []struct {
		name string
		edit func(*Profile)
		want string
	}{
		{"scope names the ledger", func(p *Profile) { p.Read = append(p.Read, p.Ledger) }, "outside the box"},
		{"scope names the artifacts", func(p *Profile) { p.Write = []string{p.Artifacts} }, "outside the box"},
		{"scope names the repository above the box", func(p *Profile) { p.Read = append(p.Read, repository) }, "outside the box"},
		{"scope names the kept executable's directory", func(p *Profile) {
			p.Kept = filepath.Join(repository, "bin", "kept")
			p.Read = append(p.Read, filepath.Dir(p.Kept))
		}, "outside the box"},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := cloneProfile(profile)
			test.edit(&changed)
			if _, err := NewNono(changed); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("NewNono error = %v, want %q", err, test.want)
			}
		})
	}

	// The other half: a protected location may not be moved inside the box,
	// where the readable box root would reach it. The path is deliberately
	// written through the unresolved temporary prefix, which is the form that
	// relates as outside a box it is inside.
	for _, test := range []struct {
		name string
		set  func(*Profile, string)
	}{
		{"ledger inside the box", func(p *Profile, path string) { p.Ledger = path }},
		{"artifacts inside the box", func(p *Profile, path string) { p.Artifacts = path }},
		{"kept executable inside the box", func(p *Profile, path string) { p.Kept = path }},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := cloneProfile(profile)
			inside := filepath.Join(unresolved(t, changed.Box), "protected")
			if err := os.MkdirAll(inside, 0o700); err != nil {
				t.Fatal(err)
			}
			test.set(&changed, inside)
			if _, err := NewNono(changed); err == nil || !strings.Contains(err.Error(), "protected path is inside the box") {
				t.Fatalf("NewNono error = %v, want a protected path refusal", err)
			}
		})
	}
}

func TestNonoRendersAReadableBoxAndAWritableScopeOnly(t *testing.T) {
	// Making the box legible is what lets a build find its own module. It must
	// not also make the box writable: the writable set stays exactly the scope
	// the promise bound (0047), and the machine-written tier is denied by name
	// because a read grant over the box root would otherwise reach it.
	profile := fixtureProfile(t)
	profile.Kept = filepath.Join(t.TempDir(), "kept")
	launch, err := NewNono(profile)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "profile.json")
	if err := writeNonoProfile(path, launch.profile, "/usr/bin/runtime", nil); err != nil {
		t.Fatal(err)
	}
	encoded, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var rendered nonoProfile
	if err := json.Unmarshal(encoded, &rendered); err != nil {
		t.Fatal(err)
	}

	box := launch.profile.Box
	if !slices.Contains(rendered.Filesystem.Read, box) {
		t.Fatalf("rendered readable set = %v, want the box root", rendered.Filesystem.Read)
	}
	if slices.Contains(rendered.Filesystem.Allow, box) {
		t.Fatalf("rendered writable set = %v, which is the whole box", rendered.Filesystem.Allow)
	}
	if len(rendered.Filesystem.Allow) != 1 || rendered.Filesystem.Allow[0] != launch.profile.Write[0] {
		t.Fatalf("rendered writable set = %v, want exactly the repair scope", rendered.Filesystem.Allow)
	}
	for _, denied := range []string{launch.profile.Kept, launch.profile.Ledger, launch.profile.Artifacts, filepath.Join(box, ".kept")} {
		if !slices.Contains(rendered.Filesystem.Deny, denied) {
			t.Fatalf("rendered deny set = %v, want %s", rendered.Filesystem.Deny, denied)
		}
	}
}

// unresolved reports the path as the test wrote it rather than as the operating
// system resolves it. On darwin the temporary tree is reached through a symlink,
// so the two differ and the difference is what a containment check must survive.
func unresolved(t *testing.T, path string) string {
	t.Helper()
	root := t.TempDir()
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if resolved == root || !strings.HasPrefix(path, resolved) {
		return path
	}
	return filepath.Join(root, strings.TrimPrefix(path, resolved))
}

func TestNonoCanaryProvesTheBoundaryBeforeItLaunches(t *testing.T) {
	launch := liveNono(t)
	if err := launch.Verified(); err != nil {
		t.Fatalf("real nono canary failed: %v", err)
	}

	// A passed canary leaves neither its .kept probe nor an escape behind in
	// the repair tree. The assertion becomes useful if a provider permits a
	// write but returns a misleading failure from its own diagnostics.
	if _, err := os.Stat(filepath.Join(launch.profile.Box, ".kept")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canary left a .kept probe in the box: %v", err)
	}
	if _, err := os.Stat(filepath.Join(launch.profile.Write[0], "escaped")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canary wrote inside the allowed repair scope: %v", err)
	}
}

func TestNonoLaunchFiltersCredentialsAndReapsACompleteProcessGroup(t *testing.T) {
	launch := liveNono(t)
	if err := launch.Verified(); err != nil {
		t.Fatalf("real nono canary failed: %v", err)
	}
	t.Setenv("GIT_TOKEN", "must-not-reach-the-agent")

	stream := &testStream{}
	child, err := launch.Start(context.Background(), thread.Invocation{
		Path:   "/usr/bin/env",
		Env:    []string{"RUNTIME_TOKEN=only-this-credential"},
		Output: stream,
	})
	if err != nil {
		t.Fatal(err)
	}
	exit := child.Wait()
	if exit.Code == nil || *exit.Code != 0 {
		t.Fatalf("confined env exit = %#v", exit)
	}
	if output := stream.String(); !strings.Contains(output, "RUNTIME_TOKEN=only-this-credential") || strings.Contains(output, "GIT_TOKEN=") {
		t.Fatalf("confined environment = %q; injected credential was missing or origin credential leaked", output)
	}

	pidPath := filepath.Join(launch.profile.Write[0], "child.pid")
	child, err = launch.Start(context.Background(), thread.Invocation{
		Path: "/bin/sh",
		Args: []string{"-c", "sleep 30 & child=$!; printf '%s' \"$child\" > \"$1\"; wait", "sh", pidPath},
		Env:  []string{"RUNTIME_TOKEN=only-this-credential"},
	})
	if err != nil {
		t.Fatal(err)
	}
	pid := waitForPID(t, pidPath)
	if err := child.Kill(); err != nil {
		t.Fatalf("kill confined process group: %v", err)
	}
	exit = child.Wait()
	if exit.Signal != "killed" && exit.Signal != "SIGKILL" {
		t.Fatalf("killed process group exit = %#v", exit)
	}
	if err := waitForGone(pid); err != nil {
		t.Fatal(err)
	}
}

func TestNonoRefusesAnInvocationThatWouldInjectAnUnapprovedCredential(t *testing.T) {
	launch := liveNono(t)
	_, err := launch.Start(context.Background(), thread.Invocation{
		Path: "/bin/true",
		Env:  []string{"GIT_TOKEN=push-capable"},
	})
	if err == nil || !strings.Contains(err.Error(), "not in the profile allowlist") {
		t.Fatalf("Start error = %v, want an environment refusal", err)
	}
}

func TestNonoSupervisorDeadlineKillsAndReapsTheRuntimeGroup(t *testing.T) {
	launch := liveNono(t)
	store := ledger.NewFiles(t.TempDir())
	receipt, err := thread.NewReceipt(ledger.Open(store, run.Start("test", nil)), thread.MintID())
	if err != nil {
		t.Fatal(err)
	}
	pidPath := filepath.Join(launch.profile.Write[0], "deadline-child.pid")
	err = (thread.Supervisor{Timeout: 2 * time.Second}).Run(context.Background(), launch, thread.Invocation{
		Path: "/bin/sh",
		Args: []string{"-c", "sleep 30 & child=$!; printf '%s' \"$child\" > \"$1\"; wait", "sh", pidPath},
		Env:  []string{"RUNTIME_TOKEN=only-this-credential"},
	}, receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := waitForGone(waitForPID(t, pidPath)); err != nil {
		t.Fatal(err)
	}
	entries, quarantined, err := thread.ReadReceipt(store, receipt.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(quarantined) != 0 || len(entries) != 4 {
		t.Fatalf("timeout receipt = %#v, quarantine = %#v", entries, quarantined)
	}
	if entries[0].Lifecycle.Event != thread.Started || entries[1].Lifecycle.Event != thread.TimedOut || entries[2].Lifecycle.Event != thread.Killed || entries[3].Lifecycle.Event != thread.Exited {
		t.Fatalf("timeout lifecycle = %#v", entries)
	}
}

func liveNono(t *testing.T) *Nono {
	t.Helper()
	nono, err := exec.LookPath("nono")
	if err != nil {
		t.Skip("real nono provider is not installed")
	}
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	profile := fixtureProfile(t)
	profile.Nono = absolute(t, nono)
	profile.Git = absolute(t, git)
	profile.Kept = buildKept(t)
	launch, err := NewNono(profile)
	if err != nil {
		t.Fatal(err)
	}
	return launch
}

func fixtureProfile(t *testing.T) Profile {
	t.Helper()
	root := t.TempDir()
	box := filepath.Join(root, "box")
	read := filepath.Join(box, "read")
	write := filepath.Join(box, "write")
	ledger := filepath.Join(root, "ledger")
	artifacts := filepath.Join(root, "artifacts")
	for _, path := range []string{read, write, ledger, artifacts} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return Profile{
		Nono:          "/configured/nono",
		Kept:          "/configured/kept",
		Git:           "/configured/git",
		Box:           box,
		Read:          []string{box, read},
		Write:         []string{write},
		Domains:       []string{"api.openai.com"},
		Env:           []string{"RUNTIME_TOKEN"},
		Ledger:        ledger,
		Artifacts:     artifacts,
		CanaryTimeout: 15 * time.Second,
	}
}

func buildKept(t *testing.T) string {
	t.Helper()
	workdir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repository := filepath.Clean(filepath.Join(workdir, "..", ".."))
	path := filepath.Join(t.TempDir(), "kept")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-o", path, "./cmd/kept")
	cmd.Dir = repository
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build canary kept: %v\n%s", err, output)
	}
	return path
}

func absolute(t *testing.T, path string) string {
	t.Helper()
	absolute, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	return absolute
}

func waitForPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		bytes, err := os.ReadFile(path)
		if err == nil {
			pid, parseErr := strconv.Atoi(strings.TrimSpace(string(bytes)))
			if parseErr != nil {
				t.Fatal(parseErr)
			}
			return pid
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("confined process did not write child pid at %s", path)
	return 0
}

func waitForGone(pid int) error {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		err := syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("inspect child process %d: %w", pid, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("confined child process %d survived the process-group kill", pid)
}

type testStream struct {
	strings.Builder
	mu sync.Mutex
}

func (s *testStream) Write(src []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Builder.Write(src)
}

func (s *testStream) Close() error { return nil }

func (s *testStream) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Builder.String()
}

var _ io.Writer = (*testStream)(nil)
