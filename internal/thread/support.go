package thread

import (
	"slices"
	"strconv"
	"strings"
)

// A runtime version constraint stands for two properties, and they do not fail
// the same way. Reading them as one number is what made an exact pin look like
// a stronger guarantee than it is.
//
// The **direct stream shape** — the pinned argv and the stream-json vocabulary
// an adapter projects — fails *loudly*. A flag the runtime no longer parses
// exits before a session exists, and an event language that moved produces a
// receipt with no projections. Nothing silently means something else.
//
// The **documented credential precedence** — the family of parent environment
// names that would override, redirect or switch the credential — fails
// *silently*. A name kept does not refuse hands the child a different
// credential, or the same one aimed somewhere else, and the session looks
// exactly like one that worked.
//
// Measured, an exact pin did not deliver the second property. Every
// credential-bearing name claudeRefusedEnvironment omits — the websocket auth
// file descriptor, the background-session auth trio, the artifact and
// messaging tokens — is read by 2.1.241 as well, the very build the pin was
// taken against. The list was assembled by reading two of the runtime's own
// enumerations, not by exhausting its environment surface, and pinning the
// version it was read from did not make it exhaustive. What the pin actually
// records is that a person looked at this build. That is a review, and a
// review is worth having; it is just not a measurement, and the constraint
// should say which one it is.
//
// So support is written as a review record with a compatible line beside it.
// reviewed is what somebody examined. line bounds how far a version may be
// from one of those and still be dispatched at all.
type support struct {
	// product is the name an adapter version string is built under, and wire
	// is the protocol half of it. Neither moves with the runtime's version.
	product string
	wire    string

	// line is the major.minor a version must be on to be dispatchable under
	// any policy. It is a proxy and is not defended as more than one: its job
	// is to bound the distance from a reviewed build, not to certify a version
	// nobody read.
	//
	// Empty means there is no such neighbourhood and reviewed is the whole of
	// what this adapter accepts. That is not the mechanism going unused — it is
	// an adapter saying its evidence supports the builds somebody read and
	// nothing beside them, which is the honest answer for an experimental wire
	// contract or for a runtime whose proof cannot cheaply be re-run.
	line string

	// reviewed is every version a person examined for the two properties
	// above, ascending. Membership is earned against the installed binary, and
	// what was checked belongs in a comment beside the entry.
	//
	// It is not decision 0052's admission bar, which is a live confined
	// interrupted session and is held per adapter by claudeAdmitted and its
	// siblings. That gate asks whether this adapter may start this runtime at
	// all; this one asks whether this build of it is the one that was read.
	reviewed []string
}

// VersionPolicy is what an operator accepts as a runtime version. The set is
// closed and there is deliberately no member that accepts anything: a version
// off the compatible line is refused under every policy, because a runtime
// whose stream shape or credential precedence this adapter does not understand
// must still refuse.
type VersionPolicy string

const (
	// ReviewedVersions dispatches only on a version somebody examined. It is
	// what an unset policy means.
	ReviewedVersions VersionPolicy = "reviewed"

	// LineVersions also dispatches on an unreviewed version of the same line
	// that is no older than the oldest reviewed one, and records the session as
	// unreviewed. It is the operator's statement that they would rather run a
	// patch release nobody has read than not repair, and the recording is what
	// keeps that from being a silent acceptance: every native event the session
	// writes is stamped unreviewed, so its evidence cannot later be read as
	// having come from a build that was checked.
	LineVersions VersionPolicy = "line"
)

// resolve reads the policy an operator configured. An unset policy is the
// strict one rather than a refusal, and the distinction from ClaudeAsk — whose
// zero value *is* refused — is which direction the mistake runs. An adapter
// that launched with no ask produced a silently worse session that looked like
// a good one; a dispatch that falls back to reviewed refuses, loudly, and the
// operator is told which policy would have admitted the version.
func (p VersionPolicy) resolve() (VersionPolicy, error) {
	switch p {
	case "", ReviewedVersions:
		return ReviewedVersions, nil
	case LineVersions:
		return LineVersions, nil
	}
	return "", &Refusal{Reason: "runtime version policy is unknown", Detail: string(p)}
}

// admit reports whether observed may be dispatched, and whether it is a version
// somebody reviewed. A refusal names the version, what is wrong with it, and —
// where one exists — the policy that would accept it.
func (s support) admit(runtime Runtime, observed string, policy VersionPolicy) (bool, error) {
	resolved, err := policy.resolve()
	if err != nil {
		return false, err
	}
	if observed == "" {
		return false, &Refusal{Reason: "runtime version is unset", Detail: string(runtime)}
	}
	if slices.Contains(s.reviewed, observed) {
		return true, nil
	}
	parsed, ok := parseVersion(observed)
	if !ok {
		// A version kept cannot order is not a version kept can place against a
		// line or a floor, and guessing that it is newer is the unsafe guess.
		return false, &Refusal{
			Reason: "runtime version is unreadable",
			Detail: string(runtime) + " reported " + observed + ", and a version is three numbers",
		}
	}
	if s.line == "" {
		return false, &Refusal{
			Reason: "runtime version is unsupported",
			Detail: string(runtime) + " " + observed + " is not one of the reviewed " + strings.Join(s.reviewed, ", "),
		}
	}
	if parsed.line() != s.line {
		return false, &Refusal{
			Reason: "runtime version is unsupported",
			Detail: string(runtime) + " " + observed + " is not on the " + s.line + " line",
		}
	}
	if oldest, ok := s.oldestReviewed(); ok && parsed.before(oldest) {
		// Older than anything anybody read. A review looks at the build in
		// front of it and says nothing about what came before, so an earlier
		// version is outside it in the direction no policy widens.
		return false, &Refusal{
			Reason: "runtime version is unsupported",
			Detail: string(runtime) + " " + observed + " is older than the reviewed " + s.reviewed[0],
		}
	}
	if resolved != LineVersions {
		return false, &Refusal{
			Reason: "runtime version is unreviewed",
			Detail: string(runtime) + " " + observed + " is on the " + s.line +
				" line and nobody has read it; " + policyConfigKey + ": " + string(LineVersions) +
				" dispatches on it and records the session as unreviewed",
		}
	}
	return false, nil
}

// policyConfigKey is where an operator states the policy, named here so a
// refusal tells them where to go. It is the operator's own config rather than
// the tracked one, for decision 0048's reason: the installed runtime is a fact
// about one machine, and a risk posture committed to the repository would be
// one person's answer imposed on every clone.
const policyConfigKey = "repair.runtime_versions"

// adapterVersion is the string every native event in a receipt is stamped
// with. It carries the version that actually ran rather than the one this
// build was written against — under an exact pin those were forced equal, and
// the moment a policy admits a range a constant would make every event claim a
// session that did not happen.
func (s support) adapterVersion(observed string, reviewed bool) string {
	version := observed
	if !reviewed {
		version += unreviewedMark
	}
	return s.product + ":" + version + "/" + s.wire
}

// stamp is adapterVersion for a configuration, and it asks admit rather than
// deciding reviewedness a second way. Two computations of that answer would
// fail in the direction that matters: a dispatch admitted as unreviewed and
// stamped as reviewed writes exactly the evidence this design exists to
// prevent.
func (s support) stamp(observed string, policy VersionPolicy) string {
	reviewed, _ := s.admit("", observed, policy)
	return s.adapterVersion(observed, reviewed)
}

// unreviewedMark is semver's build-metadata separator, which reads as a note
// about the build rather than as part of its number, and collides with neither
// separator already structuring an adapter version string.
const unreviewedMark = "+unreviewed"

// newestReviewed is the version a caller means when it wants one this build
// accepts without qualification — a test constructing a configuration, or a
// message naming what to install.
func (s support) newestReviewed() string {
	if len(s.reviewed) == 0 {
		return ""
	}
	return s.reviewed[len(s.reviewed)-1]
}

func (s support) supports(observed string) bool {
	return slices.Contains(s.reviewed, observed)
}

func (s support) oldestReviewed() (version, bool) {
	if len(s.reviewed) == 0 {
		return version{}, false
	}
	return parseVersion(s.reviewed[0])
}

// version is three numbers. Nothing here reads a pre-release or build suffix:
// all three runtimes report a bare triple, and a shape kept has not seen is
// refused by name rather than parsed on a guess.
type version [3]int

func parseVersion(text string) (version, bool) {
	fields := strings.Split(text, ".")
	if len(fields) != 3 {
		return version{}, false
	}
	var parsed version
	for i, field := range fields {
		if field == "" {
			return version{}, false
		}
		number, err := strconv.Atoi(field)
		if err != nil || number < 0 {
			return version{}, false
		}
		parsed[i] = number
	}
	return parsed, true
}

func (v version) line() string { return strconv.Itoa(v[0]) + "." + strconv.Itoa(v[1]) }

func (v version) before(other version) bool {
	for i := range v {
		if v[i] != other[i] {
			return v[i] < other[i]
		}
	}
	return false
}
