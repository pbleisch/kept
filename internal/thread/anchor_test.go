package thread

import (
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/run"
)

// TestAnAnchorRoundTripsThroughTheReceipt: the anchor is the one thing in a
// receipt that is the work rather than evidence about the session, and a land
// reads it back out.
func TestAnAnchorRoundTripsThroughTheReceipt(t *testing.T) {
	store := ledger.NewFiles(t.TempDir())
	l := ledger.Open(store, run.Start("test", nil))
	id := MintID()
	receipt, err := NewReceipt(l, id)
	if err != nil {
		t.Fatal(err)
	}
	want := Anchor{Commit: oid('a'), Parent: oid('b'), Promise: "promise:x"}
	if err := receipt.RecordAnchor(want); err != nil {
		t.Fatal(err)
	}
	if err := l.Close(run.Clean); err != nil {
		t.Fatal(err)
	}

	entries, quarantined, err := ReadReceipt(store, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(quarantined) != 0 {
		t.Fatalf("the anchor did not read back as a receipt entry: %+v", quarantined)
	}
	got, found := LatestAnchor(entries)
	if !found {
		t.Fatal("the receipt holds no anchor")
	}
	if got != want {
		t.Fatalf("read back %+v, wrote %+v", got, want)
	}
}

// TestALatestAnchorIsTheLastOne: a receipt is append-only, so a thread that
// anchored twice anchored the second one on purpose.
func TestALatestAnchorIsTheLastOne(t *testing.T) {
	first := Anchor{Commit: oid('a'), Parent: oid('c'), Promise: "promise:x"}
	second := Anchor{Commit: oid('b'), Parent: oid('c'), Promise: "promise:x"}
	entries := []Entry{
		{Envelope: ledger.Envelope{Kind: ledger.ThreadAnchor}, Anchor: &first},
		{Envelope: ledger.Envelope{Kind: ledger.ThreadLifecycle}},
		{Envelope: ledger.Envelope{Kind: ledger.ThreadAnchor}, Anchor: &second},
	}
	got, found := LatestAnchor(entries)
	if !found || got != second {
		t.Fatalf("LatestAnchor = %+v (%v), want %+v", got, found, second)
	}
	if _, found := LatestAnchor(nil); found {
		t.Fatal("an empty receipt reported an anchor")
	}
}

// TestAnIncompleteAnchorIsRefused breaks the guard. Each of these would hand a
// land something it cannot gate: no commit to merge, no parent to diff against,
// no promise to bound the change, or a commit that is its own parent — which is
// a repair that changed nothing wearing the shape of one that did.
func TestAnIncompleteAnchorIsRefused(t *testing.T) {
	store := ledger.NewFiles(t.TempDir())
	l := ledger.Open(store, run.Start("test", nil))
	receipt, err := NewReceipt(l, MintID())
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []Anchor{
		{Parent: oid('b'), Promise: "promise:x"},
		{Commit: oid('a'), Promise: "promise:x"},
		{Commit: oid('a'), Parent: oid('b')},
		{Commit: oid('a'), Parent: oid('a'), Promise: "promise:x"},
		{Commit: "not-a-commit", Parent: oid('b'), Promise: "promise:x"},
		{Commit: oid('a'), Parent: "HEAD", Promise: "promise:x"},
	} {
		if err := receipt.RecordAnchor(bad); err == nil {
			t.Fatalf("an anchor was recorded for %+v", bad)
		}
	}
	_ = l.Close(run.Clean)
}

func oid(c rune) string { return strings.Repeat(string(c), 40) }
