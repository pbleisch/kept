package repair

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/pbleisch/kept/internal/config"
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/thread"
)

// DefaultAttempts is how many attempts one run of a repair episode may open when
// .kept/config.yaml names no bound.
//
// Three, because one attempt and its rework is two chances to answer an argument
// and the third is the one that says whether a case keeps coming back — and
// because an adversarial pass's reject reworks until this is spent, so a smaller
// number would escalate a repair the pass had argued against only once. See
// docs/decision/0086.
const DefaultAttempts = 3

// Bounds is what one run of a repair episode may spend without a person.
//
// A run is one invocation of `kept repair`, and the attempts bound is scoped to
// it rather than to the thread. The budget exists to bound what happens between
// two words from a person, and `kept repair --rework <thread>` is one. See
// docs/decision/0086.
type Bounds struct {
	// Attempts is how many attempts this run may open, at least one.
	Attempts int
	// From says where Attempts came from, in the words a person would use to go
	// and change it.
	From string
	// Unapplied is an operator's bound that was looser than the repository's
	// and so did not apply. It is said rather than dropped: a value somebody
	// wrote and that does nothing is worth one line.
	Unapplied string

	// WindowCeiling is the utilization at or above which the next attempt is
	// refused, and zero is no ceiling.
	WindowCeiling float64

	// Automatic is whether a reading that selects rework is reworked without a
	// person's word. When it is not, that reading escalates instead and names
	// the command that would continue it.
	Automatic bool
}

// ResolveBounds is the one place the three sources of an attempts bound meet.
//
// The repository's is the bound: .kept/config.yaml's repair.attempts, or
// [DefaultAttempts] where it names none. An operator's own value and a command
// line may make it stricter and never looser, because how many unattended
// sessions may change this repository in one go is the repository's policy and
// a machine or a person at a terminal can spend less of it and not more.
//
// The two looser cases are answered differently on purpose. A flag above the
// bound is refused, because a person typed it just now and silently granting
// fewer attempts than they asked for would satisfy an impossible request with a
// different one. An operator's value above the bound does not apply and is
// reported, because it lives in a file that serves every repository on the
// machine and a stricter repository is not that file being wrong.
//
// Zero is unset for both overrides. See docs/decision/0086.
func ResolveBounds(repository int, operator config.Bounds, flag int, automatic bool) (Bounds, error) {
	out := Bounds{Attempts: DefaultAttempts, From: "the default", WindowCeiling: operator.WindowCeiling, Automatic: automatic}
	if repository > 0 {
		out.Attempts, out.From = repository, ".kept/config.yaml"
	}
	switch {
	case operator.Attempts <= 0:
	case operator.Attempts < out.Attempts:
		out.Attempts, out.From = operator.Attempts, "your config"
	case operator.Attempts > out.Attempts:
		out.Unapplied = fmt.Sprintf("your config's repair.attempts of %d is looser than the %d from %s, so it does not apply", operator.Attempts, out.Attempts, out.From)
	}
	switch {
	case flag < 0:
		return Bounds{}, fmt.Errorf("--attempts %d is not a number of attempts a run may open; at least 1 is", flag)
	case flag == 0:
	case flag > out.Attempts:
		return Bounds{}, fmt.Errorf("--attempts %d is looser than the bound of %d from %s; a run may open fewer attempts than that and never more", flag, out.Attempts, out.From)
	default:
		out.Attempts, out.From = flag, "--attempts"
	}
	return out, nil
}

// String is the bound as a run states it before it starts.
func (b Bounds) String() string {
	return fmt.Sprintf("at most %d %s, from %s", b.Attempts, plural(b.Attempts, "attempt", "attempts"), b.From)
}

// windowReading reads the latest usage-window reading this episode's repair
// sessions reported against the operator's ceiling, and reports whether the
// next attempt is refused.
//
// The sentence is returned either way, because the answer a person needs is
// different in each case and none of them is silence: no ceiling configured, no
// window reported, a reading under the ceiling, and a reading at or above it.
//
// **No window reported is not a window at zero.** A self-hosted runtime reports
// none, and so does a build that renamed the event kept reads; the ceiling then
// refuses nothing and the sentence says nobody reported one.
// [thread.LatestWindow]'s second value is the whole of that distinction, and it
// is carried here rather than dropped.
//
// The repair sessions, and not the pass's. The next attempt spends the repair
// runtime's credential, and a reading reported by a session spending another
// one is a reading of an account the next attempt does not touch. See
// docs/decision/0086.
func windowReading(entries []thread.Entry, ceiling float64) (string, bool) {
	if ceiling <= 0 {
		return "no usage-window ceiling is set in your config", false
	}
	reading, reported := latestRepairWindow(entries)
	if !reported {
		return "no repair session in this episode has reported a usage window, which refuses nothing", false
	}
	var over, under []string
	for _, window := range reading.Windows {
		utilization, err := strconv.ParseFloat(strings.TrimSpace(window.Utilization), 64)
		if err != nil {
			// A reading the receipt refused at the write never reaches here. One
			// that did and still will not parse is not a reading of zero either,
			// so it is named and treated as a window this ceiling cannot vouch
			// for.
			over = append(over, fmt.Sprintf("%s reads %q, which is not a number", window.Name, window.Utilization))
			continue
		}
		said := fmt.Sprintf("%s reads %s", window.Name, window.Utilization)
		if utilization >= ceiling {
			over = append(over, said)
		} else {
			under = append(under, said)
		}
	}
	if len(over) > 0 {
		return fmt.Sprintf("%s, at or above the usage-window ceiling of %s in your config", strings.Join(over, " and "), formatCeiling(ceiling)), true
	}
	return fmt.Sprintf("%s, under the usage-window ceiling of %s", strings.Join(under, " and "), formatCeiling(ceiling)), false
}

// latestRepairWindow is the last window reading any repair session in the
// receipt reported.
func latestRepairWindow(entries []thread.Entry) (thread.WindowUpdate, bool) {
	sessions := thread.Sessions(entries)
	for i := len(sessions) - 1; i >= 0; i-- {
		if sessions[i].Kind != ledger.ThreadPacket {
			continue
		}
		if reading, reported := thread.LatestWindow(sessions[i].Entries); reported {
			return reading, true
		}
	}
	return thread.WindowUpdate{}, false
}

func formatCeiling(ceiling float64) string {
	return strconv.FormatFloat(ceiling, 'f', -1, 64)
}
