// Package corpus is the wiki: markdown entities with YAML frontmatter, one
// file per artifact at {type}/{slug}.md, plus links and attachments.
//
// It owns the single write path — validate, write, index, commit — because the
// tick, a thread and the operator all write, and a write that skips it leaves
// the index disagreeing with git. That facade is an implementation detail, not
// an endpoint.
//
// It does not know what standing or truth are. See docs/decision/0002.
package corpus
