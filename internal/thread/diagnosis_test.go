package thread

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/ledger"
)

// sessionChild is a runtime that ran: it may have been replaced on disk while
// it did, and it may have written something on its error stream.
type sessionChild struct {
	// replace, when set, is the executable a package manager replaces the way
	// npm does while this child runs: renamed away, a new file in its place, the
	// old one deleted.
	replace string
	stderr  []byte
	omitted int64
}

func (*sessionChild) Kill() error { return nil }

func (c *sessionChild) Wait() Exit {
	if c.replace != "" {
		retired := c.replace + ".retired"
		_ = os.Rename(c.replace, retired)
		_ = os.WriteFile(c.replace, []byte("a newer runtime, installed by somebody else"), 0o700)
		_ = os.Remove(retired)
	}
	code := 0
	return Exit{Code: &code}
}

// Diagnostics is what a launch that kept the runtime's error stream answers.
func (c *sessionChild) Diagnostics() ([]byte, int64) { return c.stderr, c.omitted }

// sessionLaunch writes a runtime's output into the stream it is handed, the
// way a confinement launch forwards it, and hands back the child.
type sessionLaunch struct {
	output []byte
	child  Child
}

func (sessionLaunch) Verified() error { return nil }

func (l sessionLaunch) Start(_ context.Context, invocation Invocation) (Child, error) {
	if invocation.Output != nil && len(l.output) != 0 {
		if _, err := invocation.Output.Write(l.output); err != nil {
			return nil, err
		}
	}
	return l.child, nil
}

func dispatchSession(t *testing.T, launch sessionLaunch, cfg RuntimeConfig) (*Receipt, []Entry) {
	t.Helper()
	receipt, store := testReceipt(t)
	packet, err := AssemblePacket(packetInput())
	if err != nil {
		t.Fatal(err)
	}
	if err := Dispatch(context.Background(), cfg, packet, launch, Supervisor{Timeout: time.Minute}, receipt); err != nil {
		t.Fatal(err)
	}
	entries, quarantined, err := ReadReceipt(store, receipt.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(quarantined) != 0 {
		t.Fatalf("quarantined: %#v", quarantined)
	}
	if last := entries[len(entries)-1]; last.Lifecycle == nil || last.Lifecycle.Event != Exited {
		t.Fatalf("the receipt does not end at the reaped exit: %s", ReceiptText(entries))
	}
	return receipt, entries
}

// TestADispatchSaysWhenItsRuntimeWasReplacedUnderneathIt is the fact both failed
// threads of 2026-09-14 had to be argued from timestamps: the binary at the
// runtime's path was not the one launched. It is two records now, and the
// comparison is between them.
func TestADispatchSaysWhenItsRuntimeWasReplacedUnderneathIt(t *testing.T) {
	cfg := claudeConfig(t)
	receipt, entries := dispatchSession(t, sessionLaunch{child: &sessionChild{replace: cfg.Executable}}, cfg)

	launch, exit, launched, exited := Executables(entries)
	if !launched || !exited {
		t.Fatalf("the receipt does not hold both observations: %s", ReceiptText(entries))
	}
	if launch.Version != cfg.Version || launch.Path != cfg.Executable || launch.Missing || launch.Inode == 0 {
		t.Fatalf("launch observation = %#v, want the admitted version and an identity", launch)
	}
	if exit.Version != "" {
		t.Fatalf("the exit observation claims a version nothing asked for: %#v", exit)
	}
	if launch.SameFile(exit) {
		t.Fatalf("a replaced executable reads as the same file: %#v and %#v", launch, exit)
	}
	session, found := RepairSession(entries)
	if !found || !Diagnose(session).Replaced() {
		t.Fatalf("the diagnosis does not say the runtime was replaced")
	}
	if _, _, replaced := receipt.Replaced(); !replaced {
		t.Fatal("the receipt's own reading does not say the runtime was replaced")
	}
}

// TestADispatchWhoseRuntimeStayedPutSaysSo is the other branch, without which
// the test above could be about any two records.
func TestADispatchWhoseRuntimeStayedPutSaysSo(t *testing.T) {
	cfg := claudeConfig(t)
	receipt, entries := dispatchSession(t, sessionLaunch{child: &sessionChild{}}, cfg)
	launch, exit, launched, exited := Executables(entries)
	if !launched || !exited || !launch.SameFile(exit) {
		t.Fatalf("an untouched executable reads as replaced: %#v and %#v", launch, exit)
	}
	if _, _, replaced := receipt.Replaced(); replaced {
		t.Fatal("the receipt says an untouched runtime was replaced")
	}
}

// TestAReapedRuntimesErrorStreamIsRedactedAndKept is what a runtime says when
// its native stream does not. It is kept, and it is kept only once the
// adapter's own redaction has run over it — a runtime that names its credential
// in an error has still written the credential.
func TestAReapedRuntimesErrorStreamIsRedactedAndKept(t *testing.T) {
	cfg := claudeConfig(t)
	stderr := []byte("starting\nerror: the credential test-claude-setup-token was refused\n")
	_, entries := dispatchSession(t, sessionLaunch{child: &sessionChild{stderr: stderr, omitted: 42}}, cfg)

	var kept *Stderr
	for _, entry := range entries {
		if entry.Stderr != nil {
			kept = entry.Stderr
		}
	}
	if kept == nil {
		t.Fatalf("the runtime's error stream was not kept: %s", ReceiptText(entries))
	}
	if kept.Omitted != 42 || !kept.Redaction.Occurred || kept.Redaction.Matches != 1 {
		t.Fatalf("stderr record = %#v, want the omitted count and one redaction", kept)
	}
}

// TestAnErrorStreamIsKeptOnlyWhereItCanBeRedactedAndWhereItWasRetained holds
// the two ways nothing is recorded: a launch that kept no error stream, and an
// invocation with no redaction to apply to one.
func TestAnErrorStreamIsKeptOnlyWhereItCanBeRedactedAndWhereItWasRetained(t *testing.T) {
	stderr := []byte("error: the credential test-claude-setup-token was refused\n")

	receipt, store := testReceipt(t)
	invocation := Invocation{Path: "/nonexistent/runtime"}
	if err := recordStderr(&sessionChild{stderr: stderr}, invocation, receipt); err != nil {
		t.Fatal(err)
	}
	if entries, _, err := ReadReceipt(store, receipt.ID()); err == nil && len(entries) != 0 {
		t.Fatalf("an error stream with nothing to redact it was recorded: %s", ReceiptText(entries))
	}

	invocation.Redact = func(src []byte) ([]byte, Redaction) { return redactClaude(src, []byte("test-claude-setup-token")) }
	if err := recordStderr(plainLaunchChild{}, invocation, receipt); err != nil {
		t.Fatal(err)
	}
	if entries, _, err := ReadReceipt(store, receipt.ID()); err == nil && len(entries) != 0 {
		t.Fatalf("a launch that kept no error stream recorded one: %s", ReceiptText(entries))
	}
}

type plainLaunchChild struct{}

func (plainLaunchChild) Kill() error { return nil }
func (plainLaunchChild) Wait() Exit  { return Exit{} }

// TestTheKeptErrorStreamHoldsNoCredential reads the artifact itself, because a
// redaction count is a claim and the bytes are the evidence.
func TestTheKeptErrorStreamHoldsNoCredential(t *testing.T) {
	cfg := claudeConfig(t)
	stderr := []byte("error: the credential test-claude-setup-token was refused\n")
	receipt, store := testReceipt(t)
	packet, err := AssemblePacket(packetInput())
	if err != nil {
		t.Fatal(err)
	}
	launch := sessionLaunch{child: &sessionChild{stderr: stderr}}
	if err := Dispatch(context.Background(), cfg, packet, launch, Supervisor{Timeout: time.Minute}, receipt); err != nil {
		t.Fatal(err)
	}
	entries, _, err := ReadReceipt(store, receipt.ID())
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Stderr == nil {
			continue
		}
		kept, err := ledger.ReadArtifact(store, entry.Stderr.Artifact)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(kept, []byte(cfg.ClaudeSetupToken)) || !bytes.Contains(kept, []byte("[REDACTED]")) {
			t.Fatalf("the credential reached the kept error stream: %q", kept)
		}
		return
	}
	t.Fatalf("no error stream was kept: %s", ReceiptText(entries))
}

// TestDiagnoseReadsHowFarASessionGot is thread 1y6kx8c48hb6mtjkpda76brtz0's
// shape, dispatched: a session that started, streamed, and exited 0 with its
// output cut off partway through an event and no result.
func TestDiagnoseReadsHowFarASessionGot(t *testing.T) {
	cfg := claudeConfig(t)
	output := strings.Join([]string{
		`{"type":"system","subtype":"init","model":"claude-sonnet-5"}`,
		`{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"working"}}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"still working"}]}}`,
		`{"type":"stream_event","event":{"type":"content_block_delta"`,
	}, "\n")
	_, entries := dispatchSession(t, sessionLaunch{output: []byte(output), child: &sessionChild{}}, cfg)
	session, found := RepairSession(entries)
	if !found {
		t.Fatalf("no repair session in %s", ReceiptText(entries))
	}
	d := Diagnose(session)
	switch {
	case d.Runtime != Claude || d.Model != "claude-sonnet-5":
		t.Fatalf("runtime %q model %q", d.Runtime, d.Model)
	case d.Started.IsZero() || d.Ended.IsZero() || d.Exit == nil || d.Exit.Code == nil || *d.Exit.Code != 0:
		t.Fatalf("lifecycle = %v → %v, exit %#v", d.Started, d.Ended, d.Exit)
	case d.Natives != 3 || d.LastNative != "assistant" || d.Turns != 1:
		t.Fatalf("reached %d events, last %q, %d turns", d.Natives, d.LastNative, d.Turns)
	case d.Closed || d.ClosingEvent != "result":
		t.Fatalf("closed %v by %q, want no result", d.Closed, d.ClosingEvent)
	case d.Undecodable != 1 || !d.Unterminated:
		t.Fatalf("undecodable %d unterminated %v, want the torn line", d.Undecodable, d.Unterminated)
	case !d.LaunchObserved || !d.ExitedObserved || d.Replaced():
		t.Fatalf("executable observations %v %v replaced %v", d.LaunchObserved, d.ExitedObserved, d.Replaced())
	case d.Anchored || d.AfterMeasured:
		t.Fatal("a session that anchored nothing reads as anchored")
	}
}
