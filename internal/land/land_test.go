package land

import (
	"context"
	"os"
	oexec "os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/box"
	"github.com/pbleisch/kept/internal/canonical"
	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/gather"
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/lock"
	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/run"
	"github.com/pbleisch/kept/internal/thread"
	"github.com/pbleisch/kept/internal/workspace"
)

// TestARepairThatHoldsAtTheTargetLands is the loop closing: a promise that does
// not hold on the target holds with the repair merged in, and the change reaches
// the target on one act.
//
// The remote is a bare repository in a temporary directory. Nothing in this
// package's tests reaches a real remote, and nothing may.
func TestARepairThatHoldsAtTheTargetLands(t *testing.T) {
	w := newLandWorkspace(t)
	thread := w.anchor("promise:a-is-whole", func(box string) {
		writeFile(t, filepath.Join(box, "sub/a/note/second.md"), noteFile("second"))
	})

	res, err := Perform(context.Background(), w.request(thread))
	if err != nil {
		t.Fatalf("land: %v", err)
	}
	if res.Status != Landed {
		t.Fatalf("status = %q (%s: %s), want %q", res.Status, res.Condition, res.Detail, Landed)
	}
	if got := w.remoteHead(); got != res.Commit {
		t.Fatalf("%s is at %s, and the land reported %s", w.target, got, res.Commit)
	}
	// The gate measured both sides, and the promise moved across the merge.
	verdict := res.gate(t, "promise:a-is-whole")
	if verdict.Before != model.NotKept || verdict.After != model.Kept {
		t.Fatalf("the gate read %s → %s, want not-kept → kept", verdict.Before, verdict.After)
	}
}

// TestALandRecordsAnActCarryingItsWorld holds a land to docs/decision/0063: an
// act names the world it was performed in, and for a land that world is the tree
// it was gated against with the commit that reached the target.
func TestALandRecordsAnActCarryingItsWorld(t *testing.T) {
	w := newLandWorkspace(t)
	th := w.anchor("promise:a-is-whole", func(b string) {
		writeFile(t, filepath.Join(b, "sub/a/note/second.md"), noteFile("second"))
	})

	res, err := Perform(context.Background(), w.request(th))
	if err != nil {
		t.Fatalf("land: %v", err)
	}
	if !res.Recorded {
		t.Fatal("a land that reached the target recorded no act")
	}

	acts, quarantined, err := ledger.ReadActs(w.store)
	if err != nil {
		t.Fatal(err)
	}
	if len(quarantined) != 0 {
		t.Fatalf("the land act did not read back as an act: %+v", quarantined)
	}
	var landed []ledger.Act
	for _, a := range acts {
		if a.Kind == ledger.Landed {
			landed = append(landed, a)
		}
	}
	if len(landed) != 1 {
		t.Fatalf("the ledger holds %d land acts, want 1", len(landed))
	}
	act := landed[0]
	switch {
	case !act.Where.Situated():
		t.Fatal("the land act names no world")
	case act.World.Commit != res.Commit:
		t.Fatalf("the act names commit %s, and %s landed", act.World.Commit, res.Commit)
	case act.World.Tree == "":
		t.Fatal("the land act names no tree, so it does not say what it was gated against")
	case act.World.Ref != w.target.Ref():
		t.Fatalf("the act names ref %q, want %q — the act is where the target is carried", act.World.Ref, w.target.Ref())
	case act.World.Thread != string(th):
		t.Fatalf("the act names thread %q, want %q", act.World.Thread, th)
	case act.Artifact != "promise:a-is-whole":
		t.Fatalf("the act names artifact %q", act.Artifact)
	case act.By == "":
		t.Fatal("the land act names nobody, and a land is a person's act")
	}

	// And docs/decision/0015's three fields, which say which of two guarantees
	// this land carries. A ref advance carries *every commit that reaches a
	// landing target was gated against the tree it landed on*; a worktree
	// arrival carries something narrower. The ledger refuses an act missing any
	// of them, so this is what the one verb that performs a land actually says.
	want := ledger.Landing{Target: w.target.String(), Regime: ledger.SharedRegime, Arrival: ledger.RefArrival}
	if act.Landing != want {
		t.Fatalf("the act's landing is %+v, want %+v", act.Landing, want)
	}
	if act.Uncommitted {
		t.Fatal("a ref arrival was recorded from a tree holding changes no commit does; a gate box is clean by construction")
	}
}

// TestALandActCreatesNoStanding breaks the thing the fold is supposed to catch.
//
// A land act carries an artifact, a hash and a norm — everything a ratification
// carries — and the only thing keeping it from granting standing is that
// register's fold switches on kind. That is a silent failure if it ever stops
// being true: a land would mint standing no person granted, which is the one
// invariant the whole model rests on.
func TestALandActCreatesNoStanding(t *testing.T) {
	w := newLandWorkspace(t)
	c := w.load()
	entry, found := c.Get("promise:unratified")
	if !found {
		t.Fatal("the fixture holds no promise:unratified")
	}
	sc, _ := c.Schemas.For("promise")
	hash, err := canonical.Bind(entry, sc)
	if err != nil {
		t.Fatal(err)
	}

	l := ledger.Open(w.store, run.Start("test-land", nil))
	if _, err := l.Land("promise:unratified", hash.String(), hash.Norm,
		"Test <test@example.com>", "a land is not a ratification",
		ledger.Where{World: model.World{Tree: "t", Commit: strings.Repeat("a", 40)}},
		ledger.Landing{Target: "origin/main", Regime: ledger.SharedRegime, Arrival: ledger.RefArrival}); err != nil {
		t.Fatal(err)
	}
	if err := l.Close(run.Clean); err != nil {
		t.Fatal(err)
	}

	if got := w.standing("promise:unratified"); got == "ratified" {
		t.Fatal("a land act granted standing; only a ratification may, and only a person performs one")
	}
}

// TestACollateralBreakRefusesTheLand is docs/decision/0037's first required
// test: a repair that satisfies its own promise and breaks another does not
// land.
//
// The second promise is not the one the thread was about and nothing named it —
// it is in the gate because the change's paths touch its scope, which is the
// whole of what 0037 widened.
func TestACollateralBreakRefusesTheLand(t *testing.T) {
	w := newLandWorkspace(t)
	before := w.remoteHead()
	th := w.anchor("promise:a-is-whole", func(b string) {
		writeFile(t, filepath.Join(b, "sub/a/note/second.md"), noteFile("second"))
		// Collateral: b-is-whole holds at exactly one artifact.
		writeFile(t, filepath.Join(b, "sub/b/note/intruder.md"), noteFile("intruder"))
	})

	res, err := Perform(context.Background(), w.request(th))
	if err != nil {
		t.Fatalf("land: %v", err)
	}
	if res.Status != Refused || res.Condition != Collateral {
		t.Fatalf("status = %q · %q (%s), want %q · %q", res.Status, res.Condition, res.Detail, Refused, Collateral)
	}
	if !strings.Contains(res.Detail, "promise:b-is-whole") {
		t.Fatalf("the refusal does not name what was broken: %s", res.Detail)
	}
	if got := w.remoteHead(); got != before {
		t.Fatalf("a refused land moved %s from %s to %s", w.target, before, got)
	}

	// Both halves are in the record: the repaired promise holds, and the
	// collateral one went from holding to not.
	if v := res.gate(t, "promise:a-is-whole"); v.After != model.Kept {
		t.Fatalf("the repaired promise reads %s at the target, want kept", v.After)
	}
	if v := res.gate(t, "promise:b-is-whole"); !v.Broke() {
		t.Fatalf("the collateral promise reads %s → %s, want kept → not-kept", v.Before, v.After)
	}
}

// TestAPromiseAlreadyBrokenOnTheTargetDoesNotRefuseTheLand is the other side of
// the same rule, and it is the one that is easy to get wrong.
//
// "A collateral break" is a promise that *held* on the target and does not hold
// with this change in it. A gate that measured only the merged tree would refuse
// every land into a repository that already has a broken promise, which reports
// the state of the target rather than the effect of the change.
func TestAPromiseAlreadyBrokenOnTheTargetDoesNotRefuseTheLand(t *testing.T) {
	w := newLandWorkspace(t)
	// Break b-is-whole on the target itself, before anything is repaired.
	w.write("sub/b/note/already.md", noteFile("already"))
	w.commit()
	w.push()

	th := w.anchor("promise:a-is-whole", func(b string) {
		writeFile(t, filepath.Join(b, "sub/a/note/second.md"), noteFile("second"))
		// Touch b's scope so it is selected into the gate, without being the
		// reason it fails: it was already failing.
		writeFile(t, filepath.Join(b, "sub/b/note/already.md"), reworded("already"))
	})

	res, err := Perform(context.Background(), w.request(th))
	if err != nil {
		t.Fatalf("land: %v", err)
	}
	if res.Status != Landed {
		t.Fatalf("status = %q · %q (%s), want %q — a promise already broken on the target is not this change's doing",
			res.Status, res.Condition, res.Detail, Landed)
	}
	if v := res.gate(t, "promise:b-is-whole"); v.Before != model.NotKept || v.After != model.NotKept {
		t.Fatalf("the already-broken promise reads %s → %s, want not-kept → not-kept", v.Before, v.After)
	}
}

// TestAChangeOutsideTheRepairScopeDoesNotLand is
// docs/policy/out-of-scope-paths-are-never-landed.md at the one place that can
// enforce it.
func TestAChangeOutsideTheRepairScopeDoesNotLand(t *testing.T) {
	w := newLandWorkspace(t)
	before := w.remoteHead()
	th := w.anchor("promise:a-is-whole", func(b string) {
		writeFile(t, filepath.Join(b, "sub/a/note/second.md"), noteFile("second"))
		writeFile(t, filepath.Join(b, "elsewhere/reached.txt"), "a repair reached past its bound\n")
	})

	res, err := Perform(context.Background(), w.request(th))
	if err != nil {
		t.Fatalf("land: %v", err)
	}
	if res.Status != Refused || res.Condition != Scope {
		t.Fatalf("status = %q · %q (%s), want %q · %q", res.Status, res.Condition, res.Detail, Refused, Scope)
	}
	if len(res.Review.Outside) != 1 || res.Review.Outside[0] != "elsewhere/reached.txt" {
		t.Fatalf("the review reports %v outside the scope, want elsewhere/reached.txt", res.Review.Outside)
	}
	if got := w.remoteHead(); got != before {
		t.Fatalf("a change that reached past its bound landed: %s moved to %s", w.target, got)
	}
}

// TestARepairThatDoesNotHoldAtTheTargetDoesNotLand: the box is not the target,
// and a repair is gated against the tree it lands on rather than the one it was
// made in.
func TestARepairThatDoesNotHoldAtTheTargetDoesNotLand(t *testing.T) {
	w := newLandWorkspace(t)
	th := w.anchor("promise:a-is-whole", func(b string) {
		// Inside the promise's scope and nowhere near enough to satisfy it:
		// a-is-whole wants two artifacts and this leaves one.
		writeFile(t, filepath.Join(b, "sub/a/note/first.md"), reworded("first"))
	})

	res, err := Perform(context.Background(), w.request(th))
	if err != nil {
		t.Fatalf("land: %v", err)
	}
	if res.Status != Refused || res.Condition != Promise {
		t.Fatalf("status = %q · %q (%s), want %q · %q", res.Status, res.Condition, res.Detail, Refused, Promise)
	}
}

// TestALandWaitingOnTheLockIsRefusedRatherThanBlocked: the lock is taken for the
// critical section, and a second run that cannot have it reports contention
// rather than landing anyway.
func TestALandWaitingOnTheLockIsRefusedRatherThanBlocked(t *testing.T) {
	w := newLandWorkspace(t)
	before := w.remoteHead()
	th := w.anchor("promise:a-is-whole", func(b string) {
		writeFile(t, filepath.Join(b, "sub/a/note/second.md"), noteFile("second"))
	})

	held, err := lock.Acquire(context.Background(), w.root, lock.Land, 0)
	if err != nil {
		t.Fatalf("hold the land lock: %v", err)
	}
	defer func() { _ = held.Release() }()

	req := w.request(th)
	req.Wait = 100 * time.Millisecond
	res, err := Perform(context.Background(), req)
	if err != nil {
		t.Fatalf("land: %v", err)
	}
	if res.Status != Refused || res.Condition != Contended {
		t.Fatalf("status = %q · %q (%s), want %q · %q", res.Status, res.Condition, res.Detail, Refused, Contended)
	}
	if got := w.remoteHead(); got != before {
		t.Fatalf("a land that never held the lock pushed: %s moved to %s", w.target, got)
	}
}

// TestAThreadWithNoAnchorHasNothingToLand: every repair before anchoring
// existed, and every repair whose session changed nothing, is one of these.
func TestAThreadWithNoAnchorHasNothingToLand(t *testing.T) {
	w := newLandWorkspace(t)
	req := w.request(thread.MintID())
	if _, err := Perform(context.Background(), req); err == nil {
		t.Fatal("a thread that anchored nothing was landed")
	} else if !strings.Contains(err.Error(), "anchored no commit") {
		t.Fatalf("the refusal does not say why: %v", err)
	}
}

// TestALandOfAPromiseWithoutStandingIsRefused: a repair is authorized by
// standing, so landing one whose promise was released since would put a change
// on the target in the name of something nobody is held to.
func TestALandOfAPromiseWithoutStandingIsRefused(t *testing.T) {
	w := newLandWorkspace(t)
	th := w.anchor("promise:a-is-whole", func(b string) {
		writeFile(t, filepath.Join(b, "sub/a/note/second.md"), noteFile("second"))
	})
	w.release("promise:a-is-whole")

	if _, err := Perform(context.Background(), w.request(th)); err == nil {
		t.Fatal("a promise with no standing was landed in its name")
	} else if !strings.Contains(err.Error(), "authorized by standing") {
		t.Fatalf("the refusal does not say why: %v", err)
	}
}

// TestADryRunGatesAndPushesNothing: the gate is the expensive half and it is
// worth being able to run it on its own.
func TestADryRunGatesAndPushesNothing(t *testing.T) {
	w := newLandWorkspace(t)
	before := w.remoteHead()
	th := w.anchor("promise:a-is-whole", func(b string) {
		writeFile(t, filepath.Join(b, "sub/a/note/second.md"), noteFile("second"))
	})

	req := w.request(th)
	req.DryRun = true
	res, err := Perform(context.Background(), req)
	if err != nil {
		t.Fatalf("land: %v", err)
	}
	if res.Status != Prepared {
		t.Fatalf("status = %q (%s), want %q", res.Status, res.Detail, Prepared)
	}
	if got := w.remoteHead(); got != before {
		t.Fatalf("a dry run pushed: %s moved to %s", w.target, got)
	}
	acts, _, err := ledger.ReadActs(w.store)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range acts {
		if a.Kind == ledger.Landed {
			t.Fatal("a dry run recorded a land act")
		}
	}
}

func (r *Result) gate(t *testing.T, artifact string) Verdict {
	t.Helper()
	for _, v := range r.Gate {
		if v.Artifact == artifact {
			return v
		}
	}
	t.Fatalf("the gate did not measure %s; it measured %v", artifact, r.measured())
	return Verdict{}
}

func (r *Result) measured() []string {
	var out []string
	for _, v := range r.Gate {
		out = append(out, v.Artifact)
	}
	return out
}

// --- the fixture ---------------------------------------------------------

type landWorkspace struct {
	t      *testing.T
	root   string
	origin string
	store  ledger.Store
	// records is where the ledger's files are, which a re-executed process
	// needs in order to open the same ledger this one is reading.
	records string
	target  Target
}

func newLandWorkspace(t *testing.T) *landWorkspace {
	t.Helper()
	w := &landWorkspace{t: t, root: resolve(t, t.TempDir()), origin: resolve(t, t.TempDir()) + "/origin.git"}
	// The ledger lives outside the workspace: a box refuses a checkout holding
	// a machine-written tier, and none of this is about that guard.
	w.records = t.TempDir()
	w.store = ledger.NewFiles(w.records)
	w.target = Target{Remote: "origin", Branch: "main"}

	gitAt(t, filepath.Dir(w.origin), "init", "--quiet", "--bare", "--initial-branch=main", w.origin)

	w.write(".kept/config.yaml", "corpus: docs\n")
	w.write("docs/_schema/promise.yaml", landPromiseSchema)
	// a-is-whole is bounded by sub/** while it measures sub/a alone. A repair
	// scope wider than a gather scope is the ordinary shape — internal/** in
	// kept's own corpus is one — and it is what makes a collateral break
	// reachable at all: a repair bounded to exactly what it measures can only
	// ever break itself.
	w.write("docs/promise/a-is-whole.md", wholePromise("a-is-whole", "sub/a", 2, "sub/**"))
	w.write("docs/promise/b-is-whole.md", wholePromise("b-is-whole", "sub/b", 1, "sub/b/**"))
	w.write("docs/promise/unratified.md", wholePromise("unratified", "sub/a", 2, "sub/**"))
	// Two small corpora the promises measure. sub/a holds one artifact and
	// wants two; sub/b holds one and wants one.
	w.write("sub/a/_schema/note.yaml", landNoteSchema)
	w.write("sub/a/note/first.md", noteFile("first"))
	w.write("sub/b/_schema/note.yaml", landNoteSchema)
	w.write("sub/b/note/only.md", noteFile("only"))

	w.git("init", "--quiet", "--initial-branch=main")
	w.git("config", "user.email", "test@example.invalid")
	w.git("config", "user.name", "Test")
	w.git("remote", "add", "origin", w.origin)
	w.commit()
	w.push()

	w.ratify("promise:a-is-whole", "promise:b-is-whole")
	return w
}

func (w *landWorkspace) request(id thread.ID) Request {
	w.t.Helper()
	acts, _, err := ledger.ReadActs(w.store)
	if err != nil {
		w.t.Fatal(err)
	}
	l := ledger.Open(w.store, run.Start("test-land", nil))
	w.t.Cleanup(func() { _ = l.Close(run.Clean) })
	return Request{
		Workspace: w.root,
		Corpus:    w.load(),
		Acts:      acts,
		Thread:    string(id),
		Target:    w.target,
		By:        "Test <test@example.invalid>",
		Deadlines: Deadlines{Gatherer: 60 * time.Second, Git: 60 * time.Second},
		Store:     w.store,
		Ledger:    l,
		Wait:      5 * time.Second,
	}
}

// anchor produces what a repair leaves behind: a box cut at the workspace's
// commit, changed, committed, and pinned under refs/kept/threads/<id> with the
// receipt entry naming it.
//
// It goes through box.Anchor rather than composing a commit here, because what
// a land is handed has to be what a repair actually writes.
func (w *landWorkspace) anchor(promise string, change func(box string)) thread.ID {
	w.t.Helper()
	head := strings.TrimSpace(w.git("rev-parse", "HEAD"))
	created, err := box.Create(context.Background(), box.Request{
		Repository: w.root, Commit: head, Deadline: 60 * time.Second,
	})
	if err != nil {
		w.t.Fatal(err)
	}
	defer func() { _ = created.Close(context.Background()) }()
	change(created.Path())

	id := thread.MintID()
	ref, err := ledger.ThreadRef(string(id))
	if err != nil {
		w.t.Fatal(err)
	}
	commit, err := created.Anchor(context.Background(), ref, "repair "+promise+"\n",
		box.Identity{Name: "Test", Email: "test@example.invalid"})
	if err != nil {
		w.t.Fatal(err)
	}
	w.t.Cleanup(func() { _, _ = oexec.Command("git", "-C", w.root, "update-ref", "-d", ref).CombinedOutput() })

	l := ledger.Open(w.store, run.Start("test-anchor", nil))
	receipt, err := thread.NewReceipt(l, id)
	if err != nil {
		w.t.Fatal(err)
	}
	if err := receipt.RecordAnchor(thread.Anchor{Commit: commit, Parent: head, Promise: promise}); err != nil {
		w.t.Fatal(err)
	}
	if err := l.Close(run.Clean); err != nil {
		w.t.Fatal(err)
	}
	return id
}

func (w *landWorkspace) load() *corpus.Corpus {
	w.t.Helper()
	c, err := corpus.Load(filepath.Join(w.root, "docs"), w.root, corpus.Options{Gatherers: gather.Builtin()})
	if err != nil {
		w.t.Fatal(err)
	}
	return c
}

func (w *landWorkspace) ratify(ids ...string) {
	w.t.Helper()
	c := w.load()
	l := ledger.Open(w.store, run.Start("test-ratify", nil))
	for _, id := range ids {
		entry, found := c.Get(id)
		if !found {
			w.t.Fatalf("the fixture corpus holds no %s", id)
		}
		sc, _ := c.Schemas.For("promise")
		hash, err := canonical.Bind(entry, sc)
		if err != nil {
			w.t.Fatal(err)
		}
		if _, err := l.Ratify(id, hash.String(), hash.Norm, "Test <test@example.invalid>", "fixture", ledger.Where{}); err != nil {
			w.t.Fatal(err)
		}
	}
	if err := l.Close(run.Clean); err != nil {
		w.t.Fatal(err)
	}
}

func (w *landWorkspace) release(id string) {
	w.t.Helper()
	c := w.load()
	entry, _ := c.Get(id)
	sc, _ := c.Schemas.For("promise")
	hash, err := canonical.Bind(entry, sc)
	if err != nil {
		w.t.Fatal(err)
	}
	l := ledger.Open(w.store, run.Start("test-release", nil))
	if _, err := l.Release(id, hash.String(), hash.Norm, "Test <test@example.invalid>", "fixture", ledger.Where{}); err != nil {
		w.t.Fatal(err)
	}
	if err := l.Close(run.Clean); err != nil {
		w.t.Fatal(err)
	}
}

func (w *landWorkspace) standing(id string) string {
	w.t.Helper()
	acts, _, err := ledger.ReadActs(w.store)
	if err != nil {
		w.t.Fatal(err)
	}
	return string(standingOf(Request{Corpus: w.load(), Acts: acts}, id).State)
}

func (w *landWorkspace) write(name, body string) {
	w.t.Helper()
	writeFile(w.t, filepath.Join(w.root, filepath.FromSlash(name)), body)
}

func (w *landWorkspace) git(args ...string) string { return gitAt(w.t, w.root, args...) }

func (w *landWorkspace) commit() {
	w.t.Helper()
	w.git("add", "--all")
	w.git("commit", "--quiet", "--no-gpg-sign", "--allow-empty", "-m", "corpus")
}

func (w *landWorkspace) push() {
	w.t.Helper()
	w.git("push", "--quiet", "origin", "HEAD:refs/heads/main")
}

// remoteHead reads the branch out of the bare remote itself, never out of a
// tracking ref this process wrote. A test that read the tracking ref would be
// asking kept whether kept pushed.
func (w *landWorkspace) remoteHead() string {
	w.t.Helper()
	return strings.TrimSpace(gitAt(w.t, w.origin, "rev-parse", "--verify", "refs/heads/main"))
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func gitAt(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := oexec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return string(out)
}

func resolve(t *testing.T, path string) string {
	t.Helper()
	out, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

const landPromiseSchema = `type: promise
hash: [statement, gather, hold_when, trigger, repair]
fields:
  statement: { type: string, required: true }
  gather:    { type: map }
  hold_when: { type: string }
  trigger:   { type: enum, values: [cadence, on-act, once] }
  max_age:   { type: string }
  repair:    { type: map }
body: why
`

const landNoteSchema = `type: note
hash: [statement]
fields:
  statement: { type: string }
body: why
`

func noteFile(slug string) string {
	return `---
id: ` + slug + `
type: note
title: ` + slug + `
fields:
  statement: ` + slug + `
---

Why: a note the corpus gatherer can count.
`
}

// reworded is the same artifact with different prose. The slug is the identity,
// so it does not move: a note whose id changed would be a different artifact,
// and one whose id is not a slug would not parse and would silently change the
// count these promises are about.
func reworded(slug string) string {
	return strings.Replace(noteFile(slug), "Why: a note the corpus gatherer can count.",
		"Why: a note the corpus gatherer can count, said differently.", 1)
}

// wholePromise is measured by `corpus`, which reads a corpus root and shells
// nothing out — so it answers identically on every machine and needs no
// toolchain inside a box.
func wholePromise(slug, root string, artifacts int, repairScope string) string {
	return `---
id: ` + slug + `
type: promise
title: ` + slug + `
fields:
  statement: the corpus at ` + root + ` holds what it should
  gather:
    c:
      gatherer: corpus
      root: "` + root + `"
      scope: ["` + root + `/**"]
  hold_when: obs.c.artifacts == ` + itoa(artifacts) + `
  trigger: cadence
  repair:
    scope: ["` + repairScope + `"]
---

Why: because a promise nobody can check is not one.
`
}

func itoa(n int) string { return string(rune('0' + n)) }

// thirdPartyPush is another checkout of the same remote putting a commit on the
// target — the operator's laptop, CI, a second machine. It is the run
// docs/decision/0015 says a local lock cannot exclude, and it never touches this
// workspace's own branch.
func (w *landWorkspace) thirdPartyPush(t *testing.T) string {
	t.Helper()
	other := filepath.Join(resolve(t, t.TempDir()), "second-checkout")
	gitAt(t, filepath.Dir(other), "clone", "--quiet", w.origin, other)
	gitAt(t, other, "config", "user.email", "other@example.invalid")
	gitAt(t, other, "config", "user.name", "Other")
	writeFile(t, filepath.Join(other, "from-elsewhere.txt"), "a commit only the remote knows about\n")
	gitAt(t, other, "add", "--all")
	gitAt(t, other, "commit", "--quiet", "--no-gpg-sign", "-m", "from elsewhere")
	gitAt(t, other, "push", "--quiet", "origin", "HEAD:refs/heads/main")
	return w.remoteHead()
}

// remoteReaches reports whether what is on the target now contains a commit.
func (w *landWorkspace) remoteReaches(t *testing.T, commit string) bool {
	t.Helper()
	out, err := gitTry(w.origin, "merge-base", "--is-ancestor", commit, "refs/heads/main")
	_ = out
	return err == nil
}

// gitTry is a git call whose failure is an answer rather than a fatal error.
func gitTry(dir string, args ...string) (string, error) {
	out, err := oexec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	return string(out), err
}

// TestTheActNamesTheGatedTreeAndNotTheOperatorsWorkspace breaks the thing that
// is easy to get wrong and impossible to notice afterwards.
//
// docs/decision/0015 asks a land to record what it was gated against. The gate
// runs in a box holding the merge; the operator's workspace is a different tree
// and may hold anything at all. An act naming the second would say a land was
// gated against a world nothing measured — and because both are valid digests,
// nothing downstream could tell.
func TestTheActNamesTheGatedTreeAndNotTheOperatorsWorkspace(t *testing.T) {
	w := newLandWorkspace(t)
	th := w.anchor("promise:a-is-whole", func(b string) {
		writeFile(t, filepath.Join(b, "sub/a/note/second.md"), noteFile("second"))
	})
	// Make the operator's workspace unmistakably a different tree.
	w.write("only-in-the-operators-tree.txt", "not in any box\n")

	res, err := Perform(context.Background(), w.request(th))
	if err != nil {
		t.Fatalf("land: %v", err)
	}
	if res.Status != Landed {
		t.Fatalf("status = %q · %q (%s)", res.Status, res.Condition, res.Detail)
	}

	operator, err := workspace.Read(w.root, workspace.Options{Workers: 1, All: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Tree == "" {
		t.Fatal("the land recorded no gated tree")
	}
	if res.Tree == operator.Digest() {
		t.Fatal("the act names the operator's workspace as the tree it was gated against")
	}

	acts, _, err := ledger.ReadActs(w.store)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range acts {
		if a.Kind != ledger.Landed {
			continue
		}
		if a.World.Tree != res.Tree {
			t.Fatalf("the act names tree %q and the gate measured %q", a.World.Tree, res.Tree)
		}
		if a.World.Tree == operator.Digest() {
			t.Fatal("the recorded act names the operator's workspace")
		}
	}
}

// TestARegatedLandLeavesNoBoxBehind: --keep retains the box a person wants to
// read, and an attempt whose target moved is superseded — retaining one of those
// per attempt would leave registered worktrees in the repository.
func TestARegatedLandLeavesNoBoxBehind(t *testing.T) {
	w := newLandWorkspace(t)
	th := w.anchor("promise:a-is-whole", func(b string) {
		writeFile(t, filepath.Join(b, "sub/a/note/second.md"), noteFile("second"))
	})

	req := w.request(th)
	req.Keep = true
	res, err := Perform(context.Background(), req)
	if err != nil {
		t.Fatalf("land: %v", err)
	}
	if res.Box == "" {
		t.Fatal("--keep retained no box")
	}
	// Exactly one: the box the result is about, and nothing from an earlier
	// attempt or from the repair that produced the anchor.
	listed := strings.TrimSpace(w.git("worktree", "list", "--porcelain"))
	boxes := strings.Count(listed, "worktree ") - 1
	if boxes != 1 {
		t.Fatalf("the repository holds %d boxes after one kept land:\n%s", boxes, listed)
	}
}

// TestAWorkspaceAheadOfTheTargetCannotSmuggleItsOwnCommitsIntoALand pins what
// happens because 0015's third deployment constraint is not met.
//
// Boxes are cut from HEAD rather than from origin/main, so an anchor's parent is
// the operator's checkout rather than the landing target and 0015's "a thread's
// diff then applies to the landing target by construction" does not hold. When
// the checkout is *ahead*, merging the anchor into the target brings the
// operator's unlanded commits along with the repair.
//
// It fails closed, and that is the property worth pinning: those paths are
// outside the promise's bound, so the land refuses rather than putting work
// nobody gated onto the target. What it does not do is say that the cause is the
// checkout rather than the repair — the refusal names the stray path and leaves
// the diagnosis to a person. Cutting boxes from the target is its own slice; if
// it lands, this test should start failing and be replaced by one asserting the
// land succeeds.
func TestAWorkspaceAheadOfTheTargetCannotSmuggleItsOwnCommitsIntoALand(t *testing.T) {
	w := newLandWorkspace(t)
	before := w.remoteHead()
	w.write("unlanded-local-work.txt", "on the operator's checkout and not on the target\n")
	w.commit()

	th := w.anchor("promise:a-is-whole", func(b string) {
		writeFile(t, filepath.Join(b, "sub/a/note/second.md"), noteFile("second"))
	})
	res, err := Perform(context.Background(), w.request(th))
	if err != nil {
		t.Fatalf("land: %v", err)
	}
	if res.Status != Refused || res.Condition != Scope {
		t.Fatalf("status = %q · %q (%s), want %q · %q — an unlanded local commit must not ride along",
			res.Status, res.Condition, res.Detail, Refused, Scope)
	}
	if len(res.Review.Outside) != 1 || res.Review.Outside[0] != "unlanded-local-work.txt" {
		t.Fatalf("the review reports %v outside the bound, want the operator's own commit", res.Review.Outside)
	}
	if got := w.remoteHead(); got != before {
		t.Fatalf("the operator's unlanded work reached %s", w.target)
	}
}

// TestAWorkspaceBehindTheTargetStillLands is the same mismatch the other way,
// and it is the ordinary case: the merge brings the target's newer commits into
// the gate box, what lands is the repair alone, and the gate measured the whole
// of it.
func TestAWorkspaceBehindTheTargetStillLands(t *testing.T) {
	w := newLandWorkspace(t)
	th := w.anchor("promise:a-is-whole", func(b string) {
		writeFile(t, filepath.Join(b, "sub/a/note/second.md"), noteFile("second"))
	})
	// The target moves ahead of this checkout, from somewhere else.
	moved := w.thirdPartyPush(t)

	res, err := Perform(context.Background(), w.request(th))
	if err != nil {
		t.Fatalf("land: %v", err)
	}
	if res.Status != Landed {
		t.Fatalf("status = %q · %q (%s), want %q", res.Status, res.Condition, res.Detail, Landed)
	}
	if !res.Review.Bounded() {
		t.Fatalf("what landed reaches outside the bound: %v", res.Review.Outside)
	}
	if !w.remoteReaches(t, moved) {
		t.Fatalf("what landed does not contain the commit the target already had, %s", moved)
	}
}
