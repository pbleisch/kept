package corpus

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const shapingDesireSchema = `type: desire
hash: []
fields:
  want:         { type: string, required: true }
  becomes:      { type: enum, values: [promise, policy, canon] }
  candidate:    { type: map }
  ripe_when:    { type: string }
  reopens_when: { type: string }
body: note
`

const shapingPromiseSchema = `type: promise
hash: [statement, gather, hold_when, trigger, repair]
fields:
  statement: { type: string, required: true }
  gather:    { type: map }
  hold_when: { type: string }
  trigger:   { type: enum, values: [cadence, on-act, once] }
  max_age:   { type: string }
  repair:    { type: map }
body: why
`

// shapingCorpus writes a corpus holding the two schemas and whatever files are
// given, and loads it against a catalogue that has one gatherer.
func shapingCorpus(t *testing.T, files map[string]string) *Corpus {
	t.Helper()
	root := t.TempDir()
	files["_schema/desire.yaml"] = shapingDesireSchema
	if _, given := files["_schema/promise.yaml"]; !given {
		files["_schema/promise.yaml"] = shapingPromiseSchema
	}
	for name, body := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	c, err := Load(root, root, Options{Gatherers: catalogue{"go/tests": {"ok", "failed"}}})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func desireWith(fields string) string {
	return "---\nid: d\nfields:\n  want: repair picks what to spend a session on\n" + fields + "---\n"
}

func shaping(t *testing.T, c *Corpus) (string, string) {
	t.Helper()
	return c.Shaping(get(t, c, "desire:d"))
}

const readingCandidate = `  becomes: promise
  candidate:
    statement: a tick that could act on several drifts records which it ranked first
    gather:
      tests: { gatherer: go/tests }
    hold_when: obs.tests.failed == 0
    max_age: 7d
`

// What a desire says it would become reads, or it does not, and each way it can
// fail to read says which. A desire that names nothing is the ordinary state of
// one somebody has just captured.
func TestADesireSaysWhyItDoesNotReadYet(t *testing.T) {
	cases := []struct {
		name, fields, becomes, unread string
	}{
		{"names nothing", "", "", "names nothing it would become"},
		{"a kind and no candidate", "  becomes: promise\n", "promise", "carries no candidate"},
		{"a candidate and no kind", "  candidate: { statement: x }\n", "", "names no kind"},
		{"a kind nothing declares", "  becomes: canon\n  candidate: { statement: x }\n", "canon", "nothing declares that kind"},
		{"a kind a desire cannot become", "  becomes: document\n  candidate: { summary: x }\n", "document", "becomes one of promise, policy, canon"},
		{"a candidate that is not a map", "  becomes: promise\n  candidate: every package has a test\n", "promise", "fields of a promise are a map"},
		{"a candidate missing a required field", "  becomes: promise\n  candidate: { hold_when: obs.tests.failed == 0 }\n", "promise", "statement: required by promise"},
		{"a max_age that does not parse", strings.Replace(readingCandidate, "7d", "weekly", 1), "promise", "max_age"},
		{"a gatherer nothing answers to", strings.Replace(readingCandidate, "go/tests", "go/nothing", 1), "promise", "there is no such gatherer"},
		{"a predicate that does not parse", strings.Replace(readingCandidate, "obs.tests.failed == 0", "obs.tests.failed ==", 1), "promise", "hold_when"},
		{"a predicate reading what nothing gathers", strings.Replace(readingCandidate, "obs.tests.failed", "obs.lint.failed", 1), "promise", "lint"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := shapingCorpus(t, map[string]string{"desire/d.md": desireWith(tc.fields)})
			becomes, unread := shaping(t, c)
			if becomes != tc.becomes {
				t.Errorf("becomes = %q, want %q", becomes, tc.becomes)
			}
			if !strings.Contains(unread, tc.unread) {
				t.Errorf("unread = %q, want it to say %q", unread, tc.unread)
			}
		})
	}

	c := shapingCorpus(t, map[string]string{"desire/d.md": desireWith(readingCandidate)})
	if becomes, unread := shaping(t, c); becomes != "promise" || unread != "" {
		t.Fatalf("a candidate that reads as a promise was not shaped: becomes %q, unread %q", becomes, unread)
	}
}

// A candidate that does not read is a draft, and a draft is not a defect. The
// desire loads with nothing to say about it, and the reason travels with its
// state instead.
func TestACandidateThatDoesNotReadIsNotACorpusProblem(t *testing.T) {
	c := shapingCorpus(t, map[string]string{
		"desire/d.md": desireWith("  becomes: promise\n  candidate: { hold_when: obs.nothing.ok }\n"),
	})
	if _, unread := shaping(t, c); unread == "" {
		t.Fatal("the candidate read, so this proves nothing")
	}
	for _, p := range c.Problems {
		t.Errorf("a candidate that does not read was reported: %s: %s", p.Kind, p.Detail)
	}
}

// **Shaped is the load's own judgment, and this is what holds it to that.** Each
// candidate is also written out as a promise of its own, and the two answers
// must agree: a candidate shapes exactly when the same fields load as a promise
// with nothing wrong. A rule for shaped that drifted from the rule for loads
// would pass every case above and fail here.
func TestACandidateShapesExactlyWhenItsFieldsWouldLoad(t *testing.T) {
	candidates := map[string]string{
		"reads":             readingCandidate,
		"missing-statement": "  becomes: promise\n  candidate:\n    hold_when: obs.tests.failed == 0\n    gather: { tests: { gatherer: go/tests } }\n",
		"unknown-gatherer":  strings.Replace(readingCandidate, "go/tests", "go/nothing", 1),
		"wrong-arg":         strings.Replace(readingCandidate, "{ gatherer: go/tests }", "{ gatherer: go/tests, wrong: true }", 1),
		"bad-trigger":       readingCandidate + "    trigger: hourly\n",
		"bad-age":           strings.Replace(readingCandidate, "7d", "soon", 1),
		"bad-predicate":     strings.Replace(readingCandidate, "== 0", "==", 1),
		"ungathered":        strings.Replace(readingCandidate, "obs.tests.failed", "obs.vet.failed", 1),
	}

	for name, fields := range candidates {
		t.Run(name, func(t *testing.T) {
			c := shapingCorpus(t, map[string]string{
				"desire/d.md":  desireWith(fields),
				"promise/p.md": asPromise(t, fields),
			})
			_, unread := shaping(t, c)
			shaped := unread == ""
			loads := len(everyProblemOf(c, "promise:p")) == 0
			if shaped != loads {
				t.Fatalf("shaped = %v (%q) and the same fields as a promise load clean = %v: %v",
					shaped, unread, loads, everyProblemOf(c, "promise:p"))
			}
		})
	}
}

// asPromise lifts a candidate block out of a desire's fields and writes it as a
// promise's own. The candidate is indented one level deeper than a promise's
// fields, so the lift is an outdent.
func asPromise(t *testing.T, fields string) string {
	t.Helper()
	_, block, ok := strings.Cut(fields, "  candidate:\n")
	if !ok {
		t.Fatalf("no candidate block in %q", fields)
	}
	var b strings.Builder
	b.WriteString("---\nid: p\nfields:\n")
	for line := range strings.SplitSeq(strings.TrimRight(block, "\n"), "\n") {
		b.WriteString(strings.TrimPrefix(line, "  ") + "\n")
	}
	b.WriteString("---\n")
	return b.String()
}

func everyProblemOf(c *Corpus, id string) []string {
	var out []string
	for _, p := range c.Problems {
		if p.ID == id {
			out = append(out, string(p.Kind)+": "+p.Detail)
		}
	}
	return out
}
