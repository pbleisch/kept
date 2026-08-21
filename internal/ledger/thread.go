package ledger

import (
	"fmt"
	"path"
	"strings"
	"time"
)

// ThreadPath is the append-only receipt stream for one minted thread id. A
// thread, rather than a run, owns the path: runs may later inspect the same
// repair, while concurrent repairs never write the same receipt.
func ThreadPath(id string) (string, error) {
	if id == "" {
		return "", fmt.Errorf("a thread receipt names no thread")
	}
	if strings.ContainsAny(id, "/\\") || strings.ContainsRune(id, '\x00') {
		return "", fmt.Errorf("thread %q cannot name a receipt path", id)
	}
	return path.Join("threads", id+".jsonl"), nil
}

// AppendThread is the only ledger operation a repair receipt needs. The thread
// package supplies the versioned event shape; the ledger supplies the global
// run sequence and declares the writer before any receipt names it. Keeping
// those responsibilities together preserves the record invariant without
// making the ledger depend on runtime vocabulary.
func (l *Ledger) AppendThread(thread string, kind Kind, at time.Time, encode func(Envelope) ([]byte, error)) error {
	where, err := ThreadPath(thread)
	if err != nil {
		return err
	}
	if !threadKind(kind) {
		return fmt.Errorf("%q is not a thread receipt event", kind)
	}
	if at.IsZero() {
		return fmt.Errorf("a thread receipt has no event time")
	}

	l.seq++
	line, err := encode(Envelope{
		V:      Version,
		Kind:   kind,
		At:     at.UTC(),
		Writer: l.run.ID,
		Seq:    l.seq,
	})
	if err != nil {
		return err
	}
	if err := l.declare(); err != nil {
		return err
	}
	if err := l.store.Append(where, line); err != nil {
		return err
	}
	if !l.threads[thread] {
		l.threads[thread] = true
		l.run.Threads++
	}
	return nil
}

func threadKind(kind Kind) bool {
	switch kind {
	case ThreadPacket, ThreadNative, ThreadTranscript, ThreadCost, ThreadTurn, ThreadLifecycle:
		return true
	default:
		return false
	}
}
