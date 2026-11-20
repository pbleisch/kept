package assess

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/pbleisch/kept/internal/gather"
	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/predicate"
	"github.com/pbleisch/kept/internal/thread"
)

// TestSource is one Go test file the anchor's diff touched, as it stood on
// either side of the change.
//
// It is read for one question: whether the repair changed a test its promise
// names by name. A promise whose predicate reads `"TestX" in obs.tests.cases`
// has made TestX part of its instrument, and that test sits in a file inside the
// repair's own bound whenever it needs the package's internals — a glob cannot
// leave one file out. So the file is read rather than its path matched, and
// what is compared is code rather than bytes. See docs/decision/0091.
type TestSource struct {
	// Base is the file at the anchor's parent and Anchor the file at the anchor,
	// each nil where the file did not exist on that side. A rename's base is
	// read from the path it came from.
	Base, Anchor []byte
	// Read says both sides were asked for and answered, whether present or not.
	Read bool
}

// gathersTests reports whether any of a promise's instruments is go/tests, the
// one whose cases a predicate can name.
func gathersTests(specs map[string]model.GatherSpec) bool {
	for _, spec := range specs {
		if spec.Gatherer == gather.Tests {
			return true
		}
	}
	return false
}

// testSources reads every touched Go test file on both sides of the anchor.
func testSources(ctx context.Context, repository string, anchor thread.Anchor, touched []Touched, deadline time.Duration) map[string]TestSource {
	if deadline <= 0 {
		deadline = defaultDeadline
	}
	call, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()
	out := map[string]TestSource{}
	for _, t := range touched {
		basePath := t.Path
		if t.From != "" {
			basePath = t.From
		}
		if !strings.HasSuffix(t.Path, "_test.go") && !strings.HasSuffix(basePath, "_test.go") {
			continue
		}
		base, baseErr := blobAt(call, repository, anchor.Parent, basePath)
		after, afterErr := blobAt(call, repository, anchor.Commit, t.Path)
		out[t.Path] = TestSource{Base: base, Anchor: after, Read: baseErr == nil && afterErr == nil}
	}
	return out
}

// blobAt is a file's bytes at a commit, nil and no error where the commit holds
// no such path.
func blobAt(ctx context.Context, repository, commit, path string) ([]byte, error) {
	if commit == "" {
		return nil, fmt.Errorf("no commit to read %s at", path)
	}
	verify := exec.CommandContext(ctx, "git", "-C", repository, "rev-parse", "--verify", "--quiet", "--end-of-options", commit+":"+path)
	verify.Env = withoutWorktreeEnv()
	oid, err := verify.Output()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		if exit, isExit := err.(*exec.ExitError); isExit && exit.ExitCode() == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("find %s at %s: %w", path, commit, err)
	}
	read := exec.CommandContext(ctx, "git", "-C", repository, "cat-file", "blob", strings.TrimSpace(string(oid)))
	read.Env = withoutWorktreeEnv()
	src, err := read.Output()
	if err != nil {
		return nil, fmt.Errorf("read %s at %s: %w", path, commit, err)
	}
	return src, nil
}

// namedTestChanges is every touched test file that declared, at the base, a
// test the predicate names, with what the repair changed in it — and the paths
// that could not be read to say.
//
// What counts as changed is a top-level declaration the file held at the base
// that the anchor no longer holds in the same form, comments aside: a named
// test's body, a helper it calls, a constant sizing its fixture. A declaration
// the repair added is not a change to the instrument, because nothing measured
// with it before; and a comment is not code, so correcting prose a repair made
// stale does not read as moving the standard. A file the repair left
// unparseable has changed whatever it held.
func namedTestChanges(holdWhen string, specs map[string]model.GatherSpec, sources map[string]TestSource) (lines, unread []string) {
	if !gathersTests(specs) || len(sources) == 0 {
		return nil, nil
	}
	expr, err := predicate.Parse(holdWhen, predicate.HoldWhen)
	if err != nil {
		return nil, nil
	}
	named := map[string]bool{}
	for _, literal := range expr.Strings() {
		named[literal] = true
	}
	if len(named) == 0 {
		return nil, nil
	}
	paths := make([]string, 0, len(sources))
	for path := range sources {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		source := sources[path]
		if !source.Read {
			unread = append(unread, path)
			continue
		}
		if source.Base == nil {
			continue
		}
		base, ok := declarations(source.Base)
		if !ok {
			continue
		}
		var tests []string
		for _, name := range base.tests {
			if named[name] {
				tests = append(tests, name)
			}
		}
		if len(tests) == 0 {
			continue
		}
		var changed []string
		switch after, parsed := declarations(source.Anchor); {
		case source.Anchor == nil:
			changed = []string{"deleted the file"}
		case !parsed:
			changed = []string{"left it unparseable"}
		default:
			for _, key := range base.order {
				if after.code[key] != base.code[key] {
					changed = append(changed, key)
				}
			}
		}
		if len(changed) == 0 {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s: declares %s, which the predicate names, and the repair changed %s",
			path, strings.Join(tests, ", "), strings.Join(changed, ", ")))
	}
	return lines, unread
}

// declared is one Go file's top-level declarations as code, comments removed.
type declared struct {
	// order is every key in source order, code is each key's printed form, and
	// tests is the Test functions the file declares.
	order []string
	code  map[string]string
	tests []string
}

// declarations reads one Go file's top-level declarations, keyed by what each
// declares: a function by its receiver and name, and a constant, variable or
// type by the names its spec binds. Imports are left out — a new helper needs
// one, and an import is not part of what a test asserts.
func declarations(src []byte) (declared, bool) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", src, parser.SkipObjectResolution)
	if err != nil {
		return declared{}, false
	}
	out := declared{code: map[string]string{}}
	put := func(key string, node any) {
		var buf bytes.Buffer
		if err := printer.Fprint(&buf, fset, node); err != nil {
			return
		}
		if _, seen := out.code[key]; !seen {
			out.order = append(out.order, key)
		}
		out.code[key] += code(buf.String())
	}
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			key := d.Name.Name
			if d.Recv != nil && len(d.Recv.List) > 0 {
				var recv bytes.Buffer
				_ = printer.Fprint(&recv, fset, d.Recv.List[0].Type)
				key = "(" + recv.String() + ")." + key
			} else if strings.HasPrefix(d.Name.Name, "Test") {
				out.tests = append(out.tests, d.Name.Name)
			}
			d.Doc = nil
			put(key, d)
		case *ast.GenDecl:
			if d.Tok == token.IMPORT {
				continue
			}
			for _, spec := range d.Specs {
				var names []string
				switch s := spec.(type) {
				case *ast.ValueSpec:
					s.Doc, s.Comment = nil, nil
					for _, name := range s.Names {
						names = append(names, name.Name)
					}
				case *ast.TypeSpec:
					s.Doc, s.Comment = nil, nil
					names = append(names, s.Name.Name)
				}
				if len(names) > 0 {
					put(strings.Join(names, ", "), spec)
				}
			}
		}
	}
	return out, true
}

// code is printed Go with its layout taken out: every line trimmed and every
// blank one dropped. The printer keeps a blank line where a removed comment
// left a gap between two statements, and a gap is not code.
func code(printed string) string {
	var out []string
	for line := range strings.SplitSeq(printed, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n") + "\n"
}
