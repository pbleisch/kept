package corpus

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/predicate"
)

// catalogue is a stand-in for internal/gather, which this package must not
// import and does not need to: what is under test is the seam, and the seam is
// an interface.
type catalogue map[string][]string

func (c catalogue) Check(gather map[string]model.GatherSpec) []error {
	var out []error
	for _, name := range sortedNames(gather) {
		spec := gather[name]
		if _, ok := c[spec.Gatherer]; !ok {
			out = append(out, noSuchGatherer{name: name, gatherer: spec.Gatherer})
			continue
		}
		if _, wrong := spec.Args["wrong"]; wrong {
			out = append(out, fmt.Errorf("%s: wrong is not something this gatherer takes", name))
		}
	}
	return out
}

func (c catalogue) Sources(gather map[string]model.GatherSpec) predicate.Sources {
	return declared{gather: gather, catalogue: c}
}

type declared struct {
	gather    map[string]model.GatherSpec
	catalogue catalogue
}

func (d declared) Declared(name string) bool {
	_, ok := d.gather[name]
	return ok
}

func (d declared) Fields(name string) ([]string, bool) {
	spec, ok := d.gather[name]
	if !ok {
		return nil, false
	}
	fields, ok := d.catalogue[spec.Gatherer]
	if !ok || spec.Args["wrong"] != nil {
		return nil, false
	}
	return fields, true
}

type noSuchGatherer struct{ name, gatherer string }

func (e noSuchGatherer) Error() string {
	return fmt.Sprintf("%s names the %s gatherer, and there is no such gatherer", e.name, e.gatherer)
}

func (e noSuchGatherer) NoSuchGatherer() bool { return true }

func sortedNames(gather map[string]model.GatherSpec) []string {
	out := make([]string, 0, len(gather))
	for name := range gather {
		out = append(out, name)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

var instruments = catalogue{"tests": {"ok", "packages", "failed"}}

func promise(t *testing.T, root, slug, fields string) {
	t.Helper()
	write(t, filepath.Join(root, "promise", slug+".md"),
		"---\nid: "+slug+"\ntype: promise\nfields:\n"+fields+"---\n\nWhy.\n")
}

func loadWith(t *testing.T, root string, g Gatherers) *Corpus {
	t.Helper()
	c, err := Load(root, root, Options{Gatherers: g})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func problems(c *Corpus, kind ProblemKind) []Problem {
	var out []Problem
	for _, p := range c.Problems {
		if p.Kind == kind {
			out = append(out, p)
		}
	}
	return out
}

// A promise reading a field its gatherer never declares is an authoring error
// caught when the corpus loads, not an unknown at evaluation. That is what a
// gatherer's declaration buys, and it is why the corpus takes a catalogue at
// all. See docs/decision/0036.
func TestAPromiseReadingAnUndeclaredFieldIsRefusedWhenTheCorpusLoads(t *testing.T) {
	root := t.TempDir()
	promise(t, root, "coverage-holds", "  gather:\n    tests: {}\n  hold_when: obs.tests.coverage > 80\n")

	found := problems(loadWith(t, root, instruments), UngatheredObservation)
	if len(found) != 1 {
		t.Fatalf("problems = %v, want one of kind %s", found, UngatheredObservation)
	}
	for _, want := range []string{"coverage", "tests", "packages"} {
		if !strings.Contains(found[0].Detail, want) {
			t.Errorf("the refusal does not name %s: %s", want, found[0].Detail)
		}
	}

	// The same corpus without a catalogue is not wrong, it is checked less:
	// the field half needs one and the name half does not.
	if found := problems(loadWith(t, root, nil), UngatheredObservation); len(found) != 0 {
		t.Fatalf("without a catalogue the field check still ran: %v", found)
	}
}

// The name half runs with or without a catalogue, because a promise reading
// obs.tests while declaring no gatherer called tests is reading something that
// will never arrive whatever gatherers turn out to declare.
func TestTheNameHalfOfTheCheckNeedsNoCatalogue(t *testing.T) {
	root := t.TempDir()
	promise(t, root, "nothing-gathers-this", "  gather:\n    build: {gatherer: tests}\n  hold_when: obs.tests.ok\n")

	for _, g := range []Gatherers{nil, instruments} {
		if found := problems(loadWith(t, root, g), UngatheredObservation); len(found) != 1 {
			t.Fatalf("with %T, problems = %v", g, found)
		}
	}
}

// A gatherer nothing answers to is a different mistake from an entry whose args
// are wrong, and they are reported as different kinds because a person fixes
// them differently: one is a typo in a name, the other is a typo in what was
// asked for.
func TestAnUnknownGathererAndABadEntryAreDifferentProblems(t *testing.T) {
	root := t.TempDir()
	promise(t, root, "measured-wrong", "  gather:\n    coverage: {}\n    tests: {wrong: yes}\n")

	c := loadWith(t, root, instruments)
	if found := problems(c, UnknownGatherer); len(found) != 1 {
		t.Fatalf("problems = %v, want one of kind %s", c.Problems, UnknownGatherer)
	}
	if found := problems(c, BadGatherSpec); len(found) != 1 {
		t.Fatalf("problems = %v, want one of kind %s", c.Problems, BadGatherSpec)
	}
}

// The distinction travels as a type and never as a message. Nothing anywhere
// tells two failures apart by matching English prose, which is the rule the
// predicate language is built on and holds here for the same reason.
func TestTheUnknownGathererMarkerIsAType(t *testing.T) {
	var marker interface{ NoSuchGatherer() bool }
	errs := instruments.Check(map[string]model.GatherSpec{"coverage": {Gatherer: "coverage"}})
	if len(errs) != 1 || !errors.As(errs[0], &marker) {
		t.Fatalf("Check returned %v", errs)
	}
}

// A corpus with a broken gather entry still loads. The line between a Problem
// and a load error is identity, not severity: the artifact still is what it is.
func TestABrokenGatherEntryDoesNotStopTheLoad(t *testing.T) {
	root := t.TempDir()
	promise(t, root, "measured-wrong", "  gather:\n    coverage: {}\n  hold_when: obs.coverage.ok\n")

	c := loadWith(t, root, instruments)
	if _, ok := c.Get("promise:measured-wrong"); !ok {
		t.Fatal("the artifact did not load")
	}
}
