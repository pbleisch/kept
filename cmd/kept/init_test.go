package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/ledger"
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
	mustContain(t, ".gitattributes", read(t, filepath.Join(dir, ".gitattributes")), "ledger/**/*.jsonl merge=union")
	mustContain(t, "the config", read(t, filepath.Join(dir, ".kept", "config.yaml")), "corpus: docs")
	if _, err := os.Stat(filepath.Join(dir, "docs")); err != nil {
		t.Errorf("the corpus root the config names was not created: %v", err)
	}

	// The point of writing it: the next command finds the workspace.
	if code, _, stderr := at(t, dir, "list"); code != exitOK {
		t.Errorf("list in a freshly initialized workspace exited %d: %s", code, stderr)
	}
}

func TestInitConfiguresTheLedgerSideRefFetchOnce(t *testing.T) {
	git := gitOrSkip(t)
	dir := t.TempDir()
	git(dir, "init", "-q")
	git(dir, "config", "--add", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*")

	if code, stdout, stderr := at(t, dir, "init"); code != exitOK {
		t.Fatalf("init exited %d: %s", code, stderr)
	} else if !strings.Contains(stdout, "origin fetches "+ledger.Ref+" into "+ledger.UpstreamRef) {
		t.Fatalf("init did not report its ledger fetch configuration:\n%s", stdout)
	}
	// Into the upstream ref, and 0062 is why: a refspec whose destination is
	// the ledger makes the network the writer of the durable record — forced it
	// rewinds it, unforced it refuses every fetch while the ledger is ahead.
	// The branch refspec beside it is git's and is left exactly as it was
	// found.
	const want = "+refs/heads/*:refs/remotes/origin/*|" + ledger.FetchRefspec
	if got := strings.Fields(gitOut(t, git, dir, "config", "--get-all", "remote.origin.fetch")); strings.Join(got, "|") != want {
		t.Fatalf("origin fetch refspecs = %#v, want the ordinary and kept side refs", got)
	}

	if code, _, stderr := at(t, dir, "init"); code != exitOK {
		t.Fatalf("second init exited %d: %s", code, stderr)
	}
	if got := strings.Fields(gitOut(t, git, dir, "config", "--get-all", "remote.origin.fetch")); strings.Join(got, "|") != want {
		t.Fatalf("second init altered or duplicated fetch refspecs: %#v", got)
	}
}

// The existing-clone question, answered in the direction that leaves nothing
// armed and nothing silent. Every workspace adopted before docs/decision/0062
// carries a spelling that fetches straight into the ledger, and a new constant
// rewrites nobody's config on its own. init corrects it, because the
// alternative is a live trap in every clone that ever ran the old verb; and it
// says what it changed, because rewriting somebody's git config without a word
// is the failure on the other side of that.
//
// Both old spellings, because a clone may carry either: the forced one every
// workspace adopted before 2026-08-22 has, and the unforced one every workspace
// adopted after it has.
func TestInitCorrectsADirectLedgerRefspecAndSaysSo(t *testing.T) {
	for _, spelling := range ledger.DirectRefspecs {
		t.Run(spelling, func(t *testing.T) {
			git := gitOrSkip(t)
			dir := t.TempDir()
			git(dir, "init", "-q")
			git(dir, "config", "--add", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*")
			git(dir, "config", "--add", "remote.origin.fetch", spelling)

			code, stdout, stderr := at(t, dir, "init")
			if code != exitOK {
				t.Fatalf("init exited %d: %s", code, stderr)
			}
			mustContain(t, "init", stdout, "corrected", spelling, ledger.FetchRefspec, "rewound the ledger over acts nobody pushed")

			const want = "+refs/heads/*:refs/remotes/origin/*|" + ledger.FetchRefspec
			if got := strings.Fields(gitOut(t, git, dir, "config", "--get-all", "remote.origin.fetch")); strings.Join(got, "|") != want {
				t.Fatalf("origin fetch refspecs = %#v, want the direct ledger spelling replaced and nothing else touched", got)
			}

			// Corrected once. A second init has nothing to say about it, and
			// must not claim it did something.
			if code, stdout, stderr := at(t, dir, "init"); code != exitOK {
				t.Fatalf("second init exited %d: %s", code, stderr)
			} else if strings.Contains(stdout, "corrected") {
				t.Fatalf("init reported correcting a refspec it had already corrected:\n%s", stdout)
			}
		})
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
	mustContain(t, "init", stdout, "already there", textRule, "was left as it is",
		"bytes churn", "Ratifications are unaffected")

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

// The other half of what init writes into remote.origin.fetch, and the defect
// that made it necessary. Measured on this repository on 2026-08-25, that key
// held the ledger's refspec and nothing else: `git fetch origin` exited 0,
// updated no branch, and left refs/remotes/origin/main eight commits behind
// what origin actually had. Adding one line where there had been none had
// narrowed the remote to the ledger.
//
// init adds the ordinary branch refspec beside it, and the assertion is what a
// fetch does rather than what the config says — the config is the mechanism and
// the tracking ref is the thing that was wrong.
func TestInitConfiguresABranchRefspecWhenOriginFetchesNoBranch(t *testing.T) {
	git := gitOrSkip(t)
	root := t.TempDir()
	origin, dir := filepath.Join(root, "origin.git"), filepath.Join(root, "work")
	for _, d := range []string{origin, dir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	git(origin, "init", "-q", "--bare")
	git(dir, "init", "-q")
	git(dir, "config", "user.email", "test@example.com")
	git(dir, "config", "user.name", "test")
	git(dir, "config", "remote.origin.url", origin)
	git(dir, "config", "--add", "remote.origin.fetch", ledger.FetchRefspec)

	code, stdout, stderr := at(t, dir, "init")
	if code != exitOK {
		t.Fatalf("init exited %d: %s", code, stderr)
	}
	mustContain(t, "init", stdout, "origin fetches its branches", ledger.BranchRefspec, "a ref that reads as current and is not")

	if want := ledger.FetchRefspec + "|" + ledger.BranchRefspec; strings.Join(strings.Fields(gitOut(t, git, dir, "config", "--get-all", "remote.origin.fetch")), "|") != want {
		t.Fatalf("origin fetch refspecs = %q, want %q", gitOut(t, git, dir, "config", "--get-all", "remote.origin.fetch"), want)
	}

	// What it was for. A commit on origin's main reaches the tracking ref now.
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(dir, "add", "file.txt")
	git(dir, "commit", "-q", "-m", "one")
	git(dir, "push", "-q", origin, "HEAD:refs/heads/main")
	git(dir, "fetch", "--quiet", "origin")
	want := strings.TrimSpace(gitOut(t, git, dir, "rev-parse", "HEAD"))
	if got := strings.TrimSpace(gitOut(t, git, dir, "rev-parse", "refs/remotes/origin/main")); got != want {
		t.Fatalf("refs/remotes/origin/main = %q after a fetch, want %q", got, want)
	}

	// Added once, and a second init has nothing to say about it.
	if code, stdout, stderr := at(t, dir, "init"); code != exitOK {
		t.Fatalf("second init exited %d: %s", code, stderr)
	} else if strings.Contains(stdout, "origin fetches its branches") {
		t.Fatalf("init reported configuring a branch refspec it had already configured:\n%s", stdout)
	}
	if want := ledger.FetchRefspec + "|" + ledger.BranchRefspec; strings.Join(strings.Fields(gitOut(t, git, dir, "config", "--get-all", "remote.origin.fetch")), "|") != want {
		t.Fatalf("second init altered or duplicated fetch refspecs: %q", gitOut(t, git, dir, "config", "--get-all", "remote.origin.fetch"))
	}
}
