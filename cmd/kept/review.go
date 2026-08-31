package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"time"

	"github.com/pbleisch/kept/internal/assess"
	"github.com/pbleisch/kept/internal/config"
	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/thread"
)

// reviewCmd works the assessor's checklist over one repair thread.
//
// It judges the thread rather than the artifact. The predicate already answered
// *is the promise kept now*, in the box, from the tree the session left; this
// answers the question nothing else asks, which is whether that was a
// legitimate way to get there — and it answers it check by check, because which
// one fired is the whole diagnostic value.
//
// It reads and never writes. It does not advance the ledger the way `tick`,
// `ratify`, `retire` and `repair` do, for the reason `status`, `why` and
// `drift` do not: a reader that writes cannot be run against a checkout nobody
// may write to, and every finding here is derived from evidence that is already
// durable. See docs/decision/0062 and 0078.
func reviewCmd(args []string, stdout, stderr io.Writer) int {
	fs, root := flags("review", stderr)
	asJSON := fs.Bool("json", false, "machine-readable output; the only stable format")
	rest, err := parse(fs, args)
	if err != nil {
		return exitUsage
	}
	if len(rest) > 1 {
		fmt.Fprint(stderr, "kept: review takes one thread\n")
		return exitUsage
	}

	c, code := open(*root, stderr)
	if code != exitOK {
		return code
	}
	if code := sound(c, stderr); code != exitOK {
		return code
	}
	store := ledgerStore(c)
	if len(rest) == 0 {
		return writeReviewable(c, store, stdout, stderr)
	}

	deadline := time.Duration(0)
	if cfg, err := config.Read(filepath.Join(c.Workspace, corpus.ConfigDir)); err == nil {
		deadline = cfg.Git
	}
	assessment, err := assess.Assess(context.Background(), assess.Request{
		Store:      store,
		Repository: c.Workspace,
		Corpus:     c,
		Thread:     thread.ID(rest[0]),
		Deadline:   deadline,
	})
	if err != nil {
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return exitUsage
	}

	code = reviewAnswer(assessment)
	if *asJSON {
		return writeDocument(stdout, stderr, reviewDocument(assessment, code))
	}
	writeReview(stdout, stderr, assessment)
	return code
}

// reviewAnswer is one assessment as an exit code.
//
// A fired check is 0, and that is the same reading `kept doctor` gives its own
// findings: nothing here refuses, nothing auto-lands, and reporting a finding
// as drift would say a promise with standing is not kept when the promise is
// kept and the way it got there is in question. What is not 0 is a checklist
// nothing could work — no evidence either way, which is 0027's own definition
// of 3 — and a receipt line that could not be read, for the same reason every
// other verb reports one.
func reviewAnswer(a assess.Assessment) int {
	if !a.Worked() || len(a.Entry.Quarantined) > 0 {
		return exitUnknown
	}
	return exitOK
}

// writeReview prints the checklist.
//
// Every check, every time, including the ones that found nothing and the ones
// nothing could work. A row that is left out is a row nobody goes looking for,
// and the whole reason to record a check on its own is so that a reader can see
// which of the six is which.
func writeReview(stdout, stderr io.Writer, a assess.Assessment) {
	writeQuarantined(stderr, a.Entry.Quarantined)
	if a.Promise != "" {
		fmt.Fprintf(stdout, "%s\n", a.Promise)
	}
	fmt.Fprintf(stdout, "thread %s", a.Thread)
	if a.Entry.Anchored {
		fmt.Fprintf(stdout, " · anchored at %s", shortCommit(a.Entry.Anchor.Commit))
	}
	// How the session ended, because a repair that was killed at its deadline
	// and still anchored something is a different act from one that finished,
	// and a reviewer asking whether this was a legitimate way to get there
	// should not have to go and find that out.
	if ended := lifecycle(a); ended != "" {
		fmt.Fprintf(stdout, " · %s", ended)
	}
	fmt.Fprint(stdout, "\n\n")
	fmt.Fprint(stdout, a.String())
	if len(a.Fired()) > 0 {
		fmt.Fprint(stdout, "\na finding that keeps coming back is a predicate nobody has written:\n"+
			"the answer to one is usually a clause in hold_when, not a rule here.\n")
	}
}

// writeReviewable lists the threads there is something to review.
//
// The refs are the queue, exactly as they are for `kept land`: a thread whose
// work still exists is a thread git already indexes, and there is no listing
// operation on the ledger store for this to reach for instead.
func writeReviewable(c *corpus.Corpus, store ledger.Store, stdout, stderr io.Writer) int {
	anchored, err := ledger.AnchoredThreads(c.Workspace)
	if err != nil {
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return exitUsage
	}
	if len(anchored) == 0 {
		fmt.Fprint(stdout, "no thread has anchored a commit, so there is nothing to review.\n")
		fmt.Fprint(stdout, "kept repair <promise> produces one: a session that changes the box commits what it left.\n")
		return exitOK
	}
	ids := make([]string, 0, len(anchored))
	for id := range anchored {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	fmt.Fprint(stdout, "these threads anchored a commit, and one can be reviewed:\n\n")
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
	fmt.Fprint(stdout, "\nkept review <thread> works the checklist over one. It measures nothing and\n"+
		"records nothing; what it reads is what the repair already made durable.\n")
	return exitOK
}

// lifecycle is what the parent observed of the session, in a phrase.
//
// The last event, because a receipt is append-only and the last one is how it
// ended. Empty is a receipt that recorded none, which is not the same as a
// session that ran cleanly and is said by printing nothing.
func lifecycle(a assess.Assessment) string {
	if len(a.Entry.Lifecycle) == 0 {
		return ""
	}
	last := a.Entry.Lifecycle[len(a.Entry.Lifecycle)-1]
	switch {
	case last.Event == thread.TimedOut:
		return "timed out at " + last.Timeout
	case last.Event == thread.Exited && last.Exit != nil && last.Exit.Signal != "":
		return "reaped on " + last.Exit.Signal
	case last.Event == thread.Exited && last.Exit != nil && last.Exit.Code != nil:
		return fmt.Sprintf("exited %d", *last.Exit.Code)
	}
	return string(last.Event)
}
