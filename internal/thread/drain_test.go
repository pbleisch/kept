//go:build !windows

package thread

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/run"
)

// slowStream is an adapter's stream at the pace of a ledger: every write waits.
type slowStream struct {
	mu     sync.Mutex
	got    bytes.Buffer
	delay  time.Duration
	failOn int
	writes int
	closes int
}

func (s *slowStream) Write(src []byte) (int, error) {
	time.Sleep(s.delay)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writes++
	if s.failOn != 0 && s.writes == s.failOn {
		return 0, errors.New("the receipt could not be written")
	}
	return s.got.Write(src)
}

func (s *slowStream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closes++
	return nil
}

// TestOutputIsStillReadAfterTheAdapterFailsAndCloseSaysSo is the half of never
// wedging the runtime that holds nothing: a receipt that cannot be written
// stops being fed, the runtime is still read to its end, and the failure is
// what the close reports rather than something the launch swallowed.
func TestOutputIsStillReadAfterTheAdapterFailsAndCloseSaysSo(t *testing.T) {
	dst := &slowStream{failOn: 2}
	s := &draining{dst: dst}
	for i := range 50 {
		if _, err := s.Write(fmt.Appendf(nil, "line %d\n", i)); err != nil {
			t.Fatalf("write %d after the adapter failed was refused: %v", i, err)
		}
	}
	err := s.Close()
	if err == nil || !strings.Contains(err.Error(), "could not be written") {
		t.Fatalf("close = %v, want the adapter's failure", err)
	}
	if dst.closes != 1 {
		t.Fatalf("the adapter's stream was closed %d times after it failed", dst.closes)
	}
}

// exitingRuntimeEvents is how many stream events the stand-in writes before its
// result, which is enough that a reader at the pace of a ledger commit per line
// would still be far behind when the stand-in stops waiting, and enough that the
// journal seals more than once while the stand-in writes.
const exitingRuntimeEvents = 1500

// TestARuntimeThatDiscardsUnreadOutputAtExitLosesNothing is the loss itself,
// reproduced without a model.
//
// The stand-in behaves the way Claude Code 2.1.272 was measured to at the end of
// a session: its writes never wait — output nobody has read is queued in the
// process — and when it is done it waits a bounded time for the queue to drain,
// then exits 0 and discards the rest, cutting whatever line it was in. Read at
// the pace of the receipt, that is a receipt with no result, no cost and a torn
// final line, which is threads 1y6kx8c48hb6mtjkpda76brtz0 and
// 7gz5c1wmwn5s4pv2rsqk7whs32 exactly. Read at the pace of the runtime, it is a
// whole session.
//
// The receipt is on a real ref, so every line the adapter accepts is journaled
// and every 256 KiB is sealed into the ref while the stand-in is still writing.
// That is the claim docs/decision/0087 says has to be shown once the spool is
// gone: an adapter journaling every entry keeps pace with a runtime that does
// not wait for it.
func TestARuntimeThatDiscardsUnreadOutputAtExitLosesNothing(t *testing.T) {
	store := ledger.NewRefStore(receiptRepository(t))
	receipt, err := NewReceipt(ledger.Open(store, run.Start("test", nil)), MintID())
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := receipt.ForAdapter(Claude, claudeAdapterVersion)
	if err != nil {
		t.Fatal(err)
	}
	packet, err := AssemblePacket(packetInput())
	if err != nil {
		t.Fatal(err)
	}
	invocation, err := prepareClaude(claudeConfig(t), packet, adapter)
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	invocation.Path = executable
	invocation.Args = []string{"-test.run=^TestExitingRuntimeHelper$"}
	invocation.Env = []string{exitingRuntimeHelper + "=1"}

	if err := (Supervisor{Timeout: time.Minute}).Run(context.Background(), standInLaunch{}, invocation, receipt); err != nil {
		t.Fatal(err)
	}
	entries, quarantined, err := ReadReceipt(store, receipt.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(quarantined) != 0 {
		t.Fatalf("quarantined: %#v", quarantined)
	}
	natives, results, costs := 0, 0, 0
	for _, entry := range entries {
		switch {
		case entry.Native != nil:
			natives++
			if entry.Native.Type == "result" {
				results++
			}
		case entry.Cost != nil:
			costs++
		case entry.Undecodable != nil:
			t.Fatalf("the stream ended inside an item, so the runtime discarded output nobody read: %#v", entry.Undecodable)
		}
	}
	if natives != exitingRuntimeEvents+1 || results != 1 || costs != 1 {
		t.Fatalf("the receipt holds %d native events, %d results and %d costs; the runtime wrote %d events, a result and its cost",
			natives, results, costs, exitingRuntimeEvents+1)
	}
}

const exitingRuntimeHelper = "KEPT_THREAD_EXITING_RUNTIME"

// TestExitingRuntimeHelper is the stand-in's body, run in a child process. It is
// not a test of anything on its own and does nothing unless launched as one.
func TestExitingRuntimeHelper(t *testing.T) {
	if os.Getenv(exitingRuntimeHelper) != "1" {
		t.Skip("run as a child of TestARuntimeThatDiscardsUnreadOutputAtExitLosesNothing")
	}
	if err := syscall.SetNonblock(1, true); err != nil {
		os.Exit(3)
	}
	var queue []byte
	flush := func() {
		for len(queue) > 0 {
			n, err := syscall.Write(1, queue)
			if n > 0 {
				queue = queue[n:]
			}
			if err != nil {
				return
			}
		}
	}
	for i := range exitingRuntimeEvents {
		queue = append(queue, `{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"`...)
		queue = append(queue, strconv.Itoa(i)...)
		queue = append(queue, strings.Repeat("x", 120)...)
		queue = append(queue, "\"}}}\n"...)
		flush()
	}
	queue = append(queue, `{"type":"result","subtype":"success","num_turns":81,"total_cost_usd":1.25}`+"\n"...)
	// The session is over. Wait a bounded time for what is queued to drain, and
	// then go, whatever is left.
	deadline := time.Now().Add(time.Second)
	for len(queue) > 0 && time.Now().Before(deadline) {
		flush()
		time.Sleep(time.Millisecond)
	}
	os.Exit(0)
}
