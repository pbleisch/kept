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
	rework  = "append"
	rulings = "accept"
	window  = ""
	resets  = "1788000000"
	version = "2.1.258"
)

// exhausted ends the session the way Claude does when the account has no
// capacity left: a window reading at its limit, a notice the runtime wrote
// itself, and a result naming the 429. The shape is thread
// 7v2p52085pvwtdepzzhz34f3ze's repairer on 2026-11-20.
func exhausted() {
	emit(map[string]any{"type": "rate_limit_event", "rate_limit_info": map[string]any{"unifiedWindows": map[string]any{"five_hour": map[string]any{"utilization": json.Number("1.01"), "resetsAt": json.Number(resets)}}}})
	emit(map[string]any{"type": "assistant", "message": map[string]any{
		"model":   "<synthetic>",
		"role":    "assistant",
		"content": []any{map[string]any{"type": "text", "text": "You've hit your session limit · resets 12:40am (America/Los_Angeles)"}},
	}, "is_api_error_message": true})
	emit(map[string]any{"type": "result", "subtype": "success", "is_error": true, "terminal_reason": "api_error",
		"api_error_status": 429, "result": "You've hit your session limit · resets 12:40am (America/Los_Angeles)", "total_cost_usd": 0, "num_turns": 1})
	os.Exit(1)
}

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
		case "out-of-reach":
			text += "ruling: out-of-reach: the fix needs internal/repair/sessions_test.go, which is outside the repair scope"
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
		switch rework {
		case "regress":
			// Another artifact under the promise's scope, so the count it
			// holds to moves away from where the earlier attempt left it.
			src, err := os.ReadFile("docs/promise/unratified.md")
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(4)
			}
			if err := os.WriteFile("docs/promise/regressed.md", []byte(strings.ReplaceAll(string(src), "unratified", "regressed")), 0o644); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(4)
			}
		case "exhausted":
			if err := os.WriteFile("docs/half.txt", []byte("half of a rework\n"), 0o644); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(4)
			}
			exhausted()
		default:
			f, err := os.OpenFile("docs/rework.txt", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(4)
			}
			fmt.Fprintf(f, "attempt %d answered the case\n", len(packet.Rework.Earlier)+1)
			_ = f.Close()
		}
	default:
		if mode != "leave-broken" {
			if err := os.Remove("docs/promise/spare.md"); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(4)
			}
			if mode == "exhausted" {
				// The repair is done in the tree and the account refuses the
				// session before it says so, which is the case that must not
				// be read as a result: the tree would re-derive kept.
				exhausted()
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

	// source and stub are the stand-in's program and where it is built, so a
	// test can rebuild it between two runs of one episode.
	source, stub string
}

// rebuild replaces the stand-in with one built from other flags, at the same
// path, so the next run of the episode dispatches a runtime that answers
// differently.
func (e *episode) rebuild(t *testing.T, ldflags ...string) {
	t.Helper()
	buildGo(t, e.source, e.stub, ldflags...)
}

func newEpisode(t *testing.T, ldflags ...string) *episode {
	t.Helper()
	return buildEpisode(t, false, ldflags...)
}

// newEpisodeOnRefs is the same episode with its ledger on the repository's own
// refs, so what a repair writes and reads goes where it goes outside a test.
func newEpisodeOnRefs(t *testing.T, ldflags ...string) *episode {
	t.Helper()
	return buildEpisode(t, true, ldflags...)
}

func buildEpisode(t *testing.T, onRefs bool, ldflags ...string) *episode {
	t.Helper()
	provider := liveProvider(t)
	kept := buildKept(t)
	previous := keptExecutable
	keptExecutable = func() (string, error) { return kept, nil }
	t.Cleanup(func() { keptExecutable = previous })

	w := buildRepairWorkspace(t, ungrantedDir(t), onRefs)
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
	return &episode{w: w, req: req, source: source, stub: stub}
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

// TestAPassThatRulesTheFixOutOfReachEscalatesAtOnce is the parity episode on
// promise:a-receipt-costs-the-ledger-about-what-it-holds, reproduced without
// spending anything: the pass says no repair confined to the promise's own
// scope can make it hold. The first time it says so the episode goes to a
// person, with the pass's own reason, and no second attempt is opened — though
// the promise holds, the checks are clear, and two attempts of the budget are
// left. A person's --rework still continues it, and the next packet carries the
// ruling. See docs/decision/0096.
func TestAPassThatRulesTheFixOutOfReachEscalatesAtOnce(t *testing.T) {
	const reason = "the fix needs internal/repair/sessions_test.go, which is outside the repair scope"
	e := newEpisode(t, "-X", "main.rulings=out-of-reach,accept")
	var events []Event
	e.req.Events = func(event Event) { events = append(events, event) }
	ep, err := Run(context.Background(), e.req, bounds(3, true), "")
	if err != nil {
		t.Fatal(err)
	}
	if ep.End != Escalated || len(ep.Attempts) != 1 {
		t.Fatalf("a pass that ruled the fix out of reach ended the run %s after %d attempts, want an escalation after one: %v", ep.End, len(ep.Attempts), ep.Why)
	}
	first := ep.Attempts[0]
	if first.Status != Repaired || first.Argument == nil || first.Argument.Ruling != thread.RulingOutOfReach || first.Argument.RulingWhy != reason {
		t.Fatalf("attempt one is %s with pass %#v", first.Status, first.Argument)
	}
	want := "the adversarial pass ruled the fix out of this repair's reach: " + reason
	if !strings.Contains(strings.Join(ep.Why, "\n"), want) {
		t.Fatalf("the escalation says %v, want %q", ep.Why, want)
	}
	assertEscalated(t, e, ep)
	if !strings.Contains(ep.Handoff.Detail, reason) {
		t.Fatalf("the escalation was recorded without the pass's reason: %q", ep.Handoff.Detail)
	}
	// The receipt holds it: the reading kept derives from the thread, not from
	// the run's memory, rules out of reach for the same reason.
	if len(ep.Readings) != 1 {
		t.Fatalf("the thread reads %d attempts, want 1", len(ep.Readings))
	}
	if argued := ep.Readings[0].Assessment.Argument; argued.Ruling != thread.RulingOutOfReach || argued.RulingWhy != reason {
		t.Fatalf("the receipt reads ruling %q (%s)", argued.Ruling, argued.RulingWhy)
	}
	var decided []Event
	for _, event := range events {
		if event.Kind == Decided {
			decided = append(decided, event)
		}
		if event.Kind == PassEnded && (event.Ruling != thread.RulingOutOfReach || event.Detail != reason) {
			t.Fatalf("the pass ended with ruling %q and detail %q", event.Ruling, event.Detail)
		}
	}
	if len(decided) != 1 || decided[0].Action != ActionEscalate {
		t.Fatalf("the run decided %#v, want one escalation", decided)
	}

	// A person's word continues it all the same, and the next attempt is shown
	// which way the case went.
	again, err := Run(context.Background(), e.req, bounds(3, true), ep.Thread)
	if err != nil {
		t.Fatal(err)
	}
	if again.End != Ready || len(again.Attempts) != 1 || again.Attempts[0].Attempt != 2 {
		t.Fatalf("the rework ended %s after %d attempts: %v", again.End, len(again.Attempts), again.Why)
	}
	carried := packets(t, e.w.store, ep.Thread)
	if len(carried) != 2 || carried[1].Rework == nil || len(carried[1].Rework.Earlier) != 1 {
		t.Fatalf("the thread holds %d packets, want the rework's carrying attempt one", len(carried))
	}
	if earlier := carried[1].Rework.Earlier[0]; earlier.Ruling != thread.RulingOutOfReach || !strings.HasSuffix(earlier.Argument, thread.OutOfReachText(reason)) {
		t.Fatalf("the rework packet carries attempt one as %#v", earlier)
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
// What must happen instead is that nothing is filed as an argument, and — since
// docs/decision/0090 — that the episode waits for the account rather than
// handing a person something no rework could fix: the pass did not conclude, so
// nothing ruled, and the runtime's own words say why.
func TestANoticeTheRuntimeWroteItselfIsNotTheArgumentAndTheEpisodeSaysSo(t *testing.T) {
	e := newEpisode(t, "-X", "main.rulings=synthetic")
	ep, err := Run(context.Background(), e.req, bounds(3, true), "")
	if err != nil {
		t.Fatal(err)
	}
	if ep.End != Waiting || len(ep.Attempts) != 1 {
		t.Fatalf("the run ended %s after %d attempts, want it waiting after one", ep.End, len(ep.Attempts))
	}
	pass := ep.Attempts[0].Argument
	if pass == nil || pass.Status != ArgueExhausted {
		t.Fatalf("the pass is recorded %#v, want exhausted", pass)
	}
	if pass.Argument != (thread.ArgumentRecord{}) || pass.Said != "" {
		t.Fatalf("an argument was filed from a notice the runtime wrote itself: %#v / %q", pass.Argument, pass.Said)
	}
	if why := strings.Join(ep.Why, "; "); !strings.Contains(why, "the adversarial pass ended because the account had no capacity left") {
		t.Fatalf("the wait does not say the account cut off the pass: %v", ep.Why)
	}
	assertWaiting(t, e, ep)

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

// TestARepairSessionTheAccountCutOffIsNotAResultAndTheEpisodeWaits is the
// attempt-three repairer of thread 7v2p52085pvwtdepzzhz34f3ze: the account
// refused the session partway, and kept re-derived what it left, anchored it,
// and put it in the land queue. Here the stand-in finishes the repair in the
// tree before the account refuses it, so the tree would re-derive kept — which
// is exactly why what such a session leaves must not be read: how far it got is
// not a decision anybody made. See docs/decision/0090.
func TestARepairSessionTheAccountCutOffIsNotAResultAndTheEpisodeWaits(t *testing.T) {
	e := newEpisodeOnRefs(t, "-X", "main.mode=exhausted", "-X", "main.resets=4102444800")
	ep, err := Run(context.Background(), e.req, bounds(3, true), "")
	if err != nil {
		t.Fatal(err)
	}
	if ep.End != Waiting || len(ep.Attempts) != 1 {
		t.Fatalf("the run ended %s after %d attempts, want it waiting after one: %v", ep.End, len(ep.Attempts), ep.Why)
	}
	res := ep.Attempts[0]
	if res.Status != Exhausted || res.Exhausted == nil || !strings.Contains(res.Exhausted.Said, "session limit") {
		t.Fatalf("the attempt is %s with %#v, want exhausted in the runtime's words", res.Status, res.Exhausted)
	}
	if !res.Exhausted.Resets.Equal(time.Unix(4102444800, 0)) {
		t.Fatalf("the reset is read as %v, want the window the runtime put at its limit", res.Exhausted.Resets)
	}
	if res.After.Value != "" || res.Anchor.Commit != "" || res.Argument != nil {
		t.Fatalf("a session the account cut off was read as a result: after %q, anchor %q, pass %#v", res.After.Value, res.Anchor.Commit, res.Argument)
	}
	if res.Box == "" {
		t.Fatal("the box holding the session's unanchored work was discarded")
	}
	if _, err := os.Stat(filepath.Join(res.Box, "docs", "promise", "spare.md")); !os.IsNotExist(err) {
		t.Fatalf("the kept box does not hold the session's work: %v", err)
	}
	ref, err := ledger.ThreadRef(string(ep.Thread))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gitErr(e.w.root, "rev-parse", "--verify", "--quiet", ref); err == nil {
		t.Fatalf("%s exists, and nothing a cut-off session left may enter the land queue", ref)
	}
	entries, _, err := thread.ReadReceipt(e.w.store, ep.Thread)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		switch {
		case entry.Anchor != nil:
			t.Fatal("the receipt names an anchor for a session the account cut off")
		case entry.Brief != nil:
			t.Fatal("a pass was dispatched over work nothing anchored")
		case entry.Measurement != nil && entry.Measurement.Side == thread.After:
			t.Fatal("what a cut-off session left was re-derived")
		}
	}
	why := strings.Join(ep.Why, "; ")
	for _, want := range []string{"the repair session ended because the account had no capacity left", "2100-01-01T00:00:00Z", "kept repair promise:counted starts again"} {
		if !strings.Contains(why, want) {
			t.Fatalf("the wait does not say %q: %v", want, ep.Why)
		}
	}
	assertWaiting(t, e, ep)
}

// TestAReworkTheAccountCutOffKeepsTheEarlierAnchorAndWaitsUntilItResets is the
// same refusal one attempt later, and what resuming it does. The thread's
// anchor stays the first attempt's, the wait names the command that resumes
// it, and a resumption before the reset the runtime named opens nothing,
// spends nothing and records nothing new.
func TestAReworkTheAccountCutOffKeepsTheEarlierAnchorAndWaitsUntilItResets(t *testing.T) {
	e := newEpisode(t, "-X", "main.rulings=reject", "-X", "main.rework=exhausted", "-X", "main.resets=4102444800")
	ep, err := Run(context.Background(), e.req, bounds(3, true), "")
	if err != nil {
		t.Fatal(err)
	}
	if ep.End != Waiting || len(ep.Attempts) != 2 {
		t.Fatalf("the run ended %s after %d attempts, want it waiting after two: %v", ep.End, len(ep.Attempts), ep.Why)
	}
	first, second := ep.Attempts[0], ep.Attempts[1]
	if second.Status != Exhausted || second.Anchor.Commit != "" {
		t.Fatalf("the rework is %s anchored at %q, want exhausted and unanchored", second.Status, second.Anchor.Commit)
	}
	entries, _, err := thread.ReadReceipt(e.w.store, ep.Thread)
	if err != nil {
		t.Fatal(err)
	}
	if latest, _ := thread.LatestAnchor(entries); latest.Commit != first.Anchor.Commit {
		t.Fatalf("the thread's anchor is %s, want the first attempt's %s", latest.Commit, first.Anchor.Commit)
	}
	if !strings.Contains(strings.Join(ep.Why, "; "), "kept repair --rework "+string(ep.Thread)+" resumes it") {
		t.Fatalf("the wait does not name the command that resumes it: %v", ep.Why)
	}
	assertWaiting(t, e, ep)
	acts := countActs(t, e)

	req := e.w.request("")
	req.Provider, req.By, req.Operator, req.Assessor, req.Deadlines = e.req.Provider, e.req.By, e.req.Operator, e.req.Assessor, e.req.Deadlines
	again, err := Run(context.Background(), req, bounds(3, true), ep.Thread)
	if err != nil {
		t.Fatal(err)
	}
	if again.End != Waiting || len(again.Attempts) != 0 || again.Resumed != nil {
		t.Fatalf("a resumption before the reset ended %s, opened %d attempts and resumed %v", again.End, len(again.Attempts), again.Resumed)
	}
	if !strings.Contains(strings.Join(again.Why, "; "), "which has not come yet") {
		t.Fatalf("the resumption does not say the reset has not come: %v", again.Why)
	}
	if again.Recorded {
		t.Fatalf("the same wait on the same grounds was recorded twice: %#v", again.Handoff)
	}
	if after := countActs(t, e); after != acts {
		t.Fatalf("a resumption before the reset recorded %d more acts", after-acts)
	}
}

// TestAPassTheAccountCutOffIsRuledOnWhenTheThreadResumes is the other half of a
// wait: the pass over an anchored attempt is cut off, and resuming the thread
// argues that attempt before it opens anything — the repair it would argue
// about is already paid for. Here the resumed pass accepts, so the episode is
// ready without another repair session.
func TestAPassTheAccountCutOffIsRuledOnWhenTheThreadResumes(t *testing.T) {
	e := newEpisode(t, "-X", "main.rulings=synthetic")
	first, err := Run(context.Background(), e.req, bounds(3, true), "")
	if err != nil {
		t.Fatal(err)
	}
	if first.End != Waiting || len(first.Attempts) != 1 || first.Attempts[0].Anchor.Commit == "" {
		t.Fatalf("the first run ended %s after %d attempts: %v", first.End, len(first.Attempts), first.Why)
	}
	anchored := first.Attempts[0].Anchor
	acts := countActs(t, e)

	e.rebuild(t, "-X", "main.rulings=accept")
	req := e.w.request("")
	req.Provider, req.By, req.Operator, req.Assessor, req.Deadlines = e.req.Provider, e.req.By, e.req.Operator, e.req.Assessor, e.req.Deadlines
	ep, err := Run(context.Background(), req, bounds(3, true), first.Thread)
	if err != nil {
		t.Fatal(err)
	}
	if ep.End != Ready || len(ep.Attempts) != 0 {
		t.Fatalf("the resumed episode ended %s after opening %d attempts: %v", ep.End, len(ep.Attempts), ep.Why)
	}
	resumed := ep.Resumed
	if resumed == nil || resumed.Attempt != 1 || resumed.Anchor.Commit != anchored.Commit || resumed.Status != Repaired {
		t.Fatalf("the run resumed %#v, want attempt 1 at its anchor %s", resumed, anchored.Commit)
	}
	if resumed.Argument == nil || resumed.Argument.Ruling != thread.RulingAccept {
		t.Fatalf("the resumed pass ruled %#v", resumed.Argument)
	}
	if ep.Last() != resumed {
		t.Fatal("a run that opened nothing does not report the attempt it resumed as its last")
	}
	if after := countActs(t, e); after != acts {
		t.Fatalf("arguing an attempt that already ran recorded %d more acts", after-acts)
	}
}

// TestAReworkThatBreaksAPromiseItWasCutWhereItHeldIsNotAnchored is the
// attempt-three anchor of thread 7v2p52085pvwtdepzzhz34f3ze: a rework cut at an
// anchor that re-derived kept, leaving a tree that does not, and kept moving the
// thread's anchor — and the land queue — to it. Here the second and third
// attempts each regress. Neither is anchored, the thread's anchor stays where
// the promise held, neither is argued over, and each is cut at that anchor.
func TestAReworkThatBreaksAPromiseItWasCutWhereItHeldIsNotAnchored(t *testing.T) {
	e := newEpisodeOnRefs(t, "-X", "main.rulings=reject", "-X", "main.rework=regress")
	ep, err := Run(context.Background(), e.req, bounds(3, true), "")
	if err != nil {
		t.Fatal(err)
	}
	if ep.End != Escalated || len(ep.Attempts) != 3 {
		t.Fatalf("the run ended %s after %d attempts, want an escalation at the budget: %v", ep.End, len(ep.Attempts), ep.Why)
	}
	held := ep.Attempts[0]
	if held.Status != Repaired || held.Anchor.Commit == "" {
		t.Fatalf("attempt one is %s anchored at %q", held.Status, held.Anchor.Commit)
	}
	for _, res := range ep.Attempts[1:] {
		if res.Commit != held.Anchor.Commit {
			t.Fatalf("attempt %d was cut at %s, want the anchor that holds, %s", res.Attempt, res.Commit, held.Anchor.Commit)
		}
		if res.Before.Value != model.Kept || res.After.Value != model.NotKept || !res.Regressed {
			t.Fatalf("attempt %d read %s → %s, regressed %v", res.Attempt, res.Before.Value, res.After.Value, res.Regressed)
		}
		if res.Anchor.Commit != "" {
			t.Fatalf("attempt %d, which broke the promise, was anchored at %s", res.Attempt, res.Anchor.Commit)
		}
		if res.Box == "" {
			t.Fatalf("attempt %d's unanchored work was discarded with its box", res.Attempt)
		}
		if res.Argument == nil || res.Argument.Status != Unanchored {
			t.Fatalf("attempt %d was argued over: %#v", res.Attempt, res.Argument)
		}
	}
	ref, err := ledger.ThreadRef(string(ep.Thread))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(git(t, e.w.root, "rev-parse", ref)); got != held.Anchor.Commit {
		t.Fatalf("%s points at %s, want the anchor that holds, %s", ref, got, held.Anchor.Commit)
	}
	entries, _, err := thread.ReadReceipt(e.w.store, ep.Thread)
	if err != nil {
		t.Fatal(err)
	}
	briefs := 0
	for _, entry := range entries {
		if entry.Brief != nil {
			briefs++
		}
	}
	if briefs != 1 {
		t.Fatalf("the thread holds %d briefs, want one pass over the one anchored attempt", briefs)
	}
	if latest, _ := thread.LatestAnchor(entries); latest.Commit != held.Anchor.Commit {
		t.Fatalf("the receipt's anchor is %s, want %s", latest.Commit, held.Anchor.Commit)
	}
	// The episode's act is the anchor that holds, judged by its own
	// re-derivation rather than by a regressed tree's.
	own := thread.Attempts(entries)[0].After()
	judged := ep.Readings[len(ep.Readings)-1].Assessment.Entry.After["c"]
	if len(own) != 1 || judged.Address() != own[0].Address {
		t.Fatalf("the episode's act is judged by measurement %s, want the anchoring attempt's own %v", judged.Address(), own)
	}
	if !strings.Contains(strings.Join(ep.Why, "; "), "was not anchored and the next attempt starts from that anchor") {
		t.Fatalf("the escalation does not say the work regressed: %v", ep.Why)
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

func assertWaiting(t *testing.T, e *episode, ep *Episode) {
	t.Helper()
	if !ep.Recorded || ep.Handoff.Outcome != ledger.OutcomeWait || ep.Handoff.Thread != string(ep.Thread) {
		t.Fatalf("the wait was recorded %v as %#v", ep.Recorded, ep.Handoff)
	}
	records := dispositionsNaming(t, e.w.store, "promise:counted")
	last := records[len(records)-1]
	if last.Outcome != ledger.OutcomeWait || last.Thread != string(ep.Thread) {
		t.Fatalf("the disposition in force is %#v, want the wait naming thread %s", last, ep.Thread)
	}
	if got := countActs(t, e); got != len(ep.Attempts) {
		t.Fatalf("%d acts for %d attempts: a session the account cut off still ran and still writes one", got, len(ep.Attempts))
	}
	for _, record := range records {
		if record.Outcome == ledger.OutcomeEscalate {
			t.Fatalf("running out of capacity escalated: %#v", record)
		}
	}
}

// gitErr runs git and returns its error rather than failing the test, for an
// assertion that something is not there.
func gitErr(dir string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	return string(out), err
}

func countActs(t *testing.T, e *episode) int {
	t.Helper()
	acts := 0
	for _, record := range dispositionsNaming(t, e.w.store, "promise:counted") {
		if record.Outcome == ledger.OutcomeAct {
			acts++
		}
	}
	return acts
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

// TestAPassIsNotSpentOnAnAttemptTheChecksAlreadyEscalateAndAReworkArguesItFirst
// is docs/decision/0095. The promise here bounds its repair to the promise files
// and the Markdown at the top of docs, so the stand-in's rework note lands inside
// the box's grant and outside the bound: paths-in-scope fires on every rework.
// The first time it reworks, and the pass over that attempt runs. The second time
// it escalates whatever a ruling says, so no pass is spent on it — and a
// person's --rework argues that attempt before it opens the next one, so the
// next packet still carries the case against it.
func TestAPassIsNotSpentOnAnAttemptTheChecksAlreadyEscalateAndAReworkArguesItFirst(t *testing.T) {
	e := newEpisode(t, "-X", "main.rulings=reject")
	bounded := strings.Replace(promiseFile("counted", "docs/**", "obs.c.artifacts == 7"),
		`scope: ["docs/**"]
---`, `scope: ["docs/promise/**", "docs/*.md"]
---`, 1)
	if !strings.Contains(bounded, `"docs/*.md"`) {
		t.Fatal("the fixture's repair bound was not narrowed, so nothing here would fire")
	}
	e.w.write("docs/promise/counted.md", bounded)
	e.w.commit()
	e.w.ratify("promise:counted")
	// The request reads the ratification it now has; everything else is the
	// episode's.
	old := e.req
	e.req = e.w.request("promise:counted")
	e.req.Provider, e.req.By, e.req.Operator, e.req.Assessor, e.req.Deadlines = old.Provider, old.By, old.Operator, old.Assessor, old.Deadlines

	ep, err := Run(context.Background(), e.req, bounds(3, true), "")
	if err != nil {
		t.Fatal(err)
	}
	if ep.End != Escalated || len(ep.Attempts) != 3 {
		t.Fatalf("the episode ended %s after %d attempts, want an escalation after 3: %v", ep.End, len(ep.Attempts), ep.Why)
	}
	if pass := ep.Attempts[1].Argument; pass == nil || pass.Status != Argued {
		t.Fatalf("the pass over attempt 2, whose check fired for the first time, is %+v; it decides whether to rework and must run", pass)
	}
	deferred := ep.Attempts[2].Argument
	if deferred == nil || deferred.Status != Deferred {
		t.Fatalf("the pass over attempt 3, which the repeated check already escalates, is %+v, want deferred", deferred)
	}
	why := strings.Join(ep.Why, "; ")
	if !strings.Contains(why, "paths-in-scope fired again") || strings.Contains(why, "adversarial pass") {
		t.Fatalf("the escalation says %q; it is the repeated check's, and a deferred pass adds no reason of its own", why)
	}
	briefs := func() int {
		entries, _, err := thread.ReadReceipt(e.w.store, ep.Thread)
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for _, session := range thread.Sessions(entries) {
			if session.Kind == ledger.ThreadBrief {
				n++
			}
		}
		return n
	}
	if n := briefs(); n != 2 {
		t.Fatalf("the thread holds %d passes after three attempts, want 2: the third is deferred", n)
	}

	req := e.w.request("")
	req.Provider, req.By, req.Operator, req.Assessor, req.Deadlines = e.req.Provider, e.req.By, e.req.Operator, e.req.Assessor, e.req.Deadlines
	again, err := Run(context.Background(), req, bounds(1, true), ep.Thread)
	if err != nil {
		t.Fatal(err)
	}
	if again.Resumed == nil || again.Resumed.Argument == nil || again.Resumed.Argument.Status != Argued || again.Resumed.Attempt != 3 {
		t.Fatalf("the rework did not argue attempt 3 first: %+v", again.Resumed)
	}
	if len(again.Attempts) != 1 || again.Attempts[0].Attempt != 4 {
		t.Fatalf("after arguing the deferred attempt, the rework opened %d attempts, want attempt 4", len(again.Attempts))
	}
	// Attempt 4 repeats the check too, so its own pass is deferred in turn: the
	// thread holds attempt 1's, attempt 2's and the resumed one over attempt 3.
	if n := briefs(); n != 3 {
		t.Fatalf("the thread holds %d passes after the rework, want 3", n)
	}
	if pass := again.Attempts[0].Argument; pass == nil || pass.Status != Deferred {
		t.Fatalf("attempt 4 repeats the check and its pass is %+v, want deferred", pass)
	}
	carried := packets(t, e.w.store, ep.Thread)
	if len(carried) != 4 || carried[3].Rework == nil || len(carried[3].Rework.Earlier) != 3 {
		t.Fatalf("attempt 4's packet carries %d packets' worth of earlier attempts", len(carried))
	}
	if said := carried[3].Rework.Earlier[2].Argument; said == "" || said != again.Resumed.Argument.Said {
		t.Fatalf("attempt 4's packet carries attempt 3's argument as %q, want what the resumed pass said", said)
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
