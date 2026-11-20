package thread

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/lock"
	"github.com/pbleisch/kept/internal/run"
)

// journalFile is where a thread's receipt journal lives in a repository.
func journalFile(t *testing.T, repository string, id ID) string {
	t.Helper()
	common, err := lock.CommonDir(repository)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(common, filepath.FromSlash(ledger.JournalDir), string(id)+".jsonl")
}

// streamSegments is how many segment entries a thread's receipt stream holds.
func streamSegments(t *testing.T, store ledger.Store, id ID) int {
	t.Helper()
	where, err := ledger.ThreadPath(string(id))
	if err != nil {
		t.Fatal(err)
	}
	stream, err := store.Read(where)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for line := range bytes.SplitSeq(stream, []byte{'\n'}) {
		var entry Entry
		if json.Unmarshal(line, &entry) == nil && entry.Segment != nil {
			n++
		}
	}
	return n
}

// journalReceipt writes the captured session through the Claude adapter into a
// receipt on a real ref, and leaves it journaled and unsealed.
func journalReceipt(t *testing.T, repository string) (*ledger.Ledger, *Receipt) {
	t.Helper()
	l := ledger.Open(ledger.NewRefStore(repository), run.Start("test", nil))
	receipt, err := NewReceipt(l, MintID())
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := receipt.ForAdapter(Claude, "replay")
	if err != nil {
		t.Fatal(err)
	}
	stream := &claudeStream{receipt: adapter, secret: []byte(replaySecret)}
	if _, err := stream.Write(capturedSession(t)); err != nil {
		t.Fatal(err)
	}
	return l, receipt
}

// TestASpanSealedTwiceReadsOnce is the read half of a recovery that needs no
// exclusion: a run seals a span, and the same span is sealed again — as a run
// recovering a journal that was sealed before its writer died would seal it.
// The stream then names the segment twice, and the receipt reads each line
// once. See docs/decision/0087.
func TestASpanSealedTwiceReadsOnce(t *testing.T) {
	repository := receiptRepository(t)
	_, receipt := journalReceipt(t, repository)
	journaled, err := os.ReadFile(journalFile(t, repository, receipt.ID()))
	if err != nil {
		t.Fatal(err)
	}
	if err := receipt.Seal(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(journalFile(t, repository, receipt.ID())); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a sealed journal was left behind: %v", err)
	}

	// The same span, left as a dead run's journal, and the next run's first
	// write seals it again.
	if err := os.WriteFile(journalFile(t, repository, receipt.ID()), journaled, 0o600); err != nil {
		t.Fatal(err)
	}
	store := ledger.NewRefStore(repository)
	next := ledger.Open(store, run.Start("test", nil))
	other, err := NewReceipt(next, MintID())
	if err != nil {
		t.Fatal(err)
	}
	if err := other.Started(); err != nil {
		t.Fatal(err)
	}
	if segments := streamSegments(t, store, receipt.ID()); segments != 2 {
		t.Fatalf("the stream names %d segments; the test needs the span sealed twice", segments)
	}

	entries, quarantined, err := ReadReceipt(store, receipt.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(quarantined) != 0 {
		t.Fatalf("quarantined: %#v", quarantined)
	}
	if got, want := nativeCount(entries), len(sessionTypes(t)); got != want {
		t.Fatalf("the receipt reads %d native events for %d journaled once and sealed twice", got, want)
	}
	for _, entry := range entries {
		if entry.Segment != nil {
			t.Fatal("a reader was handed a segment entry rather than what it holds")
		}
		if entry.Transcript != nil {
			if _, err := ledger.ReadArtifact(store, entry.Transcript.Artifact); err != nil {
				t.Fatalf("a journaled transcript was not filed by the seal: %v", err)
			}
		}
	}
}

// TestALiveRunsJournalIsNotTakenByAnother is the one thing the journal's lock
// guards: removal. A second run's first write recovers every journal it can
// lock, and must leave alone one whose writer is still running.
func TestALiveRunsJournalIsNotTakenByAnother(t *testing.T) {
	repository := receiptRepository(t)
	_, receipt := journalReceipt(t, repository)

	store := ledger.NewRefStore(repository)
	other := ledger.Open(store, run.Start("test", nil))
	beside, err := NewReceipt(other, MintID())
	if err != nil {
		t.Fatal(err)
	}
	if err := beside.Started(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(journalFile(t, repository, receipt.ID())); err != nil {
		t.Fatalf("another run took a live run's journal: %v", err)
	}
	where, _ := ledger.ThreadPath(string(receipt.ID()))
	if _, err := store.Read(where); !errors.Is(err, ledger.ErrNoStream) {
		t.Fatalf("another run sealed a live run's journal: %v", err)
	}

	// And a second run cannot journal the live run's thread either, because
	// the live run would seal and remove the file under it.
	sameThread, err := NewReceipt(other, receipt.ID())
	if err != nil {
		t.Fatal(err)
	}
	if err := sameThread.Started(); err == nil || !strings.Contains(err.Error(), "another run is writing") {
		t.Fatalf("journaling a live run's thread = %v, want a refusal", err)
	}

	if err := receipt.Seal(); err != nil {
		t.Fatal(err)
	}
	entries, _, err := ReadReceipt(store, receipt.ID())
	if err != nil {
		t.Fatal(err)
	}
	if got, want := nativeCount(entries), len(sessionTypes(t)); got != want {
		t.Fatalf("the live run sealed %d of its %d native events", got, want)
	}
}

// TestALineAKillCutShortIsNotRecovered is a write that was interrupted: its
// writer never saw write(2) return, so it is not a line kept had read. The
// recovery seals every whole line before it and leaves the fragment out,
// rather than filing it where the reader would quarantine it.
func TestALineAKillCutShortIsNotRecovered(t *testing.T) {
	repository := receiptRepository(t)
	_, receipt := journalReceipt(t, repository)
	path := journalFile(t, repository, receipt.ID())
	journaled, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Stand in for a dead run: the same bytes, plus half of one more line,
	// under a file nobody holds a lock on.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	torn := append(append([]byte(nil), journaled...), `{"v":1,"kind":"thread_native","at":"2026-`...)
	if err := os.WriteFile(path, torn, 0o600); err != nil {
		t.Fatal(err)
	}

	store := ledger.NewRefStore(repository)
	next := ledger.Open(store, run.Start("test", nil))
	other, err := NewReceipt(next, MintID())
	if err != nil {
		t.Fatal(err)
	}
	if err := other.Started(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a recovered journal was left behind: %v", err)
	}
	entries, quarantined, err := ReadReceipt(store, receipt.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(quarantined) != 0 {
		t.Fatalf("the torn line was filed and quarantined: %#v", quarantined)
	}
	if got, want := nativeCount(entries), len(sessionTypes(t)); got != want {
		t.Fatalf("recovery kept %d of %d whole native events", got, want)
	}
}

// TestAReceiptInFilesIsAppendedALineAtATime is the other side of which store
// journals. A store kept in files has no Git common directory and no commit to
// amortize, so it appends each line as it always did and leaves no journal.
func TestAReceiptInFilesIsAppendedALineAtATime(t *testing.T) {
	receipt, store := testReceipt(t)
	if err := receipt.Started(); err != nil {
		t.Fatal(err)
	}
	entries, _, err := ReadReceipt(store, receipt.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Lifecycle == nil {
		t.Fatalf("a receipt in files read %#v before any seal, want its one line", entries)
	}
}
