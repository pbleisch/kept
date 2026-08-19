package predicate

import (
	"testing"

	"github.com/pbleisch/kept/internal/model"
)

// One grammar and one evaluator over two namespaces, which is what makes 0023's
// requirement of `command` met rather than approximated: the extractor names
// what to pull out of a program's output, and it is written in the language the
// promise is already written in.
func TestTheExtractorIsTheSameLanguageOverADifferentNamespace(t *testing.T) {
	out := Output{Stdout: goTestJSON, Stderr: "warning: one thing\n", Exit: 1}

	fields := map[string]string{
		"passed": `count(json_lines(stdout), e => has(e, "Test") and e.Action == "pass")`,
		"failed": `count(json_lines(stdout), e => has(e, "Test") and e.Action == "fail")`,
		"cases":  `map(filter(json_lines(stdout), e => has(e, "Test") and e.Action == "pass"), e => e.Test)`,
		"warned": `count(lines(stderr), l => l != "")`,
		"exit":   `exit`,
	}

	// What the gatherer would record: the declared fields, and nothing else.
	data := map[string]any{}
	for name, src := range fields {
		e, err := Parse(src, Extractor)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		v, measured, err := e.Extract(out.Scope())
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if measured {
			data[name] = v
		}
	}

	// And then the promise's own predicate reads exactly those fields, which is
	// the whole of what one language buys.
	predicate, err := Parse(`obs.tests.failed == 0 and "TestRevisionDropsStanding" in obs.tests.cases`, HoldWhen)
	if err != nil {
		t.Fatal(err)
	}
	v, err := predicate.Hold(scope(map[string]any{"tests": data}))
	if err != nil {
		t.Fatal(err)
	}
	if v.Value != model.NotKept {
		t.Fatalf("verdict is %s; one test failed in that stream", v.Value)
	}
}

// The one place the two jobs differ, and it is a difference in what the caller
// does with the value rather than in the language. An extractor whose
// expression is unmeasured yields no field, so the observation carries what was
// measured and nothing else — and a predicate reading the absent field gets
// unknown for the same reason it would have.
func TestAnUnmeasuredExtractorFieldIsAbsentRatherThanUnknown(t *testing.T) {
	e, err := Parse(`json(stdout).version`, Extractor)
	if err != nil {
		t.Fatal(err)
	}
	v, measured, err := e.Extract(Output{Stdout: "not json at all"}.Scope())
	if err != nil {
		t.Fatalf("output that is not JSON was refused rather than left unmeasured: %v", err)
	}
	if measured {
		t.Fatalf("the field was measured as %v", v)
	}
	if v != nil {
		t.Fatalf("an unmeasured field carried the value %v", v)
	}

	// The observation therefore has no version field, and the promise reading
	// it is unknown rather than not-kept.
	got := hold(t, `obs.tool.version == "1.0"`, map[string]any{"tool": map[string]any{}})
	if got.Value != model.Unknown {
		t.Fatalf("a promise reading the absent field is %s", got.Value)
	}
}

// The escape hatch 0004 promised, end to end: a command gatherer with no
// extractor at all, and a predicate over its exit code. It needs no builtin and
// it is the first self-hosting promise.
func TestTheEscapeHatchNeedsNoExtractorAtAll(t *testing.T) {
	for _, c := range []struct {
		exit int
		want model.Value
	}{{0, model.Kept}, {1, model.NotKept}} {
		e, err := Parse(`exit`, Extractor)
		if err != nil {
			t.Fatal(err)
		}
		v, measured, err := e.Extract(Output{Exit: c.exit}.Scope())
		if err != nil || !measured {
			t.Fatalf("exit %d: %v measured=%v", c.exit, err, measured)
		}
		got := hold(t, `obs.check.exit == 0`, map[string]any{"check": map[string]any{"exit": v}})
		if got.Value != c.want {
			t.Errorf("exit %d gives %s, want %s", c.exit, got.Value, c.want)
		}
	}
}

// An extractor reads what a program produced and nothing else. obs belongs to
// the predicate and prior() to the history, and reaching for either from here
// is refused when the expression parses.
func TestAnExtractorCannotReachIntoThePredicateNamespace(t *testing.T) {
	for _, src := range []string{`obs.tests.failed`, `prior(tests)`, `truth("promise:x")`} {
		if _, err := Parse(src, Extractor); err == nil {
			t.Errorf("%s parsed as an extractor", src)
		}
	}
	// And the reverse, so neither namespace leaks into the other.
	for _, src := range []string{`stdout`, `stderr == ""`, `exit == 0`} {
		if _, err := Parse(src, HoldWhen); err == nil {
			t.Errorf("%s parsed as a predicate", src)
		}
	}
}
