// Package progress says what kept is doing while it is doing it.
//
// A tick is silent for as long as its instruments take, and they take as long
// as other people's programs take. Measured over this repository: a forced tick
// with the test cache cleared is 56.4 seconds, of which 53.4 are inside two `go
// test` subprocesses — 94% of the wall clock in two calls — and the walk, the
// act stream, the predicate and the truth append are 55 milliseconds together.
// The same tick against a warm build cache is 3.5 seconds. Nothing on the
// terminal told a person which of those two runs they were in, or whether they
// were in neither.
//
// So exactly one thing earns a line: an invocation kept started and does not
// control. A display that also announced the walk, the fold and the evaluation
// would spread itself evenly over phases that are not evenly expensive, and the
// reader would learn least about the part that takes all the time.
//
// For the same reason nothing here counts artifacts down. A tick's remaining
// time is decided by which instruments are left rather than how many, and over
// this corpus those differ by three orders of magnitude — 8 milliseconds for
// `corpus` against 28 seconds for `go/tests`. "7 of 8" would mean either 4% or
// 96% of the wall clock, and a fraction that can be wrong by that factor is
// worse than no fraction.
//
// It is presentation and never record. Nothing here reaches the ledger, an
// observation, a verdict or a disposition; a display that failed outright would
// change no answer kept gives and no byte of what `--json` puts on stdout. See
// docs/decision/0065.
package progress

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Kind is what sort of thing a step is. There is one per verb slow enough to
// need one: a tick, which spends its time in instruments; a repair, which
// spends its time in a box and a session; and a land, which spends its time in
// a gate and on the network.
//
// The count was two and is three, which is the whole of what changed: the test
// is still whether a verb is slow enough, and a land gathers the corpus twice
// and talks to a remote.
type Kind string

const (
	// Gather is one instrument measuring one artifact.
	Gather Kind = "gather"
	// Repair is one phase of one repair.
	Repair Kind = "repair"
	// Land is one phase of one land.
	Land Kind = "land"
)

// Step is one thing kept started and does not control.
type Step struct {
	Kind Kind
	// Subject is the artifact this is about, and is empty for a step that is
	// about none — cutting a box is not about an artifact.
	Subject string
	// Detail is the instrument that is running, or the phase's own words.
	Detail string
	// Bound is the deadline that will actually fire, and zero is "none stated
	// here". It is shown because a bound nobody can see is not a bound anybody
	// is reassured by: `10m` is meaningless to a reader who cannot tell whether
	// they are twelve seconds into it or nine minutes. See docs/decision/0026.
	Bound time.Duration
}

// words is the step as both forms say it, less the clock.
func (s Step) words() string {
	switch {
	case s.Subject == "":
		return s.Detail
	case s.Detail == "":
		return s.Subject
	}
	return s.Subject + " · " + s.Detail
}

// paintEvery is how often the live form redraws. It writes only when the
// rendered text has changed, so an idle second costs nothing and a step that
// starts or ends is on screen within a quarter of a second.
const paintEvery = 250 * time.Millisecond

// settle is how long a step runs before the live form draws it at all.
//
// A warm tick finishes its seven gathers in 3.5 seconds and five of them are
// under 20 milliseconds; drawing those would be five frames of flicker
// reporting nothing a person could read. The display appears when there is
// something to wait for, which is the whole of what it is for, and a second is
// also the shortest interval this line can honestly report — its first frame
// says "1s" rather than "0s", which is what a shorter threshold made it say.
//
// The other form does not wait. A log's start line earns its place precisely by
// existing before the finish line does — that is what tells a reader watching a
// running job that the gap is a program working rather than a program gone —
// and a threshold would delete it for exactly the step that was killed early.
const settle = time.Second

// columns is how wide the live form assumes the terminal is.
//
// kept asks no terminal for its width. There is nothing in the standard library
// that does, `COLUMNS` is not exported by the shells that set it, and a module
// for it would have to pass docs/decision/0029's test to answer a question this
// cosmetic. A terminal narrower than this wraps the line, and the erase then
// clears only its last row — which is the cost of not taking the dependency,
// paid in a display that is itself cosmetic.
const columns = 80

// Display is where a run says what it is doing.
//
// A nil *Display is a run nobody is watching: every method is safe on one, so a
// caller holds the pointer and never asks whether it has one. That is
// deliberate — a progress display that callers had to guard would grow a guard
// somebody forgot, and the thing they forgot to guard is on the path that
// records a verdict.
type Display struct {
	w    io.Writer
	live bool
	now  func() time.Time

	mu      sync.Mutex
	stack   []*running
	painted string
	closed  bool

	stop chan struct{}
	done chan struct{}
}

// running is one begun step and when it began.
type running struct {
	step  Step
	began time.Time
}

// For returns the display a run shows, or nil when it shows none.
//
// The live form needs both streams to be a terminal, and that is stricter than
// "the stream I write to is a terminal" on purpose: the two are one screen, and
// a run whose result is being captured to a file is a run somebody reads later,
// where a timestamp beats an animation. Everything else — a pipe, a file, a CI
// job, an agent's captured output — gets the plain form, which emits no cursor
// control, no carriage return and no escape of any kind.
func For(stdout, stderr io.Writer, quiet bool) *Display {
	if quiet {
		return nil
	}
	return to(stderr, terminal(stdout) && terminal(stderr), time.Now)
}

// to is the constructor the tests use, because a test cannot hold a terminal
// and both forms have to be exercised.
func to(w io.Writer, live bool, now func() time.Time) *Display {
	d := &Display{w: w, live: live, now: now}
	if live {
		d.stop, d.done = make(chan struct{}), make(chan struct{})
		go d.paint()
	}
	return d
}

// terminal reports whether w is a character device.
//
// A *os.File is the only thing that can be one, and stat is the whole test: no
// ioctl, no termcap, and nothing that behaves differently on one Unix than
// another. Anything else — a buffer, a pipe, a file — is not.
func terminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// Begin reports that kept has started something it does not control, and
// returns the function that reports it finished.
//
// The returned function is safe to call more than once and safe to call after
// [Display.Close]; the second call reports nothing. Steps nest — a repair's
// phase encloses the gather its re-derivation runs — and the live form shows
// the innermost, because the most specific thing running is the thing a person
// is waiting for.
func (d *Display) Begin(step Step) func() {
	if d == nil {
		return func() {}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return func() {}
	}

	at := d.now()
	r := &running{step: step, began: at}
	d.stack = append(d.stack, r)
	if !d.live {
		d.write(at, step.Kind, "started", step.words()+bound(step.Bound))
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			d.mu.Lock()
			defer d.mu.Unlock()
			d.pop(r)
			if d.closed {
				return
			}
			done := d.now()
			if !d.live {
				d.write(done, step.Kind, "finished", step.words()+" · "+took(done.Sub(r.began)))
				return
			}
			// The line the live form has been drawing is about a step that is
			// over. Clear it now rather than at the next paint, so that a step
			// finishing right before the result is printed does not leave its
			// own text under the first line of the answer.
			d.clear()
		})
	}
}

// Aside writes something that is not progress while the display is running,
// without the two landing on one row.
//
// The live form owns a row of the terminal, and a line written to the other
// stream while it is drawn lands on top of it. So the live row is cleared first
// and nothing is painted until write has returned; the next paint draws it
// again. The plain form draws nothing to collide with, and a nil display is a
// run nobody is watching — both just write. What write puts on its own stream is
// the same bytes either way, which is what keeps an answer on stdout from asking
// whether stdout is a terminal. See docs/decision/0065 and 0086.
func (d *Display) Aside(write func()) {
	if d == nil {
		write()
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.clear()
	write()
}

// Close stops the display and leaves the stream as it found it. Every verb that
// opens one closes it before it prints an answer: the live form owns a row of
// the terminal, and an answer written under it would land on top of a line
// still being redrawn.
func (d *Display) Close() {
	if d == nil {
		return
	}
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return
	}
	d.closed = true
	d.clear()
	stop, done := d.stop, d.done
	d.mu.Unlock()

	if stop != nil {
		close(stop)
		<-done
	}
}

// pop removes one step wherever it is in the stack. By identity rather than by
// position, because a caller may finish an enclosing step before an inner one —
// nothing forbids it, and a stack that assumed otherwise would corrupt itself
// quietly rather than say so.
func (d *Display) pop(r *running) {
	for i, held := range d.stack {
		if held == r {
			d.stack = append(d.stack[:i], d.stack[i+1:]...)
			return
		}
	}
}

// write puts one line of the plain form on the stream.
//
// The clock is joined with the same separator the live line uses rather than
// with a preposition: a phase's own words end in one often enough that "in the
// box in 24.679s" was the first thing this printed.
//
// The first four fields are fixed — an RFC3339 instant in UTC, the word kept,
// the kind, and whether this is a start or a finish — and everything after them
// is the step's own words. A reader filters on the four and reads the rest; a
// person reads all of it. UTC and RFC3339 because the instant has to be one
// field and has to sort, and kept's own human stamp is neither: it carries a
// space and it is local.
func (d *Display) write(at time.Time, kind Kind, state, words string) {
	fmt.Fprintf(d.w, "%s kept %s %s %s\n", at.UTC().Format(time.RFC3339), kind, state, words)
}

// paint redraws the live line until the display is closed.
func (d *Display) paint() {
	defer close(d.done)
	tick := time.NewTicker(paintEvery)
	defer tick.Stop()
	for {
		select {
		case <-d.stop:
			return
		case <-tick.C:
			d.mu.Lock()
			d.draw(d.render(d.now()))
			d.mu.Unlock()
		}
	}
}

// render is what the live line should say at this instant, or "" when it should
// say nothing. It is separate from drawing so that the shape of the line is
// decided by a function a test can call with a clock it chose.
func (d *Display) render(now time.Time) string {
	if len(d.stack) == 0 {
		return ""
	}
	r := d.stack[len(d.stack)-1]
	since := now.Sub(r.began)
	if since < settle {
		return ""
	}

	clock := elapsed(since)
	if r.step.Bound > 0 {
		clock += " of " + duration(r.step.Bound)
	}
	return "  " + fit(r.step.words(), clock)
}

// draw puts the rendered line on the terminal, and only when it has changed.
func (d *Display) draw(line string) {
	if d.closed || line == d.painted {
		return
	}
	if line == "" {
		d.clear()
		return
	}
	// Carriage return to the start of the row, the line, then erase to the end
	// of it — so a line that shrank does not leave the tail of the one before.
	fmt.Fprintf(d.w, "\r%s\033[K", line)
	d.painted = line
}

// clear takes the live line off the terminal. It is a no-op when nothing is
// drawn, which is what keeps the plain form free of every escape in this file.
func (d *Display) clear() {
	if d.painted == "" {
		return
	}
	fmt.Fprint(d.w, "\r\033[K")
	d.painted = ""
}

// fit joins the step's words to the clock inside the terminal's width, and
// shortens the words rather than the clock.
//
// The clock is the reason the line exists: a reader who can see only part of
// this needs the part that is moving. An artifact id is long, it is the same on
// every frame, and its tail is the part a reader has already read.
func fit(words, clock string) string {
	line := words + " · " + clock
	width := columns - 2
	if runes(line) <= width || runes(clock)+4 >= width {
		return line
	}
	return truncate(words, width-runes(clock)-4) + "… · " + clock
}

// runes is a string's width in characters rather than in bytes. The separator
// this file joins with is three bytes and one column.
func runes(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

// truncate cuts to n runes, never mid-rune.
func truncate(s string, n int) string {
	var out strings.Builder
	for i, r := range []rune(s) {
		if i >= n {
			break
		}
		out.WriteRune(r)
	}
	return out.String()
}

// bound is what the plain form says about a deadline, and says nothing when
// there is none.
func bound(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	return " · bounded at " + duration(d)
}

// elapsed is how long something has been running, as the live line shows it.
// Whole seconds under a minute, because that is the resolution a person waiting
// reads at and a digit that changed ten times a second would be motion rather
// than information; minutes and seconds over one, because "312s" is a number a
// reader has to divide.
func elapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Minute {
		return strconv.Itoa(int(d.Seconds())) + "s"
	}
	return duration(d.Round(time.Second))
}

// took is how long something ran, as the plain form records it.
//
// It keeps milliseconds where the live line does not, because these lines are
// the measurement: this display was designed from a table of them, and a form
// that rounded 8ms and 204ms both to "0s" would make the next such table
// impossible to read off a log.
func took(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Millisecond:
		// Rounding to milliseconds would print "0s" for a gather that did run,
		// which is the one thing a measurement line must not say.
		return d.Round(time.Microsecond).String()
	case d < time.Minute:
		return d.Round(time.Millisecond).String()
	}
	return duration(d.Round(time.Second))
}

// duration is a deadline as it was configured: 10m rather than 10m0s.
func duration(d time.Duration) string {
	out := d.String()
	for _, zero := range []string{"h0m0s", "m0s"} {
		if strings.HasSuffix(out, zero) {
			return out[:len(out)-len(zero)+1]
		}
	}
	return out
}
