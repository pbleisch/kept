package land

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/pbleisch/kept/internal/ledger"
)

// Checkout is what this repository says about the branch a target names: which
// working tree holds it, whether that tree is this one, and whether it is
// clean.
//
// It is the whole input to the arrival table. A shared target is never checked
// out in a working tree — a remote branch lives on a remote — so it is answered
// without asking git anything.
type Checkout struct {
	// Worktree is the path of the working tree holding the branch, and "" when
	// none does.
	Worktree string
	// Here reports whether that working tree is the one this land is running
	// in.
	Here bool
	// Dirty reports whether this working tree holds anything its commit does
	// not. It is read only when Here, because the only tree a land ever writes
	// into is this one.
	Dirty bool
}

// Held reports whether any working tree has the branch checked out.
func (c Checkout) Held() bool { return c.Worktree != "" }

// Plan is where a land is going and how the change will get there.
//
// All three are resolved before anything is gated and printed before anything
// is acted on, because a land that resolved its arrival silently is how
// `landed` comes to mean two things. See docs/decision/0015.
type Plan struct {
	Target  Target
	Regime  ledger.Regime
	Arrival ledger.Arrival
	// Pinned reports whether the arrival was asked for rather than resolved.
	Pinned bool
	// Checkout is what the arrival was resolved from.
	Checkout Checkout
}

// Because says why this arrival and not the other, in the words of the row of
// docs/decision/0015's table it came from.
//
// It says the condition and what follows from it, and never repeats the
// arrival's own name: the caller has already printed that, and a reason that
// restates its conclusion tells a reader nothing they did not just read.
func (p Plan) Because() string {
	switch {
	case p.Pinned:
		return "you asked for it with --arrival"
	case !p.Checkout.Held():
		return "no working tree has " + p.Target.Branch + " checked out"
	case p.Arrival == ledger.WorktreeArrival:
		return "this working tree has " + p.Target.Branch + " checked out and is not clean, so nothing is committed and nothing moves"
	default:
		return "this working tree has " + p.Target.Branch + " checked out and is clean, so your files move with the ref"
	}
}

// Inspect asks what the target's branch is checked out in.
func Inspect(ctx context.Context, workspace string, target Target, deadline time.Duration) (Checkout, error) {
	if target.Shared() {
		// A remote branch is checked out in no working tree. Asking git about
		// refs/heads/<branch> here would answer about the local branch of the
		// same name, which is a different ref and not this target.
		return Checkout{}, nil
	}
	holder, err := worktreeHolding(ctx, workspace, deadline, target.Ref())
	if err != nil {
		return Checkout{}, err
	}
	if holder == "" {
		return Checkout{}, nil
	}
	c := Checkout{Worktree: holder}

	here, err := topLevel(ctx, workspace, deadline)
	if err != nil {
		return Checkout{}, err
	}
	c.Here = samePath(here, holder)
	if !c.Here {
		return c, nil
	}
	if c.Dirty, err = treeIsDirty(ctx, workspace, deadline); err != nil {
		return Checkout{}, err
	}
	return c, nil
}

// Resolve is docs/decision/0015's arrival table, whole.
//
//	the target is                     the tree is   ref                worktree
//	checked out in no worktree        —             resolved           refused
//	checked out here                  clean         resolved           on request
//	checked out here                  dirty         refused            resolved
//	checked out in another worktree   —             refused            refused
//
// Every refusal refuses **by name**. Pinning an impossible arrival never falls
// back to the other one: the two do not carry the same guarantee, so quietly
// satisfying a request for the stronger with the weaker is the defect the
// record was built to make visible.
//
// pinned is what --arrival asked for, and "" resolves.
func Resolve(ctx context.Context, workspace string, target Target, pinned ledger.Arrival, deadline time.Duration) (Plan, error) {
	if err := target.Valid(); err != nil {
		return Plan{}, err
	}
	switch pinned {
	case "", ledger.RefArrival, ledger.WorktreeArrival:
	default:
		return Plan{}, fmt.Errorf("%q is not an arrival; a land arrives on a ref or in a working tree", pinned)
	}

	held, err := Inspect(ctx, workspace, target, deadline)
	if err != nil {
		return Plan{}, err
	}
	plan := Plan{Target: target, Regime: target.Regime(), Pinned: pinned != "", Checkout: held}

	admits, why := admitted(target, held)
	if pinned != "" {
		if refusal := why[pinned]; refusal != "" {
			return Plan{}, fmt.Errorf("%s", refusal)
		}
		plan.Arrival = pinned
		return plan, nil
	}

	// Resolved. A dirty tree holding the target is the one row where the
	// worktree arrival is the answer rather than a request; everywhere else the
	// ref is, because a commit reaching a ref is the stronger guarantee and a
	// land takes the strongest one available.
	switch {
	case admits[ledger.RefArrival]:
		plan.Arrival = ledger.RefArrival
	case admits[ledger.WorktreeArrival]:
		plan.Arrival = ledger.WorktreeArrival
	default:
		return Plan{}, fmt.Errorf("%s is checked out in %s, so neither arrival is possible: a land moves a branch only from the working tree standing on it, and merges only into this one. Run kept land there, or name another branch with --onto",
			target, held.Worktree)
	}
	return plan, nil
}

// admitted is one row of the table: which arrivals this target and checkout
// admit, and why each refused one is refused.
func admitted(target Target, held Checkout) (map[ledger.Arrival]bool, map[ledger.Arrival]string) {
	admits := map[ledger.Arrival]bool{}
	why := map[ledger.Arrival]string{}
	switch {
	case !held.Held():
		admits[ledger.RefArrival] = true
		why[ledger.WorktreeArrival] = fmt.Sprintf(
			"there is no working tree at %s, so a worktree arrival has nothing to merge into. Drop --arrival worktree, or check that branch out here",
			target)
	case !held.Here:
		why[ledger.RefArrival] = fmt.Sprintf(
			"%s is checked out in %s, and a land moves a branch only from the working tree standing on it. Run kept land there, or name another branch with --onto",
			target, held.Worktree)
		why[ledger.WorktreeArrival] = fmt.Sprintf(
			"%s is checked out in %s, which is not this working tree, and a land merges only into this one. Run kept land there, or name another branch with --onto",
			target, held.Worktree)
	case held.Dirty:
		admits[ledger.WorktreeArrival] = true
		why[ledger.RefArrival] = fmt.Sprintf(
			"this working tree has %s checked out and is not clean, and git will not move a branch out from under a dirty tree. Commit what you are holding, or drop --arrival ref and take the worktree arrival",
			target)
	default:
		admits[ledger.RefArrival] = true
		admits[ledger.WorktreeArrival] = true
	}
	return admits, why
}

// samePath reports whether two paths name one directory, following symlinks.
// macOS puts every temporary directory behind one, so a string comparison
// answers *not here* for the tree the process is standing in.
func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if resolved, err := filepath.EvalSymlinks(a); err == nil {
		a = resolved
	}
	if resolved, err := filepath.EvalSymlinks(b); err == nil {
		b = resolved
	}
	return filepath.Clean(a) == filepath.Clean(b)
}
