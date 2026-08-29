package land

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/lock"
	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/progress"
	"github.com/pbleisch/kept/internal/thread"
)

// Two lands in flight at once, which is what docs/decision/0015's queue is for.
//
// The interleaving is driven rather than timed. These tests hold the land lock,
// so both lands finish their gates and stop at the same point — the only point
// in [Perform] where one land can wait for another — and neither can reach its
// in-lock check until the lock is released. Nothing here sleeps waiting for a
// race to happen: the phase a land prints before it blocks is the signal, and a
// land that never prints it fails the test rather than hanging the suite.
//
// # Which pairs can genuinely race
//
// A land arrives one of two ways, so there are three pairs, and they are not
// worth the same:
//
//   - **Two ref arrivals onto one target.** The real one, and the one the queue
//     exists for. Two runs on this machine, or one here and one on the
//     operator's laptop; either way the loser's in-lock check finds the target
//     somewhere else and re-gates. That is
//     [TestTwoThreadsFinishingAtOnceBothLand] and
//     [TestTheSecondThreadReGatesAgainstWhatTheFirstLanded].
//   - **Two worktree arrivals into one tree.** Also real, and it resolves
//     differently: the target's *ref* never moves under a worktree arrival, so
//     the in-lock check sees nothing and what catches the collision is 0033's
//     digest over the tree. Covered in
//     [TestTwoWorktreeArrivalsIntoOneTreeLeaveTheSecondTornRatherThanWritten],
//     which is where the difference is argued.
//   - **A ref arrival and a worktree arrival onto one target.** Both are
//     possible in flight — the table is answered before the gate, and the
//     operator may start editing while it runs — and the pair cannot survive to
//     the write, because the arrival table is asked again inside the lock and a
//     ref arrival onto a tree that has since gone dirty is refused rather than
//     downgraded. That is
//     [TestATreeThatGoesDirtyUnderAPlannedRefArrivalRefuses], and it is not
//     repeated here.
//
// A pair onto two different targets shares nothing but the lock, and a land
// that waits for a lock and then finds its own target where it left it is one
// land, twice.

// TestTwoThreadsFinishingAtOnceBothLand is the whole point of the queue: the
// second one goes in *after* the first, not instead of it.
//
// Both threads repair a different promise and touch disjoint paths, so nothing
// about either change is a reason to refuse the other. Both gate against the
// same tip, because the lock is held while they do it and nothing else can
// write the target. One of them wins the lock and lands; the other finds the
// target somewhere else, re-gates against what actually landed, and lands on
// top of it — which is visible afterwards as the second commit having the
// first as its only parent.
//
// Disjointness is the case docs/decision/0015 used to state a skip for, and the
// skip is withdrawn: the loser pays a full second gate here even though nothing
// it measured could have been affected. The assertion on Attempts is what says
// so, and it fails if a skip is ever reintroduced.
func TestTwoThreadsFinishingAtOnceBothLand(t *testing.T) {
	w := newLandWorkspace(t)
	w.alsoPromises(t, "c-is-whole", "sub/c", 2, "sub/c/**")

	one := w.anchor("promise:a-is-whole", func(b string) {
		writeFile(t, filepath.Join(b, "sub/a/note/second.md"), noteFile("second"))
	})
	two := w.anchor("promise:c-is-whole", func(b string) {
		writeFile(t, filepath.Join(b, "sub/c/note/second.md"), noteFile("second"))
	})
	base := w.remoteHead()

	first, second := w.bothGatedAgainst(t, base, one, two, w.target)

	if first.Status != Landed || second.Status != Landed {
		t.Fatalf("two threads finishing at once landed %q · %q (%s) and %q · %q (%s); both had to land",
			first.Status, first.Condition, first.Detail, second.Status, second.Condition, second.Detail)
	}

	// One of them raced and one of them did not, and the one that did is the
	// one that saw the target somewhere other than where it gated.
	winner, loser := first, second
	if len(first.Moved) > 0 {
		winner, loser = second, first
	}
	if len(winner.Moved) != 0 {
		t.Fatalf("both lands reported the target moving (%v and %v); one of them held the lock first",
			first.Moved, second.Moved)
	}
	if len(loser.Moved) != 1 || loser.Moved[0] != winner.Commit {
		t.Fatalf("the second land saw the target at %v, and the first put %s there", loser.Moved, winner.Commit)
	}
	if winner.Attempts != 1 {
		t.Fatalf("the land that won the lock gated %d times", winner.Attempts)
	}
	if loser.Attempts != 2 {
		t.Fatalf("the land that lost the lock gated %d times; a target that moved is re-gated unconditionally, "+
			"whatever the two changes touch", loser.Attempts)
	}
	if winner.GatedAgainst != base {
		t.Fatalf("the first land gated against %s, and the target was at %s", winner.GatedAgainst, base)
	}
	if loser.GatedAgainst != winner.Commit {
		t.Fatalf("the second land's re-gate ran against %s, and what landed under it was %s",
			loser.GatedAgainst, winner.Commit)
	}

	// And the target holds both changes, one on top of the other. A queue that
	// dropped the loser would leave the first file and not the second; one that
	// pushed the loser's own gate over the top would leave the second and not
	// the first.
	if head := w.remoteHead(); head != loser.Commit {
		t.Fatalf("%s is at %s and the second land reported %s", w.target, head, loser.Commit)
	}
	if parents := w.remoteParents(); len(parents) != 1 || parents[0] != winner.Commit {
		t.Fatalf("what landed second has parents %v, and the first land put %s on the target",
			parents, winner.Commit)
	}
	for _, path := range []string{"sub/a/note/second.md", "sub/c/note/second.md"} {
		if !w.remoteHolds(path) {
			t.Fatalf("%s is not on %s, so one of the two threads landed nothing", path, w.target)
		}
	}
	if acts := w.landedActs(t); len(acts) != 2 {
		t.Fatalf("two lands recorded %d acts", len(acts))
	}
}

// TestTheSecondThreadReGatesAgainstWhatTheFirstLanded is the clause the whole
// increment rests on, and the one most likely to pass by accident: every commit
// that reaches a landing target was gated against the tree it landed on.
//
// A land that merely retried its push would satisfy "both landed" and violate
// this, so the re-gate is proven by its *answer* changing rather than by its
// having run. Both threads repair `a-is-whole`, which wants exactly two
// artifacts under `sub/a`; each adds one and each is correct on its own. With
// the other already on the target there are three, so the loser's re-gate
// measures the promise it is repairing as not-kept and refuses.
//
// It is symmetric, so which one wins the lock does not matter. The loser's
// first gate having passed is not assumed either: the gate refuses *before* the
// lock is taken, so a land that printed the phase this test waits on is a land
// whose gate said yes against the tip both of them started from.
//
// Break the re-gate — return the second attempt's push without gating, or skip
// it when the paths look disjoint — and the target ends up with three notes and
// a broken promise, which is what the last two assertions read.
func TestTheSecondThreadReGatesAgainstWhatTheFirstLanded(t *testing.T) {
	w := newLandWorkspace(t)
	one := w.anchor("promise:a-is-whole", func(b string) {
		writeFile(t, filepath.Join(b, "sub/a/note/second.md"), noteFile("second"))
	})
	two := w.anchor("promise:a-is-whole", func(b string) {
		writeFile(t, filepath.Join(b, "sub/a/note/third.md"), noteFile("third"))
	})
	base := w.remoteHead()

	first, second := w.bothGatedAgainst(t, base, one, two, w.target)

	winner, loser := first, second
	if second.Status == Landed {
		winner, loser = second, first
	}
	if winner.Status != Landed {
		t.Fatalf("neither land landed: %q · %q (%s) and %q · %q (%s)",
			first.Status, first.Condition, first.Detail, second.Status, second.Condition, second.Detail)
	}
	if loser.Status != Refused || loser.Condition != Promise {
		t.Fatalf("the second land was %q · %q (%s); the promise it repairs does not hold with the first land's "+
			"change already on the target, so its re-gate had to refuse it", loser.Status, loser.Condition, loser.Detail)
	}

	// The re-gate ran, against what actually landed rather than against what
	// this land had assumed.
	if loser.Attempts != 2 || len(loser.Moved) != 1 || loser.Moved[0] != winner.Commit {
		t.Fatalf("the second land ran %d attempts and saw %v; the first put %s on the target",
			loser.Attempts, loser.Moved, winner.Commit)
	}
	if loser.GatedAgainst != winner.Commit {
		t.Fatalf("the re-gate ran against %s rather than against %s", loser.GatedAgainst, winner.Commit)
	}

	// And it re-derived rather than re-merged: the promise held on the target
	// the re-gate found, and does not hold with this change merged into it.
	// That verdict is only reachable by measuring in a box cut at the commit
	// the first land created.
	verdict := loser.gate(t, "promise:a-is-whole")
	if verdict.Before != model.Kept || verdict.After != model.NotKept {
		t.Fatalf("the re-gate read %s → %s for %s; the first land left it kept, and this change makes three notes "+
			"where the promise wants two", verdict.Before, verdict.After, "promise:a-is-whole")
	}

	// Nothing of the refused land reached the target, and the promise still
	// holds on it.
	if head := w.remoteHead(); head != winner.Commit {
		t.Fatalf("%s is at %s and the only land that passed its gate reported %s", w.target, head, winner.Commit)
	}
	landed, refused := "sub/a/note/second.md", "sub/a/note/third.md"
	if loser == first {
		landed, refused = refused, landed
	}
	if !w.remoteHolds(landed) {
		t.Fatalf("%s is not on %s", landed, w.target)
	}
	if w.remoteHolds(refused) {
		t.Fatalf("%s reached %s from a land whose gate refused it", refused, w.target)
	}
	if acts := w.landedActs(t); len(acts) != 1 {
		t.Fatalf("one land and one refusal recorded %d acts", len(acts))
	}
}

// TestTwoWorktreeArrivalsIntoOneTreeLeaveTheSecondTornRatherThanWritten is the
// other arrival, and it does not answer the same way.
//
// A worktree arrival moves no ref, so the in-lock check that catches a moved
// target sees nothing at all: what the second land collides with is the
// operator's *tree*, and the instrument for that is docs/decision/0033's digest,
// taken before the gate and re-read immediately before the write. The second
// land finds a tree that is not the one it measured, writes nothing, and says
// so — which is the same rule that refuses to write into a tree the operator is
// editing, arriving from the other side.
//
// So "both land at once" is not available for this pair, and that is the
// design rather than a gap: the change is not lost, because the anchor still
// holds it, and the second half of this test runs the land again and watches it
// arrive beside the first. What must never happen is the second land writing
// over a tree it did not measure, and the digest is what makes that impossible.
func TestTwoWorktreeArrivalsIntoOneTreeLeaveTheSecondTornRatherThanWritten(t *testing.T) {
	w := newLandWorkspace(t)
	w.alsoPromises(t, "c-is-whole", "sub/c", 2, "sub/c/**")
	// The operator is holding an edit, which is the row that admits a worktree
	// arrival at all.
	w.write("sub/b/note/only.md", noteFile("only")+"\nhalf a thought")

	one := w.anchor("promise:a-is-whole", func(b string) {
		writeFile(t, filepath.Join(b, "sub/a/note/second.md"), noteFile("second"))
	})
	two := w.anchor("promise:c-is-whole", func(b string) {
		writeFile(t, filepath.Join(b, "sub/c/note/second.md"), noteFile("second"))
	})

	target := Local("main")
	head := strings.TrimSpace(w.git("rev-parse", "HEAD"))
	first, second := w.bothGatedAgainst(t, head, one, two, target)

	winner, loser := first, second
	loserThread := two
	if second.Status == Landed {
		winner, loser = second, first
		loserThread = one
	}
	if winner.Status != Landed || winner.Plan.Arrival != ledger.WorktreeArrival {
		t.Fatalf("the first land was %q · %q arriving %q (%s)",
			winner.Status, winner.Condition, winner.Plan.Arrival, winner.Detail)
	}
	if loser.Status != Refused || loser.Condition != Contended {
		t.Fatalf("the second land was %q · %q (%s); the tree it measured had the first land's change in it by the "+
			"time it went to write, and a tree that moved is contention", loser.Status, loser.Condition, loser.Detail)
	}
	if len(loser.Arrived) != 0 {
		t.Fatalf("the torn land wrote %v into the working tree", loser.Arrived)
	}
	// The ref did not move, which is why the in-lock check had nothing to see.
	if now := strings.TrimSpace(w.git("rev-parse", "HEAD")); now != head {
		t.Fatalf("a worktree arrival moved HEAD from %s to %s", head, now)
	}
	if len(loser.Moved) != 0 {
		t.Fatalf("the second land reported the target moving to %v, and no ref moved at all", loser.Moved)
	}
	if acts := w.landedActs(t); len(acts) != 1 {
		t.Fatalf("one arrival and one refusal recorded %d acts; nothing was written for the second", len(acts))
	}

	// The change is deferred rather than dropped. Run it again against the tree
	// the operator has now, and it arrives beside the first.
	again := w.land(t, loserThread, target, "")
	if again.Status != Landed {
		t.Fatalf("the torn land was run again and reported %q · %q (%s)", again.Status, again.Condition, again.Detail)
	}
	for _, path := range []string{"sub/a/note/second.md", "sub/c/note/second.md"} {
		if _, err := os.Stat(filepath.Join(w.root, filepath.FromSlash(path))); err != nil {
			t.Fatalf("%s is not in the working tree after both threads landed: %v", path, err)
		}
	}
	if !strings.Contains(w.read("sub/b/note/only.md"), "half a thought") {
		t.Fatal("the operator's own edit did not survive two worktree arrivals")
	}
}

// --- driving the interleaving --------------------------------------------

// bothGatedAgainst runs two lands whose gates both ran against one commit, and
// returns what each of them made of it.
//
// The land lock is the only place in [Perform] where one land waits for
// another, so it is where the interleaving is driven from. Holding it means
// neither land can reach its in-lock check, its write or its record; and
// because nothing else is writing the target while it is held, both gates
// necessarily ran against the same commit. The premise is asserted rather than
// assumed: a land whose gate refused never reaches the lock at all, so a land
// that printed the phase this waits on is one whose gate passed against `at`.
func (w *landWorkspace) bothGatedAgainst(t *testing.T, at string, one, two thread.ID, target Target) (*Result, *Result) {
	t.Helper()
	held, err := lock.Acquire(context.Background(), w.root, lock.Land, 0)
	if err != nil {
		t.Fatalf("hold the land lock: %v", err)
	}
	released := false
	defer func() {
		if !released {
			_ = held.Release()
		}
	}()

	first := w.landInFlight(t, one, target)
	first.waitForPhase(t, "take the land lock")
	second := w.landInFlight(t, two, target)
	second.waitForPhase(t, "take the land lock")

	if now := w.targetNow(t, target); now != at {
		t.Fatalf("the premise failed: %s moved to %s while the land lock was held, so the two gates did not run "+
			"against one commit", target, now)
	}
	released = true
	if err := held.Release(); err != nil {
		t.Fatalf("release the land lock: %v", err)
	}
	return first.wait(t), second.wait(t)
}

// targetNow is where the target is, read the way a third party would read it:
// off the bare remote for a shared target, and off the ref for a local one.
func (w *landWorkspace) targetNow(t *testing.T, target Target) string {
	t.Helper()
	if target.Shared() {
		return w.remoteHead()
	}
	return strings.TrimSpace(w.git("rev-parse", "--verify", target.Ref()))
}

// inFlight is one land running while the test does something else, and the
// phase lines it has printed so far.
type inFlight struct {
	lines *phaseLines
	done  chan struct{}
	res   *Result
	err   error
}

func (w *landWorkspace) landInFlight(t *testing.T, id thread.ID, target Target) *inFlight {
	t.Helper()
	req := w.request(id)
	req.Target = target
	// Long enough that the loser of the lock waits for the winner's whole
	// critical section rather than reporting contention. This is not a race
	// this test is trying to lose.
	req.Wait = 2 * time.Minute

	f := &inFlight{lines: &phaseLines{}, done: make(chan struct{})}
	// The plain form, which is what anything that is not two terminals gets: a
	// line when a phase starts and a line when it finishes. No writer here is a
	// file, so nothing has to be told not to animate.
	req.Progress = progress.For(io.Discard, f.lines, false)
	go func() {
		defer close(f.done)
		defer req.Progress.Close()
		f.res, f.err = Perform(context.Background(), req)
	}()
	return f
}

// waitForPhase blocks until this land has started a named phase.
//
// Bounded, and it gives up the moment the land finishes without having printed
// it: a test that waited forever would report a timeout rather than the thing
// that went wrong, and one that waited on a phase the land never runs would
// hang the suite.
func (f *inFlight) waitForPhase(t *testing.T, words string) {
	t.Helper()
	want := "land started " + words
	for until := time.Now().Add(2 * time.Minute); time.Now().Before(until); {
		if f.lines.has(want) {
			return
		}
		select {
		case <-f.done:
			if f.lines.has(want) {
				return
			}
			t.Fatalf("the land finished without reaching %q: %+v (%v)\n%s", words, f.res, f.err, f.lines.String())
		default:
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("the land never reached %q. What it printed was:\n%s", words, f.lines.String())
}

func (f *inFlight) wait(t *testing.T) *Result {
	t.Helper()
	select {
	case <-f.done:
	case <-time.After(10 * time.Minute):
		t.Fatalf("a land never finished. What it printed was:\n%s", f.lines.String())
	}
	if f.err != nil {
		t.Fatalf("land: %v\n%s", f.err, f.lines.String())
	}
	return f.res
}

// phaseLines is the progress stream, readable from another goroutine while the
// land is still writing to it.
type phaseLines struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (p *phaseLines) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.buf.Write(b)
}

func (p *phaseLines) has(words string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return strings.Contains(p.buf.String(), words)
}

func (p *phaseLines) String() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.buf.String()
}

// alsoPromises adds a second broken promise to the fixture, over its own corpus
// root, so that two threads can repair two different things at once.
func (w *landWorkspace) alsoPromises(t *testing.T, slug, root string, artifacts int, repairScope string) {
	t.Helper()
	w.write("docs/promise/"+slug+".md", wholePromise(slug, root, artifacts, repairScope))
	w.write(root+"/_schema/note.yaml", landNoteSchema)
	w.write(root+"/note/first.md", noteFile("first"))
	w.commit()
	w.push()
	w.ratify("promise:" + slug)
}

// landedActs is every land the ledger holds, which is what a reader afterwards
// has. Two concurrent lands append to one stream, so this is also the assertion
// that neither of them lost the other's record.
func (w *landWorkspace) landedActs(t *testing.T) []ledger.Act {
	t.Helper()
	acts, quarantined, err := ledger.ReadActs(w.store)
	if err != nil {
		t.Fatal(err)
	}
	if len(quarantined) != 0 {
		t.Fatalf("the ledger holds %d lines that did not read back as acts: %+v", len(quarantined), quarantined)
	}
	var landed []ledger.Act
	for _, a := range acts {
		if a.Kind == ledger.Landed {
			landed = append(landed, a)
		}
	}
	return landed
}
