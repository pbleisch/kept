// Command kept reads a corpus of promises, records what a person has committed
// to, and prints which of those commitments still apply.
//
// The verbs that read content and the ones that create and retire standing
// exist. The verbs that answer whether a promise is *kept* need truth, which
// comes from observing the world, and nothing observes yet; they exit 2 rather
// than pretending, because a verb that answers "kept" from a corpus it never
// checked against the world is the one lie the tool must not tell.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/run"
)

// Set by the build. See the Makefile.
var (
	version = "dev"
	commit  = "unknown"
)

// Exit codes are the contract in docs/decision/0027. Only two of them can be
// reached yet: standing exists, but nothing observes the world, so nothing can
// be kept, not kept, unknown or refused.
//
// kept status therefore exits 0 on any successful read, including one that
// finds ratified content missing. Absence is a drift kind, and reporting drift
// through an exit code is kept drift's contract rather than the register's —
// giving one verb a truth-shaped code before truth exists would make the other
// inconsistent when it arrives.
const (
	exitOK    = 0
	exitUsage = 2
)

// How a moment is printed. A decision is an act a person performed, so it is
// shown to the second; a table of them is shown to the day.
const (
	stamp = "2006-01-02 15:04:05Z07:00"
	day   = "2006-01-02"
)

// startRun names this execution. Every record it appends carries the id, and
// the diagnostics behind that id are written the first time it appends
// anything. See docs/decision/0012.
func startRun(args []string) *run.Record { return run.Start(version, args) }

// ledgerRoot is where this workspace keeps its ledger.
//
// Plain files under .kept/ledger, which is deliberate and temporary: the ledger
// has to be on a ref no working tree checks out before the first box exists, or
// the repair agent's worktree contains the one thing it must never be able to
// write. Boxes arrive with increment 5 and the move is an implementation of
// internal/ledger's Store. See docs/decision/0016.
func ledgerRoot(c *corpus.Corpus) string {
	return filepath.Join(c.Workspace, corpus.ConfigDir)
}

func main() {
	os.Exit(dispatch(os.Args[1:], os.Stdout, os.Stderr))
}

func dispatch(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return exitUsage
	}

	verb, rest := args[0], args[1:]
	switch verb {
	case "help", "-h", "--help":
		usage(stdout)
		return exitOK
	case "version", "--version":
		fmt.Fprintf(stdout, "kept %s (%s)\n", version, commit)
		return exitOK
	case "list":
		return listCmd(rest, stdout, stderr)
	case "show":
		return showCmd(rest, stdout, stderr)
	case "ratify":
		return ratifyCmd(rest, stdout, stderr)
	case "release":
		return releaseCmd(rest, stdout, stderr)
	case "status":
		return statusCmd(rest, stdout, stderr)
	}

	if unbuilt[verb] {
		fmt.Fprintf(stderr, "kept: %s needs truth, and nothing observes the world yet\n", verb)
		return exitUsage
	}
	fmt.Fprintf(stderr, "kept: unknown verb %q\n", verb)
	usage(stderr)
	return exitUsage
}

// unbuilt are the verbs the model names and nothing implements. They are listed
// so that typing one gets an answer about kept rather than a spelling
// correction.
var unbuilt = map[string]bool{
	"drift": true, "why": true, "doctor": true, "init": true,
	"agent": true, "land": true, "promote": true, "repair": true,
}

func usage(w io.Writer) {
	fmt.Fprint(w, `kept — a promise is kept, or it is not.

Usage:
  kept list [--corpus DIR]        every artifact, and everything wrong with them
  kept show <artifact> [--corpus DIR]
                                  one artifact as it was read
  kept status [--corpus DIR]      what has standing, and what has been edited
                                  since it was granted
  kept ratify <artifact> [--by WHO] [--note WHY]
                                  grant standing to what this artifact says now
  kept release <artifact> [--by WHO] [--note WHY]
                                  retire standing. The only way a promise stops
                                  being held to; deleting its file is not one
  kept version
  kept help

An artifact is named "type:slug", or by its slug alone when only one type
answers to it. The corpus root comes from .kept/config.yaml unless --corpus
says otherwise, in which case that directory is the workspace root too.

A link names an artifact, a file in the workspace, or a URI. Whether a URI
leads anywhere is a question for the world, so nothing here asks it.

Standing is never written to an artifact. It is a record naming a hash of what
the artifact said, so editing the file makes the record stop matching and the
standing stop applying — with nothing having to notice the edit. There is no
invalidation step, and kept status is how you see the result across the corpus
at once.

The verbs that answer whether a promise is kept — drift, why, repair — need
truth, which means observing the world. Nothing observes yet, so they exit 2.
`)
}

// open resolves the corpus root and loads it. A load error is a usage error by
// docs/decision/0027: an unparseable corpus is exit 2, and a duplicate id is
// printed with every path claiming it so that fixing it needs no second command.
//
// Two roots are needed, because a file reference is relative to the workspace
// and an artifact lives in the corpus, and 0028 keeps those apart. Config
// supplies both. --corpus sets both to the directory given, which is what a
// self-contained corpus wants and the only reading that needs no second flag.
func open(root string, stderr io.Writer) (*corpus.Corpus, int) {
	workspace := root
	if root == "" {
		wd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(stderr, "kept: %s\n", err)
			return nil, exitUsage
		}
		resolved, base, err := corpus.FindRoot(wd)
		if err != nil {
			fmt.Fprintf(stderr, "kept: %s\n", err)
			return nil, exitUsage
		}
		root, workspace = resolved, base
	}

	c, err := corpus.Load(root, workspace)
	if err != nil {
		if dup, ok := errors.AsType[*corpus.DuplicateIDError](err); ok {
			for _, claim := range dup.Claims {
				fmt.Fprintf(stderr, "kept: %d artifacts claim %s:\n", len(claim.Paths), claim.ID)
				for _, p := range claim.Paths {
					fmt.Fprintf(stderr, "        %s\n", p)
				}
			}
			fmt.Fprint(stderr, "kept: an id names one artifact. Change one of them, or delete it.\n")
			return nil, exitUsage
		}
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return nil, exitUsage
	}
	return c, exitOK
}

func flags(name string, stderr io.Writer) (*flag.FlagSet, *string) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("corpus", "", "corpus root, overriding .kept/config.yaml")
	return fs, root
}

// parse reads flags that may appear on either side of the arguments, because
// that is where people put them. The standard parser stops at the first
// argument that is not a flag, so it is resumed after each one.
func parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return positional, nil
		}
		positional = append(positional, args[0])
		args = args[1:]
	}
}
