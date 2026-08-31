// Package assess judges one repair thread rather than one artifact.
//
// The predicate answers *is the promise kept now*. It does not answer *was this
// a legitimate way to get there*, and nothing that runs before this read a word
// the session wrote or compared the two measurements it was judged by.
//
// It has two halves, shaped differently on purpose. Neither is the other's
// fallback, and a finding says which produced it. See docs/decision/0078.
//
// **The derived checklist is closed.** It is docs/document/model.md's
// questions, in its order, minus the one that is a reading of prose, and
// [Assess] always answers one [Finding] per [Check] — including the ones that
// found nothing and the ones nothing could work. Each is worked out from
// evidence a repair already made durable: the packet's frozen authority, the two
// measurements, the anchor's own diff. Closed means a question is not added here
// without a decision, because each entry is a derivation somebody has to write,
// test and be able to break on purpose. A derivation earns its place by being
// reproducible: it runs in the suite on every commit, it costs nothing, and it
// can be broken deliberately and watched to fail — *a check that has never fired
// is not a check*.
//
// **The adversarial pass is not closed.** [Argument] is a model reading the
// whole ledger entry, the derived findings included, asked to make the case that
// the repair does not meet the thread's intent. Its findings are unbounded and
// unslotted, because the failure it exists to reach is the one nobody thought to
// ask about, and a checklist cannot reach that by construction. It is unbuilt,
// and this package reports that rather than leaving a gap where it will go.
//
// **Framed as an attack.** Every derived check looks for a way a thread can pass
// every gate and still be wrong, and fires on evidence of that rather than on
// the absence of evidence against it. The pass is asked to argue, never to
// confirm. A check — or a model — asked whether criteria are met finds that they
// are.
//
// **Verification attaches to acts.** A thread is bounded, has a ledger entry,
// and is where a change happened. Nothing here assesses a gatherer or a
// predicate in general, which is unbounded and never finishes; every finding is
// about this repair.
//
// A finding is three-valued for the same reason a verdict is. [Clear] is *this
// check was worked and found nothing*, [Fired] is *it found something*, and
// [Unread] is *the evidence it needs is not here* — which is not a pass, and
// reporting it as one would be the collapse the three-valued verdict exists to
// prevent one level down.
//
// Nothing in this half writes. The derived findings are worked out on read, so
// an assessment cannot go stale beside its source, and a promise revised after
// the fact does not change what a thread is assessed against. The pass's
// findings are the opposite case and will be durable: a session is not
// reproducible.
package assess
