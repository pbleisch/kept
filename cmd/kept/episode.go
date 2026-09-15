package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/pbleisch/kept/internal/assess"
	"github.com/pbleisch/kept/internal/land"
	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/repair"
	"github.com/pbleisch/kept/internal/thread"
)

// phaseLine is one finished phase of a repair episode, as `kept repair` prints
// it while the run is still going.
//
// One line each, and each says what the phase established rather than that it
// happened: the value the promise re-derived to, the commit that anchored the
// work, what the pass ruled, and the action the attempt's reading selected and
// why. A person watching a run that may last hours can then tell attempt two's
// session from attempt one's pass without reading a receipt. See
// docs/decision/0086.
func phaseLine(event repair.Event) string {
	prefix := fmt.Sprintf("attempt %d · ", event.Attempt)
	switch event.Kind {
	case repair.AttemptStarted:
		if event.Rework {
			return prefix + fmt.Sprintf("thread %s · box cut at %s, where the earlier attempts left the work", event.Thread, shortCommit(event.Commit))
		}
		return prefix + fmt.Sprintf("thread %s · box cut at %s", event.Thread, shortCommit(event.Commit))
	case repair.SessionEnded:
		return prefix + "session " + sessionEnded(event.Session)
	case repair.Rederived:
		line := prefix + fmt.Sprintf("re-derived from the box: %s → %s", valueOrUnknown(event.Before), valueOrUnknown(event.After))
		if event.After != model.Kept && event.Detail != "" {
			line += " · " + event.Detail
		}
		return line
	case repair.Anchored:
		if event.Commit == "" {
			return prefix + event.Detail
		}
		return prefix + "anchored at " + shortCommit(event.Commit)
	case repair.PassStarted:
		return prefix + "adversarial pass started, in a box it cannot write"
	case repair.PassEnded:
		return prefix + passEnded(event)
	case repair.Assessed:
		return prefix + checksLine(event.Fired, event.Unread)
	case repair.Decided:
		line := prefix + actionWords(event.Action) + ": " + strings.Join(event.Why, "; ")
		if event.Action == repair.ActionRework && event.Window != "" {
			line += " · " + event.Window
		}
		return line
	}
	return prefix + string(event.Kind)
}

func sessionEnded(d thread.Diagnosis) string {
	var line string
	switch {
	case d.Started.IsZero():
		return "never started"
	case d.TimedOut != "" && d.Killed:
		line = "was killed at its " + d.TimedOut + " deadline"
	case d.Exit == nil:
		line = "ended with no exit recorded"
	case d.Exit.Signal != "":
		line = "was reaped on " + d.Exit.Signal
	default:
		line = "exited " + exitWords(*d.Exit)
	}
	if !d.Ended.IsZero() {
		line += " after " + d.Ended.Sub(d.Started).Round(time.Second).String()
	}
	switch {
	case d.ClosingEvent != "" && d.Closed:
		line += ", and the runtime's " + d.ClosingEvent + " arrived"
	case d.ClosingEvent != "":
		line += ", and no " + d.ClosingEvent + " arrived"
	}
	return line
}

func passEnded(event repair.Event) string {
	switch event.Pass {
	case repair.Argued:
		if event.Ruling.Known() {
			return "adversarial pass ruled " + string(event.Ruling)
		}
		return "adversarial pass argued and made no ruling kept can read: " + event.Detail
	case repair.Unconfigured:
		return "no adversarial pass: no assessor is configured on this machine"
	case repair.Unanchored:
		return "no adversarial pass: nothing was anchored for one to argue about"
	case "":
		return event.Detail
	}
	return fmt.Sprintf("adversarial pass %s: %s", event.Pass, event.Detail)
}

func checksLine(fired, unread []assess.Finding) string {
	if len(fired) == 0 && len(unread) == 0 {
		return "checks about the act: all clear"
	}
	var parts []string
	if names := checkNames(fired); names != "" {
		parts = append(parts, "fired "+names)
	}
	if names := checkNames(unread); names != "" {
		parts = append(parts, "could not work "+names)
	}
	return "checks about the act: " + strings.Join(parts, "; ")
}

func checkNames(findings []assess.Finding) string {
	var names []string
	for _, finding := range findings {
		names = append(names, string(finding.Check))
	}
	return strings.Join(names, ", ")
}

func actionWords(action repair.Action) string {
	switch action {
	case repair.ActionReady:
		return "ready to land"
	case repair.ActionRework:
		return "rework"
	case repair.ActionEscalate:
		return "escalate"
	}
	return string(action)
}

func valueOrUnknown(value model.Value) string {
	if value == "" {
		return string(model.Unknown)
	}
	return string(value)
}

// writeEpisode is how a run ended and what the person is now expected to do.
//
// It always ends with what to do next, including when the answer is nothing,
// because the report above it is evidence and the question a person has when
// the run returns is what their move is. See docs/decision/0086 and
// desire:repair-and-tick-output-tell-the-operator-what-to-do-next.
func writeEpisode(w io.Writer, ep *repair.Episode, target string) {
	if ep.End == "" {
		return
	}
	last := ep.Last()
	anchored := anchorOf(ep)

	fmt.Fprintln(w)
	switch ep.End {
	case repair.Ready:
		fmt.Fprintln(w, "ready to land")
	case repair.Escalated:
		fmt.Fprintln(w, "escalated to a person")
	case repair.Stopped:
		fmt.Fprintln(w, "stopped")
	}
	if ep.Thread != "" {
		line := fmt.Sprintf("  thread %s · %s in this run, %s", ep.Thread, count(len(ep.Attempts), "attempt"), ep.Bounds)
		fmt.Fprintln(w, line)
	}
	if anchored.Commit != "" && ep.End != repair.Stopped {
		fmt.Fprintf(w, "  anchored at %s: the episode's whole change against %s, which is what a land takes\n", shortCommit(anchored.Commit), shortCommit(anchored.Parent))
	}
	if ep.Bounds.Unapplied != "" {
		fmt.Fprintf(w, "  %s\n", ep.Bounds.Unapplied)
	}
	for _, why := range ep.Why {
		fmt.Fprintf(w, "  %s\n", why)
	}
	writeHandoff(w, ep)

	fmt.Fprintln(w, "\nnext")
	for _, line := range nextSteps(ep, last, anchored, target) {
		fmt.Fprintf(w, "  %s\n", line)
	}
}

// nextSteps is what the person does now, as commands they can paste.
func nextSteps(ep *repair.Episode, last *repair.Result, anchored thread.Anchor, target string) []string {
	onto := "kept land " + string(ep.Thread) + " --onto " + target
	rework := "kept repair --rework " + string(ep.Thread)
	switch ep.End {
	case repair.Ready:
		return []string{
			"kept review " + string(ep.Thread) + "    read every attempt, the checklist and what the pass argued",
			onto + "    land it; nothing lands until you do",
		}
	case repair.Escalated:
		steps := []string{"kept review " + string(ep.Thread) + "    read every attempt, the checklist and what the pass argued", "then one of:"}
		steps = append(steps, "  "+rework+"    another run, carrying everything so far into it")
		if repeated(ep.Why) {
			steps = append(steps, "  revise "+ep.Promise+" so the repeating finding is a clause, kept ratify it, then kept repair "+ep.Promise)
		} else {
			steps = append(steps, "  revise "+ep.Promise+" and kept ratify it, when what the episode keeps finding is a predicate nobody has written")
		}
		if anchored.Commit != "" {
			steps = append(steps, "  "+onto+"    when the case against the work does not hold")
		}
		return steps
	}
	// Stopped. The attempt's own report above says why, and this says what
	// moves it.
	// A thread exists once a session has run in this episode or a person named
	// one to rework, and from then on the work continues there rather than in a
	// new episode.
	again := "kept repair " + ep.Promise
	if ep.Thread != "" {
		again = rework
	}
	if last == nil {
		return []string{"nothing can be done in this run; " + again + " once the usage window has room"}
	}
	switch last.Status {
	case repair.Holds:
		return []string{"nothing to repair at that commit; kept re-derives the promise in your workspace"}
	case repair.Unmeasured:
		return []string{"kept why " + ep.Promise + "    says what could not be measured"}
	case repair.Prepared:
		return []string{strings.TrimSpace(again) + "    dispatches it"}
	case repair.Refused:
		return []string{"fix what refused, then " + again}
	}
	return []string{again}
}

// repeated reports whether an escalation was a finding repeating, which is the
// one whose answer is a predicate rather than another run.
func repeated(why []string) bool {
	for _, reason := range why {
		if strings.Contains(reason, "fired again") {
			return true
		}
	}
	return false
}

// anchorOf is the anchor a land of this episode would take.
func anchorOf(ep *repair.Episode) thread.Anchor {
	for i := len(ep.Readings) - 1; i >= 0; i-- {
		if entry := ep.Readings[i].Assessment.Entry; entry.Anchored {
			return entry.Anchor
		}
	}
	for i := len(ep.Attempts) - 1; i >= 0; i-- {
		if anchor := ep.Attempts[i].Anchor; anchor.Commit != "" {
			return anchor
		}
	}
	return thread.Anchor{}
}

// landTarget is where `kept land` would put this work with no --onto, spelled
// so it can be pasted, and a placeholder where it cannot be worked out: a
// detached HEAD names no branch, and guessing one would print a command that
// lands somewhere nobody chose.
func landTarget(workspace string, deadline time.Duration) string {
	target, err := land.ResolveTarget(context.Background(), workspace, deadline)
	if err != nil {
		return "<branch>"
	}
	return target.String()
}
