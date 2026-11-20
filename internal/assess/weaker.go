package assess

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// weakened is every way the measurement the verdict came from says less than
// the one before it, in the dimensions the predicate cannot see.
//
// **In the dimensions the predicate cannot see** is the whole rule, and it is
// what separates an instrument substitution from an ordinary repair. A
// predicate already asserts a direction for every field it reads: a promise
// holding when `count(obs.corpus.broken_links, l => true) == 0` wants that list
// to empty, and a check reporting it emptying would fire on the repair the
// promise exists to get. What the predicate says nothing about is where a
// substitution lives — six tests become two and `all(…, p => p.ok)` is as true
// as it was, because `tests_run` is a dimension the wording never mentions.
//
// So this compares exactly the rest, and the throughline follows from it: a
// finding here is answered by naming the missing dimension in the predicate,
// after which the predicate carries it and this check goes quiet. A finding
// that repeats is a predicate nobody has written.
//
// It reports *less*, never *different*. A number that went up, a member that
// was added and a string that changed are all ordinary repair, and a check that
// fired on them would fire on every repair and be turned off within a week.
//
// It is not an assessment of the gatherer. Nothing here knows what a package or
// a test is: it walks two readings of one shape and reports what got smaller,
// which is a property of this act and stays bounded by it.
//
// blind is asked about a field, with an empty member, and about one member of
// a list, by that member's key. The second question is what lets a predicate
// that reads one member of a list — `"TestX" in obs.tests.cases` — stay blind to
// the others. A member it never named that left the measurement is reported
// beside the numbers that fell, and only then: the names say what a smaller
// count lost, and they never make a check fire on their own. A test renamed,
// or retired beside more tests added, is a measurement that moved. See
// docs/decision/0094.
func weakened(name string, blind func(field, member string) bool, before, after map[string]any) []string {
	w := weakening{blind: blind, present: presence(after)}
	w.compare(name, before, after)

	// A number that fell where another rose is a measurement that moved rather
	// than one that shrank — a repair turns failures into passes, and both are
	// dimensions a predicate about `ok` never mentions. Reporting the fall
	// without the rise would report every successful repair as a loss.
	if !w.rose && len(w.fell) > 0 {
		w.out = append(w.out, w.fell...)
		w.out = append(w.out, w.lost...)
	}
	sort.Strings(w.out)
	return w.out
}

// weakening is one comparison in progress.
type weakening struct {
	// blind reports whether a field name, or one member of a list under that
	// name, is something the predicate never selects.
	blind func(field, member string) bool
	// present is every value the after measurement holds anywhere, so a member
	// that left one collection and is still in another is not gone. A failing
	// test that stops failing leaves `failures` and stays in `cases`; a test
	// that was deleted leaves the measurement altogether, and only the second
	// is evidence of anything.
	present map[string]bool

	out  []string
	fell []string
	rose bool
	// lost is every member of a list the predicate reads only by naming other
	// members, gone from the measurement. It explains a fall and is reported
	// only with one.
	lost []string
}

func (w *weakening) compare(at string, before, after any) {
	if n, ok := number(before); ok {
		m, ok := number(after)
		if !ok || m == n || !w.blind(leaf(at), "") {
			return
		}
		if m > n {
			w.rose = true
			return
		}
		w.fell = append(w.fell, fmt.Sprintf("%s %s → %s", at, decimal(n), decimal(m)))
		return
	}
	switch left := before.(type) {
	case map[string]any:
		right, ok := after.(map[string]any)
		if !ok {
			return
		}
		for _, key := range sortedKeys(left) {
			value, held := right[key]
			if !held {
				if w.blind(key, "") {
					w.out = append(w.out, fmt.Sprintf("%s.%s is no longer measured", at, key))
				}
				continue
			}
			w.compare(at+"."+key, left[key], value)
		}
	case []any:
		right, ok := after.([]any)
		if !ok {
			return
		}
		w.members(at, left, right)
	}
}

// members correlates two readings of one list and compares what is in both.
//
// The key is every string-valued field of a member, jointly. It is that rather
// than a field this package chose, because choosing one means knowing what the
// list is about — which package, which file, which test — and knowing that is
// how a checklist turns into an assessment of the instrument. Strings are the
// right half because they are what names a member; the numbers are what is
// being compared.
//
// It correlates only when every member on both sides has a key and no key
// repeats. Otherwise the members are left alone and nothing is invented: a
// correlation that guessed would report a member as lost when two of them
// merely swapped places.
func (w *weakening) members(at string, before, after []any) {
	left, ok := keyed(before)
	if !ok {
		return
	}
	right, ok := keyed(after)
	if !ok {
		return
	}
	for _, key := range sortedKeys(left) {
		value, held := right[key]
		if held {
			w.compare(fmt.Sprintf("%s[%s]", at, key), left[key], value)
			continue
		}
		if w.present[key] {
			continue
		}
		gone := fmt.Sprintf("%s[%s] is no longer measured anywhere", at, key)
		switch {
		case w.blind(leaf(at), ""):
			w.out = append(w.out, gone)
		case w.blind(leaf(at), key):
			w.lost = append(w.lost, gone)
		}
	}
}

// presence is every member identity the measurement holds, at any depth.
//
// It is what makes *gone* mean gone from the measurement rather than gone from
// one field of it. A name that moved between two collections is still measured,
// and reporting that as a loss would report every repair that fixed something.
func presence(data map[string]any) map[string]bool {
	out := map[string]bool{}
	var walk func(value any)
	walk = func(value any) {
		switch t := value.(type) {
		case map[string]any:
			if key, ok := identify(t); ok {
				out[key] = true
			}
			for _, member := range t {
				walk(member)
			}
		case []any:
			for _, member := range t {
				walk(member)
			}
		case string:
			out[t] = true
		}
	}
	walk(data)
	return out
}

// keyed indexes a list's members by their string fields. A member that is not
// an object is keyed by its own text, so a list of names correlates too.
func keyed(list []any) (map[string]any, bool) {
	out := make(map[string]any, len(list))
	for _, member := range list {
		key, ok := identify(member)
		if !ok {
			return nil, false
		}
		if _, repeated := out[key]; repeated {
			return nil, false
		}
		out[key] = member
	}
	return out, true
}

func identify(member any) (string, bool) {
	switch value := member.(type) {
	case map[string]any:
		var parts []string
		for _, key := range sortedKeys(value) {
			if text, ok := value[key].(string); ok {
				parts = append(parts, key+"="+text)
			}
		}
		if len(parts) == 0 {
			return "", false
		}
		return strings.Join(parts, ","), true
	case string:
		return value, true
	}
	return "", false
}

// leaf is the field name at the end of a path, which is the name a predicate
// would have written to read it. `tests.packages[package=x].tests_run` is
// selected by writing `tests_run`, and the member key in the middle is not a
// name anybody writes.
func leaf(at string) string {
	if cut := strings.LastIndex(at, "."); cut >= 0 {
		at = at[cut+1:]
	}
	if cut := strings.Index(at, "["); cut >= 0 {
		at = at[:cut]
	}
	return at
}

// number reads the numeric types an observation can carry, whether it came off
// a gatherer in this process or back off the ledger through a decoder.
func number(value any) (float64, bool) {
	switch n := value.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float64:
		return n, true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	}
	return 0, false
}

// decimal prints a measured number the way it was measured: a whole number has
// no fractional part written for it.
func decimal(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
