package corpus

import (
	"strings"

	"github.com/pbleisch/kept/internal/schema"
)

// A sectioned body is a markdown block read as several named prose fields, one
// per H2, for a kind whose schema declares them. Everything in this file is
// frozen: a section's identifier lands in a binding preimage, so changing any
// of it re-hashes every artifact of every sectioned kind at once. It is part of
// what canonical.Version names, and it does not change without that number
// changing with it. See docs/decision/0040.

// Section is one H2 and the prose beneath it.
type Section struct {
	// ID is the identifier [schema.SectionID] produced from the heading. It is the
	// name a schema declares and a hash list writes as "body.<id>".
	ID string
	// Heading is the heading's text as authored, kept for a diagnostic that
	// has to name what it found rather than what it computed.
	Heading string
	// Text is the prose from the end of the heading line to the next H2 or the
	// end of the body, with leading and trailing blank lines removed.
	Text string
	// Line is where the heading was, 1-based within the body, as an editor
	// counts.
	Line int
}

// Sections splits a body into its H2 sections, in the order they were written.
//
// Text before the first H2 belongs to no section and is not returned. It is not
// lost: the reserved name body still means the whole markdown block, headings
// and preamble included.
func Sections(body string) []Section {
	lines := strings.Split(body, "\n")

	var out []Section
	var fenced bool
	var fence string
	start := -1 // index into lines of the current section's first prose line

	closeSection := func(end int) {
		if start >= 0 {
			out[len(out)-1].Text = trimBlankLines(lines[start:end])
		}
	}

	for i, line := range lines {
		if marker, ok := codeFence(line); ok {
			switch {
			case !fenced:
				fenced, fence = true, marker
			case strings.HasPrefix(marker, fence):
				// A closing fence is at least as long as the one that opened
				// it and of the same character, which is what keeps a ``` run
				// inside a ~~~ block from ending it.
				fenced, fence = false, ""
			}
			continue
		}
		if fenced {
			continue
		}
		heading, ok := headingText(line)
		if !ok {
			continue
		}
		id := schema.SectionID(heading)
		if id == "" {
			// A heading that is all punctuation names nothing. It stays in the
			// body and opens no section, because a section with no identifier
			// is one nothing could ever declare or bind.
			continue
		}
		closeSection(i)
		out = append(out, Section{ID: id, Heading: heading, Line: i + 1})
		start = i + 1
	}
	closeSection(len(lines))
	return out
}

// headingText returns an ATX H2's text, or false if the line is not one.
//
// Column zero and a space after the marker, both deliberately strict. An
// indented ## is prose in a list at one to three spaces and a code block at
// four, and a setext H2's marker is ---, which is also the frontmatter fence
// and a horizontal rule. Neither ambiguity belongs inside a binding hash.
func headingText(line string) (string, bool) {
	if !strings.HasPrefix(line, "##") {
		return "", false
	}
	rest := line[2:]
	if rest != "" && !strings.HasPrefix(rest, " ") {
		// ### is an H3 and ##Context is not a heading at all.
		return "", false
	}
	// ATX's closing form: a trailing run of # preceded by whitespace.
	text := strings.TrimRight(rest, " \t")
	if trimmed := strings.TrimRight(text, "#"); trimmed != text {
		if end := strings.TrimRight(trimmed, " \t"); end != trimmed || trimmed == "" {
			text = end
		}
	}
	return strings.TrimSpace(text), true
}

// codeFence reports whether a line opens or closes a fenced code block, and
// with which marker. Fences are skipped so that a ## inside an example is an
// example rather than a section — which is the difference between a document
// that says what a heading looks like and one that has that heading.
func codeFence(line string) (string, bool) {
	trimmed := strings.TrimLeft(line, " ")
	for _, c := range []string{"```", "~~~"} {
		if strings.HasPrefix(trimmed, c) {
			run := 0
			for run < len(trimmed) && trimmed[run] == c[0] {
				run++
			}
			return trimmed[:run], true
		}
	}
	return "", false
}

// trimBlankLines drops the blank lines at both ends of a section's prose. A
// blank line after a heading is not something an author meant, and a corpus
// must not de-ratify because somebody added one — 0022's argument, applied to
// the one boundary 0022 does not reach.
func trimBlankLines(lines []string) string {
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}
