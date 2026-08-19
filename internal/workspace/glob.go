package workspace

import "strings"

// Match reports whether a workspace-relative slash path is matched by one of a
// scope's globs.
//
// kept says what a glob means, because nothing else does. Go's standard library
// cannot express one — filepath.Match("internal/**", "internal/corpus/corpus.go")
// is false — and git has three answers depending on which command is asked:
// a default pathspec lets * cross a slash, :(glob) magic does not, and ls-tree
// rejects the magic outright. "Use git's globs" does not name a behaviour. See
// docs/decision/0033.
//
// Two metacharacters and no more:
//
//   - * matches within one path segment.
//   - ** matches any number of segments, including none.
//
// A pattern ending in / is the directory and everything under it: internal/ is
// internal/**. That is the form a scope is ordinarily written in, and reading
// it as a literal path ending in a slash would match nothing at all.
//
// Everything else is literal, ? and [ included. A scope that meant one of those
// as a wildcard matches nothing, and matching nothing is a distinct outcome
// that reads as always due and is surfaced by kept doctor — which is the loud
// direction, and the reason this can stay two metacharacters until a promise
// needs a third.
func Match(patterns []string, path string) bool {
	for _, p := range patterns {
		if matchOne(p, path) {
			return true
		}
	}
	return false
}

func matchOne(pattern, path string) bool {
	pattern = strings.TrimPrefix(pattern, "./")
	if pattern == "" {
		return false
	}
	if strings.HasSuffix(pattern, "/") {
		pattern += "**"
	}
	return matchSegments(strings.Split(pattern, "/"), strings.Split(path, "/"))
}

// matchSegments walks the two segment lists together. ** is the only construct
// that consumes an unknown number of segments, so it is the only place this
// branches.
func matchSegments(pattern, path []string) bool {
	for len(pattern) > 0 {
		if pattern[0] == "**" {
			rest := pattern[1:]
			if len(rest) == 0 {
				return true
			}
			for i := 0; i <= len(path); i++ {
				if matchSegments(rest, path[i:]) {
					return true
				}
			}
			return false
		}
		if len(path) == 0 {
			return false
		}
		if !matchSegment(pattern[0], path[0]) {
			return false
		}
		pattern, path = pattern[1:], path[1:]
	}
	return len(path) == 0
}

// matchSegment matches one segment, where * stands for any run of characters
// that is not a separator. Written as a two-pointer scan with one backtrack
// point rather than a recursion, so a pattern of many stars cannot cost
// exponential time on a long name.
func matchSegment(pattern, name string) bool {
	var p, n, star, mark int
	star = -1
	for n < len(name) {
		switch {
		case p < len(pattern) && pattern[p] == '*':
			star, mark = p, n
			p++
		case p < len(pattern) && pattern[p] == name[n]:
			p++
			n++
		case star >= 0:
			p = star + 1
			mark++
			n = mark
		default:
			return false
		}
	}
	for p < len(pattern) && pattern[p] == '*' {
		p++
	}
	return p == len(pattern)
}
