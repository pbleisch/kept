package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/lock"
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

	if numbered, why := carriesANumber(sc, slug); numbered {
		fmt.Fprintf(stderr, "kept: %s\n", why)
		return exitUsage
	}

	// Everything that does not depend on the number is decided before the lock
	// is taken, so the section around minting is a corpus walk and a write.
	s := &scaffold{kind: kind, schema: sc, title: *title, values: set.byName}
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
		// Nothing is created, so there is nothing to serialize: the number
		// printed is what this corpus would mint now, and it is advisory.
		id, why := mint(c, kind, sc.ID, slug)
		if why != "" {
			fmt.Fprintf(stderr, "kept: %s\n", why)
			return exitUsage
		}
		s.name(sc.ID, id)
		fmt.Fprint(stdout, s.render())
		return exitOK
	}

	rel, code := writeUnderLock(c, kind, sc, slug, s, stderr)
	if code != exitOK {
		return code
	}
	fmt.Fprintf(stdout, "wrote %s\n\nIt is unratified, which is what a new artifact is: nothing was recorded and\nnothing has standing. Fill it in, then\n\n    kept ratify %s\n",
		rel, corpus.Ref(kind, s.slug))
	return exitOK
}

// writeUnderLock reads the highest number a kind has and writes the file that
// takes the next one, with nothing else able to do the same in between.
//
// **This is the rung that prevents rather than reports.** The number is minted
// one past the highest there is, so two runs that read the corpus at the same
// moment mint the same number and produce two artifacts a person then has to
// renumber. `internal/lock` is under the shared Git common directory, so one
// file excludes every worktree of the repository, and the read and the write are
// on the same side of it.
//
// The wait is short and impatient. What is held is one corpus walk and one
// `O_EXCL` create — measured in milliseconds over this corpus — so a holder that
// is still there after a couple of seconds has wedged rather than queued, and a
// scaffold is a verb a person is standing in front of. Losing it costs the word
// *again* and nothing else: no number was taken and no file was written.
//
// A workspace that is not a Git repository has no common directory to put a lock
// in, and no worktree pool either. It mints without one, which is the same
// exclusion a single checkout already has.
func writeUnderLock(c *corpus.Corpus, kind string, sc *schema.Schema, slug string, s *scaffold, stderr io.Writer) (string, int) {
	if _, err := lock.Path(c.Workspace, lock.Mint); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), mintDeadline)
		defer cancel()
		held, err := lock.Acquire(ctx, c.Workspace, lock.Mint, mintWait)
		if err != nil {
			if errors.Is(err, lock.ErrRefused) {
				fmt.Fprint(stderr, "kept: another run is minting a number. Nothing was written; run it again\n")
				return "", exitUsage
			}
			fmt.Fprintf(stderr, "kept: %s\n", err)
			return "", exitUsage
		}
		defer held.Release()

		// Read the corpus again inside the section. The one read outside it
		// answered what the kind is; this one answers what number is free, and
		// that is the answer another run can have changed.
		fresh, code := open(c.Root, stderr)
		if code != exitOK {
			return "", code
		}
		c = fresh
	}

	id, why := mint(c, kind, sc.ID, slug)
	if why != "" {
		fmt.Fprintf(stderr, "kept: %s\n", why)
		return "", exitUsage
	}
	// An id names one artifact, and two claiming one id is the single thing
	// that stops a corpus loading. Refusing here is cheaper than writing the
	// file and discovering it on the next read.
	if e, taken := c.Get(corpus.Ref(kind, id)); taken {
		fmt.Fprintf(stderr, "kept: %s is already claimed by %s\n", corpus.Ref(kind, id), e.Path)
		return "", exitUsage
	}
	s.name(sc.ID, id)

	// The filename is not the identity — the id in frontmatter is, and renaming
	// the file changes nothing (0021). It is still the slug, because a filename
	// that is not the slug is a path-mismatch problem every read reports, and a
	// verb that generates a finding is not a scaffold.
	path := filepath.Join(c.Root, kind, id+".md")
	if err := write(path, s.render()); err != nil {
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return "", exitUsage
	}
	rel, err := filepath.Rel(c.Root, path)
	if err != nil {
		rel = path
	}
	return filepath.ToSlash(rel), exitOK
}

// How long a mint may wait for another one, and how long resolving the lock's
// own directory may take. See writeUnderLock for the first.
const (
	mintWait     = 2 * time.Second
	mintDeadline = 30 * time.Second
)

// name settles what the artifact is called, once its number is known.
func (s *scaffold) name(shape schema.IDShape, id string) {
	s.slug = id
	if s.title == "" {
		s.title = titleFromSlug(shape, id)
	}
}

// carriesANumber refuses a slug that already has one. Only the number is minted,
// and two sources for one number is the rule living in two places — renumbering
// is an edit to the file, with a re-ratification behind it where the artifact
// has standing.
func carriesANumber(sc *schema.Schema, slug string) (bool, string) {
	if !sc.ID.Numbered() {
		return false, ""
	}
	_, after, ok := sc.ID.Split(slug)
	if !ok || after == "" {
		return false, ""
	}
	return true, fmt.Sprintf("a %s id is %s and the number is minted, so give the slug alone:\n\n    kept new %s %s",
		sc.Type, sc.ID, sc.Type, after)
}

// mint completes the identity a person supplied, and for every kind that
// declares no shape that is the slug they typed and nothing else.
//
// A **numbered** kind is the one case where kept produces part of an id: the
// number is in the identity rather than in the filename — `decision:0068-...`
// is what every citation of it writes — so a `kept new decision` that skipped it
// would author the only unnumbered decision in a corpus of sixty-eight, which is
// drift from convention introduced by the tool built to stop it. 0021 already
// puts this here: the slug is *minted by the CLI when the artifact is created,
// which is also where uniqueness is enforced*.
//
// The number is the highest one the kind already has, plus one. It is read out
// of the corpus rather than counted, so a gap stays a gap. Two runs of this at
// one moment would read one highest number and mint it twice, which is what
// [writeUnderLock] exists to prevent; two *branches* cut from one commit still
// find the same free number, which nothing local can prevent and which the
// corpus reports. It is not an id minted from a counter in the sense 0012
// refuses: an ordinal is authored identity a person cites in prose, not a name
// concurrent runs allocate.
//
// **Only the number is minted**, and [carriesANumber] refuses a slug that
// already has one.
func mint(c *corpus.Corpus, kind string, shape schema.IDShape, slug string) (id, why string) {
	if !shape.Numbered() {
		return slug, ""
	}
	next := nextNumber(c, kind, shape)
	if !shape.Fits(next) {
		return "", fmt.Sprintf("the next %s number is %d and the schema declares %s, which has no room for it",
			kind, next, shape)
	}
	return shape.Format(next, slug), ""
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
// It is the id read aloud, which is what most titles in this corpus are, and it
// is safe to derive because no schema's hash list names title — it is the
// envelope's, and 0010 reserves that level for the author.
//
// A numbered kind keeps its number in front, separated from the rest, because
// the ordinal is a name rather than a word: every decision here is titled
// "0068 — An anchor is a base and a change". That comes from the shape rather
// than from knowing what a decision is.
func titleFromSlug(shape schema.IDShape, id string) string {
	if n, after, ok := shape.Split(id); ok && shape.Numbered() && after != "" {
		return shape.Number(n) + " — " + capitalized(strings.ReplaceAll(after, "-", " "))
	}
	return capitalized(strings.ReplaceAll(id, "-", " "))
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

// nextNumber is the number a new artifact of a numbered kind takes: the highest
// the kind already has, plus one.
//
// It is read out of the corpus rather than counted, so a gap left by a
// superseded number stays a gap and nothing after it shifts. Two runs against
// one corpus produce the same answer and two branches cut from one commit
// produce the same answer as each other — which is why the corpus reports a
// contested number rather than pretending minting prevents one.
//
// One function, because `kept new` and `kept doctor --fix` both need it and two
// rules for one number is how they come to disagree.
func nextNumber(c *corpus.Corpus, kind string, shape schema.IDShape) int {
	next := 1
	for _, e := range c.All() {
		if string(e.Artifact.Kind) != kind {
			continue
		}
		_, slug := corpus.SplitRef(e.Artifact.ID)
		n, after, ok := shape.Split(slug)
		if ok && after != "" && n >= next {
			next = n + 1
		}
	}
	return next
}
