package main

import (
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/ledger"
)

// Exit 4, and the first thing to reach it. 0027 reserved the code for a run
// that refused to answer — "a stale view, an unverified provider, a budget" —
// and said it was named rather than left as a hole because the point of a
// reserved code is that nothing else takes it.
//
// A ledger whose ref has held a value it can no longer reach is the archetype
// of the stale view: kept cannot say what is ratified, because the record it
// would answer from is known to be incomplete. It is not exit 1, which is
// drift — nothing here is a promise that stopped being kept — and it is not
// exit 3, which is a question kept put and could not settle. The question was
// never properly put.
//
// This is the 2026-08-22 event at the command level: acts appended, the ref
// moved backwards over them by something that is not kept, and the next kept
// run. See docs/decision/0054.
func TestKeptRefusesWhenTheLedgerRefHasGoneBackwards(t *testing.T) {
	w, lost := rewoundWorkspace(t)

	// Every verb that reads or writes the ledger refuses, and says the same
	// thing, because they are all answering out of the same incomplete record.
	for _, verb := range [][]string{
		{"status"},
		{"tick"},
		{"drift"},
		{"why", "promise:one"},
		{"ratify", "promise:two", "--by", "the operator"},
		{"release", "promise:one", "--by", "the operator"},
	} {
		code, _, stderr := w.run(verb...)
		if code != exitRefused {
			t.Errorf("kept %s exited %d, want %d:\n%s", strings.Join(verb, " "), code, exitRefused, stderr)
			continue
		}
		mustContain(t, "the refusal from kept "+strings.Join(verb, " "), stderr,
			ledger.Ref+" has gone backwards",
			lost[:7],
			"git reflog show "+ledger.Ref,
			"union",
		)
	}

	// And it says what the guard does not cover, where the person reading it is
	// already standing. A reflog entry is what keeps the lost commit alive, so
	// the recovery window is the entry's lifetime and not forever.
	_, _, stderr := w.run("status")
	mustContain(t, "the refusal", stderr, "gc.reflogExpire", "90 days")

	// The two verbs that read the corpus keep working, because they are what an
	// operator has left in this state. Refusing them would take away the tools
	// for the diagnosis at the moment it is needed.
	for _, verb := range []string{"list", "doctor"} {
		code, stdout, stderr := w.run(verb)
		if code == exitRefused {
			t.Errorf("kept %s refused, and it reads the corpus rather than the ledger:\n%s", verb, stderr)
		}
		if !strings.Contains(stdout, "artifact") {
			t.Errorf("kept %s printed nothing useful about the corpus:\n%s", verb, stdout)
		}
	}
}

// The other direction, which is the whole reason the check is safe to have on
// by default. Every ordinary ledger — one kept has only ever appended to — must
// pass, or kept refuses to answer anything in every workspace and the guard is
// removed within a day.
func TestAnOrdinaryLedgerIsNeverRefused(t *testing.T) {
	git := gitOrSkip(t)
	w := newWorkspace(t, map[string]string{"one": "a promise", "two": "another"})
	git(w.root, "init", "-q")
	if _, _, err := ledger.ConfigureFetch(w.root); err != nil {
		t.Fatal(err)
	}

	for _, id := range []string{"promise:one", "promise:two"} {
		if code, _, stderr := w.run("ratify", id, "--by", "the operator"); code != exitOK {
			t.Fatalf("ratify %s exited %d: %s", id, code, stderr)
		}
	}
	for _, verb := range []string{"status", "tick", "drift"} {
		if code, _, stderr := w.run(verb); code == exitRefused {
			t.Fatalf("kept %s refused an ordinary ledger:\n%s", verb, stderr)
		}
	}

	// The premise, checked rather than assumed: this workspace really is being
	// recorded, so the absence of a refusal means the check ran and passed
	// rather than that it had nothing to read.
	continuity, err := ledger.Continuous(ledger.ForWorkspace(w.root))
	if err != nil {
		t.Fatal(err)
	}
	if !continuity.Recorded {
		t.Fatal("nothing was recorded, so these verbs passed without the check measuring anything")
	}
}

// doctor is what tells an operator the reflog is missing, and it is the same
// 0050 shape as the forced refspec beside it: nothing refuses an unlogged
// ledger, git accepts it, kept accepts it, the corpus loads; no other kept verb
// reads the setting; and kept init is the adoption verb, so an operator who
// never re-inits is never told.
//
// A finding and never an error, so 0027's exit codes are unchanged.
func TestDoctorReportsALedgerRefGitKeepsNoReflogFor(t *testing.T) {
	git := gitOrSkip(t)
	w := newWorkspace(t, map[string]string{"one": "a promise"})
	git(w.root, "init", "-q")
	git(w.root, "config", "core.logAllRefUpdates", "true")

	code, stdout, _ := w.run("doctor")
	if !strings.Contains(stdout, "nothing records what the ledger ref has held") {
		t.Fatalf("doctor did not report the unlogged ledger:\n%s", stdout)
	}
	mustContain(t, "the finding", stdout,
		"core.logAllRefUpdates",
		ledger.LogAllRefUpdates,
		"kept init",
	)
	// The limits, where the person deciding whether to act on it is reading.
	mustContain(t, "the finding", stdout,
		"deleting the ref deletes its",
		"gc.reflogExpire",
		"clone",
	)
	if code != exitOK {
		t.Errorf("the finding changed doctor's exit code: %d", code)
	}

	// kept init is what it says corrects it, so that has to be true.
	if _, log, err := ledger.ConfigureFetch(w.root); err != nil || log != ledger.LogEnabled {
		t.Fatalf("ConfigureFetch = %q, %v", log, err)
	}
	if _, stdout, _ := w.run("doctor"); strings.Contains(stdout, "nothing records what the ledger ref has held") {
		t.Fatalf("the finding survived the correction:\n%s", stdout)
	}
}

// A workspace that is not a repository has no ref and no setting, and doctor is
// a verb a manuscript author runs. Silence, not an error and not a finding.
func TestDoctorSaysNothingAboutTheReflogOutsideARepository(t *testing.T) {
	w := newWorkspace(t, map[string]string{"one": "a promise"})
	code, stdout, stderr := w.run("doctor")
	if code != exitOK {
		t.Fatalf("doctor exited %d: %s", code, stderr)
	}
	if strings.Contains(stdout, "logAllRefUpdates") || strings.Contains(stderr, "logAllRefUpdates") {
		t.Fatalf("a directory that is not a repository was reported:\n%s\n%s", stdout, stderr)
	}
}

// rewoundWorkspace is a workspace whose ledger ref has been moved backwards
// over a ratification by something that is not kept — which is what a forced
// fetch, a push into the repository or a stray update-ref all look like from
// the ref's point of view. It returns the workspace and the value that was
// lost.
func rewoundWorkspace(t *testing.T) (*workspace, string) {
	t.Helper()
	git := gitOrSkip(t)
	w := newWorkspace(t, map[string]string{"one": "a promise", "two": "another"})
	git(w.root, "init", "-q")

	// Adoption, which is what turns the reflog on. Without it there is nothing
	// to detect the rewind with, and this test would measure nothing.
	if _, log, err := ledger.ConfigureFetch(w.root); err != nil {
		t.Fatal(err)
	} else if log != ledger.LogEnabled {
		t.Fatalf("ConfigureFetch log = %q, want %q", log, ledger.LogEnabled)
	}

	if code, _, stderr := w.run("ratify", "promise:one", "--by", "the operator"); code != exitOK {
		t.Fatalf("ratify exited %d: %s", code, stderr)
	}
	behind := strings.TrimSpace(gitOut(t, git, w.root, "rev-parse", ledger.Ref))

	if code, _, stderr := w.run("ratify", "promise:two", "--by", "the operator"); code != exitOK {
		t.Fatalf("ratify exited %d: %s", code, stderr)
	}
	lost := strings.TrimSpace(gitOut(t, git, w.root, "rev-parse", ledger.Ref))
	if lost == behind {
		t.Fatal("the second ratification did not move the ledger ref")
	}

	git(w.root, "update-ref", ledger.Ref, behind)
	return w, lost
}
