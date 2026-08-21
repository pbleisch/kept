package box

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCreatePinsTheOriginAndHidesTheSideRef(t *testing.T) {
	repository, pinned := boxRepository(t, []boxFile{{"README.md", "pinned\n", 0o644}})
	putSideRef(t, repository, pinned)
	if err := os.WriteFile(filepath.Join(repository, "unselected.md"), []byte("operator-only\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	b, err := Create(context.Background(), Request{
		Repository: repository,
		Commit:     pinned,
		Overlay: []OverlayFile{
			{Path: "README.md", Data: []byte("selected tracked operator change\n")},
			{Path: "operator.md", Data: []byte("selected operator change\n"), Mode: 0o755},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := b.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})

	if b.Commit() != pinned {
		t.Fatalf("box commit = %q, want pinned %q", b.Commit(), pinned)
	}
	if got := boxGit(t, b.Path(), "rev-parse", "HEAD"); got != pinned {
		t.Fatalf("box HEAD = %q, want pinned %q", got, pinned)
	}
	if got := readBoxFile(t, filepath.Join(b.Path(), "README.md")); got != "selected tracked operator change\n" {
		t.Fatalf("box readme = %q", got)
	}
	if got := readBoxFile(t, filepath.Join(repository, "README.md")); got != "pinned\n" {
		t.Fatalf("overlay changed the origin checkout: %q", got)
	}
	if got := readBoxFile(t, filepath.Join(b.Path(), "operator.md")); got != "selected operator change\n" {
		t.Fatalf("box overlay = %q", got)
	}
	if mode := fileMode(t, filepath.Join(b.Path(), "operator.md")); mode != 0o755 {
		t.Fatalf("box overlay mode = %o, want 755", mode)
	}
	if _, err := os.Lstat(filepath.Join(b.Path(), "unselected.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("box received an undefined dirty file: %v", err)
	}
	for _, protected := range []string{
		filepath.Join(b.Path(), ".kept", "ledger"),
		filepath.Join(b.Path(), ".kept", "artifacts"),
	} {
		if _, err := os.Lstat(protected); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("box exposes side-ref data at %s: %v", protected, err)
		}
	}
	if staged := boxGit(t, b.Path(), "diff", "--cached", "--name-only"); staged != "" {
		t.Fatalf("creating a snapshot staged repair work: %q", staged)
	}

	if err := os.WriteFile(filepath.Join(b.Path(), "repair.md"), []byte("agent repair\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	diff, err := b.Diff(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	text := string(diff)
	if !strings.Contains(text, "repair.md") || !strings.Contains(text, "agent repair") {
		t.Fatalf("repair diff does not contain the agent change:\n%s", text)
	}
	if strings.Contains(text, "operator.md") || strings.Contains(text, "README.md") || strings.Contains(text, "selected operator change") || strings.Contains(text, "selected tracked operator change") {
		t.Fatalf("repair diff re-attributed the inherited overlay:\n%s", text)
	}
	if staged := boxGit(t, b.Path(), "diff", "--cached", "--name-only"); staged != "" {
		t.Fatalf("collecting a diff staged repair work: %q", staged)
	}
}

func TestCreateRefusesMovingOrMachineWrittenInputs(t *testing.T) {
	repository, pinned := boxRepository(t, []boxFile{{"README.md", "pinned\n", 0o644}})
	if _, err := Create(context.Background(), Request{Repository: repository, Commit: "HEAD"}); !errors.Is(err, ErrUnpinnedCommit) {
		t.Fatalf("Create at HEAD error = %v, want unpinned commit", err)
	}
	if _, err := Create(context.Background(), Request{
		Repository: repository,
		Commit:     pinned,
		Overlay:    []OverlayFile{{Path: ".kept/artifacts/thread", Data: []byte("forbidden")}},
	}); err == nil || !strings.Contains(err.Error(), "machine-written") {
		t.Fatalf("Create with artifact overlay error = %v, want machine-written refusal", err)
	}

	_, tracked := commitBoxFiles(t, repository, []boxFile{{".kept/ledger/acts.jsonl", "forged\n", 0o644}}, "machine tier")
	if _, err := Create(context.Background(), Request{Repository: repository, Commit: tracked}); err == nil || !strings.Contains(err.Error(), "exposes machine-written") {
		t.Fatalf("Create with checked-out ledger error = %v, want machine-written refusal", err)
	}
	if entries := boxEntries(t, repository); len(entries) != 0 {
		t.Fatalf("failed box remained in .kept/boxes: %v", entries)
	}
	if worktrees := boxGit(t, repository, "worktree", "list", "--porcelain"); strings.Contains(worktrees, ".kept/boxes") {
		t.Fatalf("failed box remained registered:\n%s", worktrees)
	}
}

func TestCreateDiscardsABoxWhenOverlayWouldFollowASymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the repository does not support a native Windows repair box")
	}
	repository, _ := boxRepository(t, []boxFile{{"README.md", "pinned\n", 0o644}})
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(repository, "escape")); err != nil {
		t.Fatal(err)
	}
	boxGit(t, repository, "add", "escape")
	_, pinned := commitBoxFiles(t, repository, nil, "add symlink")

	_, err := Create(context.Background(), Request{
		Repository: repository,
		Commit:     pinned,
		Overlay:    []OverlayFile{{Path: "escape/agent.txt", Data: []byte("must not escape")}},
	})
	if err == nil || !strings.Contains(err.Error(), "would follow a symlink") {
		t.Fatalf("Create with symlink overlay error = %v", err)
	}
	if _, err := os.Lstat(filepath.Join(outside, "agent.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("overlay escaped its box: %v", err)
	}
	if entries := boxEntries(t, repository); len(entries) != 0 {
		t.Fatalf("symlink failure left a box behind: %v", entries)
	}
}

func TestDiffBoundAndCloseRemainSafeAfterCancellation(t *testing.T) {
	repository, pinned := boxRepository(t, []boxFile{{"README.md", "pinned\n", 0o644}})
	b, err := Create(context.Background(), Request{Repository: repository, Commit: pinned, MaxDiffBytes: 64})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(b.Path(), "large-repair.txt"), []byte(strings.Repeat("repair\n", 100)), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Diff(context.Background()); !errors.Is(err, ErrDiffTooLarge) {
		t.Fatalf("Diff error = %v, want bounded refusal", err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := b.Close(canceled); err != nil {
		t.Fatalf("Close after cancellation: %v", err)
	}
	if _, err := os.Lstat(b.Path()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("closed box remains on disk: %v", err)
	}
	if err := b.Close(context.Background()); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if _, err := b.Diff(context.Background()); !errors.Is(err, ErrClosed) {
		t.Fatalf("Diff after Close error = %v, want closed box", err)
	}
}

type boxFile struct {
	path string
	data string
	mode os.FileMode
}

func boxRepository(t *testing.T, files []boxFile) (string, string) {
	t.Helper()
	repository := t.TempDir()
	boxGit(t, repository, "init", "-q")
	boxGit(t, repository, "config", "user.email", "test@example.com")
	boxGit(t, repository, "config", "user.name", "test")
	return commitBoxFiles(t, repository, files, "pinned origin")
}

func commitBoxFiles(t *testing.T, repository string, files []boxFile, message string) (string, string) {
	t.Helper()
	for _, file := range files {
		path := filepath.Join(repository, file.path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(file.data), file.mode); err != nil {
			t.Fatal(err)
		}
	}
	boxGit(t, repository, "add", "--all")
	boxGit(t, repository, "commit", "-qm", message)
	return repository, boxGit(t, repository, "rev-parse", "HEAD")
}

func putSideRef(t *testing.T, repository, pinned string) {
	t.Helper()
	_, side := commitBoxFiles(t, repository, []boxFile{{".kept/artifacts/session", "prior transcript\n", 0o644}, {".kept/ledger/acts.jsonl", "prior act\n", 0o644}}, "side ref")
	boxGit(t, repository, "update-ref", "refs/kept/ledger", side)
	boxGit(t, repository, "reset", "--hard", pinned)
}

func boxEntries(t *testing.T, repository string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(repository, ".kept", "boxes"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	result := make([]string, 0, len(entries))
	for _, entry := range entries {
		result = append(result, entry.Name())
	}
	return result
}

func readBoxFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func fileMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

func boxGit(t *testing.T, directory string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "git", append([]string{"-C", directory}, args...)...)
	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), ctx.Err())
	}
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}
