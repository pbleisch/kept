package main

import (
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/corpus"
)

// Every artifact in this repository's corpus puts the fields its schema
// declares under fields:, where they are read and where they bind.
//
// It runs over the real corpus rather than a fixture for the reason
// [thisCorpus]'s own comment gives, and one more: the defect this guards
// against is not a shape a fixture author would think to write. Five decisions
// were authored on 2026-08-24 with `statement:` beside `id:` — 0062 through
// 0066, consecutively, while 0060, 0061, 0067 and 0068 nested it correctly — so
// it arrived as a run of ordinary-looking files nobody could tell apart by
// reading them. `kept list` called the corpus clean. The check in
// internal/corpus is what reports one; this is what notices the sixth.
//
// It sweeps every kind, not decisions: promise, policy and canon each declare
// fields that decide what is owed, and any of them can be written one level up.
func TestNoArtifactInThisCorpusWritesADeclaredFieldAtTheTopLevel(t *testing.T) {
	c := thisCorpus(t)

	var found []string
	for _, e := range c.All() {
		sc, ok := c.Schemas.For(string(e.Artifact.Kind))
		if !ok {
			t.Fatalf("%s has no schema, so nothing here declares anything for it to misplace", e.Artifact.ID)
		}
		for name := range e.Frontmatter {
			if corpus.Misplaced(name, sc) {
				found = append(found, e.Path+": "+name)
			}
		}
	}
	if len(found) > 0 {
		t.Errorf("%d field(s) are written at the top level, where nothing reads them and they do not bind:\n%s",
			len(found), strings.Join(found, "\n"))
	}

	// The sweep is only a sweep if it would find one, and a corpus that read as
	// empty or as schemaless would pass everything above in silence.
	sc, ok := c.Schemas.For("decision")
	if !ok {
		t.Fatal("this corpus declares no decision schema, so the sweep above asked nothing")
	}
	if !corpus.Misplaced("statement", sc) {
		t.Fatal("a top-level statement on a decision does not read as misplaced, so the sweep cannot find the defect it exists for")
	}
}
