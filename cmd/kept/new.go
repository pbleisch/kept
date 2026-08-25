package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/schema"
)

// newCmd writes the frontmatter a new artifact of a kind must have, generated
// from that kind's own schema.
//
// It exists because the shape was only ever available by copying a neighbour.
// Five consecutive decisions — 0062 through 0066, authored in one day — put
// `statement:` at the top level where nothing reads it, so each hashed as though
// it had no statement while its schema's hash list said otherwise. Commit
// 3dfc1e1 made that loud, by reporting it; this makes the correct shape the one
// that costs nothing to produce. See docs/decision/0069.
//
// **It reads the schema and never a template.** A template per kind would be a
// second copy of the seven files under docs/_schema, and a second copy of a
// schema that can disagree with it is the same defect this verb exists to
// prevent, one level up. Everything below is driven by [schema.Schema]: which
// fields exist, which are required, which bind, what the body is read as. A
// field added to a schema appears here with no code change.
//
// **It creates nothing but the artifact.** No ledger is opened, no standing is
// minted, no truth is recorded — a new artifact is unratified, which is what a
// new artifact is. `kept ratify` is the only thing that grants standing, and a
// person is the only thing that runs it.
func newCmd(args []string, stdout, stderr io.Writer) int {
	fs, root := flags("new", stderr)
	title := fs.String("title", "", "the artifact's title; taken from the slug when absent")
	toStdout := fs.Bool("stdout", false, "print the artifact instead of writing it, and create nothing")
	var set fieldValues
	fs.Var(&set, "set", "a declared field's value, as name=value; repeatable")

	rest, err := parse(fs, args)
	if err != nil {
		return exitUsage
	}
	if len(rest) != 2 {
		fmt.Fprint(stderr, "kept: new takes a type and a slug — kept new decision 0069-what-was-decided\n")
		return exitUsage
	}
	kind, slug := rest[0], rest[1]

	c, code := open(*root, stderr)
	if code != exitOK {
		return code
	}

	sc, ok := c.Schemas.For(kind)
	if !ok {
		fmt.Fprintf(stderr, "kept: nothing declares the type %q. This corpus has %s\n",
			kind, strings.Join(c.Schemas.Types(), ", "))
		return exitUsage
	}
	if bad := corpus.BadSlug(slug); bad != "" {
		fmt.Fprintf(stderr, "kept: %q %s, and the slug is a path component of every stream keyed to this artifact\n", slug, bad)
		return exitUsage
	}
	// An id names one artifact, and two claiming one id is the single thing
	// that stops a corpus loading. Refusing here is cheaper than writing the
	// file and discovering it on the next read.
	if e, taken := c.Get(corpus.Ref(kind, slug)); taken {
		fmt.Fprintf(stderr, "kept: %s is already claimed by %s\n", corpus.Ref(kind, slug), e.Path)
		return exitUsage
	}

	s := &scaffold{kind: kind, slug: slug, schema: sc, title: *title, values: set.byName}
	if s.title == "" {
		s.title = titleFromSlug(slug)
	}
	if refused := s.refusedValues(); len(refused) > 0 {
		for _, why := range refused {
			fmt.Fprintf(stderr, "kept: --set %s\n", why)
		}
		return exitUsage
	}
	if why := s.check(); len(why) > 0 {
		writeUnfilled(stderr, kind, why)
		return exitUsage
	}

	if *toStdout {
		fmt.Fprint(stdout, s.render())
		return exitOK
	}

	// The filename is not the identity — the id in frontmatter is, and renaming
	// the file changes nothing (0021). It is still the slug, because a filename
	// that is not the slug is a path-mismatch problem every read reports, and a
	// verb that generates a finding is not a scaffold.
	path := filepath.Join(c.Root, kind, slug+".md")
	if err := write(path, s.render()); err != nil {
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return exitUsage
	}

	rel, err := filepath.Rel(c.Root, path)
	if err != nil {
		rel = path
	}
	fmt.Fprintf(stdout, "wrote %s\n\nIt is unratified, which is what a new artifact is: nothing was recorded and\nnothing has standing. Fill it in, then\n\n    kept ratify %s\n",
		filepath.ToSlash(rel), corpus.Ref(kind, slug))
	return exitOK
}

// write refuses to overwrite. Nothing kept writes rewrites what a person wrote
// — the policy is ratified and this is the verb it most obviously applies to —
// so the file is created exclusively and an existing one is left exactly as it
// is, whatever it holds. See policy:nothing-kept-writes-rewrites-what-a-person-wrote.
func write(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%s is already there, and nothing kept writes rewrites what a person wrote", path)
		}
		return err
	}
	if _, err := io.WriteString(f, content); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// fieldValues collects repeated --set name=value flags.
type fieldValues struct{ byName map[string]string }

func (v *fieldValues) String() string { return "" }

func (v *fieldValues) Set(raw string) error {
	name, value, found := strings.Cut(raw, "=")
	if !found {
		return fmt.Errorf("%q is not name=value", raw)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("%q names no field", raw)
	}
	if v.byName == nil {
		v.byName = map[string]string{}
	}
	if _, twice := v.byName[name]; twice {
		return fmt.Errorf("%s is set twice", name)
	}
	v.byName[name] = value
	return nil
}

// scaffold is one artifact about to be written: what kind it is, what names it,
// and the values a person supplied for the fields kept will not invent.
type scaffold struct {
	kind   string
	slug   string
	title  string
	schema *schema.Schema
	values map[string]string
}

// unfilled is a field a person has to supply before this artifact can be
// written, and why kept will not supply it.
type unfilled struct {
	field schema.Field
	why   string
}

// check reports the fields that must arrive on the command line.
//
// Two rules produce the list, and only the second is subtle.
//
// **kept never writes a placeholder into a field that binds.** A bindable field
// is one the kind's hash list names, so a ratification is granted against its
// value — and an artifact scaffolded with `statement: TODO` can be ratified
// exactly as it stands, which grants standing to the word TODO. The hash list is
// the discriminator and kept already has it, so a bindable field is either the
// value its author wrote or nothing at all. A guidance comment carries the same
// help and hashes nothing.
//
// **kept never fills an enum.** Every value an enum admits is an assertion about
// the artifact, and the one value that would not be — a placeholder — is outside
// the enum and would be reported by the next read. A schema's default is
// recorded rather than substituted for the same reason a read does not
// substitute one: filling it in puts a value in the artifact that its author did
// not write. See internal/schema.Field.Default.
//
// Everything else kept can fill without asserting anything: a required string
// nothing binds gets a placeholder that is unmistakably one, and a required map
// or list gets an empty one.
func (s *scaffold) check() []unfilled {
	var out []unfilled
	for _, name := range s.schema.FieldOrder() {
		f := s.schema.Fields[name]
		if _, supplied := s.values[name]; supplied || !f.Required {
			continue
		}
		switch {
		case s.schema.Binds(name):
			out = append(out, unfilled{f, "it binds"})
		case f.Type == schema.TypeEnum:
			out = append(out, unfilled{f, "it is an enum"})
		}
	}
	return out
}

// writeUnfilled says what is missing, what the schema says about it, and the
// flag that supplies it. The rule is stated once and each field carries only
// which half of it applies, because two fields refused for one reason should
// not read as two reasons.
func writeUnfilled(w io.Writer, kind string, missing []unfilled) {
	fmt.Fprintf(w, "kept: a %s cannot be scaffolded without %s.\n\n", kind, count(len(missing), "field"))
	fmt.Fprint(w, "kept invents no value for a field that binds — a ratification is granted\n"+
		"against it, so an artifact scaffolded with a placeholder in one can be\n"+
		"ratified as it stands — and none for an enum, where every value it admits\n"+
		"is an assertion and a schema's default is recorded rather than written in.\n\n")
	for _, m := range missing {
		fmt.Fprintf(w, "  %s — %s\n", m.field.Name, m.why)
		for _, line := range wrap(m.field.Description, 66) {
			fmt.Fprintf(w, "      %s\n", line)
		}
		if len(m.field.Values) > 0 {
			fmt.Fprintf(w, "      one of: %s\n", strings.Join(m.field.Values, ", "))
		}
		fmt.Fprintf(w, "      --set %s=...\n", m.field.Name)
	}
}

// render is the artifact's bytes.
func (s *scaffold) render() string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "id: %s\n", yamlScalar(s.slug))
	fmt.Fprintf(&b, "type: %s\n", yamlScalar(s.kind))
	fmt.Fprintf(&b, "title: %s\n", yamlScalar(s.title))
	s.renderFields(&b)
	s.renderLinks(&b)
	b.WriteString("---\n")
	s.renderBody(&b)
	return b.String()
}

// renderFields writes the fields: block. Every declared field appears, and
// where it appears is the whole point: under fields:, which is where the schema
// is consulted, rather than beside id: and title:, which is the level 0010
// reserves for the author and where a declared name is read by nobody.
//
// A field kept has a value for is written. Everything else is written as a
// comment: the shape, so an author can see what the kind admits, with nothing
// asserted and nothing hashed. When that is every field — a decision declares
// one optional field and binds it — the fields: key is commented out with them,
// because a key with nothing under it is not an empty map and every read
// reports it as one that could not be read.
func (s *scaffold) renderFields(b *strings.Builder) {
	order := s.schema.FieldOrder()
	if len(order) == 0 {
		return
	}

	var block strings.Builder
	live := false
	for i, name := range order {
		f := s.schema.Fields[name]
		if i > 0 {
			block.WriteString("\n")
		}
		s.renderGuidance(&block, f)
		value, written := s.value(f)
		if !written {
			fmt.Fprintf(&block, "  # %s:\n", name)
			continue
		}
		live = true
		fmt.Fprintf(&block, "  %s: %s\n", name, value)
	}

	if live {
		b.WriteString("fields:\n")
	} else {
		b.WriteString("# fields:\n")
	}
	b.WriteString(block.String())
}

// renderGuidance writes what the schema says about a field, as YAML comments. A
// comment is where guidance belongs: it is read by the person filling the file
// in, it survives in the file, and it is not content — nothing parses it into a
// field and no hash covers it.
func (s *scaffold) renderGuidance(b *strings.Builder, f schema.Field) {
	var about []string
	if f.Type != "" {
		about = append(about, f.Type)
	}
	if f.Required {
		about = append(about, "required")
	}
	if s.schema.Binds(f.Name) {
		about = append(about, "binds")
	}
	fmt.Fprintf(b, "  # %s", f.Name)
	if len(about) > 0 {
		fmt.Fprintf(b, " — %s", strings.Join(about, ", "))
	}
	b.WriteString("\n")

	if len(f.Values) > 0 {
		fmt.Fprintf(b, "  #   one of: %s\n", strings.Join(f.Values, ", "))
	}
	if f.Default != nil {
		fmt.Fprintf(b, "  #   absent means: %v\n", f.Default)
	}
	for _, line := range wrap(f.Description, 68) {
		fmt.Fprintf(b, "  #   %s\n", line)
	}
}

// value returns what to write for a field, and whether to write it at all.
//
// An optional field is never written: absent is what the author has said so
// far, and a key with nothing after it is a value — an empty string that a
// required field reads as unanswered and that max_age reads as a duration
// nobody could parse.
func (s *scaffold) value(f schema.Field) (text string, live bool) {
	if supplied, ok := s.values[f.Name]; ok {
		return yamlScalar(supplied), true
	}
	if !f.Required {
		return "", false
	}
	// check has already refused the required fields kept will not fill, so what
	// is left is one it can fill without asserting anything.
	switch f.Type {
	case schema.TypeMap:
		return "{}", true
	case schema.TypeList:
		return "[]", true
	default:
		return unwritten, true
	}
}

// unwritten is what a required field kept had to write and could not fill holds.
// It is deliberately not prose: it must not read as an answer to anyone
// skimming, and it may only ever appear in a field nothing binds — see
// scaffold.check.
const unwritten = "TODO"

// renderLinks writes the links block as a comment, carrying the relations the
// schema admits. A link is a claim about another artifact and kept has none to
// make; an empty links: key would be a shape nothing reads.
func (s *scaffold) renderLinks(b *strings.Builder) {
	relations := s.schema.AllowedRelations
	if len(relations) == 0 {
		return
	}
	fmt.Fprintf(b, "\n# links — one of: %s\n", strings.Join(relations, ", "))
	fmt.Fprintf(b, "# links:\n#   - { target: type:slug, relation: %s }\n", relations[0])
}

// renderBody writes the markdown block.
//
// Three cases, and the schema says which. A kind whose **whole body binds** — a
// canon entry, where the prose is the commitment — gets no body at all, because
// every byte kept wrote there would be inside the hash a ratification is granted
// against, the title included. A kind whose body is read as **sections** gets
// one heading per declared section and nothing under them: a declared section
// that is absent is reported by every read, and a bindable one must be empty for
// the same reason the whole body must. Everything else gets the H1 this corpus
// starts every artifact with.
func (s *scaffold) renderBody(b *strings.Builder) {
	if s.schema.Binds(schema.HashBody) {
		return
	}
	fmt.Fprintf(b, "\n# %s\n", s.title)
	for _, id := range s.schema.Sections {
		fmt.Fprintf(b, "\n## %s\n", heading(id))
	}
}

// heading is the heading a declared section identifier is written as. The
// identifier is what binds, so the heading has to produce it again: one that
// does not is written as the identifier itself rather than as something a read
// would report missing.
func heading(id string) string {
	text := capitalized(strings.ReplaceAll(id, "-", " "))
	if schema.SectionID(text) != id {
		return id
	}
	return text
}

// titleFromSlug is the title an artifact gets when its author did not give one.
// It is the slug read aloud, which is what most titles in this corpus are, and
// it is safe to derive because no schema's hash list names title — it is the
// envelope's, and 0010 reserves that level for the author.
func titleFromSlug(slug string) string {
	return capitalized(strings.ReplaceAll(slug, "-", " "))
}

func capitalized(s string) string {
	for i, r := range s {
		if unicode.IsLetter(r) {
			return s[:i] + string(unicode.ToUpper(r)) + s[i+len(string(r)):]
		}
		if !unicode.IsSpace(r) && !unicode.IsDigit(r) {
			break
		}
	}
	return s
}

// wrap breaks prose into lines that fit a comment. Nothing is truncated: a
// description is the schema author's own words and cutting one would lose the
// half that says what to do.
func wrap(text string, width int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}
	var out []string
	line := words[0]
	for _, w := range words[1:] {
		if len(line)+1+len(w) > width {
			out = append(out, line)
			line = w
			continue
		}
		line += " " + w
	}
	return append(out, line)
}

// refusedValues says why a --set cannot be honoured, once per value.
//
// A name nothing declares is refused rather than written, because a value under
// a name the schema has no opinion about is the shape this verb exists to stop
// producing — one level down from the top-level statement, and just as unread.
// A reserved name is refused too: the envelope's keys are not fields, and the
// two that a person might try have flags of their own.
//
// A map or a list is refused because --set carries a scalar and nothing here
// parses YAML out of an argv element. What that costs is a kind whose hash list
// names a required map: kept cannot scaffold one cleanly, and says so rather
// than writing a string where a map belongs.
func (s *scaffold) refusedValues() []string {
	var out []string
	for _, name := range sortedNames(s.values) {
		f, declared := s.schema.Fields[name]
		switch {
		case corpus.ReservedKey(name):
			out = append(out, fmt.Sprintf("%s: %s is the envelope's own key rather than a field; id and title are the positional slug and --title", name, name))
		case !declared:
			out = append(out, fmt.Sprintf("%s: the %s schema declares no field %q. It has %s", name, s.kind, name, strings.Join(s.schema.FieldOrder(), ", ")))
		case f.Type == schema.TypeMap || f.Type == schema.TypeList:
			out = append(out, fmt.Sprintf("%s: %s is declared %s, and --set carries one scalar. Write it in the file", name, name, f.Type))
		case f.Type == schema.TypeEnum && len(f.Values) > 0 && !slices.Contains(f.Values, s.values[name]):
			out = append(out, fmt.Sprintf("%s: %q is not one of %s", name, s.values[name], strings.Join(f.Values, ", ")))
		case strings.TrimSpace(s.values[name]) == "":
			out = append(out, fmt.Sprintf("%s: an empty value is not an answer, and a field nobody answered is absent rather than empty", name))
		}
	}
	return out
}
