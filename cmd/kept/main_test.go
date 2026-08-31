package main

import (
	"bytes"
	"os"
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
	code = dispatch(args, &out, &errs)
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

		// Every link, with what it names and the source it was written in.
		"canon:kernel-correctness", "artifact", "serves", "frontmatter",
		"canon/portrait.png", "file",
		"https://example.com/kept/spec", "uri",
		"canon:absent-from-the-body", "body",

		// And every broken one, said in a way that can be acted on.
		"canon:nothing-claims-this", "BROKEN",
		"promise/nothing-is-here.md",

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

		// A file reference with nothing at the other end is a broken link.
		"names no file in the workspace",
	)

	// A URI target is never reported broken. Whether one leads anywhere is a
	// question for the world, and nothing here asks the world anything. The
	// message for a broken target names it after " to ", so that is what is
	// checked — a URI may legitimately appear as the subject of a manifest
	// whose target is the broken end.
	for line := range strings.SplitSeq(stdout, "\n") {
		if !strings.Contains(line, "broken-link") {
			continue
		}
		_, target, found := strings.Cut(line, " to ")
		if found && strings.Contains(target, "://") {
			t.Errorf("a URI target was reported broken: %s", line)
		}
	}
}

// The three kinds are told apart in what a person sees, so a reader can tell why
// one reference was checked against the corpus and another against the disk.
func TestShowNamesWhatEachReferenceIs(t *testing.T) {
	_, stdout, _ := kept(t, "show", "core-packages-are-tested", "--corpus", fixture)

	links, _, _ := strings.Cut(stdout[strings.Index(stdout, "\nlinks\n"):], "\nwhy")
	want := map[string]string{
		"canon:kernel-correctness":      "artifact",
		"canon/portrait.png":            "file",
		"https://example.com/kept/spec": "uri",
	}
	for target, kind := range want {
		var found bool
		for line := range strings.SplitSeq(links, "\n") {
			if strings.Contains(line, target) && strings.Contains(line, kind) {
				found = true
			}
		}
		if !found {
			t.Errorf("no link line names %s as a %s:\n%s", target, kind, links)
		}
	}
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

// A deferred verb says why it is deferred, and the two reasons are not the same
// reason. Nothing is blocked on truth any more — the world is observed — so a
// verb still claiming to need it would be telling someone to wait for something
// that already arrived.
func TestADeferredVerbSaysWhyItIsActuallyDeferred(t *testing.T) {
	for verb := range unbuilt {
		code, _, stderr := kept(t, verb)
		if code != exitUsage {
			t.Errorf("%s exited %d, want %d", verb, code, exitUsage)
		}
		if !strings.HasPrefix(stderr, "kept: "+verb+" ") {
			t.Errorf("%s does not say which verb it is about: %q", verb, stderr)
		}

		// The guard: truth exists, so no verb may claim to be waiting for it.
		// This is the mistake being corrected, so it is the one the test has to
		// be able to catch.
		if strings.Contains(stderr, "needs truth") || strings.Contains(stderr, "nothing observes") {
			t.Errorf("%s says it is waiting for truth, and the world is observed: %q", verb, stderr)
		}
	}

	// And a verb that was deferred and is now built is not in the list at all.
	// land is on this list as of increment 6: a repair anchors what its session
	// left, so a land has something to gate.
	for _, verb := range []string{"init", "list", "show", "status", "ratify", "retire", "drift", "why", "ledger", "repair", "land"} {
		if why, ok := unbuilt[verb]; ok {
			t.Errorf("%s is built and still reported as deferred: %q", verb, why)
		}
	}
}

func TestUnknownAndUnbuiltVerbsExitTwo(t *testing.T) {
	if code, _, stderr := kept(t, "ratify", "something"); code != exitUsage {
		t.Errorf("ratify exited %d, want %d (%s)", code, exitUsage, stderr)
	}
	if code, _, _ := kept(t, "nonsense"); code != exitUsage {
		t.Errorf("an unknown verb exited %d, want %d", code, exitUsage)
	}
	// No verb is not an error: it is the tick, which is the thing a person
	// types most often. Outside a workspace it is still a usage error, because
	// there is no corpus to tick.
	from, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(from)
	if code, _, stderr := kept(t); code != exitUsage || !strings.Contains(stderr, "config.yaml") {
		t.Errorf("kept outside a workspace exited %d: %q", code, stderr)
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

// The gatherers kept ships reach the load, so a promise reading a field nothing
// produces is refused where a person will see it rather than discovered in the
// code path that decides a verdict. The library seam is an interface and the
// wiring is here, so this is the test that the two were actually connected.
// See docs/decision/0036.
func TestTheGathererCatalogueReachesTheCorpusLoad(t *testing.T) {
	root := t.TempDir()
	corpus := filepath.Join(root, "docs")
	if err := os.MkdirAll(filepath.Join(corpus, "promise"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(corpus, "promise", "coverage-holds.md"), []byte(
		"---\nid: coverage-holds\ntype: promise\nfields:\n  statement: coverage holds\n"+
			"  gather:\n    tests: {gatherer: go/tests, scope: \"internal/**\"}\n"+
			"  hold_when: obs.tests.coverage > 80\n---\n\nWhy.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, stdout, _ := kept(t, "show", "coverage-holds", "--corpus", corpus)
	mustContain(t, "show", stdout, "ungathered-observation", "coverage", "packages")
}

// And the escape hatch's own layers reach it too: a command that never says
// what to pull out of a program's output is refused at load rather than storing
// the log. See docs/decision/0023.
func TestACommandWithNoExtractorIsRefusedAtLoad(t *testing.T) {
	root := t.TempDir()
	corpus := filepath.Join(root, "docs")
	if err := os.MkdirAll(filepath.Join(corpus, "promise"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(corpus, "promise", "it-builds.md"), []byte(
		"---\nid: it-builds\ntype: promise\nfields:\n  statement: it builds\n"+
			"  gather:\n    build: {gatherer: command, run: \"go build ./...\"}\n---\n\nWhy.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, stdout, _ := kept(t, "show", "it-builds", "--corpus", corpus)
	mustContain(t, "show", stdout, "bad-gather-spec", "fields:")
}
