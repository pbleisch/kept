package repair

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/config"
	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/progress"
)

// A repair says which phase it is in while it is in it, and the display changes
// nothing about what the repair decided.
//
// It stops where TestTheBoxIsMeasuredBeforeAnythingIsDispatched stops, at the
// runtime, because a live session is claudeadmission_test.go's evidence and
// costs a credential. What that reaches is every phase before the dispatch, and
// the phase that matters most — the box's own re-derivation, measured at 27
// seconds over kept's corpus — is one of them.
func TestARepairSaysWhichPhaseItIsIn(t *testing.T) {
	provider := liveProvider(t)
	w := newRepairWorkspace(t)
	w.write("docs/promise/spare.md", promiseFile("spare", "docs/**", "obs.c.ok"))
	w.commit()

	var out bytes.Buffer
	req := w.request("promise:counted")
	req.Provider = filepath.Base(provider)
	req.Operator = config.Repair{Runtime: "claude", Executable: provider}
	// A buffer is not a terminal, so this is the form a CI log or a captured
	// pane gets, which is also the only form a test can read.
	req.Progress = progress.For(&out, &out, false)

	res, err := Perform(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	req.Progress.Close()

	// The phases, in the order the measurement says they cost: the box, then
	// the measurement in it, then the runtime.
	said := out.String()
	for _, want := range []string{
		"kept repair started cut a box at the workspace's commit",
		"kept repair finished cut a box at the workspace's commit",
		"kept repair started measure promise:counted in the box",
		"kept gather started promise:counted · corpus",
		"kept repair finished measure promise:counted in the box",
		"kept repair started locate and check the claude runtime",
	} {
		if !strings.Contains(said, want) {
			t.Errorf("the repair never said %q:\n%s", want, said)
		}
	}

	// The instrument the box re-derives with is reported inside the phase that
	// runs it, not beside it. The innermost thing running is what a person is
	// waiting for.
	measuring := strings.Index(said, "started measure promise:counted in the box")
	gathering := strings.Index(said, "kept gather started promise:counted")
	finished := strings.Index(said, "finished measure promise:counted in the box")
	if !(measuring < gathering && gathering < finished) {
		t.Errorf("the gather is not reported inside the phase that runs it:\n%s", said)
	}

	// And none of it changed the repair. A display is presentation: it reaches
	// no ledger, no observation and no verdict, so the same repair with no
	// display reaches the same conclusion on the same grounds.
	quiet := w.request("promise:counted")
	quiet.Provider = filepath.Base(provider)
	quiet.Operator = config.Repair{Runtime: "claude", Executable: provider}

	silent, err := Perform(context.Background(), quiet)
	if err != nil {
		t.Fatal(err)
	}
	if silent.Status != res.Status || silent.Outcome != res.Outcome || silent.Condition != res.Condition {
		t.Errorf("a watched repair reached %s/%s/%s and an unwatched one %s/%s/%s",
			res.Status, res.Outcome, res.Condition, silent.Status, silent.Outcome, silent.Condition)
	}
	if res.Before.Value != model.NotKept || silent.Before.Value != res.Before.Value {
		t.Errorf("the box's measurement differs with a display attached: %s against %s",
			res.Before.Value, silent.Before.Value)
	}
}

// A repair nobody is watching carries a nil display through every phase, and
// nothing guards it on the way. The guard is in the display; this is the test
// that the callers really do rely on it.
func TestARepairWithNoDisplayRuns(t *testing.T) {
	provider := liveProvider(t)
	w := newRepairWorkspace(t)
	w.commit()

	req := w.request("promise:counted")
	req.Provider = filepath.Base(provider)
	req.Operator = config.Repair{Runtime: "claude", Executable: provider}
	if req.Progress != nil {
		t.Fatal("a request built with no display has one")
	}
	if _, err := Perform(context.Background(), req); err != nil {
		t.Fatal(err)
	}
}
