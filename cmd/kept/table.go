package main

import (
	"fmt"
	"io"
	"strings"
)

// A table is rows of cells whose columns are sized from the data in them.
//
// Every column but the last is padded to the widest value that column actually
// holds, and the last is never padded at all. Nothing here guesses a width, and
// that is the whole of the fix: the three tables this replaced padded an
// artifact id to 38 characters, which is shorter than most of the ids in this
// repository's own corpus, so the column after it started one place further
// along on exactly the rows a reader was looking at. A column is as wide as its
// contents or it is not a column.
//
// It measures and never truncates, which is what `git branch -v` and
// `git for-each-ref --format='%(align:…)'` both do — measured, both let a name
// wider than the field push the rest of the line right rather than cutting it,
// and `%(align:30)` turned out to be a minimum rather than a maximum. kept
// could not truncate honestly anyway: it does not know how wide the terminal is
// (see internal/progress, which assumes 80 for the same reason), and an
// abbreviated artifact id is one a reader cannot paste into `kept why`.
//
// What follows from never truncating is the column order every caller here
// uses: the fixed-width answer on the left, the unbounded name on the right.
// That is `ls -l` and `git status -s`, and it is the order in which a line too
// long for the screen spends its overflow on the part the reader has already
// identified rather than on the part they are scanning for.
type table struct {
	// indent is what every row begins with.
	indent string
	rows   []tableRow
}

// tableRow is one line, plus what could not be said on it. The hanging lines
// belong to the row rather than to the caller's loop so that a table and its
// details cannot be written in two places and drift apart.
type tableRow struct {
	cells  []string
	detail []string
}

// gap is the space between two columns. Two, because one reads as a single
// wide word and three is a gutter a reader's eye has to jump.
const gap = 2

// hang is how far a detail line is indented past its row.
const hang = 4

func (t *table) add(cells ...string) {
	t.rows = append(t.rows, tableRow{cells: cells})
}

// beneath hangs lines under the row just added: the reason a verdict is what it
// is, or what the tick did about it. Empty lines are dropped rather than
// printed blank, so a caller may pass whatever it has.
func (t *table) beneath(lines ...string) {
	if len(t.rows) == 0 {
		return
	}
	r := &t.rows[len(t.rows)-1]
	for _, l := range lines {
		if l != "" {
			r.detail = append(r.detail, l)
		}
	}
}

func (t *table) empty() bool { return len(t.rows) == 0 }

// widths is each column's width, measured over every row that reaches it. A
// column no row fills is zero wide and is skipped entirely, so a table whose
// optional column is empty throughout does not print a band of spaces where it
// would have been.
func (t *table) widths() []int {
	var widths []int
	for _, r := range t.rows {
		for i, cell := range r.cells {
			for len(widths) <= i {
				widths = append(widths, 0)
			}
			if n := displayWidth(cell); n > widths[i] {
				widths[i] = n
			}
		}
	}
	return widths
}

func (t *table) write(w io.Writer) {
	widths := t.widths()
	detailIndent := t.indent + pad(hang)

	var line strings.Builder
	for _, r := range t.rows {
		line.Reset()
		line.WriteString(t.indent)
		for i, cell := range r.cells {
			if widths[i] == 0 {
				continue
			}
			line.WriteString(cell)
			if i < len(r.cells)-1 {
				line.WriteString(pad(widths[i] - displayWidth(cell) + gap))
			}
		}
		fmt.Fprintln(w, trimRight(line.String()))
		for _, d := range r.detail {
			fmt.Fprintf(w, "%s%s\n", detailIndent, d)
		}
	}
}

// writeTable prints rows in aligned columns, with no indent and no hanging
// lines. It is what a caller with nothing but cells wants.
func writeTable(w io.Writer, rows [][]string) {
	t := table{}
	for _, r := range rows {
		t.add(r...)
	}
	t.write(w)
}

func pad(n int) string {
	out := make([]byte, max(n, 0))
	for i := range out {
		out[i] = ' '
	}
	return string(out)
}

// displayWidth is how many columns a cell occupies. One per rune, which is
// right for everything in a corpus written in a Latin script and wrong for a
// wide character — a real answer needs an East Asian width table, which is a
// dependency 0029 would have to admit for something this cosmetic.
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
