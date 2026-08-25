package main

import (
	"strings"
	"testing"
)

// TestLandIsNoLongerDeferred is the acceptance line: typing `kept land` gets an
// answer about a land rather than a reason it does not exist.
func TestLandIsNoLongerDeferred(t *testing.T) {
	if why, deferred := unbuilt["land"]; deferred {
		t.Fatalf("land is implemented and still reported as deferred: %q", why)
	}
	// The reason it used to carry was that no thread produced a landable diff.
	// Nothing may still say that, in any verb's reason.
	for verb, why := range unbuilt {
		if strings.Contains(why, "nothing to land") || strings.Contains(why, "no thread produces one") {
			t.Errorf("%s still says nothing produces a landable diff: %q", verb, why)
		}
	}
}

// TestLandWithNoThreadListsWhatCouldBeLanded: with nothing named, a land says
// what it could be pointed at rather than guessing one, which is the same shape
// `kept repair` takes and for the same reason.
func TestLandWithNoThreadListsWhatCouldBeLanded(t *testing.T) {
	w := newWorkspace(t, map[string]string{"first": "the first promise"})

	code, stdout, stderr := w.run("land")
	if code != exitOK {
		t.Fatalf("land exited %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, "nothing to land") {
		t.Fatalf("land says %q", stdout)
	}
	// It points at the verb that makes one, rather than leaving a dead end.
	if !strings.Contains(stdout, "kept repair") {
		t.Fatalf("land does not say what produces something to land: %q", stdout)
	}
}

// TestLandTakesOneThread: several threads and one target is a queue, and this
// slice does not have one.
func TestLandTakesOneThread(t *testing.T) {
	w := newWorkspace(t, map[string]string{"first": "the first promise"})

	code, _, stderr := w.run("land", "one", "two")
	if code != exitUsage {
		t.Fatalf("land with two threads exited %d", code)
	}
	if !strings.Contains(stderr, "one thread") {
		t.Fatalf("the refusal does not say why: %q", stderr)
	}
}
