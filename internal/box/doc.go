// Package box gives a thread a disposable worktree cut from one pinned origin
// commit.
//
// A box does not confine a process. It is the tree a repair changes and the
// baseline from which the parent collects a bounded diff; confinement and
// landing are separate boundaries. The caller supplies an explicit dirty
// overlay, so the box carries selected operator work without silently taking
// unrelated changes from the origin checkout.
//
// The ledger and repair artifacts live on their own ref. A box refuses a
// checkout or overlay that would put either machine-written tier in the
// repairer's tree, rather than depending on the side ref alone.
package box
