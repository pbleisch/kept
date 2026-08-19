//go:build !unix

package gather

import "os/exec"

// setGroup where there are no process groups to set. The deadline still fires
// and the process kept started is still killed; what is not guaranteed is that
// a grandchild goes with it, which is why cmd.WaitDelay is not optional here.
func setGroup(*exec.Cmd) {}

func killGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
