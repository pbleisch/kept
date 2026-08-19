package workspace

import "testing"

// The format's own rules, honoured without invoking git. These are the cases
// that decide whether a build directory counts as part of the world — and if it
// does, every build makes every promise due and change-driven cadence is worth
// nothing. See docs/decision/0033.
func TestTheIgnoreFormatIsReadAsAFormat(t *testing.T) {
	root := parseIgnore("", `
# a comment
target/
*.tmp
!keep.tmp
/only-at-the-root
doc/frotz/
build
`)
	stack := ignoreStack{root}

	for _, tc := range []struct {
		path  string
		isDir bool
		want  bool
	}{
		{"target", true, true},
		{"internal/target", true, true},   // no separator in the middle: any depth
		{"target.go", false, false},       // a directory rule is not a prefix rule
		{"a/b/c.tmp", false, true},        // no separator: any depth
		{"a/b/keep.tmp", false, false},    // ! re-includes, and it is later
		{"only-at-the-root", false, true}, // a leading slash anchors
		{"deep/only-at-the-root", false, false},
		{"doc/frotz", true, true}, // a separator in the middle anchors
		{"a/doc/frotz", true, false},
		{"build", true, true},
		{"build/out.o", false, true}, // ignoring a directory ignores what is in it
		{"src/main.go", false, false},
	} {
		if got := stack.ignored(tc.path, tc.isDir); got != tc.want {
			t.Errorf("ignored(%q, dir=%v) = %v, want %v", tc.path, tc.isDir, got, tc.want)
		}
	}
}

// Nearest-first. A .gitignore appearing mid-tree takes effect for that subtree
// from the tick that sees it, and the change of digest makes the affected
// promises due once — which is a real event and the safe direction.
func TestTheNearestFileDecides(t *testing.T) {
	stack := ignoreStack{
		parseIgnore("", "*.log\n"),
		parseIgnore("internal/corpus", "!keepme.log\n"),
	}

	if !stack.ignored("internal/x.log", false) {
		t.Error("the root file no longer governs a directory with no file of its own")
	}
	if stack.ignored("internal/corpus/keepme.log", false) {
		t.Error("a nearer file did not override a further one")
	}
}

func TestTheLastMatchingPatternInAFileWins(t *testing.T) {
	stack := ignoreStack{parseIgnore("", "!*.o\n*.o\n")}
	if !stack.ignored("a.o", false) {
		t.Error("an earlier negation beat a later exclusion")
	}
}

func TestTheFormatsOwnMetacharacters(t *testing.T) {
	stack := ignoreStack{parseIgnore("", "?.tmp\n[abc].out\nsp\\ ace\n")}
	for _, tc := range []struct {
		path string
		want bool
	}{
		{"a.tmp", true},
		{"ab.tmp", false},
		{"b.out", true},
		{"d.out", false},
		{"sp ace", true},
	} {
		if got := stack.ignored(tc.path, false); got != tc.want {
			t.Errorf("ignored(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

// ** in the format, which is the construct a naive segment matcher gets wrong.
func TestDoubleStarInTheFormat(t *testing.T) {
	stack := ignoreStack{parseIgnore("", "**/node_modules/\na/**/b\n")}
	for _, tc := range []struct {
		path  string
		isDir bool
		want  bool
	}{
		{"node_modules", true, true},
		{"web/app/node_modules", true, true},
		{"a/b", false, true},
		{"a/x/y/b", false, true},
		{"z/a/b", false, false},
	} {
		if got := stack.ignored(tc.path, tc.isDir); got != tc.want {
			t.Errorf("ignored(%q, dir=%v) = %v, want %v", tc.path, tc.isDir, got, tc.want)
		}
	}
}
