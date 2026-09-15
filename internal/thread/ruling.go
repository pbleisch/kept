package thread

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Ruling is the one closed token an adversarial pass ends with: whether the case
// it made is one another attempt at the repair should answer.
//
// It is the only part of what a pass says that kept reads. The argument stays
// verbatim and unparsed, exactly as docs/decision/0081 files it; this is the
// last line of it, compared against two exact strings, and nothing else in the
// words is scanned for anything.
//
// It is named a ruling and never a verdict. A verdict is the promise's, reached
// by measuring the world, and a token spelled like one would put the input
// policy:the-agents-report-is-never-an-input-to-the-verdict refuses one careless
// refactor away. A ruling decides whether a repair is attempted again, and
// nothing more: not whether the promise is kept, not whether the work may land.
// See docs/decision/0086.
type Ruling string

const (
	// RulingReject: the pass made a case another attempt should answer, so the
	// repair goes back for rework.
	RulingReject Ruling = "reject"
	// RulingAccept: the pass could not make such a case.
	RulingAccept Ruling = "accept"
)

// rulingLine is how a ruling is written as the last line of an argument. It is
// declared beside the ask that asks for it, so the words the pass is told to
// write and the words kept reads are one constant rather than two.
const rulingLine = "ruling: "

// RulingText is the exact line a pass ends with to make a ruling.
func RulingText(ruling Ruling) string { return rulingLine + string(ruling) }

// ReadRuling reads the ruling an argument ends with.
//
// The last line that is not blank, with surrounding whitespace removed and with
// the emphasis and code marks a markdown renderer would draw around it — a
// runtime that bolded its last line wrote the same token. That line is compared
// against exactly `ruling: reject` and `ruling: accept`, and anything else is no
// ruling at all: a ruling in the middle of a paragraph is not one, a near miss
// is not one, and nothing is inferred from the tone of what came before.
//
// The second return says why, when there is no ruling. A missing ruling is never
// read as an accept: a pass that forgot its last line would otherwise land every
// repair it had just argued against. It is derived here, on read, from words
// that are already durable, rather than recorded beside them, because a stored
// copy of a pure function of a record is the cache that can disagree with its
// source. See docs/decision/0086.
func ReadRuling(said string) (Ruling, string) {
	last := ""
	for line := range strings.SplitSeq(said, "\n") {
		if strings.TrimSpace(line) != "" {
			last = line
		}
	}
	if last == "" {
		return "", "the pass wrote nothing, so it made no ruling"
	}
	line := strings.Trim(strings.TrimSpace(last), "*_`")
	line = strings.TrimSpace(line)
	for _, ruling := range []Ruling{RulingReject, RulingAccept} {
		if line == RulingText(ruling) {
			return ruling, ""
		}
	}
	return "", fmt.Sprintf("its last line is %q, and a ruling is exactly %q or %q",
		shorten(strings.TrimSpace(last), 80), RulingText(RulingReject), RulingText(RulingAccept))
}

// Known reports whether a ruling is one of the two a pass can make. The empty
// ruling is not one: it is what an argument with no readable ruling reads as.
func (r Ruling) Known() bool { return r == RulingReject || r == RulingAccept }

// shorten cuts a line to at most n runes for a sentence that quotes it, and says
// it did.
func shorten(line string, n int) string {
	if utf8.RuneCountInString(line) <= n {
		return line
	}
	runes := []rune(line)
	return string(runes[:n]) + "…"
}
