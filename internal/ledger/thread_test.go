package ledger

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestThreadReceiptAppendUsesTheRunEnvelopeAndOneThreadPath(t *testing.T) {
	s := store(t)
	l, _ := open(t, s)
	if _, err := l.Ratify("promise:a", "sha256:aa", 1, "paul", ""); err != nil {
		t.Fatal(err)
	}
	var wrote Envelope
	err := l.AppendThread("thread-a", ThreadLifecycle, time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC), func(envelope Envelope) ([]byte, error) {
		wrote = envelope
		return json.Marshal(map[string]any{"v": envelope.V, "kind": envelope.Kind})
	})
	if err != nil {
		t.Fatal(err)
	}
	if wrote.V != Version || wrote.Kind != ThreadLifecycle || wrote.Writer == "" || wrote.Seq != 2 {
		t.Fatalf("thread envelope = %+v, want the next ledger sequence", wrote)
	}
	src, err := s.Read("threads/thread-a.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(strings.TrimSpace(string(src)), "\n") != 0 {
		t.Fatal("one thread receipt append was more than one JSONL line")
	}
	if err := l.AppendThread("thread-a", Ratified, time.Now(), func(Envelope) ([]byte, error) { return []byte("{}"), nil }); err == nil {
		t.Fatal("an act kind was accepted into a per-thread receipt")
	}
}
