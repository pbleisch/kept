package thread

import "github.com/pbleisch/kept/internal/ledger"

// Attempt is one attempt of a repair episode, grouped out of its thread's
// receipt: the measurement taken in its box before the session, the repair
// session that carried its packet, and every adversarial pass over the work it
// left.
//
// It is a derivation over the record rather than an ordinal written into it, for
// docs/decision/0079's reason: a number on every body type is one answer in ten
// places. [Sessions] already groups a receipt by its input records, and this
// adds the one thing that grouping cannot see — the measurement a later
// attempt's box takes is filed *before* its packet, so by input records alone it
// belongs to the pass over the attempt before. See docs/decision/0086.
type Attempt struct {
	// Before is the measurements filed in this attempt's box before its session
	// started, in receipt order.
	Before []Measurement
	// Repair is the session that carried this attempt's packet.
	Repair Session
	// Passes is every adversarial pass over this attempt's work, oldest first.
	Passes []Session
	// Start and End bound this attempt's entries in the receipt it was grouped
	// from: entries[Start:End] is this attempt and nothing else, and
	// entries[:End] is the episode as it stood when this attempt was done.
	Start, End int
}

// Anchor is the commit this attempt's own session left, and whether it left
// one. An attempt that changed nothing anchors nothing, and the thread's latest
// anchor is then an earlier attempt's.
func (a Attempt) Anchor() (Anchor, bool) {
	return LatestAnchor(a.Repair.Entries)
}

// After is the measurements filed after this attempt's session, in receipt
// order.
func (a Attempt) After() []Measurement {
	var out []Measurement
	for _, measured := range Measurements(a.Repair.Entries) {
		if measured.Side == After {
			out = append(out, measured)
		}
	}
	return out
}

// Attempts groups a receipt into the attempts of its episode, oldest first.
//
// A measurement on the before side opens the next attempt rather than
// extending the current session, and a packet record closes the opening: the
// first attempt's before measurement precedes its packet, and so does every
// rework's, filed after the pass over the attempt before it. A brief record
// starts a pass inside the current attempt. Entries before any packet that are
// not before-side measurements belong to no attempt and are dropped, which is
// what [Sessions] does with them too.
//
// The one exception is a before-side measurement filed after its own packet and
// before its session settled — before the process was reaped, anything was
// re-derived or anchored, or a pass began. That is still the current attempt's,
// because nothing about the attempt is over yet; only a measurement filed after
// the attempt settled can be the next box's.
//
// A receipt with no packet at all has no attempts.
func Attempts(entries []Entry) []Attempt {
	var (
		out     []Attempt
		pending []Measurement
		start   = -1
		current *Session
		settled bool
	)
	closeLast := func(at int) {
		if len(out) > 0 && out[len(out)-1].End == 0 {
			out[len(out)-1].End = at
		}
	}
	for i, entry := range entries {
		switch {
		case entry.Kind == ledger.ThreadMeasurement && entry.Measurement != nil && entry.Measurement.Side == Before && len(out) > 0 && !settled && start < 0:
			last := &out[len(out)-1]
			last.Before = append(last.Before, *entry.Measurement)
		case entry.Kind == ledger.ThreadMeasurement && entry.Measurement != nil && entry.Measurement.Side == Before:
			if start < 0 {
				start = i
			}
			pending = append(pending, *entry.Measurement)
			current = nil
		case entry.Kind == ledger.ThreadPacket && entry.Packet != nil:
			if start < 0 {
				start = i
			}
			closeLast(start)
			out = append(out, Attempt{
				Before: pending,
				Repair: Session{Kind: entry.Kind, Input: *entry.Packet},
				Start:  start,
			})
			current = &out[len(out)-1].Repair
			pending, start, settled = nil, -1, false
		case entry.Kind == ledger.ThreadBrief && entry.Brief != nil:
			if len(out) == 0 {
				continue
			}
			last := &out[len(out)-1]
			last.Passes = append(last.Passes, Session{Kind: entry.Kind, Input: *entry.Brief})
			current = &last.Passes[len(last.Passes)-1]
			settled = true
		case current != nil:
			current.Entries = append(current.Entries, entry)
			if settles(entry) {
				settled = true
			}
		}
	}
	if len(out) > 0 && out[len(out)-1].End == 0 {
		end := len(entries)
		if start >= 0 {
			// Before measurements with no packet after them are a box that was
			// measured and never dispatched into — a rework that stopped before
			// its session — and they are not the last attempt's.
			end = start
		}
		out[len(out)-1].End = end
	}
	return out
}

// settles reports whether an entry says its attempt's session is over: the
// process was reaped, the promise was measured again, or the work was anchored.
func settles(entry Entry) bool {
	switch {
	case entry.Lifecycle != nil && entry.Lifecycle.Event == Exited:
		return true
	case entry.Measurement != nil && entry.Measurement.Side == After:
		return true
	case entry.Anchor != nil:
		return true
	}
	return false
}
