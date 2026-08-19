package predicate

import (
	"strings"
	"testing"
)

// What the language accepts. Each of these is either written in a decision, in
// model.md, or in the corpus this repository already carries, so the list is
// the grammar's obligation rather than a sample of it.
func TestTheGrammarAcceptsWhatTheDecisionsWrite(t *testing.T) {
	for _, src := range []string{
		`obs.check.exit == 0`,
		`obs.tests.failed == 0`,
		`obs.build.ok and obs.chapters.hits == 0`,
		`obs.tests.failed == 0 and "TestRevisionDropsStanding" in obs.tests.cases`,
		`all(obs.tests.packages, p => p.tests_run > 0 and p.failed == 0)`,
		`obs.tests.coverage > 80.5`,
		`not obs.build.broken`,
		`obs.a.x == 1 or (obs.b.y == 2 and obs.c.z == 3)`,
		`all(prior(tests).cases, c => c in obs.tests.cases)`,
		`obs.tests.failed != -1`,
		`has(obs.tests, "coverage")`,
		`count(obs.tests.cases, c => c == "TestX") > 0`,
		`true`,
		`obs.tests.ok == false`,
	} {
		if _, err := Parse(src, HoldWhen); err != nil {
			t.Errorf("%s did not parse: %v", src, err)
		}
	}

	for _, src := range []string{
		`exit == 0`,
		`count(json_lines(stdout), e => has(e, "Test") and e.Action == "pass")`,
		`map(filter(json_lines(stdout), e => has(e, "Test") and e.Action == "pass"), e => e.Test)`,
		`find_all(stdout, "(?P<token>gh[ps]_[A-Za-z0-9]{36})")`,
		`count(lines(stderr), l => l == "")`,
		`json(stdout).version`,
		`find_all(stderr, '\d+ errors')`,
	} {
		if _, err := Parse(src, Extractor); err != nil {
			t.Errorf("%s did not parse as an extractor: %v", src, err)
		}
	}
}

// The guard for the test above. Everything here is refused, and the point of
// each case is the reason rather than the refusal: an author reads the message,
// so the message is what is asserted.
func TestTheGrammarRefusesWithAReasonAPersonCanAct(t *testing.T) {
	cases := []struct {
		src  string
		ns   Namespace
		want string
	}{
		{`obs.tests.failed ==`, HoldWhen, "a value was expected"},
		{`obs.tests.failed = 0`, HoldWhen, "a lone ="},
		{`obs.tests.failed != 0 !`, HoldWhen, "a lone !"},
		{`obs.tests.`, HoldWhen, "a field name was expected"},
		{`(obs.tests.failed == 0`, HoldWhen, ") was expected"},
		{`obs.tests.failed == 0)`, HoldWhen, "the expression ended and then )"},
		{`1 < 2 < 3`, HoldWhen, "two comparisons in a row"},
		{`stdout == ""`, HoldWhen, "read by an extractor and not by hold_when"},
		{`obs.tests.failed == 0`, Extractor, "read by hold_when and not by an extractor"},
		{`nothing.here`, HoldWhen, "nothing here is called nothing"},
		{`total(obs.tests.cases)`, HoldWhen, "there is no function called total"},
		{`all(obs.tests.cases)`, HoldWhen, "all takes 2 arguments and was given 1"},
		{`all(obs.tests.cases, "x")`, HoldWhen, "all wants a lambda here"},
		{`has(obs.tests, x => true)`, HoldWhen, "a lambda means nothing in this position"},
		{`all(obs.tests.cases, obs => true)`, HoldWhen, "obs is what this namespace reads"},
		{`all(obs.tests.cases, count => true)`, HoldWhen, "count is a function and cannot also be"},
		{`prior(tests, 2).cases`, HoldWhen, "reaches back exactly one observation"},
		{`prior(promise.truth)`, HoldWhen, "reading another artifact's truth is refused"},
		{`prior("tests")`, HoldWhen, "it wants a name"},
		{`prior(tests)`, Extractor, "there is no history of a program's output"},
		{`truth("promise:other")`, HoldWhen, "open question 1"},
		{`standing("promise:other")`, HoldWhen, "Standing is created by ratification"},
		{`fact("x")`, HoldWhen, "a predicate may not read a fact"},
		{`obs.tests.failed == "unclosed`, HoldWhen, "never closed"},
		{"obs.tests.name == \"a\nb\"", HoldWhen, "runs off the end of the line"},
		{`obs.tests.name == "\q"`, HoldWhen, `\q is not an escape`},
		{`obs.tests.failed == 0 # comment`, HoldWhen, `"#" is not part of this language`},
		{`find_all(stdout, "(")`, Extractor, "will not compile"},
		{`find_all(stdout, 3)`, Extractor, "a pattern written as text"},
		{`   `, HoldWhen, "there is no expression here"},
		{`- obs.tests.failed`, HoldWhen, "a minus sign is only a negative number"},
		{`count`, HoldWhen, "count is a function and wants arguments"},
	}
	for _, c := range cases {
		_, err := Parse(c.src, c.ns)
		if err == nil {
			t.Errorf("%s parsed and should not have", c.src)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: message is %q, and it should say %q", c.src, err.Error(), c.want)
		}
		if _, ok := err.(*Error); !ok {
			t.Errorf("%s: the error is a %T, and a refusal is always an *Error", c.src, err)
		}
	}
}

// An expression that will not parse is never Unknown. Nothing has been
// measured at parse time, so there is nothing that could have failed to measure
// — and a corpus that let a syntax error through as "nobody knows" would report
// a promise as unmeasurable forever rather than as broken once.
func TestAParseFailureIsNeverUnknown(t *testing.T) {
	v, err := Parse(`obs.tests.failed ==`, HoldWhen)
	if err == nil {
		t.Fatal("that parsed")
	}
	if v != nil {
		t.Fatal("a refused parse returned an expression")
	}
	if IsUnknown(err) {
		t.Fatal("a parse failure was reported as an unmeasured value")
	}
}

// The rendering an author sees. It is asserted rather than eyeballed because
// the caret is the part of an error message that is wrong most easily and
// noticed least.
func TestAnErrorRendersAgainstItsSource(t *testing.T) {
	_, err := Parse(`all(obs.tests.ok, p => p) and 1 < 2 < 3`, HoldWhen)
	if err == nil {
		t.Fatal("that parsed")
	}
	rendered := err.(*Error).Render()
	lines := strings.Split(rendered, "\n")
	if len(lines) != 3 {
		t.Fatalf("an error renders as message, source, caret; got:\n%s", rendered)
	}
	caret := strings.Index(lines[2], "^")
	if caret < 0 {
		t.Fatalf("no caret:\n%s", rendered)
	}
	// The caret sits under the second comparison, which is what is wrong.
	if got := lines[1][caret : caret+3]; got != "< 3" {
		t.Errorf("the caret points at %q:\n%s", got, rendered)
	}
	if !strings.Contains(lines[2], "line 1, column") {
		t.Errorf("the caret line does not say where:\n%s", rendered)
	}
}

// Position is counted in runes, so a caret under an expression with a
// multi-byte character in it lands where a person sees it.
func TestAPositionIsCountedTheWayAPersonReads(t *testing.T) {
	_, err := Parse(`obs.tests.name == "café" and 1 < 2 < 3`, HoldWhen)
	if err == nil {
		t.Fatal("that parsed")
	}
	pos := err.(*Error).Pos
	runes := []rune(`obs.tests.name == "café" and 1 < 2 < 3`)
	if got := string(runes[pos.Column-1 : pos.Column+2]); got != "< 3" {
		t.Errorf("column %d points at %q", pos.Column, got)
	}
}

// Single quotes are raw and double quotes interpret escapes. The asymmetry is
// for patterns: a regexp written through two layers of escaping is a regexp
// nobody can review.
func TestQuotingKeepsAPatternReadable(t *testing.T) {
	raw, err := Parse(`find_all(stdout, '\d+')`, Extractor)
	if err != nil {
		t.Fatal(err)
	}
	escaped, err := Parse(`find_all(stdout, "\\d+")`, Extractor)
	if err != nil {
		t.Fatal(err)
	}
	out := Output{Stdout: "12 and 345"}
	a, _, err := raw.Extract(out.Scope())
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := escaped.Extract(out.Scope())
	if err != nil {
		t.Fatal(err)
	}
	if render(a) != render(b) {
		t.Errorf("'\\d+' matched %s and \"\\\\d+\" matched %s", render(a), render(b))
	}
	if got := len(a.([]any)); got != 2 {
		t.Errorf("matched %d times, want 2", got)
	}
}

// A dot after a number is member access, not a decimal point. Without the rule
// the lexer would swallow the field name into the literal and the expression
// would mean something else with no error anywhere.
func TestANumberDoesNotSwallowTheDotAfterIt(t *testing.T) {
	e, err := Parse(`obs.a.b > 1.5`, HoldWhen)
	if err != nil {
		t.Fatal(err)
	}
	if e.Nodes() < 4 {
		t.Errorf("the tree has %d nodes, so something was swallowed", e.Nodes())
	}
}
