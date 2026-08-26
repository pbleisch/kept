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

// Merge brings a change into this box and commits the result, so the box now
// holds the tree that would exist if this change landed.
//
// This is what a gate measures. docs/decision/0015 cuts the box at the landing
// target and gates *the box*, which means the box has to contain the merge
// rather than the target: measuring the target would answer a question about
// the tree that is already there.
//
// # The base is stated, and that is the whole of docs/decision/0068
//
// An anchor is a pair — a base commit holding the tree the repair started from,
// and a commit holding the change — and this is the read of it. base is the
// anchor's parent, change is the anchor, and the box's own commit is the target.
// A three-way merge with that base yields *the target plus the change* and
// nothing else.
//
// Letting git compute the base instead takes the change's whole ancestry with
// it, and the graph does not know which of that ancestry was private to the
// machine the repair ran on: an operator's unlanded commits, or the snapshot a
// dirty repair takes of their workspace. Measured, that is how work nobody
// reviewed reaches a landing target inside a repair.
//
// # One parent
//
// The result is committed with the box's own commit as its **only** parent. A
// parent is reachable and the base is private, so making the change a second
// parent publishes it to the target — the same defect moved from the tree into
// the graph, where it is harder to see. What the change came from is on the
// ledger act, which names the thread.
//
// # A merge that changes nothing commits nothing
//
// The three-way merge is idempotent: landing the same anchor twice computes the
// same base against a target that already holds the change, both sides agree,
// and the result is the target unchanged. That is a no-op rather than a
// duplicate, so the box's own commit comes back and no empty commit is written.
//
// A conflict is [ErrConflicted] and the box is untouched — the merge is computed
// over trees, so there is no worktree state to leave behind and nothing to
// abort. A git that cannot perform the merge at all is an ordinary error and
// never a conflict.
func (b *Box) Merge(ctx context.Context, base, change, message string, by Identity) (string, error) {
	if ctx == nil {
		return "", errors.New("repair box context is nil")
	}
	if !objectID(base) {
		return "", fmt.Errorf("%q is not a commit id, and a merge states its base", base)
	}
	if !objectID(change) {
		return "", fmt.Errorf("%q is not a commit id", change)
	}
	if base == change {
		return "", fmt.Errorf("%s is both the base and the change, so there is nothing to merge", base)
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

	merged, err := b.mergeTree(ctx, base, change)
	if err != nil {
		return "", err
	}
	ours, err := git(ctx, b.path, b.deadline, nil, "rev-parse", "--verify", "--end-of-options", b.commit+"^{tree}")
	if err != nil {
		return "", fmt.Errorf("read the tree this box holds: %w", err)
	}
	if merged == strings.TrimSpace(string(ours)) {
		// The target already holds this change. Nothing is committed and the
		// box is left where it is, so the caller's own report is about the
		// commit that already carries it.
		return b.commit, nil
	}

	env := []string{
		"GIT_AUTHOR_NAME=" + by.Name, "GIT_AUTHOR_EMAIL=" + by.Email,
		"GIT_COMMITTER_NAME=" + by.Name, "GIT_COMMITTER_EMAIL=" + by.Email,
	}
	// The message arrives on standard input rather than as an argv element, so
	// nothing in it can be read as a flag. gpg signing is off for the reason
	// every machine-written commit here turns it off: it must not depend on a
	// key being present.
	out, err := gitInput(ctx, b.path, b.deadline, env, []byte(message),
		"-c", "commit.gpgSign=false", "commit-tree", merged, "-p", b.commit)
	if err != nil {
		return "", fmt.Errorf("commit the merge: %w", err)
	}
	id := strings.TrimSpace(string(out))
	if !objectID(id) {
		return "", fmt.Errorf("git returned %q rather than a commit id", id)
	}
	// The tree was computed rather than checked out, so the box is still at the
	// target. It has to hold the merge before anything measures it.
	if _, err := git(ctx, b.path, b.deadline, nil, "reset", "--hard", "--quiet", "--end-of-options", id); err != nil {
		return "", fmt.Errorf("bring the merge into the box: %w", err)
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

// mergeTree is the three-way merge itself, with the base stated.
//
// `git merge-tree --write-tree` writes the result into the object store and
// reports it, without a worktree and without moving anything. Its first line of
// standard output is the tree in every case; a conflict is exit 1 with the
// conflicted paths and git's own messages after it, and any other exit is a git
// that could not perform the merge at all.
func (b *Box) mergeTree(ctx context.Context, base, change string) (string, error) {
	out, why, code, err := gitOutcome(ctx, b.path, b.deadline,
		"merge-tree", "--write-tree", "--merge-base="+base, "--", b.commit, change)
	if err != nil {
		return "", err
	}
	first, rest, _ := strings.Cut(strings.TrimRight(out, "\n"), "\n")
	switch {
	case code == 1:
		return "", fmt.Errorf("%w: %s", ErrConflicted, conflictReport(rest))
	case code != 0:
		return "", fmt.Errorf("merge %s onto %s: git exited %d: %s",
			change, b.commit, code, strings.TrimSpace(why+out))
	case !objectID(first):
		return "", fmt.Errorf("git returned %q rather than a tree id", first)
	}
	return first, nil
}

// conflictReport is the part of a conflicted merge a person reads.
//
// merge-tree reports the conflicted file info first — one index line per stage,
// which is what git needs and not what a reader does — and its own messages
// after a blank line. The messages are the report; the index lines are kept
// only when there are none, so nothing is ever dropped to silence.
func conflictReport(rest string) string {
	if _, messages, found := strings.Cut(rest, "\n\n"); found && strings.TrimSpace(messages) != "" {
		return strings.TrimSpace(messages)
	}
	return strings.TrimSpace(rest)
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
