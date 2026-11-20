package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/ledger"
)

// bare makes an empty bare repository to stand for a remote.
func bare(t *testing.T, git func(string, ...string)) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "remote.git")
	git(filepath.Dir(dir), "init", "-q", "--bare", dir)
	return dir
}

// configValue reads one git config value in a repository, or nothing.
func configValue(t *testing.T, dir, key string) string {
	t.Helper()
	cmd := exec.Command("git", "config", "--get", key)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, _ := cmd.Output()
	return strings.TrimSpace(string(out))
}

// kept init names a private remote for session records, and refuses origin and
// anything that is origin under another name before it writes a single file.
// See docs/decision/0089.
func TestInitNamesASessionsRemoteAndRefusesOrigin(t *testing.T) {
	git := gitOrSkip(t)
	dir := t.TempDir()
	git(dir, "init", "-q")
	origin, archive := bare(t, git), bare(t, git)
	git(dir, "remote", "add", "origin", origin)
	git(dir, "remote", "add", "mirror", origin)
	git(dir, "remote", "add", "archive", archive)

	for remote, says := range map[string]string{
		"origin":  "never go where the ledger is published",
		"mirror":  "never go where the ledger is published",
		"nowhere": "not a remote here",
	} {
		code, _, stderr := at(t, dir, "init", "--sessions-remote", remote)
		if code != exitUsage {
			t.Fatalf("init --sessions-remote %s exited %d", remote, code)
		}
		mustContain(t, "the refusal", stderr, remote, says)
	}
	if _, err := os.Stat(filepath.Join(dir, ".kept")); !os.IsNotExist(err) {
		t.Fatal("a refused init wrote the workspace anyway")
	}
	if got := configValue(t, dir, ledger.SessionsRemoteKey); got != "" {
		t.Fatalf("a refused init named %q", got)
	}

	code, stdout, stderr := at(t, dir, "init", "--sessions-remote", "archive")
	if code != exitOK {
		t.Fatalf("init --sessions-remote archive exited %d: %s", code, stderr)
	}
	mustContain(t, "init", stdout, "session records go to archive", "archive fetches "+ledger.SessionsRef+" into "+ledger.SessionsUpstreamRef)
	if got := configValue(t, dir, ledger.SessionsRemoteKey); got != "archive" {
		t.Fatalf("%s = %q, want archive", ledger.SessionsRemoteKey, got)
	}
	if code, stdout, _ := at(t, dir, "init"); code != exitOK || strings.Contains(stdout, "session records go to") {
		t.Fatalf("a second init configured the sessions remote again (exit %d):\n%s", code, stdout)
	}

	outside := t.TempDir()
	if code, _, stderr := at(t, outside, "init", "--sessions-remote", "archive"); code != exitUsage || !strings.Contains(stderr, "not in a Git repository") {
		t.Fatalf("init --sessions-remote outside a repository exited %d: %s", code, stderr)
	}
}

// doctor says where session records live, in the order a repository arrives at
// each state, and never moves its exit code for any of them.
func TestDoctorSaysWhereSessionRecordsLive(t *testing.T) {
	w, git := repository(t, map[string]string{"first": "the first promise"})
	if code, _, stderr := w.run("init"); code != exitOK {
		t.Fatalf("init exited %d: %s", code, stderr)
	}
	code, stdout, _ := w.run("doctor")
	if strings.Contains(stdout, "session records") {
		t.Fatalf("a repository that recorded no session is reported:\n%s", stdout)
	}

	w.thread(t)
	got, stdout, _ := w.run("doctor")
	mustContain(t, "doctor", stdout, "session records exist only on this machine", "kept init --sessions-remote <remote>")
	if got != code {
		t.Errorf("the finding moved doctor's exit code from %d to %d", code, got)
	}

	archive := bare(t, git)
	git(w.root, "remote", "add", "archive", archive)
	if code, _, stderr := w.run("init", "--sessions-remote", "archive"); code != exitOK {
		t.Fatalf("init --sessions-remote exited %d: %s", code, stderr)
	}
	_, stdout, _ = w.run("doctor")
	mustContain(t, "doctor", stdout, "nothing here has heard from archive about session records", ledger.SessionsPushCommand("archive"))

	git(w.root, "push", "-q", "archive", ledger.SessionsRef+":"+ledger.SessionsRef)
	if _, stdout, _ := w.run("doctor"); strings.Contains(stdout, "session records") {
		t.Fatalf("session records just pushed are still reported:\n%s", stdout)
	}

	w.thread(t)
	_, stdout, _ = w.run("doctor")
	mustContain(t, "doctor", stdout, "session records archive did not have as of the last fetch or push", ledger.SessionsPushCommand("archive"))

	// Named by hand to be where the ledger is published, which init refuses.
	git(w.root, "config", "remote.origin.url", bare(t, git))
	git(w.root, "config", ledger.SessionsRemoteKey, "origin")
	_, stdout, _ = w.run("doctor")
	mustContain(t, "doctor", stdout, "session records are named to go where the ledger is published")

	// kept's hook from before the sessions guard.
	hook := filepath.Join(w.root, ".git", "hooks", ledger.PrePushHook)
	earlier := strings.Replace(read(t, hook), ledger.HookSessionsMarker+"\n", "", 1)
	if err := os.WriteFile(hook, []byte(earlier), 0o755); err != nil {
		t.Fatal(err)
	}
	_, stdout, _ = w.run("doctor")
	mustContain(t, "doctor", stdout, "kept's pre-push hook does not refuse a push carrying session records", "kept init")
}

// A real clone of a repository whose operator has repaired: origin holds the
// ledger and never the sessions ref, so the clone has standing and no session
// record. Every verb that reads a receipt refuses with the words it already
// uses, and the verbs that do not read one answer as they always did.
func TestACloneWithoutTheSessionsRefGetsTheRefusalsItAlreadyHas(t *testing.T) {
	w, git := repository(t, map[string]string{"first": "the first promise"})
	origin := bare(t, git)
	git(w.root, "remote", "add", "origin", origin)
	git(w.root, "branch", "-M", "main")
	if code, _, stderr := w.run("init"); code != exitOK {
		t.Fatalf("init exited %d: %s", code, stderr)
	}
	w.ratify(t, "promise:first")
	id := w.thread(t)
	git(w.root, "push", "-q", "origin", "main", ledger.Ref+":"+ledger.Ref)

	// Origin got the ledger and nothing else under refs/kept/.
	listed := gitOut(t, git, w.root, "ls-remote", "origin", "refs/kept/*")
	if !strings.Contains(listed, ledger.Ref) || strings.Contains(listed, "sessions") {
		t.Fatalf("origin holds:\n%s", listed)
	}

	clone := filepath.Join(t.TempDir(), "clone")
	git(filepath.Dir(clone), "clone", "-q", "-b", "main", origin, clone)
	if code, _, stderr := at(t, clone, "init"); code != exitOK {
		t.Fatalf("init in the clone exited %d: %s", code, stderr)
	}
	git(clone, "fetch", "-q", "origin")
	if code, _, stderr := at(t, clone, "ledger", "sync"); code != exitOK {
		t.Fatalf("ledger sync in the clone exited %d: %s", code, stderr)
	}
	if refs := gitOut(t, git, clone, "for-each-ref", "--format=%(refname)", "refs/kept/"); strings.Contains(refs, "sessions") {
		t.Fatalf("the clone holds a sessions ref:\n%s", refs)
	}

	if code, stdout, stderr := at(t, clone, "status", "--all"); code != exitOK || !strings.Contains(stdout, "promise:first") {
		t.Fatalf("status in the clone exited %d:\n%s\n%s", code, stdout, stderr)
	}
	code, stdout, stderr := at(t, clone, "review", id)
	if code != exitUsage || !strings.Contains(stderr, "no receipt") {
		t.Fatalf("review in the clone exited %d:\n%s\n%s", code, stdout, stderr)
	}
	// Named on the command line, as the ratify above names one: a land is an
	// act and refuses to guess who performed it, and this test must not depend
	// on whose machine runs it.
	code, stdout, stderr = at(t, clone, "land", id, "--by", "the operator <operator@example.com>")
	if code == exitOK || !strings.Contains(stdout+stderr, "anchored no commit") {
		t.Fatalf("land in the clone exited %d:\n%s\n%s", code, stdout, stderr)
	}
	if err := os.WriteFile(filepath.Join(clone, ".kept", "config.yaml"), []byte("corpus: docs\nconfine:\n  provider: nono\ntimeouts:\n  agent: 1m\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr = at(t, clone, "repair", "--rework", id)
	if code != exitUsage || !strings.Contains(stderr, "has no receipt in this ledger") {
		t.Fatalf("rework in the clone exited %d:\n%s\n%s", code, stdout, stderr)
	}
}
