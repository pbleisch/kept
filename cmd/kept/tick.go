package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/pbleisch/kept/internal/config"
	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/gather"
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/register"
	"github.com/pbleisch/kept/internal/run"
	"github.com/pbleisch/kept/internal/tick"
)

// tickCmd is what `kept` with no verb does: gather what is due, evaluate it,
// record what was found, and print what is kept, what is not, and what nobody
// could measure.
//
// It is right because it just looked. Nothing here reads a stored verdict and
// calls it an answer: an artifact whose scope has moved is re-derived before it
// is reported, and one whose scope has not is reported with the world its
// answer was reached in.
//
// There is no scheduler and no daemon. It ticks once and exits, the way `make`
// does, and driving it on a timer is cron's job rather than kept's. See
// docs/decision/0025.
func tickCmd(args []string, stdout, stderr io.Writer) int {
	fs, root := flags("kept", stderr)
	all := fs.Bool("all", false, "gather every artifact, whatever its scope says")
	asJSON := fs.Bool("json", false, "the structured document, per decision 0027")
	rest, err := parse(fs, args)
	if err != nil {
		return exitUsage
	}
	if len(rest) > 0 {
		fmt.Fprintf(stderr, "kept: %q is not a verb; kept with no verb is the tick\n", rest[0])
		return exitUsage
	}

	c, code := open(*root, stderr)
	if code != exitOK {
		return code
	}
	t, l, code := newTick(c, *all, append([]string{"tick"}, args...), stderr)
	if code != exitOK {
		return code
	}

	res, err := t.Run(context.Background())
	if err != nil {
		_ = l.Close(run.Failed)
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return exitUsage
	}
	if err := l.Close(run.Clean); err != nil {
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return exitUsage
	}

	writeQuarantined(stderr, res.Quarantined)
	code = answer(res)
	if *asJSON {
		return writeDocument(stdout, stderr, newDocument("tick", c, res, code, false))
	}
	writeVerdicts(stdout, c, res)
	return code
}

// newTick assembles a pass from the corpus and the operator's config.
//
// The config's three tick-facing keys are read here rather than defaulted:
// a gatherer deadline nobody honoured, an exclusion nobody applied and a
// confinement provider nobody read would each be a knob that silently does
// nothing. See docs/decision/0026, 0033 and 0017.
func newTick(c *corpus.Corpus, all bool, argv []string, stderr io.Writer) (*tick.Tick, *ledger.Ledger, int) {
	cfg, err := config.Read(filepath.Join(c.Workspace, corpus.ConfigDir))
	if err != nil {
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return nil, nil, exitUsage
	}

	store := ledgerStore(c)
	l := ledger.Open(store, startRun(argv))
	return &tick.Tick{
		Corpus:    c,
		Store:     store,
		Ledger:    l,
		Catalogue: gather.Builtin(),
		Options: tick.Options{
			All:      all,
			Deadline: cfg.Gatherer,
			Provider: cfg.Provider,
			Exclude:  cfg.Exclude,
			// The cold walk parallelises, which is one of 0033's four
			// accelerations and the one that matters on a large repository. It
			// must agree with the plain walk, and internal/workspace has the
			// test that it does.
			Workers: runtime.NumCPU(),
		},
	}, l, exitOK
}

// survey is the read-only half: what `kept drift` and `kept why` run. It opens
// no ledger for writing, so both answer against a checkout nothing may write to.
func survey(c *corpus.Corpus, stderr io.Writer) (*tick.Result, int) {
	t, _, code := newTick(c, false, nil, stderr)
	if code != exitOK {
		return nil, code
	}
	t.Ledger = nil

	res, err := t.Survey()
	if err != nil {
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return nil, exitUsage
	}
	writeQuarantined(stderr, res.Quarantined)
	return res, exitOK
}

// answer is a pass as an exit code, which is the contract in
// docs/decision/0027.
//
// 1 is drift proper: something with standing is not kept. 3 is the code worth
// arguing for and the reason the verdict is three-valued at all — a CI job that
// treats "the toolchain was missing" as "the promise is broken" produces
// exactly the false report the model exists to prevent, so an unknown, a
// promise nothing has ever observed and an answer about a tree this is not are
// all "could not determine".
//
// 4 stays unreachable here, and deliberately. Nothing in this build refuses to
// *answer*: the refusals it records are refusals to *act* on drift it has
// already reported, and reporting drift is exit 1.
func answer(res *tick.Result) int {
	code := exitOK
	if len(res.Quarantined) > 0 {
		code = exitUnknown
	}
	for _, s := range res.Subjects {
		if !s.Drifting {
			continue
		}
		switch s.Drift.Kind {
		case model.DriftNotKept, model.DriftRegressed, model.DriftAbsent, model.DriftConflicted:
			return exitDrift
		default:
			code = exitUnknown
		}
	}
	return code
}

// writeVerdicts prints the tick: one line per artifact that seeks truth, with
// what could not be told in one line indented beneath it.
func writeVerdicts(w io.Writer, c *corpus.Corpus, res *tick.Result) {
	var shown int
	for _, s := range res.Subjects {
		if !s.Predicated() && !s.Drifting {
			continue
		}
		shown++
		fmt.Fprintf(w, "  %s\n", strings.TrimRight(fmt.Sprintf("%-38s %-10s %s", s.ID, verdictOf(s), provenance(s)), " "))
		for _, line := range detailLines(s) {
			fmt.Fprintf(w, "      %s\n", line)
		}
	}

	if shown == 0 {
		fmt.Fprint(w, "no artifact here names a gatherer, so there is nothing to observe\n")
	} else {
		writeTickSummary(w, res, shown)
	}
	if n := len(c.Problems); n > 0 {
		fmt.Fprintf(w, "\n%s reading the corpus; kept list says what they are\n", count(n, "problem"))
	}
}

// verdictOf is the three-valued answer, or the reason there is not one. Never a
// boolean, and never a fourth value dressed as one: "unverified" and "stale"
// say that nothing has answered, which is not the same as an answer of unknown.
func verdictOf(s tick.Subject) string {
	switch {
	case s.Standing.State == register.Absent:
		return "absent"
	case s.Unevaluated != "":
		return "unevaluated"
	case s.Answer.Reached() && s.Due && !s.Answer.Gathered:
		return "stale"
	case s.Answer.Reached():
		return string(s.Answer.Value)
	default:
		return "unverified"
	}
}

// provenance is what answered and when — the half of a verdict a reader checks
// when the verdict itself is not surprising.
func provenance(s tick.Subject) string {
	if !s.Answer.Reached() {
		return ""
	}
	var out string
	if names := gatherers(s); names != "" {
		out = "by " + names
	}
	if !s.Answer.Since.IsZero() {
		out = strings.TrimSpace(out + " since " + s.Answer.Since.Local().Format(stamp))
	}
	if !s.Answer.Fresh {
		out += " · in another tree"
	}
	return out
}

func gatherers(s tick.Subject) string {
	if s.Entry == nil {
		return ""
	}
	var names []string
	for _, spec := range s.Entry.Artifact.Gather {
		names = append(names, spec.Gatherer)
	}
	sort.Strings(names)
	return strings.Join(names, " · ")
}

// detailLines is what the verdict could not say in a word: the field nobody
// could answer for, why the answer is being re-derived, and what the tick did
// about the drift.
func detailLines(s tick.Subject) []string {
	var out []string
	if s.Unevaluated != "" {
		out = append(out, firstLine(s.Unevaluated))
	}
	if s.Answer.Detail != "" {
		out = append(out, s.Answer.Detail)
	}
	if !s.Answer.Reached() && s.Because != "" {
		out = append(out, s.Because)
	}
	if s.Drifting && s.Disposition.Outcome != "" {
		out = append(out, fmt.Sprintf("%s · %s — %s", s.Disposition.Outcome, s.Disposition.Condition, s.Disposition.Detail))
	}
	return out
}

func writeTickSummary(w io.Writer, res *tick.Result, shown int) {
	counts := map[string]int{}
	var drifting int
	for _, s := range res.Subjects {
		if !s.Predicated() && !s.Drifting {
			continue
		}
		counts[verdictOf(s)]++
		if s.Drifting {
			drifting++
		}
	}

	states := make([]string, 0, len(counts))
	for state, n := range counts {
		states = append(states, fmt.Sprintf("%d %s", n, state))
	}
	sort.Strings(states)

	fmt.Fprintf(w, "\n%s", count(shown, "artifact"))
	for _, s := range states {
		fmt.Fprintf(w, " · %s", s)
	}
	fmt.Fprintln(w)

	if res.Observed > 0 {
		fmt.Fprintf(w, "%s recorded\n", count(res.Observed, "observation"))
	}
	if drifting > 0 {
		fmt.Fprintf(w, "%s with standing and without truth; kept why <artifact> says what the tick did about it\n",
			count(drifting, "artifact"))
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// ago renders how long something has stood, which is what "not repaired for six
// days" is asking for.
func ago(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	d := time.Since(at)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d minutes ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d hours ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%d days ago", int(d.Hours()/24))
	}
}
