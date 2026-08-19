package predicate

import (
	"encoding/json"
	"regexp"
	"strings"
)

// builtin is one function of the language. There are ten, and the bound on
// adding an eleventh is 0036's: a builtin is added when a promise in this
// repository or in a manuscript needs it, and the refusal message for an
// unknown function says so. Languages accrete, and the bound is the only thing
// that stops this one.
//
// Seven of them carry extraction, which the predicate does not need, and they
// are ordinary functions in the same evaluator rather than a second language.
// See docs/decision/0023 and 0036.
type builtin struct {
	name  string
	arity int
	// lambda is the index of the argument that must be written x => …, or -1.
	// It is a shape of the call rather than a value the language has: there is
	// nothing else to apply a lambda to.
	lambda int
	call   func(ev *evaluator, n *callNode) (any, *Error)
}

var builtins = map[string]*builtin{
	"has":        {name: "has", arity: 2, lambda: -1, call: callHas},
	"all":        {name: "all", arity: 2, lambda: 1, call: callAll},
	"count":      {name: "count", arity: 2, lambda: 1, call: callCount},
	"filter":     {name: "filter", arity: 2, lambda: 1, call: callFilter},
	"map":        {name: "map", arity: 2, lambda: 1, call: callMap},
	"lines":      {name: "lines", arity: 1, lambda: -1, call: callLines},
	"json":       {name: "json", arity: 1, lambda: -1, call: callJSON},
	"json_lines": {name: "json_lines", arity: 1, lambda: -1, call: callJSONLines},
	"find_all":   {name: "find_all", arity: 2, lambda: -1, call: callFindAll},
}

// has asks whether a record carries a field. It is what makes one language do
// both jobs: a missing field is Unknown everywhere by default, which is right
// for a predicate and wrong for extraction over a heterogeneous stream, where
// `go test -json` emits package-level events with no Test key and a record
// lacking one is ordinary data.
//
// The spike found this by failing without it — the first honest extractor over
// that stream returned unknown for the whole observation — which is the reason
// to trust it. See docs/decision/0036.
func callHas(ev *evaluator, n *callNode) (any, *Error) {
	container, err := ev.eval(n.args[0])
	if err != nil {
		return nil, err
	}
	name, err := ev.eval(n.args[1])
	if err != nil {
		return nil, err
	}
	if isUnknown(name) {
		return name, nil
	}
	field, ok := name.(string)
	if !ok {
		return nil, ev.errorf(n.args[1].position(), "%s names a field, so it wants text and got %s", describeCall("has", 1), kindOf(name))
	}
	if isUnknown(container) {
		// Whether a record nobody measured has a field is not a question with
		// a false answer.
		return container, nil
	}
	m, ok := container.(map[string]any)
	if !ok {
		return nil, ev.errorf(n.args[0].position(), "%s asks what a record holds, and this is %s", describeCall("has", 0), kindOf(container))
	}
	_, present := m[field]
	return present, nil
}

// list evaluates a fold's first argument. A fold over something that is not a
// list is the expression being wrong; a fold over something nobody measured is
// not.
func (ev *evaluator) list(name string, n *callNode) ([]any, any, *Error) {
	v, err := ev.eval(n.args[0])
	if err != nil {
		return nil, nil, err
	}
	if isUnknown(v) {
		return nil, v, nil
	}
	items, ok := v.([]any)
	if !ok {
		return nil, nil, ev.errorf(n.args[0].position(), "%s needs a list, and got %s", name, kindOf(v))
	}
	return items, nil, nil
}

// callAll folds a predicate over a list, three-valued. An element the lambda
// says false about settles it; an element it cannot answer for leaves the whole
// fold unmeasured. Every element is visited rather than stopping at the first
// false, so an expression that is wrong about a later element is reported
// whichever element comes first.
//
// An empty list is true, as an empty conjunction is. A promise that also means
// "and there is at least one" says so with count().
func callAll(ev *evaluator, n *callNode) (any, *Error) {
	items, unknown, err := ev.list("all", n)
	if err != nil || unknown != nil {
		return unknown, err
	}
	lambda := n.args[1].(*lambdaNode)

	result := any(true)
	for _, item := range items {
		if err := ev.step(n.pos); err != nil {
			return nil, err
		}
		v, err := ev.bind(lambda, item)
		if err != nil {
			return nil, err
		}
		if !isUnknown(v) {
			if _, ok := v.(bool); !ok {
				return nil, ev.errorf(lambda.body.position(), "all wants a true/false for each element, and one gave %s", kindOf(v))
			}
		}
		result = andValues(result, v)
	}
	return result, nil
}

// callCount counts the elements a predicate holds for. An element it cannot
// answer for makes the count unmeasured rather than uncounted: a count that
// silently skipped what it could not decide would report a number nobody
// measured.
func callCount(ev *evaluator, n *callNode) (any, *Error) {
	items, unknown, err := ev.list("count", n)
	if err != nil || unknown != nil {
		return unknown, err
	}
	lambda := n.args[1].(*lambdaNode)

	var total int64
	for _, item := range items {
		if err := ev.step(n.pos); err != nil {
			return nil, err
		}
		v, err := ev.bind(lambda, item)
		if err != nil {
			return nil, err
		}
		if isUnknown(v) {
			return v, nil
		}
		keep, ok := v.(bool)
		if !ok {
			return nil, ev.errorf(lambda.body.position(), "count wants a true/false for each element, and one gave %s", kindOf(v))
		}
		if keep {
			total++
		}
	}
	return total, nil
}

// callFilter keeps the elements a predicate holds for. An element it cannot
// answer for makes the result unmeasured, for callCount's reason: a list that
// quietly dropped what it could not decide is not the list the author asked
// for.
func callFilter(ev *evaluator, n *callNode) (any, *Error) {
	items, unknown, err := ev.list("filter", n)
	if err != nil || unknown != nil {
		return unknown, err
	}
	lambda := n.args[1].(*lambdaNode)

	out := []any{}
	for _, item := range items {
		if err := ev.step(n.pos); err != nil {
			return nil, err
		}
		v, err := ev.bind(lambda, item)
		if err != nil {
			return nil, err
		}
		if isUnknown(v) {
			return v, nil
		}
		keep, ok := v.(bool)
		if !ok {
			return nil, ev.errorf(lambda.body.position(), "filter wants a true/false for each element, and one gave %s", kindOf(v))
		}
		if keep {
			out = append(out, adopt(item))
		}
	}
	return out, nil
}

// callMap rewrites each element. One element nobody could measure makes the
// whole list unmeasured, which is what carries an unreadable record out to the
// field it would have become: an extractor's field is then absent rather than
// present and wrong.
func callMap(ev *evaluator, n *callNode) (any, *Error) {
	items, unknown, err := ev.list("map", n)
	if err != nil || unknown != nil {
		return unknown, err
	}
	lambda := n.args[1].(*lambdaNode)

	out := make([]any, 0, len(items))
	for _, item := range items {
		if err := ev.step(n.pos); err != nil {
			return nil, err
		}
		v, err := ev.bind(lambda, item)
		if err != nil {
			return nil, err
		}
		if isUnknown(v) {
			return v, nil
		}
		out = append(out, v)
	}
	return out, nil
}

// text evaluates an argument that has to be text, which is every extraction
// builtin's first one.
func (ev *evaluator) text(name string, n *callNode, index int) (string, any, *Error) {
	v, err := ev.eval(n.args[index])
	if err != nil {
		return "", nil, err
	}
	if isUnknown(v) {
		return "", v, nil
	}
	s, ok := v.(string)
	if !ok {
		return "", nil, ev.errorf(n.args[index].position(), "%s wants text and got %s", describeCall(name, index), kindOf(v))
	}
	return s, nil, nil
}

// callLines splits text into lines. A trailing newline does not produce a final
// empty line, because a program that ends its output with one has not printed
// an extra line and an extractor counting them should not be told it did. A
// CRLF ending is stripped, because whether a tool emits one is not something a
// promise should have to know.
func callLines(ev *evaluator, n *callNode) (any, *Error) {
	s, unknown, err := ev.text("lines", n, 0)
	if err != nil || unknown != nil {
		return unknown, err
	}
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return []any{}, nil
	}
	parts := strings.Split(s, "\n")
	out := make([]any, len(parts))
	for i, p := range parts {
		out[i] = strings.TrimSuffix(p, "\r")
	}
	return out, nil
}

// callJSON decodes text a program produced. Text that is not JSON is Unknown
// and not an error: the expression asked the right question and the program did
// not answer it, and a build that fell over before printing its report must not
// make a promise not-kept. That is the invariant one layer in — a machine that
// could not measure does not record what it thinks it saw.
func callJSON(ev *evaluator, n *callNode) (any, *Error) {
	s, unknown, err := ev.text("json", n, 0)
	if err != nil || unknown != nil {
		return unknown, err
	}
	v, ok := decodeJSON(s)
	if !ok {
		return unknownAt(n.pos, "%s is not JSON", reference(n.args[0])), nil
	}
	return v, nil
}

// callJSONLines decodes one JSON value per line, which is what `go test -json`
// and most streaming tools emit. A line that is not JSON makes the whole stream
// unmeasured rather than being skipped: a test binary that panicked prints
// plain text into the middle of the stream, and an extractor that silently
// dropped those lines would report a passing count over a truncated run.
func callJSONLines(ev *evaluator, n *callNode) (any, *Error) {
	s, unknown, err := ev.text("json_lines", n, 0)
	if err != nil || unknown != nil {
		return unknown, err
	}
	out := []any{}
	for _, line := range strings.Split(s, "\n") {
		if err := ev.step(n.pos); err != nil {
			return nil, err
		}
		line = strings.TrimSuffix(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		v, ok := decodeJSON(line)
		if !ok {
			return unknownAt(n.pos, "%s carries a line that is not JSON", reference(n.args[0])), nil
		}
		out = append(out, v)
	}
	return out, nil
}

// reference names an argument the way the author wrote it, falling back to the
// builtin's own idea of what it was reading.
func reference(n node) string {
	if p := pathOf(n); p != "" {
		return p
	}
	return "this text"
}

func decodeJSON(s string) (any, bool) {
	dec := json.NewDecoder(strings.NewReader(s))
	// Numbers arrive as float64 by default, which would make a count of 3
	// compare as 3 and encode as something else again. UseNumber keeps the
	// text, and adoptJSON turns it into the language's own two numeric types.
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, false
	}
	// Trailing content means the text was not one JSON value, which is a
	// different thing from being one and is not silently accepted.
	var rest any
	if err := dec.Decode(&rest); err == nil {
		return nil, false
	}
	return adoptJSON(v), true
}

// adoptJSON turns json.Number into the language's numbers. It is separate from
// adopt because only a decoder produces one.
func adoptJSON(v any) any {
	switch t := v.(type) {
	case json.Number:
		if n, err := t.Int64(); err == nil {
			return n
		}
		if f, err := t.Float64(); err == nil {
			return f
		}
		return t.String()
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = adoptJSON(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, item := range t {
			out[k] = adoptJSON(item)
		}
		return out
	}
	return v
}

// callFindAll matches a pattern over text and returns one record per match:
// "match" is the whole of it, and every named group is under its own name. An
// unnamed group is not addressable, because reaching one would need indexing
// and this language does not have it.
//
// Patterns are Go's regexp, which cannot backtrack catastrophically. An
// extractor runs over bytes a program produced, and an unbounded one is a way
// to wedge a tick. A pattern that will not compile is an [*Error]: the author
// wrote it, so it is theirs to fix, and a literal one is refused when the
// corpus loads rather than when a verdict is being decided.
func callFindAll(ev *evaluator, n *callNode) (any, *Error) {
	s, unknown, err := ev.text("find_all", n, 0)
	if err != nil || unknown != nil {
		return unknown, err
	}
	re, err := ev.pattern(n)
	if err != nil {
		return nil, err
	}

	out := []any{}
	names := re.SubexpNames()
	for _, m := range re.FindAllStringSubmatchIndex(s, -1) {
		if err := ev.step(n.pos); err != nil {
			return nil, err
		}
		record := map[string]any{"match": group(s, m, 0)}
		for i, name := range names {
			if i == 0 || name == "" {
				continue
			}
			record[name] = group(s, m, i)
		}
		out = append(out, record)
	}
	return out, nil
}

func group(s string, m []int, i int) string {
	if 2*i+1 >= len(m) || m[2*i] < 0 {
		return ""
	}
	return s[m[2*i]:m[2*i+1]]
}

// pattern resolves find_all's second argument. A literal was compiled when the
// expression was parsed, so an authored pattern that will not compile is
// refused before anything is measured; anything else compiles here.
func (ev *evaluator) pattern(n *callNode) (*regexp.Regexp, *Error) {
	if n.re != nil {
		return n.re, nil
	}
	v, err := ev.eval(n.args[1])
	if err != nil {
		return nil, err
	}
	if isUnknown(v) {
		return nil, ev.errorf(n.args[1].position(), "%s is a pattern, and one nobody measured is not a pattern this language can run", describeCall("find_all", 1))
	}
	s, ok := v.(string)
	if !ok {
		return nil, ev.errorf(n.args[1].position(), "%s wants a pattern written as text and got %s", describeCall("find_all", 1), kindOf(v))
	}
	re, cerr := regexp.Compile(s)
	if cerr != nil {
		return nil, ev.errorf(n.args[1].position(), "this pattern will not compile: %s", cerr)
	}
	return re, nil
}
