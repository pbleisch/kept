package progress

import (
	"bytes"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// clock is a hand-wound time source, so that every assertion about a display is
// about what it renders rather than about how fast the machine running the test
// happened to be.
type clock struct {
	mu sync.Mutex
	at time.Time
}

func newClock() *clock {
	return &clock{at: time.Date(2026, 8, 24, 22, 41, 3, 0, time.UTC)}
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.at
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.at = c.at.Add(d)
}

// A destination that is not a terminal gets lines a log reader can consume: one
// event to a line, an instant that sorts, and not one control character.
//
// This is the acceptance condition stated as a test. A display that wrote a
// carriage return into a CI log would leave a file whose last line is whatever
// happened to be longest, and a display that wrote an escape would leave one
// nobody can grep.
func TestAPlainDestinationGetsLinesAndNoControlCharacters(t *testing.T) {
	var out bytes.Buffer
	c := newClock()
	d := to(&out, false, c.now)

	done := d.Begin(Step{Kind: Gather, Subject: "promise:core-packages-are-tested", Detail: "go/tests", Bound: 10 * time.Minute})
	c.advance(24861 * time.Millisecond)
	done()
	d.Close()

	got := out.String()
	for _, want := range []string{
		"2026-08-24T22:41:03Z kept gather started promise:core-packages-are-tested · go/tests · bounded at 10m",
		"2026-08-24T22:41:27Z kept gather finished promise:core-packages-are-tested · go/tests · 24.861s",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the plain form does not contain %q:\n%s", want, got)
		}
	}

	if n := strings.Count(got, "\n"); n != 2 {
		t.Errorf("one step wrote %d lines, want 2:\n%q", n, got)
	}
	assertNoControl(t, got)
}

// The finish line is the measurement, so it keeps the resolution a measurement
// needs. Rounding a gather that took eight milliseconds and one that took two
// hundred to the same "0s" would make a log useless for the thing this display
// was designed from.
func TestThePlainFormKeepsMillisecondsOnShortSteps(t *testing.T) {
	var out bytes.Buffer
	c := newClock()
	d := to(&out, false, c.now)

	done := d.Begin(Step{Kind: Gather, Subject: "promise:every-citation-in-the-corpus-resolves", Detail: "corpus"})
	c.advance(8 * time.Millisecond)
	done()
	d.Close()

	if !strings.Contains(out.String(), "· 8ms") {
		t.Errorf("a short step did not record its milliseconds:\n%s", out.String())
	}
	// No bound was stated, so none is claimed. A display that invented one
	// would be reporting a deadline nothing will fire.
	if strings.Contains(out.String(), "bounded at") {
		t.Errorf("a step with no bound claimed one:\n%s", out.String())
	}
}

// Every field before the step's own words is fixed, so a log reader can filter
// on them without reading prose. Four fields: the instant, the word kept, the
// kind, and whether this is a start or a finish.
func TestThePlainFormsFirstFourFieldsAreFixed(t *testing.T) {
	var out bytes.Buffer
	c := newClock()
	d := to(&out, false, c.now)
	d.Begin(Step{Kind: Repair, Detail: "cut the box"})()
	d.Close()

	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			t.Fatalf("a line has %d fields, want at least 5: %q", len(fields), line)
		}
		if _, err := time.Parse(time.RFC3339, fields[0]); err != nil {
			t.Errorf("field 1 of %q is not an RFC3339 instant: %v", line, err)
		}
		if fields[1] != "kept" {
			t.Errorf("field 2 of %q is %q, want \"kept\"", line, fields[1])
		}
		if fields[2] != string(Repair) {
			t.Errorf("field 3 of %q is %q, want %q", line, fields[2], Repair)
		}
		if fields[3] != "started" && fields[3] != "finished" {
			t.Errorf("field 4 of %q is %q, want started or finished", line, fields[3])
		}
	}
}

// The live line says what is running and how long it has been running against
// the bound that will fire, which is the whole of what the terminal was missing.
func TestTheLiveLineNamesTheInstrumentAndTheClockAgainstItsBound(t *testing.T) {
	var out bytes.Buffer
	c := newClock()
	d := to(&out, true, c.now)
	defer d.Close()

	d.Begin(Step{Kind: Gather, Subject: "promise:the-tree-vets-clean", Detail: "command", Bound: 10 * time.Minute})
	c.advance(24 * time.Second)

	d.mu.Lock()
	line := d.render(c.now())
	d.mu.Unlock()
	if want := "  promise:the-tree-vets-clean · command · 24s of 10m"; line != want {
		t.Errorf("the live line is %q, want %q", line, want)
	}
}

// A step that finishes before anyone could have read it is never drawn. A warm
// tick's five fast gathers would otherwise be five frames of flicker.
func TestTheLiveFormDrawsNothingUntilAStepHasRunLongEnoughToWatch(t *testing.T) {
	var out bytes.Buffer
	c := newClock()
	d := to(&out, true, c.now)
	defer d.Close()

	d.Begin(Step{Kind: Gather, Subject: "promise:gatherers-do-not-write", Detail: "go/symbols"})
	c.advance(settle - time.Millisecond)
	d.mu.Lock()
	early := d.render(c.now())
	d.mu.Unlock()
	if early != "" {
		t.Errorf("a step under the settling time rendered %q, want nothing", early)
	}

	c.advance(2 * time.Millisecond)
	d.mu.Lock()
	late := d.render(c.now())
	d.mu.Unlock()
	if late == "" {
		t.Error("a step past the settling time rendered nothing")
	}
}

// Steps nest, and the innermost is what a person is waiting for: a repair's
// re-derivation encloses the instrument that is actually running.
func TestTheLiveLineShowsTheInnermostStep(t *testing.T) {
	var out bytes.Buffer
	c := newClock()
	d := to(&out, true, c.now)
	defer d.Close()

	outer := d.Begin(Step{Kind: Repair, Detail: "measure the promise in the box"})
	c.advance(time.Second)
	inner := d.Begin(Step{Kind: Gather, Subject: "promise:core-packages-are-tested", Detail: "go/tests"})
	c.advance(27 * time.Second)

	d.mu.Lock()
	line := d.render(c.now())
	d.mu.Unlock()
	if !strings.Contains(line, "go/tests") {
		t.Errorf("the live line shows the outer step, want the inner one: %q", line)
	}

	inner()
	d.mu.Lock()
	line = d.render(c.now())
	d.mu.Unlock()
	if !strings.Contains(line, "measure the promise in the box") {
		t.Errorf("the live line did not fall back to the enclosing step: %q", line)
	}
	outer()
}

// A step may finish out of order without corrupting the stack. Nothing in kept
// does it today, and a stack that assumed otherwise would go wrong quietly.
func TestAStepMayFinishBeforeTheOneItEncloses(t *testing.T) {
	var out bytes.Buffer
	c := newClock()
	d := to(&out, true, c.now)
	defer d.Close()

	outer := d.Begin(Step{Kind: Repair, Detail: "the session is running"})
	inner := d.Begin(Step{Kind: Gather, Subject: "promise:one", Detail: "command"})
	outer()
	c.advance(time.Second)

	d.mu.Lock()
	line := d.render(c.now())
	held := len(d.stack)
	d.mu.Unlock()
	if held != 1 {
		t.Fatalf("%d steps are held after one of two finished, want 1", held)
	}
	if !strings.Contains(line, "promise:one") {
		t.Errorf("the surviving step is not the one still running: %q", line)
	}
	inner()
	inner()
}

// The live line never grows past the width it assumes, and what it gives up is
// the artifact's tail rather than the clock — the clock is the part that moves,
// and a reader who can see only some of this needs that part.
func TestTheLiveLineFitsItsWidthAndKeepsTheClock(t *testing.T) {
	var out bytes.Buffer
	c := newClock()
	d := to(&out, true, c.now)
	defer d.Close()

	long := "promise:" + strings.Repeat("a-very-long-slug-", 8)
	d.Begin(Step{Kind: Gather, Subject: long, Detail: "go/tests", Bound: 10 * time.Minute})
	c.advance(24 * time.Second)

	d.mu.Lock()
	line := d.render(c.now())
	d.mu.Unlock()
	if runes(line) > columns {
		t.Errorf("the live line is %d columns wide, want at most %d: %q", runes(line), columns, line)
	}
	if !strings.HasSuffix(line, "24s of 10m") {
		t.Errorf("the live line dropped the clock rather than the words: %q", line)
	}
}

// Quiet is quiet. It is the one flag this display has, and it exists because
// problems and progress share stderr: an operator cannot get "problems only" by
// redirecting the stream, because the refusals and the quarantine warnings are
// on it too.
func TestQuietShowsNothing(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if d := For(&stdout, &stderr, true); d != nil {
		t.Fatal("--quiet still built a display")
	}
	if stderr.Len() != 0 {
		t.Errorf("a quiet run wrote to stderr: %q", stderr.String())
	}
}

// A nil display is a run nobody is watching, and every method is safe on one.
// Callers hold the pointer and never guard it, so the guard is here.
func TestANilDisplayIsSafe(t *testing.T) {
	var d *Display
	done := d.Begin(Step{Kind: Gather, Subject: "promise:one", Detail: "command"})
	done()
	done()
	d.Close()
	d.Close()
}

// A buffer is not a terminal and a pipe is not a terminal, so neither gets the
// live form. This is the test behind the acceptance condition: it breaks the
// property by handing For a real *os.File that is a pipe.
func TestAPipeIsNotATerminalSoTheFormDegrades(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()

	if terminal(w) {
		t.Error("a pipe reported itself a terminal")
	}
	var buf bytes.Buffer
	if terminal(&buf) {
		t.Error("a buffer reported itself a terminal")
	}

	d := For(w, w, false)
	if d == nil {
		t.Fatal("a pipe got no display at all; the degraded form is the point")
	}
	if d.live {
		t.Error("a pipe got the live form")
	}
	d.Close()
}

// Everything a display writes when it is not on a terminal, in one place: no
// escape, no carriage return, no bell, and no other C0 control but the newline
// that ends a line.
func assertNoControl(t *testing.T, got string) {
	t.Helper()
	for i, r := range got {
		if r == '\n' {
			continue
		}
		if r < 0x20 || r == 0x7f {
			t.Fatalf("byte %d of the plain form is control character %#U:\n%q", i, r, got)
		}
	}
}
