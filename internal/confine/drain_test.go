//go:build !windows

package confine

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/thread"
)

// burst is a child that writes more than a pipe holds, as fast as it can, and
// exits 0. It is the shape a runtime has at the end of a session: the last
// events are written and the process is gone before anybody has read them.
const burst = `awk 'BEGIN { for (i = 0; i < 4000; i++) printf "%06d %0192d\n", i, 0 }'`

// burstLines and burstBytes are what [burst] writes.
const (
	burstLines = 4000
	burstBytes = burstLines * 200
)

// slowStream is an adapter's stream that is slower than the child writing into
// it, which is what every real one is: each line it takes is a ledger append.
type slowStream struct {
	mu    sync.Mutex
	got   bytes.Buffer
	delay time.Duration
}

func (s *slowStream) Write(src []byte) (int, error) {
	time.Sleep(s.delay)
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.got.Write(src)
}

func (s *slowStream) Close() error { return nil }

func (s *slowStream) bytes() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.got.Bytes()...)
}

// TestAConfinedChildsLastOutputArrivesAfterItExits is the loss at the end of a
// session, measured through the real provider.
//
// The child fills the pipe and exits while the stream is still busy with what it
// read before. Everything the child wrote is in the pipe at that moment and
// nothing else holds it, so a launch that stops reading when the child is reaped
// loses the tail — and it tears the last line it did read, because a read ends
// where the buffer ends rather than where a line does. Measured before the drain
// waited for the end of the stream: 36 KB of 626 KB never arrived and the last
// line ended mid-record.
func TestAConfinedChildsLastOutputArrivesAfterItExits(t *testing.T) {
	launch := liveNono(t)
	stream := &slowStream{delay: 20 * time.Millisecond}
	child, err := launch.Start(context.Background(), thread.Invocation{
		Path:   "/bin/sh",
		Args:   []string{"-c", burst},
		Env:    []string{"RUNTIME_TOKEN=only-this-credential"},
		Output: stream,
	})
	if err != nil {
		t.Fatal(err)
	}
	exit := child.Wait()
	if exit.Code == nil || *exit.Code != 0 {
		t.Fatalf("the burst exited %#v", exit)
	}
	assertWholeBurst(t, stream.bytes())
}

// TestTheDrainOutlivesTheChildAndReadsToTheEndOfTheStream is the same loss
// without the provider in front of it, so it runs everywhere, and it runs
// through [child.Wait] itself rather than a copy of its logic.
func TestTheDrainOutlivesTheChildAndReadsToTheEndOfTheStream(t *testing.T) {
	stream := &slowStream{delay: 20 * time.Millisecond}
	child := plainChild(t, burst, stream, 10*time.Second)
	exit := child.Wait()
	if exit.Code == nil || *exit.Code != 0 {
		t.Fatalf("the burst exited %#v", exit)
	}
	assertWholeBurst(t, stream.bytes())
}

// TestADescendantHoldingTheStreamDoesNotHoldTheReap is the bound. Reading to the
// end of the stream is right until something outlives the child with the write
// end still open, and then the end never comes: a process that escaped the
// group, or one the kill has not reached yet. The descendant here holds stderr
// as well as stdout, and exec's own wait on stderr is the half that hung for
// the descendant's whole life until WaitDelay bounded it too. Nothing external
// runs without a deadline, and this is not an exception.
func TestADescendantHoldingTheStreamDoesNotHoldTheReap(t *testing.T) {
	pidFile := t.TempDir() + "/holder.pid"
	script := fmt.Sprintf(`printf '%%s\n' before; sleep 30 & printf '%%s' "$!" > %q; exit 0`, pidFile)
	stream := &slowStream{}
	const bound = 300 * time.Millisecond
	child := plainChild(t, script, stream, bound)

	started := time.Now()
	exit := child.Wait()
	elapsed := time.Since(started)
	if pid, err := os.ReadFile(pidFile); err == nil {
		var holder int
		if _, err := fmt.Sscanf(string(pid), "%d", &holder); err == nil {
			_ = syscall.Kill(holder, syscall.SIGKILL)
		}
	}
	if exit.Code == nil || *exit.Code != 0 {
		t.Fatalf("the child exited %#v", exit)
	}
	if elapsed > 10*time.Second {
		t.Fatalf("the reap waited %s on a stream a descendant still held open", elapsed)
	}
	if got := string(stream.bytes()); got != "before\n" {
		t.Fatalf("what the child wrote before it exited did not arrive: %q", got)
	}
}

// plainChild launches a script the way [Nono.start] launches the provider, with
// no provider: its own process group, and stdout on a pipe kept reads.
func plainChild(t *testing.T, script string, output *slowStream, bound time.Duration) *child {
	t.Helper()
	cmd := exec.Command("/bin/sh", "-c", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stdout = write
	// Stderr as the launch wires it: a writer exec copies into, which Wait waits
	// on, and which a descendant can hold open exactly as it can stdout.
	stderr, tail := &boundedBuffer{limit: 1 << 10}, &tailBuffer{limit: stderrTailLimit}
	cmd.Stderr = io.MultiWriter(stderr, tail)
	cmd.WaitDelay = bound
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_ = write.Close()
	c := &child{
		cmd:        cmd,
		cleanup:    func() {},
		stderr:     stderr,
		stderrTail: tail,
		drain:      startDrain(read, output, bound),
	}
	t.Cleanup(func() { _ = killProcessGroup(cmd.Process.Pid) })
	return c
}

func assertWholeBurst(t *testing.T, got []byte) {
	t.Helper()
	if len(got) != burstBytes {
		t.Fatalf("the stream received %d of the %d bytes the child wrote before it exited", len(got), burstBytes)
	}
	if !bytes.HasSuffix(got, []byte("\n")) {
		t.Fatalf("the stream ends inside a line: %q", got[len(got)-min(len(got), 40):])
	}
	lines := strings.Split(strings.TrimSuffix(string(got), "\n"), "\n")
	if len(lines) != burstLines || !strings.HasPrefix(lines[len(lines)-1], fmt.Sprintf("%06d ", burstLines-1)) {
		t.Fatalf("the stream received %d lines ending %q, want %d", len(lines), lines[len(lines)-1][:7], burstLines)
	}
}

// noisyExit writes a session's worth of warnings on stderr, then the one line
// that says why it stopped, and exits 1.
const noisyExit = `awk 'BEGIN { for (i = 0; i < 400; i++) printf "warning %06d: %0100d\\n", i, 0 > "/dev/stderr" }'; echo "fatal: the reason it stopped" >&2; exit 1`

// TestTheEndOfAnErrorStreamIsWhatIsKept is the retention rule on its own: a
// runtime says why it stopped in its last lines, so those are what is kept,
// and what fell off the front is counted rather than silently gone.
func TestTheEndOfAnErrorStreamIsWhatIsKept(t *testing.T) {
	tail := &tailBuffer{limit: 64}
	var written int64
	for i := range 50 {
		line := fmt.Sprintf("line %02d of what came before\n", i)
		_, _ = tail.Write([]byte(line))
		written += int64(len(line))
	}
	last := "the reason it stopped\n"
	_, _ = tail.Write([]byte(last))
	written += int64(len(last))

	kept, omitted := tail.tail()
	if !bytes.HasSuffix(kept, []byte(last)) || len(kept) != 64 {
		t.Fatalf("kept %q, want the last 64 bytes ending in the last line", kept)
	}
	if omitted != written-64 {
		t.Fatalf("omitted %d, want %d", omitted, written-64)
	}
}

// TestAChildsLastWordsOnStderrAreKept runs through child.Diagnostics, the way
// the supervisor reads it.
func TestAChildsLastWordsOnStderrAreKept(t *testing.T) {
	child := plainChild(t, noisyExit, &slowStream{}, 10*time.Second)
	exit := child.Wait()
	if exit.Code == nil || *exit.Code != 1 {
		t.Fatalf("the child exited %#v", exit)
	}
	assertLastWords(t, child)
}

// TestAConfinedRuntimesLastWordsOnStderrAreKept is the same through the real
// provider, whose own diagnostics share the stream.
func TestAConfinedRuntimesLastWordsOnStderrAreKept(t *testing.T) {
	launch := liveNono(t)
	started, err := launch.Start(context.Background(), thread.Invocation{
		Path: "/bin/sh",
		Args: []string{"-c", noisyExit},
		Env:  []string{"RUNTIME_TOKEN=only-this-credential"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if exit := started.Wait(); exit.Code == nil || *exit.Code != 1 {
		t.Fatalf("the confined child exited %#v", exit)
	}
	assertLastWords(t, started)
}

func assertLastWords(t *testing.T, started thread.Child) {
	t.Helper()
	diagnosed, ok := started.(thread.Diagnosed)
	if !ok {
		t.Fatal("the launch keeps nothing of the runtime's error stream")
	}
	kept, omitted := diagnosed.Diagnostics()
	if !bytes.HasSuffix(bytes.TrimSpace(kept), []byte("fatal: the reason it stopped")) {
		t.Fatalf("the kept end of stderr does not end with why the runtime stopped: %q", kept[max(0, len(kept)-80):])
	}
	if omitted == 0 || len(kept) > stderrTailLimit {
		t.Fatalf("kept %d bytes and omitted %d, want a bounded end and a count of what came before", len(kept), omitted)
	}
}
