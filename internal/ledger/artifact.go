package ledger

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"path"
	"strings"
)

// ArtifactVersion identifies the address and reference shape for durable
// payloads. Packet and transcript bytes cannot be reconstructed after a
// repair, so their references have to say which shape named them before the
// first one is written. See decisions 0020 and 0052.
const ArtifactVersion = 1

// ArtifactKind is deliberately closed until another irreproducible payload has
// a decided retention and redaction rule. It is not a generic blob store.
type ArtifactKind string

const (
	// ThreadPacketArtifact is the exact immutable byte sequence delivered to a
	// repair runtime.
	ThreadPacketArtifact ArtifactKind = "thread.packet"
	// SessionTranscriptArtifact is one redacted fragment of a runtime session,
	// retained in received-stream order by its receipt projections.
	SessionTranscriptArtifact ArtifactKind = "session.transcript"
)

// ArtifactRef is the small durable reference held in a receipt. The payload is
// addressed by its exact bytes; neither a timestamp nor a writer would belong
// in that address, because either would make the same packet or fragment grow
// a second copy.
type ArtifactRef struct {
	V       int          `json:"v"`
	Kind    ArtifactKind `json:"kind"`
	Address string       `json:"address"`
	Size    int          `json:"size"`
}

// ArtifactPath is where an artifact reference is filed on the ledger side ref:
// artifacts/<kind>/sha256/ab/cdef…. It rejects every kind and address this
// build has not committed to read, rather than letting a caller turn a durable
// payload path into an extension surface.
func ArtifactPath(kind ArtifactKind, address string) (string, error) {
	if !knownArtifactKind(kind) {
		return "", fmt.Errorf("%q is not a durable artifact kind this build knows", kind)
	}
	algorithm, digest, found := strings.Cut(address, ":")
	if !found || algorithm != "sha256" || len(digest) < 3 || !hexDigest(digest) {
		return "", fmt.Errorf("%q is not a sha256 artifact address", address)
	}
	return path.Join("artifacts", string(kind), algorithm, digest[:2], digest[2:]), nil
}

func knownArtifactKind(kind ArtifactKind) bool {
	return kind == ThreadPacketArtifact || kind == SessionTranscriptArtifact
}

func hexDigest(s string) bool {
	if len(s) != sha256.Size*2 {
		return false
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9') && !(r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

// AddressArtifact addresses the supplied bytes exactly as they will be stored.
// In particular it does not normalize text: a packet is the byte sequence the
// runtime received, and a transcript fragment is the redacted byte sequence
// the runtime emitted.
func AddressArtifact(content []byte) string {
	sum := sha256.Sum256(content)
	return fmt.Sprintf("sha256:%x", sum[:])
}

// CorruptArtifact is an object whose bytes do not produce the address in its
// receipt. It is not evidence, and a later write of the same address may
// replace it just as a torn observation may be replaced.
type CorruptArtifact struct {
	Ref ArtifactRef
	Why string
}

func (e *CorruptArtifact) Error() string {
	return fmt.Sprintf("artifact %s: %s", e.Ref.Address, e.Why)
}

// PutArtifact durably stores an irreproducible payload before its receipt can
// point at it. The first complete writer wins; a present object with different
// bytes is never silently accepted as the content the reference names.
func (l *Ledger) PutArtifact(kind ArtifactKind, content []byte) (ArtifactRef, error) {
	ref := ArtifactRef{
		V:       ArtifactVersion,
		Kind:    kind,
		Address: AddressArtifact(content),
		Size:    len(content),
	}
	where, err := ArtifactPath(kind, ref.Address)
	if err != nil {
		return ArtifactRef{}, err
	}

	if held, err := l.store.Read(where); err == nil {
		if err := verifyArtifact(ref, held); err == nil {
			return ref, nil
		}
		// A partial or otherwise corrupt object is not somebody else's evidence.
		// Replacing it is the completeness reaction for content-addressed data.
	} else if !errors.Is(err, ErrNoStream) {
		return ArtifactRef{}, err
	}

	if err := l.declare(); err != nil {
		return ArtifactRef{}, err
	}
	if err := l.store.Put(where, append([]byte(nil), content...)); err != nil {
		return ArtifactRef{}, err
	}
	return ref, nil
}

// ReadArtifact returns the exact stored payload after verifying its content
// address. A receipt therefore never turns a path-shaped string into evidence.
func ReadArtifact(store Store, ref ArtifactRef) ([]byte, error) {
	if ref.V == 0 {
		return nil, &CorruptArtifact{Ref: ref, Why: "no artifact reference version"}
	}
	if ref.V > ArtifactVersion {
		return nil, &CorruptArtifact{Ref: ref, Why: fmt.Sprintf("artifact version %d, and this build reads %d", ref.V, ArtifactVersion)}
	}
	where, err := ArtifactPath(ref.Kind, ref.Address)
	if err != nil {
		return nil, &CorruptArtifact{Ref: ref, Why: err.Error()}
	}
	content, err := store.Read(where)
	if err != nil {
		return nil, err
	}
	if err := verifyArtifact(ref, content); err != nil {
		return nil, err
	}
	return content, nil
}

func verifyArtifact(ref ArtifactRef, content []byte) error {
	if ref.Size != len(content) {
		return &CorruptArtifact{Ref: ref, Why: fmt.Sprintf("has %d bytes, and its reference says %d", len(content), ref.Size)}
	}
	if got := AddressArtifact(content); got != ref.Address {
		return &CorruptArtifact{Ref: ref, Why: "its bytes do not produce the address in the receipt"}
	}
	return nil
}
