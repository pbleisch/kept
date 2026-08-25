package main

import (
	"flag"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"

	"github.com/pbleisch/kept/internal/corpus"
)

// A filter narrows a listing verb: which artifacts it is about, and which of
// the states it reports are worth printing.
//
// There are two kinds of narrowing here and they are not the same kind of
// thing, which is the whole of why an empty result has two meanings.
//
// `--kind` and `--id` are **selectors**. They name a population out of the
// corpus, and they are properties an artifact has whether or not anything has
// ever been derived about it. A selector that names nothing means the question
// was never put — the same distinction the model already makes one level down,
// where *nothing has answered* is not an answer of `unknown`.
//
// `--state` is a **predicate**. It is applied to what the verb derived about
// the population, and a predicate that admits nothing is an answer: the things
// you asked about are none of them in that state. A script asking "is anything
// unratified" wants exit 0 for that, and would be badly served by a code saying
// its question could not be answered.
//
// So: a selector matching nothing is exit 3 and says so; a predicate admitting
// nothing is the verb's ordinary exit over an empty set, and also says so,
// because an empty table looks like success and kept has this lesson written
// down already — a suite of no packages is not `ok`, and a pattern matching
// nothing is the ordinary way that happens.
//
// The state vocabulary is the verb's own. `status` reports standing and `drift`
// reports truth, they share no word by accident, and `--state` typed on the
// wrong one is refused by name rather than silently matching nothing. `list`
// reports no state at all and refuses the flag outright. That refusal is what
// keeps one flag name from meaning two things: a person who types the wrong
// vocabulary is told which vocabulary the verb speaks, which is more useful
// than a flag that quietly accepts both sets and answers about neither.
type filter struct {
	verb   string
	kinds  values
	ids    values
	states values

	// vocabulary is the closed set --state admits on this verb, in the words
	// the verb prints. Empty means the verb reports no state.
	vocabulary []string
	// reports says what this verb's state column is, for the refusal that
	// names the other verb.
	reports string
}

// values is a flag that may be given more than once. Repetition is `or` and
// the flags together are `and`, so `--kind promise --kind policy --state
// ratified` is *the ratified promises and policies*, which is the question a
// person actually has.
type values struct{ list []string }

func (v *values) String() string { return strings.Join(v.list, ",") }

func (v *values) Set(s string) error {
	v.list = append(v.list, s)
	return nil
}

func (v *values) any() bool { return len(v.list) > 0 }

// narrow registers the filtering flags on a verb.
//
// --state is registered even where the verb reports no state, so that typing it
// is refused with a sentence rather than with the flag package's "flag provided
// but not defined". A refusal that names the verb that does answer the question
// is the difference between an error and an answer.
func narrow(fs *flag.FlagSet, verb, reports string, vocabulary []string) *filter {
	f := &filter{verb: verb, reports: reports, vocabulary: vocabulary}
	fs.Var(&f.kinds, "kind", "only artifacts of this type; the types are the ones docs/_schema declares. Repeatable")
	fs.Var(&f.ids, "id", `only artifacts whose whole printed id — "type:slug" — matches this glob. Quote it, or the shell expands it. Repeatable`)
	if len(vocabulary) == 0 {
		fs.Var(&f.states, "state", "not a flag of this verb: it reports no state")
		return f
	}
	fs.Var(&f.states, "state", fmt.Sprintf("only artifacts whose %s is this: %s. Repeatable",
		reports, strings.Join(vocabulary, ", ")))
	return f
}

// check refuses a filter nothing could match by name rather than by an empty
// table. An unknown kind, an unparseable glob and a word from the wrong
// vocabulary are all authoring errors in what the operator typed, and 0027
// makes those exit 2.
func (f *filter) check(c *corpus.Corpus, stderr io.Writer) int {
	kinds := knownKinds(c)
	for _, k := range f.kinds.list {
		if !contains(kinds, k) {
			fmt.Fprintf(stderr, "kept: there is no %q type here; %s takes: %s\n",
				k, f.verb, strings.Join(kinds, ", "))
			return exitUsage
		}
	}
	for _, pattern := range f.ids.list {
		if _, err := path.Match(pattern, ""); err != nil {
			fmt.Fprintf(stderr, "kept: --id %q is not a glob: %s\n", pattern, err)
			return exitUsage
		}
	}
	if !f.states.any() {
		return exitOK
	}
	if len(f.vocabulary) == 0 {
		fmt.Fprintf(stderr, "kept: %s reports no state, so it takes no --state. "+
			"kept status --state %s is that question\n", f.verb, f.states.list[0])
		return exitUsage
	}
	for _, s := range f.states.list {
		if !contains(f.vocabulary, spelled(s)) {
			fmt.Fprintf(stderr, "kept: %s reports %s, and %q is not one: %s takes %s\n",
				f.verb, f.reports, s, f.verb, strings.Join(f.vocabulary, ", "))
			return exitUsage
		}
	}
	return exitOK
}

// knownKinds is what --kind admits: every type docs/_schema declares, and every
// type the corpus actually holds.
//
// The schema directory is the list, so a kind added tomorrow is filterable with
// no code change — the rule the whole of `kept new` was built on. The union
// with what is present is not a hedge: a type directory with no schema is a
// reported problem and its artifacts are still loaded and still printed, so
// refusing to filter for one would be refusing to narrow to the rows a reader
// most wants to look at.
func knownKinds(c *corpus.Corpus) []string {
	seen := map[string]bool{}
	var out []string
	add := func(k string) {
		if k != "" && !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	for _, t := range c.Schemas.Types() {
		add(t)
	}
	for _, e := range c.All() {
		kind, _ := corpus.SplitRef(e.Artifact.ID)
		add(kind)
	}
	sort.Strings(out)
	return out
}

// selecting reports whether a population was named. A verb with no selector is
// about the whole corpus, and an empty corpus is not an unanswered question.
func (f *filter) selecting() bool { return f.kinds.any() || f.ids.any() }

func (f *filter) any() bool { return f.selecting() || f.states.any() }

// selects reports whether an artifact is in the population.
//
// The glob is matched against the whole printed id, `type:slug`, because that
// is the string every verb prints and the one spelling `show`, `why`, `ratify`
// and `repair` all take. A pattern that matched the slug alone would answer
// differently from what a reader is looking at, which is a bug they would file
// as confusion.
func (f *filter) selects(id string) bool {
	if f.kinds.any() {
		kind, _ := corpus.SplitRef(id)
		if !contains(f.kinds.list, kind) {
			return false
		}
	}
	if !f.ids.any() {
		return true
	}
	for _, pattern := range f.ids.list {
		if ok, err := path.Match(pattern, id); err == nil && ok {
			return true
		}
	}
	return false
}

// admits reports whether a state the verb prints is one that was asked for.
// The words are the ones the verb prints; a space in one is written as a hyphen
// so that it needs no quoting, and both spellings are taken.
func (f *filter) admits(states ...string) bool {
	if !f.states.any() {
		return true
	}
	for _, want := range f.states.list {
		for _, got := range states {
			if spelled(want) == spelled(got) {
				return true
			}
		}
	}
	return false
}

// spelled is a state word as this flag compares them: lowercased, and with the
// space in "never ratified" written either way.
func spelled(s string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(s)), " ", "-")
}

// typed is the narrowing as the operator wrote it, for the line that says a
// table is not the whole corpus.
func (f *filter) typed() string {
	out := f.selectors()
	for _, s := range f.states.list {
		if out != "" {
			out += " "
		}
		out += "--state " + s
	}
	return out
}

// selectors is the population half of what was typed, on its own, for the
// message that has already said something about the states.
func (f *filter) selectors() string {
	var out []string
	for _, k := range f.kinds.list {
		out = append(out, "--kind "+k)
	}
	for _, id := range f.ids.list {
		out = append(out, fmt.Sprintf("--id %q", id))
	}
	return strings.Join(out, " ")
}

// writeNarrowing says that what was printed is not the corpus.
//
// An operator who filtered knows they filtered; an operator reading the
// captured output of somebody else's command does not, and a table with no
// footer is indistinguishable from the whole of a small corpus. It is one line
// and it is only ever printed when a flag was given, so the unfiltered
// rendering is untouched.
func (f *filter) writeNarrowing(w io.Writer, shown, total int) {
	if !f.any() {
		return
	}
	fmt.Fprintf(w, "\nnarrowed to %d of %s by %s\n", shown, count(total, "artifact"), f.typed())
}

// writeUnselected says a selector named nothing at all.
//
// This is the case that must not render as an empty table, and kept has the
// lesson written down in internal/gather/tests.go: a suite of no packages is
// not `ok`, because nothing was demonstrated, and a pattern matching nothing is
// the ordinary way that happens. Here it is exit 3 — the question was not put —
// and the hint below is the difference between a glob a person can fix and one
// they will retype identically.
func (f *filter) writeUnselected(w io.Writer, c *corpus.Corpus, total int) {
	fmt.Fprintf(w, "nothing matches %s, out of %s\n", f.typed(), count(total, "artifact"))
	for _, line := range f.why(c) {
		fmt.Fprintf(w, "%s%s\n", pad(hang), line)
	}
}

// why is what to try instead, and it is only ever said where kept can be sure
// of it.
func (f *filter) why(c *corpus.Corpus) []string {
	var out []string
	for _, k := range f.kinds.list {
		if !f.holds(c, k) {
			out = append(out, fmt.Sprintf("docs/_schema declares %s and the corpus holds no artifact of that kind", k))
		}
	}
	for _, pattern := range f.ids.list {
		if f.matchesASlug(c, pattern) {
			out = append(out, fmt.Sprintf("an id is printed \"type:slug\", and %q matches a slug — try %q", pattern, "*:"+pattern))
		}
	}
	return out
}

func (f *filter) holds(c *corpus.Corpus, kind string) bool {
	for _, e := range c.All() {
		if k, _ := corpus.SplitRef(e.Artifact.ID); k == kind {
			return true
		}
	}
	return false
}

func (f *filter) matchesASlug(c *corpus.Corpus, pattern string) bool {
	for _, e := range c.All() {
		_, slug := corpus.SplitRef(e.Artifact.ID)
		if ok, err := path.Match(pattern, slug); err == nil && ok {
			return true
		}
	}
	return false
}

// writeUnadmitted says the population exists and none of it is in the state
// that was asked for. That is an answer rather than an unanswered question, so
// it is not an exit code — but it is still said, because an empty table reads
// as success and this one is a finding-shaped absence.
func (f *filter) writeUnadmitted(w io.Writer, population int) {
	matching := ""
	if f.selecting() {
		matching = " matching " + f.selectors()
	}
	fmt.Fprintf(w, "%s%s, and none of them is %s\n",
		count(population, "artifact"), matching, strings.Join(f.states.list, " or "))
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
