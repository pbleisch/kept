// Package gather runs gatherers and returns structured observations.
//
// An observation is about the world and knows nothing about any artifact,
// which is why one serves many predicates, why observations are cached per
// tick by gatherer, scope and tree state, and why they survive a revision that
// resets an artifact's truth history.
//
// A gatherer never repairs what it finds: a gatherer that fixes things cannot
// be re-run to check whether the fix worked, and its output stops being
// evidence. See docs/decision/0004.
package gather
