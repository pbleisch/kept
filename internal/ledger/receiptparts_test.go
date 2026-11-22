package ledger

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/ids"
)

// What one append to a receipt may hand git, as
// promise:a-receipt-costs-the-ledger-about-what-it-holds states it: a write
// made for a thread receipt introduces new git objects totaling at most
// receiptPartFactor times the bytes of the lines it records, plus
// receiptPartAllowance.
//
// The same clause internal/thread measures over a replayed session, measured
// here over the operation it is a property of. A receipt reaches the ref
// through [RefStore.Append] and nothing else, so this is where the clause is
// either true of every receipt size or true of the sizes a fixture happened to
// reach.
const (
	receiptPartFactor    = 2
	receiptPartAllowance = 64 << 10
)

// receiptPartLine is how long each appended line is, and receiptPartAppends how
// many are appended.
//
// Sized so that the clause is actually crossed by a layout that hands git the
// whole receipt on every append, and crossed early enough to measure in a
// suite. The two pull the same way: an append of L bytes to a file of N such
// lines introduces N × L and is allowed 2L + 64 KiB, so a longer line crosses
// in fewer appends — 4 KiB lines cross at the 18th, where 421-byte index
// entries take 158 and 55-byte ones take 1,194. 4 KiB is also a line a receipt
// really carries: an index entry is 421 bytes, and a measurement or a brief the
// parent appends is kilobytes.
//
// 32 appends is past the 18th with margin, and eleven parts rather than two, so
// the cost measured is a cost after the stream has rolled several times and not
// just after it first did.
const (
	receiptPartLine    = 4 << 10
	receiptPartAppends = 32
)

// TestAReceiptAppendDoesNotHandGitTheWholeReceipt appends to one thread's
// receipt until the receipt is many times the largest file the store will write,
// and holds every commit it made to the per-write clause.
//
// **What this would have caught.** Until [ReceiptPartBytes] a receipt was one
// file, and a commit changing a file hands git the whole new file. Measured on
// the tip this test was written against, with the 421-byte index entry a seal
// appends: the 2nd append introduced 1,165 bytes, the 50th 21,373, the 200th
// 84,523 — against a bound of 66,378 — growing by the size of the receipt for
// ever. At 4 KiB a line the same layout fails here on the 18th append. So the
// clause was false of any receipt past about 40 MiB, which is a long session
// rather than a hypothetical one, and no fixture small enough to run in a suite
// would say so unless it was sized to.
//
// **Why the line length and not the session is the fixture.** internal/thread
// measures this clause over a real replayed session, which is the form the
// promise names, and a session's own writes are 421 bytes at a time: reaching
// the 158th of them costs 158 seals and most of a minute. The clause is about
// one append against one line, so the cheapest fixture that can fail is long
// lines rather than many short ones. Both tests measure the same quantity over
// the same operation.
//
// **What it asks of the store and not of the layout.** The cost is read from
// git — every object a commit made reachable that its parent did not — and the
// lines are read back through the one reader a receipt has. Where the store put
// them is its business; that it did not hand git the whole receipt to record one
// line, and that every line is still there in order, is not.
func TestAReceiptAppendDoesNotHandGitTheWholeReceipt(t *testing.T) {
	work := neighbourRepository(t)
	store := NewRefStore(work)
	thread := ids.Mint()
	name, err := ThreadPath(thread)
	if err != nil {
		t.Fatal(err)
	}

	// Each line is its own ordinal followed by filler, so the read back below
	// shows the order the appends were made in and not only their number.
	lines := make([][]byte, receiptPartAppends)
	for k := range lines {
		head := fmt.Sprintf(`{"v":1,"line":%d,"filler":"`, k+1)
		line := append([]byte(head), bytes.Repeat([]byte("x"), receiptPartLine-len(head)-2)...)
		lines[k] = append(line, '"', '}')
	}

	limit := receiptPartFactor*(receiptPartLine+1) + receiptPartAllowance
	worst, worstAt := 0, 0
	for k, line := range lines {
		before := refValue(t, work, SessionsRef)
		if err := store.Append(name, line); err != nil {
			t.Fatal(err)
		}
		after := refValue(t, work, SessionsRef)

		// One commit, so what it introduced is attributable to this append. A
		// store that batched appends would be measured per batch, and the guard
		// below on how much it recorded is what would notice.
		commits := strings.Fields(neighbourGit(t, work, "rev-list", "--reverse", commitRange(before, after)))
		if len(commits) != 1 {
			t.Fatalf("append %d made %d commits on %s, and the clause is about one write", k+1, len(commits), SessionsRef)
		}
		introduced := introducedBytes(t, work, commits[0])
		if introduced > worst {
			worst, worstAt = introduced, k+1
		}
		// The line and its newline are what this append recorded, by
		// construction: one append of one line. Held to that rather than to
		// what the commit's own diff grew by, because a layout that wrote the
		// line to two places would then be allowed twice as much for it.
		if introduced > limit {
			t.Fatalf("append %d of %d introduced %d bytes of new objects to record %d; the bound is %d × %d + %d = %d",
				k+1, receiptPartAppends, introduced, receiptPartLine+1, receiptPartFactor, receiptPartLine+1, receiptPartAllowance, limit)
		}
	}
	t.Logf("the costliest of %d appends was the %dth, introducing %d bytes to record %d; the bound is %d",
		receiptPartAppends, worstAt, worst, receiptPartLine+1, limit)

	// Cheap writes are worth nothing if they lost a line, or reordered one. The
	// receipt is read by the path every reader names it with, so this is the
	// stream kept would recover, not the files the store chose.
	var want bytes.Buffer
	for _, line := range lines {
		want.Write(line)
		want.WriteByte('\n')
	}
	if held := readReceiptStream(t, store, thread); held != want.String() {
		t.Fatalf("the receipt reads back %d bytes after %d appends of %d, want %d; a line was lost, duplicated or reordered",
			len(held), receiptPartAppends, receiptPartLine+1, want.Len())
	}

	// And the writes were cheap for the reason the store says they are: no file
	// it wrote holds more than one part. A bound nothing enforces would pass
	// this fixture and fail a longer session, which is the failure this test
	// replaces.
	parts, err := store.receiptParts(refValue(t, work, SessionsRef), mustReceiptStream(t, name))
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) < 2 {
		t.Fatalf("%d appends of %d bytes left %d part(s), so nothing here measured a write made after the stream rolled",
			receiptPartAppends, receiptPartLine+1, len(parts))
	}
	for _, part := range parts {
		if part.size > ReceiptPartBytes {
			t.Fatalf("%s holds %d bytes, past the %d-byte part this store will hand git", part.where, part.size, ReceiptPartBytes)
		}
	}
	t.Logf("%d appends of %d bytes left %d parts of at most %d bytes", receiptPartAppends, receiptPartLine+1, len(parts), ReceiptPartBytes)
}

// TestAReceiptRollsIntoPartsAndReadsBackAsOneStream is the roll itself: that a
// line goes into the part with room for it, that a line longer than a part gets
// one of its own rather than being refused or split, and that the parts read
// back as the one stream they are.
//
// Separate from the cost above because this is the mechanism and that is the
// property: a reader joining parts in the wrong order, or a store refusing an
// oversized line, would be a receipt kept cannot recover however cheap its
// writes were.
func TestAReceiptRollsIntoPartsAndReadsBackAsOneStream(t *testing.T) {
	work := neighbourRepository(t)
	store := NewRefStore(work)
	thread := ids.Mint()
	name, err := ThreadPath(thread)
	if err != nil {
		t.Fatal(err)
	}

	var want bytes.Buffer
	appended := func(line []byte) {
		if err := store.Append(name, line); err != nil {
			t.Fatal(err)
		}
		want.Write(line)
		want.WriteByte('\n')
	}
	// Lines a quarter of a part long, so three share a part and the fourth
	// starts the next one: enough appends to roll twice without paying for the
	// seven hundred short lines a part would otherwise hold. Then one line
	// longer than a part can hold, then three more, so the oversized line is
	// read back in its place rather than at an end.
	line := func(k int) []byte {
		head := fmt.Sprintf(`{"v":1,"line":%d,"filler":"`, k)
		return append(append([]byte(head), bytes.Repeat([]byte("x"), ReceiptPartBytes/4-len(head)-2)...), '"', '}')
	}
	for k := range 9 {
		appended(line(k + 1))
	}
	oversized := append([]byte(`{"v":1,"kind":"oversized","filler":"`), append(bytes.Repeat([]byte("y"), ReceiptPartBytes+1), '"', '}')...)
	appended(oversized)
	for k := range 3 {
		appended(line(k + 10))
	}

	if held := readReceiptStream(t, store, thread); held != want.String() {
		t.Fatalf("the receipt reads back %d bytes, want %d: the parts did not join in the order they were written", len(held), want.Len())
	}

	revision := refValue(t, work, SessionsRef)
	parts, err := store.receiptParts(revision, mustReceiptStream(t, name))
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) < 5 {
		t.Fatalf("the appends left %d parts, too few to have rolled twice, held an oversized line alone, and rolled again after it", len(parts))
	}
	// Part 0 is the one name locate gives a receipt. Everything that asks the
	// store where a receipt is filed asks locate — including the tests that hold
	// a session record to being on the sessions ref and not the ledger — so a
	// first part written anywhere but there is locate reporting a path no line
	// is at.
	if _, where := locate(name); parts[0].where != where {
		t.Fatalf("the first part is at %s and locate files this receipt at %s, so the store's own answer names a path no line is in", parts[0].where, where)
	}
	// Every part is within the bound except the one holding the oversized line,
	// which holds that line and nothing else: it costs its own bytes and no
	// other line's, which is what the clause allows a line to cost.
	alone := int64(len(oversized) + 1)
	past := 0
	for k, part := range parts {
		if part.number != k {
			t.Fatalf("part %d of the receipt is numbered %d, so a reader joining them by number has a gap", k, part.number)
		}
		if part.size <= ReceiptPartBytes {
			continue
		}
		past++
		if part.size != alone {
			t.Fatalf("%s holds %d bytes, past the %d-byte part, and the one oversized line appended is %d bytes: the part holds that line and something else",
				part.where, part.size, ReceiptPartBytes, alone)
		}
	}
	if past != 1 {
		t.Fatalf("%d parts are past the bound, and exactly one oversized line was appended", past)
	}
}

// refValue is what a ref holds now, or the empty string if it holds nothing.
func refValue(t *testing.T, work, ref string) string {
	t.Helper()
	out, code := neighbourGitCode(t, work, nil, nil, "rev-parse", "--verify", "--quiet", ref)
	if code != 0 {
		return ""
	}
	return strings.TrimSpace(out)
}

// commitRange names the commits a write added, whether or not the ref held
// anything before it.
func commitRange(before, after string) string {
	if before == "" {
		return after
	}
	return before + ".." + after
}

// introducedBytes is the uncompressed size of every object commit made
// reachable that its parents did not — what kept handed git for this write.
//
// Uncompressed, and every object rather than only the trees
// [introducedTreeBytes] counts, because this clause is about the blob as well:
// the blob is the receipt's own bytes and the one a layout rewriting the whole
// receipt gets wrong.
func introducedBytes(t *testing.T, work, commit string) int {
	t.Helper()
	args := []string{"rev-list", "--objects", "--no-object-names", commit}
	for parent := range strings.FieldsSeq(neighbourGit(t, work, "log", "-1", "--format=%P", commit)) {
		args = append(args, "^"+parent)
	}
	sized := neighbourGitEnv(t, work, nil, []byte(neighbourGit(t, work, args...)), "cat-file", "--batch-check=%(objectsize)")
	total := 0
	for field := range strings.FieldsSeq(sized) {
		n, err := strconv.Atoi(field)
		if err != nil {
			t.Fatalf("cat-file sized an object as %q", field)
		}
		total += n
	}
	return total
}

func mustReceiptStream(t *testing.T, name string) receiptStream {
	t.Helper()
	stream, ok := receiptStreamOf(name)
	if !ok {
		t.Fatalf("%q is not a receipt stream the store appends to in parts", name)
	}
	return stream
}
