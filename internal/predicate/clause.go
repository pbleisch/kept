package predicate

import (
	"sort"
	"strings"
)

// Clause is one top-level conjunct of a predicate: a sentence an author could
// in principle judge on its own, joined to its neighbors by and. Splitting on
// or, not, or a parenthesized group would change what the clause means when
// it is judged alone — `a or b` is not two things that both have to hold — so
// only a top-level, unparenthesized and is a boundary. See [Expr.Clauses].
type Clause struct {
	// Src is the clause exactly as authored: the same bytes the whole
	// expression carries for this span, trimmed of the whitespace that
	// separates it from its neighbors and nothing else. A line break inside
	// the clause itself survives the trim.
	Src string
	// root is the clause's own subtree. Its positions are offsets into
	// expr.Src, not into Src, because that is the source every error and
	// every reference in this package already renders against.
	root node
	// expr is the predicate this clause was cut from, kept so Hold and Reads
	// can evaluate root without copying the machinery Expr already has.
	expr *Expr
}

// Clauses splits the predicate into its top-level conjuncts, left to right.
//
// The AST does not carry parenthesization — parseOperand's parenthesized case
// hands back the inner node unwrapped, because nothing before this needed to
// tell `(a and b) and c` apart from `a and b and c`. This does, so it consults
// [Expr.parens] rather than the tree shape alone: an and-node that was itself
// the whole of a parenthesized group is a boundary the same as an or, a not,
// or a comparison would be, and is returned as one clause rather than split.
//
// A predicate with no top-level and — `a or b`, `not a`, a bare comparison —
// is one clause: itself. That is not a special case here; it falls out of
// splitAnd finding no and-node to descend through.
func (e *Expr) Clauses() []*Clause {
	leaves, boundaries := splitAnd(e.root, e.parens)
	out := make([]*Clause, len(leaves))
	for i := range leaves {
		start := 0
		if i > 0 {
			// Past the "and" keyword that ends the previous clause. The
			// keyword's own three bytes are never part of either neighbor.
			start = boundaries[i-1].Offset + len("and")
		}
		end := len(e.Src)
		if i < len(boundaries) {
			end = boundaries[i].Offset
		}
		out[i] = &Clause{
			Src:  strings.TrimSpace(e.Src[start:end]),
			root: leaves[i],
			expr: e,
		}
	}
	return out
}

// splitAnd walks the left spine of an and-chain, returning its leaves in
// source order and the offset of each "and" keyword that separates them.
//
// It only ever needs to look left. The parser's precedence ladder makes and
// left-associative — `a and b and c` builds as `(a and b) and c` — so the
// right side of every and-node is already a single operand of lower or equal
// precedence, and the only way it could itself be an unmarked and-node is if
// the author wrote one there directly, which the grammar does not allow
// without a comparison, not, or paren between them. Recursing into left is
// therefore the whole of it.
func splitAnd(n node, parens map[node]bool) (leaves []node, boundaries []Position) {
	if b, ok := n.(*binaryNode); ok && b.op == tokenAnd && !parens[b] {
		leaves, boundaries = splitAnd(b.left, parens)
		return append(leaves, b.right), append(boundaries, b.pos)
	}
	return []node{n}, nil
}

// Hold answers this clause alone, against the same scope the whole predicate
// would see. It shares [toVerdict] with [Expr.Hold] so that a clause answered
// by itself reaches exactly the verdict it would have reached in place — the
// promise this method exists to keep is that splitting a predicate into
// clauses and joining their verdicts with three-valued and reproduces the
// verdict of evaluating it whole.
func (c *Clause) Hold(sc Scope) (Verdict, error) {
	ev := &evaluator{src: c.expr.Src, scope: sc}
	v, err := ev.eval(c.root)
	if err != nil {
		return Verdict{}, err
	}
	return toVerdict(c.expr.Src, c.root.position(), v)
}

// Reading is one observation a clause read while being answered: the path it
// was reached by, in the author's own terms, and the value found there.
//
// Value may be an [Unknown]: a clause that read something nobody measured
// still names it, rather than the reading simply not appearing. That is the
// difference between "read and empty" and "not read at all", and it is the
// reason Reads returns a value for a path rather than just the path.
type Reading struct {
	Path  string
	Value any
}

// Reads reports every observation this clause read to answer itself, deduped
// by path and sorted for a stable report.
//
// Three kinds of reference are deliberately left out:
//
//   - A reference that is itself selected further, like the obs.tests in
//     obs.tests.packages, is not maximal. Only the outermost chain — the one
//     nothing else selects from — is reported, because that is the one
//     reading that actually happened; obs.tests was never asked for on its
//     own.
//   - A lambda's bound variable, like the p in `p.ok` inside a fold. It names
//     one element of something already reported under its own path, not a
//     second observation.
//   - A value derived from a call, like json(stdout).version. It was
//     computed, not read, and a clause that says what it read should not
//     claim credit for measuring what it worked out.
func (c *Clause) Reads(sc Scope) []Reading {
	consumed := map[node]bool{}
	walk(c.root, func(n node) {
		if m, ok := n.(*memberNode); ok {
			consumed[m.recv] = true
		}
	})

	ev := &evaluator{src: c.expr.Src, scope: sc}
	seen := map[string]bool{}
	var out []Reading
	walk(c.root, func(n node) {
		switch n.(type) {
		case *rootNode, *priorNode, *memberNode:
		default:
			return
		}
		if consumed[n] || rootOf(n) == nil {
			return
		}
		path := pathOf(n)
		if path == "" || seen[path] {
			return
		}
		seen[path] = true
		v, err := ev.eval(n)
		if err != nil {
			// Every reference reaching here already type-checked as a plain
			// reads-a-path expression, so this should not happen; if it
			// somehow does, it is not a reading this clause can vouch for.
			return
		}
		out = append(out, Reading{Path: path, Value: v})
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// rootOf follows a chain of member access down to what it is ultimately
// selected from, and answers nil unless that base is something a predicate
// actually reads observations through: [rootNode] (obs) or [priorNode]
// (prior(name)). A chain grounded in a [varNode] is a lambda parameter, and
// one grounded in a [callNode] is computed; neither is an observation this
// clause read.
func rootOf(n node) node {
	for {
		m, ok := n.(*memberNode)
		if !ok {
			break
		}
		n = m.recv
	}
	switch n.(type) {
	case *rootNode, *priorNode:
		return n
	}
	return nil
}
