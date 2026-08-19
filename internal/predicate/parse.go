package predicate

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// The bounds on an authored expression. They are here rather than in a config
// because an expression that wedges a tick is not something an operator should
// be able to opt into: `hold_when` binds, so a promise that parsed once must
// parse the same way forever, and a limit anyone can raise is a limit that
// varies between two checkouts of one corpus.
//
// Each one gets a test that deliberately exceeds it, per AGENTS.md. See
// docs/decision/0036: a depth limit and a node-count cap so an authored
// expression cannot wedge a tick.
const (
	// MaxSource bounds the bytes the lexer will read. It is the first bound
	// because everything after it is proportional to the token count.
	MaxSource = 8 << 10
	// MaxDepth bounds how deeply expressions nest. It is enforced while
	// parsing rather than after, because the parser recurses and a check that
	// ran afterwards would run after the stack was already gone.
	MaxDepth = 32
	// MaxNodes bounds the whole tree. Depth alone does not: `a and a and a…`
	// is left-associative and stays shallow while growing without bound.
	MaxNodes = 1024
	// MaxSteps bounds evaluation. Depth and node count bound the source, and
	// neither bounds a fold: nested folds over a large observation are
	// polynomial in the data, which is a way to wedge a tick that no bound on
	// the text can see.
	MaxSteps = 200_000
)

// Namespace is the set of roots an expression may read. One grammar and one
// evaluator over two of them: `hold_when` reads obs, and an extractor reads
// what a program produced. See docs/decision/0036 and 0023.
type Namespace struct {
	// Name is what the namespace is called in a message.
	Name string
	// roots are the identifiers that resolve without being bound by a lambda.
	roots []string
	// prior says whether prior() is available. It is not in an extractor:
	// there is no history of a program's output, only the run that just
	// happened.
	prior bool
}

// HoldWhen is the predicate namespace: obs.<name>.<field>, plus prior().
var HoldWhen = Namespace{Name: "hold_when", roots: []string{"obs"}, prior: true}

// Extractor is the namespace a gatherer's declared fields are written in. 0023
// requires `command` to name what it pulls out of a program's output; these
// three are what there is to pull it out of.
var Extractor = Namespace{Name: "an extractor", roots: []string{"exit", "stderr", "stdout"}}

func (n Namespace) has(root string) bool {
	for _, r := range n.roots {
		if r == root {
			return true
		}
	}
	return false
}

func (n Namespace) rootList() string { return strings.Join(n.roots, ", ") }

// Expr is a parsed expression. It is immutable and safe to evaluate
// concurrently: the tree holds no evaluation state, and every scope is passed
// in.
type Expr struct {
	// Src is the expression as authored, carried for error rendering.
	Src string
	// NS is the namespace it was parsed against.
	NS   Namespace
	root node
	// nodes is what the tree cost, kept so a caller can see how close an
	// expression came to the cap.
	nodes int
}

// String returns the expression as it was authored.
func (e *Expr) String() string { return e.Src }

// Nodes is how many nodes the tree has.
func (e *Expr) Nodes() int { return e.nodes }

// Parse reads one expression in one namespace. Everything that can be decided
// without looking at the world is decided here — the syntax, the roots, the
// functions, their arity, where a lambda may appear — so that a promise whose
// predicate is unauthorable is refused when the corpus loads rather than in the
// code path that decides a verdict.
//
// The error is always an [*Error]. It is never an [Unknown]: nothing has been
// measured at parse time, so there is nothing that could have failed to
// measure.
func Parse(src string, ns Namespace) (*Expr, error) {
	if len(src) > MaxSource {
		return nil, errorf("", Position{Line: 1, Column: 1},
			"this expression is %d bytes and the limit is %d; a predicate that long is doing a gatherer's job", len(src), MaxSource)
	}
	if strings.TrimSpace(src) == "" {
		return nil, errorf(src, Position{Line: 1, Column: 1}, "there is no expression here")
	}

	tokens, lexErr := lex(src)
	if lexErr != nil {
		return nil, lexErr
	}

	p := &parser{src: src, ns: ns, tokens: tokens}
	root, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if t := p.peek(); t.kind != tokenEOF {
		return nil, errorf(src, t.pos, "the expression ended and then %s followed it", describe(t))
	}
	return &Expr{Src: src, NS: ns, root: root, nodes: p.nodes}, nil
}

func describe(t token) string {
	if t.kind == tokenIdent {
		return fmt.Sprintf("%q", t.text)
	}
	if t.kind == tokenString {
		return fmt.Sprintf("the text %q", t.text)
	}
	return t.kind.String()
}

type parser struct {
	src    string
	ns     Namespace
	tokens []token
	at     int
	depth  int
	nodes  int
	// bound is the stack of lambda parameters in scope. A bare name is a
	// parameter when one is in scope and a root otherwise, which is the only
	// place this language has scoping at all.
	bound []string
}

func (p *parser) peek() token { return p.tokens[p.at] }

func (p *parser) take() token {
	t := p.tokens[p.at]
	if t.kind != tokenEOF {
		p.at++
	}
	return t
}

func (p *parser) accept(k tokenKind) (token, bool) {
	if p.tokens[p.at].kind == k {
		return p.take(), true
	}
	return token{}, false
}

func (p *parser) expect(k tokenKind) (token, *Error) {
	t := p.peek()
	if t.kind != k {
		return token{}, errorf(p.src, t.pos, "%s was expected here and %s was found", k, describe(t))
	}
	return p.take(), nil
}

// count charges one node and refuses past the cap. Every constructor calls it,
// so there is no shape of expression that can grow the tree without being
// counted.
func (p *parser) count(pos Position) *Error {
	p.nodes++
	if p.nodes > MaxNodes {
		return errorf(p.src, pos, "this expression has more than %d parts, which is more than a predicate should need", MaxNodes)
	}
	return nil
}

func (p *parser) enter(pos Position) *Error {
	p.depth++
	if p.depth > MaxDepth {
		return errorf(p.src, pos, "this expression nests more than %d deep, which is deeper than a predicate should need", MaxDepth)
	}
	return nil
}

func (p *parser) leave() { p.depth-- }

// parseExpr is the whole precedence ladder: or, then and, then not, then a
// single non-associative comparison, then a primary with member access and
// calls hanging off it.
func (p *parser) parseExpr() (node, *Error) {
	if err := p.enter(p.peek().pos); err != nil {
		return nil, err
	}
	defer p.leave()
	return p.parseOr()
}

func (p *parser) parseOr() (node, *Error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for {
		t, ok := p.accept(tokenOr)
		if !ok {
			return left, nil
		}
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		if err := p.count(t.pos); err != nil {
			return nil, err
		}
		left = &binaryNode{pos: t.pos, op: tokenOr, left: left, right: right}
	}
}

func (p *parser) parseAnd() (node, *Error) {
	left, err := p.parseNot()
	if err != nil {
		return nil, err
	}
	for {
		t, ok := p.accept(tokenAnd)
		if !ok {
			return left, nil
		}
		right, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		if err := p.count(t.pos); err != nil {
			return nil, err
		}
		left = &binaryNode{pos: t.pos, op: tokenAnd, left: left, right: right}
	}
}

func (p *parser) parseNot() (node, *Error) {
	t, ok := p.accept(tokenNot)
	if !ok {
		return p.parseComparison()
	}
	if err := p.enter(t.pos); err != nil {
		return nil, err
	}
	defer p.leave()
	operand, err := p.parseNot()
	if err != nil {
		return nil, err
	}
	if err := p.count(t.pos); err != nil {
		return nil, err
	}
	return &notNode{pos: t.pos, operand: operand}, nil
}

// parseComparison is deliberately non-associative. `a < b < c` reads as
// something in every language that has it and means something different in
// most of them, so this one refuses it and says what to write instead.
func (p *parser) parseComparison() (node, *Error) {
	left, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	t := p.peek()
	switch t.kind {
	case tokenEQ, tokenNE, tokenLT, tokenLE, tokenGT, tokenGE, tokenIn:
	default:
		return left, nil
	}
	p.take()
	right, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	if err := p.count(t.pos); err != nil {
		return nil, err
	}
	if next := p.peek(); isComparison(next.kind) {
		return nil, errorf(p.src, next.pos, "two comparisons in a row mean nothing here; write the two halves joined by and")
	}
	return &binaryNode{pos: t.pos, op: t.kind, left: left, right: right}, nil
}

func isComparison(k tokenKind) bool {
	switch k {
	case tokenEQ, tokenNE, tokenLT, tokenLE, tokenGT, tokenGE, tokenIn:
		return true
	}
	return false
}

func (p *parser) parsePrimary() (node, *Error) {
	if err := p.enter(p.peek().pos); err != nil {
		return nil, err
	}
	defer p.leave()

	base, err := p.parseOperand()
	if err != nil {
		return nil, err
	}
	// Member access, left to right. There is no indexing: a field is named by
	// an identifier, and a key that is not one is reached with has() or not at
	// all. That bound is 0036's — a builtin is added when a promise needs it.
	for {
		dot, ok := p.accept(tokenDot)
		if !ok {
			return base, nil
		}
		name, ferr := p.expect(tokenIdent)
		if ferr != nil {
			return nil, errorf(p.src, dot.pos, "a field name was expected after the dot")
		}
		if err := p.count(dot.pos); err != nil {
			return nil, err
		}
		base = &memberNode{pos: name.pos, recv: base, name: name.text}
	}
}

func (p *parser) parseOperand() (node, *Error) {
	t := p.peek()
	switch t.kind {
	case tokenInt, tokenFloat, tokenString, tokenTrue, tokenFalse:
		p.take()
		if err := p.count(t.pos); err != nil {
			return nil, err
		}
		return &literalNode{pos: t.pos, value: t.value}, nil

	case tokenMinus:
		p.take()
		num := p.peek()
		if num.kind != tokenInt && num.kind != tokenFloat {
			return nil, errorf(p.src, t.pos, "a minus sign is only a negative number here, and %s follows it", describe(num))
		}
		p.take()
		if err := p.count(t.pos); err != nil {
			return nil, err
		}
		return &literalNode{pos: t.pos, value: negate(num.value)}, nil

	case tokenLParen:
		p.take()
		inner, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tokenRParen); err != nil {
			return nil, err
		}
		return inner, nil

	case tokenIdent:
		if p.tokens[p.at+1].kind == tokenLParen {
			return p.parseCall()
		}
		p.take()
		if err := p.count(t.pos); err != nil {
			return nil, err
		}
		if p.isBound(t.text) {
			return &varNode{pos: t.pos, name: t.text}, nil
		}
		if p.ns.has(t.text) {
			return &rootNode{pos: t.pos, name: t.text}, nil
		}
		return nil, p.unknownName(t)
	}
	return nil, errorf(p.src, t.pos, "a value was expected here and %s was found", describe(t))
}

func negate(v any) any {
	switch n := v.(type) {
	case int64:
		return -n
	case float64:
		return -n
	}
	return v
}

// unknownName is the message an author sees most, so it says what this
// namespace does have rather than only what it does not.
func (p *parser) unknownName(t token) *Error {
	if why, refused := reserved[t.text]; refused {
		return errorf(p.src, t.pos, "%s", why)
	}
	if other := namespaceOwning(t.text); other != nil && other.Name != p.ns.Name {
		return errorf(p.src, t.pos, "%s is read by %s and not by %s, which reads %s",
			t.text, other.Name, p.ns.Name, p.ns.rootList())
	}
	if _, ok := builtins[t.text]; ok {
		return errorf(p.src, t.pos, "%s is a function and wants arguments; write %s(…)", t.text, t.text)
	}
	return errorf(p.src, t.pos, "nothing here is called %s; %s reads %s", t.text, p.ns.Name, p.ns.rootList())
}

func namespaceOwning(root string) *Namespace {
	for _, ns := range []*Namespace{&HoldWhen, &Extractor} {
		if ns.has(root) {
			return ns
		}
	}
	return nil
}

func (p *parser) isBound(name string) bool {
	for i := len(p.bound) - 1; i >= 0; i-- {
		if p.bound[i] == name {
			return true
		}
	}
	return false
}

func (p *parser) parseCall() (node, *Error) {
	name := p.take()
	if _, err := p.expect(tokenLParen); err != nil {
		return nil, err
	}

	if why, refused := reserved[name.text]; refused {
		return nil, errorf(p.src, name.pos, "%s", why)
	}
	if name.text == priorName {
		return p.parsePrior(name)
	}

	fn, known := builtins[name.text]
	if !known {
		return nil, errorf(p.src, name.pos, "there is no function called %s; this language has %s, and a builtin is added when a promise needs one rather than in advance",
			name.text, builtinList())
	}
	var args []node
	if _, closed := p.accept(tokenRParen); !closed {
		for {
			arg, err := p.parseArg(fn, len(args))
			if err != nil {
				return nil, err
			}
			args = append(args, arg)
			if _, more := p.accept(tokenComma); more {
				continue
			}
			if _, err := p.expect(tokenRParen); err != nil {
				return nil, err
			}
			break
		}
	}
	if len(args) != fn.arity {
		return nil, errorf(p.src, name.pos, "%s takes %s and was given %d", name.text, plural(fn.arity, "argument"), len(args))
	}
	if fn.lambda >= 0 {
		if _, ok := args[fn.lambda].(*lambdaNode); !ok {
			return nil, errorf(p.src, args[fn.lambda].position(), "%s wants a lambda here, written x => …", name.text)
		}
	}
	if err := p.count(name.pos); err != nil {
		return nil, err
	}
	call := &callNode{pos: name.pos, name: name.text, args: args, fn: fn}
	if name.text == "find_all" {
		if lit, ok := args[1].(*literalNode); ok {
			text, isText := lit.value.(string)
			if !isText {
				return nil, errorf(p.src, lit.pos, "find_all's second argument is a pattern written as text, and this is %s", kindOf(lit.value))
			}
			re, err := regexp.Compile(text)
			if err != nil {
				return nil, errorf(p.src, lit.pos, "this pattern will not compile: %s", err)
			}
			call.re = re
		}
	}
	return call, nil
}

func plural(n int, thing string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", thing)
	}
	return fmt.Sprintf("%d %ss", n, thing)
}

// parseArg parses one argument, which is a lambda exactly where the builtin
// declares one and an ordinary expression everywhere else. A lambda has no
// meaning on its own — there is nothing to apply it to — so it is a shape of
// the call rather than a value the language has.
func (p *parser) parseArg(fn *builtin, index int) (node, *Error) {
	if fn.lambda == index && p.peek().kind == tokenIdent && p.tokens[p.at+1].kind == tokenArrow {
		param := p.take()
		arrow := p.take()
		if err := p.reserveParam(param); err != nil {
			return nil, err
		}
		p.bound = append(p.bound, param.text)
		body, err := p.parseExpr()
		p.bound = p.bound[:len(p.bound)-1]
		if err != nil {
			return nil, err
		}
		if err := p.count(arrow.pos); err != nil {
			return nil, err
		}
		return &lambdaNode{pos: param.pos, param: param.text, body: body}, nil
	}
	if p.peek().kind == tokenIdent && p.tokens[p.at+1].kind == tokenArrow {
		return nil, errorf(p.src, p.peek().pos, "a lambda means nothing in this position; %s does not fold over an argument here", fn.name)
	}
	return p.parseExpr()
}

// reserveParam refuses a lambda parameter that would shadow a root or a
// function. Shadowing obs inside a fold would make an expression read as if it
// still saw the observation, which is the kind of wrongness that looks correct.
func (p *parser) reserveParam(param token) *Error {
	switch {
	case p.ns.has(param.text):
		return errorf(p.src, param.pos, "%s is what this namespace reads and cannot also be a lambda's name", param.text)
	case param.text == priorName:
		return errorf(p.src, param.pos, "prior is a function and cannot also be a lambda's name")
	}
	if _, ok := builtins[param.text]; ok {
		return errorf(p.src, param.pos, "%s is a function and cannot also be a lambda's name", param.text)
	}
	return nil
}

// parsePrior reads prior(<name>). The argument is a name and not an
// expression: prior() reaches into the history of a *named* gatherer, and an
// expression there would evaluate to that gatherer's current data, which is the
// one thing prior() is not about.
func (p *parser) parsePrior(name token) (node, *Error) {
	if !p.ns.prior {
		return nil, errorf(p.src, name.pos, "prior() reads the previous observation and %s has none; there is no history of a program's output", p.ns.Name)
	}
	arg := p.peek()
	if arg.kind != tokenIdent {
		return nil, errorf(p.src, arg.pos, "prior() names one of this artifact's gatherers, so it wants a name and %s was found", describe(arg))
	}
	p.take()
	if next := p.peek(); next.kind == tokenComma {
		return nil, errorf(p.src, next.pos, "prior() reaches back exactly one observation and takes no second argument; a window needs a retention rule and an ordering rule, and nothing has forced either yet")
	}
	if next := p.peek(); next.kind == tokenDot {
		return nil, errorf(p.src, next.pos, "prior() names a gatherer of this artifact, not a path; reading another artifact's truth is refused, and what it would take is open question 1 in docs/open-questions.md")
	}
	if _, err := p.expect(tokenRParen); err != nil {
		return nil, err
	}
	if err := p.count(name.pos); err != nil {
		return nil, err
	}
	return &priorNode{pos: name.pos, name: arg.text}, nil
}

const priorName = "prior"

// reserved are names this language refuses on purpose, each with the reason.
// They are refused rather than merely unknown so that an author who reaches for
// one is told why it is not there and where the question lives.
var reserved = map[string]string{
	"truth":    "a predicate may not read another artifact's truth: evaluation order and cycles are real and nothing needs it yet. See open question 1 in docs/open-questions.md and decision 0036",
	"standing": "a predicate may not read standing. Standing is created by ratification and only by a person, so a predicate that could read it would be one step from a rule that mints it. See docs/decision/0007",
	"fact":     "a predicate may not read a fact. A fact is a record of an observation then, not evidence of the world now; ratify it and it becomes canon. See AGENTS.md",
}

func builtinList() string {
	names := make([]string, 0, len(builtins)+1)
	for name := range builtins {
		names = append(names, name)
	}
	names = append(names, priorName)
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// The tree. Every node knows where it is, because every message this package
// produces is rendered against the source.
type node interface{ position() Position }

type literalNode struct {
	pos   Position
	value any
}

type rootNode struct {
	pos  Position
	name string
}

type varNode struct {
	pos  Position
	name string
}

type memberNode struct {
	pos  Position
	recv node
	name string
}

type callNode struct {
	pos  Position
	name string
	args []node
	fn   *builtin
	// re is find_all's pattern when it was written as a literal, compiled
	// once here so that a pattern which will not compile is refused when the
	// corpus loads rather than when a verdict is being decided — and so that a
	// find_all inside a fold does not recompile it per element. It is written
	// during the parse and read-only afterwards, which is what keeps an Expr
	// safe to evaluate concurrently.
	re *regexp.Regexp
}

type lambdaNode struct {
	pos   Position
	param string
	body  node
}

type priorNode struct {
	pos  Position
	name string
}

type binaryNode struct {
	pos         Position
	op          tokenKind
	left, right node
}

type notNode struct {
	pos     Position
	operand node
}

func (n *literalNode) position() Position { return n.pos }
func (n *rootNode) position() Position    { return n.pos }
func (n *varNode) position() Position     { return n.pos }
func (n *memberNode) position() Position  { return n.pos }
func (n *callNode) position() Position    { return n.pos }
func (n *lambdaNode) position() Position  { return n.pos }
func (n *priorNode) position() Position   { return n.pos }
func (n *binaryNode) position() Position  { return n.pos }
func (n *notNode) position() Position     { return n.pos }

// walk visits every node once. It is how references are collected and how the
// tree is measured; nothing evaluates through it.
func walk(n node, visit func(node)) {
	if n == nil {
		return
	}
	visit(n)
	switch t := n.(type) {
	case *memberNode:
		walk(t.recv, visit)
	case *callNode:
		for _, a := range t.args {
			walk(a, visit)
		}
	case *lambdaNode:
		walk(t.body, visit)
	case *binaryNode:
		walk(t.left, visit)
		walk(t.right, visit)
	case *notNode:
		walk(t.operand, visit)
	}
}
