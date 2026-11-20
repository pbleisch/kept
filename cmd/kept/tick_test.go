package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/gittest"
)

// The command's own stand-in program. A tick's behaviour is about programs that
// answer, fail, or are not installed at all, and none of those is reproducible
// against a toolchain that has to be present on the machine running the tests.
const fakeFlag = "-kept.fake"

// TestMain also runs the suite against the git configuration its tests write
// and none a parent process handed down: init and doctor ask git where hooks
// live, and a core.hooksPath set through the environment answers for them. See
// internal/gittest.
func TestMain(m *testing.M) {
	for i, arg := range os.Args {
		if arg == fakeFlag {
			os.Exit(fake(os.Args[i+1:]))
		}
	}
	gittest.Isolate()
	os.Exit(m.Run())
}

func fake(args []string) int {
	if len(args) != 2 || args[0] != "read" {
		return 2
	}
	src, err := os.ReadFile(args[1])
	if err != nil {
		return 1
	}
	fmt.Print(string(src))
	return 0
}

// checked is a workspace holding one promise that reads a marker through a
// program which is not installed until install is called.
type checked struct {
	*workspace
	tool string
}

func newChecked(t *testing.T) *checked {
	t.Helper()
	w := newWorkspace(t, nil)
	c := &checked{workspace: w, tool: filepath.Join(t.TempDir(), "tool")}
	w.put("src/marker.txt", "nothing yet\n")
	w.put("docs/promise/the-marker-says-ok.md", "---\nid: the-marker-says-ok\ntype: promise\ntitle: The marker says ok\n"+`fields:
  statement: the marker under src says ok
  gather:
    marker:
      gatherer: command
      run: "`+quoted(t, c.tool, fakeFlag, "read", "src/marker.txt")+`"
      scope: "src/**"
      fields:
        ok: "'ok' in lines(stdout)"
  hold_when: obs.marker.ok
  trigger: cadence
---

Why: a promise nobody can check is not one.
`)
	return c
}

func (c *checked) install(t *testing.T) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(self, c.tool); err != nil {
		src, readErr := os.ReadFile(self)
		if readErr != nil {
			t.Fatalf("no stand-in for the program: %v / %v", err, readErr)
		}
		if err := os.WriteFile(c.tool, src, 0o700); err != nil {
			t.Fatal(err)
		}
	}
}

func quoted(t *testing.T, parts ...string) string {
	t.Helper()
	for i, p := range parts {
		if strings.ContainsAny(p, "'\"") {
			t.Fatalf("the fake program cannot be given %q", p)
		}
		parts[i] = "'" + p + "'"
	}
	return strings.Join(parts, " ")
}

// The tick prints what is kept, what is not, and what nobody could measure —
// and the exit code tells the three apart, which is the whole reason there are
// four codes rather than two.
func TestTheTickAnswersInThreeValuesAndExitsOnEach(t *testing.T) {
	w := newChecked(t)
	if code, _, stderr := w.run("ratify", "the-marker-says-ok", "--by", "the operator"); code != exitOK {
		t.Fatalf("ratify exited %d: %s", code, stderr)
	}

	// Nobody could measure. Exit 3 and never 1: a CI job that reads "the
	// toolchain was missing" as "the promise is broken" produces exactly the
	// false report the three-valued verdict exists to prevent.
	code, stdout, _ := w.run()
	if code != exitUnknown {
		t.Fatalf("a missing toolchain exited %d, want %d\n%s", code, exitUnknown, stdout)
	}
	mustContain(t, "the tick", stdout, "unknown", "could not be run")
	if strings.Contains(stdout, "not-kept") {
		t.Fatalf("a machine that could not measure reported the promise broken:\n%s", stdout)
	}

	// The instrument arrives, and the promise does not hold.
	w.install(t)
	code, stdout, _ = w.run("--all")
	if code != exitDrift {
		t.Fatalf("a promise that is not kept exited %d, want %d\n%s", code, exitDrift, stdout)
	}
	mustContain(t, "the tick", stdout, "not-kept", "refused", "provider")

	// The world changes, and it holds.
	w.put("src/marker.txt", "ok\n")
	code, stdout, _ = w.run()
	if code != exitOK {
		t.Fatalf("a kept promise exited %d, want %d\n%s", code, exitOK, stdout)
	}
	mustContain(t, "the tick", stdout, "kept")
}

// kept why says why a drift was left alone. That is the question the verb
// exists for: truth can be re-derived by observing again and a refusal cannot,
// so the answer has to come from a record written at the time.
func TestWhySaysWhyADriftWasLeftAlone(t *testing.T) {
	w := newChecked(t)
	w.install(t)
	w.run("ratify", "the-marker-says-ok", "--by", "the operator")
	w.run()

	code, stdout, stderr := w.run("why", "the-marker-says-ok")
	if code != exitDrift {
		t.Fatalf("why exited %d, want %d: %s", code, exitDrift, stderr)
	}

	// Three sections against one artifact.
	mustContain(t, "why", stdout, "standing", "truth", "disposition")
	// The standing that makes it drift, the verdict that makes it drift, and
	// the reason nothing was done — with the condition named, because a
	// refusal that names none is the shrug this record exists to replace.
	mustContain(t, "why", stdout,
		"ratified",
		"not-kept",
		"refused",
		"provider",
		"unset is not none",
	)
	// And the histories behind both, which is what makes "not repaired for six
	// days" answerable at all.
	if strings.Count(stdout, "history") != 2 {
		t.Errorf("why does not carry both histories:\n%s", stdout)
	}
}

// A survey observes nothing, so kept drift can be run against a checkout
// nothing may write to — and it still says what the last tick decided.
func TestDriftReadsWhatTheTickFoundAndObservesNothing(t *testing.T) {
	w := newChecked(t)
	w.install(t)
	w.run("ratify", "the-marker-says-ok", "--by", "the operator")
	w.run()

	before := w.observations(t)
	code, stdout, _ := w.run("drift")
	if code != exitDrift {
		t.Fatalf("drift exited %d, want %d\n%s", code, exitDrift, stdout)
	}
	mustContain(t, "drift", stdout, "the-marker-says-ok", "not-kept", "refused", "provider")
	if after := w.observations(t); after != before {
		t.Errorf("drift recorded %d observations", after-before)
	}

	// And when there is none, it says so rather than printing an empty table.
	w.put("src/marker.txt", "ok\n")
	w.run()
	if code, stdout, _ := w.run("drift"); code != exitOK {
		t.Errorf("drift exited %d over a kept corpus\n%s", code, stdout)
	} else {
		mustContain(t, "drift", stdout, "nothing with standing is without truth")
	}
}

// The document carries the same three-valued answer, with the schema version on
// it. A boolean here would re-introduce exactly the false report the model
// exists to prevent, so the guard is that the value is a string and one of
// three.
func TestTheDocumentCarriesAThreeValuedVerdictAndASchema(t *testing.T) {
	w := newChecked(t)
	w.run("ratify", "the-marker-says-ok", "--by", "the operator")

	// unknown, then not-kept, then kept — the same three the human rendering
	// prints, out of one envelope.
	want := []string{"unknown", "not-kept", "kept"}
	for i, value := range want {
		switch i {
		case 1:
			w.install(t)
		case 2:
			w.put("src/marker.txt", "ok\n")
		}

		args := []string{"--json"}
		if i == 1 {
			args = append(args, "--all")
		}
		_, stdout, stderr := w.run(args...)

		var doc struct {
			Schema    string                `json:"schema"`
			Verb      string                `json:"verb"`
			Exit      int                   `json:"exit"`
			World     struct{ Tree string } `json:"world"`
			Artifacts []struct {
				ID    string `json:"id"`
				Truth *struct {
					Value  any    `json:"value"`
					Detail string `json:"detail"`
				} `json:"truth"`
				Drift *struct {
					Kind string `json:"kind"`
				} `json:"drift"`
				Disposition *struct {
					Outcome   string `json:"outcome"`
					Condition string `json:"condition"`
					Detail    string `json:"detail"`
				} `json:"disposition"`
			} `json:"artifacts"`
			Summary map[string]int `json:"summary"`
		}
		if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
			t.Fatalf("the document does not parse: %v\n%s\n%s", err, stdout, stderr)
		}

		if doc.Schema != Schema {
			t.Fatalf("the document says schema %q, want %q", doc.Schema, Schema)
		}
		if doc.Verb != "tick" || doc.World.Tree == "" {
			t.Errorf("verb %q in world %q", doc.Verb, doc.World.Tree)
		}
		if len(doc.Artifacts) != 1 || doc.Artifacts[0].Truth == nil {
			t.Fatalf("the document carries no verdict:\n%s", stdout)
		}

		got := doc.Artifacts[0].Truth.Value
		if _, isBool := got.(bool); isBool {
			t.Fatalf("the verdict is a boolean, which collapses not-kept and unknown into one answer")
		}
		text, isString := got.(string)
		if !isString {
			t.Fatalf("the verdict is %T, and a verdict is one of three words", got)
		}
		if text != value {
			t.Fatalf("the verdict is %q, want %q\n%s", text, value, stdout)
		}
		if text == "unknown" && doc.Artifacts[0].Truth.Detail == "" {
			t.Error("an unknown with no reason is a shrug in a machine-readable wrapper")
		}
		if doc.Summary[value] != 1 {
			t.Errorf("the summary counts %v, and one artifact is %s", doc.Summary, value)
		}

		// The refusal travels in the document too, so an agent driving kept can
		// answer the negative case without parsing a table meant for a person.
		if value != "kept" {
			d := doc.Artifacts[0].Disposition
			if d == nil || d.Outcome == "" || d.Condition == "" {
				t.Errorf("the document says nothing about why the drift was left alone:\n%s", stdout)
			}
		}
	}
}

// status answers in the same envelope, with truth absent rather than empty: it
// looks at the register and never at the world, and an absent verdict says so.
func TestStatusSpeaksTheSameDocument(t *testing.T) {
	w := newChecked(t)
	w.run("ratify", "the-marker-says-ok", "--by", "the operator")

	_, stdout, _ := w.run("status", "--json")
	var doc struct {
		Schema    string    `json:"schema"`
		Verb      string    `json:"verb"`
		World     *struct{} `json:"world"`
		Artifacts []struct {
			ID       string `json:"id"`
			Standing struct {
				State string `json:"state"`
			} `json:"standing"`
			Truth *struct{} `json:"truth"`
			Due   *struct{} `json:"due"`
		} `json:"artifacts"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("the document does not parse: %v\n%s", err, stdout)
	}
	if doc.Schema != Schema || doc.Verb != "status" {
		t.Fatalf("schema %q verb %q", doc.Schema, doc.Verb)
	}
	if doc.World != nil {
		t.Error("status named a world it never walked")
	}
	if len(doc.Artifacts) != 1 || doc.Artifacts[0].Standing.State != "ratified" {
		t.Fatalf("status: %s", stdout)
	}
	if doc.Artifacts[0].Truth != nil || doc.Artifacts[0].Due != nil {
		t.Error("status reported a verdict, and it observed nothing")
	}
}

// observations counts what has been filed, so a test can assert that a read
// wrote nothing.
func (w *workspace) observations(t *testing.T) int {
	t.Helper()
	var n int
	root := filepath.Join(w.root, ".kept", "ledger", "observations")
	_ = filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			n++
		}
		return nil
	})
	return n
}
