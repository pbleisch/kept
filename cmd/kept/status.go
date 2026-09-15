package main

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/register"
	"github.com/pbleisch/kept/internal/tick"
)

// statusCmd prints the register: what has standing, at which content version,
// and what does not and why.
//
// One artifact at a time is not a workflow. The question this answers is *which
// of the things I approved have been changed since I approved them*, and that
// is only a question about a corpus.
//
// By default it prints the artifacts whose standing needs a person and counts
// the rest. A corpus is mostly artifacts that are exactly as they were
// approved, so a row per artifact buries the four that are not in the
// ninety-five that are, and the operator ends up narrowing by hand to see the
// thing the verb exists to say. [needsAttention] is what the default keeps and
// `--all` is the whole listing, unchanged. See docs/decision/0076.
//
// Nothing here writes. The whole answer is derived on the spot from two things
// that were already true: the acts in the ledger, and what each file says
// now. See docs/decision/0001.
func statusCmd(args []string, stdout, stderr io.Writer) int {
	fs, root := flags("status", stderr)
	asJSON := fs.Bool("json", false, "machine-readable output; the only stable format")
	listAll := fs.Bool("all", false, "list every artifact, not only the ones needing attention")
	f := narrow(fs, "status", "standing", standings)
	rest, err := parse(fs, args)
	if err != nil {
		return exitUsage
	}
	if len(rest) > 0 {
		fmt.Fprintf(stderr, "kept: status takes no arguments; did you mean show %s?\n", rest[0])
		return exitUsage
	}

	c, code := open(*root, stderr)
	if code != exitOK {
		return code
	}
	if code := f.check(c, stderr); code != exitOK {
		return code
	}
	if code := sound(c, stderr); code != exitOK {
		return code
	}

	store := ledgerStore(c)
	empty := ledger.Empty(store)
	acts, quarantined, err := ledger.ReadActs(store)
	if err != nil {
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return exitUsage
	}
	writeQuarantined(stderr, quarantined)

	all := register.Derive(c, acts)
	population, entries := narrowRegister(f, all)
	code = verdict(entries, quarantined)
	// A repair episode handed to a person is something to look at whatever the
	// artifact's standing says, so it is read here, beside the register, and
	// changes no exit code. See docs/decision/0086.
	escalations := openEscalations(store, acts, entries)

	// A selector that named nothing means the question was never put, which is
	// exit 3 and not an empty table. A state that admitted nothing means it was
	// put and the answer is none, which is whatever the empty set exits.
	// See [filter].
	if f.selecting() && len(population) == 0 {
		code = exitUnknown
	}

	if *asJSON {
		// The same envelope every other question answers in, with truth absent
		// rather than empty: status looks at the register and never at the
		// world, and an absent verdict says so. See docs/decision/0027.
		//
		// It carries every artifact the filters admit, needing attention or
		// not. The default view is an emphasis in a table and never a narrowing
		// of the question, so a consumer that read ninety-nine records
		// yesterday reads ninety-nine today; which of them is a finding travels
		// as a field on each one. See docs/decision/0076.
		return writeDocument(stdout, stderr, newDocument("status", c, registered(c, entries, quarantined, escalations), code, false))
	}

	// --state names states by hand, so it is the operator asking for those
	// states rather than for attention: it replaces the default rather than
	// composing with it, or `--state ratified` would print nothing whatever the
	// corpus held. --kind and --id name a population and compose with it, so
	// `--kind promise` is the promises that need attention.
	switch {
	case f.selecting() && len(population) == 0:
		f.writeUnselected(stdout, c, len(all))
	case f.states.any() && len(entries) == 0:
		f.writeUnadmitted(stdout, len(population))
	case *listAll || f.states.any():
		writeRegister(stdout, entries)
		writeSummary(stdout, entries, empty)
		f.writeNarrowing(stdout, len(entries), len(all))
	default:
		writeAttention(stdout, attending(entries), entries, f.selectors(), empty)
		f.writeNarrowing(stdout, len(entries), len(all))
	}

	writeEscalations(stdout, escalations)

	if n := len(c.Problems); n > 0 {
		fmt.Fprintf(stdout, "\n%s reading the corpus; kept list says what they are\n", count(n, "problem"))
	}
	return code
}

// escalated is one repair episode handed to a person and not yet answered.
type escalated struct {
	Artifact    string
	Disposition ledger.Disposition
	// Dispositions and Landings are the artifact's own streams, carried so the
	// machine surface derives the escalation the same way the table does.
	Dispositions []ledger.Disposition
	Landings     []ledger.Act
}

// openEscalations reads, for every ratified promise in the set, whether a repair
// episode escalated it and nobody has answered. It writes nothing: an
// escalation is answered by an act, a land or a revision, each already a record
// of its own, so whether one stands is derived on read. See
// [ledger.OpenEscalation].
func openEscalations(store ledger.Store, acts []ledger.Act, entries []register.Standing) []escalated {
	var out []escalated
	for _, e := range entries {
		if e.State != register.Ratified || !strings.HasPrefix(e.Artifact, "promise:") {
			continue
		}
		records, _, err := ledger.ReadDispositions(store, e.Artifact)
		if err != nil || len(records) == 0 {
			continue
		}
		var landings []ledger.Act
		for _, act := range acts {
			if act.Kind == ledger.Landed && act.Artifact == e.Artifact {
				landings = append(landings, act)
			}
		}
		if d, open := ledger.OpenEscalation(records, landings, e.Hash.String()); open {
			out = append(out, escalated{Artifact: e.Artifact, Disposition: d, Dispositions: records, Landings: landings})
		}
	}
	return out
}

// writeEscalations is the episodes waiting on a person, and what to run.
//
// It is printed after the register and never folded into it: standing is what
// a person granted, and an escalation is a repair asking for one, so a row in
// the standing table would put two questions in one column.
func writeEscalations(w io.Writer, escalations []escalated) {
	if len(escalations) == 0 {
		return
	}
	if len(escalations) == 1 {
		fmt.Fprint(w, "\n1 repair episode is escalated to a person\n")
	} else {
		fmt.Fprintf(w, "\n%d repair episodes are escalated to a person\n", len(escalations))
	}
	for _, e := range escalations {
		thread := e.Disposition.Thread
		if thread == "" {
			thread = "—"
		}
		fmt.Fprintf(w, "  %s  thread %s  %s\n", e.Disposition.At.Local().Format(stamp), thread, e.Artifact)
		if e.Disposition.Detail != "" {
			fmt.Fprintf(w, "      %s\n", e.Disposition.Detail)
		}
		if e.Disposition.Thread != "" {
			fmt.Fprintf(w, "      kept review %s reads the episode; kept repair --rework %s spends another run on it\n", e.Disposition.Thread, e.Disposition.Thread)
		}
	}
}

// verdict is the register as an exit code, so that this verb can be a hook.
// See docs/decision/0027 for the contract.
//
// 0027 reserves 1 for drift, which is standing measured against a truth that is
// not kept, and there is no truth here. What the register can answer is the
// half of that needing no observation — something was ratified and does not
// stand now — and both ways it happens are 1. Either it was edited, so what is
// in force was granted against a wording this is no longer, or its content is
// gone, which 0021 names a drift kind outright and never a retirement.
//
// An artifact nobody ever ratified is 0. It obliges nobody, an ordinary corpus
// is full of them, and exiting 1 for one would make the code useless in the
// hook it exists for.
//
// 3 is standing kept could not decide, which is not the same as standing that
// lapsed. A grant computed under canonicalization rules this build does not use
// cannot be compared against at all (0022); an artifact whose schema has gone
// has nothing to hash; and a ledger line that could not be read might have been
// the act that settles the question. Reporting any of those as 0 is
// exactly the false pass 0027's third code exists to prevent, and reporting
// them as 1 would send a reader looking for an edit nobody made.
func verdict(entries []register.Standing, quarantined []ledger.Quarantined) int {
	code := exitOK
	if len(quarantined) > 0 {
		code = exitUnknown
	}
	for _, e := range entries {
		switch {
		case e.State == register.Absent:
			return exitDrift
		case e.State != register.Unratified || e.Granted.At.IsZero():
			// In force, retired, or never granted. None of them is a finding.
			continue
		case e.Reason == register.Revised:
			return exitDrift
		default:
			// Granted once, and the comparison could not be made.
			code = exitUnknown
		}
	}
	return code
}

// registered is the register as a result, so that one document builder serves
// every question. It names no world, because none was walked.
func registered(c *corpus.Corpus, entries []register.Standing, quarantined []ledger.Quarantined, escalations []escalated) *tick.Result {
	res := &tick.Result{Quarantined: quarantined}
	streams := map[string]escalated{}
	for _, e := range escalations {
		streams[e.Artifact] = e
	}
	for _, e := range entries {
		s := tick.Subject{ID: e.Artifact, Standing: e}
		if held, found := streams[e.Artifact]; found {
			s.Dispositions, s.Landings = held.Dispositions, held.Landings
		}
		if entry, ok := c.Get(e.Artifact); ok {
			s.Entry = entry
		}
		res.Subjects = append(res.Subjects, s)
	}
	return res
}

// writeRegister prints the register: what state each artifact is in, when the
// act behind it was performed, and which artifact it is.
//
// The state is first and the id is last, which is the reverse of what this
// printed before and most of why it was hard to read. Every row of an ordinary
// corpus says the same word in the state column — eighty-five of the
// eighty-six here say `ratified` — so that column is what a reader is
// scanning, and it was starting at column seventy-five, past the right edge of
// a terminal, pushed there by the longest artifact id. Fixed-width answer
// left, unbounded name right, is `ls -l` and `git status -s`, and it is the
// only order in which a line too long for the screen spends its overflow on
// the part the reader has already found.
//
// The principal is not a column at all, and that is the rest of it. Every row
// ended `by Paul Bleisch <pbleisch@acm.org>`, thirty-four columns repeated
// eighty-six times, and the question this verb answers — *which of the things
// I approved have been changed since I approved them* — has one person in it
// already. So the names go under the table, said once, and per-row attribution
// is `kept why <artifact>`, which names the principal every time, and
// `--json`, which carries the whole grant. That is a stated cost rather than a
// free win: a corpus several people ratify into cannot be filtered by eye any
// more. It is the right trade because a name that distinguishes nothing is
// width, and 0048 is why the alternative is not to guess a shorter one — the
// operator is named and never guessed, so kept prints what the act says or
// nothing.
func writeRegister(w io.Writer, entries []register.Standing) {
	if len(entries) == 0 {
		return
	}

	t := table{indent: "  "}
	for _, e := range entries {
		t.add(stateOf(e), dayOf(e), e.Artifact)
		t.beneath(detailOf(e))
	}
	t.write(w)
	writeGrantors(w, entries)
}

// writeGrantors names who performed the acts this register is derived from.
//
// Sorted, so that the line does not depend on the order the ledger folded in,
// and counted rather than listed past three: a corpus with more ratifiers than
// that wants `--json`, and a footer that runs to two lines is a second table.
func writeGrantors(w io.Writer, entries []register.Standing) {
	seen := map[string]bool{}
	var names []string
	for _, e := range entries {
		if by := e.Granted.By; by != "" && !seen[by] {
			seen[by] = true
			names = append(names, by)
		}
	}
	sort.Strings(names)

	switch {
	case len(names) == 0:
	case len(names) == 1:
		fmt.Fprintf(w, "\nevery act here is %s's\n", names[0])
	case len(names) <= 3:
		fmt.Fprintf(w, "\ngranted by %s\n", strings.Join(names, ", "))
	default:
		fmt.Fprintf(w, "\ngranted by %s; kept why <artifact> names the one behind a row\n",
			count(len(names), "person"))
	}
}

// standingOf is the state as a person says it.
//
// Four of the five states are their own word. The fifth is `unratified`, which
// covers an artifact nobody ever granted and one whose content has moved since
// somebody did, and those two want opposite things from a reader — the first is
// the ordinary condition of a corpus and the second is a finding. So the one
// with nothing else to say says it here, and every other `unratified` row
// carries a line under it saying which way. The state in the ledger and in
// `--json` is unchanged: this is a wording, not a sixth state.
func standingOf(e register.Standing) string {
	if e.State == register.Unratified && e.Reason == register.Never {
		return "never ratified"
	}
	return string(e.State)
}

// stateOf is the word a row prints in its state column.
//
// For every artifact but a desire it is the standing. A desire's standing is only
// ever that nothing about it binds, or that it was retired, and neither says
// anything about the want — so its row prints what has become of it instead:
// raw, shaped, committed or retired. That is a wording on this surface and not a
// fifth standing; `--json` carries both, under their own names. See
// docs/decision/0083.
func stateOf(e register.Standing) string {
	if e.Desire != nil {
		return string(e.Desire.State)
	}
	return standingOf(e)
}

// dayOf is the day the act behind the state a row prints was performed. For a
// desire that is the retirement, or the latest grant of something realizing it,
// and nothing for a raw or shaped desire, which no act put in that state.
func dayOf(e register.Standing) string {
	if e.Desire == nil {
		return grantedOn(e)
	}
	switch e.Desire.State {
	case register.DesireRetired:
		return grantedOn(e)
	case register.DesireCommitted:
		return latestRealization(e.Desire).Granted.At.Format(day)
	}
	return ""
}

// detailOf is the line under a row, and for a desire it says what its state
// rests on: what is missing from a raw one, what a shaped one would become, what
// commits a committed one, and what still cites a retired one.
func detailOf(e register.Standing) string {
	if e.Desire == nil {
		return lapsed(e)
	}
	d := e.Desire
	switch d.State {
	case register.DesireRaw:
		return d.Unread
	case register.DesireShaped:
		return "it would become a " + d.Becomes + ", and its candidate reads"
	case register.DesireCommitted:
		return "realized by " + realizers(d)
	case register.DesireRetired:
		if len(d.RealizedBy) > 0 {
			return "retired after it was realized by " + realizers(d)
		}
	}
	return ""
}

func realizers(d *register.Desire) string {
	ids := make([]string, len(d.RealizedBy))
	for i, r := range d.RealizedBy {
		ids[i] = r.Artifact
	}
	return strings.Join(ids, ", ")
}

// latestRealization is the most recent grant among what realizes a desire,
// which is the act a committed desire's state rests on.
func latestRealization(d *register.Desire) register.Realization {
	var latest register.Realization
	for _, r := range d.RealizedBy {
		if r.Granted.At.After(latest.Granted.At) {
			latest = r
		}
	}
	return latest
}

// grantedOn is the day the act behind this state was performed, as a table
// cell. Which act it was is said by the state standing beside it: a ratified
// artifact was granted then, and a retired one was retired then.
func grantedOn(e register.Standing) string {
	if e.Granted.At.IsZero() {
		return ""
	}
	return e.Granted.At.Format(day)
}

// lapsed says what has happened to standing since it was granted, and is empty
// whenever the state already says it.
//
// The revised case is the one worth reading closely: it names the content
// version the operator agreed to and the one the file produces now, so that
// "this stopped applying" is a fact with two hashes behind it rather than an
// assertion. `never` and `retired` return nothing, because a line reading
// "never ratified" under a row reading `unratified` is the state a second time.
func lapsed(e register.Standing) string {
	switch e.State {
	case register.Absent:
		return fmt.Sprintf("granted at %s, and no artifact claims this id — never read as a retirement",
			e.Granted.Hash.Short())

	case register.Unratified:
		switch e.Reason {
		case register.Revised:
			return fmt.Sprintf("granted at %s, and it now says %s", e.Granted.Hash.Short(), e.Hash.Short())
		case register.Unrehashed:
			return fmt.Sprintf("granted under norm:%d, and this build hashes under norm:%d; nobody edited it",
				e.Granted.Hash.Norm, e.Hash.Norm)
		case register.Unbinding:
			return e.Unhashable
		}
	}
	return ""
}

func writeSummary(w io.Writer, entries []register.Standing, empty bool) {
	if len(entries) == 0 {
		fmt.Fprint(w, "no artifacts, so nothing to have standing\n")
		return
	}

	// Counted in the words the rows print, so a desire is counted as what has
	// become of it rather than as a standing its row never shows.
	counts := map[string]int{}
	for _, e := range entries {
		if e.Desire != nil {
			counts[string(e.Desire.State)]++
			continue
		}
		counts[string(e.State)]++
	}
	states := make([]string, 0, len(counts))
	for state, n := range counts {
		states = append(states, fmt.Sprintf("%d %s", n, state))
	}
	sort.Strings(states)

	fmt.Fprintf(w, "\n%s", count(len(entries), "artifact"))
	for _, s := range states {
		fmt.Fprintf(w, " · %s", s)
	}
	fmt.Fprintln(w)

	// A ledger nobody has written and a ledger whose records have all been
	// pruned look identical in the table above, and one of them means a clone
	// that never fetched the ledger. Saying which is the only way that does not
	// read as a system nobody has ever used. See docs/decision/0016.
	if empty {
		fmt.Fprint(w, "\nNothing has ever been ratified in this workspace.\n"+
			"kept ratify <artifact> starts the record.\n")
	}
}

// needsAttention reports whether this artifact's standing is something a person
// can and should do something about. It is the whole of what the default view
// decides, and it is one list rather than a severity ranking: the question the
// operator has is *what do I have to look at*, and an answer that sorted those
// into tiers would be asking them to read a taxonomy before reading the finding.
//
// The rule is narrow, and every line of it is a state a person acts on:
//
//   - Ratified is the resting state of an approved corpus and is not news.
//   - Retired is an act somebody performed on purpose. Nothing is owed and
//     editing the file does not bring it back, so there is nothing to do.
//   - Unratified, never granted, is the one an operator can settle with `kept
//     ratify`. It is exit 0 and it is still a finding: it is a promise or a
//     policy written and never granted standing, which is exactly what the
//     operator goes looking for.
//   - Unratified after a revision, or under canonicalization rules this build
//     does not compute in, is standing that lapsed. Those are the exit 1 and
//     exit 3 findings and they are the reason this verb has an exit code.
//   - Absent is content that was ratified and cannot be found, which 0021
//     refuses to read as a retirement.
//
// The one artifact left out is the one nothing can act on: unratified because
// nothing about its kind binds. `kept ratify` refuses it — there is no content
// version to key a grant to — so it cannot leave that state, and a permanent
// entry in a list of things to do teaches the reader to stop reading the list.
// It is counted rather than silently dropped, by [unlisted].
//
// **Unless somebody had already granted it.** A grant standing against an
// artifact whose kind stopped binding means a schema's hash list was emptied
// underneath a ratification, which de-ratified it with nobody deciding to; that
// is exit 3, nothing else reports it, and it is the opposite of a permanent
// fixture. The zero grant is the discrimination and it is the only reason this
// function looks at Granted at all.
func needsAttention(e register.Standing) bool {
	switch e.State {
	case register.Absent:
		return true
	case register.Unratified:
		if e.Reason == register.Unbinding {
			return !e.Granted.At.IsZero()
		}
		return true
	default:
		return false
	}
}

func attending(entries []register.Standing) []register.Standing {
	var out []register.Standing
	for _, e := range entries {
		if needsAttention(e) {
			out = append(out, e)
		}
	}
	return out
}

// writeAttention prints what needs a person, and accounts for what it left out.
//
// The census of states is not repeated here, because it is `--all`'s answer and
// this is not that question — what a reader of this view needs is the rows, the
// fraction they are of the whole, and one line saying where the rest went. An
// empty table is the good news and is said in words for the reason 0071 says
// every other empty result in words: an empty table reads as success and so
// does a table that failed to render.
func writeAttention(w io.Writer, attention, population []register.Standing, selectors string, empty bool) {
	if len(population) == 0 {
		fmt.Fprint(w, "no artifacts, so nothing to have standing\n")
		return
	}

	writeRegister(w, attention)
	if len(attention) == 0 {
		if len(population) == 1 {
			fmt.Fprint(w, "1 artifact, and it needs no attention\n")
		} else {
			fmt.Fprintf(w, "%s, and none of them needs attention\n", count(len(population), "artifact"))
		}
	} else {
		fmt.Fprintf(w, "\n%d of %s %s attention\n",
			len(attention), count(len(population), "artifact"), verb(len(attention), "needs", "need"))
	}
	for _, line := range unlisted(attention, population) {
		fmt.Fprintln(w, line)
	}
	// The way to the rest, spelled so that it can be pasted. A narrowed run's
	// hint carries the narrowing: `kept status --all` under `--kind promise`
	// would answer a question the reader did not ask.
	if len(attention) < len(population) {
		if selectors != "" {
			fmt.Fprintf(w, "kept status --all %s lists them all\n", selectors)
		} else {
			fmt.Fprint(w, "kept status --all lists every artifact\n")
		}
	}

	// A ledger nobody has written and a ledger whose records have all been
	// pruned look identical above, and one of them means a clone that never
	// fetched the ledger. See docs/decision/0016.
	if empty {
		fmt.Fprint(w, "\nNothing has ever been ratified in this workspace.\n"+
			"kept ratify <artifact> starts the record.\n")
	}
}

// unlisted accounts for the artifacts that were left out because nothing can be
// done about them, one line per kind.
//
// Silence would be the other answer and it is the wrong one: a reader who knows
// the corpus holds documents and sees no document anywhere would go looking for
// the bug, and a reader who does not know would never learn the kind exists.
// The line names the kind rather than the artifacts, because the reason is a
// property of the kind — nothing about a document binds — and repeating it per
// row would be a second table of things nobody can act on.
//
// Ratified and retired artifacts get no line. That they are not findings is the
// premise of the whole view rather than news, and their count is already in the
// fraction above. A desire is the exception, retired or not: its line is the one
// place the default view says what has become of the wants it left out, so every
// desire is counted there by state.
func unlisted(attention, population []register.Standing) []string {
	shown := map[string]bool{}
	for _, e := range attention {
		shown[e.Artifact] = true
	}

	counts := map[string]int{}
	wants := map[string]map[register.DesireState]int{}
	var kinds []string
	for _, e := range population {
		if shown[e.Artifact] || (e.Desire == nil && e.Reason != register.Unbinding) {
			continue
		}
		kind, _ := corpus.SplitRef(e.Artifact)
		if counts[kind] == 0 {
			kinds = append(kinds, kind)
		}
		counts[kind]++
		if e.Desire != nil {
			if wants[kind] == nil {
				wants[kind] = map[register.DesireState]int{}
			}
			wants[kind][e.Desire.State]++
		}
	}
	sort.Strings(kinds)

	var out []string
	for _, kind := range kinds {
		line := fmt.Sprintf("%s %s not listed: nothing about a %s binds, so %s cannot be ratified",
			count(counts[kind], kind), verb(counts[kind], "is", "are"), kind, pronoun(counts[kind]))
		if states := wantsByState(wants[kind]); states != "" {
			line += " — " + states
		}
		out = append(out, line)
	}
	return out
}

// wantsByState is how many desires are in each state, in the order a want moves
// through them. Every desire is counted in the one line, retired ones included,
// because the line is where the default view says what has become of the wants
// it did not list. See docs/decision/0083.
func wantsByState(n map[register.DesireState]int) string {
	var parts []string
	for _, state := range []register.DesireState{
		register.DesireRaw, register.DesireShaped, register.DesireCommitted, register.DesireRetired,
	} {
		if n[state] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n[state], state))
		}
	}
	return strings.Join(parts, ", ")
}

// verb agrees with a count. Both forms are given rather than derived: English
// makes a plural verb by dropping the s and a plural noun by adding one, and a
// helper that guessed which would be wrong on "is".
func verb(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

func pronoun(n int) string {
	if n == 1 {
		return "it"
	}
	return "they"
}

// standings is what --state admits on this verb: the words kept status prints,
// with the space in "never ratified" written as a hyphen so that it needs no
// quoting.
//
// `raw`, `shaped` and `committed` are what a desire's row prints, and a desire
// is admitted by that word alone: its standing is never the question a desire's
// row answers, so `--state unratified` does not include one. `retired` is both
// vocabularies' word for one act. See docs/decision/0083.
//
// `unratified` is every artifact that obliges nobody, however it got there —
// the state the ledger holds, the state --json carries, and the word the
// summary line counts under. `never-ratified` narrows that to the ones nobody
// ever granted, which is the wording 0066 gave those rows and the reason the
// two are not one question. There is no `revised`: an artifact that was granted
// and has moved since is what `unratified` reports and what exit 1 is about, so
// asking for it is asking for the finding rather than for a spelling.
var standings = []string{"ratified", "unratified", "never-ratified", "retired", "absent", "raw", "shaped", "committed"}

// narrowRegister applies the two halves of a filter in the order their meanings
// require: the population first, so that an empty one can be told from a state
// nothing is in.
func narrowRegister(f *filter, all []register.Standing) (population, admitted []register.Standing) {
	for _, e := range all {
		if !f.selects(e.Artifact) {
			continue
		}
		population = append(population, e)
		if e.Desire != nil {
			if f.admits(stateOf(e)) {
				admitted = append(admitted, e)
			}
			continue
		}
		if f.admits(string(e.State), standingOf(e)) {
			admitted = append(admitted, e)
		}
	}
	return population, admitted
}
