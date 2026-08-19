package gather

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/model"
)

func world(tree string) model.World {
	return model.World{Tree: "sha256:" + strings.Repeat(tree[:1], 64)}
}

// The most important behaviour in this package: a gatherer that cannot find its
// toolchain reports that nobody could measure, and never that the promise is
// broken. See AGENTS.md's invariant and docs/decision/0004.
func TestAMissingToolchainIsUnknownAndNeverNotKept(t *testing.T) {
	req := request(t, map[string]any{
		argRun:    "kept-no-such-toolchain-anywhere --version",
		argFields: map[string]any{"ok": "exit == 0"},
	})

	obs := NewRunner(Builtin()).Gather(t.Context(), model.GatherSpec{Gatherer: Command}, req)
	if obs.Err == nil {
		t.Fatalf("a missing toolchain produced an observation: %#v", obs.Data)
	}
	var missing *NotRunnable
	if !errors.As(obs.Err, &missing) {
		t.Fatalf("the failure is %#v, and nothing can tell it from a program that ran", obs.Err)
	}
	if obs.Data != nil {
		t.Errorf("a failed gather left data behind: %#v", obs.Data)
	}

	// And the verdict that follows is unknown. A false field here would be a
	// promise reported broken because a machine was not installed.
	if got := verdict(t, "check", obs, "obs.check.ok"); got != model.Unknown {
		t.Fatalf("the verdict is %s, and the toolchain was never there to measure with", got)
	}
}

// A timeout and a failure both yield unknown, and an operator who cannot tell
// them apart cannot fix either. See docs/decision/0026.
func TestATimeoutIsRecordedAsATimeoutRatherThanAFailure(t *testing.T) {
	req := request(t, map[string]any{
		argRun:    runLine(t, "sleep", "60000"),
		argFields: map[string]any{"ok": "exit == 0"},
	})
	req.Deadline = 100 * time.Millisecond

	start := time.Now()
	_, err := command{}.Gather(t.Context(), req)

	var timeout *TimedOut
	if !errors.As(err, &timeout) {
		t.Fatalf("a program that outran its deadline failed as %#v", err)
	}
	if timeout.After != req.Deadline {
		t.Errorf("the timeout says %s and the deadline was %s", timeout.After, req.Deadline)
	}
	if elapsed := time.Since(start); elapsed > 20*time.Second {
		t.Fatalf("the deadline took %s to fire, so it is not what stopped the program", elapsed)
	}
}

// Nothing external runs without a deadline, so a request that names none is
// bounded anyway. Unset is not unbounded. See docs/decision/0026.
func TestARequestWithNoDeadlineIsStillBounded(t *testing.T) {
	if (Request{}).deadline() != DefaultDeadline {
		t.Fatalf("a request with no deadline resolves to %s, and waiting has no default", (Request{}).deadline())
	}
	if (Request{Deadline: -time.Second}).deadline() != DefaultDeadline {
		t.Error("a negative deadline is not a shorter one; it is one nobody set")
	}
}

// The raw bound fires before an extractor reads anything, so a runaway program
// cannot exhaust memory before any other layer runs. It is a gather failure, so
// the verdict is unknown — a cap reporting not-kept would be the confusion the
// three-valued verdict exists to prevent. See docs/decision/0023 and 0034.
func TestAProgramThatFloodsAStreamIsRefusedAndKilled(t *testing.T) {
	for _, stream := range []string{"stdout", "stderr"} {
		t.Run(stream, func(t *testing.T) {
			req := request(t, map[string]any{
				argRun:    runLine(t, "flood", stream),
				argFields: map[string]any{"ok": "exit == 0"},
			})
			req.Deadline = 30 * time.Second

			start := time.Now()
			fields, err := command{}.Gather(t.Context(), req)
			if fields != nil {
				t.Fatalf("a flooded gather returned fields: %#v", fields)
			}
			var large *ledger.TooLarge
			if !errors.As(err, &large) {
				t.Fatalf("flooding %s failed as %#v", stream, err)
			}
			if large.Bound != stream {
				t.Errorf("the refusal names %q, so a receipt cannot say which bound was exceeded", large.Bound)
			}
			if elapsed := time.Since(start); elapsed > 20*time.Second {
				t.Errorf("the program ran for %s, so the bound did not stop it", elapsed)
			}
		})
	}
}

// A non-zero exit is a measurement, not a failure. `go test` exits 1 when a
// test fails, and reading that as a gather failure would turn every broken
// promise into an unknown.
func TestANonZeroExitIsAMeasurement(t *testing.T) {
	req := request(t, map[string]any{
		argRun:    runLine(t, "exit", "3"),
		argFields: map[string]any{"code": "exit", "ok": "exit == 0"},
	})

	fields, err := command{}.Gather(t.Context(), req)
	if err != nil {
		t.Fatalf("a program that exited 3 was read as a failure: %v", err)
	}
	if fields["code"] != int64(3) {
		t.Errorf("exit is %#v, want 3", fields["code"])
	}
	if fields["ok"] != false {
		t.Errorf("ok is %#v, and the program exited 3", fields["ok"])
	}
}

// There is no shell, so a run: line asking for one is refused by name rather
// than passed through as a literal argument nobody meant.
func TestARunLineAskingForAShellIsRefused(t *testing.T) {
	for _, line := range []string{
		"go build ./... && go vet ./...",
		"go test -json ./... | tee out.log",
		"printf $HOME",
		"go test ./... > out.log",
	} {
		if _, err := argv(line); err == nil {
			t.Errorf("%q parsed, and there is no shell to run it", line)
		} else {
			var shell *Shell
			if !errors.As(err, &shell) {
				t.Errorf("%q was refused as %v, which does not say why", line, err)
			}
		}
	}
}

// Quoting is the small POSIX subset, because a run: line binds and the reading
// of one must not change.
func TestARunLineIsSplitTheWayItWasWritten(t *testing.T) {
	for _, c := range []struct {
		line string
		want []string
	}{
		{`go test -json ./...`, []string{"go", "test", "-json", "./..."}},
		{`  go   build   ./...  `, []string{"go", "build", "./..."}},
		{`grep 'a|b' file`, []string{"grep", "a|b", "file"}},
		{`say "hello there"`, []string{"say", "hello there"}},
		{`say "a\"b"`, []string{"say", `a"b`}},
		{`say a\ b`, []string{"say", "a b"}},
		{`say ''`, []string{"say", ""}},
	} {
		got, err := argv(c.line)
		if err != nil {
			t.Errorf("%q: %v", c.line, err)
			continue
		}
		if len(got) != len(c.want) {
			t.Errorf("%q split into %q, want %q", c.line, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%q split into %q, want %q", c.line, got, c.want)
				break
			}
		}
	}
}

func TestARunLineThatNamesNoProgramIsRefused(t *testing.T) {
	for _, line := range []string{"", "   ", `"unclosed`, `'unclosed`} {
		if _, err := argv(line); err == nil {
			t.Errorf("%q parsed as a program", line)
		}
	}
}

// A caller cancelling the tick is not a timeout, and neither is a measurement.
func TestACancelledContextLeavesNoObservation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	req := request(t, map[string]any{
		argRun:    runLine(t, "print", "hello"),
		argFields: map[string]any{"ok": "exit == 0"},
	})
	if _, err := (command{}).Gather(ctx, req); err == nil {
		t.Fatal("a cancelled gather produced a measurement")
	}
}
