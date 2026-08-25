package gather

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/run"
	"github.com/pbleisch/kept/internal/workspace"
)

// module is a workspace holding a Go module, for the two gatherers that parse
// one.
type module struct {
	t    *testing.T
	root string
}

func newModule(t *testing.T, name string) *module {
	t.Helper()
	m := &module{t: t, root: t.TempDir()}
	if name != "" {
		m.file("go.mod", "module "+name+"\n\ngo 1.26\n")
	}
	return m
}

func (m *module) file(name, body string) {
	m.t.Helper()
	write(m.t, filepath.Join(m.root, filepath.FromSlash(name)), body)
}

// gather runs one gatherer over the whole module and returns what it measured.
func (m *module) gather(g Gatherer, args map[string]any) (Fields, error) {
	m.t.Helper()
	tree, err := workspace.Read(m.root, workspace.Options{})
	if err != nil {
		m.t.Fatal(err)
	}
	req := Requests{Tree: tree, World: model.World{Tree: tree.Digest()}}.For(g, args)
	return g.Gather(context.Background(), req)
}

// measured is the gather as the tick sees it, so the rule that a failure
// leaves no observation is tested where it lives rather than restated.
func (m *module) measured(g Gatherer, args map[string]any) model.Observation {
	m.t.Helper()
	tree, err := workspace.Read(m.root, workspace.Options{})
	if err != nil {
		m.t.Fatal(err)
	}
	req := Requests{Tree: tree, World: model.World{Tree: tree.Digest()}}.For(g, args)
	return measure(context.Background(), g, req)
}

func (m *module) tidy() {
	m.t.Helper()
	m.file("internal/corpus/corpus.go", `package corpus

import (
	"os"
	"sort"

	"example.com/mod/internal/model"
)

type Corpus struct{ E []model.Entry }

func (c *Corpus) Read(p string) ([]byte, error) { sort.Strings(nil); return os.ReadFile(p) }
`)
	m.file("internal/ledger/ledger.go", `package ledger

import "example.com/mod/internal/corpus"

const Ratified = "ratified"

type Ledger struct{ C *corpus.Corpus }

func (l *Ledger) Ratify() string { return Ratified }
`)
	m.file("internal/model/model.go", "package model\n\ntype Entry struct{ ID string }\n")
	m.file("cmd/tool/main.go", `package main

import (
	"os"

	"example.com/mod/internal/ledger"
)

func main() { os.WriteFile("x", []byte(ledger.Ratified), 0o644) }
`)
}

// The graph is read out of the source, so it answers over a tree that does not
// compile — which is the tree a repair agent is standing in, and the whole
// reason docs/decision/0049 parses rather than shelling out to the toolchain.
func TestImportsAnswersOverATreeThatDoesNotCompile(t *testing.T) {
	m := newModule(t, "example.com/mod")
	m.tidy()
	// A cycle and a call to a function nothing declares. `go list` exits
	// non-zero here and prints nothing, which is the false-green 0049 opens on.
	m.file("internal/model/model.go", `package model

import "example.com/mod/internal/ledger"

type Entry struct{ ID string }

func Broken() string { return ledger.NoSuchSymbol() }
`)

	f, err := m.gather(imports{}, nil)
	if err != nil {
		t.Fatalf("a tree that does not compile could not be measured: %v", err)
	}
	if f["ok"] != true {
		t.Error("a tree that parses and does not compile reports ok false")
	}
	if !hasEdge(f, "example.com/mod/internal/model", "example.com/mod/internal/ledger") {
		t.Errorf("the edge the cycle is made of is missing: %v", f["edges"])
	}
	if !reaches_(f, "example.com/mod/internal/corpus", "example.com/mod/internal/ledger") {
		t.Error("the transitive closure does not terminate on a cycle, or does not report it")
	}
}

// A file that will not parse is a **failure to measure**, and that is the line
// between it and a tree that does not compile: a syntax error yields a partial
// AST, and reading imports out of one would under-approximate the edge set
// silently — a boundary reported kept when it was crossed.
func TestAFileThatWillNotParseIsUnknownAndNotAnEmptyGraph(t *testing.T) {
	for _, g := range []Gatherer{imports{}, symbols{}} {
		m := newModule(t, "example.com/mod")
		m.tidy()
		m.file("internal/corpus/corpus.go", "package corpus\n\nfunc Broken() { this is not go }\n")

		obs := m.measured(g, nil)
		if obs.Err == nil {
			t.Fatalf("%s read a file that is not Go as a measurement", g.Name())
		}
		var unparsable *Unparsable
		if !errors.As(obs.Err, &unparsable) {
			t.Errorf("%s reports %v rather than a file it could not read", g.Name(), obs.Err)
		}
		assertNoObservation(t, g, obs)
	}
}

// A tree with no go.mod has no import paths, so there is no graph to be right
// or wrong about. Reporting an empty one would say a boundary holds in a tree
// where the question was never put.
func TestNoModuleIsUnknownAndNotAnEmptyGraph(t *testing.T) {
	for _, g := range []Gatherer{imports{}, symbols{}} {
		m := newModule(t, "")
		m.file("internal/corpus/corpus.go", "package corpus\n")

		obs := m.measured(g, nil)
		if obs.Err == nil {
			t.Fatalf("%s measured a tree with no module", g.Name())
		}
		var missing *NoModule
		if !errors.As(obs.Err, &missing) {
			t.Errorf("%s reports %v rather than a missing module", g.Name(), obs.Err)
		}
		assertNoObservation(t, g, obs)
	}
}

// symbols answers about identifiers and not about bytes, which is the whole
// reason it exists: the promise it was built for found seventeen hits with a
// pattern and every one of them was a doc comment.
func TestSymbolsDoesNotSeeAWordInAComment(t *testing.T) {
	m := newModule(t, "example.com/mod")
	m.file("internal/model/model.go", `package model

// Gatherer is name and version, "tests@2", and os.WriteFile is mentioned here
// on purpose. A pattern over these bytes matches; a parse does not.
const Note = "os.WriteFile"

type Entry struct{ ID string }
`)
	m.file("internal/gather/gather.go", `package gather

import "os"

func Write(p string, b []byte) error { return os.WriteFile(p, b, 0o644) }
`)

	f, err := m.gather(symbols{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	writers := map[string]bool{}
	for _, item := range f["refs"].([]any) {
		r := item.(map[string]any)
		if r["path"] == "os" && r["name"] == "WriteFile" {
			writers[r["package"].(string)] = true
		}
	}
	if !writers["example.com/mod/internal/gather"] {
		t.Error("the package that calls os.WriteFile does not report it")
	}
	if writers["example.com/mod/internal/model"] {
		t.Error("a doc comment and a string literal read as a call, which is the instrument this replaces")
	}
}

// The vocabulary a package declares, rather than the words its files contain.
func TestSymbolsReportsWhatAPackageDeclares(t *testing.T) {
	m := newModule(t, "example.com/mod")
	m.file("internal/model/model.go", `package model

const Tests = 1

var Command = "x"

type Entry struct{ ID string }

func Read() {}

func (e Entry) Name() string { return e.ID }
`)
	f, err := m.gather(symbols{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"Tests": "const", "Command": "var", "Entry": "type", "Read": "func", "Name": "method"}
	got := map[string]string{}
	for _, item := range f["defines"].([]any) {
		d := item.(map[string]any)
		got[d["name"].(string)] = d["kind"].(string)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the package declares %v, want %v", got, want)
	}
}

// The default is the package that ships. A test helper writing a temporary
// file is not the package writing to the corpus, and a boundary promise is
// about what ships — but `tests: true` says so and the argument binds.
func TestTestFilesAreExcludedUntilAPromiseAsksForThem(t *testing.T) {
	m := newModule(t, "example.com/mod")
	m.file("internal/gather/gather.go", "package gather\n\nfunc Read() {}\n")
	m.file("internal/gather/gather_test.go", `package gather

import "os"

func helper() { os.WriteFile("x", nil, 0o644) }
`)

	shipped, err := m.gather(symbols{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if writes(shipped) {
		t.Error("a _test.go file is counted by default, so a boundary promise answers about tests")
	}

	withTests, err := m.gather(symbols{}, map[string]any{argTests: true})
	if err != nil {
		t.Fatal(err)
	}
	if !writes(withTests) {
		t.Error("tests: true does not include _test.go files")
	}
	if _, err := (symbols{}).Declares(map[string]any{argTests: "yes"}); err == nil {
		t.Error("tests: is not a boolean and is accepted")
	}
}

// An observation is addressed by its data, so an order out of map iteration
// would mint a fresh record on every tick of an unchanging world and the dedup
// the whole content-addressed layout exists for would quietly stop working.
//
// Go randomizes map iteration, so a fold that forgot to sort fails this within
// a few rounds rather than eventually.
func TestTheseGatherersFoldInAStableOrder(t *testing.T) {
	m := newModule(t, "example.com/mod")
	m.tidy()
	// Enough packages and enough references that an unsorted fold cannot come
	// out the same way twice by luck.
	for _, name := range []string{"a", "b", "c", "d", "e", "f"} {
		m.file("internal/"+name+"/"+name+".go", `package `+name+`

import (
	"os"
	"sort"
	"strings"

	"example.com/mod/internal/model"
)

type T struct{ E model.Entry }

const A, B, C = 1, 2, 3

func F() { sort.Strings(nil); strings.TrimSpace(""); os.Getenv("X") }
`)
	}

	for _, g := range []Gatherer{imports{}, symbols{}, corpusGatherer{}} {
		if g.Name() == "corpus@1" {
			m.file(".kept/config.yaml", "corpus: docs\n")
			m.file("docs/_schema/promise.yaml", "type: promise\nhash: [statement]\nfields:\n  statement: { type: string }\n")
			m.file("docs/promise/one.md", "---\nid: one\ntype: promise\nfields:\n  statement: a\n---\n\nWhy.\n")
		}
		first := encode(t, mustGather(t, m, g))
		for i := 0; i < 8; i++ {
			if again := encode(t, mustGather(t, m, g)); again != first {
				t.Fatalf("%s folds differently on round %d, so an unchanging world mints a new observation every tick", g.Name(), i)
			}
		}
	}
}

// corpus reads a corpus and never the ledger. Ratifying moves nothing in
// `World.Tree` — `.kept/` is excluded from it — so data that moved with a
// ratification would move without the world moving, which breaks dedup
// silently.
func TestCorpusDeclaresNoLedgerReadAndItsDataDoesNotMoveWithOne(t *testing.T) {
	if len((corpusGatherer{}).Reads()) != 0 {
		t.Fatal("the corpus gatherer declares a ledger read, which 0049 refuses for it")
	}

	m := newModule(t, "example.com/mod")
	m.file(".kept/config.yaml", "corpus: docs\n")
	m.file("docs/_schema/promise.yaml", "type: promise\nhash: [statement]\nfields:\n  statement: { type: string }\n")
	m.file("docs/promise/one.md", "---\nid: one\ntype: promise\nfields:\n  statement: a\n---\n\nWhy.\n")

	before := encode(t, mustGather(t, m, corpusGatherer{}))

	store := ledger.NewFiles(filepath.Join(m.root, ".kept"))
	l := ledger.Open(store, run.Start("test", nil))
	if _, err := l.Ratify("promise:one", "sha256:"+strings.Repeat("a", 64), 1, "A Person", "", ledger.Where{}); err != nil {
		t.Fatal(err)
	}
	if err := l.Close(run.Clean); err != nil {
		t.Fatal(err)
	}

	if after := encode(t, mustGather(t, m, corpusGatherer{})); after != before {
		t.Error("a ratification moved the corpus gatherer's data, and nothing in the world moved with it")
	}
}

// A corpus root that is not one is a failure to measure. Reporting zero
// artifacts and nothing wrong would say a promise about a corpus holds in a
// tree where no corpus was read.
func TestNoCorpusIsUnknownAndNotAnEmptyCorpus(t *testing.T) {
	m := newModule(t, "example.com/mod")
	m.file("internal/model/model.go", "package model\n")

	obs := m.measured(corpusGatherer{}, nil)
	if obs.Err == nil {
		t.Fatal("a workspace with no corpus was measured as one holding nothing")
	}
	assertNoObservation(t, corpusGatherer{}, obs)

	if _, err := (corpusGatherer{}).Declares(map[string]any{argRoot: "/etc"}); err == nil {
		t.Error("an absolute corpus root is accepted, so two machines would measure different worlds")
	}
	if _, err := (corpusGatherer{}).Declares(map[string]any{argRoot: ""}); err == nil {
		t.Error("an empty corpus root is accepted")
	}
}

// A failed gather leaves no observation at all, which is what makes the verdict
// unknown rather than not-kept. The rule lives in Record, so this asserts it
// there rather than trusting each gatherer to have remembered.
func assertNoObservation(t *testing.T, g Gatherer, obs model.Observation) {
	t.Helper()
	if obs.Data != nil {
		t.Errorf("%s failed and still carries data", g.Name())
	}
	if obs.ID != "" {
		t.Errorf("%s failed and was still given an address", g.Name())
	}
	address, recorded, err := Record(nil, obs)
	if err != nil || recorded || address != "" {
		t.Errorf("%s failed and something was filed for it: %q %v %v", g.Name(), address, recorded, err)
	}
}

func mustGather(t *testing.T, m *module, g Gatherer) Fields {
	t.Helper()
	f, err := m.gather(g, nil)
	if err != nil {
		t.Fatalf("%s: %v", g.Name(), err)
	}
	return f
}

func encode(t *testing.T, f Fields) string {
	t.Helper()
	b, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func hasEdge(f Fields, from, to string) bool {
	for _, item := range f["edges"].([]any) {
		e := item.(map[string]any)
		if e["from"] == from && e["to"] == to {
			return true
		}
	}
	return false
}

func reaches_(f Fields, pkg, target string) bool {
	for _, item := range f["packages"].([]any) {
		p := item.(map[string]any)
		if p["package"] != pkg {
			continue
		}
		for _, d := range p["deps"].([]any) {
			if d == target {
				return true
			}
		}
	}
	return false
}

func writes(f Fields) bool {
	for _, item := range f["refs"].([]any) {
		r := item.(map[string]any)
		if r["path"] == "os" && r["name"] == "WriteFile" {
			return true
		}
	}
	return false
}
