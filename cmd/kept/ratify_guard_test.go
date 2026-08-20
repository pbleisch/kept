package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/config"
)

// An act names the person who performed it, in a record that is append-only and
// tracked, so a value nobody stated cannot be taken back. kept used to fall back
// to the operating system's username and then to the literal "unknown" — one
// machine-local string that collides across machines, and one that names nobody
// on the only act the model reserves to a person. Both are gone.
// See docs/decision/0048.
func TestAnActWithNoNamedOperatorIsRefusedAndWritesNothing(t *testing.T) {
	t.Setenv("KEPT_BY", "")
	noUserConfig(t)
	w := newWorkspace(t, map[string]string{"tests-run": "the tests run"})

	code, _, stderr := w.run("ratify", "promise:tests-run")
	if code != exitUsage {
		t.Fatalf("an act with no named operator exited %d, want %d — kept guessed an identity", code, exitUsage)
	}
	for _, want := range []string{"operator:", "--by", "KEPT_BY"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("the refusal does not name %q as a way to set it:\n%s", want, stderr)
		}
	}

	// Nothing was appended. A refused act that had already opened the stream
	// would leave a record of the act it refused to perform.
	if _, err := os.Stat(w.root + "/.kept/ledger/acts.jsonl"); err == nil {
		t.Error("an act was refused and the acts stream was written anyway")
	}

	// And the three routes each work, in the order 0048 sets.
	if code, _, stderr := w.run("ratify", "promise:tests-run", "--by", "by the flag"); code != exitOK {
		t.Fatalf("--by was refused: %d %s", code, stderr)
	}
	if !strings.Contains(w.read(".kept/ledger/acts.jsonl"), "by the flag") {
		t.Error("--by did not reach the record")
	}
}

// noUserConfig points os.UserConfigDir at an empty directory, so that a machine
// that has an operator set does not hide a test asserting what happens when one
// is not. It returns where the user's config would be.
func noUserConfig(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)                       // darwin: ~/Library/Application Support
	t.Setenv("XDG_CONFIG_HOME", home+"/.config") // linux
	path, err := config.UserPath()
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTheUsersOwnConfigNamesTheOperator(t *testing.T) {
	t.Setenv("KEPT_BY", "")
	dir := noUserConfig(t)
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir, []byte("operator: The Operator <op@example.com>\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	w := newWorkspace(t, map[string]string{"tests-run": "the tests run"})
	if code, _, stderr := w.run("ratify", "promise:tests-run"); code != exitOK {
		t.Fatalf("an operator named in the user's config was refused: %d %s", code, stderr)
	}
	acts := w.read(".kept/ledger/acts.jsonl")
	if !strings.Contains(acts, "The Operator") || !strings.Contains(acts, "op@example.com") {
		t.Errorf("the user's operator did not reach the record:\n%s", acts)
	}
}

// The repository's config is tracked, so an operator key in it would attribute
// the next person who clones and ratifies to whoever committed it — silently,
// and permanently. There is no repository-level fallback, deliberately, and this
// is the guard that keeps it that way. See docs/decision/0048.
func TestTheRepositorysConfigCannotNameTheOperator(t *testing.T) {
	t.Setenv("KEPT_BY", "")
	noUserConfig(t)

	w := newWorkspace(t, map[string]string{"tests-run": "the tests run"})
	w.put(".kept/config.yaml", configFile+"\noperator: Someone Who Committed This <them@example.com>\n")

	code, _, stderr := w.run("ratify", "promise:tests-run")
	if code != exitUsage {
		t.Fatalf("a tracked config named the operator and the act went through (exit %d): "+
			"every clone would attribute its acts to whoever committed that file", code)
	}
	if strings.Contains(stderr, "them@example.com") {
		t.Error("the refusal echoed the tracked identity, which suggests it was read")
	}
}
