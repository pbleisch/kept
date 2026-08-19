package predicate

import (
	"reflect"
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/model"
)

// scope builds a hold_when scope from plain data, so a test says what was
// measured and nothing else.
func scope(data map[string]any) Scope {
	now := map[string]model.Observation{}
	for name, fields := range data {
		m, _ := fields.(map[string]any)
		now[name] = model.Observation{ID: name, Data: m}
	}
	return Observed{Now: now}.Scope()
}

func hold(t *testing.T, src string, data map[string]any) Verdict {
	t.Helper()
	e, err := Parse(src, HoldWhen)
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	v, err := e.Hold(scope(data))
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	return v
}

func mustFail(t *testing.T, src string, data map[string]any) *Error {
	t.Helper()
	e, err := Parse(src, HoldWhen)
	if err != nil {
		if pe, ok := err.(*Error); ok {
			return pe
		}
		t.Fatalf("%s: the error is a %T", src, err)
	}
	_, err = e.Hold(scope(data))
	if err == nil {
		t.Fatalf("%s: that was evaluated and should have been refused", src)
	}
	pe, ok := err.(*Error)
	if !ok {
		t.Fatalf("%s: the error is a %T, and a refusal is always an *Error", src, err)
	}
	return pe
}

// The failure `expr` was disqualified on. A gatherer that did not report a
// field must not make a promise not-kept, and the way that is guaranteed here
// is that a missing field has no false in it anywhere: it is a value of its own
// and it travels out through every operator that touches it.
//
// This is the invariant in AGENTS.md — *a failed gatherer yields unknown, never
// not-kept* — sitting where it can actually be broken. See docs/decision/0036.
func TestAMissingFieldIsUnknownAndNeverNotKept(t *testing.T) {
	measured := map[string]any{"tests": map[string]any{"failed": int64(0)}}

	for _, src := range []string{
		`obs.tests.coverage > 80`,
		`obs.tests.coverage == 0`,
		`not obs.tests.green`,
		`obs.tests.failed == 0 and obs.tests.coverage > 80`,
		`obs.tests.coverage > 80 and obs.tests.failed == 0`,
		`"TestX" in obs.tests.cases`,
		`all(obs.tests.packages, p => p.ok)`,
		`count(obs.tests.packages, p => p.ok) > 0`,
		`obs.nothing.at.all`,
		`has(obs.tests.absent, "x")`,
	} {
		got := hold(t, src, measured)
		if got.Value != model.Unknown {
			t.Errorf("%s is %s, and a field nobody measured is unknown", src, got.Value)
		}
		if got.Value == model.NotKept {
			t.Errorf("%s reported not-kept for something nobody measured", src)
		}
		if got.Because == "" {
			t.Errorf("%s is unknown and does not say what was not measured", src)
		}
	}
}

// The failure CEL was rejected on. Two opposite meanings — *nobody measured*
// and *this expression is wrong* — must be told apart by asking Go which type
// it is holding, never by matching English prose in the code path that decides
// a verdict.
//
// The assertions here deliberately use no message text at all. If this test
// ever needs a strings.Contains to tell the two apart, the distinction has been
// lost and the reason this language is hand-written has gone with it.
func TestUnmeasuredAndUnauthorableAreDifferentGoTypes(t *testing.T) {
	measured := map[string]any{"tests": map[string]any{"failed": int64(1)}}

	// The observation lacks a field. A value, and no error.
	unmeasured, err := Parse(`obs.tests.coverage > 80`, HoldWhen)
	if err != nil {
		t.Fatal(err)
	}
	v, err := unmeasured.Eval(scope(measured))
	if err != nil {
		t.Fatalf("a missing field produced an error: %v", err)
	}
	if _, ok := v.(Unknown); !ok {
		t.Fatalf("a missing field evaluated to a %T", v)
	}

	// A field read off a number. An error, and no value.
	unauthorable, err := Parse(`obs.tests.failed.deeper == 1`, HoldWhen)
	if err != nil {
		t.Fatal(err)
	}
	v, err = unauthorable.Eval(scope(measured))
	if err == nil {
		t.Fatal("a field read off a number was evaluated")
	}
	if v != nil {
		t.Fatalf("a refused expression also produced the value %v", v)
	}
	if _, ok := err.(*Error); !ok {
		t.Fatalf("the refusal is a %T", err)
	}
	if IsUnknown(err) {
		t.Fatal("the refusal is also an unmeasured value, so the two have collapsed")
	}
}

// Unknown must never satisfy error. If it ever does, every caller that writes
// `if err != nil` starts treating a gatherer that could not measure as a
// failure of the expression, which is the collapse the whole three-valued model
// exists to prevent.
func TestUnknownIsNotAnError(t *testing.T) {
	var v any = Unknown{Because: "obs.tests.coverage"}
	if _, ok := v.(error); ok {
		t.Fatal("Unknown implements error")
	}
	var e any = &Error{Msg: "wrong"}
	if _, ok := e.(Unknown); ok {
		t.Fatal("an Error is also an Unknown")
	}
}

// The three-valued table, every row. Two of these must not collapse and they
// are the reason the table is applied rather than short-circuited: `false and
// unknown` is false because one conjunct is known false, and `true and unknown`
// is unknown because nothing decides it.
func TestTheThreeValuedTableHoldsInBothOrders(t *testing.T) {
	// obs.k.yes is true, obs.k.no is false, and obs.k.gone was never measured.
	data := map[string]any{"k": map[string]any{"yes": true, "no": false}}

	cases := []struct {
		src  string
		want model.Value
	}{
		{`obs.k.yes and obs.k.yes`, model.Kept},
		{`obs.k.yes and obs.k.no`, model.NotKept},
		{`obs.k.no and obs.k.no`, model.NotKept},

		{`obs.k.no and obs.k.gone`, model.NotKept},
		{`obs.k.gone and obs.k.no`, model.NotKept},
		{`obs.k.yes and obs.k.gone`, model.Unknown},
		{`obs.k.gone and obs.k.yes`, model.Unknown},
		{`obs.k.gone and obs.k.gone`, model.Unknown},

		{`obs.k.yes or obs.k.no`, model.Kept},
		{`obs.k.no or obs.k.no`, model.NotKept},
		{`obs.k.yes or obs.k.gone`, model.Kept},
		{`obs.k.gone or obs.k.yes`, model.Kept},
		{`obs.k.no or obs.k.gone`, model.Unknown},
		{`obs.k.gone or obs.k.no`, model.Unknown},
		{`obs.k.gone or obs.k.gone`, model.Unknown},

		{`not obs.k.yes`, model.NotKept},
		{`not obs.k.no`, model.Kept},
		{`not obs.k.gone`, model.Unknown},
	}
	for _, c := range cases {
		if got := hold(t, c.src, data).Value; got != c.want {
			t.Errorf("%s is %s, want %s", c.src, got, c.want)
		}
	}
}

// The guard for the row above that is easiest to get wrong. If the connectives
// were short-circuited on the left operand, `unknown and false` would be
// unknown and `false and unknown` would be false — the same expression, two
// answers, decided by the order the author happened to write it in.
func TestConjunctionIsCommutativeAcrossUnknown(t *testing.T) {
	data := map[string]any{"k": map[string]any{"no": false}}
	left := hold(t, `obs.k.no and obs.k.gone`, data).Value
	right := hold(t, `obs.k.gone and obs.k.no`, data).Value
	if left != right {
		t.Fatalf("the same conjunction is %s one way round and %s the other", left, right)
	}
	if left != model.NotKept {
		t.Fatalf("false and unknown is %s, and one conjunct is known false", left)
	}
}

// A comparison refuses a type mismatch rather than answering false for it. A
// gatherer that returned "0" where the predicate expects 0 is a mistake
// somebody has to see, and a quiet not-kept is the same false report one layer
// further in.
func TestAComparisonRefusesAMismatchRatherThanAnsweringFalse(t *testing.T) {
	data := map[string]any{"tests": map[string]any{
		"failed": "0",
		"cases":  []any{"TestA", "TestB"},
		"ok":     true,
	}}
	for _, src := range []string{
		`obs.tests.failed == 0`,
		`obs.tests.ok == 1`,
		`obs.tests.cases == "TestA"`,
		`obs.tests.failed > 0`,
		`obs.tests.ok and obs.tests.cases`,
	} {
		mustFail(t, src, data)
	}
}

// Set membership does not refuse a mismatch, and the asymmetry with comparison
// is deliberate: a comparison is two things the author wrote and asserted were
// comparable, where a collection's elements are whatever a gatherer returned.
func TestMembershipScansAMixedListWithoutRefusingIt(t *testing.T) {
	data := map[string]any{"tests": map[string]any{
		"cases": []any{int64(3), "TestA", true},
	}}
	if got := hold(t, `"TestA" in obs.tests.cases`, data).Value; got != model.Kept {
		t.Errorf(`"TestA" in a mixed list is %s`, got)
	}
	if got := hold(t, `"TestZ" in obs.tests.cases`, data).Value; got != model.NotKept {
		t.Errorf(`"TestZ" in a mixed list is %s`, got)
	}
	if got := hold(t, `3 in obs.tests.cases`, data).Value; got != model.Kept {
		t.Errorf(`3 in a mixed list is %s`, got)
	}
	// The right side still has to be a list. `in` is set membership, and text
	// is not a set.
	mustFail(t, `"a" in obs.tests.name`, map[string]any{"tests": map[string]any{"name": "abc"}})
}

// An element nobody measured could be the one being asked about, so a scan that
// reaches one and has not already found a match cannot answer no.
func TestMembershipOverAnUnmeasuredElementIsUnknown(t *testing.T) {
	e, err := Parse(`"TestZ" in obs.tests.cases`, HoldWhen)
	if err != nil {
		t.Fatal(err)
	}
	sc := Scope{Roots: map[string]any{"obs": map[string]any{
		"tests": map[string]any{"cases": []any{"TestA", Unknown{Because: "a record with no name"}}},
	}}}
	got, err := e.Hold(sc)
	if err != nil {
		t.Fatal(err)
	}
	if got.Value != model.Unknown {
		t.Fatalf("a scan that met something unmeasured answered %s", got.Value)
	}
}

// A predicate says whether something holds. One that evaluates to a list or a
// number is not a predicate, and saying so is more use than picking a
// truthiness rule nobody wrote down.
func TestAPredicateThatIsNotTrueOrFalseIsRefused(t *testing.T) {
	data := map[string]any{"tests": map[string]any{"failed": int64(0), "cases": []any{"a"}}}
	mustFail(t, `obs.tests.failed`, data)
	mustFail(t, `obs.tests.cases`, data)
}

// The reason travels out unchanged, so a person is told the field nobody could
// answer for rather than the outermost operator that gave up.
func TestUnknownSaysWhichFieldWasNotMeasured(t *testing.T) {
	data := map[string]any{"tests": map[string]any{"failed": int64(0)}}
	got := hold(t, `obs.tests.failed == 0 and obs.tests.coverage > 80`, data)
	if got.Value != model.Unknown {
		t.Fatalf("verdict is %s", got.Value)
	}
	if !strings.Contains(got.Because, "obs.tests.coverage") {
		t.Errorf("the reason is %q and does not name the field", got.Because)
	}
	if got.Pos.Line != 1 || got.Pos.Column == 0 {
		t.Errorf("the reason has no position: %+v", got.Pos)
	}
}

// A failed gatherer produces no observation, so no predicate runs against one.
// The tick should not offer one; if it does, the answer is still unknown rather
// than whatever partial data the failure left behind. See docs/decision/0004.
func TestAFailedGathererYieldsUnknownEvenWhenItLeftData(t *testing.T) {
	e, err := Parse(`obs.tests.failed == 0`, HoldWhen)
	if err != nil {
		t.Fatal(err)
	}
	sc := Observed{Now: map[string]model.Observation{
		"tests": {ID: "a", Data: map[string]any{"failed": int64(7)}, Err: errFailed{}},
	}}.Scope()
	got, err := e.Hold(sc)
	if err != nil {
		t.Fatal(err)
	}
	if got.Value != model.Unknown {
		t.Fatalf("a gatherer that failed and left data reported %s", got.Value)
	}
}

type errFailed struct{}

func (errFailed) Error() string { return "the toolchain is missing" }

// One parsed expression, many worlds. The tree holds no evaluation state, which
// is what lets a tick evaluate one promise against a box and the origin without
// reparsing and without either answer reaching the other.
func TestOneExpressionServesManyScopes(t *testing.T) {
	e, err := Parse(`obs.tests.failed == 0`, HoldWhen)
	if err != nil {
		t.Fatal(err)
	}
	origin := scope(map[string]any{"tests": map[string]any{"failed": int64(2)}})
	box := scope(map[string]any{"tests": map[string]any{"failed": int64(0)}})

	a, err := e.Hold(origin)
	if err != nil {
		t.Fatal(err)
	}
	b, err := e.Hold(box)
	if err != nil {
		t.Fatal(err)
	}
	again, err := e.Hold(origin)
	if err != nil {
		t.Fatal(err)
	}
	if a.Value != model.NotKept || b.Value != model.Kept || again.Value != model.NotKept {
		t.Fatalf("the same expression gave %s, %s and then %s", a.Value, b.Value, again.Value)
	}
}

// Numbers compare across the two shapes a decoder may hand over. A gatherer
// returning 80 and one returning 80.0 are saying the same thing.
func TestNumbersCompareAcrossTheShapesADecoderChose(t *testing.T) {
	data := map[string]any{"tests": map[string]any{"coverage": 80.0, "failed": 0}}
	if got := hold(t, `obs.tests.coverage >= 80`, data).Value; got != model.Kept {
		t.Errorf("80.0 >= 80 is %s", got)
	}
	if got := hold(t, `obs.tests.failed == 0`, data).Value; got != model.Kept {
		t.Errorf("an int from a decoder compared as %s", got)
	}
}

// obs is adopted every time an expression names it, so adopting deeply would
// copy the whole observation once per element of a fold whose lambda mentions
// obs — and neither the source bounds nor the step budget can see that, because
// the expression is small and the step count is linear. The conversion is one
// level, and the container it was handed is the container it hands on.
//
// Asserted by identity rather than by a stopwatch: a timing test for this would
// pass on a fast machine with the bug in it.
func TestReadingAnObservationDoesNotCopyIt(t *testing.T) {
	cases := make([]any, 4096)
	for i := range cases {
		cases[i] = "TestSomething"
	}
	data := map[string]any{"tests": map[string]any{"cases": cases}}

	e, err := Parse(`obs.tests.cases`, HoldWhen)
	if err != nil {
		t.Fatal(err)
	}
	v, err := e.Eval(scope(data))
	if err != nil {
		t.Fatal(err)
	}
	got, ok := v.([]any)
	if !ok {
		t.Fatalf("obs.tests.cases evaluated to a %T", v)
	}
	if reflect.ValueOf(got).Pointer() != reflect.ValueOf(cases).Pointer() {
		t.Fatal("reading a list copied it, so a fold that names obs copies the observation per element")
	}

	// A fold whose lambda names obs is the shape that turns that copy into a
	// wedge. It is linear in the list and stays inside the step budget.
	fold, err := Parse(`all(obs.tests.cases, c => obs.tests.total > 0)`, HoldWhen)
	if err != nil {
		t.Fatal(err)
	}
	data["tests"].(map[string]any)["total"] = int64(4096)
	if v, err := fold.Hold(scope(data)); err != nil || v.Value != model.Kept {
		t.Fatalf("the fold gave %v, %v", v, err)
	}
}
