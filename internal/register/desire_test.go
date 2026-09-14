package register

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/canonical"
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/run"
)

const desireSchema = `type: desire
hash: []
fields:
  want:         { type: string, required: true }
  becomes:      { type: enum, values: [promise, policy, canon] }
  candidate:    { type: map }
  ripe_when:    { type: string }
  reopens_when: { type: string }
body: note
`

// realizingSchema is a promise schema that admits the relation, as the one this
// repository ships does.
const realizingSchema = `type: promise
hash: [statement, gather, hold_when, trigger]
fields:
  statement: { type: string, required: true }
  gather:    { type: map }
  hold_when: { type: string }
  trigger:   { type: enum, values: [cadence, on-act, once] }
body: why
links:
  allowed_relations: [serves, cites, supersedes, realizes]
`

const rawDesire = `---
id: d
fields:
  want: repair picks which drift to spend a session on
---
`

const shapedDesire = `---
id: d
fields:
  want: repair picks which drift to spend a session on
  becomes: promise
  candidate:
    statement: a tick that could act on several drifts records which it ranked first
    gather:
      tests: { gatherer: go/tests }
    hold_when: obs.tests.failed == 0
---
`

// realizing is a promise whose own frontmatter says it realizes desire:d.
func realizing(statement string) string {
	return `---
id: p
fields:
  statement: ` + statement + `
  hold_when: obs.tests.failed == 0
  trigger: cadence
links:
  - { target: desire:d, relation: realizes }
---

Why.
`
}

func desireWorkspace(t *testing.T, desire string) *workspace {
	t.Helper()
	return newWorkspace(t, map[string]string{
		"_schema/desire.yaml":  desireSchema,
		"_schema/promise.yaml": realizingSchema,
		"desire/d.md":          desire,
	})
}

func (w *workspace) desire(id string) *Desire {
	w.t.Helper()
	s := w.standingOf(id)
	if s.Desire == nil {
		w.t.Fatalf("%s carries no desire state; its standing is %q", id, s.State)
	}
	return s.Desire
}

// The first two states need no ledger at all: a desire is raw until what it
// says it would become reads, and shaped once it does.
func TestADesireIsRawUntilItsCandidateReadsAndThenShaped(t *testing.T) {
	w := desireWorkspace(t, rawDesire)

	d := w.desire("desire:d")
	if d.State != DesireRaw {
		t.Fatalf("a desire naming nothing is %q, want %q", d.State, DesireRaw)
	}
	if !strings.Contains(d.Unread, "names nothing") {
		t.Errorf("a raw desire does not say what it is missing: %q", d.Unread)
	}

	w.put("desire/d.md", shapedDesire)
	d = w.desire("desire:d")
	if d.State != DesireShaped || d.Becomes != "promise" || d.Unread != "" {
		t.Fatalf("a desire with a candidate that reads is %q (becomes %q, unread %q), want shaped",
			d.State, d.Becomes, d.Unread)
	}

	// The guard: a candidate that stops reading takes the desire back to raw,
	// with nobody having marked anything.
	w.put("desire/d.md", strings.Replace(shapedDesire, "    statement: a tick that could act on several drifts records which it ranked first\n", "", 1))
	d = w.desire("desire:d")
	if d.State != DesireRaw || !strings.Contains(d.Unread, "statement") {
		t.Fatalf("a candidate missing its statement is %q (%q), want raw naming the statement", d.State, d.Unread)
	}
}

// **Writing a candidate promise does not commit anything.** A promise citing the
// desire and not ratified leaves it shaped; ratifying that promise is what
// commits it; and the desire's file was not touched in between.
func TestOnlyARatifiedArtifactThatRealizesADesireCommitsIt(t *testing.T) {
	w := desireWorkspace(t, shapedDesire)
	w.put("promise/p.md", realizing("the tick records which drift it ranked first"))

	if d := w.desire("desire:d"); d.State != DesireShaped {
		t.Fatalf("a desire cited by an unratified promise is %q, want %q", d.State, DesireShaped)
	}

	before := w.read("desire/d.md")
	granted := w.ratify("promise:p")

	d := w.desire("desire:d")
	if d.State != DesireCommitted {
		t.Fatalf("a desire a ratified promise realizes is %q, want %q", d.State, DesireCommitted)
	}
	if len(d.RealizedBy) != 1 || d.RealizedBy[0].Artifact != "promise:p" || !d.RealizedBy[0].Granted.Hash.Equal(granted) {
		t.Fatalf("the commitment does not name what realizes it and the grant: %+v", d.RealizedBy)
	}

	// Committed is derived, so the desire's file did not move.
	if after := w.read("desire/d.md"); after != before {
		t.Fatalf("committing a desire wrote into the desire:\n%s", after)
	}

	// And nothing else carries a desire state.
	if s := w.standingOf("promise:p"); s.Desire != nil {
		t.Fatalf("a promise carries a desire state: %+v", s.Desire)
	}
}

// A commitment rests on standing in force now. Revise the realizing promise and
// the desire is shaped again until the revision is ratified, which is the same
// moment the promise itself obliges anybody again.
func TestRevisingTheRealizingArtifactLeavesTheDesireShaped(t *testing.T) {
	w := desireWorkspace(t, shapedDesire)
	w.put("promise/p.md", realizing("the tick records which drift it ranked first"))
	w.ratify("promise:p")
	if d := w.desire("desire:d"); d.State != DesireCommitted {
		t.Fatalf("before the revision the desire is %q", d.State)
	}

	w.put("promise/p.md", realizing("the tick records every drift it ranked"))
	if d := w.desire("desire:d"); d.State != DesireShaped {
		t.Fatalf("a desire whose realizing promise was revised is %q, want %q", d.State, DesireShaped)
	}
	w.ratify("promise:p")
	if d := w.desire("desire:d"); d.State != DesireCommitted {
		t.Fatalf("after ratifying the revision the desire is %q, want %q", d.State, DesireCommitted)
	}
}

// Three ways a ratified artifact can mention a desire without its own author
// saying it realizes it, and none of them commits: a body link states no
// relation, a manifest edge is a third party's, and a kind whose schema does not
// admit the relation has written a defect.
func TestOnlyTheRealizingArtifactsOwnAdmittedLinkCommits(t *testing.T) {
	cases := map[string]map[string]string{
		"a body link": {
			"promise/p.md": strings.Replace(realizing("a statement"),
				"links:\n  - { target: desire:d, relation: realizes }\n", "", 1) + "\nSee [[desire:d]].\n",
		},
		"a manifest edge": {
			"promise/p.md": strings.Replace(realizing("a statement"),
				"links:\n  - { target: desire:d, relation: realizes }\n", "", 1),
			"_links/p.yaml": "subject: promise:p\nlinks:\n  - { target: desire:d, relation: realizes }\n",
		},
		"another relation": {
			"promise/p.md": strings.Replace(realizing("a statement"), "relation: realizes", "relation: cites", 1),
		},
		"a kind that does not admit it": {
			"promise/p.md":         realizing("a statement"),
			"_schema/promise.yaml": strings.Replace(realizingSchema, ", realizes]", "]", 1),
		},
	}
	for name, files := range cases {
		t.Run(name, func(t *testing.T) {
			w := desireWorkspace(t, shapedDesire)
			for path, body := range files {
				w.put(path, body)
			}
			w.ratify("promise:p")
			if s := w.standingOf("promise:p"); s.State != Ratified {
				t.Fatalf("the promise is %q, so this proves nothing", s.State)
			}
			if d := w.desire("desire:d"); d.State != DesireShaped {
				t.Fatalf("%s committed the desire: %q, %+v", name, d.State, d.RealizedBy)
			}
		})
	}
}

// Retire is how a want is declined, and the file stays exactly as it was.
func TestARetiredDesireIsRetiredAndKeepsItsFile(t *testing.T) {
	w := desireWorkspace(t, shapedDesire)
	w.release("desire:d")

	d := w.desire("desire:d")
	if d.State != DesireRetired {
		t.Fatalf("a retired desire is %q, want %q", d.State, DesireRetired)
	}
	if d.Becomes != "promise" {
		t.Errorf("a retired desire no longer says what it would have become: %q", d.Becomes)
	}
	if _, ok := w.corpus().Get("desire:d"); !ok {
		t.Fatal("the retired desire's file is gone")
	}
}

// act is one act at a moment the test chooses. The ordering rule is about which
// of two acts happened later, and a test that let the clock decide would pass
// or fail on how fast the machine appended.
func act(kind ledger.Kind, id string, hash canonical.Hash, at time.Time) ledger.Act {
	return ledger.Act{
		Envelope: ledger.Envelope{V: ledger.Version, Kind: kind, At: at, Writer: run.ID("test"), Seq: 1},
		Artifact: id, Hash: hash.String(), Norm: hash.Norm, By: "paul",
	}
}

// **A later act wins, and a file edit is not an act.** A retirement is
// overturned by a grant performed after it of something that realizes the want,
// and by nothing else: not by an earlier grant, not by a grant at the same
// moment, and not by a realizes link written into a promise ratified before the
// decline.
func TestALaterGrantOverturnsARetirementAndNothingElseDoes(t *testing.T) {
	w := desireWorkspace(t, shapedDesire)
	w.put("promise/p.md", realizing("the tick records which drift it ranked first"))
	c := w.corpus()
	hash := w.hash("promise:p")
	t0 := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	state := func(acts ...ledger.Act) *Desire {
		t.Helper()
		for _, s := range Derive(c, acts) {
			if s.Artifact == "desire:d" {
				return s.Desire
			}
		}
		t.Fatal("desire:d is not in the register")
		return nil
	}

	if d := state(act(ledger.Retired, "desire:d", canonical.Hash{}, t0),
		act(ledger.Ratified, "promise:p", hash, t0.Add(time.Hour))); d.State != DesireCommitted {
		t.Errorf("a grant after the retirement left the desire %q, want %q", d.State, DesireCommitted)
	}

	d := state(act(ledger.Ratified, "promise:p", hash, t0),
		act(ledger.Retired, "desire:d", canonical.Hash{}, t0.Add(time.Hour)))
	if d.State != DesireRetired {
		t.Errorf("a retirement after the grant left the desire %q, want %q", d.State, DesireRetired)
	}
	if len(d.RealizedBy) != 1 {
		t.Errorf("a desire retired while something realizes it does not say what: %+v", d.RealizedBy)
	}

	if d := state(act(ledger.Retired, "desire:d", canonical.Hash{}, t0),
		act(ledger.Ratified, "promise:p", hash, t0)); d.State != DesireRetired {
		t.Errorf("a grant at the same moment as the retirement left the desire %q, want %q", d.State, DesireRetired)
	}

	// The file edit. The promise was ratified before the decline without saying
	// it realizes anything; the link is added afterwards, and links are not part
	// of the grant, so the promise is still ratified — and still older than the
	// retirement.
	w.put("promise/p.md", strings.Replace(realizing("the tick records which drift it ranked first"),
		"links:\n  - { target: desire:d, relation: realizes }\n", "", 1))
	unlinked := w.hash("promise:p")
	w.put("promise/p.md", realizing("the tick records which drift it ranked first"))
	c = w.corpus()
	if !w.hash("promise:p").Equal(unlinked) {
		t.Fatal("adding a link moved the promise's hash, so this proves nothing")
	}
	acts := []ledger.Act{
		act(ledger.Ratified, "promise:p", unlinked, t0),
		act(ledger.Retired, "desire:d", canonical.Hash{}, t0.Add(time.Hour)),
	}
	for _, s := range Derive(c, acts) {
		switch s.Artifact {
		case "promise:p":
			if s.State != Ratified {
				t.Fatalf("the promise is %q after the link was added, so this proves nothing", s.State)
			}
		case "desire:d":
			if s.Desire.State != DesireRetired {
				t.Errorf("a realizes link added to an older grant overturned the retirement: %q", s.Desire.State)
			}
		}
	}
}

func (w *workspace) read(name string) string {
	w.t.Helper()
	src, err := os.ReadFile(filepath.Join(w.root, filepath.FromSlash(name)))
	if err != nil {
		w.t.Fatal(err)
	}
	return string(src)
}
