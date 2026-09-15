//go:build !windows

package confine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/thread"
)

// A kill proof is a proof only when the session it kills was still under way,
// and the receipt cannot be the thing that says so: the receipt is what the
// proof is about. So the parent keeps its own account of the runtime's stream,
// read off the pipe before the adapter sees a byte and timed against the
// deadline the supervisor actually started the runtime under.
//
// Measured, and the reason this exists: at a fixed 25-second deadline a live Pi
// session on a self-hosted thinking model emitted 190 to 260 native events every
// time, and in 4 runs of 10 not one of them was text. The first update arrives
// 10 to 14 seconds in, after the endpoint has processed the packet; the model
// then reasons in `thinking_delta` updates, and its first `text_delta` — the
// only update the adapter projects as a transcript — arrived between 17 and 24
// seconds in, which is either side of the deadline. A proof that demanded a
// transcript fragment was asserting how long the model thought, not what the
// receipt kept.

// piInterruptUpdates is how many streamed updates must have arrived before the
// deadline for the kill to count as one made mid-session. It is decision 0052's
// "a kill after multiple updates", with room in it: a stream that has started
// delivers updates by the hundred inside a second or two of its first.
const piInterruptUpdates = 8

// piArrival is one native line of a Pi RPC stream, as the parent read it.
type piArrival struct {
	At   time.Time
	Type string
	// Update is the assistantMessageEvent type a message_update carries —
	// text_delta, thinking_delta, toolcall_delta — and empty on anything else.
	Update string
	// Text is a text_delta that carries text, which is the one update the
	// adapter projects as a transcript fragment.
	Text bool
}

// piStreamAccount is everything the parent saw, and the deadline it saw it
// against.
type piStreamAccount struct {
	Started  time.Time
	Deadline time.Time
	Arrivals []piArrival
}

// before is what arrived before the deadline elapsed. Anything later arrived
// while the supervisor was already reacting to the deadline, and may or may not
// be in the receipt; nothing is claimed about it.
func (a piStreamAccount) before() []piArrival {
	var before []piArrival
	for _, arrival := range a.Arrivals {
		if arrival.At.Before(a.Deadline) {
			before = append(before, arrival)
		}
	}
	return before
}

func (a piStreamAccount) String() string {
	before := a.before()
	updates := map[string]int{}
	firstUpdate, firstText := "none", "none"
	for _, arrival := range before {
		since := arrival.At.Sub(a.Started).Round(time.Millisecond).String()
		if arrival.Type == "message_update" {
			updates[arrival.Update]++
			if firstUpdate == "none" {
				firstUpdate = since
			}
		}
		if arrival.Text && firstText == "none" {
			firstText = since
		}
	}
	return fmt.Sprintf("%d of %d native events arrived before the %v deadline; updates by kind %v; first update at %s, first text at %s",
		len(before), len(a.Arrivals), a.Deadline.Sub(a.Started).Round(time.Second), updates, firstUpdate, firstText)
}

// piPremise is whether a session was still under way when its deadline elapsed.
type piPremise int

const (
	// piUnderWay: the stream had started and had not ended, so a kill at the
	// deadline is a kill mid-session.
	piUnderWay piPremise = iota
	// piNotStarted: too few updates had arrived. A longer deadline can still
	// establish the premise.
	piNotStarted
	// piSettled: the session finished before its deadline. A longer deadline
	// cannot establish the premise, and a shorter one is where the stream has not
	// started.
	piSettled
)

// piInterruptPremise reads the parent's account, never the receipt, and says
// whether the kill it preceded was a kill mid-session.
func piInterruptPremise(a piStreamAccount) (piPremise, string) {
	updates := 0
	for _, arrival := range a.before() {
		switch arrival.Type {
		case "agent_end", "agent_settled":
			return piSettled, fmt.Sprintf("Pi emitted %s before the deadline, so the session had finished and a kill would not interrupt it: %s", arrival.Type, a)
		case "message_update":
			updates++
		}
	}
	if updates < piInterruptUpdates {
		return piNotStarted, fmt.Sprintf("%d streamed updates arrived before the deadline and the proof needs %d, so a kill would land before the session was under way: %s", updates, piInterruptUpdates, a)
	}
	return piUnderWay, ""
}

// piAnswered is whether Pi acknowledged the prompt command at all. Pi answers
// an RPC prompt before it asks a model for anything, so a session that never
// did is not an endpoint that was slow to stream.
func (a piStreamAccount) piAnswered() bool {
	for _, arrival := range a.Arrivals {
		if arrival.Type == "response" {
			return true
		}
	}
	return false
}

// piLostBeforeTheKill is decision 0052's property itself: every event the
// parent received before the deadline is in the receipt, in the order it
// arrived, and every text update among them was projected exactly once.
func piLostBeforeTheKill(a piStreamAccount, entries []thread.Entry) error {
	before := a.before()
	var natives []*thread.Entry
	for i := range entries {
		if entries[i].Native != nil {
			natives = append(natives, &entries[i])
		}
	}
	if len(natives) < len(before) {
		return fmt.Errorf("%d native events arrived before the deadline and the receipt holds %d", len(before), len(natives))
	}
	refs := map[string]bool{}
	text := 0
	for i, arrival := range before {
		if got := natives[i].Native.Type; got != arrival.Type {
			return fmt.Errorf("native event %d arrived as %s and the receipt holds %s in its place", i, arrival.Type, got)
		}
		refs[natives[i].Native.Ref] = true
		if arrival.Text {
			text++
		}
	}
	projected := 0
	for _, entry := range entries {
		if entry.Transcript != nil && refs[entry.Transcript.NativeRef] {
			projected++
		}
	}
	if projected != text {
		return fmt.Errorf("%d text updates arrived before the deadline and the receipt holds %d transcript fragments projected from them", text, projected)
	}
	return nil
}

// piObservedLaunch is a confinement launch that keeps the parent's account of
// what the runtime streamed. It changes nothing the runtime receives and
// nothing the adapter is given: the bytes pass through unaltered and in order.
type piObservedLaunch struct {
	thread.ConfinementLaunch

	mu       sync.Mutex
	started  time.Time
	deadline time.Time
	arrivals []piArrival
	pending  []byte
}

func (l *piObservedLaunch) Start(ctx context.Context, invocation thread.Invocation) (thread.Child, error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		return nil, errors.New("the runtime was started under no deadline, so nothing it emitted can be placed before one")
	}
	l.mu.Lock()
	l.started, l.deadline = time.Now(), deadline
	l.mu.Unlock()
	if invocation.Output != nil {
		invocation.Output = &piArrivalStream{EventStream: invocation.Output, launch: l}
	}
	return l.ConfinementLaunch.Start(ctx, invocation)
}

func (l *piObservedLaunch) account() piStreamAccount {
	l.mu.Lock()
	defer l.mu.Unlock()
	return piStreamAccount{Started: l.started, Deadline: l.deadline, Arrivals: append([]piArrival(nil), l.arrivals...)}
}

// observe times a line when its newline arrives. A line the stream ends inside
// never arrived whole and is not counted; the adapter's own rule for decoding
// is applied, so what is counted is what it would retain as a native event.
func (l *piObservedLaunch) observe(at time.Time, src []byte) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.pending = append(l.pending, src...)
	for {
		end := bytes.IndexByte(l.pending, '\n')
		if end < 0 {
			return
		}
		line := l.pending[:end]
		if arrival, ok := piArrivalOf(at, line); ok {
			l.arrivals = append(l.arrivals, arrival)
		}
		l.pending = append([]byte(nil), l.pending[end+1:]...)
	}
}

func piArrivalOf(at time.Time, line []byte) (piArrival, bool) {
	if len(bytes.TrimSpace(line)) == 0 {
		return piArrival{}, false
	}
	var event struct {
		Type                  string `json:"type"`
		AssistantMessageEvent struct {
			Type  string `json:"type"`
			Delta string `json:"delta"`
		} `json:"assistantMessageEvent"`
	}
	if json.Unmarshal(line, &event) != nil || event.Type == "" {
		return piArrival{}, false
	}
	arrival := piArrival{At: at, Type: event.Type}
	if event.Type == "message_update" {
		arrival.Update = event.AssistantMessageEvent.Type
		arrival.Text = arrival.Update == "text_delta" && event.AssistantMessageEvent.Delta != ""
	}
	return arrival, true
}

type piArrivalStream struct {
	thread.EventStream
	launch *piObservedLaunch
}

func (s *piArrivalStream) Write(src []byte) (int, error) {
	s.launch.observe(time.Now(), src)
	return s.EventStream.Write(src)
}

// The account is only worth what its own guards are, so each is broken here on
// purpose, with no provider and no model.

// recordingLaunch stands in for confinement: it keeps the invocation it was
// handed and starts nothing.
type recordingLaunch struct{ invocation thread.Invocation }

func (*recordingLaunch) Verified() error { return nil }

func (l *recordingLaunch) Start(_ context.Context, invocation thread.Invocation) (thread.Child, error) {
	l.invocation = invocation
	return nil, nil
}

func TestTheParentsAccountOfAPiStreamIsTakenOffThePipeAgainstTheRealDeadline(t *testing.T) {
	inner := &recordingLaunch{}
	observed := &piObservedLaunch{ConfinementLaunch: inner}
	if _, err := observed.Start(context.Background(), thread.Invocation{Output: &testStream{}}); err == nil {
		t.Fatal("a runtime started under no deadline was observed, so nothing it emitted could be placed before one")
	}

	fixture, err := os.ReadFile(filepath.Join("..", "thread", "testdata", "pi", "stream.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var wantTypes []string
	var wantText int
	for _, line := range bytes.Split(bytes.TrimSpace(fixture), []byte("\n")) {
		arrival, ok := piArrivalOf(time.Now(), line)
		if !ok {
			t.Fatalf("a native Pi fixture line is not an arrival: %s", line)
		}
		wantTypes = append(wantTypes, arrival.Type)
		if arrival.Text {
			wantText++
		}
	}
	if wantText == 0 {
		t.Fatal("the Pi fixture carries no text update, so nothing here tests that one is recognized")
	}

	downstream := &testStream{}
	deadline := time.Now().Add(time.Hour)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	if _, err := observed.Start(ctx, thread.Invocation{Output: downstream}); err != nil {
		t.Fatal(err)
	}
	// Lines split across writes, a line that is not an event, and a line the
	// stream ends inside: only whole events are arrivals.
	stream := append(append([]byte(nil), fixture...), []byte("not an event\n{\"type\":\"turn_start\"")...)
	for len(stream) > 0 {
		n := min(7, len(stream))
		if _, err := inner.invocation.Output.Write(stream[:n]); err != nil {
			t.Fatal(err)
		}
		stream = stream[n:]
	}

	account := observed.account()
	if !account.Deadline.Equal(deadline) {
		t.Fatalf("the account's deadline is %v, want the supervisor's %v", account.Deadline, deadline)
	}
	var gotTypes []string
	gotText := 0
	for _, arrival := range account.Arrivals {
		gotTypes = append(gotTypes, arrival.Type)
		if arrival.Text {
			gotText++
		}
	}
	if !slices.Equal(gotTypes, wantTypes) || gotText != wantText {
		t.Fatalf("arrivals = %v with %d text, want %v with %d", gotTypes, gotText, wantTypes, wantText)
	}
	if want := string(fixture) + "not an event\n{\"type\":\"turn_start\""; downstream.String() != want {
		t.Fatal("the observed launch altered the bytes it passed on to the adapter")
	}
}

// piTestAccount is an account whose arrivals are spaced a second apart from
// its start, with the deadline at the given second.
func piTestAccount(deadline int, arrivals ...piArrival) piStreamAccount {
	start := time.Unix(1_000_000, 0)
	for i := range arrivals {
		arrivals[i].At = start.Add(time.Duration(i) * time.Second)
	}
	return piStreamAccount{Started: start, Deadline: start.Add(time.Duration(deadline) * time.Second), Arrivals: arrivals}
}

func piUpdates(n int, update string) []piArrival {
	var updates []piArrival
	for range n {
		updates = append(updates, piArrival{Type: "message_update", Update: update, Text: update == "text_delta"})
	}
	return updates
}

func TestAnInterruptPremiseHoldsOnlyForASessionStillUnderWay(t *testing.T) {
	opening := []piArrival{{Type: "response"}, {Type: "agent_start"}, {Type: "turn_start"}}
	enough := slices.Concat(opening, piUpdates(piInterruptUpdates, "thinking_delta"))
	for _, tc := range []struct {
		name    string
		account piStreamAccount
		want    piPremise
	}{
		{"updates that are all thinking, which is the measured case", piTestAccount(len(enough), enough...), piUnderWay},
		{"nothing streamed yet", piTestAccount(len(opening), opening...), piNotStarted},
		{"one update short", piTestAccount(len(enough)-1, enough...), piNotStarted},
		{"the updates arrived only after the deadline", piTestAccount(len(opening), enough...), piNotStarted},
		{"a session that ended before its deadline", piTestAccount(len(enough)+2, slices.Concat(enough, []piArrival{{Type: "agent_end"}, {Type: "agent_settled"}})...), piSettled},
		{"a session that settled only after its deadline", piTestAccount(len(enough), slices.Concat(enough, []piArrival{{Type: "agent_settled"}})...), piUnderWay},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, why := piInterruptPremise(tc.account); got != tc.want {
				t.Fatalf("premise = %d, want %d: %s", got, tc.want, why)
			}
		})
	}

	if (piTestAccount(0, piUpdates(3, "thinking_delta")...)).piAnswered() {
		t.Fatal("a stream carrying no response to the prompt reads as one that answered it")
	}
	if !(piTestAccount(0, opening...)).piAnswered() {
		t.Fatal("a stream that answered the prompt reads as one that did not")
	}
}

// piTestReceipt is the receipt a faithful adapter writes for an account: one
// native per arrival in order, and one transcript fragment per text update.
func piTestReceipt(a piStreamAccount) []thread.Entry {
	entries := []thread.Entry{{Lifecycle: &thread.Lifecycle{Event: thread.Started}}}
	for i, arrival := range a.Arrivals {
		ref := fmt.Sprintf("native-%d", i)
		entries = append(entries, thread.Entry{Native: &thread.NativeEvent{Ref: ref, Type: arrival.Type}})
		if arrival.Text {
			entries = append(entries, thread.Entry{Transcript: &thread.TranscriptFragment{NativeRef: ref}})
		}
	}
	return entries
}

func TestAReceiptThatLostWhatArrivedBeforeTheKillIsCaught(t *testing.T) {
	arrivals := slices.Concat([]piArrival{{Type: "response"}, {Type: "agent_start"}},
		piUpdates(2, "thinking_delta"), piUpdates(2, "text_delta"), piUpdates(2, "toolcall_delta"))
	account := piTestAccount(len(arrivals), arrivals...)
	if err := piLostBeforeTheKill(account, piTestReceipt(account)); err != nil {
		t.Fatalf("a receipt holding everything that arrived is reported as losing some: %v", err)
	}

	without := func(entries []thread.Entry, drop func(thread.Entry) bool) []thread.Entry {
		var kept []thread.Entry
		dropped := false
		for _, entry := range entries {
			if !dropped && drop(entry) {
				dropped = true
				continue
			}
			kept = append(kept, entry)
		}
		return kept
	}
	for _, tc := range []struct {
		name    string
		entries []thread.Entry
	}{
		{"a native event dropped", without(piTestReceipt(account), func(e thread.Entry) bool { return e.Native != nil && e.Native.Type == "agent_start" })},
		{"the last event before the deadline dropped", piTestReceipt(piTestAccount(len(arrivals), arrivals[:len(arrivals)-1]...))},
		{"two events out of order", func() []thread.Entry {
			swapped := slices.Clone(arrivals)
			swapped[0], swapped[1] = swapped[1], swapped[0]
			return piTestReceipt(piTestAccount(len(swapped), swapped...))
		}()},
		{"a text update with no transcript fragment", without(piTestReceipt(account), func(e thread.Entry) bool { return e.Transcript != nil })},
		{"a transcript fragment projected from an update that was not text", func() []thread.Entry {
			entries := piTestReceipt(account)
			for _, entry := range entries {
				if entry.Native != nil && entry.Native.Type == "message_update" {
					return append(entries, thread.Entry{Transcript: &thread.TranscriptFragment{NativeRef: entry.Native.Ref}})
				}
			}
			t.Fatal("the receipt holds no update to misproject")
			return nil
		}()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := piLostBeforeTheKill(account, tc.entries); err == nil {
				t.Fatal("the receipt was reported as holding everything that arrived before the kill")
			}
		})
	}

	// What arrived after the deadline is not the claim, so a receipt without it
	// is not a receipt that lost anything.
	early := piTestAccount(3, arrivals...)
	if err := piLostBeforeTheKill(early, piTestReceipt(piTestAccount(3, arrivals[:3]...))); err != nil {
		t.Fatalf("a receipt missing only what arrived after the deadline is reported as losing: %v", err)
	}
}
