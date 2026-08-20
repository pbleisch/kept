package gather

import (
	"path/filepath"
	"testing"

	"github.com/pbleisch/kept/internal/workspace"
)

// A scope is a projection of the args rather than a field beside them, and this
// is the one place it is read out of them. See docs/decision/0034.
func TestAScopeIsReadOutOfTheArgs(t *testing.T) {
	for _, c := range []struct {
		args map[string]any
		want []string
	}{
		{nil, []string{Everything}},
		{map[string]any{}, []string{Everything}},
		{map[string]any{argScope: ""}, []string{Everything}},
		{map[string]any{argScope: "internal/**"}, []string{"internal/**"}},
		{map[string]any{argScope: []any{"internal/**", "cmd/**"}}, []string{"internal/**", "cmd/**"}},
		{map[string]any{argScope: []any{"internal/**", 7, ""}}, []string{"internal/**"}},
	} {
		if got := Globs(c.args); !same(got, c.want) {
			t.Errorf("%#v resolves to %q, want %q", c.args, got, c.want)
		}
	}
}

// A promise that named no scope did not mis-state one: it depends on the whole
// workspace, so its digest is the tree's rather than the "matched nothing"
// docs/decision/0033 makes always due. See docs/decision/0038.
func TestARequestWithNoScopeCoversTheWholeWorkspace(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "a.go"), "package a\n")
	write(t, filepath.Join(root, "internal", "b.go"), "package b\n")

	tree, err := workspace.Read(root, workspace.Options{})
	if err != nil {
		t.Fatal(err)
	}

	requests := Requests{Tree: tree, World: world("one")}

	whole := requests.For(command{}, nil)
	if whole.Scope == workspace.NoMatch {
		t.Fatal("a promise with no scope reads as one that matched nothing, which is always due")
	}
	if whole.Scope != tree.Digest() {
		t.Errorf("the scope digest is %s and the tree is %s", whole.Scope, tree.Digest())
	}
	if len(whole.Paths) != 2 {
		t.Errorf("the whole workspace resolved to %q", whole.Paths)
	}

	// And a narrower one is narrower, resolved from the same walk.
	part := requests.For(command{}, map[string]any{argScope: "internal/**"})
	if !same(part.Paths, []string{"internal/b.go"}) {
		t.Errorf("internal/** resolved to %q", part.Paths)
	}
	if part.Scope == whole.Scope {
		t.Error("a scope over one file digests the same as the whole workspace")
	}

	// A scope that matches nothing is a distinct outcome and not a digest over
	// an empty set, which is what keeps a mis-typed one from reading as
	// "nothing has changed" forever.
	none := requests.For(command{}, map[string]any{argScope: "docs/**"})
	if none.Scope != workspace.NoMatch {
		t.Errorf("a scope matching nothing digests as %s", none.Scope)
	}

	// The request carries what the caller has to pass on to the record.
	if whole.Root != root || whole.World.Tree == "" || whole.Deadline != 0 {
		t.Errorf("the request is %#v", whole)
	}
}
