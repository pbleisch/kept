package box

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pbleisch/kept/internal/ids"
)

// DefaultDeadline bounds each Git invocation made while creating, diffing, or
// discarding a box. A zero request deadline is still bounded; no repair setup
// can wait forever on Git.
const DefaultDeadline = 30 * time.Second

// temporarySuffix names a box's temporary directory after the box, beside it.
// Beside rather than within, because a directory inside the worktree would put
// every scratch file a build writes into the repair's diff; and named after the
// box, because the two share a lifetime and a reader should be able to see
// which box an orphaned directory belonged to.
const temporarySuffix = ".tmp"

// DefaultDiffBytes is the largest repair diff this layer retains. A repair
// transcript belongs in the ledger side ref, but a diff is presented to a
// person and must not let one damaged box consume unbounded memory first.
const DefaultDiffBytes = 1 << 20

var (
	// ErrUnpinnedCommit means a caller tried to cut a box from a moving name
	// such as HEAD or origin/main rather than the full object ID it recorded.
	ErrUnpinnedCommit = errors.New("repair box commit is not pinned")

	// ErrClosed means a caller tried to inspect a box after it was discarded.
	ErrClosed = errors.New("repair box is closed")

	// ErrDiffTooLarge means the repair changed more bytes than the caller can
	// safely retain for review.
	ErrDiffTooLarge = errors.New("repair diff exceeds its bound")
)

// OverlayFile is one deliberately inherited dirty path. Data replaces the
// path's bytes; Remove carries a deletion. Paths are relative to the
// repository root and may not name Git metadata, the ledger, or artifacts.
//
// Mode carries only the permission bits. A zero mode is a regular 0644 file.
// The overlay is data supplied by the caller rather than a read of the
// origin's whole dirty tree, so unrelated local edits never reach an agent.
type OverlayFile struct {
	Path   string
	Data   []byte
	Mode   fs.FileMode
	Remove bool
}

// Request defines one repair box. Commit must be the full object ID of the
// origin commit captured before a thread begins; this package resolves it only
// to verify that it exists locally and never accepts a ref name in its place.
type Request struct {
	Repository string
	Commit     string
	Overlay    []OverlayFile

	// Deadline bounds every Git call. Zero uses DefaultDeadline rather than
	// meaning that an external process may wait indefinitely.
	Deadline time.Duration
	// MaxDiffBytes bounds each byte sequence Change returns. Zero uses
	// DefaultDiffBytes.
	MaxDiffBytes int
}

func (r Request) deadline() time.Duration {
	if r.Deadline <= 0 {
		return DefaultDeadline
	}
	return r.Deadline
}

func (r Request) maxDiffBytes() int {
	if r.MaxDiffBytes <= 0 {
		return DefaultDiffBytes
	}
	return r.MaxDiffBytes
}

// Box is one disposable repair tree. It carries an uncommitted tree snapshot
// made after its overlay was applied, so Change reports the repair alone rather
// than re-attributing inherited operator edits to the agent.
type Box struct {
	repository string
	path       string
	temp       string
	commit     string
	baseTree   string
	deadline   time.Duration
	maxDiff    int

	mu     sync.Mutex
	closed bool
}

// Create cuts a detached worktree at req.Commit, applies the explicit dirty
// overlay, and snapshots the resulting tree. If any post-worktree step fails,
// it forcibly removes the partial box before returning the error.
func Create(ctx context.Context, req Request) (*Box, error) {
	if ctx == nil {
		return nil, errors.New("repair box context is nil")
	}
	repository, err := repositoryRoot(ctx, req.Repository, req.deadline())
	if err != nil {
		return nil, err
	}
	commit, err := pinnedCommit(ctx, repository, req.Commit, req.deadline())
	if err != nil {
		return nil, err
	}
	if err := validateOverlay(req.Overlay); err != nil {
		return nil, err
	}

	parent := filepath.Join(repository, ".kept", "boxes")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return nil, fmt.Errorf("create repair box parent: %w", err)
	}
	box := &Box{
		repository: repository,
		path:       filepath.Join(parent, ids.Mint()),
		commit:     commit,
		deadline:   req.deadline(),
		maxDiff:    req.maxDiffBytes(),
	}
	box.temp = box.path + temporarySuffix
	if _, err := git(ctx, repository, box.deadline, nil, "worktree", "add", "--detach", box.path, box.commit); err != nil {
		return nil, fmt.Errorf("create repair worktree at %s: %w", box.path, err)
	}
	if err := os.Mkdir(box.temp, 0o700); err != nil {
		return nil, box.discardAfterCreate(fmt.Errorf("create repair box temporary directory: %w", err))
	}

	if err := box.verifyHiddenTier(); err != nil {
		return nil, box.discardAfterCreate(err)
	}
	if err := applyOverlay(box.path, req.Overlay); err != nil {
		return nil, box.discardAfterCreate(err)
	}
	baseTree, err := box.snapshot(ctx)
	if err != nil {
		return nil, box.discardAfterCreate(err)
	}
	box.baseTree = baseTree
	return box, nil
}

func (b *Box) discardAfterCreate(cause error) error {
	// A creation failure must not inherit the caller's canceled deadline: the
	// same timeout that interrupted setup must still leave the worktree gone.
	if err := b.close(context.Background()); err != nil {
		return fmt.Errorf("%w; discard partial repair box: %v", cause, err)
	}
	return cause
}

// Path is the worktree handed to the repair agent.
func (b *Box) Path() string { return b.path }

// Temp is the box's own temporary directory, and it lives beside the worktree
// rather than inside it.
//
// A repair runs confined, and confinement takes the host's scratch trees back
// from the provider's baseline — which leaves the process with nowhere at all
// to write a temporary file unless something gives it one. This is that
// directory: it is created with the box, removed with it, and outside the
// worktree so a build's scratch files can never be read as part of the repair's
// diff. Nothing here grants it; the confinement profile is what does that.
func (b *Box) Temp() string { return b.temp }

// Commit is the canonical full object ID the box was cut from.
func (b *Box) Commit() string { return b.commit }

// Close discards the worktree, including any uncommitted or staged repair
// work. It is idempotent. Cleanup deliberately ignores cancellation of ctx so
// a timed-out dispatch still gets one bounded chance to remove its box.
func (b *Box) Close(ctx context.Context) error {
	if ctx == nil {
		return errors.New("repair box context is nil")
	}
	return b.close(context.WithoutCancel(ctx))
}

func (b *Box) close(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	if _, err := git(ctx, b.repository, b.deadline, nil, "worktree", "remove", "--force", "--", b.path); err != nil {
		return fmt.Errorf("discard repair worktree at %s: %w", b.path, err)
	}
	if b.temp != "" {
		if err := os.RemoveAll(b.temp); err != nil {
			return fmt.Errorf("discard repair box temporary directory at %s: %w", b.temp, err)
		}
	}
	b.closed = true
	return nil
}

func (b *Box) verifyHiddenTier() error {
	for _, protected := range []string{
		filepath.Join(b.path, ".kept", "ledger"),
		filepath.Join(b.path, ".kept", "artifacts"),
	} {
		if _, err := os.Lstat(protected); err == nil {
			return fmt.Errorf("repair worktree exposes machine-written data at %s", protected)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect repair worktree path %s: %w", protected, err)
		}
	}
	return nil
}

func (b *Box) snapshot(ctx context.Context) (string, error) {
	index, err := git(ctx, b.path, b.deadline, nil, "rev-parse", "--git-path", "index")
	if err != nil {
		return "", fmt.Errorf("locate repair worktree index: %w", err)
	}
	indexPath := strings.TrimSpace(string(index))
	if !filepath.IsAbs(indexPath) {
		indexPath = filepath.Join(b.path, indexPath)
	}
	temp, err := os.CreateTemp(filepath.Dir(indexPath), "kept-box-index-")
	if err != nil {
		return "", fmt.Errorf("create repair snapshot index: %w", err)
	}
	indexPath = temp.Name()
	if err := temp.Close(); err != nil {
		_ = os.Remove(indexPath)
		return "", fmt.Errorf("close repair snapshot index: %w", err)
	}
	defer os.Remove(indexPath)

	env := []string{"GIT_INDEX_FILE=" + indexPath}
	if _, err := git(ctx, b.path, b.deadline, env, "read-tree", "HEAD"); err != nil {
		return "", fmt.Errorf("prepare repair snapshot: %w", err)
	}
	if _, err := git(ctx, b.path, b.deadline, env, "add", "--all"); err != nil {
		return "", fmt.Errorf("stage repair snapshot: %w", err)
	}
	tree, err := git(ctx, b.path, b.deadline, env, "write-tree")
	if err != nil {
		return "", fmt.Errorf("write repair snapshot: %w", err)
	}
	return strings.TrimSpace(string(tree)), nil
}

func repositoryRoot(ctx context.Context, repository string, deadline time.Duration) (string, error) {
	if repository == "" {
		return "", errors.New("repair box repository is unset")
	}
	root, err := git(ctx, repository, deadline, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("find repair box repository: %w", err)
	}
	return filepath.EvalSymlinks(strings.TrimSpace(string(root)))
}

func pinnedCommit(ctx context.Context, repository, commit string, deadline time.Duration) (string, error) {
	if commit == "" {
		return "", fmt.Errorf("%w: commit is unset", ErrUnpinnedCommit)
	}
	resolved, err := git(ctx, repository, deadline, nil, "rev-parse", "--verify", "--end-of-options", commit+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("resolve repair box commit %q: %w", commit, err)
	}
	canonical := strings.TrimSpace(string(resolved))
	if !strings.EqualFold(commit, canonical) {
		return "", fmt.Errorf("%w: %q resolves to %s", ErrUnpinnedCommit, commit, canonical)
	}
	return canonical, nil
}

func validateOverlay(overlay []OverlayFile) error {
	seen := make(map[string]bool, len(overlay))
	for _, file := range overlay {
		clean, err := overlayPath(file.Path)
		if err != nil {
			return err
		}
		if seen[clean] {
			return fmt.Errorf("repair overlay names %q more than once", clean)
		}
		seen[clean] = true
	}
	return nil
}

func overlayPath(name string) (string, error) {
	if name == "" || filepath.IsAbs(name) || filepath.VolumeName(name) != "" {
		return "", fmt.Errorf("repair overlay path %q is not repository-relative", name)
	}
	clean := filepath.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("repair overlay path %q escapes the repository", name)
	}
	parts := strings.Split(filepath.ToSlash(clean), "/")
	if parts[0] == ".git" || (len(parts) >= 2 && parts[0] == ".kept" && (parts[1] == "ledger" || parts[1] == "artifacts")) {
		return "", fmt.Errorf("repair overlay path %q names machine-written data", name)
	}
	return clean, nil
}

func applyOverlay(root string, overlay []OverlayFile) error {
	for _, file := range overlay {
		clean, err := overlayPath(file.Path)
		if err != nil {
			return err
		}
		destination, err := overlayDestination(root, clean)
		if err != nil {
			return err
		}
		if file.Remove {
			if err := os.Remove(destination); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove repair overlay %s: %w", clean, err)
			}
			continue
		}
		if info, err := os.Lstat(destination); err == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("repair overlay %q would follow a symlink", clean)
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect repair overlay %s: %w", clean, err)
		}
		mode := file.Mode.Perm()
		if mode == 0 {
			mode = 0o644
		}
		if err := os.WriteFile(destination, file.Data, mode); err != nil {
			return fmt.Errorf("write repair overlay %s: %w", clean, err)
		}
		if err := os.Chmod(destination, mode); err != nil {
			return fmt.Errorf("set repair overlay mode %s: %w", clean, err)
		}
	}
	return nil
}

func overlayDestination(root, name string) (string, error) {
	current := root
	for _, part := range strings.Split(filepath.Dir(name), string(filepath.Separator)) {
		if part == "." || part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		switch {
		case errors.Is(err, os.ErrNotExist):
			if err := os.Mkdir(current, 0o755); err != nil {
				return "", fmt.Errorf("create repair overlay directory %s: %w", name, err)
			}
		case err != nil:
			return "", fmt.Errorf("inspect repair overlay directory %s: %w", name, err)
		case info.Mode()&os.ModeSymlink != 0:
			return "", fmt.Errorf("repair overlay %q would follow a symlink", name)
		case !info.IsDir():
			return "", fmt.Errorf("repair overlay %q has a non-directory parent", name)
		}
	}
	return filepath.Join(root, name), nil
}

// boundedGit runs one Git command whose output is presented to a person, and
// refuses rather than truncates. A truncated diff and a truncated path list
// both read as a smaller repair than the one that happened, which is the
// direction a reviewer cannot notice.
func boundedGit(ctx context.Context, directory string, deadline time.Duration, limit int, args ...string) ([]byte, error) {
	call, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()
	stdout := &bounded{limit: limit, stop: cancel}
	stderr := &bounded{limit: limit, stop: cancel}
	command := exec.CommandContext(call, "git", append([]string{"-C", directory}, args...)...)
	command.Env = withoutGitWorktreeEnv(os.Environ())
	command.Stdout = stdout
	command.Stderr = stderr
	err := command.Run()
	if stdout.over || stderr.over {
		return nil, fmt.Errorf("%w: limit %d bytes", ErrDiffTooLarge, limit)
	}
	if errors.Is(call.Err(), context.DeadlineExceeded) {
		return nil, fmt.Errorf("collect repair change: %w", call.Err())
	}
	if err != nil {
		return nil, fmt.Errorf("collect repair change: %w: %s", err, stderr.String())
	}
	return stdout.Bytes(), nil
}

type bounded struct {
	limit int
	stop  func()
	n     int
	over  bool
	b     bytes.Buffer
}

func (b *bounded) Write(src []byte) (int, error) {
	b.n += len(src)
	if b.n > b.limit {
		if !b.over {
			b.over = true
			b.stop()
		}
		return len(src), nil
	}
	_, _ = b.b.Write(src)
	return len(src), nil
}

func (b *bounded) Bytes() []byte { return append([]byte(nil), b.b.Bytes()...) }

func (b *bounded) String() string { return b.b.String() }

func git(ctx context.Context, directory string, deadline time.Duration, extraEnv []string, args ...string) ([]byte, error) {
	call, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()
	command := exec.CommandContext(call, "git", append([]string{"-C", directory}, args...)...)
	command.Env = append(withoutGitWorktreeEnv(os.Environ()), extraEnv...)
	output, err := command.CombinedOutput()
	if errors.Is(call.Err(), context.DeadlineExceeded) {
		return nil, fmt.Errorf("git %s: %w", strings.Join(args, " "), call.Err())
	}
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

func withoutGitWorktreeEnv(env []string) []string {
	filtered := make([]string, 0, len(env))
	for _, value := range env {
		name, _, _ := strings.Cut(value, "=")
		switch name {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR":
			continue
		}
		filtered = append(filtered, value)
	}
	return filtered
}
