package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// at runs kept with a directory as the working directory, the way a person
// standing in one would. init is the one verb with nothing to point at, so
// where it is run is the whole of what it acts on.
func at(t *testing.T, dir string, args ...string) (int, string, string) {
	t.Helper()
	from, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(from)
	return kept(t, args...)
}

func read(t *testing.T, path string) string {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(src)
}

// What decision 0022 asks init to write, and what makes the directory a
// workspace at all: everything after this is findable from the config.
func TestInitWritesAWorkspaceAndTheTextAttribute(t *testing.T) {
	dir := t.TempDir()
	code, stdout, stderr := at(t, dir, "init")
	if code != exitOK {
		t.Fatalf("init exited %d: %s", code, stderr)
	}
	mustContain(t, "init", stdout, "wrote", ".kept/config.yaml", ".gitattributes", "docs/")

	mustContain(t, ".gitattributes", read(t, filepath.Join(dir, ".gitattributes")), textRule)
	mustContain(t, "the config", read(t, filepath.Join(dir, ".kept", "config.yaml")), "corpus: docs")
	if _, err := os.Stat(filepath.Join(dir, "docs")); err != nil {
		t.Errorf("the corpus root the config names was not created: %v", err)
	}

	// The point of writing it: the next command finds the workspace.
	if code, _, stderr := at(t, dir, "list"); code != exitOK {
		t.Errorf("list in a freshly initialized workspace exited %d: %s", code, stderr)
	}
}

// The guard, and the reason this verb is careful rather than convenient. A
// config decides what is enforced and a text attribute decides how git stores
// every file in the tree; neither is kept's to rewrite once a person has
// written it.
func TestInitNeverOverwritesWhatSomebodyWrote(t *testing.T) {
	dir := t.TempDir()
	const mine = "corpus: manuscript\n# mine, and load-bearing\n"
	const theirs = "*.md diff=markdown\n"
	if err := os.MkdirAll(filepath.Join(dir, ".kept"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".kept", "config.yaml"), []byte(mine), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitattributes"), []byte(theirs), 0o644); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := at(t, dir, "init")
	if code != exitOK {
		t.Fatalf("init exited %d: %s", code, stderr)
	}
	if got := read(t, filepath.Join(dir, ".kept", "config.yaml")); got != mine {
		t.Errorf("init rewrote a config somebody wrote:\n%s", got)
	}
	if got := read(t, filepath.Join(dir, ".gitattributes")); got != theirs {
		t.Errorf("init rewrote a .gitattributes somebody wrote:\n%s", got)
	}

	// Left alone is not the same as unsaid. The rule 0022 wants is handed over
	// to be added by hand, with what it is and is not doing.
	mustContain(t, "init", stdout, "already there", textRule, "was left as it is", "hygiene")

	// And the config that is there is the one that decides where the corpus is.
	// Creating the directory the flag defaulted to would leave an empty one
	// beside the one the workspace actually reads.
	if _, err := os.Stat(filepath.Join(dir, "docs")); err == nil {
		t.Error("init created docs/, and the config it did not write says manuscript")
	}
	if _, err := os.Stat(filepath.Join(dir, "manuscript")); err != nil {
		t.Errorf("init did not create the corpus root the existing config names: %v", err)
	}
	if code, stdout, _ := at(t, dir, "init", "--corpus", "docs"); code != exitOK {
		t.Errorf("init exited %d", code)
	} else if !strings.Contains(stdout, "was not used") {
		t.Errorf("--corpus was overruled by the config and nothing said so:\n%s", stdout)
	}
}

// A workspace inside a workspace changes which config every command in the
// subtree resolves to, so it is a refusal rather than a surprise.
func TestInitRefusesToNestOneWorkspaceInAnother(t *testing.T) {
	dir := t.TempDir()
	if code, _, stderr := at(t, dir, "init"); code != exitOK {
		t.Fatalf("init exited %d: %s", code, stderr)
	}

	inner := filepath.Join(dir, "docs")
	code, _, stderr := at(t, inner, "init")
	if code != exitUsage {
		t.Fatalf("init inside a workspace exited %d, want %d", code, exitUsage)
	}
	mustContain(t, "the refusal", stderr, "already a workspace", "Run init there")

	if _, err := os.Stat(filepath.Join(inner, ".kept")); err == nil {
		t.Error("init wrote a second workspace inside the first")
	}

	// And re-running it where the workspace actually is is not a refusal: it
	// fills in what is missing beside what is there.
	if code, stdout, stderr := at(t, dir, "init"); code != exitOK {
		t.Errorf("re-running init in its own workspace exited %d: %s", code, stderr)
	} else if !strings.Contains(stdout, "already there") {
		t.Errorf("the second run did not say the files were already there:\n%s", stdout)
	}
}

// A manuscript's corpus root is the workspace, which is what --corpus . means.
// Nothing is created for it, and the config still says so.
func TestInitTakesTheCorpusRootItIsGiven(t *testing.T) {
	dir := t.TempDir()
	code, stdout, stderr := at(t, dir, "init", "--corpus", ".")
	if code != exitOK {
		t.Fatalf("init exited %d: %s", code, stderr)
	}
	mustContain(t, "the config", read(t, filepath.Join(dir, ".kept", "config.yaml")), "corpus: .")
	mustContain(t, "init", stdout, "this directory")
	if strings.Contains(stdout, "./") {
		t.Errorf("a corpus root of . was printed as a path:\n%s", stdout)
	}
}
