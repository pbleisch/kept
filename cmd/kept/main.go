// Command kept reads a corpus of promises, records what a person has committed
// to, observes the world, and prints which of those commitments still hold.
//
// kept with no verb is the tick: gather what is due, evaluate it, record what
// was found, and record what it did about anything with standing and without
// truth. It ticks once and exits — there is no scheduler and no daemon, and
// driving it on a timer is cron's job.
//
// kept repair hands one promise that is not kept to an agent in a box, commits
// what the session left, and prints the bounded diff it produced. kept land
// puts one of those on origin/main, gated against the tree it lands on. What is
// left is two verbs that are unwritten, each carrying the reason it is.
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
// a grant computed under canonicalization rules this build does not use, a
// ledger line that could not be read, or a run whose records never landed
// because another run held the ledger. See [unrecorded] for that last one,
// which used to be 2 and was the code saying the operator had asked for
// something wrong.
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

// unrecorded is a ledger write that did not land, as an exit code.
//
// A run that spent [ledger.RefUpdateAttempts] attempts and still lost the ref's
// compare-and-swap exits 3, and the code it exited before was 2. Two means the
// operator asked for something wrong, and losing a race to another run is not
// something an operator asked for at all — it is the ordinary consequence of
// running kept twice at once, which is a thing kept is built to allow.
//
// Three is the honest one and it is 0027's own definition rather than a
// widening of it. That table's line is *the world disagrees with a commitment*
// against *there is no evidence either way*, and a run whose records never
// landed produced no evidence: it gathered, it evaluated, and then it wrote
// nothing and printed nothing. Nothing about the tree is known false, so 1
// would be the CLI recording what it thinks it saw. And 4 would say the record
// standing is derived from is incomplete, which is the one thing that is
// certainly not true here — the ledger is intact and consistent, and this run
// is simply not in it.
//
// A write that failed for a real reason is still 2, and the discrimination is
// [ledger.ErrContended] rather than a message: a broken ref, a permission
// problem or a torn line is never retried in the first place, so the longer
// schedule cannot turn one into this. See docs/decision/0064.
func unrecorded(stderr io.Writer, err error) int {
	if !errors.Is(err, ledger.ErrContended) {
		return exitUsage
	}
	fmt.Fprint(stderr, "kept: another run held the ledger, so nothing was recorded. Run it again.\n")
	return exitUnknown
}

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
	case "new":
		return newCmd(rest, stdout, stderr)
	case "list":
		return listCmd(rest, stdout, stderr)
	case "show":
		return showCmd(rest, stdout, stderr)
	case "shape":
		return shapeCmd(rest, stdout, stderr)
	case "ratify":
		return ratifyCmd(rest, stdout, stderr)
	case "retire":
		return retireCmd(rest, stdout, stderr)
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
	case "land":
		return landCmd(rest, stdout, stderr)
	case "ledger":
		return ledgerCmd(rest, stdout, stderr)
	case "doctor":
		return doctorCmd(rest, stdout, stderr)
	case "init":
		return initCmd(rest, stdout, stderr)
	}

	if now, ok := renamed[verb]; ok {
		fmt.Fprintf(stderr, "kept: %s is now %s\n", verb, now)
		return exitUsage
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
// is one; land was, on a thread that produced something to land, and a repair
// now anchors what its session left. The two that remain are unwritten, and
// saying they need truth would misstate why — what agent generates it reads out
// of the config.
var renamed = map[string]string{
	"release": "retire",
}

var unbuilt = map[string]string{
	"agent": "is unwritten: it would generate what another agent must not touch, out of the " +
		"config that enforces it, and nothing generates that yet",
	"promote": "is unwritten: it moves a fact to a wider scope, and a fact's scope is not modelled yet",
}

func usage(w io.Writer) {
	fmt.Fprint(w, `kept — a promise is kept, or it is not.

Usage:
  kept [--all] [--json] [--quiet]
                                  the tick: gather what is due, evaluate it,
                                  record it, and print what is kept, what is
                                  not, and what nobody could measure
  kept drift [--json] [--kind T] [--id GLOB] [--state K]
                                  what has standing and lacks truth. Reads what
                                  the tick found; observes nothing
  kept why <artifact> [--json]    one artifact: its standing, its truth and the
                                  series behind it, and what the tick did about
                                  any drift
  kept init [--corpus DIR]        make this directory a workspace: the config,
                                  the corpus root it names, and the git settings
                                  the ledger needs. Existing files are left
                                  alone
  kept repair [<promise>] [--dry-run] [--keep] [--quiet]
                                  hand one promise that is not kept to an agent
                                  in a box, and print the bounded diff. With no
                                  promise, list the ones it can take. Nothing
                                  lands
  kept land [<thread>] [--dry-run] [--keep] [--quiet] [--by WHO]
                                  gate one repaired thread and put it on
                                  origin/main. With no thread, list the ones
                                  that anchored a commit
  kept ledger sync                advance the ledger onto what the last git
                                  fetch brought, when that is a fast-forward.
                                  Makes no network call
  kept doctor [--fix] [--corpus DIR]
                                  what is wrong with the names — a file nothing
                                  identifies, a filename that is not the slug,
                                  an id of the wrong shape, two artifacts
                                  numbered alike, standing whose content is gone
                                  — and a git config the ledger needs. --fix
                                  writes a missing id and renumbers a contested
                                  number, and refuses where the ledger names one
  kept new <type> <slug> [--title T] [--set field=value] [--stdout]
                                  a new artifact of that type, with the
                                  frontmatter its schema declares. A required
                                  field that binds arrives on --set; the rest
                                  are comments carrying the shape. Writes the
                                  file and nothing else — no ledger record, no
                                  standing — and refuses to overwrite
  kept list [--corpus DIR] [--kind T] [--id GLOB]
                                  every artifact, and everything wrong with them
  kept show <artifact> [--corpus DIR]
                                  one artifact as it was read
  kept shape <artifact> --trial [--against DIR] [--quiet]
                                  measure one artifact against a tree and print
                                  what each of its instruments said, beside the
                                  verdict its predicate reaches. Records
                                  nothing, and needs no standing — it is how you
                                  find out whether a check can fire before you
                                  ratify it
  kept status [--corpus DIR] [--json] [--all] [--kind T] [--id GLOB] [--state S]
                                  what needs attention: never granted, edited
                                  since it was granted, or ratified and gone.
                                  --all is the whole register
  kept ratify <artifact> [--by WHO] [--note WHY]
                                  grant standing to what this artifact says now
  kept retire <artifact> [--by WHO] [--note WHY]
                                  end standing. Deleting the file does not
  kept version
  kept help

An artifact is named "type:slug", or by its slug alone when only one type
answers to it. The corpus root comes from .kept/config.yaml unless --corpus
says otherwise, in which case that directory is the workspace root too.

kept status prints what needs attention and counts the rest: an artifact nobody
ratified, one edited since it was, one whose grant this build cannot compare
against, and one whose ratified content has gone. A ratified artifact and a
retired one are not findings and are not listed, and neither is one whose kind
declares no hash list — nothing about it binds, so nothing can ratify it, and a
row nobody can act on is a permanent entry in a list of things to do. It is
counted in a line saying so. --all is the whole register.

kept status, kept drift and kept list narrow the same three ways, and the flags
compose: repeat one and it is "or", give two and it is "and". A selector narrows
the population status then draws attention within; --state names states by hand,
so it replaces that default rather than composing with it.

  --kind TYPE    only artifacts of that type. The types are whatever
                 docs/_schema declares, so a kind added tomorrow is filterable
                 with no code change
  --id GLOB      only artifacts whose whole printed id matches — "type:slug",
                 the spelling every table prints and every verb takes. So
                 --id "promise:*" is the promises and --id "*tests*" is anything
                 with that in either half. Quote it, or the shell expands it
  --state S      only artifacts in that state, in the words the verb prints.
                 Each verb takes its own vocabulary and refuses the other by
                 name: status takes ratified, unratified, never-ratified,
                 retired and absent; drift takes unverified, not-kept,
                 regressed, stale, unknown, conflicted and absent. kept list
                 reports no state and takes no --state

A filter that matched nothing says so instead of printing an empty table, and
which kind of nothing it is decides the exit code. --kind and --id name a
population, so naming one that does not exist exits 3: the question was never
put. --state asks about that population, so a state nothing is in is an answer
and exits as an empty set does. Problems are never narrowed. The code describes
the narrowed set; unfiltered, every code is what it was.

The tick takes none of them: a filter there narrows what is recorded rather
than what is shown. --all is the flag it does have.

There is no scheduler. A promise is due when the bytes in its scope move, when
its answer is older than its max_age, or when nothing has answered for what it
says now, so a tick costs one walk of the workspace. Run it on a timer with
cron. --all forces every artifact.

Standing and truth are never written to an artifact. Both are ledger records
naming a hash of what the artifact said, so editing the file makes both stop
applying. There is no invalidation step.

A verdict has three values. unknown is not a failure: a gatherer that could not
run does not record what it thinks it saw, and kept never reports that as
not-kept.

agent and promote are unwritten. They exit 2, and each says why.

kept land is the only verb that reaches the network. No tick and no repair ever
lands.

kept repair takes one promise at a time, named. Ranking several drifts is not
built, and a tick never dispatches.

Exit codes, so a tick can be a hook or a step in CI:

  0   everything with standing is kept
  1   drift: something with standing is not kept, or was ratified and does not
      stand at the content it has now
  2   usage — bad flags, an unknown verb, a corpus that will not parse
  3   kept could not tell: a gatherer that could not run, a promise nothing has
      observed, an answer about a tree this is not, or a ledger line that could
      not be read
  4   kept refused to answer: the ledger ref has held a value it can no longer
      reach, so the record standing comes from is incomplete. kept list and
      kept doctor still read the corpus

--json is one document for status, drift, why and the tick, carrying the schema
version it was written under. Its verdict is a three-valued string, never a
boolean. It carries every artifact the filters admit, needing attention or not,
with a boolean on each saying which the table printed: the default view is an
emphasis and never a narrowing of the question. It is the only stable format
here: the tables are sized from the data in them and free to change between
releases. They are the same bytes in a
terminal and in a pipe, so piping one is safe and parsing one is not.

A tick and a repair report progress on stderr, and only about the programs kept
started and does not control — an instrument measuring an artifact, a box being
cut, a confined session running — with how long each has taken against the
deadline that will fire. On a terminal that is one line, redrawn, gone before
the answer; anywhere else, timestamped append-only lines. --quiet turns it off;
stdout and the exit code are the same either way.
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
	fmt.Fprintf(stderr, "\n    git reflog show %s\n\n"+
		"says what moved it and when. Recover by committing a union of the two tips —\n"+
		"the reflog entry that keeps the lost commit alive expires at gc.reflogExpire,\n"+
		"90 days by default.\n\n"+
		"kept list and kept doctor read the corpus and still answer.\n", ledger.Ref)
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
	root := fs.String("corpus", "", "corpus root; overrides .kept/config.yaml")
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
