package thread

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
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

// Undecodable is output a runtime wrote on its native stream that its adapter
// could not read as one native event: a line that is not an event, or the item
// the stream ended inside.
//
// It is not a [NativeEvent], and writing it as one would be the invented event
// decision 0052 refuses — a native entry carries the runtime's own type and a
// payload that is the runtime's own JSON, and bytes that do not decode have
// neither. It is not dropped either. The bytes are what the runtime wrote, a
// reader of a receipt that stops short has to be able to see where and why, and
// the moment they arrive is the only moment they can be kept. And it is not a
// failure of the dispatch: what a session did is evidence about the session,
// and whether the promise holds is measured from the tree it left. See
// docs/decision/0084.
type Undecodable struct {
	Runtime        Runtime `json:"runtime"`
	AdapterVersion string  `json:"adapter_version"`
	// Why is the adapter's reason in its own words: the decoder's error, or the
	// field an event cannot be read without.
	Why string `json:"why"`
	// Unterminated says the stream ended inside this item, with no line end
	// after it. It is observed rather than inferred, and it says nothing about
	// why the stream ended: that is the lifecycle's, which records how the
	// process did.
	Unterminated bool               `json:"unterminated"`
	Artifact     ledger.ArtifactRef `json:"artifact"`
	Redaction    Redaction          `json:"redaction"`
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

// ModelUpdate is the runtime's own report of which model answered.
//
// What kept asked for is a different claim and is deliberately not here. kept
// names no model to Claude at all, so the two would disagree by construction,
// and a field named for what answered that sometimes held what was requested is
// the collapse this projection exists to prevent. A runtime that names no model
// writes none; nothing here may be reconstructed from a default, a
// configuration or a version. See docs/decision/0077.
type ModelUpdate struct {
	NativeRef string `json:"native_ref"`
	Model     string `json:"model"`
}

// Window is one usage window the runtime reported, under the runtime's own name
// for it.
//
// Utilization is the runtime's own decimal, retained as it was written, for the
// same reason [CostUpdate] retains an amount rather than a float: kept converts
// nothing it did not measure. It is a fraction, and it is deliberately not
// bounded above — a window in overage reads above one, and refusing that would
// turn a true reading into a missing one.
type Window struct {
	Name        string    `json:"name"`
	Utilization string    `json:"utilization"`
	ResetsAt    time.Time `json:"resets_at,omitzero"`
}

// WindowUpdate is a runtime's reading of the usage windows in force for the
// account it is answering on.
//
// It is the counter a budget would refuse on, and it is the runtime's own
// reading of something shared across every machine and every session on that
// account — including a person's interactive ones — so it needs no lock, no
// decrement and cannot drift. It is not a contract: `rate_limit_event` is one
// runtime's native vocabulary, and a build that renames it must produce no
// projection rather than a wrong one.
//
// Windows is never empty, because *no window reported* and *the window is
// empty* are different answers and only the first is a state a receipt can be
// in: it is said by holding no window entry at all, which is what [LatestWindow]
// reports. See docs/decision/0077.
type WindowUpdate struct {
	NativeRef string   `json:"native_ref"`
	Windows   []Window `json:"windows"`
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
	Model      *ModelUpdate        `json:"model,omitempty"`
	Window     *WindowUpdate       `json:"window,omitempty"`
	Lifecycle  *Lifecycle          `json:"lifecycle,omitempty"`
	Anchor     *Anchor             `json:"anchor,omitempty"`

	Measurement *Measurement `json:"measurement,omitempty"`

	// Brief is the input an adversarial pass was given, and Argument is what it
	// concluded. They are two entries rather than one for the reason the packet
	// and the anchor are: the first is recorded before a session that may be
	// killed, the second only after one that finished. See docs/decision/0081.
	Brief    *ledger.ArtifactRef `json:"brief,omitempty"`
	Argument *ArgumentRecord     `json:"argument,omitempty"`

	Undecodable *Undecodable `json:"undecodable,omitempty"`
	Executable  *Executable  `json:"executable,omitempty"`
	Stderr      *Stderr      `json:"stderr,omitempty"`

	// Segment is an index entry: it names one sealed span of this receipt,
	// and [ReadReceipt] replaces it with the entries the span holds. No reader
	// is ever handed one. See docs/decision/0087.
	Segment *ledger.Segment `json:"segment,omitempty"`
}

// Stderr is the end of what a runtime wrote on its error stream.
//
// It is what a session says when its native stream does not: a runtime that
// dies of something it can name names it here. It is the parent's record,
// written once the process is reaped, and the bytes are redacted by the
// runtime's own adapter exactly as its native events are before they become
// durable. Only the end is kept, because the end is where a runtime says why it
// stopped, and Omitted says how much came before it. See docs/decision/0085.
type Stderr struct {
	Artifact  ledger.ArtifactRef `json:"artifact"`
	Redaction Redaction          `json:"redaction"`
	Omitted   int64              `json:"omitted"`
}

// ArgumentRecord is what one adversarial pass concluded about this thread.
//
// **It declares no shape for a finding.** Not an omission: the pass's findings
// are unbounded and unslotted, because the failure it exists to reach is the
// one nobody thought to ask about, and every field here that graded, categorized
// or counted them would be a question somebody wrote down in advance — which is
// the ceiling docs/decision/0078 refuses. So what is durable is the argument's
// own words, whole and unparsed, and what is beside them is provenance: which
// evidence it read and what answered it.
//
// The words themselves are an artifact rather than a string in this entry, for
// the reason a transcript fragment is one: an argument is as long as it needs to
// be, and a receipt line is not where an unbounded payload belongs.
type ArgumentRecord struct {
	// Runtime and AdapterVersion are what answered. They are the parent's own
	// facts about the session it started, not the runtime's account of itself —
	// which model actually served the request is the thread_model projection's,
	// read from the runtime's own system/init and never from what kept asked
	// for. See docs/decision/0077.
	Runtime        Runtime `json:"runtime"`
	AdapterVersion string  `json:"adapter_version"`

	// Brief is the evidence this argument was reached from, by the address the
	// brief entry already named. A conclusion that did not name what it read
	// would be unattachable to the attempt it is about once a thread holds more
	// than one.
	Brief ledger.ArtifactRef `json:"brief"`

	// Artifact is where the argument's own words are stored.
	Artifact ledger.ArtifactRef `json:"artifact"`

	// Redaction says whether the words had a credential taken out of them,
	// exactly as a transcript fragment's does.
	Redaction Redaction `json:"redaction"`
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

// Side is which of a session's two sides a measurement was taken on.
//
// Two, because a repair is judged by a comparison and one measurement is not
// one. The words are the repair's own — [repair.Result] has carried Before and
// After since there was a re-derivation to carry — rather than a numbering,
// which would say when a record was written instead of what it is about.
type Side string

const (
	// Before is the promise measured in the box at the commit it was cut from,
	// which is the gap the packet states.
	Before Side = "before"
	// After is the promise measured in the box the session left behind, which
	// is the re-derivation the verdict comes from.
	After Side = "after"
)

func (s Side) known() bool { return s == Before || s == After }

// Measurement names one instrument's measurement of the promise in the box, by
// the address it is filed under in the observation stream.
//
// It is here because a box is discarded. The measurement itself is an ordinary
// observation and lives where every other observation lives — content-addressed,
// prunable, re-gatherable against the world it names — but *which* two
// measurements a repair was judged by is not derivable from anything else once
// the tree that produced them is gone. The receipt is the only record that both
// of these were about this session, and which side of it each was on.
//
// It carries no verdict. A predicate applied to an observation is derived
// wherever else kept asks that question, the packet already holds the ratified
// `hold_when` this repair was authorized by, and a verdict stored beside the
// measurement it was reached from is the cache that can disagree with its
// source. See docs/decision/0078.
type Measurement struct {
	// Side is which side of the session this was measured on.
	Side Side `json:"side"`
	// Name is the name the promise gathers this instrument under — the same key
	// its predicate reads as obs.<name>, and the same key the packet carries the
	// measurement under. Keyed rather than ordered, for docs/decision/0061's
	// reason: a promise with two instruments makes a position meaningless.
	Name string `json:"name"`
	// Address is where the measurement is filed in the observation stream.
	Address string `json:"address"`
}

// Receipt is a parent-owned handle on one thread's receipt stream.
type Receipt struct {
	ledger *ledger.Ledger
	id     ID
	now    func() time.Time

	mu           sync.Mutex
	native       map[string]bool
	undecodable  int
	unterminated bool
	started      bool
	executables  map[ExecutableMoment]Executable

	// appending serializes writes to this stream. The adapter's stream and the
	// supervisor append from different goroutines — a deadline records its
	// timeout while events already read are still being retained — and the
	// ledger's sequence is not safe to advance from two at once.
	appending sync.Mutex
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
	return r.appendSealed(ledger.ThreadPacket, Entry{Thread: r.id, Packet: &ref})
}

// RecordAnchor records the commit a repair produced, after the commit exists
// and its ref is written. Recording it first would name an object that might
// never be created, which is the one thing an append-only record may not do.
func (r *Receipt) RecordAnchor(anchor Anchor) error {
	if why := anchorWhy(anchor); why != "" {
		return fmt.Errorf("receipt anchor %s", why)
	}
	return r.appendSealed(ledger.ThreadAnchor, Entry{Thread: r.id, Anchor: &anchor})
}

// RecordMeasurement records that one instrument measured the promise in this
// box, after the measurement was filed in the observation stream.
//
// After, for the same reason RecordAnchor comes after the commit exists: a
// receipt naming an address nothing was written to is a record of something
// that did not happen. It is the parent's, not an adapter's — a runtime reports
// what it did and never what the promise measures, which is
// policy:the-agents-report-is-never-an-input-to-the-verdict at the one place a
// runtime could otherwise reach.
func (r *Receipt) RecordMeasurement(measurement Measurement) error {
	if why := measurementEntryWhy(measurement); why != "" {
		return fmt.Errorf("receipt measurement %s", why)
	}
	return r.appendSealed(ledger.ThreadMeasurement, Entry{Thread: r.id, Measurement: &measurement})
}

// RecordBrief records the brief an adversarial pass was given, only after its
// content-addressed bytes were stored. It is the packet's counterpart, and it is
// what delimits one pass inside an episode: the entries from an input record to
// the next belong to one session.
func (r *Receipt) RecordBrief(ref ledger.ArtifactRef) error {
	if ref.Kind != ledger.ThreadBriefArtifact {
		return fmt.Errorf("receipt brief is %q, not a thread brief artifact", ref.Kind)
	}
	return r.appendSealed(ledger.ThreadBrief, Entry{Thread: r.id, Brief: &ref})
}

// RecordArgument records what an adversarial pass concluded, after its words
// were stored.
//
// After the session, and only after: a pass killed at its deadline leaves its
// transcript and no argument, which is the same guarantee an anchor gives a
// repair that produced nothing. A conclusion written before the session ended
// would be a record of something that had not happened.
//
// It is the parent's, not an adapter's. A runtime reports what it did and never
// what kept makes of it, which is
// policy:the-agents-report-is-never-an-input-to-the-verdict at the one place a
// runtime could otherwise reach — and this is a finding about an act rather
// than a verdict about a promise, so the policy is untouched either way.
func (r *Receipt) RecordArgument(argument ArgumentRecord) error {
	if why := argumentWhy(argument); why != "" {
		return fmt.Errorf("receipt argument %s", why)
	}
	return r.appendSealed(ledger.ThreadArgument, Entry{Thread: r.id, Argument: &argument})
}

// AdapterReceipt is all an adapter can append. Lifecycle methods are absent on
// purpose: timeout, kill and exit are parent-observed facts, never supplied by
// a runtime.
type AdapterReceipt interface {
	Native(typ string, payload json.RawMessage, redaction Redaction) (string, error)
	Undecodable(content []byte, reason string, unterminated bool, redaction Redaction) error
	Transcript(nativeRef string, content []byte, redaction Redaction) (ledger.ArtifactRef, error)
	Cost(nativeRef, currency, amount string) error
	Turn(nativeRef string, count uint64) error
	Model(nativeRef, model string) error
	Window(nativeRef string, windows []Window) error
	// Seal files what the receipt's journal holds, and is what an adapter's
	// stream does when the runtime's output ends. It records nothing.
	Seal() error
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

// Undecodable retains bytes the adapter could not read as an event. The bytes
// are stored before the entry names them, for the reason a transcript fragment
// is: a receipt never points at an object that was never written.
func (a adapterReceipt) Undecodable(content []byte, reason string, unterminated bool, redaction Redaction) error {
	if why := redactionWhy(redaction); why != "" {
		return fmt.Errorf("undecodable redaction: %s", why)
	}
	ref, err := a.receipt.ledger.JournalArtifact(string(a.receipt.id), ledger.SessionUndecodableArtifact, content)
	if err != nil {
		return err
	}
	item := Undecodable{
		Runtime:        a.runtime,
		AdapterVersion: a.adapterVersion,
		Why:            reason,
		Unterminated:   unterminated,
		Artifact:       ref,
		Redaction:      redaction,
	}
	if why := undecodableWhy(item); why != "" {
		return fmt.Errorf("undecodable receipt: %s", why)
	}
	if err := a.receipt.append(ledger.ThreadUndecodable, Entry{Thread: a.receipt.id, Undecodable: &item}); err != nil {
		return err
	}
	a.receipt.rememberUndecodable(unterminated)
	return nil
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
	ref, err := a.receipt.ledger.JournalArtifact(string(a.receipt.id), ledger.SessionTranscriptArtifact, content)
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

func (a adapterReceipt) Seal() error { return a.receipt.Seal() }

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

func (a adapterReceipt) Model(nativeRef, model string) error {
	if !a.receipt.hasNative(nativeRef) {
		return fmt.Errorf("model projection names an event this receipt did not receive")
	}
	entry := Entry{Thread: a.receipt.id, Model: &ModelUpdate{NativeRef: nativeRef, Model: model}}
	return a.receipt.append(ledger.ThreadModel, entry)
}

func (a adapterReceipt) Window(nativeRef string, windows []Window) error {
	if !a.receipt.hasNative(nativeRef) {
		return fmt.Errorf("window projection names an event this receipt did not receive")
	}
	entry := Entry{Thread: a.receipt.id, Window: &WindowUpdate{
		NativeRef: nativeRef, Windows: slices.Clone(windows),
	}}
	return a.receipt.append(ledger.ThreadWindow, entry)
}

// RecordExecutable records a runtime's executable as the parent observed it.
// It is the parent's, not an adapter's: a runtime reports what it did, never
// what file it was launched from.
func (r *Receipt) RecordExecutable(observed Executable) error {
	if why := executableWhy(observed); why != "" {
		return fmt.Errorf("receipt %s", why)
	}
	if err := r.append(ledger.ThreadExecutable, Entry{Thread: r.id, Executable: &observed}); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.executables == nil {
		r.executables = map[ExecutableMoment]Executable{}
	}
	r.executables[observed.Moment] = observed
	return nil
}

// Replaced reports the executable this receipt recorded at launch and after
// the reap, and whether they are different files. False when either was not
// recorded, because nobody looked is not *it stayed the same*.
func (r *Receipt) Replaced() (launch, exit Executable, replaced bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	launch, launched := r.executables[AtLaunch]
	exit, exited := r.executables[AtExit]
	return launch, exit, launched && exited && !launch.SameFile(exit)
}

// RecordStderr retains the end of a runtime's error stream, already redacted,
// after the process is reaped. An empty stream writes nothing: a receipt that
// holds no stderr entry is a runtime that said nothing there, and an entry
// holding nothing would be indistinguishable from one whose bytes were lost.
func (r *Receipt) RecordStderr(content []byte, redaction Redaction, omitted int64) error {
	if len(content) == 0 {
		return nil
	}
	ref, err := r.ledger.JournalArtifact(string(r.id), ledger.SessionStderrArtifact, content)
	if err != nil {
		return err
	}
	body := Stderr{Artifact: ref, Redaction: redaction, Omitted: omitted}
	if why := stderrWhy(body); why != "" {
		return fmt.Errorf("receipt %s", why)
	}
	return r.append(ledger.ThreadStderr, Entry{Thread: r.id, Stderr: &body})
}

func stderrWhy(body Stderr) string {
	switch {
	case body.Artifact.Kind != ledger.SessionStderrArtifact:
		return "stderr record has no stderr artifact"
	case body.Artifact.Size == 0:
		return "stderr record retains nothing"
	case body.Omitted < 0:
		return "stderr record omits a negative number of bytes"
	}
	return redactionWhy(body.Redaction)
}

// Started, TimedOut, Killed and Exited are intentionally methods on Receipt,
// not AdapterReceipt. They can only be called by the parent dispatcher.
func (r *Receipt) Started() error {
	if err := r.lifecycle(Lifecycle{Event: Started}); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.started = true
	return nil
}

// Launched reports whether this receipt recorded a runtime that started. It is
// what says a box holds a session's work: a dispatch that failed before a
// process existed left nothing in the box, and one that failed after left
// whatever the session did.
func (r *Receipt) Launched() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.started
}

func (r *Receipt) TimedOut(timeout time.Duration) error {
	if timeout <= 0 {
		return fmt.Errorf("a timeout receipt records no configured deadline")
	}
	return r.lifecycle(Lifecycle{Event: TimedOut, Timeout: timeout.String()})
}

func (r *Receipt) Killed() error { return r.lifecycle(Lifecycle{Event: Killed}) }

// Exited is the session's last lifecycle entry, so it is sealed with
// everything the journal held before it.
func (r *Receipt) Exited(exit Exit) error {
	return r.appendSealed(ledger.ThreadLifecycle, Entry{Thread: r.id, Lifecycle: &Lifecycle{Event: Exited, Exit: &exit}})
}

func (r *Receipt) lifecycle(event Lifecycle) error {
	return r.append(ledger.ThreadLifecycle, Entry{Thread: r.id, Lifecycle: &event})
}

func (r *Receipt) append(kind ledger.Kind, entry Entry) error {
	if why := entryWhy(kind, entry); why != "" {
		return fmt.Errorf("thread receipt: %s", why)
	}
	r.appending.Lock()
	defer r.appending.Unlock()
	return r.ledger.AppendThread(string(r.id), kind, r.now(), func(envelope ledger.Envelope) ([]byte, error) {
		entry.Envelope = envelope
		return json.Marshal(entry)
	})
}

// appendSealed appends an entry and seals the receipt at once. It is for the
// parent's records written outside a running session — a packet or brief
// before it, an anchor, measurement or argument after it — and for the entry
// that ends one, so that whatever reads the receipt next, in this process or
// another, finds it in the store. See docs/decision/0087.
func (r *Receipt) appendSealed(kind ledger.Kind, entry Entry) error {
	if err := r.append(kind, entry); err != nil {
		return err
	}
	return r.Seal()
}

// Seal files whatever this receipt's journal holds into the store. Nothing
// reads a receipt from its journal, so a receipt is readable up to its last
// seal; it records nothing of its own and is a no-op when nothing is unsealed.
func (r *Receipt) Seal() error {
	r.appending.Lock()
	defer r.appending.Unlock()
	return r.ledger.SealThread(string(r.id))
}

func (r *Receipt) rememberNative(ref string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.native[ref] = true
}

func (r *Receipt) rememberUndecodable(unterminated bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.undecodable++
	r.unterminated = r.unterminated || unterminated
}

// Undecoded reports how many items this receipt retained as undecodable, and
// whether the stream ended inside one. A caller reporting a session says so
// rather than presenting a receipt that stops short as a whole one.
func (r *Receipt) Undecoded() (items int, unterminated bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.undecodable, r.unterminated
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
	if entry.Model != nil {
		bodies++
	}
	if entry.Window != nil {
		bodies++
	}
	if entry.Lifecycle != nil {
		bodies++
	}
	if entry.Anchor != nil {
		bodies++
	}
	if entry.Measurement != nil {
		bodies++
	}
	if entry.Brief != nil {
		bodies++
	}
	if entry.Argument != nil {
		bodies++
	}
	if entry.Undecodable != nil {
		bodies++
	}
	if entry.Executable != nil {
		bodies++
	}
	if entry.Stderr != nil {
		bodies++
	}
	if entry.Segment != nil {
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
	case ledger.ThreadModel:
		if entry.Model == nil || entry.Model.NativeRef == "" || entry.Model.Model == "" {
			return "model projection is incomplete"
		}
	case ledger.ThreadWindow:
		if entry.Window == nil || entry.Window.NativeRef == "" {
			return "window projection is incomplete"
		}
		if why := windowWhy(*entry.Window); why != "" {
			return why
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
	case ledger.ThreadMeasurement:
		if entry.Measurement == nil {
			return "measurement event has no measurement body"
		}
		if why := measurementEntryWhy(*entry.Measurement); why != "" {
			return why
		}
	case ledger.ThreadBrief:
		if entry.Brief == nil || entry.Brief.Kind != ledger.ThreadBriefArtifact {
			return "brief event has no thread brief reference"
		}
	case ledger.ThreadArgument:
		if entry.Argument == nil {
			return "argument event has no argument body"
		}
		if why := argumentWhy(*entry.Argument); why != "" {
			return why
		}
	case ledger.ThreadUndecodable:
		if entry.Undecodable == nil {
			return "undecodable event has no undecodable body"
		}
		if why := undecodableWhy(*entry.Undecodable); why != "" {
			return why
		}
	case ledger.ThreadExecutable:
		if entry.Executable == nil {
			return "executable event has no executable body"
		}
		if why := executableWhy(*entry.Executable); why != "" {
			return why
		}
	case ledger.ThreadStderr:
		if entry.Stderr == nil {
			return "stderr event has no stderr body"
		}
		if why := stderrWhy(*entry.Stderr); why != "" {
			return why
		}
	case ledger.ThreadSegment:
		if entry.Segment == nil || entry.Segment.Artifact.Kind != ledger.ThreadSegmentArtifact {
			return "segment event has no thread segment reference"
		}
	default:
		return fmt.Sprintf("kind %q is not a thread receipt event", kind)
	}
	return ""
}

// argumentWhy holds an argument entry to naming what answered, what it read and
// where its words are.
//
// It is asked at the write and again on the way back in, so the two ends cannot
// drift. What it does *not* ask about is the argument's content: there is no
// minimum length, no required conclusion and no shape for a finding, because the
// half this records is the one that is not enumerated.
func argumentWhy(argument ArgumentRecord) string {
	switch {
	case !argument.Runtime.known():
		return "names no runtime"
	case argument.AdapterVersion == "":
		return "names no adapter version"
	case argument.Brief.Kind != ledger.ThreadBriefArtifact:
		return "names no brief it was reached from"
	case argument.Artifact.Kind != ledger.ThreadArgumentArtifact:
		return "has no argument artifact"
	}
	return redactionWhy(argument.Redaction)
}

// undecodableWhy holds an undecodable entry to naming what wrote the bytes, why
// they did not decode, and where they are. It is asked at the write and on the
// way back in.
func undecodableWhy(item Undecodable) string {
	switch {
	case !item.Runtime.known():
		return "undecodable output names an unknown runtime"
	case item.AdapterVersion == "":
		return "undecodable output has no adapter version"
	case strings.TrimSpace(item.Why) == "":
		return "undecodable output says nothing about why it did not decode"
	case item.Artifact.Kind != ledger.SessionUndecodableArtifact:
		return "undecodable output has no undecodable artifact"
	}
	return redactionWhy(item.Redaction)
}

// Arguments is every adversarial pass a receipt holds, in the order they were
// written.
//
// Every one rather than the last: a thread is an episode, an episode may hold
// several attempts, and each attempt's pass argued about the attempt in front
// of it. A reader wanting one takes the last; a reader wanting to see whether a
// pass keeps making the same point needs them all, and that reading is exactly
// the one docs/decision/0079 says becomes a derivation somebody writes.
func Arguments(entries []Entry) []ArgumentRecord {
	var out []ArgumentRecord
	for _, entry := range entries {
		if entry.Kind == ledger.ThreadArgument && entry.Argument != nil {
			out = append(out, *entry.Argument)
		}
	}
	return out
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

// LatestModel is the last model a receipt reports answered, and the bool is the
// distinction between *the runtime named one* and *nobody recorded one*.
//
// The last rather than the first: a receipt is append-only, and a runtime that
// reported a second model reported it on purpose.
func LatestModel(entries []Entry) (ModelUpdate, bool) {
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Kind == ledger.ThreadModel && entries[i].Model != nil {
			return *entries[i].Model, true
		}
	}
	return ModelUpdate{}, false
}

// LatestWindow is the last window reading a receipt holds.
//
// The bool is the whole point, and a caller that drops it has made the mistake
// this projection is shaped to prevent. False is *this session reported no
// window at all* — a self-hosted runtime reports none, and so does a build that
// renamed the event kept reads. It is not a window at zero, and a budget that
// read it as one would refuse nothing on the day it should have refused
// everything. Nothing has answered is not an answer.
func LatestWindow(entries []Entry) (WindowUpdate, bool) {
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Kind == ledger.ThreadWindow && entries[i].Window != nil {
			return *entries[i].Window, true
		}
	}
	return WindowUpdate{}, false
}

// windowWhy refuses the readings that would be wrong rather than missing.
//
// A reading of no windows is the one this exists for: a runtime that reports
// none must leave the receipt holding no window entry, because an entry saying
// nothing is indistinguishable from a window at zero once it is durable.
// Ascending order with no repeat is the second: the native reading is a JSON
// object, so an order taken from it is map iteration order, and one window read
// twice in a single event is two answers to one question.
func windowWhy(update WindowUpdate) string {
	if len(update.Windows) == 0 {
		return "window projection reports no window, and a runtime that reports none writes none"
	}
	for i, window := range update.Windows {
		if window.Name == "" {
			return "window projection reports a window with no name"
		}
		if i > 0 && update.Windows[i-1].Name >= window.Name {
			return "window projection reports its windows out of order or names one twice"
		}
		if !decimal(window.Utilization) {
			return fmt.Sprintf("window %q reports utilization %q, which is not a number", window.Name, window.Utilization)
		}
	}
	return ""
}

// decimal reports whether a retained literal is a number kept may keep as text.
// It is the same rule the cost amount is held to: a literal is retained rather
// than converted, and only a finite non-negative one is retained at all.
func decimal(literal string) bool {
	value, err := strconv.ParseFloat(strings.TrimSpace(literal), 64)
	return err == nil && !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}

// Measurements is every measurement a receipt names, in receipt order.
//
// All of them rather than the last, which is the opposite rule from
// [LatestAnchor] and [LatestModel] and is the right one here: those answer *what
// is in force*, and this answers *what was measured*. A promise with two
// instruments produces two entries per side, and folding them to one would drop
// the half that says what the repair must not have broken. See
// docs/decision/0061.
func Measurements(entries []Entry) []Measurement {
	var out []Measurement
	for _, entry := range entries {
		if entry.Kind == ledger.ThreadMeasurement && entry.Measurement != nil {
			out = append(out, *entry.Measurement)
		}
	}
	return out
}

// measurementEntryWhy refuses a measurement record that names nothing a reader could
// follow.
//
// The address is checked by asking the ledger where it would be filed, rather
// than by a pattern written here: two spellings of what an address looks like
// is two places one answer lives, and the one that decides is the one that
// stores it.
func measurementEntryWhy(measurement Measurement) string {
	switch {
	case !measurement.Side.known():
		return fmt.Sprintf("names side %q, which is neither side of a session", measurement.Side)
	case measurement.Name == "":
		return "names no instrument, so nothing says which of a promise's measurements this is"
	}
	if _, err := ledger.ObservationPath(measurement.Address); err != nil {
		return "names no measurement: " + err.Error()
	}
	return ""
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
//
// A segment entry is expanded in place into the lines its sealed span holds,
// and a line whose writer and sequence have already been read is dropped, so a
// span sealed twice — by a run and then by the next one recovering its journal
// — reads once. A receipt written before segments existed has none and reads
// exactly as it did. See docs/decision/0087.
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
		entry  Entry
		stream string
		line   int
		raw    string
	}
	var candidates []candidate
	var quarantined []ledger.Quarantined
	seen := map[ledger.Position]bool{}
	read := func(where string, lineNo int, line []byte) (Entry, bool) {
		var entry Entry
		if err := json.Unmarshal(line, &entry); err != nil {
			quarantined = append(quarantined, ledger.Quarantined{Stream: where, Line: lineNo, Raw: string(line), Why: "not a receipt: " + err.Error()})
			return Entry{}, false
		}
		if why := readWhy(id, entry); why != "" {
			quarantined = append(quarantined, ledger.Quarantined{Stream: where, Line: lineNo, Raw: string(line), Why: why})
			return Entry{}, false
		}
		return entry, true
	}
	keep := func(entry Entry, where string, lineNo int, line []byte) {
		at := ledger.Position{Writer: entry.Writer, Seq: entry.Seq}
		if seen[at] {
			return
		}
		seen[at] = true
		candidates = append(candidates, candidate{entry: entry, stream: where, line: lineNo, raw: string(line)})
	}
	for lineNo, raw := range bytes.Split(src, []byte{'\n'}) {
		line := bytes.TrimSpace(raw)
		if len(line) == 0 {
			continue
		}
		entry, ok := read(stream, lineNo+1, line)
		if !ok {
			continue
		}
		if entry.Segment == nil {
			keep(entry, stream, lineNo+1, line)
			continue
		}
		where, _ := ledger.ArtifactPath(entry.Segment.Artifact.Kind, entry.Segment.Artifact.Address)
		span, err := ledger.ReadArtifact(store, entry.Segment.Artifact)
		if err != nil {
			quarantined = append(quarantined, ledger.Quarantined{Stream: stream, Line: lineNo + 1, Raw: string(line), Why: "segment cannot be read: " + err.Error()})
			continue
		}
		for spanNo, raw := range bytes.Split(span, []byte{'\n'}) {
			line := bytes.TrimSpace(raw)
			if len(line) == 0 {
				continue
			}
			inner, ok := read(where, spanNo+1, line)
			if !ok {
				continue
			}
			if inner.Segment != nil {
				quarantined = append(quarantined, ledger.Quarantined{Stream: where, Line: spanNo + 1, Raw: string(line), Why: "a segment names another segment"})
				continue
			}
			keep(inner, where, spanNo+1, line)
		}
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
			quarantined = append(quarantined, ledger.Quarantined{Stream: candidate.stream, Line: candidate.line, Raw: candidate.raw, Why: why})
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
	case entry.Model != nil:
		ref = entry.Model.NativeRef
	case entry.Window != nil:
		ref = entry.Window.NativeRef
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

// Session is one dispatch inside a thread: the input record that opened it, and
// every entry written until the next input record.
//
// A thread is an episode and an episode holds more than one session — a rework
// appends to the thread it reworks, and an adversarial pass argues inside the
// episode it is about. So *which session wrote this* is a question a receipt has
// to be able to answer, and it is answered by a derivation over the record
// rather than by an ordinal field on ten body types: a number written in ten
// places is one answer with ten chances to disagree with itself, and a version
// bump for each. See docs/decision/0079.
type Session struct {
	// Kind is [ledger.ThreadPacket] for a repair and [ledger.ThreadBrief] for
	// an adversarial pass. It is what says which sort of session this was, and
	// it is the record's own word for it rather than an inference from what the
	// session went on to write.
	Kind ledger.Kind
	// Input is the immutable bytes that session received.
	Input ledger.ArtifactRef
	// Entries is everything written after that input record and before the
	// next, the input record itself excluded.
	Entries []Entry
}

// Sessions groups a receipt into the dispatches that produced it, in order.
//
// Entries before the first input record belong to no session and are dropped:
// the only ones there can be are a measurement filed before the launch, which is
// about the box rather than about a session, and a session cannot be claimed by
// an input that had not been recorded when it was written.
func Sessions(entries []Entry) []Session {
	var out []Session
	for _, entry := range entries {
		switch {
		case entry.Kind == ledger.ThreadPacket && entry.Packet != nil:
			out = append(out, Session{Kind: entry.Kind, Input: *entry.Packet})
		case entry.Kind == ledger.ThreadBrief && entry.Brief != nil:
			out = append(out, Session{Kind: entry.Kind, Input: *entry.Brief})
		case len(out) > 0:
			out[len(out)-1].Entries = append(out[len(out)-1].Entries, entry)
		}
	}
	return out
}
