package corpus

import (
	"net/url"
	"path"
	"strings"

	"github.com/pbleisch/kept/internal/model"
)

// Reference is one end of a link, read. Both ends of every link, in all three
// sources, are one of these — because the person asserting a link is not always
// the author of either end. See docs/decision/0030.
type Reference struct {
	// Raw is the reference exactly as it was authored, and it is what is
	// printed and compared. Nothing rewrites it.
	Raw  string
	Kind model.RefKind

	// Path is the cleaned workspace-relative path of a file reference, and
	// empty for the other two kinds. It is derived for the existence check and
	// is not what the link says.
	Path string
	// Escapes is set when a file reference resolves outside the workspace root.
	// Such a reference is refused rather than followed: nothing outside the
	// workspace is read to decide what a link means.
	Escapes bool
}

// classify reads a reference's kind from its shape, against the types this
// corpus has. Only one shape is ambiguous — `word:rest`, which is both what an
// artifact reference looks like and what a URI looks like — and the corpus's
// own vocabulary settles that one.
//
// The order matters and each step rules out what follows it:
//
//  1. No colon, or a slash before the first colon, and it is a path. A type
//     name cannot contain a slash, so nothing else it could be.
//  2. A colon followed by "//" and it is a URI. A slug cannot contain a slash
//     either (0021), so no artifact reference can take this shape — which is
//     the escape hatch when a type name and a scheme collide.
//  3. Otherwise the corpus's vocabulary wins: a type it has makes it an
//     artifact reference, a syntactically valid scheme makes it a URI, and
//     anything else is a path with a colon in its name.
//
// See docs/decision/0030 for why the vocabulary wins step 3 and what that
// costs.
func classify(raw string, hasType func(string) bool) model.RefKind {
	colon := strings.IndexByte(raw, ':')
	slash := strings.IndexByte(raw, '/')

	if colon < 0 || (slash >= 0 && slash < colon) {
		return model.RefFile
	}
	scheme, rest := raw[:colon], raw[colon+1:]
	if strings.HasPrefix(rest, "//") {
		return model.RefURI
	}
	if hasType(scheme) {
		return model.RefArtifact
	}
	if isScheme(scheme) {
		return model.RefURI
	}
	return model.RefFile
}

// isScheme reports whether s is a URI scheme by RFC 3986: a letter, then
// letters, digits, and any of "+-.".
func isScheme(s string) bool {
	for i := range len(s) {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		case i > 0 && (c >= '0' && c <= '9' || c == '+' || c == '-' || c == '.'):
		default:
			return false
		}
	}
	return len(s) > 0
}

// reference reads one reference against this corpus.
func (c *Corpus) reference(raw string) Reference {
	raw = strings.TrimSpace(raw)
	r := Reference{Raw: raw, Kind: classify(raw, c.hasType)}
	if r.Kind == model.RefFile {
		r.Path, r.Escapes = workspacePath(raw)
	}
	return r
}

// workspacePath cleans a file reference into a path relative to the workspace
// root, and says whether it escapes it.
//
// A leading slash is the workspace root rather than the filesystem root, so
// `/Makefile` and `Makefile` are the same reference. That leaves `..` as the
// only way out, and it is refused — the same bar 0021 sets for a slug, for the
// same reason: a reference that can name anything on the machine is a reference
// nobody can review.
func workspacePath(raw string) (clean string, escapes bool) {
	// Cleaned as a relative path, deliberately. Anchoring it first — cleaning
	// "/" + raw — would let path.Clean absorb a leading ".." against the root
	// and hand back a path inside the workspace, which is the containment check
	// answering its own question wrongly.
	clean = path.Clean(strings.TrimPrefix(raw, "/"))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return raw, true
	}
	return clean, false
}

// wellFormed reports whether a URI reference parses. It says nothing about
// whether anything is there: reachability is a question for the world, so it
// needs a deadline (0026), it has to be able to answer "nobody knows" (0004),
// and its answer has to name the world it measured (0014). None of that is a
// property of the text, and a link checker that reached the network would be a
// gatherer wearing a linter's clothes. See docs/decision/0030.
func wellFormed(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme != ""
}

// hasType reports whether this corpus has a type by this name — declared by a
// schema, or present as a type directory. Both count, because a corpus with no
// schema for a kind still has artifacts of it, and a reference to one of them
// must not be read as a URI on the strength of a missing file.
func (c *Corpus) hasType(name string) bool {
	return c.types[name]
}
