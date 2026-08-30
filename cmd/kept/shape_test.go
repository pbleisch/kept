package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// shapeSchema declares the fields a trialled promise needs. It is the fixture
// schema plus `repair`, because a candidate under trial is a candidate for
// ratification and those are the fields one carries.
const shapeSchema = `type: promise
hash: [statement, gather, hold_when, trigger, repair]
fields:
  statement: { type: string, required: true }
  gather:    { type: map }
  hold_when: { type: string }
  trigger:   { type: enum, values: [cadence, on-act, once] }
  repair:    { type: map }
body: why
`

// candidate is a promise that runs one program and reads one field out of it.
// The program is `cat`, so the measurement is a function of a file in the tree
// and of nothing else — which is what lets a test put the tree in one state,
// trial, put it in another, and attribute the difference.
func candidate(slug, holdWhen string) string {
	return `---
id: ` + slug + `
type: promise
title: ` + slug + `
fields:
  statement: a candidate nobody has granted anything
  gather:
    subject:
      gatherer: command
      run: cat subject.txt
      scope: ["subject.txt"]
      fields:
        words: >-
          map(find_all(stdout, '(?P<w>[a-z]+)'), m => m.w)
  hold_when: >
    ` + holdWhen + `
  trigger: cadence
---

Prose that binds nothing.
`
}

func shapeWorkspace(t *testing.T, slug, holdWhen, subject string) *workspace {
	t.Helper()
	w := &workspace{t: t, root: t.TempDir()}
	w.put(".kept/config.yaml", configFile)
	w.put("docs/_schema/promise.yaml", shapeSchema)
	w.put("docs/promise/"+slug+".md", candidate(slug, holdWhen))
	w.put("subject.txt", subject)
	return w
}

// The reading a trial is for: the measurement, printed beside the verdict.
//
// A predicate that folds an empty collection is true, so the word `kept` is the
// same word whether an instrument measured the world or measured nothing. That
// is the failure this verb exists to make visible before somebody ratifies it,
// and the only thing that separates the two cases is the observation — so this
// asserts the empty measurement is on the page, not merely that the verdict is.
func TestATrialOverAnEmptyMeasurementPrintsTheEmptyMeasurement(t *testing.T) {
	w := shapeWorkspace(t, "vacuous", "all(obs.subject.words, x => x == 'never')", "")

	code, stdout, stderr := w.run("shape", "promise:vacuous", "--trial", "--quiet")
	if code != exitOK {
		t.Fatalf("shape exited %d: %s", code, stderr)
	}
	mustContain(t, "an empty trial", stdout, "kept", "words", "[]")

	// The same predicate over a measurement that is not empty says the other
	// thing, which is what makes the line above evidence rather than a shape.
	w.put("subject.txt", "something")
	code, stdout, stderr = w.run("shape", "promise:vacuous", "--trial", "--quiet")
	if code != exitOK {
		t.Fatalf("shape exited %d: %s", code, stderr)
	}
	mustContain(t, "a trial over a real measurement", stdout, "not-kept", "something")
}

// A trial records nothing. This is the whole reason it is tick.Measure and not
// a tick with the recording turned off, so it is asserted against the ledger
// rather than against the code path.
func TestATrialRecordsNothing(t *testing.T) {
	w := shapeWorkspace(t, "records-nothing", "count(obs.subject.words, x => true) > 0", "a word")

	before := w.ledgerState(t)
	if code, _, stderr := w.run("shape", "promise:records-nothing", "--trial", "--quiet"); code != exitOK {
		t.Fatalf("shape exited %d: %s", code, stderr)
	}
	if after := w.ledgerState(t); after != before {
		t.Errorf("a trial wrote to the ledger:\nbefore %q\nafter  %q", before, after)
	}
}

// The mechanism that makes it a trial rather than a measurement: the artifact
// is read from one place and the world is measured in another, so a candidate
// that is in no corpus can still be run against the tree it would govern.
func TestATrialMeasuresATreeTheCandidateIsNotIn(t *testing.T) {
	elsewhere := &workspace{t: t, root: t.TempDir()}
	elsewhere.put(".kept/config.yaml", configFile)
	elsewhere.put("docs/_schema/promise.yaml", shapeSchema)
	elsewhere.put("docs/promise/outsider.md", candidate("outsider", "all(obs.subject.words, x => x == 'here')"))

	tree := &workspace{t: t, root: t.TempDir()}
	tree.put(".kept/config.yaml", configFile)
	tree.put("docs/_schema/promise.yaml", shapeSchema)
	tree.put("subject.txt", "here here")

	// The corpus holding the candidate has no subject.txt at all, so a trial
	// that measured its own root would read the empty set and say `kept`. The
	// verdict below is only reachable by measuring the other tree.
	code, stdout, stderr := tree.run("shape", "promise:outsider", "--trial",
		"--corpus", filepath.Join(elsewhere.root, "docs"),
		"--against", tree.root, "--quiet")
	if code != exitOK {
		t.Fatalf("shape exited %d: %s", code, stderr)
	}
	mustContain(t, "a trial against another tree", stdout, "kept", "here", tree.root)

	if _, err := os.Stat(filepath.Join(tree.root, "docs", "promise", "outsider.md")); !os.IsNotExist(err) {
		t.Error("the candidate reached the measured tree, so this proved nothing")
	}
}

// The two ways a trial settles nothing, which exit alike and read differently.
//
// A gatherer that could not run is a verdict of `unknown`; a promise whose
// predicate was never applied reached no verdict at all. Both are 3 — no
// evidence either way — and neither may be reported as `kept`, which is the
// answer a reader would otherwise take to the ratification.
func TestATrialThatSettlesNothingSaysWhichNothingItIs(t *testing.T) {
	for _, c := range []struct {
		name    string
		promise string
		want    string
	}{
		{
			name: "an instrument that could not run",
			promise: strings.Replace(candidate("settles-nothing", "count(obs.subject.words, x => true) > 0"),
				"run: cat subject.txt", "run: no-such-program-anywhere subject.txt", 1),
			want: "unknown",
		},
		{
			name: "a predicate nothing applied",
			promise: `---
id: settles-nothing
type: promise
title: settles-nothing
fields:
  statement: a candidate whose instrument nothing supplies
  gather:
    subject: { gatherer: no-such-gatherer }
  hold_when: obs.subject.anything
  trigger: cadence
---

Prose.
`,
			want: "unevaluated",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			w := shapeWorkspace(t, "settles-nothing", "count(obs.subject.words, x => true) > 0", "a word")
			w.put("docs/promise/settles-nothing.md", c.promise)

			code, stdout, stderr := w.run("shape", "promise:settles-nothing", "--trial", "--quiet")
			if code != exitUnknown {
				t.Fatalf("%s exited %d, want %d: %s", c.name, code, exitUnknown, stderr)
			}
			mustContain(t, c.name, stdout, c.want)
			if strings.Contains(stdout, "kept ") {
				t.Errorf("a trial that settled nothing reported kept:\n%s", stdout)
			}
		})
	}
}

// The reading is named rather than defaulted, so a second one can be added
// without changing what the first already did.
func TestShapeRefusesWithoutAReading(t *testing.T) {
	w := shapeWorkspace(t, "needs-a-reading", "count(obs.subject.words, x => true) > 0", "a word")

	code, _, stderr := w.run("shape", "promise:needs-a-reading")
	if code != exitUsage {
		t.Fatalf("shape with no reading exited %d, want %d", code, exitUsage)
	}
	mustContain(t, "the refusal", stderr, "--trial")
}

// A trial says whether anybody is committed to what it just measured, because
// the verb's whole occasion is the moment before somebody is.
func TestATrialSaysWhetherAnythingIsHeldToIt(t *testing.T) {
	w := shapeWorkspace(t, "held", "count(obs.subject.words, x => true) > 0", "a word")

	_, stdout, _ := w.run("shape", "promise:held", "--trial", "--quiet")
	mustContain(t, "an unratified trial", stdout, "nothing is held to this yet")

	if code, _, stderr := w.run("ratify", "promise:held", "--by", "the operator"); code != exitOK {
		t.Fatalf("ratify exited %d: %s", code, stderr)
	}
	_, stdout, _ = w.run("shape", "promise:held", "--trial", "--quiet")
	mustContain(t, "a ratified trial", stdout, "this is ratified")
}

// ledgerState is everything the ledger holds, as bytes. A trial that recorded
// anything at all moves it.
func (w *workspace) ledgerState(t *testing.T) string {
	t.Helper()
	var out strings.Builder
	root := filepath.Join(w.root, ".kept")
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		out.WriteString(rel + "\n" + string(body) + "\n")
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return out.String()
}
