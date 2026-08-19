package ledger

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/pbleisch/kept/internal/run"
)

// Ledger is one run's handle on the record. It appends, and it never revises:
// there is no update, no delete and no invalidate here, and there is nowhere
// else for one to be added, because standing is derived by comparing a hash
// rather than maintained by marking anything stale. See docs/decision/0001.
type Ledger struct {
	store Store
	run   *run.Record

	seq      uint64
	declared bool // whether this run's diagnostics have been written
}

// Open returns the ledger as this run sees it. The run's diagnostics are not
// written yet: a run that reads and records nothing has named nothing, so there
// is nothing for a writer field to resolve to and no reason to leave a file
// behind. They are written when this run first appends.
func Open(store Store, rec *run.Record) *Ledger {
	return &Ledger{store: store, run: rec}
}

// Ratify grants standing to one content version, and is the only place a
// ratification is constructed. Keeping that to one function is what makes "no
// rule, no agent and no inference mints standing" a property of the code rather
// than a habit — there is one call site to audit, and it is reached only from
// the verb a person types.
func (l *Ledger) Ratify(artifact, hash string, normVersion int, by, note string) (Act, error) {
	return l.act(Ratified, artifact, hash, normVersion, by, note)
}

// Release retires standing. It records the content version that was in force
// when the operator retired it, which is what they were looking at — but the
// register reads a release as retiring the artifact rather than a wording, so a
// released promise does not come back by being edited.
func (l *Ledger) Release(artifact, hash string, normVersion int, by, note string) (Act, error) {
	return l.act(Released, artifact, hash, normVersion, by, note)
}

func (l *Ledger) act(kind Kind, artifact, hash string, normVersion int, by, note string) (Act, error) {
	l.seq++
	d := Act{
		Envelope: Envelope{
			V:      Version,
			Kind:   kind,
			At:     time.Now().UTC(),
			Writer: l.run.ID,
			Seq:    l.seq,
		},
		Artifact: artifact,
		Hash:     hash,
		Norm:     normVersion,
		By:       by,
		Note:     note,
	}

	line, err := json.Marshal(d)
	if err != nil {
		return Act{}, err
	}
	if err := l.declare(); err != nil {
		return Act{}, err
	}
	if err := l.store.Append(Acts, line); err != nil {
		return Act{}, err
	}
	l.run.Acts++
	return d, nil
}

// declare writes this run's diagnostics, once, before the first record naming
// it exists. Written before rather than after so that a crash between the two
// leaves a run with no records — which says nothing untrue — rather than a
// record whose writer resolves to nothing, which would read as diagnostics that
// had aged out.
//
// This is not a record of an act and does not contradict docs/decision/0018: it
// asserts that a run started, which it did. Ended stays zero until Close, and
// that is the ordinary evidence of an unclean stop.
func (l *Ledger) declare() error {
	if l.declared {
		return nil
	}
	if err := l.putRun(); err != nil {
		return err
	}
	l.declared = true
	return nil
}

// Close records how the run ended. A run that appended nothing wrote no
// diagnostics and leaves none behind.
func (l *Ledger) Close(exit string) error {
	if !l.declared {
		return nil
	}
	l.run.Ended = time.Now().UTC()
	l.run.Exit = exit
	return l.putRun()
}

// RunPath is where a run's diagnostics live, one file per run. Keyed by the
// run's minted id, so two concurrent runs never choose the same path and
// nothing has to arbitrate. See docs/decision/0012.
func RunPath(id run.ID) string { return path.Join("runs", string(id)+".json") }

func (l *Ledger) putRun() error {
	body, err := json.MarshalIndent(l.run, "", "  ")
	if err != nil {
		return err
	}
	return l.store.Put(RunPath(l.run.ID), append(body, '\n'))
}

// ReadActs returns the act stream, folded into the order the acts
// happened in, alongside every line that could not be read.
//
// The fold is by event time, then by the run that wrote it, then by that run's
// seq — never by file order. Union merge interleaves two checkouts' appends
// arbitrarily, so file order says nothing; and within one run two records can
// share an event time, which is what seq is for. See docs/decision/0011 and
// 0013.
func ReadActs(store Store) ([]Act, []Quarantined, error) {
	src, err := store.Read(Acts)
	if err != nil {
		if errors.Is(err, ErrNoStream) {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	records, quarantined := parse(Acts, src, actWhy)
	fold(records)
	return records, quarantined, nil
}

// Empty reports whether this store holds no act stream at all, which is
// different from holding an empty one. A clone made without the ledger's fetch
// refspec looks exactly like a corpus nobody has ever ratified, and the only
// way to keep that from being read as a fresh start is to be able to tell the
// two apart. See docs/decision/0016.
func Empty(store Store) bool {
	_, err := store.Read(Acts)
	return errors.Is(err, ErrNoStream)
}

// fold puts a stream into the order the acts happened in: event time, then the
// run that wrote it, then that run's seq.
//
// It is generic over the record because every stream folds the same way and a
// stream that folded differently would be a bug nobody could see. Never file
// order: union merge interleaves two checkouts' appends arbitrarily, and within
// one run two records can share an event time, which is what seq is for. See
// docs/decision/0011 and 0013.
func fold[T record](records []T) {
	sort.SliceStable(records, func(i, j int) bool {
		a, b := records[i].envelope(), records[j].envelope()
		if c := a.At.Compare(b.At); c != 0 {
			return c < 0
		}
		if a.Writer != b.Writer {
			return a.Writer < b.Writer
		}
		return a.Seq < b.Seq
	})
}

// parse reads a stream line by line, checking what every record must carry and
// handing the rest to the stream's own check.
//
// A line that will not parse, or that does not carry an envelope this build can
// read, is quarantined and the rest of the stream is read — because the line
// most likely to be torn is the last one before a crash, and everything before
// it is intact. A fold that halted on one torn line would make a single crash
// stop the system permanently; one that guessed would invent a record nobody
// wrote.
func parse[T record](stream string, src []byte, check func(T) string) ([]T, []Quarantined) {
	var records []T
	var quarantined []Quarantined

	for i, raw := range strings.Split(string(src), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}

		var r T
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			quarantined = append(quarantined, Quarantined{
				Stream: stream, Line: i + 1, Raw: line, Why: fmt.Sprintf("not a record: %s", err),
			})
			continue
		}

		why := envelopeWhy(r.envelope())
		if why == "" {
			why = check(r)
		}
		if why != "" {
			quarantined = append(quarantined, Quarantined{
				Stream: stream, Line: i + 1, Raw: line, Why: why,
			})
			continue
		}
		records = append(records, r)
	}
	return records, quarantined
}

// envelopeWhy is what every record must carry whatever stream it is in.
func envelopeWhy(e Envelope) string {
	switch {
	case e.V == 0:
		return "no envelope version, so nothing says which shape this is"
	case e.V > Version:
		return fmt.Sprintf("envelope version %d, and this build reads %d", e.V, Version)
	case e.Writer == "":
		return "no writer, so nothing says which run recorded it"
	case e.At.IsZero():
		return "no event time, and the fold is by event time"
	}
	return ""
}

// actWhy is what an act must carry beyond the envelope.
func actWhy(d Act) string {
	switch {
	case d.Artifact == "":
		return "names no artifact"
	case d.Kind != Ratified && d.Kind != Released:
		return fmt.Sprintf("kind %q is not an act this build knows", d.Kind)
	case d.Kind == Ratified && d.Hash == "":
		// Standing attaches to a content version. A ratification naming no
		// content version would grant standing to whatever the file said
		// next, which is the one thing standing must never do.
		return "a ratification with no content hash grants standing to nothing in particular"
	case d.Kind == Ratified && d.Norm == 0:
		// Recorded from the first ratification, never added when the rules
		// first change: without it a refinement cannot re-hash old records
		// under the version they were written with. See docs/decision/0022.
		return "a ratification with no norm cannot be re-hashed if the rules are refined"
	}
	return ""
}
