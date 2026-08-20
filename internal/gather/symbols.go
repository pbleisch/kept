package gather

import (
	"context"
	"sort"

	"github.com/pbleisch/kept/internal/ledger"
)

// symbols observes what the packages in scope reference and what they declare,
// off the same parse [imports] uses.
//
// It exists because a byte scan is provably the wrong instrument for the
// questions it answers, and the evidence is in docs/decision/0049: the promise
// *the core carries no domain vocabulary*, scanned with a pattern over the
// files in scope, found seventeen hits and every one of them was a doc comment.
// The claim is true of the code and false of the bytes. Asking the AST which
// identifiers a package references answers about code, so a comment saying
// "tests@2" is not a hit and a package named in a string literal is not an
// import.
//
// The AST is already in hand — this is one parse with a second query — which is
// why two promises move from *pattern, assessor must pass* to *structural,
// auto-land* for the cost of a second walk of a tree already read.
type symbols struct{}

func (symbols) Name() string { return "go/symbols@1" }

// Reads nothing. What a package references is a function of the tree it was
// parsed from.
func (symbols) Reads() []ledger.Kind { return nil }

// The fields symbols declares. A promise may read these and no others.
var symbolFields = []string{
	// ok is whether at least one package was parsed. A scope matching no Go
	// file is **not** ok: a promise about what the core does not name must not
	// report that it names nothing when nothing was read.
	"ok",
	// packages is the in-module package paths that were parsed, sorted. It is
	// what a promise counts against when it means "and something was
	// measured", the way tests.packages is.
	"packages",
	// refs is one record per distinct qualified reference: {package, path,
	// name}, where package is the package making the reference, path is the
	// import path it goes through, and name is the identifier selected off it.
	// os.WriteFile in internal/gather is {"…/internal/gather", "os",
	// "WriteFile"}. This is what "gatherers do not write" folds over.
	"refs",
	// defines is one record per distinct top-level declaration: {package,
	// name, kind}, kind being func, method, type, const or var. This is what
	// "the core carries no domain vocabulary" folds over — the vocabulary a
	// package declares, rather than the words its file happens to contain.
	"defines",
}

func (symbols) Declares(args map[string]any) ([]string, error) {
	if err := refuseUnknownArgs(args, argScope, argTests); err != nil {
		return nil, err
	}
	if _, err := wantTests(args); err != nil {
		return nil, err
	}
	return symbolFields, nil
}

func (symbols) Gather(_ context.Context, req Request) (Fields, error) {
	withTests, err := wantTests(req.Args)
	if err != nil {
		return nil, err
	}
	src, err := readSource(req, withTests)
	if err != nil {
		return nil, err
	}
	return foldSymbols(src), nil
}

// foldSymbols turns the parsed module into the declared fields.
//
// Everything is sorted, for the reason every fold here is: an observation is
// addressed by its data, and an order out of map iteration would mint a fresh
// record for an unchanging world. See docs/decision/0034.
func foldSymbols(src *source) Fields {
	paths := src.paths()

	refs := make([]any, 0, len(paths))
	defines := make([]any, 0, len(paths))

	for _, full := range paths {
		p := src.packages[full]

		made := make([]qualified, 0, len(p.refs))
		for q := range p.refs {
			made = append(made, q)
		}
		sort.Slice(made, func(i, j int) bool {
			if made[i].path != made[j].path {
				return made[i].path < made[j].path
			}
			return made[i].name < made[j].name
		})
		for _, q := range made {
			refs = append(refs, map[string]any{"package": full, "path": q.path, "name": q.name})
		}

		declared := make([]defined, 0, len(p.defines))
		for d := range p.defines {
			declared = append(declared, d)
		}
		sort.Slice(declared, func(i, j int) bool {
			if declared[i].name != declared[j].name {
				return declared[i].name < declared[j].name
			}
			return declared[i].kind < declared[j].kind
		})
		for _, d := range declared {
			defines = append(defines, map[string]any{"package": full, "name": d.name, "kind": d.kind})
		}
	}

	return Fields{
		"ok":       len(paths) > 0,
		"packages": asList(paths),
		"refs":     refs,
		"defines":  defines,
	}
}
