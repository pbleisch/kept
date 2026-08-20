package gather

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// argTests is whether `_test.go` files are part of the measurement.
//
// It defaults to false, and that is not the fail-open direction dressed up: a
// boundary promise is about the package that ships, and a test helper writing a
// temporary file is not internal/gather writing to the corpus. Over this
// repository the difference is stark — ten packages call an os writer with
// tests counted and three without — and three is the answer the claim is about.
// A promise meaning "including its tests" writes tests: true, and the arg
// binds. See docs/decision/0049.
const argTests = "tests"

// source is one module's Go, parsed: the packages in scope, keyed by import
// path.
//
// It is one parse serving two gatherers. `imports` asks it for the edges and
// `symbols` asks it for the references, and both questions are answered off an
// AST that is already in hand — which is the whole reason docs/decision/0049
// builds `symbols` rather than the pattern gatherer that was proposed.
type source struct {
	// module is go.mod's module line, which is what makes a directory an
	// import path. It is not a declared field: what a promise reads is package
	// paths, and this is the prefix they already carry.
	module string
	// packages are what was parsed, keyed by import path.
	packages map[string]*sourcePackage
}

// sourcePackage is one directory's worth of Go.
type sourcePackage struct {
	// path is the import path, module plus the directory.
	path string
	// dir is the directory relative to the workspace root, in slash form.
	dir string
	// imports are the direct import paths, in-module and out.
	imports map[string]bool
	// refs are the qualified references this package makes: an import path
	// and a name selected off it.
	refs map[qualified]bool
	// defines are the top-level names this package declares.
	defines map[defined]bool
}

// qualified is one reference through an imported package: os.WriteFile is
// {"os", "WriteFile"}.
type qualified struct{ path, name string }

// defined is one top-level declaration: its name, and what kind of thing it is.
type defined struct{ name, kind string }

// NoModule is a tree with no go.mod at its root.
//
// It is a failure to measure and not a measurement. A directory becomes an
// import path by having a module line in front of it, so without one there is
// no package graph to be right or wrong about — and reporting an empty one
// would say a boundary holds in a tree where the question was never put.
type NoModule struct{ Root string }

func (e *NoModule) Error() string {
	return fmt.Sprintf("%s has no go.mod, so nothing says what these directories' import paths are", e.Root)
}

// Unparsable is a Go file that could not be read as Go.
//
// It is a failure to measure, and that is the whole difference between this and
// a tree that does not compile. A tree with a type error, a missing symbol or
// an import cycle parses perfectly, and answering there is the reason
// docs/decision/0049 parses rather than shelling out to the toolchain. A file
// with a syntax error yields a **partial** AST, and reading the imports out of
// one would under-approximate the edge set silently — which is the direction
// that reports a boundary kept when it was crossed.
type Unparsable struct {
	Path string
	Err  error
}

func (e *Unparsable) Error() string {
	return fmt.Sprintf("%s could not be read as Go, so what it imports is unknown rather than nothing", e.Path)
}

func (e *Unparsable) Unwrap() error { return e.Err }

// readSource parses every Go file the scope matched.
//
// Build constraints are not evaluated, so `program_unix.go` and
// `program_other.go` both contribute. That **over**-approximates, and for a
// boundary promise it is the fail-closed direction: a boundary that is not
// crossed on any platform is not crossed. See docs/decision/0049.
func readSource(req Request, withTests bool) (*source, error) {
	module, err := moduleOf(req.Root)
	if err != nil {
		return nil, err
	}

	src := &source{module: module, packages: map[string]*sourcePackage{}}
	fset := token.NewFileSet()

	files := append([]string(nil), req.Paths...)
	sort.Strings(files)
	for _, rel := range files {
		if !strings.HasSuffix(rel, ".go") {
			continue
		}
		if !withTests && strings.HasSuffix(rel, "_test.go") {
			continue
		}
		full := filepath.Join(req.Root, filepath.FromSlash(rel))
		file, err := parser.ParseFile(fset, full, nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, &Unparsable{Path: rel, Err: err}
		}
		src.at(path.Dir(rel)).read(file)
	}
	return src, nil
}

// at returns the package a directory holds, creating it on first sight.
func (s *source) at(dir string) *sourcePackage {
	if dir == "." {
		dir = ""
	}
	full := s.module
	if dir != "" {
		full = s.module + "/" + dir
	}
	p, ok := s.packages[full]
	if !ok {
		if dir == "" {
			dir = "."
		}
		p = &sourcePackage{
			path:    full,
			dir:     dir,
			imports: map[string]bool{},
			refs:    map[qualified]bool{},
			defines: map[defined]bool{},
		}
		s.packages[full] = p
	}
	return p
}

// read folds one file into its package.
func (p *sourcePackage) read(file *ast.File) {
	// name -> import path, for this file. A selector is read as a package
	// reference when its left side names one of these, and no scope analysis
	// is done: a local variable shadowing an import name mints a reference
	// nobody wrote. Measured over this repository that is six references in
	// seven hundred, and it is the same fail-closed direction as the build
	// tags — the failure it produces is loud, and the failure it refuses is
	// silent. See docs/decision/0049.
	named := map[string]string{}
	for _, spec := range file.Imports {
		target, err := strconv.Unquote(spec.Path.Value)
		if err != nil || target == "" {
			continue
		}
		p.imports[target] = true
		name := target[strings.LastIndex(target, "/")+1:]
		if spec.Name != nil {
			name = spec.Name.Name
		}
		// A blank import binds no name and a dot import binds every name, so
		// neither one can be the left side of a resolvable selector.
		if name == "_" || name == "." {
			continue
		}
		named[name] = target
	}

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			kind := "func"
			if d.Recv != nil {
				kind = "method"
			}
			p.defines[defined{name: d.Name.Name, kind: kind}] = true
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					p.defines[defined{name: s.Name.Name, kind: "type"}] = true
				case *ast.ValueSpec:
					kind := "var"
					if d.Tok == token.CONST {
						kind = "const"
					}
					for _, name := range s.Names {
						p.defines[defined{name: name.Name, kind: kind}] = true
					}
				}
			}
		}
	}

	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		id, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		if target, isImport := named[id.Name]; isImport {
			p.refs[qualified{path: target, name: sel.Sel.Name}] = true
		}
		return true
	})
}

// paths are the packages that were parsed, sorted.
func (s *source) paths() []string {
	out := make([]string, 0, len(s.packages))
	for full := range s.packages {
		out = append(out, full)
	}
	sort.Strings(out)
	return out
}

// inModule reports whether an import path is a package of this module.
func (s *source) inModule(target string) bool {
	return target == s.module || strings.HasPrefix(target, s.module+"/")
}

// moduleOf reads go.mod's module line.
//
// It is read straight off the root rather than out of the scope, because a
// scope names Go source and this is what makes a directory an import path at
// all. A module rename rewrites every in-module import with it, so the scope
// moves and the promise is due; nothing here depends on that file being
// matched.
func moduleOf(root string) (string, error) {
	src, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", &NoModule{Root: root}
	}
	for line := range strings.SplitSeq(string(src), "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module")
		if !ok {
			continue
		}
		name := strings.TrimSpace(rest)
		if name == "" || name == rest {
			// "modulefoo" is not a module line, and neither is a bare
			// "module".
			continue
		}
		if unquoted, err := strconv.Unquote(name); err == nil {
			name = unquoted
		}
		if name != "" {
			return name, nil
		}
	}
	return "", &NoModule{Root: root}
}

// wantTests reads the tests arg.
func wantTests(args map[string]any) (bool, error) {
	switch t := args[argTests].(type) {
	case nil:
		return false, nil
	case bool:
		return t, nil
	}
	return false, fmt.Errorf("%s: is %v, and it says whether _test.go files are part of the measurement — true or false",
		argTests, args[argTests])
}
