package canonical

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/schema"
)

// Algorithm is the digest the binding hash uses. It is written into every
// record beside the digest, so a later change is additive rather than a
// reinterpretation of records already written.
const Algorithm = "sha256"

// domain separates this preimage from any other thing that might one day be
// hashed with the same function. It carries the rules version, so a record
// hashed under norm:1 and one hashed under norm:2 cannot collide even if the
// content is identical.
const domain = "kept/binding-hash"

// Hash is what a ratification is keyed to: a digest over an artifact's binding
// content, and the version of the rules that produced it.
//
// It is world-independent by construction — nothing about where it was computed
// enters the preimage — which is why standing needs nothing added for
// distribution, ever, while truth does. See docs/decision/0013.
type Hash struct {
	Algorithm string
	Digest    string // lowercase hex
	Norm      int    // the canonicalization rules version — 0022
}

// String is the form the ledger and the terminal both use.
func (h Hash) String() string {
	if h.Digest == "" {
		return ""
	}
	return h.Algorithm + ":" + h.Digest
}

// Short abbreviates for a table. Records always carry the whole digest.
func (h Hash) Short() string {
	if len(h.Digest) < 12 {
		return h.String()
	}
	return h.Algorithm + ":" + h.Digest[:12]
}

// Equal compares two hashes as the register compares them: same digest, same
// algorithm, and same rules. A record written under an earlier norm is not
// equal to one written under a later one even when the hex agrees, because the
// two were answering different questions.
func (h Hash) Equal(other Hash) bool {
	return h.Algorithm == other.Algorithm && h.Digest == other.Digest && h.Norm == other.Norm
}

// Parse reads a hash back off a ledger record.
func Parse(s string, normVersion int) Hash {
	algorithm, digest, found := strings.Cut(s, ":")
	if !found {
		return Hash{Algorithm: Algorithm, Digest: s, Norm: normVersion}
	}
	return Hash{Algorithm: algorithm, Digest: digest, Norm: normVersion}
}

// Unbindable is returned when an artifact has no binding content to hash, which
// means it cannot be ratified at all. It is a refusal rather than a hash over
// nothing: an artifact whose hash covered nothing would keep its standing
// through every edit, which is the one failure this whole mechanism exists to
// prevent.
type Unbindable struct {
	Artifact string
	Why      string
}

func (e *Unbindable) Error() string {
	return fmt.Sprintf("%s cannot be ratified: %s", e.Artifact, e.Why)
}

// Bind computes an artifact's binding hash: the digest a ratification names and
// the register compares against.
//
// It hashes the fields the schema's hash list declares and nothing else, so
// rewriting a promise's prose leaves its standing intact while changing its
// predicate drops it. See docs/decision/0003.
//
// The list is read as a set. Two schemas naming the same fields in a different
// order describe the same commitment, and reordering a list is not a change to
// what binds — so it does not re-hash the corpus, while adding or removing a
// name does.
func Bind(e *corpus.Entry, sc *schema.Schema) (Hash, error) {
	if sc == nil {
		return Hash{}, &Unbindable{
			Artifact: e.Artifact.ID,
			Why: fmt.Sprintf("no %s/%s.yaml, so nothing declares what binds and there is nothing to key a ratification to",
				schema.Dir, e.Artifact.Kind),
		}
	}
	if len(sc.Hash) == 0 {
		return Hash{}, &Unbindable{
			Artifact: e.Artifact.ID,
			Why:      fmt.Sprintf("the %s schema declares hash: [], so nothing about a %s binds", sc.Type, sc.Type),
		}
	}

	names := append([]string(nil), sc.Hash...)
	sort.Strings(names)

	var b bytes.Buffer
	fmt.Fprintf(&b, "%s\nnorm:%d\n", domain, Version)
	b.WriteByte(tagList)
	b.WriteString(strconv.Itoa(len(names)))
	b.WriteByte(':')

	for _, name := range names {
		value, held, err := resolve(name, e)
		if err != nil {
			return Hash{}, &Unbindable{Artifact: e.Artifact.ID, Why: err.Error()}
		}
		encodeText(&b, name)
		if !held {
			b.WriteByte(absent)
			continue
		}
		b.WriteByte(present)
		encode(&b, value)
	}

	sum := sha256.Sum256(b.Bytes())
	return Hash{Algorithm: Algorithm, Digest: hex.EncodeToString(sum[:]), Norm: Version}, nil
}

// resolve says what one name in a hash list stands for. Three of the four
// answers are reserved names from decision 0003; the fourth is an ordinary
// field under the artifact's fields: block, which is where 0010 puts everything
// that binds.
func resolve(name string, e *corpus.Entry) (value any, held bool, err error) {
	switch {
	case name == schema.HashBody:
		// The markdown block always exists, even when it is empty: an artifact
		// with no prose said nothing, which is a thing it said. There is no
		// absent case to distinguish.
		return e.Artifact.Body, true, nil

	case strings.HasPrefix(name, schema.HashBodyPrefix):
		// How a schema declares its sections is open question 5. Guessing here
		// would settle it in whichever direction the first implementation
		// happened to take, and the guess would be permanent — every hash
		// computed under it is a ratification someone granted.
		return nil, false, fmt.Errorf(
			"the %s schema binds on %q, and how a schema declares its body sections is undecided (open question 5)",
			e.Artifact.Kind, name)

	case strings.HasPrefix(name, schema.HashLinksPrefix):
		relation := strings.TrimPrefix(name, schema.HashLinksPrefix)
		return authoredLinks(e, relation), true, nil

	default:
		v, ok := e.Artifact.Fields[name]
		return v, ok, nil
	}
}

// authoredLinks returns the targets of one relation, from the sources the
// artifact's own author wrote — its frontmatter and its prose — and never from
// a _links manifest.
//
// That exclusion is the point rather than an oversight. A manifest exists so a
// third party can assert a link about an artifact without editing it
// (docs/decision/0030), and a binding hash is what a person agreed to when they
// ratified. If a manifest edge bound, anyone able to write a file next to an
// artifact could change what its ratification was granted against — which is
// minting standing by the back door, from outside the one act reserved to a
// person.
//
// Targets are sorted, because a link set has no authored order and reordering a
// list is not a change to what was promised.
func authoredLinks(e *corpus.Entry, relation string) []any {
	var out []string
	for _, l := range e.Links {
		if l.From == corpus.FromManifest || l.Relation != relation {
			continue
		}
		out = append(out, l.Target)
	}
	sort.Strings(out)

	items := make([]any, len(out))
	for i, t := range out {
		items[i] = t
	}
	return items
}
