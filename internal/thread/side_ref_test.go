package thread

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/run"
)

// This is deliberately an actual Git worktree rather than a fake box. The
// side ref's reason for existing is structural: a repair worktree receives the
// repository tree but cannot receive the ledger or prior session artifacts.
func TestSideRefKeepsAThreadReceiptOutOfARepairWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Git is required to create the repair worktree: " + err.Error())
	}
	repository := t.TempDir()
	sideRefGit(t, repository, "init", "-q")
	sideRefGit(t, repository, "config", "user.email", "test@example.com")
	sideRefGit(t, repository, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(repository, "README.md"), []byte("repair target\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sideRefGit(t, repository, "add", "README.md")
	sideRefGit(t, repository, "commit", "-qm", "repair target")

	store := ledger.NewRefStore(repository)
	parent := ledger.Open(store, run.Start("test", nil))
	receipt, err := NewReceipt(parent, MintID())
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := receipt.ForAdapter(Claude, "fixture:1")
	if err != nil {
		t.Fatal(err)
	}
	native, err := adapter.Native("fixture.delta", json.RawMessage(`{"text":"partial repair evidence"}`), Redaction{})
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := adapter.Transcript(native, []byte("irreproducible repair transcript"), Redaction{})
	if err != nil {
		t.Fatal(err)
	}
	if err := parent.Close(run.Clean); err != nil {
		t.Fatal(err)
	}

	entries, quarantined, err := ReadReceipt(store, receipt.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(quarantined) != 0 || ReceiptText(entries) != "thread_native,thread_transcript" {
		t.Fatalf("parent receipt = %q, quarantine %#v", ReceiptText(entries), quarantined)
	}
	transcript, err := ledger.ReadArtifact(store, fragment)
	if err != nil || string(transcript) != "irreproducible repair transcript" {
		t.Fatalf("parent transcript = %q, %v", transcript, err)
	}

	box := filepath.Join(t.TempDir(), "repair-box")
	sideRefGit(t, repository, "worktree", "add", "--detach", box, "HEAD")
	t.Cleanup(func() { sideRefGit(t, repository, "worktree", "remove", "--force", box) })
	for _, hidden := range []string{
		filepath.Join(box, ".kept", "ledger"),
		filepath.Join(box, ".kept", "artifacts"),
	} {
		if _, err := os.Stat(hidden); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("repair worktree contains side-ref data at %s: %v", hidden, err)
		}
	}
}

func sideRefGit(t *testing.T, repository string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "git", append([]string{"-C", repository}, args...)...)
	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), ctx.Err())
	}
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}
