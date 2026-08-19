---
id: 0026-nothing-external-is-unbounded
type: decision
title: 0026 — No external invocation is unbounded
---

# 0026 — No external invocation is unbounded

**Status** accepted · a cross-cutting rule; the reactions were each decided
elsewhere

## Context

kept's work is almost entirely other people's programs: gatherers, build tools,
test suites, an agent runtime, git, and a confinement provider. Every one of
them can hang, and a hang is worse than a failure because nothing reports it.
The system is designed to run unattended, so a wedged process is a queue that
has stopped without saying so.

Timeouts have been mentioned twice in passing —
[0013](0013-the-gate-makes-concurrency-safe.md) notes that a wedged `flock`
holder is handled by giving up after one, and
[0015](0015-landing-is-a-merge-queue.md) bounds the gate inside its critical
section — and never stated as a rule.

## Decision

**Every invocation of something kept does not control has a deadline and a
defined reaction. There is no default of waiting.**

The reactions are not new. Each was decided for its own reasons, and a timeout
resolves to the same outcome as the corresponding failure:

| Times out | Reaction | Decided in |
|---|---|---|
| a gatherer | no observation, so the verdict is `unknown` | 0004, and the invariant in `AGENTS.md` |
| an agent dispatch | thread interrupted, box discarded, cost and partial transcript kept | [0018](0018-what-survives-a-crash.md), [0020](0020-durable-when-irreproducible.md) |
| the gate inside the land lock | release the lock, retry | 0015 |
| acquiring a lock | refuse, and record the disposition | 0013, [0019](0019-a-refusal-is-a-disposition.md) |
| a git fetch or push | the ledger is stale; refuse to act, keep gathering | 0013, [0016](0016-the-ledger-lives-on-its-own-ref.md) |
| the confinement canary | treated as a failed probe, so no dispatch | [0017](0017-the-box-is-not-the-jail.md) |

Two rules keep it honest:

**A timeout is reported as a timeout.** A gatherer that timed out and one that
was never installed both yield `unknown`, and the operator must be able to tell
them apart from the record. This is the same distinction 0019 makes for
refusals: the outcome may be identical while the cause is not.

**A killed process is killed, including its children.** A gatherer that shells
out leaves a process group; abandoning the parent and calling it a timeout
leaves a build running that will corrupt the next one's output.

Deadlines are configured per kind with defaults, because a test suite and a
`git fetch` do not belong on the same clock, and an agent dispatch belongs on a
much longer one than either.

## Alternatives

**Rely on the caller.** Rejected: unattended means there is no caller watching.

**One global timeout.** Rejected: any value long enough for an agent dispatch
is far too long for a lock acquisition, and any value short enough for a fetch
kills every real test suite.

**Treat a timeout as a failure of the promise.** Rejected — it is the existing
invariant restated: a machine that could not measure does not record what it
thinks it saw.

## Consequences

- **Every external call site gets a deadline argument**, which is a small tax on
  every one of them and the reason to state it as a rule rather than remember it
  case by case.
- **Timeouts are configuration and appear in the record**, so a corpus whose
  promises are all `unknown` because a default is too tight is diagnosable
  rather than mysterious.
- **The reaction table is the test list.** Each row gets a test that induces the
  timeout and asserts the outcome, per `AGENTS.md`.
