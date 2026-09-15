package repair

import (
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/config"
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/thread"
)

// TestTheAttemptsBoundIsTheRepositorysAndOnlyTightens is docs/decision/0086's
// rule for where the attempts bound comes from: the repository sets it, three
// when it names none, and an operator and a command line may only make it
// stricter.
func TestTheAttemptsBoundIsTheRepositorysAndOnlyTightens(t *testing.T) {
	for _, c := range []struct {
		name       string
		repository int
		operator   int
		flag       int
		want       int
		from       string
		unapplied  bool
	}{
		{name: "nothing names one", want: DefaultAttempts, from: "the default"},
		{name: "the repository names one", repository: 5, want: 5, from: ".kept/config.yaml"},
		{name: "the operator is stricter", repository: 5, operator: 2, want: 2, from: "your config"},
		{name: "the operator is looser", repository: 2, operator: 4, want: 2, from: ".kept/config.yaml", unapplied: true},
		{name: "the operator is looser than the default", operator: 9, want: DefaultAttempts, from: "the default", unapplied: true},
		{name: "the flag is stricter", repository: 5, operator: 3, flag: 1, want: 1, from: "--attempts"},
		{name: "the flag equals the bound", repository: 3, flag: 3, want: 3, from: "--attempts"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := ResolveBounds(c.repository, config.Bounds{Attempts: c.operator}, c.flag, true)
			if err != nil {
				t.Fatal(err)
			}
			if got.Attempts != c.want || got.From != c.from {
				t.Fatalf("bound = %d from %q, want %d from %q", got.Attempts, got.From, c.want, c.from)
			}
			if (got.Unapplied != "") != c.unapplied {
				t.Fatalf("unapplied = %q, want it said: %v", got.Unapplied, c.unapplied)
			}
		})
	}
}

// TestAFlagLooserThanTheBoundIsRefused breaks the rule from the command line.
// A person who typed --attempts 9 against a bound of 3 asked for something that
// will not happen, and a run that quietly opened three would be answering a
// different request.
func TestAFlagLooserThanTheBoundIsRefused(t *testing.T) {
	_, err := ResolveBounds(3, config.Bounds{}, 9, true)
	if err == nil {
		t.Fatal("--attempts above the repository's bound was accepted")
	}
	for _, want := range []string{"--attempts 9", "3", ".kept/config.yaml"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not name %q: %v", want, err)
		}
	}
	// And the operator's own stricter bound is the one a flag is held to.
	if _, err := ResolveBounds(5, config.Bounds{Attempts: 2}, 3, true); err == nil {
		t.Fatal("--attempts above the operator's stricter bound was accepted")
	}
	if _, err := ResolveBounds(3, config.Bounds{}, -1, true); err == nil {
		t.Fatal("a negative --attempts was accepted")
	}
}

// TestTheWindowCeilingRefusesTheNextAttemptAtOrAboveIt holds the ceiling to the
// reading the episode's own repair sessions reported, and to the two readings
// that must not be confused: no window reported, and a window at zero.
func TestTheWindowCeilingRefusesTheNextAttemptAtOrAboveIt(t *testing.T) {
	reading := func(utilizations ...string) []thread.Entry {
		var windows []thread.Window
		for i, u := range utilizations {
			windows = append(windows, thread.Window{Name: []string{"five_hour", "seven_day"}[i], Utilization: u})
		}
		return []thread.Entry{
			packetEntry(),
			{Envelope: ledger.Envelope{Kind: ledger.ThreadWindow}, Window: &thread.WindowUpdate{NativeRef: "n", Windows: windows}},
		}
	}

	for _, c := range []struct {
		name    string
		entries []thread.Entry
		ceiling float64
		refused bool
		says    string
	}{
		{name: "below the ceiling", entries: reading("0.24", "0.11"), ceiling: 0.9, says: "under the usage-window ceiling"},
		{name: "at the ceiling", entries: reading("0.90", "0.11"), ceiling: 0.9, refused: true, says: "five_hour reads 0.90, at or above"},
		{name: "above in the second window", entries: reading("0.2", "0.95"), ceiling: 0.9, refused: true, says: "seven_day reads 0.95"},
		{name: "in overage", entries: reading("1.20"), ceiling: 1.1, refused: true, says: "five_hour reads 1.20"},
		{name: "no ceiling is set", entries: reading("0.99"), ceiling: 0, says: "no usage-window ceiling"},
		{name: "no window was reported", entries: []thread.Entry{packetEntry()}, ceiling: 0.01, says: "has reported a usage window, which refuses nothing"},
	} {
		t.Run(c.name, func(t *testing.T) {
			said, refused := windowReading(c.entries, c.ceiling)
			if refused != c.refused {
				t.Fatalf("refused = %v, want %v: %s", refused, c.refused, said)
			}
			if !strings.Contains(said, c.says) {
				t.Fatalf("said %q, want it to say %q", said, c.says)
			}
		})
	}
}

// TestAWindowAPassReportedIsNotTheNextAttemptsReading breaks the ceiling's
// scope: a reading from an adversarial pass is about the credential the pass
// spends, and the next attempt spends the repair runtime's.
func TestAWindowAPassReportedIsNotTheNextAttemptsReading(t *testing.T) {
	entries := []thread.Entry{
		packetEntry(),
		{Envelope: ledger.Envelope{Kind: ledger.ThreadWindow}, Window: &thread.WindowUpdate{NativeRef: "n", Windows: []thread.Window{{Name: "five_hour", Utilization: "0.30"}}}},
		{Envelope: ledger.Envelope{Kind: ledger.ThreadBrief}, Brief: &ledger.ArtifactRef{Kind: ledger.ThreadBriefArtifact}},
		{Envelope: ledger.Envelope{Kind: ledger.ThreadWindow}, Window: &thread.WindowUpdate{NativeRef: "m", Windows: []thread.Window{{Name: "five_hour", Utilization: "0.99"}}}},
	}
	said, refused := windowReading(entries, 0.9)
	if refused {
		t.Fatalf("the pass's reading refused the next repair attempt: %s", said)
	}
	if !strings.Contains(said, "0.30") {
		t.Fatalf("the reading is not the repair session's: %s", said)
	}
}

func packetEntry() thread.Entry {
	return thread.Entry{Envelope: ledger.Envelope{Kind: ledger.ThreadPacket}, Packet: &ledger.ArtifactRef{Kind: ledger.ThreadPacketArtifact}}
}
