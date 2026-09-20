//go:build !windows

package repair

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/config"
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/thread"
)

// This file is whole repair episodes, end to end, with one stand-in where Claude
// Code would be for all three kinds of session: the box is cut, the gap
// measured, the provider verified by its real canary, the stand-in dispatched
// confined under the real provider as the repairer, re-derived, anchored, and
// then dispatched again into a read-only box as the adversarial pass — and
// again as a rework when the pass rules reject. It spends no credential and no
// model: the stand-in answers to no upstream, and what it rules is fixed when it
// is built.
//
// One of its passes answers the way a runtime does when no model answers at all,
// which is the shape recorded in internal/thread/testdata/claude/synthetic.jsonl.

// episodeRuntimeSource is the stand-in. It tells which session it is from the
// ask it was launched with, which is the channel that says so, and never from
// anything in the input.
//
// As a repairer it removes the artifact that breaks the fixture's promise,
// unless its mode is "leave-broken". As a reworker it appends a line to a file
// inside the repair scope, so the tree moves and the promise still holds. As an
// adversarial pass it argues one sentence and ends with the ruling its build
// was given for this attempt — counted from the earlier attempts its brief
// carries — where "garbled" is a near miss and "none" is no ruling line at all.
// When window is set, the repair sessions report that five-hour utilization.
const episodeRuntimeSource = `package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

var (
	mode    = "repair"
	rulings = "accept"
	window  = ""
	version = "2.1.258"
)

func emit(value any) {
	line, _ := json.Marshal(value)
	fmt.Println(string(line))
}

func say(text string) {
	emit(map[string]any{"type": "assistant", "message": map[string]any{"content": []any{map[string]any{"type": "text", "text": text}}}})
}

func finish() {
	emit(map[string]any{"type": "result", "subtype": "success", "total_cost_usd": 0, "num_turns": 1})
	os.Exit(0)
}

func main() {
	ask := ""
	for i, arg := range os.Args[1:] {
		if arg == "--version" {
			fmt.Println(version + " (Claude Code)")
			return
		}
		if arg == "--append-system-prompt" && i+2 < len(os.Args) {
			ask = os.Args[i+2]
		}
	}
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	var message struct {
		Message struct {
			Content []struct {
				Text string ` + "`json:\"text\"`" + `
			} ` + "`json:\"content\"`" + `
		} ` + "`json:\"message\"`" + `
	}
	_ = json.Unmarshal([]byte(line), &message)
	input := ""
	if len(message.Message.Content) > 0 {
		input = message.Message.Content[0].Text
	}
	emit(map[string]any{"type": "system", "subtype": "init", "model": "claude-sonnet-5"})

	switch {
	case strings.Contains(ask, "adversarial assessor"):
		// The series, from wherever the brief keeps it. A brief omits what is
		// byte for byte in the packet beside it and says so, so a reader that
		// looked in one place would see no earlier attempts at all and argue as
		// though this were the first — which is the mistake the flag exists to
		// stop, made here so that it is made somewhere it fails.
		var brief struct {
			Earlier         []json.RawMessage ` + "`json:\"earlier\"`" + `
			EarlierInPacket bool              ` + "`json:\"earlier_in_packet\"`" + `
			Packet          struct {
				Rework struct {
					Earlier []json.RawMessage ` + "`json:\"earlier\"`" + `
				} ` + "`json:\"rework\"`" + `
			} ` + "`json:\"packet\"`" + `
		}
		_ = json.Unmarshal([]byte(input), &brief)
		earlier := brief.Earlier
		if brief.EarlierInPacket {
			earlier = brief.Packet.Rework.Earlier
		}
		number := len(earlier) + 1
		all := strings.Split(rulings, ",")
		ruling := all[len(all)-1]
		if number <= len(all) {
			ruling = all[number-1]
		}
		if ruling == "synthetic" {
			// What the runtime writes when it answers instead of a model: one
			// assistant message it composed itself, and a result naming the
			// failure. Nothing here is a model's words, so nothing here is an
			// argument.
			emit(map[string]any{"type": "assistant", "message": map[string]any{
				"model":   "<synthetic>",
				"role":    "assistant",
				"content": []any{map[string]any{"type": "text", "text": "You've hit your session limit"}},
			}, "is_api_error_message": true})
			emit(map[string]any{"type": "result", "subtype": "success", "is_error": true,
				"terminal_reason": "api_error", "api_error_status": 429, "total_cost_usd": 0, "num_turns": 1})
			os.Exit(1)
		}
		text := fmt.Sprintf("The case against attempt %d: the test named for the property never asserts it.\n\n", number)
		switch ruling {
		case "garbled":
			text += "Ruling: Maybe"
		case "none":
			text += "That is the whole of it."
		default:
			text += "ruling: " + ruling
		}
		say(text)
		finish()
	case strings.Contains(ask, "reworking a repair"):
		var packet struct {
			Rework struct {
				Earlier []json.RawMessage ` + "`json:\"earlier\"`" + `
			} ` + "`json:\"rework\"`" + `
		}
		_ = json.Unmarshal([]byte(input), &packet)
		f, err := os.OpenFile("docs/rework.txt", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(4)
		}
		fmt.Fprintf(f, "attempt %d answered the case\n", len(packet.Rework.Earlier)+1)
		_ = f.Close()
	default:
		if mode != "leave-broken" {
			if err := os.Remove("docs/promise/spare.md"); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(4)
			}
		} else if err := os.WriteFile("docs/tried.txt", []byte("looked, changed a note\n"), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(4)
		}
	}
	if window != "" {
		emit(map[string]any{"type": "rate_limit_event", "rate_limit_info": map[string]any{"unifiedWindows": map[string]any{"five_hour": map[string]any{"utilization": json.Number(window), "resetsAt": 1788000000}}}})
	}
	say("done")
	finish()
}
`

// episode is a repair workspace with the stand-in configured as both the
// repairer and the assessor.
type episode struct {
	w   *repairWorkspace
	req Request
}

func newEpisode(t *testing.T, ldflags ...string) *episode {
	t.Helper()
	provider := liveProvider(t)
	kept := buildKept(t)
	previous := keptExecutable
	keptExecutable = func() (string, error) { return kept, nil }
	t.Cleanup(func() { keptExecutable = previous })

	w := newRepairWorkspaceUnder(t, ungrantedDir(t))
	w.write("docs/promise/spare.md", promiseFile("spare", "docs/**", "obs.c.ok"))
	w.commit()

	source := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(source, []byte(episodeRuntimeSource), 0o644); err != nil {
		t.Fatal(err)
	}
	stub := filepath.Join(ungrantedDir(t), "claude")
	buildGo(t, source, stub, ldflags...)

	secret := filepath.Join(ungrantedDir(t), "operator-credential")
	if err := os.WriteFile(secret, []byte("not-a-credential"), 0o600); err != nil {
		t.Fatal(err)
	}
	runtime := config.Repair{
		Runtime:           string(thread.Claude),
		Executable:        stub,
		Credential:        "file://" + secret,
		CredentialExpires: time.Now().Add(365 * 24 * time.Hour),
	}
	req := w.request("promise:counted")
	req.Provider = filepath.Base(provider)
	req.By = "Test <test@example.com>"
	req.Operator = runtime
	req.Assessor = config.Assess{Repair: runtime}
	req.Deadlines.Agent = 2 * time.Minute
	return &episode{w: w, req: req}
}

func buildGo(t *testing.T, source, binary string, ldflags ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-ldflags", strings.Join(ldflags, " "), "-o", binary, source)
	cmd.Dir = filepath.Dir(source)
	cmd.Env = append(os.Environ(), "GO111MODULE=off")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build the stand-in runtime: %v\n%s", err, output)
	}
}

func bounds(attempts int, automatic bool) Bounds {
	return Bounds{Attempts: attempts, From: "the test", Automatic: automatic}
}

// packets is every packet a thread's receipt names, read back, oldest first.
func packets(t *testing.T, store ledger.Store, id thread.ID) []thread.Contents {
	t.Helper()
	entries, _, err := thread.ReadReceipt(store, id)
	if err != nil {
		t.Fatal(err)
	}
	var out []thread.Contents
	for _, attempt := range thread.Attempts(entries) {
		contents, err := lastPacket(store, attempt)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, contents)
	}
	return out
}

func dispositionsNaming(t *testing.T, store ledger.Store, promise string) []ledger.Disposition {
	t.Helper()
	records, _, err := ledger.ReadDispositions(store, promise)
	if err != nil {
		t.Fatal(err)
	}
	return records
}

// TestARejectIsReworkedInTheSameThreadAndTheNextPacketCarriesTheArgument is the
// loop, whole. Attempt one repairs the promise, and its pass rules reject; the
// run does not stop, and attempt two is cut at attempt one's anchor in the same
// thread, carrying the argument verbatim; its pass accepts, and the episode is
// ready to land with an anchor naming both attempts' work against the base.
//
// It is also the ceiling's other half: a runtime that reports no window refuses
// nothing, however low the ceiling. See docs/decision/0086.
func TestARejectIsReworkedInTheSameThreadAndTheNextPacketCarriesTheArgument(t *testing.T) {
	e := newEpisode(t, "-X", "main.rulings=reject,accept")
	b := bounds(3, true)
	b.WindowCeiling = 0.01

	var events []Event
	e.req.Events = func(event Event) { events = append(events, event) }
	ep, err := Run(context.Background(), e.req, b, "")
	if err != nil {
		t.Fatal(err)
	}
	if ep.End != Ready {
		t.Fatalf("the episode ended %s: %v", ep.End, ep.Why)
	}
	if len(ep.Attempts) != 2 {
		t.Fatalf("the run opened %d attempts, want a repair and one rework", len(ep.Attempts))
	}
	first, second := ep.Attempts[0], ep.Attempts[1]
	if first.Thread != second.Thread || first.Thread != ep.Thread {
		t.Fatalf("the rework opened thread %s, and the episode is thread %s", second.Thread, first.Thread)
	}
	// The verdict is the re-derivation's whatever the pass ruled.
	if first.Status != Repaired || first.After.Value != model.Kept {
		t.Fatalf("attempt one reads %s (%s) though its pass rejected it, and a ruling is never an input to the verdict", first.Status, first.After.Value)
	}
	if first.Argument == nil || first.Argument.Ruling != thread.RulingReject {
		t.Fatalf("attempt one's pass ruled %v", first.Argument)
	}
	if !second.Rework || second.Commit != first.Anchor.Commit {
		t.Fatalf("the rework's box was cut at %s, want attempt one's anchor %s", second.Commit, first.Anchor.Commit)
	}
	if second.Argument == nil || second.Argument.Ruling != thread.RulingAccept {
		t.Fatalf("attempt two's pass ruled %v", second.Argument)
	}

	carried := packets(t, e.w.store, ep.Thread)
	if len(carried) != 2 || carried[0].Rework != nil || carried[1].Rework == nil {
		t.Fatalf("the thread holds %d packets, want a repair's and then a rework's", len(carried))
	}
	earlier := carried[1].Rework.Earlier
	if len(earlier) != 1 {
		t.Fatalf("the rework carries %d earlier attempts, want 1", len(earlier))
	}
	if earlier[0].Argument != first.Argument.Said || !strings.Contains(earlier[0].Argument, "ruling: reject") {
		t.Fatalf("the rework packet does not carry attempt one's argument verbatim: %q", earlier[0].Argument)
	}
	if earlier[0].Ruling != thread.RulingReject || earlier[0].Anchor == nil || earlier[0].Anchor.Commit != first.Anchor.Commit {
		t.Fatalf("the rework packet's earlier attempt is %#v", earlier[0])
	}
	if carried[1].Rework.Base != first.Commit || carried[1].Rework.Tip != first.Anchor.Commit {
		t.Fatalf("the rework names base %s and tip %s", carried[1].Rework.Base, carried[1].Rework.Tip)
	}

	// The anchor a land takes is the whole episode's change against its base.
	if second.Anchor.Parent != first.Commit {
		t.Fatalf("the rework's anchor has parent %s, want the episode's base %s", second.Anchor.Parent, first.Commit)
	}
	changed := git(t, e.w.root, "diff", "--name-only", second.Anchor.Parent, second.Anchor.Commit)
	if !strings.Contains(changed, "docs/promise/spare.md") || !strings.Contains(changed, "docs/rework.txt") {
		t.Fatalf("the episode's anchor changes %q, want both attempts' work", changed)
	}

	// Two acts, one per attempt, both naming the thread; nothing escalated.
	var acts int
	for _, record := range dispositionsNaming(t, e.w.store, "promise:counted") {
		switch record.Outcome {
		case ledger.OutcomeAct:
			acts++
			if record.Thread != string(ep.Thread) {
				t.Fatalf("an act names thread %q", record.Thread)
			}
		case ledger.OutcomeEscalate:
			t.Fatalf("a ready episode escalated: %#v", record)
		}
	}
	if acts != 2 {
		t.Fatalf("the episode recorded %d acts, want one per attempt", acts)
	}

	// And a phase line's worth of event for each phase, in order.
	var kinds []string
	for _, event := range events {
		kinds = append(kinds, string(event.Kind))
	}
	said := strings.Join(kinds, " ")
	for _, want := range []string{
		"attempt-started session-ended re-derived anchored pass-started pass-ended assessed decided",
		"attempt-started session-ended re-derived anchored pass-started pass-ended assessed decided",
	} {
		if !strings.Contains(said, want) {
			t.Fatalf("the events are %q, want each attempt to report %q", said, want)
		}
	}
	last := events[len(events)-1]
	if last.Kind != Decided || last.Action != ActionReady {
		t.Fatalf("the last event is %#v, want the ready decision", last)
	}
	for _, event := range events {
		if event.Kind == Decided && event.Action == ActionRework && !strings.Contains(event.Window, "refuses nothing") {
			t.Fatalf("the rework decision says the ceiling read %q, want that nobody reported a window", event.Window)
		}
	}
}

// TestWithAutomaticReworkOffARejectEscalates is the opt-out: the same reject,
// and the run hands the episode to a person instead of reworking it, naming the
// command that would continue it.
func TestWithAutomaticReworkOffARejectEscalates(t *testing.T) {
	e := newEpisode(t, "-X", "main.rulings=reject")
	ep, err := Run(context.Background(), e.req, bounds(3, false), "")
	if err != nil {
		t.Fatal(err)
	}
	if ep.End != Escalated || len(ep.Attempts) != 1 {
		t.Fatalf("the run ended %s after %d attempts, want an escalation after one", ep.End, len(ep.Attempts))
	}
	if !strings.Contains(strings.Join(ep.Why, "; "), "kept repair --rework "+string(ep.Thread)) {
		t.Fatalf("the escalation does not name the command that continues it: %v", ep.Why)
	}
	assertEscalated(t, e, ep)
}

// TestTheAttemptsBudgetEscalatesAtItsLimit breaks the loop's bound: a pass that
// rejects every attempt, and a run that stops at its bound with an escalation
// rather than a quiet end or another attempt — at the default of three, which
// is also the reason a reject reworks until the budget is spent, and at a bound
// a repository configured.
func TestTheAttemptsBudgetEscalatesAtItsLimit(t *testing.T) {
	for _, c := range []struct {
		name       string
		repository int
		want       int
	}{
		{"the default", 0, DefaultAttempts},
		{"a configured bound", 2, 2},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := newEpisode(t, "-X", "main.rulings=reject")
			b, err := ResolveBounds(c.repository, config.Bounds{}, 0, true)
			if err != nil {
				t.Fatal(err)
			}
			ep, err := Run(context.Background(), e.req, b, "")
			if err != nil {
				t.Fatal(err)
			}
			if ep.End != Escalated || len(ep.Attempts) != c.want {
				t.Fatalf("the run ended %s after %d attempts, want an escalation at the bound of %d", ep.End, len(ep.Attempts), c.want)
			}
			if !strings.Contains(strings.Join(ep.Why, "; "), fmt.Sprintf("opened %d of the %d attempts", c.want, c.want)) {
				t.Fatalf("the escalation does not say the budget was spent: %v", ep.Why)
			}
			assertEscalated(t, e, ep)
		})
	}
}

// TestTheWindowCeilingRefusesTheNextAttempt is the ceiling's refusing half: the
// repair session reports a window above it, the pass rejects, and the rework
// the reject would open is refused and escalated.
func TestTheWindowCeilingRefusesTheNextAttempt(t *testing.T) {
	e := newEpisode(t, "-X", "main.rulings=reject", "-X", "main.window=0.95")
	b := bounds(3, true)
	b.WindowCeiling = 0.9
	ep, err := Run(context.Background(), e.req, b, "")
	if err != nil {
		t.Fatal(err)
	}
	if ep.End != Escalated || len(ep.Attempts) != 1 {
		t.Fatalf("the run ended %s after %d attempts, want the ceiling to refuse the second", ep.End, len(ep.Attempts))
	}
	if !strings.Contains(strings.Join(ep.Why, "; "), "five_hour reads 0.95, at or above the usage-window ceiling of 0.9") {
		t.Fatalf("the escalation does not name the window and the ceiling: %v", ep.Why)
	}
	assertEscalated(t, e, ep)
}

// TestAPassThatRulesNothingReadableEscalatesAndIsNeverAnAccept breaks the token
// both ways a pass could: a near miss, and no ruling line at all. Neither is
// ready to land, and neither is reworked either.
func TestAPassThatRulesNothingReadableEscalatesAndIsNeverAnAccept(t *testing.T) {
	for _, ruling := range []string{"garbled", "none"} {
		t.Run(ruling, func(t *testing.T) {
			e := newEpisode(t, "-X", "main.rulings="+ruling)
			ep, err := Run(context.Background(), e.req, bounds(3, true), "")
			if err != nil {
				t.Fatal(err)
			}
			if ep.End != Escalated || len(ep.Attempts) != 1 {
				t.Fatalf("a pass that ruled %s ended the run %s after %d attempts", ruling, ep.End, len(ep.Attempts))
			}
			if !strings.Contains(strings.Join(ep.Why, "; "), "made no ruling kept can read") {
				t.Fatalf("the escalation does not say the ruling could not be read: %v", ep.Why)
			}
			assertEscalated(t, e, ep)
		})
	}
}

// TestANoticeTheRuntimeWroteItselfIsNotTheArgumentAndTheEpisodeSaysSo is the
// whole of the second measured episode in this repository's ledger, reproduced
// without spending anything.
//
// A pass was launched onto an account at its rate limit. The runtime answered it
// instead of a model: one assistant message it composed itself, and a result
// naming a 429. kept projected that notice as the pass's transcript and filed the
// 65 bytes as the pass's argument. The episode escalated for the right reason and
// the record said the pass argued — so anybody reading the thread afterwards
// would find an argument nobody made, attributed to a model that never ran.
//
// What must happen instead is the row decision 0086 already has: an assessor is
// configured, the pass reached no argument, and that goes to a person. No new
// outcome, and the runtime's own words for why in the detail, because *nothing to
// say* and *never reached a model* are otherwise the same receipt.
func TestANoticeTheRuntimeWroteItselfIsNotTheArgumentAndTheEpisodeSaysSo(t *testing.T) {
	e := newEpisode(t, "-X", "main.rulings=synthetic")
	ep, err := Run(context.Background(), e.req, bounds(3, true), "")
	if err != nil {
		t.Fatal(err)
	}
	if ep.End != Escalated || len(ep.Attempts) != 1 {
		t.Fatalf("the run ended %s after %d attempts", ep.End, len(ep.Attempts))
	}
	pass := ep.Attempts[0].Argument
	if pass == nil || pass.Status != Unargued {
		t.Fatalf("the pass is recorded %#v, want unargued", pass)
	}
	if pass.Argument != (thread.ArgumentRecord{}) || pass.Said != "" {
		t.Fatalf("an argument was filed from a notice the runtime wrote itself: %#v / %q", pass.Argument, pass.Said)
	}
	// The runtime's own account of why, so a reader is not left to infer it from
	// an empty transcript.
	if !strings.Contains(pass.Detail, "api_error (429)") {
		t.Fatalf("the detail does not carry the runtime's terminal error: %q", pass.Detail)
	}
	if why := strings.Join(ep.Why, "; "); !strings.Contains(why, "reached no argument") {
		t.Fatalf("the escalation does not say the pass reached no argument: %v", ep.Why)
	}
	assertEscalated(t, e, ep)

	// And nothing was thrown away: the words are still in the receipt as the
	// runtime's own native event, which is what keeps a reader who knows what
	// the runtime meant able to see it.
	entries, _, err := thread.ReadReceipt(e.w.store, ep.Thread)
	if err != nil {
		t.Fatal(err)
	}
	notice := false
	for _, entry := range entries {
		if entry.Native != nil && strings.Contains(string(entry.Native.Payload), "session limit") {
			notice = true
		}
		if entry.Argument != nil {
			t.Fatal("the receipt holds an argument entry for a pass that reached no argument")
		}
	}
	if !notice {
		t.Fatal("the runtime's own notice was not retained as a native event")
	}
}

// TestAnAcceptDoesNotMakeABrokenPromiseReady is the verdict's independence from
// the other side: the pass accepts an attempt that left the promise not-kept,
// and the run reworks it anyway, because what the tree measures is the answer.
func TestAnAcceptDoesNotMakeABrokenPromiseReady(t *testing.T) {
	e := newEpisode(t, "-X", "main.mode=leave-broken", "-X", "main.rulings=accept")
	ep, err := Run(context.Background(), e.req, bounds(1, true), "")
	if err != nil {
		t.Fatal(err)
	}
	first := ep.Attempts[0]
	if first.Status != Unrepaired || first.Argument == nil || first.Argument.Ruling != thread.RulingAccept {
		t.Fatalf("attempt one is %s with ruling %v", first.Status, first.Argument)
	}
	if ep.End == Ready {
		t.Fatal("a promise that reads not-kept was made ready to land by an accept")
	}
	if !strings.Contains(strings.Join(ep.Why, "; "), "still reads not-kept") {
		t.Fatalf("the run does not say the promise still reads not-kept: %v", ep.Why)
	}
}

func assertEscalated(t *testing.T, e *episode, ep *Episode) {
	t.Helper()
	if !ep.Recorded || ep.Handoff.Outcome != ledger.OutcomeEscalate || ep.Handoff.Thread != string(ep.Thread) {
		t.Fatalf("the escalation was recorded %v as %#v", ep.Recorded, ep.Handoff)
	}
	records := dispositionsNaming(t, e.w.store, "promise:counted")
	last := records[len(records)-1]
	if last.Outcome != ledger.OutcomeEscalate || last.Thread != string(ep.Thread) {
		t.Fatalf("the disposition in force is %#v, want the escalation naming thread %s", last, ep.Thread)
	}
	acts := 0
	for _, record := range records {
		if record.Outcome == ledger.OutcomeAct {
			acts++
		}
	}
	if acts != len(ep.Attempts) {
		t.Fatalf("%d acts for %d attempts: each attempt writes exactly one", acts, len(ep.Attempts))
	}
}

// TestAReworkOnAPersonsWordContinuesTheThreadWithItsContext is `kept repair
// --rework <thread>`: an episode a person was handed, reworked on their word.
// The next attempt is numbered after the thread's last, cut at its anchor, in
// the same thread, and its packet carries what the pass argued — and the
// escalation it answers no longer stands.
func TestAReworkOnAPersonsWordContinuesTheThreadWithItsContext(t *testing.T) {
	e := newEpisode(t, "-X", "main.rulings=reject,accept")
	first, err := Run(context.Background(), e.req, bounds(3, false), "")
	if err != nil {
		t.Fatal(err)
	}
	if first.End != Escalated {
		t.Fatalf("the first run ended %s, want it handed to a person", first.End)
	}
	earlier := first.Attempts[0]

	req := e.w.request("")
	req.Provider, req.By, req.Operator, req.Assessor, req.Deadlines = e.req.Provider, e.req.By, e.req.Operator, e.req.Assessor, e.req.Deadlines
	ep, err := Run(context.Background(), req, bounds(1, true), first.Thread)
	if err != nil {
		t.Fatal(err)
	}
	if ep.Promise != "promise:counted" {
		t.Fatalf("the rework resolved promise %q from its thread", ep.Promise)
	}
	if ep.End != Ready || len(ep.Attempts) != 1 {
		t.Fatalf("the rework ended %s after %d attempts: %v", ep.End, len(ep.Attempts), ep.Why)
	}
	reworked := ep.Attempts[0]
	if reworked.Thread != first.Thread || reworked.Attempt != 2 || !reworked.Rework {
		t.Fatalf("the rework is attempt %d of thread %s (rework %v), want attempt 2 of %s", reworked.Attempt, reworked.Thread, reworked.Rework, first.Thread)
	}
	if reworked.Commit != earlier.Anchor.Commit {
		t.Fatalf("the rework's box was cut at %s, want the thread's anchor %s", reworked.Commit, earlier.Anchor.Commit)
	}
	carried := packets(t, e.w.store, ep.Thread)
	if len(carried) != 2 || carried[1].Rework == nil || carried[1].Rework.Earlier[0].Argument != earlier.Argument.Said {
		t.Fatal("the rework packet does not carry the escalated attempt's argument verbatim")
	}

	records := dispositionsNaming(t, e.w.store, "promise:counted")
	c := e.w.load()
	_, standing, err := subject(Request{Corpus: c, Acts: req.Acts, Promise: "promise:counted"})
	if err != nil {
		t.Fatal(err)
	}
	if _, open := ledger.OpenEscalation(records, nil, standing.Hash.String()); open {
		t.Fatal("the escalation still stands after a person reworked the thread")
	}
}

// TestAReworkIsRefusedForAThreadThatCannotBeReworked holds the refusals that no
// machine and no moment would change, and records none of them: a thread this
// ledger has no receipt for, and one whose promise was ratified at another
// wording since — a rework would measure a standard nobody holds now.
func TestAReworkIsRefusedForAThreadThatCannotBeReworked(t *testing.T) {
	e := newEpisode(t, "-X", "main.rulings=reject")
	req := e.w.request("")
	req.Provider, req.Operator, req.Assessor, req.Deadlines = e.req.Provider, e.req.Operator, e.req.Assessor, e.req.Deadlines

	_, err := Run(context.Background(), req, bounds(1, true), thread.MintID())
	var unrepairable *Unrepairable
	if !asUnrepairable(err, &unrepairable) || !strings.Contains(unrepairable.Why, "has no receipt") {
		t.Fatalf("a thread with no receipt was not refused by name: %v", err)
	}

	first, err := Run(context.Background(), e.req, bounds(1, false), "")
	if err != nil {
		t.Fatal(err)
	}
	before := len(dispositionsNaming(t, e.w.store, "promise:counted"))

	e.w.write("docs/promise/counted.md", strings.Replace(promiseFile("counted", "docs/**", "obs.c.artifacts == 7"),
		"the corpus under docs/** is what it was", "the corpus under docs/** is what it was, revised", 1))
	e.w.commit()
	e.w.ratify("promise:counted")
	req = e.w.request("")
	req.Provider, req.Operator, req.Assessor, req.Deadlines = e.req.Provider, e.req.Operator, e.req.Assessor, e.req.Deadlines
	ep, err := Run(context.Background(), req, bounds(1, true), first.Thread)
	if !asUnrepairable(err, &unrepairable) || !strings.Contains(unrepairable.Why, "ratified at a different wording") {
		t.Fatalf("a thread whose promise was revised was not refused by name: %v", err)
	}
	if len(ep.Attempts) != 0 {
		t.Fatalf("a refused rework opened %d attempts", len(ep.Attempts))
	}
	if after := len(dispositionsNaming(t, e.w.store, "promise:counted")); after != before {
		t.Fatalf("a rework refused on its thread wrote %d dispositions", after-before)
	}
}

// TestTheWindowCeilingRefusesAReworkBeforeAnythingIsCut is the ceiling at the
// start of a run: the thread's repair session reported a window above it, so a
// person's rework is refused on budget before a box exists, and the refusal is
// recorded rather than escalated, because the person who asked is the person
// being told.
func TestTheWindowCeilingRefusesAReworkBeforeAnythingIsCut(t *testing.T) {
	e := newEpisode(t, "-X", "main.rulings=reject", "-X", "main.window=0.95")
	first, err := Run(context.Background(), e.req, bounds(1, false), "")
	if err != nil {
		t.Fatal(err)
	}

	req := e.w.request("")
	req.Provider, req.Operator, req.Assessor, req.Deadlines = e.req.Provider, e.req.Operator, e.req.Assessor, e.req.Deadlines
	b := bounds(3, true)
	b.WindowCeiling = 0.9
	ep, err := Run(context.Background(), req, b, first.Thread)
	if err != nil {
		t.Fatal(err)
	}
	if ep.End != Stopped || len(ep.Attempts) != 0 {
		t.Fatalf("the rework ended %s after %d attempts, want it refused before any", ep.End, len(ep.Attempts))
	}
	if !ep.Recorded || ep.Handoff.Outcome != ledger.OutcomeRefused || ep.Handoff.Condition != ledger.Budget {
		t.Fatalf("the refusal was recorded %v as %#v, want refused on budget", ep.Recorded, ep.Handoff)
	}
	if !strings.Contains(ep.Handoff.Detail, "five_hour reads 0.95") {
		t.Fatalf("the refusal does not name the window: %s", ep.Handoff.Detail)
	}
	entries, _, err := thread.ReadReceipt(e.w.store, first.Thread)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(thread.Attempts(entries)); got != 1 {
		t.Fatalf("the thread holds %d attempts after a refused rework, want the one it had", got)
	}
}
