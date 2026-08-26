// Package land puts a gated repair on the landing target.
//
// It is docs/decision/0015's merge queue: the gate runs optimistically outside
// a lock, and a cheap check inside the lock decides whether it still applies.
//
//	outside the lock   gate the box against origin/main@X          minutes
//	inside the lock    fetch; is origin/main still X?              seconds
//	     still X       merge, push origin main, release            seconds
//	     moved to Y    release; re-gate against Y, or skip; retry
//
// The gate covers every standing artifact whose scope the change touches, not
// only the promise the thread was about, because a repair that satisfies A and
// breaks B would otherwise merge cleanly, pass its own gate, and land — which
// is the hole docs/decision/0037 closes.
//
// Nothing here is load-bearing on the lock. The push is unforced, so git
// rejects a non-fast-forward whatever this package believed about the target:
// the lock saves the cost of a gate that was going to be wasted, and git
// supplies the atomicity. That is docs/decision/0013's thesis, and it is why a
// lost lock is an ordinary outcome here rather than a failure.
package land

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/pbleisch/kept/internal/box"
	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/gather"
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/lock"
	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/progress"
	"github.com/pbleisch/kept/internal/register"
	"github.com/pbleisch/kept/internal/thread"
	"github.com/pbleisch/kept/internal/tick"
	"github.com/pbleisch/kept/internal/workspace"
)

// DefaultWait is how long a land waits for the lock before giving up.
//
// The section it guards is a fetch, a comparison, a push: seconds. Waiting
// minutes for it means the holder is wedged rather than busy, and
// docs/decision/0015 accepts that `flock` has no TTL and handles a wedged
// holder by giving up rather than by a lease nobody can see.
const DefaultWait = 2 * time.Minute

// DefaultAttempts is how many times a land re-gates after the target moved
// under it.
//
// Bounded rather than unbounded, because a target that moves faster than this
// repository can gate is a contention measurement rather than something to
// spin on — and an unbounded retry is the loop docs/decision/0037 refuses in
// its own domain. The disjoint-scope skip that would make most re-gates
// unnecessary is not built; this is the correct-but-slower path it optimizes.
const DefaultAttempts = 3

// Status is how far one land got.
type Status string

const (
	// Landed: the merge is on the target.
	Landed Status = "landed"
	// Refused: the gate said no, or the change reaches outside its bound, or
	// the lock could not be had. Condition names which. Nothing was pushed.
	Refused Status = "refused"
	// Prepared: the gate ran and passed and the land was not asked for.
	Prepared Status = "prepared"
)

// Condition names what refused, so a caller reports a cause rather than a
// message. The four are separate because they send a person to four different
// places.
type Condition string

const (
	// Scope: the change reaches outside the repair scope its promise bound.
	// See docs/policy/out-of-scope-paths-are-never-landed.md.
	Scope Condition = "scope"
	// Promise: the repair does not hold at the landing target, whatever it did
	// in its own box.
	Promise Condition = "promise"
	// Collateral: the repair holds and broke something else that has standing.
	Collateral Condition = "collateral"
	// Conflict: the change does not merge into the target without a person.
	Conflict Condition = "conflict"
	// Contended: the lock is held, or the target moved out from under every
	// attempt. Neither says anything is wrong.
	Contended Condition = "contended"
)

// Deadlines are what .kept/config.yaml says an external call may take.
type Deadlines struct {
	Gatherer time.Duration
	Git      time.Duration
}

// Request is one land a person asked for, by thread.
type Request struct {
	// Workspace is the repository the land runs in, and Corpus is what was
	// read out of it. The corpus that decides which promises are gated is the
	// operator's, not the box's: the box's would be whatever the repair left
	// behind, which is the thing being checked.
	Workspace string
	Corpus    *corpus.Corpus
	Acts      []ledger.Act

	// Thread is the repair being landed, and Target is where it goes.
	Thread string
	Target Target

	// By is the person performing it, as "Name <email>". A land is a person's
	// act and kept will not guess one.
	By string

	Deadlines Deadlines
	Exclude   []string

	Store  ledger.Store
	Ledger *ledger.Ledger

	Progress *progress.Display

	// Wait is how long to wait for the lock, and Attempts is how many times to
	// re-gate after the target moved. Zero uses the defaults.
	Wait     time.Duration
	Attempts int

	// DryRun gates and stops before the lock. Nothing is fetched inside a
	// lock, nothing is pushed and no act is recorded.
	DryRun bool
	// Keep retains the gate box instead of discarding it.
	Keep bool
}

func (r Request) wait() time.Duration {
	if r.Wait <= 0 {
		return DefaultWait
	}
	return r.Wait
}

func (r Request) attempts() int {
	if r.Attempts <= 0 {
		return DefaultAttempts
	}
	return r.Attempts
}

// gated is one run of the gate: what it produced, and the box it produced it in.
//
// The box is handed back rather than closed inside the gate, because whether it
// is worth keeping depends on something the gate does not know — a gate whose
// target moved under the lock is superseded, and retaining it would leave a
// registered worktree behind for every attempt.
type gated struct {
	box    *box.Box
	merged string
	// tree is kept's digest over the box *after* the merge, which is the tree
	// the gate measured and therefore the world this land was performed in.
	// Never the operator's workspace: that is a different tree, and recording
	// it would make the act name a world nothing was gated against.
	tree     string
	verdicts []Verdict
	review   box.Review
}

// Verdict is one artifact the gate measured, on both sides of the merge.
type Verdict struct {
	Artifact string
	Before   model.Value
	After    model.Value
	Detail   string
}

// Broke reports whether this artifact held at the target and does not hold with
// the change merged in.
//
// Only kept → not-kept. A promise that could not be measured afterwards is
// `unknown`, and reading that as a break is the one lie the three-valued model
// exists to prevent — one level up from the instrument, exactly as
// docs/policy/a-failed-gather-is-unknown.md has it at the instrument.
func (v Verdict) Broke() bool {
	return v.Before == model.Kept && v.After == model.NotKept
}

// Result is one land, whole.
type Result struct {
	Thread  string
	Promise string
	Target  Target
	Status  Status

	Condition Condition
	Detail    string

	// Anchor is what was landed, GatedAgainst is the target commit the gate
	// measured against, and Commit is the merge that reached the target.
	Anchor       thread.Anchor
	GatedAgainst string
	Commit       string
	// Tree is kept's digest over what the gate measured, which is what the act
	// records as the world this land was performed in.
	Tree string

	// Gate is every artifact the change's paths selected, measured on both
	// sides. Review is what landed measured against the promise's bound.
	Gate   []Verdict
	Review box.Review

	// Attempts is how many times the gate ran, and Moved is each target commit
	// that had moved by the time the lock was taken.
	Attempts int
	Moved    []string

	// Recorded reports whether the act was appended.
	Recorded bool
	Act      ledger.Act

	// Box is where the gate box is when it was kept, and Warning is something
	// that went wrong after the result was decided.
	Box     string
	Warning string
}

// Unlandable is a request that cannot be landed as it stands — no such thread,
// no anchor, a promise that has lost standing. Nothing about the machine or the
// moment would change it, so there is nothing worth writing into the ledger.
type Unlandable struct{ Why string }

func (u *Unlandable) Error() string { return u.Why }

func phase(req Request, words string, bound time.Duration) func() {
	return req.Progress.Begin(progress.Step{Kind: progress.Land, Detail: words, Bound: bound})
}

// Perform runs one land end to end.
//
// The order is docs/decision/0015's and the expensive half is deliberately
// outside the lock: fetch the target, cut a box at it, measure every promise
// the change touches, merge, measure them again, and only then take the lock to
// ask whether the target is still where the gate assumed. A gate inside the
// lock would cap landing at one per test run.
func Perform(ctx context.Context, req Request) (*Result, error) {
	if err := req.Target.Valid(); err != nil {
		return nil, &Unlandable{Why: err.Error()}
	}
	res := &Result{Thread: req.Thread, Target: req.Target}

	anchor, err := anchorOf(req)
	if err != nil {
		return nil, err
	}
	res.Anchor, res.Promise = anchor, anchor.Promise

	entry, found := req.Corpus.Get(anchor.Promise)
	if !found {
		return nil, &Unlandable{Why: fmt.Sprintf("the corpus holds no %s, which is what thread %s repaired", anchor.Promise, req.Thread)}
	}
	standing := standingOf(req, anchor.Promise)
	if standing.State != register.Ratified {
		// A repair is authorized by standing. Landing one whose promise has
		// been released or revised since would put a change on the target in
		// the name of something nobody is held to now.
		return nil, &Unlandable{Why: fmt.Sprintf("%s reads %s, and a land is authorized by standing", anchor.Promise, standing.State)}
	}
	who, err := identity(req.By)
	if err != nil {
		return nil, &Unlandable{Why: err.Error()}
	}

	for attempt := 1; attempt <= req.attempts(); attempt++ {
		res.Attempts = attempt
		done, err := attemptLand(ctx, req, res, entry.Artifact.RepairScope, who)
		if err != nil {
			return res, err
		}
		if done {
			return res, nil
		}
	}

	// Every attempt gated and every one found the target had moved by the time
	// it held the lock. Nothing is wrong: this is contention, and the answer is
	// to run it again.
	res.Status, res.Condition = Refused, Contended
	res.Detail = fmt.Sprintf("%s moved under all %s; nothing landed and nothing is wrong",
		req.Target, plural(res.Attempts, "attempt"))
	return res, nil
}

// attemptLand is one pass of the queue: gate outside the lock, check inside it.
// It reports whether the land is finished — landed, refused, or prepared — as
// against having lost the race and being worth another pass.
func attemptLand(ctx context.Context, req Request, res *Result, scope []string, who box.Identity) (bool, error) {
	fetching := phase(req, "fetch "+req.Target.String(), req.Deadlines.Git)
	target, err := fetchTarget(ctx, req)
	fetching()
	if err != nil {
		return true, err
	}
	res.GatedAgainst = target

	run, err := runGate(ctx, req, res, target, scope, who)
	// The box is this function's to dispose of from here, whatever the gate
	// returned: an attempt that is superseded takes its box with it.
	keep := false
	defer func() {
		if run == nil || run.box == nil {
			return
		}
		if keep && req.Keep {
			res.Box = run.box.Path()
			return
		}
		if err := run.box.Close(context.Background()); err != nil && res.Warning == "" {
			res.Warning = fmt.Sprintf("the gate box at %s could not be discarded, and is a registered worktree in this repository: %s", run.box.Path(), err)
		}
	}()
	if err != nil {
		keep = true
		if errors.Is(err, box.ErrConflicted) {
			res.Status, res.Condition = Refused, Conflict
			res.Detail = fmt.Sprintf("the change does not merge into %s without a person: %s", req.Target, err)
			return true, nil
		}
		return true, err
	}
	merged := run.merged
	res.Gate, res.Review, res.Tree = run.verdicts, run.review, run.tree
	keep = true

	if why, condition, refused := gateRefuses(res, req.Target); refused {
		res.Status, res.Condition, res.Detail = Refused, condition, why
		return true, nil
	}
	if req.DryRun {
		res.Status = Prepared
		res.Detail = fmt.Sprintf("the gate passed against %s at %s. Nothing was fetched under the lock, nothing was pushed.",
			req.Target, short(target))
		return true, nil
	}

	// The critical section. Everything above took minutes; this takes seconds,
	// and it is the only part that has to be serialized.
	locking := phase(req, "take the land lock", req.wait())
	held, err := lock.Acquire(ctx, req.Workspace, lock.Land, req.wait())
	locking()
	if err != nil {
		if errors.Is(err, lock.ErrRefused) {
			res.Status, res.Condition = Refused, Contended
			res.Detail = fmt.Sprintf("another run held the land lock for %s: %s", req.wait(), err)
			return true, nil
		}
		return true, err
	}
	defer func() {
		if err := held.Release(); err != nil && res.Warning == "" {
			res.Warning = err.Error()
		}
	}()

	// The check that makes the invariant true rather than merely locally
	// consistent. It reads the *tracking* ref after a fetch, never the local
	// branch: a local lock excludes local runs and nothing else, so the
	// operator pushing from a laptop, CI, or a second machine is exactly what a
	// check against refs/heads/main would miss. See docs/decision/0015.
	rechecking := phase(req, "check "+req.Target.String()+" has not moved", req.Deadlines.Git)
	now, err := fetchTarget(ctx, req)
	rechecking()
	if err != nil {
		return true, err
	}
	if now != target {
		res.Moved = append(res.Moved, now)
		return false, nil
	}

	pushing := phase(req, "push "+short(merged)+" to "+req.Target.String(), req.Deadlines.Git)
	err = push(ctx, req, merged)
	pushing()
	if err != nil {
		return true, err
	}
	res.Status, res.Commit = Landed, merged

	if err := recordLand(ctx, req, res, who); err != nil {
		// The push happened and the record did not, which is the recoverable
		// half of docs/decision/0018's pair. A process killed between these two
		// lines cannot report anything at all, so what makes the loss survivable
		// is that nothing reads the ledger to answer the question: the next tick
		// observes the tree and re-derives, and the land run again records what
		// is already on the target. Both are killed rather than argued in
		// crash_test.go. This branch is the case where the run survives its own
		// failure, and reporting is all it may do — the alternative is a ledger
		// silently missing an act whose effect is already on the target.
		res.Warning = fmt.Sprintf("%s is on %s and the act was not recorded: %s", short(merged), req.Target, err)
	}
	return true, nil
}

// gateRefuses reads the gate. The order is deliberate: the change's own bound
// first, because a change that reaches outside it may not land whatever any
// promise says about it.
func gateRefuses(res *Result, target Target) (string, Condition, bool) {
	if !res.Review.Bounded() {
		return fmt.Sprintf("the change touches %s outside the repair scope %s bound, and nothing that reaches past its bound may land",
			plural(len(res.Review.Outside), "path"), res.Promise), Scope, true
	}
	for _, v := range res.Gate {
		if v.Artifact != res.Promise {
			continue
		}
		if v.After != model.Kept {
			return fmt.Sprintf("%s reads %s against %s with this change merged in, whatever it read in its own box",
				res.Promise, v.After, target), Promise, true
		}
	}
	var broken []string
	for _, v := range res.Gate {
		if v.Artifact != res.Promise && v.Broke() {
			broken = append(broken, v.Artifact)
		}
	}
	if len(broken) > 0 {
		// A repair that satisfied its own promise and broke another. One
		// occurrence is a bad repair rather than a conflict, and it is the next
		// packet's business to say so under prior attempts. See
		// docs/decision/0037.
		return fmt.Sprintf("%s holds, and this change breaks %s that %s: %s",
			res.Promise, plural(len(broken), "promise"), heldAt(len(broken), target), strings.Join(broken, ", ")), Collateral, true
	}
	return "", "", false
}

func heldAt(n int, target Target) string {
	if n == 1 {
		return "holds on " + target.String()
	}
	return "hold on " + target.String()
}

// runGate cuts a box at the target, measures every promise the change touches,
// merges, and measures them again.
//
// Both sides, because "a collateral break" is a promise that held on the target
// and does not hold with this change in it. Measuring only afterwards would
// refuse every land into a repository that already has a broken promise, which
// is a gate that reports the state of the target rather than the effect of the
// change.
func runGate(ctx context.Context, req Request, res *Result, target string, scope []string, who box.Identity) (*gated, error) {
	cutting := phase(req, "cut a gate box at "+req.Target.String(), req.Deadlines.Git)
	gate, err := box.Create(ctx, box.Request{
		Repository: req.Workspace,
		Commit:     target,
		Deadline:   req.Deadlines.Git,
	})
	cutting()
	if err != nil {
		return nil, err
	}
	run := &gated{box: gate}

	// The change's own paths, read before the merge because the selection has
	// to exist before the first measurement. It is the wider of the two path
	// sets — what lands is a subset once the target already holds some of it —
	// and wider is the safe direction for deciding what to gate.
	//
	// That it is wider is a property of the merge stating this same base: the
	// merge applies base → anchor and nothing else, so no path can land that
	// this selection did not see. Under a merge whose base git computed, the
	// operator's own unlanded commits landed without ever being selected.
	changed, err := gate.Names(ctx, res.Anchor.Parent, res.Anchor.Commit)
	if err != nil {
		return run, err
	}
	subjects := selected(req, changed, res.Promise)

	before, err := measureAll(ctx, req, gate.Path(), subjects)
	if err != nil {
		return run, err
	}

	// The anchor's own base, stated rather than left to the graph. Merging the
	// anchor would take its ancestry with it — an operator's unlanded commits,
	// or the snapshot a dirty repair takes of their workspace — and what lands
	// has the target's tip as its only parent. See docs/decision/0068.
	merging := phase(req, "merge the repair into "+req.Target.String(), req.Deadlines.Git)
	message := fmt.Sprintf("land %s\n\nthread %s, gated against %s at %s.\n",
		res.Promise, req.Thread, req.Target, short(target))
	merged, err := gate.Merge(ctx, res.Anchor.Parent, res.Anchor.Commit, message, who)
	merging()
	if err != nil {
		return run, err
	}
	run.merged = merged

	// The tree the gate is about to measure, which is the world this land is
	// performed in. Walked once, here, because after this the box is discarded.
	walked, err := workspace.Read(gate.Path(), workspace.Options{
		Exclude: req.Exclude,
		Workers: runtime.NumCPU(),
		All:     true,
	})
	if err != nil {
		return run, err
	}
	run.tree = walked.Digest()

	after, err := measureAll(ctx, req, gate.Path(), subjects)
	if err != nil {
		return run, err
	}

	// What actually lands, which is what the scope review is about: the target
	// may already hold part of the change.
	landing, err := gate.Names(ctx, target, merged)
	if err != nil {
		return run, err
	}
	run.review = box.Change{Paths: landing}.Against(scope)

	run.verdicts = make([]Verdict, 0, len(subjects))
	for _, id := range subjects {
		run.verdicts = append(run.verdicts, Verdict{
			Artifact: id,
			Before:   before[id].Value,
			After:    after[id].Value,
			Detail:   after[id].Detail,
		})
	}
	return run, nil
}

// selected is every standing artifact whose scope these paths touch, plus the
// promise the thread was about.
//
// An artifact that gathers nothing is skipped: it binds without being
// evaluated, so no change to a file can make it stop holding and measuring it
// would produce nothing to compare. An artifact whose gather states no scope
// covers the whole workspace, which puts it in the set — that is the safe
// direction and it is the same one docs/decision/0037 takes for an unknown
// scope.
func selected(req Request, paths []string, promise string) []string {
	standing := map[string]bool{}
	for _, s := range register.Derive(req.Corpus, req.Acts) {
		if s.State == register.Ratified {
			standing[s.Artifact] = true
		}
	}

	chosen := map[string]bool{promise: true}
	for _, e := range req.Corpus.All() {
		art := e.Artifact
		if !standing[art.ID] || len(art.Gather) == 0 {
			continue
		}
		if touches(art, paths) {
			chosen[art.ID] = true
		}
	}
	out := make([]string, 0, len(chosen))
	for id := range chosen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func touches(art *model.Artifact, paths []string) bool {
	for _, spec := range art.Gather {
		globs := gather.Globs(spec.Args)
		for _, path := range paths {
			if workspace.Match(globs, path) {
				return true
			}
		}
	}
	return false
}

// measureAll re-derives each artifact in the box.
//
// tick.Measure rather than a tick: it gathers one artifact, applies its
// predicate and writes nowhere. A tick would measure the whole corpus, and
// opening a ledger under internal/ is the shape
// promise:standing-is-minted-in-one-place refuses by name. A verdict reached in
// a box belongs to the box (docs/decision/0035), and none of these is recorded
// as truth.
func measureAll(ctx context.Context, req Request, root string, subjects []string) (map[string]tick.Answer, error) {
	corpusRoot, workspaceRoot, err := corpus.FindRoot(root)
	if err != nil {
		return nil, fmt.Errorf("read the gate box's corpus: %w", err)
	}
	c, err := corpus.Load(corpusRoot, workspaceRoot, corpus.Options{Gatherers: gather.Builtin()})
	if err != nil {
		return nil, fmt.Errorf("read the gate box's corpus: %w", err)
	}

	out := make(map[string]tick.Answer, len(subjects))
	for _, id := range subjects {
		if _, found := c.Get(id); !found {
			// The change removed the artifact from the corpus. That is not a
			// verdict: nothing measured it, and an absent artifact is
			// docs/decision/0021's business rather than the gate's.
			out[id] = tick.Answer{Value: model.Unknown, Detail: "the change leaves no " + id + " in the corpus"}
			continue
		}
		answer, _, err := tick.Measure(ctx, tick.MeasureRequest{
			Root:      root,
			Corpus:    c,
			Artifact:  id,
			Catalogue: gather.Builtin(),
			Deadline:  req.Deadlines.Gatherer,
			Exclude:   req.Exclude,
			Workers:   runtime.NumCPU(),
			Acts:      req.Acts,
			Progress:  req.Progress,
		})
		if err != nil {
			return nil, err
		}
		out[id] = answer
	}
	return out, nil
}

// recordLand appends the act, after the push. Record only what has already
// happened: an act written before the push would claim a land that a rejected
// push never performed.
func recordLand(ctx context.Context, req Request, res *Result, who box.Identity) error {
	if req.Ledger == nil {
		return nil
	}
	standing := standingOf(req, res.Promise)
	where, err := situate(req, res)
	if err != nil {
		return err
	}
	act, err := req.Ledger.Land(res.Promise, standing.Hash.String(), standing.Hash.Norm, req.By,
		fmt.Sprintf("thread %s, gated against %s at %s", req.Thread, req.Target, short(res.GatedAgainst)), where,
		landing(req))
	if err != nil {
		return err
	}
	res.Act, res.Recorded = act, true
	return nil
}

// landing is what this land says about how it arrived: docs/decision/0015's
// three fields, and the ledger refuses an act missing any of them.
//
// All three are stated here rather than derived, because there is nothing yet
// to derive them from. [Origin] is the one target constructor and it names a
// remote, so every target this build can hold is shared; and a ref advance is
// the only arrival built, so [attemptLand] pushes and there is no other path to
// this line. When a local target and a worktree arrival arrive they are read
// off the request here, and the ledger's refusal is what catches a pair that
// contradicts itself — regime shared with a worktree arrival, or an arrival
// that disagrees with Where.Uncommitted.
func landing(req Request) ledger.Landing {
	return ledger.Landing{
		Target:  req.Target.String(),
		Regime:  ledger.SharedRegime,
		Arrival: ledger.RefArrival,
	}
}

// situate is the world a land was performed in, which is the tree it was gated
// against — 0015 asks a land to record exactly that, and 0014's world is the
// shape it is recorded in.
//
// The tree is the *gate box's*, walked after the merge, and never the
// operator's workspace: those are two different trees, and recording the second
// would make the act name a world nothing was gated against — which is the one
// thing the world on a record exists to prevent.
//
// Commit is the merge that reached the target, and Ref and Remote name the
// target it reached. That is where the act carries what it landed onto: the
// gate box is detached and would name no ref, and "no ref" would lose the one
// thing about a land that is not derivable from anything else in the record.
func situate(req Request, res *Result) (ledger.Where, error) {
	if res.Tree == "" {
		return ledger.Where{}, errors.New("the gate recorded no tree, so this land can name no world")
	}
	remote, _ := remoteURL(req)
	return ledger.Where{World: model.World{
		Remote: remote,
		Ref:    req.Target.Ref(),
		Commit: res.Commit,
		Tree:   res.Tree,
		Thread: req.Thread,
	}}, nil
}

func standingOf(req Request, artifact string) register.Standing {
	for _, s := range register.Derive(req.Corpus, req.Acts) {
		if s.Artifact == artifact {
			return s
		}
	}
	return register.Standing{}
}

// anchorOf reads what the thread produced. A thread with no anchor is a repair
// that changed nothing, or one that ran before anchoring existed; either way
// there is nothing to land and saying so is the whole answer.
func anchorOf(req Request) (thread.Anchor, error) {
	entries, _, err := thread.ReadReceipt(req.Store, thread.ID(req.Thread))
	if err != nil && !errors.Is(err, ledger.ErrNoStream) {
		return thread.Anchor{}, &Unlandable{Why: fmt.Sprintf("read thread %s: %s", req.Thread, err)}
	}
	anchor, found := thread.LatestAnchor(entries)
	if !found {
		return thread.Anchor{}, &Unlandable{Why: fmt.Sprintf("thread %s anchored no commit, so there is nothing to land", req.Thread)}
	}
	return anchor, nil
}

// identity is who performs the land. Unlike a repair's anchor, which falls back
// to the repository's own Git identity, a land refuses without one: it is an
// act in the ledger, and docs/decision/0048 is explicit that kept will not
// guess who performed one.
func identity(by string) (box.Identity, error) {
	who := strings.TrimSpace(by)
	open := strings.LastIndex(who, "<")
	if open <= 0 || !strings.HasSuffix(who, ">") {
		return box.Identity{}, fmt.Errorf("a land records who performed it, and %q is not a name and an address", by)
	}
	name := strings.TrimSpace(who[:open])
	email := strings.TrimSpace(who[open+1 : len(who)-1])
	if name == "" || email == "" {
		return box.Identity{}, fmt.Errorf("a land records who performed it, and %q is not a name and an address", by)
	}
	return box.Identity{Name: name, Email: email}, nil
}

func short(commit string) string {
	if len(commit) < 7 {
		return commit
	}
	return commit[:7]
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
