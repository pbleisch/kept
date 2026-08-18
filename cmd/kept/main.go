// Command kept reads a corpus of promises, records what a person has committed
// to, and prints which of those commitments still apply.
//
// The verbs that read content, the ones that create and retire standing, and
// the one that writes a workspace exist. The verbs that answer whether a
// promise is *kept* need truth, which comes from observing the world, and
// nothing observes yet; they exit 2 rather than pretending, because a verb that
// answers "kept" from a corpus it never checked against the world is the one
// lie the tool must not tell. The rest of what the model names is unwritten
// rather than blocked, and each says which of the two it is.
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

// Exit codes are the contract in docs/decision/0027, so that a hook or a CI job
// can branch on an answer rather than parse one.
//
// Three of the five are reachable. Standing exists and truth does not, so what
// reaches them is a register answer rather than a verdict: 1 is what was
// ratified and does not stand at the content it has now, and 3 is standing kept
// could not decide either way. Both meanings widen when truth arrives — 1 gains
// drift proper, 3 gains the gatherer that could not run — and neither changes
// for the cases already here.
//
// 4 is reserved and unreachable. A refusal is a stale view, an unconfigured
// provider, a failed canary or a budget, and nothing refuses until the tick
// does. It is named here rather than left as a hole, because the whole point of
// a reserved code is that nothing else takes it.
const (
	exitOK      = 0
	exitDrift   = 1
	exitUsage   = 2
	exitUnknown = 3
	exitRefused = 4
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
	case "init":
		return initCmd(rest, stdout, stderr)
	}

	if why, ok := unbuilt[verb]; ok {
		fmt.Fprintf(stderr, "kept: %s %s\n", verb, why)
		return exitUsage
	}
	fmt.Fprintf(stderr, "kept: unknown verb %q\n", verb)
	usage(stderr)
	return exitUsage
}

// unbuilt are the verbs the model names and nothing implements, each with the
// reason it is unavailable. They are listed so that typing one gets an answer
// about kept rather than a spelling correction, and each carries its own reason
// because they are not the same reason.
//
// Three of them are blocked: they answer whether a promise is kept, which needs
// truth, and nothing observes the world yet. The rest are merely unwritten, and
// saying they need truth would misstate why — doctor's checks already run, and
// what agent generates it reads out of the config.
var unbuilt = map[string]string{
	"drift": "is standing measured against truth, and nothing observes the world yet",
	"why":   "explains a verdict, and nothing has reached one: a verdict needs a gatherer and there are none",
	"repair": "needs drift to work on, which needs truth, and a box to work in, " +
		"which needs a worktree nothing cuts yet",

	"doctor": "is unwritten rather than blocked: its checks already run — path/id divergence " +
		"and duplicate ids in kept list, standing without content in kept status — and no verb collects them",
	"agent": "is unwritten rather than blocked: it generates what another agent must not touch " +
		"out of the config that enforces it, and nothing generates it yet",
	"land": "is unwritten, and has nothing to land: a land puts a gated diff on main, " +
		"and no thread produces one yet",
	"promote": "is unwritten: it moves a fact to a wider scope, and a fact's scope is not modelled yet",
}

func usage(w io.Writer) {
	fmt.Fprint(w, `kept — a promise is kept, or it is not.

Usage:
  kept init [--corpus DIR]        make this directory a workspace: the config,
                                  the corpus root it names, and the
                                  .gitattributes decision 0022 asks for. What
                                  you already wrote is left as it is
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
truth, which means observing the world, and nothing observes yet. The rest of
what the model names — doctor, agent, land, promote — is unwritten rather than
blocked. All of them exit 2, and each says which of the two it is.

Exit codes, so status can be a hook or a step in CI:

  0   nothing that was ratified has stopped standing
  1   something was ratified and does not stand now: edited since it was
      granted, or its content has gone missing
  2   usage — bad flags, an unknown verb, a corpus that will not parse
  3   kept could not tell: a grant computed under canonicalization rules this
      build does not use, or a ledger line that could not be read

4 is reserved for a refusal, and nothing refuses yet. See decision 0027.
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
