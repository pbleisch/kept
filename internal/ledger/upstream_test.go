package ledger

import (
	"strings"
	"testing"
)

// Divergence is what docs/decision/0016 said could not be answered offline —
// "the only way to know is to ask the remote". It can, now that there are two
// refs, and every state is measured against a real fetch rather than against
// refs a test moved by hand.
func TestDivergenceIsAnsweredWithNoNetwork(t *testing.T) {
	_, work := fetchLab(t)
	store := NewRefStore(work)
	if _, _, err := ConfigureFetch(work); err != nil {
		t.Fatal(err)
	}

	// Nobody has fetched: there is nothing to compare against, and that is
	// neither agreement nor divergence.
	switch d, err := Compare(store); {
	case err != nil:
		t.Fatal(err)
	case d.Known(), d.Diverged(), d.Advanceable():
		t.Fatalf("an unfetched repository reported %+v", d)
	}

	appendAct(t, store, `{"act":"shared"}`)
	shared := ledgerAt(t, work)
	fetchGit(t, work, "push", "-q", "origin", Ref+":"+Ref)
	if code, out := fetchOrigin(t, work); code != 0 {
		t.Fatalf("fetch exited %d:\n%s", code, out)
	}

	// Level.
	switch d, err := Compare(store); {
	case err != nil:
		t.Fatal(err)
	case !d.Known() || d.Ahead != 0 || d.Behind != 0 || d.Diverged() || d.Advanceable():
		t.Fatalf("a level ledger reported %+v", d)
	}

	// Ahead: kept appended and nobody pushed. This is the resting state.
	appendAct(t, store, `{"act":"ours"}`)
	ahead := ledgerAt(t, work)
	switch d, err := Compare(store); {
	case err != nil:
		t.Fatal(err)
	case d.Ahead != 1 || d.Behind != 0 || d.Diverged() || d.Advanceable():
		t.Fatalf("a ledger one commit ahead reported %+v", d)
	}

	// Behind: the remote moved and this repository did not.
	fetchGit(t, work, "push", "-q", "origin", Ref+":"+Ref)
	fetchGit(t, work, "update-ref", Ref, shared)
	if code, out := fetchOrigin(t, work); code != 0 {
		t.Fatalf("fetch exited %d:\n%s", code, out)
	}
	switch d, err := Compare(store); {
	case err != nil:
		t.Fatal(err)
	case d.Ahead != 0 || d.Behind != 1 || d.Diverged() || !d.Advanceable():
		t.Fatalf("a ledger one commit behind reported %+v", d)
	}

	// Absent: a clone that fetched and never wrote. Everything upstream is
	// behind, and the counts have to say how much rather than nothing.
	fetchGit(t, work, "update-ref", "-d", Ref)
	switch d, err := Compare(store); {
	case err != nil:
		t.Fatal(err)
	case d.Ledger != "" || d.Behind != 2 || !d.Advanceable():
		t.Fatalf("an absent ledger reported %+v", d)
	}

	// Forked: both sides appended.
	fetchGit(t, work, "update-ref", Ref, shared)
	appendAct(t, store, `{"act":"theirs"}`)
	if got := ledgerAt(t, work); got == ahead {
		t.Fatal("the two sides are the same commit, so nothing has forked")
	}
	switch d, err := Compare(store); {
	case err != nil:
		t.Fatal(err)
	case d.Ahead != 1 || d.Behind != 1 || !d.Diverged() || d.Advanceable():
		t.Fatalf("a forked ledger reported %+v", d)
	}
}

// Advance moves the ledger and never merges. Each of the three answers is
// asserted against what the ref holds afterwards, because a verb that reported
// the right word and wrote the wrong ref would pass a test on the word alone.
func TestAdvanceFastForwardsAndRefusesAFork(t *testing.T) {
	_, work := fetchLab(t)
	store := NewRefStore(work)
	if _, _, err := ConfigureFetch(work); err != nil {
		t.Fatal(err)
	}

	// Nothing to advance onto.
	switch what, _, err := Advance(store); {
	case err != nil:
		t.Fatal(err)
	case what != SyncUnchanged:
		t.Fatalf("Advance with no upstream ref = %q, want %q", what, SyncUnchanged)
	}

	appendAct(t, store, `{"act":"shared"}`)
	shared := ledgerAt(t, work)
	appendAct(t, store, `{"act":"theirs"}`)
	theirs := ledgerAt(t, work)
	fetchGit(t, work, "push", "-q", "origin", Ref+":"+Ref)
	fetchGit(t, work, "update-ref", Ref, shared)
	if code, out := fetchOrigin(t, work); code != 0 {
		t.Fatalf("fetch exited %d:\n%s", code, out)
	}

	// Behind: it moves, and the acts the remote holds become readable.
	switch what, d, err := Advance(store); {
	case err != nil:
		t.Fatal(err)
	case what != SyncAdvanced:
		t.Fatalf("Advance onto an upstream that is ahead = %q, want %q", what, SyncAdvanced)
	case d.Behind != 1:
		t.Fatalf("Advance reported %d behind, want 1", d.Behind)
	}
	if got := ledgerAt(t, work); got != theirs {
		t.Fatalf("%s = %s after Advance, want %s", Ref, got, theirs)
	}
	if acts, err := store.Read("acts.jsonl"); err != nil || !strings.Contains(string(acts), "theirs") {
		t.Fatalf("the advanced ledger does not hold the remote's act: %q (%v)", acts, err)
	}

	// Level: nothing moves, and running it twice is not an error.
	switch what, _, err := Advance(store); {
	case err != nil:
		t.Fatal(err)
	case what != SyncUnchanged:
		t.Fatalf("Advance on a level ledger = %q, want %q", what, SyncUnchanged)
	}

	// Ahead: kept appended, and an upstream behind it is not something to
	// advance onto. Nothing moves and nothing is reported as wrong.
	appendAct(t, store, `{"act":"ours"}`)
	ours := ledgerAt(t, work)
	switch what, _, err := Advance(store); {
	case err != nil:
		t.Fatal(err)
	case what != SyncUnchanged:
		t.Fatalf("Advance on a ledger that is ahead = %q, want %q", what, SyncUnchanged)
	}
	if got := ledgerAt(t, work); got != ours {
		t.Fatalf("Advance moved a ledger that was ahead: %s, want %s", got, ours)
	}

	// Forked: refused, and the ledger is exactly where it was. This is the
	// assertion the whole verb exists for — advancing here would discard
	// everything on this side.
	fetchGit(t, work, "update-ref", Ref, shared)
	appendAct(t, store, `{"act":"ours only"}`)
	forked := ledgerAt(t, work)
	switch what, d, err := Advance(store); {
	case err != nil:
		t.Fatal(err)
	case what != SyncDiverged:
		t.Fatalf("Advance on a forked ledger = %q, want %q", what, SyncDiverged)
	case d.Ahead != 1 || d.Behind != 1:
		t.Fatalf("Advance reported %+v, want one commit each way", d)
	}
	if got := ledgerAt(t, work); got != forked {
		t.Fatalf("Advance moved a forked ledger to %s, want %s", got, forked)
	}
	if acts, err := store.Read("acts.jsonl"); err != nil || !strings.Contains(string(acts), "ours only") {
		t.Fatalf("the refused advance lost this side's acts: %q (%v)", acts, err)
	}
}

// The break for the guard: if Advance ever stopped checking ancestry and simply
// wrote the upstream value, this is the record set that would vanish. It is the
// same shape as the 2026-08-22 loss, one layer up — so it is asserted rather
// than assumed, by reading the acts back out of the ref after the refusal.
func TestAnAdvanceThatIgnoredAncestryWouldDiscardThisSidesActs(t *testing.T) {
	_, work := fetchLab(t)
	store := NewRefStore(work)
	if _, _, err := ConfigureFetch(work); err != nil {
		t.Fatal(err)
	}
	appendAct(t, store, `{"act":"shared"}`)
	shared := ledgerAt(t, work)
	appendAct(t, store, `{"act":"theirs"}`)
	fetchGit(t, work, "push", "-q", "origin", Ref+":"+Ref)
	fetchGit(t, work, "update-ref", Ref, shared)
	if code, out := fetchOrigin(t, work); code != 0 {
		t.Fatalf("fetch exited %d:\n%s", code, out)
	}
	for _, line := range []string{`{"act":"ratified decision:0017"}`, `{"act":"ratified decision:0052"}`} {
		appendAct(t, store, line)
	}

	if what, _, err := Advance(store); err != nil || what != SyncDiverged {
		t.Fatalf("Advance = %q, %v; want %q", what, err, SyncDiverged)
	}
	acts, err := store.Read("acts.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"0017", "0052"} {
		if !strings.Contains(string(acts), want) {
			t.Fatalf("the refused advance lost %s:\n%s", want, acts)
		}
	}

	// And what a write of the upstream value would have cost, measured rather
	// than argued: the same read, after the ref is moved the way an
	// ancestry-blind advance would move it.
	upstream := refAt(t, work, UpstreamRef)
	fetchGit(t, work, "update-ref", Ref, upstream)
	after, err := store.Read("acts.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(after), "0017") || strings.Contains(string(after), "0052") {
		t.Fatalf("writing the upstream value kept this side's acts, so the refusal guards nothing:\n%s", after)
	}
}

// A ledger in files has no refs, so a manuscript outside Git answers about a
// divergence it cannot have rather than failing.
func TestAFileLedgerHasNoUpstreamAndSaysSo(t *testing.T) {
	store := NewFiles(t.TempDir())
	switch d, err := Compare(store); {
	case err != nil:
		t.Fatal(err)
	case d.Known():
		t.Fatalf("a file ledger reported an upstream value: %+v", d)
	}
	switch what, _, err := Advance(store); {
	case err != nil:
		t.Fatal(err)
	case what != SyncUnchanged:
		t.Fatalf("Advance on a file ledger = %q, want %q", what, SyncUnchanged)
	}
}
