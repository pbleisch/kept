package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func repository(t *testing.T, version string, files map[string]string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := tree(t, files)
	for _, args := range [][]string{{"init", "-q"}, {"-c", "index.version=" + version, "add", "-A"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Skipf("git %v: %v: %s", args, err, out)
		}
	}
	return root
}

// Version 4 compresses a path against the one before it, which is the part of
// the format a reader can get subtly wrong — and a path read wrongly is an oid
// attributed to the wrong file, which is a digest that says the world did not
// move when it did.
func TestEveryIndexVersionReadsTheSamePaths(t *testing.T) {
	files := map[string]string{
		"a.txt":                    "a\n",
		"internal/corpus/read.go":  "package corpus\n",
		"internal/corpus/write.go": "package corpus\n",
		"internal/ledger/read.go":  "package ledger\n",
	}
	for _, version := range []string{"2", "3", "4"} {
		root := repository(t, version, files)
		index := readGitIndex(root)
		if index == nil {
			t.Errorf("version %s: no index read", version)
			continue
		}
		for name := range files {
			if _, ok := index.entries[name]; !ok {
				t.Errorf("version %s: %s is not in the index this read", version, name)
			}
		}
		if len(index.entries) != len(files) {
			t.Errorf("version %s: read %d entries, want %d", version, len(index.entries), len(files))
		}
	}
}

// The checksum is the guard, not a nicety: it is what proves the file was read
// the way git wrote it. A misparse would attribute an oid to the wrong path,
// and an oid is a key into a digest this package then reports as the world.
func TestAnIndexThatDoesNotVerifyIsDeclined(t *testing.T) {
	root := repository(t, "2", map[string]string{"a.txt": "a\n"})
	path := filepath.Join(root, ".git", "index")

	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	src[40] ^= 0xff // one bit of an oid
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, ok := parseGitIndex(src); ok {
		t.Fatal("an index whose checksum does not verify was read anyway")
	}
	if readGitIndex(root) != nil {
		t.Fatal("a corrupt index was offered as an accelerator")
	}

	// And the walk still answers, because the accelerator was never the
	// definition.
	if got := digestOf(t, root, Options{Cache: OpenCache(t.TempDir())}); len(got.Files) == 0 {
		t.Fatal("the walk stopped because git's index was unreadable")
	}
}

// A conflicted entry is nobody's recorded content, and a symlink's oid names
// the bytes of its target — which is a file's content elsewhere in the tree.
// Both are left to the plain path.
func TestTheIndexIsNotConsultedForASymlink(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := tree(t, map[string]string{"go.mod": "module example\n"})
	if err := os.Symlink("go.mod", filepath.Join(root, "link")); err != nil {
		t.Skipf("symlinks are not available here: %v", err)
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Skipf("git %v: %v: %s", args, err, out)
		}
	}

	index := readGitIndex(root)
	if index == nil {
		t.Skip("no index")
	}
	if _, ok := index.entries["link"]; ok {
		t.Error("a symlink is in the accelerator, so its target's blob could answer for it")
	}
}
