package gather

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/model"
)

// fakeFlag makes the test binary run as the program under gather rather than as
// a test.
//
// Every process test here runs this rather than a real tool, and the constraint
// is not tidiness: a gatherer's contract is about programs that are slow,
// noisy, absent or killed, and none of those is reproducible with a toolchain
// that has to be installed. It is the test binary itself because that is the
// one executable a test is guaranteed to have, on every platform, with no shell
// and nothing to build.
const fakeFlag = "-kept.fake"

// stubArgs is the file a stand-in program reads its mode from.
//
// The tests gatherer builds its own argv — `go test -json ./...` — so a
// stand-in for `go` cannot be given arguments the way the command gatherer's
// run: line can. It gets them from a file beside it instead, which is what lets
// a test point the gatherer at a canned stream without a shell, a script, or a
// second executable to build.
const stubArgs = "kept.fake.args"

func TestMain(m *testing.M) {
	if args, ok := stubMode(); ok {
		os.Exit(fake(args))
	}
	for i, arg := range os.Args {
		if arg == fakeFlag {
			os.Exit(fake(os.Args[i+1:]))
		}
	}
	os.Exit(m.Run())
}

// stubMode reads the mode of a stand-in from the file beside it. os.Args[0] is
// how the program was invoked, which is the link; os.Executable resolves it
// back to the test binary on some platforms and would look in the wrong place.
func stubMode() ([]string, bool) {
	if len(os.Args) == 0 {
		return nil, false
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(os.Args[0]), stubArgs))
	if err != nil {
		return nil, false
	}
	return strings.Split(strings.TrimRight(string(src), "\n"), "\n"), true
}

// writeStub puts a copy of the test binary somewhere named `go`, with the mode
// it should run in beside it, and returns the path.
func writeStub(t *testing.T, emit, exit string) string {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("the test binary cannot find itself: %v", err)
	}
	dir := t.TempDir()
	stub := filepath.Join(dir, "go")
	if err := os.Symlink(self, stub); err != nil {
		src, readErr := os.ReadFile(self)
		if readErr != nil {
			t.Fatalf("no stand-in for go: %v / %v", err, readErr)
		}
		if err := os.WriteFile(stub, src, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	write(t, filepath.Join(dir, stubArgs), "emit\n"+emit+"\n"+exit+"\n")
	return stub
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// textList reads a declared field back as the list of names it is.
func textList(v any) []string {
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		s, _ := item.(string)
		out = append(out, s)
	}
	return out
}

func same(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// records reads the packages field back as what it is: one record per package.
func records(t *testing.T, v any) []map[string]any {
	t.Helper()
	items, ok := v.([]any)
	if !ok {
		t.Fatalf("packages is %#v, and a promise folds over it", v)
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("a package is %#v", item)
		}
		out = append(out, m)
	}
	return out
}

// address is what the ledger would file a measurement under, with everything
// but the data held still.
func address(t *testing.T, fields Fields) string {
	t.Helper()
	return ledger.Measured(model.Observation{
		Gatherer:    "tests@1",
		World:       world("one"),
		ScopeDigest: "sha256:" + strings.Repeat("a", 64),
		Data:        fields,
	}).Address()
}

// fake is the program. Each mode is one of the shapes a gatherer has to have an
// answer for.
func fake(args []string) int {
	if len(args) == 0 {
		return 2
	}
	switch args[0] {
	case "print":
		fmt.Print(strings.Join(args[1:], " "))
		return 0

	case "warn":
		fmt.Fprint(os.Stderr, strings.Join(args[1:], " "))
		return 0

	case "emit":
		src, err := os.ReadFile(args[1])
		if err != nil {
			return 3
		}
		os.Stdout.Write(src)
		if len(args) > 2 {
			code, _ := strconv.Atoi(args[2])
			return code
		}
		return 0

	case "exit":
		code, _ := strconv.Atoi(args[1])
		return code

	case "flood":
		// More than the raw bound, in chunks, so the sink sees the overflow
		// while the program is still running and can kill it.
		out := os.Stdout
		if args[1] == "stderr" {
			out = os.Stderr
		}
		chunk := strings.Repeat("x", 64<<10)
		for range (MaxRawOutput / len(chunk)) + 4 {
			if _, err := out.WriteString(chunk); err != nil {
				return 0
			}
		}
		return 0

	case "selfkill":
		// A program killed by something that is not kept's own deadline: an
		// OOM killer, an operator, a parent shutting down.
		fmt.Print("partial out")
		signalSelf()
		time.Sleep(30 * time.Second)
		return 0

	case "sleep":
		ms, _ := strconv.Atoi(args[1])
		time.Sleep(time.Duration(ms) * time.Millisecond)
		return 0

	case "spawn-mark":
		// One line per run, appended, so a test can count how many times the
		// program actually ran.
		f, err := os.OpenFile(args[1], os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return 7
		}
		defer f.Close()
		if _, err := f.WriteString("ran\n"); err != nil {
			return 7
		}
		return 0

	case "spawn":
		// A child that outlives its parent unless the whole group is killed.
		// This is what docs/decision/0026 means by "a killed process is
		// killed, including its children".
		self, err := os.Executable()
		if err != nil {
			return 4
		}
		child := exec.Command(self, fakeFlag, "sleep", "60000")
		if err := child.Start(); err != nil {
			return 5
		}
		if err := os.WriteFile(args[1], []byte(strconv.Itoa(child.Process.Pid)), 0o600); err != nil {
			return 6
		}
		time.Sleep(60 * time.Second)
		return 0
	}
	return 2
}

// runLine builds a run: line invoking the fake program. Each argument is single
// quoted, because a temporary directory's path is not guaranteed to be free of
// anything the argv reader treats as a separator.
func runLine(t *testing.T, args ...string) string {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("the test binary cannot find itself: %v", err)
	}
	parts := append([]string{self, fakeFlag}, args...)
	for i, p := range parts {
		if strings.Contains(p, "'") {
			t.Fatalf("the fake program cannot be given %q", p)
		}
		parts[i] = "'" + p + "'"
	}
	return strings.Join(parts, " ")
}

// request is the ordinary request the process tests run under: this directory,
// a world that names a tree, and a deadline short enough that a wedged test
// fails rather than hangs.
func request(t *testing.T, args map[string]any) Request {
	t.Helper()
	return Request{
		Root:     t.TempDir(),
		Globs:    []string{Everything},
		Scope:    "sha256:" + strings.Repeat("a", 64),
		Args:     args,
		World:    world("one"),
		Deadline: 30 * time.Second,
	}
}
