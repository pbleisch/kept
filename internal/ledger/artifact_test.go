package ledger

import (
	"errors"
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/run"
)

func TestAnArtifactIsAddressedAsExactBytes(t *testing.T) {
	first := []byte("line\r\nline\n")
	second := []byte("line\nline\n")
	if AddressArtifact(first) == AddressArtifact(second) {
		t.Fatal("two different packet or transcript byte sequences share an address")
	}

	s := store(t)
	l := Open(s, run.Start("test", nil))
	ref, err := l.PutArtifact(SessionTranscriptArtifact, first)
	if err != nil {
		t.Fatal(err)
	}
	held, err := ReadArtifact(s, ref)
	if err != nil {
		t.Fatal(err)
	}
	if string(held) != string(first) {
		t.Fatalf("artifact = %q, want exact bytes %q", held, first)
	}
}

func TestAnArtifactReferenceRejectsUnknownKindsAndBadAddresses(t *testing.T) {
	address := AddressArtifact([]byte("packet"))
	for _, tc := range []struct {
		kind    ArtifactKind
		address string
	}{
		{"unreviewed.blob", address},
		{ThreadPacketArtifact, "sha256:too-short"},
		{ThreadPacketArtifact, "md5:" + strings.Repeat("a", 32)},
		{ThreadPacketArtifact, "sha256:" + strings.Repeat("A", 64)},
	} {
		if _, err := ArtifactPath(tc.kind, tc.address); err == nil {
			t.Errorf("ArtifactPath(%q, %q) accepted an uncommitted path shape", tc.kind, tc.address)
		}
	}
}

func TestAChangedArtifactIsNotEvidenceForItsReceipt(t *testing.T) {
	s := store(t)
	l := Open(s, run.Start("test", nil))
	ref, err := l.PutArtifact(ThreadPacketArtifact, []byte("exact packet"))
	if err != nil {
		t.Fatal(err)
	}
	where, err := ArtifactPath(ref.Kind, ref.Address)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put(where, []byte("edited packet")); err != nil {
		t.Fatal(err)
	}

	var corrupt *CorruptArtifact
	if _, err := ReadArtifact(s, ref); !errors.As(err, &corrupt) {
		t.Fatalf("edited artifact read as %v, want an address failure", err)
	}
}

func TestArtifactReferenceCarriesItsVersionAndSize(t *testing.T) {
	s := store(t)
	l := Open(s, run.Start("test", nil))
	ref, err := l.PutArtifact(ThreadPacketArtifact, []byte("packet"))
	if err != nil {
		t.Fatal(err)
	}
	if ref.V != ArtifactVersion || ref.Size != len("packet") {
		t.Fatalf("artifact reference = %+v, want v=%d and exact size", ref, ArtifactVersion)
	}

	ref.V = ArtifactVersion + 1
	var corrupt *CorruptArtifact
	if _, err := ReadArtifact(s, ref); !errors.As(err, &corrupt) {
		t.Fatalf("unknown artifact version read as %v", err)
	}
}
