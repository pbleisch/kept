package corpus

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/schema"
)

// Entry is one artifact as it was read: what it says, and where it was found.
//
// The split between Artifact and Path is the whole of decision 0021. Artifact
// is what the corpus read, and nothing in it comes from the filename; Path is
// where it was read from, and nothing depends on it. Renaming a file therefore
// changes Path and cannot change Artifact — not by convention, but because
// Artifact has nowhere to put a filename.
type Entry struct {
	Artifact *model.Artifact

	// Path is relative to the corpus root, in slash form. A diagnostic, never
	// an identity.
	Path string

	// Frontmatter is every key the block carried, as authored, including the
	// ones no schema declares. Nothing is dropped on a read and nothing is
	// written back: kept does not rewrite a human-authored file, so unknown
	// fields have to survive a read and no more. See docs/decision/0029.
	Frontmatter map[string]any

	// Links are the artifact's edges with the source each was written in and
	// whether anything is at the other end. Artifact.Links is the same set
	// without the diagnostics.
	Links []ResolvedLink
}

// Undeclared returns the field names this artifact carries that its schema does
// not name, sorted. They are kept, not dropped — that is what leniency means —
// and this is how a reader is told which ones nothing was checking.
func (e *Entry) Undeclared(sc *schema.Schema) []string {
	if sc == nil {
		return sortedKeys(e.Artifact.Fields)
	}
	return sc.Undeclared(e.Artifact.Fields)
}

// Corpus is a loaded corpus: every artifact, keyed by "type:slug", and
// everything about it that a reader should know.
type Corpus struct {
	// Root is the directory the corpus was read from.
	Root string
	// Schemas are the kinds this corpus declares. A type directory with no
	// schema still loads; its artifacts are validated against nothing.
	Schemas *schema.Set
	// Manifests are the _links entries, for subjects that cannot carry links.
	Manifests []Manifest
	// Problems is everything the corpus said that a reader should know about
	// and that did not stop it being read. Sorted, so two loads of one corpus
	// report identically.
	Problems []Problem

	entries []*Entry
	byID    map[string]*Entry
	bySlug  map[string][]*Entry
	// orphanLinks are the edges of a manifest subject that is not an artifact.
	// Nothing owns them, and they are still links that may point at nothing.
	orphanLinks []BrokenRef
}

// DuplicateIDError is the one thing that stops a corpus loading. Two artifacts
// claiming one id makes the map from id to artifact a lie, and every durable
// stream is keyed by that id — so it is refused, and it names every path so
// that whoever has to fix it does not have to go looking. See
// docs/decision/0021.
type DuplicateIDError struct {
	// Claims is every contested id with every path claiming it, sorted.
	Claims []Claim
}

// Claim is one id and the paths that claim it.
type Claim struct {
	ID    string
	Paths []string
}

func (e *DuplicateIDError) Error() string {
	var b strings.Builder
	for i, c := range e.Claims {
		if i > 0 {
			b.WriteString("; ")
		}
		fmt.Fprintf(&b, "%d artifacts claim %s: %s", len(c.Paths), c.ID, strings.Join(c.Paths, ", "))
	}
	return b.String()
}

// Load reads a corpus. It walks the type directories under root, reads each
// artifact's identity from its own frontmatter, and resolves the three link
// sources against what it found.
//
// It returns an error only when the corpus's identity map would be wrong —
// a root that is not a directory, a schema that cannot be read, or two
// artifacts claiming one id. Everything else the corpus gets wrong is reported
// on Problems and the corpus still loads, because a reader who has just been
// told about a broken link is better served than one who has been told nothing.
func Load(root string) (*Corpus, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("corpus root %s: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("corpus root %s is not a directory", root)
	}

	schemas, err := schema.Load(filepath.Join(root, schema.Dir))
	if err != nil {
		return nil, err
	}

	c := &Corpus{
		Root:    root,
		Schemas: schemas,
		byID:    map[string]*Entry{},
		bySlug:  map[string][]*Entry{},
	}

	claims := map[string][]string{}
	kinds, err := typeDirs(root)
	if err != nil {
		return nil, err
	}
	for _, kind := range kinds {
		sc, ok := schemas.For(kind)
		if !ok {
			c.Problems = append(c.Problems, Problem{
				Kind: NoSchema, Path: kind,
				Detail: fmt.Sprintf("no %s/%s.yaml, so nothing checks what a %s must say", schema.Dir, kind, kind),
			})
		}
		if err := c.walkKind(root, kind, sc, claims); err != nil {
			return nil, err
		}
	}

	if dup := duplicates(claims); dup != nil {
		return nil, dup
	}

	manifests, problems := loadManifests(root)
	c.Manifests = manifests
	c.Problems = append(c.Problems, problems...)

	c.resolveLinks()

	sort.Slice(c.entries, func(i, j int) bool { return c.entries[i].Artifact.ID < c.entries[j].Artifact.ID })
	sortProblems(c.Problems)
	return c, nil
}

// typeDirs returns the directories under the corpus root that hold artifacts.
// A file sitting at the root is not an entity and an underscore-prefixed
// directory is machinery, so only the rest are walked. See docs/layout.md and
// docs/decision/0028.
func typeDirs(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", root, err)
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() || strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".") {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}

func (c *Corpus) walkKind(root, kind string, sc *schema.Schema, claims map[string][]string) error {
	dir := filepath.Join(root, kind)
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if path != dir && strings.HasPrefix(name, ".") {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(name, ".") || filepath.Ext(name) != ".md" {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		src, err := os.ReadFile(path)
		if err != nil {
			c.Problems = append(c.Problems, Problem{Kind: Unreadable, Path: rel, Detail: err.Error()})
			return nil
		}

		entry, problems := read(rel, kind, src, sc)
		c.Problems = append(c.Problems, problems...)
		if entry == nil {
			return nil
		}

		id := entry.Artifact.ID
		claims[id] = append(claims[id], rel)
		c.entries = append(c.entries, entry)
		// The first claimant is indexed; if a second appears the load fails
		// anyway, so which one is indexed never reaches a caller.
		if _, taken := c.byID[id]; !taken {
			c.byID[id] = entry
		}
		_, slug := SplitRef(id)
		c.bySlug[slug] = append(c.bySlug[slug], entry)
		return nil
	})
}

func duplicates(claims map[string][]string) *DuplicateIDError {
	var out []Claim
	for id, paths := range claims {
		if len(paths) < 2 {
			continue
		}
		sorted := append([]string(nil), paths...)
		sort.Strings(sorted)
		out = append(out, Claim{ID: id, Paths: sorted})
	}
	if len(out) == 0 {
		return nil
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return &DuplicateIDError{Claims: out}
}

// resolveLinks folds each artifact's three sources into one edge set and asks
// the corpus whether anything is at the other end of each.
func (c *Corpus) resolveLinks() {
	bySubject := map[string][]model.Link{}
	for _, m := range c.Manifests {
		if p, bad := m.checkSubject(c.Root, c.claimed); bad {
			c.Problems = append(c.Problems, p)
			continue
		}
		if c.claimed(m.Subject) {
			bySubject[m.Subject] = append(bySubject[m.Subject], m.Links...)
			continue
		}
		// The subject is a file, so nothing owns these edges — but a link to
		// an id nobody claims is broken wherever it was written, and a
		// manifest is the one place a person cannot see that by reading the
		// artifact.
		c.resolveManifest(m)
	}

	for _, e := range c.entries {
		id := e.Artifact.ID
		front, malformed := linksFromFrontmatter(e.Frontmatter[keyLinks])
		for _, m := range malformed {
			c.Problems = append(c.Problems, Problem{Kind: MalformedLink, Path: e.Path, ID: id, Detail: m})
		}

		_, slug := SplitRef(id)
		e.Links = merge([]sourced{
			{FromFrontmatter, front},
			{FromBody, linksFromBody(e.Artifact.Body)},
			{FromManifest, append(append([]model.Link(nil), bySubject[id]...), bySubject[slug]...)},
		})

		sc, _ := c.Schemas.For(string(e.Artifact.Kind))
		e.Artifact.Links = make([]model.Link, 0, len(e.Links))
		for i := range e.Links {
			link := &e.Links[i]
			if sc != nil && !sc.AllowsRelation(link.Relation) {
				c.Problems = append(c.Problems, Problem{
					Kind: UndeclaredRelation, Path: e.Path, ID: id,
					Detail: fmt.Sprintf("relation %q on the link to %s is not one of %v", link.Relation, link.Target, sc.AllowedRelations),
				})
			}
			if p, bad := c.checkTarget(e, *link); bad {
				link.Broken = true
				c.Problems = append(c.Problems, p)
			}
			e.Artifact.Links = append(e.Artifact.Links, link.Link)
		}
		if len(e.Artifact.Links) == 0 {
			e.Artifact.Links = nil
		}
	}
}

// resolveManifest checks the links of a subject that is not an artifact. They
// belong to a file, so they are recorded on the manifest rather than on any
// artifact, and they are checked exactly as an artifact's are.
func (c *Corpus) resolveManifest(m Manifest) {
	for _, l := range m.Links {
		link := ResolvedLink{Link: l, From: FromManifest}
		if kind, detail := c.resolve(link); detail != "" {
			link.Broken = true
			c.Problems = append(c.Problems, Problem{
				Kind: kind, Path: m.Path,
				Detail: fmt.Sprintf("the link from %s %s", m.Subject, detail),
			})
		}
		c.orphanLinks = append(c.orphanLinks, BrokenRef{From: m.Subject, Path: m.Path, Link: link})
	}
}

// claimed reports whether any artifact answers to a reference.
func (c *Corpus) claimed(ref string) bool {
	if _, ok := c.byID[ref]; ok {
		return true
	}
	kind, slug := SplitRef(ref)
	return kind == "" && len(c.bySlug[slug]) > 0
}

func (c *Corpus) checkTarget(e *Entry, link ResolvedLink) (Problem, bool) {
	kind, detail := c.resolve(link)
	if detail == "" {
		return Problem{}, false
	}
	return Problem{
		Kind: kind, Path: e.Path, ID: e.Artifact.ID,
		Detail: fmt.Sprintf("the %s %s", link.From, detail),
	}, true
}

// resolve asks whether anything is at the other end of a link, and says what is
// wrong when nothing usable is. A qualified target names one artifact or none;
// an unqualified one may be answered by more than one type, which is a question
// rather than a guess. The empty detail means the link resolves.
func (c *Corpus) resolve(link ResolvedLink) (ProblemKind, string) {
	broken := fmt.Sprintf("link to %s has nothing at the other end", link.Target)

	if kind, _ := SplitRef(link.Target); kind != "" {
		if _, ok := c.byID[link.Target]; ok {
			return "", ""
		}
		return BrokenLink, broken
	}

	switch found := c.bySlug[link.Target]; len(found) {
	case 0:
		return BrokenLink, broken
	case 1:
		return "", ""
	default:
		return AmbiguousLink, fmt.Sprintf("link to %s could mean %s; name the type",
			link.Target, strings.Join(idsOf(found), " or "))
	}
}

func idsOf(entries []*Entry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Artifact.ID
	}
	sort.Strings(out)
	return out
}

// All returns every artifact, sorted by id.
func (c *Corpus) All() []*Entry {
	out := make([]*Entry, len(c.entries))
	copy(out, c.entries)
	return out
}

// Get returns the artifact with this exact "type:slug" id.
func (c *Corpus) Get(id string) (*Entry, bool) {
	e, ok := c.byID[id]
	return e, ok
}

// Lookup resolves a reference a person typed. It takes a "type:slug", or a bare
// slug when only one type answers to it — an ambiguous one is an error naming
// the candidates rather than a guess.
func (c *Corpus) Lookup(ref string) (*Entry, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("no artifact named")
	}
	if e, ok := c.byID[ref]; ok {
		return e, nil
	}
	if kind, _ := SplitRef(ref); kind != "" {
		return nil, fmt.Errorf("no artifact %s", ref)
	}
	switch found := c.bySlug[ref]; len(found) {
	case 0:
		return nil, fmt.Errorf("no artifact %s", ref)
	case 1:
		return found[0], nil
	default:
		return nil, fmt.Errorf("%s could mean %s; name the type", ref, strings.Join(idsOf(found), " or "))
	}
}

// Broken returns every link with nothing at the other end, wherever it was
// written — an artifact's frontmatter, its prose, or a manifest speaking for
// something that cannot carry links of its own.
func (c *Corpus) Broken() []BrokenRef {
	var out []BrokenRef
	for _, e := range c.entries {
		for _, l := range e.Links {
			if l.Broken {
				out = append(out, BrokenRef{From: e.Artifact.ID, Path: e.Path, Link: l})
			}
		}
	}
	for _, o := range c.orphanLinks {
		if o.Link.Broken {
			out = append(out, o)
		}
	}
	return out
}

// BrokenRef is one link that points at nothing, and who wrote it. From is the
// artifact that wrote it, or the subject of the manifest that did.
type BrokenRef struct {
	From string
	Path string
	Link ResolvedLink
}

// unmarshalYAML is the one call into the decoder outside frontmatter.go. It
// exists so that the decoder's own types stay inside this package, which is the
// seam decision 0029 requires of it.
func unmarshalYAML(src []byte, into any) error {
	return yaml.Unmarshal(normalize(src), into)
}

func isYAML(name string) bool {
	ext := filepath.Ext(name)
	return ext == ".yaml" || ext == ".yml"
}
