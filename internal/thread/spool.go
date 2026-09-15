package thread

import (
	"errors"
	"fmt"
	"sync"
)

// spoolLimit is how much of a runtime's output the spool holds before it stops
// accepting more and lets the runtime wait.
//
// It is a bound on memory rather than a pace. A receipt line is one ledger
// append, so retaining is always slower than a runtime writes; the whole of a
// long real session's native stream is a few megabytes, and this is several
// sessions' worth. Past it the spool applies back-pressure, which is exactly
// what every stream did before it existed — the difference is that nothing
// short of a runaway runtime reaches it.
const spoolLimit = 64 << 20

// errSpoolClosed is a write after the stream was closed.
var errSpoolClosed = errors.New("write runtime output after its stream was closed")

// spool sits between the confinement launch and an adapter's stream: it
// accepts a runtime's output as fast as the runtime writes it, and hands it to
// the adapter at the pace the receipt can be written.
//
// It exists because of what a runtime does with output nobody has read.
// Measured against Claude Code 2.1.272: it queues its stream without limit
// while a session runs, and when the session ends it waits for the queue to
// drain for at most max(2s, queued bytes ÷ 256 KiB/s), capped at thirty
// seconds, then exits 0 and discards the rest — mid-line. An adapter that
// appends each native line to the ledger as it reads it retains two to eleven
// lines a second against a runtime streaming hundreds, so a long session's
// receipt fell minutes behind it and lost its whole tail: the last turns, the
// result, the cost. A reader that holds output in memory keeps the runtime's
// queue empty, and the runtime's exit then waits for nothing. See
// docs/decision/0084.
//
// Order is kept: bytes reach the adapter in the order they were written, and
// Close returns only once every byte accepted has been handed on, so nothing a
// runtime wrote is recorded after the parent's own record of its exit.
type spool struct {
	dst   EventStream
	limit int

	mu      sync.Mutex
	arrived *sync.Cond // a chunk was queued, or the spool was closed
	taken   *sync.Cond // the queue shrank
	queue   [][]byte
	held    int
	closed  bool
	failed  error
	done    chan struct{}
	result  error
	once    sync.Once
}

func newSpool(dst EventStream) *spool {
	s := &spool{dst: dst, limit: spoolLimit, done: make(chan struct{})}
	s.arrived = sync.NewCond(&s.mu)
	s.taken = sync.NewCond(&s.mu)
	go s.run()
	return s
}

// Write accepts a copy of src and returns at once, unless the spool already
// holds spoolLimit bytes.
//
// It never returns the adapter's error. The launch stops reading at the first
// error it is handed, and a runtime whose output nobody reads is a runtime that
// waits or discards — so once the adapter has failed, what arrives is accepted
// and dropped, the runtime runs to its own end, and Close reports the failure.
func (s *spool) Write(src []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 0, errSpoolClosed
	}
	for s.held >= s.limit && s.failed == nil && !s.closed {
		s.taken.Wait()
	}
	if s.closed {
		return 0, errSpoolClosed
	}
	if s.failed != nil || len(src) == 0 {
		return len(src), nil
	}
	s.queue = append(s.queue, append([]byte(nil), src...))
	s.held += len(src)
	s.arrived.Signal()
	return len(src), nil
}

func (s *spool) run() {
	defer close(s.done)
	for {
		s.mu.Lock()
		for len(s.queue) == 0 && !s.closed {
			s.arrived.Wait()
		}
		if len(s.queue) == 0 {
			s.mu.Unlock()
			return
		}
		chunk := s.queue[0]
		s.queue[0] = nil
		s.queue = s.queue[1:]
		s.held -= len(chunk)
		s.taken.Broadcast()
		s.mu.Unlock()

		if _, err := s.dst.Write(chunk); err != nil {
			s.mu.Lock()
			s.failed = err
			s.queue, s.held = nil, 0
			s.taken.Broadcast()
			s.mu.Unlock()
		}
	}
}

// Close hands on everything accepted, closes the adapter's stream, and reports
// the first failure of either. It is idempotent.
func (s *spool) Close() error {
	s.once.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.arrived.Broadcast()
		s.taken.Broadcast()
		s.mu.Unlock()
		<-s.done

		closeErr := s.dst.Close()
		switch {
		case s.failed != nil && closeErr != nil:
			s.result = fmt.Errorf("%w; close: %v", s.failed, closeErr)
		case s.failed != nil:
			s.result = s.failed
		default:
			s.result = closeErr
		}
	})
	return s.result
}

// abandon stops the spool without closing the adapter's stream, for a launch
// that never started: a stream that was never written has nothing to retain,
// and closing an adapter's stream is where it reports what a session left.
func (s *spool) abandon() {
	s.once.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.queue, s.held = nil, 0
		s.arrived.Broadcast()
		s.taken.Broadcast()
		s.mu.Unlock()
		<-s.done
	})
}
