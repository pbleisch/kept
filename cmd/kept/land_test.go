package main

import (
	"strings"
	"testing"
)

// TestLandIsNoLongerDeferred is the acceptance line: typing `kept land` gets an
// answer about a land rather than a reason it does not exist.
func TestLandIsNoLongerDeferred(t *testing.T) {
	if why, deferred := unbuilt["land"]; deferred {
		t.Fatalf("land is implemented and still reported as deferred: %q", why)
	}
	// The reason it used to carry was that no thread produced a landable diff.
	// Nothing may still say that, in any verb's reason.
	for verb, why := range unbuilt {
		if strings.Contains(why, "nothing to land") || strings.Contains(why, "no thread produces one") {
			t.Errorf("%s still says nothing produces a landable diff: %q", verb, why)
		}
	}
}

// TestLandWithNoThreadListsWhatCouldBeLanded: with nothing named, a land says
// what it could be pointed at rather than guessing one, which is the same shape
// `kept repair` takes and for the same reason.
func TestLandWithNoThreadListsWhatCouldBeLanded(t *testing.T) {
	w := newWorkspace(t, map[string]string{"first": "the first promise"})

	code, stdout, stderr := w.run("land")
	if code != exitOK {
		t.Fatalf("land exited %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, "nothing to land") {
		t.Fatalf("land says %q", stdout)
	}
	// It points at the verb that makes one, rather than leaving a dead end.
	if !strings.Contains(stdout, "kept repair") {
		t.Fatalf("land does not say what produces something to land: %q", stdout)
	}
}

// TestLandTakesOneThread: several threads and one target is a queue, and this
// slice does not have one.
func TestLandTakesOneThread(t *testing.T) {
	w := newWorkspace(t, map[string]string{"first": "the first promise"})

	code, _, stderr := w.run("land", "one", "two")
	if code != exitUsage {
		t.Fatalf("land with two threads exited %d", code)
	}
	if !strings.Contains(stderr, "one thread") {
		t.Fatalf("the refusal does not say why: %q", stderr)
	}
}

// TestLandSaysWhereItIsGoingBeforeItActs is docs/decision/0015's "every land
// prints the target it resolved before it acts on it", and the arrival beside
// it.
//
// The land here has nothing to land, so it refuses — and the plan is on stdout
// anyway, which is the assertion: the resolution is printed before the gate
// runs rather than reported afterwards as part of the outcome.
func TestLandSaysWhereItIsGoingBeforeItActs(t *testing.T) {
	w := newWorkspace(t, map[string]string{"first": "the first promise"})
	git := w.asRepository(t)
	_ = git

	code, stdout, _ := w.run("land", "no-such-thread", "--by", "Test <test@example.invalid>")
	if code != exitUsage {
		t.Fatalf("a land of a thread that anchored nothing exited %d", code)
	}
	// No remote and no upstream, so the branch this tree is standing on is
	// where this operator's work goes, and it is a local target.
	if !strings.Contains(stdout, "landing thread no-such-thread on main") {
		t.Fatalf("the land did not say where it was going: %q", stdout)
	}
	if !strings.Contains(stdout, "local regime, ref arrival") {
		t.Fatalf("the land did not say how the change would arrive: %q", stdout)
	}
}

// TestLandRefusesAnArrivalTheTargetDoesNotAdmit: pinning an impossible arrival
// is refused by name rather than quietly satisfied by the other, and it is
// refused before anything is gated — so nothing is printed about a land that
// was never going to happen.
func TestLandRefusesAnArrivalTheTargetDoesNotAdmit(t *testing.T) {
	w := newWorkspace(t, map[string]string{"first": "the first promise"})
	w.asRepository(t)
	// The operator is mid-edit, so the branch they are standing on admits only
	// a worktree arrival.
	w.put("docs/promise/first.md", promiseAt("first", "the first promise, revised"))

	code, stdout, stderr := w.run("land", "some-thread", "--arrival", "ref", "--by", "Test <test@example.invalid>")
	if code != exitUsage {
		t.Fatalf("a pinned ref arrival onto a dirty checkout exited %d: %s", code, stderr)
	}
	if !strings.Contains(stderr, "not clean") {
		t.Fatalf("the refusal does not say why: %q", stderr)
	}
	if strings.Contains(stdout, "landing thread") {
		t.Fatalf("a land that refused its own arrival still announced one: %q", stdout)
	}
}

// TestLandOnADetachedHeadSaysWhatToType: a target is a branch, and a detached
// HEAD is not one — a land is a change reaching a place that outlives the run.
func TestLandOnADetachedHeadSaysWhatToType(t *testing.T) {
	w := newWorkspace(t, map[string]string{"first": "the first promise"})
	git := w.asRepository(t)
	git(w.root, "checkout", "--quiet", "--detach", "HEAD")

	code, _, stderr := w.run("land", "some-thread", "--by", "Test <test@example.invalid>")
	if code != exitUsage {
		t.Fatalf("a land on a detached HEAD exited %d", code)
	}
	if !strings.Contains(stderr, "--onto") {
		t.Fatalf("the refusal does not say what to type: %q", stderr)
	}
}

// TestLandOntoNamesALocalBranchWhenNoRemoteIsCalledThat: `--onto main` is a
// local branch in a repository whose only remote is called origin, and that is
// what makes the operator's own branch reachable at all.
func TestLandOntoNamesALocalBranchWhenNoRemoteIsCalledThat(t *testing.T) {
	w := newWorkspace(t, map[string]string{"first": "the first promise"})
	git := w.asRepository(t)
	git(w.root, "branch", "shipping", "main")

	code, stdout, stderr := w.run("land", "some-thread", "--onto", "shipping", "--by", "Test <test@example.invalid>")
	if code != exitUsage {
		t.Fatalf("exited %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, "on shipping") || !strings.Contains(stdout, "local regime") {
		t.Fatalf("--onto did not resolve a local branch: %q", stdout)
	}
}

// TestLandRefusesAnArrivalThisBuildDoesNotKnow: --arrival takes a value a
// person types, and one nothing knows is an authoring error rather than a
// reason to fall back on the default.
func TestLandRefusesAnArrivalThisBuildDoesNotKnow(t *testing.T) {
	w := newWorkspace(t, map[string]string{"first": "the first promise"})
	w.asRepository(t)

	code, _, stderr := w.run("land", "some-thread", "--arrival", "email", "--by", "Test <test@example.invalid>")
	if code != exitUsage {
		t.Fatalf("an unknown arrival exited %d", code)
	}
	if !strings.Contains(stderr, "email") {
		t.Fatalf("the refusal does not name the value: %q", stderr)
	}
}

// asRepository makes this workspace a git repository with one commit on main,
// which is what a land needs before it can resolve anywhere to go.
func (w *workspace) asRepository(t *testing.T) func(dir string, args ...string) {
	t.Helper()
	git := gitOrSkip(t)
	git(w.root, "init", "--quiet", "--initial-branch=main", ".")
	git(w.root, "add", "--all")
	git(w.root, "commit", "--quiet", "--no-gpg-sign", "-m", "the corpus")
	return git
}

// TestWhyPrintsTheArrivalBesideEveryLand is docs/decision/0015's sixth
// mechanism, and it is the one a reader a year later actually has.
//
// One verb produces two arrivals carrying two different claims — a ref advance
// says every commit that reached the target was gated against the tree it
// landed on, and a worktree arrival put no commit anywhere — so `kept why`
// prints the arrival beside every land and never the word `landed` alone. Both
// are put in front of it here, because the failure this prevents is the two
// reading the same.
func TestWhyPrintsTheArrivalBesideEveryLand(t *testing.T) {
	w := newWorkspace(t, map[string]string{"first": "the first promise"})
	w.asRepository(t)
	w.ratify(t, "promise:first")
	w.append(t, landedAct("promise:first", "origin/main", "shared", "ref", true))
	w.append(t, landedAct("promise:first", "main", "local", "worktree", false))

	code, stdout, stderr := w.run("why", "promise:first")
	if code != exitOK && code != exitDrift && code != exitUnknown {
		t.Fatalf("why exited %d: %s", code, stderr)
	}
	for _, want := range []string{
		"landed",
		"shared · ref",
		"local · worktree",
		"origin/main",
		// A ref advance names the commit that reached the target; a worktree
		// arrival says in its place that nothing was committed.
		"aaaaaaa",
		"nothing committed",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("kept why does not say %q:\n%s", want, stdout)
		}
	}

	// The same three fields on the machine surface, because a distinction a
	// person can see and a machine cannot is a second surface.
	_, doc, _ := w.run("why", "promise:first", "--json")
	for _, want := range []string{
		`"arrival":"ref"`, `"arrival":"worktree"`,
		`"regime":"shared"`, `"regime":"local"`,
		`"uncommitted":true`,
	} {
		if !strings.Contains(strings.Join(strings.Fields(doc), ""), want) {
			t.Fatalf("the why document does not carry %s:\n%s", want, doc)
		}
	}
	// And a worktree arrival carries no commit, because nothing reached
	// anything: the commit on its world is where the operator was standing.
	if strings.Contains(strings.Join(strings.Fields(doc), ""), `"arrival":"worktree","commit"`) {
		t.Fatalf("a worktree arrival reported a commit as though one landed:\n%s", doc)
	}
}

// landedAct is a land act as `kept land` writes one. The commit and the
// uncommitted flag are the pair the ledger refuses to disagree with itself
// about, so they are stated together here as they are there.
func landedAct(artifact, target, regime, arrival string, committed bool) string {
	world := `"world":{"tree":"sha256:` + strings.Repeat("d", 64) + `","ref":"refs/heads/main","commit":"` +
		strings.Repeat("a", 40) + `"}`
	uncommitted := ""
	if !committed {
		uncommitted = `,"uncommitted":true`
	}
	return `{"v":1,"kind":"landed","at":"2026-02-02T00:00:00Z","writer":"fixture","seq":1,` + world + uncommitted +
		`,"artifact":"` + artifact + `","hash":"sha256:` + strings.Repeat("e", 64) + `","norm":1,"by":"the operator"` +
		`,"target":"` + target + `","regime":"` + regime + `","arrival":"` + arrival + `"}`
}
