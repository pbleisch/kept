package land

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/lock"
	"github.com/pbleisch/kept/internal/thread"
)

// TestTheInLockCheckReadsTheTrackingRefRatherThanTheLocalBranch is the half of
// docs/decision/0015 that is easiest to implement wrongly and hardest to notice:
//
//	The in-lock check is against origin/main, not local main. A local lock
//	excludes local runs and nothing else — the operator pushing from a laptop,
//	CI, a second machine. Fetching inside the lock is what makes the invariant
//	true rather than merely locally consistent.
//
// Here local main is deliberately somewhere else. A check that compared the
// gate's commit against refs/heads/main would find them different on every
// attempt, report the target as having moved, and never land at all — so a land
// that succeeds while the local branch points elsewhere is a land that did not
// read it.
//
// The premise is asserted rather than assumed. If the two refs ever agreed this
// test would pass while proving nothing, which is the failure mode of every
// guard that is not checked against its own setup.
func TestTheInLockCheckReadsTheTrackingRefRatherThanTheLocalBranch(t *testing.T) {
	w := newLandWorkspace(t)
	th := w.anchor("promise:a-is-whole", func(b string) {
		writeFile(t, filepath.Join(b, "sub/a/note/second.md"), noteFile("second"))
	})

	// Move local main somewhere the target is not, without touching the remote.
	w.write("local-only.txt", "a commit that is on this machine and nowhere else\n")
	w.commit()
	local := strings.TrimSpace(w.git("rev-parse", "refs/heads/main"))
	target := w.remoteHead()
	if local == target {
		t.Fatal("the premise failed: local main and the target are the same commit, so this test proves nothing")
	}

	res, err := Perform(context.Background(), w.request(th))
	if err != nil {
		t.Fatalf("land: %v", err)
	}
	if res.Status != Landed {
		t.Fatalf("status = %q · %q (%s), want %q — a check against refs/heads/main would report the target as moved on every attempt",
			res.Status, res.Condition, res.Detail, Landed)
	}
	if res.GatedAgainst != target {
		t.Fatalf("the gate ran against %s, and the target was %s", res.GatedAgainst, target)
	}
	if len(res.Moved) != 0 {
		t.Fatalf("the target never moved and the land reported %v", res.Moved)
	}
	// Local main is untouched. A land writes the tracking ref and the remote,
	// and never the operator's branch.
	if now := strings.TrimSpace(w.git("rev-parse", "refs/heads/main")); now != local {
		t.Fatalf("the land moved local main from %s to %s", local, now)
	}
}

// TestATargetThatMovedUnderTheLockIsReGatedRatherThanPushedOver is the same rule
// from the other side: the check has to *notice* a move that only the remote
// knows about.
//
// The move is made by a second checkout pushing to the remote, which is exactly
// the run docs/decision/0015 says a local lock cannot exclude. Local main does
// not move, so a check that read it would see nothing, gate against a commit
// that is no longer the tip, and try to push a merge that does not descend from
// it.
//
// It is deterministic rather than timed: this test holds the land lock, so the
// land cannot reach its in-lock fetch until the push has already happened and
// the lock is released.
func TestATargetThatMovedUnderTheLockIsReGatedRatherThanPushedOver(t *testing.T) {
	w := newLandWorkspace(t)
	th := w.anchor("promise:a-is-whole", func(b string) {
		writeFile(t, filepath.Join(b, "sub/a/note/second.md"), noteFile("second"))
	})
	before := w.remoteHead()

	// The land's first act is to fetch, so an absent tracking ref appearing is
	// the signal that the gate has begun and the push below will be seen by the
	// check rather than by the gate.
	w.git("update-ref", "-d", w.target.Tracking())

	held, err := lock.Acquire(context.Background(), w.root, lock.Land, 0)
	if err != nil {
		t.Fatalf("hold the land lock: %v", err)
	}

	req := w.request(th)
	req.Wait = 30 * time.Second
	type outcome struct {
		res *Result
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		res, err := Perform(context.Background(), req)
		done <- outcome{res, err}
	}()

	waitForRef(t, w, w.target.Tracking())
	moved := w.thirdPartyPush(t)
	if moved == before {
		t.Fatal("the premise failed: the second checkout pushed nothing")
	}
	if err := held.Release(); err != nil {
		t.Fatalf("release the land lock: %v", err)
	}

	got := <-done
	if got.err != nil {
		t.Fatalf("land: %v", got.err)
	}
	res := got.res
	if res.Status != Landed {
		t.Fatalf("status = %q · %q (%s), want %q", res.Status, res.Condition, res.Detail, Landed)
	}
	if len(res.Moved) == 0 {
		t.Fatal("the target moved under the lock and the land did not notice; a check against refs/heads/main would see exactly this")
	}
	if res.Moved[0] != moved {
		t.Fatalf("the land saw the target move to %s, and it moved to %s", res.Moved[0], moved)
	}
	if res.Attempts < 2 {
		t.Fatalf("the land ran %d attempts; a target that moved has to be re-gated", res.Attempts)
	}
	if res.GatedAgainst != moved {
		t.Fatalf("the second gate ran against %s, want the commit the target had actually moved to, %s", res.GatedAgainst, moved)
	}
	// What landed descends from where the target actually was, which is the
	// invariant the whole critical section exists for.
	if !w.remoteReaches(t, moved) {
		t.Fatalf("what landed does not contain %s, so it was gated against a tree it did not land on", moved)
	}
	if w.remoteHead() != res.Commit {
		t.Fatalf("%s is at %s and the land reported %s", w.target, w.remoteHead(), res.Commit)
	}
}

// TestALandNeverPushesOverACommitItDidNotGateAgainst is the backstop under the
// lock rather than the lock itself.
//
// The push is unforced, so even with the check bypassed entirely git refuses a
// non-fast-forward. This drives that directly: a merge built on a commit the
// target has already moved past is rejected, and nothing on the remote is lost.
func TestALandNeverPushesOverACommitItDidNotGateAgainst(t *testing.T) {
	w := newLandWorkspace(t)
	stale := w.remoteHead()
	moved := w.thirdPartyPush(t)

	// A commit made on the stale tip, exactly as a land gating against it would
	// have produced, pushed with the same call the land uses.
	w.write("built-on-the-stale-tip.txt", "gated against a commit the target has moved past\n")
	w.git("add", "--all")
	w.git("commit", "--quiet", "--no-gpg-sign", "-m", "stale")
	built := strings.TrimSpace(w.git("rev-parse", "HEAD"))

	err := push(context.Background(), w.request(thread.MintID()), built)
	if err == nil {
		t.Fatal("a push over a commit the land never gated against succeeded; the push must be unforced")
	}
	if got := w.remoteHead(); got != moved {
		t.Fatalf("%s is at %s, and the third party left it at %s", w.target, got, moved)
	}
	if got := w.remoteHead(); got == stale || got == built {
		t.Fatalf("the rejected push moved the target to %s", got)
	}
}

// waitForRef blocks until a ref exists. It is bounded: a ref that never appears
// is a land that never fetched, and hanging the suite would report that as a
// timeout rather than as the failure it is.
func waitForRef(t *testing.T, w *landWorkspace, ref string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(w.root, ".git")); err == nil {
			out, err := gitTry(w.root, "rev-parse", "--verify", "--quiet", ref)
			if err == nil && strings.TrimSpace(out) != "" {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s never appeared, so the land never fetched the target", ref)
}
