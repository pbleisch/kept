//go:build !windows

package thread

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

// This file is the interruption machinery, exercised against a stand-in.
//
// It is deliberately not decision 0052's admission evidence and must never be
// read as it. 0052 requires a real Claude session, confined, carrying a real
// credential, killed after it has produced several receipt updates; that
// session exists and it is in internal/repair/claudeadmission_test.go, which
// skips on a machine with no configured credential. What a stand-in proves is
// the other thing, and it proves it everywhere and for nothing: that the
// adapter's stream, the supervisor's deadline and a real process group's kill
// and reap compose into the partial receipt the decision describes. A stand-in
// proves the plumbing. The plumbing is not the proof.

// claudeStandIn emits the runtime's own event shapes and then outlives its
// deadline. It replays the packet line it was given the way
// --replay-user-messages does, so the input channel is exercised rather than
// assumed, and it holds the process open afterwards so the kill lands on
// something that is still running.
const claudeStandIn = `
read -r packet
printf '%s\n' '{"type":"system","subtype":"init","claude_code_version":"2.1.241","mcp_servers":[],"plugins":[]}'
printf '%s\n' "$packet"
printf '%s\n' '{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"partial repair evidence"}}}'
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"still working"}]}}'
exec sleep 120
`

// standInLaunch starts a real process in its own group and kills the group, so
// the deadline path under test is the one a confinement launch takes rather
// than a channel a test closed. It is not confinement: it renders no profile,
// runs no canary and bounds nothing. internal/confine owns that, and its own
// tests measure it.
type standInLaunch struct{ verified error }

func (s standInLaunch) Verified() error { return s.verified }

func (s standInLaunch) Start(_ context.Context, invocation Invocation) (Child, error) {
	cmd := exec.Command(invocation.Path, invocation.Args...)
	cmd.Env = invocation.Env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	if len(invocation.Input) != 0 {
		if _, err := stdin.Write(invocation.Input); err != nil {
			return nil, err
		}
	}
	if err := stdin.Close(); err != nil {
		return nil, err
	}
	child := &standInChild{cmd: cmd, drained: make(chan struct{})}
	go func() {
		_, child.copyErr = io.Copy(invocation.Output, stdout)
		close(child.drained)
	}()
	return child, nil
}

type standInChild struct {
	cmd     *exec.Cmd
	drained chan struct{}
	copyErr error
}

func (c *standInChild) Kill() error {
	return syscall.Kill(-c.cmd.Process.Pid, syscall.SIGKILL)
}

func (c *standInChild) Wait() Exit {
	<-c.drained
	waitErr := c.cmd.Wait()
	if state := c.cmd.ProcessState; state != nil {
		if status, ok := state.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			return Exit{Signal: status.Signal().String()}
		}
		if code := state.ExitCode(); code >= 0 {
			return Exit{Code: &code}
		}
	}
	code := 1
	if waitErr == nil {
		code = 0
	}
	return Exit{Code: &code}
}

// TestClaudeStandInKilledMidStreamRetainsItsPartialReceipt is the half a
// completion-only report loses. The deadline elapses while the stand-in is
// still alive, so what is asserted is that the events already received survive
// beside the timeout, the kill and the exit the parent actually observed — and
// that nothing was written after the reap.
func TestClaudeStandInKilledMidStreamRetainsItsPartialReceipt(t *testing.T) {
	const interrupt = 2 * time.Second
	invocation, receipt, store := prepareClaudeFixture(t)
	invocation.Path = "/bin/sh"
	invocation.Args = []string{"-c", claudeStandIn}
	invocation.Env = nil

	err := (Supervisor{Timeout: interrupt}).Run(context.Background(), standInLaunch{}, invocation, receipt)
	if err != nil {
		t.Fatal(err)
	}
	entries, quarantined, err := ReadReceipt(store, receipt.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(quarantined) != 0 {
		t.Fatalf("the interrupted stand-in receipt quarantined its own evidence: %#v", quarantined)
	}

	// The lifecycle the parent observed, in the order it happened. Each item is
	// written only after it happened, so this sequence is a record rather than
	// an inference: timed out, then killed once the group kill succeeded, then
	// the exit that was actually reaped.
	var lifecycle []LifecycleEvent
	var timedOut, exited *Lifecycle
	for _, entry := range entries {
		if entry.Lifecycle == nil {
			continue
		}
		lifecycle = append(lifecycle, entry.Lifecycle.Event)
		switch entry.Lifecycle.Event {
		case TimedOut:
			timedOut = entry.Lifecycle
		case Exited:
			exited = entry.Lifecycle
		}
	}
	want := []LifecycleEvent{Started, TimedOut, Killed, Exited}
	if len(lifecycle) != len(want) {
		t.Fatalf("interrupted stand-in lifecycle = %v, want %v", lifecycle, want)
	}
	for i, event := range want {
		if lifecycle[i] != event {
			t.Fatalf("interrupted stand-in lifecycle = %v, want %v", lifecycle, want)
		}
	}
	// The cause, and the reaped exit. A timeout that did not record its own
	// deadline is a kill nobody can account for, and a killed process group that
	// reported an exit code rather than a signal was not killed.
	if timedOut == nil || timedOut.Timeout != interrupt.String() {
		t.Fatalf("the interrupted stand-in recorded no timeout cause: %#v", timedOut)
	}
	if exited == nil || exited.Exit == nil || exited.Exit.Signal == "" {
		t.Fatalf("the killed stand-in did not record a reaped terminating signal: %#v", exited)
	}

	// The partial evidence itself, which is the point: native events, the
	// packet the runtime read back out, and an incremental transcript — all of
	// it retained from a session that never reached a result.
	kinds := map[string]int{}
	for _, kind := range strings.Split(ReceiptText(entries), ",") {
		kinds[kind]++
	}
	if kinds["thread_native"] < 4 {
		t.Fatalf("the interrupted stand-in retained %d native events, so the kill measured nothing: %v", kinds["thread_native"], kinds)
	}
	if kinds["thread_transcript"] == 0 || kinds["thread_turn"] == 0 {
		t.Fatalf("the interrupted stand-in retained no incremental projection: %v", kinds)
	}
	if kinds["thread_cost"] != 0 {
		t.Fatalf("a session killed before its result reported a cost: %v", kinds)
	}
	assertClaudeProjectionsAreNativeBacked(t, entries)

	var replayed []byte
	for _, entry := range entries {
		if entry.Native != nil && entry.Native.Type == "user" {
			replayed = entry.Native.Payload
		}
	}
	if !bytes.Contains(replayed, []byte("promise:kept-is-tested")) {
		t.Fatalf("the packet did not survive the stand-in's input channel: %s", replayed)
	}

	// A native event after the exit would be a completion written after the
	// fact rather than a receipt of what arrived.
	if last := entries[len(entries)-1]; last.Lifecycle == nil || last.Lifecycle.Event != Exited {
		t.Fatalf("the interrupted receipt does not end at the reaped exit: %s", ReceiptText(entries))
	}
}

// TestClaudeStandInThatCompletesIsNotInterrupted is the other branch, without
// which the assertions above could be about any process that died. Same
// stand-in, same launch, a deadline it finishes inside.
func TestClaudeStandInThatCompletesIsNotInterrupted(t *testing.T) {
	invocation, receipt, store := prepareClaudeFixture(t)
	invocation.Path = "/bin/sh"
	invocation.Args = []string{"-c", strings.Replace(claudeStandIn, "exec sleep 120",
		`printf '%s\n' '{"type":"result","subtype":"success","num_turns":2,"total_cost_usd":0.0042}'`, 1)}
	invocation.Env = nil

	if err := (Supervisor{Timeout: time.Minute}).Run(context.Background(), standInLaunch{}, invocation, receipt); err != nil {
		t.Fatal(err)
	}
	entries, _, err := ReadReceipt(store, receipt.ID())
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Lifecycle != nil && (entry.Lifecycle.Event == TimedOut || entry.Lifecycle.Event == Killed) {
			t.Fatalf("the completed stand-in was interrupted: %#v", entry.Lifecycle)
		}
	}
	last := entries[len(entries)-1]
	if last.Lifecycle == nil || last.Lifecycle.Event != Exited || last.Lifecycle.Exit == nil || last.Lifecycle.Exit.Code == nil || *last.Lifecycle.Exit.Code != 0 {
		t.Fatalf("the completed stand-in did not exit normally: %#v", last.Lifecycle)
	}
	kinds := map[string]int{}
	for _, kind := range strings.Split(ReceiptText(entries), ",") {
		kinds[kind]++
	}
	if kinds["thread_cost"] == 0 {
		t.Fatalf("the completed stand-in reported no cost: %v", kinds)
	}
}

// TestStandInLaunchRefusesWhatItsCanaryWould keeps the stand-in honest about
// the one thing it shares with a real launch: a confinement that did not verify
// refuses before a process exists.
func TestStandInLaunchRefusesAnUnverifiedLaunch(t *testing.T) {
	invocation, receipt, store := prepareClaudeFixture(t)
	invocation.Path = "/bin/sh"
	invocation.Args = []string{"-c", "true"}
	invocation.Env = nil
	err := (Supervisor{Timeout: time.Minute}).Run(context.Background(),
		standInLaunch{verified: errStandInCanary}, invocation, receipt)
	if err == nil || !strings.Contains(err.Error(), "canary failed") {
		t.Fatalf("an unverified launch started a runtime: %v", err)
	}
	if entries, _, err := ReadReceipt(store, receipt.ID()); err == nil && len(entries) != 0 {
		t.Fatalf("a refused launch wrote a receipt stream: %s", ReceiptText(entries))
	}
}

var errStandInCanary = errStandIn("the stand-in canary reported nothing")

type errStandIn string

func (e errStandIn) Error() string { return string(e) }
