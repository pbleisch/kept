package ledger

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/pbleisch/kept/internal/run"
)

// SegmentThreshold is how much of a receipt a journal holds unsealed before the
// run files it into the ledger.
//
// 256 KiB because a replay of one real 3.8 MB receipt measured the smallest
// pack there: a smaller segment does not improve the per-write bound in any way
// that matters, and a larger one stops shrinking the pack.
//
// It is not what holds the per-write bound, and it was once written here as if
// it were. A seal appends one index entry of about 421 bytes to the receipt's
// own stream, and while that stream was one file every such append handed git
// every entry before it, so the cost of recording one line grew with the
// receipt and passed twice-its-bytes-plus-64-KiB at about 40 MiB of receipt
// whatever this threshold was. What holds the bound is that the stream is
// appended to in bounded parts — see [ReceiptPartBytes] and
// [RefStore.appendReceipt]. This threshold decides only how often a seal
// happens, which is why a test of the bound is free to seal as often as it
// likes. See docs/decision/0087.
const SegmentThreshold = 256 << 10

// ErrNoJournal is a store with nowhere to journal a receipt. It is not a
// failure: a run whose store answers this appends every receipt entry to the
// ledger as it produced it, which is what every run did before
// docs/decision/0087.
var ErrNoJournal = errors.New("this ledger store has no journal")

// Journaling is a store whose appends are too slow to make a receipt durable at
// a runtime's pace, and which can therefore say where a run may journal one
// instead.
//
// An optional interface rather than a fourth [Store] operation, because three
// operations is the seam docs/decision/0016 drew and a journal is not a ledger
// stream: it is local, it is never fetched, it holds no history, and it empties
// itself. A store that does not implement it is written to directly — [Files]
// is one append and one Sync at the receipt's own path, already durable when it
// returns, which is the property the journal exists to buy for a ref.
type Journaling interface {
	// JournalDir is the directory this store's runs journal receipts in. An
	// error wrapping [ErrNoJournal] means this store has nowhere to, which the
	// caller answers by appending to the ledger directly.
	JournalDir() (string, error)
}

// JournalPath is where one thread's receipt is journaled while its session
// runs. It is exported so that a test can read or rewind a journal the way a
// crash leaves one; nothing in kept reads a journal except a seal.
func JournalPath(store Store, thread string) (string, error) {
	if _, err := ThreadPath(thread); err != nil {
		return "", err
	}
	journaling, ok := store.(Journaling)
	if !ok {
		return "", fmt.Errorf("%T: %w", store, ErrNoJournal)
	}
	dir, err := journaling.JournalDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, thread+".jsonl"), nil
}

// Mark is one entry's place in the run that wrote it. Two marks delimit a
// segment: an index entry says which part of the receipt a sealed span holds
// without a reader having to open it.
type Mark struct {
	Writer run.ID `json:"writer"`
	Seq    uint64 `json:"seq"`
}

// Segment is one sealed span of a thread receipt, as the entry naming it says
// it: the artifact holding those lines verbatim, how many there are, and the
// first and last place in a run they came from.
//
// The shape lives here rather than in internal/thread because both ends need
// it and only one of them can own it: the ledger seals the span, and the thread
// package's read expands it. See docs/decision/0087.
type Segment struct {
	Artifact ArtifactRef `json:"artifact"`
	Entries  int         `json:"entries"`
	First    Mark        `json:"first"`
	Last     Mark        `json:"last"`
}

// SegmentWhy is what a segment entry must say, asked at the write and again on
// the way back in so the two ends cannot drift.
func SegmentWhy(segment Segment) string {
	switch {
	case segment.Artifact.Kind != ThreadSegmentArtifact:
		return "segment event names no thread segment artifact"
	case segment.Artifact.Size == 0:
		return "segment event names an empty span"
	case segment.Entries <= 0:
		return "segment event names no entries"
	case segment.First.Writer == "" || segment.Last.Writer == "":
		return "segment event does not say which run wrote the span"
	case segment.First.Seq == 0 || segment.Last.Seq == 0:
		return "segment event does not say where in its run the span begins and ends"
	}
	return ""
}

// segmentEntry is the index line a seal appends to the receipt's own stream. It
// is the thread package's entry shape in the one field the ledger writes: an
// inlined envelope, the thread, and one body. Written here because the seal
// happens here, and read there by the same JSON.
type segmentEntry struct {
	Envelope
	Thread  string   `json:"thread"`
	Segment *Segment `json:"segment"`
}

// journalLine is one line of a journal: one of an entry, an artifact's bytes or
// the record of a seal.
//
// Entry holds the receipt line as it would have been appended to the ledger,
// which is already the adapter's redacted entry — never a runtime's raw bytes,
// because a credential a runtime echoed would then sit on disk outside the
// ledger. Mark travels beside it, holding the place in this run that the ledger
// gave that entry: an encoder decides what of an envelope its own record shows,
// and a span's index may not depend on that choice. Artifact holds the bytes an
// entry is about to name, journaled before that entry, so a receipt never points
// at an object that was never written. Sealed records that everything before it
// has reached the ledger.
type journalLine struct {
	Entry    json.RawMessage  `json:"entry,omitempty"`
	Mark     *Mark            `json:"mark,omitempty"`
	Artifact *journalArtifact `json:"artifact,omitempty"`
	Sealed   *journalSealed   `json:"sealed,omitempty"`
}

type journalArtifact struct {
	Kind    ArtifactKind `json:"kind"`
	Content []byte       `json:"content"`
}

type journalSealed struct {
	Through int64  `json:"through"`
	Segment string `json:"segment,omitempty"`
}

// journal is one thread's local record while its session runs.
//
// The run holds an flock on it for as long as it is open, which is what says
// the writer is alive: flock lives on the open file description, so it releases
// when the process dies however it dies. The lock decides only liveness and
// removal — two runs that both seal one journal write the same segment address
// and one duplicate index entry, which the read drops.
type journal struct {
	thread string
	path   string

	mu     sync.Mutex
	file   *os.File
	sealed int64 // offset of the first byte no seal has filed
	held   int   // bytes written since that offset
}

// journalDirectory asks the store once where this run may journal. Called with
// journalsMu held.
func (l *Ledger) journalDirectory() (string, error) {
	if !l.journalAsked {
		l.journalAsked = true
		if journaling, ok := l.store.(Journaling); !ok {
			l.journalWhy = fmt.Errorf("%T: %w", l.store, ErrNoJournal)
		} else {
			l.journalAt, l.journalWhy = journaling.JournalDir()
		}
	}
	return l.journalAt, l.journalWhy
}

// journalFor opens this run's journal for a thread, or reports ErrNoJournal if
// this run cannot own one.
//
// Every way of not owning one is ErrNoJournal, including the ways that are a
// failure rather than an absence: nowhere to journal, a directory that cannot
// be made, a file that cannot be opened, a lock that cannot be asked for, a
// lock another run holds. A journal buys pace and nothing else — a receipt was
// written straight to the ledger before docs/decision/0087 and is written there
// still when there is no journal — so a journal that cannot be opened must cost
// the session its pace and not its lines. Returning the failure instead would
// fail the entry, and an entry that fails after the adapter accepted the line
// is a line kept read and lost, which is the clause this journal exists for.
//
// What is given up instead is the pace, and the demotion is quiet — nothing
// here has anywhere to report it. From that point the receipt is carried by the
// ledger's own writes, so a runtime that discards unread output at exit may
// discard some of it: a loss of output nobody had read, which is
// docs/decision/0084's other guarantee and not this one.
//
// A failure is not remembered, unlike the store's answer about where to
// journal, so a thread whose journal could not be opened for one entry is tried
// again for the next. The two paths interleave safely because a receipt is
// folded by event time, writer and sequence rather than by where it was written
// — see [Ledger.record].
func (l *Ledger) journalFor(thread string) (*journal, error) {
	l.journalsMu.Lock()
	defer l.journalsMu.Unlock()
	if j, ok := l.journals[thread]; ok {
		return j, nil
	}
	// Not ErrNoJournal: a thread id the ledger has no path for is a caller's
	// mistake, and it has nowhere to write the entry either. Asked before the
	// directory so that the mistake is reported whatever the store answers.
	if _, err := ThreadPath(thread); err != nil {
		return nil, err
	}
	dir, err := l.journalDirectory()
	if err != nil {
		if errors.Is(err, ErrNoJournal) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %w", ErrNoJournal, err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNoJournal, err)
	}
	path := filepath.Join(dir, thread+".jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNoJournal, err)
	}
	locked, err := tryLock(file)
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("%w: lock %s: %w", ErrNoJournal, path, err)
	}
	if !locked {
		// Another run is streaming this thread, or this platform has no file
		// lock at all. Either way this run does not own the journal and writes
		// to the ledger directly.
		file.Close()
		return nil, fmt.Errorf("%s: %w", path, ErrNoJournal)
	}
	// Whatever an earlier run left here has already been sealed and removed by
	// the recovery in declare, so the journal this run opens starts where the
	// file now ends.
	end, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("%w: %w", ErrNoJournal, err)
	}
	j := &journal{thread: thread, path: path, file: file, sealed: end}
	l.journals[thread] = j
	return j, nil
}

// write appends one line and returns once write(2) has. There is no fsync: what
// this guarantees is survival of a kill of kept, for which the bytes reaching
// the operating system is enough. A machine crash is a different question, and
// the answer for it is no worse than a ref update's — both are local writes
// whose sync to disk is core.fsync's to decide. See docs/decision/0087.
func (j *journal) write(line journalLine) error {
	encoded, err := json.Marshal(line)
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	if _, err := j.file.Write(encoded); err != nil {
		return err
	}
	j.held += len(encoded)
	return nil
}

// journalEntry records one receipt line, and where in this run it came from, and
// seals the span if it has grown past the threshold.
func (l *Ledger) journalEntry(j *journal, line []byte, at Mark) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if err := j.write(journalLine{Entry: json.RawMessage(line), Mark: &at}); err != nil {
		return err
	}
	if j.held < SegmentThreshold {
		return nil
	}
	return l.seal(j)
}

// journalArtifactBytes records the bytes of an artifact a receipt entry is
// about to name. It does not seal: the entry naming it follows immediately and
// a span is sealed on entry boundaries.
func (l *Ledger) journalArtifactBytes(j *journal, kind ArtifactKind, content []byte) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.write(journalLine{Artifact: &journalArtifact{Kind: kind, Content: content}})
}

// SealThread files everything this run has journaled for a thread and not yet
// filed. A thread with no journal has nothing to seal and reports no error: its
// entries are already in the ledger.
//
// It is called at the end of a session's stream and at every lifecycle event,
// which is what makes a finished receipt whole in the ledger. Nothing reads a
// receipt while its session runs, so a reader never seals — and ReadReceipt has
// no run identity to seal with.
func (l *Ledger) SealThread(thread string) error {
	l.journalsMu.Lock()
	j := l.journals[thread]
	l.journalsMu.Unlock()
	if j == nil {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	return l.seal(j)
}

// seal files the journal's unsealed span and records that it has. Called with
// j.mu held.
//
// The marker is written after the ledger has the span, so a run that dies
// between the two leaves a span the next writer files again — which costs one
// duplicate index entry naming the same address, and the read drops an entry it
// has already seen. The failure the order prevents is the other one: a marker
// for a span the ledger never received.
func (l *Ledger) seal(j *journal) error {
	through, filed, err := l.fileSpan(j.thread, j.path, j.sealed)
	if err != nil {
		return err
	}
	if filed != "" {
		if err := j.write(journalLine{Sealed: &journalSealed{Through: through, Segment: filed}}); err != nil {
			return err
		}
	}
	info, err := j.file.Stat()
	if err != nil {
		return err
	}
	j.sealed, j.held = info.Size(), 0
	return nil
}

// fileSpan puts everything one journal holds from an offset into the ledger: the
// artifacts the span names, the span itself as one content-addressed artifact,
// and one index entry in the receipt's own stream naming it.
//
// It returns how far it read and the segment's address, or the offset it was
// given and no address if it filed nothing. On failure it reports the offset
// unchanged, so the caller marks nothing and a later seal files the span again.
func (l *Ledger) fileSpan(thread, path string, from int64) (int64, string, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return from, "", nil
		}
		return from, "", err
	}
	defer file.Close()
	if _, err := file.Seek(from, io.SeekStart); err != nil {
		return from, "", err
	}

	at := from
	var span []byte
	var artifacts []journalArtifact
	var first, last Mark
	entries := 0
	reader := bufio.NewReader(file)
	for {
		raw, err := reader.ReadBytes('\n')
		if err != nil {
			// A line with no newline is a write that did not finish, which is
			// what a kill leaves. It is not a record, and nothing follows it.
			break
		}
		var line journalLine
		if json.Unmarshal(raw[:len(raw)-1], &line) != nil {
			break
		}
		at += int64(len(raw))
		switch {
		case line.Sealed != nil:
			// Everything up to here has reached the ledger, whether this run
			// filed it or a run that died did. One function therefore serves
			// both the live seal and the recovery below.
			span, artifacts, entries = nil, nil, 0
			first, last = Mark{}, Mark{}
		case line.Artifact != nil:
			artifacts = append(artifacts, *line.Artifact)
		case len(line.Entry) != 0:
			// The mark the ledger recorded beside the entry. An entry written
			// by something that is not this ledger has none, and the envelope
			// its own record shows is the last thing left to ask.
			mark := line.Mark
			if mark == nil {
				var envelope Envelope
				if json.Unmarshal(line.Entry, &envelope) == nil && envelope.Writer != "" {
					mark = &Mark{Writer: envelope.Writer, Seq: envelope.Seq}
				}
			}
			if mark != nil {
				if first.Writer == "" {
					first = *mark
				}
				last = *mark
			}
			entries++
			span = append(span, line.Entry...)
			span = append(span, '\n')
		}
	}
	if entries == 0 && len(artifacts) == 0 {
		return at, "", nil
	}

	// The artifacts first, and before the entries that name them, so a receipt
	// in the ledger never points at an object that was never written. Each is
	// addressed by its bytes, so a fragment a session produced twice is one
	// object and one put.
	put := map[string]bool{}
	for _, artifact := range artifacts {
		address := AddressArtifact(artifact.Content)
		if put[address] {
			continue
		}
		if _, err := l.PutArtifact(artifact.Kind, artifact.Content); err != nil {
			return from, "", err
		}
		put[address] = true
	}
	if entries == 0 {
		return at, "", nil
	}
	ref, err := l.PutArtifact(ThreadSegmentArtifact, span)
	if err != nil {
		return from, "", err
	}
	segment := Segment{Artifact: ref, Entries: entries, First: first, Last: last}
	if why := SegmentWhy(segment); why != "" {
		return from, "", fmt.Errorf("seal %s: %s", path, why)
	}
	err = l.AppendThread(thread, ThreadSegment, time.Now(), func(envelope Envelope) ([]byte, error) {
		return json.Marshal(segmentEntry{Envelope: envelope, Thread: thread, Segment: &segment})
	})
	if err != nil {
		return from, "", err
	}
	return at, ref.Address, nil
}

// recoverJournals seals what runs that died left journaled, before this run's
// first ledger write names it. That is the point at which a killed session's
// lines reach the ledger: the next run's first record.
//
// A journal whose lock this run cannot take belongs to a live writer and is
// left alone. A journal it can take is sealed and then removed while the lock
// is still held, so a live writer's journal is never unlinked under it.
//
// Nowhere to look is not a failure: a store with no journal, a store that
// cannot say where journals live, a directory no run has created or whose path
// runs through something that is not a directory. None of them is a journal
// this run can recover — and this is the first ledger write of every run, not
// only a session's, so failing here would refuse a ratification or an
// observation over a directory nobody was going to write to.
//
// A directory that is there and cannot be read is the other thing, and it was
// once answered as if it were the first. The difference is what is at stake. A
// journal is the only copy of a dead session's lines: while its session ran it
// was the durable one and the ledger held nothing, so this is the write that
// moves those lines into the ledger and nothing writes them again. A listing
// refused for permission, or failing on the device under it, is therefore a run
// that cannot tell whether there were lines to recover — and reporting no error
// there is how a session's whole output disappears with nobody told. See
// docs/decision/0018 and docs/decision/0084.
//
// What is a failure is likewise a journal this run did find and could not seal:
// those lines are a dead session's too, and they are lost if nothing says so.
func (l *Ledger) recoverJournals() error {
	l.journalsMu.Lock()
	dir, why := l.journalDirectory()
	mine := make(map[string]bool, len(l.journals))
	for _, j := range l.journals {
		mine[j.path] = true
	}
	l.journalsMu.Unlock()
	if why != nil {
		return nil // No journals to recover, whatever the reason.
	}

	listed, err := os.ReadDir(dir)
	if err != nil {
		if noJournalsThere(err) {
			return nil
		}
		return fmt.Errorf("list the journals in %s, which may hold the lines of a session that died: %w", dir, err)
	}
	var failed []error
	for _, entry := range listed {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		thread := strings.TrimSuffix(name, ".jsonl")
		if _, err := ThreadPath(thread); err != nil {
			continue
		}
		path := filepath.Join(dir, name)
		if mine[path] {
			continue
		}
		if err := l.recoverJournal(thread, path); err != nil {
			failed = append(failed, fmt.Errorf("seal the journal %s: %w", path, err))
		}
	}
	return errors.Join(failed...)
}

// noJournalsThere is whether a failure to look where journals live means there
// were none to look at.
//
// A path that does not exist, and a path running through something that is not
// a directory, are both *no run ever journaled here*: a journal is created
// under this directory by the run that writes it, so neither can be hiding one.
// Everything else — a directory this run may not read, a device that failed
// under it, descriptors it could not spare — is a directory that may hold a
// dead session's only copy and could not be asked.
func noJournalsThere(err error) bool {
	return errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOTDIR)
}

func (l *Ledger) recoverJournal(thread, path string) error {
	file, err := os.OpenFile(path, os.O_RDWR, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer file.Close()
	locked, err := tryLock(file)
	if err != nil {
		// Asking for the lock failed, which is not an answer about who owns the
		// journal. The file is there and this run cannot tell whether its writer
		// is alive, so it can neither seal it — two sealers are safe, but a seal
		// here would be followed by removing a live writer's journal under it —
		// nor leave the lines unaccounted for.
		return fmt.Errorf("ask for the lock on %s, so this run cannot tell whether its writer is alive: %w", path, err)
	}
	if !locked {
		// A live writer owns it, or this platform has no file lock. Neither is
		// this run's to recover.
		return nil
	}
	if _, _, err := l.fileSpan(thread, path, 0); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// closeJournals seals, removes and releases every journal this run opened. A
// run that reaches its own end leaves nothing for the next one to recover.
func (l *Ledger) closeJournals() error {
	l.journalsMu.Lock()
	open := make([]*journal, 0, len(l.journals))
	for _, j := range l.journals {
		open = append(open, j)
	}
	l.journals = map[string]*journal{}
	l.journalsMu.Unlock()
	sort.Slice(open, func(i, j int) bool { return open[i].path < open[j].path })

	var failed []error
	for _, j := range open {
		j.mu.Lock()
		err := l.seal(j)
		if err == nil {
			if removeErr := os.Remove(j.path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				err = removeErr
			}
		}
		if closeErr := j.file.Close(); err == nil {
			err = closeErr
		}
		j.mu.Unlock()
		if err != nil {
			failed = append(failed, fmt.Errorf("file the journal %s: %w", j.path, err))
		}
	}
	return errors.Join(failed...)
}

// PutThreadArtifact stores a payload one receipt entry is about to name.
//
// While a session runs that means journaling the bytes, before the entry, and
// returning the reference the entry will carry; the seal puts them. A store
// with no journal puts it now, exactly as [Ledger.PutArtifact] always has. The
// reference is the same either way, because an artifact is addressed by its
// bytes and nothing about where it was written is in that address.
func (l *Ledger) PutThreadArtifact(thread string, kind ArtifactKind, content []byte) (ArtifactRef, error) {
	ref := ArtifactRef{
		V:       ArtifactVersion,
		Kind:    kind,
		Address: AddressArtifact(content),
		Size:    len(content),
	}
	if _, err := ArtifactPath(kind, ref.Address); err != nil {
		return ArtifactRef{}, err
	}
	if err := l.declare(); err != nil {
		return ArtifactRef{}, err
	}
	j, err := l.journalFor(thread)
	if err != nil {
		if !errors.Is(err, ErrNoJournal) {
			return ArtifactRef{}, err
		}
		return l.PutArtifact(kind, content)
	}
	if err := l.journalArtifactBytes(j, kind, content); err != nil {
		return ArtifactRef{}, err
	}
	return ref, nil
}
