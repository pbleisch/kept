package main

import (
	"fmt"
	"io"
	"sort"

	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/register"
)

// statusCmd prints the register: what has standing, at which content version,
// and what does not and why — for the whole corpus at once.
//
// One artifact at a time is not a workflow. The question this answers is *which
// of the things I approved have been changed since I approved them*, and that
// is only a question about a corpus.
//
// Nothing here writes. The whole answer is derived on the spot from two things
// that were already true: the decisions in the ledger, and what each file says
// now. See docs/decision/0001.
func statusCmd(args []string, stdout, stderr io.Writer) int {
	fs, root := flags("status", stderr)
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

	store := ledger.NewFiles(ledgerRoot(c))
	empty := ledger.Empty(store)
	decisions, quarantined, err := ledger.ReadDecisions(store)
	if err != nil {
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return exitUsage
	}
	writeQuarantined(stderr, quarantined)

	entries := register.Derive(c, decisions)
	writeRegister(stdout, entries)
	writeSummary(stdout, entries, empty)

	if n := len(c.Problems); n > 0 {
		fmt.Fprintf(stdout, "\n%s reading the corpus; kept list says what they are\n", count(n, "problem"))
	}
	return verdict(entries, quarantined)
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
// the decision that settles the question. Reporting any of those as 0 is
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

func writeRegister(w io.Writer, entries []register.Standing) {
	if len(entries) == 0 {
		return
	}
	rows := make([][]string, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, []string{e.Artifact, string(e.State), detail(e)})
	}
	writeTable(w, rows)
}

// detail says why an entry is in the state it is in, in a form that can be
// acted on. The revised case is the one worth reading closely: it names the
// content version the operator agreed to and the one the file produces now, so
// that "this stopped applying" is a fact with two hashes behind it rather than
// an assertion.
func detail(e register.Standing) string {
	switch e.State {
	case register.Ratified:
		return fmt.Sprintf("%s by %s", e.Granted.At.Format(day), e.Granted.By)

	case register.Released:
		if e.Granted.At.IsZero() {
			return "retired"
		}
		return fmt.Sprintf("retired %s by %s", e.Granted.At.Format(day), e.Granted.By)

	case register.Absent:
		return fmt.Sprintf("ratified %s at %s, and no artifact claims this id — never read as a release",
			e.Granted.At.Format(day), e.Granted.Hash.Short())

	case register.Unratified:
		switch e.Reason {
		case register.Revised:
			return fmt.Sprintf("ratified %s at %s, and it now says %s",
				e.Granted.At.Format(day), e.Granted.Hash.Short(), e.Hash.Short())
		case register.Unrehashed:
			return fmt.Sprintf("ratified %s under norm:%d, and this build hashes under norm:%d; nobody edited it",
				e.Granted.At.Format(day), e.Granted.Hash.Norm, e.Hash.Norm)
		case register.Unbinding:
			return e.Unhashable
		default:
			return "never ratified"
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
		fmt.Fprint(w, "\nThere is no decision stream here, so nothing has ever been ratified in\n"+
			"this workspace. kept ratify <artifact> is what starts one.\n")
	}
}
