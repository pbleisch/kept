package main

import (
	"fmt"
	"io"

	"github.com/pbleisch/kept/internal/register"
	"github.com/pbleisch/kept/internal/tick"
)

// whyCmd explains one artifact: what obliges anyone, what is so, and what the
// tick did about the gap between them.
//
// The third section is the one this verb exists for. Truth can be re-derived by
// observing again; a refusal cannot — the budget's balance at that moment, the
// lock another run held then and the rank among that tick's drift are all gone
// — so *why did you not fix this* is answerable only because every tick wrote
// down its answer at the time. See docs/decision/0019.
//
// It measures nothing, and it folds the disposition stream across every world
// on purpose. The question is about the queue rather than about a tree:
// splitting "refused on four days and suppressed on two" per world would
// fragment the one question this verb was written to make answerable, and would
// do it in a way that looks like more precision. See docs/decision/0035.
func whyCmd(args []string, stdout, stderr io.Writer) int {
	fs, root := flags("why", stderr)
	asJSON := fs.Bool("json", false, "the structured document, per decision 0027")
	rest, err := parse(fs, args)
	if err != nil {
		return exitUsage
	}
	if len(rest) != 1 {
		fmt.Fprint(stderr, "kept: why takes one artifact, named \"type:slug\" or by its slug alone\n")
		return exitUsage
	}

	c, code := open(*root, stderr)
	if code != exitOK {
		return code
	}
	id, code := lookup(c, rest[0], stderr)
	if code != exitOK {
		return code
	}
	res, code := survey(c, stderr)
	if code != exitOK {
		return code
	}

	one := &tick.Result{World: res.World, Tree: res.Tree, Quarantined: res.Quarantined}
	for _, s := range res.Subjects {
		if s.ID == id {
			one.Subjects = append(one.Subjects, s)
		}
	}
	if len(one.Subjects) == 0 {
		fmt.Fprintf(stderr, "kept: %s is not in the register\n", id)
		return exitUsage
	}

	code = answer(one)
	if *asJSON {
		return writeDocument(stdout, stderr, newDocument("why", c, one, code, true))
	}
	writeWhy(stdout, one.Subjects[0])
	return code
}

func writeWhy(w io.Writer, s tick.Subject) {
	fmt.Fprintln(w, s.ID)
	if s.Entry != nil {
		if s.Entry.Artifact.Title != "" {
			fmt.Fprintf(w, "  %s\n", s.Entry.Artifact.Title)
		}
		if s.Entry.Artifact.Statement != "" {
			fmt.Fprintf(w, "  %s\n", s.Entry.Artifact.Statement)
		}
	}

	writeStandingSection(w, s)
	writeTruthSection(w, s)
	writeDispositionSection(w, s)
}

func writeStandingSection(w io.Writer, s tick.Subject) {
	fmt.Fprint(w, "\nstanding\n")
	fmt.Fprintf(w, "  %s — %s\n", s.Standing.State, detail(s.Standing))
	if s.Standing.State == register.Ratified {
		fmt.Fprintf(w, "  in force against %s, which is what this file says now\n", s.Standing.Hash.Short())
	}
}

// writeTruthSection prints the verdict in force and the series behind it.
//
// The world is printed beside every verdict rather than left implicit, because
// a verdict reached in another tree is a different fact about a different
// world, and one carried forward is in force only because nothing this artifact
// looks at has moved since. Saying which is which is the difference between an
// answer and an assertion.
func writeTruthSection(w io.Writer, s tick.Subject) {
	fmt.Fprint(w, "\ntruth\n")
	switch {
	case !s.Predicated() && s.Entry != nil:
		fmt.Fprint(w, "  nothing observes this: it names no gatherer, so it binds without being evaluated\n")
	case s.Entry == nil:
		fmt.Fprint(w, "  no artifact claims this id, so there is nothing to observe\n")
	case s.Unevaluated != "":
		fmt.Fprintf(w, "  no verdict: %s\n", s.Unevaluated)
	case !s.Answer.Reached():
		fmt.Fprintf(w, "  nothing has answered — %s\n", s.Because)
	default:
		fmt.Fprintf(w, "  %s since %s (%s)\n", s.Answer.Value, s.Answer.Since.Local().Format(stamp), ago(s.Answer.Since))
		if s.Answer.Detail != "" {
			fmt.Fprintf(w, "      %s\n", s.Answer.Detail)
		}
		fmt.Fprintf(w, "      reached in world %s%s\n", short(s.Answer.World.Tree), carriedNote(s))
		for _, address := range s.Answer.From {
			fmt.Fprintf(w, "      from observation %s\n", short(address))
		}
	}
	if s.Due && s.Because != "" && s.Answer.Reached() {
		fmt.Fprintf(w, "  due: %s\n", s.Because)
	}

	if len(s.Truth) == 0 {
		return
	}
	fmt.Fprint(w, "  history\n")
	for _, r := range s.Truth {
		// A verdict about an earlier wording is said to be one. Standing and
		// truth are both keyed to a content version, so those verdicts were
		// about a different statement and never transfer to this one.
		var wording string
		if r.Hash != s.Standing.Hash.String() {
			wording = "  · an earlier wording"
		}
		fmt.Fprintf(w, "      %s  %-9s %s%s\n", r.At.Local().Format(stamp), r.Value, r.Detail, wording)
	}
}

// carriedNote says what an answer reached in another tree is doing here. There
// are two of those and they are opposites: one still applies because nothing
// the artifact looks at has moved, and the other is the last thing anyone knew.
func carriedNote(s tick.Subject) string {
	switch {
	case s.Answer.Fresh:
		return ""
	case s.Due:
		return ", and it is the last answer rather than the answer — the bytes\n      this artifact looks at have moved, and kept is what re-derives it"
	default:
		return ", and carried forward because nothing this artifact looks at has moved since"
	}
}

// writeDispositionSection is the answer to "why was this drift left alone".
//
// It prints the whole stream, across every world, because the question is about
// the queue rather than about a tree. And it says plainly when what it is
// printing is history: a refusal that is no longer in force reads as a live one
// if nothing distinguishes them.
func writeDispositionSection(w io.Writer, s tick.Subject) {
	fmt.Fprint(w, "\ndisposition\n")
	d, held := dispositionOf(s)

	switch {
	case s.Drifting:
		fmt.Fprintf(w, "  %s since %s\n", s.Drift.Kind, ago(s.Drift.Since))
		if !held {
			fmt.Fprint(w, "  no tick has disposed of this yet; kept is what records one\n")
			return
		}
		fmt.Fprintf(w, "  %s", d.Outcome)
		if d.Condition != "" {
			fmt.Fprintf(w, " · %s", d.Condition)
		}
		if at := s.DisposedSince(); !at.IsZero() {
			fmt.Fprintf(w, " since %s (%s)", at.Local().Format(stamp), ago(at))
		}
		fmt.Fprintln(w)
		if d.Detail != "" {
			fmt.Fprintf(w, "      %s\n", d.Detail)
		}

	case held:
		fmt.Fprint(w, "  there is no drift here now, so no tick is deciding anything about it.\n")
		fmt.Fprint(w, "  what earlier ticks decided, while there was:\n")

	default:
		fmt.Fprint(w, "  there is no drift here, so there is nothing for a tick to have decided\n")
		return
	}

	if len(s.Dispositions) == 0 {
		return
	}
	fmt.Fprint(w, "  history\n")
	for _, r := range s.Dispositions {
		fmt.Fprintf(w, "      %s  %-9s %-9s %s\n", r.At.Local().Format(stamp), r.Outcome, r.Condition, r.Detail)
	}
}

// short abbreviates a digest the way git abbreviates a sha. Records always
// carry the whole thing.
func short(digest string) string {
	if len(digest) < 20 {
		return digest
	}
	return digest[:20] + "…"
}
