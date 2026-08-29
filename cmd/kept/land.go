package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/pbleisch/kept/internal/config"
	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/land"
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/progress"
	"github.com/pbleisch/kept/internal/run"
	"github.com/pbleisch/kept/internal/thread"
)

// landCmd puts one thread's gated repair on the landing target.
//
// It is the only verb that *can* reach the network, and it is a person's act: a
// tick never lands, a repair never lands, and nothing here happens without
// somebody typing this. Whether it reaches the network at all is a property of
// the target they named — a target on a remote is fetched and pushed, and one
// on this machine is a ref read and a ref write. See docs/decision/0015.
func landCmd(args []string, stdout, stderr io.Writer) int {
	fs, root := flags("land", stderr)
	onto := fs.String("onto", "", "the branch to land on, as \"<remote>/<branch>\" or \"<branch>\"; else this branch's upstream, else this branch")
	arrival := fs.String("arrival", "", "pin how the change arrives: \"ref\" advances the branch, \"worktree\" merges into this working tree")
	dry := fs.Bool("dry-run", false, "run the gate and stop before the lock; read nothing under it and write nothing")
	keep := fs.Bool("keep", false, "leave the gate box behind instead of discarding it")
	quiet := fs.Bool("quiet", false, "no progress on stderr")
	by := fs.String("by", "", "who is performing this; else KEPT_BY, else operator: in your user config")
	rest, err := parse(fs, args)
	if err != nil {
		return exitUsage
	}
	if len(rest) > 1 {
		fmt.Fprint(stderr, "kept: land takes one thread\n")
		return exitUsage
	}

	c, code := open(*root, stderr)
	if code != exitOK {
		return code
	}
	if code := sound(c, stderr); code != exitOK {
		return code
	}
	// A land records an act, so it writes, so it advances first. See
	// docs/decision/0062.
	advance(c, stderr)

	cfg, err := config.Read(filepath.Join(c.Workspace, corpus.ConfigDir))
	if err != nil {
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return exitUsage
	}
	// Where the change is going and how it gets there, resolved before
	// anything is gated so it can be printed before anything is acted on. A
	// silently resolved arrival is exactly how `landed` comes to mean two
	// things. See docs/decision/0015.
	plan, planned := landingPlan(context.Background(), c.Workspace, *onto, *arrival, cfg.Git)

	store := ledgerStore(c)
	if len(rest) == 0 {
		// The listing gates nothing and writes nothing, so a workspace with no
		// target it could resolve still gets the answer it asked for. It says
		// what it could not work out rather than reporting it as a failure.
		return writeLandable(c, store, plan, planned, stdout, stderr)
	}
	if planned != nil {
		fmt.Fprintf(stderr, "kept: %s\n", planned)
		return exitUsage
	}

	who, code := operator(*by, stderr)
	if code != exitOK {
		return code
	}
	acts, quarantined, err := ledger.ReadActs(store)
	if err != nil {
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return exitUsage
	}
	writeQuarantined(stderr, quarantined)

	writePlan(stdout, rest[0], plan)

	l := ledger.Open(store, startRun(append([]string{"land"}, args...)))
	show := progress.For(stdout, stderr, *quiet)
	res, err := land.Perform(context.Background(), land.Request{
		Workspace: c.Workspace,
		Corpus:    c,
		Acts:      acts,
		Thread:    rest[0],
		Target:    plan.Target,
		Arrival:   plan.Arrival,
		By:        who,
		Deadlines: land.Deadlines{Gatherer: cfg.Gatherer, Git: cfg.Git},
		Exclude:   cfg.Exclude,
		Store:     store,
		Ledger:    l,
		Progress:  show,
		DryRun:    *dry,
		Keep:      *keep,
	})
	show.Close()
	if err != nil {
		_ = l.Close(run.Failed)
		var unlandable *land.Unlandable
		if errors.As(err, &unlandable) {
			fmt.Fprintf(stderr, "kept: %s\n", unlandable.Why)
			return exitUsage
		}
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return unrecorded(stderr, err)
	}
	if err := l.Close(run.Clean); err != nil {
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return unrecorded(stderr, err)
	}

	writeLand(stdout, res)
	return landAnswer(res)
}

// landAnswer is one land as an exit code.
//
// A refusal is 1 for the reading repairAnswer already records: nothing here
// refuses to *answer*, so a refusal to land drift it has reported is drift.
// Contention is 3 rather than 1, and for the same reason as [unrecorded] — a
// land that gated and then lost a race produced no evidence either way, and
// reporting the target as broken because another run got there first is the CLI
// recording what it thinks it saw.
func landAnswer(res *land.Result) int {
	switch res.Status {
	case land.Landed, land.Prepared:
		return exitOK
	case land.Refused:
		if res.Condition == land.Contended {
			return exitUnknown
		}
		return exitDrift
	default:
		return exitUnknown
	}
}

// landingPlan resolves where this land is going and how the change will get
// there.
//
// Both halves refuse rather than fall back. A target that is not a branch this
// repository has — or a detached HEAD with no --onto — is the operator asking
// for a place a land cannot reach, and an arrival the target does not admit is
// the operator asking for a guarantee it cannot carry; quietly satisfying
// either with the other is how a record comes to mean two things. See
// docs/decision/0015.
func landingPlan(ctx context.Context, workspace, onto, arrival string, deadline time.Duration) (land.Plan, error) {
	target, err := land.Origin(), error(nil)
	switch {
	case onto != "":
		target, err = land.ParseTarget(ctx, workspace, onto, deadline)
	default:
		target, err = land.ResolveTarget(ctx, workspace, deadline)
	}
	if err != nil {
		return land.Plan{}, err
	}
	return land.Resolve(ctx, workspace, target, ledger.Arrival(arrival), deadline)
}

// writePlan says where the change is going and how it will get there, before
// the gate runs.
//
// Printed rather than left implicit, because resolving the arrival is fine and
// printing the resolution is what makes it fine: the two arrivals do not carry
// the same guarantee, and a person who did not ask for one has to be told which
// they are getting while they can still stop it.
func writePlan(w io.Writer, thread string, plan land.Plan) {
	fmt.Fprintf(w, "landing thread %s on %s\n", thread, plan.Target)
	fmt.Fprintf(w, "  %s regime, %s arrival — %s\n\n", plan.Regime, plan.Arrival, plan.Because())
}

// writeLandable is `kept land` with nothing named: the threads that anchored a
// commit, and are therefore something a land could be pointed at.
//
// It gates nothing, so it says nothing about whether any of them would land —
// that answer costs a box and a corpus-wide re-derivation, and a listing that
// implied it would be claiming a gate it never ran.
func writeLandable(c *corpus.Corpus, store ledger.Store, plan land.Plan, planned error, stdout, stderr io.Writer) int {
	anchored, err := ledger.AnchoredThreads(c.Workspace)
	if err != nil {
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return exitUsage
	}
	if len(anchored) == 0 {
		fmt.Fprint(stdout, "no thread has anchored a commit, so there is nothing to land.\n")
		fmt.Fprint(stdout, "kept repair <promise> produces one: a session that changes the box commits what it left.\n")
		return exitOK
	}

	ids := make([]string, 0, len(anchored))
	for id := range anchored {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	if planned != nil {
		fmt.Fprint(stdout, "these threads anchored a commit, and one could be landed:\n\n")
	} else {
		fmt.Fprintf(stdout, "these threads anchored a commit, and one can be landed on %s:\n\n", plan.Target)
	}
	t := table{indent: "  "}
	for _, id := range ids {
		promise := "—"
		if entries, _, err := thread.ReadReceipt(store, thread.ID(id)); err == nil {
			if anchor, found := thread.LatestAnchor(entries); found {
				promise = anchor.Promise
			}
		}
		t.add(shortCommit(anchored[id]), promise, id)
	}
	t.write(stdout)
	fmt.Fprint(stdout, "\nkept land <thread> gates one and lands it. The gate re-derives every promise\n"+
		"the change touches, so nothing here says whether one would pass.\n")
	if planned != nil {
		fmt.Fprintf(stdout, "where it would arrive is not settled: %s\n", planned)
	} else {
		fmt.Fprintf(stdout, "it would arrive as a %s arrival: %s.\n", plan.Arrival, plan.Because())
	}
	return exitUsage
}

// writeLand is what a person reads back: what was gated, against what, what the
// gate found, and what happened.
func writeLand(w io.Writer, res *land.Result) {
	fmt.Fprintf(w, "%s\n", res.Promise)
	fmt.Fprintf(w, "  thread %s anchored %s\n", res.Thread, shortCommit(res.Anchor.Commit))
	if res.GatedAgainst != "" {
		fmt.Fprintf(w, "  gated against %s at %s\n", res.Target, shortCommit(res.GatedAgainst))
	}
	if len(res.Moved) > 0 {
		fmt.Fprintf(w, "  %s moved under %s, and the gate ran again\n",
			res.Target, plural(len(res.Moved), "attempt"))
	}

	writeGate(w, res)

	switch res.Status {
	case land.Refused:
		fmt.Fprintf(w, "\nrefused · %s\n", res.Condition)
		for _, line := range strings.Split(strings.TrimSpace(res.Detail), "\n") {
			fmt.Fprintf(w, "  %s\n", line)
		}
		fmt.Fprintf(w, "\nnothing landed, and %s.\n", untouched(res))
	case land.Prepared:
		fmt.Fprintf(w, "\n%s\n", strings.TrimSpace(res.Detail))
	case land.Landed:
		writeArrived(w, res)
		if res.Recorded {
			fmt.Fprintf(w, "  recorded: %s by %s\n", res.Act.Kind, res.Act.By)
		}
	}
	if res.Box != "" {
		fmt.Fprintf(w, "\nthe gate box is at %s\n", res.Box)
	}
	if res.Warning != "" {
		fmt.Fprintf(w, "\n%s\n", res.Warning)
	}
}

// writeArrived says what happened, in the words of the arrival that happened.
//
// The two never share a sentence. A ref advance names the commit that reached
// the target, because that is the thing a reader can go and look at; a worktree
// arrival says in its place that nothing was committed, and names the paths it
// changed instead — which is all there is to look at, and is what the operator
// now has in front of them.
func writeArrived(w io.Writer, res *land.Result) {
	if res.Plan.Arrival != ledger.WorktreeArrival {
		fmt.Fprintf(w, "\nlanded %s on %s\n", shortCommit(res.Commit), res.Target)
		return
	}
	fmt.Fprintf(w, "\nmerged into this working tree · nothing was committed and %s did not move\n", res.Target)
	if len(res.Arrived) == 0 {
		fmt.Fprint(w, "  this working tree already held the change, so nothing was written\n")
		return
	}
	for _, path := range res.Arrived {
		fmt.Fprintf(w, "  %s\n", path)
	}
	fmt.Fprint(w, "  these are yours now: nothing is staged and nothing is committed\n")
}

// untouched is what a refusal promises it did not disturb, which differs by
// arrival: one would have moved a ref and the other would have written into the
// tree the operator is standing in.
func untouched(res *land.Result) string {
	if res.Plan.Arrival == ledger.WorktreeArrival {
		return "this working tree is exactly as it was"
	}
	return "nothing on the target moved"
}

// writeGate prints what the gate measured, on both sides of the merge. The
// before column is what makes a collateral break readable as one: a promise
// that was already failing on the target is not this change's doing.
func writeGate(w io.Writer, res *land.Result) {
	if len(res.Gate) == 0 {
		return
	}
	fmt.Fprintf(w, "\nthe gate re-derived %s the change touches:\n", plural(len(res.Gate), "promise"))
	t := table{indent: "  "}
	for _, v := range res.Gate {
		note := ""
		switch {
		case v.Broke():
			note = "broken by this change"
		case v.Artifact == res.Promise:
			note = "the promise this thread repaired"
		}
		t.add(string(v.Before)+" → "+string(v.After), note, v.Artifact)
	}
	t.write(w)
	if !res.Review.Bounded() {
		fmt.Fprint(w, "\noutside the repair scope this promise bound, and nothing that reaches past it may land\n")
		for _, path := range res.Review.Outside {
			fmt.Fprintf(w, "  %s\n", path)
		}
	}
}

// plural is a count and its noun, so a report reads as a sentence.
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
