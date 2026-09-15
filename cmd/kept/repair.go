package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pbleisch/kept/internal/config"
	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/progress"
	"github.com/pbleisch/kept/internal/repair"
	"github.com/pbleisch/kept/internal/run"
	"github.com/pbleisch/kept/internal/tick"
)

// repairCmd hands one drifted promise to an agent and gives back a bounded diff.
//
// One promise, named. Several drifts and one credential is a ranking and a
// budget, and neither exists yet; with no name this prints what could be handed
// over and makes the choice a person's. See docs/decision/0060.
//
// It lands nothing, ever. The operator's tree is not touched and nothing is
// pushed: what comes back is a diff to read and, with --keep, a worktree to read
// it in. What the session left is committed and anchored under
// refs/kept/threads/<thread> before the box goes, so `kept land` has something
// to gate — the commit is not on any branch and reaches nothing until a person
// lands it. See docs/decision/0067.
func repairCmd(args []string, stdout, stderr io.Writer) int {
	fs, root := flags("repair", stderr)
	dry := fs.Bool("dry-run", false, "cut the box, assemble the packet and verify the provider; dispatch nothing")
	keep := fs.Bool("keep", false, "leave the box behind instead of discarding it")
	quiet := fs.Bool("quiet", false, "no progress on stderr")
	rest, err := parse(fs, args)
	if err != nil {
		return exitUsage
	}
	if len(rest) > 1 {
		fmt.Fprint(stderr, "kept: repair takes one promise, named \"type:slug\" or by its slug alone\n")
		return exitUsage
	}

	c, code := open(*root, stderr)
	if code != exitOK {
		return code
	}
	if code := sound(c, stderr); code != exitOK {
		return code
	}
	// A repair records a thread and its dispositions, so it writes, so it
	// advances first. See docs/decision/0062.
	advance(c, stderr)
	if len(rest) == 0 {
		return writeRepairable(c, stdout, stderr)
	}
	id, code := lookup(c, rest[0], stderr)
	if code != exitOK {
		return code
	}

	cfg, err := config.Read(filepath.Join(c.Workspace, corpus.ConfigDir))
	if err != nil {
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return exitUsage
	}
	user, err := config.ReadUser()
	if err != nil {
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return exitUsage
	}

	store := ledgerStore(c)
	acts, quarantined, err := ledger.ReadActs(store)
	if err != nil {
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return exitUsage
	}
	writeQuarantined(stderr, quarantined)

	l := ledger.Open(store, startRun(append([]string{"repair"}, args...)))
	// A repair is the longest thing kept does: a confined session is bounded by
	// timeouts.agent, sixty minutes here, and the phases either side of it were
	// each measured at twenty-seven seconds over this repository. It is closed
	// before the report is printed. See docs/decision/0065.
	show := progress.For(stdout, stderr, *quiet)
	res, err := repair.Perform(context.Background(), repair.Request{
		Workspace: c.Workspace,
		Corpus:    c,
		Acts:      acts,
		Promise:   id,
		By:        user.Operator,
		Provider:  cfg.Provider,
		Operator:  user.Repair,
		Assessor:  user.Assess,
		Deadlines: repair.Deadlines{Gatherer: cfg.Gatherer, Agent: cfg.Agent, Git: cfg.Git},
		Exclude:   cfg.Exclude,
		Store:     store,
		Ledger:    l,
		Progress:  show,
		Keep:      *keep,
		DryRun:    *dry,
	})
	show.Close()
	if err != nil {
		_ = l.Close(run.Failed)
		var unrepairable *repair.Unrepairable
		if errors.As(err, &unrepairable) {
			fmt.Fprintf(stderr, "kept: %s\n", unrepairable.Why)
			return exitUsage
		}
		// A repair that failed after it opened a thread explains itself from
		// the thread's receipt, because the one-line error is the least of what
		// a person needs: how far the session got, how it ended, where its work
		// is, and what to run. See docs/decision/0085.
		var failure *repair.Failure
		if errors.As(err, &failure) {
			writeRepairFailure(stderr, store, failure)
			return unrecorded(stderr, err)
		}
		// A session that ran and then something else failed is a session that
		// spent a credential, so the thread it left is named before the error
		// rather than lost with it.
		if res != nil && res.Thread != "" {
			fmt.Fprintf(stderr, "kept: thread %s ran a confined session, and its receipt is in the ledger\n", res.Thread)
		}
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return unrecorded(stderr, err)
	}
	if err := l.Close(run.Clean); err != nil {
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return unrecorded(stderr, err)
	}

	writeRepair(stdout, res)
	return repairAnswer(res)
}

// repairAnswer is one repair as an exit code, in docs/decision/0027's terms.
//
// 1 is where a refusal lands, and that is the reading main.go already records:
// nothing here refuses to *answer*, so a refusal to act on drift it has reported
// is drift. A repair that reached outside the scope its promise bound is 1 as
// well — the promise may hold in that box, and nothing that reaches past its
// bound may land.
func repairAnswer(res *repair.Result) int {
	switch res.Status {
	case repair.Repaired:
		if !res.Review.Bounded() {
			return exitDrift
		}
		return exitOK
	case repair.Unrepaired, repair.Refused:
		return exitDrift
	case repair.Undecided, repair.Unmeasured:
		return exitUnknown
	default:
		// Holds and Prepared. Nothing is broken at the commit, or nothing was
		// asked to run.
		return exitOK
	}
}

// writeRepairable is `kept repair` with nothing named: the promises a repair
// could be pointed at, from what the last tick found.
//
// It observes nothing, so it is a reading of the queue rather than a fresh
// verdict, and it says so. Only a promise that is *not kept* is listed: an
// answer that is stale, unverified or unknown says nobody has measured, and the
// answer to that is a tick rather than an agent.
func writeRepairable(c *corpus.Corpus, stdout, stderr io.Writer) int {
	res, code := survey(c, stderr)
	if code != exitOK {
		return code
	}

	var repairable []tick.Subject
	for _, s := range res.Subjects {
		if s.Drifting && repairableKind(s.Drift.Kind) {
			repairable = append(repairable, s)
		}
	}
	if len(repairable) == 0 {
		fmt.Fprint(stdout, "nothing with standing reads not-kept, so there is nothing to hand to an agent.\n")
		fmt.Fprint(stdout, "kept drift says what else is unanswered; a stale or unverified promise wants a tick, not an agent.\n")
		return exitOK
	}

	fmt.Fprint(stdout, "these promises read not-kept as of the last tick, and one can be handed to an agent:\n\n")
	t := table{indent: "  "}
	for _, s := range repairable {
		t.add(string(s.Drift.Kind), ago(s.Drift.Since), s.ID)
	}
	t.write(stdout)
	fmt.Fprint(stdout, "\nkept repair <promise> dispatches one. One promise per run: ranking several against a budget is not built.\n")
	return exitUsage
}

func repairableKind(kind model.DriftKind) bool {
	return kind == model.DriftNotKept || kind == model.DriftRegressed
}

// writeRepair is what a person reads back: what kept did, what it recorded, what
// the promise now says, and the change measured against the scope its promise
// bound.
func writeRepair(w io.Writer, res *repair.Result) {
	fmt.Fprintf(w, "%s\n", res.Promise)
	if res.Commit != "" {
		fmt.Fprintf(w, "  box cut at %s\n", shortCommit(res.Commit))
	}

	switch res.Status {
	case repair.Refused:
		fmt.Fprintf(w, "\nrefused · %s\n", res.Condition)
		for _, line := range strings.Split(strings.TrimSpace(res.Detail), "\n") {
			fmt.Fprintf(w, "  %s\n", line)
		}
		writeDispositionNote(w, res)
		writeRepairWarning(w, res)
		return
	case repair.Holds:
		fmt.Fprint(w, "\nthe promise holds at that commit, so there is nothing to repair\n")
		fmt.Fprintf(w, "  %s\n", firstLine(res.Before.Detail))
		fmt.Fprint(w, "  an uncommitted break reads like this, and so does a stale verdict.\n"+
			"  Run kept to re-derive.\n")
		return
	case repair.Unmeasured:
		fmt.Fprint(w, "\nnothing could measure the promise in that box, so there is no evidence to carry\n")
		for _, line := range strings.Split(strings.TrimSpace(res.Before.Detail), "\n") {
			fmt.Fprintf(w, "  %s\n", line)
		}
		return
	}

	writeRepairScope(w, res)
	if res.Status == repair.Prepared {
		fmt.Fprint(w, "\nthe provider verified and the packet is assembled. Nothing was dispatched.\n")
		fmt.Fprintf(w, "  the promise reads %s in that box\n", res.Before.Value)
		fmt.Fprintf(w, "\nthe packet, which is every word the agent is given:\n\n%s\n", indent(string(res.Packet.Bytes())))
		return
	}

	fmt.Fprintf(w, "\nthread %s ran a confined session\n", res.Thread)
	fmt.Fprintf(w, "  re-derived from the box: %s → %s\n", res.Before.Value, res.After.Value)
	if res.After.Detail != "" {
		fmt.Fprintf(w, "  %s\n", firstLine(res.After.Detail))
	}

	// The review is printed only when the change was read. A review of a change
	// nobody could collect says "the repair changed nothing", which is the one
	// thing it must not say when the session changed too much to retain.
	if res.Change.Paths != nil || res.Warning == "" {
		fmt.Fprintf(w, "\n%s", indent(res.Review.String()))
	}
	writeDispositionNote(w, res)
	writeArgument(w, res.Argument)
	writeRepairWarning(w, res)
	if len(res.Change.Diff) == 0 {
		if res.Warning == "" {
			fmt.Fprint(w, "\nthe session changed nothing in the box.\n")
		}
		return
	}
	if res.Box != "" {
		fmt.Fprintf(w, "\nthe box is at %s\n", res.Box)
		fmt.Fprint(w, "  git -C <box> diff reads it there, and git worktree remove discards it\n")
	}
	fmt.Fprint(w, "\nnothing landed. The diff is yours to read:\n\n")
	fmt.Fprintf(w, "%s", res.Change.Diff)
	if len(res.Change.Diff) > 0 && res.Change.Diff[len(res.Change.Diff)-1] != '\n' {
		fmt.Fprintln(w)
	}
}

// writeArgument prints what the adversarial pass argued, whole.
//
// Whole, and not a count or a summary. The half of the assessor this reports is
// the one whose value is what nobody enumerated, so condensing it would throw
// away the thing it was brought in for — and there is nothing here to condense
// it *by*, because no finding shape was declared over its output.
//
// It gates nothing and it moves no exit code. A pass that argued is printed
// beside the verdict the re-derivation reached and never in front of it, which
// is policy:the-agents-report-is-never-an-input-to-the-verdict as a matter of
// layout as well as of code.
func writeArgument(w io.Writer, pass *repair.ArgueResult) {
	if pass == nil {
		return
	}
	if pass.Status != repair.Argued {
		fmt.Fprintf(w, "\nno adversarial pass read this thread · %s\n", pass.Status)
		for _, line := range strings.Split(strings.TrimSpace(pass.Detail), "\n") {
			if line != "" {
				fmt.Fprintf(w, "  %s\n", line)
			}
		}
		return
	}
	fmt.Fprintf(w, "\nan adversarial pass read this thread in a box it could not write, and argued:\n\n")
	fmt.Fprintf(w, "%s", indent(strings.TrimRight(pass.Said, "\n")+"\n"))
	fmt.Fprint(w, "\nthat is an argument and not a verdict. The promise's answer is the re-derivation above,\n"+
		"and nothing here decides whether this may land.\n")
}

// writeRepairScope prints what the promise bound and what confinement was
// actually granted, because the second is always at least as wide as the first
// and a person reading a bounded review should be able to see the difference.
func writeRepairScope(w io.Writer, res *repair.Result) {
	if len(res.Grants) == 0 {
		return
	}
	fmt.Fprint(w, "\nthe box could write:\n")
	grants := append([]string(nil), res.Grants...)
	sort.Strings(grants)
	for _, grant := range grants {
		fmt.Fprintf(w, "  %s\n", grant)
	}
	fmt.Fprint(w, "  a scope is a glob and a grant is a path, so this is at least as wide as the bound. The review below is what a land refuses on.\n")
}

// writeRepairWarning is what went wrong after the result was decided: a box that
// would not discard, or a diff too large to retain. Neither changes the verdict,
// and both leave something in the repository a person has to know about.
func writeRepairWarning(w io.Writer, res *repair.Result) {
	if res.Warning == "" {
		return
	}
	fmt.Fprintf(w, "\n%s\n", res.Warning)
	if res.Box != "" {
		fmt.Fprintf(w, "  the box is at %s\n", res.Box)
	}
}

func writeDispositionNote(w io.Writer, res *repair.Result) {
	switch {
	case res.Recorded:
		fmt.Fprintf(w, "\nrecorded: %s", dispositionLine(res))
	case res.Outcome != "":
		fmt.Fprintf(w, "\nunchanged from what already stood: %s", dispositionLine(res))
	}
}

func dispositionLine(res *repair.Result) string {
	if res.Condition == "" {
		return fmt.Sprintf("%s\n", res.Outcome)
	}
	return fmt.Sprintf("%s · %s\n", res.Outcome, res.Condition)
}

// indent shifts a block two spaces so a report's sections read as one document.
func indent(src string) string {
	var out strings.Builder
	for line := range strings.SplitSeq(strings.TrimRight(src, "\n"), "\n") {
		if line == "" {
			out.WriteString("\n")
			continue
		}
		out.WriteString("  " + line + "\n")
	}
	return out.String()
}
