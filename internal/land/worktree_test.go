package land

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/box"
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/lock"
	"github.com/pbleisch/kept/internal/thread"
	"github.com/pbleisch/kept/internal/workspace"
)

// TestALocalRefArrivalAdvancesTheBranchAndReachesNoNetwork is the row above the
// one this slice is really about, and it is the one that makes *kept works
// locally* true: a repository with no remote at all is still a repository kept
// can land in.
//
// The target here does have a remote, and the point is that a local target
// ignores it: nothing is fetched, nothing is pushed, and the bare repository is
// exactly where it was.
func TestALocalRefArrivalAdvancesTheBranchAndReachesNoNetwork(t *testing.T) {
	w := newLandWorkspace(t)
	w.git("branch", "shipping", "main")
	before := strings.TrimSpace(w.git("rev-parse", "refs/heads/shipping"))
	remoteBefore := w.remoteHead()

	th := w.anchor("promise:a-is-whole", func(b string) {
		writeFile(t, filepath.Join(b, "sub/a/note/second.md"), noteFile("second"))
	})
	res := w.land(t, th, Local("shipping"), "")

	if res.Status != Landed {
		t.Fatalf("status = %q (%s: %s)", res.Status, res.Condition, res.Detail)
	}
	if res.Plan.Regime != ledger.LocalRegime || res.Plan.Arrival != ledger.RefArrival {
		t.Fatalf("the plan was %+v", res.Plan)
	}
	after := strings.TrimSpace(w.git("rev-parse", "refs/heads/shipping"))
	if after != res.Commit || after == before {
		t.Fatalf("shipping is at %s, the land reported %s, and it started at %s", after, res.Commit, before)
	}
	// Landing onto a local branch publishes nothing. See docs/decision/0015.
	if w.remoteHead() != remoteBefore {
		t.Fatal("a local land moved the remote")
	}
	// And the act says which regime it checked under, because a reader cannot
	// derive it: the world names a tree and a commit and says nothing about it.
	act := w.landedAct(t)
	want := ledger.Landing{Target: "shipping", Regime: ledger.LocalRegime, Arrival: ledger.RefArrival}
	if act.Landing != want {
		t.Fatalf("the act's landing is %+v, want %+v", act.Landing, want)
	}
	if act.Uncommitted {
		t.Fatal("a ref arrival was recorded from a tree holding changes no commit does")
	}
}

// TestALocalRefArrivalIntoThisCheckedOutTreeUpdatesTheFilesWithTheRef is the
// second row of the table: the branch is checked out here and the tree is
// clean, so git updates the files along with the ref — which docs/decision/0015
// calls the point rather than a side effect.
func TestALocalRefArrivalIntoThisCheckedOutTreeUpdatesTheFilesWithTheRef(t *testing.T) {
	w := newLandWorkspace(t)
	th := w.anchor("promise:a-is-whole", func(b string) {
		writeFile(t, filepath.Join(b, "sub/a/note/second.md"), noteFile("second"))
	})

	res := w.land(t, th, Local("main"), "")
	if res.Status != Landed {
		t.Fatalf("status = %q (%s: %s)", res.Status, res.Condition, res.Detail)
	}
	if res.Plan.Arrival != ledger.RefArrival {
		t.Fatalf("a clean checkout arrived %s", res.Plan.Arrival)
	}
	if head := strings.TrimSpace(w.git("rev-parse", "HEAD")); head != res.Commit {
		t.Fatalf("HEAD is at %s and the land reported %s", head, res.Commit)
	}
	// The files moved with it, which is what distinguishes this row from a ref
	// advance onto a branch nobody has checked out.
	if _, err := os.Stat(filepath.Join(w.root, "sub/a/note/second.md")); err != nil {
		t.Fatalf("the ref moved and the working tree did not follow: %v", err)
	}
	if dirty := strings.TrimSpace(w.git("status", "--porcelain")); dirty != "" {
		t.Fatalf("a ref arrival into a clean tree left it dirty:\n%s", dirty)
	}
}

// TestAWorktreeArrivalMergesIntoTheTreeAndMovesNothing is the row this slice
// exists for: the operator is standing on the branch they want the repair on,
// their tree is not clean, and the change arrives beside their own edits.
//
// Nothing moves — no ref, no commit, no index entry — and what happens to it
// next is theirs.
func TestAWorktreeArrivalMergesIntoTheTreeAndMovesNothing(t *testing.T) {
	w := newLandWorkspace(t)
	// The operator is mid-sentence in a file the repair does not touch. It has
	// to still be there afterwards, unstaged, exactly as they left it.
	w.write("sub/b/note/only.md", noteFile("only")+"\nhalf a thought")
	mine := w.read("sub/b/note/only.md")

	th := w.anchor("promise:a-is-whole", func(b string) {
		writeFile(t, filepath.Join(b, "sub/a/note/second.md"), noteFile("second"))
	})
	head := strings.TrimSpace(w.git("rev-parse", "HEAD"))
	ref := strings.TrimSpace(w.git("rev-parse", "refs/heads/main"))

	res := w.land(t, th, Local("main"), "")
	if res.Status != Landed {
		t.Fatalf("status = %q (%s: %s)", res.Status, res.Condition, res.Detail)
	}
	if res.Plan.Arrival != ledger.WorktreeArrival {
		t.Fatalf("a dirty checkout arrived %s", res.Plan.Arrival)
	}

	// The change is in the working tree.
	if got := w.read("sub/a/note/second.md"); !strings.Contains(got, "second") {
		t.Fatalf("the change did not arrive: %q", got)
	}
	if want := []string{"sub/a/note/second.md"}; !equal(res.Arrived, want) {
		t.Fatalf("the land says it wrote %v, want %v", res.Arrived, want)
	}
	// And nothing moved.
	if now := strings.TrimSpace(w.git("rev-parse", "HEAD")); now != head {
		t.Fatalf("HEAD moved from %s to %s under a worktree arrival", head, now)
	}
	if now := strings.TrimSpace(w.git("rev-parse", "refs/heads/main")); now != ref {
		t.Fatalf("main moved from %s to %s under a worktree arrival", ref, now)
	}
	if res.Commit != "" {
		t.Fatalf("a worktree arrival reported commit %s, and it put a commit nowhere", res.Commit)
	}
	// Nothing is staged: the merge is in the working tree and the index is the
	// operator's.
	if staged := strings.TrimSpace(w.git("diff", "--cached", "--name-only")); staged != "" {
		t.Fatalf("a worktree arrival staged:\n%s", staged)
	}
	// The operator's own edit is where they left it.
	if got := w.read("sub/b/note/only.md"); got != mine {
		t.Fatalf("the operator's uncommitted edit was rewritten:\n%q", got)
	}
}

// TestAWorktreeArrivalRecordsTheNarrowerGuarantee is the part
// docs/decision/0015 spends a section on: two different things are about to be
// called `landed`, and a reader a year later has to be able to tell which one a
// given act is worth.
//
// Every field the decision names as a mechanism is asserted here, because each
// of them is the difference between the two claims and none of them can be
// derived afterwards.
func TestAWorktreeArrivalRecordsTheNarrowerGuarantee(t *testing.T) {
	w := newLandWorkspace(t)
	w.write("sub/b/note/only.md", noteFile("only")+"\nheld")
	th := w.anchor("promise:a-is-whole", func(b string) {
		writeFile(t, filepath.Join(b, "sub/a/note/second.md"), noteFile("second"))
	})
	head := strings.TrimSpace(w.git("rev-parse", "HEAD"))

	res := w.land(t, th, Local("main"), "")
	if res.Status != Landed || !res.Recorded {
		t.Fatalf("status = %q, recorded = %v (%s)", res.Status, res.Recorded, res.Detail)
	}

	act := w.landedAct(t)
	want := ledger.Landing{Target: "main", Regime: ledger.LocalRegime, Arrival: ledger.WorktreeArrival}
	if act.Landing != want {
		t.Fatalf("the act's landing is %+v, want %+v", act.Landing, want)
	}
	// Uncommitted is the discriminator a reader already knows: true on every
	// worktree arrival, because the change it wrote is in no commit.
	if !act.Uncommitted {
		t.Fatal("a worktree arrival was recorded as though the tree held no uncommitted change")
	}
	// World.Commit keeps 0063's meaning and gains no second one: it is where
	// the operator was standing, and it landed nowhere.
	if act.World.Commit != head {
		t.Fatalf("the act names commit %s and the operator was standing on %s", act.World.Commit, head)
	}
	// The tree is still what the land was gated against, because both arrivals
	// are gated the same way and that is the one thing a world on an act is for.
	if act.World.Tree == "" || act.World.Tree != res.Tree {
		t.Fatalf("the act names tree %q and the gate measured %q", act.World.Tree, res.Tree)
	}
	if act.World.Thread != string(th) {
		t.Fatalf("the act names thread %q, want %q", act.World.Thread, th)
	}
}

// TestATornTreeRefusesAndLeavesTheWorkspaceExactlyAsItWas is the failure this
// arrival can have that no other arrival can, and it is the one worth building
// first: a half-applied merge into a working tree the operator is standing in
// is worse than refusing to land at all.
//
// The tree is moved between the gate and the write, which is exactly the window
// docs/decision/0015's third compare-and-swap covers. The lock is held by this
// test until after the change, so the ordering is not a race: the digest is
// taken before the gate, the tree moves while the land waits, and the swap runs
// after it is let through.
func TestATornTreeRefusesAndLeavesTheWorkspaceExactlyAsItWas(t *testing.T) {
	w := newLandWorkspace(t)
	w.write("sub/b/note/only.md", noteFile("only")+"\nhalf a thought")
	th := w.anchor("promise:a-is-whole", func(b string) {
		writeFile(t, filepath.Join(b, "sub/a/note/second.md"), noteFile("second"))
	})

	held, err := lock.Acquire(context.Background(), w.root, lock.Land, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	req := w.request(th)
	req.Target, req.Wait = Local("main"), 30*time.Second
	done := make(chan *Result, 1)
	fail := make(chan error, 1)
	go func() {
		res, err := Perform(context.Background(), req)
		if err != nil {
			fail <- err
			return
		}
		done <- res
	}()

	// The gate box existing is the signal that this attempt is past the point
	// where it read the tree's digest: the digest is taken before the box is
	// cut, and the box is cut before anything is measured.
	w.waitForGateBox(t)
	w.write("sub/b/note/only.md", noteFile("only")+"\nthe rest of the thought")
	afterward := w.snapshotDigest(t)
	if err := held.Release(); err != nil {
		t.Fatal(err)
	}

	var res *Result
	select {
	case res = <-done:
	case err := <-fail:
		t.Fatalf("land: %v", err)
	case <-time.After(4 * time.Minute):
		t.Fatal("the land never finished")
	}

	if res.Status != Refused || res.Condition != Contended {
		t.Fatalf("a torn tree was answered %q · %q: %s", res.Status, res.Condition, res.Detail)
	}
	if !strings.Contains(res.Detail, "moved") {
		t.Fatalf("the refusal does not say the tree moved: %q", res.Detail)
	}
	// Nothing was written. The digest is the whole assertion: it covers every
	// path in the workspace, so a single byte anywhere would move it.
	if now := w.snapshotDigest(t); now != afterward {
		t.Fatal("a refused worktree arrival wrote into the working tree")
	}
	if _, err := os.Stat(filepath.Join(w.root, "sub/a/note/second.md")); err == nil {
		t.Fatal("a refused worktree arrival left the change behind")
	}
	// And no act, because nothing happened. Recording one would claim a land
	// that never arrived. See docs/policy/record-only-what-has-already-happened.
	if res.Recorded {
		t.Fatal("a refused land recorded an act")
	}
}

// TestAWorktreeArrivalRunAgainWritesNothingAndStillRecords is
// docs/decision/0018's self-correction for the arrival that puts no commit
// anywhere.
//
// "Already landed" is a different question here: there is no ref to read it
// off, so what makes a re-run a no-op is that the merge into a tree that
// already holds the change produces that tree. The re-run must still record,
// because the crash it is correcting for is the one that wrote the files and
// lost the act.
func TestAWorktreeArrivalRunAgainWritesNothingAndStillRecords(t *testing.T) {
	w := newLandWorkspace(t)
	w.write("sub/b/note/only.md", noteFile("only")+"\nheld")
	th := w.anchor("promise:a-is-whole", func(b string) {
		writeFile(t, filepath.Join(b, "sub/a/note/second.md"), noteFile("second"))
	})

	first := w.land(t, th, Local("main"), "")
	if first.Status != Landed || len(first.Arrived) != 1 {
		t.Fatalf("the first land was %q and wrote %v", first.Status, first.Arrived)
	}
	settled := w.snapshotDigest(t)

	again := w.land(t, th, Local("main"), "")
	if again.Status != Landed {
		t.Fatalf("the second land was %q (%s: %s)", again.Status, again.Condition, again.Detail)
	}
	if len(again.Arrived) != 0 {
		t.Fatalf("the second land wrote %v into a tree that already held the change", again.Arrived)
	}
	if now := w.snapshotDigest(t); now != settled {
		t.Fatal("a second land changed a tree that already held the change")
	}
	if !again.Recorded {
		t.Fatal("a land that found the change already there recorded nothing, so a crash between the write and the record is unrecoverable")
	}
}

// TestAWorktreeArrivalThatConflictsRefusesAndWritesNothing: the operator has
// edited the very file the repair changed, in a way the merge cannot settle.
// That is a person's to resolve, and the refusal leaves them what they had.
func TestAWorktreeArrivalThatConflictsRefusesAndWritesNothing(t *testing.T) {
	w := newLandWorkspace(t)
	th := w.anchor("promise:a-is-whole", func(b string) {
		writeFile(t, filepath.Join(b, "sub/a/note/first.md"), noteFile("first")+"\nthe repair's line")
		writeFile(t, filepath.Join(b, "sub/a/note/second.md"), noteFile("second"))
	})
	w.write("sub/a/note/first.md", noteFile("first")+"\nthe operator's line")
	before := w.snapshotDigest(t)

	res := w.land(t, th, Local("main"), "")
	if res.Status != Refused || res.Condition != Conflict {
		t.Fatalf("a conflicting merge was answered %q · %q: %s", res.Status, res.Condition, res.Detail)
	}
	if now := w.snapshotDigest(t); now != before {
		t.Fatal("a conflicted worktree arrival wrote into the working tree")
	}
	if got := w.read("sub/a/note/first.md"); !strings.Contains(got, "the operator's line") {
		t.Fatalf("the operator's edit did not survive a conflicted arrival: %q", got)
	}
	if strings.Contains(w.read("sub/a/note/first.md"), "<<<<") {
		t.Fatal("a conflicted arrival wrote conflict markers into the operator's file")
	}
}

// TestAWorktreeArrivalCarriesADeleteAndAModeAcrossToo: the merge is the change,
// whole. A delete is exactly what a scope review most needs to see, and an
// executable bit dropped in transit is a change the gate measured and the tree
// did not get.
func TestAWorktreeArrivalCarriesADeleteAndAModeAcrossToo(t *testing.T) {
	w := newLandWorkspace(t)
	w.write("sub/spare/leaving.txt", "this file is on its way out\n")
	w.commit()
	w.write("sub/b/note/only.md", noteFile("only")+"\nheld")

	th := w.anchor("promise:a-is-whole", func(b string) {
		writeFile(t, filepath.Join(b, "sub/a/note/second.md"), noteFile("second"))
		if err := os.Remove(filepath.Join(b, "sub/spare/leaving.txt")); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(b, "sub/spare/run.sh"), "#!/bin/sh\necho hello\n")
		if err := os.Chmod(filepath.Join(b, "sub/spare/run.sh"), 0o755); err != nil {
			t.Fatal(err)
		}
	})

	res := w.land(t, th, Local("main"), "")
	if res.Status != Landed {
		t.Fatalf("status = %q (%s: %s)", res.Status, res.Condition, res.Detail)
	}
	if _, err := os.Stat(filepath.Join(w.root, "sub/spare/leaving.txt")); !os.IsNotExist(err) {
		t.Fatalf("the delete did not arrive: %v", err)
	}
	info, err := os.Stat(filepath.Join(w.root, "sub/spare/run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("the executable bit did not arrive: %v", info.Mode())
	}
}

// TestARefusedArrivalNeverTouchesTheWorkingTree sweeps the four cells of
// docs/decision/0015's table that refuse, and asserts the property they all
// share: a refusal that half-writes is a defect rather than a refusal.
//
// These refuse before a gate box is cut, so the assertion is that they refuse
// at all and that the tree is byte-identical afterwards — which is the same
// digest the arrival's own compare-and-swap reads.
func TestARefusedArrivalNeverTouchesTheWorkingTree(t *testing.T) {
	for _, c := range []struct {
		name    string
		arrange func(*landWorkspace) Target
		pinned  ledger.Arrival
	}{
		{"no worktree holds it", func(w *landWorkspace) Target { return Origin() }, ledger.WorktreeArrival},
		{"here and dirty", func(w *landWorkspace) Target {
			w.write("sub/b/note/only.md", noteFile("only")+"\nheld")
			return Local("main")
		}, ledger.RefArrival},
		{"another worktree, ref", func(w *landWorkspace) Target { return w.elsewhere(t) }, ledger.RefArrival},
		{"another worktree, worktree", func(w *landWorkspace) Target { return w.elsewhere(t) }, ledger.WorktreeArrival},
	} {
		t.Run(c.name, func(t *testing.T) {
			w := newLandWorkspace(t)
			target := c.arrange(w)
			th := w.anchor("promise:a-is-whole", func(b string) {
				writeFile(t, filepath.Join(b, "sub/a/note/second.md"), noteFile("second"))
			})
			before := w.snapshotDigest(t)

			req := w.request(th)
			req.Target, req.Arrival = target, c.pinned
			res, err := Perform(context.Background(), req)
			if err == nil {
				t.Fatalf("%s admitted a %s arrival: %+v", target, c.pinned, res)
			}
			var unlandable *Unlandable
			if !asUnlandable(err, &unlandable) {
				t.Fatalf("the refusal is not a request kept can answer: %v", err)
			}
			if now := w.snapshotDigest(t); now != before {
				t.Fatalf("a refused %s arrival wrote into the working tree", c.pinned)
			}
		})
	}
}

// TestATornTreeIsCaughtByTheDigestAndNotByTheClock reaches under the land to the
// swap itself, and breaks the thing it is supposed to catch: a gate digest that
// does not describe the tree in front of it.
//
// The end-to-end proof above is the one that matters; this is the guard that
// fails loudly if the comparison is ever removed, dropped to a stat, or made to
// compare a tree against itself.
func TestATornTreeIsCaughtByTheDigestAndNotByTheClock(t *testing.T) {
	w := newLandWorkspace(t)
	req := w.request("unused")
	head := strings.TrimSpace(w.git("rev-parse", "HEAD"))

	now, err := workspaceDigest(req)
	if err != nil {
		t.Fatal(err)
	}
	// The digest as it is refuses nothing about the merge, which is what makes
	// the next assertion mean something: the swap is what refuses, not the
	// merge failing for some other reason.
	if _, err := arriveInWorktree(context.Background(), req, head, head, now+"x", identityFor(t)); err == nil {
		t.Fatal("a digest that does not describe this tree was accepted")
	} else if !strings.Contains(err.Error(), "moved between the gate and the write") {
		t.Fatalf("the refusal is not the swap: %v", err)
	}
}

// --- fixture helpers -----------------------------------------------------

// land runs one land onto a stated target with a stated arrival, which is the
// shape every row of the table needs and the one `w.request` does not give.
func (w *landWorkspace) land(t *testing.T, id thread.ID, target Target, pinned ledger.Arrival) *Result {
	t.Helper()
	req := w.request(id)
	req.Target, req.Arrival = target, pinned
	res, err := Perform(context.Background(), req)
	if err != nil {
		t.Fatalf("land onto %s: %v", target, err)
	}
	return res
}

// snapshotDigest is docs/decision/0033's digest over the workspace, which is
// the instrument the arrival's own compare-and-swap uses. A test that compared
// file contents instead would be asserting about the paths it thought to name.
func (w *landWorkspace) snapshotDigest(t *testing.T) string {
	t.Helper()
	walked, err := workspace.Read(w.root, workspace.Options{Workers: 1, All: true})
	if err != nil {
		t.Fatal(err)
	}
	return walked.Digest()
}

func (w *landWorkspace) read(name string) string {
	w.t.Helper()
	body, err := os.ReadFile(filepath.Join(w.root, filepath.FromSlash(name)))
	if err != nil {
		w.t.Fatal(err)
	}
	return string(body)
}

// elsewhere puts a branch in a second worktree of this repository, which is the
// bottom row of the arrival table.
func (w *landWorkspace) elsewhere(t *testing.T) Target {
	t.Helper()
	at := filepath.Join(t.TempDir(), "elsewhere")
	w.git("worktree", "add", "--quiet", "-b", "elsewhere", at, "HEAD")
	t.Cleanup(func() { _, _ = gitTry(w.root, "worktree", "remove", "--force", at) })
	return Local("elsewhere")
}

// waitForGateBox blocks until this attempt has cut its gate box, which is the
// point after which the working tree's digest has been read.
func (w *landWorkspace) waitForGateBox(t *testing.T) {
	t.Helper()
	boxes := filepath.Join(w.root, ".kept", "boxes")
	for until := time.Now().Add(2 * time.Minute); time.Now().Before(until); {
		if entries, err := os.ReadDir(boxes); err == nil && len(entries) > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("no gate box appeared, so the land never got past reading the target")
}

// landedAct is the one land act in the ledger, which is what a reader a year
// later actually has.
func (w *landWorkspace) landedAct(t *testing.T) ledger.Act {
	t.Helper()
	acts, quarantined, err := ledger.ReadActs(w.store)
	if err != nil {
		t.Fatal(err)
	}
	if len(quarantined) != 0 {
		t.Fatalf("the land act did not read back as an act: %+v", quarantined)
	}
	var landed []ledger.Act
	for _, a := range acts {
		if a.Kind == ledger.Landed {
			landed = append(landed, a)
		}
	}
	if len(landed) != 1 {
		t.Fatalf("the ledger holds %d land acts, want 1", len(landed))
	}
	return landed[0]
}

// identityFor is who the fixture performs an act as.
func identityFor(t *testing.T) box.Identity {
	t.Helper()
	return box.Identity{Name: "Test", Email: "test@example.invalid"}
}

// asUnlandable is errors.As, named so the assertion above reads as the question
// it is asking.
func asUnlandable(err error, into **Unlandable) bool { return errors.As(err, into) }

func equal(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// TestNothingArrivesInTheWorkingTreeThatTheScopeReviewDidNotSee keeps 0037's
// gate honest across the arrival that puts no commit anywhere.
//
// The review is computed over the paths the merge would put on the target, and
// what a worktree arrival writes is the diff between the operator's tree and
// the merged one. Those are two different sets and the second has to stay
// inside the first, or a path could reach the operator's tree without any
// promise's repair scope having been asked about it.
func TestNothingArrivesInTheWorkingTreeThatTheScopeReviewDidNotSee(t *testing.T) {
	w := newLandWorkspace(t)
	// Two dirty files the repair does not touch, so the operator's tree and the
	// target's differ in ways the change knows nothing about.
	w.write("sub/b/note/only.md", noteFile("only")+"\nheld")
	w.write("sub/spare/mine.txt", "work in progress\n")

	th := w.anchor("promise:a-is-whole", func(b string) {
		writeFile(t, filepath.Join(b, "sub/a/note/second.md"), noteFile("second"))
	})
	res := w.land(t, th, Local("main"), "")
	if res.Status != Landed {
		t.Fatalf("status = %q (%s: %s)", res.Status, res.Condition, res.Detail)
	}

	reviewed := map[string]bool{}
	for _, path := range res.Review.InScope {
		reviewed[path] = true
	}
	for _, path := range res.Review.Outside {
		reviewed[path] = true
	}
	for _, path := range res.Arrived {
		if !reviewed[path] {
			t.Fatalf("%s arrived in the working tree and the scope review never saw it; it saw %v", path, reviewed)
		}
	}
	if len(res.Arrived) == 0 {
		t.Fatal("nothing arrived, so this proves nothing")
	}
}

// TestTheDirtinessQuestionIgnoresKeptsOwnTier breaks the thing the pathspec is
// supposed to catch.
//
// A land cuts its gate box under .kept/boxes/, so by the time anything inside
// the lock asks *is this tree clean* the box is sitting in it. Without the
// exclusion every local ref arrival refuses itself, and it refuses with a
// message about the operator's work when the only thing in the way is kept's.
func TestTheDirtinessQuestionIgnoresKeptsOwnTier(t *testing.T) {
	w := newLandWorkspace(t)
	dirty, err := treeIsDirty(context.Background(), w.root, arrivalDeadline)
	if err != nil {
		t.Fatal(err)
	}
	if dirty {
		t.Fatal("the fixture's tree is dirty before anything wrote to it")
	}

	writeFile(t, filepath.Join(w.root, ".kept", "boxes", "abc", "note.txt"), "a gate box\n")
	if dirty, err = treeIsDirty(context.Background(), w.root, arrivalDeadline); err != nil {
		t.Fatal(err)
	}
	if dirty {
		t.Fatal("a gate box made the operator's tree read as dirty, which refuses every local ref arrival")
	}

	// And the question still answers about the operator's own work.
	w.write("sub/b/note/only.md", noteFile("only")+"\nheld")
	if dirty, err = treeIsDirty(context.Background(), w.root, arrivalDeadline); err != nil {
		t.Fatal(err)
	}
	if !dirty {
		t.Fatal("an uncommitted edit reads as clean")
	}
}

// TestAWorktreeArrivalInterruptedPartWayThroughIsFinishedByRunningItAgain is
// docs/decision/0018's third state, which is the one failure a worktree arrival
// has that no arrival onto a ref does.
//
// The write is several files and there is no transaction, so a run that dies
// part way through leaves a tree holding some of the change. What makes that
// survivable is the same thing that makes the ref arrival's lost record
// survivable: nothing reads the ledger to answer the question. A second run
// snapshots the tree it finds, merges into that, and writes what is still
// missing.
//
// The partial state is produced by the write itself rather than staged by hand:
// a directory standing where the second file has to go is a rename the code
// cannot perform, and it fails after the first file is already on disk.
func TestAWorktreeArrivalInterruptedPartWayThroughIsFinishedByRunningItAgain(t *testing.T) {
	w := newLandWorkspace(t)
	w.write("sub/b/note/only.md", noteFile("only")+"\nheld")

	th := w.anchor("promise:a-is-whole", func(b string) {
		writeFile(t, filepath.Join(b, "sub/a/note/second.md"), noteFile("second"))
		writeFile(t, filepath.Join(b, "sub/spare/zz.txt"), "the second file the change carries\n")
	})

	// An empty directory where the second file goes. Git does not see it, so
	// the merge still adds the file and the rename is what fails.
	blocked := filepath.Join(w.root, "sub", "spare", "zz.txt")
	if err := os.MkdirAll(blocked, 0o755); err != nil {
		t.Fatal(err)
	}

	req := w.request(th)
	req.Target = Local("main")
	if _, err := Perform(context.Background(), req); err == nil {
		t.Fatal("a write that could not be performed was reported as a land")
	}
	// The third state, named: the first file is on disk and the second is not.
	if _, err := os.Stat(filepath.Join(w.root, "sub/a/note/second.md")); err != nil {
		t.Fatalf("nothing was written at all, so this proves nothing about a partial one: %v", err)
	}
	if body, err := os.ReadFile(blocked); err == nil {
		t.Fatalf("the blocked path was written after all: %q", body)
	}

	// Clear what blocked it and run it again. The second run reads the tree it
	// finds rather than the tree the first one started from, so what it writes
	// is exactly what is still missing.
	if err := os.Remove(blocked); err != nil {
		t.Fatal(err)
	}
	res := w.land(t, th, Local("main"), "")
	if res.Status != Landed {
		t.Fatalf("the second run was %q (%s: %s)", res.Status, res.Condition, res.Detail)
	}
	if got := w.read("sub/spare/zz.txt"); !strings.Contains(got, "the second file") {
		t.Fatalf("the second run did not finish the change: %q", got)
	}
	if want := []string{"sub/spare/zz.txt"}; !equal(res.Arrived, want) {
		t.Fatalf("the second run wrote %v, and only what was missing should have been left: %v", res.Arrived, want)
	}
	if !res.Recorded {
		t.Fatal("the run that finished the change recorded nothing")
	}
}

// TestATreeThatGoesDirtyUnderAPlannedRefArrivalRefuses is the arrival table
// asked again inside the lock.
//
// The gate takes minutes and the table was answered before it started, so the
// row can change under a land: this one is planned as a ref arrival because the
// tree is clean, and the operator starts editing while it runs. Moving the
// branch then would move it out from under a dirty tree, which is the thing the
// row refuses — so the land refuses too, and the ref does not move.
func TestATreeThatGoesDirtyUnderAPlannedRefArrivalRefuses(t *testing.T) {
	w := newLandWorkspace(t)
	th := w.anchor("promise:a-is-whole", func(b string) {
		writeFile(t, filepath.Join(b, "sub/a/note/second.md"), noteFile("second"))
	})
	ref := strings.TrimSpace(w.git("rev-parse", "refs/heads/main"))

	held, err := lock.Acquire(context.Background(), w.root, lock.Land, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	req := w.request(th)
	req.Target, req.Wait = Local("main"), 30*time.Second

	done := make(chan *Result, 1)
	fail := make(chan error, 1)
	go func() {
		res, err := Perform(context.Background(), req)
		if err != nil {
			fail <- err
			return
		}
		done <- res
	}()

	w.waitForGateBox(t)
	w.write("sub/b/note/only.md", noteFile("only")+"\nthe operator started typing")
	if err := held.Release(); err != nil {
		t.Fatal(err)
	}

	var res *Result
	select {
	case res = <-done:
	case err := <-fail:
		t.Fatalf("land: %v", err)
	case <-time.After(4 * time.Minute):
		t.Fatal("the land never finished")
	}

	if res.Plan.Arrival != ledger.RefArrival {
		t.Fatalf("the land was planned as a %s arrival, so this proves nothing", res.Plan.Arrival)
	}
	if res.Status != Refused || res.Condition != Contended {
		t.Fatalf("a tree that went dirty was answered %q · %q: %s", res.Status, res.Condition, res.Detail)
	}
	if now := strings.TrimSpace(w.git("rev-parse", "refs/heads/main")); now != ref {
		t.Fatalf("main moved from %s to %s under a refused land", ref, now)
	}
	if res.Recorded {
		t.Fatal("a refused land recorded an act")
	}
}
