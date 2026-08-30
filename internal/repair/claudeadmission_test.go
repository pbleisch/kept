//go:build !windows

package repair

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/box"
	"github.com/pbleisch/kept/internal/canonical"
	"github.com/pbleisch/kept/internal/config"
	"github.com/pbleisch/kept/internal/confine"
	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/gather"
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/run"
	"github.com/pbleisch/kept/internal/thread"
	"github.com/pbleisch/kept/internal/tick"
	"github.com/pbleisch/kept/internal/workspace"
)

// This file is decision 0052's admission evidence for the Claude adapter, and
// it is what `claudeAdmitted` rests on.
//
// It is the file the earlier ones deliberately were not.
// internal/confine/claudeinbox_test.go measures what a confined session needs
// and says in its own header that it is not admission; the file beside this one
// proves the brokered credential route and says the same; and
// internal/thread/claudestandin_test.go kills a stand-in, which is the plumbing
// rather than the proof. What all three lacked was a real session with a real
// credential.
//
// The credential is the operator's own, reached the way a dispatcher reaches
// it: config.ReadUser gives a *reference*, internal/repair assembles a brokered
// route from it, and the value never enters this process. A machine that has not
// configured one skips, because a missing credential is not a broken adapter.
//
// WHAT WAS MEASURED, in two rounds, against Claude Code 2.1.241 confined by nono
// 0.74.0 with a brokered credential answered by api.anthropic.com. Every session
// ran in a box cut from kept's own corpus and source, carrying the packet for a
// promise deliberately broken there.
//
//   - 2026-08-23, eight sessions, no declared ask. The launch, the packet and
//     the projections: settled. Sessions started in the box, replayed the packet
//     as a native user event, and produced native events with transcript, cost
//     and turn projections referencing them. Two were killed at their deadline
//     and retained their partial receipts, the timeout cause and the reaped
//     signal. The repair: not settled. Of the six that ran to their own end,
//     exactly one performed it, and that one had read this file out of the box —
//     the contamination keptRepairRepository now removes. Every session with a
//     clean box located the offending call by itself, wrote a correct account of
//     why the promise does not hold, and then stopped without editing anything.
//     Carrying the dispatch policies the packet had been missing changed
//     nothing, and neither did the write boundary: a shell in the same box
//     appends to the file under repair and is refused one directory up.
//
//   - 2026-08-24, three sessions, with the declared ask of decision 0058 and
//     nothing else changed. A session with a clean box removed the offending
//     call and stopped; the promise, re-derived from that box's world, read
//     kept, and the diff was one line in the one file the promise's repair scope
//     names. A second ran to its own end producing 808 native events, 44 turns
//     and 23 transcript fragments with every projection naming the native event
//     it came from. A third was killed at fifteen seconds and kept 126 native
//     events, 6 turn updates, the timeout cause, the reaped signal and no cost.
//
// The gap the second round closed was never confinement and never the
// credential. A packet states authority and evidence and never an ask — decision
// 0009 is deliberate about that — and this runtime, launched with `--print`,
// reads one as a question. Pi's harness supplies that framing itself; Claude
// Code's does not, so kept declares it, on a channel that is not the packet's
// and in the same words for every repair. See docs/decision/0058.

// The promise this repair is about.
//
// It is chosen for four reasons and against two, and the choice is deliberately
// not the one the Pi proof made — an admission is per adapter, and running Pi's
// promise again would measure Pi's break a second time rather than Claude
// repairing something.
//
// For: its instrument is `go/symbols`, which parses and never shells out
// (decision 0049), so re-deriving the verdict inside a box needs no toolchain
// and answers on a tree that need not compile. Its observation carries the
// offending reference itself, so the gap the agent has to close is *in the
// evidence* rather than in a message kept wrote. Its bound repair scope,
// `internal/gather/**/*.go`, is narrower than the package tree the box holds,
// so decision 0047's bound is exercised rather than nominally present. And the
// break is a call rather than an import, so what Claude has to find is not what
// Pi had to find.
//
// Against: `core-packages-are-tested`, `the-confinement-suite-passes` and
// `the-tree-vets-clean` measure with instruments that shell out to the Go
// toolchain, and kept's own internal/confine tests cannot be re-derived inside a
// kept box — which is why the promise holding them authorizes no repair at all.
// And `standing-is-minted-in-one-place` folds
// every symbol under `internal/**`: measured, its observation is 176 KB against
// this one's 18 KB, and a packet nobody should pay for is a packet nobody should
// assemble.
const (
	claudeRepairPromise = "promise:gatherers-do-not-write"
	claudeRepairFile    = "internal/gather/source.go"
	claudeRepairWriter  = "MkdirAll"

	// claudeRepairMarker is what the break leaves in the tree, and the box may
	// hold it in exactly one file: the one under repair.
	//
	// claudeRepairAnswerKey is this file, which is the reason the rule needs
	// stating. A repair box is a copy of a repository, and kept's repository
	// contains the test that authors the break — so the first run of this proof
	// put its own answer key in the box, and the session found it: it grepped
	// for the marker, read this file, and wrote "this is the injected
	// violation" before removing the line. It had already located the call from
	// the packet's evidence, so the repair itself was sound; but a proof that
	// leaves the answer where the subject can read it has measured reading
	// rather than repairing, and nobody can tell afterwards which one happened.
	claudeRepairMarker    = "parse-cache"
	claudeRepairAnswerKey = "internal/repair/claudeadmission_test.go"

	// And the same rule one step out. claudeRepairAccount is not an answer key
	// — it names no promise, no file and no marker — but it is kept's own
	// account of *this measurement*: that confined sessions used to read a
	// packet and stop without editing, and that an ask was declared so that one
	// would act. A session that read it would learn it is the subject of a
	// proof and what the proof is hoping for, which confounds the one question
	// being put here as surely as the answer key confounded the last one. In a
	// repository that is not kept's this file does not exist; in a box cut from
	// kept's it has to be left out for the same reason the test is.
	claudeRepairAccount = "docs/decision/0058-the-ask-is-declared-and-travels-beside-the-packet.md"
)

// claudeRepairDeadline bounds a completed repair session, and
// claudeRepairInterrupt is the deliberately short one the interruption proof
// uses. Neither is a default: an external runtime with no deadline is what
// decision 0026 refuses.
//
// Both are set against measurements rather than guessed, and the second one
// moved once already. Confined repairs of this promise ran 30 and 90 seconds
// end to end, emitting 325 and 808 native events, 17 and 44 turns, and 8 and 23
// transcript fragments before their results. So the first is a bound with room
// in it rather than a target.
//
// The second is half of the *faster* of those, which is the number that matters:
// an interruption proof whose deadline the session beats is not one, and the
// earlier 25 seconds was set against a 64-second session from before the ask of
// decision 0058 — after which sessions act instead of answering, and finish
// sooner. At this event rate fifteen seconds is well past startup, the packet's
// replay and the first turns, and well before the result a cost comes from.
const (
	claudeRepairDeadline  = 8 * time.Minute
	claudeRepairInterrupt = 15 * time.Second
)

// TestAConfinedClaudeSessionCarriesThePacketAndProjectsItsOwnEvents is decision
// 0052's live-launch bar for this adapter, and the whole of it: a real session
// in a box, receiving the packet marker, with transcript, cost and turn
// projections that each name the native Claude event they came from.
//
// It is deliberately separate from the repair below. 0052's table asks for a
// launch, its projections and an interrupted kill; it does not ask the agent to
// succeed. Keeping the two apart is what lets the record say which one holds.
func TestAConfinedClaudeSessionCarriesThePacketAndProjectsItsOwnEvents(t *testing.T) {
	session := liveClaudeRepairSession(t)
	receipt, store := session.dispatch(t, claudeRepairDeadline)
	entries, quarantined, err := thread.ReadReceipt(store, receipt.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(quarantined) != 0 {
		t.Fatalf("the live Claude receipt quarantined its own evidence: %#v", quarantined)
	}

	kinds := receiptKinds(entries)
	for _, want := range []string{"thread_packet", "thread_native", "thread_transcript", "thread_cost", "thread_turn", "thread_lifecycle"} {
		if kinds[want] == 0 {
			t.Fatalf("the live Claude receipt has no %s entry: %v", want, kinds)
		}
	}
	if kinds["thread_turn"] < 2 {
		t.Fatalf("the live Claude session produced %d turn updates, so it did not stay active long enough to be incremental evidence", kinds["thread_turn"])
	}
	assertProjectionsReferenceNativeEvents(t, entries)
	assertNativeTypes(t, entries, "system", "user", "assistant", "stream_event", "result")

	// The packet reached the runtime, twice over. The bytes the parent stored
	// before the launch are the bytes it assembled, and the runtime's own
	// replay of the user message it read carries them back out.
	assertStoredPacketIsExact(t, store, entries, session.packet)
	if !sawPacketMarker(t, store, entries) {
		t.Fatal("no retained Claude transcript or native event carries the packet marker, so the session did not receive the packet")
	}

	// Claude's session ends when it has answered, so a session that ran to its
	// own end is a normal exit rather than a deadline kill.
	last := entries[len(entries)-1]
	if last.Lifecycle == nil || last.Lifecycle.Event != thread.Exited || last.Lifecycle.Exit == nil || last.Lifecycle.Exit.Code == nil || *last.Lifecycle.Exit.Code != 0 {
		t.Fatalf("the live Claude session did not exit normally: %#v", last.Lifecycle)
	}
	for _, entry := range entries {
		if entry.Lifecycle != nil && (entry.Lifecycle.Event == thread.TimedOut || entry.Lifecycle.Event == thread.Killed) {
			t.Fatalf("the completed Claude session was interrupted: %#v", entry.Lifecycle)
		}
	}
	t.Logf("confined Claude session: %v native events, %v turns, %v transcript fragments, %v cost updates",
		kinds["thread_native"], kinds["thread_turn"], kinds["thread_transcript"], kinds["thread_cost"])
	t.Logf("what the confined session did:\n%s", claudeSessionSummary(entries))
}

// TestConfinedClaudeRepairsABrokenPromiseAndTheVerdictIsReDerived is the whole
// loop: cut a box, break the promise in it, gather and find it not kept,
// dispatch Claude under confinement with a brokered credential, gather again,
// and take the verdict from the second measurement.
//
// The re-gather is the point.
// `policy:the-agents-report-is-never-an-input-to-the-verdict` is not a
// convention this test observes — the receipt is read for evidence about the
// session and is never consulted about whether the promise is kept.
//
// This is Pi's precedent rather than 0052's table, and it is the one that was
// last to hold. See the header: without the declared ask of decision 0058 a
// session reads the packet, answers it precisely, and leaves this red — so a
// failure here is more likely to be about what the runtime was told than about
// what it was allowed to do.
func TestConfinedClaudeRepairsABrokenPromiseAndTheVerdictIsReDerived(t *testing.T) {
	session := liveClaudeRepairSession(t)

	before := session.verdict(t)
	if before.Value != model.NotKept {
		t.Fatalf("the promise reads %q in the box before the repair, so there is nothing to repair: %s", before.Value, before.Detail)
	}

	receipt, store := session.dispatch(t, claudeRepairDeadline)
	entries, quarantined, err := thread.ReadReceipt(store, receipt.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(quarantined) != 0 {
		t.Fatalf("the live Claude receipt quarantined its own evidence: %#v", quarantined)
	}

	kinds := receiptKinds(entries)
	t.Logf("receipt: %v; lifecycle: %s", kinds, lifecycleText(entries))

	// What the session did, in its own words, before anything is asserted about
	// whether it worked. A repair that failed, a repair that was refused and a
	// session that answered the packet instead of acting on it all look the same
	// from the verdict; they do not look the same from here.
	t.Logf("what the confined session did:\n%s", claudeSessionSummary(entries))

	// The repair itself, re-derived. Nothing above is an input to this.
	after := session.verdict(t)
	if after.Value != model.Kept {
		t.Fatalf("the re-derived verdict after the repair is %q: %s\nthe session may have answered the packet rather than acting on it; see this file's header.\n%s",
			after.Value, after.Detail, session.change(t).Diff)
	}
	if after.World.Tree == before.World.Tree {
		t.Fatal("the box's world did not move, so the second verdict is the first one under another name")
	}

	// And the repair is the agent's, inside its scope, and reported as a diff.
	// The scope is measured rather than assumed: confinement grants a path and
	// the promise bound a glob, so a write the grant admits and the glob does
	// not is found here or nowhere. See docs/decision/0059.
	changed := session.change(t)
	review := changed.Against(session.scope)
	t.Logf("confined Claude repair: %v native events, %v turns, %v transcript fragments, %v cost updates",
		kinds["thread_native"], kinds["thread_turn"], kinds["thread_transcript"], kinds["thread_cost"])
	t.Logf("re-derived verdict: %s -> %s", before.Value, after.Value)
	t.Logf("repair diff:\n%s", changed.Diff)
	t.Logf("what a person reads:\n%s", review)
	if !review.Bounded() {
		t.Fatalf("the repair reached outside the scope its promise bound:\n%s\n%s", review, changed.Diff)
	}
	if got := changed.Paths; len(got) != 1 || got[0] != claudeRepairFile {
		t.Fatalf("the repair changed %v, want exactly %s:\n%s", got, claudeRepairFile, changed.Diff)
	}
}

// TestConfinedClaudeKilledMidSessionRetainsItsPartialReceipt is the
// interruption proof decision 0052 requires, and it is the half that matters: a
// completion-only report loses exactly this.
//
// The deadline is short enough that Claude is still streaming when it elapses,
// so what is asserted is that the events already received survive alongside the
// timeout, the kill and the exit the parent actually observed — and that no
// completion was inferred after the fact.
func TestConfinedClaudeKilledMidSessionRetainsItsPartialReceipt(t *testing.T) {
	session := liveClaudeRepairSession(t)

	receipt, store := session.dispatch(t, claudeRepairInterrupt)
	entries, quarantined, err := thread.ReadReceipt(store, receipt.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(quarantined) != 0 {
		t.Fatalf("the interrupted Claude receipt quarantined its own evidence: %#v", quarantined)
	}

	kinds := receiptKinds(entries)
	if kinds["thread_native"] < 8 {
		t.Fatalf("the interrupted Claude session retained %d native events, so the kill measured nothing: %v", kinds["thread_native"], kinds)
	}
	// The incremental projection a killed repair actually has, which is turns
	// and not transcript — measured, and worth saying rather than working
	// around. This adapter's transcript is assistant *text*: a `stream_event`
	// carrying a `text_delta`, or an assistant text block when no streamed text
	// arrived. A repair session spends its opening turns reading and editing,
	// and a tool-using turn emits neither — the deltas are `input_json_delta`
	// and the content blocks are `tool_use`. So at fifteen seconds this receipt
	// held 126 native events and 6 turn updates with no transcript at all, and
	// the text arrived later, in the summary the completed session reaches.
	//
	// Nothing is lost by that: decision 0052 keeps the native entry as the
	// evidence precisely so an empty projection is not an empty record, and the
	// completed session above is where transcript, cost and turn are all
	// present together. What the kill has to retain is what had already
	// happened, and it does.
	if kinds["thread_turn"] == 0 && kinds["thread_transcript"] == 0 {
		t.Fatalf("the interrupted Claude session retained no incremental projection at all: %v", kinds)
	}
	// The result event is where a cost comes from, and a session killed before
	// one has no cost to report. A receipt carrying one here would be a
	// completion manufactured on the failure path.
	if kinds["thread_cost"] != 0 {
		t.Fatalf("a Claude session killed before its result reported a cost: %v", kinds)
	}
	t.Logf("interrupted Claude receipt: %v", kinds)
	assertProjectionsReferenceNativeEvents(t, entries)
	assertStoredPacketIsExact(t, store, entries, session.packet)

	// The lifecycle the parent observed, in the order it happened. Each item is
	// written only after it happened, so this sequence is a record and never an
	// inference: timed out, then killed once the group kill succeeded, then the
	// exit that was actually reaped.
	var lifecycle []thread.LifecycleEvent
	var timedOut, exited *thread.Lifecycle
	for _, entry := range entries {
		if entry.Lifecycle == nil {
			continue
		}
		lifecycle = append(lifecycle, entry.Lifecycle.Event)
		switch entry.Lifecycle.Event {
		case thread.TimedOut:
			timedOut = entry.Lifecycle
		case thread.Exited:
			exited = entry.Lifecycle
		}
	}
	want := []thread.LifecycleEvent{thread.Started, thread.TimedOut, thread.Killed, thread.Exited}
	if len(lifecycle) != len(want) {
		t.Fatalf("interrupted Claude lifecycle = %v, want %v", lifecycle, want)
	}
	for i, event := range want {
		if lifecycle[i] != event {
			t.Fatalf("interrupted Claude lifecycle = %v, want %v", lifecycle, want)
		}
	}

	// The cause, and the reaped exit. A timeout that did not record its own
	// deadline is a kill nobody can account for, and a killed process group that
	// reported an exit code rather than a signal was not killed.
	if timedOut == nil || timedOut.Timeout != claudeRepairInterrupt.String() {
		t.Fatalf("the interrupted Claude session recorded no timeout cause: %#v", timedOut)
	}
	if exited == nil || exited.Exit == nil || exited.Exit.Signal == "" {
		t.Fatalf("the killed Claude session did not record a reaped terminating signal: %#v", exited)
	}
	// A native event after the exit would be a completion written after the
	// fact rather than a receipt of what arrived.
	if entries[len(entries)-1].Lifecycle == nil || entries[len(entries)-1].Lifecycle.Event != thread.Exited {
		t.Fatalf("the interrupted Claude receipt does not end at the reaped exit: %s", thread.ReceiptText(entries))
	}
}

// TestTheClaudeInvocationIsArgvWithNoShell is the other half of the
// launch, and it costs nothing to run: the adapter is asked for the invocation
// a dispatch would hand confinement, and what comes back is inspected rather
// than started.
//
// Four properties, each of which would be a different failure. There is no
// shell, so a `run:`-style injection surface does not exist here either
// (decision 0038 refuses one for gatherers and this is the same rule one layer
// up). The packet's exact bytes survive the input channel, which is what makes
// the marker found in a live session evidence about *this* packet. The declared
// ask travels beside the packet rather than inside it, and says the same thing
// whatever the packet is. And under the brokered channel the child environment
// carries no credential at all, because the profile publishes the phantom and
// an invocation that named it is refused by the launch.
func TestTheClaudeInvocationIsArgvWithNoShell(t *testing.T) {
	session := liveClaudeRepairSession(t)

	store := ledger.NewFiles(t.TempDir())
	writable := ledger.Open(store, run.Start("claude-admission-argv", nil))
	receipt, err := thread.NewReceipt(writable, thread.MintID())
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := thread.Select(string(thread.Claude))
	if err != nil {
		t.Fatal(err)
	}
	adapterReceipt, err := receipt.ForAdapter(adapter.Runtime(), adapter.AdapterVersion(session.runtime.Config))
	if err != nil {
		t.Fatal(err)
	}
	invocation, err := adapter.Prepare(session.runtime.Config, session.packet, adapterReceipt)
	if err != nil {
		if isUnadmitted(err) {
			t.Fatalf("the Claude adapter refuses as unadmitted, and this file is its admission evidence: %v", err)
		}
		t.Fatalf("the Claude adapter refused to prepare an invocation: %v", err)
	}

	if invocation.Path != session.runtime.Config.Executable {
		t.Fatalf("the invocation runs %s, and the configured runtime is %s", invocation.Path, session.runtime.Config.Executable)
	}
	if !filepath.IsAbs(invocation.Path) {
		t.Fatalf("the invocation runs %q, which is not an explicit absolute path", invocation.Path)
	}
	// The ask decision 0058 added is prose, and prose has spaces in it. It is
	// exempted from the metacharacter rule below and held to a stronger one
	// instead — it is a reviewed constant in the adapter, it is one argv
	// element, and it says the same thing for every packet, which is what the
	// second half of this test measures. Nothing else in the argv is exempt:
	// the rule is about what the invocation composes from configuration or from
	// a packet, and a shell metacharacter in one of those is a value somebody
	// expected an interpreter to read.
	ask := claudeAskArgument(t, invocation.Args)
	for _, arg := range invocation.Args {
		if arg == ask {
			continue
		}
		if arg == "-c" || strings.HasSuffix(arg, "/sh") || strings.HasSuffix(arg, "/bash") || strings.ContainsAny(arg, "|&;<>()$`\\\"' \t\n") {
			t.Fatalf("the invocation argv carries %q, which is a shell or something only a shell would read: %v", arg, invocation.Args)
		}
	}
	// And the exemption is not a hole: the ask itself is no shell either.
	if ask == "-c" || strings.HasSuffix(ask, "/sh") || strings.HasSuffix(ask, "/bash") {
		t.Fatalf("the ask argument is a shell: %q", ask)
	}

	// The exact packet bytes, through the adapter's own documented channel. The
	// input is one JSONL user message whose single text block is the packet and
	// nothing else — no prompt prefix, no rebuilt corpus data.
	var delivered struct {
		Type    string `json:"type"`
		Message struct {
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"message"`
	}
	line, rest, found := strings.Cut(string(invocation.Input), "\n")
	if !found || strings.TrimSpace(rest) != "" {
		t.Fatalf("the adapter's input channel carried %d lines, and a packet is one: %q", strings.Count(string(invocation.Input), "\n"), invocation.Input)
	}
	if err := json.Unmarshal([]byte(line), &delivered); err != nil {
		t.Fatalf("the adapter's input channel is not one JSONL user message: %v", err)
	}
	if len(delivered.Message.Content) != 1 || delivered.Message.Content[0].Text != string(session.packet.Bytes()) {
		t.Fatalf("the packet did not survive the adapter's input channel: %#v", delivered.Message.Content)
	}

	// The ask, beside the packet rather than in it. This test is outside the
	// thread package on purpose: what it can check is the boundary a caller can
	// see, which is that the ask names nothing from the packet and does not
	// move when the packet does. The words themselves are the adapter's, and
	// internal/thread is where they are pinned.
	if strings.Contains(ask, claudeRepairPromise) || strings.Contains(ask, claudeRepairFile) || strings.Contains(ask, claudeRepairMarker) {
		t.Fatalf("the ask names this repair, so it is an instruction about one packet rather than a declaration about the work: %q", ask)
	}
	if strings.Contains(string(session.packet.Bytes()), ask) {
		t.Fatal("the packet carries the ask, which is what decision 0009 keeps out of it")
	}

	// And no credential, because there is none in this process to put there.
	for _, entry := range invocation.Env {
		name, _, _ := strings.Cut(entry, "=")
		if name == thread.ClaudeCredentialEnvironment {
			t.Fatalf("the brokered invocation names %s, which the credential route publishes", name)
		}
	}
}

// claudeAskArgument is the one argv element decision 0058 appends, found by the
// flag that introduces it. A missing one is a failure rather than a skip: an
// invocation with no ask is the unframed session that channel exists to stop
// being.
func claudeAskArgument(t *testing.T, args []string) string {
	t.Helper()
	index := slices.Index(args, "--append-system-prompt")
	if index < 0 || index+1 >= len(args) {
		t.Fatalf("the invocation carries no declared ask: %v", args)
	}
	if strings.TrimSpace(args[index+1]) == "" {
		t.Fatalf("the invocation's ask is empty: %v", args)
	}
	return args[index+1]
}

// isUnadmitted is the admission gate seen from outside the thread package, and
// it exists to tell one failure from every other one: a shut gate means this
// file's evidence was discarded, and a broken profile or a lapsed credential
// means something else entirely. It is a string test because the gate is a
// Refusal's detail rather than a type.
func isUnadmitted(err error) bool {
	return err != nil && strings.Contains(err.Error(), "runtime is unadmitted")
}

// TestARealFailedCanaryRefusesTheConfinedClaudeDispatch is the other half of
// "the runtime cannot read the ledger, the keychain or a credential that can
// push to origin": those are the canary's probes, and this is what says a canary
// that did not report is a refusal rather than a launch.
//
// The break is the canary helper itself — the profile names an executable that
// is not kept, so the confined helper produces no report at all. Nothing about
// the repair profile is weakened, which is why the refusal is about verification
// rather than about a boundary this test removed. And the unbroken profile
// verifies at the end, which is the same statement from the other side: every
// probe decision 0052 names was attempted in this box and every one was refused.
func TestARealFailedCanaryRefusesTheConfinedClaudeDispatch(t *testing.T) {
	session := liveClaudeRepairSession(t)
	broken := session.profile
	broken.Kept = "/bin/echo"
	launch, err := confine.NewNono(broken)
	if err != nil {
		t.Fatal(err)
	}
	if err := launch.Verified(); err == nil {
		t.Fatal("a canary that produced no report verified the launch")
	} else if !strings.Contains(err.Error(), "canary") {
		t.Fatalf("verification error = %v, want it to name the canary", err)
	}

	store := ledger.NewFiles(t.TempDir())
	writable := ledger.Open(store, run.Start("claude-admission-canary", nil))
	receipt, err := thread.NewReceipt(writable, thread.MintID())
	if err != nil {
		t.Fatal(err)
	}
	err = thread.Dispatch(context.Background(), session.runtime.Config, session.packet, launch, thread.Supervisor{Timeout: time.Minute}, receipt)
	if err == nil || !strings.Contains(err.Error(), "canary failed") {
		t.Fatalf("a failed canary did not refuse the Claude dispatch: %v", err)
	}
	if entries, _, err := thread.ReadReceipt(store, receipt.ID()); err == nil && len(entries) != 0 {
		t.Fatalf("a refused Claude dispatch wrote a receipt stream: %s", thread.ReceiptText(entries))
	}

	// And the unbroken profile verifies, which is what says the refusal was
	// about the canary rather than about anything else in this box.
	if err := session.launch.Verified(); err != nil {
		t.Fatalf("the repair profile's own canary failed: %v", err)
	}
}

// claudeRepairSession is one confined Claude repair: the box, the profile, the
// assembled runtime and the packet. It is what a dispatcher will assemble;
// nothing in the tree dispatches a repair yet, so this test stands in for one
// and says so.
type claudeRepairSession struct {
	launch  *confine.Nono
	profile confine.Profile
	box     *box.Box
	runtime Runtime
	packet  thread.Packet
	// scope is the promise's bound repair scope, kept beside the packet
	// because a packet's bytes are sealed and a review needs the globs.
	scope []string
}

func liveClaudeRepairSession(t *testing.T) *claudeRepairSession {
	t.Helper()
	nono := liveProvider(t)
	user := operatorRepair(t)

	root := ungrantedDir(t)
	repository, created := keptRepairBox(t, root)

	// The session's home lies beside the box, outside the tree whose diff is
	// collected, and it is empty: the adapter refuses a home that is not, so a
	// session cannot inherit another one's state.
	home := filepath.Join(root, "claude-home-"+filepath.Base(created.Path()))
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(home) })

	assembled, err := Assemble(context.Background(), user, Session{Home: home, Temp: created.Temp()})
	if err != nil {
		t.Fatalf("assemble the operator's own repair runtime: %v", err)
	}
	toolchain, err := confine.ResolveNative(user.Executable)
	if err != nil {
		t.Fatalf("resolve the native Claude runtime: %v", err)
	}

	profile := confinementProfile(t, repository, created, nono)
	// The promise's bound repair scope, which is what may be changed in its
	// name, and it is narrower than the tree the box holds. See
	// docs/decision/0047.
	profile.Write = []string{filepath.Join(created.Path(), "internal", "gather")}
	profile.Domains = []string{assembled.Domain}
	profile.Env = assembled.Env
	profile.TempEnv = assembled.TempEnv
	profile.Home = home
	profile.Credential = assembled.Credential
	profile.ToolPath = append(append([]string(nil), toolchain.Path...), "/usr/bin", "/bin")
	profile.ToolRead = toolchain.Read
	launch, err := confine.NewNono(profile)
	if err != nil {
		t.Fatal(err)
	}

	session := &claudeRepairSession{launch: launch, profile: profile, box: created, runtime: assembled}
	session.packet = session.assemblePacket(t)
	return session
}

// dispatch runs one confined session through the boundary a repair will use.
func (s *claudeRepairSession) dispatch(t *testing.T, deadline time.Duration) (*thread.Receipt, ledger.Store) {
	t.Helper()
	// A scratch ledger: this is a box's session, and what it records is
	// evidence about the session rather than an act in the operator's history.
	store := ledger.NewFiles(t.TempDir())
	writable := ledger.Open(store, run.Start("claude-admission", nil))
	receipt, err := thread.NewReceipt(writable, thread.MintID())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), deadline+2*time.Minute)
	defer cancel()
	err = thread.Dispatch(ctx, s.runtime.Config, s.packet, s.launch, thread.Supervisor{Timeout: deadline}, receipt)
	if err != nil {
		if isUnadmitted(err) {
			// Not a skip. This file is what admits the adapter, so a gate found
			// shut here means somebody closed it without removing the evidence
			// that opened it, and a suite that went quiet on that would be
			// reporting nothing as though it were nothing wrong.
			t.Fatalf("the Claude adapter refuses as unadmitted, and this file is its admission evidence: %v", err)
		}
		t.Fatalf("dispatch a confined Claude repair: %v", err)
	}
	if err := writable.Close(run.Clean); err != nil {
		t.Fatal(err)
	}
	return receipt, store
}

// verdict re-derives the promise's answer from the box's world.
//
// It is a full pass: the tree is walked, the world digest is computed over what
// is actually there, the gatherer runs, and the predicate is applied. Nothing
// the runtime reported is read.
func (s *claudeRepairSession) verdict(t *testing.T) tick.Answer {
	t.Helper()
	return verdictInTheBox(t, s.box)
}

func verdictInTheBox(t *testing.T, created *box.Box) tick.Answer {
	t.Helper()
	c, err := corpus.Load(filepath.Join(created.Path(), "docs"), created.Path(), corpus.Options{Gatherers: gather.Builtin()})
	if err != nil {
		t.Fatal(err)
	}
	store := ledger.NewFiles(t.TempDir())
	writable := ledger.Open(store, run.Start("claude-admission-gather", nil))
	pass := &tick.Tick{
		Corpus:    c,
		Store:     store,
		Ledger:    writable,
		Catalogue: gather.Builtin(),
		Options:   tick.Options{Deadline: 5 * time.Minute},
	}
	res, err := pass.Run(context.Background())
	if err != nil {
		t.Fatalf("re-derive the promise in the box: %v", err)
	}
	if err := writable.Close(run.Clean); err != nil {
		t.Fatal(err)
	}
	for _, subject := range res.Subjects {
		if subject.ID != claudeRepairPromise {
			continue
		}
		if !subject.Answer.Gathered {
			t.Fatalf("the re-derivation carried an answer forward instead of measuring: %#v", subject.Answer)
		}
		return subject.Answer
	}
	t.Fatalf("the box's corpus holds no %s", claudeRepairPromise)
	return tick.Answer{}
}

// change is what the agent changed, which is what a person reviews: the paths
// and the diff, read from one snapshot of the box.
func (s *claudeRepairSession) change(t *testing.T) box.Change { return changeOf(t, s.box) }

func changeOf(t *testing.T, created *box.Box) box.Change {
	t.Helper()
	changed, err := created.Change(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return changed
}

// assemblePacket builds the packet from the box's own corpus: the promise's
// binding content, its body, the observation that says it is not kept, and the
// scope it bound.
//
// Ratified is set here because nothing in the tree derives it yet. A dispatcher
// reads it from the ledger, and refusing an unratified promise is already the
// packet's own guard; this test stands in for the caller rather than for the
// grant.
func (s *claudeRepairSession) assemblePacket(t *testing.T) thread.Packet {
	t.Helper()
	c, err := corpus.Load(filepath.Join(s.box.Path(), "docs"), s.box.Path(), corpus.Options{Gatherers: gather.Builtin()})
	if err != nil {
		t.Fatal(err)
	}
	entry, found := c.Get(claudeRepairPromise)
	if !found {
		t.Fatalf("the box's corpus holds no %s", claudeRepairPromise)
	}
	art := entry.Artifact
	kind, _ := corpus.SplitRef(art.ID)
	sc, known := c.Schemas.For(kind)
	if !known {
		t.Fatalf("the box's corpus declares no %s schema", kind)
	}
	hash, err := canonical.Bind(entry, sc)
	if err != nil {
		t.Fatal(err)
	}
	// The bound travels beside the packet: a packet's bytes are sealed, so a
	// review of what the repair touched reads the globs from here.
	s.scope = art.RepairScope
	packet, err := thread.AssemblePacket(thread.PacketInput{
		Promise: thread.RatifiedPromise{
			ID:          art.ID,
			Hash:        hash.Digest,
			Norm:        hash.Norm,
			Ratified:    true,
			Statement:   art.Statement,
			Gather:      art.Gather,
			HoldWhen:    art.HoldWhen,
			Trigger:     art.Trigger,
			RepairScope: art.RepairScope,
		},
		Why:          art.Body,
		Observations: s.observations(t, art),
		Policies:     dispatchPolicies(t, c),
	})
	if err != nil {
		t.Fatal(err)
	}
	return packet
}

// observations measures the broken promise in the box with every instrument it
// names, which is the evidence the packet carries. They are the same instruments
// the verdict is re-derived with.
//
// Keyed by the promise's own gather names, because that is what the packet
// carries them under and what its predicate reads them as. This promise names
// one, so the map has one entry — the singular case is the plural case with one
// key. See docs/decision/0061.
func (s *claudeRepairSession) observations(t *testing.T, art *model.Artifact) map[string]ledger.Observation {
	t.Helper()
	catalogue := gather.Builtin()
	if len(art.Gather) == 0 {
		t.Fatalf("%s names no gatherer, so there is no evidence to carry", art.ID)
	}
	tree, err := workspace.Read(s.box.Path(), workspace.Options{})
	if err != nil {
		t.Fatal(err)
	}
	requests := gather.Requests{Tree: tree, World: model.World{Tree: tree.Digest()}, Deadline: 5 * time.Minute}
	measured := map[string]ledger.Observation{}
	for name, spec := range art.Gather {
		g, known := catalogue[spec.Gatherer]
		if !known {
			t.Fatalf("%s names the %s gatherer, and there is no such gatherer", name, spec.Gatherer)
		}
		observed := gather.NewRunner(catalogue).Gather(context.Background(), spec, requests.For(g, spec.Args))
		if observed.Err != nil {
			t.Fatalf("measure %s in the box with %s: %v", art.ID, name, observed.Err)
		}
		measured[name] = ledger.Measured(observed)
	}
	return measured
}

// breakTheGatherPromise is the deliberate break: internal/gather calls a writer,
// which is the one thing the promise says it does not.
//
// It is an added statement and nothing else, so the diff a repair has to produce
// is the removal of one line. `os` and `path/filepath` are already imported
// there, so nothing about the file's shape changes, and the tree still parses —
// which is what `go/symbols` needs in order to report the reference at all.
func breakTheGatherPromise(t *testing.T, repository string) {
	t.Helper()
	path := filepath.Join(repository, filepath.FromSlash(claudeRepairFile))
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	const anchor = "\tsrc := &source{module: module, packages: map[string]*sourcePackage{}}\n"
	if !strings.Contains(string(src), anchor) {
		t.Fatalf("%s no longer builds its source set there, so the break has nowhere to go", claudeRepairFile)
	}
	broken := strings.Replace(string(src), anchor,
		"\t_ = os."+claudeRepairWriter+"(filepath.Join(req.Root, \".kept\", \""+claudeRepairMarker+"\"), 0o755)\n"+anchor, 1)
	if err := os.WriteFile(path, []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestTheScanThatKeepsTheAnswerKeyOutOfTheBoxCanFindOne is that guard, aimed at
// something it must find: this checkout, where the file that authors the break
// really is present. A scan that has never found anything is not a scan.
func TestTheScanThatKeepsTheAnswerKeyOutOfTheBoxCanFindOne(t *testing.T) {
	found := filesNamingTheBreak(t, moduleRoot(t))
	if !slices.Contains(found, claudeRepairAnswerKey) {
		t.Fatalf("the scan did not find %s in this checkout, so it cannot keep it out of a box: %v", claudeRepairAnswerKey, found)
	}
}

// assertOnlyTheCodeUnderRepairNamesTheBreak holds a repository copy to the rule
// above: the marker may be in the file being repaired and nowhere else.
func assertOnlyTheCodeUnderRepairNamesTheBreak(t *testing.T, root string) {
	t.Helper()
	found := filesNamingTheBreak(t, root)
	if len(found) != 1 || found[0] != claudeRepairFile {
		t.Fatalf("the repair box would hold %v naming the break, and only %s may", found, claudeRepairFile)
	}
}

// filesNamingTheBreak is every file under root whose bytes carry the marker, in
// slash form. The derived and version-control tiers are skipped because neither
// is source a session reads.
func filesNamingTheBreak(t *testing.T, root string) []string {
	t.Helper()
	var found []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if name := entry.Name(); path != root && (name == ".git" || name == ".kept") {
				return fs.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !strings.Contains(string(src), claudeRepairMarker) {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		found = append(found, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(found)
	return found
}

// TestTheBreakIsRealAndItsRepairIsOneLine holds the harness to its own premise,
// with no runtime and no credential involved.
//
// Everything the live tests assert about a session rests on two things being
// true of the box before one starts: the promise really is broken there, and
// removing the single line really does mend it. If either drifts — the promise's
// predicate changes, the anchor moves, the instrument answers differently — a
// live session would fail in a way that reads like the runtime's fault. This is
// the same measurement without the runtime, and it costs nothing to run.
func TestTheBreakIsRealAndItsRepairIsOneLine(t *testing.T) {
	_, created := keptRepairBox(t, ungrantedDir(t))

	before := verdictInTheBox(t, created)
	if before.Value != model.NotKept {
		t.Fatalf("the committed break does not break the promise: %s %s", before.Value, before.Detail)
	}

	path := filepath.Join(created.Path(), filepath.FromSlash(claudeRepairFile))
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var kept []string
	removed := 0
	for line := range strings.SplitSeq(string(src), "\n") {
		if strings.Contains(line, "os."+claudeRepairWriter+"(") {
			removed++
			continue
		}
		kept = append(kept, line)
	}
	if removed != 1 {
		t.Fatalf("the break put %d writer calls in %s, and a one-line repair needs exactly one", removed, claudeRepairFile)
	}
	if err := os.WriteFile(path, []byte(strings.Join(kept, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	after := verdictInTheBox(t, created)
	if after.Value != model.Kept {
		t.Fatalf("removing the one line does not mend the promise: %s %s", after.Value, after.Detail)
	}
	if touched := changeOf(t, created).Paths; len(touched) != 1 || touched[0] != claudeRepairFile {
		t.Fatalf("the one-line repair shows as %v in the box's diff, want exactly %s", touched, claudeRepairFile)
	}
}

// keptRepairBox is the box a repair stands in: kept's own corpus and source with
// the promise already broken, cut where the provider grants nothing.
//
// The break is committed rather than overlaid, and the box carries no dirty
// overlay at all. An overlay is the shape a dispatcher uses for uncommitted
// local drift, but here it would also be a tell: the one line this proof adds
// would be the only uncommitted change in the tree, and `git diff` inside the
// box would point straight at it. Committed with everything else in one commit,
// it is indistinguishable from code somebody wrote — which is what the promise
// is actually about.
func keptRepairBox(t *testing.T, parent string) (string, *box.Box) {
	t.Helper()
	repository, commit := keptRepairRepository(t, parent)
	created, err := box.Create(context.Background(), box.Request{Repository: repository, Commit: commit})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := created.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	return repository, created
}

// keptRepairRepository is a repository holding kept's own corpus and source, cut
// where the provider grants nothing.
//
// It is a copy rather than the checkout this test runs in: a box is created
// under <repository>/.kept/boxes, and cutting one inside the worktree a test
// runs from would write into it.
//
// Three promises are deliberately left out of the copy, and the reason is not
// tidiness. `core-packages-are-tested` measures with `go/tests` over
// `./internal/...`, `the-confinement-suite-passes` measures `./internal/confine`
// the same way, and `the-tree-vets-clean` runs `go vet ./...`; any of the three
// shells the Go toolchain into the box — the first would run this very file,
// dispatch a second live session from inside a re-derivation and spend the
// operator's credential recursively, and the second would cut boxes and run
// canaries inside the box that is already one. Nothing that remains links to any
// of them, so the corpus that remains is whole.
func keptRepairRepository(t *testing.T, parent string) (string, string) {
	t.Helper()
	source := moduleRoot(t)
	root, err := os.MkdirTemp(parent, "kept-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"internal", "docs"} {
		if err := os.CopyFS(filepath.Join(root, name), os.DirFS(filepath.Join(source, name))); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"core-packages-are-tested.md", "the-confinement-suite-passes.md", "the-tree-vets-clean.md"} {
		if err := os.Remove(filepath.Join(root, "docs", "promise", name)); err != nil {
			t.Fatal(err)
		}
	}
	// And the two files that are kept's own account of this proof: the one that
	// says what the break is, and the one that says what a session carrying a
	// packet is expected to do about it. See claudeRepairAnswerKey and
	// claudeRepairAccount. Package `repair` still compiles without the first and
	// nothing in the corpus links to the second, so what the box holds is a
	// coherent tree rather than a hollowed-out one.
	for _, name := range []string{claudeRepairAnswerKey, claudeRepairAccount} {
		if err := os.Remove(filepath.Join(root, filepath.FromSlash(name))); err != nil {
			t.Fatal(err)
		}
	}
	// go.mod because the package paths the instrument reports come from it, and
	// .gitignore because kept's own derived tier is ignored there — without it
	// the stat cache a re-derivation writes into the box would arrive in the
	// repair's diff as though an agent had put it there.
	for _, name := range []string{"go.mod", ".gitignore"} {
		src, err := os.ReadFile(filepath.Join(source, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), src, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	breakTheGatherPromise(t, root)
	assertOnlyTheCodeUnderRepairNamesTheBreak(t, root)

	// One commit, with the break already in it. See the box request for why it
	// is committed rather than overlaid.
	git(t, root, "init", "-q")
	git(t, root, "config", "user.email", "test@example.com")
	git(t, root, "config", "user.name", "test")
	git(t, root, "add", "--all")
	git(t, root, "commit", "-qm", "origin")
	return root, strings.TrimSpace(git(t, root, "rev-parse", "HEAD"))
}

// operatorRepair is the operator's own per-machine repair configuration, read
// the way a dispatcher reads it.
//
// Nothing here reaches a keychain, and nothing reads a secret: `Credential` is a
// reference the confinement provider resolves. A machine that has not configured
// one skips, because a credential nobody minted is not a broken adapter — which
// is the same reason the version gate skips rather than failing.
func operatorRepair(t *testing.T) config.Repair {
	t.Helper()
	user, err := config.ReadUser()
	if err != nil {
		t.Fatal(err)
	}
	repair := user.Repair
	switch {
	case repair.Runtime != string(thread.Claude):
		t.Skipf("this machine's configured repair runtime is %q, and this is the Claude adapter's evidence", repair.Runtime)
	case repair.Executable == "":
		t.Skip("this machine's configuration names no repair executable")
	case repair.Credential == "":
		t.Skip("this machine's configuration names no repair credential reference")
	case repair.CredentialExpires.IsZero():
		t.Skip("this machine's configuration does not say when the repair credential lapses")
	}
	if _, err := os.Stat(repair.Executable); err != nil {
		t.Skipf("the configured repair runtime is not installed at %s", repair.Executable)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	reported, err := exec.CommandContext(ctx, repair.Executable, "--version").Output()
	if err != nil {
		t.Skip("the configured repair runtime did not report a version: " + err.Error())
	}
	version, _, _ := strings.Cut(strings.TrimSpace(string(reported)), " ")
	if !thread.ClaudeSupports(version) {
		t.Skipf("the configured runtime is %s, which this adapter has not reviewed", version)
	}
	return repair
}

func receiptKinds(entries []thread.Entry) map[string]int {
	kinds := map[string]int{}
	for kind := range strings.SplitSeq(thread.ReceiptText(entries), ",") {
		kinds[kind]++
	}
	return kinds
}

// assertProjectionsReferenceNativeEvents holds every projection to decision
// 0052's rule: a projection cannot manufacture a cost, turn or transcript the
// runtime did not report, so each one names the native event it came from and
// that event is in the same receipt.
func assertProjectionsReferenceNativeEvents(t *testing.T, entries []thread.Entry) {
	t.Helper()
	refs := map[string]string{}
	for _, entry := range entries {
		if entry.Native != nil {
			refs[entry.Native.Ref] = entry.Native.Type
		}
	}
	for i, entry := range entries {
		var ref, kind string
		switch {
		case entry.Transcript != nil:
			ref, kind = entry.Transcript.NativeRef, "transcript"
		case entry.Cost != nil:
			ref, kind = entry.Cost.NativeRef, "cost"
		case entry.Turn != nil:
			ref, kind = entry.Turn.NativeRef, "turn"
		default:
			continue
		}
		if ref == "" {
			t.Fatalf("entry %d is a %s projection naming no native event", i, kind)
		}
		if _, found := refs[ref]; !found {
			t.Fatalf("entry %d is a %s projection referring to %s, which this receipt does not hold", i, kind, ref)
		}
	}
}

func assertNativeTypes(t *testing.T, entries []thread.Entry, want ...string) {
	t.Helper()
	seen := map[string]bool{}
	for _, entry := range entries {
		if entry.Native != nil {
			seen[entry.Native.Type] = true
		}
	}
	for _, typ := range want {
		if !seen[typ] {
			t.Fatalf("the live Claude receipt retained no %s native event: %v", typ, seen)
		}
	}
}

// assertStoredPacketIsExact reads back the bytes the parent stored before the
// launch. Decision 0052 says an adapter never rebuilds a packet from a later
// corpus or ledger read, and this is the receipt's own copy compared against the
// one assembled.
func assertStoredPacketIsExact(t *testing.T, store ledger.Store, entries []thread.Entry, packet thread.Packet) {
	t.Helper()
	var recorded int
	for _, entry := range entries {
		if entry.Packet == nil {
			continue
		}
		recorded++
		stored, err := ledger.ReadArtifact(store, *entry.Packet)
		if err != nil {
			t.Fatal(err)
		}
		if string(stored) != string(packet.Bytes()) {
			t.Fatalf("the receipt's packet is %d bytes and the one assembled is %d", len(stored), len(packet.Bytes()))
		}
	}
	if recorded != 1 {
		t.Fatalf("the receipt records %d packets, and a session receives one", recorded)
	}
}

// sawPacketMarker looks for the packet's marker in what the runtime produced.
// The promise id is the marker: the adapter puts it nowhere but the packet, so
// finding it in the session's own evidence is what says the bytes arrived.
func sawPacketMarker(t *testing.T, store ledger.Store, entries []thread.Entry) bool {
	t.Helper()
	for _, entry := range entries {
		if entry.Native != nil && strings.Contains(string(entry.Native.Payload), claudeRepairPromise) {
			return true
		}
		if entry.Transcript == nil {
			continue
		}
		src, err := ledger.ReadArtifact(store, entry.Transcript.Artifact)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(src), claudeRepairPromise) {
			return true
		}
	}
	return false
}

// claudeSessionSummary is the session's own account, compressed to what a
// person reading a failure needs: which tools it reached for, and how it said
// it ended. It is a diagnostic and never an input to a verdict — the assertion
// it sits above re-derives the promise from the box and reads none of this.
func claudeSessionSummary(entries []thread.Entry) string {
	var out []string
	for _, entry := range entries {
		if entry.Native == nil {
			continue
		}
		switch entry.Native.Type {
		case "assistant":
			var event struct {
				Message struct {
					Content []struct {
						Type string          `json:"type"`
						Name string          `json:"name"`
						Text string          `json:"text"`
						In   json.RawMessage `json:"input"`
					} `json:"content"`
				} `json:"message"`
			}
			if json.Unmarshal(entry.Native.Payload, &event) != nil {
				continue
			}
			for _, block := range event.Message.Content {
				switch block.Type {
				case "tool_use":
					out = append(out, "  tool "+block.Name+" "+clip(string(block.In), 240))
				case "text":
					out = append(out, "  said "+clip(block.Text, 400))
				}
			}
		case "result":
			out = append(out, "  result "+clip(string(entry.Native.Payload), 800))
		}
	}
	return strings.Join(out, "\n")
}

func clip(src string, limit int) string {
	src = strings.Join(strings.Fields(src), " ")
	if len(src) <= limit {
		return src
	}
	return src[:limit] + "…"
}

// dispatchPolicies is the ratified authority that binds this act, read out of
// the box's own corpus.
//
// Decision 0052 says a packet carries the policies that bind dispatch, and
// leaving them out makes the packet smaller than the one a dispatcher
// assembles — which is a difference this proof cannot afford, because the whole
// question it is about is what the runtime does with what it is given. Which
// policies those are is not this test's judgement either: a policy declares the
// act it is checked before, and this collects the ones that say dispatch.
//
// Ratified is set here for the reason it is on the promise: nothing in the tree
// derives standing yet, and the packet's own guard already refuses an
// unratified policy.
func dispatchPolicies(t *testing.T, c *corpus.Corpus) []thread.RatifiedDispatchPolicy {
	t.Helper()
	var policies []thread.RatifiedDispatchPolicy
	for _, entry := range c.All() {
		art := entry.Artifact
		applies, _ := art.Fields["applies_to"].(string)
		if applies != "dispatch" {
			continue
		}
		kind, _ := corpus.SplitRef(art.ID)
		sc, known := c.Schemas.For(kind)
		if !known {
			t.Fatalf("the box's corpus declares no %s schema", kind)
		}
		hash, err := canonical.Bind(entry, sc)
		if err != nil {
			t.Fatal(err)
		}
		onFail, _ := art.Fields["on_fail"].(string)
		policies = append(policies, thread.RatifiedDispatchPolicy{
			ID:        art.ID,
			Hash:      hash.Digest,
			Norm:      hash.Norm,
			Ratified:  true,
			Statement: art.Statement,
			AppliesTo: applies,
			Gather:    art.Gather,
			HoldWhen:  art.HoldWhen,
			OnFail:    onFail,
		})
	}
	if len(policies) == 0 {
		t.Fatal("the box's corpus holds no policy that binds a dispatch, so the packet would carry authority this one does not have")
	}
	// Sorted, because a packet assembled twice from one corpus must be the same
	// bytes and map iteration is not an order.
	slices.SortFunc(policies, func(a, b thread.RatifiedDispatchPolicy) int { return strings.Compare(a.ID, b.ID) })
	return policies
}

// lifecycleText is the parent-observed sequence, for a failure that needs to
// say whether the runtime ran at all.
func lifecycleText(entries []thread.Entry) string {
	var events []string
	for _, entry := range entries {
		if entry.Lifecycle == nil {
			continue
		}
		text := string(entry.Lifecycle.Event)
		if entry.Lifecycle.Exit != nil {
			if code := entry.Lifecycle.Exit.Code; code != nil {
				text += fmt.Sprintf("(%d)", *code)
			}
			if signal := entry.Lifecycle.Exit.Signal; signal != "" {
				text += "(" + signal + ")"
			}
		}
		events = append(events, text)
	}
	return strings.Join(events, " -> ")
}
