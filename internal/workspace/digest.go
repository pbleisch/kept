package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"

	"github.com/pbleisch/kept/internal/canonical"
)

// Algorithm is the digest a tree and a scope are folded with. It is written
// into every value, so a later change is additive rather than a
// reinterpretation of records already written.
const Algorithm = "sha256"

// treeDomain separates this fold from anything else hashed with the same
// function. It carries no version of its own: decision 0034 puts the scope
// digest inside an observation's preimage, so refining what the fold covers is
// an obs: bump and never an edit. See docs/decision/0034.
const treeDomain = "kept/tree-digest\n"

// MatchedNothing is what a scope digest reads as when its globs matched no
// path.
//
// It is a distinct outcome rather than a digest, and that is load-bearing. The
// fold over an empty set is a perfectly stable value, so a mis-typed scope
// would read as "nothing has changed" forever — the same silent permanence as
// the glob failure decision 0033 was written about, reached from the other
// side. A scope that matches nothing is always due.
const MatchedNothing = "matched:none"

// Digest is the world's tree: a content digest over the workspace root,
// honouring ignore rules.
//
// This is model.World.Tree, and it is the same number in a repository, outside
// one, and inside a box. Remote, ref and commit are provenance, filled in from
// git where git is present, and never part of it.
func (t *Tree) Digest() string { return fold(t.Files) }

// ScopeDigest is what a promise's scope currently hashes to, and how much it
// covered. Due-ness compares it against the digest on the last observation.
type ScopeDigest struct {
	// Digest is the fold over the matched paths, or empty when nothing
	// matched.
	Digest string
	Paths  int
	Bytes  int64
}

// Scope folds the paths a scope's globs match. Paths are matched under the
// workspace root, not the corpus root, because a scope is about the world.
func (t *Tree) Scope(globs []string) ScopeDigest {
	var matched []File
	var bytes int64
	for _, f := range t.Files {
		if !Match(globs, f.Path) {
			continue
		}
		matched = append(matched, f)
		bytes += f.Size
	}
	if len(matched) == 0 {
		return ScopeDigest{}
	}
	return ScopeDigest{Digest: fold(matched), Paths: len(matched), Bytes: bytes}
}

// MatchedNothing reports whether the scope's globs matched no path at all,
// which is always due and is what kept doctor surfaces as a mis-stated scope.
func (s ScopeDigest) MatchedNothing() bool { return s.Paths == 0 }

// String is the form recorded on an observation.
func (s ScopeDigest) String() string {
	if s.MatchedNothing() {
		return MatchedNothing
	}
	return s.Digest
}

// Unchanged reports whether the scope hashes to what an earlier observation
// recorded.
//
// A scope that matches nothing is never unchanged, whatever was recorded, and
// nothing recorded is never unchanged either. Both answer "due", which is the
// safe direction and the one a digest cannot deliver on its own.
func (s ScopeDigest) Unchanged(recorded string) bool {
	if s.MatchedNothing() || recorded == "" || recorded == MatchedNothing {
		return false
	}
	return s.Digest == recorded
}

// fold is the one definition both digests are: each path contributing its own
// name and a digest of its bytes, ordered by path.
//
// The encoding is decision 0031's, so the fold is unambiguous — a path and a
// digest cannot be re-read as one another, and a tree of two files cannot
// encode like a tree of one. Text is verbatim: a path is measured, not
// authored, and normalizing one would make two different names one path.
func fold(files []File) string {
	enc := canonical.NewEncoder(treeDomain, canonical.Verbatim)
	enc.List(len(files))
	for _, f := range files {
		enc.Text(f.Path)
		enc.Text(f.Digest)
	}
	return Algorithm + ":" + enc.Digest()
}

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func sumFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
