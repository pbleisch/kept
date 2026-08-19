package schema

import "testing"

// The rule is frozen, because a section identifier lands in a binding preimage
// and changing it re-hashes every artifact of every sectioned kind at once.
// These cases are the rule, written out: if one of them has to change, that is
// a norm bump under docs/decision/0022 and never an edit. See 0040.
func TestASectionIdentifierIsTheFrozenRule(t *testing.T) {
	for _, tc := range []struct{ heading, want string }{
		{"Context", "context"},
		{"Decision", "decision"},
		{"What this does not decide", "what-this-does-not-decide"},

		// Case is folded down, and folded after NFC so that two spellings of
		// one accented heading agree across platforms.
		{"WHAT THIS DOES NOT DECIDE", "what-this-does-not-decide"},
		{"Café", "café"},
		{"Café", "café"},

		// Every run of non-alphanumerics becomes one dash, whatever it was
		// made of. No markdown is interpreted, so a code span's backticks are
		// punctuation like anything else.
		{"The `run:` line", "the-run-line"},
		{"Kael's eyes", "kael-s-eyes"},
		{"A  —  B", "a-b"},
		{"  Leading and trailing  ", "leading-and-trailing"},
		{"...Decision...", "decision"},
		{"0022 and 0031", "0022-and-0031"},

		// Nothing is transliterated: a letter is a letter.
		{"Контекст", "контекст"},

		// A heading with no letter or digit in it names nothing.
		{"---", ""},
		{"", ""},
	} {
		if got := SectionID(tc.heading); got != tc.want {
			t.Errorf("SectionID(%q) = %q, want %q", tc.heading, got, tc.want)
		}
	}
}

// A schema declares identifiers, never headings. One that needed normalizing
// before it could be compared would make body.Decision and body.decision two
// spellings of one binding field, and a hash list may have exactly one — so
// this is an error rather than a diagnostic, the way every schema fault is.
func TestASchemaDeclaringAHeadingRatherThanAnIdentifierIsRefused(t *testing.T) {
	if _, err := parse([]byte("type: decision\nbody: [Decision]\n")); err == nil {
		t.Fatal("a schema declaring a section as a heading was accepted")
	} else if !contains(err.Error(), "decision") {
		t.Errorf("the refusal does not say what to write instead: %v", err)
	}

	if _, err := parse([]byte("type: decision\nbody: [context, context]\n")); err == nil {
		t.Error("a schema declaring one section twice was accepted")
	}
	if _, err := parse([]byte("type: decision\nbody: [4]\n")); err == nil {
		t.Error("a schema declaring a section that is not a name was accepted")
	}
}

// The two shapes of body:, which are the two cases 0010 names.
func TestBodyIsAFieldNameOrAListOfSections(t *testing.T) {
	one, err := parse([]byte("type: promise\nbody: why\n"))
	if err != nil {
		t.Fatal(err)
	}
	if one.Body != "why" || len(one.Sections) != 0 {
		t.Errorf("a string body became %q with sections %v", one.Body, one.Sections)
	}

	many, err := parse([]byte("type: decision\nbody: [context, decision, consequences]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if many.Body != "" {
		t.Errorf("a sectioned body also named a prose field: %q", many.Body)
	}
	if len(many.Sections) != 3 || many.Sections[0] != "context" || many.Sections[2] != "consequences" {
		t.Errorf("sections are %v, and the schema's own order is what a reader should see", many.Sections)
	}
	if !many.DeclaresSection("decision") || many.DeclaresSection("alternatives") {
		t.Error("DeclaresSection does not answer for the list it was given")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
