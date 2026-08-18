// Package schema loads _schema/{type}.yaml and validates leniently: required
// fields are enforced, unknown fields are allowed and must survive a
// parse-write round trip untouched.
//
// It also answers the question the ledger depends on — which fields of a kind
// are binding, and therefore what the content hash covers. Reserved names are
// body, body.<section> and links.<relation>. See docs/decision/0003.
package schema
