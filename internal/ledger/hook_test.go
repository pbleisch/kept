package ledger

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The hook goes where git looks for one, which is the shared hooks directory
// rather than a worktree's own — the same place internal/lock puts a lock file
// and for the same reason. One install covers every linked worktree.
func TestTheHookIsWrittenWhereGitLooksForOneAndCoversEveryWorktree(t *testing.T) {
	_, work := fetchLab(t)

	h, err := ConfigureHook(work)
	if err != nil {
		t.Fatal(err)
	}
	if h.State != HookWritten {
		t.Fatalf("ConfigureHook = %q, want %q", h.State, HookWritten)
	}
	want := filepath.Join(work, ".git", "hooks", PrePushHook)
	if h.Path != want {
		t.Errorf("hook path = %s, want %s", h.Path, want)
	}
	body, err := os.ReadFile(h.Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != HookBody {
		t.Error("the hook on disk is not the body kept writes")
	}
	info, err := os.Stat(h.Path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o100 == 0 {
		t.Errorf("hook mode is %v, and git runs nothing it cannot execute", info.Mode().Perm())
	}

	// A linked worktree resolves to that one file rather than to a second one
	// beside itself. Reading it from the other side is the whole assertion.
	linked := filepath.Join(t.TempDir(), "wt")
	commit(t, work, "a.txt", "one")
	fetchGit(t, work, "worktree", "add", "-q", "-b", "topic", linked)
	from, err := HookState(linked)
	if err != nil {
		t.Fatal(err)
	}
	// Compared through the filesystem: git answers a linked worktree with the
	// resolved path and the primary checkout with the one it was handed, and
	// the assertion is that they are one file.
	if from.State != HookPresent || !sameDir(from.Path, want) {
		t.Errorf("from the linked worktree: %q at %s, want %q at %s", from.State, from.Path, HookPresent, want)
	}
}

// kept init writes what is missing and never rewrites what is there, and the
// hook follows that rule rather than a version of it. A person who edited
// kept's own hook keeps their edit: the marker is what says the file is kept's,
// and being kept's is not permission to overwrite it.
func TestASecondRunRewritesNothing(t *testing.T) {
	_, work := fetchLab(t)
	if _, err := ConfigureHook(work); err != nil {
		t.Fatal(err)
	}

	edited := HookBody + "\n# and a line the operator added\n"
	h, err := HookState(work)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(h.Path, []byte(edited), 0o755); err != nil {
		t.Fatal(err)
	}

	again, err := ConfigureHook(work)
	if err != nil {
		t.Fatal(err)
	}
	if again.State != HookPresent {
		t.Fatalf("the second run = %q, want %q", again.State, HookPresent)
	}
	body, err := os.ReadFile(h.Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != edited {
		t.Error("the second run rewrote a hook that was already there")
	}
}

// The guard, broken deliberately: take the marker line out and the same file is
// somebody else's, left exactly as it is. Git runs one pre-push hook, so there
// is no version of this that appends, replaces, or writes a file beside it.
func TestAHookKeptDidNotWriteIsLeftExactlyAsItIs(t *testing.T) {
	_, work := fetchLab(t)
	theirs := "#!/bin/sh\n# somebody's own hook\nexit 0\n"
	hooks := filepath.Join(work, ".git", "hooks")
	if err := os.MkdirAll(hooks, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(hooks, PrePushHook)
	if err := os.WriteFile(path, []byte(theirs), 0o755); err != nil {
		t.Fatal(err)
	}

	h, err := ConfigureHook(work)
	if err != nil {
		t.Fatal(err)
	}
	if h.State != HookForeign {
		t.Fatalf("ConfigureHook = %q, want %q", h.State, HookForeign)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != theirs {
		t.Fatalf("kept wrote into a hook it did not author:\n%s", body)
	}
	// Nothing beside it either. A file nothing invokes reads as installed and
	// is not.
	entries, err := os.ReadDir(hooks)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != PrePushHook && !strings.HasSuffix(e.Name(), ".sample") {
			t.Errorf("kept wrote %s beside a hook it did not author", e.Name())
		}
	}
}

// core.hooksPath is a total bypass, and kept writes nothing anywhere when it is
// set: not into the directory git reads, which belongs to whatever hook runner
// set it, and not into the one git does not, which would be a file reported as
// written that can never run.
func TestACoreHooksPathIsReportedAndNothingIsWritten(t *testing.T) {
	_, work := fetchLab(t)
	elsewhere := filepath.Join(work, "myhooks")
	if err := os.MkdirAll(elsewhere, 0o755); err != nil {
		t.Fatal(err)
	}
	fetchGit(t, work, "config", "core.hooksPath", elsewhere)

	h, err := ConfigureHook(work)
	if err != nil {
		t.Fatal(err)
	}
	if h.State != HookBypassed {
		t.Fatalf("ConfigureHook = %q, want %q", h.State, HookBypassed)
	}
	if h.HooksPath != elsewhere {
		t.Errorf("HooksPath = %q, want %q", h.HooksPath, elsewhere)
	}
	for _, dir := range []string{elsewhere, filepath.Join(work, ".git", "hooks")} {
		if _, err := os.Stat(filepath.Join(dir, PrePushHook)); !os.IsNotExist(err) {
			t.Errorf("kept wrote a hook into %s", dir)
		}
	}

	// Pointed back at the directory git would have read anyway, it is not a
	// bypass. The check is over which directory git names, never over whether
	// the setting is present.
	fetchGit(t, work, "config", "core.hooksPath", filepath.Join(work, ".git", "hooks"))
	if back, err := ConfigureHook(work); err != nil {
		t.Fatal(err)
	} else if back.State != HookWritten {
		t.Errorf("with core.hooksPath at the common hooks directory: %q, want %q", back.State, HookWritten)
	}
}

// A manuscript can be initialized before it is committed to Git. There is no
// hook to write and no push to warn on, which is silence rather than a failure.
func TestOutsideARepositoryThereIsNoHook(t *testing.T) {
	h, err := HookState(t.TempDir())
	if !errors.Is(err, ErrNotRepository) {
		t.Fatalf("HookState outside a repository: %v, want %v", err, ErrNotRepository)
	}
	if h.State != HookNoRepository {
		t.Errorf("state = %q, want %q", h.State, HookNoRepository)
	}
	if _, err := ConfigureHook(t.TempDir()); !errors.Is(err, ErrNotRepository) {
		t.Fatalf("ConfigureHook outside a repository: %v, want %v", err, ErrNotRepository)
	}
}

// The whole point, driven through a real push: the ledger holds acts origin
// does not have, the operator pushes a branch, and the warning arrives on
// stderr with the count and the command — while the push itself succeeds.
//
// It is a real push against a real bare origin, because what is being measured
// is what git does with a hook.
func TestAPushWarnsWhenTheLedgerHoldsActsOriginDoesNotHave(t *testing.T) {
	origin, work, store := hookLab(t)

	appendAct(t, store, `{"act":"ratified decision:0073"}`)
	fetchGit(t, work, "push", "-q", "origin", Ref+":"+Ref)
	if refAt(t, work, UpstreamRef) == "" {
		t.Fatal("publishing the ledger left no upstream value; the warning cannot self-quiet")
	}

	// One act, so the count is singular. kept never prints "1 commits".
	appendAct(t, store, `{"act":"ratified decision:0050"}`)
	commit(t, work, "a.txt", "two")
	code, out := runGit(t, work, "push", "origin", "main")
	if code != 0 {
		t.Fatalf("the push exited %d; the warning must never refuse one:\n%s", code, out)
	}
	mustSay(t, out,
		"the ledger holds 1 commit origin did not have as of the last fetch",
		"git push does not carry a side ref",
		PushCommand)
	if strings.Contains(out, "1 commits") {
		t.Errorf("the warning does not count in English:\n%s", out)
	}

	// And it says how many, rather than that there are some.
	appendAct(t, store, `{"act":"ratified decision:0062"}`)
	appendAct(t, store, `{"act":"ratified decision:0054"}`)
	commit(t, work, "a.txt", "three")
	_, out = runGit(t, work, "push", "origin", "main")
	mustSay(t, out, "the ledger holds 3 commits origin did not have")

	// Published, it goes quiet — which works only because kept init configured
	// the refspec that maps the ledger into the upstream ref. That is
	// docs/decision/0062's consequence arriving as the thing that makes this
	// check free.
	fetchGit(t, work, "push", "-q", "origin", Ref+":"+Ref)
	commit(t, work, "a.txt", "four")
	code, out = runGit(t, work, "push", "origin", "main")
	if code != 0 || strings.Contains(out, "kept:") {
		t.Fatalf("the push said something about a published ledger (exit %d):\n%s", code, out)
	}
	_ = origin
}

// A dry run warns and publishes nothing, which is the whole reason a warning is
// safe where a publishing hook is not: measured, a hook's stdin and environment
// are identical between a real push and a dry run, so there is no signal to
// condition a write on.
func TestADryRunWarnsAndMovesNothing(t *testing.T) {
	origin, work, store := hookLab(t)
	appendAct(t, store, `{"act":"first"}`)
	fetchGit(t, work, "push", "-q", "origin", Ref+":"+Ref)
	appendAct(t, store, `{"act":"second"}`)

	published := refAt(t, origin, Ref)
	commit(t, work, "a.txt", "two")
	head := refAt(t, origin, "refs/heads/main")

	code, out := runGit(t, work, "push", "--dry-run", "origin", "main")
	if code != 0 {
		t.Fatalf("the dry run exited %d:\n%s", code, out)
	}
	mustSay(t, out, "the ledger holds 1 commit origin did not have")
	if now := refAt(t, origin, Ref); now != published {
		t.Errorf("the dry run moved origin's ledger from %s to %s", published, now)
	}
	if now := refAt(t, origin, "refs/heads/main"); now != head {
		t.Errorf("the dry run moved origin's branch from %s to %s", head, now)
	}
}

// A push already carrying the ledger has nothing to warn about. Without the ref
// list being read, running the command the warning just printed prints it
// again.
func TestThePushThatCarriesTheLedgerSaysNothing(t *testing.T) {
	_, work, store := hookLab(t)
	appendAct(t, store, `{"act":"first"}`)
	fetchGit(t, work, "push", "-q", "origin", Ref+":"+Ref)
	appendAct(t, store, `{"act":"second"}`)

	code, out := runGit(t, work, "push", "origin", Ref+":"+Ref)
	if code != 0 {
		t.Fatalf("publishing the ledger exited %d:\n%s", code, out)
	}
	if strings.Contains(out, "kept:") {
		t.Fatalf("the push that publishes the ledger warned about publishing it:\n%s", out)
	}
}

// A fork is not an unpublished ledger, and it must not be handed a push origin
// would refuse — whose refusal git annotates with advice that is wrong for an
// append-only record.
func TestAForkedLedgerIsSentToDoctorRatherThanToAPush(t *testing.T) {
	_, work, store := hookLab(t)
	appendAct(t, store, `{"act":"shared"}`)
	fetchGit(t, work, "push", "-q", "origin", Ref+":"+Ref)
	shared := ledgerAt(t, work)

	// Origin moved on somebody else's acts, and so did this ledger.
	appendAct(t, store, `{"act":"theirs"}`)
	fetchGit(t, work, "push", "-q", "origin", Ref+":"+Ref)
	fetchGit(t, work, "update-ref", Ref, shared)
	appendAct(t, store, `{"act":"mine"}`)

	commit(t, work, "a.txt", "two")
	code, out := runGit(t, work, "push", "origin", "main")
	if code != 0 {
		t.Fatalf("the push exited %d; a forked ledger is not the operator's branch's problem:\n%s", code, out)
	}
	mustSay(t, out, "have both moved", "kept doctor")
	if strings.Contains(out, PushCommand) {
		t.Errorf("the warning handed over a push origin would refuse:\n%s", out)
	}
}

// The fresh adoption, which is the sequence the decision is about: clone,
// kept init, ratify, push. Nothing has fetched, so there is no upstream value
// to compare against — and an earlier draft that kept silent here said nothing
// at all through the whole of it.
//
// It claims only what it can: nobody has asked. And it hands over the
// publication rather than a fetch, because a fetch that brings nothing leaves
// the state exactly as it was and the warning would repeat forever, while the
// push resolves it in either direction and quiets it when it lands.
func TestNothingHavingHeardFromOriginIsSaidAndIsQuietedByPublishing(t *testing.T) {
	origin, work, store := hookLab(t)
	appendAct(t, store, `{"act":"ratified promise:one"}`)
	if refAt(t, work, UpstreamRef) != "" {
		t.Fatal("this lab has an upstream value and cannot measure its absence")
	}
	if refAt(t, origin, Ref) != "" {
		t.Fatal("origin already holds a ledger")
	}

	commit(t, work, "a.txt", "two")
	code, out := runGit(t, work, "push", "origin", "main")
	if code != 0 {
		t.Fatalf("the push exited %d:\n%s", code, out)
	}
	mustSay(t, out, "nothing here has heard from origin about the ledger", PushCommand)
	if strings.Contains(out, "as of the last fetch") {
		t.Errorf("the warning claimed something about origin that no local ref supports:\n%s", out)
	}

	// The command it printed, run: it lands, and the next push is silent.
	fetchGit(t, work, "push", "-q", "origin", Ref+":"+Ref)
	commit(t, work, "a.txt", "three")
	code, out = runGit(t, work, "push", "origin", "main")
	if code != 0 || strings.Contains(out, "kept:") {
		t.Fatalf("the warning survived the publication it asked for (exit %d):\n%s", code, out)
	}
}

// A repository with no ledger at all says nothing, on any push. This is every
// repository that has adopted kept and not yet recorded anything.
func TestAPushInARepositoryWithNoLedgerSaysNothing(t *testing.T) {
	_, work, _ := hookLab(t)
	commit(t, work, "a.txt", "two")
	code, out := runGit(t, work, "push", "origin", "main")
	if code != 0 || strings.Contains(out, "kept:") {
		t.Fatalf("the push spoke with no ledger to speak about (exit %d):\n%s", code, out)
	}
}

// hookLab is fetchLab with the whole of adoption run over it — the refspec, the
// reflog and the hook — plus a first commit on a branch origin has, so a push
// is something a test can perform.
func hookLab(t *testing.T) (origin, work string, store *RefStore) {
	t.Helper()
	origin, work = fetchLab(t)
	if _, _, _, err := ConfigureFetch(work); err != nil {
		t.Fatal(err)
	}
	if h, err := ConfigureHook(work); err != nil {
		t.Fatal(err)
	} else if h.State != HookWritten {
		t.Fatalf("ConfigureHook = %q, want %q", h.State, HookWritten)
	}
	commit(t, work, "a.txt", "one")
	// git init names the first branch from configuration this test does not
	// control, and every push below names one.
	fetchGit(t, work, "branch", "-M", "main")
	fetchGit(t, work, "push", "-q", "origin", "main")
	return origin, work, NewRefStore(work)
}

func commit(t *testing.T, work, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(work, name), []byte(body+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fetchGit(t, work, "add", name)
	fetchGit(t, work, "commit", "-q", "-m", body)
}

func mustSay(t *testing.T, out string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Errorf("the push said nothing about %q:\n%s", w, out)
		}
	}
}
