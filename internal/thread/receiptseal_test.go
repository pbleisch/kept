package thread

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/run"
)

// receiptPaceLines is how much of a session the pace is measured over. Long
// enough to carry the journal past ledger.SegmentThreshold, so the measured
// window holds a seal — Git commits and all — rather than only local appends.
const receiptPaceLines = 700

// receiptPaceFloor is the pace a journaling adapter has to keep, in lines a
// second, to be reading a runtime rather than holding it up: Claude Code was
// measured streaming hundreds of lines a second, and an adapter appending each
// one to a ref retained two to eleven (docs/decision/0084). Measured here at
// about 950 lines a second over 700 lines with one seal inside the window, so
// the floor leaves nearly five times the margin for a loaded machine.
const receiptPaceFloor = 200

// TestAJournalingAdapterKeepsUpWithARuntimeStreamingHundredsOfLinesASecond is
// the claim the journal is for: an adapter that makes every entry durable as it
// accepts it is still faster than the runtime writing them.
//
// It is measured through the whole write path a session uses — the adapter's
// stream, the receipt, a ledger on a real ref — because what was slow was the
// ledger, not the adapter. The first replay is outside the measurement: it pays
// for the run's own declaration, one commit that happens once per run and not
// per line. Everything after it is the pace a streaming runtime meets, and the
// window is long enough to contain a seal, so the cost of reaching the ledger is
// inside the number rather than deferred past it.
//
// The pace is evidence only if the lines were really retained, so the receipt is
// read back afterward and asked for every one. See docs/decision/0087.
func TestAJournalingAdapterKeepsUpWithARuntimeStreamingHundredsOfLinesASecond(t *testing.T) {
	repository := receiptRepository(t)
	store := ledger.NewRefStore(repository)
	filed := ledger.Open(store, run.Start("test", nil))
	receipt, err := NewReceipt(filed, MintID())
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := receipt.ForAdapter(Claude, "replay")
	if err != nil {
		t.Fatal(err)
	}
	stream := &claudeStream{receipt: adapter, secret: []byte(replaySecret)}

	session := capturedSession(t)
	perReplay := bytes.Count(session, []byte{'\n'})
	if _, err := stream.Write(session); err != nil {
		t.Fatal(err)
	}

	started := time.Now()
	streamed := 0
	for streamed < receiptPaceLines {
		if _, err := stream.Write(session); err != nil {
			t.Fatal(err)
		}
		streamed += perReplay
	}
	took := time.Since(started)

	// Asked before the stream is closed, because closing seals: a measurement
	// whose window held no seal would be the pace of the journal alone, which is
	// not the pace a session keeps.
	if sealed := len(receiptSegments(t, store, receipt.ID())); sealed == 0 {
		t.Fatalf("%d lines left the journal unsealed, so the measured pace never reached the ledger", streamed)
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	if err := filed.Close("0"); err != nil {
		t.Fatal(err)
	}

	pace := float64(streamed) / took.Seconds()
	if pace < receiptPaceFloor {
		t.Fatalf("the adapter retained %d lines in %s, %.0f lines a second; a runtime streams hundreds, and the floor is %d",
			streamed, took, pace, receiptPaceFloor)
	}

	entries, quarantined, err := ReadReceipt(store, receipt.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(quarantined) != 0 {
		t.Fatalf("the replay quarantined its own evidence: %#v", quarantined)
	}
	if natives := nativeCount(entries); natives != streamed+perReplay {
		t.Fatalf("the receipt holds %d native events for %d accepted lines, so the pace was bought by losing lines", natives, streamed+perReplay)
	}
	t.Logf("%d lines in %s: %.0f lines a second", streamed, took, pace)
}

// TestASpanSealedTwiceIsReadOnce is the duplicate a crash makes.
//
// A seal files the span and then records in the journal that it has, in that
// order: a run that dies between the two leaves a span the ledger already holds
// and a journal that does not know it, so the next run seals it again. That is
// the safe side of the order — the alternative loses a span the marker claims —
// and it is only safe if a receipt read twice reads once.
//
// The journal here is a real one, left exactly as that crash leaves it: a
// session streams, its span is sealed, and the marker is rewound from the file
// the session wrote. Nothing simulates the seal, so the two index entries name
// the same address because the spans are the same bytes, and the entries inside
// them carry the writer and sequence the first run gave them. See
// docs/decision/0087.
func TestASpanSealedTwiceIsReadOnce(t *testing.T) {
	repository := receiptRepository(t)
	store := ledger.NewRefStore(repository)
	first := ledger.Open(store, run.Start("test", nil))
	receipt, err := NewReceipt(first, MintID())
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := receipt.ForAdapter(Claude, "replay")
	if err != nil {
		t.Fatal(err)
	}
	stream := &claudeStream{receipt: adapter, secret: []byte(replaySecret)}

	session := capturedSession(t)
	lines := 0
	for range 3 {
		if _, err := stream.Write(session); err != nil {
			t.Fatal(err)
		}
		lines += bytes.Count(session, []byte{'\n'})
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}

	// The journal as the dead run left it: everything it had journaled, and no
	// record that any of it was filed.
	path, err := ledger.JournalPath(store, string(receipt.ID()))
	if err != nil {
		t.Fatal(err)
	}
	rewound := rewindSeal(t, path)
	if err := first.Close("0"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, rewound, 0o600); err != nil {
		t.Fatal(err)
	}

	// The next run, which recovers a journal nobody holds before its own first
	// ledger write — and seals that span a second time.
	second := ledger.Open(store, run.Start("test", nil))
	other, err := NewReceipt(second, MintID())
	if err != nil {
		t.Fatal(err)
	}
	if err := other.Started(); err != nil {
		t.Fatal(err)
	}
	if err := second.Close("0"); err != nil {
		t.Fatal(err)
	}

	// The receipt really was sealed twice. Without this the test could pass on a
	// recovery that silently filed nothing, which is the failure the write order
	// was chosen to avoid.
	segments := receiptSegments(t, store, receipt.ID())
	if len(segments) != 2 {
		t.Fatalf("the receipt names %d sealed spans, want the two a crash between a seal and its marker leaves", len(segments))
	}
	if segments[0].Artifact.Address != segments[1].Artifact.Address {
		t.Fatalf("the two seals filed different spans, %s and %s, so they are not one span sealed twice",
			segments[0].Artifact.Address, segments[1].Artifact.Address)
	}

	entries, quarantined, err := ReadReceipt(store, receipt.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(quarantined) != 0 {
		t.Fatalf("a span sealed twice quarantined the receipt: %#v", quarantined)
	}
	if natives := nativeCount(entries); natives != lines {
		t.Fatalf("the receipt reads %d native events for the %d lines the session streamed", natives, lines)
	}
	seen := map[mark]bool{}
	for _, entry := range entries {
		at := mark{writer: entry.Writer, seq: entry.Seq}
		if seen[at] {
			t.Fatalf("entry %d of run %s is read twice", at.seq, at.writer)
		}
		seen[at] = true
	}
}

// rewindSeal is the journal's bytes with the record of its last seal removed,
// which is what a run that filed a span and died before marking it leaves
// behind. It insists the marker was there: a journal with none was sealed by
// some other path than the one this test is about.
func rewindSeal(t *testing.T, path string) []byte {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimRight(src, "\n"), []byte{'\n'})
	if len(lines) == 0 {
		t.Fatalf("%s is empty, so the session journaled nothing", path)
	}
	var line struct {
		Sealed *struct {
			Through int64  `json:"through"`
			Segment string `json:"segment"`
		} `json:"sealed"`
	}
	last := lines[len(lines)-1]
	if err := json.Unmarshal(last, &line); err != nil {
		t.Fatalf("%s ends in a line that is not a journal record: %v", path, err)
	}
	if line.Sealed == nil || line.Sealed.Segment == "" {
		t.Fatalf("%s does not end in the record of a seal, so the span it holds was never filed", path)
	}
	rewound := bytes.Join(lines[:len(lines)-1], []byte{'\n'})
	return append(rewound, '\n')
}

// receiptSegments is every sealed span a receipt's own stream names, in the
// order it names them. It reads the stream rather than ReadReceipt, which
// expands the spans and drops the entries naming them.
func receiptSegments(t *testing.T, store ledger.Store, id ID) []ledger.Segment {
	t.Helper()
	where, err := ledger.ThreadPath(string(id))
	if err != nil {
		t.Fatal(err)
	}
	src, err := store.Read(where)
	if err != nil {
		t.Fatal(err)
	}
	var segments []ledger.Segment
	for _, raw := range bytes.Split(src, []byte{'\n'}) {
		line := bytes.TrimSpace(raw)
		if len(line) == 0 {
			continue
		}
		var entry Entry
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Fatalf("the receipt holds a line that is not an entry: %v", err)
		}
		if entry.Segment != nil {
			segments = append(segments, *entry.Segment)
		}
	}
	return segments
}
