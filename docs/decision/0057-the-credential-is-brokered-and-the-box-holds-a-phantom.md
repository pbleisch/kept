---
id: 0057-the-credential-is-brokered-and-the-box-holds-a-phantom
type: decision
title: 0057 — The credential is brokered, and the box holds a phantom
fields:
  statement: >-
    A repair runtime's inference credential reaches its upstream through the confinement
    provider's credential route rather than through kept: the profile names a reference the
    provider resolves, an exact HTTPS upstream, the child variable the phantom is published
    under, and a default-deny endpoint allowlist. kept never reads the secret, never renders
    it, and cannot redact it because it never has it. The operator's reference and the date
    the credential lapses are per-machine configuration, and a dispatcher outside the adapter
    package is what assembles them.
links:
  - { target: decision:0052-repair-runtimes-are-closed-adapters-with-native-receipts, relation: extends }
  - { target: decision:0056-a-native-runtime-declares-nothing-and-names-its-own-scratch, relation: extends }
  - { target: decision:0017-the-box-is-not-the-jail, relation: extends }
  - { target: decision:0048-the-operator-is-named-and-never-guessed, relation: depends-on }
---

# 0057 — The credential is brokered, and the box holds a phantom

## Context

[0052](0052-repair-runtimes-are-closed-adapters-with-native-receipts.md) permits
two credential channels and no others: a named child environment value, and a
brokered route where the provider injects the secret and the child never holds
it. The first is built — `CLAUDE_CODE_OAUTH_TOKEN` — and
[0017](0017-the-box-is-not-the-jail.md) accepts its cost in as many words: a
tool-capable agent inside the box can read its own credential. The second had
never been used by anything, and the reason it stayed unused was that nobody
knew whether a real runtime would work against a phantom.

Claude Code starts, runs and is measured in a box as of
[0056](0056-a-native-runtime-declares-nothing-and-names-its-own-scratch.md).
What it does not have is a credential. So the question had to be settled before
either channel was wired, and it was settled by measurement rather than by
reading the provider's documentation.

**The measurement needed no valid token, which is what made it possible at all.**
A deliberately invalid credential carried through a real credential route
answers the question completely: either the request arrives upstream with the
header rewritten, or it does not.

Four things were measured, each against nono 0.74.0 and Claude Code 2.1.241.

**The rewrite happens, and it was read at the upstream rather than inferred.**
A 401 from `api.anthropic.com` proves the host was reached but cannot tell a
rewritten header from a phantom passed straight through: measured both ways, the
upstream answers `OAuth access token is invalid` for *any* bearer value, so the
message is not a discriminator. Pointing the route's upstream at a loopback
listener and reading the request it received is. The child sent
`Authorization: Bearer <phantom>`; the listener received
`Authorization: Bearer <the real value>`.

**A real runtime completes a session through it.** Claude Code 2.1.241, confined,
reached `result: success` — init, the replayed packet, streamed deltas, an
assistant turn, a rate-limit event and a result — with the real credential
present only in the supervisor's resolution of the reference. The provider
publishes the phantom in the variable the profile names and rewrites the header
on egress. Nothing in the box ever held the secret.

**The phantom is a capability, not decoration.** A request carrying the wrong
bearer value, or none, is refused by the proxy with 401 and never reaches the
upstream. So the phantom is what authorizes use of the credential, and it is
scoped to one session and reachable only from inside that box.

**One endpoint is what a repair calls.** A complete session called
`POST /v1/messages?beta=true` and nothing else on that host. `endpoint_rules` are
default-deny and were measured as such: an unnamed path and an unnamed method
each returned 403 without reaching the upstream, and a path rule matches with a
query string present.

Two constraints came out of the same measurements and are not optional.
`network.block: true` and a credential route are refused by the provider as
contradictory — *credential injection requires the proxy to be reachable* — so a
box with a brokered credential names its host and cannot block. And a profile
carrying a credential route but no domain allowlist reaches every host on the
internet, which is the trap decision 0017 already names for an empty allowlist,
arriving by a second door.

## Decision

**The brokered route is how a repair runtime's inference credential reaches its
upstream.** `Profile.Credential` is one optional typed route carrying the
provider's service name, an exact HTTPS upstream, the reference the provider
resolves, the child variable the phantom is published under, the header and
format it is injected with, and a non-empty endpoint allowlist. The renderer
emits `network.credentials` and `network.custom_credentials` from it and from
nothing else.

**The profile never holds a credential value — only a reference.** kept refuses
a reference that is not a `scheme://` URI, because a bare account name cannot be
told apart from a pasted secret. It refuses `env://` by name, because that scheme
resolves against the supervisor's environment, which is kept's own process — the
one thing this decision exists to avoid. It refuses `cmd://`, because that runs a
program nobody reviewed to produce a secret.

**The profile owns the phantom variable, and an invocation may not name it.**
This is the rule `Home` and `TempEnv` already have, for the reason they have it:
a variable with two sources has one value and two policies. An adapter that set
the credential variable itself would be the direct channel wearing the brokered
one's clothes, and the launch refuses it by name.

**A brokered adapter carries no token.** `RuntimeConfig` gains a sibling field
saying which channel is in force; exactly one of the two may be. Under the
brokered one the adapter's child environment omits the credential entirely, and
it has no secret to redact — which is a statement about what it *cannot* leak
rather than a weakening of the direct channel's redaction.

**The reference and the lapse date are per-machine configuration.**
[0048](0048-the-operator-is-named-and-never-guessed.md) decided this question
already for operator identity: `.kept/config.yaml` is tracked, so a key naming
one person's keychain item would be committed to the repository and be wrong on
every other clone. The credential reference names an item on one machine and the
lapse date describes one minting of one token, so both live beside `operator` in
the user's own config.

**Configuration states when the credential lapses, not when it was minted.**
`claude setup-token` takes no options, prints no expiry and has no revoke, so the
lifetime is neither selectable nor readable back. A mint date plus a lifetime kept
assumed would be a second place the answer lived, one kept cannot check and one
that goes silently wrong if the vendor changes the lifetime. The lapse is what the
existing refusal needs, so the lapse is what is written down. Nothing reads the
token to answer it.

**A dispatcher outside `internal/thread` assembles all of it.** 0052 forbids an
adapter from reading a keychain, an auth database or the parent environment;
`internal/repair` is where configuration becomes a `RuntimeConfig` and a
`Profile`, and the adapter package keeps no route to any of those sources. The
boundary is structural rather than a comment: the adapter's package cannot reach
the credential, because under this channel the credential never enters the
process.

## Consequences

**kept cannot redact the phantom, and says so.** The provider mints it and never
tells kept, so a session that echoes its own environment into a tool result puts
the phantom in the receipt. That is a smaller exposure than the direct channel's
— the phantom dies with the session and is useless outside that box — but it is
not nothing, and nothing in kept can remove it.

**`nono why` is not a complete account of this either.** 0056 already records
that the provider's command policy gives the launched entrypoint an exec bypass
the explain tool does not report. The credential route adds a second such gap:
the pre-flight asks about paths, and a credential route is not a path. What
proves the boundary is the measurement above and the canary's existing
`host_outside_allowlist` probe.

**A box with a brokered credential cannot block the network.** It names its host
instead. That is one more host than a Go build box needs and it is the same host
0056 already named for this runtime, so nothing widens — but the two settings are
now mutually exclusive in a way the provider enforces and kept refuses earlier,
with the reason.

**The endpoint allowlist is what was measured and no wider.** A repair that
reaches for an endpoint nobody measured is refused with a 403 that appears in the
receipt. That is a legible failure and a deliberate one: granting
`/v1/messages/**` would grant endpoints the runtime image references and no
measured session called.

**A keychain-backed reference costs a one-time authorization, measured.** The
provider resolving a macOS keychain item raises the system's own approval dialog
on first access and blocks until it is answered. That is the operator's to grant
once, and it is a reason a repair dispatched with nobody at the machine can hang
on its first run rather than fail. It is not a reason to hold the secret
somewhere kept can read.

**This does not admit Claude.** 0052 wants a real, confined, deliberately
interrupted session with a real credential, and `claudeAdmitted` stays false. The
route is proven; a valid token is what is missing, and only the operator can mint
one.
