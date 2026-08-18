package corpus

import (
	"bytes"
	"errors"
	"strings"

	"github.com/goccy/go-yaml"
)

// errNoFrontmatter is returned when a file has no frontmatter block at all.
var errNoFrontmatter = errors.New("no frontmatter block")

const fence = "---"

// normalize makes a file's bytes comparable regardless of how it was checked
// out: the byte-order mark a Windows editor may prepend is dropped and CRLF
// becomes LF. This is not a rewrite — nothing is written back — it is what
// makes the same content read the same on either platform, which is the
// property docs/decision/0022 later has to hold for the hash as well.
func normalize(src []byte) []byte {
	src = bytes.TrimPrefix(src, []byte{0xEF, 0xBB, 0xBF})
	src = bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n"))
	return bytes.ReplaceAll(src, []byte("\r"), []byte("\n"))
}

// split separates the frontmatter block from the markdown body. The block opens
// on the first line with a --- fence and closes on the next line that is --- or
// ... on its own.
func split(src []byte) (front []byte, body string, err error) {
	text := string(normalize(src))
	if !strings.HasPrefix(text, fence) {
		return nil, "", errNoFrontmatter
	}
	rest := text[len(fence):]
	// The opening fence must be the whole line.
	nl := strings.IndexByte(rest, '\n')
	if nl == -1 || strings.TrimSpace(rest[:nl]) != "" {
		return nil, "", errNoFrontmatter
	}
	rest = rest[nl+1:]

	var block []string
	for {
		line, remainder, more := strings.Cut(rest, "\n")
		trimmed := strings.TrimRight(line, " \t")
		if trimmed == fence || trimmed == "..." {
			return []byte(strings.Join(block, "\n")), trimBody(remainder), nil
		}
		if !more {
			// An unterminated block is a frontmatter block the author meant and
			// did not finish. Reporting it as absent would send them looking
			// for the wrong thing.
			return nil, "", errors.New("frontmatter block is never closed")
		}
		block = append(block, line)
		rest = remainder
	}
}

// trimBody drops the blank lines between the closing fence and the prose, and
// the trailing whitespace at the end of the file. Neither is content: what a
// reader means by the body starts at its first word.
func trimBody(s string) string {
	return strings.TrimRight(strings.TrimLeft(s, "\n"), " \t\n")
}

// decode reads a frontmatter block into the keys it declared. Every key
// survives, including ones no schema names, because the caller is entitled to
// all of them and kept never writes this file back. See docs/decision/0029.
func decode(front []byte) (map[string]any, error) {
	if len(bytes.TrimSpace(front)) == 0 {
		return map[string]any{}, nil
	}
	var out map[string]any
	if err := yaml.Unmarshal(front, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}
