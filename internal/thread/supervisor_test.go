package thread

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeLaunch struct {
	verified error
	child    *fakeChild
	started  int
}

func (f *fakeLaunch) Verified() error { return f.verified }

func (f *fakeLaunch) Start(_ context.Context, _ Invocation) (Child, error) {
	f.started++
	if f.child == nil {
		return nil, errors.New("no child")
	}
	return f.child, nil
}

type fakeChild struct {
	mu       sync.Mutex
	wait     chan Exit
	killErr  error
	killCall int
}

func normalChild(code int) *fakeChild {
	ch := &fakeChild{wait: make(chan Exit, 1)}
	ch.wait <- Exit{Code: ptr(code)}
	return ch
}

func (f *fakeChild) Kill() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.killCall++
	if f.killErr != nil {
		return f.killErr
	}
	f.wait <- Exit{Signal: "SIGKILL"}
	return nil
}

func (f *fakeChild) Wait() Exit { return <-f.wait }

type fakeEventStream struct {
	closed int
	err    error
}

func (*fakeEventStream) Write(src []byte) (int, error) { return len(src), nil }

func (f *fakeEventStream) Close() error {
	f.closed++
	return f.err
}

func TestSupervisorRecordsOnlyParentObservedNormalLifecycle(t *testing.T) {
	receipt, store := testReceipt(t)
	launch := &fakeLaunch{child: normalChild(0)}
	err := (Supervisor{Timeout: time.Second}).Run(context.Background(), launch, Invocation{}, receipt)
	if err != nil {
		t.Fatal(err)
	}
	entries, quarantined, err := ReadReceipt(store, receipt.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(quarantined) != 0 || len(entries) != 2 {
		t.Fatalf("lifecycle receipt = %#v with quarantine %#v, want started and reaped exit", entries, quarantined)
	}
	if entries[0].Lifecycle.Event != Started || entries[1].Lifecycle.Event != Exited || entries[1].Lifecycle.Exit.Code == nil || *entries[1].Lifecycle.Exit.Code != 0 {
		t.Fatalf("lifecycle = %#v, %#v; want parent observed started then exit code 0", entries[0].Lifecycle, entries[1].Lifecycle)
	}
	if launch.started != 1 || launch.child.killCall != 0 {
		t.Fatalf("normal child start/kill = %d/%d", launch.started, launch.child.killCall)
	}
}

func TestSupervisorDeadlineKillsAndReapsTheWholeChildSeam(t *testing.T) {
	receipt, store := testReceipt(t)
	child := &fakeChild{wait: make(chan Exit, 1)}
	launch := &fakeLaunch{child: child}
	err := (Supervisor{Timeout: time.Millisecond}).Run(context.Background(), launch, Invocation{}, receipt)
	if err != nil {
		t.Fatal(err)
	}
	entries, quarantined, err := ReadReceipt(store, receipt.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(quarantined) != 0 || ReceiptText(entries) != "thread_lifecycle,thread_lifecycle,thread_lifecycle,thread_lifecycle" {
		t.Fatalf("timeout receipt = %q, quarantine %#v", ReceiptText(entries), quarantined)
	}
	if entries[1].Lifecycle.Event != TimedOut || entries[1].Lifecycle.Timeout != time.Millisecond.String() || entries[2].Lifecycle.Event != Killed || entries[3].Lifecycle.Event != Exited || entries[3].Lifecycle.Exit.Signal != "SIGKILL" {
		t.Fatalf("timeout lifecycle events = %#v", entries)
	}
	if child.killCall != 1 {
		t.Fatalf("timed-out child Kill calls = %d, want one process-group kill", child.killCall)
	}
}

func TestSupervisorClosesNativeStreamOnlyAfterTheChildIsReaped(t *testing.T) {
	receipt, store := testReceipt(t)
	stream := &fakeEventStream{}
	launch := &fakeLaunch{child: normalChild(0)}
	err := (Supervisor{Timeout: time.Second}).Run(context.Background(), launch, Invocation{Output: stream}, receipt)
	if err != nil {
		t.Fatal(err)
	}
	if stream.closed != 1 {
		t.Fatalf("native stream close count = %d, want one close after the child exit", stream.closed)
	}
	entries, quarantined, err := ReadReceipt(store, receipt.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(quarantined) != 0 || len(entries) != 2 || entries[1].Lifecycle.Event != Exited {
		t.Fatalf("stream close prevented recording the reaped exit: %#v, quarantine %#v", entries, quarantined)
	}
}

func TestSupervisorStillRecordsTheReapedExitWhenNativeStreamCloseFails(t *testing.T) {
	receipt, store := testReceipt(t)
	stream := &fakeEventStream{err: errors.New("bad native JSON")}
	launch := &fakeLaunch{child: normalChild(0)}
	err := (Supervisor{Timeout: time.Second}).Run(context.Background(), launch, Invocation{Output: stream}, receipt)
	if err == nil || !strings.Contains(err.Error(), "retain runtime output") {
		t.Fatalf("supervision error = %v, want the native-stream failure", err)
	}
	entries, quarantined, readErr := ReadReceipt(store, receipt.ID())
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(quarantined) != 0 || len(entries) != 2 || entries[1].Lifecycle.Event != Exited {
		t.Fatalf("stream failure hid the actual reaped exit: %#v, quarantine %#v", entries, quarantined)
	}
}

func TestSupervisorRefusesWithoutAProviderDeadlineOrPassingCanary(t *testing.T) {
	receipt, _ := testReceipt(t)
	for _, tc := range []struct {
		name       string
		launch     ConfinementLaunch
		supervisor Supervisor
		want       string
	}{
		{"unset provider", nil, Supervisor{Timeout: time.Second}, "provider is unset"},
		{"failed canary", &fakeLaunch{verified: errors.New("write escaped"), child: normalChild(0)}, Supervisor{Timeout: time.Second}, "canary failed"},
		{"unset deadline", &fakeLaunch{child: normalChild(0)}, Supervisor{}, "deadline is unset"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.supervisor.Run(context.Background(), tc.launch, Invocation{}, receipt)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Run error = %v, want refusal %q", err, tc.want)
			}
			if launch, ok := tc.launch.(*fakeLaunch); ok && launch.started != 0 {
				t.Fatal("a refused supervision started a runtime")
			}
		})
	}
}
