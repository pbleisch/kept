package thread

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Child is the confined child process the parent owns. Kill must terminate the
// complete process group the confinement launch created; an adapter is never
// handed this capability.
type Child interface {
	Kill() error
	Wait() Exit
}

// ConfinementLaunch is supplied by the already selected and canary-verified
// confinement provider. Its profile, canary and resource policy remain outside
// adapters: an adapter provides argv only after it is admitted.
type ConfinementLaunch interface {
	Verified() error
	Start(context.Context, Invocation) (Child, error)
}

// Supervisor holds the parent-owned timeout reaction. Timeout has no default:
// a caller must configure it before an external runtime can start.
type Supervisor struct {
	Timeout time.Duration
}

// Run starts a confined child, appends only events the parent observed, and
// reaps it. A deadline produces timed_out, then killed only after Kill succeeds,
// then the actual reaped exit; a crash between any two leaves the honest partial
// receipt decisions 0020 and 0026 require.
func (s Supervisor) Run(ctx context.Context, launch ConfinementLaunch, invocation Invocation, receipt *Receipt) error {
	if receipt == nil {
		return &Refusal{Reason: "thread receipt is absent"}
	}
	if launch == nil {
		return &Refusal{Reason: "confinement provider is unset"}
	}
	if err := launch.Verified(); err != nil {
		return &Refusal{Reason: "confinement canary failed", Detail: err.Error()}
	}
	return s.run(ctx, launch, invocation, receipt)
}

// run starts after the parent has verified confinement. Dispatch uses this
// after its own pre-adapter verification so a canary is never run twice merely
// because the runtime boundary is present.
//
// Whatever way the session ends, the receipt is sealed before this returns, so
// a caller reading it next — a diagnosis of a launch that failed included —
// finds everything it journaled. See docs/decision/0087.
func (s Supervisor) run(ctx context.Context, launch ConfinementLaunch, invocation Invocation, receipt *Receipt) error {
	err := s.supervise(ctx, launch, invocation, receipt)
	if sealErr := receipt.Seal(); sealErr != nil {
		if err == nil {
			return fmt.Errorf("seal runtime receipt: %w", sealErr)
		}
		return fmt.Errorf("%w; seal runtime receipt: %v", err, sealErr)
	}
	return err
}

func (s Supervisor) supervise(ctx context.Context, launch ConfinementLaunch, invocation Invocation, receipt *Receipt) error {
	if s.Timeout <= 0 {
		return &Refusal{Reason: "runtime deadline is unset"}
	}

	deadline, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()
	// The launch hands the runtime's output to the adapter directly. Each line
	// the adapter accepts is journaled by the time its Write returns, which is
	// an append to a file and keeps pace with the runtime on its own; nothing
	// is held in memory between the two. See docs/decision/0087, which retires
	// the spool docs/decision/0084 put here.
	if invocation.Output != nil {
		invocation.Output = &draining{dst: invocation.Output}
	}
	child, err := launch.Start(deadline, invocation)
	if err != nil {
		return err
	}
	if err := receipt.Started(); err != nil {
		// The child is real even when its first receipt append failed. Do not
		// turn a ledger outage into an unbounded external process.
		if killErr := child.Kill(); killErr != nil {
			return fmt.Errorf("record started runtime: %w; kill unrecorded runtime: %v", err, killErr)
		}
		// A successful group kill is not yet a reaped process. Wait even though
		// this receipt can no longer record the exit, then close the native
		// stream so the confinement launch does not retain its pipe.
		child.Wait()
		if outputErr := closeOutput(invocation); outputErr != nil {
			return fmt.Errorf("record started runtime: %w; close unrecorded runtime stream: %v", err, outputErr)
		}
		return fmt.Errorf("record started runtime: %w", err)
	}

	exited := make(chan Exit, 1)
	go func() { exited <- child.Wait() }()

	select {
	case exit := <-exited:
		outputErr := errors.Join(closeOutput(invocation), recordStderr(child, invocation, receipt), recordExecutableAtExit(invocation, receipt))
		exitErr := receipt.Exited(exit)
		return supervisedResult(outputErr, exitErr)
	case <-deadline.Done():
		if deadline.Err() != context.DeadlineExceeded {
			return fmt.Errorf("runtime supervision canceled before its deadline: %w", deadline.Err())
		}
		timeoutErr := receipt.TimedOut(s.Timeout)
		if err := child.Kill(); err != nil {
			if timeoutErr != nil {
				return fmt.Errorf("record timeout: %w; kill timed-out runtime: %v", timeoutErr, err)
			}
			return fmt.Errorf("kill timed-out runtime: %w", err)
		}
		killedErr := receipt.Killed()
		exit := <-exited
		outputErr := errors.Join(closeOutput(invocation), recordStderr(child, invocation, receipt), recordExecutableAtExit(invocation, receipt))
		exitErr := receipt.Exited(exit)
		if timeoutErr != nil {
			return fmt.Errorf("record timeout: %w", timeoutErr)
		}
		if killedErr != nil {
			return fmt.Errorf("record killed runtime: %w", killedErr)
		}
		return supervisedResult(outputErr, exitErr)
	}
}

// recordStderr retains the end of the runtime's error stream, once the process
// is reaped and before the parent records the exit, so the receipt still ends
// at the exit. It records nothing from a launch that kept none, and nothing an
// adapter has not redacted. See docs/decision/0085.
func recordStderr(child Child, invocation Invocation, receipt *Receipt) error {
	diagnosed, ok := child.(Diagnosed)
	if !ok || invocation.Redact == nil {
		return nil
	}
	tail, omitted := diagnosed.Diagnostics()
	if len(tail) == 0 {
		return nil
	}
	redacted, redaction := invocation.Redact(tail)
	return receipt.RecordStderr(redacted, redaction, omitted)
}

// recordExecutableAtExit looks at the runtime's executable again once the
// process is reaped, and before the exit is recorded, so the receipt still ends
// at the exit. A launch record is what makes the second look a comparison; an
// invocation that names no path has nothing to look at. See docs/decision/0085.
func recordExecutableAtExit(invocation Invocation, receipt *Receipt) error {
	if invocation.Path == "" {
		return nil
	}
	receipt.mu.Lock()
	_, launched := receipt.executables[AtLaunch]
	receipt.mu.Unlock()
	if !launched {
		return nil
	}
	return receipt.RecordExecutable(IdentifyExecutable(invocation.Path, AtExit, ""))
}

// errOutputClosed is a write after the runtime's stream was closed.
var errOutputClosed = errors.New("write runtime output after its stream was closed")

// draining hands a runtime's output straight to the adapter's stream, and once
// a write fails it accepts and discards the rest, so a receipt that can no
// longer be written never wedges the runtime: the launch stops reading at the
// first error it is handed, and a runtime nobody reads waits or discards. The
// failure is what Close reports. It holds nothing; that is the half of
// docs/decision/0084's spool that docs/decision/0087 keeps.
type draining struct {
	dst EventStream

	mu     sync.Mutex
	failed error
	closed bool
	result error
}

func (d *draining) Write(src []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return 0, errOutputClosed
	}
	if d.failed == nil {
		if _, err := d.dst.Write(src); err != nil {
			d.failed = err
		}
	}
	return len(src), nil
}

// Close closes the adapter's stream and reports the first failure of either.
// It is idempotent.
func (d *draining) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return d.result
	}
	d.closed = true
	closeErr := d.dst.Close()
	switch {
	case d.failed != nil && closeErr != nil:
		d.result = fmt.Errorf("%w; close: %v", d.failed, closeErr)
	case d.failed != nil:
		d.result = d.failed
	default:
		d.result = closeErr
	}
	return d.result
}

func closeOutput(invocation Invocation) error {
	if invocation.Output == nil {
		return nil
	}
	return invocation.Output.Close()
}

func supervisedResult(outputErr, exitErr error) error {
	if outputErr != nil && exitErr != nil {
		return fmt.Errorf("retain runtime output: %w; record reaped exit: %v", outputErr, exitErr)
	}
	if outputErr != nil {
		return fmt.Errorf("retain runtime output: %w", outputErr)
	}
	return exitErr
}

// Dispatch is the small boundary that proves confinement before an adapter can
// prepare a runtime invocation. Confinement is verified first and unconditionally:
// a failed canary refuses here, before any adapter is asked for argv and before
// a packet is stored, so a refused dispatch leaves no thread receipt at all
// rather than a lifecycle for a runtime that never ran.
func Dispatch(ctx context.Context, cfg RuntimeConfig, input Input, launch ConfinementLaunch, supervisor Supervisor, receipt *Receipt) error {
	if receipt == nil {
		return &Refusal{Reason: "thread receipt is absent"}
	}
	if launch == nil {
		return &Refusal{Reason: "confinement provider is unset"}
	}
	if err := launch.Verified(); err != nil {
		return &Refusal{Reason: "confinement canary failed", Detail: err.Error()}
	}
	adapter, err := Select(cfg.Runtime)
	if err != nil {
		return err
	}
	adapterReceipt, err := receipt.ForAdapter(adapter.Runtime(), adapter.AdapterVersion(cfg))
	if err != nil {
		return err
	}
	invocation, err := adapter.Prepare(cfg, input, adapterReceipt)
	if err != nil {
		return err
	}
	// Preparation is not a launch. Once an admitted adapter has made its fixed
	// argv and input channel choice, persist and receipt the exact bytes before
	// handing that invocation to confinement. A later corpus or ledger read
	// therefore has no opportunity to rebuild what the child receives.
	//
	// Which record kind that is belongs to the input rather than to a choice
	// made here: a packet files a packet and a brief files a brief, so nothing
	// at this seam can record one kind of session as the other.
	ref, err := input.Store(receipt.ledger)
	if err != nil {
		return err
	}
	if err := input.record(receipt, ref); err != nil {
		return err
	}
	// The executable, looked at before it is handed to confinement and again
	// once the process is reaped. The two records are what say whether the
	// binary a session ran was the one it was admitted as; a package manager
	// replacing it underneath a running session is otherwise an inference from
	// timestamps nobody wrote down. See docs/decision/0085.
	if err := receipt.RecordExecutable(IdentifyExecutable(invocation.Path, AtLaunch, cfg.Version)); err != nil {
		return err
	}
	return supervisor.run(ctx, launch, invocation, receipt)
}
