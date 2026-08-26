package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/gather"
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/register"
)

// doctorCmd collects the identity checks the other verbs already run, and
// offers to write the one thing an adopted file is missing.
//
// Identity is most of the remit: what names an artifact, and whether everything
// the ledger names is here. Schema violations, broken links and predicates that
// will not parse are kept list's, because they are about what an artifact says
// and that is what a person wrote. The boundary is what makes --fix safe to
// have at all. See docs/decision/0042.
//
// The findings that are not about identity are an instrument nobody checked, an
// origin whose fetch writes the ledger ref itself, an origin whose fetch
// updates no branch at all, a ledger that has drifted from what the last fetch
// brought, a ledger holding acts origin did not have when this repository last
// heard from it, and a ledger ref git keeps no reflog for.
// docs/decision/0050 is what admits them and where the line is: **doctor
// reports what is well-formed and unwise, and kept list reports what is
// wrong.** Nothing refuses any of the six, nothing is blocked by them, and
// none has a reader anywhere else once a workspace is adopted. --fix is
// untouched by all of them: it writes identity, and a git config is not
// identity — kept init is what writes both git settings, and it says so when it
// does. See docs/decision/0016, docs/decision/0054 and docs/decision/0062.
func doctorCmd(args []string, stdout, stderr io.Writer) int {
	fs, root := flags("doctor", stderr)
	fix := fs.Bool("fix", false, "write an id an adopted file is missing, and renumber a contested number where that is free")
	rest, err := parse(fs, args)
	if err != nil {
		return exitUsage
	}
	if len(rest) > 0 {
		fmt.Fprintf(stderr, "kept: doctor takes no arguments; it reads the whole corpus\n")
		return exitUsage
	}

	// A duplicate id is a load error, so it arrives here as one and open says
	// which files claim what. Nothing fixes it: choosing which of two files
	// keeps the name is a decision, and this verb does not make decisions.
	c, code := open(*root, stderr)
	if code != exitOK {
		return code
	}

	// The ledger is read before anything is written, because what it names is
	// what may not be renumbered: an id is the key every act is keyed to, so
	// moving one the record mentions leaves a grant naming an artifact that is
	// not here — in a record that is append-only. Reading is all that happens
	// here; nothing kept does about an identity writes to the ledger.
	store := ledgerStore(c)
	acts, quarantined, err := ledger.ReadActs(store)
	if err != nil {
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return exitUsage
	}
	writeQuarantined(stderr, quarantined)

	found := identityProblems(c)
	if *fix && len(found) > 0 {
		fixIdentity(c, found, named(acts), stdout)
		// Read the corpus again rather than subtracting what was written from
		// what was found. A file that has just been given an id is a new
		// artifact, and it may have brought a duplicate or a path/id
		// divergence with it — which is a finding this run should report
		// rather than one the next run discovers. A renumber is the same case:
		// it rewrites citations, and whether the corpus still resolves after
		// one is a question only a second read answers.
		if c, code = open(*root, stderr); code != exitOK {
			return code
		}
		found = identityProblems(c)
	}

	entries := register.Derive(c, acts)
	var absent []register.Standing
	for _, s := range entries {
		if s.State == register.Absent {
			absent = append(absent, s)
		}
	}

	logging, unrecorded := unlogged(c, stderr)
	apart := divergence(c, stderr)
	writeFindings(stdout, c, found, unchecked(c), absent, unaccounted(entries, c.Workspace),
		directFetch(c, stderr), branchFetch(c, stderr), apart, publishing(c, apart, stderr), logging, unrecorded, *fix)

	// 0027's table, unchanged. Absent standing is in the known-false half on
	// purpose; a filename that is not a slug is in neither half, so it is
	// reported and exits 0 the way kept list's problems do.
	if len(absent) > 0 {
		return exitDrift
	}
	if len(quarantined) > 0 {
		return exitUnknown
	}
	return exitOK
}

// identity is a problem kind doctor answers for. Everything else the corpus
// reports is about content and belongs to kept list.
//
// An id of the wrong shape and two artifacts numbered alike are here for the
// reason PathMismatch is: doctor's remit is what is wrong with the *names*, and
// both are. Neither is fixable and neither becomes so by being listed — fixIdentity
// writes an id nobody wrote and does nothing else, and renumbering a ratified
// artifact is a person's act with a re-ratification behind it (0021, 0069).
func identity(k corpus.ProblemKind) bool {
	switch k {
	case corpus.NoFrontmatter, corpus.NoID, corpus.BadID,
		corpus.PathMismatch, corpus.TypeMismatch, corpus.Unreadable,
		corpus.IDShape, corpus.DuplicateNumber:
		return true
	}
	return false
}

// directFetch reports every refspec this workspace's origin still carries whose
// destination is the ledger ref itself. A workspace that is not a repository has
// no origin and no ledger ref, which is silence rather than a finding.
//
// It costs one local git config read and never contacts a remote. A direct
// spelling makes the destination of a network fetch the durable record: forced
// it rewinds the ledger over acts nobody pushed, and unforced it refuses every
// fetch while the ledger is ahead of its remote — which is the resting state.
// See docs/decision/0062.
func directFetch(c *corpus.Corpus, stderr io.Writer) []string {
	direct, err := ledger.DirectFetch(c.Workspace)
	if err != nil {
		if !errors.Is(err, ledger.ErrNotRepository) {
			fmt.Fprintf(stderr, "kept: read remote.origin.fetch: %s\n", err)
		}
		return nil
	}
	return direct
}

// branchFetch reports whether an ordinary git fetch here updates any
// remote-tracking branch. A workspace that is not a repository, and one whose
// origin has no URL, are both silence rather than a finding: there is nothing
// to track.
//
// It is a finding under docs/decision/0050 for the reason the direct spelling
// beside it is. Nothing refuses it — git accepts the config, kept accepts it,
// and kept land refreshes its own target before reading it. Nothing else in
// kept reports it. And what kept does about it is bounded the way the other two
// git settings are: kept init writes the line, and doctor is what says the line
// is missing in a workspace nobody has re-initialized.
//
// It costs one local git config read and never contacts a remote.
func branchFetch(c *corpus.Corpus, stderr io.Writer) ledger.BranchTracking {
	branches, err := ledger.BranchFetch(c.Workspace)
	if err != nil {
		if !errors.Is(err, ledger.ErrNotRepository) {
			fmt.Fprintf(stderr, "kept: read remote.origin.fetch: %s\n", err)
		}
		return ledger.NoRemote
	}
	return branches
}

// divergence reports what the ledger ref and the upstream ref say about each
// other. It is one local rev-list and never contacts a remote.
//
// docs/decision/0016 said this could not be answered — "the only way to know is
// to ask the remote" — and that was true of a refspec that wrote the remote's
// value straight into the local ref, because nothing was left to compare
// against. With both values in one repository it is arithmetic.
//
// It is a finding under docs/decision/0050 for the reason the two beside it are:
// nothing refuses a ledger that is behind or forked, and nothing else in kept
// reports it. What kept does about it is bounded and doctor is outside that
// bound — the verbs that *write* fast-forward, and doctor reads, so a checkout
// that is only ever read stays behind forever while every answer it gives comes
// from the older record. See docs/decision/0062.
func divergence(c *corpus.Corpus, stderr io.Writer) ledger.Divergence {
	d, err := ledger.Compare(ledgerStore(c))
	if err != nil {
		fmt.Fprintf(stderr, "kept: compare %s against %s: %s\n", ledger.Ref, ledger.UpstreamRef, err)
		return ledger.Divergence{}
	}
	return d
}

// publishState is what this repository's own refs can say about whether origin
// holds the acts the ledger holds. Every value is derived from two local refs
// and one local config read; nothing here contacts a remote.
type publishState string

const (
	// publishSilent: there is nothing to say. No origin to publish to, no
	// ledger to publish, a ledger origin already had when this repository last
	// heard from it, or a fork — which is the finding beside this one, and
	// whose answer is a union commit rather than a push.
	publishSilent publishState = ""
	// publishAhead: the ledger holds commits the upstream value cannot reach,
	// and none the other way. One fast-forward push publishes them.
	publishAhead publishState = "ahead"
	// publishUnfetched: there is an origin and a ledger, and no upstream value
	// to compare against. This is *nobody has asked* rather than *origin does
	// not have it* — the same distinction [ledger.Divergence.Known] draws, and
	// the same one *nothing has answered* draws about a verdict.
	publishUnfetched publishState = "unfetched"
)

// publishing reports whether this repository is holding acts it has never
// published. It is one local config read on top of the comparison divergence
// already made, and it never contacts a remote.
//
// **git push does not carry a side ref.** So a person ratifies, git status is
// clean, and refs/kept/ledger never leaves the machine — measured on this
// repository on 2026-08-25, three commits of it, silently, for a day. Nothing
// said so, because a ledger ahead of its remote is this system's resting state
// (docs/decision/0062) and every reader treated it as one.
//
// **What it may claim is bounded by what a local ref can support.**
// [ledger.UpstreamRef] is as old as whatever last wrote it, and that is a fetch
// or a push at a moment kept did not choose, so the finding says *origin did
// not have these when this repository last heard from it* and never *origin
// does not have these*. The second needs the network, and kept has never
// fetched and does not start here.
//
// **The date of that last hearing is not knowable, and the finding therefore
// prints none.** Measured: git writes a reflog entry for the upstream ref only
// when the ref *moves*, so a fetch that brings nothing new leaves no trace, and
// a ref that has not moved in a week may have been fetched a minute ago. And it
// is logged at all only where core.logAllRefUpdates is `always` — refs/kept/ is
// outside the prefixes git logs by default, which is the finding two below this
// one. A printed date would read as *when you last fetched*, be wrong in the
// common case, and be absent in a default repository.
//
// It passes docs/decision/0050's three conditions. Nothing refuses an
// unpublished ledger: every verb keeps answering, and must, because publishing
// is not a precondition of recording. Nothing else in kept reports it — kept
// ledger sync is the inbound half and says nothing about the outbound one. And
// kept does nothing about it: a push would be the first network call kept ever
// makes, and the boundary sync's own help states is worth more than the
// convenience, so the finding hands over the command instead. That is the shape
// kept new prints the ratify line in rather than ratifying, and a contested
// number hands over kept doctor --fix in.
func publishing(c *corpus.Corpus, apart ledger.Divergence, stderr io.Writer) publishState {
	url, err := ledger.Origin(c.Workspace)
	if err != nil {
		if !errors.Is(err, ledger.ErrNotRepository) {
			fmt.Fprintf(stderr, "kept: read remote.origin.url: %s\n", err)
		}
		return publishSilent
	}
	switch {
	case url == "":
		// No origin is nowhere to publish to. A workspace initialized before it
		// had a remote is in this state, and so is every manuscript.
		return publishSilent
	case apart.Ledger == "":
		// Nothing has ever been recorded here, so there is nothing to publish.
		return publishSilent
	case apart.Diverged():
		// A fork is reported once, by the finding that owns it: origin would
		// refuse this push anyway, and saying *publish* beside *union these by
		// hand* is two instructions where only one can be followed.
		return publishSilent
	case !apart.Known():
		return publishUnfetched
	case apart.Unpublished():
		return publishAhead
	}
	return publishSilent
}

// unlogged reports whether git will keep no reflog for this workspace's ledger
// ref. A workspace that is not a repository has no ref and no setting, which is
// silence rather than a finding.
//
// It reads one local git setting and asks nothing of the ref's history. What
// makes this a finding is what happens next rather than what has happened: a
// repository git does not log is one where the next rewind leaves nothing
// behind, and an empty reflog in a correctly configured repository is an
// artifact of the ledger not having moved yet. See docs/decision/0054.
// The configured value travels with the verdict because the finding prints it,
// the way the forced refspec beside it prints the refspec that is there. An
// empty value is one nobody has written.
func unlogged(c *corpus.Corpus, stderr io.Writer) (string, bool) {
	value, logged, err := ledger.Logged(c.Workspace)
	if err != nil {
		if !errors.Is(err, ledger.ErrNotRepository) {
			fmt.Fprintf(stderr, "kept: read core.logAllRefUpdates: %s\n", err)
		}
		return "", false
	}
	return value, !logged
}

// grant is one grant still in force that this tree cannot account for: the
// artifact it names, the act behind it, and what the tree could say about the
// commit that act was performed in.
type grant struct {
	Artifact string
	Grant    register.Grant
	Reach    ledger.Reach
}

// unaccounted is every grant in force whose content version this corpus does
// not hold and whose world this tree cannot vouch for.
//
// **The discriminator is reachability, not equality.** Every ratification is
// against a tree this is not the moment anything lands, so comparing worlds
// would report the whole corpus. What separates the two cases is whether the
// commit the act was performed in is one this tree can reach: reachable means
// that wording landed here once and was revised, which is the ordinary state of
// every artifact that has ever been edited, and it is silence. Unreachable means
// it landed nowhere the reader can see.
//
// Three kinds of silence, and each is a different reason.
//
// An act with no world at all names nothing — every act written before
// docs/decision/0063 is one, and *nobody recorded it* is not *it never landed*.
// An act whose world names a tree and no commit was performed where there was
// no commit to name, outside a repository or on an unborn branch, and there is
// no history for it to be missing from. And a repository that cannot answer —
// no HEAD, no git, a shallow clone — establishes nothing, which is the same
// split docs/decision/0004 makes between a failed gather and a negative answer.
//
// It is a finding under docs/decision/0050's three-part test: nothing refuses a
// grant like this, kept does nothing about it, and until now nothing reported
// it. See docs/decision/0063.
func unaccounted(entries []register.Standing, repository string) []grant {
	var candidates []grant
	commits := map[string]bool{}
	for _, s := range entries {
		for _, g := range s.Dormant {
			if !g.Where.Situated() || g.Where.World.Commit == "" {
				continue
			}
			candidates = append(candidates, grant{Artifact: s.Artifact, Grant: g})
			commits[g.Where.World.Commit] = true
		}
	}
	if len(candidates) == 0 {
		return nil
	}

	reach := ledger.Reaches(repository, sortedKeys(commits))
	var out []grant
	for _, c := range candidates {
		c.Reach = reach[c.Grant.Where.World.Commit]
		unreachable := c.Reach == ledger.Unreachable
		vouchless := c.Reach == ledger.Reachable && c.Grant.Where.Uncommitted
		if unreachable || vouchless {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Artifact != out[j].Artifact {
			return out[i].Artifact < out[j].Artifact
		}
		return out[i].Grant.At.Before(out[j].Grant.At)
	})
	return out
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// instrument is one gather entry that ran a program and never looked at
// whether it worked.
type instrument struct {
	// Artifact is the promise that wrote it, and Name is the key of its gather
	// map — which is what the predicate reads it under and is not necessarily
	// the gatherer's own name.
	Artifact string
	Name     string
	Run      string
}

// unchecked is every `command` entry in the corpus whose declared fields never
// reference `exit`.
//
// The false-green it protects against is a class bug in the escape hatch:
// docs/decision/0049 opens on a promise that reported kept because the program
// it ran exited non-zero, printed nothing, and the predicate read the empty
// output as an absence of violations. Three native gatherers fix three
// instances of that, and `command` covers every domain they do not — which a
// manuscript author will be writing promises in for years. See
// docs/decision/0050.
func unchecked(c *corpus.Corpus) []instrument {
	var out []instrument
	for _, e := range c.All() {
		for _, name := range sortedNames(e.Artifact.Gather) {
			spec := e.Artifact.Gather[name]
			if !gather.UncheckedInstrument(spec) {
				continue
			}
			line, _ := spec.Args["run"].(string)
			out = append(out, instrument{Artifact: e.Artifact.ID, Name: name, Run: line})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Artifact != out[j].Artifact {
			return out[i].Artifact < out[j].Artifact
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// sortedNames is a map's keys in order. It is generic because two callers want
// it over different values and the answer is about the keys either way.
func sortedNames[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for name := range m {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func identityProblems(c *corpus.Corpus) []corpus.Problem {
	var out []corpus.Problem
	for _, p := range c.Problems {
		if identity(p.Kind) {
			out = append(out, p)
		}
	}
	return out
}

// fixIdentity writes the identity that is missing and returns what is left.
//
// It is not a repair: a repair is an agent in a box closing a promise's gap, and
// this writes frontmatter kept can derive with nobody deciding anything.
//
// One block per file and not a byte more: the file's own line endings are used
// for the lines inserted, a byte-order mark stays in front of them, and nothing
// already in the file is parsed, reordered or re-emitted. See 0029 and 0042.
func fixIdentity(c *corpus.Corpus, found []corpus.Problem, held map[string]bool, stdout io.Writer) {
	var written, refused int

	for _, p := range found {
		if p.Kind != corpus.NoFrontmatter && p.Kind != corpus.NoID {
			continue
		}
		path := filepath.Join(c.Root, filepath.FromSlash(p.Path))
		wrote, why := adopt(path, p)
		if why != "" {
			if refused == 0 {
				fmt.Fprint(stdout, "left alone:\n")
			}
			refused++
			fmt.Fprintf(stdout, "  %s\n      %s\n", p.Path, why)
			continue
		}
		if written == 0 {
			fmt.Fprint(stdout, "wrote:\n")
		}
		written++
		fmt.Fprintf(stdout, "  %s\n      %s\n", p.Path, wrote)
	}

	if written > 0 {
		fmt.Fprintf(stdout, "\n%s. Read the diff: an id is authored, and this one came from the filename.\n",
			count(written, "file"))
	}

	fixNumbers(c, held, stdout)
}

// fixNumbers resolves the contested numbers that are free to resolve, and says
// why it left the rest alone.
//
// It is the second thing --fix writes and the first that touches a file other
// than the one whose identity is wrong. What keeps it inside *identity and only
// identity* is that every byte it changes elsewhere is an occurrence of the id
// it just moved: a citation is a name rather than a sentence, and after the
// rewrite no file says anything it did not say before. Every changed file and
// what changed in it is printed, which is the check on that claim.
//
// See docs/decision/0069 and renumber.go, which holds the rules.
func fixNumbers(c *corpus.Corpus, held map[string]bool, stdout io.Writer) {
	moves, refused := planRenumbering(c, held)
	if len(moves) == 0 && len(refused) == 0 {
		return
	}

	var done []renumbering
	for _, m := range moves {
		if err := apply(c.Root, m); err != nil {
			refused = append(refused, refusal{
				number: m.from,
				why:    fmt.Sprintf("it could not be renumbered: %s", err),
			})
			continue
		}
		done = append(done, m)
	}
	writeRenumbering(stdout, done, refused)
	if len(done) > 0 {
		fmt.Fprint(stdout, "\nRead the diff: the id moved, and every citation of it moved with it.\n")
	}
}

// named is every artifact id the ledger mentions, whatever it says about it.
//
// The question a renumber has to ask is not whether standing is in force — a
// release names an id too, and a ratification at a content version this is not
// is still a grant that would be left pointing at nothing. It is whether the
// durable record mentions this id at all, because that record is append-only
// and nothing can go back and say the artifact is now called something else.
func named(acts []ledger.Act) map[string]bool {
	out := make(map[string]bool, len(acts))
	for _, a := range acts {
		out[a.Artifact] = true
	}
	return out
}

// adopt writes one file's identity. It returns what it did, or why it did not.
func adopt(path string, p corpus.Problem) (did, why string) {
	src, err := os.ReadFile(path)
	if err != nil {
		return "", err.Error()
	}

	kind, base := filepath.Base(filepath.Dir(path)), strings.TrimSuffix(filepath.Base(path), ".md")
	if bad := corpus.BadSlug(base); bad != "" {
		return "", fmt.Sprintf("the filename %q %s, so it cannot be the id; rename the file or write an id by hand", base, bad)
	}

	bom, body := splitBOM(src)
	eol := "\n"
	if bytes.Contains(src, []byte("\r\n")) {
		eol = "\r\n"
	}

	var insert []byte
	if p.Kind == corpus.NoID {
		// 0029's sanction word for word: one line into a block that exists.
		// Nothing else is added, because a block somebody wrote may be missing
		// a key on purpose.
		out, ok := insertID(body, base, eol)
		if !ok {
			return "", "the frontmatter block could not be read, and inserting a line into a block nothing parsed is guessing"
		}
		insert = out
		did = "id: " + base
	} else {
		block := []string{"---", "id: " + base, "type: " + kind}
		if title := firstHeading(string(body)); title != "" {
			block = append(block, "title: "+yamlScalar(title))
		}
		block = append(block, "---", "")
		insert = append([]byte(strings.Join(block, eol)+eol), body...)
		did = strings.Join(block[1:len(block)-2], ", ")
	}

	if err := os.WriteFile(path, append(bom, insert...), 0o644); err != nil {
		return "", err.Error()
	}
	return did, ""
}

// insertID puts one id line at the top of a frontmatter block that is already
// there, without reading anything else in it.
func insertID(body []byte, slug, eol string) ([]byte, bool) {
	text := string(body)
	line, rest, found := strings.Cut(text, "\n")
	if !found || strings.TrimSpace(line) != "---" {
		return nil, false
	}
	return []byte(strings.TrimRight(line, "\r") + eol + "id: " + slug + eol + rest), true
}

// splitBOM keeps a byte-order mark in front of whatever is written. It is not
// content and it is not kept's to remove.
func splitBOM(src []byte) (bom, rest []byte) {
	mark := []byte{0xEF, 0xBB, 0xBF}
	if bytes.HasPrefix(src, mark) {
		return mark, src[len(mark):]
	}
	return nil, src
}

// firstHeading is the text of the first ATX H1, which is what a title is taken
// from during adoption. A file with none gets no title, because there is
// nothing to take one from and inventing one is writing content.
func firstHeading(body string) string {
	for _, line := range strings.Split(body, "\n") {
		if rest, ok := strings.CutPrefix(line, "# "); ok {
			return strings.TrimSpace(strings.TrimRight(rest, " \t#"))
		}
	}
	return ""
}

// yamlScalar quotes a value when the plain form would not read back as itself.
// Files kept composes must be well-formed, and it composed this one, so it
// knows every key. See docs/decision/0029.
//
// "Itself" includes its type. A plain 123 or true reads back as a number or a
// boolean, and every field kept writes this way is declared a string — so a
// value that another type would claim is quoted along with the ones that would
// not parse at all.
func yamlScalar(s string) string {
	plain := s != "" &&
		!strings.ContainsAny(s[:1], "-?:,[]{}#&*!|>'\"%@`") &&
		!strings.Contains(s, ": ") &&
		!strings.Contains(s, " #") &&
		!strings.HasSuffix(s, ":") &&
		strings.TrimSpace(s) == s &&
		!readsAsAnotherType(s)
	if plain {
		return s
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

// readsAsAnotherType reports whether YAML would read a plain scalar as
// something other than a string.
func readsAsAnotherType(s string) bool {
	switch strings.ToLower(s) {
	case "true", "false", "null", "yes", "no", "on", "off", "~":
		return true
	}
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		return true
	}
	return false
}

func writeFindings(w io.Writer, c *corpus.Corpus, found []corpus.Problem, loose []instrument, absent []register.Standing, adrift []grant, direct []string, branches ledger.BranchTracking, apart ledger.Divergence, pushing publishState, logging string, unlogged, fixed bool) {
	if len(found) == 0 && len(absent) == 0 && len(loose) == 0 && len(adrift) == 0 && len(direct) == 0 && branches != ledger.BranchesUntracked && !unsynced(apart) && pushing == publishSilent && !unlogged {
		fmt.Fprintf(w, "%s, nothing wrong\n", count(len(c.All()), "artifact"))
		return
	}

	if len(found) > 0 {
		fmt.Fprintf(w, "\n%s:\n\n", count(len(found), "identity problem"))
		for _, p := range found {
			where := p.Path
			if where == "" {
				where = p.ID
			}
			fmt.Fprintf(w, "  %s\n      %s: %s\n", where, p.Kind, p.Detail)
		}
		if n := fixable(found); !fixed && n > 0 {
			fmt.Fprintf(w, "\n%s of these is an id nobody wrote; kept doctor --fix writes it.\n", count(n, "problem"))
		}
	}

	if len(loose) > 0 {
		fmt.Fprintf(w, "\n%s that ran a program and never looked at whether it worked:\n\n",
			count(len(loose), "gather entry"))
		for _, i := range loose {
			fmt.Fprintf(w, "  %s\n      %s runs %s and no declared field reads exit\n", i.Artifact, i.Name, i.Run)
		}
		fmt.Fprint(w, "\nA program that fails often prints nothing, and a predicate over nothing is\n"+
			"usually true. Write ok: \"exit == 0\" and read it, or leave it: wc -l ignores\n"+
			"exit status and so does this. Nothing here is refused.\n")
	}

	if len(direct) > 0 {
		fmt.Fprintf(w, "\norigin fetches the ledger straight into itself:\n\n  remote.origin.fetch\n      %s\n\n",
			strings.Join(direct, "\n      "))
		fmt.Fprintf(w, "That makes an ordinary git fetch write the durable record. Forced, it rewinds\n"+
			"the ledger over acts nobody pushed, with exit 0 and no output. Unforced, git\n"+
			"refuses the fetch, so every bare git fetch fails while the ledger is ahead of\n"+
			"its remote — this system's resting state. kept init replaces the line with\n\n"+
			"      %s\n\nand changes nothing else.\n",
			ledger.FetchRefspec)
	}

	if branches == ledger.BranchesUntracked {
		fmt.Fprintf(w, "\norigin fetches no branch:\n\n  remote.origin.fetch\n      %s\n\n", ledger.BranchRefspec)
		fmt.Fprint(w, "Nothing here brings refs/heads/ into refs/remotes/, so an ordinary git fetch\n"+
			"exits 0 and updates no remote-tracking branch. refs/remotes/origin/main holds\n"+
			"whatever was last pushed and drifts further from origin every day, while reading\n"+
			"exactly like a current ref.\n\n"+
			"kept land is unaffected: it states its target's refspec in full and fetches it\n"+
			"itself. What breaks is git log origin/main, git rebase origin/main, and anything\n"+
			"deciding whether a branch is behind. kept init adds the line above and removes,\n"+
			"narrows and widens nothing.\n")
	}

	writeDivergence(w, apart)
	writePublication(w, apart, pushing)

	if unlogged {
		configured := logging
		if configured == "" {
			configured = "(not set)"
		}
		fmt.Fprintf(w, "\nnothing records what the ledger ref has held:\n\n  core.logAllRefUpdates\n      %s\n\n", configured)
		fmt.Fprintf(w, "Git keeps a reflog only for refs/heads/, refs/remotes/, refs/notes/ and HEAD\n"+
			"unless this is %q; the value true does not do it. So %s\ncan be moved by a fetch, a push into this repository or a stray update-ref\n"+
			"with nothing left to say that it was. Set, kept refuses to act when the reflog\n"+
			"shows the ledger has gone backwards. kept init writes this one value.\n\n"+
			"It makes a loss survivable, not impossible: deleting the ref deletes its\n"+
			"reflog, .git/logs is an ordinary directory anyone can remove, entries expire\n"+
			"at gc.reflogExpire — 90 days by default — and a clone starts with neither the\n"+
			"setting nor the entries.\n", ledger.LogAllRefUpdates, ledger.Ref)
	}

	if len(absent) > 0 {
		fmt.Fprintf(w, "\n%s with standing and no content:\n\n", count(len(absent), "artifact"))
		for _, s := range absent {
			fmt.Fprintf(w, "  %s\n      ratified %s at %s, and no artifact claims this id — never read as a release\n",
				s.Artifact, s.Granted.At.Format(day), s.Granted.Hash.Short())
		}
		fmt.Fprint(w, "\nEither the content is gone or an id was edited by hand. Deleting a file does not\n"+
			"retire a promise; kept release does.\n")
	}

	writeGrants(w, adrift)
}

// writePublication says whether origin holds what the ledger holds, in the two
// states the local refs can speak to.
//
// **Neither of them claims live knowledge of origin.** The comparison is
// against a ref something wrote at a moment kept did not choose, so the heading
// is *when this repository last heard from it* and the closing line says the
// same thing again in the place a reader stops. No date is printed, because
// none is knowable: see [publishing] for the measurement.
//
// The command is printed verbatim rather than described. It is
// [ledger.PushCommand], so the line a person copies and the line kept names
// cannot drift, and kept does not run it.
func writePublication(w io.Writer, d ledger.Divergence, pushing publishState) {
	switch pushing {
	case publishAhead:
		fmt.Fprintf(w, "\nthe ledger holds acts origin did not have when this repository last heard\nfrom it:\n\n  %s\n      %s, %s ahead\n  %s\n      %s\n\n",
			ledger.Ref, shortCommit(d.Ledger), count(d.Ahead, "commit"),
			ledger.UpstreamRef, shortCommit(d.Upstream))
		fmt.Fprintf(w, "git push does not carry a side ref, so a ratification stays on the machine it\n"+
			"was made on however clean git status looks. Publish with\n\n      %s\n\n"+
			"unforced on purpose: origin refuses it instead of rewinding if somebody else's\n"+
			"acts got there first, and the answer to that is git fetch and kept ledger sync.\n"+
			"Nothing refuses an unpublished ledger, and kept will not push one.\n\n"+
			"This is what two local refs say and no more. %s is as old as\nwhatever last wrote it, so fetch to make the comparison current.\n",
			ledger.PushCommand, ledger.UpstreamRef)
	case publishUnfetched:
		fmt.Fprintf(w, "\nnothing here has heard from origin about the ledger:\n\n  %s\n      %s\n  %s\n      absent\n\n",
			ledger.Ref, shortCommit(d.Ledger), ledger.UpstreamRef)
		fmt.Fprintf(w, "No fetch has brought a value to compare against, so whether origin holds these\n"+
			"acts is unknown here. That is not a report that it does not. Two situations look\n"+
			"exactly like this and only a fetch separates them: nobody has fetched since this\n"+
			"workspace was configured, and origin holds no ledger at all. Run\n\n"+
			"      git fetch origin\n\nand ask again. If the upstream ref is still absent, origin has no ledger and this\n"+
			"repository holds the only copy, which\n\n      %s\n\npublishes. kept makes neither call.\n",
			ledger.PushCommand)
	}
}

// writeGrants says what this tree cannot account for about a grant still in
// force. Two findings and not one, because they send a reader to different
// places: one says the wording landed nowhere, and the other says the commit
// landed and cannot vouch for the wording.
func writeGrants(w io.Writer, adrift []grant) {
	nowhere, hashed := split(adrift)

	if len(nowhere) > 0 {
		fmt.Fprintf(w, "\n%s in force for content that landed nowhere this tree can reach:\n\n",
			count(len(nowhere), "grant"))
		for _, g := range nowhere {
			fmt.Fprintf(w, "  %s\n      ratified %s at %s, in %s, which this tree cannot reach\n",
				g.Artifact, g.Grant.At.Format(day), g.Grant.Hash.Short(), shortCommit(g.Grant.Where.World.Commit))
		}
		fmt.Fprint(w, "\nStanding attaches to a content version, not to a place, so this grant applies\n"+
			"the moment those exact bytes appear anywhere in this repository, with nobody\n"+
			"granting anything. The usual cause is a task branch that ratified and was then\n"+
			"declined or rebased away. A release empties an artifact's whole set, not one\n"+
			"wording, so there is no way to withdraw just this one: ratify a revision with\n"+
			"the merge, not with the edit. Nothing refuses this; it is a report.\n")
	}

	if len(hashed) > 0 {
		fmt.Fprintf(w, "\n%s in force for content hashed out of an uncommitted tree:\n\n",
			count(len(hashed), "grant"))
		for _, g := range hashed {
			fmt.Fprintf(w, "  %s\n      ratified %s at %s, and the workspace held changes %s did not\n",
				g.Artifact, g.Grant.At.Format(day), g.Grant.Hash.Short(), shortCommit(g.Grant.Where.World.Commit))
		}
		fmt.Fprint(w, "\nThat commit is reachable and says nothing about these bytes: they were hashed\n"+
			"out of a working tree, so they may be in no commit at all. Reachability is the\n"+
			"only evidence an act carries that content landed, and a dirty tree is where it\n"+
			"stops being evidence. Commit before you ratify and the question does not arise.\n")
	}
}

// split separates the two findings: the grants whose commit this tree cannot
// reach, and the grants whose commit it can and which were hashed beside it
// rather than out of it.
//
// A grant that is both is reported once, on the stronger of the two: a commit
// nothing can reach makes the dirtiness of that tree beside the point.
func split(adrift []grant) (nowhere, hashed []grant) {
	for _, g := range adrift {
		if g.Reach == ledger.Unreachable {
			nowhere = append(nowhere, g)
			continue
		}
		hashed = append(hashed, g)
	}
	return nowhere, hashed
}

func fixable(found []corpus.Problem) int {
	n := 0
	for _, p := range found {
		if p.Kind == corpus.NoFrontmatter || p.Kind == corpus.NoID {
			n++
		}
	}
	return n
}

// unsynced is whether the divergence is worth a finding at all. A repository
// nobody has fetched into has no upstream value, and a ledger holding
// everything the last fetch brought is the ordinary case.
func unsynced(d ledger.Divergence) bool { return d.Known() && (d.Diverged() || d.Advanceable()) }

// writeDivergence says how the ledger and the last fetch's value stand, in the
// two states that are worth saying anything about.
//
// They get different text because they are different situations and only one of
// them has an answer. Behind is arithmetic with a verb attached; forked is two
// clones that both appended, which nothing here will resolve — see
// docs/decision/0062 on why kept fast-forwards and does not merge.
func writeDivergence(w io.Writer, d ledger.Divergence) {
	switch {
	case !unsynced(d):
		return
	case d.Diverged():
		fmt.Fprintf(w, "\nthe ledger and what origin last held have both moved:\n\n  %s\n      %s, %s the other cannot reach\n  %s\n      %s, %s the other cannot reach\n\n",
			ledger.Ref, shortCommit(d.Ledger), count(d.Ahead, "commit"),
			ledger.UpstreamRef, shortCommit(d.Upstream), count(d.Behind, "commit"))
		fmt.Fprint(w, "Two clones appended, and neither side's records are a prefix of the other's.\n"+
			"kept ledger sync only fast-forwards, so it refuses this. A ledger is\n"+
			"append-only, so the answer is both sides: commit a union of the two tips by\n"+
			"hand, which is what the .gitattributes merge=union rule is for. Nothing is\n"+
			"lost while both refs exist.\n")
	default:
		held := shortCommit(d.Ledger)
		if d.Ledger == "" {
			held = "nothing yet"
		}
		fmt.Fprintf(w, "\nthe ledger is behind what origin last held:\n\n  %s\n      %s\n  %s\n      %s, %s ahead\n\n",
			ledger.Ref, held, ledger.UpstreamRef, shortCommit(d.Upstream), count(d.Behind, "commit"))
		fmt.Fprint(w, "A fetch writes the upstream ref and never the ledger, so records origin holds\n"+
			"are in this repository and outside what kept reads. Every verb that writes\n"+
			"fast-forwards the ledger onto them first, and kept ledger sync does it on its\n"+
			"own. A verb that only reads never will, so that it can run against a read-only\n"+
			"checkout. Until then, what is ratified here is the older answer.\n")
	}
}
