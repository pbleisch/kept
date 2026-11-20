package ledger

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pbleisch/kept/internal/lock"
	"github.com/pbleisch/kept/internal/run"
)

// SegmentSize is how much a thread's journal holds unsealed before kept seals
// it into the store. A replay of a recorded receipt measured the smallest pack
// at this size: a smaller segment does not bound a write any more usefully, and
// a larger one stops shrinking the pack. See docs/decision/0087.
const SegmentSize = 256 << 10

// JournalDir is where a repository's receipt journals live, below its Git
// common directory, so a run that died in one worktree is recovered by the next
// writer in any worktree. It is never in a working tree and never in a box.
const JournalDir = "kept/journal"

// Segment is what a [ThreadSegment] entry says about one sealed span: where
// the span is filed, and the first and last writer and sequence it holds.
type Segment struct {
	Artifact ArtifactRef `json:"artifact"`
	First    Position    `json:"first"`
	Last     Position    `json:"last"`
}

// Position is one receipt line's identity: the run that wrote it and that
// run's sequence number. A reader drops a line whose position it has already
// seen, which is what makes sealing one span twice harmless.
type Position struct {
	Writer run.ID `json:"writer"`
	Seq    uint64 `json:"seq"`
}

// segmentEntry is the index line a seal appends to a thread's receipt stream.
// Its JSON is a receipt entry's: an envelope, the thread, and one body.
type segmentEntry struct {
	Envelope
	Thread  string   `json:"thread"`
	Segment *Segment `json:"segment"`
}

// journaled is a payload a receipt entry names, journaled with its bytes ahead
// of that entry so that a seal can file it before the entry that names it. The
// key is one no receipt entry has, which is how a seal tells the two apart.
type journaled struct {
	Artifact *journaledArtifact `json:"journal_artifact"`
}

type journaledArtifact struct {
	Kind    ArtifactKind `json:"kind"`
	Content []byte       `json:"content"`
}

// journaling is a store whose receipts are journaled rather than appended a
// line at a time. Only a store on a Git ref is: a commit per line is what makes
// a receipt cost the square of its length, and a store kept in files appends a
// line for what a line costs.
type journaling interface {
	journalDir() (string, error)
}

func (s *RefStore) journalDir() (string, error) {
	common, err := lock.CommonDir(s.repository)
	if err != nil {
		return "", err
	}
	return filepath.Join(common, filepath.FromSlash(JournalDir)), nil
}

// journalRoot is where this ledger's store journals receipts, and false for a
// store that does not. It is asked of Git once per run rather than once per
// line: a line is journaled at the pace a runtime writes, and a Git invocation
// per line is the pace the journal exists to escape.
func (l *Ledger) journalRoot() (string, bool, error) {
	journals, ok := l.store.(journaling)
	if !ok {
		return "", false, nil
	}
	l.rootOnce.Do(func() { l.root, l.rootErr = journals.journalDir() })
	return l.root, true, l.rootErr
}

// journal is one thread's open journal, locked by this run for as long as it
// holds anything unsealed.
type journal struct {
	path     string
	file     *os.File
	unsealed int
}

// journalPath is where one thread's journal lives.
func journalPath(dir, thread string) (string, error) {
	if _, err := ThreadPath(thread); err != nil {
		return "", err
	}
	return filepath.Join(dir, thread+".jsonl"), nil
}

// openJournal opens a journal and takes its lock without waiting. It reports
// false when another live run holds the lock, or when the path stopped naming
// the file opened before the lock was taken — the journal was sealed and
// removed in between, so there is nothing of it left to take.
//
// The second check is what makes removal safe. A journal is unlinked only by
// the run holding its lock, so a run that opened the path just before that
// would otherwise lock an inode nobody will ever open again and append lines
// into it that no seal can find.
func openJournal(path string, create bool) (*os.File, bool, error) {
	for {
		flags := os.O_RDWR | os.O_APPEND
		if create {
			flags |= os.O_CREATE
		}
		file, err := os.OpenFile(path, flags, 0o600)
		if err != nil {
			return nil, false, err
		}
		taken, err := lock.TryFile(file)
		if err != nil || !taken {
			_ = file.Close()
			return nil, false, err
		}
		opened, err := file.Stat()
		if err != nil {
			_ = file.Close()
			return nil, false, err
		}
		named, err := os.Stat(path)
		if err == nil && os.SameFile(opened, named) {
			return file, true, nil
		}
		_ = file.Close()
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, false, err
		}
		if !create {
			return nil, false, nil
		}
	}
}

// journalFor returns this run's open journal for a thread, opening it on the
// first line. It refuses a thread another live run is journaling, because a
// line appended to a journal the other run then seals and removes would be in
// a file nobody reads.
func (l *Ledger) journalFor(dir, thread string) (*journal, error) {
	if j := l.journals[thread]; j != nil {
		return j, nil
	}
	path, err := journalPath(dir, thread)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create the receipt journal directory: %w", err)
	}
	file, taken, err := openJournal(path, true)
	if err != nil {
		return nil, fmt.Errorf("open the receipt journal for thread %s: %w", thread, err)
	}
	if !taken {
		return nil, fmt.Errorf("another run is writing thread %s's receipt journal at %s", thread, path)
	}
	j := &journal{path: path, file: file}
	l.journals[thread] = j
	return j, nil
}

// journalLine appends one line to a thread's journal and returns once write(2)
// has returned, which is the whole of what makes a line kept has read survive
// a kill of kept. It seals once the journal holds [SegmentSize] unsealed.
func (l *Ledger) journalLine(dir, thread string, line []byte) error {
	j, err := l.journalFor(dir, thread)
	if err != nil {
		return err
	}
	out := make([]byte, 0, len(line)+1)
	out = append(out, line...)
	out = append(out, '\n')
	if _, err := j.file.Write(out); err != nil {
		return fmt.Errorf("journal thread %s's receipt: %w", thread, err)
	}
	j.unsealed += len(out)
	if j.unsealed >= SegmentSize {
		return l.seal(thread)
	}
	return nil
}

// JournalArtifact stores a payload a receipt entry is about to name. Where the
// receipt is journaled, the bytes are journaled ahead of the entry and filed
// when the span is sealed, so a receipt never points at an object that was
// never written; anywhere else it is [Ledger.PutArtifact].
func (l *Ledger) JournalArtifact(thread string, kind ArtifactKind, content []byte) (ArtifactRef, error) {
	ref := ArtifactRef{V: ArtifactVersion, Kind: kind, Address: AddressArtifact(content), Size: len(content)}
	if _, err := ArtifactPath(kind, ref.Address); err != nil {
		return ArtifactRef{}, err
	}
	dir, ok, err := l.journalRoot()
	if !ok {
		return l.PutArtifact(kind, content)
	}
	if err != nil {
		return ArtifactRef{}, err
	}
	line, err := json.Marshal(journaled{Artifact: &journaledArtifact{Kind: kind, Content: content}})
	if err != nil {
		return ArtifactRef{}, err
	}
	l.journaling.Lock()
	defer l.journaling.Unlock()
	if err := l.declare(); err != nil {
		return ArtifactRef{}, err
	}
	if err := l.journalLine(dir, thread, line); err != nil {
		return ArtifactRef{}, err
	}
	return ref, nil
}

// SealThread seals whatever a thread's journal holds, so a reader of the store
// sees the receipt whole. It is a no-op for a thread with nothing unsealed.
func (l *Ledger) SealThread(thread string) error {
	l.journaling.Lock()
	defer l.journaling.Unlock()
	return l.seal(thread)
}

// sealAll seals every journal this run holds.
func (l *Ledger) sealAll() error {
	l.journaling.Lock()
	defer l.journaling.Unlock()
	var errs []error
	for thread := range l.journals {
		errs = append(errs, l.seal(thread))
	}
	return errors.Join(errs...)
}

// seal files one open journal's span and removes the journal while this run
// still holds its lock. A journal that could not be sealed stays, locked and
// whole, for the next attempt or the next run. It is called with journaling
// held.
func (l *Ledger) seal(thread string) error {
	j := l.journals[thread]
	if j == nil {
		return nil
	}
	content, err := io.ReadAll(io.NewSectionReader(j.file, 0, 1<<62))
	if err != nil {
		return fmt.Errorf("read thread %s's receipt journal: %w", thread, err)
	}
	if err := l.sealSpan(thread, content); err != nil {
		return err
	}
	delete(l.journals, thread)
	removeErr := os.Remove(j.path)
	closeErr := j.file.Close()
	if removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
		return fmt.Errorf("remove thread %s's sealed receipt journal: %w", thread, removeErr)
	}
	return closeErr
}

// sealSpan files what one journal held: every payload it carries, then the
// receipt lines as one segment, then one index entry naming the segment. In
// that order, so nothing the store holds ever names what it does not.
//
// A final line with no newline is a write a kill interrupted. Its writer never
// saw write(2) return, so it is not a line kept had read, and it is dropped.
// Anything else that is not a journaled payload goes into the segment as it
// stands, where the receipt's reader quarantines what it cannot read rather
// than this losing it.
func (l *Ledger) sealSpan(thread string, content []byte) error {
	where, err := ThreadPath(thread)
	if err != nil {
		return err
	}
	var segment []byte
	var first, last Position
	positioned := false
	for len(content) > 0 {
		at := bytes.IndexByte(content, '\n')
		if at < 0 {
			break
		}
		line := content[:at+1]
		content = content[at+1:]
		body := bytes.TrimSpace(line)
		if len(body) == 0 {
			continue
		}
		var payload journaled
		if json.Unmarshal(body, &payload) == nil && payload.Artifact != nil {
			if _, err := l.PutArtifact(payload.Artifact.Kind, payload.Artifact.Content); err != nil {
				return fmt.Errorf("seal thread %s's receipt: %w", thread, err)
			}
			continue
		}
		segment = append(segment, line...)
		var envelope Envelope
		if json.Unmarshal(body, &envelope) == nil && envelope.Writer != "" {
			if !positioned {
				first = Position{Writer: envelope.Writer, Seq: envelope.Seq}
				positioned = true
			}
			last = Position{Writer: envelope.Writer, Seq: envelope.Seq}
		}
	}
	if len(segment) == 0 {
		return nil
	}
	ref, err := l.PutArtifact(ThreadSegmentArtifact, segment)
	if err != nil {
		return fmt.Errorf("seal thread %s's receipt: %w", thread, err)
	}
	l.seq++
	line, err := json.Marshal(segmentEntry{
		Envelope: Envelope{V: Version, Kind: ThreadSegment, At: time.Now().UTC(), Writer: l.run.ID, Seq: l.seq},
		Thread:   thread,
		Segment:  &Segment{Artifact: ref, First: first, Last: last},
	})
	if err != nil {
		return err
	}
	if err := l.store.Append(where, line); err != nil {
		return fmt.Errorf("seal thread %s's receipt: %w", thread, err)
	}
	l.countThread(thread)
	return nil
}

// recoverJournals seals every journal a dead run left, and removes each while
// holding its lock. A journal whose lock another process holds belongs to a
// run that is alive, and is left to it.
//
// Nothing here excludes another run doing the same: two runs that both seal
// one journal write one segment address and one duplicate index entry, which
// the reader drops. The lock decides only whether a writer is alive, and guards
// only removal.
func (l *Ledger) recoverJournals() error {
	dir, ok, err := l.journalRoot()
	if !ok || err != nil {
		return err
	}
	names, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read the receipt journal directory: %w", err)
	}
	for _, name := range names {
		thread, ok := strings.CutSuffix(name.Name(), ".jsonl")
		if !ok || name.IsDir() || l.journals[thread] != nil {
			continue
		}
		path, err := journalPath(dir, thread)
		if err != nil {
			continue
		}
		file, taken, err := openJournal(path, false)
		if errors.Is(err, os.ErrNotExist) || (err == nil && !taken) {
			continue
		}
		if err != nil {
			return fmt.Errorf("open a receipt journal a dead run left at %s: %w", path, err)
		}
		content, err := io.ReadAll(io.NewSectionReader(file, 0, 1<<62))
		if err == nil {
			err = l.sealSpan(thread, content)
		}
		if err == nil {
			if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				err = removeErr
			}
		}
		_ = file.Close()
		if err != nil {
			return fmt.Errorf("seal a receipt journal a dead run left at %s: %w", path, err)
		}
	}
	return nil
}
