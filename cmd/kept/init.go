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
	"github.com/pbleisch/kept/internal/ledger"
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
// file order. The paths are on refs/kept/ledger, which no worktree checks out.
const attributes = `# Text is normalized on the way in, and normalized again inside the binding
# hash — see decision 0022. This file is hygiene; the hash is the guarantee.
* text=auto eol=lf

# The ledger is append-only, so concurrent appends from two checkouts should
# keep both sides. Safe only because every derived read folds by event time,
# not by file order. These paths live on refs/kept/ledger. See decisions 0011
# and 0016.
ledger/**/*.jsonl merge=union
`

// textRule is the attribute 0022 names. It is looked for by the pattern and the
// attribute rather than by the whole line, because a workspace that spells it
// `* text=auto` or sets it per extension has already answered the question and
// does not need telling.
const textRule = "* text=auto eol=lf"

func configText(root string) string {
	return fmt.Sprintf(`# Where the corpus lives, relative to this directory. Leave it out and the
# corpus root is this directory, which is what a manuscript wants: there the
# corpus is the content. See docs/layout.md and decision 0028.
corpus: %s

# Deadlines. Nothing external runs without one, and a timeout resolves to the
# same outcome as the matching failure — see decision 0026.
timeouts:
  gatherer: 10m

# Confinement. An unset provider is not the same as none: a run refuses to
# dispatch until one is named, and the canary must pass before an agent starts.
# See decision 0017.
confine:
  provider:

# Retention is configured here too, and arrives with the machinery that reads
# it rather than sitting here as a key nothing consults. See decision 0020.
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
// One thing decision 0027 asks of this verb is not here yet: `kept agent
// install`, because there is no generator to run. The three git settings are
// installed below when this workspace is a Git checkout, and each is reported:
// the ledger's upstream fetch refspec and the reflog the ledger ref needs
// (decisions 0062 and 0054), and the ordinary branch refspec beside them, which
// is added only when nothing already fetches branches into remote-tracking
// refs — see [ledger.BranchRefspec] for what its absence was costing.
//
// The pre-push hook is the fourth, and it is the first file kept writes into
// the git directory rather than a setting it configures. It says at push time
// what a push cannot: git push does not carry a side ref, so the ledger stays
// on the machine while git status reads clean. It warns and never publishes,
// and it is written here rather than behind a flag for the reason the three
// settings are — a workspace that has to ask for its own safety net does not
// have one. See decision 0073.
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
		{filepath.Join(corpus.ConfigDir, corpus.ConfigFile), configText(dir)},
		{".gitattributes", attributes},
	} {
		written, err := put(filepath.Join(wd, f.path), f.content)
		if err != nil {
			fmt.Fprintf(stderr, "kept: %s\n", err)
			return exitUsage
		}
		note(existing(written), f.path)
	}

	// A manuscript can be initialized before it is committed to Git. It has no
	// ref to configure yet, so this is deliberately a no-op there rather than a
	// reason init refuses to make its corpus and config.
	//
	// What is there is read before it is changed, so the report can name the
	// line that was removed rather than the two lines it could have been.
	direct, _ := ledger.DirectFetch(wd)
	fetch, branches, log, err := ledger.ConfigureFetch(wd)
	if err != nil && !errors.Is(err, ledger.ErrNotRepository) {
		fmt.Fprintf(stderr, "kept: configure the ledger ref: %s\n", err)
		return exitUsage
	}
	if branches == ledger.FetchAdded {
		note("configured", "origin fetches its branches into refs/remotes/origin/")
	}
	switch fetch {
	case ledger.FetchAdded:
		note("configured", "origin fetches "+ledger.Ref+" into "+ledger.UpstreamRef)
	case ledger.FetchRedirected:
		note("corrected", "origin fetches "+ledger.Ref+" into "+ledger.UpstreamRef+", not over it")
	}
	if log == ledger.LogEnabled {
		note("configured", "git logs every update to "+ledger.Ref)
	}

	hook, err := ledger.ConfigureHook(wd)
	if err != nil && !errors.Is(err, ledger.ErrNotRepository) {
		fmt.Fprintf(stderr, "kept: write the %s hook: %s\n", ledger.PrePushHook, err)
		return exitUsage
	}
	switch hook.State {
	case ledger.HookWritten:
		note("wrote", relative(wd, hook.Path))
	case ledger.HookPresent:
		note("already there", relative(wd, hook.Path))
	}

	fmt.Fprintf(stdout, "%s\n", strings.Join(report, "\n"))
	if branches == ledger.FetchAdded {
		noteBranchFetch(stdout)
	}
	if fetch == ledger.FetchRedirected {
		warnDirectFetch(stdout, direct)
	}
	if log == ledger.LogEnabled {
		noteLedgerLog(stdout)
	}
	noteHook(stdout, wd, hook)
	advise(stdout, wd, dir)
	return exitOK
}

// noteBranchFetch says what was added to somebody's git config and why, for the
// reason warnDirectFetch does. It is the smallest of the three writes and the
// easiest to read as noise, so it says what was broken rather than what is now
// set.
func noteBranchFetch(w io.Writer) {
	fmt.Fprintf(w, "\nremote.origin.fetch named the ledger and no branch, so an ordinary git fetch\n"+
		"updated nothing under refs/remotes/origin/. Added beside it:\n\n    %s\n\n"+
		"It was costing a ref that reads as current and is not. Nothing was removed\nor narrowed.\n",
		ledger.BranchRefspec)
}

// warnDirectFetch says what was changed in somebody's git config and why,
// because a config kept rewrites without a word is the other half of the
// failure docs/decision/0016 is about. It names the lines that were there
// rather than the ones that could have been.
func warnDirectFetch(w io.Writer, direct []string) {
	fmt.Fprintf(w, "\nremote.origin.fetch fetched the ledger straight into itself:\n\n    %s\n\n"+
		"and that is now\n\n    %s\n\n"+
		"An ordinary git fetch rewound the ledger over acts nobody pushed, or refused\noutright. No ref moved.\n",
		strings.Join(direct, "\n    "), ledger.FetchRefspec)
}

// noteLedgerLog says what was changed in somebody's git config and why, for the
// reason warnForcedFetch does — and it says what the setting does not cover,
// because a guard an operator over-reads is worse than one they can size.
func noteLedgerLog(w io.Writer) {
	fmt.Fprintf(w, "\n%s is now %q, so git keeps a reflog for\n"+
		"%s; the value true does not do it. It is repository-wide, at\n"+
		"roughly 120 bytes per ref update.\n",
		"core.logAllRefUpdates", ledger.LogAllRefUpdates, ledger.Ref)
}

// noteHook says why kept wrote no pre-push hook, in the two states where it
// wrote none and something is worth saying. Both are somebody else's decision
// about their own repository, so neither is corrected and neither refuses
// anything: what kept owes them is a plain statement that the push-time warning
// is not there.
//
// A hook kept did not write is left exactly as it is. Git runs one pre-push
// hook, so the alternatives are editing a program a person wrote or leaving a
// file beside it that nothing runs — one is refused by policy and the other
// reads as installed and is not. See decision 0073.
func noteHook(w io.Writer, wd string, hook ledger.Hook) {
	switch hook.State {
	case ledger.HookForeign:
		fmt.Fprintf(w, "\n%s is yours and was left as it is, so nothing at push time\n"+
			"says the ledger is unpublished. kept doctor still reports it.\n",
			relative(wd, hook.Path))
	case ledger.HookBypassed:
		fmt.Fprintf(w, "\ncore.hooksPath sends git elsewhere for its hooks:\n\n    %s\n\n"+
			"so kept wrote none, and nothing at push time says the ledger is unpublished.\n"+
			"That directory is whatever hook runner you configured. kept doctor still\nreports it.\n",
			hook.HooksPath)
	}
}

// relative names a path the way the report names the others: from the workspace
// where it is inside one. A linked worktree's hooks live in the common
// directory, which is outside this tree, and an absolute path is what that is.
func relative(wd, path string) string {
	rel, err := filepath.Rel(wd, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return path
	}
	return filepath.ToSlash(rel)
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
			"text, and kept wants one:\n\n    %s\n\n"+
			"Without it the bytes churn in every diff. Ratifications are unaffected\neither way.\n", textRule)
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
