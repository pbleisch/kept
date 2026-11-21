package ledger

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/ids"
)

// What a write to one thread's receipt may cost in the trees it rewrites, as
// promise:a-receipt-write-does-not-grow-with-the-threads-beside-it states it:
// in a sessions ref that already holds a receipt for each of receiptNeighbours
// threads, every commit a receipt write makes introduces tree objects totaling
// at most receiptNeighbourTreeBound bytes.
//
// Trees and nothing else, because the trees are what the number of other
// threads moves. A commit writes one blob for the file it changed and a new
// tree for every directory on that file's path, so a directory holding every
// thread's receipt is rewritten whole, about sixty bytes an entry, on every
// write to any of them. The blob is the receipt's own cost and is
// promise:a-receipt-costs-the-ledger-about-what-it-holds's to bound. See
// docs/decision/0097.
const (
	receiptNeighbours         = 10_000
	receiptNeighbourTreeBound = 16 << 10
)

// TestAReceiptWriteDoesNotGrowWithTheThreadsBesideIt seeds a sessions ref with
// a receipt for each of receiptNeighbours threads, appends to one of them and
// opens one more, and holds every commit those two writes made to the bound.
//
// The seed is filed where the store files a receipt: each thread's path comes
// from ThreadPath and its location from locate, which is the one place that
// decides where a record sits in a ref's tree. So the test names no layout. A
// layout that decides elsewhere leaves the seed where the store does not read
// it, and the first guard below says so rather than measuring a tree the
// writer never touches.
func TestAReceiptWriteDoesNotGrowWithTheThreadsBesideIt(t *testing.T) {
	work := neighbourRepository(t)
	store := NewRefStore(work)

	seeded := make([]string, receiptNeighbours)
	files := make(map[string]string, receiptNeighbours)
	for k := range seeded {
		seeded[k] = ids.Mint()
		name, err := ThreadPath(seeded[k])
		if err != nil {
			t.Fatal(err)
		}
		ref, where := locate(name)
		if ref != SessionsRef {
			t.Fatalf("a receipt is filed on %s, and docs/decision/0089 files it on %s", ref, SessionsRef)
		}
		files[where] = seededLine
	}
	seedRef(t, work, SessionsRef, files)

	// The seed is evidence only if the store reads it where it was put.
	for _, id := range []string{seeded[0], seeded[len(seeded)/2], seeded[len(seeded)-1]} {
		if held := readReceiptStream(t, store, id); held != seededLine {
			t.Fatalf("thread %s was seeded where locate files it and the store reads %q there, so the layout is decided somewhere locate does not see", id, held)
		}
	}

	existing, opened := seeded[len(seeded)/3], ids.Mint()
	for _, write := range []struct{ what, thread string }{
		{"an append to a receipt that has a line", existing},
		{"the first line of a new thread's receipt", opened},
	} {
		before := strings.TrimSpace(neighbourGit(t, work, "rev-parse", "--verify", SessionsRef))
		name, err := ThreadPath(write.thread)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Append(name, []byte(`{"line":"measured"}`)); err != nil {
			t.Fatal(err)
		}
		after := strings.TrimSpace(neighbourGit(t, work, "rev-parse", "--verify", SessionsRef))
		commits := strings.Fields(neighbourGit(t, work, "rev-list", "--reverse", before+".."+after))
		if len(commits) == 0 {
			t.Fatalf("%s made no commit on %s, so there is no write for the bound to judge", write.what, SessionsRef)
		}
		for _, commit := range commits {
			trees := introducedTreeBytes(t, work, commit)
			t.Logf("%s made commit %s introducing %d bytes of tree objects", write.what, commit, trees)
			if trees > receiptNeighbourTreeBound {
				t.Fatalf("%s, in a sessions ref holding %d other threads, made commit %s introducing %d bytes of tree objects; the bound is %d",
					write.what, receiptNeighbours, commit, trees, receiptNeighbourTreeBound)
			}
		}
	}

	// Small trees are worth nothing if the writer got them by leaving the
	// other threads behind. Every seeded receipt is still on the ref, and both
	// measured receipts read back whole.
	if held := len(strings.Fields(neighbourGit(t, work, "ls-tree", "-r", "--name-only", SessionsRef))); held < receiptNeighbours+1 {
		t.Fatalf("the sessions ref holds %d files after the writes, fewer than the %d receipts written to it", held, receiptNeighbours+1)
	}
	if held := readReceiptStream(t, store, seeded[len(seeded)-1]); held != seededLine {
		t.Fatalf("a seeded thread nothing wrote to reads %q after the writes", held)
	}
	if held, want := readReceiptStream(t, store, existing), seededLine+"{\"line\":\"measured\"}\n"; held != want {
		t.Fatalf("the receipt appended to reads %q, want %q", held, want)
	}
	if held, want := readReceiptStream(t, store, opened), "{\"line\":\"measured\"}\n"; held != want {
		t.Fatalf("the receipt opened reads %q, want %q", held, want)
	}
}

// TestAReceiptFiledBeforeTheLayoutMovedIsReadWhole writes one thread's receipt
// the way kept has filed receipts before, and asks the store for it by the path
// every reader uses.
//
// The two earlier locations are history and are spelled out for that reason:
// below ledger/threads/ on the ledger before docs/decision/0089, and at
// threads/<id>.jsonl on the sessions ref after it. A thread opened under either
// and reworked now has a half in each and a third wherever new lines go, and it
// reads as one receipt in that order. Nothing already written is moved.
func TestAReceiptFiledBeforeTheLayoutMovedIsReadWhole(t *testing.T) {
	work := neighbourRepository(t)
	store := NewRefStore(work)
	thread := ids.Mint()

	ledgerHalf := "ledger/threads/" + thread + ".jsonl"
	sessionsHalf := "threads/" + thread + ".jsonl"
	seedRef(t, work, Ref, map[string]string{ledgerHalf: "{\"line\":\"before the sessions ref\"}\n"})
	seedRef(t, work, SessionsRef, map[string]string{sessionsHalf: "{\"line\":\"on the sessions ref\"}\n"})

	name, err := ThreadPath(thread)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Append(name, []byte(`{"line":"now"}`)); err != nil {
		t.Fatal(err)
	}

	want := "{\"line\":\"before the sessions ref\"}\n{\"line\":\"on the sessions ref\"}\n{\"line\":\"now\"}\n"
	if held := readReceiptStream(t, store, thread); held != want {
		t.Fatalf("the receipt reads %q, want %q", held, want)
	}
	if held := neighbourGit(t, work, "show", Ref+":"+ledgerHalf); held != "{\"line\":\"before the sessions ref\"}\n" {
		t.Fatalf("the ledger's half reads %q after the write", held)
	}
	if held := neighbourGit(t, work, "show", SessionsRef+":"+sessionsHalf); !strings.HasPrefix(held, "{\"line\":\"on the sessions ref\"}\n") {
		t.Fatalf("the sessions half filed before the layout moved reads %q after the write", held)
	}
}

const seededLine = "{\"line\":\"seeded\"}\n"

// readReceiptStream is a thread's receipt stream as the store reads it, by the
// path every reader names it with.
func readReceiptStream(t *testing.T, store *RefStore, thread string) string {
	t.Helper()
	name, err := ThreadPath(thread)
	if err != nil {
		t.Fatal(err)
	}
	held, err := store.Read(name)
	if errors.Is(err, ErrNoStream) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(held)
}

// seedRef points ref at one commit holding files, on top of whatever ref held.
// It is one commit however many files, through a scratch index, so ten thousand
// receipts cost a second rather than ten thousand writes.
func seedRef(t *testing.T, work, ref string, files map[string]string) {
	t.Helper()
	index := t.TempDir() + "/index"
	env := []string{"GIT_INDEX_FILE=" + index}
	parent := ""
	if out, code := neighbourGitCode(t, work, nil, nil, "rev-parse", "--verify", "--quiet", ref); code == 0 {
		parent = strings.TrimSpace(out)
		neighbourGitEnv(t, work, env, nil, "read-tree", parent)
	}
	blobs := map[string]string{}
	var entries strings.Builder
	for where, body := range files {
		blob, ok := blobs[body]
		if !ok {
			blob = strings.TrimSpace(neighbourGitEnv(t, work, nil, []byte(body), "hash-object", "-w", "--stdin"))
			blobs[body] = blob
		}
		fmt.Fprintf(&entries, "100644 blob %s\t%s\n", blob, where)
	}
	neighbourGitEnv(t, work, env, []byte(entries.String()), "update-index", "--index-info")
	tree := strings.TrimSpace(neighbourGitEnv(t, work, env, nil, "write-tree"))
	args := []string{"commit-tree", tree, "-m", "seed"}
	if parent != "" {
		args = append(args, "-p", parent)
	}
	commit := strings.TrimSpace(neighbourGitEnv(t, work, nil, nil, args...))
	neighbourGit(t, work, "update-ref", ref, commit)
}

// introducedTreeBytes is the uncompressed size of every tree object commit made
// reachable that its parents did not.
func introducedTreeBytes(t *testing.T, work, commit string) int {
	t.Helper()
	args := []string{"rev-list", "--objects", "--no-object-names", commit}
	for parent := range strings.FieldsSeq(neighbourGit(t, work, "log", "-1", "--format=%P", commit)) {
		args = append(args, "^"+parent)
	}
	objects := neighbourGit(t, work, args...)
	sized := neighbourGitEnv(t, work, nil, []byte(objects), "cat-file", "--batch-check=%(objecttype) %(objectsize)")
	total := 0
	for line := range strings.SplitSeq(strings.TrimSpace(sized), "\n") {
		kind, size, ok := strings.Cut(line, " ")
		if !ok || kind != "tree" {
			continue
		}
		n, err := strconv.Atoi(size)
		if err != nil {
			t.Fatalf("cat-file reported %q", line)
		}
		total += n
	}
	return total
}

func neighbourRepository(t *testing.T) string {
	t.Helper()
	work := t.TempDir()
	neighbourGit(t, work, "init", "-q")
	return work
}

func neighbourGit(t *testing.T, work string, args ...string) string {
	t.Helper()
	return neighbourGitEnv(t, work, nil, nil, args...)
}

func neighbourGitEnv(t *testing.T, work string, env []string, input []byte, args ...string) string {
	t.Helper()
	out, code := neighbourGitCode(t, work, env, input, args...)
	if code != 0 {
		t.Fatalf("git %s exited %d:\n%s", strings.Join(args, " "), code, out)
	}
	return out
}

func neighbourGitCode(t *testing.T, work string, env []string, input []byte, args ...string) (string, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "git", append([]string{"-C", work}, args...)...)
	command.Env = append(os.Environ(), append([]string{
		"GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0",
		"GIT_AUTHOR_NAME=kept", "GIT_AUTHOR_EMAIL=kept@invalid",
		"GIT_COMMITTER_NAME=kept", "GIT_COMMITTER_EMAIL=kept@invalid",
	}, env...)...)
	if input != nil {
		command.Stdin = strings.NewReader(string(input))
	}
	out, err := command.Output()
	if ctx.Err() != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), ctx.Err())
	}
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		return string(out) + string(exit.Stderr), exit.ExitCode()
	}
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return string(out), 0
}
