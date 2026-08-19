package corpus

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/predicate"
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
	fieldMaxAge    = "max_age"
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
func read(path, kind string, src []byte, sc *schema.Schema, gatherers Gatherers) (*Entry, []Problem) {
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

	checkPredicate(art, gatherers, add)
	if _, err := readAge(fields[fieldMaxAge]); err != nil {
		add(FieldViolation, "%s: %s", fieldMaxAge, err)
	}

	sections := Sections(body)
	if sc == nil {
		// No schema for the kind. Nothing is checked, and that is reported once
		// per type directory by the walk rather than once per artifact.
		return &Entry{Artifact: art, Path: path, Frontmatter: fm, Sections: sections}, problems
	}
	for _, v := range sc.Check(fields) {
		add(FieldViolation, "%s%s", v, misnested(v, fm))
	}
	checkSections(sc, sections, add)
	return &Entry{Artifact: art, Path: path, Frontmatter: fm, Sections: sections}, problems
}

// checkSections reports the two things a sectioned body can get wrong, and
// nothing else.
//
// An H2 the schema does not declare is not among them. It is the lenient case —
// the same answer Schema.Undeclared gives for a field nobody declared — so a
// kind's schema does not have to enumerate every heading anyone might write.
// What an undeclared section may not do is bind, and canonical refuses that
// rather than reporting it here.
func checkSections(sc *schema.Schema, sections []Section, add func(ProblemKind, string, ...any)) {
	if len(sc.Sections) == 0 {
		return
	}

	seen := map[string][]Section{}
	for _, s := range sections {
		seen[s.ID] = append(seen[s.ID], s)
	}
	for _, id := range sc.Sections {
		if len(seen[id]) == 0 {
			add(MissingSection, "the %s schema reads the body as sections and there is no heading for %q; the field is absent rather than empty",
				sc.Type, id)
		}
	}
	for _, id := range sc.Sections {
		if len(seen[id]) < 2 {
			continue
		}
		headings := make([]string, 0, len(seen[id]))
		for _, s := range seen[id] {
			headings = append(headings, fmt.Sprintf("%q on line %d", s.Heading, s.Line))
		}
		add(AmbiguousSection, "%s both name the section %q, and nothing picks one: %s cannot bind until one of them is renamed",
			strings.Join(headings, " and "), id, schema.HashBodyPrefix+id)
	}
}

// checkPredicate reads the artifact's gather map and its hold_when, and reports
// what can be decided without looking at the world: whether every gather entry
// names a gatherer that exists and can be run as written, whether the predicate
// parses, and whether every obs.<name>.<field> it reads is something the
// gatherer bound to that name says it returns.
//
// The last of those is what a catalogue adds, and it is the reason a gatherer
// declares its output at all: it turns most of the missing-field problem from a
// runtime three-valued question into an error a person can fix. It does not
// remove the runtime rule and is not allowed to — a gatherer can return less
// than it declared, so an absent field is still unknown however well the
// reference checked. Without a catalogue the name half still runs, because a
// promise reading obs.tests while declaring no gatherer called tests is reading
// something that will never arrive whatever gatherers turn out to declare. See
// docs/decision/0036.
//
// All of it is a diagnostic and not a load error, by the rule at the top of
// problem.go: the artifact still is what it is, and the map from id to
// artifact is not made wrong by a predicate nobody can evaluate. What it stops
// is the predicate being discovered in the code path that decides a verdict,
// which is where 0036 says it must not be.
//
// Nothing here evaluates anything, and nothing here yields unknown. Nothing has
// been measured at load, so there is nothing that could have failed to measure.
func checkPredicate(art *model.Artifact, gatherers Gatherers, add func(ProblemKind, string, ...any)) {
	sources := predicate.Sources(predicate.Gathered(art.Gather))
	if gatherers != nil {
		for _, bad := range gatherers.Check(art.Gather) {
			var missing interface{ NoSuchGatherer() bool }
			if errors.As(bad, &missing) && missing.NoSuchGatherer() {
				add(UnknownGatherer, "gather: %s", bad)
				continue
			}
			add(BadGatherSpec, "gather: %s", bad)
		}
		sources = gatherers.Sources(art.Gather)
	}

	if art.HoldWhen == "" {
		return
	}
	expr, err := predicate.Parse(art.HoldWhen, predicate.HoldWhen)
	if err != nil {
		add(BadPredicate, "hold_when: %s", err)
		return
	}
	for _, problem := range expr.Check(sources) {
		add(UngatheredObservation, "hold_when: %s", problem)
	}
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
	if bad := BadSlug(slug); bad != "" {
		return "", append(problems, Problem{
			Kind: BadID, Path: path,
			Detail: fmt.Sprintf("%q %s, and the slug is a path component of every stream keyed to this artifact", slug, bad),
		})
	}
	return slug, problems
}

// BadSlug says why a slug cannot be used, or "" if it can. The bar is that it
// must be a path component: the truth and disposition streams are files named
// after it, so a slug that is not one names a file nobody meant.
//
// It is exported because kept doctor mints a slug from a filename during
// adoption and has to refuse the ones this refuses, rather than writing an id
// the next read would reject.
func BadSlug(slug string) string {
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
	art.MaxAge, _ = readAge(fields[fieldMaxAge])
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

// readAge reads how old an answer may be before the artifact is due again.
//
// Go's own duration syntax, plus d and w, because docs/decision/0025 writes
// `max_age: 7d` and a week is the unit a review cadence is actually written in.
// An absent field is no maximum at all and is not an error; anything else that
// will not read as a duration is, because a max_age nobody could parse would
// silently mean "never stale".
func readAge(v any) (time.Duration, error) {
	if v == nil {
		return 0, nil
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return 0, fmt.Errorf("%q is not a duration, so nothing says how old an answer may be", v)
	}
	if n, unit, found := wholeUnits(s); found {
		return time.Duration(n) * unit, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("%q is not a duration: write it as 7d, 12h or 90m", s)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%q is not a duration an answer can age past", s)
	}
	return d, nil
}

// wholeUnits reads the two units Go's parser has no letter for.
func wholeUnits(s string) (int, time.Duration, bool) {
	var unit time.Duration
	switch s[len(s)-1] {
	case 'd':
		unit = 24 * time.Hour
	case 'w':
		unit = 7 * 24 * time.Hour
	default:
		return 0, 0, false
	}
	n, err := strconv.Atoi(s[:len(s)-1])
	if err != nil || n <= 0 {
		return 0, 0, false
	}
	return n, unit, true
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
