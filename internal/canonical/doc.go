// Package canonical turns an artifact's binding content into the bytes a
// ratification is keyed to.
//
// Standing is not stored anywhere near the artifact. It is a ledger record
// naming a content hash, so an artifact edited outside the tool simply stops
// matching the record and stops obliging anyone. Nothing invalidates anything;
// the comparison is the whole mechanism. That only works if the hash is stable
// against every difference that is not a change of content, which is what this
// package is for. See docs/decision/0001 and 0022.
//
// # Two rules, from two decisions
//
// [Schema.Hash] names which fields bind — decision 0003 — so reformatting the
// prose of a promise does not drop its standing while changing its predicate
// does. And normalization happens inside the hash rather than only at rest —
// decision 0022 — so a checkout with core.autocrlf=true does not silently
// de-ratify an entire corpus. The file keeps its literal bytes; only the hash
// sees the normalized form.
//
// # Versioned, because the rules will be refined
//
// Every hash carries [Version], recorded on the ledger record as norm. A later
// refinement re-hashes old records under the version they were written with, so
// changing the rules is not a bulk de-ratification. That is what keeps 0022
// from being a one-way door, and it is why nothing here may change without
// [Version] changing with it.
//
// # What it does not know
//
// It does not know what standing is, what the ledger holds, or whether anything
// has ever been ratified. It answers one question — what does this artifact
// currently say, in bytes that two machines agree on — and the register asks it.
package canonical
