package predicate

import (
	"sort"
	"strings"

	"github.com/pbleisch/kept/internal/model"
)

// Sources is what a static check knows about the observations an expression may
// read: which names the artifact declares a gatherer for, and what each of
// those gatherers says it returns.
//
// The second half is separate because it arrives later. 0023 makes a gatherer
// declare its output, and 0036 points out that the declaration is worth more
// than 0023 claims — it lets every obs.<name>.<field> be checked against the
// gatherer bound to <name> when the corpus loads, and a promise reading a field
// nothing produces refused with the field, the gatherer, and what the gatherer
// does declare all named. Until a gatherer declares anything, Fields answers
// that it does not know, and the field half of the check does not run.
//
// The name half runs today and needs nothing: a promise reading obs.tests when
// it declares no gatherer called tests is reading something that will never
// arrive, whatever gatherers turn out to declare.
type Sources interface {
	// Declared reports whether the artifact names a gatherer this way.
	Declared(name string) bool
	// Fields returns the fields that gatherer declares it returns. The second
	// result is whether the declaration is known at all; false means the field
	// half of the check is skipped rather than that the gatherer returns
	// nothing.
	Fields(name string) ([]string, bool)
}

// Gathered is the [Sources] an artifact is on its own: the names its gather map
// declares. It answers the name half of the check and defers the field half,
// which is where a gatherer catalogue plugs in.
type Gathered map[string]model.GatherSpec

func (g Gathered) Declared(name string) bool {
	_, ok := g[name]
	return ok
}

func (g Gathered) Fields(string) ([]string, bool) { return nil, false }

// Names returns what the artifact does declare, for a message that says what is
// available rather than only what is not.
//
// It is not on [Sources], because a source that cannot enumerate its names is
// still a usable source — the check runs, and only the "it gathers …" half of
// the refusal is dropped. Anything that can enumerate them says so by having
// this method, which is how a catalogue-backed source keeps the message
// without the interface growing a mandatory member for one sentence of prose.
func (g Gathered) Names() []string {
	out := make([]string, 0, len(g))
	for name := range g {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Check reports every reference the expression makes that its sources cannot
// answer. It is run when the corpus loads, which turns most of the missing-field
// problem from a runtime three-valued question into an error a person can fix.
//
// It does not remove the runtime rule and is not allowed to: a gatherer can
// return less than it declared, so Unknown remains what an absent field
// evaluates to. See docs/decision/0036.
//
// Every result is an [*Error]. Nothing has been measured, so nothing here can
// be [Unknown].
func (e *Expr) Check(src Sources) []*Error {
	if !e.NS.prior {
		// Only the predicate namespace reads observations. An extractor's
		// roots are fixed and were checked when it parsed.
		return nil
	}
	var out []*Error
	// One report per thing that is wrong, not one per mention. obs.tests and
	// obs.tests.packages are the same undeclared gatherer read twice, and an
	// author fixing it fixes one thing.
	seen := map[string]bool{}
	report := func(key string, err *Error) {
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, err)
	}

	walk(e.root, func(n node) {
		gatherer, field, pos, ok := observationRef(n)
		if !ok {
			return
		}
		if !src.Declared(gatherer) {
			report(gatherer, errorf(e.Src, pos, "%s reads an observation this artifact does not gather%s", refText(gatherer, field), declares(src)))
			return
		}
		if field == "" {
			return
		}
		fields, known := src.Fields(gatherer)
		if !known {
			return
		}
		for _, f := range fields {
			if f == field {
				return
			}
		}
		report(gatherer+"."+field, errorf(e.Src, pos, "the %s gatherer does not return %s; it returns %s", gatherer, field, list(fields)))
	})

	sort.SliceStable(out, func(i, j int) bool { return out[i].Pos.Offset < out[j].Pos.Offset })
	return out
}

// Roots is every namespace root the expression reads, sorted.
//
// It answers off the tree and never off the text, and that is the whole reason
// it is here rather than at the caller: `exit` is a root, `exit_code` is a
// different name, `"exit"` is a string, and `r.exit` is a field selected off a
// record. A caller matching a substring reports all four, and the one it is
// looking for is the first.
func (e *Expr) Roots() []string {
	seen := map[string]bool{}
	walk(e.root, func(n node) {
		if root, ok := n.(*rootNode); ok {
			seen[root.name] = true
		}
	})
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Selected is every field name the expression selects, anywhere, sorted.
//
// It is what the expression has an opinion about. `obs.tests.ok` selects `ok`,
// `count(obs.tests.packages, p => p.tests_run > 0)` selects `packages` and
// `tests_run`, and a measurement's field that appears in neither is a dimension
// this predicate cannot see move. That is what an assessment of a repair
// compares: a predicate already asserts what it reads, and an instrument
// substitution is a change in what it does not.
//
// It is a union over the whole expression rather than an answer per gatherer,
// so a promise with two instruments treats a name read on either as read on
// both. That over-approximates what is seen, which is the quiet direction: a
// check reports less rather than reporting a repair that was fine.
//
// Off the tree and never off the text, for the reason [Roots] gives.
func (e *Expr) Selected() []string {
	seen := map[string]bool{}
	walk(e.root, func(n node) {
		if member, ok := n.(*memberNode); ok {
			seen[member.name] = true
		}
	})
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Strings is every string literal the expression holds, anywhere, sorted and
// without repeats.
//
// It is what the expression names by name. `"TestX" in obs.tests.cases` names a
// test, and a reader asking which tests a promise's instrument is made of asks
// this rather than matching quotes in the text, for [Roots]'s reason: a quoted
// word inside a regular expression's pattern is part of a literal too, and the
// tree is what knows where one literal ends.
func (e *Expr) Strings() []string {
	seen := map[string]bool{}
	walk(e.root, func(n node) {
		if literal, ok := n.(*literalNode); ok {
			if text, isText := literal.value.(string); isText {
				seen[text] = true
			}
		}
	})
	out := make([]string, 0, len(seen))
	for text := range seen {
		out = append(out, text)
	}
	sort.Strings(out)
	return out
}

// Reads reports whether the expression reads one particular root.
func (e *Expr) Reads(root string) bool {
	for _, name := range e.Roots() {
		if name == root {
			return true
		}
	}
	return false
}

func refText(gatherer, field string) string {
	if field == "" {
		return "obs." + gatherer
	}
	return "obs." + gatherer + "." + field
}

func declares(src Sources) string {
	g, ok := src.(interface{ Names() []string })
	if !ok {
		return ""
	}
	names := g.Names()
	if len(names) == 0 {
		return ", and it gathers nothing at all"
	}
	return "; it gathers " + list(names)
}

func list(items []string) string {
	switch len(items) {
	case 0:
		return "nothing"
	case 1:
		return items[0]
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}

// observationRef reads a node as a reference into the obs namespace. It returns
// the gatherer's name, the field beneath it when there is one, and where the
// reference is.
//
// prior(<name>) is one of these: it reaches into the history of a gatherer this
// artifact declares, so a prior() naming one it does not is the same mistake.
func observationRef(n node) (gatherer, field string, pos Position, ok bool) {
	switch t := n.(type) {
	case *priorNode:
		return t.name, "", t.pos, true

	case *memberNode:
		switch recv := t.recv.(type) {
		case *rootNode:
			if recv.name == "obs" {
				return t.name, "", t.pos, true
			}
		case *priorNode:
			return recv.name, t.name, t.pos, true
		case *memberNode:
			if root, isRoot := recv.recv.(*rootNode); isRoot && root.name == "obs" {
				return recv.name, t.name, t.pos, true
			}
		}
	}
	return "", "", Position{}, false
}
