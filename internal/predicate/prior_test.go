package predicate

import (
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/model"
)

func at(seconds int) time.Time {
	return time.Date(2026, 1, 1, 0, 0, seconds, 0, time.UTC)
}

// observed builds one observation. The world is named by its thread, because
// the thread is the half of the world prior() folds on: a gather inside a
// repair and the gather on the tree that ships are the same gatherer in two
// lineages.
func observed(id, thread string, when time.Time, cases ...string) model.Observation {
	list := make([]any, len(cases))
	for i, c := range cases {
		list[i] = c
	}
	return model.Observation{
		ID:    id,
		World: model.World{Thread: thread, Tree: id},
		At:    when,
		Data:  map[string]any{"cases": list},
	}
}

func priorScope(now model.Observation, history ...model.Observation) Scope {
	return Observed{
		Now:     map[string]model.Observation{"tests": now},
		History: map[string][]model.Observation{"tests": history},
	}.Scope()
}

func neverShrinks(t *testing.T, sc Scope) Verdict {
	t.Helper()
	e, err := Parse(`all(prior(tests).cases, c => c in obs.tests.cases)`, HoldWhen)
	if err != nil {
		t.Fatal(err)
	}
	v, err := e.Hold(sc)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// The promise the whole three-part model was built for: the set of tests never
// shrinks. It needs the previous measurement whatever verdict it produced, and
// no boolean check can express it at any level of sophistication.
func TestPriorAnswersTheSetOfTestsNeverShrinks(t *testing.T) {
	grew := priorScope(
		observed("t2", "", at(20), "TestA", "TestB", "TestC"),
		observed("t1", "", at(10), "TestA", "TestB"),
	)
	if got := neverShrinks(t, grew).Value; got != model.Kept {
		t.Errorf("a grown set is %s", got)
	}

	shrank := priorScope(
		observed("t2", "", at(20), "TestA"),
		observed("t1", "", at(10), "TestA", "TestB"),
	)
	if got := neverShrinks(t, shrank).Value; got != model.NotKept {
		t.Errorf("a shrunk set is %s", got)
	}
}

// **The line that is silently wrong if it is omitted.** prior() folds the
// observations whose thread matches the current one — empty matching empty —
// so a gather in a box is never the predecessor of a gather on the tree that
// ships. Without the rule, "never shrinks" compares a box against the origin
// and presents two trees as a change over time, which is the second of the
// three failures 0014 was written to stop.
//
// The history here interleaves deliberately: the box's observations are newer
// than the workspace's, so an unfiltered fold would pick one of them every
// time.
func TestPriorFoldsWithinAWorldLineage(t *testing.T) {
	workspaceNow := observed("w2", "", at(50), "TestA", "TestB", "TestC")
	history := []model.Observation{
		observed("w1", "", at(10), "TestA", "TestB"),
		// A repair's box, measured later and holding a smaller set — exactly
		// the shape that makes an unfiltered fold answer not-kept.
		observed("b1", "thread-1", at(20), "TestA"),
		observed("b2", "thread-1", at(30), "TestA"),
		observed("b3", "thread-2", at(40), "TestZ"),
	}
	if got := neverShrinks(t, priorScope(workspaceNow, history...)).Value; got != model.Kept {
		t.Fatalf("the workspace's verdict is %s, so a box's observation was folded into the workspace's lineage", got)
	}

	// And the box's own lineage answers for itself, from its own predecessor
	// rather than from the workspace's newer one.
	boxNow := observed("b4", "thread-1", at(60), "TestA", "TestB")
	if got := neverShrinks(t, priorScope(boxNow, append(history, workspaceNow)...)).Value; got != model.Kept {
		t.Fatalf("the box's verdict is %s", got)
	}
	boxShrank := observed("b4", "thread-1", at(60))
	if got := neverShrinks(t, priorScope(boxShrank, append(history, workspaceNow)...)).Value; got != model.NotKept {
		t.Fatalf("a box that lost a test is %s", got)
	}
}

// It reaches back exactly one. A window needs a retention rule, an ordering
// rule and a syntax, and nothing has forced any of them; guessing now would be
// permanent in every predicate written under the guess.
func TestPriorReachesBackExactlyOne(t *testing.T) {
	sc := priorScope(
		observed("t3", "", at(30), "TestA"),
		observed("t1", "", at(10), "TestA", "TestB", "TestC"),
		observed("t2", "", at(20), "TestA"),
	)
	// The one before this one had only TestA, so nothing shrank. Reaching two
	// back would find TestB and TestC and answer not-kept.
	if got := neverShrinks(t, sc).Value; got != model.Kept {
		t.Fatalf("verdict is %s, so prior() reached further back than one", got)
	}
}

// Event time, then id. Never the order the caller handed them over: the ledger
// is union-merged and interleaves arbitrarily, and a fold that depends on file
// order looks correct in every single-run test.
func TestPriorOrdersByEventTimeThenID(t *testing.T) {
	now := observed("t9", "", at(90), "TestA")
	shuffled := []model.Observation{
		observed("t1", "", at(10), "TestA"),
		observed("t3", "", at(30), "TestA", "TestB"),
		observed("t2", "", at(20), "TestA"),
	}
	// t3 is the latest by time and is written first. If the fold took the last
	// one it was handed, it would find t2 and answer kept.
	if got := neverShrinks(t, priorScope(now, shuffled...)).Value; got != model.NotKept {
		t.Fatalf("verdict is %s, so the fold read file order rather than event time", got)
	}

	// Two at one instant are separated by id, which is the tiebreaker because
	// an observation names no run: it is a fact about a tree, and a fact does
	// not have an author. See docs/decision/0034.
	tied := []model.Observation{
		observed("bbb", "", at(10), "TestA", "TestB"),
		observed("aaa", "", at(10), "TestA"),
	}
	if got := neverShrinks(t, priorScope(now, tied...)).Value; got != model.NotKept {
		t.Fatalf("verdict is %s, so the tie was not broken by id", got)
	}
}

// No history is unknown, not false. A promise about a change over time cannot
// be broken by a first measurement, and reporting not-kept for one would drive
// a repair against a comparison that never happened.
func TestPriorWithNoHistoryIsUnknown(t *testing.T) {
	only := observed("t1", "", at(10), "TestA")
	got := neverShrinks(t, priorScope(only))
	if got.Value != model.Unknown {
		t.Fatalf("a first measurement is %s", got.Value)
	}
	if got.Because == "" {
		t.Fatal("nothing said why")
	}

	// A history holding only the current observation is no history: it is what
	// the comparison is against, not what it compares to.
	got = neverShrinks(t, priorScope(only, only))
	if got.Value != model.Unknown {
		t.Fatalf("an observation compared against itself is %s", got.Value)
	}

	// And a history holding only observations from another lineage is no
	// history either.
	got = neverShrinks(t, priorScope(only, observed("b1", "thread-1", at(5), "TestA", "TestB")))
	if got.Value != model.Unknown {
		t.Fatalf("a box's observation answered for the workspace: %s", got.Value)
	}
}

// It returns the previous observation whatever verdict it produced. A
// last-good variant is speculative and prior_kept() is additive if something
// ever needs it — but "never shrinks" needs the previous measurement even when
// the promise did not hold, which is the case this exists for.
func TestPriorReturnsThePreviousObservationWhateverVerdictItProduced(t *testing.T) {
	// The previous measurement broke the promise: it had a test this one does
	// not. The one before that was fine. prior() must still return the broken
	// one, so the comparison is against what was last true of the world.
	sc := priorScope(
		observed("t3", "", at(30), "TestA", "TestB"),
		observed("t1", "", at(10), "TestA"),
		observed("t2", "", at(20), "TestA", "TestB", "TestC"),
	)
	if got := neverShrinks(t, sc).Value; got != model.NotKept {
		t.Fatalf("verdict is %s, so prior() skipped the observation that did not hold", got)
	}
}

// A scope built without any history at all answers unknown rather than
// erroring. A tick that offered no history has measured nothing about the past,
// which is what unknown means.
func TestPriorWithNoHistoryOfferedIsUnknownAndNotAnError(t *testing.T) {
	e, err := Parse(`all(prior(tests).cases, c => c in obs.tests.cases)`, HoldWhen)
	if err != nil {
		t.Fatal(err)
	}
	v, err := e.Hold(Scope{Roots: map[string]any{"obs": map[string]any{"tests": map[string]any{"cases": []any{}}}}})
	if err != nil {
		t.Fatalf("a scope with no Prior errored: %v", err)
	}
	if v.Value != model.Unknown {
		t.Fatalf("verdict is %s", v.Value)
	}
}

// A gatherer that failed has no observation, so there is nothing for a prior
// one to be prior to.
func TestPriorAgainstAFailedGathererIsUnknown(t *testing.T) {
	failed := observed("t2", "", at(20), "TestA")
	failed.Err = errFailed{}
	got := neverShrinks(t, priorScope(failed, observed("t1", "", at(10), "TestA")))
	if got.Value != model.Unknown {
		t.Fatalf("verdict is %s", got.Value)
	}
}
