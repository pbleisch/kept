//go:build !windows

package confine

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/box"
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/run"
	"github.com/pbleisch/kept/internal/thread"
)

// This file is what a confined Claude repair needs, measured, and it is not
// decision 0052's admission evidence.
//
// The distinction is the whole point of the file and every test in it holds to
// it. 0052 admits a runtime on a real session, confined, carrying a real
// credential, killed after it has produced several receipt updates; that
// session exists now and it is in internal/repair/claudeadmission_test.go,
// where it belongs, because it needs the operator's own credential and skips
// without one.
//
// What is measured here is everything that does not need one: whether the
// runtime starts in a box at all, what it needs in order to, which host it
// reaches, what it can no longer reach, and which of those requirements produce
// a refusal before dispatch rather than an obscure exit from inside the box.
// The credential channel is measured here too, and by the one means that costs
// nothing and holds no secret: a deliberately invalid value, carried through the
// channel and rejected upstream by name.

// The one host this runtime needs. It is explicit configuration in the sense
// decision 0052 requires: nothing discovers a provider, a model or an endpoint.
const claudeLiveHost = "api.anthropic.com"

// claudeLiveDeadline bounds a session that reaches authentication and stops.
// Nothing external runs without a deadline. See docs/decision/0026.
const claudeLiveDeadline = 3 * time.Minute

// claudeLivePacketMarker stands in for the packet a dispatcher assembles. It is
// a promise id because that is what the Pi proof used and what a real packet
// carries; the adapter puts it nowhere but the packet, so finding it in the
// session's own evidence is what says the bytes arrived.
const claudeLivePacketMarker = "promise:the-corpus-cannot-write-standing-or-truth"

// claudeInvalidCredential is deliberately not a credential. Its whole job is to
// be carried through the channel and rejected upstream by name, which is what
// makes the channel measurable without spending anything or holding a secret.
const claudeInvalidCredential = "sk-ant-oat01-not-a-real-token"

// TestConfinedClaudeStartsReceivesThePacketAndIsAnsweredByName is the session
// that can be run today, end to end, and it settles four questions at once.
//
// The runtime starts inside a box. The packet reaches it and comes back out in
// its own words. The credential channel carries what it is supposed to carry,
// because the upstream identified it as an OAuth access token rather than an
// API key. And the surface `--safe-mode` leaves standing is what the runtime
// itself reports rather than what the flag's help claims.
//
// It is not a repair and it does not pretend to be one: there is no valid
// credential, so the session reaches authentication and stops.
func TestConfinedClaudeStartsReceivesThePacketAndIsAnsweredByName(t *testing.T) {
	session := liveClaudeSession(t)
	exit, stream, stderr := session.run(t, session.argv())

	if exit.Code == nil {
		t.Fatalf("the confined Claude session was signalled rather than exiting: %#v\n%s", exit, stderr)
	}
	events := claudeEvents(t, stream)
	init := claudeEvent(t, events, "system", "init")
	if init == nil {
		t.Fatalf("the confined session emitted no init event: %s\n%s", stream, stderr)
	}

	// It really is inside the box, by its own account.
	if cwd, _ := init["cwd"].(string); cwd != session.box.Path() {
		t.Fatalf("the confined session reports cwd %q, want the box at %q", cwd, session.box.Path())
	}
	if version, _ := init["claude_code_version"].(string); version != thread.ClaudeVersion {
		t.Fatalf("the confined session reports version %q, and the adapter is pinned to %s", version, thread.ClaudeVersion)
	}

	// The operator's own configuration surface is unreachable, because HOME and
	// the configuration directory are a fresh empty tree rather than theirs.
	for _, empty := range []string{"mcp_servers", "plugins"} {
		if list, _ := init[empty].([]any); len(list) != 0 {
			t.Fatalf("the confined session loaded %s: %v", empty, list)
		}
	}

	// The packet arrived, and this is the runtime's own record of reading it
	// rather than a marker that happened to turn up in a reply.
	replay := claudeEvent(t, events, "user", "")
	if replay == nil {
		t.Fatalf("the confined session replayed no user message, so the packet's arrival is unproven: %s", stream)
	}
	encoded, err := json.Marshal(replay)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), claudeLivePacketMarker) {
		t.Fatalf("the replayed message does not carry the packet marker: %s", encoded)
	}

	// The credential channel, answered by the upstream. A 401 naming an OAuth
	// access token says the value left the box through the one variable the
	// adapter sets, reached api.anthropic.com through the provider's allowlist,
	// and was read as the subscription credential it is meant to be. What is
	// missing is a valid one.
	result := claudeEvent(t, events, "result", "")
	if result == nil {
		t.Fatalf("the confined session produced no result event: %s", stream)
	}
	answer, _ := result["result"].(string)
	if !strings.Contains(answer, "401") || !strings.Contains(strings.ToLower(answer), "oauth") {
		t.Fatalf("the upstream did not answer the credential channel by name: %q", answer)
	}
	if status, _ := result["api_error_status"].(float64); status != 401 {
		t.Fatalf("the confined session's api_error_status = %v, want the 401 that proves the host was reached", result["api_error_status"])
	}
	t.Logf("confined Claude %s: %d native events, upstream answered %q", thread.ClaudeVersion, len(events), answer)

	// And the argv this session was driven with is the adapter's own, rather
	// than a copy of it that could drift. What the adapter adds to the pinned
	// list is the declared ask of decision 0058, which needs no measurement
	// here: this file is about what a confined session needs in order to start,
	// and the ask is about what it does once it has.
	adapterArgs := session.adapterArgv(t)
	if !slices.Equal(adapterArgs[:len(claudeFixtureArgv)], claudeFixtureArgv) {
		t.Fatalf("the argv this session ran is not the adapter's: ran %v, adapter %v", claudeFixtureArgv, adapterArgs)
	}
	if rest := adapterArgs[len(claudeFixtureArgv):]; len(rest) != 2 || rest[0] != "--append-system-prompt" || rest[1] == "" {
		t.Fatalf("the adapter's argv adds %v to the pinned list, want the declared ask", rest)
	}
}

// TestAConfinedClaudeNeedsItsOwnNameForTheTemporaryDirectory is the measurement
// decision 0056 was written from, run both ways so the second half is not an
// argument.
//
// The runtime does not read TMPDIR. It reads CLAUDE_CODE_TMPDIR and otherwise
// uses a hardcoded /tmp, which a repair profile denies by name — so a session
// without the variable opens a path nobody granted and dies before emitting a
// single event. Declared, the same session runs.
func TestAConfinedClaudeNeedsItsOwnNameForTheTemporaryDirectory(t *testing.T) {
	session := liveClaudeSession(t)

	// Declared: the session starts and gets as far as its credential.
	if _, stream, _ := session.run(t, session.argv()); claudeEvent(t, claudeEvents(t, stream), "system", "init") == nil {
		t.Fatalf("the declared scratch directory did not let the session start: %s", stream)
	}
	// And it used it, rather than working for some other reason.
	if _, err := os.Stat(filepath.Join(session.box.Temp(), fmt.Sprintf("claude-%d", os.Getuid()))); err != nil {
		t.Fatalf("the confined session wrote no scratch tier under the declared directory: %v", err)
	}

	// Undeclared: it starts, cannot make its scratch tier at the path it
	// hardcodes, and fails from inside the box with no refusal anywhere.
	//
	// The profile is what publishes the name, so taking it out of the
	// invocation's environment changes nothing — which is the mechanism working
	// and is why the undeclared half has to be a different profile.
	undeclared := cloneProfile(session.launch.profile)
	undeclared.TempEnv = nil
	launch, err := NewNono(undeclared)
	if err != nil {
		t.Fatal(err)
	}
	bare := *session
	bare.launch = launch
	bare.temp = ""
	exit, stream, stderr := bare.run(t, bare.argv())
	if exit.Code == nil || *exit.Code == 0 {
		t.Fatalf("a session with no declared scratch directory still succeeded: %#v\n%s", exit, stream)
	}
	if claudeEvent(t, claudeEvents(t, stream), "system", "init") != nil {
		t.Fatalf("a session with no declared scratch directory still started: %s", stream)
	}
	if !strings.Contains(stderr, "EPERM") || !strings.Contains(stderr, "/tmp/") {
		t.Fatalf("the undeclared scratch directory failed some other way, so this measures something else:\n%s", stderr)
	}
	t.Logf("undeclared scratch directory: exit %d, %q", *exit.Code, stderr)

	// The provider agrees the hardcoded path is denied, which is what says the
	// failure above is this profile's boundary rather than a broken machine.
	answer := askPath(t, session.nono, writeRendered(t, renderedProfile(t, session.launch, session.executable)), "/tmp")
	if answer.Status == providerAllowed {
		t.Fatalf("the profile grants the host scratch tree it is supposed to deny: %#v", answer)
	}
}

// TestSafeModeStopsTheBoxsOwnHookFromRunning confirms the flag in the box
// rather than trusting its help, and the thing it is aimed at is real: the tree
// under repair is somebody's repository and may carry its own
// .claude/settings.json.
//
// Confinement still bounds what such a command could touch — decision 0017 —
// but the flag is what keeps the tree being repaired from choosing what runs at
// all. Both branches run, because a hook that did not fire proves nothing
// unless the same hook fires when the flag is dropped.
func TestSafeModeStopsTheBoxsOwnHookFromRunning(t *testing.T) {
	for _, tc := range []struct {
		name  string
		safe  bool
		fired bool
	}{
		{"with --safe-mode", true, false},
		{"without it", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			session := liveClaudeSession(t)
			marker := filepath.Join(session.box.Path(), "internal", "the-hook-ran")
			settings := filepath.Join(session.box.Path(), ".claude")
			if err := os.MkdirAll(settings, 0o755); err != nil {
				t.Fatal(err)
			}
			hook := `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"/usr/bin/touch ` + marker + `"}]}]}}`
			if err := os.WriteFile(filepath.Join(settings, "settings.json"), []byte(hook), 0o644); err != nil {
				t.Fatal(err)
			}
			argv := session.argv()
			if !tc.safe {
				argv = without(argv, "--safe-mode")
			}
			_, stream, stderr := session.run(t, argv)
			if claudeEvent(t, claudeEvents(t, stream), "system", "init") == nil {
				t.Fatalf("the session did not start, so the hook had no chance to run: %s\n%s", stream, stderr)
			}
			_, err := os.Stat(marker)
			if fired := err == nil; fired != tc.fired {
				t.Fatalf("the box's own SessionStart hook fired = %v, want %v", fired, tc.fired)
			}
		})
	}
}

// TestConfinedClaudeReachesOnlyItsOneDeclaredHost measures the network from
// inside the box rather than from the profile, the way the Pi proof does.
//
// A repair box for this runtime cannot block the network: the subscription
// credential is answered by one host and nothing else will do. Measured with
// the network blocked, the session retried ten times with exponential backoff
// and gave up after roughly three minutes with ConnectionRefused — which is why
// the profile names a host instead.
//
// What the allowlist grants is a proxy rather than a socket, which is the same
// lesson decision 0055 records from the other side. Measured here: the declared
// host answers a proxy-aware client and refuses a raw connection to its own
// port 443, an undeclared host is refused through the proxy, and an undeclared
// loopback port is refused outright. So "reachable" means reachable the way an
// HTTP client reaches it, and nothing in the box gets a socket to the internet.
func TestConfinedClaudeReachesOnlyItsOneDeclaredHost(t *testing.T) {
	session := liveClaudeSession(t)

	// The undeclared host is a listener of the parent's own, so it is really
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
	undeclared := listener.Addr().String()
	probe, err := net.DialTimeout("tcp", undeclared, 10*time.Second)
	if err != nil {
		t.Skipf("this machine cannot reach %s unconfined, so a measurement from inside the box would mean nothing: %v", undeclared, err)
	}
	_ = probe.Close()

	stream := &testStream{}
	child, err := session.launch.Start(context.Background(), thread.Invocation{
		Path: "/bin/sh",
		Args: []string{"-c", `
/usr/bin/env | /usr/bin/grep -i proxy | /usr/bin/sed 's/^/env /'
/usr/bin/curl -s -o /dev/null -m 20 -w 'declared=%{http_code}
' "https://$1/v1/models"
/usr/bin/curl -s -o /dev/null -m 20 -w 'other=%{http_code}
' https://example.com/
/usr/bin/nc -z -G 5 -w 5 "$1" 443 >/dev/null 2>&1 && echo raw=reached || echo raw=refused
/usr/bin/nc -z -G 5 -w 5 127.0.0.1 "$2" >/dev/null 2>&1 && echo loopback=reached || echo loopback=refused
`, "sh", claudeLiveHost, portOf(t, undeclared)},
		Env:    session.childEnv(),
		Output: stream,
	})
	if err != nil {
		t.Fatal(err)
	}
	child.Wait()
	report := stream.String()
	t.Logf("confined network report:\n%s", report)
	for _, want := range []string{
		// Through the provider's proxy, the upstream itself answered. 401 is
		// the host's own reply to a request carrying no credential, so it is a
		// stronger statement than a connection that merely opened.
		"declared=401",
		// A host nobody named: curl reports no HTTP status at all.
		"other=000",
		// And the allowlist is not a socket grant, which is why the runtime's
		// own client reaches the host and this does not.
		"raw=refused",
		"loopback=refused",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("confined network report is missing %q:\n%s", want, report)
		}
	}
	// The provider routes the child through its own proxy and says so in the
	// child's environment. The adapter refuses those names in the *parent*
	// environment for a different reason — an operator's proxy is an unreviewed
	// redirect of the credential — and the two do not collide, because kept
	// never sets them and the provider always does.
	if !strings.Contains(report, "env HTTPS_PROXY=http://") {
		t.Fatalf("the confined child was not routed through the provider's proxy:\n%s", report)
	}
}

// TestAConfinedNativeRuntimeNeedsNoFilesystemGrant is the resolution
// measurement, and its answer is the opposite of the Node-hosted one.
//
// A Node-hosted runtime is a script somebody else has to read, so the box needs
// the package granted or it reports MODULE_NOT_FOUND. A self-contained native
// image has nothing to read: the provider's command policy grants the launched
// entrypoint a direct exec bypass, so the image runs with no grant naming it at
// all. Both halves run at a path the provider answers `path_not_granted` for,
// because measuring this under /opt/homebrew would measure the baseline.
func TestAConfinedNativeRuntimeNeedsNoFilesystemGrant(t *testing.T) {
	claude := liveClaude(t)
	real, err := filepath.EvalSymlinks(claude)
	if err != nil {
		t.Fatal(err)
	}
	root := ungrantedDir(t)
	installed := filepath.Join(root, "opt", "claude-code", "bin")
	if err := os.MkdirAll(installed, 0o700); err != nil {
		t.Fatal(err)
	}
	// A hard link rather than a copy: it is the same image at a second name,
	// and the sandbox decides about paths.
	linked := filepath.Join(installed, "claude")
	if err := os.Link(real, linked); err != nil {
		t.Skipf("the runtime image cannot be linked outside its own volume, so an ungranted path cannot be measured here: %v", err)
	}

	session := claudeSessionAt(t, root, linked)
	// The premise: without a declaration, the provider reaches this through
	// nothing. Otherwise the comparison below is between two grants.
	if answer := askPath(t, session.nono, writeRendered(t, renderedProfile(t, session.launch, linked)), linked); answer.Status == providerAllowed {
		t.Skipf("this host's provider baseline already grants %s, so the absence cannot be measured here: %#v", linked, answer)
	}

	// Undeclared, which is what kept renders: the image runs anyway.
	if _, stream, stderr := session.run(t, session.argv()); claudeEvent(t, claudeEvents(t, stream), "system", "init") == nil {
		t.Fatalf("the native runtime did not start at an ungranted path: %s\n%s", stream, stderr)
	}

	// And declared, which is what a Node-hosted runtime would have needed: the
	// same session, so the absence above is not a session that failed quietly.
	granted := cloneProfile(session.launch.profile)
	granted.ToolRead = append(granted.ToolRead, installed)
	launch, err := NewNono(granted)
	if err != nil {
		t.Fatal(err)
	}
	declared := *session
	declared.launch = launch
	if _, stream, stderr := declared.run(t, declared.argv()); claudeEvent(t, claudeEvents(t, stream), "system", "init") == nil {
		t.Fatalf("the native runtime did not start with its directory granted either: %s\n%s", stream, stderr)
	}

	// Which is why resolution declares nothing for it, and refuses the shape
	// that would have needed a declaration.
	toolchain, err := ResolveNative(linked)
	if err != nil {
		t.Fatalf("resolve the native runtime: %v", err)
	}
	if len(toolchain.Read) != 0 || len(toolchain.Path) != 0 {
		t.Fatalf("native resolution declared %+v for an image that reads nothing", toolchain)
	}
}

// TestARealFailedCanaryBlocksTheConfinedClaudeLaunch is the other half of "the
// runtime cannot read the ledger, the keychain or a credential that can push to
// origin": those are the canary's probes, and this is what says a canary that
// did not report is a refusal rather than a launch.
//
// The break is the canary helper itself — the profile names an executable that
// is not kept, so the confined helper produces no report at all. Nothing about
// the repair profile is weakened, which is why the refusal is about
// verification rather than about a boundary this test removed.
func TestARealFailedCanaryBlocksTheConfinedClaudeLaunch(t *testing.T) {
	session := liveClaudeSession(t)
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
	if _, err := launch.Start(context.Background(), thread.Invocation{Path: session.executable, Output: &testStream{}}); err == nil {
		t.Fatal("a failed canary did not refuse the launch")
	} else if !strings.Contains(err.Error(), "canary failed") {
		t.Fatalf("launch error = %v, want it to name the canary", err)
	}

	// And the unbroken profile verifies, which is what says the refusal was
	// about the canary rather than about anything else in this box.
	if err := session.launch.Verified(); err != nil {
		t.Fatalf("the repair profile's own canary failed: %v", err)
	}
}

// claudeSession is one confined Claude box: the box, the profile, the scratch
// directory and the session home. It is what a dispatcher will assemble;
// nothing in the tree dispatches a repair yet, so this stands in for one and
// says so.
type claudeSession struct {
	nono       string
	launch     *Nono
	box        *box.Box
	executable string
	home       string
	temp       string
}

func liveClaudeSession(t *testing.T) *claudeSession {
	t.Helper()
	return claudeSessionAt(t, ungrantedDir(t), liveClaude(t))
}

func claudeSessionAt(t *testing.T, root, executable string) *claudeSession {
	t.Helper()
	nono := liveProvider(t)
	repository, created := scratchBox(t, root)
	home := filepath.Join(root, "claude-home-"+filepath.Base(created.Path()))
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(home) })

	// Resolution declares nothing for a native image, so this asserts the
	// absence rather than spreading the empty toolchain around.
	toolchain, err := ResolveNative(executable)
	if err != nil {
		t.Fatalf("resolve the native Claude runtime: %v", err)
	}
	profile := boxProfile(t, repository, created, nono, buildKept(t), absolute(t, realGit(t)))
	// The promise's bound repair scope, which is what may be changed in its
	// name. See docs/decision/0047.
	profile.Write = []string{filepath.Join(created.Path(), "internal")}
	// One host. A blocked network is wrong for this runtime and was measured as
	// such: ten retries, roughly three minutes, ConnectionRefused.
	profile.Domains = []string{claudeLiveHost}
	profile.Env = []string{"CLAUDE_CODE_OAUTH_TOKEN", "CLAUDE_CONFIG_DIR"}
	profile.Home = home
	profile.TempEnv = []string{"CLAUDE_CODE_TMPDIR"}
	profile.ToolPath = append(append([]string(nil), toolchain.Path...), "/usr/bin", "/bin")
	profile.ToolRead = toolchain.Read
	launch, err := NewNono(profile)
	if err != nil {
		t.Fatal(err)
	}
	return &claudeSession{
		nono:       nono,
		launch:     launch,
		box:        created,
		executable: executable,
		home:       home,
		temp:       created.Temp(),
	}
}

// argv is the adapter's own fixed argv. It is read from the fixture the adapter
// pins rather than restated here, so a test cannot pass against a flag list the
// adapter does not use.
func (s *claudeSession) argv() []string {
	return append([]string(nil), claudeFixtureArgv...)
}

func (s *claudeSession) childEnv() []string {
	env := []string{
		"HOME=" + s.home,
		"CLAUDE_CONFIG_DIR=" + filepath.Join(s.home, "config"),
		"CLAUDE_CODE_OAUTH_TOKEN=" + claudeInvalidCredential,
	}
	if s.temp != "" {
		env = append(env, "CLAUDE_CODE_TMPDIR="+s.temp)
	}
	return env
}

// run is one confined session, bounded, with its stdout and stderr separated:
// the stream is evidence, and the diagnostic is how a failure inside the box
// says what it was.
func (s *claudeSession) run(t *testing.T, argv []string) (thread.Exit, string, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), claudeLiveDeadline)
	defer cancel()
	stream := &testStream{}
	child, err := s.launch.Start(ctx, thread.Invocation{
		Path:  s.executable,
		Args:  argv,
		Input: claudeLivePacket(),
		Env:   s.childEnv(),
		// The runtime's session ends when it has answered, so the launch closes
		// the input channel itself. Nothing here binds it.
		Output: stream,
	})
	if err != nil {
		t.Fatal(err)
	}
	exit := child.Wait()
	diagnostic := ""
	if reporter, ok := child.(interface{ diagnostic() string }); ok {
		diagnostic = reporter.diagnostic()
	}
	return exit, stream.String(), diagnostic
}

// adapterArgv is the argv the adapter would hand confinement for this box. It
// is asked for rather than started: what this file needs from the adapter is
// the command line, and the session that runs it is the one above.
//
// The home is a fresh one rather than this session's, because the adapter
// requires an empty session directory and a session that has already run has
// written into its own. That is a dispatcher's job per session; here it only
// has to be clean enough that the invocation is built.
func (s *claudeSession) adapterArgv(t *testing.T) []string {
	t.Helper()
	store := ledger.NewFiles(t.TempDir())
	writable := ledger.Open(store, run.Start("claude-in-box", nil))
	receipt, err := thread.NewReceipt(writable, thread.MintID())
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := thread.Select(string(thread.Claude))
	if err != nil {
		t.Fatal(err)
	}
	adapterReceipt, err := receipt.ForAdapter(adapter.Runtime(), adapter.AdapterVersion())
	if err != nil {
		t.Fatal(err)
	}
	packet, err := claudeProofPacket()
	if err != nil {
		t.Fatal(err)
	}
	invocation, err := adapter.Prepare(thread.RuntimeConfig{
		Runtime:                string(thread.Claude),
		Executable:             s.executable,
		Version:                thread.ClaudeVersion,
		ClaudeSetupToken:       claudeInvalidCredential,
		ClaudeSetupTokenExpiry: time.Now().Add(365 * 24 * time.Hour),
		ClaudeHome:             t.TempDir(),
		ClaudeTemp:             s.temp,
		ClaudeAsk:              thread.ClaudeRepairAsk,
	}, packet, adapterReceipt)
	if err != nil {
		t.Fatalf("the adapter refused to build an invocation for this box: %v", err)
	}
	return invocation.Args
}

// claudeLivePacket is the one JSONL user message the adapter's input channel
// carries, written here rather than taken from the adapter. This file measures
// what a session needs in order to start, and building a packet from a corpus
// would make that depend on what a corpus said. The argv is taken from the
// adapter, because the argv is the thing under measurement.
func claudeLivePacket() []byte {
	return []byte(`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"` +
		claudeLivePacketMarker + ` is not kept in this box"}]}}` + "\n")
}

func claudeProofPacket() (thread.Packet, error) {
	return thread.AssemblePacket(thread.PacketInput{
		Promise: thread.RatifiedPromise{
			ID:          claudeLivePacketMarker,
			Hash:        "sha256:claude-proof",
			Norm:        1,
			Ratified:    true,
			Statement:   "the corpus cannot write standing or truth",
			Gather:      map[string]model.GatherSpec{"imports": {Gatherer: "go/imports"}},
			HoldWhen:    "obs.imports.ok",
			Trigger:     model.TriggerCadence,
			RepairScope: []string{"internal/**"},
		},
		Why: "the adapter's dispatch gate is what this packet is for.",
		Observation: ledger.Observation{
			V:        ledger.ObsVersion,
			At:       time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC),
			Gatherer: "go/imports@1",
			World:    model.World{Tree: "sha256:claude-proof-world"},
			Scope:    "sha256:claude-proof-scope",
			Data:     map[string]any{"ok": false},
		},
	})
}

// claudeFixtureArgv is the adapter's fixed argv, read from the fixture the
// adapter's own test pins to it.
var claudeFixtureArgv = loadClaudeArgv()

func loadClaudeArgv() []string {
	src, err := os.ReadFile(filepath.Join("..", "thread", "testdata", "claude", "fixture.json"))
	if err != nil {
		return nil
	}
	var fixture struct {
		Argv []string `json:"direct_argv"`
	}
	if json.Unmarshal(src, &fixture) != nil {
		return nil
	}
	return fixture.Argv
}

// claudeEvents parses a session's stdout into the native events it carried.
func claudeEvents(t *testing.T, stream string) []map[string]any {
	t.Helper()
	var events []map[string]any
	for _, line := range strings.Split(stream, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("the confined session emitted a line that is not a native event: %q: %v", line, err)
		}
		events = append(events, event)
	}
	return events
}

func claudeEvent(t *testing.T, events []map[string]any, typ, subtype string) map[string]any {
	t.Helper()
	for _, event := range events {
		if event["type"] != typ {
			continue
		}
		if subtype != "" && event["subtype"] != subtype {
			continue
		}
		return event
	}
	return nil
}

// realClaude is the installed runtime, whatever version it is. Resolution does
// not depend on a version, so its tests do not skip on one.
func realClaude(t *testing.T) string {
	t.Helper()
	executable, err := exec.LookPath("claude")
	if err != nil {
		t.Skip("Claude Code is not installed")
	}
	return absolute(t, executable)
}

// liveClaude is the installed runtime at the version the adapter is pinned to.
// A different one is skipped rather than measured: a session's evidence is
// about the version that produced it.
func liveClaude(t *testing.T) string {
	t.Helper()
	executable := realClaude(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	reported, err := exec.CommandContext(ctx, executable, "--version").Output()
	if err != nil {
		t.Skip("Claude Code did not report a version: " + err.Error())
	}
	version, _, _ := strings.Cut(strings.TrimSpace(string(reported)), " ")
	if version != thread.ClaudeVersion {
		t.Skipf("installed Claude Code is %s, the adapter is pinned to %s", version, thread.ClaudeVersion)
	}
	if len(claudeFixtureArgv) == 0 {
		t.Fatal("the Claude fixture declares no direct argv, so a session here would measure a flag list nobody pinned")
	}
	return executable
}
