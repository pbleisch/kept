//go:build !windows

package repair

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/assess"
	"github.com/pbleisch/kept/internal/box"
	"github.com/pbleisch/kept/internal/config"
	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/gather"
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/run"
	"github.com/pbleisch/kept/internal/thread"
	"github.com/pbleisch/kept/internal/tick"
)

// This file is decision 0052's live evidence for the *second* kind of confined
// session kept dispatches: the adversarial pass of decisions 0078 and 0081.
//
// It is a separate proof from the repair's beside it, and deliberately so. An
// admission is per adapter *and* per kind of work: the repair proof establishes
// that a session carrying a packet acts on it, and it says nothing about whether
// a session carrying a brief argues about one. The two ask opposite things of the
// same runtime — one says *close this gap*, the other says *attack this act* —
// and 0058's whole point is that the framing decides what comes back.
//
// WHAT IS MEASURED HERE, end to end and in one test:
//
//   - a real confined repair runs on the operator's own brokered credential and
//     leaves a real thread: a packet, native events, a transcript, a lifecycle;
//   - the tree it left is committed and anchored, and both box measurements are
//     filed, so what the pass reads is the ledger entry a person would read;
//   - a real confined *pass* runs in a box cut at that anchor, with the assessor's
//     own declared ask, its own runtime configuration and a profile that grants
//     nothing inside the box;
//   - the box is kept and inspected afterwards: a live session read the tree it
//     was judging and changed not one byte of it;
//   - and what it argued is in the receipt, verbatim, where `kept review` reads
//     it back.
//
// Two sessions run here, so this is the most expensive test in the tree. It is
// skipped with the other live ones by the GOFLAGS list in AGENTS.md.

// argumentDeadline bounds the pass. It is shorter than the repair's because the
// work is shorter: a pass reads a brief and writes an account, where a repair
// reads a tree, edits it and re-reads it. Nothing external runs without a
// deadline. See docs/decision/0026.
const argumentDeadline = 6 * time.Minute

// TestAConfinedAdversarialPassArguesOverARealRepairThread is the whole loop for
// the assessor's open half.
func TestAConfinedAdversarialPassArguesOverARealRepairThread(t *testing.T) {
	session := liveClaudeRepairSession(t)
	assessor := operatorAssess(t)
	// Under `go test` os.Executable() is this binary, and a canary launched
	// into it reaches the testing package's flag parser rather than a probe.
	// See keptExecutable: a canary that cannot start is a refusal, correctly,
	// so the dispatcher this test stands in for has to name a real kept.
	kept := buildKept(t)
	restore := keptExecutable
	keptExecutable = func() (string, error) { return kept, nil }
	t.Cleanup(func() { keptExecutable = restore })

	// One ledger for the whole episode, on a scratch store: what this records is
	// evidence about two sessions rather than acts in the operator's history.
	store := ledger.NewFiles(t.TempDir())
	writable := ledger.Open(store, run.Start("claude-argument", nil))
	receipt, err := thread.NewReceipt(writable, thread.MintID())
	if err != nil {
		t.Fatal(err)
	}

	// The gap, filed where measurements live, before anything is dispatched —
	// exactly as Perform files it. It is what the pass compares against.
	before, beforeObserved := measureInTheBox(t, session.box)
	fileMeasurements(t, writable, receipt, thread.Before, beforeObserved)

	// 1. A real confined repair. Its verdict is not what this test is about, and
	// it is logged rather than asserted: what matters here is that the thread it
	// leaves is a real one.
	ctx, cancel := context.WithTimeout(context.Background(), claudeRepairDeadline+2*time.Minute)
	defer cancel()
	if err := thread.Dispatch(ctx, session.runtime.Config, session.packet, session.launch,
		thread.Supervisor{Timeout: claudeRepairDeadline}, receipt); err != nil {
		if isUnadmitted(err) {
			t.Fatalf("the Claude adapter refuses as unadmitted: %v", err)
		}
		t.Fatalf("dispatch a confined Claude repair: %v", err)
	}
	after, afterObserved := measureInTheBox(t, session.box)
	fileMeasurements(t, writable, receipt, thread.After, afterObserved)
	t.Logf("the repair re-derived %s → %s", before.Value, after.Value)

	// 2. The anchor, which is what a pass argues about. A repair that changed
	// nothing anchors nothing and there would be no act here to attack.
	changed, err := session.box.Change(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(changed.Paths) == 0 {
		t.Fatalf("the confined repair changed nothing, so there is no act for a pass to argue about; see the repair proof's header:\n%s",
			claudeSessionSummaryOf(t, store, receipt.ID()))
	}
	anchorTheSession(t, session.box, receipt)
	if err := writable.Close(run.Clean); err != nil {
		t.Fatal(err)
	}

	// 3. The pass. It cuts its own read-only box at the anchor, assembles the
	// brief from the entry above, and dispatches with the assessor's own ask.
	repository := session.repository
	arguing := ledger.Open(store, run.Start("claude-argument-pass", nil))
	result, err := Argue(context.Background(), ArgueRequest{
		Workspace: repository,
		Thread:    receipt.ID(),
		Provider:  "nono",
		Assessor:  assessor,
		Deadlines: Deadlines{Agent: argumentDeadline, Git: box.DefaultDeadline, Gatherer: 5 * time.Minute},
		Store:     store,
		Ledger:    arguing,
		// Kept, because the box is the evidence for the half of this test that
		// is about what the pass could not do.
		Keep: true,
	})
	if err != nil {
		t.Fatalf("argue over thread %s: %v", receipt.ID(), err)
	}
	if err := arguing.Close(run.Clean); err != nil {
		t.Fatal(err)
	}
	if result.Box != "" {
		t.Cleanup(func() {
			_ = exec.Command("git", "-C", repository, "worktree", "remove", "--force", result.Box).Run()
		})
	}
	if result.Status != Argued {
		t.Fatalf("the pass reports %s: %s", result.Status, result.Detail)
	}

	// What it argued, in its own words, before anything is asserted about the
	// record. A pass that ran and said something unusable and one that found a
	// real gap look the same from a status.
	t.Logf("the box the pass could write in: %v", result.Unwritable)
	t.Logf("what the confined pass argued:\n%s", result.Said)

	// 4. It could not write. The canary proved the boundary before the session
	// started; this is the tree afterwards, which is the same claim measured
	// against what a live session actually did in it.
	if result.Box == "" {
		t.Fatal("the pass discarded its box, so there is nothing to inspect")
	}
	dirty := strings.TrimSpace(git(t, result.Box, "status", "--porcelain"))
	if dirty != "" {
		t.Fatalf("a read-only box holds changes after a live pass ran in it:\n%s", dirty)
	}
	if len(result.Unwritable) == 0 {
		t.Fatal("the pass declared no unwritable path, so its canary measured nothing about read-only-ness")
	}

	// 5. And the finding is durable. Not in this process, not in a log: in the
	// thread's own receipt, addressed by its bytes, where a reader who was not
	// here finds it.
	entries, quarantined, err := thread.ReadReceipt(store, receipt.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(quarantined) != 0 {
		t.Fatalf("the episode's receipt quarantined its own evidence: %#v", quarantined)
	}
	kinds := receiptKinds(entries)
	for _, want := range []string{"thread_packet", "thread_brief", "thread_argument", "thread_anchor", "thread_measurement"} {
		if kinds[want] == 0 {
			t.Fatalf("the episode's receipt has no %s entry: %v", want, kinds)
		}
	}
	if kinds["thread_argument"] != 1 {
		t.Fatalf("one pass wrote %d arguments", kinds["thread_argument"])
	}

	// The two sessions are told apart by their input records, which is
	// docs/decision/0079's rule worked over a receipt that really holds two.
	sessions := thread.Sessions(entries)
	if len(sessions) != 2 {
		t.Fatalf("an episode holding a repair and a pass grouped into %d sessions", len(sessions))
	}
	if sessions[0].Kind != ledger.ThreadPacket || sessions[1].Kind != ledger.ThreadBrief {
		t.Fatalf("the episode's sessions are %s then %s", sessions[0].Kind, sessions[1].Kind)
	}

	arguments := thread.Arguments(entries)
	if len(arguments) != 1 {
		t.Fatalf("the receipt holds %d arguments", len(arguments))
	}
	if arguments[0].Brief != result.Brief {
		t.Fatalf("the argument names brief %v and the pass was given %v", arguments[0].Brief, result.Brief)
	}
	said, err := ledger.ReadArtifact(store, arguments[0].Artifact)
	if err != nil {
		t.Fatal(err)
	}
	if string(said) != result.Said {
		t.Fatalf("what the receipt holds is not what the pass said:\n%s", said)
	}

	// 6. And `kept review` reads it back. This is the half that makes the
	// finding useful rather than merely stored: a person typing the reader's
	// verb sees the pass's own words beside the derived checklist, and the
	// checklist is exactly what it was.
	assessment, err := assess.Assess(context.Background(), assess.Request{
		Store: store, Repository: repository, Thread: receipt.ID(), Deadline: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !assessment.Argument.Ran {
		t.Fatalf("the reader reports no pass over a thread that was argued over: %s", assessment.Argument.Detail())
	}
	if assessment.Argument.Said != result.Said {
		t.Fatal("the reader's copy of the argument is not the one the pass wrote")
	}
	if len(assessment.Findings) != len(assess.Checklist()) {
		t.Fatalf("the derived checklist answered %d of %d questions after a pass ran",
			len(assessment.Findings), len(assess.Checklist()))
	}
	t.Logf("what a person reads:\n%s", assessment)
	// What this cost, per session, so the record says it rather than leaving it
	// to be re-measured. A confined session spends a credential and there is no
	// cheaper way to establish what one does.
	for i, session := range sessions {
		t.Logf("session %d (%s): %s", i+1, session.Kind, spent(session.Entries))
	}
}

// spent is what one session reported of itself: its cost, its turns, and the
// usage windows it saw.
//
// It is read from the receipt's own projections rather than timed here, because
// a duration measured by the parent is a fact about this machine and the
// runtime's own reading is the one docs/decision/0077 makes durable. A window
// that was not reported is *not a window at zero* and is said so.
func spent(entries []thread.Entry) string {
	var parts []string
	var turns uint64
	for _, entry := range entries {
		switch {
		case entry.Cost != nil:
			parts = append(parts, entry.Cost.Amount+" "+entry.Cost.Currency)
		case entry.Turn != nil && entry.Turn.Count > turns:
			turns = entry.Turn.Count
		}
	}
	if window, found := thread.LatestWindow(entries); found {
		for _, w := range window.Windows {
			parts = append(parts, w.Name+" at "+w.Utilization)
		}
	} else {
		parts = append(parts, "no usage window reported")
	}
	return fmt.Sprintf("%d turns, %s", turns, strings.Join(parts, ", "))
}

// TestAnAdversarialPassIsReportedRatherThanRefusedWhenNobodyConfiguredOne is the
// state every machine is in until an operator sets an assessor up, and it costs
// nothing to run.
//
// It matters because of what the alternative would be. A repair that failed
// because no pass could run would make the pass load-bearing for the repair,
// which is the coupling docs/decision/0079 is careful to avoid: the verdict is
// the re-derivation's, and an assessor that could not be started does not unmake
// it.
func TestAnAdversarialPassIsReportedRatherThanRefusedWhenNobodyConfiguredOne(t *testing.T) {
	result, err := Argue(context.Background(), ArgueRequest{Thread: thread.MintID()})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != Unconfigured {
		t.Fatalf("a machine with no assessor reports %s", result.Status)
	}
	if !strings.Contains(result.Detail, "no assessor runtime") {
		t.Fatalf("the report says %q", result.Detail)
	}

	// And a machine that has one still refuses to argue about a thread with no
	// act in it. There is nothing to attack, and a pass dispatched over one
	// would spend a credential on a tree nothing durably names.
	store := ledger.NewFiles(t.TempDir())
	writable := ledger.Open(store, run.Start("argue-unanchored", nil))
	receipt, err := thread.NewReceipt(writable, thread.MintID())
	if err != nil {
		t.Fatal(err)
	}
	if err := receipt.Started(); err != nil {
		t.Fatal(err)
	}
	if err := writable.Close(run.Clean); err != nil {
		t.Fatal(err)
	}
	result, err = Argue(context.Background(), ArgueRequest{
		Thread:    receipt.ID(),
		Provider:  "nono",
		Assessor:  config.Assess{Repair: config.Repair{Runtime: "claude", Executable: "/nowhere/claude"}},
		Deadlines: Deadlines{Agent: time.Minute},
		Store:     store,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != Unanchored {
		t.Fatalf("a thread that anchored nothing reports %s: %s", result.Status, result.Detail)
	}
}

// operatorAssess is the assessor's own per-machine configuration, read the way a
// dispatcher reads it.
//
// **It falls back to the repair block, and says so.** Decision 0078 selects the
// assessor's runtime, model and effort independently of the repairer's, and
// internal/config's own tests are what hold that separation. What this test is
// about is whether a confined pass runs at all, and a machine that has not yet
// written an assess block would otherwise skip it — which would leave the half
// this file exists to prove unmeasured for the ordinary reason that nobody had
// finished a configuration file.
func operatorAssess(t *testing.T) config.Assess {
	t.Helper()
	user, err := config.ReadUser()
	if err != nil {
		t.Fatal(err)
	}
	if user.Assess.Runtime != "" {
		if user.Assess.Runtime != string(thread.Claude) {
			t.Skipf("this machine's configured assessor runtime is %q, and this is the Claude adapter's evidence", user.Assess.Runtime)
		}
		return user.Assess
	}
	t.Log("this machine names no assess block, so the pass runs on the repair block's runtime and credential; " +
		"the separation of the two is internal/config's to prove and this is the pass's")
	return config.Assess{Repair: operatorRepair(t)}
}

// measureInTheBox re-derives the one promise this proof is about, with
// tick.Measure: one artifact, no ledger opened, nothing recorded — which is what
// a verdict reached in a box should do anyway. See docs/decision/0035.
func measureInTheBox(t *testing.T, created *box.Box) (tick.Answer, map[string]model.Observation) {
	t.Helper()
	c, err := corpus.Load(filepath.Join(created.Path(), "docs"), created.Path(), corpus.Options{Gatherers: gather.Builtin()})
	if err != nil {
		t.Fatal(err)
	}
	answer, observations, err := tick.Measure(context.Background(), tick.MeasureRequest{
		Root:      created.Path(),
		Corpus:    c,
		Artifact:  claudeRepairPromise,
		Catalogue: gather.Builtin(),
		Deadline:  5 * time.Minute,
		Workers:   runtime.NumCPU(),
	})
	if err != nil {
		t.Fatalf("measure %s in the box: %v", claudeRepairPromise, err)
	}
	return answer, observations
}

// fileMeasurements files one side of the comparison where measurements live and
// names it in the receipt, exactly as Perform's own file does.
func fileMeasurements(t *testing.T, l *ledger.Ledger, receipt *thread.Receipt, side thread.Side, observations map[string]model.Observation) {
	t.Helper()
	for name, observation := range observations {
		address, err := l.Observe(ledger.Measured(observation))
		if err != nil {
			t.Fatalf("file the %s measurement of %s: %v", side, name, err)
		}
		if err := receipt.RecordMeasurement(thread.Measurement{Side: side, Name: name, Address: address}); err != nil {
			t.Fatal(err)
		}
	}
}

// anchorTheSession commits what the confined repair left and points the thread's
// ref at it, exactly as Perform does — the commit's own diff against its parent
// is then the change the pass is shown. See docs/decision/0067.
func anchorTheSession(t *testing.T, created *box.Box, receipt *thread.Receipt) {
	t.Helper()
	ref, err := ledger.ThreadRef(string(receipt.ID()))
	if err != nil {
		t.Fatal(err)
	}
	parent := created.Commit()
	commit, err := created.Anchor(context.Background(), ref,
		"repair "+claudeRepairPromise+"\n\nthread "+string(receipt.ID())+".\n",
		box.Identity{Name: "Test", Email: "test@example.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	if err := receipt.RecordAnchor(thread.Anchor{Commit: commit, Parent: parent, Promise: claudeRepairPromise}); err != nil {
		t.Fatal(err)
	}
}

// claudeSessionSummaryOf is claudeSessionSummary over a receipt read back out of
// a store, for the one failure that needs to say what the session did rather
// than what it produced.
func claudeSessionSummaryOf(t *testing.T, store ledger.Store, id thread.ID) string {
	t.Helper()
	entries, _, err := thread.ReadReceipt(store, id)
	if err != nil {
		return err.Error()
	}
	return claudeSessionSummary(entries)
}
