package repair

import (
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/assess"
	"github.com/pbleisch/kept/internal/thread"
)

// finding is one derived check's answer.
func finding(check assess.Check, outcome assess.Outcome) assess.Finding {
	return assess.Finding{Check: check, Outcome: outcome, Detail: string(check) + " " + string(outcome)}
}

// clearChecklist is every check worked and clear, with shared-evidence set to
// whatever the case says — it fires on every repair under internal/ here.
func checklist(shared assess.Outcome, overrides ...assess.Finding) []assess.Finding {
	out := []assess.Finding{
		finding(assess.InstrumentChanged, assess.Clear),
		finding(assess.AlreadySatisfied, assess.Clear),
		finding(assess.PropertyExercised, assess.Clear),
		finding(assess.PathsInScope, assess.Clear),
		finding(assess.SharedEvidence, shared),
	}
	for _, override := range overrides {
		for i := range out {
			if out[i].Check == override.Check {
				out[i] = override
			}
		}
	}
	return out
}

func reading(number int, findings []assess.Finding, ruling thread.Ruling) assess.AttemptReading {
	argument := assess.Argument{Ran: true, Said: "a case\n\nruling: " + string(ruling), Ruling: ruling}
	if ruling == "" {
		argument = assess.Argument{Ran: true, Said: "a case with no ruling", RulingWhy: "its last line is \"a case with no ruling\""}
	}
	return assess.AttemptReading{
		Number: number,
		Assessment: assess.Assessment{
			Findings: findings,
			Argument: argument,
			Entry:    assess.Entry{Anchored: true},
		},
	}
}

func dispatched(status Status, pass ArgueStatus) *Result {
	return &Result{Status: status, Argument: &ArgueResult{Status: pass}}
}

// TestAnAttemptsReadingSelectsItsActionInTheTablesOrder is docs/decision/0086's
// table, row by row, including the rows that are easy to get wrong: a check
// about the repository that fires and selects nothing, a check that repeats, an
// unread check whose cause is the repair itself, and a pass that made no ruling
// kept can read.
func TestAnAttemptsReadingSelectsItsActionInTheTablesOrder(t *testing.T) {
	for _, c := range []struct {
		name     string
		earlier  []assess.AttemptReading
		current  assess.AttemptReading
		result   *Result
		assessor bool
		want     Action
		says     string
	}{
		{
			name:     "every check about the act clear and the pass accepts",
			current:  reading(1, checklist(assess.Clear), thread.RulingAccept),
			result:   dispatched(Repaired, Argued),
			assessor: true,
			want:     ActionReady,
			says:     "ruled accept",
		},
		{
			name:     "shared-evidence fires and selects nothing",
			current:  reading(1, checklist(assess.Fired), thread.RulingAccept),
			result:   dispatched(Repaired, Argued),
			assessor: true,
			want:     ActionReady,
		},
		{
			name:    "no assessor is configured",
			current: reading(1, checklist(assess.Fired), ""),
			result:  dispatched(Repaired, Unconfigured),
			want:    ActionReady,
			says:    "no assessor is configured",
		},
		{
			name:     "the pass rules reject",
			current:  reading(1, checklist(assess.Clear), thread.RulingReject),
			result:   dispatched(Repaired, Argued),
			assessor: true,
			want:     ActionRework,
			says:     "ruled reject",
		},
		{
			name:     "a check about the act fires for the first time",
			current:  reading(2, checklist(assess.Clear, finding(assess.PropertyExercised, assess.Fired)), thread.RulingAccept),
			earlier:  []assess.AttemptReading{reading(1, checklist(assess.Clear), thread.RulingReject)},
			result:   dispatched(Repaired, Argued),
			assessor: true,
			want:     ActionRework,
			says:     "property-exercised fired",
		},
		{
			name:     "the same check fires again",
			current:  reading(2, checklist(assess.Clear, finding(assess.PathsInScope, assess.Fired)), thread.RulingAccept),
			earlier:  []assess.AttemptReading{reading(1, checklist(assess.Clear, finding(assess.PathsInScope, assess.Fired)), thread.RulingReject)},
			result:   dispatched(Repaired, Argued),
			assessor: true,
			want:     ActionEscalate,
			says:     "paths-in-scope fired again, as it did after attempt 1",
		},
		{
			name:     "shared-evidence fires twice and still selects nothing",
			current:  reading(2, checklist(assess.Fired), thread.RulingAccept),
			earlier:  []assess.AttemptReading{reading(1, checklist(assess.Fired), thread.RulingReject)},
			result:   dispatched(Repaired, Argued),
			assessor: true,
			want:     ActionReady,
		},
		{
			name:     "escalation beats rework",
			current:  reading(2, checklist(assess.Clear, finding(assess.InstrumentChanged, assess.Fired)), thread.RulingReject),
			earlier:  []assess.AttemptReading{reading(1, checklist(assess.Clear, finding(assess.InstrumentChanged, assess.Fired)), thread.RulingReject)},
			result:   dispatched(Repaired, Argued),
			assessor: true,
			want:     ActionEscalate,
		},
		{
			name:     "a check about the act unread from outside the episode",
			current:  reading(1, checklist(assess.Clear, finding(assess.PathsInScope, assess.Unread)), thread.RulingAccept),
			result:   dispatched(Repaired, Argued),
			assessor: true,
			want:     ActionEscalate,
			says:     "paths-in-scope could not be worked",
		},
		{
			name:     "unread because the attempt left the promise unmeasurable",
			current:  reading(1, checklist(assess.Clear, finding(assess.PropertyExercised, assess.Unread)), thread.RulingReject),
			result:   dispatched(Undecided, Argued),
			assessor: true,
			want:     ActionRework,
			says:     "nothing could measure the promise",
		},
		{
			name: "unread because the attempt changed nothing and nothing is anchored",
			current: func() assess.AttemptReading {
				r := reading(1, checklist(assess.Unread, finding(assess.PathsInScope, assess.Unread)), "")
				r.Assessment.Entry.Anchored = false
				r.Assessment.Argument = assess.Argument{}
				return r
			}(),
			result:   &Result{Status: Unrepaired, Unchanged: true, Argument: &ArgueResult{Status: Unanchored}},
			assessor: true,
			want:     ActionRework,
			says:     "still reads not-kept",
		},
		{
			name:     "the pass argued and made no ruling",
			current:  reading(1, checklist(assess.Clear), ""),
			result:   dispatched(Repaired, Argued),
			assessor: true,
			want:     ActionEscalate,
			says:     "made no ruling kept can read",
		},
		{
			name:     "the pass was refused",
			current:  reading(1, checklist(assess.Clear), ""),
			result:   &Result{Status: Repaired, Argument: &ArgueResult{Status: ArgueRefused, Detail: "the canary failed"}},
			assessor: true,
			want:     ActionEscalate,
			says:     "the canary failed",
		},
		{
			name:     "the pass could not be worked at all",
			current:  reading(1, checklist(assess.Clear), ""),
			result:   &Result{Status: Repaired},
			assessor: true,
			want:     ActionEscalate,
			says:     "could not be worked",
		},
		{
			name:     "an accept over a promise that is not kept",
			current:  reading(1, checklist(assess.Clear), thread.RulingAccept),
			result:   dispatched(Unrepaired, Argued),
			assessor: true,
			want:     ActionRework,
			says:     "still reads not-kept",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			readings := append(append([]assess.AttemptReading(nil), c.earlier...), c.current)
			action, why := decide(readings, c.result, c.assessor)
			if action != c.want {
				t.Fatalf("selected %s (%v), want %s", action, why, c.want)
			}
			if len(why) == 0 {
				t.Fatal("an action was selected with no reason given")
			}
			if c.says != "" && !strings.Contains(strings.Join(why, "; "), c.says) {
				t.Fatalf("the reasons are %v, want one saying %q", why, c.says)
			}
		})
	}
}

// TestOnlyChecksAboutTheActSelect holds the classification the table rests on,
// in both directions.
func TestOnlyChecksAboutTheActSelect(t *testing.T) {
	for _, check := range assess.Checklist() {
		want := check != assess.SharedEvidence
		if check.OfTheAct() != want {
			t.Errorf("%s is about the act: %v, want %v", check, check.OfTheAct(), want)
		}
	}
}
