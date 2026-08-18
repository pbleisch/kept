package ledger

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/run"
)

func store(t *testing.T) *Files {
	t.Helper()
	return NewFiles(filepath.Join(t.TempDir(), ".kept"))
}

func open(t *testing.T, s Store) (*Ledger, *run.Record) {
	t.Helper()
	rec := run.Start("test", []string{"ratify", "promise:p"})
	return Open(s, rec), rec
}

func read(t *testing.T, s Store) ([]Decision, []Quarantined) {
	t.Helper()
	records, quarantined, err := ReadDecisions(s)
	if err != nil {
		t.Fatalf("ReadDecisions: %v", err)
	}
	return records, quarantined
}

// Every record names its run, carries its seq and its envelope version. None of
// the three can be added afterwards to records already written, which is why
// they are here from the first durable write. See docs/decision/0013.
func TestEveryRecordNamesItsRunAndCarriesSeqAndVersion(t *testing.T) {
	s := store(t)
	l, rec := open(t, s)

	if _, err := l.Ratify("promise:a", "sha256:aa", 1, "paul", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Ratify("promise:b", "sha256:bb", 1, "paul", ""); err != nil {
		t.Fatal(err)
	}

	records, _ := read(t, s)
	if len(records) != 2 {
		t.Fatalf("read %d records, want 2", len(records))
	}
	for i, d := range records {
		if d.V != Version {
			t.Errorf("record %d: V = %d, want %d", i, d.V, Version)
		}
		if d.Writer != rec.ID {
			t.Errorf("record %d: Writer = %q, want %q", i, d.Writer, rec.ID)
		}
		if want := uint64(i + 1); d.Seq != want {
			t.Errorf("record %d: Seq = %d, want %d", i, d.Seq, want)
		}
		if d.At.IsZero() {
			t.Errorf("record %d has no event time, and the fold is by event time", i)
		}
	}
}

// The guard on the fold. Union merge interleaves two checkouts' appends
// arbitrarily, so a read that trusted file order would be silently wrong — and
// it would look right in every test with one run in it, which is why this one
// has three and writes them backwards.
func TestTheFoldIgnoresFileOrder(t *testing.T) {
	s := store(t)
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	// Written newest first, as a union merge is entitled to leave them.
	appendRaw(t, s, Decision{
		Envelope: Envelope{V: 1, Kind: Released, At: base.Add(2 * time.Hour), Writer: "runb", Seq: 1},
		Artifact: "promise:p", By: "paul",
	})
	appendRaw(t, s, Decision{
		Envelope: Envelope{V: 1, Kind: Ratified, At: base, Writer: "runa", Seq: 1},
		Artifact: "promise:p", Hash: "sha256:aa", Norm: 1, By: "paul",
	})
	appendRaw(t, s, Decision{
		Envelope: Envelope{V: 1, Kind: Ratified, At: base.Add(time.Hour), Writer: "runa", Seq: 2},
		Artifact: "promise:p", Hash: "sha256:bb", Norm: 1, By: "paul",
	})

	records, _ := read(t, s)
	got := make([]string, len(records))
	for i, d := range records {
		got[i] = string(d.Kind) + " " + d.Hash
	}
	want := "ratified sha256:aa|ratified sha256:bb|released "
	if strings.Join(got, "|") != want {
		t.Fatalf("folded to %q, want %q — the read is following file order", strings.Join(got, "|"), want)
	}
}

// The guard on seq. Two records written in one tick can carry the same event
// time, and a clock stepped backwards makes pure event-time folding wrong on a
// single machine. Without seq, these two would fold in whichever order the file
// happened to hold them.
func TestSeqOrdersRecordsThatShareAnEventTime(t *testing.T) {
	s := store(t)
	at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	appendRaw(t, s, Decision{
		Envelope: Envelope{V: 1, Kind: Released, At: at, Writer: "runa", Seq: 2},
		Artifact: "promise:p", By: "paul",
	})
	appendRaw(t, s, Decision{
		Envelope: Envelope{V: 1, Kind: Ratified, At: at, Writer: "runa", Seq: 1},
		Artifact: "promise:p", Hash: "sha256:aa", Norm: 1, By: "paul",
	})

	records, _ := read(t, s)
	if records[0].Kind != Ratified || records[1].Kind != Released {
		t.Fatalf("two records at one instant folded to %s then %s; seq says the ratification came first",
			records[0].Kind, records[1].Kind)
	}
}

// The guard on quarantine. A crash mid-append leaves a torn line, and the line
// most likely to be torn is the last one — so a fold that halted would make one
// crash stop the system permanently, and a fold that guessed would invent a
// decision nobody made.
func TestATornLineIsSetAsideAndTheRestIsRead(t *testing.T) {
	s := store(t)
	l, _ := open(t, s)
	if _, err := l.Ratify("promise:a", "sha256:aa", 1, "paul", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.Append(Decisions, []byte(`{"v":1,"kind":"ratified","at":"2026-03-0`)); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Ratify("promise:b", "sha256:bb", 1, "paul", ""); err != nil {
		t.Fatal(err)
	}

	records, quarantined := read(t, s)
	if len(records) != 2 {
		t.Fatalf("read %d records around one torn line, want 2", len(records))
	}
	if len(quarantined) != 1 {
		t.Fatalf("quarantined %d lines, want 1", len(quarantined))
	}
	if quarantined[0].Line != 2 {
		t.Errorf("the torn line is reported as line %d, and it is line 2", quarantined[0].Line)
	}
	if quarantined[0].Raw == "" || quarantined[0].Why == "" {
		t.Errorf("a quarantined line is reported without its content or its reason: %#v", quarantined[0])
	}
}

// A record whose envelope this build cannot read is set aside rather than read
// as something else. The alternative is a future kept writing a shape this one
// misreads into a decision, which is the failure the version exists to prevent.
func TestARecordThisBuildCannotReadIsSetAside(t *testing.T) {
	for _, tc := range []struct{ name, line, why string }{
		{"no envelope version",
			`{"kind":"ratified","at":"2026-03-01T00:00:00Z","writer":"r","seq":1,"artifact":"promise:p","hash":"sha256:aa","norm":1}`,
			"envelope version"},
		{"a version from the future",
			`{"v":99,"kind":"ratified","at":"2026-03-01T00:00:00Z","writer":"r","seq":1,"artifact":"promise:p","hash":"sha256:aa","norm":1}`,
			"this build reads"},
		{"a kind nothing here knows",
			`{"v":1,"kind":"delegated","at":"2026-03-01T00:00:00Z","writer":"r","seq":1,"artifact":"promise:p"}`,
			"not a decision this build knows"},
		{"no writer",
			`{"v":1,"kind":"ratified","at":"2026-03-01T00:00:00Z","seq":1,"artifact":"promise:p","hash":"sha256:aa","norm":1}`,
			"which run"},
		// A ratification naming no content version would grant standing to
		// whatever the file said next, which is the one thing standing must
		// never do.
		{"a ratification with no content hash",
			`{"v":1,"kind":"ratified","at":"2026-03-01T00:00:00Z","writer":"r","seq":1,"artifact":"promise:p","norm":1}`,
			"grants standing to nothing in particular"},
		// Recorded from the first ratification, or a later refinement of the
		// rules cannot re-hash old records under the version they were written
		// with, and refining them becomes a bulk de-ratification.
		{"a ratification with no norm",
			`{"v":1,"kind":"ratified","at":"2026-03-01T00:00:00Z","writer":"r","seq":1,"artifact":"promise:p","hash":"sha256:aa"}`,
			"re-hashed if the rules are refined"},
	} {
		s := store(t)
		if err := s.Append(Decisions, []byte(tc.line)); err != nil {
			t.Fatal(err)
		}
		records, quarantined := read(t, s)
		if len(records) != 0 {
			t.Errorf("%s: was read as a decision", tc.name)
			continue
		}
		if len(quarantined) != 1 || !strings.Contains(quarantined[0].Why, tc.why) {
			t.Errorf("%s: quarantined as %#v, and the reason should mention %q", tc.name, quarantined, tc.why)
		}
	}
}

// A ledger nobody has written and a ledger whose records were pruned look the
// same in a table, and one of them is a clone that never fetched the ledger.
// See docs/decision/0016.
func TestAnAbsentStreamIsNotAnEmptyOne(t *testing.T) {
	s := store(t)
	if !Empty(s) {
		t.Fatal("a store with no decision stream did not report as absent")
	}
	records, quarantined := read(t, s)
	if len(records) != 0 || len(quarantined) != 0 {
		t.Fatalf("an absent stream read as %d records and %d quarantined lines", len(records), len(quarantined))
	}

	if err := s.Append(Decisions, []byte("")); err != nil {
		t.Fatal(err)
	}
	if Empty(s) {
		t.Fatal("a stream that exists and holds nothing reported as absent")
	}
}

// A run's diagnostics exist so that a writer field resolves to something. A run
// that recorded nothing named nothing, so it leaves no file behind — and a run
// that recorded something leaves one whether or not it reached its own
// shutdown, because a missing Ended is the evidence of an unclean stop.
func TestOnlyARunThatRecordsWritesItsDiagnostics(t *testing.T) {
	s := store(t)
	l, rec := open(t, s)

	if _, _, err := ReadDecisions(s); err != nil {
		t.Fatal(err)
	}
	if err := l.Close(run.Clean); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Read(RunPath(rec.ID)); !errors.Is(err, ErrNoStream) {
		t.Fatalf("a run that recorded nothing left diagnostics behind: %v", err)
	}

	if _, err := l.Ratify("promise:a", "sha256:aa", 1, "paul", ""); err != nil {
		t.Fatal(err)
	}

	// Before Close, which is the state a killed run leaves.
	interrupted := readRun(t, s, rec.ID)
	if !interrupted.Ended.IsZero() || interrupted.Exit != "" {
		t.Errorf("a run that has not finished already claims an ending: ended %v, exit %q", interrupted.Ended, interrupted.Exit)
	}

	if err := l.Close(run.Clean); err != nil {
		t.Fatal(err)
	}
	finished := readRun(t, s, rec.ID)
	if finished.Ended.IsZero() {
		t.Error("a run that closed cleanly recorded no ending")
	}
	if finished.Exit != run.Clean {
		t.Errorf("Exit = %q, want %q", finished.Exit, run.Clean)
	}
	if finished.Decisions != 1 {
		t.Errorf("Decisions = %d, want 1", finished.Decisions)
	}
	if finished.Version != "test" || len(finished.Argv) == 0 {
		t.Errorf("the run does not say what it was: %#v", finished)
	}

	// The config that says whether a hostname may be written is not read yet,
	// so nothing writes one. Omitting is the fail-safe direction: attribution
	// survives it, and a repository that asked not to name machines is not
	// named in anyway. See docs/decision/0012.
	if finished.Host != "" || finished.PID != 0 {
		t.Errorf("a hostname or pid was written before anything read diagnostics.host: %q %d", finished.Host, finished.PID)
	}
}

// The layout rule that makes concurrency need no arbitration: two runs never
// choose the same path, because every path is keyed by something already
// unique. See docs/decision/0011 and 0012.
func TestTwoRunsNeverChooseOneDiagnosticsPath(t *testing.T) {
	s := store(t)
	seen := map[string]bool{}
	for range 64 {
		l, rec := open(t, s)
		if _, err := l.Ratify("promise:a", "sha256:aa", 1, "paul", ""); err != nil {
			t.Fatal(err)
		}
		path := RunPath(rec.ID)
		if seen[path] {
			t.Fatalf("two runs chose %s", path)
		}
		seen[path] = true
	}
	if _, quarantined := read(t, s); len(quarantined) != 0 {
		t.Fatalf("64 runs appending to one stream produced %d unreadable lines", len(quarantined))
	}
}

func appendRaw(t *testing.T, s Store, d Decision) {
	t.Helper()
	line, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Append(Decisions, line); err != nil {
		t.Fatal(err)
	}
}

func readRun(t *testing.T, s Store, id run.ID) run.Record {
	t.Helper()
	src, err := s.Read(RunPath(id))
	if err != nil {
		t.Fatalf("read %s: %v", RunPath(id), err)
	}
	var rec run.Record
	if err := json.Unmarshal(src, &rec); err != nil {
		t.Fatalf("%s is not readable: %v", RunPath(id), err)
	}
	return rec
}

// A record is one line, so a reader sees a whole record or nothing. Two runs
// appending to one stream is the ordinary case — decision 0011 removed the need
// for them to coordinate, and this is what that rests on.
func TestARecordIsOneLine(t *testing.T) {
	s := store(t)
	l, _ := open(t, s)
	if _, err := l.Ratify("promise:a", "sha256:aa", 1, "paul", "a note\nwith a newline in it"); err != nil {
		t.Fatal(err)
	}

	src, err := s.Read(Decisions)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(strings.TrimRight(string(src), "\n"), "\n"); n != 0 {
		t.Fatalf("one record spans %d newlines; a torn append would take its neighbours with it", n+1)
	}
}

// Files is one implementation of Store and the ledger has to move to another —
// refs/kept/ledger, before any box exists. Nothing above this line touched the
// filesystem directly, and this is the only test that does.
func TestTheFilesStoreIsUnderTheLedgerDirectory(t *testing.T) {
	kept := filepath.Join(t.TempDir(), ".kept")
	s := NewFiles(kept)
	l, rec := open(t, s)
	if _, err := l.Ratify("promise:a", "sha256:aa", 1, "paul", ""); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{Decisions, RunPath(rec.ID)} {
		if _, err := os.Stat(filepath.Join(kept, Dir, filepath.FromSlash(name))); err != nil {
			t.Errorf("%s is not under %s/%s: %v", name, kept, Dir, err)
		}
	}
}
