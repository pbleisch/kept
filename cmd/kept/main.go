// Command kept reads a corpus of promises and prints what it found.
//
// Only the verbs that read content exist so far. The verbs that answer whether
// a promise is kept need standing and truth, and neither is built; they exit 2
// rather than pretending, because a verb that answers "kept" from a corpus it
// never checked against the world is the one lie the tool must not tell.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/pbleisch/kept/internal/corpus"
)

// Set by the build. See the Makefile.
var (
	version = "dev"
	commit  = "unknown"
)

// Exit codes are the contract in docs/decision/0027. Only two of them can be
// reached yet: nothing has standing, so nothing can be kept, not kept, unknown
// or refused.
const (
	exitOK    = 0
	exitUsage = 2
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
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
	}

	if unbuilt[verb] {
		fmt.Fprintf(stderr, "kept: %s needs standing or truth, and neither is built yet\n", verb)
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
	"ratify": true, "release": true, "status": true, "drift": true,
	"why": true, "doctor": true, "init": true, "agent": true,
	"land": true, "promote": true,
}

func usage(w io.Writer) {
	fmt.Fprint(w, `kept — a promise is kept, or it is not.

Usage:
  kept list [--corpus DIR]        every artifact, and everything wrong with them
  kept show <artifact> [--corpus DIR]
                                  one artifact as it was read
  kept version
  kept help

An artifact is named "type:slug", or by its slug alone when only one type
answers to it. The corpus root comes from .kept/config.yaml unless --corpus
says otherwise.

The verbs that answer whether a promise is kept — ratify, status, drift, why —
need standing and truth. Neither is built, so they exit 2.
`)
}

// open resolves the corpus root and loads it. A load error is a usage error by
// docs/decision/0027: an unparseable corpus is exit 2, and a duplicate id is
// printed with every path claiming it so that fixing it needs no second command.
func open(root string, stderr io.Writer) (*corpus.Corpus, int) {
	if root == "" {
		wd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(stderr, "kept: %s\n", err)
			return nil, exitUsage
		}
		resolved, _, err := corpus.FindRoot(wd)
		if err != nil {
			fmt.Fprintf(stderr, "kept: %s\n", err)
			return nil, exitUsage
		}
		root = resolved
	}

	c, err := corpus.Load(root)
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
