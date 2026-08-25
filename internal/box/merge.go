package box

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ErrConflicted means the merge left conflicts a person has to settle. It is
// not a promise failing: nothing was measured, and the change cannot be gated
// in this box at all.
var ErrConflicted = errors.New("the merge conflicts")

// Merge brings commit into this box and commits the result, so the box now
// holds the tree that would exist if this change landed.
//
// This is what a gate measures. docs/decision/0015 cuts the box at the landing
// target and gates *the box*, which means the box has to contain the merge
// rather than the target: measuring the target would answer a question about
// the tree that is already there.
//
// A conflict is [ErrConflicted] and the box is left as git left it. The merge
// is aborted first so a caller that reports and moves on does not leave a
// worktree mid-merge, and the abort's own failure is reported with the
// conflict rather than instead of it.
func (b *Box) Merge(ctx context.Context, commit, message string, by Identity) (string, error) {
	if ctx == nil {
		return "", errors.New("repair box context is nil")
	}
	if !objectID(commit) {
		return "", fmt.Errorf("%q is not a commit id", commit)
	}
	if strings.TrimSpace(message) == "" {
		return "", errors.New("a merge commit has no message")
	}
	if !by.valid() {
		return "", errors.New("a merge commit names no author")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return "", ErrClosed
	}

	env := []string{
		"GIT_AUTHOR_NAME=" + by.Name, "GIT_AUTHOR_EMAIL=" + by.Email,
		"GIT_COMMITTER_NAME=" + by.Name, "GIT_COMMITTER_EMAIL=" + by.Email,
	}
	// --no-ff so the landing commit is always a merge with both parents, which
	// is what makes what landed and what it landed on both readable off the
	// commit afterwards. A fast-forward would record only one of them.
	//
	// The message is an argv element rather than standard input because git
	// merge has no way to read one from a stream: -F takes a file and does not
	// accept `-`. It is composed here and never supplied by a caller, and every
	// message this package writes begins with a fixed word, so nothing in it
	// can be read as a flag.
	_, err := git(ctx, b.path, b.deadline, env,
		"-c", "commit.gpgSign=false", "merge", "--no-ff", "--no-edit", "-m", message, "--", commit)
	if err != nil {
		if _, abort := git(ctx, b.path, b.deadline, nil, "merge", "--abort"); abort != nil {
			return "", fmt.Errorf("%w: %v; and the merge would not abort: %v", ErrConflicted, err, abort)
		}
		return "", fmt.Errorf("%w: %v", ErrConflicted, err)
	}

	merged, err := git(ctx, b.path, b.deadline, nil, "rev-parse", "--verify", "--end-of-options", "HEAD^{commit}")
	if err != nil {
		return "", fmt.Errorf("read the merge commit: %w", err)
	}
	id := strings.TrimSpace(string(merged))
	if !objectID(id) {
		return "", fmt.Errorf("git returned %q rather than a commit id", id)
	}
	// The box's own baseline moves with it. Change is "what the repair did to
	// this box", and after a merge the tree the box started from is no longer
	// the tree anything is measured against.
	b.commit = id
	tree, err := b.snapshot(ctx)
	if err != nil {
		return "", err
	}
	b.baseTree = tree
	return id, nil
}

// Names is the paths one commit changed against another, as git reports them:
// workspace-relative, slash-separated and sorted.
//
// Read from git rather than parsed out of diff text, for the reason
// [Change.Paths] is: a delete's diff header names /dev/null, so reading `+++ b/`
// drops exactly the change a scope review most needs to see.
func (b *Box) Names(ctx context.Context, from, to string) ([]string, error) {
	if ctx == nil {
		return nil, errors.New("repair box context is nil")
	}
	if !objectID(from) || !objectID(to) {
		return nil, fmt.Errorf("%q..%q is not a pair of object ids", from, to)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, ErrClosed
	}
	names, err := boundedGit(ctx, b.path, b.deadline, b.maxDiff,
		"diff", "--name-only", "-z", "--no-ext-diff", "--no-textconv", from, to)
	if err != nil {
		return nil, err
	}
	return changedPaths(names), nil
}
