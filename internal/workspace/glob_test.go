package workspace

import "testing"

// The failure that made kept define this itself: git rev-parse 'HEAD:internal/**'
// echoes its argument and exits zero, and filepath.Match returns false for the
// path any author means. Both are silent, so both leave a globbed promise
// permanently not due. See docs/decision/0033.
func TestWhatAGlobMeans(t *testing.T) {
	for _, tc := range []struct {
		pattern string
		path    string
		want    bool
	}{
		{"internal/**", "internal/corpus/corpus.go", true},
		{"internal/**", "internal/corpus.go", true},
		{"internal/**", "cmd/kept/main.go", false},
		{"internal/**", "internals/x.go", false},

		// ** matches no segments at all, which is what makes a/**/b mean
		// "b under a, however deep, including directly".
		{"docs/**/*.md", "docs/model.md", true},
		{"docs/**/*.md", "docs/decision/0033.md", true},
		{"docs/**/*.md", "docs/a/b/c.md", true},
		{"docs/**/*.md", "docs/a/b/c.txt", false},

		// * stays inside one segment. This is the half git's default
		// pathspecs get wrong, and getting it wrong widens every scope.
		{"*.go", "main.go", true},
		{"*.go", "cmd/kept/main.go", false},
		{"internal/*/doc.go", "internal/corpus/doc.go", true},
		{"internal/*/doc.go", "internal/corpus/deep/doc.go", false},

		// The form scopes are ordinarily authored in.
		{"internal/", "internal/corpus/corpus.go", true},
		{"internal/", "cmd/main.go", false},

		{"go.mod", "go.mod", true},
		{"go.mod", "internal/go.mod", false},

		// Not a metacharacter here, and deliberately so: a scope meaning it
		// as one matches nothing, which reads as always due rather than as
		// silently narrower than its author thought.
		{"main.?o", "main.go", false},
		{"main.go", "main.go", true},
	} {
		if got := Match([]string{tc.pattern}, tc.path); got != tc.want {
			t.Errorf("Match(%q, %q) = %v, want %v", tc.pattern, tc.path, got, tc.want)
		}
	}
}

func TestAScopeIsAnyOfItsGlobs(t *testing.T) {
	scope := []string{"internal/**", "go.mod"}
	for _, path := range []string{"internal/ledger/ledger.go", "go.mod"} {
		if !Match(scope, path) {
			t.Errorf("%q is in the scope and did not match", path)
		}
	}
	if Match(scope, "docs/model.md") {
		t.Error("docs/model.md is outside the scope and matched")
	}
}

// A pattern of many stars against a long name that cannot match is the shape
// that costs exponential time under a naive recursion. A scope is authored, so
// this is not an adversary — it is a promise that would appear to hang.
func TestManyStarsDoNotBacktrackForever(t *testing.T) {
	pattern := "a" + repeat("*a", 24)
	name := repeat("a", 200) + "b"
	if matchSegment(pattern, name) {
		t.Fatal("matched a name it cannot match")
	}
}

func repeat(s string, n int) string {
	out := ""
	for range n {
		out += s
	}
	return out
}
