package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/pbleisch/kept/internal/corpus"
)

// The four rungs of one ladder, strongest first: do not create a contested
// number, report one, refuse to ratify past one, and renumber one where that is
// still free. See docs/decision/0069.

// numberedCorpus is a corpus with a numbered kind and a kind that cites it,
// which is the shape every one of these needs: a renumber that does not carry
// its citations is the defect rather than the fix.
func numberedCorpus(t *testing.T) *workspace {
	t.Helper()
	w := &workspace{t: t, root: t.TempDir()}
	w.put(".kept/config.yaml", configFile)
	for _, kind := range []string{"decision", "policy"} {
		w.put("docs/_schema/"+kind+".yaml", repoFile(t, "docs/_schema/"+kind+".yaml"))
	}
	return w
}

// adrAt writes one decision, optionally citing another by every form this
// corpus uses: a frontmatter target, an inline wiki link, and a relative file
// link whose text is the ordinal.
func adrAt(slug, title, cites string) string {
	body := "---\nid: " + slug + "\ntype: decision\ntitle: " + title + "\n"
	if cites != "" {
		body += "links:\n  - { target: decision:" + cites + ", relation: extends }\n"
	}
	body += "---\n\n# " + title + "\n\n## Context\n\n"
	if cites != "" {
		body += "See [[decision:" + cites + "]] and [" + cites[:4] + "](" + cites + ".md).\n"
	} else {
		body += "nothing\n"
	}
	return body + "\n## Decision\n\nthe thing\n\n## Consequences\n\nthe cost\n"
}

func policyCiting(slug, cites string) string {
	return "---\nid: " + slug + "\ntype: policy\ntitle: " + slug + "\nfields:\n" +
		"  statement: something must hold\n  applies_to: write\nlinks:\n" +
		"  - { target: decision:" + cites + ", relation: cites }\n---\n\n" +
		"See [[decision:" + cites + "]] and [" + cites[:4] + "](../decision/" + cites + ".md).\n"
}

func (w *workspace) exists(name string) bool {
	_, err := os.Stat(filepath.Join(w.root, filepath.FromSlash(name)))
	return err == nil
}

// Rung one, and the only one that prevents rather than reports. Two runs that
// read the corpus at the same moment would read one highest number and mint it
// twice; the mint lock is what stops them.
func TestConcurrentMintsTakeDifferentNumbers(t *testing.T) {
	w := numberedCorpus(t)
	if out, err := exec.Command("git", "init", "-q", w.root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}

	const runs = 8
	var wg sync.WaitGroup
	codes := make([]int, runs)
	for i := range runs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// --corpus rather than the working directory, because a chdir is
			// process-wide and these run at once. The lock is per repository
			// either way, and two opens in one process exclude each other.
			codes[i], _, _ = kept(t, "new", "decision", "a-thing-"+string(rune('a'+i)), "--corpus", filepath.Join(w.root, "docs"))
		}()
	}
	wg.Wait()

	for i, code := range codes {
		if code != exitOK {
			t.Errorf("run %d exited %d", i, code)
		}
	}
	c := load(t, w)
	if n := len(c.All()); n != runs {
		t.Fatalf("%d artifacts, want %d", n, runs)
	}
	for _, p := range c.Problems {
		if p.Kind == corpus.DuplicateNumber {
			t.Errorf("concurrent mints collided: %s", p.Detail)
		}
	}
}

// Rung three. Ratification is the trap door: while neither artifact of a
// contested number is ratified, moving one is free; ratify one and the options
// narrow; ratify both and there is no fix. So the refusal is at the door.
//
// It refuses for either artifact rather than letting the first through, because
// *you cannot ratify a number two artifacts share* is one sentence a person can
// hold and *the first may and the second may not* is a race with a lecture.
func TestRatifyRefusesEitherArtifactOfAContestedNumberAndNamesTheFix(t *testing.T) {
	w := numberedCorpus(t)
	w.put("docs/decision/0002-one.md", adrAt("0002-one", "0002 — One", ""))
	w.put("docs/decision/0002-two.md", adrAt("0002-two", "0002 — Two", ""))

	for _, id := range []string{"decision:0002-one", "decision:0002-two"} {
		code, _, stderr := w.run("ratify", id, "--by", "the operator")
		if code != exitUsage {
			t.Fatalf("ratify %s exited %d, want %d", id, code, exitUsage)
		}
		mustContain(t, "the refusal", stderr, "numbered 0002 as well", "kept doctor --fix")
	}

	// One command, and then it ratifies. If the way out took two steps or a
	// decision, the refusal would be documentation rather than a fix.
	if code, _, stderr := w.run("doctor", "--fix"); code != exitOK {
		t.Fatalf("doctor --fix exited %d: %s", code, stderr)
	}
	c := load(t, w)
	if len(c.Problems) != 0 {
		t.Fatalf("the corpus is not clean after the one command: %v", c.Problems)
	}
	for _, e := range c.All() {
		if code, _, stderr := w.run("ratify", e.Artifact.ID, "--by", "the operator"); code != exitOK {
			t.Errorf("ratify %s exited %d after the fix: %s", e.Artifact.ID, code, stderr)
		}
	}
}

// Retire is untouched: it is the way out, and a verb that refused to let go of
// a contested artifact would be a trap with no door.
func TestRetireIsNotRefusedForAContestedNumber(t *testing.T) {
	w := numberedCorpus(t)
	w.put("docs/decision/0002-one.md", adrAt("0002-one", "0002 — One", ""))
	w.ratify(t, "decision:0002-one")
	w.put("docs/decision/0002-two.md", adrAt("0002-two", "0002 — Two", ""))

	if code, _, stderr := w.run("retire", "decision:0002-one", "--by", "the operator"); code != exitOK {
		t.Fatalf("release exited %d: %s", code, stderr)
	}
}

// Rung four, the case where the number is free: move one of them, and carry
// every citation with it.
func TestFixRenumbersAContestedNumberAndCarriesEveryCitation(t *testing.T) {
	w := numberedCorpus(t)
	w.put("docs/decision/0001-the-first.md", adrAt("0001-the-first", "0001 — The first", ""))
	w.put("docs/decision/0002-cited.md", adrAt("0002-cited", "0002 — Cited", "0001-the-first"))
	w.put("docs/decision/0002-uncited.md", adrAt("0002-uncited", "0002 — Uncited", "0001-the-first"))
	w.put("docs/policy/a-rule.md", policyCiting("a-rule", "0002-cited"))

	if code, stdout, stderr := w.run("doctor", "--fix"); code != exitOK {
		t.Fatalf("doctor --fix exited %d: %s%s", code, stdout, stderr)
	}

	// The uncited one moved, because every citation of a mover has to be
	// rewritten and it had none.
	if !w.exists("docs/decision/0003-uncited.md") || w.exists("docs/decision/0002-uncited.md") {
		t.Fatalf("the uncited artifact did not move; the corpus has %v", tree(t, w.root))
	}
	if !w.exists("docs/decision/0002-cited.md") {
		t.Error("the cited artifact moved instead")
	}

	moved := w.read("docs/decision/0003-uncited.md")
	mustContain(t, "the renumbered artifact", moved, "id: 0003-uncited", "title: 0003 — Uncited", "# 0003 — Uncited")

	code, stdout, _ := w.run("list")
	if code != exitOK {
		t.Fatalf("list exited %d", code)
	}
	mustContain(t, "list", stdout, "nothing wrong")
}

// A renumber that does not rewrite its citations converts one reported problem
// into a broken ratified promise — and, for a relative markdown link, into a
// broken reference nothing reports at all, because kept parses no markdown link.
//
// So every form this corpus writes moves together: the frontmatter target, the
// inline wiki link, the relative file link, and the ordinal that link uses as
// its own text.
func TestEveryFormOfACitationMovesWithTheID(t *testing.T) {
	w := numberedCorpus(t)
	w.put("docs/decision/0001-the-first.md", adrAt("0001-the-first", "0001 — The first", ""))
	w.put("docs/decision/0002-uncited.md", adrAt("0002-uncited", "0002 — Uncited", "0001-the-first"))

	// Ratified while its number is its own, which is the only way a contested
	// number ever has a ratified member: kept ratify refuses the other order.
	w.ratify(t, "decision:0002-uncited")

	w.put("docs/decision/0002-cited.md", adrAt("0002-cited", "0002 — Cited", "0001-the-first"))
	w.put("docs/policy/a-rule.md", policyCiting("a-rule", "0002-cited"))

	if code, stdout, stderr := w.run("doctor", "--fix"); code != exitOK {
		t.Fatalf("doctor --fix exited %d: %s%s", code, stdout, stderr)
	}

	rule := w.read("docs/policy/a-rule.md")
	mustContain(t, "the citing policy", rule,
		"target: decision:0003-cited",
		"[[decision:0003-cited]]",
		"(../decision/0003-cited.md)",
		"[0003](")
	if strings.Contains(rule, "0002-cited") {
		t.Errorf("a citation was left behind:\n%s", rule)
	}
	if !w.exists("docs/decision/0002-uncited.md") {
		t.Error("the artifact the ledger names moved")
	}

	code, stdout, _ := w.run("list")
	if code != exitOK {
		t.Fatalf("list exited %d", code)
	}
	mustContain(t, "list", stdout, "nothing wrong")
}

// The ordinal that opens the artifact's own title and its H1 moves with the id,
// and finding the H1 means knowing where the frontmatter ends.
//
// A scaffolded artifact carries commented-out keys at column zero — `# fields:`
// — which are neither a heading nor a key. Read without the fences the first of
// those is taken for the H1, the real one is never reached, and the file ends up
// saying one number while its id says another. It is measured rather than
// argued: this is what a `kept new` decision looks like.
func TestTheOrdinalInTheArtifactsOwnTitleAndHeadingMovesWithIt(t *testing.T) {
	w := numberedCorpus(t)
	if code, _, stderr := w.run("new", "decision", "the-first"); code != exitOK {
		t.Fatalf("new exited %d: %s", code, stderr)
	}
	if code, _, stderr := w.run("new", "decision", "the-second"); code != exitOK {
		t.Fatalf("new exited %d: %s", code, stderr)
	}

	// Force the collision the way a merge of two branches does.
	scaffolded := w.read("docs/decision/0002-the-second.md")
	clashing := strings.NewReplacer("0002-the-second", "0001-the-second", "title: 0002", "title: 0001", "# 0002", "# 0001").Replace(scaffolded)
	w.put("docs/decision/0001-the-second.md", clashing)
	if err := os.Remove(filepath.Join(w.root, "docs", "decision", "0002-the-second.md")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clashing, "# fields:") {
		t.Fatal("the scaffold no longer writes a commented key at column zero; this test measured that shape")
	}

	if code, stdout, stderr := w.run("doctor", "--fix"); code != exitOK {
		t.Fatalf("doctor --fix exited %d: %s%s", code, stdout, stderr)
	}

	moved := w.read("docs/decision/0002-the-second.md")
	mustContain(t, "the renumbered artifact", moved,
		"id: 0002-the-second", "title: 0002 — The second", "\n# 0002 — The second")
	if strings.Contains(moved, "0001") {
		t.Errorf("the old number was left somewhere:\n%s", moved)
	}
}

// An artifact whose standing was released is still one the ledger names. The
// question is not whether standing is in force — it is whether the record
// mentions the id, because the record cannot be amended.
func TestARetiredArtifactIsStillNamedByTheLedgerAndDoesNotMove(t *testing.T) {
	w := numberedCorpus(t)
	w.put("docs/decision/0002-retired.md", adrAt("0002-retired", "0002 — Retired", ""))
	w.ratify(t, "decision:0002-retired")
	if code, _, stderr := w.run("retire", "decision:0002-retired", "--by", "the operator"); code != exitOK {
		t.Fatalf("release exited %d: %s", code, stderr)
	}
	w.put("docs/decision/0002-fresh.md", adrAt("0002-fresh", "0002 — Fresh", ""))

	if code, stdout, stderr := w.run("doctor", "--fix"); code == exitUsage {
		t.Fatalf("doctor --fix exited %d: %s%s", code, stdout, stderr)
	}
	if !w.exists("docs/decision/0002-retired.md") {
		t.Error("an artifact the ledger names was renumbered")
	}
	if !w.exists("docs/decision/0003-fresh.md") {
		t.Errorf("the free artifact did not move; the corpus has %v", tree(t, w.root))
	}
}

// The guard on the case ratify closes. Getting here needs two branches that each
// ratified while their artifact was the only one carrying its number, so the test
// builds it that way. Nothing can be done from here, and the refusal says that
// rather than documenting a repair that does not exist.
func TestFixRefusesToRenumberWhenTheLedgerNamesBoth(t *testing.T) {
	w := numberedCorpus(t)
	w.put("docs/decision/0002-one.md", adrAt("0002-one", "0002 — One", ""))
	w.ratify(t, "decision:0002-one")

	// The second branch: the first artifact is not in its corpus, so the number
	// is free there and the ratification is clean where it is performed.
	away := filepath.Join(w.root, "0002-one.md.away")
	if err := os.Rename(filepath.Join(w.root, "docs", "decision", "0002-one.md"), away); err != nil {
		t.Fatal(err)
	}
	w.put("docs/decision/0002-two.md", adrAt("0002-two", "0002 — Two", ""))
	w.ratify(t, "decision:0002-two")
	if err := os.Rename(away, filepath.Join(w.root, "docs", "decision", "0002-one.md")); err != nil {
		t.Fatal(err)
	}

	before := tree(t, w.root)
	code, stdout, _ := w.run("doctor", "--fix")
	if code == exitUsage {
		t.Fatalf("doctor --fix exited %d", code)
	}

	mustContain(t, "the refusal", stdout,
		"left alone", "decision:0002-one", "decision:0002-two", "stranding its standing", "retire one")
	for path, was := range tree(t, w.root) {
		if before[path] != was {
			t.Errorf("a refused renumber changed %s", path)
		}
	}
	// And the finding is still reported, so nothing goes quiet.
	mustContain(t, "doctor", stdout, "duplicate-number")
}

// --fix is the flag. Without it nothing is written, whatever is found.
func TestDoctorWithoutFixRenumbersNothing(t *testing.T) {
	w := numberedCorpus(t)
	w.put("docs/decision/0002-one.md", adrAt("0002-one", "0002 — One", ""))
	w.put("docs/decision/0002-two.md", adrAt("0002-two", "0002 — Two", ""))

	before := tree(t, w.root)
	code, stdout, _ := w.run("doctor")
	if code == exitUsage {
		t.Fatalf("doctor exited %d", code)
	}
	mustContain(t, "doctor", stdout, "duplicate-number")
	for path, was := range tree(t, w.root) {
		if before[path] != was {
			t.Errorf("doctor without --fix changed %s", path)
		}
	}
}

// Three claiming one number: one keeps it and the rest take the next free ones,
// which must not be the same number twice.
func TestThreeArtifactsSharingANumberAllEndUpSomewhereDifferent(t *testing.T) {
	w := numberedCorpus(t)
	for _, slug := range []string{"0002-a", "0002-b", "0002-c"} {
		w.put("docs/decision/"+slug+".md", adrAt(slug, "0002 — "+slug, ""))
	}

	if code, stdout, stderr := w.run("doctor", "--fix"); code != exitOK {
		t.Fatalf("doctor --fix exited %d: %s%s", code, stdout, stderr)
	}

	c := load(t, w)
	if len(c.All()) != 3 {
		t.Fatalf("%d artifacts, want 3", len(c.All()))
	}
	seen := map[string]bool{}
	for _, e := range c.All() {
		_, slug := corpus.SplitRef(e.Artifact.ID)
		number, _, _ := strings.Cut(slug, "-")
		if seen[number] {
			t.Errorf("two artifacts still share %s: %v", number, idsOf(c))
		}
		seen[number] = true
	}
	if len(c.Problems) != 0 {
		t.Errorf("the corpus is not clean: %v", c.Problems)
	}
}

// An id of the wrong shape is not a collision, and --fix does not touch it: what
// is wrong with it is one file's authoring, and 0069 says why that is a person's.
func TestFixDoesNotRenumberAnIDOfTheWrongShape(t *testing.T) {
	w := numberedCorpus(t)
	w.put("docs/decision/unnumbered.md", adrAt("unnumbered", "Unnumbered", ""))

	code, stdout, _ := w.run("doctor", "--fix")
	if code == exitUsage {
		t.Fatalf("doctor --fix exited %d", code)
	}
	if !w.exists("docs/decision/unnumbered.md") {
		t.Error("an id of the wrong shape was rewritten")
	}
	mustContain(t, "doctor", stdout, "id-shape")
}

// The match is bounded so that it cannot reach a longer name that starts with
// the one being moved, and cannot reach digits somebody wrote in prose.
func TestTheRewriteCannotReachALongerNameOrProse(t *testing.T) {
	w := numberedCorpus(t)
	w.put("docs/decision/0001-a-thing.md", adrAt("0001-a-thing", "0001 — A thing", ""))
	w.put("docs/decision/0002-a-clash.md", adrAt("0002-a-clash", "0002 — A clash", ""))
	w.put("docs/decision/0002-a-clash-considered-again.md",
		adrAt("0002-a-clash-considered-again", "0002 — A clash considered again", ""))
	w.put("docs/policy/a-rule.md", "---\nid: a-rule\ntype: policy\ntitle: a-rule\nfields:\n"+
		"  statement: something must hold\n  applies_to: write\n---\n\n"+
		"decision:0002-a-clash-considered-again is a different artifact, and 0002 is a number in a sentence.\n")

	if code, stdout, stderr := w.run("doctor", "--fix"); code != exitOK {
		t.Fatalf("doctor --fix exited %d: %s%s", code, stdout, stderr)
	}

	rule := w.read("docs/policy/a-rule.md")
	if !strings.Contains(rule, "0002 is a number in a sentence") {
		t.Errorf("a number in prose was rewritten:\n%s", rule)
	}
	c := load(t, w)
	if len(c.Problems) != 0 {
		t.Errorf("the corpus is not clean: %v", c.Problems)
	}
}

// Nothing about a renumber touches the ledger. Standing is created only by
// ratification and only by a person, and moving an id is neither.
func TestARenumberWritesNothingToTheLedger(t *testing.T) {
	w := numberedCorpus(t)
	w.put("docs/decision/0002-one.md", adrAt("0002-one", "0002 — One", ""))
	w.put("docs/decision/0002-two.md", adrAt("0002-two", "0002 — Two", ""))

	_, before, _ := w.run("status")
	if code, _, stderr := w.run("doctor", "--fix"); code != exitOK {
		t.Fatalf("doctor --fix exited %d: %s", code, stderr)
	}
	code, after, _ := w.run("status")
	if code == exitUsage {
		t.Fatal("status exited 2")
	}
	if strings.Contains(after, "  ratified ") {
		t.Errorf("something gained standing:\n%s", after)
	}
	if strings.Count(before, "never ratified") != strings.Count(after, "never ratified") {
		t.Errorf("standing changed:\n%s\n---\n%s", before, after)
	}
}

// The bounded name match on its own, because it is what keeps the rewrite from
// becoming one that edits prose.
func TestReplaceNameMatchesAWholeNameAndNothingElse(t *testing.T) {
	for _, tc := range []struct {
		in, want string
		n        int
	}{
		{"decision:0002-a and [[decision:0002-a]]", "decision:0003-a and [[decision:0003-a]]", 2},
		{"](0002-a.md)", "](0003-a.md)", 1},
		{"0002-a-longer-name", "0002-a-longer-name", 0},
		{"prefixed-0002-a", "prefixed-0002-a", 0},
		{"0002-a", "0003-a", 1},
		{"0002-ab", "0002-ab", 0},
		{"x 0002-a. y", "x 0003-a. y", 1},
	} {
		got, n := replaceName(tc.in, "0002-a", "0003-a")
		if got != tc.want || n != tc.n {
			t.Errorf("replaceName(%q) = %q, %d; want %q, %d", tc.in, got, n, tc.want, tc.n)
		}
	}
}
