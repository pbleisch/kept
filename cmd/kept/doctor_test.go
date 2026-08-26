package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/register"
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

// The other finding docs/decision/0050 admits, and the one this repository paid
// for. An origin whose ledger refspec writes the ledger ref is well-formed —
// git accepts it, kept accepts it, nothing refuses it — and unwise, because it
// makes the network the writer of the durable record: forced, an ordinary
// `git fetch` from a remote that has fallen behind erases acts with exit 0 and
// no reflog; unforced, every bare fetch is refused while the ledger is ahead,
// which is the resting state. Nothing else in kept reads it once a workspace is
// adopted, which is the argument for giving it a reader here. See
// docs/decision/0016 and docs/decision/0062.
func TestDoctorReportsAnOriginThatFetchesStraightIntoTheLedger(t *testing.T) {
	const finding = "straight into itself"
	for _, spelling := range ledger.DirectRefspecs {
		t.Run(spelling, func(t *testing.T) {
			git := gitOrSkip(t)
			w := newWorkspace(t, map[string]string{"one": "a promise"})
			git(w.root, "init", "-q")

			if _, stdout, _ := w.run("doctor"); strings.Contains(stdout, finding) {
				t.Fatalf("a repository with no ledger refspec at all is reported:\n%s", stdout)
			}
			git(w.root, "config", "--add", "remote.origin.fetch", ledger.FetchRefspec)
			if _, stdout, _ := w.run("doctor"); strings.Contains(stdout, finding) {
				t.Fatalf("the tracking refspec kept writes is reported:\n%s", stdout)
			}

			git(w.root, "config", "--add", "remote.origin.fetch", spelling)
			code, stdout, _ := w.run("doctor")
			if !strings.Contains(stdout, finding) || !strings.Contains(stdout, spelling) {
				t.Fatalf("doctor did not report the direct refspec:\n%s", stdout)
			}
			if !strings.Contains(stdout, "kept init") {
				t.Errorf("the finding did not say what corrects it:\n%s", stdout)
			}
			// 0027's table is unchanged by it, the same as an unchecked
			// instrument: this is a judgment about wisdom, not a commitment and
			// not evidence.
			if code != exitOK {
				t.Errorf("the finding changed doctor's exit code: %d", code)
			}

			if _, _, _, err := ledger.ConfigureFetch(w.root); err != nil {
				t.Fatal(err)
			}
			if _, stdout, _ := w.run("doctor"); strings.Contains(stdout, finding) {
				t.Fatalf("the finding survived the correction:\n%s", stdout)
			}
		})
	}
}

// A workspace that is not a repository has no origin, and doctor is a verb a
// manuscript author runs. Silence, not an error and not a finding.
func TestDoctorSaysNothingAboutFetchOutsideARepository(t *testing.T) {
	w := newWorkspace(t, map[string]string{"one": "a promise"})
	code, stdout, stderr := w.run("doctor")
	if code != exitOK || strings.Contains(stdout, "straight into itself") || strings.Contains(stderr, "remote.origin.fetch") {
		t.Fatalf("doctor exited %d outside a repository:\n%s\n%s", code, stdout, stderr)
	}
}

// A grant in force for a content version this corpus does not hold is the
// ordinary state of every artifact that has ever been revised, so the finding
// docs/decision/0063 admits cannot key on that. It keys on whether the commit
// the act was performed in is one this tree can reach.
//
// This is the case that is not reachable: a task branch ratified, and main
// never took it. Standing is untouched — the grant is for bytes main does not
// have, so nothing here is ratified against them — and that is exactly why it
// had no reader until now.
func TestDoctorReportsAGrantForContentThatLandedNowhere(t *testing.T) {
	const finding = "landed nowhere this tree can reach"
	w, git := repository(t, map[string]string{"one": "a promise"})
	w.ratify(t, "promise:one")

	if _, stdout, _ := w.run("doctor"); strings.Contains(stdout, finding) {
		t.Fatalf("a grant made on the branch it is read from is reported:\n%s", stdout)
	}

	git(w.root, "checkout", "-q", "-b", "task")
	w.put("docs/promise/one.md", promiseAt("one", "a promise, revised on a branch"))
	git(w.root, "commit", "-qam", "revise")
	w.ratify(t, "promise:one")
	if _, stdout, _ := w.run("doctor"); strings.Contains(stdout, finding) {
		t.Fatalf("standing on the branch that granted it, the grant is reported:\n%s", stdout)
	}

	// The branch is declined: main never takes the commit, and the grant for a
	// wording that exists nowhere main can see stays in force forever.
	git(w.root, "checkout", "-q", "-")
	code, stdout, _ := w.run("doctor")
	if !strings.Contains(stdout, finding) || !strings.Contains(stdout, "promise:one") {
		t.Fatalf("doctor did not report the grant:\n%s", stdout)
	}
	if !strings.Contains(stdout, "decision 0063") {
		t.Errorf("the finding does not say where the reasoning is:\n%s", stdout)
	}
	// 0027's table is unchanged, the same as every other finding 0050 admits:
	// this is a judgment about wisdom, not a commitment and not evidence.
	if code != exitOK {
		t.Errorf("the finding changed doctor's exit code: %d", code)
	}
	if line := w.line("promise:one"); !strings.Contains(line, "ratified") || strings.Contains(line, "unratified") {
		t.Errorf("the extra grant changed what main is ratified at: %q", line)
	}
}

// The other side of the same check, and the reason equality would not do: a
// grant for a wording that landed once and was revised is deliberately still in
// force, because standing attaches to a content version and reverting to that
// wording is meant to be ratified again with nobody granting anything.
//
// A check that fires on that fires on every revised artifact in every corpus.
func TestDoctorSaysNothingAboutAGrantForAWordingThatLandedOnce(t *testing.T) {
	w, git := repository(t, map[string]string{"one": "a promise"})
	w.ratify(t, "promise:one")

	w.put("docs/promise/one.md", promiseAt("one", "a promise, revised"))
	git(w.root, "commit", "-qam", "revise")

	// There is something to be silent about: the earlier wording's grant is in
	// force, and the artifact reads unratified against it.
	if line := w.line("promise:one"); !strings.Contains(line, "unratified") {
		t.Fatalf("the revision left no grant for an earlier wording: %q", line)
	}
	code, stdout, _ := w.run("doctor")
	if code != exitOK || strings.Contains(stdout, "landed nowhere") || strings.Contains(stdout, "uncommitted tree") {
		t.Fatalf("a wording that landed once is reported (%d):\n%s", code, stdout)
	}

	// And it stays silent once the revision is ratified too, which is the state
	// this repository's own corpus is in for every decision ever revised.
	w.ratify(t, "promise:one")
	if _, stdout, _ := w.run("doctor"); strings.Contains(stdout, "landed nowhere") {
		t.Fatalf("ratifying the revision made the earlier grant a finding:\n%s", stdout)
	}
}

// Every act written before docs/decision/0063 names no world, and that is the
// whole existing ledger. A missing world is *nobody recorded one*, which is not
// *it never landed*, and reading it as the finding would report a corpus's
// entire history the first time this ran.
//
// Two hand-written acts differing in one field, because a test that only
// asserts silence is a test that passes when the check is gone.
func TestDoctorSaysNothingAboutAnActThatNamesNoWorld(t *testing.T) {
	w, _ := repository(t, map[string]string{"one": "a promise"})
	nowhere := strings.Repeat("f", 40)

	w.append(t, legacyAct("promise:one", strings.Repeat("a", 64), ""))
	if _, stdout, _ := w.run("doctor"); strings.Contains(stdout, "landed nowhere") {
		t.Fatalf("an act that names no world is reported:\n%s", stdout)
	}

	w.append(t, legacyAct("promise:one", strings.Repeat("b", 64), nowhere))
	_, stdout, _ := w.run("doctor")
	if !strings.Contains(stdout, "landed nowhere") {
		t.Fatalf("the same act with a world nothing can reach is not reported:\n%s", stdout)
	}
	if strings.Count(stdout, "promise:one") != 1 {
		t.Errorf("the world-less act was reported alongside it:\n%s", stdout)
	}
}

// The third case, and the one that would otherwise read as supported: the
// commit is reachable and holds none of these bytes, because they were hashed
// out of a working tree. Reachability is the only evidence an act carries that
// content landed, so it is said rather than folded into the finding above.
func TestDoctorReportsAGrantHashedOutOfAnUncommittedTree(t *testing.T) {
	const finding = "hashed out of an uncommitted tree"
	w, git := repository(t, map[string]string{"one": "a promise"})

	// Edited and ratified before it is committed, which is the ordinary
	// accident. While those bytes are what the artifact says, they are the
	// grant in force and there is nothing to report.
	w.put("docs/promise/one.md", promiseAt("one", "a promise, revised but not committed"))
	w.ratify(t, "promise:one")
	if _, stdout, _ := w.run("doctor"); strings.Contains(stdout, finding) {
		t.Fatalf("the wording the artifact says now is reported:\n%s", stdout)
	}

	// Abandoned rather than committed: the grant is now for bytes that were
	// never in the commit it names, and that commit's reachability says so.
	git(w.root, "checkout", "-q", "--", "docs/promise/one.md")
	code, stdout, _ := w.run("doctor")
	if !strings.Contains(stdout, finding) || !strings.Contains(stdout, "promise:one") {
		t.Fatalf("doctor did not report the grant:\n%s", stdout)
	}
	if strings.Contains(stdout, "landed nowhere") {
		t.Errorf("it was reported as both findings at once:\n%s", stdout)
	}
	if code != exitOK {
		t.Errorf("the finding changed doctor's exit code: %d", code)
	}
}

// Over this repository's own ledger, where every act was written before
// docs/decision/0063 and names no world at all. A check whose first run reports
// the corpus it was written in is a check written to fit, and this one is
// answering *nobody recorded it* for every grant here.
func TestTheGrantFindingFiresOnNothingHere(t *testing.T) {
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
	acts, _, err := ledger.ReadActs(ledgerStore(c))
	if err != nil {
		t.Fatal(err)
	}
	if adrift := unaccounted(register.Derive(c, acts), c.Workspace); len(adrift) != 0 {
		t.Errorf("this corpus reports %d grants it cannot account for: %+v", len(adrift), adrift)
	}
}

// repository is a workspace that is also a repository with everything in it
// committed, which is the state a person ratifies from. .kept is ignored rather
// than committed so that the ledger moving does not make the tree dirty.
func repository(t *testing.T, promises map[string]string) (*workspace, func(dir string, args ...string)) {
	t.Helper()
	git := gitOrSkip(t)
	w := newWorkspace(t, promises)
	w.put(".gitignore", ".kept/\n")
	git(w.root, "init", "-q")
	git(w.root, "add", "-A")
	git(w.root, "commit", "-qm", "the corpus")
	return w, git
}

func (w *workspace) ratify(t *testing.T, id string) {
	t.Helper()
	if code, _, stderr := w.run("ratify", id, "--by", "the operator"); code != exitOK {
		t.Fatalf("ratify %s exited %d: %s", id, code, stderr)
	}
}

// append writes one line straight into the act stream, which is how a record
// written by an earlier build is put in front of this one.
func (w *workspace) append(t *testing.T, line string) {
	t.Helper()
	if err := ledger.NewRefStore(w.root).Append(ledger.Acts, []byte(line)); err != nil {
		t.Fatal(err)
	}
}

// legacyAct is an act as an earlier build wrote one, optionally with the one
// field docs/decision/0063 adds. The hash names a wording this corpus does not
// hold, so the grant it creates is in force and dormant.
func legacyAct(artifact, digest, commit string) string {
	world := ""
	if commit != "" {
		world = `"world":{"commit":"` + commit + `","tree":"sha256:` + strings.Repeat("c", 64) + `"},`
	}
	return `{"v":1,"kind":"ratified","at":"2026-01-01T00:00:00Z","writer":"legacy","seq":1,` + world +
		`"artifact":"` + artifact + `","hash":"sha256:` + digest + `","norm":1,"by":"the operator"}`
}

// The finding beside it, and the one this repository was actually in.
//
// It passes docs/decision/0050's three conditions. Nothing refuses an origin
// that fetches no branch — git accepts it and so does kept, and kept land
// refreshes its own target's tracking ref before reading it, so decision 0015's
// in-lock check is not the caller this breaks. Nothing else in kept reports it.
// And what kept does about it is one line kept init writes.
//
// The states are what an operator can actually be holding, and the order is the
// order they arrive in: a repository with no origin at all says nothing,
// because there is nothing to track; one whose origin fetches only the ledger
// is the state adding one line where there had been none produces; and a branch
// refspec of any width answers the question.
func TestDoctorReportsAnOriginThatFetchesNoBranch(t *testing.T) {
	const finding = "origin fetches no branch"
	git := gitOrSkip(t)
	w := newWorkspace(t, map[string]string{"one": "a promise"})
	git(w.root, "init", "-q")

	if _, stdout, _ := w.run("doctor"); strings.Contains(stdout, finding) {
		t.Fatalf("a repository with no origin URL is reported:\n%s", stdout)
	}

	git(w.root, "config", "remote.origin.url", t.TempDir())
	if _, stdout, _ := w.run("doctor"); !strings.Contains(stdout, finding) {
		t.Fatalf("an origin that fetches nothing was not reported:\n%s", stdout)
	}
	git(w.root, "config", "--add", "remote.origin.fetch", ledger.FetchRefspec)
	code, stdout, _ := w.run("doctor")
	if !strings.Contains(stdout, finding) || !strings.Contains(stdout, ledger.BranchRefspec) {
		t.Fatalf("a ledger-only origin was not reported, or the finding did not name the line:\n%s", stdout)
	}
	if !strings.Contains(stdout, "kept init") {
		t.Errorf("the finding did not say what corrects it:\n%s", stdout)
	}
	// 0027's table is unchanged by it, as it is by the direct spelling above.
	if code != exitOK {
		t.Errorf("the finding changed doctor's exit code: %d", code)
	}

	if _, _, _, err := ledger.ConfigureFetch(w.root); err != nil {
		t.Fatal(err)
	}
	if _, stdout, _ := w.run("doctor"); strings.Contains(stdout, finding) {
		t.Fatalf("the finding survived the correction:\n%s", stdout)
	}

	// A narrower branch refspec somebody wrote answers it too, and is never
	// widened: the question is whether a fetch updates tracking refs, not
	// whether it updates all of them.
	other := newWorkspace(t, map[string]string{"one": "a promise"})
	git(other.root, "init", "-q")
	git(other.root, "config", "remote.origin.url", t.TempDir())
	git(other.root, "config", "--add", "remote.origin.fetch", "+refs/heads/main:refs/remotes/origin/main")
	if _, stdout, _ := other.run("doctor"); strings.Contains(stdout, finding) {
		t.Fatalf("a narrowed branch refspec is reported as no branch refspec:\n%s", stdout)
	}
}

// The outbound half of docs/decision/0062, and the state this repository was
// actually in on 2026-08-25: three ledger commits nobody had published, with
// git status clean and nothing saying so.
//
// It passes docs/decision/0050's three conditions. Nothing refuses an
// unpublished ledger — every verb keeps answering, and must, because publishing
// is not a precondition of recording. Nothing else in kept reports it: kept
// ledger sync is the inbound half and says nothing about the outbound one. And
// kept does nothing about it, deliberately: a push would be the first network
// call kept ever makes, so the finding hands over the command instead.
//
// The states are the ones an operator can be holding, in the order they arrive:
// no origin, no ledger, a ledger origin has heard of, and one it has not.
func TestDoctorReportsALedgerOriginDidNotHaveWhenThisRepositoryLastHeardFromIt(t *testing.T) {
	const finding = "did not have when this repository last heard"
	w, git := repository(t, map[string]string{"one": "a promise", "two": "another promise"})

	// No origin is nowhere to publish to, whatever the ledger holds.
	w.ratify(t, "promise:one")
	if _, stdout, _ := w.run("doctor"); strings.Contains(stdout, finding) {
		t.Fatalf("a repository with no origin URL is reported:\n%s", stdout)
	}

	git(w.root, "config", "remote.origin.url", t.TempDir())
	held := strings.TrimSpace(gitOut(t, git, w.root, "rev-parse", ledger.Ref))
	git(w.root, "update-ref", ledger.UpstreamRef, held)
	if _, stdout, _ := w.run("doctor"); strings.Contains(stdout, finding) {
		t.Fatalf("a ledger origin already holds is reported:\n%s", stdout)
	}

	w.ratify(t, "promise:two")
	code, stdout, _ := w.run("doctor")
	if !strings.Contains(stdout, finding) {
		t.Fatalf("an unpublished ledger was not reported:\n%s", stdout)
	}
	// The count is commits rather than acts — one ledger commit is one write —
	// so it is asked of git rather than assumed from the number of verbs run.
	ahead := strings.TrimSpace(gitOut(t, git, w.root, "rev-list", "--count", held+".."+ledger.Ref))
	mustContain(t, "doctor", stdout, ledger.PushCommand, ledger.Ref, ledger.UpstreamRef, ahead+" commit")
	// 0027's table is unchanged by it, the same as the two git settings beside
	// it: this is a judgment about wisdom, not a commitment and not evidence.
	if code != exitOK {
		t.Errorf("the finding changed doctor's exit code: %d", code)
	}

	// And it clears the moment origin has heard of them, which the push the
	// finding names is what does — git updates the tracking ref on a push it
	// accepts, so the same two refs answer.
	git(w.root, "update-ref", ledger.UpstreamRef, strings.TrimSpace(gitOut(t, git, w.root, "rev-parse", ledger.Ref)))
	if _, stdout, _ := w.run("doctor"); strings.Contains(stdout, finding) {
		t.Fatalf("the finding survived the publication:\n%s", stdout)
	}
}

// A repository with an origin and a ledger and no upstream value at all. It is
// not "you are ahead": nobody has asked origin anything, so whether origin
// holds these acts is unknown here — the same distinction *nothing has
// answered is not an answer of unknown* draws one level up.
//
// Two situations produce it and no local read separates them, so the finding
// names both and names the fetch that tells them apart.
func TestDoctorSeparatesALedgerNobodyHasFetchedAgainstFromAnUnpublishedOne(t *testing.T) {
	const unfetched = "nothing here has heard from origin about the ledger"
	const ahead = "did not have when this repository last heard"
	w, git := repository(t, map[string]string{"one": "a promise"})
	git(w.root, "config", "remote.origin.url", t.TempDir())

	// A ledger nobody has written is nothing to publish, not an unfetched one.
	if _, stdout, _ := w.run("doctor"); strings.Contains(stdout, unfetched) || strings.Contains(stdout, ahead) {
		t.Fatalf("a repository with no ledger at all is reported:\n%s", stdout)
	}

	w.ratify(t, "promise:one")
	code, stdout, _ := w.run("doctor")
	if !strings.Contains(stdout, unfetched) {
		t.Fatalf("a ledger with no upstream value was not reported:\n%s", stdout)
	}
	if strings.Contains(stdout, ahead) {
		t.Fatalf("an unfetched ledger was reported as an unpublished one:\n%s", stdout)
	}
	mustContain(t, "doctor", stdout, "git fetch origin", ledger.PushCommand, "unknown here")
	if code != exitOK {
		t.Errorf("the finding changed doctor's exit code: %d", code)
	}
}

// A fork is reported once, by the finding that owns it. Origin would refuse
// this push anyway, and printing "publish these" beside "union these two tips
// by hand" is two instructions where only one can be followed.
func TestAForkedLedgerIsReportedAsAForkAndNotAsAnUnpublishedOne(t *testing.T) {
	w, git := repository(t, map[string]string{"one": "a promise"})
	git(w.root, "config", "remote.origin.url", t.TempDir())
	w.ratify(t, "promise:one")
	// The repository's own commit reaches nothing the ledger does and is
	// reached by nothing it holds, which is what two clones that both appended
	// look like from here.
	git(w.root, "update-ref", ledger.UpstreamRef, strings.TrimSpace(gitOut(t, git, w.root, "rev-parse", "HEAD")))

	code, stdout, _ := w.run("doctor")
	if !strings.Contains(stdout, "have both moved") {
		t.Fatalf("a forked ledger was not reported as a fork:\n%s", stdout)
	}
	if strings.Contains(stdout, "did not have when this repository last heard") {
		t.Fatalf("a fork was also reported as an unpublished ledger:\n%s", stdout)
	}
	if code != exitOK {
		t.Errorf("the finding changed doctor's exit code: %d", code)
	}
}

// The claim the finding makes, proven by making it false about origin.
//
// Origin really holds the acts, and the upstream ref still says it does not, so
// doctor reports them as unpublished. That is the wording earning its keep:
// **origin did not have these when this repository last heard from it** is
// true, and *origin does not have these* would be false. A doctor that fetched
// would report nothing here, so the finding is also the proof that it does not.
func TestTheUnpublishedFindingReadsTwoLocalRefsAndFetchesNothing(t *testing.T) {
	git := gitOrSkip(t)
	origin := t.TempDir()
	git(origin, "init", "-q", "--bare")

	w, _ := repository(t, map[string]string{"one": "a promise", "two": "another promise"})
	git(w.root, "config", "remote.origin.url", origin)
	w.ratify(t, "promise:one")
	first := strings.TrimSpace(gitOut(t, git, w.root, "rev-parse", ledger.Ref))
	git(w.root, "update-ref", ledger.UpstreamRef, first)

	// Origin is given everything, and the local record of what origin holds is
	// left at what it held before.
	w.ratify(t, "promise:two")
	git(w.root, "push", "-q", origin, ledger.Ref+":"+ledger.Ref)
	git(w.root, "update-ref", ledger.UpstreamRef, first)

	_, stdout, stderr := w.run("doctor")
	if !strings.Contains(stdout, "did not have when this repository last heard") {
		t.Fatalf("doctor answered about origin rather than about the upstream ref:\n%s\n%s", stdout, stderr)
	}
	if after := strings.TrimSpace(gitOut(t, git, w.root, "rev-parse", ledger.UpstreamRef)); after != first {
		t.Fatalf("doctor moved %s from %s to %s; something fetched", ledger.UpstreamRef, first, after)
	}
	// And the wording never claims the other thing.
	for _, forbidden := range []string{"origin does not have", "origin does not hold"} {
		if strings.Contains(stdout, forbidden) {
			t.Errorf("the finding claims live knowledge of origin: %q\n%s", forbidden, stdout)
		}
	}
}
