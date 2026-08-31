package assess

import (
	"sort"
	"strings"

	"github.com/pbleisch/kept/internal/gather"
	"github.com/pbleisch/kept/internal/model"
)

// Kind is what sort of thing observed the world.
//
// Autonomy is bounded by the instrument rather than by the model, so the grade
// is a property of the promise and is known when the promise is written. See
// docs/document/model.md, "Autonomy".
type Kind string

const (
	// Structural: the measurement is over the shape of the tree, and kept wrote
	// the instrument.
	Structural Kind = "structural"
	// Pattern: the measurement is a match over text or paths.
	Pattern Kind = "pattern"
	// Judgement: a model observed the world.
	Judgement Kind = "judgement"
	// Reading: a person observed the world, and the truth decays back to
	// unknown on a cadence.
	Reading Kind = "reading"
)

// Gate is what must happen before a repair of a promise may land.
type Gate string

const (
	// AutoLand: inside the repair scope, nothing else is required.
	AutoLand Gate = "auto-land"
	// AssessorPasses: the checklist must find nothing.
	AssessorPasses Gate = "assessor"
	// AssessorAndSpotCheck: the checklist, and a person reading a sample of
	// the diff.
	AssessorAndSpotCheck Gate = "assessor-and-spot-check"
	// PersonLands: a person, on the work itself.
	PersonLands Gate = "person"
)

// strength orders the gates from the one that asks least to the one that asks
// most, which is what "the weakest sets the gate" is measured on.
//
// It is a function over a closed set rather than an integer field on each gate,
// so a gate added without a place in the order fails to compile here rather
// than silently sorting as zero — which is the weakest, and therefore the
// direction that lands things nobody meant to land.
func strength(g Gate) int {
	switch g {
	case AutoLand:
		return 0
	case AssessorPasses:
		return 1
	case AssessorAndSpotCheck:
		return 2
	case PersonLands:
		return 3
	}
	return -1
}

// gates is the table docs/document/model.md declares, as far as the catalogue
// has built instruments to put in it.
//
// It is keyed by the name a promise writes rather than by a field on the
// gatherer, because the grade is about who wrote the instrument and what sort
// of thing it observes — which is a claim kept makes, not one a gatherer makes
// about itself. A third party's gatherer could otherwise declare itself
// structural.
var gates = map[Kind]Gate{
	Structural: AutoLand,
	Pattern:    AssessorPasses,
	Judgement:  AssessorAndSpotCheck,
	Reading:    PersonLands,
}

// kinds is what each built-in gatherer is.
//
// `command` is deliberately absent, and so is everything under the third-party
// marker. Neither is an oversight and neither may be defaulted: `command` runs
// whatever a promise names, so what it observes is not knowable from the name,
// and a gatherer kept did not write is not kept's to grade — the group in a
// name says what the tree must have, never who vouches for the measurement.
// Absent means kept has not graded it, which [Grade] reports as *no gate* and
// never as one of the four.
var kinds = map[string]Kind{
	gather.Corpus:  Structural,
	gather.Tests:   Structural,
	gather.Imports: Structural,
	gather.Symbols: Structural,
}

// Grading is what a promise's instruments say about how a repair of it may
// land.
type Grading struct {
	// Gate is the weakest gate any of the promise's instruments sets, and it is
	// meaningful only when Graded is true.
	Gate Gate
	// Graded reports whether every instrument the promise names has a grade.
	// False is *kept has not graded one of these*, which is not a gate of any
	// strength: nothing has answered is not an answer.
	Graded bool
	// Ungraded names the gatherers with no row, sorted, so a report says which
	// rather than only that there was one.
	Ungraded []string
	// Weakest names the gatherer the gate came from, empty when Graded is
	// false.
	Weakest string
}

// Grade is the gate a promise's instruments set.
//
// The weakest one sets it, which is why a promise mixing a reading into
// anything else waits for a person: a gate is a claim about the whole promise,
// and the strongest instrument in it cannot vouch for the weakest.
//
// A promise naming no instrument is not graded either. It measures nothing, so
// no repair of it can be judged by re-derivation, and a gate derived from an
// empty set would be the *all() over an empty list is true* mistake wearing a
// different hat.
func Grade(gather map[string]model.GatherSpec) Grading {
	if len(gather) == 0 {
		return Grading{}
	}
	out := Grading{Graded: true}
	seen := map[string]bool{}
	for _, name := range sortedNames(gather) {
		gatherer := gather[name].Gatherer
		kind, graded := kinds[gatherer]
		if !graded {
			if !seen[gatherer] {
				seen[gatherer] = true
				out.Ungraded = append(out.Ungraded, gatherer)
			}
			out.Graded = false
			continue
		}
		gate := gates[kind]
		if out.Weakest == "" || strength(gate) > strength(out.Gate) {
			out.Gate, out.Weakest = gate, gatherer
		}
	}
	sort.Strings(out.Ungraded)
	if !out.Graded {
		out.Gate, out.Weakest = "", ""
	}
	return out
}

// String is the one line a report prints about a grade.
//
// It says what the gate requires rather than what happens next, because nothing
// auto-lands: the grade is what a landing gate will read, and a line claiming a
// land would describe a mechanism that is not there.
//
// The word is *grade* rather than *gate*, and that is not a synonym chosen for
// variety. A promise has one grade, derived from its instruments and fixed
// while its wording is; a thread passes through gates, and which one it is
// waiting at is the act/wait/suppress/escalate policy, which is open question 3
// and is not built. A line printing one under the other's name would be
// answering a question nothing has answered.
func (g Grading) String() string {
	if !g.Graded {
		return "grade: none — kept has not graded " + strings.Join(g.Ungraded, ", ")
	}
	switch g.Gate {
	case AutoLand:
		return "grade: nothing beyond the repair scope, set by " + g.Weakest
	case AssessorPasses:
		return "grade: the checklist must find nothing, set by " + g.Weakest
	case AssessorAndSpotCheck:
		return "grade: the checklist, and a person reading the diff, set by " + g.Weakest
	case PersonLands:
		return "grade: a person, set by " + g.Weakest
	}
	return "grade: " + string(g.Gate)
}

func sortedNames(gather map[string]model.GatherSpec) []string {
	out := make([]string, 0, len(gather))
	for name := range gather {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
