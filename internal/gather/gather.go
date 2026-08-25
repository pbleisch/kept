package gather

import (
	"context"
	"fmt"
	"time"

	"github.com/pbleisch/kept/internal/canonical"
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/workspace"
)

// DefaultDeadline is what a gather is bounded by when config names no deadline
// of its own.
//
// There is a default rather than an absence, because docs/decision/0026 says
// there is no default of waiting: a Request with a zero deadline is one nobody
// configured, not one nobody bounded. A test suite is the ordinary case and it
// is slow, so this is generous; a false timeout costs an `unknown`, which is
// noise rather than a wrong answer, and config is what tightens it.
const DefaultDeadline = 5 * time.Minute

// Request is what a gatherer is given: where to look, what the promise said to
// look at, and how long it has.
//
// The scope arrives three ways because three different things need it. Globs
// are what the promise authored, Paths are what they matched, and Scope is the
// digest recorded on the observation — a gatherer that reads files wants the
// paths, and nothing has to re-walk the tree to get them.
type Request struct {
	// Root is the workspace root. A gatherer runs here, so a relative path a
	// program prints means the same thing to everyone reading the record.
	Root string
	// Globs are the scope as the promise authored it. They are a projection of
	// Args rather than a field beside them, which is the relation
	// model.Artifact.Statement has to Fields. See docs/decision/0034.
	Globs []string
	// Paths are the workspace-relative paths those globs matched, already
	// resolved by the walk the tick did once.
	Paths []string
	// Scope is the digest of those paths, per docs/decision/0033, as it is
	// recorded on the observation. workspace.NoMatch when the globs matched
	// nothing, which is a distinct outcome and not a digest over an empty set.
	//
	// For a gatherer that declared a ledger read it is the composition
	// docs/decision/0046 describes rather than the tree's digest alone; see
	// [Scope]. Nothing downstream can tell, and nothing downstream should:
	// this is the one string due-ness, the cache key and the address are all
	// driven by.
	Scope string

	// scope is that same digest as a value, so a caller deciding due-ness
	// asks the computation that produced this one rather than deriving a
	// second. Two computations of it would have to compose identically
	// forever; diverging one way leaves a promise due always and the other
	// way never, and the second is silent. See docs/decision/0046.
	scope Scope
	// Args is the gather entry less the gatherer's own name: what the promise
	// asked for, and what the observation cache is keyed by.
	Args map[string]any
	// World is the tree being measured. It is part of the observation's
	// address, so two worlds never collide. See docs/decision/0014.
	World model.World
	// Deadline bounds the whole gather. Zero is DefaultDeadline and never
	// unbounded: nothing external runs without one. See docs/decision/0026.
	Deadline time.Duration
}

func (r Request) deadline() time.Duration {
	if r.Deadline <= 0 {
		return DefaultDeadline
	}
	return r.Deadline
}

// Fields is what a gatherer measured: the fields it declared, and only those.
// Undeclared output is dropped rather than stored, which is docs/decision/0023's
// first layer and the reason a gatherer's output shape is part of its contract.
//
// A field whose value changes between two measurements of one unchanging world
// — a duration, a timestamp, a pid — is a mistake that nothing here can catch:
// the observation is addressed by its data, so a field like that mints a fresh
// record every tick and dedup stops working. Measure what was true, not how
// long it took.
type Fields map[string]any

// Gatherer is one instrument. It observes and it never repairs: a gatherer that
// fixes what it finds cannot be re-run to check the fix, and its output stops
// being evidence. See docs/decision/0004.
type Gatherer interface {
	// Name is the gatherer's name and version, "tests@1". The version is part
	// of an observation's address, because a gatherer's bug fix must not reuse
	// pre-fix observations. The catalogue is keyed by the bare name.
	Name() string

	// Declares is the fields this gatherer returns, given the args a promise
	// wrote. It takes the args because whoever decides the output shape
	// declares it, which is docs/decision/0034's asymmetry as
	// docs/decision/0043 generalises it: where kept decides the shape it is in
	// code, and where the promise decides it — `command`'s authored extractor,
	// or a gatherer whose per-item fields come from schemas kept does not ship
	// — it is in the args.
	//
	// The error is an authoring error — an arg nothing accepts, an extractor
	// that will not parse. It is what makes those reachable when the corpus
	// loads, so that a person is told to fix an entry rather than left to read
	// an `unknown` against a promise nobody could have kept. The gather path
	// asks the same question again, and must: a gatherer that answered it only
	// at load would be refused by `kept list` and measured with anyway. See
	// [refuseUnrunnableSpec].
	Declares(args map[string]any) ([]string, error)

	// Reads is the ledger acts this gatherer's measurement depends on, and it
	// is nil for every gatherer whose data is a function of the tree alone.
	//
	// It is a declaration rather than a capability: the gatherer never opens
	// the ledger. The tick projects the declared kinds over the artifacts in
	// the promise's scope and folds the result into the scope digest, so the
	// address and the cache key both come to name what the data depends on —
	// which is the property `command`'s cache key asserts today and a
	// ledger-reading gatherer would otherwise falsify. See
	// docs/decision/0046.
	//
	// **Only a kind the tick does not write may be declared.** Truth and
	// dispositions are appended every tick, so a gatherer naming one would
	// gather, cause an append, move its own digest and be due again. The
	// catalogue refuses that when the corpus loads; see [ledger.HumanPaced].
	Reads() []ledger.Kind

	// Gather measures. An error means **no observation**, which means the
	// verdict is `unknown` — not a false field, not an empty observation, and
	// never `not-kept`. That is what makes a missing toolchain report "nobody
	// could measure" rather than "the promise is broken". See
	// docs/decision/0004 and the invariant in AGENTS.md.
	Gather(ctx context.Context, req Request) (Fields, error)
}

// TimedOut is a gatherer that ran out of the deadline docs/decision/0026 gives
// it.
//
// It is its own type because a timeout and a failure resolve to the same
// outcome and must still be tellable apart from the record: a gatherer that
// timed out and one that was never installed are both `unknown`, and an
// operator who cannot tell which cannot fix either.
type TimedOut struct {
	What  string
	After time.Duration
}

func (e *TimedOut) Error() string {
	return fmt.Sprintf("%s did not finish within %s", e.What, e.After)
}

// NotRunnable is a program that never started: no such binary, not executable,
// a toolchain that is not installed.
//
// This is the case the three-valued verdict was built for. It is `unknown`,
// because a machine that could not measure does not record what it thinks it
// saw.
type NotRunnable struct {
	Program string
	Err     error
}

func (e *NotRunnable) Error() string {
	return fmt.Sprintf("%s could not be run: %v", e.Program, e.Err)
}

func (e *NotRunnable) Unwrap() error { return e.Err }

// Signalled is a program that was killed by something other than kept's own
// deadline — an OOM killer, an operator, a parent shutting down.
//
// It is a failure and not a measurement. A program that did not choose its own
// exit did not finish saying what it saw, so reading its partial output as
// evidence would be recording what the machine thinks it saw.
type Signalled struct {
	Program string
	Status  string
}

func (e *Signalled) Error() string {
	return fmt.Sprintf("%s was killed (%s), so what it printed is not a measurement", e.Program, e.Status)
}

// BadField is one of `command`'s declared fields whose extractor is wrong —
// a fold over something that is not a list, a comparison between a number and
// a record.
//
// It is a gather failure, so the verdict is `unknown` and never `not-kept`, and
// it names the field so a receipt can say which expression was wrong rather
// than that the gather failed. Most of these are caught when the corpus loads;
// what reaches here is the kind that only a real program's output can produce.
type BadField struct {
	Field string
	Err   error
}

func (e *BadField) Error() string {
	return fmt.Sprintf("the extractor for %s is wrong: %v", e.Field, e.Err)
}

func (e *BadField) Unwrap() error { return e.Err }

// Acts is the ledger as a scope digest may name it: a digest over the acts of
// the declared kinds that name an artifact whose content is one of these paths.
//
// It is an interface the tick implements rather than a ledger read taken here,
// and that is docs/decision/0046's arrangement rather than an indirection. A
// gatherer never opens the ledger — the tick projects it and hands the answer
// over in the Request, exactly as it already hands over Paths and Scope — which
// is what makes a declaring gatherer work inside a box, where no ref carrying
// the ledger is ever checked out.
//
// Nil is a caller with nothing to project, and a gatherer that declares nothing
// never asks.
type Acts interface {
	Project(kinds []ledger.Kind, paths []string) string
}

// scopeDomain separates the composition below from anything else hashed with
// the same function.
const scopeDomain = "kept/scope-digest\n"

// Scope is what due-ness is driven by: the digest over the bytes a promise's
// globs matched, and — only for a gatherer that declared one — a digest over
// the ledger acts it reads, projected to the artifacts in that scope.
//
// **A gatherer that declares nothing gets docs/decision/0033's digest, byte for
// byte.** That is not an optimisation, it is the whole reason the projection
// goes here rather than into the observation preimage: the scope is already in
// the address and already in the cache key, so a declaration moves both
// together and moves nothing for anyone else. A composition that hashed the
// tree digest even when there was nothing to compose it with would re-gather
// every promise in every corpus for no answer that changed.
//
// A scope that matched no path is [workspace.NoMatch] whatever the ledger says.
// That is a mis-stated scope and it is always due; a projection cannot rescue
// globs that name nothing.
type Scope struct {
	tree  workspace.ScopeDigest
	reads string
}

// String is the form recorded on an observation.
func (s Scope) String() string {
	if s.tree.MatchedNothing() {
		return workspace.NoMatch
	}
	if s.reads == "" {
		return s.tree.String()
	}
	enc := canonical.NewEncoder(scopeDomain, canonical.Verbatim)
	enc.Text(s.tree.String())
	enc.Text(s.reads)
	return canonical.Algorithm + ":" + enc.Digest()
}

// MatchedNothing reports whether the globs matched no path at all.
func (s Scope) MatchedNothing() bool { return s.tree.MatchedNothing() }

// Unchanged reports whether this scope hashes to what an earlier observation
// recorded. A scope that matches nothing is never unchanged, and nothing
// recorded is never unchanged either: both answer "due", which is the safe
// direction.
func (s Scope) Unchanged(recorded string) bool {
	if s.MatchedNothing() || recorded == "" || recorded == workspace.NoMatch {
		return false
	}
	return s.String() == recorded
}

// MatchedNothing reports whether this request's globs matched no path, and
// Unchanged whether its scope hashes to what an earlier observation recorded.
//
// They are on the request rather than beside it so that the pass deciding
// due-ness and the pass recording an observation ask one computation. See
// docs/decision/0046.
func (r Request) MatchedNothing() bool { return r.scope.MatchedNothing() }

// Unchanged reports whether this request's scope hashes to what an earlier
// observation recorded.
func (r Request) Unchanged(recorded string) bool { return r.scope.Unchanged(recorded) }

// Requests builds the request for one gather entry against a tree that has
// already been walked.
//
// It is a value with a method rather than a function with six arguments
// because both of its callers — the pass that decides what is due and the pass
// that observes — must build the request the same way. See docs/decision/0046:
// two computations of one digest have to compose identically forever, and the
// direction they fail in silently is the unsafe one.
type Requests struct {
	// Tree is the workspace this pass walked.
	Tree *workspace.Tree
	// World is the tree being measured.
	World model.World
	// Deadline bounds each gather. Zero is DefaultDeadline.
	Deadline time.Duration
	// Acts projects the ledger for a gatherer that declared a read of it. Nil
	// is a caller with none to offer, which is every caller whose gatherers
	// declare nothing.
	Acts Acts
}

// For builds the request one gatherer gets for one gather entry.
//
// The scope is read out of the args here and nowhere else, because
// docs/decision/0034 makes it a projection of them: the whole args map is what
// binds and what the observation cache is keyed by, and a scope beside them
// would be a second answer to the same question.
func (r Requests) For(g Gatherer, args map[string]any) Request {
	globs := Globs(args)
	scope := Scope{tree: r.Tree.Scope(globs)}

	var paths []string
	for _, f := range r.Tree.Files {
		if workspace.Match(globs, f.Path) {
			paths = append(paths, f.Path)
		}
	}

	// The composition happens here and never in internal/workspace. Putting it
	// there would make the ledger part of the world, which is 0033's
	// World.Tree ceasing to be the tree and the tick feeding itself at the
	// deepest level there is.
	if kinds := g.Reads(); len(kinds) > 0 && r.Acts != nil {
		scope.reads = r.Acts.Project(kinds, paths)
	}

	return Request{
		Root:     r.Tree.Root,
		Globs:    globs,
		Paths:    paths,
		Scope:    scope.String(),
		scope:    scope,
		Args:     args,
		World:    r.World,
		Deadline: r.Deadline,
	}
}

// Everything is the scope of a promise that named none.
//
// It is a real glob rather than an empty list, and the difference matters: an
// empty list matches nothing, and a scope matching nothing is a distinct
// outcome that reads as always due and is surfaced as a mis-stated scope
// (docs/decision/0033). A promise that ran `go build ./...` without naming a
// scope did not mis-state one — it depends on the whole workspace, and its
// digest should be the tree's. See docs/decision/0038.
const Everything = "**"

// Globs reads the scope a promise wrote. One glob and a list of them both mean
// a list, and an absent scope means the whole workspace: a gatherer that was
// told nothing about where to look was told to look everywhere.
func Globs(args map[string]any) []string {
	var out []string
	switch t := args[argScope].(type) {
	case string:
		if t != "" {
			out = []string{t}
		}
	case []any:
		for _, item := range t {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
	case []string:
		out = append(out, t...)
	}
	if len(out) == 0 {
		return []string{Everything}
	}
	return out
}

// The args every gatherer understands. `scope` is one of them because a scope
// is about where a gatherer looked rather than about what it is.
const argScope = "scope"
