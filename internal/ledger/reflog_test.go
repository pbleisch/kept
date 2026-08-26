package ledger

import (
	"strings"
	"testing"
)

// The other half of the regression, and the same event. 0016 closed the vector:
// an unforced refspec makes git refuse the fetch that erased two ratifications
// from this repository on 2026-08-22. What it could not close is everything
// else that moves a ref — git update-ref, a push into the repository, a
// hand-edited config, and every clone kept init has not corrected — and against
// those there was no detection at all, because git logs no ref outside
// refs/heads/, refs/remotes/, refs/notes/ and HEAD.
//
// So this reproduces the incident rather than an abstraction of it: a ledger
// ahead of a stale remote, the forced refspec the repository actually carried,
// and an ordinary fetch. The forced spelling is put back by hand, because
// ConfigureFetch now removes it — reproducing the loss is exactly as hard as
// 0016 made it, and the value of the reflog is what happens when something
// arrives at the ref by a route 0016 does not cover.
//
// Both directions are asserted here and in the test below it: unlogged, the
// fetch takes the ledger and leaves nothing at all; logged, it leaves the lost
// value, the command that did it, and the word forced-update. See
// docs/decision/0054.
func TestARewindLeavesTheLostValueInTheReflog(t *testing.T) {
	work, lost, _ := rewindLab(t, LogAllRefUpdates)

	entries := reflogOf(t, work)
	if len(entries) == 0 {
		t.Fatal("the ledger ref has no reflog, so the rewind left nothing behind")
	}
	if !strings.Contains(strings.Join(entries, "\n"), lost) {
		t.Fatalf("the lost value %s is not in the reflog:\n%s", lost, strings.Join(entries, "\n"))
	}

	// The reason git attaches is what turned an evening of archaeology into a
	// line of output, so it is asserted rather than assumed.
	var forced string
	for _, entry := range entries {
		if strings.Contains(entry, "forced-update") {
			forced = entry
		}
	}
	if forced == "" {
		t.Fatalf("no entry says the update was forced, so nothing records how the ledger moved:\n%s",
			strings.Join(entries, "\n"))
	}
	if !strings.Contains(forced, "fetch") {
		t.Errorf("the entry does not name the command that did it: %s", forced)
	}
}

// The break test for the one above, and the reason this work is worth doing at
// all. Everything is identical except core.logAllRefUpdates, which is `true` in
// every repository that has not run kept init — the value git writes itself.
//
// A test that passes but would also have passed before the change is worth
// nothing here, so this asserts the failure directly: the same fetch, the same
// stale remote, the same forced refspec, and afterwards the repository holds no
// record whatsoever that the ledger ever held another value. Set
// core.logAllRefUpdates to `always` in rewindLab below and this test fails on
// its first assertion, which is what makes the pair a measurement.
func TestAnUnloggedLedgerLosesTheValueWithNothingRecorded(t *testing.T) {
	work, lost, _ := rewindLab(t, "true")

	if entries := reflogOf(t, work); len(entries) != 0 {
		t.Fatalf("core.logAllRefUpdates=true logged refs/kept/, so this test measures nothing:\n%s",
			strings.Join(entries, "\n"))
	}

	// And the check has nothing to work from, which is *nothing has been
	// recorded* rather than a clean ledger. Collapsing those two is the mistake
	// one level up from collapsing unknown into not-kept.
	continuity, err := NewRefStore(work).Continuous()
	if err != nil {
		t.Fatal(err)
	}
	if continuity.Recorded {
		t.Error("Continuous claimed something was recorded in a repository with no reflog")
	}
	if continuity.Rewound() {
		t.Error("Continuous reported a rewind it cannot have measured")
	}
	if got := ledgerAt(t, work); got == lost {
		t.Fatal("the ledger did not rewind, so the premise of this test is gone")
	}
}

// What kept does with the reflog: the value the ledger held is named, and it is
// named as unreachable rather than merely absent.
func TestContinuousNamesWhatTheLedgerCanNoLongerReach(t *testing.T) {
	work, lost, _ := rewindLab(t, LogAllRefUpdates)

	continuity, err := NewRefStore(work).Continuous()
	if err != nil {
		t.Fatal(err)
	}
	if !continuity.Recorded {
		t.Fatal("nothing was recorded, so the reflog this test is about was not written")
	}
	if !continuity.Rewound() {
		t.Fatalf("Continuous did not report the rewind: %+v", continuity)
	}
	// Two ratifications were appended after the push, so the ledger held more
	// than one value origin never saw and the fetch took all of them. What is
	// reported is the head: recovering it recovers its ancestors, and listing
	// every intermediate value would bury the one a person has to act on.
	if len(continuity.Lost) != 1 || continuity.Lost[0] != lost {
		t.Fatalf("Continuous lost = %v, want exactly the value the ledger held [%s]", continuity.Lost, lost)
	}

	// The reduction has to be safe rather than merely short. Everything the
	// reflog recorded that the tip cannot reach must be reachable from what was
	// reported, or the refusal is naming a recovery that does not recover.
	head := continuity.Lost[0]
	var stranded int
	for _, entry := range reflogOf(t, work) {
		value := strings.Fields(entry)[0]
		if isAncestor(t, work, value, continuity.Held) {
			continue
		}
		stranded++
		if !isAncestor(t, work, value, head) {
			t.Errorf("%s is unreachable from the tip and from the reported head %s, so recovering the head would not restore it",
				value, head)
		}
	}
	if stranded < 2 {
		t.Fatalf("only %d recorded values were stranded, so the reduction was not exercised", stranded)
	}
	if continuity.Held != ledgerAt(t, work) {
		t.Errorf("Continuous held = %s, want the value the ref holds now %s", continuity.Held, ledgerAt(t, work))
	}
}

// The half that decides whether the guard survives contact with an operator. A
// guard that fires and never stops firing is a guard that gets switched off, so
// the correct repair — a union commit naming the lost value, which is what the
// 2026-08-22 repair actually used — has to make it go quiet on its own, with no
// acknowledgment file, no suppression list and no baseline to reset.
//
// And the incorrect repair must not. Discarding the lost records leaves them
// unreachable, and that is a state kept should keep refusing.
func TestTheRefusalClearsWhenTheLostValueIsUnionedBackAndNotOtherwise(t *testing.T) {
	work, lost, _ := rewindLab(t, LogAllRefUpdates)
	store := NewRefStore(work)

	// An append on the rewound base is what a tick would do next. It does not
	// repair anything, so the refusal must survive it.
	if err := store.Append("acts.jsonl", []byte(`{"act":"appended after the rewind"}`)); err != nil {
		t.Fatal(err)
	}
	if continuity, err := store.Continuous(); err != nil {
		t.Fatal(err)
	} else if !continuity.Rewound() {
		t.Fatal("a plain append cleared the refusal, so the lost records are being called recovered")
	}

	// The repair: one commit whose parents are both tips.
	tip := ledgerAt(t, work)
	tree := strings.TrimSpace(gitCapture(t, work, "rev-parse", tip+"^{tree}"))
	union := strings.TrimSpace(gitCapture(t, work,
		"-c", "user.name=kept", "-c", "user.email=kept@invalid",
		"commit-tree", tree, "-p", tip, "-p", lost, "-m", "kept ledger"))
	fetchGit(t, work, "update-ref", "-m", "kept ledger union repair", Ref, union)

	continuity, err := store.Continuous()
	if err != nil {
		t.Fatal(err)
	}
	if continuity.Rewound() {
		t.Fatalf("the union repair did not clear the refusal: %v is still unreachable", continuity.Lost)
	}
	if !continuity.Recorded {
		t.Error("the repair also erased the record that anything was ever measured")
	}
}

// The blind spot in reading only what git prints for each entry. `reflog show`
// reports the value each update moved the ref *to*, and the value it moved
// *from* survives only on the oldest entry — which is exactly the value at risk
// in a repository that already had a ledger when logging was switched on.
//
// That is not a corner: it is kept init on a live repository, followed by a
// rewind before kept next appends anything. Reading only the new values reports
// a clean ledger there, so this test builds precisely that sequence and asserts
// the lost value is still named.
func TestContinuousReadsThePreImageOfTheOldestEntry(t *testing.T) {
	_, work := fetchLab(t)
	store := NewRefStore(work)

	// A repository with a ledger, and no reflog for it: what every workspace
	// adopted before this change looks like.
	fetchGit(t, work, "config", logAllRefUpdatesKey, "true")
	if err := store.Append("acts.jsonl", []byte(`{"act":"ratified decision:0017"}`)); err != nil {
		t.Fatal(err)
	}
	lost := ledgerAt(t, work)
	if entries := reflogOf(t, work); len(entries) != 0 {
		t.Fatalf("the ledger was already logged, so this test does not measure adoption:\n%s", strings.Join(entries, "\n"))
	}

	// kept init. From here git records, and the very first thing it records is
	// the rewind.
	if _, _, log, err := ConfigureFetch(work); err != nil {
		t.Fatal(err)
	} else if log != LogEnabled {
		t.Fatalf("ConfigureFetch log = %q, want %q", log, LogEnabled)
	}
	empty := emptyLedger(t, work)
	fetchGit(t, work, "update-ref", "-m", "fetch origin: forced-update", Ref, empty)

	entries := reflogOf(t, work)
	if len(entries) != 1 {
		t.Fatalf("want exactly the rewind recorded, got %d entries:\n%s", len(entries), strings.Join(entries, "\n"))
	}
	if strings.Contains(entries[0], lost) {
		t.Fatal("the lost value is the entry's new value, so this test no longer measures the pre-image")
	}

	continuity, err := store.Continuous()
	if err != nil {
		t.Fatal(err)
	}
	if !continuity.Rewound() {
		t.Fatalf("the rewind was not reported: reading only each entry's new value misses it, which is the bug this test exists for (%+v)", continuity)
	}
	if len(continuity.Lost) != 1 || continuity.Lost[0] != lost {
		t.Fatalf("Continuous lost = %v, want [%s]", continuity.Lost, lost)
	}
}

// Nothing has answered is not an answer. A repository whose ledger ref has no
// reflog has recorded nothing, and that is neither a rewind nor a failure — it
// is the fresh-clone state, which doctor reports and nothing refuses.
func TestContinuousSaysNothingWasRecordedRatherThanNothingIsWrong(t *testing.T) {
	_, work := fetchLab(t)
	store := NewRefStore(work)
	fetchGit(t, work, "config", logAllRefUpdatesKey, "true")

	// No ledger at all.
	if continuity, err := store.Continuous(); err != nil || continuity.Recorded || continuity.Rewound() {
		t.Fatalf("Continuous with no ledger ref = %+v, %v", continuity, err)
	}

	// A ledger, and no reflog for it.
	if err := store.Append("acts.jsonl", []byte(`{"act":"first"}`)); err != nil {
		t.Fatal(err)
	}
	continuity, err := store.Continuous()
	if err != nil {
		t.Fatal(err)
	}
	if continuity.Recorded {
		t.Error("Continuous claimed a recording that does not exist")
	}
	if continuity.Held != ledgerAt(t, work) {
		t.Errorf("Continuous held = %q, want %s", continuity.Held, ledgerAt(t, work))
	}

	// A ledger kept in files, for a manuscript outside Git: no ref, so nothing
	// to say about one.
	if continuity, err := Continuous(NewFiles(t.TempDir())); err != nil || continuity.Recorded {
		t.Fatalf("Continuous on a file store = %+v, %v", continuity, err)
	}
}

// The other direction for the guard as a whole: every legitimate way a ledger
// moves has to pass, or the refusal is a permanent one and gets disabled.
// kept's own appends and an advance onto what a fetch brought — into a clone
// that has no ledger, and onto a remote that is ahead — are all fast-forwards,
// and all three are what this system does every day.
//
// The fetch no longer moves the ledger at all, which is docs/decision/0062: it
// writes the upstream ref, and Advance is what moves the durable record. That
// composition is not luck — a fast-forward's destination reaches every value
// the source did, so Continuous is a statement about the same DAG.
func TestContinuousStaysQuietForEveryLegitimateLedger(t *testing.T) {
	_, work := fetchLab(t)
	store := NewRefStore(work)
	if _, _, log, err := ConfigureFetch(work); err != nil || log != LogEnabled {
		t.Fatalf("ConfigureFetch = %q, %v", log, err)
	}

	quiet := func(what string) {
		t.Helper()
		continuity, err := store.Continuous()
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
		if continuity.Rewound() {
			t.Fatalf("%s was reported as a rewind: %v is unreachable from %s", what, continuity.Lost, continuity.Held)
		}
	}

	for _, line := range []string{`{"act":"first"}`, `{"act":"second"}`, `{"act":"third"}`} {
		if err := store.Append("acts.jsonl", []byte(line)); err != nil {
			t.Fatal(err)
		}
		quiet("an append")
	}
	full := ledgerAt(t, work)

	fetchGit(t, work, "push", "-q", "origin", Ref+":"+Ref)
	if err := store.Append("acts.jsonl", []byte(`{"act":"fourth"}`)); err != nil {
		t.Fatal(err)
	}
	fetchGit(t, work, "push", "-q", "origin", Ref+":"+Ref)
	ahead := ledgerAt(t, work)

	// A remote that is ahead: the fetch writes the upstream ref, Advance moves
	// the ledger onto it, and every value it held is still an ancestor.
	fetchGit(t, work, "update-ref", Ref, full)
	if code, out := fetchOrigin(t, work); code != 0 {
		t.Fatalf("fetch from a remote that is ahead exited %d:\n%s", code, out)
	}
	if got := ledgerAt(t, work); got != full {
		t.Fatalf("the fetch moved the ledger to %s; the network must not write it", got)
	}
	if what, _, err := Advance(store); err != nil || what != SyncAdvanced {
		t.Fatalf("Advance onto a remote that is ahead = %q, %v", what, err)
	}
	if got := ledgerAt(t, work); got != ahead {
		t.Fatalf("the ledger did not fast-forward: %s, want %s", got, ahead)
	}
	quiet("an advance onto a remote that is ahead")

	// A clone that has never seen the ledger: the ref is created, and the
	// reflog records the creation rather than a replacement.
	fetchGit(t, work, "update-ref", "-d", Ref)
	if code, out := fetchOrigin(t, work); code != 0 {
		t.Fatalf("fetch into a repository with no ledger ref exited %d:\n%s", code, out)
	}
	if what, _, err := Advance(store); err != nil || what != SyncAdvanced {
		t.Fatalf("Advance into a repository with no ledger = %q, %v", what, err)
	}
	quiet("a ledger advanced into a repository that had none")
}

// Logged is what kept doctor reads, and it reports the state of somebody else's
// config rather than a value kept wrote, so each state is put there by hand.
//
// The load-bearing case is `true` against `always`. Git logs a ref update only
// for a hardcoded set of prefixes and refs/kept/ is not among them; `always`
// lifts that and `true` does not, which is the single fact this whole mechanism
// rests on. It is asserted against git rather than argued: the value is set,
// the ref is moved twice, and the reflog is counted.
func TestLoggedReportsWhetherGitWillRecordTheLedger(t *testing.T) {
	_, work := fetchLab(t)
	store := NewRefStore(work)
	if err := store.Append("acts.jsonl", []byte(`{"act":"first"}`)); err != nil {
		t.Fatal(err)
	}
	first := ledgerAt(t, work)
	second := emptyLedger(t, work)

	for _, c := range []struct {
		value  string
		logged bool
	}{
		{"true", false},
		{"false", false},
		{"always", true},
		// Git reads the value case-insensitively, so a config written by hand
		// as Always is logging and must not be reported as a finding.
		{"Always", true},
		{"ALWAYS", true},
	} {
		// Deleting the ref takes its reflog with it, which is both how this
		// test returns to the unlogged state and one of the limits this whole
		// mechanism accepts: nothing here recovers a deleted ref's history.
		fetchGit(t, work, "config", logAllRefUpdatesKey, c.value)
		fetchGit(t, work, "update-ref", "-d", Ref)
		fetchGit(t, work, "update-ref", Ref, second)
		fetchGit(t, work, "update-ref", Ref, first)

		value, logged, err := Logged(work)
		if err != nil {
			t.Fatalf("%s=%s: %v", logAllRefUpdatesKey, c.value, err)
		}
		if logged != c.logged {
			t.Errorf("Logged with %s=%s = %v, want %v", logAllRefUpdatesKey, c.value, logged, c.logged)
		}
		// doctor prints this, so it has to be what is configured rather than
		// what kept would like to be configured.
		if value != c.value {
			t.Errorf("Logged reported the value %q, want %q", value, c.value)
		}
		// What git actually did, which is the claim rather than the setting.
		if recorded := len(reflogOf(t, work)) > 0; recorded != c.logged {
			t.Errorf("%s=%s: git recorded %v ledger updates, but Logged says %v",
				logAllRefUpdatesKey, c.value, recorded, logged)
		}
	}

	// A manuscript that is not in a repository has no setting to report on.
	if _, _, err := Logged(t.TempDir()); err == nil {
		t.Fatal("Logged outside a repository returned no error")
	}
}

// ConfigureFetch installs both settings and says which it wrote, because init
// reports each and a config kept rewrites without a word is the failure on the
// other side of leaving a trap armed. It is run again on every re-init, so the
// unchanged case has to write nothing.
func TestConfigureFetchTurnsLoggingOnAndSaysSo(t *testing.T) {
	_, work := fetchLab(t)
	fetchGit(t, work, "config", logAllRefUpdatesKey, "true")

	fetch, _, log, err := ConfigureFetch(work)
	if err != nil {
		t.Fatal(err)
	}
	if fetch != FetchAdded {
		t.Errorf("ConfigureFetch fetch = %q, want %q", fetch, FetchAdded)
	}
	if log != LogEnabled {
		t.Errorf("ConfigureFetch log = %q, want %q", log, LogEnabled)
	}
	if got := strings.TrimSpace(gitCapture(t, work, "config", "--get", logAllRefUpdatesKey)); got != LogAllRefUpdates {
		t.Errorf("%s = %q, want %q", logAllRefUpdatesKey, got, LogAllRefUpdates)
	}

	if _, _, log, err := ConfigureFetch(work); err != nil || log != LogUnchanged {
		t.Errorf("re-init log = %q, %v; want %q", log, err, LogUnchanged)
	}

	// An explicit false is overwritten, and that is the one value here kept did
	// not author. A repository that logs no ref update cannot record what the
	// ledger held, so adopting it without this would be adopting it into a
	// guarantee that cannot hold there. init says what it changed.
	fetchGit(t, work, "config", logAllRefUpdatesKey, "false")
	if _, _, log, err := ConfigureFetch(work); err != nil || log != LogEnabled {
		t.Errorf("ConfigureFetch over an explicit false = %q, %v; want %q", log, err, LogEnabled)
	}
}

// kept's own writes name themselves, so an entry that is not one stands out at
// a glance. This is the audit trail 0016 said the side ref did not have.
func TestKeptsOwnLedgerWritesSayWhatWroteThem(t *testing.T) {
	_, work := fetchLab(t)
	if _, _, _, err := ConfigureFetch(work); err != nil {
		t.Fatal(err)
	}
	store := NewRefStore(work)
	if err := store.Append("acts.jsonl", []byte(`{"act":"first"}`)); err != nil {
		t.Fatal(err)
	}
	if err := store.Put("artifacts/ab/cd", []byte("packet")); err != nil {
		t.Fatal(err)
	}

	entries := strings.Join(reflogOf(t, work), "\n")
	for _, want := range []string{"kept ledger/acts.jsonl", "kept artifacts/ab/cd"} {
		if !strings.Contains(entries, want) {
			t.Errorf("no reflog entry says %q:\n%s", want, entries)
		}
	}
}

// rewindLab reproduces 2026-08-22 under a chosen core.logAllRefUpdates: a
// ledger pushed to origin, two ratifications appended locally and never pushed,
// the forced refspec the repository actually carried, and one ordinary fetch.
// It returns the work repository and the value the ledger held before the
// fetch, which is the value that was lost.
//
// The setting is written before anything moves the ref, because it decides
// whether git creates the log at all. The forced direct spelling is added after
// ConfigureFetch, which now removes it — it is the only refspec that still
// moves the ledger ref backwards, which is what this lab needs and what
// docs/decision/0062 made unreachable through the configuration kept writes.
func rewindLab(t *testing.T, logging string) (work, lost, stale string) {
	t.Helper()
	_, work = fetchLab(t)
	fetchGit(t, work, "config", logAllRefUpdatesKey, logging)

	store := NewRefStore(work)
	if err := store.Append("acts.jsonl", []byte(`{"act":"pushed"}`)); err != nil {
		t.Fatal(err)
	}
	fetchGit(t, work, "push", "-q", "origin", Ref+":"+Ref)
	stale = remoteLedger(t, work)

	for _, line := range []string{`{"act":"ratified decision:0017"}`, `{"act":"ratified decision:0052"}`} {
		if err := store.Append("acts.jsonl", []byte(line)); err != nil {
			t.Fatal(err)
		}
	}
	lost = ledgerAt(t, work)
	if lost == stale {
		t.Fatalf("the ledger did not advance past what origin holds: %s", lost)
	}

	if _, _, _, err := ConfigureFetch(work); err != nil {
		t.Fatal(err)
	}
	fetchGit(t, work, "config", "--unset-all", "remote.origin.fetch")
	fetchGit(t, work, "config", "--add", "remote.origin.fetch", ForcedDirectRefspec)

	// ConfigureFetch now writes the logging setting itself, so the unlogged
	// case has to put back what kept left behind before this change — which is
	// git's own default, and is what every repository in existence carried on
	// 2026-08-22. Written again here rather than only at the top, and read back,
	// because a test measuring the wrong setting would report the wrong thing
	// twice: once as a pass, and once as evidence.
	fetchGit(t, work, "config", logAllRefUpdatesKey, logging)
	if got := strings.TrimSpace(gitCapture(t, work, "config", "--get", logAllRefUpdatesKey)); got != logging {
		t.Fatalf("%s = %q before the fetch, want %q", logAllRefUpdatesKey, got, logging)
	}

	if code, out := runGit(t, work, "fetch", "origin", "--prune", "--quiet"); code != 0 {
		t.Fatalf("the forced fetch exited %d, so it was refused and nothing was lost:\n%s", code, out)
	}
	if got := ledgerAt(t, work); got != stale {
		t.Fatalf("the ledger is at %s after the forced fetch, want the stale remote value %s", got, stale)
	}
	return work, lost, stale
}

// reflogOf is every entry git holds for the ledger ref, newest first, as the
// whole value it moved to and the reason git recorded. An empty slice is a ref
// git is not logging.
//
// The format is asked for rather than defaulted: `git reflog show` abbreviates
// the object name, so a test matching a full commit id against its output would
// pass or fail on how long git decided the prefix should be.
func reflogOf(t *testing.T, work string) []string {
	t.Helper()
	code, out := runGit(t, work, "reflog", "show", Ref, "--format=%H %gs")
	if code != 0 {
		return nil
	}
	var entries []string
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) != "" {
			entries = append(entries, line)
		}
	}
	return entries
}

// isAncestor asks git rather than reasoning about the shape of the history the
// test built, because what is being checked is a claim about reachability.
func isAncestor(t *testing.T, work, value, of string) bool {
	t.Helper()
	code, out := runGit(t, work, "merge-base", "--is-ancestor", value, of)
	switch code {
	case 0:
		return true
	case 1:
		return false
	}
	t.Fatalf("merge-base --is-ancestor exited %d:\n%s", code, out)
	return false
}

func gitCapture(t *testing.T, work string, args ...string) string {
	t.Helper()
	code, out := runGit(t, work, args...)
	if code != 0 {
		t.Fatalf("git %s exited %d:\n%s", strings.Join(args, " "), code, out)
	}
	return out
}

// emptyLedger is a commit with an empty tree and no parent — a value the ledger
// can be moved to that nothing in its history reaches, which is what a rewind
// onto an unrelated remote looks like from the ref's point of view.
func emptyLedger(t *testing.T, work string) string {
	t.Helper()
	tree := strings.TrimSpace(gitCapture(t, work, "hash-object", "-t", "tree", "-w", "--stdin"))
	return strings.TrimSpace(gitCapture(t, work,
		"-c", "user.name=kept", "-c", "user.email=kept@invalid",
		"commit-tree", tree, "-m", "kept ledger"))
}
