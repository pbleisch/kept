package land

import (
	"context"
	"fmt"
	"os"
	oexec "os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/gather"
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/lock"
	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/run"
	"github.com/pbleisch/kept/internal/thread"
	"github.com/pbleisch/kept/internal/tick"
)

// The crash window, staged rather than simulated.
//
// docs/decision/0018 makes one ordering claim about a land — *push first,
// record after* — and two guarantees fall out of it: a crash can lose the
// record of something that happened, and can never produce a record of
// something that did not. A test that reached that by not calling the record
// function would be proving the order of two lines in this file. What is worth
// proving is survivability, so these tests kill a real process inside the
// window and then run a real tick against what the kill left behind.
//
// The window is held open by the remote itself. A `post-update` hook runs on
// the bare origin *after* the refs are updated, and `git push` does not return
// until it exits — measured, not assumed. So a hook that reports and then waits
// puts the land in exactly one state: the write has happened and the record
// cannot have. The parent reads the marker, kills the whole process group, and
// everything after that is an observation of the world rather than of this
// package.
const (
	crashChild  = "KEPT_LAND_CRASH_CHILD"
	crashRoot   = "KEPT_LAND_CRASH_ROOT"
	crashLedger = "KEPT_LAND_CRASH_LEDGER"
	crashThread = "KEPT_LAND_CRASH_THREAD"
)

// TestMain re-executes this binary as the process that performs the land, so
// that the crash is a signal delivered to a real process rather than a branch
// taken inside a test.
//
// The child runs no test: it rebuilds the request from the fixture's paths and
// calls [Perform], which is the same code `kept land` calls.
func TestMain(m *testing.M) {
	if os.Getenv(crashChild) == "" {
		os.Exit(m.Run())
	}
	os.Exit(landAsChild())
}

// landAsChild is the run that gets killed. It reports nothing back: whether the
// land happened is read off the remote and the ledger afterwards, because a
// process that is killed mid-push has no way to say so and a test that believed
// it would be trusting the crashed run's own account.
func landAsChild() int {
	root, records, id := os.Getenv(crashRoot), os.Getenv(crashLedger), os.Getenv(crashThread)
	store := ledger.NewFiles(records)
	c, err := corpus.Load(filepath.Join(root, "docs"), root, corpus.Options{Gatherers: gather.Builtin()})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	acts, _, err := ledger.ReadActs(store)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	l := ledger.Open(store, run.Start("land-crash-child", nil))
	res, err := Perform(context.Background(), Request{
		Workspace: root,
		Corpus:    c,
		Acts:      acts,
		Thread:    id,
		Target:    Target{Remote: "origin", Branch: "main"},
		By:        "Test <test@example.invalid>",
		Deadlines: Deadlines{Gatherer: 2 * time.Minute, Git: 2 * time.Minute},
		Store:     store,
		Ledger:    l,
		Wait:      30 * time.Second,
	})
	if err != nil {
		_ = l.Close(run.Failed)
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if err := l.Close(run.Clean); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	fmt.Printf("%s %s recorded=%v\n", res.Status, res.Commit, res.Recorded)
	return 0
}

// TestACrashBetweenThePushAndTheRecordLosesTheRecordAndNotTheLand is
// docs/decision/0018's ordering, killed rather than argued.
//
// The land is performed by a separate process, which is SIGKILLed while it is
// inside the push and after the remote's ref has already moved. What the crash
// leaves is the recoverable half of 0018's pair: the change is on the target
// and the ledger does not mention it. Reverse the two lines in [attemptLand] and
// this test fails on the assertion that says so, because the record would
// already be on disk when the process dies.
func TestACrashBetweenThePushAndTheRecordLosesTheRecordAndNotTheLand(t *testing.T) {
	w := newLandWorkspace(t)
	th := w.anchor("promise:a-is-whole", func(b string) {
		writeFile(t, filepath.Join(b, "sub/a/note/second.md"), noteFile("second"))
	})
	before := w.remoteHead()
	runsBefore := w.runRecords(t)

	crashed := w.crashInsideTheWindow(t, th)

	// The write happened. This is the premise rather than the finding: a hook
	// that ran without the ref moving would make everything below vacuous.
	landed := w.remoteHead()
	if landed == before {
		t.Fatalf("the premise failed: the push hook ran and %s is still at %s, so nothing was written to crash after",
			w.target, before)
	}
	if !crashed.killed {
		t.Fatalf("the land exited %d rather than dying on a signal, so no crash was staged:\n%s",
			crashed.code, crashed.output)
	}

	// The finding. A land that pushed and died records nothing, so the ledger
	// is behind the world and never ahead of it.
	acts, quarantined, err := ledger.ReadActs(w.store)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range acts {
		if a.Kind == ledger.Landed {
			t.Fatalf("the ledger holds a landed act for %s after a crash inside the push; "+
				"an act written before the write is one nothing can detect afterwards", a.Artifact)
		}
	}
	if len(quarantined) != 0 {
		t.Fatalf("the crash left %d unreadable ledger lines: %+v", len(quarantined), quarantined)
	}

	// What the crashed run left of itself, which is not what 0018's table
	// predicts. It says a dead run's runs/{id}.json keeps Ended zero and Exit
	// empty; a land appends nothing before the push, diagnostics are written
	// with the first append, so a land killed inside this window leaves no run
	// record at all. Nothing untrue is recorded either way — which is the
	// guarantee — but a reader going looking for the unclean-stop evidence 0018
	// describes will not find one, so this pins what is actually there.
	if now := w.runRecords(t); len(now) != len(runsBefore) {
		t.Fatalf("the crashed land left %d run records where there were %d; "+
			"a run that appended nothing declares nothing", len(now), len(runsBefore))
	}

	// And the lock is not the casualty. 0018 says the flock is released by the
	// kernel; a crash that left it held would make "the land can be run again"
	// false in the one situation it is claimed for.
	held, err := lock.Acquire(context.Background(), w.root, lock.Land, 0)
	if err != nil {
		t.Fatalf("the crashed run left the land lock held, so nothing can land after a crash: %v", err)
	}
	if err := held.Release(); err != nil {
		t.Fatal(err)
	}
}

// TestTheTickAfterACrashedLandObservesTheWorldRatherThanTheLedger is the other
// half of the same sentence, and it is the one that makes the loss recoverable
// rather than merely honest.
//
// The record is gone. Nothing reads it to answer the question anyway: truth is
// derived on read, so a tick over a tree that now holds what landed reaches
// `kept` and records it — 0018's *the correct outcome by a different route*.
func TestTheTickAfterACrashedLandObservesTheWorldRatherThanTheLedger(t *testing.T) {
	w := newLandWorkspace(t)
	th := w.anchor("promise:a-is-whole", func(b string) {
		writeFile(t, filepath.Join(b, "sub/a/note/second.md"), noteFile("second"))
	})

	// The promise does not hold before the land, which is what makes the
	// verdict afterwards evidence of the land rather than of the fixture.
	if got := w.tick(t, "promise:a-is-whole").Answer.Value; got != model.NotKept {
		t.Fatalf("the premise failed: %s reads %s before anything landed, want not-kept", "promise:a-is-whole", got)
	}

	crashed := w.crashInsideTheWindow(t, th)
	if !crashed.killed {
		t.Fatalf("the land exited %d rather than dying on a signal:\n%s", crashed.code, crashed.output)
	}
	for _, a := range w.acts(t) {
		if a.Kind == ledger.Landed {
			t.Fatal("the crash recorded a land, so this test would pass on the record rather than on the world")
		}
	}

	// The operator's checkout takes what is on the target, exactly as it would
	// on any other morning. Nothing about this is a recovery path.
	w.git("fetch", "--quiet", "origin", "main")
	w.git("merge", "--quiet", "--ff-only", "FETCH_HEAD")

	subject := w.tick(t, "promise:a-is-whole")
	if subject.Answer.Value != model.Kept {
		t.Fatalf("the tick after a crashed land reads %s (%s), want kept — the change is on the target and in this tree",
			subject.Answer.Value, subject.Answer.Detail)
	}
	if !subject.Answer.Gathered {
		t.Fatal("the tick carried an answer forward instead of observing the world it was asked about")
	}
	if !subject.Answer.Recorded {
		t.Fatal("the tick reached kept and recorded no truth, so the lost record was not replaced by an observed one")
	}
	// Still no land act, and that is correct rather than a gap: nothing
	// observed a land, so nothing may record one. The ledger is behind the
	// world, which is the only direction 0018 allows.
	for _, a := range w.acts(t) {
		if a.Kind == ledger.Landed {
			t.Fatal("a tick recorded a landed act; a tick observes and never lands")
		}
	}
}

// TestALandRunAgainAfterACrashRecordsWhatIsAlreadyOnTheTarget pins 0018's
// second bound — *the change is not lost, because the anchor still holds it and
// the land can be run again* — for the arrival that exists.
//
// The re-run gates against a target that already holds the change, so its merge
// is a no-op and its push moves nothing. What it does do is record the act the
// crash lost, which is a record of something that has already happened and is
// therefore the one direction the ordering rule permits.
//
// It is also where docs/decision/0068's *an empty result is refused rather than
// committed* is caught. The three-way merge with the base stated is idempotent,
// so a second land computes a tree the target already has; without the check
// that returns the box's own commit for it, this land writes an empty commit
// and pushes it, and the assertion that the target did not move is what fails.
func TestALandRunAgainAfterACrashRecordsWhatIsAlreadyOnTheTarget(t *testing.T) {
	w := newLandWorkspace(t)
	th := w.anchor("promise:a-is-whole", func(b string) {
		writeFile(t, filepath.Join(b, "sub/a/note/second.md"), noteFile("second"))
	})

	crashed := w.crashInsideTheWindow(t, th)
	if !crashed.killed {
		t.Fatalf("the land exited %d rather than dying on a signal:\n%s", crashed.code, crashed.output)
	}
	landed := w.remoteHead()

	// The anchor is what makes "run it again" mean anything: a land reads it
	// and never writes it, so a crash mid-land cannot take the change with it.
	entries, _, err := thread.ReadReceipt(w.store, th)
	if err != nil {
		t.Fatalf("read the thread after a crashed land: %v", err)
	}
	anchor, found := thread.LatestAnchor(entries)
	if !found {
		t.Fatal("the crash left the thread with no anchor, so the change the land was carrying is lost")
	}
	ref, err := ledger.ThreadRef(string(th))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(w.git("rev-parse", "--verify", ref)); got != anchor.Commit {
		t.Fatalf("%s names %s after the crash, and the thread anchored %s", ref, got, anchor.Commit)
	}

	// The hook is gone, so this land runs to completion.
	w.unblockThePush(t)
	res, err := Perform(context.Background(), w.request(th))
	if err != nil {
		t.Fatalf("the land could not be run again after a crash: %v", err)
	}
	if res.Status != Landed {
		t.Fatalf("re-running a crashed land reports %q · %q (%s), want %q",
			res.Status, res.Condition, res.Detail, Landed)
	}
	if !res.Recorded {
		t.Fatal("the land ran again and still recorded nothing, so the lost act is unrecoverable by the verb that lost it")
	}
	if got := w.remoteHead(); got != landed {
		t.Fatalf("re-running a crashed land moved %s from %s to %s; the change was already there",
			w.target, landed, got)
	}
	acts := 0
	for _, a := range w.acts(t) {
		if a.Kind == ledger.Landed {
			acts++
		}
	}
	if acts != 1 {
		t.Fatalf("the ledger holds %d landed acts after one crash and one re-run, want 1", acts)
	}
}

// --- staging the crash ---------------------------------------------------

type crashed struct {
	killed bool
	code   int
	output string
}

// crashInsideTheWindow performs a land in a separate process and kills it after
// the remote's ref has moved and before anything could be recorded.
//
// The order is what makes it deterministic rather than timed. `post-update`
// runs on the remote once the refs are updated, `git push` blocks until it
// exits, and this waits for the marker that hook writes — so at the moment of
// the kill the write is done and the run has not returned from the push, let
// alone reached [recordLand].
func (w *landWorkspace) crashInsideTheWindow(t *testing.T, id thread.ID) crashed {
	t.Helper()
	pushed, release := w.blockThePushAfterTheRefMoves(t)

	child := oexec.Command(os.Args[0])
	child.Env = append(os.Environ(),
		crashChild+"=1",
		crashRoot+"="+w.root,
		crashLedger+"="+w.records,
		crashThread+"="+string(id),
	)
	var out strings.Builder
	child.Stdout, child.Stderr = &out, &out
	// Its own process group, so the kill reaches the push and the remote's
	// blocked hook with it. A crash takes the whole run, not just the parent
	// frame of it.
	child.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := child.Start(); err != nil {
		t.Fatalf("start the land that is going to be killed: %v", err)
	}
	group := child.Process.Pid
	t.Cleanup(func() { _ = syscall.Kill(-group, syscall.SIGKILL) })

	waitForFile(t, pushed, "the land never reached the push", 3*time.Minute, child)
	if err := syscall.Kill(-group, syscall.SIGKILL); err != nil {
		t.Fatalf("kill the land mid-push: %v", err)
	}
	// Whatever survived the group kill is released rather than left waiting.
	_ = os.WriteFile(release, nil, 0o644)
	_ = child.Wait()

	state := child.ProcessState
	return crashed{
		killed: state != nil && state.ExitCode() == -1,
		code:   state.ExitCode(),
		output: out.String(),
	}
}

// blockThePushAfterTheRefMoves installs the hook that holds the window open.
//
// It reports and then waits. The wait is bounded because a hook that never
// returns is a `git receive-pack` that never returns, and a suite that hangs
// reports a timeout rather than the failure underneath it.
func (w *landWorkspace) blockThePushAfterTheRefMoves(t *testing.T) (pushed, release string) {
	t.Helper()
	signals := resolve(t, t.TempDir())
	pushed = filepath.Join(signals, "pushed")
	release = filepath.Join(signals, "release")
	hook := filepath.Join(w.origin, "hooks", "post-update")
	writeFile(t, hook, "#!/bin/sh\n"+
		": > '"+pushed+"'\n"+
		"i=0\n"+
		"while [ ! -e '"+release+"' ] && [ \"$i\" -lt 1800 ]; do sleep 0.1; i=$((i+1)); done\n"+
		"exit 0\n")
	if err := os.Chmod(hook, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.WriteFile(release, nil, 0o644) })
	return pushed, release
}

// unblockThePush takes the hook away, so a land after the staged crash runs to
// completion the way any other land does.
func (w *landWorkspace) unblockThePush(t *testing.T) {
	t.Helper()
	if err := os.Remove(filepath.Join(w.origin, "hooks", "post-update")); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

// waitForFile blocks until a path exists, and gives up if the process that was
// supposed to create it has already gone.
func waitForFile(t *testing.T, path, why string, bound time.Duration, child *oexec.Cmd) {
	t.Helper()
	deadline := time.Now().Add(bound)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		if child.ProcessState != nil {
			t.Fatalf("%s: it exited %d first", why, child.ProcessState.ExitCode())
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s within %s", why, bound)
}

// --- reading the world afterwards ----------------------------------------

// tick is a real pass over the fixture's corpus: the tree is walked, the world
// digest is computed over what is there, the gatherer runs and the predicate is
// applied. Nothing any land reported is read.
func (w *landWorkspace) tick(t *testing.T, artifact string) tick.Subject {
	t.Helper()
	writable := ledger.Open(w.store, run.Start("test-tick", nil))
	pass := &tick.Tick{
		Corpus:    w.load(),
		Store:     w.store,
		Ledger:    writable,
		Catalogue: gather.Builtin(),
		Options:   tick.Options{Deadline: 2 * time.Minute},
	}
	res, err := pass.Run(context.Background())
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if err := writable.Close(run.Clean); err != nil {
		t.Fatal(err)
	}
	for _, s := range res.Subjects {
		if s.ID == artifact {
			return s
		}
	}
	t.Fatalf("the tick did not measure %s", artifact)
	return tick.Subject{}
}

// runRecords is every run that has declared itself, read off the store rather
// than counted, because the crashed run's id is minted in a process this one
// never hears from.
func (w *landWorkspace) runRecords(t *testing.T) []string {
	t.Helper()
	entries, err := filepath.Glob(filepath.Join(w.records, ledger.Dir, "runs", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	return entries
}

func (w *landWorkspace) acts(t *testing.T) []ledger.Act {
	t.Helper()
	acts, _, err := ledger.ReadActs(w.store)
	if err != nil {
		t.Fatal(err)
	}
	return acts
}
