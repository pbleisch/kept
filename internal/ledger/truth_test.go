package ledger

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/run"
)

var (
	ships = model.World{Tree: "sha256:1111", Ref: "refs/heads/main"}
	box   = model.World{Tree: "sha256:2222", Thread: "3k9wq2m1x8b4nptv0zrh7cdfy6"}
	other = model.World{Tree: "sha256:3333"}
)

func verdict(world model.World, value model.Value) Truth {
	return Truth{
		Artifact: "promise:core-packages-are-tested",
		Hash:     "sha256:abcd",
		Norm:     1,
		World:    world,
		Value:    value,
	}
}

// A world is a field, not a path component. A path per world would mint a
// tracked file every time anyone edits anything the promise looks at — dozens
// in an afternoon, all permanent, all holding one or two lines. The artifact is
// the thing that is named once, and it is already the key.
func TestAWorldIsAFieldAndNotAPathComponent(t *testing.T) {
	s := store(t)
	l, _ := open(t, s)

	if _, err := l.Reached(verdict(ships, model.Kept)); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Reached(verdict(other, model.NotKept)); err != nil {
		t.Fatal(err)
	}

	where, err := TruthPath("promise:core-packages-are-tested")
	if err != nil {
		t.Fatal(err)
	}
	if want := "truth/promise/core-packages-are-tested.jsonl"; where != want {
		t.Fatalf("path %q, want %q", where, want)
	}
	src, err := s.Read(where)
	if err != nil {
		t.Fatalf("two worlds did not write to one file: %v", err)
	}
	if len(src) == 0 {
		t.Fatal("nothing was recorded")
	}
}

// The cost of the field, stated by 0035 and paid here: a fold that forgets to
// filter is silently wrong and looks correct in every single-run test. So the
// test interleaves two worlds deliberately.
func TestTheFoldFiltersByWorld(t *testing.T) {
	s := store(t)
	writer := run.Mint()
	at := time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC)

	// Written in an order no reader may depend on, which is what union merge
	// produces from two checkouts.
	for i, w := range []struct {
		world model.World
		value model.Value
		after time.Duration
	}{
		{ships, model.NotKept, 0},
		{other, model.Kept, time.Minute},
		{ships, model.Kept, 2 * time.Minute},
		{other, model.NotKept, 3 * time.Minute},
	} {
		rec := verdict(w.world, w.value)
		rec.V, rec.Kind, rec.At, rec.Writer, rec.Seq = Version, Verdict, at.Add(w.after), writer, uint64(i+1)
		line, err := json.Marshal(rec)
		if err != nil {
			t.Fatal(err)
		}
		where, err := TruthPath(rec.Artifact)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Append(where, line); err != nil {
			t.Fatal(err)
		}
	}

	records, quarantined, err := ReadTruth(s, "promise:core-packages-are-tested")
	if err != nil {
		t.Fatal(err)
	}
	if len(quarantined) != 0 {
		t.Fatalf("quarantined %v", quarantined)
	}
	if len(records) != 4 {
		t.Fatalf("read %d records, want 4", len(records))
	}

	for _, tc := range []struct {
		world model.World
		want  model.Value
	}{
		{ships, model.Kept},
		{other, model.NotKept},
	} {
		latest, found := Latest(records, "sha256:abcd", tc.world)
		if !found {
			t.Fatalf("no verdict for %s", tc.world.Tree)
		}
		if latest.Value != tc.want {
			t.Errorf("%s: latest = %q, want %q — the fold returned whichever tree was written last",
				tc.world.Tree, latest.Value, tc.want)
		}
		if got := InWorld(records, tc.world); len(got) != 2 {
			t.Errorf("%s: %d records in this world, want 2", tc.world.Tree, len(got))
		}
	}
}

// A revision starts a new series: the earlier verdicts concerned a different
// statement.
func TestATruthSeriesIsPerContentVersion(t *testing.T) {
	s := store(t)
	l, _ := open(t, s)

	kept := verdict(ships, model.Kept)
	if _, err := l.Reached(kept); err != nil {
		t.Fatal(err)
	}
	revised := verdict(ships, model.NotKept)
	revised.Hash = "sha256:eeee"
	if _, err := l.Reached(revised); err != nil {
		t.Fatal(err)
	}

	records, _, err := ReadTruth(s, kept.Artifact)
	if err != nil {
		t.Fatal(err)
	}
	if latest, found := Latest(records, "sha256:abcd", ships); !found || latest.Value != model.Kept {
		t.Errorf("the earlier wording's verdict is %v (found=%v), want kept", latest.Value, found)
	}
	if _, found := Latest(records, "sha256:ffff", ships); found {
		t.Error("a wording nobody reached a verdict about has one")
	}
}

// A verdict reached inside a box answers whether the repair worked on its own
// terms, which is evidence about the repair. Recording it in the artifact's
// stream would fill the answers a person reads with worlds nobody has.
func TestAVerdictReachedInABoxBelongsToTheThread(t *testing.T) {
	s := store(t)
	l, _ := open(t, s)

	_, err := l.Reached(verdict(box, model.Kept))
	var thread *ThreadVerdict
	if !errors.As(err, &thread) {
		t.Fatalf("recorded a box's verdict in the artifact's truth: %v", err)
	}
	if thread.Thread != box.Thread {
		t.Errorf("the refusal names thread %q", thread.Thread)
	}
}

func TestAVerdictWithoutAWorldOrAWordingIsRefused(t *testing.T) {
	s := store(t)
	l, _ := open(t, s)

	worldless := verdict(model.World{}, model.Kept)
	if _, err := l.Reached(worldless); err == nil {
		t.Error("recorded a verdict that names no tree")
	}
	unbound := verdict(ships, model.Kept)
	unbound.Hash = ""
	if _, err := l.Reached(unbound); err == nil {
		t.Error("recorded a verdict about no content version, so a revision would not start a new series")
	}
	nonsense := verdict(ships, model.Value("probably"))
	if _, err := l.Reached(nonsense); err == nil {
		t.Error("recorded something that is not one of the three values")
	}
}

// One torn line after a crash must not be a system that stops. The line most
// likely to be torn is the last one before the crash, and everything before it
// is intact.
func TestATornVerdictIsQuarantinedAndTheRestIsRead(t *testing.T) {
	s := store(t)
	l, _ := open(t, s)

	if _, err := l.Reached(verdict(ships, model.Kept)); err != nil {
		t.Fatal(err)
	}
	where, err := TruthPath("promise:core-packages-are-tested")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Append(where, []byte(`{"v":1,"kind":"verdict","artifact":"promise:core-p`)); err != nil {
		t.Fatal(err)
	}
	if err := s.Append(where, []byte(`{"v":1,"kind":"verdict","at":"2026-05-01T00:00:00Z","writer":"w","seq":1,"artifact":"promise:core-packages-are-tested","hash":"sha256:abcd","value":"kept"}`)); err != nil {
		t.Fatal(err)
	}

	records, quarantined, err := ReadTruth(s, "promise:core-packages-are-tested")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Errorf("read %d records, want the one that is whole", len(records))
	}
	if len(quarantined) != 2 {
		t.Fatalf("quarantined %d lines, want 2", len(quarantined))
	}
	if quarantined[1].Why == "" {
		t.Error("a quarantined line says nothing about why")
	}
}

// Transitions, not ticks: a promise kept for a month is a handful of lines.
func TestOnlyAMovedVerdictIsATransition(t *testing.T) {
	held := verdict(ships, model.Kept)
	if held.Moved(Truth{}, false) != true {
		t.Error("a first verdict is not a transition")
	}
	if held.Moved(verdict(ships, model.Kept), true) {
		t.Error("the same verdict again is a transition")
	}
	if !held.Moved(verdict(ships, model.Unknown), true) {
		t.Error("a verdict that moved is not a transition")
	}
}
