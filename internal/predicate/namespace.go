package predicate

import (
	"sort"

	"github.com/pbleisch/kept/internal/model"
)

// Observed is the hold_when namespace: what each of an artifact's named
// gatherers just returned, and the history prior() reaches into.
//
// It is a view over observations the caller already has. Nothing here gathers,
// nothing here writes, and nothing here knows what a verdict is.
type Observed struct {
	// Now is the observation each declared gatherer produced this time, keyed
	// by the name the predicate uses — the key of the artifact's gather map,
	// which is not necessarily the gatherer's own name.
	Now map[string]model.Observation
	// History is every observation available for those same names, in any
	// order. prior() folds it; the caller does not have to sort or filter it,
	// because the filtering rule is the part that is silently wrong when it is
	// left out.
	History map[string][]model.Observation
}

// Scope builds the evaluation scope. obs.<name> is the observation's data, and
// an observation that failed to be one is absent from obs entirely — a failed
// gatherer yields unknown, never not-kept, and the way that is guaranteed here
// is that there is nothing for the predicate to read.
func (o Observed) Scope() Scope {
	obs := make(map[string]any, len(o.Now))
	for name, ob := range o.Now {
		if ob.Err != nil {
			// 0004: a failed gatherer produces no observation, so no predicate
			// runs against one. The tick should not have offered it; if it
			// does, the answer is still unknown rather than whatever partial
			// data the failure left behind.
			continue
		}
		obs[name] = ob.Data
	}
	return Scope{
		Roots: map[string]any{"obs": obs},
		Prior: o.prior,
	}
}

// prior resolves prior(<name>) to the previous observation of that gatherer,
// whatever verdict it produced, reaching back exactly one.
//
// **It folds within a world lineage**, and that is the line that is silently
// wrong if it is omitted. The thread loop gathers three times against two
// worlds, so a history that mixed them would make "the set of tests never
// shrinks" compare a box against the origin — two trees presented as a change
// over time, which is the second of the three failures 0014 was written to
// stop. A thread matches a thread, and empty matches empty.
//
// Order is event time, then id. Never the order the caller happened to hand
// them over: the ledger is union-merged and interleaves arbitrarily, and a fold
// that depends on file order looks correct in every single-run test.
func (o Observed) prior(name string) any {
	current, ok := o.Now[name]
	if !ok || current.Err != nil {
		return Unknown{Because: "prior(" + name + "): nothing was observed now, so there is nothing this is prior to"}
	}

	var lineage []model.Observation
	for _, ob := range o.History[name] {
		if ob.Err != nil || ob.ID == current.ID {
			continue
		}
		if ob.World.Thread != current.World.Thread {
			continue
		}
		if !before(ob, current) {
			continue
		}
		lineage = append(lineage, ob)
	}
	if len(lineage) == 0 {
		return Unknown{Because: "prior(" + name + "): nothing was observed before this"}
	}
	sort.Slice(lineage, func(i, j int) bool { return before(lineage[i], lineage[j]) })
	return lineage[len(lineage)-1].Data
}

// before orders two observations by event time, then id. The id is the
// tiebreaker rather than anything about who wrote them, because an observation
// names no run: it is a fact about a tree, and a fact does not have an author.
// See docs/decision/0034.
func before(a, b model.Observation) bool {
	if !a.At.Equal(b.At) {
		return a.At.Before(b.At)
	}
	return a.ID < b.ID
}

// Output is the extractor namespace: what a program produced. 0023 requires
// `command` to name what it pulls out of that rather than storing the lot, and
// these three are what there is to pull it out of.
type Output struct {
	Stdout string
	Stderr string
	Exit   int
}

// Scope builds the evaluation scope for a gatherer's declared fields.
func (o Output) Scope() Scope {
	return Scope{Roots: map[string]any{
		"stdout": o.Stdout,
		"stderr": o.Stderr,
		"exit":   int64(o.Exit),
	}}
}
