// Package workspace is what the world looks like to kept: which paths count,
// what a glob means, and the content digest that says whether anything changed.
//
// One definition, stated once and computed several ways: **the world's tree is
// a content digest over the workspace root, honouring ignore rules** — the same
// digest in a repository, outside one, and inside a box. A scope's digest is the
// same fold over the paths a promise's globs match. Neither asks git anything.
// See docs/decision/0033.
//
// # Why not git
//
// The mechanism decision 0025 named — git rev-parse HEAD:<path> — cannot resolve
// a glob and fails by succeeding, cannot see an unstaged edit, and compares a
// whole tree against some paths. And a git tree oid as the definition would make
// one tree produce two identities depending on whether git happened to be
// installed, splitting the observation cache, the truth series and prior() with
// nothing reporting it.
//
// # The accelerations are not the definition
//
// Four of them, and none changes what the number means: honour ignore rules,
// keep a stat cache under .kept/cache/, parallelise the cold path, and use
// git's index as a pre-warmed cache when it is there. That is testable and it
// is tested — every path yields an identical digest for one tree, and the test
// desynchronises the caches to prove the content path wins.
//
// The stat cache is derived state under docs/decision/0018's tier rule: losing
// it costs exactly one slow tick and can never corrupt a record. Stat is the
// cache key and content is the definition, which is not a performance
// distinction — two identical checkouts on different machines have different
// mtimes, and a digest over stat would mean their observations never dedup.
package workspace
