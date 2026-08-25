package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/schema"
)

// The scaffold is generated from the schemas this repository ships rather than
// from a copy of them, for the reason the verb exists: a second copy of a schema
// is a thing that can disagree with the schema.
func newCorpus(t *testing.T, kinds ...string) *workspace {
	t.Helper()
	w := &workspace{t: t, root: t.TempDir()}
	w.put(".kept/config.yaml", configFile)
	if len(kinds) == 0 {
		kinds = shippedKinds(t)
	}
	for _, kind := range kinds {
		w.put("docs/_schema/"+kind+".yaml", repoFile(t, "docs/_schema/"+kind+".yaml"))
	}
	return w
}

// shippedKinds is every kind this repository declares a schema for. The tests
// below run over it rather than over a list, so a kind added to docs/_schema is
// a kind they cover the day it appears.
func shippedKinds(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join("..", "..", "docs", schema.Dir))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if name := e.Name(); !e.IsDir() && strings.HasSuffix(name, ".yaml") {
			out = append(out, strings.TrimSuffix(name, ".yaml"))
		}
	}
	if len(out) == 0 {
		t.Fatal("no schemas under docs/_schema")
	}
	return out
}

// what a kind needs on the command line before kept will write one: the fields
// it declares required and binding, which kept refuses to invent a value for.
// It is the argument list rather than a property under test — the refusal that
// produces it has its own test below.
var required = map[string][]string{
	"promise": {"--set", "statement=every package under internal/ has a test"},
	"policy":  {"--set", "statement=a packet carries no credential", "--set", "applies_to=dispatch"},
	"canon":   {"--set", "statement=the kernel is what the rest is checked against"},
}

// newArtifact scaffolds one and returns the id kept minted and the path it
// wrote, both read off the verb's own output — a numbered kind's id is not the
// slug that was typed, and a test that assumed it was would be asserting against
// the shape rather than reading it.
func newArtifact(t *testing.T, w *workspace, kind, slug string) (id, path string) {
	t.Helper()
	args := append([]string{"new", kind, slug}, required[kind]...)
	code, stdout, stderr := w.run(args...)
	if code != exitOK {
		t.Fatalf("new %s exited %d: %s", kind, code, stderr)
	}
	rel, _, ok := strings.Cut(strings.TrimPrefix(stdout, "wrote "), "\n")
	if !ok {
		t.Fatalf("new %s did not say what it wrote:\n%s", kind, stdout)
	}
	return corpus.Ref(kind, strings.TrimSuffix(filepath.Base(rel), ".md")), "docs/" + rel
}

// The acceptance condition, and the one that cannot be checked by eye: what the
// scaffold writes is read back by the corpus reader with nothing to say about
// it. If a generated artifact can produce a problem of any kind — a misplaced
// field, a missing section, a required field nobody answered — the scaffold is
// what is wrong.
func TestEveryScaffoldedKindIsReadWithNoProblem(t *testing.T) {
	w := newCorpus(t)

	for _, kind := range shippedKinds(t) {
		newArtifact(t, w, kind, "a-scaffolded-"+kind)
	}

	code, stdout, stderr := w.run("list")
	if code != exitOK {
		t.Fatalf("list exited %d: %s", code, stderr)
	}
	if strings.Contains(stdout, "problem") {
		t.Fatalf("kept list reported a problem about what kept new wrote:\n%s", stdout)
	}
	mustContain(t, "list", stdout, "nothing wrong")
}

// The half of the defect this verb exists to prevent. A declared field goes
// under fields:, which is where the schema is consulted; the top level is the
// author's and a declared name written there is read by nobody — which is how
// five decisions came to hash as though they had no statement.
func TestADeclaredFieldIsWrittenUnderFieldsAndNeverAtTheTopLevel(t *testing.T) {
	w := newCorpus(t, "promise")
	_, path := newArtifact(t, w, "promise", "every-package-has-a-test")

	src := w.read(path)
	if strings.Contains(src, "\nstatement:") {
		t.Errorf("statement is at the top level, where nothing reads it:\n%s", src)
	}
	if !strings.Contains(src, "fields:\n") || !strings.Contains(src, "\n  statement: ") {
		t.Errorf("statement is not under fields::\n%s", src)
	}

	// And the reader agrees, which is the assertion that survives a change to
	// how the block is spelled.
	c := load(t, w)
	e, ok := c.Get("promise:every-package-has-a-test")
	if !ok {
		t.Fatal("the promise was not read")
	}
	if e.Artifact.Statement == "" {
		t.Error("the statement was written where the corpus reader does not look")
	}
	if corpus.Misplaced("statement", schemaFor(t, c, "promise")) && e.Frontmatter["statement"] != nil {
		t.Error("statement is misplaced")
	}
}

// A decision's hash list names body.decision, so a scaffold that omits the
// heading produces an artifact whose hash covers a section that does not exist.
func TestAScaffoldedDecisionHasTheSectionItsHashCovers(t *testing.T) {
	w := newCorpus(t, "decision")
	id, _ := newArtifact(t, w, "decision", "what-was-decided")

	c := load(t, w)
	e, ok := c.Get(id)
	if !ok {
		t.Fatal("the decision was not read")
	}

	sc := schemaFor(t, c, "decision")
	for _, name := range sc.Hash {
		id, sectioned := strings.CutPrefix(name, schema.HashBodyPrefix)
		if !sectioned {
			continue
		}
		if _, found, ambiguous := e.Section(id); !found || ambiguous {
			t.Errorf("the hash covers %s and the scaffold's body has found=%v ambiguous=%v", name, found, ambiguous)
		}
	}
	// Every declared section, not only the binding one — an absent declared
	// section is reported by every read.
	for _, id := range sc.Sections {
		if _, found, _ := e.Section(id); !found {
			t.Errorf("the schema declares the section %q and the scaffold wrote no heading for it", id)
		}
	}
}

// A field that binds is never given words kept invented. A ratification is
// granted against its value, so a promise scaffolded with a placeholder
// statement can be ratified exactly as it stands — which grants standing to the
// placeholder. The hash list is the discriminator, and kept already has it.
func TestABindingFieldIsNeverGivenScaffoldText(t *testing.T) {
	w := newCorpus(t)

	for _, kind := range shippedKinds(t) {
		code, _, stderr := w.run("new", kind, "unfilled-"+kind, "--stdout")
		sc := schemaFor(t, load(t, w), kind)

		var binding []string
		for _, name := range sc.FieldOrder() {
			if sc.Fields[name].Required && sc.Binds(name) {
				binding = append(binding, name)
			}
		}
		if len(binding) == 0 {
			if code != exitOK {
				t.Errorf("new %s exited %d with nothing to refuse: %s", kind, code, stderr)
			}
			continue
		}
		if code != exitUsage {
			t.Errorf("new %s exited %d; %v binds and is required, so kept has nothing to write there",
				kind, code, binding)
		}
		for _, name := range binding {
			mustContain(t, "the refusal for "+kind, stderr, name, "--set "+name+"=")
		}
	}
}

// The other half of the same rule: nothing kept writes into a binding field
// unless a person supplied it, and the placeholder it does write only ever
// lands where nothing binds.
func TestTheOnlyPlaceholderIsInAFieldNothingBinds(t *testing.T) {
	w := newCorpus(t)

	for _, kind := range shippedKinds(t) {
		_, path := newArtifact(t, w, kind, "placeholder-"+kind)
		src := w.read(path)
		sc := schemaFor(t, load(t, w), kind)

		for _, line := range strings.Split(src, "\n") {
			name, value, ok := strings.Cut(strings.TrimSpace(line), ": ")
			if !ok || strings.HasPrefix(name, "#") || !strings.Contains(value, unwritten) {
				continue
			}
			if sc.Binds(name) {
				t.Errorf("%s: %s binds and holds %q", kind, name, value)
			}
		}
	}
}

// A kind whose whole body binds gets no body at all. Every byte kept wrote there
// would be inside the hash a ratification is granted against — the H1 included,
// which is a title kept derived from the slug when nobody passed --title.
func TestAKindWhoseWholeBodyBindsIsScaffoldedWithNoBody(t *testing.T) {
	w := newCorpus(t)

	for _, kind := range shippedKinds(t) {
		id, _ := newArtifact(t, w, kind, "body-"+kind)
		c := load(t, w)
		e, ok := c.Get(id)
		if !ok {
			t.Fatalf("%s was not read", kind)
		}
		binds := schemaFor(t, c, kind).Binds(schema.HashBody)
		if binds && e.Artifact.Body != "" {
			t.Errorf("%s binds its whole body and the scaffold wrote %q into it", kind, e.Artifact.Body)
		}
		if !binds && e.Artifact.Body == "" {
			t.Errorf("%s does not bind its body and the scaffold wrote nothing there", kind)
		}
	}
}

// A schema's default is recorded rather than substituted when an artifact is
// read, and writing one into a new artifact is the same substitution by another
// route: it puts a value in the file that its author did not write.
func TestNoSchemaDefaultIsWrittenIntoAScaffold(t *testing.T) {
	w := newCorpus(t)

	for _, kind := range shippedKinds(t) {
		id, _ := newArtifact(t, w, kind, "no-defaults-"+kind)
		c := load(t, w)
		e, ok := c.Get(id)
		if !ok {
			t.Fatalf("%s was not read", kind)
		}
		sc := schemaFor(t, c, kind)
		for _, name := range sc.FieldOrder() {
			if sc.Fields[name].Default == nil {
				continue
			}
			if _, written := e.Artifact.Fields[name]; written {
				t.Errorf("%s: %s was written, and its schema only says what its absence means", kind, name)
			}
		}
	}
}

// Nothing kept writes rewrites what a person wrote. The policy is ratified, and
// this is the verb it most obviously applies to.
func TestNewRefusesToOverwriteAndLeavesTheFileExactlyAsItWas(t *testing.T) {
	w := newCorpus(t, "promise")
	newArtifact(t, w, "promise", "every-package-has-a-test")

	w.put("docs/promise/every-package-has-a-test.md", "the person's own words\n")

	code, stdout, stderr := w.run("new", "promise", "every-package-has-a-test",
		"--set", "statement=something else entirely")
	if code != exitUsage {
		t.Fatalf("exit %d, want %d: %s%s", code, exitUsage, stdout, stderr)
	}
	if got := w.read("docs/promise/every-package-has-a-test.md"); got != "the person's own words\n" {
		t.Fatalf("the file was rewritten:\n%s", got)
	}
}

// An id names one artifact, and two claiming one is the single thing that stops
// a corpus loading — so a slug already taken is refused before anything is
// written, even when the file it would write is not the one holding it.
func TestNewRefusesASlugTheCorpusAlreadyClaims(t *testing.T) {
	w := newCorpus(t, "promise")
	w.put("docs/promise/named-something-else.md",
		"---\nid: every-package-has-a-test\ntitle: t\nfields:\n  statement: s\n---\n\nwhy\n")

	code, _, stderr := w.run("new", "promise", "every-package-has-a-test", "--set", "statement=s")
	if code != exitUsage {
		t.Fatalf("exit %d, want %d", code, exitUsage)
	}
	mustContain(t, "the refusal", stderr, "already claimed by", "named-something-else.md")
}

// The whole design constraint, demonstrated: a field added to a schema reaches
// the scaffold with no code change, because the scaffold reads the schema.
func TestAFieldAddedToASchemaAppearsInTheScaffoldWithNoCodeChange(t *testing.T) {
	w := newCorpus(t, "promise")

	before := w.read("docs/_schema/promise.yaml")
	w.put("docs/_schema/promise.yaml", strings.Replace(before, "fields:\n", `fields:
  witnessed_by:
    type: string
    description: "Who watched this being measured."
`, 1))

	newArtifact(t, w, "promise", "every-package-has-a-test")
	src := w.read("docs/promise/every-package-has-a-test.md")
	mustContain(t, "the scaffold", src, "witnessed_by", "Who watched this being measured.")
}

// kept writes the artifact and nothing else. A new artifact is unratified, and
// standing is created only by ratification and only by a person.
func TestNewWritesNothingButTheArtifact(t *testing.T) {
	w := newCorpus(t, "promise")

	before := tree(t, w.root)
	newArtifact(t, w, "promise", "every-package-has-a-test")
	after := tree(t, w.root)

	for path := range after {
		if _, existed := before[path]; !existed && path != "docs/promise/every-package-has-a-test.md" {
			t.Errorf("kept new created %s", path)
		}
	}
	for path, was := range before {
		if after[path] != was {
			t.Errorf("kept new changed %s", path)
		}
	}

	code, stdout, _ := w.run("status")
	if code == exitUsage {
		t.Fatalf("status exited %d", code)
	}
	mustContain(t, "status", stdout, "never ratified", "promise:every-package-has-a-test")
}

// --stdout prints the same bytes and creates nothing, which is what a person
// reviewing the shape before it lands in their corpus wants.
func TestStdoutPrintsTheArtifactAndCreatesNothing(t *testing.T) {
	w := newCorpus(t, "promise")

	code, stdout, stderr := w.run("new", "promise", "every-package-has-a-test",
		"--set", "statement=every package under internal/ has a test", "--stdout")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(w.root, "docs", "promise", "every-package-has-a-test.md")); err == nil {
		t.Error("--stdout wrote the file")
	}

	newArtifact(t, w, "promise", "every-package-has-a-test")
	if written := w.read("docs/promise/every-package-has-a-test.md"); written != stdout {
		t.Errorf("--stdout and the file disagree:\n%s\n---\n%s", stdout, written)
	}
}

// A --set nothing declares is refused rather than written. A value under a name
// the schema has no opinion about is the shape this verb exists to stop
// producing, one level down from the top-level statement and just as unread.
func TestSetRefusesWhatTheSchemaDoesNotDeclare(t *testing.T) {
	w := newCorpus(t, "promise")

	for _, bad := range [][]string{
		{"--set", "statment=a typo"},
		{"--set", "title=the envelope's own"},
		{"--set", "gather=a map cannot arrive as a scalar"},
		{"--set", "trigger=whenever"},
		{"--set", "max_age="},
	} {
		args := append([]string{"new", "promise", "every-package-has-a-test", "--set",
			"statement=every package under internal/ has a test"}, bad...)
		code, _, stderr := w.run(args...)
		if code != exitUsage {
			t.Errorf("%v exited %d, want %d", bad, code, exitUsage)
		}
		if !strings.Contains(stderr, "--set") {
			t.Errorf("%v was refused without saying which flag: %s", bad, stderr)
		}
	}
	if _, err := os.Stat(filepath.Join(w.root, "docs", "promise", "every-package-has-a-test.md")); err == nil {
		t.Error("a refused run still wrote the file")
	}
}

// A kind with no schema has no shape to generate, so it is refused by name
// rather than written from nothing.
func TestNewRefusesAKindNothingDeclares(t *testing.T) {
	w := newCorpus(t, "promise")

	code, _, stderr := w.run("new", "chapter", "the-fall-of-rome")
	if code != exitUsage {
		t.Fatalf("exit %d, want %d", code, exitUsage)
	}
	mustContain(t, "the refusal", stderr, "chapter", "promise")
}

// The identity is the slug in frontmatter and the filename is not it (0021).
// The file is still named for the slug, because a filename that is not the slug
// is a path-mismatch problem every read reports — and a scaffold that generates
// a finding is not one.
func TestTheFileIsNamedForTheSlugAndTheSlugIsTheAuthoredID(t *testing.T) {
	w := newCorpus(t, "promise")
	newArtifact(t, w, "promise", "every-package-has-a-test")

	src := w.read("docs/promise/every-package-has-a-test.md")
	mustContain(t, "the frontmatter", src, "id: every-package-has-a-test")

	// Renaming the file changes nothing, which is the whole of 0021 and is why
	// the filename is a convenience rather than the identity.
	from := filepath.Join(w.root, "docs", "promise", "every-package-has-a-test.md")
	to := filepath.Join(w.root, "docs", "promise", "renamed.md")
	if err := os.Rename(from, to); err != nil {
		t.Fatal(err)
	}
	if _, ok := load(t, w).Get("promise:every-package-has-a-test"); !ok {
		t.Error("renaming the file changed the artifact's identity")
	}
}

// A slug that is not a path component names a file nobody meant, and the truth
// and disposition streams are named after it.
func TestNewRefusesASlugThatIsNotAPathComponent(t *testing.T) {
	w := newCorpus(t, "promise")

	for _, slug := range []string{"has/a/separator", "..", "has a space"} {
		code, _, stderr := w.run("new", "promise", slug, "--set", "statement=s")
		if code != exitUsage {
			t.Errorf("%q exited %d, want %d", slug, code, exitUsage)
		}
		if !strings.Contains(stderr, "slug") {
			t.Errorf("%q was refused without saying why: %s", slug, stderr)
		}
	}
}

// A heading has to produce the identifier the schema declared, or the section it
// opens is not the one the hash covers.
func TestEveryScaffoldedHeadingProducesTheIdentifierItWasWrittenFor(t *testing.T) {
	for _, id := range []string{"context", "decision", "consequences", "what-this-does-not-decide", "kael-s-eyes", "0-and-1"} {
		if got := schema.SectionID(heading(id)); got != id {
			t.Errorf("heading(%q) = %q, which reads back as %q", id, heading(id), got)
		}
	}
}

func schemaFor(t *testing.T, c *corpus.Corpus, kind string) *schema.Schema {
	t.Helper()
	sc, ok := c.Schemas.For(kind)
	if !ok {
		t.Fatalf("no %s schema", kind)
	}
	return sc
}

func load(t *testing.T, w *workspace) *corpus.Corpus {
	t.Helper()
	c, err := corpus.Load(filepath.Join(w.root, "docs"), w.root, corpus.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// tree is every file under root and its contents, so that a verb can be held to
// having written one file and touched nothing else.
func tree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = string(src)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// The number in a decision's id is minted, and only the number.
//
// It is part of the identity every citation writes — `decision:0068-...` — so a
// scaffold that skipped it would author the only unnumbered decision in a corpus
// of sixty-eight, which is drift from convention introduced by the tool built to
// stop it. The schema is what says the kind is numbered; nothing here knows what
// a decision is.
func TestANumberedKindsIDIsMintedFromTheSchemasDeclaredShape(t *testing.T) {
	w := newCorpus(t, "decision")

	newArtifact(t, w, "decision", "the-first-thing")
	newArtifact(t, w, "decision", "the-second-thing")

	c := load(t, w)
	for _, id := range []string{"decision:0001-the-first-thing", "decision:0002-the-second-thing"} {
		if _, ok := c.Get(id); !ok {
			t.Errorf("%s was not minted; the corpus has %v", id, idsOf(c))
		}
	}
	// The file is named for the minted id, and the title carries the number the
	// way every decision in this repository does.
	src := w.read("docs/decision/0002-the-second-thing.md")
	mustContain(t, "the scaffold", src, "id: 0002-the-second-thing", "title: 0002 — The second thing")
}

// The next number is the highest the kind already has, plus one — read out of
// the corpus rather than counted, so a gap does not shift everything after it.
func TestTheMintedNumberFollowsTheHighestTheKindAlreadyHas(t *testing.T) {
	w := newCorpus(t, "decision")
	w.put("docs/decision/0007-a-gap-before-this.md",
		"---\nid: 0007-a-gap-before-this\ntitle: t\n---\n\n## Context\n\nc\n\n## Decision\n\nd\n\n## Consequences\n\nq\n")

	newArtifact(t, w, "decision", "after-the-gap")
	if _, ok := load(t, w).Get("decision:0008-after-the-gap"); !ok {
		t.Errorf("the next number did not follow the highest: %v", idsOf(load(t, w)))
	}
}

// Only the number is minted. A slug that already carries one is refused rather
// than accepted or renumbered, because two sources for one number is the rule
// living in two places — and renumbering is an edit to the file.
func TestASlugThatAlreadyCarriesANumberIsRefused(t *testing.T) {
	w := newCorpus(t, "decision")

	code, _, stderr := w.run("new", "decision", "0069-already-numbered")
	if code != exitUsage {
		t.Fatalf("exit %d, want %d", code, exitUsage)
	}
	mustContain(t, "the refusal", stderr, "NNNN-slug", "kept new decision already-numbered")
}

// The whole point of putting the shape in the schema: a kind becomes numbered by
// declaring it, with no code change.
func TestAKindBecomesNumberedByDeclaringItInItsSchema(t *testing.T) {
	w := newCorpus(t, "promise")

	newArtifact(t, w, "promise", "before-the-declaration")
	if _, ok := load(t, w).Get("promise:before-the-declaration"); !ok {
		t.Fatal("an undeclared shape did not mint a plain slug")
	}

	w.put("docs/_schema/promise.yaml", "id:\n  shape: numbered\n  width: 3\n"+w.read("docs/_schema/promise.yaml"))
	newArtifact(t, w, "promise", "after-the-declaration")

	c := load(t, w)
	if _, ok := c.Get("promise:001-after-the-declaration"); !ok {
		t.Errorf("declaring the shape did not mint a number: %v", idsOf(c))
	}
	// And the one written before the declaration is now reported, because the
	// check and the generator read one declaration.
	var reported bool
	for _, p := range c.Problems {
		reported = reported || (p.Kind == corpus.IDShape && strings.Contains(p.Detail, "before-the-declaration"))
	}
	if !reported {
		t.Errorf("the id written before the declaration was not reported: %v", c.Problems)
	}
}

// kept never mints an id the corpus would then report, so the whole scaffold
// stays clean for a numbered kind too.
func TestAMintedIDIsTheShapeTheCorpusChecksFor(t *testing.T) {
	w := newCorpus(t)
	for _, kind := range shippedKinds(t) {
		newArtifact(t, w, kind, "a-minted-"+kind)
	}

	code, stdout, stderr := w.run("list")
	if code != exitOK {
		t.Fatalf("list exited %d: %s", code, stderr)
	}
	mustContain(t, "list", stdout, "nothing wrong")
}

func idsOf(c *corpus.Corpus) []string {
	var out []string
	for _, e := range c.All() {
		out = append(out, e.Artifact.ID)
	}
	return out
}
