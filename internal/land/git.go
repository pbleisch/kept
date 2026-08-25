package land

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
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
// because that configuration is not kept's to assume: this repository's own is
// narrowed to the ledger's refs, so an ordinary `git fetch origin` here writes
// no branch at all and a land reading refs/remotes/origin/main would be reading
// a ref nothing ever wrote. Naming the refspec makes the answer independent of
// how the operator's remote happens to be configured.
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
