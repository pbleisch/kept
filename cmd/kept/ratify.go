package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pbleisch/kept/internal/canonical"
	"github.com/pbleisch/kept/internal/config"
	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/register"
	"github.com/pbleisch/kept/internal/run"
	worktree "github.com/pbleisch/kept/internal/workspace"
)

// ratifyCmd grants standing to what an artifact says right now.
//
// It is a person's act and it is the only way standing is created. Nothing else
// in this binary reaches [ledger.Ledger.Ratify] — not a rule, not an inference,
// and once there are agents, not one of them: an agent that can run kept can
// ratify, which is why docs/decision/0024 denies kept inside the box rather
// than trusting this file to stay short.
func ratifyCmd(args []string, stdout, stderr io.Writer) int {
	return perform("ratify", ledger.Ratified, args, stdout, stderr)
}

// retireCmd is a person retiring an artifact. For a kind that bears standing it
// ends standing, and it is how a promise stops being held to; deleting its file
// is not, because an absence cannot be told apart from a bad merge or a corpus
// root pointed at the wrong place. For a desire it declines the want and keeps
// the file. See docs/decision/0021, docs/decision/0074 for why the word is
// retire, and docs/decision/0083 for a desire.
func retireCmd(args []string, stdout, stderr io.Writer) int {
	return perform("retire", ledger.Retired, args, stdout, stderr)
}

func perform(verb string, kind ledger.Kind, args []string, stdout, stderr io.Writer) int {
	fs, root := flags(verb, stderr)
	by := fs.String("by", "", "who is performing this; else KEPT_BY, else operator: in your user config")
	note := fs.String("note", "", "why, recorded with the act")
	rest, err := parse(fs, args)
	if err != nil {
		return exitUsage
	}
	if len(rest) != 1 {
		fmt.Fprintf(stderr, "kept: %s takes one artifact, named \"type:slug\" or by its slug alone\n", verb)
		return exitUsage
	}

	c, code := open(*root, stderr)
	if code != exitOK {
		return code
	}
	// Before the act, not after: a ratification appended to a ledger known to
	// be incomplete would be a grant nobody can later derive standing from with
	// confidence, and this is the one verb that writes what standing is made of.
	if code := sound(c, stderr); code != exitOK {
		return code
	}
	// And onto what the last fetch brought, for the same reason and in the same
	// order: a grant appended to a ledger that is behind forks the durable
	// record. See docs/decision/0062.
	advance(c, stderr)

	id, hash, code := subject(kind, c, rest[0], stderr)
	if code != exitOK {
		return code
	}
	if kind == ledger.Ratified {
		if why := contestedNumber(c, id); why != "" {
			fmt.Fprint(stderr, why)
			return exitUsage
		}
	}

	// The register is read before the act, so an act that would change
	// nothing is refused rather than recorded. Nothing is written on the way
	// past: reading standing has never written anything anywhere.
	store := ledgerStore(c)
	acts, quarantined, err := ledger.ReadActs(store)
	if err != nil {
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return exitUsage
	}
	writeQuarantined(stderr, quarantined)

	proceed, code := wanted(kind, id, c, acts, stdout, stderr)
	if code != exitOK || !proceed {
		return code
	}

	who, code := operator(*by, stderr)
	if code != exitOK {
		return code
	}

	rec := startRun(append([]string{verb}, args...))
	l := ledger.Open(store, rec)
	d, err := record(l, kind, id, hash, who, *note, situate(c, stderr))
	if err != nil {
		l.Close(run.Failed)
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return unrecorded(stderr, err)
	}
	if err := l.Close(run.Clean); err != nil {
		fmt.Fprintf(stderr, "kept: the act was recorded and the run's diagnostics were not: %s\n", err)
	}

	writeAct(stdout, d, rec.ID)
	return exitOK
}

// contestedNumber refuses to ratify an artifact whose number another artifact in
// this corpus also claims, and names the one command that resolves it.
//
// **Ratification is the trap door.** While no artifact of a contested number is
// ratified, moving one is free — kept doctor --fix does it and carries every
// citation with it. Ratify one and the options narrow to moving the other;
// ratify both and there is no fix at all, because an id is the key every act is
// written against and the ledger is append-only. So the refusal is at the door
// rather than a diagnosis of the room behind it.
//
// It refuses for **either** artifact rather than letting the first through. *You
// cannot ratify a number two artifacts share* is one sentence a person can hold;
// *the first may and the second may not* is a race with a lecture attached, and
// it leaves the corpus in the half-ratified state where the fix stops being one
// command.
//
// A corpus-wide condition refusing a single-artifact verb is new here, and it is
// the right trade on this verb alone: ratify is the one that writes something
// nobody can take back. Retire is untouched — it is the way out, and a verb
// that refused to let go of a contested artifact would be a trap with no door.
//
// What it cannot see is a number contested across branches: each branch's corpus
// holds one artifact, so each ratification is clean where it is performed and
// the collision appears at the merge. kept list reports it there, and doctor
// says what is left. See docs/decision/0069.
func contestedNumber(c *corpus.Corpus, id string) string {
	kind, slug := corpus.SplitRef(id)
	sc, ok := c.Schemas.For(kind)
	if !ok || !sc.ID.Numbered() {
		return ""
	}
	n, after, well := sc.ID.Split(slug)
	if !well || after == "" {
		return ""
	}

	var also []string
	for _, e := range c.All() {
		if e.Artifact.ID == id || string(e.Artifact.Kind) != kind {
			continue
		}
		_, other := corpus.SplitRef(e.Artifact.ID)
		if m, rest, ok := sc.ID.Split(other); ok && rest != "" && m == n {
			also = append(also, e.Artifact.ID)
		}
	}
	if len(also) == 0 {
		return ""
	}

	return fmt.Sprintf("kept: %s cannot be ratified while %s is numbered %s as well.\n\n"+
		"Renumbering is free until one of them is ratified:\n\n    kept doctor --fix\n\nThen ratify.\n",
		id, and(also), sc.ID.Number(n))
}

func record(l *ledger.Ledger, kind ledger.Kind, id string, hash canonical.Hash, by, note string, where ledger.Where) (ledger.Act, error) {
	if kind == ledger.Ratified {
		return l.Ratify(id, hash.String(), hash.Norm, by, note, where)
	}
	return l.Retire(id, hash.String(), hash.Norm, by, note, where)
}

// situate is where this act is being performed, for the record and for nothing
// else. The act is recorded whatever comes back: the world is advisory, so a
// tree kept could not walk or a git that would not answer costs a diagnosis
// later and must never cost a person their act. See docs/decision/0063.
//
// One walk of the workspace, on a verb a person types by hand a few times a
// week. No stat cache is opened for it: the cache is the tick's, and warming it
// from here would make ratify a verb that writes into .kept/ on the way past.
func situate(c *corpus.Corpus, stderr io.Writer) ledger.Where {
	cfg, err := config.Read(filepath.Join(c.Workspace, corpus.ConfigDir))
	if err != nil {
		fmt.Fprintf(stderr, "kept: this act will name no world: %s\n", err)
		return ledger.Where{}
	}
	// Aliased because cmd/kept's own tests declare a type by that name, and a
	// test helper is not a reason to rename a package.
	walked, err := worktree.Read(c.Workspace, worktree.Options{
		Exclude: cfg.Exclude,
		Workers: runtime.NumCPU(),
	})
	if err != nil {
		fmt.Fprintf(stderr, "kept: this act will name no world: %s\n", err)
		return ledger.Where{}
	}

	where, err := ledger.Situate(c.Workspace, walked.Digest())
	if err != nil && !errors.Is(err, ledger.ErrNotRepository) {
		// The tree still stands: a world is a tree, and the rest is provenance
		// that is filled in from git where git answers. See docs/decision/0033.
		fmt.Fprintf(stderr, "kept: this act names its tree and no history: %s\n", err)
	}
	return where
}

func writeAct(w io.Writer, d ledger.Act, id run.ID) {
	fmt.Fprintf(w, "%s %s\n", d.Kind, d.Artifact)
	if d.Hash != "" {
		fmt.Fprintf(w, "  content  %s  (norm:%d)\n", d.Hash, d.Norm)
	}
	fmt.Fprintf(w, "  by       %s\n", d.By)
	fmt.Fprintf(w, "  at       %s\n", d.At.Format(stamp))
	fmt.Fprintf(w, "  run      %s\n", id)
}

// subject resolves what is being acted on, and computes the content version the
// act is about.
//
// The two verbs need different things here. Ratifying requires an artifact that
// is present and has binding content, because standing attaches to a content
// version and there is otherwise nothing to attach it to. Releasing must work
// on an artifact whose file is gone — that is precisely the case it exists for,
// since standing outlives the content and deleting a file is not a way to be
// rid of a promise.
func subject(kind ledger.Kind, c *corpus.Corpus, ref string, stderr io.Writer) (string, canonical.Hash, int) {
	entry, err := c.Lookup(ref)
	if err != nil {
		if kind == ledger.Ratified {
			fmt.Fprintf(stderr, "kept: %s\n", err)
			return "", canonical.Hash{}, exitUsage
		}
		// Releasing something the corpus does not hold. Its id has to be
		// written out in full: there is no artifact to resolve a bare slug
		// against, and a durable record is not the place to guess.
		id := strings.TrimSpace(ref)
		if qualifier, _ := corpus.SplitRef(id); qualifier == "" {
			fmt.Fprintf(stderr, "kept: %s, and releasing an artifact the corpus does not hold needs its full \"type:slug\"\n", err)
			return "", canonical.Hash{}, exitUsage
		}
		return id, canonical.Hash{}, exitOK
	}

	sc, _ := c.Schemas.For(string(entry.Artifact.Kind))
	hash, err := canonical.Bind(entry, sc)
	if err != nil {
		if kind == ledger.Retired {
			// The content is here and cannot be hashed. A retirement still means
			// something: it retires the artifact, not a wording.
			return entry.Artifact.ID, canonical.Hash{}, exitOK
		}
		fmt.Fprintf(stderr, "kept: %s\n", err)
		if entry.Artifact.Kind == model.KindDesire {
			fmt.Fprintf(stderr, "A desire is committed by ratifying an artifact whose own links say { target: %s, relation: %s }.\n",
				entry.Artifact.ID, register.Realizes)
		}
		return "", canonical.Hash{}, exitUsage
	}
	return entry.Artifact.ID, hash, exitOK
}

// wanted says whether the act would change anything, and refuses the ones that
// would not. Recording only what has already happened cuts both ways: a
// ratification of standing already in force is a permanent line describing an
// act with no effect, and a reader a year later cannot tell it from one that
// mattered.
//
// A retirement changes something for an artifact with standing to end and for a
// desire that is not already retired. A document or a fact has neither standing
// nor a state a retirement moves, so retiring one is refused.
func wanted(kind ledger.Kind, id string, c *corpus.Corpus, acts []ledger.Act, stdout, stderr io.Writer) (bool, int) {
	var now register.Standing
	for _, s := range register.Derive(c, acts) {
		if s.Artifact == id {
			now = s
			break
		}
	}

	if kind == ledger.Ratified {
		if now.State == register.Ratified {
			fmt.Fprintf(stdout, "%s is already ratified at this content version, by %s on %s.\nNothing recorded.\n",
				id, now.Granted.By, now.Granted.At.Format(day))
			return false, exitOK
		}
		return true, exitOK
	}

	// A desire has no standing to end, and retiring one is still an act: it
	// declines the want, and the file stays as the record of what was declined.
	// It changes what is derived in every state but one, including a committed
	// desire — retire is the way out and is never refused where it moves
	// anything. See docs/decision/0083.
	if now.Desire != nil {
		if now.Desire.State == register.DesireRetired {
			fmt.Fprintf(stdout, "%s was already retired, by %s on %s.\nNothing recorded.\n",
				id, now.Granted.By, now.Granted.At.Format(day))
			return false, exitOK
		}
		return true, exitOK
	}

	switch now.State {
	case register.Ratified, register.Absent:
		return true, exitOK
	case register.Retired:
		fmt.Fprintf(stdout, "%s was already retired, by %s on %s.\nNothing recorded.\n",
			id, now.Granted.By, now.Granted.At.Format(day))
		return false, exitOK
	case register.Unratified:
		if !now.Granted.At.IsZero() {
			// Ratified once and edited since. Retiring is still the right act:
			// what is being taken back is the standing at the earlier wording.
			return true, exitOK
		}
	}
	fmt.Fprintf(stderr, "kept: %s has no standing to retire\n", id)
	return false, exitUsage
}

// whoami names the person performing the act. It reads the process's own user
// and never shells out to git: nothing external runs without a deadline
// (docs/decision/0026), and the config that would carry one is not read yet. A
// richer and verifiable identity is what signing brings, and docs/decision/0024
// defers that until there is a second principal.
// operator answers who is performing an act, and refuses rather than guessing.
//
// A ratification names the person who granted standing, in a record that is
// append-only and tracked, so a value nobody stated cannot be taken back. An
// operating system's username is not an identity — it is machine-local, and two
// people called `paul` on two machines are one string in a cloned ledger — and
// "unknown" is worse, because it names nobody on the one act the model reserves
// to a person. Both were defaults here and both are gone. See
// docs/decision/0048.
func operator(by string, stderr io.Writer) (string, int) {
	if v := strings.TrimSpace(by); v != "" {
		return v, exitOK
	}
	if v := strings.TrimSpace(os.Getenv("KEPT_BY")); v != "" {
		return v, exitOK
	}
	u, err := config.ReadUser()
	if err != nil {
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return "", exitUsage
	}
	if u.Operator != "" {
		return u.Operator, exitOK
	}

	// Where, not just what: being told to set something is only useful with the
	// path attached.
	where, err := config.UserPath()
	if err != nil {
		where = "your user config"
	}
	fmt.Fprintf(stderr, "kept: this act records who performed it, and nothing says who that is.\n"+
		"      Set it once per machine, in %s:\n"+
		"          operator: Your Name <you@example.com>\n"+
		"      or per act:   --by \"Your Name <you@example.com>\"\n"+
		"      or per shell: KEPT_BY=\"Your Name <you@example.com>\"\n"+
		"      Not in .kept/config.yaml: that file is tracked, so an identity there would name whoever committed it rather than you.\n", where)
	return "", exitUsage
}

func writeQuarantined(w io.Writer, quarantined []ledger.Quarantined) {
	if len(quarantined) == 0 {
		return
	}
	fmt.Fprintf(w, "kept: %s in the ledger could not be read, and %s set aside rather than guessed at:\n",
		count(len(quarantined), "line"), were(len(quarantined)))
	for _, q := range quarantined {
		fmt.Fprintf(w, "  %s:%d  %s\n", q.Stream, q.Line, q.Why)
	}
}

func were(n int) string {
	if n == 1 {
		return "was"
	}
	return "were"
}
