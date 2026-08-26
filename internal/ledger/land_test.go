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
	landed := Landing{Target: "origin/main", Regime: SharedRegime, Arrival: RefArrival}
	if _, err := l.Land("promise:x", "sha256:"+strings.Repeat("b", 64), 1, "Test <t@example.invalid>", "note", where, landed); err != nil {
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
	if acts[0].Landing != landed {
		t.Fatalf("the landing did not survive the round trip: %+v", acts[0].Landing)
	}
}

// TestALandWithNoCommitIsQuarantined breaks the guard: an act's world is
// advisory on the acts that create standing, where it is provenance. On a ref
// arrival it is the substance — a land record naming no commit does not say
// what landed, which is the one thing it exists to say.
func TestALandWithNoCommitIsQuarantined(t *testing.T) {
	if why := actWhy(Act{
		Envelope: Envelope{Kind: Landed}, Artifact: "promise:x",
		Landing: Landing{Target: "origin/main", Regime: SharedRegime, Arrival: RefArrival},
	}); why == "" {
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

// landingCase is one landed act and what must be wrong with it. The subject is
// a rule rather than a call, so every case is run at both ends: through
// [Ledger.Land], which must refuse before anything is appended, and through
// actWhy, which must quarantine the same line if it arrives from an editor, a
// merge or an older build.
type landingCase struct {
	name    string
	where   Where
	landing Landing
	// why is a fragment of the refusal, so a case cannot pass by being refused
	// for some other reason entirely.
	why string
}

// The three fields, each missing on its own, and the combinations the rules
// forbid. docs/decision/0015: none of the three has a default, `regime: shared`
// implies `arrival: ref`, and Uncommitted is false on every ref arrival and
// true on every worktree one.
//
// A malformed act is permanent — the ledger appends and never revises — so the
// write is the only moment any of this can be prevented. That is why the table
// runs against Land rather than only against the fold.
var landingCases = []landingCase{
	{"no target", cleanLand, Landing{Regime: SharedRegime, Arrival: RefArrival}, "names no target"},
	{"no regime", cleanLand, Landing{Target: "origin/main", Arrival: RefArrival}, "names no check regime"},
	{"no arrival", cleanLand, Landing{Target: "origin/main", Regime: SharedRegime}, "names no arrival"},
	{"nothing at all", cleanLand, Landing{}, "names no target"},
	{"a regime this build does not know", cleanLand,
		Landing{Target: "origin/main", Regime: "trusted", Arrival: RefArrival}, "not a check regime"},
	{"an arrival this build does not know", cleanLand,
		Landing{Target: "origin/main", Regime: SharedRegime, Arrival: "email"}, "not an arrival"},
	// The obvious forbidden pair: the strongest check and the weakest arrival
	// never combine, because a worktree arrival's target is always the branch
	// this working tree has checked out.
	{"shared with a worktree arrival", dirtyLand,
		Landing{Target: "origin/main", Regime: SharedRegime, Arrival: WorktreeArrival}, "shared regime"},
	{"a ref arrival from a tree with uncommitted changes", dirtyLand,
		Landing{Target: "origin/main", Regime: SharedRegime, Arrival: RefArrival}, "always gated in a clean tree"},
	{"a worktree arrival that claims a clean tree", cleanLand,
		Landing{Target: "main", Regime: LocalRegime, Arrival: WorktreeArrival}, "says the tree was clean"},
	{"a ref arrival with no commit", Where{World: model.World{Tree: "t"}},
		Landing{Target: "origin/main", Regime: SharedRegime, Arrival: RefArrival}, "does not say what landed"},
}

var (
	cleanLand = Where{World: model.World{Tree: "t", Commit: strings.Repeat("a", 40), Ref: "refs/heads/main"}}
	dirtyLand = Where{World: model.World{Tree: "t", Commit: strings.Repeat("a", 40)}, Uncommitted: true}
)

// The refusal at the write, which is the half that matters: the ledger is
// append-only, so an act that reaches it is permanent and a check on the read
// path alone would only ever describe damage.
func TestALandMissingWhatItMustSayIsNotWritten(t *testing.T) {
	for _, c := range landingCases {
		t.Run(c.name, func(t *testing.T) {
			store := NewFiles(t.TempDir())
			l := Open(store, run.Start("test", nil))
			_, err := l.Land("promise:x", "sha256:"+strings.Repeat("b", 64), 1,
				"Test <t@example.invalid>", "note", c.where, c.landing)
			if err == nil {
				t.Fatalf("Land accepted %s", c.name)
			}
			if !strings.Contains(err.Error(), c.why) {
				t.Fatalf("Land refused %s with %q, want it to say %q", c.name, err, c.why)
			}
			// And nothing was appended. A refusal that had already written the
			// line would be the failure this exists to prevent, spelled with a
			// message on top.
			if acts, _, err := ReadActs(store); err != nil || len(acts) != 0 {
				t.Fatalf("the refused land left %d acts (%v)", len(acts), err)
			}
		})
	}
}

// The same table on the read path. An act is JSON in a git object anybody's
// editor can reach, and a line an older build wrote carries no landing at all —
// so the fold has to make the same judgment the write made, from the same
// function, or the two will drift.
func TestALandMissingWhatItMustSayIsQuarantined(t *testing.T) {
	for _, c := range landingCases {
		t.Run(c.name, func(t *testing.T) {
			why := actWhy(Act{
				Envelope: Envelope{Kind: Landed}, Where: c.where,
				Artifact: "promise:x", Landing: c.landing,
			})
			if why == "" {
				t.Fatalf("the fold read %s as an act", c.name)
			}
			if !strings.Contains(why, c.why) {
				t.Fatalf("the fold quarantined %s with %q, want it to say %q", c.name, why, c.why)
			}
		})
	}
}

// The guard on the guard: every shape the rules do admit is written and read
// back. A refusal that refused everything would pass every case above.
func TestTheLandingsTheRulesAdmitAreWritten(t *testing.T) {
	for _, c := range []landingCase{
		{"shared, onto a ref", cleanLand,
			Landing{Target: "origin/main", Regime: SharedRegime, Arrival: RefArrival}, ""},
		{"local, onto a ref", cleanLand,
			Landing{Target: "main", Regime: LocalRegime, Arrival: RefArrival}, ""},
		{"local, into a working tree", dirtyLand,
			Landing{Target: "main", Regime: LocalRegime, Arrival: WorktreeArrival}, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			store := NewFiles(t.TempDir())
			l := Open(store, run.Start("test", nil))
			if _, err := l.Land("promise:x", "sha256:"+strings.Repeat("b", 64), 1,
				"Test <t@example.invalid>", "note", c.where, c.landing); err != nil {
				t.Fatalf("Land refused %s: %v", c.name, err)
			}
			if err := l.Close(run.Clean); err != nil {
				t.Fatal(err)
			}
			acts, quarantined, err := ReadActs(store)
			if err != nil || len(quarantined) != 0 || len(acts) != 1 {
				t.Fatalf("%s read back as %d acts and %d quarantined (%v)", c.name, len(acts), len(quarantined), err)
			}
			if acts[0].Landing != c.landing {
				t.Fatalf("%s read back as %+v", c.name, acts[0].Landing)
			}
		})
	}
}

// A landing belongs to a land and to nothing else. A ratification that named a
// target would be answering a question nobody asked of it, and the two
// constructors that create standing take no landing at all — so this is about a
// line that arrived from somewhere else.
func TestOnlyALandCarriesALanding(t *testing.T) {
	for _, kind := range []Kind{Ratified, Released} {
		for _, c := range []struct {
			name    string
			landing Landing
			why     string
		}{
			{"a target", Landing{Target: "origin/main"}, "names a landing target"},
			{"a regime", Landing{Regime: SharedRegime}, "names a check regime"},
			{"an arrival", Landing{Arrival: RefArrival}, "names an arrival"},
		} {
			t.Run(string(kind)+" with "+c.name, func(t *testing.T) {
				why := actWhy(Act{
					Envelope: Envelope{Kind: kind}, Artifact: "promise:x",
					Hash: "sha256:" + strings.Repeat("b", 64), Norm: 1, Landing: c.landing,
				})
				if why == "" {
					t.Fatalf("a %s act carrying %s was read as an act", kind, c.name)
				}
				if !strings.Contains(why, c.why) {
					t.Fatalf("quarantined with %q, want it to say %q", why, c.why)
				}
			})
		}
	}

	// And the constructors cannot produce one: they take no landing, so this is
	// a property of the shape rather than of a caller's care.
	store := NewFiles(t.TempDir())
	l := Open(store, run.Start("test", nil))
	hash := "sha256:" + strings.Repeat("b", 64)
	for _, act := range []func() (Act, error){
		func() (Act, error) { return l.Ratify("promise:x", hash, 1, "T <t@example.invalid>", "", cleanLand) },
		func() (Act, error) { return l.Release("promise:x", hash, 1, "T <t@example.invalid>", "", cleanLand) },
	} {
		got, err := act()
		if err != nil {
			t.Fatal(err)
		}
		if got.Landing != (Landing{}) {
			t.Fatalf("a %s act was written carrying %+v", got.Kind, got.Landing)
		}
	}
}
