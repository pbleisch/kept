package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/pbleisch/kept/internal/corpus"
)

func listCmd(args []string, stdout, stderr io.Writer) int {
	fs, root := flags("list", stderr)
	rest, err := parse(fs, args)
	if err != nil {
		return exitUsage
	}
	if len(rest) > 0 {
		fmt.Fprintf(stderr, "kept: list takes no arguments; did you mean show %s?\n", rest[0])
		return exitUsage
	}

	c, code := open(*root, stderr)
	if code != exitOK {
		return code
	}

	entries := c.All()
	rows := make([][]string, 0, len(entries))
	for _, e := range entries {
		kind, slug := corpus.SplitRef(e.Artifact.ID)
		rows = append(rows, []string{kind, slug, e.Artifact.Title})
	}
	writeTable(stdout, rows)

	if len(entries) == 0 {
		fmt.Fprintf(stdout, "no artifacts under %s\n", c.Root)
	}
	writeProblems(stdout, c, len(entries))
	return exitOK
}

// writeProblems prints everything wrong with the corpus. It prints them on
// success rather than as an error because reading a corpus back is the whole
// of what this does: a broken link is the finding, not a failure to report one.
func writeProblems(w io.Writer, c *corpus.Corpus, artifacts int) {
	if len(c.Problems) == 0 {
		if artifacts > 0 {
			fmt.Fprintf(w, "\n%s, nothing wrong\n", count(artifacts, "artifact"))
		}
		return
	}

	fmt.Fprintf(w, "\n%s, %s\n\n", count(artifacts, "artifact"), count(len(c.Problems), "problem"))
	for _, p := range c.Problems {
		where := p.Path
		if where == "" {
			where = p.ID
		}
		fmt.Fprintf(w, "  %s\n      %s: %s\n", where, p.Kind, p.Detail)
	}
}

func count(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// writeTable prints rows in aligned columns. The last column is never padded,
// so nothing trails a line with spaces.
func writeTable(w io.Writer, rows [][]string) {
	if len(rows) == 0 {
		return
	}
	widths := make([]int, len(rows[0]))
	for _, row := range rows {
		for i, cell := range row {
			if n := displayWidth(cell); n > widths[i] {
				widths[i] = n
			}
		}
	}
	var line strings.Builder
	for _, row := range rows {
		line.Reset()
		for i, cell := range row {
			if i == len(row)-1 {
				line.WriteString(cell)
				break
			}
			line.WriteString(cell)
			line.WriteString(pad(widths[i] - displayWidth(cell) + 2))
		}
		fmt.Fprintln(w, trimRight(line.String()))
	}
}

func pad(n int) string {
	out := make([]byte, max(n, 0))
	for i := range out {
		out[i] = ' '
	}
	return string(out)
}

func displayWidth(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

func trimRight(s string) string {
	for len(s) > 0 && s[len(s)-1] == ' ' {
		s = s[:len(s)-1]
	}
	return s
}
