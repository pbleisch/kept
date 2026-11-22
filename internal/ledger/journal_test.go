package ledger

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// blockedJournalStore says where this run may journal and is wrong about it:
// the path it names cannot be made into a directory, or the journal file in it
// cannot be opened. A store can be wrong this way for reasons that have nothing
// to do with kept — a read-only mount, a full disk, a name something else is
// already using — which is why it is a case and not a bug.
type blockedJournalStore struct {
	Store
	dir string
}

func (s blockedJournalStore) JournalDir() (string, error) { return s.dir, nil }

// TestAJournalThatCannotBeOpenedCostsThePaceAndNotTheLines is the asymmetry
// [Ledger.journalFor] exists to hold.
//
// A journal buys pace. A receipt went straight to the ledger before
// docs/decision/0087 and goes there still when a store has nowhere to journal,
// so every way of not owning a journal — including the ways that are a failure
// rather than an absence — has to cost the session its pace and not its lines.
// Reporting the failure instead would fail the entry, and an entry that fails
// after the adapter has accepted the line is a line kept read and lost, which
// is the one thing journaling was introduced to stop.
//
// Asserted as lines in the ledger rather than as the shape of an error, because
// the error is a detail of how the fallback is signalled and the lines are the
// guarantee. The wrapping is asked of journalFor directly as well, so that a
// future caller reading errors.Is is reading something a test holds.
func TestAJournalThatCannotBeOpenedCostsThePaceAndNotTheLines(t *testing.T) {
	cases := []struct {
		name    string
		journal func(*testing.T) string
	}{
		{"the directory cannot be made", func(t *testing.T) string {
			inTheWay := filepath.Join(t.TempDir(), "in-the-way")
			if err := os.WriteFile(inTheWay, []byte("not a directory"), 0o600); err != nil {
				t.Fatal(err)
			}
			return filepath.Join(inTheWay, "kept", "journal")
		}},
		{"the file cannot be opened", func(t *testing.T) string {
			dir := filepath.Join(t.TempDir(), "journal")
			// A directory where the journal's file goes: the name exists and
			// cannot be written to.
			if err := os.MkdirAll(filepath.Join(dir, "thread-a.jsonl"), 0o755); err != nil {
				t.Fatal(err)
			}
			return dir
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := blockedJournalStore{Store: store(t), dir: c.journal(t)}
			l, _ := open(t, s)

			if _, err := l.journalFor("thread-a"); !errors.Is(err, ErrNoJournal) {
				t.Fatalf("journalFor = %v, want an error reporting ErrNoJournal so the entry falls back to the ledger", err)
			}

			// A session's worth of entries, at the pace of a runtime: a
			// lifecycle event the parent writes, then the lines the runtime
			// produces, which are the journaled kinds.
			at := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
			kinds := []Kind{ThreadLifecycle, ThreadNative, ThreadNative, ThreadCost, ThreadNative}
			for i, kind := range kinds {
				err := l.AppendThread("thread-a", kind, at.Add(time.Duration(i)*time.Second), func(envelope Envelope) ([]byte, error) {
					return json.Marshal(map[string]any{"v": envelope.V, "kind": envelope.Kind, "seq": envelope.Seq})
				})
				if err != nil {
					t.Fatalf("receipt entry %d (%s) failed because this run could not journal: %v", i+1, kind, err)
				}
			}
			if err := l.Close("0"); err != nil {
				t.Fatal(err)
			}

			where, err := ThreadPath("thread-a")
			if err != nil {
				t.Fatal(err)
			}
			src, err := s.Read(where)
			if err != nil {
				t.Fatalf("read the receipt of a run that could not journal: %v", err)
			}
			var got []Kind
			for line := range strings.SplitSeq(strings.TrimSpace(string(src)), "\n") {
				var envelope Envelope
				if err := json.Unmarshal([]byte(line), &envelope); err != nil {
					t.Fatal(err)
				}
				got = append(got, envelope.Kind)
			}
			if len(got) != len(kinds) {
				t.Fatalf("the receipt holds %d of the %d entries this run recorded: %v", len(got), len(kinds), got)
			}
			for i, kind := range kinds {
				if got[i] != kind {
					t.Fatalf("receipt entry %d is %q, want %q", i+1, got[i], kind)
				}
			}
		})
	}
}

// TestACallersMistakeAboutAThreadIsNotAMissingJournal is the other side of that
// asymmetry: the fallback is for a journal this run cannot own, never for an
// argument the ledger cannot use.
//
// A thread id with no receipt path has nowhere to journal and nowhere to append
// either, so writing it to the ledger instead is not a slower way of keeping
// the line — there is no stream to keep it in. That has to be reported.
func TestACallersMistakeAboutAThreadIsNotAMissingJournal(t *testing.T) {
	s := journalingStore{Store: store(t), dir: filepath.Join(t.TempDir(), "journal")}
	l, _ := open(t, s)
	_, err := l.journalFor("bad/thread")
	if err == nil || errors.Is(err, ErrNoJournal) {
		t.Fatalf("journalFor a thread with no receipt path = %v, want the mistake reported rather than treated as a missing journal", err)
	}
	if _, err := l.PutThreadArtifact("bad/thread", SessionTranscriptArtifact, []byte("x")); err == nil {
		t.Fatal("an artifact was stored for a thread with no receipt path")
	}
}

// journalingStore is a store with a journal directory that works.
type journalingStore struct {
	Store
	dir string
}

func (s journalingStore) JournalDir() (string, error) { return s.dir, nil }
