package register

import (
	"errors"
	"sort"
	"time"

	"github.com/pbleisch/kept/internal/canonical"
	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/ledger"
)

// State is what the ledger and the artifact say together.
type State string

const (
	// Ratified: the ledger holds a ratification naming the hash this artifact's
	// content produces now, and nothing has released it since.
	Ratified State = "ratified"
	// Unratified: it obliges nobody. Reason says which way — never granted, or
	// granted at a content version this is no longer.
	Unratified State = "unratified"
	// Released: a person retired it. It stops being held to, and editing it
	// does not bring it back.
	Released State = "released"
	// Absent: ratified, and the content it was ratified against cannot be
	// found. It keeps its standing and says the content is gone — because an
	// absence cannot be told apart from a bad merge, a partial checkout or a
	// corpus root pointed at the wrong place, and one misconfiguration must not
	// silently release a corpus. See docs/decision/0021.
	Absent State = "absent"
)

// Reason says why an artifact is Unratified. It is empty for every other state.
type Reason string

const (
	// Never: no act in the ledger names this artifact.
	Never Reason = "never"
	// Revised: it was ratified, and the content has changed since. This is the
	// ordinary case, and it is reached by comparing hashes rather than by
	// anything having marked the artifact when it was edited.
	Revised Reason = "revised"
	// Unbinding: nothing about this kind binds, so there is no content version
	// to key a ratification to and there never will be. A desire is the worked
	// example: ratifying one means writing the promise or the canon entry it
	// became, not granting standing where it sits.
	Unbinding Reason = "unbinding"
	// Unrehashed: every ratification for it was computed under canonicalization
	// rules this build does not compute under. Decision 0022 says such a record
	// is re-hashed under the version it was written with, so that refining the
	// rules is not a bulk de-ratification — and that re-hashing is not built,
	// because there has only ever been one version.
	//
	// It is a separate reason rather than Revised because the two send a reader
	// to different places. Revised means somebody edited the artifact.
	// Unrehashed means nobody did, and the tool changed underneath it.
	Unrehashed Reason = "unrehashed"
)

// Grant is the act behind a state: which content version, who, and when.
type Grant struct {
	// Hash is the content version, with the rules it was computed under. Both
	// halves are compared: a digest that agrees under different rules answered
	// a different question. See docs/decision/0022.
	Hash canonical.Hash
	By   string
	At   time.Time
	Note string

	// Where is the world the act was performed in, carried through untouched.
	// Nothing in this package reads it: standing is derived by comparing a
	// content hash, and a grant that depended on a world would evaporate under
	// an ordinary rebase, merge or clone. It is here so that a reader who has
	// the grant has what the act said about where it happened. See
	// docs/decision/0063.
	Where ledger.Where
}

// Standing is one artifact's entry in the register.
type Standing struct {
	// Artifact is "type:slug". It is the authored identity, so this entry
	// survives the file being renamed and does not survive the id being edited
	// by hand — which presents as one artifact gone absent and one new one
	// appearing, because that is what it is.
	Artifact string
	State    State
	Reason   Reason

	// Hash is what the artifact's binding content hashes to now, and is zero
	// when the content is absent or when nothing about the kind binds.
	Hash canonical.Hash
	// Unhashable is why Hash is empty for an artifact that is present.
	Unhashable string

	// Granted is the act that produced State: the ratification in force
	// for Ratified, the most recent ratification for Unratified/Revised, and
	// the release for Released. Zero when nothing ever named this artifact.
	Granted Grant

	// Dormant is every grant still in force for a content version this
	// artifact does not say now, oldest first.
	//
	// They are live standing and not leftovers: standing attaches to a content
	// version, so reverting the artifact to one of these wordings makes it
	// ratified again with nobody granting anything. That is the property
	// [history.granted] exists for, and it is also why a grant matching no
	// content here cannot on its own be read as a mistake — one of these is
	// the ordinary state of every artifact that has ever been revised.
	//
	// A release empties the set, so a released artifact has none. An absent
	// one has all of them, because there is no content to be the current
	// wording.
	Dormant []Grant
}

// Derive computes the register: for every artifact the corpus holds, and every
// artifact the ledger names and the corpus does not.
//
// It reads and returns. Nothing is written, nothing is cached, and no artifact
// is marked — the comparison between a recorded hash and a current one is the
// entire mechanism, so an edit made with any editor, by any process, at any
// time, is accounted for the next time this runs.
//
// acts must already be folded into the order they happened in;
// [ledger.ReadActs] does that.
func Derive(c *corpus.Corpus, acts []ledger.Act) []Standing {
	folded := map[string]*history{}
	for _, d := range acts {
		h, ok := folded[d.Artifact]
		if !ok {
			h = &history{granted: map[canonical.Hash]Grant{}}
			folded[d.Artifact] = h
		}
		h.apply(d)
	}

	var out []Standing
	present := map[string]bool{}
	for _, e := range c.All() {
		id := e.Artifact.ID
		present[id] = true
		out = append(out, standing(e, c, folded[id]))
	}

	// An artifact the ledger names and the corpus does not. Standing outlives
	// the file, so this is where a deleted promise surfaces instead of
	// vanishing — which is the whole of why deletion is not a way to be rid of
	// one.
	for id, h := range folded {
		if present[id] {
			continue
		}
		out = append(out, h.absent(id))
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Artifact < out[j].Artifact })
	return out
}

// standing answers for one artifact the corpus holds.
func standing(e *corpus.Entry, c *corpus.Corpus, h *history) Standing {
	s := Standing{Artifact: e.Artifact.ID}

	sc, _ := c.Schemas.For(string(e.Artifact.Kind))
	hash, err := canonical.Bind(e, sc)
	if err != nil {
		s.Unhashable = why(err)
	} else {
		s.Hash = hash
	}
	bound := s.Hash.Digest != ""

	switch {
	case h == nil:
		s.State, s.Reason = Unratified, Never
		if !bound {
			s.Reason = Unbinding
		}
		return s

	case len(h.granted) == 0:
		// Every ratification has been retired, or there was never one and only
		// a release. Either way nothing is in force and the release says so.
		s.State, s.Granted = Released, h.retired
		return s
	}

	if g, ok := h.granted[s.Hash]; ok && bound {
		s.State, s.Granted, s.Dormant = Ratified, g, h.dormant(s.Hash)
		return s
	}
	s.Dormant = h.dormant(s.Hash)

	// Ratified once, at a content version this is not. Nothing had to notice
	// the edit: the hash the file produces now is simply not one the operator
	// ever granted.
	s.State, s.Reason, s.Granted = Unratified, Revised, h.latest
	switch {
	case !bound:
		s.Reason = Unbinding
	case h.underOtherRules(s.Hash.Norm):
		// Nobody edited anything. Every grant was computed under rules this
		// build does not use, so the comparison could not have matched and
		// reporting a revision would send a reader looking for an edit that
		// never happened.
		s.Reason = Unrehashed
	}
	return s
}

// why is the reason without the subject. An entry already names its artifact,
// and an error that repeats it reads as two different things having gone wrong.
func why(err error) string {
	if u, ok := errors.AsType[*canonical.Unbindable](err); ok {
		return u.Why
	}
	return err.Error()
}

// history folds one artifact's acts.
type history struct {
	// granted is every content version with standing in force, keyed by hash.
	// A set rather than the last one, because standing attaches to a content
	// version: revert a promise to a wording the operator already ratified and
	// that grant applies again, exactly as it did before the revision. Nothing
	// had to be re-granted, because nothing was ever withdrawn.
	granted map[canonical.Hash]Grant
	// latest is the most recent ratification, whatever its hash. It is what a
	// revised artifact is compared against when it is reported.
	latest Grant
	// retired is the release that emptied the set.
	retired Grant
}

func (h *history) apply(d ledger.Act) {
	g := Grant{Hash: canonical.Parse(d.Hash, d.Norm), By: d.By, At: d.At, Note: d.Note, Where: d.Where}
	switch d.Kind {
	case ledger.Ratified:
		h.granted[g.Hash] = g
		h.latest = g
	case ledger.Released:
		// A release retires the artifact rather than a wording. Clearing the
		// set is what keeps a released promise from coming back by being
		// edited into a shape someone once approved.
		h.granted = map[canonical.Hash]Grant{}
		h.retired = g
	}
}

// underOtherRules reports whether every grant was computed under a
// canonicalization version other than this one, which makes a mismatch a fact
// about the tool rather than about the artifact.
func (h *history) underOtherRules(now int) bool {
	for hash := range h.granted {
		if hash.Norm == now {
			return false
		}
	}
	return len(h.granted) > 0
}

// dormant is every grant in force for a content version other than the one the
// artifact says now, oldest first. A zero `now` is an artifact with no content
// to be the current wording, so every grant is dormant.
//
// Oldest first, then by digest: a map has no order, and a report that listed
// one artifact's grants differently on two runs would read as the grants having
// moved.
func (h *history) dormant(now canonical.Hash) []Grant {
	var out []Grant
	for hash, g := range h.granted {
		if now.Digest != "" && hash == now {
			continue
		}
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].At.Equal(out[j].At) {
			return out[i].At.Before(out[j].At)
		}
		return out[i].Hash.Digest < out[j].Hash.Digest
	})
	return out
}

// absent answers for an artifact the ledger names and the corpus does not.
func (h *history) absent(id string) Standing {
	if len(h.granted) == 0 {
		return Standing{Artifact: id, State: Released, Granted: h.retired}
	}
	return Standing{Artifact: id, State: Absent, Granted: h.latest, Dormant: h.dormant(canonical.Hash{})}
}

// Count returns how many entries are in each state, for a summary line.
func Count(entries []Standing) map[State]int {
	out := map[State]int{}
	for _, e := range entries {
		out[e.State]++
	}
	return out
}
