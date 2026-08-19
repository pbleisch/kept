package predicate

import (
	"fmt"
	"strings"
)

// Error is one of the two ways an expression can fail, and it is the one that
// is the author's fault: the expression is unauthorable. A type mismatch, a
// function nothing declares, a fold over something that is not a list, a root
// this namespace does not have.
//
// The other way is [Unknown], which is a value rather than an error. Keeping
// them apart is why this language is hand-written at all: CEL returns both
// through one channel, so *nobody measured* and *this expression is wrong* are
// told apart by matching English prose in the code path that decides a verdict.
// Here they are two Go types, and nothing anywhere needs to read a message to
// know which happened. See docs/decision/0036.
//
// Never test for a kind of failure by matching Msg. If a caller needs to tell
// two errors apart, that distinction belongs in the type system, not in the
// wording.
type Error struct {
	// Msg says what is wrong, in the author's terms rather than the parser's.
	Msg string
	// Pos is where in the source it is wrong.
	Pos Position
	// Src is the whole expression, carried so the error can render itself
	// against the line it came from. It is a copy: an Error outlives the parse.
	Src string
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s (line %d, column %d)", e.Msg, e.Pos.Line, e.Pos.Column)
}

// Render is the multi-line form a person reads: what is wrong, the line it is
// wrong on, and a caret under it. See docs/decision/0036 for the shape.
func (e *Error) Render() string {
	var b strings.Builder
	b.WriteString(e.Msg)
	line := e.sourceLine()
	if line == "" {
		fmt.Fprintf(&b, "\n  at line %d, column %d", e.Pos.Line, e.Pos.Column)
		return b.String()
	}
	b.WriteString("\n  ")
	b.WriteString(line)
	b.WriteString("\n  ")
	// The caret is placed by rune count, because the column is counted the same
	// way. A tab in an expression will still point one character wide; nobody
	// writes one and being approximately right there is not worth a table.
	b.WriteString(strings.Repeat(" ", max(e.Pos.Column-1, 0)))
	fmt.Fprintf(&b, "^ at line %d, column %d", e.Pos.Line, e.Pos.Column)
	return b.String()
}

func (e *Error) sourceLine() string {
	if e.Src == "" || e.Pos.Line < 1 {
		return ""
	}
	lines := strings.Split(e.Src, "\n")
	if e.Pos.Line > len(lines) {
		return ""
	}
	return lines[e.Pos.Line-1]
}

// Position is where in an expression something is. Line and Column are
// one-based and counted in runes, so a caret under a multi-byte source lands
// where a person sees the character.
type Position struct {
	Offset int
	Line   int
	Column int
}

func (p Position) String() string { return fmt.Sprintf("line %d, column %d", p.Line, p.Column) }

// errorf builds an Error against a source. The source is copied onto every one
// of them because an Error is returned to a caller that has no parser left to
// ask.
func errorf(src string, pos Position, format string, args ...any) *Error {
	return &Error{Msg: fmt.Sprintf(format, args...), Pos: pos, Src: src}
}
