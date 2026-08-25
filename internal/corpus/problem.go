package corpus

import (
	"fmt"
	"sort"
)

// Problem is something the corpus said that a reader should know about but that
// does not stop it being read.
//
// The line between a Problem and a load error is not severity, it is identity:
// a load error is one that makes the corpus's map from id to artifact wrong, and
// everything else is a diagnostic. Two artifacts claiming one id is a load
// error because after it the map is a lie. A required field nobody wrote is a
// diagnostic, because the artifact still is what it is.
type Problem struct {
	Kind ProblemKind

	// Path is where the problem was found, relative to the corpus root. It is
	// present even when ID is not, because a file with no id is exactly the
	// case a reader needs a path for.
	Path string
	// ID is the artifact the problem belongs to, when one could be read.
	ID string

	Detail string
}

func (p Problem) String() string {
	where := p.Path
	if where == "" {
		where = p.ID
	}
	return fmt.Sprintf("%s: %s: %s", where, p.Kind, p.Detail)
}

// ProblemKind is what kind of thing was wrong. Each traces to a decision that
// says why it is worth saying.
type ProblemKind string

const (
	// NoFrontmatter is a markdown file in a type directory with no frontmatter
	// block. It is not an artifact: becoming one is placement plus frontmatter,
	// and placement alone is not enough. See docs/decision/0028.
	NoFrontmatter ProblemKind = "no-frontmatter"
	// Unreadable is a frontmatter block that is not YAML. The file has no
	// readable id, so it cannot be an artifact — but one such file does not
	// make the corpus unparseable, so the rest of it still loads.
	Unreadable ProblemKind = "unreadable"
	// NoID is frontmatter with no id. Identity is authored, so a file without
	// one is not yet an artifact; kept doctor is what offers to mint it. See
	// docs/decision/0021 and 0029.
	NoID ProblemKind = "no-id"
	// BadID is an id that cannot be used. The truth and disposition streams are
	// keyed by the slug as a path component, so a slug that is not one would
	// name a file nobody meant. See docs/decision/0021.
	BadID ProblemKind = "bad-id"
	// TypeMismatch is frontmatter declaring a type its directory contradicts.
	// The directory wins, because placement is what makes an artifact a kind.
	// See docs/decision/0028.
	TypeMismatch ProblemKind = "type-mismatch"
	// PathMismatch is a filename that is not the slug. It changes nothing —
	// the filename is not identity — and it is reported because a corpus is
	// easier to read when the two agree. See docs/decision/0021.
	PathMismatch ProblemKind = "path-mismatch"
	// IDShape is a well-formed id that is not the shape its kind's schema
	// declares — an unnumbered decision in a corpus where every citation of one
	// writes the number. The id is usable and the artifact loads under it; what
	// is wrong is that it is an identity of a shape nothing else in the kind
	// has, which is a convention nobody was checking. See docs/decision/0069.
	IDShape ProblemKind = "id-shape"
	// DuplicateNumber is two artifacts of a numbered kind claiming one ordinal.
	// It is not a duplicate id — the slugs differ, so the map from id to
	// artifact is still right — and it is not made wrong by anything kept does:
	// a number is claimed by whoever reaches the shared branch first, so two
	// branches cut from one commit both mint the same free one. Reported so
	// that the second is renumbered while that is still free. See
	// docs/decision/0069.
	DuplicateNumber ProblemKind = "duplicate-number"
	// NoSchema is a type directory with no _schema/{type}.yaml. Its artifacts
	// load and are validated against nothing.
	NoSchema ProblemKind = "no-schema"
	// FieldViolation is a field that disagrees with its schema — required and
	// absent, the wrong type, or outside an enum. The value is kept either way.
	FieldViolation ProblemKind = "field-violation"
	// UndeclaredRelation is a link whose relation is not in the schema's
	// allowed_relations.
	UndeclaredRelation ProblemKind = "undeclared-relation"
	// BrokenLink is a reference with nothing at the other end: an id no
	// artifact claims, or a path no file in the workspace answers to. A URI is
	// never reported this way, because whether one leads anywhere is a question
	// for the world. See docs/decision/0002 and 0030.
	BrokenLink ProblemKind = "broken-link"
	// OutsideWorkspace is a file reference that resolves above the workspace
	// root. It is refused rather than followed: a reference that can name
	// anything on the machine is one nobody can review. See docs/decision/0030.
	OutsideWorkspace ProblemKind = "outside-workspace"
	// MalformedURI is a reference read as a URI that will not parse as one. It
	// says nothing about whether the URI resolves, which is not asked here.
	MalformedURI ProblemKind = "malformed-uri"
	// MalformedLink is a link entry that is not a link — no target, or a shape
	// nothing can read.
	MalformedLink ProblemKind = "malformed-link"
	// BadPredicate is a hold_when that will not parse, or that is
	// unauthorable — a fold over something that is not a list, a function
	// nothing declares, a pattern that will not compile. It is reported at
	// load because it is answerable without looking at the world, and because
	// the alternative is discovering it in the code path that decides a
	// verdict. See docs/decision/0036.
	BadPredicate ProblemKind = "bad-predicate"
	// UngatheredObservation is a predicate reading obs.<name> for a name the
	// artifact declares no gatherer for. Nothing will ever produce it, so the
	// promise is a question with no instrument — which is a different thing
	// from a gatherer that could not answer, and is a mistake rather than an
	// unknown. See docs/decision/0036.
	UngatheredObservation ProblemKind = "ungathered-observation"
	// UnknownGatherer is a gather entry naming a gatherer kept does not have.
	// Nothing will ever run it, so every promise resting on it is a question
	// with no instrument — which is a mistake rather than an unknown. See
	// docs/decision/0005 and 0034.
	UnknownGatherer ProblemKind = "unknown-gatherer"
	// BadGatherSpec is a gather entry naming a gatherer that exists and asking
	// it for something it cannot do: an arg nothing accepts, a `command` with
	// no extractor, an extractor that will not parse. Reported at load for
	// BadPredicate's reason — the alternative is discovering it in the code
	// path that decides a verdict. See docs/decision/0023 and 0036.
	BadGatherSpec ProblemKind = "bad-gather-spec"
	// MisplacedField is a frontmatter key written at the top level that the
	// kind's own schema declares as a field. Nothing reads it there — Fields is
	// populated from the fields: block alone — so the value a person wrote is
	// ignored by every path that asks the artifact what it says, and by the
	// binding hash when the schema binds on it.
	//
	// It is reported and never refused, and the artifact still loads carrying the
	// key: the file is wrong about where it put a value, not about what it is. It
	// is narrow on purpose — a top-level key nobody declares is the author's own
	// and is reported by nothing, which is what 0010 reserves that level for. What
	// is reported is the disagreement: a name the schema has an opinion about,
	// written where that opinion is never consulted. See docs/decision/0003 and
	// 0010.
	MisplacedField ProblemKind = "misplaced-field"
	// MissingSection is a body section the schema declares and the artifact
	// does not have. The field is absent rather than empty, and the artifact
	// still loads and can still be ratified — this is reported because it is
	// the half of a heading typo that says what to do about it. An H2 nobody
	// declared is not reported at all, the way an undeclared field is not. See
	// docs/decision/0040.
	MissingSection ProblemKind = "missing-section"
	// AmbiguousSection is two headings producing one section identifier. It is
	// reported and it is unbindable: nothing here picks one, because which
	// heading comes first is a property of how the document is ordered. See
	// docs/decision/0040.
	AmbiguousSection ProblemKind = "ambiguous-section"
	// ManifestSubject is a _links manifest naming a subject with nothing at the
	// other end. Kept distinct from BrokenLink because a manifest whose subject
	// is gone asserts nothing at all, where a manifest with one broken target
	// still asserts the rest. See docs/decision/0030.
	ManifestSubject ProblemKind = "manifest-subject"
)

func sortProblems(ps []Problem) {
	sort.SliceStable(ps, func(i, j int) bool {
		a, b := ps[i], ps[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.Detail < b.Detail
	})
}
