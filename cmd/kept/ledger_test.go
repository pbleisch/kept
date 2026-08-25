package main

import (
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/ledger"
)

// clones is two workspaces over one bare origin, wired the way kept init wires
// one. It is the shape the whole of docs/decision/0062 is about, and it is built
// out of two real repositories rather than by moving refs by hand: a test that
// simulated the second clone with `update-ref` backwards would be reproducing
// 0054's rewind instead, and would be refused for the right reason at the wrong
// moment.
func clones(t *testing.T, promises map[string]string) (*workspace, *workspace, func(dir string, args ...string)) {
	t.Helper()
	git := gitOrSkip(t)
	origin := t.TempDir()
	git(origin, "init", "-q", "--bare")

	adopt := func() *workspace {
		w := newWorkspace(t, promises)
		git(w.root, "init", "-q")
		git(w.root, "remote", "add", "--no-tags", "origin", origin)
		if code, _, stderr := w.run("init"); code != exitOK {
			t.Fatalf("kept init exited %d: %s", code, stderr)
		}
		return w
	}
	return adopt(), adopt(), git
}

// The two refs, and which one each verb is allowed to touch. This is the whole
// shape of docs/decision/0062 seen from the command line.
func TestOnlyAWritingVerbAdvancesTheLedger(t *testing.T) {
	here, there, git := clones(t, map[string]string{"one": "a promise"})

	// The other clone ratifies and pushes. This one has fetched nothing yet.
	if code, _, stderr := there.run("ratify", "promise:one", "--by", "the operator"); code != exitOK {
		t.Fatalf("ratify exited %d: %s", code, stderr)
	}
	git(there.root, "push", "-q", "origin", ledger.Ref+":"+ledger.Ref)
	full := gitRef(t, there.root, ledger.Ref)

	// An ordinary fetch. It writes the upstream ref and never the ledger.
	git(here.root, "fetch", "origin")
	if got := gitRef(t, here.root, ledger.Ref); got != "" {
		t.Fatalf("the fetch wrote %s: %s", ledger.Ref, got)
	}
	if got := gitRef(t, here.root, ledger.UpstreamRef); got != full {
		t.Fatalf("%s = %q after a fetch, want %q", ledger.UpstreamRef, got, full)
	}

	// A read verb answers, and leaves the ledger exactly where it found it.
	// This is what makes kept status runnable against a read-only checkout.
	// "never ratified" rather than "unratified": the register says which of the
	// two unratified cases this is, and here it is the one meaning no act in
	// this repository's ledger names this artifact. See docs/decision/0066.
	if line := here.line("promise:one"); !strings.Contains(line, "never ratified") {
		t.Fatalf("status read standing the ledger does not hold: %q", line)
	}
	if got := gitRef(t, here.root, ledger.Ref); got != "" {
		t.Fatalf("a read verb advanced the ledger to %s", got)
	}

	// The verb that exists for it does, and says how far it moved.
	code, stdout, stderr := here.run("ledger", "sync")
	if code != exitOK {
		t.Fatalf("ledger sync exited %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, "advanced") {
		t.Fatalf("ledger sync did not say it advanced:\n%s", stdout)
	}
	if got := gitRef(t, here.root, ledger.Ref); got != full {
		t.Fatalf("%s = %q after ledger sync, want %q", ledger.Ref, got, full)
	}
	if line := here.line("promise:one"); !strings.Contains(line, " ratified") {
		t.Fatalf("the advanced ledger does not carry the grant: %q", line)
	}

	// Idempotent, and it says so rather than pretending it did something.
	if code, stdout, _ := here.run("ledger", "sync"); code != exitOK || strings.Contains(stdout, "advanced") {
		t.Fatalf("a second ledger sync exited %d and said:\n%s", code, stdout)
	}
}

// The reason a writing verb advances at all, and it is not convenience.
// Appending onto a ledger that is behind forks it, so a clone that fetched and
// then ticked would manufacture a divergence out of nothing. Git makes you
// merge before you can push for the same reason.
func TestAWritingVerbAdvancesBeforeItAppends(t *testing.T) {
	here, there, git := clones(t, map[string]string{"one": "a promise", "two": "another promise"})

	if code, _, stderr := there.run("ratify", "promise:one", "--by", "the operator"); code != exitOK {
		t.Fatalf("ratify exited %d: %s", code, stderr)
	}
	git(there.root, "push", "-q", "origin", ledger.Ref+":"+ledger.Ref)
	full := gitRef(t, there.root, ledger.Ref)
	git(here.root, "fetch", "origin")

	// The act this clone appends must build on what origin holds rather than
	// beside it.
	code, _, stderr := here.run("ratify", "promise:two", "--by", "the operator")
	if code != exitOK {
		t.Fatalf("ratify exited %d: %s", code, stderr)
	}
	if !strings.Contains(stderr, "advanced") {
		t.Fatalf("the writing verb did not say it advanced the ledger:\n%s", stderr)
	}
	if !isAncestorOf(t, here.root, full, gitRef(t, here.root, ledger.Ref)) {
		t.Fatalf("the act was appended beside %s rather than onto it, which is a fork", full)
	}
	for _, id := range []string{"promise:one", "promise:two"} {
		if line := here.line(id); !strings.Contains(line, " ratified") {
			t.Fatalf("%s is not ratified after the advance: %q", id, line)
		}
	}
}

// A fork, which is two clones that both appended without either having seen the
// other. The verb refuses it, exits 4, and says what a person has to do — and
// every other verb keeps answering, because the records on this side are all
// still here.
func TestLedgerSyncRefusesAForkAndNothingElseIsBlocked(t *testing.T) {
	here, there, git := clones(t, map[string]string{"one": "a promise", "two": "another promise"})

	if code, _, stderr := there.run("ratify", "promise:one", "--by", "the operator"); code != exitOK {
		t.Fatalf("ratify exited %d: %s", code, stderr)
	}
	git(there.root, "push", "-q", "origin", ledger.Ref+":"+ledger.Ref)

	if code, _, stderr := here.run("ratify", "promise:two", "--by", "the operator"); code != exitOK {
		t.Fatalf("ratify exited %d: %s", code, stderr)
	}
	ours := gitRef(t, here.root, ledger.Ref)
	git(here.root, "fetch", "origin")

	code, _, stderr := here.run("ledger", "sync")
	if code != exitRefused {
		t.Fatalf("ledger sync on a fork exited %d, want %d:\n%s", code, exitRefused, stderr)
	}
	for _, want := range []string{"both moved", "union", "git merge-base"} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("the refusal does not say %q:\n%s", want, stderr)
		}
	}
	if got := gitRef(t, here.root, ledger.Ref); got != ours {
		t.Fatalf("the refused sync moved the ledger to %s, want %s", got, ours)
	}

	// Nothing else is blocked, and a writing verb still records what it found:
	// a run that could not write because another clone had also written would
	// be a worse failure than a fold missing that clone's acts.
	if line := here.line("promise:two"); !strings.Contains(line, " ratified") {
		t.Fatalf("status stopped answering on a forked ledger: %q", line)
	}
	code, _, stderr = here.run("release", "promise:two", "--by", "the operator")
	if code == exitRefused {
		t.Fatalf("a writing verb refused on a forked ledger:\n%s", stderr)
	}
	if !strings.Contains(stderr, "both moved") {
		t.Fatalf("the writing verb did not warn about the fork:\n%s", stderr)
	}
}

// doctor reports both states offline, which docs/decision/0016 said was
// impossible — "the only way to know is to ask the remote". Nothing here
// contacts one.
func TestDoctorReportsDivergenceWithNoNetwork(t *testing.T) {
	const behind, forked = "behind what origin", "have both moved"
	here, there, git := clones(t, map[string]string{"one": "a promise", "two": "another promise", "three": "a third promise"})

	if _, stdout, _ := here.run("doctor"); strings.Contains(stdout, behind) {
		t.Fatalf("a repository nobody has fetched into is reported:\n%s", stdout)
	}

	if code, _, stderr := there.run("ratify", "promise:one", "--by", "the operator"); code != exitOK {
		t.Fatalf("ratify exited %d: %s", code, stderr)
	}
	git(there.root, "push", "-q", "origin", ledger.Ref+":"+ledger.Ref)

	// Level: nothing to say. The push updated the other clone's upstream ref,
	// which is git updating a remote-tracking ref for a ref it just pushed.
	if _, stdout, _ := there.run("doctor"); strings.Contains(stdout, behind) || strings.Contains(stdout, forked) {
		t.Fatalf("a level ledger is reported:\n%s", stdout)
	}

	// Ahead, which is the resting state of this system and is not a finding.
	if code, _, stderr := there.run("ratify", "promise:two", "--by", "the operator"); code != exitOK {
		t.Fatalf("ratify exited %d: %s", code, stderr)
	}
	if _, stdout, _ := there.run("doctor"); strings.Contains(stdout, behind) || strings.Contains(stdout, forked) {
		t.Fatalf("a ledger that is merely ahead is reported:\n%s", stdout)
	}

	// Behind: reported, with the verb that fixes it.
	git(here.root, "fetch", "origin")
	code, stdout, _ := here.run("doctor")
	if !strings.Contains(stdout, behind) || !strings.Contains(stdout, "kept ledger sync") {
		t.Fatalf("doctor did not report a ledger that is behind:\n%s", stdout)
	}
	if code != exitOK {
		t.Errorf("the finding changed doctor's exit code: %d", code)
	}

	// Forked, in the order it actually happens: this clone appends — advancing
	// onto what it had fetched, so nothing forks yet — the other clone appends
	// and pushes, and only then does this one fetch. A fork is what a fetch
	// discovers rather than what a write creates, which is the design working.
	if code, _, stderr := here.run("ratify", "promise:three", "--by", "the operator"); code != exitOK {
		t.Fatalf("ratify exited %d: %s", code, stderr)
	}
	if code, _, stderr := there.run("release", "promise:one", "--by", "the operator"); code != exitOK {
		t.Fatalf("release exited %d: %s", code, stderr)
	}
	git(there.root, "push", "-q", "origin", ledger.Ref+":"+ledger.Ref)
	git(here.root, "fetch", "origin")
	code, stdout, _ = here.run("doctor")
	if !strings.Contains(stdout, forked) || !strings.Contains(stdout, "union") {
		t.Fatalf("doctor did not report a forked ledger:\n%s", stdout)
	}
	if code != exitOK {
		t.Errorf("the finding changed doctor's exit code: %d", code)
	}
}

// A manuscript outside git has no refs, and a repository with no origin has
// nothing to have fetched. Silence in both, because a finding nobody can act on
// is noise.
func TestNoUpstreamRefIsSilenceRatherThanAFinding(t *testing.T) {
	w := newWorkspace(t, map[string]string{"one": "a promise"})
	code, stdout, stderr := w.run("doctor")
	if code != exitOK || strings.Contains(stdout, "behind what origin") || strings.Contains(stdout, "have both moved") {
		t.Fatalf("doctor exited %d outside a repository:\n%s\n%s", code, stdout, stderr)
	}
	if code, stdout, stderr := w.run("ledger", "sync"); code != exitOK || strings.Contains(stdout, "advanced") {
		t.Fatalf("ledger sync exited %d outside a repository:\n%s\n%s", code, stdout, stderr)
	}
}

// The verb's own surface: one subcommand, and anything else is a usage error
// rather than a silent no-op.
func TestLedgerTakesOneSubcommand(t *testing.T) {
	w := newWorkspace(t, map[string]string{"one": "a promise"})
	for _, args := range [][]string{{"ledger"}, {"ledger", "push"}, {"ledger", "sync", "extra"}} {
		if code, _, stderr := w.run(args...); code != exitUsage {
			t.Errorf("kept %s exited %d, want %d:\n%s", strings.Join(args, " "), code, exitUsage, stderr)
		}
	}
	if code, stdout, _ := w.run("ledger", "help"); code != exitOK || !strings.Contains(stdout, "sync") {
		t.Errorf("kept ledger help exited %d:\n%s", code, stdout)
	}
}

func gitRef(t *testing.T, dir, ref string) string {
	t.Helper()
	return strings.TrimSpace(gitOut(t, nil, dir, "for-each-ref", "--format=%(objectname)", ref))
}

func isAncestorOf(t *testing.T, dir, value, of string) bool {
	t.Helper()
	return strings.TrimSpace(gitOut(t, nil, dir, "rev-list", "--count", of+".."+value)) == "0"
}
