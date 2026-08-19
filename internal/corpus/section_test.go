package corpus

import (
	"path/filepath"
	"strings"
	"testing"
)

func sectionIDs(sections []Section) []string {
	out := make([]string, len(sections))
	for i, s := range sections {
		out[i] = s.ID
	}
	return out
}

// What opens a section and what does not. The strictness is deliberate: an
// indented ## is prose in a list or a code block, and a setext H2's marker is
// ---, which is also the frontmatter fence. Neither ambiguity belongs inside a
// binding hash. See docs/decision/0040.
func TestOnlyAnATXHeadingAtColumnZeroOpensASection(t *testing.T) {
	body := strings.Join([]string{
		"# 0040 — a title",
		"preamble, which belongs to no section",
		"## Context",
		"what led here",
		"### A subheading is not a section",
		"still context",
		"  ## Indented, so prose in a list",
		"##NoSpace",
		"Setext",
		"------",
		"## Decision ##",
		"the decision",
	}, "\n")

	got := sectionIDs(Sections(body))
	want := []string{"context", "decision"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("sections are %v, want %v", got, want)
	}
}

// A ## inside a fence is an example of a heading, not a heading. Without this a
// document that says what a section looks like would have that section, and for
// a sectioned kind that is binding content arriving out of a code block.
func TestAHeadingInsideAFenceIsNotASection(t *testing.T) {
	body := strings.Join([]string{
		"## Context",
		"here is what one looks like:",
		"```markdown",
		"## Decision",
		"~~~",
		"```",
		"~~~",
		"## Consequences",
		"~~~",
		"## Consequences",
		"the real one",
	}, "\n")

	got := sectionIDs(Sections(body))
	want := []string{"context", "consequences"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("sections are %v, want %v — a fenced heading opened one", got, want)
	}
}

// A section's prose runs to the next H2, and the blank lines at either end are
// not content. That last is 0022's argument at the one boundary 0022 does not
// reach: a corpus must not de-ratify because somebody added a blank line under
// a heading.
func TestASectionsProseRunsToTheNextHeadingAndIsTrimmed(t *testing.T) {
	spaced := Sections("## Context\n\n\nwhat led here\n\n\n## Decision\n\nthe decision\n")
	tight := Sections("## Context\nwhat led here\n## Decision\nthe decision")

	if len(spaced) != 2 || len(tight) != 2 {
		t.Fatalf("split into %d and %d sections", len(spaced), len(tight))
	}
	for i := range spaced {
		if spaced[i].Text != tight[i].Text {
			t.Errorf("%s: %q with blank lines, %q without", spaced[i].ID, spaced[i].Text, tight[i].Text)
		}
	}
	if spaced[0].Text != "what led here" {
		t.Errorf("context is %q", spaced[0].Text)
	}
	if spaced[0].Line != 1 || spaced[1].Line != 7 {
		t.Errorf("headings reported at lines %d and %d", spaced[0].Line, spaced[1].Line)
	}
}

// Text before the first H2 belongs to no section, and is not lost: the reserved
// name body still means the whole block.
func TestAPreambleBelongsToNoSection(t *testing.T) {
	sections := Sections("# A title\n\n**Status** accepted\n\n## Context\n\nhere")
	if len(sections) != 1 || sections[0].ID != "context" {
		t.Fatalf("sections are %v", sectionIDs(sections))
	}
	if strings.Contains(sections[0].Text, "Status") {
		t.Errorf("the preamble was swept into the first section: %q", sections[0].Text)
	}
}

// The guard the schema check exists for: a declared section the artifact does
// not have is reported, and an H2 nobody declared is not. Reporting the first is
// the half of a heading typo that says what to do about it; reporting the second
// would make every kind's schema enumerate every heading anyone might write.
func TestAMissingDeclaredSectionIsReportedAndAnUndeclaredOneIsNot(t *testing.T) {
	c := laidOut(t, map[string]string{
		"_schema/canon.yaml": "type: canon\nhash: [statement]\nfields:\n  statement: { type: string }\n" +
			"body: [context, decision]\n",
		"canon/typo.md": "---\nid: typo\nfields:\n  statement: s\n---\n\n" +
			"## Context\n\nhere\n\n## Desicion\n\nspelled wrong\n\n## Alternatives\n\nnobody declared this",
	})

	var missing, ambiguous, other int
	for _, p := range c.Problems {
		switch p.Kind {
		case MissingSection:
			missing++
			if !strings.Contains(p.Detail, "decision") {
				t.Errorf("the report does not name the section: %s", p.Detail)
			}
		case AmbiguousSection:
			ambiguous++
		default:
			other++
			t.Errorf("unexpected problem: %s", p)
		}
	}
	if missing != 1 {
		t.Errorf("%d missing-section problems, want exactly 1 — for decision, not for the two nobody declared", missing)
	}
	if ambiguous != 0 {
		t.Errorf("%d ambiguous-section problems, want 0", ambiguous)
	}
}

// Two headings on one identifier: reported, and the artifact still loads. What
// it cannot do is bind, which internal/canonical refuses.
func TestTwoHeadingsOnOneIdentifierAreReported(t *testing.T) {
	c := laidOut(t, map[string]string{
		"_schema/canon.yaml": "type: canon\nhash: [statement]\nfields:\n  statement: { type: string }\n" +
			"body: [decision]\n",
		"canon/twice.md": "---\nid: twice\nfields:\n  statement: s\n---\n\n" +
			"## Decision\n\nfirst\n\n## decision?\n\nsecond",
	})
	if _, ok := c.Get("canon:twice"); !ok {
		t.Fatal("an artifact with two headings on one identifier did not load; this is a diagnostic, not a load error")
	}

	var found bool
	for _, p := range c.Problems {
		if p.Kind == AmbiguousSection {
			found = true
			if !strings.Contains(p.Detail, "Decision") || !strings.Contains(p.Detail, "decision?") {
				t.Errorf("the report does not name both headings: %s", p.Detail)
			}
		}
	}
	if !found {
		t.Error("two headings naming one section were not reported, so one of them won silently")
	}
}

// laidOut lays a corpus out on disk and loads it, so a test can state the corpus
// it is about in one literal.
func laidOut(t *testing.T, files map[string]string) *Corpus {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		write(t, filepath.Join(root, filepath.FromSlash(name)), body)
	}
	c, err := Load(root, root, Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return c
}
