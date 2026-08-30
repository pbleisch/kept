---
id: 0072-a-runtime-version-constraint-is-a-review-record
type: decision
title: 0072 — A runtime version constraint is a review record
fields:
  statement: >-
    A runtime adapter states the versions a person reviewed and, where its evidence supports
    one, the line those sit on; an operator states whether an unreviewed version of that line
    may be dispatched, and a session that runs on one is recorded as unreviewed in every event
    it writes.
links:
  - { target: decision:0052-repair-runtimes-are-closed-adapters-with-native-receipts, relation: extends }
  - { target: decision:0048-the-operator-is-named-and-never-guessed, relation: extends }
  - { target: decision:0009-repairs-are-not-enumerated, relation: depends-on }
---

# 0072 — A runtime version constraint is a review record

## Context

[0052](0052-repair-runtimes-are-closed-adapters-with-native-receipts.md) decided
that "a version outside the adapter's supported **range** is a dispatch
refusal", and listed "the documented executable/version range" among the
per-adapter configuration decisions. What was built was narrower: one exact
patch version per adapter, compared for equality.

The comment on that pin named exactly what it protected — the runtime's
**direct stream shape** and its **documented credential precedence** — and said
what a bump costs: it "moved from 2.1.240 to 2.1.241 against the installed
binary rather than by assumption".

Two things then happened. The installed Claude Code became 2.1.251, so every
dispatch refused and self-hosting had no repair verb. And the pin's guarantee
turned out to be weaker than it reads.

**The two properties do not fail the same way.** The stream shape fails
*loudly*: a flag the runtime no longer parses exits before a session exists —
measured once already, when `--output-format stream-json` was found to require
`--verbose` — and an event language that moved produces a receipt with no
projections. The credential precedence fails *silently*: a parent environment
name kept does not refuse hands the child a different credential, or the same
one aimed elsewhere, and the session looks exactly like one that worked.

**Measured, the exact pin did not deliver the silent half.** Reading both
builds' `process.env` surface, 2.1.251 differs from 2.1.241 by seven names added
and thirty-nine removed, and no added name is a credential or redirect channel
on the platform [0053](0053-a-confined-repair-resolves-its-toolchain.md) confines
a repair on. But every credential-bearing name `claudeRefusedEnvironment` omits —
`CLAUDE_CODE_WEBSOCKET_AUTH_FILE_DESCRIPTOR`, the `CLAUDE_BG_*_AUTH` trio, the
artifact and messaging tokens, `CLAUDE_CODE_PROXY_AUTHENTICATE` standing beside
two names that *are* refused — is read by **2.1.241 as well**, the very build the
pin was taken against. That family was assembled by reading two of the runtime's
own enumerations, not by exhausting its environment surface, and pinning the
version it was read from did not make it exhaustive.

So the pin records that a person looked at a build. That is a review, and a
review is worth having. It is not a measurement, and a constraint that reads as
one invites the conclusion that the list beneath it is complete.

A capability probe was considered as the durable alternative, since the durable
check of a behaviour is of the behaviour. It fails on the half that matters. The
loud half is already self-checking, so probing it buys a second detector for a
failure that announces itself — measured, `claude --help` costs 83 ms against a
`--version` call the dispatcher already makes at 7 ms, on repair sessions that
run 18 to 63 seconds. And the silent half **cannot** be probed: establishing
where a credential goes means spending a request against a live upstream, which
[0052](0052-repair-runtimes-are-closed-adapters-with-native-receipts.md) forbids
an adapter from doing. Reading the names out of the image is not a probe but an
inspection, and `docs/nono-surface.md` already records that a literal scan of a
bundle under-reports — the unsafe direction. **The credential half is
irreducibly a human review step**, and the honest thing is to name it one.

## Decision

**An adapter states a review record, not a pin.** `support` carries the versions
somebody examined, and optionally the `major.minor` line those sit on. An empty
line means the reviewed set is the whole of what the adapter accepts — the
honest answer for an experimental wire contract, or a runtime whose proof cannot
cheaply be re-run. Codex and Pi declare none; Claude declares `2.1`.

**Membership is earned against the binary**, and what was checked is recorded
beside the entry. It is not
[0052](0052-repair-runtimes-are-closed-adapters-with-native-receipts.md)'s
admission bar — a live confined interrupted session, held per adapter by
`claudeAdmitted` — which asks whether the adapter may start the runtime at all.
This asks whether *this build of it* is the one that was read.

**The operator states the policy, in their own config.** `repair.runtime_versions`
is `reviewed` or `line`. Unset is `reviewed`. It is in the per-machine file
rather than the tracked one for [0048](0048-the-operator-is-named-and-never-guessed.md)'s
reason: the installed runtime is a fact about one machine, so which of its
versions are acceptable is one person's risk posture, and a tracked answer would
be imposed on every clone. It also travels beside the executable it qualifies.

**There is no policy that accepts anything.** Off the line, older than the
oldest reviewed, unreadable as three numbers, or an unrecognized policy name:
refused under every policy, by name. `line` widens forwards along one line and
in no other direction.

**A session on an unreviewed version says so in every event it writes.** The
adapter version stamped on each native receipt event is composed from the
version that actually ran and carries `+unreviewed` when nobody read it. Under
an exact pin the recorded and running versions were forced equal and a constant
could not be wrong; the moment a policy admits more than one, a constant claims a
session that did not happen. Reviewedness is computed once, by the same call that
admits, because two computations of it would disagree in the direction that
writes the misleading receipt.

**2.1.251 joins the reviewed set** on a static review matching the bar the
2.1.240→2.1.241 bump used, plus the version-to-version diff a second entry makes
possible: all ten pinned argv elements present and listed in its own `--help`,
every token `claudeStream` projects present in both builds, and the environment
diff above.

## Consequences

- A dispatch proceeds on the installed 2.1.251 under the default policy, with
  no operator configuration. Self-hosting's repair verb is unblocked.
- The next patch release refuses by default, and the refusal names
  `repair.runtime_versions: line` as what would take it. That is the
  inconvenience being kept deliberately: the alternative is a constraint that
  goes quiet exactly when the surface it guards has moved.
- **An operator who sets `line` accepts a real exposure**, and it is the silent
  half: a build nobody read may add a credential channel this adapter does not
  refuse. What that policy buys is that the exposure is *recorded* rather than
  assumed away — the receipt says which build ran and that nobody read it — so a
  session's evidence never means something else. The refusal it replaces was
  honest; this is the smallest thing that is still honest and also dispatches.
- **The refused-family gap is now visible and is not closed here.** Several
  credential-bearing names are read by both builds and refused by neither. That
  is a defect this decision documents rather than repairs, because closing it is
  an audit of a runtime's environment surface and not a change to how versions
  are constrained. See open question 12.
- `ClaudeVersion` becomes the newest reviewed version rather than the only
  accepted one, and callers asking whether a build is usable ask
  `ClaudeSupports`. A test or skip guard comparing against the newest would
  retire evidence the day a newer entry lands.
- Adding a reviewed entry costs a review and no re-ratification: the set is
  code, and nothing in a hash list names it.
- Codex and Pi are strictly unchanged in what they accept. The mechanism is
  shared so the policy is one thing rather than three, and each adapter states
  what its own evidence supports — which for both is a set with no
  neighbourhood.

## Alternatives

**Bump the pin to 2.1.251.** The cheapest thing, and it is what the previous
bump did. It restores dispatch today and refuses again on 2.1.252, which is the
problem rather than a solution to it. It also leaves the guarantee misread.

**A minor-line constraint alone, with no reviewed set.** This is the range
[0052](0052-repair-runtimes-are-closed-adapters-with-native-receipts.md)
literally licenses, and it is a proxy that assumes patch releases do not move
what the adapter reads. Measured, patch releases *do* move it — 46 names changed
between two of them. Without a reviewed floor there is also nothing to be newer
than, so an ancient 2.1.0 would be admitted by a constraint written to admit a
newer build.

**A verified set with no line.** Honest, and it is what Codex and Pi get. For
Claude it degrades to the current refusal the day a patch ships, which is the
thing being fixed.

**A capability probe on every dispatch.** Rejected on the reasoning above: it
cannot cover the half that fails silently, and the half it can cover already
fails loudly.

**A probe that runs once and caches.** A third mechanism with a staleness
question of its own, and a cache that can disagree with its source — which
[0001](0001-standing-and-truth-live-in-the-ledger.md) and
[0032](0032-standing-is-a-fold-over-acts.md) both refuse for
standing, for the same reason.
