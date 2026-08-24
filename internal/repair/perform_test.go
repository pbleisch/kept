package repair

import (
	"context"
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
		{"a packet carries one measurement", "promise:two-instruments", "and a packet carries one measurement"},
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
	w.write("docs/promise/counted.md", promiseFile("counted", "docs/**", "obs.c.artifacts == 6"))
	w.write("docs/promise/unratified.md", promiseFile("unratified", "docs/**", "obs.c.ok"))
	w.write("docs/promise/unbounded.md", strings.Replace(promiseFile("unbounded", "docs/**", "obs.c.ok"),
		"  repair:\n    scope: [\"docs/**\"]\n", "", 1))
	w.write("docs/promise/two-instruments.md", twoInstrumentPromise)

	w.ratify("promise:counted", "promise:unbounded", "promise:two-instruments", "policy:dispatch-is-bounded", "decision:0001-a-decision")
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
		if _, err := l.Ratify(id, hash.String(), hash.Norm, "Test <test@example.com>", "fixture"); err != nil {
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
	if _, err := l.Release(id, hash.String(), hash.Norm, "Test <test@example.com>", "fixture"); err != nil {
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

const twoInstrumentPromise = `---
id: two-instruments
type: promise
title: Two instruments
fields:
  statement: this promise is measured twice
  gather:
    a:
      gatherer: corpus
      scope: ["docs/**"]
    b:
      gatherer: corpus
      args: { root: docs }
      scope: ["docs/**"]
  hold_when: obs.a.ok and obs.b.ok
  trigger: cadence
  repair:
    scope: ["docs/**"]
---

Why: a packet carries one measurement.
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
