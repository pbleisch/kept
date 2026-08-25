package land

import (
	"strings"
	"testing"
)

// TestTheTargetIsCarriedAsAValueRatherThanSpelledOutAtEachSite: only origin/main
// is constructed today, and every ref the land uses is derived from the one
// value rather than written out again where it is needed.
func TestTheTargetIsCarriedAsAValueRatherThanSpelledOutAtEachSite(t *testing.T) {
	target := Origin()
	if target.Remote != "origin" || target.Branch != "main" {
		t.Fatalf("Origin() = %+v", target)
	}
	for name, got := range map[string]string{
		"Ref":      target.Ref(),
		"Tracking": target.Tracking(),
		"Refspec":  target.Refspec(),
		"String":   target.String(),
	} {
		if !strings.Contains(got, target.Branch) {
			t.Fatalf("%s() = %q, which does not derive from the target's branch", name, got)
		}
	}
	if target.Ref() != "refs/heads/main" {
		t.Fatalf("Ref() = %q", target.Ref())
	}
	// The tracking ref is never the local branch. That is the whole of
	// docs/decision/0015's in-lock rule, expressed where it cannot be missed.
	if target.Tracking() != "refs/remotes/origin/main" {
		t.Fatalf("Tracking() = %q", target.Tracking())
	}
	if target.Tracking() == target.Ref() {
		t.Fatal("the tracking ref and the local branch are the same ref")
	}
	// Forced, because a tracking ref is a copy of what the remote holds and git
	// forces every one of them. See docs/decision/0062.
	if !strings.HasPrefix(target.Refspec(), "+") {
		t.Fatalf("Refspec() = %q, and a tracking ref is forced", target.Refspec())
	}
	if target.Refspec() != "+refs/heads/main:refs/remotes/origin/main" {
		t.Fatalf("Refspec() = %q", target.Refspec())
	}
}

// TestATargetThatCouldBeReadAsAFlagIsRefused: a target becomes an argument to
// git, and a value out of a config file is a value somebody can write.
func TestATargetThatCouldBeReadAsAFlagIsRefused(t *testing.T) {
	if err := Origin().Valid(); err != nil {
		t.Fatalf("origin/main is not valid: %v", err)
	}
	for _, bad := range []Target{
		{Remote: "", Branch: "main"},
		{Remote: "origin", Branch: ""},
		{Remote: "--upload-pack=touch", Branch: "main"},
		{Remote: "origin", Branch: "--force"},
		{Remote: "origin", Branch: "main:refs/heads/other"},
		{Remote: "origin", Branch: "a b"},
		{Remote: "origin", Branch: "a..b"},
		{Remote: "origin", Branch: "a^b"},
		{Remote: "origin/main", Branch: "main"},
		{Remote: "origin", Branch: "a\x00b"},
	} {
		if err := bad.Valid(); err == nil {
			t.Fatalf("%+v was accepted as a landing target", bad)
		}
	}
}
