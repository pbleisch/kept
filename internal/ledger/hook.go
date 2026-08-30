package ledger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PrePushHook is the hook git runs before a push, and the only hook kept
// writes. It is where the one thing an ordinary `git push` cannot say gets
// said: **git push does not carry a side ref**, so a person ratifies, the push
// exits 0, `git status` is clean, and [Ref] never leaves the machine —
// measured on this repository on 2026-08-25, three commits of it, for a day.
//
// It fires on every push regardless of refspec, which is why it is this hook
// and not a push refspec: a refspec configured into remote.origin.push is
// ignored by `git push origin main` and by `git push --all`, and it carries an
// outer --force into the ledger, which is the 2026-08-22 rewind arriving on the
// outbound side. See docs/decision/0073.
const PrePushHook = "pre-push"

// HookMarker is the line that says a pre-push hook is kept's, and it is the
// whole of how kept tells its own file from somebody else's. A hook is a
// program a person wrote, so nothing here parses one: a file carrying this line
// is kept's and is left alone as already installed, and a file without it is
// somebody's and is left alone as theirs.
const HookMarker = "# Written by kept: it warns when the ledger holds acts origin does not have."

// HookBody is what kept writes. Every ref name and the command it hands over
// are the same constants kept doctor prints, so the line a person copies out of
// a push and the line they copy out of doctor cannot drift.
//
// It warns and does nothing else. It writes nothing, refuses nothing — exit 0
// on every path, so a push is never held hostage to a ledger — and reaches no
// network: the comparison is two local refs, which is [Compare]'s arithmetic
// spelled in shell. It therefore claims only what a local ref supports, *origin
// did not have these as of the last fetch*, exactly as the finding does.
//
// Three shell details are load-bearing. The ref list on stdin is **read**
// rather than discarded, because a push already carrying the ledger has nothing
// to warn about — running the command this hook prints would otherwise print it
// again — and reading it to the end is also what keeps git from seeing a closed
// pipe. The counts come from one `rev-list --left-right --count`, so a forked
// ledger is told apart from an unpublished one and is never handed a push
// command origin would refuse. And a missing [UpstreamRef] speaks rather than
// staying quiet, because that is the state of every fresh adoption — clone,
// `kept init`, ratify, push — and the one this exists for. It says *nobody has
// asked* and hands over the publication rather than a fetch: a fetch that
// brings nothing leaves the state exactly as it was, so a hook that named one
// would repeat itself forever, while the push is unforced and resolves it in
// either direction.
var HookBody = fmt.Sprintf(`#!/bin/sh
%s
# It never refuses, writes, or reaches a network, and kept doctor says the same
# thing. Delete this file to stop it; kept init writes it again.

# The ref list says what this push carries. One already carrying the ledger has
# nothing to warn about, and reading to the end is what git expects.
carrying=
while read -r _ _ remote _; do
	case $remote in
	%s) carrying=yes ;;
	esac
done
[ -n "$carrying" ] && exit 0

# Two local refs and no round trip. An upstream ref nothing has fetched is a
# state of its own: it says nobody has asked, and it hands over the publication
# because a fetch that brings nothing would leave it saying the same thing.
mine=$(git rev-parse -q --verify %s) || exit 0
seen=$(git rev-parse -q --verify %s)
if [ -z "$seen" ]; then
	cat >&2 <<EOF

kept: nothing here has heard from origin about the ledger, and git push does
      not carry a side ref. Publishing it is unforced and safe either way:

          %s

EOF
	exit 0
fi
[ "$mine" = "$seen" ] && exit 0

# One walk for both directions, so a fork is not handed a push that would be
# refused. The positional parameters are done with by here.
apart=$(git rev-list --left-right --count "$mine...$seen") || exit 0
set -- $apart
[ "$1" -gt 0 ] 2>/dev/null || exit 0

commits="$1 commits"
[ "$1" -eq 1 ] && commits="1 commit"

if [ "$2" -gt 0 ] 2>/dev/null; then
	cat >&2 <<EOF

kept: the ledger and what origin last held have both moved, so origin would
      refuse the $commits it does not have.

          kept doctor

EOF
else
	cat >&2 <<EOF

kept: the ledger holds $commits origin did not have as of the last fetch.
      git push does not carry a side ref, so this push leaves them behind:

          %s

EOF
fi
exit 0
`, HookMarker, Ref, Ref, UpstreamRef, PushCommand, PushCommand)

// HookConfig is what a repository does with the pre-push warning, and what
// [ConfigureHook] did about it.
type HookConfig string

const (
	// HookAbsent: git would run a hook from a directory kept writes to, and
	// there is none. Nothing at push time says the ledger is unpublished.
	HookAbsent HookConfig = "absent"
	// HookPresent: kept's hook is already there. [ConfigureHook] wrote nothing.
	HookPresent HookConfig = "present"
	// HookWritten: kept wrote the hook.
	HookWritten HookConfig = "written"
	// HookForeign: there is a pre-push hook and kept did not write it. It is
	// left exactly as it is — git runs one pre-push hook, and appending to
	// somebody's program is editing it.
	HookForeign HookConfig = "foreign"
	// HookBypassed: core.hooksPath points somewhere other than the common
	// hooks directory, so no hook kept could write would ever run. Nothing is
	// written there: that directory belongs to whatever hook runner was
	// configured, a relative value resolves per worktree, and a global one
	// would reach every repository on the machine.
	HookBypassed HookConfig = "bypassed"
	// HookNoRepository: not a Git repository, so there is no hook to write and
	// no push to warn on. A manuscript initialized before it is committed is
	// in this state.
	HookNoRepository HookConfig = "no-repository"
)

// Hook is what this repository does with the pre-push warning: which state it
// is in, where kept's hook is or would be, and — when that is what bypasses
// it — the core.hooksPath git reads instead.
type Hook struct {
	State     HookConfig
	Path      string
	HooksPath string
}

// HookState reports the state and writes nothing. It is two local git calls and
// one stat, and never contacts a remote.
func HookState(repository string) (Hook, error) {
	s := NewRefStore(repository)
	if _, _, err := s.revision(); err != nil {
		if isNotRepository(err) {
			return Hook{State: HookNoRepository}, ErrNotRepository
		}
		return Hook{}, err
	}
	return s.hook()
}

// ConfigureHook writes the pre-push warning where git will run it, and writes
// nothing anywhere else.
//
// It follows kept init's own rule — write what is missing, never rewrite what
// is there — so a second run is a stat and a report. A hook carrying
// [HookMarker] is kept's and is already installed; one without it is somebody's
// program and is reported rather than appended to, replaced or shadowed by a
// file beside it that nothing would run. See docs/decision/0073.
func ConfigureHook(repository string) (Hook, error) {
	s := NewRefStore(repository)
	if _, _, err := s.revision(); err != nil {
		if isNotRepository(err) {
			return Hook{State: HookNoRepository}, ErrNotRepository
		}
		return Hook{}, err
	}
	h, err := s.hook()
	if err != nil || h.State != HookAbsent {
		return h, err
	}
	if err := os.MkdirAll(filepath.Dir(h.Path), 0o755); err != nil {
		return h, err
	}
	if err := os.WriteFile(h.Path, []byte(HookBody), 0o755); err != nil {
		return h, err
	}
	h.State = HookWritten
	return h, nil
}

// hook decides the state from two questions git answers about itself: where it
// would look for a hook, and where the shared hooks directory is.
//
// Asking git where it looks is the whole of the core.hooksPath check. The
// value is documented as absolute or relative to where the command runs, and
// measured it is worse than that: a relative one resolves against the top of
// *each* working tree, so `myhooks` found a hook from the primary checkout and
// found none from a linked worktree of the same repository. Comparing git's own
// answer against the common hooks directory needs none of that resolved.
func (s *RefStore) hook() (Hook, error) {
	looks, err := s.path("--git-path", "hooks")
	if err != nil {
		return Hook{}, err
	}
	common, err := s.path("--git-common-dir")
	if err != nil {
		return Hook{}, err
	}
	// Hooks live in the common directory, so one install covers every linked
	// worktree — the same reason internal/lock puts its lock files there.
	mine := filepath.Join(common, "hooks")
	h := Hook{Path: filepath.Join(mine, PrePushHook)}

	if !sameDir(looks, mine) {
		value, err := s.hooksPath()
		if err != nil {
			return h, err
		}
		h.State, h.HooksPath = HookBypassed, value
		return h, nil
	}

	body, err := os.ReadFile(h.Path)
	switch {
	case os.IsNotExist(err):
		h.State = HookAbsent
	case err != nil:
		return h, err
	case strings.Contains(string(body), HookMarker):
		h.State = HookPresent
	default:
		h.State = HookForeign
	}
	return h, nil
}

// path asks git for a directory and returns it absolute. Git answers relative
// to its own working directory, which is the repository this store names, so
// that is what a relative answer is joined against. Symlinks are resolved for
// the reason internal/lock resolves them: two paths reaching one directory must
// compare equal.
func (s *RefStore) path(args ...string) (string, error) {
	out, err := s.git(nil, nil, append([]string{"rev-parse"}, args...)...)
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(out))
	if value == "" {
		return "", fmt.Errorf("git rev-parse %s returned nothing", strings.Join(args, " "))
	}
	if !filepath.IsAbs(value) {
		value = filepath.Join(s.repository, value)
	}
	return value, nil
}

// sameDir compares two directories that may not exist. A path that resolves is
// compared resolved, so a symlinked git directory does not read as a bypass.
func sameDir(a, b string) bool {
	if a == b {
		return true
	}
	ra, err := filepath.EvalSymlinks(a)
	if err != nil {
		return false
	}
	rb, err := filepath.EvalSymlinks(b)
	if err != nil {
		return false
	}
	return ra == rb
}

// hooksPath reads core.hooksPath. An unset value is exit 1 and is not an error.
func (s *RefStore) hooksPath() (string, error) {
	out, err := s.git(nil, nil, "config", "--get", "core.hooksPath")
	if err != nil && !isExit(err, 1) {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
