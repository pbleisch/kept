package tick

import (
	"fmt"
	"time"

	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/register"
)

// drift is the query "has standing and lacks truth", asked of one artifact.
//
// It is computed and never written down, so it cannot go stale, cannot need
// invalidating, and cannot leak. The kinds are only the ways truth-seeking
// failed, and telling them apart is the whole diagnostic value: a missing
// toolchain and a broken promise are both "not kept" to a boolean and are two
// different conversations.
func (s Subject) drift() (model.Drift, bool) {
	standing := s.Standing.State
	if standing != register.Ratified && standing != register.Absent {
		// It obliges nobody. An ordinary corpus is full of these, and treating
		// one as drift would make the whole query useless.
		return model.Drift{}, false
	}

	d := model.Drift{ArtifactID: s.ID}
	switch {
	case standing == register.Absent:
		// Ratified content that cannot be found. Never read as a release: an
		// absence cannot be told apart from a bad merge, a partial checkout or
		// a corpus root pointed at the wrong place. See docs/decision/0021.
		d.Kind, d.Since = model.DriftAbsent, s.Standing.Granted.At

	case !s.Predicated():
		// It binds without being evaluated, which is what most canon does.
		// There is no truth to lack.
		return model.Drift{}, false

	case s.Unevaluated != "":
		// A predicate that will not parse is a mistake rather than an unknown,
		// and the corpus reported it. Nothing measured, so nothing is known.
		d.Kind, d.Since = model.DriftUnverified, s.Standing.Granted.At

	case !s.Answer.Reached():
		d.Kind, d.Since = model.DriftUnverified, s.Standing.Granted.At

	case s.Due && !s.Answer.Gathered:
		// There is an answer and it is about a tree this is not. Nothing has
		// re-observed it, which is what stale means.
		d.Kind, d.Since = model.DriftStale, s.Answer.Since

	case s.Answer.Value == model.Unknown:
		d.Kind, d.Since = model.DriftUnknown, s.Answer.Since

	case s.Answer.Value == model.NotKept:
		d.Kind, d.Since = model.DriftNotKept, s.Answer.Since
		if s.heldBefore() {
			// False now, true before. Worth its own kind because a promise
			// that has never held and one that stopped holding send a reader
			// to different places.
			d.Kind = model.DriftRegressed
		}

	default:
		return model.Drift{}, false
	}

	d.Truth = model.Truth{
		ArtifactID:  s.ID,
		ContentHash: s.Standing.Hash.String(),
		World:       s.Answer.World,
		Value:       s.Answer.Value,
		At:          s.Answer.Since,
		From:        s.Answer.From,
		Detail:      s.Answer.Detail,
	}
	return d, true
}

// heldBefore reports whether this wording was ever kept.
func (s Subject) heldBefore() bool {
	hash := s.Standing.Hash.String()
	for _, r := range s.Truth {
		if r.Hash == hash && r.Value == model.Kept {
			return true
		}
	}
	return false
}

// dispose records what this pass did about one drift, and why.
//
// Nothing repairs in this build, so every disposition is a refusal — and the
// condition is not a placeholder. Confinement is what a repair is dispatched
// under, `unset` is not `none`, and a named provider is verified by a canary
// rather than trusted by name; neither is satisfied here, and both are
// permanent statements about this workspace rather than about this increment.
// A refusal that named no condition would be the shrug docs/decision/0019 was
// written to replace.
func (t *Tick) dispose(s *Subject, world model.World) error {
	d := ledger.Disposition{
		Artifact:  s.ID,
		Hash:      s.Standing.Hash.String(),
		Norm:      s.Standing.Hash.Norm,
		World:     world,
		Outcome:   ledger.Refused,
		Condition: ledger.Provider,
		Detail:    t.refusal(),
	}
	s.Disposition = d

	last, held := ledger.LastDisposition(s.Dispositions)
	// Transitions, not ticks: the same refusal on the same grounds for four
	// hundred consecutive ticks is one record and a heartbeat. Without this the
	// volume argument defeats the decision.
	heartbeat := held && s.maxAge() > 0 && time.Since(last.At) > s.maxAge()
	if !d.Moved(last, held) && !heartbeat {
		s.Disposition = last
		return nil
	}

	written, err := t.Ledger.Dispose(d)
	if err != nil {
		return err
	}
	s.Disposition, s.Disposed = written, true
	return nil
}

func (t *Tick) refusal() string {
	if t.Options.Provider == "" {
		return "no confinement provider is named in .kept/config.yaml, and unset is not none, " +
			"so nothing may be dispatched into a box"
	}
	return fmt.Sprintf("the confinement provider %q has not been verified by a canary, "+
		"and a provider is verified rather than trusted by name", t.Options.Provider)
}

// maxAge is the artifact's own heartbeat interval, which dispositions share
// with truth. There is one knob for how old an answer may be, and a refusal
// that has stood unrepeated for longer than that is as invisible as a verdict
// that has.
func (s Subject) maxAge() time.Duration {
	if s.Entry == nil {
		return 0
	}
	return s.Entry.Artifact.MaxAge
}

// DisposedSince is when the disposition in force was reached, for a reader who
// wants "refused for six days" rather than a timestamp.
//
// It walks back through the stream while the outcome and the condition hold,
// because the same refusal on the same grounds is one record and a heartbeat:
// the last record's timestamp is when it was last repeated, and what a person
// is asking is when it started.
func (s Subject) DisposedSince() time.Time {
	held := s.Disposition
	if held.Outcome == "" {
		last, found := ledger.LastDisposition(s.Dispositions)
		if !found {
			return time.Time{}
		}
		held = last
	}

	var at time.Time
	for i := len(s.Dispositions) - 1; i >= 0; i-- {
		d := s.Dispositions[i]
		if d.Outcome != held.Outcome || d.Condition != held.Condition {
			break
		}
		at = d.At
	}
	return at
}
