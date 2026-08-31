package ledger

import (
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/run"
)

func act(kind Kind, artifact, hash, by string, at time.Time) Act {
	return Act{
		Envelope: Envelope{V: Version, Kind: kind, At: at, Writer: run.ID("r"), Seq: 1},
		Artifact: artifact, Hash: hash, Norm: 1, By: by,
	}
}

// A gatherer that declares nothing gets no projection, and the empty string is
// what keeps its scope digest byte for byte what docs/decision/0033 defines.
// It is not a digest over an empty set — that would be a stable value folded
// into every declaring gatherer's scope, which is a different claim.
func TestNoDeclarationProjectsNothing(t *testing.T) {
	when := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	acts := []Act{act(Ratified, "promise:one", "sha256:aa", "A", when)}

	if got := Project(acts, nil, []string{"promise:one"}); got != "" {
		t.Errorf("a gatherer declaring nothing projects %q", got)
	}
	if got := Project(acts, []Kind{Ratified}, nil); got == "" {
		t.Error("a declared kind over no artifact projects nothing, so a scope emptying would read as a declaration withdrawn")
	}
}

// The projection moves when a person records an act about something in scope,
// and stands still otherwise. Both halves matter: the first is the whole
// reason it exists, and the second is what stops a declaring promise being due
// on every tick.
func TestAProjectionMovesOnlyForActsItNames(t *testing.T) {
	when := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	base := []Act{act(Ratified, "promise:one", "sha256:aa", "A", when)}
	kinds := []Kind{Ratified}
	scope := []string{"promise:one"}

	start := Project(base, kinds, scope)

	for _, c := range []struct {
		why   string
		acts  []Act
		moves bool
	}{
		{"the same acts read again", base, false},
		{"an act about an artifact out of scope",
			append(append([]Act{}, base...), act(Ratified, "promise:two", "sha256:bb", "A", when.Add(time.Hour))), false},
		{"an act of a kind nothing declared",
			append(append([]Act{}, base...), act(Retired, "promise:one", "sha256:aa", "A", when.Add(time.Hour))), false},
		{"a new act about an artifact in scope",
			append(append([]Act{}, base...), act(Ratified, "promise:one", "sha256:cc", "A", when.Add(time.Hour))), true},
		{"the same content ratified by somebody else",
			append(append([]Act{}, base...), act(Ratified, "promise:one", "sha256:aa", "B", when.Add(time.Hour))), true},
	} {
		got := Project(c.acts, kinds, scope)
		if moved := got != start; moved != c.moves {
			t.Errorf("%s: moved=%v, want %v", c.why, moved, c.moves)
		}
	}
}

// The stream is union-merged and interleaves arbitrarily, so a projection that
// folded in the order it was handed the records would be silently wrong and
// would look correct in every single-run test.
func TestAProjectionDoesNotDependOnTheOrderItIsHandedActs(t *testing.T) {
	when := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	forwards := []Act{
		act(Ratified, "promise:one", "sha256:aa", "A", when),
		act(Ratified, "promise:two", "sha256:bb", "A", when.Add(time.Hour)),
		act(Retired, "promise:one", "sha256:aa", "A", when.Add(2*time.Hour)),
	}
	backwards := []Act{forwards[2], forwards[1], forwards[0]}

	kinds := []Kind{Ratified, Retired}
	scope := []string{"promise:one", "promise:two"}
	if Project(forwards, kinds, scope) != Project(backwards, kinds, scope) {
		t.Error("a projection depends on the order it was handed the acts")
	}
}

// The guard docs/decision/0046 turns on, at the level it is decided: acts are
// human-paced and everything else is written by the tick.
func TestOnlyActsAPersonPerformsAreHumanPaced(t *testing.T) {
	for _, k := range []Kind{Ratified, Retired} {
		if !HumanPaced(k) {
			t.Errorf("%s is an act a person performs and does not read as one", k)
		}
	}
	for _, k := range []Kind{Verdict, Disposed, "", "observation", "run"} {
		if HumanPaced(k) {
			t.Errorf("%s is written by the tick and reads as human-paced, so a gatherer declaring it would feed itself", k)
		}
	}
}
