package assess

import (
	"context"
	"fmt"

	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/thread"
)

// AttemptReading is one attempt of a repair episode, assessed as the episode
// stood when that attempt was done.
//
// Everything in it is derived on read from the thread's receipt, like every
// other finding here, and nothing about it is recorded. The derived findings
// are over the episode's act up to this attempt — the first box's measurement,
// this attempt's re-derivation, and the anchor in force after it — because
// that is the change a land of it would take. The argument is the pass over
// this attempt's work and no other. See docs/decision/0086.
type AttemptReading struct {
	// Number is the attempt's place in its episode, from 1, counted out of the
	// receipt's own order.
	Number int
	// Attempt is the grouping it was read from.
	Attempt thread.Attempt

	// Cut is the commit this attempt's box was cut at, where the record says:
	// the episode's base for the first attempt, and the anchor in force before
	// it for a later one. Empty is a record that does not say.
	Cut string
	// Anchor is the commit this attempt's own session left, and Anchored
	// reports whether it left one.
	Anchor   thread.Anchor
	Anchored bool

	// Before and After are what the promise read in this attempt's own box on
	// either side of its session, applying the packet's predicate to the
	// measurements the receipt names. Empty is not reached: nothing recorded,
	// or nothing the predicate could decide.
	Before, After model.Value

	// Assessment is the thread as it stood when this attempt was done.
	Assessment Assessment
}

// Attempts assesses every attempt of one thread's episode, oldest first.
//
// One receipt read, and one diff per attempt that anchored. A thread with no
// packet has no attempts and answers none, which is not an error: it is a
// thread nothing was dispatched into.
func Attempts(ctx context.Context, req Request) ([]AttemptReading, error) {
	if req.Store == nil {
		return nil, fmt.Errorf("an assessment has no ledger to read")
	}
	entries, quarantined, err := receiptOf(req.Store, req.Thread)
	if err != nil {
		return nil, err
	}
	var (
		out  []AttemptReading
		base string
	)
	if anchor, found := thread.LatestAnchor(entries); found {
		base = anchor.Parent
	}
	for i, attempt := range thread.Attempts(entries) {
		prefix := entries[:attempt.End]
		entry := entryOf(ctx, req.Store, req.Repository, req.Thread, prefix, quarantined, req.Deadline)
		s := subject{entry: entry, corpus: req.Corpus}
		reading := AttemptReading{
			Number:  i + 1,
			Attempt: attempt,
			Cut:     base,
			Assessment: Assessment{
				Thread:   req.Thread,
				Promise:  entry.Packet.Promise.ID,
				Entry:    entry,
				Findings: s.findings(),
				Argument: argument(entry),
				Grading:  Grade(entry.Packet.Promise.Gather),
			},
		}
		if earlier, found := thread.LatestAnchor(entries[:attempt.Start]); found {
			reading.Cut = earlier.Commit
		}
		reading.Anchor, reading.Anchored = attempt.Anchor()

		before, _ := measurements(req.Store, attempt.Before)
		if len(before) == 0 && i == 0 && entry.Delivered {
			// The first attempt's packet carries the measurement its box took,
			// for a receipt written before measurements were named in it.
			before = entry.Packet.Observations
		}
		_, after := measurements(req.Store, attempt.After())
		if value, reached := s.reached(before); reached {
			reading.Before = value
		}
		if value, reached := s.reached(after); reached {
			reading.After = value
		}
		out = append(out, reading)
	}
	return out, nil
}

// FiredOfTheAct is the checks about the act that fired, in checklist order.
func (a Assessment) FiredOfTheAct() []Finding {
	var out []Finding
	for _, finding := range a.Fired() {
		if finding.Check.OfTheAct() {
			out = append(out, finding)
		}
	}
	return out
}

// UnreadOfTheAct is the checks about the act nothing could work, in checklist
// order.
func (a Assessment) UnreadOfTheAct() []Finding {
	var out []Finding
	for _, finding := range a.only(Unread) {
		if finding.Check.OfTheAct() {
			out = append(out, finding)
		}
	}
	return out
}
