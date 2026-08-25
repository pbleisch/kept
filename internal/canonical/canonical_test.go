package canonical

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/corpus"
)

// promiseSchema binds the four fields a promise binds on, plus its links, so
// that every reserved name decision 0003 defines is exercised somewhere.
const promiseSchema = `type: promise
hash: [statement, gather, hold_when, trigger, links.cites]
fields:
  statement: { type: string, required: true }
  gather:    { type: map }
  hold_when: { type: string }
  trigger:   { type: enum, values: [cadence, on-act, once] }
body: why
links:
  allowed_relations: [cites, serves]
`

const canonSchema = `type: canon
hash: [statement, body]
fields:
  statement: { type: string, required: true }
body: statement_detail
`

// build writes a corpus and loads it. Paths are relative to the corpus root,
// which is also the workspace root.
func build(t *testing.T, files map[string]string) *corpus.Corpus {
	t.Helper()
	root := t.TempDir()
	write(t, root, files)

	c, err := corpus.Load(root, root, corpus.Options{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return c
}

func write(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// hashOf binds one artifact of a corpus, failing the test if it cannot.
func hashOf(t *testing.T, c *corpus.Corpus, id string) Hash {
	t.Helper()
	e, ok := c.Get(id)
	if !ok {
		t.Fatalf("no artifact %s", id)
	}
	kind, _ := corpus.SplitRef(id)
	sc, _ := c.Schemas.For(kind)
	h, err := Bind(e, sc)
	if err != nil {
		t.Fatalf("Bind(%s): %v", id, err)
	}
	return h
}

// promise returns a corpus holding one promise with the given frontmatter body.
func promise(t *testing.T, front, body string) *corpus.Corpus {
	t.Helper()
	return build(t, map[string]string{
		"_schema/promise.yaml": promiseSchema,
		"promise/p.md":         "---\n" + front + "---\n\n" + body,
	})
}

const basePromise = `id: p
title: A promise
fields:
  statement: tests pass
  hold_when: obs.tests.failed == 0
  trigger: cadence
links:
  - { target: "canon:one", relation: cites }
`

// The five rules of decision 0022, each one on its own, because a normalizer
// that skips one is a normalizer that de-ratifies a corpus on the day someone
// opens a file in a different editor.
func TestNormalizeAppliesEveryRule(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"a leading byte-order mark is dropped", "\uFEFFtext", "text\n"},
		{"CRLF becomes LF", "a\r\nb", "a\nb\n"},
		{"a lone CR becomes LF", "a\rb", "a\nb\n"},
		{"trailing spaces and tabs go", "a  \t\nb\t", "a\nb\n"},
		{"exactly one final newline", "a\n\n\n", "a\n"},
		{"a missing final newline is added", "a", "a\n"},
		{"NFD becomes NFC", "Kae\u0301l", "Ka\u00e9l\n"},
	} {
		if got := Normalize(tc.in); got != tc.want {
			t.Errorf("%s: Normalize(%q) = %q, want %q", tc.name, tc.in, got, tc.want)
		}
	}

	// The guard on rule 4: whitespace is a wide category and some of it is
	// prose. A non-breaking space at the end of a line is content its author
	// typed, and trimming it would say two different texts are the same one.
	if got := Normalize("a\u00a0"); got != "a\u00a0\n" {
		t.Errorf("Normalize stripped a non-breaking space: %q", got)
	}
}

// The macOS and Linux case decision 0022 names outright: the same file, the
// same words, two unicode forms, and standing must not depend on which one the
// filesystem handed back.
func TestTheSameWordsInTwoUnicodeFormsHashTheSame(t *testing.T) {
	// Written as code points rather than as literal characters. This source
	// file is itself stored in one form, so a pair of literals could be
	// normalized to the same bytes by an editor and the test would quietly stop
	// testing anything.
	const composed = "Ka\u00e9l is tested"    // NFC: e-acute, one rune
	const decomposed = "Kae\u0301l is tested" // NFD: e, then a combining acute
	if composed == decomposed {
		t.Fatal("the two forms are the same bytes, so this test proves nothing")
	}

	nfc := hashOf(t, promise(t, strings.Replace(basePromise, "tests pass", composed, 1), "why"), "promise:p")
	nfd := hashOf(t, promise(t, strings.Replace(basePromise, "tests pass", decomposed, 1), "why"), "promise:p")

	if !nfc.Equal(nfd) {
		t.Fatalf("NFC hashed %s and NFD hashed %s; one corpus copied between macOS and Linux would de-ratify itself",
			nfc.Short(), nfd.Short())
	}
}

// Decision 0022 asks for normalization *inside the hash*, not only at rest, and
// this is the case that shows why the distinction is not pedantry. The corpus
// reader strips CRLF from a file's bytes, so a CRLF checkout is handled twice
// over — but a carriage return written as a YAML escape is produced by the
// decoder after that pass and reaches the hash intact. Only the rule inside the
// hash sees it.
func TestACarriageReturnFromTheDecoderIsStillNormalized(t *testing.T) {
	escaped := hashOf(t, promise(t, `id: p
fields:
  statement: "one\r\ntwo"
`, "why"), "promise:p")

	plain := hashOf(t, promise(t, `id: p
fields:
  statement: "one\ntwo"
`, "why"), "promise:p")

	if !escaped.Equal(plain) {
		t.Fatalf("a carriage return that arrived from the YAML decoder changed the hash: %s and %s",
			escaped.Short(), plain.Short())
	}
}

// The guard on the length prefixes. Without them the preimage is a
// concatenation, and a concatenation lets one promise be rewritten into a
// different promise without changing its hash — which is standing surviving a
// change to what was promised.
func TestTheEncodingIsUnambiguous(t *testing.T) {
	for _, tc := range []struct {
		name string
		a, b any
	}{
		{"a key and a value can trade a character",
			map[string]any{"ab": "c"}, map[string]any{"a": "bc"}},
		{"two fields can trade a character",
			map[string]any{"a": "b", "cd": "e"}, map[string]any{"a": "bc", "d": "e"}},
		{"a list of two is not a list of one",
			[]any{"a", "b"}, []any{"ab"}},
		{"a number is not the text of a number",
			int64(12), "12"},
		{"a bool is not the text of a bool",
			true, "true"},

		// The one that only the length prefix catches, and it is not
		// contrived: normalization ends every text with a newline, so a
		// multi-line body whose lines happen to start with a tag letter can
		// be re-read as the key and value that follow it. A promise's why is
		// prose, and prose has lines beginning with s.
		{"a multi-line value can be re-read as a key and a value",
			map[string]any{"a": "b\nsc\nsd"}, map[string]any{"a\nsb": "c\nsd"}},
	} {
		if encodeValue(tc.a) == encodeValue(tc.b) {
			t.Errorf("%s: %#v and %#v encode identically", tc.name, tc.a, tc.b)
		}
	}
}

func encodeValue(v any) string {
	e := NewEncoder("", Normalized)
	e.Value(v)
	return string(e.Bytes())
}

// Reformatting is free. Decision 0003 says the hash is over the canonical
// serialization of the named pairs and not over raw bytes, so key order,
// quoting and indentation must not reach it.
func TestReformattingDoesNotDropStanding(t *testing.T) {
	written := hashOf(t, promise(t, basePromise, "why"), "promise:p")

	reordered := hashOf(t, promise(t, `id: p
title: A promise
links:
  - { target: 'canon:one', relation: cites }
fields:
  trigger: "cadence"
  hold_when: "obs.tests.failed == 0"
  statement: >-
    tests pass
`, "why"), "promise:p")

	if !written.Equal(reordered) {
		t.Fatalf("reordering and requoting frontmatter changed the hash: %s then %s", written.Short(), reordered.Short())
	}
}

// The whole of decision 0003: the schema decides what binds, so prose is free
// and the predicate is not. Both halves get a check, because a hash that
// covered everything and a hash that covered nothing would each pass one of
// them.
func TestOnlyTheSchemasHashListBinds(t *testing.T) {
	base := hashOf(t, promise(t, basePromise, "why this matters"), "promise:p")

	prose := hashOf(t, promise(t, basePromise, "an entirely different why, rewritten"), "promise:p")
	if !base.Equal(prose) {
		t.Errorf("rewriting a promise's prose dropped its standing; body is not in the promise hash list")
	}

	title := hashOf(t, promise(t, strings.Replace(basePromise, "title: A promise", "title: Renamed", 1), "why this matters"), "promise:p")
	if !base.Equal(title) {
		t.Errorf("retitling dropped standing; title is not in the hash list")
	}

	predicate := hashOf(t, promise(t, strings.Replace(basePromise, "obs.tests.failed == 0", "obs.tests.failed < 3", 1), "why this matters"), "promise:p")
	if base.Equal(predicate) {
		t.Fatalf("changing hold_when did not change the hash, so a rewritten predicate would keep the standing granted to the old one")
	}
}

// For canon the prose is the commitment, which is the case that makes a
// per-kind hash list worth having at all. See docs/decision/0007.
func TestCanonBindsItsBody(t *testing.T) {
	one := build(t, map[string]string{
		"_schema/canon.yaml": canonSchema,
		"canon/c.md":         "---\nid: c\nfields:\n  statement: Kael's eyes are green\n---\n\nAnd they have been since book one.",
	})
	two := build(t, map[string]string{
		"_schema/canon.yaml": canonSchema,
		"canon/c.md":         "---\nid: c\nfields:\n  statement: Kael's eyes are green\n---\n\nAnd they have been since book two.",
	})
	if hashOf(t, one, "canon:c").Equal(hashOf(t, two, "canon:c")) {
		t.Fatal("canon binds on body, and rewriting the body did not change the hash")
	}
}

// The hash list is read as a set. Reordering it is not a change to what binds
// and must not re-hash the corpus; adding or removing a name is, and must.
func TestTheHashListIsASetAndItsMembershipMatters(t *testing.T) {
	files := func(list string) map[string]string {
		return map[string]string{
			"_schema/promise.yaml": "type: promise\nhash: " + list + "\nfields:\n  statement: { type: string }\n  trigger: { type: enum }\n",
			"promise/p.md":         "---\n" + basePromise + "---\n\nwhy",
		}
	}
	ordered := hashOf(t, build(t, files("[statement, trigger]")), "promise:p")
	reversed := hashOf(t, build(t, files("[trigger, statement]")), "promise:p")
	if !ordered.Equal(reversed) {
		t.Errorf("reordering a hash list re-hashed the corpus, and nothing about what binds changed")
	}

	narrower := hashOf(t, build(t, files("[statement]")), "promise:p")
	if ordered.Equal(narrower) {
		t.Fatal("dropping a name from the hash list did not change the hash; decision 0003 requires that to be a bulk re-ratification")
	}
}

// The guard on the present/absent tag. A field nobody wrote and a field written
// empty are different content, and an encoding that could not tell them apart
// would let one be turned into the other silently.
func TestAnAbsentFieldIsNotAnEmptyOne(t *testing.T) {
	files := func(fields string) map[string]string {
		return map[string]string{
			"_schema/promise.yaml": "type: promise\nhash: [statement, hold_when]\nfields:\n  statement: { type: string }\n  hold_when: { type: string }\n",
			"promise/p.md":         "---\nid: p\nfields:\n  statement: s\n" + fields + "---\n\nwhy",
		}
	}
	unwritten := hashOf(t, build(t, files("")), "promise:p")
	empty := hashOf(t, build(t, files("  hold_when: \"\"\n")), "promise:p")
	if unwritten.Equal(empty) {
		t.Fatal("a promise with no hold_when hashed the same as one with an empty hold_when")
	}
}

// Decision 0030 left this open on purpose and said increment 2 should reach it
// deliberately. A manifest exists so a third party can assert a link about an
// artifact without editing it — so if a manifest edge bound, anyone who can
// write a file next to an artifact could change what its ratification was
// granted against. That is minting standing from outside the one act reserved
// to a person, so it does not bind, and this is the check that says so.
func TestAManifestLinkDoesNotBindAndAFrontmatterLinkDoes(t *testing.T) {
	files := func(manifest, front string) map[string]string {
		return map[string]string{
			"_schema/promise.yaml": promiseSchema,
			"_schema/canon.yaml":   canonSchema,
			"canon/one.md":         "---\nid: one\nfields:\n  statement: one\n---\n\nx",
			"canon/two.md":         "---\nid: two\nfields:\n  statement: two\n---\n\nx",
			"_links/p.yaml":        manifest,
			"promise/p.md":         "---\nid: p\nfields:\n  statement: tests pass\nlinks:\n" + front + "---\n\nwhy",
		}
	}
	none := hashOf(t, build(t, files("subject: promise:p\nlinks: []\n",
		"  - { target: canon:one, relation: cites }\n")), "promise:p")

	asserted := hashOf(t, build(t, files("subject: promise:p\nlinks:\n  - { target: canon:two, relation: cites }\n",
		"  - { target: canon:one, relation: cites }\n")), "promise:p")

	if !none.Equal(asserted) {
		t.Fatalf("a third party writing _links/p.yaml changed what promise:p's ratification is keyed to: %s then %s",
			none.Short(), asserted.Short())
	}

	// The other half, or the check above would pass on an implementation that
	// ignored links entirely.
	authored := hashOf(t, build(t, files("subject: promise:p\nlinks: []\n",
		"  - { target: canon:one, relation: cites }\n  - { target: canon:two, relation: cites }\n")), "promise:p")
	if none.Equal(authored) {
		t.Fatal("the author added a cites link in frontmatter and the hash did not move, though the schema binds links.cites")
	}
}

// A sectioned kind, for the four checks below. Decision 0040: the schema
// declares the sections, and only a declared one may bind.
const sectionedSchema = "type: canon\nhash: [statement, body.rationale]\n" +
	"fields:\n  statement: { type: string }\nbody: [rationale, aside]\n"

func sectionedHash(t *testing.T, body string) Hash {
	t.Helper()
	c := build(t, map[string]string{
		"_schema/canon.yaml": sectionedSchema,
		"canon/c.md":         "---\nid: c\nfields:\n  statement: s\n---\n\n" + body,
	})
	return hashOf(t, c, "canon:c")
}

// Decision 0040 binds body.<section>, and this is the ordinary case working.
func TestASectionBindsAndItsProseIsWhatMoves(t *testing.T) {
	one := sectionedHash(t, "## Rationale\n\nbecause\n\n## Aside\n\nby the way")
	same := sectionedHash(t, "## Rationale\n\nbecause\n\n## Aside\n\nsomething else entirely")
	if !one.Equal(same) {
		t.Error("rewriting a section nothing binds moved the hash")
	}
	moved := sectionedHash(t, "## Rationale\n\nbecause it is\n\n## Aside\n\nby the way")
	if one.Equal(moved) {
		t.Error("rewriting the section the schema binds did not move the hash")
	}
}

// The guard: a declared section the artifact does not have is *absent*, not
// empty. 0031 spends its present/absent marks on exactly this, and collapsing
// the two would make deleting a section and blanking it hash identically —
// which is a change to binding content that no hash would show.
func TestAMissingSectionIsAbsentAndNotEmpty(t *testing.T) {
	missing := sectionedHash(t, "## Aside\n\nby the way")
	empty := sectionedHash(t, "## Rationale\n\n## Aside\n\nby the way")
	if missing.Equal(empty) {
		t.Fatal("an artifact with no rationale section hashed the same as one whose rationale is empty")
	}
}

// The guard: an H2 nobody declared is legal, is kept, and does not bind. It is
// 0002's leniency applied to the body, and the failure it prevents is a heading
// anybody can add quietly deciding what a ratification was granted against.
func TestAnUndeclaredSectionIsLegalAndDoesNotBind(t *testing.T) {
	plain := sectionedHash(t, "## Rationale\n\nbecause")
	extra := sectionedHash(t, "## Rationale\n\nbecause\n\n## Something nobody declared\n\nwords")
	if !plain.Equal(extra) {
		t.Error("adding a section the schema does not declare moved the binding hash")
	}

	c := build(t, map[string]string{
		"_schema/canon.yaml": "type: canon\nhash: [statement, body.undeclared]\n" +
			"fields:\n  statement: { type: string }\nbody: [rationale]\n",
		"canon/c.md": "---\nid: c\nfields:\n  statement: s\n---\n\n## Undeclared\n\nwords",
	})
	e, _ := c.Get("canon:c")
	sc, _ := c.Schemas.For("canon")
	if _, err := Bind(e, sc); err == nil {
		t.Fatal("a hash list bound a section the schema does not declare")
	} else if !strings.Contains(err.Error(), "declares no such section") {
		t.Errorf("the refusal does not say why: %v", err)
	}
}

// The guard: two headings landing on one identifier are refused rather than
// resolved first-wins. First-wins would put document order inside the binding
// hash, so that moving a section changes what was ratified with no diff saying
// so.
func TestTwoHeadingsOnOneIdentifierCannotBeRatified(t *testing.T) {
	c := build(t, map[string]string{
		"_schema/canon.yaml": sectionedSchema,
		"canon/c.md": "---\nid: c\nfields:\n  statement: s\n---\n\n" +
			"## Rationale\n\nfirst\n\n## rationale!\n\nsecond",
	})
	e, _ := c.Get("canon:c")
	sc, _ := c.Schemas.For("canon")
	if _, err := Bind(e, sc); err == nil {
		t.Fatal("an artifact with two headings naming one section was hashed, so one of them won silently")
	} else if !strings.Contains(err.Error(), "nothing picks one") {
		t.Errorf("the refusal does not say why: %v", err)
	}
}

// An artifact with nothing binding cannot be ratified, and refusing is the
// point: a hash over nothing is the same for every version of the file, so
// standing granted against it would survive every edit ever made.
func TestNothingBindingIsARefusalAndNotAHashOverNothing(t *testing.T) {
	c := build(t, map[string]string{
		"_schema/desire.yaml": "type: desire\nhash: []\nfields:\n  want: { type: string }\n",
		"desire/d.md":         "---\nid: d\nfields:\n  want: something\n---\n\nnote",
		"document/x.md":       "---\nid: x\nfields:\n  statement: s\n---\n\nprose",
	})

	desire, _ := c.Get("desire:d")
	sc, _ := c.Schemas.For("desire")
	var unbindable *Unbindable
	if _, err := Bind(desire, sc); err == nil {
		t.Fatal("a desire was hashed, and a desire cannot be ratified in place")
	} else if !errors.As(err, &unbindable) {
		t.Errorf("refusal is not an Unbindable: %T", err)
	}

	document, _ := c.Get("document:x")
	if _, err := Bind(document, nil); err == nil {
		t.Fatal("an artifact of a kind with no schema was hashed, so nothing declared what binds and it was hashed anyway")
	}
}

// A hash carries the rules that produced it, so a later refinement re-hashes
// old records under the version they were written with rather than dropping
// every ratification at once. Two hashes that agree on hex and disagree on norm
// answered different questions.
func TestAHashCarriesItsRulesVersion(t *testing.T) {
	h := hashOf(t, promise(t, basePromise, "why"), "promise:p")
	if h.Norm != Version {
		t.Errorf("Norm = %d, want %d", h.Norm, Version)
	}
	if h.Algorithm != Algorithm {
		t.Errorf("Algorithm = %q, want %q", h.Algorithm, Algorithm)
	}
	if h.Equal(Hash{Algorithm: h.Algorithm, Digest: h.Digest, Norm: h.Norm + 1}) {
		t.Fatal("two hashes with different norm compared equal, so refining the rules would not be visible")
	}
	if got := Parse(h.String(), h.Norm); !got.Equal(h) {
		t.Errorf("Parse(%q) = %#v, want %#v", h.String(), got, h)
	}
}

// A hash is a fact about content, not about where the content was read. Two
// copies of one corpus in two directories must agree, or a box and the tree it
// was cut from would disagree about what was ratified.
func TestTheHashDoesNotDependOnWhereTheCorpusIs(t *testing.T) {
	a := hashOf(t, promise(t, basePromise, "why"), "promise:p")
	b := hashOf(t, promise(t, basePromise, "why"), "promise:p")
	if !a.Equal(b) {
		t.Fatalf("two copies of one corpus hashed differently: %s and %s", a.Short(), b.Short())
	}
}

// The encoding is shared and the text rule is not. Decision 0034 reuses this
// preimage for an observation's address and hashes text as it was measured,
// because all four of these pairs are genuinely different measurements and
// 0022's rules make them one. Under content addressing that is not a wrong
// digest — it is the second measurement being discarded as a duplicate of the
// first, which is the stale answer 0014 exists to prevent.
//
// This is the check that fires if someone reaches for Normalized there later.
func TestVerbatimKeepsApartWhatNormalizedCollides(t *testing.T) {
	for _, tc := range []struct {
		name string
		a, b string
	}{
		{"trailing whitespace is data in a log", "FAIL\ttrailing   \n", "FAIL\ttrailing\n"},
		{"a tool that emits CRLF and one that does not", "line\r\nline2\r\n", "line\nline2\n"},
		{"how many newlines a program actually printed", "out", "out\n\n\n"},
		{"a filename a tool printed back", "café", "café"},
	} {
		if Normalized(tc.a) != Normalized(tc.b) {
			t.Errorf("%s: 0022's rules no longer collide %q and %q, so 0034's argument needs rereading", tc.name, tc.a, tc.b)
		}

		measured := func(s string) string {
			e := NewEncoder("kept/observation\nobs:1\n", Verbatim)
			e.Text(s)
			return e.Digest()
		}
		if measured(tc.a) == measured(tc.b) {
			t.Errorf("%s: %q and %q address identically, so one measurement is discarded as a duplicate of the other", tc.name, tc.a, tc.b)
		}
	}
}

// A domain carries its version, so a binding hash and a measurement's address
// cannot collide even where the bytes hashed are identical.
func TestTwoDomainsDoNotCollideOnIdenticalContent(t *testing.T) {
	binding := NewEncoder("kept/binding-hash\nnorm:1\n", Verbatim)
	binding.Text("same")
	observation := NewEncoder("kept/observation\nobs:1\n", Verbatim)
	observation.Text("same")

	if binding.Digest() == observation.Digest() {
		t.Error("two domains hashing one value produce one digest")
	}
}

// shippedCanonSchema is the real `docs/_schema/canon.yaml`, read rather than
// copied, so that the guard below is a guard on what ships. A local copy would
// go on passing after somebody shortened the hash list in the file that
// actually decides what a ratification is granted against.
func shippedCanonSchema(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "docs", "_schema", "canon.yaml"))
	if err != nil {
		t.Fatalf("read the shipped canon schema: %v", err)
	}
	return string(b)
}

// shippedSchema is the real `docs/_schema/{kind}.yaml`, read rather than
// copied, so these guards go on guarding the file that decides what a
// ratification was granted against.
func shippedSchema(t *testing.T, kind string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "docs", "_schema", kind+".yaml"))
	if err != nil {
		t.Fatalf("read the shipped %s schema: %v", kind, err)
	}
	return string(b)
}

// promiseHash binds one promise written against the shipped schema.
func promiseHash(t *testing.T, fields string) Hash {
	t.Helper()
	c := build(t, map[string]string{
		"_schema/promise.yaml": shippedSchema(t, "promise"),
		"promise/p.md": "---\nid: p\nfields:\n" + fields +
			"---\n\nEvery package under internal/ is tested.\n",
	})
	return hashOf(t, c, "promise:p")
}

const scopedPromise = `  statement: every package under internal/ has a test that passes
  gather:
    tests:
      scope: "internal/**"
  hold_when: obs.tests.ok
  trigger: cadence
  max_age: 7d
  repair:
    scope: ["internal/**"]
`

// A repair agent may write the corpus, so a promise whose scope reaches it can
// widen its own scope in the diff it is repairing with. If that does not move
// the hash, the widened bound is in force and standing never noticed — and the
// assessor that would catch it is two increments away. See docs/decision/0047.
func TestWideningARepairScopeDropsAPromisesStanding(t *testing.T) {
	scoped := promiseHash(t, scopedPromise)

	widened := strings.Replace(scopedPromise, `["internal/**"]`, `["**"]`, 1)
	if widened == scopedPromise {
		t.Fatal("the replacement matched nothing, so this test proves nothing")
	}
	if scoped.Equal(promiseHash(t, widened)) {
		t.Fatalf("a promise's repair scope was widened to the whole workspace and its hash did not move (%s): "+
			"an agent may now write anywhere and the ratification was granted against internal/**",
			scoped.Short())
	}

	// The other direction, or a schema that hashed the whole file would pass
	// the check above. max_age is how often kept re-asks rather than what is
	// owed, so tuning it must not charge a re-ratification.
	retimed := strings.Replace(scopedPromise, "max_age: 7d", "max_age: 30d", 1)
	if retimed == scopedPromise {
		t.Fatal("the replacement matched nothing, so this test proves nothing")
	}
	if !scoped.Equal(promiseHash(t, retimed)) {
		t.Error("a promise binds on max_age, so changing how often it is checked de-ratifies it")
	}
}

// canonHash binds one canon entry written against the shipped schema.
func canonHash(t *testing.T, fields string) Hash {
	t.Helper()
	c := build(t, map[string]string{
		"_schema/canon.yaml": shippedCanonSchema(t),
		"canon/c.md": "---\nid: c\nfields:\n" + fields +
			"---\n\nKael's eyes are green, in every book.\n",
	})
	return hashOf(t, c, "canon:c")
}

const checkedCanon = `  statement: Kael's eyes are green
  gather:
    contradictions:
      gatherer: text
      scope: "chapters/**"
      pattern: 'Kael[^.]{0,60}eyes[^.]{0,20}(blue|brown|grey)'
  hold_when: obs.contradictions.count == 0
`

// The guard on decision 0045, and it is the exact gutting the manuscript spike
// reproduced end to end: leave statement and body untouched, swap the pattern
// for one that matches nothing, and replace the predicate with the literal
// true. Under `hash: [statement, body]` that canon entry stayed ratified, so a
// repair agent — which may write the corpus — could weaken the instrument a
// ratified canon entry is checked by and land it, and standing did not notice.
func TestGuttingACanonEntrysInstrumentDropsItsStanding(t *testing.T) {
	checked := canonHash(t, checkedCanon)

	gutted := strings.NewReplacer(
		"(blue|brown|grey)", "ZZZNOMATCH(blue|brown|grey)",
		"obs.contradictions.count == 0", `"true"`,
	).Replace(checkedCanon)
	if gutted == checkedCanon {
		t.Fatal("the replacement matched nothing, so this test proves nothing")
	}

	if checked.Equal(canonHash(t, gutted)) {
		t.Fatalf("a canon entry's gatherer and predicate were replaced and its hash did not move (%s): "+
			"it is still ratified, and the thing it is checked by is not the thing it was ratified with",
			checked.Short())
	}

	// Each half on its own, or the test above passes on a schema that binds
	// only one of them.
	pattern := strings.Replace(checkedCanon, "(blue|brown|grey)", "ZZZNOMATCH(blue|brown|grey)", 1)
	if checked.Equal(canonHash(t, pattern)) {
		t.Error("canon does not bind on gather: the instrument was replaced and the hash held still")
	}
	predicate := strings.Replace(checkedCanon, "obs.contradictions.count == 0", `"true"`, 1)
	if checked.Equal(canonHash(t, predicate)) {
		t.Error("canon does not bind on hold_when: the predicate became the literal true and the hash held still")
	}

	// The other direction, or a schema that simply hashed the whole file would
	// pass every check above. `established_in` is where this entry was first
	// committed to, which is provenance rather than commitment, so it is
	// declared and does not bind.
	if !checked.Equal(canonHash(t, checkedCanon+"  established_in: chapter:one\n")) {
		t.Error("canon binds on established_in, so recording where a statement came from de-ratifies it")
	}
}

// The stake behind corpus's misplaced-field report: a binding field written at
// the top level does not reach the hash, so the artifact hashes exactly as if
// the author had written nothing at all.
//
// [resolve]'s ordinary case reads a name out of [corpus.Entry]'s Fields, and
// Fields is populated from the fields: block alone — which is right, and is what
// 0010 puts the block there for. What it cannot do is notice that a person wrote
// the value one level up. Five decisions in this repository were authored that
// way and were called clean: each hashed as though it had no statement, so their
// ratifications were granted against wording nobody could change and nothing
// would report. The check is in internal/corpus, where the schema and the file
// are both in hand; this is why it is worth having.
func TestAStatementAtTheTopLevelDoesNotReachTheHash(t *testing.T) {
	const nested = `id: p
title: A promise
fields:
  statement: tests pass
`
	const misplaced = `id: p
title: A promise
statement: tests pass
`
	const absent = `id: p
title: A promise
fields: {}
`
	hash := func(front string) Hash {
		return hashOf(t, promise(t, front, "Why.\n"), "promise:p")
	}

	// The control. Without it a match below would prove only that two hashes of
	// nothing agree, and the binding list is what has to be doing the work.
	if hash(nested).Equal(hash(absent)) {
		t.Fatal("a nested statement hashes the same as none, so the schema is not binding on it and nothing here is measured")
	}

	if !hash(misplaced).Equal(hash(absent)) {
		t.Error("a top-level statement reached the hash, which is not what Fields can do — has 0010 changed?")
	}

	// And the wording is free: this is the property the report exists to name.
	reworded := strings.Replace(misplaced, "tests pass", "tests do not pass", 1)
	if !hash(misplaced).Equal(hash(reworded)) {
		t.Error("a top-level statement was rewritten into its own negation and the hash moved")
	}
}
