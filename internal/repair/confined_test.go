//go:build !windows

package repair

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/box"
	"github.com/pbleisch/kept/internal/config"
	"github.com/pbleisch/kept/internal/confine"
	"github.com/pbleisch/kept/internal/thread"
)

// This file is decision 0057's measurement, run as a test.
//
// The question it answers could not be settled by reading the provider's
// documentation, and it could not be settled from the upstream's reply either:
// api.anthropic.com answers `OAuth access token is invalid` for *any* bearer
// value, so a 401 proves the host was reached and says nothing about whether
// the header was rewritten. Reading the request at the upstream is what
// separates them, so the upstream here is a listener of this test's own.
//
// None of it needs a valid credential, which is why it can run at all.

// theBrokeredSecret is what the operator's keychain would hold. Here it is
// written to a file the confined child is granted nothing under, so the value
// really exists on this machine and its absence from the box is a measurement
// rather than a tautology.
const theBrokeredSecret = "sk-ant-oat01-brokered-proof-value-not-a-real-token"

// claudeLiveDeadline bounds a session that reaches its upstream and stops.
// Nothing external runs without a deadline. See docs/decision/0026.
const claudeLiveDeadline = 3 * time.Minute

// TestAConfinedClaudeSendsAPhantomAndTheUpstreamReceivesTheCredential is the
// whole channel, end to end, with a real runtime.
//
// Three things are read rather than argued: what the child holds, what the
// upstream receives, and that they are different.
func TestAConfinedClaudeSendsAPhantomAndTheUpstreamReceivesTheCredential(t *testing.T) {
	upstream := recordingUpstream_(t)
	session := brokeredClaudeSession(t, upstream.URL())

	// What the child holds. The provider publishes the phantom in the variable
	// the profile named, and the profile is the only thing that names it — the
	// invocation may not, and the launch refuses one that does.
	phantom := session.probe(t, `printf 'phantom=%s\n' "$CLAUDE_CODE_OAUTH_TOKEN"`)
	held := strings.TrimSpace(strings.TrimPrefix(phantom, "phantom="))
	if held == "" {
		t.Fatalf("the confined child holds no credential variable at all: %q", phantom)
	}
	if held == theBrokeredSecret {
		t.Fatal("the confined child holds the real credential, so nothing was brokered")
	}
	if strings.Contains(phantom, theBrokeredSecret) {
		t.Fatalf("the credential value reached the box: %q", phantom)
	}
	t.Logf("the box holds a phantom of %d characters and not the credential", len(held))

	// What the upstream receives. This is the half no reply from Anthropic can
	// establish, and it is the reason the upstream is a listener here.
	exit, stream, stderr := session.run(t)
	if claudeEvent(t, claudeEvents(t, stream), "system", "init") == nil {
		t.Fatalf("the confined session did not start: %#v\n%s\n%s", exit, stream, stderr)
	}
	request := upstream.first(t)
	if request == nil {
		t.Fatalf("the upstream received no request, so nothing was routed: %s\n%s", stream, stderr)
	}
	if got := request.Header.Get("Authorization"); got != "Bearer "+theBrokeredSecret {
		t.Fatalf("the upstream received Authorization %q, want the credential the reference names", got)
	}
	if strings.Contains(request.Header.Get("Authorization"), held) {
		t.Fatal("the upstream received the phantom, so the header was never rewritten")
	}
	// And it is the runtime's own request rather than something else in the
	// box: one endpoint, which is the one the endpoint rules name.
	if request.Method != "POST" || !strings.HasPrefix(request.URL.Path, "/v1/messages") {
		t.Fatalf("the upstream received %s %s, which is not what the endpoint rules admit", request.Method, request.URL.Path)
	}
	t.Logf("the upstream received %s %s carrying the credential the box never held", request.Method, request.URL.RequestURI())
}

// TestABrokeredClaudeReachesNoEndpointNobodyMeasured is the default-deny half.
// The rules are an allowlist at the provider, and a rule list that has never
// refused anything is not a rule list.
func TestABrokeredClaudeReachesNoEndpointNobodyMeasured(t *testing.T) {
	upstream := recordingUpstream_(t)
	session := brokeredClaudeSession(t, upstream.URL())

	report := session.probe(t, `
B="$ANTHROPIC_BASE_URL"
/usr/bin/curl -s -o /dev/null -m 20 -w 'named=%{http_code}\n'   -X POST "$B/v1/messages" -H "Authorization: Bearer $CLAUDE_CODE_OAUTH_TOKEN" -d '{}'
/usr/bin/curl -s -o /dev/null -m 20 -w 'unnamed=%{http_code}\n' -X POST "$B/v1/organizations" -H "Authorization: Bearer $CLAUDE_CODE_OAUTH_TOKEN" -d '{}'
/usr/bin/curl -s -o /dev/null -m 20 -w 'method=%{http_code}\n'  -X GET  "$B/v1/messages" -H "Authorization: Bearer $CLAUDE_CODE_OAUTH_TOKEN"
/usr/bin/curl -s -o /dev/null -m 20 -w 'nophantom=%{http_code}\n' -X POST "$B/v1/messages" -H "Authorization: Bearer not-the-phantom" -d '{}'
`)
	for _, want := range []string{
		// The measured endpoint, forwarded.
		"named=200",
		// An endpoint nobody measured, and a method nobody measured: refused by
		// the provider, so the credential is never spent on either.
		"unnamed=403",
		"method=403",
		// And the phantom is a capability rather than decoration: without it the
		// proxy refuses before the upstream is reached.
		"nophantom=401",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("confined endpoint report is missing %q:\n%s", want, report)
		}
	}
	// Only the admitted one reached the upstream, which is what says the 403s
	// were refusals rather than requests that failed for some other reason.
	for _, request := range upstream.all() {
		if request.Method != "POST" || !strings.HasPrefix(request.URL.Path, "/v1/messages") {
			t.Fatalf("a refused request reached the upstream anyway: %s %s", request.Method, request.URL.Path)
		}
	}
}

// TestARealConfinedClaudeIsAnsweredByTheRealUpstreamThroughTheRoute is the
// reachability half, against api.anthropic.com itself.
//
// It cannot show a rewrite — the upstream answers the same way for any bearer
// value, which is measured above — so what it establishes is the other thing:
// the route that carries a phantom out of the box really does reach the host
// the credential is answered by, and the answer names the credential kind.
func TestARealConfinedClaudeIsAnsweredByTheRealUpstreamThroughTheRoute(t *testing.T) {
	session := brokeredClaudeSession(t, ClaudeUpstream)
	exit, stream, stderr := session.run(t)
	if exit.Code == nil {
		t.Fatalf("the confined session was signalled rather than exiting: %#v\n%s", exit, stderr)
	}
	events := claudeEvents(t, stream)
	if claudeEvent(t, events, "system", "init") == nil {
		t.Fatalf("the confined session did not start: %s\n%s", stream, stderr)
	}
	result := claudeEvent(t, events, "result", "")
	if result == nil {
		t.Fatalf("the confined session produced no result event: %s", stream)
	}
	answer, _ := result["result"].(string)
	if !strings.Contains(answer, "401") || !strings.Contains(strings.ToLower(answer), "oauth") {
		t.Fatalf("the upstream did not answer the brokered channel by name: %q", answer)
	}
	// Nothing in the session's own evidence carries the credential, and there
	// is nothing in this process that could have put it there.
	if strings.Contains(stream, theBrokeredSecret) || strings.Contains(stderr, theBrokeredSecret) {
		t.Fatal("the credential value appears in the session's output")
	}
	t.Logf("brokered Claude %s: upstream answered %q", thread.ClaudeVersion, answer)
}

// brokeredClaudeSession is one confined Claude box whose credential is brokered,
// assembled the way a dispatcher assembles one: the operator's configuration in,
// a runtime configuration and a credential route out.
func brokeredClaudeSession(t *testing.T, upstream string) *brokeredSession {
	t.Helper()
	nono := liveProvider(t)
	executable := liveClaude(t)
	root := ungrantedDir(t)
	repository, created := scratchBox(t, root)

	home := filepath.Join(root, "claude-home-"+filepath.Base(created.Path()))
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(home) })

	// The credential lives where an operator's does: outside the box, outside
	// every grant, readable only by its owner. A keychain reference is the
	// other supported shape and is not used here, because resolving one raises
	// the system's approval dialog and blocks until somebody answers it.
	secret := filepath.Join(ungrantedDir(t), "operator-credential")
	if err := os.WriteFile(secret, []byte(theBrokeredSecret), 0o600); err != nil {
		t.Fatal(err)
	}

	assembled, err := Assemble(context.Background(), config.Repair{
		Runtime:           string(thread.Claude),
		Executable:        executable,
		Credential:        "file://" + secret,
		CredentialExpires: time.Now().Add(365 * 24 * time.Hour),
	}, Session{Home: home, Temp: created.Temp()})
	if err != nil {
		t.Fatalf("assemble the brokered runtime: %v", err)
	}
	route := *assembled.Credential
	route.Upstream = upstream

	toolchain, err := confine.ResolveNative(executable)
	if err != nil {
		t.Fatal(err)
	}
	profile := confinementProfile(t, repository, created, nono)
	profile.Write = []string{filepath.Join(created.Path(), "internal")}
	profile.Domains = []string{assembled.Domain}
	profile.Env = assembled.Env
	profile.TempEnv = assembled.TempEnv
	profile.Home = home
	profile.Credential = &route
	profile.ToolPath = append(append([]string(nil), toolchain.Path...), "/usr/bin", "/bin")
	profile.ToolRead = toolchain.Read
	launch, err := confine.NewNono(profile)
	if err != nil {
		t.Fatal(err)
	}
	return &brokeredSession{launch: launch, executable: executable, home: home}
}

type brokeredSession struct {
	launch     *confine.Nono
	executable string
	home       string
}

// childEnv is the adapter's child environment under this channel: no
// credential, because the profile publishes it. The launch refuses one that
// names the credential variable, so this cannot drift.
func (s *brokeredSession) childEnv() []string {
	return []string{
		"HOME=" + s.home,
		thread.ClaudeConfigEnvironment + "=" + filepath.Join(s.home, "config"),
	}
}

func (s *brokeredSession) run(t *testing.T) (thread.Exit, string, string) {
	t.Helper()
	return s.start(t, s.executable, claudeArgv(t), claudeLivePacket())
}

// probe runs a shell in the same box and under the same profile, which is how
// the child's own view of the credential channel is read.
func (s *brokeredSession) probe(t *testing.T, script string) string {
	t.Helper()
	_, out, stderr := s.start(t, "/bin/sh", []string{"-c", script}, nil)
	if strings.TrimSpace(out) == "" && stderr != "" {
		t.Fatalf("the confined probe produced nothing: %s", stderr)
	}
	return out
}

func (s *brokeredSession) start(t *testing.T, path string, args []string, input []byte) (thread.Exit, string, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), claudeLiveDeadline)
	defer cancel()
	stream := &testStream{}
	child, err := s.launch.Start(ctx, thread.Invocation{
		Path:   path,
		Args:   args,
		Input:  input,
		Env:    s.childEnv(),
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

// recordingUpstream stands in for the service the credential is spent against,
// so that what it receives can be read. It answers a stream the runtime accepts,
// because a session that fails at the first reply proves less than one that
// completes.
type recordingUpstream struct {
	listener net.Listener
	mu       sync.Mutex
	seen     []*http.Request
}

func (u *recordingUpstream) URL() string { return "http://" + u.listener.Addr().String() }

func (u *recordingUpstream) all() []*http.Request {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]*http.Request(nil), u.seen...)
}

func (u *recordingUpstream) first(t *testing.T) *http.Request {
	t.Helper()
	all := u.all()
	if len(all) == 0 {
		return nil
	}
	return all[0]
}

func recordingUpstream_(t *testing.T) *recordingUpstream {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	upstream := &recordingUpstream{listener: listener}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		upstream.mu.Lock()
		upstream.seen = append(upstream.seen, r.Clone(context.Background()))
		upstream.mu.Unlock()
		if !strings.HasPrefix(r.URL.Path, "/v1/messages") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, anthropicStreamReply)
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	})
	return upstream
}

// anthropicStreamReply is the shape of a completed message, so a session that
// reaches the upstream finishes rather than erroring at the first byte.
var anthropicStreamReply = strings.Join([]string{
	`event: message_start`,
	`data: {"type":"message_start","message":{"id":"msg_proof","type":"message","role":"assistant","model":"claude-sonnet-5","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}}`, ``,
	`event: content_block_start`,
	`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`, ``,
	`event: content_block_delta`,
	`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}`, ``,
	`event: content_block_stop`,
	`data: {"type":"content_block_stop","index":0}`, ``,
	`event: message_delta`,
	`data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":1}}`, ``,
	`event: message_stop`,
	`data: {"type":"message_stop"}`, ``, ``,
}, "\n")

// confinementProfile is the repair profile every box in this file runs under,
// built from exported policy fields rather than borrowed from the confinement
// package's own tests. Only the credential half is what these tests are about;
// the rest is the boundary decisions 0017 and 0053 already established.
func confinementProfile(t *testing.T, repository string, created *box.Box, nono string) confine.Profile {
	t.Helper()
	return confine.Profile{
		Nono:          nono,
		Kept:          buildKept(t),
		Git:           liveGit(t),
		Box:           created.Path(),
		Read:          []string{created.Path()},
		Write:         []string{filepath.Join(created.Path(), "internal")},
		Ledger:        filepath.Join(repository, ".kept", "ledger"),
		Artifacts:     filepath.Join(repository, ".kept", "artifacts"),
		Temp:          created.Temp(),
		ToolPath:      []string{"/usr/bin", "/bin"},
		CanaryTimeout: 120 * time.Second,
	}
}

// ungrantedDir is a directory under the operator's home, because the profile
// denies the host's scratch trees by name and a box inside one is refused
// outright. See the confinement package for why that is not negotiable.
//
// It is the same root that package's own tests use rather than a second one:
// two locations would be two things to clean up and two things to explain, and
// the reason for the location is one fact about the provider's baseline.
func ungrantedDir(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("this machine has no home directory to cut an ungranted box under")
	}
	root := filepath.Join(home, ".cache", "kept-confine-test")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	// Removed only when this run left it empty, so concurrent packages do not
	// pull the directory out from under each other.
	t.Cleanup(func() { _ = os.Remove(root) })
	dir, err := os.MkdirTemp(root, "brokered-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

// scratchBox cuts a real box from a real repository, the way a repair does.
func scratchBox(t *testing.T, parent string) (string, *box.Box) {
	t.Helper()
	root, err := os.MkdirTemp(parent, "repository-")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "internal"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "internal", "subject.go"), []byte("package internal\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, root, "init", "--quiet")
	git(t, root, "config", "user.email", "test@example.com")
	git(t, root, "config", "user.name", "Test")
	git(t, root, "add", ".")
	git(t, root, "commit", "--quiet", "-m", "subject")
	commit := strings.TrimSpace(git(t, root, "rev-parse", "HEAD"))
	created, err := box.Create(context.Background(), box.Request{Repository: root, Commit: commit})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = created.Close(context.Background()) })
	return root, created
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, liveGit(t), append([]string{"-C", dir}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return string(output)
}

func liveGit(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}
	resolved, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

// buildKept is the binary the canary runs and the profile denies by name.
func buildKept(t *testing.T) string {
	t.Helper()
	workdir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(ungrantedDir(t), "kept")
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-o", path, "./cmd/kept")
	cmd.Dir = filepath.Clean(filepath.Join(workdir, "..", ".."))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build canary kept: %v\n%s", err, output)
	}
	return path
}

func liveProvider(t *testing.T) string {
	t.Helper()
	nono, err := exec.LookPath("nono")
	if err != nil {
		t.Skip("real nono provider is not installed")
	}
	resolved, err := filepath.Abs(nono)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

// liveClaude is the installed runtime at the version the adapter is pinned to.
// A different one is skipped rather than measured: a session's evidence is
// about the version that produced it.
func liveClaude(t *testing.T) string {
	t.Helper()
	executable, err := exec.LookPath("claude")
	if err != nil {
		t.Skip("Claude Code is not installed")
	}
	resolved, err := filepath.Abs(executable)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	reported, err := exec.CommandContext(ctx, resolved, "--version").Output()
	if err != nil {
		t.Skip("Claude Code did not report a version: " + err.Error())
	}
	version, _, _ := strings.Cut(strings.TrimSpace(string(reported)), " ")
	if version != thread.ClaudeVersion {
		t.Skipf("installed Claude Code is %s, the adapter is pinned to %s", version, thread.ClaudeVersion)
	}
	return resolved
}

// claudeFixtureArgv is the adapter's fixed argv, read from the fixture its own
// test pins, so a session here cannot measure a flag list nobody pinned.
func claudeArgv(t *testing.T) []string {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("..", "thread", "testdata", "claude", "fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Argv []string `json:"direct_argv"`
	}
	if err := json.Unmarshal(src, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Argv) == 0 {
		t.Fatal("the Claude fixture declares no direct argv")
	}
	return fixture.Argv
}

// claudeLivePacket is one JSONL user message. The adapter refuses to prepare an
// invocation while it is unadmitted, which is the gate working, so the wire
// shape is written here rather than taken from it.
func claudeLivePacket() []byte {
	return []byte(`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"promise:the-corpus-cannot-write-standing-or-truth is not kept in this box"}]}}` + "\n")
}

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

// testStream collects a confined session's stdout.
type testStream struct {
	mu      sync.Mutex
	builder strings.Builder
}

func (s *testStream) Write(src []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.builder.Write(src)
}

func (s *testStream) Close() error { return nil }

func (s *testStream) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.builder.String()
}
