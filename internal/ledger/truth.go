package ledger

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/pbleisch/kept/internal/model"
)

// Verdict is what a truth record is: a verdict reached about one content
// version of one artifact, in one world.
const Verdict Kind = "verdict"

// TruthDir and DispositionDir are the two per-artifact streams. Both are one
// file per artifact, which is docs/decision/0011's keying rule: two runs
// recording about different artifacts never touch the same file, a diff shows
// which promise moved, and pruning one artifact's history does not rewrite
// everyone else's.
const (
	TruthDir       = "truth"
	DispositionDir = "disposition"
)

// Truth is a verdict transition. Transitions, not ticks: a verdict is appended
// when it changes, plus a heartbeat so staleness is visible — which is also
// what makes max_age terminate, since an observation's timestamp never advances
// for an unchanging world.
//
// It carries its world as a **field**, and the fold filters. A path component
// per world was the other option and it mints tracked files without bound: a
// world is identified by a tree, a working tree changes on every save, and
// every one of those files would be permanent and hold one or two lines. The
// artifact is the thing that is named once, and it is already the key. See
// docs/decision/0035.
type Truth struct {
	Envelope

	// Artifact is "type:slug", the authored identity.
	Artifact string `json:"artifact"`
	// Hash is the content version the verdict was about, and Norm the rules it
	// was computed under. A revision starts a new series: the earlier verdicts
	// concerned a different statement.
	Hash string `json:"hash"`
	Norm int    `json:"norm,omitempty"`

	// World is the tree the verdict was reached in. A truth series is per
	// (Hash, World) and the fold filters by it, because a verdict reached in a
	// repair's worktree and one reached on the tree that ships are about
	// different things.
	World model.World `json:"world"`

	Value model.Value `json:"value"`
	// From are the observations the predicate read, by address. They are what
	// pins those observations against retention.
	From []string `json:"from,omitempty"`
	// Detail is the human-facing reason, drawn from the observation — for an
	// unknown, the field nobody could answer for.
	Detail string `json:"detail,omitempty"`
}

// ThreadVerdict is a verdict reached inside a box, offered to an artifact's
// truth stream.
//
// It is refused rather than recorded. The thread loop gathers against two
// worlds and only one of them is about the artifact: the gate's gather, on the
// tree that ships. The in-box gather answers whether the repair worked on its
// own terms, which is evidence about the repair, so it belongs on the thread —
// where it is pruned with the thread rather than accumulating forever beside
// the answers a person reads. See docs/decision/0035 and 0020.
type ThreadVerdict struct {
	Artifact string
	Thread   string
}

func (e *ThreadVerdict) Error() string {
	return fmt.Sprintf("a verdict about %s was reached in thread %s's box, and belongs on the thread rather than in the artifact's truth", e.Artifact, e.Thread)
}

// TruthPath is where an artifact's verdicts live: truth/{type}/{slug}.jsonl.
func TruthPath(artifact string) (string, error) { return streamPath(TruthDir, artifact) }

// streamPath turns "promise:core-packages-are-tested" into a per-artifact
// stream path. The slug is authored, so it is checked rather than trusted: a
// slug carrying a separator would name a file outside the stream it belongs to.
func streamPath(dir, artifact string) (string, error) {
	kind, slug, found := strings.Cut(artifact, ":")
	if !found || kind == "" || slug == "" {
		return "", fmt.Errorf("%q is not an artifact id of the form type:slug", artifact)
	}
	for _, part := range []string{kind, slug} {
		if strings.ContainsAny(part, "/\\") || part == "." || part == ".." {
			return "", fmt.Errorf("%q cannot name a stream", artifact)
		}
	}
	return path.Join(dir, kind, slug+".jsonl"), nil
}

// Reached records a verdict.
//
// It records what has already happened: the gatherers ran, the predicate was
// applied, and this is what it said. See docs/decision/0018.
func (l *Ledger) Reached(t Truth) (Truth, error) {
	if t.Artifact == "" {
		return Truth{}, errors.New("a verdict names no artifact")
	}
	if t.Hash == "" {
		// A verdict about no content version could not be superseded by a
		// revision, so it would go on applying to a statement nobody wrote.
		return Truth{}, errors.New("a verdict names no content version, so a revision would not start a new series")
	}
	if t.World.Tree == "" {
		return Truth{}, errors.New("a verdict names no world, and a record written without one cannot be backfilled")
	}
	if t.World.Thread != "" {
		return Truth{}, &ThreadVerdict{Artifact: t.Artifact, Thread: t.World.Thread}
	}
	switch t.Value {
	case model.Kept, model.NotKept, model.Unknown:
	default:
		return Truth{}, fmt.Errorf("%q is not a verdict", t.Value)
	}

	where, err := TruthPath(t.Artifact)
	if err != nil {
		return Truth{}, err
	}

	l.seq++
	t.V = Version
	t.Kind = Verdict
	t.At = time.Now().UTC()
	t.Writer = l.run.ID
	t.Seq = l.seq

	line, err := json.Marshal(t)
	if err != nil {
		return Truth{}, err
	}
	if err := l.declare(); err != nil {
		return Truth{}, err
	}
	if err := l.store.Append(where, line); err != nil {
		return Truth{}, err
	}
	l.run.Transitions++
	return t, nil
}

// ReadTruth returns an artifact's verdicts, folded into the order they were
// reached in, alongside every line that could not be read.
//
// It returns every world's records. Filtering is [InWorld] and it is a separate
// call on purpose: a fold that forgets to filter is silently wrong and looks
// correct in every single-run test, so the filter is a thing a reader has to
// name rather than a default they can inherit.
func ReadTruth(store Store, artifact string) ([]Truth, []Quarantined, error) {
	where, err := TruthPath(artifact)
	if err != nil {
		return nil, nil, err
	}
	src, err := store.Read(where)
	if err != nil {
		if errors.Is(err, ErrNoStream) {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	records, quarantined := parse(where, src, truthWhy)
	fold(records)
	return records, quarantined, nil
}

func truthWhy(t Truth) string {
	switch {
	case t.Kind != Verdict:
		return fmt.Sprintf("kind %q is not a verdict", t.Kind)
	case t.Artifact == "":
		return "names no artifact"
	case t.Hash == "":
		return "names no content version, so nothing says which statement it was about"
	case t.World.Tree == "":
		// docs/decision/0014: nothing else records which tree was measured, so
		// a verdict without one cannot be compared with any other.
		return "names no world, so it cannot be told from a verdict reached in another tree"
	case t.Value != model.Kept && t.Value != model.NotKept && t.Value != model.Unknown:
		return fmt.Sprintf("%q is not a verdict this build knows", t.Value)
	}
	return ""
}

// InWorld narrows a folded series to the world it is being asked about.
//
// This is the filter the cost of the field is paid for. Without it a verdict
// reached in a repair's worktree and one reached on the tree that ships fold
// into one series and the answer is whichever was written last, which is the
// third of the failures docs/decision/0014 was written to stop.
func InWorld(records []Truth, world model.World) []Truth {
	var out []Truth
	for _, t := range records {
		if t.World.Same(world) {
			out = append(out, t)
		}
	}
	return out
}

// Latest is the verdict in force for one content version in one world, and
// whether there is one at all. Never observed and observed-and-unknown are
// different answers and a caller has to be able to tell them apart.
func Latest(records []Truth, hash string, world model.World) (Truth, bool) {
	var held Truth
	var found bool
	for _, t := range records {
		if t.Hash != hash || !t.World.Same(world) {
			continue
		}
		held, found = t, true
	}
	return held, found
}

// Moved reports whether a verdict is a transition rather than a repetition.
//
// Truth records transitions, not ticks: a promise kept for a month is a handful
// of lines. The other reason to append is the heartbeat, which is a question
// about age and belongs to whoever knows max_age.
func (t Truth) Moved(previous Truth, found bool) bool {
	return !found || previous.Value != t.Value
}
