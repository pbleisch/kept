package main

import (
	"fmt"
	"io"

	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/ledger"
)

// ledgerCmd is the verb docs/decision/0016 named and never built. It has one
// subcommand, and the reason there is only one is worth stating rather than
// leaving as an omission.
//
// 0016 named three — `sync`, `push`, `fsck`. `push` is `git push origin
// refs/kept/ledger:refs/kept/ledger`, which is git's job and which kept
// wrapping would turn into a network call inside a tool that has never made
// one; and *every run pushes at exit*, which 0016 also decided, is rejected on
// the measurement rather than on taste — had it existed, the first tick after
// the 2026-08-22 rewind would have published the damaged ledger to origin as a
// clean fast-forward, overwriting the last copy of the good one. `fsck` is
// [ledger.Continuous], which every verb already runs.
//
// See docs/decision/0062.
func ledgerCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		ledgerUsage(stderr)
		return exitUsage
	}
	switch args[0] {
	case "sync":
		return ledgerSyncCmd(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		ledgerUsage(stdout)
		return exitOK
	}
	fmt.Fprintf(stderr, "kept: ledger has no subcommand %q\n", args[0])
	ledgerUsage(stderr)
	return exitUsage
}

func ledgerUsage(w io.Writer) {
	fmt.Fprintf(w, `kept ledger sync    advance %s onto what the last git fetch
                    brought, when that is a fast-forward

Run git fetch first: kept makes no network call. A fetch writes
%s and never the ledger itself, so sync is the
second step. It does not push and it does not merge; a fork is refused.
`, ledger.Ref, ledger.UpstreamRef)
}

// ledgerSyncCmd fast-forwards the ledger onto the upstream ref, and refuses a
// divergence rather than resolving one.
//
// **What it does not attempt, said plainly.** It does not fetch, it does not
// push, and it does not merge. Two clones that both appended have forked, and
// the answer for an append-only record is the union of both sides — which is a
// commit somebody has to make, with both tips named, and which nothing here
// will make on their behalf. A fast-forward cannot lose a record; a merge kept
// composed could, and would do it to the one file in this system that is not
// reproducible.
//
// Nothing is lost while it is refused. Both tips are refs in this repository,
// so the union stays available for as long as somebody wants to make it.
func ledgerSyncCmd(args []string, stdout, stderr io.Writer) int {
	fs, root := flags("ledger sync", stderr)
	rest, err := parse(fs, args)
	if err != nil {
		return exitUsage
	}
	if len(rest) > 0 {
		fmt.Fprintf(stderr, "kept: ledger sync takes no arguments\n")
		return exitUsage
	}

	c, code := open(*root, stderr)
	if code != exitOK {
		return code
	}
	// Before the act, and for ratify's reason: advancing a ledger already known
	// to be incomplete would build the durable record on a value nobody can
	// account for.
	if code := sound(c, stderr); code != exitOK {
		return code
	}

	what, d, err := ledger.Advance(ledgerStore(c))
	if err != nil {
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return exitUsage
	}

	switch what {
	case ledger.SyncAdvanced:
		fmt.Fprintf(stdout, "%s advanced to %s, %s.\n",
			ledger.Ref, shortCommit(d.Upstream), count(d.Behind, "commit"))
		return exitOK
	case ledger.SyncDiverged:
		writeDiverged(stderr, d)
		return exitRefused
	}

	switch {
	case !d.Known():
		fmt.Fprintf(stdout, "%s holds nothing, so there is nothing to advance onto.\n"+
			"git fetch writes it; kept does not fetch.\n", ledger.UpstreamRef)
	default:
		fmt.Fprintf(stdout, "%s already holds everything %s does.\n", ledger.Ref, ledger.UpstreamRef)
	}
	return exitOK
}

// writeDiverged is exit 4's account of itself.
//
// It is a refusal to answer rather than a refusal to act, which is what puts it
// at 4 beside a rewound ledger rather than at 1 beside drift: there are acts in
// this repository's object store that the fold every verdict is derived from
// cannot see, so the record is known to be incomplete. It is deliberately the
// only place a divergence refuses anything — a tick that could not record what
// it found because two clones had forked would be a worse failure than a fold
// that is missing the other clone's acts, and appending to either side loses
// nothing that a union will not recover. See docs/decision/0054 and 0062.
func writeDiverged(w io.Writer, d ledger.Divergence) {
	fmt.Fprintf(w, "kept: %s and %s have both moved, so advancing would discard one side.\n\n",
		ledger.Ref, ledger.UpstreamRef)
	fmt.Fprintf(w, "  %s      %s, %s the other cannot reach\n", shortCommit(d.Ledger), ledger.Ref, count(d.Ahead, "commit"))
	fmt.Fprintf(w, "  %s      %s, %s the other cannot reach\n\n", shortCommit(d.Upstream), ledger.UpstreamRef, count(d.Behind, "commit"))
	fmt.Fprintf(w, "This verb only fast-forwards. Commit a union of the two tips instead;\n"+
		"a ledger is append-only, so both sides are the answer.\n\n"+
		"    git merge-base %s %s\n\n"+
		"names where they parted. Every other verb still answers.\n", ledger.Ref, ledger.UpstreamRef)
}

// advance fast-forwards the ledger before a verb writes to it, and says so.
//
// Every verb that *writes* calls this and no verb that only reads does. A read
// that advanced could not run against a checkout nobody may write to, which is
// what docs/decision/0001 and 0032 make kept status and kept why answer from.
// A write that did not advance is worse: appending onto a ledger that is behind
// forks it, so a clone that fetched and then ticked would manufacture a
// divergence out of nothing.
//
// It never refuses. A divergence is said on stderr and the verb continues:
// records are append-only, so appending to either side loses nothing a union
// will not recover, and a machine that could not record what it found because
// another clone had also written would be a worse failure than a fold missing
// that clone's acts. kept ledger sync is the one place it is a refusal.
//
// The report goes to stderr because a tick's stdout is a document when --json
// is given, and moving the durable record silently is the failure on the other
// side of moving it at all.
func advance(c *corpus.Corpus, stderr io.Writer) {
	what, d, err := ledger.Advance(ledgerStore(c))
	if err != nil {
		fmt.Fprintf(stderr, "kept: advance %s onto %s: %s\n", ledger.Ref, ledger.UpstreamRef, err)
		return
	}
	switch what {
	case ledger.SyncAdvanced:
		fmt.Fprintf(stderr, "kept: %s advanced to %s, %s the last git fetch brought.\n",
			ledger.Ref, shortCommit(d.Upstream), count(d.Behind, "commit"))
	case ledger.SyncDiverged:
		fmt.Fprintf(stderr, "kept: %s and %s have both moved — %s ahead, %s behind.\n"+
			"        This run appends to the ledger as it is; nothing is lost.\n"+
			"        kept ledger sync says what to do about it.\n",
			ledger.Ref, ledger.UpstreamRef, count(d.Ahead, "commit"), count(d.Behind, "commit"))
	}
}
