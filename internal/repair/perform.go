package repair

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/pbleisch/kept/internal/assess"
	"github.com/pbleisch/kept/internal/box"
	"github.com/pbleisch/kept/internal/config"
	"github.com/pbleisch/kept/internal/confine"
	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/gather"
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/progress"
	"github.com/pbleisch/kept/internal/register"
	"github.com/pbleisch/kept/internal/thread"
	"github.com/pbleisch/kept/internal/tick"
	"github.com/pbleisch/kept/internal/workspace"
)

// canaryDeadline bounds verifying the provider, which is the pre-flight and the
// canary together.
//
// It is not a configuration key and should not become one. What it bounds is
// kept asking a provider about a policy kept just rendered — the same work for
// every repair on a machine, with no per-repository answer — and a knob for it
// would be a knob for how long to wait before deciding the jail is broken. The
// deadlines an operator does set bound the things that vary: the gatherer, the
// agent, and Git. See docs/decision/0026.
const canaryDeadline = 2 * time.Minute

// dispatchMargin is how much longer the parent's own context runs than the
// deadline it gives the session. The supervisor's timeout is what kills the
// child; this is only so the outer context cannot cancel the kill, the reap and
// the final receipt appends it has to perform afterwards.
const dispatchMargin = 2 * time.Minute

// Status is how far one repair got.
//
// The six that are not a completed dispatch are as much a result as the one
// that is, and each sends a reader somewhere different. A repair that was
// refused, one that found nothing to repair and one that ran and did not work
// look identical to a boolean, and telling them apart is the whole of what a
// person asked for.
type Status string

const (
	// Repaired: a session ran, and re-deriving the promise from the box says it
	// holds there. The verdict is the re-derivation's and never the session's.
	Repaired Status = "repaired"
	// Unrepaired: a session ran and the promise still does not hold.
	Unrepaired Status = "unrepaired"
	// Undecided: a session ran and nothing could measure the promise afterwards.
	Undecided Status = "undecided"
	// Holds: the promise holds at the commit the box was cut from, so there is
	// no gap for a packet to state. Drift in the working tree that is not in the
	// commit reads like this, and so does a verdict nothing has refreshed.
	Holds Status = "holds"
	// Unmeasured: nothing could measure the promise in the box, so there is no
	// evidence to carry. A failed gather is unknown and never not-kept, one
	// level up as well as at the instrument.
	Unmeasured Status = "unmeasured"
	// Refused: a condition refused, and Condition names which. Nothing started.
	Refused Status = "refused"
	// Prepared: the dispatch was not asked for. Everything up to it happened,
	// including the canary and the pre-flight.
	Prepared Status = "prepared"
	// Exhausted: a session ran and ended because the account it ran on had no
	// capacity left, in the runtime's own words. It is not a result: nothing is
	// re-derived, nothing is anchored, no pass argues about it, and the episode
	// waits for the account rather than reworking. Running out is an expected
	// way for a session to end, and what it left is half of something. See
	// docs/decision/0090.
	Exhausted Status = "exhausted"
)

// Deadlines are what .kept/config.yaml says an external call may take. They are
// carried as a struct rather than read here so that this package has exactly one
// source for them and the command line has the other half of the same file. See
// docs/decision/0026.
type Deadlines struct {
	Gatherer time.Duration
	Agent    time.Duration
	Git      time.Duration
}

// Request is one repair a person asked for, by name.
//
// One promise, named. Choosing which of several drifts to spend a live session
// on is a ranking and a budget, and neither exists; until they do the cut is a
// person naming one. See docs/decision/0060.
type Request struct {
	// Workspace is the repository root the box is cut from, and Corpus is what
	// was read out of it.
	Workspace string
	Corpus    *corpus.Corpus
	// Acts is the folded act stream standing is derived from.
	Acts []ledger.Act
	// Promise is the artifact id, already resolved to its "type:slug" form.
	Promise string

	// By is the person kept knows about, as "Name <email>", or empty where the
	// machine names none. It authors the anchor commit, because a commit that
	// can reach a branch is authored by a person and never by a tool or the
	// agent that produced the diff — see AGENTS.md.
	//
	// It is not required. A repair with nobody named still runs and still
	// reports; it is simply not anchored, and therefore not landable, which is
	// a smaller cost than refusing to repair at all on a machine that has never
	// needed an identity. See docs/decision/0048.
	By string

	// Provider is .kept/config.yaml's confine.provider, and Operator is the
	// per-machine repair runtime. The two halves are deliberately in different
	// files: one is the repository's policy and the other names one person's
	// executable and one person's credential. See docs/decision/0048.
	Provider  string
	Operator  config.Repair
	Deadlines Deadlines
	Exclude   []string

	// Assessor is the per-machine runtime the adversarial pass spends, selected
	// independently of Operator above — which is the whole of what
	// docs/decision/0078 asks for, since two instances of one model can find one
	// substitution reasonable. An unset block is a machine with no assessor
	// configured, and the pass reports that rather than failing.
	Assessor config.Assess

	// Store and Ledger are where the thread receipt and the disposition go.
	Store  ledger.Store
	Ledger *ledger.Ledger

	// Progress is where this repair says what it is doing while it does it, and
	// nil is a repair nobody is watching. It is presentation only, so nothing
	// here reads it back and no record depends on it. See docs/decision/0065.
	Progress *progress.Display

	// Events is told as each phase of an episode finishes, and nil is a run
	// nobody is watching. It is presentation's input too, and nothing reads an
	// event back. See [Event] and docs/decision/0086.
	Events func(Event)

	// Keep retains the box after the repair instead of discarding it, for a
	// person who wants to work in the tree rather than read the diff.
	Keep bool
	// DryRun stops after the canary and the pre-flight, with the packet
	// assembled and nothing dispatched.
	DryRun bool
}

// Result is one repair, whole: what kept did, what it recorded about it, and
// what a person reviews.
type Result struct {
	Promise string
	Status  Status

	// Outcome, Condition and Detail are what docs/decision/0019 records about
	// this drift, and Recorded reports whether the record was appended — the
	// same refusal on the same grounds is one record and a heartbeat.
	Outcome     ledger.Outcome
	Condition   ledger.Condition
	Detail      string
	Disposition ledger.Disposition
	Recorded    bool

	// Commit is what the box was cut at, Box is where it is when it was kept,
	// and Grants is the write grant the repair scope was rendered as. The last
	// is reported because it is always at least as wide as the scope, and a
	// person reading a bounded review should be able to see what confinement
	// actually permitted. See docs/decision/0059.
	Commit string
	Box    string
	Grants []string

	Packet thread.Packet
	Thread thread.ID

	// RuntimeAfter is what the runtime's executable reported itself as once
	// the session was over, asked only when the file at its path was not the
	// one launched. See docs/decision/0085.
	RuntimeAfter string

	// Anchor is the commit the session's work was committed as, and the ref
	// holding it against gc. It is what makes a repair landable: without it the
	// tree is discarded with the box and the diff is printed and dropped. Zero
	// when the session changed nothing, or when no session ran.
	Anchor    thread.Anchor
	AnchorRef string

	// Before and After are the promise re-derived in the box, on either side of
	// the session. Neither reads anything the session reported.
	Before tick.Answer
	After  tick.Answer

	Change box.Change
	Review box.Review

	// Argument is the adversarial pass over this thread, and nil is a repair
	// that never reached one — refused, holding, unmeasured, or stopped at a dry
	// run. A pass that did not run because nobody configured one is a result
	// with a status saying so, which is a different answer from no field at
	// all. See docs/decision/0078 and 0081.
	Argument *ArgueResult

	// Warning is something that went wrong after the result was decided and
	// does not change it — a box that would not discard. It is printed and
	// never recorded: a disposition is about the drift.
	Warning string

	// Attempt is this attempt's place in the run that opened it, from 1, and
	// Rework says it continued earlier work in its thread.
	Attempt int
	Rework  bool
	// Unchanged says the change the session left was read and was nothing, so
	// there was nothing to anchor. It is separate from an anchor that is missing,
	// which may be one that could not be written.
	Unchanged bool

	// Exhausted is the runtime's account of why a session it ended had no
	// capacity left, and nil for every session that did not end that way. It is
	// set exactly when Status is [Exhausted] for the repair session, and on
	// [ArgueResult] for a pass.
	Exhausted *thread.Exhaustion
	// Regressed says this attempt was a rework cut at an anchor that holds, and
	// what it left does not, so its work was not anchored: the thread's anchor
	// stays at the one it was cut at, and the box is kept. See
	// docs/decision/0090.
	Regressed bool
}

// Phase is where a repair that had opened its thread stopped.
//
// Four, because each sends a person somewhere different: a session that never
// started spent nothing and left nothing, and the other three ran a session and
// left its work in a box.
type Phase string

const (
	// PhaseLaunch: the session did not start.
	PhaseLaunch Phase = "launch"
	// PhaseSession: the session ran, and what it wrote or how it exited could
	// not be retained.
	PhaseSession Phase = "session"
	// PhaseRecord: the session ran, and recording that it had failed.
	PhaseRecord Phase = "record"
	// PhaseRederive: the session ran, and measuring the promise in the box
	// afterwards failed.
	PhaseRederive Phase = "re-derive"
)

// Failure is a repair that failed after it opened a thread, which is every
// failure a person has a receipt to read about.
//
// It carries only what the receipt cannot say: which phase this run was in, and
// where the box was left. Everything about the session itself — how far it got,
// how it ended, what it last wrote, whether its binary was replaced — is read
// back from the receipt by whatever reports it, so the terminal and a later
// `kept review` tell it the same way. See docs/decision/0085.
type Failure struct {
	Phase   Phase
	Promise string
	Thread  thread.ID
	// Box is where the box was kept, and empty when it was discarded because
	// the session never ran in it.
	Box string
	// RuntimeAfter is what the runtime's executable reported itself as once
	// the session was over, asked only when the file at its path was not the
	// one launched. Empty otherwise.
	RuntimeAfter string
	Err          error
}

func (f *Failure) Error() string {
	if f.Box == "" {
		return f.Err.Error()
	}
	return fmt.Sprintf("%s; the box the session ran in is kept at %s: git -C %s diff reads what it left, and git worktree remove --force %s discards it",
		f.Err, f.Box, f.Box, f.Box)
}

func (f *Failure) Unwrap() error { return f.Err }

// warn adds to what went wrong after the result was decided. Warnings
// accumulate rather than replace one another: a repair whose output ended
// mid-item and whose anchor could not be written has two things to say, and the
// second must not erase the first.
func (r *Result) warn(message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	if r.Warning == "" {
		r.Warning = message
		return
	}
	r.Warning += "; " + message
}

// undecodedWarning says that a session's receipt holds output that was not an
// event, and nothing when it holds none. It changes no verdict — the promise is
// measured from the tree — and it is said because a receipt that stops short
// and one that is whole read alike otherwise. See docs/decision/0084.
func undecodedWarning(receipt *thread.Receipt) string {
	items, unterminated := receipt.Undecoded()
	switch {
	case items == 0:
		return ""
	case unterminated && items == 1:
		return fmt.Sprintf("thread %s's output ended partway through a line, which its receipt keeps as it arrived", receipt.ID())
	case unterminated:
		return fmt.Sprintf("thread %s's output held %d lines that were not events, the last cut off where the output ended; its receipt keeps them as they arrived", receipt.ID(), items)
	default:
		return fmt.Sprintf("thread %s's output held %d %s that %s not events; its receipt keeps them as they arrived", receipt.ID(), items, plural(items, "line", "lines"), plural(items, "was", "were"))
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// Dispatched reports whether a session ran.
func (r *Result) Dispatched() bool {
	return r.Status == Repaired || r.Status == Unrepaired || r.Status == Undecided
}

// Unrepairable is a promise that cannot be repaired as it is authored: it has no
// standing, it bounds nothing, or a packet cannot state its gap. It is separate
// from a refusal because nothing about the machine or the moment would change
// it, so there is nothing about it worth writing into the ledger.
type Unrepairable struct{ Why string }

func (e *Unrepairable) Error() string { return e.Why }

// phase says that a repair has started one of its steps, and returns the
// function that says it finished.
//
// A repair reports phases rather than instruments because that is what a repair
// is made of, and it reports these ones because measurement says that is where
// the time goes. Over this repository a dry run is 203ms cutting the box, 27.4s
// measuring the promise in it, 13ms locating the runtime, 7ms rendering the
// profile and 885ms verifying the provider — and the session that follows a
// real one was measured between 18 and 63 seconds against a bound of sixty
// minutes. The three-figure milliseconds are reported anyway: each of them
// starts a program that can hang, and the live form draws none of them unless
// one does.
//
// The bound is the deadline that will actually fire, which is the whole point
// of showing one. A phase with none says none rather than borrowing a
// neighbour's. See docs/decision/0065.
func phase(req Request, words string, bound time.Duration) func() {
	return req.Progress.Begin(progress.Step{Kind: progress.Repair, Detail: words, Bound: bound})
}

// Perform runs one repair end to end: cut a box at the workspace's commit,
// measure the promise there, assemble the packet, dispatch a confined session,
// re-derive the promise from the box, and review what changed against the scope
// the promise bound.
//
// It lands nothing. A repair produces a bounded diff a person reads, and the
// merge is their hands on git. See docs/increments.md.
//
// The order is deliberate. The gap is established in the box before anything is
// recorded and before a session starts, because a packet states a gap
// (docs/decision/0009) and a promise that holds at the commit has none — and
// because a disposition about a drift kept never established would be a record
// of a decision nobody made.
func Perform(ctx context.Context, req Request) (*Result, error) {
	return attempt(ctx, req, 1, nil)
}

// attempt is one attempt of a repair episode: [Perform] for the first, and a
// rework for a later one when plan is set.
//
// A rework differs in five places and nowhere else. Its thread is the episode's
// rather than a fresh one; its box is cut at the tip the earlier attempts left
// rather than at the workspace's HEAD; a promise that holds there is still
// dispatched, because what its packet states is the gap of a second kind; its
// packet carries the earlier attempts and travels with the rework ask; and its
// anchor is committed onto the episode's base. See docs/decision/0086.
func attempt(ctx context.Context, req Request, number int, plan *reworkPlan) (_ *Result, failure error) {
	res := &Result{Promise: req.Promise, Attempt: number, Rework: plan != nil}
	entry, standing, err := subject(req)
	if err != nil {
		return nil, err
	}
	art := entry.Artifact

	provider, err := providerExecutable(req.Provider)
	if err != nil {
		return refuse(req, res, standing, ledger.Provider, err.Error())
	}
	if why := runtimeUnready(req.Operator); why != "" {
		return refuse(req, res, standing, ledger.Runtime, why)
	}
	if req.Deadlines.Agent <= 0 {
		// Nothing external runs without a deadline, so with none there is
		// nothing to run. It is asked here rather than at the launch because
		// the answer is in a file and does not need a box and a canary first.
		// See docs/decision/0026.
		return refuse(req, res, standing, ledger.Runtime,
			"timeouts.agent is unset in .kept/config.yaml, and nothing external runs without a deadline")
	}

	// The thread is minted before the box, because the box is the thread's and
	// everything measured in it names the thread it was measured in. The receipt
	// adopts this id rather than minting its own: a receipt is opened once there
	// is something to record, and a repair that holds, refuses or stops at a dry
	// run records nothing and leaves the id naming a box that was discarded. See
	// docs/decision/0080.
	id := thread.MintID()
	if plan != nil {
		id = plan.Thread
	}

	var (
		created *box.Box
		home    string
	)
	if plan == nil {
		cutting := phase(req, "cut a box at the workspace's commit", req.Deadlines.Git)
		created, home, err = cut(ctx, req)
		cutting()
	} else {
		// The tree the earlier attempts left, so the session starts inside the
		// work rather than rebuilding it. See docs/decision/0086.
		cutting := phase(req, "cut a box at the work thread "+string(plan.Thread)+" left", req.Deadlines.Git)
		created, home, err = cutAt(ctx, req.Workspace, plan.Tip, req.Deadlines.Git)
		cutting()
	}
	if err != nil {
		return nil, err
	}
	req.emit(Event{Kind: AttemptStarted, Attempt: number, Thread: id, Commit: created.Commit(), Rework: plan != nil})
	// holdsWork says the box holds something a session did that nothing else
	// holds. It is set once a session has run in the box and cleared once what
	// the session left is anchored, or is known to be nothing.
	holdsWork := false
	discard := func() {
		// The session's private state goes either way: it is the runtime's
		// home and the box's scratch, both discarded with the session, and
		// neither is part of the tree a person keeps a box to read.
		_ = os.RemoveAll(home)
		// A box whose session's work is not anchored is the only copy of that
		// work — an error before the anchor, an anchor that would not be
		// written, a change that could not be read — so it stays, and whatever
		// reports the repair says where. A box is disposable once its work is
		// anchored or there was never any. See docs/decision/0084.
		stranded := holdsWork && res.Anchor.Commit == ""
		if req.Keep || stranded {
			_ = os.RemoveAll(created.Temp())
			res.Box = created.Path()
			var failed *Failure
			if stranded && errors.As(failure, &failed) {
				failed.Box = res.Box
			}
			return
		}
		if err := created.Close(context.Background()); err != nil {
			// A box that will not close is worth saying out loud rather than
			// swallowing: what is left behind is a registered worktree in the
			// operator's repository.
			res.warn(fmt.Sprintf("the box at %s could not be discarded, and is a registered worktree in this repository: %s", created.Path(), err))
		}
	}
	defer discard()
	res.Commit = created.Commit()

	var lineage map[string][]model.Observation
	if plan != nil {
		lineage = plan.Lineage
	}
	measuring := phase(req, "measure "+req.Promise+" in the box", 0)
	before, observations, err := derive(ctx, req, created.Path(), id, lineage)
	measuring()
	if err != nil {
		return nil, err
	}
	res.Before = before
	unmeasured := ""
	switch before.Value {
	case model.Kept:
		// A rework is dispatched on a promise that holds: what its packet
		// states is that the way the earlier work got there was not accepted.
		// A first attempt has no such gap to state. See docs/decision/0079.
		if plan == nil {
			res.Status = Holds
			return res, nil
		}
	case model.NotKept:
	default:
		// A gatherer that could not run in the box is not a broken promise, and
		// there is nothing for a packet to state. See
		// policy:a-failed-gather-is-unknown.
		if plan == nil {
			res.Status = Unmeasured
			return res, nil
		}
		// A rework whose box nothing can measure is the earlier attempt having
		// left the promise unmeasurable, and the packet says so rather than
		// carrying a measurement from somewhere else.
		unmeasured = strings.TrimSpace(before.Detail)
		if unmeasured == "" {
			unmeasured = "nothing could measure the promise in this box"
		}
	}

	packet, err := assemblePacket(req, entry, standing, recorded(observations), plan.evidence(unmeasured))
	if err != nil {
		return nil, err
	}
	res.Packet = packet

	assembling := phase(req, "locate and check the "+string(req.Operator.Runtime)+" runtime", 0)
	session := Session{Home: home, Temp: created.Temp()}
	var assembled Runtime
	if plan == nil {
		assembled, err = Assemble(ctx, req.Operator, session)
	} else {
		assembled, err = AssembleRework(ctx, req.Operator, session)
	}
	assembling()
	if err != nil {
		return refuse(req, res, standing, ledger.Runtime, err.Error())
	}
	rendering := phase(req, "render the "+req.Provider+" profile and ask it about every path", 0)
	profile, err := confinement(ctx, boxPolicy{
		Workspace: req.Workspace,
		Box:       created,
		Home:      home,
		Operator:  req.Operator,
		Runtime:   assembled,
		Deadlines: req.Deadlines,
		Promise:   art.ID,
		Gather:    art.Gather,
		Scope:     art.RepairScope,
	}, provider)
	rendering()
	if err != nil {
		return refuse(req, res, standing, ledger.Provider, err.Error())
	}
	res.Grants = append([]string(nil), profile.Write...)
	launch, err := confine.NewNono(profile)
	if err != nil {
		return refuse(req, res, standing, ledger.Provider, err.Error())
	}
	// The canary and the pre-flight, before a session exists and before a dry
	// run reports that everything is ready. A provider is verified rather than
	// trusted by name, and a dry run that skipped this would report readiness it
	// had not established. See docs/decision/0017 and 0053.
	verifying := phase(req, "verify the box with the canary", canaryDeadline)
	err = launch.Verified()
	verifying()
	if err != nil {
		return refuse(req, res, standing, ledger.Provider, err.Error())
	}
	if req.DryRun {
		res.Status = Prepared
		return res, nil
	}

	receipt, err := thread.NewReceipt(req.Ledger, id)
	if err != nil {
		return nil, err
	}
	// Past here a failure has a thread a person can read, so it is reported as
	// one: which phase, which thread, and where the box went.
	failed := func(phase Phase, err error) *Failure {
		return &Failure{Phase: phase, Promise: req.Promise, Thread: receipt.ID(), RuntimeAfter: res.RuntimeAfter, Err: err}
	}
	res.Thread = receipt.ID()
	// The gap, filed where measurements live and named by the thread it was
	// measured for. The packet carries these bytes too and this is not a second
	// copy of them: the packet is frozen input a runtime received, and this is
	// the measurement, addressed by what it measured, that a later reader
	// compares the re-derivation against. It happens before the dispatch because
	// it has already happened. See docs/decision/0078.
	if err := file(req, receipt, thread.Before, observations); err != nil {
		return nil, err
	}
	deadline, cancel := context.WithTimeout(ctx, req.Deadlines.Agent+dispatchMargin)
	defer cancel()
	// The one place in kept where a person may wait an hour. It is bounded by
	// timeouts.agent and by nothing else, and until this display existed the
	// terminal said nothing at all for the whole of it.
	running := phase(req, "thread "+string(receipt.ID())+": a confined "+string(req.Operator.Runtime)+" session is running", req.Deadlines.Agent)
	err = thread.Dispatch(deadline, assembled.Config, packet, launch, thread.Supervisor{Timeout: req.Deadlines.Agent}, receipt)
	running()
	holdsWork = receipt.Launched()
	var ended thread.Diagnosis
	if holdsWork {
		ended = sessionDiagnosis(req.Store, id)
		req.emit(Event{Kind: SessionEnded, Attempt: number, Thread: id, Session: ended})
	}
	// The file at the runtime's path, compared with the one that was launched.
	// A different file is asked what it is, because a runtime that was replaced
	// underneath its session is a fact a person reading the result should not
	// have to reconstruct. See docs/decision/0085.
	if launch, _, replaced := receipt.Replaced(); replaced {
		res.RuntimeAfter = replacedRuntime(ctx, launch)
		res.warn(fmt.Sprintf("the runtime at %s was replaced during thread %s's session: it was admitted as %s and reports %s now",
			launch.Path, receipt.ID(), launch.Version, res.RuntimeAfter))
	}
	// A thread opened, which is what `act` records: a session that ran and cost
	// a credential must be in the record whatever happens next. Record only what
	// has already happened, and this has. See docs/decision/0018.
	ran := func() error {
		detail := fmt.Sprintf("thread %s: a confined %s session ran in a box under %s", receipt.ID(), req.Operator.Runtime, req.Provider)
		if plan != nil {
			detail = fmt.Sprintf("thread %s, attempt %d: a confined %s session reworked it in a box under %s", receipt.ID(), number, req.Operator.Runtime, req.Provider)
		}
		return record(req, res, standing, ledger.OutcomeAct, "", string(receipt.ID()), detail)
	}
	if err != nil {
		var refusal *thread.Refusal
		if errors.As(err, &refusal) {
			// The canary is already verified, so a refusal here is the
			// adapter's: an unadmitted runtime, a version it is not pinned
			// against, or a credential that has lapsed.
			return refuse(req, res, standing, ledger.Runtime, refusal.Error())
		}
		// What failed is kept's, not the session's — the output could not be
		// retained, or the parent's own record of the process could not be
		// written — and the session may well have run to its end. It is recorded
		// as having run, and the box it ran in outlives this error. The verdict is
		// not guessed at: nothing is re-derived from a thread whose record is
		// broken. See docs/decision/0084.
		if !receipt.Launched() {
			// Nothing started, so there is no session to name and nothing in the
			// box but the commit it was cut from.
			return res, failed(PhaseLaunch, err)
		}
		if recordErr := ran(); recordErr != nil {
			err = fmt.Errorf("%w; record that the session ran: %v", err, recordErr)
		}
		return res, failed(PhaseSession, err)
	}
	if err := ran(); err != nil {
		return res, failed(PhaseRecord, err)
	}
	res.warn(undecodedWarning(receipt))

	// A session the account had no capacity left for is not a result. What it
	// left is half of something, a re-derivation of it measures how far the
	// session got before the account refused it, and anchoring it would put that
	// half in the land queue. The session ran and cost a credential, so the act
	// above stands; everything after it waits for the account. Its box is kept
	// when it changed anything, because nothing else holds that work. See
	// docs/decision/0090.
	if ended.Exhausted != nil {
		res.Status, res.Exhausted = Exhausted, ended.Exhausted
		if changed, err := created.Change(ctx); err == nil && len(changed.Paths) == 0 {
			holdsWork, res.Unchanged = false, true
		}
		return res, nil
	}

	// What the session did is evidence about the session. What the promise now
	// says is measured again, from the tree the session left behind, with the
	// instrument the promise names. See
	// policy:the-agents-report-is-never-an-input-to-the-verdict.
	rederiving := phase(req, "re-derive "+req.Promise+" from the box", 0)
	after, measured, err := derive(ctx, req, created.Path(), id, extend(lineage, observations))
	rederiving()
	if err != nil {
		return res, failed(PhaseRederive, err)
	}
	res.After = after
	req.emit(Event{Kind: Rederived, Attempt: number, Thread: id, Before: before.Value, After: after.Value, Detail: firstLineOf(after.Detail)})
	// The second half of the comparison an assessor works from. It is filed
	// after the re-derivation rather than with it, because a measurement is
	// recorded once it has been made and never before. A failure here is a
	// warning and not a result: the session ran, the verdict is reached, and
	// losing the evidence a later reader wants is smaller than losing the
	// repair. See docs/decision/0078.
	if err := file(req, receipt, thread.After, measured); err != nil {
		res.warn(fmt.Sprintf("the re-derivation could not be filed, so an assessor has only one side of this thread: %s", err))
	}
	switch after.Value {
	case model.Kept:
		res.Status = Repaired
	case model.NotKept:
		res.Status = Unrepaired
	default:
		res.Status = Undecided
	}

	reading := phase(req, "read what changed in the box", req.Deadlines.Git)
	changed, readErr := created.Change(ctx)
	reading()
	if readErr == nil && len(changed.Paths) == 0 {
		holdsWork = false
		res.Unchanged = true
	}

	// A rework cut at an anchor that holds moves the thread's anchor only to
	// work that holds as well. The anchor is what a land takes and what the next
	// attempt is cut at, so replacing one that holds with one that does not
	// throws away the best work the episode has for the worst. The attempt is
	// still an attempt — its session is recorded, its measurement filed and its
	// reading carried into the next packet — and its box is kept, because it is
	// the only copy of that work. See docs/decision/0090.
	res.Regressed = plan != nil && before.Value == model.Kept && after.Value != model.Kept

	// The work is anchored before the diff is reported, and on the path where
	// the diff could not be read at all. A repair that changed too much to
	// show is exactly the one a person cannot review by eye, and discarding its
	// tree because the presentation failed would lose the work over a display
	// bound. See docs/decision/0067.
	tooLarge := errors.Is(readErr, box.ErrDiffTooLarge)
	if res.Regressed && (readErr == nil && len(changed.Paths) > 0 || tooLarge) {
		req.emit(Event{Kind: Anchored, Attempt: number, Thread: id, Detail: fmt.Sprintf(
			"not anchored: the work re-derived %s, and the anchor it was cut at, %s, re-derived kept, so the thread's anchor stays there",
			valueWord(after.Value), shortCommitOf(plan.Tip))})
	} else if readErr == nil && len(changed.Paths) > 0 || tooLarge {
		if err := anchor(ctx, req, res, created, receipt, plan); err != nil {
			// The session ran and is recorded; an anchor that would not be
			// written is worth saying out loud and is not a repair that did not
			// happen.
			res.warn(fmt.Sprintf("the repair could not be anchored, so there is nothing for a land to take and the box is kept: %s", err))
			req.emit(Event{Kind: Anchored, Attempt: number, Thread: id, Detail: "the work could not be anchored: " + err.Error()})
		} else {
			req.emit(Event{Kind: Anchored, Attempt: number, Thread: id, Commit: res.Anchor.Commit})
		}
	} else if res.Unchanged {
		req.emit(Event{Kind: Anchored, Attempt: number, Thread: id, Detail: "the session changed nothing in the box, so there was nothing to anchor"})
	}

	if readErr != nil {
		// A diff past its bound is one of these, and it is not a repair that
		// did not happen: the thread is recorded, the box is named, and the
		// person is told where to read it.
		unread := readErr.Error()
		if !req.Keep && res.Anchor.Commit != "" {
			unread += "; --keep would have left the box to read it in"
		}
		res.warn(unread)
		return judged(ctx, req, res), nil
	}
	res.Change = changed
	// The one classification. The report a person reads beside the diff and the
	// check a land refuses on are the same computation, so they cannot disagree.
	// Nothing here parses diff text. See docs/decision/0059.
	res.Review = changed.Against(art.RepairScope)
	return judged(ctx, req, res), nil
}

// judged is the adversarial pass over an attempt that left work of its own to
// argue about, and the reason there is none for one that did not.
//
// A regressed attempt left no anchor of its own: the thread's anchor is the one
// it was cut at, which a pass has already argued about. Arguing again would cut
// a box at that anchor and spend a session repeating an argument the next
// packet already carries. See docs/decision/0090.
func judged(ctx context.Context, req Request, res *Result) *Result {
	if !res.Regressed {
		if why := alreadyEscalated(ctx, req, res); why != "" {
			res.Argument = &ArgueResult{Thread: res.Thread, Status: Deferred, Detail: why}
			req.emit(Event{Kind: PassEnded, Attempt: res.Attempt, Thread: res.Thread, Pass: Deferred, Detail: why})
			return res
		}
		return argued(ctx, req, res)
	}
	res.Argument = &ArgueResult{Thread: res.Thread, Status: Unanchored, Detail: "this attempt's work was not anchored, so there is no new act for a pass to argue about"}
	req.emit(Event{Kind: PassEnded, Attempt: res.Attempt, Thread: res.Thread, Pass: Unanchored, Detail: res.Argument.Detail})
	return res
}

// alreadyEscalated is why a pass over this attempt would be spent on a ruling
// that cannot change what follows, and empty when a pass is worth running.
//
// The derived checks are worked before the pass, from the same receipt, and two
// of 0086's rows escalate on them alone: a check about the act that fired again,
// and one that could not be worked for a cause outside the episode. Escalation
// wins over every row a ruling can select, so a pass over such an attempt costs a
// session and decides nothing. Measured on thread 2ntpka5qh7ebj0sg6fjvbn5fmw, two
// of its three passes were that: $10.12 of $32.51. The pass is deferred rather
// than dropped — `kept repair --rework` argues the attempt first — so the case
// against this work still reaches the next attempt if a person sends it back,
// and costs nothing if they answer the escalation another way. See
// docs/decision/0095.
//
// No assessor configured is not a deferral; that attempt is reported as it
// always was. And a reading that cannot be worked out is never a reason to
// skip: the pass runs and the table decides as before.
func alreadyEscalated(ctx context.Context, req Request, res *Result) string {
	if req.Assessor.Runtime == "" || res.Thread == "" {
		return ""
	}
	readings, err := assess.Attempts(ctx, assess.Request{
		Store:      req.Store,
		Repository: req.Workspace,
		Corpus:     req.Corpus,
		Thread:     res.Thread,
		Deadline:   req.Deadlines.Git,
	})
	if err != nil || len(readings) == 0 {
		return ""
	}
	escalations := derivedEscalations(readings, res)
	if len(escalations) == 0 {
		return ""
	}
	return "the derived checks already hand this attempt to a person, so no pass is spent on it; kept repair --rework " + string(res.Thread) + " argues it first: " + strings.Join(escalations, "; ")
}

// valueWord is a re-derived value as a sentence names it, with nothing reached
// named as unknown rather than as an empty word.
func valueWord(value model.Value) string {
	if value == "" {
		return string(model.Unknown)
	}
	return string(value)
}

// shortCommitOf is a commit as a phase line names it.
func shortCommitOf(commit string) string {
	if len(commit) > 7 {
		return commit[:7]
	}
	return commit
}

// argued runs the adversarial pass over the thread this repair just wrote, and
// is where docs/decision/0079 puts the assessor: inside the episode, after the
// re-derivation and the scope review, rather than behind a verb a person types.
//
// **It gates nothing and lands nothing.** What the pass finds is filed in the
// thread and printed; no disposition is written for it, no exit code moves, and
// nothing selects an action from it — that is the dispatch policy of open
// question 3, and taking it here would be deciding it. This is the loop's other
// half being built, not the loop being turned on.
//
// It runs after the anchor exists on purpose. A pass argues about an act, its
// box is cut at the commit that act produced, and a pass dispatched before the
// anchor was written would be arguing about a tree nothing durably names.
//
// A failure here is a warning rather than a result. The repair ran, the verdict
// is the re-derivation's, and a pass that could not be started does not unmake
// any of that — reporting it as a failed repair would put the assessor in front
// of the verdict, which is the one place
// policy:the-agents-report-is-never-an-input-to-the-verdict says it may not be.
func argued(ctx context.Context, req Request, res *Result) *Result {
	if res.Thread == "" {
		return res
	}
	if req.Assessor.Runtime != "" {
		req.emit(Event{Kind: PassStarted, Attempt: res.Attempt, Thread: res.Thread})
	}
	pass, err := Argue(ctx, ArgueRequest{
		Workspace: req.Workspace,
		Thread:    res.Thread,
		Corpus:    req.Corpus,
		Provider:  req.Provider,
		Assessor:  req.Assessor,
		Deadlines: req.Deadlines,
		Store:     req.Store,
		Ledger:    req.Ledger,
		Progress:  req.Progress,
	})
	if err != nil {
		res.warn("the adversarial pass could not be worked: " + err.Error())
		req.emit(Event{Kind: PassEnded, Attempt: res.Attempt, Thread: res.Thread, Detail: "the adversarial pass could not be worked: " + err.Error()})
		return res
	}
	res.Argument = pass
	if pass.Warning != "" {
		res.warn(pass.Warning)
	}
	req.emit(Event{Kind: PassEnded, Attempt: res.Attempt, Thread: res.Thread, Pass: pass.Status, Ruling: pass.Ruling, Detail: passDetail(pass)})
	return res
}

// passDetail is the sentence a pass that made no ruling is reported with.
func passDetail(pass *ArgueResult) string {
	if pass.Status == Argued {
		return pass.RulingWhy
	}
	return pass.Detail
}

// anchor commits what the session left and points refs/kept/threads/<id> at it,
// then records that in the receipt.
//
// The order is the record invariant: the commit and its ref exist before
// anything names them, so a receipt never points at an object that was never
// written. See docs/policy/record-only-what-has-already-happened.md.
//
// It anchors whatever the session produced, repaired or not and bounded or not.
// What may land is the land's question — it re-derives every promise the change
// touches and refuses on the scope review — and deciding it here would mean two
// places answered it, which is the split docs/decision/0059 already closed for
// the review a person reads.
func anchor(ctx context.Context, req Request, res *Result, created *box.Box, receipt *thread.Receipt, plan *reworkPlan) error {
	ref, err := ledger.ThreadRef(string(receipt.ID()))
	if err != nil {
		return err
	}
	who, err := identity(req.By, req.Workspace, req.Deadlines.Git)
	if err != nil {
		return err
	}
	parent := created.Commit()
	message := fmt.Sprintf("repair %s\n\nthread %s, a confined %s session under %s.\nThe verdict is kept's re-derivation from this tree and never the session's\naccount of itself.\n",
		req.Promise, receipt.ID(), req.Operator.Runtime, req.Provider)

	var commit string
	if plan == nil {
		commit, err = created.Anchor(ctx, ref, message, who)
	} else {
		// A rework's anchor names the whole episode's change against its base,
		// which is what a land merges, and keeps the earlier anchor reachable as
		// its second parent. See docs/decision/0086.
		parent = plan.Base
		message = fmt.Sprintf("rework %s\n\nthread %s, attempt %d, a confined %s session under %s.\nThe verdict is kept's re-derivation from this tree and never the session's\naccount of itself.\n",
			req.Promise, receipt.ID(), res.Attempt, req.Operator.Runtime, req.Provider)
		commit, err = created.AnchorOnto(ctx, ref, message, who, plan.Base)
	}
	if err != nil {
		return err
	}
	recorded := thread.Anchor{Commit: commit, Parent: parent, Promise: req.Promise}
	if err := receipt.RecordAnchor(recorded); err != nil {
		return err
	}
	res.Anchor, res.AnchorRef = recorded, ref
	return nil
}

// identity is who authors the anchor commit.
//
// kept's own operator first, because that is the name a person set deliberately
// and the one every act in the ledger already carries. Failing that, the
// identity Git itself would put on a commit made here — the repository's own if
// it has one, and the machine's otherwise, since that is what `git config
// --get` answers and what any commit would actually carry. Failing both, an
// error and no anchor: an identity kept invented would put a name nobody chose
// on a commit that can reach a branch, and docs/decision/0048 is explicit that
// an operating system's username is not an identity.
//
// Asking Git rather than reading a file is the point. A name that came from
// `user.name` is one the person set, wherever they set it; a name kept composed
// from the environment is one nobody did.
func identity(by, workspace string, deadline time.Duration) (box.Identity, error) {
	if name, email, ok := splitIdentity(by); ok {
		return box.Identity{Name: name, Email: email}, nil
	}
	if deadline <= 0 {
		deadline = box.DefaultDeadline
	}
	name := gitConfig(workspace, "user.name", deadline)
	email := gitConfig(workspace, "user.email", deadline)
	if name != "" && email != "" {
		return box.Identity{Name: name, Email: email}, nil
	}
	return box.Identity{}, errors.New("nobody is named to author it: set operator: in your user config, or user.name and user.email in this repository")
}

// splitIdentity reads "Name <email>". Anything else is not an identity Git can
// be handed, and half of one is not better than none.
func splitIdentity(who string) (string, string, bool) {
	who = strings.TrimSpace(who)
	open := strings.LastIndex(who, "<")
	if open <= 0 || !strings.HasSuffix(who, ">") {
		return "", "", false
	}
	name := strings.TrimSpace(who[:open])
	email := strings.TrimSpace(who[open+1 : len(who)-1])
	if name == "" || email == "" || strings.ContainsAny(email, "<> \t") || strings.ContainsAny(name, "<>") {
		return "", "", false
	}
	return name, email, true
}

func gitConfig(workspace, key string, deadline time.Duration) string {
	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()
	git, err := exec.LookPath("git")
	if err != nil {
		return ""
	}
	// Standard output alone: Apple's /usr/bin/git shim writes to stderr on
	// every invocation, and a value read out of the two merged is that noise.
	out, err := exec.CommandContext(ctx, git, "-C", workspace, "config", "--get", key).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// subject resolves the promise and its standing, and refuses the artifacts that
// cannot be repaired however the machine is configured.
func subject(req Request) (*corpus.Entry, register.Standing, error) {
	entry, found := req.Corpus.Get(req.Promise)
	if !found {
		return nil, register.Standing{}, &Unrepairable{Why: fmt.Sprintf("the corpus holds no %s", req.Promise)}
	}
	art := entry.Artifact
	if art.Kind != "promise" {
		// Only a promise bounds what a repair may change, so only a promise can
		// authorize one. See docs/decision/0047.
		return nil, register.Standing{}, &Unrepairable{Why: fmt.Sprintf("%s is a %s, and a repair is authorized by a promise", art.ID, art.Kind)}
	}

	var standing register.Standing
	for _, s := range register.Derive(req.Corpus, req.Acts) {
		if s.Artifact == art.ID {
			standing = s
			break
		}
	}
	switch {
	case standing.State != register.Ratified:
		return nil, standing, &Unrepairable{Why: fmt.Sprintf("%s reads %s, and nothing is owed until a person ratifies what it says now", art.ID, standing.State)}
	case len(art.RepairScope) == 0:
		return nil, standing, &Unrepairable{Why: fmt.Sprintf("%s bounds no repair scope, so it authorizes no repair", art.ID)}
	case art.HoldWhen == "" || len(art.Gather) == 0:
		return nil, standing, &Unrepairable{Why: fmt.Sprintf("%s names no instrument, so nothing can state a gap or check that one closed", art.ID)}
	}
	return entry, standing, nil
}

// providerExecutable resolves the named confinement provider to a path.
//
// The name is the repository's policy and the path is this machine's answer to
// it. Resolution happens once, here, and what is rendered into the profile is
// the resolved path: neither the provider nor the canary resolves a command on a
// search path, because a path chosen at each use is not a policy anybody wrote
// down. `unset` is not `none`. See docs/decision/0017.
func providerExecutable(provider string) (string, error) {
	if provider == "" {
		return "", errors.New("no confinement provider is named in .kept/config.yaml, and unset is not none, " +
			"so nothing may be dispatched into a box")
	}
	if provider != nonoProvider {
		// The set is closed and it has one member. A name kept does not know
		// how to render a profile for is refused rather than resolved on a
		// search path and started, because what would be verified then is a
		// program nobody described.
		return "", fmt.Errorf("%q is not a confinement provider kept renders a profile for; the one it does is %q", provider, nonoProvider)
	}
	resolved, err := exec.LookPath(provider)
	if err != nil {
		return "", fmt.Errorf("the confinement provider %q is named in .kept/config.yaml and is not installed", provider)
	}
	return filepath.Abs(resolved)
}

// nonoProvider is the one provider this build renders a profile for. See
// docs/decision/0017 and internal/confine.
const nonoProvider = "nono"

// runtimeUnready is what can be said about the repair runtime before a box
// exists. The rest — the version it reports and the life left in its credential
// — is asked once the gap is established, because both cost an external call
// and neither is worth making about a promise that turns out to hold.
func runtimeUnready(user config.Repair) string {
	switch {
	case user.Runtime == "":
		return "no repair runtime is named in the operator's config, so there is nothing to dispatch"
	case user.Executable == "":
		return fmt.Sprintf("the operator's config names the %s runtime and no executable for it", user.Runtime)
	case !filepath.IsAbs(user.Executable):
		return fmt.Sprintf("the repair runtime executable %q is not an explicit absolute path", user.Executable)
	}
	if _, err := os.Stat(user.Executable); err != nil {
		return fmt.Sprintf("the repair runtime is configured at %s, which is not there", user.Executable)
	}
	return ""
}

// cut creates the box and the session home beside it.
//
// The box is cut at the workspace's own HEAD, with no dirty overlay. What that
// costs is stated rather than hidden: work in the working tree and not in the
// commit is not in the box, so a promise broken only by uncommitted edits reads
// as holding there and nothing is dispatched. The overlay exists for carrying
// selected operator work in and is not used here, because choosing which dirty
// paths a repair inherits is a decision a person makes and nothing asks them.
//
// The home lies beside the box rather than inside it: a runtime's private state
// in the tree would arrive in the repair's diff as though an agent had put it
// there. It is empty, because the adapter refuses a home that is not — a session
// cannot inherit another one's state.
func cut(ctx context.Context, req Request) (*box.Box, string, error) {
	commit, err := head(ctx, req.Workspace, req.Deadlines.Git)
	if err != nil {
		return nil, "", err
	}
	created, err := box.Create(ctx, box.Request{
		Repository: req.Workspace,
		Commit:     commit,
		Deadline:   req.Deadlines.Git,
	})
	if err != nil {
		return nil, "", err
	}
	home := created.Path() + ".home"
	if err := os.MkdirAll(home, 0o700); err != nil {
		_ = created.Close(context.Background())
		return nil, "", fmt.Errorf("create the repair session home: %w", err)
	}
	return created, home, nil
}

// head is the commit the box is pinned to. A box refuses a moving name, so the
// object id is resolved here and the box verifies it exists.
func head(ctx context.Context, workspace string, deadline time.Duration) (string, error) {
	if deadline <= 0 {
		deadline = box.DefaultDeadline
	}
	ctx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()
	git, err := exec.LookPath("git")
	if err != nil {
		return "", errors.New("git is not installed, and a repair box is a worktree")
	}
	out, err := exec.CommandContext(ctx, git, "-C", workspace, "rev-parse", "HEAD").Output()
	if ctx.Err() != nil {
		return "", fmt.Errorf("resolving HEAD in %s timed out after %s", workspace, deadline)
	}
	if err != nil {
		return "", fmt.Errorf("resolve HEAD in %s: %w", workspace, err)
	}
	commit := strings.TrimSpace(string(out))
	if commit == "" {
		return "", fmt.Errorf("%s has no commit to cut a box from", workspace)
	}
	return commit, nil
}

// file records the box's measurements where measurements live, and names them in
// the receipt.
//
// Two writes rather than one, and they are different kinds of record. The
// observation goes to the content-addressed stream every other measurement goes
// to — prunable, re-gatherable against the world it names, and deduplicated
// against an identical measurement whoever made it. The receipt entry is the
// part that is not derivable from anything else: a box is discarded, so without
// it nothing says these two were the sides of this session.
//
// Sorted by name, because map iteration is not an order and a receipt is
// append-only: two repairs of one promise must write their measurements in the
// same order or the stream says something about iteration that it does not mean.
func file(req Request, receipt *thread.Receipt, side thread.Side, observations map[string]model.Observation) error {
	for _, name := range slices.Sorted(maps.Keys(observations)) {
		address, err := req.Ledger.Observe(ledger.Measured(observations[name]))
		if err != nil {
			return fmt.Errorf("file the %s measurement of %s: %w", side, name, err)
		}
		if err := receipt.RecordMeasurement(thread.Measurement{Side: side, Name: name, Address: address}); err != nil {
			return err
		}
	}
	return nil
}

// history is what a later measurement in this thread's box folds as prior(),
// keyed the way the predicate reads it.
//
// One thread's own lineage and nothing else. These are the same measurements
// [file] has already put in the observation stream and the receipt has already
// named, so the lineage is durable before it is folded and a re-derivation
// performed later reads it back from there; in the middle of an episode it is
// simply already in hand. An observation carries its own address from the moment
// it is measured, which is what lets prior() tell a predecessor from the
// measurement it precedes. See docs/decision/0080.
func history(observations map[string]model.Observation) map[string][]model.Observation {
	if len(observations) == 0 {
		return nil
	}
	out := make(map[string][]model.Observation, len(observations))
	for name, observation := range observations {
		out[name] = []model.Observation{observation}
	}
	return out
}

// extend is a thread's lineage with one more measurement of each instrument
// appended, which is what a re-derivation in the same box folds as prior().
func extend(lineage map[string][]model.Observation, observations map[string]model.Observation) map[string][]model.Observation {
	if len(lineage) == 0 {
		return history(observations)
	}
	out := make(map[string][]model.Observation, len(lineage)+len(observations))
	for name, series := range lineage {
		out[name] = append([]model.Observation(nil), series...)
	}
	for _, name := range slices.Sorted(maps.Keys(observations)) {
		out[name] = append(out[name], observations[name])
	}
	return out
}

// sessionDiagnosis is how far the last repair session in a thread got, read back
// from the receipt it wrote. A receipt that cannot be read is a diagnosis of
// nothing, which reads as a session that never started — and the failure that
// explains why is the dispatch's to report, not this line's.
func sessionDiagnosis(store ledger.Store, id thread.ID) thread.Diagnosis {
	entries, _, err := thread.ReadReceipt(store, id)
	if err != nil {
		return thread.Diagnosis{}
	}
	session, found := thread.RepairSession(entries)
	if !found {
		return thread.Diagnosis{}
	}
	return thread.Diagnose(session)
}

func firstLineOf(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	return line
}

// recorded is a box's measurements as the packet carries them.
func recorded(observations map[string]model.Observation) map[string]ledger.Observation {
	out := make(map[string]ledger.Observation, len(observations))
	for name, observation := range observations {
		out[name] = ledger.Measured(observation)
	}
	return out
}

// derive measures one promise where it stands and applies its predicate, and
// returns every instrument's measurement keyed by the name the promise gathers
// it under.
//
// The tree is walked, the world digest is computed over what is actually there,
// the instrument runs and the predicate is applied. Nothing is recorded
// anywhere: a verdict reached in a box belongs to the thread rather than to the
// workspace's truth stream, and this package opens no ledger of its own. See
// docs/decision/0035 and promise:standing-is-minted-in-one-place.
//
// The acts are the operator's, because an act is a person's and a box holds no
// ledger at all. They reach a gatherer only if it declared a read of them.
func derive(ctx context.Context, req Request, root string, id thread.ID, prior map[string][]model.Observation) (tick.Answer, map[string]model.Observation, error) {
	corpusRoot, workspaceRoot, err := corpus.FindRoot(root)
	if err != nil {
		return tick.Answer{}, nil, fmt.Errorf("read the box's corpus: %w", err)
	}
	c, err := corpus.Load(corpusRoot, workspaceRoot, corpus.Options{Gatherers: gather.Builtin()})
	if err != nil {
		return tick.Answer{}, nil, fmt.Errorf("read the box's corpus: %w", err)
	}

	// Every instrument the artifact names, under the name it names it by. The
	// verdict already refuses to be reached at all when one of them could not be
	// measured, so a NotKept answer arrives here with the set complete — which
	// is what lets the packet's own guard hold it to that. See
	// docs/decision/0061.
	//
	// The thread is what makes this a measurement in a box rather than a reading
	// of the tree that ships, and it travels into the world the measurement is
	// addressed by. The prior measurements are this thread's own; the first
	// derivation of an episode has none, which is what a promise reading prior()
	// answers unknown against. See docs/decision/0080.
	return tick.Measure(ctx, tick.MeasureRequest{
		Root:      root,
		Corpus:    c,
		Artifact:  req.Promise,
		Catalogue: gather.Builtin(),
		Deadline:  req.Deadlines.Gatherer,
		Exclude:   req.Exclude,
		Workers:   runtime.NumCPU(),
		Acts:      req.Acts,
		Progress:  req.Progress,
		Thread:    string(id),
		History:   prior,
	})
}

// assemblePacket freezes the packet from ratified authority and the evidence
// measured in the box.
//
// The observations are the box's, not the workspace's, and that is the point:
// the agent stands in the box, and evidence about a tree it is not in would be
// evidence about somewhere else. See docs/decision/0014.
//
// All of them, and never a chosen subset. Which half of a two-dimensional
// promise is failing is a reading of the evidence, and kept making that reading
// on the agent's behalf is a step toward enumerating the repair — and it is
// wrong outright when the clauses are coupled. See docs/decision/0061.
func assemblePacket(req Request, entry *corpus.Entry, standing register.Standing, observations map[string]ledger.Observation, rework *thread.Rework) (thread.Packet, error) {
	art := entry.Artifact
	policies, err := dispatchAuthority(req)
	if err != nil {
		return thread.Packet{}, err
	}
	packet, err := thread.AssemblePacket(thread.PacketInput{
		Promise: thread.RatifiedPromise{
			ID:          art.ID,
			Hash:        standing.Hash.Digest,
			Norm:        standing.Hash.Norm,
			Ratified:    true,
			Statement:   art.Statement,
			Gather:      art.Gather,
			HoldWhen:    art.HoldWhen,
			Trigger:     art.Trigger,
			RepairScope: art.RepairScope,
		},
		Why:          art.Body,
		Observations: observations,
		Policies:     policies,
		Rework:       rework,
	})
	if err != nil {
		return thread.Packet{}, &Unrepairable{Why: err.Error()}
	}
	return packet, nil
}

// dispatchPolicies is the ratified authority that binds this act, read out of
// the corpus.
//
// Which policies those are is not a judgement made here: a policy declares the
// act it is checked before, and this collects the ones that say dispatch. An
// unratified one refuses the whole packet rather than being dropped from it —
// authority nobody granted is not authority a repair may be handed, and silently
// omitting it would make the packet smaller with nothing saying so.
func dispatchAuthority(req Request) ([]thread.RatifiedDispatchPolicy, error) {
	standings := map[string]register.Standing{}
	for _, s := range register.Derive(req.Corpus, req.Acts) {
		standings[s.Artifact] = s
	}

	var policies []thread.RatifiedDispatchPolicy
	for _, entry := range req.Corpus.All() {
		art := entry.Artifact
		applies, _ := art.Fields["applies_to"].(string)
		if applies != "dispatch" {
			continue
		}
		standing := standings[art.ID]
		if standing.State != register.Ratified {
			return nil, &Unrepairable{Why: fmt.Sprintf("%s binds a dispatch and reads %s, so a packet would carry authority nobody granted", art.ID, standing.State)}
		}
		onFail, _ := art.Fields["on_fail"].(string)
		policies = append(policies, thread.RatifiedDispatchPolicy{
			ID:        art.ID,
			Hash:      standing.Hash.Digest,
			Norm:      standing.Hash.Norm,
			Ratified:  true,
			Statement: art.Statement,
			AppliesTo: applies,
			Gather:    art.Gather,
			HoldWhen:  art.HoldWhen,
			OnFail:    onFail,
		})
	}
	if len(policies) == 0 {
		return nil, &Unrepairable{Why: "no ratified policy in this corpus binds a dispatch, so a packet would carry no authority at all"}
	}
	// Sorted, because a packet assembled twice from one corpus must be the same
	// bytes and map iteration is not an order.
	slices.SortFunc(policies, func(a, b thread.RatifiedDispatchPolicy) int { return strings.Compare(a.ID, b.ID) })
	return policies, nil
}

// boxPolicy is what deciding a confinement profile actually depends on, which
// is less than a whole [Request].
//
// It is a struct because there are two callers and they differ in one field: a
// repair names the promise's bound scope and an adversarial pass names none.
// Passing a Request would mean the pass had to build one for a verb that does
// not repair anything, and passing eight parameters would mean the difference
// between the two boxes was somewhere in the middle of a call. See
// docs/decision/0081.
type boxPolicy struct {
	Workspace string
	Box       *box.Box
	Home      string
	Operator  config.Repair
	Runtime   Runtime
	Deadlines Deadlines

	// Promise names the artifact whose instruments say what the box must have,
	// and Gather is those instruments. They come from the promise for a repair
	// and from the packet's frozen copy of it for a pass, which is the same
	// answer read out of the record that already carried it.
	Promise string
	Gather  map[string]model.GatherSpec

	// Scope is the promise's bound repair scope, and it is required either
	// way: for a repair it is what may be written, and for a pass it is what
	// must be refused.
	Scope []string

	// ReadOnly is an adversarial pass's box. Nothing inside it is writable, and
	// what a repair of the same promise would have been granted becomes the
	// paths the canary must be refused in — which is what makes the declaration
	// a boundary rather than a claim. See docs/decision/0081.
	ReadOnly bool
}

// confinement renders the profile one confined session runs under.
//
// Everything in it is either the box's, the promise's, or something a tool was
// asked about itself. Nothing is a path list kept carries: the runtime is
// located by inspecting it, and a toolchain by running it. See
// docs/decision/0053 and 0056.
func confinement(ctx context.Context, policy boxPolicy, provider string) (confine.Profile, error) {
	kept, err := keptExecutable()
	if err != nil {
		return confine.Profile{}, fmt.Errorf("locate the kept executable the canary runs: %w", err)
	}
	kept, err = filepath.Abs(kept)
	if err != nil {
		return confine.Profile{}, err
	}
	git, err := exec.LookPath("git")
	if err != nil {
		return confine.Profile{}, errors.New("git is not installed, and the canary probes a push with it")
	}
	git, err = filepath.Abs(git)
	if err != nil {
		return confine.Profile{}, err
	}

	// The same rendering answers both questions, and that is deliberate: the
	// grant a repair is given and the grant a pass is refused are the same
	// computation over the same scope, so they cannot come to disagree about
	// what a repair of this promise would have been allowed to write.
	grants, err := writeGrants(policy.Box.Path(), policy.Scope)
	if err != nil {
		return confine.Profile{}, err
	}
	write, unwritable := grants, []string(nil)
	if policy.ReadOnly {
		write, unwritable = nil, grants
	}

	profile := confine.Profile{
		Nono:       provider,
		Kept:       kept,
		Git:        git,
		Box:        policy.Box.Path(),
		Read:       []string{policy.Box.Path()},
		Write:      write,
		ReadOnly:   policy.ReadOnly,
		Unwritable: unwritable,
		// The two machine-written tiers, denied by name. In a Git workspace
		// they live on refs/kept/ledger and these paths hold nothing; a box
		// still may not reach where they would be, and a manuscript's do live
		// there. See docs/decision/0016 and 0023.
		Ledger:    filepath.Join(policy.Workspace, corpus.ConfigDir, ledger.Dir),
		Artifacts: filepath.Join(policy.Workspace, corpus.ConfigDir, "artifacts"),
		Temp:      policy.Box.Temp(),
		Home:      policy.Home,
		// The one host this runtime's credential is answered by. A box that
		// wanted none would say so; an empty allowlist is the provider's
		// spelling of unrestricted. See docs/decision/0053.
		Domains:       []string{policy.Runtime.Domain},
		Env:           policy.Runtime.Env,
		TempEnv:       policy.Runtime.TempEnv,
		Credential:    policy.Runtime.Credential,
		CanaryTimeout: canaryDeadline,
	}

	tools, err := toolchains(ctx, policy)
	if err != nil {
		return confine.Profile{}, err
	}
	profile.ToolPath = tools.Path
	profile.ToolRead = tools.Read
	profile.ToolWrite = tools.Write
	profile.ToolEnv = tools.Env
	return profile, nil
}

// keptExecutable is where kept itself is: the binary the canary runs a sealed
// copy of, and the one the profile denies by name so a confined session cannot
// re-execute it to ratify from inside a box.
//
// It is a variable rather than the call itself for one reason, and it is a
// measurement rather than a preference. Under `go test` os.Executable() is the
// *test binary*, so a canary launched into it never reaches a probe — it reaches
// the testing package's flag parser, which refuses the canary argument and exits
// 2 having measured nothing. A test standing in for a dispatcher therefore has to
// be able to point this at a real kept, and a canary that cannot start is
// correctly a refusal rather than a boundary that held.
var keptExecutable = os.Executable

// systemToolPath is what every box gets on its search path beyond what a
// resolver found. It grants nothing — a search path is resolution and not
// containment — and the provider's own baseline is what reaches these.
var systemToolPath = []string{"/usr/bin", "/bin"}

// toolchains is what this box can run: the repair runtime, and the toolchain the
// promise's own instrument names.
//
// What decides that a repair needs Go is the promise, and it already says so:
// docs/decision/0051 put the requirement in the instrument's name, so `go/tests`
// means the Go toolchain and `corpus` means none. No workspace configuration is
// added and no second per-promise field exists. See docs/decision/0053.
func toolchains(ctx context.Context, policy boxPolicy) (confine.Toolchain, error) {
	native, err := runtimeToolchain(policy.Operator)
	if err != nil {
		return confine.Toolchain{}, err
	}
	merged := confine.Toolchain{
		Path:  append(append([]string(nil), native.Path...), systemToolPath...),
		Read:  append([]string(nil), native.Read...),
		Write: append([]string(nil), native.Write...),
		Env:   map[string]string{},
	}
	for name, value := range native.Env {
		merged.Env[name] = value
	}

	if !needsGo(policy.Gather) {
		return merged, nil
	}
	executable, err := exec.LookPath("go")
	if err != nil {
		return confine.Toolchain{}, fmt.Errorf("%s measures with a go/ instrument and this machine has no go to resolve", policy.Promise)
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return confine.Toolchain{}, err
	}
	deadline := policy.Deadlines.Git
	if deadline <= 0 {
		deadline = box.DefaultDeadline
	}
	toolchain, err := confine.ResolveGo(ctx, executable, deadline)
	if err != nil {
		return confine.Toolchain{}, err
	}
	merged.Path = append(merged.Path, toolchain.Path...)
	merged.Read = append(merged.Read, toolchain.Read...)
	merged.Write = append(merged.Write, toolchain.Write...)
	for name, value := range toolchain.Env {
		merged.Env[name] = value
	}
	merged.Path = dedupe(merged.Path)
	merged.Read = dedupe(merged.Read)
	merged.Write = dedupe(merged.Write)
	return merged, nil
}

func needsGo(gather map[string]model.GatherSpec) bool {
	for _, spec := range gather {
		if strings.HasPrefix(spec.Gatherer, "go/") {
			return true
		}
	}
	return false
}

// runtimeToolchain locates the repair runtime the way its own shape requires.
//
// A native image and a Node-hosted script are resolved differently and the wrong
// one succeeds rather than failing: resolving a self-contained Mach-O image as
// though it were hosted returns the Node installation root, which is a grant for
// a tree the runtime never opens. See docs/decision/0056.
func runtimeToolchain(user config.Repair) (confine.Toolchain, error) {
	if thread.Runtime(user.Runtime) != thread.Claude {
		return confine.Toolchain{}, fmt.Errorf("no confinement is described for the %s runtime", user.Runtime)
	}
	return confine.ResolveNative(user.Executable)
}

// writeGrants is the confinement grant a repair scope becomes.
//
// A scope is a glob and a filesystem grant is a path, so what is rendered is
// always at least as wide as the bound it stands for: `internal/gather/**/*.go`
// grants `internal/gather`, and there is no finer expression of it. A scope
// whose first segment is already a wildcard grants the box root, which is the
// whole tree.
//
// That width is why the review is what reports and what a land refuses on.
// Confinement is not the enforcement of a repair scope and never was — measured
// in a real box, a write the grant admits and the glob does not lands, and so
// does a delete. See docs/decision/0059.
func writeGrants(root string, scope []string) ([]string, error) {
	var grants []string
	for _, pattern := range scope {
		literal, err := literalPrefix(pattern)
		if err != nil {
			return nil, err
		}
		if literal == "" {
			return []string{root}, nil
		}
		grants = append(grants, filepath.Join(root, filepath.FromSlash(literal)))
	}
	if len(grants) == 0 {
		// A promise with no scope authorizes no repair, and this is unreachable
		// because the promise was refused before a box was cut. It is here so
		// that a future caller cannot render an empty grant as though a scope
		// covering nothing covered everything.
		return nil, errors.New("a repair scope naming nothing renders no confinement grant")
	}
	return coarsest(grants), nil
}

// literalPrefix is the leading run of a glob's segments that carries no
// wildcard. A pattern with no wildcard at all names one path and its parent is
// what a repair writes in, because a repair that may replace a file may also
// have to create one beside it.
func literalPrefix(pattern string) (string, error) {
	pattern = strings.TrimPrefix(strings.TrimSpace(pattern), "./")
	if pattern == "" {
		return "", errors.New("a repair scope entry is empty")
	}
	// A trailing separator is the matcher's spelling of a whole subtree, so the
	// directory it names is the bound rather than the last segment before one.
	subtree := strings.HasSuffix(pattern, "/")
	var literal []string
	glob := false
	for _, segment := range strings.Split(strings.TrimSuffix(pattern, "/"), "/") {
		if segment == ".." || segment == "" {
			return "", fmt.Errorf("a repair scope entry may not leave the workspace: %s", pattern)
		}
		if strings.Contains(segment, "*") {
			glob = true
			break
		}
		literal = append(literal, segment)
	}
	if !glob && !subtree && len(literal) > 0 {
		literal = literal[:len(literal)-1]
	}
	return strings.Join(literal, "/"), nil
}

// coarsest drops any grant another grant already contains, so the profile does
// not name the same tree twice under two spellings.
func coarsest(paths []string) []string {
	paths = dedupe(paths)
	var out []string
	for _, path := range paths {
		covered := false
		for _, other := range paths {
			if other != path && strings.HasPrefix(path, other+string(filepath.Separator)) {
				covered = true
				break
			}
		}
		if !covered {
			out = append(out, path)
		}
	}
	return out
}

func dedupe(paths []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, path := range paths {
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

// refuse records a refusal and returns it as the result.
func refuse(req Request, res *Result, standing register.Standing, condition ledger.Condition, detail string) (*Result, error) {
	// A refusal names no thread. The one refusal reached after a receipt exists
	// is the adapter's, and thread.Dispatch refuses before a packet is stored,
	// so that id names a receipt stream with nothing in it.
	res.Status = Refused
	if err := record(req, res, standing, ledger.OutcomeRefused, condition, "", detail); err != nil {
		return res, err
	}
	return res, nil
}

// record appends what this run decided about the drift, and only what has
// already happened.
//
// The world is the workspace's, because this run stands in the workspace: the
// evidence was measured in a box and that is the thread receipt's business, and
// a disposition naming a box's world would say the decision was reached
// somewhere kept was not. See docs/decision/0019 and 0035.
func record(req Request, res *Result, standing register.Standing, outcome ledger.Outcome, condition ledger.Condition, thread, detail string) error {
	res.Outcome, res.Condition, res.Detail = outcome, condition, detail
	written, recorded, err := dispose(req, res.Promise, standing, outcome, condition, thread, detail)
	if err != nil {
		return err
	}
	res.Disposition, res.Recorded = written, recorded
	return nil
}

// dispose appends one disposition about a promise, under the transition rule
// every other caller uses, and returns what stands and whether it was written.
func dispose(req Request, promise string, standing register.Standing, outcome ledger.Outcome, condition ledger.Condition, thread, detail string) (ledger.Disposition, bool, error) {
	if req.Ledger == nil {
		return ledger.Disposition{}, false, nil
	}

	tree, err := workspace.Read(req.Workspace, workspace.Options{Exclude: req.Exclude, Workers: runtime.NumCPU()})
	if err != nil {
		return ledger.Disposition{}, false, err
	}
	previous, _, err := ledger.ReadDispositions(req.Store, promise)
	if err != nil {
		return ledger.Disposition{}, false, err
	}
	var maxAge time.Duration
	if entry, found := req.Corpus.Get(promise); found {
		maxAge = entry.Artifact.MaxAge
	}

	return req.Ledger.DisposeMoved(ledger.Disposition{
		Artifact:  promise,
		Hash:      standing.Hash.String(),
		Norm:      standing.Hash.Norm,
		World:     model.World{Tree: tree.Digest()},
		Outcome:   outcome,
		Condition: condition,
		Thread:    thread,
		Detail:    detail,
	}, previous, maxAge)
}
