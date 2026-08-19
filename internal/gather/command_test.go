package gather

import (
	"errors"
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/model"
)

// command never stores what a program printed. Wanting the whole log means
// asking for it, and asking is the moment to think about what is in it — a
// build log carries tokens and connection strings, and nothing rewrites a
// ledger. See docs/decision/0023.
func TestCommandWithNoExtractorIsRefused(t *testing.T) {
	for _, args := range []map[string]any{
		{argRun: "go build ./..."},
		{argRun: "go build ./...", argFields: map[string]any{}},
		{argRun: "go build ./...", argFields: "ok"},
	} {
		if _, err := (command{}).Declares(args); err == nil {
			t.Errorf("%#v declared fields without an extractor", args)
		} else if !strings.Contains(err.Error(), "fields:") {
			t.Errorf("%#v was refused as %q, which does not say what to write", args, err)
		}
	}
}

// An extractor is a promise's own expression, so one that will not parse is an
// authoring error a person fixes rather than an unknown reported against a
// promise nobody could have kept.
func TestAnExtractorThatWillNotParseIsRefusedBeforeAnythingRuns(t *testing.T) {
	_, err := (command{}).Declares(map[string]any{
		argRun:    "go build ./...",
		argFields: map[string]any{"ok": "exit ==="},
	})
	if err == nil {
		t.Fatal("an extractor that does not parse was accepted")
	}
	if !strings.Contains(err.Error(), "ok") {
		t.Errorf("the refusal is %q and does not name the field", err)
	}
}

// An extractor reads what a program produced, and nothing else. A field naming
// an observation is a namespace confusion, not a measurement.
func TestAnExtractorCannotReadObservations(t *testing.T) {
	_, err := (command{}).Declares(map[string]any{
		argRun:    "go build ./...",
		argFields: map[string]any{"ok": "obs.tests.failed == 0"},
	})
	if err == nil {
		t.Fatal("an extractor read the predicate namespace")
	}
}

// A gatherer that quietly ignored an arg would measure something other than
// what was asked and say nothing about it — and because the args bind, the
// mistake would be baked into a ratified promise.
func TestAnArgNothingAcceptsIsRefused(t *testing.T) {
	_, err := (command{}).Declares(map[string]any{
		argRun:    "go build ./...",
		argFields: map[string]any{"ok": "exit == 0"},
		"cmd":     "go vet ./...",
	})
	if err == nil {
		t.Fatal("command accepted an arg it does not have")
	}
	if !strings.Contains(err.Error(), "cmd") || !strings.Contains(err.Error(), argRun) {
		t.Errorf("the refusal is %q, and should name both what was written and what is taken", err)
	}
}

// The observation carries the declared fields and nothing else. Undeclared
// output is dropped rather than stored, which is docs/decision/0023's first
// layer.
func TestOnlyDeclaredFieldsAreRecorded(t *testing.T) {
	fields, err := (command{}).Gather(t.Context(), request(t, map[string]any{
		argRun: runLine(t, "print", "one two three"),
		argFields: map[string]any{
			"words": `count(lines(stdout), l => l != "")`,
			"ok":    "exit == 0",
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) != 2 {
		t.Fatalf("the observation carries %#v, and the promise declared two fields", fields)
	}
	for _, undeclared := range []string{"stdout", "stderr", "exit", "out"} {
		if _, present := fields[undeclared]; present {
			t.Errorf("%s was stored without being declared", undeclared)
		}
	}
}

// An extractor that measured nothing yields no field rather than a field
// holding an unknown, so a predicate reading it gets unknown for its own
// reason. See docs/decision/0036.
func TestAnUnmeasuredFieldIsAbsentFromTheObservation(t *testing.T) {
	fields, err := (command{}).Gather(t.Context(), request(t, map[string]any{
		argRun: runLine(t, "print", "not json at all"),
		argFields: map[string]any{
			"ok":     "exit == 0",
			"parsed": `json(stdout)`,
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if _, present := fields["parsed"]; present {
		t.Fatalf("the observation carries parsed = %#v for output that is not JSON", fields["parsed"])
	}
	if fields["ok"] != true {
		t.Errorf("the field that was measured is %#v", fields["ok"])
	}

	obs := model.Observation{Data: map[string]any(fields)}
	if got := verdict(t, "check", obs, `has(obs.check, "parsed")`); got != model.NotKept {
		t.Errorf("has() on the absent field says %s", got)
	}
	if got := verdict(t, "check", obs, `obs.check.parsed == 1`); got != model.Unknown {
		t.Errorf("reading the absent field says %s, and nobody measured it", got)
	}
}

// An extractor that is wrong at evaluation is a gather failure naming the
// field, not a verdict. Most are caught at load; this is the kind only a real
// program's output produces.
func TestAnExtractorThatIsWrongAtEvaluationNamesItsField(t *testing.T) {
	_, err := (command{}).Gather(t.Context(), request(t, map[string]any{
		argRun:    runLine(t, "print", "hello"),
		argFields: map[string]any{"broken": `all(stdout, x => x)`},
	}))
	var bad *BadField
	if !errors.As(err, &bad) {
		t.Fatalf("a fold over text failed as %#v", err)
	}
	if bad.Field != "broken" {
		t.Errorf("the failure names %q", bad.Field)
	}
}

// The escape hatch reads a program's output the way a promise about this
// repository would: stdout, stderr and exit, and the language the promise is
// already written in.
func TestCommandExtractsAcrossAllThreeRoots(t *testing.T) {
	fields, err := (command{}).Gather(t.Context(), request(t, map[string]any{
		argRun: runLine(t, "warn", "vet: internal/a.go:1: bad\nvet: internal/b.go:2: worse\n"),
		argFields: map[string]any{
			"ok":       "exit == 0",
			"findings": `count(lines(stderr), l => l != "")`,
			"quiet":    `stdout == ""`,
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if fields["ok"] != true || fields["quiet"] != true || fields["findings"] != int64(2) {
		t.Fatalf("the observation is %#v", fields)
	}
}
