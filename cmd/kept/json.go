package main

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/pbleisch/kept/internal/assess"
	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/register"
	"github.com/pbleisch/kept/internal/thread"
	"github.com/pbleisch/kept/internal/tick"
)

// Schema is the version on every structured payload.
//
// A consumer that cannot tell which shape it is holding will guess, and guess
// wrong exactly once — so the version is on the document from the first
// retire, exactly as it is on every ledger record. It is one envelope for
// status, drift, why and the tick, because four shapes for one set of facts is
// four things to version. See docs/decision/0027.
const Schema = "kept/v1"

// document is the machine-facing answer.
//
// The verdict in it is a **three-valued string** and never a boolean, and that
// is the whole reason this shape was held back until truth existed. A boolean
// here would collapse "the promise is broken" and "the toolchain was missing"
// into one value, which is the false report the three-valued model exists to
// prevent, and CI is exactly where that collapse does damage. The reason nobody
// could measure travels beside it, because a value of "unknown" with no reason
// is a shrug in a machine-readable wrapper.
type document struct {
	Schema string `json:"schema"`
	// Verb is which question this answers. The shape does not vary with it;
	// what is absent varies, and every absence is meaningful.
	Verb string `json:"verb"`
	// Kept is the build that answered.
	Kept string    `json:"kept"`
	At   time.Time `json:"at"`
	// Exit is the code this invocation returned, so a consumer reading a saved
	// document knows what the process said. See docs/decision/0027.
	Exit int `json:"exit"`
	// World is the tree this was answered against. Absent for a question that
	// did not look at one.
	World *worldPayload `json:"world,omitempty"`

	Artifacts   []artifactPayload    `json:"artifacts"`
	Summary     map[string]int       `json:"summary"`
	Problems    []problemPayload     `json:"problems,omitempty"`
	Quarantined []quarantinedPayload `json:"quarantined,omitempty"`

	// Assessment is one thread's checklist, and it is absent on every verb
	// that is not review. That is the envelope working as it was designed to:
	// the shape does not vary with the question, what is absent varies, and a
	// second document for a fifth verb would be a second thing to version. See
	// docs/decision/0027 and 0078.
	Assessment *assessmentPayload `json:"assessment,omitempty"`
}

// assessmentPayload is one repair thread, judged.
//
// There is no verdict field, and its absence is the design rather than an
// omission: which check fired is the whole diagnostic value, and a boolean over
// the six would be exactly the summary a consumer would then gate on. A
// consumer wanting one counts the outcomes, which makes it their reading rather
// than kept's.
type assessmentPayload struct {
	Thread  string `json:"thread"`
	Promise string `json:"promise,omitempty"`
	// Anchor is the commit the session left and the commit the box was cut
	// from, absent when the repair anchored nothing.
	Anchor *anchorPayload `json:"anchor,omitempty"`
	// Findings is one per derived check, in checklist order, always all of
	// them. A check that found nothing and a check nothing could work are both
	// here, because a consumer that saw only what fired could not tell a clean
	// checklist from an unread one.
	//
	// It is half the assessor. A consumer reading it as the whole of one has
	// read the closed half and missed the argument.
	Findings []findingPayload `json:"findings"`
	Argument argumentPayload  `json:"argument"`
	Grade    gradePayload     `json:"grade"`
	// Session is how far the repair session got, derived from the receipt, and
	// absent for a receipt with no entries. It is what `kept repair` prints when
	// a repair fails, so a consumer and a person read one account of it. See
	// docs/decision/0085.
	Session *sessionPayload `json:"session,omitempty"`
	// Attempts is every attempt of the episode, oldest first, each assessed as
	// the thread stood when it was done. See docs/decision/0086.
	Attempts []attemptPayload `json:"attempts,omitempty"`
	// Escalation is the disposition that handed this thread to a person, when
	// it is the last one naming the thread.
	Escalation *escalationPayload `json:"escalation,omitempty"`
}

// attemptPayload is one attempt of an episode on the machine surface.
//
// Ruling is the one token read out of what the pass said, and it is empty when
// none could be read; RulingWhy says why, or carries the pass's own reason for
// an out-of-reach ruling (0096). The words themselves are the
// assessment's argument, and they are not repeated here per attempt.
type attemptPayload struct {
	Number    int      `json:"number"`
	Cut       string   `json:"cut,omitempty"`
	Anchor    string   `json:"anchor,omitempty"`
	Before    string   `json:"before,omitempty"`
	After     string   `json:"after,omitempty"`
	Fired     []string `json:"fired,omitempty"`
	Unread    []string `json:"unread,omitempty"`
	Argued    bool     `json:"argued"`
	Ruling    string   `json:"ruling,omitempty"`
	RulingWhy string   `json:"ruling_why,omitempty"`
}

// escalationPayload is an episode handed to a person, and why.
type escalationPayload struct {
	At     time.Time `json:"at"`
	Thread string    `json:"thread,omitempty"`
	Detail string    `json:"detail"`
}

func attemptsPayload(readings []assess.AttemptReading) []attemptPayload {
	var out []attemptPayload
	for _, reading := range readings {
		payload := attemptPayload{
			Number:    reading.Number,
			Cut:       reading.Cut,
			Before:    string(reading.Before),
			After:     string(reading.After),
			Argued:    reading.Assessment.Argument.Ran,
			Ruling:    string(reading.Assessment.Argument.Ruling),
			RulingWhy: reading.Assessment.Argument.RulingWhy,
		}
		if reading.Anchored {
			payload.Anchor = reading.Anchor.Commit
		}
		for _, finding := range reading.Assessment.FiredOfTheAct() {
			payload.Fired = append(payload.Fired, string(finding.Check))
		}
		for _, finding := range reading.Assessment.UnreadOfTheAct() {
			payload.Unread = append(payload.Unread, string(finding.Check))
		}
		out = append(out, payload)
	}
	return out
}

// sessionPayload is a [thread.Diagnosis] on the machine surface. Every time is
// receipt time — when kept recorded something, not when the runtime did it.
type sessionPayload struct {
	Runtime       string             `json:"runtime,omitempty"`
	Started       *time.Time         `json:"started,omitempty"`
	Ended         *time.Time         `json:"ended,omitempty"`
	Exit          *thread.Exit       `json:"exit,omitempty"`
	TimedOut      string             `json:"timed_out,omitempty"`
	Killed        bool               `json:"killed"`
	Events        int                `json:"events"`
	LastEvent     string             `json:"last_event,omitempty"`
	LastEventAt   *time.Time         `json:"last_event_at,omitempty"`
	Turns         uint64             `json:"turns"`
	Model         string             `json:"model,omitempty"`
	Closed        bool               `json:"closed"`
	ClosingEvent  string             `json:"closing_event,omitempty"`
	Undecodable   int                `json:"undecodable"`
	Unterminated  bool               `json:"unterminated"`
	Launch        *thread.Executable `json:"launch,omitempty"`
	AtExit        *thread.Executable `json:"at_exit,omitempty"`
	Replaced      bool               `json:"replaced"`
	Stderr        *thread.Stderr     `json:"stderr,omitempty"`
	Anchored      bool               `json:"anchored"`
	AfterMeasured bool               `json:"after_measured"`
}

func sessionDocument(d thread.Diagnosis) *sessionPayload {
	at := func(t time.Time) *time.Time {
		if t.IsZero() {
			return nil
		}
		return &t
	}
	payload := &sessionPayload{
		Runtime:       string(d.Runtime),
		Started:       at(d.Started),
		Ended:         at(d.Ended),
		Exit:          d.Exit,
		TimedOut:      d.TimedOut,
		Killed:        d.Killed,
		Events:        d.Natives,
		LastEvent:     d.LastNative,
		LastEventAt:   at(d.LastNativeAt),
		Turns:         d.Turns,
		Model:         d.Model,
		Closed:        d.Closed,
		ClosingEvent:  d.ClosingEvent,
		Undecodable:   d.Undecodable,
		Unterminated:  d.Unterminated,
		Replaced:      d.Replaced(),
		Stderr:        d.Stderr,
		Anchored:      d.Anchored,
		AfterMeasured: d.AfterMeasured,
	}
	if d.LaunchObserved {
		launch := d.Launch
		payload.Launch = &launch
	}
	if d.ExitedObserved {
		exit := d.AtExit
		payload.AtExit = &exit
	}
	return payload
}

// argumentPayload is the adversarial pass.
//
// It carries **no findings field**, and that absence is load-bearing now that
// the pass is built rather than only while it was not: what the pass says is
// unbounded and unslotted, so `said` is its own words and there is nothing here
// that counts, grades or categorizes them. A consumer wanting structure over an
// argument would be inventing the schema docs/decision/0078 refuses, and a
// producer emitting one would be deciding open question 3 on its behalf.
//
// Ran is emitted always and never omitted when false, so a consumer can tell
// *no pass has read this thread* from a build that does not answer the
// question. It is the same rule the truth section follows one level down:
// nothing has answered is not an answer.
type argumentPayload struct {
	Ran bool `json:"ran"`
	// Transcripts is what is waiting for it, which separates *nobody has
	// argued this* from *there is nothing here to argue over*.
	Transcripts int    `json:"transcripts"`
	Detail      string `json:"detail"`
	// Passes is how many adversarial passes the thread holds, and Runtime and
	// Model are what answered the last. The model is the runtime's own report
	// of what served the request rather than what an operator asked for, so a
	// session given a name it did not honour says so. See docs/decision/0077.
	Passes  int    `json:"passes"`
	Runtime string `json:"runtime,omitempty"`
	Model   string `json:"model,omitempty"`
	// Said is what the last pass argued, verbatim. Ruling is the one line read
	// out of it, empty when none could be read, and RulingWhy says why, or
	// carries the pass's own reason for an out-of-reach ruling. See
	// docs/decision/0086 and 0096.
	Said      string `json:"said,omitempty"`
	Ruling    string `json:"ruling,omitempty"`
	RulingWhy string `json:"ruling_why,omitempty"`
}

type anchorPayload struct {
	Commit string `json:"commit"`
	Parent string `json:"parent"`
}

// findingPayload is one check, answered.
//
// Outcome is a three-valued string and never a boolean, for the reason a
// verdict is: "unread" is *nobody has answered this*, and a consumer holding a
// boolean would read it as a pass.
type findingPayload struct {
	Check   string   `json:"check"`
	Outcome string   `json:"outcome"`
	Detail  string   `json:"detail"`
	Lines   []string `json:"lines,omitempty"`
}

// gradePayload is what the promise's weakest instrument requires before a
// repair of it may land.
//
// Graded is emitted always and never omitted when false, so a consumer can tell
// *this promise names an instrument kept has not graded* from *this build does
// not answer the question*. Gate is absent exactly when Graded is false, which
// is the same rule the truth section follows: nothing has answered is not an
// answer.
type gradePayload struct {
	Graded   bool     `json:"graded"`
	Gate     string   `json:"gate,omitempty"`
	Weakest  string   `json:"weakest,omitempty"`
	Ungraded []string `json:"ungraded,omitempty"`
}

type worldPayload struct {
	Tree   string `json:"tree"`
	Remote string `json:"remote,omitempty"`
	Ref    string `json:"ref,omitempty"`
	Commit string `json:"commit,omitempty"`
	Thread string `json:"thread,omitempty"`
}

type artifactPayload struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title,omitempty"`
	// Statement is what binds, and it is here because a machine reporting a
	// broken promise should be able to say what was promised.
	Statement string `json:"statement,omitempty"`

	Standing standingPayload `json:"standing"`
	// Desire is what has become of a want, on a desire's record and on no
	// other. It sits beside standing rather than replacing it: the row a table
	// prints for a desire says this, and a distinction a person can see and a
	// machine cannot is a second surface. See docs/decision/0083.
	Desire *desirePayload `json:"desire,omitempty"`
	// Truth is absent when nothing has answered, which is not the same as an
	// answer of "unknown": one means nobody looked, the other means somebody
	// looked and could not tell.
	Truth *truthPayload `json:"truth,omitempty"`
	// Due says whether the answer needs re-deriving, and why.
	Due *duePayload `json:"due,omitempty"`
	// Drift is absent when there is none.
	Drift *driftPayload `json:"drift,omitempty"`
	// Disposition is what the tick did about that drift, and why it did not do
	// more. It is the answer to "why was this left alone".
	Disposition *dispositionPayload `json:"disposition,omitempty"`
	// Landings are what has been landed in this artifact's name, and how each
	// one arrived. It travels on the same surface the section does — `kept
	// why` — because a distinction a person can see and a machine cannot is a
	// second surface. See docs/decision/0015.
	Landings []landingPayload `json:"landings,omitempty"`
	// Escalation is a repair episode handed to a person and not yet answered:
	// nothing has opened another attempt, landed its thread or revised the
	// artifact since. It is derived on read. See docs/decision/0086.
	Escalation *escalationPayload `json:"escalation,omitempty"`
}

// landingPayload is one land act, whole enough to tell which of two guarantees
// it carries.
//
// Regime and arrival are here rather than derived, for the reason the record
// carries them rather than deriving them: the world names a tree and a commit
// and says nothing about where the change was going, and a reader working the
// arrival out from an absent field eventually reads a worktree arrival as a
// land onto a ref. Commit is absent on a worktree arrival because nothing was
// committed, and uncommitted says so in the field a reader already knows.
type landingPayload struct {
	At          time.Time `json:"at"`
	By          string    `json:"by,omitempty"`
	Target      string    `json:"target"`
	Regime      string    `json:"regime"`
	Arrival     string    `json:"arrival"`
	Commit      string    `json:"commit,omitempty"`
	Uncommitted bool      `json:"uncommitted,omitempty"`
	Note        string    `json:"note,omitempty"`
}

type standingPayload struct {
	State  string `json:"state"`
	Reason string `json:"reason,omitempty"`
	// Attention is whether this artifact's standing is something a person can
	// and should act on, which is what `kept status` prints a row for by
	// default. It is here rather than expressed by which records the document
	// carries: the table's default is an emphasis and --json is the only stable
	// format, so a consumer that read the whole register yesterday reads the
	// whole register today and asks this field which rows the operator saw.
	//
	// It is emitted for every record and never omitted when false, so a
	// consumer can tell a build that answers this question from one that does
	// not. See docs/decision/0076.
	Attention bool `json:"attention"`
	// Hash is what the artifact's binding content produces now, and Granted is
	// the content version standing was granted against. They differ exactly
	// when the artifact has been revised since.
	Hash    string        `json:"hash,omitempty"`
	Norm    int           `json:"norm,omitempty"`
	Granted *grantPayload `json:"granted,omitempty"`
}

// desirePayload is one desire's derived state.
//
// State is raw, shaped, committed or retired, and it is never written anywhere:
// it is derived on read from what the desire says, the ratified artifacts whose
// own links say they realize it, and the acts naming it. Unread is why a raw
// desire is not shaped; realized_by is every ratified artifact realizing it, with
// the grant in force for each, in every state.
type desirePayload struct {
	State      string               `json:"state"`
	Becomes    string               `json:"becomes,omitempty"`
	Unread     string               `json:"unread,omitempty"`
	RealizedBy []realizationPayload `json:"realized_by,omitempty"`
}

type realizationPayload struct {
	Artifact string       `json:"artifact"`
	Granted  grantPayload `json:"granted"`
}

type grantPayload struct {
	Hash string    `json:"hash,omitempty"`
	Norm int       `json:"norm,omitempty"`
	By   string    `json:"by,omitempty"`
	At   time.Time `json:"at"`
	Note string    `json:"note,omitempty"`
}

type truthPayload struct {
	// Value is "kept", "not-kept" or "unknown". Three values, never two.
	Value  string `json:"value"`
	Detail string `json:"detail,omitempty"`
	// Since is when this verdict was first reached, not when it was last
	// confirmed.
	Since time.Time    `json:"since"`
	World worldPayload `json:"world"`
	From  []string     `json:"from,omitempty"`
	// Fresh reports whether the answer was reached against the world above. A
	// carried answer is not fresh and is still in force: nothing the artifact
	// looks at has moved since.
	Fresh bool `json:"fresh"`
	// Recorded reports whether this invocation appended it.
	Recorded bool `json:"recorded"`
	// History is every verdict in this artifact's stream, oldest first. Only
	// `why` carries it.
	History []truthEventPayload `json:"history,omitempty"`
}

type truthEventPayload struct {
	Value  string       `json:"value"`
	Detail string       `json:"detail,omitempty"`
	At     time.Time    `json:"at"`
	Hash   string       `json:"hash,omitempty"`
	World  worldPayload `json:"world"`
}

type duePayload struct {
	Due bool `json:"due"`
	// Because is which of the three reasons: nothing has ever answered, the
	// bytes in a scope moved, or the answer aged past max_age.
	Because string `json:"because,omitempty"`
}

type driftPayload struct {
	Kind  string    `json:"kind"`
	Since time.Time `json:"since"`
}

type dispositionPayload struct {
	Outcome   string    `json:"outcome"`
	Condition string    `json:"condition,omitempty"`
	Detail    string    `json:"detail,omitempty"`
	Since     time.Time `json:"since"`
	// Recorded reports whether this invocation appended it. A repetition of the
	// same refusal on the same grounds is not a transition and is not written.
	Recorded bool                      `json:"recorded"`
	History  []dispositionEventPayload `json:"history,omitempty"`
}

type dispositionEventPayload struct {
	Outcome   string       `json:"outcome"`
	Condition string       `json:"condition,omitempty"`
	Detail    string       `json:"detail,omitempty"`
	At        time.Time    `json:"at"`
	World     worldPayload `json:"world"`
}

type problemPayload struct {
	Kind   string `json:"kind"`
	Path   string `json:"path,omitempty"`
	ID     string `json:"id,omitempty"`
	Detail string `json:"detail"`
}

type quarantinedPayload struct {
	Stream string `json:"stream"`
	Line   int    `json:"line"`
	Why    string `json:"why"`
}

// document assembles the payload for one invocation.
//
// It is built from the same values the human rendering prints — nothing here
// re-derives anything — so the two cannot disagree about what was found.
func newDocument(verb string, c *corpus.Corpus, res *tick.Result, code int, history bool) document {
	doc := document{
		Schema:  Schema,
		Verb:    verb,
		Kept:    version,
		At:      time.Now().UTC(),
		Exit:    code,
		Summary: map[string]int{},
	}
	if res.World.Tree != "" {
		w := world(res.World)
		doc.World = &w
	}

	// A question that walked no tree cannot say whether anything is due, and a
	// due: false it never asked would read as an answer.
	looked := res.World.Tree != ""

	doc.Artifacts = make([]artifactPayload, 0, len(res.Subjects))
	for _, s := range res.Subjects {
		doc.Artifacts = append(doc.Artifacts, artifact(s, looked, history))
		doc.Summary[summaryKey(s)]++
	}

	for _, p := range c.Problems {
		doc.Problems = append(doc.Problems, problemPayload{Kind: string(p.Kind), Path: p.Path, ID: p.ID, Detail: p.Detail})
	}
	for _, q := range res.Quarantined {
		doc.Quarantined = append(doc.Quarantined, quarantinedPayload{Stream: q.Stream, Line: q.Line, Why: q.Why})
	}
	return doc
}

func artifact(s tick.Subject, looked, history bool) artifactPayload {
	out := artifactPayload{ID: s.ID, Standing: standing(s.Standing), Desire: desire(s.Standing.Desire)}
	if s.Entry != nil {
		out.Type = string(s.Entry.Artifact.Kind)
		out.Title = s.Entry.Artifact.Title
		out.Statement = s.Entry.Artifact.Statement
	} else {
		kind, _ := corpus.SplitRef(s.ID)
		out.Type = kind
	}

	if looked && s.Predicated() {
		out.Due = &duePayload{Due: s.Due, Because: s.Because}
	}
	if s.Answer.Reached() {
		t := &truthPayload{
			Value:    string(s.Answer.Value),
			Detail:   s.Answer.Detail,
			Since:    s.Answer.Since,
			World:    world(s.Answer.World),
			From:     s.Answer.From,
			Fresh:    s.Answer.Fresh,
			Recorded: s.Answer.Recorded,
		}
		if history {
			for _, r := range s.Truth {
				t.History = append(t.History, truthEventPayload{
					Value: string(r.Value), Detail: r.Detail, At: r.At, Hash: r.Hash, World: world(r.World),
				})
			}
		}
		out.Truth = t
	}
	if s.Drifting {
		out.Drift = &driftPayload{Kind: string(s.Drift.Kind), Since: s.Drift.Since}
	}
	// The disposition travels whenever there is one to read: what the tick is
	// doing about this drift now, or — when `why` asks for the history and
	// there is no drift left — what it did while there was. The absence of a
	// drift beside it is what says which.
	if last, held := dispositionOf(s); held && (s.Drifting || history) {
		d := &dispositionPayload{
			Outcome:   string(last.Outcome),
			Condition: string(last.Condition),
			Detail:    last.Detail,
			Since:     s.DisposedSince(),
			Recorded:  s.Disposed,
		}
		if history {
			for _, r := range s.Dispositions {
				d.History = append(d.History, dispositionEventPayload{
					Outcome: string(r.Outcome), Condition: string(r.Condition),
					Detail: r.Detail, At: r.At, World: world(r.World),
				})
			}
		}
		out.Disposition = d
	}
	if escalation, open := ledger.OpenEscalation(s.Dispositions, s.Landings, s.Standing.Hash.String()); open {
		out.Escalation = &escalationPayload{At: escalation.At, Thread: escalation.Thread, Detail: escalation.Detail}
	}
	if history {
		for _, act := range s.Landings {
			out.Landings = append(out.Landings, landingPayload{
				At: act.At, By: act.By, Target: act.Target,
				Regime: string(act.Regime), Arrival: string(act.Arrival),
				Commit: landedCommit(act), Uncommitted: act.Uncommitted, Note: act.Note,
			})
		}
	}
	return out
}

// landedCommit is what reached the target, and nothing at all when nothing
// did. A worktree arrival's World.Commit is where the operator was standing, so
// reporting it under this name would say a commit landed when none did.
func landedCommit(act ledger.Act) string {
	if act.Arrival == ledger.WorktreeArrival {
		return ""
	}
	return act.World.Commit
}

func standing(st register.Standing) standingPayload {
	out := standingPayload{
		State: string(st.State), Reason: string(st.Reason), Attention: needsAttention(st),
		Hash: st.Hash.String(), Norm: st.Hash.Norm,
	}
	if !st.Granted.At.IsZero() {
		g := grantOf(st.Granted)
		out.Granted = &g
	}
	return out
}

func desire(d *register.Desire) *desirePayload {
	if d == nil {
		return nil
	}
	out := &desirePayload{State: string(d.State), Becomes: d.Becomes, Unread: d.Unread}
	for _, r := range d.RealizedBy {
		out.RealizedBy = append(out.RealizedBy, realizationPayload{
			Artifact: r.Artifact,
			Granted:  grantOf(r.Granted),
		})
	}
	return out
}

func grantOf(g register.Grant) grantPayload {
	return grantPayload{Hash: g.Hash.String(), Norm: g.Hash.Norm, By: g.By, At: g.At, Note: g.Note}
}

func world(w model.World) worldPayload {
	return worldPayload{Tree: w.Tree, Remote: w.Remote, Ref: w.Ref, Commit: w.Commit, Thread: w.Thread}
}

// summaryKey is the one word this artifact counts as. It is the verdict when
// there is one, so a consumer can add up "kept", "not-kept" and "unknown"
// without walking the list.
func summaryKey(s tick.Subject) string {
	if !s.Predicated() && s.Standing.State != register.Absent {
		return "unevaluated"
	}
	return verdictOf(s)
}

// writeDocument prints the payload. It is indented because a person reads it
// too, in the terminal, when something is wrong with what a script saw.
func writeDocument(stdout, stderr io.Writer, doc document) int {
	body, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return exitUsage
	}
	fmt.Fprintf(stdout, "%s\n", body)
	return doc.Exit
}

// dispositionOf is what a `why` prints when the tick has never disposed of an
// artifact: there is no record, and saying so is different from saying nothing.
func dispositionOf(s tick.Subject) (ledger.Disposition, bool) {
	if s.Disposition.Outcome != "" {
		return s.Disposition, true
	}
	return ledger.LastDisposition(s.Dispositions)
}

// reviewDocument is one assessment on the machine surface.
//
// It carries no artifact record. A review answers about a thread, and the
// promise it names is authority the thread was given rather than an artifact
// whose standing and truth this verb went and worked out — reporting one would
// be answering a question nobody asked, from evidence this verb never read.
func reviewDocument(a assess.Assessment, exit int) document {
	payload := &assessmentPayload{
		Thread:  string(a.Thread),
		Promise: a.Promise,
		Grade: gradePayload{
			Graded:   a.Grading.Graded,
			Gate:     string(a.Grading.Gate),
			Weakest:  a.Grading.Weakest,
			Ungraded: a.Grading.Ungraded,
		},
	}
	if a.Entry.Anchored {
		payload.Anchor = &anchorPayload{Commit: a.Entry.Anchor.Commit, Parent: a.Entry.Anchor.Parent}
	}
	if a.Entry.SessionRead {
		payload.Session = sessionDocument(a.Entry.Session)
	}
	payload.Argument = argumentPayload{
		Ran:         a.Argument.Ran,
		Transcripts: a.Argument.Transcripts,
		Detail:      a.Argument.Detail(),
		Passes:      a.Argument.Passes,
		Runtime:     a.Argument.Runtime,
		Model:       a.Argument.Model,
		Said:        a.Argument.Said,
		Ruling:      string(a.Argument.Ruling),
		RulingWhy:   a.Argument.RulingWhy,
	}
	summary := map[string]int{}
	for _, finding := range a.Findings {
		payload.Findings = append(payload.Findings, findingPayload{
			Check:   string(finding.Check),
			Outcome: string(finding.Outcome),
			Detail:  finding.Detail,
			Lines:   finding.Lines,
		})
		summary[string(finding.Outcome)]++
	}
	return document{
		Schema:      Schema,
		Verb:        "review",
		Kept:        version,
		At:          time.Now().UTC(),
		Exit:        exit,
		Artifacts:   []artifactPayload{},
		Summary:     summary,
		Quarantined: quarantined(a.Entry.Quarantined),
		Assessment:  payload,
	}
}

// quarantined is the lines a receipt held that could not be read. They travel
// on the machine surface for the reason they travel on every other one: a
// partial entry read as a whole one is a checklist worked over evidence nobody
// knows is missing.
func quarantined(lines []ledger.Quarantined) []quarantinedPayload {
	var out []quarantinedPayload
	for _, q := range lines {
		out = append(out, quarantinedPayload{Stream: q.Stream, Line: q.Line, Why: q.Why})
	}
	return out
}
