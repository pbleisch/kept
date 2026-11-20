package assess

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/thread"
)

// defaultDeadline bounds the one external call an assessment makes, which is
// reading the anchor's own diff. Nothing external runs without a deadline. See
// docs/decision/0026.
const defaultDeadline = 2 * time.Minute

// Status is how the repair touched one path, as git reports it.
//
// It is read rather than inferred, and the reason is the same one
// [box.Change] gives for reading names from `--name-only` instead of the diff
// text: *rewritten* and *added* are the difference between a repair that wrote
// new evidence and one that replaced somebody's, and a path list that carries
// only the name cannot tell them apart.
type Status string

const (
	Added    Status = "added"
	Modified Status = "modified"
	Deleted  Status = "deleted"
	Renamed  Status = "renamed"
	Copied   Status = "copied"
	Retyped  Status = "type changed"
	Unclear  Status = "unclear"
)

// Existing reports whether this status is one that acted on a file that was
// already there.
func (s Status) Existing() bool {
	switch s {
	case Modified, Deleted, Renamed, Retyped:
		return true
	}
	return false
}

// Touched is one path the repair changed, and how.
type Touched struct {
	Path   string
	Status Status
	// From is the path a rename or a copy came from, and is empty otherwise.
	From string
}

// Entry is one thread's ledger entry, as an assessment reads it.
//
// Everything in it is durable and none of it is re-derived by measuring
// anything: the packet is the frozen authority and evidence a runtime received,
// the two measurements are addressed in the observation stream, and the diff is
// the anchor commit's own against its parent. That is what makes an assessment
// of a thread answer the same way after the promise it was about has been
// revised — which is the ordinary next step when one fires.
type Entry struct {
	Thread thread.ID

	// Packet is what the session was given, and Delivered reports whether the
	// receipt named one at all. A thread with no packet is not a thread with an
	// empty one.
	Packet    thread.Contents
	Delivered bool

	// Before and After are the promise's measurements on either side of the
	// session, keyed by the name the promise gathers each instrument under.
	// Either may be absent: a session killed at its deadline leaves no
	// re-derivation, and a receipt written before measurements were recorded
	// names neither.
	Before, After map[string]ledger.Observation

	// Anchor is the commit the session left, and Anchored reports whether there
	// is one. A repair that changed nothing anchors nothing.
	Anchor   thread.Anchor
	Anchored bool

	// Touched is the anchor's own diff against its parent, sorted by path, and
	// Read reports whether it could be taken at all. A repository that no
	// longer holds the anchor answers false rather than empty: *the diff was
	// not read* and *the repair changed nothing* are different facts and only
	// the second is about the repair.
	Touched     []Touched
	TouchedRead bool

	// Transcripts is how many transcript fragments the receipt holds. It is a
	// count rather than the text: what reads a transcript is a session, and the
	// count is what says whether there is anything for one to read.
	Transcripts int

	// Lifecycle is what the parent observed of the child process, in order.
	Lifecycle []thread.Lifecycle

	// Arguments is every adversarial pass this thread holds, oldest first, and
	// Argued is what the last one said. ArguedModel is the model that session
	// reported serving it.
	//
	// They are read rather than derived, which is the structural difference
	// between the assessor's two halves: a derived finding is worked out on read
	// because it is reproducible, and a pass is not, so its words are durable
	// and this is where they are found. See
	// policy:durability-follows-reproducibility and docs/decision/0078.
	Arguments   []thread.ArgumentRecord
	Argued      string
	ArguedModel string

	// Quarantined are the receipt lines that could not be read. They are
	// reported and never repaired, and an assessment that ignored them would be
	// reading a partial entry as a whole one.
	Quarantined []ledger.Quarantined

	// Session is how far the repair session got, derived from the receipt, and
	// SessionRead says there were entries to derive it from. It is presentation
	// for a person reading a thread and nothing here checks it. See
	// docs/decision/0085.
	Session     thread.Diagnosis
	SessionRead bool
}

// Read assembles one thread's ledger entry.
//
// It reads and never measures. A missing piece is absent rather than an error:
// a thread killed before it anchored, one whose observations were pruned, and
// one whose repository no longer holds the commit are all entries a checklist
// can still be worked over, and each check says for itself which evidence it
// needed.
//
// The one thing it refuses is a thread with no receipt at all, because there is
// then no act to attach a verification to.
func Read(ctx context.Context, store ledger.Store, repository string, id thread.ID, deadline time.Duration) (Entry, error) {
	entries, quarantined, err := receiptOf(store, id)
	if err != nil {
		return Entry{}, err
	}
	return entryOf(ctx, store, repository, id, entries, quarantined, deadline), nil
}

// receiptOf reads a thread's receipt, and answers a thread nothing wrote about
// as one that does not exist here.
func receiptOf(store ledger.Store, id thread.ID) ([]thread.Entry, []ledger.Quarantined, error) {
	entries, quarantined, err := thread.ReadReceipt(store, id)
	if errors.Is(err, ledger.ErrNoStream) {
		// A stream that is not there and a stream with nothing in it are the
		// same answer to this question, and it is not an error about a file: it
		// is *this thread does not exist here*, which a person reads as a
		// mistyped id and a script reads as a question that was never put.
		return nil, nil, noReceipt(id)
	}
	if err != nil {
		return nil, nil, err
	}
	if len(entries) == 0 && len(quarantined) == 0 {
		return nil, nil, noReceipt(id)
	}
	return entries, quarantined, nil
}

// entryOf assembles one thread's entry out of receipt entries already read.
//
// The entries may be a prefix of the receipt — the episode as it stood when one
// attempt was done — and everything here is then the entry as it stood then:
// the latest packet and anchor in the prefix, the latest re-derivation, and the
// passes over the last attempt in it. The before side is the episode's first,
// whatever the prefix, because the gap a person asked about is the one the first
// box measured; a rework's own box may measure the promise already holding, and
// judging the episode's act against that would call every rework a no-op. See
// docs/decision/0086.
func entryOf(ctx context.Context, store ledger.Store, repository string, id thread.ID, entries []thread.Entry, quarantined []ledger.Quarantined, deadline time.Duration) Entry {
	out := Entry{Thread: id, Quarantined: quarantined}
	// How far the repair session got, derived from the same entries. A receipt
	// with no packet is still a session a person may need to read about, so it
	// is diagnosed whole. See docs/decision/0085.
	session, found := thread.RepairSession(entries)
	if !found {
		session = thread.Session{Entries: entries}
	}
	out.Session, out.SessionRead = thread.Diagnose(session), len(entries) > 0
	var first thread.Contents
	for _, entry := range entries {
		switch {
		case entry.Packet != nil:
			src, err := ledger.ReadArtifact(store, *entry.Packet)
			if err != nil {
				continue
			}
			contents, err := thread.ReadPacket(src)
			if err != nil {
				continue
			}
			if !out.Delivered {
				first = contents
			}
			out.Packet, out.Delivered = contents, true
		case entry.Transcript != nil:
			out.Transcripts++
		case entry.Lifecycle != nil:
			out.Lifecycle = append(out.Lifecycle, *entry.Lifecycle)
		}
	}
	if anchor, found := thread.LatestAnchor(entries); found {
		out.Anchor, out.Anchored = anchor, true
	}
	// The passes over the last attempt, and only those. A thread whose latest
	// attempt was never argued over — no assessor configured, or a pass that
	// could not start — must not be reported with the argument about the
	// attempt before it, which is about a tree the anchor no longer names.
	var passes []thread.Session
	if attempts := thread.Attempts(entries); len(attempts) > 0 {
		passes = attempts[len(attempts)-1].Passes
	} else {
		for _, session := range thread.Sessions(entries) {
			if session.Kind == ledger.ThreadBrief {
				passes = append(passes, session)
			}
		}
	}
	out.Arguments, out.Argued, out.ArguedModel = argumentsOver(store, passes)
	out.Before, out.After = measurements(store, thread.Measurements(entries))
	// The after side is the re-derivation of the tree the anchor in force
	// names, which is the attempt that left it. A later attempt that changed
	// nothing measured that same tree, and one whose work was not anchored
	// because it regressed (docs/decision/0090) measured a tree a land would
	// never take — so judging the anchor by that attempt's measurement would be
	// judging work by a tree it is not.
	if left, found := anchoring(entries); found {
		if _, after := measurements(store, left.After()); len(after) > 0 {
			out.After = after
		}
	}
	if len(out.Before) == 0 && out.Delivered {
		// The packet's own bytes hold the measurement that found the gap, which
		// is the before side by construction: it is what the re-derivation in
		// the box returned and what the packet was assembled from. So a receipt
		// written before a repair named its measurements still has one side to
		// read, and every thread in every ledger is assessable rather than only
		// the ones written since. It is not a second source for the same fact —
		// a receipt that names the address wins, and this is the same
		// measurement read out of the record that already carried it.
		out.Before = maps.Clone(first.Observations)
	}
	if out.Anchored && repository != "" {
		if touched, err := diff(ctx, repository, out.Anchor, deadline); err == nil {
			out.Touched, out.TouchedRead = touched, true
		}
	}
	return out
}

// anchoring is the last attempt in a receipt whose own session left an anchor,
// and whether there is one.
func anchoring(entries []thread.Entry) (thread.Attempt, bool) {
	attempts := thread.Attempts(entries)
	for i := len(attempts) - 1; i >= 0; i-- {
		if _, anchored := attempts[i].Anchor(); anchored {
			return attempts[i], true
		}
	}
	return thread.Attempt{}, false
}

// argumentsOver is every argument the given passes filed, oldest first, the
// last one's words, and the model that answered the last pass that filed one.
//
// The model is read from that pass's own session rather than from the thread's
// last projection. A thread holds the repairer's reading too, and a pass that
// reported none would otherwise be labelled with the model that repaired — which
// is the one thing a provenance field must not do.
func argumentsOver(store ledger.Store, passes []thread.Session) ([]thread.ArgumentRecord, string, string) {
	var (
		records []thread.ArgumentRecord
		said    string
		model   string
	)
	for _, pass := range passes {
		filed := thread.Arguments(pass.Entries)
		if len(filed) == 0 {
			continue
		}
		records = append(records, filed...)
		// The pass's own words. A pass whose artifact the store no longer holds
		// leaves the record and loses the text, which reads as a pass that ran
		// and said nothing findable — the same shape a pruned observation leaves
		// a check, and honest for the same reason.
		said = ""
		if src, err := ledger.ReadArtifact(store, filed[len(filed)-1].Artifact); err == nil {
			said = string(src)
		}
		model = ""
		if answered, found := thread.LatestModel(pass.Entries); found {
			model = answered.Model
		}
	}
	return records, said, model
}

// noReceipt is what a thread nothing wrote about is answered with.
//
// A checklist over one would answer six rows of nothing, which reads exactly
// like a repair nobody found fault with. There is no act here to attach a
// verification to, so this says so instead of assessing an absence.
func noReceipt(id thread.ID) error {
	return fmt.Errorf("thread %s has no receipt, so there is no act to assess; kept review lists the threads there are", id)
}

// measurements resolves the addresses a receipt named into the measurements
// themselves, by side.
//
// A measurement that cannot be read is absent rather than fatal, which is what
// prunability already costs everywhere else: an observation is re-gatherable
// against the world it names, so losing one costs a check its evidence and
// never the whole entry. The check that needed it reads [Unread].
//
// The before side is the first measurement of each instrument and the after side
// is the last. An episode's first box measured the gap a person asked about, and
// its latest re-derivation is what the episode's act left; a rework's own box
// sits between them and is neither. For a thread of one attempt the first and
// the last are the same record. See docs/decision/0086.
func measurements(store ledger.Store, named []thread.Measurement) (before, after map[string]ledger.Observation) {
	before, after = map[string]ledger.Observation{}, map[string]ledger.Observation{}
	for _, m := range named {
		if _, taken := before[m.Name]; taken && m.Side == thread.Before {
			continue
		}
		observation, err := ledger.ReadObservation(store, m.Address)
		if err != nil {
			continue
		}
		switch m.Side {
		case thread.Before:
			before[m.Name] = observation
		case thread.After:
			after[m.Name] = observation
		}
	}
	return before, after
}

// status is git's own single-letter answer, mapped to the word a person reads.
var statuses = map[byte]Status{
	'A': Added, 'M': Modified, 'D': Deleted, 'R': Renamed, 'C': Copied, 'T': Retyped,
}

// score matches a --name-status field: a letter, optionally followed by the
// similarity percentage a rename or a copy carries.
var score = regexp.MustCompile(`^[A-Z][0-9]*$`)

// diff is the anchor commit's own change against its parent.
//
// The anchor is a pair and this is why: the commit's diff against the commit
// the box was cut from is exactly the change a person reviewed, so nothing here
// has to know what a merge base is or what the landing target has done since.
// See docs/decision/0067.
//
// -z rather than the default, because git quotes and escapes a path with a
// special character otherwise, and an unquoted reading of that would compare
// the wrong string against a scope. --no-renames is deliberately not passed:
// a rename is how an instrument's evidence gets replaced without a deletion
// showing up, and it is exactly what a checklist wants to see.
func diff(ctx context.Context, repository string, anchor thread.Anchor, deadline time.Duration) ([]Touched, error) {
	if deadline <= 0 {
		deadline = defaultDeadline
	}
	call, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()
	command := exec.CommandContext(call, "git", "-C", repository,
		"diff", "--name-status", "-z", "--no-ext-diff", "--no-textconv", "--end-of-options",
		anchor.Parent, anchor.Commit)
	command.Env = withoutWorktreeEnv()
	var stdout, stderr strings.Builder
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	if call.Err() != nil {
		return nil, fmt.Errorf("read the anchor's diff: %w", call.Err())
	}
	if err != nil {
		return nil, fmt.Errorf("read the anchor's diff: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return parseStatus(stdout.String())
}

// Diff is the anchor commit's own change against its parent, as text.
//
// It is the same question [diff] asks and a different answer: that one is the
// path list every derived check compares against a scope, and this is the change
// itself, which no derivation reads and an adversarial pass does. Both are the
// commit's diff against the commit the box was cut from, so neither has to know
// what a merge base is or what the landing target has done since
// (docs/decision/0067).
//
// It is here rather than in the caller so that the two are one command with one
// set of flags: -z is deliberately absent because this is text a person and a
// model read rather than a field list something parses, and --no-ext-diff and
// --no-textconv are present for the reason they are there — a repository's own
// configured external differ is not something kept puts in front of a runtime.
func Diff(ctx context.Context, repository string, anchor thread.Anchor, deadline time.Duration) (string, error) {
	if repository == "" {
		return "", errors.New("no repository to read the anchor's diff from")
	}
	if why := anchorWhy(anchor); why != "" {
		return "", fmt.Errorf("read the anchor's diff: %s", why)
	}
	if deadline <= 0 {
		deadline = defaultDeadline
	}
	call, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()
	command := exec.CommandContext(call, "git", "-C", repository,
		"diff", "--no-ext-diff", "--no-textconv", "--end-of-options", anchor.Parent, anchor.Commit)
	command.Env = withoutWorktreeEnv()
	var stdout, stderr strings.Builder
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	if call.Err() != nil {
		return "", fmt.Errorf("read the anchor's diff: %w", call.Err())
	}
	if err != nil {
		return "", fmt.Errorf("read the anchor's diff: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// anchorWhy is the little that has to be true before git is asked about an
// anchor. internal/thread refuses the same shape on the way into a receipt; this
// is the reader's half, so that a caller holding a zero anchor is told what is
// missing rather than handed git's own complaint about an empty revision.
func anchorWhy(anchor thread.Anchor) string {
	switch {
	case anchor.Commit == "":
		return "the anchor names no commit"
	case anchor.Parent == "":
		return "the anchor names no parent"
	}
	return ""
}

// parseStatus reads git's NUL-separated --name-status stream.
//
// A rename and a copy carry two paths where every other status carries one, so
// the fields cannot be read in pairs. Reading them in pairs is the mistake that
// makes the path after a rename look like the status of the next entry.
func parseStatus(out string) ([]Touched, error) {
	fields := strings.Split(out, "\x00")
	var touched []Touched
	for i := 0; i < len(fields); i++ {
		field := fields[i]
		if field == "" {
			continue
		}
		if !score.MatchString(field) {
			return nil, fmt.Errorf("git named %q where a change status was expected", field)
		}
		status, known := statuses[field[0]]
		if !known {
			status = Unclear
		}
		takes := 1
		if status == Renamed || status == Copied {
			takes = 2
		}
		if i+takes >= len(fields) {
			return nil, errors.New("git named a change status with no path after it")
		}
		if takes == 2 {
			touched = append(touched, Touched{Path: fields[i+2], Status: status, From: fields[i+1]})
		} else {
			touched = append(touched, Touched{Path: fields[i+1], Status: status})
		}
		i += takes
	}
	sort.Slice(touched, func(a, b int) bool { return touched[a].Path < touched[b].Path })
	return touched, nil
}

// Paths is every path the repair touched, as a scope review compares them.
//
// A rename's origin is one of them. A repair that renamed a file out of a scope
// touched both places, and a review that saw only the destination would report
// a change to somebody else's evidence as untouched.
func (e Entry) Paths() []string {
	var out []string
	for _, t := range e.Touched {
		out = append(out, t.Path)
		if t.From != "" {
			out = append(out, t.From)
		}
	}
	sort.Strings(out)
	return out
}

// withoutWorktreeEnv drops the variables that would point a command at
// whichever worktree the caller happened to inherit, which is never the one it
// was given.
func withoutWorktreeEnv() []string {
	src := os.Environ()
	out := make([]string, 0, len(src))
	for _, value := range src {
		switch name, _, _ := strings.Cut(value, "="); name {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR", "GIT_OBJECT_DIRECTORY":
		default:
			out = append(out, value)
		}
	}
	return out
}
