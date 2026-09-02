//go:build !windows

package confine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/pbleisch/kept/internal/thread"
)

// Profile is kept's confinement policy for one repair box. It is deliberately
// not nono's profile shape: callers derive Read and Write from the promise's
// bound repair scope, name the domain allowlist and the one child environment
// they are willing to inject, and this package renders the supported subset.
//
// Every path is absolute and stays below Box, and neither set may name the
// box's own .kept: the machine-written tier is not part of a repair tree.
// Read names the box root, because that is the confined child's working
// directory and a child that cannot read it cannot resolve its own cwd. Write
// never does: the writable set is the promise's repair scope, and the box root
// is the whole tree. Ledger and Artifacts are the independently protected
// side-ref locations the canary proves unreachable.
type Profile struct {
	// Nono and Kept are explicit executable paths. Neither provider nor
	// canary resolves a command on PATH: a path chosen by the operator is part
	// of the policy that is verified.
	Nono string
	Kept string
	Git  string

	Box       string
	Read      []string
	Write     []string
	Domains   []string
	Env       []string
	Ledger    string
	Artifacts string

	// ReadOnly is a box the confined child may read and may not write.
	//
	// It is what an adversarial pass runs in. A pass argues about a repair that
	// has already happened; it produces words rather than a diff, and a session
	// that could edit the tree it is judging could make its own argument true.
	// So Write is empty — refused if it is not — and the only writable paths a
	// read-only box has are the runtime's own scratch and home, which lie
	// outside it.
	//
	// It is not the same statement as an empty Write. A repair profile with no
	// grants is a configuration mistake and is refused as one; this is a
	// declaration, and it is what makes the canary require the probe below. See
	// docs/decision/0081.
	ReadOnly bool

	// Unwritable are directories inside a read-only box the canary must be
	// refused in, and they are the whole difference between a boundary and an
	// assertion.
	//
	// The caller names the paths a *repair* of the same promise would have been
	// granted, because those are the only ones whose refusal says anything. A
	// probe aimed at the box root proves nothing here: the root is refused as a
	// write grant in every profile this package renders, so a read-only box that
	// had quietly kept a repair's grants would pass it — which is the canary
	// that measures nothing that this package's whole design exists to catch.
	//
	// The parent proves it can write in each of them itself before the child is
	// refused one, for the reason every other escape probe has that premise: a
	// refusal from a directory nobody could write proves nothing either.
	Unwritable []string

	// BlockNetwork denies every host. An empty domain allowlist is the
	// provider's spelling of *unrestricted*, not of *none*, so a box that
	// only builds has to say this rather than name a host it does not want.
	// The two are mutually exclusive: a blocked network with an allowlist
	// beside it is a record that disagrees with what is enforced.
	BlockNetwork bool

	// LoopbackPorts are the loopback TCP ports the confined child may reach.
	//
	// A self-hosted endpoint on this machine is not a host: the provider's
	// domain allowlist is a proxy and it exempts loopback from it, so naming
	// localhost there grants nothing and the connection is refused at the
	// socket. A port is the provider's only lever for this, and on macOS it
	// grants connect *and* bind — there is no connect-only capability — so a
	// repair may also listen on a port it may reach. That width is accepted
	// and named rather than discovered later.
	//
	// The pre-flight asks the provider nothing about these. `nono why --host`
	// reports a declared loopback port as blocked against the very profile
	// that reaches it, so asking would refuse a dispatch that works. What
	// proves the boundary is the canary's host probe, which dials a loopback
	// port kept never named and must be refused. See docs/decision/0055.
	LoopbackPorts []int

	// Temp is the box's own temporary directory. The profile takes the host's
	// scratch trees back from the provider's baseline, which leaves a confined
	// child with no writable temporary directory at all, and a toolchain
	// without one fails in whatever way its own temporary file happened to be
	// used. It lies outside the box on purpose: a temporary directory inside
	// the worktree would put a build's scratch files in the repair's diff.
	Temp string

	// TempEnv are the additional variable names Temp is published under.
	//
	// TMPDIR is not universal. Claude Code reads CLAUDE_CODE_TMPDIR and falls
	// back to a hardcoded /tmp, ignoring TMPDIR entirely — measured, from
	// inside a box, as `EPERM: operation not permitted, open '/tmp/claude-501'`
	// with no stream output at all, because this profile denies the host's
	// scratch trees by name and denial beats a grant. A runtime that reads its
	// own name for the temporary directory therefore needs that name declared,
	// or it dies at a path nobody granted for a reason nobody can see.
	//
	// They are declared here rather than allowed through the child's variable
	// allowlist, for the reason Home is: one field decides where the box's
	// temporary directory is, grants it, and sets every variable naming it, so
	// a grant and a variable cannot come to disagree. An invocation may name
	// one of these, and exactly one value: Temp. See docs/decision/0056.
	TempEnv []string

	// Home is the confined child's home directory, and it is empty when the
	// child needs none — a Go build box works with no HOME at all.
	//
	// It cannot come from the child's allowlist: the provider fills an allowed
	// name from the environment of its own process, which is kept's, whose HOME
	// is the operator's. So kept declares the value instead, and one field both
	// grants the directory and sets the variable, the way Temp does — a runtime
	// that reads a home nobody projected is a runtime reading the operator's.
	//
	// It lies outside the box, because a runtime's private state inside the
	// tree would arrive in the repair's diff, and it is granted writable: a
	// runtime that locks a file to read it fails on a read-only home rather
	// than working with fewer writes. See docs/decision/0055.
	Home string

	// ToolPath is the confined child's search path, exactly.
	//
	// nono derives the child's PATH from the PATH of its own process and does
	// not filter it against the profile's variable allowlist, so this is the
	// only place the child's search path is decided — and inheriting the
	// operator's would hand a box every directory on it while the record said
	// kept granted none.
	//
	// It is resolution, not containment: a name on the search path grants
	// nothing and a name off it forbids nothing. So kept renders no grant for
	// these directories, and the pre-flight refuses a dispatch whose search
	// path the provider does not already reach.
	ToolPath []string

	// ToolRead, ToolWrite and ToolEnv are what a resolved toolchain needs:
	// paths outside the box it must read, paths outside the box it must write,
	// and the variables that tell it where they are. They come from
	// interrogating the tool — see Toolchain — never from a path list kept
	// carries. TMPDIR is not among them; Temp is where that is decided.
	ToolRead  []string
	ToolWrite []string
	ToolEnv   map[string]string

	// Credential is the one brokered credential route this box carries, or nil.
	//
	// It is the second of the two channels decision 0052 permits, and the one
	// where kept never holds the secret: the profile names a reference the
	// provider resolves, the provider publishes a session phantom in the child
	// variable named here, and it rewrites the phantom into the real value on
	// egress to the declared upstream. Measured, against a loopback listener
	// standing in for the upstream: the child sent the phantom and the listener
	// received the credential. See docs/decision/0057.
	//
	// At most one. Three runtimes need one credential each, and a list here
	// would be the general secret-provider framework 0052 refuses.
	Credential *CredentialRoute

	// CanaryTimeout bounds the provider verification, which is the pre-flight
	// and the canary. Zero is refused rather than turning a broken provider
	// into an unbounded external invocation.
	CanaryTimeout time.Duration
}

// CredentialRoute is one brokered credential, stated in kept's terms rather
// than the provider's. Nothing in it is a secret and nothing in it may become
// one: Reference names where the provider looks, and the value it finds there
// never enters this process.
type CredentialRoute struct {
	// Service is the provider's name for this route. It is not decoration: the
	// provider publishes the route's local address as <SERVICE>_BASE_URL in the
	// child, so the name a runtime already reads is the name that must be used.
	// For Claude Code that is `anthropic`, because it reads ANTHROPIC_BASE_URL.
	Service string

	// Upstream is the exact HTTPS origin the credential is spent against, with
	// no path and no query. A credential is answered by one host and naming a
	// second would be a second policy.
	Upstream string

	// Reference is what the provider resolves to get the real value. It is a
	// scheme://... URI and never a value: a bare account name is refused
	// because it cannot be told apart from a pasted secret, `env://` because it
	// resolves against this process's own environment, `cmd://` because it runs
	// a program nobody reviewed, and `keyring://` because the provider does not
	// accept it on this field.
	//
	// Two shapes reach a secret on macOS, both measured end to end at the
	// upstream: `file://<absolute path>`, and `apple-password://<server>/<account>`,
	// which the provider reads with `security find-internet-password`. That is
	// the *internet* password class and no other — a generic password stored
	// under the same names resolves to nothing, and the route then fails
	// quietly rather than refusing: `credential_not_found` on the provider's
	// stderr, an empty phantom, and a 503 to the child. Nothing pre-flights
	// resolution. See docs/nono-surface.md.
	Reference string

	// Phantom is the child variable the provider publishes the session's
	// phantom token under. The profile owns it, the way Home and TempEnv are
	// owned: an invocation may not name it, because a variable with two sources
	// has one value and two policies.
	Phantom string

	// Header and Format are how the credential is injected on egress. Format
	// carries a single {} where the secret goes.
	Header string
	Format string

	// Endpoints is the method-and-path allowlist, and it is default-deny at the
	// provider. An empty list is the provider's spelling of *every endpoint*,
	// which is the same trap an empty domain allowlist is, so it is refused.
	Endpoints []CredentialEndpoint
}

// CredentialEndpoint is one method and one path pattern the route forwards.
// `*` matches one path segment and `**` matches zero or more; a rule matches
// with a query string present, which is measured rather than assumed.
type CredentialEndpoint struct {
	Method string
	Path   string
}

// Nono is a production ConfinementLaunch. It invokes `nono run` and owns the
// process group it starts; the repair runtime never receives a process handle.
// One Nono belongs to one profile and verifies that profile before its first
// launch. A caller that changes any policy data constructs a new Nono and gets
// a fresh canary rather than accidentally carrying a prior result forward.
type Nono struct {
	profile Profile

	verifyOnce sync.Once
	verifyErr  error
}

// NewNono validates the policy before a process exists. A profile that the
// provider cannot express is refused here, rather than narrowed silently at
// launch. It does no provider I/O; Verified performs the bounded real canary.
func NewNono(profile Profile) (*Nono, error) {
	profile = cloneProfile(profile)
	if err := normalizeBoxPaths(&profile); err != nil {
		return nil, err
	}
	if err := validateProfile(profile); err != nil {
		return nil, err
	}
	return &Nono{profile: profile}, nil
}

func cloneProfile(profile Profile) Profile {
	profile.Read = append([]string(nil), profile.Read...)
	profile.Write = append([]string(nil), profile.Write...)
	profile.Domains = append([]string(nil), profile.Domains...)
	profile.Env = append([]string(nil), profile.Env...)
	profile.LoopbackPorts = append([]int(nil), profile.LoopbackPorts...)
	profile.TempEnv = append([]string(nil), profile.TempEnv...)
	profile.ToolPath = append([]string(nil), profile.ToolPath...)
	profile.ToolRead = append([]string(nil), profile.ToolRead...)
	profile.ToolWrite = append([]string(nil), profile.ToolWrite...)
	if profile.ToolEnv != nil {
		environment := make(map[string]string, len(profile.ToolEnv))
		for name, value := range profile.ToolEnv {
			environment[name] = value
		}
		profile.ToolEnv = environment
	}
	if profile.Credential != nil {
		route := *profile.Credential
		route.Endpoints = append([]CredentialEndpoint(nil), route.Endpoints...)
		profile.Credential = &route
	}
	return profile
}

func normalizeBoxPaths(profile *Profile) error {
	if profile.Box == "" || !filepath.IsAbs(profile.Box) {
		return nil
	}
	box, err := canonicalPath(profile.Box)
	if err != nil {
		return fmt.Errorf("resolve confinement box: %w", err)
	}
	profile.Box = box
	for _, set := range []struct {
		want  grant
		paths *[]string
	}{
		{readable, &profile.Read},
		{writable, &profile.Write},
	} {
		for i, path := range *set.paths {
			if err := validateBoxPath(box, path, set.want); err != nil {
				return err
			}
			resolved, err := canonicalPath(path)
			if err != nil {
				return fmt.Errorf("resolve confinement scope path %s: %w", path, err)
			}
			(*set.paths)[i] = resolved
		}
	}
	// A tool grant lies outside the box by construction, so it is canonicalized
	// here rather than through validateBoxPath: what kept renders has to be the
	// path the kernel decides about, or a deny the profile writes and a grant it
	// writes can relate to each other differently than they enforce.
	for _, paths := range []*[]string{&profile.ToolRead, &profile.ToolWrite} {
		for i, path := range *paths {
			if filepath.IsAbs(path) {
				(*paths)[i] = canonicalAncestor(path)
			}
		}
	}
	for _, path := range []*string{&profile.Temp, &profile.Home} {
		if filepath.IsAbs(*path) {
			*path = canonicalAncestor(*path)
		}
	}
	return nil
}

func validateProfile(profile Profile) error {
	for _, executable := range []struct {
		name string
		path string
	}{
		{"nono executable", profile.Nono},
		{"kept executable", profile.Kept},
		{"git executable", profile.Git},
	} {
		if executable.path == "" {
			return fmt.Errorf("nono confinement %s is unset", executable.name)
		}
		if !filepath.IsAbs(executable.path) {
			return fmt.Errorf("nono confinement %s is not an explicit absolute path: %s", executable.name, executable.path)
		}
	}
	if profile.Box == "" || !filepath.IsAbs(profile.Box) {
		return errors.New("nono confinement box is not an explicit absolute path")
	}
	for _, root := range hostTemporaryRoots() {
		// Denial beats a grant, so a box under a root the profile takes back is
		// a box the confined child cannot read, cannot write and cannot resolve
		// its own working directory in — while every escape probe is refused
		// and the canary passes. Measured. That is the failure this whole
		// package's canary exists to catch, so the profile is refused here
		// rather than rendered into a boundary that proves nothing.
		if within(root, canonicalAncestor(profile.Box)) {
			return fmt.Errorf("nono confinement box is inside the host temporary tree the profile denies at %s: %s", root, profile.Box)
		}
	}
	if profile.Ledger == "" || !filepath.IsAbs(profile.Ledger) {
		return errors.New("nono confinement ledger is not an explicit absolute path")
	}
	if profile.Artifacts == "" || !filepath.IsAbs(profile.Artifacts) {
		return errors.New("nono confinement artifacts are not an explicit absolute path")
	}
	if profile.CanaryTimeout <= 0 {
		return errors.New("nono confinement canary deadline is unset")
	}
	if len(profile.Read) == 0 {
		return errors.New("nono confinement has no scope-derived readable paths")
	}
	if !slices.Contains(profile.Read, profile.Box) {
		// Nono.start makes the box root the child's working directory. A
		// readable set that omits it produces a child whose getcwd is refused,
		// which reads downstream as a build that cannot find its own module
		// rather than as a confinement that was configured wrong.
		return errors.New("nono confinement readable paths do not name the box root, which is the confined working directory")
	}
	if err := validateReadOnly(profile); err != nil {
		return err
	}
	if profile.BlockNetwork && len(profile.Domains) > 0 {
		// The provider lets a block and an allowlist stand together and the
		// block wins, so the allowlist would be a list of hosts the record
		// says are reachable and the sandbox refuses.
		return errors.New("nono confinement blocks the network and names a domain allowlist")
	}
	if !profile.BlockNetwork && len(profile.Domains) == 0 {
		// An empty allowlist is the provider's spelling of unrestricted, not
		// of none: a profile whose only network stanza is an empty list starts
		// no proxy and reaches every host, which decision 0017 rejects. So a
		// box that wants no network says so, and one that wants some names it.
		return errors.New("nono confinement neither blocks the network nor names a domain allowlist")
	}
	seenPort := make(map[int]bool, len(profile.LoopbackPorts))
	for _, port := range profile.LoopbackPorts {
		if port < 1 || port > 65535 {
			return fmt.Errorf("nono confinement loopback port is not a TCP port: %d", port)
		}
		if seenPort[port] {
			return fmt.Errorf("nono confinement names loopback port %d twice", port)
		}
		seenPort[port] = true
	}
	if len(profile.Env) == 0 {
		return errors.New("nono confinement child environment allowlist is empty")
	}
	if err := validateToolPolicy(profile); err != nil {
		return err
	}
	if err := validateCredentialRoute(profile); err != nil {
		return err
	}

	box := profile.Box
	for _, set := range []struct {
		want  grant
		paths []string
	}{
		{readable, profile.Read},
		{writable, profile.Write},
	} {
		for _, path := range set.paths {
			if err := validateBoxPath(box, path, set.want); err != nil {
				return err
			}
		}
	}
	for _, path := range []string{profile.Kept, profile.Ledger, profile.Artifacts} {
		// Resolve before comparing: the box is canonical, so a protected path
		// written through a symlinked prefix would otherwise relate as outside
		// a box it is in fact inside.
		if within(box, canonicalAncestor(path)) {
			return fmt.Errorf("nono confinement protected path is inside the box: %s", path)
		}
	}
	for _, name := range profile.Env {
		if !environmentName(name) {
			return fmt.Errorf("nono confinement child environment name is invalid: %q", name)
		}
		if originCredentialEnvironment(name) {
			return fmt.Errorf("nono confinement child environment %s can reach origin", name)
		}
		switch name {
		case "PATH":
			// The provider ignores its own allowlist for PATH and synthesizes
			// the child's from the launching process's, so allowing it here
			// would be a policy line that decides nothing. ToolPath is where
			// the child's search path is declared.
			return errors.New("nono confinement child environment allows PATH, which the provider does not filter; declare ToolPath instead")
		case "HOME":
			// The provider resolves its own configuration and every ~-rooted
			// path against HOME, so it is the provider's variable. Passing the
			// operator's through to the child would hand a box an addressing
			// base for a home directory kept granted nothing under.
			return errors.New("nono confinement child environment allows HOME, which is the provider's own resolution base")
		}
	}
	for _, domain := range profile.Domains {
		if strings.TrimSpace(domain) == "" || strings.ContainsAny(domain, "\x00\r\n") {
			return fmt.Errorf("nono confinement domain is invalid: %q", domain)
		}
	}
	return nil
}

// validateReadOnly decides the one question that separates the two kinds of box
// this package renders.
//
// A repair box has a writable set and it is the promise's bound scope, so an
// empty one is a profile that authorizes nothing and is a mistake. A read-only
// box has none by declaration, and what makes that a boundary rather than a
// claim is the paths it names for the canary to be refused in — so a read-only
// profile that named none would be exactly the assertion the declaration is
// supposed to replace.
func validateReadOnly(profile Profile) error {
	if !profile.ReadOnly {
		if len(profile.Unwritable) > 0 {
			return errors.New("nono confinement names unwritable paths in a box that is not read-only")
		}
		if len(profile.Write) == 0 {
			return errors.New("nono confinement has no scope-derived writable paths")
		}
		return nil
	}
	if len(profile.Write) > 0 {
		return fmt.Errorf("nono confinement is read-only and grants a writable path: %s", profile.Write[0])
	}
	if len(profile.Unwritable) == 0 {
		return errors.New("nono confinement is read-only and names no path for the canary to be refused in, " +
			"so nothing would measure the boundary it declares")
	}
	for _, path := range profile.Unwritable {
		if !filepath.IsAbs(path) {
			return fmt.Errorf("nono confinement unwritable path is not an explicit absolute path: %s", path)
		}
		if strings.ContainsAny(path, "\n\r\x00") {
			// The canary carries these to the confined helper in one
			// environment value, one per line, so a path that carries a
			// separator would arrive as two paths that are neither of them
			// real.
			return fmt.Errorf("nono confinement unwritable path is invalid: %q", path)
		}
		resolved := canonicalAncestor(path)
		if !within(profile.Box, resolved) {
			return fmt.Errorf("nono confinement unwritable path is outside the box: %s", path)
		}
		rel, err := filepath.Rel(profile.Box, resolved)
		if err != nil {
			return fmt.Errorf("relate confinement unwritable path %s: %w", path, err)
		}
		if rel == hiddenTier || strings.HasPrefix(rel, hiddenTier+string(filepath.Separator)) {
			// The hidden tier is denied by name in every profile, so a refusal
			// there is the deny entry rather than the absence of a grant. It is
			// already probed as write_kept, and reusing it here would report a
			// tautology as evidence about a read-only box.
			return fmt.Errorf("nono confinement unwritable path reaches .kept inside the box: %s", path)
		}
	}
	return nil
}

// validateToolPolicy checks the half of the profile that lies outside the box:
// the search path the child is given, the toolchain paths it reaches, the
// variables that point it at them, and the box's own temporary directory.
//
// A tool grant is outside the box by construction — that is what distinguishes
// it from a repair scope — so validateBoxPath does not apply and this is the
// check that keeps "outside the box" from becoming "anywhere".
func validateToolPolicy(profile Profile) error {
	if len(profile.ToolPath) == 0 {
		return errors.New("nono confinement declares no search path for the confined child")
	}
	for _, path := range profile.ToolPath {
		if !filepath.IsAbs(path) {
			return fmt.Errorf("nono confinement search path entry is not an explicit absolute path: %s", path)
		}
		if strings.ContainsAny(path, string(os.PathListSeparator)+"\x00\r\n") {
			return fmt.Errorf("nono confinement search path entry is invalid: %q", path)
		}
	}

	if profile.Temp == "" || !filepath.IsAbs(profile.Temp) {
		return errors.New("nono confinement box temporary directory is not an explicit absolute path")
	}
	// A TMPDIR that is not there is the state this field exists to end: the
	// profile denies the host's scratch trees, so a missing box temporary
	// directory leaves the child with nowhere to write a temporary file and a
	// grant that names nothing.
	if info, err := os.Stat(profile.Temp); err != nil {
		return fmt.Errorf("nono confinement box temporary directory is not there: %w", err)
	} else if !info.IsDir() {
		return fmt.Errorf("nono confinement box temporary directory is not a directory: %s", profile.Temp)
	}

	allowed := make(map[string]bool, len(profile.Env))
	for _, name := range profile.Env {
		allowed[name] = true
	}
	named := make(map[string]bool, len(profile.TempEnv))
	for _, name := range profile.TempEnv {
		switch {
		case !environmentName(name):
			return fmt.Errorf("nono confinement temporary-directory variable name is invalid: %q", name)
		case originCredentialEnvironment(name):
			return fmt.Errorf("nono confinement temporary-directory variable %s can reach origin", name)
		case name == "TMPDIR":
			// Already set from Temp. Naming it again is a second statement of
			// one fact, which is what this field exists to prevent.
			return errors.New("nono confinement names TMPDIR as a temporary-directory variable, which Temp already sets")
		case name == "PATH" || name == "HOME":
			return fmt.Errorf("nono confinement names %s as a temporary-directory variable, and it is not one", name)
		case strings.HasPrefix(name, "NONO_"):
			return fmt.Errorf("nono confinement names %s as a temporary-directory variable, which the provider reserves", name)
		case named[name]:
			return fmt.Errorf("nono confinement names the temporary-directory variable %s twice", name)
		case allowed[name]:
			// The allowlist fills a name from the launching process, so a
			// variable on both would have two sources and one value.
			return fmt.Errorf("nono confinement child environment allows %s, which Temp already sets", name)
		}
		if _, set := profile.ToolEnv[name]; set {
			return fmt.Errorf("nono confinement tool environment sets %s, which Temp already sets", name)
		}
		named[name] = true
	}

	written := append(append([]string(nil), profile.ToolWrite...), profile.Temp)
	if profile.Home != "" {
		if !filepath.IsAbs(profile.Home) {
			return fmt.Errorf("nono confinement child home directory is not an explicit absolute path: %s", profile.Home)
		}
		// The same rule the temporary directory has, and for the same reason:
		// a home that is not there is a grant naming nothing, and the runtime
		// pointed at it fails in whatever way its own first write happened to
		// be made.
		if info, err := os.Stat(profile.Home); err != nil {
			return fmt.Errorf("nono confinement child home directory is not there: %w", err)
		} else if !info.IsDir() {
			return fmt.Errorf("nono confinement child home directory is not a directory: %s", profile.Home)
		}
		written = append(written, profile.Home)
	}

	outside := map[string]bool{}
	for _, set := range []struct {
		want  grant
		paths []string
	}{
		{readable, profile.ToolRead},
		{writable, written},
	} {
		for _, path := range set.paths {
			if err := validateToolPath(profile, path); err != nil {
				return err
			}
			if held, seen := outside[path]; seen && held != (set.want == writable) {
				return fmt.Errorf("nono confinement grants %s both readable and writable", path)
			}
			outside[path] = set.want == writable
		}
	}

	for name, value := range profile.ToolEnv {
		if !environmentName(name) {
			return fmt.Errorf("nono confinement tool environment name is invalid: %q", name)
		}
		if originCredentialEnvironment(name) {
			return fmt.Errorf("nono confinement tool environment %s can reach origin", name)
		}
		switch {
		case name == "PATH":
			// The provider rejects it at load, and ToolPath is where the
			// child's search path is decided.
			return errors.New("nono confinement tool environment sets PATH, which the provider reserves")
		case name == "TMPDIR":
			// One place decides where the box's temporary directory is, and it
			// is the field that is also granted.
			return errors.New("nono confinement tool environment sets TMPDIR, which Temp already decides")
		case name == "HOME":
			// Same rule: the variable and the grant it has to agree with would
			// otherwise be two fields.
			return errors.New("nono confinement tool environment sets HOME, which Home already decides")
		case strings.HasPrefix(name, "NONO_"):
			return fmt.Errorf("nono confinement tool environment sets %s, which the provider reserves", name)
		}
		if value == "" || strings.ContainsAny(value, "\x00\r\n") {
			return fmt.Errorf("nono confinement tool environment value for %s is invalid: %q", name, value)
		}
		if strings.Contains(value, "$") {
			// The provider expands nine variables and accepts every other
			// dollar sign as a literal, silently — a grant on $GOPATH/pkg/mod
			// validates clean and names a directory that does not exist. kept
			// resolves, so a variable here is a value that was never resolved.
			return fmt.Errorf("nono confinement tool environment value for %s was not resolved: %q", name, value)
		}
	}
	return nil
}

// validateToolPath refuses a path outside the box that reaches something the
// repair boundary protects, or that lands where the profile's own denials would
// take it back.
func validateToolPath(profile Profile, path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("nono confinement tool path is not an explicit absolute path: %s", path)
	}
	resolved := canonicalAncestor(path)
	if resolved == string(filepath.Separator) {
		return errors.New("nono confinement tool path is the filesystem root")
	}
	if within(resolved, profile.Box) || within(profile.Box, resolved) {
		// Inside the box is a scope path and goes through Read and Write;
		// containing the box is a grant over the whole repair tree.
		return fmt.Errorf("nono confinement tool path relates to the box, which is what Read and Write are for: %s", path)
	}
	for _, protected := range []string{profile.Kept, profile.Ledger, profile.Artifacts} {
		other := canonicalAncestor(protected)
		if within(resolved, other) || within(other, resolved) {
			return fmt.Errorf("nono confinement tool path reaches a protected location at %s: %s", protected, path)
		}
	}
	for _, root := range hostTemporaryRoots() {
		// Denial beats a grant, so a tool path inside a tree the profile takes
		// back is a grant the provider will refuse — which arrives from inside
		// a box as a toolchain that cannot find itself.
		if within(root, resolved) {
			return fmt.Errorf("nono confinement tool path is inside the host temporary tree the profile denies at %s: %s", root, path)
		}
	}
	return nil
}

// validateCredentialRoute checks the brokered credential channel before a
// process exists. Every refusal here is one of the measurements decision 0057
// records, turned into something that fails at configuration time rather than
// from inside a box.
func validateCredentialRoute(profile Profile) error {
	route := profile.Credential
	if route == nil {
		return nil
	}
	if profile.BlockNetwork {
		// The provider refuses this combination outright — "--block-net and
		// --credential are contradictory: credential injection requires the
		// proxy to be reachable" — so kept refuses it earlier and says which
		// two settings disagree.
		return errors.New("nono confinement blocks the network and carries a brokered credential, which needs the proxy reachable")
	}
	if err := CheckCredentialRoute(route, profile.Domains); err != nil {
		return err
	}
	// The rest is about this profile rather than about the route: a reference
	// whose target the child can read, and a phantom variable something else in
	// the profile also fills.
	if scheme, rest, _ := strings.Cut(route.Reference, "://"); scheme == "file" {
		if err := validateCredentialFile(profile, rest); err != nil {
			return err
		}
	}
	switch {
	case slices.Contains(profile.Env, route.Phantom):
		// The allowlist fills a name from kept's own process. A phantom
		// variable on it as well would have two sources and one value, and the
		// source kept controls is the one holding a real credential.
		return fmt.Errorf("nono confinement child environment allows %s, which the credential route already publishes", route.Phantom)
	case slices.Contains(profile.TempEnv, route.Phantom):
		return fmt.Errorf("nono confinement names %s as a temporary-directory variable, which the credential route already publishes", route.Phantom)
	}
	if _, set := profile.ToolEnv[route.Phantom]; set {
		return fmt.Errorf("nono confinement tool environment sets %s, which the credential route already publishes", route.Phantom)
	}
	return nil
}

// CheckCredentialRoute is the half of the check that is about the route itself
// rather than about the profile around it.
//
// It is exported for one reason: whatever assembles a route has to be held to
// these rules at the point it assembles one, not only once a whole profile has
// been built around it. Two packages agreeing about a credential by having each
// written the rules down is how they come to disagree.
func CheckCredentialRoute(route *CredentialRoute, domains []string) error {
	if route == nil {
		return errors.New("nono confinement credential route is absent")
	}
	if !identifierName(route.Service) {
		// It becomes <SERVICE>_BASE_URL in the child, so a name that is not an
		// environment name produces a variable no runtime can read.
		return fmt.Errorf("nono confinement credential service name is invalid: %q", route.Service)
	}
	host, err := credentialUpstreamHost(route.Upstream)
	if err != nil {
		return err
	}
	if !loopbackHost(host) && !slices.Contains(domains, host) {
		// The route is what reaches the upstream, but the allowlist is what
		// says which hosts this box may reach at all. A route naming a host the
		// allowlist does not would be a record that disagrees with the policy
		// beside it — and a profile with a route and no allowlist reaches every
		// host on the internet, measured.
		//
		// A loopback upstream is exempt for the reason decision 0055 records:
		// the allowlist is a proxy and the provider exempts loopback from it,
		// so naming one there grants nothing and would be a policy line that
		// decides nothing.
		return fmt.Errorf("nono confinement credential upstream %s is not in the domain allowlist", host)
	}
	if err := validateCredentialReference(route.Reference); err != nil {
		return err
	}
	if err := validateCredentialPhantom(route.Phantom); err != nil {
		return err
	}
	if !headerName(route.Header) {
		return fmt.Errorf("nono confinement credential header is invalid: %q", route.Header)
	}
	if strings.Count(route.Format, "{}") != 1 || strings.ContainsAny(route.Format, "\x00\r\n") {
		// One placeholder exactly: none injects a constant and never the
		// credential, and two would put it in the request twice.
		return fmt.Errorf("nono confinement credential format does not carry exactly one {} placeholder: %q", route.Format)
	}
	if len(route.Endpoints) == 0 {
		// The provider reads an absent or empty rule list as *every endpoint*,
		// which is the same spelling trap an empty domain allowlist is.
		return errors.New("nono confinement credential route names no endpoints, which the provider reads as every endpoint")
	}
	seen := make(map[string]bool, len(route.Endpoints))
	for _, endpoint := range route.Endpoints {
		if endpoint.Method == "" || strings.ContainsAny(endpoint.Method, " \t\x00\r\n") {
			return fmt.Errorf("nono confinement credential endpoint method is invalid: %q", endpoint.Method)
		}
		if !strings.HasPrefix(endpoint.Path, "/") || strings.ContainsAny(endpoint.Path, " \t\x00\r\n?#") {
			// A rule already matches with a query string present, so a `?` here
			// is a rule that was written to match one and never will.
			return fmt.Errorf("nono confinement credential endpoint path is invalid: %q", endpoint.Path)
		}
		key := endpoint.Method + " " + endpoint.Path
		if seen[key] {
			return fmt.Errorf("nono confinement credential endpoint is named twice: %s", key)
		}
		seen[key] = true
	}
	return nil
}

// credentialUpstreamHost checks the upstream is one exact HTTPS origin and
// returns its host. A path or a query would be silently ignored by the provider,
// which routes on the origin.
func credentialUpstreamHost(upstream string) (string, error) {
	parsed, err := url.Parse(upstream)
	if err != nil {
		return "", fmt.Errorf("nono confinement credential upstream is not a URL: %q", upstream)
	}
	switch {
	case parsed.Scheme == "http" && !loopbackHost(parsed.Hostname()):
		// The provider's own rule, and kept keeps it rather than narrowing it:
		// an unencrypted upstream off this machine would put the credential on
		// the wire in the clear, and a self-hosted endpoint on loopback is the
		// case decision 0055 declared a port for.
		return "", fmt.Errorf("nono confinement credential upstream is http and is not loopback: %q", upstream)
	case parsed.Scheme != "https" && parsed.Scheme != "http":
		return "", fmt.Errorf("nono confinement credential upstream is not https: %q", upstream)
	case parsed.Host == "":
		return "", fmt.Errorf("nono confinement credential upstream names no host: %q", upstream)
	case parsed.User != nil:
		return "", fmt.Errorf("nono confinement credential upstream carries userinfo: %q", upstream)
	case parsed.Path != "" && parsed.Path != "/":
		return "", fmt.Errorf("nono confinement credential upstream carries a path: %q", upstream)
	case parsed.RawQuery != "" || parsed.Fragment != "":
		return "", fmt.Errorf("nono confinement credential upstream carries a query or fragment: %q", upstream)
	}
	return parsed.Hostname(), nil
}

// credentialReferenceRefused is the scheme kept will not resolve through, and
// why. None is a judgment about the provider: the first two are fine features
// that route the secret somewhere kept promised it would not be, and the third
// is one the provider does not offer on this field at all.
//
// keyring:// is the one to be careful with, because the provider's own README
// shows it inside a worked credential route. It does not parse there: measured
// against nono 0.74.0, every spelling — `service/account`, `service:account`,
// with and without `?decode=go-keyring` — is refused before any process exists
// with "must contain only alphanumeric characters and underscores (or use
// op:// / bw:// / apple-password:// / file:// / env:// / cmd:// URI)". It is
// real on the provider's *other* credential surface, the one that fills a child
// variable directly, which is the channel decision 0052 refuses. Refusing it
// here turns a late parse error into an early refusal that says which field.
var credentialReferenceRefused = map[string]string{
	"env":     "resolves against this process's own environment, so kept would hold the secret",
	"cmd":     "runs a program to produce the secret, and nothing reviewed that program",
	"keyring": "the provider refuses on a credential route however its own guide spells it, and accepts only on the direct channel kept does not use",
}

// validateCredentialReference refuses anything that could be a value rather
// than a reference. A bare account name is the dangerous case: it is what the
// provider's own documentation shows first, it is indistinguishable from a
// pasted secret, and a secret pasted there would be rendered into a policy file.
func validateCredentialReference(reference string) error {
	if reference == "" {
		return errors.New("nono confinement credential reference is unset")
	}
	if strings.ContainsAny(reference, " \t\x00\r\n") {
		return fmt.Errorf("nono confinement credential reference is invalid: %q", reference)
	}
	scheme, rest, found := strings.Cut(reference, "://")
	if !found || scheme == "" || rest == "" {
		return fmt.Errorf("nono confinement credential reference is not a scheme://... reference, and a bare name cannot be told apart from a secret: %q", reference)
	}
	if !identifierScheme(scheme) {
		return fmt.Errorf("nono confinement credential reference scheme is invalid: %q", scheme)
	}
	if why, refused := credentialReferenceRefused[scheme]; refused {
		return fmt.Errorf("nono confinement credential reference uses %s://, which %s", scheme, why)
	}
	if scheme == "file" && !filepath.IsAbs(rest) {
		return fmt.Errorf("nono confinement credential file reference is not an explicit absolute path: %s", rest)
	}
	return nil
}

// validateCredentialFile keeps a plaintext reference out of the child's reach.
//
// A file:// reference is not refused — kept still never reads it, and a 0600
// file is an ordinary way for an operator to hold a secret — but it is the one
// scheme whose target is a path, and a path the profile grants is a path the
// confined child reads. That turns the brokered channel back into the direct
// one without anything saying so.
func validateCredentialFile(profile Profile, path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("nono confinement credential file reference is not an explicit absolute path: %s", path)
	}
	resolved := canonicalAncestor(path)
	if within(profile.Box, resolved) {
		return fmt.Errorf("nono confinement credential file reference is inside the box: %s", path)
	}
	granted := append(append([]string(nil), profile.Read...), profile.Write...)
	granted = append(granted, profile.ToolRead...)
	granted = append(granted, profile.ToolWrite...)
	granted = append(granted, profile.Temp)
	if profile.Home != "" {
		granted = append(granted, profile.Home)
	}
	for _, grant := range granted {
		if within(canonicalAncestor(grant), resolved) {
			return fmt.Errorf("nono confinement credential file reference is inside a path the child is granted at %s: %s", grant, path)
		}
	}
	return nil
}

// validateCredentialPhantom checks the name itself. Whether something else in
// a profile also fills it is that profile's question, and is asked where the
// profile is known.
func validateCredentialPhantom(name string) error {
	switch {
	case !environmentName(name):
		return fmt.Errorf("nono confinement credential phantom variable name is invalid: %q", name)
	case originCredentialEnvironment(name):
		return fmt.Errorf("nono confinement credential phantom variable %s can reach origin", name)
	case name == "PATH" || name == "HOME" || name == "TMPDIR":
		return fmt.Errorf("nono confinement names %s as the credential phantom variable, and it is not one", name)
	case strings.HasPrefix(name, "NONO_"):
		return fmt.Errorf("nono confinement names %s as the credential phantom variable, which the provider reserves", name)
	}
	return nil
}

// identifierName is the provider's service-name rule: it is uppercased into
// <SERVICE>_BASE_URL, so it has to survive that.
func identifierName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '_':
		case c >= '0' && c <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}

func identifierScheme(scheme string) bool {
	for i := 0; i < len(scheme); i++ {
		c := scheme[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c == '-' && i > 0:
		default:
			return false
		}
	}
	return scheme != ""
}

// headerName is RFC 7230's token rule, narrowed to what a policy file should
// ever carry.
func headerName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_':
		default:
			return false
		}
	}
	return true
}

// loopbackHost is this machine and nothing else. A name is not resolved to find
// out: a hostname that resolves to loopback today is a hostname somebody else
// controls tomorrow, so only the literals count.
func loopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

func originCredentialEnvironment(name string) bool {
	switch name {
	case "GIT_ASKPASS", "GIT_SSH", "GIT_SSH_COMMAND", "GIT_TOKEN", "GH_TOKEN", "GITHUB_TOKEN", "SSH_AUTH_SOCK":
		return true
	default:
		return false
	}
}

func canonicalPath(path string) (string, error) {
	return filepath.EvalSymlinks(path)
}

// canonicalAncestor resolves the deepest existing ancestor of path and rejoins
// the remainder. Existence is not a precondition for relating a path to the
// box: a scope path may name a directory this box does not have, and a
// protected location may never be materialized at all, because the ledger lives
// on a side ref. EvalSymlinks answers nothing about a path that is not there,
// and on a machine whose temporary tree is itself a symlink an unresolved path
// then relates as outside a box it is in fact inside.
func canonicalAncestor(path string) string {
	path = filepath.Clean(path)
	remainder := ""
	for {
		if resolved, err := canonicalPath(path); err == nil {
			return filepath.Join(resolved, remainder)
		}
		parent := filepath.Dir(path)
		if parent == path {
			return filepath.Join(path, remainder)
		}
		remainder = filepath.Join(filepath.Base(path), remainder)
		path = parent
	}
}

// grant is what one scope path is being validated for. The two differ at the
// box root and nowhere else, and the difference is the boundary: the root is
// the confined child's working directory and must be readable, while making it
// writable would hand a repair the whole tree instead of the scope its promise
// bound. See docs/decision/0047-a-promise-binds-what-a-repair-may-touch.md.
type grant string

const (
	readable grant = "readable"
	writable grant = "writable"
)

// hiddenTier is the machine-written directory, relative to a tree's root.
const hiddenTier = ".kept"

// validateBoxPath decides whether one scope path may carry one grant.
//
// The hidden tier is located relative to the box and never by scanning the
// absolute path for a component named .kept. A box lives at
// <repository>/.kept/boxes/<id>, so every path inside an ordinary box carries
// that component and a scan refuses every box internal/box creates. What has to
// stay out of reach is the machine-written tier itself — the ledger, the
// artifacts and kept's own state — and all of that lies outside the box:
// containment is the check that keeps a scope path away from it, and
// validateProfile separately refuses a profile that puts one of them inside.
func validateBoxPath(box, path string, want grant) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("nono confinement scope path is not an explicit absolute path: %s", path)
	}
	// Relate the path as written as well as resolved. The written form refuses a
	// name that reaches the hidden tier through a symlink pointing somewhere
	// innocent; the resolved form refuses a symlink that reaches it under an
	// innocent name. Containment is asked only of the resolved form, because an
	// unresolved path can differ from the canonical box by a symlinked prefix.
	if err := validateBoxRelation(box, path, path, want); err != nil {
		return err
	}
	resolved := canonicalAncestor(path)
	if !within(box, resolved) {
		return fmt.Errorf("nono confinement scope path is outside the box: %s", path)
	}
	return validateBoxRelation(box, resolved, path, want)
}

// validateBoxRelation applies the rules about where inside the box a path may
// lie. reported is the path as the caller wrote it, so a refusal names what was
// configured rather than what a symlink resolved to.
func validateBoxRelation(box, path, reported string, want grant) error {
	rel, err := filepath.Rel(box, path)
	if err != nil {
		return fmt.Errorf("relate confinement scope path %s: %w", reported, err)
	}
	if rel == hiddenTier || strings.HasPrefix(rel, hiddenTier+string(filepath.Separator)) {
		return fmt.Errorf("nono confinement scope path reaches .kept inside the box: %s", reported)
	}
	if rel == "." && want == writable {
		return fmt.Errorf("nono confinement writable scope path is the box root: %s", reported)
	}
	return nil
}

func within(parent, path string) bool {
	rel, err := filepath.Rel(parent, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func environmentName(name string) bool {
	for i, r := range name {
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r == '_' {
			continue
		}
		if i > 0 && r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return name != ""
}

// Verified runs the real canary exactly once for this immutable profile. A
// failed or timed-out result remains failed: it is evidence about this launch,
// not a transient affirmative to retry until it says yes.
func (n *Nono) Verified() error {
	n.verifyOnce.Do(func() { n.verifyErr = n.verify() })
	return n.verifyErr
}

// Start starts a directly shaped runtime invocation under `nono run`. The
// caller has already called Verified; Start repeats neither canary nor policy
// selection. Its Child kills the complete nono-led process group and reaps it.
func (n *Nono) Start(ctx context.Context, invocation thread.Invocation) (thread.Child, error) {
	if ctx == nil {
		return nil, errors.New("nono confinement launch context is nil")
	}
	if err := n.Verified(); err != nil {
		return nil, fmt.Errorf("nono confinement canary failed: %w", err)
	}
	if invocation.Path == "" || !filepath.IsAbs(invocation.Path) {
		return nil, errors.New("nono confinement runtime executable is not an explicit absolute path")
	}
	if err := validateInvocationEnvironment(n.profile, invocation.Env); err != nil {
		return nil, err
	}
	return n.start(ctx, invocation, repairLaunch)
}

// validateInvocationEnvironment holds an adapter's declared child environment
// to what the profile actually delivers.
//
// There are two channels and they are not interchangeable. A name on the
// allowlist is passed through from the provider's process, which is how a
// credential reaches a child without being written to a policy file; HOME is
// the one name the profile itself sets, so an invocation may name it and may
// name exactly the declared value. A disagreement is refused rather than
// silently resolved in the profile's favour, because the two are one fact.
func validateInvocationEnvironment(profile Profile, values []string) error {
	allowedSet := make(map[string]bool, len(profile.Env))
	for _, name := range profile.Env {
		allowedSet[name] = true
	}
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		name, rest, found := strings.Cut(value, "=")
		if !found || !environmentName(name) {
			return fmt.Errorf("nono confinement child environment is malformed: %q", value)
		}
		if seen[name] {
			return fmt.Errorf("nono confinement child environment names %s twice", name)
		}
		if name == "HOME" {
			if profile.Home == "" {
				return errors.New("nono confinement child environment names HOME and the profile declares no child home")
			}
			if canonicalAncestor(rest) != profile.Home {
				return fmt.Errorf("nono confinement child environment sets HOME to %s and the profile declares %s", rest, profile.Home)
			}
			seen[name] = true
			continue
		}
		// The same rule, for the same reason: the adapter's environment and the
		// rendered policy are two statements of one fact, and a runtime reading
		// a temporary directory nobody granted is a runtime dying at a denied
		// path with nothing saying why. See docs/decision/0056.
		if slices.Contains(profile.TempEnv, name) {
			if canonicalAncestor(rest) != profile.Temp {
				return fmt.Errorf("nono confinement child environment sets %s to %s and the profile declares %s", name, rest, profile.Temp)
			}
			seen[name] = true
			continue
		}
		if profile.Credential != nil && name == profile.Credential.Phantom {
			// The provider publishes this one, with a value it mints. An
			// adapter setting it as well is the direct credential channel
			// wearing the brokered one's clothes, and the two cannot both be
			// in force. See docs/decision/0057.
			return fmt.Errorf("nono confinement child environment names %s, which the credential route publishes", name)
		}
		if !allowedSet[name] {
			return fmt.Errorf("nono confinement child environment %s is not in the profile allowlist", name)
		}
		seen[name] = true
	}
	return nil
}

// launchKind is which of the two things this package starts. The rendered
// policy is the same for both, which is what makes the canary evidence about a
// repair rather than about itself; only the child environment differs, because
// the confined helper is told where its sentinels are.
type launchKind bool

const (
	repairLaunch launchKind = false
	canaryLaunch launchKind = true
)

func (n *Nono) start(ctx context.Context, invocation thread.Invocation, kind launchKind) (thread.Child, error) {
	dir, err := os.MkdirTemp("", "kept-nono-")
	if err != nil {
		return nil, fmt.Errorf("create nono launch state: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(dir) }

	profile := cloneProfile(n.profile)
	if kind == canaryLaunch {
		profile.Env = append([]string(nil), canaryEnvironment...)
	}
	profilePath := filepath.Join(dir, "profile.json")
	if err := writeNonoProfile(profilePath, profile, invocation.Path); err != nil {
		cleanup()
		return nil, err
	}
	if err := n.preflight(ctx, profilePath); err != nil {
		cleanup()
		return nil, err
	}

	args := []string{"run", "--silent", "--no-audit", "--no-rollback", "--profile", profilePath, "--", invocation.Path}
	args = append(args, invocation.Args...)
	cmd := exec.Command(n.profile.Nono, args...)
	cmd.Dir = profile.Box
	cmd.Env = childEnvironment(profile, invocation.Env)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("open nono runtime input: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("open nono runtime output: %w", err)
	}
	stderr := &boundedBuffer{limit: 8 << 10}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		cleanup()
		return nil, fmt.Errorf("start nono run: %w", err)
	}
	if invocation.BindInput != nil {
		if err := invocation.BindInput(stdin); err != nil {
			_ = killProcessGroup(cmd.Process.Pid)
			_ = cmd.Wait()
			cleanup()
			return nil, fmt.Errorf("bind confined runtime input: %w", err)
		}
	}
	if len(invocation.Input) != 0 {
		if _, err := stdin.Write(invocation.Input); err != nil {
			_ = killProcessGroup(cmd.Process.Pid)
			_ = cmd.Wait()
			cleanup()
			return nil, fmt.Errorf("write confined runtime input: %w", err)
		}
	}
	if invocation.BindInput == nil {
		if err := stdin.Close(); err != nil {
			_ = killProcessGroup(cmd.Process.Pid)
			_ = cmd.Wait()
			cleanup()
			return nil, fmt.Errorf("close confined runtime input: %w", err)
		}
	}

	child := &child{cmd: cmd, stdin: stdin, cleanup: cleanup, stderr: stderr}
	child.copyDone = make(chan struct{})
	go func() {
		if invocation.Output != nil {
			_, child.copyErr = io.Copy(invocation.Output, stdout)
		} else {
			_, child.copyErr = io.Copy(io.Discard, stdout)
		}
		close(child.copyDone)
	}()
	return child, nil
}

// childEnvironment builds the environment of the **nono process**, which is not
// the environment of the confined child: the provider filters that one against
// the profile's variable allowlist. PATH is the exception it does not filter —
// it synthesizes the child's search path from its own — so merging the
// operator's environment here handed a repair every directory on the operator's
// PATH while kept's record said it granted none. The profile's ToolPath is the
// only place the child's search path is decided.
//
// HOME goes the other way. The provider resolves its own configuration and
// every ~-rooted policy path against it and refuses to start without one, so it
// is given the operator's — and validateProfile refuses HOME on the child's
// allowlist, which is what keeps it the provider's and not the box's.
func childEnvironment(profile Profile, injected []string) []string {
	merged := make(map[string]string, len(injected)+2)
	for _, value := range injected {
		name, _, _ := strings.Cut(value, "=")
		merged[name] = value
	}
	merged["PATH"] = "PATH=" + strings.Join(profile.ToolPath, string(os.PathListSeparator))
	if home, ok := os.LookupEnv("HOME"); ok {
		merged["HOME"] = "HOME=" + home
	}
	names := make([]string, 0, len(merged))
	for name := range merged {
		names = append(names, name)
	}
	sort.Strings(names)
	output := make([]string, 0, len(names))
	for _, name := range names {
		output = append(output, merged[name])
	}
	return output
}

type nonoProfile struct {
	// Groups is rendered empty and on purpose. The provider applies eighteen
	// deny and system groups to any profile including this one, and a
	// toolchain group on top of a resolved path list grants directories nobody
	// resolved and the pre-flight never checked. See docs/decision/0053.
	Groups struct {
		Include []string `json:"include"`
	} `json:"groups"`
	Filesystem struct {
		Read  []string `json:"read"`
		Allow []string `json:"allow"`
		Deny  []string `json:"deny"`
	} `json:"filesystem"`
	Network struct {
		Block       bool     `json:"block,omitempty"`
		AllowDomain []string `json:"allow_domain,omitempty"`
		OpenPort    []int    `json:"open_port,omitempty"`
		// Credentials names which routes the provider actually starts. A route
		// defined and not named here is configuration that does nothing, which
		// the provider's own guide says in as many words.
		Credentials       []string                        `json:"credentials,omitempty"`
		CustomCredentials map[string]nonoCustomCredential `json:"custom_credentials,omitempty"`
	} `json:"network"`
	Environment struct {
		Allow []string          `json:"allow_vars"`
		Set   map[string]string `json:"set_vars,omitempty"`
	} `json:"environment"`
	CommandPolicies map[string]any `json:"command_policies"`
}

// nonoCustomCredential is the provider's shape for one brokered route. Every
// field here is a reference, a name or a pattern: the credential value has no
// field, which is the property the whole channel exists for.
type nonoCustomCredential struct {
	Upstream      string             `json:"upstream"`
	CredentialKey string             `json:"credential_key"`
	EnvVar        string             `json:"env_var"`
	InjectMode    string             `json:"inject_mode"`
	InjectHeader  string             `json:"inject_header"`
	Format        string             `json:"credential_format"`
	EndpointRules []nonoEndpointRule `json:"endpoint_rules"`
}

type nonoEndpointRule struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

// hostTemporaryRoots are the host's scratch trees, which the profile takes back
// from the provider's baseline.
//
// They are the largest grant nothing in kept ever asked for: nono's baseline
// grants the operating system's temporary trees write access *recursively*, so
// a repair that was refused every path kept names could still write anywhere
// under them — measured, from inside a rendered profile, before this entry
// existed. Nothing narrower closes it: denying /tmp alone leaves the per-user
// root under /var/folders granted, and on darwin the grant covers that whole
// tree rather than only its T directory.
//
// A path here is not conditional on the platform. A root that does not exist
// cannot be reached and the provider accepts naming it, while omitting one per
// platform would make the rendered policy differ from the policy read.
// Consequences: a box under one of these roots is refused outright rather than
// silently unconfined, which is why a test that measures the boundary cuts its
// box under the operator's home; and a confined child is left with no writable
// temporary directory at all until one inside the box is granted to it.
func hostTemporaryRoots() []string {
	seen := make(map[string]bool, 8)
	roots := make([]string, 0, 8)
	add := func(path string) {
		path = filepath.Clean(path)
		// The filesystem root would deny the box along with everything else.
		if path == "" || path == string(filepath.Separator) || seen[path] {
			return
		}
		seen[path] = true
		roots = append(roots, path)
	}
	for _, root := range []string{"/tmp", "/var/tmp", "/var/folders", os.TempDir()} {
		// Both forms: the provider resolves a deny entry, and a policy that
		// depends on it resolving one is a policy that reads differently from
		// what it enforces.
		add(root)
		add(canonicalAncestor(root))
	}
	return roots
}

func writeNonoProfile(path string, profile Profile, entrypoint string) error {
	rendered, err := renderNonoProfile(profile, entrypoint)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(rendered)
	if err != nil {
		return fmt.Errorf("render nono profile: %w", err)
	}
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		return fmt.Errorf("write nono profile: %w", err)
	}
	return nil
}

func renderNonoProfile(profile Profile, entrypoint string) (nonoProfile, error) {
	var rendered nonoProfile
	// Never nil: the provider rejects `include: null`, and an empty list is
	// kept saying it names no policy group rather than saying nothing.
	rendered.Groups.Include = []string{}
	rendered.Filesystem.Read = append(append([]string(nil), profile.Read...), profile.ToolRead...)
	rendered.Filesystem.Allow = append(append([]string(nil), profile.Write...), profile.ToolWrite...)
	// The box's own temporary directory is the one writable path outside the
	// box that exists because the profile denied something, rather than because
	// a toolchain asked for it.
	rendered.Filesystem.Allow = append(rendered.Filesystem.Allow, profile.Temp)
	if profile.Home != "" {
		rendered.Filesystem.Allow = append(rendered.Filesystem.Allow, profile.Home)
	}
	// The box root is readable, so the box's own machine-written tier is denied
	// by name rather than by being left ungranted. Denial beats a read grant, so
	// this is what keeps kept's state out of a repair's reach once the tree
	// around it is legible. The canary's write_kept probe is aimed at this
	// entry, which is policy every launch carries rather than a canary-only one.
	rendered.Filesystem.Deny = append([]string{
		profile.Kept,
		profile.Ledger,
		profile.Artifacts,
		filepath.Join(profile.Box, hiddenTier),
	}, hostTemporaryRoots()...)
	if profile.BlockNetwork {
		rendered.Network.Block = true
	} else {
		rendered.Network.AllowDomain = append([]string(nil), profile.Domains...)
	}
	if route := profile.Credential; route != nil {
		rules := make([]nonoEndpointRule, 0, len(route.Endpoints))
		for _, endpoint := range route.Endpoints {
			rules = append(rules, nonoEndpointRule{Method: endpoint.Method, Path: endpoint.Path})
		}
		rendered.Network.Credentials = []string{route.Service}
		rendered.Network.CustomCredentials = map[string]nonoCustomCredential{route.Service: {
			Upstream:      route.Upstream,
			CredentialKey: route.Reference,
			EnvVar:        route.Phantom,
			InjectMode:    "header",
			InjectHeader:  route.Header,
			Format:        route.Format,
			EndpointRules: rules,
		}}
	}
	// Loopback stands beside either. It is not routed through the proxy the
	// allowlist starts — the provider exempts loopback from it — so it is
	// orthogonal to whether hosts are reachable rather than a third setting of
	// the same switch.
	rendered.Network.OpenPort = append([]int(nil), profile.LoopbackPorts...)
	rendered.Environment.Allow = append([]string(nil), profile.Env...)
	rendered.Environment.Set = make(map[string]string, len(profile.ToolEnv)+1)
	for name, value := range profile.ToolEnv {
		rendered.Environment.Set[name] = value
	}
	rendered.Environment.Set["TMPDIR"] = profile.Temp
	// The same directory under whatever else names it. One field decided it,
	// so the grant above and every one of these is one statement.
	for _, name := range profile.TempEnv {
		rendered.Environment.Set[name] = profile.Temp
	}
	if profile.Home != "" {
		rendered.Environment.Set["HOME"] = profile.Home
	}
	// Filesystem denial is not enough for an executable that macOS has already
	// mapped for a process. nono's tool sandbox guards the direct session edge
	// as well, so the known kept binary cannot be re-executed to ratify from a
	// repair even if the host's executable cache can read it.
	entryName := filepath.Base(entrypoint)
	if entryName == filepath.Base(profile.Kept) {
		return rendered, errors.New("nono confinement runtime executable is the denied kept executable")
	}
	rendered.CommandPolicies = map[string]any{
		"entrypoint": entryName,
		"commands": map[string]any{
			entryName: map[string]any{
				"executable":                entrypoint,
				"sandbox":                   map[string]any{},
				"allow_writable_executable": true,
				"allow_direct_exec_bypass":  []string{entrypoint},
			},
			filepath.Base(profile.Kept): map[string]any{
				"executable":                profile.Kept,
				"from":                      map[string]any{"session": "deny"},
				"allow_writable_executable": true,
			},
		},
	}
	return rendered, nil
}

type child struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	cleanup func()
	stderr  *boundedBuffer

	copyDone chan struct{}
	copyErr  error
	waitOnce sync.Once
	exit     thread.Exit
}

func (c *child) diagnostic() string { return c.stderr.String() }

type boundedBuffer struct {
	mu        sync.Mutex
	bytes     bytes.Buffer
	limit     int
	truncated bool
}

func (b *boundedBuffer) Write(src []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	remaining := b.limit - b.bytes.Len()
	if b.bytes.Len() < b.limit {
		if remaining > len(src) {
			remaining = len(src)
		}
		_, _ = b.bytes.Write(src[:remaining])
	}
	if len(src) > remaining {
		b.truncated = true
	}
	return len(src), nil
}

func (b *boundedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.TrimSpace(b.bytes.String())
}

func (b *boundedBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.bytes.Bytes()...)
}

func (b *boundedBuffer) Truncated() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.truncated
}

func (c *child) Kill() error {
	return killProcessGroup(c.cmd.Process.Pid)
}

func killProcessGroup(pid int) error {
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
		return fmt.Errorf("kill nono process group %d: %w", pid, err)
	}
	return nil
}

func (c *child) Wait() thread.Exit {
	c.waitOnce.Do(func() {
		waitErr := c.cmd.Wait()
		if c.stdin != nil {
			_ = c.stdin.Close()
		}
		<-c.copyDone
		c.exit = processExit(c.cmd.ProcessState, waitErr)
		c.cleanup()
	})
	return c.exit
}

func processExit(state *os.ProcessState, waitErr error) thread.Exit {
	if state != nil {
		if status, ok := state.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			return thread.Exit{Signal: status.Signal().String()}
		}
		code := state.ExitCode()
		if code >= 0 {
			return thread.Exit{Code: &code}
		}
	}
	// A process that could not be reaped is still an observed failure, not a
	// successful runtime. Exit's shape has no transport-error field, so retain
	// the conventional nonzero code while callers surface launch errors at the
	// boundary that produced them.
	code := 1
	if waitErr == nil {
		code = 0
	}
	return thread.Exit{Code: &code}
}

func (n *Nono) verify() error {
	ctx, cancel := context.WithTimeout(context.Background(), n.profile.CanaryTimeout)
	defer cancel()

	state, err := makeCanaryState(n.profile)
	if err != nil {
		return err
	}
	defer state.close()

	copyPath, release, err := sealedCanary(n.profile.Kept)
	if err != nil {
		return err
	}
	defer release()
	input := thread.Invocation{
		Path:   copyPath,
		Args:   []string{canaryArgument},
		Env:    state.environment(),
		Output: &state.outputBuf,
	}
	child, err := n.start(ctx, input, canaryLaunch)
	if err != nil {
		return err
	}

	done := make(chan thread.Exit, 1)
	go func() { done <- child.Wait() }()
	select {
	case exit := <-done:
		if state.outputBuf.Truncated() {
			return errors.New("nono canary report exceeded its bound")
		}
		report, err := parseCanaryReport(state.output())
		if err != nil {
			detail := ""
			if diagnostic, ok := child.(interface{ diagnostic() string }); ok {
				detail = diagnostic.diagnostic()
			}
			return fmt.Errorf("nono canary did not complete (%s): %w: %s", describeExit(exit), err, detail)
		}
		if err := report.valid(n.profile.ReadOnly); err != nil {
			return err
		}
		if exit.Code == nil || *exit.Code != 0 {
			return fmt.Errorf("nono canary reported success but exited %s", describeExit(exit))
		}
		return nil
	case <-ctx.Done():
		if err := child.Kill(); err != nil {
			return fmt.Errorf("nono canary timed out: %w; kill canary: %v", ctx.Err(), err)
		}
		<-done
		return fmt.Errorf("nono canary timed out: %w", ctx.Err())
	}

}

func describeExit(exit thread.Exit) string {
	if exit.Code != nil {
		return fmt.Sprintf("code %d", *exit.Code)
	}
	if exit.Signal != "" {
		return "signal " + exit.Signal
	}
	return "an unknown exit"
}

// sealedCanary copies the configured kept executable somewhere the launching
// process cannot then change, and returns the copy with the release that
// removes it.
//
// Tool-sandbox correctly refuses an entrypoint whose parent is writable by the
// process launching it, so both the file and its directory are made immutable
// before nono sees them. Neither belongs to the repair profile.
func sealedCanary(kept string) (string, func(), error) {
	dir, err := os.MkdirTemp("", "kept-nono-canary-helper-")
	if err != nil {
		return "", nil, fmt.Errorf("create nono canary helper state: %w", err)
	}
	release := func() {
		_ = os.Chmod(dir, 0o700)
		_ = os.RemoveAll(dir)
	}
	copyPath, err := copyCanary(kept, dir)
	if err != nil {
		release()
		return "", nil, err
	}
	if err := os.Chmod(copyPath, 0o500); err != nil {
		release()
		return "", nil, fmt.Errorf("seal nono canary executable: %w", err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		release()
		return "", nil, fmt.Errorf("seal nono canary helper directory: %w", err)
	}
	return copyPath, release, nil
}

func copyCanary(source, dir string) (string, error) {
	input, err := os.Open(source)
	if err != nil {
		return "", fmt.Errorf("open kept canary executable: %w", err)
	}
	defer input.Close()
	copy := filepath.Join(dir, "canary")
	output, err := os.OpenFile(copy, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o700)
	if err != nil {
		return "", fmt.Errorf("create kept canary executable: %w", err)
	}
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		return "", fmt.Errorf("copy kept canary executable: %w", err)
	}
	if err := output.Close(); err != nil {
		return "", fmt.Errorf("close kept canary executable: %w", err)
	}
	return copy, nil
}

type canaryState struct {
	dir         string
	credential  string
	outside     string
	hostTemp    string
	unix        net.Listener
	tcp         net.Listener
	push        canaryPushSentinel
	profile     canaryPolicyPaths
	outputBuf   canaryOutput
	createdKept bool
	// unwritable is the read-only box's declared probe targets, empty for a
	// box that grants a repair scope.
	unwritable []string
}

type canaryOutput struct{ output boundedBuffer }

func (o *canaryOutput) Write(src []byte) (int, error) { return o.output.Write(src) }
func (*canaryOutput) Close() error                    { return nil }
func (o *canaryOutput) Bytes() []byte                 { return o.output.Bytes() }
func (o *canaryOutput) Truncated() bool               { return o.output.Truncated() }
