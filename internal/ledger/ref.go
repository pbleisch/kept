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
const FetchRefspec = "+refs/kept/ledger:refs/kept/ledger"

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

// ConfigureFetch adds kept's ledger refspec to origin exactly once. A
// non-Git directory is deliberately reported to the caller, because init is
// also useful for a manuscript before it lives in a repository.
func ConfigureFetch(repository string) (bool, error) {
	s := NewRefStore(repository)
	if _, _, err := s.revision(); err != nil {
		if isNotRepository(err) {
			return false, ErrNotRepository
		}
		return false, err
	}

	out, err := s.git(nil, nil, "config", "--get-all", "remote.origin.fetch")
	if err != nil && !isExit(err, 1) {
		return false, err
	}
	for _, value := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(value) == FetchRefspec {
			return false, nil
		}
	}
	if _, err := s.git(nil, nil, "config", "--add", "remote.origin.fetch", FetchRefspec); err != nil {
		return false, err
	}
	return true, nil
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
