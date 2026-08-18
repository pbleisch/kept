// Package ids mints identifiers for things named concurrently.
//
// Several runs of kept execute at once and none of them coordinate, so anything
// they each name — a run, a thread, a lease — needs an identifier that is
// unique without anyone allocating it. A counter is the tempting alternative and
// it collides silently: two runs opening a thread in the same second both call
// it t_9b2, and their records merge into one apparent thread.
//
// The rule the package exists to enforce is that a minted identifier carries no
// meaning. It is 128 random bits and nothing else. Host, pid, version and time
// belong in the record the identifier names, where they can be corrected,
// omitted or added to; encoded into the identifier they would be unfixable,
// parseable, and — for a hostname in a tracked repository — leaked.
//
// Identifiers minted here are opaque. They are not sortable by time, and
// ordering does not need them to be: every record carries its own event time,
// and decision 0011 requires derived reads to fold by that.
package ids

import (
	"crypto/rand"
	"encoding/binary"
	"strings"
)

// alphabet is Crockford base32: no I, L, O or U, so an identifier read aloud or
// retyped from a terminal does not turn into a different one.
const alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// Len is the length of a minted identifier. 128 bits in base32.
const Len = 26

// Mint returns a fresh identifier. It draws from crypto/rand and panics if the
// system entropy source fails, because every durable write depends on this
// value being distinct and there is no weaker identifier worth continuing with.
func Mint() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("kept: no entropy to mint an id: " + err.Error())
	}
	hi := binary.BigEndian.Uint64(b[0:8])
	lo := binary.BigEndian.Uint64(b[8:16])

	out := make([]byte, Len)
	for i := Len - 1; i >= 0; i-- {
		out[i] = alphabet[lo&31]
		lo = lo>>5 | hi<<59
		hi >>= 5
	}
	return strings.ToLower(string(out))
}

// Short returns the prefix used in logs, tables and console output. Records
// always carry the full identifier; nothing matches on a Short value.
func Short(id string) string {
	if len(id) < 8 {
		return id
	}
	return id[:8]
}
