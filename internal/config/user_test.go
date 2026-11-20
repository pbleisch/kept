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

// TestTheAssessorsConfigurationIsItsOwnAndIsReadWhole is the second block, and
// what it asserts is separateness rather than parsing.
//
// Decision 0078 selects the assessor's runtime, model and effort independently
// of the repairer's, and the reason is the limit it exists to get past: two
// instances of one model can find one substitution reasonable. So the runtime,
// the executable, the credential and the version policy are all deliberately
// different here from the repair block above them, and the test would pass on a
// shared field only if it stopped being a test of anything.
func TestTheAssessorsConfigurationIsItsOwnAndIsReadWhole(t *testing.T) {
	userConfig(t, `
operator: Test Operator
repair:
  runtime: claude
  executable: /opt/homebrew/bin/claude
  credential: apple-password://api.anthropic.com/repairer
  credential_expires: 2027-03-04
assess:
  runtime: claude
  executable: /usr/local/bin/claude
  credential: apple-password://api.anthropic.com/assessor
  runtime_versions: line
  credential_expires: 2028-05-06
  model: opus
  effort: high
`)
	user, err := ReadUser()
	if err != nil {
		t.Fatal(err)
	}
	if user.Assess.Executable != "/usr/local/bin/claude" || user.Assess.Executable == user.Repair.Executable {
		t.Fatalf("the assessor's executable is %q and the repairer's is %q", user.Assess.Executable, user.Repair.Executable)
	}
	if user.Assess.Credential != "apple-password://api.anthropic.com/assessor" || user.Assess.Credential == user.Repair.Credential {
		t.Fatalf("the assessor's credential reference is %q and the repairer's is %q", user.Assess.Credential, user.Repair.Credential)
	}
	if user.Assess.RuntimeVersions != "line" || user.Repair.RuntimeVersions != "" {
		t.Fatalf("the version policies are assess=%q repair=%q", user.Assess.RuntimeVersions, user.Repair.RuntimeVersions)
	}
	if user.Assess.Model != "opus" || user.Assess.Effort != "high" {
		t.Fatalf("assessor model = %q, effort = %q", user.Assess.Model, user.Assess.Effort)
	}
	if want := time.Date(2028, time.May, 7, 0, 0, 0, 0, time.Local); !user.Assess.CredentialExpires.Equal(want) {
		t.Fatalf("assessor credential_expires = %s, want %s", user.Assess.CredentialExpires, want)
	}
	if user.Repair.CredentialExpires.Equal(user.Assess.CredentialExpires) {
		t.Fatal("the two blocks resolved to one expiry, so one of them was read from the other")
	}
}

// TestTheRepairersModelAndEffortAreReadFromItsOwnBlock is decision 0093: the
// repair: block may name a model and an effort, the assess: block may name
// others, and reading either one never fills in the other.
func TestTheRepairersModelAndEffortAreReadFromItsOwnBlock(t *testing.T) {
	userConfig(t, `
operator: Test Operator
repair:
  runtime: claude
  executable: /opt/homebrew/bin/claude
  credential: apple-password://api.anthropic.com/repairer
  model: " opus "
  effort: max
assess:
  runtime: claude
  executable: /opt/homebrew/bin/claude
  credential: apple-password://api.anthropic.com/assessor
  model: sonnet
`)
	user, err := ReadUser()
	if err != nil {
		t.Fatal(err)
	}
	if user.Repair.Model != "opus" || user.Repair.Effort != "max" {
		t.Fatalf("repairer model = %q, effort = %q", user.Repair.Model, user.Repair.Effort)
	}
	if user.Assess.Model != "sonnet" || user.Assess.Effort != "" {
		t.Fatalf("assessor model = %q, effort = %q; the assess block names sonnet and no effort", user.Assess.Model, user.Assess.Effort)
	}
}

// TestAConfigWithNoAssessorBlockIsNotAnAssessorConfiguredWrong is the state
// every machine is in until somebody sets one up: no adversarial pass runs, a
// repair still runs and still reports, and nothing here is an error. Reading an
// unset block as the repairer's would silently spend the repairer's credential
// on the pass, which is the one thing 0078 says these two must not share.
func TestAConfigWithNoAssessorBlockIsNotAnAssessorConfiguredWrong(t *testing.T) {
	userConfig(t, `
operator: Test Operator
repair:
  runtime: claude
  executable: /opt/homebrew/bin/claude
  credential: apple-password://api.anthropic.com/repairer
  credential_expires: 2027-03-04
`)
	user, err := ReadUser()
	if err != nil {
		t.Fatal(err)
	}
	if user.Assess.Runtime != "" || user.Assess.Executable != "" || user.Assess.Credential != "" {
		t.Fatalf("an absent assess block read as %#v", user.Assess)
	}
	if !user.Assess.CredentialExpires.IsZero() {
		t.Fatalf("an absent assess block has an expiry: %s", user.Assess.CredentialExpires)
	}
}

// TestTheOperatorsBoundsAreReadFromTheRepairBlock covers the two keys an
// operator sets about what an episode may spend. They are read from repair:
// alone, because the assessor's block embeds the runtime and a bound written
// there is a key nothing reads rather than a second bound. See
// docs/decision/0086.
func TestTheOperatorsBoundsAreReadFromTheRepairBlock(t *testing.T) {
	userConfig(t, `
repair:
  runtime: claude
  attempts: 2
  window_ceiling: 0.9
assess:
  runtime: claude
  attempts: 7
  window_ceiling: 0.1
`)
	user, err := ReadUser()
	if err != nil {
		t.Fatal(err)
	}
	if user.Bounds.Attempts != 2 {
		t.Errorf("attempts = %d, want the repair block's 2", user.Bounds.Attempts)
	}
	if user.Bounds.WindowCeiling != 0.9 {
		t.Errorf("window_ceiling = %v, want the repair block's 0.9", user.Bounds.WindowCeiling)
	}
}

// TestUnsetBoundsReadAsUnset keeps *nobody set a ceiling* apart from *a
// ceiling of zero*, which would refuse every attempt.
func TestUnsetBoundsReadAsUnset(t *testing.T) {
	userConfig(t, "repair:\n  runtime: claude\n")
	user, err := ReadUser()
	if err != nil {
		t.Fatal(err)
	}
	if user.Bounds != (Bounds{}) {
		t.Fatalf("an unset repair block read as bounds %+v", user.Bounds)
	}
}

// TestABoundThatCannotBeOneIsRefused breaks each key the way a slip of the
// keyboard would, and holds the read to refusing rather than to rounding.
func TestABoundThatCannotBeOneIsRefused(t *testing.T) {
	for _, bad := range []struct{ body, key string }{
		{"repair:\n  attempts: 0\n", "repair.attempts"},
		{"repair:\n  attempts: -2\n", "repair.attempts"},
		{"repair:\n  window_ceiling: 0\n", "repair.window_ceiling"},
		{"repair:\n  window_ceiling: -0.5\n", "repair.window_ceiling"},
	} {
		userConfig(t, bad.body)
		if _, err := ReadUser(); err == nil {
			t.Errorf("%q was read as a bound", bad.body)
		} else if !strings.Contains(err.Error(), bad.key) {
			t.Errorf("the refusal of %q does not name %s: %v", bad.body, bad.key, err)
		}
	}
}
