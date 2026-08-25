package ledger

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const someTree = "sha256:" + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// A workspace that is not a repository still has a world: the tree is what
// makes one world one world, and remote, ref and commit are provenance filled
// in from git where git has something to say. See docs/decision/0033.
func TestSituateOutsideARepositoryNamesTheTreeAndNoHistory(t *testing.T) {
	where, err := Situate(t.TempDir(), someTree)
	if !errors.Is(err, ErrNotRepository) {
		t.Fatalf("Situate outside a repository = %v, want ErrNotRepository", err)
	}
	if !where.Situated() || where.World.Tree != someTree {
		t.Errorf("the tree did not survive: %+v", where.World)
	}
	if where.World.Commit != "" || where.World.Ref != "" || where.World.Remote != "" || where.Uncommitted {
		t.Errorf("a directory that is not a repository reported a history: %+v", where)
	}
}

// The ordinary case, and every field of it. A world with a commit is what the
// reachability check in doctor has to work with, so a commit read wrong is a
// finding read wrong.
func TestSituateNamesTheCommitTheRefAndTheRemote(t *testing.T) {
	work := situateLab(t)
	fetchGit(t, work, "remote", "add", "--no-tags", "origin", "git@example.invalid:nobody/nothing.git")
	head := commitAll(t, work, "one.txt", "first")

	where, err := Situate(work, someTree)
	if err != nil {
		t.Fatal(err)
	}
	if where.World.Commit != head {
		t.Errorf("the commit is %q, and HEAD is %q", where.World.Commit, head)
	}
	if !strings.HasPrefix(where.World.Ref, "refs/heads/") {
		t.Errorf("the ref is %q, and a checked-out branch is under refs/heads/", where.World.Ref)
	}
	if where.World.Remote != "git@example.invalid:nobody/nothing.git" {
		t.Errorf("the remote is %q", where.World.Remote)
	}
	if where.Uncommitted {
		t.Error("a tree with nothing in it but a commit reported as uncommitted")
	}
	if where.World.Thread != "" {
		t.Errorf("a workspace named a thread: %q", where.World.Thread)
	}
}

// The case docs/decision/0063 will not let read as supported: content hashed
// out of a tree the commit does not hold. Both shapes count, and the untracked
// one is the one that matters — a decision authored and ratified before it is
// added is in no commit at all, and git diff would not have seen it.
func TestSituateReportsATreeTheCommitDoesNotHold(t *testing.T) {
	for _, dirty := range []struct {
		name string
		make func(t *testing.T, work string)
	}{
		{"a modified file", func(t *testing.T, work string) { write(t, work, "one.txt", "edited") }},
		{"an untracked file", func(t *testing.T, work string) { write(t, work, "two.txt", "never added") }},
	} {
		t.Run(dirty.name, func(t *testing.T) {
			work := situateLab(t)
			commitAll(t, work, "one.txt", "first")

			where, err := Situate(work, someTree)
			if err != nil || where.Uncommitted {
				t.Fatalf("a clean tree reported %+v, %v", where, err)
			}
			dirty.make(t, work)

			where, err = Situate(work, someTree)
			if err != nil {
				t.Fatal(err)
			}
			if !where.Uncommitted {
				t.Errorf("%s did not make the tree uncommitted: %+v", dirty.name, where)
			}
			if where.World.Commit == "" {
				t.Error("a dirty tree lost the commit it is dirty against")
			}
		})
	}
}

// An unborn branch has a ref and no commit. Everything in it is uncommitted,
// which the absent commit already says, so saying it twice would make one
// situation read as two.
func TestSituateOnAnUnbornBranchNamesNoCommitAndNoDirtiness(t *testing.T) {
	work := situateLab(t)
	write(t, work, "one.txt", "not added")

	where, err := Situate(work, someTree)
	if err != nil {
		t.Fatal(err)
	}
	if where.World.Commit != "" || where.Uncommitted {
		t.Errorf("a repository with no commit reported %+v", where)
	}
	if !where.Situated() {
		t.Error("a repository with no commit lost its tree")
	}
}

// Reachability is the discriminator docs/decision/0063 rests on, so each of its
// three answers gets asked for. Equality would be useless — every act is
// against a tree this is not the moment anything lands — and the three are
// genuinely different: an ancestor landed, a sibling did not, and an object
// nobody here has cannot have.
func TestReachesSeparatesAnAncestorFromABranchThisTreeNeverTook(t *testing.T) {
	work := situateLab(t)
	first := commitAll(t, work, "one.txt", "first")
	fetchGit(t, work, "checkout", "-q", "-b", "aside")
	aside := commitAll(t, work, "two.txt", "on a branch")
	fetchGit(t, work, "checkout", "-q", "-")
	later := commitAll(t, work, "three.txt", "and on")

	absent := strings.Repeat("f", 40)
	reach := Reaches(work, []string{first, later, aside, absent, "not-an-object-id"})

	for commit, want := range map[string]Reach{
		first:              Reachable,
		later:              Reachable,
		aside:              Unreachable,
		absent:             Unreachable,
		"not-an-object-id": ReachUnknown,
	} {
		if got := reach[commit]; got != want {
			t.Errorf("%s is %q, want %q", commit, got, want)
		}
	}
}

// Nothing established is not an answer of no. A directory that is not a
// repository, and one with no commit to ask from, both say so — because
// *nobody could ask* being read as *it never landed* is the finding firing on
// every workspace a manuscript author has.
func TestReachesEstablishesNothingWithoutAHeadToAskFrom(t *testing.T) {
	commit := strings.Repeat("a", 40)
	for name, repository := range map[string]string{
		"not a repository":      t.TempDir(),
		"no commit to ask from": situateLab(t),
	} {
		if got := Reaches(repository, []string{commit})[commit]; got != ReachUnknown {
			t.Errorf("%s answered %q, want %q", name, got, ReachUnknown)
		}
	}
}

func situateLab(t *testing.T) string {
	t.Helper()
	work := t.TempDir()
	fetchGit(t, work, "init", "-q")
	fetchGit(t, work, "config", "user.email", "test@example.com")
	fetchGit(t, work, "config", "user.name", "test")
	return work
}

func write(t *testing.T, work, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(work, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func commitAll(t *testing.T, work, name, body string) string {
	t.Helper()
	write(t, work, name, body)
	fetchGit(t, work, "add", "-A")
	fetchGit(t, work, "commit", "-q", "-m", name)
	code, out := runGit(t, work, "rev-parse", "HEAD")
	if code != 0 {
		t.Fatalf("rev-parse exited %d: %s", code, out)
	}
	return strings.TrimSpace(out)
}
