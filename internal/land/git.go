package land

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// defaultDeadline bounds each Git call a land makes. Two of them reach the
// network, which is exactly the reason nothing external runs without one. See
// docs/decision/0026.
const defaultDeadline = 2 * time.Minute

// ErrNoTarget means the remote has no such branch, or the fetch could not reach
// it. A land into a repository whose target does not exist is refused rather
// than invented: the alternative is pushing a first commit onto a branch nobody
// created, from a verb whose whole job is to be gated.
var ErrNoTarget = errors.New("the landing target could not be read")

// fetchTarget brings the target's branch into its tracking ref and returns the
// commit it now names.
//
// The refspec is stated in full rather than left to remote.<name>.fetch,
// because that configuration is not kept's to assume. It is measured rather
// than imagined: on 2026-08-25 this repository's own key held the ledger's
// refspec and nothing else, so an ordinary `git fetch origin` wrote no branch
// at all and refs/remotes/origin/main was eight commits stale. kept init now
// configures a branch refspec too and kept doctor reports a workspace missing
// one — but a remote an operator narrowed deliberately is still theirs, so
// naming the refspec here is what makes the answer independent of it. This is
// why decision 0015's in-lock check reads a tracking ref this call refreshed.
func fetchTarget(ctx context.Context, req Request) (string, error) {
	deadline := req.Deadlines.Git
	if deadline <= 0 {
		deadline = defaultDeadline
	}
	if _, err := git(ctx, req.Workspace, deadline,
		"fetch", "--no-tags", "--quiet", "--end-of-options", req.Target.Remote, req.Target.Refspec()); err != nil {
		return "", fmt.Errorf("%w: fetch %s: %v", ErrNoTarget, req.Target, err)
	}
	commit, err := git(ctx, req.Workspace, deadline,
		"rev-parse", "--verify", "--quiet", "--end-of-options", req.Target.Tracking()+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("%w: %s names no commit after a fetch", ErrNoTarget, req.Target.Tracking())
	}
	value := strings.TrimSpace(commit)
	if value == "" {
		return "", fmt.Errorf("%w: %s names no commit after a fetch", ErrNoTarget, req.Target.Tracking())
	}
	return value, nil
}

// push puts one commit on the target's branch.
//
// Unforced, and that is load-bearing rather than tidy. Git rejects a
// non-fast-forward, so if the in-lock check somehow raced — a machine that
// shares no lock, a clock, a bug here — the push fails rather than discarding
// somebody else's commit. The lock saves the cost of a gate that was going to
// be wasted; this is what makes the operation safe. See docs/decision/0013.
func push(ctx context.Context, req Request, commit string) error {
	deadline := req.Deadlines.Git
	if deadline <= 0 {
		deadline = defaultDeadline
	}
	if _, err := git(ctx, req.Workspace, deadline,
		"push", "--quiet", "--end-of-options", req.Target.Remote, commit+":"+req.Target.Ref()); err != nil {
		return fmt.Errorf("push %s to %s: %w", short(commit), req.Target, err)
	}
	return nil
}

func remoteURL(req Request) (string, error) {
	out, err := git(context.Background(), req.Workspace, defaultDeadline,
		"config", "--get", "remote."+req.Target.Remote+".url")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// git runs one command and returns its standard output.
//
// Standard output alone, never the two merged: Apple's /usr/bin/git is an xcrun
// shim that writes cache-file errors to standard error, and a commit id read
// out of a merge of the two is that noise with an id somewhere in it. The
// detail is kept for the error. See AGENTS.md.
func git(ctx context.Context, dir string, deadline time.Duration, args ...string) (string, error) {
	call, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()
	command := exec.CommandContext(call, "git", append([]string{"-C", dir}, args...)...)
	command.Env = withoutGitWorktreeEnv()
	var stdout, stderr strings.Builder
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	if call.Err() != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), call.Err())
	}
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// withoutGitWorktreeEnv drops the variables that would point a command at
// whichever worktree the caller happened to inherit, which is never the one it
// was given.
func withoutGitWorktreeEnv() []string {
	src := os.Environ()
	filtered := make([]string, 0, len(src))
	for _, value := range src {
		name, _, _ := strings.Cut(value, "=")
		switch name {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR":
			continue
		}
		filtered = append(filtered, value)
	}
	return filtered
}

// readTarget is the commit the target names right now, under the regime the
// target implies.
//
// A shared target is fetched and read out of the tracking ref it just
// refreshed; a local one is a ref read and reaches no network at all. That
// split is the whole of docs/decision/0015's check regime, and it is made here
// once rather than at each of the three sites that ask.
func readTarget(ctx context.Context, req Request) (string, error) {
	if req.Target.Shared() {
		return fetchTarget(ctx, req)
	}
	return localTarget(ctx, req)
}

// localTarget reads the branch itself. Nothing outside this machine can write
// a local ref, so there is nothing to fetch and no network to reach.
func localTarget(ctx context.Context, req Request) (string, error) {
	commit, err := gitValue(ctx, req, "rev-parse", "--verify", "--quiet", "--end-of-options", req.Target.Ref()+"^{commit}")
	if err != nil || strings.TrimSpace(commit) == "" {
		return "", fmt.Errorf("%w: %s names no commit", ErrNoTarget, req.Target)
	}
	return strings.TrimSpace(commit), nil
}

// swapRef moves a local branch from the value the gate saw to the merge, and
// refuses if it is anywhere else.
//
// This is docs/decision/0015's compare-and-swap for the local ref arrival, and
// it is the backstop rather than the lock: `git update-ref` takes the expected
// old value and fails without writing when the ref is not on it, exactly as an
// unforced push does for the shared case.
func swapRef(ctx context.Context, req Request, merged, expected string) error {
	if _, err := gitValue(ctx, req, "update-ref", "--end-of-options", req.Target.Ref(), merged, expected); err != nil {
		return fmt.Errorf("advance %s from %s to %s: %w", req.Target, short(expected), short(merged), err)
	}
	return nil
}

// syncCheckout brings the index and the working tree across after the ref moved
// under them.
//
// Two calls rather than one because the swap is the land and this is the files
// following it: `git update-ref` is the compare-and-swap docs/decision/0015
// asks for and `git merge --ff-only` is not one, so the ref moves first and
// this catches up. The tree was verified clean immediately before, so a hard
// reset discards nothing — and if it fails, the ref has already moved and the
// caller reports a tree the operator must sync by hand rather than pretending
// nothing landed.
func syncCheckout(ctx context.Context, req Request, merged string) error {
	if _, err := gitValue(ctx, req, "reset", "--hard", "--quiet", "--end-of-options", merged); err != nil {
		return fmt.Errorf("bring the working tree to %s: %w", short(merged), err)
	}
	return nil
}

// checkedOutBranch is the branch this working tree holds, or "" for a detached
// HEAD. A detached HEAD is not an error here: it is an answer, and the caller
// decides what it means.
func checkedOutBranch(ctx context.Context, workspace string, deadline time.Duration) (string, error) {
	args := []string{"symbolic-ref", "--quiet", "--short", "HEAD"}
	out, why, code, err := gitCode(ctx, workspace, deadline, args...)
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", absent(args, why, code)
	}
	return strings.TrimSpace(out), nil
}

// configValue reads one git configuration key. An unset key is exit 1 with
// nothing on either stream, which is *there is none* rather than a failure.
func configValue(ctx context.Context, workspace string, deadline time.Duration, key string) (string, error) {
	args := []string{"config", "--get", "--end-of-options", key}
	out, why, code, err := gitCode(ctx, workspace, deadline, args...)
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", absent(args, why, code)
	}
	return strings.TrimSpace(out), nil
}

// remotes is what this repository configures, as a set. It is what decides
// whether the first segment of a target a person typed is a remote or part of a
// branch name.
func remotes(ctx context.Context, workspace string, deadline time.Duration) (map[string]bool, error) {
	if deadline <= 0 {
		deadline = defaultDeadline
	}
	out, why, code, err := gitCode(ctx, workspace, deadline, "remote")
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, fmt.Errorf("read this repository's remotes: git exited %d: %s", code, strings.TrimSpace(why))
	}
	found := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		if name := strings.TrimSpace(line); name != "" {
			found[name] = true
		}
	}
	return found, nil
}

// worktreeHolding is the worktree this repository has checked out on a branch,
// and "" when no worktree holds it.
//
// Read from `git worktree list --porcelain`, which is git's own index of the
// answer: the records are blank-line separated, a `worktree` line opens each
// one and a `branch` line names what it holds — absent for a detached one and
// for the bare repository.
func worktreeHolding(ctx context.Context, workspace string, deadline time.Duration, ref string) (string, error) {
	if deadline <= 0 {
		deadline = defaultDeadline
	}
	out, why, code, err := gitCode(ctx, workspace, deadline, "worktree", "list", "--porcelain")
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", fmt.Errorf("read which working tree holds %s: git exited %d: %s", ref, code, strings.TrimSpace(why))
	}
	path := ""
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			path = strings.TrimPrefix(line, "worktree ")
		case strings.TrimSpace(line) == "":
			path = ""
		case strings.HasPrefix(line, "branch "):
			if strings.TrimPrefix(line, "branch ") == ref {
				return path, nil
			}
		}
	}
	return "", nil
}

// topLevel is the root of the working tree the workspace is in. It is what
// "here" means in the arrival table, and it is asked of git rather than assumed
// to be the workspace root, which is where .kept is and need not be the same
// directory.
func topLevel(ctx context.Context, workspace string, deadline time.Duration) (string, error) {
	if deadline <= 0 {
		deadline = defaultDeadline
	}
	out, why, code, err := gitCode(ctx, workspace, deadline, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", fmt.Errorf("read the root of this working tree: git exited %d: %s", code, strings.TrimSpace(why))
	}
	return strings.TrimSpace(out), nil
}

// headCommit is the commit this working tree is standing on, which is what a
// worktree arrival's act records as the world's commit: it is where the
// operator was, and the change landed nowhere near it.
func headCommit(ctx context.Context, workspace string, deadline time.Duration) (string, error) {
	if deadline <= 0 {
		deadline = defaultDeadline
	}
	args := []string{"rev-parse", "--verify", "--quiet", "--end-of-options", "HEAD^{commit}"}
	out, why, code, err := gitCode(ctx, workspace, deadline, args...)
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", absent(args, why, code)
	}
	return strings.TrimSpace(out), nil
}

// notKept is the pathspec that keeps kept's own tier out of a question about
// the operator's work.
//
// It is load-bearing rather than tidy, and it is measured: a land cuts its gate
// box at <workspace>/.kept/boxes/<id>, so by the time anything inside the lock
// asks *is this tree clean* the box is sitting in it. Without this, every local
// ref arrival refuses itself and every worktree snapshot swallows a whole
// second checkout. internal/workspace excludes the same directory from the 0033
// digest for the neighbouring reason, so the two instruments agree about what
// counts as the operator's tree. A repository that gitignores .kept/ gets the
// same answer either way; one that does not is not kept's to assume.
var notKept = []string{"--", ".", ":(exclude).kept"}

// treeIsDirty reports whether the working tree holds anything HEAD does not.
//
// Untracked files count, for the reason [ledger.Situate] counts them: a file
// the operator has written and not added is their work, and a merge that
// overwrote it would have taken it. Ignored files do not, which is the same
// exclusion kept's own digest makes.
func treeIsDirty(ctx context.Context, workspace string, deadline time.Duration) (bool, error) {
	if deadline <= 0 {
		deadline = defaultDeadline
	}
	args := append([]string{"status", "--porcelain", "--untracked-files=normal"}, notKept...)
	out, why, code, err := gitCode(ctx, workspace, deadline, args...)
	if err != nil {
		return false, err
	}
	if code != 0 {
		return false, fmt.Errorf("read whether the working tree at %s is clean: git exited %d: %s", workspace, code, strings.TrimSpace(why))
	}
	return strings.TrimSpace(out) != "", nil
}

// gitValue runs one command against the request's workspace under its deadline.
func gitValue(ctx context.Context, req Request, args ...string) (string, error) {
	deadline := req.Deadlines.Git
	if deadline <= 0 {
		deadline = defaultDeadline
	}
	return git(ctx, req.Workspace, deadline, args...)
}

// gitCode runs one command and hands back its exit code and both streams
// rather than an error, so a caller reads exit 1 as the answer it is — no such
// ref, no such key, a clean tree — instead of out of an error string.
//
// The two streams are kept apart, and the distinction is what every caller
// here needs: git answers *there is none* with exit 1 and *this is not a
// repository* with exit 128, and collapsing the two reports a directory that
// is not a checkout as a detached HEAD. err is returned only when the command
// could not be run at all or its deadline expired.
func gitCode(ctx context.Context, dir string, deadline time.Duration, args ...string) (out, why string, code int, err error) {
	if deadline <= 0 {
		deadline = defaultDeadline
	}
	call, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()
	command := exec.CommandContext(call, "git", append([]string{"-C", dir}, args...)...)
	command.Env = withoutGitWorktreeEnv()
	var stdout, stderr strings.Builder
	command.Stdout = &stdout
	command.Stderr = &stderr
	ran := command.Run()
	if call.Err() != nil {
		return "", "", 0, fmt.Errorf("git %s: %w", strings.Join(args, " "), call.Err())
	}
	var exit *exec.ExitError
	switch {
	case ran == nil:
		return stdout.String(), stderr.String(), 0, nil
	case errors.As(ran, &exit):
		return stdout.String(), stderr.String(), exit.ExitCode(), nil
	default:
		return "", "", 0, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), ran, strings.TrimSpace(stderr.String()))
	}
}

// absent reports whether a git command answered *there is none* rather than
// failing. Exit 1 alone is that answer; every other code is a failure and is
// reported with git's own words, because a repository that is not there and a
// ref that is not there are different findings.
func absent(args []string, why string, code int) error {
	if code == 1 {
		return nil
	}
	return fmt.Errorf("git %s: exited %d: %s", strings.Join(args, " "), code, strings.TrimSpace(why))
}

// gitEnv runs one command against the request's workspace with extra
// environment. It is what a scratch index and a stated commit identity need,
// and it exists separately so the ordinary path cannot accidentally inherit
// either.
func gitEnv(ctx context.Context, req Request, extra []string, args ...string) (string, error) {
	deadline := req.Deadlines.Git
	if deadline <= 0 {
		deadline = defaultDeadline
	}
	call, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()
	command := exec.CommandContext(call, "git", append([]string{"-C", req.Workspace}, args...)...)
	command.Env = append(withoutGitWorktreeEnv(), extra...)
	var stdout, stderr strings.Builder
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	if call.Err() != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), call.Err())
	}
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// sortStrings sorts in place. A path list a person reads or a test compares is
// ordered, and an order that came from a map is one that moves on its own.
func sortStrings(values []string) { sort.Strings(values) }
