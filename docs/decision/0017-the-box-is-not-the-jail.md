---
id: 0017-the-box-is-not-the-jail
type: decision
title: 0017 — The box bounds what may land; confinement bounds what may be touched
---

# 0017 — The box bounds what may land; confinement bounds what may be touched

**Status** accepted · separates two things an earlier draft of
[0013](0013-the-gate-makes-concurrency-safe.md) conflated

## Context

A box is a git worktree, and the temptation is to call it the isolation
boundary and stop. It is not one. A worktree is a directory: a process with its
working directory set there can `cd ..`, write `$HOME`, read every credential
on the machine, reach any host, and write to the origin repository directly —
and the box would never know.

The two boundaries do different jobs, and grft — which is in `lineage.md` as a
predecessor — separated them and wrote down why:

> The box decides what may *land*, the jail decides what may be *touched*.

`nono` arrives at the same separation from the other side, and shows where the
confinement half goes when taken seriously: filesystem scoping, network
allowlisting, and a **credential broker** that gates secrets per tool rather
than exposing them to the agent, with policy in a profile rather than in the
prompt, and tools the agent delegates to confined separately from the agent
itself.

There is a sharper reason kept cannot defer this the way grft did. grft parked
everything but a write allow-list, and named the condition for resuming:

> Resume when either becomes true: a session needs to run against a repository
> whose contents the operator is not willing to expose, or **sessions start
> running unattended.**

Unattended is kept's normal mode. The premise is a queue that drains without
you. So kept crosses grft's own resume threshold on day one, and a write
allow-list — under which an agent can still read every credential on the
machine and post it anywhere — is not the finished answer here even though it
was a reasonable place for grft to pause.

## Decision

**The box is a tree. Confinement is a separate, explicitly configured policy
over resources. They compose and neither substitutes for the other.**

| | bounds | mechanism |
|---|---|---|
| **box** | what may reach the repository | a worktree, plus scope enforcement at land, plus the gate |
| **confinement** | what the process may touch | a named provider enforcing a policy |

Confinement policy names its resources separately, because they fail
separately:

- **filesystem** — writable paths; everything else read-only
- **network** — reachable hosts, and whether egress goes through a proxy
- **credentials** — what the process holds, and what is brokered to a
  named tool instead of held
- **process** — what may be executed, and what confinement a delegated tool
  inherits

**Confinement is configured, never inferred, and `unset` is not `none`.** A run
refuses to dispatch an agent until a provider is named. `none` is a legal value
and records a decision; a typo must not silently produce an unconfined agent.
This is grft's answer, arrived at by use, and it is adopted rather than
re-derived.

**A provider is never trusted on its name.** Before an agent runs, a canary
executes inside the jail, attempts each escape the policy claims to prevent,
and requires refusal. This is `AGENTS.md`'s own rule — a check that has never
fired is not a check — applied to the boundary, and it catches the failure that
matters: a profile subtly wrong, or a provider that silently did nothing on the
wrong platform, produces an agent that runs perfectly well and a thread record
saying it was contained.

### Two specifics kept must not get wrong

**No push credentials in a box.** 0015 guarantees every commit on
`origin/main` was gated. An agent holding push access to `origin` walks around
that guarantee entirely, and the guarantee becomes decorative. A box's git
remote is the local repository, and the agent's environment does not carry a
credential that reaches the origin.

**The ledger is not reachable.** [0016](0016-the-ledger-lives-on-its-own-ref.md)
already keeps it off every checked-out ref, so confinement is the second of two
independent mechanisms rather than the only one.

### The provider is `nono`, and kept does not build a jail

Confinement is a solved problem kept has no reason to solve again. `nono`
already does the four resources — filesystem scoping, network allowlisting,
credential brokering through a proxy, and separate policies for the tools an
agent delegates to — on darwin, linux and WSL2, with no daemon and no
container. grft's experience is the argument: the jail is where a project's
time goes to be parked, and grft's own confinement work stalled at a write
allow-list for exactly that reason.

So kept **shells out** rather than writing profiles of its own. The `Jail`
interface stays, because `none` must remain expressible and a native provider
must remain possible, but no native provider is built.

### The profile is kept's; nono receives a projection of it

Delegating the mechanism is not delegating the model. kept's confinement
profile is its own data structure, and `nono`'s configuration is a **subset**
rendered from it, because kept has to express things that are not a sandbox's
business:

- a **derived** writable set — a promise's `RepairScope` bounds what a repair
  may change, so the writable paths follow from the artifact rather than from
  static config
- confinement for **gatherers**, not only agents; a gatherer is a subprocess
  running project code and it is not automatically more trusted than a repair
- the **canary probe set** that verifies this particular profile
- the profile's own **content hash**, so a thread record can say which profile
  was in force and mean it

**A provider that cannot render some part of the profile refuses**, and the run
does not dispatch. This is the same reasoning as `unset` not meaning `none`: a
profile silently narrowed to what the provider happens to support produces an
agent that runs perfectly well and a record claiming a boundary that was not
there.

### The v1 bar

Unattended is day one, so two are not negotiable and two are staged:

| | |
|---|---|
| **must** | writes confined to the box |
| **must** | no credential in the box that reaches `origin` |
| **must** | `kept` itself not executable in the box ([0024](0024-signing-is-designed-and-deferred.md)) |
| should | network egress restricted to the hosts the agent needs |
| later | credential brokering per delegated tool |

The first two are what the rest of the model already depends on. An agent that
can write outside its box has bypassed the scope check; an agent that can push
to `origin` has bypassed the gate, and [0015](0015-landing-is-a-merge-queue.md)
becomes decorative.

### The canary's probes

Four, and each is the negation of a guarantee stated elsewhere:

| Probe | Guarantee it defends |
|---|---|
| write outside the box | scope enforcement at land |
| write `.kept/**` | standing is minted only by a person ([0016](0016-the-ledger-lives-on-its-own-ref.md)) |
| push to `origin` | every commit on `main` was gated (0015) |
| read a credential path | the agent holds no secret it was not given |
| run `kept ratify` | standing is minted only by a person (0024) |

A probe that cannot be attempted counts as a failure, not as a pass.

## Alternatives

**The box is the boundary; confine nothing.** Rejected: it protects the
repository and nothing else, and the thread record would claim containment that
does not exist.

**Confine, and skip the box.** Rejected: they are orthogonal. A perfectly
jailed agent that writes the working tree directly has still bypassed the gate.

**Ask per run.** Rejected with grft: one explicit setting, and unattended runs
have nobody to ask.

**Build a native jail.** Rejected: `sandbox-exec`, namespaces and seccomp are
each a project, and grft demonstrates what happens to that project when it
competes with the work the tool actually exists to do.

**Adopt nono's configuration as kept's model.** Rejected: kept's profile has to
express a writable set derived from a promise's scope, and confinement for
gatherers as well as agents. Those are kept's concepts and would have nowhere
to live.

**Defer confinement to a later increment, as grft did.** Rejected on the
condition grft itself named. Its reasoning was sound for a tool with a person
watching every session; kept's premise removes the person.

## Consequences

- **A thread record carries the provider and the canary's report.** A repair
  contained by a jail that was not actually confining is a different fact from
  one that was, and the assessor should be able to see which.
- **Confinement can refuse to run**, on an unnamed provider, an unavailable
  provider, or a failed canary. That is a dispatch refusal, and it is reported
  as such rather than as drift that could not be repaired.
- **The credential question has no clean answer yet.** Deny the keychain and the
  agent cannot authenticate; allow it and the agent holds its own key. Brokering
  is the direction — `nono`'s proxy is the worked example — and it is not a v1
  requirement. It becomes one at the same moment the read policy does.
- **kept depends on `nono`.** `AGENTS.md` requires a dependency to be a
  decision with an ADR, and this is it. The exposure is bounded by the `Jail`
  interface and by the profile being kept's own: replacing the provider means
  writing a second renderer, not rethinking the model.
- **`none` remains legal and remains a decision.** A repository whose operator
  accepts an unconfined agent records that choice rather than defaulting into
  it.
