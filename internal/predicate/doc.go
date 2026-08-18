// Package predicate evaluates an artifact's hold_when against the
// observations it named.
//
// The language stays small: comparison, existence, set membership, a fold over
// a collection, and prior() for the previous observation. That last one is
// what makes "the set of tests never shrinks" expressible, which no boolean
// check can express at any level of sophistication.
//
// If any declared gatherer produced no observation, no predicate runs and the
// verdict is Unknown. See docs/decision/0004 and 0005.
package predicate
