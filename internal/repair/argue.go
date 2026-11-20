package repair

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/pbleisch/kept/internal/assess"
	"github.com/pbleisch/kept/internal/box"
	"github.com/pbleisch/kept/internal/config"
	"github.com/pbleisch/kept/internal/confine"
	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/progress"
	"github.com/pbleisch/kept/internal/thread"
)

// briefBound is how much of the anchor's diff and of the repair's transcript one
// brief carries.
//
// A bound rather than everything, because the pass reads its brief as one
// message and a runtime that refuses an oversized one refuses it at the launch,
// which is the worst place to find out. Elision is said out loud in the brief
// itself — a pass arguing about a change it was shown half of, and not told so,
// would be arguing about something that did not happen.
//
// It is not a configuration key. What it bounds is kept's own message to a
// runtime kept chose, with no per-repository answer, and the deadlines an
// operator does set bound the things that vary. See docs/decision/0026 for the
// same rule about the canary.
const briefBound = 256 << 10

// ArgueStatus is how far one adversarial pass got.
//
// Five outcomes and not a boolean, for the reason a repair has seven: a pass
// nobody configured, a thread with nothing to argue about, a session that was
// refused, a session that ran and concluded, and a session that ran and left no
// conclusion are five different things to do next, and a reader that could not
// tell them apart would read the first as the last.
type ArgueStatus string

const (
	// Argued: a session ran and its argument is in the receipt.
	Argued ArgueStatus = "argued"
	// Unargued: a session ran and left no conclusion — killed at its deadline
	// before it wrote anything, or exited having written nothing. It is not a
	// pass that found nothing: *nobody argued this* and *the argument found
	// nothing* are the distinction the whole three-valued reading rests on.
	Unargued ArgueStatus = "unargued"
	// Unconfigured: this machine names no assessor, so no pass ran. It is an
	// ordinary state and never an error — nobody has set one up.
	Unconfigured ArgueStatus = "unconfigured"
	// Unanchored: the thread anchored nothing, so there is no act to argue
	// about. A repair that changed nothing is a failed repair and the
	// re-derivation already says so.
	Unanchored ArgueStatus = "unanchored"
	// ArgueRefused: a condition refused and nothing started.
	ArgueRefused ArgueStatus = "refused"
	// ArgueExhausted: a session ran and ended because the account it ran on had
	// no capacity left. Whatever it wrote stays in the receipt and is not filed
	// as an argument, so no ruling is read from it: a pass the account cut off
	// did not conclude, and the episode waits to argue again rather than acting
	// on half an argument. See docs/decision/0090.
	ArgueExhausted ArgueStatus = "exhausted"
)

// ArgueRequest is one adversarial pass over one thread.
type ArgueRequest struct {
	// Workspace is the repository the box is cut from, and the anchor's commit
	// must be reachable in it.
	Workspace string
	// Thread is the episode to argue about.
	Thread thread.ID

	// Corpus is the repository as it stands now, for the one derived check that
	// asks what else reads these paths. Nil is a caller with none.
	Corpus *corpus.Corpus

	// Provider is .kept/config.yaml's confine.provider, and Assessor is the
	// per-machine assessor runtime. The two halves are in different files for
	// docs/decision/0048's reason.
	Provider  string
	Assessor  config.Assess
	Deadlines Deadlines

	// Store is where the thread's entry is read from, and Ledger is where the
	// brief, the argument and the session's own evidence are written.
	Store  ledger.Store
	Ledger *ledger.Ledger

	Progress *progress.Display

	// Keep retains the read-only box after the pass instead of discarding it.
	// It is what makes *the session could not write* something a caller can
	// measure against the tree rather than take on the canary's word.
	Keep bool
}

// ArgueResult is one adversarial pass, whole.
//
// There is no verdict field and there will not be one. The pass's findings are
// unbounded and unslotted, so nothing here grades, counts or categorizes them:
// what it produced is [ArgueResult.Said], in the words it wrote. The one thing
// read out of those words is the ruling their last line makes — whether the
// case is one another attempt should answer — and it is read, not recorded, and
// it is not a grade. See docs/decision/0078 and 0086.
type ArgueResult struct {
	Thread thread.ID
	Status ArgueStatus
	// Detail says why, for the statuses that are not a session that concluded.
	Detail string

	// Box is where the read-only box is when it was kept, and Unwritable is
	// what the canary was refused in — reported because it is the whole of the
	// difference between this box and a repair's, and a person reading a pass
	// should be able to see what was actually enforced.
	Box        string
	Unwritable []string

	// Brief is the evidence the pass was given, and Argument is the receipt
	// entry naming what it concluded. Said is those words.
	Brief    ledger.ArtifactRef
	Argument thread.ArgumentRecord
	Said     string

	// Ruling is what the last line of Said rules, and RulingWhy says why there is
	// none when it is empty. See [thread.ReadRuling].
	Ruling    thread.Ruling
	RulingWhy string

	// Exhausted is the runtime's account of the capacity it ran out of, set
	// exactly when Status is [ArgueExhausted].
	Exhausted *thread.Exhaustion

	// Warning is something that went wrong after the result was decided and
	// does not change it — a box that would not discard.
	Warning string
}

// Argue runs one adversarial pass over one thread's ledger entry.
//
// It cuts a **read-only** box at the anchor the repair produced, assembles the
// thread's entry into a brief, dispatches a confined session with the assessor's
// own declared ask, and files what came back. Nothing it starts can write to the
// tree it is judging: a session that could edit the code it argues about could
// make its own case true, and the canary is refused in exactly the directories a
// repair of the same promise would have been granted.
//
// It writes no disposition. A disposition is a decision about a drift
// (docs/decision/0019), and this is a finding about an act that already
// happened; what an outcome selects is the dispatch policy of open question 3
// and is not settled here. What it durably leaves is the brief, the session's
// own evidence, and the argument — all in the thread's receipt, because a
// session is not reproducible and
// policy:durability-follows-reproducibility is what decides that.
//
// See docs/decision/0078 and 0081.
func Argue(ctx context.Context, req ArgueRequest) (*ArgueResult, error) {
	res := &ArgueResult{Thread: req.Thread}
	if req.Assessor.Runtime == "" {
		// Nobody has configured one. It is reported rather than refused: a
		// repair still runs and still reports, and the pass saying it did not
		// run is the honest answer rather than a failure.
		res.Status, res.Detail = Unconfigured, "no assessor runtime is named in the operator's config, so no adversarial pass read this thread"
		return res, nil
	}
	if req.Deadlines.Agent <= 0 {
		return refuseArgument(res, "timeouts.agent is unset in .kept/config.yaml, and nothing external runs without a deadline"), nil
	}
	provider, err := providerExecutable(req.Provider)
	if err != nil {
		return refuseArgument(res, err.Error()), nil
	}

	reading := phaseOf(req.Progress, "read thread "+string(req.Thread)+" out of the ledger", req.Deadlines.Git)
	assessment, err := assess.Assess(ctx, assess.Request{
		Store:      req.Store,
		Repository: req.Workspace,
		Corpus:     req.Corpus,
		Thread:     req.Thread,
		Deadline:   req.Deadlines.Git,
	})
	reading()
	if err != nil {
		return nil, err
	}
	if !assessment.Entry.Anchored {
		res.Status, res.Detail = Unanchored, "the thread anchored no commit, so there is no act for a pass to argue about"
		return res, nil
	}

	entries, _, err := thread.ReadReceipt(req.Store, req.Thread)
	if err != nil {
		return nil, err
	}
	// The attempts before the one argued over here, which is how a pass over a
	// rework sees the case it is being answered on. See docs/decision/0086.
	readings, err := assess.Attempts(ctx, assess.Request{
		Store:      req.Store,
		Repository: req.Workspace,
		Corpus:     req.Corpus,
		Thread:     req.Thread,
		Deadline:   req.Deadlines.Git,
	})
	if err != nil {
		return nil, err
	}
	var earlier []thread.EarlierAttempt
	if len(readings) > 1 {
		earlier = earlierAttempts(readings[:len(readings)-1])
	}
	brief, err := assembleBrief(req, assessment, entries, earlier)
	if err != nil {
		return nil, err
	}

	cutting := phaseOf(req.Progress, "cut a read-only box at the anchor", req.Deadlines.Git)
	created, home, err := cutAt(ctx, req.Workspace, assessment.Entry.Anchor.Commit, req.Deadlines.Git)
	cutting()
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = os.RemoveAll(home)
		if req.Keep {
			_ = os.RemoveAll(created.Temp())
			res.Box = created.Path()
			return
		}
		if err := created.Close(context.Background()); err != nil {
			res.Warning = fmt.Sprintf("the box at %s could not be discarded, and is a registered worktree in this repository: %s", created.Path(), err)
		}
	}()

	assembling := phaseOf(req.Progress, "locate and check the "+req.Assessor.Runtime+" runtime", 0)
	assembled, err := AssembleAssessor(ctx, req.Assessor, Session{Home: home, Temp: created.Temp()})
	assembling()
	if err != nil {
		return refuseArgument(res, err.Error()), nil
	}
	rendering := phaseOf(req.Progress, "render the "+req.Provider+" read-only profile and ask it about every path", 0)
	profile, err := confinement(ctx, boxPolicy{
		Workspace: req.Workspace,
		Box:       created,
		Home:      home,
		Operator:  req.Assessor.Repair,
		Runtime:   assembled,
		Deadlines: req.Deadlines,
		// The promise as the packet froze it, rather than as the corpus holds
		// it now. The ordinary next step after a finding is revising the
		// promise, and a box rendered from today's wording would be refused in
		// paths this repair was never given.
		Promise: assessment.Entry.Packet.Promise.ID,
		Gather:  assessment.Entry.Packet.Promise.Gather,
		Scope:   assessment.Entry.Packet.Promise.RepairScope,
		// The whole of the difference from a repair's box.
		ReadOnly: true,
	}, provider)
	rendering()
	if err != nil {
		return refuseArgument(res, err.Error()), nil
	}
	res.Unwritable = append([]string(nil), profile.Unwritable...)
	launch, err := confine.NewNono(profile)
	if err != nil {
		return refuseArgument(res, err.Error()), nil
	}
	verifying := phaseOf(req.Progress, "verify the read-only box with the canary", canaryDeadline)
	err = launch.Verified()
	verifying()
	if err != nil {
		return refuseArgument(res, err.Error()), nil
	}
	// The receipt is the thread's own, adopted rather than minted: the pass is
	// part of the episode it argues about, and a receipt of its own would leave
	// the argument in a stream nothing connects to the repair. See
	// docs/decision/0079.
	receipt, err := thread.NewReceipt(req.Ledger, req.Thread)
	if err != nil {
		return nil, err
	}
	deadline, cancel := context.WithTimeout(ctx, req.Deadlines.Agent+dispatchMargin)
	defer cancel()
	session := phaseOf(req.Progress, "thread "+string(req.Thread)+": a confined "+req.Assessor.Runtime+" session is arguing", req.Deadlines.Agent)
	err = thread.Dispatch(deadline, assembled.Config, brief, launch, thread.Supervisor{Timeout: req.Deadlines.Agent}, receipt)
	session()
	if err != nil {
		var refusal *thread.Refusal
		if errors.As(err, &refusal) {
			return refuseArgument(res, refusal.Error()), nil
		}
		return nil, err
	}

	return fileArgument(req, res, assembled)
}

// fileArgument reads what the pass wrote back out of the receipt and records it
// as the argument.
//
// The words are read back rather than captured on the way past, and that is not
// a detour. What a session said is durable the moment the adapter projects it,
// so composing the argument from the receipt is composing it from the record
// rather than from a copy this process happened to hold — and it is the same
// text a reader of the receipt will see. What the argument entry adds is that
// these fragments were one conclusion, reached from one brief, by one runtime;
// a box is discarded and a stream of fragments does not say that about itself.
func fileArgument(req ArgueRequest, res *ArgueResult, assembled Runtime) (*ArgueResult, error) {
	entries, _, err := thread.ReadReceipt(req.Store, req.Thread)
	if err != nil {
		return nil, err
	}
	sessions := thread.Sessions(entries)
	if len(sessions) == 0 || sessions[len(sessions)-1].Kind != ledger.ThreadBrief {
		return nil, fmt.Errorf("the pass's own brief is not the last input in thread %s, so its evidence cannot be told from the repair's", req.Thread)
	}
	pass := sessions[len(sessions)-1]
	said, redaction := transcriptOf(req.Store, pass.Entries)
	res.Brief = pass.Input
	// A pass the account cut off did not conclude, whatever it wrote first: its
	// last line is the ruling, and a session refused partway never reached the
	// line it would have ended on. Its words are in the receipt as it streamed
	// them, and they are not filed as an argument, because a filed argument is
	// what every reader takes a ruling from. See docs/decision/0090.
	if exhausted, out := thread.OutOfCapacity(pass); out {
		res.Status, res.Exhausted = ArgueExhausted, &exhausted
		res.Detail = "the account the pass ran on had no capacity left: " + exhausted.Said
		return res, nil
	}
	if strings.TrimSpace(said) == "" {
		// A session that ran and left nothing. It is reported rather than
		// filed: an argument entry over no words would say a pass concluded
		// when it did not, which is the one thing the entry exists to be able
		// to say the opposite of.
		//
		// The runtime's own account of why goes in the detail, because *nothing
		// to say* and *never reached a model* are the same receipt otherwise and
		// only the runtime can tell them apart. It is said here and it decides
		// nothing: a session that argued and then met a limit on its last
		// request argued, and discarding its words on this flag would throw away
		// the evidence the pass exists to produce. See docs/decision/0086, whose
		// row for a pass that reached no argument escalates this to a person.
		res.Status = Unargued
		res.Detail = "the pass ran and wrote nothing, so there is no argument to file"
		if why := thread.TerminalError(pass); why != "" {
			res.Detail += "; the runtime reported " + why
		}
		return res, nil
	}

	stored, err := req.Ledger.PutArtifact(ledger.ThreadArgumentArtifact, []byte(said))
	if err != nil {
		return nil, fmt.Errorf("store what the pass argued: %w", err)
	}
	adapter, err := thread.Select(assembled.Config.Runtime)
	if err != nil {
		return nil, err
	}
	record := thread.ArgumentRecord{
		Runtime:        adapter.Runtime(),
		AdapterVersion: adapter.AdapterVersion(assembled.Config),
		Brief:          pass.Input,
		Artifact:       stored,
		Redaction:      redaction,
	}
	receipt, err := thread.NewReceipt(req.Ledger, req.Thread)
	if err != nil {
		return nil, err
	}
	if err := receipt.RecordArgument(record); err != nil {
		return nil, err
	}
	res.Status, res.Argument, res.Said = Argued, record, said
	res.Ruling, res.RulingWhy = thread.ReadRuling(said)
	return res, nil
}

// assembleBrief turns one thread's ledger entry into the bytes a pass receives.
//
// Everything in it is already durable, and every piece is read from the record
// rather than measured again: the packet is the frozen authority the repair
// acted under, the two measurements are addressed in the observation stream, the
// diff is the anchor commit's own against its parent, and the transcript is what
// the repair session wrote. The derived findings are the closed half's, worked
// out on read here exactly as `kept review` works them out.
func assembleBrief(req ArgueRequest, assessment assess.Assessment, entries []thread.Entry, earlier []thread.EarlierAttempt) (thread.Brief, error) {
	said, _ := transcriptOf(req.Store, repairSession(entries))
	transcript, transcriptElided := bounded(said, briefBound)

	entry := assessment.Entry
	diff, diffElided := "", ""
	if changed, err := assess.Diff(context.Background(), req.Workspace, entry.Anchor, req.Deadlines.Git); err == nil {
		diff, diffElided = bounded(changed, briefBound)
	} else {
		diffElided = "the anchor's diff could not be read: " + err.Error()
	}

	var touched []thread.BriefPath
	for _, t := range entry.Touched {
		touched = append(touched, thread.BriefPath{Path: t.Path, Status: string(t.Status), From: t.From})
	}
	derived := briefFindings(assessment.Findings)
	elided := strings.TrimSpace(strings.Join([]string{diffElided, transcriptElided}, " "))
	return thread.AssembleBrief(thread.BriefInput{
		Thread:     string(req.Thread),
		Packet:     entry.Packet,
		Anchor:     entry.Anchor,
		Touched:    touched,
		Diff:       diff,
		Elided:     elided,
		Before:     entry.Before,
		After:      entry.After,
		Transcript: transcript,
		Lifecycle:  entry.Lifecycle,
		Derived:    derived,
		Earlier:    earlier,
	})
}

// earlierAttempts is an episode's attempts as the next packet and the next brief
// carry them: what each left, what the checklist found, and what the pass over
// it argued and ruled — the argument whole, and the ruling the one line read out
// of it.
func earlierAttempts(readings []assess.AttemptReading) []thread.EarlierAttempt {
	out := make([]thread.EarlierAttempt, 0, len(readings))
	for _, reading := range readings {
		attempt := thread.EarlierAttempt{Rederived: reading.After}
		if reading.Anchored {
			anchor := reading.Anchor
			attempt.Anchor = &anchor
		}
		attempt.Derived = briefFindings(reading.Assessment.Findings)
		if argument := reading.Assessment.Argument; argument.Ran && strings.TrimSpace(argument.Said) != "" {
			attempt.Argument = argument.Said
			if argument.Ruling.Known() {
				attempt.Ruling = argument.Ruling
			}
		}
		out = append(out, attempt)
	}
	return out
}

// briefFindings is the closed checklist's findings on the wire, all of them and
// in checklist order.
func briefFindings(findings []assess.Finding) []thread.BriefFinding {
	var out []thread.BriefFinding
	for _, finding := range findings {
		out = append(out, thread.BriefFinding{
			Check:   string(finding.Check),
			Outcome: string(finding.Outcome),
			Detail:  finding.Detail,
			Lines:   finding.Lines,
		})
	}
	return out
}

// repairSession is the entries belonging to the last repair dispatch in a
// thread.
//
// The last, because a thread is an episode and a rework appends to it; the one
// a pass argues about is the attempt that produced the anchor it is looking at.
// A thread with no packet at all yields nothing, which reads downstream as a
// brief carrying no transcript — which is what a thread nobody dispatched
// actually has.
func repairSession(entries []thread.Entry) []thread.Entry {
	for _, session := range reverse(thread.Sessions(entries)) {
		if session.Kind == ledger.ThreadPacket {
			return session.Entries
		}
	}
	return nil
}

func reverse(sessions []thread.Session) []thread.Session {
	out := make([]thread.Session, 0, len(sessions))
	for i := len(sessions) - 1; i >= 0; i-- {
		out = append(out, sessions[i])
	}
	return out
}

// transcriptOf is what one session wrote, in the order it wrote it, and what was
// taken out of it on the way.
//
// A fragment that cannot be read is skipped rather than fatal, for the reason a
// pruned observation is: a transcript is durable but a store is not infallible,
// and losing a fragment costs the reader a paragraph rather than the whole
// entry. The redaction is summed across the fragments, because a record saying
// nothing was redacted when one fragment's was would be the one claim about a
// credential that must not be wrong.
func transcriptOf(store ledger.Store, entries []thread.Entry) (string, thread.Redaction) {
	var parts []string
	var redaction thread.Redaction
	for _, entry := range entries {
		if entry.Transcript == nil {
			continue
		}
		if entry.Transcript.Redaction.Occurred {
			redaction.Occurred = true
		}
		redaction.Matches += entry.Transcript.Redaction.Matches
		src, err := ledger.ReadArtifact(store, entry.Transcript.Artifact)
		if err != nil {
			continue
		}
		parts = append(parts, string(src))
	}
	return strings.Join(parts, ""), redaction
}

// bounded cuts a payload to what a brief carries and says what it left out.
//
// The second return is the sentence the brief prints about it. Silence would be
// worse than the truncation: a pass arguing about a diff it was shown the first
// half of, and not told so, would be arguing about a change that did not happen.
func bounded(text string, limit int) (string, string) {
	if len(text) <= limit {
		return text, ""
	}
	// Back off to a rune boundary. A cut through a multi-byte character
	// encodes as a replacement rune, which is a byte the runtime never emitted
	// arriving in evidence about what it emitted.
	cut := limit
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut], fmt.Sprintf("%d of %d bytes were left out of this brief because it carries at most %d.", len(text)-cut, len(text), limit)
}

// cutAt cuts a box at one commit, with the session home beside it.
//
// It is [cut]'s other caller and takes the commit rather than resolving HEAD: a
// pass stands in the tree the repair left, which is the anchor, and a box cut at
// the workspace's own HEAD would be a box arguing about somewhere else. See
// docs/decision/0079 for why the box is cut again rather than kept.
func cutAt(ctx context.Context, workspace, commit string, deadline time.Duration) (*box.Box, string, error) {
	created, err := box.Create(ctx, box.Request{Repository: workspace, Commit: commit, Deadline: deadline})
	if err != nil {
		return nil, "", err
	}
	home := created.Path() + ".home"
	if err := os.MkdirAll(home, 0o700); err != nil {
		_ = created.Close(context.Background())
		return nil, "", fmt.Errorf("create the assessor session home: %w", err)
	}
	return created, home, nil
}

func refuseArgument(res *ArgueResult, detail string) *ArgueResult {
	res.Status, res.Detail = ArgueRefused, detail
	return res
}

// phaseOf is [phase] for a caller that has a display rather than a whole
// request.
func phaseOf(display *progress.Display, words string, bound time.Duration) func() {
	return display.Begin(progress.Step{Kind: progress.Repair, Detail: words, Bound: bound})
}
