package land

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pbleisch/kept/internal/ledger"
)

// Target is where a land is going: one branch, on a remote or on this machine.
//
// It is a value carried through the gate, the lock, the in-lock check and the
// act rather than a constant read at each of those sites: the ref, the tracking
// ref, the refspec and the push destination are all derived from it, so the one
// place a landing target is named is here.
//
// A target with no remote is a **local** target and is written that way — the
// branch alone. That is not a degenerate case: it is every repository on its
// first day and many of them forever, and it is the branch the operator is
// standing on when they want the repair here. See docs/decision/0015.
type Target struct {
	// Remote is the remote the branch lives on, and "" for a branch that lives
	// only on this machine.
	Remote string
	Branch string
}

// Origin is origin/main, which is one target rather than the definition of one.
func Origin() Target { return Target{Remote: "origin", Branch: "main"} }

// Local is a branch on this machine and on no remote.
func Local(branch string) Target { return Target{Branch: branch} }

// Shared reports whether this target has writers a local lock cannot exclude —
// another machine, CI, a second checkout of the same remote. It is exactly
// *does it name a remote*, and the check regime follows it.
func (t Target) Shared() bool { return t.Remote != "" }

// Regime is which in-lock check this target gets. It is derived here and never
// configured: a configured regime is a second place the answer lives and can
// disagree with where the change is actually going.
func (t Target) Regime() ledger.Regime {
	if t.Shared() {
		return ledger.SharedRegime
	}
	return ledger.LocalRegime
}

// Valid refuses a target that could not be handed to git as it stands. A ref
// name is checked rather than trusted because it becomes an argument, and a
// value that starts with a dash is a flag wherever it is put.
//
// An empty remote is a local target and is not an error; an empty branch is,
// because a target is a branch.
func (t Target) Valid() error {
	names := map[string]string{"branch": t.Branch}
	if t.Remote != "" {
		names["remote"] = t.Remote
	}
	for what, value := range names {
		switch {
		case strings.HasPrefix(value, "-"):
			return fmt.Errorf("a landing target's %s %q would be read as a flag", what, value)
		case strings.ContainsAny(value, " \t\n:?*[\\^~\x00"):
			return fmt.Errorf("a landing target's %s %q is not a name git accepts", what, value)
		case strings.Contains(value, ".."):
			return fmt.Errorf("a landing target's %s %q is not a name git accepts", what, value)
		}
	}
	if t.Branch == "" {
		return fmt.Errorf("a landing target names no branch")
	}
	if strings.Contains(t.Remote, "/") {
		return fmt.Errorf("a landing target's remote %q is not a remote name", t.Remote)
	}
	return nil
}

// Ref is the branch, as its own repository holds it.
func (t Target) Ref() string { return "refs/heads/" + t.Branch }

// Tracking is the local copy of a shared branch — the ref a fetch writes and
// the in-lock check reads. It is never the local branch of the same name: a
// local lock excludes local runs and nothing else, so a check against
// refs/heads/main would be answering about this machine rather than about the
// remote. A local target has no tracking ref and does not need one.
func (t Target) Tracking() string {
	if !t.Shared() {
		return ""
	}
	return "refs/remotes/" + t.Remote + "/" + t.Branch
}

// Check is the ref the in-lock check reads: the tracking ref a fetch just
// refreshed for a shared target, and the branch itself for a local one, which
// nothing outside this machine can write.
func (t Target) Check() string {
	if t.Shared() {
		return t.Tracking()
	}
	return t.Ref()
}

// Refspec is what a land fetches, stated in full rather than left to whatever
// remote.<name>.fetch happens to say.
//
// Forced, and for the reason docs/decision/0062 gives: a tracking ref is a copy
// of what the remote holds, and git forces every one of them. The destination
// is a tracking ref and never a durable record, which is the distinction that
// decision was written about.
func (t Target) Refspec() string {
	if !t.Shared() {
		return ""
	}
	return "+" + t.Ref() + ":" + t.Tracking()
}

// String is the target as a person names it, and as the act records it:
// "origin/main" for a shared target and "main" for a local one.
func (t Target) String() string {
	if t.Shared() {
		return t.Remote + "/" + t.Branch
	}
	return t.Branch
}

// ParseTarget reads a target as a person spells it on --onto.
//
// The first segment is a remote when this repository configures one by that
// name, and the whole string is a local branch otherwise. A branch whose own
// name would be read as a remote is spelled in full, as refs/heads/<name>,
// which is the escape hatch rather than a second syntax. See
// docs/decision/0015.
func ParseTarget(ctx context.Context, workspace, spec string, deadline time.Duration) (Target, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return Target{}, fmt.Errorf("a landing target names no branch")
	}
	if branch, found := strings.CutPrefix(spec, "refs/heads/"); found {
		t := Local(branch)
		return t, t.Valid()
	}
	if head, rest, found := strings.Cut(spec, "/"); found && rest != "" {
		configured, err := remotes(ctx, workspace, deadline)
		if err != nil {
			return Target{}, err
		}
		if configured[head] {
			t := Target{Remote: head, Branch: rest}
			return t, t.Valid()
		}
	}
	t := Local(spec)
	return t, t.Valid()
}

// ResolveTarget is git's own answer to *where does my work go*: the upstream of
// the branch this working tree has checked out, and the branch itself when it
// has none.
//
// One rule rather than a table of deployments, and it degrades in the right
// direction at every step — a tracking branch lands on its remote, a branch
// with no upstream lands on itself, and a detached HEAD is not a branch and so
// is not a place a land can reach. See docs/decision/0015.
func ResolveTarget(ctx context.Context, workspace string, deadline time.Duration) (Target, error) {
	branch, err := checkedOutBranch(ctx, workspace, deadline)
	if err != nil {
		return Target{}, err
	}
	if branch == "" {
		return Target{}, fmt.Errorf("this working tree is on a detached HEAD, which is not a branch a land can reach; name one with --onto")
	}

	remote, err := configValue(ctx, workspace, deadline, "branch."+branch+".remote")
	if err != nil {
		return Target{}, err
	}
	merge, err := configValue(ctx, workspace, deadline, "branch."+branch+".merge")
	if err != nil {
		return Target{}, err
	}
	upstream, named := strings.CutPrefix(merge, "refs/heads/")
	if remote == "" || !named || upstream == "" {
		// No upstream, or one kept cannot read as a branch. The branch is
		// where this operator's own work goes and it is a target.
		t := Local(branch)
		return t, t.Valid()
	}
	if remote == "." {
		// git's spelling of a local upstream. It names no remote, so it is a
		// local target and the land reaches no network.
		t := Local(upstream)
		return t, t.Valid()
	}
	t := Target{Remote: remote, Branch: upstream}
	return t, t.Valid()
}
