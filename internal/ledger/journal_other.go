//go:build !unix

package ledger

import "os"

// tryLock has no implementation off unix, so no run there owns a journal and
// every receipt entry is appended to the ledger as it is produced, which is
// where a receipt has always gone when there is nowhere to journal.
//
// It reports no error, unlike internal/lock's counterpart. There, a lock that
// cannot be taken is a failure, because the lock is the mutual exclusion. Here
// a lock nobody can take means only *there is no journal on this platform*, and
// a receipt written straight to the ledger is whole: every entry is durable
// before the write that produced it returns, so a kill of kept still loses no
// line it had read. That configuration is measured on every platform by
// TestASlowLedgerWithNoJournalHoldsEveryLineItAccepted in internal/thread,
// which runs a slow store with nowhere to journal rather than needing one
// without an flock.
//
// What this platform does not have is the pace. docs/decision/0084's other
// guarantee — that the runtime's own queue stays empty, so a runtime which
// discards unread output at exit discards nothing — was the 64 MiB spool's, and
// 0087 retired the spool because the journal replaced it. Off unix neither
// exists, and a session read at the pace of a ref would lose the tail the
// runtime drops. That is not a regression anyone is streaming through: there is
// no confinement off macOS at all (docs/decision/0053), so what this file keeps
// alive is a package that compiles and a ledger that can be read, not a second
// way of running a session. It is written this way rather than refusing, as
// internal/lock's counterpart does, because a ledger with no journal is a
// configuration kept supports on every platform and this is how that
// configuration is reached here.
func tryLock(*os.File) (bool, error) {
	return false, nil
}
