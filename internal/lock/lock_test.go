package lock

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTwoWorktreesOfOneRepositoryShareOneLockFile(t *testing.T) {
	// The invariant, and the reason the lock is where it is: a lock beside one
	// worktree excludes nobody. Both paths are resolved through git rather than
	// composed here, because composing them is what this is meant to catch.
	repository := repositoryWithWorktree(t)
	primary, linked := repository.root, repository.worktree

	first, err := Path(primary, Land)
	if err != nil {
		t.Fatalf("Path from the primary checkout: %v", err)
	}
	second, err := Path(linked, Land)
	if err != nil {
		t.Fatalf("Path from the linked worktree: %v", err)
	}
	if first != second {
		t.Fatalf("two worktrees of one repository resolve to two lock files:\n  %s\n  %s", first, second)
	}
	if !strings.HasPrefix(first, repository.common) {
		t.Fatalf("the lock is at %s, which is not under the shared common directory %s", first, repository.common)
	}
}

func TestALockExcludesASecondRunOfTheSameRepository(t *testing.T) {
	repository := repositoryWithWorktree(t)

	held, err := Acquire(context.Background(), repository.root, Land, 0)
	if err != nil {
		t.Fatalf("acquire from the primary checkout: %v", err)
	}
	defer func() { _ = held.Release() }()

	// From the *other* worktree, which is the run a local lock beside a
	// worktree would fail to exclude.
	second, err := Acquire(context.Background(), repository.worktree, Land, 50*time.Millisecond)
	if err == nil {
		_ = second.Release()
		t.Fatal("a second run acquired a lock the first run holds")
	}
	if !errors.Is(err, ErrRefused) {
		t.Fatalf("a contended lock returned %v, want ErrRefused", err)
	}
}

func TestReleasingALockLetsTheNextRunTakeIt(t *testing.T) {
	repository := repositoryWithWorktree(t)

	held, err := Acquire(context.Background(), repository.root, Land, 0)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if err := held.Release(); err != nil {
		t.Fatalf("release: %v", err)
	}

	next, err := Acquire(context.Background(), repository.worktree, Land, time.Second)
	if err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	if err := next.Release(); err != nil {
		t.Fatalf("release the second: %v", err)
	}
	// Idempotent: a caller that releases in a defer and on the happy path must
	// not turn the second call into a failure.
	if err := next.Release(); err != nil {
		t.Fatalf("releasing twice: %v", err)
	}
}

func TestAWaitingAcquirerTakesTheLockWhenTheHolderReleases(t *testing.T) {
	repository := repositoryWithWorktree(t)

	held, err := Acquire(context.Background(), repository.root, Land, 0)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	go func() {
		time.Sleep(100 * time.Millisecond)
		_ = held.Release()
	}()

	next, err := Acquire(context.Background(), repository.worktree, Land, 5*time.Second)
	if err != nil {
		t.Fatalf("a waiting acquirer never got the lock: %v", err)
	}
	if err := next.Release(); err != nil {
		t.Fatalf("release: %v", err)
	}
}

func TestACanceledAcquisitionReportsTheCancellationRatherThanARefusal(t *testing.T) {
	// A caller that gave up is not a caller that lost the lock, and reporting
	// the second as the first would make a canceled run look like contention.
	repository := repositoryWithWorktree(t)
	held, err := Acquire(context.Background(), repository.root, Land, 0)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer func() { _ = held.Release() }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Acquire(ctx, repository.worktree, Land, time.Minute); !errors.Is(err, context.Canceled) {
		t.Fatalf("a canceled acquisition returned %v, want context.Canceled", err)
	}
}

func TestALockNameThatWouldEscapeTheLockDirectoryIsRefused(t *testing.T) {
	repository := repositoryWithWorktree(t)
	for _, name := range []string{"", ".", "..", "a/b", `a\b`, "a\x00b"} {
		if _, err := Path(repository.root, name); err == nil {
			t.Fatalf("Path accepted %q as a lock name", name)
		}
	}
}

func TestAPathOutsideARepositoryIsRefusedRatherThanGuessed(t *testing.T) {
	if _, err := Path(t.TempDir(), Land); err == nil {
		t.Fatal("Path invented a lock location outside a repository")
	}
	if _, err := Path("", Land); err == nil {
		t.Fatal("Path accepted an unset repository")
	}
}

type worktreePair struct {
	root     string
	worktree string
	common   string
}

// repositoryWithWorktree builds a repository and a second worktree of it, which
// is the only arrangement in which the shared-common-directory property can be
// observed at all.
func repositoryWithWorktree(t *testing.T) worktreePair {
	t.Helper()
	// Two temporary roots rather than one: a worktree registered inside the
	// repository it belongs to would be removed by the repository's own
	// cleanup, and the order of two t.TempDir cleanups is not the question
	// under test.
	root := t.TempDir()
	git(t, root, "init", "--quiet", "--initial-branch=main", ".")
	git(t, root, "config", "user.email", "test@example.invalid")
	git(t, root, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	git(t, root, "add", "a.txt")
	git(t, root, "commit", "--quiet", "--no-gpg-sign", "-m", "one")

	linked := filepath.Join(t.TempDir(), "linked")
	git(t, root, "worktree", "add", "--detach", "--quiet", linked)
	t.Cleanup(func() {
		_, _ = exec.Command("git", "-C", root, "worktree", "remove", "--force", linked).CombinedOutput()
	})

	common := strings.TrimSpace(git(t, root, "rev-parse", "--git-common-dir"))
	if !filepath.IsAbs(common) {
		common = filepath.Join(root, common)
	}
	if resolved, err := filepath.EvalSymlinks(common); err == nil {
		common = resolved
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("resolve %s: %v", root, err)
	}
	return worktreePair{root: resolvedRoot, worktree: linked, common: common}
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}
