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
	merged, err := gate.Merge(context.Background(), anchored, "land\n", who)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(gate.Path(), "repaired.txt")); err != nil {
		t.Fatalf("the merge did not bring the change into the box: %v", err)
	} else if string(got) != "the repair\n" {
		t.Fatalf("the box holds %q", got)
	}
	// Both parents, so what landed and what it landed on are both readable.
	parents := strings.Fields(strings.TrimSpace(boxGit(t, repo, "rev-list", "--parents", "-n", "1", merged)))
	if len(parents) != 3 || parents[1] != target || parents[2] != anchored {
		t.Fatalf("the merge's parents are %v, want %s and %s", parents[1:], target, anchored)
	}
	// The baseline moved with it: the box has not "changed" relative to the
	// merge it now holds.
	if unchanged, err := gate.Unchanged(context.Background()); err != nil {
		t.Fatal(err)
	} else if !unchanged {
		t.Fatal("after a merge the box reports itself changed against the tree it started from")
	}
}

// TestAMergeThatConflictsIsRefusedAndLeavesNoMergeInProgress: a conflict is not
// a promise failing — nothing was measured — and a caller that reports it must
// not be left with a worktree mid-merge.
func TestAMergeThatConflictsIsRefusedAndLeavesNoMergeInProgress(t *testing.T) {
	repo := repositoryFor(t)
	who := Identity{Name: "Test", Email: "t@example.invalid"}

	work := boxAt(t, repo)
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

	if _, err := gate.Merge(context.Background(), anchored, "land\n", who); err == nil {
		t.Fatal("a conflicting merge succeeded")
	} else if !strings.Contains(err.Error(), "the merge conflicts") {
		t.Fatalf("a conflict is not reported as one: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, ".git", "worktrees")); err != nil {
		t.Fatal(err)
	}
	// MERGE_HEAD gone: the merge was aborted, so the box is usable again.
	if out, err := exec.Command("git", "-C", gate.Path(), "rev-parse", "--verify", "--quiet", "MERGE_HEAD").CombinedOutput(); err == nil && strings.TrimSpace(string(out)) != "" {
		t.Fatalf("the box is left mid-merge at %s", strings.TrimSpace(string(out)))
	}
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
