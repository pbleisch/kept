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
package gather
