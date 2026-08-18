package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// The command is exercised against the same hand-written corpus the library
// reads, so that what a person sees and what the loader found cannot drift.
var (
	fixture   = filepath.Join("..", "..", "internal", "corpus", "testdata", "corpus")
	duplicate = filepath.Join("..", "..", "internal", "corpus", "testdata", "duplicate")
)

func kept(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errs bytes.Buffer
	code = run(args, &out, &errs)
	return code, out.String(), errs.String()
}

func mustContain(t *testing.T, what, got string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Errorf("%s does not contain %q:\n%s", what, want, got)
		}
	}
}

// Prints it faithfully: every field, every link, and every broken one.
func TestShowPrintsEveryFieldAndLink(t *testing.T) {
	code, stdout, stderr := kept(t, "show", "core-packages-are-tested", "--corpus", fixture)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}

	mustContain(t, "show", stdout,
		"promise:core-packages-are-tested",
		"Core packages are tested",
		"promise/core-packages-are-tested.md",

		// Every declared field, with its value.
		"statement", "every package under internal/ has a test that runs and passes",
		"gather", "gatherer: command", "scope: internal/**",
		"hold_when", "all(obs.tests.packages,",
		"trigger", "cadence",
		"repair", "scope: internal/",

		// Every link, with the source it was written in.
		"canon:kernel-correctness", "serves", "frontmatter",
		"canon:absent-from-the-body", "body",

		// And every broken one, said in a way that can be acted on.
		"canon:nothing-claims-this", "BROKEN",

		// The prose, under the name the schema gives it.
		"why (the body)", "Tests that cannot fail are not evidence of correctness.",
	)
}

// Unknown fields intact and available: what a person sees includes the fields
// nothing declares, marked as undeclared rather than dropped.
func TestShowPrintsFieldsNothingDeclares(t *testing.T) {
	_, stdout, _ := kept(t, "show", "core-packages-are-tested", "--corpus", fixture)

	for _, name := range []string{"owner", "review_every", "reviewed_by"} {
		if !strings.Contains(stdout, name) {
			t.Errorf("show dropped the undeclared field %q:\n%s", name, stdout)
		}
	}
	mustContain(t, "show", stdout, "paul", "90d", "the operator", undeclared)

	// The guard: a declared field must not be marked undeclared, or the marker
	// says nothing.
	for line := range strings.SplitSeq(stdout, "\n") {
		if strings.Contains(line, "statement") && strings.Contains(line, undeclared) {
			t.Errorf("a declared field was marked undeclared: %s", line)
		}
	}
}

func TestListPrintsEveryArtifactAndEveryProblem(t *testing.T) {
	code, stdout, stderr := kept(t, "list", "--corpus", fixture)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}

	mustContain(t, "list", stdout,
		"core-packages-are-tested", "kernel-correctness", "loose-note", "reported-not-refused",
		"4 artifacts",
		"broken-link", "no-frontmatter", "field-violation", "no-schema", "path-mismatch",
	)
}

// Two artifacts claiming one slug fail the load, and both paths are named on
// the way out so that fixing it needs no second command.
func TestDuplicateSlugExitsTwoNamingBothPaths(t *testing.T) {
	code, _, stderr := kept(t, "list", "--corpus", duplicate)

	if code != exitUsage {
		t.Errorf("exit = %d, want %d — an unparseable corpus is a usage error", code, exitUsage)
	}
	mustContain(t, "stderr", stderr,
		"promise:tests-run",
		"promise/first.md",
		"promise/second.md",
	)
}

func TestUnknownAndUnbuiltVerbsExitTwo(t *testing.T) {
	if code, _, stderr := kept(t, "ratify", "something"); code != exitUsage {
		t.Errorf("ratify exited %d, want %d (%s)", code, exitUsage, stderr)
	}
	if code, _, _ := kept(t, "nonsense"); code != exitUsage {
		t.Errorf("an unknown verb exited %d, want %d", code, exitUsage)
	}
	if code, _, _ := kept(t); code != exitUsage {
		t.Errorf("no verb exited %d, want %d", code, exitUsage)
	}
}

func TestShowRefusesAnArtifactItCannotName(t *testing.T) {
	if code, _, stderr := kept(t, "show", "no-such-thing", "--corpus", fixture); code != exitUsage {
		t.Errorf("exit = %d, want %d (%s)", code, exitUsage, stderr)
	}
	if code, _, _ := kept(t, "show", "--corpus", fixture); code != exitUsage {
		t.Error("show with no artifact succeeded")
	}
}

func TestHelpAndVersion(t *testing.T) {
	code, stdout, _ := kept(t, "help")
	if code != exitOK {
		t.Errorf("help exited %d", code)
	}
	mustContain(t, "help", stdout, "kept list", "kept show")

	if code, stdout, _ := kept(t, "version"); code != exitOK || !strings.HasPrefix(stdout, "kept ") {
		t.Errorf("version exited %d with %q", code, stdout)
	}
}

// The rendering is deterministic, so one corpus prints the same way twice and a
// map's iteration order never reaches a person.
func TestRenderingIsDeterministic(t *testing.T) {
	_, first, _ := kept(t, "show", "core-packages-are-tested", "--corpus", fixture)
	for range 8 {
		if _, again, _ := kept(t, "show", "core-packages-are-tested", "--corpus", fixture); again != first {
			t.Fatal("show printed the same artifact two different ways")
		}
	}
}
