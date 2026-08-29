package land

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/ledger"
)

const arrivalDeadline = 60 * time.Second

// TestTheArrivalTableIsAnsweredCellByCell is docs/decision/0015's table, all
// eight cells, and it is the acceptance criteria for this whole arrangement.
//
//	the target is                     the tree is   ref          worktree
//	checked out in no worktree        —             resolved     refused
//	checked out here                  clean         resolved     on request
//	checked out here                  dirty         refused      resolved
//	checked out in another worktree   —             refused      refused
//
// The rows are separate workspaces because the row is a property of the
// repository rather than of the request, and a fixture shared across them would
// be one test asserting four different worlds.
func TestTheArrivalTableIsAnsweredCellByCell(t *testing.T) {
	t.Run("checked out in no worktree", func(t *testing.T) {
		w := newLandWorkspace(t)
		// A remote branch lives on a remote, so no working tree holds it. It is
		// the same row a local branch nobody has checked out is in, and both
		// are asserted, because "no worktree" is the condition rather than
		// "shared".
		w.git("branch", "shipping", "main")

		for _, target := range []Target{Origin(), Local("shipping")} {
			held := w.inspect(target)
			if held.Held() {
				t.Fatalf("%s reads as checked out in %s", target, held.Worktree)
			}
			if got := w.resolve(target, ""); got.Arrival != ledger.RefArrival {
				t.Fatalf("%s resolved %s, and the ref is the only arrival this row admits", target, got.Arrival)
			}
			if got := w.resolve(target, ledger.RefArrival); got.Arrival != ledger.RefArrival {
				t.Fatalf("%s refused a pinned ref arrival: %+v", target, got)
			}
			// Refused by name, never satisfied by the other. A worktree
			// arrival that quietly became a ref advance is how the weaker
			// case borrows the stronger case's name.
			why := w.refuses(target, ledger.WorktreeArrival)
			if !strings.Contains(why, "no working tree at "+target.String()) {
				t.Fatalf("the refusal does not name what is missing: %q", why)
			}
		}
	})

	t.Run("checked out here and clean", func(t *testing.T) {
		w := newLandWorkspace(t)
		held := w.inspect(Local("main"))
		if !held.Held() || !held.Here || held.Dirty {
			t.Fatalf("main reads as %+v in the tree that has it checked out and clean", held)
		}
		// The ref is resolved, because git updates the files along with the ref
		// and a commit reaching a ref is the stronger guarantee.
		if got := w.resolve(Local("main"), ""); got.Arrival != ledger.RefArrival {
			t.Fatalf("a clean checkout resolved %s", got.Arrival)
		}
		// And the worktree arrival is available on request rather than refused:
		// this is the one cell where both are possible.
		if got := w.resolve(Local("main"), ledger.WorktreeArrival); got.Arrival != ledger.WorktreeArrival {
			t.Fatalf("a clean checkout refused a worktree arrival that was asked for: %+v", got)
		}
	})

	t.Run("checked out here and dirty", func(t *testing.T) {
		w := newLandWorkspace(t)
		w.write("sub/a/note/first.md", noteFile("first")+"\nthe operator is mid-sentence")

		held := w.inspect(Local("main"))
		if !held.Here || !held.Dirty {
			t.Fatalf("a dirty checkout reads as %+v", held)
		}
		if got := w.resolve(Local("main"), ""); got.Arrival != ledger.WorktreeArrival {
			t.Fatalf("a dirty checkout resolved %s, and git will not move a branch out from under one", got.Arrival)
		}
		why := w.refuses(Local("main"), ledger.RefArrival)
		if !strings.Contains(why, "not clean") {
			t.Fatalf("the refusal does not say the tree is dirty: %q", why)
		}
	})

	t.Run("checked out in another worktree", func(t *testing.T) {
		w := newLandWorkspace(t)
		elsewhere := filepath.Join(t.TempDir(), "elsewhere")
		w.git("worktree", "add", "--quiet", "-b", "elsewhere", elsewhere, "HEAD")
		t.Cleanup(func() { _, _ = gitTry(w.root, "worktree", "remove", "--force", elsewhere) })

		held := w.inspect(Local("elsewhere"))
		if !held.Held() || held.Here {
			t.Fatalf("a branch checked out elsewhere reads as %+v", held)
		}
		// Both refused, and the ref refusal names the worktree, which is a
		// thing a person can go and look at.
		for _, pinned := range []ledger.Arrival{ledger.RefArrival, ledger.WorktreeArrival} {
			why := w.refuses(Local("elsewhere"), pinned)
			if !strings.Contains(why, elsewhere) {
				t.Fatalf("the %s refusal does not name the worktree: %q", pinned, why)
			}
		}
		// And resolving refuses too, rather than picking whichever is left.
		if _, err := Resolve(context.Background(), w.root, Local("elsewhere"), "", arrivalDeadline); err == nil {
			t.Fatal("a branch checked out in another worktree resolved an arrival")
		} else if !strings.Contains(err.Error(), elsewhere) {
			t.Fatalf("the refusal does not name the worktree: %v", err)
		}
	})
}

// TestAPinnedArrivalIsNeverSatisfiedByTheOther is the rule the table's four
// refusals exist for, stated once over all of them: docs/decision/0015 says
// "pinning an impossible one is refused by name rather than quietly satisfied
// by the other", and the failure it prevents is a record whose arrival is not
// what happened.
func TestAPinnedArrivalIsNeverSatisfiedByTheOther(t *testing.T) {
	w := newLandWorkspace(t)
	w.write("sub/a/note/first.md", noteFile("first")+"\nheld")

	// The dirty row admits only the worktree arrival. A ref arrival asked for
	// here must refuse, never come back as a worktree arrival.
	plan, err := Resolve(context.Background(), w.root, Local("main"), ledger.RefArrival, arrivalDeadline)
	if err == nil {
		t.Fatalf("a ref arrival onto a dirty checkout resolved as %+v", plan)
	}

	// The other direction, in the row that admits only the ref.
	if plan, err := Resolve(context.Background(), w.root, Origin(), ledger.WorktreeArrival, arrivalDeadline); err == nil {
		t.Fatalf("a worktree arrival onto a remote branch resolved as %+v", plan)
	}
}

// TestAnArrivalThisBuildDoesNotKnowIsRefusedByName: --arrival takes a value a
// person types, and a value nothing knows is an authoring error rather than a
// reason to fall back on the default.
func TestAnArrivalThisBuildDoesNotKnowIsRefusedByName(t *testing.T) {
	w := newLandWorkspace(t)
	_, err := Resolve(context.Background(), w.root, Local("main"), "email", arrivalDeadline)
	if err == nil || !strings.Contains(err.Error(), "email") {
		t.Fatalf("an unknown arrival was answered with %v", err)
	}
}

// TestAWorktreeArrivalIsAlwaysTheLocalRegime pins the half of
// docs/decision/0015 the ledger refuses on: "a worktree arrival's target is
// always the branch this working tree has checked out, so it is always the
// local regime — the strongest check and the weakest arrival never combine."
//
// The mechanism is that a shared target is checked out in no working tree, so
// the row that admits a worktree arrival cannot be reached with one. This walks
// every target the fixture can produce and checks the pair the resolver
// actually returns, rather than restating the rule.
func TestAWorktreeArrivalIsAlwaysTheLocalRegime(t *testing.T) {
	w := newLandWorkspace(t)
	w.git("branch", "shipping", "main")
	dirty := newLandWorkspace(t)
	dirty.write("sub/a/note/first.md", noteFile("first")+"\nheld")

	for _, c := range []struct {
		in     *landWorkspace
		target Target
	}{
		{w, Origin()},
		{w, Local("main")},
		{w, Local("shipping")},
		{dirty, Local("main")},
	} {
		for _, pinned := range []ledger.Arrival{"", ledger.RefArrival, ledger.WorktreeArrival} {
			plan, err := Resolve(context.Background(), c.in.root, c.target, pinned, arrivalDeadline)
			if err != nil {
				continue
			}
			if plan.Arrival == ledger.WorktreeArrival && plan.Regime != ledger.LocalRegime {
				t.Fatalf("%s resolved a worktree arrival under the %s regime", c.target, plan.Regime)
			}
			// The same pair the ledger refuses, asked of the resolver so the
			// two ends cannot drift: a plan that would be refused on the way
			// into the record must not be produced here.
			if plan.Regime == ledger.SharedRegime && plan.Arrival != ledger.RefArrival {
				t.Fatalf("%s resolved %+v, which the ledger refuses to record", c.target, plan)
			}
		}
	}
}

// TestTheDefaultTargetIsWhereTheOperatorsOwnWorkGoes is
// docs/decision/0015's resolution table: the upstream of the branch this
// working tree has checked out, the branch itself when it has none, and nothing
// at all on a detached HEAD.
func TestTheDefaultTargetIsWhereTheOperatorsOwnWorkGoes(t *testing.T) {
	w := newLandWorkspace(t)

	// No upstream configured yet: the branch is where this operator's work
	// goes, and it is a local target that reaches no network.
	target, err := ResolveTarget(context.Background(), w.root, arrivalDeadline)
	if err != nil {
		t.Fatal(err)
	}
	if target.Shared() || target.Branch != "main" {
		t.Fatalf("a branch with no upstream resolved %+v", target)
	}
	if target.Regime() != ledger.LocalRegime {
		t.Fatalf("a target naming no remote is the %s regime", target.Regime())
	}

	// With one, git's own answer is the upstream, and the regime follows.
	w.git("fetch", "--quiet", "origin")
	w.git("branch", "--set-upstream-to=origin/main", "main")
	target, err = ResolveTarget(context.Background(), w.root, arrivalDeadline)
	if err != nil {
		t.Fatal(err)
	}
	if target.Remote != "origin" || target.Branch != "main" {
		t.Fatalf("a tracking branch resolved %+v", target)
	}
	if target.Regime() != ledger.SharedRegime {
		t.Fatalf("a target naming a remote is the %s regime", target.Regime())
	}

	// A detached HEAD is not a branch, so it is not a place a land can reach,
	// and the refusal says what to type instead.
	w.git("checkout", "--quiet", "--detach", "HEAD")
	if _, err := ResolveTarget(context.Background(), w.root, arrivalDeadline); err == nil {
		t.Fatal("a detached HEAD resolved a landing target")
	} else if !strings.Contains(err.Error(), "--onto") {
		t.Fatalf("the refusal does not say what to type: %v", err)
	}
}

// TestATargetIsReadAsARemoteOnlyWhenThisRepositoryHasOne: the first segment is
// a remote when this repository configures one by that name, and part of a
// branch name otherwise — which is what makes `--onto main` land locally in a
// repository that also has an `origin`.
func TestATargetIsReadAsARemoteOnlyWhenThisRepositoryHasOne(t *testing.T) {
	w := newLandWorkspace(t)
	for _, c := range []struct {
		spec   string
		remote string
		branch string
	}{
		{"origin/main", "origin", "main"},
		{"main", "", "main"},
		{"origin/feature/x", "origin", "feature/x"},
		// No remote called "feature", so the whole string is a branch name.
		{"feature/x", "", "feature/x"},
		// The escape hatch for a branch whose own name would be read as a
		// remote. It is spelled in full and is never a second syntax.
		{"refs/heads/origin/main", "", "origin/main"},
	} {
		got, err := ParseTarget(context.Background(), w.root, c.spec, arrivalDeadline)
		if err != nil {
			t.Fatalf("%s: %v", c.spec, err)
		}
		if got.Remote != c.remote || got.Branch != c.branch {
			t.Fatalf("%s parsed as %+v, wanted remote %q branch %q", c.spec, got, c.remote, c.branch)
		}
	}
}

// --- fixture helpers -----------------------------------------------------

func (w *landWorkspace) inspect(target Target) Checkout {
	w.t.Helper()
	held, err := Inspect(context.Background(), w.root, target, arrivalDeadline)
	if err != nil {
		w.t.Fatal(err)
	}
	return held
}

func (w *landWorkspace) resolve(target Target, pinned ledger.Arrival) Plan {
	w.t.Helper()
	plan, err := Resolve(context.Background(), w.root, target, pinned, arrivalDeadline)
	if err != nil {
		w.t.Fatalf("%s with %q pinned: %v", target, pinned, err)
	}
	return plan
}

// refuses is the refusal a cell of the table produces, and it fails the test
// when the cell did not refuse — a refusal that is quietly satisfied by the
// other arrival is the defect this whole table exists to prevent.
func (w *landWorkspace) refuses(target Target, pinned ledger.Arrival) string {
	w.t.Helper()
	plan, err := Resolve(context.Background(), w.root, target, pinned, arrivalDeadline)
	if err == nil {
		w.t.Fatalf("%s admitted a %s arrival: %+v", target, pinned, plan)
	}
	return err.Error()
}
