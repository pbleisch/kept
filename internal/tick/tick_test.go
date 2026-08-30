package tick

import (
	"strings"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/model"
)

const marker = "promise:the-marker-says-ok"

// A real promise goes unknown → not-kept → kept as the world changes, and the
// tick is right about each because it just looked.
//
// This is increment 3's own done-condition, written against a promise that
// names a program rather than a fixture, so that what is under test is the
// whole path: walk, scope digest, gather, extract, evaluate, record.
func TestAPromiseMovesWithTheWorld(t *testing.T) {
	b := newBox(t)
	b.write("src/marker.txt", "nothing yet\n")
	b.promise("the-marker-says-ok", "src/**", runLine(t, b.tool, "read", "src/marker.txt"), "")
	b.ratify("the-marker-says-ok")

	// The program the promise names is not installed, so nobody measured.
	// Never not-kept: a machine that could not measure does not record what it
	// thinks it saw.
	if got := b.tick().subject(t, marker); got.Answer.Value != model.Unknown {
		t.Fatalf("a program that could not be run produced %q, and the only honest answer is unknown", got.Answer.Value)
	}

	// The instrument arrives. Installing one is not a change to the world any
	// promise looks at, which is what --all is for.
	b.install()
	if got := b.tick(forced).subject(t, marker); got.Answer.Value != model.NotKept {
		t.Fatalf("the marker says %q and the verdict is %q: %s", "nothing yet", got.Answer.Value, got.Answer.Detail)
	}

	// The world changes, and the promise holds. Nothing forced this one: the
	// bytes in its scope moved, which is the whole of what due means.
	b.write("src/marker.txt", "ok\n")
	kept := b.tick().subject(t, marker)
	if kept.Answer.Value != model.Kept {
		t.Fatalf("the marker says ok and the verdict is %q: %s", kept.Answer.Value, kept.Answer.Detail)
	}
	if kept.Drifting {
		t.Errorf("a kept promise is drift: %v", kept.Drift)
	}

	// One wording throughout, so this is one series and not three.
	values := valuesOf(b.truthLines(marker))
	want := []model.Value{model.Unknown, model.NotKept, model.Kept}
	if len(values) != len(want) {
		t.Fatalf("the truth stream is %v, and the sequence was %v", values, want)
	}
	for i := range want {
		if values[i] != want[i] {
			t.Fatalf("the truth stream is %v, want %v", values, want)
		}
	}
}

// forced is --all: gather every artifact whatever its scope says.
func forced(o *Options) { o.All = true }

// A missing toolchain yields unknown rather than not-kept, and says which field
// nobody could answer for.
//
// The guard is the second assertion. A verdict of not-kept here is the one lie
// the tool must not tell: it reports a broken promise where there is only a
// machine that could not measure, and CI is where that costs something.
func TestAMissingToolchainIsUnknownAndNeverNotKept(t *testing.T) {
	b := newBox(t)
	b.write("src/marker.txt", "ok\n")
	b.promise("the-marker-says-ok", "src/**", runLine(t, b.tool, "read", "src/marker.txt"), "")
	b.ratify("the-marker-says-ok")

	s := b.tick().subject(t, marker)
	if s.Answer.Value != model.Unknown {
		t.Fatalf("a toolchain that is not installed produced %q", s.Answer.Value)
	}
	if s.Answer.Value == model.NotKept {
		t.Fatal("a machine that could not measure reported the promise broken")
	}
	// The detail is the gatherer's own account of why it could not measure,
	// not the predicate's account of a field it could not read. An operator
	// who cannot tell a missing toolchain from a timeout cannot fix either.
	if !strings.Contains(s.Answer.Detail, "marker") || !strings.Contains(s.Answer.Detail, "could not be run") {
		t.Errorf("the unknown does not name what could not measure: %q", s.Answer.Detail)
	}
	if s.Drift.Kind != model.DriftUnknown {
		t.Errorf("drift is %q, and a gatherer that could not run is unknown", s.Drift.Kind)
	}

	// And nothing was filed for it. A failed gather leaves no observation at
	// all, which is what makes the verdict unknown rather than not-kept.
	if n := len(b.truthLines(marker)[0].From); n != 0 {
		t.Errorf("a failed gather pinned %d observations", n)
	}
}

// A promise is due when the bytes in its scope move, and not otherwise. An edit
// somewhere else changes the world and must not cost a gather.
func TestDueIsAboutTheScopeAndNotTheWorld(t *testing.T) {
	b := newBox(t)
	b.install()
	b.write("src/marker.txt", "ok\n")
	b.promise("the-marker-says-ok", "src/**", runLine(t, b.tool, "read", "src/marker.txt"), "")
	b.ratify("the-marker-says-ok")

	first := b.tick()
	if first.Observed != 1 {
		t.Fatalf("the first tick recorded %d observations", first.Observed)
	}

	// A file the promise does not look at. The world's digest changes; the
	// scope's does not.
	b.write("elsewhere.txt", "a note nobody promised anything about\n")
	quiet := b.tick()
	if quiet.Observed != 0 {
		t.Errorf("an edit outside the scope cost %d gathers", quiet.Observed)
	}
	s := quiet.subject(t, marker)
	if s.Due {
		t.Errorf("an edit outside the scope made the promise due: %s", s.Because)
	}
	// The answer still applies, and it says which tree it was reached in.
	if s.Answer.Value != model.Kept {
		t.Errorf("the carried answer is %q", s.Answer.Value)
	}
	if s.Answer.Fresh {
		t.Error("an answer reached in another tree is reported as fresh")
	}
	if n := len(b.truthLines(marker)); n != 1 {
		t.Errorf("the truth stream holds %d records after an edit nothing promised anything about", n)
	}

	// A file it does look at.
	b.write("src/marker.txt", "ok\nand a second line nobody promised anything about\n")
	moved := b.tick()
	if moved.Observed != 1 {
		t.Errorf("an edit inside the scope cost %d gathers", moved.Observed)
	}
	if got := moved.subject(t, marker); !got.Due || !strings.Contains(got.Because, "marker") {
		t.Errorf("due is %v because %q", got.Due, got.Because)
	}

	// Transitions, not ticks. The promise was due, the gatherer ran, and the
	// verdict is the one already in force — so nothing is appended. A record
	// per tick is what makes a promise kept for a month thousands of lines
	// instead of one.
	if n := len(b.truthLines(marker)); n != 1 {
		t.Errorf("a gather that reached the verdict already in force wrote %d records, want 1", n)
	}
}

// A survey of a promise whose scope has moved reports a stale answer rather
// than no answer at all. The two are different: one was never put, the other
// was answered about a tree this is not, and only a tick re-derives it.
func TestASurveyOfAMovedScopeIsStaleAndNotUnverified(t *testing.T) {
	b := newBox(t)
	b.install()
	b.write("src/marker.txt", "ok\n")
	b.promise("the-marker-says-ok", "src/**", runLine(t, b.tool, "read", "src/marker.txt"), "")
	b.ratify("the-marker-says-ok")
	b.tick()

	b.write("src/marker.txt", "not ok any more\n")
	s := b.survey().subject(t, marker)
	if !s.Due {
		t.Fatal("a moved scope is not due")
	}
	if s.Drift.Kind != model.DriftStale {
		t.Fatalf("a survey of a moved scope reports %q drift", s.Drift.Kind)
	}
	if s.Answer.Value != model.Kept || s.Answer.Fresh {
		t.Errorf("the carried answer is %q, fresh=%v", s.Answer.Value, s.Answer.Fresh)
	}
	if s.Answer.Gathered {
		t.Error("a survey gathered")
	}
}

// Recording truth does not make everything due. The ledger is plain files
// inside the workspace this increment, so without .kept being out of the world
// the tick would feed itself: recording a transition would change the world,
// which would make every promise due, which would record more.
func TestRecordingTruthDoesNotFeedTheTick(t *testing.T) {
	b := newBox(t)
	b.install()
	b.write("src/marker.txt", "ok\n")
	b.promise("the-marker-says-ok", "**", runLine(t, b.tool, "read", "src/marker.txt"), "")
	b.ratify("the-marker-says-ok")

	b.tick()
	for range 3 {
		if again := b.tick(); again.Observed != 0 {
			t.Fatalf("a tick that recorded made itself due again: %d observations", again.Observed)
		}
	}
	if n := len(b.truthLines(marker)); n != 1 {
		t.Fatalf("four ticks over an unchanging world wrote %d truth records", n)
	}
}

// Transitions, not ticks — plus the heartbeat, which is the only thing that
// makes a stale answer visible. There is no separate timer: an observation's
// timestamp means first-seen and never advances for an unchanging world, so
// max_age is where freshness comes from and nowhere else.
func TestTheHeartbeatIsMaxAgeAndNothingElse(t *testing.T) {
	b := newBox(t)
	b.install()
	b.write("src/marker.txt", "ok\n")
	b.promise("the-marker-says-ok", "src/**", runLine(t, b.tool, "read", "src/marker.txt"), "1d")
	b.ratify("the-marker-says-ok")

	b.tick()
	if n := len(b.truthLines(marker)); n != 1 {
		t.Fatalf("the first tick wrote %d records", n)
	}

	// Nothing has moved and the answer is young, so nothing is due and nothing
	// is written.
	b.tick()
	if n := len(b.truthLines(marker)); n != 1 {
		t.Fatalf("a second tick over an unchanging world wrote %d records", n)
	}

	// The answer ages past max_age. It is due for the same reason a moved
	// scope is due, and the unchanged verdict is appended so that its age is
	// visible in the stream.
	b.backdate("truth/promise/the-marker-says-ok.jsonl", 48*time.Hour)
	s := b.tick().subject(t, marker)
	if !s.Due || !strings.Contains(s.Because, "older than") {
		t.Fatalf("an answer past max_age is due=%v because %q", s.Due, s.Because)
	}
	records := b.truthLines(marker)
	if len(records) != 2 {
		t.Fatalf("the heartbeat wrote %d records, want 2", len(records))
	}
	if records[0].Value != records[1].Value {
		t.Fatalf("the heartbeat changed the verdict: %v", valuesOf(records))
	}
}

// A drift the tick declined to act on leaves a record saying why. None of the
// reasons can be reconstructed afterwards, and a queue whose silence cannot be
// explained is not one to trust.
func TestEveryDriftLeavesADispositionSayingWhy(t *testing.T) {
	b := newBox(t)
	b.install()
	b.write("src/marker.txt", "not ok\n")
	b.promise("the-marker-says-ok", "src/**", runLine(t, b.tool, "read", "src/marker.txt"), "")
	b.ratify("the-marker-says-ok")

	s := b.tick().subject(t, marker)
	if !s.Drifting {
		t.Fatal("a ratified promise that is not kept is not drift")
	}
	if !s.Disposed {
		t.Fatal("the tick did nothing about a drift and recorded nothing about it")
	}
	if s.Disposition.Outcome != ledger.OutcomeRefused || s.Disposition.Condition == "" {
		t.Fatalf("the disposition is %q/%q, and a refusal that names no condition is the shrug this stream replaces",
			s.Disposition.Outcome, s.Disposition.Condition)
	}
	if s.Disposition.Detail == "" {
		t.Error("the refusal says nothing a person could act on")
	}
	if s.Disposition.World.Tree == "" {
		t.Error("the disposition names no world, and a record written without one cannot be backfilled")
	}

	// Transitions, not ticks: the same refusal on the same grounds is one
	// record and a heartbeat.
	for range 3 {
		b.tick()
	}
	if n := len(b.dispositionLines(marker)); n != 1 {
		t.Errorf("four ticks refusing on the same grounds wrote %d dispositions", n)
	}

	// And a survey reads it back, which is the whole of why it was written
	// down: `kept why` answers the negative case from the record, not by
	// re-deriving something momentary.
	back := b.survey().subject(t, marker)
	if back.Disposition.Condition != s.Disposition.Condition {
		t.Errorf("a survey read the disposition back as %q", back.Disposition.Condition)
	}
	if back.Disposed {
		t.Error("a survey recorded a disposition")
	}
}

// A promise nobody ratified obliges nobody, so it is never drift — and it is
// still observed, because truth is about content and standing is about
// obligation.
func TestAnUnratifiedPromiseIsObservedAndIsNotDrift(t *testing.T) {
	b := newBox(t)
	b.install()
	b.write("src/marker.txt", "not ok\n")
	b.promise("the-marker-says-ok", "src/**", runLine(t, b.tool, "read", "src/marker.txt"), "")

	s := b.tick().subject(t, marker)
	if s.Answer.Value != model.NotKept {
		t.Errorf("an unratified promise was not observed: %q", s.Answer.Value)
	}
	if s.Drifting {
		t.Error("an unratified promise is drift, and it obliges nobody")
	}
	if n := len(b.dispositionLines(marker)); n != 0 {
		t.Errorf("the tick disposed of an artifact with no standing, %d times", n)
	}
}

// Ratified content that cannot be found keeps its standing and says the content
// is gone. It is never read as a release: an absence cannot be told apart from
// a bad merge or a corpus root pointed at the wrong place.
func TestAbsentContentIsDriftAndNotARetirement(t *testing.T) {
	b := newBox(t)
	b.install()
	b.write("src/marker.txt", "ok\n")
	b.promise("the-marker-says-ok", "src/**", runLine(t, b.tool, "read", "src/marker.txt"), "")
	b.ratify("the-marker-says-ok")
	b.tick()

	b.remove("docs/promise/the-marker-says-ok.md")
	s := b.tick().subject(t, marker)
	if s.Drift.Kind != model.DriftAbsent {
		t.Fatalf("deleted content is %q drift", s.Drift.Kind)
	}
	if !s.Disposed {
		t.Error("absent content with standing left no disposition")
	}
}

// A hold_when that will not parse is a mistake rather than an unknown, and
// recording one would say nobody measured when somebody mistyped. The two
// failures are told apart by type and never by reading a message.
func TestAPredicateThatWillNotParseIsNotAVerdict(t *testing.T) {
	b := newBox(t)
	b.install()
	b.write("src/marker.txt", "ok\n")
	b.write("docs/promise/nonsense.md", `---
id: nonsense
type: promise
fields:
  statement: something nobody can evaluate
  gather:
    marker:
      gatherer: command
      run: "`+runLine(t, b.tool, "read", "src/marker.txt")+`"
      scope: "src/**"
      fields:
        ok: "'ok' in lines(stdout)"
  hold_when: obs.marker.ok and and
  trigger: cadence
---

Why: to check that a mistake is not reported as an unknown.
`)

	s := b.tick().subject(t, "promise:nonsense")
	if s.Answer.Reached() {
		t.Fatalf("an expression that will not parse produced the verdict %q", s.Answer.Value)
	}
	if s.Unevaluated == "" {
		t.Fatal("nothing says why no verdict was reached")
	}
	if n := len(b.truthLines("promise:nonsense")); n != 0 {
		t.Errorf("%d verdicts were recorded for a predicate nothing could evaluate", n)
	}
}

// An artifact that names no gatherer binds without being evaluated, which is
// what most canon does. It is not drift, and nothing is recorded about it.
func TestAnArtifactWithNoGathererIsNotDrift(t *testing.T) {
	b := newBox(t)
	b.write("docs/promise/it-just-binds.md", `---
id: it-just-binds
type: promise
fields:
  statement: this binds and nothing checks it
  trigger: cadence
---

Why: canon usually has no instrument at all.
`)
	b.ratify("it-just-binds")

	s := b.tick().subject(t, "promise:it-just-binds")
	if s.Drifting {
		t.Errorf("an artifact nothing observes is %q drift", s.Drift.Kind)
	}
	if n := len(b.dispositionLines("promise:it-just-binds")); n != 0 {
		t.Errorf("%d dispositions were written about an artifact nothing observes", n)
	}
}

// A revision starts a new series. The earlier verdicts were about a different
// statement, so they do not transfer and the promise is due again.
func TestARevisionStartsANewSeries(t *testing.T) {
	b := newBox(t)
	b.install()
	b.write("src/marker.txt", "ok\n")
	b.promise("the-marker-says-ok", "src/**", runLine(t, b.tool, "read", "src/marker.txt"), "")
	b.ratify("the-marker-says-ok")
	b.tick()

	// Same world, different wording: the scope moves to a path that holds
	// nothing, which changes what binds.
	b.promise("the-marker-says-ok", "src/**/*.md", runLine(t, b.tool, "read", "src/marker.txt"), "")
	s := b.tick().subject(t, marker)
	if !s.Due {
		t.Fatal("a revised promise was not due")
	}
	if n := len(b.truthLines(marker)); n != 2 {
		t.Fatalf("a revision produced %d records where the second is a new series", n)
	}
}

func valuesOf(records []ledger.Truth) []model.Value {
	out := make([]model.Value, 0, len(records))
	for _, r := range records {
		out = append(out, r.Value)
	}
	return out
}

// TestAPassRestrictedToOnePromiseMeasuresOnlyThatOne is the guard behind the
// re-derivation a repair is judged by.
//
// A repair box is a copy of a repository, so a pass over the whole corpus in
// there runs every other promise's instrument against a tree nobody asked
// about — which in this repository shells the Go toolchain into the box and
// re-enters the suite that cut it. The break is aimed at exactly that: the same
// two promises, once restricted and once not.
func TestAPassRestrictedToOnePromiseMeasuresOnlyThatOne(t *testing.T) {
	b := newBox(t)
	b.install()
	b.write("src/marker.txt", "ok\n")
	b.write("other/marker.txt", "ok\n")
	b.promise("the-marker-says-ok", "src/**", runLine(t, b.tool, "read", "src/marker.txt"), "")
	b.promise("the-other-marker-says-ok", "other/**", runLine(t, b.tool, "read", "other/marker.txt"), "")
	b.ratify("the-marker-says-ok")
	b.ratify("the-other-marker-says-ok")

	res := b.tick(func(o *Options) { o.Only = []string{marker} })
	if asked := res.subject(t, marker); !asked.Answer.Gathered {
		t.Fatalf("the promise the pass was asked about was not measured: %#v", asked.Answer)
	}
	other := res.subject(t, "promise:the-other-marker-says-ok")
	if other.Answer.Gathered {
		t.Fatal("a restricted pass measured a promise nobody asked about")
	}
	if other.Answer.Reached() {
		t.Fatalf("a restricted pass reached a verdict for a promise it did not measure: %#v", other.Answer)
	}
	if n := len(b.truthLines("promise:the-other-marker-says-ok")); n != 0 {
		t.Fatalf("a restricted pass recorded %d verdicts for a promise it did not measure", n)
	}

	// The break: unrestricted, the same pass measures both. Without this the
	// test above would pass against a tick that had stopped measuring anything.
	unrestricted := b.tick()
	if !unrestricted.subject(t, "promise:the-other-marker-says-ok").Answer.Gathered {
		t.Fatal("an unrestricted pass did not measure the second promise, so the restriction proves nothing")
	}
}

// The seam docs/policy/a-failed-gather-is-unknown lives at, end to end: a gather
// entry the corpus refuses to read is not measured with, and the reason reaches
// the truth record.
//
// It is the failure kept had already predicted and did not catch. The corpus
// reported the entry as a `bad-gather-spec` — `kept list` printed it — and the
// tick gathered with it in the same breath and recorded a confident verdict,
// because only `command` asked the question again on the gather path. The
// observation then faithfully recorded args nothing had honoured, which is
// worse than no record: a reader trusting it believes the promise was measured
// as written.
func TestASpecTheCorpusRefusesIsNotMeasuredAndTheReasonReachesTheRecord(t *testing.T) {
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

	// What `kept list` says.
	var reported *corpus.Problem
	for i, p := range b.load().Problems {
		if p.Kind == corpus.BadGatherSpec {
			reported = &b.load().Problems[i]
		}
	}
	if reported == nil {
		t.Fatal("the corpus reads a spec carrying an arg nothing takes, so this test is measuring nothing")
	}
	if !strings.Contains(reported.Detail, "bogusarg") {
		t.Fatalf("the corpus's reason does not name the arg: %s", reported.Detail)
	}

	// And what the tick says, which has to be the same thing. It never runs
	// `go test`: the refusal is reached before the gatherer is.
	s := b.tick().subject(t, "promise:tests-pass")
	if s.Answer.Value != model.Unknown {
		t.Fatalf("a spec the corpus refuses reads %q from the tick", s.Answer.Value)
	}
	if !strings.Contains(s.Answer.Detail, "bogusarg") {
		t.Errorf("the tick's reason does not name the arg: %s", s.Answer.Detail)
	}

	records := b.truthLines("promise:tests-pass")
	if len(records) != 1 {
		t.Fatalf("the tick wrote %d truth records", len(records))
	}
	if records[0].Value != model.Unknown || !strings.Contains(records[0].Detail, "bogusarg") {
		t.Errorf("the record says %q because %q", records[0].Value, records[0].Detail)
	}
	if len(records[0].From) != 0 {
		t.Errorf("a gather that never ran named %d observations", len(records[0].From))
	}
}
