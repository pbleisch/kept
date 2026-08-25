package land

import (
	"fmt"
	"strings"
)

// Target is where a land is going: one branch on one remote.
//
// It is a value carried through the gate, the lock, the in-lock check and the
// act rather than a constant read at each of those sites. Only origin/main is
// constructed today — [Origin] is the one place that names it — but a landing
// target that is spelled out four times is one that cannot be changed in fewer
// than four places, and a repository whose work lands on a branch is a shape
// kept is expected to grow into.
//
// This is deliberately not multi-target support: nothing selects a target, no
// flag sets one, and the only constructor returns origin/main.
type Target struct {
	Remote string
	Branch string
}

// Origin is the one target this build lands on.
func Origin() Target { return Target{Remote: "origin", Branch: "main"} }

// Valid refuses a target that could not be handed to git as it stands. A ref
// name is checked rather than trusted because it becomes an argument, and a
// value that starts with a dash is a flag wherever it is put.
func (t Target) Valid() error {
	for what, value := range map[string]string{"remote": t.Remote, "branch": t.Branch} {
		switch {
		case value == "":
			return fmt.Errorf("a landing target names no %s", what)
		case strings.HasPrefix(value, "-"):
			return fmt.Errorf("a landing target's %s %q would be read as a flag", what, value)
		case strings.ContainsAny(value, " \t\n:?*[\\^~\x00"):
			return fmt.Errorf("a landing target's %s %q is not a name git accepts", what, value)
		case strings.Contains(value, ".."):
			return fmt.Errorf("a landing target's %s %q is not a name git accepts", what, value)
		}
	}
	if strings.Contains(t.Remote, "/") {
		return fmt.Errorf("a landing target's remote %q is not a remote name", t.Remote)
	}
	return nil
}

// Ref is the branch as the remote holds it.
func (t Target) Ref() string { return "refs/heads/" + t.Branch }

// Tracking is the local copy of that branch — the ref a fetch writes and the
// in-lock check reads. It is never the local branch of the same name: a local
// lock excludes local runs and nothing else, so a check against refs/heads/main
// would be answering about this machine rather than about the remote. See
// docs/decision/0015.
func (t Target) Tracking() string { return "refs/remotes/" + t.Remote + "/" + t.Branch }

// Refspec is what a land fetches, stated in full rather than left to whatever
// remote.<name>.fetch happens to say.
//
// Forced, and for the reason docs/decision/0062 gives: a tracking ref is a copy
// of what the remote holds, and git forces every one of them. The destination
// is a tracking ref and never a durable record, which is the distinction that
// decision was written about.
func (t Target) Refspec() string { return "+" + t.Ref() + ":" + t.Tracking() }

// String is the target as a person names it.
func (t Target) String() string { return t.Remote + "/" + t.Branch }
