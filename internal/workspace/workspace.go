package workspace

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

// Excluded are the directories no ignore file can re-include.
//
// .git is not content. .kept is the tick feeding itself: while the ledger is
// plain files inside the workspace, recording a truth transition would change
// the world, which would make every promise due, which would record more.
// docs/decision/0016 removes the cause by moving the ledger to a ref no
// worktree checks out; until then this is a stated rule rather than a
// consequence of layout.
var Excluded = []string{".git", ".kept"}

// Options are the accelerations and what a workspace excludes beyond its
// ignore files. The zero value is the plain definition: walk everything that is
// not ignored, read every file, one worker at a time.
type Options struct {
	// Exclude is what config.yaml adds, in the ignore format, applied at the
	// root. Nothing beyond .git and .kept is excluded by default.
	Exclude []string
	// Cache is the stat cache. Nil reads every file.
	Cache *Cache
	// Workers is how many files are hashed at once. Zero is one.
	Workers int
	// All forces every file to be read, whatever any cache says. It is the
	// answer to the one failure the cache can have that nothing reports: stat
	// lying in the silent direction.
	All bool
	// NoGit declines git's index as a pre-warmed cache. The accelerator must
	// agree with the plain path, and this is what lets a test ask both.
	NoGit bool
}

// File is one path that counts, and the digest of what is in it.
type File struct {
	// Path is workspace-relative and slash-separated on every platform, which
	// is what a scope's globs are matched against.
	Path string
	// Digest is sha256 over the file's bytes, lowercase hex. For a symlink it
	// is over the target it names, tagged so that a link and a file holding
	// the same text are not one content.
	Digest string
	Size   int64
}

// Tree is one walk of a workspace: every path that counts, sorted by path.
// The world's digest and every scope's digest are folds over it, which is why
// a tick costs one walk rather than one walk per scope.
type Tree struct {
	Root  string
	Files []File
	// Read is how many files were hashed rather than served from a cache. It
	// is diagnostic — the difference between a cold tick and a warm one.
	Read int
}

// Read walks the workspace once and digests everything in it that counts.
func Read(root string, opt Options) (*Tree, error) {
	root = filepath.Clean(root)
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s: not a directory", root)
	}

	w := &walker{root: root, opt: opt}
	if len(opt.Exclude) > 0 {
		w.config = parseIgnore("", strings.Join(opt.Exclude, "\n"))
	}
	if !opt.NoGit && !opt.All {
		w.index = readGitIndex(root)
	}

	if err := w.walk(""); err != nil {
		return nil, err
	}
	if err := w.digest(); err != nil {
		return nil, err
	}

	sort.Slice(w.found, func(i, j int) bool { return w.found[i].Path < w.found[j].Path })
	return &Tree{Root: root, Files: w.found, Read: w.read}, nil
}

type walker struct {
	root   string
	opt    Options
	config *ignoreFile
	index  *gitIndex

	stack   ignoreStack
	found   []File
	pending []*candidate
	read    int
}

// candidate is a path that counts, before its content is known. Hashing is
// deferred so the cold path can be parallelised, which is the third of 0033's
// accelerations.
type candidate struct {
	path   string
	full   string
	link   bool
	stat   statKey
	digest string
	err    error
}

// walk descends one directory, pushing its ignore file for the subtree below.
func (w *walker) walk(rel string) error {
	dir := w.root
	if rel != "" {
		dir = filepath.Join(w.root, filepath.FromSlash(rel))
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	depth := len(w.stack)
	for _, e := range entries {
		if e.Name() == ".gitignore" && !e.IsDir() {
			src, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				return err
			}
			w.stack = append(w.stack, parseIgnore(rel, string(src)))
			break
		}
	}
	defer func() { w.stack = w.stack[:depth] }()

	for _, e := range entries {
		name := e.Name()
		child := name
		if rel != "" {
			child = rel + "/" + name
		}

		if e.IsDir() {
			if excluded(name) || w.ignored(child, true) {
				continue
			}
			if err := w.walk(child); err != nil {
				return err
			}
			continue
		}

		mode := e.Type()
		if mode&fs.ModeSymlink == 0 && !mode.IsRegular() {
			// A socket or a fifo is not content, and reading one blocks.
			continue
		}
		if w.ignored(child, false) {
			continue
		}

		info, err := e.Info()
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				// The walk raced a deletion. The tree is being measured, not
				// locked, and a file that is gone is not part of it.
				continue
			}
			return err
		}
		w.pending = append(w.pending, &candidate{
			path: child,
			full: filepath.Join(dir, name),
			link: mode&fs.ModeSymlink != 0,
			stat: statOf(info),
		})
	}
	return nil
}

// ignored asks what config.yaml excludes first, and the workspace's ignore
// files after.
//
// The order is the rule: an ignore file cannot re-include what the config
// excluded. A config is the operator's statement about this workspace and a
// .gitignore is the repository's statement to git, so where the two disagree
// about what counts, the operator's is the one kept was configured with.
func (w *walker) ignored(path string, isDir bool) bool {
	if w.config != nil {
		if matched, ignored := w.config.match(path, isDir); matched {
			return ignored
		}
	}
	return w.stack.ignored(path, isDir)
}

func excluded(name string) bool {
	for _, e := range Excluded {
		if name == e {
			return true
		}
	}
	return false
}

// digest fills in every candidate's content digest, from the stat cache, from
// git's index by way of a blob already digested, or by reading the file.
func (w *walker) digest() error {
	work := make(chan *candidate)
	workers := w.opt.Workers
	if workers < 1 {
		workers = 1
	}
	if workers > runtime.GOMAXPROCS(0) {
		workers = runtime.GOMAXPROCS(0)
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for c := range work {
				digest, err := content(c)
				if err != nil {
					c.err = err
					continue
				}
				c.digest = digest
				mu.Lock()
				w.read++
				mu.Unlock()
			}
		}()
	}

	for _, c := range w.pending {
		if w.cached(c) {
			continue
		}
		work <- c
	}
	close(work)
	wg.Wait()

	for _, c := range w.pending {
		if c.err != nil {
			if errors.Is(c.err, os.ErrNotExist) {
				continue
			}
			return fmt.Errorf("%s: %w", c.path, c.err)
		}
		w.found = append(w.found, File{Path: c.path, Digest: c.digest, Size: c.stat.Size})
		w.remember(c)
	}
	return nil
}

// cached answers a candidate from what has already been digested: this
// workspace's stat cache first, then git's index by way of the blob oid it
// already computed. Both are keys into the same content digest, and neither is
// consulted under All.
func (w *walker) cached(c *candidate) bool {
	if w.opt.All || w.opt.Cache == nil {
		return false
	}
	if digest, ok := w.opt.Cache.byStat(c.path, c.stat); ok {
		c.digest = digest
		return true
	}
	if w.index == nil || c.link {
		return false
	}
	oid, ok := w.index.blob(c.path, c.stat)
	if !ok {
		return false
	}
	c.stat.blob = oid
	digest, ok := w.opt.Cache.byBlob(oid)
	if !ok {
		return false
	}
	c.digest = digest
	return true
}

func (w *walker) remember(c *candidate) {
	if w.opt.Cache == nil {
		return
	}
	w.opt.Cache.put(c.path, c.stat, c.digest)
}

// content is the definition: sha256 over what is in the file.
//
// A symlink is hashed over the target it names rather than followed. Following
// would leave the workspace and could loop, and the tag keeps a link and a file
// holding the same text from being one content.
func content(c *candidate) (string, error) {
	if c.link {
		target, err := os.Readlink(c.full)
		if err != nil {
			return "", err
		}
		return sum([]byte("link:" + target)), nil
	}
	return sumFile(c.full)
}

// Rel returns a path inside the workspace as the digest names it: relative to
// the root and slash-separated.
func Rel(root, p string) (string, bool) {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "", false
	}
	return path.Clean(rel), true
}
