//go:build !unix

package lock

import (
	"errors"
	"os"
)

// tryLock has no implementation off unix. kept's confinement is macOS-only
// (docs/decision/0053), so this exists to keep the package compiling rather
// than to offer a second mechanism — and it refuses rather than pretending to
// have taken a lock, because a lock that silently grants is worse than none.
func tryLock(*os.File) (bool, error) {
	return false, errors.New("kept has no file lock on this platform")
}
