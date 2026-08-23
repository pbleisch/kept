package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// userConfig writes a config where ReadUser will find it.
//
// The redirection is HOME, and the check below is not a formality. Every
// platform os.UserConfigDir supports derives its answer from HOME — but not all
// of them from XDG_CONFIG_HOME, which darwin ignores outright — so a test that
// redirected only the XDG variable would resolve to the operator's real
// configuration and overwrite it. This asserts the resolved path is inside the
// directory this test owns *before* anything is written to it, and fails rather
// than skips: a test that quietly stopped checking would leave the write it was
// guarding against as the next thing somebody added.
func userConfig(t *testing.T, contents string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	path, err := UserPath()
	if err != nil {
		t.Skip("this machine has no user configuration directory")
	}
	if !strings.HasPrefix(path, root+string(filepath.Separator)) {
		t.Fatalf("the user config resolves to %q, which is outside the directory this test owns at %q", path, root)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestTheOperatorsRepairConfigurationIsReadWhole covers the keys a dispatcher
// needs. None of them is a secret: the credential is a reference the
// confinement provider resolves. See docs/decision/0057.
func TestTheOperatorsRepairConfigurationIsReadWhole(t *testing.T) {
	userConfig(t, `
operator: Test Operator
repair:
  runtime: claude
  executable: /opt/homebrew/bin/claude
  credential: apple-password://api.anthropic.com/operator
  credential_expires: 2027-03-04
`)
	user, err := ReadUser()
	if err != nil {
		t.Fatal(err)
	}
	if user.Operator != "Test Operator" {
		t.Fatalf("operator = %q", user.Operator)
	}
	if user.Repair.Runtime != "claude" || user.Repair.Executable != "/opt/homebrew/bin/claude" {
		t.Fatalf("repair = %+v", user.Repair)
	}
	if user.Repair.Credential != "apple-password://api.anthropic.com/operator" {
		t.Fatalf("credential = %q", user.Repair.Credential)
	}
	// A bare date means the credential is good through that day rather than
	// dead at the midnight it begins, which is what somebody writing a date
	// means by it.
	want := time.Date(2027, 3, 5, 0, 0, 0, 0, time.Local)
	if !user.Repair.CredentialExpires.Equal(want) {
		t.Fatalf("credential_expires = %v, want %v", user.Repair.CredentialExpires, want)
	}
}

// TestARepairConfigurationWithNoLapseDateReadsAsUnset keeps "nobody said" apart
// from "it lapses at the zero time". The adapter refuses an unset expiry by
// name, so the distinction has to survive the read.
func TestARepairConfigurationWithNoLapseDateReadsAsUnset(t *testing.T) {
	userConfig(t, "operator: Test Operator\n")
	user, err := ReadUser()
	if err != nil {
		t.Fatal(err)
	}
	if !user.Repair.CredentialExpires.IsZero() {
		t.Fatalf("an absent lapse date read as %v", user.Repair.CredentialExpires)
	}
	if user.Repair.Runtime != "" || user.Repair.Credential != "" {
		t.Fatalf("an absent repair section read as %+v", user.Repair)
	}
}

// TestAnUnreadableLapseDateIsRefusedRatherThanRoundedToZero is the guard that
// matters most here: a lapse date nothing could parse, read as the zero time,
// would refuse every dispatch with "expiry is unset" and send an operator
// looking in the wrong place.
func TestAnUnreadableLapseDateIsRefusedRatherThanRoundedToZero(t *testing.T) {
	for _, written := range []string{"next year", "2027-13-45", "03/04/2027"} {
		userConfig(t, "repair:\n  credential_expires: \""+written+"\"\n")
		_, err := ReadUser()
		if err == nil {
			t.Fatalf("a lapse date of %q was accepted", written)
		}
		if !strings.Contains(err.Error(), "credential_expires") {
			t.Fatalf("error for %q = %v, want it to name the key", written, err)
		}
	}
}

// TestALapseDateMayBeAnExactInstant covers the other spelling, because a
// credential minted at a known moment lapses at one.
func TestALapseDateMayBeAnExactInstant(t *testing.T) {
	userConfig(t, "repair:\n  credential_expires: 2027-03-04T15:04:05Z\n")
	user, err := ReadUser()
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2027, 3, 4, 15, 4, 5, 0, time.UTC)
	if !user.Repair.CredentialExpires.Equal(want) {
		t.Fatalf("credential_expires = %v, want %v", user.Repair.CredentialExpires, want)
	}
}
