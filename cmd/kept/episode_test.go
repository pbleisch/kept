package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/assess"
	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/gather"
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/progress"
	"github.com/pbleisch/kept/internal/register"
	"github.com/pbleisch/kept/internal/repair"
	"github.com/pbleisch/kept/internal/run"
	"github.com/pbleisch/kept/internal/thread"
)

// TestReworkAndAPromiseAreNotBoth: a rework's promise is the one its thread was
// opened for, so naming another beside it is two answers to one question.
func TestReworkAndAPromiseAreNotBoth(t *testing.T) {
	w := newWorkspace(t, nil)
	w.put("docs/promise/counted.md", corpusPromise("counted", "obs.c.artifacts == 99"))
	code, _, stderr := w.run("repair", "promise:counted", "--rework", "6ca1kqeyfz205a2s4qf4cb991a")
	if code != exitUsage {
		t.Fatalf("a promise and --rework together exited %d, want %d", code, exitUsage)
	}
	mustContain(t, "the refusal", stderr, "a promise or --rework <thread>, not both")

	// And a rework of a thread this ledger holds nothing for is refused by name,
	// as a usage error, before anything is cut.
	if code, _, stderr := w.run("ratify", "promise:counted", "--by", "the operator"); code != exitOK {
		t.Fatalf("ratify exited %d: %s", code, stderr)
	}
	w.put(".kept/config.yaml", "corpus: docs\nconfine:\n  provider: nono\ntimeouts:\n  agent: 1m\n")
	code, stdout, stderr := w.run("repair", "--rework", "6ca1kqeyfz205a2s4qf4cb991a")
	if code != exitUsage || !strings.Contains(stderr, "has no receipt in this ledger") {
		t.Fatalf("a rework of an unknown thread exited %d:\n%s\n%s", code, stdout, stderr)
	}
}

// TestEachFinishedPhaseIsOneLineSayingWhatItEstablished holds the lines a run
// prints while it goes to saying what each phase found, rather than that it
// happened. See docs/decision/0086.
func TestEachFinishedPhaseIsOneLineSayingWhatItEstablished(t *testing.T) {
	code := 0
	for _, c := range []struct {
		event repair.Event
		want  string
	}{
		{repair.Event{Kind: repair.AttemptStarted, Attempt: 1, Thread: "T", Commit: strings.Repeat("a", 40)},
			"attempt 1 · thread T · box cut at aaaaaaa"},
		{repair.Event{Kind: repair.AttemptStarted, Attempt: 2, Thread: "T", Commit: strings.Repeat("b", 40), Rework: true},
			"attempt 2 · thread T · box cut at bbbbbbb, where the earlier attempts left the work"},
		{repair.Event{Kind: repair.SessionEnded, Attempt: 1, Session: thread.Diagnosis{
			Started: time.Unix(0, 0), Ended: time.Unix(47*60+22, 0), Exit: &thread.Exit{Code: &code}, Closed: true, ClosingEvent: "result"}},
			"attempt 1 · session exited 0 after 47m22s, and the runtime's result arrived"},
		{repair.Event{Kind: repair.Rederived, Attempt: 1, Before: model.NotKept, After: model.Kept},
			"attempt 1 · re-derived from the box: not-kept → kept"},
		{repair.Event{Kind: repair.Anchored, Attempt: 1, Commit: strings.Repeat("c", 40)},
			"attempt 1 · anchored at ccccccc"},
		{repair.Event{Kind: repair.PassStarted, Attempt: 1},
			"attempt 1 · adversarial pass started"},
		{repair.Event{Kind: repair.PassEnded, Attempt: 1, Pass: repair.Argued, Ruling: thread.RulingReject},
			"attempt 1 · adversarial pass ruled reject"},
		{repair.Event{Kind: repair.PassEnded, Attempt: 1, Pass: repair.Argued, Detail: "its last line is \"done\""},
			"attempt 1 · adversarial pass argued and made no ruling kept can read: its last line is \"done\""},
		{repair.Event{Kind: repair.Assessed, Attempt: 1},
			"attempt 1 · checks about the act: all clear"},
		{repair.Event{Kind: repair.Assessed, Attempt: 1, Fired: []assess.Finding{{Check: assess.PropertyExercised}}},
			"attempt 1 · checks about the act: fired property-exercised"},
		{repair.Event{Kind: repair.Decided, Attempt: 1, Action: repair.ActionRework, Why: []string{"the adversarial pass ruled reject"}, Window: "no repair session in this episode has reported a usage window, which refuses nothing"},
			"attempt 1 · rework: the adversarial pass ruled reject · no repair session in this episode has reported a usage window"},
		{repair.Event{Kind: repair.Decided, Attempt: 2, Action: repair.ActionReady, Why: []string{"the promise holds"}},
			"attempt 2 · ready to land: the promise holds"},
	} {
		if got := phaseLine(c.event); !strings.HasPrefix(got, c.want) {
			t.Errorf("%s reads %q, want %q", c.event.Kind, got, c.want)
		}
	}
}

// TestAPhaseLineIsPrintedOnStdoutAsItHappens is the wiring between the events a
// run emits and the lines a person reads: stdout, one line per event, the same
// bytes with or without a display.
func TestAPhaseLineIsPrintedOnStdoutAsItHappens(t *testing.T) {
	var stdout, stderr bytes.Buffer
	show := progress.For(&stdout, &stderr, false)
	emit := phaseLines(show, &stdout)
	emit(repair.Event{Kind: repair.Anchored, Attempt: 1, Commit: strings.Repeat("c", 40)})
	emit(repair.Event{Kind: repair.PassEnded, Attempt: 1, Pass: repair.Argued, Ruling: thread.RulingAccept})
	show.Close()
	if stdout.String() != "attempt 1 · anchored at ccccccc\nattempt 1 · adversarial pass ruled accept\n" {
		t.Fatalf("stdout is %q", stdout.String())
	}
	var quiet bytes.Buffer
	phaseLines(nil, &quiet)(repair.Event{Kind: repair.Anchored, Attempt: 1, Commit: strings.Repeat("c", 40)})
	if quiet.String() != "attempt 1 · anchored at ccccccc\n" {
		t.Fatalf("a run nobody watches printed %q", quiet.String())
	}
}

// TestARunEndsWithWhatToDoNext is the last thing a run prints, for each way it
// can end, and it is always a command a person can paste.
func TestARunEndsWithWhatToDoNext(t *testing.T) {
	anchor := thread.Anchor{Commit: strings.Repeat("c", 40), Parent: strings.Repeat("b", 40), Promise: "promise:counted"}
	repaired := &repair.Result{Status: repair.Repaired, Anchor: anchor}
	for _, c := range []struct {
		name string
		ep   *repair.Episode
		want []string
		not  []string
	}{
		{
			name: "ready to land",
			ep:   &repair.Episode{Promise: "promise:counted", Thread: "T", End: repair.Ready, Attempts: []*repair.Result{repaired}, Why: []string{"the promise holds and every check about the act is clear, and the adversarial pass ruled accept"}},
			want: []string{"ready to land", "anchored at ccccccc", "next", "kept review T", "kept land T --onto origin/main"},
			not:  []string{"--rework"},
		},
		{
			name: "escalated on a repeating finding",
			ep: &repair.Episode{Promise: "promise:counted", Thread: "T", End: repair.Escalated, Attempts: []*repair.Result{repaired, repaired},
				Why:     []string{"property-exercised fired again, as it did after attempt 1"},
				Handoff: ledger.Disposition{Outcome: ledger.OutcomeEscalate, Thread: "T"}, Recorded: true},
			want: []string{"escalated to a person", "recorded: escalate", "kept review T", "kept repair --rework T", "revise promise:counted so the repeating finding is a clause", "kept land T --onto origin/main"},
		},
		{
			name: "refused on budget before any attempt",
			ep: &repair.Episode{Promise: "promise:counted", Thread: "T", End: repair.Stopped, Why: []string{"five_hour reads 0.95, at or above the usage-window ceiling of 0.9 in your config"},
				Handoff: ledger.Disposition{Outcome: ledger.OutcomeRefused, Condition: ledger.Budget}, Recorded: true},
			want: []string{"stopped", "recorded: refused · budget", "kept repair --rework T once the usage window has room"},
		},
		{
			name: "the promise holds",
			ep:   &repair.Episode{Promise: "promise:counted", End: repair.Stopped, Attempts: []*repair.Result{{Status: repair.Holds}}, Why: []string{"the promise holds where the box was cut, so there is nothing to repair"}},
			want: []string{"stopped", "next", "nothing to repair at that commit"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			var out bytes.Buffer
			writeEpisode(&out, c.ep, "origin/main")
			said := out.String()
			mustContain(t, "the end of the run", said, c.want...)
			for _, unwanted := range c.not {
				if strings.Contains(said, unwanted) {
					t.Errorf("the end of the run says %q:\n%s", unwanted, said)
				}
			}
			if !strings.Contains(said, "\nnext\n") {
				t.Errorf("the run does not end with what to do next:\n%s", said)
			}
		})
	}
}

// TestAnEscalatedEpisodeIsVisibleInReviewStatusAndWhy is an episode of two
// attempts handed to a person, read back by every verb a person would reach
// for: review shows each attempt and what followed it, status lists the
// escalation beside the register, and why says it before the disposition in
// force. None of them writes.
func TestAnEscalatedEpisodeIsVisibleInReviewStatusAndWhy(t *testing.T) {
	w := newWorkspace(t, nil)
	w.put("docs/promise/counted.md", corpusPromise("counted", "obs.c.artifacts == 99"))
	if code, _, stderr := w.run("ratify", "promise:counted", "--by", "the operator"); code != exitOK {
		t.Fatalf("ratify exited %d: %s", code, stderr)
	}
	id := w.escalatedEpisode(t, "promise:counted")

	code, stdout, stderr := w.run("review", string(id))
	if code == exitUsage {
		t.Fatalf("review exited %d: %s", code, stderr)
	}
	mustContain(t, "the review", stdout,
		"attempts",
		"cut bbbbbbb",
		"anchored aaaaaaa",
		"ruling: reject",
		"reworked",
		"escalated",
		"the attempts budget is spent",
		"the checklist below is over the episode's whole change",
	)

	_, stdout, _ = w.run("review", string(id), "--json")
	var doc struct {
		Assessment struct {
			Attempts []struct {
				Number int    `json:"number"`
				Ruling string `json:"ruling"`
			} `json:"attempts"`
			Escalation *struct {
				Detail string `json:"detail"`
			} `json:"escalation"`
			Argument struct {
				Ruling string `json:"ruling"`
			} `json:"argument"`
		} `json:"assessment"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("the review document does not parse: %v\n%s", err, stdout)
	}
	if len(doc.Assessment.Attempts) != 2 || doc.Assessment.Attempts[1].Number != 2 || doc.Assessment.Attempts[0].Ruling != "reject" {
		t.Fatalf("the review document carries attempts %#v", doc.Assessment.Attempts)
	}
	if doc.Assessment.Escalation == nil || doc.Assessment.Argument.Ruling != "reject" {
		t.Fatalf("the review document carries escalation %v and ruling %q", doc.Assessment.Escalation, doc.Assessment.Argument.Ruling)
	}

	_, stdout, _ = w.run("status")
	mustContain(t, "the status", stdout, "1 repair episode is escalated to a person", "thread "+string(id)+"  promise:counted", "kept repair --rework "+string(id))
	_, stdout, _ = w.run("status", "--json")
	if !strings.Contains(stdout, `"escalation"`) {
		t.Fatalf("the status document does not carry the escalation:\n%s", stdout)
	}

	_, stdout, _ = w.run("why", "promise:counted")
	mustContain(t, "why", stdout, "escalated to a person since", "thread "+string(id), "the attempts budget is spent")
}

// escalatedEpisode writes a thread of two attempts and the escalation that ended
// it into this workspace's ledger, through the same receipt and ledger a run
// uses. The commits are named rather than made: the verbs under test read the
// record and say what the diff could not be read, which is not what this is
// about.
func (w *workspace) escalatedEpisode(t *testing.T, promise string) thread.ID {
	t.Helper()
	store := ledger.ForWorkspace(w.root)
	l := ledger.Open(store, run.Start("test-episode", nil))
	c, err := corpus.Load(filepath.Join(w.root, "docs"), w.root, corpus.Options{Gatherers: gather.Builtin()})
	if err != nil {
		t.Fatal(err)
	}
	acts, _, err := ledger.ReadActs(store)
	if err != nil {
		t.Fatal(err)
	}
	var standing register.Standing
	for _, s := range register.Derive(c, acts) {
		if s.Artifact == promise {
			standing = s
		}
	}
	entry, _ := c.Get(promise)
	id := thread.MintID()
	receipt, err := thread.NewReceipt(l, id)
	if err != nil {
		t.Fatal(err)
	}
	base, first, second := strings.Repeat("b", 40), strings.Repeat("a", 40), strings.Repeat("d", 40)
	input := thread.PacketInput{
		Promise: thread.RatifiedPromise{
			ID: promise, Hash: standing.Hash.Digest, Norm: standing.Hash.Norm, Ratified: true,
			Statement: entry.Artifact.Statement, Gather: entry.Artifact.Gather, HoldWhen: entry.Artifact.HoldWhen,
			Trigger: entry.Artifact.Trigger, RepairScope: entry.Artifact.RepairScope,
		},
		Why: "because",
		Observations: map[string]ledger.Observation{"c": {
			V: ledger.ObsVersion, At: time.Now().UTC(), Gatherer: "corpus@1",
			World: model.World{Tree: "sha256:tree", Thread: string(id)}, Scope: "sha256:scope", Data: map[string]any{"artifacts": 1},
		}},
		Policies: []thread.RatifiedDispatchPolicy{{ID: "policy:p", Hash: "sha256:p", Norm: 1, Ratified: true, Statement: "s", AppliesTo: "dispatch", OnFail: "refuse"}},
	}
	session := func(packet thread.Packet, anchor string, said string) {
		ref, err := packet.Store(l)
		if err != nil {
			t.Fatal(err)
		}
		if err := receipt.RecordPacket(ref); err != nil {
			t.Fatal(err)
		}
		if err := receipt.RecordAnchor(thread.Anchor{Commit: anchor, Parent: base, Promise: promise}); err != nil {
			t.Fatal(err)
		}
		contents, err := thread.ReadPacket(packet.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		brief, err := thread.AssembleBrief(thread.BriefInput{Thread: string(id), Packet: contents, Anchor: thread.Anchor{Commit: anchor, Parent: base, Promise: promise}})
		if err != nil {
			t.Fatal(err)
		}
		briefRef, err := brief.Store(l)
		if err != nil {
			t.Fatal(err)
		}
		if err := receipt.RecordBrief(briefRef); err != nil {
			t.Fatal(err)
		}
		words, err := l.PutArtifact(ledger.ThreadArgumentArtifact, []byte(said))
		if err != nil {
			t.Fatal(err)
		}
		if err := receipt.RecordArgument(thread.ArgumentRecord{Runtime: thread.Claude, AdapterVersion: "test@1", Brief: briefRef, Artifact: words}); err != nil {
			t.Fatal(err)
		}
	}
	packet, err := thread.AssemblePacket(input)
	if err != nil {
		t.Fatal(err)
	}
	session(packet, first, "The tests assert less than their names.\n\nruling: reject")
	input.Rework = &thread.Rework{Base: base, Tip: first, Earlier: []thread.EarlierAttempt{{Argument: "The tests assert less than their names.\n\nruling: reject", Ruling: thread.RulingReject}}}
	packet, err = thread.AssemblePacket(input)
	if err != nil {
		t.Fatal(err)
	}
	session(packet, second, "Still weak.\n\nruling: reject")

	if _, err := l.Dispose(ledger.Disposition{
		Artifact: promise, Hash: standing.Hash.String(), Norm: standing.Hash.Norm, World: model.World{Tree: "sha256:tree"},
		Outcome: ledger.OutcomeEscalate, Thread: string(id),
		Detail: "the adversarial pass ruled reject; this run has opened 2 of the 2 attempts it may: the attempts budget is spent",
	}); err != nil {
		t.Fatal(err)
	}
	if err := l.Close(run.Clean); err != nil {
		t.Fatal(err)
	}
	return id
}
