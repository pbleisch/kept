package workspace

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// gitIndex is git's index read as a pre-warmed cache: for every path whose stat
// still matches what git recorded, the oid of the blob git already hashed.
//
// It is an accelerator and never the definition. It has to agree with the plain
// walk, and the agreement is tested, because a definition that changed with
// whether git happened to be installed would split the observation cache, the
// truth series and prior() with nothing reporting it. See docs/decision/0033.
//
// What it buys is the case the stat cache cannot have: a checkout rewrites
// every mtime and no blob oid, so kept's own stat cache misses everything while
// git's index vouches for all of it.
//
// It is read as a file format, like .gitignore, and no git is invoked. Anything
// unexpected — a version this does not read, a checksum that does not verify,
// an entry that does not parse — yields no index at all rather than a guess:
// the plain path is always there, and it is the answer this one has to match.
type gitIndex struct {
	entries map[string]gitEntry
	// taken is the index file's own mtime, which is the snapshot the racy rule
	// is measured against — git's own rule, applied to git's own file.
	taken time.Time
}

type gitEntry struct {
	mtimeSec, mtimeNS uint32
	inode, size, mode uint32
	oid               string
}

// readGitIndex returns the index under a workspace root, or nil.
func readGitIndex(root string) *gitIndex {
	dir := gitDir(root)
	if dir == "" {
		return nil
	}
	path := filepath.Join(dir, "index")
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	entries, ok := parseGitIndex(src)
	if !ok {
		return nil
	}
	return &gitIndex{entries: entries, taken: info.ModTime()}
}

// gitDir resolves .git, which is a directory in an ordinary checkout and a file
// naming one in a worktree — which is what a box is.
func gitDir(root string) string {
	path := filepath.Join(root, ".git")
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	if info.IsDir() {
		return path
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(src))
	rest, found := strings.CutPrefix(line, "gitdir:")
	if !found {
		return ""
	}
	dir := strings.TrimSpace(rest)
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(root, dir)
	}
	return dir
}

// blob returns the oid git holds for a path whose stat still matches what git
// recorded, and whether it may be believed.
//
// The comparison is git's own, truncations included: the index stores size and
// inode in thirty-two bits, so that is what is compared. Anything that does not
// match — including a mode that differs only in its executable bit — is read
// from disk instead.
func (g *gitIndex) blob(path string, key statKey) (string, bool) {
	e, ok := g.entries[path]
	if !ok {
		return "", false
	}
	if e.size != uint32(key.Size) || e.inode != uint32(key.Inode) {
		return "", false
	}
	sec, ns := uint32(key.MtimeNS/int64(time.Second)), uint32(key.MtimeNS%int64(time.Second))
	if e.mtimeSec != sec || e.mtimeNS != ns {
		return "", false
	}
	if e.mode&0o111 != key.Mode&0o111 {
		return "", false
	}
	if racy(key.MtimeNS, g.taken) {
		return "", false
	}
	return e.oid, true
}

const (
	gitHeader   = "DIRC"
	gitOIDLen   = 20 // sha1 object format; a sha256 index fails the checksum and is declined
	gitEntryLen = 62
)

// parseGitIndex reads versions 2, 3 and 4.
//
// The trailing checksum is verified before anything is returned, and that is
// the guard rather than a nicety: it is what proves this read the file the way
// git wrote it. Without it a misparse would produce oids that name the wrong
// content, and an oid is a key into a digest this package will then report as
// the world.
func parseGitIndex(src []byte) (map[string]gitEntry, bool) {
	if len(src) < 12+gitOIDLen || string(src[:4]) != gitHeader {
		return nil, false
	}
	sum := sha1.Sum(src[:len(src)-gitOIDLen])
	if !bytes.Equal(sum[:], src[len(src)-gitOIDLen:]) {
		return nil, false
	}

	version := binary.BigEndian.Uint32(src[4:8])
	if version < 2 || version > 4 {
		return nil, false
	}
	count := binary.BigEndian.Uint32(src[8:12])

	entries := make(map[string]gitEntry, count)
	at := 12
	var previous string
	for range count {
		if at+gitEntryLen > len(src)-gitOIDLen {
			return nil, false
		}
		start := at
		raw := src[at:]
		e := gitEntry{
			mtimeSec: binary.BigEndian.Uint32(raw[8:12]),
			mtimeNS:  binary.BigEndian.Uint32(raw[12:16]),
			inode:    binary.BigEndian.Uint32(raw[20:24]),
			mode:     binary.BigEndian.Uint32(raw[24:28]),
			size:     binary.BigEndian.Uint32(raw[36:40]),
			oid:      hex.EncodeToString(raw[40 : 40+gitOIDLen]),
		}
		flags := binary.BigEndian.Uint16(raw[60:62])
		nameLen := int(flags & 0x0fff)
		stage := (flags >> 12) & 3
		extended := flags&0x4000 != 0

		at += gitEntryLen
		if extended {
			if version < 3 || at+2 > len(src) {
				return nil, false
			}
			at += 2
		}

		var name string
		var ok bool
		if version >= 4 {
			name, at, ok = readCompressedName(src, at, previous)
		} else {
			name, at, ok = readName(src, start, at, nameLen)
		}
		if !ok {
			return nil, false
		}
		previous = name

		// Stage is non-zero only in a conflicted index, where the working tree
		// file is nobody's recorded content. Anything that is not a regular
		// file — a symlink, a gitlink — is left to the plain path, because the
		// oid of a symlink names its target's bytes and would collide with a
		// file holding the same text.
		if stage == 0 && (e.mode&0o170000) == 0o100000 {
			entries[name] = e
		}
	}
	return entries, true
}

// readName reads a version 2 or 3 name and the padding after it. An entry is
// padded with NULs to a multiple of eight measured from its own first byte, so
// the next one begins aligned — which is why the entry's start is needed and
// the name's is not enough.
func readName(src []byte, start, at, nameLen int) (string, int, bool) {
	if nameLen == 0x0fff {
		// The length did not fit in the flags, so the name runs to a NUL.
		end := bytes.IndexByte(src[at:], 0)
		if end < 0 {
			return "", 0, false
		}
		nameLen = end
	}
	if at+nameLen >= len(src) {
		return "", 0, false
	}
	name := string(src[at : at+nameLen])

	next := start + ((at + nameLen - start + 8) &^ 7)
	if next > len(src) {
		return "", 0, false
	}
	return name, next, true
}

// readCompressedName reads a version 4 name: how many bytes to strip from the
// previous name, then the suffix that replaces them. No padding.
func readCompressedName(src []byte, at int, previous string) (string, int, bool) {
	strip, at, ok := readVarint(src, at)
	if !ok || strip > len(previous) {
		return "", 0, false
	}
	end := bytes.IndexByte(src[at:], 0)
	if end < 0 {
		return "", 0, false
	}
	name := previous[:len(previous)-strip] + string(src[at:at+end])
	return name, at + end + 1, true
}

// readVarint is git's offset encoding, which is not the usual one: each
// continuation adds one before shifting, so that a given value has exactly one
// representation.
func readVarint(src []byte, at int) (int, int, bool) {
	if at >= len(src) {
		return 0, 0, false
	}
	b := src[at]
	at++
	value := int(b & 0x7f)
	for b&0x80 != 0 {
		if at >= len(src) {
			return 0, 0, false
		}
		b = src[at]
		at++
		value = ((value + 1) << 7) | int(b&0x7f)
	}
	return value, at, true
}
