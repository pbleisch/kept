package repair

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/assess"
	"github.com/pbleisch/kept/internal/ledger"
)

// refTree is every path a ref's tree holds in the workspace's repository.
func refTree(t *testing.T, root, ref string) []string {
	t.Helper()
	out, err := exec.Command("git", "-C", root, "ls-tree", "-r", "--name-only", ref).Output()
	if err != nil {
		t.Fatalf("ls-tree %s: %v", ref, err)
	}
	return strings.Fields(string(out))
}

// A whole episode on a ledger kept on refs, as every Git workspace keeps it: a
// repair, a pass that rejects it, a rework cut at the first attempt's anchor
// and carrying the argument, and a pass that accepts. Every step after the
// first reads what an earlier step wrote — the rework reads the packet and the
// argument back out of the receipt — so an episode that ends ready has read
// and written session records. Afterwards every one of them is on
// refs/kept/sessions and none is on refs/kept/ledger, which is the ref a clone
// fetches and a person publishes. Then kept review's own read of the thread,
// from the same store. See docs/decision/0089.
func TestARepairFilesItsSessionOnTheSessionsRefAndTheLedgerCarriesNone(t *testing.T) {
	e := newEpisodeOnRefs(t, "-X", "main.rulings=reject,accept")
	ep, err := Run(context.Background(), e.req, bounds(3, true), "")
	if err != nil {
		t.Fatal(err)
	}
	if ep.End != Ready || len(ep.Attempts) != 2 {
		t.Fatalf("the episode ended %s after %d attempts: %v", ep.End, len(ep.Attempts), ep.Why)
	}

	for _, p := range refTree(t, e.w.root, ledger.Ref) {
		if strings.Contains(p, "threads/") || strings.Contains(p, "artifacts/") {
			t.Fatalf("the ledger holds %s after a repair, which a push would publish", p)
		}
	}
	// The receipt is asked for by the path every reader names it with, never
	// by where in the sessions ref's tree the store files it: that is the
	// store's decision (docs/decision/0097). The ledger holds no threads/ path,
	// so a receipt that reads here was read from the sessions ref, and the
	// clone at the end of this test reads none once that ref is gone.
	receipt, err := ledger.ThreadPath(string(ep.Thread))
	if err != nil {
		t.Fatal(err)
	}
	if lines, err := e.w.store.Read(receipt); err != nil || len(lines) == 0 {
		t.Fatalf("the sessions ref holds no receipt for thread %s: %q, %v", ep.Thread, lines, err)
	}
	held := strings.Join(refTree(t, e.w.root, ledger.SessionsRef), "\n")
	for _, want := range []string{
		"artifacts/" + string(ledger.ThreadPacketArtifact) + "/",
		"artifacts/" + string(ledger.ThreadBriefArtifact) + "/",
		"artifacts/" + string(ledger.ThreadArgumentArtifact) + "/",
		"artifacts/" + string(ledger.SessionTranscriptArtifact) + "/",
	} {
		if !strings.Contains(held, want) {
			t.Errorf("the sessions ref holds no %s:\n%s", want, held)
		}
	}
	// What a repair says about itself in public is still on the ledger: the
	// disposition that names the thread.
	acted := false
	for _, d := range dispositionsNaming(t, e.w.store, "promise:counted") {
		acted = acted || (d.Outcome == ledger.OutcomeAct && d.Thread == string(ep.Thread))
	}
	if !acted {
		t.Fatal("no act disposition on the ledger names the thread")
	}

	entry, err := assess.Read(context.Background(), e.w.store, e.w.root, ep.Thread, time.Minute)
	if err != nil {
		t.Fatalf("review could not read the thread from the sessions ref: %v", err)
	}
	if !entry.Delivered {
		t.Fatal("review read the thread and found no packet")
	}

	// A clone has the ledger and not the sessions ref. Review then says the
	// thread has no receipt here, which is the refusal it already gives.
	if out, err := exec.Command("git", "-C", e.w.root, "update-ref", "-d", ledger.SessionsRef).CombinedOutput(); err != nil {
		t.Fatalf("update-ref -d: %v\n%s", err, out)
	}
	if _, err := assess.Read(context.Background(), e.w.store, e.w.root, ep.Thread, time.Minute); err == nil {
		t.Fatal("review read a thread whose receipt is not here")
	} else if !strings.Contains(err.Error(), "no receipt") {
		t.Fatalf("the refusal is not the one review already gives: %v", err)
	} else if errors.Is(err, ledger.ErrNoStream) {
		t.Fatalf("the refusal leaks the store's error rather than naming the thread: %v", err)
	}
}
