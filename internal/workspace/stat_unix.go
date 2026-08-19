//go:build unix

package workspace

import (
	"io/fs"
	"syscall"
)

// statOf is the cache key: the four fields together, because each of them alone
// can stay put across a change. The inode is the one that needs the platform,
// and where it cannot be had the other three still key the cache — a file
// replaced by a rename with the same size, mode and mtime is then the one case
// this misses, which is what --all is for.
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
