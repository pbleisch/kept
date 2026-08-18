package corpus

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/pbleisch/kept/internal/model"
)

// LinksDir is the directory under the corpus root holding manifests for things
// that cannot carry frontmatter of their own — an image, a PDF, anything
// binary. Like _schema it is underscore-prefixed, so the walk does not mistake
// it for a type directory.
const LinksDir = "_links"

// LinkSource is where a link was written. Three sources, and their order here
// is their priority: a target already linked from a higher one is not added
// again from a lower. See docs/decision/0002.
type LinkSource string

const (
	// FromFrontmatter is a links: entry. It is first because it is the only
	// source that states a relation, so it says the most.
	FromFrontmatter LinkSource = "frontmatter"
	// FromBody is an inline [[type:slug|Display]] in the prose. It carries no
	// relation, which is why it loses to a frontmatter entry naming the same
	// target rather than adding an untyped duplicate of it.
	FromBody LinkSource = "body"
	// FromManifest is a _links entry, for a subject that cannot carry its own.
	FromManifest LinkSource = "manifest"
)

// ResolvedLink is one edge, with where it was written and whether anything is
// at the other end.
type ResolvedLink struct {
	model.Link

	From LinkSource
	// Broken is set when no artifact claims Target. A broken link is a
	// diagnostic, never a load error: the corpus is exactly what it says it
	// is, and one of the things it says is wrong.
	Broken bool
}

// wikiLink matches an inline [[type:slug]] or [[type:slug|Display]]. The
// display half is for a reader and is not part of the edge.
var wikiLink = regexp.MustCompile(`\[\[([^\[\]|]+)(?:\|[^\[\]]*)?\]\]`)

// linksFromFrontmatter reads the links: list. Each entry states a target and
// may state a relation; an entry that states neither is reported rather than
// guessed at.
func linksFromFrontmatter(raw any) ([]model.Link, []string) {
	if raw == nil {
		return nil, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, []string{fmt.Sprintf("links is %T and a list was expected", raw)}
	}

	var out []model.Link
	var malformed []string
	for i, item := range items {
		switch t := item.(type) {
		case string:
			// A bare string is a target with no relation stated. It is the
			// shorthand a person reaches for, and refusing it would be
			// leniency stopping exactly where it is most useful.
			if target := strings.TrimSpace(t); target != "" {
				out = append(out, model.Link{Target: target})
				continue
			}
			malformed = append(malformed, fmt.Sprintf("link %d names nothing", i+1))
		default:
			entry := asStringMap(item)
			if entry == nil {
				malformed = append(malformed, fmt.Sprintf("link %d is %T and a target was expected", i+1, item))
				continue
			}
			target := strings.TrimSpace(asString(entry["target"]))
			if target == "" {
				malformed = append(malformed, fmt.Sprintf("link %d has no target", i+1))
				continue
			}
			out = append(out, model.Link{
				Target:   target,
				Relation: strings.TrimSpace(asString(entry["relation"])),
			})
		}
	}
	return out, malformed
}

// linksFromBody finds the inline wiki-links in prose, in the order they are
// written. Fenced blocks and inline code spans are skipped: a reference inside
// a code sample is a sample, and reporting it as a broken link is how a linter
// becomes something people stop reading.
func linksFromBody(body string) []model.Link {
	var out []model.Link
	seen := map[string]bool{}
	for _, m := range wikiLink.FindAllStringSubmatch(stripCode(body), -1) {
		target := strings.TrimSpace(m[1])
		if target == "" || seen[target] {
			continue
		}
		seen[target] = true
		out = append(out, model.Link{Target: target})
	}
	return out
}

// stripCode blanks out fenced blocks and inline code spans, keeping the line
// structure so nothing else shifts.
func stripCode(body string) string {
	var b strings.Builder
	fenced := ""
	for line := range strings.SplitSeq(body, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case fenced != "":
			if strings.HasPrefix(trimmed, fenced) {
				fenced = ""
			}
			b.WriteByte('\n')
			continue
		case strings.HasPrefix(trimmed, "```"):
			fenced = "```"
			b.WriteByte('\n')
			continue
		case strings.HasPrefix(trimmed, "~~~"):
			fenced = "~~~"
			b.WriteByte('\n')
			continue
		}
		b.WriteString(stripSpans(line))
		b.WriteByte('\n')
	}
	return b.String()
}

// stripSpans removes `code` runs from one line. A run opens on a backtick
// sequence and closes on the next of the same length, which is the rule
// markdown uses and is enough to keep a [[ref]] in backticks out of the graph.
func stripSpans(line string) string {
	var b strings.Builder
	for i := 0; i < len(line); {
		if line[i] != '`' {
			b.WriteByte(line[i])
			i++
			continue
		}
		open := i
		for i < len(line) && line[i] == '`' {
			i++
		}
		ticks := line[open:i]
		end := strings.Index(line[i:], ticks)
		if end < 0 {
			// Unterminated: the rest of the line is ordinary text.
			b.WriteString(line[open:])
			return b.String()
		}
		i += end + len(ticks)
	}
	return b.String()
}

// merge folds the three sources into one edge set, in priority order. Within a
// source an author may name one target twice with different relations and mean
// both; across sources a target already linked is not linked again, because the
// lower source says strictly less about it.
func merge(groups []sourced) []ResolvedLink {
	var out []ResolvedLink
	claimed := map[string]bool{}
	seen := map[string]bool{}

	for _, g := range groups {
		for _, l := range g.links {
			if claimed[l.Target] {
				continue
			}
			key := l.Target + "\x00" + l.Relation
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, ResolvedLink{Link: l, From: g.from})
		}
		for _, l := range g.links {
			claimed[l.Target] = true
		}
	}
	return out
}

type sourced struct {
	from  LinkSource
	links []model.Link
}
