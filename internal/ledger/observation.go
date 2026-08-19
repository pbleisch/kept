package ledger

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/pbleisch/kept/internal/canonical"
	"github.com/pbleisch/kept/internal/model"
)

// ObsVersion is the version of what an observation's address is computed over.
//
// It is its own version and not canonical.Version, and the difference is the
// whole of decision 0034: the encoding is shared with the binding hash and the
// text rule is not, so a record claiming norm: while not normalizing would be a
// lie a reader cannot detect. Two versions travel together and mean different
// things — norm: is what an artifact's hash was computed under, obs: is what a
// measurement's address was.
const ObsVersion = 1

// obsDomain separates a measurement's preimage from an artifact's. Two domains
// hashing identical bytes cannot collide.
const obsDomain = "kept/observation"

// Observations is the directory content-addressed measurements live in, one
// file per measurement. See docs/layout.md.
const Observations = "observations"

// Observation is a measurement, as it is recorded.
//
// It is addressed by what it measured — the gatherer and its version, the args,
// the world, the scope digest, and the data — and it records when it was first
// seen. Nothing else is on it, and three absences are the decision rather than
// an omission:
//
//   - **No writer and no seq.** Which machine wrote this stops mattering once
//     observations are keyed by world: the same tree measured by the same
//     gatherer is the same observation, whoever ran it, which is what content
//     addressing already asserts. A writer would also make two runs write
//     different bytes for one measurement, which is the property docs/layout.md
//     claims. See docs/decision/0013.
//   - **No error.** A failed gather leaves no observation at all — that is what
//     makes the verdict unknown rather than not-kept. An error field would make
//     a failure a kind of observation, which is the collapse the three-valued
//     verdict exists to prevent. It survives as an in-process value on
//     model.Observation and never reaches here. See docs/decision/0004.
//   - **At is not addressed.** A timestamp in the address would mean no two
//     runs ever produce one file, and the retention window would fill with
//     identical measurements. So At means first-seen and the first writer wins.
//
// That last one has a consequence worth stating where it will be read: **a
// promise's freshness cannot come from its observation**, because At never
// advances for an unchanging world. It comes from the truth stream's heartbeat.
// Anyone about to refresh this timestamp is breaking content addressing to fix
// a problem the heartbeat already solves.
type Observation struct {
	// V is the shape this record was written under.
	V int `json:"v"`
	// At is when this measurement was first seen, not when it was last made.
	At time.Time `json:"at"`

	// Gatherer is name and version, "tests@2". The version is addressed
	// because a gatherer's bug fix must not reuse pre-fix observations.
	Gatherer string `json:"gatherer"`
	// Args is what the gatherer was asked for. It is what the observation
	// cache is keyed by, per docs/decision/0014, so two command gathers
	// differing only in what they ran do not collide.
	Args map[string]any `json:"args,omitempty"`
	// World is the tree this was measured against.
	World model.World `json:"world"`
	// Scope is the digest of the paths the promise named, per
	// docs/decision/0033. It is part of what the gatherer saw, and having it
	// on the record is what makes the record verifiable by re-hashing its own
	// fields.
	Scope string `json:"scope"`
	// Data is the fields the gatherer declared, and only those.
	Data map[string]any `json:"data"`
}

// MaxObservation bounds the encoded record.
//
// It is the layer that bounds what the absence of redaction can cost, and it is
// here rather than deferred with redaction because the two are not the same
// kind of thing: redaction needs a detector and can only ever be best-effort,
// while a cap is a comparison that always works. A well-formed extractor must
// not be able to write a hundred megabytes into a permanent, tracked,
// append-only record. See docs/decision/0023 and 0034.
//
// Exceeding it is a gather failure, so the verdict is unknown — a cap that
// reported not-kept would be the confusion the three-valued verdict exists to
// prevent — and the receipt says which bound and by how much.
const MaxObservation = 1 << 20

// TooLarge is a payload over one of the two bounds decision 0034 requires. It
// names which bound and by how much, because that is what a gather receipt has
// to say for the refusal to be diagnosable.
type TooLarge struct {
	Bound string
	Size  int
	Limit int
}

func (e *TooLarge) Error() string {
	return fmt.Sprintf("%s is %d bytes, which is %d over the %d-byte bound",
		e.Bound, e.Size, e.Size-e.Limit, e.Limit)
}

// Corrupt is a stored observation whose bytes do not produce the address they
// are filed under.
//
// Observations get docs/decision/0018's completeness guarantee for free and
// better than a stream does: a partial write is detectable by hashing what is
// there, so the file is discarded rather than quarantined as a line.
type Corrupt struct {
	Address string
	Why     string
}

func (e *Corrupt) Error() string { return fmt.Sprintf("observation %s: %s", e.Address, e.Why) }

// Address is what this observation is filed under: the digest of a preimage
// over what it measured.
//
// The encoding is decision 0031's, so it is deterministic and unambiguous. The
// text rule is **verbatim**, which is 0034's sharp edge: 0022's rules exist
// because a person wrote a file and a checkout may rewrite it, and neither half
// is true of a gatherer's output. Normalizing it would make "FAIL\ttrailing "
// and "FAIL\ttrailing" one measurement, and under content addressing that is
// not a wrong digest — it is the second measurement being discarded as a
// duplicate of the first.
func (o Observation) Address() string {
	enc := canonical.NewEncoder(fmt.Sprintf("%s\nobs:%d\n", obsDomain, ObsVersion), canonical.Verbatim)
	enc.Text(o.Gatherer)
	enc.Value(fields(o.Args))
	enc.Value(worldFields(o.World))
	enc.Text(o.Scope)
	enc.Value(fields(o.Data))
	return canonical.Algorithm + ":" + enc.Digest()
}

// fields is what a map contributes to the preimage: the values as the record
// stores them.
//
// An absent map and an empty one are the same thing, because a gatherer that
// took no arguments and one that took an empty map measured the same world the
// same way, and two files for it would be two answers to one question.
//
// The round trip through JSON is not tidiness. Decision 0034 requires an
// observation to be verifiable by re-deriving its address from its own fields,
// and a record is read back through a decoder that does not know an integer
// from a float. Addressing the values as the record carries them is what makes
// the check on read the same computation as the address on write; addressing
// the in-process values instead would make every stored observation fail its
// own verification. It is not a hash over serialized JSON, which
// docs/decision/0031 rejects for its key order and its number formatting — the
// encoding is still 0031's, over values JSON round-trips without changing.
func fields(m map[string]any) map[string]any {
	if len(m) == 0 {
		return map[string]any{}
	}
	line, err := json.Marshal(m)
	if err != nil {
		return m
	}
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.UseNumber()
	var out map[string]any
	if err := decoder.Decode(&out); err != nil {
		return m
	}
	return adopt(out).(map[string]any)
}

// adopt turns what the decoder produced back into the small domain the encoder
// has a tag for. A json.Number is an integer if it reads as one, which is the
// same answer the decoder gives on the way out.
func adopt(v any) any {
	switch t := v.(type) {
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return i
		}
		if f, err := t.Float64(); err == nil {
			return f
		}
		return t.String()
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, value := range t {
			out[k] = adopt(value)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, value := range t {
			out[i] = adopt(value)
		}
		return out
	default:
		return v
	}
}

// worldFields encodes the world as a map, with every key present whether or not
// git filled it in.
//
// All five are addressed, which is what decision 0034 writes. Remote, ref and
// commit are provenance in 0033's sense — they are filled in from git where git
// is present and are not what makes one tree one tree — so a workspace measured
// with and without git present addresses two ways. That is stated here rather
// than quietly narrowed, because narrowing it is an obs: bump and not an edit,
// and because in this increment nothing fills them in at all.
func worldFields(w model.World) map[string]any {
	return map[string]any{
		"remote": w.Remote,
		"ref":    w.Ref,
		"commit": w.Commit,
		"tree":   w.Tree,
		"thread": w.Thread,
	}
}

// ObservationPath is where an address is filed: observations/sha256/ab/cdef…,
// keyed by the observation's own content, so two runs that observe the same
// thing write identical bytes to one path and two that observe different things
// write to different ones. See docs/layout.md.
func ObservationPath(address string) (string, error) {
	algorithm, digest, found := strings.Cut(address, ":")
	if !found || algorithm == "" || len(digest) < 3 {
		return "", fmt.Errorf("%q is not an observation address", address)
	}
	return path.Join(Observations, algorithm, digest[:2], digest[2:]), nil
}

// Observe records a measurement, and returns the address it is filed under.
//
// The first writer wins. A second run measuring the same thing finds the path
// taken and leaves it alone, which is what keeps At meaning first-seen and what
// makes "two runs that observe the same thing write identical bytes to one
// path" true rather than aspirational.
//
// It records what has already happened: the gatherer ran, and this is what it
// returned. See docs/decision/0018.
func (l *Ledger) Observe(o Observation) (string, error) {
	o.V = ObsVersion
	if o.At.IsZero() {
		o.At = time.Now().UTC()
	}
	if o.Gatherer == "" {
		return "", errors.New("an observation names no gatherer, so nothing says what measured it")
	}
	if o.World.Tree == "" {
		// A measurement that does not name the tree it was made against cannot
		// be backfilled: nothing else records it, and prior() folds over
		// exactly that history. See docs/decision/0014.
		return "", errors.New("an observation names no world, and a record written without one cannot be backfilled")
	}

	address := o.Address()
	where, err := ObservationPath(address)
	if err != nil {
		return "", err
	}

	line, err := json.Marshal(o)
	if err != nil {
		return "", err
	}
	if len(line) > MaxObservation {
		return "", &TooLarge{Bound: "the encoded observation", Size: len(line), Limit: MaxObservation}
	}

	if held, err := l.store.Read(where); err == nil {
		if _, bad := decodeObservation(address, held); bad == nil {
			return address, nil
		}
		// What is there does not hash to where it is filed, so it is a torn
		// write and not somebody else's answer. Replacing it is the discard
		// docs/decision/0018 asks for.
	} else if !errors.Is(err, ErrNoStream) {
		return "", err
	}

	if err := l.declare(); err != nil {
		return "", err
	}
	if err := l.store.Put(where, append(line, '\n')); err != nil {
		return "", err
	}
	l.run.Observations++
	return address, nil
}

// ReadObservation returns one stored measurement, verifying that its bytes
// produce the address it is filed under.
func ReadObservation(store Store, address string) (Observation, error) {
	where, err := ObservationPath(address)
	if err != nil {
		return Observation{}, err
	}
	src, err := store.Read(where)
	if err != nil {
		return Observation{}, err
	}
	return decodeObservation(address, src)
}

func decodeObservation(address string, src []byte) (Observation, error) {
	var o Observation
	if err := json.Unmarshal(src, &o); err != nil {
		return Observation{}, &Corrupt{Address: address, Why: "does not parse: " + err.Error()}
	}
	switch {
	case o.V == 0:
		return Observation{}, &Corrupt{Address: address, Why: "no envelope version, so nothing says which shape this is"}
	case o.V > ObsVersion:
		return Observation{}, &Corrupt{Address: address, Why: fmt.Sprintf("obs:%d, and this build reads obs:%d", o.V, ObsVersion)}
	case o.Address() != address:
		return Observation{}, &Corrupt{Address: address, Why: "its own fields do not produce the address it is filed under"}
	}
	return o, nil
}

// Record is what a gatherer's in-process observation becomes on its way to the
// ledger. It is a narrowing rather than a copy: the id is the address and is
// computed here, and the error does not travel, because a failed gather leaves
// no observation.
func Record(o model.Observation) Observation {
	return Observation{
		V:        ObsVersion,
		At:       o.At,
		Gatherer: o.Gatherer,
		Args:     o.Args,
		World:    o.World,
		Scope:    o.ScopeDigest,
		Data:     o.Data,
	}
}
