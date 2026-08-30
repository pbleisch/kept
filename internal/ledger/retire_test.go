package ledger

import (
	"strings"
	"testing"
	"time"
)

// The one line in this repository's own ledger that is spelled the old way, and
// the property that keeps it meaning what it meant.
//
// `decision:0070-first-of-a-pair` was ratified and then retired on 2026-08-25,
// under the value `released`. Nothing writes that word again, the bytes are
// never rewritten, and docs/decision/0074 is why. What holds it together is
// [retirement], which is one function two callers ask — the malformed-act check
// on the way in, and the read that hands records to everything else.
//
// The failure this guards is quiet in the direction that matters. Delete
// [retiredHistorical] and that line is quarantined; a quarantined act is not in
// the fold; an artifact a person retired reads as ratified. An act undone by a
// refactor is the one thing an append-only record must not be able to do.
func TestARetirementWrittenUnderTheOldSpellingIsStillARetirement(t *testing.T) {
	s := store(t)
	at := time.Date(2026, 8, 25, 20, 56, 28, 0, time.UTC)

	// Written the way the real line is written, as bytes rather than through
	// the writer — because the writer cannot produce this any more, which is
	// the whole point.
	line := `{"v":1,"kind":"released","at":"` + at.Format(time.RFC3339Nano) +
		`","writer":"246xhk9etzyakr0z6gcr7h75nh","seq":1,"artifact":"decision:0070-first-of-a-pair","by":"the operator"}`
	writeLine(t, s, line)

	records, quarantined := read(t, s)
	if len(quarantined) != 0 {
		t.Fatalf("the historical spelling was quarantined, so the retirement is not in the fold: %+v", quarantined)
	}
	if len(records) != 1 {
		t.Fatalf("read %d acts, want 1", len(records))
	}
	if records[0].Kind != Retired {
		t.Errorf("Kind = %q, want %q — one meaning must leave the read as one word", records[0].Kind, Retired)
	}
}

// The other half: the old word is read and never written. A retirement recorded
// today carries the new value, so the historical spelling stays exactly one line
// long and does not grow.
func TestARetirementRecordedNowIsWrittenAsRetire(t *testing.T) {
	s := store(t)
	l, _ := open(t, s)

	if _, err := l.Retire("promise:p", "sha256:aa", 1, "the operator", "", Where{}); err != nil {
		t.Fatal(err)
	}
	raw, err := s.Read(Acts)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(raw); !strings.Contains(got, `"kind":"retire"`) {
		t.Errorf("a retirement recorded now is not written as retire:\n%s", got)
	} else if strings.Contains(got, `"kind":"released"`) {
		t.Errorf("a retirement recorded now still writes the old spelling:\n%s", got)
	}
}

// An act kind nothing knows is still quarantined, which is what makes the
// acceptance above a decision rather than a hole. A reader that accepted
// anything would have no way to report the record it could not identify.
func TestAnActKindNothingKnowsIsStillQuarantined(t *testing.T) {
	s := store(t)
	writeLine(t, s, `{"v":1,"kind":"relinquished","at":"2026-08-25T20:56:28Z","writer":"r","seq":1,"artifact":"promise:p","by":"the operator"}`)

	records, quarantined := read(t, s)
	if len(records) != 0 {
		t.Errorf("read %d acts, want 0", len(records))
	}
	if len(quarantined) != 1 {
		t.Fatalf("quarantined %d lines, want 1", len(quarantined))
	}
	if !strings.Contains(quarantined[0].Why, "not an act this build knows") {
		t.Errorf("the reason was %q", quarantined[0].Why)
	}
}

// writeLine puts bytes in the act stream without going through the writer,
// because the value under test is one the writer can no longer produce.
func writeLine(t *testing.T, s Store, line string) {
	t.Helper()
	if err := s.Append(Acts, []byte(line)); err != nil {
		t.Fatal(err)
	}
}
