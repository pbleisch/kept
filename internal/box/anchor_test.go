package box

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestAnAnchoredCommitSurvivesTheBoxBeingDiscarded is the whole reason anchoring
// exists.
//
// Before it, a repair's work lived only in a worktree that the run removed on
// its way out: the diff was printed and dropped, and nothing durable named the
// bytes. This asserts the opposite — the box is gone, and the tree the session
// produced is still readable by name.
func TestAnAnchoredCommitSurvivesTheBoxBeingDiscarded(t *testing.T) {
	repo := repositoryFor(t)
	b := boxAt(t, repo)

	writeAt(t, filepath.Join(b.Path(), "repaired.txt"), "what the session left\n")
	const ref = "refs/kept/threads/test-anchor"
	commit, err := b.Anchor(context.Background(), ref, "repair promise:x\n", Identity{Name: "Test", Email: "t@example.invalid"})
	if err != nil {
		t.Fatalf("anchor: %v", err)
	}
	if err := b.Close(context.Background()); err != nil {
		t.Fatalf("discard the box: %v", err)
	}
	if _, err := os.Stat(b.Path()); !os.IsNotExist(err) {
		t.Fatalf("the box is still at %s", b.Path())
	}

	// The ref resolves, and the content is there.
	if got := strings.TrimSpace(boxGit(t, repo, "rev-parse", "--verify", ref)); got != commit {
		t.Fatalf("%s is %s, and the anchor is %s", ref, got, commit)
	}
	if got := boxGit(t, repo, "show", commit+":repaired.txt"); got != "what the session left" {
		t.Fatalf("the anchored tree holds %q", got)
	}
}

// TestAnAnchorsDiffAgainstItsParentIsTheChangeAPersonReviewed keeps the anchor
// honest: a land gates what the commit says it changed, and the commit has to
// say the same thing the diff a person read said.
func TestAnAnchorsDiffAgainstItsParentIsTheChangeAPersonReviewed(t *testing.T) {
	repo := repositoryFor(t)
	b := boxAt(t, repo)
	base := b.Commit()

	writeAt(t, filepath.Join(b.Path(), "added.txt"), "new\n")
	writeAt(t, filepath.Join(b.Path(), "seed.txt"), "changed\n")
	if err := os.Remove(filepath.Join(b.Path(), "removed.txt")); err != nil {
		t.Fatal(err)
	}
	change, err := b.Change(context.Background())
	if err != nil {
		t.Fatalf("change: %v", err)
	}

	commit, err := b.Anchor(context.Background(), "refs/kept/threads/test-diff", "repair\n",
		Identity{Name: "Test", Email: "t@example.invalid"})
	if err != nil {
		t.Fatalf("anchor: %v", err)
	}
	names, err := b.Names(context.Background(), base, commit)
	if err != nil {
		t.Fatalf("names: %v", err)
	}
	if strings.Join(names, "|") != strings.Join(change.Paths, "|") {
		t.Fatalf("the commit changed %v and the reviewed change was %v", names, change.Paths)
	}
	// A delete is in it, which is the one a diff-text reading would drop.
	if !contains(names, "removed.txt") {
		t.Fatalf("the deleted path is missing from %v", names)
	}
	// The parent is what the box was cut from, so the diff is the repair alone.
	if parent := strings.TrimSpace(boxGit(t, repo, "rev-parse", commit+"^")); parent != base {
		t.Fatalf("the anchor's parent is %s, and the box was cut at %s", parent, base)
	}
}

// TestAnAnchorIsAuthoredByAPersonAndNeverByATool is AGENTS.md's rule at the one
// place kept writes a commit that can reach a branch: an agent produced the
// diff, and a person is accountable for it.
func TestAnAnchorIsAuthoredByAPersonAndNeverByATool(t *testing.T) {
	repo := repositoryFor(t)
	b := boxAt(t, repo)
	defer func() { _ = b.Close(context.Background()) }()
	writeAt(t, filepath.Join(b.Path(), "repaired.txt"), "x\n")

	if _, err := b.Anchor(context.Background(), "refs/kept/threads/t", "repair\n", Identity{}); err == nil {
		t.Fatal("an anchor was written with nobody named as its author")
	}
	if _, err := b.Anchor(context.Background(), "refs/kept/threads/t", "repair\n", Identity{Name: "Only"}); err == nil {
		t.Fatal("an anchor was written with half an identity")
	}

	commit, err := b.Anchor(context.Background(), "refs/kept/threads/t", "repair\n",
		Identity{Name: "Paul", Email: "p@example.invalid"})
	if err != nil {
		t.Fatalf("anchor: %v", err)
	}
	who := strings.TrimSpace(boxGit(t, repo, "show", "--no-patch", "--format=%an <%ae>|%cn <%ce>", commit))
	if who != "Paul <p@example.invalid>|Paul <p@example.invalid>" {
		t.Fatalf("the anchor is authored %q", who)
	}
	for _, name := range []string{"kept", "claude", "agent", "bot"} {
		if strings.Contains(strings.ToLower(who), name) {
			t.Fatalf("the anchor credits a tool: %q", who)
		}
	}
}

// TestAnAnchorRefusesANameThatIsNotAFullRef: the ref becomes an argument to
// update-ref, and a partial name would let git resolve it somewhere else — a
// branch, most likely, which is the one place an unlanded repair may not be.
func TestAnAnchorRefusesANameThatIsNotAFullRef(t *testing.T) {
	repo := repositoryFor(t)
	b := boxAt(t, repo)
	defer func() { _ = b.Close(context.Background()) }()
	writeAt(t, filepath.Join(b.Path(), "repaired.txt"), "x\n")

	who := Identity{Name: "Test", Email: "t@example.invalid"}
	for _, ref := range []string{"", "main", "heads/main", "-delete"} {
		if _, err := b.Anchor(context.Background(), ref, "repair\n", who); err == nil {
			t.Fatalf("an anchor was written at %q", ref)
		}
	}
	if _, err := b.Anchor(context.Background(), "refs/kept/threads/t", "   ", who); err == nil {
		t.Fatal("an anchor was written with an empty message")
	}
}

// TestAnUnchangedBoxIsReportedAsUnchanged is what keeps a session that did
// nothing from anchoring an empty commit for a land to gate.
func TestAnUnchangedBoxIsReportedAsUnchanged(t *testing.T) {
	repo := repositoryFor(t)
	b := boxAt(t, repo)
	defer func() { _ = b.Close(context.Background()) }()

	unchanged, err := b.Unchanged(context.Background())
	if err != nil {
		t.Fatalf("unchanged: %v", err)
	}
	if !unchanged {
		t.Fatal("a box nothing touched reports as changed")
	}
	writeAt(t, filepath.Join(b.Path(), "touched.txt"), "x\n")
	if unchanged, err = b.Unchanged(context.Background()); err != nil {
		t.Fatalf("unchanged: %v", err)
	} else if unchanged {
		t.Fatal("a box a session wrote in reports as unchanged")
	}
}

// TestAMergeBringsTheChangeIntoTheBoxAndMovesItsBaseline: the gate measures the
// tree that would exist if this landed, so the box has to contain the merge.
func TestAMergeBringsTheChangeIntoTheBoxAndMovesItsBaseline(t *testing.T) {
	repo := repositoryFor(t)
	who := Identity{Name: "Test", Email: "t@example.invalid"}

	// One box produces the change, as a repair does.
	work := boxAt(t, repo)
	base := work.Commit()
	writeAt(t, filepath.Join(work.Path(), "repaired.txt"), "the repair\n")
	anchored, err := work.Anchor(context.Background(), "refs/kept/threads/merge-test", "repair\n", who)
	if err != nil {
		t.Fatalf("anchor: %v", err)
	}
	if err := work.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Another gates it.
	gate := boxAt(t, repo)
	defer func() { _ = gate.Close(context.Background()) }()
	target := gate.Commit()
	merged, err := gate.Merge(context.Background(), base, anchored, "land\n", who)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(gate.Path(), "repaired.txt")); err != nil {
		t.Fatalf("the merge did not bring the change into the box: %v", err)
	} else if string(got) != "the repair\n" {
		t.Fatalf("the box holds %q", got)
	}
	// One parent, the target's tip. The anchor is never a parent of what lands:
	// a parent is reachable and the base is private. See docs/decision/0068.
	parents := strings.Fields(strings.TrimSpace(boxGit(t, repo, "rev-list", "--parents", "-n", "1", merged)))
	if len(parents) != 2 || parents[1] != target {
		t.Fatalf("the merge's parents are %v, want %s alone", parents[1:], target)
	}
	if reachable(t, repo, anchored, merged) {
		t.Fatalf("the anchor %s is reachable from what landed", anchored)
	}
	// The baseline moved with it: the box has not "changed" relative to the
	// merge it now holds.
	if unchanged, err := gate.Unchanged(context.Background()); err != nil {
		t.Fatal(err)
	} else if !unchanged {
		t.Fatal("after a merge the box reports itself changed against the tree it started from")
	}
	// And what the gate is about to measure is what would land: the merge is
	// computed over trees and then brought into the box, so a box holding
	// anything the commit does not would be a gate over a tree nobody gets.
	if dirty := strings.TrimSpace(boxGit(t, gate.Path(), "status", "--porcelain")); dirty != "" {
		t.Fatalf("the box holds what the merge commit does not:\n%s", dirty)
	}
}

// TestAMergeThatConflictsIsRefusedAndLeavesTheBoxWhereItWas: a conflict is not
// a promise failing — nothing was measured — and a caller that reports it must
// be able to go on using the box.
//
// The merge is computed over trees, so there is no worktree state to leave
// behind: MERGE_HEAD is asserted absent because "nothing to abort" is the
// property, not because an abort ran.
func TestAMergeThatConflictsIsRefusedAndLeavesTheBoxWhereItWas(t *testing.T) {
	repo := repositoryFor(t)
	who := Identity{Name: "Test", Email: "t@example.invalid"}

	work := boxAt(t, repo)
	base := work.Commit()
	writeAt(t, filepath.Join(work.Path(), "seed.txt"), "one side\n")
	anchored, err := work.Anchor(context.Background(), "refs/kept/threads/conflict", "repair\n", who)
	if err != nil {
		t.Fatal(err)
	}
	if err := work.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Move the target's own copy of the same file the other way.
	writeAt(t, filepath.Join(repo, "seed.txt"), "the other side\n")
	boxGit(t, repo, "add", "--all")
	boxGit(t, repo, "commit", "--quiet", "--no-gpg-sign", "-m", "diverge")
	moved := strings.TrimSpace(boxGit(t, repo, "rev-parse", "HEAD"))

	gate, err := Create(context.Background(), Request{Repository: repo, Commit: moved, Deadline: 30 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = gate.Close(context.Background()) }()

	if _, err := gate.Merge(context.Background(), base, anchored, "land\n", who); err == nil {
		t.Fatal("a conflicting merge succeeded")
	} else if !strings.Contains(err.Error(), "the merge conflicts") {
		t.Fatalf("a conflict is not reported as one: %v", err)
	} else if !strings.Contains(err.Error(), "seed.txt") {
		t.Fatalf("the conflict does not say what a person has to settle: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, ".git", "worktrees")); err != nil {
		t.Fatal(err)
	}
	if got := gate.Commit(); got != moved {
		t.Fatalf("a conflicting merge moved the box from %s to %s", moved, got)
	}
	if out, err := exec.Command("git", "-C", gate.Path(), "rev-parse", "--verify", "--quiet", "MERGE_HEAD").CombinedOutput(); err == nil && strings.TrimSpace(string(out)) != "" {
		t.Fatalf("the box is left mid-merge at %s", strings.TrimSpace(string(out)))
	}
}

// TestAMergeTakesTheChangeAndNotTheAncestryItWasMadeOn is docs/decision/0068's
// mechanism, with the thing it prevents measured beside it.
//
// The anchor sits on a base the target has never seen — an operator's unlanded
// commit here, and a snapshot of their uncommitted tree in the case a dirty
// repair produces. Stating that base merges the change alone. Letting git find
// the base merges everything between the target and the anchor, which is the
// second half of this test: it computes that merge too, and shows the private
// commit's file in the result.
func TestAMergeTakesTheChangeAndNotTheAncestryItWasMadeOn(t *testing.T) {
	repo := repositoryFor(t)
	who := Identity{Name: "Test", Email: "t@example.invalid"}
	target := strings.TrimSpace(boxGit(t, repo, "rev-parse", "HEAD"))

	// The base: a commit on no branch that the target does not have.
	private := boxAt(t, repo)
	writeAt(t, filepath.Join(private.Path(), "private.txt"), "the operator has not landed this\n")
	base, err := private.Anchor(context.Background(), "refs/kept/threads/private", "not landed\n", who)
	if err != nil {
		t.Fatal(err)
	}
	if err := private.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	// The change, made on that base.
	work, err := Create(context.Background(), Request{Repository: repo, Commit: base, Deadline: 30 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	writeAt(t, filepath.Join(work.Path(), "repaired.txt"), "the repair\n")
	anchored, err := work.Anchor(context.Background(), "refs/kept/threads/change", "repair\n", who)
	if err != nil {
		t.Fatal(err)
	}
	if err := work.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	gate := boxAt(t, repo)
	defer func() { _ = gate.Close(context.Background()) }()
	merged, err := gate.Merge(context.Background(), base, anchored, "land\n", who)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if !holds(t, repo, merged, "repaired.txt") {
		t.Fatal("the change did not land")
	}
	if holds(t, repo, merged, "private.txt") {
		t.Fatal("the operator's unlanded commit rode onto the target inside the repair")
	}

	// The same merge with the base git would compute, which is what this
	// replaces. If it stopped carrying the private file the assertion above
	// would be measuring nothing.
	computed := strings.TrimSpace(boxGit(t, gate.Path(), "merge-tree", "--write-tree", "--", target, anchored))
	if !strings.Contains(boxGit(t, repo, "ls-tree", "--name-only", computed), "private.txt") {
		t.Fatal("the premise failed: a merge whose base git computed left the private commit out too, " +
			"so stating the base is not what keeps it out")
	}
}

// TestAMergeThatChangesNothingCommitsNothing: the three-way merge is
// idempotent, so landing the same anchor twice computes the same base against a
// target that already holds the change. An empty commit pushed onto the target
// would be a duplicate of something already there, so the box's own commit
// comes back instead.
func TestAMergeThatChangesNothingCommitsNothing(t *testing.T) {
	repo := repositoryFor(t)
	who := Identity{Name: "Test", Email: "t@example.invalid"}

	work := boxAt(t, repo)
	base := work.Commit()
	writeAt(t, filepath.Join(work.Path(), "repaired.txt"), "the repair\n")
	anchored, err := work.Anchor(context.Background(), "refs/kept/threads/twice", "repair\n", who)
	if err != nil {
		t.Fatal(err)
	}
	if err := work.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	gate := boxAt(t, repo)
	defer func() { _ = gate.Close(context.Background()) }()
	first, err := gate.Merge(context.Background(), base, anchored, "land\n", who)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	second, err := gate.Merge(context.Background(), base, anchored, "land again\n", who)
	if err != nil {
		t.Fatalf("merge the same change again: %v", err)
	}
	if second != first {
		t.Fatalf("landing the same change twice wrote %s on top of %s", second, first)
	}
}

func holds(t *testing.T, repo, commit, path string) bool {
	t.Helper()
	err := exec.Command("git", "-C", repo, "cat-file", "-e", commit+":"+path).Run()
	return err == nil
}

func reachable(t *testing.T, repo, commit, from string) bool {
	t.Helper()
	err := exec.Command("git", "-C", repo, "merge-base", "--is-ancestor", commit, from).Run()
	return err == nil
}

func repositoryFor(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	boxGit(t, resolved, "init", "--quiet", "--initial-branch=main", ".")
	boxGit(t, resolved, "config", "user.email", "seed@example.invalid")
	boxGit(t, resolved, "config", "user.name", "Seed")
	writeAt(t, filepath.Join(resolved, "seed.txt"), "seed\n")
	writeAt(t, filepath.Join(resolved, "removed.txt"), "gone soon\n")
	boxGit(t, resolved, "add", "--all")
	boxGit(t, resolved, "commit", "--quiet", "--no-gpg-sign", "-m", "seed")
	return resolved
}

func boxAt(t *testing.T, repo string) *Box {
	t.Helper()
	head := strings.TrimSpace(boxGit(t, repo, "rev-parse", "HEAD"))
	b, err := Create(context.Background(), Request{Repository: repo, Commit: head, Deadline: 30 * time.Second})
	if err != nil {
		t.Fatalf("create a box: %v", err)
	}
	t.Cleanup(func() { _ = b.Close(context.Background()) })
	return b
}

func writeAt(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func contains(all []string, want string) bool {
	for _, v := range all {
		if v == want {
			return true
		}
	}
	return false
}

// TestAReworksAnchorIsTheWholeEpisodesChangeAgainstItsBase is the anchor of a
// box cut at an earlier anchor. The pair a land takes is the commit and its
// first parent, so the first parent is the episode's base and the diff is every
// attempt's work together; the second parent keeps the earlier anchor reachable.
// See docs/decision/0086.
func TestAReworksAnchorIsTheWholeEpisodesChangeAgainstItsBase(t *testing.T) {
	repo := repositoryFor(t)
	base := strings.TrimSpace(boxGit(t, repo, "rev-parse", "HEAD"))
	who := Identity{Name: "Test", Email: "t@example.invalid"}

	first := boxAt(t, repo)
	writeAt(t, filepath.Join(first.Path(), "first.txt"), "attempt one\n")
	earlier, err := first.Anchor(context.Background(), "refs/kept/threads/test-episode", "repair\n", who)
	if err != nil {
		t.Fatalf("anchor the first attempt: %v", err)
	}

	second, err := Create(context.Background(), Request{Repository: repo, Commit: earlier, Deadline: 30 * time.Second})
	if err != nil {
		t.Fatalf("cut the rework's box at the earlier anchor: %v", err)
	}
	t.Cleanup(func() { _ = second.Close(context.Background()) })
	writeAt(t, filepath.Join(second.Path(), "second.txt"), "attempt two\n")
	commit, err := second.AnchorOnto(context.Background(), "refs/kept/threads/test-episode", "rework\n", who, base)
	if err != nil {
		t.Fatalf("anchor the rework: %v", err)
	}

	if got := strings.TrimSpace(boxGit(t, repo, "rev-parse", commit+"^1")); got != base {
		t.Fatalf("the rework's first parent is %s, want the episode's base %s", got, base)
	}
	if got := strings.TrimSpace(boxGit(t, repo, "rev-parse", commit+"^2")); got != earlier {
		t.Fatalf("the rework's second parent is %s, want the earlier anchor %s", got, earlier)
	}
	names, err := second.Names(context.Background(), base, commit)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(names, "first.txt") || !contains(names, "second.txt") {
		t.Fatalf("the change against the base is %v, want both attempts' work", names)
	}
	// And the rework's own change, against the box it was cut at, is its alone.
	own, err := second.Names(context.Background(), earlier, commit)
	if err != nil {
		t.Fatal(err)
	}
	if contains(own, "first.txt") || !contains(own, "second.txt") {
		t.Fatalf("the rework's own change is %v", own)
	}

	if _, err := second.AnchorOnto(context.Background(), "refs/kept/threads/test-episode", "rework\n", who, "HEAD"); err == nil {
		t.Fatal("an anchor was based on a name rather than a commit")
	}
}
