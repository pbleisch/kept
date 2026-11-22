package thread

import (
	"errors"
	"fmt"
	"sync"
)

// errRelayClosed is a write after the stream was closed.
var errRelayClosed = errors.New("write runtime output after its stream was closed")

// relay sits between the confinement launch and an adapter's stream: it hands
// on every byte it accepts, in order, before the write that brought it returns.
//
// It holds nothing. Its predecessor did — a 64 MiB queue, because of what a
// runtime does with output nobody has read. Measured against Claude Code
// 2.1.272: it queues its stream without limit while a session runs, and when
// the session ends it waits for the queue to drain for at most max(2s, queued
// bytes ÷ 256 KiB/s), capped at thirty seconds, then exits 0 and discards the
// rest — mid-line. An adapter that appended each native line to the ledger as
// it read it retained two to eleven lines a second against a runtime streaming
// hundreds, so a long session's receipt fell minutes behind it and lost its
// whole tail: the last turns, the result, the cost. A reader that held output
// in memory kept the runtime's queue empty, and the runtime's exit then waited
// for nothing. See docs/decision/0084.
//
// What the queue cost was the other guarantee. Lines it held were in memory
// only, so a kill of kept lost every line kept had read and not yet written —
// which is most of them, at that pace. An adapter now journals each entry at
// the moment it accepts it, one append to a local file, which is both durable
// when it returns and fast enough to keep up; so the pace 0084 needed no longer
// needs a queue, and what this relay is for is the three disciplines around the
// stream rather than the distance between two paces. Back-pressure now reaches
// the runtime when the disk cannot keep up, and the bound on what kept holds is
// the disk rather than 64 MiB of memory. See docs/decision/0087.
//
// The disciplines: order is kept and a write returns only once the adapter has
// the bytes, so nothing a runtime wrote is recorded after the parent's own
// record of its exit; an adapter that fails is not fed again, while the runtime
// is still read to the end of its stream, because a runtime whose output nobody
// reads is one that waits or discards; and the adapter's stream is closed
// exactly once, by whichever of Close or abandon comes first.
type relay struct {
	dst EventStream

	mu     sync.Mutex
	closed bool
	failed error

	once   sync.Once
	result error
}

func newRelay(dst EventStream) *relay { return &relay{dst: dst} }

// Write hands src to the adapter and returns once it has been accepted.
//
// It never returns the adapter's error. The launch stops reading at the first
// error it is handed, and a runtime whose output nobody reads is a runtime that
// waits or discards — so once the adapter has failed, what arrives is accepted
// and dropped, the runtime runs to its own end, and Close reports the failure.
func (r *relay) Write(src []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return 0, errRelayClosed
	}
	if r.failed != nil || len(src) == 0 {
		return len(src), nil
	}
	if _, err := r.dst.Write(src); err != nil {
		r.failed = err
	}
	return len(src), nil
}

// Close closes the adapter's stream and reports the first failure of either the
// stream or a write. It is idempotent, and it waits for a write in flight, so
// the adapter sees no write after its close.
func (r *relay) Close() error {
	r.once.Do(func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.closed = true
		closeErr := r.dst.Close()
		switch {
		case r.failed != nil && closeErr != nil:
			r.result = fmt.Errorf("%w; close: %v", r.failed, closeErr)
		case r.failed != nil:
			r.result = r.failed
		default:
			r.result = closeErr
		}
	})
	return r.result
}

// abandon stops the relay without closing the adapter's stream, for a launch
// that never started: a stream that was never written has nothing to retain,
// and closing an adapter's stream is where it reports what a session left.
func (r *relay) abandon() {
	r.once.Do(func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.closed = true
	})
}
