package ids

import (
	"strings"
	"testing"
)

func TestMintIsWellFormed(t *testing.T) {
	id := Mint()
	if len(id) != Len {
		t.Fatalf("length %d, want %d", len(id), Len)
	}
	if id != strings.ToLower(id) {
		t.Fatalf("%q is not lowercase", id)
	}
	for i, c := range id {
		if !strings.ContainsRune(strings.ToLower(alphabet), c) {
			t.Fatalf("byte %d of %q is %q, outside the alphabet", i, id, c)
		}
	}
	for _, c := range "ilou" {
		if strings.ContainsRune(id, c) {
			t.Fatalf("%q contains %q, which Crockford base32 excludes", id, c)
		}
	}
}

// TestMintUsesEveryPosition is the check that would have caught a Mint that
// returned the same id every time, or one that filled only its low bytes: both
// pass a length assertion, and both would let two concurrent runs write records
// no one can tell apart.
func TestMintUsesEveryPosition(t *testing.T) {
	const n = 4096
	seen := make(map[string]bool, n)
	varies := make([]map[byte]bool, Len)
	for i := range varies {
		varies[i] = map[byte]bool{}
	}
	for range n {
		id := Mint()
		if seen[id] {
			t.Fatalf("Mint returned %q twice in %d draws", id, n)
		}
		seen[id] = true
		for i := 0; i < Len; i++ {
			varies[i][id[i]] = true
		}
	}
	// The leading character holds 3 bits of a 130-bit rendering of 128, so it
	// spans 8 values; every other position spans 32. Assert most of each.
	for i, v := range varies {
		want := 24
		if i == 0 {
			want = 6
		}
		if len(v) < want {
			t.Errorf("position %d took only %d distinct values in %d draws, want >= %d", i, len(v), n, want)
		}
	}
}

func TestShortDoesNotPad(t *testing.T) {
	if got := Short("abc"); got != "abc" {
		t.Fatalf("Short of a short id = %q", got)
	}
	if got := Short(Mint()); len(got) != 8 {
		t.Fatalf("Short = %q, want 8 characters", got)
	}
}
