package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// payload is the artifacts array of a document, on its own. The problems beside
// it name ids too, and those are never narrowed — see [narrowInventory] — so an
// assertion over the whole document would be asserting the opposite of what is
// intended.
func payload(t *testing.T, doc string) string {
	t.Helper()
	var out struct {
		Artifacts []map[string]any `json:"artifacts"`
	}
	if err := json.Unmarshal([]byte(doc), &out); err != nil {
		t.Fatalf("the document does not parse: %s\n%s", err, doc)
	}
	body, err := json.Marshal(out.Artifacts)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// The unfiltered rendering of every listing verb, byte for byte.
//
// A filter that changes the default output is not a filter, and this is the only
// check that would notice. The goldens were captured from the build immediately
// before the flags existed, so passing them is a statement about a commit rather
// than about a fixture somebody wrote afterwards.
//
// Refresh with `go test ./cmd/kept -run TestTheUnfilteredRenderingIsWhatItWas
// -update`, and read the diff: 0066 says the tables are free to change between
// releases, so this file moving is a decision rather than an accident.
func TestTheUnfilteredRenderingIsWhatItWas(t *testing.T) {
	for _, verb := range []string{"status", "list", "drift"} {
		code, got, stderr := kept(t, verb, "--corpus", corpusFor(t, verb))
		if stderr != "" {
			t.Fatalf("kept %s wrote to stderr: %s", verb, stderr)
		}
		if code != exitOK {
			t.Fatalf("kept %s exited %d", verb, code)
		}

		path := filepath.Join("testdata", verb+".txt")
		if *update {
			if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if got != string(want) {
			t.Errorf("kept %s is not what it was:\n--- want ---\n%s\n--- got ---\n%s", verb, want, got)
		}
	}
}

// filtered is a workspace with more than one kind in it, so that narrowing by
// kind has something to narrow away.
func filtered(t *testing.T) *workspace {
	t.Helper()
	w := newWorkspace(t, map[string]string{
		"tests-run":     "every package has a test",
		"docs-are-true": "the deck matches main.go",
	})
	w.put("docs/_schema/decision.yaml", "type: decision\nhash: [statement]\nfields:\n  statement: { type: string }\nbody: decision\n")
	w.put("docs/decision/0001-a-choice.md", "---\nid: 0001-a-choice\ntitle: 0001 — A choice\nfields:\n  statement: something was chosen\n---\n\n## Decision\n\nIt was.\n")
	return w
}

// printed is a "type:slug" as a table prints one: no quotes around it, nothing
// after the colon but a slug, and no glob in it. It is deliberately strict,
// because the lines this scans past — a problem's kind, the narrowing footer —
// all carry a colon too.
var printed = regexp.MustCompile(`^[a-z][a-z0-9]*:[a-z0-9][a-z0-9-]*$`)

// ids is every artifact a verb printed a row for, in the order it printed them.
func (w *workspace) ids(args ...string) []string {
	w.t.Helper()
	_, stdout, _ := w.run(args...)
	var out []string
	for _, line := range strings.Split(stdout, "\n") {
		if strings.HasPrefix(line, "narrowed to ") || strings.HasPrefix(line, "nothing matches ") {
			continue
		}
		for _, field := range strings.Fields(line) {
			if printed.MatchString(field) {
				out = append(out, field)
			}
		}
	}
	return out
}

// By kind, by state, by a glob over the id — and the three of them together.
func TestAFilterNarrowsByKindByStateAndByGlobAndTheyCompose(t *testing.T) {
	w := filtered(t)
	if code, _, stderr := w.run("ratify", "promise:tests-run", "--by", "the operator"); code != exitOK {
		t.Fatalf("ratify exited %d: %s", code, stderr)
	}

	cases := []struct {
		args []string
		want []string
	}{
		{[]string{"status", "--kind", "promise"}, []string{"promise:docs-are-true", "promise:tests-run"}},
		{[]string{"status", "--kind", "decision"}, []string{"decision:0001-a-choice"}},
		{[]string{"status", "--state", "ratified"}, []string{"promise:tests-run"}},
		{[]string{"status", "--state", "never-ratified"}, []string{"decision:0001-a-choice", "promise:docs-are-true"}},
		{[]string{"status", "--id", "promise:*"}, []string{"promise:docs-are-true", "promise:tests-run"}},
		{[]string{"status", "--id", "*a-choice"}, []string{"decision:0001-a-choice"}},

		// Repeating a flag is "or".
		{[]string{"status", "--kind", "promise", "--kind", "decision"},
			[]string{"decision:0001-a-choice", "promise:docs-are-true", "promise:tests-run"}},

		// Two flags together are "and", which is the composition the whole
		// point of three separate flags.
		{[]string{"status", "--kind", "promise", "--state", "ratified"}, []string{"promise:tests-run"}},
		{[]string{"status", "--kind", "promise", "--id", "*tests*", "--state", "ratified"}, []string{"promise:tests-run"}},

		// And list narrows the same way, with no --state of its own.
		{[]string{"list", "--kind", "decision"}, []string{"decision:0001-a-choice"}},
		{[]string{"list", "--id", "promise:*"}, []string{"promise:docs-are-true", "promise:tests-run"}},
	}
	for _, tc := range cases {
		got := w.ids(tc.args...)
		if strings.Join(got, " ") != strings.Join(tc.want, " ") {
			t.Errorf("kept %s selected %v, want %v", strings.Join(tc.args, " "), got, tc.want)
		}
	}
}

// --kind reads docs/_schema, so a kind nobody wrote code for is filterable the
// day its schema lands. This is the rule the whole of `kept new` was built on
// and it is checked the same way: by adding a schema and nothing else.
func TestKindTakesItsValuesFromTheSchemaDirectory(t *testing.T) {
	w := filtered(t)

	// A type this build has never heard of, declared the only way a type is
	// ever declared here.
	w.put("docs/_schema/gizmo.yaml", "type: gizmo\nhash: [statement]\nfields:\n  statement: { type: string }\n")
	w.put("docs/gizmo/one.md", "---\nid: one\ntitle: One\nfields:\n  statement: a gizmo\n---\n")

	code, stdout, stderr := w.run("list", "--kind", "gizmo")
	if code != exitOK {
		t.Fatalf("list --kind gizmo exited %d: %s%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "gizmo:one") {
		t.Errorf("a kind added by writing a schema is not filterable:\n%s", stdout)
	}
	if strings.Contains(stdout, "promise:") {
		t.Errorf("--kind gizmo did not narrow:\n%s", stdout)
	}

	// And a type nothing declares is refused by name, with the declared ones
	// listed, rather than answered with an empty table.
	code, _, stderr = w.run("list", "--kind", "widget")
	if code != exitUsage {
		t.Errorf("--kind widget exited %d, want %d", code, exitUsage)
	}
	mustContain(t, "the refusal", stderr, `no "widget" type`, "decision", "gizmo", "promise")
}

// The glob matches the whole printed id. A pattern that quietly matched the slug
// alone would answer differently from what the reader is looking at.
func TestTheGlobMatchesTheWholePrintedID(t *testing.T) {
	w := filtered(t)

	if got := w.ids("status", "--id", "promise:tests-run"); len(got) != 1 || got[0] != "promise:tests-run" {
		t.Errorf("the whole id does not match itself: %v", got)
	}

	// The slug alone is not the id, so it matches nothing — and says so, with
	// the pattern that would have worked.
	code, stdout, _ := w.run("status", "--id", "tests-run")
	if code != exitUnknown {
		t.Errorf("a pattern matching no id exited %d, want %d", code, exitUnknown)
	}
	mustContain(t, "the answer", stdout, "nothing matches", `"type:slug"`, `"*:tests-run"`)

	// A malformed glob is the operator's error and is refused as one.
	if code, _, stderr := w.run("status", "--id", "promise:[a-"); code != exitUsage {
		t.Errorf("a bad glob exited %d, want %d: %s", code, exitUsage, stderr)
	}
}

// The exit code is about the filtered set. A script asking "are my promises
// ratified" wants the answer about promises, and the opposite choice is
// defensible enough that somebody will assume it — so it is pinned.
func TestTheExitCodeDescribesTheFilteredSet(t *testing.T) {
	w := filtered(t)
	for _, id := range []string{"promise:tests-run", "promise:docs-are-true"} {
		if code, _, stderr := w.run("ratify", id, "--by", "the operator"); code != exitOK {
			t.Fatalf("ratify %s exited %d: %s", id, code, stderr)
		}
	}

	// One of them is edited after the fact, which is the finding kept status
	// exits 1 for.
	w.put("docs/promise/tests-run.md",
		strings.Replace(promiseAt("tests-run", "every package has a test"),
			"obs.tests.failed == 0", "obs.tests.failed < 3", 1))

	if code, _, _ := w.run("status"); code != exitDrift {
		t.Fatalf("unfiltered status exited %d with an edited ratification, want %d", code, exitDrift)
	}

	// Narrowed onto the artifact that is fine, the answer is that it is fine.
	if code, stdout, _ := w.run("status", "--id", "promise:docs-are-true"); code != exitOK {
		t.Errorf("status narrowed onto a clean artifact exited %d, want %d:\n%s", code, exitOK, stdout)
	}
	if code, _, _ := w.run("status", "--kind", "decision"); code != exitOK {
		t.Errorf("status narrowed onto an unaffected kind exited %d, want %d", code, exitOK)
	}

	// Narrowed onto the artifact that is not, it is still 1.
	if code, _, _ := w.run("status", "--id", "promise:tests-run"); code != exitDrift {
		t.Errorf("status narrowed onto the edited artifact exited %d, want %d", code, exitDrift)
	}

	// And --json carries the same code it exited with, filtered the same way.
	code, stdout, _ := w.run("status", "--json", "--kind", "decision")
	if code != exitOK {
		t.Errorf("status --json --kind decision exited %d, want %d", code, exitOK)
	}
	mustContain(t, "the document", stdout, `"exit": 0`, "decision:0001-a-choice")
	if strings.Contains(payload(t, stdout), "promise:") {
		t.Errorf("--json was not narrowed:\n%s", stdout)
	}
}

// A selector naming nothing is the question never having been put, which is
// exit 3 and a sentence — never an empty table. internal/gather/tests.go has the
// same rule one level down: a suite of no packages is not ok, because nothing
// was demonstrated, and a pattern matching nothing is the ordinary way that
// happens.
func TestASelectorThatMatchesNothingSaysSoAndIsNotAnEmptyTable(t *testing.T) {
	w := filtered(t)

	// A kind the corpus declares and holds none of. The message says which,
	// because "there are no desires" and "you spelled it wrong" want different
	// things from the reader.
	w.put("docs/_schema/desire.yaml", "type: desire\nfields:\n  statement: { type: string }\n")

	for _, verb := range []string{"status", "list", "drift"} {
		code, stdout, _ := w.run(verb, "--kind", "desire")
		if code != exitUnknown {
			t.Errorf("kept %s --kind desire exited %d, want %d", verb, code, exitUnknown)
		}
		mustContain(t, "kept "+verb, stdout,
			"nothing matches --kind desire", "declares desire and the corpus holds no artifact")
	}

	// And a glob that matches nothing at all, with nothing to hint at.
	code, stdout, _ := w.run("status", "--id", "*nothing-here*")
	if code != exitUnknown {
		t.Errorf("a glob matching nothing exited %d, want %d", code, exitUnknown)
	}
	mustContain(t, "the answer", stdout, `nothing matches --id "*nothing-here*"`)
}

// A state nothing is in is an answer rather than an unanswered question: the
// population exists, it was asked about, and none of it is in that state. It is
// still said out loud, because an empty table reads as success.
func TestAStateNothingIsInIsAnAnswerRatherThanAnUnansweredQuestion(t *testing.T) {
	w := filtered(t)

	code, stdout, _ := w.run("status", "--state", "released")
	if code != exitOK {
		t.Errorf("a state nothing is in exited %d, want %d", code, exitOK)
	}
	mustContain(t, "the answer", stdout, "3 artifacts, and none of them is released")

	code, stdout, _ = w.run("status", "--kind", "promise", "--state", "released")
	if code != exitOK {
		t.Errorf("a state nothing is in exited %d, want %d", code, exitOK)
	}
	mustContain(t, "the answer", stdout, "2 artifacts matching --kind promise, and none of them is released")
}

// One flag name, and each verb takes only the vocabulary it prints. A word from
// the other one is refused with the vocabulary it does take, which is what keeps
// --state from meaning two things depending on where it is typed.
func TestEachVerbTakesOnlyTheStateVocabularyItReports(t *testing.T) {
	w := filtered(t)

	code, _, stderr := w.run("drift", "--state", "ratified")
	if code != exitUsage {
		t.Errorf("drift --state ratified exited %d, want %d", code, exitUsage)
	}
	mustContain(t, "the refusal", stderr, "drift reports drift kind", "not-kept", "unverified")

	code, _, stderr = w.run("status", "--state", "not-kept")
	if code != exitUsage {
		t.Errorf("status --state not-kept exited %d, want %d", code, exitUsage)
	}
	mustContain(t, "the refusal", stderr, "status reports standing", "ratified", "never-ratified")

	// list reports no state at all, and says where the question is asked.
	code, _, stderr = w.run("list", "--state", "ratified")
	if code != exitUsage {
		t.Errorf("list --state exited %d, want %d", code, exitUsage)
	}
	mustContain(t, "the refusal", stderr, "list reports no state", "kept status --state ratified")

	// Both spellings of the one state whose name has a space in it.
	for _, spelling := range []string{"never-ratified", "never ratified"} {
		if code, _, stderr := w.run("status", "--state", spelling); code != exitOK {
			t.Errorf("--state %q exited %d: %s", spelling, code, stderr)
		}
	}
}

// The document is the same query rendered differently, not a second surface.
func TestTheStructuredDocumentIsNarrowedTheSameWay(t *testing.T) {
	w := filtered(t)

	_, table, _ := w.run("status", "--kind", "promise")
	_, doc, _ := w.run("status", "--json", "--kind", "promise")

	for _, id := range []string{"promise:docs-are-true", "promise:tests-run"} {
		mustContain(t, "the table", table, id)
		mustContain(t, "the document", doc, id)
	}
	if strings.Contains(payload(t, doc), "decision:0001-a-choice") {
		t.Errorf("--json carried an artifact the table did not:\n%s", doc)
	}
	// The summary is over the filtered set too, or a consumer adding it up
	// would get a number no row of the document supports.
	if strings.Count(doc, `"unevaluated": 2`) != 1 {
		t.Errorf("the summary was not narrowed with the artifacts:\n%s", doc)
	}
}

// A filter narrows the rows a reader asked for and never the defects somebody
// must fix. A problem with no reader is how a corpus rots quietly.
func TestProblemsAreNeverNarrowed(t *testing.T) {
	// The promises here read an observation they do not gather, which is a
	// defect of the corpus rather than of any row a reader asked to see.
	w := filtered(t)

	_, whole, _ := w.run("list")
	mustContain(t, "the unfiltered list", whole, "ungathered-observation", "promise/tests-run.md")

	_, narrowed, _ := w.run("list", "--kind", "decision")
	mustContain(t, "a narrowed list", narrowed, "ungathered-observation", "promise/tests-run.md")
	if strings.Contains(narrowed, "\npromise:tests-run\n") {
		t.Errorf("a narrowed list printed a row it was not asked for:\n%s", narrowed)
	}
}

// drift narrows the same three ways, over its own vocabulary. This is the verb
// where a shared flag name would have gone wrong quietly: the words it prints
// are truth and share only `absent` with standing.
func TestDriftNarrowsOverItsOwnVocabulary(t *testing.T) {
	w := newChecked(t)
	w.install(t)
	w.run("ratify", "the-marker-says-ok", "--by", "the operator")
	w.run()

	// It is not kept, and unfiltered that is exit 1.
	if code, stdout, _ := w.run("drift"); code != exitDrift {
		t.Fatalf("drift exited %d, want %d\n%s", code, exitDrift, stdout)
	}

	// Narrowed onto the drift kind it actually has, the row and the code are
	// both still there.
	code, stdout, _ := w.run("drift", "--state", "not-kept")
	if code != exitDrift {
		t.Errorf("drift --state not-kept exited %d, want %d\n%s", code, exitDrift, stdout)
	}
	mustContain(t, "drift", stdout, "promise:the-marker-says-ok", "not-kept")

	// Narrowed onto one it does not have, the answer is that nothing is in that
	// state — and the exit code follows the narrowed set, so it is no longer 1.
	code, stdout, _ = w.run("drift", "--state", "stale")
	if code != exitOK {
		t.Errorf("drift --state stale exited %d, want %d\n%s", code, exitOK, stdout)
	}
	mustContain(t, "drift", stdout, "and none of them is stale")

	// And by kind and by glob, over the same subject.
	if got := w.ids("drift", "--kind", "promise", "--state", "not-kept"); len(got) != 1 {
		t.Errorf("drift --kind promise selected %v", got)
	}
	if code, _, _ := w.run("drift", "--kind", "promise", "--id", "*:nothing-here"); code != exitUnknown {
		t.Errorf("a selector matching nothing on drift exited %d, want %d", code, exitUnknown)
	}
}
