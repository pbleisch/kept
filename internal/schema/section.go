package schema

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// A section identifier is what a schema's body list declares and what a hash
// list writes as "body.<id>". The rule that produces one from a heading is
// frozen: the name lands in a binding preimage, so changing any of it re-hashes
// every artifact of every sectioned kind at once. It is part of what
// canonical.Version names, and it does not change without that number changing
// with it. See docs/decision/0040.

// SectionID turns a heading into the name a schema declares and a hash list
// writes. It is frozen; see the note at the top of this file and 0040.
//
//	## Context                    → context
//	## What this does not decide  → what-this-does-not-decide
//	## The `run:` line            → the-run-line
//	## Kael's eyes                → kael-s-eyes
//
// Nothing markdown means is interpreted, so a code span's backticks are
// punctuation like any other character, and nothing is transliterated. The
// alternative is a list of characters treated specially, and such a list grows
// — each time re-hashing every sectioned artifact there is.
func SectionID(heading string) string {
	s := strings.ToLower(norm.NFC.String(heading))

	var b strings.Builder
	b.Grow(len(s))
	dash := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
			continue
		}
		dash = true
	}
	return b.String()
}

// IsSectionID reports whether a name is already what SectionID would produce.
// A schema declares identifiers rather than headings, and one that needs
// normalizing before it can be compared would make two spellings of one binding
// field — which a hash list may not have.
func IsSectionID(name string) bool {
	return name != "" && SectionID(name) == name
}
