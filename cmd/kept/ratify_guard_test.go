package main

import (
	"os"
	"strings"
	"testing"
)

// An act names the person who performed it, in a record that is append-only and
// tracked, so a value nobody stated cannot be taken back. kept used to fall back
// to the operating system's username and then to the literal "unknown" — one
// machine-local string that collides across machines, and one that names nobody
// on the only act the model reserves to a person. Both are gone.
// See docs/decision/0048.
func TestAnActWithNoNamedOperatorIsRefusedAndWritesNothing(t *testing.T) {
	t.Setenv("KEPT_BY", "")
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

func TestConfigNamesTheOperatorWhenNothingElseDoes(t *testing.T) {
	t.Setenv("KEPT_BY", "")
	w := newWorkspace(t, map[string]string{"tests-run": "the tests run"})
	w.put(".kept/config.yaml", configFile+"\noperator: The Operator <op@example.com>\n")

	if code, _, stderr := w.run("ratify", "promise:tests-run"); code != exitOK {
		t.Fatalf("an operator named in config was refused: %d %s", code, stderr)
	}
	// The record JSON-escapes the angle brackets, so match the parts.
	acts := w.read(".kept/ledger/acts.jsonl")
	if !strings.Contains(acts, "The Operator") || !strings.Contains(acts, "op@example.com") {
		t.Errorf("operator: did not reach the record:\n%s", acts)
	}
}
