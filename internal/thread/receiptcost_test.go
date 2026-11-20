package thread

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/run"
)

// What a receipt write may cost the ledger, as the candidate promise in
// docs/desire/a-thread-receipt-costs-the-ledger-about-what-it-holds.md states
// it: each ledger commit made for a thread receipt introduces new git objects
// totaling at most receiptWriteFactor times the bytes it records, plus
// receiptWriteAllowance.
//
// The quantity is what kept hands git, not what git stores. Packed size depends
// on which deltas git chooses and how deep it lets a chain go, and kept
// controls neither; the uncompressed size of the objects a commit introduces is
// deterministic and is something kept writes. Bounded per write, the total is
// linear in the session whatever git packs afterward.
const (
	receiptWriteFactor    = 2
	receiptWriteAllowance = 64 << 10
)

// receiptReplayBytes is how much recorded runtime output the replay feeds the
// receipt. It has to carry the receipt past receiptWriteAllowance, or a writer
// that rewrites the whole receipt on every line would pass on a fixture too
// small to show the curve.
const receiptReplayBytes = 80 << 10

// replaySecret is the credential the replayed stream is redacted against. The
// captured session carries none, so nothing is redacted, but the adapter's
// stream is the one a real session writes through.
const replaySecret = "test-claude-setup-token"

// TestEachReceiptWriteIsBoundedByTheLinesItRecords replays a captured Claude
// session through the adapter's stream into a ledger on a real ref, then reads
// every commit that ledger received and holds each one to the bound.
//
// Per write rather than per session, so the check does not depend on the
// fixture: a layout whose writes grow with the receipt fails it on the first
// write past the allowance, however long the session is. And over every
// commit, not only the ones naming threads/, because a transcript fragment or
// an undecodable item is written to artifacts/ on a receipt's behalf and costs
// exactly as a receipt line does.
//
// The commits read are the sessions ref's, because that is where a receipt and
// its payloads are filed (docs/decision/0089). Reading the ledger's instead
// would find only the run's diagnostics and pass without measuring a receipt
// at all.
func TestEachReceiptWriteIsBoundedByTheLinesItRecords(t *testing.T) {
	repository := receiptRepository(t)
	store := ledger.NewRefStore(repository)
	receipt, err := NewReceipt(ledger.Open(store, run.Start("test", nil)), MintID())
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := receipt.ForAdapter(Claude, "replay")
	if err != nil {
		t.Fatal(err)
	}
	stream := &claudeStream{receipt: adapter, secret: []byte(replaySecret)}

	session := capturedSession(t)
	replayed, lines := 0, 0
	for replayed < receiptReplayBytes {
		if _, err := stream.Write(session); err != nil {
			t.Fatal(err)
		}
		replayed += len(session)
		lines += bytes.Count(session, []byte{'\n'})
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}

	// The replay is evidence only if it reached the ledger whole, and only if
	// the receipt it left is large enough for the bound to be able to fire.
	entries, quarantined, err := ReadReceipt(store, receipt.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(quarantined) != 0 {
		t.Fatalf("the replay quarantined its own evidence: %#v", quarantined)
	}
	if natives := nativeCount(entries); natives != lines {
		t.Fatalf("the receipt holds %d native events for %d replayed lines", natives, lines)
	}
	where, err := ledger.ThreadPath(string(receipt.ID()))
	if err != nil {
		t.Fatal(err)
	}
	held, err := store.Read(where)
	if err != nil {
		t.Fatal(err)
	}
	if len(held) <= receiptWriteAllowance {
		t.Fatalf("the replay left a %d-byte receipt, which the %d-byte allowance alone admits, so the bound could not fire", len(held), receiptWriteAllowance)
	}

	for k, write := range ledgerWrites(t, repository) {
		limit := receiptWriteFactor*write.recorded + receiptWriteAllowance
		if write.introduced > limit {
			t.Fatalf("ledger write %d (%s) introduced %d bytes of new objects to record %d bytes; the bound is %d × %d + %d = %d",
				k+1, strings.Join(write.paths, ", "), write.introduced, write.recorded, receiptWriteFactor, write.recorded, receiptWriteAllowance, limit)
		}
	}
}

// ledgerWrite is one commit on the ledger's ref: what it introduced, and what
// it recorded.
type ledgerWrite struct {
	commit     string
	paths      []string
	introduced int // uncompressed bytes of every object this commit made reachable that its parent did not
	recorded   int // bytes the files it changed grew by
}

// ledgerWrites reads every commit the ref a receipt is filed on holds, oldest
// first.
func ledgerWrites(t *testing.T, repository string) []ledgerWrite {
	t.Helper()
	commits := strings.Fields(receiptGit(t, repository, nil, "rev-list", "--reverse", ledger.SessionsRef))
	var writes []ledgerWrite
	for _, commit := range commits {
		parent := ""
		if out, err := receiptGitErr(repository, nil, "rev-parse", "--verify", "--quiet", commit+"^"); err == nil {
			parent = strings.TrimSpace(out)
		}

		listArgs := []string{"rev-list", "--objects", commit}
		diffArgs := []string{"diff-tree", "-r", "--raw", "--no-commit-id", "--no-abbrev"}
		if parent == "" {
			diffArgs = append(diffArgs, "--root", commit)
		} else {
			listArgs = append(listArgs, "--not", parent)
			diffArgs = append(diffArgs, parent, commit)
		}

		var objects []string
		for line := range strings.SplitSeq(strings.TrimSpace(receiptGit(t, repository, nil, listArgs...)), "\n") {
			if fields := strings.Fields(line); len(fields) > 0 {
				objects = append(objects, fields[0])
			}
		}
		write := ledgerWrite{commit: commit}
		for _, size := range objectSizes(t, repository, objects) {
			write.introduced += size
		}

		// :<old mode> <new mode> <old oid> <new oid> <status>\t<path>
		var olds, news []string
		for line := range strings.SplitSeq(strings.TrimSpace(receiptGit(t, repository, nil, diffArgs...)), "\n") {
			meta, path, found := strings.Cut(line, "\t")
			fields := strings.Fields(meta)
			if !found || len(fields) < 5 {
				continue
			}
			write.paths = append(write.paths, path)
			olds = append(olds, fields[2])
			news = append(news, fields[3])
		}
		oldSizes, newSizes := objectSizes(t, repository, olds), objectSizes(t, repository, news)
		for i := range news {
			if grew := newSizes[i] - oldSizes[i]; grew > 0 {
				write.recorded += grew
			}
		}
		writes = append(writes, write)
	}
	return writes
}

// objectSizes is each object's uncompressed size, in order. The all-zero id a
// diff names for a file that did not exist is size zero.
func objectSizes(t *testing.T, repository string, objects []string) []int {
	t.Helper()
	sizes := make([]int, len(objects))
	var ask []string
	var at []int
	for i, oid := range objects {
		if strings.Trim(oid, "0") == "" {
			continue
		}
		ask = append(ask, oid)
		at = append(at, i)
	}
	if len(ask) == 0 {
		return sizes
	}
	out := receiptGit(t, repository, []byte(strings.Join(ask, "\n")+"\n"), "cat-file", "--batch-check=%(objectsize)")
	got := strings.Fields(out)
	if len(got) != len(ask) {
		t.Fatalf("cat-file sized %d of %d objects: %q", len(got), len(ask), out)
	}
	for k, field := range got {
		size, err := strconv.Atoi(field)
		if err != nil {
			t.Fatalf("cat-file size %q: %v", field, err)
		}
		sizes[at[k]] = size
	}
	return sizes
}

// capturedSession is a real Claude session's native stream, the one
// TestClaudeProjectsItsCapturedConfinedSession reads, ending in a newline so it
// can be replayed back to back.
func capturedSession(t testing.TB) []byte {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("testdata", "claude", "session.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasSuffix(src, []byte{'\n'}) {
		src = append(src, '\n')
	}
	return src
}

func nativeCount(entries []Entry) int {
	n := 0
	for _, entry := range entries {
		if entry.Native != nil {
			n++
		}
	}
	return n
}

// receiptRepository is an empty repository for a ledger to live on.
func receiptRepository(t testing.TB) string {
	t.Helper()
	dir := t.TempDir()
	if _, err := receiptGitErr(dir, nil, "init", "-q"); err != nil {
		t.Fatal(err)
	}
	return dir
}

func receiptGit(t testing.TB, repository string, stdin []byte, args ...string) string {
	t.Helper()
	out, err := receiptGitErr(repository, stdin, args...)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func receiptGitErr(repository string, stdin []byte, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", repository}, args...)...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}
