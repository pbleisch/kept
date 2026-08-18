package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/schema"
)

// The corpus root a workspace gets when nothing says otherwise. A repository
// keeps its corpus in a directory beside its code; a manuscript, where the
// corpus is the content, is --corpus "." and the root is the workspace. See
// docs/layout.md and docs/decision/0028.
const defaultCorpus = "docs"

// attributes is what decision 0022 asks a workspace to carry, and it is
// hygiene rather than the guarantee. The binding hash normalizes line endings,
// unicode form and trailing whitespace inside the hash function, so a checkout
// with core.autocrlf=true keeps every ratification whether or not this file was
// present, honoured or reachable — increment 2's CRLF test runs deliberately
// without one. What this stops is the bytes churning in every diff.
//
// The merge rule is decision 0011's: append-only streams merge by union, which
// is safe only because every derived read folds by event time rather than by
// file order. Those paths move to refs/kept/ledger when boxes arrive (0016) and
// the rule follows them there.
const attributes = `# Text is normalized on the way in, and normalized again inside the binding
# hash — see decision 0022. This file is hygiene; the hash is the guarantee.
* text=auto eol=lf

# The ledger is append-only, so concurrent appends from two checkouts should
# keep both sides. Safe only because every derived read folds by event time,
# not by file order. See decisions 0011 and 0016.
.kept/ledger/**/*.jsonl merge=union
`

// textRule is the attribute 0022 names. It is looked for by the pattern and the
// attribute rather than by the whole line, because a workspace that spells it
// `* text=auto` or sets it per extension has already answered the question and
// does not need telling.
const textRule = "* text=auto eol=lf"

func config(root string) string {
	return fmt.Sprintf(`# Where the corpus lives, relative to this directory. Leave it out and the
# corpus root is this directory, which is what a manuscript wants: there the
# corpus is the content. See docs/layout.md and decision 0028.
corpus: %s

# Retention, deadlines and confinement are configured here too. Each has a
# decision behind it — 0020, 0026, 0017 — and each arrives with the machinery
# that reads it, rather than sitting here as a key nothing consults.
`, root)
}

// initCmd makes a directory a workspace: the config that says where the corpus
// is, the directory it names, and the .gitattributes decision 0022 asks for.
//
// It writes what is missing and never rewrites what is there. A config is the
// operator's — it decides what is enforced — and a text attribute is
// repository-wide, so appending one to a .gitattributes somebody else wrote
// would change how git stores every file in their tree. Both are reported and
// left alone, with the line to add printed rather than added.
//
// Two things decisions elsewhere ask of this verb are not here yet, because
// what they need does not exist. 0027 has init run `kept agent install`, and
// there is no generator to run. 0016 has it add the fetch refspec for
// refs/kept/ledger, and the ledger is still plain files on this branch until
// boxes arrive.
func initCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("corpus", defaultCorpus, "where the corpus lives, relative to this directory")
	rest, err := parse(fs, args)
	if err != nil {
		return exitUsage
	}
	if len(rest) > 0 {
		fmt.Fprintf(stderr, "kept: init takes no arguments and works on the current directory; --corpus says where the corpus lives inside it\n")
		return exitUsage
	}

	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return exitUsage
	}
	declared, initialized, code := claim(wd, stderr)
	if code != exitOK {
		return code
	}

	// A config that is already there decides where the corpus is. Creating the
	// directory --corpus named instead would leave an empty one the workspace
	// does not read, next to the one it does.
	dir := path.Clean(filepath.ToSlash(*root))
	if initialized {
		if given(fs, "corpus") && declared != dir {
			fmt.Fprintf(stdout, "%s/%s already says the corpus is %s, so --corpus %s was not used.\n\n",
				corpus.ConfigDir, corpus.ConfigFile, declared, dir)
		}
		dir = declared
	}

	var report []string
	note := func(what, path string) { report = append(report, fmt.Sprintf("  %-13s %s", what, path)) }

	// The corpus root, unless it is the workspace itself — a manuscript's is,
	// and a directory that was always going to be there is not news.
	if dir != "." {
		written, err := create(filepath.Join(wd, filepath.FromSlash(dir)))
		if err != nil {
			fmt.Fprintf(stderr, "kept: %s\n", err)
			return exitUsage
		}
		note(existing(written), dir+"/")
	}

	for _, f := range []struct{ path, content string }{
		{filepath.Join(corpus.ConfigDir, corpus.ConfigFile), config(dir)},
		{".gitattributes", attributes},
	} {
		written, err := put(filepath.Join(wd, f.path), f.content)
		if err != nil {
			fmt.Fprintf(stderr, "kept: %s\n", err)
			return exitUsage
		}
		note(existing(written), f.path)
	}

	fmt.Fprintf(stdout, "%s\n", strings.Join(report, "\n"))
	advise(stdout, wd, dir)
	return exitOK
}

// given reports whether a flag was typed, rather than defaulted into.
func given(fs *flag.FlagSet, name string) bool {
	var found bool
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

func existing(written bool) string {
	if written {
		return "wrote"
	}
	return "already there"
}

// claim says whether this directory is already a workspace and, if it is, what
// its config says the corpus root is.
//
// A workspace inside a workspace is refused. Which config a command finds is
// decided by the nearest one above it, so a nested one silently changes what
// every command in the subtree resolves to — a deliberate act, not something to
// fall into from the directory you happened to be standing in.
func claim(wd string, stderr io.Writer) (root string, initialized bool, code int) {
	resolved, base, err := corpus.FindRoot(wd)
	switch {
	case errors.Is(err, corpus.ErrNoConfig):
		return "", false, exitOK
	case err != nil:
		// A config that will not parse. It is still somebody's, so it is
		// reported rather than replaced.
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return "", false, exitUsage
	case base != wd:
		fmt.Fprintf(stderr, "kept: %s is already a workspace, and this directory is inside it.\n"+
			"        A second one would change which config every command here resolves to.\n"+
			"        Run init there, or somewhere outside it.\n", base)
		return "", false, exitUsage
	}

	// Already a workspace here. Everything below still runs, so a workspace
	// missing a file it should have gains it beside the ones it has.
	rel, err := filepath.Rel(base, resolved)
	if err != nil {
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return "", false, exitUsage
	}
	return path.Clean(filepath.ToSlash(rel)), true, exitOK
}

// create makes a directory, and says whether it had to.
func create(path string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil
	}
	return true, os.MkdirAll(path, 0o755)
}

// put writes a file that is not there, and says whether it wrote. A file that
// exists is left exactly as it is: nothing here rewrites what a person authored.
func put(path, content string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	return true, os.WriteFile(path, []byte(content), 0o644)
}

// advise says what is left to do. The .gitattributes case is the one worth
// printing at length: a file that was already there is not touched, so the rule
// 0022 asks for has to be handed over rather than added.
func advise(w io.Writer, wd, root string) {
	src, err := os.ReadFile(filepath.Join(wd, ".gitattributes"))
	if err == nil && !strings.Contains(string(src), textRule) {
		fmt.Fprintf(w, "\n.gitattributes is yours and was left as it is. It carries no rule for\n"+
			"text, and decision 0022 asks for one:\n\n    %s\n\n"+
			"That is hygiene rather than the guarantee. The binding hash normalizes\n"+
			"line endings itself, so a CRLF checkout keeps its ratifications with or\n"+
			"without the line; without it the bytes churn in every diff.\n", textRule)
	}

	dir := path.Clean(filepath.ToSlash(root))
	where := dir + "/"
	if dir == "." {
		where = "this directory"
	}
	fmt.Fprintf(w, "\nThe corpus root is %s, and what goes in it is yours to write:\n\n"+
		"  %s\n      declares what a kind must say, and which of its fields bind\n"+
		"  %s\n      is an artifact of that kind\n\n"+
		"kept list reads them back, and says everything that is wrong with them.\n",
		where,
		path.Join(dir, schema.Dir, "{type}.yaml"),
		path.Join(dir, "{type}", "{slug}.md"))
}
