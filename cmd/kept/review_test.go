package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/assess"
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/run"
	"github.com/pbleisch/kept/internal/thread"
)

// TestReviewWithNoThreadListsWhatCouldBeReviewed: with nothing named, review
// says what it could be pointed at rather than guessing one — the same shape
// `kept land` and `kept repair` take, and for the same reason.
func TestReviewWithNoThreadListsWhatCouldBeReviewed(t *testing.T) {
	w := newWorkspace(t, map[string]string{"first": "the first promise"})

	code, stdout, stderr := w.run("review")
	if code != exitOK {
		t.Fatalf("review exited %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, "nothing to review") {
		t.Fatalf("review says %q", stdout)
	}
	if !strings.Contains(stdout, "kept repair") {
		t.Fatalf("review does not say what produces something to review: %q", stdout)
	}
}

// TestReviewTakesOneThread. Several threads is a queue, and this verb has none.
func TestReviewTakesOneThread(t *testing.T) {
	w := newWorkspace(t, map[string]string{"first": "the first promise"})

	code, _, stderr := w.run("review", "one", "two")
	if code != exitUsage {
		t.Fatalf("review with two threads exited %d", code)
	}
	if !strings.Contains(stderr, "one thread") {
		t.Fatalf("review says %q", stderr)
	}
}

// TestReviewOfAThreadWithNoReceiptSaysSoRatherThanAnsweringAnEmptyChecklist.
//
// A checklist over a thread that does not exist would answer six rows of
// nothing, which reads exactly like a repair nobody found fault with. There is
// no act to attach a verification to, so it says that instead.
func TestReviewOfAThreadWithNoReceiptSaysSoRatherThanAnsweringAnEmptyChecklist(t *testing.T) {
	w := newWorkspace(t, map[string]string{"first": "the first promise"})

	code, stdout, stderr := w.run("review", "nosuchthread")
	if code != exitUsage {
		t.Fatalf("review of an absent thread exited %d: %s", code, stderr)
	}
	if !strings.Contains(stderr, "no receipt") {
		t.Fatalf("review says %q", stderr)
	}
	if strings.Contains(stdout, string(assess.PropertyExercised)) {
		t.Fatalf("review answered a checklist over a thread with no receipt:\n%s", stdout)
	}
}

// TestReviewAnswersEveryCheckAndSaysWhichNothingCouldWork.
//
// The row that is left out is the row nobody goes looking for, so a thread with
// almost no evidence still gets six answers — and each says, in its own words,
// what it needed and did not have. The exit code is 3 rather than 0 because
// nothing was worked: no evidence either way is not a clean checklist.
func TestReviewAnswersEveryCheckAndSaysWhichNothingCouldWork(t *testing.T) {
	w := newWorkspace(t, map[string]string{"first": "the first promise"})
	id := w.thread(t)

	code, stdout, stderr := w.run("review", id)
	if code != exitUnknown {
		t.Fatalf("review of a thread with nothing measured exited %d: %s", code, stderr)
	}
	for _, check := range assess.Checklist() {
		if !strings.Contains(stdout, string(check)) {
			t.Errorf("review answered no %s:\n%s", check, stdout)
		}
	}
	if !strings.Contains(stdout, "unread") && !strings.Contains(stdout, "??") {
		t.Errorf("nothing in the report says a check could not be worked:\n%s", stdout)
	}
}

// TestReviewJSONCarriesEveryCheckAndTheGrade holds the machine surface to the
// same rule the table follows: every check, always, with a three-valued outcome
// and never a boolean over them.
func TestReviewJSONCarriesEveryCheckAndTheGrade(t *testing.T) {
	w := newWorkspace(t, map[string]string{"first": "the first promise"})
	id := w.thread(t)

	_, stdout, stderr := w.run("review", id, "--json")
	var doc struct {
		Schema     string `json:"schema"`
		Verb       string `json:"verb"`
		Assessment *struct {
			Thread   string `json:"thread"`
			Findings []struct {
				Check   string `json:"check"`
				Outcome string `json:"outcome"`
			} `json:"findings"`
			Argument struct {
				Ran         bool   `json:"ran"`
				Transcripts int    `json:"transcripts"`
				Detail      string `json:"detail"`
			} `json:"argument"`
			Grade struct {
				Graded bool `json:"graded"`
			} `json:"grade"`
		} `json:"assessment"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("the document does not parse: %v\n%s\n%s", err, stdout, stderr)
	}
	if doc.Schema != Schema || doc.Verb != "review" {
		t.Fatalf("the document is %q/%q", doc.Schema, doc.Verb)
	}
	if doc.Assessment == nil {
		t.Fatal("the document carries no assessment")
	}
	if doc.Assessment.Thread != id {
		t.Errorf("the assessment names thread %q", doc.Assessment.Thread)
	}
	if len(doc.Assessment.Findings) != len(assess.Checklist()) {
		t.Fatalf("the document carries %d findings and the checklist has %d",
			len(doc.Assessment.Findings), len(assess.Checklist()))
	}
	for _, finding := range doc.Assessment.Findings {
		switch assess.Outcome(finding.Outcome) {
		case assess.Clear, assess.Fired, assess.Unread:
		default:
			t.Errorf("%s answered %q, which is not one of the three", finding.Check, finding.Outcome)
		}
	}
	// The other half is a section beside them and never a finding among them,
	// and it carries no findings field of its own: what an adversarial pass
	// says is unbounded, and a shape declared before the pass exists is the
	// first narrowing of it.
	if doc.Assessment.Argument.Ran {
		t.Error("the document says an adversarial pass ran, and this build has none")
	}
	if doc.Assessment.Argument.Detail == "" {
		t.Error("the document says nothing about why no pass has read this thread")
	}
	if strings.Contains(stdout, "\"check\": \"predicate-optimized\"") {
		t.Error("the reading half is emitted as a checklist entry, which gives it a ceiling of one question")
	}
}

// TestReviewRecordsNothing. It is a read verb, and a reader that writes cannot
// be run against a checkout nobody may write to.
func TestReviewRecordsNothing(t *testing.T) {
	w := newWorkspace(t, map[string]string{"first": "the first promise"})
	id := w.thread(t)

	before := w.streams(t)
	if _, _, stderr := w.run("review", id); strings.Contains(stderr, "panic") {
		t.Fatal(stderr)
	}
	if after := w.streams(t); after != before {
		t.Fatalf("review changed the ledger:\n%s\nto\n%s", before, after)
	}
}

// thread writes one thread receipt into this workspace's ledger and returns its
// id. It goes through the same [thread.Receipt] a dispatch uses, so a change to
// what a receipt may hold breaks this too.
func (w *workspace) thread(t *testing.T) string {
	t.Helper()
	store := ledger.ForWorkspace(w.root)
	l := ledger.Open(store, run.Start("test-review", nil))
	receipt, err := thread.NewReceipt(l, thread.MintID())
	if err != nil {
		t.Fatal(err)
	}
	if err := receipt.Started(); err != nil {
		t.Fatal(err)
	}
	code := 0
	if err := receipt.Exited(thread.Exit{Code: &code}); err != nil {
		t.Fatal(err)
	}
	if err := l.Close(run.Clean); err != nil {
		t.Fatal(err)
	}
	return string(receipt.ID())
}

// streams is the whole ledger as text, so a test can say that a verb wrote
// nothing rather than that it wrote nothing it happened to look at.
func (w *workspace) streams(t *testing.T) string {
	t.Helper()
	var out strings.Builder
	store := ledger.ForWorkspace(w.root)
	for _, name := range []string{ledger.Acts} {
		if src, err := store.Read(name); err == nil {
			out.WriteString(name + ":" + string(src) + "\n")
		}
	}
	return out.String()
}
