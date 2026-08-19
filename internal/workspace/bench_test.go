package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The benchmarks belong with the digest, because decision 0033 is a
// measurement: the accelerations were chosen against numbers on a 6.6 GB
// repository — 27.3 s for a naive full hash, 2.53 s honouring ignore rules,
// 0.64 s parallelised, 0.16 s in steady state — and the claim that steady state
// is milliseconds is the bar for something invoked the way make is.
//
// Run them with -bench . -benchtime 1x. They measure this machine, not that
// one; what they are for is noticing if a path stops being an acceleration.

func benchTree(b *testing.B, dirs, per, size int) string {
	b.Helper()
	root := b.TempDir()
	body := strings.Repeat("x", size)
	old := time.Now().Add(-10 * time.Second)
	for d := range dirs {
		dir := filepath.Join(root, fmt.Sprintf("pkg%03d", d))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			b.Fatal(err)
		}
		for f := range per {
			path := filepath.Join(dir, fmt.Sprintf("file%03d.go", f))
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				b.Fatal(err)
			}
			if err := os.Chtimes(path, old, old); err != nil {
				b.Fatal(err)
			}
		}
	}
	return root
}

func BenchmarkCold(b *testing.B) {
	root := benchTree(b, 40, 25, 4096)
	for b.Loop() {
		if _, err := Read(root, Options{NoGit: true}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkColdParallel(b *testing.B) {
	root := benchTree(b, 40, 25, 4096)
	for b.Loop() {
		if _, err := Read(root, Options{NoGit: true, Workers: 8}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSteadyState(b *testing.B) {
	root := benchTree(b, 40, 25, 4096)
	kept := b.TempDir()
	cache := OpenCache(kept)
	if _, err := Read(root, Options{NoGit: true, Cache: cache}); err != nil {
		b.Fatal(err)
	}
	if err := cache.Save(); err != nil {
		b.Fatal(err)
	}

	for b.Loop() {
		warm := OpenCache(kept)
		got, err := Read(root, Options{NoGit: true, Cache: warm, Workers: 8})
		if err != nil {
			b.Fatal(err)
		}
		if got.Read != 0 {
			b.Fatalf("read %d files in steady state", got.Read)
		}
	}
}

// This repository, full content hash. Decision 0033 measured 0.03 s.
func BenchmarkThisRepository(b *testing.B) {
	root, err := filepath.Abs("../..")
	if err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		if _, err := Read(root, Options{NoGit: true, Workers: 8}); err != nil {
			b.Fatal(err)
		}
	}
}
