package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/pbleisch/kept/internal/corpus"
)

func listCmd(args []string, stdout, stderr io.Writer) int {
	fs, root := flags("list", stderr)
	f := narrow(fs, "list", "", nil)
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
	if code := f.check(c, stderr); code != exitOK {
		return code
	}

	all := c.All()
	entries := narrowInventory(f, all)

	// A selector naming nothing is exit 3 here as it is everywhere else: the
	// question was not put. Unfiltered, this verb still only ever exits 0, and
	// an empty corpus is not an unanswered question — nobody asked about a
	// subset of it. See [filter].
	if f.selecting() && len(entries) == 0 {
		f.writeUnselected(stdout, c, len(all))
		writeProblems(stdout, c, 0)
		return exitUnknown
	}

	writeInventory(stdout, entries)

	if len(entries) == 0 {
		fmt.Fprintf(stdout, "no artifacts under %s\n", c.Root)
	}
	writeProblems(stdout, c, len(entries))
	f.writeNarrowing(stdout, len(entries), len(all))
	return exitOK
}

// narrowInventory keeps the artifacts a selector named, in the order the corpus
// read them.
//
// There is no --state here and the flag is refused by name: list reports no
// state, because it never touches the ledger and standing and truth both live
// there. What it reports beside the inventory is problems, and those are never
// narrowed — a defect somebody must fix is a fact about reading the corpus
// rather than about the rows a reader asked to see, and a filter is not a
// licence to hide one. See [filter].
func narrowInventory(f *filter, all []*corpus.Entry) []*corpus.Entry {
	if !f.any() {
		return all
	}
	var out []*corpus.Entry
	for _, e := range all {
		if f.selects(e.Artifact.ID) {
			out = append(out, e)
		}
	}
	return out
}

// writeInventory prints one artifact per line: the id, and the title only when
// the title says something the id does not.
//
// Two columns became one because the first of the three was the type, printed
// again on every row of a list already grouped by it, and the id every other
// verb prints carries the type in front of the slug anyway. One id, in the one
// spelling `show`, `why`, `ratify` and `repair` all take, is both narrower and
// the only rendering a reader can paste into the next command.
//
// The title stopped being a column because it was mostly the same sentence
// twice. Over this repository's own corpus, sixty-five of eighty-six titles
// were the slug with a clause added and nineteen were the slug spelled out with
// spaces — `promise  core-packages-are-tested  Core packages are tested`, a
// column of width thirty-eight buying a reader nothing. The twenty that do say
// something else hang under their id instead of widening every row to fit them,
// because a column only a quarter of the rows fill is a gutter for the other
// three quarters. What is lost is the clause a decision's title adds after its
// slug runs out, and that is a stated cost: `kept show` prints one artifact
// whole and `kept status --json` carries every title in the corpus, so this is
// not the only place a title can be read.
func writeInventory(w io.Writer, entries []*corpus.Entry) {
	var t table
	for _, e := range entries {
		t.add(e.Artifact.ID)
		if !restatesID(e.Artifact.ID, e.Artifact.Title) {
			t.beneath(e.Artifact.Title)
		}
	}
	t.write(w)
}

// restatesID reports whether a title is its own slug read aloud.
//
// The test is on the letters and digits alone, in order: a slug that is a
// prefix of its title has already said everything the title's opening says, and
// what a decision's title adds after that point is a clause `kept show` will
// give in full. A corpus whose slugs are not descriptive — `ch01` against *The
// Fall of Rome* — fails the test on the first letter and keeps its titles,
// which is the case this rule exists to leave alone.
func restatesID(id, title string) bool {
	_, slug := corpus.SplitRef(id)
	if slug == "" || title == "" {
		return false
	}
	return strings.HasPrefix(letters(title), letters(slug))
}

// letters is a string reduced to what a slug and a title have in common: the
// letters and digits, lowercased, in the order they were written. Punctuation,
// spaces and the em dash a decision's title carries are what differ between the
// two spellings of one sentence, so none of them may count as a difference.
func letters(s string) string {
	var out strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out.WriteRune(r)
		}
	}
	return out.String()
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
	writeProblemList(w, c.Problems)
}

// writeProblemList names every problem: where it is, what kind it is, and what
// it says.
//
// One function with two callers — this verb and the tick — because the tick
// used to print a count and the name of another command, and a count is the
// weakest useful thing kept can say about a defect it has already identified in
// full. Two renderings of one list would be two answers that can disagree; this
// way the tick cannot become less specific than kept list without somebody
// deciding to make it so.
//
// Nothing is elided. A truncated list is the same failure one step along — a
// summary that invites a reader to believe they have seen everything — the
// count is bounded by the corpus a person wrote, and these lines come after the
// verdicts rather than instead of them.
func writeProblemList(w io.Writer, problems []corpus.Problem) {
	for _, p := range problems {
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
