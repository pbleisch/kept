//go:build !unix

package workspace

import "io/fs"

// statOf without an inode, which is what a platform that does not report one
// leaves. The digest is unaffected: stat is only the cache key, and content is
// the definition.
func statOf(info fs.FileInfo) statKey {
	return statKey{
		Size:    info.Size(),
		MtimeNS: info.ModTime().UnixNano(),
		Mode:    uint32(info.Mode()),
	}
}
