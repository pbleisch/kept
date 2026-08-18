package schema

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
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
	// HashBodyPrefix names one H2 section — "body.<section>". How a schema
	// declares its sections is open question 5, so nothing reads one yet.
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
	// field. A body split into named H2 sections is open question 5 and is not
	// read here.
	Body string

	// AllowedRelations bounds what a link of this kind may mean. Empty leaves
	// it unbounded.
	AllowedRelations []string

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
	Body        string        `yaml:"body"`
	Links       *fileLinks    `yaml:"links"`
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
		Body:        f.Body,
		Fields:      map[string]Field{},
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
