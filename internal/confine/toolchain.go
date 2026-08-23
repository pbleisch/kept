//go:build !windows

package confine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Toolchain is one toolchain as this machine actually has it.
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
