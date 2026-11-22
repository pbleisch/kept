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

// TestAJournalThisRunCannotLookForIsReportedAndOneThatIsNotThereIsNot is the
// asymmetry [Ledger.recoverJournals] exists to hold, and the one it answered
// wrongly.
//
// A journal is the only copy of a dead session's lines. While that session ran
// the journal was the durable record and the ledger held nothing of it, so the
// next run's first ledger write is the moment those lines move into the ledger
// and nothing writes them again (docs/decision/0018 and 0084). That makes the
// two ways of finding nothing completely different things:
//
//   - Nowhere to look is not a failure. No run has journaled here, so there is
//     nothing to recover, and this is the first ledger write of *every* run —
//     failing it would refuse a ratification or an observation over a directory
//     nobody was going to write to.
//   - Somewhere to look that could not be looked in is a failure. The run
//     cannot tell whether a session's whole output is sitting there, and
//     answering no error is how it disappears with nobody told.
//
// Asserted as whether the write was refused, rather than as the shape of an
// error, because what is at stake is whether the run carried on as though
// there were nothing to recover.
func TestAJournalThisRunCannotLookForIsReportedAndOneThatIsNotThereIsNot(t *testing.T) {
	cases := []struct {
		name     string
		journal  func(*testing.T) string
		reported bool
	}{
		{"no run has journaled here", func(t *testing.T) string {
			return filepath.Join(t.TempDir(), "journal")
		}, false},
		{"the path runs through something that is not a directory", func(t *testing.T) string {
			inTheWay := filepath.Join(t.TempDir(), "in-the-way")
			if err := os.WriteFile(inTheWay, []byte("not a directory"), 0o600); err != nil {
				t.Fatal(err)
			}
			return filepath.Join(inTheWay, "kept", "journal")
		}, false},
		{"the journals are there and cannot be listed", func(t *testing.T) string {
			if os.Geteuid() == 0 {
				t.Skip("root reads a directory whatever its mode, so there is no unreadable directory to make")
			}
			dir := unreadableJournals(t)
			return dir
		}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := journalingStore{Store: store(t), dir: c.journal(t)}
			l, _ := open(t, s)

			// Any first ledger write. A run that is not a session's reaches
			// recovery through the same declaration, which is the reason the
			// absent cases must not fail.
			err := l.AppendThread("thread-a", ThreadLifecycle, time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC), func(envelope Envelope) ([]byte, error) {
				return json.Marshal(map[string]any{"v": envelope.V, "kind": envelope.Kind, "seq": envelope.Seq})
			})
			switch {
			case c.reported && err == nil:
				t.Fatal("the run wrote the ledger without being able to look for the journals a dead session may have left, so those lines are lost and nothing says so")
			case !c.reported && err != nil:
				t.Fatalf("the run refused its first ledger write over journals nothing had written: %v", err)
			}
			if !c.reported {
				return
			}
			if !strings.Contains(err.Error(), s.dir) {
				t.Fatalf("the failure does not say which directory could not be looked in: %v", err)
			}
		})
	}
}

// TestAJournalThatIsThereAndCanBeSealedReachesTheLedger is the other half of
// the same write: the recovery this one is about actually recovering a dead
// session's lines, so that the case above is a failure to do something that
// works rather than a failure to do nothing.
func TestAJournalThatIsThereAndCanBeSealedReachesTheLedger(t *testing.T) {
	dir := t.TempDir()
	s := journalingStore{Store: store(t), dir: dir}

	// A journal exactly as a run that died leaves one: entries, no seal marker,
	// and no live writer holding its lock.
	dead, _ := open(t, journalingStore{Store: store(t), dir: dir})
	journal, err := dead.journalFor("thread-a")
	if err != nil {
		t.Fatal(err)
	}
	for seq := range 3 {
		line := []byte(`{"v":1,"kind":"thread_native","seq":` + string(rune('1'+seq)) + `}`)
		if err := dead.journalEntry(journal, line, Mark{Writer: "01deadrunwrotethisandstopped", Seq: uint64(seq + 1)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := journal.file.Close(); err != nil {
		t.Fatal(err) // Closing releases the lock, which is what a death does.
	}

	next, _ := open(t, s)
	if err := next.recoverJournals(); err != nil {
		t.Fatalf("recover the journal a dead run left: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "thread-a.jsonl")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the journal is still there after it was sealed: %v", err)
	}

	where, err := ThreadPath("thread-a")
	if err != nil {
		t.Fatal(err)
	}
	held, err := s.Read(where)
	if err != nil {
		t.Fatalf("read the receipt the recovery filed: %v", err)
	}
	if !strings.Contains(string(held), string(ThreadSegment)) {
		t.Fatalf("the recovered receipt holds no sealed span: %q", held)
	}
}

// unreadableJournals is a journal directory holding a journal, which this
// process cannot list. The mode is put back on cleanup so the temporary
// directory can be removed.
func unreadableJournals(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "journal")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "thread-b.jsonl"), []byte(`{"entry":{"v":1,"kind":"thread_native"},"mark":{"writer":"01deadrun","seq":1}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if _, err := os.ReadDir(dir); err == nil {
		t.Skip("this filesystem lists a directory with no permissions, so there is no unreadable directory to make")
	}
	return dir
}
