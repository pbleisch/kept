package assess

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/thread"
)

// TestAnEpisodeIsAssessedAttemptByAttemptAndAsAWhole is a thread holding two
// attempts, written the way a rework writes one: the second box is measured at
// the first attempt's anchor, where the promise already holds, and its session
// changes the tree again. See docs/decision/0086.
//
// Three things are easy to get wrong and each is asserted. The before side of
// the episode is the first box's, so the second attempt is not called a no-op
// for standing where the promise already held. The pass over the first attempt
// is not reported as the pass over the second. And each attempt reads its own
// box and its own cut.
func TestAnEpisodeIsAssessedAttemptByAttemptAndAsAWhole(t *testing.T) {
	w := newRepair(t, weakPredicate)
	w.rewrite(map[string]string{"sandbox/sandbox_test.go": twoTests})
	w.commit("attempt one")
	w.measure()
	first := w.assess(t)
	firstAnchor := first.Entry.Anchor
	w.argue(t, "The tests assert less than their names.\n\nruling: reject")

	// Attempt two's box, cut at attempt one's anchor: the promise holds there.
	boxBefore := w.measure()
	if boxBefore.Value != model.Kept {
		t.Fatalf("the rework's box reads %s, want the promise already holding", boxBefore.Value)
	}
	w.rewrite(map[string]string{"sandbox/notes.txt": "the rework answered the case\n"})
	w.commit("attempt two")
	w.measure()
	w.rework(t, firstAnchor, boxBefore.observations)

	ctx := context.Background()
	request := Request{Store: w.store, Repository: w.root, Corpus: w.corpus, Thread: w.id, Deadline: time.Minute}
	readings, err := Attempts(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(readings) != 2 {
		t.Fatalf("read %d attempts, want 2", len(readings))
	}
	one, two := readings[0], readings[1]
	if one.Number != 1 || two.Number != 2 {
		t.Fatalf("attempts are numbered %d and %d", one.Number, two.Number)
	}
	if one.Cut != w.parent || two.Cut != firstAnchor.Commit {
		t.Fatalf("the boxes were cut at %s and %s, want the base %s and then attempt one's anchor %s", one.Cut, two.Cut, w.parent, firstAnchor.Commit)
	}
	if one.Before != model.NotKept || two.Before != model.Kept {
		t.Fatalf("the boxes read %q and %q before their sessions, want not-kept and then kept", one.Before, two.Before)
	}
	if one.Assessment.Argument.Ruling != thread.RulingReject {
		t.Fatalf("attempt one's pass ruled %q (%s)", one.Assessment.Argument.Ruling, one.Assessment.Argument.RulingWhy)
	}
	if two.Assessment.Argument.Ran {
		t.Fatal("attempt one's pass is reported as the pass over attempt two, whose work nobody argued about")
	}
	if !one.Anchored || one.Anchor.Commit != firstAnchor.Commit || !two.Anchored || two.Anchor.Commit == firstAnchor.Commit {
		t.Fatalf("the attempts anchored %v and %v", one.Anchor, two.Anchor)
	}

	whole, err := Assess(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if whole.Argument.Ran {
		t.Fatal("the thread is reported as argued over, and its latest attempt was not")
	}
	if f := finding(t, whole, AlreadySatisfied); f.Outcome != Clear {
		t.Fatalf("already-satisfied reads %s over the episode (%s): the gap a person asked about is the first box's", f.Outcome, f.Detail)
	}
	if f := finding(t, two.Assessment, AlreadySatisfied); f.Outcome != Clear {
		t.Fatalf("already-satisfied reads %s for attempt two alone (%s)", f.Outcome, f.Detail)
	}
	if !strings.Contains(strings.Join(whole.Entry.Paths(), " "), "sandbox/notes.txt") ||
		!strings.Contains(strings.Join(whole.Entry.Paths(), " "), "sandbox/sandbox_test.go") {
		t.Fatalf("the episode's change is %v, want both attempts' work against the base", whole.Entry.Paths())
	}
}

// TestARulingIsReadFromTheArgumentAndAMissingOneSaysWhy holds the report to the
// one line kept reads, and to saying so when there is none.
func TestARulingIsReadFromTheArgumentAndAMissingOneSaysWhy(t *testing.T) {
	for _, c := range []struct {
		said string
		want string
	}{
		{"A case.\n\nruling: reject\n", "the ruling kept reads: reject"},
		{"I cannot make a case against this.\nruling: accept", "the ruling kept reads: accept"},
		{"A case, and no ruling at the end.", "no ruling kept can read: its last line is"},
	} {
		w := newRepair(t, weakPredicate)
		w.rewrite(map[string]string{"sandbox/sandbox_test.go": twoTests})
		w.commit("repair")
		w.measure()
		w.assess(t)
		w.argue(t, c.said)
		a, err := Assess(context.Background(), Request{Store: w.store, Repository: w.root, Corpus: w.corpus, Thread: w.id, Deadline: time.Minute})
		if err != nil {
			t.Fatal(err)
		}
		if line := a.Argument.RulingLine(); !strings.HasPrefix(line, c.want) {
			t.Fatalf("an argument ending %q reads %q, want %q", c.said, line, c.want)
		}
		if !strings.Contains(a.String(), c.want) {
			t.Fatalf("the report does not print the ruling line:\n%s", a.String())
		}
		// And the words are still the words: the ruling is read beside them.
		if a.Argument.Said != c.said {
			t.Fatalf("the argument is %q, want it verbatim", a.Argument.Said)
		}
	}
}

// rework appends attempt two to this repair's thread, the way a rework writes
// one: the box's own measurement first, then a version-3 packet carrying the
// earlier attempt, the session, the re-derivation, and an anchor committed onto
// the episode's base.
func (w *repair) rework(t *testing.T, earlier thread.Anchor, boxBefore map[string]model.Observation) {
	t.Helper()
	receipt, err := thread.NewReceipt(w.ledger, w.id)
	if err != nil {
		t.Fatal(err)
	}
	w.file(receipt, thread.Before, boxBefore)

	entry, found := w.corpus.Get(w.promise)
	if !found {
		t.Fatalf("%s is not in the corpus", w.promise)
	}
	packet, err := thread.AssemblePacket(thread.PacketInput{
		Promise: thread.RatifiedPromise{
			ID: w.promise, Hash: "sha256:" + strings.Repeat("a", 64), Norm: 1, Ratified: true,
			Statement:   entry.Artifact.Statement,
			Gather:      entry.Artifact.Gather,
			HoldWhen:    entry.Artifact.HoldWhen,
			Trigger:     entry.Artifact.Trigger,
			RepairScope: entry.Artifact.RepairScope,
		},
		Why:          entry.Artifact.Body,
		Observations: recorded(boxBefore),
		Rework: &thread.Rework{
			Base: w.parent,
			Tip:  earlier.Commit,
			Earlier: []thread.EarlierAttempt{{
				Anchor:   &earlier,
				Argument: "The tests assert less than their names.\n\nruling: reject",
				Ruling:   thread.RulingReject,
			}},
		},
	})
	if err != nil {
		t.Fatalf("assemble the rework packet: %v", err)
	}
	ref, err := packet.Store(w.ledger)
	if err != nil {
		t.Fatal(err)
	}
	if err := receipt.RecordPacket(ref); err != nil {
		t.Fatal(err)
	}
	if err := receipt.Started(); err != nil {
		t.Fatal(err)
	}
	w.file(receipt, thread.After, w.after.observations)
	code := 0
	if err := receipt.Exited(thread.Exit{Code: &code}); err != nil {
		t.Fatal(err)
	}
	commit := strings.TrimSpace(w.git("rev-parse", "HEAD"))
	if err := receipt.RecordAnchor(thread.Anchor{Commit: commit, Parent: w.parent, Promise: w.promise}); err != nil {
		t.Fatal(err)
	}
}
