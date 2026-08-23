//go:build !windows

package confine

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResolveGoAsksTheToolchainWhereItIs(t *testing.T) {
	toolchain := resolveRealGo(t)

	// Every answer is this machine's, and the point of asking rather than
	// declaring is that none of it is written down anywhere in kept.
	for _, path := range append(append([]string(nil), toolchain.Read...), toolchain.Write...) {
		if !filepath.IsAbs(path) {
			t.Fatalf("resolved toolchain path %q is not absolute", path)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("resolved toolchain path %s is not there: %v", path, err)
		}
	}
	if len(toolchain.Path) != 1 {
		t.Fatalf("resolved search path = %v, want the one directory the executable is in", toolchain.Path)
	}
	// The two structural variables are the ones a path list could never carry,
	// and -buildvcs=false is not a preference: a box is a git worktree whose
	// .git points outside the box, so a default build fails to stamp it.
	for name, want := range map[string]string{"GOTOOLCHAIN": "local", "GOFLAGS": "-buildvcs=false"} {
		if toolchain.Env[name] != want {
			t.Fatalf("resolved %s = %q, want %q", name, toolchain.Env[name], want)
		}
	}
	for _, name := range goResolution {
		if toolchain.Env[name] == "" {
			t.Fatalf("resolved environment is missing %s: %#v", name, toolchain.Env)
		}
	}
	if toolchain.Env["GOROOT"] != toolchain.Read[0] {
		t.Fatalf("resolved GOROOT = %q but the read grant is %q; the child would be told about a path nobody granted", toolchain.Env["GOROOT"], toolchain.Read[0])
	}
}

// TestResolveGoRefusesWhatItCannotResolve breaks each thing the resolver is
// supposed to notice. A silent wrong guess is the worst outcome here: it
// renders a boundary around a path the toolchain does not use, and the failure
// arrives from inside a box looking like a repair that did nothing.
func TestResolveGoRefusesWhatItCannotResolve(t *testing.T) {
	real := realGo(t)
	for _, test := range []struct {
		name       string
		executable func(*testing.T) string
		deadline   time.Duration
		want       string
	}{
		{
			name:       "a name rather than a path",
			executable: func(*testing.T) string { return "go" },
			want:       "not an explicit absolute path",
		},
		{
			name:       "no deadline",
			executable: func(*testing.T) string { return real },
			deadline:   -1,
			want:       "deadline is unset",
		},
		{
			name: "the toolchain refuses to answer",
			executable: func(t *testing.T) string {
				return fakeGo(t, "echo 'go: cannot determine GOROOT' >&2\nexit 2\n")
			},
			want: "resolve the go toolchain",
		},
		{
			name: "the toolchain answers with something that is not a path",
			executable: func(t *testing.T) string {
				return fakeGo(t, "printf 'auto\\nauto\\nauto\\nauto\\n'\n")
			},
			want: "which is not an absolute path",
		},
		{
			name: "the toolchain answers fewer variables than it was asked",
			executable: func(t *testing.T) string {
				return fakeGo(t, "printf '/one\\n/two\\n'\n")
			},
			want: "answered 2 of 4 variables",
		},
		{
			name: "a granted path is not on this machine",
			executable: func(t *testing.T) string {
				return fakeGo(t, "printf '/no/such/goroot\\n/no/such/gopath\\n/no/such/modcache\\n/no/such/cache\\n'\n")
			},
			want: "which is not there",
		},
		{
			name: "a granted path is a file rather than a directory",
			executable: func(t *testing.T) string {
				file := filepath.Join(t.TempDir(), "goroot")
				if err := os.WriteFile(file, nil, 0o600); err != nil {
					t.Fatal(err)
				}
				return fakeGo(t, "printf '"+file+"\\n/p\\n/m\\n/c\\n'\n")
			},
			want: "which is not a directory",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			deadline := test.deadline
			if deadline == 0 {
				deadline = 30 * time.Second
			}
			_, err := ResolveGo(context.Background(), test.executable(t), deadline)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ResolveGo error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestResolveGoStopsAtItsDeadline(t *testing.T) {
	slow := fakeGo(t, "sleep 30\n")
	start := time.Now()
	_, err := ResolveGo(context.Background(), slow, 200*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("ResolveGo error = %v, want a timeout", err)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("ResolveGo waited %s past its deadline", elapsed)
	}
}

func realGo(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go toolchain to interrogate")
	}
	return absolute(t, path)
}

func resolveRealGo(t *testing.T) Toolchain {
	t.Helper()
	toolchain, err := ResolveGo(context.Background(), realGo(t), 30*time.Second)
	if err != nil {
		t.Fatalf("resolve the real go toolchain: %v", err)
	}
	return toolchain
}

// fakeGo writes a stand-in that answers `go env` however a test needs. It is
// what lets each resolver refusal be measured rather than argued about.
func fakeGo(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "go")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}
