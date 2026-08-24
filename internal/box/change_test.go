package box

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// The measurement decision 0059 rests on, stated as a test: a repair scope is a
// glob and a confinement grant is a path, so a rendered profile is always at
// least as wide as the bound it stands for, and the difference has to be found
// in the diff.
//
// The three cases here are the three a `+++ b/` reading of the diff text gets
// wrong or right by accident. A create outside the glob is the one a reader
// might notice; a *delete* outside the glob is the one they cannot, because a
// delete's diff header names /dev/null and the path never appears after `+++`.
func TestAChangeReportsEveryPathTheScopeDoesNotCover(t *testing.T) {
	repository, pinned := boxRepository(t, []boxFile{
		{"internal/scratch.go", "package scratch\n", 0o644},
		{"internal/README.md", "authored, and not a Go file\n", 0o644},
		{"docs/note.md", "authored\n", 0o644},
	})
	b, err := Create(context.Background(), Request{Repository: repository, Commit: pinned})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close(context.Background()) })

	// What a repair did: one edit the promise authorized, one file it created
	// that the glob does not cover, and one file it deleted that the glob does
	// not cover either.
	write(t, b, "internal/scratch.go", "package scratch\n\n// repaired\n")
	write(t, b, "internal/notes.txt", "a note nobody asked for\n")
	remove(t, b, "internal/README.md")

	change, err := b.Change(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"internal/README.md", "internal/notes.txt", "internal/scratch.go"}
	if !reflect.DeepEqual(change.Paths, want) {
		t.Fatalf("changed paths = %v, want %v", change.Paths, want)
	}
	// The delete is in the list and nowhere in the diff's `+++` lines, which is
	// why Paths is read from git rather than parsed out of this text.
	if strings.Contains(diffTargets(string(change.Diff)), "internal/README.md") {
		t.Fatal("the deleted path appears after +++ in the diff, so this test no longer distinguishes the two readings")
	}

	review := change.Against([]string{"internal/*.go"})
	if review.Bounded() {
		t.Fatalf("a repair that created and deleted files outside its scope reviewed as bounded:\n%s", review)
	}
	if got := review.InScope; !reflect.DeepEqual(got, []string{"internal/scratch.go"}) {
		t.Errorf("in scope = %v, want the one file the glob names", got)
	}
	if got := review.Outside; !reflect.DeepEqual(got, []string{"internal/README.md", "internal/notes.txt"}) {
		t.Errorf("outside the scope = %v, want the created and the deleted path", got)
	}
	for _, want := range []string{"3 paths changed, 2 outside", "internal/*.go", "internal/notes.txt", "internal/README.md"} {
		if !strings.Contains(review.String(), want) {
			t.Errorf("the report a person reads is missing %q:\n%s", want, review)
		}
	}

	// The guard broken on purpose: the same change under a scope that does
	// cover those paths is bounded. A review that answered `not bounded` for
	// every repair would pass every assertion above and check nothing.
	if wide := change.Against([]string{"internal/**"}); !wide.Bounded() {
		t.Fatalf("a scope that covers every changed path still reviewed as unbounded:\n%s", wide)
	}
}

// Fail-closed at the edge that has no author: a promise that bound nothing
// authorizes no repair, rather than authorizing all of one.
func TestAScopeThatNamesNothingCoversNothing(t *testing.T) {
	change := Change{Paths: []string{"internal/scratch.go"}}
	for _, scope := range [][]string{nil, {}} {
		review := change.Against(scope)
		if review.Bounded() {
			t.Fatalf("a change reviewed as bounded against scope %#v", scope)
		}
		if !slices.Equal(review.Outside, []string{"internal/scratch.go"}) {
			t.Fatalf("outside = %v, want every path", review.Outside)
		}
	}
	if empty := (Change{}).Against(nil); !empty.Bounded() {
		t.Fatal("a repair that changed nothing did not review as bounded")
	}
}

func write(t *testing.T, b *Box, name, data string) {
	t.Helper()
	path := filepath.Join(b.Path(), filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func remove(t *testing.T, b *Box, name string) {
	t.Helper()
	if err := os.Remove(filepath.Join(b.Path(), filepath.FromSlash(name))); err != nil {
		t.Fatal(err)
	}
}

// diffTargets is the reading this package deliberately does not use: the paths
// a diff names after `+++ b/`. It exists so a test can assert what it misses.
func diffTargets(diff string) string {
	var out []string
	for _, line := range strings.Split(diff, "\n") {
		if rest, found := strings.CutPrefix(line, "+++ b/"); found {
			out = append(out, strings.TrimSpace(rest))
		}
	}
	return strings.Join(out, "\n")
}
