package gather

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/ledger"
)

// argRoot is the corpus root, relative to the workspace.
//
// It is an argument rather than a fact about where kept is running, because
// this gatherer answers about *a* corpus and not about *this* one: a promise in
// a repository can be about the manuscript beside it. Absent means the corpus
// the workspace's own .kept/config.yaml names, which is what every other verb
// reads.
const argRoot = "root"

// corpusGatherer observes kept reading a corpus root: how many artifacts it
// holds, what the load found wrong, and which links point at nothing.
//
// **It reads the corpus and never the ledger, and that is an invariant rather
// than a preference.** `.kept/` is excluded from `World.Tree`
// (docs/decision/0033), so a gatherer that counted ratifications would produce
// a new observation every time anything was ratified while the world digest
// stood still — data moving without the world moving, which docs/decision/0034
// forbids and which would break dedup silently.
//
// docs/decision/0046 turns that from *forbidden* into *permitted on
// declaration*: a gatherer may declare a read of the acts a person performs,
// and the declaration is what makes the address name what the data depends on.
// This one declares nothing, so the original refusal stands for it, on the
// original grounds. See docs/decision/0049.
type corpusGatherer struct{}

func (corpusGatherer) Name() string { return "corpus@1" }

// Reads nothing. See the type comment: this is the refusal, not an omission.
func (corpusGatherer) Reads() []ledger.Kind { return nil }

// The fields corpus declares. A promise may read these and no others.
var corpusFields = []string{
	// ok is whether the corpus loaded and holds at least one artifact. A
	// corpus that will not load at all is a **gather failure** and produces no
	// observation, so this is never false for that reason; it is false for a
	// root that loaded and holds nothing, which is a measurement.
	"ok",
	// artifacts is how many the corpus holds. It is a count and not the list,
	// because the list is the corpus and an observation is not a copy of the
	// thing it measured.
	"artifacts",
	// findings is one record per diagnostic the load reported: {artifact,
	// kind, detail}. artifact is the id where one could be read and the path
	// otherwise, because a file with no id is exactly the case that needs a
	// path.
	"findings",
	// broken_links is one record per edge with nothing at the other end:
	// {from, target, relation}. It is separate from findings because "every
	// policy cites a decision that exists" is a promise about edges, and
	// folding it out of a mixed list would mean matching on a kind string.
	"broken_links",
}

func (corpusGatherer) Declares(args map[string]any) ([]string, error) {
	if err := refuseUnknownArgs(args, argScope, argRoot); err != nil {
		return nil, err
	}
	if _, err := corpusRoot(args); err != nil {
		return nil, err
	}
	return corpusFields, nil
}

// corpusRoot reads the root a promise named. Empty means the one the
// workspace's config names, resolved at gather time because that is when the
// workspace is known.
func corpusRoot(args map[string]any) (string, error) {
	switch t := args[argRoot].(type) {
	case nil:
		return "", nil
	case string:
		if t == "" {
			return "", fmt.Errorf("%s: is empty, so nothing says which corpus to read", argRoot)
		}
		if filepath.IsAbs(filepath.FromSlash(t)) {
			// An absolute path would make the observation depend on where the
			// tree happens to be checked out, and two machines measuring one
			// world would disagree. It is also outside the world, which is
			// what a scope is a digest over.
			return "", fmt.Errorf("%s: %s is absolute, and a corpus root is named relative to the workspace", argRoot, t)
		}
		return t, nil
	}
	return "", fmt.Errorf("%s: is not a path", argRoot)
}

func (corpusGatherer) Gather(_ context.Context, req Request) (Fields, error) {
	named, err := corpusRoot(req.Args)
	if err != nil {
		return nil, err
	}

	root := ""
	if named != "" {
		root = filepath.Join(req.Root, filepath.FromSlash(named))
	} else {
		found, _, err := corpus.FindRoot(req.Root)
		if err != nil {
			// Nothing says where the corpus is. That is a failure to measure
			// and not a measurement of an empty corpus.
			return nil, fmt.Errorf("no corpus to read: %w", err)
		}
		root = found
	}

	// Loaded without a catalogue on purpose. What a gather entry names is
	// checked against the gatherers the *reading* kept ships, and answering
	// that here would make this gatherer's output depend on its own build
	// rather than on the corpus it read — and it would let a promise about a
	// corpus report on the instrument rather than on the artifacts.
	c, err := corpus.Load(root, req.Root, corpus.Options{})
	if err != nil {
		// A corpus that will not load is a machine that could not measure. The
		// verdict is unknown, never not-kept: two artifacts claiming one id is
		// a thing a person fixes, and reporting it as a broken promise would
		// say the corpus is wrong about itself.
		return nil, err
	}
	return foldCorpus(c), nil
}

// foldCorpus turns a loaded corpus into the declared fields.
//
// Everything is sorted, for the reason every fold here is: an observation is
// addressed by its data, and an order that came from a directory walk would
// mint a fresh record for an unchanging world. See docs/decision/0034.
func foldCorpus(c *corpus.Corpus) Fields {
	problems := append([]corpus.Problem(nil), c.Problems...)
	sort.Slice(problems, func(i, j int) bool {
		a, b := problems[i], problems[j]
		if named(a) != named(b) {
			return named(a) < named(b)
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.Detail < b.Detail
	})
	findings := make([]any, 0, len(problems))
	for _, p := range problems {
		findings = append(findings, map[string]any{
			"artifact": named(p),
			"kind":     string(p.Kind),
			"detail":   p.Detail,
		})
	}

	broken := c.Broken()
	sort.Slice(broken, func(i, j int) bool {
		a, b := broken[i], broken[j]
		if a.From != b.From {
			return a.From < b.From
		}
		if a.Link.Target != b.Link.Target {
			return a.Link.Target < b.Link.Target
		}
		return a.Link.Relation < b.Link.Relation
	})
	links := make([]any, 0, len(broken))
	for _, l := range broken {
		links = append(links, map[string]any{
			"from":     l.From,
			"target":   l.Link.Target,
			"relation": l.Link.Relation,
		})
	}

	return Fields{
		"ok":           len(c.All()) > 0,
		"artifacts":    len(c.All()),
		"findings":     findings,
		"broken_links": links,
	}
}

// named is what a finding is about: the artifact's id where one could be read,
// and the path otherwise. A file with no id is exactly the case that has no id
// to be named by.
func named(p corpus.Problem) string {
	if p.ID != "" {
		return p.ID
	}
	return p.Path
}
