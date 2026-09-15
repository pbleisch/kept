package thread

import (
	"strings"
	"testing"
)

// TestARulingIsTheLastLineAndNothingElse is the one place kept reads what an
// adversarial pass said, and what it asserts is how little is read: the last
// line that is not blank, compared against two exact strings. See
// docs/decision/0086.
func TestARulingIsTheLastLineAndNothingElse(t *testing.T) {
	for _, c := range []struct {
		name string
		said string
		want Ruling
	}{
		{"a reject", "The tests assert less than their names.\n\nruling: reject\n", RulingReject},
		{"an accept", "I could not make a case against this.\nruling: accept", RulingAccept},
		{"trailing blank lines", "case\nruling: reject\n\n\n  \n", RulingReject},
		{"surrounding whitespace", "case\n   ruling: accept   \n", RulingAccept},
		{"bolded", "case\n**ruling: reject**\n", RulingReject},
		{"as code", "case\n`ruling: accept`\n", RulingAccept},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, why := ReadRuling(c.said)
			if got != c.want {
				t.Fatalf("ReadRuling = %q (%s), want %q", got, why, c.want)
			}
			if why != "" {
				t.Fatalf("a readable ruling came with a reason it was not one: %s", why)
			}
		})
	}
}

// TestAnArgumentWithNoRulingIsNeverAnAccept breaks the token every way a pass
// could plausibly get it wrong, and holds the reading to *no ruling* with a
// reason — never to accept, which would land every repair a forgetful pass had
// just argued against, and never to reject either, which is a reading of the
// tone rather than of the token.
func TestAnArgumentWithNoRulingIsNeverAnAccept(t *testing.T) {
	for _, c := range []struct {
		name string
		said string
		why  string
	}{
		{"nothing at all", "", "wrote nothing"},
		{"only blank lines", "\n  \n\t\n", "wrote nothing"},
		{"no ruling line", "The repair does not meet the statement.\n", "its last line is"},
		{"a ruling that is not last", "ruling: reject\nAnd one more thing about the tests.\n", "its last line is"},
		{"capitalized", "case\nRuling: Reject\n", "its last line is"},
		{"a word too many", "case\nruling: reject, mostly\n", "its last line is"},
		{"a value outside the set", "case\nruling: rework\n", "its last line is"},
		{"punctuated", "case\nruling: accept.\n", "its last line is"},
		{"no space", "case\nruling:reject\n", "its last line is"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, why := ReadRuling(c.said)
			if got != "" {
				t.Fatalf("ReadRuling(%q) = %q, want no ruling", c.said, got)
			}
			if !strings.Contains(why, c.why) {
				t.Fatalf("the reason is %q, want it to say %q", why, c.why)
			}
		})
	}
	// A long last line is quoted short, so the reason stays a sentence.
	_, why := ReadRuling(strings.Repeat("x", 500))
	if len(why) > 300 {
		t.Fatalf("the reason quotes a whole paragraph: %d bytes", len(why))
	}
}

// TestTheAssessorIsAskedForExactlyTheLinesKeptReads holds the ask and the reader
// to one spelling. A pass told to write one thing and read for another would end
// every argument with a ruling kept cannot see, and every episode would escalate
// on a typo in a constant.
func TestTheAssessorIsAskedForExactlyTheLinesKeptReads(t *testing.T) {
	ask, err := claudeAskText(ClaudeAssessAsk)
	if err != nil {
		t.Fatal(err)
	}
	for _, ruling := range []Ruling{RulingReject, RulingAccept} {
		if !strings.Contains(ask, RulingText(ruling)) {
			t.Errorf("the assessor's ask does not name %q, which is what kept reads", RulingText(ruling))
		}
		if got, _ := ReadRuling("case\n" + RulingText(ruling)); got != ruling {
			t.Errorf("the line the ask names reads as %q, want %q", got, ruling)
		}
	}
	// And the ruling is framed as deciding whether to try again, never as the
	// promise's answer or as a land.
	for _, says := range []string{"does not decide whether the promise is kept", "does not decide whether the work lands"} {
		if !strings.Contains(ask, says) {
			t.Errorf("the assessor's ask no longer says the ruling %s", says)
		}
	}
}
