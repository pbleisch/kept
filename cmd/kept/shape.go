package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"runtime"

	"github.com/pbleisch/kept/internal/config"
	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/gather"
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/progress"
	"github.com/pbleisch/kept/internal/register"
	"github.com/pbleisch/kept/internal/tick"
)

// shapeCmd runs one artifact's instruments against a tree and prints what they
// measured and what its predicate makes of it. It records nothing.
//
// It exists for the sentence this repository keeps as a convention and has
// never been able to check: *a check that has never fired is not a check*. A
// promise is authored, ratified, and then holds — and until the world moves
// against it, nothing distinguishes a predicate that is watching something from
// one that cannot say `not-kept` at all. That distinction is worth the most
// before standing exists, because standing is a person's act and the vacuous
// promise it grants is indistinguishable afterwards from a promise nothing has
// broken yet.
//
// So the reading is the measurement itself rather than the verdict. A trial
// prints every instrument's output beside the answer, because *the answer is
// `kept`* tells a reader nothing about whether it could have been anything
// else, and the observation does.
//
// Nothing here writes, and that is the whole reason it is [tick.Measure] and
// not a tick with the recording turned off: one artifact, no ledger opened, no
// truth appended, no disposition. A trial is not an observation of this
// workspace — nobody has committed to the thing being trialled, and recording a
// verdict about it would put an answer in the stream for a question nobody
// asked. See docs/decision/0035.
func shapeCmd(args []string, stdout, stderr io.Writer) int {
	fs, root := flags("shape", stderr)
	trial := fs.Bool("trial", false, "measure it against a tree and print what its instruments said")
	against := fs.String("against", "", "the tree to measure; defaults to this workspace")
	quiet := fs.Bool("quiet", false, "no progress on stderr")
	rest, err := parse(fs, args)
	if err != nil {
		return exitUsage
	}
	if len(rest) != 1 {
		fmt.Fprint(stderr, "kept: shape takes one artifact, named \"type:slug\" or by its slug alone\n")
		return exitUsage
	}
	if !*trial {
		// The reading is named rather than defaulted. shape is the verb the
		// model reserves for working a candidate up before anybody grants it
		// standing, and --trial is one reading of it; a bare shape that quietly
		// meant this one could not grow a second without changing what the
		// first already did.
		fmt.Fprint(stderr, "kept: shape needs a reading, and --trial is the one it has\n")
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

	tree := c.Workspace
	if *against != "" {
		tree = *against
	}

	cfg, err := config.Read(filepath.Join(c.Workspace, corpus.ConfigDir))
	if err != nil {
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return exitUsage
	}

	show := progress.For(stdout, stderr, *quiet)
	answer, observed, err := tick.Measure(context.Background(), tick.MeasureRequest{
		Root:      tree,
		Corpus:    c,
		Artifact:  entry.Artifact.ID,
		Catalogue: gather.Builtin(),
		Deadline:  cfg.Gatherer,
		Exclude:   cfg.Exclude,
		Workers:   runtime.NumCPU(),
		Progress:  show,
	})
	show.Close()
	if err != nil {
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return exitUsage
	}

	writeTrial(stdout, c, entry.Artifact.ID, tree, answer, observed)
	return trialCode(answer)
}

// trialCode is what a trial exits.
//
// It answers *did this settle the question* and never *is anything owed*, and
// the difference is the whole occasion for the verb: a trial is run before
// standing exists, so `not-kept` is the outcome it is most often looking for
// and reporting the best news as a failure would invert it. Nothing is in
// breach of a candidate nobody ratified.
//
// So drift's code is not reachable here and 3 keeps its meaning from
// docs/decision/0027 exactly: no evidence either way. Two states reach it and
// they are not the same state — a gatherer that could not run is a verdict of
// `unknown`, and a promise whose predicate was never applied reached no verdict
// at all — but they answer this question alike, and the words above the exit
// code are where they are told apart.
func trialCode(answer tick.Answer) int {
	if !answer.Reached() || answer.Value == model.Unknown {
		return exitUnknown
	}
	return exitOK
}

// writeTrial prints the reading: the verdict, what each instrument measured,
// and who is held to any of it.
//
// The observations are the point and are never elided. A verdict alone cannot
// answer the question the verb is for — `kept` from an instrument that measured
// an empty set and `kept` from one that measured the world are the same word —
// so what the predicate read is printed beside what it concluded.
func writeTrial(w io.Writer, c *corpus.Corpus, id, tree string, answer tick.Answer, observed map[string]model.Observation) {
	t := table{indent: "  "}
	t.add(trialVerdict(answer), id)
	t.beneath(answer.Detail)
	t.write(w)

	if len(observed) == 0 {
		fmt.Fprint(w, "\nno instrument recorded a measurement\n")
	} else {
		fmt.Fprint(w, "\nmeasured\n\n")
		m := table{indent: "  "}
		for _, name := range sortedNames(observed) {
			obs := observed[name]
			for _, field := range sortedNames(obs.Data) {
				m.add(name, field, render(obs.Data[field]))
			}
			if len(obs.Data) == 0 {
				m.add(name, "", "— nothing")
			}
		}
		m.write(w)
	}

	fmt.Fprintf(w, "\nagainst %s\n", tree)
	fmt.Fprintf(w, "%s\n", heldTo(c, id))
}

// trialVerdict is the word a trial reports, and `unevaluated` is one of them.
//
// A measurement that reached no verdict is not `unknown`: unknown is an answer
// a predicate produced, and this is the predicate never having been applied —
// an instrument that could not run, or a promise that names none. Collapsing
// the two here would be the same mistake the three-valued verdict exists to
// prevent, one verb further out.
func trialVerdict(answer tick.Answer) string {
	if !answer.Reached() {
		return "unevaluated"
	}
	return string(answer.Value)
}

// heldTo says whether anybody is committed to what was just measured.
//
// It is the sentence the verb exists to put in front of a reader before they
// type `kept ratify`, and it reads the register rather than the world: a trial
// of something ratified is a second opinion, and a trial of something nobody
// has granted is what this is for. Nothing is advanced and nothing is written —
// a reader that moved the ledger could not be run against a checkout it may not
// write. See docs/decision/0062.
func heldTo(c *corpus.Corpus, id string) string {
	acts, _, err := ledger.ReadActs(ledgerStore(c))
	if err != nil {
		// The ledger is unreadable, which is a real problem and not this
		// verb's. The measurement above stands on its own, so the reading is
		// printed and only the standing line says it could not tell.
		return "nothing here says whether this is ratified: " + err.Error()
	}
	for _, s := range register.Derive(c, acts) {
		if s.Artifact != id {
			continue
		}
		if s.State == register.Ratified {
			return "this is ratified, so the answer above is one somebody is held to. Nothing was recorded."
		}
		return "nothing is held to this yet. Nothing was recorded."
	}
	return "nothing is held to this yet. Nothing was recorded."
}
