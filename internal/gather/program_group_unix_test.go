//go:build unix

package gather

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A killed process is killed, including its children. Abandoning the parent and
// calling it a timeout leaves a build running that will corrupt the next one's
// output — which is a failure nothing reports, because the tick that timed out
// already recorded its unknown and moved on. See docs/decision/0026.
//
// This is the test that fires when setGroup or killGroup is removed: without
// the group, the grandchild outlives the gather and is still there afterwards.
func TestAKilledGatherTakesTheChildrenItSpawned(t *testing.T) {
	pidfile := filepath.Join(t.TempDir(), "grandchild.pid")

	req := request(t, map[string]any{
		argRun:    runLine(t, "spawn", pidfile),
		argFields: map[string]any{"ok": "exit == 0"},
	})
	req.Deadline = 2 * time.Second

	_, err := (command{}).Gather(t.Context(), req)
	var timeout *TimedOut
	if !errors.As(err, &timeout) {
		t.Fatalf("the gather ended as %#v rather than a timeout", err)
	}

	pid := grandchild(t, pidfile)
	// The signal has to land before this can be asked. Signal 0 delivers
	// nothing and only reports whether the process is still there.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); err != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Do not leave it running whatever the verdict.
	_ = syscall.Kill(pid, syscall.SIGKILL)
	t.Fatalf("the gather timed out and left process %d running, so the group was never killed", pid)
}

func grandchild(t *testing.T, pidfile string) int {
	t.Helper()
	src, err := os.ReadFile(pidfile)
	if err != nil {
		t.Fatalf("the program under gather never recorded a child: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(src)))
	if err != nil || pid <= 0 {
		t.Fatalf("the recorded pid is %q", src)
	}
	return pid
}
