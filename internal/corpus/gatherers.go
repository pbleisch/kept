package corpus

import (
	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/predicate"
)

// Options are what a load needs that the corpus does not say about itself.
type Options struct {
	// Gatherers is the catalogue an artifact's gather map is checked against.
	//
	// Nil runs the half of the check that needs no catalogue — every
	// obs.<name> a predicate reads names a gatherer the artifact declares —
	// and skips the half that does, which is the fallback
	// predicate.Gathered is written for. A corpus loaded without one is not
	// wrong; it is checked less.
	Gatherers Gatherers
}

// Gatherers is what an artifact's gather map is checked against when the corpus
// loads.
//
// It is an interface here, and internal/gather is what satisfies it. The corpus
// package reads a corpus and runs nothing, and a gatherer's implementation
// reaches the ledger, which reaches back to the binding hash and so to the
// corpus — so importing one from here would close a loop asserting that reading
// a corpus means knowing how to measure the world. Whoever holds both hands the
// one to the other.
type Gatherers interface {
	// Check reports every gather entry that cannot be run as written: a
	// gatherer nothing answers to, an arg nothing accepts, an extractor that
	// will not parse.
	//
	// An error that also answers NoSuchGatherer() true named a gatherer that
	// does not exist, which is a different mistake from one whose args are
	// wrong and is reported as a different kind.
	Check(gather map[string]model.GatherSpec) []error

	// Sources is what a hold_when's obs.<name>.<field> references are checked
	// against: which names the artifact declares a gatherer for, and what each
	// of those gatherers says it returns.
	Sources(gather map[string]model.GatherSpec) predicate.Sources
}
