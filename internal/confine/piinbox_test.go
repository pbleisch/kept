//go:build !windows

package confine

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/box"
	"github.com/pbleisch/kept/internal/canonical"
	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/gather"
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/run"
	"github.com/pbleisch/kept/internal/runtimestate"
	"github.com/pbleisch/kept/internal/thread"
	"github.com/pbleisch/kept/internal/tick"
	"github.com/pbleisch/kept/internal/workspace"
)

// This file is decision 0052's admission evidence for the Pi adapter, and it is
// the first repair kept performs.
//
// A confined Pi repair either works or it does not, and the only way to know is
// to run one: a real Pi, in a real box cut from a real repository, under the
// profile a repair runs under — no provider policy group, no network but one
// named loopback port, the interpreter and the runtime's package located by
// asking rather than by a list, a temporary directory and a home that belong to
// the box. The verdict is re-derived by kept from the box's world afterwards,
// and the agent's own account of what it did is never read.

// The self-hosted authority an unattended repair uses here. It is explicit
// configuration in the sense decision 0052 requires: nothing discovers a
// provider, a model or an endpoint, and a machine that does not serve this one
// skips rather than substituting another.
const (
	piLiveProvider = "workshop"
	piLiveModel    = "qwen3.6-27b-mlx@6bit"
	piLiveEndpoint = "http://localhost:1234/v1"
	piLiveHost     = "localhost"
	piLivePort     = 1234
)

// piLiveDeadline bounds a completed repair session, and piLiveInterrupt is the
// deliberately short one the interruption proof uses. Neither is a default: an
// external runtime with no deadline is what decision 0026 refuses.
const (
	piLiveDeadline  = 10 * time.Minute
	piLiveInterrupt = 25 * time.Second
)

// The promise this repair is about.
//
// It is chosen for three reasons and against one. Its instrument is `go/imports`,
// which parses and never shells out, so re-deriving the verdict inside a box
// needs no toolchain and reports a graph even for the tree a repair stands in —
// which here does not compile, because the break is an import cycle and that is
// what the promise is about. Its observation carries the package graph, so the
// gap the agent has to close is *in the evidence* rather than in a message kept
// wrote. And its repair scope is a real bound rather than the whole tree.
//
// Against: kept's own internal/confine tests cannot pass inside a kept box, so
// a promise gathered with `go/tests` over internal/** is not the one to prove
// this with. That is an open question of the operator's and not this test's.
const (
	piRepairPromise = "promise:the-corpus-cannot-write-standing-or-truth"
	piRepairFile    = "internal/corpus/corpus.go"
	piRepairEdge    = "github.com/pbleisch/kept/internal/ledger"
)

// TestConfinedPiRepairsABrokenPromiseAndTheVerdictIsReDerived is the whole loop:
// cut a box, break the promise in it, gather and find it not kept, dispatch Pi
// under confinement, gather again, and take the verdict from the second
// measurement.
//
// The re-gather is the point. `policy:the-agents-report-is-never-an-input-to-the-verdict`
// is not a convention this test observes — the receipt is read for evidence
// about the session and is never consulted about whether the promise is kept.
func TestConfinedPiRepairsABrokenPromiseAndTheVerdictIsReDerived(t *testing.T) {
	session := livePiSession(t)

	before := session.verdict(t)
	if before.Value != model.NotKept {
		t.Fatalf("the promise reads %q in the box before the repair, so there is nothing to repair: %s", before.Value, before.Detail)
	}

	receipt, store := session.dispatch(t, piLiveDeadline)
	entries, quarantined, err := thread.ReadReceipt(store, receipt.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(quarantined) != 0 {
		t.Fatalf("the live Pi receipt quarantined its own evidence: %#v", quarantined)
	}

	// Decision 0052's four receipt requirements, from a real session: native
	// events retained with their own type, and transcript, cost and turn
	// projections that each name the native event they came from.
	kinds := receiptKinds(entries)
	for _, want := range []string{"thread_native", "thread_transcript", "thread_cost", "thread_turn", "thread_lifecycle"} {
		if kinds[want] == 0 {
			t.Fatalf("the live Pi receipt has no %s entry: %v", want, kinds)
		}
	}
	if kinds["thread_turn"] < 2 {
		t.Fatalf("the live Pi session produced %d turn updates, so it did not stay active long enough to be incremental evidence", kinds["thread_turn"])
	}
	assertProjectionsReferenceNativeEvents(t, entries)
	assertNativeTypes(t, entries, "response", "agent_start", "turn_start", "turn_end", "agent_settled")

	// The packet reached the runtime. Its marker is the promise id, which the
	// adapter never writes anywhere else.
	if !sawPacket(t, store, entries) {
		t.Fatal("no retained Pi transcript or native event carries the packet marker, so the session did not receive the packet")
	}

	// Pi ends at input EOF and the adapter closes the channel on its own
	// settled event, so a completed session is a normal exit rather than a
	// deadline kill.
	last := entries[len(entries)-1]
	if last.Lifecycle == nil || last.Lifecycle.Event != thread.Exited || last.Lifecycle.Exit == nil || last.Lifecycle.Exit.Code == nil || *last.Lifecycle.Exit.Code != 0 {
		t.Fatalf("the live Pi session did not exit normally: %#v", last.Lifecycle)
	}
	for _, entry := range entries {
		if entry.Lifecycle != nil && (entry.Lifecycle.Event == thread.TimedOut || entry.Lifecycle.Event == thread.Killed) {
			t.Fatalf("the completed Pi session was interrupted: %#v", entry.Lifecycle)
		}
	}

	// The repair itself, re-derived. Nothing above is an input to this.
	after := session.verdict(t)
	if after.Value != model.Kept {
		t.Fatalf("the re-derived verdict after the repair is %q: %s\n%s", after.Value, after.Detail, session.change(t).Diff)
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
	t.Logf("confined Pi repair: %v native events, %v turns, %v transcript fragments, %v cost updates", kinds["thread_native"], kinds["thread_turn"], kinds["thread_transcript"], kinds["thread_cost"])
	t.Logf("re-derived verdict: %s -> %s", before.Value, after.Value)
	t.Logf("repair diff:\n%s", changed.Diff)
	t.Logf("what a person reads:\n%s", review)
	if !review.Bounded() {
		t.Fatalf("the repair reached outside the scope its promise bound:\n%s\n%s", review, changed.Diff)
	}
	if got := changed.Paths; len(got) != 1 || got[0] != piRepairFile {
		t.Fatalf("the repair changed %v, want exactly %s:\n%s", got, piRepairFile, changed.Diff)
	}
}

// TestConfinedPiKilledMidSessionRetainsItsPartialReceipt is the interruption
// proof decision 0052 requires, and it is the half that matters: a
// completion-only report loses exactly this.
//
// The deadline is short enough that Pi is still streaming when it elapses, so
// what is asserted is that the events already received survive alongside the
// timeout, the kill and the exit the parent actually observed — and that no
// completion was inferred after the fact.
func TestConfinedPiKilledMidSessionRetainsItsPartialReceipt(t *testing.T) {
	session := livePiSession(t)

	receipt, store := session.dispatch(t, piLiveInterrupt)
	entries, quarantined, err := thread.ReadReceipt(store, receipt.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(quarantined) != 0 {
		t.Fatalf("the interrupted Pi receipt quarantined its own evidence: %#v", quarantined)
	}

	kinds := receiptKinds(entries)
	if kinds["thread_native"] < 2 {
		t.Fatalf("the interrupted Pi session retained %d native events, so the kill measured nothing", kinds["thread_native"])
	}
	if kinds["thread_transcript"] == 0 {
		t.Fatalf("the interrupted Pi session retained no transcript fragment: %v", kinds)
	}
	t.Logf("interrupted Pi receipt: %v", kinds)
	assertProjectionsReferenceNativeEvents(t, entries)

	// The lifecycle the parent observed, in the order it happened. Each item is
	// written only after it happened, so this sequence is a record and never an
	// inference: timed out, then killed once the group kill succeeded, then the
	// exit that was actually reaped.
	var lifecycle []thread.LifecycleEvent
	for _, entry := range entries {
		if entry.Lifecycle != nil {
			lifecycle = append(lifecycle, entry.Lifecycle.Event)
		}
	}
	want := []thread.LifecycleEvent{thread.Started, thread.TimedOut, thread.Killed, thread.Exited}
	if len(lifecycle) != len(want) {
		t.Fatalf("interrupted Pi lifecycle = %v, want %v", lifecycle, want)
	}
	for i, event := range want {
		if lifecycle[i] != event {
			t.Fatalf("interrupted Pi lifecycle = %v, want %v", lifecycle, want)
		}
	}

	// The cause, and the reaped exit. A timeout that did not record its own
	// deadline is a kill nobody can account for.
	var timedOut, exited *thread.Lifecycle
	for _, entry := range entries {
		switch {
		case entry.Lifecycle == nil:
		case entry.Lifecycle.Event == thread.TimedOut:
			timedOut = entry.Lifecycle
		case entry.Lifecycle.Event == thread.Exited:
			exited = entry.Lifecycle
		}
	}
	if timedOut == nil || timedOut.Timeout != piLiveInterrupt.String() {
		t.Fatalf("the interrupted Pi session recorded no timeout cause: %#v", timedOut)
	}
	if exited == nil || exited.Exit == nil || exited.Exit.Signal == "" {
		t.Fatalf("the killed Pi session did not record a reaped terminating signal: %#v", exited)
	}
	// A native event after the exit would be a completion written after the
	// fact rather than a receipt of what arrived.
	if entries[len(entries)-1].Lifecycle == nil || entries[len(entries)-1].Lifecycle.Event != thread.Exited {
		t.Fatalf("the interrupted Pi receipt does not end at the reaped exit: %s", receiptKindText(entries))
	}
}

// TestConfinedPiReachesOnlyTheDeclaredLoopbackPort measures the network from
// inside the box rather than from the profile.
//
// The provider's own explain tool answers `network_blocked` for the port this
// profile reaches, so there is nothing to ask it: the boundary has to be
// measured through a socket in the box. The premise is checked unconfined
// first, because a refusal measured against a port nothing serves proves
// nothing.
func TestConfinedPiReachesOnlyTheDeclaredLoopbackPort(t *testing.T) {
	session := livePiSession(t)

	// The other loopback port is the parent's own listener, so it is really
	// there and really refused rather than merely absent.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			_ = connection.Close()
		}
	}()

	declared := fmt.Sprintf("%s:%d", piLiveHost, piLivePort)
	undeclared := listener.Addr().String()
	for _, address := range []string{declared, undeclared} {
		probe, err := net.DialTimeout("tcp", address, 10*time.Second)
		if err != nil {
			t.Skipf("this machine cannot reach %s unconfined, so a measurement from inside the box would mean nothing: %v", address, err)
		}
		_ = probe.Close()
	}

	stream := &testStream{}
	child, err := session.launch.Start(context.Background(), thread.Invocation{
		Path: "/bin/sh",
		Args: []string{"-c", `
/usr/bin/nc -z -G 5 -w 5 "$1" "$2" >/dev/null 2>&1 && echo declared=reached || echo declared=refused
/usr/bin/nc -z -G 5 -w 5 "$3" "$4" >/dev/null 2>&1 && echo undeclared=reached || echo undeclared=refused
/usr/bin/nc -z -G 5 -w 5 example.com 443 >/dev/null 2>&1 && echo host=reached || echo host=refused
`, "sh", piLiveHost, fmt.Sprint(piLivePort), "127.0.0.1", portOf(t, undeclared)},
		Env:    session.childEnv(),
		Output: stream,
	})
	if err != nil {
		t.Fatal(err)
	}
	if exit := child.Wait(); exit.Code == nil || *exit.Code != 0 {
		t.Fatalf("confined loopback probe exit = %#v: %s", exit, stream.String())
	}
	report := stream.String()
	for _, want := range []string{"declared=reached", "undeclared=refused", "host=refused"} {
		if !strings.Contains(report, want) {
			t.Fatalf("confined network report is missing %q:\n%s", want, report)
		}
	}
}

// TestARealFailedCanaryBlocksTheConfinedPiLaunch is the other half of "the
// runtime cannot read the ledger, the keychain or a credential that can push to
// origin": those are the canary's probes, and this is what says a canary that
// did not report is a refusal rather than a launch.
//
// The break is the canary helper itself — the profile names an executable that
// is not kept, so the confined helper produces no report at all. Nothing about
// the repair profile is weakened, which is why the refusal is about
// verification rather than about a boundary this test removed.
func TestARealFailedCanaryBlocksTheConfinedPiLaunch(t *testing.T) {
	session := livePiSession(t)
	broken := cloneProfile(session.launch.profile)
	broken.Kept = "/bin/echo"
	launch, err := NewNono(broken)
	if err != nil {
		t.Fatal(err)
	}
	if err := launch.Verified(); err == nil {
		t.Fatal("a canary that produced no report verified the launch")
	} else if !strings.Contains(err.Error(), "canary") {
		t.Fatalf("verification error = %v, want it to name the canary", err)
	}

	store := ledger.NewFiles(t.TempDir())
	writable := ledger.Open(store, run.Start("pi-live-proof-canary", nil))
	receipt, err := thread.NewReceipt(writable, thread.MintID())
	if err != nil {
		t.Fatal(err)
	}
	err = thread.Dispatch(context.Background(), session.config(), session.packet, launch, thread.Supervisor{Timeout: time.Minute}, receipt)
	if err == nil || !strings.Contains(err.Error(), "canary failed") {
		t.Fatalf("a failed canary did not refuse the Pi dispatch: %v", err)
	}
	if entries, _, err := thread.ReadReceipt(store, receipt.ID()); err == nil {
		t.Fatalf("a refused Pi dispatch wrote a receipt stream: %s", thread.ReceiptText(entries))
	}

	// And the unbroken profile verifies, which is what says the refusal was
	// about the canary rather than about anything else in this box.
	if err := session.launch.Verified(); err != nil {
		t.Fatalf("the repair profile's own canary failed: %v", err)
	}
}

// TestPreflightRefusesANodeInterpreterTheProviderCannotReach is the search-path
// refusal, measured against a real provider.
//
// An interpreter reached through a shim resolves correctly — the shim execs a
// real Node, which knows where it lives — while the shim's own directory is
// granted by nothing. Without the pre-flight that is a dispatch that starts and
// then reports `env: node: No such file or directory` from inside a box, which
// is indistinguishable from a repair that ran and did nothing.
func TestPreflightRefusesANodeInterpreterTheProviderCannotReach(t *testing.T) {
	nono := liveProvider(t)
	real := realNode(t)
	root := ungrantedDir(t)
	shim := filepath.Join(root, "shim")
	if err := os.Mkdir(shim, 0o700); err != nil {
		t.Fatal(err)
	}
	shimmed := filepath.Join(shim, "node")
	if err := os.WriteFile(shimmed, []byte("#!/bin/sh\nexec "+real+" \"$@\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	repository, created := scratchBox(t, root)
	toolchain, err := ResolveNode(context.Background(), shimmed, livePi(t), 30*time.Second)
	if err != nil {
		t.Fatalf("resolve a shimmed node interpreter: %v", err)
	}
	if !slices.Equal(toolchain.Path, []string{shim}) {
		t.Fatalf("resolved search path = %v, want the shim directory %s", toolchain.Path, shim)
	}

	profile := nodeProfile(t, repository, created, nono, toolchain)
	launch, err := NewNono(profile)
	if err != nil {
		t.Fatal(err)
	}
	err = launch.Verified()
	if err == nil {
		t.Fatal("the launch was verified with an interpreter the provider cannot reach")
	}
	for _, want := range []string{"pre-flight refused", shim, "read", "search path"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("refusal = %v, want it to name %q", err, want)
		}
	}

	// And the other branch: grant the shim's directory and the same launch is
	// verified. Without this, the refusal above could be anything.
	granted := cloneProfile(profile)
	granted.ToolRead = append(granted.ToolRead, shim)
	reachable, err := NewNono(granted)
	if err != nil {
		t.Fatal(err)
	}
	if err := reachable.Verified(); err != nil {
		t.Fatalf("provider verification failed once the shim directory was granted: %v", err)
	}
}

// TestAConfinedNodeRuntimeNeedsThePackageItResolves is the grant broken on
// purpose, and it is the measurement decision 0055 was written from.
//
// A runtime installed where the provider grants nothing — `npm install -g`
// under a prefix, which is covered by no capability — starts and then cannot
// read itself. The failure is MODULE_NOT_FOUND from inside a box, with an exit
// code and no refusal anywhere. Declaring the package Node reads is the whole
// difference, and both halves are run here so the second is not an argument.
func TestAConfinedNodeRuntimeNeedsThePackageItResolves(t *testing.T) {
	nono := liveProvider(t)
	node := realNode(t)
	root := ungrantedDir(t)

	// A package tree laid out the way npm lays one out, somewhere the provider
	// grants nothing. The premise is the point: this is not a path kept denied.
	packaged := filepath.Join(root, "lib", "node_modules", "hosted")
	if err := os.MkdirAll(filepath.Join(packaged, "dist"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packaged, "package.json"), []byte(`{"name":"hosted","bin":{"hosted":"dist/cli.js"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	hosted := filepath.Join(packaged, "dist", "cli.js")
	if err := os.WriteFile(hosted, []byte("#!/usr/bin/env node\nconsole.log(\"the hosted runtime ran\");\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	repository, created := scratchBox(t, root)
	toolchain, err := ResolveNode(context.Background(), node, hosted, 30*time.Second)
	if err != nil {
		t.Fatalf("resolve a node runtime under an ungranted prefix: %v", err)
	}
	if toolchain.Read[1] != packaged {
		t.Fatalf("resolved package root = %s, want the ungranted package at %s", toolchain.Read[1], packaged)
	}
	// If the provider reached this on its own the comparison below would be
	// between two grants rather than between a grant and its absence.
	profile := nodeProfile(t, repository, created, nono, toolchain)
	if answer := askPath(t, nono, writeRendered(t, renderedWithout(t, profile, packaged)), packaged); answer.Status == providerAllowed {
		t.Skipf("this host's provider baseline already grants %s, so the declaration cannot be measured here: %#v", packaged, answer)
	}

	launchHosted := func(t *testing.T, profile Profile) (thread.Exit, string) {
		t.Helper()
		launch, err := NewNono(profile)
		if err != nil {
			t.Fatal(err)
		}
		stream := &testStream{}
		child, err := launch.Start(context.Background(), thread.Invocation{
			Path:   hosted,
			Env:    []string{"RUNTIME_TOKEN=only-this-credential"},
			Output: stream,
		})
		if err != nil {
			t.Fatal(err)
		}
		exit := child.Wait()
		report := stream.String()
		if reporter, ok := child.(interface{ diagnostic() string }); ok {
			report += "\n" + reporter.diagnostic()
		}
		return exit, report
	}

	// Declared: the runtime reads itself and runs.
	if exit, report := launchHosted(t, profile); exit.Code == nil || *exit.Code != 0 || !strings.Contains(report, "the hosted runtime ran") {
		t.Fatalf("the declared node runtime did not run: exit %#v\n%s", exit, report)
	}

	// Undeclared: it starts, cannot read its own package, and fails from inside
	// the box with no refusal anywhere.
	undeclared := cloneProfile(profile)
	undeclared.ToolRead = []string{toolchain.Read[0]}
	exit, report := launchHosted(t, undeclared)
	if exit.Code == nil || *exit.Code == 0 {
		t.Fatalf("an undeclared runtime package still ran: exit %#v\n%s", exit, report)
	}
	if !strings.Contains(report, "MODULE_NOT_FOUND") {
		t.Fatalf("an undeclared runtime package failed some other way, so this measures something else:\n%s", report)
	}
}

// nodeProfile is the profile a Node-hosted repair runtime runs under, minus the
// projection and the endpoint: the box readable, its scope writable, no policy
// group, no network, and the resolved interpreter and package.
func nodeProfile(t *testing.T, repository string, created *box.Box, nono string, toolchain Toolchain) Profile {
	t.Helper()
	profile := boxProfile(t, repository, created, nono, buildKept(t), absolute(t, realGit(t)))
	profile.Domains = nil
	profile.BlockNetwork = true
	profile.ToolPath = append(append([]string(nil), toolchain.Path...), "/usr/bin", "/bin")
	profile.ToolRead = toolchain.Read
	return profile
}

// renderedWithout is the rendered profile with one grant taken out, which is
// how a test asks the provider whether its own baseline reaches a path kept
// declared.
func renderedWithout(t *testing.T, profile Profile, path string) nonoProfile {
	t.Helper()
	launch, err := NewNono(profile)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := renderNonoProfile(launch.profile, "/usr/bin/runtime")
	if err != nil {
		t.Fatal(err)
	}
	rendered.Filesystem.Read = without(rendered.Filesystem.Read, canonicalAncestor(path))
	return rendered
}

func askPath(t *testing.T, nono, profilePath, path string) providerAnswer {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	answer, err := askProvider(ctx, nono, profilePath, grantRequest{path: path, op: readable, why: "a measured premise"})
	if err != nil {
		t.Fatal(err)
	}
	return answer
}

// piSession is one confined Pi repair: the box, the profile, the projection and
// the packet. It is what a dispatcher will assemble; nothing in the tree
// dispatches a repair yet, so this test stands in for one and says so.
type piSession struct {
	launch     *Nono
	box        *box.Box
	state      *runtimestate.PiState
	executable string
	packet     thread.Packet
	// scope is the promise's bound repair scope, kept beside the packet
	// because a packet's bytes are sealed and a review needs the globs.
	scope []string
}

func livePiSession(t *testing.T) *piSession {
	t.Helper()
	nono := liveProvider(t)
	executable := livePi(t)
	requireLiveModel(t)
	node := realNode(t)

	root := ungrantedDir(t)
	repository, commit := piRepairRepository(t, root)
	created, err := box.Create(context.Background(), box.Request{
		Repository: repository,
		Commit:     commit,
		// The break is an overlay rather than a commit: a promise is broken in
		// the box a repair stands in, and the repository it was cut from is
		// left exactly as it was.
		Overlay: []box.OverlayFile{brokenPromiseOverlay(t, repository)},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := created.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})

	// The projection lies beside the box and the box's temporary directory,
	// outside the tree whose diff is collected and outside the source it was
	// copied from.
	state := piProjectionBeside(t, created)
	toolchain, err := ResolveNode(context.Background(), node, executable, 30*time.Second)
	if err != nil {
		t.Fatalf("resolve the node runtime hosting Pi: %v", err)
	}

	profile := boxProfile(t, repository, created, nono, buildKept(t), absolute(t, realGit(t)))
	// The promise's bound repair scope, which is what may be changed in its
	// name. See docs/decision/0047.
	profile.Write = []string{filepath.Join(created.Path(), "internal")}
	profile.Domains = nil
	profile.BlockNetwork = true
	profile.LoopbackPorts = []int{piLivePort}
	profile.Env = []string{runtimestate.PiCredentialEnvironment, "PI_CODING_AGENT_DIR"}
	profile.Home = state.Dir()
	profile.ToolPath = append(append([]string(nil), toolchain.Path...), "/usr/bin", "/bin")
	profile.ToolRead = toolchain.Read
	launch, err := NewNono(profile)
	if err != nil {
		t.Fatal(err)
	}

	session := &piSession{
		launch:     launch,
		box:        created,
		state:      state,
		executable: executable,
	}
	session.packet = session.assemblePacket(t)
	return session
}

func (s *piSession) config() thread.RuntimeConfig {
	return thread.RuntimeConfig{
		Runtime:    string(thread.Pi),
		Executable: s.executable,
		Version:    thread.PiVersion,
		PiProvider: piLiveProvider,
		PiModel:    piLiveModel,
		// The self-hosted endpoint's credential. It is separately configured,
		// reaches the child through the one kept-owned environment name the
		// projection refers to, and is never in argv, the packet or a record.
		PiCredential: "lm-studio",
		PiState:      s.state,
	}
}

func (s *piSession) childEnv() []string {
	return []string{
		"HOME=" + s.state.Dir(),
		"PI_CODING_AGENT_DIR=" + s.state.Dir(),
		runtimestate.PiCredentialEnvironment + "=lm-studio",
	}
}

// dispatch runs one confined session through the boundary a repair will use.
func (s *piSession) dispatch(t *testing.T, deadline time.Duration) (*thread.Receipt, ledger.Store) {
	t.Helper()
	// A scratch ledger: this is a box's session, and what it records is
	// evidence about the session rather than an act in the operator's history.
	store := ledger.NewFiles(t.TempDir())
	writable := ledger.Open(store, run.Start("pi-live-proof", nil))
	receipt, err := thread.NewReceipt(writable, thread.MintID())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), deadline+2*time.Minute)
	defer cancel()
	err = thread.Dispatch(ctx, s.config(), s.packet, s.launch, thread.Supervisor{Timeout: deadline}, receipt)
	if err != nil {
		t.Fatalf("dispatch a confined Pi repair: %v", err)
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
func (s *piSession) verdict(t *testing.T) tick.Answer {
	t.Helper()
	c, err := corpus.Load(filepath.Join(s.box.Path(), "docs"), s.box.Path(), corpus.Options{Gatherers: gather.Builtin()})
	if err != nil {
		t.Fatal(err)
	}
	store := ledger.NewFiles(t.TempDir())
	writable := ledger.Open(store, run.Start("pi-live-proof-gather", nil))
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
		if subject.ID != piRepairPromise {
			continue
		}
		if !subject.Answer.Gathered {
			t.Fatalf("the re-derivation carried an answer forward instead of measuring: %#v", subject.Answer)
		}
		return subject.Answer
	}
	t.Fatalf("the box's corpus holds no %s", piRepairPromise)
	return tick.Answer{}
}

// change is what the agent changed, which is what a person reviews: the paths
// and the diff, read from one snapshot of the box.
func (s *piSession) change(t *testing.T) box.Change {
	t.Helper()
	changed, err := s.box.Change(context.Background())
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
func (s *piSession) assemblePacket(t *testing.T) thread.Packet {
	t.Helper()
	c, err := corpus.Load(filepath.Join(s.box.Path(), "docs"), s.box.Path(), corpus.Options{Gatherers: gather.Builtin()})
	if err != nil {
		t.Fatal(err)
	}
	entry, found := c.Get(piRepairPromise)
	if !found {
		t.Fatalf("the box's corpus holds no %s", piRepairPromise)
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
	observation := s.observation(t, art)
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
		Why:         art.Body,
		Observation: observation,
		Policies:    nil,
	})
	if err != nil {
		t.Fatal(err)
	}
	return packet
}

// observation measures the broken promise in the box, which is the evidence the
// packet carries. It is the same instrument the verdict is re-derived with.
func (s *piSession) observation(t *testing.T, art *model.Artifact) ledger.Observation {
	t.Helper()
	catalogue := gather.Builtin()
	if len(art.Gather) != 1 {
		// One instrument, so the evidence in the packet is the whole of what
		// the promise was measured with rather than one of several.
		t.Fatalf("%s names %d gatherers, and this proof carries the evidence of one", art.ID, len(art.Gather))
	}
	name, spec := "", model.GatherSpec{}
	for key, value := range art.Gather {
		name, spec = key, value
	}
	g, known := catalogue[spec.Gatherer]
	if !known {
		t.Fatalf("%s names the %s gatherer, and there is no such gatherer", name, spec.Gatherer)
	}
	tree, err := workspace.Read(s.box.Path(), workspace.Options{})
	if err != nil {
		t.Fatal(err)
	}
	requests := gather.Requests{Tree: tree, World: model.World{Tree: tree.Digest()}, Deadline: 5 * time.Minute}
	observed := gather.NewRunner(catalogue).Gather(context.Background(), spec, requests.For(g, spec.Args))
	if observed.Err != nil {
		t.Fatalf("measure %s in the box: %v", art.ID, observed.Err)
	}
	return ledger.Measured(observed)
}

// brokenPromiseOverlay is the deliberate break: internal/corpus reaches
// internal/ledger, which is the one thing the promise says it does not.
//
// It is an added import and nothing else, so the diff a repair has to produce
// is the removal of one line. The tree no longer compiles, which is correct —
// the edge is a cycle — and `go/imports` still reports the graph, which is
// exactly what decision 0049 says a parsing instrument is for.
func brokenPromiseOverlay(t *testing.T, repository string) box.OverlayFile {
	t.Helper()
	path := filepath.Join(repository, filepath.FromSlash(piRepairFile))
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	const anchor = "\t\"github.com/pbleisch/kept/internal/model\"\n"
	if !strings.Contains(string(src), anchor) {
		t.Fatalf("%s no longer imports internal/model, so the break has nowhere to go", piRepairFile)
	}
	broken := strings.Replace(string(src), anchor, "\t_ \""+piRepairEdge+"\"\n"+anchor, 1)
	return box.OverlayFile{Path: piRepairFile, Data: []byte(broken), Mode: 0o644}
}

// piRepairRepository is a repository holding kept's own corpus and source, cut
// where the provider grants nothing.
//
// It is a copy rather than the checkout this test runs in: a box is created
// under <repository>/.kept/boxes, and cutting one inside the worktree a test
// runs from would write into it. The corpus and the Go source are what the
// promise is about; nothing else is copied.
func piRepairRepository(t *testing.T, parent string) (string, string) {
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
	scratchGit(t, root, "init", "-q")
	scratchGit(t, root, "config", "user.email", "test@example.com")
	scratchGit(t, root, "config", "user.name", "test")
	scratchGit(t, root, "add", "--all")
	scratchGit(t, root, "commit", "-qm", "origin")
	return root, strings.TrimSpace(scratchGit(t, root, "rev-parse", "HEAD"))
}

// piProjectionBeside materializes the Pi configuration projection beside the
// box, from a source that stands in for the operator's approved Pi agent
// directory. The operator's own is never read: decision 0052 says an adapter
// does not read the operator's home, and neither does its evidence.
func piProjectionBeside(t *testing.T, created *box.Box) *runtimestate.PiState {
	t.Helper()
	source := created.Path() + ".pi-source"
	if err := os.MkdirAll(source, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(source) })
	write := func(name string, value any) {
		src, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(source, name), src, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("auth.json", map[string]any{
		piLiveProvider: map[string]any{"type": "api_key", "key": "source-side-placeholder"},
	})
	write("models.json", map[string]any{"providers": map[string]any{
		piLiveProvider: map[string]any{
			"baseUrl": piLiveEndpoint,
			"api":     "openai-completions",
			"models":  []any{map[string]any{"id": piLiveModel}},
		},
	}})
	state, err := runtimestate.ProjectPi(runtimestate.PiRequest{
		Source:    source,
		Dir:       created.Path() + ".pi",
		Box:       created.Path(),
		Provider:  piLiveProvider,
		Model:     piLiveModel,
		Endpoints: []string{fmt.Sprintf("%s:%d", piLiveHost, piLivePort)},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = state.Remove() })
	return state
}

func livePi(t *testing.T) string {
	t.Helper()
	executable, err := exec.LookPath("pi")
	if err != nil {
		t.Skip("Pi is not installed")
	}
	executable = absolute(t, executable)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	reported, err := exec.CommandContext(ctx, executable, "--version").Output()
	if err != nil {
		t.Skip("Pi did not report a version: " + err.Error())
	}
	if strings.TrimSpace(string(reported)) != thread.PiVersion {
		t.Skipf("installed Pi is %s, the adapter is pinned to %s", strings.TrimSpace(string(reported)), thread.PiVersion)
	}
	return executable
}

// requireLiveModel checks the premise rather than discovering one. kept never
// asks an endpoint what it serves; this test does, so that a machine with
// nothing loaded skips instead of failing as though the adapter were broken.
func requireLiveModel(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, piLiveEndpoint+"/models", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Skipf("no self-hosted endpoint at %s: %v", piLiveEndpoint, err)
	}
	defer response.Body.Close()
	var served struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&served); err != nil {
		t.Skipf("the endpoint at %s did not answer with a model list: %v", piLiveEndpoint, err)
	}
	for _, model := range served.Data {
		if model.ID == piLiveModel {
			return
		}
	}
	t.Skipf("the endpoint at %s does not serve %s, so this repair has no authority to reach", piLiveEndpoint, piLiveModel)
}

func realGit(t *testing.T) string {
	t.Helper()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	return git
}

// moduleRoot is the checkout this test is compiled from.
func moduleRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("%s is not the module root: %v", root, err)
	}
	return root
}

func portOf(t *testing.T, address string) string {
	t.Helper()
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	return port
}

func receiptKinds(entries []thread.Entry) map[string]int {
	kinds := map[string]int{}
	for _, kind := range strings.Split(thread.ReceiptText(entries), ",") {
		kinds[kind]++
	}
	return kinds
}

func receiptKindText(entries []thread.Entry) string { return thread.ReceiptText(entries) }

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
			t.Fatalf("the live Pi receipt retained no %s native event: %v", typ, seen)
		}
	}
}

// sawPacket looks for the packet's marker in what the runtime produced. The
// promise id is the marker: the adapter puts it nowhere but the packet, so
// finding it in the session's own evidence is what says the bytes arrived.
func sawPacket(t *testing.T, store ledger.Store, entries []thread.Entry) bool {
	t.Helper()
	for _, entry := range entries {
		if entry.Native != nil && strings.Contains(string(entry.Native.Payload), piRepairPromise) {
			return true
		}
		if entry.Transcript == nil {
			continue
		}
		src, err := ledger.ReadArtifact(store, entry.Transcript.Artifact)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(src), piRepairPromise) {
			return true
		}
	}
	return false
}
