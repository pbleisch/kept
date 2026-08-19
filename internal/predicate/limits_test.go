package predicate

import (
	"strings"
	"testing"
)

// Every bound gets a test that deliberately exceeds it, per AGENTS.md: a check
// that has never fired is not a check. Each also gets a case comfortably inside
// it, because a limit that refuses an expression a person would really write is
// a different bug with the same symptom.

// Nesting is bounded while the parser recurses rather than after, because the
// parser recurses: a check that ran afterwards would run after the stack was
// already gone. A thousand parentheses is what a fuzzer finds in a second and
// what a corrupted corpus file looks like, and it fits inside the source bound
// so that this test fires on the bound it is about.
func TestDepthIsBoundedBeforeTheStackIs(t *testing.T) {
	deep := strings.Repeat("(", 1000) + "obs.k.x" + strings.Repeat(")", 1000)
	if len(deep) > MaxSource {
		t.Fatalf("the fixture is %d bytes and would be refused by the source bound instead", len(deep))
	}
	_, err := Parse(deep, HoldWhen)
	if err == nil {
		t.Fatal("a thousand nested parentheses parsed")
	}
	if !strings.Contains(err.Error(), "nests more than") {
		t.Errorf("message is %q", err.Error())
	}
	if _, ok := err.(*Error); !ok {
		t.Errorf("the refusal is a %T", err)
	}

	// A repeated prefix operator nests the same way and is the other shape of
	// the same attack.
	if _, err := Parse(strings.Repeat("not ", MaxDepth+1)+"obs.k.x", HoldWhen); err == nil {
		t.Fatal("a stack of nots parsed")
	}

	// And what a person writes still parses.
	if _, err := Parse(`((obs.a.x == 1 or obs.b.y == 2) and not (obs.c.z == 3))`, HoldWhen); err != nil {
		t.Errorf("an ordinary parenthesised expression was refused: %v", err)
	}
}

// Depth alone does not bound a tree. `a and a and a…` is left-associative and
// stays shallow while growing without bound, so the node count is the second
// bound and it is the one that catches this.
func TestTheNodeCountIsBoundedSeparatelyFromDepth(t *testing.T) {
	wide := strings.Repeat("obs.k.x and ", 400) + "obs.k.x"
	if len(wide) > MaxSource {
		t.Fatalf("the fixture is %d bytes and would be refused by the source bound instead", len(wide))
	}
	_, err := Parse(wide, HoldWhen)
	if err == nil {
		t.Fatal("a flat expression of four hundred conjuncts parsed")
	}
	if !strings.Contains(err.Error(), "more than") {
		t.Errorf("message is %q", err.Error())
	}

	// A long but reasonable predicate still parses, and the count is
	// observable so a caller can see how close one came.
	e, err := Parse(strings.Repeat("obs.k.x and ", 20)+"obs.k.x", HoldWhen)
	if err != nil {
		t.Fatalf("twenty conjuncts were refused: %v", err)
	}
	if e.Nodes() == 0 || e.Nodes() > MaxNodes {
		t.Errorf("Nodes() = %d", e.Nodes())
	}
}

// The source bound runs before the lexer, because everything after it is
// proportional to the token count.
func TestTheSourceBoundRunsBeforeAnythingReadsIt(t *testing.T) {
	huge := "obs.k.name == \"" + strings.Repeat("x", MaxSource) + "\""
	_, err := Parse(huge, HoldWhen)
	if err == nil {
		t.Fatal("an expression larger than the source bound parsed")
	}
	if !strings.Contains(err.Error(), "doing a gatherer's job") {
		t.Errorf("message is %q", err.Error())
	}
}

// Neither text bound sees a fold. Nested folds over a large observation are
// polynomial in the data, so the source can be short and well-formed and the
// evaluation unbounded — which is a way to wedge a tick that no limit on the
// text can catch.
func TestEvaluationIsBoundedWhereTheTextBoundsCannotSee(t *testing.T) {
	list := make([]any, 100)
	for i := range list {
		list[i] = int64(i)
	}
	data := map[string]any{"tests": map[string]any{"cases": list}}

	src := `all(obs.tests.cases, a => all(obs.tests.cases, b => all(obs.tests.cases, c => c == c)))`
	e, err := Parse(src, HoldWhen)
	if err != nil {
		t.Fatalf("the expression itself is small and should parse: %v", err)
	}
	if _, err := e.Hold(scope(data)); err == nil {
		t.Fatal("a million-element fold ran to completion")
	} else if !strings.Contains(err.Error(), "steps") {
		t.Errorf("message is %q", err.Error())
	}

	// One fold over the same list is nothing like the bound.
	one, err := Parse(`all(obs.tests.cases, a => a >= 0)`, HoldWhen)
	if err != nil {
		t.Fatal(err)
	}
	if v, err := one.Hold(scope(data)); err != nil || v.Value != "kept" {
		t.Errorf("a single fold over a hundred elements gave %v, %v", v, err)
	}
}

// A bound is refused, never answered. An expression stopped by a limit must not
// come back as not-kept, or a promise would break because its own predicate was
// too expensive rather than because the world changed.
func TestExceedingABoundIsNeverAVerdict(t *testing.T) {
	list := make([]any, 100)
	for i := range list {
		list[i] = int64(i)
	}
	data := map[string]any{"tests": map[string]any{"cases": list}}
	e, err := Parse(`all(obs.tests.cases, a => all(obs.tests.cases, b => all(obs.tests.cases, c => c == c)))`, HoldWhen)
	if err != nil {
		t.Fatal(err)
	}
	v, err := e.Hold(scope(data))
	if err == nil {
		t.Fatal("the fold completed")
	}
	if v.Value != "" {
		t.Fatalf("a refused evaluation also produced the verdict %q", v.Value)
	}
	if IsUnknown(err) {
		t.Fatal("a bound was reported as something nobody measured")
	}
}
