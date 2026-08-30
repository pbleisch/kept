package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/schema"
)

// Resolving a number two artifacts claim.
//
// A `duplicate-number` is the one identity finding with an answer that is not a
// judgment call: the number is minted one past the highest there is, so two
// branches cut from one commit find the same free one, and the second to land
// simply has to take another. That is arithmetic, and `kept doctor --fix` can do
// it — but only where it is genuinely free, and the two things that make it not
// free are what most of this file is about.
//
// **The id is the ledger key.** An act names its artifact as "type:slug", so
// renumbering an artifact the ledger names leaves a grant naming an artifact
// that is not here — which is the `absent` standing doctor already reports, and
// the ledger is append-only, so nothing can amend it afterwards. Manufacturing
// the finding another check exists to catch is not a fix. It is refused rather
// than warned about, and the honest path is a person releasing it and
// re-ratifying under the new id, which is a person's act.
//
// **A citation is the id written down somewhere else.** This corpus holds 194
// `target: decision:NNNN-…` entries, 14 inline `[[decision:NNNN-…]]` links and
// 285 relative `](NNNN-….md)` links into `docs/decision`. The first two are what
// `promise:every-citation-in-the-corpus-resolves` is in force over, so a
// renumber that left them behind would convert one reported problem into a
// broken ratified promise; the third is invisible to kept entirely — nothing
// parses a markdown link — so leaving those behind would break up to sixteen
// references per decision with nothing reporting it, which is worse. So every
// occurrence moves with the id, or the renumber is refused.
//
// The rewrite is bounded to exactly that. It replaces occurrences of the old
// **slug** — a long, unique string, matched only where it is not part of a
// longer name — and, in the artifact's own file alone, the ordinal that opens
// its title and its H1. A citation is a name rather than a sentence: what these
// edits change is how an identity kept itself just changed is spelled, and no
// file says anything different afterwards. That is the reading under which this
// stays inside `policy:nothing-kept-writes-rewrites-what-a-person-wrote`'s
// *identity and only identity*, and it is the whole of why the match is bounded
// and every changed line is printed.

// contested is one number that more than one artifact of a kind claims.
type contested struct {
	kind   string
	shape  schema.IDShape
	number int
	// members are the artifacts claiming it, by id, sorted.
	members []*corpus.Entry
}

// renumbering is one artifact moving to a free number, with every file that has
// to change for it.
type renumbering struct {
	entry *corpus.Entry
	from  string // the old id, "type:slug"
	to    string // the new one
	// oldSlug and newSlug are the halves the corpus actually holds — a
	// citation may be qualified, a filename is not, and both contain the slug.
	oldSlug, newSlug string
	number           int
	shape            schema.IDShape

	// why says how this artifact was chosen over the ones that stayed.
	why string
	// edits are the files to rewrite, in corpus order.
	edits []fileEdit
	// rename is where the artifact's own file moves to, corpus-relative.
	rename string
}

// fileEdit is one file's new bytes and what changed in it.
type fileEdit struct {
	path    string // corpus-relative
	content string
	changed []string
}

// refusal is a contested number kept will not resolve, and why.
type refusal struct {
	number string
	why    string
}

// planRenumbering decides what --fix would do about every contested number.
//
// held is the set of ids the ledger names — any act at all, not only a
// ratification in force. A retirement names an id too, and a ratification at a
// content version this is not is still a grant that would be left pointing at
// nothing; the question is whether the durable record mentions this id, because
// that record cannot be amended.
func planRenumbering(c *corpus.Corpus, held map[string]bool) ([]renumbering, []refusal) {
	var moves []renumbering
	var refused []refusal

	// The next free number per kind, advanced as each move takes one, so that
	// two collisions resolved in one run do not both move to the same place.
	next := map[string]int{}

	for _, group := range contestedNumbers(c) {
		free, taken := partition(group.members, held)

		if len(taken) > 1 {
			// The guard on a case kept ratify closes: it refuses either
			// artifact of a contested number, so arriving here needs two
			// branches that each ratified before they met. Nothing can be done
			// from here — moving either strands its standing in a record
			// nothing can amend — so it is said once and left.
			refused = append(refused, refusal{
				number: group.kind + " " + group.shape.Number(group.number),
				why: fmt.Sprintf("%s are %s named by the ledger, so neither can move without stranding its standing. "+
					"kept ratify refuses a contested number, so this took two branches that each ratified before they met, "+
					"and a person has to retire one and ratify it again under a new number",
					and(idsOfEntries(taken)), everyOne(len(taken))),
			})
			continue
		}

		// Two cases, and one rule: the artifact that cannot move keeps the
		// number. When the ledger names one it is that one, because moving it
		// would strand its standing; when it names none it is the one cited
		// most, because every citation of a mover has to be rewritten.
		movers := free
		if len(taken) == 0 {
			movers = without(free, mostCited(c, free))
		}

		for _, e := range movers {
			if next[group.kind] == 0 {
				next[group.kind] = nextNumber(c, group.kind, group.shape)
			}
			n := next[group.kind]
			next[group.kind] = n + 1

			move, why := planMove(c, e, group, n, len(taken) > 0)
			if why != "" {
				refused = append(refused, refusal{number: group.kind + " " + group.shape.Number(group.number), why: why})
				continue
			}
			moves = append(moves, move)
		}
	}
	return moves, refused
}

// planMove works out every byte that changes for one artifact taking a new
// number, and refuses rather than moving when it cannot account for all of them.
func planMove(c *corpus.Corpus, e *corpus.Entry, group contested, n int, kept bool) (renumbering, string) {
	shape := group.shape
	if !shape.Fits(n) {
		return renumbering{}, fmt.Sprintf("the next free %s number is %d and the schema declares %s, which has no room for it",
			group.kind, n, shape)
	}

	_, oldSlug := corpus.SplitRef(e.Artifact.ID)
	_, after, ok := shape.Split(oldSlug)
	if !ok || after == "" {
		// A member whose id is not the declared shape is reported as an
		// id-shape problem and is not renumbered here: what is wrong with it is
		// its shape, which is one file's authoring error rather than a clash.
		return renumbering{}, ""
	}
	newSlug := shape.Format(n, after)

	move := renumbering{
		entry:   e,
		from:    e.Artifact.ID,
		to:      corpus.Ref(group.kind, newSlug),
		oldSlug: oldSlug,
		newSlug: newSlug,
		number:  n,
		shape:   shape,
		why:     chosen(c, e, group, kept),
		rename:  filepath.ToSlash(filepath.Join(filepath.Dir(e.Path), newSlug+".md")),
	}

	if _, taken := c.Get(move.to); taken {
		return renumbering{}, fmt.Sprintf("%s would become %s, which already names an artifact", move.from, move.to)
	}
	if _, err := os.Stat(filepath.Join(c.Root, filepath.FromSlash(move.rename))); err == nil {
		return renumbering{}, fmt.Sprintf("%s is already a file, and nothing kept writes rewrites what a person wrote", move.rename)
	}

	files, err := corpusFiles(c.Root)
	if err != nil {
		return renumbering{}, err.Error()
	}
	for _, rel := range files {
		src, err := os.ReadFile(filepath.Join(c.Root, filepath.FromSlash(rel)))
		if err != nil {
			return renumbering{}, fmt.Sprintf("%s holds a citation and could not be read: %s", rel, err)
		}
		edit := rewrite(rel, string(src), move, rel == e.Path)
		if edit != nil {
			move.edits = append(move.edits, *edit)
		}
	}
	return move, ""
}

// rewrite is one file's new bytes, or nil when nothing in it names the artifact.
//
// It replaces the old slug wherever it is not part of a longer name. That is the
// whole match rule, and it is enough because a slug is long and unique: every
// spelling a citation takes — `decision:0069-…` in a links: entry, `[[decision:0069-…]]`
// in prose, `](0069-….md)` as a relative file link, the bare id in the artifact's
// own frontmatter — contains it, so nothing that names this artifact is left
// behind by construction.
//
// own is the artifact's own file, where the ordinal that opens the title and the
// H1 moves as well. Everywhere else the ordinal alone is left exactly as it is:
// four digits are not a name, and replacing them wherever they appear is how a
// rewrite bounded to identity turns into one that edits prose.
func rewrite(rel, src string, move renumbering, own bool) *fileEdit {
	out, slugs := replaceName(src, move.oldSlug, move.newSlug)
	var changed []string
	if slugs > 0 {
		changed = append(changed, count(slugs, "citation"))
	}
	if labels := relabel(&out, move); labels > 0 {
		changed = append(changed, count(labels, "link label"))
	}
	if own {
		var moved []string
		out, moved = renumberOwnHeadings(out, move)
		if slugs > 0 {
			changed = []string{"the id"}
		}
		changed = append(changed, moved...)
	}
	if out == src {
		return nil
	}
	return &fileEdit{path: rel, content: out, changed: changed}
}

// relabel moves the ordinal a markdown link uses as its own text, and only
// there. Every one of this corpus's 285 relative links into docs/decision is
// written `[0013](0013-the-gate-makes-concurrency-safe.md)`, so a renumber that
// moved the target and left the text would produce a link labelled with a number
// it no longer points at — the same lie one level out.
//
// It fires only where the text is exactly the old ordinal and the target is the
// file this renumber has just renamed, which is why it cannot reach prose: a
// bare 0013 in a sentence is four digits somebody wrote, and nothing here is
// entitled to decide what it meant. A wiki link's display half is left alone for
// that reason too — it is free text, and none in this corpus is a bare ordinal.
func relabel(src *string, move renumbering) int {
	shape := move.shape
	old, want := shape.Number(numberOf(shape, move.oldSlug)), shape.Number(move.number)
	if old == want {
		return 0
	}
	pattern := regexp.MustCompile(`\[` + regexp.QuoteMeta(old) + `\](\((?:[^)\s]*/)?` + regexp.QuoteMeta(move.newSlug) + `\.md(?:#[^)\s]*)?\))`)

	var n int
	out := pattern.ReplaceAllStringFunc(*src, func(m string) string {
		n++
		return "[" + want + "]" + m[len(old)+2:]
	})
	*src = out
	return n
}

// replaceName replaces every occurrence of old that is not part of a longer
// name, and says how many it replaced. A slug is letters, digits and hyphens, so
// an occurrence with one of those on either side is a different, longer name —
// `0069-a-thing` inside `0069-a-thing-considered-again` is not a citation of it.
func replaceName(src, old, new string) (string, int) {
	var b strings.Builder
	var n int
	for i := 0; i < len(src); {
		j := strings.Index(src[i:], old)
		if j < 0 {
			b.WriteString(src[i:])
			break
		}
		j += i
		end := j + len(old)
		if nameByte(src, j-1) || nameByte(src, end) {
			b.WriteString(src[i : j+1])
			i = j + 1
			continue
		}
		b.WriteString(src[i:j])
		b.WriteString(new)
		n++
		i = end
	}
	if n == 0 {
		return src, 0
	}
	return b.String(), n
}

// nameByte reports whether the byte at i is one a slug is made of. Out of range
// is not, which makes the start and the end of a file boundaries.
func nameByte(s string, i int) bool {
	if i < 0 || i >= len(s) {
		return false
	}
	c := s[i]
	return c == '-' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}

// renumberOwnHeadings moves the ordinal that opens the artifact's title and its
// first H1, and nothing else.
//
// Both carry the number in this corpus — `title: 0068 — An anchor is a base and
// a change` — and leaving them behind would make the file say one number and the
// id say another. It is tightly bounded on purpose: the value must *start* with
// the old ordinal, and the only two lines considered are the title: key in
// frontmatter and the first H1. Neither binds anywhere a numbered kind reaches —
// a title is the envelope's and an H1 sits before the first H2 — and standing is
// not in play at all, because an artifact the ledger names is never moved.
func renumberOwnHeadings(src string, move renumbering) (string, []string) {
	shape := move.shape
	old, want := shape.Number(numberOf(shape, move.oldSlug)), shape.Number(move.number)

	var changed []string
	lines := strings.Split(src, "\n")

	// Which half of the file a line is in decides what it can be. The title is
	// a frontmatter key and the H1 is prose, and a scaffolded artifact carries
	// commented-out keys at column zero — `# fields:` — which are neither. Read
	// without the fences, the first of those is taken for the heading and the
	// heading is never reached.
	front, title, heading := false, false, false
	for i, line := range lines {
		if strings.TrimRight(line, " \t") == "---" {
			if i == 0 {
				front = true
				continue
			}
			if front {
				front = false
			}
			continue
		}
		switch {
		case front && !title && strings.HasPrefix(line, "title:"):
			title = true
			if moved, ok := leadingNumber(strings.TrimPrefix(line, "title:"), old, want); ok {
				lines[i] = "title:" + moved
				changed = append(changed, "the title")
			}
		case !front && !heading && strings.HasPrefix(line, "# "):
			heading = true
			if moved, ok := leadingNumber(strings.TrimPrefix(line, "#"), old, want); ok {
				lines[i] = "#" + moved
				changed = append(changed, "the heading")
			}
		}
	}
	return strings.Join(lines, "\n"), changed
}

// leadingNumber replaces an ordinal a value opens with. A value that opens with
// something else is left alone: it is a title somebody wrote rather than one
// derived from the number.
func leadingNumber(value, old, want string) (string, bool) {
	trimmed := strings.TrimLeft(value, " \t")
	space := value[:len(value)-len(trimmed)]
	rest, ok := strings.CutPrefix(trimmed, old)
	if !ok || nameByte(rest, 0) {
		return value, false
	}
	return space + want + rest, true
}

func numberOf(shape schema.IDShape, slug string) int {
	n, _, _ := shape.Split(slug)
	return n
}

// apply writes one renumbering. The rename is last, so that a failure part way
// through leaves the artifact where every reader expects to find it.
func apply(root string, move renumbering) error {
	for _, e := range move.edits {
		path := filepath.Join(root, filepath.FromSlash(e.path))
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(e.content), info.Mode().Perm()); err != nil {
			return err
		}
	}
	from := filepath.Join(root, filepath.FromSlash(move.entry.Path))
	to := filepath.Join(root, filepath.FromSlash(move.rename))
	if _, err := os.Stat(to); err == nil {
		return fmt.Errorf("%s is already a file", move.rename)
	}
	return os.Rename(from, to)
}

// writeRenumbering says exactly what moved and what was edited to follow it. A
// renumber that silently touched five files would be the shape of defect this
// verb exists to report.
func writeRenumbering(w io.Writer, moves []renumbering, refused []refusal) {
	if len(moves) > 0 {
		fmt.Fprint(w, "renumbered:\n")
		for _, m := range moves {
			fmt.Fprintf(w, "  %s → %s\n", m.entry.Path, m.rename)
			fmt.Fprintf(w, "      %s is now %s\n", m.from, m.to)
			if m.why != "" {
				fmt.Fprintf(w, "      %s\n", m.why)
			}
			for _, e := range m.edits {
				fmt.Fprintf(w, "      %s: %s\n", e.path, strings.Join(e.changed, ", "))
			}
		}
	}
	if len(refused) > 0 {
		fmt.Fprint(w, "left alone:\n")
		for _, r := range refused {
			fmt.Fprintf(w, "  %s\n", r.number)
			for _, line := range wrap(r.why, 66) {
				fmt.Fprintf(w, "      %s\n", line)
			}
		}
	}
}

// contestedNumbers groups the artifacts of every numbered kind by the ordinal
// they claim, and returns the ordinals more than one of them claims.
func contestedNumbers(c *corpus.Corpus) []contested {
	byNumber := map[string]map[int][]*corpus.Entry{}
	for _, e := range c.All() {
		kind := string(e.Artifact.Kind)
		sc, ok := c.Schemas.For(kind)
		if !ok || !sc.ID.Numbered() {
			continue
		}
		_, slug := corpus.SplitRef(e.Artifact.ID)
		n, after, well := sc.ID.Split(slug)
		if !well || after == "" {
			continue
		}
		if byNumber[kind] == nil {
			byNumber[kind] = map[int][]*corpus.Entry{}
		}
		byNumber[kind][n] = append(byNumber[kind][n], e)
	}

	var out []contested
	for _, kind := range sortedNames(byNumber) {
		sc, _ := c.Schemas.For(kind)
		numbers := make([]int, 0, len(byNumber[kind]))
		for n := range byNumber[kind] {
			numbers = append(numbers, n)
		}
		sort.Ints(numbers)
		for _, n := range numbers {
			members := byNumber[kind][n]
			if len(members) < 2 {
				continue
			}
			sort.Slice(members, func(i, j int) bool { return members[i].Artifact.ID < members[j].Artifact.ID })
			out = append(out, contested{kind: kind, shape: sc.ID, number: n, members: members})
		}
	}
	return out
}

// partition separates the members the ledger names from the ones it does not.
func partition(members []*corpus.Entry, held map[string]bool) (free, taken []*corpus.Entry) {
	for _, e := range members {
		if held[e.Artifact.ID] {
			taken = append(taken, e)
			continue
		}
		free = append(free, e)
	}
	return free, taken
}

// mostCited is the member with the most references to it elsewhere in the
// corpus, so that the artifact that moves is the one whose move rewrites least.
// Ties go to the first id, so two runs of this decide the same way.
func mostCited(c *corpus.Corpus, members []*corpus.Entry) *corpus.Entry {
	best, bestN := members[0], -1
	for _, e := range members {
		if n := citations(c, e); n > bestN {
			best, bestN = e, n
		}
	}
	return best
}

// citations counts the references to an artifact written anywhere in the corpus
// but its own file. It counts occurrences of the slug rather than resolved
// links, because a relative markdown link into docs/decision is a citation the
// corpus does not resolve and a renumber still has to carry.
func citations(c *corpus.Corpus, e *corpus.Entry) int {
	_, slug := corpus.SplitRef(e.Artifact.ID)
	files, err := corpusFiles(c.Root)
	if err != nil {
		return 0
	}
	var n int
	for _, rel := range files {
		if rel == e.Path {
			continue
		}
		src, err := os.ReadFile(filepath.Join(c.Root, filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		_, found := replaceName(string(src), slug, slug)
		n += found
	}
	return n
}

// chosen says why this artifact moved and the others did not.
func chosen(c *corpus.Corpus, e *corpus.Entry, group contested, kept bool) string {
	var others []string
	for _, other := range group.members {
		if other == e {
			continue
		}
		others = append(others, other.Artifact.ID)
	}
	if kept {
		return fmt.Sprintf("%s is named by the ledger and cannot move, so this one did", and(others))
	}
	return fmt.Sprintf("nothing in the ledger names either; %s is cited %s and this one %s, so this one moved",
		and(others), count(citationsOf(c, group.members, e), "time"), count(citations(c, e), "time"))
}

func citationsOf(c *corpus.Corpus, members []*corpus.Entry, but *corpus.Entry) int {
	var n int
	for _, e := range members {
		if e != but {
			n += citations(c, e)
		}
	}
	return n
}

// corpusFiles is every file a citation could be written in: the markdown of the
// corpus and the manifests beside it.
func corpusFiles(root string) ([]string, error) {
	var out []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		name := info.Name()
		if info.IsDir() {
			if path != root && strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(name, ".") {
			return nil
		}
		switch filepath.Ext(name) {
		case ".md", ".yaml", ".yml":
		default:
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(out)
	return out, err
}

func without(entries []*corpus.Entry, but *corpus.Entry) []*corpus.Entry {
	var out []*corpus.Entry
	for _, e := range entries {
		if e != but {
			out = append(out, e)
		}
	}
	return out
}

func idsOfEntries(entries []*corpus.Entry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Artifact.ID
	}
	return out
}

// everyOne is how a sentence quantifies a list it has just named.
func everyOne(n int) string {
	if n == 2 {
		return "both"
	}
	return "all"
}

// and is a list as a sentence names one.
func and(items []string) string {
	switch len(items) {
	case 0:
		return "nothing"
	case 1:
		return items[0]
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}
