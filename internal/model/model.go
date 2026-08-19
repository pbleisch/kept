// Package model holds the types the whole system agrees on: what an artifact
// is, and the two properties that may attach to one.
//
// The rule the types exist to enforce is that standing and truth are not
// fields on an artifact. An Artifact is content. Standing and Truth are
// derived on read from the ledger, keyed by the artifact's binding hash, so
// editing the file makes both stop applying without anything having to notice
// the edit. See docs/decision/0001.
package model

import "time"

// Kind is the artifact's type. It is a small closed set: the model does not
// need it to tell a promise from a policy (standing, trigger and truth already
// do that), but the corpus needs it for directory placement, schema selection,
// and to keep candidates out of a packet.
type Kind string

const (
	KindPromise  Kind = "promise"  // ratified; a cadence; truth by predicate
	KindPolicy   Kind = "policy"   // ratified; evaluated at the moment of acting
	KindCanon    Kind = "canon"    // ratified; often no gatherer at all
	KindFact     Kind = "fact"     // no standing; truth inherited from provenance
	KindDocument Kind = "document" // neither; force only by being cited
	KindDesire   Kind = "desire"   // neither, and never placed in a packet
)

// Trigger says when an artifact's truth is sought.
type Trigger string

const (
	// TriggerCadence re-derives on a schedule or on a change to its scope.
	TriggerCadence Trigger = "cadence"
	// TriggerOnAct evaluates at the moment something is about to land. This is
	// what makes a policy a policy rather than a promise.
	TriggerOnAct Trigger = "on-act"
	// TriggerOnce belongs to work, and is spent when it fires.
	TriggerOnce Trigger = "once"
	// TriggerNone is legal and common: most canon binds without being checked.
	TriggerNone Trigger = ""
)

// Standing answers "may this oblige anyone?". It is never stored on an
// artifact; it is a ledger record naming a content hash.
type Standing string

const (
	StandingNone      Standing = "none"
	StandingRatified  Standing = "ratified"  // minted by a person, and only by a person
	StandingDelegated Standing = "delegated" // passed to work from a promise lacking truth
)

// Value is the three-valued verdict. Three rather than two because a gatherer
// that could not run must not be reported as a failure: no observation means
// no predicate ran, so the answer is that nobody knows.
type Value string

const (
	Kept    Value = "kept"
	NotKept Value = "not-kept"
	Unknown Value = "unknown"
)

// Artifact is content, and only content. It is one markdown file with YAML
// frontmatter in the corpus.
type Artifact struct {
	// ID is "type:slug", where the slug is authored in frontmatter and never
	// changes. It is not derived from the filename, so renaming a file changes
	// nothing; a file whose name and slug diverge is a diagnostic, not an
	// error. See docs/decision/0021.
	ID    string
	Kind  Kind
	Title string // short; suggests the slug when the CLI mints one

	// Statement is the binding one-liner. For most canon it is the whole of
	// what is promised.
	Statement string
	// Body is the prose beneath the frontmatter — the "why", handed to
	// whoever repairs this. Withholding it is how a worker satisfies a
	// standard without knowing what it stood for.
	Body string

	// Gather names the observations this artifact's predicate reads, keyed by
	// the name the predicate uses. Empty is legal: canon that nothing checks.
	Gather map[string]GatherSpec
	// HoldWhen is the predicate over those observations. Empty alongside an
	// empty Gather means this artifact binds without being evaluated.
	HoldWhen string
	Trigger  Trigger
	// MaxAge is how old this artifact's answer may be before it is due again.
	// Zero means the author named none, and the artifact is due only when the
	// bytes in its scope move.
	//
	// It is the second half of *due* rather than a mechanism of its own: a
	// promise whose answer is too old is due for the same reason as one whose
	// inputs moved. It is also what makes the truth stream's heartbeat
	// terminate, since an observation's timestamp means first-seen and never
	// advances for an unchanging world. See docs/decision/0025 and 0034.
	MaxAge time.Duration

	// RepairScope bounds what a repair may change. A promise has a scope, so
	// work outside it is not covered by the promise and is never landed.
	RepairScope []string

	Links  []Link
	Fields map[string]any // domain fields. Gatherers and predicates read these; the core does not.

	Created, Modified time.Time
}

// GatherSpec is one named entry in an artifact's gather map. Gatherer defaults
// to the map key, so a promise reading one gatherer names it once.
type GatherSpec struct {
	Gatherer string
	Args     map[string]any
}

// RefKind says which of the three things a reference names. Both ends of a link
// take any of them, in every source a link can be written in, because the
// person asserting a link is not always the author of either end. See
// docs/decision/0030.
type RefKind string

const (
	// RefArtifact is "type:slug": a corpus entity, which may have standing and
	// truth. Identity is authored, so renaming its file changes nothing.
	RefArtifact RefKind = "artifact"
	// RefFile is a path inside the workspace — source, config, a fixture, a
	// binary. It is the world (0028), so it has content and only content, and
	// it is named by where it is because there is no authored id to name.
	RefFile RefKind = "file"
	// RefURI is anything outside the workspace. Whether it is reachable is a
	// question for the world and therefore for a gatherer; nothing here asks
	// it.
	RefURI RefKind = "uri"
)

// Link is a typed edge from one reference to another. Citation is how something
// without standing acquires force: canon binds because a promise points at it.
type Link struct {
	// Target is the far end, as authored.
	Target string
	// TargetKind is which of the three Target names. It is read from Target
	// against the types the corpus has, never guessed from what exists.
	TargetKind RefKind
	Relation   string
}

// World identifies the tree a measurement was made against. Scope says which
// paths were looked at; World says where. Without it the thread loop's two
// gathers — one in the box after a repair, one on the origin — are recorded
// identically, and a cache keyed without it serves the second the first's
// answer. See docs/decision/0014.
type World struct {
	Remote string `json:"remote,omitempty"` // "" for a repository with no remote
	Ref    string `json:"ref,omitempty"`    // the checked-out ref, or "" when detached
	Commit string `json:"commit,omitempty"` // the commit HEAD resolves to
	// Tree is what the gatherer actually ran against. It differs from the
	// commit's tree whenever the working tree is dirty, which is the ordinary
	// state of a box mid-repair — so the tree, not the commit, is the identity.
	//
	// It is a content digest over the workspace root honouring ignore rules —
	// internal/workspace computes it — and it is the same digest in a
	// repository, outside one, and inside a box. A git tree oid would have
	// been cheaper and would have made one tree produce two identities
	// depending on whether git happened to be installed. See
	// docs/decision/0033.
	Tree string `json:"tree"`
	// Thread is set when the box this was measured in belongs to a repair,
	// which is what distinguishes a gather inside a thread from the gather on
	// the tree that ships.
	Thread string `json:"thread,omitempty"`
}

// Same reports whether two worlds are the same world.
//
// The tree measured, and the thread whose box it was measured in. Remote, ref
// and commit are provenance — filled in from git where git is present, empty
// where it is not — and an identity that depended on them would change with
// what happens to be installed, which is the split docs/decision/0033 fixes
// World.Tree to prevent.
//
// This is what a truth fold filters by: verdicts reached in different trees are
// about different things and are never folded together, and a fold that forgets
// to filter is silently wrong and looks correct in every single-run test. See
// docs/decision/0014 and 0035.
func (w World) Same(other World) bool {
	return w.Tree == other.Tree && w.Thread == other.Thread
}

// Observation is what a gatherer returned. It is about the world and knows
// nothing about any artifact, which is why one serves many predicates and why
// observations survive a revision that resets an artifact's truth history.
type Observation struct {
	// ID is the content address the ledger files this under, once it has been
	// recorded. It is derived from what was measured, so two runs measuring
	// the same thing produce the same id. See docs/decision/0034.
	ID       string
	Gatherer string // name and version, e.g. "tests@2"
	// Args is what the gatherer was asked for, and it is what binds: the
	// observation cache is keyed by (gatherer, args, world), so two command
	// gathers differing only in what they ran do not collide. The scope a
	// promise named is one of these args — a projection of them rather than a
	// field beside them, which is the relation Statement has to Fields.
	Args map[string]any
	// ScopeDigest is the digest of the paths the promise's globs matched, per
	// docs/decision/0033. It is recorded because it is part of what the
	// gatherer saw, and because it makes the record verifiable by re-hashing
	// its own fields.
	ScopeDigest string
	// World is the tree this was measured against. It is part of the cache key
	// and part of the content address, so two worlds never collide.
	World World
	At    time.Time
	Data  map[string]any
	// Err is set when the gatherer could not run, and it never reaches the
	// ledger: a failed gather leaves no observation at all, which is what makes
	// the verdict Unknown rather than NotKept. An error field on the record
	// would make a failure a kind of observation. See docs/decision/0004 and
	// 0034.
	Err error
}

// Truth is the result of applying a predicate to observations. It is derived
// at read and appended to the ledger; it is never written to the artifact.
type Truth struct {
	ArtifactID string
	// ContentHash is the binding hash the verdict was about. A revision starts
	// a new series: the earlier verdicts concerned a different statement.
	ContentHash string
	// World is the tree the verdict was reached in. A truth series is per
	// (ContentHash, World): verdicts from different trees are about different
	// things and are never folded together.
	World World
	Value Value
	At    time.Time
	// From are the observations the predicate read.
	From []string
	// Detail is the human-facing reason, drawn from the observation.
	Detail string
}

// Ratification is the only way standing is created. It names a content hash,
// so revising the artifact leaves it no longer applying.
type Ratification struct {
	ArtifactID  string
	ContentHash string
	By          string
	At          time.Time
}

// Drift is not stored. It is the query "has standing and lacks truth", and its
// kind is only the way truth-seeking failed.
type Drift struct {
	ArtifactID string
	Kind       DriftKind
	Since      time.Time
	Truth      Truth
}

type DriftKind string

const (
	DriftUnverified DriftKind = "unverified" // never observed
	DriftNotKept    DriftKind = "not-kept"   // observed, predicate false
	DriftRegressed  DriftKind = "regressed"  // false now, true before
	DriftStale      DriftKind = "stale"      // observation older than the freshness rule
	DriftUnknown    DriftKind = "unknown"    // the gatherer could not run
	// DriftConflicted is two standing artifacts that cannot both hold. It is
	// never computed from their verdicts — both failing is not conflict, and
	// both kept may be — but reached by exhausted repair: a repair satisfied its
	// own promise and broke another, was told, and did it again. Both artifacts
	// carry it, because nothing says which of two ratified statements should
	// yield. See docs/decision/0037.
	DriftConflicted DriftKind = "conflicted"
	// DriftAbsent is ratified content that cannot be found. Never inferred to
	// be a release: absence cannot be told apart from a bad merge, a sparse
	// checkout or a misconfigured corpus root. See docs/decision/0021.
	DriftAbsent DriftKind = "absent"
)
