//go:build !windows

package thread

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

func (s *slowStream) received() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.got.Bytes()...)
}

// TestTheRelayHandsOnEveryByteBeforeTheWriteReturns is the property the receipt's
// durability rests on: the moment kept accepts a line from a runtime is the
// moment the adapter has it, so a kill of kept between two writes cannot lose
// one that was accepted.
//
// Its predecessor accepted into a queue and asserted only that every byte
// arrived in order eventually, because retaining a line then cost a ledger
// commit and the queue was what let reading keep pace with the runtime. The
// adapter journals now, so the queue is gone and the stronger property is
// available: this asks after every single write, not at the end.
func TestTheRelayHandsOnEveryByteBeforeTheWriteReturns(t *testing.T) {
	dst := &slowStream{delay: time.Millisecond}
	r := newRelay(dst)
	var want bytes.Buffer
	for i := range 100 {
		chunk := fmt.Appendf(nil, "chunk %03d\n", i)
		want.Write(chunk)
		if _, err := r.Write(chunk); err != nil {
			t.Fatal(err)
		}
		if got := dst.received(); !bytes.Equal(got, want.Bytes()) {
			t.Fatalf("after write %d the adapter holds %q, want every byte accepted so far in the order written", i+1, got)
		}
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if dst.closes != 1 {
		t.Fatalf("the adapter's stream was closed %d times", dst.closes)
	}
	if _, err := r.Write([]byte("late")); err == nil {
		t.Fatal("a write after close was accepted")
	}
}

// TestTheRelayKeepsReadingAfterTheAdapterFailsAndSaysSoAtClose is the other
// half of never wedging the runtime: a receipt that cannot be written stops
// being fed, the runtime is still read to its end, and the failure is what the
// close reports rather than something the launch swallowed.
func TestTheRelayKeepsReadingAfterTheAdapterFailsAndSaysSoAtClose(t *testing.T) {
	dst := &slowStream{failOn: 2}
	r := newRelay(dst)
	for i := range 50 {
		if _, err := r.Write(fmt.Appendf(nil, "line %d\n", i)); err != nil {
			t.Fatalf("write %d after the adapter failed was refused: %v", i, err)
		}
	}
	err := r.Close()
	if err == nil || !strings.Contains(err.Error(), "could not be written") {
		t.Fatalf("close = %v, want the adapter's failure", err)
	}
	if dst.closes != 1 {
		t.Fatalf("the adapter's stream was closed %d times after it failed", dst.closes)
	}
}

// TestTheRelayHoldsNothingSoBackPressureReachesTheRuntime is where it waits.
//
// Its predecessor held 64 MiB and applied back-pressure past that bound; this
// holds nothing at all, which is the same guarantee at a bound of zero and a
// stronger one to assert: a runtime that outruns what kept can retain waits
// immediately rather than after a queue has filled, and what kept holds is
// bounded by the disk the journal is on rather than by memory.
func TestTheRelayHoldsNothingSoBackPressureReachesTheRuntime(t *testing.T) {
	release := make(chan struct{})
	dst := &gatedStream{release: release}
	r := newRelay(dst)

	blocked := make(chan struct{})
	go func() {
		_, _ = r.Write(bytes.Repeat([]byte("x"), 32))
		close(blocked)
	}()
	dst.waitEntered()
	select {
	case <-blocked:
		t.Fatal("a write returned before the adapter had accepted the bytes")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	select {
	case <-blocked:
	case <-time.After(5 * time.Second):
		t.Fatal("a write held at the adapter never proceeded once it caught up")
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
}

type gatedStream struct {
	release chan struct{}
	entered sync.Once
	in      chan struct{}
	mu      sync.Mutex
}

func (g *gatedStream) Write(src []byte) (int, error) {
	g.mu.Lock()
	if g.in == nil {
		g.in = make(chan struct{})
	}
	in := g.in
	g.mu.Unlock()
	g.entered.Do(func() { close(in) })
	<-g.release
	return len(src), nil
}

func (g *gatedStream) waitEntered() {
	for {
		g.mu.Lock()
		in := g.in
		g.mu.Unlock()
		if in != nil {
			<-in
			return
		}
		time.Sleep(time.Millisecond)
	}
}

func (*gatedStream) Close() error { return nil }

// slowStore is a ledger at the pace of a real one: the receipt's stream lives on
// a ref, and every append is several Git invocations. Measured on the operator's
// machine over thread 1y6kx8c48hb6mtjkpda76brtz0, the median gap between two
// appends was 122 ms.
//
// A journal path makes it the store a session is actually written through on the
// platform kept confines one on: a ref store is slow to append and says where
// to journal, which is the pair docs/decision/0087 rests on. An empty one makes
// it the other configuration, which also ships — a store with no journal
// (ledger.Files, or any store off unix, where there is no flock and so no run
// owns one) reports ledger.ErrNoJournal and takes every receipt entry straight
// into the ledger. Both are measured, and they do not guarantee the same thing:
// see TestARuntimeThatDiscardsUnreadOutputAtExitLosesNothing for the first and
// TestASlowLedgerWithNoJournalHoldsEveryLineItAccepted for the second.
type slowStore struct {
	ledger.Store
	delay   time.Duration
	journal string
}

func (s slowStore) Append(stream string, line []byte) error {
	if strings.HasPrefix(stream, "ledger/threads/") || strings.Contains(stream, "threads/") {
		time.Sleep(s.delay)
	}
	return s.Store.Append(stream, line)
}

func (s slowStore) JournalDir() (string, error) {
	if s.journal == "" {
		return "", fmt.Errorf("%T: %w", s, ledger.ErrNoJournal)
	}
	return s.journal, nil
}

// exitingRuntimeEvents is how many stream events the stand-in writes before its
// result, which is enough that a reader at the pace of the receipt is still far
// behind when the stand-in stops waiting.
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
// The fixture journals, and that is what makes this hold rather than a detail of
// how it is set up: an entry is durable in a local append instead of a ref
// update, so the reader keeps up and the queue the runtime discards is empty
// when it goes. Before docs/decision/0087 the same guarantee rested on a 64 MiB
// in-process spool, which 0087 retired because the journal replaced it — so
// where there is no journal there is no longer a spool either, and this
// particular guarantee is not available. That configuration is measured for
// what it does guarantee by
// TestASlowLedgerWithNoJournalHoldsEveryLineItAccepted.
func TestARuntimeThatDiscardsUnreadOutputAtExitLosesNothing(t *testing.T) {
	kept := t.TempDir()
	store := slowStore{Store: ledger.NewFiles(kept), delay: time.Millisecond, journal: filepath.Join(kept, "journal")}
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

// noJournalFloor is how many of the stand-in's lines the receipt must hold for
// the no-journal measurement to be worth anything. Far below what the
// configuration actually manages — the pipe buffer alone carries a few hundred
// events, and the reader drains some while the runtime is still writing — and
// well above a receipt that recorded a handful and passed on a technicality.
const noJournalFloor = 100

// TestASlowLedgerWithNoJournalHoldsEveryLineItAccepted is the same slow ledger
// with nowhere to journal, which is a configuration that still ships:
// ledger.Files is one, ledger.ForWorkspace answers one for a workspace that is
// not a repository, and a ref store is one off unix, where nothing can take an
// flock and so no run owns a journal. Such a run appends every receipt entry to
// the ledger as it is produced, which is what every run did before
// docs/decision/0087.
//
// What that guarantees, and what this measures, is the clause this promise is
// about: a line kept has read is in the ledger, in order, by the time the write
// that accepted it returns. It needs no journal — there, the append *is* the
// durability — so it is exactly as true here as on the journaling path, and a
// receipt read back holds a whole prefix of the session with nothing missing
// from the middle.
//
// What it does not guarantee is the other half of docs/decision/0084: the
// runtime's queue does not stay empty, so a runtime that discards unread output
// at exit discards some of it. Said plainly rather than left to be inferred,
// because 0087 retired the spool that used to keep that queue empty everywhere:
// off a journal the no-discard guarantee is gone with it, and a reader who was
// told parity had been kept would be told something untrue.
//
// So this asserts a prefix and not a total, and measures it against what the
// adapter took rather than against what the runtime wrote. How much is lost is a
// race between a 1 ms append and the stand-in's drain deadline; a test that
// asserted the loss would be asserting the race, and one that asserted no loss
// would be asserting the guarantee this path does not have.
func TestASlowLedgerWithNoJournalHoldsEveryLineItAccepted(t *testing.T) {
	store := slowStore{Store: ledger.NewFiles(t.TempDir()), delay: time.Millisecond}
	if _, err := ledger.JournalPath(store, "thread-a"); !errors.Is(err, ledger.ErrNoJournal) {
		t.Fatalf("the fixture's store answers %v for where to journal, so this is not the no-journal configuration", err)
	}
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
	accepted := &countingStream{to: invocation.Output}
	invocation.Output = accepted

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
	var natives []*NativeEvent
	torn := 0
	for _, entry := range entries {
		switch {
		case entry.Native != nil:
			natives = append(natives, entry.Native)
		case entry.Undecodable != nil:
			torn++
		}
	}
	// One torn item is the stream ending inside one, which is the runtime's
	// discard and is this path's to have. Two would mean something in the
	// middle arrived broken.
	if torn > 1 {
		t.Fatalf("the receipt holds %d undecodable items; at most the one the runtime was cut off inside can be", torn)
	}
	lines := accepted.lines()
	if lines < noJournalFloor {
		t.Fatalf("the adapter accepted %d complete lines, fewer than %d: the fixture did not get far enough into a session for this to measure anything", lines, noJournalFloor)
	}
	if len(natives) < lines {
		t.Fatalf("the adapter accepted %d complete lines and the ledger holds %d of them; with no journal every accepted line is appended before the write accepting it returns", lines, len(natives))
	}
	// And they are the session's own lines, in its own order. The stand-in
	// numbers every event it writes, which is what makes a line dropped out of
	// the middle visible: every event is otherwise the same shape, so a count
	// alone cannot tell a whole prefix from one with a hole in it.
	for i, native := range natives {
		if i == exitingRuntimeEvents {
			if native.Type != "result" {
				t.Fatalf("native event %d is %q, want the session's result", i+1, native.Type)
			}
			continue
		}
		var event struct {
			Event struct {
				Delta struct {
					PartialJSON string `json:"partial_json"`
				} `json:"delta"`
			} `json:"event"`
		}
		if err := json.Unmarshal(native.Payload, &event); err != nil {
			t.Fatalf("native event %d does not decode: %v", i+1, err)
		}
		if want := strconv.Itoa(i) + strings.Repeat("x", 120); event.Event.Delta.PartialJSON != want {
			t.Fatalf("native event %d carries %q, want the %dth event the runtime wrote: a line is missing or out of order",
				i+1, event.Event.Delta.PartialJSON, i)
		}
	}
}

// countingStream counts the complete lines an adapter's stream took from it.
//
// Counted from what the write consumed rather than from what was handed to it,
// because that is the adapter's own account of which lines it recorded: a write
// that failed part way reports the bytes it got through, and the lines in those
// bytes are the ones that were accepted. A final item with no newline is not
// counted, which is right — it is what the runtime was cut off inside, and
// nothing accepted it.
type countingStream struct {
	to EventStream
	mu sync.Mutex
	n  int
}

func (c *countingStream) Write(src []byte) (int, error) {
	n, err := c.to.Write(src)
	if n > 0 {
		c.mu.Lock()
		c.n += bytes.Count(src[:min(n, len(src))], []byte{'\n'})
		c.mu.Unlock()
	}
	return n, err
}

func (c *countingStream) Close() error { return c.to.Close() }

func (c *countingStream) lines() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
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
