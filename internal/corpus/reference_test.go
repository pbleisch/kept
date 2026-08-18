package corpus

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/model"
)

// The corpus this fixture describes has these kinds, and one of them — spec —
// is also a plausible URI scheme. That collision is the whole point.
func vocabulary(names ...string) func(string) bool {
	have := map[string]bool{}
	for _, n := range names {
		have[n] = true
	}
	return func(s string) bool { return have[s] }
}

func TestAReferenceIsAnArtifactAFileOrAURI(t *testing.T) {
	has := vocabulary("promise", "canon", "spec")

	cases := []struct {
		raw  string
		want model.RefKind
		why  string
	}{
		// An artifact, because the corpus has the type.
		{"promise:tests-run", model.RefArtifact, "a declared type"},
		{"canon:kernel-correctness", model.RefArtifact, "a declared type"},

		// A URI, because nothing in the corpus is called mailto or https.
		{"https://example.com/spec", model.RefURI, "a scheme with an authority"},
		{"mailto:paul@example.com", model.RefURI, "a scheme with no authority"},
		{"urn:isbn:0451450523", model.RefURI, "a scheme whose rest has more colons"},

		// A file, because there is no colon, or a slash comes first.
		{"docs/model.md", model.RefFile, "a path"},
		{"Makefile", model.RefFile, "a bare filename is not an artifact slug"},
		{"internal/corpus/corpus.go", model.RefFile, "a path"},

		// The awkward ones.
		{"spec:rfc-1234", model.RefArtifact, "the corpus's own vocabulary wins the ambiguous shape"},
		{"spec://rfc-1234", model.RefURI, "a slug cannot contain a slash, so this cannot be an artifact"},
		{"docs/notes:draft.md", model.RefFile, "the slash comes before the colon"},
		{"9lives:x", model.RefFile, "not a declared type, and not a legal scheme either"},
		{"has space:x", model.RefFile, "a scheme has no spaces"},
	}

	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			if got := classify(tc.raw, has); got != tc.want {
				t.Errorf("classify(%q) = %q, want %q — %s", tc.raw, got, tc.want, tc.why)
			}
		})
	}
}

// The vocabulary is what the corpus has, which is more than what it declares: a
// type directory with no schema still holds artifacts, and a reference to one
// must not be read as a URI on the strength of a missing schema file.
func TestTheVocabularyIsTypesDeclaredAndTypesPresent(t *testing.T) {
	c := load(t, fixture)

	// document has artifacts and no schema; promise has both.
	for _, name := range []string{"promise", "canon", "document"} {
		if !c.hasType(name) {
			t.Errorf("%q is not in the vocabulary", name)
		}
	}
	if c.hasType("https") || c.hasType("mailto") {
		t.Error("a URI scheme is in the vocabulary")
	}

	if got := c.reference("document:loose-note").Kind; got != model.RefArtifact {
		t.Errorf("a type directory with no schema gave kind %q, want %q", got, model.RefArtifact)
	}
}

// The collision the rule accepts, and the escape hatch that survives it. Adding
// a type named after a scheme in use reinterprets the ambiguous shape — which is
// the cost 0030 records — and `scheme://` is never ambiguous, whatever the
// corpus calls its kinds.
func TestATypeNamedAfterASchemeShadowsTheAmbiguousShapeOnly(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "canon", "held.md"), "---\nid: held\n---\n\nProse.\n")
	before := load(t, root)

	if got := before.reference("spec:rfc-1234").Kind; got != model.RefURI {
		t.Errorf("with no spec type, spec:rfc-1234 is %q, want %q", got, model.RefURI)
	}
	if got := before.reference("spec://rfc-1234").Kind; got != model.RefURI {
		t.Errorf("spec://rfc-1234 is %q, want %q", got, model.RefURI)
	}

	// Someone adds the type.
	write(t, filepath.Join(root, "spec", "rfc-1234.md"), "---\nid: rfc-1234\n---\n\nProse.\n")
	after := load(t, root)

	if got := after.reference("spec:rfc-1234").Kind; got != model.RefArtifact {
		t.Errorf("with a spec type, spec:rfc-1234 is %q, want %q", got, model.RefArtifact)
	}
	// The escape hatch is unaffected, which is what makes the cost bearable.
	if got := after.reference("spec://rfc-1234").Kind; got != model.RefURI {
		t.Errorf("spec://rfc-1234 became %q once spec was a type; it must stay %q", got, model.RefURI)
	}
}

// A workspace reference is rooted at the workspace, not at the corpus. 0028
// keeps those apart, and the interesting references are to the world — source,
// config, a fixture — which is everything the corpus is not.
func TestAFileReferenceIsRootedAtTheWorkspace(t *testing.T) {
	workspace := t.TempDir()
	corpusRoot := filepath.Join(workspace, "docs")
	write(t, filepath.Join(corpusRoot, "promise", "a.md"),
		"---\nid: a\n---\n\nAbout [[internal/corpus/corpus.go]] and [[docs/promise/a.md]].\n")
	write(t, filepath.Join(workspace, "internal", "corpus", "corpus.go"), "package corpus\n")

	c, err := Load(corpusRoot, workspace)
	if err != nil {
		t.Fatal(err)
	}
	e := get(t, c, "promise:a")

	got := map[string]bool{}
	for _, l := range e.Links {
		if l.TargetKind != model.RefFile {
			t.Errorf("%s is %q, want %q", l.Target, l.TargetKind, model.RefFile)
		}
		got[l.Target] = l.Broken
	}
	// Rooted at the workspace: the source file resolves, and the corpus-relative
	// spelling of the artifact's own file does not, because docs/ is not the root.
	want := map[string]bool{"internal/corpus/corpus.go": false, "docs/promise/a.md": false}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("links = %v, want %v", got, want)
	}
}

func TestAFileReferenceMayNotEscapeTheWorkspace(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "promise", "a.md"),
		"---\nid: a\n---\n\nAbout [[../../../etc/passwd]].\n")

	c := load(t, root)
	if !hasKind(c, OutsideWorkspace) {
		t.Fatalf("problems = %v, want one of kind %s", c.Problems, OutsideWorkspace)
	}
	e := get(t, c, "promise:a")
	if len(e.Links) != 1 || !e.Links[0].Broken {
		t.Errorf("an escaping reference was not refused: %+v", e.Links)
	}

	// A leading slash is the workspace root, not the filesystem root, so it is
	// contained rather than absolute.
	if got := c.reference("/promise/a.md"); got.Escapes || got.Path != "promise/a.md" {
		t.Errorf("/promise/a.md read as %+v, want the workspace-relative promise/a.md", got)
	}
}

// A URI is checked for shape and never for reach. Reachability is a question
// for the world: it needs a deadline, it has to be able to answer "nobody
// knows", and its answer belongs to a truth series. None of that is increment
// 1's, and a linter that fetched would be a gatherer wearing a linter's clothes.
//
// The proof that nothing is fetched is the .invalid TLD, which RFC 2606
// guarantees never resolves. If this code touched the network the reference
// below would fail to resolve and be reported broken; it is not.
func TestAURIIsCheckedForShapeAndNeverForReach(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "promise", "a.md"), `---
id: a
links:
  - { target: "https://nothing.invalid/never-resolves" }
  - { target: "https://example.com/fine" }
  - { target: "http://%zz-not-a-uri" }
---

Prose.
`)

	c := load(t, root)
	e := get(t, c, "promise:a")

	broken := map[string]bool{}
	for _, l := range e.Links {
		if l.TargetKind != model.RefURI {
			t.Errorf("%s is %q, want %q", l.Target, l.TargetKind, model.RefURI)
		}
		broken[l.Target] = l.Broken
	}
	if broken["https://nothing.invalid/never-resolves"] {
		t.Error("an unreachable URI was reported broken; nothing here may ask the network")
	}
	if broken["https://example.com/fine"] {
		t.Error("a well-formed URI was reported broken")
	}
	if !broken["http://%zz-not-a-uri"] {
		t.Error("a URI that will not parse was not reported")
	}
	if !hasKind(c, MalformedURI) {
		t.Errorf("problems = %v, want one of kind %s", c.Problems, MalformedURI)
	}

	// The guard on the guard: nothing anywhere reported a URI as a broken link,
	// which is the kind reserved for something that was actually looked for.
	for _, p := range c.Problems {
		if p.Kind == BrokenLink && strings.Contains(p.Detail, "://") {
			t.Errorf("a URI was reported as a broken link: %s", p)
		}
	}
}

// An artifact link target names its type. The bare-slug convenience is for a
// person at a terminal, where an error can be read; a tracked reference whose
// meaning depends on what else exists is one nobody can review.
func TestAnArtifactLinkTargetNamesItsType(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "canon", "held.md"), "---\nid: held\n---\n\nProse.\n")
	write(t, filepath.Join(root, "promise", "a.md"), "---\nid: a\n---\n\nAbout [[held]] and [[canon:held]].\n")

	c := load(t, root)
	e := get(t, c, "promise:a")

	kinds := map[string]model.RefKind{}
	for _, l := range e.Links {
		kinds[l.Target] = l.TargetKind
	}
	if kinds["canon:held"] != model.RefArtifact {
		t.Errorf("canon:held is %q, want %q", kinds["canon:held"], model.RefArtifact)
	}
	if kinds["held"] != model.RefFile {
		t.Errorf("a bare slug is %q, want %q — only a person may type one", kinds["held"], model.RefFile)
	}

	// Lookup keeps the convenience, because a person is present to read the error.
	if got, err := c.Lookup("held"); err != nil || got.Artifact.ID != "canon:held" {
		t.Errorf("Lookup(held) = %v, %v", got, err)
	}
}

// Every source takes every kind. The captain's reason for manifests is the
// general form of 0030's: a third party must be able to assert a link about an
// artifact without editing that artifact.
func TestEverySourceTakesEveryKind(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "canon", "held.md"), "---\nid: held\n---\n\nProse.\n")
	write(t, filepath.Join(root, "canon", "portrait.png"), "not really a png")
	write(t, filepath.Join(root, "_links", "third-party.yaml"), `subject: canon:held
links:
  - { target: canon/portrait.png, relation: about }
  - { target: "https://example.com/review", relation: about }
`)

	c := load(t, root)
	e := get(t, c, "canon:held")

	// The manifest's assertions reached the artifact without the artifact being
	// touched: its own file declares no links at all.
	got := map[string]model.RefKind{}
	for _, l := range e.Links {
		if l.From != FromManifest {
			t.Errorf("%s came from %q, want %q", l.Target, l.From, FromManifest)
		}
		if l.Broken {
			t.Errorf("%s was reported broken", l.Target)
		}
		got[l.Target] = l.TargetKind
	}
	want := map[string]model.RefKind{
		"canon/portrait.png":         model.RefFile,
		"https://example.com/review": model.RefURI,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("manifest links = %v, want %v", got, want)
	}
}

// A manifest subject takes every kind too, and a subject that is not an artifact
// still has its links checked — that is the one place a person cannot see a
// broken link by reading the artifact.
func TestAManifestSubjectTakesEveryKind(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "canon", "held.md"), "---\nid: held\n---\n\nProse.\n")
	write(t, filepath.Join(root, "canon", "portrait.png"), "not really a png")
	write(t, filepath.Join(root, "_links", "file.yaml"),
		"subject: canon/portrait.png\nlinks:\n  - { target: canon:no-such-canon }\n")
	write(t, filepath.Join(root, "_links", "uri.yaml"),
		"subject: https://example.com/spec\nlinks:\n  - { target: canon:held }\n")
	write(t, filepath.Join(root, "_links", "gone.yaml"),
		"subject: canon/no-such-file.png\nlinks:\n  - { target: canon:held }\n")

	c := load(t, root)

	kinds := map[string]model.RefKind{}
	for _, m := range c.Manifests {
		kinds[m.Subject] = m.Reference.Kind
	}
	want := map[string]model.RefKind{
		"canon/portrait.png":       model.RefFile,
		"https://example.com/spec": model.RefURI,
		"canon/no-such-file.png":   model.RefFile,
	}
	if !reflect.DeepEqual(kinds, want) {
		t.Errorf("subject kinds = %v, want %v", kinds, want)
	}

	// A subject nothing answers to is reported as a subject, not as a link.
	if !hasKind(c, ManifestSubject) {
		t.Errorf("problems = %v, want one of kind %s", c.Problems, ManifestSubject)
	}
	// And a file subject's own broken target is still found.
	var found bool
	for _, b := range c.Broken() {
		if b.From == "canon/portrait.png" && b.Link.Target == "canon:no-such-canon" {
			found = true
		}
	}
	if !found {
		t.Errorf("a file subject's broken target was not reported: %v", c.Broken())
	}
}

func TestIsScheme(t *testing.T) {
	for _, ok := range []string{"http", "https", "mailto", "urn", "a", "x-y", "a+b", "a.b", "h2"} {
		if !isScheme(ok) {
			t.Errorf("isScheme(%q) = false", ok)
		}
	}
	for _, bad := range []string{"", "9lives", "-x", "a b", "a/b", "a_b", "docs/notes"} {
		if isScheme(bad) {
			t.Errorf("isScheme(%q) = true", bad)
		}
	}
}

func TestWorkspacePathCleansAndContains(t *testing.T) {
	cases := []struct {
		raw     string
		clean   string
		escapes bool
	}{
		{"docs/model.md", "docs/model.md", false},
		{"/docs/model.md", "docs/model.md", false},
		{"./docs/model.md", "docs/model.md", false},
		{"docs/../internal/x.go", "internal/x.go", false},
		{"../outside", "", true},
		{"../../etc/passwd", "", true},
		{"docs/../../outside", "", true},
		{".", "", true},
		{"", "", true},
	}
	for _, tc := range cases {
		clean, escapes := workspacePath(tc.raw)
		if escapes != tc.escapes {
			t.Errorf("workspacePath(%q) escapes = %v, want %v", tc.raw, escapes, tc.escapes)
			continue
		}
		if !escapes && clean != tc.clean {
			t.Errorf("workspacePath(%q) = %q, want %q", tc.raw, clean, tc.clean)
		}
	}
}

// The corpus root and the workspace root are the same directory for a
// manuscript, because there the corpus is the content.
func TestAManuscriptHasOneRoot(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "chapter", "one.md"), "---\nid: one\n---\n\nAbout [[chapter/one.md]].\n")

	c, err := Load(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if c.Workspace != root {
		t.Errorf("Workspace = %q, want %q", c.Workspace, root)
	}
	if l := get(t, c, "chapter:one").Links; len(l) != 1 || l[0].Broken {
		t.Errorf("links = %+v, want one that resolves", l)
	}
}
