// Package box gives a thread an isolated worktree cut from the origin, and
// lands named paths back.
//
// Two things that were learned the hard way elsewhere: a bare worktree checks
// out a commit and silently loses uncommitted work, so the origin's dirty
// files are overlaid; and paths outside the artifact's declared scope are
// reported rather than landed, because work outside a scope is not covered by
// the promise at all.
package box
