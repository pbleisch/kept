package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/pbleisch/kept/internal/canonical"
	"github.com/pbleisch/kept/internal/config"
	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/register"
	"github.com/pbleisch/kept/internal/run"
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

// releaseCmd retires standing. It is how a promise stops being held to;
// deleting its file is not, because an absence cannot be told apart from a bad
// merge or a corpus root pointed at the wrong place. See docs/decision/0021.
func releaseCmd(args []string, stdout, stderr io.Writer) int {
	return perform("release", ledger.Released, args, stdout, stderr)
}

func perform(verb string, kind ledger.Kind, args []string, stdout, stderr io.Writer) int {
	fs, root := flags(verb, stderr)
	by := fs.String("by", "", "who is performing this; else KEPT_BY, else operator: in .kept/config.yaml")
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

	id, hash, code := subject(kind, c, rest[0], stderr)
	if code != exitOK {
		return code
	}

	// The register is read before the act, so an act that would change
	// nothing is refused rather than recorded. Nothing is written on the way
	// past: reading standing has never written anything anywhere.
	store := ledger.NewFiles(ledgerRoot(c))
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

	who, code := operator(*by, c, stderr)
	if code != exitOK {
		return code
	}

	rec := startRun(append([]string{verb}, args...))
	l := ledger.Open(store, rec)
	d, err := record(l, kind, id, hash, who, *note)
	if err != nil {
		l.Close(run.Failed)
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return exitUsage
	}
	if err := l.Close(run.Clean); err != nil {
		fmt.Fprintf(stderr, "kept: the act was recorded and the run's diagnostics were not: %s\n", err)
	}

	writeAct(stdout, d, rec.ID)
	return exitOK
}

func record(l *ledger.Ledger, kind ledger.Kind, id string, hash canonical.Hash, by, note string) (ledger.Act, error) {
	if kind == ledger.Ratified {
		return l.Ratify(id, hash.String(), hash.Norm, by, note)
	}
	return l.Release(id, hash.String(), hash.Norm, by, note)
}

func writeAct(w io.Writer, d ledger.Act, id run.ID) {
	fmt.Fprintf(w, "%s %s\n", d.Kind, d.Artifact)
	if d.Hash != "" {
		fmt.Fprintf(w, "  content  %s  (norm:%d)\n", d.Hash, d.Norm)
	}
	fmt.Fprintf(w, "  by       %s\n", d.By)
	fmt.Fprintf(w, "  at       %s\n", d.At.Format(stamp))
	fmt.Fprintf(w, "  run      %s\n", id)

	if d.Kind == ledger.Ratified {
		fmt.Fprint(w, "\nEdit the artifact and this stops applying. Nothing has to notice:\n"+
			"the content hashes to something else, and the record names this one.\n")
	}
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
		if kind == ledger.Released {
			// The content is here and cannot be hashed. A release still means
			// something: it retires the artifact, not a wording.
			return entry.Artifact.ID, canonical.Hash{}, exitOK
		}
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return "", canonical.Hash{}, exitUsage
	}
	return entry.Artifact.ID, hash, exitOK
}

// wanted says whether the act would change anything, and refuses the ones that
// would not. Recording only what has already happened cuts both ways: a
// ratification of standing already in force is a permanent line describing an
// act with no effect, and a reader a year later cannot tell it from one that
// mattered.
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

	switch now.State {
	case register.Ratified, register.Absent:
		return true, exitOK
	case register.Released:
		fmt.Fprintf(stdout, "%s was already released, by %s on %s.\nNothing recorded.\n",
			id, now.Granted.By, now.Granted.At.Format(day))
		return false, exitOK
	case register.Unratified:
		if !now.Granted.At.IsZero() {
			// Ratified once and edited since. Releasing is still the right act:
			// what is being taken back is the standing at the earlier wording.
			return true, exitOK
		}
	}
	fmt.Fprintf(stderr, "kept: %s has no standing to retire, so there is nothing for release to do\n", id)
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
func operator(by string, c *corpus.Corpus, stderr io.Writer) (string, int) {
	if v := strings.TrimSpace(by); v != "" {
		return v, exitOK
	}
	if v := strings.TrimSpace(os.Getenv("KEPT_BY")); v != "" {
		return v, exitOK
	}
	cfg, err := config.Read(ledgerRoot(c))
	if err != nil {
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return "", exitUsage
	}
	if cfg.Operator != "" {
		return cfg.Operator, exitOK
	}
	fmt.Fprint(stderr, "kept: this act records who performed it, and nothing here says who that is.\n"+
		"      Standing is created only by a person, so kept will not guess one.\n"+
		"      Set it once:   operator: Your Name <you@example.com>   in .kept/config.yaml\n"+
		"      or per act:    --by \"Your Name <you@example.com>\"\n"+
		"      or per shell:  KEPT_BY=\"Your Name <you@example.com>\"\n")
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
