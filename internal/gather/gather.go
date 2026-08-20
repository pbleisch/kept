package gather

import (
	"context"
	"fmt"
	"time"

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
	Scope string
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
	// that will not parse — and it is what makes those reachable when the
	// corpus loads rather than in the code path that decides a verdict.
	Declares(args map[string]any) ([]string, error)

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

// NewRequest builds the request for one gather entry against a tree that has
// already been walked.
//
// The scope is read out of the args here and nowhere else, because
// docs/decision/0034 makes it a projection of them: the whole args map is what
// binds and what the observation cache is keyed by, and a scope beside them
// would be a second answer to the same question.
func NewRequest(tree *workspace.Tree, world model.World, args map[string]any, deadline time.Duration) Request {
	globs := Globs(args)
	digest := tree.Scope(globs)

	var paths []string
	for _, f := range tree.Files {
		if workspace.Match(globs, f.Path) {
			paths = append(paths, f.Path)
		}
	}

	return Request{
		Root:     tree.Root,
		Globs:    globs,
		Paths:    paths,
		Scope:    digest.String(),
		Args:     args,
		World:    world,
		Deadline: deadline,
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
