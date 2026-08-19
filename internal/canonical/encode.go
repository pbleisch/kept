package canonical

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
)

// The preimage is a tagged, length-prefixed encoding of the values that bind.
// Two properties are required of it and neither is negotiable, because a
// ratification recorded under it is permanent:
//
//   - **Deterministic.** Two machines reading one file produce identical bytes.
//     That rules out map iteration order and any float formatting that varies.
//   - **Unambiguous.** No two different values encode to the same bytes. That is
//     what the length prefixes are for: without them {"ab": "c"} and {"a": "bc"}
//     would be indistinguishable, and an author could change a promise into a
//     different promise without dropping its standing.
//
// It is not a serialization format. Nothing reads it back, nothing writes it to
// disk, and it never appears in a signature. It exists to be hashed.
const (
	tagNil   = 'n'
	tagBool  = 'b'
	tagInt   = 'i'
	tagUint  = 'u'
	tagFloat = 'f'
	tagText  = 's'
	tagList  = 'l'
	tagMap   = 'm'

	// present and absent distinguish a field the author wrote as empty from one
	// they did not write at all. They are different content and must be
	// different bytes.
	present = '+'
	absent  = '-'
)

// TextRule is what happens to a string on its way into a preimage. The
// encoding is one thing and the text rule is another, and separating them is
// the point rather than a convenience: decision 0034 reuses this encoding for
// an observation's address and must not reuse [Normalized].
type TextRule func(string) string

// Normalized is decision 0022's five rules, and it is right for an artifact: a
// person wrote the file, a checkout may rewrite it, and a CRLF working copy
// must not de-ratify a corpus.
func Normalized(s string) string { return Normalize(s) }

// Verbatim hashes text as it was measured. It is what decision 0034 requires of
// an observation, and reaching for [Normalized] there is the mistake that
// decision exists to prevent: neither half of 0022's argument is true of a
// gatherer's output, and normalizing it collides measurements that genuinely
// differ — trailing whitespace in a log, a tool that emits CRLF and one that
// does not, how many newlines a program actually printed, and a filename a tool
// printed back in NFD.
//
// Observations are addressed by content and deduplicated by path, so the
// consequence of such a collision is not a wrong digest. It is the second
// measurement being discarded as a duplicate of the first, which is the stale
// answer decision 0014 was written to prevent.
func Verbatim(s string) string { return s }

// Encoder builds one preimage. A caller opens it with the domain the digest is
// for and the rule its text is encoded under, feeds it values, and hashes what
// comes out.
//
// The domain carries its own version — norm: for a binding hash, obs: for a
// measurement — so two records hashed under different rules cannot collide even
// where the content is identical, and so a reader can never mistake one for the
// other. See docs/decision/0031 and 0034.
type Encoder struct {
	b    bytes.Buffer
	text TextRule
}

// NewEncoder starts a preimage. The domain is written first and whole, newline
// and version included, so nothing after it can be read as part of it.
func NewEncoder(domain string, text TextRule) *Encoder {
	e := &Encoder{text: text}
	e.b.WriteString(domain)
	return e
}

// Value appends one value.
func (e *Encoder) Value(v any) { e.encode(v) }

// Text appends one string under this encoder's text rule. Field names, map
// keys, a body and a link target all pass through here, so a preimage never
// mixes two rules.
func (e *Encoder) Text(s string) { e.encodeText(s) }

// List opens a list of n items, which the caller then appends. It exists so a
// caller with a heterogeneous sequence — a hash list's names paired with
// present or absent — can write the count itself.
func (e *Encoder) List(n int) {
	e.b.WriteByte(tagList)
	e.b.WriteString(strconv.Itoa(n))
	e.b.WriteByte(':')
}

// Present and Absent mark whether the field just named was written at all. A
// field the author did not write and one they wrote as empty are different
// content, and without the mark deleting a line and blanking one would be
// indistinguishable.
func (e *Encoder) Present() { e.b.WriteByte(present) }
func (e *Encoder) Absent()  { e.b.WriteByte(absent) }

// Bytes is the preimage. Nothing parses it; it exists to be hashed.
func (e *Encoder) Bytes() []byte { return e.b.Bytes() }

// Digest is the preimage's sha256, lowercase hex.
func (e *Encoder) Digest() string {
	sum := sha256.Sum256(e.b.Bytes())
	return hex.EncodeToString(sum[:])
}

// encode appends the canonical encoding of one value.
//
// The value comes from a YAML decoder, a JSON decoder or a gatherer's data map,
// so it is one of: nil, a bool, an integer in whichever width the decoder
// chose, a float, a string, a list, or a map with keys of either shape.
// Anything else is rendered through fmt as text — which is lossy in principle
// and unreachable in practice, and is preferred to silently encoding two
// different values identically.
func (e *Encoder) encode(v any) {
	switch t := v.(type) {
	case nil:
		e.b.WriteByte(tagNil)

	case bool:
		e.b.WriteByte(tagBool)
		if t {
			e.b.WriteByte('1')
		} else {
			e.b.WriteByte('0')
		}

	case int:
		e.encodeInt(int64(t))
	case int8:
		e.encodeInt(int64(t))
	case int16:
		e.encodeInt(int64(t))
	case int32:
		e.encodeInt(int64(t))
	case int64:
		e.encodeInt(t)

	case uint:
		e.encodeUint(uint64(t))
	case uint8:
		e.encodeUint(uint64(t))
	case uint16:
		e.encodeUint(uint64(t))
	case uint32:
		e.encodeUint(uint64(t))
	case uint64:
		e.encodeUint(t)

	case float32:
		e.encodeFloat(float64(t))
	case float64:
		e.encodeFloat(t)

	case string:
		e.encodeText(t)

	case []any:
		e.List(len(t))
		for _, item := range t {
			e.encode(item)
		}

	case map[string]any:
		e.encodeMap(t)
	case map[any]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[fmt.Sprint(k)] = val
		}
		e.encodeMap(out)

	default:
		e.encodeText(fmt.Sprint(t))
	}
}

// encodeInt writes a signed integer in its shortest decimal form, so that an
// integer and the same integer from a decoder that chose a wider type encode
// identically. Negative zero does not survive strconv and does not need to.
func (e *Encoder) encodeInt(v int64) {
	e.b.WriteByte(tagInt)
	e.b.WriteString(strconv.FormatInt(v, 10))
	e.b.WriteByte(';')
}

// encodeUint carries its own tag rather than widening into int64, because a
// uint64 above the signed range would otherwise have nowhere to go.
func (e *Encoder) encodeUint(v uint64) {
	e.b.WriteByte(tagUint)
	e.b.WriteString(strconv.FormatUint(v, 10))
	e.b.WriteByte(';')
}

// encodeFloat uses the shortest representation that round-trips, which is the
// one thing about float formatting that is stable across platforms and
// versions. A float in a binding field is unusual; encoding it wrongly would
// still be permanent.
func (e *Encoder) encodeFloat(v float64) {
	e.b.WriteByte(tagFloat)
	e.b.WriteString(strconv.FormatFloat(v, 'g', -1, 64))
	e.b.WriteByte(';')
}

// encodeText is where the text rule is applied, so every string in one
// preimage — a field name, a map key, a body, a path — passes through the same
// rule. Applying it to values only would leave a corpus whose keys were written
// on a different platform hashing differently for no reason a reader could see.
func (e *Encoder) encodeText(s string) {
	n := e.text(s)
	e.b.WriteByte(tagText)
	e.b.WriteString(strconv.Itoa(len(n)))
	e.b.WriteByte(':')
	e.b.WriteString(n)
}

// encodeMap sorts by the encoded key rather than by the key itself, so the
// ordering is a property of the bytes being hashed and not of Go's collation.
func (e *Encoder) encodeMap(m map[string]any) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return e.encoded(keys[i]) < e.encoded(keys[j]) })

	e.b.WriteByte(tagMap)
	e.b.WriteString(strconv.Itoa(len(keys)))
	e.b.WriteByte(':')
	for _, k := range keys {
		e.encodeText(k)
		e.encode(m[k])
	}
}

func (e *Encoder) encoded(s string) string {
	sub := &Encoder{text: e.text}
	sub.encodeText(s)
	return sub.b.String()
}
