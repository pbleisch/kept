//go:build unix

package gather

import (
	"os/exec"
	"syscall"
)

// setGroup makes the child lead its own process group, so the whole tree it
// spawns can be killed together. A gatherer that shells out leaves one behind
// otherwise. See docs/decision/0026.
func setGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killGroup kills the group the child leads. The negative pid is what makes it
// the group rather than the one process.
func killGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		// The group may already be gone, which is the outcome asked for.
		return cmd.Process.Kill()
	}
	return nil
}
