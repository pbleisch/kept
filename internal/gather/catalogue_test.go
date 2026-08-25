package gather

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/predicate"
)

// hold parses a hold_when and checks it against what the artifact's gatherers
// declare, the way the corpus does when it loads.
func hold(t *testing.T, src string, gather map[string]model.GatherSpec) []*predicate.Error {
	t.Helper()
	expr, err := predicate.Parse(src, predicate.HoldWhen)
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	return expr.Check(Builtin().Sources(gather))
}

// A promise reading a field its gatherer never declares is an authoring error
// caught at load, not an unknown at evaluation. This is what a gatherer's
// declaration buys beyond docs/decision/0023's claim for it, and it is
// docs/decision/0036's reason for the check.
func TestAPromiseReadingAFieldNothingDeclaresIsRefusedAtLoad(t *testing.T) {
	gather := map[string]model.GatherSpec{
		"tests": {Gatherer: Tests, Args: map[string]any{argScope: "internal/**"}},
	}

	problems := hold(t, "obs.tests.coverage > 80", gather)
	if len(problems) != 1 {
		t.Fatalf("reading an undeclared field produced %v", problems)
	}
	message := problems[0].Error()
	for _, want := range []string{"coverage", "tests", "packages"} {
		if !strings.Contains(message, want) {
			t.Errorf("the refusal does not name %s:\n%s", want, message)
		}
	}

	// And the fields it does declare pass, or the check is worthless.
	if problems := hold(t, "obs.tests.failed == 0 and obs.tests.ok", gather); len(problems) != 0 {
		t.Fatalf("a promise reading declared fields was refused: %v", problems)
	}
}

// `command` declares its fields in the promise, and the check reads them from
// there. That asymmetry is the decision rather than an inconsistency: whoever
// decides the output shape declares it, and an authored extractor names fields
// kept has never seen. It is the only gatherer that does so today, and
// docs/decision/0043 is why that is a fact about the catalogue rather than the
// rule.
func TestCommandsFieldsAreCheckedFromThePromise(t *testing.T) {
	gather := map[string]model.GatherSpec{
		"vet": {Gatherer: Command, Args: map[string]any{
			argRun:    "go vet ./...",
			argFields: map[string]any{"ok": "exit == 0", "findings": `count(lines(stderr), l => l != "")`},
		}},
	}

	if problems := hold(t, "obs.vet.ok and obs.vet.findings == 0", gather); len(problems) != 0 {
		t.Fatalf("a promise reading its own extractor's fields was refused: %v", problems)
	}
	problems := hold(t, "obs.vet.warnings == 0", gather)
	if len(problems) != 1 {
		t.Fatalf("reading a field the extractor does not produce gave %v", problems)
	}
	if !strings.Contains(problems[0].Error(), "findings") {
		t.Errorf("the refusal does not say what the extractor does declare:\n%s", problems[0])
	}
}

// A gather entry whose own args are wrong is told about once. Being told that
// every field it reads is missing as well would bury the one thing to fix.
func TestAnEntryThatIsItselfWrongDoesNotAlsoFailEveryField(t *testing.T) {
	gather := map[string]model.GatherSpec{
		"vet": {Gatherer: Command, Args: map[string]any{argRun: "go vet ./..."}},
	}
	if problems := hold(t, "obs.vet.ok and obs.vet.findings == 0", gather); len(problems) != 0 {
		t.Fatalf("a promise whose gather entry is broken was also told about its fields: %v", problems)
	}

	bad := Builtin().Check(gather)
	if len(bad) != 1 {
		t.Fatalf("the entry itself produced %v", bad)
	}
}

// The name half of the check needs no catalogue at all: a promise reading
// obs.tests while declaring no gatherer called tests is reading something that
// will never arrive, whatever gatherers turn out to declare.
func TestAPromiseReadingANameItDoesNotGatherIsStillRefused(t *testing.T) {
	problems := hold(t, "obs.tests.ok", map[string]model.GatherSpec{
		"build": {Gatherer: Command, Args: map[string]any{
			argRun: "go build ./...", argFields: map[string]any{"ok": "exit == 0"},
		}},
	})
	if len(problems) != 1 {
		t.Fatalf("reading an ungathered name produced %v", problems)
	}
	if !strings.Contains(problems[0].Error(), "it gathers build") {
		t.Errorf("the refusal does not say what is available:\n%s", problems[0])
	}
}

// A gatherer that does not exist is a different mistake from an entry whose
// args are wrong, and a caller that cannot import this package must still be
// able to tell them apart without reading a message.
func TestAnUnknownGathererIsToldApartFromABadEntry(t *testing.T) {
	bad := Builtin().Check(map[string]model.GatherSpec{
		"coverage": {Gatherer: "coverage"},
		"vet":      {Gatherer: Command, Args: map[string]any{argRun: "go vet ./..."}},
	})
	if len(bad) != 2 {
		t.Fatalf("two wrong entries produced %v", bad)
	}

	var missing []string
	for _, err := range bad {
		var spec *BadSpec
		if !errors.As(err, &spec) {
			t.Fatalf("%v is not a gather-entry refusal", err)
		}
		if spec.NoSuchGatherer() {
			missing = append(missing, spec.Name)
		}
	}
	if !same(missing, []string{"coverage"}) {
		t.Fatalf("the entries naming no gatherer are %q", missing)
	}
}

// A catalogue that shipped a gatherer whose name and version disagreed with its
// key would file measurements under a name no promise could reach.
func TestEveryBuiltinAnswersToItsKey(t *testing.T) {
	for key, g := range Builtin() {
		name, version, found := strings.Cut(g.Name(), "@")
		if !found || version == "" {
			t.Errorf("%s is named %q, and an observation's address carries the version", key, g.Name())
		}
		if name != key {
			t.Errorf("the catalogue calls it %s and it calls itself %s", key, name)
		}
	}
}

// kept decides these three output shapes, so their fields are declared in code
// and a promise reading something else is refused when the corpus loads rather
// than reported unknown in the path that decides a verdict. That is
// docs/decision/0043's rule, and this is the half of it that is not `command`.
func TestTheParsingGatherersDeclareTheirFieldsInCode(t *testing.T) {
	for _, c := range []struct {
		gatherer string
		good     string
		bad      string
		names    []string
	}{
		{Imports,
			`all(obs.imports.edges, e => e.from != "x") and obs.imports.ok`,
			"obs.imports.cycles",
			[]string{"cycles", "imports", "edges", "external"}},
		{Symbols,
			`count(obs.symbols.refs, r => r.path == "os") == 0 and obs.symbols.ok`,
			"obs.symbols.hits",
			[]string{"hits", "symbols", "refs", "defines"}},
		{Corpus,
			"obs.corpus.artifacts > 0 and count(obs.corpus.broken_links, l => true) == 0",
			"obs.corpus.ratified",
			[]string{"ratified", "corpus", "findings", "broken_links"}},
	} {
		// The map key is the name the predicate reads under, and it is an
		// identifier; a grouped gatherer name carries a slash and cannot be
		// one. So the key is the bare tail and the gatherer is named. See
		// docs/decision/0051.
		key := c.gatherer
		if i := strings.LastIndex(key, "/"); i >= 0 {
			key = key[i+1:]
		}
		gather := map[string]model.GatherSpec{key: {Gatherer: c.gatherer}}

		if problems := hold(t, c.good, gather); len(problems) != 0 {
			t.Errorf("%s: a promise reading declared fields was refused: %v", c.gatherer, problems)
		}
		problems := hold(t, c.bad, gather)
		if len(problems) != 1 {
			t.Fatalf("%s: reading %s produced %v", c.gatherer, c.bad, problems)
		}
		for _, want := range c.names {
			if !strings.Contains(problems[0].Error(), want) {
				t.Errorf("%s: the refusal does not name %s:\n%s", c.gatherer, want, problems[0])
			}
		}
	}
}

// An arg nothing accepts is an authoring error too, and it is named alongside
// what is accepted — a gatherer that quietly ignored `test:` for `tests:`
// would measure something other than what was asked, and because the args bind
// the mistake would be baked into a ratified promise.
func TestTheParsingGatherersRefuseAnArgNothingTakes(t *testing.T) {
	for _, name := range []string{Imports, Symbols, Corpus} {
		errs := Builtin().Check(map[string]model.GatherSpec{
			name: {Gatherer: name, Args: map[string]any{"packages": "./..."}},
		})
		if len(errs) != 1 {
			t.Fatalf("%s accepted an arg it does not take: %v", name, errs)
		}
		var bad *BadSpec
		if !errors.As(errs[0], &bad) || bad.NoSuchGatherer() {
			t.Errorf("%s reports %#v", name, errs[0])
		}
	}
}

// runnableArgs is a spec every built-in accepts, one entry per catalogue key.
//
// It is a table rather than a bare unknown key handed to all five, because a
// gatherer that refused for some *other* reason — `command` with no `run:` —
// would pass a test that only asked whether it refused. Each entry is asserted
// runnable on its own below, so the unknown key added to it is the one thing
// wrong.
var runnableArgs = map[string]map[string]any{
	Command: {argRun: "true", argFields: map[string]any{"ok": "exit == 0"}},
	Corpus:  {argRoot: "docs"},
	Tests:   {argPackages: "./internal/...", argExclude: "./internal/confine"},
	Imports: {argTests: true},
	Symbols: {argTests: true},
}

// The load and the gather path answer one question about a spec, and this is
// the test that they do.
//
// It failed for four of the five before [refuseUnrunnableSpec] existed. `kept
// list` refused a `go/tests` entry carrying an arg nothing accepts and the tick
// measured with it anyway and recorded `kept` — an observation faithfully
// recording an `exclude:` that never happened, from a promise whose statement
// said the exclusion was the point. Only `command` refused, and only because
// [readCommand] happens to be shared between its Declares and its Gather.
//
// It is driven over [Builtin] rather than over a list of names so that a
// gatherer added later is covered the day it is added, and it fails outright
// for one that has no entry in the table above.
func TestAnUnknownArgIsRefusedOnTheGatherPathByEveryGatherer(t *testing.T) {
	c := Builtin()
	for key, g := range c {
		args, listed := runnableArgs[key]
		if !listed {
			t.Fatalf("%s is in the catalogue and not in this table, so nothing here says what it accepts", key)
		}
		// The control. Without it a refusal below proves only that something
		// was wrong, and the whole defect is a gatherer refusing at load and
		// measuring anyway.
		if _, err := g.Declares(args); err != nil {
			t.Fatalf("%s refuses args this table calls valid: %v", key, err)
		}

		bogus := map[string]any{"bogusarg": true}
		for k, v := range args {
			bogus[k] = v
		}
		spec := map[string]model.GatherSpec{"probe": {Gatherer: key, Args: bogus}}

		if errs := c.Check(spec); len(errs) != 1 {
			t.Fatalf("%s: the load reports %v for a spec carrying an arg nothing takes", key, errs)
		}
		obs := NewRunner(c).Gather(t.Context(), spec["probe"], request(t, bogus))
		if obs.Err == nil {
			t.Fatalf("%s measured with a spec the load refuses, and produced %v", key, obs.Data)
		}
		if obs.Data != nil {
			t.Errorf("%s left data behind for a spec nobody can run as written", key)
		}
		if !strings.Contains(obs.Err.Error(), "bogusarg") {
			t.Errorf("%s: the reason does not name the arg:\n%s", key, obs.Err)
		}
		if got := verdict(t, "probe", obs, "obs.probe.ok"); got != model.Unknown {
			t.Errorf("%s: a spec nobody can run reads %s", key, got)
		}
	}
}

// The same seam from the other side: a spec the load accepts is still measured.
// A guard that refused everything would satisfy the test above and break every
// promise in every corpus, and this is what notices.
func TestASpecTheLoadAcceptsIsStillMeasured(t *testing.T) {
	c := Builtin()
	args := map[string]any{argRun: "true", argFields: map[string]any{"ok": "exit == 0"}}
	if errs := c.Check(map[string]model.GatherSpec{"check": {Gatherer: Command, Args: args}}); len(errs) != 0 {
		t.Fatalf("the load refuses a good spec: %v", errs)
	}
	obs := NewRunner(c).Gather(t.Context(), model.GatherSpec{Gatherer: Command}, request(t, args))
	if obs.Err != nil {
		t.Fatalf("a spec the load accepts was refused on the gather path: %v", obs.Err)
	}
}

// The reported case, pinned: `go/tests` carrying an arg nothing accepts, over a
// toolchain that would answer.
//
// The sweep above proves the property for every gatherer; this one proves the
// *stake*. Against a stand-in `go` that prints a real stream, the gather
// succeeds and folds a full observation — so before the guard the tick recorded
// a confident `kept` from a spec `kept list` had already refused, and the
// observation faithfully recorded args that were never honoured. What is under
// test is that nothing is measured at all.
func TestGoTestsWithAnArgNothingTakesMeasuresNothingEvenWhenTheToolchainWouldAnswer(t *testing.T) {
	abs, err := filepath.Abs(stream)
	if err != nil {
		t.Fatal(err)
	}
	c := Catalogue{Tests: tests{binary: writeStub(t, abs, "0")}}
	spec := model.GatherSpec{Gatherer: Tests}
	good := map[string]any{argPackages: "./internal/..."}

	// The control, and the whole reason this test is not a tautology: with the
	// same args less the unknown key, this gatherer measures.
	if obs := NewRunner(c).Gather(t.Context(), spec, request(t, good)); obs.Err != nil {
		t.Fatalf("the stand-in toolchain did not answer, so a refusal below proves nothing: %v", obs.Err)
	} else if obs.Data["ok"] == nil {
		t.Fatalf("the stand-in toolchain produced no verdict field: %v", obs.Data)
	}

	bogus := map[string]any{argPackages: "./internal/...", "bogusarg": true}
	if errs := c.Check(map[string]model.GatherSpec{"tests": {Gatherer: Tests, Args: bogus}}); len(errs) != 1 {
		t.Fatalf("the load reports %v", errs)
	}
	obs := NewRunner(c).Gather(t.Context(), spec, request(t, bogus))
	if obs.Err == nil {
		t.Fatalf("go/tests measured with a spec the load refuses, and recorded %v", obs.Data)
	}
	if !strings.Contains(obs.Err.Error(), "bogusarg") {
		t.Errorf("the reason does not name the arg:\n%s", obs.Err)
	}
	if got := verdict(t, "tests", obs, "obs.tests.ok"); got != model.Unknown {
		t.Errorf("a refused go/tests spec reads %s", got)
	}
}
