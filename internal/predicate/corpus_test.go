package predicate_test

import (
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/predicate"
)

// This lives in an external test package on purpose: internal/corpus already
// imports internal/predicate to check a gather map against a hold_when, and a
// test that needs both a real corpus and predicate's exported API has no
// reason to risk that cycle when Clauses, Hold and Reads are all it needs.
//
// Every predicate this repository actually ships must split into clauses that
// reassemble it and answer, joined by three-valued and, exactly what the whole
// expression answers. The synthetic examples in clause_test.go are chosen to
// be easy to read; this sweep is the check against what an author actually
// wrote, including the nested lambdas and multi-line conjunctions the corpus
// carries and a hand-picked example would not think to cover.
func TestEveryPredicateInThisCorpusIsTheConjunctionOfItsClauses(t *testing.T) {
	c, err := corpus.Load("../../docs", "../../docs", corpus.Options{})
	if err != nil {
		t.Fatal(err)
	}

	// An empty obs is enough: every reference it does not answer evaluates to
	// unknown rather than erroring, so the sweep exercises Clauses, Hold and
	// Reads against the real shape of every authored expression without
	// needing a fabricated observation for each one.
	sc := predicate.Scope{Roots: map[string]any{"obs": map[string]any{}}}

	tested := 0
	for _, e := range c.All() {
		art := e.Artifact
		if strings.TrimSpace(art.HoldWhen) == "" {
			continue
		}
		tested++
		t.Run(art.ID, func(t *testing.T) {
			expr, err := predicate.Parse(art.HoldWhen, predicate.HoldWhen)
			if err != nil {
				t.Fatalf("%s: %v", art.ID, err)
			}

			clauses := expr.Clauses()
			if len(clauses) == 0 {
				t.Fatalf("%s: Clauses returned none", art.ID)
			}

			// Rejoining the clauses on " and " and collapsing whitespace on
			// both sides must reproduce the authored predicate. The break
			// this catches is a clause that ate a token at a boundary or lost
			// one — invisible from a clause's own Src in isolation, and
			// exactly what putting the pieces back together exposes.
			rejoined := clauses[0].Src
			for _, cl := range clauses[1:] {
				rejoined += " and " + cl.Src
			}
			have := strings.Join(strings.Fields(rejoined), " ")
			want := strings.Join(strings.Fields(art.HoldWhen), " ")
			if have != want {
				t.Errorf("%s: clauses do not reassemble the predicate:\n  have %s\n  want %s", art.ID, have, want)
			}

			whole, err := expr.Hold(sc)
			if err != nil {
				t.Fatalf("%s: %v", art.ID, err)
			}

			var acc model.Value
			for i, cl := range clauses {
				v, err := cl.Hold(sc)
				if err != nil {
					t.Fatalf("%s: clause %d (%s): %v", art.ID, i, cl.Src, err)
				}
				if i == 0 {
					acc = v.Value
				} else {
					acc = joinAnd(acc, v.Value)
				}
				// Must not panic on a real, arbitrarily nested expression.
				cl.Reads(sc)
			}
			if acc != whole.Value {
				t.Errorf("%s: joining the clauses' verdicts gives %s, and the whole predicate gives %s", art.ID, acc, whole.Value)
			}
		})
	}

	if tested == 0 {
		t.Fatal("no predicate in this corpus was exercised; the sweep found nothing to check")
	}
}

// joinAnd is the same three-valued table [Expr.Hold] uses, restated over
// [model.Value] because this package cannot reach the unexported value domain
// [Clause.Hold] evaluates over.
func joinAnd(a, b model.Value) model.Value {
	if a == model.NotKept || b == model.NotKept {
		return model.NotKept
	}
	if a == model.Unknown || b == model.Unknown {
		return model.Unknown
	}
	return model.Kept
}
