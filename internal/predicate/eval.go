package predicate

import (
	"fmt"

	"github.com/pbleisch/kept/internal/model"
)

// Scope is what an expression's roots resolve to at evaluation time. The tree
// holds none of this, so one parsed expression serves every world it is asked
// about.
type Scope struct {
	// Roots are the namespace's roots by name: obs for a predicate, and
	// stdout, stderr and exit for an extractor. A root the scope does not hold
	// evaluates to Unknown rather than to an error — nobody measured is the
	// honest answer for a namespace that was assembled without it, and a
	// machine that could not measure does not record what it thinks it saw.
	Roots map[string]any
	// Prior resolves prior(<name>) to the previous observation's data, or to
	// an Unknown when there is no history. A nil Prior means the scope was
	// built without one, and every prior() in the expression is Unknown.
	//
	// [Observed] builds one that folds within a world lineage, which is the
	// part that is silently wrong if it is omitted. See docs/decision/0014.
	Prior func(name string) any
}

// Verdict is what a hold_when said about one world.
type Verdict struct {
	Value model.Value
	// Because is set when Value is Unknown: what the expression could not
	// read, in the author's own terms. It is what a truth record's Detail
	// carries, so `kept why` can name the field nobody answered for.
	Because string
	// Pos is where in the expression the unmeasured thing was read.
	Pos Position
}

// Hold evaluates a predicate to the three-valued verdict.
//
// The two failures go to different places and this is the signature that keeps
// them apart. A gatherer that did not report a field produces Unknown, which is
// a verdict; an expression that is wrong produces an [*Error], which is not a
// verdict at all and must never be recorded as one. Nothing here reads a
// message to tell them apart.
func (e *Expr) Hold(sc Scope) (Verdict, error) {
	v, err := e.Eval(sc)
	if err != nil {
		return Verdict{}, err
	}
	switch t := v.(type) {
	case Unknown:
		return Verdict{Value: model.Unknown, Because: t.Because, Pos: t.Pos}, nil
	case bool:
		if t {
			return Verdict{Value: model.Kept}, nil
		}
		return Verdict{Value: model.NotKept}, nil
	}
	return Verdict{}, errorf(e.Src, e.root.position(), "a predicate says whether something holds, so it must be true or false, and this is %s", kindOf(v))
}

// Extract evaluates one of a gatherer's declared fields.
//
// The second result is whether anything was measured. An extractor whose
// expression is Unknown yields no field rather than a field holding an unknown:
// the observation then carries what was measured and nothing else, and a
// predicate reading the absent field gets Unknown for the same reason it would
// have. That is the one place the two jobs differ, and it is a difference in
// what the caller does with the value rather than in the language. See
// docs/decision/0023 and 0036.
func (e *Expr) Extract(sc Scope) (any, bool, error) {
	v, err := e.Eval(sc)
	if err != nil {
		return nil, false, err
	}
	if isUnknown(v) {
		return nil, false, nil
	}
	return v, true, nil
}

// Eval evaluates the expression to a value, which may be [Unknown]. The error
// is always an [*Error].
func (e *Expr) Eval(sc Scope) (any, error) {
	ev := &evaluator{src: e.Src, scope: sc}
	v, err := ev.eval(e.root)
	if err != nil {
		return nil, err
	}
	return v, nil
}

type evaluator struct {
	src   string
	scope Scope
	steps int
	// frames are the lambda bindings in scope, innermost last.
	frames []frame
}

type frame struct {
	name  string
	value any
}

func (ev *evaluator) errorf(pos Position, format string, args ...any) *Error {
	return errorf(ev.src, pos, format, args...)
}

// step charges one unit of work. It bounds a fold the way MaxNodes bounds the
// text: nested folds over a large observation are polynomial in the data, and
// no limit on the source can see that.
func (ev *evaluator) step(pos Position) *Error {
	ev.steps++
	if ev.steps > MaxSteps {
		return ev.errorf(pos, "evaluating this took more than %d steps, which is more than a predicate should need", MaxSteps)
	}
	return nil
}

func (ev *evaluator) eval(n node) (any, *Error) {
	if err := ev.step(n.position()); err != nil {
		return nil, err
	}
	switch t := n.(type) {
	case *literalNode:
		return t.value, nil

	case *rootNode:
		v, ok := ev.scope.Roots[t.name]
		if !ok {
			return unknownAt(t.pos, "%s: nothing was gathered", t.name), nil
		}
		return adopt(v), nil

	case *varNode:
		for i := len(ev.frames) - 1; i >= 0; i-- {
			if ev.frames[i].name == t.name {
				return ev.frames[i].value, nil
			}
		}
		return nil, ev.errorf(t.pos, "%s is not bound here", t.name)

	case *priorNode:
		if ev.scope.Prior == nil {
			return unknownAt(t.pos, "prior(%s): no history was offered", t.name), nil
		}
		return adopt(ev.scope.Prior(t.name)), nil

	case *memberNode:
		return ev.member(t)

	case *callNode:
		return ev.call(t)

	case *notNode:
		v, err := ev.eval(t.operand)
		if err != nil {
			return nil, err
		}
		if isUnknown(v) {
			return v, nil
		}
		b, ok := v.(bool)
		if !ok {
			return nil, ev.errorf(t.pos, "not wants a true/false and got %s", kindOf(v))
		}
		return !b, nil

	case *binaryNode:
		return ev.binary(t)

	case *lambdaNode:
		return nil, ev.errorf(t.pos, "a lambda is not a value; it only means something as an argument to a fold")
	}
	return nil, ev.errorf(n.position(), "this expression cannot be evaluated")
}

// member is where a missing field becomes Unknown and a field read off
// something that has none becomes an Error. The whole language exists for this
// line: the two are decided here, where the failure occurs, and they are two Go
// types from here on. See docs/decision/0036.
func (ev *evaluator) member(n *memberNode) (any, *Error) {
	recv, err := ev.eval(n.recv)
	if err != nil {
		return nil, err
	}
	if isUnknown(recv) {
		// The deepest unmeasured thing is the useful one, so the reason
		// travels outward unchanged rather than being wrapped at each step.
		return recv, nil
	}
	m, ok := recv.(map[string]any)
	if !ok {
		return nil, ev.errorf(n.pos, "%s is %s and has no fields, so .%s reads nothing", pathOf(n.recv), kindOf(recv), n.name)
	}
	v, present := m[n.name]
	if !present {
		return unknownAt(n.pos, "%s", pathOf(n)), nil
	}
	return adopt(v), nil
}

func (ev *evaluator) binary(n *binaryNode) (any, *Error) {
	left, err := ev.eval(n.left)
	if err != nil {
		return nil, err
	}
	right, err := ev.eval(n.right)
	if err != nil {
		return nil, err
	}

	switch n.op {
	case tokenAnd, tokenOr:
		// Both sides are evaluated, and then the table is applied. Stopping at
		// the first operand would give the right answer for `false and
		// unknown` only when the author happened to write the false one first,
		// and the two rows that must not collapse — `false and unknown` is
		// false, `true and unknown` is unknown — are exactly the ones that
		// order would decide.
		if err := ev.boolean(n.left.position(), n.op, left); err != nil {
			return nil, err
		}
		if err := ev.boolean(n.right.position(), n.op, right); err != nil {
			return nil, err
		}
		if n.op == tokenAnd {
			return andValues(left, right), nil
		}
		return orValues(left, right), nil

	case tokenIn:
		return ev.contains(n, left, right)
	}
	return ev.compare(n, left, right)
}

func (ev *evaluator) boolean(pos Position, op tokenKind, v any) *Error {
	if isUnknown(v) {
		return nil
	}
	if _, ok := v.(bool); !ok {
		return ev.errorf(pos, "%s joins two true/falses and one side is %s", op, kindOf(v))
	}
	return nil
}

// compare refuses a mismatch rather than answering false for it. The author
// asserted that these two things are comparable, and a promise that quietly
// says `not-kept` because a gatherer returned "0" where the predicate expected
// 0 is the report the three-valued verdict exists to prevent — arriving one
// layer further in.
func (ev *evaluator) compare(n *binaryNode, left, right any) (any, *Error) {
	if isUnknown(left) {
		return left, nil
	}
	if isUnknown(right) {
		return right, nil
	}

	if ln, lok := numeric(left); lok {
		if rn, rok := numeric(right); rok {
			return compareNumbers(n.op, ln, rn), nil
		}
		return nil, ev.errorf(n.pos, "%s compares a number with %s", n.op, kindOf(right))
	}

	switch n.op {
	case tokenEQ, tokenNE:
		same, ok := equal(left, right)
		if !ok {
			return nil, ev.errorf(n.pos, "%s cannot be compared with %s", kindOf(left), kindOf(right))
		}
		if n.op == tokenNE {
			return !same, nil
		}
		return same, nil
	}
	return nil, ev.errorf(n.pos, "%s orders numbers, and this compares %s with %s", n.op, kindOf(left), kindOf(right))
}

func numeric(v any) (float64, bool) {
	switch t := v.(type) {
	case int64:
		return float64(t), true
	case float64:
		return t, true
	}
	return 0, false
}

func compareNumbers(op tokenKind, a, b float64) bool {
	switch op {
	case tokenEQ:
		return a == b
	case tokenNE:
		return a != b
	case tokenLT:
		return a < b
	case tokenLE:
		return a <= b
	case tokenGT:
		return a > b
	}
	return a >= b
}

// equal answers for the values a comparison can be written about. The second
// result is whether the two are comparable at all; false means the expression
// is wrong rather than that the answer is no.
func equal(a, b any) (bool, bool) {
	switch l := a.(type) {
	case bool:
		r, ok := b.(bool)
		return ok && l == r, ok
	case string:
		r, ok := b.(string)
		return ok && l == r, ok
	}
	return false, false
}

// contains is set membership. Unlike a comparison it does not refuse a type
// mismatch between the needle and an element, and the asymmetry is deliberate:
// a comparison is two things the author wrote and asserted were comparable,
// where a collection's elements are whatever a gatherer returned. An element of
// another type is not this one; it is not a mistake in the expression.
func (ev *evaluator) contains(n *binaryNode, needle, haystack any) (any, *Error) {
	if isUnknown(haystack) {
		return haystack, nil
	}
	if isUnknown(needle) {
		return needle, nil
	}
	list, ok := haystack.([]any)
	if !ok {
		return nil, ev.errorf(n.pos, "in asks whether a list holds something, and the right side is %s", kindOf(haystack))
	}
	for _, item := range list {
		if err := ev.step(n.pos); err != nil {
			return nil, err
		}
		if isUnknown(item) {
			// An element nobody measured could be the one being asked about.
			return item, nil
		}
		if ln, lok := numeric(needle); lok {
			if rn, rok := numeric(item); rok && ln == rn {
				return true, nil
			}
			continue
		}
		if same, comparable := equal(needle, item); comparable && same {
			return true, nil
		}
	}
	return false, nil
}

// pathOf renders a reference the way the author wrote it, so a message names
// obs.tests.coverage rather than a node. An expression that is not a plain
// reference renders as itself being unnameable, which is what a message about
// it should say.
func pathOf(n node) string {
	switch t := n.(type) {
	case *rootNode:
		return t.name
	case *varNode:
		return t.name
	case *priorNode:
		return fmt.Sprintf("prior(%s)", t.name)
	case *memberNode:
		base := pathOf(t.recv)
		if base == "" {
			return ""
		}
		return base + "." + t.name
	case *callNode:
		return t.name + "(…)"
	}
	return ""
}

// bind evaluates a lambda body against one element. The frame stack is the
// whole of this language's scoping.
func (ev *evaluator) bind(l *lambdaNode, value any) (any, *Error) {
	ev.frames = append(ev.frames, frame{name: l.param, value: value})
	v, err := ev.eval(l.body)
	ev.frames = ev.frames[:len(ev.frames)-1]
	return v, err
}

func (ev *evaluator) call(n *callNode) (any, *Error) {
	return n.fn.call(ev, n)
}

// describeCall is the prefix a builtin's message wants: the function's own name
// and the argument that disappointed it.
func describeCall(name string, index int) string {
	return fmt.Sprintf("%s's %s argument", name, ordinal(index+1))
}

func ordinal(n int) string {
	switch n {
	case 1:
		return "first"
	case 2:
		return "second"
	case 3:
		return "third"
	}
	return fmt.Sprintf("%dth", n)
}
