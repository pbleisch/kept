package tick

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"github.com/pbleisch/kept/internal/canonical"
	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/gather"
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/predicate"
	"github.com/pbleisch/kept/internal/register"
	"github.com/pbleisch/kept/internal/workspace"
)

// Options are what the operator's config and the command line say about one
// pass.
type Options struct {
	// All forces every artifact to be gathered whatever its scope says, and
	// forces every file to be read whatever the stat cache says. It is what an
	// operator runs after changing a gatherer, and the answer to the one
	// failure the cache has that nothing else reports. See docs/decision/0033.
	All bool
	// Deadline bounds each gather. Zero is gather.DefaultDeadline and never
	// unbounded. See docs/decision/0026.
	Deadline time.Duration
	// Provider is the confinement provider named in config. Empty is unset,
	// and unset is not none. See docs/decision/0017.
	Provider string
	// Exclude is what this workspace excludes beyond .git, .kept and its
	// ignore files.
	Exclude []string
	// Workers is how many files the walk hashes at once.
	Workers int
}

// Tick is one pass over a corpus: what to read it from, what to measure with,
// and where to record what happened.
type Tick struct {
	Corpus    *corpus.Corpus
	Store     ledger.Store
	Catalogue gather.Catalogue
	Options   Options

	// Ledger is where a pass that observes records what it found. A [Survey]
	// leaves it nil, and a survey writes nothing anywhere — which is what lets
	// `kept why` be run against a read-only checkout.
	Ledger *ledger.Ledger
}

// Answer is the verdict in force for one artifact, and where it came from.
//
// Value is a three-valued verdict and never a boolean, and it is empty only
// when nothing has ever answered — never observed and observed-and-unknown are
// different answers, and a caller that cannot tell them apart reports a machine
// that could not measure as a promise that is broken.
type Answer struct {
	Value  model.Value
	Detail string
	// Since is when this verdict was first reached, which is what "not-kept
	// since Tuesday" means. It is the transition, not the last tick.
	Since time.Time
	// World is the tree the verdict was reached in.
	World model.World
	// From are the observations the predicate read, by address.
	From []string
	// Fresh reports whether this answer was reached against the tree this pass
	// walked. A carried answer is not fresh and is still in force: nothing the
	// promise looks at has moved since it was reached.
	Fresh bool
	// Recorded reports whether this pass appended a truth record.
	Recorded bool
	// Gathered reports whether this pass ran the artifact's gatherers.
	Gathered bool
}

// Reached reports whether there is any verdict at all.
func (a Answer) Reached() bool { return a.Value != "" }

// Subject is one artifact as the tick sees it: what the register says, what the
// truth stream says, what this pass measured, and what it did about the result.
type Subject struct {
	ID string
	// Entry is the artifact as the corpus read it, and is nil for an artifact
	// the ledger names and the corpus does not.
	Entry    *corpus.Entry
	Standing register.Standing

	Answer Answer
	// Truth and Dispositions are the artifact's own streams, folded into the
	// order they happened in. Every world's, which is deliberate for the
	// second: `kept why` answers about the queue. See docs/decision/0035.
	Truth        []ledger.Truth
	Dispositions []ledger.Disposition

	// Due reports whether this artifact's answer needed re-deriving, and
	// Because says which of 0033's three reasons it was.
	Due     bool
	Because string

	// Drift is how truth-seeking failed for an artifact that has standing, and
	// Drifting reports whether there is any. Nothing creates drift and nothing
	// resolves it: it is a query result, computed here and never written down.
	Drift    model.Drift
	Drifting bool

	// Disposition is what this pass did about that drift, and Disposed reports
	// whether it appended a record for it — a repetition of the same refusal
	// is the heartbeat's business, not a transition's.
	Disposition ledger.Disposition
	Disposed    bool

	// Unevaluated is why no verdict was reached at all, when the reason is
	// neither a measurement nor an absence of one: a hold_when that will not
	// parse, or a gather entry naming a gatherer nothing answers to. It is
	// **not** an unknown — an expression that is wrong is not a verdict, and
	// recording one would say nobody measured when somebody mistyped. See
	// docs/decision/0036.
	Unevaluated string
}

// Predicated reports whether this artifact seeks truth at all. One that
// declares no gatherer and no predicate binds without being evaluated — most
// canon does — so it is never drift and nothing is ever recorded about it.
func (s Subject) Predicated() bool {
	return s.Entry != nil && len(s.Entry.Artifact.Gather) > 0 && s.Entry.Artifact.HoldWhen != ""
}

// Result is one pass, whole.
type Result struct {
	// World is the tree this pass walked.
	World model.World
	Tree  *workspace.Tree
	// Subjects are every artifact the corpus holds and every one the ledger
	// names, sorted by id.
	Subjects []Subject
	// Quarantined is every ledger line that could not be read, from every
	// stream this pass touched. Reported and never repaired.
	Quarantined []ledger.Quarantined
	// Observed is how many measurements this pass recorded.
	Observed int
}

// Survey answers from the ledger and the tree, and measures nothing.
//
// It is what `kept drift` and `kept why` run: both are questions about what is
// already known, and a question that gathered would make reading the queue cost
// a test suite. Nothing here writes, so it runs against a read-only checkout.
func (t *Tick) Survey() (*Result, error) { return t.pass(context.Background(), false) }

// Run gathers what is due, evaluates it, and records both the verdict and what
// it did about any drift.
//
// Observe, act, then record: every write here is retrospective. A crash may
// lose the record of something that happened and can never produce a record of
// something that did not. See docs/decision/0018.
func (t *Tick) Run(ctx context.Context) (*Result, error) {
	if t.Ledger == nil {
		return nil, errors.New("a tick that records needs a ledger to record into")
	}
	return t.pass(ctx, true)
}

func (t *Tick) pass(ctx context.Context, observing bool) (*Result, error) {
	// The stat cache is derived, disposable and this package's own business:
	// losing it costs one slow tick and can never corrupt a record, so nothing
	// upstream has to be told it exists. See docs/decision/0033.
	cache := workspace.OpenCache(filepath.Join(t.Corpus.Workspace, corpus.ConfigDir))
	tree, err := workspace.Read(t.Corpus.Workspace, workspace.Options{
		Exclude: t.Options.Exclude,
		Cache:   cache,
		Workers: t.Options.Workers,
		All:     t.Options.All,
	})
	if err != nil {
		return nil, err
	}

	// The world is the tree, and only the tree. Remote, ref and commit are
	// provenance filled in from git where git is present, and nothing fills
	// them in yet; they are not part of what makes one tree one tree, so a
	// record written without them is not a record written without a world. See
	// docs/decision/0033.
	res := &Result{World: model.World{Tree: tree.Digest()}, Tree: tree}

	decisions, quarantined, err := ledger.ReadDecisions(t.Store)
	if err != nil {
		return nil, err
	}
	res.Quarantined = append(res.Quarantined, quarantined...)

	// One runner for the whole pass, so two promises naming the same gatherer
	// over the same scope share the measurement. They must: a gather runs a
	// program, and the second run would cost the same again to produce a record
	// content addressing would immediately discard as a duplicate.
	runner := gather.NewRunner(t.Catalogue)

	for _, standing := range register.Derive(t.Corpus, decisions) {
		s, err := t.subject(ctx, res, runner, standing, observing)
		if err != nil {
			return nil, err
		}
		res.Subjects = append(res.Subjects, s)
	}

	// A cache that cannot be saved — a read-only checkout, a full disk — costs
	// one slow tick and can never corrupt a record, so it is not worth failing
	// a pass over.
	_ = cache.Save()
	return res, nil
}

func (t *Tick) subject(ctx context.Context, res *Result, runner *gather.Runner, standing register.Standing, observing bool) (Subject, error) {
	s := Subject{ID: standing.Artifact, Standing: standing}
	if e, ok := t.Corpus.Get(standing.Artifact); ok {
		s.Entry = e
	}

	records, quarantined, err := ledger.ReadTruth(t.Store, s.ID)
	if err != nil {
		return s, err
	}
	res.Quarantined = append(res.Quarantined, quarantined...)
	s.Truth = records

	dispositions, quarantined, err := ledger.ReadDispositions(t.Store, s.ID)
	if err != nil {
		return s, err
	}
	res.Quarantined = append(res.Quarantined, quarantined...)
	s.Dispositions = dispositions

	// The series is per content version: a revision starts a new one, because
	// the earlier verdicts concerned a different statement.
	hash := standing.Hash
	series := forWording(records, hash)
	last, held := lastOf(series)

	if s.Predicated() && hash.Digest != "" {
		s.Due, s.Because = t.due(res.Tree, s.Entry.Artifact, last, held)
		if observing && s.Due {
			answer, err := t.observe(ctx, res, runner, &s, hash, series, last, held)
			if err != nil {
				return s, err
			}
			s.Answer = answer
			if answer.Value == "" {
				s.Unevaluated = answer.Detail
			}
		}
	}
	if !s.Answer.Reached() && s.Unevaluated == "" && held {
		// The last verdict for this wording, carried forward. When nothing the
		// artifact looks at has moved it is still the answer, and when
		// something has it is an answer about a tree this is not — which is
		// what `stale` means, and why a survey reports one rather than
		// reporting that nothing has ever answered. See the package comment.
		s.Answer = carried(series, last, res.World)
	}

	s.Drift, s.Drifting = s.drift()
	switch {
	case observing && s.Drifting:
		if err := t.dispose(&s, res.World); err != nil {
			return s, err
		}
	case s.Drifting:
		// A survey decides nothing, so what is in force is whatever the last
		// tick decided. Reading it back is the whole of why it was written
		// down: a refusal cannot be re-derived.
		s.Disposition, _ = ledger.LastDisposition(s.Dispositions)
	}
	return s, nil
}

// due is docs/decision/0033's question, asked of one artifact.
//
// Three ways in, and the first two are what make a tick cheap: nothing has ever
// answered for this wording, the bytes in some gatherer's scope have moved, or
// the answer is older than max_age. A scope that matches nothing is always due,
// which is a distinct outcome rather than a digest over an empty set — the fold
// over an empty set is perfectly stable, so a mis-typed scope would otherwise
// read as "nothing has changed" forever.
func (t *Tick) due(tree *workspace.Tree, art *model.Artifact, last ledger.Truth, held bool) (bool, string) {
	if t.Options.All {
		return true, "every artifact was forced"
	}
	if !held {
		return true, "nothing has answered for what this says now"
	}

	measured := t.measured(last)
	for _, name := range gatherNames(art.Gather) {
		spec := art.Gather[name]
		g, known := t.Catalogue[spec.Gatherer]
		if !known {
			return true, fmt.Sprintf("%s names the %s gatherer, and there is no such gatherer", name, spec.Gatherer)
		}
		scope := tree.Scope(gather.Globs(spec.Args))
		recorded, found := measured[identity(g.Name(), spec.Args)]
		switch {
		case !found:
			return true, fmt.Sprintf("%s did not produce the last answer", name)
		case scope.MatchedNothing():
			return true, fmt.Sprintf("%s looks at no path in this workspace", name)
		case !scope.Unchanged(recorded):
			return true, fmt.Sprintf("the bytes %s looks at have changed", name)
		}
	}

	if art.MaxAge > 0 && time.Since(last.At) > art.MaxAge {
		return true, fmt.Sprintf("the answer is older than %s", art.MaxAge)
	}
	return false, ""
}

// measured is the scope digest each of an answer's observations recorded,
// keyed by what identifies a gather entry.
//
// An observation that cannot be read is simply absent, which makes its entry
// due. That is the safe direction: a pruned or torn observation costs one
// gather, and treating it as unchanged would leave a promise permanently
// unmeasured with nothing reporting it.
func (t *Tick) measured(last ledger.Truth) map[string]string {
	out := make(map[string]string, len(last.From))
	for _, address := range last.From {
		obs, err := ledger.ReadObservation(t.Store, address)
		if err != nil {
			continue
		}
		out[identity(obs.Gatherer, obs.Args)] = obs.Scope
	}
	return out
}

// identity is what makes two measurements measurements of the same thing, less
// the world and the scope they were taken in: the gatherer with its version,
// and what it was asked for.
//
// It is the observation's own preimage with the moving parts held still, rather
// than a string composed here, so that a change to what an observation is
// addressed by moves this with it. The version is part of it because a
// gatherer's bug fix must not reuse pre-fix observations, which means it also
// makes every promise reading that gatherer due once — which is right.
func identity(gatherer string, args map[string]any) string {
	return ledger.Observation{Gatherer: gatherer, Args: args}.Address()
}

// observe gathers, evaluates, and records the verdict if it is a transition or
// a heartbeat.
func (t *Tick) observe(ctx context.Context, res *Result, runner *gather.Runner, s *Subject, hash canonical.Hash, series []ledger.Truth, last ledger.Truth, held bool) (Answer, error) {
	art := s.Entry.Artifact
	now := map[string]model.Observation{}
	var from []string
	var unmeasured []string

	for _, name := range gatherNames(art.Gather) {
		spec := art.Gather[name]
		if _, known := t.Catalogue[spec.Gatherer]; !known {
			// Nothing will ever run it, so this is a question with no
			// instrument. The corpus reported it at load; it is not an unknown
			// and no verdict is reached.
			return Answer{Detail: fmt.Sprintf("%s names the %s gatherer, and there is no such gatherer", name, spec.Gatherer), Gathered: true}, nil
		}
		req := gather.NewRequest(res.Tree, res.World, spec.Args, t.Options.Deadline)
		obs := runner.Gather(ctx, spec, req)
		now[name] = obs
		if obs.Err != nil {
			unmeasured = append(unmeasured, fmt.Sprintf("%s: %s", name, obs.Err))
			continue
		}
		address, recorded, err := gather.Record(t.Ledger, obs)
		if err != nil {
			// A ledger that will not take a measurement is not a verdict of
			// any kind. Reporting it as unknown would say nobody measured when
			// somebody did and the record was lost.
			return Answer{}, err
		}
		if recorded {
			res.Observed++
			from = append(from, address)
		}
	}

	answer := Answer{World: res.World, From: from, Fresh: true, Gathered: true}

	// If any declared gatherer failed to produce an observation, truth is
	// unknown — not evaluated against what did arrive. A machine that could not
	// measure does not record what it thinks it saw, and the field nobody could
	// answer for is what the detail names. See docs/decision/0004.
	if len(unmeasured) > 0 {
		answer.Value = model.Unknown
		answer.Detail = joinLines(unmeasured)
	} else {
		expr, err := predicate.Parse(art.HoldWhen, predicate.HoldWhen)
		if err != nil {
			return Answer{Detail: err.Error(), Gathered: true}, nil
		}
		verdict, err := expr.Hold(predicate.Observed{Now: now, History: t.history(art, s.Truth)}.Scope())
		if err != nil {
			// An expression that is wrong is not a verdict at all. The two
			// failures are told apart by type and never by reading a message.
			return Answer{Detail: err.Error(), Gathered: true}, nil
		}
		answer.Value, answer.Detail = verdict.Value, verdict.Because
	}

	record := ledger.Truth{
		Artifact: art.ID,
		Hash:     hash.String(),
		Norm:     hash.Norm,
		World:    res.World,
		Value:    answer.Value,
		From:     from,
		Detail:   answer.Detail,
	}
	// Transitions, not ticks — plus the heartbeat, which is the only thing that
	// makes a stale answer visible: an observation's timestamp means first-seen
	// and never advances for an unchanging world.
	heartbeat := held && art.MaxAge > 0 && time.Since(last.At) > art.MaxAge
	if record.Moved(last, held) || heartbeat {
		written, err := t.Ledger.Reached(record)
		if err != nil {
			return Answer{}, err
		}
		answer.Since, answer.Recorded = written.At, true
		return answer, nil
	}

	// The verdict has not moved, so the answer has held since it was first
	// reached rather than since this tick.
	answer.Since = since(series, last)
	return answer, nil
}

// carried is the answer an artifact keeps when nothing it looks at has moved.
func carried(series []ledger.Truth, last ledger.Truth, world model.World) Answer {
	return Answer{
		Value:  last.Value,
		Detail: last.Detail,
		Since:  since(series, last),
		World:  last.World,
		From:   last.From,
		Fresh:  last.World.Same(world),
	}
}

// since walks back through a folded series to where the verdict in force was
// first reached, so that a heartbeat does not read as a transition.
func since(series []ledger.Truth, last ledger.Truth) time.Time {
	at := last.At
	for i := len(series) - 1; i >= 0; i-- {
		if series[i].Value != last.Value {
			break
		}
		at = series[i].At
	}
	return at
}

// history is what prior() reaches into: every observation this artifact's
// verdicts have ever named, keyed by the name the predicate reads it under.
//
// Every wording's, not just this one's. Observations are about the world and
// know nothing about any promise, so they survive a revision that resets the
// artifact's truth history — which is what lets a strengthened predicate read
// history from before it existed.
func (t *Tick) history(art *model.Artifact, records []ledger.Truth) map[string][]model.Observation {
	named := map[string]string{}
	for _, name := range gatherNames(art.Gather) {
		spec := art.Gather[name]
		if g, known := t.Catalogue[spec.Gatherer]; known {
			named[identity(g.Name(), spec.Args)] = name
		}
	}

	out := map[string][]model.Observation{}
	read := map[string]bool{}
	for _, r := range records {
		for _, address := range r.From {
			if read[address] {
				continue
			}
			read[address] = true
			obs, err := ledger.ReadObservation(t.Store, address)
			if err != nil {
				// Pruned, or torn. prior() reaching one observation less is
				// what retention already costs; refusing to evaluate would
				// make a prune break a promise.
				continue
			}
			name, reads := named[identity(obs.Gatherer, obs.Args)]
			if !reads {
				continue
			}
			out[name] = append(out[name], model.Observation{
				ID:          address,
				Gatherer:    obs.Gatherer,
				Args:        obs.Args,
				ScopeDigest: obs.Scope,
				World:       obs.World,
				At:          obs.At,
				Data:        obs.Data,
			})
		}
	}
	return out
}

// forWording narrows a folded stream to the verdicts about one content version.
func forWording(records []ledger.Truth, hash canonical.Hash) []ledger.Truth {
	if hash.Digest == "" {
		return nil
	}
	var out []ledger.Truth
	for _, r := range records {
		if r.Hash == hash.String() {
			out = append(out, r)
		}
	}
	return out
}

func lastOf(series []ledger.Truth) (ledger.Truth, bool) {
	if len(series) == 0 {
		return ledger.Truth{}, false
	}
	return series[len(series)-1], true
}

// gatherNames is the gather map's keys, sorted, so that one pass measures in
// one order and two runs over one tree agree.
func gatherNames(m map[string]model.GatherSpec) []string {
	out := make([]string, 0, len(m))
	for name := range m {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func joinLines(items []string) string {
	out := ""
	for i, item := range items {
		if i > 0 {
			out += "; "
		}
		out += item
	}
	return out
}
