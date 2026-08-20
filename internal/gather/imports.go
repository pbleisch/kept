package gather

import (
	"context"
	"sort"

	"github.com/pbleisch/kept/internal/ledger"
)

// imports observes the in-module package graph, read from source.
//
// It parses with go/parser and never shells out to `go list`, and the reason is
// not that a toolchain dependency is expensive. **It answers on a tree that
// does not compile**, which is exactly the tree a repair agent is standing in
// and therefore the tree the gate re-derives from. A boundary promise written
// over `go list` reports `unknown` there at best; the version that started
// docs/decision/0049 reported `kept`, because breaking the boundary created an
// import cycle, `go list` exited non-zero, and an empty list contains nothing.
//
// Its fields are declared in code, because kept decides this output shape —
// docs/decision/0043's rule, and the reason a promise reading obs.imports.deps
// is refused when the corpus loads rather than when a verdict is being decided.
type imports struct{}

func (imports) Name() string { return "go/imports@1" }

// Reads nothing. A package graph is a function of the tree it was parsed from.
func (imports) Reads() []ledger.Kind { return nil }

// The fields imports declares. A promise may read these and no others.
var importFields = []string{
	// ok is whether at least one package was parsed. A scope matching no Go
	// file is **not** ok: nothing was demonstrated, and a promise about a
	// boundary between packages that were never read must not report that the
	// boundary holds.
	"ok",
	// packages is one record per package: package, dir, imports, deps.
	// `imports` is what it names directly, in-module and out; `deps` is the
	// transitive closure over in-module edges, and holds in-module packages
	// only, because nothing outside the module was parsed and a closure that
	// stopped silently at its edge would be a closure nobody could read.
	"packages",
	// edges is the in-module graph flattened: one {from, to} per direct edge
	// between two packages of this module. It is what "internal/corpus does
	// not reach internal/ledger" folds over when the promise is about one hop
	// rather than about reachability.
	"edges",
	// external is every out-of-module import path any package in scope names,
	// sorted and deduplicated. A promise about what a module depends on is
	// written against this, and docs/dependencies.md is what it is checked
	// against.
	"external",
}

func (imports) Declares(args map[string]any) ([]string, error) {
	if err := refuseUnknownArgs(args, argScope, argTests); err != nil {
		return nil, err
	}
	if _, err := wantTests(args); err != nil {
		return nil, err
	}
	return importFields, nil
}

func (imports) Gather(_ context.Context, req Request) (Fields, error) {
	withTests, err := wantTests(req.Args)
	if err != nil {
		return nil, err
	}
	src, err := readSource(req, withTests)
	if err != nil {
		return nil, err
	}
	return foldImports(src), nil
}

// foldImports turns the parsed module into the declared fields.
//
// Everything is sorted, and that is not tidiness: an observation is addressed
// by its data, so an order that came from map iteration would give one
// measurement a different address every time it was made and dedup would
// quietly stop working. See docs/decision/0034.
func foldImports(src *source) Fields {
	paths := src.paths()

	records := make([]any, 0, len(paths))
	edges := make([]any, 0, len(paths))
	external := map[string]bool{}

	for _, full := range paths {
		p := src.packages[full]

		direct := sortedSet(p.imports)
		for _, target := range direct {
			if src.inModule(target) {
				edges = append(edges, map[string]any{"from": full, "to": target})
				continue
			}
			external[target] = true
		}

		records = append(records, map[string]any{
			"package": full,
			"dir":     p.dir,
			"imports": asList(direct),
			"deps":    asList(reaches(src, full)),
		})
	}

	sort.Slice(edges, func(i, j int) bool {
		a, b := edges[i].(map[string]any), edges[j].(map[string]any)
		if a["from"] != b["from"] {
			return a["from"].(string) < b["from"].(string)
		}
		return a["to"].(string) < b["to"].(string)
	})

	return Fields{
		"ok":       len(paths) > 0,
		"packages": records,
		"edges":    edges,
		"external": asList(sortedSet(external)),
	}
}

// reaches is the transitive closure of one package's in-module imports.
//
// A package that is not in scope is a package that was not parsed, so the walk
// stops there rather than guessing: the closure covers what was measured, and
// a promise whose scope excludes half the module gets an answer about the half
// it named. A cycle terminates, because a package already seen is not walked
// again — and a cycle is exactly the tree `go list` cannot answer in.
func reaches(src *source, from string) []string {
	seen := map[string]bool{}
	var walk func(string)
	walk = func(at string) {
		p, parsed := src.packages[at]
		if !parsed {
			return
		}
		for _, target := range sortedSet(p.imports) {
			if !src.inModule(target) || seen[target] {
				continue
			}
			seen[target] = true
			walk(target)
		}
	}
	walk(from)
	// A package reaching itself through a cycle is a fact about the graph and
	// is reported; the promise that cares says so.
	return sortedSet(seen)
}

func sortedSet(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// asList is a sorted list as an observation carries it. It is never nil,
// because an absent field and an empty one mean different things to a
// predicate: absent is nobody measured, and empty is measured and there were
// none.
func asList(items []string) []any {
	out := make([]any, len(items))
	for i, item := range items {
		out[i] = item
	}
	return out
}
