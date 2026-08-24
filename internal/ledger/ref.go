package ledger

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Ref is the ref that carries kept's machine-written tier. No worktree checks
// it out: a repair box receives the tree being repaired, never the standing,
// truth, or prior-agent evidence kept recorded about it.
const Ref = "refs/kept/ledger"

// UpstreamRef is where an ordinary git fetch puts what the remote holds. It is
// the second ref git gives every branch, and kept had none: an upstream copy
// the network overwrites freely, beside a local ref only an explicit act
// advances. Every safety property people attribute to "normal branch flow"
// comes from that split rather than from living under refs/heads/.
//
// Nothing reads standing or truth from here. It is a copy of a remote's
// opinion, and it becomes part of the durable record only when [Advance] moves
// Ref onto it. See docs/decision/0062.
const UpstreamRef = "refs/kept/upstream/ledger"

// FetchRefspec makes an ordinary git fetch bring the ledger ref with the
// repository it describes, and put it somewhere that is not the durable record.
// It is kept alongside Ref so init cannot configure a second spelling of the
// same durable location.
//
// Every character of it is load-bearing and two of them read as the opposite of
// what an earlier spelling decided.
//
// **The destination is UpstreamRef.** The previous spelling made the
// destination of a network fetch the authoritative record itself, which is the
// shape underneath everything that has gone wrong here: forced, it erased two
// ratifications on 2026-08-22; unforced, git refuses the fetch, and a ledger
// ahead of its remote is this system's resting state — so an ordinary
// `git fetch` fails, and a worktree pool that fetches on acquisition hands out
// nothing. Against an upstream destination the authoritative ref is not written
// by the network at all, which makes that whole class of loss unreachable
// rather than refused.
//
// **The leading + is back, and it is not a reversal.** It was wrong on a ref
// kept's own writer owns and it is right on a ref that is a copy of what a
// remote holds — which is why +refs/heads/*:refs/remotes/origin/* has carried
// one since git had remotes. A remote that rewinds its own ledger is a fact
// about the remote, and an upstream ref exists to report it.
//
// **The * is not decoration.** A single-ref refspec whose source the remote does
// not have is fatal — exit 128, `couldn't find remote ref` — which is the state
// of every repository kept has ever adopted, before anything has ticked. A
// pattern that matches nothing is a no-op. Measured: that one character is the
// whole of the fresh-adoption fix.
//
// See docs/decision/0062, which amends docs/decision/0016.
const FetchRefspec = "+" + Ref + "*:" + UpstreamRef + "*"

// DirectRefspec is the spelling that made the destination of a fetch the
// durable record, and ForcedDirectRefspec is the same thing forced. Nothing
// kept writes configures either any more; they are named so an origin
// configured before docs/decision/0062 can be recognized and corrected, and so
// kept doctor can report one nobody has corrected yet.
//
// Both are removed rather than left beside the tracking spelling. Git applies
// the first refspec that matches, so a config holding a direct line and a
// tracking one still writes the ledger when the direct line comes first — which
// is the order `config --add` produces — and what a fetch does then turns on
// config file order rather than on anything kept decided. That is measured, not
// reasoned: it is why ConfigureFetch removes rather than adding around.
const (
	DirectRefspec       = Ref + ":" + Ref
	ForcedDirectRefspec = "+" + DirectRefspec
)

// DirectRefspecs is both spellings, forced first, which is the order they are
// reported in.
var DirectRefspecs = []string{ForcedDirectRefspec, DirectRefspec}

// directPattern matches the two direct spellings and nothing else. A git config
// value pattern is a POSIX extended regular expression, so the leading + has to
// be escaped or it reads as a repetition operator.
var directPattern = "^" + regexp.QuoteMeta("+") + "?" + regexp.QuoteMeta(DirectRefspec) + "$"

// DefaultRefDeadline bounds each local Git invocation the ref store makes.
// Ref operations do not contact a remote, but a corrupt repository, hook, or
// locked ref can still wedge an unattended run.
const DefaultRefDeadline = 30 * time.Second

// RefStore stores the ledger tree on Ref. It writes through a temporary index
// and reads with git show, so the Store interface stays about ledger paths
// rather than becoming a filesystem exposed to a repair runtime.
type RefStore struct {
	repository string
	deadline   time.Duration
}

var _ Store = (*RefStore)(nil)

// NewRefStore returns a store rooted at the Git repository containing the
// workspace. Operations report a non-repository or Git failure when used
// outside one; [ForWorkspace] is the compatibility choice for a workspace
// that has no Git metadata at all.
func NewRefStore(repository string) *RefStore {
	return &RefStore{repository: repository, deadline: DefaultRefDeadline}
}

// ForWorkspace selects the side ref whenever a workspace is a Git checkout.
// A non-Git manuscript still has no possible repair worktree, so it keeps the
// existing local-only ledger behavior rather than pretending it has a ref.
// A present but broken .git is not a reason to fall back: it must report the
// Git failure rather than writing a second ledger beside it.
func ForWorkspace(workspace string) Store {
	_, err := os.Lstat(filepath.Join(workspace, ".git"))
	if err == nil || !errors.Is(err, os.ErrNotExist) {
		return NewRefStore(workspace)
	}
	return NewFiles(filepath.Join(workspace, ".kept"))
}

// FetchConfig is what [ConfigureFetch] found at origin and what it did about
// it. Configuring the refspec and redirecting one that wrote the durable record
// are different acts and init says which happened, because rewriting somebody's
// git config without saying so is the failure mode on the other side of leaving
// a trap armed. See docs/decision/0016 and docs/decision/0062.
type FetchConfig string

const (
	// FetchUnchanged is an origin that already fetches the ledger into
	// [UpstreamRef] and carries no direct spelling. Nothing was written.
	FetchUnchanged FetchConfig = "unchanged"
	// FetchAdded is an origin that did not fetch the ledger at all.
	FetchAdded FetchConfig = "added"
	// FetchRedirected is an origin carrying a direct spelling — where a fetch
	// wrote the durable record itself — which was removed. This is the state
	// every clone adopted before docs/decision/0062 is in.
	FetchRedirected FetchConfig = "redirected"
)

// ConfigureFetch makes origin fetch the ledger into [UpstreamRef], and makes
// git write a reflog for the ledger ref. A non-Git directory is deliberately
// reported to the caller, because init is also useful for a manuscript before
// it lives in a repository.
//
// Two config lines, and they are the same subject: what an ordinary git fetch
// does to the durable tier. The refspec decides whether the network can write
// it at all; [LogAllRefUpdates] decides whether anything is left afterwards to
// say that something did. One is prevention and the other is detection and
// recovery, and an adoption verb that installed only the first would leave the
// second to an operator who does not know to ask for it.
//
// A direct spelling already configured — forced or not — is removed rather than
// left beside the tracking one. Both together is not a cautious halfway house:
// git applies the first refspec that matches, so the outcome would turn on
// config file order. Correcting kept's own prior output is not rewriting what a
// person wrote — these are two exact strings that kept authored — and init
// reports it either way.
//
// The outcomes are returned separately because they are separate acts and init
// says which happened. See docs/decision/0016, docs/decision/0054 and
// docs/decision/0062.
func ConfigureFetch(repository string) (FetchConfig, LogConfig, error) {
	s := NewRefStore(repository)
	if _, _, err := s.revision(); err != nil {
		if isNotRepository(err) {
			return FetchUnchanged, LogUnchanged, ErrNotRepository
		}
		return FetchUnchanged, LogUnchanged, err
	}

	tracking, direct, err := s.fetchRefspecs()
	if err != nil {
		return FetchUnchanged, LogUnchanged, err
	}
	if len(direct) > 0 {
		// Exit 5 is "no value matched", which is already the wanted state.
		if _, err := s.git(nil, nil, "config", "--unset-all", "remote.origin.fetch", directPattern); err != nil && !isExit(err, 5) {
			return FetchUnchanged, LogUnchanged, err
		}
	}
	if !tracking {
		if _, err := s.git(nil, nil, "config", "--add", "remote.origin.fetch", FetchRefspec); err != nil {
			return FetchUnchanged, LogUnchanged, err
		}
	}

	// Read back rather than trust the write. Reporting a direct refspec
	// redirected while leaving it configured is precisely the silent trap this
	// exists to close, so it is checked rather than assumed.
	if _, stillDirect, err := s.fetchRefspecs(); err != nil {
		return FetchUnchanged, LogUnchanged, err
	} else if len(stillDirect) > 0 {
		return FetchUnchanged, LogUnchanged, fmt.Errorf("origin still fetches straight into %s: remove %s from remote.origin.fetch by hand", Ref, strings.Join(stillDirect, " and "))
	}

	log, err := s.configureLog()
	if err != nil {
		return FetchUnchanged, LogUnchanged, err
	}

	switch {
	case len(direct) > 0:
		return FetchRedirected, log, nil
	case tracking:
		return FetchUnchanged, log, nil
	}
	return FetchAdded, log, nil
}

// LogAllRefUpdates is the core.logAllRefUpdates value that makes git keep a
// reflog for Ref, and the value `true` is not it.
//
// Git logs a ref update only for a hardcoded set of path prefixes —
// refs/heads/, refs/remotes/, refs/notes/ and HEAD — and refs/kept/ is not one
// of them, which is why the durable tier sat in the one namespace with no
// safety log. `always` lifts that restriction for every ref under refs/, and it
// is the whole of the difference: measured, `true` leaves the ledger ref with
// no reflog at all and `always` records every update to it, under both the
// files and the reftable backend.
//
// It is the config value rather than a pre-created log file because git logs a
// ref "only when the file exists", and deleting the ref deletes the file:
// measured, a touched log file stops logging permanently after the ref is
// deleted once, while `always` recreates it on the next update. A witness that
// silently stops witnessing is worse than none.
//
// See docs/decision/0054.
const LogAllRefUpdates = "always"

// logAllRefUpdatesKey is the setting LogAllRefUpdates is written to. It is
// repository-wide rather than per-ref: git offers no way to log one ref
// hierarchy and not another, so covering the ledger covers every tag and
// remote-tracking ref too. Measured: 29 KB of logs for a repository holding 251
// refs, about 115 bytes per entry, and about 140 for kept's own entries, whose
// reason strings are longer. Accepted in docs/decision/0054.
const logAllRefUpdatesKey = "core.logAllRefUpdates"

// LogConfig is what [ConfigureFetch] found for [logAllRefUpdatesKey] and what
// it did about it.
type LogConfig string

const (
	// LogUnchanged is a repository already logging every ref update. Nothing
	// was written.
	LogUnchanged LogConfig = "unchanged"
	// LogEnabled is a repository that was not logging the ledger ref, and now
	// is. This is the state every clone is in, because reflogs are not cloned
	// and the setting reverts to git's default.
	LogEnabled LogConfig = "enabled"
)

// configureLog turns logging on unless it is already on, and reads the value
// back rather than trusting the write, for the reason the refspec does.
//
// It overwrites an explicit `false`. That is deliberate and it is the one place
// here that changes a value kept did not author: a repository that logs no ref
// update cannot record what the ledger held, so leaving it would be adopting a
// workspace into a guarantee that cannot hold there. init says what it changed,
// which is the same treatment docs/decision/0016 gave the refspec.
func (s *RefStore) configureLog() (LogConfig, error) {
	_, logged, err := s.logged()
	if err != nil {
		return LogUnchanged, err
	}
	if logged {
		return LogUnchanged, nil
	}
	if _, err := s.git(nil, nil, "config", logAllRefUpdatesKey, LogAllRefUpdates); err != nil {
		return LogUnchanged, err
	}
	switch _, logged, err := s.logged(); {
	case err != nil:
		return LogUnchanged, err
	case !logged:
		return LogUnchanged, fmt.Errorf("%s is still not %q: set it by hand", logAllRefUpdatesKey, LogAllRefUpdates)
	}
	return LogEnabled, nil
}

// Logged reports whether git will write a reflog for the ledger ref in this
// repository. It is what kept doctor reads, and it is the same 0050 shape as
// the forced refspec beside it: nothing refuses an unlogged ledger, no kept
// verb reads the setting once a workspace is adopted, and kept init is what
// sets it — so an operator who never re-inits is never told.
//
// It answers about what git will do next rather than about how many entries
// exist. A correctly configured repository whose ledger ref has not moved since
// adoption has an empty reflog and no defect, and reporting that would be a
// finding nobody can act on.
// It returns the configured value as well as the verdict, because doctor
// reports what is there rather than what should be — a finding that printed the
// wanted value would read as though the repository already had it.
func Logged(repository string) (string, bool, error) {
	s := NewRefStore(repository)
	if _, _, err := s.revision(); err != nil {
		if isNotRepository(err) {
			return "", false, ErrNotRepository
		}
		return "", false, err
	}
	return s.logged()
}

// logged reads the setting. Git accepts the value in any case, so it is
// compared that way; an unset value is exit 1 and is not an error, and is
// reported as the empty string rather than as a value nobody wrote.
func (s *RefStore) logged() (string, bool, error) {
	out, err := s.git(nil, nil, "config", "--get", logAllRefUpdatesKey)
	if err != nil {
		if isExit(err, 1) {
			return "", false, nil
		}
		return "", false, err
	}
	value := strings.TrimSpace(string(out))
	return value, strings.EqualFold(value, LogAllRefUpdates), nil
}

// DirectFetch reports every direct spelling origin still carries — a refspec
// whose destination is the ledger ref itself, so an ordinary git fetch writes
// the durable record. It is what kept doctor reads: nothing refuses the
// configuration, no kept verb fetches, and nothing else in kept looks at it once
// a workspace is adopted — so an operator who never runs init again is never
// told. Running kept init is what removes it. See docs/decision/0062.
func DirectFetch(repository string) ([]string, error) {
	s := NewRefStore(repository)
	if _, _, err := s.revision(); err != nil {
		if isNotRepository(err) {
			return nil, ErrNotRepository
		}
		return nil, err
	}
	_, direct, err := s.fetchRefspecs()
	return direct, err
}

// fetchRefspecs reports whether origin fetches the ledger into [UpstreamRef],
// and which direct spellings it also carries. Both are true at once in a clone
// adopted under a direct spelling and configured again by a build that only
// added.
//
// The direct spellings are returned in [DirectRefspecs] order rather than in
// config order, so a repository carrying both reports them the same way twice.
func (s *RefStore) fetchRefspecs() (tracking bool, direct []string, err error) {
	out, err := s.git(nil, nil, "config", "--get-all", "remote.origin.fetch")
	if err != nil && !isExit(err, 1) {
		return false, nil, err
	}
	found := make(map[string]bool)
	for _, value := range strings.Split(string(out), "\n") {
		switch value := strings.TrimSpace(value); value {
		case FetchRefspec:
			tracking = true
		case DirectRefspec, ForcedDirectRefspec:
			found[value] = true
		}
	}
	for _, spelling := range DirectRefspecs {
		if found[spelling] {
			direct = append(direct, spelling)
		}
	}
	return tracking, direct, nil
}

// ErrNotRepository distinguishes an ordinary directory that init may adopt
// from a repository whose side ref cannot be read or written.
var ErrNotRepository = errors.New("not a Git repository")

// Append writes one complete ledger line. A compare-and-swap retry preserves
// both writers' lines when two runs append concurrently without making a lock
// load-bearing.
func (s *RefStore) Append(stream string, line []byte) error {
	return s.mutate(stream, func(existing []byte, _ bool) ([]byte, bool) {
		next := append(append([]byte(nil), existing...), line...)
		next = append(next, '\n')
		return next, false
	})
}

// Read returns one complete file from Ref, or ErrNoStream when either the ref
// or its requested path is absent.
func (s *RefStore) Read(name string) ([]byte, error) {
	if err := refPath(name); err != nil {
		return nil, err
	}
	where := sidePath(name)
	revision, held, err := s.revision()
	if err != nil {
		return nil, err
	}
	if !held {
		return nil, fmt.Errorf("%s: %w", name, ErrNoStream)
	}
	content, found, err := s.content(revision, where)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("%s: %w", name, ErrNoStream)
	}
	return content, nil
}

// Put replaces a whole file addressed by its caller. The content-addressed
// artifact caller verifies any pre-existing object before accepting it; a
// changed payload here therefore replaces only a corrupt object at that path.
func (s *RefStore) Put(name string, content []byte) error {
	return s.mutate(name, func(existing []byte, found bool) ([]byte, bool) {
		if found && bytes.Equal(existing, content) {
			return nil, true
		}
		return append([]byte(nil), content...), false
	})
}

func (s *RefStore) mutate(name string, change func([]byte, bool) ([]byte, bool)) error {
	if err := refPath(name); err != nil {
		return err
	}
	where := sidePath(name)
	var last error
	for range 8 {
		base, held, err := s.revision()
		if err != nil {
			return err
		}
		var current []byte
		found := false
		if held {
			current, found, err = s.content(base, where)
			if err != nil {
				return err
			}
		}
		next, unchanged := change(current, found)
		if unchanged {
			return nil
		}
		commit, err := s.commit(base, held, where, next)
		if err != nil {
			return err
		}
		old := base
		if !held {
			// A zero object name is the compare-and-swap value Git expects for
			// an absent ref. Its width follows the repository object format.
			old = strings.Repeat("0", len(commit))
		}
		// -m attributes the entry. Every writer git logs leaves a reason —
		// `fetch origin: forced-update` is what the 2026-08-22 fetch would have
		// left — so kept naming its own writes is what makes an entry that is
		// not one legible at a glance. It costs one argument and it is local
		// diagnostic text: nothing reads it back, and nothing is addressed by
		// it. See docs/decision/0054.
		if _, err := s.git(nil, nil, "update-ref", "-m", "kept "+where, Ref, commit, old); err == nil {
			return nil
		} else {
			if !retryUpdate(err) {
				return err
			}
			last = err
		}
	}
	return fmt.Errorf("update %s after concurrent ledger writes: %w", Ref, last)
}

func retryUpdate(err error) bool {
	var exit *gitExitError
	if !errors.As(err, &exit) || !strings.Contains(exit.output, "cannot lock ref") {
		return false
	}
	return strings.Contains(exit.output, "expected") || strings.Contains(exit.output, "reference already exists")
}

// sidePath preserves the deliberate split in the side-ref layout: durable
// records live below ledger/, while content-addressed packets and transcripts
// live below artifacts/. Store callers name the same logical paths they did
// under Files; the location decision is this implementation's alone.
func sidePath(name string) string {
	if strings.HasPrefix(name, "artifacts/") {
		return name
	}
	return "ledger/" + name
}

// Continuous reports every value this ref's reflog records that the value it
// holds now cannot reach.
//
// It returns values rather than an error, because the values are what a person
// needs in order to recover and an error string is not a list.
//
// Nothing kept does removes a ledger record. [RefStore.mutate] reads the ref,
// builds on the value it read, and writes with a compare-and-swap, so kept's
// writer can only move the ref forward — which makes the whole integrity
// question one sentence about git's own DAG: every value the ref has held must
// be an ancestor of the value it holds now. A recorded value the tip cannot
// reach means something outside kept moved the ref backwards, which is the one
// failure a compare-and-swap cannot prevent, because the writer that did it was
// not kept.
//
// The rule is stated over the DAG rather than over the ledger's contents on
// purpose. The failure replaces the ref, and every summary of the contents — an
// act count, a hash chain, the ratification set — answers a question about the
// records instead. It is also self-clearing under the correct repair and only
// under it: a union commit naming the lost value makes it an ancestor again and
// this goes quiet, while a repair that discards the lost records leaves it
// standing.
//
// [Continuity.Recorded] is whether anything was recorded at all. A repository
// with no reflog for Ref answers false — nothing has been recorded, which is
// neither a rewind nor a failure, and doctor is where that absence is reported.
// It is the same distinction [ErrNoStream] draws one level down, and collapsing
// it would make every fresh clone look like a broken one.
//
// See docs/decision/0054.
func (s *RefStore) Continuous() (Continuity, error) {
	tip, held, err := s.revision()
	if err != nil || !held {
		return Continuity{}, err
	}
	values, err := s.recorded()
	if err != nil || len(values) == 0 {
		return Continuity{Held: tip}, err
	}

	// One walk, whatever the number of entry points: rev-list traverses the DAG
	// once and stops at what the tip reaches. `^tip` is written on stdin beside
	// the values rather than passed as `--not tip` on argv, so that there is one
	// document saying what is interesting and nothing depending on how a flag
	// on the command line interacts with revisions read from a pipe.
	var walk bytes.Buffer
	for _, value := range values {
		walk.WriteString(value + "\n")
	}
	walk.WriteString("^" + tip + "\n")
	out, err := s.git(nil, walk.Bytes(), "rev-list", "--count", "--stdin")
	if err != nil {
		return Continuity{Held: tip}, err
	}
	if strings.TrimSpace(string(out)) == "0" {
		return Continuity{Held: tip, Recorded: true}, nil
	}

	// Only now, in the failure path, is it worth one invocation per value to say
	// which of them the tip cannot reach. A count answers whether; it does not
	// answer what to recover, and what to recover is the whole point.
	var lost []string
	for _, value := range values {
		if _, err := s.git(nil, nil, "merge-base", "--is-ancestor", value, tip); err != nil {
			if isExit(err, 1) {
				lost = append(lost, value)
				continue
			}
			return Continuity{Held: tip}, err
		}
	}
	// Reduced to the values nothing else lost can reach. The ref moves several
	// times per act — the acts stream, then the run's diagnostics — so a single
	// rewind strands a chain of intermediate values, and listing all of them
	// buries the one a person has to act on. Recovering a head recovers its
	// ancestors, so the heads are the answer and the rest are implied.
	lost, err = s.independent(lost)
	if err != nil {
		return Continuity{Held: tip}, err
	}
	// Sorted, because the reflog's order is the order git happened to write it
	// and a refusal that lists two values differently on two runs reads as two
	// different refusals.
	sort.Strings(lost)
	return Continuity{Held: tip, Lost: lost, Recorded: true}, nil
}

// independent is the subset of values that no other value in the set can
// reach. Two genuinely separate rewinds leave two heads and both are reported;
// one rewind over a chain leaves one.
func (s *RefStore) independent(values []string) ([]string, error) {
	if len(values) < 2 {
		return values, nil
	}
	out, err := s.git(nil, nil, append([]string{"merge-base", "--independent"}, values...)...)
	if err != nil {
		return nil, err
	}
	var heads []string
	for _, line := range strings.Split(string(out), "\n") {
		if value := strings.TrimSpace(line); objectID(value) {
			heads = append(heads, value)
		}
	}
	// Never report nothing where something was lost: a reduction that came back
	// empty is a reduction that failed, and the unreduced list is still true.
	if len(heads) == 0 {
		return values, nil
	}
	return heads, nil
}

// Continuity is what Ref's reflog says about the value the ref holds now.
type Continuity struct {
	// Held is the value Ref holds now, and is empty when it holds none.
	Held string
	// Lost is every recorded value Held cannot reach, sorted. Empty is the
	// answer for a ledger that has only ever moved forward.
	Lost []string
	// Recorded says whether the reflog held anything to check. False is
	// *nothing has been recorded* — a fresh clone, or a repository adopted
	// before the reflog was configured — and it is not a clean bill.
	Recorded bool
}

// Rewound is the question a caller asks: did this ref hold a value it can no
// longer reach. A ledger with nothing recorded is not rewound and not clean; it
// is unmeasured, which is why Recorded is read here rather than assumed.
func (c Continuity) Rewound() bool { return c.Recorded && len(c.Lost) > 0 }

// recorded is every distinct value Ref's reflog says it has held, newest first
// and deduplicated.
//
// Two sources, because one of them is not enough. `reflog show` reports the
// value each entry moved the ref *to*, and that misses the value it moved *from*
// on the oldest entry — which is exactly the value at risk in a repository that
// already had a ledger when logging was switched on. Measured: adopt a
// repository holding a ledger, then let the first logged update be the rewind,
// and reading only the new values reports nothing wrong. The pre-image of the
// oldest entry is `Ref@{n}` where n is the entry count; git resolves it when the
// ref existed beforehand and refuses it when that entry created the ref, which
// is the distinction wanted and needs no special case.
func (s *RefStore) recorded() ([]string, error) {
	out, err := s.git(nil, nil, "reflog", "show", Ref, "--format=%H")
	if err != nil {
		return nil, err
	}

	entries := 0
	seen := make(map[string]bool)
	var values []string
	for _, line := range strings.Split(string(out), "\n") {
		value := strings.TrimSpace(line)
		if value == "" {
			continue
		}
		entries++
		if !objectID(value) || seen[value] {
			continue
		}
		seen[value] = true
		values = append(values, value)
	}
	if entries == 0 {
		return nil, nil
	}

	// A pre-image that does not resolve is the ordinary case — the oldest entry
	// created the ref — so the error is the answer rather than a failure. Any
	// other Git trouble here would already have surfaced above.
	pre, err := s.git(nil, nil, "rev-parse", "--verify", "--quiet", fmt.Sprintf("%s@{%d}", Ref, entries))
	if err == nil {
		if value := strings.TrimSpace(string(pre)); objectID(value) && !seen[value] {
			values = append(values, value)
		}
	}
	return values, nil
}

// Continuous is a fact about Ref, so a ledger kept in files has none to report.
// A manuscript outside Git has no ref for anything to rewind and no reflog to
// read, which is *nothing has been recorded* rather than a clean bill.
func Continuous(store Store) (Continuity, error) {
	s, ok := store.(*RefStore)
	if !ok {
		return Continuity{}, nil
	}
	return s.Continuous()
}

func (s *RefStore) revision() (string, bool, error) {
	out, err := s.git(nil, nil, "rev-parse", "--verify", "--quiet", Ref+"^{commit}")
	if err != nil {
		if isExit(err, 1) {
			return "", false, nil
		}
		return "", false, err
	}
	revision := strings.TrimSpace(string(out))
	if revision == "" {
		return "", false, fmt.Errorf("Git returned an empty %s revision", Ref)
	}
	return revision, true, nil
}

func (s *RefStore) content(revision, name string) ([]byte, bool, error) {
	// ls-tree returns an empty result, not a special exit code, for a missing
	// path. That makes ErrNoStream a fact about the ref rather than a parsed
	// English Git diagnostic.
	listed, err := s.git(nil, nil, "ls-tree", "-z", revision, "--", name)
	if err != nil {
		return nil, false, err
	}
	if len(listed) == 0 {
		return nil, false, nil
	}
	content, err := s.git(nil, nil, "show", revision+":"+name)
	if err != nil {
		return nil, false, err
	}
	return content, true, nil
}

func (s *RefStore) commit(base string, held bool, name string, content []byte) (string, error) {
	index, err := os.CreateTemp("", "kept-ledger-index-*")
	if err != nil {
		return "", err
	}
	indexPath := index.Name()
	if err := index.Close(); err != nil {
		os.Remove(indexPath)
		return "", err
	}
	defer os.Remove(indexPath)

	env := []string{"GIT_INDEX_FILE=" + indexPath}
	if held {
		if _, err := s.git(env, nil, "read-tree", base); err != nil {
			return "", err
		}
	} else if _, err := s.git(env, nil, "read-tree", "--empty"); err != nil {
		return "", err
	}

	blob, err := s.git(env, content, "hash-object", "-w", "--stdin")
	if err != nil {
		return "", err
	}
	digest := strings.TrimSpace(string(blob))
	if !objectID(digest) {
		return "", fmt.Errorf("Git returned invalid object id %q", digest)
	}
	if _, err := s.git(env, nil, "update-index", "--add", "--cacheinfo", "100644,"+digest+","+name); err != nil {
		return "", err
	}
	tree, err := s.git(env, nil, "write-tree")
	if err != nil {
		return "", err
	}
	args := []string{"-c", "commit.gpgSign=false", "commit-tree", strings.TrimSpace(string(tree))}
	if held {
		args = append(args, "-p", base)
	}
	commitEnv := append(env,
		"GIT_AUTHOR_NAME=kept", "GIT_AUTHOR_EMAIL=kept@invalid",
		"GIT_COMMITTER_NAME=kept", "GIT_COMMITTER_EMAIL=kept@invalid",
	)
	commit, err := s.git(commitEnv, []byte("kept ledger\n"), args...)
	if err != nil {
		return "", err
	}
	commitID := strings.TrimSpace(string(commit))
	if !objectID(commitID) {
		return "", fmt.Errorf("Git returned invalid commit id %q", commitID)
	}
	return commitID, nil
}

func objectID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, r := range value {
		if !('0' <= r && r <= '9') && !('a' <= r && r <= 'f') {
			return false
		}
	}
	return true
}

func refPath(name string) error {
	if name == "" || strings.ContainsAny(name, "\\\x00") || path.IsAbs(name) || path.Clean(name) != name || name == "." || strings.HasPrefix(name, "../") {
		return fmt.Errorf("%q is not a ledger-side-ref path", name)
	}
	return nil
}

type gitExitError struct {
	args   []string
	output string
	err    error
}

func (e *gitExitError) Error() string {
	output := strings.TrimSpace(e.output)
	if output == "" {
		return fmt.Sprintf("git %s: %v", strings.Join(e.args, " "), e.err)
	}
	return fmt.Sprintf("git %s: %v: %s", strings.Join(e.args, " "), e.err, output)
}

func (e *gitExitError) Unwrap() error { return e.err }

func isExit(err error, code int) bool {
	var exit *gitExitError
	if !errors.As(err, &exit) {
		return false
	}
	var status *exec.ExitError
	return errors.As(exit.err, &status) && status.ExitCode() == code
}

func isNotRepository(err error) bool {
	var exit *gitExitError
	return errors.As(err, &exit) && (strings.Contains(exit.output, "not a git repository") || strings.Contains(exit.output, "not a Git repository"))
}

func (s *RefStore) git(extraEnv []string, input []byte, args ...string) ([]byte, error) {
	deadline := s.deadline
	if deadline <= 0 {
		deadline = DefaultRefDeadline
	}
	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()

	command := exec.CommandContext(ctx, "git", append([]string{"-C", s.repository}, args...)...)
	if len(extraEnv) > 0 {
		command.Env = append(os.Environ(), extraEnv...)
	}
	if input != nil {
		command.Stdin = bytes.NewReader(input)
	}
	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("git %s: %w", strings.Join(args, " "), ctx.Err())
	}
	if err != nil {
		return nil, &gitExitError{args: args, output: string(output), err: err}
	}
	return output, nil
}
