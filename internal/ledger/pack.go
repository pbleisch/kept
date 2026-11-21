package ledger

import (
	"context"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// PackThreshold is how many bytes of content a process hands git for one
// repository, through the ref store, before it packs that repository's loose
// objects. A store opened by [NewRefStore] packs at this; a test that needs to
// see a packing happen sets its own store's bound lower. See
// docs/decision/0092.
//
// Every ledger and session write is plumbing: hash-object, write-tree,
// commit-tree, update-ref. None of those runs git's own automatic maintenance,
// which only porcelain triggers, so every object kept writes stays loose until
// somebody runs git gc by hand. That is ordinary for a commit's worth of
// objects and it is not ordinary for a receipt: a receipt stream is rewritten
// whole on every line, so an episode leaves one loose copy of it per line, and
// two real repair runs left 101.9 GiB and 53.6 GiB that a git gc then packed
// into 136 MiB and 71 MiB.
//
// It counts bytes rather than objects because bytes are what fill a disk, and
// git's own trigger (gc.auto, 6,700 loose objects) is an object count that a
// receipt reaches only after several gigabytes. It counts what kept handed git,
// uncompressed, because that is known at the write without asking git
// anything; what lands on disk is that compressed, so the loose bytes on disk
// at any moment stay below this.
const PackThreshold int64 = 256 << 20

// packDeadline bounds one packing. It is longer than [DefaultRefDeadline]
// because a repack reads every loose object and a roll-up rewrites packs, and a
// repack cut off by it leaves the repository as it found it: git writes the new
// pack under a temporary name and removes nothing until it is complete.
const packDeadline = 5 * time.Minute

// handed is the bytes this process has handed git for each repository since it
// last packed it, keyed by the repository path a store was opened with. It is
// process-wide rather than per store, because a run opens more than one store
// over one repository and each would otherwise count only its own share.
var handed sync.Map // string -> *atomic.Int64

// packing is the repositories a packing is running in, so that writers which
// cross the threshold while one runs do not start a second.
var packing sync.Map // string -> struct{}

// wrote accounts for content handed git by a write that landed, and packs the
// repository once the account crosses the store's bound, which is
// [PackThreshold] unless a test lowered it. Zero never packs.
//
// It runs in the writer, after the ref moved. The write is already durable by
// then, so nothing about the packing can undo it: a packing that fails, is cut
// off or is skipped leaves the objects loose, exactly as they were before this
// existed, and the next crossing tries again.
func (s *RefStore) wrote(bytes int) {
	if s.packAt <= 0 || bytes <= 0 {
		return
	}
	counter, _ := handed.LoadOrStore(s.repository, new(atomic.Int64))
	total := counter.(*atomic.Int64).Add(int64(bytes))
	if total < s.packAt {
		return
	}
	if _, running := packing.LoadOrStore(s.repository, struct{}{}); running {
		return
	}
	defer packing.Delete(s.repository)
	counter.(*atomic.Int64).Store(0)
	s.pack()
}

// pack packs the repository's loose objects, and rolls its packs up so that
// there are few of them.
//
// `repack --geometric=2 -d` does both in one command: it puts every loose
// object into a pack and merges the smallest packs until each is at least
// twice the size of the next, so the number of packs grows with the logarithm
// of what was written rather than with the number of packings. Merging the
// small packs is also what lets git store each receipt version as a delta of
// the last rather than once per pack.
//
// An operator who set gc.auto to 0 has said this repository is never packed
// automatically, and kept honours that the way git's own porcelain does.
func (s *RefStore) pack() {
	ctx, cancel := context.WithTimeout(context.Background(), packDeadline)
	defer cancel()
	setting, err := exec.CommandContext(ctx, "git", "-C", s.repository, "config", "--get", "gc.auto").Output()
	if err == nil && strings.TrimSpace(string(setting)) == "0" {
		return
	}
	_ = exec.CommandContext(ctx, "git", "-C", s.repository, "repack", "-d", "-q", "--geometric=2").Run()
}
