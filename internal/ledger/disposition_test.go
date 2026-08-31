package ledger

import (
	"testing"

	"github.com/pbleisch/kept/internal/model"
)

func refusal(world model.World, condition Condition, detail string) Disposition {
	return Disposition{
		Artifact:  "promise:core-packages-are-tested",
		Hash:      "sha256:abcd",
		Norm:      1,
		World:     world,
		Outcome:   OutcomeRefused,
		Condition: condition,
		Detail:    detail,
	}
}

// A disposition carries its world and is not keyed by it. kept why folds the
// whole stream: the question the verb exists for is *why has this not been
// repaired for six days*, and the answer is about the queue, not about a tree.
// Splitting it per world would fragment the one question 0019 was written to
// make answerable, in a way that looks like more precision.
func TestADispositionCarriesItsWorldAndIsNotKeyedByIt(t *testing.T) {
	s := store(t)
	l, _ := open(t, s)

	for _, d := range []Disposition{
		refusal(ships, Budget, "day limit reached: 4 of 4 dispatches"),
		refusal(box, Lock, "another run holds it"),
		refusal(other, Policy, "suppressed until the release"),
	} {
		if _, err := l.Dispose(d); err != nil {
			t.Fatal(err)
		}
	}

	records, quarantined, err := ReadDispositions(s, "promise:core-packages-are-tested")
	if err != nil {
		t.Fatal(err)
	}
	if len(quarantined) != 0 {
		t.Fatalf("quarantined %v", quarantined)
	}
	if len(records) != 3 {
		t.Fatalf("kept why sees %d of 3 refusals: the stream is being filtered by world", len(records))
	}

	// And the world is on each of them, because it cannot be added later.
	for _, d := range records {
		if d.World.Tree == "" {
			t.Error("a disposition was recorded without the tree it was decided in")
		}
	}

	where, err := DispositionPath("promise:core-packages-are-tested")
	if err != nil {
		t.Fatal(err)
	}
	if want := "disposition/promise/core-packages-are-tested.jsonl"; where != want {
		t.Errorf("path %q, want %q", where, want)
	}
}

// A refusal that names no condition is the shrug this stream exists to replace.
// Refused on write, quarantined on read.
func TestARefusalNamesWhatRefused(t *testing.T) {
	s := store(t)
	l, _ := open(t, s)

	shrug := refusal(ships, "", "")
	if _, err := l.Dispose(shrug); err == nil {
		t.Fatal("recorded a refusal that says nothing about what refused")
	}

	where, err := DispositionPath("promise:core-packages-are-tested")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Append(where, []byte(`{"v":1,"kind":"disposition","at":"2026-05-01T00:00:00Z","writer":"w","seq":1,"artifact":"promise:core-packages-are-tested","world":{"tree":"sha256:1111"},"outcome":"refused"}`)); err != nil {
		t.Fatal(err)
	}
	records, quarantined, err := ReadDispositions(s, "promise:core-packages-are-tested")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 || len(quarantined) != 1 {
		t.Fatalf("read %d records and quarantined %d, want 0 and 1", len(records), len(quarantined))
	}
}

// The policy's own outcomes need no condition: what the policy said is the
// reason.
func TestThePolicysOwnOutcomesAreRecordedWhole(t *testing.T) {
	s := store(t)
	l, _ := open(t, s)

	for _, outcome := range []Outcome{OutcomeAct, OutcomeWait, OutcomeSuppress, OutcomeEscalate} {
		d := refusal(ships, "", "")
		d.Outcome = outcome
		if outcome == OutcomeAct {
			d.Thread = "6ca1kqeyfz205a2s4qf4cb991a"
		}
		if _, err := l.Dispose(d); err != nil {
			t.Errorf("%s: %v", outcome, err)
		}
	}
	if _, err := l.Dispose(Disposition{Artifact: "promise:p", World: ships, Outcome: "shrugged"}); err == nil {
		t.Error("recorded an outcome this build does not know")
	}
}

// Dispositions are machine output at a rate three orders of magnitude above the
// spine, and mixing them would drown the record a person actually reads.
func TestADispositionIsNotInTheSpine(t *testing.T) {
	s := store(t)
	l, _ := open(t, s)

	if _, err := l.Dispose(refusal(ships, Rank, "below the cut")); err != nil {
		t.Fatal(err)
	}
	if !Empty(s) {
		t.Error("a disposition reached acts.jsonl")
	}
}

// Transitions, not ticks — and the detail is deliberately not what is compared:
// "3 of 4 dispatches" becoming "4 of 4" is the same refusal on the same
// grounds.
func TestOnlyAChangedDispositionIsATransition(t *testing.T) {
	held := refusal(ships, Budget, "day limit reached: 4 of 4 dispatches")

	if !held.Moved(Disposition{}, false) {
		t.Error("a first disposition is not a change")
	}
	if held.Moved(refusal(ships, Budget, "3 of 4 dispatches"), true) {
		t.Error("the same refusal on the same grounds is a change")
	}
	if !held.Moved(refusal(ships, Lock, "another run holds it"), true) {
		t.Error("a refusal on different grounds is not a change")
	}

	acted := refusal(ships, "", "")
	acted.Outcome = OutcomeAct
	if !acted.Moved(held, true) {
		t.Error("acting after a refusal is not a change")
	}
}

func TestADispositionWithoutAWorldIsRefused(t *testing.T) {
	s := store(t)
	l, _ := open(t, s)

	if _, err := l.Dispose(refusal(model.World{}, Budget, "")); err == nil {
		t.Fatal("recorded a refusal that cannot be told from one reached in a box")
	}
}

func TestAnArtifactIdThatCouldNameAnotherFileIsRefused(t *testing.T) {
	for _, id := range []string{"promise:../../escape", "promise:", ":slug", "nonsense"} {
		if _, err := DispositionPath(id); err == nil {
			t.Errorf("%q resolved to a stream path", id)
		}
		if _, err := TruthPath(id); err == nil {
			t.Errorf("%q resolved to a stream path", id)
		}
	}
}

func TestLastDispositionFoldsAcrossWorlds(t *testing.T) {
	s := store(t)
	l, _ := open(t, s)

	if _, err := l.Dispose(refusal(ships, Budget, "first")); err != nil {
		t.Fatal(err)
	}
	last, err := l.Dispose(refusal(box, Lock, "second"))
	if err != nil {
		t.Fatal(err)
	}

	records, _, err := ReadDispositions(s, "promise:core-packages-are-tested")
	if err != nil {
		t.Fatal(err)
	}
	held, found := LastDisposition(records)
	if !found {
		t.Fatal("no disposition")
	}
	if held.Condition != last.Condition {
		t.Errorf("the last disposition is %q, want %q", held.Condition, last.Condition)
	}
}

// TestOnlyAnActNamesItsThreadAndItAlwaysDoes is decision 0077's disposition
// half. `act` is defined as *a thread opened*, so the record that says a session
// happened says which one — read rather than lifted out of the prose detail with
// a regex.
//
// The other outcomes name none because none of them has one to name: a refusal
// that reaches the runtime is refused before a packet is stored, so its id would
// send a reader to a receipt stream that does not exist.
func TestOnlyAnActNamesItsThreadAndItAlwaysDoes(t *testing.T) {
	s := store(t)
	l, _ := open(t, s)

	act := refusal(ships, "", "")
	act.Outcome, act.Thread = OutcomeAct, "6ca1kqeyfz205a2s4qf4cb991a"
	written, err := l.Dispose(act)
	if err != nil {
		t.Fatal(err)
	}
	if written.Thread != act.Thread {
		t.Fatalf("recorded thread %q, want %q", written.Thread, act.Thread)
	}

	missing := act
	missing.Thread = ""
	if _, err := l.Dispose(missing); err == nil {
		t.Error("recorded an act that names no thread")
	}
	unnamable := act
	unnamable.Thread = "threads/../acts"
	if _, err := l.Dispose(unnamable); err == nil {
		t.Error("recorded a thread that cannot name a receipt")
	}
	for _, outcome := range []Outcome{OutcomeWait, OutcomeSuppress, OutcomeEscalate, OutcomeRefused} {
		d := refusal(ships, Runtime, "the adapter refused")
		d.Outcome, d.Thread = outcome, "6ca1kqeyfz205a2s4qf4cb991a"
		if _, err := l.Dispose(d); err == nil {
			t.Errorf("recorded a %s naming a thread", outcome)
		}
	}
}

// TestAnActWrittenBeforeDispositionsNamedTheirThreadIsStillAnAct is the reason
// the rule above is a write-time one.
//
// Requiring a thread on the way back in would quarantine every act disposition
// written before the field existed — this repository's own, from 2026-08-29,
// among them — and an act taken out of the fold by a refactor is an
// append-only record revised after the fact. A missing thread is silence: it
// says nobody recorded one, never that no session ran.
func TestAnActWrittenBeforeDispositionsNamedTheirThreadIsStillAnAct(t *testing.T) {
	s := store(t)
	where, err := DispositionPath("promise:every-citation-in-the-corpus-resolves")
	if err != nil {
		t.Fatal(err)
	}
	// The shape of the line this repository's ledger actually holds.
	old := `{"v":1,"kind":"disposition","at":"2026-08-29T06:11:39Z","writer":"2vfb9jjyz0m5twyy5gcdtaqcbt","seq":9,` +
		`"artifact":"promise:every-citation-in-the-corpus-resolves","hash":"sha256:abcd","norm":1,` +
		`"world":{"tree":"sha256:1111"},"outcome":"act",` +
		`"detail":"thread 6ca1kqeyfz205a2s4qf4cb991a: a confined claude session ran in a box under nono"}`
	if err := s.Append(where, []byte(old)); err != nil {
		t.Fatal(err)
	}
	records, quarantined, err := ReadDispositions(s, "promise:every-citation-in-the-corpus-resolves")
	if err != nil {
		t.Fatal(err)
	}
	if len(quarantined) != 0 {
		t.Fatalf("an act written before the field existed was quarantined: %#v", quarantined)
	}
	if len(records) != 1 || records[0].Outcome != OutcomeAct || records[0].Thread != "" {
		t.Fatalf("read %#v, want one act naming no thread", records)
	}
	// A malformed id is still a line that would send a reader nowhere, and that
	// one is quarantined — so the leniency above is about absence and nothing
	// else.
	broken := `{"v":1,"kind":"disposition","at":"2026-08-29T06:11:40Z","writer":"w","seq":1,` +
		`"artifact":"promise:every-citation-in-the-corpus-resolves","world":{"tree":"sha256:1111"},` +
		`"outcome":"act","thread":"threads/../acts"}`
	if err := s.Append(where, []byte(broken)); err != nil {
		t.Fatal(err)
	}
	records, quarantined, err = ReadDispositions(s, "promise:every-citation-in-the-corpus-resolves")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || len(quarantined) != 1 {
		t.Fatalf("read %d records and quarantined %d, want 1 and 1", len(records), len(quarantined))
	}
}
