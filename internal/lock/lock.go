// Package lock is the one place kept excludes another run.
//
// It is `flock` on a path under the shared Git common directory, which every
// worktree of a repository can see and none of them track. That location is the
// whole mechanism: a lock beside one worktree excludes nobody, and a repository
// is the thing two runs actually contend over.
//
// Exclusion goes through one interface — acquire, and a refusal a caller
// handles on the normal path — because calling `flock` inline at call sites
// encodes the assumption in every caller, and a deployment with no shared lock
// then becomes a rewrite rather than a configuration. See docs/decision/0013.
//
// It is a critical section rather than a lease. `flock` lives on a file
// descriptor, so the kernel releases it when the holding process dies: a
// crashed holder needs no TTL, no reclaim and no takeover. The honest cost is
// the other direction — a holder that is alive and wedged blocks indefinitely —
// and that is handled by giving up after a wait rather than by a timer nobody
// can see. See docs/decision/0015.
package lock

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Land is the section docs/decision/0015 puts around a land: fetch, check that
// origin/main has not moved, merge, push. It is repository-wide rather than per
// artifact, because what it serializes is the read-modify-write over one ref
// and not the work on one promise.
const Land = "land"

// Mint is the section around choosing a number for a new artifact: read the
// highest one a kind has, and write the file that takes the next. It is
// repository-wide rather than per kind, because what it serializes is the
// read-then-write over the corpus and two kinds cannot contend for one number
// anyway.
//
// It is the one lock that prevents rather than reports. Everywhere else losing
// a lock costs duplicated work and never a wrong outcome (0013); here two runs
// reading one highest number both mint it, and the result is two artifacts a
// person then has to renumber. See docs/decision/0069.
const Mint = "mint"

// DefaultDeadline bounds resolving the repository's common directory. Nothing
// external runs without one, and locating a directory is a Git call like any
// other. See docs/decision/0026.
const DefaultDeadline = 30 * time.Second

// poll is how often a waiting acquirer re-asks. The section this guards is held
// for seconds, so a wait is short and a busy interval would spend more on
// asking than on waiting.
const poll = 25 * time.Millisecond

// ErrRefused means another run holds this lock.
//
// It is an ordinary outcome rather than an error, and callers handle it on the
// normal path: losing a lock costs a duplicated repair and never a wrong
// outcome, which is what keeps a lockless deployment a configuration change.
// The land is the exception docs/decision/0013 names — there, losing it means
// waiting rather than proceeding.
var ErrRefused = errors.New("another run holds this lock")

// Held is one acquired lock. It owns the open file whose descriptor carries the
// flock, so releasing it is closing that file and nothing else.
type Held struct {
	path string
	file *os.File
}

// Path is where a named lock lives for this repository: under the shared Git
// common directory, so every worktree resolves to one file.
func Path(repository, name string) (string, error) {
	if err := validName(name); err != nil {
		return "", err
	}
	common, err := CommonDir(repository)
	if err != nil {
		return "", err
	}
	return filepath.Join(common, "kept", "locks", name+".lock"), nil
}

// CommonDir is the repository's shared Git common directory, which every
// worktree of it can see and none of them track. It is where kept keeps what
// all of a repository's worktrees must share: these locks, and a receipt's
// journal (docs/decision/0087).
//
// A linked worktree's common directory is reported absolute and the primary
// checkout's is reported relative, so the answer is joined against the
// repository before it is used. Reading only the first is how a lock ends up
// beside one worktree, excluding nobody.
func CommonDir(repository string) (string, error) {
	if repository == "" {
		return "", errors.New("a lock names no repository")
	}
	ctx, cancel := context.WithTimeout(context.Background(), DefaultDeadline)
	defer cancel()

	command := exec.CommandContext(ctx, "git", "-C", repository, "rev-parse", "--git-common-dir")
	var stdout, stderr strings.Builder
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("locate the lock directory in %s: %w", repository, ctx.Err())
		}
		return "", fmt.Errorf("locate the lock directory in %s: %w: %s", repository, err, strings.TrimSpace(stderr.String()))
	}
	common := strings.TrimSpace(stdout.String())
	if common == "" {
		return "", fmt.Errorf("%s reports no Git common directory", repository)
	}
	if !filepath.IsAbs(common) {
		common = filepath.Join(repository, common)
	}
	// Symlinks are resolved so two worktrees reached by different paths do not
	// produce two lock files for one repository. A common directory that does
	// not resolve is used as it stands: the directory is created by its user.
	if resolved, err := filepath.EvalSymlinks(common); err == nil {
		common = resolved
	}
	return common, nil
}

// TryFile takes an exclusive lock on a file the caller opened, without
// waiting, and reports whether it got one. The lock is released when the file
// is closed or the process dies.
//
// It is for a lock that lives on the file it guards rather than on a named
// lock beside it: a receipt's journal is locked by the run writing it, so the
// next run can tell a journal whose writer died from one still being written
// (docs/decision/0087). A caller that unlinks such a file must do so while
// holding the lock, and a caller that takes the lock must check that the path
// still names the file it opened, or it may hold a lock on an inode nobody
// else will ever open.
func TryFile(file *os.File) (bool, error) { return tryLock(file) }

// Acquire takes the named lock for this repository, waiting up to wait for a
// holder to release it.
//
// It returns [ErrRefused] rather than an error when the wait runs out, because
// a lock somebody else holds is not a malfunction — and separating the two is
// what lets a caller retry one and report the other.
func Acquire(ctx context.Context, repository, name string, wait time.Duration) (*Held, error) {
	if ctx == nil {
		return nil, errors.New("a lock acquisition has no context")
	}
	path, err := Path(repository, name)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create the lock directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open the lock at %s: %w", path, err)
	}

	deadline := time.Now().Add(wait)
	for {
		taken, err := tryLock(file)
		if err != nil {
			_ = file.Close()
			return nil, fmt.Errorf("lock %s: %w", path, err)
		}
		if taken {
			return &Held{path: path, file: file}, nil
		}
		if ctx.Err() != nil {
			_ = file.Close()
			return nil, ctx.Err()
		}
		if !time.Now().Before(deadline) {
			_ = file.Close()
			return nil, fmt.Errorf("%w: %s", ErrRefused, path)
		}
		select {
		case <-ctx.Done():
			_ = file.Close()
			return nil, ctx.Err()
		case <-time.After(poll):
		}
	}
}

// Path is the file this lock is held on, for a report that says where.
func (h *Held) Path() string { return h.path }

// Release drops the lock. It is idempotent, and closing the file is what
// releases the flock — there is no separate unlock to get wrong, and no state
// left behind if this is never reached.
func (h *Held) Release() error {
	if h == nil || h.file == nil {
		return nil
	}
	file := h.file
	h.file = nil
	// The lock file itself is left in place. Unlinking it races: another run
	// that already opened this path would hold a flock on an unlinked inode and
	// exclude nobody, which is the one failure a lock must not have.
	if err := file.Close(); err != nil {
		return fmt.Errorf("release the lock at %s: %w", h.path, err)
	}
	return nil
}

func validName(name string) error {
	if name == "" {
		return errors.New("a lock has no name")
	}
	if strings.ContainsAny(name, `/\`) || strings.ContainsRune(name, 0) || name == "." || name == ".." {
		return fmt.Errorf("%q cannot name a lock file", name)
	}
	return nil
}
