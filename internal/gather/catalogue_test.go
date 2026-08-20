package gather

import (
	"errors"
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
