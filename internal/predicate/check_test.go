package predicate

import (
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/model"
)

func gathers(names ...string) Gathered {
	g := Gathered{}
	for _, n := range names {
		g[n] = model.GatherSpec{Gatherer: n}
	}
	return g
}

// declared is a Sources that also knows what each gatherer returns, which is
// what a gatherer catalogue will supply. Until one exists, [Gathered] answers
// that it does not know and the field half of the check does not run.
type declared map[string][]string

func (d declared) Declared(name string) bool { _, ok := d[name]; return ok }

func (d declared) Fields(name string) ([]string, bool) {
	f, ok := d[name]
	return f, ok
}

// A promise reading an observation nothing gathers is refused when the corpus
// loads, naming what it does gather. It is checked here rather than at
// evaluation because it is answerable without looking at the world, and a
// person can fix it.
func TestAReferenceToAnUngatheredObservationIsRefusedAtLoad(t *testing.T) {
	e, err := Parse(`obs.tests.failed == 0 and obs.covrage.pct > 80`, HoldWhen)
	if err != nil {
		t.Fatal(err)
	}
	problems := e.Check(gathers("tests", "coverage"))
	if len(problems) != 1 {
		t.Fatalf("%d problems, want 1: %v", len(problems), problems)
	}
	msg := problems[0].Error()
	for _, want := range []string{"obs.covrage.pct", "does not gather", "coverage", "tests"} {
		if !strings.Contains(msg, want) {
			t.Errorf("the message does not say %q: %s", want, msg)
		}
	}
}

// One report per thing that is wrong, not one per mention. obs.tests and
// obs.tests.packages are the same undeclared gatherer read twice, and an author
// fixing it fixes one thing.
func TestAnUngatheredObservationIsReportedOnce(t *testing.T) {
	e, err := Parse(`has(obs.tests, "x") and obs.tests.failed == 0 and obs.tests.ok`, HoldWhen)
	if err != nil {
		t.Fatal(err)
	}
	if problems := e.Check(gathers("build")); len(problems) != 1 {
		t.Fatalf("%d problems, want 1: %v", len(problems), problems)
	}
}

// prior() reaches into the history of a gatherer this artifact declares, so a
// prior() naming one it does not is the same mistake and is caught the same
// way.
func TestPriorNamingAnUngatheredObservationIsRefusedAtLoad(t *testing.T) {
	e, err := Parse(`all(prior(covrage).cases, c => c in obs.tests.cases)`, HoldWhen)
	if err != nil {
		t.Fatal(err)
	}
	problems := e.Check(gathers("tests", "coverage"))
	if len(problems) != 1 {
		t.Fatalf("%d problems, want 1: %v", len(problems), problems)
	}
	if !strings.Contains(problems[0].Error(), "covrage") {
		t.Errorf("the message does not name it: %s", problems[0])
	}
}

// A promise reading a field its gatherer does not return is refused too, naming
// the field, the gatherer, and what the gatherer does declare. This is the half
// that waits for a gatherer to declare anything; the seam is here so that it
// arrives as a check rather than as a redesign.
func TestAFieldNoGathererReturnsIsRefusedOnceOneDeclares(t *testing.T) {
	e, err := Parse(`obs.tests.coverage > 80`, HoldWhen)
	if err != nil {
		t.Fatal(err)
	}
	problems := e.Check(declared{"tests": {"failed", "passed", "cases"}})
	if len(problems) != 1 {
		t.Fatalf("%d problems, want 1: %v", len(problems), problems)
	}
	msg := problems[0].Error()
	for _, want := range []string{"tests", "coverage", "failed", "passed", "cases"} {
		if !strings.Contains(msg, want) {
			t.Errorf("the message does not say %q: %s", want, msg)
		}
	}

	// And a field it does return is not a problem.
	ok, err := Parse(`obs.tests.failed == 0`, HoldWhen)
	if err != nil {
		t.Fatal(err)
	}
	if problems := ok.Check(declared{"tests": {"failed"}}); len(problems) != 0 {
		t.Fatalf("a declared field was reported: %v", problems)
	}
}

// The check that has not arrived does not fire. A gatherer that declares
// nothing is one nobody has written a declaration for, which is different from
// one that returns nothing — and reporting every field of every promise as
// undeclared would make the check useless the moment it landed.
func TestFieldsGoUncheckedUntilAGathererDeclaresThem(t *testing.T) {
	e, err := Parse(`obs.tests.whatever_this_is > 80`, HoldWhen)
	if err != nil {
		t.Fatal(err)
	}
	if problems := e.Check(gathers("tests")); len(problems) != 0 {
		t.Fatalf("a field was checked against a gatherer that declares nothing: %v", problems)
	}
}

// A load-time check does not remove the runtime rule and is not allowed to: a
// gatherer can return less than it declared, so an absent field is still
// Unknown when a verdict is being decided.
func TestTheLoadTimeCheckDoesNotReplaceTheRuntimeRule(t *testing.T) {
	e, err := Parse(`obs.tests.failed == 0`, HoldWhen)
	if err != nil {
		t.Fatal(err)
	}
	if problems := e.Check(declared{"tests": {"failed"}}); len(problems) != 0 {
		t.Fatalf("the declared field was reported at load: %v", problems)
	}
	// The gatherer declared it and then did not return it.
	got := hold(t, `obs.tests.failed == 0`, map[string]any{"tests": map[string]any{}})
	if got.Value != model.Unknown {
		t.Fatalf("a declared field that did not arrive is %s", got.Value)
	}
}

// An extractor has no observations to check against; its roots are fixed and
// were checked when it parsed.
func TestAnExtractorHasNothingToCheck(t *testing.T) {
	e, err := Parse(`count(lines(stdout), l => l != "")`, Extractor)
	if err != nil {
		t.Fatal(err)
	}
	if problems := e.Check(gathers()); len(problems) != 0 {
		t.Fatalf("an extractor was checked against a gather map: %v", problems)
	}
}

// Roots answers off the tree and never off the text, which is what lets a
// caller ask whether an extractor ever looked at a program's exit status
// without matching a substring. `exit`, `exit_code`, `"exit"` and `r.exit` are
// four different things and a substring match reports all four.
func TestRootsReadsTheTreeAndNotTheText(t *testing.T) {
	for _, c := range []struct {
		src   string
		roots []string
	}{
		{"exit == 0", []string{"exit"}},
		{`"exit" in lines(stdout)`, []string{"stdout"}},
		{`count(lines(stdout), r => r != "") == 0`, []string{"stdout"}},
		{`exit == 0 and not ("x" in lines(stderr))`, []string{"exit", "stderr"}},
		{`lines(stdout)`, []string{"stdout"}},
	} {
		expr, err := Parse(c.src, Extractor)
		if err != nil {
			t.Fatalf("%s: %v", c.src, err)
		}
		got := expr.Roots()
		if len(got) != len(c.roots) {
			t.Errorf("%s reads %q, want %q", c.src, got, c.roots)
			continue
		}
		for i, want := range c.roots {
			if got[i] != want {
				t.Errorf("%s reads %q, want %q", c.src, got, c.roots)
				break
			}
		}
		if expr.Reads("exit") != (len(c.roots) > 0 && c.roots[0] == "exit") {
			t.Errorf("%s: Reads(exit) disagrees with Roots()", c.src)
		}
	}
}
