package assess

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/thread"
)

// whatAPassMightSay is five findings no derived check reaches, each about a
// different thing, and none of them a question anybody wrote down in advance.
//
// It is the shape of the half this file guards. Two of these are close to a
// derived check and are not one — *the promise was the wrong proxy* is not *the
// diff touched the instrument*, and *the transcript shows the agent optimizing
// for the predicate* is deliberately not a [Check] at all. The other three are
// about the episode, the repository and the wording, and no derivation reaches
// any of them.
var whatAPassMightSay = []string{
	"The statement is about a suite that exercises the parser, and this repair made the suite pass by narrowing what it exercises. The promise was the wrong proxy for the property before the repair and it still is.",
	"The transcript shows the session reading hold_when first and the statement not at all, then working backwards from the expression. That is optimizing for the predicate rather than for what the promise says.",
	"Two earlier attempts in this episode each made a different clause true and broke the other, and the cost climbed while the diff got smaller. That is thrashing rather than converging.",
	"The change is inside the bound scope, and the reviewer it was written for is not the reviewer who will read it: nothing in the diff explains why the second helper exists.",
	"Nobody has asked whether this promise should exist. It has never fired against a real regression, and a check that has never fired is not a check.",
}

// TestWhatAPassArguesIsBoundedByNothingAndFitsNoSlot is the guard on
// docs/decision/0078's open half, and it breaks the freedom on purpose so that a
// reader can see what the freedom is worth.
//
// The pass exists to reach the failure nobody thought to ask about. Every
// mechanism that would tidy its output narrows it, and the narrowing that would
// actually happen is not somebody deciding to remove the freedom — it is a
// finding shape arriving because it looked useful, after which what the pass
// says is whatever fits the fields.
//
// So this files an argument saying five things no derived check reaches, reads it
// back through the record, and asserts every word survived. Then it applies the
// mechanism that would have been added — one finding per [Check], anything that
// fits nowhere dropped — and asserts it loses four of the five. That second half
// is the break: it is what the guard is guarding against, run so that it fails
// visibly rather than being argued about in a comment.
func TestWhatAPassArguesIsBoundedByNothingAndFitsNoSlot(t *testing.T) {
	w := newRepair(t, weakPredicate)
	w.rewrite(map[string]string{"sandbox/sandbox_test.go": twoTests})
	w.commit("make the suite pass")
	w.measure()
	assessment := w.assess(t)

	said := strings.Join(whatAPassMightSay, "\n\n")
	w.argue(t, said)

	// Read back through the same reader `kept review` uses. Nothing here parsed
	// the argument, so what comes out is what went in.
	argued, err := Assess(context.Background(), Request{
		Store: w.store, Repository: w.root, Corpus: w.corpus, Thread: w.id, Deadline: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !argued.Argument.Ran {
		t.Fatalf("a thread with an argument in its receipt reports no pass: %s", argued.Argument.Detail())
	}
	if argued.Argument.Said != said {
		t.Fatalf("the argument did not survive the record verbatim:\ngot:\n%s\nwant:\n%s", argued.Argument.Said, said)
	}
	for _, finding := range whatAPassMightSay {
		if !strings.Contains(argued.Argument.Said, finding) {
			t.Errorf("the record lost a finding no check reaches:\n%s", finding)
		}
	}

	// The closed half is exactly what it was. A pass that could add to it, or
	// whose findings were reported as checks, would be the collapse of the two
	// halves into one — and the checklist would then have a ceiling somebody
	// raised by writing prose rather than by writing a derivation.
	if before, after := len(assessment.Findings), len(argued.Findings); before != after || after != len(Checklist()) {
		t.Fatalf("the checklist was %d rows before the pass and %d after, and it has %d entries", before, after, len(Checklist()))
	}
	for _, finding := range argued.Findings {
		for _, line := range append([]string{finding.Detail}, finding.Lines...) {
			for _, argued := range whatAPassMightSay {
				if line != "" && strings.Contains(line, argued) {
					t.Errorf("%s carries what the pass said, so the argument was slotted into a check: %q", finding.Check, line)
				}
			}
		}
	}

	// And the report prints it beside the checklist rather than among it: a row
	// would put the open half inside the closed one, which is the split
	// TestTheAdversarialPassIsNotACheckAndHasNoCeiling exists to hold.
	report := argued.String()
	for _, finding := range whatAPassMightSay {
		if !strings.Contains(report, finding) {
			t.Errorf("the report drops what the pass argued:\n%s", finding)
		}
	}

	// The break. This is the mechanism a future reader would add — a slot per
	// check, a finding mapped onto the nearest one, the rest dropped — applied
	// to the same argument. It has to lose things, and saying how many is the
	// whole of why the freedom is worth its untidiness.
	kept, lost := slotted(whatAPassMightSay)
	if len(lost) == 0 {
		t.Fatal("slotting the argument into the checklist lost nothing, so this test no longer demonstrates what the freedom buys")
	}
	if len(kept) >= len(whatAPassMightSay) {
		t.Fatalf("a slotted argument kept %d of %d findings", len(kept), len(whatAPassMightSay))
	}
	t.Logf("a checklist-shaped record would keep %d of %d findings and drop:\n%s",
		len(kept), len(whatAPassMightSay), strings.Join(lost, "\n"))
}

// slotted is the narrowing this half refuses, written out so that it can be run.
//
// One finding per [Check], matched on the words the check is about, and anything
// that fits no check discarded — which is exactly what a schema mapping an
// argument back to the checklist would do, and exactly what would be lost.
func slotted(findings []string) (kept map[Check]string, lost []string) {
	kept = map[Check]string{}
	words := map[Check][]string{
		InstrumentChanged: {"gatherer", "predicate", "instrument"},
		AlreadySatisfied:  {"already", "no-op", "before the repair"},
		PropertyExercised: {"exercis", "vacuous"},
		PathsInScope:      {"scope", "path"},
		SharedEvidence:    {"other promises", "shared"},
	}
	for _, finding := range findings {
		placed := false
		for _, check := range Checklist() {
			if _, taken := kept[check]; taken {
				continue
			}
			for _, word := range words[check] {
				if strings.Contains(strings.ToLower(finding), word) {
					kept[check], placed = finding, true
					break
				}
			}
			if placed {
				break
			}
		}
		if !placed {
			lost = append(lost, finding)
		}
	}
	return kept, lost
}

// TestAPassThatRanAndAPassThatFoundNothingAreDifferentAnswers is [Unread]'s rule
// one level out, and it is the reason [Argument.Ran] is a field rather than a
// derivation from whether there are any words.
//
// A thread nobody argued over and a thread argued over by a pass that found
// nothing look the same to anything reading the words alone. Under
// docs/decision/0079's table they select opposite actions: one escalates to a
// person because half the assessor did not run, and the other is a thread ready
// to land.
func TestAPassThatRanAndAPassThatFoundNothingAreDifferentAnswers(t *testing.T) {
	w := newRepair(t, weakPredicate)
	w.rewrite(map[string]string{"sandbox/sandbox_test.go": twoTests})
	w.commit("make the suite pass")
	w.measure()

	before := w.assess(t)
	if before.Argument.Ran {
		t.Fatal("a thread nothing argued over reports a pass")
	}
	if !strings.Contains(before.Argument.Detail(), "no adversarial pass has read this thread") {
		t.Fatalf("a thread nobody argued over says %q", before.Argument.Detail())
	}

	// A pass that ran and could not make a case. It is the shortest thing a
	// session may legitimately say, and it is not nothing.
	w.argue(t, "I cannot make a case against this repair.")
	after, err := Assess(context.Background(), Request{
		Store: w.store, Repository: w.root, Corpus: w.corpus, Thread: w.id, Deadline: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !after.Argument.Ran {
		t.Fatal("a pass that argued and found nothing reads as a pass that did not run")
	}
	if after.Argument.Passes != 1 {
		t.Fatalf("the thread holds %d passes", after.Argument.Passes)
	}
	if !strings.Contains(after.Argument.Detail(), "an adversarial pass read this thread") {
		t.Fatalf("a thread that was argued over says %q", after.Argument.Detail())
	}
	// And what it said is still what it said. A build that reported *found
	// nothing* by dropping the words would have thrown away the one sentence a
	// person reading a clean thread actually wants.
	if !strings.Contains(after.Argument.Said, "cannot make a case") {
		t.Fatalf("the pass's own words are %q", after.Argument.Said)
	}
}

// argue appends one adversarial pass to this repair's thread, the way
// internal/repair.Argue does: the brief it was given, then the words it wrote.
//
// The session itself is not run here. A confined pass spends a live credential,
// and what this file is about is what the record holds and what a reader makes
// of it; internal/repair is where a real one is dispatched and proved. Every byte
// of this goes through the same [thread.Receipt] a dispatch uses, so a change to
// what a receipt may hold breaks this too.
func (w *repair) argue(t *testing.T, said string) {
	t.Helper()
	receipt, err := thread.NewReceipt(w.ledger, w.id)
	if err != nil {
		t.Fatal(err)
	}
	entries, _, err := thread.ReadReceipt(w.store, w.id)
	if err != nil {
		t.Fatal(err)
	}
	anchor, found := thread.LatestAnchor(entries)
	if !found {
		t.Fatal("the thread anchored nothing, so there is nothing for a pass to argue about")
	}
	var packet thread.Contents
	for _, entry := range entries {
		if entry.Packet == nil {
			continue
		}
		src, err := ledger.ReadArtifact(w.store, *entry.Packet)
		if err != nil {
			t.Fatal(err)
		}
		if packet, err = thread.ReadPacket(src); err != nil {
			t.Fatal(err)
		}
	}
	brief, err := thread.AssembleBrief(thread.BriefInput{
		Thread: string(w.id), Packet: packet, Anchor: anchor, Transcript: "done",
	})
	if err != nil {
		t.Fatal(err)
	}
	ref, err := brief.Store(w.ledger)
	if err != nil {
		t.Fatal(err)
	}
	if err := receipt.RecordBrief(ref); err != nil {
		t.Fatal(err)
	}
	stored, err := w.ledger.PutArtifact(ledger.ThreadArgumentArtifact, []byte(said))
	if err != nil {
		t.Fatal(err)
	}
	if err := receipt.RecordArgument(thread.ArgumentRecord{
		Runtime: thread.Claude, AdapterVersion: "test@1", Brief: ref, Artifact: stored,
	}); err != nil {
		t.Fatal(err)
	}
}
