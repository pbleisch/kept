package predicate

import (
	"testing"

	"github.com/pbleisch/kept/internal/model"
)

// A clause is a top-level and-conjunct written exactly as authored: the same
// bytes, trimmed only of the whitespace that separated it from its neighbors.
// Parens, or, and not each close a boundary rather than being split through —
// the naive break here is splitting on every "and" token in the text, which
// would cut the one inside a string literal or inside a paren group the
// author deliberately kept together.
func TestAClauseIsATopLevelConjunctWrittenExactlyAsAuthored(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want []string
	}{
		{
			name: "a chain of three ands is three clauses",
			src:  `obs.a.x == 1 and obs.b.y == 2 and obs.c.z == 3`,
			want: []string{`obs.a.x == 1`, `obs.b.y == 2`, `obs.c.z == 3`},
		},
		{
			name: "a parenthesized conjunct is one clause, not two",
			src:  `(obs.a.x == 1 and obs.b.y == 2) and obs.c.z == 3`,
			want: []string{`(obs.a.x == 1 and obs.b.y == 2)`, `obs.c.z == 3`},
		},
		{
			name: "a top-level or is one clause",
			src:  `obs.a.x == 1 or obs.b.y == 2`,
			want: []string{`obs.a.x == 1 or obs.b.y == 2`},
		},
		{
			name: "not over an and is one clause",
			src:  `not (obs.a.x == 1 and obs.b.y == 2)`,
			want: []string{`not (obs.a.x == 1 and obs.b.y == 2)`},
		},
		{
			name: "the whole predicate wrapped in parens is one clause",
			src:  `(obs.a.x == 1 and obs.b.y == 2)`,
			want: []string{`(obs.a.x == 1 and obs.b.y == 2)`},
		},
		{
			name: "an and inside a string is not a boundary",
			src:  `obs.a.name == "x and y" and obs.b.ok`,
			want: []string{`obs.a.name == "x and y"`, `obs.b.ok`},
		},
		{
			name: "an and inside a lambda body is not a boundary, and a clause may span lines",

			src: "all(obs.tests.packages, p => p.ok and p.failed == 0)\n" +
				"and obs.tests.coverage > 80",
			want: []string{
				`all(obs.tests.packages, p => p.ok and p.failed == 0)`,
				`obs.tests.coverage > 80`,
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e, err := Parse(c.src, HoldWhen)
			if err != nil {
				t.Fatalf("%s did not parse: %v", c.src, err)
			}
			clauses := e.Clauses()
			if len(clauses) != len(c.want) {
				t.Fatalf("got %d clauses, want %d: %#v", len(clauses), len(c.want), clauses)
			}
			for i, want := range c.want {
				if got := clauses[i].Src; got != want {
					t.Errorf("clause %d: got %q, want %q", i, got, want)
				}
			}
		})
	}
}

// Each clause answers alone, and joining those answers with three-valued and
// reproduces the verdict the whole predicate would have reached. The break
// this catches is a clause that evaluates against a scope it does not fully
// isolate — one that leaks the wrong verdict for a conjunct the author wrote
// as independent, or a whole-predicate Hold that disagrees with what its own
// clauses say about it.
func TestEachClauseAnswersAloneAndTheirConjunctionIsTheVerdict(t *testing.T) {
	const src = `obs.a.ok and obs.b.missing and obs.c.ok`
	data := map[string]any{
		"a": map[string]any{"ok": true},
		"b": map[string]any{},
		"c": map[string]any{"ok": true},
	}

	e, err := Parse(src, HoldWhen)
	if err != nil {
		t.Fatal(err)
	}
	sc := scope(data)

	whole, err := e.Hold(sc)
	if err != nil {
		t.Fatal(err)
	}

	clauses := e.Clauses()
	if len(clauses) != 3 {
		t.Fatalf("got %d clauses, want 3", len(clauses))
	}
	wantValues := []model.Value{model.Kept, model.Unknown, model.Kept}
	var joined any = true
	for i, c := range clauses {
		v, err := c.Hold(sc)
		if err != nil {
			t.Fatalf("clause %d: %v", i, err)
		}
		if v.Value != wantValues[i] {
			t.Errorf("clause %d: got %v, want %v", i, v.Value, wantValues[i])
		}
		joined = andValues(joined, verdictValue(v))
	}
	joinedVerdict, err := toVerdict(src, Position{}, joined)
	if err != nil {
		t.Fatal(err)
	}
	if joinedVerdict.Value != whole.Value {
		t.Errorf("joining the clauses gives %v, and the whole predicate gives %v", joinedVerdict.Value, whole.Value)
	}
}

// verdictValue is the inverse of toVerdict's decision, so a test can feed a
// Verdict back through the same three-valued and the evaluator uses and check
// that a clause split reproduces the whole.
func verdictValue(v Verdict) any {
	switch v.Value {
	case model.Kept:
		return true
	case model.NotKept:
		return false
	default:
		return Unknown{Because: v.Because, Pos: v.Pos}
	}
}

// A clause says what it read, including a measurement that came back empty,
// and it excludes a lambda's own parameter and anything derived rather than
// read. The break this catches is a Reads that treats "empty" the same as
// "not read" and drops it, or one that reports the lambda variable as if it
// were a second observation.
func TestAClauseSaysWhatItReadIncludingAnEmptyMeasurement(t *testing.T) {
	const src = `all(obs.a.items, x => x.ok) or obs.b.missing`
	data := map[string]any{
		"a": map[string]any{"items": []any{}},
		"b": map[string]any{},
	}

	e, err := Parse(src, HoldWhen)
	if err != nil {
		t.Fatal(err)
	}
	clauses := e.Clauses()
	if len(clauses) != 1 {
		t.Fatalf("got %d clauses, want 1: %#v", len(clauses), clauses)
	}

	reads := clauses[0].Reads(scope(data))
	byPath := map[string]any{}
	for _, r := range reads {
		byPath[r.Path] = r.Value
	}

	items, ok := byPath["obs.a.items"]
	if !ok {
		t.Fatal("obs.a.items was measured empty and should still be reported")
	}
	list, ok := items.([]any)
	if !ok || len(list) != 0 {
		t.Errorf("obs.a.items: got %#v, want an empty list", items)
	}

	missing, ok := byPath["obs.b.missing"]
	if !ok {
		t.Fatal("obs.b.missing was read and nobody measured it, and that must still be reported")
	}
	if !IsUnknown(missing) {
		t.Errorf("obs.b.missing: got %#v, want unknown", missing)
	}

	if _, ok := byPath["x.ok"]; ok {
		t.Error("x is the lambda's own parameter, not a second observation, and must not be reported")
	}
	if len(reads) != 2 {
		t.Errorf("got %d readings, want exactly 2: %#v", len(reads), reads)
	}
}
