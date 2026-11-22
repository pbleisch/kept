package ledger

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/run"
)

// treeOf is every path a ref's tree holds, or nothing when the ref is absent.
func treeOf(t *testing.T, work, ref string) []string {
	t.Helper()
	if refAt(t, work, ref) == "" {
		return nil
	}
	code, out := runGit(t, work, "ls-tree", "-r", "--name-only", ref)
	if code != 0 {
		t.Fatalf("ls-tree %s exited %d:\n%s", ref, code, out)
	}
	return strings.Fields(out)
}

// sessionPaths is every path in a tree that is a session record, wherever in
// the tree it sits: a receipt or a payload, under the old layout or the new.
func sessionPaths(paths []string) []string {
	var found []string
	for _, p := range paths {
		if strings.Contains(p, "threads/") || strings.Contains(p, "artifacts/") {
			found = append(found, p)
		}
	}
	return found
}

// A receipt line, a payload, an act and a run's diagnostics, written through
// the one Store every caller is handed. The two session records land on the
// sessions ref and nothing session-shaped is on the ledger; the act and the run
// land on the ledger and nothing else is on the sessions ref. That split is the
// whole of what makes the ledger publishable, and no caller chose it.
func TestASessionRecordIsFiledOnTheSessionsRefAndNeverOnTheLedger(t *testing.T) {
	_, work := fetchLab(t)
	store := NewRefStore(work)
	l := Open(store, run.Start("test", nil))

	if err := l.AppendThread("3k9wq2m1x8b4nptv0zrh7cdfy6", ThreadNative, run.Start("t", nil).Started, func(e Envelope) ([]byte, error) {
		return []byte(`{"v":1,"kind":"thread_native","writer":"` + string(e.Writer) + `"}`), nil
	}); err != nil {
		t.Fatal(err)
	}
	ref, err := l.PutArtifact(SessionTranscriptArtifact, []byte("what the session said"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.Ratify("promise:one", "sha256:"+strings.Repeat("a", 64), 1, "the operator", "", Where{}); err != nil {
		t.Fatal(err)
	}
	if err := l.Close(run.Clean); err != nil {
		t.Fatal(err)
	}

	ledger, sessions := treeOf(t, work, Ref), treeOf(t, work, SessionsRef)
	if leaked := sessionPaths(ledger); len(leaked) > 0 {
		t.Fatalf("the ledger holds session records, which a push would publish: %v", leaked)
	}
	payload, err := ArtifactPath(SessionTranscriptArtifact, ref.Address)
	if err != nil {
		t.Fatal(err)
	}
	// The receipt is looked for where the store files one rather than at a
	// spelled layout: where in the sessions ref's tree a receipt sits is
	// locate's decision and no caller's (docs/decision/0097). That it is on
	// this ref and not the other is what this test is about, and it is the
	// loop below and the ledger's tree above that say so.
	stream, err := ThreadPath("3k9wq2m1x8b4nptv0zrh7cdfy6")
	if err != nil {
		t.Fatal(err)
	}
	_, where := locate(stream)
	for _, want := range []string{where, payload} {
		if !slicesContain(sessions, want) {
			t.Errorf("the sessions ref has no %s: %v", want, sessions)
		}
	}
	// Whatever the layout, a receipt is named by the whole thread id, so
	// `git ls-tree -r | grep <id>` finds it.
	if !strings.HasSuffix(where, "/3k9wq2m1x8b4nptv0zrh7cdfy6.jsonl") {
		t.Errorf("a receipt is filed at %s, which does not name the thread", where)
	}
	for _, p := range sessions {
		if !strings.HasPrefix(p, "threads/") && !strings.HasPrefix(p, "artifacts/") {
			t.Errorf("the sessions ref holds %s, which is not a session record", p)
		}
	}
	if !slicesContain(ledger, "ledger/acts.jsonl") {
		t.Errorf("the act is not on the ledger: %v", ledger)
	}

	// And read back through the same store, from the ref it was filed on, by
	// the logical path the writer used.
	if held, err := ReadArtifact(store, ref); err != nil || string(held) != "what the session said" {
		t.Fatalf("ReadArtifact = %q, %v", held, err)
	}
	if held := receiptLines(t, store, stream); !strings.Contains(string(held), `"kind":"thread_native"`) {
		t.Fatalf("the receipt reads %q", held)
	}
}

// receiptLines is every line of a thread's receipt as the store holds it, each
// sealed span expanded in place.
//
// A run whose store journals files a span as one artifact and names it in the
// receipt's own stream (docs/decision/0087), so reading only that stream's own
// bytes would be asking where this layout keeps a receipt rather than whether
// the store holds it. Both halves are read through the one store, from the ref
// it filed them on, by the logical paths the writer used — which is what the
// caller above is asking. internal/thread does the same expansion on the way
// into a receipt; this is as much of it as this package can ask.
func receiptLines(t *testing.T, store Store, stream string) []byte {
	t.Helper()
	src, err := store.Read(stream)
	if err != nil {
		t.Fatal(err)
	}
	var held []byte
	for _, raw := range bytes.Split(src, []byte{'\n'}) {
		line := bytes.TrimSpace(raw)
		if len(line) == 0 {
			continue
		}
		var entry segmentEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Fatalf("the receipt holds a line that is not a record: %v", err)
		}
		if entry.Segment == nil {
			held = append(append(held, line...), '\n')
			continue
		}
		span, err := ReadArtifact(store, entry.Segment.Artifact)
		if err != nil {
			t.Fatalf("the span %s the receipt names cannot be read: %v", entry.Segment.Artifact.Address, err)
		}
		held = append(held, span...)
	}
	return held
}

// The route is decided by the first segment of the logical path, and by
// nothing a record holds. A path that only mentions threads deeper in, or
// begins with a near spelling, is the ledger's.
func TestTheRouteIsTheFirstSegmentAndNothingElse(t *testing.T) {
	for name, want := range map[string]string{
		"threads/a.jsonl":                     SessionsRef,
		"artifacts/thread.packet/sha256/ab/c": SessionsRef,
		"acts.jsonl":                          Ref,
		"truth/promise/threads.jsonl":         Ref,
		"disposition/promise/x.jsonl":         Ref,
		"observations/sha256/ab/cd":           Ref,
		"runs/abc.json":                       Ref,
		"threadsx/a.jsonl":                    Ref,
		"artifactsx/a":                        Ref,
	} {
		if got, _ := locate(name); got != want {
			t.Errorf("locate(%q) files it on %s, want %s", name, got, want)
		}
	}
}

// A repository that recorded sessions before 0089 keeps them where they are.
// A receipt that was opened then and reworked now reads as one receipt — the
// ledger's half first and the sessions half after it — and a payload filed
// then is read from where it was filed. Nothing is moved, and nothing new is
// written to the old location.
func TestARecordWrittenBeforeTheSessionsRefIsReadWhereItIs(t *testing.T) {
	_, work := fetchLab(t)
	store := NewRefStore(work)

	// The old layout, written the way the old build wrote it: onto Ref, below
	// ledger/threads/ and artifacts/. The older half ends torn, with no final
	// newline, as a killed writer can leave it.
	old := func(where, body string) {
		t.Helper()
		base, held, err := store.revision()
		if err != nil {
			t.Fatal(err)
		}
		commit, err := store.commit(base, held, where, []byte(body), "kept ledger\n")
		if err != nil {
			t.Fatal(err)
		}
		args := []string{"update-ref", Ref, commit}
		if held {
			args = append(args, base)
		}
		fetchGit(t, work, args...)
	}
	old("ledger/threads/t1.jsonl", `{"line":"before"}`)
	old("artifacts/thread.packet/sha256/ab/cdef", "the packet as given")

	if err := store.Append("threads/t1.jsonl", []byte(`{"line":"after"}`)); err != nil {
		t.Fatal(err)
	}
	held, err := store.Read("threads/t1.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if want := "{\"line\":\"before\"}\n{\"line\":\"after\"}\n"; string(held) != want {
		t.Fatalf("the receipt reads %q, want %q", held, want)
	}
	if packet, err := store.Read("artifacts/thread.packet/sha256/ab/cdef"); err != nil || string(packet) != "the packet as given" {
		t.Fatalf("the earlier payload reads %q, %v", packet, err)
	}
	if _, err := store.Read("threads/never.jsonl"); !errors.Is(err, ErrNoStream) {
		t.Fatalf("a thread on neither ref reads %v, want ErrNoStream", err)
	}

	// The new line went to the sessions ref, where the store files a receipt
	// now, and the ledger's half is exactly what it was. The location is read
	// from locate rather than spelled here, for the reason
	// docs/decision/0097 gives: it is the store's decision.
	if code, out := runGit(t, work, "show", Ref+":ledger/threads/t1.jsonl"); code != 0 || out != `{"line":"before"}` {
		t.Fatalf("the ledger's half was rewritten to %q (exit %d)", out, code)
	}
	_, where := locate("threads/t1.jsonl")
	if code, out := runGit(t, work, "show", SessionsRef+":"+where); code != 0 || out != "{\"line\":\"after\"}\n" {
		t.Fatalf("the sessions half reads %q (exit %d)", out, code)
	}
	// And no earlier location was written. The ledger's half above is one of
	// them; the flat sessions path a build before 0097 filed a receipt at is
	// the other, and the write must not have created one there.
	if code, out := runGit(t, work, "cat-file", "-e", SessionsRef+":threads/t1.jsonl"); code == 0 {
		t.Errorf("the write filed a receipt at the flat sessions path, which nothing writes any more: %q", out)
	}
}

// An ordinary fetch of origin never brings the sessions ref, under any name,
// even when origin holds one. Then the same fetch with a sessions record named
// under the ledger's prefix: it does arrive, which is the reason the name is
// what it is, and the proof the first half could have failed.
func TestTheFetchRefspecNeverMatchesTheSessionsRef(t *testing.T) {
	origin, work := fetchLab(t)
	if _, _, _, err := ConfigureFetch(work); err != nil {
		t.Fatal(err)
	}
	store := NewRefStore(work)
	appendAct(t, store, `{"act":"first"}`)
	if err := store.Append("threads/t1.jsonl", []byte(`{"session":"private"}`)); err != nil {
		t.Fatal(err)
	}
	// Put both on origin directly, past any hook, as if somebody had.
	fetchGit(t, work, "push", "-q", "--no-verify", origin, Ref+":"+Ref, SessionsRef+":"+SessionsRef)

	clone := filepath.Join(t.TempDir(), "clone")
	fetchGit(t, filepath.Dir(clone), "init", "-q", clone)
	fetchGit(t, clone, "remote", "add", "--no-tags", "origin", origin)
	fetchGit(t, clone, "config", "--unset-all", "remote.origin.fetch")
	if _, _, _, err := ConfigureFetch(clone); err != nil {
		t.Fatal(err)
	}
	if code, out := fetchOrigin(t, clone); code != 0 {
		t.Fatalf("fetch exited %d:\n%s", code, out)
	}
	if refAt(t, clone, UpstreamRef) == "" {
		t.Fatal("the fetch did not bring the ledger, so it proves nothing about what else it brought")
	}
	code, refs := runGit(t, clone, "for-each-ref", "--format=%(refname)", "refs/kept/")
	if code != 0 {
		t.Fatalf("for-each-ref exited %d:\n%s", code, refs)
	}
	if strings.Contains(refs, "sessions") {
		t.Fatalf("an ordinary fetch of origin brought a sessions ref:\n%s", refs)
	}

	// The name this refused: under the ledger's prefix the same fetch copies
	// it into every clone.
	fetchGit(t, work, "push", "-q", "--no-verify", origin, SessionsRef+":"+Ref+"-sessions")
	if code, out := fetchOrigin(t, clone); code != 0 {
		t.Fatalf("fetch exited %d:\n%s", code, out)
	}
	if refAt(t, clone, UpstreamRef+"-sessions") == "" {
		t.Fatalf("a ref named under %s was not fetched, so the pattern is not what this test assumes", Ref)
	}
}

// sessionsLab is hookLab with a second bare remote, archive, standing for the
// private one a person names.
func sessionsLab(t *testing.T) (origin, archive, work string, store *RefStore) {
	t.Helper()
	origin, work, store = hookLab(t)
	archive = filepath.Join(t.TempDir(), "archive.git")
	if err := os.MkdirAll(archive, 0o755); err != nil {
		t.Fatal(err)
	}
	fetchGit(t, archive, "init", "-q", "--bare")
	fetchGit(t, work, "remote", "add", "--no-tags", "archive", archive)
	if err := store.Append("threads/t1.jsonl", []byte(`{"session":"private"}`)); err != nil {
		t.Fatal(err)
	}
	return origin, archive, work, store
}

// remoteHas is whether a remote holds a ref, asked over the wire.
func remoteHas(t *testing.T, work, remote, ref string) bool {
	t.Helper()
	code, out := runGit(t, work, "ls-remote", remote, ref)
	if code != 0 {
		t.Fatalf("ls-remote %s exited %d:\n%s", remote, code, out)
	}
	return strings.TrimSpace(out) != ""
}

// The pushes that would publish every session kept ran, driven through a real
// hook against a real bare origin: a mirror, a wildcard over refs/kept/, and
// the ref by name. Each is refused, origin never receives the sessions ref, and
// the refusal names what to do. Before a remote is named the refusal names
// kept init; after, it names the one push that is allowed.
func TestThePushThatWouldCarrySessionRecordsToOriginIsRefused(t *testing.T) {
	origin, _, work, _ := sessionsLab(t)

	for _, push := range [][]string{
		{"push", "--mirror", "origin"},
		{"push", "origin", "refs/kept/*:refs/kept/*"},
		{"push", "origin", SessionsRef + ":" + SessionsRef},
	} {
		code, out := runGit(t, work, push...)
		if code == 0 {
			t.Fatalf("git %s exited 0 with no sessions remote named:\n%s", strings.Join(push, " "), out)
		}
		mustSay(t, out, "this push carries "+SessionsRef, "kept init --sessions-remote")
	}
	if remoteHas(t, work, "origin", SessionsRef) {
		t.Fatal("origin received the sessions ref")
	}

	if _, err := ConfigureSessions(work, "archive"); err != nil {
		t.Fatal(err)
	}
	code, out := runGit(t, work, "push", "origin", SessionsRef+":"+SessionsRef)
	if code == 0 {
		t.Fatalf("pushing session records to origin exited 0 with archive named:\n%s", out)
	}
	mustSay(t, out, "session records go only to archive", SessionsPushCommand("archive"))
	if remoteHas(t, work, "origin", SessionsRef) {
		t.Fatal("origin received the sessions ref")
	}
	_ = origin
}

// The named remote is the one place the push goes through, by name or by URL,
// and it leaves the upstream copy doctor compares against. A branch push and a
// ledger push say nothing about session records at all.
func TestTheNamedRemoteReceivesSessionRecordsAndNothingElseIsRefused(t *testing.T) {
	_, archive, work, store := sessionsLab(t)
	if _, err := ConfigureSessions(work, "archive"); err != nil {
		t.Fatal(err)
	}

	code, out := runGit(t, work, "push", "archive", SessionsRef+":"+SessionsRef)
	if code != 0 || strings.Contains(out, "kept:") {
		t.Fatalf("the push to the named remote exited %d:\n%s", code, out)
	}
	if refAt(t, work, SessionsUpstreamRef) != refAt(t, work, SessionsRef) {
		t.Fatal("the push left no upstream copy, so doctor cannot say what the remote holds")
	}

	if err := store.Append("threads/t1.jsonl", []byte(`{"session":"more"}`)); err != nil {
		t.Fatal(err)
	}
	if code, out := runGit(t, work, "push", archive, SessionsRef+":"+SessionsRef); code != 0 {
		t.Fatalf("the push to the named remote's URL exited %d:\n%s", code, out)
	}

	appendAct(t, store, `{"act":"first"}`)
	commit(t, work, "a.txt", "two")
	if code, out := runGit(t, work, "push", "origin", "main", Ref+":"+Ref); code != 0 || strings.Contains(out, "kept:") {
		t.Fatalf("a branch and ledger push said something about session records (exit %d):\n%s", code, out)
	}
}

// A remote named by hand in git config to be origin is still refused at push
// time: the hook reads the destination, not only the setting. And taking the
// sessions ref back off a remote is never refused, because that is how a push
// that got through is undone.
func TestOriginIsRefusedEvenWhenItIsNamedAndADeleteIsNot(t *testing.T) {
	origin, _, work, _ := sessionsLab(t)
	fetchGit(t, work, "config", SessionsRemoteKey, "origin")
	code, out := runGit(t, work, "push", "origin", SessionsRef+":"+SessionsRef)
	if code == 0 {
		t.Fatalf("a push to origin named as the sessions remote exited 0:\n%s", out)
	}
	if remoteHas(t, work, "origin", SessionsRef) {
		t.Fatal("origin received the sessions ref")
	}

	fetchGit(t, work, "push", "-q", "--no-verify", origin, SessionsRef+":"+SessionsRef)
	if code, out := runGit(t, work, "push", "origin", ":"+SessionsRef); code != 0 {
		t.Fatalf("deleting the sessions ref from origin exited %d:\n%s", code, out)
	}
	if remoteHas(t, work, "origin", SessionsRef) {
		t.Fatal("the delete did not take the sessions ref off origin")
	}
}

// What init writes when a person names a remote, and what it refuses before
// writing anything.
func TestConfiguringTheSessionsRemoteRefusesOriginAndWritesOnlyForAPrivateOne(t *testing.T) {
	origin, archive, work, _ := sessionsLab(t)

	fetchGit(t, work, "remote", "add", "mirror", origin)
	for remote, want := range map[string]error{
		"origin":    ErrPublishedRemote,
		"mirror":    ErrPublishedRemote,
		"elsewhere": ErrNotARemote,
	} {
		if _, err := ConfigureSessions(work, remote); !errors.Is(err, want) {
			t.Errorf("ConfigureSessions(%q) = %v, want %v", remote, err, want)
		}
	}
	if code, out := runGit(t, work, "config", "--get", SessionsRemoteKey); code != 1 {
		t.Fatalf("a refused remote was written anyway: %s", out)
	}
	for _, remote := range []string{"origin", "mirror"} {
		if code, out := runGit(t, work, "config", "--get-all", "remote."+remote+".fetch"); strings.Contains(out, SessionsFetchRefspec) {
			t.Fatalf("a refused remote was given the sessions refspec (exit %d): %s", code, out)
		}
	}

	done, err := ConfigureSessions(work, "archive")
	if err != nil || !done.Named || done.Fetch != FetchAdded || done.Remote != "archive" {
		t.Fatalf("ConfigureSessions(archive) = %+v, %v", done, err)
	}
	again, err := ConfigureSessions(work, "")
	if err != nil || again.Named || again.Fetch != FetchUnchanged || again.Remote != "archive" {
		t.Fatalf("a second run = %+v, %v; it should find everything in place", again, err)
	}
	if code, out := runGit(t, work, "config", "--get-all", "remote.archive.fetch"); code != 0 || strings.Count(out, SessionsFetchRefspec) != 1 {
		t.Fatalf("remote.archive.fetch = %q", out)
	}
	_ = archive
}

// The states doctor reads, in the order a repository arrives at them.
func TestWhereSessionRecordsLiveIsReadFromLocalRefsAlone(t *testing.T) {
	_, work := fetchLab(t)
	store := NewRefStore(work)
	if state, err := Sessions(work); err != nil || state != (SessionsState{}) {
		t.Fatalf("a repository that has recorded nothing = %+v, %v", state, err)
	}
	if err := store.Append("threads/t1.jsonl", []byte(`{"line":1}`)); err != nil {
		t.Fatal(err)
	}
	if state, _ := Sessions(work); !state.Unbacked() {
		t.Fatalf("records with no remote named = %+v, want unbacked", state)
	}

	archive := filepath.Join(t.TempDir(), "archive.git")
	fetchGit(t, filepath.Dir(archive), "init", "-q", "--bare", archive)
	fetchGit(t, work, "remote", "add", "--no-tags", "archive", archive)
	fetchGit(t, work, "config", SessionsRemoteKey, "archive")
	if state, _ := Sessions(work); state.Tracked || state.Unheard() {
		t.Fatalf("a remote named by hand without the refspec = %+v, want untracked", state)
	}
	if _, err := ConfigureSessions(work, ""); err != nil {
		t.Fatal(err)
	}
	if state, _ := Sessions(work); !state.Unheard() {
		t.Fatalf("records nothing has pushed = %+v, want unheard", state)
	}

	fetchGit(t, work, "push", "-q", "archive", SessionsRef+":"+SessionsRef)
	if state, _ := Sessions(work); state.Unpublished() || state.Unheard() || state.Diverged() {
		t.Fatalf("records just pushed = %+v, want quiet", state)
	}
	if err := store.Append("threads/t1.jsonl", []byte(`{"line":2}`)); err != nil {
		t.Fatal(err)
	}
	state, _ := Sessions(work)
	if !state.Unpublished() || state.Ahead != 1 {
		t.Fatalf("one record past the push = %+v, want one ahead", state)
	}
}

// A hook kept wrote before the sessions guard is kept's own, outdated, and
// never rewritten. One with the guard is present.
func TestAHookWrittenBeforeTheSessionsGuardIsOutdatedAndLeftAlone(t *testing.T) {
	_, work := fetchLab(t)
	h, err := ConfigureHook(work)
	if err != nil || h.State != HookWritten {
		t.Fatalf("ConfigureHook = %+v, %v", h, err)
	}
	if now, _ := HookState(work); now.State != HookPresent {
		t.Fatalf("a hook with the guard reads %q, want %q", now.State, HookPresent)
	}

	earlier := strings.Replace(HookBody, HookSessionsMarker+"\n", "", 1)
	if earlier == HookBody {
		t.Fatal("the hook body does not carry the sessions marker on a line of its own")
	}
	if err := os.WriteFile(h.Path, []byte(earlier), 0o755); err != nil {
		t.Fatal(err)
	}
	again, err := ConfigureHook(work)
	if err != nil || again.State != HookOutdated {
		t.Fatalf("ConfigureHook over an earlier hook = %+v, %v; want %q", again, err, HookOutdated)
	}
	if held, _ := os.ReadFile(h.Path); string(held) != earlier {
		t.Fatal("kept rewrote its own earlier hook")
	}
}

func slicesContain(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
