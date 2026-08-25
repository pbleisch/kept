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
	"github.com/pbleisch/kept/internal/land"
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/progress"
	"github.com/pbleisch/kept/internal/run"
	"github.com/pbleisch/kept/internal/thread"
)

// landCmd puts one thread's gated repair on the landing target.
//
// It is the only verb that reaches the network, and it is a person's act: a
// tick never lands, a repair never lands, and nothing here happens without
// somebody typing this. See docs/decision/0015.
func landCmd(args []string, stdout, stderr io.Writer) int {
	fs, root := flags("land", stderr)
	dry := fs.Bool("dry-run", false, "run the gate and stop before the lock; fetch nothing under it and push nothing")
	keep := fs.Bool("keep", false, "leave the gate box behind instead of discarding it")
	quiet := fs.Bool("quiet", false, "say nothing while the land runs")
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

	store := ledgerStore(c)
	if len(rest) == 0 {
		return writeLandable(c, store, stdout, stderr)
	}

	who, code := operator(*by, stderr)
	if code != exitOK {
		return code
	}
	cfg, err := config.Read(filepath.Join(c.Workspace, corpus.ConfigDir))
	if err != nil {
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return exitUsage
	}
	acts, quarantined, err := ledger.ReadActs(store)
	if err != nil {
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return exitUsage
	}
	writeQuarantined(stderr, quarantined)

	l := ledger.Open(store, startRun(append([]string{"land"}, args...)))
	show := progress.For(stdout, stderr, *quiet)
	res, err := land.Perform(context.Background(), land.Request{
		Workspace: c.Workspace,
		Corpus:    c,
		Acts:      acts,
		Thread:    rest[0],
		Target:    land.Origin(),
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

// writeLandable is `kept land` with nothing named: the threads that anchored a
// commit, and are therefore something a land could be pointed at.
//
// It gates nothing, so it says nothing about whether any of them would land —
// that answer costs a box and a corpus-wide re-derivation, and a listing that
// implied it would be claiming a gate it never ran.
func writeLandable(c *corpus.Corpus, store ledger.Store, stdout, stderr io.Writer) int {
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

	fmt.Fprintf(stdout, "these threads anchored a commit, and one can be landed on %s:\n\n", land.Origin())
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
		"this change touches, not only the one the thread was about, so nothing here\n"+
		"says whether any of them would pass it.\n")
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
		fmt.Fprint(w, "\nnothing landed, and nothing on the target moved.\n")
	case land.Prepared:
		fmt.Fprintf(w, "\n%s\n", strings.TrimSpace(res.Detail))
	case land.Landed:
		fmt.Fprintf(w, "\nlanded %s on %s\n", shortCommit(res.Commit), res.Target)
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
		fmt.Fprint(w, "\noutside the repair scope this promise bound, and nothing that reaches it may land\n")
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
