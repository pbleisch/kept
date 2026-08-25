package box

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Identity is who a commit is authored and committed by.
//
// It is always a person. An agent produced the diff; a person is accountable
// for it, and the history of a repository is a record of decisions and who made
// them. Nothing here writes a name for a model, a tool or kept itself into a
// commit that can reach a branch.
type Identity struct {
	Name  string
	Email string
}

func (i Identity) valid() bool { return i.Name != "" && i.Email != "" }

// Anchor commits what the box holds now and points ref at the result, so the
// repair survives the box being discarded.
//
// Without this a repair's tree lives only in a worktree that is removed at the
// end of the run: the diff is printed and dropped, and nothing durable names
// the bytes. The commit's parent is the commit the box was cut from, so the
// commit's own diff against its parent is exactly the change a person reviewed.
//
// The ref is what keeps it: an unreferenced commit is collectable, and the one
// thing an anchor may not be is a tree that git is free to take away between
// the repair and the land. See docs/decision/0067.
func (b *Box) Anchor(ctx context.Context, ref, message string, by Identity) (string, error) {
	if ctx == nil {
		return "", errors.New("repair box context is nil")
	}
	if !strings.HasPrefix(ref, "refs/") {
		return "", fmt.Errorf("%q is not a full ref name", ref)
	}
	if strings.TrimSpace(message) == "" {
		return "", errors.New("an anchor commit has no message")
	}
	if !by.valid() {
		return "", errors.New("an anchor commit names no author")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return "", ErrClosed
	}

	tree, err := b.snapshot(ctx)
	if err != nil {
		return "", err
	}
	// The operator authors and commits it. gpg signing is off for the same
	// reason the ledger's own commits turn it off: a machine-written commit
	// must not depend on a key being present, and nothing here is a claim about
	// who reviewed the change.
	env := []string{
		"GIT_AUTHOR_NAME=" + by.Name, "GIT_AUTHOR_EMAIL=" + by.Email,
		"GIT_COMMITTER_NAME=" + by.Name, "GIT_COMMITTER_EMAIL=" + by.Email,
	}
	out, err := gitInput(ctx, b.path, b.deadline, env, []byte(message),
		"-c", "commit.gpgSign=false", "commit-tree", tree, "-p", b.commit)
	if err != nil {
		return "", fmt.Errorf("commit the repair: %w", err)
	}
	commit := strings.TrimSpace(string(out))
	if !objectID(commit) {
		return "", fmt.Errorf("git returned %q rather than a commit id", commit)
	}
	if _, err := git(ctx, b.path, b.deadline, nil, "update-ref", "-m", "kept anchor", ref, commit); err != nil {
		return "", fmt.Errorf("anchor the repair at %s: %w", ref, err)
	}
	return commit, nil
}

// Unchanged reports whether the box still holds exactly the tree it started
// with, so a caller can tell a session that changed nothing from one that did
// without reading a diff.
func (b *Box) Unchanged(ctx context.Context) (bool, error) {
	if ctx == nil {
		return false, errors.New("repair box context is nil")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return false, ErrClosed
	}
	current, err := b.snapshot(ctx)
	if err != nil {
		return false, err
	}
	return current == b.baseTree, nil
}

func objectID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}
