package repair

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/pbleisch/kept/internal/box"
	"github.com/pbleisch/kept/internal/config"
	"github.com/pbleisch/kept/internal/confine"
	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/gather"
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/model"
	"github.com/pbleisch/kept/internal/register"
	"github.com/pbleisch/kept/internal/thread"
	"github.com/pbleisch/kept/internal/tick"
	"github.com/pbleisch/kept/internal/workspace"
)

// canaryDeadline bounds verifying the provider, which is the pre-flight and the
// canary together.
//
// It is not a configuration key and should not become one. What it bounds is
// kept asking a provider about a policy kept just rendered — the same work for
// every repair on a machine, with no per-repository answer — and a knob for it
// would be a knob for how long to wait before deciding the jail is broken. The
// deadlines an operator does set bound the things that vary: the gatherer, the
// agent, and Git. See docs/decision/0026.
const canaryDeadline = 2 * time.Minute

// dispatchMargin is how much longer the parent's own context runs than the
// deadline it gives the session. The supervisor's timeout is what kills the
// child; this is only so the outer context cannot cancel the kill, the reap and
// the final receipt appends it has to perform afterwards.
const dispatchMargin = 2 * time.Minute

// Status is how far one repair got.
//
// The six that are not a completed dispatch are as much a result as the one
// that is, and each sends a reader somewhere different. A repair that was
// refused, one that found nothing to repair and one that ran and did not work
// look identical to a boolean, and telling them apart is the whole of what a
// person asked for.
type Status string

const (
	// Repaired: a session ran, and re-deriving the promise from the box says it
	// holds there. The verdict is the re-derivation's and never the session's.
	Repaired Status = "repaired"
	// Unrepaired: a session ran and the promise still does not hold.
	Unrepaired Status = "unrepaired"
	// Undecided: a session ran and nothing could measure the promise afterwards.
	Undecided Status = "undecided"
	// Holds: the promise holds at the commit the box was cut from, so there is
	// no gap for a packet to state. Drift in the working tree that is not in the
	// commit reads like this, and so does a verdict nothing has refreshed.
	Holds Status = "holds"
	// Unmeasured: nothing could measure the promise in the box, so there is no
	// evidence to carry. A failed gather is unknown and never not-kept, one
	// level up as well as at the instrument.
	Unmeasured Status = "unmeasured"
	// Refused: a condition refused, and Condition names which. Nothing started.
	Refused Status = "refused"
	// Prepared: the dispatch was not asked for. Everything up to it happened,
	// including the canary and the pre-flight.
	Prepared Status = "prepared"
)

// Deadlines are what .kept/config.yaml says an external call may take. They are
// carried as a struct rather than read here so that this package has exactly one
// source for them and the command line has the other half of the same file. See
// docs/decision/0026.
type Deadlines struct {
	Gatherer time.Duration
	Agent    time.Duration
	Git      time.Duration
}

// Request is one repair a person asked for, by name.
//
// One promise, named. Choosing which of several drifts to spend a live session
// on is a ranking and a budget, and neither exists; until they do the cut is a
// person naming one. See docs/decision/0060.
type Request struct {
	// Workspace is the repository root the box is cut from, and Corpus is what
	// was read out of it.
	Workspace string
	Corpus    *corpus.Corpus
	// Acts is the folded act stream standing is derived from.
	Acts []ledger.Act
	// Promise is the artifact id, already resolved to its "type:slug" form.
	Promise string

	// Provider is .kept/config.yaml's confine.provider, and Operator is the
	// per-machine repair runtime. The two halves are deliberately in different
	// files: one is the repository's policy and the other names one person's
	// executable and one person's credential. See docs/decision/0048.
	Provider  string
	Operator  config.Repair
	Deadlines Deadlines
	Exclude   []string

	// Store and Ledger are where the thread receipt and the disposition go.
	Store  ledger.Store
	Ledger *ledger.Ledger

	// Keep retains the box after the repair instead of discarding it, for a
	// person who wants to work in the tree rather than read the diff.
	Keep bool
	// DryRun stops after the canary and the pre-flight, with the packet
	// assembled and nothing dispatched.
	DryRun bool
}

// Result is one repair, whole: what kept did, what it recorded about it, and
// what a person reviews.
type Result struct {
	Promise string
	Status  Status

	// Outcome, Condition and Detail are what docs/decision/0019 records about
	// this drift, and Recorded reports whether the record was appended — the
	// same refusal on the same grounds is one record and a heartbeat.
	Outcome     ledger.Outcome
	Condition   ledger.Condition
	Detail      string
	Disposition ledger.Disposition
	Recorded    bool

	// Commit is what the box was cut at, Box is where it is when it was kept,
	// and Grants is the write grant the repair scope was rendered as. The last
	// is reported because it is always at least as wide as the scope, and a
	// person reading a bounded review should be able to see what confinement
	// actually permitted. See docs/decision/0059.
	Commit string
	Box    string
	Grants []string

	Packet thread.Packet
	Thread thread.ID

	// Before and After are the promise re-derived in the box, on either side of
	// the session. Neither reads anything the session reported.
	Before tick.Answer
	After  tick.Answer

	Change box.Change
	Review box.Review

	// Warning is something that went wrong after the result was decided and
	// does not change it — a box that would not discard. It is printed and
	// never recorded: a disposition is about the drift.
	Warning string
}

// Dispatched reports whether a session ran.
func (r *Result) Dispatched() bool {
	return r.Status == Repaired || r.Status == Unrepaired || r.Status == Undecided
}

// Unrepairable is a promise that cannot be repaired as it is authored: it has no
// standing, it bounds nothing, or a packet cannot state its gap. It is separate
// from a refusal because nothing about the machine or the moment would change
// it, so there is nothing about it worth writing into the ledger.
type Unrepairable struct{ Why string }

func (e *Unrepairable) Error() string { return e.Why }

// Perform runs one repair end to end: cut a box at the workspace's commit,
// measure the promise there, assemble the packet, dispatch a confined session,
// re-derive the promise from the box, and review what changed against the scope
// the promise bound.
//
// It lands nothing. A repair produces a bounded diff a person reads, and the
// merge is their hands on git. See docs/increments.md.
//
// The order is deliberate. The gap is established in the box before anything is
// recorded and before a session starts, because a packet states a gap
// (docs/decision/0009) and a promise that holds at the commit has none — and
// because a disposition about a drift kept never established would be a record
// of a decision nobody made.
func Perform(ctx context.Context, req Request) (*Result, error) {
	res := &Result{Promise: req.Promise}
	entry, standing, err := subject(req)
	if err != nil {
		return nil, err
	}
	art := entry.Artifact

	provider, err := providerExecutable(req.Provider)
	if err != nil {
		return refuse(req, res, standing, ledger.Provider, err.Error())
	}
	if why := runtimeUnready(req.Operator); why != "" {
		return refuse(req, res, standing, ledger.Runtime, why)
	}
	if req.Deadlines.Agent <= 0 {
		// Nothing external runs without a deadline, so with none there is
		// nothing to run. It is asked here rather than at the launch because
		// the answer is in a file and does not need a box and a canary first.
		// See docs/decision/0026.
		return refuse(req, res, standing, ledger.Runtime,
			"timeouts.agent is unset in .kept/config.yaml, and nothing external runs without a deadline")
	}

	created, home, err := cut(ctx, req)
	if err != nil {
		return nil, err
	}
	discard := func() {
		// The session's private state goes either way: it is the runtime's
		// home and the box's scratch, both discarded with the session, and
		// neither is part of the tree a person keeps a box to read.
		_ = os.RemoveAll(home)
		if req.Keep {
			_ = os.RemoveAll(created.Temp())
			res.Box = created.Path()
			return
		}
		if err := created.Close(context.Background()); err != nil {
			// A box that will not close is worth saying out loud rather than
			// swallowing: what is left behind is a registered worktree in the
			// operator's repository.
			res.Warning = fmt.Sprintf("the box at %s could not be discarded, and is a registered worktree in this repository: %s", created.Path(), err)
		}
	}
	defer discard()
	res.Commit = created.Commit()

	before, observation, err := derive(ctx, req, created.Path())
	if err != nil {
		return nil, err
	}
	res.Before = before
	switch before.Value {
	case model.Kept:
		res.Status = Holds
		return res, nil
	case model.NotKept:
	default:
		// A gatherer that could not run in the box is not a broken promise, and
		// there is nothing for a packet to state. See
		// policy:a-failed-gather-is-unknown.
		res.Status = Unmeasured
		return res, nil
	}

	packet, err := assemblePacket(req, entry, standing, observation)
	if err != nil {
		return nil, err
	}
	res.Packet = packet

	assembled, err := Assemble(ctx, req.Operator, Session{Home: home, Temp: created.Temp()})
	if err != nil {
		return refuse(req, res, standing, ledger.Runtime, err.Error())
	}
	profile, err := confinement(ctx, req, created, home, provider, assembled, art)
	if err != nil {
		return refuse(req, res, standing, ledger.Provider, err.Error())
	}
	res.Grants = append([]string(nil), profile.Write...)
	launch, err := confine.NewNono(profile)
	if err != nil {
		return refuse(req, res, standing, ledger.Provider, err.Error())
	}
	// The canary and the pre-flight, before a session exists and before a dry
	// run reports that everything is ready. A provider is verified rather than
	// trusted by name, and a dry run that skipped this would report readiness it
	// had not established. See docs/decision/0017 and 0053.
	if err := launch.Verified(); err != nil {
		return refuse(req, res, standing, ledger.Provider, err.Error())
	}
	if req.DryRun {
		res.Status = Prepared
		return res, nil
	}

	receipt, err := thread.NewReceipt(req.Ledger, thread.MintID())
	if err != nil {
		return nil, err
	}
	res.Thread = receipt.ID()
	deadline, cancel := context.WithTimeout(ctx, req.Deadlines.Agent+dispatchMargin)
	defer cancel()
	if err := thread.Dispatch(deadline, assembled.Config, packet, launch, thread.Supervisor{Timeout: req.Deadlines.Agent}, receipt); err != nil {
		var refusal *thread.Refusal
		if errors.As(err, &refusal) {
			// The canary is already verified, so a refusal here is the
			// adapter's: an unadmitted runtime, a version it is not pinned
			// against, or a credential that has lapsed.
			return refuse(req, res, standing, ledger.Runtime, refusal.Error())
		}
		return nil, err
	}

	// A thread opened, which is what `act` records, and it is recorded now
	// rather than after the review: everything below can fail, and a session
	// that ran and cost a credential must be in the record either way. Record
	// only what has already happened, and this has. See docs/decision/0018.
	if err := record(req, res, standing, ledger.OutcomeAct, "",
		fmt.Sprintf("thread %s: a confined %s session ran in a box under %s", receipt.ID(), req.Operator.Runtime, req.Provider)); err != nil {
		return res, err
	}

	// What the session did is evidence about the session. What the promise now
	// says is measured again, from the tree the session left behind, with the
	// instrument the promise names. See
	// policy:the-agents-report-is-never-an-input-to-the-verdict.
	after, _, err := derive(ctx, req, created.Path())
	if err != nil {
		return res, err
	}
	res.After = after
	switch after.Value {
	case model.Kept:
		res.Status = Repaired
	case model.NotKept:
		res.Status = Unrepaired
	default:
		res.Status = Undecided
	}

	changed, err := created.Change(ctx)
	if err != nil {
		// A diff past its bound is one of these, and it is not a repair that
		// did not happen: the thread is recorded, the box is named, and the
		// person is told where to read it.
		res.Warning = err.Error()
		if !req.Keep {
			res.Warning += "; --keep would have left the box to read it in"
		}
		return res, nil
	}
	res.Change = changed
	// The one classification. The report a person reads beside the diff and the
	// check a land refuses on are the same computation, so they cannot disagree.
	// Nothing here parses diff text. See docs/decision/0059.
	res.Review = changed.Against(art.RepairScope)
	return res, nil
}

// subject resolves the promise and its standing, and refuses the artifacts that
// cannot be repaired however the machine is configured.
func subject(req Request) (*corpus.Entry, register.Standing, error) {
	entry, found := req.Corpus.Get(req.Promise)
	if !found {
		return nil, register.Standing{}, &Unrepairable{Why: fmt.Sprintf("the corpus holds no %s", req.Promise)}
	}
	art := entry.Artifact
	if art.Kind != "promise" {
		// Only a promise bounds what a repair may change, so only a promise can
		// authorize one. See docs/decision/0047.
		return nil, register.Standing{}, &Unrepairable{Why: fmt.Sprintf("%s is a %s, and a repair is authorized by a promise", art.ID, art.Kind)}
	}

	var standing register.Standing
	for _, s := range register.Derive(req.Corpus, req.Acts) {
		if s.Artifact == art.ID {
			standing = s
			break
		}
	}
	switch {
	case standing.State != register.Ratified:
		return nil, standing, &Unrepairable{Why: fmt.Sprintf("%s reads %s, and nothing is owed until a person ratifies what it says now", art.ID, standing.State)}
	case len(art.RepairScope) == 0:
		return nil, standing, &Unrepairable{Why: fmt.Sprintf("%s bounds no repair scope, so it authorizes no repair", art.ID)}
	case art.HoldWhen == "" || len(art.Gather) == 0:
		return nil, standing, &Unrepairable{Why: fmt.Sprintf("%s names no instrument, so nothing can state a gap or check that one closed", art.ID)}
	case len(art.Gather) != 1:
		// A packet carries one measurement, and its shape is frozen at
		// docs/decision/0052's version. A promise with two instruments needs a
		// packet that can carry two, which is a version of the packet rather
		// than a judgement made here about which one matters.
		return nil, standing, &Unrepairable{Why: fmt.Sprintf("%s names %d instruments and a packet carries one measurement", art.ID, len(art.Gather))}
	}
	return entry, standing, nil
}

// providerExecutable resolves the named confinement provider to a path.
//
// The name is the repository's policy and the path is this machine's answer to
// it. Resolution happens once, here, and what is rendered into the profile is
// the resolved path: neither the provider nor the canary resolves a command on a
// search path, because a path chosen at each use is not a policy anybody wrote
// down. `unset` is not `none`. See docs/decision/0017.
func providerExecutable(provider string) (string, error) {
	if provider == "" {
		return "", errors.New("no confinement provider is named in .kept/config.yaml, and unset is not none, " +
			"so nothing may be dispatched into a box")
	}
	if provider != nonoProvider {
		// The set is closed and it has one member. A name kept does not know
		// how to render a profile for is refused rather than resolved on a
		// search path and started, because what would be verified then is a
		// program nobody described.
		return "", fmt.Errorf("%q is not a confinement provider kept renders a profile for; the one it does is %q", provider, nonoProvider)
	}
	resolved, err := exec.LookPath(provider)
	if err != nil {
		return "", fmt.Errorf("the confinement provider %q is named in .kept/config.yaml and is not installed", provider)
	}
	return filepath.Abs(resolved)
}

// nonoProvider is the one provider this build renders a profile for. See
// docs/decision/0017 and internal/confine.
const nonoProvider = "nono"

// runtimeUnready is what can be said about the repair runtime before a box
// exists. The rest — the version it reports and the life left in its credential
// — is asked once the gap is established, because both cost an external call
// and neither is worth making about a promise that turns out to hold.
func runtimeUnready(user config.Repair) string {
	switch {
	case user.Runtime == "":
		return "no repair runtime is named in the operator's config, so there is nothing to dispatch"
	case user.Executable == "":
		return fmt.Sprintf("the operator's config names the %s runtime and no executable for it", user.Runtime)
	case !filepath.IsAbs(user.Executable):
		return fmt.Sprintf("the repair runtime executable %q is not an explicit absolute path", user.Executable)
	}
	if _, err := os.Stat(user.Executable); err != nil {
		return fmt.Sprintf("the repair runtime is configured at %s, which is not there", user.Executable)
	}
	return ""
}

// cut creates the box and the session home beside it.
//
// The box is cut at the workspace's own HEAD, with no dirty overlay. What that
// costs is stated rather than hidden: work in the working tree and not in the
// commit is not in the box, so a promise broken only by uncommitted edits reads
// as holding there and nothing is dispatched. The overlay exists for carrying
// selected operator work in and is not used here, because choosing which dirty
// paths a repair inherits is a decision a person makes and nothing asks them.
//
// The home lies beside the box rather than inside it: a runtime's private state
// in the tree would arrive in the repair's diff as though an agent had put it
// there. It is empty, because the adapter refuses a home that is not — a session
// cannot inherit another one's state.
func cut(ctx context.Context, req Request) (*box.Box, string, error) {
	commit, err := head(ctx, req.Workspace, req.Deadlines.Git)
	if err != nil {
		return nil, "", err
	}
	created, err := box.Create(ctx, box.Request{
		Repository: req.Workspace,
		Commit:     commit,
		Deadline:   req.Deadlines.Git,
	})
	if err != nil {
		return nil, "", err
	}
	home := created.Path() + ".home"
	if err := os.MkdirAll(home, 0o700); err != nil {
		_ = created.Close(context.Background())
		return nil, "", fmt.Errorf("create the repair session home: %w", err)
	}
	return created, home, nil
}

// head is the commit the box is pinned to. A box refuses a moving name, so the
// object id is resolved here and the box verifies it exists.
func head(ctx context.Context, workspace string, deadline time.Duration) (string, error) {
	if deadline <= 0 {
		deadline = box.DefaultDeadline
	}
	ctx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()
	git, err := exec.LookPath("git")
	if err != nil {
		return "", errors.New("git is not installed, and a repair box is a worktree")
	}
	out, err := exec.CommandContext(ctx, git, "-C", workspace, "rev-parse", "HEAD").Output()
	if ctx.Err() != nil {
		return "", fmt.Errorf("resolving HEAD in %s timed out after %s", workspace, deadline)
	}
	if err != nil {
		return "", fmt.Errorf("resolve HEAD in %s: %w", workspace, err)
	}
	commit := strings.TrimSpace(string(out))
	if commit == "" {
		return "", fmt.Errorf("%s has no commit to cut a box from", workspace)
	}
	return commit, nil
}

// derive measures one promise where it stands and applies its predicate.
//
// The tree is walked, the world digest is computed over what is actually there,
// the instrument runs and the predicate is applied. Nothing is recorded
// anywhere: a verdict reached in a box belongs to the thread rather than to the
// workspace's truth stream, and this package opens no ledger of its own. See
// docs/decision/0035 and promise:standing-is-minted-in-one-place.
//
// The acts are the operator's, because an act is a person's and a box holds no
// ledger at all. They reach a gatherer only if it declared a read of them.
func derive(ctx context.Context, req Request, root string) (tick.Answer, ledger.Observation, error) {
	corpusRoot, workspaceRoot, err := corpus.FindRoot(root)
	if err != nil {
		return tick.Answer{}, ledger.Observation{}, fmt.Errorf("read the box's corpus: %w", err)
	}
	c, err := corpus.Load(corpusRoot, workspaceRoot, corpus.Options{Gatherers: gather.Builtin()})
	if err != nil {
		return tick.Answer{}, ledger.Observation{}, fmt.Errorf("read the box's corpus: %w", err)
	}

	answer, observations, err := tick.Measure(ctx, tick.MeasureRequest{
		Root:      root,
		Corpus:    c,
		Artifact:  req.Promise,
		Catalogue: gather.Builtin(),
		Deadline:  req.Deadlines.Gatherer,
		Exclude:   req.Exclude,
		Workers:   runtime.NumCPU(),
		Acts:      req.Acts,
	})
	if err != nil {
		return tick.Answer{}, ledger.Observation{}, err
	}
	if len(observations) == 0 {
		return answer, ledger.Observation{}, nil
	}
	return answer, ledger.Measured(observations[0]), nil
}

// assemblePacket freezes the packet from ratified authority and the evidence
// measured in the box.
//
// The observation is the box's, not the workspace's, and that is the point: the
// agent stands in the box, and evidence about a tree it is not in would be
// evidence about somewhere else. See docs/decision/0014.
func assemblePacket(req Request, entry *corpus.Entry, standing register.Standing, observation ledger.Observation) (thread.Packet, error) {
	art := entry.Artifact
	policies, err := dispatchAuthority(req)
	if err != nil {
		return thread.Packet{}, err
	}
	packet, err := thread.AssemblePacket(thread.PacketInput{
		Promise: thread.RatifiedPromise{
			ID:          art.ID,
			Hash:        standing.Hash.Digest,
			Norm:        standing.Hash.Norm,
			Ratified:    true,
			Statement:   art.Statement,
			Gather:      art.Gather,
			HoldWhen:    art.HoldWhen,
			Trigger:     art.Trigger,
			RepairScope: art.RepairScope,
		},
		Why:         art.Body,
		Observation: observation,
		Policies:    policies,
	})
	if err != nil {
		return thread.Packet{}, &Unrepairable{Why: err.Error()}
	}
	return packet, nil
}

// dispatchPolicies is the ratified authority that binds this act, read out of
// the corpus.
//
// Which policies those are is not a judgement made here: a policy declares the
// act it is checked before, and this collects the ones that say dispatch. An
// unratified one refuses the whole packet rather than being dropped from it —
// authority nobody granted is not authority a repair may be handed, and silently
// omitting it would make the packet smaller with nothing saying so.
func dispatchAuthority(req Request) ([]thread.RatifiedDispatchPolicy, error) {
	standings := map[string]register.Standing{}
	for _, s := range register.Derive(req.Corpus, req.Acts) {
		standings[s.Artifact] = s
	}

	var policies []thread.RatifiedDispatchPolicy
	for _, entry := range req.Corpus.All() {
		art := entry.Artifact
		applies, _ := art.Fields["applies_to"].(string)
		if applies != "dispatch" {
			continue
		}
		standing := standings[art.ID]
		if standing.State != register.Ratified {
			return nil, &Unrepairable{Why: fmt.Sprintf("%s binds a dispatch and reads %s, so a packet would carry authority nobody granted", art.ID, standing.State)}
		}
		onFail, _ := art.Fields["on_fail"].(string)
		policies = append(policies, thread.RatifiedDispatchPolicy{
			ID:        art.ID,
			Hash:      standing.Hash.Digest,
			Norm:      standing.Hash.Norm,
			Ratified:  true,
			Statement: art.Statement,
			AppliesTo: applies,
			Gather:    art.Gather,
			HoldWhen:  art.HoldWhen,
			OnFail:    onFail,
		})
	}
	if len(policies) == 0 {
		return nil, &Unrepairable{Why: "no ratified policy in this corpus binds a dispatch, so a packet would carry no authority at all"}
	}
	// Sorted, because a packet assembled twice from one corpus must be the same
	// bytes and map iteration is not an order.
	slices.SortFunc(policies, func(a, b thread.RatifiedDispatchPolicy) int { return strings.Compare(a.ID, b.ID) })
	return policies, nil
}

// confinement renders the profile this repair runs under.
//
// Everything in it is either the box's, the promise's, or something a tool was
// asked about itself. Nothing is a path list kept carries: the runtime is
// located by inspecting it, and a toolchain by running it. See
// docs/decision/0053 and 0056.
func confinement(ctx context.Context, req Request, created *box.Box, home, provider string, assembled Runtime, art *model.Artifact) (confine.Profile, error) {
	kept, err := os.Executable()
	if err != nil {
		return confine.Profile{}, fmt.Errorf("locate the kept executable the canary runs: %w", err)
	}
	kept, err = filepath.Abs(kept)
	if err != nil {
		return confine.Profile{}, err
	}
	git, err := exec.LookPath("git")
	if err != nil {
		return confine.Profile{}, errors.New("git is not installed, and the canary probes a push with it")
	}
	git, err = filepath.Abs(git)
	if err != nil {
		return confine.Profile{}, err
	}

	grants, err := writeGrants(created.Path(), art.RepairScope)
	if err != nil {
		return confine.Profile{}, err
	}

	profile := confine.Profile{
		Nono:  provider,
		Kept:  kept,
		Git:   git,
		Box:   created.Path(),
		Read:  []string{created.Path()},
		Write: grants,
		// The two machine-written tiers, denied by name. In a Git workspace
		// they live on refs/kept/ledger and these paths hold nothing; a box
		// still may not reach where they would be, and a manuscript's do live
		// there. See docs/decision/0016 and 0023.
		Ledger:    filepath.Join(req.Workspace, corpus.ConfigDir, ledger.Dir),
		Artifacts: filepath.Join(req.Workspace, corpus.ConfigDir, "artifacts"),
		Temp:      created.Temp(),
		Home:      home,
		// The one host this runtime's credential is answered by. A box that
		// wanted none would say so; an empty allowlist is the provider's
		// spelling of unrestricted. See docs/decision/0053.
		Domains:       []string{assembled.Domain},
		Env:           assembled.Env,
		TempEnv:       assembled.TempEnv,
		Credential:    assembled.Credential,
		CanaryTimeout: canaryDeadline,
	}

	tools, err := toolchains(ctx, req, art)
	if err != nil {
		return confine.Profile{}, err
	}
	profile.ToolPath = tools.Path
	profile.ToolRead = tools.Read
	profile.ToolWrite = tools.Write
	profile.ToolEnv = tools.Env
	return profile, nil
}

// systemToolPath is what every box gets on its search path beyond what a
// resolver found. It grants nothing — a search path is resolution and not
// containment — and the provider's own baseline is what reaches these.
var systemToolPath = []string{"/usr/bin", "/bin"}

// toolchains is what this box can run: the repair runtime, and the toolchain the
// promise's own instrument names.
//
// What decides that a repair needs Go is the promise, and it already says so:
// docs/decision/0051 put the requirement in the instrument's name, so `go/tests`
// means the Go toolchain and `corpus` means none. No workspace configuration is
// added and no second per-promise field exists. See docs/decision/0053.
func toolchains(ctx context.Context, req Request, art *model.Artifact) (confine.Toolchain, error) {
	native, err := runtimeToolchain(req.Operator)
	if err != nil {
		return confine.Toolchain{}, err
	}
	merged := confine.Toolchain{
		Path:  append(append([]string(nil), native.Path...), systemToolPath...),
		Read:  append([]string(nil), native.Read...),
		Write: append([]string(nil), native.Write...),
		Env:   map[string]string{},
	}
	for name, value := range native.Env {
		merged.Env[name] = value
	}

	if !needsGo(art) {
		return merged, nil
	}
	executable, err := exec.LookPath("go")
	if err != nil {
		return confine.Toolchain{}, fmt.Errorf("%s measures with a go/ instrument and this machine has no go to resolve", art.ID)
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return confine.Toolchain{}, err
	}
	deadline := req.Deadlines.Git
	if deadline <= 0 {
		deadline = box.DefaultDeadline
	}
	toolchain, err := confine.ResolveGo(ctx, executable, deadline)
	if err != nil {
		return confine.Toolchain{}, err
	}
	merged.Path = append(merged.Path, toolchain.Path...)
	merged.Read = append(merged.Read, toolchain.Read...)
	merged.Write = append(merged.Write, toolchain.Write...)
	for name, value := range toolchain.Env {
		merged.Env[name] = value
	}
	merged.Path = dedupe(merged.Path)
	merged.Read = dedupe(merged.Read)
	merged.Write = dedupe(merged.Write)
	return merged, nil
}

func needsGo(art *model.Artifact) bool {
	for _, spec := range art.Gather {
		if strings.HasPrefix(spec.Gatherer, "go/") {
			return true
		}
	}
	return false
}

// runtimeToolchain locates the repair runtime the way its own shape requires.
//
// A native image and a Node-hosted script are resolved differently and the wrong
// one succeeds rather than failing: resolving a self-contained Mach-O image as
// though it were hosted returns the Node installation root, which is a grant for
// a tree the runtime never opens. See docs/decision/0056.
func runtimeToolchain(user config.Repair) (confine.Toolchain, error) {
	if thread.Runtime(user.Runtime) != thread.Claude {
		return confine.Toolchain{}, fmt.Errorf("no confinement is described for the %s runtime", user.Runtime)
	}
	return confine.ResolveNative(user.Executable)
}

// writeGrants is the confinement grant a repair scope becomes.
//
// A scope is a glob and a filesystem grant is a path, so what is rendered is
// always at least as wide as the bound it stands for: `internal/gather/**/*.go`
// grants `internal/gather`, and there is no finer expression of it. A scope
// whose first segment is already a wildcard grants the box root, which is the
// whole tree.
//
// That width is why the review is what reports and what a land refuses on.
// Confinement is not the enforcement of a repair scope and never was — measured
// in a real box, a write the grant admits and the glob does not lands, and so
// does a delete. See docs/decision/0059.
func writeGrants(root string, scope []string) ([]string, error) {
	var grants []string
	for _, pattern := range scope {
		literal, err := literalPrefix(pattern)
		if err != nil {
			return nil, err
		}
		if literal == "" {
			return []string{root}, nil
		}
		grants = append(grants, filepath.Join(root, filepath.FromSlash(literal)))
	}
	if len(grants) == 0 {
		// A promise with no scope authorizes no repair, and this is unreachable
		// because the promise was refused before a box was cut. It is here so
		// that a future caller cannot render an empty grant as though a scope
		// covering nothing covered everything.
		return nil, errors.New("a repair scope naming nothing renders no confinement grant")
	}
	return coarsest(grants), nil
}

// literalPrefix is the leading run of a glob's segments that carries no
// wildcard. A pattern with no wildcard at all names one path and its parent is
// what a repair writes in, because a repair that may replace a file may also
// have to create one beside it.
func literalPrefix(pattern string) (string, error) {
	pattern = strings.TrimPrefix(strings.TrimSpace(pattern), "./")
	if pattern == "" {
		return "", errors.New("a repair scope entry is empty")
	}
	// A trailing separator is the matcher's spelling of a whole subtree, so the
	// directory it names is the bound rather than the last segment before one.
	subtree := strings.HasSuffix(pattern, "/")
	var literal []string
	glob := false
	for _, segment := range strings.Split(strings.TrimSuffix(pattern, "/"), "/") {
		if segment == ".." || segment == "" {
			return "", fmt.Errorf("a repair scope entry may not leave the workspace: %s", pattern)
		}
		if strings.Contains(segment, "*") {
			glob = true
			break
		}
		literal = append(literal, segment)
	}
	if !glob && !subtree && len(literal) > 0 {
		literal = literal[:len(literal)-1]
	}
	return strings.Join(literal, "/"), nil
}

// coarsest drops any grant another grant already contains, so the profile does
// not name the same tree twice under two spellings.
func coarsest(paths []string) []string {
	paths = dedupe(paths)
	var out []string
	for _, path := range paths {
		covered := false
		for _, other := range paths {
			if other != path && strings.HasPrefix(path, other+string(filepath.Separator)) {
				covered = true
				break
			}
		}
		if !covered {
			out = append(out, path)
		}
	}
	return out
}

func dedupe(paths []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, path := range paths {
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

// refuse records a refusal and returns it as the result.
func refuse(req Request, res *Result, standing register.Standing, condition ledger.Condition, detail string) (*Result, error) {
	res.Status = Refused
	if err := record(req, res, standing, ledger.OutcomeRefused, condition, detail); err != nil {
		return res, err
	}
	return res, nil
}

// record appends what this run decided about the drift, and only what has
// already happened.
//
// The world is the workspace's, because this run stands in the workspace: the
// evidence was measured in a box and that is the thread receipt's business, and
// a disposition naming a box's world would say the decision was reached
// somewhere kept was not. See docs/decision/0019 and 0035.
func record(req Request, res *Result, standing register.Standing, outcome ledger.Outcome, condition ledger.Condition, detail string) error {
	res.Outcome, res.Condition, res.Detail = outcome, condition, detail
	if req.Ledger == nil {
		return nil
	}

	tree, err := workspace.Read(req.Workspace, workspace.Options{Exclude: req.Exclude, Workers: runtime.NumCPU()})
	if err != nil {
		return err
	}
	previous, _, err := ledger.ReadDispositions(req.Store, res.Promise)
	if err != nil {
		return err
	}
	var maxAge time.Duration
	if entry, found := req.Corpus.Get(res.Promise); found {
		maxAge = entry.Artifact.MaxAge
	}

	written, recorded, err := req.Ledger.DisposeMoved(ledger.Disposition{
		Artifact:  res.Promise,
		Hash:      standing.Hash.String(),
		Norm:      standing.Hash.Norm,
		World:     model.World{Tree: tree.Digest()},
		Outcome:   outcome,
		Condition: condition,
		Detail:    detail,
	}, previous, maxAge)
	if err != nil {
		return err
	}
	res.Disposition, res.Recorded = written, recorded
	return nil
}
