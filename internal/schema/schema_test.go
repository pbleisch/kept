package schema

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// The schemas this repository ships are the first corpus kept has to read, so
// they are read here rather than a copy of them.
func shipped(t *testing.T) *Set {
	t.Helper()
	set, err := Load(filepath.Join("..", "..", "docs", Dir))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return set
}

func TestLoadReadsAShippedSchema(t *testing.T) {
	sc, ok := shipped(t).For("promise")
	if !ok {
		t.Fatal("no promise schema")
	}

	if want := []string{"statement", "gather", "hold_when", "trigger"}; !reflect.DeepEqual(sc.Hash, want) {
		t.Errorf("Hash = %v, want %v", sc.Hash, want)
	}
	if sc.Body != "why" {
		t.Errorf("Body = %q, want %q", sc.Body, "why")
	}
	if want := []string{"serves", "cites", "supersedes"}; !reflect.DeepEqual(sc.AllowedRelations, want) {
		t.Errorf("AllowedRelations = %v, want %v", sc.AllowedRelations, want)
	}

	statement := sc.Fields["statement"]
	if !statement.Required || statement.Type != TypeString {
		t.Errorf("statement = %+v, want a required string", statement)
	}
	trigger := sc.Fields["trigger"]
	if want := []string{"cadence", "on-act", "once"}; !reflect.DeepEqual(trigger.Values, want) {
		t.Errorf("trigger values = %v, want %v", trigger.Values, want)
	}
	if trigger.Default != "cadence" {
		t.Errorf("trigger default = %v, want cadence", trigger.Default)
	}

	// The declared order is kept, because it is the order a reader should see
	// the fields in and a map has none.
	if want := []string{"statement", "gather", "hold_when", "trigger", "max_age", "repair"}; !reflect.DeepEqual(sc.FieldOrder(), want) {
		t.Errorf("FieldOrder = %v, want %v", sc.FieldOrder(), want)
	}
}

func TestEveryShippedSchemaLoads(t *testing.T) {
	set := shipped(t)
	want := []string{"canon", "decision", "desire", "document", "fact", "policy", "promise"}
	if !reflect.DeepEqual(set.Types(), want) {
		t.Errorf("Types = %v, want %v", set.Types(), want)
	}

	// The one shipped kind whose body is sections rather than a prose field.
	// Both halves matter: the sections are what a hash list may name, and an
	// empty Body is what says the block is not one field. See 0040 and 0041.
	decision, _ := set.For("decision")
	if want := []string{"context", "decision", "consequences"}; !reflect.DeepEqual(decision.Sections, want) {
		t.Errorf("decision sections = %v, want %v", decision.Sections, want)
	}
	if decision.Body != "" {
		t.Errorf("decision also names a prose field: %q", decision.Body)
	}
	if want := []string{"statement", "body.decision"}; !reflect.DeepEqual(decision.Hash, want) {
		t.Errorf("decision hash = %v, want %v — the decision section binds and the rest informs", decision.Hash, want)
	}
}

// A corpus with no schema directory is a corpus that checks nothing, not a
// corpus that fails.
func TestNoSchemaDirectoryIsAnEmptySet(t *testing.T) {
	set, err := Load(filepath.Join(t.TempDir(), "absent"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(set.Types()) != 0 {
		t.Errorf("Types = %v, want none", set.Types())
	}
}

// Leniency is not the absence of checking: what the schema declares is checked,
// and what it does not declare is left alone.
func TestCheckReportsWhatTheSchemaDeclares(t *testing.T) {
	sc, _ := shipped(t).For("promise")

	cases := []struct {
		name   string
		fields map[string]any
		want   []Violation
	}{{
		name: "a complete promise has nothing to report",
		fields: map[string]any{
			"statement": "the tests run",
			"gather":    map[string]any{"tests": map[string]any{}},
			"hold_when": "obs.tests.ok",
			"trigger":   "cadence",
		},
	}, {
		name:   "a required field nobody wrote",
		fields: map[string]any{"trigger": "cadence"},
		want:   []Violation{{Kind: MissingRequired, Field: "statement"}},
	}, {
		name:   "a required field written empty is not written",
		fields: map[string]any{"statement": ""},
		want:   []Violation{{Kind: MissingRequired, Field: "statement"}},
	}, {
		name:   "a value outside the enum",
		fields: map[string]any{"statement": "s", "trigger": "on-change"},
		want:   []Violation{{Kind: NotInEnum, Field: "trigger"}},
	}, {
		name:   "a value of the wrong type",
		fields: map[string]any{"statement": "s", "gather": "not a map"},
		want:   []Violation{{Kind: WrongType, Field: "gather"}},
	}, {
		name: "a field nothing declares is not a violation",
		fields: map[string]any{
			"statement": "the tests run",
			"owner":     "paul",
			"anything":  map[string]any{"at": "all"},
		},
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sc.Check(tc.fields)
			if len(got) != len(tc.want) {
				t.Fatalf("Check = %v, want %d violations", got, len(tc.want))
			}
			for i, w := range tc.want {
				if got[i].Kind != w.Kind || got[i].Field != w.Field {
					t.Errorf("violation %d = %s on %q, want %s on %q", i, got[i].Kind, got[i].Field, w.Kind, w.Field)
				}
				if got[i].Detail == "" {
					t.Errorf("violation %d has no detail", i)
				}
			}
		})
	}
}

// Check reads the fields and never writes them: an absent field with a declared
// default stays absent, because filling it in would put a value in the artifact
// that its author did not write.
func TestCheckDoesNotFillInADefault(t *testing.T) {
	sc, _ := shipped(t).For("promise")
	fields := map[string]any{"statement": "the tests run"}

	sc.Check(fields)

	if _, present := fields["trigger"]; present {
		t.Error("Check wrote the trigger default into the artifact's fields")
	}
	if len(fields) != 1 {
		t.Errorf("fields = %v, want only what was written", fields)
	}
}

func TestUndeclaredNamesWhatNothingChecks(t *testing.T) {
	sc, _ := shipped(t).For("promise")
	fields := map[string]any{"statement": "s", "owner": "paul", "review_every": "90d"}

	if want := []string{"owner", "review_every"}; !reflect.DeepEqual(sc.Undeclared(fields), want) {
		t.Errorf("Undeclared = %v, want %v", sc.Undeclared(fields), want)
	}
	if !sc.Declares("statement") || sc.Declares("owner") {
		t.Error("Declares disagrees with Undeclared")
	}
}

func TestAllowsRelation(t *testing.T) {
	promise, _ := shipped(t).For("promise")
	if !promise.AllowsRelation("serves") {
		t.Error("promise refuses a relation it declares")
	}
	if promise.AllowsRelation("about") {
		t.Error("promise allows a relation it does not declare")
	}
	// An inline body link states no relation, and an unstated one is always
	// allowed — there is nothing to disagree with.
	if !promise.AllowsRelation("") {
		t.Error("an unstated relation was refused")
	}

	// A schema that bounds nothing bounds nothing.
	desire, _ := shipped(t).For("desire")
	if !desire.AllowsRelation("anything-at-all") {
		t.Error("a schema with no allowed_relations refused a relation")
	}
}

func TestBindsAnswersWhatTheHashCovers(t *testing.T) {
	set := shipped(t)

	promise, _ := set.For("promise")
	if !promise.Binds("hold_when") {
		t.Error("a promise does not bind on hold_when")
	}
	if promise.Binds("repair") {
		t.Error("a promise binds on repair")
	}

	// For canon the prose is the commitment, so the body binds.
	canon, _ := set.For("canon")
	if !canon.Binds(HashBody) {
		t.Error("canon does not bind on its body")
	}
	// And where a canon entry carries one, the instrument binds too. Without
	// these two a ratified canon entry's gatherer could be replaced with one
	// that measures nothing and its predicate with the literal true, and its
	// standing would survive — which is what the identical edit to a promise
	// does not do. See docs/decision/0045.
	if !canon.Binds("gather") {
		t.Error("canon does not bind on gather, so a ratified entry's instrument can be swapped without dropping standing")
	}
	if !canon.Binds("hold_when") {
		t.Error("canon does not bind on hold_when, so a ratified entry's predicate can be replaced without dropping standing")
	}

	// A desire cannot be ratified in place, so nothing about it binds.
	desire, _ := set.For("desire")
	if len(desire.Hash) != 0 {
		t.Errorf("desire binds on %v", desire.Hash)
	}
}

// A schema that cannot be parsed is an error. Every artifact of the kind would
// otherwise be validated against nothing, and a check that never fires is not
// a check.
func TestAnUnparseableSchemaIsAnError(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "promise.yaml"), "type: promise\nfields: [unclosed\n")

	if _, err := Load(dir); err == nil {
		t.Error("an unparseable schema loaded")
	}
}

func TestTwoSchemasDeclaringOneTypeIsAnError(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "promise.yaml"), "type: promise\n")
	write(t, filepath.Join(dir, "also-promise.yaml"), "type: promise\n")

	if _, err := Load(dir); err == nil {
		t.Error("two schemas declaring one type loaded")
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
