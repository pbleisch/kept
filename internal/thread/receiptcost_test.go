package thread

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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

// receiptWriteSurplus is the second thing each write is held to: how many bytes
// it may introduce that it did not record.
//
// **Why the clause above is not enough on its own.** The write whose cost grows
// with the receipt is not the one that records a line. Under
// docs/decision/0087 a receipt's lines reach the ref as sealed spans, each span
// written once under its content address; what grows is the index, because the
// entry naming a span is appended to the receipt's own stream. While that
// stream was one file, the Nth such append handed git a blob of all N index
// entries while recording only the bytes of the last — and an index entry is
// 421 bytes, so the clause above, which allows 64 KiB over what a write
// recorded, was not crossed until the 158th seal. That is about 40 MiB of
// receipt: a long session, but far more replay than a suite can run. A test
// sized below it cannot fail on a layout that rewrites the whole receipt, and
// the one this replaces was sized below it.
//
// **What this measures instead.** The surplus is the quantity that must not
// grow: a write records the line it was given and introduces that line plus
// whatever the layout makes it carry. ledger.ReceiptPartBytes is what the store
// says it carries — the largest file it will hand git for a receipt — and
// receiptTreeAllowance is the trees on that file's path, which a sessions ref
// holding one thread's receipt and its payloads keeps small. Their sum is
// crossed by a whole-receipt rewrite after 59 index entries rather than 158,
// which is a fixture that runs in seconds.
//
// It is tighter than the promise's clause and deliberately so: it is the
// mechanism that makes the clause true at every receipt size rather than only
// below 40 MiB. Both are asserted, and the clause is asserted first, so a
// failure says which of the two was broken. The clause measured over an append
// of any length is
// TestAReceiptAppendDoesNotHandGitTheWholeReceipt in internal/ledger.
const (
	receiptTreeAllowance = 8 << 10
	receiptWriteSurplus  = ledger.ReceiptPartBytes + receiptTreeAllowance
)

// receiptReplayBytes is how much recorded runtime output the replay feeds the
// receipt, receiptReplaySeals how many times the run files what it journaled,
// and receiptReplayWrites the fewest writes the ref must receive.
//
// receiptReplayBytes is a multiple of ledger.SegmentThreshold so that the
// fixture is a session's worth of output rather than a handful of lines: the
// receipt it leaves holds several segments and is far past the allowance, which
// is what the guards below check before measuring anything.
//
// receiptReplaySeals is sized from receiptWriteSurplus: 72 index entries of 421
// bytes is 30 KB of index, which a layout holding the index in one file would
// hand git whole and which is past the surplus with margin either side. A seal
// is also the only write a session makes many of, so this is the number that
// decides whether the window holds a write made after the receipt's stream
// rolled.
//
// The seals are asked for rather than waited for. ledger.SegmentThreshold
// decides how often a run seals on its own — 72 of them would be 18 MiB of
// replay — and a seal is the same write whether a threshold, a lifecycle event
// or the end of a session triggered it. What the bound is about is what one
// seal's write costs, so the fixture triggers them directly and spends its
// bytes on being a real session's stream instead.
const (
	receiptReplayBytes  = 4 * ledger.SegmentThreshold
	receiptReplaySeals  = 72
	receiptReplayWrites = 8
)

// replaySecret is the credential the replayed stream is redacted against. The
// captured session carries none, so nothing is redacted, but the adapter's
// stream is the one a real session writes through.
const replaySecret = "test-claude-setup-token"

// TestEachReceiptWriteIsBoundedByTheLinesItRecords replays a captured Claude
// session through the adapter's stream into a ledger on a real ref, then reads
// every commit that ledger received and holds each one to the bound.
//
// Per write rather than per session, because that is the form the statement
// takes and the form a growing write fails: one write past the allowance is a
// failure whatever the session's total comes to.
//
// It does depend on the fixture, and says how. A write can only be seen to grow
// if the window holds more than one of it, so the replay is sized to be written
// to the ref several times and refuses a window that was not — see
// receiptReplayBytes. What the per-write form buys is that the growth shows as
// one write exceeding its own bound rather than as a total somebody has to
// interpret.
//
// And over every commit, not only the ones naming threads/, because a
// transcript fragment or an undecodable item is written to artifacts/ on a
// receipt's behalf and costs exactly as a receipt line does.
//
// The commits read are the sessions ref's, because that is where a receipt and
// its payloads are filed (docs/decision/0089). Reading the ledger's instead
// would find only the run's diagnostics and pass without measuring a receipt
// at all.
func TestEachReceiptWriteIsBoundedByTheLinesItRecords(t *testing.T) {
	repository := receiptRepository(t)
	store := ledger.NewRefStore(repository)
	filed := ledger.Open(store, run.Start("test", nil))
	receipt, err := NewReceipt(filed, MintID())
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := receipt.ForAdapter(Claude, "replay")
	if err != nil {
		t.Fatal(err)
	}
	stream := &claudeStream{receipt: adapter, secret: []byte(replaySecret)}

	// The replay, in receiptReplaySeals spans: the captured session's lines
	// through the adapter's stream, and then the run filing what it journaled,
	// which is the write the bound is about. See receiptReplaySeals.
	session := capturedSession(t)
	replayed, lines := 0, 0
	for seal := range receiptReplaySeals {
		for span := 0; span < receiptReplayBytes/receiptReplaySeals; span += len(session) {
			if _, err := stream.Write(session); err != nil {
				t.Fatal(err)
			}
			replayed += len(session)
			lines += bytes.Count(session, []byte{'\n'})
		}
		if err := filed.SealThread(string(receipt.ID())); err != nil {
			t.Fatalf("seal %d of %d: %v", seal+1, receiptReplaySeals, err)
		}
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}

	// The replay is evidence only if it reached the sessions ref whole, and
	// only if the receipt it left is large enough for the bound to be able to
	// fire.
	//
	// Both are asked of what every layout answers, never of where one layout
	// keeps the receipt. The receipt is read back through ReadReceipt, which
	// is the one reader a layout has to satisfy, and its size is the bytes of
	// the lines it holds: a layout that keeps them anywhere but the one file at
	// the thread's path — the sealed segments decision 0087 describes — leaves
	// that file small, and a guard reading it would refuse every such layout
	// as a fixture too small for the bound. And the ref is asked how much its
	// writes recorded, because a layout that kept the lines somewhere else
	// entirely would make no write for the bound to judge and pass it with
	// nothing measured.
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
	if held := nativeBytes(entries); held <= receiptWriteAllowance {
		t.Fatalf("the replay left a receipt holding %d bytes of lines, which the %d-byte allowance alone admits, so the bound could not fire", held, receiptWriteAllowance)
	}
	writes := ledgerWrites(t, repository)
	recorded := 0
	for _, write := range writes {
		recorded += write.recorded
	}
	if recorded <= receiptWriteAllowance {
		t.Fatalf("the sessions ref's %d writes recorded %d bytes, which the %d-byte allowance alone admits, so the receipt did not reach the ref for the bound to judge", len(writes), recorded, receiptWriteAllowance)
	}
	// And the window has to hold enough writes for a growing one to be visible
	// in it. A receipt reaching the ref in a single write satisfies the bound
	// for a reason the bound is not about — there was nothing before it to grow
	// from — and the allowance alone would then carry a layout whose second
	// write, or hundredth, costs the whole receipt again.
	//
	// Asked of the window rather than of any particular path, because which of
	// a layout's writes is the one that grows is the layout's business. That
	// several were made, so that some write was made twice, is what makes the
	// window able to show it.
	if len(writes) < receiptReplayWrites {
		t.Fatalf("the replay of %d bytes reached the sessions ref in %d writes; fewer than %d cannot show a write growing with the receipt, so %d bytes is no longer a fixture this bound can be measured on",
			replayed, len(writes), receiptReplayWrites, replayed)
	}

	// And the window has to hold as many writes to the receipt's own stream as
	// seals were asked for, because that stream is the one a layout rewrites
	// whole. Fewer means the index the surplus is measured against was never
	// built, and everything below was measured on a receipt small enough that
	// no layout had to carry anything.
	//
	// Counted by the logical prefix every receipt stream is named with, which is
	// ThreadPath's own and not a layout's: where under it the store files the
	// lines is the store's business, and a layout that wrote them outside it
	// would be caught by the count of lines read back above.
	if streamed := streamWrites(writes); streamed < receiptReplaySeals {
		t.Fatalf("the replay wrote the receipt's own stream %d times for %d seals asked for, so the index a write would grow with was not built in this window", streamed, receiptReplaySeals)
	}

	worst, worstAt := 0, 0
	for k, write := range writes {
		limit := receiptWriteFactor*write.recorded + receiptWriteAllowance
		if write.introduced > limit {
			t.Fatalf("ledger write %d (%s) introduced %d bytes of new objects to record %d bytes; the bound is %d × %d + %d = %d",
				k+1, strings.Join(write.paths, ", "), write.introduced, write.recorded, receiptWriteFactor, write.recorded, receiptWriteAllowance, limit)
		}
		// The surplus, which is what must not grow with the receipt. See
		// receiptWriteSurplus.
		if surplus := write.introduced - write.recorded; surplus > worst {
			worst, worstAt = surplus, k+1
		}
	}
	t.Logf("%d writes recorded %d bytes; the costliest carried %d bytes it did not record, at write %d of %d",
		len(writes), recorded, worst, worstAt, len(writes))
	if worst > receiptWriteSurplus {
		write := writes[worstAt-1]
		t.Fatalf("ledger write %d of %d (%s) introduced %d bytes of new objects to record %d, carrying %d bytes it did not record; a write may carry %d + %d = %d, which is the largest file this store hands git for a receipt and the trees on its path",
			worstAt, len(writes), strings.Join(write.paths, ", "), write.introduced, write.recorded, worst, ledger.ReceiptPartBytes, receiptTreeAllowance, receiptWriteSurplus)
	}
}

// streamWrites is how many of these writes changed a receipt's own stream
// rather than a payload a receipt names.
func streamWrites(writes []ledgerWrite) int {
	n := 0
	for _, write := range writes {
		if slices.ContainsFunc(write.paths, func(where string) bool { return strings.HasPrefix(where, "threads/") }) {
			n++
		}
	}
	return n
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

// nativeBytes is how many bytes of runtime output a receipt holds, as its
// entries read back.
func nativeBytes(entries []Entry) int {
	n := 0
	for _, entry := range entries {
		if entry.Native != nil {
			n += len(entry.Native.Payload)
		}
	}
	return n
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
