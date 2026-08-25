package main

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/register"
	"github.com/pbleisch/kept/internal/tick"
)

// statusCmd prints the register: what has standing, at which content version,
// and what does not and why — for the whole corpus at once.
//
// One artifact at a time is not a workflow. The question this answers is *which
// of the things I approved have been changed since I approved them*, and that
// is only a question about a corpus.
//
// Nothing here writes. The whole answer is derived on the spot from two things
// that were already true: the acts in the ledger, and what each file says
// now. See docs/decision/0001.
func statusCmd(args []string, stdout, stderr io.Writer) int {
	fs, root := flags("status", stderr)
	asJSON := fs.Bool("json", false, "the structured document, per decision 0027")
	rest, err := parse(fs, args)
	if err != nil {
		return exitUsage
	}
	if len(rest) > 0 {
		fmt.Fprintf(stderr, "kept: status takes no arguments; did you mean show %s?\n", rest[0])
		return exitUsage
	}

	c, code := open(*root, stderr)
	if code != exitOK {
		return code
	}
	if code := sound(c, stderr); code != exitOK {
		return code
	}

	store := ledgerStore(c)
	empty := ledger.Empty(store)
	acts, quarantined, err := ledger.ReadActs(store)
	if err != nil {
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return exitUsage
	}
	writeQuarantined(stderr, quarantined)

	entries := register.Derive(c, acts)
	code = verdict(entries, quarantined)

	if *asJSON {
		// The same envelope every other question answers in, with truth absent
		// rather than empty: status looks at the register and never at the
		// world, and an absent verdict says so. See docs/decision/0027.
		return writeDocument(stdout, stderr, newDocument("status", c, registered(c, entries, quarantined), code, false))
	}

	writeRegister(stdout, entries)
	writeSummary(stdout, entries, empty)

	if n := len(c.Problems); n > 0 {
		fmt.Fprintf(stdout, "\n%s reading the corpus; kept list says what they are\n", count(n, "problem"))
	}
	return code
}

// verdict is the register as an exit code, so that this verb can be a hook.
// See docs/decision/0027 for the contract.
//
// 0027 reserves 1 for drift, which is standing measured against a truth that is
// not kept, and there is no truth here. What the register can answer is the
// half of that needing no observation — something was ratified and does not
// stand now — and both ways it happens are 1. Either it was edited, so what is
// in force was granted against a wording this is no longer, or its content is
// gone, which 0021 names a drift kind outright and never a release.
//
// An artifact nobody ever ratified is 0. It obliges nobody, an ordinary corpus
// is full of them, and exiting 1 for one would make the code useless in the
// hook it exists for.
//
// 3 is standing kept could not decide, which is not the same as standing that
// lapsed. A grant computed under canonicalization rules this build does not use
// cannot be compared against at all (0022); an artifact whose schema has gone
// has nothing to hash; and a ledger line that could not be read might have been
// the act that settles the question. Reporting any of those as 0 is
// exactly the false pass 0027's third code exists to prevent, and reporting
// them as 1 would send a reader looking for an edit nobody made.
func verdict(entries []register.Standing, quarantined []ledger.Quarantined) int {
	code := exitOK
	if len(quarantined) > 0 {
		code = exitUnknown
	}
	for _, e := range entries {
		switch {
		case e.State == register.Absent:
			return exitDrift
		case e.State != register.Unratified || e.Granted.At.IsZero():
			// In force, retired, or never granted. None of them is a finding.
			continue
		case e.Reason == register.Revised:
			return exitDrift
		default:
			// Granted once, and the comparison could not be made.
			code = exitUnknown
		}
	}
	return code
}

// registered is the register as a result, so that one document builder serves
// every question. It names no world, because none was walked.
func registered(c *corpus.Corpus, entries []register.Standing, quarantined []ledger.Quarantined) *tick.Result {
	res := &tick.Result{Quarantined: quarantined}
	for _, e := range entries {
		s := tick.Subject{ID: e.Artifact, Standing: e}
		if entry, ok := c.Get(e.Artifact); ok {
			s.Entry = entry
		}
		res.Subjects = append(res.Subjects, s)
	}
	return res
}

// writeRegister prints the register: what state each artifact is in, when the
// act behind it was performed, and which artifact it is.
//
// The state is first and the id is last, which is the reverse of what this
// printed before and most of why it was hard to read. Every row of an ordinary
// corpus says the same word in the state column — eighty-five of the
// eighty-six here say `ratified` — so that column is what a reader is
// scanning, and it was starting at column seventy-five, past the right edge of
// a terminal, pushed there by the longest artifact id. Fixed-width answer
// left, unbounded name right, is `ls -l` and `git status -s`, and it is the
// only order in which a line too long for the screen spends its overflow on
// the part the reader has already found.
//
// The principal is not a column at all, and that is the rest of it. Every row
// ended `by Paul Bleisch <pbleisch@acm.org>`, thirty-four columns repeated
// eighty-six times, and the question this verb answers — *which of the things
// I approved have been changed since I approved them* — has one person in it
// already. So the names go under the table, said once, and per-row attribution
// is `kept why <artifact>`, which names the principal every time, and
// `--json`, which carries the whole grant. That is a stated cost rather than a
// free win: a corpus several people ratify into cannot be filtered by eye any
// more. It is the right trade because a name that distinguishes nothing is
// width, and 0048 is why the alternative is not to guess a shorter one — the
// operator is named and never guessed, so kept prints what the act says or
// nothing.
func writeRegister(w io.Writer, entries []register.Standing) {
	if len(entries) == 0 {
		return
	}

	t := table{indent: "  "}
	for _, e := range entries {
		t.add(standingOf(e), grantedOn(e), e.Artifact)
		t.beneath(lapsed(e))
	}
	t.write(w)
	writeGrantors(w, entries)
}

// writeGrantors names who performed the acts this register is derived from.
//
// Sorted, so that the line does not depend on the order the ledger folded in,
// and counted rather than listed past three: a corpus with more ratifiers than
// that wants `--json`, and a footer that runs to two lines is a second table.
func writeGrantors(w io.Writer, entries []register.Standing) {
	seen := map[string]bool{}
	var names []string
	for _, e := range entries {
		if by := e.Granted.By; by != "" && !seen[by] {
			seen[by] = true
			names = append(names, by)
		}
	}
	sort.Strings(names)

	switch {
	case len(names) == 0:
	case len(names) == 1:
		fmt.Fprintf(w, "\nevery act here is %s's\n", names[0])
	case len(names) <= 3:
		fmt.Fprintf(w, "\ngranted by %s\n", strings.Join(names, ", "))
	default:
		fmt.Fprintf(w, "\ngranted by %s; kept why <artifact> names the one behind a row\n",
			count(len(names), "person"))
	}
}

// standingOf is the state as a person says it.
//
// Four of the five states are their own word. The fifth is `unratified`, which
// covers an artifact nobody ever granted and one whose content has moved since
// somebody did, and those two want opposite things from a reader — the first is
// the ordinary condition of a corpus and the second is a finding. So the one
// with nothing else to say says it here, and every other `unratified` row
// carries a line under it saying which way. The state in the ledger and in
// `--json` is unchanged: this is a wording, not a sixth state.
func standingOf(e register.Standing) string {
	if e.State == register.Unratified && e.Reason == register.Never {
		return "never ratified"
	}
	return string(e.State)
}

// grantedOn is the day the act behind this state was performed, as a table
// cell. Which act it was is said by the state standing beside it: a ratified
// artifact was granted then, and a released one was retired then.
func grantedOn(e register.Standing) string {
	if e.Granted.At.IsZero() {
		return ""
	}
	return e.Granted.At.Format(day)
}

// lapsed says what has happened to standing since it was granted, and is empty
// whenever the state already says it.
//
// The revised case is the one worth reading closely: it names the content
// version the operator agreed to and the one the file produces now, so that
// "this stopped applying" is a fact with two hashes behind it rather than an
// assertion. `never` and `released` return nothing, because a line reading
// "never ratified" under a row reading `unratified` is the state a second time.
func lapsed(e register.Standing) string {
	switch e.State {
	case register.Absent:
		return fmt.Sprintf("granted at %s, and no artifact claims this id — never read as a release",
			e.Granted.Hash.Short())

	case register.Unratified:
		switch e.Reason {
		case register.Revised:
			return fmt.Sprintf("granted at %s, and it now says %s", e.Granted.Hash.Short(), e.Hash.Short())
		case register.Unrehashed:
			return fmt.Sprintf("granted under norm:%d, and this build hashes under norm:%d; nobody edited it",
				e.Granted.Hash.Norm, e.Hash.Norm)
		case register.Unbinding:
			return e.Unhashable
		}
	}
	return ""
}

func writeSummary(w io.Writer, entries []register.Standing, empty bool) {
	if len(entries) == 0 {
		fmt.Fprint(w, "no artifacts, so nothing to have standing\n")
		return
	}

	counts := register.Count(entries)
	states := make([]string, 0, len(counts))
	for state, n := range counts {
		states = append(states, fmt.Sprintf("%d %s", n, state))
	}
	sort.Strings(states)

	fmt.Fprintf(w, "\n%s", count(len(entries), "artifact"))
	for _, s := range states {
		fmt.Fprintf(w, " · %s", s)
	}
	fmt.Fprintln(w)

	// A ledger nobody has written and a ledger whose records have all been
	// pruned look identical in the table above, and one of them means a clone
	// that never fetched the ledger. Saying which is the only way that does not
	// read as a system nobody has ever used. See docs/decision/0016.
	if empty {
		fmt.Fprint(w, "\nThere is no act stream here, so nothing has ever been ratified in\n"+
			"this workspace. kept ratify <artifact> is what starts one.\n")
	}
}
