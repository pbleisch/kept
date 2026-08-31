package thread

import (
	"strings"
	"testing"
)

// The constraint's whole job is to refuse a runtime whose stream shape or
// credential precedence this adapter does not understand. Widening it from one
// exact version to a policy must not cost that, so every case below breaks the
// property it names and watches the refusal land.

func TestAVersionOffTheLineIsRefusedUnderEveryPolicy(t *testing.T) {
	for _, policy := range []VersionPolicy{"", ReviewedVersions, LineVersions} {
		for _, version := range []string{"2.2.0", "3.0.0", "1.9.999"} {
			reviewed, err := claudeSupport.admit(Claude, version, policy)
			if err == nil {
				t.Fatalf("claude %s admitted under %q policy (reviewed=%v); a version off the 2.1 line must refuse", version, policy, reviewed)
			}
			if !strings.Contains(err.Error(), "version is unsupported") || !strings.Contains(err.Error(), "not on the 2.1 line") {
				t.Fatalf("claude %s under %q refused with %v; want it named as off the line", version, policy, err)
			}
		}
	}
}

func TestAVersionOlderThanAnyReviewedIsRefusedUnderEveryPolicy(t *testing.T) {
	// The 2.1 line, so the line check passes, and older than 2.1.241, so no
	// review ever looked at it. LineVersions widens forwards and not back.
	for _, policy := range []VersionPolicy{"", ReviewedVersions, LineVersions} {
		if _, err := claudeSupport.admit(Claude, "2.1.238", policy); err == nil {
			t.Fatalf("claude 2.1.238 admitted under %q; it is older than the oldest reviewed build", policy)
		} else if !strings.Contains(err.Error(), "older than the reviewed 2.1.241") {
			t.Fatalf("claude 2.1.238 under %q refused with %v; want it named as older than the reviewed floor", policy, err)
		}
	}
}

func TestAnUnreviewedVersionOnTheLineRefusesByDefaultAndNamesThePolicyThatWouldTakeIt(t *testing.T) {
	for _, policy := range []VersionPolicy{"", ReviewedVersions} {
		_, err := claudeSupport.admit(Claude, "2.1.999", policy)
		if err == nil {
			t.Fatalf("claude 2.1.999 admitted under %q; nobody has read it", policy)
		}
		if !strings.Contains(err.Error(), "version is unreviewed") {
			t.Fatalf("refusal was %v; an unread build on the line is unreviewed rather than unsupported", err)
		}
		// A refusal that does not say what to run leaves the operator to guess,
		// and the guess that unblocks them is the one that removes the check.
		if !strings.Contains(err.Error(), "repair.runtime_versions: line") {
			t.Fatalf("refusal was %v; it must name the policy that would dispatch on it", err)
		}
	}
}

func TestTheLinePolicyTakesAnUnreviewedVersionAndSaysSoInEveryEventItWrites(t *testing.T) {
	reviewed, err := claudeSupport.admit(Claude, "2.1.999", LineVersions)
	if err != nil {
		t.Fatalf("claude 2.1.999 refused under the line policy: %v", err)
	}
	if reviewed {
		t.Fatal("claude 2.1.999 reported as reviewed; nobody has read it")
	}
	// This is what keeps the line policy from being a silent acceptance. The
	// stamp travels on every native event, so a receipt from an unread build
	// cannot later be read as having come from one that was checked.
	stamp := claudeSupport.stamp("2.1.999", LineVersions)
	if !strings.Contains(stamp, unreviewedMark) {
		t.Fatalf("adapter version %q does not mark the session unreviewed", stamp)
	}
	if !strings.Contains(stamp, "2.1.999") {
		t.Fatalf("adapter version %q does not name the version that ran", stamp)
	}
}

func TestAReviewedVersionIsStampedWithTheVersionThatRanAndNoMark(t *testing.T) {
	// The failure this pins is a constant adapter version: under an exact pin
	// the recorded and the running version were forced equal, and a policy that
	// admits more than one makes a constant a claim about a session that did
	// not happen.
	for _, version := range claudeSupport.reviewed {
		reviewed, err := claudeSupport.admit(Claude, version, ReviewedVersions)
		if err != nil || !reviewed {
			t.Fatalf("reviewed claude %s was not admitted as reviewed: %v", version, err)
		}
		stamp := claudeSupport.stamp(version, ReviewedVersions)
		if !strings.Contains(stamp, version) {
			t.Fatalf("adapter version %q does not name the version that ran", stamp)
		}
		if strings.Contains(stamp, unreviewedMark) {
			t.Fatalf("adapter version %q marks a reviewed build unreviewed", stamp)
		}
	}
}

func TestAnAdapterWithNoLineRefusesEveryUnreviewedVersion(t *testing.T) {
	// Codex and Pi state their evidence as a set with no neighbourhood. The
	// nearest possible patch release is still refused, under the policy that
	// would have taken one for Claude.
	for _, tc := range []struct {
		runtime Runtime
		support support
		version string
	}{
		{Codex, codexSupport, "0.149.1"},
		{Pi, piSupport, "0.84.1"},
	} {
		if _, err := tc.support.admit(tc.runtime, tc.version, LineVersions); err == nil {
			t.Fatalf("%s %s admitted; an adapter declaring no line accepts only what was reviewed", tc.runtime, tc.version)
		} else if !strings.Contains(err.Error(), "version is unsupported") {
			t.Fatalf("%s %s refused with %v; want it named unsupported", tc.runtime, tc.version, err)
		}
	}
}

func TestAnUnknownPolicyIsRefusedRatherThanReadAsTheLooseOne(t *testing.T) {
	// A typo must not resolve to whichever member it is nearest, and it must
	// certainly not resolve to the one that dispatches on unread builds.
	for _, policy := range []VersionPolicy{"any", "all", "off", "Line", "REVIEWED"} {
		if _, err := claudeSupport.admit(Claude, ClaudeVersion, policy); err == nil {
			t.Fatalf("policy %q was accepted; the set is closed", policy)
		} else if !strings.Contains(err.Error(), "version policy is unknown") {
			t.Fatalf("policy %q refused with %v; want it named as unknown", policy, err)
		}
	}
}

func TestAnUnreadableVersionIsRefusedRatherThanGuessedNewer(t *testing.T) {
	// Guessing that an unparseable version is newer is the unsafe guess, and
	// the runtime's own reporting is what a refusal should point at.
	for _, version := range []string{"pending", "2.1", "2.1.251.1", "v2.1.251", "2.1.x", "2..1"} {
		if _, err := claudeSupport.admit(Claude, version, LineVersions); err == nil {
			t.Fatalf("claude %q admitted; it is not a version kept can order", version)
		} else if !strings.Contains(err.Error(), "version is unreadable") {
			t.Fatalf("claude %q refused with %v; want it named as unreadable", version, err)
		}
	}
}

func TestTheInstalledVersionIsAdmittedWithoutConfiguration(t *testing.T) {
	// Self-hosting's own question. 2.1.251 is in the reviewed set on the
	// evidence recorded beside claudeSupport, so the strict default takes it
	// and an operator sets nothing.
	reviewed, err := claudeSupport.admit(Claude, "2.1.251", "")
	if err != nil {
		t.Fatalf("the installed 2.1.251 was refused under the default policy: %v", err)
	}
	if !reviewed {
		t.Fatal("2.1.251 admitted but not as reviewed; it is in the reviewed set")
	}
}

func TestTheStampNeverDisagreesWithTheAdmission(t *testing.T) {
	// Two computations of reviewedness would fail in the direction that
	// matters: admitted as unreviewed and stamped as reviewed writes exactly
	// the evidence this design exists to prevent.
	for _, version := range []string{"2.1.241", "2.1.251", "2.1.999", "2.2.0", "pending"} {
		for _, policy := range []VersionPolicy{"", ReviewedVersions, LineVersions} {
			reviewed, err := claudeSupport.admit(Claude, version, policy)
			stamp := claudeSupport.stamp(version, policy)
			marked := strings.Contains(stamp, unreviewedMark)
			if err == nil && reviewed == marked {
				t.Fatalf("claude %s under %q: admitted reviewed=%v but stamp %q says otherwise", version, policy, reviewed, stamp)
			}
			if err != nil && !marked {
				t.Fatalf("claude %s under %q was refused, and its stamp %q does not mark it unreviewed", version, policy, stamp)
			}
		}
	}
}
