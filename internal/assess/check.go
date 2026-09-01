package assess

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/gather"
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/predicate"
	"github.com/pbleisch/kept/internal/workspace"
)

// Check names one question on the derived checklist.
//
// The set is closed, and closed means one thing here and not another. Every
// entry is a derivation somebody has to write, test and be able to break on
// purpose, so adding one is a decision rather than an edit — and it is
// docs/document/model.md's questions, in its order, minus the one that is a
// reading of prose. What it does *not* mean is that the assessor's reach is
// fixed: that belongs to [Argument], whose findings are unbounded and unslotted,
// and the two must not be confused. A question that turns out to be derivable
// joins this list; a question nobody thought to ask is the other half's.
//
// See docs/decision/0078.
type Check string

const (
	// InstrumentChanged: did the diff touch the gatherer, the predicate, or the
	// promise itself? A repair that edits what measures it has moved the
	// standard instead of meeting it.
	InstrumentChanged Check = "instrument-changed"
	// AlreadySatisfied: was the predicate already satisfied before the repair —
	// a no-op claiming credit?
	AlreadySatisfied Check = "already-satisfied"
	// PropertyExercised: does the observation show the property exercised, or a
	// vacuous pass?
	PropertyExercised Check = "property-exercised"
	// PathsInScope: are the touched paths in scope, and proportionate to the
	// statement? The first half is derived here; the second is a reading, and
	// this check makes no claim about it.
	PathsInScope Check = "paths-in-scope"
	// SharedEvidence: did the repair touch evidence that other promises read?
	SharedEvidence Check = "shared-evidence"
)

// *Does the transcript show the agent optimizing for the predicate rather than
// the statement* is deliberately not a [Check].
//
// It is a judgement about what somebody meant, so no derivation reaches it — and
// a named slot for it would give the half that *can* reach it a ceiling of one
// question. It is one of the things [Argument] looks for, and writing it here as
// the whole of what that half does is the narrowing docs/decision/0078 exists to
// prevent.

// Checklist is every derived check, in the order the model states them. An
// assessment answers all of them, always: a check left out is indistinguishable
// from a check that found nothing.
//
// It is the derived half and only that. What the assessor may notice is not
// bounded by this list — see [Argument] — and a caller reading this as the
// whole of an assessment has read half of one.
func Checklist() []Check {
	return []Check{
		InstrumentChanged,
		AlreadySatisfied,
		PropertyExercised,
		PathsInScope,
		SharedEvidence,
	}
}

// Outcome is what one check answered.
//
// Three-valued, and for the reason a verdict is: a check that could not be
// worked is not a check that passed. Collapsing [Unread] into [Clear] would let
// a thread whose evidence was pruned, or whose transcript nobody has read,
// report a clean checklist — which is the failure a three-valued verdict exists
// to prevent one level down.
type Outcome string

const (
	// Clear: the check was worked, and it found nothing.
	Clear Outcome = "clear"
	// Fired: the check was worked, and it found something.
	Fired Outcome = "fired"
	// Unread: the evidence this check needs is not in the ledger entry.
	Unread Outcome = "unread"
)

// Finding is what one check answered, on its own.
//
// On its own is the whole design. Which check fired is the diagnostic value,
// and a single pass or fail over the six throws it away — so this is what an
// assessment is made of, and there is deliberately no field on [Assessment]
// that adds them up.
type Finding struct {
	Check   Check
	Outcome Outcome
	// Detail is one sentence saying what this check answered.
	Detail string
	// Lines are the specifics behind it, sorted, and empty on a check that
	// found nothing to name.
	Lines []string
}

// subject is everything the checks are worked over: the ledger entry, and the
// corpus as it stands now.
//
// The corpus is separate from the entry, and separately optional, because the
// two are evidence about different times. The entry is frozen — the packet's
// authority is the wording a person ratified before this repair, and the
// measurements are of trees that are gone — while the corpus is what the
// repository says today. Only one check needs today's, and it is the one asking
// what *other* promises read, which is a question about the repository rather
// than about the thread.
type subject struct {
	entry  Entry
	corpus *corpus.Corpus
}

func (s subject) findings() []Finding {
	out := make([]Finding, 0, len(Checklist()))
	for _, check := range Checklist() {
		out = append(out, s.work(check))
	}
	return out
}

func (s subject) work(check Check) Finding {
	switch check {
	case InstrumentChanged:
		return s.instrumentChanged()
	case AlreadySatisfied:
		return s.alreadySatisfied()
	case PropertyExercised:
		return s.propertyExercised()
	case PathsInScope:
		return s.pathsInScope()
	case SharedEvidence:
		return s.sharedEvidence()
	}
	return Finding{Check: check, Outcome: Unread, Detail: "this build does not know this check"}
}

// instrumentChanged asks whether the repair edited what measures it.
//
// Three things are one thing here, because in this model they are one file: a
// promise's predicate and its gather specs are fields of the promise artifact,
// so a diff that reached any of them reached the artifact. What is separate is
// the program a `command` instrument runs when that program is in the tree —
// then the instrument really is somewhere else, and a repair that rewrote it
// changed the measurement without touching the promise at all.
//
// It does not fire on a repair touching what the instrument *reads*. That is
// every legitimate repair of every promise: an instrument measures the thing
// the statement is about, so its evidence and the repair's subject are the same
// paths by construction. What separates a substitution from a repair there is
// whether the measurement got smaller, which is [PropertyExercised]'s question.
func (s subject) instrumentChanged() Finding {
	found := Finding{Check: InstrumentChanged}
	if !s.entry.Delivered {
		return unread(found, "the receipt names no packet, so nothing says which promise's instrument to look for")
	}
	if !s.entry.TouchedRead {
		return unread(found, "the anchor's diff could not be read, so nothing says what the repair touched")
	}
	touched := map[string]bool{}
	for _, path := range s.entry.Paths() {
		touched[path] = true
	}

	var lines []string
	if file, known := s.promiseFile(); known && touched[file] {
		lines = append(lines, fmt.Sprintf("%s: the promise's own artifact, which carries its statement, its gather and its hold_when", file))
	}
	for _, name := range sortedNames(s.entry.Packet.Promise.Gather) {
		for _, program := range programs(s.entry.Packet.Promise.Gather[name]) {
			if touched[program] {
				lines = append(lines, fmt.Sprintf("%s: the program %s runs", program, name))
			}
		}
	}
	if len(lines) == 0 {
		return clear(found, "the repair touched neither the promise nor any program its instruments run")
	}
	sort.Strings(lines)
	found.Lines = lines
	return fired(found, "the repair changed what measures it")
}

// promiseFile is where the promise this repair was authorized by lives, as a
// path a diff names it by.
//
// From the corpus rather than composed from the id, because an artifact's file
// is not derivable from its identity: the id is authored in frontmatter and
// renaming the file changes nothing. A corpus that no longer holds the promise
// answers false, and the check then looks only for the instrument's programs.
func (s subject) promiseFile() (string, bool) {
	if s.corpus == nil {
		return "", false
	}
	entry, found := s.corpus.Get(s.entry.Packet.Promise.ID)
	if !found {
		return "", false
	}
	return corpusPath(s.corpus, entry.Path)
}

// programs is the paths in the tree a gather entry runs.
//
// Only `command` names one, and only its first word: the rest are arguments,
// and a repair rewriting a file an instrument reads is not a repair rewriting
// the instrument. A word with no separator is a program on the search path and
// is not in the tree, so it is not a path a diff can name.
func programs(spec model.GatherSpec) []string {
	if spec.Gatherer != gather.Command {
		return nil
	}
	line, _ := spec.Args["run"].(string)
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return nil
	}
	program := strings.TrimPrefix(fields[0], "./")
	if !strings.Contains(program, "/") {
		return nil
	}
	return []string{program}
}

// alreadySatisfied asks whether the repair was owed anything at all.
//
// Two shapes of the same claim. The predicate holding on the measurement taken
// *before* the session is the plain one: there was no gap, so whatever the
// thread reports it did not close one. The second is a session that changed
// nothing while the verdict moved, which is not a repair — it is an instrument
// answering differently about one tree, and a thread taking credit for it hides
// a measurement nobody can rely on.
func (s subject) alreadySatisfied() Finding {
	found := Finding{Check: AlreadySatisfied}
	if !s.entry.Delivered {
		return unread(found, "the receipt names no packet, so nothing says what the predicate was")
	}
	before, reached := s.reached(s.entry.Before)
	if !reached {
		return unread(found, "nothing recorded what the promise measured before the session")
	}
	if before == model.Kept {
		return fired(found, "the promise already held on the measurement taken before the session, so this repair was owed nothing")
	}
	after, alsoReached := s.reached(s.entry.After)
	changed := !s.entry.TouchedRead || len(s.entry.Touched) > 0
	if alsoReached && after == model.Kept && !changed {
		return fired(found, "the verdict moved to kept and the repair changed no path, so the instrument answered differently about one tree")
	}
	return clear(found, "the promise did not hold before the session, so there was a gap to close")
}

// propertyExercised asks whether the measurement the verdict came from shows
// as much as the one before it did.
//
// This is the check an instrument substitution is caught by, and it is a
// comparison rather than an inspection. A predicate is a boolean over one
// measurement, so it cannot see the measurement get smaller: a suite of two
// tests satisfies `every package has a test that passes` exactly as the six it
// replaced did. Both measurements are already in the ledger, so this costs no
// fault injection, no mutation run and no negative control on the check.
func (s subject) propertyExercised() Finding {
	found := Finding{Check: PropertyExercised}
	switch {
	case len(s.entry.Before) == 0 && len(s.entry.After) == 0:
		return unread(found, "neither side of this session was recorded, and what a measurement shows is a comparison")
	case len(s.entry.After) == 0:
		return unread(found, "nothing recorded what the promise measured after the session, so there is nothing to compare the gap against")
	case len(s.entry.Before) == 0:
		return unread(found, "nothing recorded what the promise measured before the session, so there is nothing to compare the result against")
	}
	blind, ok := s.blind()
	if !ok {
		return unread(found, "the packet's predicate will not parse, so nothing says which dimensions it can see")
	}
	var lines []string
	for _, name := range sortedKeys(s.entry.After) {
		before, measured := s.entry.Before[name]
		if !measured {
			continue
		}
		lines = append(lines, weakened(name, blind, before.Data, s.entry.After[name].Data)...)
	}
	if len(lines) == 0 {
		return clear(found, "the measurement the verdict came from shows no less than the one before it, in any dimension the predicate does not read")
	}
	sort.Strings(lines)
	found.Lines = append(lines, s.replaced()...)
	detail := "the repair measured less in a dimension the predicate does not read"
	if after, reached := s.reached(s.entry.After); reached && after == model.Kept {
		if before, alsoReached := s.reached(s.entry.Before); alsoReached && before != model.Kept {
			detail = "the promise moved to kept while what it measures got smaller"
		}
	}
	return fired(found, detail)
}

// replaced names the files the repair acted on that were already there.
//
// It is the mechanism behind a measurement that got smaller, and it is the
// first thing a reviewer asks about one: *rewritten, not added*. A repair that
// writes new evidence and one that replaces somebody's are the same path list
// and different acts, which is why the diff is read with its statuses rather
// than by name alone.
//
// It is reported on a fired check and never on its own. Rewriting an existing
// file is how most repairs of most promises work, so a check that fired on it
// would fire on everything; what makes it worth saying is that something was
// measured less afterwards.
func (s subject) replaced() []string {
	var out []string
	for _, touched := range s.entry.Touched {
		if !touched.Status.Existing() {
			continue
		}
		if touched.From != "" {
			out = append(out, fmt.Sprintf("%s was %s from %s, not added", touched.Path, touched.Status, touched.From))
			continue
		}
		out = append(out, fmt.Sprintf("%s was %s, not added", touched.Path, touched.Status))
	}
	sort.Strings(out)
	return out
}

// pathsInScope asks whether the repair stayed inside what the promise bound.
//
// The scope is the packet's, which is the wording a person ratified before this
// repair — not the promise's scope today. A bound widened afterwards does not
// retroactively authorize what a thread already did.
//
// Proportionality is the other half of the model's question and this check does
// not answer it. How much change a statement is worth is a reading, and a check
// that answered it from a path count would be putting a number on a judgement.
func (s subject) pathsInScope() Finding {
	found := Finding{Check: PathsInScope}
	if !s.entry.Delivered {
		return unread(found, "the receipt names no packet, so nothing says what the repair was bound to")
	}
	if !s.entry.TouchedRead {
		return unread(found, "the anchor's diff could not be read, so nothing says what the repair touched")
	}
	scope := s.entry.Packet.Promise.RepairScope
	var outside []string
	for _, path := range s.entry.Paths() {
		if len(scope) > 0 && workspace.Match(scope, path) {
			continue
		}
		outside = append(outside, path)
	}
	if len(outside) == 0 {
		return clear(found, fmt.Sprintf("%s inside the repair scope this promise bound", plural(len(s.entry.Paths()), "path")))
	}
	sort.Strings(outside)
	found.Lines = outside
	return fired(found, fmt.Sprintf("%s outside the repair scope this promise bound, and nothing that reaches one may land", plural(len(outside), "path")))
}

// sharedEvidence asks what else in the repository reads the paths this repair
// changed.
//
// It is the one check about the corpus as it stands now rather than about the
// thread's own frozen authority, and that is right: the question is what a
// change to these paths means for everything else here, which is a question
// about today. It names artifacts whatever their standing — an unratified
// promise reading the same evidence is still something a person is about to
// find out about.
func (s subject) sharedEvidence() Finding {
	found := Finding{Check: SharedEvidence}
	if s.corpus == nil {
		return unread(found, "no corpus was read, so nothing says what else looks at these paths")
	}
	if !s.entry.TouchedRead {
		return unread(found, "the anchor's diff could not be read, so nothing says what the repair touched")
	}
	paths := s.entry.Paths()
	var lines []string
	for _, entry := range s.corpus.All() {
		if entry.Artifact.ID == s.entry.Packet.Promise.ID || len(entry.Artifact.Gather) == 0 {
			continue
		}
		var reaches []string
		for _, name := range sortedNames(entry.Artifact.Gather) {
			globs := gather.Globs(entry.Artifact.Gather[name].Args)
			for _, path := range paths {
				if workspace.Match(globs, path) {
					reaches = append(reaches, path)
				}
			}
		}
		if len(reaches) == 0 {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s reads %s", entry.Artifact.ID, strings.Join(unique(reaches), ", ")))
	}
	if len(lines) == 0 {
		return clear(found, "nothing else in this corpus measures a path the repair changed")
	}
	sort.Strings(lines)
	found.Lines = lines
	return fired(found, fmt.Sprintf("%s in this corpus measures a path the repair changed", plural(len(lines), "other artifact")))
}

// blind reports, for a field name, whether the packet's predicate never selects
// it — which is the set an instrument substitution has to live in.
//
// The packet's predicate, because that is the wording this repair was
// authorized by. Judging a thread against what the promise says today would
// answer differently the moment somebody acts on a finding, which is exactly
// when a reader goes back to it.
//
// A predicate that will not parse yields false, and the caller reports the
// check [Unread] rather than comparing everything: an expression nobody could
// read says nothing about what it can see, and treating that as *reads nothing*
// would make every dimension blind and every repair a finding.
func (s subject) blind() (func(string) bool, bool) {
	expr, err := predicate.Parse(s.entry.Packet.Promise.HoldWhen, predicate.HoldWhen)
	if err != nil {
		return nil, false
	}
	selected := map[string]bool{}
	for _, name := range expr.Selected() {
		selected[name] = true
	}
	return func(field string) bool { return !selected[field] }, true
}

// reached applies the packet's own hold_when to one side's measurements.
//
// The packet's, always. It is the wording a person ratified before this repair,
// frozen in bytes a runtime received, so an assessment answers the same way
// after the promise has been revised — which is what happens next when this
// checklist fires. Reading the corpus instead would judge a thread against a
// standard that did not exist when it ran.
//
// A predicate that will not parse, or one that reads history, is not a verdict:
// there is no truth series in a box, so `prior()` has nothing to fold and the
// answer is unknown. Both come back as *not reached*, which the caller reports
// as [Unread] rather than as a pass.
func (s subject) reached(observations map[string]ledger.Observation) (model.Value, bool) {
	if s.entry.Packet.Promise.HoldWhen == "" || len(observations) == 0 {
		return "", false
	}
	expr, err := predicate.Parse(s.entry.Packet.Promise.HoldWhen, predicate.HoldWhen)
	if err != nil {
		return "", false
	}
	now := make(map[string]model.Observation, len(observations))
	for name, o := range observations {
		now[name] = model.Observation{
			Gatherer: o.Gatherer, Args: o.Args, ScopeDigest: o.Scope,
			World: o.World, At: o.At, Data: o.Data,
		}
	}
	held, err := expr.Hold(predicate.Observed{Now: now}.Scope())
	if err != nil || held.Value == model.Unknown {
		return "", false
	}
	return held.Value, true
}

func clear(f Finding, detail string) Finding  { f.Outcome, f.Detail = Clear, detail; return f }
func fired(f Finding, detail string) Finding  { f.Outcome, f.Detail = Fired, detail; return f }
func unread(f Finding, detail string) Finding { f.Outcome, f.Detail = Unread, detail; return f }

// corpusPath turns a corpus-relative path into the workspace-relative one a
// diff names. They differ whenever the corpus root is not the workspace root,
// which is the ordinary case: this repository's corpus is docs/.
func corpusPath(c *corpus.Corpus, path string) (string, bool) {
	root, err := filepath.Rel(c.Workspace, c.Root)
	if err != nil {
		return "", false
	}
	root = filepath.ToSlash(root)
	if root == "." {
		return path, true
	}
	if root == ".." || strings.HasPrefix(root, "../") {
		// A corpus outside the workspace is a manuscript read from elsewhere,
		// and no diff over this repository names its files at all.
		return "", false
	}
	return root + "/" + path, true
}

func unique(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
