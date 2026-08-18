// Package index is the derived SQLite and FTS5 store: search, the link graph,
// and the drift query.
//
// It is authoritative for nothing and is rebuildable from the corpus and the
// ledger. If it disagrees with them, it is wrong.
package index
