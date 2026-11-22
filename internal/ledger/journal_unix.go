//go:build unix

package ledger

import (
	"errors"
	"os"
	"syscall"
)

// tryLock takes an exclusive advisory lock without blocking, and reports
// whether it got one.
//
// The lock lives on the open file description rather than on the process, which
// is what makes it release on death however the process dies — the whole of
// what says a journal's writer is still running. It is the same mechanism
// internal/lock uses for the same reason, written again here because a ledger
// may not depend on a lock directory it does not own.
func tryLock(file *os.File) (bool, error) {
	err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, syscall.EWOULDBLOCK):
		return false, nil
	default:
		return false, err
	}
}
