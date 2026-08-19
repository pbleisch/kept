// Package tick is the loop: gather what is due, evaluate, record, then decide
// what to do about anything with standing and without truth.
//
// No model runs here. Gathering, evaluating, ranking and the
// act/wait/suppress/escalate decision are deterministic, because a queue that
// cannot be explained is worse than a queue that is occasionally wrong.
//
// # Due, and what a not-due answer means
//
// A cadence is due when the bytes in its scope move, when its answer is older
// than max_age, or when nothing has ever answered for the wording it has now.
// That is docs/decision/0033, and it is what keeps a tick to one walk of the
// workspace rather than one gather per promise.
//
// It has a consequence that has to be said out loud, because the alternative
// reading is silently wrong. A verdict is recorded against the world it was
// reached in, and a world is the whole tree — so editing any file anywhere
// produces a world no verdict has ever been reached in. If the answer in force
// were only ever the verdict recorded against *this* world, every promise would
// read `unknown` after any edit, and the tick would have to re-gather
// everything on every keystroke, which is the cost 0033 exists to avoid.
//
// So the answer in force is the last verdict for this wording that is **not
// due** against the tree in front of us: nothing the promise looks at has
// moved, so the verdict still applies, and it is reported with the world it was
// reached in. The filter docs/decision/0035 requires is still the one doing the
// work — it is expressed as *the scope digests on that verdict's observations
// still match*, which is a stronger test than tree equality and the reason a
// carried answer is sound. A verdict reached in a box never enters the stream
// at all; [ledger.Ledger.Reached] refuses it.
//
// # The heartbeat is forced
//
// A truth record is appended when the verdict changes, or when a gather ran and
// the last record is older than max_age. There is no separate timer and no
// configured interval: an observation's timestamp means first-seen and never
// advances for an unchanging world (docs/decision/0034), so freshness has
// nowhere else to come from, and max_age is already the second half of *due*.
//
// # A tick that does nothing still records why
//
// Every drift the tick declines to act on leaves a disposition saying which
// condition refused, because none of those reasons can be reconstructed
// afterwards. See docs/decision/0019.
package tick
