package assess

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/thread"
)

// Request is one thread a person asked about, by name.
//
// One thread, because verification attaches to acts. A thread is bounded, has a
// ledger entry, and is where a change happened; a request to assess a gatherer
// or a promise in general is a request that never finishes.
type Request struct {
	// Store is where the receipt, the packet and the measurements are read
	// from. Nothing here writes to it.
	Store ledger.Store
	// Repository is the checkout the anchor's diff is read out of, and empty is
	// a caller with none — a report over a ledger copied elsewhere, which can
	// still work every check that does not need the diff.
	Repository string
	// Corpus is the repository as it stands now, for the one check that asks
	// what else reads these paths. Nil is a caller with none.
	Corpus *corpus.Corpus
	// Thread is the thread to assess.
	Thread thread.ID
	// Deadline bounds the one external call this makes. Zero is
	// [defaultDeadline] and never unbounded. See docs/decision/0026.
	Deadline time.Duration
}

// Assessment is one thread, judged.
//
// There is no verdict field and there will not be one. Which check fired is the
// whole diagnostic value, so the findings are the answer; a boolean over them
// would be a summary that a reader trusts and a reviewer cannot act on. What is
// here beside them is the gate the promise's own instruments set, which is what
// says how much of a person this thread needs — and it is derived from the
// promise rather than from the findings, because autonomy is bounded by the
// instrument and not by how a repair went.
type Assessment struct {
	Thread  thread.ID
	Promise string
	Entry   Entry
	// Findings is one per derived check, in checklist order, always all of
	// them. It is half the assessor and never the whole of it.
	Findings []Finding
	// Argument is the adversarial pass. It is a field beside the findings
	// rather than one of them, because its findings are unbounded and a slot is
	// a ceiling. See docs/decision/0078.
	Argument Argument
	// Grading is what the promise's weakest instrument requires.
	Grading Grading
}

// Assess works the derived checklist over one thread's ledger entry, and reports
// the state of the adversarial pass beside it.
//
// It reads. Nothing here gathers, measures, dispatches or records: the evidence
// is what a repair already made durable, and a derived finding is worked out on
// read the way standing and truth are. That is what keeps this half from being a
// second record that can disagree with the one it came from, and what lets it
// run against a checkout nobody may write to.
//
// It dispatches nothing, and that stays true now that the adversarial pass
// exists: the pass is a confined session with its own ask and its own read-only
// box, it runs inside the repair episode, and what it wrote is durable — so what
// this reads is a record rather than something it starts. A verb a person types
// that could spend a credential would be a reader that gates, which is the split
// docs/decision/0079 keeps. See also 0078 and 0081.
func Assess(ctx context.Context, req Request) (Assessment, error) {
	if req.Store == nil {
		return Assessment{}, fmt.Errorf("an assessment has no ledger to read")
	}
	if req.Thread == "" {
		return Assessment{}, fmt.Errorf("an assessment names no thread")
	}
	entry, err := Read(ctx, req.Store, req.Repository, req.Thread, req.Deadline)
	if err != nil {
		return Assessment{}, err
	}
	s := subject{entry: entry, corpus: req.Corpus}
	return Assessment{
		Thread:   req.Thread,
		Promise:  entry.Packet.Promise.ID,
		Entry:    entry,
		Findings: s.findings(),
		Argument: argument(entry),
		Grading:  Grade(entry.Packet.Promise.Gather),
	}, nil
}

// Fired is every check that found something, in checklist order.
func (a Assessment) Fired() []Finding { return a.only(Fired) }

// Worked reports whether any derived check was worked at all.
//
// False is *nothing has answered*, which is a different thing from a clean
// checklist and is what separates an assessment of a thread whose evidence is
// gone from one that found nothing wrong. A caller that cannot tell them apart
// reports a pruned ledger as a clean repair.
//
// It says nothing about the adversarial pass, which has its own answer to that
// question in [Argument.Ran] — a thread whose checklist nothing could work may
// still have been argued over, and the reverse is the ordinary case.
func (a Assessment) Worked() bool { return len(a.only(Clear))+len(a.only(Fired)) > 0 }

func (a Assessment) only(outcome Outcome) []Finding {
	var out []Finding
	for _, finding := range a.Findings {
		if finding.Outcome == outcome {
			out = append(out, finding)
		}
	}
	return out
}

// String is the report a person reads.
//
// A fired check is marked, a clear one is not, and one nothing could work says
// so in its own words rather than being left out — the row that is missing is
// the row nobody goes looking for.
//
// The adversarial pass gets a line of its own beneath them rather than a row
// among them. A row would put it in the checklist, and it is the half that is
// not one.
func (a Assessment) String() string {
	var out strings.Builder
	for _, finding := range a.Findings {
		fmt.Fprintf(&out, "%s  %s  %s\n", mark(finding.Outcome), finding.Check, finding.Detail)
		for _, line := range finding.Lines {
			fmt.Fprintf(&out, "      %s\n", line)
		}
	}
	fmt.Fprintf(&out, "\n%s  %s\n", argued(a.Argument), a.Argument.Detail())
	// And what it said, whole. Not a summary and not a count: the value of this
	// half is what nobody enumerated, so a report that condensed it would be
	// throwing away the thing it was brought in for.
	for _, line := range a.Argument.Lines() {
		fmt.Fprintf(&out, "      %s\n", line)
	}
	// The one line kept read out of the words, said beneath them rather than in
	// place of them. See docs/decision/0086.
	if ruling := a.Argument.RulingLine(); ruling != "" {
		fmt.Fprintf(&out, "\n    %s\n", ruling)
	}
	fmt.Fprintf(&out, "\n%s\n", a.Grading)
	return out.String()
}

// argued is how the pass's line reads down the same left margin as the checks,
// and it is deliberately **not** one of their three marks.
//
// A pass that has not run is the same *nobody has answered* an unread check
// reports, so it borrows that one. A pass that has run gets a mark of its own,
// because the checklist's other two would both be lies: nothing here graded what
// it argued, so kept cannot say it found something, and saying it found nothing
// would report a devastating case as a clean row. The mark points at the text
// beneath it, which is the only honest thing to say — read it.
func argued(argument Argument) string {
	if argument.Ran {
		return ">>"
	}
	return mark(Unread)
}

// mark is how each outcome reads down the left of the report. Two characters,
// so the checks line up whatever they answered, and a clear row is blank rather
// than ticked: what a reader is looking for is what fired.
func mark(outcome Outcome) string {
	switch outcome {
	case Fired:
		return "!!"
	case Unread:
		return "??"
	}
	return " ·"
}
