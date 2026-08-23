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

// The regression. A ledger that is ahead of a remote which has fallen behind
// is the ordinary state of this system: acts are appended on every tick and
// nothing in kept pushes, so between two pushes the local ref always holds
// records the remote does not. The refspec kept configures decides what an
// ordinary `git fetch origin` does about that, and the two answers are not
// close together — one rewinds the durable tier over records nobody can
// recover, with exit 0 and no output, and the other refuses.
//
// This is not hypothetical. It happened on 2026-08-22: two ratifications were
// erased from refs/kept/ledger by a fetch that a worktree pool ran on its own,
// and there was no reflog to read afterwards, because git does not log refs
// outside refs/heads/. See docs/decision/0016.
//
// Run this against FetchRefspec with a leading + and it fails on the acts, at
// the last assertion.
func TestAFetchFromAStaleRemoteCannotRewindTheLedger(t *testing.T) {
	_, work := fetchLab(t)

	store := NewRefStore(work)
	if err := store.Append("acts.jsonl", []byte(`{"act":"pushed"}`)); err != nil {
		t.Fatal(err)
	}
	fetchGit(t, work, "push", "-q", "origin", Ref+":"+Ref)
	behind := remoteLedger(t, work)

	// Everything after the push is what an unpushed ratification looks like.
	for _, line := range []string{`{"act":"ratified decision:0017"}`, `{"act":"ratified decision:0052"}`} {
		if err := store.Append("acts.jsonl", []byte(line)); err != nil {
			t.Fatal(err)
		}
	}
	ahead := ledgerAt(t, work)
	if ahead == behind {
		t.Fatalf("the ledger did not advance past what origin holds: %s", ahead)
	}

	// Configured through kept's own verb, because the constant is only a
	// hazard by way of what init writes into somebody's git config.
	if got, _, err := ConfigureFetch(work); err != nil || got != FetchAdded {
		t.Fatalf("ConfigureFetch = %q, %v; want %q", got, err, FetchAdded)
	}

	code, out := fetchOrigin(t, work)
	if code == 0 {
		t.Errorf("git fetch origin against a stale remote exited 0, so nothing told anyone:\n%s", out)
	}
	if !strings.Contains(out, "rejected") || !strings.Contains(out, "non-fast-forward") {
		t.Errorf("the refusal did not say the ledger and the remote have diverged:\n%s", out)
	}

	// The assertion the whole test is for. A forced refspec fails here.
	acts, err := store.Read("acts.jsonl")
	if err != nil {
		t.Fatalf("read the ledger back: %v", err)
	}
	if got := strings.Count(string(acts), "\n"); got != 3 {
		t.Fatalf("the fetch rewound the ledger: %d acts survive, want 3\n%s", got, acts)
	}
	if now := ledgerAt(t, work); now != ahead {
		t.Fatalf("the ledger ref moved to %s, want %s", now, ahead)
	}
}

// The other direction, and the reason the force was never needed. Every fetch
// a ledger legitimately receives is a fast-forward, because RefStore.mutate
// only ever moves the ref forward from a value it just read. An unforced
// refspec has to let all three of these through or it has traded one silent
// failure for a loud useless one.
func TestAnUnforcedRefspecStillFetchesEveryLegitimateLedger(t *testing.T) {
	_, work := fetchLab(t)
	store := NewRefStore(work)
	if err := store.Append("acts.jsonl", []byte(`{"act":"first"}`)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ConfigureFetch(work); err != nil {
		t.Fatal(err)
	}
	fetchGit(t, work, "push", "-q", "origin", Ref+":"+Ref)
	full := ledgerAt(t, work)

	// A clone that has never seen the ledger: the ref is created.
	fetchGit(t, work, "update-ref", "-d", Ref)
	if code, out := fetchOrigin(t, work); code != 0 {
		t.Fatalf("fetch into a repository with no ledger ref exited %d:\n%s", code, out)
	}
	if got := ledgerAt(t, work); got != full {
		t.Fatalf("the absent ledger was not created: %s, want %s", got, full)
	}

	// A remote that is ahead: the ref fast-forwards.
	if err := store.Append("acts.jsonl", []byte(`{"act":"second"}`)); err != nil {
		t.Fatal(err)
	}
	fetchGit(t, work, "push", "-q", "origin", Ref+":"+Ref)
	two := ledgerAt(t, work)
	fetchGit(t, work, "update-ref", Ref, full)
	if code, out := fetchOrigin(t, work); code != 0 {
		t.Fatalf("fetch from a remote that is ahead exited %d:\n%s", code, out)
	}
	if got := ledgerAt(t, work); got != two {
		t.Fatalf("the ledger did not fast-forward: %s, want %s", got, two)
	}

	// Nothing to do: quiet and clean.
	if code, out := fetchOrigin(t, work); code != 0 {
		t.Fatalf("fetch with nothing to do exited %d:\n%s", code, out)
	}
}

// The trap in the middle, and why ConfigureFetch removes rather than adds. A
// clone adopted under the forced spelling already carries it,
// and a build that only added the unforced one beside it would leave a config
// whose behavior turns on which line git reads first. Measured: with the
// forced line first — the order `config --add` produced — the fetch is still
// forced and the ledger is still destroyed.
func TestAddingTheUnforcedRefspecBesideAForcedOneWouldNotDisarmIt(t *testing.T) {
	_, work := fetchLab(t)
	store := NewRefStore(work)
	if err := store.Append("acts.jsonl", []byte(`{"act":"pushed"}`)); err != nil {
		t.Fatal(err)
	}
	fetchGit(t, work, "push", "-q", "origin", Ref+":"+Ref)
	if err := store.Append("acts.jsonl", []byte(`{"act":"unpushed"}`)); err != nil {
		t.Fatal(err)
	}
	ahead := ledgerAt(t, work)

	// What a clone looks like today, plus the naive repair.
	fetchGit(t, work, "config", "--add", "remote.origin.fetch", ForcedFetchRefspec)
	fetchGit(t, work, "config", "--add", "remote.origin.fetch", FetchRefspec)
	if code, out := fetchOrigin(t, work); code != 0 {
		t.Fatalf("the forced spelling stopped applying, so this test no longer measures anything: %d\n%s", code, out)
	}
	if ledgerAt(t, work) == ahead {
		t.Fatal("the forced refspec did not rewind the ledger; both spellings configured is no longer the hazard this describes")
	}

	// So the verb removes it, and says it did.
	fetchGit(t, work, "update-ref", Ref, ahead)
	got, _, err := ConfigureFetch(work)
	if err != nil || got != FetchForceRemoved {
		t.Fatalf("ConfigureFetch = %q, %v; want %q", got, err, FetchForceRemoved)
	}
	if values := configured(t, work); strings.Join(values, "|") != FetchRefspec {
		t.Fatalf("remote.origin.fetch = %#v, want only the unforced ledger refspec", values)
	}
	if code, _ := fetchOrigin(t, work); code == 0 {
		t.Error("the fetch was still accepted after the force was removed")
	}
	if got := ledgerAt(t, work); got != ahead {
		t.Fatalf("the ledger moved to %s, want %s", got, ahead)
	}
}

// ForcedFetch is what kept doctor reads, and it reports the state of somebody
// else's config rather than a value kept wrote, so it gets a test that puts
// each state there by hand.
func TestForcedFetchReportsWhatOriginIsConfiguredToDo(t *testing.T) {
	_, work := fetchLab(t)
	if forced, err := ForcedFetch(work); err != nil || forced {
		t.Fatalf("ForcedFetch on an origin with no ledger refspec = %v, %v", forced, err)
	}
	fetchGit(t, work, "config", "--add", "remote.origin.fetch", ForcedFetchRefspec)
	if forced, err := ForcedFetch(work); err != nil || !forced {
		t.Fatalf("ForcedFetch on a forced origin = %v, %v", forced, err)
	}
	if _, _, err := ConfigureFetch(work); err != nil {
		t.Fatal(err)
	}
	if forced, err := ForcedFetch(work); err != nil || forced {
		t.Fatalf("ForcedFetch after the correction = %v, %v", forced, err)
	}

	// A manuscript that is not in a repository has no origin to report on.
	if _, err := ForcedFetch(t.TempDir()); !errors.Is(err, ErrNotRepository) {
		t.Fatalf("ForcedFetch outside a repository = %v, want ErrNotRepository", err)
	}
}

// ConfigureFetch is run on adoption and again on every re-init, so the
// unchanged case has to be exactly that.
func TestConfiguringAnAlreadyConfiguredOriginWritesNothing(t *testing.T) {
	_, work := fetchLab(t)
	fetchGit(t, work, "config", "--add", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*")
	if got, _, err := ConfigureFetch(work); err != nil || got != FetchAdded {
		t.Fatalf("first ConfigureFetch = %q, %v; want %q", got, err, FetchAdded)
	}
	first := configured(t, work)
	if got, _, err := ConfigureFetch(work); err != nil || got != FetchUnchanged {
		t.Fatalf("second ConfigureFetch = %q, %v; want %q", got, err, FetchUnchanged)
	}
	if second := configured(t, work); strings.Join(second, "|") != strings.Join(first, "|") {
		t.Fatalf("re-configuring altered or duplicated the refspecs: %#v, was %#v", second, first)
	}
	if want := "+refs/heads/*:refs/remotes/origin/*|" + FetchRefspec; strings.Join(first, "|") != want {
		t.Fatalf("remote.origin.fetch = %#v, want the ordinary and the ledger refspecs", first)
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

func fetchGit(t *testing.T, repository string, args ...string) {
	t.Helper()
	if code, out := runGit(t, repository, args...); code != 0 {
		t.Fatalf("git %s exited %d:\n%s", strings.Join(args, " "), code, out)
	}
}

func fetchOrigin(t *testing.T, work string) (int, string) {
	t.Helper()
	return runGit(t, work, "fetch", "origin")
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
	code, out := runGit(t, work, "rev-parse", "--verify", "--quiet", Ref)
	if code != 0 {
		return ""
	}
	return strings.TrimSpace(out)
}

func remoteLedger(t *testing.T, work string) string {
	t.Helper()
	code, out := runGit(t, work, "ls-remote", "origin", Ref)
	if code != 0 {
		t.Fatalf("ls-remote exited %d:\n%s", code, out)
	}
	return strings.TrimSpace(strings.SplitN(strings.TrimSpace(out), "\t", 2)[0])
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
