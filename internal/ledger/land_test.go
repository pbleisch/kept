package ledger

import (
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/run"
)

// TestALandActReadsBackAsAnAct: adding a kind to the act stream is adding a kind
// a reader has to know, and a build that did not would quarantine every land as
// a record it could not identify.
func TestALandActReadsBackAsAnAct(t *testing.T) {
	store := NewFiles(t.TempDir())
	l := Open(store, run.Start("test", nil))
	where := Where{World: model.World{Tree: "t", Commit: strings.Repeat("a", 40), Ref: "refs/heads/main"}}
	if _, err := l.Land("promise:x", "sha256:"+strings.Repeat("b", 64), 1, "Test <t@example.invalid>", "note", where); err != nil {
		t.Fatal(err)
	}
	if err := l.Close(run.Clean); err != nil {
		t.Fatal(err)
	}

	acts, quarantined, err := ReadActs(store)
	if err != nil {
		t.Fatal(err)
	}
	if len(quarantined) != 0 {
		t.Fatalf("a land act was quarantined: %+v", quarantined)
	}
	if len(acts) != 1 || acts[0].Kind != Landed {
		t.Fatalf("read back %d acts: %+v", len(acts), acts)
	}
	if acts[0].World.Commit != where.World.Commit || acts[0].World.Ref != where.World.Ref {
		t.Fatalf("the world did not survive the round trip: %+v", acts[0].Where)
	}
}

// TestALandWithNoCommitIsQuarantined breaks the guard: an act's world is
// advisory on the acts that create standing, where it is provenance. On a land
// it is the substance — a land record naming no commit does not say what
// landed, which is the one thing it exists to say.
func TestALandWithNoCommitIsQuarantined(t *testing.T) {
	if why := actWhy(Act{Envelope: Envelope{Kind: Landed}, Artifact: "promise:x"}); why == "" {
		t.Fatal("a land act naming no commit was read as an act")
	}
	// The same absence on a ratification is fine, because there it is advisory
	// and every act written before docs/decision/0063 is one.
	if why := actWhy(Act{
		Envelope: Envelope{Kind: Ratified}, Artifact: "promise:x",
		Hash: "sha256:" + strings.Repeat("b", 64), Norm: 1,
	}); why != "" {
		t.Fatalf("a ratification with no world was quarantined: %s", why)
	}
}

// TestAThreadRefIsUnderKeptAndNamesItsThread: the anchor's ref may not be a
// branch, and it may not be anywhere a fetch would write.
func TestAThreadRefIsUnderKeptAndNamesItsThread(t *testing.T) {
	ref, err := ThreadRef("abc123")
	if err != nil {
		t.Fatal(err)
	}
	if ref != "refs/kept/threads/abc123" {
		t.Fatalf("ThreadRef = %q", ref)
	}
	if strings.HasPrefix(ref, "refs/heads/") || strings.HasPrefix(ref, "refs/remotes/") {
		t.Fatalf("%s is a branch; an unlanded repair may not be on one", ref)
	}
	// Deliberately outside the ledger's own fetch pattern: an anchor is local
	// work nobody has reviewed, and a fetch that collected other people's would
	// be collecting exactly that.
	if strings.HasPrefix(ref, strings.TrimSuffix(strings.SplitN(FetchRefspec, ":", 2)[0], "*")[1:]) {
		t.Fatalf("%s is inside the ledger's fetch refspec %s", ref, FetchRefspec)
	}
	for _, bad := range []string{"", "a/b", `a\b`, "a\x00b"} {
		if _, err := ThreadRef(bad); err == nil {
			t.Fatalf("ThreadRef accepted %q", bad)
		}
	}
}

// TestAnchoredThreadsAnswersEmptyOutsideARepository: a manuscript has no
// worktree to repair in, so it has nothing to land, and that is an empty answer
// rather than a failure.
func TestAnchoredThreadsAnswersEmptyOutsideARepository(t *testing.T) {
	got, err := AnchoredThreads(t.TempDir())
	if err != nil {
		t.Fatalf("AnchoredThreads outside a repository: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("AnchoredThreads found %v outside a repository", got)
	}
}
