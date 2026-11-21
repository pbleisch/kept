package ledger

import (
	"bytes"
	"encoding/hex"
	"math/rand/v2"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// packLab is a repository a store has written a receipt-shaped stream into,
// rewritten whole on every line the way a thread's receipt is, and what the
// object store holds afterwards.
type packLab struct {
	repo   string
	store  *RefStore
	lines  [][]byte
	loose  int64 // bytes of loose objects on disk
	packed int   // packs
}

// packingLines is how many lines the lab writes and packingLine how long each
// is. Together they hand git about 8 MiB, every byte of it a copy of an earlier
// version of the same stream — which is what a receipt does to an object store.
const (
	packingLines = 90
	packingLine  = 2000
)

func writeReceiptShaped(t *testing.T, packAt int64, configure ...string) packLab {
	t.Helper()
	repo := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	for i := 0; i+1 < len(configure); i += 2 {
		if out, err := exec.Command("git", "-C", repo, "config", configure[i], configure[i+1]).CombinedOutput(); err != nil {
			t.Fatalf("git config: %v\n%s", err, out)
		}
	}
	store := NewRefStore(repo)
	store.packAt = packAt
	// Hex of random bytes, so what git compresses is about half of what it was
	// handed and the loose bytes on disk say something about the count.
	random := rand.New(rand.NewPCG(1, 2))
	lab := packLab{repo: repo, store: store}
	for range packingLines {
		raw := make([]byte, packingLine/2)
		for i := range raw {
			raw[i] = byte(random.Uint32())
		}
		line := []byte(hex.EncodeToString(raw))
		if err := store.Append("threads/packing.jsonl", line); err != nil {
			t.Fatal(err)
		}
		lab.lines = append(lab.lines, line)
	}
	lab.loose, lab.packed = objectStore(t, repo)
	return lab
}

// objectStore is what git count-objects says the repository holds loose, in
// bytes, and how many packs it has.
func objectStore(t *testing.T, repo string) (loose int64, packs int) {
	t.Helper()
	out, err := exec.Command("git", "-C", repo, "count-objects", "-v").CombinedOutput()
	if err != nil {
		t.Fatalf("git count-objects: %v\n%s", err, out)
	}
	for _, field := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		name, value, _ := strings.Cut(field, ": ")
		n, _ := strconv.ParseInt(value, 10, 64)
		switch name {
		case "size":
			loose = n << 10
		case "packs":
			packs = int(n)
		}
	}
	return loose, packs
}

// The bound a lab is held to: what a store hands git between two packings is at
// most its threshold plus the one write that crossed it, and git stores that
// compressed. Twice the threshold is room for the trees and commits each write
// adds, and it is still a small fraction of what the lab writes in all.
func packedBound(packAt int64) int64 { return 2 * packAt }

// A store that has handed git more than its threshold packs what it wrote, so
// the loose bytes on disk stay near the threshold however much is written, and
// the stream reads back whole. The same writes with packing turned off leave
// several times the bound loose, which is what shows the bound can fire.
func TestWritesPastThePackThresholdArePackedAndReadBackWhole(t *testing.T) {
	const packAt = 512 << 10

	packed := writeReceiptShaped(t, packAt)
	if packed.loose > packedBound(packAt) {
		t.Fatalf("after packing every %d bytes, %d bytes are loose; the bound is %d", packAt, packed.loose, packedBound(packAt))
	}
	if packed.packed == 0 {
		t.Fatalf("nothing was packed after the store handed git %d lines of %d bytes each", packingLines, packingLine)
	}
	held, err := packed.store.Read("threads/packing.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	want := append(bytes.Join(packed.lines, []byte("\n")), '\n')
	if !bytes.Equal(held, want) {
		t.Fatalf("the stream read back after packing is %d bytes, and %d were written", len(held), len(want))
	}

	unpacked := writeReceiptShaped(t, 0)
	t.Logf("loose bytes after the same writes: %d packed every %d bytes, %d never packed", packed.loose, packAt, unpacked.loose)
	if unpacked.loose <= packedBound(packAt) {
		t.Fatalf("with packing off, %d bytes are loose, which is inside the bound %d, so the bound above cannot tell packing from its absence", unpacked.loose, packedBound(packAt))
	}
	if unpacked.packed != 0 {
		t.Fatalf("with packing off, the store left %d packs", unpacked.packed)
	}
}

// An operator who set gc.auto to 0 has said the repository is never packed
// automatically, and a store honours that the way git's own porcelain does.
func TestAStoreDoesNotPackARepositoryWhoseOperatorTurnedAutomaticPackingOff(t *testing.T) {
	const packAt = 512 << 10
	lab := writeReceiptShaped(t, packAt, "gc.auto", "0")
	if lab.packed != 0 {
		t.Fatalf("gc.auto is 0 and the store packed anyway: %d packs", lab.packed)
	}
	if lab.loose <= packedBound(packAt) {
		t.Fatalf("gc.auto is 0 and only %d bytes are loose, so this test did not write enough to tell", lab.loose)
	}
}
