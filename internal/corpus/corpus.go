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

	// Sections are the body's H2 sections, in the order they were written,
	// whether or not the schema declares them. An undeclared one is kept for
	// the same reason an undeclared field is: it survives the read and the
	// caller is entitled to it. What it may not do is bind. See
	// docs/decision/0040.
	Sections []Section
}

// Section returns one body section's prose.
//
// found is false when no heading produced this identifier, which is different
// from a section that is present and says nothing: 0031 spends its
// present/absent marks on exactly that distinction, and collapsing the two here
// would make deleting a section and emptying one hash identically.
//
// ambiguous is true when more than one heading produced it. Nothing may bind on
// an ambiguous section: picking the first would make document order part of the
// binding hash, so that moving a section changes what a ratification was
// granted against with no diff showing it.
func (e *Entry) Section(id string) (text string, found, ambiguous bool) {
	for _, s := range e.Sections {
		if s.ID != id {
			continue
		}
		if found {
			return "", true, true
		}
		text, found = s.Text, true
	}
	return text, found, false
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
	// Workspace is the root a file reference is relative to: the directory
	// holding .kept, which is the repository the corpus describes. For a
	// manuscript it is the same directory as Root, because there the corpus is
	// the content. See docs/decision/0030.
	Workspace string
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
	// types is what this corpus calls its kinds, from the schemas it declares
	// and the type directories it has. It is the vocabulary that settles
	// whether "spec:rfc-1234" is an artifact or a URI.
	types map[string]bool
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
func Load(root, workspace string, opt Options) (*Corpus, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("corpus root %s: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("corpus root %s is not a directory", root)
	}
	if workspace == "" {
		workspace = root
	}

	schemas, err := schema.Load(filepath.Join(root, schema.Dir))
	if err != nil {
		return nil, err
	}

	c := &Corpus{
		Root:      root,
		Workspace: workspace,
		Schemas:   schemas,
		byID:      map[string]*Entry{},
		bySlug:    map[string][]*Entry{},
		types:     map[string]bool{},
	}
	for _, t := range schemas.Types() {
		c.types[t] = true
	}

	claims := map[string][]string{}
	kinds, err := typeDirs(root)
	if err != nil {
		return nil, err
	}
	for _, kind := range kinds {
		c.types[kind] = true
	}
	for _, kind := range kinds {
		sc, ok := schemas.For(kind)
		if !ok {
			c.Problems = append(c.Problems, Problem{
				Kind: NoSchema, Path: kind,
				Detail: fmt.Sprintf("no %s/%s.yaml, so nothing checks what a %s must say", schema.Dir, kind, kind),
			})
		}
		if err := c.walkKind(root, kind, sc, opt.Gatherers, claims); err != nil {
			return nil, err
		}
	}

	if dup := duplicates(claims); dup != nil {
		return nil, dup
	}

	c.checkOrdinals()

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

func (c *Corpus) walkKind(root, kind string, sc *schema.Schema, gatherers Gatherers, claims map[string][]string) error {
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

		entry, problems := read(rel, kind, src, sc, gatherers)
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

// checkOrdinals reports two artifacts of a numbered kind claiming one number.
//
// It is a diagnostic and not the load error a duplicate id is, and the
// difference is problem.go's own line: the slugs differ, so the map from id to
// artifact is still right and every stream keyed to one is still that
// artifact's. What is wrong is that a number in an id is how a person cites a
// decision in prose, and two artifacts numbered alike make every such citation
// ambiguous to a reader even though no code is confused by it.
//
// Nothing kept does creates one: whatever mints an id takes the highest number
// in the kind and adds one. Two branches cut from the same commit both find the
// same highest number, which is why this is reported at all — the second one to
// land is the one that has to be renumbered, and it is cheapest to know that
// before it is ratified. See docs/decision/0069.
func (c *Corpus) checkOrdinals() {
	claimed := map[string]map[int][]string{}
	for _, e := range c.entries {
		kind := string(e.Artifact.Kind)
		sc, ok := c.Schemas.For(kind)
		if !ok || !sc.ID.Numbered() {
			continue
		}
		_, slug := SplitRef(e.Artifact.ID)
		n, after, well := sc.ID.Split(slug)
		if !well || after == "" {
			// Already reported as a shape problem, and a number nobody could
			// read cannot contest one.
			continue
		}
		if claimed[kind] == nil {
			claimed[kind] = map[int][]string{}
		}
		claimed[kind][n] = append(claimed[kind][n], e.Artifact.ID)
	}

	for _, kind := range sortedStrings(claimed) {
		sc, _ := c.Schemas.For(kind)
		for _, n := range sortedInts(claimed[kind]) {
			ids := claimed[kind][n]
			if len(ids) < 2 {
				continue
			}
			sort.Strings(ids)
			c.Problems = append(c.Problems, Problem{
				Kind: DuplicateNumber, Path: kind,
				Detail: fmt.Sprintf("%d artifacts are numbered %s: %s. A number is part of the id every citation writes, so renumber all but one — while it is unratified, which costs nothing",
					len(ids), sc.ID.Number(n), strings.Join(ids, ", ")),
			})
		}
	}
}

func sortedStrings[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedInts[V any](m map[int]V) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
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
// the corpus what is at the other end of each.
func (c *Corpus) resolveLinks() {
	bySubject := map[string][]model.Link{}
	for i := range c.Manifests {
		m := &c.Manifests[i]
		m.Reference = c.reference(m.Subject)
		if p, bad := c.checkSubject(m); bad {
			c.Problems = append(c.Problems, p)
			continue
		}
		if m.Reference.Kind == model.RefArtifact {
			bySubject[m.Reference.Raw] = append(bySubject[m.Reference.Raw], m.Links...)
			continue
		}
		// The subject is a file or a URI, so no artifact owns these edges — and
		// a link with nothing at the other end is broken wherever it was
		// written, which is why they are still checked.
		c.resolveOrphan(m)
	}

	for _, e := range c.entries {
		id := e.Artifact.ID
		front, malformed := linksFromFrontmatter(e.Frontmatter[keyLinks])
		for _, msg := range malformed {
			c.Problems = append(c.Problems, Problem{Kind: MalformedLink, Path: e.Path, ID: id, Detail: msg})
		}

		e.Links = merge([]sourced{
			{FromFrontmatter, front},
			{FromBody, linksFromBody(e.Artifact.Body)},
			{FromManifest, bySubject[id]},
		})

		sc, _ := c.Schemas.For(string(e.Artifact.Kind))
		e.Artifact.Links = make([]model.Link, 0, len(e.Links))
		for i := range e.Links {
			link := &e.Links[i]
			ref := c.reference(link.Target)
			link.TargetKind = ref.Kind

			if sc != nil && !sc.AllowsRelation(link.Relation) {
				c.Problems = append(c.Problems, Problem{
					Kind: UndeclaredRelation, Path: e.Path, ID: id,
					Detail: fmt.Sprintf("relation %q on the link to %s is not one of %v", link.Relation, link.Target, sc.AllowedRelations),
				})
			}
			if kind, why := c.check(ref); why != "" {
				link.Broken = true
				c.Problems = append(c.Problems, Problem{
					Kind: kind, Path: e.Path, ID: id,
					Detail: fmt.Sprintf("the %s link to %s %s", link.From, ref.Raw, why),
				})
			}
			e.Artifact.Links = append(e.Artifact.Links, link.Link)
		}
		if len(e.Artifact.Links) == 0 {
			e.Artifact.Links = nil
		}
	}
}

// resolveOrphan checks the links of a manifest subject that is not an artifact.
// They belong to a file or to something outside the workspace, so they are
// recorded against the manifest rather than any artifact, and they are checked
// exactly as an artifact's are.
func (c *Corpus) resolveOrphan(m *Manifest) {
	for _, l := range m.Links {
		ref := c.reference(l.Target)
		l.TargetKind = ref.Kind
		link := ResolvedLink{Link: l, From: FromManifest}
		if kind, why := c.check(ref); why != "" {
			link.Broken = true
			c.Problems = append(c.Problems, Problem{
				Kind: kind, Path: m.Path,
				Detail: fmt.Sprintf("the link from %s to %s %s", m.Subject, ref.Raw, why),
			})
		}
		c.orphanLinks = append(c.orphanLinks, BrokenRef{From: m.Subject, Path: m.Path, Link: link})
	}
}

// checkSubject asks the same question of a manifest's subject that check asks
// of a target, because a subject is a reference like any other.
func (c *Corpus) checkSubject(m *Manifest) (Problem, bool) {
	kind, why := c.check(m.Reference)
	if why == "" {
		return Problem{}, false
	}
	if kind == BrokenLink {
		kind = ManifestSubject
	}
	return Problem{
		Kind: kind, Path: m.Path,
		Detail: fmt.Sprintf("the subject %s %s", m.Reference.Raw, why),
	}, true
}

// check says what is wrong at the far end of a reference, or "" if nothing is.
// What it asks depends on what the reference names, and the three questions are
// deliberately not the same question:
//
//   - An artifact is claimed by the corpus, or it is not. Nothing outside the
//     corpus is consulted.
//   - A file is in the workspace, or it is not. That is a stat in the same tree
//     the walk already read, on no deadline and with no way to answer "nobody
//     knows".
//   - A URI is well formed, or it is not — and that is all. Whether anything
//     answers it is a question for the world, which means a gatherer, a
//     deadline (0026) and an observation naming the world it measured (0014).
//     Asking it here would make a linter into a truth source, and one that
//     reports a promise broken because a laptop was on a train. See
//     docs/decision/0030.
func (c *Corpus) check(r Reference) (ProblemKind, string) {
	switch r.Kind {
	case model.RefArtifact:
		if _, ok := c.byID[r.Raw]; ok {
			return "", ""
		}
		return BrokenLink, "names no artifact"

	case model.RefFile:
		if r.Escapes {
			return OutsideWorkspace, "resolves outside the workspace root"
		}
		if _, err := os.Stat(filepath.Join(c.Workspace, filepath.FromSlash(r.Path))); err != nil {
			return BrokenLink, "names no file in the workspace"
		}
		return "", ""

	default:
		if !wellFormed(r.Raw) {
			return MalformedURI, "is not a URI anything could follow"
		}
		return "", ""
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

// Lookup resolves an artifact a person typed. It takes a "type:slug", or a bare
// slug when only one type answers to it — an ambiguous one is an error naming
// the candidates rather than a guess.
//
// A link target is never resolved this way. A person at a terminal is present
// to read an error, so convenience costs nothing there; a link is durable and
// tracked, and a reference whose meaning depends on what else happens to exist
// is one nobody can review. See docs/decision/0030.
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
