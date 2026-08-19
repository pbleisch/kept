// Package gather runs gatherers and returns structured observations.
//
// An observation is about the world and knows nothing about any artifact,
// which is why one serves many predicates, why observations are cached per tick
// by gatherer, args and world — docs/decision/0014 — and why they survive a
// revision that resets an artifact's truth history. What one is addressed by is
// docs/decision/0034 and the record lives in internal/ledger; what a gatherer
// may return, and the two bounds on how much of it, are here.
//
// A gatherer never repairs what it finds: a gatherer that fixes things cannot
// be re-run to check whether the fix worked, and its output stops being
// evidence. See docs/decision/0004.
//
// # The one rule
//
// [Gatherer.Gather] returning an error means **no observation**, which means
// the verdict is `unknown`. Not a false field, not an empty observation —
// nothing recorded. That is what makes a missing toolchain report "nobody could
// measure" rather than "the promise is broken", and it is why [Record] exists:
// the rule lives in one function rather than at every call site, because a site
// that forgot it would report not-kept for a machine that could not measure.
//
// # What is here
//
//   - [Gatherer], [Request] and [Fields] — the interface. A request carries the
//     workspace, the scope as authored, the paths it matched, its digest, the
//     args, the world and a deadline.
//   - [Catalogue] — the named map a promise writes against (docs/decision/0005),
//     and the seam a corpus checks its gather entries through.
//   - [Runner] — the per-tick observation cache. Two promises naming one
//     gatherer over one world share the work, because a gather runs a program.
//   - `command`, the escape hatch, and `tests`, the structural one.
//
// # Declaring fields
//
// A gatherer declares what it returns, and the declaration is checked against a
// promise's hold_when when the corpus loads. Built-ins declare in code;
// `command` alone declares in the promise, because it is the escape hatch and
// only it can name fields kept has never seen. That asymmetry is
// docs/decision/0034's answer rather than an inconsistency, and the check it
// enables is docs/decision/0036's: a promise reading a field nothing produces
// is an authoring error a person fixes, not an unknown discovered in the code
// path that decides a verdict.
//
// # What is not here
//
// **Redaction.** docs/decision/0023 designs four layers against a secret
// reaching a permanent record — declared fields, an explicit extractor, a size
// cap, and redaction — and 0034 ships the first three and defers the fourth
// deliberately. An authored extractor can capture a secret straight into a
// declared field, and the record is permanent and unrewritable. That is a known
// accepted cost, bounded by the two bounds here, and it is the first thing to
// revisit before this repository is shared. Nothing in this package echoes raw
// program output into an error, a message or a record for that reason: the
// bounded, redacted receipt 0023 designs is where it belongs, and it does not
// exist yet.
package gather
