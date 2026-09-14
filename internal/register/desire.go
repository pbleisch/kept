package register

import (
	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/model"
)

// DesireState is what has become of a want. It is derived on read from the same
// two inputs standing is — what the corpus says now and the acts in the ledger —
// and it is written nowhere, which is why a desire's schema has no status field.
// See docs/decision/0083.
type DesireState string

const (
	// DesireRaw: it exists, and does not yet say what it would become in a form
	// that reads. [Desire.Unread] says which part is missing.
	DesireRaw DesireState = "raw"
	// DesireShaped: it names a kind, and its candidate reads as the fields of an
	// artifact of that kind with nothing wrong. See [corpus.Corpus.Shaping].
	DesireShaped DesireState = "shaped"
	// DesireCommitted: an artifact with standing in force says, in its own
	// links, that it realizes this desire. Writing a candidate into the corpus
	// does not reach it; a person's grant does.
	DesireCommitted DesireState = "committed"
	// DesireRetired: a person retired it, and nothing granted since realizes it.
	DesireRetired DesireState = "retired"
)

// Realizes is the relation an artifact commits to a desire with, written in the
// realizing artifact's own links as { target: desire:x, relation: realizes }.
const Realizes = "realizes"

// Desire is one desire's state, and what it was derived from.
type Desire struct {
	State DesireState

	// Becomes is the kind the desire names, as authored, whatever its state.
	Becomes string
	// Unread is why a raw desire is not shaped. It is empty for every other
	// state.
	Unread string

	// RealizedBy is every ratified artifact whose own links say it realizes
	// this desire, with the grant in force for each, in id order. It is carried
	// in every state, so a retired desire still names what cites it — which is
	// how a reader learns that a decline was recorded after the commitment.
	RealizedBy []Realization
}

// Realization is one ratified artifact realizing a desire, and the grant that
// gives it standing now.
type Realization struct {
	Artifact string
	Granted  Grant
}

// desires fills in what has become of every desire the corpus holds.
//
// A retired desire is committed exactly when something realizing it was granted
// after the retirement, and retired otherwise. It is the fold 0032 already
// performs for a kind that bears standing — a retirement empties the set and a
// later ratification is the way back — generalized to a kind whose way back is
// a grant of something else. What it rules out is a file edit overturning an
// act: a realizes link added to a promise ratified before the retirement moves
// nothing, because that grant is older than the decline.
//
// An absent desire has no entry, so it has no fields to shape and no Desire; the
// register's own retired or absent state is what a reader gets for it.
func desires(c *corpus.Corpus, entries []Standing) {
	index := make(map[string]int, len(entries))
	for i, s := range entries {
		index[s.Artifact] = i
	}
	realized := realizations(c, entries, index)

	for _, e := range c.All() {
		if e.Artifact.Kind != model.KindDesire {
			continue
		}
		i, ok := index[e.Artifact.ID]
		if !ok {
			continue
		}
		entries[i].Desire = desire(c, e, entries[i], realized[e.Artifact.ID])
	}
}

func desire(c *corpus.Corpus, e *corpus.Entry, s Standing, by []Realization) *Desire {
	d := &Desire{RealizedBy: by}
	becomes, unread := c.Shaping(e)
	d.Becomes = becomes

	retired := s.State == Retired
	switch {
	case len(by) > 0 && (!retired || grantedAfter(by, s.Granted)):
		d.State = DesireCommitted
	case retired:
		d.State = DesireRetired
	case unread == "":
		d.State = DesireShaped
	default:
		d.State, d.Unread = DesireRaw, unread
	}
	return d
}

// realizations is every desire id a ratified artifact realizes, keyed by that
// id.
//
// Three things narrow what counts, and each is the reason a written file cannot
// commit anything. The artifact must be ratified now, so a candidate promise
// sitting unratified leaves its desire shaped and a revised one stops
// committing it. The link must be in the artifact's own frontmatter: a body
// wiki-link states no relation, and a _links manifest edge is a third party's
// assertion about an artifact it did not write (docs/decision/0030). And the
// artifact's schema must admit the relation, because a relation its kind does
// not admit is a problem kept list reports, and a defect commits nothing.
func realizations(c *corpus.Corpus, entries []Standing, index map[string]int) map[string][]Realization {
	out := map[string][]Realization{}
	for _, e := range c.All() {
		i, ok := index[e.Artifact.ID]
		if !ok || entries[i].State != Ratified {
			continue
		}
		sc, ok := c.Schemas.For(string(e.Artifact.Kind))
		if !ok || !sc.AllowsRelation(Realizes) {
			continue
		}
		for _, l := range e.Links {
			if l.From != corpus.FromFrontmatter || l.Relation != Realizes {
				continue
			}
			out[l.Target] = append(out[l.Target], Realization{Artifact: e.Artifact.ID, Granted: entries[i].Granted})
		}
	}
	return out
}

// grantedAfter reports whether any of these grants was performed after the
// retirement. Strictly after: a tie is not a later act, and the decline stands.
func grantedAfter(by []Realization, retirement Grant) bool {
	for _, r := range by {
		if r.Granted.At.After(retirement.At) {
			return true
		}
	}
	return false
}
