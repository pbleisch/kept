package repair

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/pbleisch/kept/internal/assess"
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/thread"
)

// Action is what one attempt's reading selects.
type Action string

const (
	// ActionReady: the promise holds, every check about the act is clear, and
	// the pass accepted or no assessor is configured. A person lands it.
	ActionReady Action = "ready"
	// ActionRework: something the next attempt could answer.
	ActionRework Action = "rework"
	// ActionEscalate: something no rework can fix, which is a person's.
	ActionEscalate Action = "escalate"
)

// End is how one run of an episode stopped.
type End string

const (
	// Ready: the episode's work is ready to land, and landing it is a person's
	// act.
	Ready End = "ready"
	// Escalated: the episode handed itself to a person, and the disposition
	// that says so names the thread.
	Escalated End = "escalated"
	// Stopped: an attempt did not dispatch — the promise held where the box was
	// cut, nothing could measure it, a dry run, or a refusal — and the attempt's
	// own result says which.
	Stopped End = "stopped"
	// Waiting: a session ended because the account it ran on had no capacity
	// left, or the account had said so and the time it named has not come. The
	// episode is not over and nothing is wrong with it; the disposition that
	// says so names the thread, and the run says what resumes it. See
	// docs/decision/0090.
	Waiting End = "waiting"
)

// Episode is one run of a repair episode, whole.
//
// The thread is the episode and a run is one invocation of `kept repair`: the
// first attempt of a new episode, or the next attempt of an existing one, and
// every rework after it until the episode is ready to land or has handed itself
// to a person. See docs/decision/0079 and 0086.
type Episode struct {
	Promise string
	Thread  thread.ID
	Bounds  Bounds

	// Attempts is every attempt this run opened, oldest first, including one
	// that stopped before its session.
	Attempts []*Result
	// Resumed is the attempt an earlier run opened and this run argued over,
	// because the account cut off the pass that should have; nil when this run
	// resumed nothing. It is not in Attempts, because this run did not open it.
	Resumed *Result
	// Readings is every attempt of the episode as its receipt reads after the
	// last one this run assessed, including attempts earlier runs opened.
	Readings []assess.AttemptReading

	End End
	// Why is one sentence for each reason the run stopped where it did.
	Why []string

	// Handoff is the disposition an escalation, a wait or a budget refusal
	// wrote, and Recorded reports whether it was appended.
	Handoff  ledger.Disposition
	Recorded bool

	// Resume is the command that continues a waiting episode, and empty for
	// one that is not waiting.
	Resume string
}

// Last is the last attempt this run opened, the one it resumed when it opened
// none, and nil for a run that did neither.
func (e *Episode) Last() *Result {
	if len(e.Attempts) == 0 {
		return e.Resumed
	}
	return e.Attempts[len(e.Attempts)-1]
}

// reworkPlan is what a later attempt of an episode is cut and told from.
type reworkPlan struct {
	Thread thread.ID
	// Base is the commit the episode's first box was cut from, and Tip is the
	// commit this attempt's box is cut at: the work the earlier attempts left.
	Base, Tip string
	// Earlier is every earlier attempt as the packet carries it.
	Earlier []thread.EarlierAttempt
	// Touched, Diff and Elided are the change so far, base to tip.
	Touched []thread.BriefPath
	Diff    string
	Elided  string
	// Lineage is the thread's own earlier measurements, which a measurement in
	// this attempt's box folds as prior(). See docs/decision/0080.
	Lineage map[string][]model.Observation

	// readings is the episode as it stood when the plan was made, kept so that
	// a run resuming a thread can see whether its last attempt is still waiting
	// for a pass.
	readings []assess.AttemptReading
}

// evidence is the rework field of the packet this plan's attempt carries, and
// nil for a first attempt.
func (p *reworkPlan) evidence(unmeasured string) *thread.Rework {
	if p == nil {
		return nil
	}
	return &thread.Rework{
		Base:       p.Base,
		Tip:        p.Tip,
		Touched:    p.Touched,
		Diff:       p.Diff,
		Elided:     p.Elided,
		Unmeasured: unmeasured,
		Earlier:    p.Earlier,
	}
}

// Run drives one run of a repair episode: the first attempt of a new one, or,
// when rework names a thread, the next attempt of that one — and every rework
// after it, until the episode is ready to land, has escalated, or an attempt
// did not dispatch.
//
// It lands nothing, ever. What it produces is an anchored change and a record of
// how it got there; landing is a person's act. And the verdict at every step is
// the re-derivation's: an attempt's reading decides whether to try again, never
// whether the promise is kept. See docs/decision/0086.
func Run(ctx context.Context, req Request, bounds Bounds, rework thread.ID) (*Episode, error) {
	if bounds.Attempts < 1 {
		return &Episode{Promise: req.Promise, Thread: rework, Bounds: bounds}, errors.New("a run of a repair episode may open at least one attempt, and these bounds allow none")
	}
	ep := &Episode{Promise: req.Promise, Thread: rework, Bounds: bounds}

	var plan *reworkPlan
	// Attempts are numbered within the thread rather than within the run, so a
	// rework's first attempt is the one after the thread's last. The attempts
	// budget still counts this run's, below.
	earlier := 0
	if rework != "" {
		opened, entries, err := openRework(ctx, &req, rework)
		if err != nil {
			return ep, err
		}
		earlier = len(opened.Earlier)
		ep.Promise = req.Promise
		// The account, before anything is cut: one that said it had no
		// capacity left, and named when that changes, is not asked again
		// before then. A session started now meets the same refusal and spends
		// a dispatch learning it. See docs/decision/0090.
		if said, waiting := stillExhausted(entries, time.Now()); waiting {
			return ep, wait(req, ep, said)
		}
		// The ceiling, before anything is cut. Nothing has opened in this run,
		// so a refusal here is `refused` on `budget` rather than an escalation:
		// the person who asked is the person being told.
		if said, refused := windowReading(entries, bounds.WindowCeiling); refused {
			return ep, refuseBudget(req, ep, said)
		}
		plan = opened
		// An attempt whose pass the account cut off has not been ruled on, and
		// it is ruled on before anything else is opened: its anchor is work an
		// earlier run already paid for, and reworking it unread would spend a
		// session to replace what a pass may have accepted.
		if resumed := pendingPass(opened.readings, entries); resumed != nil {
			resumed.Promise = req.Promise
			ep.Resumed = argued(ctx, req, resumed)
			next, done, err := settle(ctx, req, ep, bounds, ep.Resumed, 0)
			if err != nil || done {
				return ep, err
			}
			plan = next
		}
	}

	for number := 1; ; number++ {
		res, err := attempt(ctx, req, earlier+number, plan)
		if res != nil {
			ep.Attempts = append(ep.Attempts, res)
			if res.Thread != "" {
				ep.Thread = res.Thread
			}
		}
		if err != nil {
			return ep, err
		}
		if !res.Dispatched() && res.Status != Exhausted {
			ep.End, ep.Why = Stopped, []string{stoppedWhy(res)}
			return ep, nil
		}
		next, done, err := settle(ctx, req, ep, bounds, res, number)
		if err != nil || done {
			return ep, err
		}
		plan = next
	}
}

// settle reads one attempt whose session ran and selects what follows it: the
// episode waits, is ready, escalates, or opens another attempt from the plan it
// returns. opened is how many attempts this run has opened, counting this one,
// and it is zero for an attempt an earlier run opened and this one resumed.
func settle(ctx context.Context, req Request, ep *Episode, bounds Bounds, res *Result, opened int) (*reworkPlan, bool, error) {
	// A session the account cut off is not a result, so there is no reading of
	// it to decide from. The episode waits for the account rather than
	// reworking, because a rework dispatched now meets the same refusal, and
	// rather than escalating, because running out is an expected way for a
	// session to end and nothing about it is a person's to fix. See
	// docs/decision/0090.
	if said, exhausted := exhaustion(res); exhausted {
		return nil, true, wait(req, ep, said)
	}

	readings, err := assess.Attempts(ctx, assess.Request{
		Store:      req.Store,
		Repository: req.Workspace,
		Corpus:     req.Corpus,
		Thread:     res.Thread,
		Deadline:   req.Deadlines.Git,
	})
	if err != nil {
		return nil, true, fmt.Errorf("assess thread %s after attempt %d: %w", res.Thread, res.Attempt, err)
	}
	if len(readings) == 0 {
		return nil, true, fmt.Errorf("thread %s holds no attempt after one was dispatched into it", res.Thread)
	}
	ep.Readings = readings
	current := readings[len(readings)-1].Assessment
	req.emit(Event{Kind: Assessed, Attempt: res.Attempt, Thread: res.Thread, Fired: current.FiredOfTheAct(), Unread: current.UnreadOfTheAct()})

	action, why := decide(readings, res, req.Assessor.Runtime != "")
	switch action {
	case ActionReady:
		req.emit(Event{Kind: Decided, Attempt: res.Attempt, Thread: res.Thread, Action: ActionReady, Why: why})
		ep.End, ep.Why = Ready, why
		return nil, true, nil
	case ActionEscalate:
		req.emit(Event{Kind: Decided, Attempt: res.Attempt, Thread: res.Thread, Action: ActionEscalate, Why: why})
		return nil, true, escalate(req, ep, why)
	}

	// A reading that selects rework, and the gates a run applies before it
	// opens another attempt: a person's word, the attempts budget, and the
	// usage-window ceiling. Each of them turns the rework into an escalation
	// rather than a quiet stop.
	entries, _, err := thread.ReadReceipt(req.Store, res.Thread)
	if err != nil {
		return nil, true, err
	}
	window, refused := windowReading(entries, bounds.WindowCeiling)
	switch {
	case !bounds.Automatic:
		why = append(why, fmt.Sprintf("automatic rework is off, and kept repair --rework %s continues it", res.Thread))
	case opened >= bounds.Attempts:
		why = append(why, fmt.Sprintf("this run has opened %d of the %d %s it may, from %s", opened, bounds.Attempts, plural(bounds.Attempts, "attempt", "attempts"), bounds.From))
	case refused:
		why = append(why, window)
	default:
		req.emit(Event{Kind: Decided, Attempt: res.Attempt, Thread: res.Thread, Action: ActionRework, Why: why, Window: window})
		plan, err := nextPlan(ctx, req, ep, res.Thread, entries, readings)
		return plan, err != nil, err
	}
	req.emit(Event{Kind: Decided, Attempt: res.Attempt, Thread: res.Thread, Action: ActionEscalate, Why: why, Window: window})
	return nil, true, escalate(req, ep, why)
}

// exhaustion is what the runtime said when the account cut off an attempt's
// repair session or the pass over its work, and whether it did.
func exhaustion(res *Result) (string, bool) {
	switch {
	case res.Status == Exhausted && res.Exhausted != nil:
		return capacityWords("the repair session", *res.Exhausted), true
	case res.Status == Exhausted:
		return "the repair session ended because the account had no capacity left", true
	case res.Argument != nil && res.Argument.Status == ArgueExhausted && res.Argument.Exhausted != nil:
		return capacityWords("the adversarial pass", *res.Argument.Exhausted), true
	case res.Argument != nil && res.Argument.Status == ArgueExhausted:
		return "the adversarial pass ended because the account had no capacity left", true
	}
	return "", false
}

// capacityWords is the sentence a wait is reported and recorded with.
func capacityWords(who string, exhausted thread.Exhaustion) string {
	said := fmt.Sprintf("%s ended because the account had no capacity left; the runtime said %q", who, firstLineOf(exhausted.Said))
	if !exhausted.Resets.IsZero() {
		said += fmt.Sprintf(", and that its usage window resets at %s", exhausted.Resets.UTC().Format(time.RFC3339))
	}
	return said
}

// stillExhausted reports whether the last session in a thread that the account
// cut off named a reset that has not come yet.
//
// It reads the runtime's own reading and nothing else: a session that reported
// no reset, or one whose reset has passed, says nothing about now, and the
// next session is the only way to ask. And only the last cut-off session, and
// only when nothing ran after it: a session that ran to its end since then is
// the account answering again.
func stillExhausted(entries []thread.Entry, now time.Time) (string, bool) {
	sessions := thread.Sessions(entries)
	if len(sessions) == 0 {
		return "", false
	}
	last := sessions[len(sessions)-1]
	exhausted, out := thread.OutOfCapacity(last)
	if !out || exhausted.Resets.IsZero() || !now.Before(exhausted.Resets) {
		return "", false
	}
	who := "the repair session"
	if last.Kind == ledger.ThreadBrief {
		who = "the adversarial pass"
	}
	return capacityWords(who, exhausted) + ", which has not come yet", true
}

// pendingPass is the last attempt of an episode when it anchored work and the
// pass over that work was cut off by the account before it concluded, and nil
// otherwise. What comes back is the attempt as a result a pass and the table
// can read: its thread, its place, its anchor, and the value its own
// re-derivation reached.
func pendingPass(readings []assess.AttemptReading, entries []thread.Entry) *Result {
	if len(readings) == 0 {
		return nil
	}
	last := readings[len(readings)-1]
	if !last.Anchored || len(last.Attempt.Passes) == 0 {
		return nil
	}
	for _, pass := range last.Attempt.Passes {
		if len(thread.Arguments(pass.Entries)) > 0 {
			return nil
		}
	}
	if _, out := thread.OutOfCapacity(last.Attempt.Passes[len(last.Attempt.Passes)-1]); !out {
		return nil
	}
	res := &Result{
		Thread:  last.Assessment.Thread,
		Attempt: last.Number,
		Rework:  last.Number > 1,
		Commit:  last.Cut,
		Anchor:  last.Anchor,
	}
	switch last.After {
	case model.Kept:
		res.Status = Repaired
	case model.NotKept:
		res.Status = Unrepaired
	default:
		res.Status = Undecided
	}
	return res
}

// wait hands the episode to the account rather than to a person, and records
// that it did.
//
// It is the disposition of the attempt the episode declined to open, as an
// escalation is, and it names the thread for the same reason: whoever resumes
// it has to know which one. The resumption is a person's word — kept runs once
// and exits, and has no timer to come back on — so the run ends by saying what
// that word is. See docs/decision/0090.
func wait(req Request, ep *Episode, said string) error {
	ep.Resume = "kept repair --rework " + string(ep.Thread)
	resume := ep.Resume + " resumes it"
	if !anchoredAnything(req.Store, ep.Thread) {
		// Nothing in the episode was anchored, so there is no work to resume
		// from, and the next attempt is a new episode's first.
		ep.Resume = "kept repair " + ep.Promise
		resume = fmt.Sprintf("nothing in thread %s was anchored, so %s starts again", ep.Thread, ep.Resume)
	}
	ep.End, ep.Why = Waiting, []string{said, resume}
	_, standing, err := subject(req)
	if err != nil {
		return err
	}
	written, recorded, err := dispose(req, ep.Promise, standing, ledger.OutcomeWait, "", string(ep.Thread), said+"; "+resume)
	if err != nil {
		return err
	}
	ep.Handoff, ep.Recorded = written, recorded
	return nil
}

// anchoredAnything reports whether a thread's receipt names an anchor.
func anchoredAnything(store ledger.Store, id thread.ID) bool {
	if id == "" {
		return false
	}
	entries, _, err := thread.ReadReceipt(store, id)
	if err != nil {
		return false
	}
	_, anchored := thread.LatestAnchor(entries)
	return anchored
}

// decide is docs/decision/0086's table: what one attempt's reading selects, in
// the table's order, with every reason that applies.
//
// Escalation wins. Every row that escalates describes something no rework can
// fix — a finding that repeats, a check that could not be worked for a cause
// outside the episode, a pass that ruled nothing kept can read or never reached
// an argument — and a second attempt dispatched past one of them spends a
// session to meet the same condition.
//
// The verdict is an input here and never an output. What this selects is
// whether to try again; whether the promise is kept was settled by measuring the
// tree, and nothing here moves it.
func decide(readings []assess.AttemptReading, res *Result, assessorConfigured bool) (Action, []string) {
	current := readings[len(readings)-1]
	a := current.Assessment
	var escalations, reworks []string

	// A check about the act that fired before, and fires again.
	firedBefore := map[assess.Check]int{}
	for _, earlier := range readings[:len(readings)-1] {
		for _, finding := range earlier.Assessment.FiredOfTheAct() {
			if _, seen := firedBefore[finding.Check]; !seen {
				firedBefore[finding.Check] = earlier.Number
			}
		}
	}
	for _, finding := range a.FiredOfTheAct() {
		if number, seen := firedBefore[finding.Check]; seen {
			escalations = append(escalations, fmt.Sprintf("%s fired again, as it did after attempt %d: %s — a finding that repeats is a predicate nobody has written", finding.Check, number, finding.Detail))
			continue
		}
		reworks = append(reworks, fmt.Sprintf("%s fired: %s", finding.Check, finding.Detail))
	}

	// A check about the act that could not be worked. Inside the episode is
	// the repair's own doing and its row below reworks it; outside is a person's.
	inside := res.Status == Undecided || (res.Unchanged && !a.Entry.Anchored)
	if !inside {
		for _, finding := range a.UnreadOfTheAct() {
			escalations = append(escalations, fmt.Sprintf("%s could not be worked: %s", finding.Check, finding.Detail))
		}
	}

	// The pass. Only its ruling is read, and a pass that argued without one is
	// never read as an accept.
	pass := res.Argument
	switch {
	case pass == nil && assessorConfigured:
		escalations = append(escalations, "the adversarial pass could not be worked, so nothing ruled on this attempt")
	case pass == nil, pass.Status == Unconfigured, pass.Status == Unanchored:
	case pass.Status == Argued && !a.Argument.Ran:
		escalations = append(escalations, "the adversarial pass argued and its argument cannot be read back out of the thread")
	case pass.Status == Argued && a.Argument.Ruling == thread.RulingReject:
		reworks = append(reworks, "the adversarial pass ruled reject")
	case pass.Status == Argued && a.Argument.Ruling == thread.RulingAccept:
	case pass.Status == Argued:
		escalations = append(escalations, "the adversarial pass argued and made no ruling kept can read: "+a.Argument.RulingWhy)
	default:
		escalations = append(escalations, fmt.Sprintf("the adversarial pass reached no argument (%s): %s", pass.Status, pass.Detail))
	}

	// The promise, in the tree the attempt left.
	switch {
	case res.Regressed:
		reworks = append(reworks, fmt.Sprintf("the work re-derived %s where the anchor it was cut at holds, so it was not anchored and the next attempt starts from that anchor", valueWord(res.After.Value)))
	case res.Status == Unrepaired:
		reworks = append(reworks, "the promise still reads not-kept in the tree this attempt left")
	case res.Status == Undecided:
		reworks = append(reworks, "nothing could measure the promise in the tree this attempt left: "+firstLineOf(res.After.Detail))
	}

	switch {
	case len(escalations) > 0:
		return ActionEscalate, escalations
	case len(reworks) > 0:
		return ActionRework, reworks
	}
	why := "the promise holds and every check about the act is clear"
	switch {
	case pass != nil && pass.Status == Argued:
		why += ", and the adversarial pass ruled accept"
	case pass != nil && pass.Status == Unconfigured:
		why += ", and no assessor is configured on this machine to argue with it"
	}
	return ActionReady, []string{why}
}

// openRework resolves `kept repair --rework <thread>` into the plan for its next
// attempt, and refuses a thread that cannot be reworked however the machine is
// configured. None of those refusals is recorded, for the reason an
// unrepairable promise is not: each is re-derivable from the corpus and the
// ledger at any later time.
func openRework(ctx context.Context, req *Request, id thread.ID) (*reworkPlan, []thread.Entry, error) {
	entries, _, err := thread.ReadReceipt(req.Store, id)
	if errors.Is(err, ledger.ErrNoStream) || (err == nil && len(entries) == 0) {
		return nil, nil, &Unrepairable{Why: fmt.Sprintf("thread %s has no receipt in this ledger, so there is nothing to rework", id)}
	}
	if err != nil {
		return nil, nil, err
	}
	attempts := thread.Attempts(entries)
	if len(attempts) == 0 {
		return nil, nil, &Unrepairable{Why: fmt.Sprintf("thread %s holds no repair packet, so there is no repair to rework", id)}
	}
	contents, err := lastPacket(req.Store, attempts[len(attempts)-1])
	if err != nil {
		return nil, nil, err
	}
	promise := contents.Promise.ID
	anchor, anchored := thread.LatestAnchor(entries)
	if !anchored {
		return nil, nil, &Unrepairable{Why: fmt.Sprintf("thread %s anchored nothing, so there is no work to rework; kept repair %s opens a new episode", id, promise)}
	}
	req.Promise = promise
	_, standing, err := subject(*req)
	if err != nil {
		return nil, nil, err
	}
	// A promise re-ratified at another wording is a new standard, and a rework
	// would measure the old one in a box cut before the revision and call that
	// the gap.
	if standing.Hash.Digest != contents.Promise.Hash || standing.Hash.Norm != contents.Promise.Norm {
		return nil, nil, &Unrepairable{Why: fmt.Sprintf("%s has been ratified at a different wording since thread %s acted on it, so a rework would measure a standard nobody holds now; kept repair %s opens a new episode", promise, id, promise)}
	}
	readings, err := assess.Attempts(ctx, assess.Request{
		Store:      req.Store,
		Repository: req.Workspace,
		Corpus:     req.Corpus,
		Thread:     id,
		Deadline:   req.Deadlines.Git,
	})
	if err != nil {
		return nil, nil, err
	}
	plan, err := planFrom(ctx, *req, id, entries, readings, anchor.Parent, anchor.Commit)
	if plan != nil {
		plan.readings = readings
	}
	return plan, entries, err
}

// nextPlan is the plan for the attempt after one this run just opened.
func nextPlan(ctx context.Context, req Request, ep *Episode, id thread.ID, entries []thread.Entry, readings []assess.AttemptReading) (*reworkPlan, error) {
	if anchor, anchored := thread.LatestAnchor(entries); anchored {
		return planFrom(ctx, req, id, entries, readings, anchor.Parent, anchor.Commit)
	}
	// Nothing in the episode has anchored anything, so the tip is still where
	// the first box was cut, and the base is the same commit.
	cut := ep.Attempts[0].Commit
	return planFrom(ctx, req, id, entries, readings, cut, cut)
}

// planFrom assembles a rework's plan from the thread as it stands.
func planFrom(ctx context.Context, req Request, id thread.ID, entries []thread.Entry, readings []assess.AttemptReading, base, tip string) (*reworkPlan, error) {
	plan := &reworkPlan{
		Thread:  id,
		Base:    base,
		Tip:     tip,
		Earlier: earlierAttempts(readings),
		Lineage: lineageOf(req.Store, id, entries),
	}
	if base != tip {
		changed, err := assess.Diff(ctx, req.Workspace, thread.Anchor{Commit: tip, Parent: base, Promise: req.Promise}, req.Deadlines.Git)
		if err == nil {
			plan.Diff, plan.Elided = bounded(changed, briefBound)
		} else {
			plan.Elided = "the change so far could not be read: " + err.Error()
		}
		if len(readings) > 0 {
			for _, touched := range readings[len(readings)-1].Assessment.Entry.Touched {
				plan.Touched = append(plan.Touched, thread.BriefPath{Path: touched.Path, Status: string(touched.Status), From: touched.From})
			}
		}
	}
	return plan, nil
}

// lineageOf is a thread's own measurements, read back out of the observation
// stream the receipt names them in, keyed by instrument and in receipt order.
//
// Only the thread's own: a measurement that names no thread was taken before a
// box named its episode, and handing it to a box as this lineage would be the
// mistake [tick.MeasureRequest.History] refuses. A measurement the store no
// longer holds is left out, which is what pruning costs prior() everywhere.
func lineageOf(store ledger.Store, id thread.ID, entries []thread.Entry) map[string][]model.Observation {
	out := map[string][]model.Observation{}
	for _, named := range thread.Measurements(entries) {
		observation, err := ledger.ReadObservation(store, named.Address)
		if err != nil || observation.World.Thread != string(id) {
			continue
		}
		out[named.Name] = append(out[named.Name], model.Observation{
			ID:          observation.Address(),
			Gatherer:    observation.Gatherer,
			Args:        observation.Args,
			ScopeDigest: observation.Scope,
			World:       observation.World,
			At:          observation.At,
			Data:        observation.Data,
		})
	}
	if len(out) == 0 {
		return nil
	}
	for _, name := range slices.Sorted(maps.Keys(out)) {
		out[name] = slices.Clip(out[name])
	}
	return out
}

// lastPacket reads the packet an attempt carried.
func lastPacket(store ledger.Store, attempt thread.Attempt) (thread.Contents, error) {
	src, err := ledger.ReadArtifact(store, attempt.Repair.Input)
	if err != nil {
		return thread.Contents{}, fmt.Errorf("read the packet thread's last attempt carried: %w", err)
	}
	return thread.ReadPacket(src)
}

// escalate hands the episode to a person, and records that it did.
//
// It is the disposition of the attempt the episode declined to open, so each
// attempt still writes exactly one, and it names the thread: the person it is
// written for has to know which one to read, rework or land. See
// docs/decision/0086.
func escalate(req Request, ep *Episode, why []string) error {
	ep.End, ep.Why = Escalated, why
	_, standing, err := subject(req)
	if err != nil {
		return err
	}
	written, recorded, err := dispose(req, ep.Promise, standing, ledger.OutcomeEscalate, "", string(ep.Thread), strings.Join(why, "; "))
	if err != nil {
		return err
	}
	ep.Handoff, ep.Recorded = written, recorded
	return nil
}

// refuseBudget records a budget that refused before the run opened any attempt.
func refuseBudget(req Request, ep *Episode, said string) error {
	ep.End, ep.Why = Stopped, []string{said}
	_, standing, err := subject(req)
	if err != nil {
		return err
	}
	detail := fmt.Sprintf("thread %s was not reworked: %s", ep.Thread, said)
	written, recorded, err := dispose(req, ep.Promise, standing, ledger.OutcomeRefused, ledger.Budget, "", detail)
	if err != nil {
		return err
	}
	ep.Handoff, ep.Recorded = written, recorded
	return nil
}

// stoppedWhy is the sentence a run that stopped at an undispatched attempt ends
// with.
func stoppedWhy(res *Result) string {
	switch res.Status {
	case Holds:
		return "the promise holds where the box was cut, so there is nothing to repair"
	case Unmeasured:
		return "nothing could measure the promise in the box, so there is no evidence to carry"
	case Prepared:
		return "the packet is assembled and the provider verified, and nothing was dispatched"
	case Refused:
		return fmt.Sprintf("refused on %s: %s", res.Condition, firstLineOf(res.Detail))
	}
	return string(res.Status)
}
