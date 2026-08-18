// Package corpus is the wiki: markdown entities with YAML frontmatter, one
// file per artifact at {type}/{slug}.md, plus links and attachments.
//
// It owns the single write path — validate, write, index, commit — because the
// tick, a thread and the operator all write, and a write that skips it leaves
// the index disagreeing with git. That facade is an implementation detail, not
// an endpoint. Nothing writes yet; this package reads.
//
// # Identity
//
// An artifact's identity is the slug authored in its frontmatter. The filename
// is not identity, so renaming a file changes nothing — a property this package
// gets structurally rather than by care: model.Artifact has nowhere to put a
// filename, and the path lives beside it on the Entry. Two artifacts claiming
// one id is the one thing that stops a corpus loading, and the error names
// every path so that whoever has to fix it does not have to go looking. See
// docs/decision/0021.
//
// # What is an error, and what is reported
//
// The line is identity, not severity. A load error is one that would make the
// map from id to artifact wrong: an unreadable root, a schema nothing could
// parse, two artifacts claiming one id. Everything else — a file with no
// frontmatter, a required field nobody wrote, a link pointing at nothing — is a
// Problem, and the corpus loads with it. A reader who has just been told about
// a broken link is better served than one who has been told nothing.
//
// # Leniency
//
// A field no schema declares survives the read and is available to the caller,
// on Artifact.Fields and Entry.Frontmatter. It is never rejected and never
// silently dropped. It is also never written back, because kept does not
// rewrite a human-authored file — which is why a read is all it has to survive.
// See docs/decision/0002 and 0029.
//
// # The YAML seam
//
// This package and internal/schema are the only two that decode YAML, and no
// type from the decoder appears in a signature either of them exports. That is
// the seam decision 0029 admitted the dependency behind, and a decoder type
// reaching past it is a review finding rather than a matter of taste.
//
// It does not know what standing or truth are. See docs/decision/0002.
package corpus
