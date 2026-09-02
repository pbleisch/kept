//go:build !windows

package confine

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/thread"
)

// A read-only box is what the adversarial pass runs in, and the claim it makes
// is narrow and easy to state wrongly: *nothing inside the box may be written*.
//
// The tests below measure it in a real box under the real provider, and they are
// deliberately differential. A profile that refused every write everywhere would
// satisfy a one-sided assertion while proving nothing about read-only-ness — the
// box root is refused as a write grant in every profile this package renders, so
// a probe aimed there passes for a repair box too. What separates the two is the
// directory a repair of the same promise *would* have been granted: a repair
// writes in it, and a pass must not.

// TestAReadOnlyBoxRefusesTheWritesARepairBoxAdmits is that difference, measured
// twice in one test rather than asserted once.
//
// The same box, the same scope and the same probe script run under two profiles
// that differ in one field. If the second one's writes were refused for any
// reason other than the declaration — an unwritable checkout, a provider that
// grants nothing under this root, a probe aimed somewhere nobody could write —
// the first one's would be refused too, and the test says so.
func TestAReadOnlyBoxRefusesTheWritesARepairBoxAdmits(t *testing.T) {
	nono, err := exec.LookPath("nono")
	if err != nil {
		t.Skip("real nono provider is not installed")
	}
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	kept := buildKept(t)

	// The probe writes where a repair of this promise is entitled to write, and
	// reads what a pass legitimately needs: the tree it is arguing about.
	const probe = `
ls . >/dev/null 2>&1 && echo list=reached || echo list=refused
cat internal/scratch.go >/dev/null 2>&1 && echo read=reached || echo read=refused
{ echo argued > internal/argued; } 2>/dev/null && echo scope=written || echo scope=refused
{ echo argued >> internal/scratch.go; } 2>/dev/null && echo existing=written || echo existing=refused
{ rm internal/scratch_test.go; } 2>/dev/null && echo delete=removed || echo delete=refused
`

	for _, run := range []struct {
		name     string
		readOnly bool
		want     []string
	}{
		{
			// The premise. A repair box grants the scope, so every write in it
			// lands — and if this half failed, the other half's refusals would
			// be about the host rather than about the profile.
			name: "a repair box writes in the scope its promise bound",
			want: []string{"list=reached", "read=reached", "scope=written", "existing=written", "delete=removed"},
		},
		{
			// The claim. The same directory, the same writes, and the box is
			// still legible — a pass that could not read the tree would be
			// arguing about nothing.
			name:     "a read-only box refuses every one of them",
			readOnly: true,
			want:     []string{"list=reached", "read=reached", "scope=refused", "existing=refused", "delete=refused"},
		},
	} {
		t.Run(run.name, func(t *testing.T) {
			root := ungrantedDir(t)
			repository, created := scratchBox(t, root)
			profile := boxProfile(t, repository, created, absolute(t, nono), kept, absolute(t, git))
			if run.readOnly {
				profile.ReadOnly = true
				profile.Unwritable = profile.Write
				profile.Write = nil
			}
			launch, err := NewNono(profile)
			if err != nil {
				t.Fatal(err)
			}
			if err := launch.Verified(); err != nil {
				t.Fatalf("the canary failed in a box the box package cut: %v", err)
			}

			stream := &testStream{}
			child, err := launch.Start(context.Background(), thread.Invocation{
				Path:   "/bin/sh",
				Args:   []string{"-c", probe},
				Env:    []string{"RUNTIME_TOKEN=only-this-credential"},
				Output: stream,
			})
			if err != nil {
				t.Fatal(err)
			}
			if exit := child.Wait(); exit.Code == nil || *exit.Code != 0 {
				t.Fatalf("confined probe exit = %#v: %s", exit, stream.String())
			}
			report := stream.String()
			for _, want := range run.want {
				if !strings.Contains(report, want) {
					t.Fatalf("confined probe report is missing %q:\n%s", want, report)
				}
			}

			// And the tree itself, because a report is what a shell said and a
			// box is what is there. A read-only box the session could not write
			// has the file it was handed and none it was not.
			changed, err := created.Change(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if run.readOnly {
				if len(changed.Paths) != 0 {
					t.Fatalf("a read-only box holds changes the session made: %v\n%s", changed.Paths, changed.Diff)
				}
				return
			}
			if len(changed.Paths) == 0 {
				t.Fatal("the repair box holds no change, so the comparison above measures nothing")
			}
		})
	}
}

// TestTheCanaryNoticesAReadOnlyBoxThatIsNotOne breaks the thing the declaration
// rests on and watches it fail.
//
// A `ReadOnly` field is a claim, and the canary is what turns it into a
// boundary. So the rendered profile keeps the grant a repair would have had
// while the canary is still told the box is read-only — which is exactly what a
// bug that forgot to clear the writable set would produce — and the probe has to
// notice.
func TestTheCanaryNoticesAReadOnlyBoxThatIsNotOne(t *testing.T) {
	launch := liveReadOnlyNono(t)
	granted := launch.profile.Unwritable
	report := measureCanary(t, launch, func(rendered *nonoProfile, _ *canaryState) {
		rendered.Filesystem.Allow = append(rendered.Filesystem.Allow, granted...)
	}, nil)

	found := false
	for _, probe := range report.Probes {
		if probe.Name != readOnlyCanaryProbe {
			if probe.Outcome != probeRefused {
				t.Fatalf("%s outcome = %q (%s) while %s was the one broken", probe.Name, probe.Outcome, probe.Detail, readOnlyCanaryProbe)
			}
			continue
		}
		found = true
		if probe.Outcome != probeReached {
			t.Fatalf("%s outcome = %q (%s): a read-only box that kept a repair's grant was not noticed", probe.Name, probe.Outcome, probe.Detail)
		}
	}
	if !found {
		t.Fatalf("the canary report does not contain %s: %#v", readOnlyCanaryProbe, report.Probes)
	}
	if err := report.valid(true); err == nil || !strings.Contains(err.Error(), readOnlyCanaryProbe) {
		t.Fatalf("report.valid(true) = %v, want a refusal naming %s", err, readOnlyCanaryProbe)
	}
}

// TestAReadOnlyProfileWithNothingToProveIsRefused is the other way the
// declaration could become an assertion: a profile that says read-only and names
// no path for the canary to be refused in would pass a canary that measured
// nothing about it.
func TestAReadOnlyProfileWithNothingToProveIsRefused(t *testing.T) {
	profile := fixtureProfile(t)
	profile.ReadOnly = true
	profile.Write = nil
	if _, err := NewNono(profile); err == nil {
		t.Fatal("a read-only profile with nothing for the canary to measure was accepted")
	} else if !strings.Contains(err.Error(), "names no path") {
		t.Fatalf("refusal = %v, want it to name what is missing", err)
	}

	// And the two halves may not both be in force: a writable path in a box
	// that says nothing may be written is a record disagreeing with what is
	// enforced, which is the same defect a blocked network beside a domain
	// allowlist is.
	profile = fixtureProfile(t)
	profile.ReadOnly = true
	profile.Unwritable = profile.Write
	if _, err := NewNono(profile); err == nil {
		t.Fatal("a read-only profile that grants a writable path was accepted")
	} else if !strings.Contains(err.Error(), "read-only and grants a writable path") {
		t.Fatalf("refusal = %v, want it to name the grant", err)
	}

	// A box that is not read-only may not name unwritable paths either: those
	// are the paths a repair is entitled to write, and a canary refused in them
	// would refuse every repair.
	profile = fixtureProfile(t)
	profile.Unwritable = profile.Write
	if _, err := NewNono(profile); err == nil {
		t.Fatal("a repair profile naming unwritable paths was accepted")
	} else if !strings.Contains(err.Error(), "not read-only") {
		t.Fatalf("refusal = %v, want it to name the declaration", err)
	}
}

// TestAReadOnlyProbeMayNotBeAimedAtSomethingAlreadyDenied is the rule the whole
// canary is built on, applied to the newest probe: a target kept itself put in
// filesystem.deny demonstrates that a denied path is denied, which is a
// tautology dressed as evidence. The hidden tier is denied by name in every
// profile and is already probed as write_kept.
func TestAReadOnlyProbeMayNotBeAimedAtSomethingAlreadyDenied(t *testing.T) {
	profile := fixtureProfile(t)
	profile.ReadOnly = true
	profile.Write = nil
	profile.Unwritable = []string{filepath.Join(profile.Box, hiddenTier, "boxes")}
	if _, err := NewNono(profile); err == nil {
		t.Fatal("a read-only probe aimed at the hidden tier was accepted")
	} else if !strings.Contains(err.Error(), "reaches .kept") {
		t.Fatalf("refusal = %v, want it to name the tier", err)
	}

	profile = fixtureProfile(t)
	profile.ReadOnly = true
	profile.Write = nil
	profile.Unwritable = []string{filepath.Dir(profile.Box)}
	if _, err := NewNono(profile); err == nil {
		t.Fatal("a read-only probe aimed outside the box was accepted")
	} else if !strings.Contains(err.Error(), "outside the box") {
		t.Fatalf("refusal = %v, want it to say where it is", err)
	}
}

// TestTheReadOnlyProbesPremiseIsProvedByTheParent is the premise every write
// probe in this package rests on, and the one the newest probe would be easiest
// to ship without: a refusal from a directory nobody could write proves nothing.
//
// The break is a directory the parent cannot write either, and what has to
// happen is that the canary refuses to run at all rather than reporting a
// boundary that held.
func TestTheReadOnlyProbesPremiseIsProvedByTheParent(t *testing.T) {
	nono, err := exec.LookPath("nono")
	if err != nil {
		t.Skip("real nono provider is not installed")
	}
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	repository, created := scratchBox(t, ungrantedDir(t))
	profile := boxProfile(t, repository, created, absolute(t, nono), buildKept(t), absolute(t, git))
	sealed := filepath.Join(created.Path(), "internal")
	profile.ReadOnly = true
	profile.Unwritable = []string{sealed}
	profile.Write = nil

	// The window is the evidence and nothing longer: the bit goes back the
	// moment the canary has answered, and a cleanup covers a failing or
	// panicking run.
	if err := os.Chmod(sealed, 0o500); err != nil {
		t.Fatal(err)
	}
	restored := false
	restore := func() {
		if restored {
			return
		}
		restored = true
		_ = os.Chmod(sealed, 0o700)
	}
	t.Cleanup(restore)

	launch, err := NewNono(profile)
	if err != nil {
		t.Fatal(err)
	}
	err = launch.Verified()
	restore()
	if err == nil {
		t.Fatal("the canary passed with a probe target the parent itself cannot write, so its refusal proved nothing")
	}
	if !strings.Contains(err.Error(), "is not writable") {
		t.Fatalf("verification error = %v, want it to name the unwritable target", err)
	}
}

// liveReadOnlyNono is the fixture profile as an adversarial pass renders it: the
// scope a repair would have been granted becomes the paths the canary must be
// refused in, and nothing inside the box is writable.
func liveReadOnlyNono(t *testing.T) *Nono {
	t.Helper()
	nono, err := exec.LookPath("nono")
	if err != nil {
		t.Skip("real nono provider is not installed")
	}
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	profile := fixtureProfile(t)
	profile.Nono = absolute(t, nono)
	profile.Git = absolute(t, git)
	profile.Kept = buildKept(t)
	profile.ReadOnly = true
	profile.Unwritable = profile.Write
	profile.Write = nil
	launch, err := NewNono(profile)
	if err != nil {
		t.Fatal(err)
	}
	return launch
}
