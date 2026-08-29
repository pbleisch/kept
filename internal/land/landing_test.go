package land

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/run"
	"github.com/pbleisch/kept/internal/thread"
)

// TestEveryLandingALandComposesNamesAllThree is the positive half, and it is
// what makes the refusal below a guard rather than a bar nothing clears.
//
// A `landed` act carries a target, a check regime and an arrival, and none of
// the three has a default — because a ref advance and an arrival into a working
// tree do not carry the same guarantee, and `landed` on its own would mean
// both. Every shape a land can actually produce is run here, end to end, and
// the act is read back out of the ledger rather than off the result: what a
// reader a year later has is the record.
func TestEveryLandingALandComposesNamesAllThree(t *testing.T) {
	for _, c := range []struct {
		name  string
		setUp func(t *testing.T, w *landWorkspace) Target
		want  ledger.Landing
	}{
		{
			"shared, onto a ref",
			func(t *testing.T, w *landWorkspace) Target { return w.target },
			ledger.Landing{Target: "origin/main", Regime: ledger.SharedRegime, Arrival: ledger.RefArrival},
		},
		{
			"local, onto a ref nobody has checked out",
			func(t *testing.T, w *landWorkspace) Target {
				w.git("branch", "shipping", "main")
				return Local("shipping")
			},
			ledger.Landing{Target: "shipping", Regime: ledger.LocalRegime, Arrival: ledger.RefArrival},
		},
		{
			"local, into this working tree",
			func(t *testing.T, w *landWorkspace) Target {
				// A tree holding an edit is the row that admits the weaker
				// arrival, and the only row that does.
				w.write("sub/b/note/only.md", noteFile("only")+"\nhalf a thought")
				return Local("main")
			},
			ledger.Landing{Target: "main", Regime: ledger.LocalRegime, Arrival: ledger.WorktreeArrival},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			w := newLandWorkspace(t)
			target := c.setUp(t, w)
			th := w.anchor("promise:a-is-whole", func(b string) {
				writeFile(t, filepath.Join(b, "sub/a/note/second.md"), noteFile("second"))
			})

			res := w.land(t, th, target, "")
			if res.Status != Landed {
				t.Fatalf("status = %q · %q (%s)", res.Status, res.Condition, res.Detail)
			}
			act := w.landedAct(t)
			if act.Landing != c.want {
				t.Fatalf("the act's landing is %+v, want %+v", act.Landing, c.want)
			}
			// And the record says what the land actually did, rather than a
			// default that happens to match: the plan the write followed is the
			// same three values.
			if act.Target != res.Plan.Target.String() || act.Regime != res.Plan.Regime || act.Arrival != res.Plan.Arrival {
				t.Fatalf("the act records %+v and the land followed %+v", act.Landing, res.Plan)
			}
		})
	}
}

// TestALandActWithNoTargetRegimeOrArrivalIsRefusedRatherThanWritten breaks the
// composition and watches the write refuse it.
//
// The ledger's own table already drives these three absences through
// [ledger.Ledger.Land] from hand-written values. What this adds is the seam:
// the landing a land records is read off the plan it followed, so the question
// is whether a plan that lost one of its three fields can produce an act at
// all. It is asked through [recordLand] — the one caller — against a ledger
// nothing else has written, so "nothing was appended" is a whole answer rather
// than a count.
//
// It has to be refused at the write. The ledger appends and never revises, so a
// malformed act that reaches it is permanent, and a check on the read path
// alone would only ever describe the damage.
func TestALandActWithNoTargetRegimeOrArrivalIsRefusedRatherThanWritten(t *testing.T) {
	w := newLandWorkspace(t)
	th := w.anchor("promise:a-is-whole", func(b string) {
		writeFile(t, filepath.Join(b, "sub/a/note/second.md"), noteFile("second"))
	})
	landed := w.land(t, th, w.target, "")
	if landed.Status != Landed {
		t.Fatalf("status = %q · %q (%s)", landed.Status, landed.Condition, landed.Detail)
	}

	for _, c := range []struct {
		name string
		drop func(plan *Plan)
		why  string
	}{
		{"no target", func(plan *Plan) { plan.Target = Target{} }, "names no target"},
		{"no check regime", func(plan *Plan) { plan.Regime = "" }, "names no check regime"},
		{"no arrival", func(plan *Plan) { plan.Arrival = "" }, "names no arrival"},
		{"nothing at all", func(plan *Plan) { *plan = Plan{} }, "names no target"},
	} {
		t.Run(c.name, func(t *testing.T) {
			// Everything a real land produced, with one thing taken away.
			// Recorded is cleared because it is this call's answer and not the
			// earlier land's, and a copy that carried it would report the
			// wrong one whichever way this went.
			degraded := *landed
			degraded.Recorded, degraded.Act = false, ledger.Act{}
			c.drop(&degraded.Plan)

			req, fresh := w.requestWritingTo(t, th)
			who, err := identity(req.By)
			if err != nil {
				t.Fatal(err)
			}
			err = recordLand(context.Background(), req, &degraded, who)
			if err == nil {
				t.Fatalf("a land carrying %s was recorded as an act", c.name)
			}
			if !strings.Contains(err.Error(), c.why) {
				t.Fatalf("recording a land carrying %s failed with %q, want it to say %q", c.name, err, c.why)
			}
			if degraded.Recorded {
				t.Fatalf("a land carrying %s reported itself recorded", c.name)
			}
			acts, quarantined, err := ledger.ReadActs(fresh)
			if err != nil {
				t.Fatal(err)
			}
			if len(acts) != 0 || len(quarantined) != 0 {
				t.Fatalf("the refused land left %d acts and %d unreadable lines", len(acts), len(quarantined))
			}
		})
	}

	// The guard on the guard: the same call, with the plan the land actually
	// followed, writes the act. A refusal that refused everything would pass
	// every case above.
	admitted := *landed
	admitted.Recorded, admitted.Act = false, ledger.Act{}
	req, fresh := w.requestWritingTo(t, th)
	who, err := identity(req.By)
	if err != nil {
		t.Fatal(err)
	}
	if err := recordLand(context.Background(), req, &admitted, who); err != nil {
		t.Fatalf("the landing a real land composed was refused: %v", err)
	}
	if !admitted.Recorded {
		t.Fatal("an admitted landing was written and the land did not report itself recorded")
	}
	acts, _, err := ledger.ReadActs(fresh)
	if err != nil {
		t.Fatal(err)
	}
	if len(acts) != 1 || acts[0].Kind != ledger.Landed {
		t.Fatalf("recording an admitted landing left %d acts: %+v", len(acts), acts)
	}
}

// requestWritingTo is the fixture's request with a ledger nobody else has
// written, so that what the act stream holds afterwards is this call's doing
// and nothing else's.
func (w *landWorkspace) requestWritingTo(t *testing.T, id thread.ID) (Request, ledger.Store) {
	t.Helper()
	fresh := ledger.NewFiles(t.TempDir())
	writable := ledger.Open(fresh, run.Start("test-record", nil))
	t.Cleanup(func() { _ = writable.Close(run.Clean) })
	req := w.request(id)
	req.Ledger = writable
	return req, fresh
}
