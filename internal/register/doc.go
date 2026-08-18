// Package register answers questions about artifacts that have standing: what
// is ratified, at which content version, and what is therefore in force.
//
// Standing is never read from a file. It is a ledger record naming a content
// hash, so an artifact edited outside the tool simply stops matching and stops
// obliging. See docs/decision/0001.
package register
