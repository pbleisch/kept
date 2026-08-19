// Package register answers what is in force: which artifacts have standing, at
// which content version, and which do not and why.
//
// # It is derived, and that is the whole design
//
// The register is not a store. There is nothing in it to keep in step with
// anything, because it is computed from two inputs every time it is asked: the
// ledger's acts, and what each artifact says right now. An artifact is
// ratified when the ledger holds a ratification naming the hash its content
// currently produces — so editing the file changes the hash, the record stops
// matching, and standing stops applying with nothing having noticed the edit.
//
// **There is no invalidation step, and there is nowhere to add one.** Nothing
// in this package writes. A flag that something had to remember to clear would
// be a second place the answer lived, and the two could disagree; a cache that
// can disagree with its source is worse than none. See docs/decision/0001, and
// docs/decision/0022 for why a checkout on another platform does not silently
// revoke the entire corpus.
//
// # What it does not answer
//
// Whether a promise is kept. That is truth, it comes from observing the world,
// and nothing here looks. An artifact can be ratified and false at the same
// time — that is drift, and it needs a gatherer.
package register
