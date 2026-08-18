package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const schemaFile = `type: promise
hash: [statement, gather, hold_when, trigger]
fields:
  statement: { type: string, required: true }
  gather:    { type: map }
  hold_when: { type: string }
  trigger:   { type: enum, values: [cadence, on-act, once] }
body: why
`

const configFile = "corpus: docs\n"

// promiseAt writes a promise with a distinguishable statement, so that editing
// one and reading the register back cannot be confused with reading another.
func promiseAt(slug, statement string) string {
	return "---\nid: " + slug + "\ntitle: " + slug + `
fields:
  statement: ` + statement + `
  hold_when: obs.tests.failed == 0
  trigger: cadence
---

Prose that binds nothing, so it may be rewritten freely.
`
}

// workspace is a repository with a corpus in docs/ and a .kept beside it, which
// is the layout .kept/config.yaml describes. Commands are run against it by
// changing directory, so that the corpus root and the ledger are both found the
// way they are found in a real repository.
type workspace struct {
	t    *testing.T
	root string
}

func newWorkspace(t *testing.T, promises map[string]string) *workspace {
	t.Helper()
	w := &workspace{t: t, root: t.TempDir()}
	w.put(".kept/config.yaml", configFile)
	w.put("docs/_schema/promise.yaml", schemaFile)
	for slug, statement := range promises {
		w.put("docs/promise/"+slug+".md", promiseAt(slug, statement))
	}
	return w
}

func (w *workspace) put(name, body string) {
	w.t.Helper()
	path := filepath.Join(w.root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		w.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		w.t.Fatal(err)
	}
}

// run invokes kept with this workspace as the working directory, exactly as a
// person at a terminal would.
func (w *workspace) run(args ...string) (int, string, string) {
	w.t.Helper()
	from, err := os.Getwd()
	if err != nil {
		w.t.Fatal(err)
	}
	if err := os.Chdir(w.root); err != nil {
		w.t.Fatal(err)
	}
	defer os.Chdir(from)

	return kept(w.t, args...)
}

// line returns the register row for one artifact.
func (w *workspace) line(id string) string {
	w.t.Helper()
	code, stdout, stderr := w.run("status")
	if code != exitOK {
		w.t.Fatalf("status exited %d: %s", code, stderr)
	}
	for _, l := range strings.Split(stdout, "\n") {
		if strings.HasPrefix(l, id+" ") {
			return l
		}
	}
	w.t.Fatalf("%s is not in the register:\n%s", id, stdout)
	return ""
}

// The second clause of increment 2's done-condition: kept status shows across
// the corpus which of the things you approved have been edited since you
// approved them. One artifact at a time is not a workflow.
func TestStatusShowsEditedRatificationsAcrossTheCorpus(t *testing.T) {
	w := newWorkspace(t, map[string]string{
		"tests-run":     "every package has a test",
		"docs-are-true": "the deck matches main.go",
		"deps-recorded": "every module has a line in dependencies.md",
		"nobody-asked":  "this one was never approved",
	})

	for _, id := range []string{"promise:tests-run", "promise:docs-are-true", "promise:deps-recorded"} {
		if code, _, stderr := w.run("ratify", id); code != exitOK {
			t.Fatalf("ratify %s exited %d: %s", id, code, stderr)
		}
	}

	// Two edits, of two different kinds. One changes what is promised; the
	// other rewrites prose the schema does not bind.
	w.put("docs/promise/tests-run.md",
		strings.Replace(promiseAt("tests-run", "every package has a test"),
			"obs.tests.failed == 0", "obs.tests.failed < 3", 1))
	w.put("docs/promise/docs-are-true.md",
		strings.Replace(promiseAt("docs-are-true", "the deck matches main.go"),
			"Prose that binds nothing, so it may be rewritten freely.",
			"Rewritten prose. Entirely different words, and the same promise.", 1))

	code, stdout, stderr := w.run("status")
	if code != exitOK {
		t.Fatalf("status exited %d: %s", code, stderr)
	}

	mustContain(t, "status", stdout,
		// The headline: approved, then changed.
		"promise:tests-run", "unratified",
		// Untouched, and prose-only, both still in force.
		"promise:docs-are-true", "promise:deps-recorded", "ratified",
		// Never approved at all, and said differently from the above.
		"promise:nobody-asked", "never ratified",
		// The corpus-wide summary, which is the point of the verb.
		"4 artifacts",
	)

	if got := w.line("promise:tests-run"); !strings.Contains(got, "unratified") {
		t.Errorf("an edited promise reads %q", got)
	}
	if got := w.line("promise:docs-are-true"); !strings.Contains(got, "ratified") || strings.Contains(got, "unratified") {
		t.Errorf("rewriting prose dropped standing: %q", got)
	}
	if got := w.line("promise:deps-recorded"); strings.Contains(got, "unratified") {
		t.Errorf("an untouched promise reads %q", got)
	}

	// The row has to be actionable: which version was granted, and which is
	// there now.
	edited := w.line("promise:tests-run")
	if strings.Count(edited, "sha256:") != 2 {
		t.Errorf("the row does not name both content versions: %q", edited)
	}
}

// The third clause, and the reason decision 0022 exists. Line endings are
// rewritten by an actual checkout with core.autocrlf=true — asserting the
// normalizer on its own would prove nothing about the path a corpus takes from
// git to a hash.
func TestACRLFCheckoutDoesNotDeRatifyTheCorpus(t *testing.T) {
	git := gitOrSkip(t)
	w := newWorkspace(t, map[string]string{
		"tests-run":     "every package has a test",
		"docs-are-true": "the deck matches main.go",
	})

	// Deliberately no .gitattributes. Decision 0022 requires the hash to hold
	// whether or not one was present, honoured, or reachable — a guarantee that
	// depends on every checkout being configured correctly is not a guarantee.
	git(w.root, "init", "-q")
	git(w.root, "config", "user.email", "test@example.com")
	git(w.root, "config", "user.name", "test")
	git(w.root, "config", "core.autocrlf", "false")
	git(w.root, "add", "-A")
	git(w.root, "commit", "-qm", "the corpus")

	for _, id := range []string{"promise:tests-run", "promise:docs-are-true"} {
		if code, _, stderr := w.run("ratify", id); code != exitOK {
			t.Fatalf("ratify %s exited %d: %s", id, code, stderr)
		}
	}
	git(w.root, "add", "-A")
	git(w.root, "commit", "-qm", "the ledger")

	// A second checkout of the same commit, on a machine configured the way a
	// Windows machine is configured.
	other := filepath.Join(t.TempDir(), "checkout")
	git("", "clone", "-q", w.root, other)
	git(other, "config", "core.autocrlf", "true")

	// Config alone does not re-materialize files that are already there, so
	// remove the tracked ones and let git write them back under the new rule.
	tracked := gitOut(t, git, other, "ls-files")
	for _, name := range strings.Fields(tracked) {
		if err := os.Remove(filepath.Join(other, filepath.FromSlash(name))); err != nil {
			t.Fatal(err)
		}
	}
	git(other, "checkout", "--", ".")

	// The premise, checked rather than assumed. A test that passed because
	// autocrlf did nothing would be worse than one that failed.
	src, err := os.ReadFile(filepath.Join(other, "docs", "promise", "tests-run.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "\r\n") {
		t.Fatalf("the checkout has no CRLF in it, so this test proves nothing about a CRLF checkout")
	}

	checked := &workspace{t: t, root: other}
	code, stdout, stderr := checked.run("status")
	if code != exitOK {
		t.Fatalf("status exited %d: %s", code, stderr)
	}
	if strings.Contains(stdout, "unratified") {
		t.Fatalf("a checkout with core.autocrlf=true de-ratified the corpus:\n%s", stdout)
	}
	mustContain(t, "status", stdout, "promise:tests-run", "promise:docs-are-true", "2 ratified")
}

// The same guarantee without git in the way, and wider: every difference decision
// 0022 names, applied to the bytes on disk at once. This one fires wherever the
// tests run.
func TestRewritingLineEndingsAndUnicodeFormDoesNotDeRatify(t *testing.T) {
	w := newWorkspace(t, map[string]string{"tests-run": "every package has a test"})
	if code, _, stderr := w.run("ratify", "promise:tests-run"); code != exitOK {
		t.Fatalf("ratify exited %d: %s", code, stderr)
	}

	path := filepath.Join(w.root, "docs", "promise", "tests-run.md")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// A checkout on a machine that disagrees about all four: line endings, a
	// byte-order mark, trailing whitespace, and the final newline.
	text := strings.ReplaceAll(string(src), "\n", "\r\n")
	text = strings.ReplaceAll(text, " has a test\r\n", " has a test  \t\r\n")
	text = "\uFEFF" + strings.TrimRight(text, "\r\n") + "\r\n\r\n\r\n"
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := w.line("promise:tests-run"); strings.Contains(got, "unratified") {
		t.Fatalf("rewriting line endings, adding a BOM and trailing whitespace dropped standing: %q", got)
	}

	// And the guard: one changed word still does.
	if err := os.WriteFile(path, []byte(strings.Replace(text, "every package", "some packages", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := w.line("promise:tests-run"); !strings.Contains(got, "unratified") {
		t.Fatalf("changing a word in the statement did not drop standing: %q", got)
	}
}

// Standing is created by one act and retired by another, and both are a
// person's. Deleting the file is neither.
func TestReleaseIsHowAPromiseStopsBeingHeldTo(t *testing.T) {
	w := newWorkspace(t, map[string]string{"tests-run": "every package has a test"})
	if code, _, stderr := w.run("ratify", "promise:tests-run"); code != exitOK {
		t.Fatalf("ratify exited %d: %s", code, stderr)
	}

	if err := os.Remove(filepath.Join(w.root, "docs", "promise", "tests-run.md")); err != nil {
		t.Fatal(err)
	}
	if got := w.line("promise:tests-run"); !strings.Contains(got, "absent") {
		t.Fatalf("a deleted ratified promise reads %q, and deletion is not a release", got)
	}

	// Releasing it is what retires it, and it has to work on an artifact whose
	// file is gone, since that is the case it exists for.
	code, _, stderr := w.run("release", "promise:tests-run", "--note", "superseded")
	if code != exitOK {
		t.Fatalf("release of an absent artifact exited %d: %s", code, stderr)
	}
	if got := w.line("promise:tests-run"); !strings.Contains(got, "released") {
		t.Fatalf("after release the register says %q", got)
	}
}

// An act that would change nothing is refused rather than recorded, in both
// directions. The ledger is permanent, and a line describing an act with no
// effect cannot be told apart later from one that mattered.
func TestAnActThatChangesNothingIsNotRecorded(t *testing.T) {
	w := newWorkspace(t, map[string]string{"tests-run": "every package has a test"})
	w.run("ratify", "promise:tests-run")

	before := w.decisions()
	code, stdout, _ := w.run("ratify", "promise:tests-run")
	if code != exitOK {
		t.Fatalf("ratifying twice exited %d", code)
	}
	mustContain(t, "the second ratify", stdout, "already ratified", "Nothing recorded")
	if after := w.decisions(); after != before {
		t.Fatalf("ratifying twice appended a second record:\n%s", after)
	}

	code, stderr := 0, ""
	code, _, stderr = w.run("release", "promise:nothing-here")
	if code != exitUsage {
		t.Errorf("releasing an artifact nobody ratified exited %d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr, "no standing to retire") && !strings.Contains(stderr, "no artifact") {
		t.Errorf("the refusal does not say why: %q", stderr)
	}
}

func (w *workspace) decisions() string {
	w.t.Helper()
	src, err := os.ReadFile(filepath.Join(w.root, ".kept", "ledger", "decisions.jsonl"))
	if err != nil {
		w.t.Fatal(err)
	}
	return string(src)
}

// An artifact with nothing binding cannot be ratified, and a refusal is the
// right answer: standing granted against a hash over nothing would survive
// every edit ever made to the file.
func TestRatifyRefusesAnArtifactWithNothingBinding(t *testing.T) {
	w := newWorkspace(t, nil)
	w.put("docs/_schema/desire.yaml", "type: desire\nhash: []\nfields:\n  want: { type: string }\n")
	w.put("docs/desire/d.md", "---\nid: d\nfields:\n  want: something\n---\n\nnote")

	code, _, stderr := w.run("ratify", "desire:d")
	if code != exitUsage {
		t.Fatalf("ratifying a desire exited %d, want %d", code, exitUsage)
	}
	mustContain(t, "the refusal", stderr, "hash: []", "cannot be ratified")
}

// A torn line is reported and the rest of the ledger is still read, because the
// line most likely to be torn is the last one written before a crash.
func TestStatusReportsQuarantinedLinesAndReadsTheRest(t *testing.T) {
	w := newWorkspace(t, map[string]string{"tests-run": "every package has a test"})
	w.run("ratify", "promise:tests-run")

	path := filepath.Join(w.root, ".kept", "ledger", "decisions.jsonl")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(src, []byte(`{"v":1,"kind":"rati`+"\n")...), 0o644); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := w.run("status")
	if code != exitOK {
		t.Fatalf("status exited %d with a torn line in the ledger: %s", code, stderr)
	}
	mustContain(t, "the report", stderr, "could not be read", "decisions.jsonl:2")
	if !strings.Contains(stdout, "ratified") || strings.Contains(w.line("promise:tests-run"), "unratified") {
		t.Fatalf("a torn line stopped the intact records being read:\n%s", stdout)
	}
}

// A corpus nobody has ratified anything in, and a clone that never fetched the
// ledger, look identical in a table. See docs/decision/0016.
func TestStatusSaysWhenThereIsNoLedgerAtAll(t *testing.T) {
	w := newWorkspace(t, map[string]string{"tests-run": "every package has a test"})
	code, stdout, stderr := w.run("status")
	if code != exitOK {
		t.Fatalf("status exited %d: %s", code, stderr)
	}
	mustContain(t, "status", stdout, "no decision stream", "nothing has ever been ratified")

	w.run("ratify", "promise:tests-run")
	if _, stdout, _ := w.run("status"); strings.Contains(stdout, "no decision stream") {
		t.Fatal("a ledger with a record in it still reports as absent")
	}
}

// gitOrSkip returns a runner, or skips. Every clause of the done-condition has
// a test that fires without git; this one is the end-to-end version of the CRLF
// clause and needs the real thing.
func gitOrSkip(t *testing.T) func(dir string, args ...string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH; TestRewritingLineEndingsAndUnicodeFormDoesNotDeRatify covers the same guarantee")
	}
	return func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		// A repository written by a test must not read the operator's own git
		// configuration, or an autocrlf set globally decides the outcome.
		cmd.Env = append(os.Environ(),
			"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
}

func gitOut(t *testing.T, _ func(string, ...string), dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return string(out)
}
