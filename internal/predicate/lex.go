package predicate

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// tokenKind is what a piece of source is. The set is small on purpose: the
// language has comparison, existence, set membership and a fold, and every
// operator it does not have is one an author cannot come to depend on. Adding
// one later is additive; taking one away is not, because a promise's hold_when
// binds. See docs/decision/0036.
type tokenKind int

const (
	tokenEOF tokenKind = iota
	tokenIdent
	tokenInt
	tokenFloat
	tokenString
	tokenLParen
	tokenRParen
	tokenComma
	tokenDot
	tokenArrow // =>
	tokenMinus
	tokenEQ
	tokenNE
	tokenLT
	tokenLE
	tokenGT
	tokenGE
	// The word operators. They are lexed as their own kinds rather than as
	// identifiers so that `and` can never be a field name by accident.
	tokenAnd
	tokenOr
	tokenNot
	tokenIn
	tokenTrue
	tokenFalse
)

func (k tokenKind) String() string {
	switch k {
	case tokenEOF:
		return "the end of the expression"
	case tokenIdent:
		return "a name"
	case tokenInt, tokenFloat:
		return "a number"
	case tokenString:
		return "text"
	case tokenLParen:
		return "("
	case tokenRParen:
		return ")"
	case tokenComma:
		return ","
	case tokenDot:
		return "."
	case tokenArrow:
		return "=>"
	case tokenMinus:
		return "-"
	case tokenEQ:
		return "=="
	case tokenNE:
		return "!="
	case tokenLT:
		return "<"
	case tokenLE:
		return "<="
	case tokenGT:
		return ">"
	case tokenGE:
		return ">="
	case tokenAnd:
		return "and"
	case tokenOr:
		return "or"
	case tokenNot:
		return "not"
	case tokenIn:
		return "in"
	case tokenTrue:
		return "true"
	case tokenFalse:
		return "false"
	}
	return "something"
}

type token struct {
	kind tokenKind
	pos  Position
	// text is the source as written, for identifiers and for messages.
	text string
	// value is the decoded literal: a string with its escapes resolved, an
	// int64, or a float64.
	value any
}

var words = map[string]tokenKind{
	"and":   tokenAnd,
	"or":    tokenOr,
	"not":   tokenNot,
	"in":    tokenIn,
	"true":  tokenTrue,
	"false": tokenFalse,
}

// lex turns source into tokens, or returns the first thing in it that is not
// part of this language. It reads the whole expression up front: an authored
// hold_when is bounded by [MaxSource], so there is no streaming case to serve
// and a token slice is what the parser wants to look ahead in.
func lex(src string) ([]token, *Error) {
	l := &lexer{src: src, line: 1, col: 1}
	var out []token
	for {
		t, err := l.next()
		if err != nil {
			return nil, err
		}
		out = append(out, t)
		if t.kind == tokenEOF {
			return out, nil
		}
	}
}

type lexer struct {
	src  string
	off  int
	line int
	col  int
}

func (l *lexer) pos() Position { return Position{Offset: l.off, Line: l.line, Column: l.col} }

func (l *lexer) peek() (rune, int) {
	if l.off >= len(l.src) {
		return 0, 0
	}
	return utf8.DecodeRuneInString(l.src[l.off:])
}

func (l *lexer) advance(width int) {
	for i := 0; i < width; {
		r, w := utf8.DecodeRuneInString(l.src[l.off+i:])
		if r == '\n' {
			l.line++
			l.col = 1
		} else {
			l.col++
		}
		i += w
	}
	l.off += width
}

func (l *lexer) next() (token, *Error) {
	l.skipSpace()
	start := l.pos()
	r, w := l.peek()
	if w == 0 {
		return token{kind: tokenEOF, pos: start}, nil
	}

	switch r {
	case '(':
		l.advance(w)
		return token{kind: tokenLParen, pos: start, text: "("}, nil
	case ')':
		l.advance(w)
		return token{kind: tokenRParen, pos: start, text: ")"}, nil
	case ',':
		l.advance(w)
		return token{kind: tokenComma, pos: start, text: ","}, nil
	case '.':
		l.advance(w)
		return token{kind: tokenDot, pos: start, text: "."}, nil
	case '-':
		l.advance(w)
		return token{kind: tokenMinus, pos: start, text: "-"}, nil
	case '"', '\'':
		return l.lexString(r)
	case '=':
		l.advance(w)
		switch next, nw := l.peek(); next {
		case '=':
			l.advance(nw)
			return token{kind: tokenEQ, pos: start, text: "=="}, nil
		case '>':
			l.advance(nw)
			return token{kind: tokenArrow, pos: start, text: "=>"}, nil
		}
		return token{}, errorf(l.src, start, "a lone = means nothing here; == compares and => introduces a lambda")
	case '!':
		l.advance(w)
		if next, nw := l.peek(); next == '=' {
			l.advance(nw)
			return token{kind: tokenNE, pos: start, text: "!="}, nil
		}
		return token{}, errorf(l.src, start, "a lone ! means nothing here; write not")
	case '<', '>':
		l.advance(w)
		kind := tokenLT
		text := "<"
		if r == '>' {
			kind, text = tokenGT, ">"
		}
		if next, nw := l.peek(); next == '=' {
			l.advance(nw)
			if r == '<' {
				return token{kind: tokenLE, pos: start, text: "<="}, nil
			}
			return token{kind: tokenGE, pos: start, text: ">="}, nil
		}
		return token{kind: kind, pos: start, text: text}, nil
	}

	switch {
	case unicode.IsDigit(r):
		return l.lexNumber()
	case r == '_' || unicode.IsLetter(r):
		return l.lexIdent()
	}
	return token{}, errorf(l.src, start, "%q is not part of this language", string(r))
}

func (l *lexer) skipSpace() {
	for {
		r, w := l.peek()
		if w == 0 || !unicode.IsSpace(r) {
			return
		}
		l.advance(w)
	}
}

func (l *lexer) lexIdent() (token, *Error) {
	start := l.pos()
	begin := l.off
	for {
		r, w := l.peek()
		if w == 0 || !(r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)) {
			break
		}
		l.advance(w)
	}
	text := l.src[begin:l.off]
	if kind, ok := words[text]; ok {
		t := token{kind: kind, pos: start, text: text}
		switch kind {
		case tokenTrue:
			t.value = true
		case tokenFalse:
			t.value = false
		}
		return t, nil
	}
	return token{kind: tokenIdent, pos: start, text: text}, nil
}

func (l *lexer) lexNumber() (token, *Error) {
	start := l.pos()
	begin := l.off
	float := false
	for {
		r, w := l.peek()
		if w == 0 {
			break
		}
		if unicode.IsDigit(r) {
			l.advance(w)
			continue
		}
		// A dot is only part of the number when a digit follows it. Without
		// that rule `obs.tests.1` would lex as a float and the member access
		// would vanish into the literal.
		if r == '.' && !float && l.off+w < len(l.src) {
			if next, _ := utf8.DecodeRuneInString(l.src[l.off+w:]); unicode.IsDigit(next) {
				float = true
				l.advance(w)
				continue
			}
		}
		break
	}
	text := l.src[begin:l.off]
	if float {
		f, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return token{}, errorf(l.src, start, "%s is not a number this language can read", text)
		}
		return token{kind: tokenFloat, pos: start, text: text, value: f}, nil
	}
	n, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return token{}, errorf(l.src, start, "%s does not fit in a whole number", text)
	}
	return token{kind: tokenInt, pos: start, text: text, value: n}, nil
}

// lexString reads a quoted literal. Double quotes interpret escapes; single
// quotes do not, and that asymmetry is for one reason: patterns are Go's
// regexp, and a raw form is what makes '\d+' writable without a second layer of
// backslashes on top of YAML's. An unknown escape is refused rather than passed
// through, because passing it through would silently change what a pattern
// matches.
func (l *lexer) lexString(quote rune) (token, *Error) {
	start := l.pos()
	l.advance(utf8.RuneLen(quote))

	var b strings.Builder
	for {
		r, w := l.peek()
		if w == 0 {
			return token{}, errorf(l.src, start, "this text is never closed; it wants a matching %c", quote)
		}
		if r == '\n' {
			return token{}, errorf(l.src, start, "this text runs off the end of the line; it wants a matching %c", quote)
		}
		if r == quote {
			l.advance(w)
			return token{kind: tokenString, pos: start, text: b.String(), value: b.String()}, nil
		}
		if quote == '"' && r == '\\' {
			l.advance(w)
			esc, ew := l.peek()
			if ew == 0 {
				return token{}, errorf(l.src, start, "this text is never closed; it wants a matching %c", quote)
			}
			pos := l.pos()
			l.advance(ew)
			switch esc {
			case '\\':
				b.WriteByte('\\')
			case '"':
				b.WriteByte('"')
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case 't':
				b.WriteByte('\t')
			default:
				return token{}, errorf(l.src, pos, `\%s is not an escape this language knows; write it in single quotes to mean it literally`, string(esc))
			}
			continue
		}
		l.advance(w)
		b.WriteRune(r)
	}
}
