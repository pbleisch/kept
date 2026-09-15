//go:build !windows

package repair

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/config"
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/run"
	"github.com/pbleisch/kept/internal/thread"
)

// This file is a whole repair, end to end, with a stand-in where Claude Code
// would be: the box is cut, the gap measured, the provider verified by its real
// canary, the stand-in dispatched confined under the real provider, and the
// promise re-derived from what it left. It needs no credential and spends
// nothing, because the stand-in answers to no upstream.
//
// What it stands in for is the end of threads 1y6kx8c48hb6mtjkpda76brtz0 and
// 7gz5c1wmwn5s4pv2rsqk7whs32: a runtime that did its work, wrote part of a line
// and exited 0.

// stubRuntimeSource is the stand-in. It reports a version this adapter has
// reviewed, reads the packet, repairs the fixture's broken promise by removing
// the artifact that broke it, and then ends its output partway through an event.
//
// Its mode is set when it is built. "breaks-corpus" also writes a second
// artifact claiming the promise's own id, which no corpus loads, so the promise
// cannot be measured in the box afterwards. "slow" waits before it finishes,
// for a test to replace it underneath its session.
const stubRuntimeSource = `package main

import (
	"bufio"
	"fmt"
	"os"
	"time"
)

var (
	mode    = "torn"
	version = "2.1.258"
)

func main() {
	for _, arg := range os.Args[1:] {
		if arg == "--version" {
			fmt.Println(version + " (Claude Code)")
			return
		}
	}
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	fmt.Println("{\"type\":\"system\",\"subtype\":\"init\",\"model\":\"claude-sonnet-5\"}")
	if err := os.Remove("docs/promise/spare.md"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(4)
	}
	switch mode {
	case "breaks-corpus":
		counted, err := os.ReadFile("docs/promise/counted.md")
		if err == nil {
			err = os.WriteFile("docs/promise/counted-again.md", counted, 0o644)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(4)
		}
	case "slow":
		time.Sleep(4 * time.Second)
	}
	fmt.Fprintln(os.Stderr, "stub: the session is over")
	fmt.Println("{\"type\":\"assistant\",\"message\":{\"content\":[{\"type\":\"text\",\"text\":\"removed the artifact that broke the count\"}]}}")
	os.Stdout.WriteString("{\"type\":\"stream_event\",\"event\":{\"type\":\"content_block_delta\"")
	os.Exit(0)
}
`

// stubRepair is a repair request against a broken promise, with the stand-in as
// its runtime. wrap, when set, stands between the repair and its ledger.
func stubRepair(t *testing.T, wrap func(ledger.Store) ledger.Store) (*repairWorkspace, Request) {
	return stubRepairIn(t, wrap, "torn")
}

func stubRepairIn(t *testing.T, wrap func(ledger.Store) ledger.Store, mode string) (*repairWorkspace, Request) {
	t.Helper()
	provider := liveProvider(t)
	kept := buildKept(t)
	previous := keptExecutable
	keptExecutable = func() (string, error) { return kept, nil }
	t.Cleanup(func() { keptExecutable = previous })

	w := newRepairWorkspaceUnder(t, ungrantedDir(t))
	w.write("docs/promise/spare.md", promiseFile("spare", "docs/**", "obs.c.ok"))
	w.commit()

	stub := buildStubRuntime(t, "-X", "main.mode="+mode)
	// A reference to a credential the stand-in never uses, outside every grant,
	// because a brokered route is what a Claude box is launched with.
	secret := filepath.Join(ungrantedDir(t), "operator-credential")
	if err := os.WriteFile(secret, []byte("not-a-credential"), 0o600); err != nil {
		t.Fatal(err)
	}

	req := w.request("promise:counted")
	if wrap != nil {
		l := ledger.Open(wrap(w.store), run.Start("test-repair", nil))
		t.Cleanup(func() { _ = l.Close(run.Clean) })
		req.Ledger = l
	}
	req.Provider = filepath.Base(provider)
	req.By = "Test <test@example.com>"
	req.Operator = config.Repair{
		Runtime:           string(thread.Claude),
		Executable:        stub,
		Credential:        "file://" + secret,
		CredentialExpires: time.Now().Add(365 * 24 * time.Hour),
	}
	req.Deadlines.Agent = 2 * time.Minute
	return w, req
}

func buildStubRuntime(t *testing.T, ldflags ...string) string {
	t.Helper()
	return buildStubRuntimeAt(t, filepath.Join(ungrantedDir(t), "claude"), ldflags...)
}

func buildStubRuntimeAt(t *testing.T, binary string, ldflags ...string) string {
	t.Helper()
	source := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(source, []byte(stubRuntimeSource), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-ldflags", strings.Join(ldflags, " "), "-o", binary, source)
	cmd.Dir = filepath.Dir(source)
	cmd.Env = append(os.Environ(), "GO111MODULE=off")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build the stand-in runtime: %v\n%s", err, output)
	}
	return binary
}

// TestARepairWhoseOutputEndsInsideALineIsStillReDerivedAndAnchored is the
// behaviour docs/decision/0084 chose. The torn line is kept in the receipt and
// said out loud, and it changes nothing else: the promise is measured from the
// tree the session left, the work is anchored, and the act is recorded — which
// is exactly what a clean exit gets.
func TestARepairWhoseOutputEndsInsideALineIsStillReDerivedAndAnchored(t *testing.T) {
	w, req := stubRepair(t, nil)
	res, err := Perform(context.Background(), req)
	if err != nil {
		t.Fatalf("a session whose output ended inside a line failed the repair: %v", err)
	}
	if res.Status != Repaired {
		t.Fatalf("status = %s (%s → %s: %s), want the re-derivation to find the repair", res.Status, res.Before.Value, res.After.Value, res.After.Detail)
	}
	if res.Anchor.Commit == "" {
		t.Fatalf("the session's work was not anchored: %s", res.Warning)
	}
	if !strings.Contains(res.Warning, "partway through a line") {
		t.Fatalf("the repair did not say its session's output was cut off: %q", res.Warning)
	}

	entries, quarantined, err := thread.ReadReceipt(w.store, res.Thread)
	if err != nil {
		t.Fatal(err)
	}
	if len(quarantined) != 0 {
		t.Fatalf("quarantined: %#v", quarantined)
	}
	var torn *thread.Undecodable
	for _, entry := range entries {
		if entry.Undecodable != nil {
			torn = entry.Undecodable
		}
	}
	if torn == nil || !torn.Unterminated {
		t.Fatalf("the receipt does not keep the torn line: %s", thread.ReceiptText(entries))
	}
	assertTheActWasRecorded(t, w, res.Thread)
}

// TestARepairWhoseOutputCannotBeRetainedKeepsTheBoxItRanIn is the other half:
// a failure that is kept's own — here, a receipt that will not take the
// session's events — after a session has run. Nothing is re-derived from a
// thread whose record is broken, so nothing anchors the work, and the box is the
// only copy of it. It stays, the error says where, and the act is still
// recorded.
func TestARepairWhoseOutputCannotBeRetainedKeepsTheBoxItRanIn(t *testing.T) {
	w, req := stubRepair(t, func(store ledger.Store) ledger.Store {
		return refusingNative{Store: store}
	})
	res, err := Perform(context.Background(), req)
	if err == nil {
		t.Fatal("a repair whose receipt refused every event reported success")
	}
	if !strings.Contains(err.Error(), "retain runtime output") {
		t.Fatalf("error = %v, want the retention failure", err)
	}
	var failure *Failure
	if !errors.As(err, &failure) || failure.Phase != PhaseSession {
		t.Fatalf("error %T %v, want a failure in the session phase", err, err)
	}
	if res != nil && (failure.Thread != res.Thread || failure.Box != res.Box || failure.Promise != "promise:counted") {
		t.Fatalf("failure = %#v, want the thread, box and promise the result names", failure)
	}
	if res == nil || res.Thread == "" {
		t.Fatalf("the failure did not name the thread that ran: %#v", res)
	}
	if res.Box == "" || !strings.Contains(err.Error(), res.Box) {
		t.Fatalf("the error does not say where the box is: %v (box %q)", err, res.Box)
	}
	t.Cleanup(func() { git(t, w.root, "worktree", "remove", "--force", res.Box) })
	if info, statErr := os.Stat(res.Box); statErr != nil || !info.IsDir() {
		t.Fatalf("the box the session ran in was discarded: %v", statErr)
	}
	// And it holds the session's work, which is the reason it was kept.
	if _, statErr := os.Stat(filepath.Join(res.Box, "docs", "promise", "spare.md")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("the kept box does not hold what the session did: %v", statErr)
	}
	if listed := git(t, w.root, "worktree", "list", "--porcelain"); !strings.Contains(listed, res.Box) {
		t.Fatalf("the kept box is not a registered worktree, so doctor would not report it:\n%s", listed)
	}
	assertTheActWasRecorded(t, w, res.Thread)
}

// TestARepairThatCannotBeAnchoredKeepsTheBoxItRanIn is the same rule on a path
// that is not an error. An anchor that cannot be written is a warning, because
// the verdict is reached and the session is recorded — and the work it would
// have committed is then in the box and nowhere else, so the box stays.
func TestARepairThatCannotBeAnchoredKeepsTheBoxItRanIn(t *testing.T) {
	w, req := stubRepair(t, nil)
	// Nobody to author the anchor: no operator named, and git answers with no
	// identity anywhere. The canary names its own, so it is untouched.
	req.By = ""
	w.git("config", "--unset", "user.name")
	w.git("config", "--unset", "user.email")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)

	res, err := Perform(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != Repaired || res.Anchor.Commit != "" {
		t.Fatalf("status %s, anchor %q: want a repair that held and could not be anchored", res.Status, res.Anchor.Commit)
	}
	if !strings.Contains(res.Warning, "could not be anchored") {
		t.Fatalf("warning = %q, want the anchor failure", res.Warning)
	}
	if res.Box == "" {
		t.Fatal("the box holding the only copy of the session's work was discarded")
	}
	t.Cleanup(func() { git(t, w.root, "worktree", "remove", "--force", res.Box) })
	if _, statErr := os.Stat(filepath.Join(res.Box, "docs", "promise", "spare.md")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("the kept box does not hold what the session did: %v", statErr)
	}
}

// refusingNative is a ledger that will not take a runtime's events and takes
// everything else — the parent's lifecycle, the measurements, the disposition.
type refusingNative struct{ ledger.Store }

func (s refusingNative) Append(stream string, line []byte) error {
	if strings.Contains(stream, "threads/") && bytes.Contains(line, []byte(`"kind":"thread_native"`)) {
		return errors.New("this ledger refuses native events")
	}
	return s.Store.Append(stream, line)
}

func assertTheActWasRecorded(t *testing.T, w *repairWorkspace, id thread.ID) {
	t.Helper()
	records, _, err := ledger.ReadDispositions(w.store, "promise:counted")
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range records {
		if record.Outcome == ledger.OutcomeAct && record.Thread == string(id) {
			return
		}
	}
	t.Fatalf("no act names thread %s, so a session that ran is missing from the record: %#v", id, records)
}

// TestARepairThatCannotMeasureItsBoxAfterwardsSaysWhichPhaseFailed is the phase
// a person most needs named: the session ran and did its work, and then the
// promise could not be measured in the tree it left. Nothing anchored that work,
// so the box stays.
func TestARepairThatCannotMeasureItsBoxAfterwardsSaysWhichPhaseFailed(t *testing.T) {
	w, req := stubRepairIn(t, nil, "breaks-corpus")
	res, err := Perform(context.Background(), req)
	var failure *Failure
	if !errors.As(err, &failure) || failure.Phase != PhaseRederive {
		t.Fatalf("error %T %v, want a failure in the re-derive phase", err, err)
	}
	if res == nil || failure.Box == "" || failure.Box != res.Box {
		t.Fatalf("the box the session ran in was not kept and named: %#v", failure)
	}
	t.Cleanup(func() { git(t, w.root, "worktree", "remove", "--force", failure.Box) })
	assertTheActWasRecorded(t, w, failure.Thread)
}

// TestARepairWhoseRuntimeWasReplacedUnderneathItSaysSo is the replacement both
// failed threads were first blamed on, made to happen: a different binary put
// at the runtime's path, npm's way, while the session runs. It does not end the
// session, and the repair says it happened and what the new file calls itself.
func TestARepairWhoseRuntimeWasReplacedUnderneathItSaysSo(t *testing.T) {
	w, req := stubRepairIn(t, nil, "slow")
	newer := buildStubRuntimeAt(t, filepath.Join(ungrantedDir(t), "claude"), "-X", "main.mode=slow", "-X", "main.version=2.1.251")

	replaced := make(chan error, 1)
	go func() {
		// Once the session has done its work in the box, and while it waits.
		deadline := time.Now().Add(time.Minute)
		for time.Now().Before(deadline) {
			boxes, _ := filepath.Glob(filepath.Join(w.root, ".kept", "boxes", "*", "docs", "promise"))
			for _, promises := range boxes {
				if _, err := os.Stat(filepath.Join(promises, "spare.md")); errors.Is(err, os.ErrNotExist) {
					retired := req.Operator.Executable + ".retired"
					if err := os.Rename(req.Operator.Executable, retired); err != nil {
						replaced <- err
						return
					}
					src, err := os.ReadFile(newer)
					if err == nil {
						err = os.WriteFile(req.Operator.Executable, src, 0o755)
					}
					_ = os.Remove(retired)
					replaced <- err
					return
				}
			}
			time.Sleep(20 * time.Millisecond)
		}
		replaced <- errors.New("the session never did its work, so nothing was replaced underneath it")
	}()

	res, err := Perform(context.Background(), req)
	if swapErr := <-replaced; swapErr != nil {
		t.Fatal(swapErr)
	}
	if err != nil {
		t.Fatalf("a session whose runtime was replaced failed the repair: %v", err)
	}
	if res.Status != Repaired {
		t.Fatalf("status = %s, want the replacement to change nothing about the verdict", res.Status)
	}
	if res.RuntimeAfter != "2.1.251" {
		t.Fatalf("the repair says the runtime reports %q after its session, want the replacement's own version", res.RuntimeAfter)
	}
	if !strings.Contains(res.Warning, "was replaced during thread") || !strings.Contains(res.Warning, "2.1.258") {
		t.Fatalf("warning = %q, want the replacement and the version it was admitted as", res.Warning)
	}
}
