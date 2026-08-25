package main

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/ledger"
)

// TestLosingTheLedgerIsNotAUsageError is the exit code being honest. A run that
// spent its compare-and-swap budget against other runs exited 2 — the code that
// means the operator asked for something wrong — and 2 is now reserved for
// exactly that.
func TestLosingTheLedgerIsNotAUsageError(t *testing.T) {
	// Shaped the way internal/ledger shapes it: the sentinel and the Git error
	// underneath it, both wrapped, so that a caller can read either.
	git := errors.New("git update-ref: exit status 128: cannot lock ref 'refs/kept/ledger': is at abc but expected def")
	contended := fmt.Errorf("update %s after %d attempts: %w: %w", ledger.Ref, ledger.RefUpdateAttempts, ledger.ErrContended, git)

	var stderr bytes.Buffer
	if code := unrecorded(&stderr, contended); code != exitUnknown {
		t.Errorf("a contended ledger write exited %d, want %d", code, exitUnknown)
	}
	if !strings.Contains(stderr.String(), "run it again") && !strings.Contains(stderr.String(), "Run it again") {
		t.Errorf("nothing told the operator what to do about it:\n%s", stderr.String())
	}
	if !errors.Is(contended, ledger.ErrContended) {
		t.Error("the error kept builds for an exhausted budget does not read as contention")
	}
}

// TestAGenuineWriteFailureStaysAUsageError is the other half. A broken ref, a
// permission problem or a torn line is not a race, and a longer retry schedule
// must not make it look like one.
func TestAGenuineWriteFailureStaysAUsageError(t *testing.T) {
	for _, err := range []error{
		errors.New("git update-ref: cannot lock ref 'refs/kept/ledger': 'refs/kept/ledger/child' exists; cannot create 'refs/kept/ledger'"),
		errors.New("git hash-object: exit status 128: fatal: unable to write object"),
		fmt.Errorf("reading the ledger: %w", ledger.ErrNoStream),
	} {
		var stderr bytes.Buffer
		if code := unrecorded(&stderr, err); code != exitUsage {
			t.Errorf("%v exited %d, want %d", err, code, exitUsage)
		}
		if stderr.Len() != 0 {
			t.Errorf("a genuine failure was given contention's advice:\n%s", stderr.String())
		}
	}
}
