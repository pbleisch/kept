//go:build !windows

package confine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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
		Read:          []string{read},
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
