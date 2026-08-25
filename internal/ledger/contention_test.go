package ledger

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// cohort is the contention this package is measured under: writers concurrent
// runs of `kept` and each appending what one of them appends.
//
// One line per writer does not reproduce anything and never did — measured, all
// eight land under every schedule, because eight processes starting eight
// separate Git pipelines rarely overlap on the one update-ref between them.
// What a tick actually does is append a run's worth of records, so a writer
// here does too, and the contention is sustained rather than momentary.
const (
	cohort    = 8
	perWriter = 10
)

// contend runs `writers` concurrent appenders against one ledger ref and
// reports how many of their appends came back with an error, and how many
// lines are on the ref afterwards.
//
// Goroutines rather than processes, and that is faithful rather than
// convenient: every attempt is a real Git invocation against a real repository
// and the compare-and-swap being contended is Git's own, so what serializes
// them is the ref rather than anything in this process.
func contend(t *testing.T, retry retrySchedule, writers, each int) (failed, lines int, sample error) {
	t.Helper()
	_, work := fetchLab(t)

	var mu sync.Mutex
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := NewRefStore(work)
			s.retry = retry
			<-start
			for j := range each {
				if err := s.Append("acts.jsonl", fmt.Appendf(nil, `{"act":"w%d-%d"}`, i, j)); err != nil {
					mu.Lock()
					failed++
					if sample == nil {
						sample = err
					}
					mu.Unlock()
				}
			}
		}()
	}
	close(start)
	wg.Wait()

	content, err := NewRefStore(work).Read("acts.jsonl")
	if err != nil {
		t.Fatalf("reading the ledger back: %v", err)
	}
	return failed, len(strings.Split(strings.TrimSpace(string(content)), "\n")), sample
}

// TestEveryConcurrentWriterLandsItsLines is the acceptance. Nothing here is
// about speed: a losing writer is expected to take longer, and the claim is
// only that it eventually gets in.
func TestEveryConcurrentWriterLandsItsLines(t *testing.T) {
	failed, lines, sample := contend(t, refUpdateSchedule, cohort, perWriter)
	if failed != 0 {
		t.Errorf("%d of %d appends were abandoned; every one of them should have landed: %v", failed, cohort*perWriter, sample)
	}
	if want := cohort * perWriter; lines != want {
		t.Errorf("%d lines on the ref, want %d — a compare-and-swap retry must preserve every writer's lines", lines, want)
	}
}

// TestTheScheduleThisReplacedAbandonsWriters is the check firing. It runs the
// schedule that was there — eight attempts and no wait at all between them —
// against the same contention, and the test is that it loses lines.
//
// It is here rather than in a commit message because the number is the whole
// argument for the one above it. A retry budget that is never seen to run out
// is a budget nobody can say is big enough.
func TestTheScheduleThisReplacedAbandonsWriters(t *testing.T) {
	failed, lines, sample := contend(t, retrySchedule{Attempts: 8}, cohort, perWriter)
	if failed == 0 {
		t.Fatalf("eight attempts with no wait between them abandoned nothing under %d concurrent writers; "+
			"if that is now true, the measurement behind %+v is stale", cohort, refUpdateSchedule)
	}
	if !errors.Is(sample, ErrContended) {
		t.Errorf("an exhausted budget did not report itself as contention: %v", sample)
	}
	t.Logf("the schedule this replaced abandoned %d of %d appends and landed %d lines",
		failed, cohort*perWriter, lines)
}

// TestAnExhaustedBudgetIsNotAFailedWrite is the discrimination the longer
// schedule must not blunt: contention reports [ErrContended] and a write that
// failed for a real reason reports itself, on the first attempt.
func TestAnExhaustedBudgetIsNotAFailedWrite(t *testing.T) {
	_, work := fetchLab(t)

	// A ref cannot be created below a ref. Git refuses this one every time and
	// for a reason that has nothing to do with another writer, so a schedule
	// that retried it would be spending a budget on a certainty.
	blocked := filepath.Join(work, ".git", filepath.FromSlash(Ref), "child")
	if err := os.MkdirAll(filepath.Dir(blocked), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(blocked, []byte(strings.Repeat("0", 40)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	began := time.Now()
	err := NewRefStore(work).Append("acts.jsonl", []byte(`{"act":"blocked"}`))
	if err == nil {
		t.Fatal("a ref Git cannot create was reported as written")
	}
	if errors.Is(err, ErrContended) {
		t.Errorf("a genuine write failure was reported as contention: %v", err)
	}
	if !strings.Contains(err.Error(), "cannot create") {
		t.Errorf("the error does not carry Git's own reason: %v", err)
	}
	if took := time.Since(began); took > RefUpdateBudget {
		t.Errorf("a genuine write failure took %v, which is longer than the whole retry budget %v — it was retried", took, RefUpdateBudget)
	}
}

// TestOnlyANoticeOfAnotherWriterIsRetried holds [retryUpdate] to the two
// messages it is allowed to act on. Every string here is one Git has actually
// produced; the refusals are what keeps a longer schedule from swallowing a
// defect.
func TestOnlyANoticeOfAnotherWriterIsRetried(t *testing.T) {
	for _, c := range []struct {
		what  string
		out   string
		retry bool
	}{
		{"another writer won the compare-and-swap",
			"fatal: update_ref failed for ref 'refs/kept/ledger': cannot lock ref 'refs/kept/ledger': is at abc but expected def", true},
		{"another writer created the ref first",
			"fatal: update_ref failed for ref 'refs/kept/ledger': cannot lock ref 'refs/kept/ledger': reference already exists", true},
		{"a ref exists below this one",
			"fatal: update_ref failed for ref 'refs/kept/ledger': cannot lock ref 'refs/kept/ledger': 'refs/kept/ledger/child' exists; cannot create 'refs/kept/ledger'", false},
		{"the ref file cannot be written",
			"fatal: cannot lock ref 'refs/kept/ledger': Unable to create '.git/refs/kept/ledger.lock': Permission denied", false},
		{"the object is not there",
			"fatal: update_ref failed for ref 'refs/kept/ledger': cannot update ref 'refs/kept/ledger': trying to write ref 'refs/kept/ledger' with nonexistent object", false},
		{"the repository is gone",
			"fatal: not a git repository: '.'", false},
	} {
		got := retryUpdate(&gitExitError{args: []string{"update-ref"}, output: c.out, err: errors.New("exit status 128")})
		if got != c.retry {
			t.Errorf("%s: retried = %v, want %v", c.what, got, c.retry)
		}
	}
	if retryUpdate(errors.New("something that is not a Git exit")) {
		t.Error("an error that is not a Git exit was retried")
	}
}

// TestTheWaitIsDrawnFromADoublingWindow is the schedule's own arithmetic: the
// window doubles, it stops at the cap, and every draw falls inside it.
func TestTheWaitIsDrawnFromADoublingWindow(t *testing.T) {
	r := refUpdateSchedule
	for attempt, want := range []time.Duration{r.Floor, 2 * r.Floor, 4 * r.Floor} {
		if got := r.window(attempt, 0); got != want {
			t.Errorf("window(%d) with a free attempt = %v, want the floor doubling: %v", attempt, got, want)
		}
	}

	// The window an attempt draws from is a multiple of what that attempt
	// cost, which is the whole reason it is not a constant: the same schedule
	// has to widen on a machine where every Git invocation is slower.
	const cost = 200 * time.Millisecond
	if got, want := r.window(0, cost), time.Duration(r.Scale)*cost; got != want {
		t.Errorf("window(0) after an attempt costing %v = %v, want %v", cost, got, want)
	}
	if got, want := r.window(2, cost), 4*time.Duration(r.Scale)*cost; got != want {
		t.Errorf("window(2) after an attempt costing %v = %v, want %v", cost, got, want)
	}
	if got := r.window(0, time.Nanosecond); got != r.Floor {
		t.Errorf("an attempt that cost nothing drew from %v, want the floor %v", got, r.Floor)
	}
	if got := r.window(r.Attempts-1, 0); got != r.Cap {
		t.Errorf("the last window is %v, want the cap %v", got, r.Cap)
	}
	for attempt := range r.Attempts {
		for _, cost := range []time.Duration{0, cost} {
			window := r.window(attempt, cost)
			for range 64 {
				if wait := r.wait(attempt, cost); wait < 0 || wait >= window {
					t.Fatalf("wait(%d, %v) = %v, outside [0, %v)", attempt, cost, wait, window)
				}
			}
		}
	}

	// A schedule with no window is the one this replaced, and it must not
	// divide by zero on its way to waiting for nothing.
	if wait := (retrySchedule{Attempts: 8}).wait(0, 0); wait != 0 {
		t.Errorf("a schedule with no window waited %v", wait)
	}

	// The budget is the ceiling: every window at the cap, one fewer than there
	// are attempts because the last attempt is not followed by a wait.
	if want := time.Duration(r.Attempts-1) * r.Cap; RefUpdateBudget != want {
		t.Errorf("RefUpdateBudget is %v, want %v", RefUpdateBudget, want)
	}
}
