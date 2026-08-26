package ledger

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fetchState is one state a remote and a local ledger can be in when somebody
// runs an ordinary `git fetch`. The property is *exits 0 in every state*, so
// the states are enumerated in one place and both the refspec under test and
// the spellings it replaces are run against all of them.
type fetchState struct {
	name string
	// set puts the repository into the state and returns the value the
	// authoritative ref must still hold afterwards.
	set func(t *testing.T, work string, store *RefStore) string
}

// fetchStates is every state the report enumerated, plus the two nobody had
// written down. A remote with no ledger at all is the state of every repository
// kept has ever adopted; a genuine fork is the state two writing clones reach.
var fetchStates = []fetchState{
	{"remote has no ledger", func(t *testing.T, work string, store *RefStore) string {
		// Nothing has ticked and nothing has been pushed. This is a fresh
		// adoption, and it is the case that exits 128 under a single-ref
		// refspec because the source is absent rather than because anything
		// is wrong.
		return ledgerAt(t, work)
	}},
	{"local ahead of a stale remote", func(t *testing.T, work string, store *RefStore) string {
		appendAct(t, store, `{"act":"pushed"}`)
		fetchGit(t, work, "push", "-q", "origin", Ref+":"+Ref)
		appendAct(t, store, `{"act":"ratified decision:0017"}`)
		appendAct(t, store, `{"act":"ratified decision:0052"}`)
		return ledgerAt(t, work)
	}},
	{"nothing to do", func(t *testing.T, work string, store *RefStore) string {
		appendAct(t, store, `{"act":"first"}`)
		fetchGit(t, work, "push", "-q", "origin", Ref+":"+Ref)
		return ledgerAt(t, work)
	}},
	{"remote ahead", func(t *testing.T, work string, store *RefStore) string {
		appendAct(t, store, `{"act":"first"}`)
		here := ledgerAt(t, work)
		appendAct(t, store, `{"act":"second"}`)
		fetchGit(t, work, "push", "-q", "origin", Ref+":"+Ref)
		fetchGit(t, work, "update-ref", Ref, here)
		return here
	}},
	{"local has no ledger", func(t *testing.T, work string, store *RefStore) string {
		appendAct(t, store, `{"act":"first"}`)
		fetchGit(t, work, "push", "-q", "origin", Ref+":"+Ref)
		fetchGit(t, work, "update-ref", "-d", Ref)
		return ""
	}},
	{"both moved", func(t *testing.T, work string, store *RefStore) string {
		appendAct(t, store, `{"act":"shared"}`)
		shared := ledgerAt(t, work)
		appendAct(t, store, `{"act":"theirs"}`)
		fetchGit(t, work, "push", "-q", "origin", Ref+":"+Ref)
		fetchGit(t, work, "update-ref", Ref, shared)
		appendAct(t, store, `{"act":"ours"}`)
		return ledgerAt(t, work)
	}},
	{"remote rewound its own ledger", func(t *testing.T, work string, store *RefStore) string {
		appendAct(t, store, `{"act":"first"}`)
		here := ledgerAt(t, work)
		appendAct(t, store, `{"act":"second"}`)
		fetchGit(t, work, "push", "-q", "origin", Ref+":"+Ref)
		fetchGit(t, work, "push", "-q", "--force", "origin", here+":"+Ref)
		return ledgerAt(t, work)
	}},
}

// The property, and the whole deliverable. An ordinary `git fetch origin` must
// exit 0 whatever state the remote and the local ledger are in, and must never
// write the ref the durable record lives on.
//
// Both halves matter and they are different failures. A non-zero fetch breaks
// every caller that runs one — measured, a worktree pool acquiring a worktree
// exits 1 and hands out nothing, and a ledger ahead of its remote is this
// system's resting state. A fetch that writes the ledger is how two
// ratifications were erased on 2026-08-22.
//
// The states are not the ones the previous test enumerated. It called three
// cases "every legitimate ledger" and pushed the ledger to origin before each,
// so **a remote that has no ledger at all was not among them** — which is the
// state of every repository kept has ever adopted, and the one that exits 128.
//
// See docs/decision/0062.
func TestAnOrdinaryFetchExitsZeroInEveryStateAndNeverWritesTheLedger(t *testing.T) {
	for _, state := range fetchStates {
		t.Run(state.name, func(t *testing.T) {
			_, work := fetchLab(t)
			store := NewRefStore(work)
			// Configured through kept's own verb, because the constant is only
			// ever a hazard by way of what init writes into somebody's config.
			if got, _, _, err := ConfigureFetch(work); err != nil || got != FetchAdded {
				t.Fatalf("ConfigureFetch = %q, %v; want %q", got, err, FetchAdded)
			}
			want := state.set(t, work, store)
			acts, _ := store.Read("acts.jsonl")

			code, out := fetchOrigin(t, work)
			if code != 0 {
				t.Errorf("git fetch origin exited %d:\n%s", code, out)
			}
			if got := ledgerAt(t, work); got != want {
				t.Fatalf("the fetch moved %s to %q, want %q:\n%s", Ref, got, want, out)
			}
			if want == "" {
				return
			}
			now, err := store.Read("acts.jsonl")
			if err != nil || string(now) != string(acts) {
				t.Fatalf("the fetch changed the acts (%v):\n%s\nwant\n%s", err, now, acts)
			}
		})
	}
}

// The test that breaks the guard. Every state above passes trivially if the
// refspec is not the thing being measured, so the two spellings this replaced
// are run through the same table: each must fail at least one state, and the
// failures must be the two that were reported.
//
// Forced, the loss is silent — exit 0, no output, and the ledger rewound onto a
// value the remote holds. Unforced, the ledger survives and the fetch is
// rejected, which is correct and which breaks every caller.
func TestTheSpellingsThisReplacedFailTheStatesItPasses(t *testing.T) {
	for _, spec := range DirectRefspecs {
		t.Run(spec, func(t *testing.T) {
			var refused, rewound []string
			for _, state := range fetchStates {
				_, work := fetchLab(t)
				store := NewRefStore(work)
				fetchGit(t, work, "config", "--add", "remote.origin.fetch", spec)
				want := state.set(t, work, store)

				code, _ := fetchOrigin(t, work)
				if code != 0 {
					refused = append(refused, state.name)
				}
				if got := ledgerAt(t, work); got != want {
					rewound = append(rewound, state.name)
				}
			}
			if len(refused) == 0 && len(rewound) == 0 {
				t.Fatalf("%s passed every state, so the refspec is not what this measures", spec)
			}
			if spec == ForcedDirectRefspec && len(rewound) == 0 {
				t.Errorf("the forced spelling never rewound the ledger; refused: %v", refused)
			}
			if spec == DirectRefspec && len(refused) == 0 {
				t.Errorf("the unforced spelling never refused a fetch; rewound: %v", rewound)
			}
			// Both spellings must fail the fresh adoption, because their source
			// is a single ref the remote does not have.
			if !contains(refused, "remote has no ledger") {
				t.Errorf("%s did not fail on a remote with no ledger; refused: %v", spec, refused)
			}
			t.Logf("%s refused %v and rewound %v", spec, refused, rewound)
		})
	}
}

// The glob, on its own, because it is the whole of the fresh-adoption fix and
// it reads as decoration. A single-ref refspec whose source the remote does not
// have is fatal; a pattern that matches nothing is a no-op.
func TestASingleRefSourceIsFatalWhenTheRemoteHasNoLedger(t *testing.T) {
	for _, spec := range []struct {
		refspec string
		want    int
	}{
		{ForcedDirectRefspec, 128},
		{DirectRefspec, 128},
		{"+" + Ref + ":" + UpstreamRef, 128},
		{FetchRefspec, 0},
	} {
		_, work := fetchLab(t)
		fetchGit(t, work, "config", "--add", "remote.origin.fetch", spec.refspec)
		code, out := fetchOrigin(t, work)
		if code != spec.want {
			t.Errorf("git fetch with %s exited %d, want %d:\n%s", spec.refspec, code, spec.want, out)
		}
	}
}

// Where a fetch puts what it brought, which is the other half of the property.
// Exiting 0 while writing nothing anywhere would be a refspec that fetches
// nothing, and then a clone would read a ledger that has never been used.
func TestAFetchWritesTheUpstreamRefAndTheLedgerIsAdvancedOntoIt(t *testing.T) {
	_, work := fetchLab(t)
	store := NewRefStore(work)
	if _, _, _, err := ConfigureFetch(work); err != nil {
		t.Fatal(err)
	}
	appendAct(t, store, `{"act":"first"}`)
	appendAct(t, store, `{"act":"second"}`)
	full := ledgerAt(t, work)
	fetchGit(t, work, "push", "-q", "origin", Ref+":"+Ref)

	// A clone: the upstream ref arrives and the ledger does not.
	fetchGit(t, work, "update-ref", "-d", Ref)
	fetchGit(t, work, "update-ref", "-d", UpstreamRef)
	if code, out := fetchOrigin(t, work); code != 0 {
		t.Fatalf("fetch into a repository with no ledger exited %d:\n%s", code, out)
	}
	if got := ledgerAt(t, work); got != "" {
		t.Fatalf("the fetch created %s at %s; the network must not write it", Ref, got)
	}
	if got := refAt(t, work, UpstreamRef); got != full {
		t.Fatalf("%s = %q, want %q", UpstreamRef, got, full)
	}

	// And kept advances onto it, which is what makes the clone usable.
	switch what, d, err := Advance(store); {
	case err != nil:
		t.Fatal(err)
	case what != SyncAdvanced:
		t.Fatalf("Advance into an empty clone = %q, want %q", what, SyncAdvanced)
	case d.Behind != 2:
		t.Fatalf("Advance reported %d commits behind, want 2", d.Behind)
	}
	if got := ledgerAt(t, work); got != full {
		t.Fatalf("%s = %q after Advance, want %q", Ref, got, full)
	}
	if acts, err := store.Read("acts.jsonl"); err != nil || strings.Count(string(acts), "\n") != 2 {
		t.Fatalf("the advanced ledger holds %q (%v), want two acts", acts, err)
	}
}

// The trap in the middle, and why ConfigureFetch removes rather than adds. A
// clone adopted under either direct spelling already carries it, and a build
// that only added the tracking one beside it would leave a config whose
// behavior turns on which line git reads first.
func TestAddingTheTrackingRefspecBesideADirectOneWouldNotDisarmIt(t *testing.T) {
	for _, spec := range DirectRefspecs {
		t.Run(spec, func(t *testing.T) {
			_, work := fetchLab(t)
			store := NewRefStore(work)
			appendAct(t, store, `{"act":"pushed"}`)
			fetchGit(t, work, "push", "-q", "origin", Ref+":"+Ref)
			appendAct(t, store, `{"act":"unpushed"}`)
			ahead := ledgerAt(t, work)

			// What a clone looks like today, plus the naive repair.
			fetchGit(t, work, "config", "--add", "remote.origin.fetch", spec)
			fetchGit(t, work, "config", "--add", "remote.origin.fetch", FetchRefspec)
			code, _ := fetchOrigin(t, work)
			if code == 0 && ledgerAt(t, work) == ahead {
				t.Fatal("the direct spelling stopped applying, so this test no longer measures anything")
			}

			// So the verb removes it, and says it did.
			fetchGit(t, work, "update-ref", Ref, ahead)
			got, _, _, err := ConfigureFetch(work)
			if err != nil || got != FetchRedirected {
				t.Fatalf("ConfigureFetch = %q, %v; want %q", got, err, FetchRedirected)
			}
			// The ledger's tracking refspec was already there, so what
			// ConfigureFetch adds is the branch one this origin had none of.
			if want := FetchRefspec + "|" + BranchRefspec; strings.Join(configured(t, work), "|") != want {
				t.Fatalf("remote.origin.fetch = %#v, want %q", configured(t, work), want)
			}
			if code, out := fetchOrigin(t, work); code != 0 {
				t.Errorf("the fetch was still refused after the correction: %d\n%s", code, out)
			}
			if got := ledgerAt(t, work); got != ahead {
				t.Fatalf("the ledger moved to %s, want %s", got, ahead)
			}
		})
	}
}

// A repository carrying both direct spellings at once — which `config --add`
// twice produces — is corrected in one pass, because the pattern matches both.
func TestBothDirectSpellingsAtOnceAreRemovedTogether(t *testing.T) {
	_, work := fetchLab(t)
	for _, spec := range DirectRefspecs {
		fetchGit(t, work, "config", "--add", "remote.origin.fetch", spec)
	}
	if got, _, _, err := ConfigureFetch(work); err != nil || got != FetchRedirected {
		t.Fatalf("ConfigureFetch = %q, %v; want %q", got, err, FetchRedirected)
	}
	// Written from nothing, and the branch refspec comes first: it is the line
	// git itself would have written, and the ledger's is the one kept adds.
	if want := BranchRefspec + "|" + FetchRefspec; strings.Join(configured(t, work), "|") != want {
		t.Fatalf("remote.origin.fetch = %#v, want %q", configured(t, work), want)
	}
}

// DirectFetch is what kept doctor reads, and it reports the state of somebody
// else's config rather than a value kept wrote, so it gets a test that puts
// each state there by hand.
func TestDirectFetchReportsWhatOriginIsConfiguredToDo(t *testing.T) {
	_, work := fetchLab(t)
	if direct, err := DirectFetch(work); err != nil || len(direct) != 0 {
		t.Fatalf("DirectFetch on an origin with no ledger refspec = %v, %v", direct, err)
	}
	fetchGit(t, work, "config", "--add", "remote.origin.fetch", DirectRefspec)
	if direct, err := DirectFetch(work); err != nil || strings.Join(direct, "|") != DirectRefspec {
		t.Fatalf("DirectFetch on an unforced direct origin = %v, %v", direct, err)
	}
	fetchGit(t, work, "config", "--add", "remote.origin.fetch", ForcedDirectRefspec)
	// Reported forced first, whatever order the config holds them in.
	if direct, err := DirectFetch(work); err != nil || strings.Join(direct, "|") != ForcedDirectRefspec+"|"+DirectRefspec {
		t.Fatalf("DirectFetch on an origin carrying both = %v, %v", direct, err)
	}
	if _, _, _, err := ConfigureFetch(work); err != nil {
		t.Fatal(err)
	}
	if direct, err := DirectFetch(work); err != nil || len(direct) != 0 {
		t.Fatalf("DirectFetch after the correction = %v, %v", direct, err)
	}

	// A manuscript that is not in a repository has no origin to report on.
	if _, err := DirectFetch(t.TempDir()); !errors.Is(err, ErrNotRepository) {
		t.Fatalf("DirectFetch outside a repository = %v, want ErrNotRepository", err)
	}
}

// ConfigureFetch is run on adoption and again on every re-init, so the
// unchanged case has to be exactly that.
func TestConfiguringAnAlreadyConfiguredOriginWritesNothing(t *testing.T) {
	_, work := fetchLab(t)
	fetchGit(t, work, "config", "--add", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*")
	if got, _, _, err := ConfigureFetch(work); err != nil || got != FetchAdded {
		t.Fatalf("first ConfigureFetch = %q, %v; want %q", got, err, FetchAdded)
	}
	first := configured(t, work)
	if got, _, _, err := ConfigureFetch(work); err != nil || got != FetchUnchanged {
		t.Fatalf("second ConfigureFetch = %q, %v; want %q", got, err, FetchUnchanged)
	}
	if second := configured(t, work); strings.Join(second, "|") != strings.Join(first, "|") {
		t.Fatalf("re-configuring altered or duplicated the refspecs: %#v, was %#v", second, first)
	}
	if want := "+refs/heads/*:refs/remotes/origin/*|" + FetchRefspec; strings.Join(first, "|") != want {
		t.Fatalf("remote.origin.fetch = %#v, want the ordinary and the ledger refspecs", first)
	}
}

// The upstream ref must not be reachable by the source pattern, or a fetch
// would map it back onto itself and mint a ref per fetch.
func TestTheUpstreamRefIsNotItsOwnSource(t *testing.T) {
	if strings.HasPrefix(UpstreamRef, Ref) {
		t.Fatalf("%s starts with %s, so the refspec %s maps it onto itself", UpstreamRef, Ref, FetchRefspec)
	}
}

// fetchLab builds a bare origin and a work repository wired to it, with no
// fetch refspec configured. Both are real repositories: what is being measured
// is what git does, so nothing here may be a stand-in for it.
func fetchLab(t *testing.T) (origin, work string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Git is required to fetch anything: " + err.Error())
	}
	root := t.TempDir()
	origin, work = filepath.Join(root, "origin.git"), filepath.Join(root, "work")
	for _, dir := range []string{origin, work} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	fetchGit(t, origin, "init", "-q", "--bare")
	fetchGit(t, work, "init", "-q")
	fetchGit(t, work, "config", "user.email", "test@example.com")
	fetchGit(t, work, "config", "user.name", "test")
	fetchGit(t, work, "remote", "add", "--no-tags", "origin", origin)
	// git remote add writes a branch refspec. The ledger is what is under
	// test, so it starts with none of its own.
	fetchGit(t, work, "config", "--unset-all", "remote.origin.fetch")
	return origin, work
}

func appendAct(t *testing.T, store *RefStore, line string) {
	t.Helper()
	if err := store.Append("acts.jsonl", []byte(line)); err != nil {
		t.Fatal(err)
	}
}

func fetchGit(t *testing.T, repository string, args ...string) {
	t.Helper()
	if code, out := runGit(t, repository, args...); code != 0 {
		t.Fatalf("git %s exited %d:\n%s", strings.Join(args, " "), code, out)
	}
}

func fetchOrigin(t *testing.T, work string, args ...string) (int, string) {
	t.Helper()
	return runGit(t, work, append([]string{"fetch"}, append(args, "origin")...)...)
}

func runGit(t *testing.T, repository string, args ...string) (int, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "git", append([]string{"-C", repository}, args...)...)
	command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "HOME="+t.TempDir(), "GIT_TERMINAL_PROMPT=0")
	out, err := command.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), ctx.Err())
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode(), string(out)
	}
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return 0, string(out)
}

func ledgerAt(t *testing.T, work string) string {
	t.Helper()
	return refAt(t, work, Ref)
}

func refAt(t *testing.T, work, ref string) string {
	t.Helper()
	code, out := runGit(t, work, "rev-parse", "--verify", "--quiet", ref)
	if code != 0 {
		return ""
	}
	return strings.TrimSpace(out)
}

func configured(t *testing.T, work string) []string {
	t.Helper()
	code, out := runGit(t, work, "config", "--get-all", "remote.origin.fetch")
	if code == 1 {
		return nil
	}
	if code != 0 {
		t.Fatalf("config --get-all exited %d:\n%s", code, out)
	}
	return strings.Fields(out)
}

// remoteLedger is what origin holds for the ledger ref. Asked over the wire
// rather than read from a local ref, because the local refs are the subject.
func remoteLedger(t *testing.T, work string) string {
	t.Helper()
	code, out := runGit(t, work, "ls-remote", "origin", Ref)
	if code != 0 {
		t.Fatalf("ls-remote exited %d:\n%s", code, out)
	}
	return strings.TrimSpace(strings.SplitN(strings.TrimSpace(out), "\t", 2)[0])
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// The defect this repository was in on 2026-08-25, as a property rather than as
// a case: after kept has configured a workspace, an ordinary `git fetch origin`
// updates the remote-tracking branch.
//
// It is measured with a real branch on a real origin, because what is being
// claimed is what git does. Before the fix the fetch exited 0 and wrote
// nothing, which is the shape of the whole problem: the failure is silent and
// the ref it leaves behind reads exactly like a current one.
func TestAnOrdinaryFetchUpdatesTheTrackingBranchAfterConfigureFetch(t *testing.T) {
	origin, work := fetchLab(t)
	if _, branch, _, err := ConfigureFetch(work); err != nil || branch != FetchAdded {
		t.Fatalf("ConfigureFetch branch = %q, %v; want %q", branch, err, FetchAdded)
	}

	commit := commitOnOriginMain(t, origin, work)
	if code, out := fetchOrigin(t, work); code != 0 {
		t.Fatalf("git fetch origin exited %d:\n%s", code, out)
	}
	if got := refAt(t, work, "refs/remotes/origin/main"); got != commit {
		t.Fatalf("refs/remotes/origin/main = %q after a fetch, want %q", got, commit)
	}
}

// The test that breaks the guard. With the ledger's refspec as the sole entry —
// which is what kept configured before this — the same fetch exits 0 and leaves
// no tracking ref at all. A property whose subject is not what it measures is
// worth nothing, so the state it was written against has to fail it.
func TestTheLedgerRefspecAloneUpdatesNoTrackingBranch(t *testing.T) {
	origin, work := fetchLab(t)
	fetchGit(t, work, "config", "--add", "remote.origin.fetch", FetchRefspec)

	commitOnOriginMain(t, origin, work)
	if code, out := fetchOrigin(t, work); code != 0 {
		t.Fatalf("git fetch origin exited %d:\n%s", code, out)
	}
	if got := refAt(t, work, "refs/remotes/origin/main"); got != "" {
		t.Fatalf("refs/remotes/origin/main = %q, so the ledger refspec alone is not what this measures", got)
	}
}

// A branch refspec somebody else wrote is theirs. An operator who narrowed
// origin to one branch has answered this question, and widening it would be
// kept deciding what a person's remote fetches — which is a different act from
// removing the two direct spellings kept itself authored.
func TestAnExistingBranchRefspecIsNeitherWidenedNorDuplicated(t *testing.T) {
	for _, spec := range []string{
		"+refs/heads/main:refs/remotes/origin/main",
		"refs/heads/*:refs/remotes/origin/*",
		BranchRefspec,
	} {
		t.Run(spec, func(t *testing.T) {
			_, work := fetchLab(t)
			fetchGit(t, work, "config", "--add", "remote.origin.fetch", spec)
			if _, branch, _, err := ConfigureFetch(work); err != nil || branch != FetchUnchanged {
				t.Fatalf("ConfigureFetch branch = %q, %v; want %q", branch, err, FetchUnchanged)
			}
			if want := spec + "|" + FetchRefspec; strings.Join(configured(t, work), "|") != want {
				t.Fatalf("remote.origin.fetch = %#v, want %q", configured(t, work), want)
			}
		})
	}
}

// BranchFetch is what kept doctor reads, and it reports the state of somebody
// else's config rather than a value kept wrote, so each state is put there by
// hand.
//
// The three-way answer is the point. *There is no origin to track* is not *this
// origin tracks nothing*: a workspace adopted before it had a remote is in the
// first state, and reporting it as the second would be a finding its operator
// can do nothing about.
func TestBranchFetchReportsWhatOriginIsConfiguredToDo(t *testing.T) {
	_, work := fetchLab(t)
	if got, err := BranchFetch(work); err != nil || got != BranchesUntracked {
		t.Fatalf("BranchFetch on an origin with no refspec = %q, %v; want %q", got, err, BranchesUntracked)
	}
	fetchGit(t, work, "config", "--add", "remote.origin.fetch", FetchRefspec)
	if got, err := BranchFetch(work); err != nil || got != BranchesUntracked {
		t.Fatalf("BranchFetch on a ledger-only origin = %q, %v; want %q", got, err, BranchesUntracked)
	}
	fetchGit(t, work, "config", "--add", "remote.origin.fetch", BranchRefspec)
	if got, err := BranchFetch(work); err != nil || got != BranchesTracked {
		t.Fatalf("BranchFetch after the branch refspec = %q, %v; want %q", got, err, BranchesTracked)
	}

	// No origin URL: nothing to track, and nothing to say about it. kept init
	// writes no branch refspec here either, so the two agree.
	bare := t.TempDir()
	fetchGit(t, bare, "init", "-q")
	if got, err := BranchFetch(bare); err != nil || got != NoRemote {
		t.Fatalf("BranchFetch with no origin = %q, %v; want %q", got, err, NoRemote)
	}
	if _, branch, _, err := ConfigureFetch(bare); err != nil || branch != FetchUnchanged {
		t.Fatalf("ConfigureFetch branch with no origin = %q, %v; want %q", branch, err, FetchUnchanged)
	}
	if got := configured(t, bare); contains(got, BranchRefspec) {
		t.Fatalf("a repository with no origin was given %q: %#v", BranchRefspec, got)
	}

	// A manuscript that is not in a repository has no origin to report on.
	if _, err := BranchFetch(t.TempDir()); !errors.Is(err, ErrNotRepository) {
		t.Fatalf("BranchFetch outside a repository = %v, want ErrNotRepository", err)
	}
}

// Which half of a refspec decides the question. Either one alone admits a
// config that leaves the tracking refs stale, so both are required — and each
// row here is a config an operator could plausibly be holding.
func TestOnlyARefspecThatBringsBranchesIntoTrackingRefsCounts(t *testing.T) {
	for _, c := range []struct {
		refspec string
		want    bool
	}{
		{BranchRefspec, true},
		{"refs/heads/*:refs/remotes/origin/*", true},
		{"+refs/heads/main:refs/remotes/origin/main", true},
		{FetchRefspec, false},
		{"+refs/heads/*:refs/heads/*", false},         // fetches branches, tracks none
		{"+refs/tags/*:refs/remotes/origin/*", false}, // writes tracking refs, from no branch
		{"+refs/heads/*", false},                      // not a mapping at all
		{"", false},
	} {
		if got := tracksBranches(c.refspec); got != c.want {
			t.Errorf("tracksBranches(%q) = %v, want %v", c.refspec, got, c.want)
		}
	}
}

// commitOnOriginMain puts a real commit on origin's main branch and returns it.
// The work repository is what pushes it, because fetchLab's origin is bare.
func commitOnOriginMain(t *testing.T, origin, work string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(work, "file.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fetchGit(t, work, "add", "file.txt")
	fetchGit(t, work, "commit", "-q", "-m", "one")
	fetchGit(t, work, "push", "-q", origin, "HEAD:refs/heads/main")
	code, out := runGit(t, work, "rev-parse", "HEAD")
	if code != 0 {
		t.Fatalf("rev-parse HEAD exited %d:\n%s", code, out)
	}
	return strings.TrimSpace(out)
}
