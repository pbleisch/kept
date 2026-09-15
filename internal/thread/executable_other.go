//go:build !unix

package thread

import "os"

// inodeOf has no inode to read here, so a replaced executable is told apart by
// its size and modification time alone.
func inodeOf(os.FileInfo) uint64 { return 0 }
