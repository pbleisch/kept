package ledger

import (
	"errors"
	"fmt"
	"path"
	"strings"
	"time"
)

// ThreadPath is the append-only receipt stream for one minted thread id. A
// thread, rather than a run, owns the path: runs may later inspect the same
// repair, while concurrent repairs never write the same receipt. The store
// files it on refs/kept/sessions, never on the ledger (docs/decision/0089).
func ThreadPath(id string) (string, error) {
	if id == "" {
		return "", fmt.Errorf("a thread receipt names no thread")
	}
	if strings.ContainsAny(id, "/\\") || strings.ContainsRune(id, '\x00') {
		return "", fmt.Errorf("thread %q cannot name a receipt path", id)
	}
	return path.Join("threads", id+".jsonl"), nil
}

// ThreadRef is where a thread's anchored commit is held against gc:
// refs/kept/threads/<id>.
//
// Under refs/kept/ because that is where kept already anchors what it writes
// and no worktree checks it out, and derived from the thread id rather than
// recorded beside it, because a name that is a pure function of an id it is
// already stored next to would be a second place for one answer to live.
//
// It is deliberately outside [FetchRefspec]'s refs/kept/ledger* pattern: an
// anchor is local work that has not landed, and a fetch that copied other
// people's unlanded repairs into this repository would be collecting what
// nobody reviewed.
func ThreadRef(id string) (string, error) {
	if id == "" {
		return "", fmt.Errorf("a thread anchor names no thread")
	}
	if strings.ContainsAny(id, "/\\") || strings.ContainsRune(id, '\x00') {
		return "", fmt.Errorf("thread %q cannot name an anchor ref", id)
	}
	return "refs/kept/threads/" + id, nil
}

// AnchoredThreads is every thread this repository holds an anchor for, as
// thread id to commit.
//
// The refs are the queue. There is no listing operation on [Store] and there
// should not be — three operations is the seam, and a fourth that enumerated
// would make the next implementation a filesystem — so what can be enumerated
// is what git already indexes, which is exactly the set of repairs whose work
// still exists.
//
// A repository with no anchors, and a workspace that is not a repository at
// all, both answer empty rather than failing: a manuscript has no worktree to
// repair in, so it has nothing to land.
func AnchoredThreads(repository string) (map[string]string, error) {
	out := map[string]string{}
	listed, err := gitValue(repository, "for-each-ref", "--format=%(refname) %(objectname)", "refs/kept/threads/")
	if err != nil {
		if isNotRepository(err) {
			return out, nil
		}
		return nil, err
	}
	for line := range strings.SplitSeq(listed, "\n") {
		name, commit, found := strings.Cut(strings.TrimSpace(line), " ")
		if !found {
			continue
		}
		id := strings.TrimPrefix(name, "refs/kept/threads/")
		if id == name || id == "" {
			continue
		}
		out[id] = commit
	}
	return out, nil
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
	envelope := Envelope{
		V:      Version,
		Kind:   kind,
		At:     at.UTC(),
		Writer: l.run.ID,
		Seq:    l.seq,
	}
	line, err := encode(envelope)
	if err != nil {
		return err
	}
	if err := l.declare(); err != nil {
		return err
	}
	if err := l.record(thread, where, kind, line, Mark{Writer: envelope.Writer, Seq: envelope.Seq}); err != nil {
		return err
	}
	if !l.threads[thread] {
		l.threads[thread] = true
		l.run.Threads++
	}
	return nil
}

// record puts one receipt line where this store keeps it.
//
// An entry that arrives at a runtime's pace goes to the thread's journal, which
// is durable when the write returns and fast enough to keep up, and reaches the
// ledger in a sealed segment. Everything else — what the parent itself observed
// and recorded — is appended to the receipt's own stream as it always was, for
// three reasons: it is not written at a runtime's pace, so it costs nothing to
// make durable immediately; the receipt exists in the ledger from its first
// lifecycle event, so a reader can see that a session started while it runs;
// and the index stays small, which is what keeps a seal inside the per-write
// allowance.
//
// That the two interleave is sound because a receipt is folded by event time,
// writer and sequence on the way back in, never by file order — the same
// property that lets two checkouts' appends union-merge. See docs/decision/0011
// and 0087.
func (l *Ledger) record(thread, where string, kind Kind, line []byte, at Mark) error {
	if journaledKind(kind) {
		journal, err := l.journalFor(thread)
		if err == nil {
			return l.journalEntry(journal, line, at)
		}
		if !errors.Is(err, ErrNoJournal) {
			return err
		}
	}
	if err := l.store.Append(where, line); err != nil {
		return err
	}
	// A lifecycle event is the parent's own record of the session's shape, and
	// its last one is the end of the stream. Sealing here is what makes a
	// finished receipt whole in the ledger, and what keeps a span from
	// outliving the session it is about.
	if kind == ThreadLifecycle {
		return l.SealThread(thread)
	}
	return nil
}

// journaledKind is whether an event arrives at a runtime's pace. These are the
// entries a session's own stream produces, as fast as the runtime writes them;
// the rest are the parent's and are written at the pace of a supervisor.
func journaledKind(kind Kind) bool {
	switch kind {
	case ThreadNative, ThreadTranscript, ThreadCost, ThreadTurn, ThreadModel, ThreadWindow, ThreadUndecodable:
		return true
	default:
		return false
	}
}

func threadKind(kind Kind) bool {
	switch kind {
	case ThreadPacket, ThreadNative, ThreadTranscript, ThreadCost, ThreadTurn, ThreadModel, ThreadWindow, ThreadLifecycle, ThreadAnchor, ThreadMeasurement, ThreadBrief, ThreadArgument, ThreadUndecodable, ThreadExecutable, ThreadStderr, ThreadSegment:
		return true
	default:
		return false
	}
}
