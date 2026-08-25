package thread

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pbleisch/kept/internal/ids"
	"github.com/pbleisch/kept/internal/ledger"
)

// ID names one repair thread. It is minted rather than composed from an
// artifact, run or timestamp: the receipt contains those facts separately.
type ID string

// MintID returns a fresh opaque thread id.
func MintID() ID { return ID(ids.Mint()) }

// Redaction states whether the bytes that became durable were altered before
// the write. A silent redaction would make a reviewer mistake altered evidence
// for the runtime's original evidence; see decision 0023.
type Redaction struct {
	Occurred bool `json:"occurred"`
	Matches  int  `json:"matches"`
}

// NativeEvent is one runtime event retained in its native vocabulary. Payload
// is raw JSON rather than a synthetic common message: adapters may project from
// it, but it remains the evidence when their mapping is incomplete.
type NativeEvent struct {
	Ref            string          `json:"ref"`
	Runtime        Runtime         `json:"runtime"`
	AdapterVersion string          `json:"adapter_version"`
	Type           string          `json:"type"`
	Payload        json.RawMessage `json:"payload"`
	Redaction      Redaction       `json:"redaction"`
}

// TranscriptFragment is a common projection from one native event. Received
// order is receipt order, not map or file order.
type TranscriptFragment struct {
	NativeRef string             `json:"native_ref"`
	Artifact  ledger.ArtifactRef `json:"artifact"`
	Redaction Redaction          `json:"redaction"`
}

// CostUpdate is a runtime-reported monetary update. V1 deliberately does not
// invent a cross-runtime usage conversion; a future conversion needs its own
// evidence and version before it can be written.
type CostUpdate struct {
	NativeRef string `json:"native_ref"`
	Currency  string `json:"currency"`
	Amount    string `json:"amount"`
}

// TurnUpdate is the runtime's own cumulative turn or iteration count.
type TurnUpdate struct {
	NativeRef string `json:"native_ref"`
	Count     uint64 `json:"count"`
}

// LifecycleEvent is a fact observed by the parent process, not a runtime
// report. It is intentionally smaller than native runtime event vocabulary.
type LifecycleEvent string

const (
	Started  LifecycleEvent = "started"
	TimedOut LifecycleEvent = "timed_out"
	Killed   LifecycleEvent = "killed"
	Exited   LifecycleEvent = "exited"
)

// Exit is the actual reaped exit. Exactly one of Code and Signal is present.
type Exit struct {
	Code   *int   `json:"code,omitempty"`
	Signal string `json:"signal,omitempty"`
}

// Lifecycle is the shared parent projection. Timeout is configured before a
// launch and retained only on the timed-out event, so a partial receipt says
// both that the deadline elapsed and which bound was in force.
type Lifecycle struct {
	Event   LifecycleEvent `json:"event"`
	Timeout string         `json:"timeout,omitempty"`
	Exit    *Exit          `json:"exit,omitempty"`
}

// Entry is one append-only receipt item. Exactly one body is present. The
// envelope is supplied by ledger so its writer and sequence remain monotonic
// with every other record the parent run wrote.
type Entry struct {
	ledger.Envelope
	Thread     ID                  `json:"thread"`
	Packet     *ledger.ArtifactRef `json:"packet,omitempty"`
	Native     *NativeEvent        `json:"native,omitempty"`
	Transcript *TranscriptFragment `json:"transcript,omitempty"`
	Cost       *CostUpdate         `json:"cost,omitempty"`
	Turn       *TurnUpdate         `json:"turn,omitempty"`
	Lifecycle  *Lifecycle          `json:"lifecycle,omitempty"`
	Anchor     *Anchor             `json:"anchor,omitempty"`
}

// Anchor is the commit a repair produced, and the promise it was produced for.
//
// It is what a land is handed. Everything else in a receipt is evidence about
// the session; this is the work itself, addressed the one way git addresses
// work. The ref holding it against gc is not a field here: it is a pure
// function of the thread id this entry already carries, and a name stored
// beside the thing it is derived from is a second place for one answer to live.
// See [ledger.ThreadRef] and docs/decision/0067.
type Anchor struct {
	// Commit is what the session left, committed on top of Parent, so the
	// commit's own diff against its parent is the change a person reviewed.
	Commit string `json:"commit"`
	// Parent is the commit the box was cut from.
	Parent string `json:"parent"`
	// Promise is the artifact the repair was authorized by, carried because a
	// land has to know which promise's scope bounds this change and a receipt
	// otherwise names no artifact at all.
	Promise string `json:"promise"`
}

// Receipt is a parent-owned handle on one thread's receipt stream.
type Receipt struct {
	ledger *ledger.Ledger
	id     ID
	now    func() time.Time

	mu     sync.Mutex
	native map[string]bool
}

// NewReceipt starts an empty append-only receipt. It does not write an intent:
// the first item is appended only after the corresponding packet, process or
// event actually exists.
func NewReceipt(l *ledger.Ledger, id ID) (*Receipt, error) {
	if l == nil {
		return nil, fmt.Errorf("a receipt has no ledger")
	}
	if _, err := ledger.ThreadPath(string(id)); err != nil {
		return nil, err
	}
	return &Receipt{
		ledger: l, id: id, now: func() time.Time { return time.Now().UTC() }, native: map[string]bool{},
	}, nil
}

// ID returns the thread this receipt belongs to.
func (r *Receipt) ID() ID { return r.id }

// RecordPacket records a packet only after its content-addressed bytes were
// stored. A packet cannot be substituted with a later corpus reconstruction.
func (r *Receipt) RecordPacket(ref ledger.ArtifactRef) error {
	if ref.Kind != ledger.ThreadPacketArtifact {
		return fmt.Errorf("receipt packet is %q, not a thread packet artifact", ref.Kind)
	}
	return r.append(ledger.ThreadPacket, Entry{Thread: r.id, Packet: &ref})
}

// RecordAnchor records the commit a repair produced, after the commit exists
// and its ref is written. Recording it first would name an object that might
// never be created, which is the one thing an append-only record may not do.
func (r *Receipt) RecordAnchor(anchor Anchor) error {
	if why := anchorWhy(anchor); why != "" {
		return fmt.Errorf("receipt anchor %s", why)
	}
	return r.append(ledger.ThreadAnchor, Entry{Thread: r.id, Anchor: &anchor})
}

// AdapterReceipt is all an adapter can append. Lifecycle methods are absent on
// purpose: timeout, kill and exit are parent-observed facts, never supplied by
// a runtime.
type AdapterReceipt interface {
	Native(typ string, payload json.RawMessage, redaction Redaction) (string, error)
	Transcript(nativeRef string, content []byte, redaction Redaction) (ledger.ArtifactRef, error)
	Cost(nativeRef, currency, amount string) error
	Turn(nativeRef string, count uint64) error
}

type adapterReceipt struct {
	receipt        *Receipt
	runtime        Runtime
	adapterVersion string
}

// ForAdapter binds the native receipt fields to the statically selected
// runtime. It does not accept a runtime outside the closed V1 set.
func (r *Receipt) ForAdapter(runtime Runtime, adapterVersion string) (AdapterReceipt, error) {
	if !runtime.known() {
		return nil, fmt.Errorf("%q is not a repair runtime this build knows", runtime)
	}
	if adapterVersion == "" {
		return nil, fmt.Errorf("%s adapter records no adapter version", runtime)
	}
	return adapterReceipt{receipt: r, runtime: runtime, adapterVersion: adapterVersion}, nil
}

func (a adapterReceipt) Native(typ string, payload json.RawMessage, redaction Redaction) (string, error) {
	event := NativeEvent{
		Ref:            ids.Mint(),
		Runtime:        a.runtime,
		AdapterVersion: a.adapterVersion,
		Type:           typ,
		Payload:        append(json.RawMessage(nil), payload...),
		Redaction:      redaction,
	}
	if why := nativeWhy(event); why != "" {
		return "", fmt.Errorf("native receipt: %s", why)
	}
	if err := a.receipt.append(ledger.ThreadNative, Entry{Thread: a.receipt.id, Native: &event}); err != nil {
		return "", err
	}
	a.receipt.rememberNative(event.Ref)
	return event.Ref, nil
}

func (a adapterReceipt) Transcript(nativeRef string, content []byte, redaction Redaction) (ledger.ArtifactRef, error) {
	if nativeRef == "" {
		return ledger.ArtifactRef{}, fmt.Errorf("transcript projection names no native event")
	}
	if !a.receipt.hasNative(nativeRef) {
		return ledger.ArtifactRef{}, fmt.Errorf("transcript projection names an event this receipt did not receive")
	}
	if why := redactionWhy(redaction); why != "" {
		return ledger.ArtifactRef{}, fmt.Errorf("transcript redaction: %s", why)
	}
	ref, err := a.receipt.ledger.PutArtifact(ledger.SessionTranscriptArtifact, content)
	if err != nil {
		return ledger.ArtifactRef{}, err
	}
	entry := Entry{Thread: a.receipt.id, Transcript: &TranscriptFragment{
		NativeRef: nativeRef, Artifact: ref, Redaction: redaction,
	}}
	if err := a.receipt.append(ledger.ThreadTranscript, entry); err != nil {
		return ledger.ArtifactRef{}, err
	}
	return ref, nil
}

func (a adapterReceipt) Cost(nativeRef, currency, amount string) error {
	if !a.receipt.hasNative(nativeRef) {
		return fmt.Errorf("cost projection names an event this receipt did not receive")
	}
	entry := Entry{Thread: a.receipt.id, Cost: &CostUpdate{
		NativeRef: nativeRef, Currency: currency, Amount: amount,
	}}
	return a.receipt.append(ledger.ThreadCost, entry)
}

func (a adapterReceipt) Turn(nativeRef string, count uint64) error {
	if !a.receipt.hasNative(nativeRef) {
		return fmt.Errorf("turn projection names an event this receipt did not receive")
	}
	entry := Entry{Thread: a.receipt.id, Turn: &TurnUpdate{NativeRef: nativeRef, Count: count}}
	return a.receipt.append(ledger.ThreadTurn, entry)
}

// Started, TimedOut, Killed and Exited are intentionally methods on Receipt,
// not AdapterReceipt. They can only be called by the parent dispatcher.
func (r *Receipt) Started() error {
	return r.lifecycle(Lifecycle{Event: Started})
}

func (r *Receipt) TimedOut(timeout time.Duration) error {
	if timeout <= 0 {
		return fmt.Errorf("a timeout receipt records no configured deadline")
	}
	return r.lifecycle(Lifecycle{Event: TimedOut, Timeout: timeout.String()})
}

func (r *Receipt) Killed() error { return r.lifecycle(Lifecycle{Event: Killed}) }

func (r *Receipt) Exited(exit Exit) error {
	return r.lifecycle(Lifecycle{Event: Exited, Exit: &exit})
}

func (r *Receipt) lifecycle(event Lifecycle) error {
	return r.append(ledger.ThreadLifecycle, Entry{Thread: r.id, Lifecycle: &event})
}

func (r *Receipt) append(kind ledger.Kind, entry Entry) error {
	if why := entryWhy(kind, entry); why != "" {
		return fmt.Errorf("thread receipt: %s", why)
	}
	return r.ledger.AppendThread(string(r.id), kind, r.now(), func(envelope ledger.Envelope) ([]byte, error) {
		entry.Envelope = envelope
		return json.Marshal(entry)
	})
}

func (r *Receipt) rememberNative(ref string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.native[ref] = true
}

func (r *Receipt) hasNative(ref string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.native[ref]
}

func entryWhy(kind ledger.Kind, entry Entry) string {
	if entry.Thread == "" {
		return "names no thread"
	}
	bodies := 0
	if entry.Packet != nil {
		bodies++
	}
	if entry.Native != nil {
		bodies++
	}
	if entry.Transcript != nil {
		bodies++
	}
	if entry.Cost != nil {
		bodies++
	}
	if entry.Turn != nil {
		bodies++
	}
	if entry.Lifecycle != nil {
		bodies++
	}
	if entry.Anchor != nil {
		bodies++
	}
	if bodies != 1 {
		return "has zero or more than one event body"
	}
	switch kind {
	case ledger.ThreadPacket:
		if entry.Packet == nil || entry.Packet.Kind != ledger.ThreadPacketArtifact {
			return "packet event has no thread packet reference"
		}
	case ledger.ThreadNative:
		if entry.Native == nil {
			return "native event has no native body"
		}
		if why := nativeWhy(*entry.Native); why != "" {
			return why
		}
	case ledger.ThreadTranscript:
		if entry.Transcript == nil {
			return "transcript event has no transcript body"
		}
		if entry.Transcript.NativeRef == "" {
			return "transcript projection names no native event"
		}
		if entry.Transcript.Artifact.Kind != ledger.SessionTranscriptArtifact {
			return "transcript projection has no transcript artifact"
		}
		if why := redactionWhy(entry.Transcript.Redaction); why != "" {
			return why
		}
	case ledger.ThreadCost:
		if entry.Cost == nil || entry.Cost.NativeRef == "" || entry.Cost.Currency == "" || entry.Cost.Amount == "" {
			return "cost projection is incomplete"
		}
	case ledger.ThreadTurn:
		if entry.Turn == nil || entry.Turn.NativeRef == "" || entry.Turn.Count == 0 {
			return "turn projection is incomplete"
		}
	case ledger.ThreadLifecycle:
		if entry.Lifecycle == nil {
			return "lifecycle event has no lifecycle body"
		}
		if why := lifecycleWhy(*entry.Lifecycle); why != "" {
			return why
		}
	case ledger.ThreadAnchor:
		if entry.Anchor == nil {
			return "anchor event has no anchor body"
		}
		if why := anchorWhy(*entry.Anchor); why != "" {
			return why
		}
	default:
		return fmt.Sprintf("kind %q is not a thread receipt event", kind)
	}
	return ""
}

// LatestAnchor is the last anchor a receipt holds.
//
// The last rather than the first: a receipt is append-only and a thread that
// anchored twice anchored the second one on purpose. In practice there is one,
// because a repair dispatches one session.
func LatestAnchor(entries []Entry) (Anchor, bool) {
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Kind == ledger.ThreadAnchor && entries[i].Anchor != nil {
			return *entries[i].Anchor, true
		}
	}
	return Anchor{}, false
}

func anchorWhy(anchor Anchor) string {
	switch {
	case !objectID(anchor.Commit):
		return "names no commit"
	case !objectID(anchor.Parent):
		return "names no parent"
	case anchor.Commit == anchor.Parent:
		// A commit identical to its parent is a repair that changed nothing,
		// and an anchor for it would hand a land an empty change to gate.
		return "names a commit that is its own parent"
	case anchor.Promise == "":
		return "names no promise"
	}
	return ""
}

func objectID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func nativeWhy(event NativeEvent) string {
	switch {
	case event.Ref == "":
		return "native event has no event reference"
	case !event.Runtime.known():
		return "native event names an unknown runtime"
	case event.AdapterVersion == "":
		return "native event has no adapter version"
	case event.Type == "":
		return "native event has no native type"
	case !json.Valid(event.Payload):
		return "native event payload is not JSON"
	}
	return redactionWhy(event.Redaction)
}

func redactionWhy(redaction Redaction) string {
	switch {
	case redaction.Matches < 0:
		return "redaction has a negative match count"
	case redaction.Occurred && redaction.Matches == 0:
		return "redaction says it occurred but names no replaced match"
	case !redaction.Occurred && redaction.Matches != 0:
		return "redaction names replaced matches without saying it occurred"
	}
	return ""
}

func lifecycleWhy(event Lifecycle) string {
	switch event.Event {
	case Started, Killed:
		if event.Timeout != "" || event.Exit != nil {
			return fmt.Sprintf("%s lifecycle event carries an outcome it did not observe", event.Event)
		}
	case TimedOut:
		if event.Timeout == "" || event.Exit != nil {
			return "timeout lifecycle event is incomplete"
		}
	case Exited:
		if event.Timeout != "" || event.Exit == nil {
			return "exit lifecycle event is incomplete"
		}
		if (event.Exit.Code == nil) == (event.Exit.Signal == "") {
			return "exit lifecycle event names neither or both code and signal"
		}
	default:
		return fmt.Sprintf("unknown lifecycle event %q", event.Event)
	}
	return ""
}

// ReadReceipt reads a receipt in event order, never in union-merge file order.
// An unreadable line is quarantined while the surrounding evidence remains
// inspectable; a partial interrupted session must not hide earlier updates.
func ReadReceipt(store ledger.Store, id ID) ([]Entry, []ledger.Quarantined, error) {
	stream, err := ledger.ThreadPath(string(id))
	if err != nil {
		return nil, nil, err
	}
	src, err := store.Read(stream)
	if err != nil {
		return nil, nil, err
	}

	type candidate struct {
		entry Entry
		line  int
		raw   string
	}
	var candidates []candidate
	var quarantined []ledger.Quarantined
	for lineNo, raw := range bytes.Split(src, []byte{'\n'}) {
		line := bytes.TrimSpace(raw)
		if len(line) == 0 {
			continue
		}
		var entry Entry
		if err := json.Unmarshal(line, &entry); err != nil {
			quarantined = append(quarantined, ledger.Quarantined{Stream: stream, Line: lineNo + 1, Raw: string(line), Why: "not a receipt: " + err.Error()})
			continue
		}
		if why := readWhy(id, entry); why != "" {
			quarantined = append(quarantined, ledger.Quarantined{Stream: stream, Line: lineNo + 1, Raw: string(line), Why: why})
			continue
		}
		candidates = append(candidates, candidate{entry: entry, line: lineNo + 1, raw: string(line)})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i].entry.Envelope, candidates[j].entry.Envelope
		if c := a.At.Compare(b.At); c != 0 {
			return c < 0
		}
		if a.Writer != b.Writer {
			return a.Writer < b.Writer
		}
		return a.Seq < b.Seq
	})

	seenNative := map[string]bool{}
	entries := make([]Entry, 0, len(candidates))
	for _, candidate := range candidates {
		entry := candidate.entry
		if why := referenceWhy(entry, seenNative); why != "" {
			quarantined = append(quarantined, ledger.Quarantined{Stream: stream, Line: candidate.line, Raw: candidate.raw, Why: why})
			continue
		}
		if entry.Native != nil {
			seenNative[entry.Native.Ref] = true
		}
		entries = append(entries, entry)
	}
	return entries, quarantined, nil
}

func referenceWhy(entry Entry, seenNative map[string]bool) string {
	if entry.Native != nil && seenNative[entry.Native.Ref] {
		return "native event reference was already used in this receipt"
	}
	var ref string
	switch {
	case entry.Transcript != nil:
		ref = entry.Transcript.NativeRef
	case entry.Cost != nil:
		ref = entry.Cost.NativeRef
	case entry.Turn != nil:
		ref = entry.Turn.NativeRef
	}
	if ref != "" && !seenNative[ref] {
		return "projection names a native event not retained earlier in this receipt"
	}
	return ""
}

func readWhy(id ID, entry Entry) string {
	switch {
	case entry.V == 0:
		return "no receipt envelope version"
	case entry.V > ledger.Version:
		return fmt.Sprintf("receipt envelope version %d, and this build reads %d", entry.V, ledger.Version)
	case entry.Writer == "":
		return "no receipt writer"
	case entry.At.IsZero():
		return "no receipt event time"
	case entry.Thread != id:
		return "receipt entry names a different thread than its path"
	}
	return entryWhy(entry.Kind, entry)
}

// ReceiptText is used only by diagnostics and tests; it deliberately does not
// turn a receipt into a current-state summary.
func ReceiptText(entries []Entry) string {
	parts := make([]string, 0, len(entries))
	for _, entry := range entries {
		parts = append(parts, string(entry.Kind))
	}
	return strings.Join(parts, ",")
}
