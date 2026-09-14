package main

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/schema"
)

// wantField is the one field kept want writes.
const wantField = "want"

// slugBound is how long a slug kept want mints may be, in bytes: the longest
// slug a person has written in this corpus, measured on 2026-09-13 as
// `0056-a-native-runtime-declares-nothing-and-names-its-own-scratch`. The want
// itself is written whole whatever the slug keeps. See docs/decision/0083.
const slugBound = 64

// wantCmd captures a want as a desire, and writes nothing but the file.
//
// It is the cheap end of the desire pipeline, and cheap is the requirement:
// docs/desire/ stayed empty while capturing a want meant writing frontmatter and
// minting a slug by hand. So the file holds an id and a want: line and nothing
// else — no title, no guidance comments, no body — because a person typing kept
// want is asking to have a want recorded, and everything a scaffold would add is
// something they did not ask for yet.
//
// It stays inside policy:nothing-kept-writes-rewrites-what-a-person-wrote the
// way kept new does: the file is created exclusively, an existing one is left
// exactly as it is, and nothing is opened on the ledger. A desire binds nothing,
// so there is no field here a ratification could be granted against.
func wantCmd(args []string, stdout, stderr io.Writer) int {
	fs, root := flags("want", stderr)
	rest, err := parse(fs, args)
	if err != nil {
		return exitUsage
	}
	if len(rest) != 1 {
		fmt.Fprint(stderr, "kept: want takes one want, quoted — kept want \"repair should pick which drift to spend a session on\"\n")
		return exitUsage
	}
	text := strings.TrimSpace(rest[0])
	if why := unwantable(text); why != "" {
		fmt.Fprintf(stderr, "kept: %s\n", why)
		return exitUsage
	}

	c, code := open(*root, stderr)
	if code != exitOK {
		return code
	}
	kind := string(model.KindDesire)
	sc, ok := c.Schemas.For(kind)
	if !ok {
		fmt.Fprintf(stderr, "kept: nothing declares the type %q. This corpus has %s\n",
			kind, strings.Join(c.Schemas.Types(), ", "))
		return exitUsage
	}
	if !sc.Declares(wantField) {
		fmt.Fprintf(stderr, "kept: the %s schema declares no field %q, which is the one field kept want writes\n", kind, wantField)
		return exitUsage
	}

	slug := wantSlug(text)
	if slug == "" {
		fmt.Fprintf(stderr, "kept: %q has no letter or digit to make a slug from. Name it:\n\n    kept new %s <slug> --set %s=%s\n",
			text, kind, wantField, yamlScalar(text))
		return exitUsage
	}

	// A collision is refused, never numbered. The slug is a function of the text
	// alone, so two wants that produce one slug are the same want or near enough
	// that the first should be read before the second is written — and the first
	// may be one somebody declined.
	id := corpus.Ref(kind, slug)
	if e, taken := c.Get(id); taken {
		fmt.Fprintf(stderr, "kept: %s is already claimed by %s, and nothing was written.\n\n    kept why %s\n\nsays what became of it.\n",
			id, e.Path, id)
		return exitUsage
	}

	path := filepath.Join(c.Root, kind, slug+".md")
	if err := write(path, renderWant(slug, text)); err != nil {
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return exitUsage
	}
	rel, err := filepath.Rel(c.Root, path)
	if err != nil {
		rel = path
	}
	fmt.Fprintf(stdout, "wrote %s\n\nNothing was recorded.\n", filepath.ToSlash(rel))
	return exitOK
}

// unwantable says why text cannot be written as a want, or nothing.
//
// A want is one line. A line break or a control character is refused rather
// than folded, because folding it rewrites what a person typed, and the detail
// that wanted a second line belongs in the note under the frontmatter.
func unwantable(text string) string {
	if text == "" {
		return "an empty want is not a want"
	}
	if !utf8.ValidString(text) {
		return "a want is text, and this is not valid UTF-8"
	}
	for _, r := range text {
		if unicode.IsControl(r) {
			return "a want is one line, with no line break or control character in it; the detail belongs in the note under the frontmatter"
		}
	}
	return ""
}

// wantSlug is the slug a want is written under, and it is a function of the
// text alone.
//
// It is [schema.SectionID] — NFC, lowercase, every run of what is not a letter
// or a digit to one hyphen — rather than a rule of its own, because that rule is
// already frozen under docs/decision/0040: the same words produce the same slug
// from every build, not only from this one. Nothing is transliterated and no
// word is dropped, because choosing which words a want is about is authoring.
//
// It is bounded at [slugBound] bytes, cut at the last hyphen that fits, and at
// the last whole character when a single word is longer than the bound. The cut
// is deterministic too, so two long wants that agree for sixty-four bytes
// produce one slug and the second is refused, naming the first.
func wantSlug(text string) string {
	slug := schema.SectionID(text)
	if len(slug) <= slugBound {
		return slug
	}
	cut := slug[:slugBound+1]
	if i := strings.LastIndexByte(cut, '-'); i > 0 {
		return slug[:i]
	}
	end := slugBound
	for end > 0 && !utf8.RuneStart(slug[end]) {
		end--
	}
	return slug[:end]
}

// renderWant is the file: an id, and a want under fields:, where the schema is
// consulted. Nothing else is written.
func renderWant(slug, text string) string {
	return fmt.Sprintf("---\nid: %s\nfields:\n  %s: %s\n---\n", yamlScalar(slug), wantField, yamlScalar(text))
}
