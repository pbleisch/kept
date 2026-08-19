package canonical

import (
	"strings"

	"golang.org/x/text/unicode/norm"
)

// Version is the canonicalization rules version. It is recorded on every
// ratification as norm, so refining these rules re-hashes old records under the
// version they were written with rather than de-ratifying the corpus at once.
// See docs/decision/0022.
//
// Nothing below changes without this number changing with it, and neither does
// schema.SectionID: a section identifier is produced from a heading and lands
// in the preimage, so that rule is under this version too. It lives there
// rather than here because canonical imports corpus and schema, and not the
// other way round. See docs/decision/0040.
const Version = 1

// bom is the byte-order mark a Windows editor may prepend. It is content to
// nobody and it changes a hash, so it goes before anything else looks at the
// text.
const bom = "\uFEFF"

// Normalize is the five rules of decision 0022, applied to one text value:
//
//  1. a leading byte-order mark is dropped
//  2. the text is normalized to Unicode NFC
//  3. CRLF and lone CR become LF
//  4. trailing spaces and tabs are stripped from each line
//  5. the result ends with exactly one newline
//
// Rule 2 is the one that is easy to forget and expensive to omit: macOS and
// Linux disagree about NFC and NFD, so a manuscript with accented character
// names round-tripped through both hashes differently and drops standing with
// no cause anyone can point at.
//
// Rule 4 strips spaces and tabs and nothing else. Whitespace is a wide category
// in Unicode and some of it — a non-breaking space, most obviously — is
// deliberate prose. Trimming those would declare two texts the same when their
// author meant them apart, which is the one direction this must not fail in.
//
// The rules are lossy for the hash and lossless for the file. Two bodies
// differing only in trailing whitespace are the same promise; two differing in
// a word are not. Nothing here writes anything back.
func Normalize(s string) string {
	s = strings.TrimPrefix(s, bom)
	s = norm.NFC.String(s)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	var b strings.Builder
	b.Grow(len(s) + 1)
	for i, line := range strings.Split(s, "\n") {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(strings.TrimRight(line, " \t"))
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}
