// Command kept reads a corpus of promises, records what a person has committed
// to, observes the world, and prints which of those commitments still hold.
//
// kept with no verb is the tick: gather what is due, evaluate it, record what
// was found, and record what it did about anything with standing and without
// truth. It ticks once and exits — there is no scheduler and no daemon, and
// driving it on a timer is cron's job.
//
// kept repair hands one promise that is not kept to an agent in a box and
// prints the bounded diff it produced. It lands nothing: the merge is a
// person's hands on git. What is left is three verbs that are unwritten, each
// carrying the reason it is.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/pbleisch/kept/internal/confine"
	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/gather"
	"github.com/pbleisch/kept/internal/ledger"
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
// Four of the five are reachable, and truth is what widened two of them. 1 is
// drift: something ratified is not kept, or does not stand at the content it
// has now. 3 is everything kept could not decide — a gatherer that could not
// run, a promise nothing has ever observed, an answer about a tree this is not,
// a grant computed under canonicalization rules this build does not use, or a
// ledger line that could not be read.
//
// The split between 1 and 3 is the whole reason the verdict is three-valued. A
// CI job that treats "the toolchain was missing" as "the promise is broken"
// produces exactly the false report the model exists to prevent, and CI is
// where that collapse does damage.
//
// 4 is a run that refused to answer, and two things reach it: a ledger whose
// ref has gone backwards, and a kept ledger sync against a ledger that has
// forked from what the last fetch brought. Every other refusal this build
// records is a refusal to *act* on drift it has already reported, which is 1.
// The distinction is that both of these make the question unanswerable rather
// than the answer unwelcome — the record standing would be derived from is
// known to be incomplete, which is the archetype of the stale view this code
// was reserved for. A fork is the second case and not the first: the records
// are all still in this repository, and what is missing is a commit naming both
// tips. See docs/decision/0054 and docs/decision/0062.
const (
	exitOK      = 0
	exitDrift   = 1
	exitUsage   = 2
	exitUnknown = 3
	exitRefused = 4
)

// How a moment is printed. An act is something a person performed, so it is
// shown to the second; a table of them is shown to the day.
const (
	stamp = "2006-01-02 15:04:05Z07:00"
	day   = "2006-01-02"
)

// startRun names this execution. Every record it appends carries the id, and
// the diagnostics behind that id are written the first time it appends
// anything. See docs/decision/0012.
func startRun(args []string) *run.Record { return run.Start(version, args) }

// ledgerStore keeps every Git workspace's machine-written tier on the ref no
// worktree checks out. A non-Git manuscript has no possible repair worktree,
// so ledger.ForWorkspace preserves its existing local-only behavior.
func ledgerStore(c *corpus.Corpus) ledger.Store { return ledger.ForWorkspace(c.Workspace) }

func main() {
	if confine.CanaryArgs(os.Args[1:]) {
		os.Exit(confine.RunCanary(os.Args[1:], os.Stdout))
	}
	os.Exit(dispatch(os.Args[1:], os.Stdout, os.Stderr))
}

func dispatch(args []string, stdout, stderr io.Writer) int {
	// No verb is the tick. kept is run the way make is, and the thing a person
	// types most often should be the thing it does.
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return tickCmd(args, stdout, stderr)
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
	case "drift":
		return driftCmd(rest, stdout, stderr)
	case "why":
		return whyCmd(rest, stdout, stderr)
	case "tick":
		return tickCmd(rest, stdout, stderr)
	case "repair":
		return repairCmd(rest, stdout, stderr)
	case "ledger":
		return ledgerCmd(rest, stdout, stderr)
	case "doctor":
		return doctorCmd(rest, stdout, stderr)
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
// None of them is blocked any more. repair was, on a box to work in, and there
// is one: it cuts a worktree, dispatches under a verified provider and gives
// back a bounded diff. The three that are left are unwritten, and saying they
// need truth would misstate why — what agent generates it reads out of the
// config, and land has nothing to land until a person has read a diff.
var unbuilt = map[string]string{
	"agent": "is unwritten rather than blocked: it generates what another agent must not touch " +
		"out of the config that enforces it, and nothing generates it yet",
	"land": "is unwritten, and has nothing to land: a land puts a gated diff on main, " +
		"and no thread produces one yet",
	"promote": "is unwritten: it moves a fact to a wider scope, and a fact's scope is not modelled yet",
}

func usage(w io.Writer) {
	fmt.Fprint(w, `kept — a promise is kept, or it is not.

Usage:
  kept [--all] [--json]           the tick: gather what is due, evaluate it,
                                  record what was found, and print what is kept,
                                  what is not, and what nobody could measure
  kept drift [--json]             what has standing and lacks truth. It reads
                                  what the tick found and observes nothing
  kept why <artifact> [--json]    one artifact, whole: its standing, its truth
                                  with the series behind it, and what the tick
                                  did about any drift — including why it was
                                  left alone
  kept init [--corpus DIR]        make this directory a workspace: the config,
                                  the corpus root it names, and the
                                  .gitattributes decision 0022 asks for. What
                                  you already wrote is left as it is
  kept repair [<promise>] [--dry-run] [--keep]
                                  hand one promise that is not kept to an agent
                                  in a box, and print the bounded diff it
                                  produced. With no promise it lists the ones it
                                  could be pointed at. Nothing lands: the merge
                                  is your hands on git
  kept ledger sync                advance the ledger onto what the last git
                                  fetch brought, when that is a fast-forward.
                                  It touches no network: a fetch writes the
                                  upstream ref, and advancing is the act
  kept doctor [--fix] [--corpus DIR]
                                  what is wrong with the names: a file nothing
                                  identifies, a filename that is not the slug,
                                  standing whose content is gone. --fix writes
                                  the frontmatter an adopted file is missing,
                                  and nothing else
  kept list [--corpus DIR]        every artifact, and everything wrong with them
  kept show <artifact> [--corpus DIR]
                                  one artifact as it was read
  kept status [--corpus DIR] [--json]
                                  what has standing, and what has been edited
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

There is no scheduler. A promise is due when the bytes in its scope move, when
its answer is older than its max_age, or when nothing has answered for what it
says now — so a tick costs one walk of the workspace, and driving it on a timer
is cron's job rather than kept's. --all forces every artifact.

Standing is never written to an artifact, and neither is truth. Both are ledger
records naming a hash of what the artifact said, so editing the file makes both
stop applying with nothing having to notice the edit. There is no invalidation
step anywhere.

A verdict has three values. `+"`"+`unknown`+"`"+` is not a failure: a gatherer that could not
run does not record what it thinks it saw, and reporting that as `+"`"+`not-kept`+"`"+` is
the one lie this tool must not tell.

agent, land and promote are unwritten. They exit 2, and each says why.

A repair is one promise at a time, named. Choosing which of several drifts to
spend a live agent session on is a ranking and a budget, and neither is built,
so the cut is a person naming one. A tick never dispatches. See decision 0060.

Exit codes, so a tick can be a hook or a step in CI:

  0   everything with standing is kept
  1   drift: something with standing is not kept, or was ratified and does not
      stand at the content it has now
  2   usage — bad flags, an unknown verb, a corpus that will not parse
  3   kept could not tell: a gatherer that could not run, a promise nothing has
      observed, an answer about a tree this is not, or a ledger line that could
      not be read
  4   kept refused to answer: the ledger ref has held a value it can no longer
      reach, so the record standing is derived from is incomplete. kept list and
      kept doctor still read the corpus

See decisions 0027 and 0054.

--json is one document for status, drift, why and the tick, carrying the
schema version it was written under. Its verdict is a three-valued string and
never a boolean.
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

	// The gatherers kept ships are what an artifact's gather map and its
	// hold_when are checked against, and this is where the two halves are
	// handed to one another: the corpus reads a corpus and runs nothing, and
	// the catalogue knows how to measure the world. See docs/decision/0036.
	c, err := corpus.Load(root, workspace, corpus.Options{Gatherers: gather.Builtin()})
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

// sound refuses the run when the ledger ref has held a value it can no longer
// reach, and reports nothing at all otherwise.
//
// Every verb that reads or writes the ledger calls this, and kept list, kept
// doctor and kept show do not. That split is the point rather than an omission:
// the two verbs that read the corpus are what an operator has left in this
// state, and doctor is where the missing reflog behind it gets reported. A
// verdict derived from a ledger known to be incomplete is the one answer this
// tool must not give confidently.
//
// A reflog that cannot be read is *nothing has been recorded*, not a rewind.
// It is said on stderr and the run continues: refusing every verb because a
// diagnostic failed would make an unreadable reflog worse than a rewound
// ledger, and nothing here has established that anything is wrong. See
// docs/decision/0054.
func sound(c *corpus.Corpus, stderr io.Writer) int {
	continuity, err := ledger.Continuous(ledgerStore(c))
	if err != nil {
		fmt.Fprintf(stderr, "kept: read the ledger ref's reflog: %s\n", err)
		return exitOK
	}
	if !continuity.Rewound() {
		return exitOK
	}

	fmt.Fprintf(stderr, "kept: %s has gone backwards, so what is ratified cannot be answered.\n\n", ledger.Ref)
	for _, value := range continuity.Lost {
		fmt.Fprintf(stderr, "  %s      recorded as this ref's value, and %s cannot reach it\n",
			shortCommit(value), shortCommit(continuity.Held))
	}
	fmt.Fprintf(stderr, "\nNothing kept does removes a ledger record — it only ever moves this ref\n"+
		"forward from a value it just read — so something else moved it. Run\n\n"+
		"    git reflog show %s\n\n"+
		"to see what, and when: git writes the reason on every entry.\n\n"+
		"Recover by committing a union of the two tips. This goes quiet the moment\n"+
		"every recorded value is reachable again, and stays standing if the lost\n"+
		"records are discarded instead. Do it before the entry expires: the reflog\n"+
		"entry is what keeps the lost commit alive, and entries expire at\n"+
		"gc.reflogExpire, 90 days by default.\n\n"+
		"kept list and kept doctor read the corpus and still answer. See decision 0054.\n", ledger.Ref)
	return exitRefused
}

// shortCommit is a commit id as a person reads one. Seven characters, because
// that is what git reflog show prints and this text exists to be compared
// against it. Diagnostic only: nothing is addressed by an abbreviation.
func shortCommit(value string) string {
	if len(value) < 7 {
		return value
	}
	return value[:7]
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
