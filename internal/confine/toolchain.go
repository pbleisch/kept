//go:build !windows

package confine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Toolchain is one located program — a toolchain a promise names, or the
// interpreter and package a repair runtime is hosted by — as this machine
// actually has it.
//
// Every path in it was answered by the tool itself, so it is a **fact about
// this machine** and never a claim about what a box may do. What a profile
// makes of it is a grant, and whether the provider agrees is what the
// pre-flight asks. Nothing here names a provider policy group: with the
// resolved paths in hand a group grants directories nobody resolved and the
// pre-flight never checks, which is the one category the pre-flight exists to
// remove. See docs/decision/0053.
//
// Path is where the tool's own executable lives. It is **declared, not
// granted**: the child's search path decides which name resolves and settles
// nothing about what is permitted, so kept renders no grant for it and the
// pre-flight refuses a dispatch whose search path the provider does not
// already reach.
type Toolchain struct {
	Path  []string
	Read  []string
	Write []string
	Env   map[string]string
}

// goResolution is what `go env` is asked, in the order it answers.
var goResolution = []string{"GOROOT", "GOPATH", "GOMODCACHE", "GOCACHE"}

// ResolveGo asks a Go toolchain where it lives.
//
// The executable is an explicit absolute path for the same reason the provider
// and the canary binaries are: a name resolved on the operator's search path is
// not a policy anybody wrote down. Interrogation rather than declaration is
// what makes this work on a layout kept did not anticipate — a Homebrew Cellar
// path, an official tarball under /usr/local/go, a manager's shim — because the
// tool is the authority on its own location and a static path list is wrong the
// moment a machine differs.
//
// Two variables are structural rather than located. GOTOOLCHAIN=local keeps a
// module's `toolchain` line from sending the box to fetch a compiler it cannot
// reach; GOFLAGS=-buildvcs=false is required because a box is a git worktree
// whose .git is a file pointing outside the box, so a default build stamps VCS
// information it cannot read and fails.
func ResolveGo(ctx context.Context, executable string, deadline time.Duration) (Toolchain, error) {
	if ctx == nil {
		return Toolchain{}, errors.New("go toolchain resolution context is nil")
	}
	if executable == "" || !filepath.IsAbs(executable) {
		return Toolchain{}, fmt.Errorf("go toolchain executable is not an explicit absolute path: %s", executable)
	}
	if deadline <= 0 {
		// Nothing external runs without one. See docs/decision/0026.
		return Toolchain{}, errors.New("go toolchain resolution deadline is unset")
	}
	ctx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()

	command := exec.CommandContext(ctx, executable, append([]string{"env"}, goResolution...)...)
	// Killing the process is not the same as being done with it: a resolver
	// that forked something holding the output pipe would keep Wait blocked
	// past the deadline it was given. See docs/decision/0026.
	command.WaitDelay = externalWaitDelay
	output, err := command.Output()
	if ctx.Err() != nil {
		return Toolchain{}, fmt.Errorf("go toolchain resolution timed out after %s: %s env", deadline, executable)
	}
	if err != nil {
		return Toolchain{}, fmt.Errorf("resolve the go toolchain with %s env: %w: %s", executable, err, providerDiagnostic(err))
	}
	answered := strings.Split(strings.TrimRight(string(output), "\n"), "\n")
	if len(answered) != len(goResolution) {
		return Toolchain{}, fmt.Errorf("go toolchain resolution answered %d of %d variables: %q", len(answered), len(goResolution), string(output))
	}

	resolved := make(map[string]string, len(goResolution))
	for i, name := range goResolution {
		value := strings.TrimSpace(answered[i])
		if value == "" || !filepath.IsAbs(value) {
			return Toolchain{}, fmt.Errorf("go toolchain resolution answered %s with %q, which is not an absolute path", name, value)
		}
		resolved[name] = canonicalAncestor(value)
	}

	// Only the three kept grants have to be there. A grant on a path that does
	// not exist reads as a boundary and is not one, and the failure it produces
	// arrives from inside a box as a build that could not find its own cache.
	// GOPATH is deliberately not in this list: it is set so Go can locate the
	// module cache under it, and kept grants the cache rather than the tree.
	granted := []string{"GOROOT", "GOMODCACHE", "GOCACHE"}
	for _, name := range granted {
		info, err := os.Stat(resolved[name])
		if err != nil {
			return Toolchain{}, fmt.Errorf("go toolchain %s is at %s, which is not there: %w", name, resolved[name], err)
		}
		if !info.IsDir() {
			return Toolchain{}, fmt.Errorf("go toolchain %s is at %s, which is not a directory", name, resolved[name])
		}
	}

	return Toolchain{
		Path:  []string{filepath.Dir(executable)},
		Read:  []string{resolved["GOROOT"], resolved["GOMODCACHE"]},
		Write: []string{resolved["GOCACHE"]},
		Env: map[string]string{
			"GOROOT":      resolved["GOROOT"],
			"GOPATH":      resolved["GOPATH"],
			"GOMODCACHE":  resolved["GOMODCACHE"],
			"GOCACHE":     resolved["GOCACHE"],
			"GOTOOLCHAIN": "local",
			"GOFLAGS":     "-buildvcs=false",
		},
	}, nil
}

// externalWaitDelay bounds how long a killed external process may keep kept
// waiting on its output after its deadline has already passed.
const externalWaitDelay = 2 * time.Second

// providerDiagnostic returns what an external process wrote to stderr, so a
// refusal names what the tool said rather than only that it failed.
func providerDiagnostic(err error) string {
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return strings.TrimSpace(string(exit.Stderr))
	}
	return ""
}

// nodeResolution is what a Node interpreter is asked. process.execPath is the
// real path of the running interpreter, symlinks already resolved by Node
// itself — the same shape as `go env GOROOT`, and true on a layout kept did
// not anticipate because a real interpreter answered it.
const nodeResolution = "process.execPath"

// nodePackageMarker is Node's own definition of a package boundary. Walking up
// to it is what keeps the grant from being a depth kept guessed at.
const nodePackageMarker = "package.json"

// ResolveNode asks a Node interpreter where it lives and follows a Node-hosted
// executable to the file Node will actually read.
//
// A hosted runtime has two locations and neither is the other. The interpreter
// is reached through the executable's `#!/usr/bin/env node` line, so its own
// directory has to be on the child's declared search path or the box reports
// `env: node: No such file or directory` and nothing asked the provider
// anything. The script is somewhere else entirely — `pi` is a symlink into a
// package tree — and a box that cannot read that tree reports MODULE_NOT_FOUND,
// which reads exactly like a repair that ran and did nothing.
//
// Both become declared paths, so both go through the pre-flight. What this does
// not cover is stated rather than assumed: a Homebrew interpreter's dynamic
// libraries lie outside its installation root and the box reaches them through
// the provider's baseline grant on /opt, the same way a Homebrew Go toolchain
// does under docs/decision/0053. See docs/decision/0055.
func ResolveNode(ctx context.Context, node, hosted string, deadline time.Duration) (Toolchain, error) {
	if ctx == nil {
		return Toolchain{}, errors.New("node runtime resolution context is nil")
	}
	for _, path := range []struct{ name, value string }{
		{"interpreter", node},
		{"hosted executable", hosted},
	} {
		if path.value == "" || !filepath.IsAbs(path.value) {
			return Toolchain{}, fmt.Errorf("node runtime %s is not an explicit absolute path: %s", path.name, path.value)
		}
	}
	if deadline <= 0 {
		// Nothing external runs without one. See docs/decision/0026.
		return Toolchain{}, errors.New("node runtime resolution deadline is unset")
	}
	ctx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()

	command := exec.CommandContext(ctx, node, "-p", nodeResolution)
	command.WaitDelay = externalWaitDelay
	output, err := command.Output()
	if ctx.Err() != nil {
		return Toolchain{}, fmt.Errorf("node runtime resolution timed out after %s: %s -p %s", deadline, node, nodeResolution)
	}
	if err != nil {
		return Toolchain{}, fmt.Errorf("resolve the node interpreter with %s -p %s: %w: %s", node, nodeResolution, err, providerDiagnostic(err))
	}
	executable := strings.TrimSpace(string(output))
	if executable == "" || !filepath.IsAbs(executable) {
		return Toolchain{}, fmt.Errorf("node runtime resolution answered %s with %q, which is not an absolute path", nodeResolution, executable)
	}
	executable = canonicalAncestor(executable)

	// The parent of the directory the real interpreter is in. Every Node
	// distribution puts it at <root>/bin/node, and the root is what carries the
	// interpreter's own library tree.
	root := filepath.Dir(filepath.Dir(executable))
	if root == string(filepath.Separator) {
		return Toolchain{}, fmt.Errorf("node runtime interpreter at %s has no installation root above its directory", executable)
	}
	// The search path is the directory of the executable the operator named,
	// not of the one Node answered with. It is the same rule ResolveGo uses and
	// it matters for the same reason: a shim resolves correctly because it execs
	// a real interpreter, while the directory the shebang has to find the name
	// in is the shim's own — and that directory is declared, granted by nothing,
	// and refused by the pre-flight when the provider cannot reach it.
	search := filepath.Dir(node)
	packaged, err := nodePackageRoot(hosted)
	if err != nil {
		return Toolchain{}, err
	}

	read := make([]string, 0, 2)
	for _, path := range []string{root, packaged} {
		info, err := os.Stat(path)
		if err != nil {
			return Toolchain{}, fmt.Errorf("node runtime path %s is not there: %w", path, err)
		}
		if !info.IsDir() {
			return Toolchain{}, fmt.Errorf("node runtime path %s is not a directory", path)
		}
		if !slices.Contains(read, path) {
			read = append(read, path)
		}
	}
	return Toolchain{Path: []string{search}, Read: read}, nil
}

// nodePackageRoot follows a Node-hosted executable to the package Node loads it
// as. The symlink is resolved first, because an executable on a search path is
// routinely a link into a package tree somewhere else, and it is the tree that
// has to be readable.
func nodePackageRoot(hosted string) (string, error) {
	resolved, err := canonicalPath(hosted)
	if err != nil {
		return "", fmt.Errorf("resolve the node-hosted executable %s: %w", hosted, err)
	}
	for path := filepath.Dir(resolved); ; path = filepath.Dir(path) {
		if info, err := os.Stat(filepath.Join(path, nodePackageMarker)); err == nil && info.Mode().IsRegular() {
			return path, nil
		}
		if parent := filepath.Dir(path); parent == path {
			// Granting the script's own directory instead would be a boundary
			// kept invented for a layout Node does not describe.
			return "", fmt.Errorf("node-hosted executable %s has no %s above it, so the package Node reads cannot be named", hosted, nodePackageMarker)
		}
	}
}

// nativeMagic is what a Mach-O executable starts with, in the byte order it is
// written on disk. A thin arm64 or x86-64 image starts with MH_MAGIC_64 or its
// byte-swapped form; a universal image starts with a fat header. Nothing else
// is a program this machine can run without an interpreter.
var nativeMagic = [][]byte{
	{0xcf, 0xfa, 0xed, 0xfe}, // MH_MAGIC_64, little-endian on disk
	{0xfe, 0xed, 0xfa, 0xcf}, // MH_CIGAM_64
	{0xce, 0xfa, 0xed, 0xfe}, // MH_MAGIC
	{0xfe, 0xed, 0xfa, 0xce}, // MH_CIGAM
	{0xca, 0xfe, 0xba, 0xbe}, // FAT_MAGIC
	{0xca, 0xfe, 0xba, 0xbf}, // FAT_MAGIC_64
}

// scriptMarker is the two bytes that make a file an interpreter's problem
// rather than the kernel's.
var scriptMarker = []byte{'#', '!'}

// ResolveNative locates a repair runtime that is a self-contained native
// executable, and it is mostly a refusal.
//
// A native runtime is the case where interrogation has nothing to ask. A Go
// toolchain keeps its caches elsewhere and a Node-hosted runtime is a script
// somebody else has to read, so both are asked where they live
// ([0053](../../docs/decision/0053-a-confined-repair-resolves-its-toolchain.md),
// [0055](../../docs/decision/0055-a-confined-runtime-is-resolved-homed-and-reaches-one-port.md)).
// A single Mach-O image has no elsewhere: the provider's command policy grants
// the launched entrypoint a direct exec bypass, so the image runs with no
// filesystem grant naming it at all. Measured against a real provider, at a
// path the provider answers `path_not_granted` for: a full confined session
// behaves identically whether the executable's directory is granted or nothing
// is. So this declares nothing, and declaring the directory anyway would be a
// grant kept invented for a read nobody makes.
//
// What it is for is the other shape. The runtime's own npm package ships a
// `#!/usr/bin/env node` fallback launcher, used whenever a postinstall did not
// run, and the vendor documents it as such — so an executable that is a script
// is a real thing to be handed, not a hypothetical. Launched under a profile
// that declares no interpreter it reports `env: node: No such file or
// directory` from inside the box, which is
// [0055](../../docs/decision/0055-a-confined-runtime-is-resolved-homed-and-reaches-one-port.md)'s
// measured failure exactly: an obscure exit that reads like a repair that ran
// and did nothing. Refusing it by name is the whole point of the function.
//
// It takes no deadline and no context because it starts nothing. A native image
// cannot be asked where it lives, and a version flag would answer a different
// question.
func ResolveNative(executable string) (Toolchain, error) {
	if executable == "" || !filepath.IsAbs(executable) {
		return Toolchain{}, fmt.Errorf("native runtime executable is not an explicit absolute path: %s", executable)
	}
	resolved, err := canonicalPath(executable)
	if err != nil {
		return Toolchain{}, fmt.Errorf("resolve the native runtime executable %s: %w", executable, err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return Toolchain{}, fmt.Errorf("native runtime executable %s is not there: %w", resolved, err)
	}
	if !info.Mode().IsRegular() {
		return Toolchain{}, fmt.Errorf("native runtime executable %s is not a regular file", resolved)
	}
	if info.Mode()&0111 == 0 {
		return Toolchain{}, fmt.Errorf("native runtime executable %s is not executable", resolved)
	}
	header, err := readHeader(resolved, 4)
	if err != nil {
		return Toolchain{}, err
	}
	if bytes.HasPrefix(header, scriptMarker) {
		return Toolchain{}, fmt.Errorf("native runtime executable %s is a script, and this adapter declares no interpreter for it: %s", resolved, shebang(resolved))
	}
	for _, magic := range nativeMagic {
		if bytes.Equal(header, magic) {
			// Nothing declared, deliberately. See the comment above: the
			// entrypoint's exec bypass is what makes the image runnable, the
			// residual is the provider's, and it is stated rather than granted.
			return Toolchain{}, nil
		}
	}
	return Toolchain{}, fmt.Errorf("native runtime executable %s is not a native image for this machine: it begins %#x", resolved, header)
}

// readHeader is the first bytes of a file, which is how a program says what
// kind of thing it is.
func readHeader(path string, size int) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read the native runtime executable %s: %w", path, err)
	}
	defer file.Close()
	header := make([]byte, size)
	read, err := io.ReadFull(file, header)
	if err != nil && read == 0 {
		return nil, fmt.Errorf("read the native runtime executable %s: %w", path, err)
	}
	return header[:read], nil
}

// shebang is the interpreter line a refused script names, so the refusal says
// which interpreter nobody resolved rather than only that there is one.
func shebang(path string) string {
	line, err := readHeader(path, 128)
	if err != nil {
		return "its interpreter line could not be read"
	}
	if at := bytes.IndexAny(line, "\r\n"); at >= 0 {
		line = line[:at]
	}
	return strings.TrimSpace(string(line))
}
