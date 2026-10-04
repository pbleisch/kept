//go:build unix

package thread

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/run"
)

const (
	// killChild names the repository a re-executed test binary writes a
	// receipt into, and killThread the thread it writes.
	killChild  = "KEPT_THREAD_KILL_CHILD"
	killThread = "KEPT_THREAD_KILL_THREAD"

	// killAfter is how many lines kept must have read before the parent kills
	// it. Enough that a writer holding read lines anywhere but durable storage
	// has some held when the kill lands.
	killAfter = 200

	// killChildLines stops the stand-in runtime eventually if the parent never
	// kills, so a broken test cannot fill the spool for ever.
	killChildLines = 100000
)

// TestMain re-executes this binary as the kept process a kill lands on, the way
// internal/land/crash_test.go does for a land. Everything else runs the
// ordinary suite.
func TestMain(m *testing.M) {
	if repository := os.Getenv(killChild); repository != "" {
		os.Exit(runKillChild(repository, ID(os.Getenv(killThread))))
	}
	os.Exit(m.Run())
}

// TestKeptKilledMidSessionLosesNoLineItHadRead kills kept itself while a
// runtime is streaming, and asks whether every line kept had read survived.
//
// "Read" is the moment kept accepted the line from the runtime: the spool's
// Write returning, which is when the runtime is free to forget it. The child
// reports each one on stdout only after that, so every line the parent counts
// was certainly read, and the kill — of the whole process group, with no
// chance to flush — lands with as many more read and unreported as the moment
// allows. What survived is read from the ledger after the next run has opened
// it and written one record of its own, which is where a layout that holds
// lines anywhere but the ledger is expected to recover them; never from the
// killed run's own account. See docs/decision/0018 and 0084.
func TestKeptKilledMidSessionLosesNoLineItHadRead(t *testing.T) {
	repository := receiptRepository(t)
	thread := MintID()

	child := exec.Command(os.Args[0], "-test.run=^$")
	child.Env = append(os.Environ(), killChild+"="+repository, killThread+"="+string(thread))
	child.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	child.Stderr = os.Stderr
	out, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	group := child.Process.Pid
	t.Cleanup(func() { _ = syscall.Kill(-group, syscall.SIGKILL) })

	read, killed := 0, false
	deadline := time.AfterFunc(2*time.Minute, func() { _ = syscall.Kill(-group, syscall.SIGKILL) })
	defer deadline.Stop()
	scanner := bufio.NewScanner(out)
	for scanner.Scan() {
		n, ok := strings.CutPrefix(scanner.Text(), "read ")
		if !ok {
			continue
		}
		k, err := strconv.Atoi(n)
		if err != nil {
			t.Fatalf("child reported %q", scanner.Text())
		}
		read = max(read, k)
		// Mid-session means the receipt already exists in the ledger: a kill
		// before its first line is a kill of a session that had not started
		// recording, which proves less.
		if !killed && read >= killAfter && receiptExists(repository, thread) {
			if err := syscall.Kill(-group, syscall.SIGKILL); err != nil {
				t.Fatal(err)
			}
			killed = true
		}
	}
	_ = child.Wait()
	if !killed {
		t.Fatalf("the child stopped after reading %d lines, before the kill", read)
	}

	// The next run: open the ledger and write one record, as any writing verb
	// does. Nothing here reads the killed run's memory, because it has none.
	store := ledger.NewRefStore(repository)
	next := ledger.Open(store, run.Start("test", nil))
	other, err := NewReceipt(next, MintID())
	if err != nil {
		t.Fatal(err)
	}
	if err := other.Started(); err != nil {
		t.Fatal(err)
	}
	if err := next.Close("0"); err != nil {
		t.Fatal(err)
	}

	entries, quarantined, err := ReadReceipt(store, thread)
	if err != nil && !errors.Is(err, ledger.ErrNoStream) {
		t.Fatalf("read the killed run's receipt: %v", err)
	}
	if len(quarantined) != 0 {
		t.Fatalf("the killed run's receipt quarantined %d lines: %#v", len(quarantined), quarantined)
	}
	want := sessionTypes(t)
	var got []string
	for _, entry := range entries {
		if entry.Native != nil {
			got = append(got, entry.Native.Type)
		}
	}
	if len(got) < read {
		t.Fatalf("kept had read %d lines when it was killed, and the ledger holds %d of them", read, len(got))
	}
	for i := range read {
		if expect := want[i%len(want)]; got[i] != expect {
			t.Fatalf("native event %d is %q, want %q: the lines survived out of order", i+1, got[i], expect)
		}
	}
}

// receiptExists is whether the thread's receipt has reached the ledger's ref.
func receiptExists(repository string, thread ID) bool {
	where, err := ledger.ThreadPath(string(thread))
	if err != nil {
		return false
	}
	_, err = receiptGitErr(repository, nil, "cat-file", "-e", ledger.Ref+":ledger/"+where)
	return err == nil
}

// sessionTypes is the native type of each line of the captured session, in
// order.
func sessionTypes(t *testing.T) []string {
	t.Helper()
	var types []string
	for line := range bytes.SplitSeq(bytes.TrimSpace(capturedSession(t)), []byte{'\n'}) {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(line, &envelope); err != nil {
			t.Fatal(err)
		}
		types = append(types, envelope.Type)
	}
	return types
}

// runKillChild is the kept process the parent kills. It writes a receipt the
// way a repair does — the supervisor, its spool, the Claude adapter's stream,
// a ledger on a real ref — fed by a stand-in runtime that streams the captured
// session for as long as it is let.
func runKillChild(repository string, thread ID) int {
	fail := func(err error) int {
		fmt.Fprintln(os.Stderr, "kill child:", err)
		return 2
	}
	store := ledger.NewRefStore(repository)
	receipt, err := NewReceipt(ledger.Open(store, run.Start("kill-child", nil)), thread)
	if err != nil {
		return fail(err)
	}
	adapter, err := receipt.ForAdapter(Claude, "replay")
	if err != nil {
		return fail(err)
	}
	src, err := os.ReadFile("testdata/claude/session.jsonl")
	if err != nil {
		return fail(err)
	}
	launch := &streamingLaunch{lines: bytes.SplitAfter(bytes.TrimSpace(src), []byte{'\n'})}
	invocation := Invocation{Output: &claudeStream{receipt: adapter, secret: []byte(replaySecret)}}
	if err := (Supervisor{Timeout: time.Hour}).Run(context.Background(), launch, invocation, receipt); err != nil {
		return fail(err)
	}
	return 0
}

// streamingLaunch is a runtime that writes the captured session into what the
// supervisor hands it, line by line, and reports each line on stdout once kept
// has accepted it. It never exits on its own; the parent's kill ends it.
type streamingLaunch struct {
	lines [][]byte
}

func (*streamingLaunch) Verified() error { return nil }

func (l *streamingLaunch) Start(_ context.Context, invocation Invocation) (Child, error) {
	go func() {
		for i := 1; i <= killChildLines; i++ {
			line := l.lines[(i-1)%len(l.lines)]
			if !bytes.HasSuffix(line, []byte{'\n'}) {
				line = append(append([]byte(nil), line...), '\n')
			}
			if _, err := invocation.Output.Write(line); err != nil {
				fmt.Fprintln(os.Stderr, "kill child: stream:", err)
				return
			}
			if _, err := io.WriteString(os.Stdout, "read "+strconv.Itoa(i)+"\n"); err != nil {
				return
			}
		}
	}()
	return foreverChild{}, nil
}

// foreverChild is a runtime process nothing reaps but the parent's kill.
type foreverChild struct{}

func (foreverChild) Kill() error { return nil }
func (foreverChild) Wait() Exit  { select {} }
