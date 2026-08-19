package gather

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/predicate"
	"github.com/pbleisch/kept/internal/run"
)

// verdict evaluates a hold_when against one observation, the way a tick would.
// It is here rather than in a helper file because what it asserts — that a
// failed gather yields unknown — is the thing most of these tests are about.
func verdict(t *testing.T, name string, obs model.Observation, src string) model.Value {
	t.Helper()
	expr, err := predicate.Parse(src, predicate.HoldWhen)
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	observed := predicate.Observed{Now: map[string]model.Observation{name: obs}}
	v, err := expr.Hold(observed.Scope())
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	return v.Value
}

func newLedger(t *testing.T) (*ledger.Ledger, ledger.Store) {
	t.Helper()
	store := ledger.NewFiles(t.TempDir())
	return ledger.Open(store, run.Start("test", nil)), store
}

// A failed gather leaves no observation. Not a false field, not an empty
// observation — nothing recorded. Proving it takes both halves: the runner
// hands back a failure, and Record files nothing for one. See
// docs/decision/0004 and 0034.
func TestAFailedGatherLeavesNoObservation(t *testing.T) {
	obs := NewRunner(Builtin()).Gather(t.Context(), model.GatherSpec{Gatherer: Command}, request(t, map[string]any{
		argRun:    "kept-no-such-program",
		argFields: map[string]any{"ok": "exit == 0"},
	}))
	if obs.Err == nil {
		t.Fatal("the gather succeeded, so this proves nothing")
	}

	l, store := newLedger(t)
	address, recorded, err := Record(l, obs)
	if err != nil {
		t.Fatalf("Record returned an error for a failure it should have ignored: %v", err)
	}
	if recorded || address != "" {
		t.Fatalf("a failed gather was filed as %q", address)
	}

	// And nothing is there to be read. An error field on the record would make
	// a failure a kind of observation, which is the collapse the three-valued
	// verdict exists to prevent.
	probe := ledger.Measured(obs)
	probe.Data = map[string]any{}
	where, err := ledger.ObservationPath(probe.Address())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Read(where); !errors.Is(err, ledger.ErrNoStream) {
		t.Fatalf("something was written for a gather that failed: %v", err)
	}
}

// The break: a Record that filed failures too would put a record in the ledger
// for a gather that did not happen, and a predicate reading it would report
// not-kept for a machine that could not measure.
func TestRecordFilesAMeasurementThatSucceeded(t *testing.T) {
	obs := NewRunner(Builtin()).Gather(t.Context(), model.GatherSpec{Gatherer: Command}, request(t, map[string]any{
		argRun:    runLine(t, "exit", "0"),
		argFields: map[string]any{"ok": "exit == 0"},
	}))
	if obs.Err != nil {
		t.Fatalf("the gather failed: %v", obs.Err)
	}

	l, store := newLedger(t)
	address, recorded, err := Record(l, obs)
	if err != nil || !recorded {
		t.Fatalf("Record filed nothing for a measurement: %v", err)
	}
	if address != obs.ID {
		t.Errorf("the ledger filed it under %s and the observation says %s", address, obs.ID)
	}
	back, err := ledger.ReadObservation(store, address)
	if err != nil {
		t.Fatalf("the record does not read back: %v", err)
	}
	if back.Data["ok"] != true {
		t.Errorf("the record holds %#v", back.Data)
	}
}

// An observation names no run. Two runners are two processes as far as the
// record is concerned, and the same tree measured the same way is one
// observation whoever ran it — which is what content addressing already
// asserts. See docs/decision/0013 and 0034.
func TestTwoRunnersMeasuringOneTreeAgreeOnTheAddress(t *testing.T) {
	req := request(t, map[string]any{
		argRun:    runLine(t, "print", "steady"),
		argFields: map[string]any{"out": "stdout"},
	})

	first := NewRunner(Builtin()).Gather(t.Context(), model.GatherSpec{Gatherer: Command}, req)
	second := NewRunner(Builtin()).Gather(t.Context(), model.GatherSpec{Gatherer: Command}, req)
	if first.Err != nil || second.Err != nil {
		t.Fatalf("%v / %v", first.Err, second.Err)
	}
	if first.ID != second.ID {
		t.Fatalf("two runs of one measurement addressed it as %s and %s", first.ID, second.ID)
	}

	// And the record carries nothing naming either of them.
	line := string(mustMarshal(t, ledger.Measured(first)))
	for _, forbidden := range []string{"writer", "seq", "pid", "host"} {
		if strings.Contains(line, `"`+forbidden+`"`) {
			t.Errorf("the record carries %s: %s", forbidden, line)
		}
	}
}

// Two promises naming one gatherer over one world share the work. A gather runs
// a program, so gathering twice costs the same again to produce a record
// content addressing would immediately discard as a duplicate.
func TestTwoPromisesSharingAGathererShareTheWork(t *testing.T) {
	marks := filepath.Join(t.TempDir(), "marks")
	spec := model.GatherSpec{Gatherer: Command}
	req := request(t, map[string]any{
		argRun:    runLine(t, "spawn-mark", marks),
		argFields: map[string]any{"out": "stdout"},
	})

	r := NewRunner(Builtin())
	first := r.Gather(t.Context(), spec, req)
	second := r.Gather(t.Context(), spec, req)
	if first.Err != nil {
		t.Fatalf("the gather failed: %v", first.Err)
	}
	if first.ID != second.ID {
		t.Fatalf("the shared gather produced two observations: %s and %s", first.ID, second.ID)
	}
	if n := countMarks(t, marks); n != 1 {
		t.Fatalf("the program ran %d times for two promises reading one gatherer", n)
	}

	// A different world is a different measurement, and sharing it would let a
	// gather in a box answer for the tree that ships. See docs/decision/0014.
	elsewhere := req
	elsewhere.World = world("bbbb")
	if third := r.Gather(t.Context(), spec, elsewhere); third.ID == first.ID {
		t.Fatal("two worlds shared one observation")
	}
	if n := countMarks(t, marks); n != 2 {
		t.Fatalf("the program ran %d times across two worlds", n)
	}
}

// The sharing has to hold for callers that overlap, which is every real tick
// and no single-threaded test.
func TestConcurrentCallersShareOneGather(t *testing.T) {
	marks := filepath.Join(t.TempDir(), "marks")
	spec := model.GatherSpec{Gatherer: Command}
	req := request(t, map[string]any{
		argRun:    runLine(t, "spawn-mark", marks),
		argFields: map[string]any{"out": "stdout"},
	})

	r := NewRunner(Builtin())
	var wg sync.WaitGroup
	ids := make([]string, 8)
	for i := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ids[i] = r.Gather(context.Background(), spec, req).ID
		}()
	}
	wg.Wait()

	for _, id := range ids {
		if id == "" || id != ids[0] {
			t.Fatalf("concurrent callers got %q", ids)
		}
	}
	if n := countMarks(t, marks); n != 1 {
		t.Fatalf("the program ran %d times for eight concurrent callers", n)
	}
}

// The second of the two bounds, and it fires here rather than at write time:
// over it the gather **failed**, so the verdict is unknown. Discovering it when
// the record was written would mean the predicate had already run on data that
// was never kept, and a promise reported kept against a record that does not
// exist. See docs/decision/0034.
func TestAnExtractorPastTheRecordBoundIsRefused(t *testing.T) {
	big := strings.Repeat("y", ledger.MaxObservation+4096)
	dir := t.TempDir()
	payload := filepath.Join(dir, "payload")
	if err := os.WriteFile(payload, []byte(big), 0o600); err != nil {
		t.Fatal(err)
	}

	obs := NewRunner(Builtin()).Gather(t.Context(), model.GatherSpec{Gatherer: Command}, request(t, map[string]any{
		argRun:    runLine(t, "emit", payload),
		argFields: map[string]any{"everything": "stdout"},
	}))

	var large *ledger.TooLarge
	if !errors.As(obs.Err, &large) {
		t.Fatalf("a well-formed extractor wrote %d bytes into the record, refused as %#v", len(big), obs.Err)
	}
	if !strings.Contains(large.Bound, "observation") {
		t.Errorf("the refusal names %q, and a receipt has to say which of the two bounds it was", large.Bound)
	}
	if obs.Data != nil {
		t.Error("the refused measurement kept its data")
	}
	if got := verdict(t, "check", obs, "obs.check.everything != \"\""); got != model.Unknown {
		t.Fatalf("the verdict is %s, and a cap that reported not-kept would be the confusion the three values exist to prevent", got)
	}
}

// A gather entry naming a gatherer nothing answers to is a failure and never a
// guess at what was meant.
func TestAGatherEntryNamingNoGathererMeasuresNothing(t *testing.T) {
	r := NewRunner(Builtin())
	for _, spec := range []model.GatherSpec{{Gatherer: "coverage"}, {}} {
		obs := r.Gather(t.Context(), spec, request(t, nil))
		if obs.Err == nil {
			t.Fatalf("%#v produced an observation", spec)
		}
		if obs.Data != nil {
			t.Errorf("%#v left data behind", spec)
		}
	}
}

func countMarks(t *testing.T, path string) int {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		t.Fatal(err)
	}
	return strings.Count(string(src), "\n")
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	line, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return line
}
