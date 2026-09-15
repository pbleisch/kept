package assess

import (
	"fmt"
	"strings"

	"github.com/pbleisch/kept/internal/thread"
)

// Argument is the state of the adversarial pass over one thread.
//
// The pass is a model reading the whole ledger entry — the packet, the anchor's
// diff, both measurements, the transcript, and the derived findings — asked to
// make the case that the repair does not meet the thread's intent. It is the
// assessor's other half, beside [Checklist] and never inside it.
//
// **Its findings are unbounded and unslotted**, and that is the point rather
// than an omission. The failure it exists to reach is the one nobody thought to
// ask about — a checklist cannot reach that by construction, because somebody
// had to write the question — so there is no fixed question set here, no
// enumeration of what it may look for, and no mapping from what it says back to
// a checklist entry. Every mechanism that would make its output tidier is a
// mechanism that narrows it. When a finding it produces turns out to be
// derivable, the answer is to write the derivation and add it to the closed
// half; it is never to add the question to this one.
//
// **What it said is the whole of what is recorded.** The pass is built now and
// this type still declares no shape for a finding it makes: [Said] is its own
// words, unparsed, and there is no severity, no category, no count and no
// verdict beside them. A record shape written over its output would be the
// guess docs/decision/0078 says costs exactly the freedom the half is for, and
// what a gate makes of the words is the dispatch policy of open question 3
// rather than a field somebody wrote here.
//
// See docs/decision/0078 and 0081.
type Argument struct {
	// Ran reports whether the pass was performed. It is a field rather than a
	// derivation from the words, because *nobody argued this* and *the argument
	// found nothing* are different answers and a report that could not tell them
	// apart would be the same collapse [Unread] exists to prevent in the other
	// half.
	Ran bool
	// Transcripts is how many transcript fragments the receipt holds for it to
	// read. It is what separates *nobody has argued this* from *there is
	// nothing here to argue over*.
	Transcripts int
	// Runtime and Model are what answered, and they come from two places on
	// purpose: the runtime is the parent's own fact about the session it
	// started, and the model is the runtime's own report of what served the
	// request. A session given a model it did not honour says so. See
	// docs/decision/0077.
	Runtime string
	Model   string
	// Said is what the last pass argued, verbatim. Empty is a pass that has not
	// run, or one whose words the store no longer holds.
	Said string
	// Passes is how many adversarial passes argued over the attempt this reads.
	// More than one is a pass run again over the same work.
	Passes int

	// Ruling is the ruling read from the last line of what the pass said, and
	// RulingWhy says why there is none when it is empty. It is derived here, on
	// read, from the words, rather than recorded beside them: it is a pure
	// function of a durable record, and a stored copy is the cache that can
	// disagree with its source. It is the one thing read out of the words, and
	// it is not a grade — see [thread.Ruling] and docs/decision/0086.
	Ruling    thread.Ruling
	RulingWhy string
}

// Detail is the one sentence a report prints about the pass.
func (a Argument) Detail() string {
	if a.Ran {
		who := a.Runtime
		if a.Model != "" {
			who += ", " + a.Model
		}
		if a.Passes > 1 {
			return fmt.Sprintf("%s adversarial passes have read this thread; the last was %s", plural(a.Passes, "pass"), who)
		}
		return "an adversarial pass read this thread: " + who
	}
	if a.Transcripts == 0 {
		return "no adversarial pass has read this thread, and it retained no transcript for one to read"
	}
	return fmt.Sprintf("no adversarial pass has read this thread; %s waiting for one",
		plural(a.Transcripts, "transcript fragment"))
}

// Lines is what the pass argued, as a report prints it beneath its own line.
//
// It is the words and nothing else: no heading a reader could mistake for a
// finding count, and no summary, because summarizing an argument whose value is
// what nobody enumerated is the narrowing this half exists to avoid. A blank
// line inside it is kept, because paragraphs are how an argument reads.
func (a Argument) Lines() []string {
	if strings.TrimSpace(a.Said) == "" {
		return nil
	}
	return strings.Split(strings.TrimRight(a.Said, "\n"), "\n")
}

// argument is the state of the pass over one entry, read from the receipt.
//
// A pass is durable and derived from nothing: it ran or it did not, and the
// receipt is the only thing that says so. That is the opposite of the closed
// half beside it, and it is why this reads a record where [subject.findings]
// works one out.
func argument(entry Entry) Argument {
	out := Argument{Transcripts: entry.Transcripts, Passes: len(entry.Arguments)}
	if len(entry.Arguments) == 0 {
		return out
	}
	last := entry.Arguments[len(entry.Arguments)-1]
	out.Ran = true
	out.Runtime = string(last.Runtime)
	out.Said = entry.Argued
	out.Model = entry.ArguedModel
	out.Ruling, out.RulingWhy = thread.ReadRuling(entry.Argued)
	return out
}

// RulingLine is the one line a report prints about the ruling, beneath what the
// pass said. Empty for a pass that has not run, which already says so.
func (a Argument) RulingLine() string {
	switch {
	case !a.Ran:
		return ""
	case a.Ruling.Known():
		return "ruling: " + string(a.Ruling)
	}
	return "no ruling: " + a.RulingWhy
}
