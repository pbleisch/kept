//go:build !windows

package confine

import (
	"bytes"
	"context"
	"fmt"
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
	stderr := &boundedBuffer{limit: 1 << 10}
	cmd.Stderr = stderr
	cmd.WaitDelay = bound
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_ = write.Close()
	c := &child{
		cmd:     cmd,
		cleanup: func() {},
		stderr:  stderr,
		drain:   startDrain(read, output, bound),
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
