package main

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/pbleisch/kept/internal/schema"
)

// want captures one and returns the path it wrote, read off the verb's own
// output rather than assumed.
func want(t *testing.T, w *workspace, text string) string {
	t.Helper()
	args := []string{"want", text}
	if strings.HasPrefix(text, "-") {
		// A want that opens with a dash reads as a flag, so it is given after
		// the flag parser's own terminator, as it would be at a terminal.
		args = []string{"want", "--", text}
	}
	code, stdout, stderr := w.run(args...)
	if code != exitOK {
		t.Fatalf("want %q exited %d: %s", text, code, stderr)
	}
	rel, _, ok := strings.Cut(strings.TrimPrefix(stdout, "wrote "), "\n")
	if !ok {
		t.Fatalf("want did not say what it wrote:\n%s", stdout)
	}
	return "docs/" + rel
}

// The capture is an id and a want, under fields:, and nothing else: no title, no
// guidance, no body, and nothing anywhere but the one file. A new desire is
// raw, and it is read back with nothing to say about it.
func TestWantWritesAnIDAndAWantAndNothingElse(t *testing.T) {
	w := newCorpus(t)
	before := tree(t, w.root)

	path := want(t, w, "repair should pick which drift to spend a session on")

	if path != "docs/desire/repair-should-pick-which-drift-to-spend-a-session-on.md" {
		t.Fatalf("wrote %s", path)
	}
	wantBytes := "---\nid: repair-should-pick-which-drift-to-spend-a-session-on\nfields:\n" +
		"  want: repair should pick which drift to spend a session on\n---\n"
	if got := w.read(path); got != wantBytes {
		t.Fatalf("the file is:\n%s\nwant:\n%s", got, wantBytes)
	}

	after := tree(t, w.root)
	for p := range after {
		if _, existed := before[p]; !existed && p != path {
			t.Errorf("kept want created %s", p)
		}
	}
	for p, was := range before {
		if after[p] != was {
			t.Errorf("kept want changed %s", p)
		}
	}

	c := load(t, w)
	for _, p := range c.Problems {
		t.Errorf("a captured want was read with a problem: %s: %s", p.Kind, p.Detail)
	}
	mustContain(t, "status", w.line("desire:repair-should-pick-which-drift-to-spend-a-session-on"), "raw")
}

// **What a deterministic slug means**, one rule at a time: it is SectionID over
// the text, so case, punctuation, whitespace and Unicode form do not move it,
// and nothing is transliterated or dropped.
func TestTheSlugIsAFunctionOfTheTextAlone(t *testing.T) {
	cases := map[string]string{
		"repair should pick which drift":           "repair-should-pick-which-drift",
		"Repair SHOULD pick — which drift?!":       "repair-should-pick-which-drift",
		"  repair\u00a0should   pick which drift ": "repair-should-pick-which-drift",
		"the `run:` line takes no shell":           "the-run-line-takes-no-shell",
		"Kael's eyes":                              "kael-s-eyes",
		"café au lait":                             "café-au-lait",
		"cafe\u0301 au lait":                       "café-au-lait",
		"0056 is the longest":                      "0056-is-the-longest",
		"日本語の願い":                                   "日本語の願い",
	}
	for text, slug := range cases {
		if got := wantSlug(text); got != slug {
			t.Errorf("wantSlug(%q) = %q, want %q", text, got, slug)
		}
		if got := wantSlug(text); got != wantSlug(text) {
			t.Errorf("wantSlug(%q) is not stable: %q", text, got)
		}
	}

	// And it is the frozen rule rather than a copy of it: whatever SectionID
	// produces for a want that fits the bound is the slug.
	for text := range cases {
		if s := schema.SectionID(text); len(s) <= slugBound && wantSlug(text) != s {
			t.Errorf("wantSlug(%q) = %q and SectionID says %q", text, wantSlug(text), s)
		}
	}
}

// The bound is sixty-four bytes, cut at the last hyphen that fits, so the slug
// ends on a whole word; a single word longer than the bound is cut at the last
// whole character, never inside one.
func TestTheSlugIsBoundedAtTheLastWordThatFits(t *testing.T) {
	long := "a tick that could act on several drifts records which it ranked first and why it ranked it"
	slug := wantSlug(long)
	full := schema.SectionID(long)
	if len(slug) > slugBound || !strings.HasPrefix(full, slug) || strings.HasSuffix(slug, "-") {
		t.Fatalf("wantSlug = %q (%d bytes)", slug, len(slug))
	}
	if next := full[len(slug)]; next != '-' {
		t.Fatalf("the slug %q was cut inside a word of %q", slug, full)
	}
	if slug != "a-tick-that-could-act-on-several-drifts-records-which-it-ranked" {
		t.Fatalf("wantSlug = %q", slug)
	}

	// A hyphen exactly at the bound is a word that fits exactly.
	exact := strings.Repeat("a", slugBound) + " b"
	if got := wantSlug(exact); got != strings.Repeat("a", slugBound) {
		t.Fatalf("a word exactly at the bound was cut: %q", got)
	}

	// One word longer than the bound, in characters that are three bytes each.
	word := strings.Repeat("語", 40)
	got := wantSlug(word)
	if len(got) > slugBound || !utf8.ValidString(got) || !strings.HasPrefix(word, got) || len(got) < slugBound-2 {
		t.Fatalf("a long word was cut to %q (%d bytes, valid %v)", got, len(got), utf8.ValidString(got))
	}
}

// **A collision is refused, never numbered.** Two wants whose words make one
// slug are the same want spelled twice, and the second is refused naming the
// file that holds the first — which is how somebody re-proposing a declined
// want finds the decline. Nothing is written, and the first file is untouched.
func TestAWantWhoseSlugIsTakenIsRefusedNamingTheFile(t *testing.T) {
	w := newCorpus(t)
	path := want(t, w, "repair should pick which drift to spend a session on")
	first := w.read(path)
	before := tree(t, w.root)

	code, stdout, stderr := w.run("want", "Repair should pick which drift to spend a session on!")
	if code != exitUsage {
		t.Fatalf("a second want with the same slug exited %d: %s%s", code, stdout, stderr)
	}
	mustContain(t, "the refusal", stderr,
		"desire:repair-should-pick-which-drift-to-spend-a-session-on is already claimed by",
		"desire/repair-should-pick-which-drift-to-spend-a-session-on.md",
		"nothing was written",
		"kept why desire:repair-should-pick-which-drift-to-spend-a-session-on")
	if w.read(path) != first {
		t.Fatal("the first want's file was rewritten")
	}
	if after := tree(t, w.root); len(after) != len(before) {
		t.Fatalf("a refused want wrote something: %d files, was %d", len(after), len(before))
	}

	// Two long wants that agree for the whole bound collide too, and the bound
	// is what decides it.
	long := "a tick that could act on several drifts records which it ranked first"
	want(t, w, long)
	if code, _, _ := w.run("want", long+" and why it ranked it there"); code != exitUsage {
		t.Fatalf("two wants agreeing for sixty-four bytes did not collide: exit %d", code)
	}
}

// The slug is claimed by an id, and the path by a file, and they can disagree: a
// file already at the path that is not the artifact with that id is left
// exactly as it was.
func TestAFileAlreadyAtTheWantsPathIsLeftAsItWas(t *testing.T) {
	w := newCorpus(t)
	w.put("docs/desire/make-the-tick-faster.md", "---\nid: something-else\nfields:\n  want: a person's own words\n---\n")

	code, _, stderr := w.run("want", "make the tick faster")
	if code != exitUsage {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	mustContain(t, "the refusal", stderr, "make-the-tick-faster.md", "already there")
	if got := w.read("docs/desire/make-the-tick-faster.md"); !strings.Contains(got, "a person's own words") {
		t.Fatalf("the file was rewritten:\n%s", got)
	}
}

// What a person typed is what the reader reads back, byte for byte, including
// the texts YAML would otherwise read as something else.
func TestAWantIsReadBackExactlyAsItWasTyped(t *testing.T) {
	texts := []string{
		"key: value is not a map",
		"- not a list item",
		"# not a comment",
		"true",
		"123",
		"1e3",
		"null",
		"a want with a # in it",
		`say "why" and mean it`,
		`a back\slash`,
		"'single quoted'",
		"ends with a colon:",
		"[not a flow sequence]",
		"{not a flow map}",
		"% not a directive",
		"@ not reserved",
		"café — naïve, 日本語",
	}
	w := newCorpus(t)
	byPath := map[string]string{}
	for _, text := range texts {
		byPath[want(t, w, text)] = text
	}

	c := load(t, w)
	for _, p := range c.Problems {
		t.Errorf("%s: %s: %s", p.Path, p.Kind, p.Detail)
	}
	for _, e := range c.All() {
		text, ok := byPath["docs/"+e.Path]
		if !ok {
			continue
		}
		if got := e.Artifact.Fields["want"]; got != text {
			t.Errorf("wrote %q and read back %#v", text, got)
		}
		delete(byPath, "docs/"+e.Path)
	}
	for path := range byPath {
		t.Errorf("%s was written and is not an artifact", path)
	}
}

// What cannot be a want is refused before anything is read or written, and each
// refusal says what to do instead.
func TestWhatCannotBeAWantIsRefusedAndWritesNothing(t *testing.T) {
	cases := map[string][]string{
		"no words":         {"want"},
		"two words":        {"want", "make", "it"},
		"empty":            {"want", "   "},
		"a line break":     {"want", "first line\nsecond line"},
		"a tab":            {"want", "a\ttab"},
		"no letter at all": {"want", "?! — …"},
	}
	says := map[string]string{
		"no words":         "quoted",
		"two words":        "quoted",
		"empty":            "empty",
		"a line break":     "one line",
		"a tab":            "one line",
		"no letter at all": "kept new desire <slug>",
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			w := newCorpus(t)
			before := tree(t, w.root)
			code, _, stderr := w.run(args...)
			if code != exitUsage {
				t.Fatalf("exit %d: %s", code, stderr)
			}
			mustContain(t, "the refusal", stderr, says[name])
			if after := tree(t, w.root); len(after) != len(before) {
				t.Fatalf("a refused want wrote something")
			}
		})
	}

	w := newCorpus(t, "promise")
	code, _, stderr := w.run("want", "a corpus with no desire kind")
	if code != exitUsage {
		t.Fatalf("a corpus declaring no desire exited %d", code)
	}
	mustContain(t, "the refusal", stderr, `nothing declares the type "desire"`)
}
