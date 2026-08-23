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
		{"no network policy", func(p *Profile) { p.Domains = nil }, "neither blocks the network nor names a domain allowlist"},
		{"blocked network with an allowlist", func(p *Profile) { p.BlockNetwork = true }, "blocks the network and names a domain allowlist"},
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

func TestNonoRefusesABoxInsideTheHostTemporaryTree(t *testing.T) {
	// The profile takes the host's scratch trees back from the provider's
	// baseline, and denial beats a grant. A box cut inside one is therefore a
	// box its own confined child cannot read, write, or resolve a working
	// directory in — while every escape probe is refused and the canary passes.
	// Measured: Verified() returned nil for such a box. A boundary that cannot
	// fail is worse than none, so the profile is refused before a process
	// exists.
	profile := fixtureProfileUnder(t, t.TempDir())
	if _, err := NewNono(profile); err == nil || !strings.Contains(err.Error(), "host temporary tree") {
		t.Fatalf("NewNono error = %v, want a refusal naming the host temporary tree", err)
	}
}

func TestNonoExpressesABoxBeneathTheHiddenTier(t *testing.T) {
	// internal/box puts every box at <repository>/.kept/boxes/<id>, so every
	// scope path in an ordinary box carries a .kept component. Refusing on that
	// component refuses the whole design; what must be refused is the tier
	// relative to the box.
	repository := ungrantedDir(t)
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
	// written through a symlinked prefix, which is the form that relates as
	// outside a box it is inside.
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
	if err := writeNonoProfile(path, launch.profile, "/usr/bin/runtime"); err != nil {
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
	// The writable set is the repair scope plus the box's own temporary
	// directory, and nothing else. The temporary directory is there because
	// the profile denies the host's scratch trees: a confined child with no
	// writable temporary directory at all is not a narrower boundary, it is a
	// toolchain that fails wherever its own scratch file happened to be used.
	if want := []string{launch.profile.Write[0], launch.profile.Temp}; !slices.Equal(rendered.Filesystem.Allow, want) {
		t.Fatalf("rendered writable set = %v, want exactly %v", rendered.Filesystem.Allow, want)
	}
	if rendered.Environment.Set["TMPDIR"] != launch.profile.Temp {
		t.Fatalf("rendered TMPDIR = %q, want the box temporary directory it also grants at %s", rendered.Environment.Set["TMPDIR"], launch.profile.Temp)
	}
	// kept names no provider policy group. The list is rendered rather than
	// omitted, because omitting it says nothing while an empty list says kept
	// decided. See docs/decision/0053.
	if rendered.Groups.Include == nil || len(rendered.Groups.Include) != 0 {
		t.Fatalf("rendered groups = %#v, want an empty list", rendered.Groups.Include)
	}
	for _, denied := range []string{launch.profile.Kept, launch.profile.Ledger, launch.profile.Artifacts, filepath.Join(box, ".kept")} {
		if !slices.Contains(rendered.Filesystem.Deny, denied) {
			t.Fatalf("rendered deny set = %v, want %s", rendered.Filesystem.Deny, denied)
		}
	}
	// The host's scratch trees are the grant nobody asked for: the provider
	// baseline hands them out recursively, so a profile that does not take them
	// back leaves a repair able to write outside its box however narrow the rest
	// of the policy is.
	for _, root := range hostTemporaryRoots() {
		if !slices.Contains(rendered.Filesystem.Deny, root) {
			t.Fatalf("rendered deny set = %v, want the host temporary root %s", rendered.Filesystem.Deny, root)
		}
	}
	if !slices.Contains(hostTemporaryRoots(), filepath.Clean(os.TempDir())) {
		t.Fatalf("host temporary roots = %v, want this process's own temporary root %s", hostTemporaryRoots(), os.TempDir())
	}
}

// unresolved names path through a symlinked prefix, so the path as written and
// the path as the operating system resolves it differ. That difference is what
// a containment check has to survive. The symlink is made here rather than
// borrowed from the platform: darwin reaches its temporary tree through one and
// linux does not, so a test that borrowed the platform's checked nothing on
// half the machines it ran on.
func unresolved(t *testing.T, path string) string {
	t.Helper()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(filepath.Dir(path), link); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(link, filepath.Base(path))
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

// TestNonoCanaryProbesFailWhenTheBoundaryIsBroken breaks, one at a time, the
// property each probe defends, and requires that probe to say so.
//
// A check that has never fired is not a check, and an escape probe is the case
// where that costs the most: it is read as evidence. The probes here were each
// unable to fail for the right reason. write_outside_box aimed at a directory
// kept created and then denied by name, so it demonstrated that a denied path
// is denied — measured, a path declared read-only was written from inside the
// box and the suite stayed green. kept_executable read any error from exec as a
// refusal, so a configured path naming nothing passed with the guard absent.
//
// Every other probe in each run must stay refused, which is what says the
// weakening reached the property it was aimed at and nothing else.
func TestNonoCanaryProbesFailWhenTheBoundaryIsBroken(t *testing.T) {
	launch := liveNono(t)
	for _, test := range []struct {
		name    string
		probe   string
		outcome canaryOutcome
		weaken  func(*nonoProfile, *canaryState)
		aim     func(*canaryState, []string) []string
	}{
		{
			name:    "the location outside the box is granted",
			probe:   "write_outside_box",
			outcome: probeReached,
			weaken: func(rendered *nonoProfile, state *canaryState) {
				rendered.Filesystem.Allow = append(rendered.Filesystem.Allow, state.outside)
			},
		},
		{
			name:    "the host temporary tree is left as the baseline grants it",
			probe:   "write_host_temp",
			outcome: probeReached,
			weaken: func(rendered *nonoProfile, _ *canaryState) {
				rendered.Filesystem.Deny = withoutHostTemporaryRoots(rendered.Filesystem.Deny)
			},
		},
		{
			name:    "the kept executable is reachable",
			probe:   "kept_executable",
			outcome: probeReached,
			weaken: func(rendered *nonoProfile, state *canaryState) {
				rendered.Filesystem.Read = append(rendered.Filesystem.Read, filepath.Dir(state.profileKept()))
				rendered.Filesystem.Deny = without(rendered.Filesystem.Deny, state.profileKept())
				commands, _ := rendered.CommandPolicies["commands"].(map[string]any)
				delete(commands, filepath.Base(state.profileKept()))
			},
		},
		{
			// The distinction fix: a configured path that names nothing is not
			// a boundary that held, and the canary must not read it as one.
			name:    "the kept executable is not where the profile says",
			probe:   "kept_executable",
			outcome: probeAbsent,
			aim: func(state *canaryState, values []string) []string {
				return withCanaryValue(values, canaryKept, filepath.Join(state.dir, "kept-that-is-not-there"))
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			report := measureCanary(t, launch, test.weaken, test.aim)
			found := false
			for _, probe := range report.Probes {
				if probe.Name == test.probe {
					found = true
					if probe.Outcome != test.outcome {
						t.Fatalf("%s outcome = %q (%s), want %q: the probe did not notice the break", probe.Name, probe.Outcome, probe.Detail, test.outcome)
					}
					continue
				}
				if probe.Outcome != probeRefused {
					t.Fatalf("%s outcome = %q (%s) while %s was the one broken", probe.Name, probe.Outcome, probe.Detail, test.probe)
				}
			}
			if !found {
				t.Fatalf("canary report does not contain %s: %#v", test.probe, report.Probes)
			}
			if err := report.valid(); err == nil || !strings.Contains(err.Error(), test.probe) {
				t.Fatalf("report.valid() = %v, want a refusal naming %s", err, test.probe)
			}
		})
	}
}

// measureCanary runs the confined helper against a deliberately weakened policy
// and returns what it measured. It renders through writeNonoProfile and then
// edits the rendered profile, because Profile itself refuses every weakening
// worth testing — which is the point of validating there, and the reason a test
// that breaks the boundary has to reach past it.
func measureCanary(t *testing.T, launch *Nono, weaken func(*nonoProfile, *canaryState), aim func(*canaryState, []string) []string) canaryReport {
	t.Helper()
	state, err := makeCanaryState(launch.profile)
	if err != nil {
		t.Fatal(err)
	}
	defer state.close()
	copyPath, release, err := sealedCanary(launch.profile.Kept)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	profile := cloneProfile(launch.profile)
	profile.Env = append([]string(nil), canaryEnvironment...)
	path := filepath.Join(t.TempDir(), "profile.json")
	if err := writeNonoProfile(path, profile, copyPath); err != nil {
		t.Fatal(err)
	}
	if weaken != nil {
		encoded, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var rendered nonoProfile
		if err := json.Unmarshal(encoded, &rendered); err != nil {
			t.Fatal(err)
		}
		weaken(&rendered, state)
		if encoded, err = json.Marshal(rendered); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, encoded, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	values := state.environment()
	if aim != nil {
		values = aim(state, values)
	}
	ctx, cancel := context.WithTimeout(context.Background(), launch.profile.CanaryTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, launch.profile.Nono,
		"run", "--silent", "--no-audit", "--no-rollback", "--profile", path, "--", copyPath, canaryArgument)
	cmd.Dir = launch.profile.Box
	cmd.Env = childEnvironment(launch.profile, values)
	output, diagnostic := &strings.Builder{}, &strings.Builder{}
	cmd.Stdout, cmd.Stderr = output, diagnostic
	_ = cmd.Run()
	report, err := parseCanaryReport([]byte(output.String()))
	if err != nil {
		t.Fatalf("weakened canary did not report: %v: %s", err, diagnostic.String())
	}
	return report
}

func withCanaryValue(values []string, name, value string) []string {
	replaced := append([]string(nil), values...)
	for i, entry := range replaced {
		if strings.HasPrefix(entry, name+"=") {
			replaced[i] = name + "=" + value
			return replaced
		}
	}
	return append(replaced, name+"="+value)
}

func withoutHostTemporaryRoots(paths []string) []string {
	kept := paths
	for _, root := range hostTemporaryRoots() {
		kept = without(kept, root)
	}
	return kept
}

func without(paths []string, unwanted string) []string {
	kept := make([]string, 0, len(paths))
	for _, path := range paths {
		if path != unwanted {
			kept = append(kept, path)
		}
	}
	return kept
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

// fixtureProfile cuts its box where the provider's baseline grants nothing.
// Not t.TempDir(): the baseline grants the operating system's temporary trees
// write access recursively and the profile takes them back, so a box there is
// unconfinable — and NewNono refuses it. See ungrantedRoot.
func fixtureProfile(t *testing.T) Profile {
	t.Helper()
	return fixtureProfileUnder(t, ungrantedDir(t))
}

func fixtureProfileUnder(t *testing.T, root string) Profile {
	t.Helper()
	box := filepath.Join(root, "box")
	read := filepath.Join(box, "read")
	write := filepath.Join(box, "write")
	ledger := filepath.Join(root, "ledger")
	artifacts := filepath.Join(root, "artifacts")
	temp := filepath.Join(root, "box.tmp")
	for _, path := range []string{read, write, ledger, artifacts, temp} {
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
		Temp:          temp,
		ToolPath:      []string{"/usr/bin", "/bin"},
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
	// The kept_executable probe asks whether the profile's own guard refuses
	// this binary. Built into the temporary tree it would be refused by the
	// host-temporary denial instead, and the probe would pass while saying
	// nothing about the guard it exists for.
	path := filepath.Join(ungrantedDir(t), "kept")
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
