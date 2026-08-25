package main

import (
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// scratchFixture is the shared corpus copied somewhere a run may write.
//
// `drift` walks the workspace, and a walk opens the stat cache under `.kept/`,
// so pointing it at `internal/corpus/testdata/corpus` leaves a derived
// directory inside the repository's own testdata. The digest is over content
// (docs/decision/0033), so a copy answers with the same world and the golden
// holds either way.
func scratchFixture(t *testing.T) string {
	t.Helper()
	to := filepath.Join(t.TempDir(), "corpus")
	err := filepath.WalkDir(fixture, func(from string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(fixture, from)
		if err != nil {
			return err
		}
		at := filepath.Join(to, rel)
		if d.IsDir() {
			return os.MkdirAll(at, 0o755)
		}
		src, err := os.ReadFile(from)
		if err != nil {
			return err
		}
		return os.WriteFile(at, src, 0o644)
	})
	if err != nil {
		t.Fatalf("copy the fixture corpus: %s", err)
	}
	return to
}

// corpusFor is where a verb should be pointed: a scratch copy for the verbs
// that walk the workspace, and the fixture itself for the ones that only read
// it.
func corpusFor(t *testing.T, verb string) string {
	if verb == "drift" || verb == "tick" {
		return scratchFixture(t)
	}
	return fixture
}

// The structured document is the contract, and the human rendering is not.
// These are what keeps that true in the direction it is easy to break:
// somebody improving a table must not move a byte of what a script reads.
//
// docs/decision/0027 makes the split and docs/decision/0066 is what made it
// load-bearing, by rearranging every table kept prints. A golden is the only
// check that would have noticed if one of those rearrangements had reached
// into the payload — a shared helper quietly reused, a field renamed to suit a
// column heading — because every other test here asserts what output contains
// rather than what it is.

// update rewrites the goldens instead of comparing against them. It is a flag
// rather than an environment variable so that `go test -h` lists it.
var update = flag.Bool("update", false, "rewrite the golden documents in testdata/")

// clock is the one field of the document that is allowed to differ between two
// runs over one corpus: the moment this invocation happened.
var clock = regexp.MustCompile(`"at": "[^"]*"`)

// steady is a document with its own clock removed, which is every byte the
// corpus and the ledger decide.
func steady(doc string) string { return clock.ReplaceAllString(doc, `"at": "«when»"`) }

// Two runs over one corpus produce one document. The clock is the exception and
// it is named rather than tolerated: masking every timestamp would also mask a
// ratification's date, which the corpus decides and which must not move.
func TestTheStructuredDocumentIsTheSameBytesTwice(t *testing.T) {
	for _, verb := range []string{"status", "drift"} {
		root := corpusFor(t, verb)
		_, first, _ := kept(t, verb, "--json", "--corpus", root)
		_, second, _ := kept(t, verb, "--json", "--corpus", root)

		if steady(first) != steady(second) {
			t.Errorf("kept %s --json produced two different documents:\n%s\n%s", verb, first, second)
		}
		if first == second {
			t.Errorf("kept %s --json carried no clock at all, so this check is masking nothing", verb)
		}
	}
}

// And the document is what it was. A golden per verb, byte for byte apart from
// the clock, so that a change to a table cannot change what a script reads
// without somebody deciding to update this file.
//
// Refresh with `go test ./cmd/kept -run TestTheStructuredDocumentIsWhatItWas
// -update`, and read the diff before committing it: an intended change to the
// payload is a change to a versioned surface, and 0027 says a consumer that
// cannot tell which shape it is holding will guess wrong exactly once.
func TestTheStructuredDocumentIsWhatItWas(t *testing.T) {
	for _, verb := range []string{"status", "drift"} {
		code, got, stderr := kept(t, verb, "--json", "--corpus", corpusFor(t, verb))
		if stderr != "" {
			t.Fatalf("kept %s --json wrote to stderr: %s", verb, stderr)
		}
		if code != exitOK {
			t.Fatalf("kept %s --json exited %d", verb, code)
		}

		path := filepath.Join("testdata", verb+".json")
		if *update {
			if err := os.MkdirAll("testdata", 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(steady(got)), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}

		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %s — run with -update to write it", path, err)
		}
		if steady(got) != string(want) {
			t.Errorf("kept %s --json has moved:\n%s", verb, firstDifference(string(want), steady(got)))
		}
	}
}

// firstDifference names the line that moved, because a diff of a four-thousand
// byte document printed whole says less than the one line that changed.
func firstDifference(want, got string) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := range max(len(w), len(g)) {
		var wl, gl string
		if i < len(w) {
			wl = w[i]
		}
		if i < len(g) {
			gl = g[i]
		}
		if wl != gl {
			return "line " + itoa(i+1) + "\n  was: " + wl + "\n  now: " + gl
		}
	}
	return "the documents are identical, which means this check is broken"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var out []byte
	for n > 0 {
		out = append([]byte{byte('0' + n%10)}, out...)
		n /= 10
	}
	return string(out)
}

// Exit codes do not move either. The table below is 0027's, and it is asserted
// against the same corpus the goldens are taken over so that a rendering change
// that swallowed a finding would be caught by the code as well as by the bytes.
func TestExitCodesAreUnchangedByTheRendering(t *testing.T) {
	for _, c := range []struct {
		args []string
		want int
	}{
		{[]string{"list", "--corpus", fixture}, exitOK},
		{[]string{"status", "--corpus", fixture}, exitOK},
		{[]string{"status", "--json", "--corpus", fixture}, exitOK},
		{[]string{"drift", "--corpus", corpusFor(t, "drift")}, exitOK},
		{[]string{"drift", "--json", "--corpus", corpusFor(t, "drift")}, exitOK},
		{[]string{"show", "core-packages-are-tested", "--corpus", fixture}, exitOK},
		{[]string{"list", "extra", "--corpus", fixture}, exitUsage},
	} {
		if code, _, stderr := kept(t, c.args...); code != c.want {
			t.Errorf("kept %v exited %d, want %d: %s", c.args, code, c.want, stderr)
		}
	}
}

// The human answer and the structured one are two renderings of one pass, and
// nothing a table drops may be missing from the document. `kept list` stopped
// printing titles; this is what says they are still reachable.
func TestWhatTheTablesStoppedPrintingIsStillInTheDocument(t *testing.T) {
	_, listed, _ := kept(t, "list", "--corpus", fixture)
	_, doc, _ := kept(t, "status", "--json", "--corpus", fixture)

	for _, want := range []string{
		`"title": "Core packages are tested"`,
		`"statement": "every package under internal/ has a test that runs and passes"`,
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("status --json does not carry %s:\n%s", want, doc)
		}
	}
	if strings.Contains(listed, "Core packages are tested") {
		t.Error("kept list still prints a title its slug already says; the table did not change")
	}
}
