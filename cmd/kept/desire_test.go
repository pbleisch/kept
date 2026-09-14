package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// A candidate every shipped schema reads: a promise with a statement and an
// instrument the catalogue can run as written.
const shapedFields = `  becomes: promise
  candidate:
    statement: the README names every verb
    gather:
      readme:
        gatherer: command
        run: cat README.md
        scope: ["README.md"]
        fields:
          ok: exit == 0
    hold_when: obs.readme.ok
`

func desireFile(slug, fields string) string {
	return "---\nid: " + slug + "\nfields:\n  want: " + strings.ReplaceAll(slug, "-", " ") + "\n" + fields + "---\n"
}

// realizer is a promise whose own frontmatter says it realizes a desire.
func realizer(slug, desire string) string {
	return "---\nid: " + slug + "\nfields:\n  statement: " + slug + " holds\n" +
		"links:\n  - { target: desire:" + desire + ", relation: realizes }\n---\n\nWhy.\n"
}

// desireCorpus is every shipped schema, and one desire in each of the states a
// reader has to tell apart — including the one that looks committed and is not.
func desireCorpus(t *testing.T) *workspace {
	t.Helper()
	w := newCorpus(t)
	w.put("docs/desire/a-raw-want.md", desireFile("a-raw-want", ""))
	w.put("docs/desire/a-shaped-want.md", desireFile("a-shaped-want", shapedFields))
	w.put("docs/desire/a-want-an-unratified-promise-cites.md", desireFile("a-want-an-unratified-promise-cites", shapedFields))
	w.put("docs/desire/a-committed-want.md", desireFile("a-committed-want", shapedFields))
	w.put("docs/desire/a-declined-want.md", desireFile("a-declined-want", shapedFields+"  reopens_when: never\n"))
	w.put("docs/promise/an-unratified-promise.md", realizer("an-unratified-promise", "a-want-an-unratified-promise-cites"))
	w.put("docs/promise/a-ratified-promise.md", realizer("a-ratified-promise", "a-committed-want"))

	for _, args := range [][]string{
		{"ratify", "promise:a-ratified-promise", "--by", "the operator"},
		{"retire", "desire:a-declined-want", "--by", "the operator", "--note", "not worth a session"},
	} {
		if code, _, stderr := w.run(args...); code != exitOK {
			t.Fatalf("%s exited %d: %s", strings.Join(args, " "), code, stderr)
		}
	}
	return w
}

// **Every state, as kept status prints it.** Each row says what has become of
// the want, and the one a reader would get wrong is the want an unratified
// promise cites: it is shaped, because writing a candidate commits nothing.
func TestStatusPrintsWhatHasBecomeOfEachDesire(t *testing.T) {
	w := desireCorpus(t)

	for id, state := range map[string]string{
		"desire:a-raw-want":                         "raw",
		"desire:a-shaped-want":                      "shaped",
		"desire:a-want-an-unratified-promise-cites": "shaped",
		"desire:a-committed-want":                   "committed",
		"desire:a-declined-want":                    "retired",
	} {
		row := w.line(id)
		if got := strings.Fields(row)[0]; got != state {
			t.Errorf("%s reads %q, want %q:\n%s", id, got, state, row)
		}
	}
	mustContain(t, "the committed row", w.line("desire:a-committed-want"), "realized by promise:a-ratified-promise")
	mustContain(t, "the raw row", w.line("desire:a-raw-want"), "names nothing it would become")
	mustContain(t, "the shaped row", w.line("desire:a-shaped-want"), "would become a promise")

	// Nothing about a want is owed, so the default view lists none of them, and
	// says what became of them in the one line that counts them.
	code, stdout, stderr := w.run("status")
	if code != exitOK {
		t.Fatalf("status exited %d: %s", code, stderr)
	}
	mustContain(t, "status", stdout,
		"5 desires are not listed", "1 raw, 2 shaped, 1 committed, 1 retired")
	if strings.Contains(stdout, "desire:a-") {
		t.Errorf("the default view listed a desire:\n%s", stdout)
	}
}

// --state takes a desire's words, and admits a desire by the word its row
// prints and by nothing else. --state unratified used to admit every desire,
// because that is their standing; no desire row prints it any more.
func TestStateNarrowsDesiresByTheWordTheirRowPrints(t *testing.T) {
	w := desireCorpus(t)

	code, stdout, stderr := w.run("status", "--state", "shaped")
	if code != exitOK {
		t.Fatalf("status --state shaped exited %d: %s", code, stderr)
	}
	mustContain(t, "--state shaped", stdout, "desire:a-shaped-want", "desire:a-want-an-unratified-promise-cites")
	for _, other := range []string{"desire:a-raw-want", "desire:a-committed-want", "desire:a-declined-want"} {
		if strings.Contains(stdout, other) {
			t.Errorf("--state shaped admitted %s:\n%s", other, stdout)
		}
	}

	_, stdout, _ = w.run("status", "--state", "unratified")
	if strings.Contains(stdout, "desire:") {
		t.Errorf("--state unratified admitted a desire:\n%s", stdout)
	}
	mustContain(t, "--state unratified", stdout, "promise:an-unratified-promise")

	_, stdout, _ = w.run("status", "--state", "retired")
	mustContain(t, "--state retired", stdout, "desire:a-declined-want")
}

// The machine surface carries what the table prints: a desire object on a
// desire's record, beside its standing, and on no other record.
func TestJSONCarriesADesireObjectOnADesireAlone(t *testing.T) {
	w := desireCorpus(t)
	code, stdout, stderr := w.run("status", "--json")
	if code != exitOK {
		t.Fatalf("status --json exited %d: %s", code, stderr)
	}

	var doc struct {
		Artifacts []struct {
			ID       string `json:"id"`
			Standing struct {
				State string `json:"state"`
			} `json:"standing"`
			Desire *struct {
				State      string `json:"state"`
				Becomes    string `json:"becomes"`
				Unread     string `json:"unread"`
				RealizedBy []struct {
					Artifact string `json:"artifact"`
					Granted  struct {
						Hash string `json:"hash"`
						By   string `json:"by"`
					} `json:"granted"`
				} `json:"realized_by"`
			} `json:"desire"`
		} `json:"artifacts"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, stdout)
	}

	seen := 0
	for _, a := range doc.Artifacts {
		isDesire := strings.HasPrefix(a.ID, "desire:")
		if isDesire != (a.Desire != nil) {
			t.Errorf("%s: desire object present = %v", a.ID, a.Desire != nil)
			continue
		}
		if !isDesire {
			continue
		}
		seen++
		switch a.ID {
		case "desire:a-committed-want":
			if a.Desire.State != "committed" || len(a.Desire.RealizedBy) != 1 ||
				a.Desire.RealizedBy[0].Artifact != "promise:a-ratified-promise" || a.Desire.RealizedBy[0].Granted.Hash == "" {
				t.Errorf("%s: %+v", a.ID, a.Desire)
			}
			if a.Standing.State != "unratified" {
				t.Errorf("%s: standing moved with the desire state: %q", a.ID, a.Standing.State)
			}
		case "desire:a-raw-want":
			if a.Desire.State != "raw" || a.Desire.Unread == "" {
				t.Errorf("%s: %+v", a.ID, a.Desire)
			}
		case "desire:a-declined-want":
			if a.Desire.State != "retired" || a.Standing.State != "retired" || a.Desire.Becomes != "promise" {
				t.Errorf("%s: %+v, standing %q", a.ID, a.Desire, a.Standing.State)
			}
		}
	}
	if seen != 5 {
		t.Fatalf("the document carries %d desires, want 5", seen)
	}
}

// **Retire generalized.** Retiring a desire records the act, with its note, and
// leaves the file exactly as it was; retiring it again records nothing; and a
// document, which has neither standing nor a state a retirement moves, is still
// refused.
func TestRetiringADesireRecordsTheActAndKeepsTheFile(t *testing.T) {
	w := newCorpus(t)
	w.put("docs/desire/a-want.md", desireFile("a-want", ""))
	w.put("docs/document/a-document.md", "---\nid: a-document\n---\n\n# A document\n")
	file := w.read("docs/desire/a-want.md")

	code, stdout, stderr := w.run("retire", "desire:a-want", "--by", "the operator", "--note", "not now")
	if code != exitOK {
		t.Fatalf("retiring a desire exited %d: %s", code, stderr)
	}
	mustContain(t, "the act", stdout, "retire desire:a-want", "by       the operator")
	mustContain(t, "the ledger", w.acts(), `"artifact":"desire:a-want"`, `"kind":"retire"`, `"note":"not now"`)
	if strings.Contains(w.acts(), `"hash"`) {
		t.Errorf("a retirement of something that binds nothing named a content version:\n%s", w.acts())
	}
	if got := w.read("docs/desire/a-want.md"); got != file {
		t.Fatalf("retiring the desire changed its file:\n%s", got)
	}
	if row := w.line("desire:a-want"); !strings.HasPrefix(strings.TrimSpace(row), "retired") {
		t.Fatalf("after retiring, the row is:\n%s", row)
	}

	before := w.acts()
	code, stdout, _ = w.run("retire", "desire:a-want", "--by", "the operator")
	if code != exitOK || !strings.Contains(stdout, "already retired") {
		t.Fatalf("retiring twice exited %d: %s", code, stdout)
	}
	if w.acts() != before {
		t.Fatal("retiring a retired desire appended a record")
	}

	code, _, stderr = w.run("retire", "document:a-document", "--by", "the operator")
	if code != exitUsage || !strings.Contains(stderr, "no standing to retire") {
		t.Fatalf("retiring a document exited %d: %s", code, stderr)
	}
	if w.acts() != before {
		t.Fatal("a refused retirement appended a record")
	}

	// And ratifying one is still refused, with where commitment comes from.
	code, _, stderr = w.run("ratify", "desire:a-want", "--by", "the operator")
	if code != exitUsage {
		t.Fatalf("ratifying a desire exited %d", code)
	}
	mustContain(t, "the refusal", stderr, "cannot be ratified", "relation: realizes")
}

// A later act wins, end to end: a grant of something realizing a retired want
// commits it, and a retirement after that is accepted rather than refused as
// already done, because it moves the desire back.
func TestALaterActDecidesBetweenCommittedAndRetired(t *testing.T) {
	w := newCorpus(t)
	w.put("docs/desire/a-want.md", desireFile("a-want", shapedFields))
	w.put("docs/promise/a-realizer.md", realizer("a-realizer", "a-want"))

	for _, step := range []struct {
		args  []string
		state string
	}{
		{[]string{"retire", "desire:a-want", "--by", "the operator"}, "retired"},
		{[]string{"ratify", "promise:a-realizer", "--by", "the operator"}, "committed"},
		{[]string{"retire", "desire:a-want", "--by", "the operator", "--note", "declined after all"}, "retired"},
	} {
		code, stdout, stderr := w.run(step.args...)
		if code != exitOK || strings.Contains(stdout, "Nothing recorded") {
			t.Fatalf("%s exited %d: %s%s", strings.Join(step.args, " "), code, stdout, stderr)
		}
		if got := strings.Fields(w.line("desire:a-want"))[0]; got != step.state {
			t.Fatalf("after %s the desire reads %q, want %q", strings.Join(step.args, " "), got, step.state)
		}
	}
	mustContain(t, "the retired row", w.line("desire:a-want"), "retired after it was realized by promise:a-realizer")
}

// kept why says what became of a want, and for a declined one both halves of
// the decline: the reason on the act and the condition on the desire.
func TestWhySaysWhatBecameOfADesire(t *testing.T) {
	w := desireCorpus(t)

	code, stdout, stderr := w.run("why", "desire:a-declined-want")
	if code == exitUsage {
		t.Fatalf("why exited %d: %s", code, stderr)
	}
	mustContain(t, "why a declined want", stdout, "\ndesire\n  retired\n", "because: not worth a session", "reopens when: never")

	_, stdout, _ = w.run("why", "desire:a-committed-want")
	mustContain(t, "why a committed want", stdout, "\ndesire\n  committed ", "realized by promise:a-ratified-promise, ratified ")

	_, stdout, _ = w.run("why", "desire:a-raw-want")
	mustContain(t, "why a raw want", stdout, "\ndesire\n  raw\n", "names nothing it would become")

	_, stdout, _ = w.run("why", "promise:a-ratified-promise")
	if strings.Contains(stdout, "\ndesire\n") {
		t.Errorf("a promise's why carries a desire section:\n%s", stdout)
	}
}

// **The findings two fields make possible.** A retired desire that says nothing
// about what would re-open it, one still carrying its ripeness condition, and a
// live desire carrying a re-open condition are each reported — and a retired
// desire saying never, a live one saying what would ripen it, and a committed
// one are not.
func TestDoctorReportsAConditionWhereItsStateDoesNotReadIt(t *testing.T) {
	w := newCorpus(t)
	w.put("docs/desire/declined-silently.md", desireFile("declined-silently", ""))
	w.put("docs/desire/declined-with-a-leftover.md", desireFile("declined-with-a-leftover",
		"  ripe_when: a gatherer reports duration\n  reopens_when: never\n"))
	w.put("docs/desire/declined-properly.md", desireFile("declined-properly", "  reopens_when: a corpus of twenty thousand\n"))
	w.put("docs/desire/live-with-a-reopen.md", desireFile("live-with-a-reopen", "  reopens_when: never\n"))
	w.put("docs/desire/live-and-ripening.md", desireFile("live-and-ripening", "  ripe_when: internal/thread assembles a packet\n"))
	w.put("docs/desire/committed-with-both.md", desireFile("committed-with-both", shapedFields+"  ripe_when: x\n  reopens_when: y\n"))
	w.put("docs/promise/a-realizer.md", realizer("a-realizer", "committed-with-both"))

	for _, args := range [][]string{
		{"retire", "desire:declined-silently", "--by", "the operator"},
		{"retire", "desire:declined-with-a-leftover", "--by", "the operator"},
		{"retire", "desire:declined-properly", "--by", "the operator"},
		{"ratify", "promise:a-realizer", "--by", "the operator"},
	} {
		if code, _, stderr := w.run(args...); code != exitOK {
			t.Fatalf("%s exited %d: %s", strings.Join(args, " "), code, stderr)
		}
	}

	code, stdout, stderr := w.run("doctor")
	if code != exitOK {
		t.Fatalf("doctor exited %d, and a finding does not move the exit code: %s", code, stderr)
	}
	mustContain(t, "doctor", stdout,
		"3 desires whose condition is not where its state reads it",
		"desire:declined-silently\n      retired, and nothing says what would re-open it",
		"desire:declined-with-a-leftover\n      retired, and it still carries ripe_when",
		"desire:live-with-a-reopen\n      raw, and it carries reopens_when")
	for _, quiet := range []string{"desire:declined-properly", "desire:live-and-ripening", "desire:committed-with-both"} {
		if strings.Contains(stdout, quiet) {
			t.Errorf("doctor reported %s:\n%s", quiet, stdout)
		}
	}

	// The guard, broken on purpose: give the silent decline a condition and move
	// the leftover out, and those findings go.
	w.put("docs/desire/declined-silently.md", desireFile("declined-silently", "  reopens_when: never\n"))
	w.put("docs/desire/declined-with-a-leftover.md", desireFile("declined-with-a-leftover", "  reopens_when: never\n"))
	_, stdout, _ = w.run("doctor")
	if strings.Contains(stdout, "declined-silently") || strings.Contains(stdout, "declined-with-a-leftover") {
		t.Errorf("a condition put where its state reads it is still reported:\n%s", stdout)
	}
	mustContain(t, "doctor after the fix", stdout, "1 desire whose condition", "desire:live-with-a-reopen")
}
