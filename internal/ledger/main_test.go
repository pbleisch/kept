package ledger

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/gittest"
)

// inheritGitConfig is set by the one test below that needs to see this suite
// run the way it ran before it isolated itself, and by nothing else.
const inheritGitConfig = "KEPT_LEDGER_TEST_INHERITS_GIT_CONFIG"

// TestMain runs this suite against the git configuration its tests write and
// none a parent process handed down. Several tests here ask git where hooks
// live and what the remote's refspecs are, and both answers move under a
// core.hooksPath or a refspec set through the environment. See
// internal/gittest.
func TestMain(m *testing.M) {
	if os.Getenv(inheritGitConfig) == "" {
		gittest.Isolate()
	}
	os.Exit(m.Run())
}

// TestAHookTestPassesUnderASupervisorThatSetsCoreHooksPath re-runs one hook
// test as a child that inherits the configuration a supervisor sets for every
// task, twice: once told to keep it, where the test must fail — which is what
// shows the leak is real and that this proof could fire — and once as the
// suite ordinarily runs, where it must pass.
func TestAHookTestPassesUnderASupervisorThatSetsCoreHooksPath(t *testing.T) {
	if os.Getenv(inheritGitConfig) != "" {
		t.Skip("this is the child of the proof, not the proof")
	}
	elsewhere := filepath.Join(t.TempDir(), "supervisor-hooks")
	leak := []string{
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=core.hooksPath",
		"GIT_CONFIG_VALUE_0=" + elsewhere,
	}
	run := func(extra ...string) (string, error) {
		child := exec.Command(os.Args[0], "-test.run=^TestTheHookIsWrittenWhereGitLooksForOneAndCoversEveryWorktree$", "-test.count=1")
		child.Env = append(append(os.Environ(), leak...), extra...)
		out, err := child.CombinedOutput()
		return string(out), err
	}

	if out, err := run(inheritGitConfig + "=1"); err == nil {
		t.Fatalf("with the supervisor's core.hooksPath kept, the hook test passed, so this proof cannot tell isolation from its absence:\n%s", out)
	} else if !strings.Contains(out, "--- FAIL: TestTheHookIsWrittenWhereGitLooksForOneAndCoversEveryWorktree") {
		t.Fatalf("with the supervisor's core.hooksPath kept, the child failed for some other reason: %v\n%s", err, out)
	}
	if out, err := run(); err != nil {
		t.Fatalf("under the supervisor's core.hooksPath, the suite's own isolation did not hold: %v\n%s", err, out)
	}
}
