package predicate

import (
	"strings"
	"testing"
)

func extract(t *testing.T, src string, out Output) (any, bool) {
	t.Helper()
	e, err := Parse(src, Extractor)
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	v, measured, err := e.Extract(out.Scope())
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	return v, measured
}

func refuse(t *testing.T, src string, out Output) *Error {
	t.Helper()
	e, err := Parse(src, Extractor)
	if err != nil {
		if pe, ok := err.(*Error); ok {
			return pe
		}
		t.Fatalf("%s: the error is a %T", src, err)
	}
	_, _, err = e.Extract(out.Scope())
	if err == nil {
		t.Fatalf("%s: that was evaluated and should have been refused", src)
	}
	pe, ok := err.(*Error)
	if !ok {
		t.Fatalf("%s: the error is a %T", src, err)
	}
	return pe
}

// The stream the spike failed on, which is how has() was found. `go test -json`
// emits package-level events with no Test key, so a record lacking one there is
// ordinary data rather than a failure to measure — and without has() the first
// honest extractor over it returns unknown for the whole observation.
const goTestJSON = `{"Time":"2026-01-01T00:00:00Z","Action":"run","Package":"kept/internal/corpus","Test":"TestRenamingAFileChangesNothingItRead"}
{"Time":"2026-01-01T00:00:01Z","Action":"pass","Package":"kept/internal/corpus","Test":"TestRenamingAFileChangesNothingItRead"}
{"Time":"2026-01-01T00:00:01Z","Action":"run","Package":"kept/internal/corpus","Test":"TestRevisionDropsStanding"}
{"Time":"2026-01-01T00:00:02Z","Action":"fail","Package":"kept/internal/corpus","Test":"TestRevisionDropsStanding"}
{"Time":"2026-01-01T00:00:02Z","Action":"fail","Package":"kept/internal/corpus","Elapsed":2.1}
`

func TestHasIsWhatMakesOneLanguageDoBothJobs(t *testing.T) {
	out := Output{Stdout: goTestJSON}

	passed, measured := extract(t, `count(json_lines(stdout), e => has(e, "Test") and e.Action == "pass")`, out)
	if !measured {
		t.Fatal("the guarded extractor measured nothing")
	}
	if passed != int64(1) {
		t.Errorf("passed = %v, want 1", passed)
	}

	failed, _ := extract(t, `count(json_lines(stdout), e => has(e, "Test") and e.Action == "fail")`, out)
	if failed != int64(1) {
		t.Errorf("failed = %v, want 1 — the package-level fail has no Test and is not one", failed)
	}

	cases, _ := extract(t, `map(filter(json_lines(stdout), e => has(e, "Test") and e.Action == "pass"), e => e.Test)`, out)
	if got := render(cases); got != `["TestRenamingAFileChangesNothingItRead"]` {
		t.Errorf("cases = %s", got)
	}
}

// The guard for the test above: this is what the spike did before has()
// existed. The same question asked without the guard reads Test off the
// package-level event, which has none, and the whole field goes unmeasured.
// That is right — nothing here should quietly answer 4 — and it is why the
// builtin is not optional.
func TestWithoutHasTheHeterogeneousStreamIsUnmeasured(t *testing.T) {
	out := Output{Stdout: goTestJSON}
	v, measured := extract(t, `count(json_lines(stdout), e => e.Test != "")`, out)
	if measured {
		t.Fatalf("the unguarded extractor measured %v, and one record has no Test at all", v)
	}
}

func TestFoldsAreThreeValued(t *testing.T) {
	packages := map[string]any{"tests": map[string]any{"packages": []any{
		map[string]any{"name": "corpus", "tests_run": int64(9), "failed": int64(0)},
		map[string]any{"name": "ledger", "tests_run": int64(4), "failed": int64(0)},
	}}}
	if got := hold(t, `all(obs.tests.packages, p => p.tests_run > 0 and p.failed == 0)`, packages).Value; got != "kept" {
		t.Errorf("all over two clean packages is %s", got)
	}

	// One package the gatherer said nothing about. all() cannot answer, and it
	// says so rather than counting it as a pass or as a failure.
	partial := map[string]any{"tests": map[string]any{"packages": []any{
		map[string]any{"name": "corpus", "tests_run": int64(9), "failed": int64(0)},
		map[string]any{"name": "ledger"},
	}}}
	if got := hold(t, `all(obs.tests.packages, p => p.tests_run > 0 and p.failed == 0)`, partial).Value; got != "unknown" {
		t.Errorf("all over a package nobody measured is %s", got)
	}

	// And one that is measured and bad settles it, whatever else is missing:
	// a conjunct known false makes the conjunction false.
	broken := map[string]any{"tests": map[string]any{"packages": []any{
		map[string]any{"name": "corpus", "tests_run": int64(9), "failed": int64(2)},
		map[string]any{"name": "ledger"},
	}}}
	if got := hold(t, `all(obs.tests.packages, p => p.failed == 0)`, broken).Value; got != "not-kept" {
		t.Errorf("all over one failing package and one unmeasured is %s", got)
	}
}

// An empty list is true, as an empty conjunction is. Written down because it is
// the reading that surprises people, and because a promise that also means
// "and there is at least one" has count() to say so with.
func TestAllOverAnEmptyListIsTrue(t *testing.T) {
	empty := map[string]any{"tests": map[string]any{"packages": []any{}}}
	if got := hold(t, `all(obs.tests.packages, p => p.failed == 0)`, empty).Value; got != "kept" {
		t.Errorf("all over nothing is %s", got)
	}
	if got := hold(t, `count(obs.tests.packages, p => true) > 0`, empty).Value; got != "not-kept" {
		t.Errorf("count over nothing is %s", got)
	}
}

// A count that skipped what it could not decide would report a number nobody
// measured, and a filter that dropped one would return a list the author did
// not ask for.
func TestCountAndFilterRefuseToGuessOverAnUnmeasuredElement(t *testing.T) {
	data := map[string]any{"tests": map[string]any{"cases": []any{
		map[string]any{"name": "a", "ok": true},
		map[string]any{"name": "b"},
	}}}
	if got := hold(t, `count(obs.tests.cases, c => c.ok) == 1`, data).Value; got != "unknown" {
		t.Errorf("count over an unmeasured element is %s", got)
	}
	if got := hold(t, `count(filter(obs.tests.cases, c => c.ok), c => true) == 1`, data).Value; got != "unknown" {
		t.Errorf("filter over an unmeasured element is %s", got)
	}
}

func TestLinesReadsWhatAProgramActuallyPrinted(t *testing.T) {
	v, _ := extract(t, `lines(stdout)`, Output{Stdout: "a\nb\n"})
	if got := render(v); got != `["a", "b"]` {
		t.Errorf("lines = %s; a trailing newline is not an extra line", got)
	}
	v, _ = extract(t, `lines(stdout)`, Output{Stdout: "a\r\nb\r\n"})
	if got := render(v); got != `["a", "b"]` {
		t.Errorf("lines over CRLF = %s", got)
	}
	v, _ = extract(t, `lines(stdout)`, Output{Stdout: ""})
	if got := render(v); got != `[]` {
		t.Errorf("lines of nothing = %s", got)
	}
	v, _ = extract(t, `lines(stdout)`, Output{Stdout: "a\n\nb"})
	if got := render(v); got != `["a", "", "b"]` {
		t.Errorf("lines = %s; a blank line in the middle is a line", got)
	}
}

// Text that is not JSON is unmeasured and not an error. The expression asked
// the right question and the program did not answer it, and a build that fell
// over before printing its report must not make a promise not-kept.
func TestOutputThatIsNotJSONIsUnmeasuredRatherThanWrong(t *testing.T) {
	if _, measured := extract(t, `json(stdout).version`, Output{Stdout: "panic: runtime error\n"}); measured {
		t.Error("text that is not JSON was read as JSON")
	}
	// A stream with one plain line in the middle of it — a test binary that
	// panicked — makes the whole stream unmeasured rather than being skipped.
	// Skipping it would report a passing count over a truncated run.
	broken := goTestJSON + "panic: test timed out after 10m0s\n"
	if _, measured := extract(t, `count(json_lines(stdout), e => has(e, "Test"))`, Output{Stdout: broken}); measured {
		t.Error("a stream with a panic in it was counted anyway")
	}
	// Two JSON values run together are not one JSON value, and are not
	// silently read as the first of them.
	if _, measured := extract(t, `json(stdout)`, Output{Stdout: `{"a":1} {"b":2}`}); measured {
		t.Error("two values run together were read as one")
	}
}

// A pattern that will not compile is the author's, so it is an error rather
// than an unmeasured value — and a literal one is refused when the expression
// parses rather than when a verdict is being decided.
func TestABadPatternIsRefusedBeforeAnythingIsMeasured(t *testing.T) {
	if _, err := Parse(`find_all(stdout, "(?P<x>")`, Extractor); err == nil {
		t.Fatal("an uncompilable literal pattern parsed")
	}
	err := refuse(t, `find_all(stdout, stderr)`, Output{Stdout: "x", Stderr: "("})
	if !strings.Contains(err.Error(), "will not compile") {
		t.Errorf("message is %q", err.Error())
	}
}

func TestFindAllReturnsTheMatchAndItsNamedGroups(t *testing.T) {
	out := Output{Stdout: "ok 12 files, 3 errors\nok 4 files, 0 errors\n"}
	v, measured := extract(t, `find_all(stdout, '(?P<files>\d+) files, (?P<errors>\d+) errors')`, out)
	if !measured {
		t.Fatal("nothing matched")
	}
	list := v.([]any)
	if len(list) != 2 {
		t.Fatalf("%d matches, want 2", len(list))
	}
	first := list[0].(map[string]any)
	if first["files"] != "12" || first["errors"] != "3" {
		t.Errorf("groups = %v", first)
	}
	if !strings.Contains(first["match"].(string), "12 files") {
		t.Errorf("match = %v", first["match"])
	}

	// Nothing matching is an empty list, not an unmeasured value: the pattern
	// ran and found nothing, which is a measurement.
	v, measured = extract(t, `find_all(stdout, 'nothing like this')`, out)
	if !measured || render(v) != "[]" {
		t.Errorf("no matches gave %v, measured=%v", render(v), measured)
	}
}

// A fold over something that is not a list, and a field read off something with
// none, are the expression being wrong. Each is an *Error and none of them is
// unknown.
func TestAFoldOverTheWrongThingIsRefused(t *testing.T) {
	data := map[string]any{"tests": map[string]any{"ok": true, "count": int64(3), "name": "x"}}
	for _, src := range []string{
		`all(obs.tests.ok, p => p)`,
		`count(obs.tests.count, p => true) > 0`,
		`map(obs.tests.name, p => p) == 1`,
		`filter(obs.tests.ok, p => p) == 1`,
		`has(obs.tests.count, "x")`,
		`all(obs.tests.cases, p => p)`,
	} {
		if src == `all(obs.tests.cases, p => p)` {
			// The one exception, and it is the rule rather than an exception:
			// a fold over a list nobody measured is unmeasured, not wrong.
			if got := hold(t, src, data).Value; got != "unknown" {
				t.Errorf("%s is %s", src, got)
			}
			continue
		}
		mustFail(t, src, data)
	}
}

// The message a fold gives, because 0036 writes it out and an author reads it.
func TestAFoldSaysWhatItNeededAndWhatItGot(t *testing.T) {
	err := mustFail(t, `all(obs.tests.ok, p => p)`, map[string]any{"tests": map[string]any{"ok": true}})
	if !strings.Contains(err.Msg, "all needs a list, and got a true/false") {
		t.Errorf("message is %q", err.Msg)
	}
	if !strings.Contains(err.Render(), "^ at line 1, column") {
		t.Errorf("rendering is:\n%s", err.Render())
	}
}
