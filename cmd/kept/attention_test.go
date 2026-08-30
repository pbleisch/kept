package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The default view of `kept status` and everything it decides.
//
// The rule under test is [needsAttention], and the reason it needs this many
// tests is that its failure mode is silence: a view that drops a finding still
// prints a table, still exits with the same code, and looks exactly like a
// corpus with nothing wrong in it.

// documentSchema is a kind that binds nothing, which is the one artifact the
// default view leaves out. `kept ratify` refuses it — there is no content
// version to key a grant to — so it can never leave the unratified state.
const documentSchema = "type: document\nhash: []\nfields:\n  statement: { type: string }\n"

func documentAt(slug string) string {
	return "---\nid: " + slug + "\ntitle: " + slug + "\nfields:\n  statement: prose\n---\n"
}

// attending is a workspace holding one artifact in each state the register
// reports, so that one run exercises every branch of the rule.
//
//	promise:in-force     ratified, and untouched since
//	promise:edited       ratified, and edited since — the exit 1 finding
//	promise:ungranted    never ratified — the finding an operator can settle
//	promise:withdrawn    retired, which is an act somebody performed
//	document:a-note      unratified, and nothing can ever ratify it
func attendingWorkspace(t *testing.T) *workspace {
	t.Helper()
	w := newWorkspace(t, map[string]string{
		"in-force":  "the first thing",
		"edited":    "the second thing",
		"ungranted": "the third thing",
		"withdrawn": "the fourth thing",
	})
	w.put("docs/_schema/document.yaml", documentSchema)
	w.put("docs/document/a-note.md", documentAt("a-note"))

	for _, slug := range []string{"in-force", "edited", "withdrawn"} {
		if code, _, stderr := w.run("ratify", "promise:"+slug, "--by", "the operator"); code != exitOK {
			t.Fatalf("ratify %s exited %d: %s", slug, code, stderr)
		}
	}
	if code, _, stderr := w.run("retire", "promise:withdrawn", "--by", "the operator"); code != exitOK {
		t.Fatalf("retire exited %d: %s", code, stderr)
	}
	w.put("docs/promise/edited.md", promiseAt("edited", "the second thing, reworded"))
	return w
}

// The default is the findings, and `--all` is the register.
//
// Both halves are asserted, because each without the other passes for the wrong
// reason: a default that printed nothing at all would satisfy the first, and a
// `--all` that ignored the flag would satisfy the second.
func TestTheDefaultViewIsWhatNeedsAttentionAndAllIsTheWholeRegister(t *testing.T) {
	w := attendingWorkspace(t)

	got := w.ids("status")
	want := []string{"promise:edited", "promise:ungranted"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("kept status listed %v, want %v", got, want)
	}

	all := w.ids("status", "--all")
	for _, id := range []string{
		"document:a-note", "promise:edited", "promise:in-force",
		"promise:ungranted", "promise:withdrawn",
	} {
		if !contains(all, id) {
			t.Errorf("kept status --all did not list %s: %v", id, all)
		}
	}
	if len(all) != 5 {
		t.Errorf("kept status --all listed %d artifacts, want 5: %v", len(all), all)
	}
}

// A ratified artifact and a retired one are the two that are deliberately not
// findings, and each is a judgement rather than an omission: the first is the
// resting state of an approved corpus, and the second is an act a person
// performed on purpose, after which nothing is owed.
func TestNeitherARatifiedArtifactNorARetiredOneNeedsAttention(t *testing.T) {
	w := attendingWorkspace(t)

	_, stdout, _ := w.run("status")
	for _, id := range []string{"promise:in-force", "promise:withdrawn"} {
		if strings.Contains(stdout, id) {
			t.Errorf("the default view listed %s, which needs nothing:\n%s", id, stdout)
		}
	}
	mustContain(t, "the default view", stdout, "2 of 5 artifacts need attention")
}

// An artifact nothing can ratify is counted rather than listed, and the count
// names its kind.
//
// Silence is the other answer and it is the wrong one: a reader who knows the
// corpus holds documents and sees no document anywhere goes looking for the
// bug. Listing it is also wrong, and for a stronger reason — it can never leave
// that state, so it would be a permanent entry in a list of things to do, which
// teaches the reader to stop reading the list.
func TestAnArtifactNothingCanRatifyIsCountedRatherThanListed(t *testing.T) {
	w := attendingWorkspace(t)
	w.put("docs/document/another.md", documentAt("another"))

	_, stdout, _ := w.run("status")
	if strings.Contains(stdout, "document:a-note") || strings.Contains(stdout, "document:another") {
		t.Errorf("the default view listed an artifact nothing can ratify:\n%s", stdout)
	}
	mustContain(t, "the default view", stdout,
		"2 documents are not listed", "nothing about a document binds", "they cannot be ratified")

	// One of them says it in the singular, because a line that reads "1
	// documents" is a line a reader stops trusting.
	if code, _, stderr := w.run("ratify", "document:another"); code == exitOK {
		t.Fatalf("kept ratify accepted an unbindable artifact, so this test's premise is gone: %s", stderr)
	}
	if err := os.Remove(filepath.Join(w.root, "docs", "document", "another.md")); err != nil {
		t.Fatal(err)
	}
	_, stdout, _ = w.run("status")
	mustContain(t, "the default view", stdout,
		"1 document is not listed", "nothing about a document binds", "it cannot be ratified")
}

// But an unbindable artifact somebody had already ratified **is** a finding,
// and this is the case the rule above would swallow.
//
// A grant in force against an artifact whose kind no longer binds means a
// schema's hash list was emptied underneath a ratification: it was de-ratified
// with nobody deciding to. That is exit 3, nothing else reports it, and it is
// the opposite of a permanent fixture — somebody can put the hash list back.
func TestAnUnbindableArtifactThatWasOnceRatifiedIsStillAFinding(t *testing.T) {
	w := newWorkspace(t, map[string]string{"in-force": "the first thing"})
	if code, _, stderr := w.run("ratify", "promise:in-force", "--by", "the operator"); code != exitOK {
		t.Fatalf("ratify exited %d: %s", code, stderr)
	}

	// The schema stops binding under an artifact that was granted against it.
	w.put("docs/_schema/promise.yaml", strings.Replace(schemaFile,
		"hash: [statement, gather, hold_when, trigger]", "hash: []", 1))

	code, stdout, _ := w.run("status")
	if code != exitUnknown {
		t.Errorf("a grant against a kind that stopped binding exited %d, want %d\n%s", code, exitUnknown, stdout)
	}
	if !strings.Contains(stdout, "promise:in-force") {
		t.Errorf("the default view swallowed a ratification that was undone by a schema edit:\n%s", stdout)
	}
	if strings.Contains(stdout, "is not listed") {
		t.Errorf("it was counted as something nothing can ratify:\n%s", stdout)
	}
}

// --state names states by hand, so it replaces the default rather than
// composing with it. Asking for a state is asking to see it, and the opposite
// choice would make `--state ratified` print nothing whatever the corpus held.
func TestAnExplicitStateReplacesTheAttentionDefault(t *testing.T) {
	w := attendingWorkspace(t)

	for _, tc := range []struct {
		state string
		want  []string
	}{
		{"ratified", []string{"promise:in-force"}},
		{"retired", []string{"promise:withdrawn"}},
		{"unratified", []string{"document:a-note", "promise:edited", "promise:ungranted"}},
	} {
		got := w.ids("status", "--state", tc.state)
		if strings.Join(got, " ") != strings.Join(tc.want, " ") {
			t.Errorf("kept status --state %s listed %v, want %v", tc.state, got, tc.want)
		}
	}
}

// --kind and --id name a population, and the default draws attention within it.
// A selector narrows what a reader would otherwise have seen, which is the only
// reading under which the three flags still compose.
func TestASelectorNarrowsThePopulationTheDefaultDrawsAttentionWithin(t *testing.T) {
	w := attendingWorkspace(t)

	if got := w.ids("status", "--kind", "promise"); strings.Join(got, " ") != "promise:edited promise:ungranted" {
		t.Errorf("kept status --kind promise listed %v", got)
	}
	if got := w.ids("status", "--id", "*edited"); strings.Join(got, " ") != "promise:edited" {
		t.Errorf("kept status --id *edited listed %v", got)
	}

	// A population with nothing in it needing attention is an answer, said in
	// words: an empty table reads as success and so does one that failed to
	// render.
	code, stdout, _ := w.run("status", "--kind", "document")
	if code != exitOK {
		t.Errorf("kept status --kind document exited %d, want %d", code, exitOK)
	}
	mustContain(t, "the answer", stdout, "1 artifact, and it needs no attention")

	// And the way to the rest carries the narrowing, so that the line can be
	// pasted rather than read and edited.
	mustContain(t, "the answer", stdout, "kept status --all --kind document lists them all")
}

// The exit code is what it was. The default view changing what a code means
// would make every script reading one wrong, silently, which is the one cost
// this change may not have.
func TestTheDefaultViewDoesNotMoveAnyExitCode(t *testing.T) {
	w := attendingWorkspace(t)

	// The register here holds an edited ratification, which is exit 1.
	for _, args := range [][]string{
		{"status"},
		{"status", "--all"},
		{"status", "--json"},
	} {
		if code, _, _ := w.run(args...); code != exitDrift {
			t.Errorf("kept %s exited %d, want %d", strings.Join(args, " "), code, exitDrift)
		}
	}

	// And the default and --all agree on the code under every narrowing, which
	// is what says the code follows the population rather than the rows.
	for _, narrowing := range [][]string{
		{"--kind", "promise"}, {"--kind", "document"},
		{"--id", "promise:in-force"}, {"--id", "promise:edited"},
	} {
		plain, _, _ := w.run(append([]string{"status"}, narrowing...)...)
		all, _, _ := w.run(append([]string{"status", "--all"}, narrowing...)...)
		if plain != all {
			t.Errorf("kept status %s exited %d and --all exited %d",
				strings.Join(narrowing, " "), plain, all)
		}
	}
}

// The document carries every artifact the filters admit, and says on each one
// whether the table would have printed it.
//
// --json is the only stable format, so a default that dropped records from it
// would break every consumer at once and without a word. The emphasis travels
// as a field instead, which is strictly more than the table has.
func TestTheDocumentCarriesEveryArtifactAndFlagsTheOnesNeedingAttention(t *testing.T) {
	w := attendingWorkspace(t)

	_, doc, _ := w.run("status", "--json")
	var out struct {
		Artifacts []struct {
			ID       string `json:"id"`
			Standing struct {
				Attention bool `json:"attention"`
			} `json:"standing"`
		} `json:"artifacts"`
	}
	if err := json.Unmarshal([]byte(doc), &out); err != nil {
		t.Fatalf("the document does not parse: %s\n%s", err, doc)
	}

	if len(out.Artifacts) != 5 {
		t.Errorf("--json carried %d artifacts, want the whole register of 5", len(out.Artifacts))
	}
	var flagged []string
	for _, a := range out.Artifacts {
		if a.Standing.Attention {
			flagged = append(flagged, a.ID)
		}
	}
	if strings.Join(flagged, " ") != "promise:edited promise:ungranted" {
		t.Errorf("--json flagged %v, want the two rows the table printed", flagged)
	}

	// The field is on every record rather than only on the true ones, so a
	// consumer can tell a build that answers this question from one that does
	// not.
	if n := strings.Count(doc, `"attention"`); n != 5 {
		t.Errorf("the document carried %d attention fields over 5 artifacts", n)
	}
}

// A corpus with nothing wrong in it still says something, and what it says is
// the answer rather than an empty table.
func TestACorpusWithNothingToActOnSaysSo(t *testing.T) {
	w := newWorkspace(t, map[string]string{"in-force": "the first thing"})
	if code, _, stderr := w.run("ratify", "promise:in-force", "--by", "the operator"); code != exitOK {
		t.Fatalf("ratify exited %d: %s", code, stderr)
	}

	code, stdout, _ := w.run("status")
	if code != exitOK {
		t.Errorf("a clean corpus exited %d, want %d\n%s", code, exitOK, stdout)
	}
	mustContain(t, "the answer", stdout, "1 artifact, and it needs no attention",
		"kept status --all lists every artifact")
	if strings.Contains(stdout, "promise:in-force") {
		t.Errorf("a clean corpus listed a row:\n%s", stdout)
	}
}

// `kept status --all` is what `kept status` was, byte for byte.
//
// The golden was captured from the build immediately before this flag existed,
// so passing it is a statement about a commit rather than about a fixture
// written afterwards — the same standard 0071 held the unfiltered rendering to,
// moved onto the flag that now carries that rendering.
//
// Refresh with `go test ./cmd/kept -run TestAllIsTheRenderingStatusHad -update`,
// and read the diff: this file moving means the whole listing changed, which is
// a decision rather than an accident.
func TestAllIsTheRenderingStatusHad(t *testing.T) {
	code, got, stderr := kept(t, "status", "--all", "--corpus", fixture)
	if stderr != "" {
		t.Fatalf("kept status --all wrote to stderr: %s", stderr)
	}
	if code != exitOK {
		t.Fatalf("kept status --all exited %d", code)
	}

	path := filepath.Join("testdata", "status-all.txt")
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("kept status --all is not the rendering status had:\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}
