package land

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pbleisch/kept/internal/box"
	"github.com/pbleisch/kept/internal/workspace"
)

// ErrTorn means the working tree moved between the gate and the write.
//
// It is the failure docs/decision/0015's third compare-and-swap exists to
// catch: a worktree arrival's whole guarantee is *the change was gated against
// exactly the tree it produced, and the tree it was written into had not moved
// since the gate*, and a tree that moved makes the second half false. Nothing
// is written, and nothing is wrong — the answer is to run it again.
var ErrTorn = errors.New("the working tree moved between the gate and the write")

// maxArrivalBytes bounds what one arrival brings into memory before it writes
// anything.
//
// Every file is read out of the object store in full before the first one is
// written, because that is what makes a failure to read a refusal rather than a
// half-written tree. A bound is the cost of that: a change larger than this is
// refused by name rather than taking the process down, and a repair's diff is
// smaller than this by construction.
const maxArrivalBytes = 64 << 20

// arrival is what a worktree arrival is about to do, computed whole before any
// of it is done.
type arrival struct {
	// Write is each path the merge changes and the bytes to put there.
	Write []arrivalFile
	// Delete is each path the merge removes.
	Delete []string
}

// arrivalFile is one path the merge produces, with the mode git recorded for
// it. The content is held rather than streamed for the reason [maxArrivalBytes]
// gives.
type arrivalFile struct {
	Path    string
	Mode    string
	Content []byte
}

// Paths is every path this arrival touches, sorted, which is what the report
// says arrived.
func (a arrival) Paths() []string {
	out := make([]string, 0, len(a.Write)+len(a.Delete))
	for _, f := range a.Write {
		out = append(out, f.Path)
	}
	out = append(out, a.Delete...)
	sortStrings(out)
	return out
}

// arriveInWorktree merges the change into the working tree the operator is
// standing in, and moves nothing: no ref, no commit, no index entry. What
// happens to it next is theirs.
//
// The order is the whole of the safety property. Everything that can refuse
// happens before anything is written:
//
//	re-read the tree's digest    ← the compare-and-swap; a tree that moved refuses
//	snapshot the tree            ← into a scratch index, so the operator's is untouched
//	three-way merge              ← base target, ours the tree, theirs the merge
//	read every changed blob      ← in full, so a read that fails writes nothing
//	write, then delete           ← the only part that touches the operator's files
//
// A conflict, a torn tree, a file kept cannot merge and a change too large to
// hold are all refusals, and each of them leaves the working tree exactly as it
// was. A half-applied merge into a tree the operator is standing in is the
// worst thing this can do — worse than not landing at all — so the write is the
// last step and nothing after it can fail in a way that needs one.
//
// A crash *during* the write leaves docs/decision/0018's third state, which no
// arrival onto a ref has. That is a property of writing several files without a
// transaction and is accepted there rather than papered over here.
func arriveInWorktree(ctx context.Context, req Request, target, merged, gateDigest string, who box.Identity) (arrival, error) {
	moved, err := workspaceDigest(req)
	if err != nil {
		return arrival{}, err
	}
	if moved != gateDigest {
		return arrival{}, fmt.Errorf("%w: it was gated against %s and now digests %s", ErrTorn, short(gateDigest), short(moved))
	}

	snapshot, err := snapshotWorktree(ctx, req, target, who)
	if err != nil {
		return arrival{}, err
	}
	tree, err := mergeIntoSnapshot(ctx, req, target, snapshot, merged)
	if err != nil {
		return arrival{}, err
	}
	plan, err := readArrival(ctx, req, snapshot, tree)
	if err != nil {
		return arrival{}, err
	}
	if err := plan.apply(req.Workspace); err != nil {
		return plan, err
	}
	return plan, nil
}

// workspaceDigest is docs/decision/0033's digest over the operator's workspace:
// the same number in a repository, outside one, and in a box, computed from
// bytes rather than asked of git.
//
// All: true, because the swap is the one place a stat cache lying in the silent
// direction would cost the operator their work.
func workspaceDigest(req Request) (string, error) {
	walked, err := workspace.Read(req.Workspace, workspace.Options{
		Exclude: req.Exclude,
		Workers: runtime.NumCPU(),
		All:     true,
	})
	if err != nil {
		return "", err
	}
	return walked.Digest(), nil
}

// snapshotWorktree writes the working tree as it stands into a commit on no
// branch, so the merge has something to state as *ours*.
//
// Through a scratch index rather than the operator's, which is what keeps a
// land from staging anything: the real index is copied so the stat cache still
// serves, `git add -A` records what is on disk over the top of it, and the
// operator's own index file is never opened for writing. The commit is
// unreferenced and becomes garbage, which is the same cost docs/decision/0068
// names for a dirty repair's snapshot.
func snapshotWorktree(ctx context.Context, req Request, target string, who box.Identity) (string, error) {
	scratch, err := os.CreateTemp("", "kept-land-index-")
	if err != nil {
		return "", err
	}
	index := scratch.Name()
	_ = scratch.Close()
	defer func() { _ = os.Remove(index) }()

	if err := copyIndex(ctx, req, index); err != nil {
		return "", err
	}
	if _, err := gitEnv(ctx, req, []string{"GIT_INDEX_FILE=" + index}, append([]string{"add", "--all"}, notKept...)...); err != nil {
		return "", fmt.Errorf("read the working tree: %w", err)
	}
	tree, err := gitEnv(ctx, req, []string{"GIT_INDEX_FILE=" + index}, "write-tree")
	if err != nil {
		return "", fmt.Errorf("read the working tree: %w", err)
	}
	commit, err := gitEnv(ctx, req, []string{
		"GIT_AUTHOR_NAME=" + who.Name, "GIT_AUTHOR_EMAIL=" + who.Email,
		"GIT_COMMITTER_NAME=" + who.Name, "GIT_COMMITTER_EMAIL=" + who.Email,
	}, "-c", "commit.gpgSign=false", "commit-tree", strings.TrimSpace(tree), "-p", target,
		"-m", "the working tree as this land found it")
	if err != nil {
		return "", fmt.Errorf("read the working tree: %w", err)
	}
	return strings.TrimSpace(commit), nil
}

// copyIndex seeds the scratch index from the repository's own, so `git add -A`
// stats rather than re-hashes. A repository with no index yet is not an error:
// an empty scratch index reads the whole tree, which is the same answer more
// slowly.
func copyIndex(ctx context.Context, req Request, into string) error {
	dir, err := gitValue(ctx, req, "rev-parse", "--git-dir")
	if err != nil {
		return err
	}
	from := strings.TrimSpace(dir)
	if !filepath.IsAbs(from) {
		from = filepath.Join(req.Workspace, from)
	}
	src, err := os.Open(filepath.Join(from, "index"))
	if err != nil {
		return nil
	}
	defer func() { _ = src.Close() }()
	dst, err := os.OpenFile(into, os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = dst.Close() }()
	if _, err := io.Copy(dst, src); err != nil {
		return err
	}
	return dst.Close()
}

// mergeIntoSnapshot is the three-way merge, with the base stated: the target
// the gate measured, the operator's tree as ours, and the gated merge as
// theirs.
//
// The base is stated for docs/decision/0068's reason and for one more of this
// arrival's own: git would compute a base from the graph, and the operator's
// working tree is on no graph at all.
func mergeIntoSnapshot(ctx context.Context, req Request, base, ours, theirs string) (string, error) {
	deadline := req.Deadlines.Git
	out, _, code, err := gitCode(ctx, req.Workspace, deadline,
		"merge-tree", "--write-tree", "--merge-base="+base, "--", ours, theirs)
	if err != nil {
		return "", err
	}
	first, rest, _ := strings.Cut(strings.TrimRight(out, "\n"), "\n")
	switch {
	case code == 1:
		return "", fmt.Errorf("%w into this working tree: %s", box.ErrConflicted, mergeMessages(rest))
	case code != 0:
		return "", fmt.Errorf("merge into this working tree: git exited %d: %s", code, strings.TrimSpace(out))
	case !objectID(first):
		return "", fmt.Errorf("git returned %q rather than a tree id", first)
	}
	return first, nil
}

// mergeMessages is the part of a conflicted merge a person reads. merge-tree
// reports the conflicted index entries first and its own messages after a blank
// line; the messages are the report, and the index lines are kept only when
// there are none so nothing is dropped to silence.
func mergeMessages(rest string) string {
	if _, messages, found := strings.Cut(rest, "\n\n"); found && strings.TrimSpace(messages) != "" {
		return strings.TrimSpace(messages)
	}
	return strings.TrimSpace(rest)
}

// readArrival works out what the merge changes about this working tree, and
// reads every byte of it, before anything is written.
//
// The diff is against the snapshot rather than against the target: a path the
// change touches and the operator has not is a write, and a path the operator
// already holds the merged content of is nothing at all — which is what makes
// landing the same anchor twice a no-op here, the way it is on a ref.
func readArrival(ctx context.Context, req Request, snapshot, tree string) (arrival, error) {
	names, err := gitValue(ctx, req, "diff", "--no-renames", "--name-status", "-z",
		"--no-ext-diff", "--no-textconv", snapshot, tree)
	if err != nil {
		return arrival{}, err
	}
	fields := strings.Split(strings.TrimSuffix(names, "\x00"), "\x00")

	var plan arrival
	var wanted []string
	for i := 0; i+1 < len(fields); i += 2 {
		status, path := fields[i], fields[i+1]
		if status == "" || path == "" {
			continue
		}
		if status[0] == 'D' {
			plan.Delete = append(plan.Delete, path)
			continue
		}
		wanted = append(wanted, path)
	}
	if len(wanted) == 0 {
		sortStrings(plan.Delete)
		return plan, nil
	}

	files, err := lsTree(ctx, req, tree, wanted)
	if err != nil {
		return arrival{}, err
	}
	total := 0
	for _, path := range wanted {
		entry, found := files[path]
		if !found {
			return arrival{}, fmt.Errorf("the merge changed %s and the merged tree holds no such path", path)
		}
		switch entry.mode {
		case "100644", "100755", "120000":
		case "160000":
			return arrival{}, fmt.Errorf("%s is a submodule, and a worktree arrival has no way to bring one across", path)
		default:
			return arrival{}, fmt.Errorf("%s has mode %s, which is not a file a worktree arrival can write", path, entry.mode)
		}
		content, err := catBlob(ctx, req, entry.oid)
		if err != nil {
			return arrival{}, err
		}
		total += len(content)
		if total > maxArrivalBytes {
			return arrival{}, fmt.Errorf("this change brings more than %d MiB into the working tree, which is more than a worktree arrival reads at once; land it onto the ref instead", maxArrivalBytes>>20)
		}
		plan.Write = append(plan.Write, arrivalFile{Path: path, Mode: entry.mode, Content: content})
	}
	sortStrings(plan.Delete)
	return plan, nil
}

// objectID reports whether a value git handed back is an object id. A value
// out of a subprocess is checked before it becomes an argument to the next one.
func objectID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, c := range value {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

type treeEntry struct {
	mode string
	oid  string
}

// lsTree reads the mode and object of each path out of the merged tree. The
// mode is read rather than assumed: an executable bit and a symlink are both
// part of the change, and a worktree arrival that dropped either would land
// something the gate did not measure.
func lsTree(ctx context.Context, req Request, tree string, paths []string) (map[string]treeEntry, error) {
	args := append([]string{"ls-tree", "-z", "--full-tree", "--end-of-options", tree, "--"}, paths...)
	out, err := gitValue(ctx, req, args...)
	if err != nil {
		return nil, err
	}
	entries := map[string]treeEntry{}
	for _, record := range strings.Split(strings.TrimSuffix(out, "\x00"), "\x00") {
		if record == "" {
			continue
		}
		meta, path, found := strings.Cut(record, "\t")
		if !found {
			continue
		}
		parts := strings.Fields(meta)
		if len(parts) < 3 {
			continue
		}
		entries[path] = treeEntry{mode: parts[0], oid: parts[2]}
	}
	return entries, nil
}

// catBlob reads one object's bytes. Standard output alone, because Apple's git
// shim writes to standard error on every invocation and a blob read out of a
// merge of the two is that noise with the file somewhere in it.
func catBlob(ctx context.Context, req Request, oid string) ([]byte, error) {
	out, err := gitValue(ctx, req, "cat-file", "blob", oid)
	if err != nil {
		return nil, err
	}
	return []byte(out), nil
}

// apply is the only part of a worktree arrival that touches the operator's
// files. Everything that can refuse has already refused.
//
// Each file is written beside its destination and renamed into place, so no
// path is ever observed half-written. The set of them is not a transaction and
// cannot be — that is the third state docs/decision/0018 names.
func (a arrival) apply(root string) error {
	for _, f := range a.Write {
		if err := writeArrivalFile(root, f); err != nil {
			return err
		}
	}
	for _, path := range a.Delete {
		full, err := underRoot(root, path)
		if err != nil {
			return err
		}
		if err := os.Remove(full); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		pruneEmpty(root, filepath.Dir(full))
	}
	return nil
}

func writeArrivalFile(root string, f arrivalFile) error {
	full, err := underRoot(root, f.Path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	if f.Mode == "120000" {
		// A symlink is replaced rather than written through: writing through
		// one would follow it and put the change wherever it points, which is
		// a path the scope review never saw.
		if err := os.Remove(full); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return os.Symlink(string(f.Content), full)
	}

	mode := os.FileMode(0o644)
	if f.Mode == "100755" {
		mode = 0o755
	}
	temp, err := os.CreateTemp(filepath.Dir(full), ".kept-land-*")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer func() { _ = os.Remove(name) }()
	if _, err := temp.Write(f.Content); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, mode); err != nil {
		return err
	}
	// A destination that is itself a symlink is removed first, for the reason
	// above: a rename over one replaces it, but an existing link the merge is
	// not changing must not be followed.
	if info, err := os.Lstat(full); err == nil && info.Mode()&os.ModeSymlink != 0 {
		if err := os.Remove(full); err != nil {
			return err
		}
	}
	return os.Rename(name, full)
}

// underRoot resolves a workspace-relative path git reported and refuses one
// that leaves the workspace. Git does not produce such a path, and a value that
// becomes a filesystem write is checked anyway.
func underRoot(root, path string) (string, error) {
	if path == "" || filepath.IsAbs(path) {
		return "", fmt.Errorf("%q is not a path inside this working tree", path)
	}
	full := filepath.Join(root, filepath.FromSlash(path))
	rel, err := filepath.Rel(root, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%q is not a path inside this working tree", path)
	}
	return full, nil
}

// pruneEmpty removes directories a delete emptied, up to but never including
// the workspace root. It is what git does after a checkout that removes the
// last file in a directory, and a directory left behind would show up in
// nothing and confuse the next walk.
func pruneEmpty(root, dir string) {
	for {
		if samePath(dir, root) || dir == "." || dir == string(filepath.Separator) {
			return
		}
		if err := os.Remove(dir); err != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}
