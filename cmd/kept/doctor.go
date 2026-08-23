package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/gather"
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/model"
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
// The findings that are not about identity are an instrument nobody checked
// and an origin that force-fetches the ledger, and docs/decision/0050 is what
// admits them and where the line is: **doctor reports what is well-formed and
// unwise, and kept list reports what is wrong.** Nothing refuses either one,
// nothing is blocked by either, and neither has a reader anywhere else once a
// workspace is adopted. --fix is untouched by both: it writes identity, and a
// git config is not identity — kept init is what corrects the refspec, and it
// says so when it does. See docs/decision/0016.
func doctorCmd(args []string, stdout, stderr io.Writer) int {
	fs, root := flags("doctor", stderr)
	fix := fs.Bool("fix", false, "write the identity an adopted file is missing, and nothing else")
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

	found := identityProblems(c)
	if *fix && len(found) > 0 {
		repair(c, found, stdout)
		// Read the corpus again rather than subtracting what was written from
		// what was found. A file that has just been given an id is a new
		// artifact, and it may have brought a duplicate or a path/id
		// divergence with it — which is a finding this run should report
		// rather than one the next run discovers.
		if c, code = open(*root, stderr); code != exitOK {
			return code
		}
		found = identityProblems(c)
	}

	store := ledgerStore(c)
	acts, quarantined, err := ledger.ReadActs(store)
	if err != nil {
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return exitUsage
	}
	writeQuarantined(stderr, quarantined)

	var absent []register.Standing
	for _, s := range register.Derive(c, acts) {
		if s.State == register.Absent {
			absent = append(absent, s)
		}
	}

	writeFindings(stdout, c, found, unchecked(c), absent, forcedFetch(c, stderr), *fix)

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
func identity(k corpus.ProblemKind) bool {
	switch k {
	case corpus.NoFrontmatter, corpus.NoID, corpus.BadID,
		corpus.PathMismatch, corpus.TypeMismatch, corpus.Unreadable:
		return true
	}
	return false
}

// forcedFetch reports whether this workspace's origin still force-fetches the
// ledger. A workspace that is not a repository has no origin and no ledger ref,
// which is silence rather than a finding.
//
// It costs one local git config read and never contacts a remote. Whether the
// remote is behind is the question an operator actually has, and doctor cannot
// answer it: the refspec writes the remote's value straight into the local ref,
// so there is no remote-tracking ref to compare against, and asking would mean
// a network call from a diagnostic. What is knowable locally is whether the
// refusal that makes divergence safe has been turned off. See
// docs/decision/0016.
func forcedFetch(c *corpus.Corpus, stderr io.Writer) bool {
	forced, err := ledger.ForcedFetch(c.Workspace)
	if err != nil {
		if !errors.Is(err, ledger.ErrNotRepository) {
			fmt.Fprintf(stderr, "kept: read remote.origin.fetch: %s\n", err)
		}
		return false
	}
	return forced
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

func sortedNames(m map[string]model.GatherSpec) []string {
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

// repair writes the identity that is missing and returns what is left.
//
// One block per file and not a byte more: the file's own line endings are used
// for the lines inserted, a byte-order mark stays in front of them, and nothing
// already in the file is parsed, reordered or re-emitted. See 0029 and 0042.
func repair(c *corpus.Corpus, found []corpus.Problem, stdout io.Writer) {
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
		fmt.Fprintf(stdout, "\n%s. Read the diff: an id is authored, and this one was taken from the filename.\n",
			count(written, "file"))
	}
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
func yamlScalar(s string) string {
	plain := s != "" &&
		!strings.ContainsAny(s[:1], "-?:,[]{}#&*!|>'\"%@`") &&
		!strings.Contains(s, ": ") &&
		!strings.Contains(s, " #") &&
		!strings.HasSuffix(s, ":") &&
		strings.TrimSpace(s) == s
	if plain {
		return s
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

func writeFindings(w io.Writer, c *corpus.Corpus, found []corpus.Problem, loose []instrument, absent []register.Standing, forced, fixed bool) {
	if len(found) == 0 && len(absent) == 0 && len(loose) == 0 && !forced {
		fmt.Fprintf(w, "%s, and nothing wrong with what names them\n", count(len(c.All()), "artifact"))
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
			fmt.Fprintf(w, "\n%s of these is an identity nobody wrote, which kept doctor --fix writes.\n", count(n, "problem"))
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
			"exit status and so does this. Nothing here is refused. See decision 0050.\n")
	}

	if forced {
		fmt.Fprintf(w, "\norigin force-fetches the ledger:\n\n  remote.origin.fetch\n      %s\n\n",
			ledger.ForcedFetchRefspec)
		fmt.Fprint(w, "The leading + lets a fetch from a remote that has fallen behind rewind\n"+
			"the ledger over acts nobody pushed — exit 0, no output, and no reflog,\n"+
			"because side refs are not logged. Unforced, git refuses that fetch by name.\n"+
			"kept init rewrites this one line and changes nothing else. See decision 0016.\n")
	}

	if len(absent) > 0 {
		fmt.Fprintf(w, "\n%s with standing and no content:\n\n", count(len(absent), "artifact"))
		for _, s := range absent {
			fmt.Fprintf(w, "  %s\n      ratified %s at %s, and no artifact claims this id — never read as a release\n",
				s.Artifact, s.Granted.At.Format(day), s.Granted.Hash.Short())
		}
		fmt.Fprint(w, "\nEither the content is gone or an id was edited by hand. Deleting a file is not\n"+
			"a way to retire a promise; kept release is. See decision 0021.\n")
	}
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
