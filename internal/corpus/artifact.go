package corpus

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/schema"
)

// Reserved frontmatter keys. Everything else at the top level is the author's,
// survives the read, and is available on Entry.Frontmatter.
const (
	keyID    = "id"
	keyType  = "type"
	keyTitle = "title"
	// keyFields holds the fields a schema declares. Frontmatter binds and the
	// body informs, and the fields that bind live under one key so that the
	// keys around them stay the artifact's own. See docs/decision/0010.
	keyFields = "fields"
	keyLinks  = "links"
)

// ReservedKey reports whether a top-level frontmatter key is the envelope's
// own. Everything else at that level is the author's, and nothing declares it.
func ReservedKey(name string) bool {
	switch name {
	case keyID, keyType, keyTitle, keyFields, keyLinks:
		return true
	}
	return false
}

// Well-known field names the core reads. Everything else under fields is a
// domain field: gatherers and predicates read it, the core does not.
const (
	fieldStatement = "statement"
	fieldGather    = "gather"
	fieldHoldWhen  = "hold_when"
	fieldTrigger   = "trigger"
	fieldRepair    = "repair"
)

// Ref is "type:slug" — how an artifact is named in a link, on the command line,
// and as the key of every durable stream. The slug alone is what the author
// writes in frontmatter; the type comes from where the file sits.
func Ref(kind, slug string) string { return kind + ":" + slug }

// SplitRef separates a "type:slug" reference. An unqualified reference has no
// type, and resolving it is the corpus's job because only the corpus knows
// which types answer to a slug.
func SplitRef(ref string) (kind, slug string) {
	kind, slug, found := strings.Cut(ref, ":")
	if !found {
		return "", ref
	}
	return kind, slug
}

// read turns one file into an artifact. It returns a nil entry when the file is
// not an artifact — no frontmatter, unreadable frontmatter, or no authored id —
// which is a diagnostic rather than a failure: the corpus still loads.
func read(path, kind string, src []byte, sc *schema.Schema) (*Entry, []Problem) {
	var problems []Problem
	add := func(k ProblemKind, format string, args ...any) {
		problems = append(problems, Problem{Kind: k, Path: path, Detail: fmt.Sprintf(format, args...)})
	}

	front, body, err := split(src)
	if err != nil {
		add(NoFrontmatter, "%s, so this is a file in a type directory and not an artifact", err)
		return nil, problems
	}
	fm, err := decode(front)
	if err != nil {
		add(Unreadable, "frontmatter is not YAML: %s", firstLine(err.Error()))
		return nil, problems
	}

	// Identity, and nothing about it comes from the filename. See 0021.
	slug, idProblems := readID(path, kind, fm)
	problems = append(problems, idProblems...)
	if slug == "" {
		return nil, problems
	}

	id := Ref(kind, slug)
	for i := range problems {
		problems[i].ID = id
	}
	add = func(k ProblemKind, format string, args ...any) {
		problems = append(problems, Problem{Kind: k, Path: path, ID: id, Detail: fmt.Sprintf(format, args...)})
	}

	if base := strings.TrimSuffix(filepath.Base(path), ".md"); base != slug {
		add(PathMismatch, "the file is named %q and the artifact is %q; the id is what counts, so this changes nothing", base, slug)
	}
	if declared, ok := fm[keyType].(string); ok && declared != "" && declared != kind {
		add(TypeMismatch, "frontmatter says type %q and the directory says %q; the directory is what makes it one", declared, kind)
	}

	fields := asStringMap(fm[keyFields])
	if _, present := fm[keyFields]; present && fields == nil {
		add(FieldViolation, "fields is not a map, so nothing under it could be read")
		fields = map[string]any{}
	}
	if fields == nil {
		fields = map[string]any{}
	}

	art := &model.Artifact{
		ID:     id,
		Kind:   model.Kind(kind),
		Title:  asString(fm[keyTitle]),
		Body:   body,
		Fields: fields,
	}
	project(art, fields)

	if sc == nil {
		// No schema for the kind. Nothing is checked, and that is reported once
		// per type directory by the walk rather than once per artifact.
		return &Entry{Artifact: art, Path: path, Frontmatter: fm}, problems
	}
	for _, v := range sc.Check(fields) {
		add(FieldViolation, "%s%s", v, misnested(v, fm))
	}
	return &Entry{Artifact: art, Path: path, Frontmatter: fm}, problems
}

// misnested names the likeliest cause of a required field being missing: the
// author wrote it at the top level, where nothing declares it, rather than
// under fields:.
//
// Both halves of that are already known here — the schema says the field is
// required and absent, and the frontmatter has a key of that exact name — and
// reporting them as two unrelated facts leaves an author looking at a value the
// tool can plainly see and will not mention. Nothing is corrected: kept does not
// rewrite human-authored files, so this says what it thinks happened and stops.
//
// A reserved key is never the cause. A schema may declare a field called
// "title" or "links" — that collision is exactly what the fields: block exists
// to prevent (docs/decision/0010) — and a top-level title: is the envelope's
// own and means what it says.
func misnested(v schema.Violation, fm map[string]any) string {
	if v.Kind != schema.MissingRequired || ReservedKey(v.Field) {
		return ""
	}
	if _, present := fm[v.Field]; !present {
		return ""
	}
	return fmt.Sprintf("; the frontmatter has a top-level %s that nothing declares — did you mean to nest it under fields:?", v.Field)
}

// readID reads the authored slug. A frontmatter id may be written bare, as
// decision 0021 writes it, or qualified as "type:slug", as the example in 0010
// writes it; both name the same artifact, and a qualifier that contradicts the
// directory is reported rather than obeyed.
func readID(path, kind string, fm map[string]any) (string, []Problem) {
	raw, present := fm[keyID]
	if !present {
		return "", []Problem{{
			Kind: NoID, Path: path,
			Detail: "no id in frontmatter, so nothing names this artifact; identity is authored and is never taken from the filename",
		}}
	}
	text := strings.TrimSpace(asString(raw))
	if text == "" {
		return "", []Problem{{
			Kind: NoID, Path: path,
			Detail: fmt.Sprintf("id is %v, which names nothing", raw),
		}}
	}

	var problems []Problem
	qualifier, slug := SplitRef(text)
	if qualifier != "" && qualifier != kind {
		problems = append(problems, Problem{
			Kind: TypeMismatch, Path: path,
			Detail: fmt.Sprintf("the id is qualified %q and the directory says %q; the directory is what makes it one", qualifier, kind),
		})
	}
	if bad := badSlug(slug); bad != "" {
		return "", append(problems, Problem{
			Kind: BadID, Path: path,
			Detail: fmt.Sprintf("%q %s, and the slug is a path component of every stream keyed to this artifact", slug, bad),
		})
	}
	return slug, problems
}

// badSlug says why a slug cannot be used, or "" if it can. The bar is that it
// must be a path component: the truth and disposition streams are files named
// after it, so a slug that is not one names a file nobody meant.
func badSlug(slug string) string {
	switch {
	case slug == "":
		return "is empty"
	case slug == "." || slug == "..":
		return "is a directory reference"
	case strings.ContainsAny(slug, `/\`):
		return "contains a path separator"
	}
	for _, r := range slug {
		if unicode.IsSpace(r) {
			return "contains whitespace"
		}
		if unicode.IsControl(r) {
			return "contains a control character"
		}
	}
	return ""
}

// project fills the fields the core itself reads. They are a view over Fields,
// not a second copy of the truth: the map is what the file said, and these are
// the entries the tick, the packet and the gate know the names of.
func project(art *model.Artifact, fields map[string]any) {
	art.Statement = asString(fields[fieldStatement])
	art.HoldWhen = asString(fields[fieldHoldWhen])
	art.Trigger = model.Trigger(asString(fields[fieldTrigger]))
	art.Gather = readGather(fields[fieldGather])
	art.RepairScope = readRepairScope(fields[fieldRepair])
}

// readGather reads the named map a predicate's namespace comes from. The
// gatherer defaults to the map key, so a promise reading one gatherer names it
// once; everything else in the entry is an argument.
func readGather(v any) map[string]model.GatherSpec {
	entries := asStringMap(v)
	if len(entries) == 0 {
		return nil
	}
	out := make(map[string]model.GatherSpec, len(entries))
	for name, raw := range entries {
		spec := model.GatherSpec{Gatherer: name}
		args := asStringMap(raw)
		if len(args) > 0 {
			spec.Args = make(map[string]any, len(args))
			for k, val := range args {
				if k == "gatherer" {
					if named := asString(val); named != "" {
						spec.Gatherer = named
						continue
					}
				}
				spec.Args[k] = val
			}
			if len(spec.Args) == 0 {
				spec.Args = nil
			}
		}
		out[name] = spec
	}
	return out
}

// readRepairScope reads what a repair may change. A scope is written as one
// glob or as a list of them, and both mean a list.
func readRepairScope(v any) []string {
	repair := asStringMap(v)
	if repair == nil {
		return nil
	}
	return asStringList(repair["scope"])
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func asStringList(v any) []string {
	switch t := v.(type) {
	case string:
		if t == "" {
			return nil
		}
		return []string{t}
	case []any:
		var out []string
		for _, item := range t {
			if s := asString(item); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// asStringMap accepts either shape a YAML mapping may arrive in. Nothing is
// dropped: a key that is not a string is rendered rather than discarded,
// because the caller is entitled to everything the file said.
func asStringMap(v any) map[string]any {
	switch t := v.(type) {
	case map[string]any:
		return t
	case map[any]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[fmt.Sprint(k)] = val
		}
		return out
	}
	return nil
}

func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}
