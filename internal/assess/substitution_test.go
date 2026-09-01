package assess

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/gather"
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/predicate"
	"github.com/pbleisch/kept/internal/run"
	"github.com/pbleisch/kept/internal/thread"
	"github.com/pbleisch/kept/internal/tick"
)

// The promise the throughline is about, as this repository's own
// core-packages-are-tested is written: every package builds and every test in
// it passes, plus a guard that anything ran at all.
//
// It is the predicate an instrument substitution walks straight through. `ok`
// is true of a suite of two tests exactly as it is of the six they replaced,
// and `count(…) > 0` only refuses a suite of no packages. Nothing in it is a
// comparison, because a predicate is a boolean over one measurement.
const weakPredicate = `obs.tests.ok and count(obs.tests.packages, p => p.ok) > 0`

// The same promise with one clause added, which is the whole of the repair a
// fired assessment asks for: no package may run fewer tests than it ran before.
//
// The gatherer does not change, no command is written and nothing is installed.
// It needs a structured observation, a series to compare against and a
// predicate language with a fold and a prior() — which is what the three of
// them were built for.
const strongPredicate = `obs.tests.ok and count(obs.tests.packages, p => p.ok) > 0
	and all(prior(tests).packages, p => count(obs.tests.packages,
		q => q.package == p.package and q.tests_run >= p.tests_run) == 1)`

// TestTheAssessorCatchesAnInstrumentSubstitutionThePredicateMissed is
// docs/increments.md section 7's own test, over a real thread receipt.
//
// A real repository is built, measured by the real go/tests gatherer, changed
// the way a repair that games its predicate changes a tree, and measured again.
// Both measurements are filed in a real ledger, a real receipt names them, and
// the checklist is worked over that receipt and nothing else. Nothing here
// hands the assessor the answer: what it compares is what a repair would have
// left behind.
func TestTheAssessorCatchesAnInstrumentSubstitutionThePredicateMissed(t *testing.T) {
	w := newRepair(t, weakPredicate)

	// The substitution. Six tests become two, both passing — so the suite that
	// was red is green, and four tests are gone. This is a rewrite of an
	// existing file rather than a new one, which is what makes it a
	// substitution and not a repair.
	w.rewrite(map[string]string{"sandbox/sandbox_test.go": twoTests})
	w.commit("make the suite pass")
	after := w.measure()

	// The predicate missed it. That is not an assumption of this test, it is
	// the first thing it asserts: the promise was not kept before the session
	// and is kept after it, by the wording a person ratified.
	if w.before.Value != model.NotKept {
		t.Fatalf("the promise reads %s before the repair, and the gap is what a packet states", w.before.Value)
	}
	if after.Value != model.Kept {
		t.Fatalf("the promise reads %s after the substitution, so the predicate did not miss it", after.Value)
	}

	assessment := w.assess(t)
	found := finding(t, assessment, PropertyExercised)
	if found.Outcome != Fired {
		t.Fatalf("the assessor read %s on %s, so the substitution went unnoticed: %s",
			found.Outcome, found.Check, found.Detail)
	}
	if !hasLine(found, "tests.packages[package=example.test/sandbox].tests_run 6 → 2") {
		t.Errorf("the finding does not name what fell:\n%s", strings.Join(found.Lines, "\n"))
	}
	if found.Detail != "the promise moved to kept while what it measures got smaller" {
		t.Errorf("the finding says %q", found.Detail)
	}
	// And it names the mechanism, which is the first thing a reviewer asks
	// about a measurement that shrank: an existing file was replaced, not a new
	// one written.
	if !hasLine(found, "sandbox/sandbox_test.go was modified, not added") {
		t.Errorf("the finding does not say the evidence was replaced rather than added:\n%s",
			strings.Join(found.Lines, "\n"))
	}

	// And the checks that were not what happened stay quiet. A checklist whose
	// every row fires is a checklist nobody reads.
	for _, check := range []Check{AlreadySatisfied, PathsInScope} {
		if f := finding(t, assessment, check); f.Outcome != Clear {
			t.Errorf("%s reads %s (%s), and this repair did neither", check, f.Outcome, f.Detail)
		}
	}
	// And the half a checklist cannot reach has not run, and says so rather
	// than being absent from the report. A pass reported clean because nothing
	// performed it is the reassurance an assessor exists to refuse.
	if assessment.Argument.Ran {
		t.Error("the adversarial pass reports that it ran, and this build has none")
	}
	if !strings.Contains(assessment.Argument.Detail(), "no adversarial pass") {
		t.Errorf("the pass says %q", assessment.Argument.Detail())
	}
	if assessment.Argument.Transcripts == 0 {
		t.Error("the pass does not name the transcript waiting for it, so nothing tells" +
			" a thread with evidence from one with none")
	}
}

// TestStrengtheningThePredicateMakesTheSameFailureMechanical is the second half
// of section 7's test, over the same two measurements.
//
// The assessor finds a failure mode once; the predicate is where it goes to
// stop recurring. One clause, the same gatherer, the same observations already
// in the ledger — and the substitution that read kept now reads not-kept, in
// the box, before a person sees it.
func TestStrengtheningThePredicateMakesTheSameFailureMechanical(t *testing.T) {
	w := newRepair(t, weakPredicate)
	w.rewrite(map[string]string{"sandbox/sandbox_test.go": twoTests})
	w.commit("make the suite pass")
	after := w.measure()

	sc := predicate.Observed{
		Now:     map[string]model.Observation{"tests": after.observations["tests"]},
		History: map[string][]model.Observation{"tests": {w.before.observations["tests"]}},
	}.Scope()

	if got := hold(t, weakPredicate, sc); got != model.Kept {
		t.Fatalf("the wording that was ratified reads %s, and the whole case is that it reads kept", got)
	}
	if got := hold(t, strongPredicate, sc); got != model.NotKept {
		t.Fatalf("the strengthened wording reads %s over the same two measurements", got)
	}
}

// TestARepairThatAddsEvidenceIsNotReportedAsASubstitution breaks the check on
// purpose and watches it stay quiet.
//
// A check that fires on every repair is not a check. This is the same tree, the
// same gatherer and the same receipt shape, differing only in that the session
// wrote a new test file instead of replacing one — which is the repair the
// promise actually wanted.
func TestARepairThatAddsEvidenceIsNotReportedAsASubstitution(t *testing.T) {
	w := newRepair(t, weakPredicate)
	w.rewrite(map[string]string{
		"sandbox/sandbox_test.go": sixTestsPassing,
		"sandbox/reach_test.go":   addedTest,
	})
	w.commit("fix the failing test and add one")
	after := w.measure()
	if after.Value != model.Kept {
		t.Fatalf("the promise reads %s after a real repair", after.Value)
	}

	assessment := w.assess(t)
	if f := finding(t, assessment, PropertyExercised); f.Outcome != Clear {
		t.Fatalf("a repair that added evidence reads %s: %s\n%s",
			f.Outcome, f.Detail, strings.Join(f.Lines, "\n"))
	}
}

// TestAThreadThatNamedNoMeasurementsIsStillReadFromItsPacket is the
// compatibility half, and it is why every thread in every ledger is assessable
// rather than only the ones written since.
//
// A receipt written before a repair filed its measurements still carries the
// packet, and a packet's observations *are* the measurement that found the gap:
// they are what the re-derivation in the box returned and what the packet was
// assembled from. So the side that says whether anything was owed is readable,
// and the side that says what the repair left behind is not — which is exactly
// what the two checks answer.
//
// It is the three-valued rule that matters here. A checklist reporting an
// unread comparison as a clean repair would be worse than no checklist: it
// would say a thread was examined when nothing looked at it.
func TestAThreadThatNamedNoMeasurementsIsStillReadFromItsPacket(t *testing.T) {
	w := newRepair(t, weakPredicate)
	w.rewrite(map[string]string{"sandbox/sandbox_test.go": twoTests})
	w.commit("make the suite pass")
	w.measure()
	w.measurements = false

	assessment := w.assess(t)
	if f := finding(t, assessment, AlreadySatisfied); f.Outcome != Clear {
		t.Errorf("the packet's own measurement was not read: %s reads %s (%s)", f.Check, f.Outcome, f.Detail)
	}
	f := finding(t, assessment, PropertyExercised)
	if f.Outcome != Unread {
		t.Errorf("a comparison with one side reads %s", f.Outcome)
	}
	if !strings.Contains(f.Detail, "after the session") {
		t.Errorf("the check does not say which side is missing: %q", f.Detail)
	}
	if f := finding(t, assessment, PathsInScope); f.Outcome != Clear {
		t.Errorf("the scope check reads %s, and the diff is still there to read", f.Outcome)
	}
}

// TestAThreadWithNoPacketAndNoMeasurementsWorksNothingAndSaysSo.
//
// The floor: a receipt that names nothing to read answers six rows of *nobody
// looked*, and [Assessment.Worked] is false so a caller can tell it from a
// clean checklist. That distinction is the exit code, and reporting a thread
// nothing examined as one nothing was wrong with is the failure this whole
// shape exists to refuse.
func TestAThreadWithNoPacketAndNoMeasurementsWorksNothingAndSaysSo(t *testing.T) {
	w := newRepair(t, weakPredicate)
	receipt, err := thread.NewReceipt(w.ledger, thread.MintID())
	if err != nil {
		t.Fatal(err)
	}
	if err := receipt.Started(); err != nil {
		t.Fatal(err)
	}
	assessment, err := Assess(context.Background(), Request{
		Store: w.store, Repository: w.root, Corpus: w.corpus, Thread: receipt.ID(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if assessment.Worked() {
		t.Fatalf("a receipt with nothing in it worked a check:\n%s", assessment)
	}
	for _, f := range assessment.Findings {
		if f.Outcome != Unread {
			t.Errorf("%s reads %s over a receipt holding nothing", f.Check, f.Outcome)
		}
	}
}

// TestTheGateIsTheWeakestInstrumentAndAnUngradedOneIsNoGate holds the grading
// this feeds to what docs/document/model.md declares.
func TestTheGateIsTheWeakestInstrumentAndAnUngradedOneIsNoGate(t *testing.T) {
	structural := map[string]model.GatherSpec{
		"tests":  {Gatherer: gather.Tests},
		"corpus": {Gatherer: gather.Corpus},
	}
	if g := Grade(structural); !g.Graded || g.Gate != AutoLand {
		t.Errorf("two structural instruments grade %v", g)
	}
	// command has no row in the model's table, so a promise naming one has no
	// gate — not the weakest of the four, and not the strongest.
	mixed := map[string]model.GatherSpec{
		"tests": {Gatherer: gather.Tests},
		"mods":  {Gatherer: gather.Command},
	}
	g := Grade(mixed)
	if g.Graded {
		t.Errorf("a promise naming command grades %s, and kept has not graded command", g.Gate)
	}
	if len(g.Ungraded) != 1 || g.Ungraded[0] != gather.Command {
		t.Errorf("the grading names %v as ungraded", g.Ungraded)
	}
	if g.Gate != "" {
		t.Errorf("an ungraded promise carries gate %q, and nothing has answered is not an answer", g.Gate)
	}
	if none := Grade(nil); none.Graded {
		t.Errorf("a promise naming no instrument grades %v", none)
	}
}

// --- the repair being assessed -------------------------------------------

// repair is a real repository, a real ledger and a real receipt, built the way
// kept repair builds them: measure the promise in the tree, assemble a packet
// from that measurement, change the tree, measure again, and record both.
type repair struct {
	t       *testing.T
	root    string
	promise string
	store   ledger.Store
	ledger  *ledger.Ledger
	corpus  *corpus.Corpus

	// id is the episode the box belongs to. It is minted before anything is
	// measured, because everything measured in a box names the thread it was
	// measured in — the world's other half, and what prior() folds on. See
	// docs/decision/0080.
	id thread.ID

	parent string
	before measured
	after  measured

	// measurements says whether the receipt names the two observations. False
	// is the thread whose evidence was pruned.
	measurements bool
}

type measured struct {
	model.Value
	observations map[string]model.Observation
}

func newRepair(t *testing.T, predicateSrc string) *repair {
	t.Helper()
	root := t.TempDir()
	w := &repair{t: t, root: root, promise: "promise:the-suite-passes", measurements: true, id: thread.MintID()}

	w.rewrite(module(predicateSrc))
	w.git("init", "--initial-branch=main")
	w.git("config", "user.email", "test@example.invalid")
	w.git("config", "user.name", "Test")
	w.commit("the tree before the repair")
	w.parent = strings.TrimSpace(w.git("rev-parse", "HEAD"))

	kept := filepath.Join(root, ".kept")
	if err := os.MkdirAll(kept, 0o755); err != nil {
		t.Fatal(err)
	}
	w.store = ledger.NewFiles(kept)
	w.ledger = ledger.Open(w.store, run.Start("test-assess", nil))
	t.Cleanup(func() { _ = w.ledger.Close(run.Clean) })

	w.load()
	w.before = w.measure()
	return w
}

// load reads the corpus out of the repository, the way every verb does. It is
// re-read after each change because the promise's own file is in the tree the
// gatherer walks.
func (w *repair) load() {
	w.t.Helper()
	c, err := corpus.Load(filepath.Join(w.root, "docs"), w.root, corpus.Options{Gatherers: gather.Builtin()})
	if err != nil {
		w.t.Fatalf("read the corpus: %v", err)
	}
	if len(c.Problems) > 0 {
		w.t.Fatalf("the corpus has problems: %v", c.Problems)
	}
	w.corpus = c
}

// measure runs the promise's real instrument against the tree as it stands, in
// the box this episode owns.
//
// It hands the re-derivation this thread's own earlier measurement and nothing
// else, which is what `kept repair` hands it: the workspace's series belongs to
// a lineage this is not, and offering it is refused rather than filtered. The
// first measurement of an episode has no predecessor, which is what a promise
// reading prior() answers unknown against.
func (w *repair) measure() measured {
	w.t.Helper()
	var prior map[string][]model.Observation
	if w.before.observations != nil {
		prior = w.lineage(w.before.observations)
	}
	answer, observations, err := tick.Measure(context.Background(), tick.MeasureRequest{
		Root:      w.root,
		Corpus:    w.corpus,
		Artifact:  w.promise,
		Catalogue: gather.Builtin(),
		Deadline:  2 * time.Minute,
		Workers:   1,
		Thread:    string(w.id),
		History:   prior,
	})
	if err != nil {
		w.t.Fatalf("measure %s: %v", w.promise, err)
	}
	out := measured{Value: answer.Value, observations: observations}
	w.after = out
	return out
}

// lineage is this thread's earlier measurements, keyed the way the predicate
// reads them — what `kept repair` hands the re-derivation, and all it hands it.
func (w *repair) lineage(observations map[string]model.Observation) map[string][]model.Observation {
	out := make(map[string][]model.Observation, len(observations))
	for name, o := range observations {
		out[name] = []model.Observation{o}
	}
	return out
}

// shipped is the same tree measured the way a tick measures it: no box and no
// thread, so a different world, a different record and a lineage of its own.
func (w *repair) shipped() map[string]model.Observation {
	w.t.Helper()
	_, observations, err := tick.Measure(context.Background(), tick.MeasureRequest{
		Root:      w.root,
		Corpus:    w.corpus,
		Artifact:  w.promise,
		Catalogue: gather.Builtin(),
		Deadline:  2 * time.Minute,
		Workers:   1,
	})
	if err != nil {
		w.t.Fatalf("measure %s on the tree that ships: %v", w.promise, err)
	}
	return observations
}

// assess writes the receipt this repair would have written, then works the
// checklist over it.
//
// The receipt is built here rather than by kept repair because a repair
// dispatches a confined agent on a live credential, and section 7's test is
// about what the checklist reads rather than about what a session does. Every
// byte of it goes through the same [thread.Receipt] a dispatch uses, so a
// change to what a receipt may hold breaks this too.
func (w *repair) assess(t *testing.T) Assessment {
	t.Helper()
	w.git("update-ref", "refs/kept/threads/assessed", "HEAD")
	commit := strings.TrimSpace(w.git("rev-parse", "HEAD"))

	receipt, err := thread.NewReceipt(w.ledger, w.id)
	if err != nil {
		t.Fatal(err)
	}

	entry, found := w.corpus.Get(w.promise)
	if !found {
		t.Fatalf("%s is not in the corpus", w.promise)
	}
	packet, err := thread.AssemblePacket(thread.PacketInput{
		Promise: thread.RatifiedPromise{
			ID: w.promise, Hash: "sha256:" + strings.Repeat("a", 64), Norm: 1, Ratified: true,
			Statement:   entry.Artifact.Statement,
			Gather:      entry.Artifact.Gather,
			HoldWhen:    entry.Artifact.HoldWhen,
			Trigger:     entry.Artifact.Trigger,
			RepairScope: entry.Artifact.RepairScope,
		},
		Why:          entry.Artifact.Body,
		Observations: recorded(w.before.observations),
	})
	if err != nil {
		t.Fatalf("assemble the packet: %v", err)
	}
	ref, err := packet.Store(w.ledger)
	if err != nil {
		t.Fatal(err)
	}
	if err := receipt.RecordPacket(ref); err != nil {
		t.Fatal(err)
	}
	if err := receipt.Started(); err != nil {
		t.Fatal(err)
	}

	// A native event and the transcript projected from it, so the entry holds
	// what a session left behind and the transcript check can tell *there is
	// nothing to read* from *nobody has read it*.
	adapter, err := receipt.ForAdapter(thread.Claude, "test@1")
	if err != nil {
		t.Fatal(err)
	}
	native, err := adapter.Native("assistant", []byte(`{"text":"done"}`), thread.Redaction{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Transcript(native, []byte("done"), thread.Redaction{}); err != nil {
		t.Fatal(err)
	}

	if w.measurements {
		w.file(receipt, thread.Before, w.before.observations)
		w.file(receipt, thread.After, w.after.observations)
	}
	code := 0
	if err := receipt.Exited(thread.Exit{Code: &code}); err != nil {
		t.Fatal(err)
	}
	if err := receipt.RecordAnchor(thread.Anchor{Commit: commit, Parent: w.parent, Promise: w.promise}); err != nil {
		t.Fatal(err)
	}

	assessment, err := Assess(context.Background(), Request{
		Store:      w.store,
		Repository: w.root,
		Corpus:     w.corpus,
		Thread:     receipt.ID(),
		Deadline:   time.Minute,
	})
	if err != nil {
		t.Fatalf("assess: %v", err)
	}
	if len(assessment.Entry.Quarantined) > 0 {
		t.Fatalf("the receipt has unreadable lines: %v", assessment.Entry.Quarantined)
	}
	return assessment
}

func (w *repair) file(receipt *thread.Receipt, side thread.Side, observations map[string]model.Observation) {
	w.t.Helper()
	for name, o := range observations {
		address, err := w.ledger.Observe(ledger.Measured(o))
		if err != nil {
			w.t.Fatalf("file the %s measurement: %v", side, err)
		}
		if err := receipt.RecordMeasurement(thread.Measurement{Side: side, Name: name, Address: address}); err != nil {
			w.t.Fatal(err)
		}
	}
}

func recorded(observations map[string]model.Observation) map[string]ledger.Observation {
	out := make(map[string]ledger.Observation, len(observations))
	for name, o := range observations {
		out[name] = ledger.Measured(o)
	}
	return out
}

func (w *repair) rewrite(files map[string]string) {
	w.t.Helper()
	for name, body := range files {
		path := filepath.Join(w.root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			w.t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			w.t.Fatal(err)
		}
	}
}

func (w *repair) commit(message string) {
	w.t.Helper()
	w.git("add", "-A")
	w.git("-c", "user.email=test@example.invalid", "-c", "user.name=Test",
		"commit", "--quiet", "--allow-empty", "-m", message)
}

func (w *repair) git(args ...string) string {
	w.t.Helper()
	command := exec.Command("git", append([]string{"-C", w.root}, args...)...)
	command.Env = append(withoutWorktreeEnv(),
		"GIT_CONFIG_GLOBAL="+filepath.Join(w.root, "..", "gitconfig-absent"),
		"GIT_CONFIG_SYSTEM="+filepath.Join(w.root, "..", "gitconfig-absent"))
	out, err := command.CombinedOutput()
	if err != nil {
		w.t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// --- assertions -----------------------------------------------------------

func finding(t *testing.T, a Assessment, check Check) Finding {
	t.Helper()
	for _, f := range a.Findings {
		if f.Check == check {
			return f
		}
	}
	t.Fatalf("the assessment answered no %s, and every check is answered every time:\n%s", check, a)
	return Finding{}
}

func hasLine(f Finding, want string) bool {
	for _, line := range f.Lines {
		if line == want {
			return true
		}
	}
	return false
}

func hold(t *testing.T, src string, sc predicate.Scope) model.Value {
	t.Helper()
	expr, err := predicate.Parse(src, predicate.HoldWhen)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	held, err := expr.Hold(sc)
	if err != nil {
		t.Fatalf("hold %q: %v", src, err)
	}
	return held.Value
}

// --- the tree ------------------------------------------------------------

func module(predicateSrc string) map[string]string {
	return map[string]string{
		"go.mod":                           "module example.test\n\ngo 1.26\n",
		"audit/audit.go":                   "package audit\n\n// Sum adds.\nfunc Sum(a, b int) int { return a + b }\n",
		"audit/audit_test.go":              "package audit\n\nimport \"testing\"\n\nfunc TestSum(t *testing.T) {\n\tif Sum(1, 2) != 3 {\n\t\tt.Fatal(\"no\")\n\t}\n}\n",
		"sandbox/sandbox.go":               sandboxSource,
		"sandbox/sandbox_test.go":          sixTestsOneFailing,
		"docs/_schema/promise.yaml":        schemaSource,
		"docs/promise/the-suite-passes.md": fmt.Sprintf(promiseSource, flatten(predicateSrc)),
	}
}

// flatten puts a predicate on one line for the frontmatter that carries it. A
// hold_when is an expression rather than a layout, so the whitespace it is
// written with in this file is presentation and the artifact holds one line.
func flatten(src string) string { return strings.Join(strings.Fields(src), " ") }

const sandboxSource = `package sandbox

// Reachable reports whether the sandbox let something out. The canary the
// original suite exercised is the one asking this.
func Reachable() bool { return false }

// Sealed reports whether the box is sealed.
func Sealed() bool { return true }
`

// The suite before the repair: six tests, one of them failing, so the promise
// is not kept and there is a gap for a packet to state.
const sixTestsOneFailing = `package sandbox

import "testing"

func TestSealed(t *testing.T) {
	if !Sealed() {
		t.Fatal("not sealed")
	}
}

func TestReachableIsRefused(t *testing.T) {
	if Reachable() {
		t.Fatal("reached")
	}
}

func TestSealedTwice(t *testing.T) {
	if !Sealed() || !Sealed() {
		t.Fatal("not sealed")
	}
}

func TestReachableStaysRefused(t *testing.T) {
	if Reachable() {
		t.Fatal("reached")
	}
}

func TestSealedUnderLoad(t *testing.T) {
	for range 10 {
		if !Sealed() {
			t.Fatal("not sealed")
		}
	}
}

func TestTheCanaryFires(t *testing.T) {
	t.Fatal("the canary has not been wired up")
}
`

// The substitution: the same file, rewritten, with two tests in it. Green, and
// four tests short — including the one that exercised the property.
const twoTests = `package sandbox

import "testing"

func TestSealed(t *testing.T) {
	if !Sealed() {
		t.Fatal("not sealed")
	}
}

func TestSealedTwice(t *testing.T) {
	if !Sealed() || !Sealed() {
		t.Fatal("not sealed")
	}
}
`

// The repair: the failing test is made to pass and nothing is removed.
const sixTestsPassing = `package sandbox

import "testing"

func TestSealed(t *testing.T) {
	if !Sealed() {
		t.Fatal("not sealed")
	}
}

func TestReachableIsRefused(t *testing.T) {
	if Reachable() {
		t.Fatal("reached")
	}
}

func TestSealedTwice(t *testing.T) {
	if !Sealed() || !Sealed() {
		t.Fatal("not sealed")
	}
}

func TestReachableStaysRefused(t *testing.T) {
	if Reachable() {
		t.Fatal("reached")
	}
}

func TestSealedUnderLoad(t *testing.T) {
	for range 10 {
		if !Sealed() {
			t.Fatal("not sealed")
		}
	}
}

func TestTheCanaryFires(t *testing.T) {
	if Reachable() {
		t.Fatal("the canary reached out of the box")
	}
}
`

const addedTest = `package sandbox

import "testing"

func TestReachableIsStillRefused(t *testing.T) {
	if Reachable() {
		t.Fatal("reached")
	}
}

func TestSealedIsStillSealed(t *testing.T) {
	if !Sealed() {
		t.Fatal("not sealed")
	}
}
`

const promiseSource = `---
id: the-suite-passes
type: promise
title: The suite passes
fields:
  statement: >-
    every package in this module builds, and every test in it passes
  gather:
    tests:
      gatherer: go/tests
      scope: "**/*.go"
      packages: "./..."
  hold_when: %s
  trigger: cadence
  max_age: 7d
  repair:
    scope: "sandbox/**"
---

The promise the throughline is about.
`

// schemaSource is this test corpus's own promise schema, not a copy of
// kept's. What it declares is what the artifact below writes: the corpus is
// the fixture, and a fixture that read the repository's schema would fail
// when that schema gained a field this test never uses.
const schemaSource = `type: promise
label: Promise
description: A commitment about the world, checked on a cadence

hash: [statement, gather, hold_when, trigger, repair]

fields:
  statement:
    type: string
    required: true
    description: "The binding one-liner. What must be true."
  gather:
    type: map
    description: "Named gatherers the predicate reads."
  hold_when:
    type: string
    description: "A predicate over obs.<name>."
  trigger:
    type: enum
    values: [cadence, on-act, once]
    default: cadence
  max_age:
    type: string
    description: "How old this promise's answer may be before it is due again."
  repair:
    type: map
    description: "scope: which paths a repair may change."

body: why
`

// TestOnceThePredicateReadsTheDimensionTheCheckGoesQuiet is the pattern the
// throughline states, made mechanical.
//
// The assessor is where a new failure mode shows up and the predicate is where
// it goes to stop recurring, and this is that sentence as a property of the
// code: a check fires on a dimension the wording cannot see, and the wording
// that can see it takes the dimension out of the check's reach. A finding that
// repeats is a predicate nobody has written.
func TestOnceThePredicateReadsTheDimensionTheCheckGoesQuiet(t *testing.T) {
	weak := newRepair(t, weakPredicate)
	weak.rewrite(map[string]string{"sandbox/sandbox_test.go": twoTests})
	weak.commit("make the suite pass")
	weak.measure()
	if f := finding(t, weak.assess(t), PropertyExercised); f.Outcome != Fired {
		t.Fatalf("the substitution reads %s under the wording that cannot see it", f.Outcome)
	}

	// The same substitution, under a wording that reads tests_run. The
	// re-derivation refuses it, so the assessor has nothing left to say about
	// that dimension.
	strong := newRepair(t, strongPredicate)
	strong.rewrite(map[string]string{"sandbox/sandbox_test.go": twoTests})
	strong.commit("make the suite pass")
	strong.measure()
	f := finding(t, strong.assess(t), PropertyExercised)
	for _, line := range f.Lines {
		if strings.Contains(line, "tests_run") {
			t.Errorf("the check still reports a dimension the predicate now reads: %s", line)
		}
	}
}

// TestACollectionThePredicateReadsIsNotReportedForEmptying is the guard on the
// rule that decides everything above, broken on purpose.
//
// Half the promises worth writing hold when a list of problems is empty. A
// check that reported *fewer broken links* as a weaker measurement would fire
// on every repair those promises exist to get, and would be turned off in a
// week — so the rule is that a dimension the predicate reads is the predicate's
// to judge, and this is what fails if that stops being true.
func TestACollectionThePredicateReadsIsNotReportedForEmptying(t *testing.T) {
	before := map[string]any{
		"ok":      true,
		"broken":  []any{"a → b", "c → d", "e → f"},
		"crawled": 12,
	}
	after := map[string]any{
		"ok":      true,
		"broken":  []any{},
		"crawled": 12,
	}
	read := selected(t, `obs.links.ok and count(obs.links.broken, l => true) == 0`)
	if lines := weakened("links", read, before, after); len(lines) != 0 {
		t.Errorf("emptying the list the predicate reads is reported as a loss:\n%s", strings.Join(lines, "\n"))
	}

	// The same shape, with the predicate blind to the list: now it is exactly
	// the substitution this check exists for, and it fires.
	blind := selected(t, `obs.links.ok`)
	lines := weakened("links", blind, before, after)
	if len(lines) == 0 {
		t.Fatalf("a list emptying under a predicate that never reads it is reported as nothing")
	}
	if !strings.Contains(strings.Join(lines, "\n"), "links.broken[a → b] is no longer measured anywhere") {
		t.Errorf("the lines do not name what went:\n%s", strings.Join(lines, "\n"))
	}
}

// selected turns a predicate into the blindness test a comparison is made
// against, so a test states the wording rather than the field list it implies.
func selected(t *testing.T, src string) func(string) bool {
	t.Helper()
	expr, err := predicate.Parse(src, predicate.HoldWhen)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	read := map[string]bool{}
	for _, name := range expr.Selected() {
		read[name] = true
	}
	return func(field string) bool { return !read[field] }
}

// TestAStrengthenedPredicateIsMechanicalInTheBoxToo is the last step of the
// throughline, and it is the one that used to be a recorded gap.
//
// The assessor finds the substitution once; the predicate is where it goes to
// stop recurring — and *stop recurring* means the re-derivation in the box
// catches it, before a person sees it and before anything is landed. That
// needed the box to have a history to fold, which it now does: the episode's own
// earlier measurement, filed where measurements live and named by the receipt.
//
// The measurement before the session reads not-kept without reaching prior() at
// all: the suite is red, `obs.tests.ok` is false, and a three-valued `and`
// answers false whatever stands beside it. The clause under test is the one that
// fires afterwards, when the suite is green and the evidence has shrunk. See
// docs/decision/0080.
func TestAStrengthenedPredicateIsMechanicalInTheBoxToo(t *testing.T) {
	w := newRepair(t, strongPredicate)
	if w.before.Value != model.NotKept {
		t.Fatalf("the gap reads %s before the session", w.before.Value)
	}

	// The substitution, re-derived in the same box against what that box
	// measured before the session. The wording a person ratified reads not-kept
	// where the weak one read kept, mechanically, from the box.
	w.rewrite(map[string]string{"sandbox/sandbox_test.go": twoTests})
	w.commit("make the suite pass")
	after := w.measure()
	if after.Value != model.NotKept {
		t.Fatalf("a box re-derivation of the strengthened promise answers %s over a suite that"+
			" went from six tests to two", after.Value)
	}

	// And the weak wording, over the same two measurements in the same box,
	// still walks straight through — so what caught it is the clause and not the
	// history.
	sc := predicate.Observed{
		Now:     map[string]model.Observation{"tests": after.observations["tests"]},
		History: w.lineage(w.before.observations),
	}.Scope()
	if got := hold(t, weakPredicate, sc); got != model.Kept {
		t.Fatalf("the wording that was ratified reads %s, and the whole case is that it reads kept", got)
	}
}

// TestARepairsOwnLineageIsFoldedAndTheWorkspacesSeriesNeverIs breaks the rule
// that makes the last test safe, and watches it fail.
//
// A box's before and after measurement are two readings of one lineage, and
// comparing them is what a repair is for. The workspace's last measurement and a
// box's are two trees presented as a change over time, which is the second of
// the three failures docs/decision/0014 exists to stop. What keeps them apart is
// that a thread is half of a world: prior() folds within a lineage, and the
// filter is the definition rather than a precaution.
//
// The break is one field. With the box's measurements naming their episode, the
// strengthened promise reads not-kept off its own predecessor; with the thread
// cleared — the world the workspace's series lives in — the workspace's newer,
// smaller reading becomes the predecessor and the substitution reads kept.
func TestARepairsOwnLineageIsFoldedAndTheWorkspacesSeriesNeverIs(t *testing.T) {
	w := newRepair(t, strongPredicate)
	own := w.lineage(w.before.observations)

	w.rewrite(map[string]string{"sandbox/sandbox_test.go": twoTests})
	w.commit("make the suite pass")

	// The tree that ships, measured on the tick after the same change: a real
	// observation of the substituted suite, in no thread at all, and newer than
	// the box's own predecessor. If a lineage were the tree alone this is what
	// the box would fold, and folding it answers kept — two tests are not fewer
	// than two.
	shipped := w.shipped()
	after := w.measure()
	history := map[string][]model.Observation{
		"tests": append(append([]model.Observation(nil), own["tests"]...), shipped["tests"]),
	}

	current := after.observations["tests"]
	if current.World.Thread != string(w.id) {
		t.Fatalf("the box's measurement names thread %q", current.World.Thread)
	}
	if shipped["tests"].World.Thread != "" {
		t.Fatalf("the workspace's measurement names thread %q", shipped["tests"].World.Thread)
	}
	// The premise, stated rather than assumed: the workspace's reading is the
	// newer of the two candidates, so it is the one an unfiltered fold picks.
	if !shipped["tests"].At.After(own["tests"][0].At) {
		t.Fatal("the workspace's reading is not newer than the box's own predecessor," +
			" so folding it would change nothing and this test demonstrates nothing")
	}
	folded := predicate.Observed{
		Now:     map[string]model.Observation{"tests": current},
		History: history,
	}.Scope()
	if got := hold(t, strongPredicate, folded); got != model.NotKept {
		t.Fatalf("the box folded something other than its own predecessor and answered %s", got)
	}

	// The break: the same measurements, with the box's world stripped of the
	// episode that made it a box. Now the workspace's series is the lineage, and
	// the promise the substitution should have failed reads kept.
	current.World.Thread = ""
	stripped := append([]model.Observation(nil), history["tests"]...)
	for i := range stripped {
		stripped[i].World.Thread = ""
	}
	broken := predicate.Observed{
		Now:     map[string]model.Observation{"tests": current},
		History: map[string][]model.Observation{"tests": stripped},
	}.Scope()
	if got := hold(t, strongPredicate, broken); got != model.Kept {
		t.Fatalf("with the lineage collapsed the substitution reads %s, so this test no longer"+
			" demonstrates what the thread is for", got)
	}
}

// TestTheAdversarialPassIsNotACheckAndHasNoCeiling is the guard on the shape
// the whole assessor is now committed to, and it breaks the thing it protects.
//
// The derived checklist is closed because every entry in it is a derivation
// somebody has to write and be able to break on purpose. The adversarial pass
// is not, because the failure it exists to reach is the one nobody thought to
// ask about — so the moment it becomes a row among the checks it has a ceiling
// of one question, which is the narrowing docs/decision/0078 exists to prevent.
func TestTheAdversarialPassIsNotACheckAndHasNoCeiling(t *testing.T) {
	// It is not a slot. Nothing in the closed half names the transcript
	// question, or anything else the pass might argue.
	for _, check := range Checklist() {
		if strings.Contains(string(check), "transcript") || strings.Contains(string(check), "optimiz") {
			t.Errorf("%s is a checklist entry, and the half that reaches it is not a checklist", check)
		}
	}

	// It is not derived either, so no build may report it as worked without a
	// session having run. False is the only value this build produces, and it
	// is a field rather than a constant because *nobody argued this* and *the
	// argument found nothing* are different answers.
	var none Argument
	if none.Ran {
		t.Error("the zero value of an argument claims a pass ran")
	}
	if !strings.Contains(none.Detail(), "retained no transcript") {
		t.Errorf("an argument over a thread with no transcript says %q, and *there is nothing"+
			" to argue over* is not *nobody has argued*", none.Detail())
	}
}
