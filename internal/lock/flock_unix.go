//go:build unix

package lock

import (
	"errors"
	"os"
	"syscall"
)

// tryLock takes an exclusive advisory lock without blocking, and reports
// whether it got one.
//
// The lock lives on the open file description rather than on the process, which
// is what makes it release on death — and what makes two opens of one path
// inside a single process exclude each other, so a test does not need two
// processes to observe the invariant.
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
