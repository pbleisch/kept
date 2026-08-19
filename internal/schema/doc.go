// Package schema loads _schema/{type}.yaml and validates leniently: required
// fields are enforced, and a field no schema declares is reported and kept
// rather than rejected or dropped. It has to survive a read and be available to
// the caller, and no more than that — kept never rewrites a human-authored
// file, so nothing here re-emits one. See docs/decision/0029, which retired the
// round-trip requirement this package was first described with.
//
// It also answers the question the ledger depends on — which fields of a kind
// are binding, and therefore what the content hash covers. Reserved names are
// body, body.<section> and links.<relation>. See docs/decision/0003.
//
// A kind whose body is several prose blocks declares its H2 sections in body:,
// and section.go holds the rule that turns a heading into the name it declares.
// That rule is frozen: the name lands in a binding preimage, so changing it
// re-hashes every artifact of every sectioned kind. See docs/decision/0040.
//
// Leniency stops at the schemas themselves. An artifact that disagrees with its
// schema is reported; a schema that cannot be read is an error, because every
// artifact of that kind would otherwise be validated against nothing and the
// check would silently never fire.
package schema
