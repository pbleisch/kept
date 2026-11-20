package ledger

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Dir is the ledger's directory under .kept.
const Dir = "ledger"

// ErrNoStream is returned by [Store.Read] for a stream nothing has written yet.
// It is distinct from an empty stream on purpose: a ledger that has never been
// written and one whose records were all pruned look the same to a fold and
// should not look the same to a person. A fresh clone that reverted every
// promise to unratified is the failure docs/layout.md warns about, and the way
// to notice it is to be able to say which of the two happened.
var ErrNoStream = errors.New("no such ledger stream")

// Store is where the ledger's bytes live.
//
// It is three operations because the ledger needs three and no more: append one
// line to a stream, read a stream back, and write one whole file that has
// exactly one writer. Anything wider would be a filesystem, and the point of
// the seam is that the next implementation is not one — the ledger belongs on
// refs/kept/ledger, written with a temporary index and read with git show, so
// that no working tree checks it out and no box contains it. See
// docs/decision/0016. Session records — receipts and the payloads they name —
// go through the same three operations and are filed on refs/kept/sessions
// instead, which origin never receives; the implementation decides that by
// path, so a caller never does. See docs/decision/0089.
type Store interface {
	// Append adds one line to a stream. The line is written whole, so a reader
	// sees a complete record or nothing, and the append is durable before it
	// returns — a ratification is one of the two acts docs/decision/0018 says
	// is worth the cost.
	Append(stream string, line []byte) error

	// Read returns a whole stream, or ErrNoStream if nothing has written it.
	Read(stream string) ([]byte, error)

	// Put writes one whole file. It is for records with exactly one writer —
	// a run's diagnostics — which is why it needs no merge strategy and no
	// append semantics.
	Put(path string, content []byte) error
}

// Files is the local-only store for a workspace outside Git. A repair needs a
// Git worktree, so a Git workspace always uses RefStore instead; keeping this
// implementation preserves kept's read-and-record workflow for a manuscript
// before it becomes a repository.
type Files struct {
	// Dir is the ledger root, ordinarily <workspace>/.kept/ledger.
	Dir string
}

// NewFiles returns a local-only store rooted at a workspace's .kept/ledger.
func NewFiles(kept string) *Files { return &Files{Dir: filepath.Join(kept, Dir)} }

func (f *Files) path(name string) string { return filepath.Join(f.Dir, filepath.FromSlash(name)) }

// Append opens for append and writes the line in one call, so two concurrent
// runs appending to one stream interleave by record and never within one.
func (f *Files) Append(stream string, line []byte) error {
	path := f.path(stream)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	out := make([]byte, 0, len(line)+1)
	out = append(out, line...)
	out = append(out, '\n')
	if _, err := file.Write(out); err != nil {
		return err
	}
	return file.Sync()
}

func (f *Files) Read(stream string) ([]byte, error) {
	src, err := os.ReadFile(f.path(stream))
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%s: %w", stream, ErrNoStream)
	}
	return src, err
}

// Put writes through a temporary file and renames, so a crash mid-write leaves
// the previous content rather than a truncated file. The path has one writer,
// so there is nothing to arbitrate and no merge strategy to choose.
func (f *Files) Put(path string, content []byte) error {
	full := f.path(path)
	dir := filepath.Dir(full)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(full)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), full)
}
