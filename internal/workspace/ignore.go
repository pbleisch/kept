package workspace

import (
	"strings"
)

// ignoreFile is the rules from one .gitignore, and the directory they govern.
//
// A .gitignore applies to its own directory and below, nearest-first, with the
// last matching pattern in a file winning and ! re-including. Those are the
// format's own rules, and kept honours them as a file format: no git is
// invoked, nothing requires a repository, and a workspace that is not one and
// happens to carry a .gitignore gets the same answer as one that is.
//
// Two things git consults and kept deliberately does not: .git/info/exclude and
// a global core.excludesFile. Both are per-checkout and untracked, so honouring
// them would make one tree produce two digests on two machines — the identity
// split docs/decision/0033 exists to prevent.
type ignoreFile struct {
	// dir is the workspace-relative slash directory holding the file, "" at
	// the root.
	dir   string
	rules []ignoreRule
}

type ignoreRule struct {
	pattern string
	negate  bool
	dirOnly bool
	// anchored is set when the pattern has a separator anywhere but its end,
	// which the format reads as relative to the file's own directory. An
	// unanchored pattern may match at any depth below it.
	anchored bool
}

// parseIgnore reads one .gitignore's text. Malformed is not a category here:
// every line is either a comment, a blank, or a pattern, which is what makes
// this a format rather than a program.
func parseIgnore(dir, src string) *ignoreFile {
	f := &ignoreFile{dir: dir}
	for _, line := range strings.Split(src, "\n") {
		line = strings.TrimRight(line, "\r")
		line = trimUnescapedTrailingSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		var r ignoreRule
		if strings.HasPrefix(line, "!") {
			r.negate = true
			line = line[1:]
		} else if strings.HasPrefix(line, `\`) && len(line) > 1 && (line[1] == '#' || line[1] == '!') {
			line = line[1:]
		}
		if strings.HasSuffix(line, "/") {
			r.dirOnly = true
			line = strings.TrimSuffix(line, "/")
		}
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "/") {
			r.anchored = true
			line = strings.TrimPrefix(line, "/")
		} else if strings.Contains(line, "/") {
			r.anchored = true
		}
		r.pattern = line
		f.rules = append(f.rules, r)
	}
	return f
}

// trimUnescapedTrailingSpace drops trailing spaces, which the format says are
// insignificant unless a backslash makes one significant.
func trimUnescapedTrailingSpace(line string) string {
	end := len(line)
	for end > 0 && line[end-1] == ' ' {
		escapes := 0
		for i := end - 2; i >= 0 && line[i] == '\\'; i-- {
			escapes++
		}
		if escapes%2 == 1 {
			break
		}
		end--
	}
	return line[:end]
}

// match reports what this file says about a path: whether any rule matched, and
// whether the last one to match ignores it.
func (f *ignoreFile) match(path string, isDir bool) (matched, ignored bool) {
	rel, ok := under(f.dir, path)
	if !ok {
		return false, false
	}
	for _, r := range f.rules {
		if r.dirOnly && !isDir {
			continue
		}
		if !r.matches(rel) {
			continue
		}
		matched, ignored = true, !r.negate
	}
	return matched, ignored
}

func (r ignoreRule) matches(rel string) bool {
	pattern := r.pattern
	if !r.anchored {
		// The format reads a pattern with no separator as matching at any
		// depth below the file's directory, which is exactly a ** prefix.
		pattern = "**/" + pattern
	}
	return matchIgnoreSegments(strings.Split(pattern, "/"), strings.Split(rel, "/"))
}

// under returns path relative to dir, and whether it is under it at all.
func under(dir, path string) (string, bool) {
	if dir == "" {
		return path, true
	}
	if !strings.HasPrefix(path, dir+"/") {
		return "", false
	}
	return path[len(dir)+1:], true
}

// matchIgnoreSegments is the scope glob's walk over ** with the format's own
// segment matcher underneath.
//
// It is a second matcher on purpose. [Match] is kept's scope language, where
// two metacharacters are the whole of it and a third would be permanent
// (docs/decision/0033); this one is somebody else's format, where ?, a
// character class and a backslash escape are part of what a .gitignore already
// means and reading one without them would quietly widen what a workspace
// thought it had excluded.
func matchIgnoreSegments(pattern, path []string) bool {
	for len(pattern) > 0 {
		if pattern[0] == "**" {
			rest := pattern[1:]
			if len(rest) == 0 {
				return true
			}
			for i := 0; i <= len(path); i++ {
				if matchIgnoreSegments(rest, path[i:]) {
					return true
				}
			}
			return false
		}
		if len(path) == 0 {
			return false
		}
		if !matchIgnoreSegment(pattern[0], path[0]) {
			return false
		}
		pattern, path = pattern[1:], path[1:]
	}
	// A pattern that matched a prefix of the path matched a directory of it,
	// and ignoring a directory ignores what is under it.
	return true
}

// matchIgnoreSegment is fnmatch over one segment: * for any run, ? for one
// character, [...] for a class, and a backslash for a literal.
func matchIgnoreSegment(pattern, name string) bool {
	var p, n, star, mark int
	star = -1
	for n < len(name) {
		if p < len(pattern) {
			switch pattern[p] {
			case '*':
				star, mark = p, n
				p++
				continue
			case '?':
				p++
				n++
				continue
			case '[':
				if width, ok := matchClass(pattern[p:], name[n]); ok {
					p += width
					n++
					continue
				}
			case '\\':
				if p+1 < len(pattern) && pattern[p+1] == name[n] {
					p += 2
					n++
					continue
				}
			default:
				if pattern[p] == name[n] {
					p++
					n++
					continue
				}
			}
		}
		if star < 0 {
			return false
		}
		p = star + 1
		mark++
		n = mark
	}
	for p < len(pattern) && pattern[p] == '*' {
		p++
	}
	return p == len(pattern)
}

// matchClass reads one [...] against one byte, returning how much of the
// pattern it consumed. An unterminated class is a literal bracket, which is
// what the format says.
func matchClass(pattern string, c byte) (width int, matched bool) {
	i := 1
	negate := false
	if i < len(pattern) && (pattern[i] == '!' || pattern[i] == '^') {
		negate = true
		i++
	}
	hit := false
	for first := true; i < len(pattern); first = false {
		if pattern[i] == ']' && !first {
			if negate {
				hit = !hit
			}
			return i + 1, hit
		}
		lo := pattern[i]
		i++
		if i+1 < len(pattern) && pattern[i] == '-' && pattern[i+1] != ']' {
			hi := pattern[i+1]
			i += 2
			if c >= lo && c <= hi {
				hit = true
			}
			continue
		}
		if c == lo {
			hit = true
		}
	}
	return 0, false
}

// ignoreStack is the files governing the directory being walked, root first.
type ignoreStack []*ignoreFile

// ignored answers for one path, nearest file first: the deepest .gitignore that
// says anything about a path decides, and within one file the last matching
// pattern wins.
func (s ignoreStack) ignored(path string, isDir bool) bool {
	for i := len(s) - 1; i >= 0; i-- {
		if matched, ignored := s[i].match(path, isDir); matched {
			return ignored
		}
	}
	return false
}
