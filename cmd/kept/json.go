package main

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/register"
	"github.com/pbleisch/kept/internal/tick"
)

// Schema is the version on every structured payload.
//
// A consumer that cannot tell which shape it is holding will guess, and guess
// wrong exactly once — so the version is on the document from the first
// release, exactly as it is on every ledger record. It is one envelope for
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
	// Hash is what the artifact's binding content produces now, and Granted is
	// the content version standing was granted against. They differ exactly
	// when the artifact has been revised since.
	Hash    string        `json:"hash,omitempty"`
	Norm    int           `json:"norm,omitempty"`
	Granted *grantPayload `json:"granted,omitempty"`
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
	out := artifactPayload{ID: s.ID, Standing: standing(s.Standing)}
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
	out := standingPayload{State: string(st.State), Reason: string(st.Reason), Hash: st.Hash.String(), Norm: st.Hash.Norm}
	if !st.Granted.At.IsZero() {
		out.Granted = &grantPayload{
			Hash: st.Granted.Hash.String(), Norm: st.Granted.Hash.Norm,
			By: st.Granted.By, At: st.Granted.At, Note: st.Granted.Note,
		}
	}
	return out
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
