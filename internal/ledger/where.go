package ledger

import (
	"context"
	"errors"
	"os/exec"
	"sort"
	"strings"

	"github.com/pbleisch/kept/internal/model"
)

// Where is where an act was performed: the world kept measured, and whether
// that world held changes no commit does.
//
// **It is advisory, and that is the whole safety property.** Nothing reads it
// to decide standing: standing attaches to a content version and to nothing
// else, so a grant means the same thing in every tree it is read in and an act
// recorded without a world is read exactly like one recorded with it. What it
// buys is diagnosis — a grant for a wording that landed nowhere can be told
// apart from a wording that landed once and was revised, which nothing in an
// act could say before. See docs/decision/0063.
type Where struct {
	// World is the tree the act was performed against, in 0014's shape and no
	// other: a world is folded into the observation preimage docs/decision/0031
	// freezes, so this shape is not one an act may extend.
	//
	// A world with no tree is no world, which is the same test
	// [Ledger.Reached] makes of a verdict — with the opposite answer, because
	// a verdict without one is refused and an act without one is read.
	World model.World `json:"world,omitzero"`

	// Uncommitted says the workspace held changes World.Commit does not, so
	// the bytes this act was about may be in no commit at all and the commit's
	// reachability says nothing about them.
	//
	// It is recorded because it cannot be derived afterwards: World.Tree is
	// kept's digest over a workspace, and a commit's tree cannot be turned
	// into one without checking it out. It travels beside World rather than
	// inside it for the reason above.
	Uncommitted bool `json:"uncommitted,omitempty"`
}

// Situated reports whether this names a world at all. Acts written before
// docs/decision/0063 name none, and a missing world is *nobody recorded one*
// rather than a claim about any tree.
func (w Where) Situated() bool { return w.World.Tree != "" }

// Situate fills in the provenance half of the world an act is being performed
// in — origin's URL, the ref checked out, the commit it resolves to — and
// reports whether the working tree holds changes that commit does not.
//
// The tree digest is the caller's, because the tree is what kept computes over
// a workspace rather than what git reports about one, and it is what makes one
// world one world. See docs/decision/0033.
//
// Outside a repository it returns the tree alone with [ErrNotRepository]: a
// workspace that is not a repository still has a world, there is simply no
// history for it to name. A repository with no commit yet is the same answer
// without the error, because an unborn branch resolves to nothing.
func Situate(repository, tree string) (Where, error) {
	w := Where{World: model.World{Tree: tree}}

	commit, err := gitOptional(repository, "rev-parse", "--verify", "--quiet", "--end-of-options", "HEAD^{commit}")
	if err != nil {
		if isNotRepository(err) {
			return w, ErrNotRepository
		}
		return w, err
	}
	w.World.Commit = commit

	// A detached HEAD names no ref and that is exit 1 rather than a failure,
	// which is the same reading 0014 gives the field: "" when detached.
	if w.World.Ref, err = gitOptional(repository, "symbolic-ref", "--quiet", "HEAD"); err != nil {
		return w, err
	}
	// A repository with no remote is 0014's empty Remote, not an error.
	if w.World.Remote, err = gitOptional(repository, "config", "--get", "remote.origin.url"); err != nil {
		return w, err
	}

	// Untracked files count, and they are the case that matters: a decision
	// authored and ratified before it is added is content no commit holds, and
	// `git status` reports it here where `git diff` would not. Ignored files do
	// not, which is the same exclusion kept's own digest makes.
	dirty, err := gitOptional(repository, "status", "--porcelain", "--untracked-files=normal")
	if err != nil {
		return w, err
	}
	w.Uncommitted = dirty != ""

	// An unborn branch has a ref and no commit, so it cannot be dirty against
	// anything: everything in it is uncommitted, which the empty commit already
	// says. Saying it twice would make the finding read as two problems.
	if w.World.Commit == "" {
		w.Uncommitted = false
	}
	return w, nil
}

// Reach is what this repository can say about one commit an act named.
type Reach string

const (
	// Reachable: the tree being asked in can reach that commit, so the wording
	// the act granted landed here at least once.
	Reachable Reach = "reachable"
	// Unreachable: it cannot — the commit is on a branch this tree does not
	// contain, or this repository does not hold the object at all. Either way
	// the content that act was about landed nowhere the reader can see.
	Unreachable Reach = "unreachable"
	// ReachUnknown: nothing was established. Not a repository, no HEAD to ask
	// from, or git failed for a reason that is not an answer. It is never
	// reported as a finding, because *nobody could ask* is not *it never
	// landed* — the same split docs/decision/0004 makes one level down.
	ReachUnknown Reach = "unknown"
)

// Reaches answers, for every commit given, whether this repository's HEAD can
// reach it.
//
// From HEAD rather than from every ref this repository holds, and that is the
// decision rather than an implementation detail: the question is whether the
// wording landed in *the tree being asked in*, which is the same thing 0014
// makes true of a verdict. Standing on the branch that granted it, the answer
// is yes and there is nothing to report; standing on the branch that declined
// it, the answer is no, whether or not the branch's ref is still lying around.
func Reaches(repository string, commits []string) map[string]Reach {
	out := make(map[string]Reach, len(commits))
	head, err := gitOptional(repository, "rev-parse", "--verify", "--quiet", "--end-of-options", "HEAD^{commit}")
	for _, commit := range commits {
		out[commit] = ReachUnknown
	}
	if err != nil || head == "" {
		return out
	}

	// One walk first, and in the ordinary case it is the only one: rev-list
	// traverses the DAG once and stops at what HEAD reaches, so a count of zero
	// says every commit given is an ancestor and there is nothing to report.
	// The per-commit questions below are worth an invocation each only when
	// that fails — a count answers whether, and which one is the finding. It is
	// the same two-phase shape [RefStore.Continuous] uses.
	if reachable(repository, head, keys(out)) {
		for commit := range out {
			out[commit] = Reachable
		}
		return out
	}

	for commit := range out {
		// A value out of a durable record is checked before it becomes an
		// argument: an act is JSON anybody's editor can reach, and a commit
		// that is not an object id is one nothing here can answer about rather
		// than something to hand to git and see what happens.
		if !objectID(commit) {
			continue
		}
		// --is-ancestor is exit 0 for reachable and exit 1 for not. A commit
		// this repository does not hold is neither: git fails to resolve the
		// name, and that is an object nobody here has rather than an ancestry
		// question, so it is asked separately rather than read out of the
		// error text.
		if _, err := gitValue(repository, "merge-base", "--is-ancestor", commit, head); err == nil {
			out[commit] = Reachable
			continue
		} else if isExitCode(err, 1) {
			out[commit] = Unreachable
			continue
		}
		if _, err := gitValue(repository, "cat-file", "-e", commit+"^{commit}"); err != nil {
			// The object is not here. A shallow clone would say the same, which
			// is why the finding this feeds says *this tree cannot reach it*
			// rather than *it does not exist*.
			out[commit] = Unreachable
		}
	}
	return out
}

// gitValue runs one read-only git command in a repository and returns its
// standard output, trimmed. Every non-zero exit is an error, so a caller that
// reads an exit code as an answer says so.
//
// Standard output only, and never the two streams combined: Apple's
// /usr/bin/git is an xcrun shim that writes cache-file errors to standard error
// on every invocation, and a value read out of a merge of the two is that noise
// with the answer somewhere in it. The detail is kept for the error. See
// AGENTS.md.
//
// Bounded, because nothing external runs without a deadline. See
// docs/decision/0026.
func gitValue(repository string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultRefDeadline)
	defer cancel()

	command := exec.CommandContext(ctx, "git", append([]string{"-C", repository}, args...)...)
	var stdout, stderr strings.Builder
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		return "", &gitExitError{args: args, output: strings.TrimSpace(stderr.String()), err: err}
	}
	return strings.TrimSpace(stdout.String()), nil
}

// gitOptional is gitValue where git's exit 1 means *there is none*: no HEAD, a
// detached one, an unset config key, a clean tree. Only a silent exit 1 is read
// that way — one that also printed something failed and is reported.
func gitOptional(repository string, args ...string) (string, error) {
	out, err := gitValue(repository, args...)
	var exit *gitExitError
	if errors.As(err, &exit) && isExitCode(err, 1) && exit.output == "" {
		return "", nil
	}
	return out, err
}

func isExitCode(err error, code int) bool {
	var wrapped *gitExitError
	if errors.As(err, &wrapped) {
		err = wrapped.err
	}
	var status *exec.ExitError
	return errors.As(err, &status) && status.ExitCode() == code
}

// reachable reports whether HEAD reaches every one of these commits, in one
// walk. False is *not established* rather than *no*: an unknown object makes
// rev-list fail outright, and the caller asks one commit at a time from there.
func reachable(repository, head string, commits []string) bool {
	if len(commits) == 0 {
		return true
	}
	var walk strings.Builder
	for _, commit := range commits {
		if !objectID(commit) {
			return false
		}
		walk.WriteString(commit + "\n")
	}
	walk.WriteString("^" + head + "\n")

	ctx, cancel := context.WithTimeout(context.Background(), DefaultRefDeadline)
	defer cancel()
	command := exec.CommandContext(ctx, "git", "-C", repository, "rev-list", "--count", "--stdin")
	command.Stdin = strings.NewReader(walk.String())
	var stdout strings.Builder
	command.Stdout = &stdout
	if err := command.Run(); err != nil {
		return false
	}
	return strings.TrimSpace(stdout.String()) == "0"
}

func keys(m map[string]Reach) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
