package tick

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/canonical"
	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/gather"
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/run"
)

// fakeFlag makes the test binary run as the program a gatherer runs.
//
// It is the test binary because that is the one executable a test is guaranteed
// to have, on every platform, with no shell and nothing to build — and because
// a tick's behaviour is about programs that answer, fail, or are not there at
// all, none of which is reproducible against a toolchain that has to be
// installed.
const fakeFlag = "-kept.fake"

func TestMain(m *testing.M) {
	for i, arg := range os.Args {
		if arg == fakeFlag {
			os.Exit(fake(os.Args[i+1:]))
		}
	}
	os.Exit(m.Run())
}

// fake reads a file and prints it, which is enough to be a gatherer that
// answers differently as the world changes.
func fake(args []string) int {
	if len(args) != 2 || args[0] != "read" {
		return 2
	}
	src, err := os.ReadFile(args[1])
	if err != nil {
		return 1
	}
	fmt.Print(string(src))
	return 0
}

// runLine builds a run: line invoking the program at path, quoted because a
// temporary directory's path is not guaranteed to be free of anything the argv
// reader treats as a separator.
func runLine(t *testing.T, path string, args ...string) string {
	t.Helper()
	parts := append([]string{path, fakeFlag}, args...)
	for i, p := range parts {
		if strings.Contains(p, "'") {
			t.Fatalf("the fake program cannot be given %q", p)
		}
		parts[i] = "'" + p + "'"
	}
	return strings.Join(parts, " ")
}

const schemaFile = `type: promise
hash: [statement, gather, hold_when, trigger]
fields:
  statement: { type: string, required: true }
  gather:    { type: map }
  hold_when: { type: string }
  trigger:   { type: enum, values: [cadence, on-act, once] }
  max_age:   { type: string }
body: why
`

// box is a workspace with a corpus in docs/ and a .kept beside it, which is the
// layout .kept/config.yaml describes.
type box struct {
	t    *testing.T
	root string
	// tool is where a promise's program lives. It is outside the workspace on
	// purpose: installing a toolchain is not a change to the world any promise
	// looks at, which is exactly why --all exists.
	tool string
}

func newBox(t *testing.T) *box {
	t.Helper()
	b := &box{t: t, root: t.TempDir(), tool: filepath.Join(t.TempDir(), "tool")}
	b.write(".kept/config.yaml", "corpus: docs\n")
	b.write("docs/_schema/promise.yaml", schemaFile)
	return b
}

// install puts the program a promise names where the promise says it is. Until
// it is called there is no such program, which is what a missing toolchain is.
func (b *box) install() {
	b.t.Helper()
	self, err := os.Executable()
	if err != nil {
		b.t.Fatalf("the test binary cannot find itself: %v", err)
	}
	if err := os.Symlink(self, b.tool); err != nil {
		src, readErr := os.ReadFile(self)
		if readErr != nil {
			b.t.Fatalf("no stand-in for the program: %v / %v", err, readErr)
		}
		if err := os.WriteFile(b.tool, src, 0o700); err != nil {
			b.t.Fatal(err)
		}
	}
}

func (b *box) write(name, body string) {
	b.t.Helper()
	path := filepath.Join(b.root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		b.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		b.t.Fatal(err)
	}
}

func (b *box) remove(name string) {
	b.t.Helper()
	if err := os.Remove(filepath.Join(b.root, filepath.FromSlash(name))); err != nil {
		b.t.Fatal(err)
	}
}

// promise writes one that reads a file through the fake program: kept when the
// file says ok, not-kept when it says anything else, unknown when the program
// cannot be run at all.
func (b *box) promise(slug, scope, program, maxAge string) {
	b.t.Helper()
	age := ""
	if maxAge != "" {
		age = "  max_age: " + maxAge + "\n"
	}
	b.write("docs/promise/"+slug+".md", "---\nid: "+slug+"\ntype: promise\ntitle: "+slug+`
fields:
  statement: the marker under `+scope+` says ok
  gather:
    marker:
      gatherer: command
      run: "`+program+`"
      scope: "`+scope+`"
      fields:
        ok: "'ok' in lines(stdout)"
  hold_when: obs.marker.ok
  trigger: cadence
`+age+`---

Why: because a promise nobody can check is not one.
`)
}

func (b *box) store() *ledger.Files {
	return ledger.NewFiles(filepath.Join(b.root, corpus.ConfigDir))
}

func (b *box) load() *corpus.Corpus {
	b.t.Helper()
	c, err := corpus.Load(filepath.Join(b.root, "docs"), b.root, corpus.Options{Gatherers: gather.Builtin()})
	if err != nil {
		b.t.Fatal(err)
	}
	return c
}

// tick runs one pass that observes and records.
func (b *box) tick(opts ...func(*Options)) *Result {
	b.t.Helper()
	c := b.load()
	store := b.store()
	l := ledger.Open(store, run.Start("test", nil))

	t := &Tick{Corpus: c, Store: store, Catalogue: gather.Builtin(), Ledger: l}
	t.Options.Deadline = 30 * time.Second
	for _, opt := range opts {
		opt(&t.Options)
	}

	res, err := t.Run(b.t.Context())
	if err != nil {
		b.t.Fatalf("the tick failed: %v", err)
	}
	if err := l.Close(run.Clean); err != nil {
		b.t.Fatal(err)
	}
	return res
}

// survey runs one pass that observes nothing.
func (b *box) survey() *Result {
	b.t.Helper()
	t := &Tick{Corpus: b.load(), Store: b.store(), Catalogue: gather.Builtin()}
	res, err := t.Survey()
	if err != nil {
		b.t.Fatalf("the survey failed: %v", err)
	}
	return res
}

// ratify grants standing the way a person does, which is the only way it is
// created.
func (b *box) ratify(slug string) {
	b.t.Helper()
	c := b.load()
	entry, err := c.Lookup(slug)
	if err != nil {
		b.t.Fatal(err)
	}
	sc, _ := c.Schemas.For(string(entry.Artifact.Kind))
	hash, err := canonical.Bind(entry, sc)
	if err != nil {
		b.t.Fatal(err)
	}
	l := ledger.Open(b.store(), run.Start("test", nil))
	if _, err := l.Ratify(entry.Artifact.ID, hash.String(), hash.Norm, "the operator", "", ledger.Where{}); err != nil {
		b.t.Fatal(err)
	}
	if err := l.Close(run.Clean); err != nil {
		b.t.Fatal(err)
	}
}

// subject is one artifact out of a pass.
func (r *Result) subject(t *testing.T, id string) Subject {
	t.Helper()
	for _, s := range r.Subjects {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("%s is not in the register", id)
	return Subject{}
}

// truthLines reads an artifact's truth stream back as it was written, which is
// how a test counts records rather than verdicts.
func (b *box) truthLines(id string) []ledger.Truth {
	b.t.Helper()
	records, quarantined, err := ledger.ReadTruth(b.store(), id)
	if err != nil {
		b.t.Fatal(err)
	}
	if len(quarantined) > 0 {
		b.t.Fatalf("%d truth lines could not be read", len(quarantined))
	}
	return records
}

func (b *box) dispositionLines(id string) []ledger.Disposition {
	b.t.Helper()
	records, quarantined, err := ledger.ReadDispositions(b.store(), id)
	if err != nil {
		b.t.Fatal(err)
	}
	if len(quarantined) > 0 {
		b.t.Fatalf("%d disposition lines could not be read", len(quarantined))
	}
	return records
}

// backdate moves every record in a stream into the past, so that a heartbeat
// can be reached without a test sleeping for a week.
//
// It rewrites the file rather than injecting a clock, because the rule under
// test is about what the tick does with the timestamps the ledger actually
// wrote — and a clock the tick could be handed is a clock a caller could get
// wrong.
func (b *box) backdate(stream string, by time.Duration) {
	b.t.Helper()
	path := filepath.Join(b.root, corpus.ConfigDir, ledger.Dir, filepath.FromSlash(stream))
	src, err := os.ReadFile(path)
	if err != nil {
		b.t.Fatal(err)
	}
	var out strings.Builder
	for _, line := range strings.Split(strings.TrimRight(string(src), "\n"), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			b.t.Fatal(err)
		}
		at, err := time.Parse(time.RFC3339Nano, record["at"].(string))
		if err != nil {
			b.t.Fatal(err)
		}
		record["at"] = at.Add(-by).Format(time.RFC3339Nano)
		again, err := json.Marshal(record)
		if err != nil {
			b.t.Fatal(err)
		}
		out.Write(again)
		out.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(out.String()), 0o644); err != nil {
		b.t.Fatal(err)
	}
}
