package schema

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
)

// Dir is the directory under the corpus root that holds the schemas. It is
// underscore-prefixed because it is not a type directory, so the corpus walk
// never mistakes it for one.
const Dir = "_schema"

// Reserved names a hash list may use in place of a field name. They are
// recorded here because the ledger depends on them; nothing in this package
// resolves them yet.
//
// See docs/decision/0003.
const (
	// HashBody names the whole markdown block.
	HashBody = "body"
	// HashBodyPrefix names one H2 section — "body.<section>". A schema declares
	// its sections in its body list, and only a declared one may bind. See
	// docs/decision/0040.
	HashBodyPrefix = "body."
	// HashLinksPrefix names the links of one relation — "links.<relation>".
	HashLinksPrefix = "links."
)

// Field types a schema may declare. An artifact's value is checked against the
// declared type and reported when it disagrees; it is never coerced and never
// dropped.
const (
	TypeString    = "string"
	TypeMap       = "map"
	TypeList      = "list"
	TypeEnum      = "enum"
	TypeReference = "reference"
)

// Schema is one _schema/{type}.yaml: what artifacts of a kind may say, which of
// their fields bind, and what their links may mean.
//
// A schema is not lenient about itself. An unreadable one is an error rather
// than a diagnostic, because validating every artifact of a kind against a
// schema nobody could parse would be a check that silently never fires.
type Schema struct {
	Type        string
	Label       string
	Description string
	Icon        string

	// Hash names the fields that standing and truth records are keyed to. It
	// may name a field, or one of the reserved names above. Changing the list
	// re-hashes every artifact of the kind at once and drops their standing,
	// which is the intended cost of changing what counts as the same promise.
	// See docs/decision/0003.
	Hash []string

	// Fields are the fields the kind declares. An artifact may carry others:
	// leniency is the rule, and an undeclared field is reported and kept, never
	// rejected and never silently dropped. See docs/decision/0002.
	Fields map[string]Field

	// Body names the prose field the markdown block becomes — "why" for a
	// promise, "statement_detail" for canon. Empty means the kind has no prose
	// field, and it is empty for a sectioned kind, where Sections is what the
	// block becomes instead.
	Body string

	// Sections are the H2 sections this kind's body is read as, in the order
	// the schema wrote them. They are identifiers rather than headings — a
	// heading maps to one by corpus.SectionID — and a hash list names one as
	// "body.<id>". Empty is the ordinary case: an unsectioned body is the
	// kind's one prose field. See docs/decision/0010 and 0040.
	Sections []string

	// AllowedRelations bounds what a link of this kind may mean. Empty leaves
	// it unbounded.
	AllowedRelations []string

	// ID is the shape this kind's identities take. Its zero value is a plain
	// slug, which is every kind here but one. See [IDShape].
	ID IDShape

	// order is the order the fields were authored in, so a reader sees them the
	// way the schema presents them rather than alphabetically.
	order []string
}

// Field is one declared field.
type Field struct {
	Name        string
	Type        string
	Required    bool
	Description string

	// Values are an enum's permitted values.
	Values []string
	// Default is what the schema says the field means when it is absent. It is
	// recorded rather than substituted: an absent field stays absent, because
	// filling one in would put a value in the artifact that its author did not
	// write.
	Default any
}

// FieldOrder returns the declared field names in the order the schema authored
// them.
func (s *Schema) FieldOrder() []string {
	out := make([]string, len(s.order))
	copy(out, s.order)
	return out
}

// Declares reports whether the schema names this field. A field it does not
// name is not an error — it is the lenient case, and the caller is expected to
// keep the value and say that nothing declared it.
func (s *Schema) Declares(name string) bool {
	_, ok := s.Fields[name]
	return ok
}

// AllowsRelation reports whether a link may carry this relation. A schema with
// no allowed_relations allows any, and an unstated relation — which is what an
// inline body link has — is always allowed.
func (s *Schema) AllowsRelation(relation string) bool {
	if len(s.AllowedRelations) == 0 || relation == "" {
		return true
	}
	return slices.Contains(s.AllowedRelations, relation)
}

// DeclaresSection reports whether the schema names this body section. One it
// does not name is not an error — it is the lenient case, the same as an
// undeclared field — but it may not bind, because the schema is what says
// which prose is part of the commitment. See docs/decision/0040.
func (s *Schema) DeclaresSection(id string) bool {
	return slices.Contains(s.Sections, id)
}

// Binds reports whether a change to this field changes the artifact's binding
// hash. Nothing hashes yet; this is the question the ledger will ask.
func (s *Schema) Binds(field string) bool {
	return slices.Contains(s.Hash, field)
}

// Set is the schemas of one corpus, by type.
type Set struct {
	byType map[string]*Schema
}

// Load reads every _schema/*.yaml under dir. A corpus with no schema directory
// loads as an empty set rather than an error: a kind with no schema is validated
// against nothing, which is the lenient end of the same rule that lets an
// artifact carry a field nobody declared.
func Load(dir string) (*Set, error) {
	set := &Set{byType: map[string]*Schema{}}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return set, nil
		}
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}

	for _, e := range entries {
		if e.IsDir() || !isYAML(e.Name()) {
			continue
		}
		path := filepath.Join(dir, e.Name())
		src, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		s, err := parse(src)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if s.Type == "" {
			// The filename is the fallback, not the identity — the same rule
			// artifacts follow, applied to the one file that names a kind.
			s.Type = strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		}
		if prior, ok := set.byType[s.Type]; ok {
			return nil, fmt.Errorf("%s: two schemas declare type %q", path, prior.Type)
		}
		set.byType[s.Type] = s
	}
	return set, nil
}

func isYAML(name string) bool {
	ext := filepath.Ext(name)
	return ext == ".yaml" || ext == ".yml"
}

// For returns the schema for a type.
func (s *Set) For(kind string) (*Schema, bool) {
	sc, ok := s.byType[kind]
	return sc, ok
}

// Types returns every declared type, sorted.
func (s *Set) Types() []string {
	out := make([]string, 0, len(s.byType))
	for t := range s.byType {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// file is the on-disk shape of a schema. It exists so that no type from the
// YAML decoder appears in this package's own signatures — the seam decision
// 0029 requires.
type file struct {
	Type        string        `yaml:"type"`
	Label       string        `yaml:"label"`
	Description string        `yaml:"description"`
	Icon        string        `yaml:"icon"`
	Hash        []string      `yaml:"hash"`
	Fields      yaml.MapSlice `yaml:"fields"`
	Body        any           `yaml:"body"`
	Links       *fileLinks    `yaml:"links"`
	ID          *fileID       `yaml:"id"`
}

type fileID struct {
	Shape string `yaml:"shape"`
	Width int    `yaml:"width"`
}

type fileLinks struct {
	AllowedRelations []string `yaml:"allowed_relations"`
}

type fileField struct {
	Type        string   `yaml:"type"`
	Required    bool     `yaml:"required"`
	Description string   `yaml:"description"`
	Values      []string `yaml:"values"`
	Default     any      `yaml:"default"`
}

func parse(src []byte) (*Schema, error) {
	var f file
	if err := yaml.Unmarshal(src, &f); err != nil {
		return nil, err
	}

	s := &Schema{
		Type:        f.Type,
		Label:       f.Label,
		Description: f.Description,
		Icon:        f.Icon,
		Hash:        f.Hash,
		Fields:      map[string]Field{},
	}
	if err := s.readBody(f.Body); err != nil {
		return nil, err
	}
	if err := s.readID(f.ID); err != nil {
		return nil, err
	}
	if f.Links != nil {
		s.AllowedRelations = f.Links.AllowedRelations
	}

	// A MapSlice preserves the authored order, which is the order a reader
	// should see the fields in.
	for _, item := range f.Fields {
		name, ok := item.Key.(string)
		if !ok {
			return nil, fmt.Errorf("field name %v is not a string", item.Key)
		}
		var ff fileField
		if item.Value != nil {
			raw, err := yaml.Marshal(item.Value)
			if err != nil {
				return nil, fmt.Errorf("field %q: %w", name, err)
			}
			if err := yaml.Unmarshal(raw, &ff); err != nil {
				return nil, fmt.Errorf("field %q: %w", name, err)
			}
		}
		s.Fields[name] = Field{
			Name:        name,
			Type:        ff.Type,
			Required:    ff.Required,
			Description: ff.Description,
			Values:      ff.Values,
			Default:     ff.Default,
		}
		s.order = append(s.order, name)
	}
	return s, nil
}

// readBody reads what the markdown block becomes. A string names one prose
// field, which is every kind whose body is one thing; a list names the H2
// sections the block is read as. The two forms are the two cases
// docs/decision/0010 describes, and which one a kind is is visible in one line.
//
// Leniency stops here. A declared name that is not already a section
// identifier is an error rather than a diagnostic, because a name needing
// normalization before it could be compared would make "body.Decision" and
// "body.decision" two spellings of one binding field — and a hash list may
// have exactly one. See docs/decision/0040.
func (s *Schema) readBody(v any) error {
	switch t := v.(type) {
	case nil:
		return nil
	case string:
		s.Body = t
		return nil
	case []any:
		for _, item := range t {
			name, ok := item.(string)
			if !ok {
				return fmt.Errorf("body: section name %v is not a string", item)
			}
			if !IsSectionID(name) {
				return fmt.Errorf(
					"body: %q is not a section identifier, and a schema declares identifiers rather than headings: write %q",
					name, SectionID(name))
			}
			if slices.Contains(s.Sections, name) {
				return fmt.Errorf("body: section %q is declared twice", name)
			}
			s.Sections = append(s.Sections, name)
		}
		return nil
	default:
		return fmt.Errorf("body: %T is neither a prose field name nor a list of sections", v)
	}
}

// readID reads what a kind's identities look like. Absent is a plain slug,
// which is the ordinary case and the one that needs no declaration.
//
// Leniency stops here, as it does for body: a shape nothing recognizes is a
// schema error rather than a diagnostic, because a kind validated against a
// shape nobody could read is a check that silently never fires — and the
// generator that mints an id reads the same declaration, so a spelling the
// reader ignored would mint ids nothing checks.
func (s *Schema) readID(f *fileID) error {
	if f == nil {
		return nil
	}
	switch f.Shape {
	case "", ShapeSlug:
		if f.Width != 0 {
			return fmt.Errorf("id: width is the number of digits a numbered id opens with, and %s has none", ShapeSlug)
		}
		return nil
	case ShapeNumbered:
		if f.Width < 1 {
			return fmt.Errorf("id: %s declares how many digits its ordinal has, and width is %d", ShapeNumbered, f.Width)
		}
		s.ID = IDShape{Width: f.Width}
		return nil
	default:
		return fmt.Errorf("id: shape %q is neither %s nor %s", f.Shape, ShapeSlug, ShapeNumbered)
	}
}

// The shapes an id may take. The set is closed rather than a pattern, and the
// reason is that one declaration has to serve two readers: the corpus, which
// checks an id, and whatever mints one. A regular expression answers the first
// and cannot answer the second — nothing derives the next value from
// `^[0-9]{4}-`— so a corpus with a pattern would need a second declaration
// saying how to produce a matching id, which is the rule living in two places
// that can disagree. A closed set is answerable both ways, and a kind that
// needs a third shape adds a value here rather than a language.
const (
	// ShapeSlug is a plain slug, and it is what a kind that declares nothing
	// gets.
	ShapeSlug = "slug"
	// ShapeNumbered is a zero-padded ordinal, a hyphen, then the slug —
	// "0068-an-anchor-is-a-base-and-a-change". Every decision in this
	// repository is one and nothing else here is.
	ShapeNumbered = "numbered"
)

// IDShape is what a kind's ids look like beyond being usable as a path
// component, which every id must be whatever its kind declares.
//
// It belongs to the schema for the reason the field list does: it is per-kind
// knowledge, and a generator or a check that carried its own copy would be a
// second copy of a rule that can disagree with the schema. The number is part
// of the identity every link target uses rather than a filename habit, so an
// unnumbered decision is not a cosmetic difference — it is an id of a shape no
// citation in the corpus has.
type IDShape struct {
	// Width is how many digits the ordinal has. Zero is a plain slug.
	Width int
}

// Numbered reports whether ids of this kind open with an ordinal.
func (s IDShape) Numbered() bool { return s.Width > 0 }

// Split separates an id into the ordinal it opens with and the slug after it.
// ok is false when the id is not of this shape, and it is always true for a
// shape that declares no ordinal.
func (s IDShape) Split(id string) (n int, slug string, ok bool) {
	if !s.Numbered() {
		return 0, id, true
	}
	if len(id) < s.Width+2 || id[s.Width] != '-' {
		return 0, "", false
	}
	digits := id[:s.Width]
	for _, r := range digits {
		if r < '0' || r > '9' {
			return 0, "", false
		}
	}
	n, err := strconv.Atoi(digits)
	if err != nil {
		return 0, "", false
	}
	return n, id[s.Width+1:], true
}

// Format writes an id of this shape.
func (s IDShape) Format(n int, slug string) string {
	if !s.Numbered() {
		return slug
	}
	return fmt.Sprintf("%0*d-%s", s.Width, n, slug)
}

// Number is an ordinal written to the declared width, without the slug after
// it — how a person names a decision when they say "0069".
func (s IDShape) Number(n int) string {
	if !s.Numbered() {
		return ""
	}
	return fmt.Sprintf("%0*d", s.Width, n)
}

// Fits reports whether an ordinal can be written in the declared width. A
// counter that has run past its digits is a shape nothing can mint into.
func (s IDShape) Fits(n int) bool {
	if !s.Numbered() {
		return true
	}
	return n >= 0 && len(strconv.Itoa(n)) <= s.Width
}

// String is the shape as a person reads one — "NNNN-slug", or "slug".
func (s IDShape) String() string {
	if !s.Numbered() {
		return ShapeSlug
	}
	return strings.Repeat("N", s.Width) + "-" + ShapeSlug
}
