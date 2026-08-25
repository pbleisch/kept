package main

import (
	"fmt"
	"io"

	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/tick"
)

// driftCmd prints what has standing and lacks truth.
//
// It measures nothing. Drift is a query result — nothing creates it and nothing
// resolves it — and a query that ran a test suite would make reading the queue
// cost what deriving it costs. `kept` is what re-derives; this is what reads
// back what `kept` found, and it says plainly when an answer is about a tree
// this is not.
func driftCmd(args []string, stdout, stderr io.Writer) int {
	fs, root := flags("drift", stderr)
	asJSON := fs.Bool("json", false, "the structured document, per decision 0027")
	f := narrow(fs, "drift", "drift kind", drifts)
	rest, err := parse(fs, args)
	if err != nil {
		return exitUsage
	}
	if len(rest) > 0 {
		fmt.Fprintf(stderr, "kept: drift takes no arguments; did you mean why %s?\n", rest[0])
		return exitUsage
	}

	c, code := open(*root, stderr)
	if code != exitOK {
		return code
	}
	if code := f.check(c, stderr); code != exitOK {
		return code
	}
	if code := sound(c, stderr); code != exitOK {
		return code
	}
	res, code := survey(c, stderr)
	if code != exitOK {
		return code
	}

	total := len(res.Subjects)
	population, narrowed := narrowSurvey(f, res)
	code = answer(narrowed)
	if f.selecting() && population == 0 {
		code = exitUnknown
	}

	if *asJSON {
		return writeDocument(stdout, stderr, newDocument("drift", c, narrowed, code, false))
	}

	switch {
	case f.selecting() && population == 0:
		f.writeUnselected(stdout, c, total)
	case f.states.any() && len(narrowed.Subjects) == 0:
		f.writeUnadmitted(stdout, population)
	default:
		writeDrift(stdout, narrowed)
		f.writeNarrowing(stdout, len(narrowed.Subjects), total)
	}
	return code
}

// drifts is what --state admits on this verb: the drift kinds, which is truth
// and never standing. `kept drift --state ratified` is refused by name and told
// where that question is asked, which is the whole of how one flag name serves
// two vocabularies without meaning two things. See [filter].
var drifts = []string{
	string(model.DriftUnverified), string(model.DriftNotKept), string(model.DriftRegressed),
	string(model.DriftStale), string(model.DriftUnknown), string(model.DriftConflicted),
	string(model.DriftAbsent),
}

// narrowSurvey applies the filter to a survey, keeping the result's own fields
// so that one document builder still serves the answer.
//
// The population is what the selectors named, drifting or not, because a
// subject with no drift is still an artifact that was asked about. --state is a
// question about a drift kind, so an artifact with no drift answers no to every
// one of them.
func narrowSurvey(f *filter, res *tick.Result) (population int, out *tick.Result) {
	if !f.any() {
		return len(res.Subjects), res
	}
	out = &tick.Result{World: res.World, Tree: res.Tree, Quarantined: res.Quarantined, Observed: res.Observed}
	for _, s := range res.Subjects {
		if !f.selects(s.ID) {
			continue
		}
		population++
		if !f.states.any() {
			out.Subjects = append(out.Subjects, s)
			continue
		}
		if s.Drifting && f.admits(string(s.Drift.Kind)) {
			out.Subjects = append(out.Subjects, s)
		}
	}
	return population, out
}

func writeDrift(w io.Writer, res *tick.Result) {
	t := table{indent: "  "}
	for _, s := range res.Subjects {
		if !s.Drifting {
			continue
		}
		t.add(string(s.Drift.Kind), ago(s.Drift.Since), s.ID)
		t.beneath(detailLines(s)...)
	}
	t.write(w)
	drifting := len(t.rows)

	if drifting == 0 {
		fmt.Fprint(w, "nothing with standing is without truth\n")
		return
	}
	fmt.Fprintf(w, "\n%s with standing and without truth\n", count(drifting, "artifact"))
	fmt.Fprint(w, "kept why <artifact> says what the tick did about one, and why\n")
}

// lookup resolves an artifact the way every verb that takes one does: by
// "type:slug", or by a slug alone when only one type answers to it.
func lookup(c *corpus.Corpus, name string, stderr io.Writer) (string, int) {
	entry, err := c.Lookup(name)
	if err != nil {
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return "", exitUsage
	}
	return entry.Artifact.ID, exitOK
}
