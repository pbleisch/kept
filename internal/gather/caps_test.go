package gather

import (
	"errors"
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/ledger"
)

// The bound on raw output fires before an extractor reads anything, and it says
// which bound and by how much. Per AGENTS.md, a check that has never fired is
// not a check.
func TestAProgramThatEmitsPastTheRawBoundIsRefused(t *testing.T) {
	if err := Bound("stdout", MaxRawOutput, MaxRawOutput); err != nil {
		t.Fatalf("output exactly at the bound was refused: %v", err)
	}

	err := Bound("stdout", MaxRawOutput+4096, MaxRawOutput)
	var large *ledger.TooLarge
	if !errors.As(err, &large) {
		t.Fatalf("Bound returned %v, want a refusal", err)
	}
	if large.Bound != "stdout" {
		t.Errorf("the refusal names %q, so a receipt cannot say which bound was exceeded", large.Bound)
	}
	if !strings.Contains(err.Error(), "4096 over") {
		t.Errorf("the refusal reads as %q, and does not say by how much", err)
	}
}
