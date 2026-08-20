package gather

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/workspace"
)

// declares is a gatherer that reads whatever a test tells it to. It exists so
// that the guard below can be broken deliberately: nothing kept ships declares
// a ledger read, so a check written only against the built-ins would never
// fire.
type declares struct {
	name  string
	reads []ledger.Kind
}

func (d declares) Name() string {
	if d.name == "" {
		return "declares@1"
	}
	return d.name
}
func (d declares) Reads() []ledger.Kind                    { return d.reads }
func (declares) Declares(map[string]any) ([]string, error) { return []string{"ok"}, nil }
func (declares) Gather(context.Context, Request) (Fields, error) {
	return Fields{"ok": true}, nil
}

// counted is an [Acts] that answers a fixed digest and says how often it was
// asked, so a gatherer that declares nothing can be shown never to ask.
type counted struct {
	digest string
	asked  int
}

func (c *counted) Project([]ledger.Kind, []string) string {
	c.asked++
	return c.digest
}

// The whole cost of docs/decision/0046 is that there is none for anybody else.
// A gatherer that declares nothing gets docs/decision/0033's digest byte for
// byte, and if it ever stops doing so every promise in every corpus re-gathers
// for an answer that did not change.
//
// This is the test that would catch a composition made unconditional.
func TestAGathererThatReadsNothingDigestsExactlyTheTree(t *testing.T) {
	tree := treeFor(t)
	acts := &counted{digest: "sha256:" + strings.Repeat("b", 64)}
	requests := Requests{Tree: tree, World: world("one"), Acts: acts}

	for name, g := range Builtin() {
		if len(g.Reads()) != 0 {
			t.Fatalf("%s declares a ledger read, and this test is the reason to think hard about that", name)
		}
		for _, args := range []map[string]any{
			nil,
			{argScope: "internal/**"},
			{argScope: []any{"internal/**", "cmd/**"}},
		} {
			req := requests.For(g, args)
			want := tree.Scope(Globs(args)).String()
			if req.Scope != want {
				t.Errorf("%s over %v digests as %s, and the tree alone digests as %s", name, args, req.Scope, want)
			}
		}
	}

	if acts.asked != 0 {
		t.Errorf("a gatherer that declares nothing asked the ledger %d times", acts.asked)
	}
}

// And a gatherer that does declare one gets a different digest, in both the
// place due-ness reads and the place the record and the cache key read — which
// is one place, which is the point of putting the projection in the scope
// digest rather than in the observation preimage.
func TestADeclaredReadMovesTheScopeDigest(t *testing.T) {
	tree := treeFor(t)
	g := declares{reads: []ledger.Kind{ledger.Ratified}}

	plain := Requests{Tree: tree, World: world("one")}.For(g, nil)
	before := Requests{Tree: tree, World: world("one"),
		Acts: &counted{digest: ledger.Project(nil, []ledger.Kind{ledger.Ratified}, nil)}}.For(g, nil)
	after := Requests{Tree: tree, World: world("one"),
		Acts: &counted{digest: "sha256:" + strings.Repeat("c", 64)}}.For(g, nil)

	if before.Scope == after.Scope {
		t.Error("an act about something in scope did not move the scope digest")
	}
	if before.Scope == tree.Digest() || after.Scope == tree.Digest() {
		t.Error("a declared read digests as the tree alone, so nothing would notice an act")
	}
	// Nil Acts is a caller with nothing to offer rather than an empty
	// projection, and it must not silently answer as though the ledger were
	// empty — it is the tree's digest, and the declaring promise is due.
	if plain.Scope != tree.Digest() {
		t.Errorf("with no acts to project the digest is %s, and the tree is %s", plain.Scope, tree.Digest())
	}

	// The same value drives due-ness, so a recorded digest that is still
	// current reads as unchanged and one that is not reads as due — with no
	// clause added to the tick to ask it.
	if !after.Unchanged(after.Scope) {
		t.Error("a scope compared against its own digest reads as changed")
	}
	if after.Unchanged(before.Scope) {
		t.Error("a scope compared against a stale digest reads as unchanged")
	}
}

// A scope that matched no path is a mis-stated scope and is always due,
// whatever the ledger says. A projection may not rescue globs that name
// nothing.
func TestAProjectionDoesNotRescueAScopeThatMatchedNothing(t *testing.T) {
	tree := treeFor(t)
	g := declares{reads: []ledger.Kind{ledger.Ratified}}
	req := Requests{Tree: tree, World: world("one"),
		Acts: &counted{digest: "sha256:" + strings.Repeat("d", 64)}}.For(g, map[string]any{argScope: "nowhere/**"})

	if req.Scope != workspace.NoMatch {
		t.Errorf("a scope matching nothing digests as %s", req.Scope)
	}
	if !req.MatchedNothing() || req.Unchanged(workspace.NoMatch) {
		t.Error("a scope matching nothing does not read as always due")
	}
}

// The guard docs/decision/0046 turns on: a gatherer may declare a dependency
// only on acts a person performs. Truth and dispositions are written every
// tick, so one declaring those would gather, cause an append, move its own
// digest and be due again.
//
// Broken deliberately, because nothing kept ships declares anything at all.
func TestACatalogueRefusesAReadOfWhatTheTickWrites(t *testing.T) {
	for _, c := range []struct {
		kinds   []ledger.Kind
		refused bool
	}{
		{nil, false},
		{[]ledger.Kind{ledger.Ratified}, false},
		{[]ledger.Kind{ledger.Ratified, ledger.Released}, false},
		{[]ledger.Kind{ledger.Verdict}, true},
		{[]ledger.Kind{ledger.Disposed}, true},
		{[]ledger.Kind{ledger.Ratified, ledger.Verdict}, true},
		{[]ledger.Kind{"something this build has never heard of"}, true},
	} {
		c := c
		cat := Catalogue{"x/reader": declares{name: "x/reader@1", reads: c.kinds}}
		errs := cat.Check(map[string]model.GatherSpec{"reader": {Gatherer: "x/reader"}})
		switch {
		case c.refused && len(errs) == 0:
			t.Errorf("declaring %v is accepted", c.kinds)
		case !c.refused && len(errs) > 0:
			t.Errorf("declaring %v is refused: %v", c.kinds, errs[0])
		case c.refused:
			if !strings.Contains(errs[0].Error(), "a person performs") {
				t.Errorf("declaring %v is refused with %q", c.kinds, errs[0])
			}
		}
	}
}

// A refused declaration is not a gatherer nothing answers to, and the corpus
// tells the two apart by type rather than by reading a message.
func TestARefusedDeclarationIsNotAnUnknownGatherer(t *testing.T) {
	cat := Catalogue{"x/reader": declares{name: "x/reader@1", reads: []ledger.Kind{ledger.Verdict}}}
	errs := cat.Check(map[string]model.GatherSpec{"reader": {Gatherer: "x/reader"}})
	if len(errs) != 1 {
		t.Fatalf("one bad declaration reports %d problems", len(errs))
	}
	bad, ok := errs[0].(*BadSpec)
	if !ok || bad.NoSuchGatherer() {
		t.Errorf("a refused declaration reports as %#v", errs[0])
	}
}

func treeFor(t *testing.T) *workspace.Tree {
	t.Helper()
	root := t.TempDir()
	write(t, filepath.Join(root, "a.go"), "package a\n")
	write(t, filepath.Join(root, "internal", "b.go"), "package b\n")
	write(t, filepath.Join(root, "cmd", "c.go"), "package c\n")

	tree, err := workspace.Read(root, workspace.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return tree
}
