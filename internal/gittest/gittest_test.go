package gittest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestIsolateTakesBackTheHooksPathAParentHandedGit sets the configuration a
// supervisor hands every task it starts, shows that git really does answer
// with it, and then shows that it does not once Isolate has run. The first half
// is the break: without it the second half would pass in an environment that
// never leaked anything.
func TestIsolateTakesBackTheHooksPathAParentHandedGit(t *testing.T) {
	repository := t.TempDir()
	git(t, repository, "init", "-q")
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	elsewhere := filepath.Join(parent, "supervisor-hooks")

	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "core.hooksPath")
	t.Setenv("GIT_CONFIG_VALUE_0", elsewhere)
	t.Setenv("GIT_CONFIG_PARAMETERS", "'core.hookspath'='"+elsewhere+"'")
	if got := git(t, repository, "rev-parse", "--git-path", "hooks"); got != elsewhere {
		t.Fatalf("with the configuration set, git puts hooks at %q, want %q: the leak this guards against is not reproduced", got, elsewhere)
	}

	Isolate()
	for _, name := range []string{"GIT_CONFIG_COUNT", "GIT_CONFIG_KEY_0", "GIT_CONFIG_VALUE_0", "GIT_CONFIG_PARAMETERS"} {
		if value, set := os.LookupEnv(name); set {
			t.Errorf("%s is still set to %q", name, value)
		}
	}
	if got := git(t, repository, "rev-parse", "--git-path", "hooks"); got == elsewhere || !strings.HasSuffix(got, filepath.Join(".git", "hooks")) {
		t.Fatalf("after Isolate, git puts hooks at %q, want the repository's own .git/hooks", got)
	}
}

func TestOnlyConfigurationHandedOnTheSideIsInjected(t *testing.T) {
	for name, want := range map[string]bool{
		"GIT_CONFIG_COUNT":      true,
		"GIT_CONFIG_KEY_0":      true,
		"GIT_CONFIG_VALUE_12":   true,
		"GIT_CONFIG_PARAMETERS": true,
		// A file is configuration a test can see and name, and several tests
		// point these at /dev/null on purpose.
		"GIT_CONFIG_GLOBAL":   false,
		"GIT_CONFIG_SYSTEM":   false,
		"GIT_CONFIG_NOSYSTEM": false,
		"GIT_CONFIG":          false,
		"GIT_DIR":             false,
	} {
		if got := Injected(name); got != want {
			t.Errorf("Injected(%q) = %v, want %v", name, got, want)
		}
	}
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
	got := strings.TrimSpace(string(out))
	if !filepath.IsAbs(got) {
		got = filepath.Join(dir, got)
	}
	if resolved, err := filepath.EvalSymlinks(filepath.Dir(got)); err == nil {
		got = filepath.Join(resolved, filepath.Base(got))
	}
	return got
}
