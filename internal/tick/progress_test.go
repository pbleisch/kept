package tick

import (
	"bytes"
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/progress"
)

// A pass says which instrument is measuring which artifact, and the display
// changes nothing about what the pass found.
//
// The second half is the whole of what a display is allowed to be: presentation
// (docs/decision/0065). The verdict, its detail and the truth record are read
// back from a watched pass and an unwatched one and have to agree.
func TestAPassSaysWhichInstrumentIsMeasuringWhichArtifact(t *testing.T) {
	b := newBox(t)
	b.install()
	b.write("src/marker.txt", "ok\n")
	b.promise("the-marker-says-ok", "src/**", runLine(t, b.tool, "read", "src/marker.txt"), "")
	b.ratify("the-marker-says-ok")

	var out bytes.Buffer
	// A buffer is not a terminal, so this is the form a CI job and a captured
	// pane get — and the only form a test can read.
	show := progress.For(&out, &out, false)
	watched := b.tick(func(o *Options) { o.Progress = show })
	show.Close()

	said := out.String()
	for _, want := range []string{
		"kept gather started promise:the-marker-says-ok · command",
		"kept gather finished promise:the-marker-says-ok · command",
	} {
		if !strings.Contains(said, want) {
			t.Errorf("the pass never said %q:\n%s", want, said)
		}
	}

	seen := watched.subject(t, "promise:the-marker-says-ok")
	if seen.Answer.Value != model.Kept {
		t.Fatalf("the watched pass reads %q: %s", seen.Answer.Value, seen.Answer.Detail)
	}
	records := b.truthLines("promise:the-marker-says-ok")
	if len(records) != 1 || records[0].Value != model.Kept {
		t.Fatalf("the watched pass wrote %d truth records", len(records))
	}

	// The same tree measured again with nothing watching. Nothing moved, so the
	// verdict is carried and no second record is appended — which is what a
	// display must not be able to change.
	silent := b.tick()
	if quiet := silent.subject(t, "promise:the-marker-says-ok"); quiet.Answer.Value != seen.Answer.Value {
		t.Errorf("an unwatched pass reads %q where a watched one read %q", quiet.Answer.Value, seen.Answer.Value)
	}
	if after := b.truthLines("promise:the-marker-says-ok"); len(after) != len(records) {
		t.Errorf("the ledger holds %d truth records after an unwatched pass and %d after the watched one",
			len(after), len(records))
	}
}

// A spec nothing can run as written is refused inside the runner, so no program
// exists — and the display still reports the step, because the step is kept
// asking for a measurement rather than kept running a program.
//
// It is pinned rather than left to chance. The refusal and the display arrived
// separately, and the two readings available afterwards are "go/tests ran" and
// "kept asked and was refused in microseconds". The second is the true one, and
// the way to keep it true is to leave the question of whether a spec is runnable
// in the one place that owns it — Runner.Gather — rather than asking it again
// here to decide whether to draw. See docs/decision/0065.
func TestASpecTheRunnerRefusesIsStillReportedAndClaimsNoProgram(t *testing.T) {
	b := newBox(t)
	b.write("internal/thing/thing.go", "package thing\n")
	b.write("docs/promise/tests-pass.md", `---
id: tests-pass
type: promise
title: tests-pass
fields:
  statement: every package under internal/ passes
  gather:
    tests:
      gatherer: go/tests
      scope: "internal/**"
      packages: "./internal/..."
      bogusarg: yes
  hold_when: obs.tests.ok
  trigger: cadence
---

Why: because a promise nobody can read as written is not one.
`)
	b.ratify("tests-pass")

	var out bytes.Buffer
	show := progress.For(&out, &out, false)
	res := b.tick(func(o *Options) { o.Progress = show })
	show.Close()

	said := out.String()
	if !strings.Contains(said, "kept gather started promise:tests-pass · go/tests") {
		t.Errorf("the refused spec was not reported at all:\n%s", said)
	}
	if !strings.Contains(said, "kept gather finished promise:tests-pass · go/tests") {
		t.Errorf("the refused spec was begun and never finished:\n%s", said)
	}

	// It says how long kept waited and nothing more. No verdict, no reason, no
	// claim that the instrument ran: the tick's own answer carries all of that,
	// and a display that repeated it would be a second place it could be wrong.
	if strings.Contains(said, "bogusarg") || strings.Contains(said, "unknown") {
		t.Errorf("the display reported the verdict as well as the wait:\n%s", said)
	}

	// And the refusal is untouched by being watched: still unknown, still
	// naming the arg, still no observation behind it.
	s := res.subject(t, "promise:tests-pass")
	if s.Answer.Value != model.Unknown || !strings.Contains(s.Answer.Detail, "bogusarg") {
		t.Fatalf("a watched pass reads %q because %q", s.Answer.Value, s.Answer.Detail)
	}
	records := b.truthLines("promise:tests-pass")
	if len(records) != 1 || len(records[0].From) != 0 {
		t.Fatalf("a gather that never ran left %d records naming observations", len(records))
	}
}

// A pass with no display runs, and nothing on the way guards the nil. The guard
// is in the display; this is the test that the callers really do rely on it.
func TestAPassWithNoDisplayRuns(t *testing.T) {
	b := newBox(t)
	b.install()
	b.write("src/marker.txt", "ok\n")
	b.promise("the-marker-says-ok", "src/**", runLine(t, b.tool, "read", "src/marker.txt"), "")
	b.ratify("the-marker-says-ok")

	res := b.tick(func(o *Options) { o.Progress = nil })
	if s := res.subject(t, "promise:the-marker-says-ok"); s.Answer.Value != model.Kept {
		t.Fatalf("an unwatched pass reads %q: %s", s.Answer.Value, s.Answer.Detail)
	}
}
