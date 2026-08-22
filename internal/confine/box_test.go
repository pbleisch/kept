//go:build !windows

package confine

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/box"
	"github.com/pbleisch/kept/internal/thread"
)

// The two packages were each tested alone and each passed alone, and the
// composition they exist for did not work: a box lives at
// <repository>/.kept/boxes/<id>, and confinement refused every path carrying a
// .kept component. These tests are that composition, so the gap which hid the
// refusal is itself covered.

func TestConfinementExpressesABoxTheBoxPackageCut(t *testing.T) {
	repository, created := scratchBox(t, ungrantedDir(t))

	launch, err := NewNono(boxProfile(t, repository, created, "/configured/nono", "/configured/kept", "/configured/git"))
	if err != nil {
		t.Fatalf("NewNono refused a box internal/box created: %v", err)
	}
	if launch.profile.Box != created.Path() {
		t.Fatalf("confined box = %s, want the box the box package cut at %s", launch.profile.Box, created.Path())
	}
	if !strings.Contains(created.Path(), string(filepath.Separator)+hiddenTier+string(filepath.Separator)) {
		t.Fatalf("box path %s no longer sits under the hidden tier, so this composition proves nothing", created.Path())
	}
}

func TestConfinedRepairReadsItsBoxAndWritesOnlyItsScope(t *testing.T) {
	nono, err := exec.LookPath("nono")
	if err != nil {
		t.Skip("real nono provider is not installed")
	}
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	root := ungrantedDir(t)
	repository, created := scratchBox(t, root)
	launch, err := NewNono(boxProfile(t, repository, created, absolute(t, nono), buildKept(t), absolute(t, git)))
	if err != nil {
		t.Fatal(err)
	}
	if err := launch.Verified(); err != nil {
		t.Fatalf("real nono canary failed in a box the box package cut: %v", err)
	}

	stream := &testStream{}
	child, err := launch.Start(context.Background(), thread.Invocation{
		Path: "/bin/sh",
		// Each line reports one measured capability. A build that cannot
		// resolve its own working directory reports a module it cannot find,
		// which is why the working directory and the module file are probed
		// apart from the writes.
		Args: []string{"-c", `
printf 'working-directory=%s\n' "$(pwd)"
ls . >/dev/null 2>&1 && echo list=reached || echo list=refused
cat go.mod >/dev/null 2>&1 && echo module=reached || echo module=refused
cat .kept/config.yaml >/dev/null 2>&1 && echo tier=reached || echo tier=refused
{ echo repaired > internal/repaired; } 2>/dev/null && echo scope=written || echo scope=refused
{ echo escaped > docs/escaped; } 2>/dev/null && echo outside-scope=written || echo outside-scope=refused
{ echo escaped > escaped; } 2>/dev/null && echo box-root=written || echo box-root=refused
`},
		Env:    []string{"RUNTIME_TOKEN=only-this-credential"},
		Output: stream,
	})
	if err != nil {
		t.Fatal(err)
	}
	if exit := child.Wait(); exit.Code == nil || *exit.Code != 0 {
		t.Fatalf("confined probe exit = %#v: %s", exit, stream.String())
	}

	report := stream.String()
	for _, want := range []string{
		// The box root is the confined working directory, and it is legible.
		// This is what lets a build find the module it stands in.
		"working-directory=" + created.Path(),
		"list=reached",
		"module=reached",
		// kept's own state stays out of reach even though the tree around it
		// is readable.
		"tier=refused",
		"scope=written",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("confined probe report is missing %q:\n%s", want, report)
		}
	}
	// Legible is not writable. The writable set is exactly the scope the
	// promise bound, and neither the rest of the tree nor the box root is in
	// it. See docs/decision/0047-a-promise-binds-what-a-repair-may-touch.md.
	for _, want := range []string{"outside-scope=refused", "box-root=refused"} {
		if !strings.Contains(report, want) {
			t.Fatalf("confined child wrote where the profile grants nothing, so %q is missing. Either confinement is not holding, or this host's provider baseline grants %s write access recursively and no boundary can be measured under it:\n%s", want, root, report)
		}
	}

	if _, err := os.Stat(filepath.Join(created.Path(), "internal", "repaired")); err != nil {
		t.Fatalf("in-scope repair did not reach the box: %v", err)
	}
	for _, escape := range []string{"escaped", filepath.Join("docs", "escaped")} {
		if _, err := os.Stat(filepath.Join(created.Path(), escape)); err == nil {
			t.Fatalf("confined child wrote outside its scope at %s", escape)
		}
	}
}

func boxProfile(t *testing.T, repository string, created *box.Box, nono, kept, git string) Profile {
	t.Helper()
	return Profile{
		Nono:          nono,
		Kept:          kept,
		Git:           git,
		Box:           created.Path(),
		Read:          []string{created.Path()},
		Write:         []string{filepath.Join(created.Path(), "internal")},
		Domains:       []string{"api.openai.com"},
		Env:           []string{"RUNTIME_TOKEN"},
		Ledger:        filepath.Join(repository, ".kept", "ledger"),
		Artifacts:     filepath.Join(repository, ".kept", "artifacts"),
		CanaryTimeout: 60 * time.Second,
	}
}

// scratchBox cuts a real box from a real repository under parent, the way a
// repair does.
func scratchBox(t *testing.T, parent string) (string, *box.Box) {
	t.Helper()
	repository, commit := scratchRepository(t, parent)
	created, err := box.Create(context.Background(), box.Request{Repository: repository, Commit: commit})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := created.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	if boxes := filepath.Join(repository, ".kept", "boxes"); filepath.Dir(created.Path()) != boxes {
		t.Fatalf("box at %s did not come from the scratch repository under %s", created.Path(), boxes)
	}
	return repository, created
}

func scratchRepository(t *testing.T, parent string) (string, string) {
	t.Helper()
	root, err := os.MkdirTemp(parent, "repository-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	scratchGit(t, root, "init", "-q")
	scratchGit(t, root, "config", "user.email", "test@example.com")
	scratchGit(t, root, "config", "user.name", "test")
	for path, data := range map[string]string{
		"go.mod":              "module scratch\n\ngo 1.26\n",
		"internal/scratch.go": "package scratch\n",
		"docs/note.md":        "a path outside the repair scope\n",
		".kept/config.yaml":   "corpus: docs\n",
	} {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	scratchGit(t, root, "add", "--all")
	scratchGit(t, root, "commit", "-qm", "origin")
	return root, strings.TrimSpace(scratchGit(t, root, "rev-parse", "HEAD"))
}

// ungrantedRoot is where a test that measures the confinement boundary puts its
// repository. Neither the operating system's temporary tree nor the checkout
// can be used for it: nono's baseline grants the temporary tree write access
// recursively, and a checkout may itself be sitting in one — so a boundary
// asserted about a box there passes with no boundary present. A repository
// under the operator's home is where a real one lives, and nothing is granted
// under it.
func ungrantedRoot(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("locate a directory the provider grants nothing under: %v", err)
	}
	root := filepath.Join(home, ".cache", "kept-confine-test")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	// Removed only when this run left it empty, so concurrent packages do not
	// pull the directory out from under each other.
	t.Cleanup(func() { _ = os.Remove(root) })
	return root
}

// ungrantedDir is one test's own directory under that root, resolved, because a
// profile relates every path to a canonical box.
func ungrantedDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp(ungrantedRoot(t), "fixture-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func scratchGit(t *testing.T, directory string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", directory}, args...)...)
	// The operator's own configuration must not decide whether this repository
	// can be committed to: an includeIf, a hooks path or a signing requirement
	// would make the test depend on the machine it runs on.
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return string(output)
}
