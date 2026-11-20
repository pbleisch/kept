package thread

import (
	"strconv"
	"strings"
	"time"

	"github.com/pbleisch/kept/internal/ledger"
)

// Diagnosis is how far one session got, derived from its receipt.
//
// It is worked out on read and stored nowhere, for the reason every other
// reading of a receipt is: the entries are the record, and a summary written
// beside them is a second place the answer lives. What it answers is the
// question a failed repair leaves a person with — did the session start, how far
// did it get, did it finish, what did it last say, and was the binary it ran the
// one it was admitted as. See docs/decision/0085.
type Diagnosis struct {
	// Runtime is the adapter the session's own entries name, and empty when it
	// wrote none.
	Runtime Runtime

	// Started and Ended are the parent's lifecycle, in receipt time: when kept
	// recorded that the process started and was reaped. They are not when the
	// runtime did anything — a receipt can be minutes behind its runtime.
	Started, Ended time.Time
	// Exit is the reaped exit, nil when the receipt holds none: a session whose
	// run was interrupted before the parent could record one.
	Exit *Exit
	// TimedOut is the deadline in force when the parent killed the session, and
	// empty when no deadline elapsed. Killed says the kill was performed.
	TimedOut string
	Killed   bool

	// Natives is how many native events arrived, and LastNative and LastNativeAt
	// are the type and receipt time of the final one.
	Natives      int
	LastNative   string
	LastNativeAt time.Time
	// Turns is the last turn count the runtime reported, and Model the model it
	// said answered.
	Turns uint64
	Model string
	// Closed says the runtime's own closing event arrived — `result` for Claude
	// — which is what a session that finished and whose output all arrived
	// ends with. ClosingEvent names what was looked for, and is empty for a
	// runtime this build knows no closing event for.
	Closed       bool
	ClosingEvent string
	// Undecodable counts output that was not an event, and Unterminated says
	// the output ended inside an item.
	Undecodable  int
	Unterminated bool

	// Launch and Exited are the executable as the parent observed it before the
	// launch and after the reap, each with whether it was recorded.
	Launch, AtExit                 Executable
	LaunchObserved, ExitedObserved bool

	// Stderr is the retained end of the runtime's error stream, nil when there
	// was none to keep.
	Stderr *Stderr

	// Anchored says the session's work was committed and recorded, and
	// AfterMeasured that the promise was measured in the box after it.
	Anchored, AfterMeasured bool

	// Exhausted is the runtime saying the account had no capacity left when
	// the session ended, and nil when it said no such thing. See
	// [OutOfCapacity].
	Exhausted *Exhaustion
}

// Replaced reports whether the executable at the session's path was a
// different file after the reap than before the launch. False when either
// observation is missing: nobody looked is not *it stayed the same*.
func (d Diagnosis) Replaced() bool {
	return d.LaunchObserved && d.ExitedObserved && !d.Launch.SameFile(d.AtExit)
}

// closingEvent is the native type a runtime ends a finished session with.
func closingEvent(runtime Runtime) string {
	switch runtime {
	case Claude:
		return "result"
	case Pi:
		return "agent_settled"
	case Codex:
		return "turn/completed"
	}
	return ""
}

// RepairSession is the last session in a receipt that carried a packet — the
// repair, rather than an adversarial pass over it — and whether there is one.
func RepairSession(entries []Entry) (Session, bool) {
	sessions := Sessions(entries)
	for i := len(sessions) - 1; i >= 0; i-- {
		if sessions[i].Kind == ledger.ThreadPacket {
			return sessions[i], true
		}
	}
	return Session{}, false
}

// TerminalError is the runtime's own report that a session ended in a failure it
// can name, read from that session's closing event, and empty when it reported
// none or when this build reads no such report for the runtime.
//
// It is what a reader needs in order to tell a session that had nothing to say
// from one that was never given a chance to say it. Both leave a receipt with no
// transcript, and only the runtime knows which happened — so this is read from
// the runtime's own words and is never derived from the absence.
//
// It explains an outcome and does not decide one. The one failure that does
// decide one is a session the account had no capacity left for, and that is
// [OutOfCapacity]'s question, asked of the same closing event.
func TerminalError(session Session) string {
	for i := len(session.Entries) - 1; i >= 0; i-- {
		native := session.Entries[i].Native
		if native == nil || native.Type != closingEvent(native.Runtime) {
			continue
		}
		if native.Runtime == Claude {
			return claudeTerminalError(native.Payload)
		}
		return ""
	}
	return ""
}

// Exhaustion is a session that ended because the account it ran on had no
// capacity left, in the runtime's own words.
type Exhaustion struct {
	// Said is what the runtime said when it stopped, verbatim — for Claude, its
	// closing result's text, such as "You've hit your session limit · resets
	// 12:40am (America/Los_Angeles)".
	Said string
	// Resets is the earliest moment a usage window the session last reported
	// at or over its limit resets, and zero when the session reported no
	// window at its limit. It is the runtime's reading, never kept's guess.
	Resets time.Time
}

// OutOfCapacity reports whether a session ended because the account it ran on
// had no capacity left, read from the runtime's own closing event.
//
// It is read rather than inferred, for [TerminalError]'s reason: a session that
// ran out and a session that finished early leave receipts that differ only in
// what the runtime says about itself. And it is narrower than TerminalError on
// purpose. A rate limit is the one failure whose answer is the same every time
// — the account says not yet, and a session started before it resets meets the
// same answer — so it is the one failure that decides something: what such a
// session left is not a result. See docs/decision/0090.
//
// A runtime this build reads no such report for answers false. Claude is the
// only one: its closing `result` carries `is_error` and the HTTP status the
// request was refused with, and 429 is the status an exhausted account is
// refused with.
func OutOfCapacity(session Session) (Exhaustion, bool) {
	for i := len(session.Entries) - 1; i >= 0; i-- {
		native := session.Entries[i].Native
		if native == nil || native.Type != closingEvent(native.Runtime) {
			continue
		}
		if native.Runtime != Claude {
			return Exhaustion{}, false
		}
		said, out := claudeOutOfCapacity(native.Payload)
		if !out {
			return Exhaustion{}, false
		}
		return Exhaustion{Said: said, Resets: exhaustedResets(session.Entries)}, true
	}
	return Exhaustion{}, false
}

// exhaustedResets is the earliest reset of any window the session's last
// window reading put at or over its limit, and zero when none was.
//
// A utilization is the runtime's decimal held as text and may read above one in
// overage, so it is compared as a number; one that will not parse says nothing
// about when capacity returns and is passed over.
func exhaustedResets(entries []Entry) time.Time {
	reading, reported := LatestWindow(entries)
	if !reported {
		return time.Time{}
	}
	var earliest time.Time
	for _, window := range reading.Windows {
		utilization, err := strconv.ParseFloat(strings.TrimSpace(window.Utilization), 64)
		if err != nil || utilization < 1 || window.ResetsAt.IsZero() {
			continue
		}
		if earliest.IsZero() || window.ResetsAt.Before(earliest) {
			earliest = window.ResetsAt
		}
	}
	return earliest
}

// Diagnose reads one session's entries.
func Diagnose(session Session) Diagnosis {
	var d Diagnosis
	for _, entry := range session.Entries {
		switch {
		case entry.Lifecycle != nil:
			switch entry.Lifecycle.Event {
			case Started:
				d.Started = entry.At
			case TimedOut:
				d.TimedOut = entry.Lifecycle.Timeout
			case Killed:
				d.Killed = true
			case Exited:
				d.Ended, d.Exit = entry.At, entry.Lifecycle.Exit
			}
		case entry.Native != nil:
			d.Runtime = entry.Native.Runtime
			d.Natives++
			d.LastNative, d.LastNativeAt = entry.Native.Type, entry.At
			if entry.Native.Type == closingEvent(entry.Native.Runtime) {
				d.Closed = true
			}
		case entry.Undecodable != nil:
			d.Runtime = entry.Undecodable.Runtime
			d.Undecodable++
			d.Unterminated = d.Unterminated || entry.Undecodable.Unterminated
		case entry.Turn != nil:
			d.Turns = entry.Turn.Count
		case entry.Model != nil:
			d.Model = entry.Model.Model
		case entry.Executable != nil:
			switch entry.Executable.Moment {
			case AtLaunch:
				d.Launch, d.LaunchObserved = *entry.Executable, true
			case AtExit:
				d.AtExit, d.ExitedObserved = *entry.Executable, true
			}
		case entry.Stderr != nil:
			stderr := *entry.Stderr
			d.Stderr = &stderr
		case entry.Anchor != nil:
			d.Anchored = true
		case entry.Measurement != nil && entry.Measurement.Side == After:
			d.AfterMeasured = true
		}
	}
	d.ClosingEvent = closingEvent(d.Runtime)
	if exhausted, out := OutOfCapacity(session); out {
		d.Exhausted = &exhausted
	}
	return d
}
