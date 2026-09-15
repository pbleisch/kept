package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/repair"
	"github.com/pbleisch/kept/internal/run"
	"github.com/pbleisch/kept/internal/thread"
)

// unfinishedThread writes the receipt of a session shaped like thread
// 1y6kx8c48hb6mtjkpda76brtz0: it started, streamed, had its runtime replaced
// underneath it, said why on stderr, and exited 0 with its output cut off
// partway through an event. It goes through the same thread.Receipt a dispatch
// uses, so a change to what a receipt may hold breaks this too.
func (w *workspace) unfinishedThread(t *testing.T) string {
	t.Helper()
	store := ledger.ForWorkspace(w.root)
	l := ledger.Open(store, run.Start("test-diagnosis", nil))
	receipt, err := thread.NewReceipt(l, thread.MintID())
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := receipt.ForAdapter(thread.Claude, "claude-code:2.1.271+unreviewed/stream-json:1")
	if err != nil {
		t.Fatal(err)
	}

	executable := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(executable, []byte("2.1.271"), 0o755); err != nil {
		t.Fatal(err)
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(receipt.RecordExecutable(thread.IdentifyExecutable(executable, thread.AtLaunch, "2.1.271")))
	must(receipt.Started())
	init, err := adapter.Native("system", json.RawMessage(`{"type":"system","subtype":"init","model":"claude-sonnet-5"}`), thread.Redaction{})
	must(err)
	must(adapter.Model(init, "claude-sonnet-5"))
	assistant, err := adapter.Native("assistant", json.RawMessage(`{"type":"assistant","message":{"content":[]}}`), thread.Redaction{})
	must(err)
	must(adapter.Turn(assistant, 81))
	_, err = adapter.Native("stream_event", json.RawMessage(`{"type":"stream_event","event":{"type":"content_block_delta"}}`), thread.Redaction{})
	must(err)
	must(adapter.Undecodable([]byte(`{"type":"stream_event","event":{"type":"content_blo`), "decode Claude native stream event: unexpected end of JSON input", true, thread.Redaction{}))

	// Replaced the way npm does it.
	must(os.Rename(executable, executable+".retired"))
	must(os.WriteFile(executable, []byte("2.1.272, somebody else's install"), 0o755))
	must(os.Remove(executable + ".retired"))
	must(receipt.RecordStderr([]byte("warning: something earlier\nfatal: the reason it stopped\n"), thread.Redaction{}, 1024))
	must(receipt.RecordExecutable(thread.IdentifyExecutable(executable, thread.AtExit, "")))
	code := 0
	must(receipt.Exited(thread.Exit{Code: &code}))
	must(l.Close(run.Clean))
	return string(receipt.ID())
}

// TestAFailedRepairExplainsItselfFromItsReceipt is the report the captain did
// not have on 2026-09-14, when a repair that had run for nineteen minutes said
// one line and discarded its box: which phase failed, how far the session got,
// how it ended, whether its runtime was replaced, what it last said, where its
// work is, and what to run.
func TestAFailedRepairExplainsItselfFromItsReceipt(t *testing.T) {
	w := newWorkspace(t, map[string]string{"first": "the first promise"})
	id := w.unfinishedThread(t)

	var out bytes.Buffer
	writeRepairFailure(&out, ledger.ForWorkspace(w.root), &repair.Failure{
		Phase:        repair.PhaseSession,
		Promise:      "promise:first",
		Thread:       thread.ID(id),
		Box:          "/repository/.kept/boxes/" + id,
		RuntimeAfter: "2.1.272",
		Err:          errors.New("retain runtime output: the ledger refused an event"),
	})
	report := out.String()
	mustContain(t, "the failure report", report,
		"kept: promise:first: the session ran, and what it wrote or how it ended could not be recorded",
		"thread   "+id,
		"exited 0 at",
		"81 turns and 3 events; the last was stream_event, recorded at",
		"no result arrived, and the output ended partway through a line",
		"claude 2.1.271 from ",
		"replaced during the session, and it reports 2.1.272 now",
		`"fatal: the reason it stopped" is the last line; 1024 bytes before the retained end were not kept`,
		"kept at /repository/.kept/boxes/"+id,
		"retain runtime output: the ledger refused an event",
		"git -C /repository/.kept/boxes/"+id+" diff reads what the session left",
		"kept review "+id,
		"kept why promise:first",
		"kept repair promise:first tries again",
	)
	// The error row carries the failure without the box sentence, which has a
	// row of its own.
	if strings.Count(report, "kept at /repository") != 1 {
		t.Fatalf("the report says where the box is more than once:\n%s", report)
	}
}

// TestReviewOfAThreadThatNeverFinishedSaysHowFarItsSessionGot is the same
// account a day later, from the verb a person types to read a thread.
func TestReviewOfAThreadThatNeverFinishedSaysHowFarItsSessionGot(t *testing.T) {
	w := newWorkspace(t, map[string]string{"first": "the first promise"})
	id := w.unfinishedThread(t)

	_, stdout, stderr := w.run("review", id)
	mustContain(t, "the review", stdout,
		"session  started",
		"reached  81 turns and 3 events",
		"closed   no result arrived, and the output ended partway through a line",
		"a different file was at that path after the session",
		`"fatal: the reason it stopped" is the last line`,
	)
	if strings.Contains(stderr, "panic") {
		t.Fatal(stderr)
	}

	_, stdout, _ = w.run("review", id, "--json")
	var doc struct {
		Assessment struct {
			Session *struct {
				Events       int    `json:"events"`
				LastEvent    string `json:"last_event"`
				Turns        uint64 `json:"turns"`
				Closed       bool   `json:"closed"`
				ClosingEvent string `json:"closing_event"`
				Unterminated bool   `json:"unterminated"`
				Replaced     bool   `json:"replaced"`
				Launch       *struct {
					Version string `json:"version"`
				} `json:"launch"`
				Stderr *struct {
					Omitted int64 `json:"omitted"`
				} `json:"stderr"`
			} `json:"session"`
		} `json:"assessment"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("the review document does not parse: %v\n%s", err, stdout)
	}
	s := doc.Assessment.Session
	switch {
	case s == nil:
		t.Fatalf("the review document carries no session:\n%s", stdout)
	case s.Events != 3 || s.LastEvent != "stream_event" || s.Turns != 81:
		t.Fatalf("session reached %d events, last %q, %d turns", s.Events, s.LastEvent, s.Turns)
	case s.Closed || s.ClosingEvent != "result" || !s.Unterminated:
		t.Fatalf("session closed %v by %q, unterminated %v", s.Closed, s.ClosingEvent, s.Unterminated)
	case !s.Replaced || s.Launch == nil || s.Launch.Version != "2.1.271":
		t.Fatalf("session replaced %v, launch %#v", s.Replaced, s.Launch)
	case s.Stderr == nil || s.Stderr.Omitted != 1024:
		t.Fatalf("session stderr %#v", s.Stderr)
	}
}
