package ledger

import (
	"sort"
	"time"

	"github.com/pbleisch/kept/internal/canonical"
)

// projectionDomain separates a ledger projection from anything else hashed with
// the same function. Two domains hashing identical bytes cannot collide.
const projectionDomain = "kept/ledger-projection\n"

// HumanPaced reports whether a kind is one a person performs.
//
// This is docs/decision/0046's guard, and it is the whole defense. The ledger's
// streams split cleanly: `acts.jsonl` is human-paced, and everything else —
// truth, dispositions, observations, runs — is written by the tick. A gatherer
// declaring a dependency on a tick-written stream would gather, cause an
// append, move its own scope digest, and be due again, which is the tick
// feeding itself one level deeper than the `.kept/` exclusion reaches.
//
// It answers about a kind rather than a stream because a kind is what a
// gatherer declares and what a record carries, and because the two questions
// have one answer: every kind in the acts stream is a person's, and every kind
// outside it is the tick's.
func HumanPaced(k Kind) bool {
	switch k {
	case Ratified, Released:
		return true
	}
	return false
}

// Project is the ledger as a scope digest may name it: a digest over the acts
// of the declared kinds that name one of these artifacts.
//
// It is a projection and never a head. A head digest, or a per-artifact stream
// version, moves on every truth record and every disposition, so everything
// would be due every tick; a digest over a whole stream converges but makes a
// promise about chapters 1–5 due when chapter 40 is read. This moves only when
// a person records an act about something the promise is about. See
// docs/decision/0046.
//
// The empty string is what a gatherer that declared nothing gets, and it is not
// a digest over an empty set: it is the answer that there is nothing to fold
// in, which is what keeps every existing gatherer's scope digest byte for byte
// what it was.
//
// The fold is over the assertion each act makes — its kind, the artifact, the
// content version, who performed it and when — sorted here rather than taken in
// the order the caller happened to hold them. The stream is union-merged and
// interleaves arbitrarily, so an order that came from file order would be
// silently wrong and would look correct in every single-run test.
func Project(acts []Act, kinds []Kind, artifacts []string) string {
	if len(kinds) == 0 {
		return ""
	}
	want := make(map[Kind]bool, len(kinds))
	for _, k := range kinds {
		want[k] = true
	}
	about := make(map[string]bool, len(artifacts))
	for _, a := range artifacts {
		about[a] = true
	}

	type assertion struct {
		kind     string
		artifact string
		hash     string
		by       string
		at       string
	}
	var found []assertion
	for _, a := range acts {
		if !want[a.Kind] || !about[a.Artifact] {
			continue
		}
		found = append(found, assertion{
			kind:     string(a.Kind),
			artifact: a.Artifact,
			hash:     a.Hash,
			by:       a.By,
			at:       a.At.UTC().Format(time.RFC3339Nano),
		})
	}
	sort.Slice(found, func(i, j int) bool {
		a, b := found[i], found[j]
		switch {
		case a.at != b.at:
			return a.at < b.at
		case a.artifact != b.artifact:
			return a.artifact < b.artifact
		case a.kind != b.kind:
			return a.kind < b.kind
		case a.hash != b.hash:
			return a.hash < b.hash
		}
		return a.by < b.by
	})

	// The encoding is docs/decision/0031's, so the fold is unambiguous: two
	// acts cannot encode like one, and a field cannot be re-read as its
	// neighbour. Text is verbatim, because every one of these was measured
	// off a record rather than authored here.
	enc := canonical.NewEncoder(projectionDomain, canonical.Verbatim)
	enc.List(len(found))
	for _, a := range found {
		enc.Text(a.kind)
		enc.Text(a.artifact)
		enc.Text(a.hash)
		enc.Text(a.by)
		enc.Text(a.at)
	}
	return canonical.Algorithm + ":" + enc.Digest()
}
