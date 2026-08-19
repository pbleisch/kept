package predicate

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// The parser is the part of this language a person does not write and a corpus
// file can carry anything into, so it is fuzzed. The corpus under
// testdata/fuzz/ is checked in: a crash found once is a case that runs on every
// build afterwards, which is the whole value of finding it.
//
// The properties asserted are the ones a caller depends on. Nothing here
// asserts what any particular expression means — that is what the rest of the
// suite is for — only that no input reaches a panic, an unbounded run, or an
// error that cannot be shown to the person who wrote it.
func FuzzParse(f *testing.F) {
	for _, seed := range fuzzSeeds {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, src string) {
		for _, ns := range []Namespace{HoldWhen, Extractor} {
			e, err := Parse(src, ns)
			if err != nil {
				pe, ok := err.(*Error)
				if !ok {
					t.Fatalf("%q in %s: the error is a %T, and a refusal is always an *Error", src, ns.Name, err)
				}
				checkError(t, src, pe)
				continue
			}
			if e.Nodes() > MaxNodes {
				t.Fatalf("%q parsed to %d nodes, past the cap", src, e.Nodes())
			}

			// A parsed expression is evaluated against a scope holding a
			// little of everything, so that member access, folds and the
			// extraction builtins are all reached. It must return a value or
			// an *Error, and it must return.
			v, err := e.Hold(fuzzScope())
			if err != nil {
				if _, ok := err.(*Error); !ok {
					t.Fatalf("%q: evaluation returned a %T", src, err)
				}
			} else {
				switch v.Value {
				case "kept", "not-kept", "unknown":
				default:
					t.Fatalf("%q evaluated to the verdict %q", src, v.Value)
				}
				if v.Value == "unknown" && v.Because == "" {
					t.Fatalf("%q is unknown and does not say what was not measured", src)
				}
			}

			// Reparsing what was parsed produces the same tree. The source is
			// carried on the Expr for error rendering, and an Expr whose Src
			// does not reparse would render an error against a line that is
			// not the one it came from.
			again, err := Parse(e.Src, ns)
			if err != nil {
				t.Fatalf("%q parsed and its own Src did not: %v", src, err)
			}
			if again.Nodes() != e.Nodes() {
				t.Fatalf("%q reparsed to %d nodes, was %d", src, again.Nodes(), e.Nodes())
			}
		}
	})
}

// checkError asserts what a refusal owes the person who wrote the expression:
// a position inside the source, and a rendering that can be shown.
func checkError(t *testing.T, src string, e *Error) {
	t.Helper()
	if e.Msg == "" {
		t.Fatalf("%q was refused with no message", src)
	}
	if e.Pos.Line < 1 || e.Pos.Column < 1 {
		t.Fatalf("%q was refused at %+v", src, e.Pos)
	}
	if e.Pos.Offset < 0 || e.Pos.Offset > len(src) {
		t.Fatalf("%q was refused at offset %d, and the source is %d bytes", src, e.Pos.Offset, len(src))
	}
	rendered := e.Render()
	if !strings.Contains(rendered, e.Msg) {
		t.Fatalf("%q renders without its own message:\n%s", src, rendered)
	}
	if !utf8.ValidString(rendered) && utf8.ValidString(src) {
		t.Fatalf("%q rendered to invalid UTF-8", src)
	}
}

// fuzzScope holds a little of everything, so a fuzzed expression reaches member
// access, the folds and the extraction builtins rather than stopping at the
// first unmeasured field.
func fuzzScope() Scope {
	return Scope{
		Roots: map[string]any{
			"obs": map[string]any{
				"tests": map[string]any{
					"failed":   int64(0),
					"coverage": 91.5,
					"ok":       true,
					"name":     "corpus",
					"cases":    []any{"TestA", "TestB"},
					"packages": []any{map[string]any{"tests_run": int64(2), "failed": int64(0)}},
					"nested":   map[string]any{"deep": int64(1)},
				},
			},
			"stdout": goTestJSON,
			"stderr": "1 error\n",
			"exit":   int64(0),
		},
		Prior: func(name string) any {
			if name == "tests" {
				return map[string]any{"cases": []any{"TestA"}}
			}
			return Unknown{Because: "prior(" + name + "): nothing was observed before this"}
		},
	}
}

// The seeds are the expressions the decisions write, the shapes that have
// already been wrong once, and the ones that sit against a bound.
var fuzzSeeds = []string{
	`obs.tests.failed == 0`,
	`obs.check.exit == 0`,
	`obs.tests.failed == 0 and "TestRevisionDropsStanding" in obs.tests.cases`,
	`all(obs.tests.packages, p => p.tests_run > 0 and p.failed == 0)`,
	`all(prior(tests).cases, c => c in obs.tests.cases)`,
	`count(json_lines(stdout), e => has(e, "Test") and e.Action == "pass")`,
	`map(filter(json_lines(stdout), e => has(e, "Test") and e.Action == "pass"), e => e.Test)`,
	`find_all(stdout, "(?P<token>gh[ps]_[A-Za-z0-9]{36})")`,
	`find_all(stderr, '\d+ errors')`,
	`not obs.tests.ok or obs.tests.coverage > 80.5`,
	`obs.tests.failed != -1`,
	`json(stdout).version`,
	`has(obs.tests, "coverage")`,
	`lines(stderr)`,
	`true`,
	``,
	`(`,
	`)`,
	`.`,
	`,`,
	`=`,
	`=>`,
	`"`,
	`'`,
	`0.`,
	`1 < 2 < 3`,
	`obs.`,
	`obs.tests.`,
	`prior()`,
	`prior(tests, 2)`,
	`truth("promise:other")`,
	`all(obs, x => x)`,
	`all(obs.tests.cases, obs => obs)`,
	`obs.tests.name == "\q"`,
	"obs.tests.name == \"a\nb\"",
	`obs.tests.name == "café"`,
	`((((((((((obs.tests.ok))))))))))`,
	`not not not not obs.tests.ok`,
	`obs.tests.ok and obs.tests.ok and obs.tests.ok and obs.tests.ok`,
	`find_all(stdout, "(")`,
	`json_lines(exit)`,
	`999999999999999999999999`,
	`-0`,
}
