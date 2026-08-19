package workspace

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// CacheFile is where the stat cache lives, under the derived tier.
//
// It is rebuildable, gitignored and disposable: losing it costs exactly one
// slow tick and can never corrupt a record, which is what docs/decision/0018's
// tier rule requires of anything that is not the corpus or the ledger.
const CacheFile = "cache/digest.json"

// cacheVersion is the shape of the file. A cache written under another one is
// discarded rather than read, because there is nothing to lose by rebuilding it
// and something to lose by misreading it.
const cacheVersion = 1

// racyWindow is git's racy-index rule.
//
// A file whose mtime falls inside the cache's own snapshot granularity may have
// been written after it was hashed and within the same clock tick, so its stat
// says nothing and it is read again. One second, because a coarse filesystem
// stamps whole seconds and the window has to cover the coarsest thing the cache
// might be sitting on.
//
// It narrows the window; it does not close it. Stat can still lie, and it lies
// in the silent direction: content changed, stat did not, the promise is never
// re-observed and nothing reports it. That is accepted deliberately — the
// alternative is reading the whole tree on every tick to close a window
// measured in nanoseconds — and --all is the answer when it matters. See
// docs/decision/0033.
const racyWindow = time.Second

// statKey is what a content digest is cached against: the four fields together,
// because each of them alone can stay put across a change.
type statKey struct {
	Size    int64  `json:"size"`
	MtimeNS int64  `json:"mtime_ns"`
	Inode   uint64 `json:"inode"`
	Mode    uint32 `json:"mode"`

	// blob is git's oid for this content when the index vouched for it. It is
	// not part of the key; it is what the second cache is keyed by.
	blob string
}

func (a statKey) same(b statKey) bool {
	return a.Size == b.Size && a.MtimeNS == b.MtimeNS && a.Inode == b.Inode && a.Mode == b.Mode
}

func statOf(info fs.FileInfo) statKey {
	k := statKey{
		Size:    info.Size(),
		MtimeNS: info.ModTime().UnixNano(),
		Mode:    uint32(info.Mode()),
	}
	if sys, ok := info.Sys().(*syscall.Stat_t); ok {
		k.Inode = uint64(sys.Ino)
	}
	return k
}

// Cache maps a path's stat to the digest of what was in it, and a git blob oid
// to the same. Two keys into one answer: stat catches a file nobody touched,
// and the oid catches a file git already hashed — a checkout changes every
// mtime and no blob oid, which is exactly when the first key misses and the
// second does not.
//
// The digest is the definition and stat is only the key. Two identical
// checkouts on different machines have different mtimes and different inodes,
// so a digest over stat would mean their observations never dedup — which
// breaks docs/decision/0013's rule that the same tree measured by the same
// gatherer is the same observation, whoever ran it.
type Cache struct {
	path string

	mu      sync.Mutex
	taken   time.Time
	entries map[string]statKey
	digests map[string]string // path -> content digest
	blobs   map[string]string // git blob oid -> content digest
	seen    map[string]bool
	seenBlb map[string]bool
}

type cacheFile struct {
	V     int                 `json:"v"`
	Taken time.Time           `json:"taken"`
	Stat  map[string]cacheRow `json:"stat"`
	Blobs map[string]string   `json:"blobs,omitempty"`
}

type cacheRow struct {
	statKey
	Digest string `json:"digest"`
}

// OpenCache reads the cache under a workspace's .kept, or returns an empty one.
// Every failure is silent by design: a cache that cannot be read is a cache
// that is not used, and the walk answers the same either way.
func OpenCache(kept string) *Cache {
	c := &Cache{
		path:    filepath.Join(kept, filepath.FromSlash(CacheFile)),
		entries: map[string]statKey{},
		digests: map[string]string{},
		blobs:   map[string]string{},
		seen:    map[string]bool{},
		seenBlb: map[string]bool{},
	}

	src, err := os.ReadFile(c.path)
	if err != nil {
		return c
	}
	var f cacheFile
	if err := json.Unmarshal(src, &f); err != nil || f.V != cacheVersion {
		return c
	}
	c.taken = f.Taken
	for path, row := range f.Stat {
		c.entries[path] = row.statKey
		c.digests[path] = row.Digest
	}
	for oid, digest := range f.Blobs {
		c.blobs[oid] = digest
	}
	return c
}

// byStat answers for a path whose stat is unchanged and outside the racy
// window.
func (c *Cache) byStat(path string, key statKey) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	held, ok := c.entries[path]
	if !ok || !held.same(key) {
		return "", false
	}
	if racy(key.MtimeNS, c.taken) {
		return "", false
	}
	digest, ok := c.digests[path]
	return digest, ok
}

// byBlob answers for content git has already hashed, whatever its mtime now is.
func (c *Cache) byBlob(oid string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	digest, ok := c.blobs[oid]
	if ok {
		c.seenBlb[oid] = true
	}
	return digest, ok
}

func (c *Cache) put(path string, key statKey, digest string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[path] = key
	c.digests[path] = digest
	c.seen[path] = true
	if key.blob != "" {
		c.blobs[key.blob] = digest
		c.seenBlb[key.blob] = true
	}
}

// racy reports whether an mtime is too close to the snapshot to be evidence of
// anything.
func racy(mtimeNS int64, taken time.Time) bool {
	if taken.IsZero() {
		return true
	}
	return mtimeNS >= taken.Add(-racyWindow).UnixNano()
}

// Save writes the cache back, keeping only what this walk saw, so a path that
// went away does not accumulate. It is written whole and renamed into place: a
// crash leaves the previous cache rather than a torn one, and a torn one would
// cost a slow tick rather than a wrong answer.
//
// Taken is stamped now, which is what makes the racy window self-healing: an
// entry that was too fresh to trust in one walk is old enough in the next.
func (c *Cache) Save() error {
	c.mu.Lock()
	f := cacheFile{V: cacheVersion, Taken: time.Now(), Stat: map[string]cacheRow{}, Blobs: map[string]string{}}
	for path, key := range c.entries {
		if !c.seen[path] {
			continue
		}
		f.Stat[path] = cacheRow{statKey: key, Digest: c.digests[path]}
	}
	for oid, digest := range c.blobs {
		if c.seenBlb[oid] {
			f.Blobs[oid] = digest
		}
	}
	c.mu.Unlock()

	body, err := json.Marshal(f)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(c.path), "."+filepath.Base(c.path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), c.path)
}

// Discard removes the cache. Nothing derived from it is lost, which is the
// property that lets it be thrown away on any doubt.
func Discard(kept string) error {
	err := os.Remove(filepath.Join(kept, filepath.FromSlash(CacheFile)))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
