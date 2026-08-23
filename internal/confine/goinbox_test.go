//go:build !windows

package confine

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/box"
	"github.com/pbleisch/kept/internal/thread"
)

// A confined Go repair either works or it does not, and the only way to know is
// to run one. These tests build, test and vet a real module inside a real box
// through kept's own launcher, under the profile a repair runs under: no
// provider policy group, no network, the toolchain located by asking it, and a
// temporary directory that belongs to the box.

func TestConfinedGoToolchainBuildsTestsAndVetsInsideItsBox(t *testing.T) {
	launch, created := liveGoBox(t)
	if err := launch.Verified(); err != nil {
		t.Fatalf("provider verification failed for a Go repair box: %v", err)
	}

	built := filepath.Join(created.Path(), "internal", "scratch.bin")
	for _, work := range []struct {
		name string
		args []string
	}{
		{"build", []string{"build", "-o", built, "./internal/"}},
		{"test", []string{"test", "-count=1", "./internal/"}},
		{"vet", []string{"vet", "./..."}},
	} {
		t.Run(work.name, func(t *testing.T) {
			stream := &testStream{}
			child, err := launch.Start(context.Background(), thread.Invocation{
				Path:   launch.profile.ToolPath[0] + string(filepath.Separator) + "go",
				Args:   work.args,
				Env:    []string{"RUNTIME_TOKEN=only-this-credential"},
				Output: stream,
			})
			if err != nil {
				t.Fatal(err)
			}
			if exit := child.Wait(); exit.Code == nil || *exit.Code != 0 {
				diagnostic := ""
				if reporter, ok := child.(interface{ diagnostic() string }); ok {
					diagnostic = reporter.diagnostic()
				}
				t.Fatalf("confined go %s exit = %#v\n%s\n%s", work.name, exit, stream.String(), diagnostic)
			}
		})
	}
	if _, err := os.Stat(built); err != nil {
		t.Fatalf("the confined build produced nothing at %s: %v", built, err)
	}
}

// A build-only box reaches nothing. An empty domain allowlist is the provider's
// spelling of unrestricted, so this is measured from the provider rather than
// assumed from the absence of a list.
func TestConfinedGoBoxReachesNoHost(t *testing.T) {
	launch, _ := liveGoBox(t)
	rendered := renderedProfile(t, launch, "/usr/bin/runtime")
	if !rendered.Network.Block {
		t.Fatalf("rendered network = %#v, want the network blocked", rendered.Network)
	}
	if len(rendered.Network.AllowDomain) != 0 {
		t.Fatalf("rendered domain allowlist = %v beside a blocked network", rendered.Network.AllowDomain)
	}
	path := writeRendered(t, rendered)
	for _, host := range []string{"proxy.golang.org", "api.openai.com", "github.com"} {
		answer := askHost(t, launch.profile.Nono, path, host)
		if answer.Status == providerAllowed {
			t.Fatalf("the provider reaches %s from a build-only box: %#v", host, answer)
		}
	}
	// The canary's own host probe is what proves this from inside the box
	// rather than from the profile.
	if err := launch.Verified(); err != nil {
		t.Fatalf("provider verification failed for a build-only box: %v", err)
	}

	// And once from inside the box against a host that is really there. The
	// premise is checked first, because a refusal measured while this machine
	// has no network at all would prove nothing.
	const reachable = "example.com:443"
	probe, err := net.DialTimeout("tcp", reachable, 10*time.Second)
	if err != nil {
		t.Skipf("this machine cannot reach %s unconfined, so a refusal from inside the box would measure nothing", reachable)
	}
	_ = probe.Close()

	stream := &testStream{}
	child, err := launch.Start(context.Background(), thread.Invocation{
		Path:   "/usr/bin/curl",
		Args:   []string{"-s", "-o", "/dev/null", "-w", "%{http_code}", "--max-time", "12", "https://example.com/"},
		Env:    []string{"RUNTIME_TOKEN=only-this-credential"},
		Output: stream,
	})
	if err != nil {
		t.Fatal(err)
	}
	child.Wait()
	if code := strings.TrimSpace(stream.String()); code != "000" {
		t.Fatalf("a build-only box reached https://example.com/ and got %q", code)
	}
}

// TestConfinedChildReceivesExactlyTheDeclaredSearchPath is the PATH leak, made
// into a test.
//
// The provider derives the child's search path from the PATH of its own
// process and ignores the profile's variable allowlist for it, so kept merging
// os.Environ() into that process handed every repair the operator's entire
// search path while the rendered policy said kept granted none of it.
func TestConfinedChildReceivesExactlyTheDeclaredSearchPath(t *testing.T) {
	launch, _ := liveGoBox(t)
	// A directory that is on the operator's search path and on no policy kept
	// wrote. If the child sees it, the provider's environment was inherited.
	leak := filepath.Join(t.TempDir(), "operator-only")
	if err := os.Mkdir(leak, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", os.Getenv("PATH")+string(os.PathListSeparator)+leak)

	stream := &testStream{}
	child, err := launch.Start(context.Background(), thread.Invocation{
		Path:   "/usr/bin/env",
		Env:    []string{"RUNTIME_TOKEN=only-this-credential"},
		Output: stream,
	})
	if err != nil {
		t.Fatal(err)
	}
	if exit := child.Wait(); exit.Code == nil || *exit.Code != 0 {
		t.Fatalf("confined env exit = %#v: %s", exit, stream.String())
	}

	value := ""
	for _, line := range strings.Split(stream.String(), "\n") {
		if rest, found := strings.CutPrefix(line, "PATH="); found {
			value = rest
		}
	}
	if value == "" {
		t.Fatalf("the confined child reported no search path:\n%s", stream.String())
	}
	entries := strings.Split(value, string(os.PathListSeparator))
	declared := launch.profile.ToolPath
	if len(entries) < len(declared) {
		t.Fatalf("confined search path = %v, which is shorter than the %v kept declared", entries, declared)
	}
	// The provider prepends its own shim directory and kept cannot stop it.
	// What kept decides is everything after that, and it must be exactly the
	// declared list — same entries, same order, nothing extra.
	if tail := entries[len(entries)-len(declared):]; !slices.Equal(tail, declared) {
		t.Fatalf("confined search path = %v, want it to end in exactly the declared %v", entries, declared)
	}
	for _, entry := range entries[:len(entries)-len(declared)] {
		// The provider puts its shim and its tool-sandbox directories ahead of
		// everything; both are named after itself, and nothing else may be
		// there.
		if !strings.Contains(entry, "nono-") {
			t.Fatalf("confined search path carries %s, which kept neither declared nor recognises as the provider's own shim", entry)
		}
	}
	if strings.Contains(value, leak) {
		t.Fatalf("the operator's search path reached the box: %s is in %s", leak, value)
	}
}

// TestPreflightRefusesAToolchainTheProviderCannotReach is the refusal that
// makes resolution safe, measured against a real provider.
//
// A toolchain reached through a shim resolves correctly — the shim execs a real
// toolchain, which knows where it lives — while the shim's own directory is
// granted by nothing. Without the pre-flight that is a dispatch that starts and
// then reports `env: go: No such file or directory` from inside a box, which is
// indistinguishable from a repair that ran and did nothing.
func TestPreflightRefusesAToolchainTheProviderCannotReach(t *testing.T) {
	nono := liveProvider(t)
	real := realGo(t)
	root := ungrantedDir(t)
	shim := filepath.Join(root, "shim")
	if err := os.Mkdir(shim, 0o700); err != nil {
		t.Fatal(err)
	}
	shimmed := filepath.Join(shim, "go")
	if err := os.WriteFile(shimmed, []byte("#!/bin/sh\nexec "+real+" \"$@\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	// The premise: the shim's directory is granted by nothing, so a refusal
	// aimed at it is the boundary rather than a tautology about a path kept
	// itself denied.
	repository, created := scratchBox(t, root)
	toolchain, err := ResolveGo(context.Background(), shimmed, 30*time.Second)
	if err != nil {
		t.Fatalf("resolve a shimmed go toolchain: %v", err)
	}
	if !slices.Equal(toolchain.Path, []string{shim}) {
		t.Fatalf("resolved search path = %v, want the shim directory %s", toolchain.Path, shim)
	}

	profile := goProfile(t, repository, created, nono, toolchain)
	launch, err := NewNono(profile)
	if err != nil {
		t.Fatal(err)
	}
	err = launch.Verified()
	if err == nil {
		t.Fatal("the launch was verified with a toolchain the provider cannot reach")
	}
	for _, want := range []string{"pre-flight refused", shim, "read", "search path"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("refusal = %v, want it to name %q", err, want)
		}
	}

	// And the other branch: grant the shim's directory and the same launch is
	// verified. Without this, the refusal above could be anything.
	granted := cloneProfile(profile)
	granted.ToolRead = append(granted.ToolRead, shim)
	reachable, err := NewNono(granted)
	if err != nil {
		t.Fatal(err)
	}
	if err := reachable.Verified(); err != nil {
		t.Fatalf("provider verification failed once the shim directory was granted: %v", err)
	}
}

// TestPreflightRefusesARenderingThatDropsADeclaredGrant is the guard fired on
// purpose. kept renders the profile it then asks about, so the only way to see
// the pre-flight refuse a path kept declared is to take the grant out of the
// rendering after it was made — which is exactly the class of failure it exists
// for: a renderer that silently did not say what kept meant.
func TestPreflightRefusesARenderingThatDropsADeclaredGrant(t *testing.T) {
	launch, _ := liveGoBox(t)
	rendered := renderedProfile(t, launch, "/usr/bin/runtime")
	// The build cache, deliberately: a Homebrew toolchain root is reachable
	// through the provider's own baseline whether kept grants it or not, so
	// dropping that grant would measure nothing. user_caches_macos is not in
	// the baseline, so nothing but kept's own rendering grants this one.
	dropped := launch.profile.ToolWrite[0]
	rendered.Filesystem.Allow = without(rendered.Filesystem.Allow, dropped)

	err := launch.preflight(context.Background(), writeRendered(t, rendered))
	if err == nil {
		t.Fatalf("the pre-flight passed a rendering with no grant for %s", dropped)
	}
	for _, want := range []string{"pre-flight refused", dropped, "resolved toolchain cache"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("refusal = %v, want it to name %q", err, want)
		}
	}
	// The unweakened rendering is what says the refusal was about the drop.
	if err := launch.preflight(context.Background(), writeRendered(t, renderedProfile(t, launch, "/usr/bin/runtime"))); err != nil {
		t.Fatalf("the pre-flight refused the rendering kept actually makes: %v", err)
	}
}

func TestNonoRefusesAToolPolicyItCannotEnforce(t *testing.T) {
	profile := fixtureProfile(t)
	outside := ungrantedDir(t)
	for _, test := range []struct {
		name string
		edit func(*Profile)
		want string
	}{
		{"no search path", func(p *Profile) { p.ToolPath = nil }, "declares no search path"},
		{"a search path entry that is a name", func(p *Profile) { p.ToolPath = []string{"usr/bin"} }, "search path entry is not an explicit absolute path"},
		{"no temporary directory", func(p *Profile) { p.Temp = "" }, "box temporary directory is not an explicit absolute path"},
		{"a temporary directory that is not there", func(p *Profile) { p.Temp = filepath.Join(outside, "missing") }, "box temporary directory is not there"},
		{"a temporary directory in the host temporary tree", func(p *Profile) { p.Temp = t.TempDir() }, "inside the host temporary tree"},
		{"a temporary directory inside the box", func(p *Profile) { p.Temp = p.Box }, "relates to the box"},
		{"a tool grant that contains the box", func(p *Profile) { p.ToolRead = []string{filepath.Dir(p.Box)} }, "relates to the box"},
		{"a tool grant on the ledger", func(p *Profile) { p.ToolRead = []string{p.Ledger} }, "reaches a protected location"},
		{"a tool grant on the filesystem root", func(p *Profile) { p.ToolRead = []string{string(filepath.Separator)} }, "the filesystem root"},
		{"a tool cache in the host temporary tree", func(p *Profile) { p.ToolWrite = []string{t.TempDir()} }, "inside the host temporary tree"},
		{"one path granted both ways", func(p *Profile) {
			p.ToolRead = []string{outside}
			p.ToolWrite = []string{outside}
		}, "both readable and writable"},
		{"a tool variable the provider reserves", func(p *Profile) { p.ToolEnv = map[string]string{"PATH": "/usr/bin"} }, "which the provider reserves"},
		{"a tool variable that decides the temporary directory twice", func(p *Profile) { p.ToolEnv = map[string]string{"TMPDIR": outside} }, "which Temp already decides"},
		{"a tool variable that was never resolved", func(p *Profile) { p.ToolEnv = map[string]string{"GOMODCACHE": "$GOPATH/pkg/mod"} }, "was not resolved"},
		{"a tool variable that can reach origin", func(p *Profile) { p.ToolEnv = map[string]string{"GIT_ASKPASS": "/bin/false"} }, "can reach origin"},
		{"a child environment allowing PATH", func(p *Profile) { p.Env = []string{"PATH"} }, "which the provider does not filter"},
		{"a child environment allowing HOME", func(p *Profile) { p.Env = []string{"HOME"} }, "the provider's own resolution base"},
		{"a tool variable that decides the child home twice", func(p *Profile) { p.ToolEnv = map[string]string{"HOME": outside} }, "which Home already decides"},
		{"a child home that is a name", func(p *Profile) { p.Home = "state" }, "child home directory is not an explicit absolute path"},
		{"a child home that is not there", func(p *Profile) { p.Home = filepath.Join(outside, "missing") }, "child home directory is not there"},
		{"a child home that is a file", func(p *Profile) { p.Home = writtenFile(t, outside, "home") }, "child home directory is not a directory"},
		{"a child home inside the box", func(p *Profile) { p.Home = p.Box }, "relates to the box"},
		{"a child home in the host temporary tree", func(p *Profile) { p.Home = t.TempDir() }, "inside the host temporary tree"},
		{"a child home on the ledger", func(p *Profile) { p.Home = p.Ledger }, "reaches a protected location"},
		{"a loopback port that is not one", func(p *Profile) { p.LoopbackPorts = []int{0} }, "is not a TCP port"},
		{"a loopback port past the range", func(p *Profile) { p.LoopbackPorts = []int{70000} }, "is not a TCP port"},
		{"the same loopback port twice", func(p *Profile) { p.LoopbackPorts = []int{1234, 1234} }, "loopback port 1234 twice"},
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

// liveGoBox is a real box, cut where the provider grants nothing, with a real
// Go toolchain resolved into its profile.
func liveGoBox(t *testing.T) (*Nono, *box.Box) {
	t.Helper()
	nono := liveProvider(t)
	toolchain := resolveRealGo(t)
	repository, created := scratchBox(t, ungrantedDir(t))
	launch, err := NewNono(goProfile(t, repository, created, nono, toolchain))
	if err != nil {
		t.Fatal(err)
	}
	return launch, created
}

func liveProvider(t *testing.T) string {
	t.Helper()
	nono, err := exec.LookPath("nono")
	if err != nil {
		t.Skip("real nono provider is not installed")
	}
	return absolute(t, nono)
}

// goProfile is the profile a Go repair runs under: the box readable, the
// promise's scope writable, the resolved toolchain and nothing else, no
// network, and a temporary directory the box owns.
func goProfile(t *testing.T, repository string, created *box.Box, nono string, toolchain Toolchain) Profile {
	t.Helper()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	profile := boxProfile(t, repository, created, nono, buildKept(t), absolute(t, git))
	profile.Domains = nil
	profile.BlockNetwork = true
	profile.ToolPath = append(append([]string(nil), toolchain.Path...), "/usr/bin", "/bin")
	profile.ToolRead = toolchain.Read
	profile.ToolWrite = toolchain.Write
	profile.ToolEnv = toolchain.Env
	return profile
}

func renderedProfile(t *testing.T, launch *Nono, entrypoint string) nonoProfile {
	t.Helper()
	rendered, err := renderNonoProfile(launch.profile, entrypoint)
	if err != nil {
		t.Fatal(err)
	}
	return rendered
}

func writeRendered(t *testing.T, rendered nonoProfile) string {
	t.Helper()
	encoded, err := json.Marshal(rendered)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "profile.json")
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func askHost(t *testing.T, nono, profilePath, host string) providerAnswer {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, nono, "why", "--json", "--profile", profilePath, "--host", host).Output()
	if err != nil {
		t.Fatalf("ask the provider about %s: %v: %s", host, err, providerDiagnostic(err))
	}
	var answer providerAnswer
	if err := json.Unmarshal(output, &answer); err != nil {
		t.Fatalf("read the provider's answer about %s: %v: %s", host, err, output)
	}
	return answer
}

// writtenFile is a regular file where a test needs a path that exists and is
// not a directory.
func writtenFile(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
