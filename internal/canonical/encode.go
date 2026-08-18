package canonical

import (
	"bytes"
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

// encode appends the canonical encoding of one value.
//
// The value comes from a YAML decoder, so it is one of: nil, a bool, an integer
// in whichever width the decoder chose, a float, a string, a list, or a map
// with keys of either shape. Anything else is rendered through fmt as text —
// which is lossy in principle and unreachable in practice, and is preferred to
// silently encoding two different values identically.
func encode(b *bytes.Buffer, v any) {
	switch t := v.(type) {
	case nil:
		b.WriteByte(tagNil)

	case bool:
		b.WriteByte(tagBool)
		if t {
			b.WriteByte('1')
		} else {
			b.WriteByte('0')
		}

	case int:
		encodeInt(b, int64(t))
	case int8:
		encodeInt(b, int64(t))
	case int16:
		encodeInt(b, int64(t))
	case int32:
		encodeInt(b, int64(t))
	case int64:
		encodeInt(b, t)

	case uint:
		encodeUint(b, uint64(t))
	case uint8:
		encodeUint(b, uint64(t))
	case uint16:
		encodeUint(b, uint64(t))
	case uint32:
		encodeUint(b, uint64(t))
	case uint64:
		encodeUint(b, t)

	case float32:
		encodeFloat(b, float64(t))
	case float64:
		encodeFloat(b, t)

	case string:
		encodeText(b, t)

	case []any:
		b.WriteByte(tagList)
		b.WriteString(strconv.Itoa(len(t)))
		b.WriteByte(':')
		for _, item := range t {
			encode(b, item)
		}

	case map[string]any:
		encodeMap(b, t)
	case map[any]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[fmt.Sprint(k)] = val
		}
		encodeMap(b, out)

	default:
		encodeText(b, fmt.Sprint(t))
	}
}

// encodeInt writes a signed integer in its shortest decimal form, so that an
// integer and the same integer from a decoder that chose a wider type encode
// identically. Negative zero does not survive strconv and does not need to.
func encodeInt(b *bytes.Buffer, v int64) {
	b.WriteByte(tagInt)
	b.WriteString(strconv.FormatInt(v, 10))
	b.WriteByte(';')
}

// encodeUint carries its own tag rather than widening into int64, because a
// uint64 above the signed range would otherwise have nowhere to go.
func encodeUint(b *bytes.Buffer, v uint64) {
	b.WriteByte(tagUint)
	b.WriteString(strconv.FormatUint(v, 10))
	b.WriteByte(';')
}

// encodeFloat uses the shortest representation that round-trips, which is the
// one thing about float formatting that is stable across platforms and
// versions. A float in a binding field is unusual; encoding it wrongly would
// still be permanent.
func encodeFloat(b *bytes.Buffer, v float64) {
	b.WriteByte(tagFloat)
	b.WriteString(strconv.FormatFloat(v, 'g', -1, 64))
	b.WriteByte(';')
}

// encodeText is where normalization happens, so every string in the preimage —
// a field name, a map key, a body, a link target — passes through the same five
// rules. Applying them to values only would leave a corpus whose keys were
// written on a different platform hashing differently for no reason a reader
// could see.
func encodeText(b *bytes.Buffer, s string) {
	n := Normalize(s)
	b.WriteByte(tagText)
	b.WriteString(strconv.Itoa(len(n)))
	b.WriteByte(':')
	b.WriteString(n)
}

// encodeMap sorts by the encoded key rather than by the key itself, so the
// ordering is a property of the bytes being hashed and not of Go's collation.
func encodeMap(b *bytes.Buffer, m map[string]any) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return encoded(keys[i]) < encoded(keys[j]) })

	b.WriteByte(tagMap)
	b.WriteString(strconv.Itoa(len(keys)))
	b.WriteByte(':')
	for _, k := range keys {
		encodeText(b, k)
		encode(b, m[k])
	}
}

func encoded(s string) string {
	var b bytes.Buffer
	encodeText(&b, s)
	return b.String()
}
