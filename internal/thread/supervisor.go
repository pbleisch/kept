package thread

import (
	"context"
	"fmt"
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
func (s Supervisor) run(ctx context.Context, launch ConfinementLaunch, invocation Invocation, receipt *Receipt) error {
	if s.Timeout <= 0 {
		return &Refusal{Reason: "runtime deadline is unset"}
	}

	deadline, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()
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
		outputErr := closeOutput(invocation)
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
		outputErr := closeOutput(invocation)
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
// prepare a runtime invocation. Today every static adapter refuses as
// unadmitted, so Start is unreachable; the seam is here for the adapter's live
// fixture to exercise without giving it a provider profile or lifecycle writer.
func Dispatch(ctx context.Context, cfg RuntimeConfig, packet Packet, launch ConfinementLaunch, supervisor Supervisor, receipt *Receipt) error {
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
	adapterReceipt, err := receipt.ForAdapter(adapter.Runtime(), adapter.AdapterVersion())
	if err != nil {
		return err
	}
	invocation, err := adapter.Prepare(cfg, packet, adapterReceipt)
	if err != nil {
		return err
	}
	// Preparation is not a launch. Once an admitted adapter has made its fixed
	// argv and input channel choice, persist and receipt the exact packet bytes
	// before handing that invocation to confinement. A later corpus or ledger
	// read therefore has no opportunity to rebuild what the child receives.
	ref, err := packet.Store(receipt.ledger)
	if err != nil {
		return err
	}
	if err := receipt.RecordPacket(ref); err != nil {
		return err
	}
	return supervisor.run(ctx, launch, invocation, receipt)
}
