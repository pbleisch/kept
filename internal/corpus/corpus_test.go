package corpus

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/model"
)

const (
	fixture   = "testdata/corpus"
	duplicate = "testdata/duplicate"
	promiseID = "promise:core-packages-are-tested"
	canonID   = "canon:kernel-correctness"
)

func load(t *testing.T, root string) *Corpus {
	t.Helper()
	c, err := Load(root, root)
	if err != nil {
		t.Fatalf("Load(%s): %v", root, err)
	}
	return c
}

func get(t *testing.T, c *Corpus, id string) *Entry {
	t.Helper()
	e, ok := c.Get(id)
	if !ok {
		t.Fatalf("no artifact %s; the corpus has %v", id, ids(c))
	}
	return e
}

func ids(c *Corpus) []string {
	var out []string
	for _, e := range c.All() {
		out = append(out, e.Artifact.ID)
	}
	return out
}

// Reads a hand-written corpus and prints it faithfully — every field.
func TestLoadReadsEveryFieldOfAHandWrittenArtifact(t *testing.T) {
	c := load(t, fixture)
	art := get(t, c, promiseID).Artifact

	if art.Kind != model.KindPromise {
		t.Errorf("Kind = %q, want %q", art.Kind, model.KindPromise)
	}
	if want := "Core packages are tested"; art.Title != want {
		t.Errorf("Title = %q, want %q", art.Title, want)
	}
	if want := "every package under internal/ has a test that runs and passes"; art.Statement != want {
		t.Errorf("Statement = %q, want %q", art.Statement, want)
	}
	if want := "all(obs.tests.packages, p => p.tests_run > 0 and p.failed == 0)"; art.HoldWhen != want {
		t.Errorf("HoldWhen = %q, want %q", art.HoldWhen, want)
	}
	if art.Trigger != model.TriggerCadence {
		t.Errorf("Trigger = %q, want %q", art.Trigger, model.TriggerCadence)
	}
	if want := []string{"internal/"}; !reflect.DeepEqual(art.RepairScope, want) {
		t.Errorf("RepairScope = %v, want %v", art.RepairScope, want)
	}

	// The gatherer defaults to the map key and is overridden when the entry
	// names one; every other key is an argument.
	wantGather := map[string]model.GatherSpec{
		"tests": {Gatherer: "tests", Args: map[string]any{"scope": "internal/**"}},
		"build": {Gatherer: "command", Args: map[string]any{"run": "go build ./..."}},
	}
	if !reflect.DeepEqual(art.Gather, wantGather) {
		t.Errorf("Gather = %#v, want %#v", art.Gather, wantGather)
	}

	if !strings.HasPrefix(art.Body, "Tests that cannot fail are not evidence") {
		t.Errorf("Body starts %q", firstLine(art.Body))
	}
	if strings.HasSuffix(art.Body, "\n") {
		t.Error("Body keeps the file's trailing newline")
	}
}

// Reads a hand-written corpus and prints it faithfully — every link, from all
// three sources, deduplicated by priority.
func TestLoadResolvesTheThreeLinkSources(t *testing.T) {
	c := load(t, fixture)

	promise := get(t, c, promiseID)
	want := []ResolvedLink{
		{Link: link(canonID, model.RefArtifact, "serves"), From: FromFrontmatter},
		{Link: link("canon:nothing-claims-this", model.RefArtifact, "cites"), From: FromFrontmatter, Broken: true},
		{Link: link("canon/portrait.png", model.RefFile, "cites"), From: FromFrontmatter},
		{Link: link("promise/nothing-is-here.md", model.RefFile, "cites"), From: FromFrontmatter, Broken: true},
		{Link: link("https://example.com/kept/spec", model.RefURI, "cites"), From: FromFrontmatter},
		{Link: link("canon:absent-from-the-body", model.RefArtifact, ""), From: FromBody, Broken: true},
		{Link: link("canon/correctness.md", model.RefFile, ""), From: FromBody},
		{Link: link("https://example.com/kept/rationale", model.RefURI, ""), From: FromBody},
	}
	if !reflect.DeepEqual(promise.Links, want) {
		t.Errorf("links =\n%+v\nwant\n%+v", promise.Links, want)
	}

	// The body also names canon:kernel-correctness. The frontmatter entry says
	// more about the same edge, so the body one is dropped rather than added
	// as an untyped duplicate.
	for _, l := range promise.Links {
		if l.Target == canonID && l.From != FromFrontmatter {
			t.Errorf("the frontmatter link to %s lost to a %s one", canonID, l.From)
		}
	}

	// A manifest is the third source, and it loses to frontmatter the same way.
	canon := get(t, c, canonID)
	wantCanon := []ResolvedLink{
		{Link: link(promiseID, model.RefArtifact, "serves"), From: FromFrontmatter},
		{Link: link("canon:only-a-manifest-names-this", model.RefArtifact, "about"), From: FromManifest, Broken: true},
	}
	if !reflect.DeepEqual(canon.Links, wantCanon) {
		t.Errorf("canon links =\n%+v\nwant\n%+v", canon.Links, wantCanon)
	}
}

// Reads a hand-written corpus and prints it faithfully — and every broken link.
func TestLoadDetectsBrokenLinks(t *testing.T) {
	c := load(t, fixture)

	got := map[string]string{}
	for _, b := range c.Broken() {
		got[b.Link.Target] = b.From
	}
	want := map[string]string{
		"canon:nothing-claims-this":        promiseID,
		"canon:absent-from-the-body":       promiseID,
		"promise/nothing-is-here.md":       promiseID,
		"canon:only-a-manifest-names-this": canonID,
		"canon:no-such-canon":              "https://example.com/kept/spec",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("broken =\n%v\nwant\n%v", got, want)
	}

	// The guard: a link that does resolve must not be reported, or the check
	// above would pass without ever having fired. One of each kind.
	for _, b := range c.Broken() {
		switch b.Link.Target {
		case canonID, "canon/portrait.png", "https://example.com/kept/spec":
			t.Errorf("%s resolves and was reported broken", b.Link.Target)
		}
	}
}

func link(target string, kind model.RefKind, relation string) model.Link {
	return model.Link{Target: target, TargetKind: kind, Relation: relation}
}

// A reference inside a code fence or a code span is a sample, not an edge.
func TestBodyLinksIgnoreCodeFencesAndSpans(t *testing.T) {
	c := load(t, fixture)
	for _, e := range c.All() {
		for _, l := range e.Links {
			if strings.Contains(l.Target, "inside-a-") {
				t.Errorf("%s links to %s, which is inside a code sample", e.Artifact.ID, l.Target)
			}
		}
	}
}

// Unknown fields intact and available: a field no schema declares survives the
// read, keeps its value, and is reachable without going back to the file.
func TestUnknownFieldsSurviveTheRead(t *testing.T) {
	c := load(t, fixture)
	e := get(t, c, promiseID)

	for name, want := range map[string]any{"owner": "paul", "review_every": "90d"} {
		got, present := e.Artifact.Fields[name]
		if !present {
			t.Errorf("field %q did not survive the read", name)
			continue
		}
		if got != want {
			t.Errorf("field %q = %v, want %v", name, got, want)
		}
	}

	// A top-level key nobody declares survives too, on the frontmatter the
	// artifact was read from.
	if got := e.Frontmatter["reviewed_by"]; got != "the operator" {
		t.Errorf("frontmatter reviewed_by = %v, want %q", got, "the operator")
	}

	sc, ok := c.Schemas.For("promise")
	if !ok {
		t.Fatal("no promise schema")
	}
	if want := []string{"owner", "review_every"}; !reflect.DeepEqual(e.Undeclared(sc), want) {
		t.Errorf("Undeclared = %v, want %v", e.Undeclared(sc), want)
	}

	// Undeclared is not a violation. Nothing about these two may appear as a
	// problem against this artifact.
	for _, p := range c.Problems {
		if p.ID != promiseID {
			continue
		}
		if strings.Contains(p.Detail, "owner") || strings.Contains(p.Detail, "review_every") {
			t.Errorf("an undeclared field was reported as a problem: %s", p)
		}
	}
}

// The guard for the test above: leniency is not the absence of checking. A
// required field nobody wrote, and an enum value the schema does not allow, are
// both reported — and neither stops the corpus loading.
func TestValidationReportsAndDoesNotRefuse(t *testing.T) {
	c := load(t, fixture)
	id := "promise:reported-not-refused"

	if _, ok := c.Get(id); !ok {
		t.Fatalf("%s did not load; a field violation is a report, not a refusal", id)
	}

	var details []string
	for _, p := range c.Problems {
		if p.ID == id && p.Kind == FieldViolation {
			details = append(details, p.Detail)
		}
	}
	joined := strings.Join(details, "\n")
	for _, want := range []string{"statement", "trigger", "on-change"} {
		if !strings.Contains(joined, want) {
			t.Errorf("no field violation mentions %q; got:\n%s", want, joined)
		}
	}
}

// Renaming a file changes nothing about what it read. Identity is the slug in
// frontmatter and the filename is not identity, so the artifacts a rename
// produces are the artifacts it replaced.
func TestRenamingAFileChangesNothingItRead(t *testing.T) {
	root := t.TempDir()
	copyFixture(t, fixture, root)

	before := load(t, root)
	beforeArtifacts := artifactsByID(before)
	beforePath := get(t, before, promiseID).Path

	from := filepath.Join(root, "promise", "core-packages-are-tested.md")
	to := filepath.Join(root, "promise", "renamed-and-not-the-slug.md")
	if err := os.Rename(from, to); err != nil {
		t.Fatal(err)
	}

	after := load(t, root)
	afterEntry := get(t, after, promiseID)

	// The rename must have been real, or nothing below is a test.
	if afterEntry.Path == beforePath {
		t.Fatalf("the file was not renamed: still %s", afterEntry.Path)
	}
	if want := "promise/renamed-and-not-the-slug.md"; afterEntry.Path != want {
		t.Fatalf("Path = %q, want %q", afterEntry.Path, want)
	}

	if !reflect.DeepEqual(artifactsByID(after), beforeArtifacts) {
		t.Error("renaming a file changed what the corpus read")
	}
	if !reflect.DeepEqual(afterEntry.Frontmatter, get(t, before, promiseID).Frontmatter) {
		t.Error("renaming a file changed the frontmatter it read")
	}
	if !reflect.DeepEqual(afterEntry.Links, get(t, before, promiseID).Links) {
		t.Error("renaming a file changed the links it resolved")
	}
	if !reflect.DeepEqual(ids(after), ids(before)) {
		t.Errorf("ids after = %v, before = %v", ids(after), ids(before))
	}
}

// artifactsByID is what the corpus read, with nothing about where it read it.
func artifactsByID(c *Corpus) map[string]*model.Artifact {
	out := map[string]*model.Artifact{}
	for _, e := range c.All() {
		out[e.Artifact.ID] = e.Artifact
	}
	return out
}

// The same rule from the other side: a file whose name never matched its slug
// is read by its slug, and the divergence is reported rather than obeyed.
func TestFilenameIsNotIdentity(t *testing.T) {
	c := load(t, fixture)
	e := get(t, c, canonID)

	if e.Path != "canon/correctness.md" {
		t.Fatalf("Path = %q", e.Path)
	}
	if _, ok := c.Get("canon:correctness"); ok {
		t.Error("the corpus indexed an artifact under its filename")
	}

	var reported bool
	for _, p := range c.Problems {
		if p.ID == canonID && p.Kind == PathMismatch {
			reported = true
		}
	}
	if !reported {
		t.Error("a filename that is not the slug was not reported")
	}
}

// Two artifacts claiming one slug fail the load, with both paths named.
func TestTwoArtifactsClaimingOneSlugFailTheLoad(t *testing.T) {
	c, err := Load(duplicate, duplicate)
	if err == nil {
		t.Fatalf("the corpus loaded with a duplicate id: %v", ids(c))
	}

	dup, ok := errors.AsType[*DuplicateIDError](err)
	if !ok {
		t.Fatalf("error is %T, want *DuplicateIDError: %v", err, err)
	}
	if len(dup.Claims) != 1 {
		t.Fatalf("Claims = %+v, want one contested id", dup.Claims)
	}
	claim := dup.Claims[0]
	if claim.ID != "promise:tests-run" {
		t.Errorf("ID = %q, want %q", claim.ID, "promise:tests-run")
	}

	// Both paths, not one and not a count: whoever has to fix this should not
	// have to go looking for the other half of it.
	want := []string{"promise/first.md", "promise/second.md"}
	if !reflect.DeepEqual(claim.Paths, want) {
		t.Errorf("Paths = %v, want %v", claim.Paths, want)
	}
	for _, path := range want {
		if !strings.Contains(err.Error(), path) {
			t.Errorf("the error message does not name %s: %s", path, err)
		}
	}
}

// A slug that cannot be a path component would name a truth stream nobody
// meant, so it is refused as an identity and reported.
func TestASlugMustBeAPathComponent(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "promise", "traversal.md"), "---\nid: ../../escape\n---\n\nBody.\n")

	c := load(t, root)
	if len(c.All()) != 0 {
		t.Errorf("an unusable slug became an artifact: %v", ids(c))
	}
	if !hasKind(c, BadID) {
		t.Errorf("problems = %v, want one of kind %s", c.Problems, BadID)
	}
}

// A file in a type directory with no frontmatter is not an artifact: becoming
// one is placement plus frontmatter, and placement alone is not enough.
func TestPlacementAloneIsNotAnArtifact(t *testing.T) {
	c := load(t, fixture)
	for _, e := range c.All() {
		if strings.HasSuffix(e.Path, "not-an-artifact.md") {
			t.Fatalf("%s became %s", e.Path, e.Artifact.ID)
		}
	}
	if !hasKind(c, NoFrontmatter) {
		t.Errorf("a file with no frontmatter was not reported")
	}
}

// A checkout with CRLF line endings and a byte-order mark reads as the same
// content, which is the property the binding hash later has to hold too.
func TestLineEndingsAndAByteOrderMarkDoNotChangeWhatIsRead(t *testing.T) {
	body := "---\nid: same-either-way\ntype: promise\ntitle: Same\nfields:\n  statement: it reads the same\n---\n\nProse, and more prose.\n"

	plain := t.TempDir()
	write(t, filepath.Join(plain, "promise", "a.md"), body)

	odd := t.TempDir()
	write(t, filepath.Join(odd, "promise", "a.md"), "\uFEFF"+strings.ReplaceAll(body, "\n", "\r\n"))

	got := load(t, odd)
	if !reflect.DeepEqual(artifactsByID(got), artifactsByID(load(t, plain))) {
		t.Error("a CRLF checkout read differently from an LF one")
	}
	if _, ok := got.Get("promise:same-either-way"); !ok {
		t.Errorf("the artifact did not load: %v", got.Problems)
	}
}

// A schema that cannot be read is an error rather than a diagnostic: every
// artifact of the kind would otherwise be checked against nothing.
func TestAnUnreadableSchemaStopsTheLoad(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "_schema", "promise.yaml"), "type: promise\nfields: [this is not a map\n")
	write(t, filepath.Join(root, "promise", "a.md"), "---\nid: a\n---\n\nBody.\n")

	if _, err := Load(root, root); err == nil {
		t.Error("a corpus with an unparseable schema loaded")
	}
}

// A manifest speaks for something that cannot carry links of its own, and its
// links are checked exactly as an artifact's are — otherwise the one place a
// person cannot see a broken link by reading the artifact is the one place
// nothing checks.
func TestAManifestForAFileReportsItsBrokenLinks(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "canon", "held.md"), "---\nid: held\n---\n\nProse.\n")
	write(t, filepath.Join(root, "canon", "portrait.png"), "not really a png")
	write(t, filepath.Join(root, "_links", "portrait.yaml"),
		"subject: canon/portrait.png\nlinks:\n  - { target: canon:held, relation: about }\n"+
			"  - { target: canon:nobody-claims-this, relation: about }\n")

	c := load(t, root)

	var broken []string
	for _, b := range c.Broken() {
		if b.From != "canon/portrait.png" {
			t.Errorf("broken link attributed to %q", b.From)
		}
		broken = append(broken, b.Link.Target)
	}
	if want := []string{"canon:nobody-claims-this"}; !reflect.DeepEqual(broken, want) {
		t.Errorf("broken = %v, want %v", broken, want)
	}
}

// A manifest may not speak for something outside the workspace. It is reported
// as an escape rather than as a subject nothing claims, because those are
// different mistakes and only one of them is fixed by creating the file.
func TestAManifestSubjectStaysInsideTheWorkspace(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "_links", "escape.yaml"),
		"subject: ../../../etc/passwd\nlinks:\n  - { target: canon:anything }\n")

	c := load(t, root)
	if !hasKind(c, OutsideWorkspace) {
		t.Errorf("problems = %v, want one of kind %s", c.Problems, OutsideWorkspace)
	}
	// And its links are not resolved, because nothing owns them: the subject
	// was refused before anything was read.
	if len(c.Broken()) != 0 {
		t.Errorf("links of a refused subject were resolved: %v", c.Broken())
	}
}

func TestLookupResolvesASlugAndRefusesAnAmbiguousOne(t *testing.T) {
	c := load(t, fixture)

	if e, err := c.Lookup("kernel-correctness"); err != nil || e.Artifact.ID != canonID {
		t.Errorf("Lookup by slug = %v, %v", e, err)
	}
	if e, err := c.Lookup(canonID); err != nil || e.Artifact.ID != canonID {
		t.Errorf("Lookup by ref = %v, %v", e, err)
	}
	if _, err := c.Lookup("no-such-artifact"); err == nil {
		t.Error("Lookup of a missing artifact succeeded")
	}

	// Two types answering to one slug is a question, not a guess.
	root := t.TempDir()
	write(t, filepath.Join(root, "promise", "a.md"), "---\nid: shared\n---\n\nOne.\n")
	write(t, filepath.Join(root, "canon", "b.md"), "---\nid: shared\n---\n\nTwo.\n")

	two := load(t, root)
	_, err := two.Lookup("shared")
	if err == nil {
		t.Fatal("an ambiguous slug resolved")
	}
	for _, want := range []string{"canon:shared", "promise:shared"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error does not name %s: %s", want, err)
		}
	}
}

func TestFindRootReadsTheConfiguredCorpus(t *testing.T) {
	base := t.TempDir()
	write(t, filepath.Join(base, ConfigDir, ConfigFile), "corpus: docs\n")
	write(t, filepath.Join(base, "docs", "promise", "a.md"), "---\nid: a\n---\n\nBody.\n")

	// From a subdirectory, so the walk upward is exercised.
	root, found, err := FindRoot(filepath.Join(base, "docs", "promise"))
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(base, "docs"); root != want {
		t.Errorf("root = %q, want %q", root, want)
	}
	if found != base {
		t.Errorf("base = %q, want %q", found, base)
	}

	// A manuscript declares no corpus: there the corpus is the content, so the
	// root is the directory holding .kept.
	manuscript := t.TempDir()
	write(t, filepath.Join(manuscript, ConfigDir, ConfigFile), "# nothing declared\n")
	root, _, err = FindRoot(manuscript)
	if err != nil || root != manuscript {
		t.Errorf("FindRoot on a manuscript = %q, %v; want %q", root, err, manuscript)
	}

	if _, _, err := FindRoot(t.TempDir()); !errors.Is(err, ErrNoConfig) {
		t.Errorf("FindRoot with no config = %v, want ErrNoConfig", err)
	}
}

// The corpus this repository keeps about itself must load with the schemas it
// ships, or the first reader of either is the one who finds out.
func TestTheShippedCorpusLoads(t *testing.T) {
	root := filepath.Join("..", "..", "docs")
	c := load(t, root)

	for _, want := range []string{"promise", "canon", "fact", "desire"} {
		if _, ok := c.Schemas.For(want); !ok {
			t.Errorf("no shipped schema for %q", want)
		}
	}
}

func hasKind(c *Corpus, kind ProblemKind) bool {
	for _, p := range c.Problems {
		if p.Kind == kind {
			return true
		}
	}
	return false
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func copyFixture(t *testing.T, from, to string) {
	t.Helper()
	if err := os.RemoveAll(to); err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(to, os.DirFS(from)); err != nil {
		t.Fatal(err)
	}
}

// A required field written at the top level rather than under fields: is the
// first mistake a new author makes, and reporting it as two unrelated facts —
// "required and not set" beside "here is a key nothing declares" — leaves them
// staring at a value the tool can see and will not mention.
func TestAMisnestedRequiredFieldSaysWhereItProbablyWent(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "_schema", "promise.yaml"),
		"type: promise\nhash: [statement]\nfields:\n  statement: { type: string, required: true }\n  title:     { type: string, required: true }\n")

	// The mistake: statement at the top level, where nothing declares it.
	write(t, filepath.Join(root, "promise", "misnested.md"),
		"---\nid: misnested\nstatement: every package under internal/ has a test\n---\n\nWhy.\n")

	// The guard: the same field missing, with nothing of that name anywhere. A
	// hint here would be a guess.
	write(t, filepath.Join(root, "promise", "just-absent.md"),
		"---\nid: just-absent\nfields:\n  hold_when: obs.tests.failed == 0\n---\n\nWhy.\n")

	// The other guard: a schema may declare a field called title, which is the
	// collision the fields: block exists to prevent. A top-level title: is the
	// envelope's own and means what it says, so it is never the cause.
	write(t, filepath.Join(root, "promise", "titled.md"),
		"---\nid: titled\ntitle: An ordinary title\nfields:\n  statement: it says something\n---\n\nWhy.\n")

	c := load(t, root)
	const hint = "did you mean to nest it under fields:?"

	got := violations(c, "promise:misnested")
	if !strings.Contains(got, "statement") || !strings.Contains(got, hint) {
		t.Errorf("a misnested statement was not named as the likely cause:\n%s", got)
	}
	// Both guards are only guards if the violation they qualify is there at
	// all, so each is checked for the report before it is checked for silence.
	for _, id := range []string{"promise:just-absent", "promise:titled"} {
		got := violations(c, id)
		if !strings.Contains(got, "required by promise") {
			t.Fatalf("%s reported no missing required field, so the guard proves nothing:\n%s", id, got)
		}
		if strings.Contains(got, hint) {
			t.Errorf("%s was reported as misnested and nothing of that name is at the top level:\n%s", id, got)
		}
	}
}

func violations(c *Corpus, id string) string {
	var out []string
	for _, p := range c.Problems {
		if p.ID == id && p.Kind == FieldViolation {
			out = append(out, p.Detail)
		}
	}
	return strings.Join(out, "\n")
}

// A predicate's references are checked when the corpus loads, which turns most
// of the missing-field problem from a runtime three-valued question into an
// error a person can fix. It is a report and not a refusal, by the rule at the
// top of problem.go: a predicate nobody can evaluate does not make the map from
// id to artifact wrong. See docs/decision/0036.
func TestAPredicateIsCheckedWhenTheCorpusLoads(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "_schema", "promise.yaml"),
		"type: promise\nhash: [statement, gather, hold_when]\nfields:\n  statement: { type: string }\n  gather: { type: map }\n  hold_when: { type: string }\n")

	// Reads what it gathers. Nothing to say.
	write(t, filepath.Join(root, "promise", "sound.md"),
		"---\nid: sound\nfields:\n  gather:\n    tests: { scope: \"internal/**\" }\n  hold_when: all(obs.tests.packages, p => p.failed == 0)\n---\n\nWhy.\n")

	// Reads an observation nothing gathers. It will never arrive, so this is a
	// question with no instrument rather than an instrument that could not
	// answer.
	write(t, filepath.Join(root, "promise", "ungathered.md"),
		"---\nid: ungathered\nfields:\n  gather:\n    tests: { scope: \"internal/**\" }\n  hold_when: obs.covrage.pct > 80\n---\n\nWhy.\n")

	// Will not parse at all.
	write(t, filepath.Join(root, "promise", "unparseable.md"),
		"---\nid: unparseable\nfields:\n  hold_when: \"obs.tests.failed ==\"\n---\n\nWhy.\n")

	// Parses and is unauthorable: a lambda where nothing folds.
	write(t, filepath.Join(root, "promise", "unauthorable.md"),
		"---\nid: unauthorable\nfields:\n  gather:\n    tests: {}\n  hold_when: \"has(obs.tests, p => true)\"\n---\n\nWhy.\n")

	c := load(t, root)

	// Every one of them is still an artifact. Reported, not refused.
	for _, id := range []string{"promise:sound", "promise:ungathered", "promise:unparseable", "promise:unauthorable"} {
		if _, ok := c.Get(id); !ok {
			t.Errorf("%s did not load; a predicate problem is a report, not a refusal", id)
		}
	}

	if got := problemsOf(c, "promise:sound"); got != "" {
		t.Errorf("a predicate reading what it gathers was reported:\n%s", got)
	}

	ungathered := kindsOf(c, "promise:ungathered")
	if !ungathered[UngatheredObservation] {
		t.Errorf("obs.covrage was not reported as ungathered; got %v", ungathered)
	}
	if detail := problemsOf(c, "promise:ungathered"); !strings.Contains(detail, "covrage") || !strings.Contains(detail, "tests") {
		t.Errorf("the report names neither the reference nor what is gathered:\n%s", detail)
	}

	for _, id := range []string{"promise:unparseable", "promise:unauthorable"} {
		if !kindsOf(c, id)[BadPredicate] {
			t.Errorf("%s was not reported as a bad predicate; got %v", id, kindsOf(c, id))
		}
	}

	// A refusal says where. An expression is authored on one line inside a
	// YAML file, so the column is the whole of what a person has to go on.
	if detail := problemsOf(c, "promise:unparseable"); !strings.Contains(detail, "column") {
		t.Errorf("the report does not say where:\n%s", detail)
	}
}

// The guard for the test above: a promise with no hold_when at all is not a
// promise with a broken one. Most canon binds without being evaluated, and
// reporting every one of them would make the check noise.
func TestAnArtifactWithNoPredicateIsNotReported(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "_schema", "canon.yaml"),
		"type: canon\nhash: [statement]\nfields:\n  statement: { type: string }\n")
	write(t, filepath.Join(root, "canon", "quiet.md"),
		"---\nid: quiet\nfields:\n  statement: the kernel is correct\n---\n\nWhy.\n")

	c := load(t, root)
	if got := problemsOf(c, "canon:quiet"); got != "" {
		t.Errorf("canon with no predicate was reported:\n%s", got)
	}
}

func problemsOf(c *Corpus, id string) string {
	var out []string
	for _, p := range c.Problems {
		if p.ID == id && (p.Kind == BadPredicate || p.Kind == UngatheredObservation) {
			out = append(out, p.String())
		}
	}
	return strings.Join(out, "\n")
}

func kindsOf(c *Corpus, id string) map[ProblemKind]bool {
	out := map[ProblemKind]bool{}
	for _, p := range c.Problems {
		if p.ID == id {
			out[p.Kind] = true
		}
	}
	return out
}
