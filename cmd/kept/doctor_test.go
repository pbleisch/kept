package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/ledger"
)

// adopting is a repository in the state every corpus is in before it is
// adopted: files in a type directory, and nothing naming them.
func adopting(t *testing.T, files map[string]string) *workspace {
	t.Helper()
	w := &workspace{t: t, root: t.TempDir()}
	w.put(".kept/config.yaml", configFile)
	w.put("docs/_schema/decision.yaml", decisionSchemaFile)
	for name, body := range files {
		w.put("docs/"+name, body)
	}
	return w
}

func (w *workspace) read(name string) string {
	w.t.Helper()
	src, err := os.ReadFile(filepath.Join(w.root, filepath.FromSlash(name)))
	if err != nil {
		w.t.Fatal(err)
	}
	return string(src)
}

const adr = "# 0001 — A thing was decided\n\n**Status** accepted\n\n" +
	"## Context\n\nwhat led here\n\n## Decision\n\nthe thing\n\n## Consequences\n\nwhat it cost\n"

// The adoption path, end to end: a file nothing identifies becomes an artifact
// that can be ratified, and the id comes from the filename because that is the
// only thing on hand that a person chose. See docs/decision/0042.
func TestFixWritesTheIdentityAnAdoptedFileIsMissing(t *testing.T) {
	w := adopting(t, map[string]string{"decision/0001-a-thing-was-decided.md": adr})

	code, stdout, stderr := w.run("doctor", "--fix")
	if code != exitOK {
		t.Fatalf("doctor --fix exited %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, "0001-a-thing-was-decided.md") {
		t.Errorf("doctor --fix did not name the file it changed:\n%s", stdout)
	}

	got := w.read("docs/decision/0001-a-thing-was-decided.md")
	want := "---\nid: 0001-a-thing-was-decided\ntype: decision\ntitle: 0001 — A thing was decided\n---\n\n" + adr
	if got != want {
		t.Errorf("the file is\n%q\nand should be\n%q", got, want)
	}

	if code, _, stderr := w.run("ratify", "decision:0001-a-thing-was-decided", "--by", "the operator"); code != exitOK {
		t.Fatalf("the adopted file could not be ratified: %d %s", code, stderr)
	}
	if line := w.line("decision:0001-a-thing-was-decided"); !strings.Contains(line, "ratified") {
		t.Errorf("after adoption and ratification the register says %q", line)
	}
}

// A file that already has an id is not a finding, so a second --fix has nothing
// to do. The failure this catches is a fixer that prepends a second block, which
// would leave the first one as prose and the artifact unreadable.
func TestFixDoesNothingTheSecondTime(t *testing.T) {
	w := adopting(t, map[string]string{"decision/0001-a-thing-was-decided.md": adr})

	w.run("doctor", "--fix")
	once := w.read("docs/decision/0001-a-thing-was-decided.md")
	code, stdout, _ := w.run("doctor", "--fix")
	twice := w.read("docs/decision/0001-a-thing-was-decided.md")

	if once != twice {
		t.Errorf("a second --fix rewrote the file:\n%q", twice)
	}
	if code != exitOK || strings.Contains(stdout, "wrote:") {
		t.Errorf("a second --fix reported writing something:\n%s", stdout)
	}
}

// 0029 permits one line into a block a person wrote, and nothing more. A block
// that exists is one its author wrote, and a key they left out may be one they
// left out on purpose.
func TestFixInsertsOneLineIntoABlockThatExists(t *testing.T) {
	authored := "---\ntype: decision\nfields:\n  statement: a thing was decided\nkeeper: unknown to every schema\n---\n\n" + adr
	w := adopting(t, map[string]string{"decision/0001-a-thing-was-decided.md": authored})

	if code, _, stderr := w.run("doctor", "--fix"); code != exitOK {
		t.Fatalf("doctor --fix exited %d: %s", code, stderr)
	}

	got := w.read("docs/decision/0001-a-thing-was-decided.md")
	want := "---\nid: 0001-a-thing-was-decided\n" + strings.TrimPrefix(authored, "---\n")
	if got != want {
		t.Errorf("the file is\n%q\nand should be\n%q", got, want)
	}
	if strings.Contains(got, "title:") {
		t.Error("--fix added a title to a block its author wrote")
	}
	if !strings.Contains(got, "keeper: unknown to every schema") {
		t.Error("a field nobody declared did not survive --fix")
	}
}

// Nothing already in the file moves. The file's own line endings are used for
// what is inserted and a byte-order mark stays in front, because a --fix that
// normalized either would be re-emitting a document kept parsed — the round
// trip 0029 retired.
func TestFixKeepsLineEndingsAndAByteOrderMark(t *testing.T) {
	crlf := "\ufeff" + strings.ReplaceAll(adr, "\n", "\r\n")
	w := adopting(t, map[string]string{"decision/0001-a-thing-was-decided.md": crlf})

	if code, _, stderr := w.run("doctor", "--fix"); code != exitOK {
		t.Fatalf("doctor --fix exited %d: %s", code, stderr)
	}

	got := w.read("docs/decision/0001-a-thing-was-decided.md")
	if !strings.HasPrefix(got, "\ufeff---\r\n") {
		t.Errorf("the mark or the line endings did not survive: %q", got[:24])
	}
	if strings.Contains(strings.ReplaceAll(got, "\r\n", ""), "\n") {
		t.Error("--fix wrote LF into a CRLF file")
	}
	if !strings.Contains(got, "\ufeff---\r\nid: 0001-a-thing-was-decided\r\n") {
		t.Errorf("the block is not what was expected:\n%q", got)
	}
}

// A filename that cannot be a path component cannot be an id, and writing one
// would put a value in the file that the next read rejects. It is refused by
// name rather than mangled into something that works.
func TestFixRefusesAFilenameThatCannotBeASlug(t *testing.T) {
	w := adopting(t, map[string]string{"decision/a name with spaces.md": adr})

	code, stdout, _ := w.run("doctor", "--fix")
	if code != exitOK {
		t.Fatalf("doctor --fix exited %d", code)
	}
	if !strings.Contains(stdout, "left alone") || !strings.Contains(stdout, "whitespace") {
		t.Errorf("--fix did not refuse a filename that cannot be a slug:\n%s", stdout)
	}
	if got := w.read("docs/decision/a name with spaces.md"); got != adr {
		t.Errorf("the file was written anyway:\n%q", got)
	}
}

// The remit is identity, and this is the guard that says so. A required field
// nobody wrote is content, it is reported by kept list, and doctor must stay
// quiet about it — because a doctor that reported content would be a doctor
// somebody would eventually ask to fix it.
func TestDoctorReportsIdentityAndNotContent(t *testing.T) {
	w := adopting(t, map[string]string{
		"_schema/canon.yaml": "type: canon\nhash: [statement]\nfields:\n  statement: { type: string, required: true }\nbody: detail\n",
		"canon/nothing-said.md": "---\nid: nothing-said\n---\n\nno statement, which every schema check reports\n" +
			"\nand a [[canon:nowhere]] that leads nowhere\n",
	})

	code, stdout, _ := w.run("doctor")
	if code != exitOK {
		t.Fatalf("doctor exited %d", code)
	}
	if strings.Contains(stdout, "field-violation") || strings.Contains(stdout, "broken-link") {
		t.Errorf("doctor reported content, which is kept list's:\n%s", stdout)
	}
	if !strings.Contains(stdout, "nothing wrong with what names them") {
		t.Errorf("doctor did not report a corpus whose names are all fine:\n%s", stdout)
	}

	// The other half, or the check above passes on a doctor that reports
	// nothing at all.
	if _, listed, _ := w.run("list"); !strings.Contains(listed, "field-violation") {
		t.Errorf("kept list did not report the content problem either:\n%s", listed)
	}
}

// 0032 named this check and left it unbuilt. Standing whose content cannot be
// found is a definite finding, and 0027 puts it in the known-false half — never
// read as a release, because an absence cannot be told from a bad merge.
func TestDoctorReportsStandingWhoseContentIsGone(t *testing.T) {
	w := adopting(t, map[string]string{"decision/0001-a-thing-was-decided.md": adr})
	w.run("doctor", "--fix")
	if code, _, stderr := w.run("ratify", "decision:0001-a-thing-was-decided", "--by", "the operator"); code != exitOK {
		t.Fatalf("ratify exited %d: %s", code, stderr)
	}

	if err := os.Remove(filepath.Join(w.root, "docs", "decision", "0001-a-thing-was-decided.md")); err != nil {
		t.Fatal(err)
	}

	code, stdout, _ := w.run("doctor")
	if code != exitDrift {
		t.Errorf("doctor exited %d for standing with no content, want %d", code, exitDrift)
	}
	if !strings.Contains(stdout, "standing and no content") ||
		!strings.Contains(stdout, "decision:0001-a-thing-was-decided") {
		t.Errorf("doctor did not name the artifact whose content is gone:\n%s", stdout)
	}
}

// commandPromise writes a promise that runs a program and extracts the fields
// named. Whether one of them reads `exit` is the whole subject below.
func commandPromise(slug string, fields map[string]string) string {
	lines := ""
	for _, name := range sortedKeysOf(fields) {
		// Single-quoted, because an extractor routinely contains a double
		// quote and a YAML scalar that swallowed it would make this test
		// about the fixture rather than about the finding.
		lines += "        " + name + ": '" + strings.ReplaceAll(fields[name], "'", "''") + "'\n"
	}
	return "---\nid: " + slug + "\ntitle: " + slug + `
fields:
  statement: the tree vets clean
  gather:
    vet:
      gatherer: command
      run: "go vet ./..."
      fields:
` + lines + `  hold_when: obs.vet.ok
  trigger: cadence
---

Why: because a promise nobody can check is not one.
`
}

func sortedKeysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// docs/decision/0050: a `command` gatherer whose declared fields never
// reference `exit` ran a program and never looked at whether it worked.
//
// It is deliberately broken here, because the corpus this was written in has
// no such promise — which is the next test.
func TestDoctorReportsAnInstrumentNobodyChecked(t *testing.T) {
	w := &workspace{t: t, root: t.TempDir()}
	w.put(".kept/config.yaml", configFile)
	w.put("docs/_schema/promise.yaml", schemaFile)
	w.put("docs/promise/loose.md", commandPromise("loose", map[string]string{
		"ok": `not ("VIOLATION" in lines(stdout))`,
	}))

	code, stdout, stderr := w.run("doctor")
	if code != exitOK {
		t.Fatalf("doctor exited %d: %s", code, stderr)
	}
	for _, want := range []string{"loose", "vet", "go vet ./...", "exit"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("the finding does not name %s:\n%s", want, stdout)
		}
	}

	// It is a finding and never an error: nothing is refused, the corpus still
	// loads, and kept list still says nothing is wrong. wc -l legitimately
	// ignores exit status.
	if code, out, _ := w.run("list"); code != exitOK || !strings.Contains(out, "nothing wrong") {
		t.Errorf("an unchecked instrument made the corpus wrong: %d\n%s", code, out)
	}

	// And the same promise reading exit anywhere in any declared field is not
	// a finding.
	w.put("docs/promise/loose.md", commandPromise("loose", map[string]string{
		"ok":       `exit == 0 and not ("VIOLATION" in lines(stdout))`,
		"findings": `count(lines(stdout), l => l != "")`,
	}))
	if _, stdout, _ := w.run("doctor"); strings.Contains(stdout, "never looked at") {
		t.Errorf("a promise that reads exit is reported anyway:\n%s", stdout)
	}
}

// The finding reads the parsed expression and never its text. A field named
// exit_code, a string "exit" and a record field called exit are three
// different things, and a substring match reports all three.
func TestTheUncheckedInstrumentFindingReadsTheExpressionAndNotItsText(t *testing.T) {
	for _, c := range []struct {
		why    string
		fields map[string]string
		found  bool
	}{
		{"a field named for exit and not reading it",
			map[string]string{"exit_code": `count(lines(stdout), l => l != "")`}, true},
		{"the word in a string",
			map[string]string{"ok": `"exit" in lines(stdout)`}, true},
		{"the root itself", map[string]string{"ok": "exit == 0"}, false},
		{"the root inside a fold",
			map[string]string{"ok": `count(lines(stdout), l => l != "") == 0 and exit == 0`}, false},
	} {
		w := &workspace{t: t, root: t.TempDir()}
		w.put(".kept/config.yaml", configFile)
		w.put("docs/_schema/promise.yaml", schemaFile)
		w.put("docs/promise/loose.md", commandPromise("loose", c.fields))

		_, stdout, _ := w.run("doctor")
		if found := strings.Contains(stdout, "never looked at"); found != c.found {
			t.Errorf("%s: reported=%v, want %v\n%s", c.why, found, c.found, stdout)
		}
	}
}

// A check whose first run reports the corpus it was written in is a check
// written to fit. the-tree-vets-clean already carries ok: "exit == 0".
func TestTheUncheckedInstrumentFindingFiresOnNothingHere(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "docs", "promise")); err != nil {
		t.Skip("not running inside the repository's own tree")
	}

	c, code := open(filepath.Join(root, "docs"), os.Stderr)
	if code != exitOK {
		t.Fatalf("this repository's corpus would not load: %d", code)
	}
	if loose := unchecked(c); len(loose) != 0 {
		t.Errorf("this corpus reports %d unchecked instruments: %+v", len(loose), loose)
	}
}

// The other finding docs/decision/0050 admits, and the one this repository
// paid for. An origin that force-fetches the ledger is well-formed — git
// accepts it, kept accepts it, nothing refuses it — and unwise, because an
// ordinary `git fetch` from a remote that has fallen behind then erases acts
// with exit 0 and no reflog. Nothing else in kept reads it once a workspace is
// adopted, which is the argument for giving it a reader here. See
// docs/decision/0016.
func TestDoctorReportsAnOriginThatForceFetchesTheLedger(t *testing.T) {
	git := gitOrSkip(t)
	w := newWorkspace(t, map[string]string{"one": "a promise"})
	git(w.root, "init", "-q")

	if _, stdout, _ := w.run("doctor"); strings.Contains(stdout, "force-fetches") {
		t.Fatalf("a repository with no ledger refspec at all is reported:\n%s", stdout)
	}
	git(w.root, "config", "--add", "remote.origin.fetch", ledger.FetchRefspec)
	if _, stdout, _ := w.run("doctor"); strings.Contains(stdout, "force-fetches") {
		t.Fatalf("the unforced refspec kept writes is reported:\n%s", stdout)
	}

	git(w.root, "config", "--add", "remote.origin.fetch", ledger.ForcedFetchRefspec)
	code, stdout, _ := w.run("doctor")
	if !strings.Contains(stdout, "force-fetches") || !strings.Contains(stdout, ledger.ForcedFetchRefspec) {
		t.Fatalf("doctor did not report the forced refspec:\n%s", stdout)
	}
	if !strings.Contains(stdout, "kept init") {
		t.Errorf("the finding did not say what corrects it:\n%s", stdout)
	}
	// 0027's table is unchanged by it, the same as an unchecked instrument:
	// this is a judgment about wisdom, not a commitment and not evidence.
	if code != exitOK {
		t.Errorf("the finding changed doctor's exit code: %d", code)
	}

	if _, err := ledger.ConfigureFetch(w.root); err != nil {
		t.Fatal(err)
	}
	if _, stdout, _ := w.run("doctor"); strings.Contains(stdout, "force-fetches") {
		t.Fatalf("the finding survived the correction:\n%s", stdout)
	}
}

// A workspace that is not a repository has no origin, and doctor is a verb a
// manuscript author runs. Silence, not an error and not a finding.
func TestDoctorSaysNothingAboutFetchOutsideARepository(t *testing.T) {
	w := newWorkspace(t, map[string]string{"one": "a promise"})
	code, stdout, stderr := w.run("doctor")
	if code != exitOK || strings.Contains(stdout, "force-fetches") || strings.Contains(stderr, "remote.origin.fetch") {
		t.Fatalf("doctor exited %d outside a repository:\n%s\n%s", code, stdout, stderr)
	}
}
