package corpus

import (
	"fmt"
	"slices"
	"strings"

	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/schema"
)

// The fields a desire says what it would become in. Neither binds — a desire's
// hash list is empty — and neither is a state: whether a desire is shaped is
// read out of them, and never written into one. See docs/decision/0083.
const (
	fieldBecomes   = "becomes"
	fieldCandidate = "candidate"
)

// The two fields a desire writes the condition that would change its answer in.
// Which one belongs is decided by the desire's state, which the corpus cannot
// see: reopens_when is read on a retired desire and ripe_when on one nobody has
// retired. They are exported because the readers that know the state are the
// ones that ask. See docs/decision/0083.
const (
	FieldRipeWhen    = "ripe_when"
	FieldReopensWhen = "reopens_when"
)

// Shaping reads what a desire says it would become, and says why that does not
// read yet. An empty reason is a desire that is shaped: it names a kind, and its
// candidate is the fields of an artifact of that kind with nothing wrong.
//
// **It is the load's own judgment applied to the candidate, and not a second
// one.** A candidate reads when the schema of the kind it names reports no
// violation over it, its max_age parses, every gather entry is one the
// catalogue this corpus was loaded with can run as written, and its hold_when
// parses and reads only what those gatherers declare — which is exactly what
// [read] reports about an artifact of that kind. A rule for *shaped* written
// separately would drift from the rule for *loads*, and a candidate that shaped
// and then would not load is the defect this is here to prevent.
//
// Nothing is reported. A candidate that does not read is a want somebody is
// still working, which is the ordinary state of one, and a corpus problem for
// every draft would make kept list a list of drafts. The reason is carried to
// whoever reads the desire's state instead.
//
// becomes is returned whatever the answer, as authored, so that a reader can
// say what a desire would become even when it does not read yet.
func (c *Corpus) Shaping(e *Entry) (becomes, unread string) {
	fields := e.Artifact.Fields
	becomes = asString(fields[fieldBecomes])
	raw := fields[fieldCandidate]

	switch {
	case becomes == "" && raw == nil:
		return "", "it names nothing it would become"
	case becomes == "":
		return "", "it carries a candidate and names no kind to read it as"
	}

	if own, ok := c.Schemas.For(string(e.Artifact.Kind)); ok {
		if f, declared := own.Fields[fieldBecomes]; declared && f.Type == schema.TypeEnum &&
			len(f.Values) > 0 && !slices.Contains(f.Values, becomes) {
			return becomes, fmt.Sprintf("it would become a %s, and a %s becomes one of %s",
				becomes, e.Artifact.Kind, strings.Join(f.Values, ", "))
		}
	}
	sc, ok := c.Schemas.For(becomes)
	if !ok {
		return becomes, fmt.Sprintf("it would become a %s, and nothing declares that kind", becomes)
	}
	if raw == nil {
		return becomes, fmt.Sprintf("it would become a %s and carries no candidate", becomes)
	}
	candidate := asStringMap(raw)
	if candidate == nil {
		return becomes, fmt.Sprintf("its candidate is %T, and the fields of a %s are a map", raw, becomes)
	}

	var wrong []string
	for _, v := range sc.Check(candidate) {
		wrong = append(wrong, v.String())
	}
	if _, err := readAge(candidate[fieldMaxAge]); err != nil {
		wrong = append(wrong, fmt.Sprintf("%s: %s", fieldMaxAge, err))
	}
	art := &model.Artifact{ID: e.Artifact.ID, Kind: model.Kind(becomes), Fields: candidate}
	project(art, candidate)
	checkPredicate(art, c.gatherers, func(_ ProblemKind, format string, args ...any) {
		wrong = append(wrong, fmt.Sprintf(format, args...))
	})
	if len(wrong) > 0 {
		return becomes, fmt.Sprintf("its candidate does not read as a %s: %s", becomes, strings.Join(wrong, "; "))
	}
	return becomes, ""
}
