package repair

import (
	"github.com/pbleisch/kept/internal/assess"
	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/thread"
)

// EventKind is which phase of a repair episode has finished.
type EventKind string

const (
	// AttemptStarted: a box was cut for an attempt. Commit is where, and Rework
	// says it continues earlier work.
	AttemptStarted EventKind = "attempt-started"
	// SessionEnded: the attempt's session is over. Session is how far it got.
	SessionEnded EventKind = "session-ended"
	// Rederived: the promise was measured again in the tree the session left.
	Rederived EventKind = "re-derived"
	// Anchored: the session's work was committed and its ref written, and
	// Commit is the anchor. An empty Commit is an attempt that anchored nothing,
	// and Detail says why.
	Anchored EventKind = "anchored"
	// Assessed: the derived checks were worked over the episode's act. Fired and
	// Unread are the checks about the act that fired and that could not be
	// worked.
	Assessed EventKind = "assessed"
	// PassStarted: an adversarial pass is about to be dispatched.
	PassStarted EventKind = "pass-started"
	// PassEnded: the pass is over. Pass is how far it got and Ruling what it
	// ruled, with Detail saying why there is none.
	PassEnded EventKind = "pass-ended"
	// Decided: the attempt's reading selected an action, and Why says why.
	// Window is what the usage-window ceiling read, for a rework.
	Decided EventKind = "decided"
)

// Event is one phase of a repair episode that has finished, and what it
// established.
//
// It is presentation's input and nothing reads it back. `internal/repair`
// emits one as each phase finishes and `kept repair` prints a line for it, so a
// person watching a run that may last hours can tell the second attempt's
// session from the first attempt's pass. Nothing here reaches a record: every
// fact an event carries is already in the receipt, the observation stream or a
// disposition. See docs/decision/0086.
type Event struct {
	Kind    EventKind
	Attempt int
	Thread  thread.ID

	Commit  string
	Rework  bool
	Session thread.Diagnosis

	Before, After model.Value

	Fired, Unread []assess.Finding

	Pass   ArgueStatus
	Ruling thread.Ruling

	Action Action
	Why    []string
	Window string

	Detail string
}

// emit hands an event to whoever is watching, and to nobody when nobody is.
func (req Request) emit(event Event) {
	if req.Events != nil {
		req.Events(event)
	}
}
