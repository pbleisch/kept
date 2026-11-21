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
	// RulingOutOfReach: the pass made a case no attempt at this repair can
	// answer — the fix lies outside the paths the promise's repair scope binds,
	// or needs a decision only a person can make — so the episode goes to a
	// person and no further attempt is opened. It is the one ruling that carries
	// words of its own: the reason, on the same line, after a colon. See
	// docs/decision/0096.
	RulingOutOfReach Ruling = "out-of-reach"
)

// rulingLine is how a ruling is written as the last line of an argument. It is
// declared beside the ask that asks for it, so the words the pass is told to
// write and the words kept reads are one constant rather than two.
const rulingLine = "ruling: "

// RulingText is the exact line a pass ends with to make a ruling. An
// out-of-reach ruling continues past it, with a colon and its reason.
func RulingText(ruling Ruling) string { return rulingLine + string(ruling) }

// OutOfReachText is the whole line a pass ends with to rule that the fix is out
// of the repair's reach, for one reason.
func OutOfReachText(reason string) string {
	return RulingText(RulingOutOfReach) + ": " + reason
}

// ReadRuling reads the ruling an argument ends with.
//
// The last line that is not blank, with surrounding whitespace removed and with
// the emphasis and code marks a markdown renderer would draw around it — a
// runtime that bolded its last line wrote the same token. That line is compared
// against exactly `ruling: reject` and `ruling: accept`, or must begin with
// exactly `ruling: out-of-reach: ` and go on to a reason, and anything else is
// no ruling at all: a ruling in the middle of a paragraph is not one, a near
// miss is not one, an out-of-reach ruling that gives no reason is not one, and
// nothing is inferred from the tone of what came before.
//
// The second return says why: why there is no ruling, or, for an out-of-reach
// ruling, the reason the pass gave on that line, verbatim. It is empty for an
// accept and for a reject, whose reasons are the argument above them. A missing
// ruling is never read as an accept: a pass that forgot its last line would
// otherwise land every repair it had just argued against. It is derived here,
// on read, from words that are already durable, rather than recorded beside
// them, because a stored copy of a pure function of a record is the cache that
// can disagree with its source. See docs/decision/0086 and 0096.
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
	if reason, ok := strings.CutPrefix(line, RulingText(RulingOutOfReach)+":"); ok {
		if reason = strings.TrimSpace(reason); reason != "" && strings.HasPrefix(line, OutOfReachText("")) {
			return RulingOutOfReach, reason
		}
		return "", fmt.Sprintf("its last line is %q, and an out-of-reach ruling is %q followed by its reason",
			shorten(strings.TrimSpace(last), 80), OutOfReachText(""))
	}
	return "", fmt.Sprintf("its last line is %q, and a ruling is exactly %q or %q, or %q followed by a reason",
		shorten(strings.TrimSpace(last), 80), RulingText(RulingReject), RulingText(RulingAccept), OutOfReachText(""))
}

// Known reports whether a ruling is one of the three a pass can make. The empty
// ruling is not one: it is what an argument with no readable ruling reads as.
func (r Ruling) Known() bool {
	return r == RulingReject || r == RulingAccept || r == RulingOutOfReach
}

// shorten cuts a line to at most n runes for a sentence that quotes it, and says
// it did.
func shorten(line string, n int) string {
	if utf8.RuneCountInString(line) <= n {
		return line
	}
	runes := []rune(line)
	return string(runes[:n]) + "…"
}
