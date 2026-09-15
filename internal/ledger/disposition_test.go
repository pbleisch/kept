package ledger

import (
	"testing"
	"time"

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
	for _, outcome := range []Outcome{OutcomeWait, OutcomeSuppress, OutcomeRefused} {
		d := refusal(ships, Runtime, "the adapter refused")
		d.Outcome, d.Thread = outcome, "6ca1kqeyfz205a2s4qf4cb991a"
		if _, err := l.Dispose(d); err == nil {
			t.Errorf("recorded a %s naming a thread", outcome)
		}
	}
}

// TestAnEscalationMayNameTheEpisodeItHandsOver is the one outcome besides an act
// that names a thread, and the reason is the person it is written for: an
// escalation always follows a receipt that exists, and the person it hands the
// episode to has to know which thread to read, rework or land. See
// docs/decision/0086.
func TestAnEscalationMayNameTheEpisodeItHandsOver(t *testing.T) {
	s := store(t)
	l, _ := open(t, s)

	escalation := refusal(ships, "", "the adversarial pass ruled reject on every attempt")
	escalation.Outcome, escalation.Thread = OutcomeEscalate, "6ca1kqeyfz205a2s4qf4cb991a"
	written, err := l.Dispose(escalation)
	if err != nil {
		t.Fatalf("an escalation naming its thread was refused: %v", err)
	}
	if written.Thread != escalation.Thread {
		t.Fatalf("recorded thread %q, want %q", written.Thread, escalation.Thread)
	}
	escalation.Thread = ""
	if _, err := l.Dispose(escalation); err != nil {
		t.Fatalf("an escalation naming no thread was refused: %v", err)
	}
	escalation.Thread = "threads/../acts"
	if _, err := l.Dispose(escalation); err == nil {
		t.Error("recorded an escalation naming a thread that cannot name a receipt")
	}
}

// TestEveryActIsRecordedAndARefusalRepeatedIsNot is the transition rule with its
// one exception, broken on purpose from both sides. An episode's attempts name
// one thread, and a second attempt treated as the first repeated would leave the
// record saying one session ran when two spent a credential.
func TestEveryActIsRecordedAndARefusalRepeatedIsNot(t *testing.T) {
	s := store(t)
	l, _ := open(t, s)

	act := refusal(ships, "", "attempt")
	act.Outcome, act.Thread = OutcomeAct, "6ca1kqeyfz205a2s4qf4cb991a"
	var previous []Disposition
	for range 3 {
		written, recorded, err := l.DisposeMoved(act, previous, 0)
		if err != nil {
			t.Fatal(err)
		}
		if !recorded {
			t.Fatal("a second act on the same thread was read as the first one repeated")
		}
		previous = append(previous, written)
	}

	refused := refusal(ships, Budget, "the window is at its ceiling")
	written, recorded, err := l.DisposeMoved(refused, previous, 0)
	if err != nil || !recorded {
		t.Fatalf("the first refusal was not recorded: %v", err)
	}
	previous = append(previous, written)
	if _, recorded, err := l.DisposeMoved(refused, previous, 0); err != nil || recorded {
		t.Fatalf("the same refusal on the same grounds was recorded twice (err %v)", err)
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

// TestAnEscalationStandsUntilSomebodyAnswersIt holds the reading `kept status`
// and `kept why` use to say a person is waiting on something, broken from each
// side: every answer ends it, and a tick's refusal is not an answer.
func TestAnEscalationStandsUntilSomebodyAnswersIt(t *testing.T) {
	at := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	const thread = "6ca1kqeyfz205a2s4qf4cb991a"
	escalation := Disposition{Envelope: Envelope{At: at}, Artifact: "promise:p", Hash: "sha256:now", Outcome: OutcomeEscalate, Thread: thread, Detail: "the attempts budget is spent"}
	act := Disposition{Envelope: Envelope{At: at.Add(time.Hour)}, Artifact: "promise:p", Hash: "sha256:now", Outcome: OutcomeAct, Thread: thread}
	tickRefused := Disposition{Envelope: Envelope{At: at.Add(time.Hour)}, Artifact: "promise:p", Hash: "sha256:now", Outcome: OutcomeRefused, Condition: Provider}
	landed := Act{Envelope: Envelope{Kind: Landed, At: at.Add(time.Hour)}, Where: Where{World: model.World{Thread: thread}}}

	if got, open := OpenEscalation([]Disposition{act, escalation}, nil, "sha256:now"); !open || got.Thread != thread {
		t.Fatalf("an escalation nobody answered reads %v %#v", open, got)
	}
	if _, open := OpenEscalation([]Disposition{escalation, tickRefused}, nil, "sha256:now"); !open {
		t.Fatal("a tick's refusal ended an escalation, and a tick does not act on one")
	}
	if _, open := OpenEscalation([]Disposition{escalation, act}, nil, "sha256:now"); open {
		t.Fatal("an escalation stands after somebody opened another attempt")
	}
	if _, open := OpenEscalation([]Disposition{escalation}, []Act{landed}, "sha256:now"); open {
		t.Fatal("an escalation stands after its thread was landed")
	}
	if _, open := OpenEscalation([]Disposition{escalation}, nil, "sha256:revised"); open {
		t.Fatal("an escalation stands after the promise was revised")
	}
	earlierLand := landed
	earlierLand.At = at.Add(-time.Hour)
	if _, open := OpenEscalation([]Disposition{escalation}, []Act{earlierLand}, "sha256:now"); !open {
		t.Fatal("a land from before the escalation answered it")
	}
	if _, open := OpenEscalation(nil, nil, "sha256:now"); open {
		t.Fatal("an artifact with no dispositions reads as escalated")
	}
}
