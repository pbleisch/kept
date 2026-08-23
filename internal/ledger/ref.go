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
	"strings"
	"time"
)

// Ref is the ref that carries kept's machine-written tier. No worktree checks
// it out: a repair box receives the tree being repaired, never the standing,
// truth, or prior-agent evidence kept recorded about it.
const Ref = "refs/kept/ledger"

// FetchRefspec makes an ordinary git fetch bring the ledger ref with the
// repository it describes. It is kept alongside Ref so init cannot configure
// a second spelling of the same durable location.
//
// It is not forced, and that is the load-bearing half. [RefStore.mutate] only
// ever moves the ref forward from a value it just read, so a ledger advances
// and never rewinds, and every legitimate fetch is a fast-forward: a new ref
// in a clone that has none, or a remote that is ahead. The only case a leading
// `+` adds is the one that must never happen — a remote that has fallen behind
// overwriting acts nobody pushed — which git refuses by name and with a
// non-zero exit unless something opts out of the refusal. Side refs get no
// reflog, so that refusal is the only warning there is. See
// docs/decision/0016.
const FetchRefspec = "refs/kept/ledger:refs/kept/ledger"

// ForcedFetchRefspec is the same durable location, forced. Nothing kept writes
// configures it any more; it is named so an origin configured before
// docs/decision/0016 can be recognized and corrected, and so kept doctor can
// report one nobody has corrected yet.
//
// Adding FetchRefspec beside it does not disarm it. Git applies the first
// refspec that matches, so a config holding both still force-updates when the
// forced line comes first — which is the order `config --add` produces — and
// what a fetch does then depends on config file order rather than on anything
// kept decided. That is measured, not reasoned: it is why ConfigureFetch
// removes this rather than adding around it.
const ForcedFetchRefspec = "+" + FetchRefspec

// forcedFetchPattern matches ForcedFetchRefspec and nothing else. A git config
// value pattern is a POSIX extended regular expression, so the leading + has
// to be escaped or it reads as a repetition operator.
var forcedFetchPattern = "^" + regexp.QuoteMeta(ForcedFetchRefspec) + "$"

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
// it. Configuring the refspec and correcting a forced one are different acts
// and init says which happened, because rewriting somebody's git config
// without saying so is the failure mode on the other side of leaving a trap
// armed. See docs/decision/0016.
type FetchConfig string

const (
	// FetchUnchanged is an origin that already fetches the ledger, unforced.
	// Nothing was written.
	FetchUnchanged FetchConfig = "unchanged"
	// FetchAdded is an origin that did not fetch the ledger at all.
	FetchAdded FetchConfig = "added"
	// FetchForceRemoved is an origin carrying [ForcedFetchRefspec], which was
	// removed. This is the state every clone adopted under the forced
	// spelling is in.
	FetchForceRemoved FetchConfig = "force-removed"
)

// ConfigureFetch makes origin fetch the ledger, unforced, whatever it was
// doing before. A non-Git directory is deliberately reported to the caller,
// because init is also useful for a manuscript before it lives in a
// repository.
//
// A forced spelling already configured is removed rather than left beside the
// unforced one. Both together is not a cautious halfway house: the forced line
// still wins when it comes first, so the outcome would turn on config file
// order. Correcting kept's own prior output is not rewriting what a person
// wrote — this is one exact string that kept authored — and init reports it
// either way.
func ConfigureFetch(repository string) (FetchConfig, error) {
	s := NewRefStore(repository)
	if _, _, err := s.revision(); err != nil {
		if isNotRepository(err) {
			return FetchUnchanged, ErrNotRepository
		}
		return FetchUnchanged, err
	}

	unforced, forced, err := s.fetchRefspecs()
	if err != nil {
		return FetchUnchanged, err
	}
	if forced {
		// Exit 5 is "no value matched", which is already the wanted state.
		if _, err := s.git(nil, nil, "config", "--unset-all", "remote.origin.fetch", forcedFetchPattern); err != nil && !isExit(err, 5) {
			return FetchUnchanged, err
		}
	}
	if !unforced {
		if _, err := s.git(nil, nil, "config", "--add", "remote.origin.fetch", FetchRefspec); err != nil {
			return FetchUnchanged, err
		}
	}

	// Read back rather than trust the write. Reporting a forced refspec
	// corrected while leaving it configured is precisely the silent trap this
	// exists to close, so it is checked rather than assumed.
	if _, stillForced, err := s.fetchRefspecs(); err != nil {
		return FetchUnchanged, err
	} else if stillForced {
		return FetchUnchanged, fmt.Errorf("origin still force-fetches %s: remove %s from remote.origin.fetch by hand", Ref, ForcedFetchRefspec)
	}

	switch {
	case forced:
		return FetchForceRemoved, nil
	case unforced:
		return FetchUnchanged, nil
	}
	return FetchAdded, nil
}

// ForcedFetch reports whether origin is configured to force-fetch the ledger.
// It is what kept doctor reads: nothing refuses the configuration, no kept
// verb fetches, and nothing else in kept looks at it once a workspace is
// adopted — so an operator who never runs init again is never told. Running
// kept init is what removes it. See docs/decision/0016.
func ForcedFetch(repository string) (bool, error) {
	s := NewRefStore(repository)
	if _, _, err := s.revision(); err != nil {
		if isNotRepository(err) {
			return false, ErrNotRepository
		}
		return false, err
	}
	_, forced, err := s.fetchRefspecs()
	return forced, err
}

// fetchRefspecs reports whether origin fetches the ledger and whether it does
// so forced. Both are true at once in a clone adopted under the forced
// spelling and configured again by a build that only added.
func (s *RefStore) fetchRefspecs() (unforced, forced bool, err error) {
	out, err := s.git(nil, nil, "config", "--get-all", "remote.origin.fetch")
	if err != nil && !isExit(err, 1) {
		return false, false, err
	}
	for _, value := range strings.Split(string(out), "\n") {
		switch strings.TrimSpace(value) {
		case FetchRefspec:
			unforced = true
		case ForcedFetchRefspec:
			forced = true
		}
	}
	return unforced, forced, nil
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
		if _, err := s.git(nil, nil, "update-ref", Ref, commit, old); err == nil {
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
