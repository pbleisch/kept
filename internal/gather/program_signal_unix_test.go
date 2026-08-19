//go:build unix

package gather

import (
	"errors"
	"os"
	"syscall"
	"testing"
)

func signalSelf() { _ = syscall.Kill(os.Getpid(), syscall.SIGKILL) }

// A program that did not choose its own exit did not finish saying what it saw,
// so reading its partial output as evidence would be recording what the machine
// thinks it saw. It is a failure and not a measurement, and it is told apart
// from a timeout because docs/decision/0026 requires the two to be
// distinguishable from the record even though both are unknown.
func TestAProgramKilledByAnotherHandIsNotAMeasurement(t *testing.T) {
	fields, err := (command{}).Gather(t.Context(), request(t, map[string]any{
		argRun:    runLine(t, "selfkill"),
		argFields: map[string]any{"out": "stdout", "ok": "exit == 0"},
	}))
	if fields != nil {
		t.Fatalf("a killed program was read as %#v", fields)
	}
	var killed *Signalled
	if !errors.As(err, &killed) {
		t.Fatalf("a killed program failed as %#v", err)
	}
	var timeout *TimedOut
	if errors.As(err, &timeout) {
		t.Error("a program somebody else killed is reported as a deadline kept did not reach")
	}
}
