package assess

import "fmt"

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
// **This build declares no shape for a finding it makes.** Not an oversight: a
// record shape written before the thing that produces it is a guess about what
// it will say, and that freedom is the first thing such a guess would cost. The
// shape arrives with the pass, whose findings are durable — a session is not
// reproducible — and therefore receipt entries rather than a derivation.
//
// See docs/decision/0078.
type Argument struct {
	// Ran reports whether the pass was performed. It is false in this build,
	// which has no pass; it is a field rather than a constant because *nobody
	// argued this* and *the argument found nothing* are different answers and
	// a report that could not tell them apart would be the same collapse
	// [Unread] exists to prevent in the other half.
	Ran bool
	// Transcripts is how many transcript fragments the receipt holds for it to
	// read. It is what separates *nobody has argued this* from *there is
	// nothing here to argue over*.
	Transcripts int
}

// Detail is the one sentence a report prints about the pass.
func (a Argument) Detail() string {
	if a.Ran {
		return "the adversarial pass read this thread"
	}
	if a.Transcripts == 0 {
		return "no adversarial pass has read this thread, and it retained no transcript for one to read"
	}
	return fmt.Sprintf("no adversarial pass has read this thread; %s waiting for one",
		plural(a.Transcripts, "transcript fragment"))
}
