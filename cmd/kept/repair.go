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
// It lands nothing, ever. The box is discarded, the operator's tree is not
// touched, nothing is committed and nothing is pushed: what comes back is a diff
// to read and, with --keep, a worktree to read it in.
func repairCmd(args []string, stdout, stderr io.Writer) int {
	fs, root := flags("repair", stderr)
	dry := fs.Bool("dry-run", false, "cut the box, assemble the packet and verify the provider; dispatch nothing")
	keep := fs.Bool("keep", false, "leave the box behind instead of discarding it")
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
	res, err := repair.Perform(context.Background(), repair.Request{
		Workspace: c.Workspace,
		Corpus:    c,
		Acts:      acts,
		Promise:   id,
		Provider:  cfg.Provider,
		Operator:  user.Repair,
		Deadlines: repair.Deadlines{Gatherer: cfg.Gatherer, Agent: cfg.Agent, Git: cfg.Git},
		Exclude:   cfg.Exclude,
		Store:     store,
		Ledger:    l,
		Keep:      *keep,
		DryRun:    *dry,
	})
	if err != nil {
		_ = l.Close(run.Failed)
		var unrepairable *repair.Unrepairable
		if errors.As(err, &unrepairable) {
			fmt.Fprintf(stderr, "kept: %s\n", unrepairable.Why)
			return exitUsage
		}
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return exitUsage
	}
	if err := l.Close(run.Clean); err != nil {
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return exitUsage
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
	for _, s := range repairable {
		fmt.Fprintf(stdout, "  %-38s %-12s %s\n", s.ID, s.Drift.Kind, ago(s.Drift.Since))
	}
	fmt.Fprintf(stdout, "\nkept repair <promise> dispatches one. It repairs one promise, not several:\n"+
		"choosing among them is a ranking and a budget, and neither is built.\n")
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
		return
	case repair.Holds:
		fmt.Fprintf(w, "\nthe promise holds at that commit, so there is no gap for a packet to state\n")
		fmt.Fprintf(w, "  %s\n", firstLine(res.Before.Detail))
		fmt.Fprint(w, "  drift the working tree carries and the commit does not reads like this;\n"+
			"  so does a verdict nothing has refreshed. kept re-derives it.\n")
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
	fmt.Fprint(w, "  the verdict is that re-derivation and never the session's account of itself\n")

	fmt.Fprintf(w, "\n%s", indent(res.Review.String()))
	writeDispositionNote(w, res)

	if len(res.Change.Diff) == 0 {
		fmt.Fprint(w, "\nthe session changed nothing in the box.\n")
		return
	}
	if res.Box != "" {
		fmt.Fprintf(w, "\nthe box is at %s\n", res.Box)
		fmt.Fprint(w, "  git -C <box> diff reads it there, and git worktree remove discards it\n")
	}
	fmt.Fprint(w, "\nnothing landed. This diff is yours to read, and the merge is your hands on git:\n\n")
	fmt.Fprintf(w, "%s", res.Change.Diff)
	if len(res.Change.Diff) > 0 && res.Change.Diff[len(res.Change.Diff)-1] != '\n' {
		fmt.Fprintln(w)
	}
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
	fmt.Fprint(w, "  a scope is a glob and a grant is a path, so this is at least as wide as the\n"+
		"  bound it stands for. The review below is what reports, and what a land refuses on.\n")
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
