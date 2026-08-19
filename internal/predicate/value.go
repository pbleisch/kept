package predicate

import (
	"fmt"
	"strings"
)

// Unknown is the value of an expression nobody measured: the observation
// carries no such field, prior() has no history, a list has nothing by that
// name. It is a **value**, not an error, and that is the whole shape of the
// three-valued verdict — `unknown` falls out of what an absent field evaluates
// to rather than being bolted on afterwards.
//
// It deliberately has no Error method. A caller that has to tell *nobody
// measured* from *this expression is wrong* asks Go which type it is holding;
// it never reads a message. That is the distinction the whole model rests on,
// and it is why `expr` — which evaluates a missing field to false, and would
// report `not-kept` for a gatherer that could not measure — was rejected on
// evidence. See docs/decision/0036 and the invariant in AGENTS.md: *a failed
// gatherer yields `unknown`, never `not-kept`*.
type Unknown struct {
	// Because names what was not measured, in the author's own terms —
	// "obs.tests.coverage", not "map lookup failed". It is what a truth
	// record's Detail carries, so a person reading `kept why` sees the field
	// nobody could answer for.
	Because string
	// Pos is where in the expression the unmeasured thing was read.
	Pos Position
}

func (u Unknown) String() string {
	if u.Because == "" {
		return "unknown"
	}
	return "unknown: " + u.Because
}

// unknownAt is the one place an Unknown is minted, so every one of them says
// what was missing and where.
func unknownAt(pos Position, format string, args ...any) Unknown {
	return Unknown{Because: fmt.Sprintf(format, args...), Pos: pos}
}

// isUnknown reports whether a value is the unmeasured one. Written as a
// function rather than repeated type switches so that adding a second unknown
// shape later — which would be a mistake — has one place to be caught.
func isUnknown(v any) bool {
	_, ok := v.(Unknown)
	return ok
}

// IsUnknown reports whether an evaluated value is [Unknown]. Callers outside
// this package use it rather than a type assertion so the check reads the same
// everywhere.
func IsUnknown(v any) bool { return isUnknown(v) }

// adopt maps a value from outside — a YAML or JSON decoder, a gatherer's data
// map — into the small domain this language evaluates over: bool, int64,
// float64, string, []any, map[string]any, and Unknown.
//
// It converts one level and no more. A deep conversion would be simpler to
// read and is a way to wedge a tick: obs is adopted every time an expression
// names it, so a deep adopt inside a fold copies the whole observation once per
// element, and no bound on the source or on the step count can see that. So
// every place a value is taken *out* of a container — member access, a fold's
// element, a membership scan — adopts what it took, and the containers
// themselves are handed on as they are.
//
// Nothing is dropped and nothing is guessed. A type the language has no
// operations for arrives as itself and every operation on it reports what it
// found, which is an error the author can fix rather than a silent false.
func adopt(v any) any {
	switch t := v.(type) {
	case nil:
		return nil
	case bool, string, int64, float64, Unknown, []any, map[string]any:
		return t
	case int:
		return int64(t)
	case int8:
		return int64(t)
	case int16:
		return int64(t)
	case int32:
		return int64(t)
	case uint:
		return int64(t)
	case uint8:
		return int64(t)
	case uint16:
		return int64(t)
	case uint32:
		return int64(t)
	case uint64:
		return int64(t)
	case float32:
		return float64(t)
	case []string:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = item
		}
		return out
	case map[any]any:
		// One level: the keys become strings so member access can reach them,
		// and the values are adopted when something reads one.
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[fmt.Sprint(k)] = val
		}
		return out
	}
	return v
}

// kindOf names a value the way the author would, because every message this
// package produces is read by whoever wrote the expression.
func kindOf(v any) string {
	switch t := v.(type) {
	case nil:
		return "nothing"
	case Unknown:
		return "unknown"
	case bool:
		return "a true/false"
	case int64, float64:
		return "a number"
	case string:
		return "text"
	case []any:
		return "a list"
	case map[string]any:
		return "a record"
	default:
		return fmt.Sprintf("a %T", t)
	}
}

// render is how a value appears inside a message. It is not a serialization
// format and nothing reads it back.
func render(v any) string {
	switch t := v.(type) {
	case string:
		return fmt.Sprintf("%q", t)
	case []any:
		parts := make([]string, len(t))
		for i, item := range t {
			parts[i] = render(item)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	default:
		return fmt.Sprint(v)
	}
}

// The three-valued connectives. Both are commutative in the case that matters:
// `false and unknown` is false whichever side the unknown is on, and
// `true or unknown` is true either way. A short-circuit that stopped at the
// first operand would be commutative too, but only by accident of the order the
// author wrote — so both operands are evaluated and the table is applied.
//
// The two rows that must not collapse are `false and unknown` (false: one
// conjunct is known false, so the conjunction is known false) and
// `true and unknown` (unknown: nothing decides it). Each has a test.
func andValues(a, b any) any {
	if a == false || b == false {
		return false
	}
	if isUnknown(a) {
		return a
	}
	if isUnknown(b) {
		return b
	}
	return true
}

func orValues(a, b any) any {
	if a == true || b == true {
		return true
	}
	if isUnknown(a) {
		return a
	}
	if isUnknown(b) {
		return b
	}
	return false
}
