package thread

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// claudeSupport is the Claude Code versions whose direct stream shape and
// documented credential precedence this adapter understands. A version off the
// line is a refusal, not a best-effort invocation whose evidence might mean
// something else; a version on it that nobody has read is a refusal too, unless
// the operator has said otherwise, and then it is recorded as unreviewed. See
// support for why the constraint is a review record rather than a pin.
//
// Membership is earned against the installed binary rather than by assumption.
//
// 2.1.241 replaced 2.1.240 on the checks the entry was written with: every name
// in the refused family below still present, `setup-token` still taking no
// options, `auth` still having no revoke, and the direct argv still resolving —
// with one correction that pin never measured, recorded on claudeArgs.
//
// 2.1.251 was read the same way and once further, because a second reviewed
// entry makes a version-to-version diff possible where a single pin did not.
// All ten elements of claudeArgs are present and listed in its own `--help`;
// every token claudeStream projects — `stream_event`, `content_block_delta`,
// `text_delta`, `assistant`, `result`, `total_cost_usd`, `num_turns` — is
// present in both builds; and its `process.env` surface differs from 2.1.241 by
// seven names added and thirty-nine removed, of which the only credential-shaped
// addition is CLAUDE_CODE_FORCE_WINDOWS_CREDMAN, a Windows credential store on a
// platform decision 0053 does not confine a repair on. No live confined session
// has run on it: that is decision 0052's admission bar, it is held separately by
// claudeAdmitted, and it is not what this list is.
//
// 2.1.258 was read the same way, and its diff against 2.1.251 is the whole
// reason the entry exists rather than being taken on trust. All ten elements of
// claudeArgs are in its own `--help`, and so are `--model` and `--effort`, which
// docs/decision/0081 needs and which 2.1.251 also carries. Every token
// claudeStream projects is present. Its `process.env` surface differs from
// 2.1.251 by **two names added and seven removed**, and the added pair is the
// finding: CLAUDE_CODE_SESSION_KIND is not credential-shaped, and
// CLAUDE_CODE_GATEWAY_TOKEN_FILE_DESCRIPTOR is — a file descriptor carrying a
// gateway token, exactly the shape CLAUDE_CODE_API_KEY_FILE_DESCRIPTOR already
// has — so it is refused below, and the review is what found it. Three of the
// seven removed are the credential-bearing names AGENTS.md records as knowingly
// omitted (CLAUDE_CODE_ARTIFACTS_API_TOKEN, CLAUDE_CODE_SLACK_TAG_TOKEN,
// CLAUDE_CODE_WEBSOCKET_AUTH_FILE_DESCRIPTOR): this build no longer reads them.
var claudeSupport = support{
	product:  "claude-code",
	wire:     "stream-json:1",
	line:     "2.1",
	reviewed: []string{"2.1.241", "2.1.251", "2.1.258"},
}

// ClaudeVersion is the newest Claude Code version this adapter has reviewed. It
// is what to install and what a caller states when it wants a version accepted
// under every policy; it is not the only one accepted, which is what
// ClaudeSupports answers.
var ClaudeVersion = claudeSupport.newestReviewed()

// ClaudeSupports reports whether an installed Claude Code version is one this
// adapter has reviewed. A caller deciding whether the binary in front of it can
// be used asks this rather than comparing against ClaudeVersion, which would
// refuse every reviewed version but the newest.
func ClaudeSupports(version string) bool { return claudeSupport.supports(version) }

// ClaudeReviewedVersions is every Claude Code version this adapter has
// reviewed, oldest first. A caller exercising the set rather than one constant
// covers an entry the day it is added.
func ClaudeReviewedVersions() []string { return slices.Clone(claudeSupport.reviewed) }

// claudeAdapterVersion is what a session on the newest reviewed version stamps
// its native events with. The value an actual dispatch records is composed from
// the version that ran — see support.adapterVersion — and this is that string
// for the unqualified case.
var claudeAdapterVersion = claudeSupport.adapterVersion(ClaudeVersion, true)

const (
	// Admitted, on the evidence in internal/repair/claudeadmission_test.go.
	//
	// Decision 0052's table asks for a real confined session receiving the
	// packet marker, transcript, cost and turn projections referencing native
	// Claude events, and a kill after multiple updates that retains the partial
	// receipt, the timeout cause and the reaped exit. Pi's precedent asks for
	// one thing more, which is that the session actually repair a promise with
	// the verdict re-derived from the box rather than taken from what the
	// session said. All of it now holds, measured with the operator's own
	// brokered credential against api.anthropic.com in boxes cut from this
	// corpus with a promise broken in them.
	//
	// It did not hold before decision 0058, and the difference is worth keeping
	// here because it is the thing most likely to be undone by accident. Eight
	// earlier sessions launched, replayed the packet and were killed and reaped
	// exactly as the table requires, and then did not repair anything: they read
	// the packet, located the fault precisely, wrote a correct account of it and
	// stopped. A packet states authority and evidence and never an ask, and this
	// runtime reads one as a question unless something else says otherwise. The
	// ask is that something else, it travels beside the packet rather than in
	// it, and removing it puts this flag back where it was.
	claudeAdmitted = true
)

const (
	// claudeCredentialEnvironment is the one named child environment value
	// decision 0052 admits for this adapter: the long-lived subscription token
	// `claude setup-token` mints. Decision 0017 accepts the exposure it carries
	// — a tool-capable agent in the box can read its own credential — because
	// the alternative on macOS is a keychain grant, which that decision refuses
	// outright. The brokered route 0052 also permits is a separate channel and
	// is not built here.
	claudeCredentialEnvironment = "CLAUDE_CODE_OAUTH_TOKEN"

	// claudeAPIKeyEnvironment is never written by this adapter. It is named
	// only so its presence can be refused: Claude Code prefers an API key over
	// the subscription token, so one reaching this child would silently bill at
	// API rates instead of drawing on the subscription that was configured. It
	// is one member of the family below rather than a special case.
	claudeAPIKeyEnvironment = "ANTHROPIC_API_KEY"

	// claudeConfigEnvironment relocates every Claude configuration read into
	// the session's own empty directory, so nothing the operator authored is
	// reachable from the box.
	claudeConfigEnvironment = "CLAUDE_CONFIG_DIR"

	// claudeTempEnvironment is where Claude Code puts its scratch tier, and it
	// is not TMPDIR. `gme()` in the installed binary reads CLAUDE_CODE_TMPDIR
	// and otherwise returns the string "/tmp" — TMPDIR is never consulted — so
	// a confined session with this unset opens /tmp/claude-<uid>, which a
	// repair profile denies by name because it is a host scratch root. Measured
	// from inside a real box: `EPERM: operation not permitted, open
	// '/tmp/claude-501'`, exit 1, and not one stream event. The binary's own
	// guard even names the remedy — "Set CLAUDE_CODE_TMPDIR to a directory you
	// control". The value is the box's own temporary directory, which the
	// confinement profile grants and declares in one field. See 0056.
	claudeTempEnvironment = "CLAUDE_CODE_TMPDIR"
)

// The three names above, exported for the one caller outside this package that
// has to agree with them: a dispatcher renders a confinement profile that
// grants the scratch directory, publishes the credential phantom and allows the
// configuration directory, and those are the same facts as the child
// environment this adapter builds. Two spellings of one fact is the failure
// every one of them was written to prevent, so there is one spelling and it
// lives here. The values are not exported; nothing outside needs a credential.
const (
	ClaudeCredentialEnvironment = claudeCredentialEnvironment
	ClaudeConfigEnvironment     = claudeConfigEnvironment
	ClaudeTempEnvironment       = claudeTempEnvironment
)

// The hazard a refused parent variable represents. It is the Refusal's detail,
// so an operator is told what the name would have done rather than only that it
// was unwelcome.
const (
	claudeOverridesCredential = "overrides or supplies the credential"
	claudeCarriesAuthHelper   = "names configuration that can carry an auth helper"
	claudeRedirectsUpstream   = "redirects where the credential is sent"
	claudeSwitchesProvider    = "switches to a provider with its own credentials"
)

// claudeRefusedNamespace is refused whole, every name under it, rather than by
// enumeration. It is the vendor's provider-configuration namespace and it holds
// the entire first-party credential surface — ANTHROPIC_API_KEY and
// ANTHROPIC_AUTH_TOKEN, ANTHROPIC_BASE_URL and ANTHROPIC_API_HOST,
// ANTHROPIC_CUSTOM_HEADERS, ANTHROPIC_UNIX_SOCKET, ANTHROPIC_IDENTITY_TOKEN and
// its _FILE, the ANTHROPIC_FEDERATION_RULE_ID / ANTHROPIC_ORGANIZATION_ID
// workload-identity pair, ANTHROPIC_CONFIG_DIR, and every third-party base URL
// and key. Enumerating that is a list that rots on the next release; refusing
// the namespace covers a name Anthropic has not shipped yet.
//
// It is safe to refuse wholesale because it is configuration rather than
// ambient session state: a live Claude Code session exports none of it, so this
// cannot fire merely because kept was started from one. The CLAUDE_ family
// below is the opposite — a running session exports a dozen of those names —
// which is why that half is enumerated and this half is not.
const claudeRefusedNamespace = "ANTHROPIC_"

// claudeRefusedEnvironment is every remaining parent variable that can override
// the subscription credential, hand Claude a different one, redirect where it
// is sent, or switch it onto a provider billed some other way. The membership
// is read off the installed 2.1.241 binary rather than guessed: its own
// auth-state report enumerates ANTHROPIC_API_KEY, ANTHROPIC_AUTH_TOKEN,
// apiKeyHelper, CLAUDE_CODE_API_KEY_FILE_DESCRIPTOR, CLAUDE_CODE_OAUTH_TOKEN
// and ANTHROPIC_UNIX_SOCKET as the sources it considers, and its
// strictPolicyHelperKeys set names apiKeyHelper, proxyAuthHelper,
// awsCredentialExport, awsAuthRefresh, gcpAuthRefresh and processWrapper as
// settings that resolve a credential by running a program.
//
// Two exclusions are deliberate, because a guard that fires on an ordinary
// machine is a guard somebody turns off. A third-party credential store —
// AWS_*, AZURE_*, GOOGLE_APPLICATION_CREDENTIALS — does nothing until one of
// the provider switches below is on, so the switch is refused and the store is
// not. And NO_PROXY only excludes hosts from proxying; it cannot send the
// credential anywhere, so it is not a redirect.
var claudeRefusedEnvironment = map[string]string{
	// Carries a credential, or says where to read one.
	"CLAUDE_CODE_API_KEY_FILE_DESCRIPTOR": claudeOverridesCredential,
	// New in 2.1.258, found by diffing its environment surface against
	// 2.1.251's. Same shape as the descriptor above and refused for the same
	// reason: a file descriptor is a credential a parent hands across without
	// it ever appearing in a value kept could see.
	"CLAUDE_CODE_GATEWAY_TOKEN_FILE_DESCRIPTOR": claudeOverridesCredential,
	"CLAUDE_CODE_OAUTH_TOKEN_FILE_DESCRIPTOR":   claudeOverridesCredential,
	"CLAUDE_CODE_OAUTH_REFRESH_TOKEN":           claudeOverridesCredential,
	"CLAUDE_CODE_HOST_AUTH_ENV_VAR":             claudeOverridesCredential,
	"CLAUDE_CODE_HOST_CREDS_FILE":               claudeOverridesCredential,
	"CLAUDE_API_KEY":                            claudeOverridesCredential,
	"CLAUDE_BRIDGE_OAUTH_TOKEN":                 claudeOverridesCredential,
	"CLAUDE_TRUSTED_DEVICE_TOKEN":               claudeOverridesCredential,
	"CLAUDE_SESSION_INGRESS_TOKEN_FILE":         claudeOverridesCredential,
	"CLAUDE_BG_AUTH_SNAPSHOT_PATH":              claudeOverridesCredential,
	"CLAUDE_BG_SOCKET_TOKENS_PATH":              claudeOverridesCredential,
	"CCR_OAUTH_TOKEN_FILE":                      claudeOverridesCredential,
	// A git credential-helper password rather than an inference credential:
	// the binary builds a helper script around it. 0017's rule that no
	// credential in the box reaches a remote is what refuses this one.
	"CLAUDE_CODE_SESSION_ACCESS_TOKEN": claudeOverridesCredential,

	// Names settings or an environment file kept did not write. --safe-mode
	// disables customization but says admin-managed policy settings still
	// apply, and a policy settings file is exactly where apiKeyHelper and
	// proxyAuthHelper live — so this is the route --safe-mode leaves open.
	"CLAUDE_CODE_MANAGED_SETTINGS_PATH": claudeCarriesAuthHelper,
	"CLAUDE_CODE_REMOTE_SETTINGS_PATH":  claudeCarriesAuthHelper,
	"CLAUDE_SECURESTORAGE_CONFIG_DIR":   claudeCarriesAuthHelper,
	"CLAUDE_ENV_FILE":                   claudeCarriesAuthHelper,
	// kept sets this itself; an inherited one must not be left to win a merge.
	claudeConfigEnvironment: claudeCarriesAuthHelper,

	// Sends the credential somewhere other than the configured upstream.
	"CLAUDE_CODE_API_BASE_URL":             claudeRedirectsUpstream,
	"CLAUDE_CODE_CUSTOM_OAUTH_URL":         claudeRedirectsUpstream,
	"CLAUDE_CODE_PROXY_URL":                claudeRedirectsUpstream,
	"CLAUDE_CODE_PROXY_HOST":               claudeRedirectsUpstream,
	"CLAUDE_CODE_HTTP_PROXY":               claudeRedirectsUpstream,
	"CLAUDE_CODE_HTTPS_PROXY":              claudeRedirectsUpstream,
	"CLAUDE_CODE_ENABLE_PROXY_AUTH_HELPER": claudeRedirectsUpstream,
	"CLAUDE_AI_AUTHORIZE_URL":              claudeRedirectsUpstream,
	"CLAUDE_LOCAL_OAUTH_API_BASE":          claudeRedirectsUpstream,
	"CLAUDE_BRIDGE_BASE_URL":               claudeRedirectsUpstream,
	"HTTP_PROXY":                           claudeRedirectsUpstream,
	"HTTPS_PROXY":                          claudeRedirectsUpstream,
	"ALL_PROXY":                            claudeRedirectsUpstream,
	"http_proxy":                           claudeRedirectsUpstream,
	"https_proxy":                          claudeRedirectsUpstream,
	"all_proxy":                            claudeRedirectsUpstream,
	// Trusting an extra certificate authority is what turns a redirect into an
	// interception the child cannot tell from the real upstream.
	"NODE_EXTRA_CA_CERTS": claudeRedirectsUpstream,

	// Authenticates and bills somewhere other than the operator's
	// subscription, which is the whole reason this channel was chosen.
	"CLAUDE_CODE_USE_BEDROCK":                claudeSwitchesProvider,
	"CLAUDE_CODE_USE_VERTEX":                 claudeSwitchesProvider,
	"CLAUDE_CODE_USE_FOUNDRY":                claudeSwitchesProvider,
	"CLAUDE_CODE_USE_MANTLE":                 claudeSwitchesProvider,
	"CLAUDE_CODE_USE_GATEWAY":                claudeSwitchesProvider,
	"CLAUDE_CODE_USE_CCR_V2":                 claudeSwitchesProvider,
	"CLAUDE_CODE_USE_ANTHROPIC_AWS":          claudeSwitchesProvider,
	"CLAUDE_CODE_USE_ANTHROPIC_GOOGLE_CLOUD": claudeSwitchesProvider,
}

// claudeRefusedParentEnvironment reports the offending variable's name and what
// it would have done. It reads names only and never a value: the value is the
// secret, and a refusal that quoted one would put it in a disposition. Offenders
// are sorted so two hostile names produce the same refusal every run rather than
// whichever the environment happened to list first.
func claudeRefusedParentEnvironment(environment []string) (string, string, bool) {
	var found []string
	for _, entry := range environment {
		name, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if strings.HasPrefix(name, claudeRefusedNamespace) {
			found = append(found, name)
			continue
		}
		if _, refused := claudeRefusedEnvironment[name]; refused {
			found = append(found, name)
		}
	}
	if len(found) == 0 {
		return "", "", false
	}
	sort.Strings(found)
	name := found[0]
	hazard, ok := claudeRefusedEnvironment[name]
	if !ok {
		hazard = claudeOverridesCredential
	}
	return name, hazard, true
}

// claudeSetupTokenMargin is how much life a setup token must have left before
// a repair may start on it. `claude setup-token` accepts no options, prints no
// expiry and has no revoke command, so the lifetime is neither selectable nor
// readable back, and the only way to learn that a token died is to spend a
// request against it — which is a live authenticated call the adapter must not
// make. So the operator states when it lapses and this margin decides how early
// the refusal lands. A day is longer than any repair deadline a caller can
// sensibly configure under 0026, so a dispatch that passes this check cannot
// reach the lapse while it is still running; and it costs a one-year token
// nothing that matters.
const claudeSetupTokenMargin = 24 * time.Hour

// claudeArgs is the documented noninteractive stream mode for Claude Code
// 2.1.240.
//
// --bare is deliberately absent and must not come back. Its own help says
// Anthropic auth under it is strictly ANTHROPIC_API_KEY or an apiKeyHelper and
// that OAuth is never read, so it cannot authenticate the subscription token
// this adapter now carries — the flag and the credential channel are mutually
// exclusive. --safe-mode replaces the half of --bare that was actually wanted:
// it disables CLAUDE.md, skills, plugins, hooks, MCP servers, custom commands,
// agents and the rest of the customization surface, so the packet over stdin
// stays the only instruction the session receives, while auth, built-in tools
// and permissions keep working.
//
// --bare also skipped keychain reads, and dropping it means the process may
// attempt one. That is confinement's answer and not an adapter flag's: 0017
// grants no keychain, ever, so the attempt is refused by the profile the canary
// verified — and a flag that could be forgotten was never the thing standing
// between the box and the operator's login. The injected token takes precedence
// over a stored credential in any case.
//
// --include-partial-messages is what makes the transcript incremental rather
// than a completion-only summary, which 0052 refuses as evidence. Permission
// prompts are bypassed only inside the already verified external confinement
// provider; no adapter creates or relaxes that provider.
//
// --verbose is not decoration and is not optional: `--print` with
// `--output-format stream-json` refuses to start without it. Measured, because
// the flag list this adapter was first written from came out of `--help` and
// was never launched — a confined session exited 1 with `Error: When using
// --print, --output-format=stream-json requires --verbose` and no session at
// all. A pinned argv that cannot start is a fixture, not an invocation.
//
// And --safe-mode is load-bearing in a repair box rather than merely tidy. The
// tree under repair is somebody's repository and may carry its own
// .claude/settings.json; measured both ways in a real box, a SessionStart hook
// planted there runs without the flag and does not run with it. Confinement
// still bounds what such a command could touch — 0017 — but the flag is what
// keeps the tree being repaired from choosing what runs at all. What it does
// not disable is the runtime's own built-in skills, which are in the image:
// measured identical with and without it, and named here because the flag's own
// help says otherwise.
//
// --replay-user-messages is what makes the packet's arrival evidence rather
// than inference. The runtime re-emits the user message it read from stdin as a
// native `user` event carrying `isReplay`, so the receipt holds the runtime's
// own account of the bytes it received — measured in a confined box. Without
// it, "the packet arrived" can only be argued from a marker turning up in an
// assistant turn, and the session decision 0052 cares most about is the one
// killed before any assistant turn exists. The packet is already stored as its
// own artifact before launch, so this is a second, independent record of the
// same bytes and never the first.
//
// This is not the whole command line. The declared ask of decision 0058 is
// appended to it — see claudeAskArgs — so that the packet stays the whole of
// the input channel and the framing travels somewhere else. A reader comparing
// this list against a running process will find two more elements there.
var claudeArgs = []string{
	"--safe-mode",
	"--print",
	"--verbose",
	"--input-format", "stream-json",
	"--output-format", "stream-json",
	"--include-partial-messages",
	"--replay-user-messages",
	"--no-session-persistence",
	"--permission-mode", "bypassPermissions",
}

// ClaudeAsk is what this session is being asked to do with the packet it
// receives. It is a declared input rather than a property of the runtime,
// because the framing a runtime needs is a property of the pair: this runtime,
// doing this kind of work. Decision 0058.
//
// The set is closed and it has one member today. A second kind of work is a
// second value and a second arm below — never a template, a registry or an
// operator-supplied string, because a channel that can carry anything can carry
// the instruction decision 0009 keeps out of a packet.
type ClaudeAsk string

// ClaudeRepairAsk is the ask a confined repair session is given: the packet in
// front of you is work rather than a question. It names no promise, no file and
// no evidence — that is the packet's, and this is the same sentence for every
// repair there will ever be.
const ClaudeRepairAsk ClaudeAsk = "repair"

// ClaudeAssessAsk is the ask a confined adversarial pass is given: the brief in
// front of you is a repair somebody already performed, and the work is to argue
// that it does not meet what the thread was for.
//
// It is the second member of the closed set, and a second constant rather than
// the first one parameterized. The two frame opposite work — one says *act on
// this gap*, the other says *attack this act* — and a template that could
// produce both could produce anything, which is the channel decision 0009 keeps
// an instruction out of.
const ClaudeAssessAsk ClaudeAsk = "assess"

// claudeRepairAskText is that ask, in the words the runtime is given.
//
// It exists because of a measurement rather than a preference. Eight confined
// sessions carrying a well-formed packet and a live credential located the
// fault exactly, wrote a correct account of why the promise did not hold, and
// then stopped without editing anything; the write boundary was not the cause,
// and carrying the packet's dispatch policies changed nothing. A packet states
// authority and evidence and never an ask — decision 0009 — and this runtime,
// launched with `--print`, reads one as a question. Pi's own harness supplies
// this framing; Claude Code's does not.
//
// Three things it deliberately does not do. It does not say how to repair
// anything, which would be the repair library decision 0009 refuses. It does
// not name what is broken, because that is the packet's and a second copy of it
// here is a second place the answer could live. And it does not travel in the
// packet or in front of it: claudeInput still puts exactly the packet's bytes in
// the user-message channel with nothing prepended.
//
// The same rule holds one step out and is easy to break: an ask is
// byte-identical for every thread, so it may not assert anything that is only
// true of some of them. A pass runs over a repair that failed and over one that
// reached outside its scope as readily as over one that held, so this one says
// the verdict is settled elsewhere rather than saying what it settled.
const claudeRepairAskText = `You are a repair agent. This session runs unattended, in a disposable copy of a repository, and the message you have been given is a repair packet rather than a question put to you.

A packet is a record: the promise that is not kept, the ratified authority for acting on it, the measured observation that found the gap, and the paths a repair may change. It deliberately does not say what to do about it. Deciding that is the work, and nothing else is going to tell you.

So act. Read whatever you need, change the tree, and stop when the promise holds. There is nobody here to approve a plan or to answer a question, and what you say about the fault is evidence for a later reviewer rather than the thing being asked for: whether the promise is kept is settled by measuring it again in this copy after you exit. An accurate account that leaves the tree unchanged is a failed repair.

Change nothing outside the repair scope the packet names.`

// claudeAssessAskText is the adversarial pass's ask, in the words the runtime
// is given.
//
// **It is not a question list and must not become one.** The failure this half
// exists to reach is the one nobody thought to ask about, and every question
// written here is a question the session would answer instead of looking — so
// what this says is what kind of work it is and what would make it worthless,
// and it stops. The one example it gives is named as an example in the model
// document too, and it is deliberately not the whole of what the pass may say.
//
// Three things it does not do, each for a reason. It names no promise, no file
// and no evidence: those are the brief's, and a second copy here is a second
// place the answer could live. It does not say what to do about anything it
// finds, which would be enumerating the repair one level up from decision 0009.
// And it does not ask for a verdict on whether the promise is kept — that is
// settled by measuring the world, and a session's opinion about it is what
// policy:the-agents-report-is-never-an-input-to-the-verdict keeps out.
//
// It is framed as an attack rather than an evaluation because the framing
// decides the answer: an assessor asked whether criteria are met finds that they
// are. The cost is accepted and named — a session asked to attack will produce
// an argument whether or not there is one to make — and it is why nothing here
// grades what comes back, and why the record keeps the words rather than a
// verdict derived from them.
const claudeAssessAskText = `You are an adversarial assessor. This session runs unattended, in a read-only copy of a repository, and the message you have been given is the complete ledger entry for one repair that has already happened: what the repairer was authorized to do, what it changed, what the instrument measured before and after, what it said while it worked, and what a fixed set of derived checks already found.

Whether the promise is kept in the tree that session left has already been measured, and the brief carries both measurements. That question is settled by measuring the world and is not yours; so is whether the change stayed inside the paths the promise bound.

Your work is to make the case that the repair does not meet what the thread was for. Read the statement the promise makes, then read what was actually done, and argue the gap between them. Be specific and stay with the evidence: point at the line, the measurement or the sentence you are relying on, and say plainly when the evidence you would need is not here. An example of the kind of thing worth finding, and not the whole of it: a session that worked toward satisfying the predicate rather than toward the property the statement describes. There is no list of questions and nothing is out of scope — what is worth catching is what nobody thought to ask about, so look for that rather than working through what somebody did.

Change nothing. Nothing you write decides whether the promise is kept; that was measured, and your account is read by a person and by a later gate. Say what you found and stop. If you genuinely cannot make a case against this repair, say that instead of manufacturing one — an argument nobody can act on costs more than a page that says there is nothing here.`

// claudeAskText resolves the declared ask. The zero value is not a default: an
// adapter that quietly launched with no ask would be the unframed session this
// channel exists to stop being, and it would look exactly like a session that
// had one.
func claudeAskText(ask ClaudeAsk) (string, error) {
	switch ask {
	case ClaudeRepairAsk:
		return claudeRepairAskText, nil
	case ClaudeAssessAsk:
		return claudeAssessAskText, nil
	case "":
		return "", &Refusal{Reason: "claude ask is unset", Detail: string(Claude)}
	}
	return "", &Refusal{Reason: "claude ask is unknown", Detail: string(ask)}
}

// claudeAskArgs is how the ask reaches the runtime: appended to the pinned argv
// above as a system-prompt addition, which is a channel of its own and not the
// packet's.
//
// `--append-system-prompt` rather than `--system-prompt`, because replacing the
// default prompt would take the runtime's own tool and permission instructions
// with it, and those are what a repair uses. Measured against 2.1.241 with the
// pinned argv: the appended text is honoured under `--safe-mode` and `--print`
// with stream-json output, which the flag's own help does not say — --safe-mode
// disables what a session *discovers*, and an explicit argv value is not that.
func claudeAskArgs(ask ClaudeAsk) ([]string, error) {
	text, err := claudeAskText(ask)
	if err != nil {
		return nil, err
	}
	return []string{"--append-system-prompt", text}, nil
}

// ClaudeEfforts is the effort levels this runtime accepts, in the order its own
// help lists them. It is exported so an operator's configuration is checked
// against the runtime's set rather than against a second copy of it somewhere
// else, and it is a closed list rather than a pattern because a value the
// runtime does not parse exits before a session exists — which is a loud failure
// a person meets at the worst moment rather than at the moment they configured
// it.
//
// Read off the installed binary: `--effort <level>` in 2.1.251 and 2.1.258,
// documented as low, medium, high, xhigh, max.
func ClaudeEfforts() []string { return []string{"low", "medium", "high", "xhigh", "max"} }

// claudeSelectionArgs is which model answers and how hard it tries, appended to
// the pinned argv beside the ask.
//
// It is separate from claudeArgs because the two are different kinds of thing.
// claudeArgs is the launch shape this adapter is pinned to and every element of
// it was measured; these are the operator's selection, they vary between one
// session and another on one machine, and an empty one is the runtime's own
// default rather than a value kept chose.
//
// Both flags are in `--help` for 2.1.251 and 2.1.258. Neither is validated by
// asking the runtime: `--model` takes an alias or a full name and the set moves
// with the vendor rather than with this build, so a list here would be a second
// place the answer lived and it would be the stale one. What is checked is that
// the value cannot be read as something other than a value — see
// claudeSelectionWhy.
func claudeSelectionArgs(cfg RuntimeConfig) []string {
	var args []string
	if cfg.ClaudeModel != "" {
		args = append(args, "--model", cfg.ClaudeModel)
	}
	if cfg.ClaudeEffort != "" {
		args = append(args, "--effort", cfg.ClaudeEffort)
	}
	return args
}

// claudeSelectionRefused is every byte a model or effort value may not carry:
// whitespace, NUL, and the characters only a shell reads.
const claudeSelectionRefused = " \t\r\n\x00|&;<>()$`\\\"'*?[]{}~!#"

// claudeSelectionWhy refuses a model or an effort that would not be one argv
// value.
//
// A name beginning with a dash is read by the runtime's own parser as the next
// flag rather than as this one's value, so `--model --dangerously-skip-x` would
// be a flag kept never chose arriving through a configuration field. Whitespace
// and a shell metacharacter are refused for the reason decision 0038 refuses
// them in a `run:` line: there is no shell here, and a value that expects one is
// a value somebody expected to be interpreted.
func claudeSelectionWhy(cfg RuntimeConfig) string {
	for _, selection := range []struct{ flag, value string }{
		{"--model", cfg.ClaudeModel},
		{"--effort", cfg.ClaudeEffort},
	} {
		if selection.value == "" {
			continue
		}
		if strings.HasPrefix(selection.value, "-") {
			return selection.flag + " value begins with a dash, which the runtime reads as another flag"
		}
		if strings.ContainsAny(selection.value, claudeSelectionRefused) {
			return selection.flag + " value is not a single argv value"
		}
	}
	if cfg.ClaudeEffort != "" && !slices.Contains(ClaudeEfforts(), cfg.ClaudeEffort) {
		return "--effort value is not one of " + strings.Join(ClaudeEfforts(), ", ")
	}
	return ""
}

// claudeAskSuits holds the declared ask to the input it was declared for.
//
// The pairing is here rather than in the input types because it is this
// runtime's: an ask is a property of the pair — this runtime, doing this kind of
// work — and another adapter needing none is not a gap in this table.
func claudeAskSuits(ask ClaudeAsk, input Input) error {
	var want ClaudeAsk
	switch input.(type) {
	case Packet:
		want = ClaudeRepairAsk
	case Brief:
		want = ClaudeAssessAsk
	default:
		return &Refusal{Reason: "claude was given an input this adapter has no ask for", Detail: string(Claude)}
	}
	if ask != want {
		return &Refusal{
			Reason: "claude ask does not suit its input",
			Detail: "the ask is " + string(ask) + " and this input needs " + string(want),
		}
	}
	return nil
}

func checkClaudeConfig(cfg RuntimeConfig, packet Input) error {
	switch {
	case cfg.Runtime != string(Claude):
		return &Refusal{Reason: "runtime configuration does not name claude", Detail: cfg.Runtime}
	case cfg.Executable == "":
		return &Refusal{Reason: "runtime executable is unset", Detail: string(Claude)}
	case !filepath.IsAbs(cfg.Executable):
		return &Refusal{Reason: "runtime executable is not an explicit absolute path", Detail: cfg.Executable}
	}
	// The version constraint keeps its place in the order — after the runtime
	// is named and its executable resolved, before anything about the packet or
	// the credential — because what it refuses is the binary, and a caller
	// reading the refusal should not have to satisfy a credential first to
	// learn that the runtime is the wrong one.
	if _, err := claudeSupport.admit(Claude, cfg.Version, cfg.Versions); err != nil {
		return err
	}
	switch {
	case packet.Empty():
		return &Refusal{Reason: "runtime packet is absent", Detail: string(Claude)}
	case cfg.ClaudeBrokeredCredential && cfg.ClaudeSetupToken != "":
		// Two channels in force is one channel too many, and the failure is
		// silent in the direction that matters: the provider publishes the
		// phantom, this adapter would publish the real token, and whichever won
		// the merge would decide whether the box holds a credential. Decision
		// 0057 makes them exclusive here and the confinement launch refuses the
		// same collision from the other side.
		return &Refusal{Reason: "claude credential is both brokered and direct", Detail: string(Claude)}
	case !cfg.ClaudeBrokeredCredential && cfg.ClaudeSetupToken == "":
		return &Refusal{Reason: claudeCredentialEnvironment + " is unset", Detail: string(Claude)}
	case strings.IndexByte(cfg.ClaudeSetupToken, 0) >= 0:
		return &Refusal{Reason: claudeCredentialEnvironment + " contains NUL", Detail: string(Claude)}
	case cfg.ClaudeSetupToken != "" && packetContains(cfg.ClaudeSetupToken, packet.Bytes()):
		return &Refusal{Reason: claudeCredentialEnvironment + " appears in the packet", Detail: string(Claude)}
	case cfg.ClaudeHome == "":
		return &Refusal{Reason: "claude HOME is unset", Detail: string(Claude)}
	case !filepath.IsAbs(cfg.ClaudeHome):
		return &Refusal{Reason: "claude HOME is not an explicit absolute path", Detail: cfg.ClaudeHome}
	case cfg.ClaudeTemp == "":
		return &Refusal{Reason: claudeTempEnvironment + " is unset", Detail: string(Claude)}
	case !filepath.IsAbs(cfg.ClaudeTemp):
		return &Refusal{Reason: claudeTempEnvironment + " is not an explicit absolute path", Detail: cfg.ClaudeTemp}
	case cfg.ClaudeTemp == cfg.ClaudeHome:
		// Two directories discarded on different schedules and holding
		// different things. Collapsing them would put the session's scratch
		// tier inside the state directory whose emptiness is checked below.
		return &Refusal{Reason: claudeTempEnvironment + " is the session home", Detail: cfg.ClaudeTemp}
	}
	// The ask is resolved here rather than only where argv is built, so a
	// configuration naming no ask is refused by the same check that refuses a
	// missing credential rather than at launch.
	if _, err := claudeAskText(cfg.ClaudeAsk); err != nil {
		return err
	}
	// And the ask has to be the one this input is for. The two are one fact
	// stated twice — a packet is work and a brief is an act to argue about —
	// and a mismatch is the failure that looks most like success: a session
	// handed a brief and told to repair would read a record of somebody else's
	// work as a gap to close, in a box it cannot write to, and produce a
	// session that ran and did nothing anybody can account for. See
	// docs/decision/0058 and 0081.
	if err := claudeAskSuits(cfg.ClaudeAsk, packet); err != nil {
		return err
	}
	if why := claudeSelectionWhy(cfg); why != "" {
		return &Refusal{Reason: "claude runtime selection is not a value", Detail: why}
	}
	if err := checkClaudeSetupTokenLife(cfg.ClaudeSetupTokenExpiry, time.Now()); err != nil {
		return err
	}

	// The parent environment is read here for one purpose, which is to refuse.
	// Decision 0052 forbids reading it to *obtain* a credential and this never
	// does: no value is read, nothing is copied and nothing reaches the child.
	// Only names are consulted. Invocation.Env is the complete child
	// environment rather than an addition to this process's, so none of these
	// can reach Claude through this adapter — but a provider that merged
	// instead of replacing would hand it a credential it prefers, or an
	// upstream that is not the one configured, and that failure looks exactly
	// like success while billing or disclosing somewhere else.
	// An exported name is a configured intent whatever its value, so an empty
	// one is refused too rather than reasoned about.
	if name, hazard, refused := claudeRefusedParentEnvironment(os.Environ()); refused {
		return &Refusal{Reason: name + " is present in the parent environment", Detail: hazard}
	}

	info, err := os.Stat(cfg.Executable)
	if err != nil {
		return &Refusal{Reason: "runtime executable is unavailable", Detail: cfg.Executable}
	}
	if !info.Mode().IsRegular() || info.Mode()&0111 == 0 {
		return &Refusal{Reason: "runtime executable is not executable", Detail: cfg.Executable}
	}
	info, err = os.Stat(cfg.ClaudeHome)
	if err != nil {
		return &Refusal{Reason: "claude HOME is unavailable", Detail: cfg.ClaudeHome}
	}
	if !info.IsDir() {
		return &Refusal{Reason: "claude HOME is not a directory", Detail: cfg.ClaudeHome}
	}
	entries, err := os.ReadDir(cfg.ClaudeHome)
	if err != nil {
		return &Refusal{Reason: "claude HOME cannot be checked", Detail: cfg.ClaudeHome}
	}
	if len(entries) != 0 {
		return &Refusal{Reason: "claude HOME is not clean", Detail: cfg.ClaudeHome}
	}
	// The scratch directory is not required to be empty — it is the box's own
	// temporary directory and a toolchain may already have used it — but a
	// grant naming something that is not there is a boundary that is not one,
	// and the failure it produces arrives from inside a box as a runtime that
	// could not make its own scratch tier.
	temp, err := os.Stat(cfg.ClaudeTemp)
	if err != nil {
		return &Refusal{Reason: claudeTempEnvironment + " is unavailable", Detail: cfg.ClaudeTemp}
	}
	if !temp.IsDir() {
		return &Refusal{Reason: claudeTempEnvironment + " is not a directory", Detail: cfg.ClaudeTemp}
	}
	return nil
}

// checkClaudeSetupTokenLife refuses a dispatch that could outlive its
// credential. Lapsed and lapsing-soon are kept apart because they tell the
// operator different things: one says mint a replacement now, the other says
// there is still a window. Both refuse, and neither is discovered by asking
// the provider — no live call is made to find out.
func checkClaudeSetupTokenLife(expiry, now time.Time) error {
	switch {
	case expiry.IsZero():
		return &Refusal{Reason: "claude setup-token expiry is unset", Detail: string(Claude)}
	case !now.Before(expiry):
		return &Refusal{Reason: "claude setup-token has lapsed", Detail: expiry.Format(time.RFC3339)}
	case expiry.Sub(now) < claudeSetupTokenMargin:
		return &Refusal{Reason: "claude setup-token lapses inside the dispatch margin", Detail: expiry.Format(time.RFC3339)}
	}
	return nil
}

// prepareClaude constructs the admitted invocation. The child environment is
// built entirely from explicit configuration: nothing is copied from this
// process's environment, the operator's home, a keychain or an auth file. It is
// separate from claudeAdapter.Prepare so that the admission gate has one place
// to stand and this has none, which is what let the fixture exercise every safe
// path while the gate was shut and is what keeps the two from merging now that
// it is open.
func prepareClaude(cfg RuntimeConfig, packet Input, receipt AdapterReceipt) (Invocation, error) {
	if err := checkClaudeConfig(cfg, packet); err != nil {
		return Invocation{}, err
	}
	if receipt == nil {
		return Invocation{}, &Refusal{Reason: "thread adapter receipt is absent", Detail: string(Claude)}
	}
	input, err := claudeInput(packet)
	if err != nil {
		return Invocation{}, err
	}
	environment := []string{
		"HOME=" + cfg.ClaudeHome,
		claudeConfigEnvironment + "=" + filepath.Join(cfg.ClaudeHome, "config"),
		claudeTempEnvironment + "=" + cfg.ClaudeTemp,
	}
	// Under the brokered channel the credential variable is the profile's, and
	// naming it here is refused by the confinement launch. Under the direct one
	// it is this adapter's, and the value it carries is what the stream redacts.
	if !cfg.ClaudeBrokeredCredential {
		environment = append(environment, claudeCredentialEnvironment+"="+cfg.ClaudeSetupToken)
	}
	ask, err := claudeAskArgs(cfg.ClaudeAsk)
	if err != nil {
		return Invocation{}, err
	}
	return Invocation{
		Path:   cfg.Executable,
		Args:   append(append(append([]string(nil), claudeArgs...), claudeSelectionArgs(cfg)...), ask...),
		Input:  input,
		Env:    environment,
		Output: &claudeStream{receipt: receipt, secret: []byte(cfg.ClaudeSetupToken)},
		Redact: func(src []byte) ([]byte, Redaction) {
			redacted, redaction := redactClaude(src, []byte(cfg.ClaudeSetupToken))
			return redacted, redaction
		},
	}, nil
}

// claudeInput puts exactly one immutable packet byte sequence in Claude's
// documented stdin JSONL user-message channel. The packet is the text value;
// no prompt prefix or rebuilt corpus data can alter what Claude receives.
func claudeInput(packet Input) ([]byte, error) {
	wire := struct {
		Type    string `json:"type"`
		Message struct {
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"message"`
	}{Type: "user"}
	wire.Message.Role = "user"
	wire.Message.Content = []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}{{Type: "text", Text: string(packet.Bytes())}}
	encoded, err := json.Marshal(wire)
	if err != nil {
		return nil, fmt.Errorf("encode Claude stream input: %w", err)
	}
	return append(encoded, '\n'), nil
}

// claudeStream retains each native JSONL item before making the small common
// projections decision 0052 permits. It is intentionally an output sink, not
// a process handle: timeout, kill and reaping stay with Supervisor.
type claudeStream struct {
	receipt       AdapterReceipt
	secret        []byte
	pending       []byte
	turns         uint64
	sawStreamText bool
	closed        bool
}

func (s *claudeStream) Write(src []byte) (int, error) {
	if s.closed {
		return 0, fmt.Errorf("write Claude stream after close")
	}
	consumed := 0
	for len(src) > 0 {
		at := bytes.IndexByte(src, '\n')
		if at < 0 {
			if err := s.append(src); err != nil {
				return consumed, err
			}
			return consumed + len(src), nil
		}
		if err := s.append(src[:at]); err != nil {
			return consumed, err
		}
		consumed += at + 1
		line := s.pending
		s.pending = nil
		if err := s.record(line, false); err != nil {
			return consumed, err
		}
		src = src[at+1:]
	}
	return consumed, nil
}

// Close retains what the stream ended inside. A final item that decodes is an
// event whatever its line end, and one that does not is kept as undecodable and
// marked unterminated — which is what a runtime that exited before it finished
// writing leaves, and measured, is what Claude Code leaves whenever it exits with
// output still queued. See docs/decision/0084.
func (s *claudeStream) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	if len(bytes.TrimSpace(s.pending)) == 0 {
		return nil
	}
	if err := s.record(s.pending, true); err != nil {
		return err
	}
	s.pending = nil
	return nil
}

func (s *claudeStream) append(src []byte) error {
	s.pending = append(s.pending, src...)
	return nil
}

func (s *claudeStream) record(raw []byte, unterminated bool) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return s.undecodable(raw, fmt.Sprintf("decode Claude native stream event: %v", err), unterminated)
	}
	if envelope.Type == "" {
		return s.undecodable(raw, "Claude native stream event has no type", unterminated)
	}
	redacted, redaction := redactClaude(raw, s.secret)
	ref, err := s.receipt.Native(envelope.Type, redacted, redaction)
	if err != nil {
		return err
	}
	return s.project(envelope.Type, raw, ref)
}

// undecodable retains a line that is not an event, redacted exactly as an event
// would have been, and goes on to the next one. Nothing is projected from it.
func (s *claudeStream) undecodable(raw []byte, why string, unterminated bool) error {
	content, redaction := redactClaude(raw, s.secret)
	return s.receipt.Undecodable(content, why, unterminated, redaction)
}

func (s *claudeStream) project(typ string, raw []byte, ref string) error {
	switch typ {
	case "stream_event":
		text, ok := claudeDeltaText(raw)
		if !ok {
			return nil
		}
		s.sawStreamText = true
		content, redaction := redactClaude([]byte(text), s.secret)
		_, err := s.receipt.Transcript(ref, content, redaction)
		return err
	case "assistant":
		if !s.sawStreamText {
			for _, text := range claudeAssistantText(raw) {
				content, redaction := redactClaude([]byte(text), s.secret)
				if _, err := s.receipt.Transcript(ref, content, redaction); err != nil {
					return err
				}
			}
		}
		s.turns++
		return s.receipt.Turn(ref, s.turns)
	case "system":
		// The model the runtime says answered, from its own account of the
		// session it started. `result.modelUsage` names it a second time and is
		// retained as native evidence; projecting from both would put two
		// answers to one question in the receipt, and the init is the one that
		// exists whether or not the session reached a result.
		if model, ok := claudeInitModel(raw); ok {
			return s.receipt.Model(ref, model)
		}
	case "rate_limit_event":
		// The account's own usage windows, shared across every machine and
		// every session on it. This event kind is Claude's vocabulary and not a
		// contract: a build that renames it, or that reports a window kept
		// cannot read, projects nothing here and the receipt then says no
		// window was reported — which is the honest answer, and not the same as
		// a window at zero. See docs/decision/0077.
		if windows, ok := claudeWindows(raw); ok {
			return s.receipt.Window(ref, windows)
		}
	case "result":
		if amount, ok := claudeResultNumber(raw, "total_cost_usd"); ok {
			if err := s.receipt.Cost(ref, "USD", amount); err != nil {
				return err
			}
		}
		if count, ok := claudeResultTurns(raw); ok {
			return s.receipt.Turn(ref, count)
		}
	}
	return nil
}

func claudeAssistantText(raw []byte) []string {
	var event struct {
		Message struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal(raw, &event) != nil {
		return nil
	}
	var text []string
	for _, block := range event.Message.Content {
		if block.Type == "text" && block.Text != "" {
			text = append(text, block.Text)
		}
	}
	return text
}

func claudeDeltaText(raw []byte) (string, bool) {
	var event struct {
		Event struct {
			Type  string `json:"type"`
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
		} `json:"event"`
	}
	if json.Unmarshal(raw, &event) != nil || event.Event.Type != "content_block_delta" || event.Event.Delta.Type != "text_delta" || event.Event.Delta.Text == "" {
		return "", false
	}
	return event.Event.Delta.Text, true
}

// claudeInitModel is the model named by the runtime's session-start event. An
// init that names none, and every other `system` subtype, project nothing: a
// model kept could not read is missing, never guessed from configuration.
func claudeInitModel(raw []byte) (string, bool) {
	var event struct {
		Subtype string `json:"subtype"`
		Model   string `json:"model"`
	}
	if json.Unmarshal(raw, &event) != nil || event.Subtype != "init" {
		return "", false
	}
	model := strings.TrimSpace(event.Model)
	if model == "" {
		return "", false
	}
	return model, true
}

// claudeWindows reads the unified usage windows out of one rate-limit event.
//
// All of them or none: a window whose utilization kept cannot read makes the
// whole reading missing rather than partial, because a partial reading is a
// receipt that answers *what is left* with one window silently dropped. The
// native reading is a JSON object, so the result is sorted — an order taken
// from map iteration would differ between two readings of one event.
func claudeWindows(raw []byte) ([]Window, bool) {
	var event struct {
		Info struct {
			Unified map[string]struct {
				Utilization json.RawMessage `json:"utilization"`
				ResetsAt    int64           `json:"resetsAt"`
			} `json:"unifiedWindows"`
		} `json:"rate_limit_info"`
	}
	if json.Unmarshal(raw, &event) != nil || len(event.Info.Unified) == 0 {
		return nil, false
	}
	windows := make([]Window, 0, len(event.Info.Unified))
	for name, reading := range event.Info.Unified {
		utilization := strings.TrimSpace(string(reading.Utilization))
		if name == "" || !decimal(utilization) {
			return nil, false
		}
		window := Window{Name: name, Utilization: utilization}
		if reading.ResetsAt > 0 {
			window.ResetsAt = time.Unix(reading.ResetsAt, 0).UTC()
		}
		windows = append(windows, window)
	}
	sort.Slice(windows, func(i, j int) bool { return windows[i].Name < windows[j].Name })
	return windows, true
}

func claudeResultNumber(raw []byte, field string) (string, bool) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return "", false
	}
	src, ok := fields[field]
	if !ok {
		return "", false
	}
	amount := strings.TrimSpace(string(src))
	value, err := strconv.ParseFloat(amount, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return "", false
	}
	return amount, true
}

func claudeResultTurns(raw []byte) (uint64, bool) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return 0, false
	}
	src, ok := fields["num_turns"]
	if !ok {
		return 0, false
	}
	count, err := strconv.ParseUint(strings.TrimSpace(string(src)), 10, 64)
	if err != nil || count == 0 {
		return 0, false
	}
	return count, true
}

// redactClaude replaces every form of the injected credential the packet check
// already refuses, so the two cannot disagree about what counts as the secret
// appearing. A token echoed back inside a JSON string is escaped, and matching
// only the raw bytes would retain it.
func redactClaude(src, secret []byte) (json.RawMessage, Redaction) {
	redacted := append(json.RawMessage(nil), src...)
	matches := 0
	for _, form := range secretForms(secret) {
		count := bytes.Count(redacted, form)
		if count == 0 {
			continue
		}
		matches += count
		redacted = bytes.ReplaceAll(redacted, form, []byte("[REDACTED]"))
	}
	if matches == 0 {
		return redacted, Redaction{}
	}
	return redacted, Redaction{Occurred: true, Matches: matches}
}
