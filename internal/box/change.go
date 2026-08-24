package box

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/pbleisch/kept/internal/workspace"
)

// Change is one repair's work as a person reviews it: every path it touched,
// and the bounded diff over them.
//
// Both are read from one snapshot of the box. Two methods would take two, and a
// box that moved between them yields a path list and a diff that disagree —
// silently, and in the direction where a path is missing from the list the
// review is made against.
type Change struct {
	// Paths are workspace-relative and slash-separated, as git reports them,
	// sorted.
	//
	// They are read from git rather than parsed out of the diff text. A
	// delete's diff header names /dev/null, so reading `+++ b/` drops exactly
	// the change a scope review most needs to see — a repair that removed a
	// file it was never authorized to touch. A diff past its bound has no text
	// to read at all.
	Paths []string
	// Diff is the bounded binary diff from the post-overlay snapshot to the
	// box's current tree.
	Diff []byte
}

// Change returns what the repair did to this box: the paths and the diff, both
// over one snapshot.
func (b *Box) Change(ctx context.Context) (Change, error) {
	if ctx == nil {
		return Change{}, errors.New("repair box context is nil")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return Change{}, ErrClosed
	}

	current, err := b.snapshot(ctx)
	if err != nil {
		return Change{}, err
	}
	names, err := boundedGit(ctx, b.path, b.deadline, b.maxDiff,
		"diff", "--name-only", "-z", "--no-ext-diff", "--no-textconv", b.baseTree, current)
	if err != nil {
		return Change{}, err
	}
	diff, err := boundedGit(ctx, b.path, b.deadline, b.maxDiff,
		"diff", "--binary", "--full-index", "--no-ext-diff", "--no-textconv", b.baseTree, current)
	if err != nil {
		return Change{}, err
	}
	return Change{Paths: changedPaths(names), Diff: diff}, nil
}

// changedPaths splits git's NUL-separated name list. -z rather than the default
// because git quotes and escapes a path with a special character otherwise, and
// an unquoted reading of that would compare the wrong string against a scope.
func changedPaths(names []byte) []string {
	var paths []string
	for _, name := range strings.Split(string(names), "\x00") {
		if name != "" {
			paths = append(paths, name)
		}
	}
	sort.Strings(paths)
	return paths
}

// Review is a change measured against the repair scope its promise bound: what
// the repair was authorized to touch, and what it touched anyway.
//
// It is one computation because two would be two answers. The report a person
// reads beside a diff and the check a land refuses on ask the same question,
// and a second implementation of it fails in the direction where the report is
// clean and the gate is not.
type Review struct {
	// Scope is the promise's bound, carried so a report can say what the
	// repair was measured against rather than only what it did.
	Scope   []string
	InScope []string
	Outside []string
}

// Against measures this change against the repair scope a promise bound.
//
// It fails closed at every edge. A scope that names nothing covers nothing, so
// every changed path is outside it: a promise with no bound authorizes no
// repair rather than authorizing all of one. See
// docs/policy/out-of-scope-paths-are-never-landed.md.
func (c Change) Against(scope []string) Review {
	review := Review{Scope: append([]string(nil), scope...)}
	for _, path := range c.Paths {
		if len(scope) > 0 && workspace.Match(scope, path) {
			review.InScope = append(review.InScope, path)
			continue
		}
		review.Outside = append(review.Outside, path)
	}
	return review
}

// Bounded reports whether every path the repair touched lies inside the scope.
func (r Review) Bounded() bool { return len(r.Outside) == 0 }

// String is the report a person reads beside the diff.
func (r Review) String() string {
	touched := len(r.InScope) + len(r.Outside)
	var out strings.Builder
	switch {
	case touched == 0:
		return "the repair changed nothing.\n"
	case r.Bounded() && touched == 1:
		out.WriteString("1 path changed, inside the repair scope this promise bound.\n")
	case r.Bounded():
		fmt.Fprintf(&out, "%s changed, every one inside the repair scope this promise bound.\n", plural(touched, "path"))
	default:
		fmt.Fprintf(&out, "%s changed, %d outside the repair scope this promise bound.\n", plural(touched, "path"), len(r.Outside))
	}
	section(&out, "scope", r.Scope)
	section(&out, "in scope", r.InScope)
	section(&out, "outside the scope, and nothing that reaches it may land", r.Outside)
	return out.String()
}

func section(out *strings.Builder, title string, lines []string) {
	if len(lines) == 0 {
		return
	}
	fmt.Fprintf(out, "\n%s\n", title)
	for _, line := range lines {
		fmt.Fprintf(out, "  %s\n", line)
	}
}

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
