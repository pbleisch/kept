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
	// ThreadBriefArtifact is the exact immutable byte sequence delivered to an
	// adversarial pass: one thread's ledger entry as that session received it.
	//
	// It is stored for the reason a packet is, and it is not the packet's
	// bytes. A brief carries the derived findings, the anchor's diff and the
	// session transcript as they stood when the pass read them, and the corpus
	// half of that moves — the ordinary next step after a finding is revising
	// the promise. A later reader rebuilding it would rebuild a different one.
	ThreadBriefArtifact ArtifactKind = "thread.brief"
	// ThreadArgumentArtifact is what one adversarial pass concluded, in the
	// words it wrote and unparsed by anything here.
	//
	// A session is not reproducible, so this is durable rather than derived —
	// policy:durability-follows-reproducibility, which is the same rule that
	// keeps a transcript and drops an observation. Nothing in this build reads
	// its bytes as anything but text: the pass's findings are unbounded and
	// unslotted, and a parser here would be the schema decision 0078 refuses.
	ThreadArgumentArtifact ArtifactKind = "thread.argument"
	// SessionUndecodableArtifact is output a runtime wrote on its native stream
	// that its adapter could not read as an event, redacted, as the bytes
	// arrived.
	//
	// An artifact rather than a string in the receipt line, because the bytes
	// are not JSON by definition and need not be UTF-8 — a stream torn inside a
	// multi-byte character is exactly the case this exists for — and a JSON
	// string would replace what it cannot hold without saying so. Retained for
	// the reason a transcript fragment is: a session cannot be re-run. See
	// docs/decision/0084.
	SessionUndecodableArtifact ArtifactKind = "session.undecodable"
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
	switch kind {
	case ThreadPacketArtifact, SessionTranscriptArtifact, ThreadBriefArtifact, ThreadArgumentArtifact, SessionUndecodableArtifact:
		return true
	}
	return false
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
