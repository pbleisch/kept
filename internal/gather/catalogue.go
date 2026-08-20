package gather

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/predicate"
)

// Catalogue is the gatherers a promise may name, keyed by the bare name it
// writes under `gatherer:`. A promise names gatherers as a map and the gatherer
// defaults to the map key, so a single-gatherer promise names it once. See
// docs/decision/0005.
type Catalogue map[string]Gatherer

// Builtin is what kept ships.
//
// It is a function rather than a package variable so that no caller can add to
// or remove from the one map every corpus is checked against. A third-party
// gatherer arrives as its own Catalogue, over the interface declaration that is
// already this one's contract — open question 8.
func Builtin() Catalogue {
	return Catalogue{
		Command: command{},
		Tests:   tests{},
		Imports: imports{},
		Symbols: symbols{},
		Corpus:  corpusGatherer{},
	}
}

// The names the built-ins are written under.
const (
	Command = "command"
	Tests   = "tests"
	Imports = "imports"
	Symbols = "symbols"
	Corpus  = "corpus"
)

// BadSpec is a gather entry that cannot be run as written: a gatherer nothing
// answers to, an arg nothing accepts, an extractor that will not parse.
//
// It is reported when the corpus loads, which is the whole point of a gatherer
// declaring its fields: an authoring mistake is a thing a person fixes, not an
// `unknown` discovered in the code path that decides a verdict. See
// docs/decision/0036.
type BadSpec struct {
	// Name is the binding name in the predicate namespace — the key of the
	// gather map, which is not necessarily the gatherer's own name.
	Name string
	// Gatherer is what the entry asked for.
	Gatherer string
	// Unknown is true when the catalogue has nothing by that name, which is a
	// different mistake from an entry whose args are wrong.
	Unknown bool
	Err     error
}

func (e *BadSpec) Error() string {
	if e.Unknown {
		return fmt.Sprintf("%s names the %s gatherer, and there is no such gatherer", e.Name, e.Gatherer)
	}
	return fmt.Sprintf("%s: %v", e.Name, e.Err)
}

func (e *BadSpec) Unwrap() error { return e.Err }

// NoSuchGatherer reports whether this entry named a gatherer that does not
// exist, which is a different mistake from one whose args are wrong and is
// reported as a different kind.
//
// It is a method rather than a bare field so that a caller which cannot import
// this package — the corpus, which must not — can still tell the two apart
// without reading a message. That is the same rule the predicate language is
// built on: nothing tells two failures apart by matching English prose.
func (e *BadSpec) NoSuchGatherer() bool { return e.Unknown }

// Check reports every gather entry the catalogue cannot answer for.
//
// Nothing here measures anything, so nothing here can be Unknown: an artifact's
// gather map is authored, and everything wrong with it is wrong before the
// world is looked at.
func (c Catalogue) Check(gather map[string]model.GatherSpec) []error {
	var out []error
	for _, name := range sortedKeys(gather) {
		spec := gather[name]
		g, ok := c[spec.Gatherer]
		if !ok {
			out = append(out, &BadSpec{Name: name, Gatherer: spec.Gatherer, Unknown: true})
			continue
		}
		if err := refuseTickWrittenReads(g); err != nil {
			out = append(out, &BadSpec{Name: name, Gatherer: spec.Gatherer, Err: err})
			continue
		}
		if _, err := g.Declares(spec.Args); err != nil {
			out = append(out, &BadSpec{Name: name, Gatherer: spec.Gatherer, Err: err})
		}
	}
	return out
}

// Sources is what a static check of a hold_when knows about the observations it
// may read: the names the artifact declares a gatherer for, and what each of
// those gatherers says it returns.
//
// The second half is what a catalogue adds. Without it predicate.Gathered
// answers the name half and defers the field half, which is exactly this seam.
func (c Catalogue) Sources(gather map[string]model.GatherSpec) predicate.Sources {
	return sources{gather: gather, catalogue: c}
}

type sources struct {
	gather    map[string]model.GatherSpec
	catalogue Catalogue
}

func (s sources) Declared(name string) bool {
	_, ok := s.gather[name]
	return ok
}

// Fields answers what the gatherer bound to a name returns. The second result
// is false when the declaration is not known at all — an unknown gatherer, or
// args it refuses — and false means the field half of the check is skipped
// rather than that the gatherer returns nothing. A promise whose gather entry
// is itself wrong is told about that entry once, and is not also told that
// every field it reads is missing.
func (s sources) Fields(name string) ([]string, bool) {
	spec, ok := s.gather[name]
	if !ok {
		return nil, false
	}
	g, ok := s.catalogue[spec.Gatherer]
	if !ok {
		return nil, false
	}
	fields, err := g.Declares(spec.Args)
	if err != nil {
		return nil, false
	}
	return fields, true
}

// Names is what the artifact does gather, for a refusal that says what is
// available rather than only what is not.
func (s sources) Names() []string { return sortedKeys(s.gather) }

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// refuseTickWrittenReads is docs/decision/0046's guard, asked of one gatherer.
//
// A gatherer may declare a dependency only on ledger acts a person performs.
// Truth and dispositions are appended on every tick, so a gatherer naming one
// would gather, cause an append, move its own scope digest and be due again —
// which is the tick feeding itself one level below where the `.kept/`
// exclusion reaches, and it would converge on nothing.
//
// It runs where the corpus loads, beside every other thing a person can fix
// before a verdict depends on it. A third-party catalogue is checked by the
// same call as a built-in, which is the point: this is the rule that keeps the
// declaration from becoming a way to read the whole ledger.
func refuseTickWrittenReads(g Gatherer) error {
	var refused []string
	for _, k := range g.Reads() {
		if !ledger.HumanPaced(k) {
			refused = append(refused, string(k))
		}
	}
	if len(refused) == 0 {
		return nil
	}
	sort.Strings(refused)
	return fmt.Errorf(
		"the %s gatherer declares a read of %s, and the tick writes %s every pass; a gatherer may declare only acts a person performs",
		g.Name(), joinWords(refused), plural(len(refused), "that", "those"))
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// refuseUnknownArgs is the guard every gatherer runs over what a promise wrote.
//
// A gatherer that quietly ignored `package:` for `packages:` would measure
// something other than what was asked and say nothing about it, and because the
// args bind, the mistake would be baked into a ratified promise. So an arg
// nothing accepts is an authoring error, named alongside what is accepted.
func refuseUnknownArgs(args map[string]any, accepted ...string) error {
	allowed := make(map[string]bool, len(accepted))
	for _, a := range accepted {
		allowed[a] = true
	}
	var unknown []string
	for _, k := range sortedKeys(args) {
		if !allowed[k] {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(accepted)
	return fmt.Errorf("%s is not something this gatherer takes; it takes %s",
		joinWords(unknown), joinWords(accepted))
}

func joinWords(items []string) string {
	switch len(items) {
	case 0:
		return "nothing"
	case 1:
		return items[0]
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}
