package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// A destination that is not a terminal gets the plain form: one event to a
// line, an instant that sorts, and no cursor control of any kind.
//
// The test's writers are buffers, which is what a CI job, a pipe and a captured
// pane all look like from inside kept — so this is the degraded form under
// exactly the conditions decision 0065 promises it for.
func TestATickSaysWhatItIsMeasuringOnACapturedStream(t *testing.T) {
	w := newChecked(t)
	w.install(t)
	if code, _, stderr := w.run("ratify", "the-marker-says-ok", "--by", "the operator"); code != exitOK {
		t.Fatalf("ratify exited %d: %s", code, stderr)
	}

	_, _, stderr := w.run("--all")
	for _, want := range []string{
		"kept gather started promise:the-marker-says-ok · command",
		"kept gather finished promise:the-marker-says-ok · command",
	} {
		if !strings.Contains(stderr, want) {
			t.Errorf("the tick never said %q:\n%s", want, stderr)
		}
	}
	assertNoCursorControl(t, stderr)
}

// --quiet is quiet, and it is the only flag this display has.
//
// It earns itself rather than being a verbosity ladder: problems and progress
// share stderr — a refusal, a quarantined ledger line and an advanced ledger
// are all on it — so an operator cannot get "problems only" by redirecting the
// stream. This is the one thing the default cannot do for them.
func TestQuietSaysNothingWhileTheTickRuns(t *testing.T) {
	w := newChecked(t)
	w.install(t)
	if code, _, stderr := w.run("ratify", "the-marker-says-ok", "--by", "the operator"); code != exitOK {
		t.Fatalf("ratify exited %d: %s", code, stderr)
	}

	_, _, stderr := w.run("--all", "--quiet")
	if strings.Contains(stderr, "kept gather") {
		t.Errorf("--quiet still reported a gather:\n%s", stderr)
	}
}

// The structured document is untouched by the display, byte for byte.
//
// docs/decision/0027 makes the machine-facing surface a contract, and progress
// is not part of it: the document is on stdout and the display is on stderr, so
// the two runs below differ only in the instant the document says it was
// written. Everything else — including the exit code the document carries — is
// the same bytes in the same order.
func TestTheStructuredDocumentIsUnchangedByTheDisplay(t *testing.T) {
	w := newChecked(t)
	w.install(t)
	if code, _, stderr := w.run("ratify", "the-marker-says-ok", "--by", "the operator"); code != exitOK {
		t.Fatalf("ratify exited %d: %s", code, stderr)
	}
	// One pass first, so that neither of the two below records anything new and
	// the only moving part left is the document's own timestamp. Both force
	// every artifact, because a settled tick gathers nothing and a display with
	// nothing to report proves nothing about a display.
	w.run("--all")

	quietCode, quiet, _ := w.run("--all", "--json", "--quiet")
	watchedCode, watched, watchedErr := w.run("--all", "--json")

	if quietCode != watchedCode {
		t.Fatalf("the display changed the exit code: %d against %d", quietCode, watchedCode)
	}
	if strings.Contains(quiet, "kept gather") || strings.Contains(watched, "kept gather") {
		t.Fatalf("a progress line reached stdout:\n%s", watched)
	}
	if !strings.Contains(watchedErr, "kept gather") {
		t.Fatalf("the watched run reported nothing on stderr:\n%s", watchedErr)
	}

	// The document's `at` is the moment it was written and is the one field two
	// runs cannot share. Substituting it is what makes the rest a byte
	// comparison rather than a comparison of meaning.
	if substituted := strings.Replace(watched, writtenAt(t, watched), writtenAt(t, quiet), 1); substituted != quiet {
		t.Errorf("the display changed the document:\n--- quiet ---\n%s\n--- watched ---\n%s", quiet, substituted)
	}
}

// Exit codes are the other half of 0027's contract, and the display is not
// allowed near them. Three values, three codes, watched and unwatched alike.
func TestExitCodesAreUnchangedByTheDisplay(t *testing.T) {
	for _, quiet := range []bool{false, true} {
		w := newChecked(t)
		if code, _, stderr := w.run("ratify", "the-marker-says-ok", "--by", "the operator"); code != exitOK {
			t.Fatalf("ratify exited %d: %s", code, stderr)
		}
		args := func(rest ...string) []string {
			if quiet {
				return append(rest, "--quiet")
			}
			return rest
		}

		// No instrument: exit 3, because a machine that could not measure has
		// not reported a broken promise.
		if code, _, _ := w.run(args()...); code != exitUnknown {
			t.Errorf("a missing toolchain with quiet=%v exited %d, want %d", quiet, code, exitUnknown)
		}
		// The instrument arrives and the marker is wrong: exit 1.
		w.install(t)
		if code, _, _ := w.run(args("--all")...); code != exitDrift {
			t.Errorf("a broken promise with quiet=%v exited %d, want %d", quiet, code, exitDrift)
		}
		// The marker is mended: exit 0.
		w.put("src/marker.txt", "ok\n")
		if code, _, _ := w.run(args("--all")...); code != exitOK {
			t.Errorf("a kept promise with quiet=%v exited %d, want %d", quiet, code, exitOK)
		}
	}
}

// The tick names what is wrong with the corpus instead of counting it and
// pointing at another verb.
//
// The count was the weakest useful thing kept could say, and it sat directly
// under a confident summary: "1 problem reading the corpus" beneath "1 artifact
// · 1 kept" reads as a footnote rather than a finding. The words are `kept
// list`'s own, from the same function, so the two cannot come to disagree.
func TestTheTickNamesTheProblemsItUsedToCount(t *testing.T) {
	w := newChecked(t)
	w.install(t)
	if code, _, stderr := w.run("ratify", "the-marker-says-ok", "--by", "the operator"); code != exitOK {
		t.Fatalf("ratify exited %d: %s", code, stderr)
	}
	// A file in a type directory with no frontmatter is not an artifact, and
	// saying so is what a problem is.
	w.put("docs/promise/half-written.md", "A promise somebody started and did not finish.\n")

	_, stdout, _ := w.run("--all")
	mustContain(t, "the tick", stdout,
		"1 problem reading the corpus",
		"promise/half-written.md",
		"no-frontmatter",
	)
	if strings.Contains(stdout, "kept list says what they are") {
		t.Errorf("the tick still points at another verb instead of answering:\n%s", stdout)
	}

	// And it is the same sentence kept list prints, because it is the same
	// function printing it.
	_, listed, _ := w.run("list")
	for _, line := range strings.Split(stdout, "\n") {
		if strings.Contains(line, "no-frontmatter") && !strings.Contains(listed, strings.TrimSpace(line)) {
			t.Errorf("the tick and kept list describe the problem differently:\n%s\n---\n%s", line, listed)
		}
	}
}

// writtenAt reads the document's own timestamp out of it.
func writtenAt(t *testing.T, document string) string {
	t.Helper()
	var doc struct {
		At string `json:"at"`
	}
	if err := json.Unmarshal([]byte(document), &doc); err != nil {
		t.Fatalf("the document does not parse: %v\n%s", err, document)
	}
	if doc.At == "" {
		t.Fatalf("the document carries no instant:\n%s", document)
	}
	return doc.At
}

// assertNoCursorControl is the acceptance condition for a destination that is
// not a terminal: no escape, no carriage return, no bell, and no other C0
// control but the newline that ends a line.
func assertNoCursorControl(t *testing.T, got string) {
	t.Helper()
	for i, r := range got {
		if r == '\n' || r == '\t' {
			continue
		}
		if r < 0x20 || r == 0x7f {
			t.Fatalf("byte %d of a captured stream is control character %#U:\n%q", i, r, got)
		}
	}
}
