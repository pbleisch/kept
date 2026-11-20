package assess

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/thread"
)

// namedHold names one test the way a-receipt-costs-the-ledger-about-what-it-holds
// does, and a string literal that is not a test beside it.
const namedHold = `obs.tests.ok and count(obs.tests.packages, p => p.name == "./internal/thread") == 1 and "TestBound" in obs.tests.cases`

var testsGather = map[string]model.GatherSpec{"tests": {Gatherer: "go/tests"}}

// boundFile is a test file holding a named test, a helper and the constant that
// sizes its fixture, in the shape of internal/thread/receiptcost_test.go.
const boundFile = `package thread

import "testing"

// replayBytes has to carry the receipt past the allowance.
const replayBytes = 80 << 10

// TestBound holds every write to the bound.
func TestBound(t *testing.T) {
	// The guard: a replay too small for the bound to fire is refused.
	if held := replay(replayBytes); held <= 64<<10 {
		t.Fatalf("too small: %d", held)
	}
}

func replay(n int) int { return n }
`

func sources(base, anchor string) map[string]TestSource {
	src := TestSource{Read: true}
	if base != "" {
		src.Base = []byte(base)
	}
	if anchor != "" {
		src.Anchor = []byte(anchor)
	}
	return map[string]TestSource{"internal/thread/bound_test.go": src}
}

// TestChangingATestThePredicateNamesFiresAndWritingBesideItDoesNot is the
// instrument a go/tests promise names, broken each way a repair of
// a-receipt-costs-the-ledger-about-what-it-holds broke it on 2026-11-20 — the
// vacuity guard rewritten, the fixture enlarged — and held quiet for what a
// repair is entitled to do in that file: correct prose, and add beside it.
func TestChangingATestThePredicateNamesFiresAndWritingBesideItDoesNot(t *testing.T) {
	for _, c := range []struct {
		name   string
		anchor string
		fires  string
	}{
		{"the guard weakened", strings.Replace(boundFile, "held <= 64<<10", "held < 0", 1), "TestBound"},
		{"the fixture enlarged", strings.Replace(boundFile, "80 << 10", "320 << 10", 1), "replayBytes"},
		{"a helper changed", strings.Replace(boundFile, "return n }", "return n * 4 }", 1), "replay"},
		{"the file deleted", "", "deleted the file"},
		{"the file left unparseable", boundFile + "\nfunc {", "left it unparseable"},
		{"prose corrected", strings.Replace(boundFile, "// The guard: a replay too small for the bound to fire is refused.", "// The guard, reworded,\n\t// over two lines.\n", 1), ""},
		{"a test added beside it", boundFile + "\nfunc TestSeals(t *testing.T) { _ = replay(1) }\n", ""},
		{"nothing", boundFile, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			lines, unread := namedTestChanges(namedHold, testsGather, sources(boundFile, c.anchor))
			if len(unread) != 0 {
				t.Fatalf("unread %v", unread)
			}
			if c.fires == "" {
				if len(lines) != 0 {
					t.Fatalf("fired on a change that is not to the instrument: %v", lines)
				}
				return
			}
			if len(lines) != 1 || !strings.Contains(lines[0], "declares TestBound, which the predicate names") || !strings.Contains(lines[0], c.fires) {
				t.Fatalf("lines = %v, want one naming TestBound and %q", lines, c.fires)
			}
		})
	}
}

// TestATestTheRepairWroteIsNotAChangeToTheInstrument is the other direction of
// the same rule. A promise may name a test nobody has written yet — the repair
// is what writes it — and a repair writing what its promise asked for has not
// moved the standard. So only a named test that stood at the base makes its file
// the instrument.
func TestATestTheRepairWroteIsNotAChangeToTheInstrument(t *testing.T) {
	withoutNamed := strings.Replace(boundFile, "func TestBound(", "func TestOther(", 1)
	if lines, _ := namedTestChanges(namedHold, testsGather, sources("", boundFile)); len(lines) != 0 {
		t.Fatalf("a new file holding the named test fired: %v", lines)
	}
	if lines, _ := namedTestChanges(namedHold, testsGather, sources(withoutNamed, boundFile)); len(lines) != 0 {
		t.Fatalf("a named test added to a file that held none fired: %v", lines)
	}
	if lines, _ := namedTestChanges(namedHold, map[string]model.GatherSpec{"c": {Gatherer: "corpus"}}, sources(boundFile, "")); len(lines) != 0 {
		t.Fatalf("a promise with no go/tests instrument fired on a test file: %v", lines)
	}
	if lines, _ := namedTestChanges(`obs.tests.ok`, testsGather, sources(boundFile, "")); len(lines) != 0 {
		t.Fatalf("a predicate naming no test fired: %v", lines)
	}
	_, unread := namedTestChanges(namedHold, testsGather, map[string]TestSource{"internal/thread/bound_test.go": {}})
	if len(unread) != 1 {
		t.Fatalf("a file that could not be read was not reported as unread: %v", unread)
	}
}

// TestInstrumentChangedReadsANamedTestOffTheAnchorsOwnCommits is the whole
// path over a real repository: the entry reads the touched test file at the
// anchor's parent and at the anchor, a rename's base from where it came from,
// and the check fires naming it.
func TestInstrumentChangedReadsANamedTestOffTheAnchorsOwnCommits(t *testing.T) {
	repository := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repository, "-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(repository, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("init", "-q")
	write("internal/thread/bound_test.go", boundFile)
	write("internal/thread/receipt.go", "package thread\n")
	run("add", "-A")
	run("commit", "-q", "-m", "base")
	base := run("rev-parse", "HEAD")

	run("mv", "internal/thread/bound_test.go", "internal/thread/cost_test.go")
	write("internal/thread/cost_test.go", strings.Replace(boundFile, "held <= 64<<10", "held < 0", 1))
	write("internal/thread/receipt.go", "package thread\n\nconst journaled = true\n")
	run("add", "-A")
	run("commit", "-q", "-m", "repair")
	anchor := thread.Anchor{Commit: run("rev-parse", "HEAD"), Parent: base, Promise: "promise:bound"}

	touched, err := diff(context.Background(), repository, anchor, 0)
	if err != nil {
		t.Fatal(err)
	}
	s := subject{entry: Entry{
		Delivered:   true,
		Packet:      thread.Contents{Promise: thread.RatifiedPromise{ID: "promise:bound", HoldWhen: namedHold, Gather: testsGather}},
		Anchor:      anchor,
		Anchored:    true,
		Touched:     touched,
		TouchedRead: true,
		TestSources: testSources(context.Background(), repository, anchor, touched, 0),
	}}
	found := s.instrumentChanged()
	if found.Outcome != Fired || len(found.Lines) != 1 || !strings.Contains(found.Lines[0], "cost_test.go: declares TestBound") {
		t.Fatalf("instrument-changed = %s %v, want it fired on the renamed file that declared the named test", found.Outcome, found.Lines)
	}
	if !strings.Contains(found.Lines[0], "the repair changed TestBound") {
		t.Fatalf("the finding does not name what changed: %v", found.Lines)
	}
}
