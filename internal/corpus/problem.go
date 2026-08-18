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
	// NoSchema is a type directory with no _schema/{type}.yaml. Its artifacts
	// load and are validated against nothing.
	NoSchema ProblemKind = "no-schema"
	// FieldViolation is a field that disagrees with its schema — required and
	// absent, the wrong type, or outside an enum. The value is kept either way.
	FieldViolation ProblemKind = "field-violation"
	// UndeclaredRelation is a link whose relation is not in the schema's
	// allowed_relations.
	UndeclaredRelation ProblemKind = "undeclared-relation"
	// BrokenLink is a link to an id no artifact claims. See docs/decision/0002.
	BrokenLink ProblemKind = "broken-link"
	// AmbiguousLink is an unqualified link that more than one type answers to.
	AmbiguousLink ProblemKind = "ambiguous-link"
	// MalformedLink is a link entry that is not a link — no target, or a shape
	// nothing can read.
	MalformedLink ProblemKind = "malformed-link"
	// ManifestSubject is a _links manifest naming a subject that is neither an
	// artifact nor a file inside the corpus. A subject reaching outside the
	// corpus root is refused here rather than followed. See docs/decision/0002.
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
