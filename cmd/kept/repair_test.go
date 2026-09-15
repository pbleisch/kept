package main

import (
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/box"
	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/repair"
)

// TestRepairIsAVerbRatherThanAReasonItCannotBeOne is the smallest statement that
// increment 5 is reachable: the verb that used to answer "blocked on a box to
// work in" now runs, and the reason it gave is gone from the tree.
func TestRepairIsAVerbRatherThanAReasonItCannotBeOne(t *testing.T) {
	if why, listed := unbuilt["repair"]; listed {
		t.Fatalf("repair is still listed as unbuilt: %s", why)
	}
	code, stdout, _ := kept(t, "help")
	if code != exitOK {
		t.Fatalf("kept help exited %d", code)
	}
	if !strings.Contains(stdout, "kept repair") {
		t.Fatal("the usage does not say how to reach a repair")
	}
	if strings.Contains(stdout, "repair is blocked") {
		t.Fatal("the usage still says repair is blocked on a box, and there is one")
	}
}

// TestRepairWithNoPromiseListsWhatItCouldBePointedAt is the answer to several
// drifted promises: it shows them and makes the choice a person's.
//
// Only a promise that reads not-kept is listed. An answer that is stale,
// unverified or unknown says nobody has measured, and the answer to that is a
// tick rather than an agent — dispatching a live session on a reading nothing
// refreshed would spend one to discover the promise was fine.
func TestRepairWithNoPromiseListsWhatItCouldBePointedAt(t *testing.T) {
	w := newWorkspace(t, nil)
	w.put("docs/promise/counted.md", corpusPromise("counted", "obs.c.artifacts == 99"))
	w.put("docs/promise/holds.md", corpusPromise("holds", "obs.c.ok"))

	if code, _, stderr := w.run("ratify", "promise:counted", "--by", "the operator"); code != exitOK {
		t.Fatalf("ratify exited %d: %s", code, stderr)
	}
	if code, _, stderr := w.run("ratify", "promise:holds", "--by", "the operator"); code != exitOK {
		t.Fatalf("ratify exited %d: %s", code, stderr)
	}

	// Nothing has measured anything yet, so nothing is repairable and the verb
	// says which verb is.
	code, stdout, _ := w.run("repair")
	if code != exitOK {
		t.Fatalf("kept repair with nothing measured exited %d:\n%s", code, stdout)
	}
	if !strings.Contains(stdout, "wants a tick, not an agent") {
		t.Fatalf("the empty listing does not say what to do instead:\n%s", stdout)
	}

	if code, stdout, stderr := w.run("tick"); code != exitDrift {
		t.Fatalf("the tick exited %d, want drift:\n%s\n%s", code, stdout, stderr)
	}

	code, stdout, _ = w.run("repair")
	if code != exitUsage {
		t.Fatalf("kept repair with a choice to make exited %d, want %d:\n%s", code, exitUsage, stdout)
	}
	if !strings.Contains(stdout, "promise:counted") {
		t.Fatalf("the promise that is not kept is not listed:\n%s", stdout)
	}
	if strings.Contains(stdout, "promise:holds") {
		t.Fatalf("a promise that is kept was offered to an agent:\n%s", stdout)
	}
	if !strings.Contains(stdout, "One promise per run") {
		t.Fatalf("the listing does not say it repairs one rather than several:\n%s", stdout)
	}
}

// TestRepairRefusesWithItsConditionNamed is the refusal at the command line: it
// is not a placeholder, it names which condition stopped the dispatch, and it
// exits on drift rather than on a usage error, because it refused to act on
// something it had already established.
func TestRepairRefusesWithItsConditionNamed(t *testing.T) {
	w := newWorkspace(t, nil)
	w.put("docs/promise/counted.md", corpusPromise("counted", "obs.c.artifacts == 99"))
	if code, _, stderr := w.run("ratify", "promise:counted", "--by", "the operator"); code != exitOK {
		t.Fatalf("ratify exited %d: %s", code, stderr)
	}

	code, stdout, stderr := w.run("repair", "promise:counted")
	if code != exitDrift {
		t.Fatalf("a refused repair exited %d, want %d:\n%s\n%s", code, exitDrift, stdout, stderr)
	}
	mustContain(t, "the refusal", stdout,
		"refused · provider",
		"unset is not none",
	)

	// And it is answerable afterwards, which is the whole of why it was written
	// down: a refusal cannot be re-derived.
	code, stdout, stderr = w.run("why", "promise:counted")
	if code == exitUsage {
		t.Fatalf("why exited %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, "provider") {
		t.Fatalf("kept why does not report the refusal:\n%s", stdout)
	}
}

// TestRepairRefusesAPromiseThatAuthorizesNothing keeps the two families apart. A
// promise nobody ratified and one bounding no scope are usage errors: nothing
// about the machine or the moment would change them, so nothing is recorded and
// the exit code says the request was wrong rather than that drift persists.
func TestRepairRefusesAPromiseThatAuthorizesNothing(t *testing.T) {
	w := newWorkspace(t, nil)
	w.put("docs/promise/counted.md", corpusPromise("counted", "obs.c.artifacts == 99"))

	code, _, stderr := w.run("repair", "promise:counted")
	if code != exitUsage {
		t.Fatalf("repairing an unratified promise exited %d, want %d: %s", code, exitUsage, stderr)
	}
	if !strings.Contains(stderr, "nothing is owed until a person ratifies") {
		t.Fatalf("the refusal does not say why: %s", stderr)
	}

	code, _, stderr = w.run("repair", "promise:nowhere")
	if code != exitUsage {
		t.Fatalf("repairing an artifact nobody wrote exited %d, want %d: %s", code, exitUsage, stderr)
	}
}

// TestARepairThatReachedPastItsBoundIsNotAPass is the exit code half of decision
// 0059: the promise may well hold in that box, and nothing that reached outside
// the scope it bound may land, so the run does not report success.
func TestARepairThatReachedPastItsBoundIsNotAPass(t *testing.T) {
	inside := box.Change{Paths: []string{"internal/gather/source.go"}}
	outside := box.Change{Paths: []string{"internal/gather/source.go", "cmd/kept/main.go"}}
	scope := []string{"internal/gather/**/*.go"}

	bounded := &repair.Result{Status: repair.Repaired, Review: inside.Against(scope)}
	if got := repairAnswer(bounded); got != exitOK {
		t.Fatalf("a bounded repair exited %d, want %d", got, exitOK)
	}
	unbounded := &repair.Result{Status: repair.Repaired, Review: outside.Against(scope)}
	if got := repairAnswer(unbounded); got != exitDrift {
		t.Fatalf("a repair that reached outside its scope exited %d, want %d", got, exitDrift)
	}

	for _, c := range []struct {
		status repair.Status
		want   int
	}{
		{repair.Unrepaired, exitDrift},
		{repair.Refused, exitDrift},
		{repair.Undecided, exitUnknown},
		{repair.Unmeasured, exitUnknown},
		{repair.Holds, exitOK},
		{repair.Prepared, exitOK},
	} {
		if got := repairAnswer(&repair.Result{Status: c.status}); got != c.want {
			t.Fatalf("%s exited %d, want %d", c.status, got, c.want)
		}
	}
}

// TestOnlyABrokenPromiseIsOfferedToAnAgent holds the listing to the kinds an
// agent can address. A packet states a gap, and only these two are one.
func TestOnlyABrokenPromiseIsOfferedToAnAgent(t *testing.T) {
	repairable := map[model.DriftKind]bool{model.DriftNotKept: true, model.DriftRegressed: true}
	for _, kind := range []model.DriftKind{
		model.DriftNotKept,
		model.DriftRegressed,
		model.DriftStale,
		model.DriftUnverified,
		model.DriftUnknown,
		model.DriftAbsent,
		model.DriftConflicted,
	} {
		if repairableKind(kind) != repairable[kind] {
			t.Fatalf("%s is offered to an agent: %v, want %v", kind, repairableKind(kind), repairable[kind])
		}
	}
}

// corpusPromise is a promise measured by `corpus`, which reads a corpus root and
// shells nothing out — so it answers the same on every machine and needs no
// toolchain installed for a command-level test to reach a verdict.
func corpusPromise(slug, hold string) string {
	return "---\nid: " + slug + "\ntitle: " + slug + `
fields:
  statement: the corpus is what it was
  gather:
    c:
      gatherer: corpus
      scope: ["docs/**"]
  hold_when: ` + hold + `
  trigger: cadence
  repair:
    scope: ["docs/**"]
---

Why: because a promise nobody can check is not one.
`
}

// TestAnAttemptsBoundLooserThanTheRepositorysIsRefusedBeforeAnythingIsCut is the
// command line's half of docs/decision/0086's budget: a person who asks for more
// attempts than the repository allows is told so, by name, before a box exists.
func TestAnAttemptsBoundLooserThanTheRepositorysIsRefusedBeforeAnythingIsCut(t *testing.T) {
	w := newWorkspace(t, nil)
	w.put("docs/promise/counted.md", corpusPromise("counted", "obs.c.artifacts == 99"))
	if code, _, stderr := w.run("ratify", "promise:counted", "--by", "the operator"); code != exitOK {
		t.Fatalf("ratify exited %d: %s", code, stderr)
	}

	code, stdout, stderr := w.run("repair", "promise:counted", "--attempts", "9")
	if code != exitUsage {
		t.Fatalf("--attempts above the bound exited %d, want %d:\n%s\n%s", code, exitUsage, stdout, stderr)
	}
	mustContain(t, "the refusal", stderr, "--attempts 9 is looser than the bound of 3 from the default")
	if strings.Contains(stdout, "box cut at") || strings.Contains(stdout, "refused · provider") {
		t.Fatalf("a refused bound still reached the repair:\n%s", stdout)
	}

	w.put(".kept/config.yaml", "corpus: docs\nrepair:\n  attempts: 2\n")
	code, _, stderr = w.run("repair", "promise:counted", "--attempts", "3")
	if code != exitUsage || !strings.Contains(stderr, "the bound of 2 from .kept/config.yaml") {
		t.Fatalf("--attempts above a configured bound exited %d: %s", code, stderr)
	}

	// A stricter one is accepted, and the repair goes on to refuse on its own
	// condition as it did before the flag existed.
	code, stdout, _ = w.run("repair", "promise:counted", "--attempts", "1", "--no-auto-rework")
	if code != exitDrift || !strings.Contains(stdout, "refused · provider") {
		t.Fatalf("a stricter bound exited %d:\n%s", code, stdout)
	}
}
