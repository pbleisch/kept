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

func TestResolveNodeAsksTheInterpreterAndFollowsTheHostedExecutable(t *testing.T) {
	node, hosted := realNode(t), realPi(t)
	toolchain, err := ResolveNode(context.Background(), node, hosted, 30*time.Second)
	if err != nil {
		t.Fatalf("resolve the real node runtime: %v", err)
	}
	if len(toolchain.Path) != 1 {
		t.Fatalf("resolved search path = %v, want the one directory the interpreter is in", toolchain.Path)
	}
	// The shebang is what makes this the search path and not a grant: the
	// hosted executable is started by absolute path, and the kernel then looks
	// the interpreter up by name.
	if _, err := os.Stat(filepath.Join(toolchain.Path[0], "node")); err != nil {
		t.Fatalf("the declared search path %s holds no node for the shebang to find: %v", toolchain.Path[0], err)
	}
	if len(toolchain.Read) != 2 {
		t.Fatalf("resolved read grants = %v, want the interpreter's root and the runtime's package", toolchain.Read)
	}
	for _, path := range toolchain.Read {
		if !filepath.IsAbs(path) {
			t.Fatalf("resolved node path %q is not absolute", path)
		}
		if info, err := os.Stat(path); err != nil || !info.IsDir() {
			t.Fatalf("resolved node path %s is not a directory on this machine: %v", path, err)
		}
	}
	// The search path is the directory of the executable the operator named,
	// and the grant is over the root the interpreter answered with. They are
	// different answers to different questions: a Homebrew node is named
	// through /opt/homebrew/bin and lives under its Cellar.
	if toolchain.Path[0] != filepath.Dir(node) {
		t.Fatalf("declared search path = %s, want the directory of the named interpreter %s", toolchain.Path[0], node)
	}
	real, err := filepath.EvalSymlinks(node)
	if err != nil {
		t.Fatal(err)
	}
	if !within(toolchain.Read[0], real) {
		t.Fatalf("the interpreter at %s is not under the root %s that was granted", real, toolchain.Read[0])
	}
	// The package root, not the script's own directory: `pi` is a symlink into
	// dist/, and the tree Node reads is the package above it.
	resolved, err := filepath.EvalSymlinks(hosted)
	if err != nil {
		t.Fatal(err)
	}
	if !within(toolchain.Read[1], resolved) {
		t.Fatalf("the granted package %s does not contain the file node reads at %s", toolchain.Read[1], resolved)
	}
	if _, err := os.Stat(filepath.Join(toolchain.Read[1], nodePackageMarker)); err != nil {
		t.Fatalf("the granted package root %s holds no %s: %v", toolchain.Read[1], nodePackageMarker, err)
	}
	// Resolution grants nothing a toolchain would set. A Node runtime needs no
	// variables, and inventing one here would be a value nobody resolved.
	if len(toolchain.Env) != 0 {
		t.Fatalf("resolved node environment = %#v, want none", toolchain.Env)
	}
}

// TestResolveNodeRefusesWhatItCannotResolve breaks each thing the resolver has
// to notice. Every one of these, unresolved, arrives from inside a box as
// `env: node: No such file or directory` or MODULE_NOT_FOUND — which is what a
// repair that ran and did nothing also looks like.
func TestResolveNodeRefusesWhatItCannotResolve(t *testing.T) {
	node, hosted := realNode(t), realPi(t)
	for _, test := range []struct {
		name     string
		node     func(*testing.T) string
		hosted   func(*testing.T) string
		deadline time.Duration
		want     string
	}{
		{
			name: "an interpreter named rather than located",
			node: func(*testing.T) string { return "node" },
			want: "interpreter is not an explicit absolute path",
		},
		{
			name:   "a hosted executable named rather than located",
			hosted: func(*testing.T) string { return "pi" },
			want:   "hosted executable is not an explicit absolute path",
		},
		{
			name:     "no deadline",
			deadline: -1,
			want:     "deadline is unset",
		},
		{
			name: "the interpreter refuses to answer",
			node: func(t *testing.T) string {
				return fakeNode(t, "echo 'node: bad option' >&2\nexit 9\n")
			},
			want: "resolve the node interpreter",
		},
		{
			name: "the interpreter answers with something that is not a path",
			node: func(t *testing.T) string { return fakeNode(t, "printf 'node\\n'\n") },
			want: "which is not an absolute path",
		},
		{
			name: "the interpreter answers with a path that is not on this machine",
			node: func(t *testing.T) string { return fakeNode(t, "printf '/no/such/prefix/bin/node\\n'\n") },
			want: "is not there",
		},
		{
			name: "the interpreter has no installation root above its directory",
			node: func(t *testing.T) string { return fakeNode(t, "printf '/node\\n'\n") },
			want: "no installation root above its directory",
		},
		{
			name: "a hosted executable that is not on this machine",
			hosted: func(t *testing.T) string {
				return filepath.Join(ungrantedDir(t), "absent")
			},
			want: "resolve the node-hosted executable",
		},
		{
			name: "a hosted executable with no package above it",
			hosted: func(t *testing.T) string {
				// Under the operator's own root, so the walk up reaches the
				// filesystem root without meeting somebody else's package.json.
				loose := filepath.Join(ungrantedDir(t), "loose.js")
				if err := os.WriteFile(loose, []byte("#!/usr/bin/env node\n"), 0o700); err != nil {
					t.Fatal(err)
				}
				return loose
			},
			want: "has no package.json above it",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			interpreter, executable, deadline := node, hosted, test.deadline
			if test.node != nil {
				interpreter = test.node(t)
			}
			if test.hosted != nil {
				executable = test.hosted(t)
			}
			if deadline == 0 {
				deadline = 30 * time.Second
			}
			_, err := ResolveNode(context.Background(), interpreter, executable, deadline)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ResolveNode error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestResolveNodeStopsAtItsDeadline(t *testing.T) {
	slow := fakeNode(t, "sleep 30\n")
	start := time.Now()
	_, err := ResolveNode(context.Background(), slow, realPi(t), 200*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("ResolveNode error = %v, want a timeout", err)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("ResolveNode waited %s past its deadline", elapsed)
	}
}

func realNode(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("node")
	if err != nil {
		t.Skip("no node interpreter to interrogate")
	}
	return absolute(t, path)
}

func realPi(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("pi")
	if err != nil {
		t.Skip("Pi is not installed")
	}
	return absolute(t, path)
}

// fakeNode writes a stand-in that answers `node -p` however a test needs.
func fakeNode(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "node")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}
