package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// tree writes a workspace and backdates every file, so that the racy-index rule
// — which distrusts anything written within the cache's own snapshot
// granularity — is not what a test about caching is measuring.
func tree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	writeInto(t, root, files)
	return root
}

func writeInto(t *testing.T, root string, files map[string]string) {
	t.Helper()
	old := time.Now().Add(-10 * time.Second)
	for name, body := range files {
		full := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(full, old, old); err != nil {
			t.Fatal(err)
		}
	}
}

func digestOf(t *testing.T, root string, opt Options) *Tree {
	t.Helper()
	got, err := Read(root, opt)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

var sample = map[string]string{
	".gitignore":                   "target/\n*.tmp\n",
	"go.mod":                       "module example\n",
	"cmd/kept/main.go":             "package main\n",
	"internal/corpus/read.go":      "package corpus\n",
	"internal/corpus/read_test.go": "package corpus\n",
	"docs/model.md":                "# the model\n",
	"target/debug/huge.bin":        "not the world\n",
	"scratch.tmp":                  "not the world either\n",
}

// The check that keeps an accelerator from becoming a second definition. One
// tree, every path to the digest, one answer — and it must be able to fire, so
// the caches are deliberately desynchronised further down.
//
// Decision 0033 names four accelerations and says outright that none of them
// changes what the number means. This is where that is true or not.
func TestEveryPathToTheDigestAgrees(t *testing.T) {
	root := tree(t, sample)
	kept := filepath.Join(root, ".kept")

	plain := digestOf(t, root, Options{NoGit: true}).Digest()

	parallel := digestOf(t, root, Options{NoGit: true, Workers: 8}).Digest()
	if parallel != plain {
		t.Errorf("parallel = %s, plain = %s", parallel, plain)
	}

	cache := OpenCache(kept)
	cold := digestOf(t, root, Options{NoGit: true, Cache: cache}).Digest()
	if cold != plain {
		t.Errorf("cold cache = %s, plain = %s", cold, plain)
	}
	if err := cache.Save(); err != nil {
		t.Fatal(err)
	}

	warm := digestOf(t, root, Options{NoGit: true, Cache: OpenCache(kept), Workers: 4})
	if warm.Digest() != plain {
		t.Errorf("warm cache = %s, plain = %s", warm.Digest(), plain)
	}
	if warm.Read != 0 {
		t.Errorf("a warm cache read %d files; the stat cache is not being used", warm.Read)
	}

	forced := digestOf(t, root, Options{Cache: OpenCache(kept), All: true}).Digest()
	if forced != plain {
		t.Errorf("--all = %s, plain = %s", forced, plain)
	}

	git := gitAccelerated(t, root)
	if git != plain {
		t.Errorf("git-accelerated = %s, plain = %s", git, plain)
	}
}

// gitAccelerated digests the tree through git's index: kept's own stat cache
// knows the blobs and nothing else, which is the state a checkout leaves — every
// mtime rewritten and no blob oid changed.
func gitAccelerated(t *testing.T, root string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed, so the accelerator cannot be exercised here")
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Skipf("git %v: %v: %s", args, err, out)
		}
	}

	// A cache of its own, because a stat cache that already knows every path
	// would answer first and the index would never be asked.
	own := t.TempDir()
	warm := OpenCache(own)
	digestOf(t, root, Options{Cache: warm})
	if err := warm.Save(); err != nil {
		t.Fatal(err)
	}

	// Forget every path and keep what git already hashed, which is what a
	// checkout leaves: every mtime rewritten and no blob oid changed.
	cache := OpenCache(own)
	if len(cache.blobs) == 0 {
		t.Fatal("the index vouched for nothing, so this is not testing the accelerator")
	}
	clear(cache.entries)
	clear(cache.digests)

	got := digestOf(t, root, Options{Cache: cache})
	if got.Read != 0 {
		t.Errorf("read %d files with a usable index; the accelerator is not being used", got.Read)
	}
	return got.Digest()
}

// The desynchronisation, per AGENTS.md: a check that has never fired is not a
// check. Change the content behind the cache's back and the digest must move.
func TestTheContentPathWinsWhenTheCacheIsStale(t *testing.T) {
	root := tree(t, sample)
	kept := filepath.Join(root, ".kept")

	cache := OpenCache(kept)
	before := digestOf(t, root, Options{NoGit: true, Cache: cache}).Digest()
	if err := cache.Save(); err != nil {
		t.Fatal(err)
	}

	writeInto(t, root, map[string]string{"internal/corpus/read.go": "package corpus // repaired\n"})

	after := digestOf(t, root, Options{NoGit: true, Cache: OpenCache(kept)}).Digest()
	if after == before {
		t.Fatal("the cache served an answer for content that had changed")
	}
	if plain := digestOf(t, root, Options{NoGit: true}).Digest(); after != plain {
		t.Fatalf("cached = %s, plain = %s: the cache is a second definition", after, plain)
	}
}

// Stat can lie, and the racy rule is the mitigation that is not a proof: an
// entry whose mtime falls inside the snapshot's own granularity is read again
// rather than believed.
func TestAFileWrittenInsideTheSnapshotWindowIsReadAgain(t *testing.T) {
	root := t.TempDir()
	kept := filepath.Join(root, ".kept")
	if err := os.WriteFile(filepath.Join(root, "fresh.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cache := OpenCache(kept)
	digestOf(t, root, Options{NoGit: true, Cache: cache})
	if err := cache.Save(); err != nil {
		t.Fatal(err)
	}

	again := digestOf(t, root, Options{NoGit: true, Cache: OpenCache(kept)})
	if again.Read != 1 {
		t.Errorf("read %d files; a file written moments before the snapshot was believed on its stat", again.Read)
	}
}

// The tick must not feed itself. While the ledger is plain files in the
// workspace, a truth transition would change the world, which would make every
// promise due, which would record more. See docs/decision/0033 and 0016.
func TestTheLedgerIsNotPartOfTheWorld(t *testing.T) {
	root := tree(t, sample)
	before := digestOf(t, root, Options{NoGit: true}).Digest()

	writeInto(t, root, map[string]string{
		".kept/ledger/truth/promise/core-packages-are-tested.jsonl": `{"v":1,"kind":"verdict"}` + "\n",
		".kept/cache/digest.json":                                   `{"v":1}`,
	})

	if after := digestOf(t, root, Options{NoGit: true}).Digest(); after != before {
		t.Fatal("recording a verdict changed the world, so every promise is now due and the tick feeds itself")
	}
}

// Honouring ignore rules is ten times faster and, independently, correct: if a
// build counted, every build would make every promise due and change-driven
// cadence would be worth nothing.
func TestABuildIsNotTheWorld(t *testing.T) {
	root := tree(t, sample)
	before := digestOf(t, root, Options{NoGit: true}).Digest()

	writeInto(t, root, map[string]string{
		"target/debug/kept":  "a fresh binary\n",
		"target/debug/other": "and another\n",
		"notes.tmp":          "scratch\n",
	})
	if after := digestOf(t, root, Options{NoGit: true}).Digest(); after != before {
		t.Error("a build changed the world")
	}

	writeInto(t, root, map[string]string{"internal/corpus/read.go": "package corpus // edited\n"})
	if after := digestOf(t, root, Options{NoGit: true}).Digest(); after == before {
		t.Error("an edit to a file that counts did not change the world")
	}
}

func TestConfigExcludesMore(t *testing.T) {
	root := tree(t, sample)
	plain := digestOf(t, root, Options{NoGit: true}).Digest()
	narrowed := digestOf(t, root, Options{NoGit: true, Exclude: []string{"docs/"}}).Digest()
	if plain == narrowed {
		t.Error("an exclusion from config.yaml changed nothing")
	}
}

// Content hashing is the definition and stat is only the key. Two identical
// checkouts have different mtimes and different inodes, and if those decided
// the answer their observations would never dedup — which is docs/decision/0013's
// rule that the same tree measured by the same gatherer is the same
// observation, whoever ran it.
func TestTwoCheckoutsOfOneTreeAgree(t *testing.T) {
	here := tree(t, sample)
	there := t.TempDir()
	writeInto(t, there, sample)
	now := time.Now()
	for name := range sample {
		if err := os.Chtimes(filepath.Join(there, filepath.FromSlash(name)), now, now); err != nil {
			t.Fatal(err)
		}
	}

	if a, b := digestOf(t, here, Options{NoGit: true}).Digest(), digestOf(t, there, Options{NoGit: true}).Digest(); a != b {
		t.Fatalf("one tree in two places digests as %s and %s", a, b)
	}
}

func TestAScopeIsAFoldOverWhatItMatched(t *testing.T) {
	root := tree(t, sample)
	before := digestOf(t, root, Options{NoGit: true}).Scope([]string{"internal/**"})
	if before.MatchedNothing() {
		t.Fatal("internal/** matched nothing")
	}
	if before.Paths != 2 {
		t.Errorf("matched %d paths, want 2", before.Paths)
	}

	writeInto(t, root, map[string]string{"docs/model.md": "# the model, revised\n"})
	outside := digestOf(t, root, Options{NoGit: true}).Scope([]string{"internal/**"})
	if !outside.Unchanged(before.Digest) {
		t.Error("a change outside the scope made the scope due")
	}

	writeInto(t, root, map[string]string{"internal/corpus/read.go": "package corpus // edited\n"})
	inside := digestOf(t, root, Options{NoGit: true}).Scope([]string{"internal/**"})
	if inside.Unchanged(before.Digest) {
		t.Error("a change inside the scope did not make the scope due")
	}
}

// Matching nothing is not hashing nothing. The fold over an empty set is a
// perfectly stable value, so a mis-typed scope would read as "nothing has
// changed" forever — the silent permanence 0033 exists to stop, arriving from
// the other side.
func TestAScopeThatMatchesNothingIsAlwaysDue(t *testing.T) {
	root := tree(t, sample)
	got := digestOf(t, root, Options{NoGit: true})

	empty := got.Scope([]string{"internal/nothing/**"})
	if !empty.MatchedNothing() {
		t.Fatal("a scope over nothing reported paths")
	}
	if empty.String() != MatchedNothing {
		t.Errorf("recorded as %q, which is not distinguishable from a digest", empty.String())
	}
	if empty.Unchanged(empty.String()) {
		t.Fatal("a scope matching nothing compared equal to itself, so it would never be due again")
	}

	other := got.Scope([]string{"also/nothing/**"})
	if empty.Unchanged(other.String()) {
		t.Fatal("two scopes matching nothing agree, which is the collision that reads as unchanged")
	}
}

// A symlink is part of the tree, is never followed, and is not the same content
// as a file holding the text it points at.
func TestASymlinkIsNotTheFileItNames(t *testing.T) {
	root := t.TempDir()
	writeInto(t, root, map[string]string{"a.txt": "go.mod"})
	if err := os.Symlink("go.mod", filepath.Join(root, "b.txt")); err != nil {
		t.Skipf("symlinks are not available here: %v", err)
	}

	got := digestOf(t, root, Options{NoGit: true})
	if len(got.Files) != 2 {
		t.Fatalf("walked %d files, want 2", len(got.Files))
	}
	if got.Files[0].Digest == got.Files[1].Digest {
		t.Error("a symlink and a file holding its target are one content")
	}
}

// The operator's exclusions are not a .gitignore's to overturn. A repository
// that re-includes what the workspace was configured to leave out would make
// the config a suggestion.
func TestAnIgnoreFileCannotReincludeWhatTheConfigExcluded(t *testing.T) {
	root := tree(t, map[string]string{
		".gitignore":    "!vendor/**\n",
		"main.go":       "package main\n",
		"vendor/dep.go": "package dep\n",
	})

	with := digestOf(t, root, Options{NoGit: true, Exclude: []string{"vendor/"}})
	for _, f := range with.Files {
		if f.Path == "vendor/dep.go" {
			t.Fatal("a .gitignore re-included what config.yaml excluded")
		}
	}
	without := digestOf(t, root, Options{NoGit: true})
	if len(without.Files) != len(with.Files)+1 {
		t.Errorf("walked %d files without the exclusion and %d with it", len(without.Files), len(with.Files))
	}
}
