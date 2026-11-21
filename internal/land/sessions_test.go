package land

import (
	"context"
	oexec "os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/ledger"
)

// A land reads the anchor out of the thread's receipt, and the receipt is on
// refs/kept/sessions. The ledger the land appends its act to holds no session
// record before or after, so the receipt the land read can only have come from
// the sessions ref. See docs/decision/0089.
func TestALandReadsItsAnchorFromTheSessionsRef(t *testing.T) {
	w := newLandWorkspaceOnRefs(t)
	id := w.anchor("promise:a-is-whole", func(box string) {
		writeFile(t, filepath.Join(box, "sub/a/note/second.md"), noteFile("second"))
	})
	// The receipt is asked for by the path every reader names it with, never
	// by where in the sessions ref's tree the store files it: that is the
	// store's decision (docs/decision/0097). The ledger holds no threads/ path,
	// which the end of this test checks, so a receipt that reads here was read
	// from the sessions ref.
	where, err := ledger.ThreadPath(string(id))
	if err != nil {
		t.Fatal(err)
	}
	if lines, err := w.store.Read(where); err != nil || len(lines) == 0 {
		t.Fatalf("the anchored thread's receipt is not on %s: %q, %v", ledger.SessionsRef, lines, err)
	}

	res, err := Perform(context.Background(), w.request(id))
	if err != nil {
		t.Fatalf("land: %v", err)
	}
	if res.Status != Landed {
		t.Fatalf("status = %q (%s: %s), want %q", res.Status, res.Condition, res.Detail, Landed)
	}
	acts, _, err := ledger.ReadActs(w.store)
	if err != nil {
		t.Fatal(err)
	}
	if len(acts) == 0 || acts[len(acts)-1].Kind != ledger.Landed {
		t.Fatal("the land recorded no landed act on the ledger")
	}
	tree, err := gitIn(w.root, "ls-tree", "-r", "--name-only", ledger.Ref)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range strings.Fields(tree) {
		if strings.Contains(p, "threads/") || strings.Contains(p, "artifacts/") {
			t.Fatalf("the ledger holds %s after a land, which a push would publish", p)
		}
	}
}

// A clone holds the ledger and no sessions ref: what a land reads is not there,
// and the land says so in the words it already uses.
func TestALandOfAThreadWhoseReceiptIsNotHereSaysItAnchoredNothing(t *testing.T) {
	w := newLandWorkspaceOnRefs(t)
	id := w.anchor("promise:a-is-whole", func(box string) {
		writeFile(t, filepath.Join(box, "sub/a/note/second.md"), noteFile("second"))
	})
	if _, err := gitIn(w.root, "update-ref", "-d", ledger.SessionsRef); err != nil {
		t.Fatal(err)
	}
	if _, err := Perform(context.Background(), w.request(id)); err == nil {
		t.Fatal("a thread whose receipt is not here was landed")
	} else if !strings.Contains(err.Error(), "thread "+string(id)+" anchored no commit") {
		t.Fatalf("the refusal is not the one a land already gives: %v", err)
	}
}

// gitIn is gitAt for a question whose failure is an answer.
func gitIn(dir string, args ...string) (string, error) {
	out, err := oexec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	return string(out), err
}
