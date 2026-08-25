package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/canonical"
	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/gather"
	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/register"
	"github.com/pbleisch/kept/internal/tick"
)

// The presentation promises, each measured against this repository's own
// corpus rather than a fixture.
//
// A fixture cannot answer the question these exist for. What broke the layout
// they replaced was an artifact id longer than the thirty-eight characters the
// format string had guessed, and every id short enough to fit aligned
// perfectly — so a table built for a test would have passed on both the broken
// code and the fixed code. The corpus in docs/ has ids up to seventy-three
// characters, which is where the guess ran out.

// thisCorpus is kept's own corpus, loaded the way the command loads it.
func thisCorpus(t *testing.T) *corpus.Corpus {
	t.Helper()
	root := filepath.Join("..", "..", "docs")
	workspace := filepath.Join("..", "..")
	c, err := corpus.Load(root, workspace, corpus.Options{Gatherers: gather.Builtin()})
	if err != nil {
		t.Fatalf("load kept's own corpus: %s", err)
	}
	if len(c.All()) == 0 {
		t.Fatal("kept's own corpus read as empty, so nothing here is measuring anything")
	}
	return c
}

// longestID is the id these tables have to survive, reported so that a failure
// says which one it was.
func longestID(c *corpus.Corpus) string {
	var longest string
	for _, e := range c.All() {
		if len(e.Artifact.ID) > len(longest) {
			longest = e.Artifact.ID
		}
	}
	return longest
}

// fields returns where each column of one rendered row begins, for rows at the
// given indent. A row is a line whose leading whitespace is exactly the table's
// indent; anything deeper is a hanging detail line and belongs to the row above.
//
// Columns are separated by two or more spaces, which is what the renderer
// emits and what `column -t` and `git branch -v` both produce.
//
// It counts runes rather than bytes, and the first draft did not. A gatherer
// pair renders as `go/imports · go/symbols`, whose separator is two bytes and
// one column, so a byte-counting check reported that row as one place further
// along than every other and called a correct table misaligned. The renderer
// measures in runes; a check that measures in bytes is asking a different
// question.
func fields(line string, indent int) ([]int, bool) {
	lead := len(line) - len(strings.TrimLeft(line, " "))
	if lead != indent || strings.TrimSpace(line) == "" {
		return nil, false
	}

	r := []rune(line)
	var starts []int
	at := indent
	for at < len(r) {
		// Skip the gap, then record where the field begins.
		for at < len(r) && r[at] == ' ' {
			at++
		}
		if at >= len(r) {
			break
		}
		starts = append(starts, at)
		// Run to the next gap of two or more spaces.
		for at < len(r) {
			if r[at] == ' ' && at+1 < len(r) && r[at+1] == ' ' {
				break
			}
			at++
		}
	}
	return starts, true
}

// aligned reports whether every row of a rendered table puts its columns in the
// same places. It returns the offending row so a failure can name it.
//
// Two properties, and the first is the one the old layout broke: the last
// column of every row begins at the same offset, whatever is in the columns
// before it. The second catches a subtler break — a row whose columns land
// somewhere no other row's do — by requiring every offset to come from one set
// no larger than the widest row.
func aligned(rendered string, indent int) (string, error) {
	var rows [][]int
	var lines []string
	for _, line := range strings.Split(rendered, "\n") {
		starts, ok := fields(line, indent)
		if !ok || len(starts) == 0 {
			continue
		}
		rows = append(rows, starts)
		lines = append(lines, line)
	}
	if len(rows) < 2 {
		return "", fmt.Errorf("only %d rows rendered, which cannot demonstrate alignment", len(rows))
	}

	last, widest := rows[0][len(rows[0])-1], 0
	offsets := map[int]bool{}
	for i, starts := range rows {
		if got := starts[len(starts)-1]; got != last {
			return lines[i], fmt.Errorf("last column begins at %d here and at %d on the first row", got, last)
		}
		if len(starts) > widest {
			widest = len(starts)
		}
		for _, at := range starts {
			offsets[at] = true
		}
	}
	if len(offsets) > widest {
		return "", fmt.Errorf("rows put columns in %d different places, and the widest row has %d columns", len(offsets), widest)
	}
	return "", nil
}

func mustAlign(t *testing.T, what, rendered string, indent int) {
	t.Helper()
	if row, err := aligned(rendered, indent); err != nil {
		t.Errorf("%s does not align: %s\n%s\n%s", what, err, row, rendered)
	}
}

// Every table kept prints aligns over every id in this corpus. The four verbs
// are done together because a fix to one that left the others is worth less
// than half: they are one shape, and a reader moves between them.
func TestEveryTableAlignsOverThisRepositorysOwnCorpus(t *testing.T) {
	c := thisCorpus(t)
	t.Logf("%d artifacts, longest id %d characters: %s", len(c.All()), len(longestID(c)), longestID(c))

	var list bytes.Buffer
	writeInventory(&list, c.All())
	mustAlign(t, "kept list", list.String(), 0)

	var status bytes.Buffer
	writeRegister(&status, registerOver(c))
	mustAlign(t, "kept status", status.String(), 2)

	var drift bytes.Buffer
	writeDrift(&drift, driftingOver(c))
	mustAlign(t, "kept drift", drift.String(), 2)

	var verdicts bytes.Buffer
	writeVerdicts(&verdicts, c, verdictsOver(c))
	mustAlign(t, "the tick", verdicts.String(), 2)
}

// The guard that breaks the thing it is supposed to catch: the layout these
// replaced, given the same real ids, and the alignment check watching it fail.
//
// Without this, `aligned` is a claim rather than a check. It is written against
// the exact format string that shipped — `%-38s` for the id, then the verdict —
// because the bug was not that the columns were in the wrong order but that
// their width was a guess, and the ids in docs/ are what ran the guess out.
func TestTheLayoutThatGuessedAWidthDoesNotAlignHere(t *testing.T) {
	c := thisCorpus(t)

	var guessed bytes.Buffer
	for _, s := range verdictsOver(c).Subjects {
		fmt.Fprintf(&guessed, "  %s\n", strings.TrimRight(fmt.Sprintf("%-38s %-10s %s", s.ID, "kept", "by go/tests"), " "))
	}

	row, err := aligned(guessed.String(), 2)
	if err == nil {
		t.Fatalf("a width guessed at 38 aligned every id in this corpus, so this check cannot fail:\n%s", guessed.String())
	}
	t.Logf("caught, as it should be: %s\n%s", err, row)
}

// registerOver is every artifact in a corpus with standing, so that the
// register renders over the real ids. The states are varied deliberately: a
// table in which every row holds the same cell can align by accident.
func registerOver(c *corpus.Corpus) []register.Standing {
	var out []register.Standing
	for i, e := range c.All() {
		st := register.Standing{
			Artifact: e.Artifact.ID,
			State:    register.Ratified,
			Hash:     canonical.Hash{Algorithm: "sha256", Digest: strings.Repeat("a", 64), Norm: 1},
			Granted: register.Grant{
				Hash: canonical.Hash{Algorithm: "sha256", Digest: strings.Repeat("b", 64), Norm: 1},
				By:   "Paul Bleisch <pbleisch@acm.org>",
				At:   time.Date(2026, 8, 19, 10, 0, 0, 0, time.UTC),
			},
		}
		switch i % 7 {
		case 3:
			st.State, st.Reason = register.Unratified, register.Revised
		case 5:
			st.State, st.Reason, st.Granted = register.Unratified, register.Never, register.Grant{}
		case 6:
			st.State = register.Released
		}
		out = append(out, st)
	}
	return out
}

func driftingOver(c *corpus.Corpus) *tick.Result {
	res := &tick.Result{}
	kinds := []model.DriftKind{model.DriftNotKept, model.DriftStale, model.DriftUnverified}
	for i, e := range c.All() {
		res.Subjects = append(res.Subjects, tick.Subject{
			ID:       e.Artifact.ID,
			Entry:    e,
			Drifting: true,
			Drift: model.Drift{
				Kind:  kinds[i%len(kinds)],
				Since: time.Now().Add(-time.Duration(i+1) * time.Hour),
			},
		})
	}
	return res
}

func verdictsOver(c *corpus.Corpus) *tick.Result {
	res := &tick.Result{}
	values := []model.Value{model.Kept, model.NotKept, model.Unknown}
	for i, e := range c.All() {
		s := tick.Subject{ID: e.Artifact.ID, Entry: e, Drifting: true}
		s.Drift = model.Drift{Kind: model.DriftStale, Since: time.Now().Add(-time.Hour)}
		s.Answer.Value = values[i%len(values)]
		s.Answer.Since = time.Now().Add(-time.Duration(i+1) * time.Hour)
		s.Answer.World = model.World{Tree: strings.Repeat("c", 64)}
		s.Answer.Fresh = i%4 != 0
		res.Subjects = append(res.Subjects, s)
	}
	return res
}

// A cell wider than the column is what widens the column, and never what pushes
// the row out of line. This is the property in one assertion, on the value that
// broke it.
func TestAColumnIsAsWideAsItsWidestCell(t *testing.T) {
	var w bytes.Buffer
	tab := table{indent: "  "}
	tab.add("kept", "short")
	tab.add("kept", strings.Repeat("x", 90))
	tab.add("not-kept", "short")
	tab.write(&w)

	mustAlign(t, "a table with one very wide cell", w.String(), 2)
	if strings.Contains(w.String(), "…") || strings.Contains(w.String(), strings.Repeat("x", 89)+"\n") {
		return
	}
	if !strings.Contains(w.String(), strings.Repeat("x", 90)) {
		t.Errorf("the wide cell was not written whole:\n%s", w.String())
	}
}

// Nothing trails a line with spaces, in any table, over the real corpus. A
// trailing space is invisible in a terminal and is a difference in a diff, a
// golden file and a `grep -c ' $'`.
func TestNoRenderedLineEndsInWhitespace(t *testing.T) {
	c := thisCorpus(t)

	renderings := map[string]func(*bytes.Buffer){
		"kept list":   func(w *bytes.Buffer) { writeInventory(w, c.All()) },
		"kept status": func(w *bytes.Buffer) { writeRegister(w, registerOver(c)) },
		"kept drift":  func(w *bytes.Buffer) { writeDrift(w, driftingOver(c)) },
		"the tick":    func(w *bytes.Buffer) { writeVerdicts(w, c, verdictsOver(c)) },
	}
	for _, name := range sorted(renderings) {
		var w bytes.Buffer
		renderings[name](&w)
		for i, line := range strings.Split(w.String(), "\n") {
			if line != strings.TrimRight(line, " \t") {
				t.Errorf("%s line %d ends in whitespace: %q", name, i+1, line)
			}
		}
	}
}

func sorted[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// The answer on stdout is the same bytes whatever stdout is.
//
// kept asks whether a stream is a terminal in exactly one place — the progress
// display, which writes to stderr and is covered by docs/decision/0065 — and
// nothing that renders an answer asks at all. That is a decision rather than an
// omission: a table that rearranged itself when it was piped would be a second
// layout nobody had read, and kept cannot measure a terminal's width anyway.
// See docs/decision/0066.
func TestTheAnswerOnStdoutIsTheSameBytesWhateverStdoutIs(t *testing.T) {
	for _, args := range [][]string{
		{"list", "--corpus", fixture},
		{"show", "core-packages-are-tested", "--corpus", fixture},
	} {
		var buffered bytes.Buffer
		codeA := dispatch(args, &buffered, &bytes.Buffer{})

		path := filepath.Join(t.TempDir(), "piped")
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		codeB := dispatch(args, f, &bytes.Buffer{})
		f.Close()
		piped, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		if codeA != codeB {
			t.Errorf("kept %v exited %d to a buffer and %d to a file", args, codeA, codeB)
		}
		if buffered.String() != string(piped) {
			t.Errorf("kept %v wrote different bytes to a buffer and to a file:\n%q\n%q", args, buffered.String(), piped)
		}
	}
}

// And it carries nothing a pipe cannot hold: no escape, no carriage return, no
// trailing whitespace. The progress display owns the only cursor control kept
// emits and it is on stderr.
func TestTheAnswerOnStdoutCarriesNoTerminalControl(t *testing.T) {
	for _, args := range [][]string{
		{"list", "--corpus", fixture},
		{"status", "--corpus", fixture},
		{"show", "core-packages-are-tested", "--corpus", fixture},
	} {
		_, stdout, _ := kept(t, args...)
		if strings.ContainsAny(stdout, "\x1b\r") {
			t.Errorf("kept %v put terminal control on stdout: %q", args, stdout)
		}
		for i, line := range strings.Split(stdout, "\n") {
			if line != strings.TrimRight(line, " \t") {
				t.Errorf("kept %v line %d ends in whitespace: %q", args, i+1, line)
			}
		}
	}
}

// Two runs over one corpus print one answer. A rendering that depended on map
// iteration, on the order a directory was read, or on anything else that moves
// on its own would fail here and nowhere else, because a single run of a
// single-run test always looks correct.
func TestTheSameCorpusPrintsTheSameAnswerTwice(t *testing.T) {
	for _, args := range [][]string{
		{"list", "--corpus", fixture},
		{"status", "--corpus", fixture},
	} {
		_, first, _ := kept(t, args...)
		_, second, _ := kept(t, args...)
		if first != second {
			t.Errorf("kept %v printed two different answers:\n%s\n%s", args, first, second)
		}
	}
}
