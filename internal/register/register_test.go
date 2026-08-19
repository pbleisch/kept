package register

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/canonical"
	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/run"
)

const promiseSchema = `type: promise
hash: [statement, gather, hold_when, trigger]
fields:
  statement: { type: string, required: true }
  gather:    { type: map }
  hold_when: { type: string }
  trigger:   { type: enum, values: [cadence, on-act, once] }
body: why
`

const promiseFile = `---
id: p
title: A promise
fields:
  statement: every package under internal/ has a test that runs and passes
  hold_when: obs.tests.failed == 0
  trigger: cadence
---

Tests that cannot fail are not evidence of correctness.
`

// workspace is a corpus and a ledger in one directory, which is what the
// register reads: two inputs, no third place for an answer to live.
type workspace struct {
	t     *testing.T
	root  string
	store ledger.Store
}

func newWorkspace(t *testing.T, files map[string]string) *workspace {
	t.Helper()
	root := t.TempDir()
	w := &workspace{t: t, root: root, store: ledger.NewFiles(filepath.Join(root, corpus.ConfigDir))}
	for name, body := range files {
		w.put(name, body)
	}
	return w
}

func promiseWorkspace(t *testing.T) *workspace {
	t.Helper()
	return newWorkspace(t, map[string]string{
		"_schema/promise.yaml": promiseSchema,
		"promise/p.md":         promiseFile,
	})
}

func (w *workspace) put(name, body string) {
	w.t.Helper()
	path := filepath.Join(w.root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		w.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		w.t.Fatal(err)
	}
}

func (w *workspace) remove(name string) {
	w.t.Helper()
	if err := os.Remove(filepath.Join(w.root, filepath.FromSlash(name))); err != nil {
		w.t.Fatal(err)
	}
}

// ratify performs the human act, exactly as the command does: hash what the
// artifact says now, and record that.
func (w *workspace) ratify(id string) canonical.Hash {
	w.t.Helper()
	h := w.hash(id)
	l := ledger.Open(w.store, run.Start("test", nil))
	if _, err := l.Ratify(id, h.String(), h.Norm, "paul", ""); err != nil {
		w.t.Fatal(err)
	}
	if err := l.Close(run.Clean); err != nil {
		w.t.Fatal(err)
	}
	return h
}

func (w *workspace) release(id string) {
	w.t.Helper()
	l := ledger.Open(w.store, run.Start("test", nil))
	if _, err := l.Release(id, "", 0, "paul", ""); err != nil {
		w.t.Fatal(err)
	}
	if err := l.Close(run.Clean); err != nil {
		w.t.Fatal(err)
	}
}

func (w *workspace) hash(id string) canonical.Hash {
	w.t.Helper()
	c := w.corpus()
	e, ok := c.Get(id)
	if !ok {
		w.t.Fatalf("no artifact %s", id)
	}
	kind, _ := corpus.SplitRef(id)
	sc, _ := c.Schemas.For(kind)
	h, err := canonical.Bind(e, sc)
	if err != nil {
		w.t.Fatal(err)
	}
	return h
}

func (w *workspace) corpus() *corpus.Corpus {
	w.t.Helper()
	c, err := corpus.Load(w.root, w.root, corpus.Options{})
	if err != nil {
		w.t.Fatalf("load: %v", err)
	}
	return c
}

// standingOf reads the register from scratch, the way a fresh process would.
func (w *workspace) standingOf(id string) Standing {
	w.t.Helper()
	for _, s := range w.register() {
		if s.Artifact == id {
			return s
		}
	}
	w.t.Fatalf("%s is not in the register", id)
	return Standing{}
}

func (w *workspace) register() []Standing {
	w.t.Helper()
	acts, quarantined, err := ledger.ReadActs(w.store)
	if err != nil {
		w.t.Fatal(err)
	}
	if len(quarantined) != 0 {
		w.t.Fatalf("the ledger holds %d unreadable lines: %#v", len(quarantined), quarantined)
	}
	return Derive(w.corpus(), acts)
}

// The first clause of increment 2's done-condition, and the whole design behind
// it: editing a ratified file makes it report unratified on the next read, with
// no invalidation step anywhere.
//
// The proof is in what does *not* happen between the two reads. Nothing runs.
// No hook fires, no flag is cleared, no cache is dropped, and the ledger's bytes
// are untouched. The file changes on disk and the answer changes with it,
// because the answer was never stored — it is a comparison between the hash the
// record names and the hash the content produces now.
func TestEditingARatifiedArtifactReportsUnratified(t *testing.T) {
	w := promiseWorkspace(t)
	granted := w.ratify("promise:p")

	if s := w.standingOf("promise:p"); s.State != Ratified {
		t.Fatalf("after ratifying, State = %q (%s)", s.State, s.Unhashable)
	}

	// One edit. Nothing else. In particular, no kept process runs.
	w.put("promise/p.md", strings.Replace(promiseFile,
		"obs.tests.failed == 0", "obs.tests.failed < 3", 1))

	s := w.standingOf("promise:p")
	if s.State != Unratified {
		t.Fatalf("after editing the predicate, State = %q, want %q", s.State, Unratified)
	}
	if s.Reason != Revised {
		t.Errorf("Reason = %q, want %q", s.Reason, Revised)
	}
	if !s.Granted.Hash.Equal(granted) {
		t.Errorf("the entry does not name the version that was granted: %v", s.Granted.Hash)
	}
	if s.Hash.Equal(granted) || s.Hash.Digest == "" {
		t.Errorf("the entry does not name what the file says now: %v", s.Hash)
	}
}

// The other half of that clause, and the reason it is a design constraint
// rather than a behaviour: deriving the register writes nothing. If it wrote
// anything at all, that thing would be a second place the answer lived and the
// two could disagree — which is the invalidation step this must not have.
func TestDerivingTheRegisterWritesNothing(t *testing.T) {
	w := promiseWorkspace(t)
	w.ratify("promise:p")

	before := snapshot(t, w.root)
	for range 3 {
		w.register()
	}
	// And after an edit, which is the moment something would be tempted to
	// invalidate.
	w.put("promise/p.md", strings.Replace(promiseFile, "cadence", "on-act", 1))
	edited := snapshot(t, w.root)
	for range 3 {
		w.register()
	}

	if after := snapshot(t, w.root); after != edited {
		t.Fatalf("reading the register changed the workspace:\nbefore:\n%s\nafter:\n%s", edited, after)
	}

	// The self-check: a snapshot that did not reach the ledger would pass the
	// comparison above no matter what Derive wrote.
	if !strings.Contains(before, ledger.Acts) || !strings.Contains(before, "\"kind\":\"ratified\"") {
		t.Fatal("the snapshot does not cover the ledger, so this proves nothing")
	}
	if before == edited {
		t.Fatal("the edit did not reach the snapshot, so this proves nothing")
	}
}

// Standing attaches to a content version, not to the last thing anyone did. The
// operator agreed to a wording; put that wording back and the grant applies
// again, because it was never withdrawn. Nothing has to re-ratify a revert.
func TestRevertingToARatifiedVersionRestoresStanding(t *testing.T) {
	w := promiseWorkspace(t)
	w.ratify("promise:p")

	revised := strings.Replace(promiseFile, "obs.tests.failed == 0", "obs.tests.failed < 3", 1)
	w.put("promise/p.md", revised)
	if s := w.standingOf("promise:p"); s.State != Unratified {
		t.Fatalf("after the revision, State = %q", s.State)
	}

	w.put("promise/p.md", promiseFile)
	if s := w.standingOf("promise:p"); s.State != Ratified {
		t.Fatalf("after reverting to a wording the operator ratified, State = %q, want %q", s.State, Ratified)
	}
}

// A release retires the artifact, not a wording — so it cannot be undone by
// editing the file into a shape somebody once approved. Ratifying again is a
// person's act and is the only way back.
func TestAReleaseIsNotUndoneByEditing(t *testing.T) {
	w := promiseWorkspace(t)
	w.ratify("promise:p")
	w.release("promise:p")

	if s := w.standingOf("promise:p"); s.State != Released {
		t.Fatalf("after release, State = %q, want %q", s.State, Released)
	}

	// Edit it away and edit it back. Neither is an act that creates standing.
	w.put("promise/p.md", strings.Replace(promiseFile, "cadence", "on-act", 1))
	if s := w.standingOf("promise:p"); s.State != Released {
		t.Errorf("editing a released promise made it %q", s.State)
	}
	w.put("promise/p.md", promiseFile)
	if s := w.standingOf("promise:p"); s.State != Released {
		t.Fatalf("restoring the wording brought a released promise back as %q", s.State)
	}

	w.ratify("promise:p")
	if s := w.standingOf("promise:p"); s.State != Ratified {
		t.Fatalf("ratifying after a release left it %q", s.State)
	}
}

// The guard on decision 0021. Absence cannot be told apart from a bad merge, a
// partial checkout or a corpus root pointed at the wrong place, so it keeps its
// standing and says the content is gone. If it released, one misconfiguration
// would silently retire an entire corpus.
func TestAMissingArtifactIsAbsentAndNeverReleased(t *testing.T) {
	w := promiseWorkspace(t)
	granted := w.ratify("promise:p")
	w.remove("promise/p.md")

	s := w.standingOf("promise:p")
	if s.State != Absent {
		t.Fatalf("a ratified artifact whose file is gone reports %q, want %q", s.State, Absent)
	}
	if s.State == Released {
		t.Fatal("deletion was read as a release, which makes deleting a file a way to retire a promise")
	}
	if !s.Granted.Hash.Equal(granted) {
		t.Errorf("the absent entry does not name the version that was ratified: %v", s.Granted.Hash)
	}

	// And it comes back when the content does, with nothing to reconcile.
	w.put("promise/p.md", promiseFile)
	if s := w.standingOf("promise:p"); s.State != Ratified {
		t.Fatalf("restoring the file left it %q, want %q", s.State, Ratified)
	}
}

// Renaming a file changes nothing, because identity is authored. Increment 1
// made that structural in the loader; it has to survive into the register,
// where the durable records are keyed.
func TestRenamingTheFileDoesNotTouchStanding(t *testing.T) {
	w := promiseWorkspace(t)
	w.ratify("promise:p")

	w.remove("promise/p.md")
	w.put("promise/renamed-entirely.md", promiseFile)

	if s := w.standingOf("promise:p"); s.State != Ratified {
		t.Fatalf("after renaming the file, State = %q, want %q", s.State, Ratified)
	}
}

// An artifact nobody ever ratified and one that was ratified and edited are
// both unratified, and a person needs to tell them apart: one is work not yet
// done, the other is standing that has just evaporated.
func TestTheRegisterSaysWhichKindOfUnratified(t *testing.T) {
	w := newWorkspace(t, map[string]string{
		"_schema/promise.yaml": promiseSchema,
		"_schema/desire.yaml":  "type: desire\nhash: []\nfields:\n  want: { type: string }\n",
		"promise/p.md":         promiseFile,
		"promise/q.md":         strings.Replace(promiseFile, "id: p", "id: q", 1),
		"desire/d.md":          "---\nid: d\nfields:\n  want: something\n---\n\nnote",
	})
	w.ratify("promise:p")
	w.put("promise/p.md", strings.Replace(promiseFile, "cadence", "on-act", 1))

	if s := w.standingOf("promise:p"); s.Reason != Revised {
		t.Errorf("an edited ratification reports Reason %q, want %q", s.Reason, Revised)
	}
	if s := w.standingOf("promise:q"); s.Reason != Never {
		t.Errorf("a promise nobody ratified reports Reason %q, want %q", s.Reason, Never)
	}
	// A desire cannot be ratified in place at all, and saying "never ratified"
	// would read as work outstanding rather than a kind that does not bind.
	s := w.standingOf("desire:d")
	if s.Reason != Unbinding {
		t.Errorf("a desire reports Reason %q, want %q", s.Reason, Unbinding)
	}
	if s.Unhashable == "" || strings.Contains(s.Unhashable, "desire:d") {
		t.Errorf("the reason should say why without repeating the artifact: %q", s.Unhashable)
	}
}

// The guard on the rules version. Decision 0022 records norm on every
// ratification so that refining the rules re-hashes old records under the
// version they were written with rather than de-ratifying the corpus at once.
// That re-hashing is not built — there has only ever been one version — and
// what must not happen in the meantime is the tool reporting a refinement as an
// edit, sending whoever reads it looking for a change nobody made.
func TestAGrantUnderOtherRulesIsNotReportedAsAnEdit(t *testing.T) {
	w := promiseWorkspace(t)
	h := w.hash("promise:p")

	// The same digest, recorded as having been computed under rules this build
	// does not use. Nothing about the artifact differs.
	l := ledger.Open(w.store, run.Start("test", nil))
	if _, err := l.Ratify("promise:p", h.String(), canonical.Version+1, "paul", ""); err != nil {
		t.Fatal(err)
	}
	if err := l.Close(run.Clean); err != nil {
		t.Fatal(err)
	}

	s := w.standingOf("promise:p")
	if s.State != Unratified {
		t.Fatalf("State = %q; a grant under rules this build cannot compute is not in force", s.State)
	}
	if s.Reason != Unrehashed {
		t.Fatalf("Reason = %q, want %q — a digest that agrees under different rules answered a different question, and calling it an edit sends a reader after a change nobody made",
			s.Reason, Unrehashed)
	}

	// And the ordinary case still reports as an edit, or the distinction says
	// nothing.
	w.ratify("promise:p")
	w.put("promise/p.md", strings.Replace(promiseFile, "cadence", "on-act", 1))
	if s := w.standingOf("promise:p"); s.Reason != Revised {
		t.Fatalf("an actual edit reports Reason %q, want %q", s.Reason, Revised)
	}
}

// The register is over the corpus, not over one artifact. One at a time is not
// a workflow, and the question — which of the things I approved have changed —
// only means something across a whole corpus.
func TestTheRegisterCoversTheWholeCorpusAtOnce(t *testing.T) {
	files := map[string]string{"_schema/promise.yaml": promiseSchema}
	for _, slug := range []string{"a", "b", "c", "d"} {
		files["promise/"+slug+".md"] = strings.Replace(promiseFile, "id: p", "id: "+slug, 1)
	}
	w := newWorkspace(t, files)

	w.ratify("promise:a")
	w.ratify("promise:b")
	w.ratify("promise:c")
	w.put("promise/b.md", strings.Replace(files["promise/b.md"], "cadence", "on-act", 1))
	w.release("promise:c")

	got := map[string]State{}
	for _, s := range w.register() {
		got[s.Artifact] = s.State
	}
	want := map[string]State{
		"promise:a": Ratified,
		"promise:b": Unratified,
		"promise:c": Released,
		"promise:d": Unratified,
	}
	for id, state := range want {
		if got[id] != state {
			t.Errorf("%s = %q, want %q", id, got[id], state)
		}
	}

	counts := Count(w.register())
	if counts[Ratified] != 1 || counts[Unratified] != 2 || counts[Released] != 1 {
		t.Errorf("counts = %v", counts)
	}
}

// snapshot renders every file under a directory with its content, so that "this
// read wrote nothing" is a claim about the whole workspace rather than about the
// files a test remembered to check.
func snapshot(t *testing.T, root string) string {
	t.Helper()
	var lines []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		lines = append(lines, filepath.ToSlash(rel)+"\n"+string(src))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n---\n")
}
