package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The structural promises, each run against a module built for the test and
// then broken on purpose.
//
// A structural promise whose guard has never fired is a claim rather than a
// check, so every one of these does three things: it holds over a module shaped
// like this one, it goes not-kept when the boundary it names is crossed, and it
// stays kept when something next to that boundary moves. The third is not
// ceremony — docs/decision/0049 exists because the first version of *the core
// carries no domain vocabulary* found seventeen hits and every one of them was a
// doc comment, so an adjacent change that must **not** fire is exactly what
// separates the instrument that answers about code from the one that answers
// about bytes.
//
// They run against the promises as authored rather than a copy of them, read
// out of docs/promise. Weaken a predicate and these fail, which is the point.

// repoFile is a file of this repository, read at its authored path. The tests
// run with the package directory as their working directory, so the corpus is
// two levels up.
func repoFile(t *testing.T, rel string) string {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(src)
}

// selfModule is a workspace holding a module shaped like kept's — the same
// package names, the same edges between them, the same one constructor for a
// ratification — and one of kept's own promises about it.
//
// It is a fixture rather than this repository because a break test has to
// change the tree it is about, and the tree this test runs from is not one it
// may write to.
type selfModule struct{ *workspace }

func newSelfModule(t *testing.T, slugs ...string) *selfModule {
	t.Helper()
	m := &selfModule{workspace: newWorkspace(t, nil)}

	for _, kind := range []string{"promise", "policy", "decision"} {
		m.put("docs/_schema/"+kind+".yaml", repoFile(t, "docs/_schema/"+kind+".yaml"))
	}
	for _, slug := range slugs {
		m.put("docs/promise/"+slug+".md", repoFile(t, "docs/promise/"+slug+".md"))
	}

	m.put("go.mod", "module github.com/pbleisch/kept\n\ngo 1.26\n")

	// internal/model knows what an artifact is and imports nothing in-module.
	m.put("internal/model/model.go", `package model

type Artifact struct{ ID string }

type Kind string

// KindDesire is a want nobody has committed to, and never placed in a packet.
const KindDesire Kind = "desire"

// GatherSpec is the vocabulary the core is allowed: that there are gatherers.
// Which ones there are is the catalogue's business.
type GatherSpec struct{ Gatherer string }
`)
	// internal/predicate reaches model and nothing else.
	m.put("internal/predicate/eval.go", `package predicate

import "github.com/pbleisch/kept/internal/model"

type Gathered struct{ Of model.Artifact }
`)
	// internal/ledger declares the one constructor of a ratification.
	m.put("internal/ledger/ledger.go", `package ledger

import "github.com/pbleisch/kept/internal/model"

type Kind string

const Ratified Kind = "ratified"

type Ledger struct{ of model.Artifact }

func Open() *Ledger { return &Ledger{} }

func (l *Ledger) Ratify(artifact string) Kind { return Ratified }

// An observation is evidence, and a packet carries it.
type Observation struct{ Tree string }

// A disposition is the queue: what a tick did about a drift, and why.
type Outcome string

type Condition string

const Rank Condition = "rank"

type Disposition struct {
	Outcome   Outcome
	Condition Condition
}

// A thread's receipt is ledger records, and a transcript is one of them.
const ThreadTranscript Kind = "thread_transcript"
`)
	// internal/corpus reads what a person wrote and cannot reach the ledger.
	m.put("internal/corpus/corpus.go", `package corpus

import "github.com/pbleisch/kept/internal/model"

func Load(root string) []model.Artifact { return nil }
`)
	// internal/gather observes. os.ReadFile is the only filesystem call it makes.
	m.put("internal/gather/gather.go", `package gather

import (
	"os"

	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/model"
)

func Read(path string) ([]byte, error) { return os.ReadFile(path) }

func Kinds() []ledger.Kind { return nil }

func Of(a model.Artifact) {}
`)
	// internal/register folds the acts. It reads the ratified kind and mints
	// nothing, which is the case a promise about minting must not report.
	m.put("internal/register/register.go", `package register

import "github.com/pbleisch/kept/internal/ledger"

func stands(k ledger.Kind) bool { return k == ledger.Ratified }
`)
	// internal/tick runs the catalogue, so it imports gather by construction.
	m.put("internal/tick/tick.go", `package tick

import (
	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/gather"
	"github.com/pbleisch/kept/internal/ledger"
)

type Tick struct{ Ledger *ledger.Ledger }

func (t *Tick) Run(root string) { corpus.Load(root); gather.Kinds() }

func (t *Tick) Dispose() ledger.Disposition { return ledger.Disposition{Condition: ledger.Rank} }
`)
	// internal/thread assembles a packet from what its input carries, and is
	// where a session's words arrive. It reaches the ledger for the observation a
	// packet holds and the records a receipt is made of.
	m.put("internal/thread/packet.go", `package thread

import (
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/model"
)

type PacketInput struct {
	Promise      model.Artifact
	Why          string
	Observations map[string]ledger.Observation
}

type Packet struct{ bytes []byte }

func AssemblePacket(in PacketInput) (Packet, error) { return Packet{}, nil }

func Transcript() ledger.Kind { return ledger.ThreadTranscript }
`)
	// internal/repair is where the agent and the verdict meet: it dispatches a
	// session, re-derives with the tick, and records the disposition that says
	// it acted. It reaches both sides and must.
	m.put("internal/repair/perform.go", `package repair

import (
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/thread"
	"github.com/pbleisch/kept/internal/tick"
)

func Perform(t *tick.Tick) ledger.Disposition {
	thread.AssemblePacket(thread.PacketInput{})
	t.Run(".")
	return ledger.Disposition{Outcome: "act"}
}
`)
	return m
}

// putDecision writes a decision the fixture's corpus can resolve a citation to.
// The promises copied in here cite this repository's real decisions, and a
// citation with nothing at the other end is exactly what the citation promise
// reports — so the ones its own frontmatter names have to be present before the
// break is the thing being measured.
func (m *selfModule) putDecision(id string) {
	m.put("docs/decision/"+id+".md",
		"---\nid: "+id+"\ntype: decision\ntitle: "+id+"\n---\n\n"+
			"## Context\n\nSomething.\n\n## Decision\n\nThis.\n\n## Consequences\n\nThat.\n")
}

// verdict is what this tick decided about one artifact, as the machine-facing
// document reports it. Read out of --json rather than the table, because the
// three words are what is being asserted and a boolean would collapse two of
// them. See docs/decision/0027.
func (m *selfModule) verdict(t *testing.T, id string) string {
	t.Helper()
	_, stdout, stderr := m.run("--json", "--all")

	var doc struct {
		Artifacts []struct {
			ID    string `json:"id"`
			Truth *struct {
				Value  string `json:"value"`
				Detail string `json:"detail"`
			} `json:"truth"`
		} `json:"artifacts"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("the document does not parse: %v\n%s\n%s", err, stdout, stderr)
	}
	for _, a := range doc.Artifacts {
		if a.ID != id {
			continue
		}
		if a.Truth == nil {
			t.Fatalf("%s reached no verdict at all:\n%s", id, stdout)
		}
		return a.Truth.Value
	}
	t.Fatalf("%s is not in the document:\n%s", id, stdout)
	return ""
}

// holds asserts a verdict and says which promise and which change produced it.
func (m *selfModule) holds(t *testing.T, id, want, after string) {
	t.Helper()
	if got := m.verdict(t, id); got != want {
		t.Fatalf("%s is %s after %s, want %s", id, got, after, want)
	}
}

// Standing is minted in one place. The guard is the shape of the code: one
// declaration of the constructor, in the ledger, and no package under internal/
// opening a ledger of its own to call it with.
func TestStandingIsMintedInOnePlaceNoticesASecondConstructor(t *testing.T) {
	const id = "promise:standing-is-minted-in-one-place"
	m := newSelfModule(t, "standing-is-minted-in-one-place")
	m.holds(t, id, "kept", "the module as it is")

	// Reading the ratified kind is what the register does and must go on
	// doing. A promise about minting that fires on this is measuring the wrong
	// thing.
	m.put("internal/register/register.go", `package register

import "github.com/pbleisch/kept/internal/ledger"

func stands(k ledger.Kind) bool { return k == ledger.Ratified }

func retired(k ledger.Kind) bool { return k != ledger.Ratified }
`)
	m.holds(t, id, "kept", "the register reading the kind twice over")

	// A second declaration of the constructor, anywhere under internal/.
	m.put("internal/tick/tick.go", `package tick

import "github.com/pbleisch/kept/internal/ledger"

type Tick struct{ Ledger *ledger.Ledger }

func (t *Tick) Ratify(artifact string) {}
`)
	m.holds(t, id, "not-kept", "a second Ratify under internal/")
}

func TestStandingIsMintedInOnePlaceNoticesAPackageOpeningItsOwnLedger(t *testing.T) {
	const id = "promise:standing-is-minted-in-one-place"
	m := newSelfModule(t, "standing-is-minted-in-one-place")
	m.holds(t, id, "kept", "the module as it is")

	m.put("internal/tick/tick.go", `package tick

import "github.com/pbleisch/kept/internal/ledger"

func Own() *ledger.Ledger { return ledger.Open() }
`)
	m.holds(t, id, "not-kept", "a package under internal/ opening a ledger")
}

// The corpus cannot write standing or truth. Reachability, so the edge that
// arrives through a third package is the one this is really for.
func TestTheCorpusBoundaryNoticesTheEdgeThatArrivesThroughAThirdPackage(t *testing.T) {
	const id = "promise:the-corpus-cannot-write-standing-or-truth"
	m := newSelfModule(t, "the-corpus-cannot-write-standing-or-truth")
	m.holds(t, id, "kept", "the module as it is")

	// An in-module edge that is not the ledger's is not this boundary.
	m.put("internal/corpus/corpus.go", `package corpus

import (
	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/predicate"
)

func Load(root string) []model.Artifact { return nil }

func Check(g predicate.Gathered) {}
`)
	m.holds(t, id, "kept", "the corpus reaching the predicate")

	// Two hops, through a package that has every reason to exist. This is the
	// one a direct-imports promise would miss.
	m.put("internal/canonical/hash.go", `package canonical

import "github.com/pbleisch/kept/internal/ledger"

func Sum(k ledger.Kind) string { return string(k) }
`)
	m.put("internal/corpus/corpus.go", `package corpus

import (
	"github.com/pbleisch/kept/internal/canonical"
	"github.com/pbleisch/kept/internal/model"
)

func Load(root string) []model.Artifact { canonical.Sum(""); return nil }
`)
	m.holds(t, id, "not-kept", "the corpus reaching the ledger through canonical")
}

// Gatherers do not write. The instrument reads identifiers, so a read must not
// fire and prose must not fire.
func TestGatherersDoNotWriteNoticesAWriteAndNotAReadOrAComment(t *testing.T) {
	const id = "promise:gatherers-do-not-write"
	m := newSelfModule(t, "gatherers-do-not-write")
	m.holds(t, id, "kept", "the module as it is")

	m.put("internal/gather/gather.go", `package gather

import "os"

// Read never calls os.WriteFile or os.MkdirAll, and saying so is prose.
func Read(path string) ([]byte, error) { return os.ReadFile(path) }

func When(path string) (os.FileInfo, error) { return os.Stat(path) }
`)
	m.holds(t, id, "kept", "a second read and a comment naming the write verbs")

	m.put("internal/gather/gather.go", `package gather

import "os"

func Read(path string) ([]byte, error) { return os.ReadFile(path) }

func Fix(path string, body []byte) error { return os.WriteFile(path, body, 0o644) }
`)
	m.holds(t, id, "not-kept", "a gatherer that writes what it found")
}

// The core carries no domain vocabulary. Two halves, and each is broken on its
// own so that neither can be carrying the other.
func TestTheCoreCarriesNoDomainVocabularyNoticesAnIdentifierAndAnEdge(t *testing.T) {
	const id = "promise:the-core-carries-no-domain-vocabulary"
	m := newSelfModule(t, "the-core-carries-no-domain-vocabulary")
	m.holds(t, id, "kept", "the module as it is")

	// The measurement docs/decision/0049 records: scanned as bytes this is a
	// hit, and it is a doc comment. It must not fire.
	m.put("internal/model/model.go", `package model

type Artifact struct{ ID string }

// Gatherer is name and version, "tests@2" — and Corpus, Imports and Symbols
// are named in this sentence on purpose.
type GatherSpec struct{ Gatherer string }
`)
	m.holds(t, id, "kept", "the core's prose naming every gatherer there is")

	// An identifier named for a gatherer.
	m.put("internal/model/model.go", `package model

type Artifact struct{ ID string }

const Tests = "tests"

type GatherSpec struct{ Gatherer string }
`)
	m.holds(t, id, "not-kept", "a gatherer's name declared in the core")
}

func TestTheCoreCarriesNoDomainVocabularyNoticesAnEdgeToTheCatalogue(t *testing.T) {
	const id = "promise:the-core-carries-no-domain-vocabulary"
	m := newSelfModule(t, "the-core-carries-no-domain-vocabulary")
	m.holds(t, id, "kept", "the module as it is, with tick importing gather")

	m.put("internal/predicate/eval.go", `package predicate

import (
	"github.com/pbleisch/kept/internal/gather"
	"github.com/pbleisch/kept/internal/model"
)

type Gathered struct{ Of model.Artifact }

func Read() { gather.Kinds() }
`)
	m.holds(t, id, "not-kept", "the predicate reaching the catalogue")
}

// Every citation in the corpus resolves — which is what makes "every policy
// cites a decision that exists" a promise that can fail.
func TestEveryCitationResolvesNoticesAPolicyCitingADecisionThatIsNotThere(t *testing.T) {
	const id = "promise:every-citation-in-the-corpus-resolves"
	m := newSelfModule(t, "every-citation-in-the-corpus-resolves")
	m.putDecision("0030-a-link-manifest-is-one-file-per-subject")
	m.putDecision("0002-artifacts-are-wiki-entities")
	m.putDecision("0001-a-thing-was-decided")
	m.put("docs/policy/a-rule-about-acting.md",
		"---\nid: a-rule-about-acting\ntype: policy\ntitle: A rule about acting\nfields:\n"+
			"  statement: an act is checked before it happens\n  applies_to: land\n  on_fail: refuse\n"+
			"links:\n  - { target: decision:0001-a-thing-was-decided, relation: cites }\n---\n\nWhy.\n")
	m.holds(t, id, "kept", "a policy citing a decision that is there")

	m.put("docs/policy/a-rule-about-acting.md",
		"---\nid: a-rule-about-acting\ntype: policy\ntitle: A rule about acting\nfields:\n"+
			"  statement: an act is checked before it happens\n  applies_to: land\n  on_fail: refuse\n"+
			"links:\n  - { target: decision:0002-a-decision-nobody-wrote, relation: cites }\n---\n\nWhy.\n")
	m.holds(t, id, "not-kept", "a policy citing a decision nobody wrote")
}

// The verdict does not read the agent. Reachability for the package that holds
// the adapters and the receipt, and names for the records a receipt is made of,
// because those are the ledger's and the tick reaches the ledger by
// construction.
func TestTheVerdictDoesNotReadTheAgentNoticesTheEdgeThatArrivesThroughAThirdPackage(t *testing.T) {
	const id = "promise:the-verdict-does-not-read-the-agent"
	m := newSelfModule(t, "the-verdict-does-not-read-the-agent")
	m.holds(t, id, "kept", "the module as it is, with repair reaching both sides")

	// The edge the other way is not this boundary: a thread may lean on what a
	// predicate is, and the assessor reading a transcript is what it is for.
	m.put("internal/thread/packet.go", `package thread

import (
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/predicate"
)

type Packet struct{ bytes []byte }

func AssemblePacket(g predicate.Gathered) (Packet, error) { return Packet{}, nil }

func Transcript() ledger.Kind { return ledger.ThreadTranscript }
`)
	m.put("internal/assess/assess.go", `package assess

import (
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/thread"
)

func Read() ledger.Kind { return thread.Transcript() }
`)
	m.holds(t, id, "kept", "the thread reaching the predicate and the assessor reading a transcript")

	// Two hops, through a package the tick has every reason to import.
	m.put("internal/register/register.go", `package register

import (
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/thread"
)

func stands(k ledger.Kind) bool { return k == ledger.Ratified }

func Said() ledger.Kind { return thread.Transcript() }
`)
	m.put("internal/tick/tick.go", `package tick

import (
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/register"
)

type Tick struct{ Ledger *ledger.Ledger }

func (t *Tick) Run(root string) { register.Said() }
`)
	m.holds(t, id, "not-kept", "the tick reaching the thread through the register")
}

func TestTheVerdictDoesNotReadTheAgentNoticesThePredicateReachingTheThread(t *testing.T) {
	const id = "promise:the-verdict-does-not-read-the-agent"
	m := newSelfModule(t, "the-verdict-does-not-read-the-agent")
	m.holds(t, id, "kept", "the module as it is")

	m.put("internal/predicate/eval.go", `package predicate

import (
	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/thread"
)

type Gathered struct{ Of model.Artifact }

func Said() { thread.Transcript() }
`)
	m.holds(t, id, "not-kept", "the predicate importing the thread")
}

func TestTheVerdictDoesNotReadTheAgentNoticesATranscriptNamedWhereAVerdictIsDecided(t *testing.T) {
	const id = "promise:the-verdict-does-not-read-the-agent"
	m := newSelfModule(t, "the-verdict-does-not-read-the-agent")
	m.holds(t, id, "kept", "the module as it is")

	// The ledger's other records are the tick's business, and naming the
	// receipt's in prose is prose.
	m.put("internal/tick/tick.go", `package tick

import (
	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/gather"
	"github.com/pbleisch/kept/internal/ledger"
)

type Tick struct{ Ledger *ledger.Ledger }

// Run never reads ledger.ThreadTranscript, and saying so is a comment.
func (t *Tick) Run(root string) { corpus.Load(root); gather.Kinds() }

func (t *Tick) Dispose() ledger.Disposition { return ledger.Disposition{Condition: ledger.Rank} }

func (t *Tick) Observed() ledger.Observation { return ledger.Observation{} }
`)
	m.holds(t, id, "kept", "the tick naming an observation, a disposition and a comment")

	// A receipt record named in the tick, with no edge to the thread at all.
	m.put("internal/tick/tick.go", `package tick

import "github.com/pbleisch/kept/internal/ledger"

type Tick struct{ Ledger *ledger.Ledger }

func (t *Tick) Repaired() bool { return ledger.ThreadTranscript != "" }
`)
	m.holds(t, id, "not-kept", "the tick naming a thread's transcript")
}

// The pipeline cannot reach a packet. The package that assembles one names none
// of the queue and does not reach what decides it, and assembly stays there.
func TestThePipelineCannotReachAPacketNoticesAQueueFieldInTheInput(t *testing.T) {
	const id = "promise:the-pipeline-cannot-reach-a-packet"
	m := newSelfModule(t, "the-pipeline-cannot-reach-a-packet")
	m.holds(t, id, "kept", "the module as it is, with repair recording a disposition")

	// More evidence in the input is not the queue, and naming the queue in
	// prose is prose.
	m.put("internal/thread/packet.go", `package thread

import (
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/model"
)

// PacketInput has no Disposition, no Rank and no KindDesire, on purpose.
type PacketInput struct {
	Promise      model.Artifact
	Gather       model.GatherSpec
	Why          string
	Observations map[string]ledger.Observation
}

type Packet struct{ bytes []byte }

func AssemblePacket(in PacketInput) (Packet, error) { return Packet{}, nil }

func Transcript() ledger.Kind { return ledger.ThreadTranscript }
`)
	m.holds(t, id, "kept", "the input carrying a gather spec and a comment naming the queue")

	m.put("internal/thread/packet.go", `package thread

import (
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/model"
)

type PacketInput struct {
	Promise      model.Artifact
	Why          string
	Observations map[string]ledger.Observation
	Queue        []ledger.Disposition
}

type Packet struct{ bytes []byte }

func AssemblePacket(in PacketInput) (Packet, error) { return Packet{}, nil }
`)
	m.holds(t, id, "not-kept", "a disposition field in the packet's input")
}

func TestThePipelineCannotReachAPacketNoticesARankADesireAndTheTick(t *testing.T) {
	const id = "promise:the-pipeline-cannot-reach-a-packet"
	breaks := map[string]string{
		"a rank named where a packet is assembled": `package thread

import "github.com/pbleisch/kept/internal/ledger"

type Packet struct{ bytes []byte }

func AssemblePacket(below bool) (Packet, error) { _ = ledger.Rank; return Packet{}, nil }
`,
		"a desire named where a packet is assembled": `package thread

import "github.com/pbleisch/kept/internal/model"

type Packet struct{ bytes []byte }

func AssemblePacket(k model.Kind) (Packet, error) { _ = k == model.KindDesire; return Packet{}, nil }
`,
		// Two hops, through a package the thread has every reason to import.
		"the thread reaching the tick through the box": `package thread

import "github.com/pbleisch/kept/internal/box"

type Packet struct{ bytes []byte }

func AssemblePacket(root string) (Packet, error) { box.Cut(root); return Packet{}, nil }
`,
	}
	for after, src := range breaks {
		t.Run(after, func(t *testing.T) {
			m := newSelfModule(t, "the-pipeline-cannot-reach-a-packet")
			m.put("internal/box/box.go", `package box

func Cut(root string) {}
`)
			m.holds(t, id, "kept", "the module as it is, with a box beside it")

			if after == "the thread reaching the tick through the box" {
				m.put("internal/box/box.go", `package box

import "github.com/pbleisch/kept/internal/tick"

func Cut(root string) { (&tick.Tick{}).Run(root) }
`)
			}
			m.put("internal/thread/packet.go", src)
			m.holds(t, id, "not-kept", after)
		})
	}
}

func TestThePipelineCannotReachAPacketNoticesAssemblyMovingOutOfTheThread(t *testing.T) {
	const id = "promise:the-pipeline-cannot-reach-a-packet"
	m := newSelfModule(t, "the-pipeline-cannot-reach-a-packet")
	m.holds(t, id, "kept", "the module as it is")

	// The thread still names nothing from the queue, and the packet is now made
	// where it can: a promise about the thread would hold over a boundary with
	// nothing behind it.
	m.put("internal/thread/packet.go", `package thread

import "github.com/pbleisch/kept/internal/ledger"

type Packet struct{ Bytes []byte }

func Transcript() ledger.Kind { return ledger.ThreadTranscript }
`)
	m.put("internal/repair/perform.go", `package repair

import (
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/thread"
	"github.com/pbleisch/kept/internal/tick"
)

func AssemblePacket(d ledger.Disposition) thread.Packet { return thread.Packet{} }

func Perform(t *tick.Tick) { AssemblePacket(t.Dispose()) }
`)
	m.holds(t, id, "not-kept", "packet assembly moving into the repair")
}

// A tree that does not compile is the tree a repair agent stands in, so a
// boundary promise has to go on answering over one — and a tree that does not
// parse is one it cannot answer about at all.
func TestTheTwoPacketBoundariesAnswerOverATypeErrorAndAreUnknownOverASyntaxError(t *testing.T) {
	slugs := []string{"the-verdict-does-not-read-the-agent", "the-pipeline-cannot-reach-a-packet"}
	m := newSelfModule(t, slugs...)

	// Undefined names and a mistyped return: no compiler accepts this, and the
	// imports and references are still exactly what they were.
	m.put("internal/thread/packet.go", `package thread

import (
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/model"
)

type PacketInput struct {
	Promise      model.Artifact
	Observations map[string]ledger.Observation
}

func AssemblePacket(in PacketInput) (Packet, error) { return nowhere(in), 42 }

func Transcript() ledger.Kind { return ledger.ThreadTranscript }
`)
	for _, slug := range slugs {
		m.holds(t, "promise:"+slug, "kept", "a type error in the thread")
	}

	m.put("internal/thread/packet.go", `package thread

import "github.com/pbleisch/kept/internal/ledger"

func AssemblePacket(in PacketInput) (Packet, error) { return {
`)
	for _, slug := range slugs {
		m.holds(t, "promise:"+slug, "unknown", "a syntax error in the thread")
	}
}

// A failed gatherer yields unknown, never not-kept. Six promises, one machine
// that cannot measure: without go.mod nothing says what these directories'
// import paths are, so there is no package graph to be right or wrong about.
func TestAStructuralPromiseWhoseGathererCannotRunIsUnknownAndNeverNotKept(t *testing.T) {
	slugs := []string{
		"standing-is-minted-in-one-place",
		"the-corpus-cannot-write-standing-or-truth",
		"gatherers-do-not-write",
		"the-core-carries-no-domain-vocabulary",
		"the-verdict-does-not-read-the-agent",
		"the-pipeline-cannot-reach-a-packet",
	}
	m := newSelfModule(t, slugs...)
	for _, slug := range slugs {
		m.holds(t, "promise:"+slug, "kept", "the module as it is")
	}

	if err := os.Remove(filepath.Join(m.root, "go.mod")); err != nil {
		t.Fatal(err)
	}
	for _, slug := range slugs {
		m.holds(t, "promise:"+slug, "unknown", "the module line going away")
	}
}

// And the same for the corpus gatherer, whose failure is a root that will not
// load. The promise names a root relative to the workspace, so a workspace
// without one is a machine that could not measure rather than a corpus with
// nothing wrong in it.
func TestTheCitationPromiseIsUnknownWhenThereIsNoCorpusToRead(t *testing.T) {
	const id = "promise:every-citation-in-the-corpus-resolves"
	m := newSelfModule(t, "every-citation-in-the-corpus-resolves")
	m.putDecision("0030-a-link-manifest-is-one-file-per-subject")
	m.putDecision("0002-artifacts-are-wiki-entities")
	m.holds(t, id, "kept", "the corpus as it is")

	// The corpus moves somewhere the promise's declared root does not name.
	elsewhere := filepath.Join(m.root, "manuscript")
	if err := os.Rename(filepath.Join(m.root, "docs"), elsewhere); err != nil {
		t.Fatal(err)
	}
	m.put(".kept/config.yaml", "corpus: manuscript\n")

	if got := m.verdict(t, id); got != "unknown" {
		t.Fatalf("a corpus root that is not there reported %s, and a machine that could not measure does not record what it thinks it saw", got)
	}
}
