package gather

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/pbleisch/kept/internal/ledger"
)

// tests observes a Go test suite over `go test -json`.
//
// It ships before `build` because it is the self-hosting one — the promises in
// docs/self-hosting.md are about this repository's own tests — and because it
// is the only one that exercises the fold: `go test -json` is a heterogeneous
// stream, not a table, and that heterogeneity is what forced has() into the
// predicate language (docs/decision/0036).
//
// Its fields are declared in code, which is the asymmetry docs/decision/0034
// settles: a built-in's output shape is part of the build, and `command` alone
// declares its own in the promise, because `command` is the escape hatch and
// only it can name fields kept has never seen.
//
// **Nothing timing-dependent is recorded.** `go test -json` reports an Elapsed
// on every event and none of it is here. An observation is addressed by its
// data, so a duration would mint a fresh record on every tick of an unchanging
// world, and dedup — the property the whole content-addressed layout is for —
// would quietly stop working.
type tests struct {
	// binary is what runs the suite. Empty is "go". It is a field rather than
	// a constant because a missing toolchain is one of the behaviours under
	// test, and the only honest way to test it is to point this somewhere
	// there is nothing.
	binary string
}

func (tests) Name() string { return "go/tests@1" }

// Reads nothing. A suite's result is a function of the tree it ran over, so
// the scope digest docs/decision/0033 defines is the whole of what this
// measurement depends on. See docs/decision/0046.
func (tests) Reads() []ledger.Kind { return nil }

// argPackages is the package pattern the suite runs over.
const argPackages = "packages"

// argExclude names package patterns the suite must not run at all.
//
// Go's package patterns cannot negate: there is no spelling of `./internal/...`
// minus one package, and enumerating the rest puts a list of packages in a
// promise that binds — so a package added later would be silently unmeasured,
// which is the failure `count(obs.tests.packages, p => p.ok) > 0` exists to
// refuse one level up. The exclusion belongs where the pattern is.
//
// It is an argument rather than a second gatherer, so docs/decision/0043's
// closed catalogue is untouched; and it changes nothing about what the tree
// must have, so the `go/` group docs/decision/0051 requires still says the
// whole of what this instrument needs.
//
// **An exclusion that removes nothing is refused**, and that is the guard
// rather than a nicety: a promise that names `./internal/confine` after the
// package has been renamed would otherwise start running the very suite it was
// written to keep out, and say nothing. It cannot be an authoring error caught
// when the corpus loads, because whether a pattern matches depends on the tree
// and Declares must answer without one — so it is a failure to measure, which
// is `unknown` and never `not-kept`.
const argExclude = "exclude"

// The fields tests declares. A promise may read these and no others, and a
// hold_when naming something else is refused when the corpus loads.
var testFields = []string{
	// ok is whether at least one package was found and every one of them
	// built, ran and passed. A suite of no packages is **not** ok: nothing
	// was demonstrated, and a pattern matching nothing is the ordinary way
	// that happens. It is the coarse answer, and the promises worth writing
	// are about packages — note that all(obs.tests.packages, …) is true over
	// an empty list, as an empty conjunction is, so a promise meaning "and
	// something ran" says so with count().
	"ok",
	// packages is one record per package: package, ok, tests_run, passed,
	// failed, skipped. This is what "every package under internal/ has a test
	// that runs and passes" is written against, and it is why a gatherer
	// returns structure rather than a boolean (docs/decision/0004).
	"packages",
	// passed, failed and skipped are the totals across the run.
	"passed", "failed", "skipped",
	// cases is the set of test names that ran, sorted. It is what "the set of
	// tests never shrinks" folds over with prior(), and what a promise naming
	// one particular test asserts membership in.
	"cases",
	// failures is the subset of cases that failed, sorted, so a verdict can
	// name what broke without the whole set.
	"failures",
}

func (t tests) Declares(args map[string]any) ([]string, error) {
	if err := refuseUnknownArgs(args, argScope, argPackages, argExclude); err != nil {
		return nil, err
	}
	if _, err := packagePatterns(args); err != nil {
		return nil, err
	}
	if _, err := excludedPatterns(args); err != nil {
		return nil, err
	}
	return testFields, nil
}

// packagePatterns reads what the suite runs over. `./...` is the default,
// because a promise about this repository's tests means all of them unless it
// says otherwise.
func packagePatterns(args map[string]any) ([]string, error) {
	patterns, err := patternList(args, argPackages)
	if err != nil {
		return nil, err
	}
	if len(patterns) == 0 {
		return []string{"./..."}, nil
	}
	return patterns, nil
}

// excludedPatterns reads what the suite must not run. Absent is none; there is
// no default, because excluding something nobody asked to exclude would be this
// gatherer deciding what a promise measures.
func excludedPatterns(args map[string]any) ([]string, error) {
	return patternList(args, argExclude)
}

// patternList reads one arg as a package pattern or a list of them. Absent is
// nil, and empty is refused rather than read as absent: a promise that wrote
// the key meant something by it.
func patternList(args map[string]any, arg string) ([]string, error) {
	switch t := args[arg].(type) {
	case nil:
		return nil, nil
	case string:
		if t == "" {
			return nil, fmt.Errorf("%s: is empty, so it names no package", arg)
		}
		return []string{t}, nil
	case []any:
		var out []string
		for _, item := range t {
			s, ok := item.(string)
			if !ok || s == "" {
				return nil, fmt.Errorf("%s: holds something that is not a package pattern", arg)
			}
			out = append(out, s)
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("%s: is empty, so it names no package", arg)
		}
		return out, nil
	}
	return nil, fmt.Errorf("%s: is not a package pattern or a list of them", arg)
}

func (t tests) Gather(ctx context.Context, req Request) (Fields, error) {
	patterns, err := packagePatterns(req.Args)
	if err != nil {
		return nil, err
	}
	excluded, err := excludedPatterns(req.Args)
	if err != nil {
		return nil, err
	}
	binary := t.binary
	if binary == "" {
		binary = "go"
	}

	// One deadline over the whole gather rather than one per program. An
	// exclusion runs `go list` before `go test`, and bounding each separately
	// would let a gather take a multiple of what config bounded it to. See
	// docs/decision/0026.
	ctx, cancel := context.WithTimeout(ctx, req.deadline())
	defer cancel()

	if len(excluded) > 0 {
		patterns, err = resolveExclusions(ctx, req, binary, patterns, excluded)
		if err != nil {
			return nil, err
		}
	}

	out, err := runProgram(ctx, req, append([]string{binary, "test", "-json"}, patterns...))
	if err != nil {
		return nil, err
	}

	events, decoded := readTestEvents(out.Stdout)
	if decoded == 0 {
		// Nothing kept could read. A suite that ran and failed still emits
		// events, so an empty stream with a non-zero exit is `go` refusing the
		// invocation — a toolchain too old for -json, a pattern matching no
		// package — and that is a failure to measure rather than a measurement.
		//
		// What it printed is deliberately not echoed here. Raw output belongs
		// in the bounded, redacted receipt docs/decision/0023 designs for it,
		// and redaction does not exist yet (0034); putting the bytes in an
		// error message would route them straight to a person's terminal and,
		// through a disposition, into the ledger.
		if out.Exit != 0 {
			return nil, fmt.Errorf("go test printed nothing kept could read and exited %d (%d bytes on stderr)",
				out.Exit, len(out.Stderr))
		}
		// Exit zero and nothing to read is a suite of no packages, which is a
		// measurement: nothing ran, and a promise about packages says so.
	}

	return foldTestEvents(events), nil
}

// resolveExclusions turns a pattern and its exclusions into the explicit list of
// packages the suite runs over.
//
// The subtraction is done in import paths that `go list` produced, so `exclude`
// means exactly what a Go package pattern means everywhere else and kept
// hand-rolls no second pattern language.
//
// Every refusal here is a failure to measure and therefore `unknown`: what
// cannot be established is which packages the promise meant, and answering
// `not-kept` would report that the tests failed when nothing ran.
func resolveExclusions(ctx context.Context, req Request, binary string, patterns, excluded []string) ([]string, error) {
	suite, err := listPackages(ctx, req, binary, patterns)
	if err != nil {
		return nil, err
	}
	if len(suite) == 0 {
		return nil, fmt.Errorf("%s matched no package, so nothing says what to run", joinWords(patterns))
	}
	in := make(map[string]bool, len(suite))
	for _, name := range suite {
		in[name] = true
	}

	dropped := map[string]bool{}
	for _, pattern := range excluded {
		// One call per pattern, so a pattern that has stopped removing anything
		// is named rather than hidden behind one that still does.
		names, err := listPackages(ctx, req, binary, []string{pattern})
		if err != nil {
			return nil, err
		}
		removes := false
		for _, name := range names {
			if in[name] {
				dropped[name] = true
				removes = true
			}
		}
		if !removes {
			return nil, fmt.Errorf("%s: %q removes no package from %s, so this suite is not the one the promise describes",
				argExclude, pattern, joinWords(patterns))
		}
	}

	// go list's order, minus what was dropped. An argv nobody can predict is an
	// argv nobody can reproduce.
	out := make([]string, 0, len(suite))
	for _, name := range suite {
		if !dropped[name] {
			out = append(out, name)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: %s removes every package %s matched, so nothing would run",
			argExclude, joinWords(excluded), joinWords(patterns))
	}
	return out, nil
}

// listPackages expands package patterns to the import paths they name.
//
// `-e` because the tree a repair agent stands in is one that does not compile:
// without it a package with an unresolvable import takes the whole enumeration
// down, and a suite that would have reported a build failure reports that
// nobody measured. The `{{if .Dir}}` guard is what `-e` costs — a pattern that
// matched nothing is echoed back as a package of its own, with no directory —
// and letting one through would put the pattern string itself in the argv of
// the run that follows.
//
// What `go list` printed on stderr is counted rather than echoed, for the
// reason the fold gives: raw output belongs in a bounded, redacted receipt and
// there is no redaction yet. See docs/decision/0023.
func listPackages(ctx context.Context, req Request, binary string, patterns []string) ([]string, error) {
	argv := append([]string{binary, "list", "-e", "-f", "{{if .Dir}}{{.ImportPath}}{{end}}"}, patterns...)
	out, err := runProgram(ctx, req, argv)
	if err != nil {
		return nil, err
	}
	if out.Exit != 0 {
		return nil, fmt.Errorf("go list over %s exited %d (%d bytes on stderr), so nothing knows which packages to run",
			joinWords(patterns), out.Exit, len(out.Stderr))
	}
	var names []string
	for line := range strings.SplitSeq(out.Stdout, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			names = append(names, line)
		}
	}
	return names, nil
}

// testEvent is one line of `go test -json`, read as data.
//
// The stream is heterogeneous on purpose: a package-level event carries no
// Test, a build failure carries an ImportPath and no Package, and most lines
// are Action "output" and carry nothing worth recording. Assuming a uniform
// shape here is the mistake that made has() necessary in the predicate
// language, and it is the same mistake in Go.
type testEvent struct {
	Action     string `json:"Action"`
	Package    string `json:"Package"`
	Test       string `json:"Test"`
	ImportPath string `json:"ImportPath"`
}

// readTestEvents decodes the stream and says how many lines it understood.
//
// It is read line by line rather than as a JSON stream, because that is what
// `go test -json` is: one object per line. A line that is not one is skipped
// rather than refused — `go` writes toolchain notices to stdout before -json
// takes over, and one of those is not a reason to report that nobody could
// measure. What is a reason is understanding none of it at all.
func readTestEvents(stream string) ([]testEvent, int) {
	var out []testEvent
	decoded := 0
	for line := range strings.SplitSeq(stream, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var e testEvent
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		decoded++
		out = append(out, e)
	}
	return out, decoded
}

// pkg is one package's tally as the stream is folded.
type pkg struct {
	ok      bool
	decided bool
	run     map[string]bool
	passed  int
	failed  int
	skipped int
}

// foldTestEvents turns the stream into the declared fields.
//
// Everything is sorted, and that is not tidiness. An observation is addressed
// by its data, so an ordering that came from map iteration or from which
// package finished first would give one measurement a different address every
// time it was made, and two runs measuring the same tree would write two files
// where docs/layout.md says they write one.
func foldTestEvents(events []testEvent) Fields {
	packages := map[string]*pkg{}
	cases := map[string]bool{}
	failures := map[string]bool{}
	var passed, failed, skipped int

	at := func(name string) *pkg {
		p, ok := packages[name]
		if !ok {
			p = &pkg{run: map[string]bool{}}
			packages[name] = p
		}
		return p
	}

	for _, e := range events {
		switch {
		case e.Action == "build-fail":
			// A package that did not compile. It has no test events at all, so
			// without this it would be absent from packages entirely and a
			// promise about every package would read as kept.
			name := packageOf(e.ImportPath)
			if name == "" {
				name = e.Package
			}
			if name == "" {
				continue
			}
			p := at(name)
			p.ok, p.decided = false, true

		case e.Test != "":
			p := at(e.Package)
			switch e.Action {
			case "run", "pass", "fail", "skip":
				p.run[e.Test] = true
				cases[e.Test] = true
			}
			switch e.Action {
			case "pass":
				p.passed++
				passed++
			case "fail":
				p.failed++
				failed++
				failures[e.Test] = true
			case "skip":
				p.skipped++
				skipped++
			}

		case e.Package != "":
			switch e.Action {
			case "pass", "skip":
				p := at(e.Package)
				if !p.decided {
					p.ok, p.decided = true, true
				}
			case "fail":
				p := at(e.Package)
				p.ok, p.decided = false, true
			}
		}
	}

	ok := len(packages) > 0
	records := make([]any, 0, len(packages))
	for _, name := range sortedKeys(packages) {
		p := packages[name]
		if !p.ok {
			ok = false
		}
		records = append(records, map[string]any{
			"package":   name,
			"ok":        p.ok,
			"tests_run": len(p.run),
			"passed":    p.passed,
			"failed":    p.failed,
			"skipped":   p.skipped,
		})
	}

	return Fields{
		"ok":       ok,
		"packages": records,
		"passed":   passed,
		"failed":   failed,
		"skipped":  skipped,
		"cases":    setToList(cases),
		"failures": setToList(failures),
	}
}

// packageOf reads a package out of a build event's import path.
//
// go names the failing build "example/pkg [example/pkg.test]" when it is the
// test binary that would not compile, and "example/pkg" when it is the package
// itself. Both are the same package failing, and recording them as two would
// make a promise about every package see one that never ran and one that
// failed.
func packageOf(importPath string) string {
	name, _, _ := strings.Cut(importPath, " [")
	return strings.TrimSpace(name)
}

func setToList(set map[string]bool) []any {
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]any, len(names))
	for i, name := range names {
		out[i] = name
	}
	return out
}
