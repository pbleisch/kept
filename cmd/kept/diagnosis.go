package main

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/repair"
	"github.com/pbleisch/kept/internal/thread"
)

// repairDiagnosis is how far a thread's repair session got, read from its
// receipt: the last session that carried a packet, or the whole receipt when
// none did.
func repairDiagnosis(store ledger.Store, id thread.ID) (thread.Diagnosis, error) {
	entries, _, err := thread.ReadReceipt(store, id)
	if err != nil {
		return thread.Diagnosis{}, err
	}
	session, found := thread.RepairSession(entries)
	if !found {
		session = thread.Session{Entries: entries}
	}
	return thread.Diagnose(session), nil
}

// addSessionRows puts how far a session got into a table, one question per row.
// A failure report and a review print the same rows, so a person reading either
// reads one account of the session. See docs/decision/0085.
func addSessionRows(t *table, store ledger.Store, d thread.Diagnosis, runtimeAfter string) {
	t.add("session", sessionLine(d))
	t.add("reached", reachedLine(d))
	t.add("closed", closedLine(d))
	if line := runtimeLine(d, runtimeAfter); line != "" {
		t.add("runtime", line)
	}
	t.add("stderr", stderrLine(store, d))
}

func sessionLine(d thread.Diagnosis) string {
	if d.Started.IsZero() {
		return "never started"
	}
	line := "started " + d.Started.UTC().Format(stamp)
	switch {
	case d.TimedOut != "" && d.Killed:
		line += ", killed at its " + d.TimedOut + " deadline"
	case d.TimedOut != "":
		line += ", past its " + d.TimedOut + " deadline"
	}
	if d.Exit == nil {
		return line + "; no exit was recorded"
	}
	ended := "exited " + exitWords(*d.Exit)
	if d.Exit.Signal != "" {
		ended = "reaped on " + d.Exit.Signal
	}
	return fmt.Sprintf("%s, %s at %s (%s)", line, ended, d.Ended.UTC().Format(stamp), d.Ended.Sub(d.Started).Round(time.Second))
}

func exitWords(exit thread.Exit) string {
	if exit.Code == nil {
		return "with no code"
	}
	return fmt.Sprintf("%d", *exit.Code)
}

func reachedLine(d thread.Diagnosis) string {
	if d.Natives == 0 {
		return "no event arrived from the runtime"
	}
	line := fmt.Sprintf("%s and %s", count(int(d.Turns), "turn"), count(d.Natives, "event"))
	return fmt.Sprintf("%s; the last was %s, recorded at %s", line, d.LastNative, d.LastNativeAt.UTC().Format(stamp))
}

func closedLine(d thread.Diagnosis) string {
	var line string
	switch {
	case d.ClosingEvent == "":
		line = "no closing event is known for this runtime"
	case d.Closed:
		line = "the runtime's " + d.ClosingEvent + " arrived"
	default:
		line = "no " + d.ClosingEvent + " arrived"
	}
	decodable := d.Undecodable
	if d.Unterminated {
		line += ", and the output ended partway through a line"
		decodable--
	}
	if decodable > 0 {
		line += fmt.Sprintf(", and %s of output were not events", count(decodable, "line"))
	}
	return line
}

func runtimeLine(d thread.Diagnosis, runtimeAfter string) string {
	if !d.LaunchObserved {
		return ""
	}
	name := string(d.Runtime)
	if name == "" {
		name = "the runtime"
	}
	line := fmt.Sprintf("%s %s from %s", name, d.Launch.Version, d.Launch.Path)
	switch {
	case !d.ExitedObserved:
		return line + "; nothing looked at it again after the session"
	case d.AtExit.Missing:
		return line + "; nothing was at that path after the session"
	case d.Replaced() && runtimeAfter != "":
		return line + "; replaced during the session, and it reports " + runtimeAfter + " now"
	case d.Replaced():
		return line + "; a different file was at that path after the session"
	}
	return line + "; the same file after the session"
}

// stderrLine is the last thing the runtime wrote on its error stream.
//
// A receipt written before stderr was retained holds no entry for it, and that
// is not a runtime that wrote nothing — so the two are told apart by whether
// the receipt recorded the executable, which the same build started doing.
func stderrLine(store ledger.Store, d thread.Diagnosis) string {
	if d.Stderr == nil {
		if !d.LaunchObserved {
			return "not retained by the build that ran this session"
		}
		return "nothing was written"
	}
	src, err := ledger.ReadArtifact(store, d.Stderr.Artifact)
	if err != nil {
		return "retained, and it cannot be read: " + err.Error()
	}
	last := lastLine(src)
	line := fmt.Sprintf("%q is the last line", last)
	if d.Stderr.Redaction.Occurred {
		line += ", with the credential removed"
	}
	if d.Stderr.Omitted > 0 {
		line += fmt.Sprintf("; %d bytes before the retained end were not kept", d.Stderr.Omitted)
	}
	return line
}

// lastLine is the final non-empty line, bounded so one runaway line cannot
// fill a report. The bound is on what is shown, and the receipt holds the rest.
func lastLine(src []byte) string {
	lines := bytes.Split(bytes.TrimRight(src, "\r\n\t "), []byte("\n"))
	last := strings.TrimSpace(string(lines[len(lines)-1]))
	const shown = 240
	if len(last) > shown {
		return last[:shown] + "…"
	}
	return last
}

// writeRepairFailure is a repair that failed after it opened a thread,
// explained from its receipt: what failed, how far the session got, where the
// box is, and what to run next. See docs/decision/0085.
func writeRepairFailure(w io.Writer, store ledger.Store, f *repair.Failure) {
	fmt.Fprintf(w, "kept: %s: %s\n\n", f.Promise, phaseWords(f.Phase))
	t := table{indent: "  "}
	t.add("thread", string(f.Thread))
	d, err := repairDiagnosis(store, f.Thread)
	if err != nil {
		t.add("receipt", "could not be read: "+err.Error())
	} else {
		addSessionRows(&t, store, d, f.RuntimeAfter)
	}
	switch {
	case f.Box != "":
		t.add("box", "kept at "+f.Box)
	case f.Phase == repair.PhaseLaunch:
		t.add("box", "discarded, because no session ran in it")
	}
	t.add("error", errorWords(f))
	t.write(w)

	fmt.Fprintln(w)
	if f.Box != "" {
		fmt.Fprintf(w, "git -C %s diff reads what the session left, and git worktree remove --force %s discards it\n", f.Box, f.Box)
	}
	fmt.Fprintf(w, "kept review %s reads what the thread's receipt holds\n", f.Thread)
	fmt.Fprintf(w, "kept why %s says what stands for the promise\n", f.Promise)
	fmt.Fprintf(w, "kept repair %s tries again\n", f.Promise)
}

func phaseWords(phase repair.Phase) string {
	switch phase {
	case repair.PhaseLaunch:
		return "the repair failed before its session started"
	case repair.PhaseSession:
		return "the session ran, and what it wrote or how it ended could not be recorded"
	case repair.PhaseRecord:
		return "the session ran, and recording that it ran failed"
	case repair.PhaseRederive:
		return "the session ran, and the promise could not be measured in the box afterwards"
	}
	return "the repair failed"
}

// errorWords is the failure without the box sentence, which the report already
// gives a row of its own.
func errorWords(f *repair.Failure) string { return f.Err.Error() }
