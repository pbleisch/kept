package ledger

import (
	"fmt"
	"strconv"
	"strings"
)

// Divergence is what [Ref] and [UpstreamRef] say about each other, computed
// with no network at all.
//
// This is the question [docs/decision/0016] said could not be answered — "the
// only way to know is to ask the remote" — and it was true of a refspec that
// wrote the remote's value straight into the local ref, because there was
// nothing left to compare against. With both values present in one repository
// it is one rev-list.
//
// Ahead and Behind are counts of commits rather than of records. A commit is
// one call to [RefStore.mutate], so the number is the number of writes and not
// the number of acts; it is a magnitude a person reads, and nothing is derived
// from it.
type Divergence struct {
	// Ledger is the value Ref holds, and is empty when it holds none.
	Ledger string
	// Upstream is the value UpstreamRef holds, and is empty when no fetch has
	// brought one.
	Upstream string
	// Ahead is how many commits Ledger holds that Upstream cannot reach.
	Ahead int
	// Behind is how many commits Upstream holds that Ledger cannot reach.
	Behind int
}

// Known reports whether a fetch has brought an upstream value to compare
// against. False is *nobody has asked the remote*, which is neither agreement
// nor divergence, and reporting it as either would make a fresh clone look like
// a broken one — the same distinction [ErrNoStream] draws one level down.
func (d Divergence) Known() bool { return d.Upstream != "" }

// Diverged reports whether both refs hold commits the other cannot reach. This
// is the state kept will not resolve on its own: two clones both appended, and
// choosing which record set survives is not a fast-forward.
func (d Divergence) Diverged() bool { return d.Ahead > 0 && d.Behind > 0 }

// Advanceable reports whether Ref can be moved onto Upstream by a
// fast-forward — the upstream holds records the ledger does not, and the ledger
// holds none the upstream cannot reach. An absent local ledger is the strongest
// case of it: a clone that has fetched and never written.
func (d Divergence) Advanceable() bool { return d.Known() && d.Behind > 0 && d.Ahead == 0 }

// Sync is what [Advance] did.
type Sync string

const (
	// SyncUnchanged is a ledger that already holds everything the upstream ref
	// does, or a repository where no fetch has brought one. Nothing was
	// written.
	SyncUnchanged Sync = "unchanged"
	// SyncAdvanced is a ledger that was fast-forwarded onto the upstream value.
	SyncAdvanced Sync = "advanced"
	// SyncDiverged is a ledger and an upstream ref that have both moved.
	// Nothing was written: advancing would discard one side.
	SyncDiverged Sync = "diverged"
)

// Compare reports what [Ref] and [UpstreamRef] say about each other, and writes
// nothing. A ledger kept in files has no refs, so a manuscript outside Git
// reports an unknown divergence rather than an error.
func Compare(store Store) (Divergence, error) {
	s, ok := store.(*RefStore)
	if !ok {
		return Divergence{}, nil
	}
	return s.compare()
}

// Advance fast-forwards [Ref] onto [UpstreamRef] when that is a fast-forward,
// and does nothing otherwise.
//
// **The verbs that write call this and the verbs that read do not**, and that
// split is the whole design rather than an optimization. A read that advanced
// the ledger could not run against a checkout nobody may write to, which is
// what docs/decision/0001 and docs/decision/0032 make kept status and kept why
// answer from. A write that did *not* advance is worse: appending onto a ledger
// that is behind forks it, so a clone that fetched and then ticked would
// manufacture a divergence out of nothing. Git makes you merge before you can
// push for the same reason; kept's spelling of it is advance before you append.
//
// It cannot lose a record. A fast-forward's destination reaches every value the
// source did, so [RefStore.Continuous] stays quiet across one by construction —
// both are statements about the same DAG.
//
// A divergence is reported and never resolved here. See [Sync].
func Advance(store Store) (Sync, Divergence, error) {
	s, ok := store.(*RefStore)
	if !ok {
		return SyncUnchanged, Divergence{}, nil
	}
	return s.advance()
}

func (s *RefStore) compare() (Divergence, error) {
	ledger, _, err := s.revision()
	if err != nil {
		return Divergence{}, err
	}
	upstream, held, err := s.value(UpstreamRef)
	if err != nil || !held {
		return Divergence{Ledger: ledger}, err
	}
	d := Divergence{Ledger: ledger, Upstream: upstream}
	if ledger == "" {
		// An absent ledger reaches nothing, so everything upstream is behind.
		// rev-list refuses a range against a ref that is not there, which is
		// why this is counted rather than ranged.
		count, err := s.count(upstream)
		if err != nil {
			return d, err
		}
		d.Behind = count
		return d, nil
	}
	if ledger == upstream {
		return d, nil
	}
	// One walk for both sides. --left-right --count over a symmetric difference
	// is what git answers "how far apart" with, and it is the same question in
	// both directions.
	out, err := s.git(nil, nil, "rev-list", "--left-right", "--count", ledger+"..."+upstream)
	if err != nil {
		return d, err
	}
	fields := strings.Fields(string(out))
	if len(fields) != 2 {
		return d, fmt.Errorf("git rev-list --left-right --count returned %q", strings.TrimSpace(string(out)))
	}
	if d.Ahead, err = strconv.Atoi(fields[0]); err != nil {
		return d, err
	}
	if d.Behind, err = strconv.Atoi(fields[1]); err != nil {
		return d, err
	}
	return d, nil
}

func (s *RefStore) advance() (Sync, Divergence, error) {
	// The same retry budget the append path uses, and for the same reason: two
	// runs may advance the same ledger, and the loser reads the value the
	// winner wrote and finds nothing left to do. No lock is load-bearing.
	var last error
	for range 8 {
		d, err := s.compare()
		if err != nil {
			return SyncUnchanged, d, err
		}
		switch {
		case d.Diverged():
			return SyncDiverged, d, nil
		case !d.Advanceable():
			return SyncUnchanged, d, nil
		}

		old := d.Ledger
		if old == "" {
			// A zero object name is the compare-and-swap value Git expects for
			// an absent ref. Its width follows the repository object format.
			old = strings.Repeat("0", len(d.Upstream))
		}
		// -m for the reason every writer git logs leaves one, so an entry that
		// is not kept's is legible at a glance. See docs/decision/0054.
		if _, err := s.git(nil, nil, "update-ref", "-m", "kept ledger sync", Ref, d.Upstream, old); err == nil {
			return SyncAdvanced, d, nil
		} else if !retryUpdate(err) {
			return SyncUnchanged, d, err
		} else {
			last = err
		}
	}
	return SyncUnchanged, Divergence{}, fmt.Errorf("advance %s onto %s after concurrent ledger writes: %w", Ref, UpstreamRef, last)
}

// value reads any ref the same way [RefStore.revision] reads Ref: absent is a
// verdict rather than an error.
func (s *RefStore) value(ref string) (string, bool, error) {
	out, err := s.git(nil, nil, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	if err != nil {
		if isExit(err, 1) {
			return "", false, nil
		}
		return "", false, err
	}
	revision := strings.TrimSpace(string(out))
	if revision == "" {
		return "", false, fmt.Errorf("Git returned an empty %s revision", ref)
	}
	return revision, true, nil
}

func (s *RefStore) count(revision string) (int, error) {
	out, err := s.git(nil, nil, "rev-list", "--count", revision)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(out)))
}
