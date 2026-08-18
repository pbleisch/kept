package main

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/schema"
)

// undeclared marks a value nothing in the schema names. It is printed rather
// than dropped, which is the whole of what leniency means here: the field
// survived the read and the reader is told nothing was checking it.
const undeclared = "— nothing declares this"

func showCmd(args []string, stdout, stderr io.Writer) int {
	fs, root := flags("show", stderr)
	rest, err := parse(fs, args)
	if err != nil {
		return exitUsage
	}
	if len(rest) != 1 {
		fmt.Fprint(stderr, "kept: show takes one artifact, named \"type:slug\" or by its slug alone\n")
		return exitUsage
	}

	c, code := open(*root, stderr)
	if code != exitOK {
		return code
	}

	entry, err := c.Lookup(rest[0])
	if err != nil {
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return exitUsage
	}
	sc, _ := c.Schemas.For(string(entry.Artifact.Kind))
	writeArtifact(stdout, c, entry, sc)
	return exitOK
}

func writeArtifact(w io.Writer, c *corpus.Corpus, e *corpus.Entry, sc *schema.Schema) {
	art := e.Artifact

	fmt.Fprintln(w, art.ID)
	if art.Title != "" {
		fmt.Fprintf(w, "  title  %s\n", art.Title)
	}
	// The path is shown as provenance, not identity. Renaming the file changes
	// this line and nothing else. See docs/decision/0021.
	fmt.Fprintf(w, "  read   %s\n", e.Path)

	writeFields(w, e, sc)
	writeExtraFrontmatter(w, e)
	writeLinks(w, e)
	writeBody(w, art.Body, bodyLabel(sc))

	if problems := problemsFor(c, art.ID); len(problems) > 0 {
		fmt.Fprintf(w, "\nproblems\n")
		for _, p := range problems {
			fmt.Fprintf(w, "  %s: %s\n", p.Kind, p.Detail)
		}
	}
}

func bodyLabel(sc *schema.Schema) string {
	if sc == nil || sc.Body == "" {
		return "body"
	}
	return sc.Body + " (the body)"
}

// writeFields prints every field the artifact carries: the ones its schema
// declares, in the order the schema declares them, then the ones nothing
// declares. Both are printed, because an unknown field that survived the read
// and is not shown has not really survived it.
func writeFields(w io.Writer, e *corpus.Entry, sc *schema.Schema) {
	fields := e.Artifact.Fields
	if len(fields) == 0 {
		return
	}

	var names []string
	seen := map[string]bool{}
	if sc != nil {
		for _, name := range sc.FieldOrder() {
			if _, present := fields[name]; present {
				names = append(names, name)
				seen[name] = true
			}
		}
	}
	extra := make([]string, 0, len(fields))
	for name := range fields {
		if !seen[name] {
			extra = append(extra, name)
		}
	}
	sort.Strings(extra)
	names = append(names, extra...)

	fmt.Fprintf(w, "\nfields\n")
	width := widest(names)
	for _, name := range names {
		note := ""
		if sc != nil && !sc.Declares(name) {
			note = undeclared
		}
		writeValue(w, name, width, fields[name], note)
	}
}

// writeExtraFrontmatter prints the top-level keys that are neither reserved nor
// under fields. They are the author's, they survived the read, and this is
// where they are available from.
func writeExtraFrontmatter(w io.Writer, e *corpus.Entry) {
	var names []string
	for name := range e.Frontmatter {
		if !corpus.ReservedKey(name) {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return
	}
	sort.Strings(names)

	fmt.Fprintf(w, "\nalso in frontmatter\n")
	width := widest(names)
	for _, name := range names {
		writeValue(w, name, width, e.Frontmatter[name], undeclared)
	}
}

func writeLinks(w io.Writer, e *corpus.Entry) {
	if len(e.Links) == 0 {
		return
	}
	fmt.Fprintf(w, "\nlinks\n")

	rows := make([][]string, 0, len(e.Links))
	for _, l := range e.Links {
		relation := l.Relation
		if relation == "" {
			relation = "-"
		}
		note := string(l.From)
		if l.Broken {
			note += "   BROKEN"
		}
		rows = append(rows, []string{"  " + relation, l.Target, string(l.TargetKind), note})
	}
	writeTable(w, rows)
}

func writeBody(w io.Writer, body, label string) {
	if body == "" {
		return
	}
	fmt.Fprintf(w, "\n%s\n", label)
	for line := range strings.SplitSeq(body, "\n") {
		if line == "" {
			fmt.Fprintln(w)
			continue
		}
		fmt.Fprintf(w, "  %s\n", line)
	}
}

func problemsFor(c *corpus.Corpus, id string) []corpus.Problem {
	var out []corpus.Problem
	for _, p := range c.Problems {
		if p.ID == id {
			out = append(out, p)
		}
	}
	return out
}

func writeValue(w io.Writer, name string, width int, value any, note string) {
	rendered := render(value)
	indent := "  " + name + pad(width-displayWidth(name)+2)
	continuation := pad(displayWidth(indent))

	lines := strings.Split(rendered, "\n")
	for i, line := range lines {
		prefix := continuation
		if i == 0 {
			prefix = indent
		}
		suffix := ""
		if i == len(lines)-1 && note != "" {
			suffix = "   " + note
		}
		fmt.Fprintln(w, trimRight(prefix+line+suffix))
	}
}

func widest(names []string) int {
	n := 0
	for _, name := range names {
		if d := displayWidth(name); d > n {
			n = d
		}
	}
	return n
}

// render turns a value from frontmatter into something a person can read,
// deterministically. Map keys are sorted, so one corpus prints the same way
// twice; nothing here is a serialization and nothing is written back.
func render(v any) string {
	switch t := v.(type) {
	case nil:
		return "~"
	case string:
		return t
	case []any:
		parts := make([]string, len(t))
		for i, item := range t {
			parts[i] = renderInline(item)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case map[string]any:
		return renderMap(t)
	case map[any]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[fmt.Sprint(k)] = val
		}
		return renderMap(out)
	default:
		return fmt.Sprint(t)
	}
}

func renderMap(m map[string]any) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + ": " + renderInline(m[k])
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// renderInline is render, with newlines folded so a nested value stays on one
// line. A multi-line scalar at the top level keeps its lines; one buried in a
// map does not, because the shape is what is being shown there.
func renderInline(v any) string {
	return strings.ReplaceAll(render(v), "\n", " ")
}
