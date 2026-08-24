---
id: 0059-an-out-of-scope-path-is-found-in-the-diff
type: decision
title: 0059 — An out-of-scope path is found in the diff, not in the sandbox
links:
  - { target: decision:0047-a-promise-binds-what-a-repair-may-touch, relation: extends }
  - { target: decision:0017-the-box-is-not-the-jail, relation: depends-on }
  - { target: decision:0023-what-may-be-captured, relation: depends-on }
  - { target: policy:out-of-scope-paths-are-never-landed, relation: implements }
---

# 0059 — An out-of-scope path is found in the diff, not in the sandbox

## Context

[`policy:out-of-scope-paths-are-never-landed`](../policy/out-of-scope-paths-are-never-landed.md)
says work outside a promise's bound repair scope is **reported and stays in the
box**. Increment 5 asks for that clause and nothing demonstrated it.

The plausible reading was that confinement already enforces it: the profile's
writable set is the promise's repair scope, so a write outside it is refused
before it exists. That reading is half right, and the half that is wrong is the
half a reviewer would never see.

**A repair scope is a glob and a filesystem grant is a path.** `internal/gather/**/*.go`
has no path expression; the coarsest thing a profile can name is the directory
`internal/gather`. So a rendered grant is always at least as wide as the scope
and usually wider, and the gap between them is not reported by anything.

Measured, in a real box under nono 0.74.0, with `Profile.Write` naming
`<box>/internal` — which is what both live harnesses render — and a scope of
`internal/*.go`:

```
in-scope-go=written           internal/scratch.go
grant-but-not-glob=written    internal/notes.txt
deeper-than-glob=written      internal/sub/deep.go
outside-grant=refused         docs/escaped.md
```

Two of those three writes are outside what the promise bound. All three reached
`box.Diff`, in the same shape, with nothing distinguishing them:

```
diff --git a/internal/notes.txt b/internal/notes.txt
diff --git a/internal/scratch.go b/internal/scratch.go
diff --git a/internal/sub/deep.go b/internal/sub/deep.go
```

So kept today is in two states at once. Outside the grant: **refused, and
silent** — nothing kept keeps records that it was tried. Inside the grant and
outside the glob: **landed in the box, and undistinguished**. The first
satisfies *not landed* and fails *reported*; the second fails both.

### The provider's audit trail is not the report

`Nono.start` passes `--no-audit`, and nono documents an audit trail, a
Merkleized append-only event log, and `--audit-integrity` "filesystem-state
hashing over in-scope writable paths". The obvious question is whether kept is
switching off the record that answers this clause.

It is not. Measured against nono 0.74.0 on macOS, with the audit left on and
kept's own rendered profile, on a session that performed two writes the profile
denies:

| | |
|---|---|
| events | `session_started`, `sandbox_runtime`, `session_ended` — three, all session-level |
| `session_started` | timestamp, the entrypoint's full argv |
| `sandbox_runtime` | timestamp, platform, `tool_sandbox_active` |
| `session_ended` | timestamp, exit code |
| `session.json` | `executable_identity` (path + sha256), `tracked_paths` (the resolved grants), `network_events`, `command_policy_summary`, `merkle_roots`, `audit_integrity` |
| filesystem events | **none**, granted or denied |

Neither denied write appears — not as an event, not in a summary, not as a
count. There is no filesystem event vocabulary at all: no path, no operation,
no time. Nor is the denial anywhere else. Read with `--silent` off, nono's
stderr carries only the capability banner; read at `RUST_LOG=trace` through
`--log-file`, the 286-line log mentions the denied path exactly once, inside
the argv it echoes when it execs. That is structural rather than a gap somebody
will close: on macOS the policy is enforced by the kernel's Seatbelt, so the
denial happens in a process nono is not in.

`--audit-integrity` is measured too, and it is the wrong shape twice over. It
adds two `merkle_roots` — one before, one after — and narrows `tracked_paths`
to the **writable** set. So it attests, as one opaque number, that the in-scope
writable tree changed: exactly the set that is not the question, named to no
path, over no operation. It says nothing about anything outside the writable
set, which is where an out-of-scope write would be, and `box.Diff` already
answers the same question per path and with content.

## Decision

**The report is derived from the box's changed paths against the promise's
bound repair scope. Confinement is an outer bound and never the report, and the
provider's audit trail is neither.**

Three parts.

**`internal/box` reports the paths and the diff from one snapshot.** `Box.Change`
replaces `Box.Diff` and returns both. Two methods would take two snapshots, and
a box that moved between them yields a path list and a diff that disagree —
silently, in the direction where a path is missing from the list a review is
made against.

**The paths come from git, not from the diff text.** `git diff --name-only -z`
against the same tree pair. Parsing `+++ b/` out of the diff — which is what
the live harnesses did — misses a delete, whose header names `/dev/null`, and a
repair that deletes a file outside its scope is the case the policy exists for.
It also returns nothing at all for a diff past its bound.

**`Change.Against(scope)` is the one classification, and it fails closed.** A
scope that names nothing covers nothing, so every changed path is outside it.
`Review.Bounded` is what a land will refuse on, and it is what a report is
rendered from now, so the increment-5 report and the increment-6 gate cannot
come to disagree about what "in scope" means.

**kept keeps passing `--no-audit`**, and now for a measured reason rather than
an unexamined default. Turning it on buys no evidence for this clause and costs
something under [0023](0023-what-may-be-captured.md): it durably records the
entrypoint's full argv and the resolved grant paths, unredacted, in a store
outside kept's ledger that kept does not own, does not bound and cannot prune,
with no redaction pass and no record that redaction occurred. Today nothing
sensitive is in that argv — the Claude argv is a pinned constant list plus the
reviewed constant ask of [0058](0058-the-ask-is-declared-and-travels-beside-the-packet.md),
and the packet travels on stdin — but that holds by construction and nothing
enforces it. 0023's whole discipline is that what becomes durable is chosen and
scanned; a second durable stream nobody chose is what it exists to prevent.
[0020](0020-durable-when-irreproducible.md) points the same way: it makes the
transcript durable because it is irreproducible and it is the assessor's
evidence, and this trail is neither — it is reproducible session metadata kept
already holds in its own receipt, where the argv is derivable from a pinned
constant, the exit is `Lifecycle.Exit` and the timestamps are the envelope's.

## Alternatives

**Render the grant so it matches the scope exactly.** Impossible, not merely
expensive: a filesystem grant names a path and `internal/gather/**/*.go` is not
one. Narrowing the grant to the files the glob matches *at render time* would
also refuse the repair its own new file, which is a legitimate repair.

**Report only what the sandbox refused.** Nothing to report from — measured
above, at the event log, on stderr and at trace level. And it is the wrong
subject: the policy is about work that *stays in the box*, and a refused write
is work that never happened.

**Record the review in the thread receipt.** Rejected for now. A receipt entry
is runtime evidence or a parent-observed lifecycle fact, and this is neither —
it is a derivation over the box, and 0032's rule is that a derived answer is
computed on read rather than stored. It is also not re-derivable once the box
is discarded, which argues the other way; that tension belongs to increment 6,
where a land record says what it was gated against.

**Filter the out-of-scope paths out of the diff.** Rejected by the policy
itself: landing what is in scope and dropping the rest silently produces a
half-repair the gate then judges as if it were the whole one.

## Consequences

- **`Box.Diff` is gone**, replaced by `Box.Change`. Every caller was a test.
- **Confinement's role is stated rather than assumed.** It refuses what its
  vocabulary can express and no more, and `internal/confine`'s own boundary
  test now measures both sides: the write the grant refuses, and the write it
  admits that the scope does not.
- **The clause is satisfied for the work it is about**, which is work in the
  box. Work the sandbox refused is not reported and cannot be, and that is
  recorded here rather than left to be rediscovered.
- **A grant wider than its scope is now legible in the report** instead of
  being invisible in the profile. Nothing yet derives `Profile.Write` from
  `RepairScope` — both live harnesses hand-write it — so when a dispatcher does,
  this is the check that says how much the rendering gave away.
- **`nono profile`'s audit family joins the surface inventory** with a measured
  row rather than an assumption, in [`docs/nono-surface.md`](../nono-surface.md).
