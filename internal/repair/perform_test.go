package repair

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/canonical"
	"github.com/pbleisch/kept/internal/config"
	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/gather"
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/run"
	"github.com/pbleisch/kept/internal/thread"
)

// TestARepairScopeBecomesTheCoarsestGrantThatCanExpressIt holds the rendering to
// what decision 0059 says it is: a grant is always at least as wide as the glob
// it stands for, and there is no finer expression of one.
//
// The width is the reason the review exists, so it is measured here rather than
// assumed anywhere else.
func TestARepairScopeBecomesTheCoarsestGrantThatCanExpressIt(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "box")
	for _, c := range []struct {
		scope []string
		want  []string
		why   string
	}{
		{
			scope: []string{"internal/gather/**/*.go"},
			want:  []string{filepath.Join(root, "internal", "gather")},
			why:   "the directory a ** hangs off is the coarsest expression of it",
		},
		{
			scope: []string{"cmd/kept/main.go"},
			want:  []string{filepath.Join(root, "cmd", "kept")},
			why:   "a repair that may replace one file may have to create one beside it",
		},
		{
			scope: []string{"internal/*/scope.go", "internal/gather/**"},
			want:  []string{filepath.Join(root, "internal")},
			why:   "a grant another grant already contains is not named twice",
		},
		{
			scope: []string{"docs/promise/", "docs/decision/"},
			want:  []string{filepath.Join(root, "docs", "decision"), filepath.Join(root, "docs", "promise")},
			why:   "a trailing separator is a whole subtree and both are named",
		},
		{
			scope: []string{"**/*.go"},
			want:  []string{root},
			why:   "a scope whose first segment is a wildcard reaches the whole tree",
		},
	} {
		got, err := writeGrants(root, c.scope)
		if err != nil {
			t.Fatalf("%v: %v", c.scope, err)
		}
		if strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Fatalf("writeGrants(%v) = %v, want %v — %s", c.scope, got, c.want, c.why)
		}
	}
}

// TestARepairScopeMayNotLeaveTheBox breaks the guard above: a scope climbing out
// of the workspace would render a grant on a tree the promise never bound.
func TestARepairScopeMayNotLeaveTheBox(t *testing.T) {
	if _, err := writeGrants(filepath.Join(string(filepath.Separator), "box"), []string{"../elsewhere/**"}); err == nil {
		t.Fatal("a repair scope reaching outside the workspace rendered a grant")
	}
	if _, err := writeGrants(filepath.Join(string(filepath.Separator), "box"), []string{"  "}); err == nil {
		t.Fatal("an empty repair scope entry rendered a grant")
	}
}

// TestAnUnsetProviderIsNotNone is decision 0017's rule at the one call site that
// can now say yes, and the case beside it is the other half: a name kept renders
// no profile for is refused rather than resolved and started.
func TestAnUnsetProviderIsNotNone(t *testing.T) {
	if _, err := providerExecutable(""); err == nil {
		t.Fatal("an unset confinement provider resolved to something")
	} else if !strings.Contains(err.Error(), "unset is not none") {
		t.Fatalf("the refusal does not name the condition: %v", err)
	}
	if _, err := providerExecutable("firejail"); err == nil {
		t.Fatal("a provider kept renders no profile for resolved to something")
	}
}

// TestTheRuntimeIsRefusedBeforeABoxIsCut covers what can be said about the
// repair runtime with no external call, which is what decides whether a box is
// worth cutting at all.
func TestTheRuntimeIsRefusedBeforeABoxIsCut(t *testing.T) {
	for _, c := range []struct {
		user config.Repair
		want string
	}{
		{config.Repair{}, "no repair runtime is named"},
		{config.Repair{Runtime: "claude"}, "no executable"},
		{config.Repair{Runtime: "claude", Executable: "claude"}, "not an explicit absolute path"},
		{config.Repair{Runtime: "claude", Executable: filepath.Join(t.TempDir(), "absent")}, "which is not there"},
	} {
		why := runtimeUnready(c.user)
		if !strings.Contains(why, c.want) {
			t.Fatalf("runtimeUnready(%#v) = %q, want it to name %q", c.user, why, c.want)
		}
	}

	present := filepath.Join(t.TempDir(), "runtime")
	if err := os.WriteFile(present, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if why := runtimeUnready(config.Repair{Runtime: "claude", Executable: present}); why != "" {
		t.Fatalf("a configured, installed runtime was refused: %s", why)
	}
}

// moduleRoot is the checkout these tests are compiled from.
//
// It lives here rather than beside the admission proof because
// keptRepairRepository leaves that file out of the repository it hands a
// confined session, and package repair still has to compile there.
func moduleRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("%s is not the module root: %v", root, err)
	}
	return root
}

// TestTheConfinementPromiseAuthorizesNoRepair is the refusal above, reached
// through this repository's own corpus rather than a fixture.
//
// `promise:the-confinement-suite-passes` is gathered every tick and repaired by
// nobody, and the whole of what makes that true is that it writes no `repair`
// block — there is no flag, no list and no second mechanism. So the thing worth
// guarding is the artifact, not the code: adding a repair scope to it would
// dispatch a confined agent at a suite that starts confined children of its own,
// and nothing else in the tree would notice.
//
// It is ratified into a scratch ledger first, because a repair is refused for
// the first reason it finds and an unratified promise never reaches the one
// under test.
func TestTheConfinementPromiseAuthorizesNoRepair(t *testing.T) {
	root := moduleRoot(t)
	c, err := corpus.Load(filepath.Join(root, "docs"), root, corpus.Options{Gatherers: gather.Builtin()})
	if err != nil {
		t.Fatal(err)
	}
	const id = "promise:the-confinement-suite-passes"
	entry, found := c.Get(id)
	if !found {
		t.Fatalf("this repository's corpus holds no %s", id)
	}
	if len(entry.Artifact.Gather) == 0 || entry.Artifact.HoldWhen == "" {
		t.Fatal("the confinement promise names no instrument, so it is not measured either")
	}
	if scope := entry.Artifact.RepairScope; len(scope) != 0 {
		t.Fatalf("the confinement promise bounds %v, so the loop will dispatch an agent at it", scope)
	}

	store := ledger.NewFiles(t.TempDir())
	l := ledger.Open(store, run.Start("test-ratify", nil))
	sc, known := c.Schemas.For("promise")
	if !known {
		t.Fatal("the corpus declares no promise schema")
	}
	hash, err := canonical.Bind(entry, sc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.Ratify(id, hash.String(), hash.Norm, "Test <test@example.com>", "fixture", ledger.Where{}); err != nil {
		t.Fatal(err)
	}
	if err := l.Close(run.Clean); err != nil {
		t.Fatal(err)
	}
	acts, _, err := ledger.ReadActs(store)
	if err != nil {
		t.Fatal(err)
	}

	// Nothing here is reached: Perform resolves the subject before it resolves a
	// provider, cuts a box or opens a store, which is why a refusal on the
	// repair bound costs no machine anything.
	_, err = Perform(t.Context(), Request{
		Workspace: root,
		Corpus:    c,
		Acts:      acts,
		Promise:   id,
		Store:     store,
	})
	var unrepairable *Unrepairable
	if err == nil {
		t.Fatal("a repair was dispatched at the confinement suite")
	}
	if !asUnrepairable(err, &unrepairable) || !strings.Contains(unrepairable.Why, "bounds no repair scope") {
		t.Fatalf("the refusal is %v, want it to name the missing repair bound", err)
	}
}

// TestAPromiseThatCannotAuthorizeARepairIsRefusedWithoutARecord holds the four
// shapes that no machine and no moment would change.
//
// None of them writes a disposition, and that is the decision rather than an
// omission: what cannot be reconstructed later is worth writing down, and every
// one of these is re-derivable from the corpus and the ledger at any time.
func TestAPromiseThatCannotAuthorizeARepairIsRefusedWithoutARecord(t *testing.T) {
	w := newRepairWorkspace(t)
	for _, c := range []struct {
		name    string
		promise string
		want    string
	}{
		{"a decision is not a promise", "decision:0001-a-decision", "a repair is authorized by a promise"},
		{"nothing has ratified it", "promise:unratified", "nothing is owed until a person ratifies"},
		{"it bounds nothing", "promise:unbounded", "bounds no repair scope"},
		{"it names no instrument", "promise:uninstrumented", "names no instrument"},
		{"the corpus does not hold it", "promise:absent", "the corpus holds no"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := Perform(context.Background(), w.request(c.promise))
			var unrepairable *Unrepairable
			if err == nil {
				t.Fatal("a promise that cannot authorize a repair was accepted")
			}
			if !asUnrepairable(err, &unrepairable) || !strings.Contains(unrepairable.Why, c.want) {
				t.Fatalf("refusal = %v, want it to name %q", err, c.want)
			}
			if records, _, err := ledger.ReadDispositions(w.store, c.promise); err != nil {
				t.Fatal(err)
			} else if len(records) != 0 {
				t.Fatalf("an unrepairable promise left %d dispositions behind", len(records))
			}
		})
	}
}

// TestAnUnsetProviderRefusesTheRepairAndRecordsWhy is the refusal at the one
// call site that can now say yes, with the record docs/decision/0019 requires.
//
// The condition is named, the detail is the one an operator has to act on, and
// nothing was dispatched.
func TestAnUnsetProviderRefusesTheRepairAndRecordsWhy(t *testing.T) {
	w := newRepairWorkspace(t)
	req := w.request("promise:counted")
	req.Provider = ""

	res, err := Perform(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != Refused || res.Outcome != ledger.OutcomeRefused || res.Condition != ledger.Provider {
		t.Fatalf("an unset provider produced %s / %s / %s", res.Status, res.Outcome, res.Condition)
	}
	if !res.Recorded {
		t.Fatal("the refusal was not recorded, and a refusal cannot be re-derived")
	}
	if res.Thread != "" || res.Commit != "" {
		t.Fatalf("a refused repair cut a box or opened a thread: %#v", res)
	}

	records, _, err := ledger.ReadDispositions(w.store, "promise:counted")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("the refusal wrote %d records, want 1", len(records))
	}
	if records[0].Condition != ledger.Provider || !strings.Contains(records[0].Detail, "unset is not none") {
		t.Fatalf("the recorded refusal does not name its condition: %#v", records[0])
	}
	if records[0].World.Tree == "" {
		t.Fatal("the refusal names no world, and a record written without one cannot be backfilled")
	}

	// And the same refusal on the same grounds a second time is one record and a
	// heartbeat. Without this the volume argument defeats the decision.
	if _, err := Perform(context.Background(), w.request("promise:counted")); err != nil {
		t.Fatal(err)
	}
	records, _, err = ledger.ReadDispositions(w.store, "promise:counted")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("repeating the same refusal wrote %d records, want 1", len(records))
	}
}

// TestAnUnconfiguredRuntimeRefusesOnItsOwnCondition is the other half of the
// pair, and it is why the condition is not the provider's.
//
// A jail that verifies and a credential that has lapsed are two different
// conversations, and a stream that called both `provider` could not report the
// day one became the other: the transition rule compares the outcome and the
// condition and nothing else.
func TestAnUnconfiguredRuntimeRefusesOnItsOwnCondition(t *testing.T) {
	w := newRepairWorkspace(t)
	req := w.request("promise:counted")
	req.Provider = "nono"
	req.Operator = config.Repair{}

	res, err := Perform(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if res.Condition != ledger.Runtime {
		t.Fatalf("an unconfigured repair runtime refused on %q, want %q", res.Condition, ledger.Runtime)
	}
	if !res.Recorded {
		t.Fatal("the refusal was not recorded")
	}

	// The transition the collapsed condition would have hidden: same outcome,
	// different part of the system, so it is a new record.
	req.Provider = ""
	req.Operator = config.Repair{}
	if _, err := Perform(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	records, _, err := ledger.ReadDispositions(w.store, "promise:counted")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("runtime then provider wrote %d records, want 2", len(records))
	}
	if records[0].Condition != ledger.Runtime || records[1].Condition != ledger.Provider {
		t.Fatalf("the conditions recorded are %q then %q", records[0].Condition, records[1].Condition)
	}
}

// TestAnUnratifiedDispatchPolicyRefusesThePacket is the guard that a packet
// never carries authority nobody granted, broken deliberately: the policy that
// binds a dispatch is released, and the packet must refuse rather than quietly
// carry one fewer.
func TestAnUnratifiedDispatchPolicyRefusesThePacket(t *testing.T) {
	w := newRepairWorkspace(t)
	w.release("policy:dispatch-is-bounded")

	if _, err := dispatchAuthority(w.request("promise:counted")); err == nil {
		t.Fatal("a released dispatch policy was carried into a packet")
	} else if !strings.Contains(err.Error(), "authority nobody granted") {
		t.Fatalf("the refusal does not say why: %v", err)
	}
}

// TestAPromiseThatHoldsAtTheCommitDispatchesNothing is what a packet's premise
// costs: a packet states a gap (docs/decision/0009), and a promise that holds
// where the box was cut has none.
//
// It is also the honest report of drift the working tree carries and the commit
// does not, which is the case the box's own measurement is what settles.
func TestAPromiseThatHoldsAtTheCommitDispatchesNothing(t *testing.T) {
	provider := liveProvider(t)
	w := newRepairWorkspace(t)
	w.commit()

	req := w.request("promise:counted")
	req.Provider = filepath.Base(provider)
	req.Operator = config.Repair{Runtime: "claude", Executable: provider}

	res, err := Perform(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != Holds {
		t.Fatalf("a promise that holds at the commit produced %s: %s", res.Status, res.Detail)
	}
	if res.Before.Value != model.Kept {
		t.Fatalf("the box's own measurement reads %s", res.Before.Value)
	}
	if res.Thread != "" {
		t.Fatal("a promise with no gap opened a thread")
	}
	if records, _, err := ledger.ReadDispositions(w.store, "promise:counted"); err != nil {
		t.Fatal(err)
	} else if len(records) != 0 {
		t.Fatalf("a promise with no gap left %d dispositions behind", len(records))
	}

	// The box is gone, because a box is disposable and a repair that quietly
	// accumulated worktrees in a person's repository would be a mess they did
	// not ask for.
	if res.Box != "" {
		t.Fatalf("a discarded box was reported at %s", res.Box)
	}
	if entries, err := os.ReadDir(filepath.Join(w.root, ".kept", "boxes")); err != nil {
		t.Fatal(err)
	} else if len(entries) != 0 {
		t.Fatalf("%d boxes were left in the repository", len(entries))
	}

	// And --keep is the other half: the worktree stays and is named, and the
	// session's own scratch and home go either way.
	kept := w.request("promise:counted")
	kept.Provider = filepath.Base(provider)
	kept.Operator = config.Repair{Runtime: "claude", Executable: provider}
	kept.Keep = true
	res, err = Perform(context.Background(), kept)
	if err != nil {
		t.Fatal(err)
	}
	if res.Box == "" {
		t.Fatal("--keep discarded the box anyway")
	}
	if info, err := os.Stat(res.Box); err != nil || !info.IsDir() {
		t.Fatalf("--keep named a box that is not there: %s", res.Box)
	}
	if _, err := os.Stat(res.Box + ".tmp"); err == nil {
		t.Fatalf("--keep retained the session's scratch directory at %s.tmp", res.Box)
	}
	if _, err := os.Stat(res.Box + ".home"); err == nil {
		t.Fatalf("--keep retained the session's home at %s.home", res.Box)
	}
	git(t, w.root, "worktree", "remove", "--force", res.Box)
}

// TestTheBoxIsMeasuredBeforeAnythingIsDispatched is the same path with the
// promise really broken in the commit: the gap is established, the packet is
// assembled from evidence measured in the box, and what refuses is the runtime
// rather than anything about the promise.
//
// It stops at the runtime deliberately. Everything up to a live session is
// exercised here and costs nothing; the session itself is
// claudeadmission_test.go's evidence.
func TestTheBoxIsMeasuredBeforeAnythingIsDispatched(t *testing.T) {
	provider := liveProvider(t)
	w := newRepairWorkspace(t)
	w.write("docs/promise/spare.md", promiseFile("spare", "docs/**", "obs.c.ok"))
	w.commit()

	req := w.request("promise:counted")
	req.Provider = filepath.Base(provider)
	// An executable that exists and is not a Claude runtime: the box is cut and
	// measured before this is asked anything, which is the ordering under test.
	req.Operator = config.Repair{Runtime: "claude", Executable: provider}

	res, err := Perform(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if res.Before.Value != model.NotKept {
		t.Fatalf("the extra artifact did not break the promise in the box: %s %s", res.Before.Value, res.Before.Detail)
	}
	if res.Commit == "" {
		t.Fatal("the repair reported no commit, so no box was cut")
	}
	if res.Packet.Empty() {
		t.Fatal("the gap was established and no packet was assembled from it")
	}
	if !strings.Contains(string(res.Packet.Bytes()), "promise:counted") {
		t.Fatalf("the packet does not name the promise: %s", res.Packet.Bytes())
	}
	if res.Status != Refused || res.Condition != ledger.Runtime {
		t.Fatalf("the repair produced %s / %s: %s", res.Status, res.Condition, res.Detail)
	}
	if !res.Recorded {
		t.Fatal("a refusal after the gap was established was not recorded")
	}
}

func asUnrepairable(err error, into **Unrepairable) bool {
	u, ok := err.(*Unrepairable)
	if ok {
		*into = u
	}
	return ok
}

// repairWorkspace is a git repository with a corpus a repair can be pointed at.
//
// The promise measures with `corpus`, which reads a corpus root and needs no
// toolchain — so it answers the same on any machine, and adding one artifact is
// enough to break it.
type repairWorkspace struct {
	t     *testing.T
	root  string
	store ledger.Store
}

func newRepairWorkspace(t *testing.T) *repairWorkspace {
	t.Helper()
	w := &repairWorkspace{t: t, root: t.TempDir()}
	resolved, err := filepath.EvalSymlinks(w.root)
	if err != nil {
		t.Fatal(err)
	}
	w.root = resolved
	// The ledger is outside the workspace: a box refuses a checkout that would
	// put a machine-written tier in the repairer's tree, and this test is not
	// about that guard.
	w.store = ledger.NewFiles(t.TempDir())

	w.write(".kept/config.yaml", "corpus: docs\n")
	w.write("docs/_schema/promise.yaml", repairPromiseSchema)
	w.write("docs/_schema/policy.yaml", repairPolicySchema)
	w.write("docs/_schema/decision.yaml", repairDecisionSchema)
	w.write("docs/policy/dispatch-is-bounded.md", dispatchPolicyFile)
	w.write("docs/decision/0001-a-decision.md", decisionFile)
	w.write("docs/promise/counted.md", promiseFile("counted", "docs/**", "obs.c.artifacts == 7"))
	w.write("docs/promise/unratified.md", promiseFile("unratified", "docs/**", "obs.c.ok"))
	w.write("docs/promise/unbounded.md", strings.Replace(promiseFile("unbounded", "docs/**", "obs.c.ok"),
		"  repair:\n    scope: [\"docs/**\"]\n", "", 1))
	w.write("docs/promise/two-instruments.md", twoInstrumentPromise)
	w.write("docs/promise/uninstrumented.md", uninstrumentedPromise)
	// A second corpus root, so the two-instrument promise's halves are about
	// two different things and each can be moved without moving the other.
	w.write("notes/_schema/note.yaml", repairNoteSchema)
	w.write("notes/note/one.md", noteFile("one"))

	w.ratify("promise:counted", "promise:unbounded", "promise:two-instruments", "promise:uninstrumented",
		"policy:dispatch-is-bounded", "decision:0001-a-decision")
	w.git("init", "--quiet")
	w.git("config", "user.email", "test@example.com")
	w.git("config", "user.name", "Test")
	w.commit()
	return w
}

func (w *repairWorkspace) write(name, body string) {
	w.t.Helper()
	path := filepath.Join(w.root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		w.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		w.t.Fatal(err)
	}
}

func (w *repairWorkspace) git(args ...string) { git(w.t, w.root, args...) }

func (w *repairWorkspace) commit() {
	w.t.Helper()
	w.git("add", "--all")
	w.git("commit", "--quiet", "--allow-empty", "-m", "corpus")
}

func (w *repairWorkspace) load() *corpus.Corpus {
	w.t.Helper()
	c, err := corpus.Load(filepath.Join(w.root, "docs"), w.root, corpus.Options{Gatherers: gather.Builtin()})
	if err != nil {
		w.t.Fatal(err)
	}
	return c
}

// ratify grants standing to what each artifact says now, which is what a person
// does and nothing else may.
func (w *repairWorkspace) ratify(ids ...string) {
	w.t.Helper()
	c := w.load()
	l := ledger.Open(w.store, run.Start("test-ratify", nil))
	for _, id := range ids {
		entry, found := c.Get(id)
		if !found {
			w.t.Fatalf("the fixture corpus holds no %s", id)
		}
		kind, _ := corpus.SplitRef(id)
		sc, known := c.Schemas.For(kind)
		if !known {
			w.t.Fatalf("the fixture corpus declares no %s schema", kind)
		}
		hash, err := canonical.Bind(entry, sc)
		if err != nil {
			w.t.Fatal(err)
		}
		if _, err := l.Ratify(id, hash.String(), hash.Norm, "Test <test@example.com>", "fixture", ledger.Where{}); err != nil {
			w.t.Fatal(err)
		}
	}
	if err := l.Close(run.Clean); err != nil {
		w.t.Fatal(err)
	}
}

func (w *repairWorkspace) release(id string) {
	w.t.Helper()
	c := w.load()
	entry, found := c.Get(id)
	if !found {
		w.t.Fatalf("the fixture corpus holds no %s", id)
	}
	kind, _ := corpus.SplitRef(id)
	sc, _ := c.Schemas.For(kind)
	hash, err := canonical.Bind(entry, sc)
	if err != nil {
		w.t.Fatal(err)
	}
	l := ledger.Open(w.store, run.Start("test-release", nil))
	if _, err := l.Release(id, hash.String(), hash.Norm, "Test <test@example.com>", "fixture", ledger.Where{}); err != nil {
		w.t.Fatal(err)
	}
	if err := l.Close(run.Clean); err != nil {
		w.t.Fatal(err)
	}
}

func (w *repairWorkspace) request(promise string) Request {
	w.t.Helper()
	c := w.load()
	acts, _, err := ledger.ReadActs(w.store)
	if err != nil {
		w.t.Fatal(err)
	}
	l := ledger.Open(w.store, run.Start("test-repair", nil))
	w.t.Cleanup(func() { _ = l.Close(run.Clean) })
	return Request{
		Workspace: w.root,
		Corpus:    c,
		Acts:      acts,
		Promise:   promise,
		Provider:  "",
		Deadlines: Deadlines{Gatherer: 60 * time.Second, Agent: time.Minute, Git: 60 * time.Second},
		Store:     w.store,
		Ledger:    l,
	}
}

const repairPromiseSchema = `type: promise
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

const repairPolicySchema = `type: policy
hash: [statement, applies_to, on_fail]
fields:
  statement:  { type: string, required: true }
  applies_to: { type: string }
  on_fail:    { type: string }
body: why
`

const repairDecisionSchema = `type: decision
hash: [statement]
fields:
  statement: { type: string }
body: why
`

const dispatchPolicyFile = `---
id: dispatch-is-bounded
type: policy
title: Dispatch is bounded
fields:
  statement: A repair changes only what the promise it serves bound.
  applies_to: dispatch
  on_fail: refuse
---

Why: a repair that reached past its bound would be work nobody authorized.
`

const decisionFile = `---
id: 0001-a-decision
type: decision
title: 0001 — A decision
fields:
  statement: A decision is not a promise and authorizes no repair.
---

## Context

Something had to be decided.

## Decision

It was.

## Consequences

None.
`

// twoInstrumentPromise is a rule with two dimensions, which is simply how one
// gets written: two instruments under two names, and a predicate that reads
// both. `a` counts and `b` only asserts the corpus loaded, so the halves can be
// made to disagree — which is the case a packet carrying one measurement could
// never state honestly.
const twoInstrumentPromise = `---
id: two-instruments
type: promise
title: Two instruments
fields:
  statement: this promise is measured twice
  gather:
    docs:
      gatherer: corpus
      scope: ["docs/**"]
    notes:
      gatherer: corpus
      root: notes
      scope: ["notes/**"]
  hold_when: obs.docs.artifacts == 8 and obs.notes.artifacts == 1
  trigger: cadence
  repair:
    scope: ["docs/**", "notes/**"]
---

Why: a rule with two dimensions is measured by two instruments.
`

const repairNoteSchema = `type: note
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
  statement: a note in the second corpus
---

Why: so the other half of a two-instrument promise has something to be about.
`
}

const uninstrumentedPromise = `---
id: uninstrumented
type: promise
title: Uninstrumented
fields:
  statement: nothing measures this
  trigger: cadence
  repair:
    scope: ["docs/**"]
---

Why: a promise nothing measures can state no gap and check no repair.
`

// promiseFile is one promise measured by `corpus`, which reads a corpus root and
// shells nothing out — so it answers identically on every machine and inside a
// box with no toolchain at all.
func promiseFile(slug, scope, hold string) string {
	return `---
id: ` + slug + `
type: promise
title: ` + slug + `
fields:
  statement: the corpus under ` + scope + ` is what it was
  gather:
    c:
      gatherer: corpus
      scope: ["` + scope + `"]
  hold_when: ` + hold + `
  trigger: cadence
  repair:
    scope: ["docs/**"]
---

Why: because a promise nobody can check is not one.
`
}

// TestAPromiseWithSeveralInstrumentsCarriesEveryMeasurement is the whole of what
// docs/decision/0061 changed: a promise measured two ways dispatches, and the
// packet carries both measurements under the names the promise gathers them by.
//
// It stops at the runtime, like TestTheBoxIsMeasuredBeforeAnythingIsDispatched:
// everything up to a live session is exercised here and costs nothing.
//
// The two halves are about two different corpora, so this is not one instrument
// counted twice. That matters: a packet that carried the failing half alone
// would state the gap and say nothing about what the repair must not break.
func TestAPromiseWithSeveralInstrumentsCarriesEveryMeasurement(t *testing.T) {
	provider := liveProvider(t)
	w := newRepairWorkspace(t)

	req := w.request("promise:two-instruments")
	req.Provider = filepath.Base(provider)
	req.Operator = config.Repair{Runtime: "claude", Executable: provider}

	res, err := Perform(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if res.Before.Value != model.NotKept {
		t.Fatalf("the two-instrument promise reads %s in the box: %s", res.Before.Value, res.Before.Detail)
	}
	if res.Packet.Empty() {
		t.Fatal("a promise naming two instruments assembled no packet, so it was refused rather than dispatched")
	}

	var wire struct {
		V       int `json:"v"`
		Promise struct {
			Gather map[string]json.RawMessage `json:"gather"`
		} `json:"promise"`
		Observations map[string]ledger.Observation `json:"observations"`
	}
	if err := json.Unmarshal(res.Packet.Bytes(), &wire); err != nil {
		t.Fatal(err)
	}
	if wire.V != thread.PacketVersion {
		t.Fatalf("the packet says v%d and this build assembles v%d", wire.V, thread.PacketVersion)
	}
	for name := range wire.Promise.Gather {
		observation, carried := wire.Observations[name]
		if !carried {
			t.Fatalf("the packet gathers %q and carries no measurement for it: %s", name, res.Packet.Bytes())
		}
		if observation.World.Tree == "" || len(observation.Data) == 0 {
			t.Fatalf("the measurement under %q arrived hollow: %#v", name, observation)
		}
	}
	if len(wire.Observations) != 2 {
		t.Fatalf("the packet carries %d measurements for a promise that names 2", len(wire.Observations))
	}

	// Two instruments about two corpora, so the evidence really is two
	// measurements rather than one filed twice.
	docs, notes := wire.Observations["docs"], wire.Observations["notes"]
	if docs.Address() == notes.Address() {
		t.Fatalf("both keys carry one measurement, addressed %s", docs.Address())
	}
	if docs.Data["artifacts"] == notes.Data["artifacts"] {
		t.Fatalf("both halves counted %v artifacts, so neither says anything the other does not", docs.Data["artifacts"])
	}

	// The half that holds is in the packet beside the half that does not, which
	// is the point: kept states the gap and never reads it for the agent.
	if fmt.Sprint(notes.Data["artifacts"]) != "1" {
		t.Fatalf("the holding half measured %v notes, want 1", notes.Data["artifacts"])
	}
	if res.Status != Refused || res.Condition != ledger.Runtime {
		t.Fatalf("the repair produced %s / %s: %s", res.Status, res.Condition, res.Detail)
	}
}

// TestMendingOneHalfAndBreakingTheOtherStillReadsNotKept is the fail-safe.
//
// The verdict is the whole predicate applied to the whole tree, so a session
// that closes the gap it was shown and opens another one somewhere else in the
// same promise's scope comes back not-kept — and `Perform` reads that as
// Unrepaired, which is the record saying which of the two it was. Nothing about
// this depends on what the session reported, because nothing reads what the
// session reported.
func TestMendingOneHalfAndBreakingTheOtherStillReadsNotKept(t *testing.T) {
	w := newRepairWorkspace(t)
	req := w.request("promise:two-instruments")

	// As authored: the docs half is short one artifact and the notes half holds.
	before, observations, err := derive(context.Background(), req, w.root)
	if err != nil {
		t.Fatal(err)
	}
	if before.Value != model.NotKept {
		t.Fatalf("the two-instrument promise reads %s as authored: %s", before.Value, before.Detail)
	}
	if len(observations) != 2 {
		t.Fatalf("the re-derivation produced %d measurements for a promise that names 2", len(observations))
	}
	if counted(observations, "docs") != "7" || counted(observations, "notes") != "1" {
		t.Fatalf("the halves measured %s and %s artifacts", counted(observations, "docs"), counted(observations, "notes"))
	}

	// Now mend the half the packet's gap was about, and break the other one in
	// the same act.
	w.write("docs/promise/spare.md", promiseFile("spare", "docs/**", "obs.c.ok"))
	w.write("notes/note/two.md", noteFile("two"))

	after, observations, err := derive(context.Background(), req, w.root)
	if err != nil {
		t.Fatal(err)
	}
	if counted(observations, "docs") != "8" {
		t.Fatalf("the docs half was not mended: %s", counted(observations, "docs"))
	}
	if counted(observations, "notes") != "2" {
		t.Fatalf("the notes half was not broken: %s", counted(observations, "notes"))
	}
	if after.Value != model.NotKept {
		t.Fatalf("a promise with one half mended and the other broken reads %s: %s", after.Value, after.Detail)
	}
}

// counted reads one half's artifact count as text, because a gatherer's count is
// an int in process and a float once it has been through a packet, and this test
// is about neither.
func counted(observations map[string]ledger.Observation, name string) string {
	return fmt.Sprint(observations[name].Data["artifacts"])
}

// TestAnAnchorIsAuthoredByTheOperatorAndFallsBackToTheRepository is
// docs/decision/0048 at the one place kept writes a commit: kept's own operator
// first, then the identity any commit made in this repository would carry
// anyway, and then nothing.
//
// Nothing is the important one. An identity kept invented would put a name
// nobody chose on a commit that can reach a branch, and an operating system's
// username is not an identity.
func TestAnAnchorIsAuthoredByTheOperatorAndFallsBackToTheRepository(t *testing.T) {
	w := newRepairWorkspace(t)

	who, err := identity("Paul <p@example.invalid>", w.root, time.Minute)
	if err != nil {
		t.Fatalf("identity from the operator: %v", err)
	}
	if who.Name != "Paul" || who.Email != "p@example.invalid" {
		t.Fatalf("identity = %+v", who)
	}

	// With nobody named, the identity Git itself would put on a commit made
	// here. The fixture sets it locally, as any working checkout does, and a
	// local value beats the machine's.
	who, err = identity("", w.root, time.Minute)
	if err != nil {
		t.Fatalf("identity from the repository: %v", err)
	}
	if who.Name != "Test" || who.Email != "test@example.com" {
		t.Fatalf("identity = %+v, want the repository's own", who)
	}

	// And where Git itself would refuse to author a commit, so does this. The
	// machine's own config is taken out of the way, because otherwise this
	// passes or fails depending on whose laptop is running it — which is a test
	// that proves nothing about the case it names.
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	if _, err := identity("", t.TempDir(), time.Minute); err == nil {
		t.Fatal("an identity was invented where Git itself would name none")
	}
}

// TestAnIdentityThatIsNotANameAndAnAddressIsNotOne: half an identity is not
// better than none, and a string git cannot be handed is not one either.
func TestAnIdentityThatIsNotANameAndAnAddressIsNotOne(t *testing.T) {
	for _, bad := range []string{"", "Paul", "p@example.invalid", "<p@example.invalid>", "Paul <>", "Paul <a b>", "Paul <a<b>"} {
		if _, _, ok := splitIdentity(bad); ok {
			t.Errorf("%q was read as a name and an address", bad)
		}
	}
	if name, email, ok := splitIdentity("  Paul Bleisch  <p@example.invalid>  "); !ok || name != "Paul Bleisch" || email != "p@example.invalid" {
		t.Errorf("splitIdentity = %q %q %v", name, email, ok)
	}
}
