// Package thread runs one repair: cut a box, assemble the packet, dispatch an
// agent, gather again, assess, present, land, gather again on the origin.
//
// The packet states the gap — statement, why, observation, scope, prior
// attempts — and not what to do about it. Enumerating repairs was the mistake
// the predecessors made; see docs/decision/0009.
//
// The assessor reads the ledger entry and cannot write. It answers a different
// question from the predicate: not "is this kept now" but "was this a
// legitimate way to get here". See docs/decision/0006.
package thread
