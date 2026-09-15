package thread

import (
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/ledger"
)

func measured(side Side, name string) Entry {
	return Entry{Envelope: ledger.Envelope{Kind: ledger.ThreadMeasurement}, Measurement: &Measurement{Side: side, Name: name}}
}

func packetRecord(address string) Entry {
	return Entry{Envelope: ledger.Envelope{Kind: ledger.ThreadPacket}, Packet: &ledger.ArtifactRef{Kind: ledger.ThreadPacketArtifact, Address: address}}
}

func briefRecord(address string) Entry {
	return Entry{Envelope: ledger.Envelope{Kind: ledger.ThreadBrief}, Brief: &ledger.ArtifactRef{Kind: ledger.ThreadBriefArtifact, Address: address}}
}

func anchorRecord(commit string) Entry {
	return Entry{Envelope: ledger.Envelope{Kind: ledger.ThreadAnchor}, Anchor: &Anchor{Commit: strings.Repeat(commit, 40), Parent: strings.Repeat("0", 40), Promise: "promise:p"}}
}

func nativeRecord() Entry {
	return Entry{Envelope: ledger.Envelope{Kind: ledger.ThreadNative}, Native: &NativeEvent{Type: "assistant"}}
}

// TestAnEpisodeIsGroupedIntoAttemptsByItsOwnRecords is the derivation
// docs/decision/0086 puts in place of an ordinal on every record, and the case it
// exists for: a rework's before measurement is filed after the pass over the
// attempt before it, and grouping by input records alone would give that
// measurement to the pass.
func TestAnEpisodeIsGroupedIntoAttemptsByItsOwnRecords(t *testing.T) {
	entries := []Entry{
		measured(Before, "c"), // 0
		packetRecord("p1"),    // 1
		nativeRecord(),        // 2
		measured(After, "c"),  // 3
		anchorRecord("a"),     // 4
		briefRecord("b1"),     // 5
		nativeRecord(),        // 6
		measured(Before, "c"), // 7  — attempt 2's box, filed after pass 1
		packetRecord("p2"),    // 8
		measured(After, "c"),  // 9
		anchorRecord("b"),     // 10
		briefRecord("b2"),     // 11
	}
	attempts := Attempts(entries)
	if len(attempts) != 2 {
		t.Fatalf("grouped %d attempts, want 2", len(attempts))
	}
	first, second := attempts[0], attempts[1]
	if first.Start != 0 || first.End != 7 || second.Start != 7 || second.End != len(entries) {
		t.Fatalf("attempt bounds are [%d,%d) and [%d,%d), want [0,7) and [7,%d)", first.Start, first.End, second.Start, second.End, len(entries))
	}
	if len(first.Before) != 1 || len(second.Before) != 1 {
		t.Fatalf("before measurements grouped as %d and %d, want one each", len(first.Before), len(second.Before))
	}
	if first.Repair.Input.Address != "p1" || second.Repair.Input.Address != "p2" {
		t.Fatalf("repair sessions are %q and %q", first.Repair.Input.Address, second.Repair.Input.Address)
	}
	if len(first.Passes) != 1 || first.Passes[0].Input.Address != "b1" || len(second.Passes) != 1 {
		t.Fatalf("passes grouped as %d and %d", len(first.Passes), len(second.Passes))
	}
	// The measurement attempt 2's box took is not an entry of pass 1.
	for _, entry := range first.Passes[0].Entries {
		if entry.Measurement != nil {
			t.Fatal("the rework's before measurement was grouped into the pass over the attempt before it")
		}
	}
	anchor, anchored := first.Anchor()
	if !anchored || anchor.Commit != strings.Repeat("a", 40) {
		t.Fatalf("attempt 1's anchor is %v %v", anchor, anchored)
	}
	if anchor, _ := second.Anchor(); anchor.Commit != strings.Repeat("b", 40) {
		t.Fatalf("attempt 2's anchor is %v", anchor)
	}
	if len(first.After()) != 1 || len(second.After()) != 1 {
		t.Fatalf("after measurements are %d and %d", len(first.After()), len(second.After()))
	}
}

// TestABoxMeasuredAndNeverDispatchedIntoIsNoAttempt is a rework that stopped
// before its session: its box was measured and the runtime refused. What it
// filed belongs to no attempt, and in particular not to the one before it.
func TestABoxMeasuredAndNeverDispatchedIntoIsNoAttempt(t *testing.T) {
	entries := []Entry{
		measured(Before, "c"),
		packetRecord("p1"),
		measured(After, "c"),
		anchorRecord("a"),
		measured(Before, "c"),
	}
	attempts := Attempts(entries)
	if len(attempts) != 1 {
		t.Fatalf("grouped %d attempts, want 1", len(attempts))
	}
	if attempts[0].End != 4 {
		t.Fatalf("the attempt ends at %d, want 4, before the undispatched box's measurement", attempts[0].End)
	}
	if len(Attempts([]Entry{measured(Before, "c")})) != 0 {
		t.Fatal("a receipt with no packet grouped into an attempt")
	}
}

// TestABeforeMeasurementFiledAfterItsOwnPacketIsStillItsAttempts is the
// exception to the rule above: a receipt that filed its box's measurement after
// the packet but before the session settled has not started another attempt.
// Only a measurement after the process was reaped, the promise re-derived, the
// work anchored or a pass begun can be the next box's.
func TestABeforeMeasurementFiledAfterItsOwnPacketIsStillItsAttempts(t *testing.T) {
	exited := Entry{Envelope: ledger.Envelope{Kind: ledger.ThreadLifecycle}, Lifecycle: &Lifecycle{Event: Exited}}
	entries := []Entry{
		packetRecord("p1"),
		nativeRecord(),
		measured(Before, "c"),
		measured(After, "c"),
		exited,
		anchorRecord("a"),
		briefRecord("b1"),
		measured(Before, "c"),
		packetRecord("p2"),
	}
	attempts := Attempts(entries)
	if len(attempts) != 2 {
		t.Fatalf("grouped %d attempts, want 2", len(attempts))
	}
	if len(attempts[0].Before) != 1 || len(attempts[0].After()) != 1 {
		t.Fatalf("attempt 1 holds %d before and %d after measurements, want one each", len(attempts[0].Before), len(attempts[0].After()))
	}
	if _, anchored := attempts[0].Anchor(); !anchored || len(attempts[0].Passes) != 1 {
		t.Fatal("attempt 1 lost its anchor or its pass to a measurement it filed after its packet")
	}
	if attempts[0].End != 7 || attempts[1].Start != 7 || len(attempts[1].Before) != 1 {
		t.Fatalf("attempt bounds are [%d,%d) and [%d,%d)", attempts[0].Start, attempts[0].End, attempts[1].Start, attempts[1].End)
	}
}
