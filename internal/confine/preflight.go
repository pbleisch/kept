//go:build !windows

package confine

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// The pre-flight asks the provider, before a process exists, whether the
// profile kept just rendered actually grants everything kept declared.
//
// It is what makes resolution safe. kept no longer names a policy group and no
// longer carries a path list: it asks a toolchain where it lives and renders
// the answer. Every way that can be wrong — a toolchain in a directory the
// provider reaches through nothing, a shim whose own directory is ungranted, a
// grant the profile's own denials take back, a provider release that resolves
// its baseline differently — becomes a refusal naming the path, the operation
// and what asked for it. Without it each one arrives from inside a box as an
// obscure failure that reads like a repair that ran and did nothing.
//
// kept does not trust its own renderer here, deliberately. The provider accepts
// a path field it cannot expand and grants the literal string, reporting the
// profile valid; only asking it about a specific path catches that.

// grantRequest is one thing the profile declared, and why.
type grantRequest struct {
	path string
	op   grant
	why  string
}

// declaredGrants enumerates what the rendered profile has to permit.
//
// The search path is in it even though kept renders no grant for it: a
// directory the child cannot read is a directory it cannot execute from, and
// "the toolchain is not on the declared search path" has to be a refusal before
// dispatch rather than an `env: go: No such file or directory` from inside a
// box.
func declaredGrants(profile Profile) []grantRequest {
	requests := make([]grantRequest, 0, len(profile.Read)+len(profile.Write)+len(profile.ToolRead)+len(profile.ToolWrite)+len(profile.ToolPath)+2)
	for _, path := range profile.Read {
		why := "the repair's readable tree"
		if path == profile.Box {
			why = "the box root, which is the confined working directory"
		}
		requests = append(requests, grantRequest{path: path, op: readable, why: why})
	}
	for _, path := range profile.Write {
		requests = append(requests, grantRequest{path: path, op: writable, why: "the promise's repair scope"})
	}
	for _, path := range profile.ToolRead {
		requests = append(requests, grantRequest{path: path, op: readable, why: "a resolved toolchain path"})
	}
	for _, path := range profile.ToolWrite {
		requests = append(requests, grantRequest{path: path, op: writable, why: "a resolved toolchain cache"})
	}
	requests = append(requests, grantRequest{path: profile.Temp, op: writable, why: "the box's own temporary directory"})
	if profile.Home != "" {
		requests = append(requests, grantRequest{path: profile.Home, op: writable, why: "the confined child's home directory"})
	}
	for _, path := range profile.ToolPath {
		requests = append(requests, grantRequest{path: path, op: readable, why: "a directory on the confined child's search path"})
	}
	return requests
}

// preflight refuses the launch when the provider does not grant something the
// profile declared. It is bounded by the same deadline as the canary, because
// it is the other half of verifying a provider rather than trusting it.
func (n *Nono) preflight(ctx context.Context, profilePath string) error {
	ctx, cancel := context.WithTimeout(ctx, n.profile.CanaryTimeout)
	defer cancel()
	for _, request := range declaredGrants(n.profile) {
		answer, err := askProvider(ctx, n.profile.Nono, profilePath, request)
		if err != nil {
			return err
		}
		if answer.Status != providerAllowed {
			return fmt.Errorf("nono confinement pre-flight refused the launch: the provider does not grant %s access to %s, which is %s (%s: %s)",
				request.op.operation(), request.path, request.why, answer.Reason, answer.Details)
		}
	}
	return nil
}

const providerAllowed = "allowed"

// providerAnswer is the part of `nono why --json` kept reads. The provider
// answers with exit status 0 whether it allows or denies, so the status field
// is the answer and the exit code is only whether the question was put.
type providerAnswer struct {
	Status  string `json:"status"`
	Reason  string `json:"reason"`
	Details string `json:"details"`
}

func askProvider(ctx context.Context, nono, profilePath string, request grantRequest) (providerAnswer, error) {
	command := exec.CommandContext(ctx, nono, "why", "--json",
		"--profile", profilePath, "--path", request.path, "--op", request.op.operation())
	command.WaitDelay = externalWaitDelay
	output, err := command.Output()
	if ctx.Err() != nil {
		return providerAnswer{}, fmt.Errorf("nono confinement pre-flight timed out asking about %s: %w", request.path, ctx.Err())
	}
	if err != nil {
		return providerAnswer{}, fmt.Errorf("nono confinement pre-flight could not ask about %s: %w: %s", request.path, err, providerDiagnostic(err))
	}
	var answer providerAnswer
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(output))), &answer); err != nil {
		return providerAnswer{}, fmt.Errorf("nono confinement pre-flight could not read the provider's answer about %s: %w: %s", request.path, err, string(output))
	}
	if answer.Status == "" {
		return providerAnswer{}, fmt.Errorf("nono confinement pre-flight answer about %s carries no status: %s", request.path, string(output))
	}
	return answer, nil
}

// operation is the provider's name for what a grant permits.
func (g grant) operation() string {
	if g == writable {
		return "write"
	}
	return "read"
}
